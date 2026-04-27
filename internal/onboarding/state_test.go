package onboarding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nex-crm/wuphf/internal/backup"
)

type onboardingBackupSink struct {
	body   map[string][]byte
	putErr error
}

func (f *onboardingBackupSink) Put(_ context.Context, key string, data []byte, _ string) error {
	if f.putErr != nil {
		return f.putErr
	}
	if f.body == nil {
		f.body = make(map[string][]byte)
	}
	f.body[key] = append([]byte(nil), data...)
	return nil
}

func (f *onboardingBackupSink) Get(_ context.Context, key string) ([]byte, error) {
	if body, ok := f.body[key]; ok {
		return append([]byte(nil), body...), nil
	}
	return nil, os.ErrNotExist
}

func (f *onboardingBackupSink) Delete(_ context.Context, key string) error {
	delete(f.body, key)
	return nil
}

func (f *onboardingBackupSink) Close() error { return nil }

func captureOnboardingLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	prevPrefix := log.Prefix()
	log.SetOutput(&buf)
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
		log.SetPrefix(prevPrefix)
	})
	return &buf
}

// withTempHome redirects os.UserHomeDir (via $HOME) to a temp dir for
// the duration of f, keeping test state isolated from the real ~/.wuphf.
func withTempHome(t *testing.T, f func(home string)) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	f(dir)
}

func TestLoadFreshInstallReturnsNotOnboarded(t *testing.T) {
	withTempHome(t, func(_ string) {
		s, err := Load()
		if err != nil {
			t.Fatalf("Load: unexpected error: %v", err)
		}
		if s.Onboarded() {
			t.Fatal("fresh install should not be onboarded")
		}
		if s.Version != currentStateVersion {
			t.Fatalf("Version: got %d, want %d", s.Version, currentStateVersion)
		}
		if len(s.Checklist) == 0 {
			t.Fatal("fresh install should have a default checklist")
		}
	})
}

func TestLoadDoesNotRestoreCloudBackupWhenLocalStateMissing(t *testing.T) {
	withTempHome(t, func(_ string) {
		t.Setenv("WUPHF_CLOUD_BACKUP_PROVIDER", backup.ProviderGCS)
		t.Setenv("WUPHF_CLOUD_BACKUP_BUCKET", "dunderia-backups")
		t.Setenv("WUPHF_CLOUD_BACKUP_PREFIX", "office-a")

		sink := &onboardingBackupSink{
			body: map[string][]byte{
				"office-a/state/onboarded.json": []byte(`{"completed_at":"2026-04-14T10:23:00Z","version":1,"company_name":"Restored Corp","checklist":[{"id":"pick_team","done":true}]}`),
			},
		}
		restore := backup.SetSinkFactoryForTest(func(_ context.Context, _ backup.Settings) (backup.Sink, error) {
			return sink, nil
		})
		defer restore()

		s, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if s.Onboarded() {
			t.Fatal("fresh install should stay not onboarded until restore is explicit")
		}
		if s.CompanyName != "" {
			t.Fatalf("expected no restored company name, got %q", s.CompanyName)
		}
		if _, err := os.Stat(StatePath()); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected onboarded.json to stay missing until explicit restore, got %v", err)
		}
	})
}

func TestSaveKeepsLocalWriteWhenCloudMirrorFails(t *testing.T) {
	withTempHome(t, func(_ string) {
		t.Setenv("WUPHF_CLOUD_BACKUP_PROVIDER", backup.ProviderGCS)
		t.Setenv("WUPHF_CLOUD_BACKUP_BUCKET", "dunderia-backups")
		t.Setenv("WUPHF_CLOUD_BACKUP_PREFIX", "office-a")
		logs := captureOnboardingLogs(t)

		sink := &onboardingBackupSink{putErr: errors.New("mirror unavailable")}
		restore := backup.SetSinkFactoryForTest(func(_ context.Context, _ backup.Settings) (backup.Sink, error) {
			return sink, nil
		})
		defer restore()

		original := &State{
			Version:     currentStateVersion,
			CompanyName: "Local First Corp",
			Checklist:   DefaultChecklist(),
		}
		if err := Save(original); err != nil {
			t.Fatalf("Save should preserve local success when mirror fails: %v", err)
		}
		if _, err := os.Stat(StatePath()); err != nil {
			t.Fatalf("expected onboarded.json written locally: %v", err)
		}
		loaded, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if loaded.CompanyName != original.CompanyName {
			t.Fatalf("expected local onboarding state persisted, got %+v", loaded)
		}
		if !strings.Contains(logs.String(), "onboarding: cloud mirror failed after local write: mirror unavailable") {
			t.Fatalf("expected mirror failure to be logged, got %q", logs.String())
		}
	})
}

