// Package config handles loading, saving, and resolving WUPHF configuration.
// Resolution chain: CLI flag > environment variable > config file.
package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nex-crm/wuphf/internal/backup"
)

type cloudBackupBootstrap struct {
	Provider string `json:"provider,omitempty"`
	Bucket   string `json:"bucket,omitempty"`
	Prefix   string `json:"prefix,omitempty"`
}

var atomicReplaceFile = func(tmp, path string) error {
	return os.Rename(tmp, path)
}

func writeFileAtomically(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := atomicReplaceFile(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// RuntimeHomeDir returns the home directory WUPHF should use for persisted
// runtime state. Inventive runs may override this with WUPHF_RUNTIME_HOME so
// they don't inherit an existing office from the user's global ~/.wuphf.
func RuntimeHomeDir() string {
	if v := strings.TrimSpace(os.Getenv("WUPHF_RUNTIME_HOME")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("HOME")); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// Config mirrors ~/.wuphf/config.json.
type Config struct {
	APIKey              string `json:"api_key,omitempty"`
	MemoryBackend       string `json:"memory_backend,omitempty"`
	OneAPIKey           string `json:"one_api_key,omitempty"`
	ComposioAPIKey      string `json:"composio_api_key,omitempty"`
	ActionProvider      string `json:"action_provider,omitempty"`
	WebSearchProvider   string `json:"web_search_provider,omitempty"`
	CustomMCPConfig     string `json:"custom_mcp_config_path,omitempty"`
	Email               string `json:"email,omitempty"`
	WorkspaceID         string `json:"workspace_id,omitempty"`
	WorkspaceSlug       string `json:"workspace_slug,omitempty"`
	LLMProvider         string `json:"llm_provider,omitempty"`
	GeminiAPIKey        string `json:"gemini_api_key,omitempty"`
	AnthropicAPIKey     string `json:"anthropic_api_key,omitempty"`
	OpenAIAPIKey        string `json:"openai_api_key,omitempty"`
	MinimaxAPIKey       string `json:"minimax_api_key,omitempty"`
	BraveAPIKey         string `json:"brave_api_key,omitempty"`
	CloudBackupProvider string `json:"cloud_backup_provider,omitempty"`
	CloudBackupBucket   string `json:"cloud_backup_bucket,omitempty"`
	CloudBackupPrefix   string `json:"cloud_backup_prefix,omitempty"`
	Blueprint           string `json:"blueprint,omitempty"`
	// Pack is retained as a legacy alias for the active operation blueprint/template.
	Pack                string `json:"pack,omitempty"`
	TeamLeadSlug        string `json:"team_lead_slug,omitempty"`
	MaxConcurrent       int    `json:"max_concurrent_agents,omitempty"`
	DefaultFormat       string `json:"default_format,omitempty"`
	DefaultTimeout      int    `json:"default_timeout,omitempty"`
	DevURL              string `json:"dev_url,omitempty"`
	InsightsPollMinutes int    `json:"insights_poll_minutes,omitempty"`
	TaskFollowUpMinutes int    `json:"task_follow_up_minutes,omitempty"`
	TaskReminderMinutes int    `json:"task_reminder_minutes,omitempty"`
	TaskRecheckMinutes  int    `json:"task_recheck_minutes,omitempty"`
	TelegramBotToken    string `json:"telegram_bot_token,omitempty"`
	CompanyName         string `json:"company_name,omitempty"`
	CompanyDescription  string `json:"company_description,omitempty"`
	CompanyGoals        string `json:"company_goals,omitempty"`
	CompanySize         string `json:"company_size,omitempty"`
	CompanyPriority     string `json:"company_priority,omitempty"`
}

// NormalizeActionProvider returns a supported action provider or the empty string.
func NormalizeActionProvider(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "":
		return ""
	case "auto":
		return "auto"
	case "composio":
		return "composio"
	case "one":
		return "one"
	default:
		return ""
	}
}

// NormalizeWebSearchProvider returns a supported web search provider or the empty string.
func NormalizeWebSearchProvider(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "":
		return ""
	case "none":
		return "none"
	case "brave":
		return "brave"
	default:
		return ""
	}
}

const (
	MemoryBackendNone = "none"
)

// ActiveBlueprint returns the preferred operation blueprint/template id.
// Blueprint is the primary field; Pack remains as a compatibility alias.
func (c Config) ActiveBlueprint() string {
	if v := strings.TrimSpace(c.Blueprint); v != "" {
		return v
	}
	return strings.TrimSpace(c.Pack)
}

// SetActiveBlueprint stores the selected operation blueprint/template id in
// the preferred field. The legacy Pack alias is retained for reads only.
func (c *Config) SetActiveBlueprint(id string) {
	id = strings.TrimSpace(id)
	c.Blueprint = id
}

// ConfigPath returns the absolute path to ~/.wuphf/config.json, with a legacy
// fallback to ~/.nex/config.json when the old file already exists.
func ConfigPath() string {
	// Env override for test harnesses that need to isolate config state from
	// the user's real ~/.wuphf/config.json without remapping HOME (which
	// breaks macOS keychain-backed CLI auth).
	if p := strings.TrimSpace(os.Getenv("WUPHF_CONFIG_PATH")); p != "" {
		return p
	}
	home := RuntimeHomeDir()
	if home == "" {
		return filepath.Join(".wuphf", "config.json")
	}
	newPath := filepath.Join(home, ".wuphf", "config.json")
	legacyPath := filepath.Join(home, ".nex", "config.json")
	if _, err := os.Stat(newPath); err == nil {
		return newPath
	}
	if _, err := os.Stat(legacyPath); err == nil {
		return legacyPath
	}
	return newPath
}

// BaseURL returns the resolved base URL.
// Priority: WUPHF_DEV_URL env > NEX_DEV_URL env > config dev_url > production default.
//
// Note: as of the nex-cli migration, BaseURL is only used by the legacy
// developer API client surface (api.Client) which still backs the workflow
// engine's /v1/insights and /v1/context/ask calls. New Nex integrations
// should shell out via the internal/nex package instead.
func BaseURL() string {
	if v := os.Getenv("WUPHF_DEV_URL"); v != "" {
		return v
	}
	if v := os.Getenv("NEX_DEV_URL"); v != "" {
		return v
	}
	if cfg, err := load(ConfigPath()); err == nil && cfg.DevURL != "" {
		return cfg.DevURL
	}
	return "https://app.nex.ai"
}

// APIBase returns the developer API base URL.
func APIBase() string {
	return fmt.Sprintf("%s/api/developers", BaseURL())
}

// Load reads the config file. Returns an empty config if the file is missing or unreadable.
func Load() (Config, error) {
	return load(ConfigPath())
}

func load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return loadConfigWithCloudFallback(path)
		}
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Save writes cfg to the config file, creating parent directories as needed.
func Save(cfg Config) error {
	path := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := writeFileAtomically(path, data, 0o600); err != nil {
		return err
	}
	settings := cloudBackupSettingsFromConfig(cfg)
	if err := persistCloudBackupBootstrap(settings); err != nil {
		return err
	}
	if settings.Enabled() {
		sink, err := backup.Open(nil, settings)
		if err != nil {
			return err
		}
		if sink != nil {
			defer sink.Close()
			if err := sink.Put(nil, settings.ObjectKey("config/config.json"), data, "application/json"); err != nil {
				return err
			}
			if err := sink.Put(nil, settings.ObjectKey("config/config.last-good.json"), data, "application/json"); err != nil {
				return err
			}
			historyName := fmt.Sprintf("%s.json", time.Now().UTC().Format("20060102T150405.000000000Z"))
			if err := sink.Put(nil, settings.ObjectKey(filepath.ToSlash(filepath.Join("config", "history", historyName))), data, "application/json"); err != nil {
				return err
			}
		}
	}
	return nil
}

