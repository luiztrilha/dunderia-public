package team

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestPaperclipPhase13OperationalTriageClassifiesNoiseAndEnvironment(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{{
		ID:        "task-noise",
		Channel:   "general",
		Title:     "Validate unanswered CEO follow-up resumed",
		Details:   "A CEO follow-up is still waiting.\n\nAutomatic error recovery.",
		Owner:     "watchdog",
		Status:    "in_progress",
		TaskType:  "follow_up",
		CreatedAt: "2026-04-30T10:00:00Z",
		UpdatedAt: "2026-04-30T10:00:00Z",
	}, {
		ID:        "task-pub",
		Channel:   "general",
		Title:     "Publish task",
		Owner:     "backend",
		Status:    "review",
		CreatedAt: "2026-04-30T10:00:00Z",
		UpdatedAt: "2026-04-30T10:00:00Z",
		IssuePublication: &taskGitHubPublication{
			Status:    "deferred",
			LastError: "origin remote is not supported",
		},
	}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/operator/triage?viewer_slug=human&channel=general", nil)
	b.handleOperationalTriage(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload operationalTriageResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Summary["noise"] == 0 || payload.Summary["environment"] == 0 {
		t.Fatalf("expected noise and environment categories, got %+v", payload)
	}
}

func TestPaperclipPhase13GovernanceReplayIsDryRun(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	b.orgProposals = []orgProposal{{
		ID:                            "proposal-replay",
		Kind:                          "channel",
		Title:                         "Create releases channel",
		TargetType:                    "channel",
		TargetID:                      "releases",
		ProposedChange:                "create channel releases",
		Status:                        "approved",
		RequiresTopologyAuthorization: true,
		CreatedAt:                     "2026-04-30T10:00:00Z",
		UpdatedAt:                     "2026-04-30T10:01:00Z",
	}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/governance/replay?id=proposal-replay", nil)
	b.handleGovernanceReplay(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload governanceReplayResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Persisted || !payload.Found || len(payload.WouldRevert) == 0 || len(payload.RequiredReviews) == 0 {
		t.Fatalf("expected dry-run replay, got %+v", payload)
	}
}

func TestPaperclipPhase13SkillMetadataPreviewSuggestsLegacyFields(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	b.skills = []teamSkill{{
		ID:        "skill-legacy",
		Name:      "repo-helper",
		Title:     "Repo Helper",
		Status:    "active",
		CreatedAt: "2026-04-30T10:00:00Z",
		UpdatedAt: "2026-04-30T10:00:00Z",
	}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/skills/metadata-preview", nil)
	b.handleSkillMetadataPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload skillMetadataPreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Persisted || len(payload.Previews) != 1 || !stringSliceContains(payload.Previews[0].Capabilities, "repo.context") {
		t.Fatalf("expected metadata preview with inferred repo capability, got %+v", payload)
	}
}

func TestSkillProvenancePreviewSuggestsSourceHashAndScan(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	b.skills = []teamSkill{{
		ID:          "skill-legacy",
		Name:        "repo-helper",
		Title:       "Repo Helper",
		Description: "Reusable repository workflow.",
		Content:     "Inspect the repository and report findings.",
		SourceHash:  "stale",
		Status:      "active",
		CreatedAt:   "2026-05-04T10:00:00Z",
		UpdatedAt:   "2026-05-04T10:00:00Z",
	}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/skills/provenance-preview", nil)
	b.handleSkillProvenancePreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload skillProvenancePreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Persisted || len(payload.Previews) != 1 {
		t.Fatalf("expected one dry-run provenance preview, got %+v", payload)
	}
	preview := payload.Previews[0]
	for _, field := range []string{"source_type", "source_ref", "source_hash", "installed_at", "last_scanned_at", "scan_status", "scan_summary"} {
		if !stringSliceContains(preview.WouldUpdate, field) {
			t.Fatalf("expected field %q in provenance preview, got %+v", field, preview.WouldUpdate)
		}
	}
	if preview.SourceType != "legacy_local" || preview.SourceHash == "" || preview.ScanStatus != "passed" {
		t.Fatalf("unexpected provenance preview: %+v", preview)
	}
	if preview.SourceHash == "stale" {
		t.Fatalf("expected stale source hash to be refreshed in preview")
	}
}

func TestSkillProvenancePreviewPromotesSecretLikeContentToWarning(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	b.skills = []teamSkill{{
		ID:          "skill-risky",
		Name:        "risky-helper",
		Title:       "Risky Helper",
		Description: "Reusable repository workflow.",
		Content:     "Read API_TOKEN from local config before running.",
		ScanStatus:  "passed",
		Status:      "active",
		CreatedAt:   "2026-05-04T10:00:00Z",
		UpdatedAt:   "2026-05-04T10:00:00Z",
	}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/skills/provenance-preview", nil)
	b.handleSkillProvenancePreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload skillProvenancePreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Previews) != 1 || payload.Previews[0].ScanStatus != "warning" {
		t.Fatalf("expected warning preview for secret-like content, got %+v", payload)
	}
}

func TestPaperclipPhase13ReleaseReadinessReturnsScore(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{{ID: "task-1", Channel: "general", Title: "Release", Status: "done", CreatedAt: "2026-04-30T10:00:00Z", UpdatedAt: "2026-04-30T10:00:00Z"}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/release/readiness", nil)
	b.handleReleaseReadiness(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload releaseReadinessResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Score < 0 || payload.Score > 100 || len(payload.Checks) == 0 || payload.Status == "" {
		t.Fatalf("unexpected readiness payload: %+v", payload)
	}
}

func TestPaperclipPhase14NoiseCleanupPreviewIsDryRun(t *testing.T) {
	b := NewBroker()
	now := "2026-04-30T10:00:00Z"
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{{
		ID:        "task-noise",
		Channel:   "general",
		Title:     "Validate unanswered CEO follow-up",
		Details:   "A CEO follow-up is still waiting.\n\nAutomatic error recovery.",
		Owner:     "watchdog",
		Status:    "in_progress",
		TaskType:  "follow_up",
		CreatedAt: now,
		UpdatedAt: now,
	}}
	b.watchdogs = []watchdogAlert{{
		ID:         "watchdog-noise",
		Kind:       "task_follow_up",
		Channel:    "general",
		TargetType: "task",
		TargetID:   "task-noise",
		Status:     "active",
		Summary:    "Follow-up still open",
		CreatedAt:  now,
	}}
	b.scheduler = []schedulerJob{{
		Slug:       "follow-up-noise",
		Kind:       "task_follow_up",
		Label:      "Follow up task",
		TargetType: "task",
		TargetID:   "task-noise",
		Channel:    "general",
		Status:     "scheduled",
		NextRun:    now,
	}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/operator/noise-cleanup-preview?viewer_slug=human&channel=general", nil)
	b.handleNoiseCleanupPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload noiseCleanupPreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Persisted || payload.Summary["total"] != 3 || payload.Summary["safe"] != 3 {
		t.Fatalf("expected dry-run safe cleanup preview, got %+v", payload)
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.tasks[0].Status != "in_progress" || b.watchdogs[0].Status != "active" || b.scheduler[0].Status != "scheduled" {
		t.Fatalf("preview mutated broker state: task=%+v watchdog=%+v scheduler=%+v", b.tasks[0], b.watchdogs[0], b.scheduler[0])
	}
}

func TestPaperclipPhase14OperatorRunbookIncludesReleaseAndNoiseSteps(t *testing.T) {
	triage := operationalTriageResponse{Summary: map[string]int{"environment": 1}}
	cleanup := noiseCleanupPreviewResponse{Summary: map[string]int{"total": 2}}
	readiness := releaseReadinessResponse{
		Status: "review",
		Score:  76,
		Checks: []releaseReadinessCheck{{
			ID:       "git-status",
			Status:   "warn",
			Summary:  "Git status check",
			Detail:   "2 changed paths",
			NextStep: "Review dirty files before release.",
		}},
	}

	payload := buildOperatorRunbook(triage, cleanup, readiness)
	if payload.Persisted || payload.Summary["steps"] != 3 {
		t.Fatalf("expected dry-run runbook with 3 steps, got %+v", payload)
	}
	ids := make([]string, 0, len(payload.Steps))
	for _, step := range payload.Steps {
		if !step.DryRun {
			t.Fatalf("expected every step to be dry-run: %+v", payload.Steps)
		}
		ids = append(ids, step.ID)
	}
	for _, want := range []string{"noise-cleanup-preview", "environment-auth", "release-git-status"} {
		if !stringSliceContains(ids, want) {
			t.Fatalf("expected runbook step %q in %+v", want, payload.Steps)
		}
	}
}

func TestPaperclipPhase15ApplyPreviewRequiresExplicitConfirmation(t *testing.T) {
	b := NewBroker()
	now := "2026-04-30T10:00:00Z"
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{{
		ID:        "task-noise",
		Channel:   "general",
		Title:     "Validate unanswered CEO follow-up",
		Details:   "Automatic error recovery.",
		Owner:     "watchdog",
		Status:    "in_progress",
		TaskType:  "follow_up",
		CreatedAt: now,
		UpdatedAt: now,
	}}
	b.mu.Unlock()

	body := bytes.NewBufferString(`{"preview":"noise_cleanup","item_ids":["task:task-noise"],"actor":"human","reason":"obsolete"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/operator/apply-preview", body)
	b.handleApplyPreview(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload previewApplyResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Persisted || payload.RequiredConfirmation != "APPLY_PREVIEW" {
		t.Fatalf("expected confirmation gate, got %+v", payload)
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.tasks[0].Status != "in_progress" {
		t.Fatalf("preview gate mutated task: %+v", b.tasks[0])
	}
}

func TestPaperclipPhase15ApplyNoiseCleanupPreviewPersistsSelectedSafeItems(t *testing.T) {
	tmpDir := t.TempDir()
	oldPathFn := brokerStatePath
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	now := "2026-04-30T10:00:00Z"
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{{
		ID:        "task-noise",
		Channel:   "general",
		Title:     "Validate unanswered CEO follow-up",
		Details:   "Automatic error recovery.",
		Owner:     "watchdog",
		Status:    "in_progress",
		TaskType:  "follow_up",
		CreatedAt: now,
		UpdatedAt: now,
	}}
	b.watchdogs = []watchdogAlert{{
		ID:         "watchdog-noise",
		Kind:       "task_follow_up",
		Channel:    "general",
		TargetType: "task",
		TargetID:   "task-noise",
		Status:     "active",
		Summary:    "Follow-up still open",
		CreatedAt:  now,
	}}
	b.scheduler = []schedulerJob{{
		Slug:       "follow-up-noise",
		Kind:       "task_follow_up",
		Label:      "Follow up task",
		TargetType: "task",
		TargetID:   "task-noise",
		Channel:    "general",
		Status:     "scheduled",
		NextRun:    now,
	}}
	b.mu.Unlock()

	body := bytes.NewBufferString(`{"preview":"noise_cleanup","item_ids":["task:task-noise","watchdog:watchdog-noise","scheduler:follow-up-noise"],"actor":"human","reason":"obsolete background follow-up","confirm":true,"confirmation":"APPLY_PREVIEW"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/operator/apply-preview", body)
	b.handleApplyPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload previewApplyResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Persisted || payload.Applied != 3 {
		t.Fatalf("expected persisted preview apply, got %+v", payload)
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.tasks[0].Status != "done" || b.watchdogs[0].Status != "resolved" || b.scheduler[0].Status != "canceled" {
		t.Fatalf("expected selected cleanup applied, task=%+v watchdog=%+v scheduler=%+v", b.tasks[0], b.watchdogs[0], b.scheduler[0])
	}
}

func TestPaperclipPhase15OperatorPreviewScopesChannels(t *testing.T) {
	b := NewBroker()
	now := "2026-04-30T10:00:00Z"
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	b.tasks = []teamTask{{
		ID:        "task-general",
		Channel:   "general",
		Title:     "Validate unanswered CEO follow-up general",
		Owner:     "watchdog",
		Status:    "in_progress",
		TaskType:  "follow_up",
		CreatedAt: now,
		UpdatedAt: now,
	}, {
		ID:        "task-private",
		Channel:   "private-client",
		Title:     "Validate unanswered CEO follow-up private",
		Owner:     "watchdog",
		Status:    "in_progress",
		TaskType:  "follow_up",
		CreatedAt: now,
		UpdatedAt: now,
	}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/operator/noise-cleanup-preview?viewer_slug=builder&all_channels=true", nil)
	b.handleNoiseCleanupPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload noiseCleanupPreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Summary["total"] != 1 || len(payload.Items) != 1 || payload.Items[0].TaskID != "task-general" {
		t.Fatalf("expected channel-scoped preview, got %+v", payload)
	}
}

func TestPaperclipPhase13FollowUpUpsertCoalescesSameThread(t *testing.T) {
	b := NewBroker()
	now := time.Now().UTC().Format(time.RFC3339)
	b.mu.Lock()
	b.messages = []channelMessage{{ID: "root", Channel: "general", From: "human", Content: "Need status", Timestamp: now}}
	b.tasks = []teamTask{{
		ID:           "task-existing",
		Channel:      "general",
		ExecutionKey: "ceo-conversation-follow-up|incoming|general|old",
		Title:        "Validate unanswered CEO follow-up",
		Owner:        "ceo",
		Status:       "in_progress",
		ThreadID:     "root",
		TaskType:     "follow_up",
		PipelineID:   "follow_up",
		CreatedAt:    now,
		UpdatedAt:    now,
	}}
	before := len(b.tasks)
	changed := b.upsertCEOConversationFollowUpTaskLocked(ceoConversationFollowUpCandidate{
		ExecutionKey: "ceo-conversation-follow-up|incoming|general|new",
		Channel:      "general",
		ThreadID:     "root",
		Title:        "Validate unanswered CEO follow-up",
		Details:      "new details",
	}, now)
	after := len(b.tasks)
	task := b.tasks[0]
	b.mu.Unlock()

	if !changed || before != after || task.ExecutionKey != "ceo-conversation-follow-up|incoming|general|new" || task.Details != "new details" {
		t.Fatalf("expected follow-up to coalesce without creating task, changed=%v before=%d after=%d task=%+v", changed, before, after, task)
	}
}

func TestPaperclipPhase16BehaviorEvalsExposeContracts(t *testing.T) {
	report := RunBehaviorEvals()
	if report.Status != "pass" || report.Summary["total"] < 6 {
		t.Fatalf("unexpected eval report: %+v", report)
	}
	var found bool
	for _, c := range report.Cases {
		if c.ID == "capability-upgrade-review" && c.Surface == "/skills/capability-upgrade-preview" {
			found = true
		}
		if c.Contract == "" {
			t.Fatalf("eval case should explain contract: %+v", c)
		}
	}
	if !found {
		t.Fatalf("expected capability upgrade eval in %+v", report.Cases)
	}
}

func TestPaperclipPhase16SkillCapabilityUpgradePreviewRequiresReview(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	b.skills = []teamSkill{{
		ID:           "skill-frontend",
		Name:         "frontend-polish",
		Title:        "Frontend polish",
		Description:  "Improve frontend repo UI",
		Capabilities: []string{"skill.invoke"},
		Status:       "active",
		CreatedAt:    "2026-04-30T10:00:00Z",
		UpdatedAt:    "2026-04-30T10:00:00Z",
	}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/skills/capability-upgrade-preview", nil)
	b.handleSkillCapabilityUpgradePreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload skillCapabilityUpgradePreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Persisted || payload.Summary["requires_approval"] != 1 || len(payload.Previews) != 1 {
		t.Fatalf("unexpected capability preview: %+v", payload)
	}
	if !stringSliceContains(payload.Previews[0].AddedCapabilities, "repo.context") || !payload.Previews[0].RequiresApproval {
		t.Fatalf("expected inferred repo capability requiring approval: %+v", payload.Previews[0])
	}
}

func TestPaperclipPhase16AdapterChecksAndIntakeQueues(t *testing.T) {
	b := NewBroker()
	now := "2026-04-30T10:00:00Z"
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{{
		ID:        "task-blocked",
		Channel:   "general",
		Title:     "Resolve provider blocker",
		Owner:     "operator",
		Status:    "blocked",
		Blocked:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}, {
		ID:          "task-review",
		Channel:     "private-client",
		Title:       "Review private task",
		Owner:       "reviewer",
		Status:      "open",
		ReviewState: "needs_review",
		CreatedAt:   now,
		UpdatedAt:   now,
	}}
	b.requests = []humanInterview{{
		ID:        "request-1",
		Channel:   "general",
		Title:     "Approve next step",
		Question:  "Can we continue?",
		From:      "operator",
		Status:    "pending",
		Blocking:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}}
	b.mu.Unlock()

	adapterPayload := buildAdapterEnvironmentResponse([]officeAdapter{{ID: "local-broker", Name: "Local broker"}})
	if adapterPayload.Status != "ok" || adapterPayload.Summary["ok"] != 1 {
		t.Fatalf("unexpected adapter checks: %+v", adapterPayload)
	}
	b.mu.RLock()
	intake := b.buildIntakeQueuesLocked("human", "general", false, time.Now().UTC())
	b.mu.RUnlock()
	if intake.Summary["blockers"] != 2 {
		t.Fatalf("expected task and request blockers in general queue: %+v", intake)
	}
	for _, item := range intake.Next {
		if item.Channel == "private-client" {
			t.Fatalf("scoped intake leaked private item: %+v", intake.Next)
		}
	}
}

func TestPaperclipPhase16ReleaseArtifactIsDeterministicEnvelope(t *testing.T) {
	readiness := releaseReadinessResponse{Status: "ready", Score: 100}
	artifact := buildReleaseArtifactResponse(readiness)
	if artifact.State != "accepted" || artifact.Checksum == "" || artifact.Kind != "release_readiness" {
		t.Fatalf("unexpected release artifact: %+v", artifact)
	}
}

func TestPaperclipPhase17PluginRuntimeAndSecretRefs(t *testing.T) {
	b := NewBroker()
	now := "2026-04-30T10:00:00Z"
	b.mu.Lock()
	b.adapters = []officeAdapter{{
		ID:           "github-publication",
		Name:         "GitHub publication",
		Kind:         "integration",
		Capabilities: []string{"issue.open", "review.sync"},
		Status:       "active",
		ConfigRef:    "env:TEST_DUNDERIA_GITHUB_TOKEN",
		CreatedAt:    now,
		UpdatedAt:    now,
	}, {
		ID:        "unsafe-adapter",
		Name:      "Unsafe adapter",
		Kind:      "integration",
		Status:    "active",
		ConfigRef: "ghp_raw_secret_value_1234567890",
		CreatedAt: now,
		UpdatedAt: now,
	}}
	b.skills = []teamSkill{{
		ID:           "skill-1",
		Name:         "repo-audit",
		Title:        "Repo audit",
		PluginID:     "repo-audit",
		PluginKind:   "workflow",
		Capabilities: []string{"repo.context"},
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	}}
	b.scheduler = []schedulerJob{{Slug: "audit-nightly", SkillName: "repo-audit", Status: "scheduled", IntervalMinutes: 60, NextRun: now}}
	b.actions = []officeActionLog{{ID: "action-1", Kind: "adapter_action_requested", Source: "adapter:github-publication", Actor: "scheduler", Summary: "sync requested", CreatedAt: now}}
	runtime := b.buildPluginRuntimeLocked()
	b.mu.Unlock()

	if runtime.Summary["plugins"] < 3 || len(runtime.Jobs) != 1 || len(runtime.Runs) != 1 {
		t.Fatalf("unexpected plugin runtime inventory: %+v", runtime)
	}
	if runtime.Runs[0].ActorType != "system" {
		t.Fatalf("expected actor type on plugin run, got %+v", runtime.Runs[0])
	}
	sandbox := b.buildPluginSandboxPreviewLocked()
	if sandbox.Persisted || sandbox.Status != "blocked" || sandbox.Summary["blocked"] == 0 {
		t.Fatalf("expected read-only blocked sandbox preview, got %+v", sandbox)
	}
	var sawUnsafe bool
	var sawRepoAudit bool
	var sawNoopWorker bool
	for _, candidate := range sandbox.Candidates {
		if candidate.ID == "worker:noop-health" {
			sawNoopWorker = true
			if candidate.SandboxStatus != "ready" || candidate.WorkerClass != "noop_health" || candidate.ManifestSignature == "" {
				t.Fatalf("expected health-only noop worker to be ready with signed manifest metadata: %+v", candidate)
			}
			if candidate.NetworkPolicy != "none" || !stringSliceContains(candidate.FilesystemScope, "none") || len(candidate.SecretRefs) != 0 {
				t.Fatalf("expected noop worker to declare no network, filesystem or secrets: %+v", candidate)
			}
		}
		if candidate.ID == "adapter:unsafe-adapter" {
			sawUnsafe = true
			if !stringSliceContains(candidate.RiskSignals, "config_ref_blocked") || candidate.ConfigRef != "[redacted]" {
				t.Fatalf("expected unsafe adapter to redact and flag config risk: %+v", candidate)
			}
		}
		if candidate.ID == "skill:repo-audit" {
			sawRepoAudit = true
			if !stringSliceContains(candidate.MissingPolicies, "filesystem_scope") || !stringSliceContains(candidate.MissingPolicies, "network_policy") {
				t.Fatalf("expected repo-audit sandbox preview to require scope policies: %+v", candidate)
			}
		}
	}
	if !sawUnsafe || !sawRepoAudit || !sawNoopWorker {
		t.Fatalf("expected adapter and skill sandbox candidates, got %+v", sandbox.Candidates)
	}
	if sandbox.Summary["workers"] != 1 || sandbox.Summary["ready"] == 0 {
		t.Fatalf("expected ready no-op worker summary, got %+v", sandbox.Summary)
	}
	checks := buildAdapterConfigChecks(b.adapters)
	if checks.Status != "blocked" || checks.Summary["fail"] != 1 {
		t.Fatalf("expected raw secret config to block adapter checks: %+v", checks)
	}
	for _, check := range checks.Checks {
		if check.AdapterID == "unsafe-adapter" && check.ConfigRef != "[redacted]" {
			t.Fatalf("expected raw config ref to be redacted: %+v", check)
		}
	}
}

func TestPaperclipPhase17AdapterActionBridgeIsGoverned(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	b.adapters = []officeAdapter{{
		ID:           "github-publication",
		Name:         "GitHub publication",
		Kind:         "integration",
		Capabilities: []string{"issue.open"},
		Status:       "active",
		ConfigRef:    "config:github-publication",
	}}
	preview, err := b.applyAdapterActionLocked(adapterActionRequest{AdapterID: "github-publication", Action: "open_issue", Actor: "human"})
	if err != nil {
		t.Fatalf("preview adapter action: %v", err)
	}
	blocked, err := b.applyAdapterActionLocked(adapterActionRequest{AdapterID: "github-publication", Action: "restart_process", Actor: "human", Confirm: true, Confirmation: "ADAPTER_ACTION", Reason: "test"})
	if err != nil {
		t.Fatalf("blocked adapter action: %v", err)
	}
	b.mu.Unlock()

	if preview.Persisted || preview.Status != "preview" || preview.RequiredConfirmation != "ADAPTER_ACTION" {
		t.Fatalf("expected explicit confirmation preview, got %+v", preview)
	}
	if blocked.Status != "blocked" || len(blocked.MissingCapabilities) != 1 || blocked.MissingCapabilities[0] != "process.restart" {
		t.Fatalf("expected capability-blocked action, got %+v", blocked)
	}
}

func TestPaperclipPhase17WorkspacesAndOutcomesAreScoped(t *testing.T) {
	b := NewBroker()
	now := "2026-04-30T10:00:00Z"
	workspace := t.TempDir()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	ensureTestMemberAccess(b, "private-client", "private-agent", "Private Agent")
	b.tasks = []teamTask{{
		ID:            "task-code",
		Channel:       "general",
		Title:         "Merge feature branch",
		Owner:         "builder",
		Status:        "done",
		WorkspacePath: workspace,
		Artifacts:     []taskArtifact{{ID: "artifact-pr", Kind: "pull_request", State: "accepted", URL: "https://example.test/pr/1", CreatedAt: now, UpdatedAt: now}},
		CreatedAt:     now,
		UpdatedAt:     now,
	}, {
		ID:            "task-private",
		Channel:       "private-client",
		Title:         "Private report",
		Owner:         "private-agent",
		Status:        "done",
		WorkspacePath: filepath.Join(workspace, "private"),
		Outcome:       "private result",
		CreatedAt:     now,
		UpdatedAt:     now,
	}}
	workspaces := b.buildWorkspacesInventoryLocked("human", "general", false)
	outcomes := b.buildOutcomesLocked("human", "general", false)
	b.mu.Unlock()

	if workspaces.Summary["total"] != 1 || len(workspaces.Workspaces) != 1 || workspaces.Workspaces[0].Path != workspace {
		t.Fatalf("expected only general workspace, got %+v", workspaces)
	}
	if outcomes.Summary["merged_code"] != 1 || len(outcomes.Items) != 1 || outcomes.Items[0].Kind != "merged_code" {
		t.Fatalf("expected scoped merged-code outcome, got %+v", outcomes)
	}
}

func TestPaperclipPhase17ActivityIncludesActorType(t *testing.T) {
	b := NewBroker()
	now := "2026-04-30T10:00:00Z"
	b.mu.Lock()
	b.actions = []officeActionLog{{
		ID:        "action-adapter",
		Kind:      "adapter_action_requested",
		Source:    "adapter:github-publication",
		Channel:   "general",
		Actor:     "scheduler",
		Summary:   "sync requested",
		CreatedAt: now,
	}}
	events := b.activityEventsLocked(0)
	b.mu.Unlock()

	if len(events) != 1 || events[0].ActorType != "system" {
		t.Fatalf("expected actor_type in activity event, got %+v", events)
	}
}

func TestPaperclipPhase18AgentSessionsExposePersistentContext(t *testing.T) {
	b := NewBroker()
	now := "2026-04-30T10:00:00Z"
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	b.activity = map[string]agentActivitySnapshot{
		agentLaneKey("general", "builder"): {
			Slug:       "builder",
			Channel:    "general",
			Status:     "active",
			Activity:   "coding",
			Detail:     "Implementing trace",
			LastTime:   now,
			LivenessAt: now,
		},
	}
	b.tasks = []teamTask{{
		ID:            "task-trace",
		Channel:       "general",
		Title:         "Implement trace",
		Details:       "Add execution trace.",
		Owner:         "builder",
		Status:        "in_progress",
		WorkspacePath: filepath.Join(t.TempDir(), "repo"),
		ExecutionLock: &taskExecutionLock{RunID: "run-1", Owner: "builder", Status: "active", AcquiredAt: now, HeartbeatAt: now},
		CreatedAt:     now,
		UpdatedAt:     now,
	}}
	b.usage.Agents = map[string]usageTotals{"builder": {TotalTokens: 123, Requests: 2}}
	sessions := b.buildAgentSessionsLocked("human", "general", false)
	b.mu.Unlock()

	if sessions.Summary["with_heartbeat"] != 1 || len(sessions.Sessions) == 0 {
		t.Fatalf("expected heartbeat-backed session, got %+v", sessions)
	}
	if sessions.Summary["normalized_working"] != 1 {
		t.Fatalf("expected normalized working summary, got %+v", sessions.Summary)
	}
	got := sessions.Sessions[0]
	if got.Slug != "builder" || got.CurrentTaskID != "task-trace" || got.Usage == nil || got.Usage.TotalTokens != 123 {
		t.Fatalf("unexpected agent session: %+v", got)
	}
	if got.NormalizedStatus != "working" {
		t.Fatalf("expected normalized working session, got %+v", got)
	}
}

func TestAgentSessionsExposeLivenessHistory(t *testing.T) {
	b := NewBroker()
	now := "2026-04-30T10:00:00Z"
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	b.tasks = []teamTask{{
		ID:        "task-live",
		Channel:   "general",
		Title:     "Inspect liveness",
		Owner:     "builder",
		Status:    "in_progress",
		CreatedAt: now,
		UpdatedAt: now,
	}}
	b.actions = []officeActionLog{{
		ID:        "action-live-1",
		Kind:      "liveness_recorded",
		Source:    "runtime",
		Channel:   "general",
		Actor:     "builder",
		Summary:   "plan_only: agent only described future work",
		RelatedID: "task-live",
		CreatedAt: now,
	}}
	sessions := b.buildAgentSessionsLocked("human", "general", false)
	b.mu.Unlock()

	if len(sessions.Sessions) == 0 || len(sessions.Sessions[0].LivenessHistory) != 1 {
		t.Fatalf("expected liveness history in session, got %+v", sessions)
	}
	got := sessions.Sessions[0].LivenessHistory[0]
	if got.State != "plan_only" || got.Reason == "" || got.TaskID != "task-live" {
		t.Fatalf("unexpected liveness history event: %+v", got)
	}
	if sessions.Sessions[0].NormalizedStatus != "stale" {
		t.Fatalf("expected liveness-derived stale status, got %+v", sessions.Sessions[0])
	}
}

func TestPaperclipPhase18ExecutionTraceCombinesTaskEvents(t *testing.T) {
	b := NewBroker()
	now := "2026-04-30T10:00:00Z"
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{{
		ID:            "task-trace",
		Channel:       "general",
		Title:         "Ship traced work",
		Owner:         "builder",
		Status:        "done",
		CreatedBy:     "human",
		ThreadID:      "msg-root",
		Outcome:       "Merged",
		OutcomeStatus: "verified",
		Artifacts:     []taskArtifact{{ID: "artifact-1", Kind: "build", State: "verified", Summary: "npm run build passed", CreatedBy: "builder", CreatedAt: now}},
		Evals:         []taskEvalSignal{{ID: "eval-1", Kind: "smoke", Severity: "info", Summary: "smoke ok", CreatedAt: now}},
		CreatedAt:     now,
		UpdatedAt:     now,
	}}
	b.actions = []officeActionLog{{ID: "action-1", Kind: "task_completed", Source: "office", Channel: "general", Actor: "builder", Summary: "done", RelatedID: "task-trace", CreatedAt: now}}
	b.actions = append(b.actions, officeActionLog{ID: "action-live-1", Kind: "liveness_recorded", Source: "runtime", Channel: "general", Actor: "builder", Summary: "completed: task reached durable state", RelatedID: "task-trace", CreatedAt: now})
	b.messages = []channelMessage{{ID: "msg-root", From: "human", Channel: "general", Content: "please ship", Timestamp: now}}
	trace := b.buildExecutionTraceLocked("human", "general", false, "task-trace")
	b.mu.Unlock()

	if trace.Summary["total"] != 1 || trace.Summary["artifact"] != 1 || trace.Summary["action"] != 1 || trace.Summary["message"] != 1 || trace.Summary["liveness"] != 1 {
		t.Fatalf("expected combined execution trace, got %+v", trace)
	}
	if trace.Summary["normalized_completed"] != 1 || trace.Summary["step_normalized_completed"] == 0 {
		t.Fatalf("expected normalized trace summaries, got %+v", trace.Summary)
	}
	if trace.Traces[0].NormalizedStatus != "completed" {
		t.Fatalf("expected completed trace status, got %+v", trace.Traces[0])
	}
	foundLivenessStatus := false
	for _, step := range trace.Traces[0].Steps {
		if step.Kind == "liveness" && step.NormalizedStatus == "completed" {
			foundLivenessStatus = true
		}
	}
	if !foundLivenessStatus {
		t.Fatalf("expected normalized liveness step status, got %+v", trace.Traces[0].Steps)
	}
}

func TestRecordAgentLivenessPersistsAuditAction(t *testing.T) {
	tmpDir := t.TempDir()
	oldPathFn := brokerStatePath
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	b.RecordAgentLiveness(agentActivitySnapshot{
		Slug:           "builder",
		Channel:        "general",
		LivenessState:  "empty_response",
		LivenessReason: "runtime turn completed without output",
		LivenessTaskID: "task-live",
		LivenessAt:     "2026-04-30T10:00:00Z",
	})
	b.mu.RLock()
	defer b.mu.RUnlock()
	activity := b.activity[agentLaneKey("general", "builder")]
	if activity.LivenessState != "empty_response" || len(b.actions) != 1 {
		t.Fatalf("expected activity and action liveness record, activity=%+v actions=%+v", activity, b.actions)
	}
	if b.actions[0].Kind != "liveness_recorded" || b.actions[0].RelatedID != "task-live" {
		t.Fatalf("unexpected liveness action: %+v", b.actions[0])
	}
}

func TestPaperclipPhase18RollbackPackageRequiresConfirmation(t *testing.T) {
	tmpDir := t.TempDir()
	oldPathFn := brokerStatePath
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	now := "2026-04-30T10:00:00Z"
	b.mu.Lock()
	b.actions = []officeActionLog{{
		ID:        "action-policy",
		Kind:      "policy_created",
		Source:    "office",
		Channel:   "general",
		Actor:     "human",
		Summary:   "Require review",
		RelatedID: "policy-1",
		CreatedAt: now,
	}}
	packages := b.buildRollbackPackagesLocked("human", "general", false, "")
	preview, err := b.applyRollbackPackageLocked(rollbackApplyRequest{PackageID: "rollback:action-policy", Actor: "human"})
	if err != nil {
		t.Fatalf("preview rollback package: %v", err)
	}
	applied, err := b.applyRollbackPackageLocked(rollbackApplyRequest{PackageID: "rollback:action-policy", Actor: "human", Reason: "undo bad policy", Confirm: true, Confirmation: "ROLLBACK_PACKAGE"})
	if err != nil {
		t.Fatalf("apply rollback package: %v", err)
	}
	b.mu.Unlock()

	if packages.Summary["total"] != 1 || len(packages.Packages[0].Changes) == 0 {
		t.Fatalf("expected rollback package, got %+v", packages)
	}
	if preview.Persisted || preview.RequiredConfirmation != "ROLLBACK_PACKAGE" {
		t.Fatalf("expected confirmation-gated preview, got %+v", preview)
	}
	if !applied.Persisted || applied.AuditActionID == "" {
		t.Fatalf("expected audited rollback request, got %+v", applied)
	}
}
