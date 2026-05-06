package team

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"
)

type resumePackResponse struct {
	GeneratedAt string           `json:"generated_at"`
	Task        *resumePackTask  `json:"task,omitempty"`
	Context     []resumePackFact `json:"context,omitempty"`
	Evidence    []resumePackFact `json:"evidence,omitempty"`
	NextSteps   []string         `json:"next_steps,omitempty"`
	Warnings    []string         `json:"warnings,omitempty"`
	Source      string           `json:"source,omitempty"`
}

type resumePackTask struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Channel        string   `json:"channel,omitempty"`
	Owner          string   `json:"owner,omitempty"`
	Status         string   `json:"status,omitempty"`
	Queue          string   `json:"queue,omitempty"`
	Priority       string   `json:"priority,omitempty"`
	GoalPath       []string `json:"goal_path,omitempty"`
	GoalSummary    string   `json:"goal_summary,omitempty"`
	WorkspacePath  string   `json:"workspace_path,omitempty"`
	WorktreePath   string   `json:"worktree_path,omitempty"`
	UpdatedAt      string   `json:"updated_at,omitempty"`
	LivenessState  string   `json:"liveness_state,omitempty"`
	LivenessReason string   `json:"liveness_reason,omitempty"`
}

type resumePackFact struct {
	Label   string `json:"label"`
	Value   string `json:"value,omitempty"`
	Detail  string `json:"detail,omitempty"`
	Kind    string `json:"kind,omitempty"`
	When    string `json:"when,omitempty"`
	Source  string `json:"source,omitempty"`
	Related string `json:"related,omitempty"`
}

func (b *Broker) handleResumePack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	taskID := strings.TrimSpace(r.URL.Query().Get("task_id"))
	owner := normalizeActorSlug(r.URL.Query().Get("owner"))
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	if channel == "" {
		channel = "general"
	}
	b.mu.RLock()
	payload, ok := b.buildResumePackLocked(viewer, taskID, owner, channel)
	b.mu.RUnlock()
	if !ok {
		http.Error(w, "task not found or channel access denied", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) buildResumePackLocked(viewer, taskID, owner, channel string) (resumePackResponse, bool) {
	var selected *teamTask
	if taskID != "" {
		selected = b.findTaskByIDLocked(taskID)
		if selected == nil || !b.canAccessChannelLocked(viewer, normalizeChannelSlug(selected.Channel)) {
			return resumePackResponse{}, false
		}
	} else {
		selected = b.selectResumeTaskLocked(viewer, owner, channel)
		if selected == nil {
			return resumePackResponse{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Source: "empty"}, true
		}
	}
	task := *selected
	applyTaskOperatorContract(&task, currentTaskGoalContext())
	payload := resumePackResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Task: &resumePackTask{
			ID:             strings.TrimSpace(task.ID),
			Title:          strings.TrimSpace(task.Title),
			Channel:        normalizeChannelSlug(task.Channel),
			Owner:          strings.TrimSpace(task.Owner),
			Status:         normalizeTaskStatus(task.Status),
			Queue:          normalizeTaskQueueKey(task.QueueKey),
			Priority:       strings.TrimSpace(firstNonEmpty(task.QueuePriority, "normal")),
			GoalPath:       compactStringList(task.GoalPath),
			GoalSummary:    strings.TrimSpace(task.GoalSummary),
			WorkspacePath:  strings.TrimSpace(task.WorkspacePath),
			WorktreePath:   strings.TrimSpace(task.WorktreePath),
			UpdatedAt:      strings.TrimSpace(firstNonEmpty(task.UpdatedAt, task.CreatedAt)),
			LivenessState:  latestTaskLivenessState(task),
			LivenessReason: latestTaskLivenessReason(task),
		},
		Source: "task",
	}
	payload.Context = append(payload.Context,
		resumePackFact{Kind: "objective", Label: "Objective", Value: truncateSummary(firstNonEmpty(task.Outcome, task.Details, task.Title), 260)},
		resumePackFact{Kind: "completion", Label: "Completion contract", Value: boolSummary(task.CompletionEvidenceSatisfied), Detail: task.CompletionBlocker},
		resumePackFact{Kind: "plan", Label: "Latest plan", Value: firstNonEmpty(task.LatestPlanSummary, task.PlanStatus), Detail: task.PlanBlocker},
	)
	if len(task.DependsOn) > 0 {
		payload.Context = append(payload.Context, resumePackFact{Kind: "dependency", Label: "Dependencies", Value: strings.Join(compactStringList(task.DependsOn), ", ")})
	}
	if task.ExecutionLock != nil {
		payload.Context = append(payload.Context, resumePackFact{Kind: "execution_lock", Label: "Execution lock", Value: task.ExecutionLock.Status, Detail: task.ExecutionLock.Owner, When: task.ExecutionLock.HeartbeatAt})
	}
	for _, artifact := range task.Artifacts {
		if len(payload.Evidence) >= 4 {
			break
		}
		payload.Evidence = append(payload.Evidence, resumePackFact{
			Kind:   "artifact",
			Label:  firstNonEmpty(artifact.Title, artifact.Kind, "Artifact"),
			Value:  truncateSummary(firstNonEmpty(artifact.Summary, artifact.Path, artifact.URL), 220),
			When:   firstNonEmpty(artifact.ValidatedAt, artifact.UpdatedAt, artifact.CreatedAt),
			Source: artifact.ResultRole,
		})
	}
	if strings.TrimSpace(task.OutcomeEvidence) != "" {
		payload.Evidence = append(payload.Evidence, resumePackFact{Kind: "outcome", Label: "Outcome evidence", Value: truncateSummary(task.OutcomeEvidence, 260), When: task.OutcomeVerifiedAt})
	}
	payload.NextSteps = buildResumeNextSteps(task)
	payload.Warnings = buildResumeWarnings(task)
	return payload, true
}

