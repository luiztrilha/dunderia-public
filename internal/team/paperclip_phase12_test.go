package team

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPaperclipPhase12ResumePackSummarizesTaskWithoutMutation(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{{
		ID:                         "task-resume",
		Channel:                    "general",
		Title:                      "Finish resumable work",
		Details:                    "Implement durable handoff.",
		Status:                     "blocked",
		Blocked:                    true,
		Owner:                      "backend",
		TaskType:                   "feature",
		CompletionEvidenceRequired: true,
		Artifacts: []taskArtifact{{
			Title:     "Test output",
			Summary:   "go test passed",
			CreatedAt: "2026-04-30T10:10:00Z",
		}},
		CreatedAt: "2026-04-30T10:00:00Z",
		UpdatedAt: "2026-04-30T10:15:00Z",
	}}
	before := len(b.tasks)
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/resume-pack?task_id=task-resume&viewer_slug=human", nil)
	b.handleResumePack(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload resumePackResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Task == nil || payload.Task.ID != "task-resume" || len(payload.NextSteps) == 0 || len(payload.Evidence) == 0 {
		t.Fatalf("unexpected resume pack: %+v", payload)
	}
	b.mu.RLock()
	after := len(b.tasks)
	b.mu.RUnlock()
	if before != after {
		t.Fatalf("resume pack mutated tasks: %d -> %d", before, after)
	}
}

func TestPaperclipPhase12GovernanceHistoryIncludesRollbackPlan(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.orgProposals = []orgProposal{{
		ID:                            "proposal-1",
		Kind:                          "channel",
		Title:                         "Create audit channel",
		ProposedBy:                    "human",
		Channel:                       "general",
		TargetType:                    "channel",
		TargetID:                      "audit",
		ProposedChange:                "create channel audit",
		Status:                        "proposed",
		RequiresTopologyAuthorization: true,
		CreatedAt:                     "2026-04-30T10:00:00Z",
	}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/governance/history?limit=5", nil)
	b.handleGovernanceHistory(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload governanceHistoryResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Events) != 1 || !payload.Events[0].RequiresTopologyAuthorization || payload.Events[0].RollbackPlan == "" {
		t.Fatalf("expected topology event with rollback plan, got %+v", payload)
	}
}

func TestPaperclipPhase12TemplatePreviewReportsRiskAndReviews(t *testing.T) {
	b := NewBroker()
	body := bytes.NewBufferString(`{"title":"Starter kit token","agents":["new-agent","new-agent"],"channels":["new-channel"],"skills":["deploy-secret"],"secrets":["GITHUB_TOKEN"]}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/templates/preview", body)
	b.handleTemplatePreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload templatePreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.RiskScore == 0 || payload.RiskLevel == "" || len(payload.RequiredReviews) < 2 || len(payload.Conflicts) == 0 {
		t.Fatalf("expected enriched risk preview, got %+v", payload)
	}
	if payload.Persisted {
		t.Fatalf("template preview must remain dry-run: %+v", payload)
	}
}

func TestPaperclipPhase12SkillTrustScoresRiskySkill(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	b.skills = []teamSkill{{
		ID:                  "skill-risk",
		Name:                "risky-skill",
		Title:               "Risky skill",
		Content:             "Use API_KEY directly",
		Status:              "active",
		HealthStatus:        "error",
		LastExecutionStatus: "failed",
		CreatedAt:           "2026-04-30T10:00:00Z",
		UpdatedAt:           "2026-04-30T10:00:00Z",
	}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/skills/trust", nil)
	b.handleSkillTrust(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload skillTrustResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Summary.Low != 1 || len(payload.Skills) != 1 || payload.Skills[0].Score >= 60 {
		t.Fatalf("expected low-trust skill, got %+v", payload)
	}
}

func TestPaperclipPhase12SkillTrustTreatsLegacyMetadataAsMedium(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	b.skills = []teamSkill{{
		ID:        "skill-legacy",
		Name:      "legacy-skill",
		Title:     "Legacy skill",
		Content:   "Reusable local workflow.",
		Status:    "active",
		CreatedAt: "2026-04-30T10:00:00Z",
		UpdatedAt: "2026-04-30T10:00:00Z",
	}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/skills/trust", nil)
	b.handleSkillTrust(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload skillTrustResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Summary.Low != 0 || payload.Summary.Medium != 1 {
		t.Fatalf("expected legacy skill to be medium metadata debt, got %+v", payload)
	}
}

func TestSkillTrustIncludesProvenanceAndScanStatus(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	b.skills = []teamSkill{{
		ID:            "skill-trusted",
		Name:          "trusted-skill",
		Title:         "Trusted skill",
		Content:       "Reusable local workflow.",
		PluginID:      "starter-kit",
		PluginKind:    "skill",
		Capabilities:  []string{"skill.invoke"},
		HealthStatus:  "ready",
		SourceType:    "starter_pack",
		SourceRef:     "default-skill:trusted-skill",
		SourceHash:    "abc123",
		InstalledAt:   "2026-05-04T10:00:00Z",
		LastScannedAt: "2026-05-04T10:01:00Z",
		ScanStatus:    "passed",
		ScanSummary:   "Local static scan passed.",
		Status:        "active",
		CreatedAt:     "2026-05-04T10:00:00Z",
		UpdatedAt:     "2026-05-04T10:00:00Z",
	}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/skills/trust", nil)
	b.handleSkillTrust(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload skillTrustResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Skills) != 1 || payload.Skills[0].SourceType != "starter_pack" || payload.Skills[0].ScanStatus != "passed" {
		t.Fatalf("expected provenance in trust record, got %+v", payload)
	}
	if payload.Summary.High != 1 {
		t.Fatalf("expected fully provenanced skill to remain high trust, got %+v", payload)
	}
}

func TestPaperclipPhase12OperatorOverviewIsCompact(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{
		{
			ID:            "task-blocked",
			Channel:       "general",
			Title:         "Blocked operator work",
			Status:        "blocked",
			Blocked:       true,
			Owner:         "ceo",
			QueuePriority: "high",
			CreatedAt:     "2026-04-30T10:00:00Z",
			UpdatedAt:     "2026-04-30T10:06:00Z",
		},
		{
			ID:            "task-watchdog",
			Channel:       "general",
			Title:         "Validate unanswered CEO follow-up resumed",
			Status:        "in_progress",
			Owner:         "watchdog",
			TaskType:      "bugfix",
			QueuePriority: "high",
			Details:       "A CEO follow-up is still waiting.\n\nAutomatic error recovery.",
			CreatedAt:     "2026-04-30T10:00:00Z",
			UpdatedAt:     "2026-04-30T10:07:00Z",
		},
		{
			ID:            "task-next",
			Channel:       "general",
			Title:         "Next operator work",
			Status:        "open",
			Owner:         "backend",
			QueuePriority: "medium",
			CreatedAt:     "2026-04-30T10:00:00Z",
			UpdatedAt:     "2026-04-30T10:05:00Z",
		},
	}
	b.requests = []humanInterview{{
		ID:        "req-1",
		Kind:      "question",
		Status:    "open",
		From:      "backend",
		Channel:   "general",
		Question:  "Need approval?",
		CreatedAt: "2026-04-30T10:01:00Z",
	}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/operator/overview?viewer_slug=human&channel=general", nil)
	b.handleOperatorOverview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload operatorOverviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Counts.OpenTasks != 2 || payload.Counts.BlockedTasks != 1 || len(payload.NextWork) == 0 || payload.NextWork[0].TaskID != "task-next" || payload.Resume == nil || len(payload.Requests) != 1 {
		t.Fatalf("unexpected operator overview: %+v", payload)
	}
	if payload.Status == "blocked" {
		t.Fatalf("operator overview should remain actionable when nonblocked work exists: %+v", payload)
	}
}
