package team

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nex-crm/wuphf/internal/config"
	"github.com/nex-crm/wuphf/internal/provider"
)

var (
	headlessCodexLookPath       = exec.LookPath
	headlessCodexCommandContext = exec.CommandContext
	headlessCodexExecutablePath = os.Executable
	headlessCodexRuntimeRunTurn = func(l *Launcher, ctx context.Context, slug, notification string, channel ...string) error {
		return l.runHeadlessCodexTurn(ctx, slug, notification, channel...)
	}
	headlessClaudeRuntimeRunTurn = func(l *Launcher, ctx context.Context, slug, notification string, channel ...string) error {
		return l.runHeadlessClaudeTurn(ctx, slug, notification, channel...)
	}
	headlessGeminiRuntimeRunTurn = func(l *Launcher, ctx context.Context, slug, providerKind, notification string, channel ...string) error {
		return l.runHeadlessGeminiTurn(ctx, slug, providerKind, notification, channel...)
	}
	headlessOllamaRuntimeRunTurn = func(l *Launcher, ctx context.Context, slug, notification string, channel ...string) error {
		return l.runHeadlessOllamaTurn(ctx, slug, notification, channel...)
	}
	headlessOpenClaudeRuntimeRunTurn = func(l *Launcher, ctx context.Context, slug, providerKind, notification string, channel ...string) error {
		return l.runHeadlessOpenClaudeTurn(ctx, slug, providerKind, notification, channel...)
	}
	headlessCodexRunTurn = func(l *Launcher, ctx context.Context, slug, notification string, channel ...string) error {
		kind := provider.KindClaudeCode
		if l != nil {
			kind = l.headlessTurnProviderKind(slug, channel...)
		}
		switch kind {
		case provider.KindCodex:
			return headlessCodexRuntimeRunTurn(l, ctx, slug, notification, channel...)
		case provider.KindClaudeCode:
			return headlessClaudeRuntimeRunTurn(l, ctx, slug, notification, channel...)
		case provider.KindGemini, provider.KindGeminiVertex:
			return headlessGeminiRuntimeRunTurn(l, ctx, slug, kind, notification, channel...)
		case provider.KindOllama:
			return headlessOllamaRuntimeRunTurn(l, ctx, slug, notification, channel...)
		case provider.KindOpenclaude:
			return headlessOpenClaudeRuntimeRunTurn(l, ctx, slug, kind, notification, channel...)
		default:
			return fmt.Errorf("unsupported headless provider %q for @%s", kind, slug)
		}
	}
	// headlessWakeLeadFn is nil in production; override in tests to intercept lead wake-ups.
	headlessWakeLeadFn func(l *Launcher, specialistSlug string)
)

var (
	headlessCodexTurnTimeout              = 4 * time.Minute
	headlessCodexOfficeLaunchTurnTimeout  = 10 * time.Minute
	headlessCodexLocalWorktreeTurnTimeout = 12 * time.Minute
	headlessCodexStaleCancelAfter         = 90 * time.Second
	tomlBareKeyPattern                    = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	headlessCodexEnvVarsToStrip           = []string{
		"OLDPWD",
		"PWD",
		"CODEX_THREAD_ID",
		"CODEX_TUI_RECORD_SESSION",
		"CODEX_TUI_SESSION_LOG_PATH",
	}
	headlessRuntimeProviderAvailable = func(kind string) bool {
		switch normalizeProviderKind(kind) {
		case provider.KindCodex:
			_, _, err := provider.ResolveCodexCLIPath(headlessCodexLookPath)
			return err == nil
		case provider.KindClaudeCode:
			_, err := headlessClaudeLookPath("claude")
			return err == nil
		case provider.KindGemini:
			return strings.TrimSpace(config.ResolveGeminiAPIKey()) != ""
		case provider.KindGeminiVertex:
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, err := provider.ResolveGeminiVertexProject(ctx)
			return err == nil
		case provider.KindOpenclaude:
			_, err := headlessClaudeLookPath("openclaude")
			return err == nil
		default:
			return false
		}
	}
)

const headlessCodexLocalWorktreeRetryLimit = 2
const headlessCodexOfficeRetryLimit = 1
const headlessCodexExternalActionRetryLimit = 1
const headlessPromptEncodingRetryLimit = 1
const headlessPromptNormalizationReplacement = "\uFFFD"

type headlessCodexTurn struct {
	Prompt           string
	Channel          string // channel slug (e.g. "dm-ceo", "general")
	TaskID           string
	Attempts         int
	EnqueuedAt       time.Time
	QuickReply       bool
	ProviderOverride string
}

type headlessCodexActiveTurn struct {
	Turn              headlessCodexTurn
	StartedAt         time.Time
	Timeout           time.Duration
	Cancel            context.CancelFunc
	WorkspaceDir      string
	WorkspaceSnapshot string
}

var headlessCodexWorkspaceStatusSnapshot = func(path string) string {
	path = normalizeHeadlessWorkspaceDir(path)
	if path == "" {
		return ""
	}
	out, err := runGitOutput(path, "status", "--porcelain=v1", "-z")
	if err != nil {
		return ""
	}
	return string(out)
}

func (l *Launcher) launchHeadlessCodex() error {
	killStaleBroker()
	killStaleHeadlessTaskRunners()
	_ = exec.Command("tmux", "-L", tmuxSocketName, "kill-session", "-t", l.sessionName).Run()

	l.broker = NewBroker()
	l.broker.packSlug = l.packSlug
	l.broker.blankSlateLaunch = l.blankSlateLaunch
	l.broker.SetSessionObservabilityFn(l.SessionObservabilitySnapshot)
	if err := l.broker.SetSessionMode(l.sessionMode, l.oneOnOne); err != nil {
		return fmt.Errorf("set session mode: %w", err)
	}
	if err := l.broker.Start(); err != nil {
		return fmt.Errorf("start broker: %w", err)
	}
	if err := writeOfficePIDFile(); err != nil {
		return fmt.Errorf("write office pid: %w", err)
	}

	l.headlessCtx, l.headlessCancel = context.WithCancel(context.Background())
	l.ensureHeadlessQueueMaps()

	l.resumeInFlightWork()
	go l.notifyAgentsLoop()
	if !l.isOneOnOne() {
		go l.notifyTaskActionsLoop()
		go l.notifyOfficeChangesLoop()
		go l.watchdogSchedulerLoop()
	}

	return nil
}

func (l *Launcher) enqueueHeadlessCodexTurn(slug string, prompt string, channel ...string) {
	ch := ""
	if len(channel) > 0 {
		ch = channel[0]
	}
	slug = strings.TrimSpace(slug)
	prompt = strings.TrimSpace(prompt)
	if slug == "" || prompt == "" {
		return
	}
	prompt, changed := normalizeHeadlessPromptPayload(prompt)
	if changed {
		appendHeadlessCodexLog(slug, "queue-input-sanitize: normalized non-UTF8/sanitized bytes in incoming headless turn prompt")
	}
	l.enqueueHeadlessCodexTurnRecord(slug, headlessCodexTurn{
		Prompt:     prompt,
		Channel:    ch,
		TaskID:     headlessCodexTaskID(prompt),
		EnqueuedAt: time.Now(),
	})
}

func (l *Launcher) enqueueHeadlessCodexTurnRecord(slug string, turn headlessCodexTurn) {
	slug = strings.TrimSpace(slug)
	turn.Prompt = strings.TrimSpace(turn.Prompt)
	turn.Channel = strings.TrimSpace(turn.Channel)
	turn.TaskID = strings.TrimSpace(turn.TaskID)
	turn.ProviderOverride = ""
	if slug == "" || turn.Prompt == "" {
		return
	}
	changed := false
	turn.Prompt, changed = normalizeHeadlessPromptPayload(turn.Prompt)
	if changed {
		appendHeadlessCodexLog(slug, "queue-input-sanitize: normalized non-UTF8 bytes in queued headless prompt")
	}
	if turn.EnqueuedAt.IsZero() {
		turn.EnqueuedAt = time.Now()
	}
	l.ensureHeadlessQueueMaps()
	laneKey := agentLaneKey(turn.Channel, slug)
	if laneKey == "" {
		return
	}
	if l.headlessTurnUsesQuickReplyLane(slug, turn) {
		l.enqueueHeadlessQuickReplyTurnRecord(laneKey, slug, turn)
		return
	}
	if turn.TaskID == "" {
		turn.TaskID = headlessCodexTaskID(turn.Prompt)
	}

	var cancel context.CancelFunc
	var staleAge time.Duration
	startWorker := false

	l.headlessMu.Lock()
	urgentLeadTurn := l.headlessLeadTurnNeedsImmediateWakeLocked(slug, turn.TaskID, turn.Prompt)
	directLeadPing := slug == l.officeLeadSlug() && turn.TaskID == ""
	if turn.TaskID != "" {
		if active := l.headlessActive[laneKey]; active != nil && strings.TrimSpace(active.Turn.TaskID) == turn.TaskID {
			if !(slug == l.officeLeadSlug() && urgentLeadTurn) && turn.Attempts <= active.Turn.Attempts {
				l.headlessMu.Unlock()
				if slug == l.officeLeadSlug() {
					appendHeadlessCodexLog(slug, "queue-drop: lead already handling same task")
				} else {
					appendHeadlessCodexLog(slug, "queue-drop: agent already handling same task")
				}
				return
			}
		}
		if pending := l.replaceDuplicateTaskTurnLocked(laneKey, slug, turn); pending {
			if !l.headlessWorkers[laneKey] {
				l.headlessWorkers[laneKey] = true
				startWorker = true
			}
			l.headlessMu.Unlock()
			if slug == l.officeLeadSlug() {
				appendHeadlessCodexLog(slug, "queue-replace: refreshed pending lead turn for same task")
			} else {
				appendHeadlessCodexLog(slug, "queue-replace: refreshed pending turn for same task")
			}
			if startWorker {
				go l.runHeadlessCodexQueue(laneKey)
			}
			return
		}
	}
	// For the lead (CEO) agent, suppress task handoffs if any other specialist is
	// still active or has pending work. Human pings without a task ID should still
	// reach the lead immediately even while the office is busy.
	if slug == l.officeLeadSlug() && turn.TaskID != "" && !urgentLeadTurn {
		laneChannel := agentLaneChannel(laneKey)
		for workerLaneKey, queue := range l.headlessQueues {
			workerSlug := agentLaneSlug(workerLaneKey)
			if workerSlug == slug || !agentLaneMatchesChannel(workerLaneKey, laneChannel) {
				continue
			}
			if len(queue) > 0 {
				cp := turn
				l.headlessDeferredLead[laneKey] = &cp
				l.headlessMu.Unlock()
				appendHeadlessCodexLog(slug, "queue-hold: specialist still queued, deferring lead notification until all work lands")
				return
			}
		}
		for workerLaneKey, active := range l.headlessActive {
			workerSlug := agentLaneSlug(workerLaneKey)
			if workerSlug == slug || !agentLaneMatchesChannel(workerLaneKey, laneChannel) {
				continue
			}
			if active != nil {
				cp := turn
				l.headlessDeferredLead[laneKey] = &cp
				l.headlessMu.Unlock()
				appendHeadlessCodexLog(slug, "queue-hold: specialist still running, deferring lead notification until all work lands")
				return
			}
		}
	}
	// For the lead (CEO) agent, cap the pending queue at 1 turn.
	// Multiple rapid-fire notifications (agent completions, status pings) can
	// stack up redundant CEO turns that each re-route the same task. One pending
	// turn is enough to catch the latest state; extras are dropped.
	const leadMaxPending = 1
	if slug == l.officeLeadSlug() && len(l.headlessQueues[laneKey]) >= leadMaxPending {
		if directLeadPing {
			l.headlessQueues[laneKey][len(l.headlessQueues[laneKey])-1] = turn
			if !l.headlessWorkers[laneKey] {
				l.headlessWorkers[laneKey] = true
				startWorker = true
			}
			l.headlessMu.Unlock()
			appendHeadlessCodexLog(slug, "queue-replace: lead queue at cap, prioritizing direct human ping")
			if startWorker {
				go l.runHeadlessCodexQueue(laneKey)
			}
			return
		}
		if urgentLeadTurn {
			l.headlessQueues[laneKey][len(l.headlessQueues[laneKey])-1] = turn
			if !l.headlessWorkers[laneKey] {
				l.headlessWorkers[laneKey] = true
				startWorker = true
			}
			l.headlessMu.Unlock()
			appendHeadlessCodexLog(slug, "queue-replace: lead queue at cap, replacing pending turn with urgent task notification")
			if startWorker {
				go l.runHeadlessCodexQueue(laneKey)
			}
			return
		}
		l.headlessMu.Unlock()
		appendHeadlessCodexLog(slug, "queue-drop: lead queue at cap, dropping redundant notification")
		return
	}
	if dropped := l.coalesceQueuedNonTaskTurnsLocked(laneKey, turn); dropped > 0 {
		appendHeadlessCodexLog(slug, fmt.Sprintf("queue-coalesce: dropped %d obsolete queued non-task turn(s) in lane", dropped))
	}
	l.headlessQueues[laneKey] = append(l.headlessQueues[laneKey], turn)
	if !l.headlessWorkers[laneKey] {
		l.headlessWorkers[laneKey] = true
		startWorker = true
	}
	if active := l.headlessActive[laneKey]; active != nil && active.Cancel != nil {
		age := time.Since(active.StartedAt)
		if age >= l.headlessCodexStaleCancelAfterForTurn(active.Turn) {
			cancel = active.Cancel
			staleAge = age
		}
	}
	l.headlessMu.Unlock()

	if cancel != nil {
		appendHeadlessCodexLog(slug, fmt.Sprintf("stale-turn: cancelling active turn after %s to process queued work", staleAge.Round(time.Second)))
		l.updateHeadlessProgress(slug, "active", "queued", "preempting stale work for newer request", headlessProgressMetrics{}, turn.Channel)
		cancel()
	}
	if startWorker {
		go l.runHeadlessCodexQueue(laneKey)
	}
}

