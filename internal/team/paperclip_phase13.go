package team

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type operationalTriageResponse struct {
	GeneratedAt string                    `json:"generated_at"`
	Summary     map[string]int            `json:"summary"`
	Items       []operationalTriageItem   `json:"items"`
	Actions     []operationalTriageAction `json:"actions,omitempty"`
}

type operationalTriageItem struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Category string `json:"category"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Summary  string `json:"summary,omitempty"`
	TaskID   string `json:"task_id,omitempty"`
	Channel  string `json:"channel,omitempty"`
	NextStep string `json:"next_step,omitempty"`
}

type operationalTriageAction struct {
	Action string `json:"action"`
	Label  string `json:"label"`
	Count  int    `json:"count,omitempty"`
	DryRun bool   `json:"dry_run"`
}

type noiseCleanupPreviewResponse struct {
	Persisted   bool                      `json:"persisted"`
	GeneratedAt string                    `json:"generated_at"`
	Summary     map[string]int            `json:"summary"`
	Items       []noiseCleanupPreviewItem `json:"items"`
}

type noiseCleanupPreviewItem struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	TaskID         string `json:"task_id,omitempty"`
	WatchdogID     string `json:"watchdog_id,omitempty"`
	SchedulerSlug  string `json:"scheduler_slug,omitempty"`
	Channel        string `json:"channel,omitempty"`
	Title          string `json:"title"`
	Reason         string `json:"reason"`
	WouldAction    string `json:"would_action"`
	Safe           bool   `json:"safe"`
	RequiresReview bool   `json:"requires_review,omitempty"`
}

type previewApplyRequest struct {
	Preview      string   `json:"preview"`
	ItemIDs      []string `json:"item_ids,omitempty"`
	Actor        string   `json:"actor,omitempty"`
	Reason       string   `json:"reason,omitempty"`
	Confirm      bool     `json:"confirm"`
	Confirmation string   `json:"confirmation,omitempty"`
}

type previewApplyResponse struct {
	Persisted            bool                 `json:"persisted"`
	Preview              string               `json:"preview"`
	Applied              int                  `json:"applied"`
	Skipped              int                  `json:"skipped,omitempty"`
	RequiredConfirmation string               `json:"required_confirmation,omitempty"`
	Changes              []previewApplyChange `json:"changes,omitempty"`
	RollbackPlan         string               `json:"rollback_plan,omitempty"`
	Message              string               `json:"message,omitempty"`
}

type previewApplyChange struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Field  string `json:"field"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	Reason string `json:"reason,omitempty"`
}

func (b *Broker) handleApplyPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req previewApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	req.Preview = strings.TrimSpace(req.Preview)
	req.Actor = firstNonEmpty(strings.TrimSpace(req.Actor), "human")
	req.Reason = strings.TrimSpace(req.Reason)
	if !req.Confirm || strings.TrimSpace(req.Confirmation) != "APPLY_PREVIEW" {
		payload := previewApplyResponse{
			Persisted:            false,
			Preview:              req.Preview,
			RequiredConfirmation: "APPLY_PREVIEW",
			Message:              "Set confirm=true and confirmation=APPLY_PREVIEW to persist this preview.",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(payload)
		return
	}
	if req.Reason == "" {
		http.Error(w, "reason required", http.StatusBadRequest)
		return
	}
	b.mu.Lock()
	payload, err := b.applyPreviewLocked(req)
	b.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) applyPreviewLocked(req previewApplyRequest) (previewApplyResponse, error) {
	selected := stringSet(req.ItemIDs)
	if len(selected) == 0 {
		return previewApplyResponse{}, fmt.Errorf("item_ids required")
	}
	switch req.Preview {
	case "noise_cleanup":
		return b.applyNoiseCleanupPreviewLocked(req, selected)
	case "skill_metadata":
		return b.applySkillMetadataPreviewLocked(req, selected)
	default:
		return previewApplyResponse{}, fmt.Errorf("unknown preview %q", req.Preview)
	}
}

