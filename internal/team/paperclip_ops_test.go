package team

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestPaperclipPhase11WorktreePreviewIsDryRun(t *testing.T) {
	snapshot := runtimeDoctorSnapshot{
		RuntimeHome: "D:/Repos/dunderia",
		QuarantineSignals: []runtimeDoctorQuarantineSignal{{
			Kind:     "duplicate_active_worktree",
			Severity: "warn",
			Summary:  "2 active tasks share the same worktree.",
			TaskIDs:  []string{"task-1", "task-2"},
			Path:     "D:/Repos/shared",
		}},
	}
	preview := buildRuntimeWorktreePreview(snapshot)
	if preview.Persisted {
		t.Fatalf("expected dry-run preview, got %+v", preview)
	}
	if !preview.RequiresApproval || len(preview.Actions) != 1 || !preview.Actions[0].Mutating {
		t.Fatalf("expected mutating action to require approval, got %+v", preview)
	}
}

func TestPaperclipPhase11SecretAuditFindsPlaintextConfigWithoutValues(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("WUPHF_CONFIG_PATH", tmpDir+"/config.json")
	t.Setenv("WUPHF_SECRET_STRICT", "1")
	if err := os.WriteFile(tmpDir+"/config.json", []byte(`{"openai_api_key":"sk-test-secret","company_name":"ok"}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	audit := buildRuntimeSecretAudit()
	if !audit.Strict || audit.PlaintextConfigCount != 1 || len(audit.PlaintextConfigNames) != 1 || audit.PlaintextConfigNames[0] != "openai_api_key" {
		t.Fatalf("unexpected audit: %+v", audit)
	}
	if strings.Contains(strings.Join(audit.PlaintextConfigNames, ","), "sk-test-secret") {
		t.Fatalf("secret value leaked in audit: %+v", audit)
	}
}

func TestPaperclipPhase11HumanSessionIsReadOnly(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	beforeMembers := len(b.members)
	beforeChannels := len(b.channels)
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/humans/session?viewer_slug=human&channel=general", nil)
	b.handleHumanSession(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload humanSessionSnapshot
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.CanAccessChannel || payload.ViewerSlug != "human" {
		t.Fatalf("unexpected session payload: %+v", payload)
	}
	b.mu.RLock()
	afterMembers := len(b.members)
	afterChannels := len(b.channels)
	b.mu.RUnlock()
	if beforeMembers != afterMembers || beforeChannels != afterChannels {
		t.Fatalf("human session mutated topology: members %d->%d channels %d->%d", beforeMembers, afterMembers, beforeChannels, afterChannels)
	}
}

func TestHumanPermissionsPreviewIsReadOnlyAndBlocksTopologyMutation(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	ensureTestMemberAccess(b, "private-client", "private-agent", "Private Agent")
	beforeMembers := len(b.members)
	beforeChannels := len(b.channels)
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/humans/permissions-preview?viewer_slug=human&all_channels=true", nil)
	b.handleHumanPermissionsPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload humanPermissionsPreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Persisted || payload.Summary["topology_blocked"] == 0 || payload.Summary["can_approve_actions"] == 0 {
		t.Fatalf("expected read-only approval-capability preview with topology blocked, got %+v", payload)
	}
	for _, snapshot := range payload.Snapshots {
		if snapshot.CanMutateTopology {
			t.Fatalf("preview must not allow topology mutation: %+v", snapshot)
		}
		var sawTopologyBlock bool
		for _, capability := range snapshot.Capabilities {
			if capability.Name == "topology.mutate" && capability.Status == "blocked" {
				sawTopologyBlock = true
			}
		}
		if !sawTopologyBlock {
			t.Fatalf("expected topology.mutate blocked capability, got %+v", snapshot)
		}
	}
	b.mu.RLock()
	afterMembers := len(b.members)
	afterChannels := len(b.channels)
	b.mu.RUnlock()
	if beforeMembers != afterMembers || beforeChannels != afterChannels {
		t.Fatalf("permissions preview mutated topology: members %d->%d channels %d->%d", beforeMembers, afterMembers, beforeChannels, afterChannels)
	}
}

func TestHumanPermissionsPreviewScopesAgentViewerToAccessibleChannels(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	ensureTestMemberAccess(b, "private-client", "private-agent", "Private Agent")
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/humans/permissions-preview?viewer_slug=private-agent&all_channels=true", nil)
	b.handleHumanPermissionsPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload humanPermissionsPreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Snapshots) != 1 || payload.Snapshots[0].Channel != "private-client" {
		t.Fatalf("expected scoped private-agent snapshot only, got %+v", payload.Snapshots)
	}
	snapshot := payload.Snapshots[0]
	if snapshot.AccessLevel != "read_only" || snapshot.CanApproveActions || snapshot.CanAnswerRequests {
		t.Fatalf("expected non-human viewer to be read-only, got %+v", snapshot)
	}
}

func TestPaperclipPhase11RuntimeSmokeEndpoint(t *testing.T) {
	oldProcessList := runtimeProcessListFn
	runtimeProcessListFn = func(string) []runtimeDoctorProcess {
		return []runtimeDoctorProcess{{PID: "1", Kind: "web", CommandLine: "wuphf-current.exe --web-port 7891"}}
	}
	defer func() { runtimeProcessListFn = oldProcessList }()

	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{{ID: "task-smoke", Channel: "general", Title: "Smoke", Status: "open", CreatedAt: "2026-04-30T10:00:00Z", UpdatedAt: "2026-04-30T10:00:00Z"}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/runtime/smoke", nil)
	b.handleRuntimeSmoke(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload runtimeSmokeSnapshot
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Checks) < 4 || strings.TrimSpace(payload.Status) == "" {
		t.Fatalf("unexpected smoke payload: %+v", payload)
	}
}
