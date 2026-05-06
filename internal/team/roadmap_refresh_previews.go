package team

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

type schedulerRevisionsPreviewResponse struct {
	GeneratedAt    string                         `json:"generated_at"`
	Persisted      bool                           `json:"persisted"`
	Status         string                         `json:"status"`
	Summary        map[string]int                 `json:"summary"`
	Jobs           []schedulerRevisionsPreviewJob `json:"jobs"`
	Policies       []string                       `json:"policies"`
	BlockedActions []string                       `json:"blocked_actions"`
	NextStep       string                         `json:"next_step"`
}

type schedulerRevisionsPreviewJob struct {
	Slug             string   `json:"slug,omitempty"`
	Label            string   `json:"label,omitempty"`
	Kind             string   `json:"kind,omitempty"`
	Channel          string   `json:"channel,omitempty"`
	Status           string   `json:"status,omitempty"`
	LatestRevisionID string   `json:"latest_revision_id,omitempty"`
	RevisionCount    int      `json:"revision_count"`
	RestoreEnabled   bool     `json:"restore_enabled"`
	RestoreReadiness string   `json:"restore_readiness"`
	MissingPolicies  []string `json:"missing_policies,omitempty"`
	RequiredPolicies []string `json:"required_policies,omitempty"`
	RiskSignals      []string `json:"risk_signals,omitempty"`
	NextStep         string   `json:"next_step,omitempty"`
}

type wikiEditorPreviewResponse struct {
	GeneratedAt string                   `json:"generated_at"`
	Persisted   bool                     `json:"persisted"`
	Status      string                   `json:"status"`
	Summary     map[string]int           `json:"summary"`
	Modes       []wikiEditorPreviewMode  `json:"modes"`
	Checks      []wikiEditorPreviewCheck `json:"checks"`
	NextStep    string                   `json:"next_step"`
}

type wikiEditorPreviewMode struct {
	ID            string   `json:"id"`
	Label         string   `json:"label"`
	Readiness     string   `json:"readiness"`
	EditorEnabled bool     `json:"editor_enabled"`
	RiskSignals   []string `json:"risk_signals,omitempty"`
	NextStep      string   `json:"next_step,omitempty"`
}

type wikiEditorPreviewCheck struct {
	ID        string   `json:"id"`
	Status    string   `json:"status"`
	Summary   string   `json:"summary"`
	Contracts []string `json:"contracts,omitempty"`
	NextStep  string   `json:"next_step,omitempty"`
}

type providerCompatibilityPreviewResponse struct {
	GeneratedAt     string                         `json:"generated_at"`
	Persisted       bool                           `json:"persisted"`
	Status          string                         `json:"status"`
	Summary         map[string]int                 `json:"summary"`
	Providers       []providerCompatibilityPreview `json:"providers"`
	MutationEnabled bool                           `json:"mutation_enabled"`
	NextStep        string                         `json:"next_step"`
}

type providerCompatibilityPreview struct {
	Provider              string   `json:"provider"`
	Readiness             string   `json:"readiness"`
	KnownEventShapes      []string `json:"known_event_shapes,omitempty"`
	CompatibilityChecks   []string `json:"compatibility_checks,omitempty"`
	MissingTests          []string `json:"missing_tests,omitempty"`
	RiskSignals           []string `json:"risk_signals,omitempty"`
	ParserChangeSensitive bool     `json:"parser_change_sensitive"`
	NextStep              string   `json:"next_step,omitempty"`
}

type projectOverviewWidgetsPreviewResponse struct {
	GeneratedAt     string                         `json:"generated_at"`
	Persisted       bool                           `json:"persisted"`
	Status          string                         `json:"status"`
	Summary         map[string]int                 `json:"summary"`
	Widgets         []projectOverviewWidgetPreview `json:"widgets"`
	MutationEnabled bool                           `json:"mutation_enabled"`
	NextStep        string                         `json:"next_step"`
}

