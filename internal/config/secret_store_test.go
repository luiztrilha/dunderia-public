package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretStorePutGetListDeleteEncrypted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.enc.json")
	store, err := NewSecretStore(path, "correct horse battery staple")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	if err := store.Put("openai_api_key", "sk-test-secret"); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := store.Put("brave_api_key", "brave-secret"); err != nil {
		t.Fatalf("put second: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read encrypted file: %v", err)
	}
	if strings.Contains(string(raw), "sk-test-secret") || strings.Contains(string(raw), "brave-secret") {
		t.Fatalf("encrypted store leaked plaintext: %s", string(raw))
	}

	value, ok, err := store.Get("openai_api_key")
	if err != nil || !ok || value != "sk-test-secret" {
		t.Fatalf("get: value=%q ok=%t err=%v", value, ok, err)
	}
	names, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if strings.Join(names, ",") != "brave_api_key,openai_api_key" {
		t.Fatalf("unexpected names: %+v", names)
	}
	deleted, err := store.Delete("openai_api_key")
	if err != nil || !deleted {
		t.Fatalf("delete: deleted=%t err=%v", deleted, err)
	}
	_, ok, err = store.Get("openai_api_key")
	if err != nil || ok {
		t.Fatalf("deleted secret still present: ok=%t err=%v", ok, err)
	}
}

func TestSecretStoreRejectsWrongPassphrase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.enc.json")
	store, err := NewSecretStore(path, "right-passphrase")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.Put("token", "secret"); err != nil {
		t.Fatalf("put: %v", err)
	}

	wrong, err := NewSecretStore(path, "wrong-passphrase")
	if err != nil {
		t.Fatalf("new wrong store: %v", err)
	}
	if _, _, err := wrong.Get("token"); err == nil {
		t.Fatal("expected wrong passphrase to fail")
	}
}

func TestSecretStoreRejectsInvalidNames(t *testing.T) {
	store, err := NewSecretStore(filepath.Join(t.TempDir(), "secrets.enc.json"), "passphrase")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	for _, name := range []string{"", "../token", " token", "token/value", "token:value"} {
		if err := store.Put(name, "secret"); err == nil {
			t.Fatalf("expected invalid name %q to fail", name)
		}
	}
}

func TestNewSecretStoreFromEnv(t *testing.T) {
	t.Setenv("WUPHF_SECRET_STORE_PASSPHRASE", "env-passphrase")
	path := filepath.Join(t.TempDir(), "secrets.enc.json")
	store, err := NewSecretStoreFromEnv(path)
	if err != nil {
		t.Fatalf("new store from env: %v", err)
	}
	if got := store.Path(); got != path {
		t.Fatalf("path = %q, want %q", got, path)
	}
}

func TestMigrateConfigSecretsToStoreCopiesAndPreservesPlaintextByDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WUPHF_CONFIG_PATH", filepath.Join(dir, "config.json"))
	store, err := NewSecretStore(filepath.Join(dir, "secrets.enc.json"), "passphrase")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := Save(Config{
		OpenAIAPIKey:        "openai-secret",
		TelegramBotToken:    "telegram-secret",
		CompanyName:         "not secret",
		CompanyPriority:     "also not secret",
		TaskReminderMinutes: 7,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	results, err := MigrateConfigSecretsToStore(store, false)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !migrationResult(results, "openai_api_key").Migrated || !migrationResult(results, "telegram_bot_token").Migrated {
		t.Fatalf("expected api keys to migrate, got %+v", results)
	}
	value, ok, err := store.Get("openai_api_key")
	if err != nil || !ok || value != "openai-secret" {
		t.Fatalf("store get: value=%q ok=%t err=%v", value, ok, err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.OpenAIAPIKey != "openai-secret" || cfg.TelegramBotToken != "telegram-secret" {
		t.Fatalf("plaintext should be preserved by default, got %+v", cfg)
	}
}

func TestMigrateConfigSecretsToStoreCanClearPlaintext(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WUPHF_CONFIG_PATH", filepath.Join(dir, "config.json"))
	store, err := NewSecretStore(filepath.Join(dir, "secrets.enc.json"), "passphrase")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := Save(Config{
		ComposioAPIKey: "composio-secret",
		BraveAPIKey:    "brave-secret",
		CompanyName:    "DunderIA",
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	results, err := MigrateConfigSecretsToStore(store, true)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !migrationResult(results, "composio_api_key").Cleared || !migrationResult(results, "brave_api_key").Cleared {
		t.Fatalf("expected migrated keys to clear, got %+v", results)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.ComposioAPIKey != "" || cfg.BraveAPIKey != "" || cfg.CompanyName != "DunderIA" {
		t.Fatalf("unexpected config after clear: %+v", cfg)
	}
	value, ok, err := store.Get("brave_api_key")
	if err != nil || !ok || value != "brave-secret" {
		t.Fatalf("store get: value=%q ok=%t err=%v", value, ok, err)
	}
}

func migrationResult(results []ConfigSecretCandidate, name string) ConfigSecretCandidate {
	for _, result := range results {
		if result.Name == name {
			return result
		}
	}
	return ConfigSecretCandidate{}
}