func (b *Broker) applyNoiseCleanupPreviewLocked(req previewApplyRequest, selected map[string]struct{}) (previewApplyResponse, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	preview := b.buildNoiseCleanupPreviewLocked(req.Actor, "", true)
	allowed := make(map[string]noiseCleanupPreviewItem, len(preview.Items))
	for _, item := range preview.Items {
		if item.Safe && !item.RequiresReview {
			allowed[item.ID] = item
		}
	}
	changes := make([]previewApplyChange, 0)
	for i := range b.tasks {
		task := &b.tasks[i]
		itemID := "task:" + strings.TrimSpace(task.ID)
		item, ok := allowed[itemID]
		if !ok || !setContains(selected, itemID) || taskIsTerminal(task) {
			continue
		}
		before := task.Status
		task.Status = "done"
		task.UpdatedAt = now
		task.OutcomeStatus = firstNonEmpty(task.OutcomeStatus, "accepted")
		task.OutcomeEvidence = appendPreviewEvidence(task.OutcomeEvidence, req.Reason)
		changes = append(changes, previewApplyChange{Kind: item.Kind, ID: task.ID, Field: "status", Before: before, After: task.Status, Reason: item.Reason})
	}
	for i := range b.watchdogs {
		alert := &b.watchdogs[i]
		itemID := "watchdog:" + strings.TrimSpace(alert.ID)
		item, ok := allowed[itemID]
		if !ok || !setContains(selected, itemID) || strings.EqualFold(strings.TrimSpace(alert.Status), "resolved") {
			continue
		}
		before := alert.Status
		alert.Status = "resolved"
		alert.UpdatedAt = now
		changes = append(changes, previewApplyChange{Kind: item.Kind, ID: alert.ID, Field: "status", Before: before, After: alert.Status, Reason: item.Reason})
	}
	for i := range b.scheduler {
		job := &b.scheduler[i]
		itemID := "scheduler:" + strings.TrimSpace(job.Slug)
		item, ok := allowed[itemID]
		if !ok || !setContains(selected, itemID) || schedulerJobIsTerminal(*job) {
			continue
		}
		before := job.Status
		job.Status = "canceled"
		job.DueAt = ""
		job.NextRun = ""
		job.LastRun = now
		job.LastFinishedAt = now
		changes = append(changes, previewApplyChange{Kind: item.Kind, ID: job.Slug, Field: "status", Before: before, After: job.Status, Reason: item.Reason})
	}
	return b.finishPreviewApplyLocked("noise_cleanup", req, selected, changes, "Revert by restoring the previous task/watchdog/scheduler statuses from broker-state history if the cleanup hid useful work.")
}

func (b *Broker) applySkillMetadataPreviewLocked(req previewApplyRequest, selected map[string]struct{}) (previewApplyResponse, error) {
	changes := make([]previewApplyChange, 0)
	now := time.Now().UTC().Format(time.RFC3339)
	for i := range b.skills {
		skill := &b.skills[i]
		if skill.Status == "archived" {
			continue
		}
		preview := buildSkillMetadataMigrationPreview(*skill)
		itemID := "skill:" + strings.TrimSpace(preview.Name)
		if !setContains(selected, itemID) && !setContains(selected, strings.TrimSpace(preview.Name)) {
			continue
		}
		changedSkill := false
		if strings.TrimSpace(skill.PluginID) == "" {
			before := skill.PluginID
			skill.PluginID = preview.PluginID
			changes = append(changes, previewApplyChange{Kind: "skill", ID: skill.Name, Field: "plugin_id", Before: before, After: skill.PluginID, Reason: req.Reason})
			changedSkill = true
		}
		if strings.TrimSpace(skill.PluginKind) == "" {
			before := skill.PluginKind
			skill.PluginKind = preview.PluginKind
			changes = append(changes, previewApplyChange{Kind: "skill", ID: skill.Name, Field: "plugin_kind", Before: before, After: skill.PluginKind, Reason: req.Reason})
			changedSkill = true
		}
		if len(skill.Capabilities) == 0 {
			before := strings.Join(skill.Capabilities, ",")
			skill.Capabilities = append([]string(nil), preview.Capabilities...)
			changes = append(changes, previewApplyChange{Kind: "skill", ID: skill.Name, Field: "capabilities", Before: before, After: strings.Join(skill.Capabilities, ","), Reason: req.Reason})
			changedSkill = true
		}
		if strings.TrimSpace(skill.HealthStatus) == "" {
			before := skill.HealthStatus
			skill.HealthStatus = preview.HealthStatus
			changes = append(changes, previewApplyChange{Kind: "skill", ID: skill.Name, Field: "health_status", Before: before, After: skill.HealthStatus, Reason: req.Reason})
			changedSkill = true
		}
		if changedSkill {
			skill.UpdatedAt = now
		}
	}
	return b.finishPreviewApplyLocked("skill_metadata", req, selected, changes, "Revert by clearing or restoring the previous skill metadata fields from broker-state history.")
}