func (b *Broker) selectResumeTaskLocked(viewer, owner, channel string) *teamTask {
	var candidates []teamTask
	for _, task := range b.tasks {
		taskChannel := normalizeChannelSlug(task.Channel)
		if !b.canAccessChannelLocked(viewer, taskChannel) {
			continue
		}
		if owner != "" && normalizeActorSlug(task.Owner) != owner {
			continue
		}
		if owner == "" && channel != "" && taskChannel != channel {
			continue
		}
		status := normalizeTaskStatus(task.Status)
		if status == "done" || status == "canceled" || status == "failed" || strings.TrimSpace(task.ArchivedAt) != "" {
			continue
		}
		candidates = append(candidates, task)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return workQueueItemLess(workQueueItemFromTask(candidates[i]), workQueueItemFromTask(candidates[j]), time.Now().UTC())
	})
	if len(candidates) == 0 {
		return nil
	}
	for i := range b.tasks {
		if b.tasks[i].ID == candidates[0].ID {
			return &b.tasks[i]
		}
	}
	return nil
}

func workQueueItemFromTask(task teamTask) workQueueItem {
	return workQueueItem{
		TaskID:    strings.TrimSpace(task.ID),
		Title:     strings.TrimSpace(task.Title),
		QueueKey:  normalizeTaskQueueKey(task.QueueKey),
		Channel:   normalizeChannelSlug(task.Channel),
		Owner:     strings.TrimSpace(task.Owner),
		Status:    normalizeTaskStatus(task.Status),
		Priority:  strings.TrimSpace(firstNonEmpty(task.QueuePriority, "normal")),
		Reason:    strings.TrimSpace(task.QueueReason),
		SLAAt:     strings.TrimSpace(task.QueueSLAAt),
		UpdatedAt: strings.TrimSpace(firstNonEmpty(task.UpdatedAt, task.CreatedAt)),
	}
}

func buildResumeNextSteps(task teamTask) []string {
	var steps []string
	if task.Blocked || normalizeTaskStatus(task.Status) == "blocked" {
		steps = append(steps, "Resolve or answer the blocking request before waking more work.")
	}
	if !task.CompletionEvidenceSatisfied && task.CompletionEvidenceRequired {
		steps = append(steps, "Produce durable evidence before marking the task done.")
	}
	if task.PlanRequired && task.PlanStatus != "approved" {
		steps = append(steps, "Approve or revise the latest plan before execution.")
	}
	if len(steps) == 0 {
		steps = append(steps, "Continue from the latest evidence and publish a concrete progress update.")
	}
	return steps
}

