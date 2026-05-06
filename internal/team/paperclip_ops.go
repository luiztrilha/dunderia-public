package team

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type runtimeWorktreePreview struct {
	Persisted        bool                            `json:"persisted"`
	CanonicalRepo    string                          `json:"canonical_repo,omitempty"`
	RuntimeHome      string                          `json:"runtime_home,omitempty"`
	Signals          []runtimeDoctorQuarantineSignal `json:"signals,omitempty"`
	Actions          []runtimeWorktreePreviewAction  `json:"actions,omitempty"`
	RequiresApproval bool                            `json:"requires_approval,omitempty"`
	Summary          string                          `json:"summary,omitempty"`
}

type runtimeWorktreePreviewAction struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Mutating    bool     `json:"mutating"`
	TaskIDs     []string `json:"task_ids,omitempty"`
	Path        string   `json:"path,omitempty"`
	Description string   `json:"description,omitempty"`
}

func (b *Broker) handleRuntimeWorktreePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state := copyStudioDevConsoleState(b)
	snapshot := buildRuntimeDoctorSnapshot(state)
	preview := buildRuntimeWorktreePreview(snapshot)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(preview)
}

func buildRuntimeWorktreePreview(snapshot runtimeDoctorSnapshot) runtimeWorktreePreview {
	repo, _ := os.Getwd()
	preview := runtimeWorktreePreview{
		Persisted:     false,
		CanonicalRepo: filepath.Clean(repo),
		RuntimeHome:   strings.TrimSpace(snapshot.RuntimeHome),
		Signals:       append([]runtimeDoctorQuarantineSignal(nil), snapshot.QuarantineSignals...),
	}
	for _, signal := range snapshot.QuarantineSignals {
		switch signal.Kind {
		case "duplicate_active_worktree":
			preview.Actions = append(preview.Actions, runtimeWorktreePreviewAction{
				ID:          "split-active-worktree",
				Label:       "Split active tasks into isolated worktrees",
				Mutating:    true,
				TaskIDs:     append([]string(nil), signal.TaskIDs...),
				Path:        signal.Path,
				Description: "Dry-run only: create or assign distinct execution lanes before waking agents.",
			})
		case "working_directory_in_task_worktree", "runtime_home_in_task_worktree":
			preview.Actions = append(preview.Actions, runtimeWorktreePreviewAction{
				ID:          "restart-from-canonical-repo",
				Label:       "Restart from canonical repo",
				Mutating:    false,
				Path:        signal.Path,
				Description: firstNonEmpty(signal.NextStep, "Restart the office from the canonical repository."),
			})
		}
	}
	preview.RequiresApproval = hasMutatingPreviewAction(preview.Actions)
	if len(preview.Signals) == 0 {
		preview.Summary = "No worktree isolation repair is currently needed."
	} else {
		preview.Summary = "Worktree isolation repair preview generated without mutating task or topology state."
	}
	return preview
}

func hasMutatingPreviewAction(actions []runtimeWorktreePreviewAction) bool {
	for _, action := range actions {
		if action.Mutating {
			return true
		}
	}
	return false
}

func (b *Broker) handleRuntimeRestartAdvice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	snapshot := buildRuntimeDoctorSnapshot(copyStudioDevConsoleState(b))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snapshot.RestartAdvice)
}

func (b *Broker) handleSecretAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(buildRuntimeSecretAudit())
}

func (b *Broker) handleBackupPolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(buildRuntimeBackupPolicy())
}

type humanSessionSnapshot struct {
	ViewerSlug         string   `json:"viewer_slug"`
	Channel            string   `json:"channel,omitempty"`
	CanAccessChannel   bool     `json:"can_access_channel"`
	CanReadAllChannels bool     `json:"can_read_all_channels"`
	Capabilities       []string `json:"capabilities,omitempty"`
	GeneratedAt        string   `json:"generated_at"`
}

type humanPermissionsPreviewResponse struct {
	GeneratedAt string                           `json:"generated_at"`
	Persisted   bool                             `json:"persisted"`
	ViewerSlug  string                           `json:"viewer_slug"`
	Channel     string                           `json:"channel,omitempty"`
	Summary     map[string]int                   `json:"summary"`
	Snapshots   []humanPermissionChannelSnapshot `json:"snapshots"`
}

type humanPermissionChannelSnapshot struct {
	ID                string                      `json:"id"`
	ViewerSlug        string                      `json:"viewer_slug"`
	Channel           string                      `json:"channel"`
	AccessLevel       string                      `json:"access_level"`
	CanRead           bool                        `json:"can_read"`
	CanAnswerRequests bool                        `json:"can_answer_requests"`
	CanReviewTasks    bool                        `json:"can_review_tasks"`
	CanApproveActions bool                        `json:"can_approve_actions"`
	CanMutateTopology bool                        `json:"can_mutate_topology"`
	Capabilities      []humanPermissionCapability `json:"capabilities,omitempty"`
	Signals           []string                    `json:"signals,omitempty"`
	NextStep          string                      `json:"next_step,omitempty"`
}