func (b *Broker) finishPreviewApplyLocked(preview string, req previewApplyRequest, selected map[string]struct{}, changes []previewApplyChange, rollback string) (previewApplyResponse, error) {
	payload := previewApplyResponse{
		Persisted:            false,
		Preview:              preview,
		Applied:              len(changes),
		Skipped:              len(selected) - len(changes),
		RequiredConfirmation: "APPLY_PREVIEW",
		Changes:              changes,
		RollbackPlan:         rollback,
	}
	if payload.Skipped < 0 {
		payload.Skipped = 0
	}
	if len(changes) == 0 {
		payload.Message = "No eligible preview items were applied."
		return payload, nil
	}
	b.appendActionLocked("preview_applied", "studio", "general", req.Actor, truncateSummary(preview+" applied: "+req.Reason, 140), preview)
	if err := b.saveLocked(); err != nil {
		return previewApplyResponse{}, err
	}
	payload.Persisted = true
	payload.Message = "Preview applied with explicit confirmation."
	return payload, nil
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func setContains(set map[string]struct{}, value string) bool {
	_, ok := set[strings.TrimSpace(value)]
	return ok
}

func appendPreviewEvidence(current, reason string) string {
	line := "Preview cleanup applied: " + strings.TrimSpace(reason)
	if strings.TrimSpace(current) == "" {
		return line
	}
	return strings.TrimSpace(current) + "\n" + line
}

func (b *Broker) handleNoiseCleanupPreview(w http.ResponseWriter, r *http.Request) {
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
	payload := b.buildNoiseCleanupPreviewLocked(viewer, channel, allChannels)
	b.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) buildNoiseCleanupPreviewLocked(viewer, channel string, allChannels bool) noiseCleanupPreviewResponse {
	items := make([]noiseCleanupPreviewItem, 0)
	visibleTask := func(task teamTask) bool {
		taskChannel := normalizeChannelSlug(task.Channel)
		if taskChannel == "" {
			taskChannel = "general"
		}
		if !allChannels && taskChannel != channel {
			return false
		}
		return b.canAccessChannelLocked(viewer, taskChannel)
	}
	for _, task := range b.tasks {
		taskChannel := normalizeChannelSlug(task.Channel)
		if taskChannel == "" {
			taskChannel = "general"
		}
		if !visibleTask(task) || taskIsTerminal(&task) || strings.TrimSpace(task.ArchivedAt) != "" || !operatorTaskIsBackgroundMaintenance(task) {
			continue
		}
		items = append(items, noiseCleanupPreviewItem{
			ID:          "task:" + strings.TrimSpace(task.ID),
			Kind:        "background_task",
			TaskID:      strings.TrimSpace(task.ID),
			Channel:     taskChannel,
			Title:       firstNonEmpty(strings.TrimSpace(task.Title), strings.TrimSpace(task.ID)),
			Reason:      "Background watchdog/follow-up work is intentionally hidden from the primary operator queue.",
			WouldAction: "mark_done_if_obsolete",
			Safe:        true,
		})
	}
	for _, alert := range b.watchdogs {
		status := strings.ToLower(strings.TrimSpace(alert.Status))
		if status == "resolved" || status == "done" || status == "canceled" {
			continue
		}
		alertChannel := normalizeChannelSlug(alert.Channel)
		if alertChannel == "" {
			alertChannel = "general"
		}
		if !allChannels && alertChannel != channel {
			continue
		}
		if !b.canAccessChannelLocked(viewer, alertChannel) {
			continue
		}
		targetID := strings.TrimSpace(alert.TargetID)
		task := b.findTaskByIDLocked(targetID)
		if task != nil && !operatorTaskIsBackgroundMaintenance(*task) {
			continue
		}
		if task == nil && !strings.EqualFold(strings.TrimSpace(alert.TargetType), "task") {
			continue
		}
		items = append(items, noiseCleanupPreviewItem{
			ID:             "watchdog:" + strings.TrimSpace(alert.ID),
			Kind:           "watchdog",
			TaskID:         targetID,
			WatchdogID:     strings.TrimSpace(alert.ID),
			Channel:        alertChannel,
			Title:          firstNonEmpty(strings.TrimSpace(alert.Summary), strings.TrimSpace(alert.ID)),
			Reason:         "Watchdog points to background follow-up or an orphaned task reference.",
			WouldAction:    "resolve_watchdog",
			Safe:           task != nil,
			RequiresReview: task == nil,
		})
	}
	for _, job := range b.scheduler {
		if schedulerJobIsTerminal(job) {
			continue
		}
		jobChannel := normalizeChannelSlug(job.Channel)
		if jobChannel == "" {
			jobChannel = "general"
		}
		if !allChannels && jobChannel != channel {
			continue
		}
		if !b.canAccessChannelLocked(viewer, jobChannel) {
			continue
		}
		targetID := strings.TrimSpace(job.TargetID)
		task := b.findTaskByIDLocked(targetID)
		if task != nil && !operatorTaskIsBackgroundMaintenance(*task) {
			continue
		}
		if task == nil && !(strings.EqualFold(strings.TrimSpace(job.TargetType), "task") && strings.Contains(strings.ToLower(job.Kind+" "+job.Slug+" "+job.Label), "follow")) {
			continue
		}
		items = append(items, noiseCleanupPreviewItem{
			ID:             "scheduler:" + strings.TrimSpace(job.Slug),
			Kind:           "scheduler",
			TaskID:         targetID,
			SchedulerSlug:  strings.TrimSpace(job.Slug),
			Channel:        jobChannel,
			Title:          firstNonEmpty(strings.TrimSpace(job.Label), strings.TrimSpace(job.Slug)),
			Reason:         "Scheduler entry targets background follow-up or a stale follow-up task reference.",
			WouldAction:    "cancel_scheduler_job",
			Safe:           task != nil,
			RequiresReview: task == nil,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].RequiresReview != items[j].RequiresReview {
			return items[i].RequiresReview
		}
		if items[i].WouldAction != items[j].WouldAction {
			return items[i].WouldAction < items[j].WouldAction
		}
		return items[i].ID < items[j].ID
	})
	summary := map[string]int{"total": len(items)}
	for _, item := range items {
		summary[item.Kind]++
		summary[item.WouldAction]++
		if item.RequiresReview {
			summary["requires_review"]++
		} else if item.Safe {
			summary["safe"]++
		}
	}
	return noiseCleanupPreviewResponse{Persisted: false, GeneratedAt: time.Now().UTC().Format(time.RFC3339), Summary: summary, Items: items}
}

type operatorRunbookResponse struct {
	Persisted   bool                  `json:"persisted"`
	GeneratedAt string                `json:"generated_at"`
	Summary     map[string]int        `json:"summary"`
	Steps       []operatorRunbookStep `json:"steps"`
}

type operatorRunbookStep struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Reason   string `json:"reason,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	Command  string `json:"command,omitempty"`
	Severity string `json:"severity,omitempty"`
	DryRun   bool   `json:"dry_run"`
}

func (b *Broker) handleOperatorRunbook(w http.ResponseWriter, r *http.Request) {
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
	triage := b.buildOperationalTriageLocked(viewer, channel, allChannels)
	cleanup := b.buildNoiseCleanupPreviewLocked(viewer, channel, allChannels)
	b.mu.RUnlock()
	readiness := b.buildReleaseReadiness()
	payload := buildOperatorRunbook(triage, cleanup, readiness)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func buildOperatorRunbook(triage operationalTriageResponse, cleanup noiseCleanupPreviewResponse, readiness releaseReadinessResponse) operatorRunbookResponse {
	steps := make([]operatorRunbookStep, 0)
	if cleanup.Summary["total"] > 0 {
		steps = append(steps, operatorRunbookStep{
			ID:       "noise-cleanup-preview",
			Title:    "Review background noise cleanup preview",
			Reason:   itoa(cleanup.Summary["total"]) + " background entries can be inspected without changing state.",
			Endpoint: "/operator/noise-cleanup-preview?viewer_slug=human&all_channels=true",
			Severity: "low",
			DryRun:   true,
		})
	}
	if triage.Summary["environment"] > 0 {
		steps = append(steps, operatorRunbookStep{
			ID:       "environment-auth",
			Title:    "Check repository remote/auth publication blockers",
			Reason:   itoa(triage.Summary["environment"]) + " environment blocker(s) are delaying automation.",
			Command:  "git remote -v",
			Severity: "medium",
			DryRun:   true,
		})
	}
	if triage.Summary["blocked_dependency"] > 0 {
		steps = append(steps, operatorRunbookStep{
			ID:       "blocked-dependencies",
			Title:    "Inspect task dependency blockers",
			Reason:   itoa(triage.Summary["blocked_dependency"]) + " task(s) are waiting on upstream work.",
			Endpoint: "/operator/triage?viewer_slug=human&all_channels=true",
			Severity: "medium",
			DryRun:   true,
		})
	}
	for _, check := range readiness.Checks {
		if check.Status == "ok" {
			continue
		}
		step := operatorRunbookStep{
			ID:       "release-" + check.ID,
			Title:    firstNonEmpty(check.NextStep, check.Summary),
			Reason:   firstNonEmpty(check.Detail, check.Summary),
			Severity: check.Status,
			DryRun:   true,
		}
		switch check.ID {
		case "runtime-smoke":
			step.Command = "wuphf doctor --smoke --json"
		case "runtime-doctor":
			step.Endpoint = "/runtime/doctor"
		case "secret-audit":
			step.Endpoint = "/security/secret-audit"
			step.Command = "wuphf secret migrate-config"
		case "worktree-preview":
			step.Endpoint = "/runtime/worktree-preview"
		case "git-status":
			step.Command = "git status --short"
		default:
			step.Endpoint = "/release/readiness"
		}
		steps = append(steps, step)
	}
	summary := map[string]int{"steps": len(steps), "dry_run": len(steps)}
	if readiness.Status != "" {
		summary["release_score"] = readiness.Score
	}
	for _, step := range steps {
		if step.Severity != "" {
			summary[step.Severity]++
		}
	}
	return operatorRunbookResponse{Persisted: false, GeneratedAt: time.Now().UTC().Format(time.RFC3339), Summary: summary, Steps: steps}
}

func (b *Broker) handleOperationalTriage(w http.ResponseWriter, r *http.Request) {
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
	payload := b.buildOperationalTriageLocked(viewer, channel, allChannels)
	b.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) buildOperationalTriageLocked(viewer, channel string, allChannels bool) operationalTriageResponse {
	overview := b.buildOperatorOverviewLocked(viewer, channel, allChannels)
	items := make([]operationalTriageItem, 0)
	for _, blocker := range overview.Blockers {
		items = append(items, triageItemFromBlocker(blocker))
	}
	for _, task := range b.tasks {
		taskChannel := normalizeChannelSlug(task.Channel)
		if !allChannels && taskChannel != channel {
			continue
		}
		if !b.canAccessChannelLocked(viewer, taskChannel) || taskIsTerminal(&task) || strings.TrimSpace(task.ArchivedAt) != "" {
			continue
		}
		if operatorTaskIsBackgroundMaintenance(task) {
			items = append(items, operationalTriageItem{
				ID:       "task:" + task.ID,
				Kind:     "background_follow_up",
				Category: "noise",
				Severity: "low",
				Title:    task.Title,
				Summary:  "Automatic watchdog/follow-up work is hidden from the primary operator queue.",
				TaskID:   task.ID,
				Channel:  taskChannel,
				NextStep: "Let the follow-up dedupe guard coalesce it, or inspect only if it keeps recurring.",
			})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if triageSeverityScore(items[i].Severity) != triageSeverityScore(items[j].Severity) {
			return triageSeverityScore(items[i].Severity) > triageSeverityScore(items[j].Severity)
		}
		if items[i].Category != items[j].Category {
			return items[i].Category < items[j].Category
		}
		return items[i].ID < items[j].ID
	})
	summary := map[string]int{}
	for _, item := range items {
		summary[item.Category]++
	}
	return operationalTriageResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Summary:     summary,
		Items:       items,
		Actions: []operationalTriageAction{
			{Action: "preview_followup_dedupe", Label: "Preview follow-up dedupe", Count: summary["noise"], DryRun: true},
			{Action: "inspect_actionable", Label: "Inspect actionable blockers", Count: summary["actionable"], DryRun: true},
		},
	}
}

func triageItemFromBlocker(blocker studioBlocker) operationalTriageItem {
	category := "actionable"
	next := "Inspect the task and move the owner toward evidence or review."
	switch blocker.Kind {
	case "github_issue_publication_deferred", "github_pr_publication_deferred":
		category = "environment"
		next = "Fix repository remote/auth support, or mark publication as intentionally deferred."
	case "task_blocked_by_dependency":
		category = "blocked_dependency"
		next = "Resolve the listed dependency before waking downstream work."
	case "human_request", "request_blocking":
		category = "blocked_human"
		next = "Answer the human request."
	}
	return operationalTriageItem{
		ID:       blocker.ID,
		Kind:     blocker.Kind,
		Category: category,
		Severity: blocker.Severity,
		Title:    blocker.Title,
		Summary:  firstNonEmpty(blocker.Reason, blocker.Summary),
		TaskID:   blocker.TaskID,
		Channel:  blocker.Channel,
		NextStep: next,
	}
}

func triageSeverityScore(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical", "high":
		return 3
	case "medium", "warn":
		return 2
	default:
		return 1
	}
}

type governanceReplayResponse struct {
	Persisted       bool               `json:"persisted"`
	TargetID        string             `json:"target_id"`
	Found           bool               `json:"found"`
	Event           *governanceEvent   `json:"event,omitempty"`
	WouldRevert     []governanceChange `json:"would_revert,omitempty"`
	RequiredReviews []string           `json:"required_reviews,omitempty"`
	RollbackPlan    string             `json:"rollback_plan,omitempty"`
}

type governanceChange struct {
	Kind   string `json:"kind"`
	ID     string `json:"id,omitempty"`
	Action string `json:"action"`
	Reason string `json:"reason,omitempty"`
}

func (b *Broker) handleGovernanceReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	targetID := strings.TrimSpace(firstNonEmpty(r.URL.Query().Get("id"), r.URL.Query().Get("event_id"), r.URL.Query().Get("related_id")))
	b.mu.RLock()
	payload := b.buildGovernanceReplayLocked(targetID)
	b.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) buildGovernanceReplayLocked(targetID string) governanceReplayResponse {
	payload := governanceReplayResponse{Persisted: false, TargetID: targetID}
	for _, event := range b.buildGovernanceHistoryLocked(100) {
		if event.ID != targetID && event.RelatedID != targetID {
			continue
		}
		eventCopy := event
		payload.Found = true
		payload.Event = &eventCopy
		payload.RollbackPlan = event.RollbackPlan
		if event.RequiresTopologyAuthorization {
			payload.RequiredReviews = append(payload.RequiredReviews, "topology-authorization")
			payload.WouldRevert = append(payload.WouldRevert, governanceChange{Kind: "topology", ID: event.RelatedID, Action: "restore_snapshot", Reason: "Topology-sensitive change requires restoring from broker-state history."})
		} else if strings.Contains(strings.ToLower(event.Kind+" "+event.Summary), "skill") {
			payload.RequiredReviews = append(payload.RequiredReviews, "skill-owner")
			payload.WouldRevert = append(payload.WouldRevert, governanceChange{Kind: "skill", ID: event.RelatedID, Action: "archive_or_restore_previous", Reason: "Skills should be reverted by compensating metadata change."})
		} else {
			payload.RequiredReviews = append(payload.RequiredReviews, "operator")
			payload.WouldRevert = append(payload.WouldRevert, governanceChange{Kind: event.Kind, ID: event.RelatedID, Action: "record_compensating_action", Reason: "Dry-run only; no state mutation is applied."})
		}
		return payload
	}
	return payload
}

type skillMetadataPreviewResponse struct {
	Persisted   bool                            `json:"persisted"`
	GeneratedAt string                          `json:"generated_at"`
	Summary     map[string]int                  `json:"summary"`
	Previews    []skillMetadataMigrationPreview `json:"previews"`
}

type skillMetadataMigrationPreview struct {
	Name         string   `json:"name"`
	Title        string   `json:"title,omitempty"`
	CurrentScore int      `json:"current_score"`
	CurrentLevel string   `json:"current_level"`
	PluginID     string   `json:"plugin_id,omitempty"`
	PluginKind   string   `json:"plugin_kind,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	HealthStatus string   `json:"health_status,omitempty"`
	WouldUpdate  []string `json:"would_update,omitempty"`
}