func (l *Launcher) ensureHeadlessQueueMaps() {
	if l == nil {
		return
	}
	if l.headlessWorkers == nil {
		l.headlessWorkers = make(map[string]bool)
	}
	if l.headlessActive == nil {
		l.headlessActive = make(map[string]*headlessCodexActiveTurn)
	}
	if l.headlessQueues == nil {
		l.headlessQueues = make(map[string][]headlessCodexTurn)
	}
	if l.headlessQuickWorkers == nil {
		l.headlessQuickWorkers = make(map[string]bool)
	}
	if l.headlessQuickActive == nil {
		l.headlessQuickActive = make(map[string]*headlessCodexActiveTurn)
	}
	if l.headlessQuickQueues == nil {
		l.headlessQuickQueues = make(map[string][]headlessCodexTurn)
	}
	if l.headlessDeferredLead == nil {
		l.headlessDeferredLead = make(map[string]*headlessCodexTurn)
	}
}

func (l *Launcher) headlessTurnUsesQuickReplyLane(slug string, turn headlessCodexTurn) bool {
	if l == nil {
		return false
	}
	return strings.TrimSpace(slug) == l.officeLeadSlug() && turn.QuickReply && strings.TrimSpace(turn.TaskID) == ""
}

func (l *Launcher) headlessTurnProviderKind(slug string, channel ...string) string {
	kind := ""
	if turn := l.currentHeadlessTurn(slug, channel...); turn != nil {
		if rawOverride := strings.TrimSpace(turn.ProviderOverride); rawOverride != "" {
			if override := normalizeProviderKind(rawOverride); override != "" {
				kind = override
			}
		}
	}
	if kind == "" {
		kind = l.memberEffectiveProviderKind(slug)
	}
	if task := l.headlessTaskForExecution(slug, channel...); task != nil {
		if runtimeProvider := strings.TrimSpace(task.RuntimeProvider); runtimeProvider != "" {
			return normalizeProviderKind(runtimeProvider)
		}
		if inferred := inferRuntimeProviderFromModel(task.RuntimeModel); inferred != "" {
			return inferred
		}
		switch strings.ToLower(strings.TrimSpace(task.ExecutionMode)) {
		case "local_worktree", "external_workspace":
			return provider.KindCodex
		}
	}
	return kind
}

func normalizeHeadlessProviderOverride(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return normalizeProviderKind(raw)
}

func (l *Launcher) enqueueHeadlessQuickReplyTurnRecord(laneKey string, slug string, turn headlessCodexTurn) {
	const leadQuickMaxPending = 1
	startWorker := false

	l.headlessMu.Lock()
	queue := l.headlessQuickQueues[laneKey]
	if len(queue) >= leadQuickMaxPending {
		l.headlessQuickQueues[laneKey][len(queue)-1] = turn
	} else {
		l.headlessQuickQueues[laneKey] = append(queue, turn)
	}
	if !l.headlessQuickWorkers[laneKey] {
		l.headlessQuickWorkers[laneKey] = true
		startWorker = true
	}
	l.headlessMu.Unlock()

	if len(queue) >= leadQuickMaxPending {
		appendHeadlessCodexLog(slug, "quick-reply-replace: refreshed pending human ping in dedicated lead lane")
	} else {
		appendHeadlessCodexLog(slug, "quick-reply-enqueue: queued human ping in dedicated lead lane")
	}
	if startWorker {
		go l.runHeadlessQuickReplyQueue(laneKey)
	}
}

func (l *Launcher) replaceDuplicateTaskTurnLocked(laneKey string, slug string, turn headlessCodexTurn) bool {
	for i := range l.headlessQueues[laneKey] {
		if strings.TrimSpace(l.headlessQueues[laneKey][i].TaskID) != turn.TaskID {
			continue
		}
		l.headlessQueues[laneKey][i] = turn
		return true
	}
	if slug == l.officeLeadSlug() && l.headlessDeferredLead[laneKey] != nil && strings.TrimSpace(l.headlessDeferredLead[laneKey].TaskID) == turn.TaskID {
		cp := turn
		l.headlessDeferredLead[laneKey] = &cp
		return true
	}
	return false
}

func (l *Launcher) coalesceQueuedNonTaskTurnsLocked(laneKey string, turn headlessCodexTurn) int {
	if strings.TrimSpace(turn.TaskID) != "" {
		return 0
	}
	queue := l.headlessQueues[laneKey]
	if len(queue) == 0 {
		return 0
	}
	kept := queue[:0]
	dropped := 0
	for _, queued := range queue {
		if strings.TrimSpace(queued.TaskID) == "" {
			dropped++
			continue
		}
		kept = append(kept, queued)
	}
	if dropped > 0 {
		l.headlessQueues[laneKey] = kept
	}
	return dropped
}

func (l *Launcher) headlessLeadTurnNeedsImmediateWakeLocked(slug, taskID, prompt string) bool {
	if l == nil || l.broker == nil {
		return false
	}
	if strings.TrimSpace(slug) != l.officeLeadSlug() {
		return false
	}
	taskID = strings.TrimSpace(firstNonEmpty(taskID, headlessCodexTaskID(prompt)))
	if taskID == "" {
		return false
	}
	if task, ok := l.broker.TaskByID(taskID); ok {
		status := strings.ToLower(strings.TrimSpace(task.Status))
		review := strings.ToLower(strings.TrimSpace(task.ReviewState))
		if status == "review" || review == "ready_for_review" || status == "blocked" {
			return true
		}
		// CEO follow-up tasks are lightweight reply/validation turns. Holding them
		// behind long-running specialist work leaves human follow-ups marked
		// in_progress while the CEO lane stays idle.
		return strings.EqualFold(strings.TrimSpace(task.TaskType), "follow_up") ||
			strings.EqualFold(strings.TrimSpace(task.PipelineID), "follow_up")
	}
	return false
}

func (l *Launcher) runHeadlessCodexQueue(laneKey string) {
	l.runHeadlessQueue(laneKey, false)
}

func (l *Launcher) runHeadlessQuickReplyQueue(laneKey string) {
	l.runHeadlessQueue(laneKey, true)
}

func (l *Launcher) runHeadlessQueue(laneKey string, quick bool) {
	slug := agentLaneSlug(laneKey)
	channel := agentLaneChannel(laneKey)
	for {
		func() {
			site := "runHeadlessCodexQueue"
			if quick {
				site = "runHeadlessQuickReplyQueue"
			}
			defer recoverPanicTo(site, fmt.Sprintf("lane=%s slug=%s quick=%t", laneKey, slug, quick))
			turn, turnCtx, startedAt, timeout, ok := l.beginHeadlessCodexTurn(laneKey, quick)
			if !ok {
				l.updateHeadlessProgress(slug, "idle", "idle", "waiting for work", headlessProgressMetrics{}, channel)
				return
			}
			appendHeadlessCodexLatency(slug, fmt.Sprintf("stage=started queue_wait_ms=%d", time.Since(turn.EnqueuedAt).Milliseconds()))
			l.updateHeadlessProgress(slug, "active", "queued", "queued work packet received", headlessProgressMetrics{}, turn.Channel)
			appendHeadlessCodexLog(slug, fmt.Sprintf("dispatch-provider: provider=%s member=%s override=%s channel=%s task=%s quick=%t",
				l.headlessTurnProviderKind(slug, turn.Channel),
				l.memberEffectiveProviderKind(slug),
				normalizeHeadlessProviderOverride(turn.ProviderOverride),
				normalizeChannelSlug(turn.Channel),
				strings.TrimSpace(turn.TaskID),
				quick,
			))

			err := headlessCodexRunTurn(l, turnCtx, slug, turn.Prompt, turn.Channel)
			ctxErr := turnCtx.Err()
			if err == nil {
				l.headlessMu.Lock()
				active := l.headlessActive[laneKey]
				l.headlessMu.Unlock()
				if ok, reason := l.headlessTurnCompletedDurably(slug, active); !ok {
					appendHeadlessCodexLog(slug, "durability-error: "+reason)
					err = errors.New(reason)
				}
			}
			switch {
			case err == nil:
			case errors.Is(ctxErr, context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded):
				appendHeadlessCodexLog(slug, fmt.Sprintf("error: headless codex turn timed out after %s", timeout))
				l.updateHeadlessProgress(slug, "error", "error", fmt.Sprintf("turn timed out after %s", timeout), headlessProgressMetrics{}, turn.Channel)
				l.recoverTimedOutHeadlessTurn(slug, turn, startedAt, timeout)
			case errors.Is(ctxErr, context.Canceled) || errors.Is(err, context.Canceled):
				appendHeadlessCodexLog(slug, "error: headless codex turn cancelled so newer queued work can run")
				l.updateHeadlessProgress(slug, "active", "queued", "restarting on newer queued work", headlessProgressMetrics{}, turn.Channel)
			default:
				appendHeadlessCodexLog(slug, fmt.Sprintf("error: %v", err))
				l.updateHeadlessProgress(slug, "error", "error", truncate(err.Error(), 180), headlessProgressMetrics{}, turn.Channel)
				l.recoverFailedHeadlessTurn(slug, turn, startedAt, err.Error())
			}
			l.finishHeadlessTurn(laneKey, quick)
		}()
		l.headlessMu.Lock()
		workers, _, _ := l.headlessLaneMaps(quick)
		_, stillRunning := workers[laneKey]
		l.headlessMu.Unlock()
		if !stillRunning {
			return
		}
	}
}