func TestLoadDefaultChecklistItems(t *testing.T) {
	withTempHome(t, func(_ string) {
		s, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		defaults := DefaultChecklist()
		if len(s.Checklist) != len(defaults) {
			t.Fatalf("checklist length: got %d, want %d", len(s.Checklist), len(defaults))
		}
		for i, item := range s.Checklist {
			if item.ID != defaults[i].ID {
				t.Errorf("checklist[%d].ID: got %q, want %q", i, item.ID, defaults[i].ID)
			}
			if item.Done {
				t.Errorf("checklist[%d] should not be done on fresh install", i)
			}
		}
	})
}

func TestLoadExistingFileReturnsCorrectData(t *testing.T) {
	withTempHome(t, func(home string) {
		dir := filepath.Join(home, ".wuphf")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		raw := `{
			"completed_at": "2026-04-14T10:23:00Z",
			"version": 1,
			"company_name": "Dunder Mifflin",
			"completed_steps": ["welcome", "setup"],
			"checklist_dismissed": false,
			"checklist": [
				{"id": "pick_team", "done": true},
				{"id": "second_key", "done": false},
				{"id": "github_repo", "done": false},
				{"id": "github_star", "done": false},
				{"id": "discord", "done": false}
			]
		}`
		if err := os.WriteFile(filepath.Join(dir, "onboarded.json"), []byte(raw), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		s, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if !s.Onboarded() {
			t.Fatal("expected Onboarded()==true")
		}
		if s.CompanyName != "Dunder Mifflin" {
			t.Errorf("CompanyName: got %q, want %q", s.CompanyName, "Dunder Mifflin")
		}
		if len(s.CompletedSteps) != 2 {
			t.Errorf("CompletedSteps length: got %d, want 2", len(s.CompletedSteps))
		}
		if !s.Checklist[0].Done {
			t.Error("checklist[0] (pick_team) should be done")
		}
	})
}

func TestSaveIsAtomic(t *testing.T) {
	withTempHome(t, func(home string) {
		s := &State{
			Version:     currentStateVersion,
			CompanyName: "Initech",
			Checklist:   DefaultChecklist(),
		}
		if err := Save(s); err != nil {
			t.Fatalf("Save: %v", err)
		}
		path := StatePath()
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("file should exist at %s after Save: %v", path, err)
		}
		// Ensure no temp file leaked.
		entries, _ := os.ReadDir(filepath.Dir(path))
		for _, e := range entries {
			if e.Name() != "onboarded.json" {
				t.Errorf("unexpected file in .wuphf dir after Save: %s", e.Name())
			}
		}
	})
}

func TestSaveReplacesFileAtomically(t *testing.T) {
	withTempHome(t, func(home string) {
		dir := filepath.Join(home, ".wuphf")
		path := filepath.Join(dir, "onboarded.json")
		shadowPath := filepath.Join(dir, "onboarded-shadow.json")

		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}

		initial, err := json.MarshalIndent(&State{
			Version:     currentStateVersion,
			CompanyName: "Old Corp",
			Checklist:   DefaultChecklist(),
		}, "", "  ")
		if err != nil {
			t.Fatalf("MarshalIndent: %v", err)
		}
		initial = append(initial, '\n')
		if err := os.WriteFile(path, initial, 0o600); err != nil {
			t.Fatalf("write initial state: %v", err)
		}
		if err := os.Link(path, shadowPath); err != nil {
			t.Skipf("hard links unavailable on this filesystem: %v", err)
		}

		updated := &State{
			Version:     currentStateVersion,
			CompanyName: "New Corp",
			Checklist:   DefaultChecklist(),
		}
		if err := Save(updated); err != nil {
			t.Fatalf("Save: %v", err)
		}

		currentData, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read current state: %v", err)
		}
		if !strings.Contains(string(currentData), "\"company_name\": \"New Corp\"") {
			t.Fatalf("expected current state to contain new content, got %s", currentData)
		}

		shadowData, err := os.ReadFile(shadowPath)
		if err != nil {
			t.Fatalf("read shadow state: %v", err)
		}
		if !strings.Contains(string(shadowData), "\"company_name\": \"Old Corp\"") {
			t.Fatalf("expected hard link to keep previous content after atomic replace, got %s", shadowData)
		}
	})
}