func (b *Broker) handleSkillMetadataPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.mu.RLock()
	previews := make([]skillMetadataMigrationPreview, 0, len(b.skills))
	for _, skill := range b.skills {
		if skill.Status == "archived" {
			continue
		}
		preview := buildSkillMetadataMigrationPreview(skill)
		if len(preview.WouldUpdate) > 0 {
			previews = append(previews, preview)
		}
	}
	b.mu.RUnlock()
	sort.Slice(previews, func(i, j int) bool { return previews[i].Name < previews[j].Name })
	summary := map[string]int{"total": len(previews)}
	for _, preview := range previews {
		for _, field := range preview.WouldUpdate {
			summary[field]++
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(skillMetadataPreviewResponse{Persisted: false, GeneratedAt: time.Now().UTC().Format(time.RFC3339), Summary: summary, Previews: previews})
}

func buildSkillMetadataMigrationPreview(skill teamSkill) skillMetadataMigrationPreview {
	trust := buildSkillTrustRecord(skill)
	preview := skillMetadataMigrationPreview{
		Name:         skill.Name,
		Title:        skill.Title,
		CurrentScore: trust.Score,
		CurrentLevel: trust.Level,
		PluginID:     firstNonEmpty(skill.PluginID, "legacy-"+skillSlug(skill.Name)),
		PluginKind:   firstNonEmpty(skill.PluginKind, inferSkillPluginKind(skill)),
		Capabilities: normalizeSkillCapabilities(firstNonEmptySlice(skill.Capabilities, inferSkillCapabilities(skill))),
		HealthStatus: firstNonEmpty(skill.HealthStatus, "unknown"),
	}
	if strings.TrimSpace(skill.PluginID) == "" {
		preview.WouldUpdate = append(preview.WouldUpdate, "plugin_id")
	}
	if strings.TrimSpace(skill.PluginKind) == "" {
		preview.WouldUpdate = append(preview.WouldUpdate, "plugin_kind")
	}
	if len(skill.Capabilities) == 0 {
		preview.WouldUpdate = append(preview.WouldUpdate, "capabilities")
	}
	if strings.TrimSpace(skill.HealthStatus) == "" {
		preview.WouldUpdate = append(preview.WouldUpdate, "health_status")
	}
	return preview
}

func firstNonEmptySlice(primary, fallback []string) []string {
	if len(primary) > 0 {
		return primary
	}
	return fallback
}

func inferSkillPluginKind(skill teamSkill) string {
	if strings.TrimSpace(skill.WorkflowKey) != "" || strings.TrimSpace(skill.WorkflowDefinition) != "" {
		return "automation"
	}
	return "skill"
}

func inferSkillCapabilities(skill teamSkill) []string {
	text := strings.ToLower(skill.Name + " " + skill.Title + " " + skill.Description + " " + strings.Join(skill.Tags, " "))
	caps := []string{"skill.invoke"}
	switch {
	case strings.Contains(text, "repo"):
		caps = append(caps, "repo.context")
	case strings.Contains(text, "frontend"):
		caps = append(caps, "ui.review")
	case strings.Contains(text, "ocr") || strings.Contains(text, "midia"):
		caps = append(caps, "media.inspect")
	case strings.Contains(text, "notion"):
		caps = append(caps, "integration.notion")
	}
	return caps
}

type skillProvenancePreviewResponse struct {
	Persisted   bool                     `json:"persisted"`
	GeneratedAt string                   `json:"generated_at"`
	Summary     map[string]int           `json:"summary"`
	Previews    []skillProvenancePreview `json:"previews"`
}

type skillProvenancePreview struct {
	Name          string   `json:"name"`
	Title         string   `json:"title,omitempty"`
	CurrentScore  int      `json:"current_score"`
	CurrentLevel  string   `json:"current_level"`
	SourceType    string   `json:"source_type,omitempty"`
	SourceRef     string   `json:"source_ref,omitempty"`
	SourceHash    string   `json:"source_hash,omitempty"`
	InstalledAt   string   `json:"installed_at,omitempty"`
	LastScannedAt string   `json:"last_scanned_at,omitempty"`
	ScanStatus    string   `json:"scan_status,omitempty"`
	ScanSummary   string   `json:"scan_summary,omitempty"`
	WouldUpdate   []string `json:"would_update,omitempty"`
}

func (b *Broker) handleSkillProvenancePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	b.mu.RLock()
	previews := make([]skillProvenancePreview, 0, len(b.skills))
	for _, skill := range b.skills {
		if skill.Status == "archived" {
			continue
		}
		preview := buildSkillProvenancePreview(skill, now)
		if len(preview.WouldUpdate) > 0 {
			previews = append(previews, preview)
		}
	}
	b.mu.RUnlock()
	sort.Slice(previews, func(i, j int) bool { return previews[i].Name < previews[j].Name })
	summary := map[string]int{"total": len(previews)}
	for _, preview := range previews {
		for _, field := range preview.WouldUpdate {
			summary[field]++
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(skillProvenancePreviewResponse{Persisted: false, GeneratedAt: now, Summary: summary, Previews: previews})
}

func buildSkillProvenancePreview(skill teamSkill, now string) skillProvenancePreview {
	proposed := skill
	fillSkillProvenanceDefaults(&proposed, now)
	applySkillProvenanceScan(&proposed, now)
	trust := buildSkillTrustRecord(skill)
	preview := skillProvenancePreview{
		Name:          skill.Name,
		Title:         skill.Title,
		CurrentScore:  trust.Score,
		CurrentLevel:  trust.Level,
		SourceType:    proposed.SourceType,
		SourceRef:     proposed.SourceRef,
		SourceHash:    proposed.SourceHash,
		InstalledAt:   proposed.InstalledAt,
		LastScannedAt: proposed.LastScannedAt,
		ScanStatus:    proposed.ScanStatus,
		ScanSummary:   proposed.ScanSummary,
	}
	if strings.TrimSpace(skill.SourceType) == "" {
		preview.WouldUpdate = append(preview.WouldUpdate, "source_type")
	}
	if strings.TrimSpace(skill.SourceRef) == "" {
		preview.WouldUpdate = append(preview.WouldUpdate, "source_ref")
	}
	if strings.TrimSpace(skill.SourceHash) == "" || strings.TrimSpace(skill.SourceHash) != proposed.SourceHash {
		preview.WouldUpdate = append(preview.WouldUpdate, "source_hash")
	}
	if strings.TrimSpace(skill.InstalledAt) == "" {
		preview.WouldUpdate = append(preview.WouldUpdate, "installed_at")
	}
	if strings.TrimSpace(skill.LastScannedAt) == "" {
		preview.WouldUpdate = append(preview.WouldUpdate, "last_scanned_at")
	}
	if normalizeSkillScanStatus(skill.ScanStatus) != proposed.ScanStatus {
		preview.WouldUpdate = append(preview.WouldUpdate, "scan_status")
	}
	if strings.TrimSpace(skill.ScanSummary) != proposed.ScanSummary {
		preview.WouldUpdate = append(preview.WouldUpdate, "scan_summary")
	}
	return preview
}

func fillSkillProvenanceDefaults(skill *teamSkill, now string) {
	if skill == nil {
		return
	}
	if strings.TrimSpace(skill.SourceType) == "" {
		skill.SourceType = inferSkillSourceType(*skill)
	} else {
		skill.SourceType = normalizeSkillSourceType(skill.SourceType)
	}
	if strings.TrimSpace(skill.SourceRef) == "" {
		skill.SourceRef = inferSkillSourceRef(*skill)
	}
	if strings.TrimSpace(skill.InstalledAt) == "" {
		skill.InstalledAt = firstNonEmpty(skill.CreatedAt, now)
	}
	skill.SourceHash = skillSourceHash(*skill)
}

func applySkillProvenanceScan(skill *teamSkill, now string) {
	if skill == nil {
		return
	}
	fillSkillProvenanceDefaults(skill, now)
	status := "passed"
	summary := "Local static scan passed."
	scannedText := skill.Content + " " + skill.Description + " " + skill.WorkflowDefinition
	if contentLooksSecretBearing(scannedText) {
		status = "warning"
		summary = "Potential secret-like terms found; review before treating this skill as trusted."
	}
	if strings.TrimSpace(skill.LastScannedAt) == "" {
		skill.LastScannedAt = now
	}
	existingStatus := normalizeSkillScanStatus(skill.ScanStatus)
	if status == "warning" && (existingStatus == "" || existingStatus == "passed" || existingStatus == "unknown") {
		skill.ScanStatus = status
	} else {
		skill.ScanStatus = normalizeSkillScanStatus(firstNonEmpty(existingStatus, status))
	}
	if strings.TrimSpace(skill.ScanSummary) == "" || skill.ScanSummary == "Local static scan passed." || status == "warning" {
		skill.ScanSummary = summary
	}
}

func skillSourceHash(skill teamSkill) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(skill.Name),
		strings.TrimSpace(skill.Title),
		strings.TrimSpace(skill.Description),
		strings.TrimSpace(skill.Content),
		strings.TrimSpace(skill.Trigger),
		strings.Join(compactStringList(skill.Tags), ","),
	}, "\n")))
	return hex.EncodeToString(sum[:])
}