func loadConfigWithCloudFallback(path string) (Config, error) {
	settings := resolveCloudBackupRecoverySettings()
	if !settings.Enabled() {
		return Config{}, nil
	}
	current, currentErr := backup.ReadBytes(nil, settings, "config/config.json")
	lastGood, lastGoodErr := backup.ReadBytes(nil, settings, "config/config.last-good.json")
	var (
		cfg  Config
		data []byte
	)
	switch {
	case len(current) > 0 && json.Unmarshal(current, &cfg) == nil:
		data = current
	case len(lastGood) > 0 && json.Unmarshal(lastGood, &cfg) == nil:
		data = lastGood
	default:
		if currentErr != nil && !backup.IsNotFound(currentErr) {
			return Config{}, currentErr
		}
		if lastGoodErr != nil && !backup.IsNotFound(lastGoodErr) {
			return Config{}, lastGoodErr
		}
		return Config{}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Config{}, err
	}
	if writeErr := os.WriteFile(path, data, 0o600); writeErr != nil {
		return Config{}, writeErr
	}
	if err := persistCloudBackupBootstrap(cloudBackupSettingsFromConfig(cfg)); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func cloudBackupSettingsFromConfig(cfg Config) backup.Settings {
	return backup.Settings{
		Provider: ResolveCloudBackupProviderFromConfig(cfg),
		Bucket:   ResolveCloudBackupBucketFromConfig(cfg),
		Prefix:   ResolveCloudBackupPrefixFromConfig(cfg),
	}.Normalized()
}

func cloudBackupSettingsFromEnv() backup.Settings {
	return backup.Settings{
		Provider: NormalizeCloudBackupProvider(os.Getenv("WUPHF_CLOUD_BACKUP_PROVIDER")),
		Bucket:   strings.TrimSpace(os.Getenv("WUPHF_CLOUD_BACKUP_BUCKET")),
		Prefix:   strings.Trim(strings.ReplaceAll(strings.TrimSpace(os.Getenv("WUPHF_CLOUD_BACKUP_PREFIX")), "\\", "/"), "/"),
	}.Normalized()
}

func cloudBackupBootstrapPath() string {
	if v := strings.TrimSpace(os.Getenv("WUPHF_CLOUD_BACKUP_BOOTSTRAP_PATH")); v != "" {
		return v
	}
	path := ConfigPath()
	if path == "" {
		home := RuntimeHomeDir()
		if home == "" {
			return filepath.Join(".wuphf", "cloud-backup-bootstrap.json")
		}
		return filepath.Join(home, ".wuphf", "cloud-backup-bootstrap.json")
	}
	return filepath.Join(filepath.Dir(path), "cloud-backup-bootstrap.json")
}

func loadCloudBackupBootstrap() backup.Settings {
	path := cloudBackupBootstrapPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		return backup.Settings{}
	}
	var snapshot cloudBackupBootstrap
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return backup.Settings{}
	}
	return backup.Settings{
		Provider: NormalizeCloudBackupProvider(snapshot.Provider),
		Bucket:   strings.TrimSpace(snapshot.Bucket),
		Prefix:   strings.Trim(strings.ReplaceAll(strings.TrimSpace(snapshot.Prefix), "\\", "/"), "/"),
	}.Normalized()
}

