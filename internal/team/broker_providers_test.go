package team

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nex-crm/wuphf/internal/company"
	"github.com/nex-crm/wuphf/internal/provider"
)

func TestMemberFromSpec_CopiesProvider(t *testing.T) {
	spec := company.MemberSpec{
		Slug:     "pm-bot",
		Name:     "PM Bot",
		Provider: provider.ProviderBinding{Kind: provider.KindCodex, Model: "gpt-5.4"},
	}
	m := memberFromSpec(spec, "test", "2026-04-16T00:00:00Z", false)
	if m.Slug != "pm-bot" || m.Name != "PM Bot" {
		t.Fatalf("unexpected member: %+v", m)
	}
	if m.Provider.Kind != provider.KindCodex || m.Provider.Model != "gpt-5.4" {
		t.Fatalf("provider not copied: %+v", m.Provider)
	}
}

func TestHandleOfficeMembers_CreateWithGeminiProvider(t *testing.T) {
	_, ts, token := newBrokerHTTPTest(t)
	defer ts.Close()

	body := map[string]any{
		"action": "create",
		"slug":   "research-gemini",
		"name":   "Research Gemini",
		"provider": map[string]any{
			"kind":  "gemini",
			"model": "gemini-2.5-pro",
		},
	}
	resp := doBrokerPost(t, ts, token, "/office-members", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create status=%d", resp.StatusCode)
	}
}

func TestHandleOfficeMembers_UpdateSwitchesProvider(t *testing.T) {
	b, ts, token := newBrokerHTTPTest(t)
	defer ts.Close()

	// Create on codex
	create := map[string]any{
		"action":   "create",
		"slug":     "switcher",
		"name":     "Switcher",
		"provider": map[string]any{"kind": provider.KindCodex, "model": "gpt-5.4"},
	}
	if r := doBrokerPost(t, ts, token, "/office-members", create); r.StatusCode != http.StatusOK {
		t.Fatalf("create status=%d", r.StatusCode)
	}

	// Update to gemini-vertex
	update := map[string]any{
		"action": "update",
		"slug":   "switcher",
		"provider": map[string]any{
			"kind":  provider.KindGeminiVertex,
			"model": provider.GeminiVertexDefaultModel,
		},
	}
	if r := doBrokerPost(t, ts, token, "/office-members", update); r.StatusCode != http.StatusOK {
		t.Fatalf("update status=%d", r.StatusCode)
	}

	b.mu.Lock()
	m := b.findMemberLocked("switcher")
	b.mu.Unlock()
	if m.Provider.Kind != provider.KindGeminiVertex {
		t.Fatalf("provider not switched: %q", m.Provider.Kind)
	}
	if m.Provider.Model != provider.GeminiVertexDefaultModel {
		t.Fatalf("new gemini-vertex model missing: %+v", m.Provider)
	}
}