func buildResumeWarnings(task teamTask) []string {
	var warnings []string
	if strings.TrimSpace(task.WorktreePath) == "" && strings.Contains(strings.ToLower(task.ExecutionMode), "worktree") {
		warnings = append(warnings, "Task expects a worktree but no worktree path is recorded.")
	}
	if task.ExecutionLock != nil && strings.EqualFold(task.ExecutionLock.Status, "active") {
		warnings = append(warnings, "Another run may already hold the execution lock.")
	}
	if len(task.ReviewFindings) > 0 {
		warnings = append(warnings, "Review findings are attached; check unresolved items before completion.")
	}
	return warnings
}

func boolSummary(ok bool) string {
	if ok {
		return "satisfied"
	}
	return "pending"
}

func latestTaskLivenessState(task teamTask) string {
	if task.LastHandoff != nil && strings.TrimSpace(task.LastHandoff.StatusClaim) != "" {
		return strings.TrimSpace(task.LastHandoff.StatusClaim)
	}
	return ""
}

func latestTaskLivenessReason(task teamTask) string {
	if task.LastHandoff != nil {
		return truncateSummary(task.LastHandoff.Summary, 180)
	}
	return ""
}

type governanceHistoryResponse struct {
	GeneratedAt string                 `json:"generated_at"`
	Events      []governanceEvent      `json:"events"`
	Summary     governanceHistoryStats `json:"summary"`
}

type governanceHistoryStats struct {
	PendingApprovals     int `json:"pending_approvals"`
	TopologySensitive    int `json:"topology_sensitive"`
	RecentDecisionCount  int `json:"recent_decision_count"`
	RollbackPlanCoverage int `json:"rollback_plan_coverage"`
}

type governanceEvent struct {
	ID                            string `json:"id"`
	Kind                          string `json:"kind"`
	Status                        string `json:"status,omitempty"`
	Actor                         string `json:"actor,omitempty"`
	Channel                       string `json:"channel,omitempty"`
	Summary                       string `json:"summary"`
	RelatedID                     string `json:"related_id,omitempty"`
	RequiresTopologyAuthorization bool   `json:"requires_topology_authorization,omitempty"`
	RollbackPlan                  string `json:"rollback_plan,omitempty"`
	CreatedAt                     string `json:"created_at,omitempty"`
}