func (l *Launcher) headlessLaneMaps(quick bool) (map[string]bool, map[string]*headlessCodexActiveTurn, map[string][]headlessCodexTurn) {
	if quick {
		return l.headlessQuickWorkers, l.headlessQuickActive, l.headlessQuickQueues
	}
	return l.headlessWorkers, l.headlessActive, l.headlessQueues
}

func taskHasDurableCompletionState(task *teamTask) bool {
	if task == nil {
		return false
	}
	if taskHasPendingReconciliation(task) {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(task.Status))
	review := strings.ToLower(strings.TrimSpace(task.ReviewState))
	switch status {
	case "done", "completed", "blocked", "cancelled", "canceled", "review":
		return true
	}
	switch review {
	case "ready_for_review", "approved":
		return true
	}
	return false
}

func (l *Launcher) headlessTurnCompletedDurably(slug string, active *headlessCodexActiveTurn) (bool, string) {
	if l == nil || l.broker == nil || active == nil {
		return true, ""
	}
	task := l.timedOutTaskForTurn(slug, active.Turn)
	requiresDurableGuard := codingAgentSlugs[slug]
	requiresExternalExecution := taskRequiresRealExternalExecution(task)
	if task != nil && strings.EqualFold(strings.TrimSpace(task.ExecutionMode), "local_worktree") {
		requiresDurableGuard = true
	}
	if requiresExternalExecution {
		requiresDurableGuard = true
	}
	if !requiresDurableGuard {
		if task != nil {
			liveness := l.classifyHeadlessOfficeTurnLiveness(slug, active, task)
			l.recordHeadlessLiveness(slug, active, liveness)
			appendHeadlessCodexLog(slug, fmt.Sprintf("liveness: state=%s reason=%s", liveness.State, liveness.Reason))
			switch liveness.State {
			case runLivenessEmptyResponse, runLivenessPlanOnly, runLivenessBlocked:
				if liveness.State == runLivenessBlocked && taskHasDurableCompletionState(task) {
					return true, ""
				}
				return false, liveness.Reason
			}
		}
		return true, ""
	}
	if task != nil && requiresExternalExecution {
		executed, attempted := l.taskHasExternalWorkflowEvidenceSince(task, active.StartedAt)
		if taskHasDurableCompletionState(task) {
			status := strings.ToLower(strings.TrimSpace(task.Status))
			switch status {
			case "done", "completed", "review":
				if executed {
					return true, ""
				}
				return false, fmt.Sprintf("external-action turn for #%s marked %s/%s without recorded external execution evidence", task.ID, strings.TrimSpace(task.Status), strings.TrimSpace(task.ReviewState))
			case "blocked", "cancelled", "canceled":
				if attempted {
					return true, ""
				}
				return false, fmt.Sprintf("external-action turn for #%s moved to %s without recorded external workflow evidence", task.ID, strings.TrimSpace(task.Status))
			default:
				if executed {
					return true, ""
				}
			}
		}
		if executed {
			return true, ""
		}
	}
	if task != nil && taskHasDurableCompletionState(task) {
		return true, ""
	}
	if l.agentPostedSubstantiveMessageSince(slug, active.StartedAt) {
		return true, ""
	}
	if workspaceDir := strings.TrimSpace(active.WorkspaceDir); workspaceDir != "" {
		current := headlessCodexWorkspaceStatusSnapshot(workspaceDir)
		if strings.TrimSpace(active.WorkspaceSnapshot) != "" && current != active.WorkspaceSnapshot {
			if task != nil {
				return false, fmt.Sprintf("coding turn for #%s changed workspace %s but left task %s/%s without durable completion evidence", task.ID, workspaceDir, strings.TrimSpace(task.Status), strings.TrimSpace(task.ReviewState))
			}
			return false, fmt.Sprintf("coding turn changed workspace %s without durable completion evidence", workspaceDir)
		}
	}
	if task != nil {
		if requiresExternalExecution {
			return false, fmt.Sprintf("external-action turn for #%s completed without durable task state or external workflow evidence", task.ID)
		}
		return false, fmt.Sprintf("coding turn for #%s completed without durable task state or completion evidence", task.ID)
	}
	if requiresExternalExecution {
		return false, fmt.Sprintf("external-action turn by @%s completed without durable task state or external workflow evidence", slug)
	}
	return false, fmt.Sprintf("coding turn by @%s completed without durable task state or completion evidence", slug)
}

func (l *Launcher) taskHasExternalWorkflowEvidenceSince(task *teamTask, startedAt time.Time) (executed bool, attempted bool) {
	if l == nil || l.broker == nil || task == nil {
		return false, false
	}
	channel := normalizeChannelSlug(task.Channel)
	owner := strings.TrimSpace(task.Owner)
	for _, action := range l.broker.Actions() {
		kind := strings.ToLower(strings.TrimSpace(action.Kind))
		switch kind {
		case "external_workflow_executed",
			"external_workflow_failed",
			"external_workflow_rate_limited",
			"external_action_executed",
			"external_action_failed":
		default:
			continue
		}
		if channel != "" && normalizeChannelSlug(action.Channel) != channel {
			continue
		}
		if owner != "" {
			actor := strings.TrimSpace(action.Actor)
			if actor != "" && actor != owner && actor != "scheduler" {
				continue
			}
		}
		when, err := time.Parse(time.RFC3339, strings.TrimSpace(action.CreatedAt))
		if err != nil {
			when, err = time.Parse(time.RFC3339Nano, strings.TrimSpace(action.CreatedAt))
		}
		if err == nil && !when.Add(time.Second).After(startedAt) {
			continue
		}
		attempted = true
		if kind == "external_workflow_executed" || kind == "external_action_executed" {
			executed = true
		}
	}
	return executed, attempted
}

func (l *Launcher) finishHeadlessTurn(laneKey string, quick bool) {
	slug := agentLaneSlug(laneKey)
	channel := agentLaneChannel(laneKey)
	l.headlessMu.Lock()
	_, activeMap, _ := l.headlessLaneMaps(quick)
	if active := activeMap[laneKey]; active != nil && active.Cancel != nil {
		active.Cancel()
	}
	delete(activeMap, laneKey)
	if quick {
		l.headlessMu.Unlock()
		return
	}
	lead := l.officeLeadSlug()
	leadLaneKey := agentLaneKey(channel, lead)
	var deferredLead *headlessCodexTurn
	// Determine if this was a specialist finishing (not the lead), and if so whether
	// any other specialists are still active or queued. If the slate is clear, we
	// need to wake the lead so it can react to the specialist's completion messages.
	// Without this, the CEO misses completion broadcasts because the queue-hold
	// fires while the specialist is still "active" (process running), and after the
	// process exits there is nothing else to re-trigger the CEO.
	shouldWakeLead := slug != lead && lead != ""
	if shouldWakeLead {
		for workerLaneKey, queue := range l.headlessQueues {
			workerSlug := agentLaneSlug(workerLaneKey)
			if workerSlug == lead || !agentLaneMatchesChannel(workerLaneKey, channel) {
				continue
			}
			if len(queue) > 0 {
				shouldWakeLead = false
				break
			}
		}
	}
	if shouldWakeLead {
		for workerLaneKey, active := range l.headlessActive {
			workerSlug := agentLaneSlug(workerLaneKey)
			if workerSlug == lead || !agentLaneMatchesChannel(workerLaneKey, channel) {
				continue
			}
			if active != nil {
				shouldWakeLead = false
				break
			}
		}
	}
	// Check if the lead already has work queued — no need to wake it.
	if shouldWakeLead && len(l.headlessQueues[leadLaneKey]) > 0 {
		shouldWakeLead = false
	}
	if shouldWakeLead && l.headlessDeferredLead[leadLaneKey] != nil {
		turn := *l.headlessDeferredLead[leadLaneKey]
		delete(l.headlessDeferredLead, leadLaneKey)
		deferredLead = &turn
		shouldWakeLead = false
	}
	l.headlessMu.Unlock()

	if deferredLead != nil {
		l.enqueueHeadlessCodexTurn(lead, deferredLead.Prompt, deferredLead.Channel)
		return
	}
	if shouldWakeLead {
		if headlessWakeLeadFn != nil {
			headlessWakeLeadFn(l, slug)
		} else {
			l.wakeLeadAfterSpecialist(slug, channel)
		}
	}
}

// wakeLeadAfterSpecialist re-queues the lead (CEO) with the most recent message
// posted by the finishing specialist. This is needed because the lead's queue-hold
// suppresses notifications while a specialist is running, so the lead never sees
// the completion broadcast. We only do this when no other specialists remain active.
func (l *Launcher) wakeLeadAfterSpecialist(specialistSlug string, channel ...string) {
	if l.broker == nil {
		return
	}
	laneChannel := normalizeChannelSlug(firstNonEmpty(channel...))
	lead := l.officeLeadSlug()
	if lead == "" {
		return
	}
	targets := l.agentPaneTargets()
	target, ok := targets[lead]
	if !ok {
		return
	}
	// Find the most recent substantive message from the specialist across all
	// channels. A specialist may complete work on a non-general channel (e.g.
	// "engineering" or "marketing"), so scanning only "general" would miss those
	// completions and the lead would never react.
	msgs := l.broker.AllMessages()
	var lastMsg *channelMessage
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.From != specialistSlug || (laneChannel != "" && normalizeChannelSlug(m.Channel) != laneChannel) {
			continue
		}
		if isDM, _ := l.isChannelDM(normalizeChannelSlug(m.Channel)); isDM {
			continue
		}
		content := strings.TrimSpace(m.Content)
		if !messageIsSubstantiveOfficeContent(content) {
			continue
		}
		if messageLooksLikeBlockedNoDeltaUpdate(content) {
			signature := normalizeBlockedNoDeltaSignature(content)
			repeated := false
			for j := i - 1; j >= 0; j-- {
				prior := msgs[j]
				if prior.From != specialistSlug || normalizeChannelSlug(prior.Channel) != normalizeChannelSlug(m.Channel) {
					continue
				}
				if strings.TrimSpace(prior.ReplyTo) != strings.TrimSpace(m.ReplyTo) {
					continue
				}
				if !messageLooksLikeBlockedNoDeltaUpdate(prior.Content) {
					continue
				}
				if normalizeBlockedNoDeltaSignature(prior.Content) == signature {
					repeated = true
				}
				break
			}
			if repeated {
				continue
			}
		}
		lastMsg = &msgs[i]
		break
	}
	if lastMsg == nil {
		if action, task, ok := l.latestLeadWakeTaskAction(specialistSlug, laneChannel); ok {
			if isDM, _ := l.isChannelDM(normalizeChannelSlug(task.Channel)); isDM {
				return
			}
			content := l.taskNotificationContent(action, task)
			appendHeadlessCodexLog(lead, fmt.Sprintf("wake-lead: re-delivering task handoff from @%s (%s)", specialistSlug, task.ID))
			l.sendTaskUpdate(target, action, task, content)
		}
		return
	}
	appendHeadlessCodexLog(lead, fmt.Sprintf("wake-lead: re-delivering specialist completion from @%s (msg %s)", specialistSlug, lastMsg.ID))
	l.sendChannelUpdate(target, *lastMsg)
}

