package team

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSchedulerRevisionsPreviewIsReadOnlyAndRestoreBlocked(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	b := NewBroker()
	b.scheduler = append(b.scheduler, schedulerJob{Slug: "daily-priority", Label: "Daily priority", Kind: "routine", Channel: "general", Status: "active"})
	beforeJobs := len(b.scheduler)
	req := httptest.NewRequest(http.MethodGet, "/scheduler/revisions-preview?channel=general&viewer_slug=human", nil)
	rec := httptest.NewRecorder()
	b.handleSchedulerRevisionsPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload schedulerRevisionsPreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Persisted || payload.Status != "blocked" || len(payload.Jobs) == 0 {
		t.Fatalf("expected blocked read-only revision preview, got %+v", payload)
	}
	if payload.Jobs[0].RestoreEnabled || payload.Jobs[0].RestoreReadiness != "blocked" || !stringSliceContains(payload.Jobs[0].MissingPolicies, "restore_confirmation") {
		t.Fatalf("expected restore to stay disabled with policy gaps, got %+v", payload.Jobs[0])
	}
	if len(b.scheduler) != beforeJobs {
		t.Fatalf("scheduler revisions preview mutated scheduler: %d -> %d", beforeJobs, len(b.scheduler))
	}
}

func TestWikiEditorPreviewKeepsEditorDisabled(t *testing.T) {
	payload := buildWikiEditorPreview()
	if payload.Persisted || payload.Status != "blocked" {
		t.Fatalf("expected blocked read-only wiki editor preview, got %+v", payload)
	}
	foundWikilink := false
	foundCodeSafety := false
	for _, check := range payload.Checks {
		if check.ID == "wikilink_preservation" {
			foundWikilink = true
		}
		if check.ID == "code_region_safety" {
			foundCodeSafety = true
		}
	}
	for _, mode := range payload.Modes {
		if mode.EditorEnabled {
			t.Fatalf("wiki editor preview must not enable editing: %+v", mode)
		}
	}
	if !foundWikilink || !foundCodeSafety {
		t.Fatalf("expected wikilink and code safety checks, got %+v", payload.Checks)
	}
}

func TestProviderCompatibilityPreviewListsWireFormatRisks(t *testing.T) {
	payload := buildProviderCompatibilityPreview()
	if payload.Persisted || payload.MutationEnabled || payload.Status != "blocked" {
		t.Fatalf("expected blocked non-mutating provider compatibility preview, got %+v", payload)
	}
	var gemini *providerCompatibilityPreview
	for i := range payload.Providers {
		if payload.Providers[i].Provider == "gemini" {
			gemini = &payload.Providers[i]
		}
	}
	if gemini == nil || !stringSliceContains(gemini.KnownEventShapes, "gemini_cli_v0_38_stream_json") || !stringSliceContains(gemini.MissingTests, "gemini_v0_38_stream_json_parser") {
		t.Fatalf("expected Gemini v0.38 parser risk, got %+v", gemini)
	}
}

func TestProjectOverviewWidgetsPreviewIsReadOnly(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	b := NewBroker()
	b.tasks = append(b.tasks, teamTask{ID: "task-widget", Title: "Widget work", Status: "open", Channel: "general", WorkspacePath: "D:/Repos/dunderia"})
	beforeTasks := len(b.tasks)
	req := httptest.NewRequest(http.MethodGet, "/studio/project-overview-preview", nil)
	rec := httptest.NewRecorder()
	b.handleProjectOverviewWidgetsPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload projectOverviewWidgetsPreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Persisted || payload.MutationEnabled || len(payload.Widgets) == 0 {
		t.Fatalf("expected read-only widgets preview, got %+v", payload)
	}
	foundWorkspace := false
	for _, widget := range payload.Widgets {
		if widget.MutationEnabled {
			t.Fatalf("widget mutation must stay disabled: %+v", widget)
		}
		if widget.ID == "workspaces" && widget.Count > 0 {
			foundWorkspace = true
		}
	}
	if !foundWorkspace || len(b.tasks) != beforeTasks {
		t.Fatalf("expected workspace widget and no task mutation, widgets=%+v tasks=%d->%d", payload.Widgets, beforeTasks, len(b.tasks))
	}
}

func TestFileContextHandoffPreviewDoesNotReadContents(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	b := NewBroker()
	b.tasks = append(b.tasks, teamTask{
		ID:            "task-file",
		Title:         "File context",
		Status:        "open",
		Channel:       "general",
		WorkspacePath: "D:/Repos/dunderia",
		Artifacts: []taskArtifact{{
			ID:      "artifact-1",
			Kind:    "patch",
			Title:   "Patch file",
			Path:    "D:/Repos/dunderia/internal/team/example.go",
			Summary: "Reference only",
		}},
	})
	beforeTasks := len(b.tasks)
	req := httptest.NewRequest(http.MethodGet, "/files/context-handoff-preview?channel=general&viewer_slug=human", nil)
	rec := httptest.NewRecorder()
	b.handleFileContextHandoffPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload fileContextHandoffPreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Persisted || payload.ContentReadEnabled || len(payload.Items) < 2 {
		t.Fatalf("expected file-reference preview without content reads, got %+v", payload)
	}
	for _, item := range payload.Items {
		if item.ContentIncluded || !stringSliceContains(item.MissingPolicies, "secret_scan_policy") {
			t.Fatalf("file handoff must include references only with missing policies, got %+v", item)
		}
	}
	if len(b.tasks) != beforeTasks {
		t.Fatalf("file context preview mutated tasks: %d -> %d", beforeTasks, len(b.tasks))
	}
}
