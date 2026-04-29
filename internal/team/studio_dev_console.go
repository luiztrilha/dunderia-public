package team

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nex-crm/wuphf/internal/buildinfo"
	"github.com/nex-crm/wuphf/internal/config"
)

type studioDevConsoleResponse struct {
	Office        studioOfficeSnapshot        `json:"office"`
	Environment   studioEnvironmentSnapshot   `json:"environment"`
	ActiveContext studioActiveContextSnapshot `json:"active_context"`
	Blockers      []studioBlocker             `json:"blockers"`
	Actions       []studioActionDefinition    `json:"actions"`
}

type studioOfficeSnapshot struct {
	Status        string                     `json:"status"`
	Provider      string                     `json:"provider,omitempty"`
	FocusMode     bool                       `json:"focus_mode"`
	SessionMode   string                     `json:"session_mode,omitempty"`
	MemoryBackend string                     `json:"memory_backend,omitempty"`
	Health        studioBrokerHealthSnapshot `json:"health"`
	Bootstrap     studioBootstrapSnapshot    `json:"bootstrap"`
	TaskCounts    studioTaskCounts           `json:"task_counts"`
}

type studioBrokerHealthSnapshot struct {
	BrokerReachable bool     `json:"broker_reachable"`
	APIReachable    bool     `json:"api_reachable"`
	WebReachable    bool     `json:"web_reachable"`
	Degraded        bool     `json:"degraded"`
	Signals         []string `json:"signals,omitempty"`
	Build           any      `json:"build,omitempty"`
}

type studioBootstrapSnapshot struct {
	Ready      bool   `json:"ready"`
	Summary    string `json:"summary"`
	Members    int    `json:"members,omitempty"`
	Channels   int    `json:"channels,omitempty"`
	Tasks      int    `json:"tasks,omitempty"`
	Requests   int    `json:"requests,omitempty"`
	Workspaces int    `json:"workspaces,omitempty"`
	Workflows  int    `json:"workflows,omitempty"`
}

type studioTaskCounts struct {
	Total      int `json:"total"`
	Open       int `json:"open,omitempty"`
	InProgress int `json:"in_progress,omitempty"`
	Blocked    int `json:"blocked,omitempty"`
	Review     int `json:"review,omitempty"`
	Done       int `json:"done,omitempty"`
	Canceled   int `json:"canceled,omitempty"`
	Other      int `json:"other,omitempty"`
}

type studioEnvironmentSnapshot struct {
	Status                string   `json:"status"`
	BrokerReachable       bool     `json:"broker_reachable"`
	APIReachable          bool     `json:"api_reachable"`
	WebReachable          bool     `json:"web_reachable"`
	MemoryBackendSelected string   `json:"memory_backend_selected,omitempty"`
	MemoryBackendActive   string   `json:"memory_backend_active,omitempty"`
	MemoryBackendReady    bool     `json:"memory_backend_ready"`
	Degraded              bool     `json:"degraded"`
	Signals               []string `json:"signals,omitempty"`
	Build                 any      `json:"build,omitempty"`
}

type studioActiveContextSnapshot struct {
	SessionMode    string                    `json:"session_mode,omitempty"`
	DirectAgent    string                    `json:"direct_agent,omitempty"`
	Focus          string                    `json:"focus,omitempty"`
	NextSteps      []string                  `json:"next_steps,omitempty"`
	PrimaryChannel string                    `json:"primary_channel,omitempty"`
	Channels       []studioChannelSnapshot   `json:"channels,omitempty"`
	Flows          []studioFlowSnapshot      `json:"flows,omitempty"`
	Workspaces     []studioWorkspaceSnapshot `json:"workspaces,omitempty"`
	Tasks          []studioTaskSnapshot      `json:"tasks,omitempty"`
	Requests       []studioRequestSnapshot   `json:"requests,omitempty"`
	Messages       []studioMessageSnapshot   `json:"messages,omitempty"`
}

type studioAttentionGroup struct {
	Key      string   `json:"key"`
	Kind     string   `json:"kind"`
	Severity string   `json:"severity"`
	Title    string   `json:"title"`
	Summary  string   `json:"summary"`
	Count    int      `json:"count"`
	LatestAt string   `json:"latest_at,omitempty"`
	TaskIDs  []string `json:"task_ids,omitempty"`
}

type studioChannelSnapshot struct {
	Slug                    string                 `json:"slug"`
	Name                    string                 `json:"name,omitempty"`
	Members                 []string               `json:"members,omitempty"`
	TaskCounts              studioTaskCounts       `json:"task_counts"`
	RequestCount            int                    `json:"request_count,omitempty"`
	FlowCount               int                    `json:"flow_count,omitempty"`
	WorkspaceCount          int                    `json:"workspace_count,omitempty"`
	Blockers                []string               `json:"blockers,omitempty"`
	AttentionCount          int                    `json:"attention_count,omitempty"`
	WaitingHumanCount       int                    `json:"waiting_human_count,omitempty"`
	ActiveOwnerCount        int                    `json:"active_owner_count,omitempty"`
	LastSubstantiveUpdateAt string                 `json:"last_substantive_update_at,omitempty"`
	LastSubstantiveUpdateBy string                 `json:"last_substantive_update_by,omitempty"`
	LastSubstantivePreview  string                 `json:"last_substantive_preview,omitempty"`
	LastDecisionAt          string                 `json:"last_decision_at,omitempty"`
	LastDecisionSummary     string                 `json:"last_decision_summary,omitempty"`
	Attention               []studioAttentionGroup `json:"attention,omitempty"`
}

type studioFlowSnapshot struct {
	ID            string   `json:"id"`
	Label         string   `json:"label"`
	Channel       string   `json:"channel,omitempty"`
	Owner         string   `json:"owner,omitempty"`
	Status        string   `json:"status,omitempty"`
	ExecutionMode string   `json:"execution_mode,omitempty"`
	WorkflowKey   string   `json:"workflow_key,omitempty"`
	PipelineID    string   `json:"pipeline_id,omitempty"`
	TaskCount     int      `json:"task_count"`
	BlockedCount  int      `json:"blocked_count,omitempty"`
	Workspace     string   `json:"workspace,omitempty"`
	TaskIDs       []string `json:"task_ids,omitempty"`
}

type studioWorkspaceSnapshot struct {
	Path         string           `json:"path"`
	WorktreePath string           `json:"worktree_path,omitempty"`
	Branch       string           `json:"branch,omitempty"`
	Channel      string           `json:"channel,omitempty"`
	Owner        string           `json:"owner,omitempty"`
	Healthy      bool             `json:"healthy"`
	Issue        string           `json:"issue,omitempty"`
	TaskCounts   studioTaskCounts `json:"task_counts"`
	TaskIDs      []string         `json:"task_ids,omitempty"`
}

type studioTaskSnapshot struct {
	ID                     string   `json:"id"`
	Channel                string   `json:"channel,omitempty"`
	Title                  string   `json:"title,omitempty"`
	Owner                  string   `json:"owner,omitempty"`
	Status                 string   `json:"status,omitempty"`
	Blocked                bool     `json:"blocked,omitempty"`
	TaskType               string   `json:"task_type,omitempty"`
	ExecutionMode          string   `json:"execution_mode,omitempty"`
	WorkflowKey            string   `json:"workflow_key,omitempty"`
	PipelineID             string   `json:"pipeline_id,omitempty"`
	WorkspacePath          string   `json:"workspace_path,omitempty"`
	WorktreePath           string   `json:"worktree_path,omitempty"`
	WorktreeBranch         string   `json:"worktree_branch,omitempty"`
	HandoffStatus          string   `json:"handoff_status,omitempty"`
	BlockerRequestIDs      []string `json:"blocker_request_ids,omitempty"`
	BlockingReviewFindings int      `json:"blocking_review_findings,omitempty"`
	ReconciliationPending  bool     `json:"reconciliation_pending,omitempty"`
	IssuePublicationStatus string   `json:"issue_publication_status,omitempty"`
	IssueURL               string   `json:"issue_url,omitempty"`
	PRPublicationStatus    string   `json:"pr_publication_status,omitempty"`
	PRURL                  string   `json:"pr_url,omitempty"`
	DependsOn              []string `json:"depends_on,omitempty"`
	UpdatedAt              string   `json:"updated_at,omitempty"`
	LivenessState          string   `json:"liveness_state,omitempty"`
	LivenessReason         string   `json:"liveness_reason,omitempty"`
	LivenessAt             string   `json:"liveness_at,omitempty"`
}

type studioRequestSnapshot struct {
	ID       string `json:"id"`
	Kind     string `json:"kind,omitempty"`
	Status   string `json:"status,omitempty"`
	Channel  string `json:"channel,omitempty"`
	From     string `json:"from,omitempty"`
	Title    string `json:"title,omitempty"`
	Question string `json:"question,omitempty"`
	Blocking bool   `json:"blocking,omitempty"`
	Required bool   `json:"required,omitempty"`
	ReplyTo  string `json:"reply_to,omitempty"`
}

