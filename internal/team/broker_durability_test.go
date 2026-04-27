package team

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nex-crm/wuphf/internal/backup"
	"github.com/nex-crm/wuphf/internal/config"
)

type testBackupSink struct {
	mu       sync.Mutex
	keys     []string
	body     map[string]string
	deletes  []string
	putCalls int
	failPuts int
}

func (s *testBackupSink) Put(_ context.Context, key string, data []byte, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.putCalls++
	if s.failPuts > 0 {
		s.failPuts--
		return fmt.Errorf("forced put failure for %s", key)
	}
	if s.body == nil {
		s.body = make(map[string]string)
	}
	s.keys = append(s.keys, key)
	s.body[key] = string(data)
	return nil
}

func (s *testBackupSink) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if body, ok := s.body[key]; ok {
		return []byte(body), nil
	}
	return nil, os.ErrNotExist
}

func (s *testBackupSink) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletes = append(s.deletes, key)
	delete(s.body, key)
	return nil
}

func (s *testBackupSink) Close() error { return nil }

func TestHandlePostMessageDeduplicatesHumanClientID(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	if err := b.StartOnPort(0); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer b.Stop()

	base := fmt.Sprintf("http://%s", b.Addr())
	body, _ := json.Marshal(map[string]any{
		"from":      "you",
		"channel":   "general",
		"content":   "preserve this message exactly once",
		"client_id": "client-msg-1",
	})

	post := func() map[string]any {
		req, _ := http.NewRequest(http.MethodPost, base+"/messages", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+b.Token())
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("post message failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			raw, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200 posting message, got %d: %s", resp.StatusCode, raw)
		}
		var payload map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return payload
	}

	first := post()
	second := post()

	if first["persisted"] != true {
		t.Fatalf("expected first response to confirm persistence, got %#v", first)
	}
	if second["persisted"] != true {
		t.Fatalf("expected second response to confirm persistence, got %#v", second)
	}
	if second["duplicate"] != true {
		t.Fatalf("expected second response to be marked duplicate, got %#v", second)
	}
	if first["id"] != second["id"] {
		t.Fatalf("expected duplicate request to reuse message id, got %v and %v", first["id"], second["id"])
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.messages) != 1 {
		t.Fatalf("expected exactly one persisted message, got %d", len(b.messages))
	}
	if got := strings.TrimSpace(b.messages[0].Content); got != "preserve this message exactly once" {
		t.Fatalf("unexpected message content: %q", got)
	}
	if got := strings.TrimSpace(b.messages[0].ClientID); got != "client-msg-1" {
		t.Fatalf("expected client id to be persisted, got %q", got)
	}
}

