package team

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestToolsetProfilePreviewIsScopedAndReadOnly(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	b.members = []officeMember{
		{Slug: "eng", Name: "Engineer", Role: "Backend", PermissionMode: "plan", AllowedTools: []string{"team_poll", "team_skill_run"}},
		{Slug: "private-agent", Name: "Private Agent", Role: "Ops", PermissionMode: "plan", AllowedTools: []string{"team_poll"}},
	}
	b.channels = []teamChannel{
		{Slug: "general", Name: "General", Members: []string{"eng"}},
		{Slug: "private", Name: "Private", Members: []string{"private-agent"}},
	}
	b.skills = []teamSkill{{
		ID:            "skill-scheduler-mutator",
		Name:          "scheduler-mutator",
		Title:         "Scheduler Mutator",
		Content:       "Call /scheduler to create cron jobs when needed.",
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
	}}
	beforeMembers := len(b.members)
	beforeChannels := len(b.channels)
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/toolsets/profile-preview?channel=general&viewer_slug=human&q=eng", nil)
	b.handleToolsetProfilePreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload toolsetProfilePreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Persisted || len(payload.Profiles) != 1 {
		t.Fatalf("expected one read-only profile, got %+v", payload)
	}
	profile := payload.Profiles[0]
	if profile.AgentSlug != "eng" || profile.Channel != "general" {
		t.Fatalf("unexpected profile identity: %+v", profile)
	}
	if profile.RiskLevel != "high" || profile.SuggestedAction != "restrict" {
		t.Fatalf("expected scheduler-mutating profile to be high risk/restrict, got %+v", profile)
	}
	if !stringSliceContains(profile.Signals, "scheduler_mutating") {
		t.Fatalf("expected scheduler_mutating signal, got %+v", profile.Signals)
	}

	b.mu.RLock()
	afterMembers := len(b.members)
	afterChannels := len(b.channels)
	b.mu.RUnlock()
	if afterMembers != beforeMembers || afterChannels != beforeChannels {
		t.Fatalf("toolset preview mutated topology: members %d -> %d channels %d -> %d", beforeMembers, afterMembers, beforeChannels, afterChannels)
	}
}

func TestToolsetProfilePreviewReportsDeclaredToolDrift(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	b.members = []officeMember{{
		Slug:           "ops",
		Name:           "Ops",
		Role:           "Operations",
		PermissionMode: "plan",
		AllowedTools:   []string{"imaginary_tool"},
	}}
	b.channels = []teamChannel{{Slug: "general", Name: "General", Members: []string{"ops"}}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/toolsets/profile-preview?channel=general&viewer_slug=human", nil)
	b.handleToolsetProfilePreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload toolsetProfilePreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Profiles) != 1 {
		t.Fatalf("expected one profile, got %+v", payload)
	}
	profile := payload.Profiles[0]
	if !stringSliceContains(profile.Drift, "declared_missing_runtime") {
		t.Fatalf("expected declared_missing_runtime drift, got %+v", profile.Drift)
	}
	if profile.SuggestedAction != "review" {
		t.Fatalf("expected review action for drift/external capability profile, got %+v", profile)
	}
}