func persistCloudBackupBootstrap(settings backup.Settings) error {
	path := cloudBackupBootstrapPath()
	if !settings.Enabled() {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(cloudBackupBootstrap{
		Provider: settings.Provider,
		Bucket:   settings.Bucket,
		Prefix:   settings.Prefix,
	}, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if err := writeFileAtomically(path, payload, 0o600); err != nil {
		return err
	}
	return nil
}

func resolveCloudBackupRecoverySettings() backup.Settings {
	envSettings := cloudBackupSettingsFromEnv()
	if envSettings.Enabled() {
		return envSettings
	}
	return loadCloudBackupBootstrap()
}

// ResolveNoNex reports whether Nex-backed tools are disabled for this run.
func ResolveNoNex() bool {
	v := strings.TrimSpace(os.Getenv("WUPHF_NO_NEX"))
	if v == "" {
		return false
	}
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

// NormalizeMemoryBackend returns a supported memory backend or the empty string.
func NormalizeMemoryBackend(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case MemoryBackendNone:
		return MemoryBackendNone
	default:
		return ""
	}
}

func NormalizeCloudBackupProvider(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "":
		return ""
	case backup.ProviderNone:
		return backup.ProviderNone
	case backup.ProviderGCS:
		return backup.ProviderGCS
	default:
		return ""
	}
}

// ResolveMemoryBackend resolves the active organizational memory backend.
// Resolution: flag/env override > config file > default.
//
// Defaults:
//   - `none` for the local-only baseline
//   - legacy selections are normalized back to `none`
func ResolveMemoryBackend(flagValue string) string {
	backend := NormalizeMemoryBackend(flagValue)
	if backend == "" {
		backend = NormalizeMemoryBackend(os.Getenv("WUPHF_MEMORY_BACKEND"))
	}
	if backend == "" {
		cfg, _ := Load()
		backend = NormalizeMemoryBackend(cfg.MemoryBackend)
	}
	if backend == "" {
		return MemoryBackendNone
	}
	return backend
}

// MemoryBackendLabel returns a short user-facing label for the backend.
func MemoryBackendLabel(backend string) string {
	return "Local-only"
}

func ResolveCloudBackupProvider() string {
	if v := NormalizeCloudBackupProvider(os.Getenv("WUPHF_CLOUD_BACKUP_PROVIDER")); v != "" {
		return v
	}
	cfg, _ := Load()
	if v := ResolveCloudBackupProviderFromConfig(cfg); v != "" && v != backup.ProviderNone {
		return v
	}
	if bootstrap := loadCloudBackupBootstrap(); bootstrap.Provider != "" {
		return bootstrap.Provider
	}
	return backup.ProviderNone
}

func ResolveCloudBackupProviderFromConfig(cfg Config) string {
	if v := NormalizeCloudBackupProvider(cfg.CloudBackupProvider); v != "" {
		return v
	}
	return backup.ProviderNone
}

func ResolveCloudBackupBucket() string {
	if v := strings.TrimSpace(os.Getenv("WUPHF_CLOUD_BACKUP_BUCKET")); v != "" {
		return v
	}
	cfg, _ := Load()
	if v := ResolveCloudBackupBucketFromConfig(cfg); v != "" {
		return v
	}
	if bootstrap := loadCloudBackupBootstrap(); bootstrap.Bucket != "" {
		return bootstrap.Bucket
	}
	return ""
}

func ResolveCloudBackupBucketFromConfig(cfg Config) string {
	return strings.TrimSpace(cfg.CloudBackupBucket)
}

func ResolveCloudBackupPrefix() string {
	if v := strings.TrimSpace(os.Getenv("WUPHF_CLOUD_BACKUP_PREFIX")); v != "" {
		return v
	}
	cfg, _ := Load()
	if v := ResolveCloudBackupPrefixFromConfig(cfg); v != "" {
		return v
	}
	if bootstrap := loadCloudBackupBootstrap(); bootstrap.Prefix != "" {
		return bootstrap.Prefix
	}
	return ""
}

func ResolveCloudBackupPrefixFromConfig(cfg Config) string {
	return strings.Trim(strings.ReplaceAll(strings.TrimSpace(cfg.CloudBackupPrefix), "\\", "/"), "/")
}

func ResolveCloudBackupSettings() backup.Settings {
	return backup.Settings{
		Provider: ResolveCloudBackupProvider(),
		Bucket:   ResolveCloudBackupBucket(),
		Prefix:   ResolveCloudBackupPrefix(),
	}.Normalized()
}

// ResolveLLMProvider resolves the active LLM provider for this run.
// Resolution: flag/env override > config file > default claude-code.
// Only supported interactive providers are returned.
func ResolveLLMProvider(flagValue string) string {
	if v := normalizeLLMProvider(flagValue); v != "" {
		return v
	}
	if v := normalizeLLMProvider(os.Getenv("WUPHF_LLM_PROVIDER")); v != "" {
		return v
	}
	cfg, _ := Load()
	if v := normalizeLLMProvider(cfg.LLMProvider); v != "" {
		return v
	}
	return "claude-code"
}

func normalizeLLMProvider(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "claude-code":
		return "claude-code"
	case "codex":
		return "codex"
	case "gemini":
		return "gemini"
	case "gemini-vertex":
		return "gemini-vertex"
	case "ollama":
		return "ollama"
	default:
		return ""
	}
}

