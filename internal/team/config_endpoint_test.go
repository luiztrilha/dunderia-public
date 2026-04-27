package team

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nex-crm/wuphf/internal/backup"
	"github.com/nex-crm/wuphf/internal/config"
)

func newConfigTestBroker() *Broker {
	b := NewBroker()
	b.token = "test-token"
	return b
}

func doConfigRequest(t *testing.T, b *Broker, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if path == "/config" {
		req.Header.Set("Authorization", "Bearer "+b.Token())
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	switch path {
	case "/config":
		b.requireAuth(b.handleConfig)(rec, req)
	case "/health":
		b.handleHealth(rec, req)
	default:
		t.Fatalf("unsupported test path %q", path)
	}
	return rec
}

// TestConfigEndpointAndHealth is a smoke test for ISSUE-004: the wizard's
// POST /config must persist llm_provider and /health must reflect it.
func TestConfigEndpointAndHealth(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfgPath := filepath.Join(tmp, ".wuphf", "config.json")
	t.Setenv("WUPHF_CONFIG_PATH", cfgPath)
	t.Setenv("WUPHF_CLOUD_BACKUP_BOOTSTRAP_PATH", filepath.Join(tmp, ".wuphf", "cloud-backup-bootstrap.json"))
	if err := os.MkdirAll(filepath.Join(tmp, ".wuphf"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{"llm_provider":"claude-code"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	b := newConfigTestBroker()
	b.runtimeProvider = "claude-code"

	resp := doConfigRequest(t, b, http.MethodGet, "/health", nil)
	var h1 map[string]any
	raw1 := resp.Body.Bytes()
	_ = json.Unmarshal(raw1, &h1)
	t.Logf("GET /health (initial) -> %s", string(raw1))
	if p, _ := h1["provider"].(string); p != "claude-code" {
		t.Fatalf("expected provider=claude-code before POST, got %q", p)
	}
	if backend, _ := h1["memory_backend"].(string); backend != config.MemoryBackendNone {
		t.Fatalf("expected memory_backend=%q before POST, got %q", config.MemoryBackendNone, backend)
	}

	resp = doConfigRequest(t, b, http.MethodPost, "/config", []byte(`{"llm_provider":"codex"}`))
	raw := resp.Body.Bytes()
	t.Logf("POST /config {llm_provider:codex} -> %d %s", resp.Code, string(raw))
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /config status=%d body=%s", resp.Code, string(raw))
	}

	resp = doConfigRequest(t, b, http.MethodGet, "/health", nil)
	var h2 map[string]any
	raw2 := resp.Body.Bytes()
	_ = json.Unmarshal(raw2, &h2)
	t.Logf("GET /health (after POST) -> %s", string(raw2))
	if p, _ := h2["provider"].(string); p != "codex" {
		t.Fatalf("expected provider=codex after POST, got %q", p)
	}

	resp = doConfigRequest(t, b, http.MethodGet, "/config", nil)
	rawConfig := resp.Body.Bytes()
	var cfgResp map[string]any
	_ = json.Unmarshal(rawConfig, &cfgResp)
	if p, _ := cfgResp["llm_provider"].(string); p != "codex" {
		t.Fatalf("expected /config llm_provider=codex after POST, got %q (body=%s)", p, string(rawConfig))
	}
	if backend, _ := cfgResp["memory_backend"].(string); backend != config.MemoryBackendNone {
		t.Fatalf("expected /config memory_backend=%q, got %q", config.MemoryBackendNone, backend)
	}

	disk, _ := os.ReadFile(cfgPath)
	if !strings.Contains(string(disk), `"llm_provider": "codex"`) {
		t.Fatalf("config.json missing codex: %s", string(disk))
	}

	resp = doConfigRequest(t, b, http.MethodPost, "/config", []byte(`{"llm_provider":"anthropic"}`))
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported provider, got %d", resp.Code)
	}
}

func TestConfigEndpointAcceptsGeminiVertexProviderFamily(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfgPath := filepath.Join(tmp, ".wuphf", "config.json")
	t.Setenv("WUPHF_CONFIG_PATH", cfgPath)
	t.Setenv("WUPHF_CLOUD_BACKUP_BOOTSTRAP_PATH", filepath.Join(tmp, ".wuphf", "cloud-backup-bootstrap.json"))
	if err := os.MkdirAll(filepath.Join(tmp, ".wuphf"), 0o700); err != nil {
		t.Fatal(err)
	}

	b := newConfigTestBroker()
	b.runtimeProvider = "claude-code"

	for _, providerName := range []string{"gemini", "gemini-vertex"} {
		payload := []byte(`{"llm_provider":"` + providerName + `"}`)
		resp := doConfigRequest(t, b, http.MethodPost, "/config", payload)
		raw := resp.Body.Bytes()
		if resp.Code != http.StatusOK {
			t.Fatalf("POST /config (%s) status=%d body=%s", providerName, resp.Code, string(raw))
		}

		resp = doConfigRequest(t, b, http.MethodGet, "/config", nil)
		raw = resp.Body.Bytes()
		var cfgResp map[string]any
		_ = json.Unmarshal(raw, &cfgResp)
		if got, _ := cfgResp["llm_provider"].(string); got != providerName {
			t.Fatalf("expected /config llm_provider=%q, got %q (body=%s)", providerName, got, string(raw))
		}
	}
}

func TestConfigEndpointRoundTripsCloudBackupSettings(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfgPath := filepath.Join(tmp, ".wuphf", "config.json")
	t.Setenv("WUPHF_CONFIG_PATH", cfgPath)
	t.Setenv("WUPHF_CLOUD_BACKUP_BOOTSTRAP_PATH", filepath.Join(tmp, ".wuphf", "cloud-backup-bootstrap.json"))
	if err := os.MkdirAll(filepath.Join(tmp, ".wuphf"), 0o700); err != nil {
		t.Fatal(err)
	}

	brokerCloudBackupQueueMu.Lock()
	brokerCloudBackupPendingPlan = nil
	brokerCloudBackupWorkerActive = false
	brokerCloudBackupState = brokerCloudBackupRuntime{}
	brokerCloudBackupQueueMu.Unlock()

	oldAsync := runBrokerCloudBackupAsync
	oldSleep := sleepBrokerCloudBackup
	runBrokerCloudBackupAsync = func(fn func()) { fn() }
	sleepBrokerCloudBackup = func(time.Duration) {}
	defer func() {
		runBrokerCloudBackupAsync = oldAsync
		sleepBrokerCloudBackup = oldSleep
		brokerCloudBackupQueueMu.Lock()
		brokerCloudBackupPendingPlan = nil
		brokerCloudBackupWorkerActive = false
		brokerCloudBackupState = brokerCloudBackupRuntime{}
		brokerCloudBackupQueueMu.Unlock()
	}()

	restore := backup.SetSinkFactoryForTest(func(_ context.Context, settings backup.Settings) (backup.Sink, error) {
		return &testBackupSink{}, nil
	})
	defer restore()

	b := newConfigTestBroker()
	b.runtimeProvider = "gemini-vertex"

	resp := doConfigRequest(t, b, http.MethodPost, "/config", []byte(`{"cloud_backup_provider":"gcs","cloud_backup_bucket":"dunderia-backups","cloud_backup_prefix":"office-a"}`))
	raw := resp.Body.Bytes()
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /config cloud backup status=%d body=%s", resp.Code, string(raw))
	}

	resp = doConfigRequest(t, b, http.MethodGet, "/config", nil)
	raw = resp.Body.Bytes()
	var cfgResp map[string]any
	_ = json.Unmarshal(raw, &cfgResp)
	if got, _ := cfgResp["cloud_backup_provider"].(string); got != "gcs" {
		t.Fatalf("expected cloud_backup_provider=gcs, got %q (body=%s)", got, string(raw))
	}
	if got, _ := cfgResp["cloud_backup_bucket"].(string); got != "dunderia-backups" {
		t.Fatalf("expected cloud_backup_bucket=dunderia-backups, got %q (body=%s)", got, string(raw))
	}
	if got, _ := cfgResp["cloud_backup_prefix"].(string); got != "office-a" {
		t.Fatalf("expected cloud_backup_prefix=office-a, got %q (body=%s)", got, string(raw))
	}
}

func TestConfigEndpointRejectsLegacyMemoryBackends(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfgPath := filepath.Join(tmp, ".wuphf", "config.json")
	t.Setenv("WUPHF_CONFIG_PATH", cfgPath)
	t.Setenv("WUPHF_CLOUD_BACKUP_BOOTSTRAP_PATH", filepath.Join(tmp, ".wuphf", "cloud-backup-bootstrap.json"))
	if err := os.MkdirAll(filepath.Join(tmp, ".wuphf"), 0o700); err != nil {
		t.Fatal(err)
	}

	b := newConfigTestBroker()
	b.runtimeProvider = "codex"

	for _, backend := range []string{"nex", "gbrain"} {
		resp := doConfigRequest(t, b, http.MethodPost, "/config", []byte(`{"memory_backend":"`+backend+`"}`))
		raw := resp.Body.Bytes()
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for memory_backend=%q, got %d body=%s", backend, resp.Code, string(raw))
		}
	}
}

func TestConfigEndpointRejectsMutationWhenConfigIsUnreadable(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfgPath := filepath.Join(tmp, ".wuphf", "config.json")
	t.Setenv("WUPHF_CONFIG_PATH", cfgPath)
	t.Setenv("WUPHF_CLOUD_BACKUP_BOOTSTRAP_PATH", filepath.Join(tmp, ".wuphf", "cloud-backup-bootstrap.json"))
	if err := os.MkdirAll(filepath.Join(tmp, ".wuphf"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{"llm_provider":`), 0o600); err != nil {
		t.Fatal(err)
	}

	b := newConfigTestBroker()
	b.runtimeProvider = "gemini-vertex"

	resp := doConfigRequest(t, b, http.MethodPost, "/config", []byte(`{"web_search_provider":"brave"}`))
	raw := resp.Body.Bytes()
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when config is unreadable, got %d body=%s", resp.Code, string(raw))
	}

	disk, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config after failed POST: %v", err)
	}
	if string(disk) != `{"llm_provider":` {
		t.Fatalf("expected unreadable config to remain untouched, got %q", string(disk))
	}
}