type projectOverviewWidgetPreview struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Kind             string   `json:"kind"`
	Readiness        string   `json:"readiness"`
	Source           string   `json:"source,omitempty"`
	Count            int      `json:"count,omitempty"`
	QueryEnabled     bool     `json:"query_enabled"`
	MutationEnabled  bool     `json:"mutation_enabled"`
	RequiredPolicies []string `json:"required_policies,omitempty"`
	MissingPolicies  []string `json:"missing_policies,omitempty"`
	RiskSignals      []string `json:"risk_signals,omitempty"`
	NextStep         string   `json:"next_step,omitempty"`
}

type fileContextHandoffPreviewResponse struct {
	GeneratedAt        string                      `json:"generated_at"`
	Persisted          bool                        `json:"persisted"`
	Status             string                      `json:"status"`
	Summary            map[string]int              `json:"summary"`
	ContentReadEnabled bool                        `json:"content_read_enabled"`
	BlockedActions     []string                    `json:"blocked_actions"`
	Items              []fileContextHandoffPreview `json:"items"`
	NextStep           string                      `json:"next_step"`
}

type fileContextHandoffPreview struct {
	ID              string   `json:"id"`
	TaskID          string   `json:"task_id"`
	TaskTitle       string   `json:"task_title,omitempty"`
	Channel         string   `json:"channel,omitempty"`
	Source          string   `json:"source"`
	Path            string   `json:"path,omitempty"`
	URL             string   `json:"url,omitempty"`
	Summary         string   `json:"summary,omitempty"`
	ContentIncluded bool     `json:"content_included"`
	MissingPolicies []string `json:"missing_policies,omitempty"`
	RiskSignals     []string `json:"risk_signals,omitempty"`
	NextStep        string   `json:"next_step,omitempty"`
	UpdatedAt       string   `json:"updated_at,omitempty"`
}

func (b *Broker) handleSchedulerRevisionsPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	allChannels := channel == "" || strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all_channels")), "true")
	b.mu.RLock()
	payload := b.buildSchedulerRevisionsPreviewLocked(viewer, channel, allChannels)
	b.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) handleWikiEditorPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	payload := buildWikiEditorPreview()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) handleProviderCompatibilityPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	payload := buildProviderCompatibilityPreview()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) handleProjectOverviewWidgetsPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.mu.RLock()
	payload := b.buildProjectOverviewWidgetsPreviewLocked()
	b.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) handleFileContextHandoffPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	taskID := strings.TrimSpace(r.URL.Query().Get("task_id"))
	limit := parsePositiveLimit(r.URL.Query().Get("limit"), 40)
	allChannels := channel == "" || strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all_channels")), "true")
	b.mu.RLock()
	items := b.buildFileContextHandoffsLocked(viewer, channel, allChannels, taskID)
	b.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt != items[j].UpdatedAt {
			return studioTimestampAfter(items[i].UpdatedAt, items[j].UpdatedAt)
		}
		return items[i].ID < items[j].ID
	})
	if len(items) > limit {
		items = items[:limit]
	}
	summary := map[string]int{"total": len(items)}
	status := "ok"
	for _, item := range items {
		summary[item.Source]++
		if len(item.MissingPolicies) > 0 {
			summary["review"]++
			status = "review"
		}
		for _, signal := range item.RiskSignals {
			summary["risk_"+signal]++
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(fileContextHandoffPreviewResponse{
		GeneratedAt:        time.Now().UTC().Format(time.RFC3339),
		Persisted:          false,
		Status:             status,
		Summary:            summary,
		ContentReadEnabled: false,
		BlockedActions:     []string{"attach_file_to_prompt", "read_file_contents", "drag_drop_prompt_injection"},
		Items:              items,
		NextStep:           "Keep file context as path/URL references until file scope, secret scan, and prompt-injection review policies are implemented.",
	})
}

func (b *Broker) buildSchedulerRevisionsPreviewLocked(viewer, channel string, allChannels bool) schedulerRevisionsPreviewResponse {
	requiredPolicies := []string{"routine_revision_store", "conflict_aware_save", "dirty_edit_blocking", "restore_confirmation", "rollback_audit"}
	blockedActions := []string{"write_revision", "restore_revision", "replace_routine"}
	jobs := make([]schedulerRevisionsPreviewJob, 0, len(b.scheduler))
	for _, raw := range b.scheduler {
		job := normalizeSchedulerJob(raw)
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
		jobs = append(jobs, schedulerRevisionsPreviewJob{
			Slug:             strings.TrimSpace(job.Slug),
			Label:            strings.TrimSpace(firstNonEmpty(job.Label, job.Slug, job.TargetID)),
			Kind:             strings.TrimSpace(job.Kind),
			Channel:          jobChannel,
			Status:           strings.TrimSpace(firstNonEmpty(job.Status, job.LastStatus, "active")),
			RevisionCount:    0,
			RestoreEnabled:   false,
			RestoreReadiness: "blocked",
			MissingPolicies:  append([]string(nil), requiredPolicies...),
			RequiredPolicies: append([]string(nil), requiredPolicies...),
			RiskSignals:      []string{"no_revision_history", "restore_disabled", "preview_only"},
			NextStep:         "Design append-only routine revisions, conflict-aware save, dirty-edit blocking, restore confirmation, and rollback audit before enabling revision writes.",
		})
	}
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].Channel != jobs[j].Channel {
			return jobs[i].Channel < jobs[j].Channel
		}
		return jobs[i].Slug < jobs[j].Slug
	})
	summary := map[string]int{"total": len(jobs), "restore_disabled": len(jobs), "blocked": len(jobs), "missing_policies": len(requiredPolicies)}
	return schedulerRevisionsPreviewResponse{
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		Persisted:      false,
		Status:         "blocked",
		Summary:        summary,
		Jobs:           jobs,
		Policies:       requiredPolicies,
		BlockedActions: blockedActions,
		NextStep:       "Keep scheduler revision work read-only until persistence, restore, conflict, and audit contracts are reviewed.",
	}
}

func buildWikiEditorPreview() wikiEditorPreviewResponse {
	modes := []wikiEditorPreviewMode{
		{
			ID:            "source_markdown",
			Label:         "Source markdown",
			Readiness:     "review",
			EditorEnabled: false,
			RiskSignals:   []string{"preview_only", "no_article_apply"},
			NextStep:      "Reuse wiki preview/lint projections and add markdown round-trip tests before exposing edits.",
		},
		{
			ID:            "rich_markdown",
			Label:         "Rich markdown",
			Readiness:     "blocked",
			EditorEnabled: false,
			RiskSignals:   []string{"round_trip_unproven", "draft_restore_missing", "conflict_detection_missing"},
			NextStep:      "Keep rich editing disabled until wikilink/code-region preservation, draft restore, conflict behavior, and accessibility checks are proven.",
		},
	}
	checks := []wikiEditorPreviewCheck{
		{ID: "markdown_round_trip", Status: "missing", Summary: "Editor output must round-trip to the same markdown article.", Contracts: []string{"no_unreviewed_format_rewrite", "stable_frontmatter"}},
		{ID: "wikilink_preservation", Status: "missing", Summary: "Wiki links must survive rich/source mode switches.", Contracts: []string{"preserve_wikilinks", "lint_broken_backlinks"}},
		{ID: "code_region_safety", Status: "missing", Summary: "Fenced code and generated command snippets must not be reformatted or executed.", Contracts: []string{"code_block_opaque", "no_command_execution"}},
		{ID: "draft_restore", Status: "missing", Summary: "Unsaved article drafts need a local restore contract before editing is useful.", Contracts: []string{"draft_snapshot", "explicit_discard"}},
		{ID: "conflict_detection", Status: "missing", Summary: "Concurrent edits must block save and show a merge/review path.", Contracts: []string{"base_revision_check", "conflict_aware_save"}},
		{ID: "accessibility_labels", Status: "missing", Summary: "Toolbar controls, mode switch, and conflict banners need labels before UI exposure.", Contracts: []string{"keyboard_toolbar", "announced_conflicts"}},
	}
	summary := map[string]int{"modes": len(modes), "checks": len(checks)}
	status := "blocked"
	for _, mode := range modes {
		summary["mode_"+mode.Readiness]++
	}
	for _, check := range checks {
		summary[check.Status]++
	}
	return wikiEditorPreviewResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Persisted:   false,
		Status:      status,
		Summary:     summary,
		Modes:       modes,
		Checks:      checks,
		NextStep:    "Keep wiki editing disabled; prove markdown round-trip, wikilink/code safety, draft restore, conflicts, and accessibility before adding an editor.",
	}
}

