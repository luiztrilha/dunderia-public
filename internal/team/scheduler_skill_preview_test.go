package team

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSchedulerSkillPreviewReportsMissingAndRiskyBindingsReadOnly(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "eng", "Eng")
	b.skills = []teamSkill{
		{
			ID:            "skill-repo-audit",
			Name:          "repo-audit",
			Title:         "Repo Audit",
			Content:       "Audit the repository and report findings.",
			PluginID:      "starter-kit",
			PluginKind:    "skill",
			Capabilities:  []string{"skill.invoke"},
			HealthStatus:  "ready",
			SourceType:    "starter_pack",
			SourceRef:     "default-skill:repo-audit",
			SourceHash:    "hash-repo-audit",
			ScanStatus:    "passed",
			LastScannedAt: "2026-05-04T10:00:00Z",
			Status:        "active",
			CreatedAt:     "2026-05-04T10:00:00Z",
			UpdatedAt:     "2026-05-04T10:00:00Z",
		},
		{
			ID:            "skill-scheduler-mutator",
			Name:          "scheduler-mutator",
			Title:         "Scheduler Mutator",
			Content:       "Call /scheduler to create scheduler jobs for follow-up work.",
			PluginID:      "starter-kit",
			PluginKind:    "skill",
			Capabilities:  []string{"skill.invoke"},
			HealthStatus:  "ready",
			SourceType:    "starter_pack",
			SourceRef:     "default-skill:scheduler-mutator",
			SourceHash:    "hash-scheduler-mutator",
			ScanStatus:    "passed",
			LastScannedAt: "2026-05-04T10:00:00Z",
			Status:        "active",
			CreatedAt:     "2026-05-04T10:00:00Z",
			UpdatedAt:     "2026-05-04T10:00:00Z",
		},
	}
	b.scheduler = []schedulerJob{
		{
			Slug:         "nightly-audit",
			Kind:         "workflow",
			Label:        "Nightly Audit",
			TargetType:   "workflow",
			TargetID:     "nightly-audit",
			Channel:      "general",
			WorkflowKey:  "nightly-audit",
			ScheduleExpr: "daily",
			SkillName:    "repo-audit",
			SkillNames:   []string{"missing-skill", "scheduler-mutator"},
			Status:       "scheduled",
		},
		{
			Slug:       "private-audit",
			Label:      "Private Audit",
			Channel:    "private",
			SkillName:  "repo-audit",
			Status:     "scheduled",
			TargetType: "workflow",
		},
	}
	beforeJobs := len(b.scheduler)
	beforeSkills := len(b.skills)
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scheduler/skill-preview?channel=general&viewer_slug=eng", nil)
	b.handleSchedulerSkillPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload schedulerSkillPreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Persisted || len(payload.Jobs) != 1 {
		t.Fatalf("expected one read-only scoped job preview, got %+v", payload)
	}
	job := payload.Jobs[0]
	if job.Slug != "nightly-audit" || job.Readiness != "blocked" || job.RiskLevel != "high" {
		t.Fatalf("unexpected job readiness: %+v", job)
	}
	if len(job.Skills) != 3 {
		t.Fatalf("expected three skill bindings, got %+v", job.Skills)
	}
	if !stringSliceContains(job.Reasons, "skill not found") || !stringSliceContains(job.Reasons, "skill references scheduler mutation") {
		t.Fatalf("expected missing and scheduler mutation reasons, got %+v", job.Reasons)
	}

	b.mu.RLock()
	afterJobs := len(b.scheduler)
	afterSkills := len(b.skills)
	b.mu.RUnlock()
	if afterJobs != beforeJobs || afterSkills != beforeSkills {
		t.Fatalf("scheduler skill preview mutated state: jobs %d -> %d, skills %d -> %d", beforeJobs, afterJobs, beforeSkills, afterSkills)
	}
}

func TestSchedulerJobSkillNamesNormalizePluralBindings(t *testing.T) {
	job := normalizeSchedulerJob(schedulerJob{
		Slug:       "weekly-report",
		Label:      "Weekly Report",
		SkillNames: []string{"report", " report ", "publish"},
		Status:     "scheduled",
	})
	if job.SkillName != "report" {
		t.Fatalf("expected first plural binding to backfill skill_name, got %+v", job)
	}
	if len(job.SkillNames) != 2 || job.SkillNames[0] != "report" || job.SkillNames[1] != "publish" {
		t.Fatalf("expected unique normalized skill_names, got %+v", job.SkillNames)
	}
}
