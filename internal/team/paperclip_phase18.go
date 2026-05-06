package team

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

type agentSessionsResponse struct {
	GeneratedAt string                 `json:"generated_at"`
	Summary     map[string]int         `json:"summary"`
	Sessions    []agentSessionSnapshot `json:"sessions"`
}

type agentSessionSnapshot struct {
	Slug              string          `json:"slug"`
	Channel           string          `json:"channel,omitempty"`
	Status            string          `json:"status"`
	NormalizedStatus  string          `json:"normalized_status,omitempty"`
	Activity          string          `json:"activity,omitempty"`
	Detail            string          `json:"detail,omitempty"`
	CurrentTaskID     string          `json:"current_task_id,omitempty"`
	CurrentTaskTitle  string          `json:"current_task_title,omitempty"`
	WorkspacePath     string          `json:"workspace_path,omitempty"`
	RunID             string          `json:"run_id,omitempty"`
	HeartbeatAt       string          `json:"heartbeat_at,omitempty"`
	LastSeenAt        string          `json:"last_seen_at,omitempty"`
	OpenTaskCount     int             `json:"open_task_count,omitempty"`
	QueuedNodeCount   int             `json:"queued_node_count,omitempty"`
	Usage             *usageTotals    `json:"usage,omitempty"`
	ContextSummary    string          `json:"context_summary,omitempty"`
	NextAction        string          `json:"next_action,omitempty"`
	LivenessState     string          `json:"liveness_state,omitempty"`
	LivenessReason    string          `json:"liveness_reason,omitempty"`
	LivenessTaskID    string          `json:"liveness_task_id,omitempty"`
	LivenessAt        string          `json:"liveness_at,omitempty"`
	LivenessHistory   []livenessEvent `json:"liveness_history,omitempty"`
	PersistentContext bool            `json:"persistent_context"`
}