func normalizeSkillSourceType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "-", "_")
	switch value {
	case "starter_pack", "task_learning", "proposal", "local", "remote", "external", "legacy_local", "manual":
		return value
	default:
		if value == "" {
			return ""
		}
		return "external"
	}
}

func normalizeSkillScanStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "passed", "warning", "blocked", "unknown":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func skillDefaultSourceType(status string) string {
	if strings.EqualFold(strings.TrimSpace(status), "proposed") {
		return "proposal"
	}
	return "local"
}

func inferSkillSourceType(skill teamSkill) string {
	switch {
	case strings.EqualFold(strings.TrimSpace(skill.PluginID), "dunderia-learning") || containsString(skill.Tags, "learning"):
		return "task_learning"
	case strings.EqualFold(strings.TrimSpace(skill.CreatedBy), "system"):
		return "starter_pack"
	case strings.EqualFold(strings.TrimSpace(skill.Status), "proposed"):
		return "proposal"
	default:
		return "legacy_local"
	}
}

func inferSkillSourceRef(skill teamSkill) string {
	for _, tag := range skill.Tags {
		if strings.HasPrefix(strings.TrimSpace(tag), "task-") {
			return strings.TrimSpace(tag)
		}
	}
	if strings.TrimSpace(skill.SourceRef) != "" {
		return strings.TrimSpace(skill.SourceRef)
	}
	return "skill:" + skillSlug(skill.Name)
}