func (b *Broker) handleGovernanceHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := parsePositiveLimit(r.URL.Query().Get("limit"), 30)
	b.mu.RLock()
	events := b.buildGovernanceHistoryLocked(limit)
	b.mu.RUnlock()
	stats := governanceHistoryStats{}
	for _, event := range events {
		if event.Status == "proposed" || strings.Contains(event.Kind, "approval") {
			stats.PendingApprovals++
		}
		if event.RequiresTopologyAuthorization {
			stats.TopologySensitive++
		}
		if event.Status == "approved" || event.Status == "rejected" || strings.Contains(event.Kind, "approved") {
			stats.RecentDecisionCount++
		}
		if event.RollbackPlan != "" {
			stats.RollbackPlanCoverage++
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(governanceHistoryResponse{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Events: events, Summary: stats})
}

func (b *Broker) buildGovernanceHistoryLocked(limit int) []governanceEvent {
	var events []governanceEvent
	for _, proposal := range b.orgProposals {
		events = append(events, governanceEvent{
			ID:                            proposal.ID,
			Kind:                          "org_proposal",
			Status:                        firstNonEmpty(proposal.Status, "proposed"),
			Actor:                         firstNonEmpty(proposal.DecidedBy, proposal.ProposedBy),
			Channel:                       normalizeChannelSlug(proposal.Channel),
			Summary:                       truncateSummary(firstNonEmpty(proposal.Title, proposal.Summary, proposal.ProposedChange), 220),
			RelatedID:                     proposal.TargetID,
			RequiresTopologyAuthorization: proposal.RequiresTopologyAuthorization,
			RollbackPlan:                  governanceRollbackPlan(proposal.Kind, proposal.TargetType, proposal.ProposedChange),
			CreatedAt:                     firstNonEmpty(proposal.DecidedAt, proposal.UpdatedAt, proposal.CreatedAt),
		})
	}
	for _, action := range b.actions {
		if !isGovernanceAction(action) {
			continue
		}
		events = append(events, governanceEvent{
			ID:                            action.ID,
			Kind:                          action.Kind,
			Status:                        governanceStatusFromAction(action),
			Actor:                         action.Actor,
			Channel:                       normalizeChannelSlug(action.Channel),
			Summary:                       truncateSummary(action.Summary, 220),
			RelatedID:                     action.RelatedID,
			RequiresTopologyAuthorization: action.RequiresApproval || action.GovernanceSeverity == "topology",
			RollbackPlan:                  governanceRollbackPlan(action.Kind, "", action.Summary),
			CreatedAt:                     action.CreatedAt,
		})
	}
	sort.Slice(events, func(i, j int) bool {
		return studioTimestampAfter(events[i].CreatedAt, events[j].CreatedAt)
	})
	if len(events) > limit {
		events = events[:limit]
	}
	return events
}

func isGovernanceAction(action officeActionLog) bool {
	kind := strings.ToLower(action.Kind)
	return strings.Contains(kind, "approved") ||
		strings.Contains(kind, "approval") ||
		strings.Contains(kind, "policy") ||
		strings.Contains(kind, "adapter") ||
		strings.Contains(kind, "skill_capability") ||
		strings.Contains(kind, "template") ||
		strings.Contains(kind, "topology") ||
		strings.Contains(kind, "completion_blocked")
}

func governanceStatusFromAction(action officeActionLog) string {
	kind := strings.ToLower(action.Kind)
	switch {
	case strings.Contains(kind, "approved"):
		return "approved"
	case strings.Contains(kind, "rejected"), strings.Contains(kind, "blocked"):
		return "blocked"
	case action.RequiresApproval:
		return "approval_required"
	default:
		return "recorded"
	}
}

func governanceRollbackPlan(kind, targetType, change string) string {
	if orgProposalRequiresTopologyAuthorization(kind, targetType, change) {
		return "Keep as preview until explicitly authorized; if applied later, restore the previous roster/channel snapshot from broker-state history."
	}
	if strings.Contains(strings.ToLower(kind+" "+change), "skill") {
		return "Archive or revert the skill record; do not delete evidence."
	}
	if strings.Contains(strings.ToLower(kind+" "+change), "policy") {
		return "Deactivate the policy and record the operator decision."
	}
	return "Record a compensating action with the prior value if the change is applied."
}

type skillTrustResponse struct {
	GeneratedAt string             `json:"generated_at"`
	Summary     skillTrustSummary  `json:"summary"`
	Skills      []skillTrustRecord `json:"skills"`
}

type skillTrustSummary struct {
	Total  int `json:"total"`
	High   int `json:"high"`
	Medium int `json:"medium"`
	Low    int `json:"low"`
}

type skillTrustRecord struct {
	Name          string   `json:"name"`
	Title         string   `json:"title,omitempty"`
	PluginID      string   `json:"plugin_id,omitempty"`
	PluginKind    string   `json:"plugin_kind,omitempty"`
	SourceType    string   `json:"source_type,omitempty"`
	SourceRef     string   `json:"source_ref,omitempty"`
	SourceHash    string   `json:"source_hash,omitempty"`
	InstalledAt   string   `json:"installed_at,omitempty"`
	LastScannedAt string   `json:"last_scanned_at,omitempty"`
	ScanStatus    string   `json:"scan_status,omitempty"`
	ScanSummary   string   `json:"scan_summary,omitempty"`
	Channel       string   `json:"channel,omitempty"`
	Status        string   `json:"status,omitempty"`
	HealthStatus  string   `json:"health_status,omitempty"`
	Score         int      `json:"score"`
	Level         string   `json:"level"`
	Reasons       []string `json:"reasons,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
	LastRun       string   `json:"last_run,omitempty"`
}

func (b *Broker) handleSkillTrust(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.mu.RLock()
	records := make([]skillTrustRecord, 0, len(b.skills))
	for _, skill := range b.skills {
		if skill.Status == "archived" {
			continue
		}
		records = append(records, buildSkillTrustRecord(skill))
	}
	b.mu.RUnlock()
	sort.Slice(records, func(i, j int) bool {
		if records[i].Score != records[j].Score {
			return records[i].Score < records[j].Score
		}
		return records[i].Name < records[j].Name
	})
	summary := skillTrustSummary{Total: len(records)}
	for _, record := range records {
		switch record.Level {
		case "high":
			summary.High++
		case "medium":
			summary.Medium++
		default:
			summary.Low++
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(skillTrustResponse{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Summary: summary, Skills: records})
}

func buildSkillTrustRecord(skill teamSkill) skillTrustRecord {
	score := 100
	var reasons []string
	legacySkill := strings.TrimSpace(skill.PluginID) == "" && strings.TrimSpace(skill.PluginKind) == "" && len(skill.Capabilities) == 0
	if legacySkill {
		score -= 18
		reasons = append(reasons, "legacy skill metadata incomplete")
	} else {
		if strings.TrimSpace(skill.PluginID) == "" {
			score -= 10
			reasons = append(reasons, "missing plugin id")
		}
		if strings.TrimSpace(skill.PluginKind) == "" {
			score -= 8
			reasons = append(reasons, "missing plugin kind")
		}
		if len(skill.Capabilities) == 0 {
			score -= 12
			reasons = append(reasons, "no declared capabilities")
		}
		if !skillCanInvoke(skill) {
			score -= 18
			reasons = append(reasons, "not invokable by declared capabilities")
		}
	}
	switch normalizeSkillHealthStatus(skill.HealthStatus) {
	case "error":
		score -= 35
		reasons = append(reasons, "health error")
	case "warning", "unknown", "":
		score -= 6
		reasons = append(reasons, "health not proven ready")
	case "disabled":
		score -= 40
		reasons = append(reasons, "disabled")
	}
	if skillExecutionFailureStillRelevant(skill) {
		score -= 18
		reasons = append(reasons, "last execution failed")
	}
	if strings.TrimSpace(skill.SourceType) == "" || strings.TrimSpace(skill.SourceRef) == "" || strings.TrimSpace(skill.SourceHash) == "" {
		score -= 6
		reasons = append(reasons, "provenance incomplete")
	}
	switch normalizeSkillScanStatus(skill.ScanStatus) {
	case "warning":
		score -= 12
		reasons = append(reasons, "provenance scan warning")
	case "blocked":
		score -= 35
		reasons = append(reasons, "provenance scan blocked")
	case "":
		score -= 4
		reasons = append(reasons, "not scanned")
	}
	if contentLooksSecretBearing(skill.Content + " " + skill.Description + " " + skill.WorkflowDefinition) {
		score -= 20
		reasons = append(reasons, "mentions secret-like material")
	}
	if score < 0 {
		score = 0
	}
	level := "high"
	if score < 60 {
		level = "low"
	} else if score < 82 {
		level = "medium"
	}
	return skillTrustRecord{
		Name:          skill.Name,
		Title:         skill.Title,
		PluginID:      skill.PluginID,
		PluginKind:    skill.PluginKind,
		SourceType:    skill.SourceType,
		SourceRef:     skill.SourceRef,
		SourceHash:    skill.SourceHash,
		InstalledAt:   skill.InstalledAt,
		LastScannedAt: skill.LastScannedAt,
		ScanStatus:    firstNonEmpty(skill.ScanStatus, "unknown"),
		ScanSummary:   skill.ScanSummary,
		Channel:       skill.Channel,
		Status:        skill.Status,
		HealthStatus:  firstNonEmpty(skill.HealthStatus, "unknown"),
		Score:         score,
		Level:         level,
		Reasons:       compactStringList(reasons),
		Capabilities:  normalizeSkillCapabilities(skill.Capabilities),
		LastRun:       firstNonEmpty(skill.LastExecutionAt, skill.UpdatedAt),
	}
}

func skillExecutionFailureStillRelevant(skill teamSkill) bool {
	status := strings.ToLower(strings.TrimSpace(skill.LastExecutionStatus))
	if !strings.Contains(status, "fail") && !strings.Contains(status, "error") {
		return false
	}
	lastRun := parseBrokerTimestamp(firstNonEmpty(skill.LastExecutionAt, skill.UpdatedAt))
	if lastRun.IsZero() {
		return true
	}
	return time.Since(lastRun) <= 7*24*time.Hour
}

func contentLooksSecretBearing(value string) bool {
	lower := strings.ToLower(value)
	for _, token := range []string{"api_key", "apikey", "secret", "token", "password", "credential", "bearer "} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

type operatorOverviewResponse struct {
	GeneratedAt string                     `json:"generated_at"`
	Status      string                     `json:"status"`
	Counts      operatorOverviewCounts     `json:"counts"`
	Alerts      []operatorAlert            `json:"alerts,omitempty"`
	NextWork    []workQueueItem            `json:"next_work,omitempty"`
	Blockers    []studioBlocker            `json:"blockers,omitempty"`
	Requests    []studioRequestSnapshot    `json:"requests,omitempty"`
	Governance  []governanceEvent          `json:"governance,omitempty"`
	SkillTrust  skillTrustSummary          `json:"skill_trust"`
	Resume      *resumePackResponse        `json:"resume,omitempty"`
	Health      studioBrokerHealthSnapshot `json:"health"`
}

type operatorOverviewCounts struct {
	OpenTasks       int `json:"open_tasks"`
	BlockedTasks    int `json:"blocked_tasks"`
	HumanRequests   int `json:"human_requests"`
	GovernanceItems int `json:"governance_items"`
	NextWorkItems   int `json:"next_work_items"`
}

func (b *Broker) handleOperatorOverview(w http.ResponseWriter, r *http.Request) {
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
	payload := b.buildOperatorOverviewLocked(viewer, channel, allChannels)
	b.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) buildOperatorOverviewLocked(viewer, channel string, allChannels bool) operatorOverviewResponse {
	tasks := make([]teamTask, 0, len(b.tasks))
	for _, task := range b.tasks {
		taskChannel := normalizeChannelSlug(task.Channel)
		if !allChannels && taskChannel != channel {
			continue
		}
		if !b.canAccessChannelLocked(viewer, taskChannel) || strings.TrimSpace(task.ArchivedAt) != "" {
			continue
		}
		tasks = append(tasks, task)
	}
	now := time.Now().UTC()
	next := buildOperatorActionableNextWork(tasks, now, 5)
	state := studioDevConsoleState{
		SessionMode:    b.sessionMode,
		DirectAgent:    b.oneOnOneAgent,
		FocusMode:      b.focusMode,
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
	blockers := operatorVisibleBlockers(buildStudioBlockersFromState(state), tasks)
	alerts := b.buildOperatorAlertsFromStateLocked(state, b.usage, viewer, channel, allChannels, now)
	requests := b.buildStudioRequestSnapshotsLocked(viewer, channel, allChannels, 5)
	governance := b.buildGovernanceHistoryLocked(5)
	skillRecords := make([]skillTrustRecord, 0, len(b.skills))
	for _, skill := range b.skills {
		if skill.Status != "archived" {
			skillRecords = append(skillRecords, buildSkillTrustRecord(skill))
		}
	}
	skillSummary := skillTrustSummary{Total: len(skillRecords)}
	for _, record := range skillRecords {
		switch record.Level {
		case "high":
			skillSummary.High++
		case "medium":
			skillSummary.Medium++
		default:
			skillSummary.Low++
		}
	}
	resumeChannel := channel
	if allChannels {
		resumeChannel = ""
	}
	resumeTaskID := ""
	if len(next) > 0 {
		resumeTaskID = next[0].TaskID
	}
	resume, ok := b.buildResumePackLocked(viewer, resumeTaskID, "", resumeChannel)
	var resumePtr *resumePackResponse
	if ok && resume.Task != nil {
		resumePtr = &resume
	}
	counts := operatorOverviewCounts{HumanRequests: len(requests), GovernanceItems: len(governance), NextWorkItems: len(next)}
	for _, task := range tasks {
		if operatorTaskIsBackgroundMaintenance(task) {
			continue
		}
		status := normalizeTaskStatus(task.Status)
		if status != "done" && status != "canceled" && status != "failed" {
			counts.OpenTasks++
		}
		if status == "blocked" || task.Blocked {
			counts.BlockedTasks++
		}
	}
	status := "ok"
	if counts.BlockedTasks > 0 || len(blockers) > 0 || skillSummary.Low > 0 {
		status = "degraded"
	}
	if len(blockers) > 0 && len(next) == 0 {
		status = "blocked"
	}
	if len(alerts.Alerts) > 0 && status == "ok" {
		status = "degraded"
	}
	if len(blockers) > 5 {
		blockers = blockers[:5]
	}
	return operatorOverviewResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Status:      status,
		Counts:      counts,
		Alerts:      alerts.Alerts,
		NextWork:    next,
		Blockers:    blockers,
		Requests:    requests,
		Governance:  governance,
		SkillTrust:  skillSummary,
		Resume:      resumePtr,
		Health:      studioBrokerHealthSnapshot{BrokerReachable: true, APIReachable: true, WebReachable: len(b.webUIOrigins) > 0, Degraded: status != "ok"},
	}
}

func buildOperatorActionableNextWork(tasks []teamTask, now time.Time, limit int) []workQueueItem {
	items := make([]workQueueItem, 0, len(tasks))
	for _, task := range tasks {
		status := normalizeTaskStatus(task.Status)
		if status == "done" || status == "canceled" || status == "failed" || status == "blocked" || task.Blocked {
			continue
		}
		if operatorTaskIsBackgroundMaintenance(task) {
			continue
		}
		item := workQueueItemFromTask(task)
		if strings.TrimSpace(item.QueueKey) == "" {
			item.QueueKey = "active"
		}
		if item.QueueKey == "blocked" {
			item.QueueKey = "active"
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return workQueueItemLess(items[i], items[j], now) })
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items
}

func operatorTaskIsBackgroundMaintenance(task teamTask) bool {
	title := strings.ToLower(strings.TrimSpace(task.Title))
	details := strings.ToLower(strings.TrimSpace(task.Details))
	owner := normalizeActorSlug(task.Owner)
	if owner == "watchdog" {
		return true
	}
	if strings.Contains(title, "validate unanswered ceo follow-up") {
		return true
	}
	if strings.Contains(details, "a ceo follow-up is still waiting") && strings.Contains(details, "automatic error recovery") {
		return true
	}
	return false
}

func operatorVisibleBlockers(blockers []studioBlocker, tasks []teamTask) []studioBlocker {
	taskByID := make(map[string]teamTask, len(tasks))
	for _, task := range tasks {
		taskByID[strings.TrimSpace(task.ID)] = task
	}
	out := make([]studioBlocker, 0, len(blockers))
	for _, blocker := range blockers {
		if task, ok := taskByID[strings.TrimSpace(blocker.TaskID)]; ok && operatorTaskIsBackgroundMaintenance(task) {
			continue
		}
		if blocker.Kind == "task_blocked_by_dependency" {
			task, ok := taskByID[strings.TrimSpace(blocker.TaskID)]
			if !ok {
				continue
			}
			pending := operatorPendingDependencies(task, taskByID)
			if len(pending) == 0 {
				continue
			}
			blocker.WaitingOn = strings.Join(pending, ", ")
			blocker.Reason = "Waiting on dependencies: " + strings.Join(pending, ", ") + "."
		}
		out = append(out, blocker)
	}
	return out
}

func operatorPendingDependencies(task teamTask, taskByID map[string]teamTask) []string {
	pending := make([]string, 0, len(task.DependsOn))
	for _, depID := range task.DependsOn {
		depID = strings.TrimSpace(depID)
		if depID == "" {
			continue
		}
		dep, ok := taskByID[depID]
		if !ok {
			pending = append(pending, depID+" (missing)")
			continue
		}
		if operatorTaskIsBackgroundMaintenance(dep) {
			continue
		}
		switch normalizeTaskStatus(dep.Status) {
		case "done":
			continue
		default:
			pending = append(pending, depID)
		}
	}
	return pending
}

func (b *Broker) buildStudioRequestSnapshotsLocked(viewer, channel string, allChannels bool, limit int) []studioRequestSnapshot {
	out := make([]studioRequestSnapshot, 0, limit)
	for _, req := range b.requests {
		reqChannel := normalizeChannelSlug(req.Channel)
		if req.ArchivedAt != "" || requestIsResolvedLocked(b.requests, req.ID) {
			continue
		}
		if !allChannels && reqChannel != channel {
			continue
		}
		if !b.canAccessChannelLocked(viewer, reqChannel) {
			continue
		}
		out = append(out, studioRequestSnapshot{
			ID:       req.ID,
			Kind:     req.Kind,
			Status:   req.Status,
			Channel:  reqChannel,
			From:     req.From,
			Title:    req.Title,
			Question: truncateSummary(req.Question, 180),
			Blocking: req.Blocking,
			Required: req.Required,
			ReplyTo:  req.ReplyTo,
		})
		if len(out) >= limit {
			break
		}
	}
	return out
}
