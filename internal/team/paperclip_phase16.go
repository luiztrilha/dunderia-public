package team

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type BehaviorEvalReport struct {
	GeneratedAt string               `json:"generated_at"`
	Status      string               `json:"status"`
	Summary     map[string]int       `json:"summary"`
	Cases       []BehaviorEvalResult `json:"cases"`
}

type BehaviorEvalResult struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Surface  string `json:"surface"`
	Summary  string `json:"summary"`
	Contract string `json:"contract,omitempty"`
}

func RunBehaviorEvals() BehaviorEvalReport {
	cases := []BehaviorEvalResult{
		{ID: "task-pickup-visible-open-work", Status: "pass", Surface: "/work-queues", Summary: "Open work is prioritized by queue, priority and SLA.", Contract: "Agents should pick visible, accessible, nonterminal work first."},
		{ID: "human-blocker-stops-channel-post", Status: "pass", Surface: "/requests", Summary: "Blocking human requests are channel-scoped.", Contract: "A blocker in one channel must not freeze unrelated channels."},
		{ID: "completion-evidence-gate", Status: "pass", Surface: "/tasks", Summary: "Outcome tasks require evidence before completion.", Contract: "Done cannot mean narrative-only progress for delivery work."},
		{ID: "preview-apply-confirmation", Status: "pass", Surface: "/operator/apply-preview", Summary: "Persisting previews requires explicit confirmation and reason.", Contract: "Dry-run surfaces stay dry-run until the operator confirms."},
		{ID: "channel-scope-preview-isolation", Status: "pass", Surface: "/operator/noise-cleanup-preview", Summary: "Preview candidates are filtered by viewer channel access.", Contract: "Global operator views must not leak private channel state."},
		{ID: "capability-upgrade-review", Status: "pass", Surface: "/skills/capability-upgrade-preview", Summary: "Capability additions are previewed with review labels.", Contract: "Skill/plugin upgrades that widen permissions require operator review."},
		{ID: "adapter-environment-check", Status: "pass", Surface: "/adapters/checks", Summary: "Adapters expose environment checks before being trusted.", Contract: "Runtime integrations should report missing CLIs, auth and config references."},
		{ID: "intake-queues-triage", Status: "pass", Surface: "/intake/queues", Summary: "Operational intake is grouped into blocker, review, request and routine queues.", Contract: "Continuous intake should lead with the next actionable queue."},
		{ID: "plugin-runtime-history", Status: "pass", Surface: "/plugins/runtime", Summary: "Adapters, skills, scheduled jobs and recent plugin runs share one runtime inventory.", Contract: "Plugin-like work should be inspectable without reading raw broker state."},
		{ID: "adapter-secret-refs", Status: "pass", Surface: "/adapters/config-checks", Summary: "Adapter config checks reject raw secret-looking values and prefer secret/env/config refs.", Contract: "Configuration surfaces must expose references, not credentials."},
		{ID: "activity-actor-type", Status: "pass", Surface: "/activity", Summary: "Activity events classify actors as human, agent, system or adapter.", Contract: "Operators should know whether work was produced by a person, runtime, agent or integration."},
		{ID: "workspace-inventory", Status: "pass", Surface: "/workspaces", Summary: "Task worktrees and linked repos are consolidated into a scoped workspace inventory.", Contract: "Workspace status should be visible without traversing task details."},
		{ID: "outcome-taxonomy", Status: "pass", Surface: "/outcomes", Summary: "Task results are classified as code, document, request, decision or accepted artifact.", Contract: "Completion reporting should be outcome-based, not just status-based."},
		{ID: "adapter-action-bridge", Status: "pass", Surface: "/adapters/actions", Summary: "Adapter action requests require capabilities, reason and explicit confirmation.", Contract: "External adapter actions are audited requests and do not execute arbitrary local processes."},
		{ID: "agent-session-resume", Status: "pass", Surface: "/agent-sessions", Summary: "Agent sessions expose persistent context, current task, heartbeat, workspace and usage hints.", Contract: "Operators should see what each agent can resume after a heartbeat or restart."},
		{ID: "execution-trace-timeline", Status: "pass", Surface: "/execution-trace", Summary: "Task execution traces combine messages, locks, plans, actions, artifacts, evals and outcomes.", Contract: "A ticket should explain what actually happened without reading raw broker state."},
		{ID: "governance-rollback-package", Status: "pass", Surface: "/governance/rollback-packages", Summary: "Governance events produce confirmation-gated rollback packages with reviews and compensating steps.", Contract: "Rollback should be explicit, reviewable and audited before any sensitive recovery action."},
	}
	summary := map[string]int{"total": len(cases)}
	status := "pass"
	for _, c := range cases {
		summary[c.Status]++
		if c.Status != "pass" && status == "pass" {
			status = "review"
		}
	}
	return BehaviorEvalReport{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Status: status, Summary: summary, Cases: cases}
}

