package team

import (
	"fmt"
	"strings"
	"time"
)

type RuntimeTask struct {
	ID                     string
	Title                  string
	Owner                  string
	Status                 string
	PipelineStage          string
	ReviewState            string
	ExecutionMode          string
	RuntimeProvider        string
	RuntimeModel           string
	ReasoningEffort        string
	WorkspacePath          string
	WorktreePath           string
	WorktreeBranch         string
	ThreadID               string
	HandoffStatus          string
	BlockerRequestIDs      []string
	BlockingReviewFindings int
	ReconciliationPending  bool
	ReconciliationReason   string
	ChangedPaths           []string
	UntrackedPaths         []string
	IssuePublicationStatus string
	IssueURL               string
	PRPublicationStatus    string
	PRURL                  string
	NextAction             string
	ReplyChannel           string
	ReplyTo                string
	RelevantBlockers       []string
	Blocked                bool
}

type RuntimeRequest struct {
	ID       string
	Kind     string
	Title    string
	Question string
	From     string
	Blocking bool
	Required bool
	Status   string
	Channel  string
	Secret   bool
}

type RuntimeMessage struct {
	ID        string
	From      string
	Title     string
	Content   string
	ReplyTo   string
	Timestamp string
}

type RuntimeSnapshot struct {
	Channel       string
	SessionMode   string
	DirectAgent   string
	GeneratedAt   time.Time
	Tasks         []RuntimeTask
	Requests      []RuntimeRequest
	Recent        []RuntimeMessage
	Artifacts     []RuntimeArtifact
	Capabilities  RuntimeCapabilities
	Registry      CapabilityRegistry
	OpenSpec      OpenSpecSummary
	Observability SessionObservabilitySnapshot
	Memory        SessionMemorySnapshot
	Recovery      SessionRecovery
}

type RuntimeSnapshotInput struct {
	Channel       string
	SessionMode   string
	DirectAgent   string
	Tasks         []RuntimeTask
	Requests      []RuntimeRequest
	Recent        []RuntimeMessage
	Artifacts     []RuntimeArtifact
	Capabilities  RuntimeCapabilities
	Registry      CapabilityRegistry
	Observability SessionObservabilitySnapshot
	Now           time.Time
}

func BuildRuntimeSnapshot(input RuntimeSnapshotInput) RuntimeSnapshot {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	sessionMode := NormalizeSessionMode(input.SessionMode)
	directAgent := NormalizeOneOnOneAgent(input.DirectAgent)
	if sessionMode != SessionModeOneOnOne {
		directAgent = ""
	}
	snapshot := RuntimeSnapshot{
		Channel:       strings.TrimSpace(input.Channel),
		SessionMode:   sessionMode,
		DirectAgent:   directAgent,
		GeneratedAt:   now,
		Tasks:         append([]RuntimeTask(nil), input.Tasks...),
		Requests:      append([]RuntimeRequest(nil), input.Requests...),
		Recent:        append([]RuntimeMessage(nil), input.Recent...),
		Artifacts:     append([]RuntimeArtifact(nil), input.Artifacts...),
		Capabilities:  input.Capabilities,
		Registry:      input.Registry,
		Observability: input.Observability,
	}
	if len(snapshot.Registry.Entries) == 0 {
		snapshot.Registry = BuildCapabilityRegistry(snapshot.Capabilities)
	}
	snapshot.OpenSpec = detectOpenSpecForRuntimeTasks(snapshot.Tasks)
	snapshot.Memory = BuildSessionMemorySnapshot(sessionMode, directAgent, snapshot.Tasks, snapshot.Requests, snapshot.Recent)
	snapshot.Recovery = snapshot.Memory.ToRecovery()
	return snapshot
}