// ResolveOllamaBaseURL returns the base URL for the native Ollama API.
// Resolution: WUPHF_OLLAMA_BASE_URL > OLLAMA_HOST > default http://localhost:11434.
func ResolveOllamaBaseURL() string {
	for _, raw := range []string{
		strings.TrimSpace(os.Getenv("WUPHF_OLLAMA_BASE_URL")),
		strings.TrimSpace(os.Getenv("OLLAMA_HOST")),
	} {
		if normalized := normalizeOllamaBaseURL(raw); normalized != "" {
			return normalized
		}
	}
	return "http://localhost:11434"
}

func normalizeOllamaBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "http"
	}
	if parsed.Host == "" {
		return ""
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

var codexModelLinePattern = regexp.MustCompile(`(?m)^\s*model\s*=\s*("([^"\\]|\\.)*"|'[^']*')`)

// ResolveCodexModel returns the effective Codex model for the current working
// directory, following the documented Codex config layering:
// WUPHF_CODEX_MODEL/CODEX_MODEL env > nearest .codex/config.toml > ~/.codex/config.toml.
func ResolveCodexModel(cwd string) string {
	if v := strings.TrimSpace(os.Getenv("WUPHF_CODEX_MODEL")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("CODEX_MODEL")); v != "" {
		return v
	}
	for _, path := range codexConfigSearchPaths(cwd) {
		if model := codexModelFromFile(path); model != "" {
			return model
		}
	}
	return ""
}

func codexConfigSearchPaths(cwd string) []string {
	seen := map[string]struct{}{}
	paths := make([]string, 0, 8)
	add := func(path string) {
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}

	if absCwd, err := filepath.Abs(strings.TrimSpace(cwd)); err == nil && absCwd != "" {
		for dir := absCwd; ; dir = filepath.Dir(dir) {
			add(filepath.Join(dir, ".codex", "config.toml"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
		}
	}

	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, ".codex", "config.toml"))
	}
	return paths
}