func TestSaveLockedArchivesAppendOnlyStateHistory(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	statePath := filepath.Join(tmpDir, "broker-state.json")
	brokerStatePath = func() string { return statePath }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()

	if _, err := b.PostMessage("you", "general", "first durable message", nil, ""); err != nil {
		t.Fatalf("first post failed: %v", err)
	}
	if _, err := b.PostMessage("you", "general", "second durable message", nil, ""); err != nil {
		t.Fatalf("second post failed: %v", err)
	}

	historyDir := brokerStateHistoryDir(statePath)
	entries, err := os.ReadDir(historyDir)
	if err != nil {
		t.Fatalf("read history dir: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected at least two archived states, got %d", len(entries))
	}

	firstSnapshot, err := os.ReadFile(filepath.Join(historyDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("read first snapshot: %v", err)
	}
	lastSnapshot, err := os.ReadFile(filepath.Join(historyDir, entries[len(entries)-1].Name()))
	if err != nil {
		t.Fatalf("read last snapshot: %v", err)
	}

	if !bytes.Contains(firstSnapshot, []byte("first durable message")) {
		t.Fatalf("expected earliest snapshot to preserve first message")
	}
	if bytes.Contains(firstSnapshot, []byte("second durable message")) {
		t.Fatalf("expected earliest snapshot to remain append-only and not be replaced")
	}
	if !bytes.Contains(lastSnapshot, []byte("second durable message")) {
		t.Fatalf("expected latest snapshot to include second message")
	}
}

func TestSaveLockedMirrorsBrokerStateToConfiguredCloudBackup(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "broker-state.json")
	brokerStatePath = func() string { return statePath }
	defer func() { brokerStatePath = oldPathFn }()
	oldAsync := runBrokerCloudBackupAsync
	runBrokerCloudBackupAsync = func(fn func()) { fn() }
	defer func() { runBrokerCloudBackupAsync = oldAsync }()

	t.Setenv("HOME", tmpDir)
	t.Setenv("WUPHF_CONFIG_PATH", filepath.Join(tmpDir, ".wuphf", "config.json"))
	if err := os.MkdirAll(filepath.Join(tmpDir, ".wuphf"), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	cfgBytes, err := json.Marshal(config.Config{
		CloudBackupProvider: backup.ProviderGCS,
		CloudBackupBucket:   "dunderia-backups",
		CloudBackupPrefix:   "office-a",
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".wuphf", "config.json"), append(cfgBytes, '\n'), 0o600); err != nil {
		t.Fatalf("seed config file: %v", err)
	}

	sink := &testBackupSink{}
	restore := backup.SetSinkFactoryForTest(func(_ context.Context, settings backup.Settings) (backup.Sink, error) {
		if settings.Provider != backup.ProviderGCS || settings.Bucket != "dunderia-backups" || settings.Prefix != "office-a" {
			t.Fatalf("unexpected cloud backup settings: %+v", settings)
		}
		return sink, nil
	})
	defer restore()

	b := NewBroker()
	if _, err := b.PostMessage("you", "general", "mirror this remotely", nil, ""); err != nil {
		t.Fatalf("post failed: %v", err)
	}

	expected := []string{
		"office-a/team/broker-state.json",
		"office-a/team/broker-state.last-good.json",
	}
	for _, key := range expected {
		if _, ok := sink.body[key]; !ok {
			t.Fatalf("expected mirrored object %q, got keys %+v", key, sink.keys)
		}
	}
	foundHistory := false
	for _, key := range sink.keys {
		if strings.HasPrefix(key, "office-a/team/history/") && strings.HasSuffix(key, ".json") {
			foundHistory = true
			if !strings.Contains(sink.body[key], "mirror this remotely") {
				t.Fatalf("expected history mirror to contain message, got %q", sink.body[key])
			}
			break
		}
	}
	if !foundHistory {
		t.Fatalf("expected append-only history mirror, got keys %+v", sink.keys)
	}
	if !strings.Contains(sink.body["office-a/team/broker-state.json"], "mirror this remotely") {
		t.Fatalf("expected broker-state mirror body to contain message")
	}
}

func TestScheduleBrokerCloudBackupPlanRetriesWithBackoff(t *testing.T) {
	brokerCloudBackupQueueMu.Lock()
	brokerCloudBackupPendingPlan = nil
	brokerCloudBackupWorkerActive = false
	brokerCloudBackupQueueMu.Unlock()

	oldAsync := runBrokerCloudBackupAsync
	runBrokerCloudBackupAsync = func(fn func()) { go fn() }
	defer func() { runBrokerCloudBackupAsync = oldAsync }()

	oldSleep := sleepBrokerCloudBackup
	var sleepMu sync.Mutex
	var sleeps []time.Duration
	sleepBrokerCloudBackup = func(d time.Duration) {
		sleepMu.Lock()
		sleeps = append(sleeps, d)
		sleepMu.Unlock()
	}
	defer func() { sleepBrokerCloudBackup = oldSleep }()

	sink := &testBackupSink{failPuts: 2}
	restore := backup.SetSinkFactoryForTest(func(_ context.Context, settings backup.Settings) (backup.Sink, error) {
		if settings.Provider != backup.ProviderGCS || settings.Bucket != "dunderia-backups" || settings.Prefix != "office-a" {
			t.Fatalf("unexpected cloud backup settings: %+v", settings)
		}
		return sink, nil
	})
	defer restore()

	scheduleBrokerCloudBackupPlan(brokerCloudBackupPlan{
		settings: backup.Settings{
			Provider: backup.ProviderGCS,
			Bucket:   "dunderia-backups",
			Prefix:   "office-a",
		},
		data:          []byte(`{"messages":[{"id":"msg-1","content":"retry me"}]}`),
		writeCurrent:  true,
		writeSnapshot: true,
		writeHistory:  true,
		historyName:   "retry-history.json",
	})

	deadline := time.Now().Add(2 * time.Second)
	for {
		sink.mu.Lock()
		_, ok := sink.body["office-a/team/broker-state.json"]
		putCalls := sink.putCalls
		sink.mu.Unlock()
		if ok {
			if putCalls < 3 {
				t.Fatalf("expected at least 3 put attempts, got %d", putCalls)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for cloud backup worker to retry and succeed")
		}
		time.Sleep(10 * time.Millisecond)
	}

	sleepMu.Lock()
	gotSleeps := append([]time.Duration(nil), sleeps...)
	sleepMu.Unlock()
	if len(gotSleeps) < 2 {
		t.Fatalf("expected retry backoff sleeps, got %+v", gotSleeps)
	}
	hasOneSecond := false
	hasTwoSeconds := false
	for _, sleep := range gotSleeps {
		if sleep == time.Second {
			hasOneSecond = true
		}
		if sleep == 2*time.Second {
			hasTwoSeconds = true
		}
	}
	if !hasOneSecond || !hasTwoSeconds {
		t.Fatalf("expected exponential backoff sleeps including 1s and 2s, got %+v", gotSleeps)
	}
}

func TestBrokerLocalFirstRoundTripPersistsIndexesAndObservabilityHistory(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	statePath := filepath.Join(tmpDir, "broker-state.json")
	brokerStatePath = func() string { return statePath }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	b.SetReadinessSnapshot(brokerReadinessSnapshot{
		Live:    true,
		Ready:   false,
		State:   "warming",
		Stage:   "runtime",
		Summary: "warming providers",
	})
	if err := b.StartOnPort(0); err != nil {
		t.Fatalf("start broker: %v", err)
	}

	base := fmt.Sprintf("http://%s", b.Addr())
	do := func(method, path string, body []byte) *http.Response {
		req, err := http.NewRequest(method, base+path, bytes.NewReader(body))
		if err != nil {
			t.Fatalf("build request %s %s: %v", method, path, err)
		}
		req.Header.Set("Authorization", "Bearer "+b.Token())
		if len(body) > 0 {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do request %s %s: %v", method, path, err)
		}
		return resp
	}

	msgBody, _ := json.Marshal(map[string]any{
		"from":    "you",
		"channel": "general",
		"content": "local-first persistence round trip",
	})
	resp := do(http.MethodPost, "/messages", msgBody)
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("post message: %d %s", resp.StatusCode, raw)
	}
	resp.Body.Close()

	reqBody, _ := json.Marshal(map[string]any{
		"action":   "create",
		"from":     "ceo",
		"channel":  "general",
		"kind":     "approval",
		"title":    "Publish fix",
		"question": "Can we ship this?",
	})
	resp = do(http.MethodPost, "/requests", reqBody)
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create request: %d %s", resp.StatusCode, raw)
	}
	var created struct {
		Request humanInterview `json:"request"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		resp.Body.Close()
		t.Fatalf("decode request create: %v", err)
	}
	resp.Body.Close()
	if strings.TrimSpace(created.Request.ID) == "" {
		t.Fatal("expected created request id")
	}

	answerBody, _ := json.Marshal(map[string]any{
		"id":          created.Request.ID,
		"choice_id":   "approve",
		"choice_text": "Approve",
	})
	resp = do(http.MethodPost, "/requests/answer", answerBody)
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("answer request: %d %s", resp.StatusCode, raw)
	}
	resp.Body.Close()

	resp = do(http.MethodGet, "/messages?channel=general&viewer_slug=human&limit=10", nil)
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get messages: %d %s", resp.StatusCode, raw)
	}
	resp.Body.Close()

	b.mu.Lock()
	if err := b.saveLocked(); err != nil {
		b.mu.Unlock()
		t.Fatalf("persist broker state: %v", err)
	}
	b.mu.Unlock()
	b.Stop()

	state, err := loadBrokerStateFile(statePath)
	if err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	if state.MessageIndexSnapshot == nil || len(state.MessageIndexSnapshot.ByToken) == 0 {
		t.Fatalf("expected persisted message index snapshot, got %+v", state.MessageIndexSnapshot)
	}
	if len(state.ObservabilityHistory) == 0 {
		t.Fatalf("expected persisted observability history, got %+v", state.ObservabilityHistory)
	}

	b2 := NewBroker()
	b2.mu.Lock()
	defer b2.mu.Unlock()
	if len(b2.messageSearchIndexByToken) == 0 {
		t.Fatalf("expected restored search index, got %+v", b2.messageSearchIndexByToken)
	}
	if len(b2.observabilityHistory) == 0 {
		t.Fatalf("expected restored observability history, got %+v", b2.observabilityHistory)
	}
	foundAnswered := false
	for _, req := range b2.requests {
		if req.ID == created.Request.ID {
			foundAnswered = true
			if req.Status != "answered" || req.Answered == nil {
				t.Fatalf("expected answered request after restore, got %+v", req)
			}
		}
	}
	if !foundAnswered {
		t.Fatalf("expected restored request %q, got %+v", created.Request.ID, b2.requests)
	}
}

func TestLoadStateFallsBackToCloudBackupWhenLocalStateMissing(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "broker-state.json")
	brokerStatePath = func() string { return statePath }
	defer func() { brokerStatePath = oldPathFn }()
	oldAsync := runBrokerCloudBackupAsync
	runBrokerCloudBackupAsync = func(fn func()) { fn() }
	defer func() { runBrokerCloudBackupAsync = oldAsync }()

	t.Setenv("HOME", tmpDir)
	t.Setenv("WUPHF_CONFIG_PATH", filepath.Join(tmpDir, ".wuphf", "config.json"))
	t.Setenv("WUPHF_CLOUD_BACKUP_PROVIDER", backup.ProviderGCS)
	t.Setenv("WUPHF_CLOUD_BACKUP_BUCKET", "dunderia-backups")
	t.Setenv("WUPHF_CLOUD_BACKUP_PREFIX", "office-a")

	sink := &testBackupSink{
		body: map[string]string{
			"office-a/team/broker-state.json":           `{"messages":[{"id":"msg-1","channel":"general","from":"you","content":"from-cloud"}],"channels":[{"slug":"general","name":"general"}],"members":[{"slug":"ceo","name":"CEO"}]}`,
			"office-a/team/broker-state.last-good.json": `{"messages":[{"id":"msg-1","channel":"general","from":"you","content":"from-cloud"}],"channels":[{"slug":"general","name":"general"}],"members":[{"slug":"ceo","name":"CEO"}]}`,
		},
	}
	restore := backup.SetSinkFactoryForTest(func(_ context.Context, settings backup.Settings) (backup.Sink, error) {
		return sink, nil
	})
	defer restore()

	b := NewBroker()
	if len(b.messages) != 0 {
		t.Fatalf("expected local-first boot before deferred restore, got %+v", b.messages)
	}
	if !b.HasDeferredCloudRestore() {
		t.Fatalf("expected deferred cloud restore to be pending")
	}
	changes, unsubscribe := b.SubscribeOfficeChanges(1)
	defer unsubscribe()
	restored, err := b.RestoreDeferredCloudState()
	if err != nil {
		t.Fatalf("restore deferred cloud state: %v", err)
	}
	if !restored {
		t.Fatalf("expected deferred cloud restore to apply")
	}
	if len(b.messages) == 0 || strings.TrimSpace(b.messages[0].Content) != "from-cloud" {
		t.Fatalf("expected cloud-restored broker messages, got %+v", b.messages)
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("expected local broker state rehydrated, got %v", err)
	}
	select {
	case evt := <-changes:
		if evt.Kind != "state_restored" {
			t.Fatalf("expected state_restored event, got %+v", evt)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for state_restored event")
	}
}

func TestLoadStateFallsBackToCloudBackupUsingBootstrapFile(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "broker-state.json")
	brokerStatePath = func() string { return statePath }
	defer func() { brokerStatePath = oldPathFn }()
	oldAsync := runBrokerCloudBackupAsync
	runBrokerCloudBackupAsync = func(fn func()) { fn() }
	defer func() { runBrokerCloudBackupAsync = oldAsync }()

	t.Setenv("HOME", tmpDir)
	t.Setenv("WUPHF_CONFIG_PATH", filepath.Join(tmpDir, ".wuphf", "config.json"))
	t.Setenv("WUPHF_CLOUD_BACKUP_BOOTSTRAP_PATH", filepath.Join(tmpDir, ".wuphf", "cloud-backup-bootstrap.json"))

	if err := os.MkdirAll(filepath.Join(tmpDir, ".wuphf"), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".wuphf", "cloud-backup-bootstrap.json"), []byte("{\n  \"provider\": \"gcs\",\n  \"bucket\": \"dunderia-backups\",\n  \"prefix\": \"office-a\"\n}\n"), 0o600); err != nil {
		t.Fatalf("write bootstrap: %v", err)
	}

	sink := &testBackupSink{
		body: map[string]string{
			"office-a/team/broker-state.json":           `{"messages":[{"id":"msg-1","channel":"general","from":"you","content":"from-cloud-bootstrap"}],"channels":[{"slug":"general","name":"general"}],"members":[{"slug":"ceo","name":"CEO"}]}`,
			"office-a/team/broker-state.last-good.json": `{"messages":[{"id":"msg-1","channel":"general","from":"you","content":"from-cloud-bootstrap"}],"channels":[{"slug":"general","name":"general"}],"members":[{"slug":"ceo","name":"CEO"}]}`,
		},
	}
	restore := backup.SetSinkFactoryForTest(func(_ context.Context, settings backup.Settings) (backup.Sink, error) {
		if settings.Provider != backup.ProviderGCS || settings.Bucket != "dunderia-backups" || settings.Prefix != "office-a" {
			t.Fatalf("unexpected bootstrap settings: %+v", settings)
		}
		return sink, nil
	})
	defer restore()

	b := NewBroker()
	if len(b.messages) != 0 {
		t.Fatalf("expected local-first boot before deferred restore, got %+v", b.messages)
	}
	restored, err := b.RestoreDeferredCloudState()
	if err != nil {
		t.Fatalf("restore deferred cloud state from bootstrap: %v", err)
	}
	if !restored {
		t.Fatalf("expected bootstrap-backed deferred restore to apply")
	}
	if len(b.messages) == 0 || strings.TrimSpace(b.messages[0].Content) != "from-cloud-bootstrap" {
		t.Fatalf("expected cloud-restored broker messages from bootstrap, got %+v", b.messages)
	}
}

func TestRestoreDeferredCloudStateSkipsWhenBrokerMutatedAfterBoot(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "broker-state.json")
	brokerStatePath = func() string { return statePath }
	defer func() { brokerStatePath = oldPathFn }()
	oldAsync := runBrokerCloudBackupAsync
	runBrokerCloudBackupAsync = func(fn func()) { fn() }
	defer func() { runBrokerCloudBackupAsync = oldAsync }()

	t.Setenv("HOME", tmpDir)
	t.Setenv("WUPHF_CONFIG_PATH", filepath.Join(tmpDir, ".wuphf", "config.json"))
	t.Setenv("WUPHF_CLOUD_BACKUP_PROVIDER", backup.ProviderGCS)
	t.Setenv("WUPHF_CLOUD_BACKUP_BUCKET", "dunderia-backups")
	t.Setenv("WUPHF_CLOUD_BACKUP_PREFIX", "office-a")

	sink := &testBackupSink{
		body: map[string]string{
			"office-a/team/broker-state.json":           `{"messages":[{"id":"msg-1","channel":"general","from":"you","content":"from-cloud"}],"channels":[{"slug":"general","name":"general"}],"members":[{"slug":"ceo","name":"CEO"}]}`,
			"office-a/team/broker-state.last-good.json": `{"messages":[{"id":"msg-1","channel":"general","from":"you","content":"from-cloud"}],"channels":[{"slug":"general","name":"general"}],"members":[{"slug":"ceo","name":"CEO"}]}`,
		},
	}
	restore := backup.SetSinkFactoryForTest(func(_ context.Context, settings backup.Settings) (backup.Sink, error) {
		return sink, nil
	})
	defer restore()

	b := NewBroker()
	if !b.HasDeferredCloudRestore() {
		t.Fatalf("expected deferred cloud restore to be pending")
	}
	if _, err := b.PostMessage("you", "general", "local mutation wins", nil, ""); err != nil {
		t.Fatalf("post local message: %v", err)
	}

	restored, err := b.RestoreDeferredCloudState()
	if err != nil {
		t.Fatalf("restore deferred cloud state: %v", err)
	}
	if restored {
		t.Fatalf("expected deferred cloud restore to skip after local mutation")
	}
	if len(b.messages) != 1 || strings.TrimSpace(b.messages[0].Content) != "local mutation wins" {
		t.Fatalf("expected local mutation to remain authoritative, got %+v", b.messages)
	}
}

func TestSaveLockedPrunesRemoteCurrentStateWhenBrokerBecomesEmpty(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "broker-state.json")
	brokerStatePath = func() string { return statePath }
	defer func() { brokerStatePath = oldPathFn }()
	oldAsync := runBrokerCloudBackupAsync
	runBrokerCloudBackupAsync = func(fn func()) { fn() }
	defer func() { runBrokerCloudBackupAsync = oldAsync }()

	t.Setenv("HOME", tmpDir)
	t.Setenv("WUPHF_CONFIG_PATH", filepath.Join(tmpDir, ".wuphf", "config.json"))
	if err := os.MkdirAll(filepath.Join(tmpDir, ".wuphf"), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	cfgBytes, err := json.Marshal(config.Config{
		CloudBackupProvider: backup.ProviderGCS,
		CloudBackupBucket:   "dunderia-backups",
		CloudBackupPrefix:   "office-a",
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".wuphf", "config.json"), append(cfgBytes, '\n'), 0o600); err != nil {
		t.Fatalf("seed config file: %v", err)
	}

	sink := &testBackupSink{
		body: map[string]string{
			"office-a/team/broker-state.json":           `{"messages":[{"id":"msg-1"}]}`,
			"office-a/team/broker-state.last-good.json": `{"messages":[{"id":"msg-1"}]}`,
		},
	}
	restore := backup.SetSinkFactoryForTest(func(_ context.Context, settings backup.Settings) (backup.Sink, error) {
		return sink, nil
	})
	defer restore()

	b := NewBroker()
	b.Reset()

	if _, ok := sink.body["office-a/team/broker-state.json"]; ok {
		t.Fatalf("expected remote broker-state.json to be pruned, got %#v", sink.body)
	}
	if _, ok := sink.body["office-a/team/broker-state.last-good.json"]; ok {
		t.Fatalf("expected remote broker-state.last-good.json to be pruned, got %#v", sink.body)
	}
}
