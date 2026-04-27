package lightstate

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/nex-crm/wuphf/internal/backup"
)

type fakeSink struct {
	body map[string][]byte
}

func (f *fakeSink) Put(_ context.Context, key string, data []byte, _ string) error {
	if f.body == nil {
		f.body = make(map[string][]byte)
	}
	f.body[key] = append([]byte(nil), data...)
	return nil
}

func (f *fakeSink) Get(_ context.Context, key string) ([]byte, error) {
	if body, ok := f.body[key]; ok {
		return append([]byte(nil), body...), nil
	}
	return nil, os.ErrNotExist
}

func (f *fakeSink) Delete(_ context.Context, key string) error {
	delete(f.body, key)
	return nil
}

func (f *fakeSink) Close() error { return nil }

func TestWriteFileAtomicallyReplacesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	if err := writeFileAtomically(path, []byte("new"), 0o600); err != nil {
		t.Fatalf("writeFileAtomically: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read replaced file: %v", err)
	}
	if string(data) != "new" {
		t.Fatalf("unexpected replacement content: %q", data)
	}
}

func TestSyncDefaultStateMirrorsAndRestoresLightweightAssets(t *testing.T) {
	sourceHome := t.TempDir()
	sourceAppData := filepath.Join(sourceHome, "AppData", "Roaming")
	if err := os.MkdirAll(sourceAppData, 0o700); err != nil {
		t.Fatalf("mkdir appdata: %v", err)
	}
	sourceADC := filepath.Join(sourceAppData, "gcloud", "application_default_credentials.json")
	seedDefaultLightweightAssets(t, sourceHome, sourceADC)

	sink := &fakeSink{}
	restore := backup.SetSinkFactoryForTest(func(_ context.Context, settings backup.Settings) (backup.Sink, error) {
		if settings.Provider != backup.ProviderGCS || settings.Bucket != "dunderia-backups" || settings.Prefix != "office-a" {
			t.Fatalf("unexpected settings: %+v", settings)
		}
		return sink, nil
	})
	defer restore()

	settings := backup.Settings{Provider: backup.ProviderGCS, Bucket: "dunderia-backups", Prefix: "office-a"}

	t.Setenv("HOME", sourceHome)
	t.Setenv("APPDATA", sourceAppData)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", sourceADC)
	report, err := SyncDefaultState(settings)
	if err != nil {
		t.Fatalf("SyncDefaultState: %v", err)
	}
	if len(report.Restored) != 0 {
		t.Fatalf("expected no restored assets on source machine, got %v", report.Restored)
	}
	wantMirrored := []string{
		"company.json",
		"onboarded.json",
		"cloud-backup-bootstrap.json",
		".codex/auth.json",
		".codex/config.toml",
		".codex/skills",
		".agents/skills",
		"google-adc",
	}
	if !reflect.DeepEqual(report.Mirrored, wantMirrored) {
		t.Fatalf("unexpected mirrored assets:\n got: %v\nwant: %v", report.Mirrored, wantMirrored)
	}

	targetHome := t.TempDir()
	targetAppData := filepath.Join(targetHome, "AppData", "Roaming")
	if err := os.MkdirAll(targetAppData, 0o700); err != nil {
		t.Fatalf("mkdir target appdata: %v", err)
	}
	targetADC := filepath.Join(targetAppData, "gcloud", "application_default_credentials.json")
	t.Setenv("HOME", targetHome)
	t.Setenv("APPDATA", targetAppData)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", targetADC)

	restored, err := RestoreDefaultState(settings)
	if err != nil {
		t.Fatalf("RestoreDefaultState: %v", err)
	}
	slices.Sort(restored)
	wantRestored := append([]string(nil), wantMirrored...)
	slices.Sort(wantRestored)
	if !reflect.DeepEqual(restored, wantRestored) {
		t.Fatalf("unexpected restored assets:\n got: %v\nwant: %v", restored, wantRestored)
	}

	assertFileContent(t, filepath.Join(targetHome, ".wuphf", "company.json"), `{"name":"DunderIA"}`)
	assertFileContent(t, filepath.Join(targetHome, ".wuphf", "onboarded.json"), `{"version":1}`)
	assertFileContent(t, filepath.Join(targetHome, ".wuphf", "cloud-backup-bootstrap.json"), `{"provider":"gcs"}`)
	assertFileContent(t, filepath.Join(targetHome, ".codex", "auth.json"), `{"access_token":"token"}`)
	assertFileContent(t, filepath.Join(targetHome, ".codex", "config.toml"), `model = "gpt-5.4"`)
	assertFileContent(t, filepath.Join(targetHome, ".codex", "skills", "custom", "SKILL.md"), "# Skill")
	assertFileContent(t, filepath.Join(targetHome, ".agents", "skills", "agent-skill", "SKILL.md"), "# Agent Skill")
	assertFileContent(t, targetADC, `{"client_id":"abc"}`)
}