func TestHandleOfficeMembers_UpdateSwitchesProviderClearsOperationalBlocks(t *testing.T) {
	b, ts, token := newBrokerHTTPTest(t)
	defer ts.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	b.mu.Lock()
	b.members = append(b.members, officeMember{
		Slug: "switcher-blocked",
		Name: "Switcher Blocked",
		Provider: provider.ProviderBinding{
			Kind:  provider.KindClaudeCode,
			Model: "claude-sonnet-4-6",
		},
	})
	b.memberIndex["switcher-blocked"] = len(b.members) - 1
	b.tasks = append(b.tasks, teamTask{
		ID:        "task-1",
		Channel:   "general",
		Title:     "Recover blocked runtime",
		Owner:     "switcher-blocked",
		Status:    "blocked",
		Blocked:   true,
		Details:   "Automatic error recovery: @switcher-blocked failed before a durable task handoff. Last error: blocked by policy.",
		ThreadID:  "msg-1",
		CreatedAt: now,
		UpdatedAt: now,
	})
	b.executionNodes = append(b.executionNodes, executionNode{
		ID:               "exec-1",
		Channel:          "general",
		RootMessageID:    "msg-1",
		TriggerMessageID: "msg-1",
		OwnerAgent:       "switcher-blocked",
		Status:           "timed_out",
		LastError:        "blocked by policy",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	b.watchdogs = append(b.watchdogs, watchdogAlert{
		ID:         "watchdog-1",
		Kind:       "agent_runtime_blocked",
		Channel:    "general",
		TargetType: "agent_runtime",
		TargetID:   "switcher-blocked|general",
		Owner:      "ceo",
		Status:     "active",
		Summary:    "runtime blocked",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err := b.saveLocked(); err != nil {
		b.mu.Unlock()
		t.Fatalf("saveLocked: %v", err)
	}
	b.mu.Unlock()

	update := map[string]any{
		"action": "update",
		"slug":   "switcher-blocked",
		"provider": map[string]any{
			"kind":  provider.KindCodex,
			"model": "gpt-5.4",
		},
	}
	if r := doBrokerPost(t, ts, token, "/office-members", update); r.StatusCode != http.StatusOK {
		t.Fatalf("update status=%d", r.StatusCode)
	}

	b.mu.Lock()
	member := b.findMemberLocked("switcher-blocked")
	if member == nil {
		b.mu.Unlock()
		t.Fatal("updated member not found")
	}
	if member.Provider.Kind != provider.KindCodex || member.Provider.Model != "gpt-5.4" {
		b.mu.Unlock()
		t.Fatalf("provider not updated: %+v", member.Provider)
	}
	var task *teamTask
	for i := range b.tasks {
		if b.tasks[i].ID == "task-1" {
			task = &b.tasks[i]
			break
		}
	}
	if task == nil {
		b.mu.Unlock()
		t.Fatal("task not found after update")
	}
	if task.Blocked || task.Status != "in_progress" {
		b.mu.Unlock()
		t.Fatalf("expected runtime-blocked task to resume, got %+v", *task)
	}
	node := b.findExecutionNodeForTaskLocked(task, "switcher-blocked")
	if node == nil {
		b.mu.Unlock()
		t.Fatal("execution node not found after update")
	}
	if node.Status != "pending" {
		b.mu.Unlock()
		t.Fatalf("expected execution node to reopen, got %+v", *node)
	}
	if node.LastError != "" {
		b.mu.Unlock()
		t.Fatalf("expected execution node error cleared, got %+v", *node)
	}
	if len(b.watchdogs) == 0 || b.watchdogs[0].Status != "resolved" {
		b.mu.Unlock()
		t.Fatalf("expected runtime watchdog resolved, got %+v", b.watchdogs)
	}
	actions := append([]officeActionLog(nil), b.actions...)
	b.mu.Unlock()

	foundUnblocked := false
	for _, action := range actions {
		if action.Kind == "task_unblocked" && action.RelatedID == "task-1" {
			foundUnblocked = true
			break
		}
	}
	if !foundUnblocked {
		t.Fatalf("expected task_unblocked action, got %+v", actions)
	}
}

func TestHandleOfficeMembers_UpdateSwitchesProviderKeepsBusinessBlocks(t *testing.T) {
	b, ts, token := newBrokerHTTPTest(t)
	defer ts.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	b.mu.Lock()
	b.members = append(b.members, officeMember{
		Slug: "switcher-business",
		Name: "Switcher Business",
		Provider: provider.ProviderBinding{
			Kind: provider.KindClaudeCode,
		},
	})
	b.memberIndex["switcher-business"] = len(b.members) - 1
	b.tasks = append(b.tasks, teamTask{
		ID:        "task-1",
		Channel:   "general",
		Title:     "Wait for legal sign-off",
		Owner:     "switcher-business",
		Status:    "blocked",
		Blocked:   true,
		Details:   "Blocked pending legal approval from the business owner.",
		ThreadID:  "msg-1",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err := b.saveLocked(); err != nil {
		b.mu.Unlock()
		t.Fatalf("saveLocked: %v", err)
	}
	b.mu.Unlock()

	update := map[string]any{
		"action": "update",
		"slug":   "switcher-business",
		"provider": map[string]any{
			"kind":  provider.KindCodex,
			"model": "gpt-5.4",
		},
	}
	if r := doBrokerPost(t, ts, token, "/office-members", update); r.StatusCode != http.StatusOK {
		t.Fatalf("update status=%d", r.StatusCode)
	}

	b.mu.Lock()
	var task *teamTask
	for i := range b.tasks {
		if b.tasks[i].ID == "task-1" {
			task = &b.tasks[i]
			break
		}
	}
	b.mu.Unlock()
	if task == nil {
		t.Fatal("task not found after update")
	}
	if !task.Blocked || task.Status != "blocked" {
		t.Fatalf("expected business block to remain, got %+v", *task)
	}
}

func TestHandleOfficeMembers_UpdateRejectsRemovedProvider(t *testing.T) {
	_, ts, token := newBrokerHTTPTest(t)
	defer ts.Close()

	create := map[string]any{
		"action":   "create",
		"slug":     "switcher-legacy",
		"name":     "Switcher Legacy",
		"provider": map[string]any{"kind": provider.KindCodex, "model": "gpt-5.4"},
	}
	if r := doBrokerPost(t, ts, token, "/office-members", create); r.StatusCode != http.StatusOK {
		t.Fatalf("create status=%d", r.StatusCode)
	}

	update := map[string]any{
		"action": "update",
		"slug":   "switcher-legacy",
		"provider": map[string]any{
			"kind":  "legacy-runtime",
			"model": provider.GeminiVertexDefaultModel,
		},
	}
	if r := doBrokerPost(t, ts, token, "/office-members", update); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("removed legacy provider should be rejected, got %d", r.StatusCode)
	}
}

func TestHandleOfficeMembers_UpdateSwitchesProviderToOpenclaude(t *testing.T) {
	b, ts, token := newBrokerHTTPTest(t)
	defer ts.Close()

	create := map[string]any{
		"action":   "create",
		"slug":     "switcher-openclaude",
		"name":     "Switcher OpenClaude",
		"provider": map[string]any{"kind": provider.KindCodex, "model": "gpt-5.4"},
	}
	if r := doBrokerPost(t, ts, token, "/office-members", create); r.StatusCode != http.StatusOK {
		t.Fatalf("create status=%d", r.StatusCode)
	}

	update := map[string]any{
		"action": "update",
		"slug":   "switcher-openclaude",
		"provider": map[string]any{
			"kind":  "openclaude",
			"model": "claude-sonnet-4-6",
		},
	}
	if r := doBrokerPost(t, ts, token, "/office-members", update); r.StatusCode != http.StatusOK {
		t.Fatalf("update status=%d", r.StatusCode)
	}

	b.mu.Lock()
	m := b.findMemberLocked("switcher-openclaude")
	b.mu.Unlock()
	if m.Provider.Kind != "openclaude" {
		t.Fatalf("provider not switched: %q", m.Provider.Kind)
	}
	if m.Provider.Model != "claude-sonnet-4-6" {
		t.Fatalf("provider model not updated: %q", m.Provider.Model)
	}
}

func TestHandleOfficeMembers_InvalidProviderKind(t *testing.T) {
	_, ts, token := newBrokerHTTPTest(t)
	defer ts.Close()

	body := map[string]any{
		"action":   "create",
		"slug":     "bad-provider",
		"name":     "Bad Provider",
		"provider": map[string]any{"kind": "not-a-runtime"},
	}
	resp := doBrokerPost(t, ts, token, "/office-members", body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	if !strings.Contains(buf.String(), "claude-code") {
		t.Fatalf("error should list valid kinds, got %q", buf.String())
	}
}

func TestProviderFieldSurvivesBrokerReload(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	b.mu.Lock()
	b.members = append(b.members, officeMember{
		Slug: "persist-test",
		Name: "Persist Test",
		Provider: provider.ProviderBinding{
			Kind:  provider.KindGeminiVertex,
			Model: provider.GeminiVertexDefaultModel,
		},
	})
	b.memberIndex[b.members[len(b.members)-1].Slug] = len(b.members) - 1
	if err := b.saveLocked(); err != nil {
		b.mu.Unlock()
		t.Fatalf("saveLocked: %v", err)
	}
	b.mu.Unlock()

	reloaded := NewBroker()
	reloaded.mu.Lock()
	got := reloaded.findMemberLocked("persist-test")
	reloaded.mu.Unlock()
	if got == nil {
		t.Fatal("member did not survive reload")
	}
	if got.Provider.Kind != provider.KindGeminiVertex {
		t.Fatalf("kind not persisted: %q", got.Provider.Kind)
	}
	if got.Provider.Model != provider.GeminiVertexDefaultModel {
		t.Fatalf("provider model not persisted: %+v", got.Provider)
	}
}

func TestHandleOfficeMembers_UpdateDefaultMemberProviderSurvivesReload(t *testing.T) {
	tmpDir := isolateBrokerPersistenceEnv(t)
	manifestPath := filepath.Join(tmpDir, "company.json")
	t.Setenv("WUPHF_COMPANY_FILE", manifestPath)
	if err := company.SaveManifest(company.Manifest{
		Name: "Test Office",
		Lead: "ceo",
		Members: []company.MemberSpec{
			{Slug: "ceo", Name: "CEO", Role: "CEO", System: true},
			{Slug: "ops", Name: "Ops", Role: "Operations"},
		},
		Channels: []company.ChannelSpec{
			{Slug: "general", Name: "general", Members: []string{"ceo", "ops"}},
		},
	}); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}

	oldPathFn := brokerStatePath
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	t.Cleanup(func() { brokerStatePath = oldPathFn })

	b := NewBroker()
	mux := http.NewServeMux()
	mux.HandleFunc("/office-members", b.requireAuth(b.handleOfficeMembers))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	update := map[string]any{
		"action": "update",
		"slug":   "ops",
		"provider": map[string]any{
			"kind":  provider.KindCodex,
			"model": "gpt-5.4",
		},
	}
	if r := doBrokerPost(t, ts, b.Token(), "/office-members", update); r.StatusCode != http.StatusOK {
		t.Fatalf("update status=%d", r.StatusCode)
	}

	if _, err := os.Stat(brokerStatePath()); err != nil {
		t.Fatalf("expected broker state file after provider update: %v", err)
	}

	reloaded := NewBroker()
	reloaded.mu.Lock()
	got := reloaded.findMemberLocked("ops")
	reloaded.mu.Unlock()
	if got == nil {
		t.Fatal("default member did not survive reload")
	}
	if got.Provider.Kind != provider.KindCodex || got.Provider.Model != "gpt-5.4" {
		t.Fatalf("provider lost across reload: %+v", got.Provider)
	}
}

func TestHandleOfficeMembers_UpdateManifestProviderOverrideSurvivesReload(t *testing.T) {
	tmpDir := isolateBrokerPersistenceEnv(t)
	manifestPath := filepath.Join(tmpDir, "company.json")
	t.Setenv("WUPHF_COMPANY_FILE", manifestPath)
	if err := company.SaveManifest(company.Manifest{
		Name: "Test Office",
		Lead: "ceo",
		Members: []company.MemberSpec{
			{
				Slug:     "ceo",
				Name:     "CEO",
				Role:     "CEO",
				System:   true,
				Provider: provider.ProviderBinding{Kind: provider.KindGeminiVertex},
			},
			{
				Slug:     "frontend",
				Name:     "Frontend",
				Role:     "Frontend",
				Provider: provider.ProviderBinding{Kind: provider.KindClaudeCode, Model: "claude-sonnet-4-6"},
			},
		},
		Channels: []company.ChannelSpec{
			{Slug: "general", Name: "general", Members: []string{"ceo", "frontend"}},
		},
	}); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}

	oldPathFn := brokerStatePath
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	t.Cleanup(func() { brokerStatePath = oldPathFn })

	b := NewBroker()
	mux := http.NewServeMux()
	mux.HandleFunc("/office-members", b.requireAuth(b.handleOfficeMembers))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	update := map[string]any{
		"action": "update",
		"slug":   "frontend",
		"provider": map[string]any{
			"kind":  provider.KindCodex,
			"model": "gpt-5.4",
		},
	}
	if r := doBrokerPost(t, ts, b.Token(), "/office-members", update); r.StatusCode != http.StatusOK {
		t.Fatalf("update status=%d", r.StatusCode)
	}

	manifest, err := company.LoadManifest()
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	var manifestFrontend *company.MemberSpec
	for i := range manifest.Members {
		if manifest.Members[i].Slug == "frontend" {
			manifestFrontend = &manifest.Members[i]
			break
		}
	}
	if manifestFrontend == nil {
		t.Fatal("frontend missing from manifest after update")
	}
	if manifestFrontend.Provider.Kind != provider.KindCodex || manifestFrontend.Provider.Model != "gpt-5.4" {
		t.Fatalf("manifest provider not updated: %+v", manifestFrontend.Provider)
	}

	reloaded := NewBroker()
	reloaded.mu.Lock()
	got := reloaded.findMemberLocked("frontend")
	reloaded.mu.Unlock()
	if got == nil {
		t.Fatal("frontend did not survive reload")
	}
	if got.Provider.Kind != provider.KindCodex || got.Provider.Model != "gpt-5.4" {
		t.Fatalf("provider reverted after reload: %+v", got.Provider)
	}
}

func TestBrokerReloadPrefersPersistedMemberProviderOverStaleManifest(t *testing.T) {
	tmpDir := isolateBrokerPersistenceEnv(t)
	manifestPath := filepath.Join(tmpDir, "company.json")
	t.Setenv("WUPHF_COMPANY_FILE", manifestPath)
	if err := company.SaveManifest(company.Manifest{
		Name: "Test Office",
		Lead: "ceo",
		Members: []company.MemberSpec{
			{
				Slug:     "ceo",
				Name:     "CEO",
				Role:     "CEO",
				System:   true,
				Provider: provider.ProviderBinding{Kind: provider.KindGeminiVertex},
			},
			{
				Slug:     "reviewer",
				Name:     "Reviewer",
				Role:     "Reviewer",
				Provider: provider.ProviderBinding{Kind: provider.KindClaudeCode, Model: "claude-sonnet-4-6"},
			},
		},
		Channels: []company.ChannelSpec{
			{Slug: "general", Name: "general", Members: []string{"ceo", "reviewer"}},
		},
	}); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}

	oldPathFn := brokerStatePath
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	t.Cleanup(func() { brokerStatePath = oldPathFn })

	b := NewBroker()
	b.mu.Lock()
	reviewer := b.findMemberLocked("reviewer")
	if reviewer == nil {
		b.mu.Unlock()
		t.Fatal("reviewer missing from initial broker state")
	}
	reviewer.Provider = provider.ProviderBinding{Kind: provider.KindCodex, Model: "gpt-5.4"}
	if err := b.saveLocked(); err != nil {
		b.mu.Unlock()
		t.Fatalf("saveLocked: %v", err)
	}
	b.mu.Unlock()

	reloaded := NewBroker()
	reloaded.mu.Lock()
	got := reloaded.findMemberLocked("reviewer")
	reloaded.mu.Unlock()
	if got == nil {
		t.Fatal("reviewer missing after reload")
	}
	if got.Provider.Kind != provider.KindCodex || got.Provider.Model != "gpt-5.4" {
		t.Fatalf("expected persisted provider override to win, got %+v", got.Provider)
	}
}

func TestRebuildMemberIndex_AfterRemove(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	defer b.mu.Unlock()
	b.members = []officeMember{
		{Slug: "a", Name: "A"},
		{Slug: "b", Name: "B"},
		{Slug: "c", Name: "C"},
	}
	b.rebuildMemberIndexLocked()

	// Remove "b"
	filtered := b.members[:0]
	for _, m := range b.members {
		if m.Slug != "b" {
			filtered = append(filtered, m)
		}
	}
	b.members = filtered
	b.rebuildMemberIndexLocked()

	if got := b.findMemberLocked("b"); got != nil {
		t.Fatal("removed member still found")
	}
	if got := b.findMemberLocked("c"); got == nil || got.Name != "C" {
		t.Fatalf("shift-after-remove lost C: %+v", got)
	}
	if got := b.findMemberLocked("a"); got == nil || got.Name != "A" {
		t.Fatalf("A lookup broken after rebuild: %+v", got)
	}
}

// newBrokerHTTPTest spins up a broker with its HTTP handler attached to an
// httptest server, returning the broker, the server, and the auth token.
func newBrokerHTTPTest(t *testing.T) (*Broker, *httptest.Server, string) {
	t.Helper()
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	t.Cleanup(func() { brokerStatePath = oldPathFn })

	b := NewBroker()
	mux := http.NewServeMux()
	mux.HandleFunc("/office-members", b.requireAuth(b.handleOfficeMembers))
	ts := httptest.NewServer(mux)
	return b, ts, b.Token()
}

func doBrokerPost(t *testing.T, ts *httptest.Server, token, path string, body any) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+path, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return resp
}