func (s RuntimeSnapshot) FormatText() string {
	channel := s.Channel
	if channel == "" {
		channel = "general"
	}

	lines := []string{
		fmt.Sprintf("Runtime state for #%s", channel),
	}
	if s.SessionMode == SessionModeOneOnOne {
		lines = append(lines, fmt.Sprintf("- Session mode: 1:1 with @%s", s.DirectAgent))
	} else {
		lines = append(lines, "- Session mode: office")
	}
	lines = append(lines,
		fmt.Sprintf("- Running tasks: %d of %d", s.runningTaskCount(), len(s.Tasks)),
		fmt.Sprintf("- Isolated worktrees: %d", s.isolatedTaskCount()),
		fmt.Sprintf("- Pending human requests: %d", s.pendingRequestCount()),
	)
	if len(s.Artifacts) > 0 {
		lines = append(lines, fmt.Sprintf("- Retained execution artifacts: %d", len(s.Artifacts)))
	}

	if focus := strings.TrimSpace(s.Recovery.Focus); focus != "" {
		lines = append(lines, fmt.Sprintf("- Current focus: %s", focus))
	}

	if len(s.Recovery.NextSteps) > 0 {
		lines = append(lines, "", "Next steps:")
		for _, step := range s.Recovery.NextSteps {
			lines = append(lines, "- "+step)
		}
	}

	if len(s.Recovery.Highlights) > 0 {
		lines = append(lines, "", "Recent highlights:")
		for _, line := range s.Recovery.Highlights {
			lines = append(lines, "- "+line)
		}
	}

	if guidanceLines := s.resumeGuidanceLines(); len(guidanceLines) > 0 {
		lines = append(lines, "", "Resume guidance:")
		lines = append(lines, guidanceLines...)
	}

	if openSpecLines := s.OpenSpec.FormatLines(); len(openSpecLines) > 0 {
		lines = append(lines, "", "OpenSpec:")
		lines = append(lines, openSpecLines...)
	}

	if len(s.Artifacts) > 0 {
		lines = append(lines, "", "Execution artifacts:")
		for _, artifact := range firstRuntimeArtifacts(s.Artifacts, 4) {
			line := fmt.Sprintf("- %s [%s]", artifact.EffectiveTitle(), artifact.Kind)
			if state := strings.TrimSpace(artifact.State); state != "" {
				line += " " + strings.ReplaceAll(state, "_", " ")
			}
			if summary := strings.TrimSpace(artifact.Summary); summary != "" {
				line += ": " + summary
			}
			lines = append(lines, line)
		}
	}

	if tmuxLines := s.Capabilities.Tmux.FormatLines(); len(tmuxLines) > 0 {
		lines = append(lines, "", "Tmux runtime:")
		lines = append(lines, tmuxLines...)
	}

	if len(s.Capabilities.Items) > 0 {
		lines = append(lines, "", "Runtime capabilities:")
		for _, item := range s.Capabilities.Items {
			line := fmt.Sprintf("- %s [%s]: %s", item.Name, item.Level, item.Detail)
			if next := strings.TrimSpace(item.NextStep); next != "" {
				line += " Next: " + next
			}
			lines = append(lines, line)
		}
	}

	if len(s.Registry.Entries) > 0 {
		lines = append(lines, "", "Capability registry:")
		for _, item := range s.Registry.Entries {
			line := fmt.Sprintf("- %s (%s) [%s]: %s", item.Label, item.Category, item.Level, item.Detail)
			if next := strings.TrimSpace(item.NextStep); next != "" {
				line += " Next: " + next
			}
			lines = append(lines, line)
		}
	}

	if observabilityLines := s.Observability.FormatLines(); len(observabilityLines) > 0 {
		lines = append(lines, "", "Session observability:")
		lines = append(lines, observabilityLines...)
	}

	return strings.Join(lines, "\n")
}

func (s RuntimeSnapshot) runningTaskCount() int {
	count := 0
	for _, task := range s.Tasks {
		if runtimeTaskIsRunning(task) {
			count++
		}
	}
	return count
}

func (s RuntimeSnapshot) isolatedTaskCount() int {
	count := 0
	for _, task := range s.Tasks {
		if runtimeTaskUsesIsolation(task) {
			count++
		}
	}
	return count
}

func (s RuntimeSnapshot) pendingRequestCount() int {
	count := 0
	for _, req := range s.Requests {
		status := strings.ToLower(strings.TrimSpace(req.Status))
		if status == "" || status == "pending" || status == "open" {
			count++
		}
	}
	return count
}

func runtimeTaskIsRunning(task RuntimeTask) bool {
	status := strings.ToLower(strings.TrimSpace(task.Status))
	switch status {
	case "", "done", "completed", "canceled", "cancelled":
		return false
	default:
		return true
	}
}