func (l *Launcher) latestLeadWakeTaskAction(specialistSlug string, channel string) (officeActionLog, teamTask, bool) {
	if l == nil || l.broker == nil {
		return officeActionLog{}, teamTask{}, false
	}
	actions := l.broker.Actions()
	for i := len(actions) - 1; i >= 0; i-- {
		action := actions[i]
		if strings.TrimSpace(action.Actor) != specialistSlug {
			continue
		}
		if action.Kind != "task_updated" && action.Kind != "task_unblocked" {
			continue
		}
		task, ok := l.taskForAction(action)
		if !ok {
			continue
		}
		if normalizeChannelSlug(task.Channel) != normalizeChannelSlug(channel) {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(task.Status)) {
		case "done", "review", "blocked":
			return action, task, true
		}
	}
	return officeActionLog{}, teamTask{}, false
}

func headlessCodexTaskID(prompt string) string {
	prefixes := []string{"#task-", "task-", "#blank-slate-", "blank-slate-"}
	for _, prefix := range prefixes {
		idx := strings.Index(prompt, prefix)
		if idx == -1 {
			continue
		}
		if idx > 0 {
			prev := prompt[idx-1]
			if (prev >= 'a' && prev <= 'z') || (prev >= 'A' && prev <= 'Z') || (prev >= '0' && prev <= '9') || prev == '-' || prev == '_' {
				continue
			}
		}
		start := idx
		if prefix[0] == '#' {
			start++
		}
		end := start
		for end < len(prompt) {
			ch := prompt[end]
			if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
				end++
				continue
			}
			break
		}
		return strings.TrimSpace(prompt[start:end])
	}
	return ""
}

func (l *Launcher) agentPostedSubstantiveMessageSince(slug string, startedAt time.Time) bool {
	if l == nil || l.broker == nil {
		return false
	}
	for _, msg := range l.broker.AllMessages() {
		if msg.From != slug {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if !messageIsSubstantiveOfficeContent(content) {
			continue
		}
		when, err := time.Parse(time.RFC3339, msg.Timestamp)
		if err != nil {
			continue
		}
		if when.Add(time.Second).After(startedAt) {
			return true
		}
	}
	return false
}

func (l *Launcher) timedOutTaskForTurn(slug string, turn headlessCodexTurn) *teamTask {
	if l == nil || l.broker == nil {
		return nil
	}
	if id := strings.TrimSpace(turn.TaskID); id != "" {
		if task, ok := l.broker.TaskByID(id); ok {
			cp := task
			return &cp
		}
	}
	if channel := normalizeChannelSlug(turn.Channel); channel != "" {
		return l.agentActiveTask(slug, channel)
	}
	return l.agentActiveTask(slug)
}

func (l *Launcher) shouldRetryTimedOutHeadlessTurn(task *teamTask, turn headlessCodexTurn) bool {
	if task == nil {
		return false
	}
	return turn.Attempts < retryLimitForHeadlessTurn(task)
}

func isHeadlessCodeWorkspaceExecution(task *teamTask) bool {
	if task == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(task.ExecutionMode)) {
	case "local_worktree", "external_workspace":
		return true
	default:
		return false
	}
}

func retryLimitForHeadlessTurn(task *teamTask) int {
	if task == nil || taskHasDurableCompletionState(task) {
		return 0
	}
	if isHeadlessCodeWorkspaceExecution(task) {
		return headlessCodexLocalWorktreeRetryLimit
	}
	if taskRequiresRealExternalExecution(task) {
		return headlessCodexExternalActionRetryLimit
	}
	if strings.EqualFold(strings.TrimSpace(task.ExecutionMode), "office") && strings.TrimSpace(task.Owner) != "" {
		return headlessCodexOfficeRetryLimit
	}
	return 0
}

func headlessTimedOutRetryPrompt(slug string, prompt string, timeout time.Duration, attempt int, external bool) string {
	note := fmt.Sprintf("Previous attempt by @%s timed out after %s without a durable task handoff. Retry #%d.", strings.TrimSpace(slug), timeout, attempt)
	if external {
		note += " This is a live external-action task. Do the smallest useful live external step now. If Slack target discovery is already known, use it. If the first live Slack target fails, retry once against the resolved writable target; if that still fails, pivot immediately to the smallest useful live Notion or Drive action and report the exact blocker. Do not write repo docs or planning artifacts as substitutes."
	} else {
		note += " For this retry, move immediately from claim/status into targeted file reads and edits, then leave the task in review/done/blocked before you stop. If you cannot ship the whole slice, ship the smallest runnable sub-slice and mark that state explicitly."
	}
	if strings.TrimSpace(prompt) == "" {
		return note
	}
	return strings.TrimSpace(prompt) + "\n\n" + note
}

func headlessFailedRetryPrompt(slug string, prompt string, detail string, attempt int, external bool) string {
	note := fmt.Sprintf("Previous attempt by @%s failed before a durable task handoff. Retry #%d.", strings.TrimSpace(slug), attempt)
	if trimmed := strings.TrimSpace(detail); trimmed != "" {
		note += " Last error: " + truncate(trimmed, 180) + "."
	}
	if external {
		note += " This is a live external-action task. Do the smallest useful live external step now. Do not keep discovering or drafting repo substitutes. If the first live Slack target fails, retry once against the resolved writable target; if that still fails, pivot immediately to the smallest useful live Notion or Drive action and report the exact blocker."
	} else {
		note += " For this retry, move immediately from claim/status into targeted file reads and edits, then leave the task in review/done/blocked before you stop. If you cannot ship the whole slice, ship the smallest runnable sub-slice and mark that state explicitly."
	}
	if strings.TrimSpace(prompt) == "" {
		return note
	}
	return strings.TrimSpace(prompt) + "\n\n" + note
}

func isHeadlessRuntimeProviderFailure(detail string) bool {
	normalized := strings.ToLower(strings.TrimSpace(detail))
	if normalized == "" {
		return false
	}
	switch {
	case strings.Contains(normalized, "selected model is at capacity"),
		strings.Contains(normalized, "model is at capacity"),
		strings.Contains(normalized, "hit your limit"),
		strings.Contains(normalized, "rate limit"),
		strings.Contains(normalized, "too many requests"),
		strings.Contains(normalized, "quota exceeded"),
		strings.Contains(normalized, "login required"),
		strings.Contains(normalized, "requires login"),
		strings.Contains(normalized, "not logged in"),
		strings.Contains(normalized, "authentication required"),
		strings.Contains(normalized, "unauthorized"),
		strings.Contains(normalized, "unsupported headless provider"):
		return true
	}
	hasRuntimeMarker := strings.Contains(normalized, "codex") ||
		strings.Contains(normalized, "claude") ||
		strings.Contains(normalized, "openclaude") ||
		strings.Contains(normalized, "gemini") ||
		strings.Contains(normalized, "ollama") ||
		strings.Contains(normalized, "provider")
	if !hasRuntimeMarker {
		return false
	}
	return strings.Contains(normalized, "not found") ||
		strings.Contains(normalized, "executable file not found") ||
		strings.Contains(normalized, "failed to start")
}

func shouldRetryHeadlessTurn(task *teamTask, turn headlessCodexTurn) bool {
	return turn.Attempts < retryLimitForHeadlessTurn(task)
}

func normalizeHeadlessPromptPayload(payload string) (string, bool) {
	normalized := payload
	changed := false
	if !utf8.ValidString(normalized) {
		normalized = strings.ToValidUTF8(normalized, headlessPromptNormalizationReplacement)
		changed = true
	}
	if strings.Contains(normalized, "\x00") {
		trimmed := strings.ReplaceAll(normalized, "\x00", "")
		if trimmed != normalized {
			normalized = trimmed
			changed = true
		}
	}
	return normalized, changed
}

func isHeadlessPromptEncodingFailure(detail string) bool {
	detail = strings.ToLower(strings.TrimSpace(detail))
	if detail == "" {
		return false
	}
	return strings.Contains(detail, "failed to read prompt from stdin") ||
		strings.Contains(detail, "input is not valid utf-8")
}

func (l *Launcher) recoverTimedOutHeadlessTurn(slug string, turn headlessCodexTurn, startedAt time.Time, timeout time.Duration) {
	if l == nil || l.broker == nil {
		return
	}
	task := l.timedOutTaskForTurn(slug, turn)
	if task == nil || strings.TrimSpace(task.ID) == "" {
		appendHeadlessCodexLog(slug, "timeout-recovery: no matching task found to block")
		return
	}
	if l.timedOutTurnAlreadyRecovered(task, slug, startedAt) {
		appendHeadlessCodexLog(slug, fmt.Sprintf("timeout-recovery: %s already produced durable progress; leaving task state unchanged", task.ID))
		return
	}
	if shouldRetryHeadlessTurn(task, turn) {
		retryTurn := turn
		retryTurn.Attempts++
		retryTurn.EnqueuedAt = time.Now()
		retryTurn.Prompt = headlessTimedOutRetryPrompt(slug, turn.Prompt, timeout, retryTurn.Attempts, taskRequiresRealExternalExecution(task))
		limit := retryLimitForHeadlessTurn(task)
		l.broker.mu.Lock()
		l.broker.markExecutionNodeFallbackLocked(task, slug, fmt.Sprintf("timed out after %s before a durable reply", timeout), retryTurn.Attempts)
		l.broker.mu.Unlock()
		appendHeadlessCodexLog(slug, fmt.Sprintf("timeout-recovery: requeueing %s after silent timeout (attempt %d/%d)", task.ID, retryTurn.Attempts, limit))
		l.enqueueHeadlessCodexTurnRecord(slug, retryTurn)
		return
	}
	reason := fmt.Sprintf("Automatic timeout recovery: @%s timed out after %s before posting a substantive update. Requeue, retry, or reassign from here.", slug, timeout)
	l.broker.mu.Lock()
	l.broker.markExecutionNodeTimedOutLocked(task, slug, reason)
	l.broker.mu.Unlock()
	if _, changed, err := l.broker.BlockTask(task.ID, slug, reason); err != nil {
		appendHeadlessCodexLog(slug, fmt.Sprintf("timeout-recovery-error: could not block %s: %v", task.ID, err))
		return
	} else if changed {
		appendHeadlessCodexLog(slug, fmt.Sprintf("timeout-recovery: blocked %s after empty timeout", task.ID))
	}
	l.markTaskReconciliationAfterHeadlessFailure(task, reason)
}