type releaseReadinessResponse struct {
	GeneratedAt string                  `json:"generated_at"`
	Status      string                  `json:"status"`
	Score       int                     `json:"score"`
	Checks      []releaseReadinessCheck `json:"checks"`
	NextSteps   []string                `json:"next_steps,omitempty"`
}

type releaseReadinessCheck struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Summary  string `json:"summary"`
	Detail   string `json:"detail,omitempty"`
	NextStep string `json:"next_step,omitempty"`
}

func (b *Broker) handleReleaseReadiness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	payload := b.buildReleaseReadiness()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) buildReleaseReadiness() releaseReadinessResponse {
	smoke := b.buildRuntimeSmokeSnapshot()
	doctor := buildRuntimeDoctorSnapshot(copyStudioDevConsoleState(b))
	secretAudit := buildRuntimeSecretAudit()
	backup := buildRuntimeBackupPolicy()
	gitStatus, gitDetail := releaseGitStatus()
	checks := []releaseReadinessCheck{
		{ID: "runtime-smoke", Status: releaseCheckStatus(smoke.Status == "ok"), Summary: "Runtime smoke status: " + smoke.Status},
		{ID: "runtime-doctor", Status: releaseCheckStatus(doctor.Status == "ok"), Summary: "Runtime doctor status: " + doctor.Status},
		{ID: "web-dist", Status: releaseCheckStatus(strings.TrimSpace(doctor.WebDist.Issue) == ""), Summary: "Compiled web asset source: " + firstNonEmpty(doctor.WebDist.Source, "unknown"), Detail: doctor.WebDist.Issue},
		{ID: "secret-audit", Status: releaseCheckStatus(secretAudit.PlaintextConfigCount == 0), Summary: itoa(secretAudit.PlaintextConfigCount) + " plaintext config secrets", NextStep: "Migrate config secrets before public release."},
		{ID: "backup-policy", Status: releaseCheckStatus(backup.LocalSnapshotCount > 0), Summary: itoa(backup.LocalSnapshotCount) + " local broker snapshots"},
		{ID: "worktree-preview", Status: releaseCheckStatus(!buildRuntimeWorktreePreview(doctor).RequiresApproval), Summary: "Worktree contamination preview generated."},
		{ID: "git-status", Status: gitStatus, Summary: "Git status check", Detail: gitDetail, NextStep: "Review dirty files before release."},
	}
	score := 100
	var next []string
	status := "ready"
	for _, check := range checks {
		switch check.Status {
		case "fail":
			score -= 25
			status = "blocked"
			next = append(next, firstNonEmpty(check.NextStep, check.Summary))
		case "warn":
			score -= 12
			if status == "ready" {
				status = "review"
			}
			next = append(next, firstNonEmpty(check.NextStep, check.Summary))
		}
	}
	if score < 0 {
		score = 0
	}
	return releaseReadinessResponse{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Status: status, Score: score, Checks: checks, NextSteps: compactStringList(next)}
}

func releaseCheckStatus(ok bool) string {
	if ok {
		return "ok"
	}
	return "warn"
}

func releaseGitStatus() (string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "status", "--short")
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "warn", "git status timed out"
	}
	if err != nil {
		return "warn", err.Error()
	}
	lines := compactStringList(strings.Split(string(out), "\n"))
	if len(lines) == 0 {
		return "ok", "clean"
	}
	return "warn", itoa(len(lines)) + " changed paths"
}