type humanPermissionCapability struct {
	Name           string `json:"name"`
	Status         string `json:"status"`
	Scope          string `json:"scope,omitempty"`
	Mutating       bool   `json:"mutating,omitempty"`
	RequiresReview bool   `json:"requires_review,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

func (b *Broker) handleHumanSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := normalizeActorSlug(r.URL.Query().Get("viewer_slug"))
	if viewer == "" {
		viewer = "human"
	}
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	if channel == "" {
		channel = "general"
	}
	b.mu.RLock()
	canAccess := b.canAccessChannelLocked(viewer, channel)
	canAll := b.canAccessChannelLocked(viewer, "general")
	b.mu.RUnlock()
	caps := []string{"read:self", "requests:answer", "tasks:review"}
	if canAll {
		caps = append(caps, "channels:read-accessible")
	}
	payload := humanSessionSnapshot{
		ViewerSlug:         viewer,
		Channel:            channel,
		CanAccessChannel:   canAccess,
		CanReadAllChannels: canAll,
		Capabilities:       caps,
		GeneratedAt:        time.Now().UTC().Format(time.RFC3339),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) handleHumanPermissionsPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := normalizeActorSlug(r.URL.Query().Get("viewer_slug"))
	if viewer == "" {
		viewer = "human"
	}
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	allChannels := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all_channels")), "true")
	if channel == "" && !allChannels {
		channel = "general"
	}
	b.mu.RLock()
	payload := b.buildHumanPermissionsPreviewLocked(viewer, channel, allChannels)
	b.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) buildHumanPermissionsPreviewLocked(viewer, channel string, allChannels bool) humanPermissionsPreviewResponse {
	viewer = normalizeActorSlug(viewer)
	if viewer == "" {
		viewer = "human"
	}
	channel = normalizeChannelSlug(channel)
	channels := make([]teamChannel, 0)
	if allChannels || channel == "" {
		for _, ch := range b.channels {
			if normalizeChannelSlug(ch.Slug) == "" || ch.Archived {
				continue
			}
			if !b.canAccessChannelLocked(viewer, ch.Slug) {
				continue
			}
			channels = append(channels, ch)
		}
	} else if ch := b.findChannelLocked(channel); ch != nil && b.canAccessChannelLocked(viewer, channel) {
		channels = append(channels, *ch)
	}
	if len(channels) == 0 && channel != "" {
		channels = append(channels, teamChannel{Slug: channel, Name: channel})
	}
	sort.Slice(channels, func(i, j int) bool {
		return normalizeChannelSlug(channels[i].Slug) < normalizeChannelSlug(channels[j].Slug)
	})
	snapshots := make([]humanPermissionChannelSnapshot, 0, len(channels))
	for _, ch := range channels {
		snapshots = append(snapshots, b.buildHumanPermissionChannelSnapshotLocked(viewer, ch))
	}
	summary := map[string]int{"total": len(snapshots)}
	for _, snapshot := range snapshots {
		summary["access_"+snapshot.AccessLevel]++
		if snapshot.CanRead {
			summary["can_read"]++
		}
		if snapshot.CanAnswerRequests {
			summary["can_answer_requests"]++
		}
		if snapshot.CanApproveActions {
			summary["can_approve_actions"]++
		}
		if !snapshot.CanMutateTopology {
			summary["topology_blocked"]++
		}
	}
	return humanPermissionsPreviewResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Persisted:   false,
		ViewerSlug:  viewer,
		Channel:     channel,
		Summary:     summary,
		Snapshots:   snapshots,
	}
}

func (b *Broker) buildHumanPermissionChannelSnapshotLocked(viewer string, ch teamChannel) humanPermissionChannelSnapshot {
	channel := normalizeChannelSlug(ch.Slug)
	canRead := b.canAccessChannelLocked(viewer, channel)
	canAct := canRead && humanViewerCanAct(viewer)
	accessLevel := "none"
	if canRead {
		accessLevel = "read_only"
	}
	if canAct {
		accessLevel = "approve"
	}
	snapshot := humanPermissionChannelSnapshot{
		ID:                normalizeExecutionKey(viewer + ":" + channel),
		ViewerSlug:        viewer,
		Channel:           channel,
		AccessLevel:       accessLevel,
		CanRead:           canRead,
		CanAnswerRequests: canAct,
		CanReviewTasks:    canRead,
		CanApproveActions: canAct,
		CanMutateTopology: false,
	}
	if ch.Protected {
		snapshot.Signals = appendUnique(snapshot.Signals, "protected_channel")
	}
	if !canRead {
		snapshot.Signals = appendUnique(snapshot.Signals, "no_channel_access")
		snapshot.NextStep = "Grant channel membership through a protected topology change before this viewer can read the channel."
	} else if !canAct {
		snapshot.Signals = appendUnique(snapshot.Signals, "read_only_viewer")
		snapshot.NextStep = "Use this viewer for read-only inspection; approvals and human decisions should stay with an operator identity."
	} else {
		snapshot.Signals = appendUnique(snapshot.Signals, "operator_review_required_for_mutations")
		snapshot.NextStep = "Viewer may answer requests and review tasks, but topology changes still require an explicit protected-topology workflow."
	}
	snapshot.Capabilities = []humanPermissionCapability{
		humanPermissionCapabilityFor("channel.read", canRead, "channel", false, false, "Viewer can read channel-scoped surfaces when channel access is available."),
		humanPermissionCapabilityFor("request.answer", canAct, "channel", true, true, "Human request answers are explicit operator decisions, not silent automation."),
		humanPermissionCapabilityFor("task.review", canRead, "channel", false, false, "Viewer can inspect task/review state for accessible channels."),
		humanPermissionCapabilityFor("action.approve", canAct, "channel", true, true, "Approval-like actions stay explicit and auditable."),
		humanPermissionCapabilityFor("topology.mutate", false, "office", true, true, "Protected topology cannot be changed by this preview."),
	}
	return snapshot
}

func humanViewerCanAct(viewer string) bool {
	viewer = normalizeActorSlug(viewer)
	return viewer == "" || viewer == "human" || viewer == "you" || viewer == "ceo"
}

func humanPermissionCapabilityFor(name string, available bool, scope string, mutating bool, requiresReview bool, reason string) humanPermissionCapability {
	status := "blocked"
	if available {
		status = "available"
	}
	return humanPermissionCapability{
		Name:           name,
		Status:         status,
		Scope:          scope,
		Mutating:       mutating,
		RequiresReview: requiresReview,
		Reason:         reason,
	}
}

type runtimeSmokeSnapshot struct {
	Status      string              `json:"status"`
	GeneratedAt string              `json:"generated_at"`
	Checks      []runtimeSmokeCheck `json:"checks"`
}

type runtimeSmokeCheck struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Summary  string `json:"summary"`
	Detail   string `json:"detail,omitempty"`
	NextStep string `json:"next_step,omitempty"`
}

func (b *Broker) handleRuntimeSmoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	payload := b.buildRuntimeSmokeSnapshot()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) buildRuntimeSmokeSnapshot() runtimeSmokeSnapshot {
	state := copyStudioDevConsoleState(b)
	doctor := buildRuntimeDoctorSnapshot(state)
	queues := buildWorkQueueSnapshot(state.Tasks, currentTaskGoalContext(), time.Now().UTC())
	checks := []runtimeSmokeCheck{
		{
			ID:      "doctor",
			Status:  smokeStatusFromDoctor(doctor.Status),
			Summary: "Runtime doctor status: " + doctor.Status,
		},
		{
			ID:      "web-dist",
			Status:  smokeStatus(strings.TrimSpace(doctor.WebDist.Issue) == ""),
			Summary: "Web dist source: " + firstNonEmpty(doctor.WebDist.Source, "unknown"),
			Detail:  doctor.WebDist.Issue,
		},
		{
			ID:      "work-queues",
			Status:  smokeStatus(len(queues.Queues) >= 0),
			Summary: "Work queue snapshot generated.",
			Detail:  "queues=" + itoa(len(queues.Queues)) + " next=" + itoa(len(queues.Next)),
		},
		{
			ID:      "backup-policy",
			Status:  smokeStatus(buildRuntimeBackupPolicy().RetentionDays >= 0),
			Summary: "Backup policy snapshot generated.",
		},
		{
			ID:       "secret-audit",
			Status:   smokeStatus(buildRuntimeSecretAudit().PlaintextConfigCount == 0),
			Summary:  "Secret audit snapshot generated.",
			NextStep: "Plaintext config secrets should be migrated with `wuphf secret migrate-config --write`.",
		},
	}
	status := "ok"
	for _, check := range checks {
		if check.Status == "fail" {
			status = "blocked"
			break
		}
		if check.Status == "warn" && status == "ok" {
			status = "degraded"
		}
	}
	return runtimeSmokeSnapshot{
		Status:      status,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Checks:      checks,
	}
}

func smokeStatus(ok bool) string {
	if ok {
		return "ok"
	}
	return "warn"
}

func smokeStatusFromDoctor(status string) string {
	switch strings.TrimSpace(status) {
	case "ok":
		return "ok"
	case "blocked":
		return "fail"
	default:
		return "warn"
	}
}