func (l *Launcher) recoverFailedHeadlessTurn(slug string, turn headlessCodexTurn, startedAt time.Time, detail string) {
	if l == nil || l.broker == nil {
		return
	}
	task := l.timedOutTaskForTurn(slug, turn)
	if task == nil || strings.TrimSpace(task.ID) == "" {
		appendHeadlessCodexLog(slug, "error-recovery: no matching task found to recover")
		return
	}
	if l.timedOutTurnAlreadyRecovered(task, slug, startedAt) {
		appendHeadlessCodexLog(slug, fmt.Sprintf("error-recovery: %s already produced durable progress; leaving task state unchanged", task.ID))
		return
	}
	trimmed := strings.TrimSpace(detail)
	if trimmed == "" {
		trimmed = "unknown headless codex failure"
	}
	encodingFailure := isHeadlessPromptEncodingFailure(trimmed)
	shouldRetry := shouldRetryHeadlessTurn(task, turn)
	retryLimit := retryLimitForHeadlessTurn(task)
	if encodingFailure {
		shouldRetry = turn.Attempts < headlessPromptEncodingRetryLimit
		retryLimit = headlessPromptEncodingRetryLimit
	}
	if shouldRetry {
		retryTurn := turn
		retryTurn.Attempts++
		retryTurn.EnqueuedAt = time.Now()
		basePrompt, _ := normalizeHeadlessPromptPayload(turn.Prompt)
		retryTurn.Prompt = headlessFailedRetryPrompt(slug, basePrompt, detail, retryTurn.Attempts, taskRequiresRealExternalExecution(task))
		l.broker.mu.Lock()
		l.broker.markExecutionNodeFallbackLocked(task, slug, trimmed, retryTurn.Attempts)
		l.broker.mu.Unlock()
		appendHeadlessCodexLog(slug, fmt.Sprintf("error-recovery: requeueing %s after failed turn (attempt %d/%d)", task.ID, retryTurn.Attempts, retryLimit))
		l.enqueueHeadlessCodexTurnRecord(slug, retryTurn)
		return
	}
	reason := fmt.Sprintf("Automatic error recovery: @%s failed before a durable task handoff. Last error: %s. Requeue, retry, or reassign from here.", slug, truncate(trimmed, 220))
	l.broker.mu.Lock()
	l.broker.markExecutionNodeTimedOutLocked(task, slug, trimmed)
	l.broker.mu.Unlock()
	if _, changed, err := l.broker.BlockTask(task.ID, slug, reason); err != nil {
		appendHeadlessCodexLog(slug, fmt.Sprintf("error-recovery-error: could not block %s: %v", task.ID, err))
		return
	} else if changed {
		appendHeadlessCodexLog(slug, fmt.Sprintf("error-recovery: blocked %s after failed turn", task.ID))
	}
	l.markTaskReconciliationAfterHeadlessFailure(task, reason)
}

func (l *Launcher) markTaskReconciliationAfterHeadlessFailure(task *teamTask, reason string) {
	if l == nil || l.broker == nil || task == nil {
		return
	}
	if !(isHeadlessCodeWorkspaceExecution(task) || taskRequiresRealExternalExecution(task)) {
		return
	}
	workspacePath := effectiveTeamTaskWorkspacePath(task)
	observedDelta := ""
	if strings.TrimSpace(workspacePath) != "" {
		observedDelta = headlessCodexWorkspaceStatusSnapshot(workspacePath)
	}
	if _, _, err := l.broker.MarkTaskReconciliationPending(task.ID, workspacePath, observedDelta, reason); err != nil {
		appendHeadlessCodexLog(strings.TrimSpace(task.Owner), fmt.Sprintf("reconciliation-pending-error: could not mark %s: %v", task.ID, err))
	}
}

func (l *Launcher) timedOutTurnAlreadyRecovered(task *teamTask, slug string, startedAt time.Time) bool {
	if task == nil {
		return false
	}
	if taskHasDurableCompletionState(task) {
		return true
	}
	if l.agentRecordedTaskProgressSince(task.ID, slug, startedAt) {
		return true
	}
	if isHeadlessCodeWorkspaceExecution(task) || taskRequiresRealExternalExecution(task) {
		return false
	}
	return l.agentPostedSubstantiveMessageSince(slug, startedAt)
}

func (l *Launcher) agentRecordedTaskProgressSince(taskID, slug string, startedAt time.Time) bool {
	if l == nil || l.broker == nil {
		return false
	}
	taskID = strings.TrimSpace(taskID)
	slug = strings.TrimSpace(slug)
	if taskID == "" || slug == "" {
		return false
	}
	for _, action := range l.broker.Actions() {
		if strings.TrimSpace(action.Actor) != slug || strings.TrimSpace(action.RelatedID) != taskID {
			continue
		}
		switch strings.TrimSpace(action.Kind) {
		case "task_created", "task_updated", "task_unblocked":
		default:
			continue
		}
		when, err := time.Parse(time.RFC3339, strings.TrimSpace(action.CreatedAt))
		if err != nil {
			continue
		}
		if when.Add(time.Second).After(startedAt) {
			return true
		}
	}
	return false
}

func (l *Launcher) beginHeadlessCodexTurn(laneKey string, quick bool) (headlessCodexTurn, context.Context, time.Time, time.Duration, bool) {
	l.headlessMu.Lock()
	defer l.headlessMu.Unlock()

	workers, activeMap, queueMap := l.headlessLaneMaps(quick)
	queue := queueMap[laneKey]
	if len(queue) == 0 {
		// Atomically mark the worker as done. This must happen while the lock is
		// held so that any concurrent enqueueHeadlessCodexTurn will observe
		// headlessWorkers[slug] = false and start a new goroutine rather than
		// assuming the current one will pick up the new item.
		delete(workers, laneKey)
		delete(queueMap, laneKey)
		return headlessCodexTurn{}, nil, time.Time{}, 0, false
	}

	turn := queue[0]
	slug := agentLaneSlug(laneKey)
	channel := firstNonEmpty(strings.TrimSpace(turn.Channel), agentLaneChannel(laneKey))
	if strings.TrimSpace(turn.ProviderOverride) != "" {
		appendHeadlessCodexLog(slug, "provider-override-cleared: ignoring stale queued provider override")
		turn.ProviderOverride = ""
	}
	changed := false
	turn.Prompt, changed = normalizeHeadlessPromptPayload(turn.Prompt)
	if changed {
		appendHeadlessCodexLog(slug, "queue-dequeue-sanitize: normalized non-UTF8 bytes in turn prompt before processing")
	}
	if len(queue) == 1 {
		delete(queueMap, laneKey)
	} else {
		queueMap[laneKey] = queue[1:]
	}

	baseCtx := l.headlessCtx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	timeout := l.headlessCodexTurnTimeoutForTurn(turn)
	turnCtx, cancel := context.WithTimeout(baseCtx, timeout)
	startedAt := time.Now()
	workspaceDir := ""
	if worktreeDir := l.headlessTaskWorkspaceDirForTurn(slug, &turn, channel); worktreeDir != "" {
		workspaceDir = worktreeDir
	} else if codingAgentSlugs[slug] {
		workspaceDir = normalizeHeadlessWorkspaceDir(l.cwd)
	}
	activeMap[laneKey] = &headlessCodexActiveTurn{
		Turn:              turn,
		StartedAt:         startedAt,
		Timeout:           timeout,
		Cancel:            cancel,
		WorkspaceDir:      workspaceDir,
		WorkspaceSnapshot: headlessCodexWorkspaceStatusSnapshot(workspaceDir),
	}
	return turn, turnCtx, startedAt, timeout, true
}

func (l *Launcher) headlessCodexTurnTimeoutForTurn(turn headlessCodexTurn) time.Duration {
	if task := l.timedOutTaskForTurn("", turn); task != nil {
		if isHeadlessCodeWorkspaceExecution(task) {
			return headlessCodexLocalWorktreeTurnTimeout
		}
		if strings.EqualFold(strings.TrimSpace(task.ExecutionMode), "office") &&
			strings.EqualFold(strings.TrimSpace(task.TaskType), "launch") {
			return headlessCodexOfficeLaunchTurnTimeout
		}
	}
	return headlessCodexTurnTimeout
}

func (l *Launcher) headlessCodexStaleCancelAfterForTurn(turn headlessCodexTurn) time.Duration {
	if task := l.timedOutTaskForTurn("", turn); task != nil {
		if isHeadlessCodeWorkspaceExecution(task) {
			return l.headlessCodexTurnTimeoutForTurn(turn)
		}
		if strings.EqualFold(strings.TrimSpace(task.ExecutionMode), "office") &&
			strings.EqualFold(strings.TrimSpace(task.TaskType), "launch") {
			return l.headlessCodexTurnTimeoutForTurn(turn)
		}
	}
	return headlessCodexStaleCancelAfter
}