func buildProviderCompatibilityPreview() providerCompatibilityPreviewResponse {
	providers := []providerCompatibilityPreview{
		{
			Provider:              "codex",
			Readiness:             "review",
			KnownEventShapes:      []string{"sse_token_batch", "large_tool_call_arguments", "mid_stream_error_chunk", "response_completed"},
			CompatibilityChecks:   []string{"request_size_limit", "large_argument_trimming", "stream_error_mapping"},
			MissingTests:          []string{"codex_stream_error_chunk", "codex_large_tool_argument_trim"},
			RiskSignals:           []string{"parser_contract_unpinned"},
			ParserChangeSensitive: true,
			NextStep:              "Add stable parser fixtures before changing Codex stream handling.",
		},
		{
			Provider:              "gemini",
			Readiness:             "blocked",
			KnownEventShapes:      []string{"gemini_cli_v0_38_stream_json", "message_status_stats", "legacy_assistant_result"},
			CompatibilityChecks:   []string{"stream_json_parser", "cli_formatter", "ui_status_projection"},
			MissingTests:          []string{"gemini_v0_38_stream_json_parser", "gemini_legacy_result_compat"},
			RiskSignals:           []string{"known_cli_format_drift", "parser_contract_unpinned"},
			ParserChangeSensitive: true,
			NextStep:              "Capture Gemini v0.38 stream-json fixtures before editing provider execution.",
		},
		{
			Provider:              "claude-code",
			Readiness:             "review",
			KnownEventShapes:      []string{"resume_session_detection", "tool_use_events", "permission_prompt"},
			CompatibilityChecks:   []string{"resume_detection", "tool_use_projection", "permission_needed_status"},
			MissingTests:          []string{"claude_resume_detection_fixture", "claude_permission_prompt_projection"},
			RiskSignals:           []string{"provider_hook_assumptions"},
			ParserChangeSensitive: true,
			NextStep:              "Pin hook/event fixtures before changing normalized session status.",
		},
		{
			Provider:              "ollama",
			Readiness:             "review",
			KnownEventShapes:      []string{"local_chat_stream", "non_tool_text_delta", "model_unavailable_error"},
			CompatibilityChecks:   []string{"local_stream_text", "model_error_mapping"},
			MissingTests:          []string{"ollama_model_unavailable_error", "ollama_stream_text_delta"},
			RiskSignals:           []string{"local_model_variance"},
			ParserChangeSensitive: false,
			NextStep:              "Keep Ollama routing covered by local stream and model-unavailable fixtures.",
		},
	}
	summary := map[string]int{"total": len(providers)}
	status := "ok"
	for _, provider := range providers {
		summary[provider.Readiness]++
		summary["missing_tests"] += len(provider.MissingTests)
		if provider.Readiness == "blocked" {
			status = "blocked"
		} else if provider.Readiness == "review" && status == "ok" {
			status = "review"
		}
	}
	return providerCompatibilityPreviewResponse{
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		Persisted:       false,
		Status:          status,
		Summary:         summary,
		Providers:       providers,
		MutationEnabled: false,
		NextStep:        "Add provider wire-format fixtures and parser compatibility tests before changing runtime provider execution.",
	}
}