type livenessEvent struct {
	State     string `json:"state"`
	Reason    string `json:"reason,omitempty"`
	TaskID    string `json:"task_id,omitempty"`
	Actor     string `json:"actor,omitempty"`
	Channel   string `json:"channel,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

func (b *Broker) handleAgentSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	allChannels := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all_channels")), "true")
	if channel == "" && !allChannels {
		channel = "general"
	}
	b.mu.RLock()
	payload := b.buildAgentSessionsLocked(viewer, channel, allChannels)
	b.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) buildAgentSessionsLocked(viewer, channel string, allChannels bool) agentSessionsResponse {
	now := time.Now().UTC()
	sessions := map[string]*agentSessionSnapshot{}
	touch := func(slug, laneChannel string) *agentSessionSnapshot {
		slug = normalizeActorSlug(slug)
		if slug == "" || isHumanLikeActor(slug) || isSystemActor(slug) {
			return nil
		}
		laneChannel = normalizeChannelSlug(laneChannel)
		if laneChannel == "" {
			laneChannel = "general"
		}
		if !intakeChannelVisible(b, viewer, laneChannel, channel, allChannels) {
			return nil
		}
		key := laneChannel + ":" + slug
		session := sessions[key]
		if session == nil {
			session = &agentSessionSnapshot{Slug: slug, Channel: laneChannel, Status: "idle", Activity: "idle", PersistentContext: true}
			sessions[key] = session
		}
		return session
	}
	for _, member := range b.members {
		for _, ch := range b.channels {
			if stringSliceContains(ch.Members, member.Slug) {
				touch(member.Slug, ch.Slug)
			}
		}
	}
	for laneKey, activity := range b.activity {
		laneChannel, slug := parseAgentLaneKey(laneKey)
		session := touch(slug, laneChannel)
		if session == nil {
			continue
		}
		session.Status = firstNonEmpty(strings.TrimSpace(activity.Status), session.Status)
		session.Activity = firstNonEmpty(strings.TrimSpace(activity.Activity), session.Activity)
		session.Detail = strings.TrimSpace(activity.Detail)
		session.LastSeenAt = firstNonEmpty(activity.LastTime, activity.LivenessAt, session.LastSeenAt)
		session.LivenessState = strings.TrimSpace(activity.LivenessState)
		session.LivenessReason = strings.TrimSpace(activity.LivenessReason)
		session.LivenessTaskID = strings.TrimSpace(activity.LivenessTaskID)
		session.LivenessAt = strings.TrimSpace(activity.LivenessAt)
	}
	for _, node := range b.executionNodes {
		if !executionNodeIsOpen(node) {
			continue
		}
		session := touch(node.OwnerAgent, node.Channel)
		if session == nil {
			continue
		}
		session.QueuedNodeCount++
		session.LastSeenAt = maxTimestamp(session.LastSeenAt, node.UpdatedAt, node.CreatedAt)
		if session.NextAction == "" {
			session.NextAction = firstNonEmpty(node.ExpectedResponseKind, "respond_to_thread")
		}
	}
	for _, task := range b.tasks {
		if strings.TrimSpace(task.ArchivedAt) != "" {
			continue
		}
		taskChannel := normalizeChannelSlug(task.Channel)
		session := touch(task.Owner, taskChannel)
		if session == nil {
			continue
		}
		if !taskIsTerminal(&task) {
			session.OpenTaskCount++
		}
		if task.ExecutionLock != nil {
			lock := normalizeTaskExecutionLock(task.ExecutionLock, now)
			if lock != nil && strings.EqualFold(lock.Owner, session.Slug) {
				session.CurrentTaskID = task.ID
				session.CurrentTaskTitle = task.Title
				session.WorkspacePath = studioTaskWorkspacePath(task)
				session.RunID = lock.RunID
				session.HeartbeatAt = firstNonEmpty(lock.HeartbeatAt, lock.AcquiredAt)
				session.LastSeenAt = maxTimestamp(session.LastSeenAt, session.HeartbeatAt, task.UpdatedAt)
				if taskExecutionLockIsActive(lock, now) {
					session.Status = "active"
					session.Activity = "executing"
				}
			}
		}
		if session.CurrentTaskID == "" && !taskIsTerminal(&task) {
			session.CurrentTaskID = task.ID
			session.CurrentTaskTitle = task.Title
			session.WorkspacePath = studioTaskWorkspacePath(task)
			session.LastSeenAt = maxTimestamp(session.LastSeenAt, task.UpdatedAt, task.CreatedAt)
		}
		if session.ContextSummary == "" {
			session.ContextSummary = truncateSummary(firstNonEmpty(task.GoalSummary, task.QueueReason, task.Details, task.Title), 180)
		}
		if session.NextAction == "" {
			session.NextAction = taskNextActionSummary(task)
		}
	}
	for slug, usage := range b.usage.Agents {
		for _, session := range sessions {
			if session.Slug == normalizeActorSlug(slug) {
				copyUsage := usage
				session.Usage = &copyUsage
			}
		}
	}
	out := make([]agentSessionSnapshot, 0, len(sessions))
	for _, session := range sessions {
		session.LastSeenAt = firstNonEmpty(session.LastSeenAt, session.HeartbeatAt)
		session.LivenessHistory = b.livenessHistoryForSessionLocked(*session, 5)
		if session.LivenessState == "" && len(session.LivenessHistory) > 0 {
			latest := session.LivenessHistory[0]
			session.LivenessState = latest.State
			session.LivenessReason = latest.Reason
			session.LivenessTaskID = latest.TaskID
			session.LivenessAt = latest.CreatedAt
		}
		if session.Status == "idle" && (session.QueuedNodeCount > 0 || session.OpenTaskCount > 0) {
			session.Status = "queued"
		}
		session.NormalizedStatus = normalizeAgentSessionRuntimeStatus(*session)
		out = append(out, *session)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Status != out[j].Status {
			return sessionStatusRank(out[i].Status) > sessionStatusRank(out[j].Status)
		}
		return out[i].Slug < out[j].Slug
	})
	summary := map[string]int{"total": len(out)}
	for _, session := range out {
		summary[session.Status]++
		if session.NormalizedStatus != "" {
			summary["normalized_"+session.NormalizedStatus]++
		}
		if session.HeartbeatAt != "" {
			summary["with_heartbeat"]++
		}
		if session.CurrentTaskID != "" {
			summary["with_task"]++
		}
	}
	return agentSessionsResponse{GeneratedAt: now.Format(time.RFC3339), Summary: summary, Sessions: out}
}

func (b *Broker) livenessHistoryForSessionLocked(session agentSessionSnapshot, limit int) []livenessEvent {
	history := make([]livenessEvent, 0, limit)
	for i := len(b.actions) - 1; i >= 0; i-- {
		action := b.actions[i]
		if action.Kind != "liveness_recorded" {
			continue
		}
		if normalizeActorSlug(action.Actor) != normalizeActorSlug(session.Slug) {
			continue
		}
		if normalizeChannelSlug(action.Channel) != normalizeChannelSlug(session.Channel) {
			continue
		}
		if session.CurrentTaskID != "" && strings.TrimSpace(action.RelatedID) != session.CurrentTaskID {
			continue
		}
		state, reason := parseLivenessActionSummary(action.Summary)
		history = append(history, livenessEvent{
			State:     state,
			Reason:    reason,
			TaskID:    strings.TrimSpace(action.RelatedID),
			Actor:     strings.TrimSpace(action.Actor),
			Channel:   normalizeChannelSlug(action.Channel),
			CreatedAt: strings.TrimSpace(action.CreatedAt),
		})
		if limit > 0 && len(history) >= limit {
			break
		}
	}
	return history
}

func (b *Broker) attachTaskLivenessHistoryLocked(task *teamTask, limit int) {
	if task == nil {
		return
	}
	history := b.livenessHistoryForTaskLocked(*task, limit)
	if len(history) == 0 {
		return
	}
	task.LivenessHistory = history
	latest := history[0]
	task.LivenessState = latest.State
	task.LivenessReason = latest.Reason
	task.LivenessAt = latest.CreatedAt
}

func (b *Broker) livenessHistoryForTaskLocked(task teamTask, limit int) []livenessEvent {
	taskID := strings.TrimSpace(task.ID)
	if taskID == "" {
		return nil
	}
	channel := normalizeChannelSlug(task.Channel)
	history := make([]livenessEvent, 0, limit)
	for i := len(b.actions) - 1; i >= 0; i-- {
		action := b.actions[i]
		if action.Kind != "liveness_recorded" {
			continue
		}
		if strings.TrimSpace(action.RelatedID) != taskID {
			continue
		}
		actionChannel := normalizeChannelSlug(action.Channel)
		if channel != "" && actionChannel != "" && actionChannel != channel {
			continue
		}
		state, reason := parseLivenessActionSummary(action.Summary)
		history = append(history, livenessEvent{
			State:     state,
			Reason:    reason,
			TaskID:    taskID,
			Actor:     strings.TrimSpace(action.Actor),
			Channel:   actionChannel,
			CreatedAt: strings.TrimSpace(action.CreatedAt),
		})
		if limit > 0 && len(history) >= limit {
			break
		}
	}
	return history
}

func sessionStatusRank(status string) int {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active", "running", "executing":
		return 4
	case "queued", "pending":
		return 3
	case "blocked", "degraded":
		return 2
	default:
		return 1
	}
}

func taskNextActionSummary(task teamTask) string {
	if task.Blocked || normalizeTaskStatus(task.Status) == "blocked" {
		return firstNonEmpty(task.CompletionBlocker, task.QueueReason, "resolve_blocker")
	}
	if task.PlanRequired && task.PlanStatus != "approved" {
		return "approve_or_update_plan"
	}
	if task.CompletionEvidenceRequired && !task.CompletionEvidenceSatisfied {
		return "attach_outcome_evidence"
	}
	return firstNonEmpty(task.QueueReason, task.GoalSummary, "continue_task")
}

func maxTimestamp(values ...string) string {
	best := ""
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if best == "" || studioTimestampAfter(value, best) {
			best = value
		}
	}
	return best
}

type executionTraceResponse struct {
	GeneratedAt string                `json:"generated_at"`
	Summary     map[string]int        `json:"summary"`
	Traces      []executionTraceEntry `json:"traces"`
}

type executionTraceEntry struct {
	TaskID           string               `json:"task_id"`
	Title            string               `json:"title"`
	Channel          string               `json:"channel,omitempty"`
	Owner            string               `json:"owner,omitempty"`
	Status           string               `json:"status,omitempty"`
	NormalizedStatus string               `json:"normalized_status,omitempty"`
	StartedAt        string               `json:"started_at,omitempty"`
	UpdatedAt        string               `json:"updated_at,omitempty"`
	Steps            []executionTraceStep `json:"steps"`
}

type executionTraceStep struct {
	ID               string `json:"id"`
	Kind             string `json:"kind"`
	Actor            string `json:"actor,omitempty"`
	ActorType        string `json:"actor_type,omitempty"`
	Status           string `json:"status,omitempty"`
	NormalizedStatus string `json:"normalized_status,omitempty"`
	Summary          string `json:"summary,omitempty"`
	RelatedID        string `json:"related_id,omitempty"`
	Timestamp        string `json:"timestamp,omitempty"`
}

func (b *Broker) handleExecutionTrace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	taskID := strings.TrimSpace(r.URL.Query().Get("task_id"))
	allChannels := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all_channels")), "true")
	if channel == "" && !allChannels && taskID == "" {
		channel = "general"
	}
	b.mu.RLock()
	payload := b.buildExecutionTraceLocked(viewer, channel, allChannels, taskID)
	b.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) buildExecutionTraceLocked(viewer, channel string, allChannels bool, taskID string) executionTraceResponse {
	candidates := make([]teamTask, 0)
	for _, task := range b.tasks {
		taskChannel := normalizeChannelSlug(task.Channel)
		if taskID != "" && task.ID != taskID {
			continue
		}
		if !intakeChannelVisible(b, viewer, taskChannel, channel, allChannels || taskID != "") {
			continue
		}
		candidates = append(candidates, task)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return studioTimestampAfter(firstNonEmpty(candidates[i].UpdatedAt, candidates[i].CreatedAt), firstNonEmpty(candidates[j].UpdatedAt, candidates[j].CreatedAt))
	})
	if taskID == "" && len(candidates) > 25 {
		candidates = candidates[:25]
	}
	traces := make([]executionTraceEntry, 0, len(candidates))
	for _, task := range candidates {
		traces = append(traces, b.executionTraceForTaskLocked(task, taskID != ""))
	}
	sort.Slice(traces, func(i, j int) bool { return studioTimestampAfter(traces[i].UpdatedAt, traces[j].UpdatedAt) })
	summary := map[string]int{"total": len(traces)}
	for _, trace := range traces {
		if trace.NormalizedStatus != "" {
			summary["normalized_"+trace.NormalizedStatus]++
		}
		summary["steps"] += len(trace.Steps)
		for _, step := range trace.Steps {
			summary[step.Kind]++
			if step.NormalizedStatus != "" {
				summary["step_normalized_"+step.NormalizedStatus]++
			}
			if executionTraceStepNeedsAttention(step) {
				summary["attention"]++
			}
		}
	}
	return executionTraceResponse{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Summary: summary, Traces: traces}
}

func (b *Broker) executionTraceForTaskLocked(task teamTask, includeThread bool) executionTraceEntry {
	trace := executionTraceEntry{
		TaskID:           task.ID,
		Title:            task.Title,
		Channel:          normalizeChannelSlug(task.Channel),
		Owner:            task.Owner,
		Status:           normalizeTaskStatus(task.Status),
		NormalizedStatus: normalizeTaskRuntimeStatus(task),
		StartedAt:        task.CreatedAt,
		UpdatedAt:        firstNonEmpty(task.UpdatedAt, task.CreatedAt),
	}
	add := func(step executionTraceStep) {
		step.ID = strings.TrimSpace(step.ID)
		step.Kind = strings.TrimSpace(step.Kind)
		if step.ID == "" || step.Kind == "" {
			return
		}
		step.Actor = strings.TrimSpace(step.Actor)
		step.ActorType = actorTypeForActivity(step.Actor, "", step.Kind)
		step.Summary = truncateSummary(step.Summary, 220)
		step.Timestamp = strings.TrimSpace(step.Timestamp)
		step.NormalizedStatus = normalizeExecutionTraceStepRuntimeStatus(step)
		trace.Steps = append(trace.Steps, step)
		trace.UpdatedAt = maxTimestamp(trace.UpdatedAt, step.Timestamp)
	}
	add(executionTraceStep{ID: task.ID + ":created", Kind: "task_created", Actor: task.CreatedBy, Status: trace.Status, Summary: task.Title, Timestamp: task.CreatedAt})
	if task.ExecutionLock != nil {
		lock := normalizeTaskExecutionLock(task.ExecutionLock, time.Now().UTC())
		if lock != nil {
			add(executionTraceStep{ID: task.ID + ":lock:" + lock.RunID, Kind: "execution_lock", Actor: lock.Owner, Status: lock.Status, Summary: firstNonEmpty(lock.RunID, "execution lock"), Timestamp: firstNonEmpty(lock.HeartbeatAt, lock.AcquiredAt)})
		}
	}
	for _, revision := range task.PlanRevisions {
		add(executionTraceStep{ID: firstNonEmpty(revision.ID, fmt.Sprintf("%s:plan:%d", task.ID, revision.Version)), Kind: "plan_revision", Actor: revision.CreatedBy, Status: revision.Status, Summary: firstNonEmpty(revision.Summary, revision.Content), Timestamp: revision.CreatedAt})
	}
	for _, action := range b.actions {
		if action.RelatedID != task.ID {
			continue
		}
		if action.Kind == "liveness_recorded" {
			state, reason := parseLivenessActionSummary(action.Summary)
			add(executionTraceStep{ID: action.ID, Kind: "liveness", Actor: action.Actor, Status: state, Summary: reason, RelatedID: action.RelatedID, Timestamp: action.CreatedAt})
			continue
		}
		add(executionTraceStep{ID: action.ID, Kind: "action", Actor: action.Actor, Status: actionStatusFromKind(action.Kind), Summary: action.Kind + ": " + action.Summary, RelatedID: action.RelatedID, Timestamp: action.CreatedAt})
	}
	if includeThread {
		for _, node := range b.executionNodes {
			if task.ThreadID == "" {
				continue
			}
			if node.RootMessageID != task.ThreadID && node.TriggerMessageID != task.ThreadID && node.ResolvedByMessageID != task.ThreadID {
				continue
			}
			add(executionTraceStep{ID: node.ID, Kind: "execution_node", Actor: node.OwnerAgent, Status: normalizeExecutionNodeStatus(node.Status), Summary: firstNonEmpty(node.ExpectedResponseKind, node.LastError), RelatedID: node.RootMessageID, Timestamp: firstNonEmpty(node.UpdatedAt, node.CreatedAt)})
		}
	}
	if includeThread {
		for _, msg := range b.messages {
			if task.ThreadID == "" {
				continue
			}
			if msg.ID != task.ThreadID && msg.ReplyTo != task.ThreadID {
				continue
			}
			add(executionTraceStep{ID: msg.ID, Kind: "message", Actor: msg.From, Status: msg.Kind, Summary: firstNonEmpty(msg.Title, msg.Content), RelatedID: msg.ReplyTo, Timestamp: msg.Timestamp})
		}
	}
	for _, artifact := range task.Artifacts {
		add(executionTraceStep{ID: firstNonEmpty(artifact.ID, task.ID+":artifact:"+artifact.Kind), Kind: "artifact", Actor: artifact.CreatedBy, Status: artifact.State, Summary: firstNonEmpty(artifact.Title, artifact.Summary, artifact.URL, artifact.Path), RelatedID: task.ID, Timestamp: firstNonEmpty(artifact.UpdatedAt, artifact.CreatedAt)})
	}
	for _, eval := range task.Evals {
		add(executionTraceStep{ID: firstNonEmpty(eval.ID, task.ID+":eval:"+eval.Kind), Kind: "eval", Status: eval.Severity, Summary: eval.Summary, RelatedID: task.ID, Timestamp: eval.CreatedAt})
	}
	for _, feedback := range task.Feedback {
		add(executionTraceStep{ID: firstNonEmpty(feedback.ID, task.ID+":feedback:"+feedback.CreatedAt), Kind: "feedback", Actor: feedback.CreatedBy, Status: feedback.Rating, Summary: feedback.Comment, RelatedID: task.ID, Timestamp: feedback.CreatedAt})
	}
	if task.Outcome != "" || task.OutcomeEvidence != "" || task.OutcomeStatus != "" {
		add(executionTraceStep{ID: task.ID + ":outcome", Kind: "outcome", Actor: task.Owner, Status: firstNonEmpty(task.OutcomeStatus, trace.Status), Summary: firstNonEmpty(task.OutcomeEvidence, task.Outcome), RelatedID: task.ID, Timestamp: firstNonEmpty(task.OutcomeVerifiedAt, task.UpdatedAt)})
	}
	sort.SliceStable(trace.Steps, func(i, j int) bool {
		left := trace.Steps[i].Timestamp
		right := trace.Steps[j].Timestamp
		if left == right {
			return trace.Steps[i].ID < trace.Steps[j].ID
		}
		return !studioTimestampAfter(left, right)
	})
	return trace
}

func parseLivenessActionSummary(summary string) (string, string) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return "unknown", ""
	}
	state, reason, ok := strings.Cut(summary, ":")
	if !ok {
		return strings.TrimSpace(summary), ""
	}
	return strings.TrimSpace(state), strings.TrimSpace(reason)
}

func executionTraceStepNeedsAttention(step executionTraceStep) bool {
	status := strings.ToLower(strings.TrimSpace(step.Status))
	switch status {
	case "warning", "error", "blocked", "failed", "empty_response", "plan_only", "needs_followup":
		return true
	default:
		return false
	}
}

type rollbackPackagesResponse struct {
	GeneratedAt string                      `json:"generated_at"`
	Summary     map[string]int              `json:"summary"`
	Packages    []governanceRollbackPackage `json:"packages"`
}

type governanceRollbackPackage struct {
	ID                   string                     `json:"id"`
	EventID              string                     `json:"event_id"`
	EventKind            string                     `json:"event_kind"`
	EventSummary         string                     `json:"event_summary"`
	Status               string                     `json:"status"`
	TargetID             string                     `json:"target_id,omitempty"`
	Channel              string                     `json:"channel,omitempty"`
	RequiredConfirmation string                     `json:"required_confirmation"`
	RequiredReviews      []string                   `json:"required_reviews,omitempty"`
	RollbackPlan         string                     `json:"rollback_plan"`
	Changes              []governanceRollbackChange `json:"changes"`
	SnapshotHint         string                     `json:"snapshot_hint,omitempty"`
	CreatedAt            string                     `json:"created_at,omitempty"`
}

type governanceRollbackChange struct {
	Target         string `json:"target"`
	Field          string `json:"field,omitempty"`
	Action         string `json:"action"`
	RestoreTo      string `json:"restore_to,omitempty"`
	Reason         string `json:"reason,omitempty"`
	RequiresManual bool   `json:"requires_manual,omitempty"`
}

type rollbackApplyRequest struct {
	PackageID    string `json:"package_id"`
	EventID      string `json:"event_id,omitempty"`
	Actor        string `json:"actor,omitempty"`
	Reason       string `json:"reason,omitempty"`
	Confirm      bool   `json:"confirm,omitempty"`
	Confirmation string `json:"confirmation,omitempty"`
}

type rollbackApplyResponse struct {
	Persisted            bool                       `json:"persisted"`
	RequiredConfirmation string                     `json:"required_confirmation,omitempty"`
	Package              *governanceRollbackPackage `json:"package,omitempty"`
	AuditActionID        string                     `json:"audit_action_id,omitempty"`
	Message              string                     `json:"message,omitempty"`
}

func (b *Broker) handleGovernanceRollbackPackages(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
		channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
		allChannels := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all_channels")), "true")
		targetID := strings.TrimSpace(firstNonEmpty(r.URL.Query().Get("id"), r.URL.Query().Get("event_id"), r.URL.Query().Get("package_id")))
		if channel == "" && !allChannels && targetID == "" {
			channel = "general"
		}
		b.mu.RLock()
		payload := b.buildRollbackPackagesLocked(viewer, channel, allChannels, targetID)
		b.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	case http.MethodPost:
		var req rollbackApplyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json body", http.StatusBadRequest)
			return
		}
		b.mu.Lock()
		payload, err := b.applyRollbackPackageLocked(req)
		b.mu.Unlock()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *Broker) buildRollbackPackagesLocked(viewer, channel string, allChannels bool, targetID string) rollbackPackagesResponse {
	packages := make([]governanceRollbackPackage, 0)
	for _, event := range b.buildGovernanceHistoryLocked(50) {
		if targetID != "" && event.ID != targetID && event.RelatedID != targetID && rollbackPackageID(event.ID) != targetID {
			continue
		}
		eventChannel := normalizeChannelSlug(event.Channel)
		if eventChannel == "" {
			eventChannel = "general"
		}
		if !intakeChannelVisible(b, viewer, eventChannel, channel, allChannels || targetID != "") {
			continue
		}
		packages = append(packages, rollbackPackageForGovernanceEvent(event))
	}
	summary := map[string]int{"total": len(packages)}
	for _, pkg := range packages {
		summary[pkg.Status]++
		if len(pkg.RequiredReviews) > 0 {
			summary["requires_review"]++
		}
		for _, change := range pkg.Changes {
			if change.RequiresManual {
				summary["manual"]++
			}
		}
	}
	return rollbackPackagesResponse{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Summary: summary, Packages: packages}
}

func rollbackPackageID(eventID string) string {
	return "rollback:" + strings.TrimSpace(eventID)
}

func rollbackPackageForGovernanceEvent(event governanceEvent) governanceRollbackPackage {
	pkg := governanceRollbackPackage{
		ID:                   rollbackPackageID(event.ID),
		EventID:              event.ID,
		EventKind:            event.Kind,
		EventSummary:         event.Summary,
		Status:               "preview",
		TargetID:             event.RelatedID,
		Channel:              normalizeChannelSlug(event.Channel),
		RequiredConfirmation: "ROLLBACK_PACKAGE",
		RollbackPlan:         firstNonEmpty(event.RollbackPlan, "Record a compensating action and attach the prior value before changing state."),
		CreatedAt:            event.CreatedAt,
	}
	text := strings.ToLower(strings.Join([]string{event.Kind, event.Summary, event.RollbackPlan}, " "))
	switch {
	case event.RequiresTopologyAuthorization || strings.Contains(text, "topology") || strings.Contains(text, "roster") || strings.Contains(text, "channel"):
		pkg.RequiredReviews = []string{"topology-authorization", "operator"}
		pkg.SnapshotHint = "Use broker-state history or snapshot before applying any topology-sensitive restoration."
		pkg.Changes = append(pkg.Changes, governanceRollbackChange{Target: firstNonEmpty(event.RelatedID, event.ID), Field: "broker_state", Action: "restore_snapshot", RestoreTo: "previous_authorized_snapshot", Reason: "Topology-sensitive state cannot be changed automatically.", RequiresManual: true})
	case strings.Contains(text, "policy"):
		pkg.RequiredReviews = []string{"operator"}
		pkg.Changes = append(pkg.Changes, governanceRollbackChange{Target: firstNonEmpty(event.RelatedID, event.ID), Field: "policy.active", Action: "deactivate_policy", RestoreTo: "false", Reason: "Policy rollback is modeled as a compensating deactivation.", RequiresManual: false})
	case strings.Contains(text, "skill"):
		pkg.RequiredReviews = []string{"skill-owner", "operator"}
		pkg.Changes = append(pkg.Changes, governanceRollbackChange{Target: firstNonEmpty(event.RelatedID, event.ID), Field: "skill.metadata", Action: "restore_previous_skill_metadata", RestoreTo: "last_known_good", Reason: "Skill changes should preserve evidence while restoring metadata.", RequiresManual: true})
	case strings.Contains(text, "adapter"):
		pkg.RequiredReviews = []string{"operator"}
		pkg.Changes = append(pkg.Changes, governanceRollbackChange{Target: firstNonEmpty(event.RelatedID, event.ID), Field: "adapter.request", Action: "cancel_or_supersede_adapter_action", RestoreTo: "no_pending_action", Reason: "Adapter actions are governed requests and should be superseded rather than deleted.", RequiresManual: false})
	default:
		pkg.RequiredReviews = []string{"operator"}
		pkg.Changes = append(pkg.Changes, governanceRollbackChange{Target: firstNonEmpty(event.RelatedID, event.ID), Action: "record_compensating_action", RestoreTo: "prior_value", Reason: "No safe automatic state mutation is inferred.", RequiresManual: true})
	}
	return pkg
}

func (b *Broker) applyRollbackPackageLocked(req rollbackApplyRequest) (rollbackApplyResponse, error) {
	req.PackageID = strings.TrimSpace(req.PackageID)
	req.EventID = strings.TrimSpace(req.EventID)
	req.Actor = firstNonEmpty(strings.TrimSpace(req.Actor), "human")
	req.Reason = strings.TrimSpace(req.Reason)
	target := firstNonEmpty(req.PackageID, req.EventID)
	if target == "" {
		return rollbackApplyResponse{}, fmt.Errorf("package_id or event_id required")
	}
	packages := b.buildRollbackPackagesLocked(req.Actor, "", true, target).Packages
	if len(packages) == 0 {
		return rollbackApplyResponse{}, fmt.Errorf("rollback package not found")
	}
	pkg := packages[0]
	payload := rollbackApplyResponse{Persisted: false, RequiredConfirmation: "ROLLBACK_PACKAGE", Package: &pkg, Message: "Set confirm=true and confirmation=ROLLBACK_PACKAGE to record this rollback request."}
	if !req.Confirm || req.Confirmation != "ROLLBACK_PACKAGE" {
		return payload, nil
	}
	if req.Reason == "" {
		return rollbackApplyResponse{}, fmt.Errorf("reason required")
	}
	b.appendActionLocked("governance_rollback_requested", "governance", firstNonEmpty(pkg.Channel, "general"), req.Actor, truncateSummary(pkg.EventKind+": "+req.Reason, 180), pkg.EventID)
	action := b.actions[len(b.actions)-1]
	if err := b.saveLocked(); err != nil {
		return rollbackApplyResponse{}, fmt.Errorf("failed to persist rollback request: %w", err)
	}
	payload.Persisted = true
	payload.AuditActionID = action.ID
	payload.Message = "Rollback request recorded. Automatic state mutation was not performed; apply the package steps with the listed reviews."
	return payload, nil
}
