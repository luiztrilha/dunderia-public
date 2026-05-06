package team

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBrowserInspectionHandoffPreviewPackagesScopedEvidenceReadOnly(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{
		{
			ID:        "task-browser",
			Channel:   "general",
			Title:     "Fix checkout button",
			Status:    "active",
			Owner:     "frontend",
			CreatedAt: "2026-05-05T10:00:00Z",
			UpdatedAt: "2026-05-05T10:02:00Z",
			Artifacts: []taskArtifact{{
				ID:         "artifact-browser",
				Kind:       "browser_inspection",
				ResultRole: "evidence",
				Title:      "Checkout visual evidence",
				Summary:    "selector=[data-testid=\"checkout\"]",
				CreatedAt:  "2026-05-05T10:01:00Z",
				UpdatedAt:  "2026-05-05T10:02:00Z",
				BrowserInspection: &taskBrowserInspection{
					PageURL:        "http://localhost:7891/#/channels/general",
					Selector:       "[data-testid=\"checkout\"]",
					ElementText:    "Finalizar",
					ScreenshotPath: "D:\\tmp\\checkout.png",
					ViewportWidth:  390,
					ViewportHeight: 844,
				},
			}},
		},
		{
			ID:        "task-private-browser",
			Channel:   "private",
			Title:     "Private browser evidence",
			Status:    "active",
			CreatedAt: "2026-05-05T10:00:00Z",
			UpdatedAt: "2026-05-05T10:02:00Z",
			Artifacts: []taskArtifact{{
				ID:         "artifact-private",
				Kind:       "browser_inspection",
				ResultRole: "evidence",
				BrowserInspection: &taskBrowserInspection{
					PageURL: "http://localhost/private",
				},
			}},
		},
	}
	beforeTasks := len(b.tasks)
	beforeArtifacts := len(b.tasks[0].Artifacts)
	b.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/browser/inspection-handoff-preview?channel=general&viewer_slug=human", nil)
	rec := httptest.NewRecorder()
	b.handleBrowserInspectionHandoffPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload browserInspectionHandoffPreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Persisted || payload.Status != "ok" || len(payload.Handoffs) != 1 {
		t.Fatalf("expected one read-only browser handoff, got %+v", payload)
	}
	handoff := payload.Handoffs[0]
	if !handoff.Ready || handoff.TaskID != "task-browser" || handoff.ArtifactID != "artifact-browser" {
		t.Fatalf("expected ready browser handoff, got %+v", handoff)
	}
	if handoff.PageURL == "" || handoff.Selector == "" || handoff.ScreenshotPath == "" || !strings.Contains(handoff.HandoffPrompt, "Browser inspection handoff") {
		t.Fatalf("expected handoff prompt with browser evidence, got %+v", handoff)
	}
	if strings.Contains(handoff.HandoffPrompt, "private") {
		t.Fatalf("handoff leaked private browser evidence: %s", handoff.HandoffPrompt)
	}
	b.mu.RLock()
	afterTasks := len(b.tasks)
	afterArtifacts := len(b.tasks[0].Artifacts)
	b.mu.RUnlock()
	if afterTasks != beforeTasks || afterArtifacts != beforeArtifacts {
		t.Fatalf("browser handoff preview mutated state: tasks %d -> %d artifacts %d -> %d", beforeTasks, afterTasks, beforeArtifacts, afterArtifacts)
	}
}

func TestBrowserInspectionHandoffPreviewFlagsIncompleteEvidence(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{{
		ID:        "task-incomplete-browser",
		Channel:   "general",
		Title:     "Inspect login page",
		Status:    "active",
		CreatedAt: "2026-05-05T10:00:00Z",
		UpdatedAt: "2026-05-05T10:02:00Z",
		Artifacts: []taskArtifact{{
			ID:         "artifact-incomplete",
			Kind:       "browser_inspection",
			ResultRole: "evidence",
			Summary:    "API_TOKEN appeared in a debug panel",
			BrowserInspection: &taskBrowserInspection{
				PageURL:     "http://localhost:7891/#/login",
				ElementText: "API_TOKEN appeared in a debug panel",
			},
		}},
	}}
	b.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/browser/inspection-handoff-preview?channel=general&viewer_slug=human", nil)
	rec := httptest.NewRecorder()
	b.handleBrowserInspectionHandoffPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload browserInspectionHandoffPreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Status != "review" || len(payload.Handoffs) != 1 {
		t.Fatalf("expected review handoff, got %+v", payload)
	}
	handoff := payload.Handoffs[0]
	if handoff.Ready || !stringSliceContains(handoff.MissingFields, "selector") || !stringSliceContains(handoff.MissingFields, "screenshot_path") {
		t.Fatalf("expected missing selector and screenshot fields, got %+v", handoff)
	}
	if !stringSliceContains(handoff.RiskSignals, "secret_like_content") {
		t.Fatalf("expected secret-like risk signal, got %+v", handoff)
	}
}