func runtimeTaskUsesIsolation(task RuntimeTask) bool {
	return strings.EqualFold(strings.TrimSpace(task.ExecutionMode), "local_worktree") ||
		strings.EqualFold(strings.TrimSpace(task.ExecutionMode), "external_workspace") ||
		strings.TrimSpace(task.WorkspacePath) != "" ||
		strings.TrimSpace(task.WorktreePath) != "" ||
		strings.TrimSpace(task.WorktreeBranch) != ""
}

func firstRuntimeArtifacts(artifacts []RuntimeArtifact, limit int) []RuntimeArtifact {
	if limit <= 0 || len(artifacts) <= limit {
		return append([]RuntimeArtifact(nil), artifacts...)
	}
	return append([]RuntimeArtifact(nil), artifacts[:limit]...)
}

func runtimeTaskFromTeamTask(task teamTask) RuntimeTask {
	runtimeTask := RuntimeTask{
		ID:                     strings.TrimSpace(task.ID),
		Title:                  strings.TrimSpace(task.Title),
		Owner:                  strings.TrimSpace(task.Owner),
		Status:                 strings.TrimSpace(task.Status),
		PipelineStage:          strings.TrimSpace(task.PipelineStage),
		ReviewState:            strings.TrimSpace(task.ReviewState),
		ExecutionMode:          strings.TrimSpace(task.ExecutionMode),
		RuntimeProvider:        strings.TrimSpace(task.RuntimeProvider),
		RuntimeModel:           strings.TrimSpace(task.RuntimeModel),
		ReasoningEffort:        strings.TrimSpace(task.ReasoningEffort),
		WorkspacePath:          strings.TrimSpace(task.WorkspacePath),
		WorktreePath:           strings.TrimSpace(task.WorktreePath),
		WorktreeBranch:         strings.TrimSpace(task.WorktreeBranch),
		ThreadID:               strings.TrimSpace(task.ThreadID),
		HandoffStatus:          strings.TrimSpace(task.HandoffStatus),
		BlockerRequestIDs:      append([]string(nil), task.BlockerRequestIDs...),
		BlockingReviewFindings: countBlockingReviewFindings(task.ReviewFindings),
		ReconciliationPending:  taskHasPendingReconciliation(&task),
		IssuePublicationStatus: publicationStatus(task.IssuePublication),
		IssueURL:               publicationURL(task.IssuePublication),
		PRPublicationStatus:    publicationStatus(task.PRPublication),
		PRURL:                  publicationURL(task.PRPublication),
		Blocked:                task.Blocked,
	}
	if task.Reconciliation != nil {
		runtimeTask.ReconciliationReason = strings.TrimSpace(task.Reconciliation.Reason)
		runtimeTask.ChangedPaths = append([]string(nil), task.Reconciliation.ChangedPaths...)
		runtimeTask.UntrackedPaths = append([]string(nil), task.Reconciliation.UntrackedPaths...)
	}
	runtimeTask.ReplyChannel = normalizeChannelSlug(task.Channel)
	runtimeTask.ReplyTo = strings.TrimSpace(task.ThreadID)
	runtimeTask.RelevantBlockers = summarizeTaskResumeBlockers(task)
	runtimeTask.NextAction = runtimeTaskNextAction(runtimeTask)
	return runtimeTask
}

func summarizeTaskResumeBlockers(task teamTask) []string {
	blockers := make([]string, 0, len(task.BlockerRequestIDs)+3)
	if len(task.BlockerRequestIDs) > 0 {
		blockers = append(blockers, fmt.Sprintf("Pending blocker request(s): %s", strings.Join(task.BlockerRequestIDs, ", ")))
	}
	if task.LastHandoff == nil {
		return blockers
	}
	for _, blocker := range task.LastHandoff.Blockers {
		summary := summarizeStructuredTaskBlocker(blocker)
		if summary == "" {
			continue
		}
		blockers = appendUnique(blockers, summary)
	}
	return blockers
}

func summarizeStructuredTaskBlocker(blocker taskBlocker) string {
	need := strings.TrimSpace(blocker.Need)
	waitingOn := strings.TrimSpace(blocker.WaitingOn)
	question := strings.TrimSpace(blocker.Question)
	context := strings.TrimSpace(blocker.Context)
	switch {
	case need != "" && waitingOn != "":
		return fmt.Sprintf("Need %s; waiting on %s", need, waitingOn)
	case question != "" && waitingOn != "":
		return fmt.Sprintf("%s; waiting on %s", question, waitingOn)
	case need != "":
		return "Need " + need
	case question != "":
		return question
	case context != "":
		return truncateRecoveryText(context, 120)
	default:
		return strings.TrimSpace(blocker.Kind)
	}
}

