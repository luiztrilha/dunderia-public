package workspace

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// newMux wires the workspace routes onto a fresh ServeMux for each test so
// parallel runs don't share state. Passes a nil middleware so the test doesn't
// have to fake broker auth — RegisterRoutes substitutes a passthrough.
func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	RegisterRoutes(mux, nil)
	return mux
}

func decodeBody(t *testing.T, body string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, body)
	}
	return out
}

func TestShredHandlerRejectsNonPost(t *testing.T) {
	withRuntimeHome(t)
	req := httptest.NewRequest(http.MethodGet, "/workspace/shred", nil)
	w := httptest.NewRecorder()
	newMux().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestShredHandlerPreservesWorkspaceState(t *testing.T) {
	dir := withRuntimeHome(t)
	paths := seedWorkspace(t, dir)

	req := httptest.NewRequest(http.MethodPost, "/workspace/shred", nil)
	w := httptest.NewRecorder()
	newMux().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := decodeBody(t, w.Body.String())
	if ok, _ := body["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", body)
	}
	// Redirect hint stays present for backwards-compatible UI flows.
	if redirect, _ := body["redirect"].(string); redirect != "/" {
		t.Fatalf("expected redirect=/, got %q", redirect)
	}
	// The legacy shred endpoint must not destroy local office state.
	for _, label := range []string{"onboarded", "company", "brokerState", "officeTasks", "workflow"} {
		assertStays(t, label, paths[label])
	}
	assertStays(t, "session", paths["session"])
	assertStays(t, "worktree", paths["worktree"])
}

func TestShredHandlerReportsNoRemovedPaths(t *testing.T) {
	dir := withRuntimeHome(t)
	seedWorkspace(t, dir)

	req := httptest.NewRequest(http.MethodPost, "/workspace/shred", nil)
	w := httptest.NewRecorder()
	newMux().ServeHTTP(w, req)

	body := decodeBody(t, w.Body.String())
	if removed, ok := body["removed"].([]any); ok && len(removed) != 0 {
		t.Fatalf("expected no removed paths, got %v", removed)
	}
}

func TestShredHandlerOnEmptyHomeIsOK(t *testing.T) {
	dir := withRuntimeHome(t)
	// Ensure the home exists but is completely empty.
	if err := os.MkdirAll(filepath.Join(dir, ".wuphf"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/workspace/shred", nil)
	w := httptest.NewRecorder()
	newMux().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on empty home, got %d", w.Code)
	}
}