func TestMirrorAndRestoreDedicatedOfficeFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	companyPath := filepath.Join(home, ".wuphf", "company.json")
	onboardedPath := filepath.Join(home, ".wuphf", "onboarded.json")
	if err := os.MkdirAll(filepath.Dir(companyPath), 0o700); err != nil {
		t.Fatalf("mkdir office dir: %v", err)
	}
	if err := os.WriteFile(companyPath, []byte(`{"name":"Office"}`), 0o600); err != nil {
		t.Fatalf("write company: %v", err)
	}
	if err := os.WriteFile(onboardedPath, []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatalf("write onboarded: %v", err)
	}

	sink := &fakeSink{}
	restore := backup.SetSinkFactoryForTest(func(_ context.Context, _ backup.Settings) (backup.Sink, error) {
		return sink, nil
	})
	defer restore()

	settings := backup.Settings{Provider: backup.ProviderGCS, Bucket: "dunderia-backups", Prefix: "office-a"}
	if err := MirrorCompany(settings, companyPath); err != nil {
		t.Fatalf("MirrorCompany: %v", err)
	}
	if err := MirrorOnboarded(settings, onboardedPath); err != nil {
		t.Fatalf("MirrorOnboarded: %v", err)
	}

	if err := os.Remove(companyPath); err != nil {
		t.Fatalf("remove company: %v", err)
	}
	if err := os.Remove(onboardedPath); err != nil {
		t.Fatalf("remove onboarded: %v", err)
	}

	companyRestored, err := RestoreCompanyIfMissing(settings, companyPath)
	if err != nil {
		t.Fatalf("RestoreCompanyIfMissing: %v", err)
	}
	if !companyRestored {
		t.Fatal("expected company.json to be restored")
	}
	onboardedRestored, err := RestoreOnboardedIfMissing(settings, onboardedPath)
	if err != nil {
		t.Fatalf("RestoreOnboardedIfMissing: %v", err)
	}
	if !onboardedRestored {
		t.Fatal("expected onboarded.json to be restored")
	}
}

func seedDefaultLightweightAssets(t *testing.T, home string, adcPath string) {
	t.Helper()
	writeTestFile(t, filepath.Join(home, ".wuphf", "company.json"), `{"name":"DunderIA"}`)
	writeTestFile(t, filepath.Join(home, ".wuphf", "onboarded.json"), `{"version":1}`)
	writeTestFile(t, filepath.Join(home, ".wuphf", "cloud-backup-bootstrap.json"), `{"provider":"gcs"}`)
	writeTestFile(t, filepath.Join(home, ".codex", "auth.json"), `{"access_token":"token"}`)
	writeTestFile(t, filepath.Join(home, ".codex", "config.toml"), `model = "gpt-5.4"`)
	writeTestFile(t, filepath.Join(home, ".codex", "skills", "custom", "SKILL.md"), "# Skill")
	writeTestFile(t, filepath.Join(home, ".agents", "skills", "agent-skill", "SKILL.md"), "# Agent Skill")
	writeTestFile(t, adcPath, `{"client_id":"abc"}`)
}

func writeTestFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertFileContent(t *testing.T, path string, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("unexpected content for %s:\n got: %s\nwant: %s", path, string(data), want)
	}
}