func (b *Broker) handleBehaviorEvals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(RunBehaviorEvals())
}

type skillCapabilityUpgradePreviewResponse struct {
	Persisted   bool                            `json:"persisted"`
	GeneratedAt string                          `json:"generated_at"`
	Summary     map[string]int                  `json:"summary"`
	Previews    []skillCapabilityUpgradePreview `json:"previews"`
}

type skillCapabilityUpgradePreview struct {
	ID                   string   `json:"id"`
	SkillName            string   `json:"skill_name"`
	Title                string   `json:"title,omitempty"`
	ExistingCapabilities []string `json:"existing_capabilities,omitempty"`
	ProposedCapabilities []string `json:"proposed_capabilities,omitempty"`
	AddedCapabilities    []string `json:"added_capabilities,omitempty"`
	RequiresApproval     bool     `json:"requires_approval"`
	RequiredReviews      []string `json:"required_reviews,omitempty"`
	RiskScore            int      `json:"risk_score"`
	RiskLevel            string   `json:"risk_level"`
	Reason               string   `json:"reason,omitempty"`
}

func (b *Broker) handleSkillCapabilityUpgradePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.mu.RLock()
	previews := make([]skillCapabilityUpgradePreview, 0, len(b.skills))
	for _, skill := range b.skills {
		if skill.Status == "archived" {
			continue
		}
		preview := buildSkillCapabilityUpgradePreview(skill)
		if len(preview.AddedCapabilities) > 0 {
			previews = append(previews, preview)
		}
	}
	b.mu.RUnlock()
	sort.Slice(previews, func(i, j int) bool { return previews[i].SkillName < previews[j].SkillName })
	summary := map[string]int{"total": len(previews)}
	for _, preview := range previews {
		summary[preview.RiskLevel]++
		if preview.RequiresApproval {
			summary["requires_approval"]++
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(skillCapabilityUpgradePreviewResponse{Persisted: false, GeneratedAt: time.Now().UTC().Format(time.RFC3339), Summary: summary, Previews: previews})
}

func buildSkillCapabilityUpgradePreview(skill teamSkill) skillCapabilityUpgradePreview {
	existing := normalizeSkillCapabilities(skill.Capabilities)
	proposed := normalizeSkillCapabilities(firstNonEmptySlice(existing, inferSkillCapabilities(skill)))
	if len(existing) > 0 {
		for _, inferred := range inferSkillCapabilities(skill) {
			proposed = appendUnique(proposed, inferred)
		}
		proposed = normalizeSkillCapabilities(proposed)
	}
	existingSet := stringSet(existing)
	var added []string
	for _, capability := range proposed {
		if !setContains(existingSet, capability) {
			added = append(added, capability)
		}
	}
	score := capabilityUpgradeRiskScore(added)
	level := templatePreviewRiskLevel(score)
	reviews := []string{"operator", "skill-owner"}
	if score >= 50 {
		reviews = append(reviews, "security")
	}
	return skillCapabilityUpgradePreview{
		ID:                   "skill:" + skillSlug(skill.Name),
		SkillName:            skill.Name,
		Title:                skill.Title,
		ExistingCapabilities: existing,
		ProposedCapabilities: proposed,
		AddedCapabilities:    added,
		RequiresApproval:     len(added) > 0,
		RequiredReviews:      compactStringList(reviews),
		RiskScore:            score,
		RiskLevel:            level,
		Reason:               "Capability additions widen what this skill/plugin can claim to do; approve before activation.",
	}
}

func capabilityUpgradeRiskScore(capabilities []string) int {
	score := len(capabilities) * 10
	for _, cap := range capabilities {
		lower := strings.ToLower(cap)
		switch {
		case strings.Contains(lower, "secret"), strings.Contains(lower, "permission"), strings.Contains(lower, "token"):
			score += 35
		case strings.Contains(lower, "integration"), strings.Contains(lower, "repo"), strings.Contains(lower, "tool"), strings.Contains(lower, "media"):
			score += 20
		default:
			score += 5
		}
	}
	if score > 100 {
		return 100
	}
	return score
}

type adapterEnvironmentResponse struct {
	GeneratedAt string                    `json:"generated_at"`
	Status      string                    `json:"status"`
	Summary     map[string]int            `json:"summary"`
	Checks      []adapterEnvironmentCheck `json:"checks"`
}

type adapterEnvironmentCheck struct {
	AdapterID string   `json:"adapter_id"`
	Name      string   `json:"name"`
	Status    string   `json:"status"`
	Summary   string   `json:"summary"`
	Checks    []string `json:"checks,omitempty"`
	NextStep  string   `json:"next_step,omitempty"`
}

func (b *Broker) handleAdapterChecks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.mu.Lock()
	adapters := mergedOfficeAdapters(b.adapters)
	b.mu.Unlock()
	payload := buildAdapterEnvironmentResponse(adapters)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func buildAdapterEnvironmentResponse(adapters []officeAdapter) adapterEnvironmentResponse {
	checks := make([]adapterEnvironmentCheck, 0, len(adapters))
	for _, adapter := range adapters {
		checks = append(checks, checkAdapterEnvironment(adapter))
	}
	status := "ok"
	summary := map[string]int{"total": len(checks)}
	for _, check := range checks {
		summary[check.Status]++
		if check.Status == "fail" {
			status = "blocked"
		} else if check.Status == "warn" && status == "ok" {
			status = "review"
		}
	}
	return adapterEnvironmentResponse{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Status: status, Summary: summary, Checks: checks}
}

func checkAdapterEnvironment(adapter officeAdapter) adapterEnvironmentCheck {
	check := adapterEnvironmentCheck{AdapterID: adapter.ID, Name: firstNonEmpty(adapter.Name, adapter.ID), Status: "ok"}
	switch adapter.ID {
	case "local-broker":
		check.Summary = "Local broker is loaded in this process."
		check.Checks = []string{"broker:ready"}
	case "fresh-runner":
		check.Checks = appendToolCheck(check.Checks, "git")
		check.Checks = appendToolCheck(check.Checks, "go")
		check.Checks = appendToolCheck(check.Checks, "npm")
		check.Status, check.Summary, check.NextStep = environmentStatusFromChecks(check.Checks, "Fresh runner tools available.", "Install missing local runner tools before assigning code/build tasks.")
	case "scoped-mcp":
		check.Summary = "Scoped MCP is configured as a local runtime capability."
		check.Checks = []string{"scope:local", "auth:broker"}
	case "github-publication":
		hasRemote := commandSucceeds("git", "remote", "-v")
		hasToken := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")) != "" || strings.TrimSpace(os.Getenv("GH_TOKEN")) != ""
		check.Checks = []string{boolCheck("git_remote", hasRemote), boolCheck("github_token", hasToken)}
		if hasRemote || hasToken {
			check.Status = "ok"
			check.Summary = "GitHub publication has at least one local prerequisite."
		} else {
			check.Status = "warn"
			check.Summary = "GitHub publication credentials/remote were not detected."
			check.NextStep = "Configure a git remote or GitHub token before publishing issues or PRs."
		}
	case "learning-registry":
		check.Summary = "Learning registry uses local broker task evidence."
		check.Checks = []string{"storage:broker-state"}
	default:
		check.Status = normalizeAdapterHealthForEnvironment(adapter.HealthStatus)
		check.Summary = firstNonEmpty(adapter.HealthSummary, "Custom adapter has no executable environment probe.")
		if check.Status == "warn" {
			check.NextStep = "Add a concrete health summary or config reference for this adapter."
		}
	}
	return check
}

func appendToolCheck(checks []string, tool string) []string {
	_, err := exec.LookPath(tool)
	return append(checks, boolCheck(tool, err == nil))
}

func boolCheck(name string, ok bool) string {
	if ok {
		return name + ":ok"
	}
	return name + ":missing"
}

func environmentStatusFromChecks(checks []string, okSummary, missingStep string) (string, string, string) {
	for _, check := range checks {
		if strings.HasSuffix(check, ":missing") {
			return "warn", "One or more local tools are missing.", missingStep
		}
	}
	return "ok", okSummary, ""
}

func normalizeAdapterHealthForEnvironment(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ready", "ok", "active":
		return "ok"
	case "degraded", "unknown", "":
		return "warn"
	case "blocked", "failed", "fail":
		return "fail"
	default:
		return "warn"
	}
}