func runtimeTaskNextAction(task RuntimeTask) string {
	if next := strings.TrimSpace(task.NextAction); next != "" {
		return next
	}
	taskID := strings.TrimSpace(task.ID)
	if taskID == "" {
		taskID = "the task"
	}
	if task.ReconciliationPending {
		path := effectiveRuntimeTaskWorkspacePath(task)
		if path == "" {
			path = strings.TrimSpace(task.WorktreePath)
		}
		if path != "" {
			return fmt.Sprintf("Inspect the pending workspace delta in %s, reconcile the changed/untracked files, and submit a structured reconcile handoff before resuming %s.", path, taskID)
		}
		return fmt.Sprintf("Inspect the pending workspace delta, reconcile the changed/untracked files, and submit a structured reconcile handoff before resuming %s.", taskID)
	}
	if strings.EqualFold(strings.TrimSpace(task.HandoffStatus), "repair_required") {
		return fmt.Sprintf("Repair the structured handoff for %s before advancing the task state.", taskID)
	}
	if task.BlockingReviewFindings > 0 {
		return fmt.Sprintf("Resolve %d blocking review finding(s) and resubmit %s for review.", task.BlockingReviewFindings, taskID)
	}
	if blockers := runtimeTaskResumeBlockers(task); len(blockers) > 0 {
		return fmt.Sprintf("Resolve the active blocker on %s before continuing: %s.", taskID, blockers[0])
	}
	if state := strings.TrimSpace(task.ReviewState); state != "" && state != "not_required" && state != "approved" {
		return fmt.Sprintf("Continue the review flow for %s (%s) and post the next update in-thread.", taskID, state)
	}
	if path := effectiveRuntimeTaskWorkspacePath(task); path != "" {
		return fmt.Sprintf("Resume execution in %s and continue %s.", path, taskID)
	}
	return fmt.Sprintf("Continue %s from its latest task context and post the next concrete update in-thread.", taskID)
}

func runtimeTaskReplyChannel(task RuntimeTask) string {
	channel := normalizeChannelSlug(task.ReplyChannel)
	if channel != "" {
		return channel
	}
	return "general"
}

func runtimeTaskReplyTo(task RuntimeTask) string {
	if replyTo := strings.TrimSpace(task.ReplyTo); replyTo != "" {
		return replyTo
	}
	return strings.TrimSpace(task.ThreadID)
}

func runtimeTaskResumeBlockers(task RuntimeTask) []string {
	if len(task.RelevantBlockers) > 0 {
		return append([]string(nil), task.RelevantBlockers...)
	}
	if len(task.BlockerRequestIDs) == 0 {
		return nil
	}
	return []string{fmt.Sprintf("Pending blocker request(s): %s", strings.Join(task.BlockerRequestIDs, ", "))}
}

func (s RuntimeSnapshot) resumeGuidanceLines() []string {
	lines := []string{}
	for _, task := range s.Tasks {
		if !runtimeTaskIsRunning(task) {
			continue
		}
		next := runtimeTaskNextAction(task)
		if next == "" {
			continue
		}
		taskID := strings.TrimSpace(task.ID)
		if taskID == "" {
			taskID = strings.TrimSpace(task.Title)
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", taskID, next))
		if len(task.ChangedPaths) > 0 {
			lines = append(lines, "  Changed paths: "+strings.Join(task.ChangedPaths, ", "))
		}
		if len(task.UntrackedPaths) > 0 {
			lines = append(lines, "  Untracked paths: "+strings.Join(task.UntrackedPaths, ", "))
		}
		if blockers := runtimeTaskResumeBlockers(task); len(blockers) > 0 {
			lines = append(lines, "  Blockers: "+strings.Join(blockers, " | "))
		}
		if replyTo := runtimeTaskReplyTo(task); replyTo != "" {
			lines = append(lines, fmt.Sprintf("  Reply route: #%s -> %s", runtimeTaskReplyChannel(task), replyTo))
		} else {
			lines = append(lines, fmt.Sprintf("  Reply route: #%s", runtimeTaskReplyChannel(task)))
		}
	}
	return lines
}