func TestSavePreservesExistingStateWhenReplaceFails(t *testing.T) {
	withTempHome(t, func(home string) {
		dir := filepath.Join(home, ".wuphf")
		path := filepath.Join(dir, "onboarded.json")

		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}

		initial, err := json.MarshalIndent(&State{
			Version:     currentStateVersion,
			CompanyName: "Old Corp",
			Checklist:   DefaultChecklist(),
		}, "", "  ")
		if err != nil {
			t.Fatalf("MarshalIndent: %v", err)
		}
		initial = append(initial, '\n')
		if err := os.WriteFile(path, initial, 0o600); err != nil {
			t.Fatalf("write initial state: %v", err)
		}

		origReplace := atomicReplaceStateFile
		atomicReplaceStateFile = func(tmp, dest string) error {
			if dest != path {
				t.Fatalf("unexpected replace destination: got %q want %q", dest, path)
			}
			if _, err := os.Stat(tmp); err != nil {
				t.Fatalf("expected temp file before replace: %v", err)
			}
			return os.ErrPermission
		}
		defer func() { atomicReplaceStateFile = origReplace }()

		updated := &State{
			Version:     currentStateVersion,
			CompanyName: "New Corp",
			Checklist:   DefaultChecklist(),
		}
		if err := Save(updated); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("Save error = %v, want %v", err, os.ErrPermission)
		}

		currentData, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read preserved state: %v", err)
		}
		if !strings.Contains(string(currentData), "\"company_name\": \"Old Corp\"") {
			t.Fatalf("expected previous state to survive failed replace, got %s", currentData)
		}
	})
}

func TestSaveFlushesTempFileAndDirectoryBeforeReturning(t *testing.T) {
	withTempHome(t, func(home string) {
		dir := filepath.Join(home, ".wuphf")
		path := filepath.Join(dir, "onboarded.json")
		var steps []string

		origSyncTemp := syncStateTempFile
		origReplace := atomicReplaceStateFile
		origSyncDir := syncStateDir
		syncStateTempFile = func(file *os.File) error {
			steps = append(steps, "sync-temp")
			if _, err := os.Stat(file.Name()); err != nil {
				t.Fatalf("expected temp file before sync: %v", err)
			}
			return nil
		}
		atomicReplaceStateFile = func(tmp, dest string) error {
			steps = append(steps, "replace")
			if dest != path {
				t.Fatalf("unexpected replace destination: got %q want %q", dest, path)
			}
			if got := strings.Join(steps, ","); got != "sync-temp,replace" {
				t.Fatalf("replace should happen after temp sync, got %s", got)
			}
			return os.Rename(tmp, dest)
		}
		syncStateDir = func(targetDir string) error {
			steps = append(steps, "sync-dir")
			if targetDir != dir {
				t.Fatalf("unexpected sync dir: got %q want %q", targetDir, dir)
			}
			if got := strings.Join(steps, ","); got != "sync-temp,replace,sync-dir" {
				t.Fatalf("directory sync should happen after replace, got %s", got)
			}
			return nil
		}
		defer func() {
			syncStateTempFile = origSyncTemp
			atomicReplaceStateFile = origReplace
			syncStateDir = origSyncDir
		}()

		updated := &State{
			Version:     currentStateVersion,
			CompanyName: "Durable Corp",
			Checklist:   DefaultChecklist(),
		}
		if err := Save(updated); err != nil {
			t.Fatalf("Save: %v", err)
		}
		if got := strings.Join(steps, ","); got != "sync-temp,replace,sync-dir" {
			t.Fatalf("unexpected operation order: %s", got)
		}
	})
}