func codexModelFromFile(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	match := codexModelLinePattern.FindSubmatch(raw)
	if len(match) < 2 {
		return ""
	}
	value := strings.TrimSpace(string(match[1]))
	if len(value) >= 2 {
		if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
			if unquoted, err := strconv.Unquote(value); err == nil {
				return strings.TrimSpace(unquoted)
			}
		}
		if strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`) {
			return strings.TrimSpace(value[1 : len(value)-1])
		}
	}
	return strings.TrimSpace(value)
}

// ResolveAPIKey resolves the API key via: flag > WUPHF_API_KEY env > NEX_API_KEY env > config file.
func ResolveAPIKey(flagValue string) string {
	if ResolveNoNex() {
		return ""
	}
	if flagValue != "" {
		return flagValue
	}
	if v := os.Getenv("WUPHF_API_KEY"); v != "" {
		return v
	}
	if v := os.Getenv("NEX_API_KEY"); v != "" {
		return v
	}
	cfg, _ := Load()
	return cfg.APIKey
}

// ResolveOneSecret resolves the Nex-managed One secret.
// One is disabled entirely when Nex is disabled for the session.
// Resolution: WUPHF_ONE_SECRET env > ONE_SECRET env > config file.
func ResolveOneSecret() string {
	if ResolveNoNex() {
		return ""
	}
	if v := strings.TrimSpace(os.Getenv("WUPHF_ONE_SECRET")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("ONE_SECRET")); v != "" {
		return v
	}
	cfg, _ := Load()
	return strings.TrimSpace(cfg.OneAPIKey)
}

// ResolveOneIdentity resolves the identity scope WUPHF should use with One.
// Resolution: WUPHF_ONE_IDENTITY env > ONE_IDENTITY env > config email.
func ResolveOneIdentity() string {
	if ResolveNoNex() {
		return ""
	}
	if v := strings.TrimSpace(os.Getenv("WUPHF_ONE_IDENTITY")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("ONE_IDENTITY")); v != "" {
		return v
	}
	cfg, _ := Load()
	return strings.TrimSpace(cfg.Email)
}

// ResolveOneIdentityType resolves the One identity type.
// Resolution: WUPHF_ONE_IDENTITY_TYPE env > ONE_IDENTITY_TYPE env > "user".
func ResolveOneIdentityType() string {
	if ResolveNoNex() {
		return ""
	}
	if v := strings.TrimSpace(os.Getenv("WUPHF_ONE_IDENTITY_TYPE")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("ONE_IDENTITY_TYPE")); v != "" {
		return v
	}
	if ResolveOneIdentity() == "" {
		return ""
	}
	return "user"
}

// OneSetupSummary explains how integrations are handled for the current setup.
func OneSetupSummary() string {
	if ResolveNoNex() {
		return "disabled with Nex (--no-nex)"
	}
	email := ResolveOneIdentity()
	secret := ResolveOneSecret()
	switch {
	case email != "" && secret != "":
		return fmt.Sprintf("managed by Nex via One (%s)", email)
	case email != "":
		return fmt.Sprintf("managed by Nex via One (%s), provisioning pending", email)
	case secret != "":
		return "managed by Nex via One"
	default:
		return "managed by Nex via One after Nex setup"
	}
}

// OneSetupBlurb is the user-facing copy for setup and config surfaces.
func OneSetupBlurb() string {
	if ResolveNoNex() {
		return "Nex is disabled for this session, so managed integrations are disabled too."
	}
	email := ResolveOneIdentity()
	if email != "" {
		return fmt.Sprintf("Managed integrations run through One and are provisioned automatically from your Nex email (%s).", email)
	}
	return "Managed integrations run through One and will be provisioned automatically once Nex setup is complete."
}

// ResolveComposioAPIKey resolves the Composio API key.
// Resolution: WUPHF_COMPOSIO_API_KEY env > COMPOSIO_API_KEY env > config file.
func ResolveComposioAPIKey() string {
	if ResolveNoNex() {
		return ""
	}
	if v := strings.TrimSpace(os.Getenv("WUPHF_COMPOSIO_API_KEY")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("COMPOSIO_API_KEY")); v != "" {
		return v
	}
	cfg, _ := Load()
	return strings.TrimSpace(cfg.ComposioAPIKey)
}

// ResolveTelegramBotToken returns the stored Telegram bot token from config.
func ResolveTelegramBotToken() string {
	if v := strings.TrimSpace(os.Getenv("WUPHF_TELEGRAM_BOT_TOKEN")); v != "" {
		return v
	}
	cfg, _ := Load()
	return strings.TrimSpace(cfg.TelegramBotToken)
}

// SaveTelegramBotToken persists the bot token to config.json.
func SaveTelegramBotToken(token string) {
	cfg, _ := Load()
	cfg.TelegramBotToken = strings.TrimSpace(token)
	_ = Save(cfg)
}

// CompanyContextBlock returns a prompt fragment with company context for agent
// system prompts. Returns empty string if no company name is configured.
func CompanyContextBlock() string {
	cfg, _ := Load()
	name := strings.TrimSpace(cfg.CompanyName)
	if name == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("== COMPANY CONTEXT ==\n")
	sb.WriteString(fmt.Sprintf("Company: %s\n", name))
	if desc := strings.TrimSpace(cfg.CompanyDescription); desc != "" {
		sb.WriteString(fmt.Sprintf("What they do: %s\n", desc))
	}
	if goals := strings.TrimSpace(cfg.CompanyGoals); goals != "" {
		sb.WriteString(fmt.Sprintf("Current goals: %s\n", goals))
	}
	if priority := strings.TrimSpace(cfg.CompanyPriority); priority != "" {
		sb.WriteString(fmt.Sprintf("Immediate priority: %s\n", priority))
	}
	sb.WriteString("\n")
	return sb.String()
}

// ResolveGeminiAPIKey resolves the Gemini API key.
// Resolution: WUPHF_GEMINI_API_KEY env > GEMINI_API_KEY env > config file.
func ResolveGeminiAPIKey() string {
	if v := strings.TrimSpace(os.Getenv("WUPHF_GEMINI_API_KEY")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("GEMINI_API_KEY")); v != "" {
		return v
	}
	cfg, _ := Load()
	return strings.TrimSpace(cfg.GeminiAPIKey)
}

// ResolveAnthropicAPIKey resolves the Anthropic API key.
// Resolution: WUPHF_ANTHROPIC_API_KEY env > ANTHROPIC_API_KEY env > config file.
func ResolveAnthropicAPIKey() string {
	if v := strings.TrimSpace(os.Getenv("WUPHF_ANTHROPIC_API_KEY")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")); v != "" {
		return v
	}
	cfg, _ := Load()
	return strings.TrimSpace(cfg.AnthropicAPIKey)
}

// ResolveOpenAIAPIKey resolves the OpenAI API key.
// Resolution: WUPHF_OPENAI_API_KEY env > OPENAI_API_KEY env > config file.
func ResolveOpenAIAPIKey() string {
	if v := strings.TrimSpace(os.Getenv("WUPHF_OPENAI_API_KEY")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); v != "" {
		return v
	}
	cfg, _ := Load()
	return strings.TrimSpace(cfg.OpenAIAPIKey)
}

// ResolveMinimaxAPIKey resolves the Minimax API key.
// Resolution: WUPHF_MINIMAX_API_KEY env > MINIMAX_API_KEY env > config file.
func ResolveMinimaxAPIKey() string {
	if v := strings.TrimSpace(os.Getenv("WUPHF_MINIMAX_API_KEY")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("MINIMAX_API_KEY")); v != "" {
		return v
	}
	cfg, _ := Load()
	return strings.TrimSpace(cfg.MinimaxAPIKey)
}

// ResolveBraveAPIKey resolves the Brave Search API key.
// Resolution: WUPHF_BRAVE_API_KEY env > BRAVE_API_KEY env > config file.
func ResolveBraveAPIKey() string {
	if v := strings.TrimSpace(os.Getenv("WUPHF_BRAVE_API_KEY")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("BRAVE_API_KEY")); v != "" {
		return v
	}
	cfg, _ := Load()
	return strings.TrimSpace(cfg.BraveAPIKey)
}

// ResolveComposioUserID resolves the Composio user identity WUPHF should use.
// Resolution: WUPHF_COMPOSIO_USER_ID env > COMPOSIO_USER_ID env > config email.
func ResolveComposioUserID() string {
	if ResolveNoNex() {
		return ""
	}
	if v := strings.TrimSpace(os.Getenv("WUPHF_COMPOSIO_USER_ID")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("COMPOSIO_USER_ID")); v != "" {
		return v
	}
	cfg, _ := Load()
	return strings.TrimSpace(cfg.Email)
}

// ResolveActionProvider resolves the preferred external action provider.
// Resolution: WUPHF_ACTION_PROVIDER env > ACTION_PROVIDER env > config file > auto.
func ResolveActionProvider() string {
	if v := NormalizeActionProvider(os.Getenv("WUPHF_ACTION_PROVIDER")); v != "" {
		return v
	}
	if v := NormalizeActionProvider(os.Getenv("ACTION_PROVIDER")); v != "" {
		return v
	}
	cfg, _ := Load()
	if v := NormalizeActionProvider(cfg.ActionProvider); v != "" {
		return v
	}
	return "auto"
}

// ResolveWebSearchProvider resolves the preferred web search provider.
// Resolution: WUPHF_WEB_SEARCH_PROVIDER env > WEB_SEARCH_PROVIDER env > config file > none.
func ResolveWebSearchProvider() string {
	if v := NormalizeWebSearchProvider(os.Getenv("WUPHF_WEB_SEARCH_PROVIDER")); v != "" {
		return v
	}
	if v := NormalizeWebSearchProvider(os.Getenv("WEB_SEARCH_PROVIDER")); v != "" {
		return v
	}
	cfg, _ := Load()
	if v := NormalizeWebSearchProvider(cfg.WebSearchProvider); v != "" {
		return v
	}
	return "none"
}

// ResolveCustomMCPConfigPath resolves the optional path to a supplemental
// MCP settings JSON file.
// Resolution: WUPHF_CUSTOM_MCP_CONFIG_PATH env > config file > empty.
func ResolveCustomMCPConfigPath() string {
	if v := strings.TrimSpace(os.Getenv("WUPHF_CUSTOM_MCP_CONFIG_PATH")); v != "" {
		return v
	}
	cfg, _ := Load()
	return strings.TrimSpace(cfg.CustomMCPConfig)
}

// ResolveFormat resolves the output format via: flag > config file > "text".
func ResolveFormat(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	cfg, _ := Load()
	if cfg.DefaultFormat != "" {
		return cfg.DefaultFormat
	}
	return "text"
}

// ResolveTimeout resolves the timeout (ms) via: flag > config file > 120000.
func ResolveTimeout(flagValue string) int {
	if flagValue != "" {
		if n, err := strconv.Atoi(flagValue); err == nil {
			return n
		}
	}
	cfg, _ := Load()
	if cfg.DefaultTimeout > 0 {
		return cfg.DefaultTimeout
	}
	return 120_000
}

// PersistRegistration merges registration data into the config file.
func PersistRegistration(data map[string]interface{}) error {
	cfg, _ := Load()
	if v, ok := data["api_key"].(string); ok && v != "" {
		cfg.APIKey = v
	}
	if v, ok := data["email"].(string); ok && v != "" {
		cfg.Email = v
	}
	if v, ok := data["workspace_id"].(string); ok && v != "" {
		cfg.WorkspaceID = v
	} else if v, ok := data["workspace_id"].(float64); ok {
		cfg.WorkspaceID = strconv.FormatFloat(v, 'f', -1, 64)
	}
	if v, ok := data["workspace_slug"].(string); ok && v != "" {
		cfg.WorkspaceSlug = v
	}
	return Save(cfg)
}

func ResolveInsightsPollInterval() int {
	minutes := 15
	if raw := os.Getenv("WUPHF_INSIGHTS_INTERVAL_MINUTES"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			minutes = n
		}
	} else if raw := os.Getenv("NEX_INSIGHTS_INTERVAL_MINUTES"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			minutes = n
		}
	} else if cfg, err := Load(); err == nil && cfg.InsightsPollMinutes > 0 {
		minutes = cfg.InsightsPollMinutes
	}
	if minutes < 2 {
		minutes = 2
	}
	return minutes
}

func ResolveTaskFollowUpInterval() int {
	return resolveTaskInterval(
		"WUPHF_TASK_FOLLOWUP_MINUTES",
		"NEX_TASK_FOLLOWUP_MINUTES",
		func(cfg Config) int { return cfg.TaskFollowUpMinutes },
		60,
	)
}

func ResolveTaskReminderInterval() int {
	return resolveTaskInterval(
		"WUPHF_TASK_REMINDER_MINUTES",
		"NEX_TASK_REMINDER_MINUTES",
		func(cfg Config) int { return cfg.TaskReminderMinutes },
		30,
	)
}

func ResolveTaskRecheckInterval() int {
	return resolveTaskInterval(
		"WUPHF_TASK_RECHECK_MINUTES",
		"NEX_TASK_RECHECK_MINUTES",
		func(cfg Config) int { return cfg.TaskRecheckMinutes },
		15,
	)
}

func resolveTaskInterval(envKey, legacyEnvKey string, fromConfig func(Config) int, defaultMinutes int) int {
	minutes := defaultMinutes
	if raw := os.Getenv(envKey); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			minutes = n
		}
	} else if raw := os.Getenv(legacyEnvKey); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			minutes = n
		}
	} else if cfg, err := Load(); err == nil && fromConfig(cfg) > 0 {
		minutes = fromConfig(cfg)
	}
	if minutes < 2 {
		minutes = 2
	}
	return minutes
}