func (l *Launcher) runHeadlessCodexTurn(ctx context.Context, slug string, notification string, channel ...string) error {
	_, binaryName, err := provider.ResolveCodexCLIPath(headlessCodexLookPath)
	if err != nil {
		return fmt.Errorf("codex not found: %w", err)
	}
	notification, changed := normalizeHeadlessPromptPayload(notification)
	if changed {
		appendHeadlessCodexLog(slug, "turn-input-sanitize: normalized non-UTF8 bytes in turn notification before codex stdin dispatch")
	}
	if l == nil || l.broker == nil {
		return fmt.Errorf("broker is not running")
	}

	turnChannel := l.headlessTurnChannel(slug, channel...)
	workspaceDir := l.headlessWorkspaceDirForExecution(slug, turnChannel)
	workspaceDir = normalizeHeadlessWorkspaceDir(workspaceDir)
	if workspaceDir == "" {
		workspaceDir = "."
	}
	route := l.resolveHeadlessModelRoute(provider.KindCodex, slug, notification, turnChannel)
	appendHeadlessCodexLog(slug, "model-routing: "+route.summary())

	overrides, err := l.buildCodexOfficeConfigOverrides(slug)
	if err != nil {
		return err
	}
	if reason := headlessCodexWebsocketHeaderWorkaroundReason(workspaceDir); reason != "" {
		// Codex websocket streaming serializes turn metadata into headers. On
		// Windows, any non-ASCII workspace metadata path causes the header
		// conversion to fail before the turn can stream. Force the local
		// codex-lb provider onto plain HTTP for this turn.
		overrides = append(overrides, `model_providers.codex-lb.supports_websockets=false`)
		appendHeadlessCodexLog(slug, "workspace-header-workaround: "+reason)
	}

	args := make([]string, 0, 16+len(overrides)*2)
	// Nested Codex local-worktree turns need full bypass here. The child Codex
	// sandbox rejects both apply_patch and shell writes even with
	// workspace-write, which leaves coding tasks permanently unable to land
	// edits. External workspace turns have the same need because they are also
	// real coding lanes pointed at an assigned writable repo. Keep office/non-editing
	// turns on workspace-write.
	if l.unsafe || l.headlessCodexNeedsDangerousBypass(slug, workspaceDir, turnChannel) {
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	} else {
		args = append(args, "-a", "never", "-s", "workspace-write")
	}
	args = append(args, "--disable", "plugins")
	args = append(args,
		"exec",
		"-C", workspaceDir,
		"--skip-git-repo-check",
		"--ephemeral",
		"--color", "never",
		"--json",
	)
	if model := strings.TrimSpace(route.Model); model != "" {
		args = append(args, "--model", model)
	}
	if effort := strings.TrimSpace(l.headlessCodexReasoningEffort(slug, turnChannel)); effort != "" {
		args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%s", tomlQuote(effort)))
	}
	for _, override := range overrides {
		args = append(args, "-c", override)
	}
	args = append(args, "-")

	cmd := headlessCodexCommandContext(ctx, binaryName, args...)
	cmd.Dir = workspaceDir
	cmd.Env = l.buildHeadlessCodexEnv(slug, workspaceDir, turnChannel)
	if task := l.headlessTaskForExecution(slug, turnChannel); task != nil && workspaceDir != strings.TrimSpace(l.cwd) {
		switch strings.ToLower(strings.TrimSpace(task.ExecutionMode)) {
		case "external_workspace":
			cmd.Env = append(cmd.Env, "WUPHF_WORKSPACE_PATH="+workspaceDir)
		case "local_worktree":
			cmd.Env = append(cmd.Env, "WUPHF_WORKTREE_PATH="+workspaceDir)
		}
	} else if workspaceDir != strings.TrimSpace(l.cwd) {
		cmd.Env = append(cmd.Env, "WUPHF_WORKSPACE_PATH="+workspaceDir)
	}
	stdinPayload := buildHeadlessCodexPrompt(l.buildPrompt(slug), notification)
	stdinPayload, changed = normalizeHeadlessPromptPayload(stdinPayload)
	if changed {
		appendHeadlessCodexLog(slug, "stdin-sanitize: normalized non-UTF8 bytes in codex stdin payload")
	}
	cmd.Stdin = strings.NewReader(stdinPayload)
	configureHeadlessProcess(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("attach codex stdout: %w", err)
	}

	// Tee raw stdout to the agent stream so the web UI can display live output.
	// The ReadCodexJSONStream parser doesn't emit streaming events for exec mode's
	// item.started/item.completed format, so we pipe raw lines directly.
	var agentStream *agentStreamBuffer
	if l.broker != nil {
		agentStream = l.broker.AgentStream(slug, turnChannel)
	}
	pr, pw := io.Pipe()
	teedStdout := io.TeeReader(stdout, pw)
	// Pipe every raw line from the provider (codex/claude) to the web UI's live stream.
	// No filtering — the user sees everything the agent sees.
	go func() {
		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if agentStream != nil && line != "" {
				agentStream.Push(line)
			}
		}
	}()

	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		return err
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			terminateHeadlessProcess(cmd)
			_ = stdout.Close()
			_ = pw.CloseWithError(ctx.Err())
		case <-done:
		}
	}()

	startedAt := time.Now()
	metrics := headlessProgressMetrics{
		TotalMs:      -1,
		FirstEventMs: -1,
		FirstTextMs:  -1,
		FirstToolMs:  -1,
	}
	l.updateHeadlessProgress(slug, "active", "thinking", "reviewing work packet · "+route.progressDetail(), metrics, turnChannel)
	var firstEventAt time.Time
	var firstTextAt time.Time
	var firstToolAt time.Time
	textStarted := false
	result, parseErr := provider.ReadCodexJSONStream(teedStdout, func(event provider.CodexStreamEvent) {
		if firstEventAt.IsZero() {
			firstEventAt = time.Now()
			metrics.FirstEventMs = durationMillis(startedAt, firstEventAt)
		}
		switch event.Type {
		case "text":
			if firstTextAt.IsZero() && strings.TrimSpace(event.Text) != "" {
				firstTextAt = time.Now()
				metrics.FirstTextMs = durationMillis(startedAt, firstTextAt)
			}
			if !textStarted && strings.TrimSpace(event.Text) != "" {
				textStarted = true
				l.updateHeadlessProgress(slug, "active", "text", "drafting response", metrics, turnChannel)
			}
		case "tool_use":
			if firstToolAt.IsZero() {
				firstToolAt = time.Now()
				metrics.FirstToolMs = durationMillis(startedAt, firstToolAt)
			}
			line := fmt.Sprintf("tool_use: %s %s", event.ToolName, truncate(event.ToolInput, 120))
			appendHeadlessCodexLog(slug, line)
			l.updateHeadlessProgress(slug, "active", "tool_use", fmt.Sprintf("running %s", strings.TrimSpace(event.ToolName)), metrics, turnChannel)
		case "tool_result":
			line := "tool_result: " + truncate(event.Text, 140)
			appendHeadlessCodexLog(slug, line)
			l.updateHeadlessProgress(slug, "active", "tool_result", truncate(event.Text, 140), metrics, turnChannel)
		case "error":
			appendHeadlessCodexLog(slug, "stream_error: "+event.Detail)
			l.updateHeadlessProgress(slug, "error", "error", truncate(event.Detail, 180), metrics, turnChannel)
		}
	})
	_ = pw.Close() // signal scanner goroutine that stream is done (io.PipeWriter.Close always returns nil)
	if err := cmd.Wait(); err != nil {
		detail := firstNonEmpty(result.LastError, strings.TrimSpace(stderr.String()))
		if fallbackText := strings.TrimSpace(firstNonEmpty(result.FinalMessage, result.LastPlainLine, result.LastError)); fallbackText != "" {
			if shouldPublishHeadlessErrorFallback(fallbackText) {
				l.publishHeadlessFallbackReply(slug, notification, fallbackText, startedAt)
			} else {
				appendHeadlessCodexLog(slug, "fallback-suppressed: provider failure text was not published")
			}
		}
		metrics.TotalMs = time.Since(startedAt).Milliseconds()
		if detail != "" {
			appendHeadlessCodexLatency(slug, fmt.Sprintf("status=error profile=%s model=%q total_ms=%d first_event_ms=%d first_text_ms=%d first_tool_ms=%d detail=%q",
				route.Profile,
				route.Model,
				metrics.TotalMs,
				durationMillis(startedAt, firstEventAt),
				durationMillis(startedAt, firstTextAt),
				durationMillis(startedAt, firstToolAt),
				detail,
			))
			appendHeadlessCodexLog(slug, "stderr: "+detail)
			l.updateHeadlessProgress(slug, "error", "error", truncate(detail, 180), metrics, turnChannel)
			return fmt.Errorf("%w: %s", err, detail)
		}
		appendHeadlessCodexLatency(slug, fmt.Sprintf("status=error profile=%s model=%q total_ms=%d first_event_ms=%d first_text_ms=%d first_tool_ms=%d detail=%q",
			route.Profile,
			route.Model,
			metrics.TotalMs,
			durationMillis(startedAt, firstEventAt),
			durationMillis(startedAt, firstTextAt),
			durationMillis(startedAt, firstToolAt),
			err.Error(),
		))
		return err
	}
	if parseErr != nil {
		metrics.TotalMs = time.Since(startedAt).Milliseconds()
		l.updateHeadlessProgress(slug, "error", "error", truncate(parseErr.Error(), 180), metrics, turnChannel)
		return parseErr
	}
	metrics.TotalMs = time.Since(startedAt).Milliseconds()
	appendHeadlessCodexLatency(slug, fmt.Sprintf("status=ok profile=%s model=%q total_ms=%d first_event_ms=%d first_text_ms=%d first_tool_ms=%d final_chars=%d",
		route.Profile,
		route.Model,
		metrics.TotalMs,
		durationMillis(startedAt, firstEventAt),
		durationMillis(startedAt, firstTextAt),
		durationMillis(startedAt, firstToolAt),
		len(strings.TrimSpace(firstNonEmpty(result.FinalMessage, result.LastPlainLine))),
	))
	summary := strings.TrimSpace(formatHeadlessLatencySummary(metrics))
	if summary == "" {
		summary = "reply ready"
	} else {
		summary = "reply ready · " + summary
	}
	l.updateHeadlessProgress(slug, "idle", "idle", summary, metrics, turnChannel)
	if l.broker != nil && (result.Usage.InputTokens != 0 || result.Usage.OutputTokens != 0 || result.Usage.CacheReadTokens != 0 || result.Usage.CacheCreationTokens != 0 || result.Usage.CostUSD != 0) {
		l.broker.RecordAgentUsage(slug, route.Model, result.Usage)
	}
	if text := strings.TrimSpace(firstNonEmpty(result.FinalMessage, result.LastPlainLine)); text != "" {
		appendHeadlessCodexLog(slug, "result: "+text)
		l.publishHeadlessFallbackReply(slug, notification, text, startedAt)
	}
	return nil
}

func (l *Launcher) headlessCodexNeedsDangerousBypass(slug string, workspaceDir string, channel ...string) bool {
	if isOwnerOnlyCallableAgent(slug) {
		return true
	}
	if headlessWorkspaceNeedsDangerousBypass(workspaceDir, firstNonEmpty(
		func() string {
			if l != nil {
				return l.cwd
			}
			return ""
		}(),
	)) {
		return true
	}
	if l == nil || l.broker == nil {
		return false
	}
	task := l.headlessTaskForExecution(slug, channel...)
	if task == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(task.ExecutionMode)) {
	case "local_worktree", "external_workspace":
		return true
	default:
		return headlessWorkspaceNeedsDangerousBypass(workspaceDir, l.cwd)
	}
}

func headlessWorkspaceNeedsDangerousBypass(workspaceDir string, baseCWD string) bool {
	workspaceDir = normalizeHeadlessWorkspaceDir(workspaceDir)
	baseCWD = normalizeHeadlessWorkspaceDir(baseCWD)
	if workspaceDir == "" || baseCWD == "" {
		return false
	}
	return !strings.EqualFold(filepath.Clean(workspaceDir), filepath.Clean(baseCWD))
}