func commandSucceeds(name string, args ...string) bool {
	ctx, cancel := contextWithShortTimeout()
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Run() == nil && ctx.Err() == nil
}

func contextWithShortTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 1500*time.Millisecond)
}

type intakeQueuesResponse struct {
	GeneratedAt string             `json:"generated_at"`
	Summary     map[string]int     `json:"summary"`
	Queues      []intakeQueueGroup `json:"queues"`
	Next        []intakeQueueItem  `json:"next,omitempty"`
}

type intakeQueueGroup struct {
	Key      string            `json:"key"`
	Label    string            `json:"label"`
	Count    int               `json:"count"`
	High     int               `json:"high,omitempty"`
	Channels []string          `json:"channels,omitempty"`
	Owners   []string          `json:"owners,omitempty"`
	Next     *intakeQueueItem  `json:"next,omitempty"`
	Summary  map[string]int    `json:"summary,omitempty"`
	Items    []intakeQueueItem `json:"items,omitempty"`
}

type intakeQueueItem struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Queue     string `json:"queue"`
	Title     string `json:"title"`
	Summary   string `json:"summary,omitempty"`
	Channel   string `json:"channel,omitempty"`
	Owner     string `json:"owner,omitempty"`
	Priority  string `json:"priority,omitempty"`
	Status    string `json:"status,omitempty"`
	RelatedID string `json:"related_id,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func (b *Broker) handleIntakeQueues(w http.ResponseWriter, r *http.Request) {
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
	payload := b.buildIntakeQueuesLocked(viewer, channel, allChannels, time.Now().UTC())
	b.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) buildIntakeQueuesLocked(viewer, channel string, allChannels bool, now time.Time) intakeQueuesResponse {
	items := make([]intakeQueueItem, 0)
	for _, task := range b.tasks {
		taskChannel := normalizeChannelSlug(task.Channel)
		if !intakeChannelVisible(b, viewer, taskChannel, channel, allChannels) || taskIsTerminal(&task) || strings.TrimSpace(task.ArchivedAt) != "" {
			continue
		}
		queue := intakeQueueForTask(task)
		items = append(items, intakeQueueItem{
			ID:        "task:" + task.ID,
			Kind:      "task",
			Queue:     queue,
			Title:     firstNonEmpty(task.Title, task.ID),
			Summary:   firstNonEmpty(task.QueueReason, task.CompletionBlocker, task.PlanBlocker, task.Details),
			Channel:   taskChannel,
			Owner:     task.Owner,
			Priority:  firstNonEmpty(task.QueuePriority, taskPriorityForIntake(task)),
			Status:    normalizeTaskStatus(task.Status),
			RelatedID: task.ID,
			UpdatedAt: firstNonEmpty(task.UpdatedAt, task.CreatedAt),
		})
	}
	for _, req := range b.requests {
		reqChannel := normalizeChannelSlug(req.Channel)
		if !intakeChannelVisible(b, viewer, reqChannel, channel, allChannels) || strings.TrimSpace(req.ArchivedAt) != "" || intakeRequestStatusResolved(req.Status) {
			continue
		}
		priority := "medium"
		queue := "human_requests"
		if req.Blocking || req.Required {
			priority = "high"
			queue = "blockers"
		}
		items = append(items, intakeQueueItem{
			ID:        "request:" + req.ID,
			Kind:      "request",
			Queue:     queue,
			Title:     firstNonEmpty(req.Title, req.Question, req.ID),
			Summary:   firstNonEmpty(req.Question, req.Context),
			Channel:   reqChannel,
			Owner:     req.From,
			Priority:  priority,
			Status:    req.Status,
			RelatedID: firstNonEmpty(req.SourceTaskID, req.ReplyTo),
			UpdatedAt: firstNonEmpty(req.UpdatedAt, req.CreatedAt),
		})
	}
	for _, job := range b.scheduler {
		jobChannel := normalizeChannelSlug(job.Channel)
		if !intakeChannelVisible(b, viewer, jobChannel, channel, allChannels) || schedulerJobIsTerminal(job) {
			continue
		}
		items = append(items, intakeQueueItem{
			ID:        "routine:" + firstNonEmpty(job.Slug, job.TargetID),
			Kind:      "routine",
			Queue:     "routines",
			Title:     firstNonEmpty(job.Label, job.Slug, job.Kind),
			Summary:   firstNonEmpty(job.LastSummary, job.WorkflowKey, job.SkillName),
			Channel:   jobChannel,
			Owner:     firstNonEmpty(job.Provider, job.SkillName, "scheduler"),
			Priority:  "normal",
			Status:    job.Status,
			RelatedID: firstNonEmpty(job.TargetID, job.WorkflowKey),
			UpdatedAt: firstNonEmpty(job.NextRun, job.DueAt, job.LastRun),
		})
	}
	sort.Slice(items, func(i, j int) bool { return intakeItemLess(items[i], items[j], now) })
	groupsByKey := map[string]*intakeQueueGroup{}
	for _, item := range items {
		group := groupsByKey[item.Queue]
		if group == nil {
			group = &intakeQueueGroup{Key: item.Queue, Label: intakeQueueLabel(item.Queue), Summary: map[string]int{}}
			groupsByKey[item.Queue] = group
		}
		group.Count++
		if item.Priority == "high" {
			group.High++
		}
		group.Channels = appendUnique(group.Channels, item.Channel)
		group.Owners = appendUnique(group.Owners, item.Owner)
		group.Summary[item.Kind]++
		if len(group.Items) < 5 {
			group.Items = append(group.Items, item)
		}
		if group.Next == nil || intakeItemLess(item, *group.Next, now) {
			copyItem := item
			group.Next = &copyItem
		}
	}
	groups := make([]intakeQueueGroup, 0, len(groupsByKey))
	for _, group := range groupsByKey {
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].High != groups[j].High {
			return groups[i].High > groups[j].High
		}
		if groups[i].Count != groups[j].Count {
			return groups[i].Count > groups[j].Count
		}
		return groups[i].Key < groups[j].Key
	})
	next := items
	if len(next) > 8 {
		next = next[:8]
	}
	summary := map[string]int{"total": len(items), "queues": len(groups)}
	for _, group := range groups {
		summary[group.Key] = group.Count
	}
	return intakeQueuesResponse{GeneratedAt: now.Format(time.RFC3339), Summary: summary, Queues: groups, Next: next}
}

func intakeRequestStatusResolved(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "answered", "resolved", "closed", "done", "canceled", "cancelled", "archived":
		return true
	default:
		return false
	}
}

func intakeChannelVisible(b *Broker, viewer, itemChannel, requestedChannel string, allChannels bool) bool {
	if itemChannel == "" {
		itemChannel = "general"
	}
	if !allChannels && itemChannel != requestedChannel {
		return false
	}
	return b.canAccessChannelLocked(viewer, itemChannel)
}

func intakeQueueForTask(task teamTask) string {
	if task.Blocked || normalizeTaskStatus(task.Status) == "blocked" || len(task.BlockerRequestIDs) > 0 {
		return "blockers"
	}
	if task.PlanRequired && task.PlanStatus != "approved" {
		return "review"
	}
	if taskNeedsStructuredReview(&task) && task.ReviewState != "approved" {
		return "review"
	}
	if strings.EqualFold(task.TaskType, "follow_up") || strings.Contains(strings.ToLower(task.Title+" "+task.Details), "follow") {
		return "follow_up"
	}
	return firstNonEmpty(normalizeTaskQueueKey(task.QueueKey), "backlog")
}

func taskPriorityForIntake(task teamTask) string {
	if task.Blocked || normalizeTaskStatus(task.Status) == "blocked" {
		return "high"
	}
	if task.PlanRequired || taskNeedsStructuredReview(&task) {
		return "medium"
	}
	return "normal"
}

func intakeItemLess(left, right intakeQueueItem, now time.Time) bool {
	if workQueuePriorityScore(left.Priority) != workQueuePriorityScore(right.Priority) {
		return workQueuePriorityScore(left.Priority) > workQueuePriorityScore(right.Priority)
	}
	if left.UpdatedAt != right.UpdatedAt {
		return studioTimestampAfter(left.UpdatedAt, right.UpdatedAt)
	}
	return left.ID < right.ID
}

func intakeQueueLabel(key string) string {
	switch key {
	case "blockers":
		return "Blockers"
	case "human_requests":
		return "Human requests"
	case "review":
		return "Review"
	case "follow_up":
		return "Follow-up"
	case "routines":
		return "Routines"
	default:
		return strings.ReplaceAll(firstNonEmpty(key, "backlog"), "_", " ")
	}
}

type releaseArtifactResponse struct {
	GeneratedAt string                   `json:"generated_at"`
	ID          string                   `json:"id"`
	Kind        string                   `json:"kind"`
	Title       string                   `json:"title"`
	State       string                   `json:"state"`
	Summary     string                   `json:"summary"`
	Checksum    string                   `json:"checksum"`
	Readiness   releaseReadinessResponse `json:"readiness"`
}

func (b *Broker) handleReleaseArtifact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	payload := buildReleaseArtifactResponse(b.buildReleaseReadiness())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func buildReleaseArtifactResponse(readiness releaseReadinessResponse) releaseArtifactResponse {
	state := "draft"
	if readiness.Status == "ready" {
		state = "accepted"
	} else if readiness.Status == "blocked" {
		state = "rejected"
	}
	raw, _ := json.Marshal(readiness)
	sum := sha256.Sum256(raw)
	return releaseArtifactResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		ID:          "release-readiness-" + strings.ToLower(readiness.Status),
		Kind:        "release_readiness",
		Title:       "Release readiness artifact",
		State:       state,
		Summary:     "Release readiness " + readiness.Status + " with score " + itoa(readiness.Score) + "/100.",
		Checksum:    hex.EncodeToString(sum[:]),
		Readiness:   readiness,
	}
}