func (b *Broker) buildProjectOverviewWidgetsPreviewLocked() projectOverviewWidgetsPreviewResponse {
	openTasks, blockedTasks, workspaceCount, issueCount, prCount := 0, 0, 0, 0, 0
	workspaceSeen := map[string]struct{}{}
	for _, task := range b.tasks {
		status := normalizeTaskStatus(task.Status)
		if !roadmapTaskStatusIsTerminal(firstNonEmpty(status, task.Status)) {
			openTasks++
		}
		if task.Blocked || status == "blocked" {
			blockedTasks++
		}
		for _, path := range []string{strings.TrimSpace(task.WorkspacePath), strings.TrimSpace(task.WorktreePath)} {
			if path == "" {
				continue
			}
			workspaceSeen[path] = struct{}{}
		}
		if task.IssuePublication != nil {
			issueCount++
		}
		if task.PRPublication != nil {
			prCount++
		}
	}
	workspaceCount = len(workspaceSeen)
	widgets := []projectOverviewWidgetPreview{
		{ID: "readiness", Title: "Readiness", Kind: "status", Readiness: "ready", Source: "operator_overview", Count: blockedTasks, QueryEnabled: true, MutationEnabled: false, RiskSignals: []string{"read_only"}},
		{ID: "provider_tools", Title: "Provider tools", Kind: "diagnostic", Readiness: "review", Source: "provider_compatibility_preview", Count: 4, QueryEnabled: true, MutationEnabled: false, RequiredPolicies: []string{"parser_fixture_policy"}, MissingPolicies: []string{"parser_fixture_policy"}, RiskSignals: []string{"wire_format_drift"}},
		{ID: "github_prs", Title: "GitHub PRs", Kind: "github", Readiness: "review", Source: "task_publication_state", Count: prCount, QueryEnabled: false, MutationEnabled: false, RequiredPolicies: []string{"gh_readonly_policy", "linked_repo_policy"}, MissingPolicies: []string{"gh_readonly_policy", "linked_repo_policy"}, RiskSignals: []string{"external_command_disabled"}},
		{ID: "github_issues", Title: "GitHub issues", Kind: "github", Readiness: "review", Source: "task_publication_state", Count: issueCount, QueryEnabled: false, MutationEnabled: false, RequiredPolicies: []string{"gh_readonly_policy", "linked_repo_policy"}, MissingPolicies: []string{"gh_readonly_policy", "linked_repo_policy"}, RiskSignals: []string{"external_command_disabled"}},
		{ID: "workspaces", Title: "Workspaces", Kind: "workspace", Readiness: "ready", Source: "task_workspace_paths", Count: workspaceCount, QueryEnabled: true, MutationEnabled: false, RiskSignals: []string{"path_reference_only"}},
		{ID: "active_tasks", Title: "Active tasks", Kind: "task", Readiness: "ready", Source: "broker_tasks", Count: openTasks, QueryEnabled: true, MutationEnabled: false, RiskSignals: []string{"read_only"}},
	}
	sort.Slice(widgets, func(i, j int) bool { return widgets[i].ID < widgets[j].ID })
	summary := map[string]int{"total": len(widgets), "open_tasks": openTasks, "blocked_tasks": blockedTasks, "workspaces": workspaceCount}
	status := "ok"
	for _, widget := range widgets {
		summary[widget.Readiness]++
		summary["kind_"+widget.Kind]++
		if widget.Readiness == "blocked" {
			status = "blocked"
		} else if widget.Readiness == "review" && status == "ok" {
			status = "review"
		}
	}
	return projectOverviewWidgetsPreviewResponse{
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		Persisted:       false,
		Status:          status,
		Summary:         summary,
		Widgets:         widgets,
		MutationEnabled: false,
		NextStep:        "Keep project overview widgets as a read-only manifest until layout persistence, linked repo reads, and widget permissions are reviewed.",
	}
}

func roadmapTaskStatusIsTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "done", "completed", "canceled", "cancelled":
		return true
	default:
		return false
	}
}

func (b *Broker) buildFileContextHandoffsLocked(viewer, channel string, allChannels bool, taskID string) []fileContextHandoffPreview {
	items := make([]fileContextHandoffPreview, 0)
	for _, task := range b.tasks {
		if taskID != "" && strings.TrimSpace(task.ID) != taskID {
			continue
		}
		taskChannel := normalizeChannelSlug(task.Channel)
		if taskChannel == "" {
			taskChannel = "general"
		}
		if !allChannels && taskChannel != channel {
			continue
		}
		if !b.canAccessChannelLocked(viewer, taskChannel) {
			continue
		}
		if strings.TrimSpace(task.ArchivedAt) != "" {
			continue
		}
		for _, ref := range taskFileReferenceItems(task) {
			items = append(items, ref)
		}
	}
	return items
}

func taskFileReferenceItems(task teamTask) []fileContextHandoffPreview {
	items := make([]fileContextHandoffPreview, 0)
	for _, artifact := range task.Artifacts {
		path := strings.TrimSpace(artifact.Path)
		url := strings.TrimSpace(firstNonEmpty(artifact.URL, artifact.PreviewURL))
		if path == "" && url == "" {
			continue
		}
		source := "artifact"
		if normalizeTaskArtifactKind(artifact.Kind) == "browser_inspection" {
			source = "browser_inspection"
		}
		items = append(items, buildFileContextReference(task, source, firstNonEmpty(artifact.ID, artifact.Title, artifact.Kind), path, url, firstNonEmpty(artifact.Summary, artifact.Title, artifact.Kind), firstNonEmpty(artifact.UpdatedAt, artifact.CreatedAt, task.UpdatedAt, task.CreatedAt)))
	}
	for _, ref := range []struct {
		source string
		path   string
	}{
		{source: "workspace", path: strings.TrimSpace(task.WorkspacePath)},
		{source: "worktree", path: strings.TrimSpace(task.WorktreePath)},
	} {
		if ref.path == "" {
			continue
		}
		items = append(items, buildFileContextReference(task, ref.source, ref.source, ref.path, "", ref.path, firstNonEmpty(task.UpdatedAt, task.CreatedAt)))
	}
	return items
}

func buildFileContextReference(task teamTask, source, refID, path, url, summary, updatedAt string) fileContextHandoffPreview {
	missingPolicies := []string{"file_scope_policy", "secret_scan_policy", "prompt_injection_review"}
	signals := []string{"content_not_loaded", "preview_only"}
	if path != "" {
		signals = append(signals, "path_reference_only")
	}
	if url != "" {
		signals = append(signals, "url_reference_only")
	}
	if contentLooksSecretBearing(path + " " + url + " " + summary) {
		signals = append(signals, "secret_like_reference")
	}
	return fileContextHandoffPreview{
		ID:              fmt.Sprintf("file-context:%s:%s:%s", strings.TrimSpace(task.ID), source, normalizeChannelSlug(firstNonEmpty(refID, path, url, "ref"))),
		TaskID:          strings.TrimSpace(task.ID),
		TaskTitle:       strings.TrimSpace(task.Title),
		Channel:         normalizeChannelSlug(task.Channel),
		Source:          source,
		Path:            strings.TrimSpace(path),
		URL:             strings.TrimSpace(url),
		Summary:         truncateSummary(summary, 220),
		ContentIncluded: false,
		MissingPolicies: missingPolicies,
		RiskSignals:     compactStringList(signals),
		NextStep:        "Show this as a reference only; do not inject file contents into prompts until scope and secret-scan policies are in place.",
		UpdatedAt:       strings.TrimSpace(updatedAt),
	}
}