func (l *Launcher) buildHeadlessCodexEnv(slug string, workspaceDir string, channel string) []string {
	env := stripEnvKeys(os.Environ(), headlessCodexEnvVarsToStrip)
	if workspaceDir = normalizeHeadlessWorkspaceDir(workspaceDir); workspaceDir != "" {
		env = setEnvValue(env, "PWD", workspaceDir)
	}
	if codexHome := prepareHeadlessCodexHome(); codexHome != "" {
		if l != nil {
			_ = l.syncHeadlessCodexRuntimeConfig(codexHome, slug, workspaceDir, channel)
		}
		// Use the isolated runtime home for the headless Codex process so it
		// doesn't inherit user-global ~/.agents skills from the interactive shell.
		env = setEnvValue(env, "HOME", codexHome)
		_ = os.MkdirAll(filepath.Join(codexHome, "plugins", "cache"), 0o755)
		env = setEnvValue(env, "CODEX_HOME", codexHome)
	} else if home := config.RuntimeHomeDir(); strings.TrimSpace(home) != "" {
		env = setEnvValue(env, "HOME", home)
	}
	if base := l.headlessCodexWorkspaceCacheDir(workspaceDir); base != "" {
		goCache := filepath.Join(base, "go-build", strings.TrimSpace(slug))
		goTmp := filepath.Join(base, "go-tmp", strings.TrimSpace(slug))
		_ = os.MkdirAll(goCache, 0o755)
		_ = os.MkdirAll(goTmp, 0o755)
		env = setEnvValue(env, "GOCACHE", goCache)
		env = setEnvValue(env, "GOTMPDIR", goTmp)
	}
	env = setEnvValue(env, "WUPHF_AGENT_SLUG", slug)
	if channel = strings.TrimSpace(channel); channel != "" {
		env = setEnvValue(env, "WUPHF_CHANNEL", channel)
	}
	env = setEnvValue(env, "WUPHF_BROKER_TOKEN", l.broker.Token())
	env = setEnvValue(env, "WUPHF_BROKER_BASE_URL", l.BrokerBaseURL())
	env = setEnvValue(env, "WUPHF_HEADLESS_PROVIDER", "codex")
	if config.ResolveNoNex() {
		env = setEnvValue(env, "WUPHF_NO_NEX", "1")
	}
	if l.isOneOnOne() {
		env = setEnvValue(env, "WUPHF_ONE_ON_ONE", "1")
		env = setEnvValue(env, "WUPHF_ONE_ON_ONE_AGENT", l.oneOnOneAgent())
	}
	if secret := strings.TrimSpace(config.ResolveOneSecret()); secret != "" {
		env = setEnvValue(env, "ONE_SECRET", secret)
	}
	if identity := strings.TrimSpace(config.ResolveOneIdentity()); identity != "" {
		env = setEnvValue(env, "ONE_IDENTITY", identity)
		if identityType := strings.TrimSpace(config.ResolveOneIdentityType()); identityType != "" {
			env = setEnvValue(env, "ONE_IDENTITY_TYPE", identityType)
		}
	}
	if apiKey := strings.TrimSpace(config.ResolveAPIKey("")); apiKey != "" {
		env = setEnvValue(env, "WUPHF_API_KEY", apiKey)
		env = setEnvValue(env, "NEX_API_KEY", apiKey)
	}
	if openAIKey := strings.TrimSpace(config.ResolveOpenAIAPIKey()); openAIKey != "" {
		env = setEnvValue(env, "WUPHF_OPENAI_API_KEY", openAIKey)
		env = setEnvValue(env, "OPENAI_API_KEY", openAIKey)
	}
	return env
}

func headlessCodexHomeDir() string {
	if raw := strings.TrimSpace(os.Getenv("CODEX_HOME")); raw != "" {
		if abs, err := filepath.Abs(raw); err == nil && strings.TrimSpace(abs) != "" {
			return abs
		}
	}
	home := config.RuntimeHomeDir()
	home = strings.TrimSpace(home)
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".codex")
}

func headlessCodexGlobalHomeDir() string {
	if raw := strings.TrimSpace(os.Getenv("WUPHF_GLOBAL_HOME")); raw != "" {
		if abs, err := filepath.Abs(raw); err == nil && strings.TrimSpace(abs) != "" {
			return abs
		}
		return raw
	}
	return strings.TrimSpace(config.RuntimeHomeDir())
}

func headlessCodexRuntimeHomeDir() string {
	home := config.RuntimeHomeDir()
	home = strings.TrimSpace(home)
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".wuphf", "codex-headless")
}

func prepareHeadlessCodexHome() string {
	runtimeHome := normalizeHeadlessWorkspaceDir(headlessCodexRuntimeHomeDir())
	if runtimeHome == "" {
		return headlessCodexHomeDir()
	}
	if err := os.MkdirAll(runtimeHome, 0o755); err != nil {
		return headlessCodexHomeDir()
	}
	sourceHome := normalizeHeadlessWorkspaceDir(filepath.Join(headlessCodexGlobalHomeDir(), ".codex"))
	if sourceHome == "" {
		sourceHome = normalizeHeadlessWorkspaceDir(headlessCodexHomeDir())
	}
	if sourceHome != "" && sourceHome != runtimeHome {
		syncHeadlessCodexHomeFile(sourceHome, runtimeHome, "auth.json", 0o600)
		copyHeadlessCodexHomeFile(sourceHome, runtimeHome, "config.toml", 0o600)
	}
	if userHome := strings.TrimSpace(headlessCodexGlobalHomeDir()); userHome != "" {
		copyHeadlessCodexHomeFile(userHome, runtimeHome, filepath.Join(".one", "config.json"), 0o600)
		copyHeadlessCodexHomeFile(userHome, runtimeHome, filepath.Join(".one", "update-check.json"), 0o600)
	}
	return runtimeHome
}

func syncHeadlessCodexHomeFile(sourceHome string, runtimeHome string, rel string, mode os.FileMode) {
	if strings.TrimSpace(sourceHome) == "" || strings.TrimSpace(runtimeHome) == "" || strings.TrimSpace(rel) == "" {
		return
	}
	sourcePath := filepath.Join(sourceHome, filepath.FromSlash(rel))
	if _, err := os.Stat(sourcePath); errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(filepath.Join(runtimeHome, filepath.FromSlash(rel)))
		return
	}
	copyHeadlessCodexHomeFile(sourceHome, runtimeHome, rel, mode)
}

func copyHeadlessCodexHomeFile(sourceHome string, runtimeHome string, rel string, mode os.FileMode) {
	if strings.TrimSpace(sourceHome) == "" || strings.TrimSpace(runtimeHome) == "" || strings.TrimSpace(rel) == "" {
		return
	}
	sourcePath := filepath.Join(sourceHome, filepath.FromSlash(rel))
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return
	}
	destPath := filepath.Join(runtimeHome, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(destPath, data, mode)
}