type studioMessageSnapshot struct {
	ID        string `json:"id"`
	Channel   string `json:"channel,omitempty"`
	From      string `json:"from,omitempty"`
	Title     string `json:"title,omitempty"`
	Content   string `json:"content,omitempty"`
	ReplyTo   string `json:"reply_to,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

type studioBlocker struct {
	ID                string                   `json:"id"`
	Kind              string                   `json:"kind"`
	Severity          string                   `json:"severity"`
	Title             string                   `json:"title"`
	Summary           string                   `json:"summary"`
	Channel           string                   `json:"channel,omitempty"`
	TaskID            string                   `json:"task_id,omitempty"`
	Owner             string                   `json:"owner,omitempty"`
	Reason            string                   `json:"reason"`
	WaitingOn         string                   `json:"waiting_on,omitempty"`
	RecommendedAction string                   `json:"recommended_action,omitempty"`
	AvailableActions  []studioActionInvocation `json:"available_actions,omitempty"`
}

type studioActionDefinition struct {
	Action          string `json:"action"`
	Label           string `json:"label"`
	Description     string `json:"description,omitempty"`
	Mutating        bool   `json:"mutating,omitempty"`
	FrontendHandled bool   `json:"frontend_handled,omitempty"`
	RequiresTaskID  bool   `json:"requires_task_id,omitempty"`
	RequiresChannel bool   `json:"requires_channel,omitempty"`
	RequiresOwner   bool   `json:"requires_owner,omitempty"`
	RequiresAgent   bool   `json:"requires_agent,omitempty"`
}

type studioActionInvocation struct {
	Action          string `json:"action"`
	Label           string `json:"label,omitempty"`
	Description     string `json:"description,omitempty"`
	Mutating        bool   `json:"mutating,omitempty"`
	FrontendHandled bool   `json:"frontend_handled,omitempty"`
	RequiresTaskID  bool   `json:"requires_task_id,omitempty"`
	RequiresChannel bool   `json:"requires_channel,omitempty"`
	RequiresOwner   bool   `json:"requires_owner,omitempty"`
	RequiresAgent   bool   `json:"requires_agent,omitempty"`
	TaskID          string `json:"task_id,omitempty"`
	Channel         string `json:"channel,omitempty"`
	Owner           string `json:"owner,omitempty"`
	Agent           string `json:"agent,omitempty"`
}

type studioDevConsoleActionRequest struct {
	Action  string `json:"action"`
	TaskID  string `json:"task_id,omitempty"`
	Channel string `json:"channel,omitempty"`
	Owner   string `json:"owner,omitempty"`
	Actor   string `json:"actor,omitempty"`
	Agent   string `json:"agent,omitempty"`
}

type studioDevConsoleActionResponse struct {
	OK              bool           `json:"ok"`
	Action          string         `json:"action"`
	TaskID          string         `json:"task_id,omitempty"`
	Channel         string         `json:"channel,omitempty"`
	Message         string         `json:"message,omitempty"`
	FrontendHandled bool           `json:"frontend_handled,omitempty"`
	Task            *teamTask      `json:"task,omitempty"`
	Alert           *watchdogAlert `json:"alert,omitempty"`
}

type studioDevConsoleState struct {
	SessionMode    string
	DirectAgent    string
	FocusMode      bool
	Provider       string
	Members        []officeMember
	Channels       []teamChannel
	Tasks          []teamTask
	Requests       []humanInterview
	Actions        []officeActionLog
	Decisions      []officeDecisionRecord
	Watchdogs      []watchdogAlert
	ExecutionNodes []executionNode
	Messages       []channelMessage
	Activity       []agentActivitySnapshot
	WebUIOrigins   []string
	BrokerReady    bool
}

func (b *Broker) handleStudioDevConsole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(buildStudioDevConsoleSnapshot(b))
}

func (b *Broker) handleStudioDevConsoleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body studioDevConsoleActionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid studio action payload", http.StatusBadRequest)
		return
	}

	action := strings.ToLower(strings.TrimSpace(body.Action))
	switch action {
	case "retry_task":
		resp, err := b.handleStudioRetryTask(body)
		if err != nil {
			writeStudioDevConsoleActionError(w, err)
			return
		}
		writeStudioDevConsoleJSON(w, resp)
	case "retry_issue_publication":
		resp, err := b.handleStudioRetryIssuePublication(body)
		if err != nil {
			writeStudioDevConsoleActionError(w, err)
			return
		}
		writeStudioDevConsoleJSON(w, resp)
	case "retry_pr_publication":
		resp, err := b.handleStudioRetryPRPublication(body)
		if err != nil {
			writeStudioDevConsoleActionError(w, err)
			return
		}
		writeStudioDevConsoleJSON(w, resp)
	case "reassign_task":
		resp, err := b.handleStudioReassignTask(body)
		if err != nil {
			writeStudioDevConsoleActionError(w, err)
			return
		}
		writeStudioDevConsoleJSON(w, resp)
	case "wake_agents":
		resp, err := b.handleStudioWakeAgents(body)
		if err != nil {
			writeStudioDevConsoleActionError(w, err)
			return
		}
		writeStudioDevConsoleJSON(w, resp)
	case "inspect_task", "inspect_channel", "refresh_snapshot", "create_task":
		writeStudioDevConsoleJSON(w, studioDevConsoleActionResponse{
			OK:              true,
			Action:          action,
			TaskID:          strings.TrimSpace(body.TaskID),
			Channel:         normalizeChannelSlug(body.Channel),
			FrontendHandled: true,
			Message:         "frontend-handled action metadata returned",
		})
	default:
		http.Error(w, "unsupported studio action", http.StatusBadRequest)
	}
}

func buildStudioDevConsoleSnapshot(b *Broker) studioDevConsoleResponse {
	state := copyStudioDevConsoleState(b)
	blockers := buildStudioBlockersFromState(state)
	taskCounts := studioTaskCountsFromTasks(state.Tasks)
	health, bootstrap := buildStudioOfficeHealthAndBootstrap(state, blockers, taskCounts)
	env := buildStudioEnvironmentSnapshot(state, blockers, health, bootstrap)
	context := buildStudioActiveContextSnapshot(state, blockers)
	officeStatus := "ok"
	switch {
	case health.Degraded:
		officeStatus = "degraded"
	case !bootstrap.Ready:
		officeStatus = "initializing"
	}
	return studioDevConsoleResponse{
		Office: studioOfficeSnapshot{
			Status:        officeStatus,
			Provider:      state.Provider,
			FocusMode:     state.FocusMode,
			SessionMode:   state.SessionMode,
			MemoryBackend: env.MemoryBackendActive,
			Health:        health,
			Bootstrap:     bootstrap,
			TaskCounts:    taskCounts,
		},
		Environment:   env,
		ActiveContext: context,
		Blockers:      blockers,
		Actions:       studioDevConsoleActions(),
	}
}

func copyStudioDevConsoleState(b *Broker) studioDevConsoleState {
	b.mu.Lock()
	defer b.mu.Unlock()
	provider := strings.TrimSpace(b.runtimeProvider)
	if provider == "" {
		provider = config.ResolveLLMProvider("")
	}
	return studioDevConsoleState{
		SessionMode:    b.sessionMode,
		DirectAgent:    b.oneOnOneAgent,
		FocusMode:      b.focusMode,
		Provider:       provider,
		Members:        append([]officeMember(nil), b.members...),
		Channels:       append([]teamChannel(nil), b.channels...),
		Tasks:          append([]teamTask(nil), b.tasks...),
		Requests:       append([]humanInterview(nil), b.requests...),
		Actions:        append([]officeActionLog(nil), b.actions...),
		Decisions:      append([]officeDecisionRecord(nil), b.decisions...),
		Watchdogs:      append([]watchdogAlert(nil), b.watchdogs...),
		ExecutionNodes: append([]executionNode(nil), b.executionNodes...),
		Messages:       append([]channelMessage(nil), b.messages...),
		Activity:       studioActivitySnapshotsFromMap(b.activity),
		WebUIOrigins:   append([]string(nil), b.webUIOrigins...),
		BrokerReady:    true,
	}
}

func studioActivitySnapshotsFromMap(activity map[string]agentActivitySnapshot) []agentActivitySnapshot {
	out := make([]agentActivitySnapshot, 0, len(activity))
	for _, snapshot := range activity {
		out = append(out, snapshot)
	}
	return out
}

func buildStudioOfficeHealthAndBootstrap(state studioDevConsoleState, blockers []studioBlocker, counts studioTaskCounts) (studioBrokerHealthSnapshot, studioBootstrapSnapshot) {
	signals := studioDegradationSignals(state, blockers, counts)
	health := studioBrokerHealthSnapshot{
		BrokerReachable: state.BrokerReady,
		APIReachable:    state.BrokerReady,
		WebReachable:    len(state.WebUIOrigins) > 0,
		Degraded:        len(signals) > 0,
		Signals:         append([]string(nil), signals...),
		Build:           buildinfo.Current(),
	}
	bootstrap := studioBootstrapSnapshot{
		Members:    len(state.Members),
		Channels:   len(state.Channels),
		Tasks:      counts.Total,
		Requests:   len(state.Requests),
		Workspaces: len(studioWorkspaceSnapshotsFromTasks(state.Tasks)),
		Workflows:  len(studioFlowSnapshotsFromTasks(state.Tasks)),
	}
	bootstrap.Ready = bootstrap.Members > 0 && bootstrap.Channels > 0 && bootstrap.Tasks > 0 && !health.Degraded
	bootstrap.Summary = studioBootstrapSummary(bootstrap, health)
	return health, bootstrap
}

func buildStudioEnvironmentSnapshot(state studioDevConsoleState, blockers []studioBlocker, health studioBrokerHealthSnapshot, bootstrap studioBootstrapSnapshot) studioEnvironmentSnapshot {
	memoryStatus := ResolveMemoryBackendStatus()
	signals := append([]string(nil), health.Signals...)
	if memoryStatus.ActiveKind == config.MemoryBackendNone {
		signals = append(signals, "memory_backend_unavailable")
	}
	if len(state.WebUIOrigins) == 0 {
		signals = append(signals, "web_ui_uninitialized")
	}
	env := studioEnvironmentSnapshot{
		Status:                "ok",
		BrokerReachable:       state.BrokerReady,
		APIReachable:          state.BrokerReady,
		WebReachable:          len(state.WebUIOrigins) > 0,
		MemoryBackendSelected: memoryStatus.SelectedKind,
		MemoryBackendActive:   memoryStatus.ActiveKind,
		MemoryBackendReady:    memoryStatus.ActiveKind != config.MemoryBackendNone,
		Signals:               uniqueStudioStrings(signals),
		Build:                 buildinfo.Current(),
	}
	env.Degraded = len(env.Signals) > 0
	if !bootstrap.Ready {
		env.Degraded = true
	}
	if env.Degraded {
		env.Status = "degraded"
	}
	return env
}

func buildStudioActiveContextSnapshot(state studioDevConsoleState, blockers []studioBlocker) studioActiveContextSnapshot {
	tasks := studioTaskSnapshotsFromTasks(state.Tasks, state.Activity...)
	requests := studioRequestSnapshotsFromRequests(state.Requests)
	messages := studioRecentMessagesFromState(state.Messages, 5)
	channels := studioChannelSnapshotsFromState(state, tasks, requests, blockers)
	flows := studioFlowSnapshotsFromTasks(state.Tasks)
	workspaces := studioWorkspaceSnapshotsFromTasks(state.Tasks)
	sort.Slice(channels, func(i, j int) bool { return channels[i].Slug < channels[j].Slug })
	sort.Slice(flows, func(i, j int) bool {
		if flows[i].BlockedCount != flows[j].BlockedCount {
			return flows[i].BlockedCount > flows[j].BlockedCount
		}
		if flows[i].TaskCount != flows[j].TaskCount {
			return flows[i].TaskCount > flows[j].TaskCount
		}
		return flows[i].Label < flows[j].Label
	})
	sort.Slice(workspaces, func(i, j int) bool { return workspaces[i].Path < workspaces[j].Path })

	focus, nextSteps, primaryChannel := studioFocusAndNextSteps(state, blockers, channels, flows, workspaces)
	return studioActiveContextSnapshot{
		SessionMode:    state.SessionMode,
		DirectAgent:    state.DirectAgent,
		Focus:          focus,
		NextSteps:      nextSteps,
		PrimaryChannel: primaryChannel,
		Channels:       channels,
		Flows:          flows,
		Workspaces:     workspaces,
		Tasks:          tasks,
		Requests:       requests,
		Messages:       messages,
	}
}

func studioFocusAndNextSteps(state studioDevConsoleState, blockers []studioBlocker, channels []studioChannelSnapshot, flows []studioFlowSnapshot, workspaces []studioWorkspaceSnapshot) (string, []string, string) {
	var focus string
	nextSteps := make([]string, 0, 3)
	if req := studioFirstBlockingRequest(state.Requests); req != nil {
		focus = studioRequestTitle(*req)
		nextSteps = append(nextSteps, "Answer the blocking request in #"+normalizeChannelSlug(req.Channel)+".")
	}
	if focus == "" {
		if blocker := studioFirstBlocker(blockers); blocker != nil {
			focus = blocker.Title
			if blocker.RecommendedAction != "" {
				nextSteps = append(nextSteps, "Use "+blocker.RecommendedAction+" for "+studioBlockerTargetLabel(*blocker)+".")
			}
		}
	}
	if focus == "" {
		if task := studioFirstActiveTask(state.Tasks); task != nil {
			focus = studioTaskTitle(*task)
			if strings.TrimSpace(task.WorktreePath) != "" || strings.TrimSpace(task.WorkspacePath) != "" {
				nextSteps = append(nextSteps, "Work in the assigned workspace for "+studioTaskTitle(*task)+".")
			}
		}
	}
	if focus == "" {
		if len(flows) > 0 {
			focus = flows[0].Label
		}
	}
	if focus == "" {
		focus = "No active work detected."
	}
	if len(channels) > 0 {
		nextSteps = append(nextSteps, "Review #"+channels[0].Slug+" for the latest task and request state.")
	}
	if len(workspaces) > 0 {
		if workspaces[0].Issue != "" {
			nextSteps = append(nextSteps, workspaces[0].Issue)
		}
	}
	if len(nextSteps) == 0 {
		nextSteps = append(nextSteps, "Use refresh_snapshot to pull a fresh dev-console view.")
	}
	primaryChannel := ""
	if len(channels) > 0 {
		primaryChannel = channels[0].Slug
	}
	return focus, uniqueStudioStrings(nextSteps), primaryChannel
}

func buildStudioBlockers(b *Broker) []studioBlocker {
	if b == nil {
		return nil
	}
	return buildStudioBlockersFromState(copyStudioDevConsoleState(b))
}

func buildStudioBlockersFromState(state studioDevConsoleState) []studioBlocker {
	taskByID := make(map[string]teamTask, len(state.Tasks))
	channelBySlug := make(map[string]teamChannel, len(state.Channels))
	for _, task := range state.Tasks {
		taskByID[strings.TrimSpace(task.ID)] = task
	}
	for _, ch := range state.Channels {
		channelBySlug[normalizeChannelSlug(ch.Slug)] = ch
	}

	blockers := make([]studioBlocker, 0, 8)
	for _, task := range state.Tasks {
		if taskIsTerminal(&task) {
			continue
		}
		if blocker := studioTimeoutBlocker(task); blocker != nil {
			blockers = append(blockers, *blocker)
		}
		if blocker := studioReconciliationBlocker(task); blocker != nil {
			blockers = append(blockers, *blocker)
		}
		if blocker := studioGitHubIssuePublicationBlocker(task); blocker != nil {
			blockers = append(blockers, *blocker)
		}
		if blocker := studioGitHubPRPublicationBlocker(task); blocker != nil {
			blockers = append(blockers, *blocker)
		}
		if blocker := studioDependencyBlocker(task, taskByID); blocker != nil {
			blockers = append(blockers, *blocker)
		}
		if blocker := studioOwnerNotInChannelBlocker(task, channelBySlug); blocker != nil {
			blockers = append(blockers, *blocker)
		}
		if blocker := studioWorkflowMissingInputBlocker(task); blocker != nil {
			blockers = append(blockers, *blocker)
		}
		if blocker := studioDegradedLocalEnvironmentBlocker(task); blocker != nil {
			blockers = append(blockers, *blocker)
		}
	}
	for _, req := range state.Requests {
		if !requestIsActive(req) {
			continue
		}
		if blocker := studioDirectionWithoutTaskBlocker(req, state.Tasks); blocker != nil {
			blockers = append(blockers, *blocker)
		}
	}
	return uniqueStudioBlockers(blockers)
}

func studioTimeoutBlocker(task teamTask) *studioBlocker {
	if !studioTaskHasTimeoutSignal(task) {
		return nil
	}
	channel := normalizeChannelSlug(task.Channel)
	return &studioBlocker{
		ID:                "task:" + strings.TrimSpace(task.ID),
		Kind:              "task_timeout_no_substantive_update",
		Severity:          "high",
		Title:             studioTaskTitle(task),
		Summary:           "Task blocked after timeout before a substantive update.",
		Channel:           channel,
		TaskID:            strings.TrimSpace(task.ID),
		Owner:             strings.TrimSpace(task.Owner),
		Reason:            studioTimeoutReason(task),
		WaitingOn:         strings.TrimSpace(task.Owner),
		RecommendedAction: "retry_task",
		AvailableActions:  studioActionsForBlocker("task_timeout_no_substantive_update", task, channel, task.Owner, true),
	}
}

func studioDependencyBlocker(task teamTask, taskByID map[string]teamTask) *studioBlocker {
	if !studioTaskHasUnresolvedDependency(task, taskByID) {
		return nil
	}
	channel := normalizeChannelSlug(task.Channel)
	return &studioBlocker{
		ID:                "task:" + strings.TrimSpace(task.ID) + ":deps",
		Kind:              "task_blocked_by_dependency",
		Severity:          "high",
		Title:             studioTaskTitle(task),
		Summary:           "Task is blocked by unresolved dependencies.",
		Channel:           channel,
		TaskID:            strings.TrimSpace(task.ID),
		Owner:             strings.TrimSpace(task.Owner),
		Reason:            studioTaskDependencyReason(task),
		WaitingOn:         strings.Join(studioTaskPendingDependencies(task, taskByID), ", "),
		RecommendedAction: "inspect_task",
		AvailableActions:  studioActionsForBlocker("task_blocked_by_dependency", task, channel, task.Owner, false),
	}
}

func studioReconciliationBlocker(task teamTask) *studioBlocker {
	if !taskHasPendingReconciliation(&task) {
		return nil
	}
	channel := normalizeChannelSlug(task.Channel)
	reason := "Task has pending reconciliation."
	waitingOn := strings.TrimSpace(task.Owner)
	if task.Reconciliation != nil {
		if strings.TrimSpace(task.Reconciliation.Reason) != "" {
			reason = strings.TrimSpace(task.Reconciliation.Reason)
		}
		if waitingOn == "" && strings.TrimSpace(task.Reconciliation.WorkspacePath) != "" {
			waitingOn = strings.TrimSpace(task.Reconciliation.WorkspacePath)
		}
	}
	return &studioBlocker{
		ID:                "task:" + strings.TrimSpace(task.ID) + ":reconciliation",
		Kind:              "task_reconciliation_pending",
		Severity:          "high",
		Title:             studioTaskTitle(task),
		Summary:           "Task is blocked on reconciliation before state can advance.",
		Channel:           channel,
		TaskID:            strings.TrimSpace(task.ID),
		Owner:             strings.TrimSpace(task.Owner),
		Reason:            reason,
		WaitingOn:         waitingOn,
		RecommendedAction: "inspect_task",
		AvailableActions:  studioActionsForBlocker("task_reconciliation_pending", task, channel, task.Owner, false),
	}
}

func studioGitHubIssuePublicationBlocker(task teamTask) *studioBlocker {
	if strings.TrimSpace(publicationStatus(task.IssuePublication)) != "deferred" {
		return nil
	}
	channel := normalizeChannelSlug(task.Channel)
	return &studioBlocker{
		ID:                "task:" + strings.TrimSpace(task.ID) + ":github-issue",
		Kind:              "github_issue_publication_deferred",
		Severity:          "medium",
		Title:             studioTaskTitle(task),
		Summary:           "GitHub issue publication is deferred.",
		Channel:           channel,
		TaskID:            strings.TrimSpace(task.ID),
		Owner:             strings.TrimSpace(task.Owner),
		Reason:            strings.TrimSpace(task.IssuePublication.LastError),
		WaitingOn:         "gh auth or repo connectivity",
		RecommendedAction: "retry_issue_publication",
		AvailableActions:  studioActionsForBlocker("github_issue_publication_deferred", task, channel, task.Owner, false),
	}
}

func studioGitHubPRPublicationBlocker(task teamTask) *studioBlocker {
	if strings.TrimSpace(publicationStatus(task.PRPublication)) != "deferred" {
		return nil
	}
	channel := normalizeChannelSlug(task.Channel)
	return &studioBlocker{
		ID:                "task:" + strings.TrimSpace(task.ID) + ":github-pr",
		Kind:              "github_pr_publication_deferred",
		Severity:          "medium",
		Title:             studioTaskTitle(task),
		Summary:           "GitHub PR publication is deferred.",
		Channel:           channel,
		TaskID:            strings.TrimSpace(task.ID),
		Owner:             strings.TrimSpace(task.Owner),
		Reason:            strings.TrimSpace(task.PRPublication.LastError),
		WaitingOn:         "gh auth, push, or repo connectivity",
		RecommendedAction: "retry_pr_publication",
		AvailableActions:  studioActionsForBlocker("github_pr_publication_deferred", task, channel, task.Owner, false),
	}
}

func studioOwnerNotInChannelBlocker(task teamTask, channelBySlug map[string]teamChannel) *studioBlocker {
	if !studioTaskOwnerMissingFromChannel(task, channelBySlug) {
		return nil
	}
	channel := normalizeChannelSlug(task.Channel)
	return &studioBlocker{
		ID:                "task:" + strings.TrimSpace(task.ID) + ":owner",
		Kind:              "owner_not_in_channel",
		Severity:          "medium",
		Title:             studioTaskTitle(task),
		Summary:           "Task owner is not a channel member.",
		Channel:           channel,
		TaskID:            strings.TrimSpace(task.ID),
		Owner:             strings.TrimSpace(task.Owner),
		Reason:            fmt.Sprintf("@%s is not a member of #%s.", strings.TrimSpace(task.Owner), channel),
		WaitingOn:         strings.TrimSpace(task.Owner),
		RecommendedAction: "reassign_task",
		AvailableActions:  studioActionsForBlocker("owner_not_in_channel", task, channel, task.Owner, false),
	}
}

func studioDirectionWithoutTaskBlocker(req humanInterview, tasks []teamTask) *studioBlocker {
	if !studioRequestLooksLikeDirection(req) {
		return nil
	}
	channel := normalizeChannelSlug(req.Channel)
	if studioHasChannelTask(tasks, channel) {
		return nil
	}
	return &studioBlocker{
		ID:                "request:" + strings.TrimSpace(req.ID),
		Kind:              "direction_without_task",
		Severity:          "medium",
		Title:             studioRequestTitle(req),
		Summary:           "Direction was requested but no corresponding task exists.",
		Channel:           channel,
		Reason:            studioRequestDirectionReason(req),
		WaitingOn:         strings.TrimSpace(req.From),
		RecommendedAction: "create_task",
		AvailableActions:  studioActionsForBlocker("direction_without_task", teamTask{Channel: channel, Title: studioRequestTitle(req)}, channel, "", false),
	}
}

func studioWorkflowMissingInputBlocker(task teamTask) *studioBlocker {
	if !studioTaskLooksLikeWorkflowMissingInput(task) {
		return nil
	}
	channel := normalizeChannelSlug(task.Channel)
	return &studioBlocker{
		ID:                "task:" + strings.TrimSpace(task.ID) + ":workflow",
		Kind:              "workflow_missing_required_input",
		Severity:          "high",
		Title:             studioTaskTitle(task),
		Summary:           "Workflow is missing required input.",
		Channel:           channel,
		TaskID:            strings.TrimSpace(task.ID),
		Owner:             strings.TrimSpace(task.Owner),
		Reason:            studioTaskWorkflowReason(task),
		WaitingOn:         studioWorkflowMissingInputWaitingOn(task),
		RecommendedAction: "inspect_task",
		AvailableActions:  studioActionsForBlocker("workflow_missing_required_input", task, channel, task.Owner, false),
	}
}

func studioDegradedLocalEnvironmentBlocker(task teamTask) *studioBlocker {
	if !studioTaskLooksLikeDegradedLocalEnvironment(task) {
		return nil
	}
	channel := normalizeChannelSlug(task.Channel)
	return &studioBlocker{
		ID:                "task:" + strings.TrimSpace(task.ID) + ":environment",
		Kind:              "degraded_local_environment",
		Severity:          "high",
		Title:             studioTaskTitle(task),
		Summary:           "Local workspace or worktree is degraded.",
		Channel:           channel,
		TaskID:            strings.TrimSpace(task.ID),
		Owner:             strings.TrimSpace(task.Owner),
		Reason:            studioEnvironmentReason(task),
		WaitingOn:         studioEnvironmentWaitingOn(task),
		RecommendedAction: "inspect_task",
		AvailableActions:  studioActionsForBlocker("degraded_local_environment", task, channel, task.Owner, false),
	}
}

func studioActionsForBlocker(kind string, task teamTask, channel, owner string, includeWake bool) []studioActionInvocation {
	base := []studioActionInvocation{
		{
			Action:          "inspect_task",
			Label:           "Inspect task",
			Description:     "Open the task details in the console.",
			FrontendHandled: true,
			RequiresTaskID:  true,
			TaskID:          strings.TrimSpace(task.ID),
			Channel:         channel,
		},
	}
	switch kind {
	case "task_timeout_no_substantive_update":
		base = append(base,
			studioActionInvocation{
				Action:         "retry_task",
				Label:          "Retry task",
				Description:    "Resume the blocked task.",
				Mutating:       true,
				RequiresTaskID: true,
				TaskID:         strings.TrimSpace(task.ID),
				Channel:        channel,
			},
			studioActionInvocation{
				Action:         "reassign_task",
				Label:          "Reassign task",
				Description:    "Change the owner before retrying.",
				Mutating:       true,
				RequiresTaskID: true,
				RequiresOwner:  true,
				TaskID:         strings.TrimSpace(task.ID),
				Channel:        channel,
			},
		)
		if includeWake {
			base = append(base, studioActionInvocation{
				Action:         "wake_agents",
				Label:          "Wake agents",
				Description:    "Send an explicit watcher nudge for this task.",
				Mutating:       true,
				RequiresTaskID: true,
				TaskID:         strings.TrimSpace(task.ID),
				Channel:        channel,
				Agent:          strings.TrimSpace(owner),
			})
		}
	case "github_issue_publication_deferred":
		base = append(base, studioActionInvocation{
			Action:         "retry_issue_publication",
			Label:          "Retry issue publication",
			Description:    "Try opening the GitHub issue again.",
			Mutating:       true,
			RequiresTaskID: true,
			TaskID:         strings.TrimSpace(task.ID),
			Channel:        channel,
		})
	case "github_pr_publication_deferred":
		base = append(base, studioActionInvocation{
			Action:         "retry_pr_publication",
			Label:          "Retry PR publication",
			Description:    "Try opening the GitHub PR again.",
			Mutating:       true,
			RequiresTaskID: true,
			TaskID:         strings.TrimSpace(task.ID),
			Channel:        channel,
		})
	case "task_blocked_by_dependency", "degraded_local_environment":
		base = append(base,
			studioActionInvocation{
				Action:         "reassign_task",
				Label:          "Reassign task",
				Description:    "Move the task to a different owner.",
				Mutating:       true,
				RequiresTaskID: true,
				RequiresOwner:  true,
				TaskID:         strings.TrimSpace(task.ID),
				Channel:        channel,
			},
			studioActionInvocation{
				Action:          "refresh_snapshot",
				Label:           "Refresh snapshot",
				Description:     "Refresh the console view after the environment or dependency changes.",
				FrontendHandled: true,
			},
		)
	case "owner_not_in_channel":
		base = append(base,
			studioActionInvocation{
				Action:         "reassign_task",
				Label:          "Reassign task",
				Description:    "Move the task into a channel where the owner is present.",
				Mutating:       true,
				RequiresTaskID: true,
				RequiresOwner:  true,
				TaskID:         strings.TrimSpace(task.ID),
				Channel:        channel,
			},
			studioActionInvocation{
				Action:          "inspect_channel",
				Label:           "Inspect channel",
				Description:     "Open the channel membership view.",
				FrontendHandled: true,
				RequiresChannel: true,
				Channel:         channel,
			},
		)
	case "direction_without_task":
		base = append(base,
			studioActionInvocation{
				Action:          "create_task",
				Label:           "Create task",
				Description:     "Create a task for the direction that was requested.",
				FrontendHandled: true,
				RequiresChannel: true,
				Channel:         channel,
			},
			studioActionInvocation{
				Action:          "inspect_channel",
				Label:           "Inspect channel",
				Description:     "Review the channel before creating follow-up work.",
				FrontendHandled: true,
				RequiresChannel: true,
				Channel:         channel,
			},
		)
	case "workflow_missing_required_input":
		base = append(base,
			studioActionInvocation{
				Action:          "refresh_snapshot",
				Label:           "Refresh snapshot",
				Description:     "Re-check the workflow after the required input is supplied.",
				FrontendHandled: true,
			},
		)
	}
	return uniqueStudioActionInvocations(base)
}

func studioDevConsoleActions() []studioActionDefinition {
	return []studioActionDefinition{
		{
			Action:         "retry_task",
			Label:          "Retry task",
			Description:    "Resume a blocked task after the underlying issue is addressed.",
			Mutating:       true,
			RequiresTaskID: true,
		},
		{
			Action:         "retry_issue_publication",
			Label:          "Retry issue publication",
			Description:    "Retry opening the GitHub issue for a task.",
			Mutating:       true,
			RequiresTaskID: true,
		},
		{
			Action:         "retry_pr_publication",
			Label:          "Retry PR publication",
			Description:    "Retry opening the GitHub PR for a task.",
			Mutating:       true,
			RequiresTaskID: true,
		},
		{
			Action:         "reassign_task",
			Label:          "Reassign task",
			Description:    "Move a task to another owner and notify the channel.",
			Mutating:       true,
			RequiresTaskID: true,
			RequiresOwner:  true,
		},
		{
			Action:         "wake_agents",
			Label:          "Wake agents",
			Description:    "Send an explicit watchdog-style nudge for a specific task.",
			Mutating:       true,
			RequiresTaskID: true,
		},
		{
			Action:          "inspect_task",
			Label:           "Inspect task",
			Description:     "Frontend-handled task drill-in.",
			FrontendHandled: true,
			RequiresTaskID:  true,
		},
		{
			Action:          "inspect_channel",
			Label:           "Inspect channel",
			Description:     "Frontend-handled channel drill-in.",
			FrontendHandled: true,
			RequiresChannel: true,
		},
		{
			Action:          "create_task",
			Label:           "Create task",
			Description:     "Frontend-handled task creation from the console.",
			FrontendHandled: true,
			RequiresChannel: true,
		},
		{
			Action:          "refresh_snapshot",
			Label:           "Refresh snapshot",
			Description:     "Refresh the dev-console data.",
			FrontendHandled: true,
		},
	}
}

func (b *Broker) handleStudioRetryTask(body studioDevConsoleActionRequest) (studioDevConsoleActionResponse, error) {
	task, err := b.findStudioTaskForAction(body.TaskID, body.Channel)
	if err != nil {
		return studioDevConsoleActionResponse{}, err
	}
	if !task.Blocked && !strings.EqualFold(strings.TrimSpace(task.Status), "blocked") {
		return studioDevConsoleActionResponse{}, fmt.Errorf("task %q is not blocked", strings.TrimSpace(task.ID))
	}
	actor := strings.TrimSpace(body.Actor)
	if actor == "" {
		actor = "studio"
	}
	updated, changed, err := b.ResumeTask(task.ID, actor, "Studio dev console retry requested by @"+actor+".")
	if err != nil {
		return studioDevConsoleActionResponse{}, err
	}
	if !changed {
		return studioDevConsoleActionResponse{}, fmt.Errorf("task %q did not change state", strings.TrimSpace(task.ID))
	}
	return studioDevConsoleActionResponse{
		OK:      true,
		Action:  "retry_task",
		TaskID:  updated.ID,
		Channel: normalizeChannelSlug(updated.Channel),
		Message: "task retried",
		Task:    &updated,
	}, nil
}

func (b *Broker) handleStudioRetryIssuePublication(body studioDevConsoleActionRequest) (studioDevConsoleActionResponse, error) {
	task, err := b.findStudioTaskForAction(body.TaskID, body.Channel)
	if err != nil {
		return studioDevConsoleActionResponse{}, err
	}
	actor := strings.TrimSpace(body.Actor)
	if actor == "" {
		actor = "studio"
	}
	updated, changed, err := b.RetryTaskIssuePublication(task.ID, actor)
	if err != nil {
		return studioDevConsoleActionResponse{}, err
	}
	if changed {
		b.publishTaskIssueSoon(task.ID)
	}
	return studioDevConsoleActionResponse{
		OK:      true,
		Action:  "retry_issue_publication",
		TaskID:  updated.ID,
		Channel: normalizeChannelSlug(updated.Channel),
		Message: "issue publication queued",
		Task:    &updated,
	}, nil
}

func (b *Broker) handleStudioRetryPRPublication(body studioDevConsoleActionRequest) (studioDevConsoleActionResponse, error) {
	task, err := b.findStudioTaskForAction(body.TaskID, body.Channel)
	if err != nil {
		return studioDevConsoleActionResponse{}, err
	}
	actor := strings.TrimSpace(body.Actor)
	if actor == "" {
		actor = "studio"
	}
	updated, changed, err := b.RetryTaskPRPublication(task.ID, actor)
	if err != nil {
		return studioDevConsoleActionResponse{}, err
	}
	if changed {
		b.publishTaskPRSoon(task.ID)
	}
	return studioDevConsoleActionResponse{
		OK:      true,
		Action:  "retry_pr_publication",
		TaskID:  updated.ID,
		Channel: normalizeChannelSlug(updated.Channel),
		Message: "pr publication queued",
		Task:    &updated,
	}, nil
}

func (b *Broker) handleStudioReassignTask(body studioDevConsoleActionRequest) (studioDevConsoleActionResponse, error) {
	if strings.TrimSpace(body.Owner) == "" {
		return studioDevConsoleActionResponse{}, fmt.Errorf("owner required")
	}
	task, err := b.findStudioTaskForAction(body.TaskID, body.Channel)
	if err != nil {
		return studioDevConsoleActionResponse{}, err
	}
	actor := strings.TrimSpace(body.Actor)
	if actor == "" {
		actor = "studio"
	}
	updated, err := b.reassignStudioTask(task.ID, strings.TrimSpace(body.Owner), actor, "Studio dev console reassign requested by @"+actor+".")
	if err != nil {
		return studioDevConsoleActionResponse{}, err
	}
	return studioDevConsoleActionResponse{
		OK:      true,
		Action:  "reassign_task",
		TaskID:  updated.ID,
		Channel: normalizeChannelSlug(updated.Channel),
		Message: "task reassigned",
		Task:    &updated,
	}, nil
}

func (b *Broker) handleStudioWakeAgents(body studioDevConsoleActionRequest) (studioDevConsoleActionResponse, error) {
	taskID := strings.TrimSpace(body.TaskID)
	if taskID == "" {
		return studioDevConsoleActionResponse{}, fmt.Errorf("task_id required for wake_agents")
	}
	task, err := b.findStudioTaskForAction(taskID, body.Channel)
	if err != nil {
		return studioDevConsoleActionResponse{}, err
	}
	channel := normalizeChannelSlug(body.Channel)
	if channel != "" && normalizeChannelSlug(task.Channel) != channel {
		return studioDevConsoleActionResponse{}, fmt.Errorf("task %q is not in #%s", task.ID, channel)
	}
	if channel == "" {
		channel = normalizeChannelSlug(task.Channel)
	}
	if channel == "" {
		channel = "general"
	}
	actor := strings.TrimSpace(body.Actor)
	if actor == "" {
		actor = "studio"
	}
	summary := fmt.Sprintf("Studio wake_agents requested for %s in #%s.", studioTaskTitle(task), channel)
	if owner := strings.TrimSpace(task.Owner); owner != "" {
		summary = fmt.Sprintf("Studio wake_agents requested for @%s on %s in #%s.", owner, studioTaskTitle(task), channel)
	}
	alert, _, err := b.CreateWatchdogAlert("studio_wake_agents", channel, "task", task.ID, task.Owner, summary)
	if err != nil {
		return studioDevConsoleActionResponse{}, err
	}
	if err := b.RecordAction("watchdog_alert", "studio", channel, actor, truncateSummary(summary, 140), task.ID, nil, ""); err != nil {
		return studioDevConsoleActionResponse{}, err
	}
	return studioDevConsoleActionResponse{
		OK:      true,
		Action:  "wake_agents",
		TaskID:  task.ID,
		Channel: channel,
		Message: "agents nudged",
		Task:    &task,
		Alert:   &alert,
	}, nil
}

func (b *Broker) reassignStudioTask(taskID, owner, actor, reason string) (teamTask, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.tasks {
		if strings.TrimSpace(b.tasks[i].ID) != strings.TrimSpace(taskID) {
			continue
		}
		task := &b.tasks[i]
		previousStatus := task.Status
		prevOwner := strings.TrimSpace(task.Owner)
		task.Owner = strings.TrimSpace(owner)
		status := strings.ToLower(strings.TrimSpace(task.Status))
		if status != "done" && status != "review" {
			task.Status = "in_progress"
		}
		if taskNeedsStructuredReview(task) && strings.TrimSpace(task.ReviewState) == "" {
			task.ReviewState = "pending_review"
		}
		if strings.TrimSpace(reason) != "" {
			if strings.TrimSpace(task.Details) == "" {
				task.Details = strings.TrimSpace(reason)
			} else if !strings.Contains(task.Details, reason) {
				task.Details = strings.TrimSpace(task.Details) + "\n\n" + strings.TrimSpace(reason)
			}
		}
		b.ensureTaskOwnerChannelMembershipLocked(task.Channel, task.Owner)
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := rejectTheaterTaskForLiveBusiness(task); err != nil {
			return teamTask{}, err
		}
		if err := b.syncTaskWorktreeLocked(task); err != nil {
			return teamTask{}, err
		}
		issueQueued, prQueued := b.maybeQueueTaskGitHubPublicationLocked(task, previousStatus, "reassign", nil)
		b.appendActionLocked("task_updated", "office", normalizeChannelSlug(task.Channel), actor, truncateSummary(task.Title+" ["+task.Status+"]", 140), task.ID)
		if prevOwner != task.Owner {
			b.postTaskReassignNotificationsLocked(actor, task, prevOwner)
		}
		if err := b.saveLocked(); err != nil {
			return teamTask{}, err
		}
		if issueQueued {
			b.publishTaskIssueSoon(task.ID)
		}
		if prQueued {
			b.publishTaskPRSoon(task.ID)
		}
		return *task, nil
	}
	return teamTask{}, fmt.Errorf("task %q not found", taskID)
}

func (b *Broker) findStudioTaskForAction(taskID, channel string) (teamTask, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return teamTask{}, fmt.Errorf("task_id required")
	}
	channel = normalizeChannelSlug(channel)
	if channel != "" {
		task, ok := b.FindTask(channel, taskID)
		if !ok {
			return teamTask{}, fmt.Errorf("task %q not found", taskID)
		}
		return task, nil
	}
	if task, ok := b.TaskByID(taskID); ok {
		return task, nil
	}
	return teamTask{}, fmt.Errorf("task %q not found", taskID)
}

func studioTaskCountsFromTasks(tasks []teamTask) studioTaskCounts {
	counts := studioTaskCounts{Total: len(tasks)}
	for _, task := range tasks {
		switch strings.ToLower(strings.TrimSpace(task.Status)) {
		case "", "open":
			counts.Open++
		case "in_progress":
			counts.InProgress++
		case "blocked":
			counts.Blocked++
		case "review", "in_review":
			counts.Review++
		case "done":
			counts.Done++
		case "canceled", "cancelled":
			counts.Canceled++
		default:
			counts.Other++
		}
	}
	return counts
}

func studioTaskSnapshotsFromTasks(tasks []teamTask, activity ...agentActivitySnapshot) []studioTaskSnapshot {
	livenessByTaskID := studioLatestLivenessByTaskID(activity)
	out := make([]studioTaskSnapshot, 0, len(tasks))
	for _, task := range tasks {
		liveness := livenessByTaskID[strings.TrimSpace(task.ID)]
		out = append(out, studioTaskSnapshot{
			ID:                     strings.TrimSpace(task.ID),
			Channel:                normalizeChannelSlug(task.Channel),
			Title:                  strings.TrimSpace(task.Title),
			Owner:                  strings.TrimSpace(task.Owner),
			Status:                 strings.TrimSpace(task.Status),
			Blocked:                task.Blocked,
			TaskType:               strings.TrimSpace(task.TaskType),
			ExecutionMode:          strings.TrimSpace(task.ExecutionMode),
			WorkflowKey:            strings.TrimSpace(task.ExecutionKey),
			PipelineID:             strings.TrimSpace(task.PipelineID),
			WorkspacePath:          strings.TrimSpace(task.WorkspacePath),
			WorktreePath:           strings.TrimSpace(task.WorktreePath),
			WorktreeBranch:         strings.TrimSpace(task.WorktreeBranch),
			HandoffStatus:          strings.TrimSpace(task.HandoffStatus),
			BlockerRequestIDs:      append([]string(nil), task.BlockerRequestIDs...),
			BlockingReviewFindings: countBlockingReviewFindings(task.ReviewFindings),
			ReconciliationPending:  taskHasPendingReconciliation(&task),
			IssuePublicationStatus: publicationStatus(task.IssuePublication),
			IssueURL:               publicationURL(task.IssuePublication),
			PRPublicationStatus:    publicationStatus(task.PRPublication),
			PRURL:                  publicationURL(task.PRPublication),
			DependsOn:              append([]string(nil), task.DependsOn...),
			UpdatedAt:              strings.TrimSpace(task.UpdatedAt),
			LivenessState:          strings.TrimSpace(liveness.LivenessState),
			LivenessReason:         strings.TrimSpace(liveness.LivenessReason),
			LivenessAt:             strings.TrimSpace(liveness.LivenessAt),
		})
	}
	return out
}

func studioLatestLivenessByTaskID(activity []agentActivitySnapshot) map[string]agentActivitySnapshot {
	out := make(map[string]agentActivitySnapshot)
	for _, snapshot := range activity {
		taskID := strings.TrimSpace(snapshot.LivenessTaskID)
		if taskID == "" || strings.TrimSpace(snapshot.LivenessState) == "" {
			continue
		}
		if prior, ok := out[taskID]; ok && strings.TrimSpace(prior.LivenessAt) >= strings.TrimSpace(snapshot.LivenessAt) {
			continue
		}
		out[taskID] = snapshot
	}
	return out
}

func studioRequestSnapshotsFromRequests(requests []humanInterview) []studioRequestSnapshot {
	out := make([]studioRequestSnapshot, 0, len(requests))
	for _, req := range requests {
		if !requestIsActive(req) {
			continue
		}
		out = append(out, studioRequestSnapshot{
			ID:       strings.TrimSpace(req.ID),
			Kind:     strings.TrimSpace(req.Kind),
			Status:   strings.TrimSpace(req.Status),
			Channel:  normalizeChannelSlug(req.Channel),
			From:     strings.TrimSpace(req.From),
			Title:    strings.TrimSpace(req.Title),
			Question: strings.TrimSpace(req.Question),
			Blocking: req.Blocking,
			Required: req.Required,
			ReplyTo:  strings.TrimSpace(req.ReplyTo),
		})
	}
	return out
}

func studioRecentMessagesFromState(messages []channelMessage, limit int) []studioMessageSnapshot {
	if limit <= 0 {
		return nil
	}
	start := len(messages) - limit
	if start < 0 {
		start = 0
	}
	out := make([]studioMessageSnapshot, 0, len(messages)-start)
	for _, msg := range messages[start:] {
		out = append(out, studioMessageSnapshot{
			ID:        strings.TrimSpace(msg.ID),
			Channel:   normalizeChannelSlug(msg.Channel),
			From:      strings.TrimSpace(msg.From),
			Title:     strings.TrimSpace(msg.Title),
			Content:   strings.TrimSpace(msg.Content),
			ReplyTo:   strings.TrimSpace(msg.ReplyTo),
			Timestamp: strings.TrimSpace(msg.Timestamp),
		})
	}
	return out
}

type studioAttentionAccumulator struct {
	Key      string
	Kind     string
	Severity string
	Title    string
	Summary  string
	Count    int
	LatestAt string
	TaskIDs  map[string]struct{}
}

func studioChannelAttentionGroupsFromState(state studioDevConsoleState, blockers []studioBlocker) map[string][]studioAttentionGroup {
	taskByID := make(map[string]teamTask, len(state.Tasks))
	for _, task := range state.Tasks {
		taskByID[strings.TrimSpace(task.ID)] = task
	}

	byChannel := make(map[string]map[string]*studioAttentionAccumulator)
	add := func(channel, key, kind, severity, title, summary, latestAt, taskID string) {
		channel = normalizeChannelSlug(channel)
		if channel == "" || key == "" {
			return
		}
		if byChannel[channel] == nil {
			byChannel[channel] = make(map[string]*studioAttentionAccumulator)
		}
		acc, ok := byChannel[channel][key]
		if !ok {
			acc = &studioAttentionAccumulator{
				Key:      key,
				Kind:     strings.TrimSpace(kind),
				Severity: strings.TrimSpace(severity),
				Title:    strings.TrimSpace(title),
				Summary:  strings.TrimSpace(summary),
				TaskIDs:  make(map[string]struct{}),
			}
			byChannel[channel][key] = acc
		}
		acc.Count++
		if studioTimestampAfter(latestAt, acc.LatestAt) {
			acc.LatestAt = strings.TrimSpace(latestAt)
		}
		if taskID = strings.TrimSpace(taskID); taskID != "" {
			acc.TaskIDs[taskID] = struct{}{}
		}
	}

	for _, task := range state.Tasks {
		if !studioTaskIsOpenish(task.Status) || !studioTaskIsOperationalFollowUp(task) {
			continue
		}
		title := studioTaskTitle(task)
		add(
			task.Channel,
			"follow_up_queue|"+strings.ToLower(strings.TrimSpace(title)),
			"follow_up_queue",
			"medium",
			title,
			"Repeated follow-up tasks are waiting for owner movement.",
			strings.TrimSpace(task.UpdatedAt),
			task.ID,
		)
	}

	for _, blocker := range blockers {
		if !studioBlockerBacksActiveTask(blocker, taskByID) {
			continue
		}
		add(
			blocker.Channel,
			"blocker|"+strings.TrimSpace(blocker.Kind)+"|"+studioNormalizeAttentionText(blocker.Summary)+"|"+studioNormalizeAttentionText(blocker.WaitingOn),
			blocker.Kind,
			blocker.Severity,
			firstNonEmpty(strings.TrimSpace(blocker.Title), strings.TrimSpace(blocker.Summary)),
			firstNonEmpty(strings.TrimSpace(blocker.Summary), strings.TrimSpace(blocker.Reason)),
			"",
			blocker.TaskID,
		)
	}

	for _, req := range state.Requests {
		if !studioRequestRequiresHumanAttentionSnapshot(req) {
			continue
		}
		add(
			req.Channel,
			"request_waiting_human",
			"request_waiting_human",
			"high",
			studioRequestTitle(req),
			"Blocking request is waiting on human input.",
			strings.TrimSpace(req.UpdatedAt),
			"",
		)
	}

	for _, alert := range state.Watchdogs {
		if normalizeWatchdogStatus(alert.Status) != "active" {
			continue
		}
		if strings.TrimSpace(alert.TargetType) == "task" {
			if task, ok := taskByID[strings.TrimSpace(alert.TargetID)]; ok && studioTaskIsOperationalFollowUp(task) {
				continue
			}
		}
		add(
			alert.Channel,
			"watchdog|"+strings.TrimSpace(alert.Kind)+"|"+studioNormalizeAttentionText(alert.Summary),
			strings.TrimSpace(alert.Kind),
			studioWatchdogSeverity(alert),
			firstNonEmpty(strings.TrimSpace(alert.Kind), "watchdog"),
			firstNonEmpty(strings.TrimSpace(alert.Summary), "Operational watchdog alert is active."),
			firstNonEmpty(strings.TrimSpace(alert.UpdatedAt), strings.TrimSpace(alert.CreatedAt)),
			firstTaskIDForAttention(alert.TargetType, alert.TargetID),
		)
	}

	out := make(map[string][]studioAttentionGroup, len(byChannel))
	for channel, groups := range byChannel {
		items := make([]studioAttentionGroup, 0, len(groups))
		for _, acc := range groups {
			taskIDs := make([]string, 0, len(acc.TaskIDs))
			for taskID := range acc.TaskIDs {
				taskIDs = append(taskIDs, taskID)
			}
			sort.Strings(taskIDs)
			summary := acc.Summary
			if acc.Kind == "follow_up_queue" && acc.Count > 1 {
				summary = fmt.Sprintf("%d follow-up tasks are waiting for owner movement.", acc.Count)
			}
			items = append(items, studioAttentionGroup{
				Key:      acc.Key,
				Kind:     acc.Kind,
				Severity: acc.Severity,
				Title:    acc.Title,
				Summary:  summary,
				Count:    acc.Count,
				LatestAt: acc.LatestAt,
				TaskIDs:  taskIDs,
			})
		}
		sort.Slice(items, func(i, j int) bool {
			if studioSeverityRank(items[i].Severity) != studioSeverityRank(items[j].Severity) {
				return studioSeverityRank(items[i].Severity) > studioSeverityRank(items[j].Severity)
			}
			if items[i].Count != items[j].Count {
				return items[i].Count > items[j].Count
			}
			if items[i].LatestAt != items[j].LatestAt {
				return studioTimestampAfter(items[i].LatestAt, items[j].LatestAt)
			}
			return items[i].Key < items[j].Key
		})
		out[channel] = items
	}
	return out
}

func studioBlockerBacksActiveTask(blocker studioBlocker, taskByID map[string]teamTask) bool {
	taskID := strings.TrimSpace(blocker.TaskID)
	if taskID == "" {
		return true
	}
	task, ok := taskByID[taskID]
	if !ok {
		return true
	}
	return !taskIsTerminal(&task)
}

func studioRecentSubstantiveMessagesByChannel(messages []channelMessage) map[string]studioMessageSnapshot {
	out := make(map[string]studioMessageSnapshot)
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		channel := normalizeChannelSlug(msg.Channel)
		if channel == "" {
			continue
		}
		if _, ok := out[channel]; ok {
			continue
		}
		if !studioMessageLooksSubstantive(msg) {
			continue
		}
		out[channel] = studioMessageSnapshot{
			ID:        strings.TrimSpace(msg.ID),
			Channel:   channel,
			From:      strings.TrimSpace(msg.From),
			Title:     strings.TrimSpace(msg.Title),
			Content:   strings.TrimSpace(msg.Content),
			ReplyTo:   strings.TrimSpace(msg.ReplyTo),
			Timestamp: strings.TrimSpace(msg.Timestamp),
		}
	}
	return out
}

func studioRecentDecisionsByChannel(decisions []officeDecisionRecord) map[string]officeDecisionRecord {
	out := make(map[string]officeDecisionRecord)
	for i := len(decisions) - 1; i >= 0; i-- {
		decision := decisions[i]
		channel := normalizeChannelSlug(decision.Channel)
		if channel == "" {
			continue
		}
		if _, ok := out[channel]; ok {
			continue
		}
		out[channel] = decision
	}
	return out
}

func studioChannelSnapshotsFromState(state studioDevConsoleState, tasks []studioTaskSnapshot, requests []studioRequestSnapshot, blockers []studioBlocker) []studioChannelSnapshot {
	taskCountsByChannel := make(map[string]studioTaskCounts)
	activeOwnersByChannel := make(map[string]map[string]struct{})
	workspaceCountByChannel := make(map[string]int)
	flowCountByChannel := make(map[string]int)
	requestCountByChannel := make(map[string]int)
	blockersByChannel := make(map[string][]string)
	waitingHumanCountByChannel := studioVisibleHumanTaskCountByChannel(state)
	attentionByChannel := studioChannelAttentionGroupsFromState(state, blockers)
	lastSubstantiveByChannel := make(map[string]studioMessageSnapshot)
	lastDecisionByChannel := make(map[string]officeDecisionRecord)
	taskByID := make(map[string]teamTask, len(state.Tasks))
	for _, task := range state.Tasks {
		taskByID[strings.TrimSpace(task.ID)] = task
	}

	for _, task := range tasks {
		channel := normalizeChannelSlug(task.Channel)
		counts := taskCountsByChannel[channel]
		counts.Total++
		switch strings.ToLower(strings.TrimSpace(task.Status)) {
		case "", "open":
			counts.Open++
		case "in_progress":
			counts.InProgress++
		case "blocked":
			counts.Blocked++
		case "review", "in_review":
			counts.Review++
		case "done":
			counts.Done++
		case "canceled", "cancelled":
			counts.Canceled++
		default:
			counts.Other++
		}
		taskCountsByChannel[channel] = counts
		if studioTaskIsOpenish(task.Status) {
			if activeOwnersByChannel[channel] == nil {
				activeOwnersByChannel[channel] = make(map[string]struct{})
			}
			if owner := strings.TrimSpace(task.Owner); owner != "" {
				activeOwnersByChannel[channel][owner] = struct{}{}
			}
		}
	}
	for _, req := range requests {
		channel := normalizeChannelSlug(req.Channel)
		requestCountByChannel[channel]++
	}
	for _, flow := range studioFlowSnapshotsFromTasks(state.Tasks) {
		flowCountByChannel[normalizeChannelSlug(flow.Channel)]++
	}
	for _, ws := range studioWorkspaceSnapshotsFromTasks(state.Tasks) {
		workspaceCountByChannel[normalizeChannelSlug(ws.Channel)]++
	}
	for _, blocker := range blockers {
		if !studioBlockerBacksActiveTask(blocker, taskByID) {
			continue
		}
		channel := normalizeChannelSlug(blocker.Channel)
		blockersByChannel[channel] = append(blockersByChannel[channel], blocker.Kind)
	}
	for _, msg := range studioRecentSubstantiveMessagesByChannel(state.Messages) {
		lastSubstantiveByChannel[msg.Channel] = msg
	}
	for _, decision := range studioRecentDecisionsByChannel(state.Decisions) {
		lastDecisionByChannel[normalizeChannelSlug(decision.Channel)] = decision
	}

	out := make([]studioChannelSnapshot, 0, len(state.Channels))
	for _, ch := range state.Channels {
		slug := normalizeChannelSlug(ch.Slug)
		lastSubstantive := lastSubstantiveByChannel[slug]
		lastDecision := lastDecisionByChannel[slug]
		attention := attentionByChannel[slug]
		out = append(out, studioChannelSnapshot{
			Slug:                    slug,
			Name:                    strings.TrimSpace(ch.Name),
			Members:                 append([]string(nil), ch.Members...),
			TaskCounts:              taskCountsByChannel[slug],
			RequestCount:            requestCountByChannel[slug],
			FlowCount:               flowCountByChannel[slug],
			WorkspaceCount:          workspaceCountByChannel[slug],
			Blockers:                uniqueStudioStrings(blockersByChannel[slug]),
			AttentionCount:          len(attention),
			WaitingHumanCount:       waitingHumanCountByChannel[slug],
			ActiveOwnerCount:        len(activeOwnersByChannel[slug]),
			LastSubstantiveUpdateAt: strings.TrimSpace(lastSubstantive.Timestamp),
			LastSubstantiveUpdateBy: strings.TrimSpace(lastSubstantive.From),
			LastSubstantivePreview:  strings.TrimSpace(lastSubstantive.Content),
			LastDecisionAt:          strings.TrimSpace(lastDecision.CreatedAt),
			LastDecisionSummary:     strings.TrimSpace(lastDecision.Summary),
			Attention:               attention,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AttentionCount != out[j].AttentionCount {
			return out[i].AttentionCount > out[j].AttentionCount
		}
		if out[i].WaitingHumanCount != out[j].WaitingHumanCount {
			return out[i].WaitingHumanCount > out[j].WaitingHumanCount
		}
		if out[i].TaskCounts.Blocked != out[j].TaskCounts.Blocked {
			return out[i].TaskCounts.Blocked > out[j].TaskCounts.Blocked
		}
		if out[i].TaskCounts.InProgress != out[j].TaskCounts.InProgress {
			return out[i].TaskCounts.InProgress > out[j].TaskCounts.InProgress
		}
		return out[i].Slug < out[j].Slug
	})
	return out
}

func studioVisibleHumanTaskCountByChannel(state studioDevConsoleState) map[string]int {
	counts := make(map[string]int)
	requestRoots := make(map[string]struct{}, len(state.Requests))
	visible := make([]teamTask, 0, len(state.Requests)+len(state.ExecutionNodes))

	for _, req := range state.Requests {
		if !requestIsActive(req) {
			continue
		}
		channel := normalizeChannelSlug(req.Channel)
		if channel == "" {
			channel = "general"
		}
		visible = append(visible, teamTask{
			ID:                     "human-request-" + strings.TrimSpace(req.ID),
			Channel:                channel,
			ExecutionKey:           "human-request|" + strings.TrimSpace(req.ID),
			Status:                 "pending",
			TaskType:               "human_action",
			AwaitingHuman:          true,
			AwaitingHumanRequestID: strings.TrimSpace(req.ID),
			SourceMessageID:        strings.TrimSpace(req.ReplyTo),
		})
		if root := strings.TrimSpace(req.ReplyTo); root != "" {
			requestRoots[channel+"|"+root] = struct{}{}
		}
	}

	for _, node := range state.ExecutionNodes {
		if !node.AwaitingHumanInput || !executionNodeIsOpen(node) {
			continue
		}
		channel := normalizeChannelSlug(node.Channel)
		if channel == "" {
			channel = "general"
		}
		root := firstNonEmpty(strings.TrimSpace(node.RootMessageID), strings.TrimSpace(node.TriggerMessageID))
		if root != "" {
			if _, ok := requestRoots[channel+"|"+root]; ok {
				continue
			}
		}
		visible = append(visible, teamTask{
			ID:                 "human-node-" + strings.TrimSpace(node.ID),
			Channel:            channel,
			ExecutionKey:       "human-node|" + strings.TrimSpace(node.ID),
			Status:             "pending",
			TaskType:           "human_action",
			AwaitingHuman:      true,
			AwaitingHumanSince: strings.TrimSpace(node.AwaitingHumanSince),
			SourceMessageID:    root,
		})
	}

	for _, task := range coalesceTaskView(visible) {
		channel := normalizeChannelSlug(task.Channel)
		if channel == "" {
			channel = "general"
		}
		counts[channel]++
	}

	return counts
}

func studioFlowSnapshotsFromTasks(tasks []teamTask) []studioFlowSnapshot {
	type flowBucket struct {
		snapshot studioFlowSnapshot
	}
	buckets := make(map[string]*flowBucket)
	for _, task := range tasks {
		key := studioFlowKey(task)
		if key == "" {
			continue
		}
		bucket, ok := buckets[key]
		if !ok {
			bucket = &flowBucket{snapshot: studioFlowSnapshot{
				ID:            key,
				Label:         studioFlowLabel(task),
				Channel:       normalizeChannelSlug(task.Channel),
				Owner:         strings.TrimSpace(task.Owner),
				Status:        strings.TrimSpace(task.Status),
				ExecutionMode: strings.TrimSpace(task.ExecutionMode),
				WorkflowKey:   strings.TrimSpace(task.ExecutionKey),
				PipelineID:    strings.TrimSpace(task.PipelineID),
				Workspace:     studioTaskWorkspacePath(task),
			}}
			buckets[key] = bucket
		}
		bucket.snapshot.TaskCount++
		if task.Blocked {
			bucket.snapshot.BlockedCount++
		}
		bucket.snapshot.TaskIDs = append(bucket.snapshot.TaskIDs, strings.TrimSpace(task.ID))
		if bucket.snapshot.Owner == "" && strings.TrimSpace(task.Owner) != "" {
			bucket.snapshot.Owner = strings.TrimSpace(task.Owner)
		}
		if bucket.snapshot.Status == "" || bucket.snapshot.Status == "open" {
			bucket.snapshot.Status = strings.TrimSpace(task.Status)
		}
		if bucket.snapshot.Workspace == "" {
			bucket.snapshot.Workspace = studioTaskWorkspacePath(task)
		}
	}
	out := make([]studioFlowSnapshot, 0, len(buckets))
	for _, bucket := range buckets {
		sort.Strings(bucket.snapshot.TaskIDs)
		out = append(out, bucket.snapshot)
	}
	return out
}

func studioWorkspaceSnapshotsFromTasks(tasks []teamTask) []studioWorkspaceSnapshot {
	type workspaceBucket struct {
		snapshot studioWorkspaceSnapshot
	}
	buckets := make(map[string]*workspaceBucket)
	for _, task := range tasks {
		path := studioTaskWorkspacePath(task)
		if path == "" {
			continue
		}
		key := strings.ToLower(filepath.Clean(path))
		bucket, ok := buckets[key]
		if !ok {
			healthy, issue := studioWorkspaceHealth(task)
			bucket = &workspaceBucket{snapshot: studioWorkspaceSnapshot{
				Path:         path,
				WorktreePath: strings.TrimSpace(task.WorktreePath),
				Branch:       strings.TrimSpace(task.WorktreeBranch),
				Channel:      normalizeChannelSlug(task.Channel),
				Owner:        strings.TrimSpace(task.Owner),
				Healthy:      healthy,
				Issue:        issue,
			}}
			buckets[key] = bucket
		}
		bucket.snapshot.TaskIDs = append(bucket.snapshot.TaskIDs, strings.TrimSpace(task.ID))
		bucket.snapshot.TaskCounts = addStudioTaskCount(bucket.snapshot.TaskCounts, task)
		if !bucket.snapshot.Healthy {
			_, issue := studioWorkspaceHealth(task)
			if issue != "" {
				bucket.snapshot.Issue = issue
			}
		}
	}
	out := make([]studioWorkspaceSnapshot, 0, len(buckets))
	for _, bucket := range buckets {
		sort.Strings(bucket.snapshot.TaskIDs)
		out = append(out, bucket.snapshot)
	}
	return out
}

func studioTaskWorkspacePath(task teamTask) string {
	if path := strings.TrimSpace(task.WorkspacePath); path != "" {
		return path
	}
	return strings.TrimSpace(task.WorktreePath)
}

func studioWorkspaceHealth(task teamTask) (bool, string) {
	switch strings.ToLower(strings.TrimSpace(task.ExecutionMode)) {
	case "local_worktree":
		path := strings.TrimSpace(task.WorktreePath)
		if path == "" {
			return false, "Local worktree is missing an assigned path."
		}
		if err := verifyTaskWorktreeWritable(path); err != nil {
			return false, "Local worktree is not writable: " + err.Error()
		}
		return true, ""
	case "external_workspace":
		path := strings.TrimSpace(task.WorkspacePath)
		if path == "" {
			return false, "External workspace is missing a workspace path."
		}
		if !taskWorktreeSourceLooksUsable(path) {
			return false, "External workspace path is not a usable git workspace."
		}
		return true, ""
	default:
		path := studioTaskWorkspacePath(task)
		if path == "" {
			return true, ""
		}
		if !taskWorktreeSourceLooksUsable(path) {
			return false, "Workspace path is not usable as a git workspace."
		}
		return true, ""
	}
}

func studioFocusTask(task *teamTask) bool {
	if task == nil {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(task.Status))
	return task.Blocked || status == "blocked" || status == "in_progress" || status == "review"
}

func studioFirstBlockingRequest(requests []humanInterview) *humanInterview {
	for i := range requests {
		if requestIsActive(requests[i]) && requests[i].Blocking {
			return &requests[i]
		}
	}
	return nil
}

func studioFirstActiveTask(tasks []teamTask) *teamTask {
	for i := range tasks {
		if studioFocusTask(&tasks[i]) {
			return &tasks[i]
		}
	}
	return nil
}

func studioFirstBlocker(blockers []studioBlocker) *studioBlocker {
	if len(blockers) == 0 {
		return nil
	}
	return &blockers[0]
}

func studioTaskIsOpenish(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "done", "completed", "canceled", "cancelled":
		return false
	default:
		return true
	}
}

func studioTaskIsOperationalFollowUp(task teamTask) bool {
	if strings.EqualFold(strings.TrimSpace(task.TaskType), "follow_up") {
		return true
	}
	return strings.HasPrefix(strings.TrimSpace(task.ExecutionKey), ceoConversationFollowUpTaskPrefix+"|")
}

func studioRequestRequiresHumanAttentionSnapshot(req humanInterview) bool {
	if !requestIsActive(req) {
		return false
	}
	return req.Blocking || req.Required
}

func studioRequestRequiresHumanAttention(req studioRequestSnapshot) bool {
	status := strings.ToLower(strings.TrimSpace(req.Status))
	switch status {
	case "answered", "resolved", "completed", "done", "dismissed":
		return false
	}
	return req.Blocking || req.Required
}

func studioMessageLooksSubstantive(msg channelMessage) bool {
	content := strings.TrimSpace(msg.Content)
	if content == "" || strings.HasPrefix(content, "[STATUS]") {
		return false
	}
	switch normalizeActorSlug(msg.From) {
	case "", "watchdog", "wuphf", "scheduler":
		return false
	default:
		return true
	}
}

func studioNormalizeAttentionText(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	return truncateSummary(raw, 160)
}

func studioWatchdogSeverity(alert watchdogAlert) string {
	switch strings.ToLower(strings.TrimSpace(alert.Kind)) {
	case "agent_runtime_blocked", "request_sla_breach":
		return "high"
	default:
		return "medium"
	}
}

func firstTaskIDForAttention(targetType, targetID string) string {
	if strings.EqualFold(strings.TrimSpace(targetType), "task") {
		return strings.TrimSpace(targetID)
	}
	return ""
}

func studioSeverityRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func studioTimestampAfter(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" {
		return false
	}
	if right == "" {
		return true
	}
	leftTime, leftOK := parseStudioTimestamp(left)
	rightTime, rightOK := parseStudioTimestamp(right)
	switch {
	case leftOK && rightOK:
		return leftTime.After(rightTime)
	case leftOK:
		return true
	case rightOK:
		return false
	default:
		return left > right
	}
}

func parseStudioTimestamp(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func studioTaskTitle(task teamTask) string {
	title := strings.TrimSpace(task.Title)
	if title == "" {
		title = strings.TrimSpace(task.ID)
	}
	if title == "" {
		title = "task"
	}
	return title
}

func studioRequestTitle(req humanInterview) string {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = strings.TrimSpace(req.Question)
	}
	if title == "" {
		title = "direction requested"
	}
	return title
}

func studioBlockerTargetLabel(blocker studioBlocker) string {
	if blocker.TaskID != "" {
		return "task " + blocker.TaskID
	}
	if blocker.Channel != "" {
		return "#" + blocker.Channel
	}
	return blocker.Title
}

func studioDegradationSignals(state studioDevConsoleState, blockers []studioBlocker, counts studioTaskCounts) []string {
	signals := make([]string, 0, 8)
	if !state.BrokerReady {
		signals = append(signals, "broker_uninitialized")
	}
	if len(state.WebUIOrigins) == 0 {
		signals = append(signals, "web_ui_uninitialized")
	}
	if counts.Blocked > 0 {
		signals = append(signals, fmt.Sprintf("%d_blocked_tasks", counts.Blocked))
	}
	for _, blocker := range blockers {
		signals = append(signals, blocker.Kind)
	}
	if counts.Total == 0 {
		signals = append(signals, "no_tasks")
	}
	if len(state.Members) == 0 {
		signals = append(signals, "no_members")
	}
	if len(state.Channels) == 0 {
		signals = append(signals, "no_channels")
	}
	return uniqueStudioStrings(signals)
}

func studioBootstrapSummary(bootstrap studioBootstrapSnapshot, health studioBrokerHealthSnapshot) string {
	parts := []string{
		fmt.Sprintf("%d members", bootstrap.Members),
		fmt.Sprintf("%d channels", bootstrap.Channels),
		fmt.Sprintf("%d tasks", bootstrap.Tasks),
		fmt.Sprintf("%d requests", bootstrap.Requests),
		fmt.Sprintf("%d workspaces", bootstrap.Workspaces),
	}
	if health.Degraded {
		return "degraded: " + strings.Join(parts, ", ")
	}
	return "ready: " + strings.Join(parts, ", ")
}

func studioTaskUpdatedEpoch(task studioTaskSnapshot) int64 {
	if ts := studioParsedTaskTime(task.UpdatedAt); !ts.IsZero() {
		return ts.Unix()
	}
	return 0
}

func studioParsedTaskTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if ts, err := time.Parse(layout, raw); err == nil {
			return ts.UTC()
		}
	}
	return time.Time{}
}

func studioTaskHasTimeoutSignalFromSnapshot(task studioTaskSnapshot) bool {
	status := strings.ToLower(strings.TrimSpace(task.Status))
	if status != "blocked" && !task.Blocked {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{task.Title, task.WorkspacePath, task.WorktreePath}, " ")))
	return strings.Contains(text, "timeout") || strings.Contains(text, "timed out")
}

func studioTaskHasTimeoutSignal(task teamTask) bool {
	status := strings.ToLower(strings.TrimSpace(task.Status))
	if status != "blocked" && !task.Blocked {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{task.Title, task.Details}, " ")))
	if !strings.Contains(text, "timeout") && !strings.Contains(text, "timed out") {
		return false
	}
	ts := studioParsedTaskTime(task.UpdatedAt)
	if ts.IsZero() {
		ts = studioParsedTaskTime(task.CreatedAt)
	}
	if ts.IsZero() {
		return true
	}
	return time.Since(ts) >= 5*time.Minute
}

func studioTimeoutReason(task teamTask) string {
	reason := strings.TrimSpace(task.Details)
	if reason == "" {
		reason = strings.TrimSpace(task.Title)
	}
	if reason == "" {
		reason = "Blocked task timed out before a substantive update."
	}
	return reason
}

func studioTaskHasUnresolvedDependency(task teamTask, taskByID map[string]teamTask) bool {
	if len(task.DependsOn) == 0 {
		return false
	}
	for _, depID := range task.DependsOn {
		depID = strings.TrimSpace(depID)
		if depID == "" {
			continue
		}
		dep, ok := taskByID[depID]
		if !ok {
			return true
		}
		switch strings.ToLower(strings.TrimSpace(dep.Status)) {
		case "done", "completed":
			continue
		default:
			return true
		}
	}
	return false
}

func studioTaskPendingDependencies(task teamTask, taskByID map[string]teamTask) []string {
	if len(task.DependsOn) == 0 {
		return nil
	}
	out := make([]string, 0, len(task.DependsOn))
	for _, depID := range task.DependsOn {
		depID = strings.TrimSpace(depID)
		if depID == "" {
			continue
		}
		dep, ok := taskByID[depID]
		if !ok {
			out = append(out, depID+" (missing)")
			continue
		}
		switch strings.ToLower(strings.TrimSpace(dep.Status)) {
		case "done", "completed":
		default:
			out = append(out, depID)
		}
	}
	return out
}

func studioTaskDependencyReason(task teamTask) string {
	if len(task.DependsOn) == 0 {
		return "No dependency metadata was recorded."
	}
	return fmt.Sprintf("Waiting on dependencies: %s.", strings.Join(task.DependsOn, ", "))
}

func studioTaskOwnerMissingFromChannel(task teamTask, channelBySlug map[string]teamChannel) bool {
	owner := strings.TrimSpace(task.Owner)
	if owner == "" {
		return false
	}
	if strings.EqualFold(owner, "watchdog") {
		return false
	}
	ch, ok := channelBySlug[normalizeChannelSlug(task.Channel)]
	if !ok || ch.isDM() {
		return false
	}
	if containsString(ch.Members, owner) {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(ch.CreatedBy), owner) {
		return false
	}
	return true
}

func studioTaskLooksLikeWorkflowMissingInput(task teamTask) bool {
	if taskIsTerminal(&task) {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		task.Title,
		task.Details,
		task.TaskType,
		task.PipelineID,
		task.ExecutionKey,
	}, " ")))
	if text == "" {
		return false
	}
	if !(strings.Contains(text, "workflow") || strings.Contains(text, "pipeline") || strings.Contains(text, "automation")) {
		return false
	}
	return strings.Contains(text, "missing required input") ||
		strings.Contains(text, "required input") ||
		strings.Contains(text, "missing input") ||
		strings.Contains(text, "need input") ||
		strings.Contains(text, "awaiting input") ||
		strings.Contains(text, "connection_key") ||
		strings.Contains(text, "workflow_definition")
}

func studioTaskWorkflowReason(task teamTask) string {
	reason := strings.TrimSpace(task.Details)
	if reason == "" {
		reason = "Workflow task missing required input."
	}
	return reason
}

func studioWorkflowMissingInputWaitingOn(task teamTask) string {
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{task.Title, task.Details}, " ")))
	switch {
	case strings.Contains(text, "connection_key"):
		return "connection_key"
	case strings.Contains(text, "workflow_definition"):
		return "workflow_definition"
	default:
		return "required input"
	}
}

func studioTaskLooksLikeDegradedLocalEnvironment(task teamTask) bool {
	mode := strings.ToLower(strings.TrimSpace(task.ExecutionMode))
	switch mode {
	case "local_worktree":
		path := strings.TrimSpace(task.WorktreePath)
		if path == "" {
			return true
		}
		return verifyTaskWorktreeWritable(path) != nil
	case "external_workspace":
		path := strings.TrimSpace(task.WorkspacePath)
		if path == "" {
			return true
		}
		return !taskWorktreeSourceLooksUsable(path)
	default:
		if path := studioTaskWorkspacePath(task); path != "" {
			return !taskWorktreeSourceLooksUsable(path)
		}
		return false
	}
}

func studioEnvironmentReason(task teamTask) string {
	if strings.TrimSpace(task.WorktreePath) != "" {
		return "Worktree path is not writable."
	}
	if strings.TrimSpace(task.WorkspacePath) != "" {
		return "Workspace path is not a usable git workspace."
	}
	return "Local execution environment is degraded."
}

func studioEnvironmentWaitingOn(task teamTask) string {
	if path := strings.TrimSpace(task.WorktreePath); path != "" {
		return path
	}
	if path := strings.TrimSpace(task.WorkspacePath); path != "" {
		return path
	}
	return "local workspace"
}

func studioRequestLooksLikeDirection(req humanInterview) bool {
	if !requestIsActive(req) {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		req.Kind,
		req.Title,
		req.Question,
	}, " ")))
	if text == "" {
		return false
	}
	if strings.Contains(text, "direction") || strings.Contains(text, "guidance") || strings.Contains(text, "constraints") || strings.Contains(text, "next step") {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(req.Kind), "freeform") || strings.EqualFold(strings.TrimSpace(req.Kind), "secret")
}

func studioRequestDirectionReason(req humanInterview) string {
	reason := strings.TrimSpace(req.Question)
	if reason == "" {
		reason = strings.TrimSpace(req.Title)
	}
	if reason == "" {
		reason = "Direction was requested without a corresponding task."
	}
	return reason
}

func studioHasChannelTask(tasks []teamTask, channel string) bool {
	channel = normalizeChannelSlug(channel)
	for _, task := range tasks {
		if normalizeChannelSlug(task.Channel) != channel {
			continue
		}
		if !taskIsTerminal(&task) {
			return true
		}
	}
	return false
}

func studioFlowKey(task teamTask) string {
	channel := normalizeChannelSlug(task.Channel)
	if channel == "" {
		channel = "general"
	}
	switch {
	case strings.TrimSpace(task.ExecutionKey) != "":
		return channel + "|workflow|" + strings.TrimSpace(task.ExecutionKey)
	case strings.TrimSpace(task.PipelineID) != "":
		return channel + "|pipeline|" + strings.TrimSpace(task.PipelineID)
	case strings.TrimSpace(task.ExecutionKey) != "":
		return channel + "|execution|" + strings.TrimSpace(task.ExecutionKey)
	case strings.TrimSpace(task.TaskType) != "":
		return channel + "|type|" + strings.TrimSpace(task.TaskType)
	default:
		return channel + "|task|" + strings.TrimSpace(task.ID)
	}
}

func studioFlowLabel(task teamTask) string {
	switch {
	case strings.TrimSpace(task.ExecutionKey) != "":
		return strings.TrimSpace(task.ExecutionKey)
	case strings.TrimSpace(task.PipelineID) != "":
		return strings.TrimSpace(task.PipelineID)
	case strings.TrimSpace(task.TaskType) != "":
		return strings.TrimSpace(task.TaskType)
	default:
		return studioTaskTitle(task)
	}
}

func addStudioTaskCount(counts studioTaskCounts, task teamTask) studioTaskCounts {
	switch strings.ToLower(strings.TrimSpace(task.Status)) {
	case "", "open":
		counts.Open++
	case "in_progress":
		counts.InProgress++
	case "blocked":
		counts.Blocked++
	case "review", "in_review":
		counts.Review++
	case "done":
		counts.Done++
	case "canceled", "cancelled":
		counts.Canceled++
	default:
		counts.Other++
	}
	return counts
}

func addStudioTaskCountSnapshot(counts studioTaskCounts, task studioTaskSnapshot) studioTaskCounts {
	switch strings.ToLower(strings.TrimSpace(task.Status)) {
	case "", "open":
		counts.Open++
	case "in_progress":
		counts.InProgress++
	case "blocked":
		counts.Blocked++
	case "review", "in_review":
		counts.Review++
	case "done":
		counts.Done++
	case "canceled", "cancelled":
		counts.Canceled++
	default:
		counts.Other++
	}
	counts.Total++
	return counts
}

func writeStudioDevConsoleJSON(w http.ResponseWriter, payload studioDevConsoleActionResponse) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func writeStudioDevConsoleActionError(w http.ResponseWriter, err error) {
	msg := err.Error()
	code := http.StatusBadRequest
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "not found"):
		code = http.StatusNotFound
	case strings.Contains(lower, "not blocked"),
		strings.Contains(lower, "required"),
		strings.Contains(lower, "not in #"):
		code = http.StatusConflict
	}
	http.Error(w, msg, code)
}

func uniqueStudioStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func uniqueStudioBlockers(values []studioBlocker) []studioBlocker {
	seen := make(map[string]struct{}, len(values))
	out := make([]studioBlocker, 0, len(values))
	for _, blocker := range values {
		key := blocker.Kind + "|" + blocker.ID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, blocker)
	}
	sort.Slice(out, func(i, j int) bool {
		if studioSeverityRank(out[i].Severity) != studioSeverityRank(out[j].Severity) {
			return studioSeverityRank(out[i].Severity) > studioSeverityRank(out[j].Severity)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func uniqueStudioActionInvocations(values []studioActionInvocation) []studioActionInvocation {
	seen := make(map[string]struct{}, len(values))
	out := make([]studioActionInvocation, 0, len(values))
	for _, action := range values {
		key := action.Action + "|" + action.TaskID + "|" + action.Channel + "|" + action.Owner + "|" + action.Agent
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, action)
	}
	return out
}