func TestSaveRoundtrip(t *testing.T) {
	withTempHome(t, func(_ string) {
		original := &State{
			CompletedAt:        "2026-04-14T10:23:00Z",
			Version:            currentStateVersion,
			CompanyName:        "Paper Company",
			CompletedSteps:     []string{"welcome", "setup"},
			ChecklistDismissed: false,
			Checklist:          DefaultChecklist(),
		}
		if err := Save(original); err != nil {
			t.Fatalf("Save: %v", err)
		}
		loaded, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if loaded.CompanyName != original.CompanyName {
			t.Errorf("CompanyName: got %q, want %q", loaded.CompanyName, original.CompanyName)
		}
		if loaded.CompletedAt != original.CompletedAt {
			t.Errorf("CompletedAt: got %q, want %q", loaded.CompletedAt, original.CompletedAt)
		}
	})
}

func TestSaveProgressMergesCorrectly(t *testing.T) {
	withTempHome(t, func(_ string) {
		// First save progress for welcome step.
		welcomeAnswers := map[string]interface{}{
			"company_name": "Initech",
			"description":  "We do TPS reports",
		}
		if err := SaveProgress("welcome", welcomeAnswers); err != nil {
			t.Fatalf("SaveProgress welcome: %v", err)
		}

		// Then save progress for setup step.
		setupAnswers := map[string]interface{}{
			"anthropic_key": "sk-ant-test",
		}
		if err := SaveProgress("setup", setupAnswers); err != nil {
			t.Fatalf("SaveProgress setup: %v", err)
		}

		s, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if s.Partial == nil {
			t.Fatal("expected Partial to be non-nil")
		}
		if s.Partial.Step != "setup" {
			t.Errorf("Partial.Step: got %q, want %q", s.Partial.Step, "setup")
		}
		// Both steps' answers should be present.
		if _, ok := s.Partial.Answers["welcome"]; !ok {
			t.Error("expected welcome answers to be persisted")
		}
		if _, ok := s.Partial.Answers["setup"]; !ok {
			t.Error("expected setup answers to be persisted")
		}
		if s.Partial.Answers["welcome"]["company_name"] != "Initech" {
			t.Errorf("welcome company_name: got %v", s.Partial.Answers["welcome"]["company_name"])
		}
	})
}

func TestVersionBumpReturnsNotOnboarded(t *testing.T) {
	withTempHome(t, func(home string) {
		// Write a file that looks complete but with an old schema version.
		dir := filepath.Join(home, ".wuphf")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		stale := map[string]interface{}{
			"completed_at": time.Now().UTC().Format(time.RFC3339),
			"version":      0, // old version
			"company_name": "Old Corp",
		}
		data, _ := json.Marshal(stale)
		if err := os.WriteFile(filepath.Join(dir, "onboarded.json"), data, 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		s, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if s.Onboarded() {
			t.Fatal("stale version should return onboarded=false")
		}
		// Version should be upgraded to current.
		if s.Version != currentStateVersion {
			t.Errorf("Version: got %d, want %d", s.Version, currentStateVersion)
		}
	})
}

func TestMarkChecklistItem(t *testing.T) {
	withTempHome(t, func(_ string) {
		// Start fresh.
		if err := Save(&State{Version: currentStateVersion, Checklist: DefaultChecklist()}); err != nil {
			t.Fatalf("Save: %v", err)
		}
		if err := MarkChecklistItem("pick_team", true); err != nil {
			t.Fatalf("MarkChecklistItem: %v", err)
		}
		s, _ := Load()
		found := false
		for _, item := range s.Checklist {
			if item.ID == "pick_team" {
				found = true
				if !item.Done {
					t.Error("pick_team should be done")
				}
			}
		}
		if !found {
			t.Error("pick_team not found in checklist")
		}
	})
}

func TestDismissChecklist(t *testing.T) {
	withTempHome(t, func(_ string) {
		if err := Save(&State{Version: currentStateVersion, Checklist: DefaultChecklist()}); err != nil {
			t.Fatalf("Save: %v", err)
		}
		if err := DismissChecklist(); err != nil {
			t.Fatalf("DismissChecklist: %v", err)
		}
		s, _ := Load()
		if !s.ChecklistDismissed {
			t.Error("ChecklistDismissed should be true")
		}
	})
}
