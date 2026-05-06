package team

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRemoteSandboxPreviewIsReadOnlyAndDisablesExecution(t *testing.T) {
	b := NewBroker()
	beforeTasks := len(b.tasks)
	req := httptest.NewRequest(http.MethodGet, "/runtime/remote-sandbox-preview", nil)
	rec := httptest.NewRecorder()
	b.handleRemoteSandboxPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload remoteSandboxPreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Persisted || payload.Status != "blocked" || len(payload.Candidates) < 3 {
		t.Fatalf("expected blocked read-only remote sandbox preview, got %+v", payload)
	}
	for _, candidate := range payload.Candidates {
		if candidate.ExecutionEnabled {
			t.Fatalf("remote execution must stay disabled in preview: %+v", candidate)
		}
		if candidate.InstallEnabled || candidate.InstallPolicy == "" || !stringSliceContains(candidate.MissingPolicies, "install_command_policy") {
			t.Fatalf("remote install commands must stay governed and disabled, got %+v", candidate)
		}
		if candidate.Readiness != "blocked" || len(candidate.RequiredPolicies) == 0 || len(candidate.PolicyChecks) == 0 {
			t.Fatalf("expected governed blocked candidate with checks, got %+v", candidate)
		}
	}
	if len(b.tasks) != beforeTasks {
		t.Fatalf("remote sandbox preview mutated tasks: %d -> %d", beforeTasks, len(b.tasks))
	}
}

func TestRemoteSandboxPreviewIncludesGovernedBackends(t *testing.T) {
	payload := buildRemoteSandboxPreview()
	seen := map[string]remoteSandboxPreviewCandidate{}
	for _, candidate := range payload.Candidates {
		seen[candidate.Provider] = candidate
	}
	for _, provider := range []string{"docker", "ssh", "self_hosted_worker"} {
		if _, ok := seen[provider]; !ok {
			t.Fatalf("expected provider %q in remote sandbox preview, got %+v", provider, payload.Candidates)
		}
	}
	if !stringSliceContains(seen["ssh"].RiskSignals, "external_host") || !stringSliceContains(seen["self_hosted_worker"].MissingPolicies, "worker_manifest") {
		t.Fatalf("expected ssh/self-hosted governance signals, got ssh=%+v worker=%+v", seen["ssh"], seen["self_hosted_worker"])
	}
}