func (l *Launcher) syncHeadlessCodexRuntimeConfig(runtimeHome string, slug string, workspaceDir string, channel string) error {
	runtimeHome = strings.TrimSpace(runtimeHome)
	if runtimeHome == "" {
		return nil
	}
	configPath := filepath.Join(runtimeHome, "config.toml")
	base, err := os.ReadFile(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	servers, err := l.buildScopedMCPServerMap(slug, channel, workspaceDir)
	if err != nil {
		return err
	}
	merged := stripManagedMCPServerBlocks(string(base))
	snippet := renderManagedMCPServersTOML(servers)
	if strings.TrimSpace(snippet) == "" {
		return nil
	}
	if strings.TrimSpace(merged) != "" && !strings.HasSuffix(merged, "\n") {
		merged += "\n"
	}
	if strings.TrimSpace(merged) != "" {
		merged += "\n"
	}
	merged += snippet
	return os.WriteFile(configPath, []byte(merged), 0o600)
}

func stripManagedMCPServerBlocks(src string) string {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))
	skip := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if strings.HasPrefix(trimmed, "[mcp_servers.") {
				skip = true
				continue
			}
			skip = false
		}
		if skip {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func renderManagedMCPServersTOML(servers map[string]any) string {
	if len(servers) == 0 {
		return ""
	}
	var b strings.Builder
	names := sortedAnyKeys(servers)
	for _, name := range names {
		server, ok := servers[name].(map[string]any)
		if !ok || len(server) == 0 {
			continue
		}
		fmt.Fprintf(&b, "[mcp_servers.%s]\n", tomlTableKey(name))
		if command, ok := anyString(server["command"]); ok && command != "" {
			fmt.Fprintf(&b, "command = %s\n", tomlQuote(command))
		}
		if args := anyStringSlice(server["args"]); len(args) > 0 {
			fmt.Fprintf(&b, "args = %s\n", tomlStringArray(args))
		}
		if timeout, ok := anyInt(server["startup_timeout_sec"]); ok && timeout > 0 {
			fmt.Fprintf(&b, "startup_timeout_sec = %d\n", timeout)
		}
		if env := anyStringMap(server["env"]); len(env) > 0 {
			fmt.Fprintf(&b, "\n[mcp_servers.%s.env]\n", tomlTableKey(name))
			for _, key := range sortedStringMapKeys(env) {
				fmt.Fprintf(&b, "%s = %s\n", tomlKeyName(key), tomlQuote(env[key]))
			}
		}
		if tools := anyNestedMap(server["tools"]); len(tools) > 0 {
			for _, toolName := range sortedAnyKeys(tools) {
				toolCfg, ok := tools[toolName].(map[string]any)
				if !ok || len(toolCfg) == 0 {
					continue
				}
				fmt.Fprintf(&b, "\n[mcp_servers.%s.tools.%s]\n", tomlTableKey(name), tomlTableKey(toolName))
				for _, key := range sortedAnyKeys(toolCfg) {
					value := toolCfg[key]
					switch v := value.(type) {
					case string:
						fmt.Fprintf(&b, "%s = %s\n", tomlKeyName(key), tomlQuote(v))
					case bool:
						fmt.Fprintf(&b, "%s = %t\n", tomlKeyName(key), v)
					default:
						if n, ok := anyInt(v); ok {
							fmt.Fprintf(&b, "%s = %d\n", tomlKeyName(key), n)
						}
					}
				}
			}
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String()) + "\n"
}

func sortedAnyKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func anyString(v any) (string, bool) {
	switch value := v.(type) {
	case string:
		return value, true
	default:
		return "", false
	}
}

func anyStringSlice(v any) []string {
	switch value := v.(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func anyStringMap(v any) map[string]string {
	switch value := v.(type) {
	case map[string]string:
		out := make(map[string]string, len(value))
		for key, item := range value {
			out[key] = item
		}
		return out
	case map[string]any:
		out := make(map[string]string, len(value))
		for key, item := range value {
			if s, ok := item.(string); ok {
				out[key] = s
			}
		}
		return out
	default:
		return nil
	}
}

func anyNestedMap(v any) map[string]any {
	if value, ok := v.(map[string]any); ok {
		return value
	}
	return nil
}

func anyInt(v any) (int, bool) {
	switch value := v.(type) {
	case int:
		return value, true
	case int64:
		return int(value), true
	case float64:
		return int(value), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(value))
		return n, err == nil
	default:
		return 0, false
	}
}

func normalizeHeadlessWorkspaceDir(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil && strings.TrimSpace(abs) != "" {
		path = abs
	}
	if real, err := filepath.EvalSymlinks(path); err == nil && strings.TrimSpace(real) != "" {
		path = real
	}
	return path
}

func headlessCodexNeedsWebsocketHeaderWorkaround(workspaceDir string) bool {
	return strings.TrimSpace(headlessCodexWebsocketHeaderWorkaroundReason(workspaceDir)) != ""
}

func headlessCodexWebsocketHeaderWorkaroundReason(workspaceDir string) string {
	if headlessPathContainsNonASCII(workspaceDir) {
		return "disabled codex-lb websockets for non-ASCII workspace path"
	}
	for _, path := range headlessCodexConfigPathsForWebsocketHeaderWorkaround() {
		if headlessCodexConfigContainsNonASCIIProjectPath(path) {
			return fmt.Sprintf("disabled codex-lb websockets for non-ASCII trusted project path in %s", path)
		}
	}
	return ""
}

func headlessCodexConfigPathsForWebsocketHeaderWorkaround() []string {
	seen := map[string]struct{}{}
	paths := make([]string, 0, 3)
	add := func(path string) {
		path = normalizeHeadlessWorkspaceDir(path)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}

	add(filepath.Join(headlessCodexRuntimeHomeDir(), "config.toml"))
	add(filepath.Join(headlessCodexHomeDir(), "config.toml"))
	add(filepath.Join(headlessCodexGlobalHomeDir(), ".codex", "config.toml"))

	return paths
}

func headlessCodexConfigContainsNonASCIIProjectPath(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "[projects.") {
			continue
		}
		if headlessBytesContainNonASCII([]byte(line)) {
			return true
		}
	}
	return false
}

func headlessPathContainsNonASCII(path string) bool {
	path = normalizeHeadlessWorkspaceDir(path)
	if path == "" {
		return false
	}
	return headlessBytesContainNonASCII([]byte(path))
}

func headlessBytesContainNonASCII(raw []byte) bool {
	for _, b := range raw {
		if b >= utf8.RuneSelf {
			return true
		}
	}
	return false
}

func (l *Launcher) currentHeadlessTurn(slug string, channel ...string) *headlessCodexTurn {
	if l == nil {
		return nil
	}
	laneKey := ""
	if len(channel) > 0 {
		laneKey = agentLaneKey(channel[0], slug)
	}
	l.headlessMu.Lock()
	defer l.headlessMu.Unlock()
	if laneKey != "" {
		if active := l.headlessActive[laneKey]; active != nil {
			turn := active.Turn
			return &turn
		}
	}
	var found *headlessCodexTurn
	for key, active := range l.headlessActive {
		if active == nil || agentLaneSlug(key) != normalizeAgentLaneSlug(slug) {
			continue
		}
		if found != nil {
			return nil
		}
		turn := active.Turn
		found = &turn
	}
	return found
}

func (l *Launcher) headlessTurnChannel(slug string, channel ...string) string {
	if resolved := normalizeChannelSlug(firstNonEmpty(channel...)); resolved != "" {
		return resolved
	}
	if turn := l.currentHeadlessTurn(slug, channel...); turn != nil {
		if resolved := normalizeChannelSlug(turn.Channel); resolved != "" {
			return resolved
		}
	}
	if task := l.headlessTaskForExecution(slug, channel...); task != nil {
		return normalizeChannelSlug(task.Channel)
	}
	return ""
}

func (l *Launcher) headlessCodexModel(slug string, channel ...string) string {
	return l.resolveHeadlessModelRoute(provider.KindCodex, slug, "", channel...).Model
}

func (l *Launcher) headlessCodexReasoningEffort(slug string, channel ...string) string {
	if l == nil {
		return ""
	}
	if task := l.headlessTaskForExecution(slug, channel...); task != nil {
		effort, err := normalizeReasoningEffort(task.ReasoningEffort)
		if err == nil {
			return effort
		}
	}
	return ""
}

func (l *Launcher) channelPrimaryWorkspaceDir(slug string, channel string) string {
	if l == nil {
		return ""
	}
	if task := l.agentActiveTask(slug, channel); task != nil {
		if workspaceDir := l.taskExecutionWorkspaceDir(task); workspaceDir != "" {
			return workspaceDir
		}
	}
	if l.broker == nil {
		return ""
	}
	return normalizeHeadlessWorkspaceDir(l.broker.channelPrimaryLinkedRepoPath(channel))
}

func (l *Launcher) headlessWorkspaceDirForExecution(slug string, channel string) string {
	if workspaceDir := l.headlessTaskWorkspaceDirForTurn(slug, l.currentHeadlessTurn(slug, channel), channel); workspaceDir != "" {
		return workspaceDir
	}
	if workspaceDir := l.channelPrimaryWorkspaceDir(slug, channel); workspaceDir != "" {
		return workspaceDir
	}
	return strings.TrimSpace(l.cwd)
}

func (l *Launcher) headlessTaskForTurn(slug string, turn *headlessCodexTurn, channel ...string) *teamTask {
	if l == nil || l.broker == nil {
		return nil
	}
	if turn != nil {
		return l.timedOutTaskForTurn(slug, *turn)
	}
	return l.agentActiveTask(slug, channel...)
}

func (l *Launcher) headlessTaskForExecution(slug string, channel ...string) *teamTask {
	if task := l.headlessTaskForTurn(slug, l.currentHeadlessTurn(slug, channel...), channel...); task != nil {
		return task
	}
	return l.agentActiveTask(slug, channel...)
}

func (l *Launcher) headlessTaskWorkspaceDirForTurn(slug string, turn *headlessCodexTurn, channel ...string) string {
	task := l.headlessTaskForTurn(slug, turn, channel...)
	if task == nil {
		return ""
	}
	return l.taskExecutionWorkspaceDir(task)
}

func (l *Launcher) headlessCodexWorkspaceCacheDir(workspaceDir string) string {
	base := strings.TrimSpace(workspaceDir)
	if base == "" {
		base = strings.TrimSpace(l.cwd)
	}
	if base == "" {
		if wd, err := os.Getwd(); err == nil {
			base = wd
		}
	}
	if base == "" {
		return ""
	}
	return filepath.Join(base, ".wuphf", "cache")
}

func (l *Launcher) headlessTaskWorkspaceDir(slug string, channel ...string) string {
	return l.headlessTaskWorkspaceDirForTurn(slug, nil, channel...)
}

func (l *Launcher) buildCodexOfficeConfigOverrides(slug string) ([]string, error) {
	wuphfBinary, err := headlessCodexExecutablePath()
	if err != nil {
		return nil, err
	}
	wuphfEnvVars := []string{
		"WUPHF_AGENT_SLUG",
		"WUPHF_BROKER_TOKEN",
		"WUPHF_BROKER_BASE_URL",
	}
	if config.ResolveNoNex() {
		wuphfEnvVars = append(wuphfEnvVars, "WUPHF_NO_NEX")
	}
	if l.isOneOnOne() {
		wuphfEnvVars = append(wuphfEnvVars,
			"WUPHF_ONE_ON_ONE",
			"WUPHF_ONE_ON_ONE_AGENT",
		)
	}
	if secret := strings.TrimSpace(config.ResolveOneSecret()); secret != "" {
		wuphfEnvVars = append(wuphfEnvVars, "ONE_SECRET")
	}
	if identity := strings.TrimSpace(config.ResolveOneIdentity()); identity != "" {
		wuphfEnvVars = append(wuphfEnvVars, "ONE_IDENTITY")
		if identityType := strings.TrimSpace(config.ResolveOneIdentityType()); identityType != "" {
			wuphfEnvVars = append(wuphfEnvVars, "ONE_IDENTITY_TYPE")
		}
	}

	overrides := []string{
		fmt.Sprintf(`mcp_servers.wuphf-office.command=%s`, tomlQuote(wuphfBinary)),
		`mcp_servers.wuphf-office.args=["mcp-team"]`,
		fmt.Sprintf(`mcp_servers.wuphf-office.env_vars=%s`, tomlStringArray(wuphfEnvVars)),
	}

	if !config.ResolveNoNex() {
		if nexMCP, err := headlessCodexLookPath("nex-mcp"); err == nil {
			overrides = append(overrides, fmt.Sprintf(`mcp_servers.nex.command=%s`, tomlQuote(nexMCP)))
			if apiKey := strings.TrimSpace(config.ResolveAPIKey("")); apiKey != "" {
				overrides = append(overrides, fmt.Sprintf(`mcp_servers.nex.env_vars=%s`, tomlStringArray([]string{
					"WUPHF_API_KEY",
					"NEX_API_KEY",
				})))
			}
		}
	}

	return overrides, nil
}

func buildHeadlessCodexPrompt(systemPrompt string, prompt string) string {
	var parts []string
	if trimmed := strings.TrimSpace(systemPrompt); trimmed != "" {
		parts = append(parts, "<system>\n"+trimmed+"\n</system>")
	}
	parts = append(parts, strings.TrimSpace(`RUNTIME NOTE FOR CODEX EXEC:
- Do not use read_mcp_resource, list_mcp_resources, or list_mcp_resource_templates against the filesystem MCP server; that server may expose file tools without implementing MCP resources.
- For repo inspection, prefer filesystem file tools first (for example read_text_file, read_multiple_files, search_files, list_directory). If that path is unavailable, fall back to short local shell reads in the assigned working_directory.
- Treat "resources/list failed", "resources/read failed", "unknown MCP server", "blocked by policy", "safe.directory", and read-only sandbox errors as runtime/tooling issues. Report them explicitly instead of treating them as proof that the repo itself is broken.
- On Windows shell commands, assume a clean no-profile PowerShell environment; do not rely on profile scripts, aliases, or dot-sourcing.`))
	if trimmed := strings.TrimSpace(prompt); trimmed != "" {
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, "\n\n")
}

func wuphfLogDir() string {
	home := config.RuntimeHomeDir()
	if strings.TrimSpace(home) == "" {
		return ""
	}
	dir := filepath.Join(home, ".wuphf", "logs")
	_ = os.MkdirAll(dir, 0o700)
	return dir
}

func appendHeadlessCodexLog(slug string, line string) {
	dir := wuphfLogDir()
	if dir == "" {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "headless-codex-"+slug+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = fmt.Fprintf(f, "[%s] %s\n", time.Now().Format(time.RFC3339), strings.TrimSpace(line))
}

func appendHeadlessCodexLatency(slug string, line string) {
	dir := wuphfLogDir()
	if dir == "" {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "headless-codex-latency.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = fmt.Fprintf(f, "[%s] agent=%s %s\n", time.Now().Format(time.RFC3339), strings.TrimSpace(slug), strings.TrimSpace(line))
}

func durationMillis(start, mark time.Time) int64 {
	if start.IsZero() || mark.IsZero() {
		return -1
	}
	return mark.Sub(start).Milliseconds()
}

func tomlQuote(value string) string {
	return fmt.Sprintf("%q", value)
}

func tomlKeyName(value string) string {
	if tomlBareKeyPattern.MatchString(value) {
		return value
	}
	return tomlQuote(value)
}

func tomlTableKey(value string) string {
	return fmt.Sprintf("%q", value)
}

func tomlStringArray(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		parts = append(parts, tomlQuote(value))
	}
	if len(parts) == 0 {
		return "[]"
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func setEnvValue(env []string, key string, value string) []string {
	key = strings.TrimSpace(key)
	if key == "" {
		return env
	}
	prefix := key + "="
	filtered := env[:0]
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return append(filtered, prefix+value)
}

func stripEnvKeys(env []string, strip []string) []string {
	if len(strip) == 0 {
		return env
	}
	stripSet := make(map[string]struct{}, len(strip))
	for _, key := range strip {
		key = strings.TrimSpace(key)
		if key != "" {
			stripSet[key] = struct{}{}
		}
	}
	if len(stripSet) == 0 {
		return env
	}
	out := make([]string, 0, len(env))
	for _, entry := range env {
		key := entry
		if idx := strings.IndexByte(entry, '='); idx >= 0 {
			key = entry[:idx]
		}
		if _, ok := stripSet[key]; ok {
			continue
		}
		out = append(out, entry)
	}
	return out
}
