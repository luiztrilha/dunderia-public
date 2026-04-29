package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/crypto/scrypt"
)

const (
	secretStoreVersion = 1
	secretStoreKDF     = "scrypt"
)

var (
	secretNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
	secretStoreAAD    = []byte("wuphf-secret-store-v1")
)

type encryptedSecretStoreFile struct {
	Version    int    `json:"version"`
	KDF        string `json:"kdf"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type SecretStore struct {
	path       string
	passphrase string
}

type ConfigSecretCandidate struct {
	Name     string `json:"name"`
	Present  bool   `json:"present"`
	Migrated bool   `json:"migrated,omitempty"`
	Cleared  bool   `json:"cleared,omitempty"`
}

func SecretStorePath() string {
	if p := strings.TrimSpace(os.Getenv("WUPHF_SECRET_STORE_PATH")); p != "" {
		return p
	}
	configPath := ConfigPath()
	if strings.TrimSpace(configPath) != "" {
		return filepath.Join(filepath.Dir(configPath), "secrets.enc.json")
	}
	home := RuntimeHomeDir()
	if strings.TrimSpace(home) == "" {
		return filepath.Join(".wuphf", "secrets.enc.json")
	}
	return filepath.Join(home, ".wuphf", "secrets.enc.json")
}

func NewSecretStore(path, passphrase string) (*SecretStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = SecretStorePath()
	}
	passphrase = strings.TrimSpace(passphrase)
	if passphrase == "" {
		return nil, errors.New("secret store passphrase is required")
	}
	return &SecretStore{path: path, passphrase: passphrase}, nil
}

func NewSecretStoreFromEnv(path string) (*SecretStore, error) {
	return NewSecretStore(path, os.Getenv("WUPHF_SECRET_STORE_PASSPHRASE"))
}

func ConfigSecretCandidates(cfg Config) []ConfigSecretCandidate {
	fields := configSecretFields(&cfg)
	out := make([]ConfigSecretCandidate, 0, len(fields))
	for _, field := range fields {
		out = append(out, ConfigSecretCandidate{
			Name:    field.name,
			Present: strings.TrimSpace(*field.value) != "",
		})
	}
	return out
}

func MigrateConfigSecretsToStore(store *SecretStore, clearPlaintext bool) ([]ConfigSecretCandidate, error) {
	if store == nil {
		return nil, errors.New("secret store is not configured")
	}
	cfg, err := Load()
	if err != nil {
		return nil, err
	}
	fields := configSecretFields(&cfg)
	out := make([]ConfigSecretCandidate, 0, len(fields))
	for _, field := range fields {
		value := strings.TrimSpace(*field.value)
		candidate := ConfigSecretCandidate{
			Name:    field.name,
			Present: value != "",
		}
		if value != "" {
			if err := store.Put(field.name, value); err != nil {
				return nil, err
			}
			candidate.Migrated = true
			if clearPlaintext {
				*field.value = ""
				candidate.Cleared = true
			}
		}
		out = append(out, candidate)
	}
	if clearPlaintext {
		if err := Save(cfg); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *SecretStore) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *SecretStore) Put(name, value string) error {
	if err := validateSecretName(name); err != nil {
		return err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("secret value is required")
	}
	secrets, err := s.load()
	if err != nil {
		return err
	}
	secrets[strings.TrimSpace(name)] = value
	return s.save(secrets)
}

func (s *SecretStore) Get(name string) (string, bool, error) {
	if err := validateSecretName(name); err != nil {
		return "", false, err
	}
	secrets, err := s.load()
	if err != nil {
		return "", false, err
	}
	value, ok := secrets[strings.TrimSpace(name)]
	return value, ok, nil
}

func (s *SecretStore) Delete(name string) (bool, error) {
	if err := validateSecretName(name); err != nil {
		return false, err
	}
	secrets, err := s.load()
	if err != nil {
		return false, err
	}
	name = strings.TrimSpace(name)
	if _, ok := secrets[name]; !ok {
		return false, nil
	}
	delete(secrets, name)
	return true, s.save(secrets)
}

func (s *SecretStore) List() ([]string, error) {
	secrets, err := s.load()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(secrets))
	for name := range secrets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (s *SecretStore) load() (map[string]string, error) {
	if s == nil || strings.TrimSpace(s.path) == "" || strings.TrimSpace(s.passphrase) == "" {
		return nil, errors.New("secret store is not configured")
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	var file encryptedSecretStoreFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	plain, err := decryptSecretStore(file, s.passphrase)
	if err != nil {
		return nil, err
	}
	var secrets map[string]string
	if err := json.Unmarshal(plain, &secrets); err != nil {
		return nil, err
	}
	if secrets == nil {
		secrets = map[string]string{}
	}
	return secrets, nil
}

func (s *SecretStore) save(secrets map[string]string) error {
	if s == nil || strings.TrimSpace(s.path) == "" || strings.TrimSpace(s.passphrase) == "" {
		return errors.New("secret store is not configured")
	}
	if secrets == nil {
		secrets = map[string]string{}
	}
	plain, err := json.Marshal(secrets)
	if err != nil {
		return err
	}
	file, err := encryptSecretStore(plain, s.passphrase)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return writeFileAtomically(s.path, data, 0o600)
}

func encryptSecretStore(plain []byte, passphrase string) (encryptedSecretStoreFile, error) {
	salt, err := randomBytes(16)
	if err != nil {
		return encryptedSecretStoreFile{}, err
	}
	key, err := deriveSecretStoreKey(passphrase, salt)
	if err != nil {
		return encryptedSecretStoreFile{}, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return encryptedSecretStoreFile{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return encryptedSecretStoreFile{}, err
	}
	nonce, err := randomBytes(gcm.NonceSize())
	if err != nil {
		return encryptedSecretStoreFile{}, err
	}
	ciphertext := gcm.Seal(nil, nonce, plain, secretStoreAAD)
	return encryptedSecretStoreFile{
		Version:    secretStoreVersion,
		KDF:        secretStoreKDF,
		Salt:       base64.StdEncoding.EncodeToString(salt),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}, nil
}

func decryptSecretStore(file encryptedSecretStoreFile, passphrase string) ([]byte, error) {
	if file.Version != secretStoreVersion {
		return nil, fmt.Errorf("unsupported secret store version %d", file.Version)
	}
	if strings.TrimSpace(file.KDF) != secretStoreKDF {
		return nil, fmt.Errorf("unsupported secret store kdf %q", file.KDF)
	}
	salt, err := base64.StdEncoding.DecodeString(file.Salt)
	if err != nil {
		return nil, err
	}
	nonce, err := base64.StdEncoding.DecodeString(file.Nonce)
	if err != nil {
		return nil, err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(file.Ciphertext)
	if err != nil {
		return nil, err
	}
	key, err := deriveSecretStoreKey(passphrase, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, errors.New("invalid secret store nonce size")
	}
	return gcm.Open(nil, nonce, ciphertext, secretStoreAAD)
}

func deriveSecretStoreKey(passphrase string, salt []byte) ([]byte, error) {
	if strings.TrimSpace(passphrase) == "" {
		return nil, errors.New("secret store passphrase is required")
	}
	if len(salt) == 0 {
		return nil, errors.New("secret store salt is required")
	}
	return scrypt.Key([]byte(passphrase), salt, 32768, 8, 1, 32)
}

func randomBytes(n int) ([]byte, error) {
	out := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, out); err != nil {
		return nil, err
	}
	return out, nil
}

func validateSecretName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return errors.New("secret name is required")
	}
	if trimmed != name {
		return fmt.Errorf("invalid secret name %q", name)
	}
	if !secretNamePattern.MatchString(name) {
		return fmt.Errorf("invalid secret name %q", name)
	}
	return nil
}

type configSecretField struct {
	name  string
	value *string
}

func configSecretFields(cfg *Config) []configSecretField {
	return []configSecretField{
		{name: "api_key", value: &cfg.APIKey},
		{name: "one_api_key", value: &cfg.OneAPIKey},
		{name: "composio_api_key", value: &cfg.ComposioAPIKey},
		{name: "gemini_api_key", value: &cfg.GeminiAPIKey},
		{name: "anthropic_api_key", value: &cfg.AnthropicAPIKey},
		{name: "openai_api_key", value: &cfg.OpenAIAPIKey},
		{name: "minimax_api_key", value: &cfg.MinimaxAPIKey},
		{name: "brave_api_key", value: &cfg.BraveAPIKey},
		{name: "telegram_bot_token", value: &cfg.TelegramBotToken},
	}
}
