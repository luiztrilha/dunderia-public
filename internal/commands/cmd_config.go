package commands

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/nex-crm/wuphf/internal/config"
)

func cmdConfig(ctx *SlashContext, args string) error {
	positional, _ := parseFlags(args)

	sub := "show"
	if len(positional) > 0 {
		sub = positional[0]
	}

	switch sub {
	case "show":
		return configShow(ctx)
	case "set":
		if len(positional) < 3 {
			ctx.AddMessage("system", "Usage: /config set <key> <value>")
			return nil
		}
		value := strings.TrimSpace(strings.Join(positional[2:], " "))
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		return configSet(ctx, positional[1], value)
	case "path":
		ctx.AddMessage("system", config.ConfigPath())
		return nil
	default:
		ctx.AddMessage("system", "Unknown subcommand: "+sub+"\nUsage: /config [show|set|path]")
		return nil
	}
}

func configShow(ctx *SlashContext) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	masked := maskKey(cfg.APIKey)
	workspace := cfg.WorkspaceSlug
	if workspace == "" {
		workspace = cfg.WorkspaceID
	}
	if workspace == "" {
		workspace = "(not set)"
	}

	provider := cfg.LLMProvider
	if provider == "" {
		provider = "(not set)"
	}
	memoryBackend := cfg.MemoryBackend
	if memoryBackend == "" {
		memoryBackend = config.ResolveMemoryBackend("")
	}
	if memoryBackend == "" {
		memoryBackend = "(not set)"
	}

	blueprint := cfg.ActiveBlueprint()
	if blueprint == "" {
		blueprint = "(not set)"
	}

	baseURL := config.BaseURL()
	actionProvider := config.ResolveActionProvider()
	if actionProvider == "" {
		actionProvider = "auto"
	}
	webSearchProvider := config.ResolveWebSearchProvider()
	if webSearchProvider == "" {
		webSearchProvider = "none"
	}

	var sb strings.Builder
	sb.WriteString("Configuration:\n")
	sb.WriteString(fmt.Sprintf("  API Key:   %s\n", masked))
	sb.WriteString(fmt.Sprintf("  Integrations: %s\n", config.OneSetupSummary()))
	sb.WriteString(fmt.Sprintf("  Action provider: %s\n", actionProvider))
	sb.WriteString(fmt.Sprintf("  Web search: %s\n", webSearchProvider))
	sb.WriteString(fmt.Sprintf("  Workspace: %s\n", workspace))
	sb.WriteString(fmt.Sprintf("  Memory:    %s\n", memoryBackend))
	sb.WriteString(fmt.Sprintf("  Provider:  %s\n", provider))
	sb.WriteString(fmt.Sprintf("  Gemini:    %s\n", maskKey(cfg.GeminiAPIKey)))
	sb.WriteString(fmt.Sprintf("  Anthropic: %s\n", maskKey(cfg.AnthropicAPIKey)))
	sb.WriteString(fmt.Sprintf("  OpenAI:    %s\n", maskKey(cfg.OpenAIAPIKey)))
	sb.WriteString(fmt.Sprintf("  Minimax:   %s\n", maskKey(cfg.MinimaxAPIKey)))
	sb.WriteString(fmt.Sprintf("  Brave:     %s\n", maskKey(cfg.BraveAPIKey)))
	sb.WriteString(fmt.Sprintf("  Blueprint: %s\n", blueprint))
	if legacy := strings.TrimSpace(cfg.Pack); legacy != "" && legacy != blueprint {
		sb.WriteString(fmt.Sprintf("  Legacy pack: %s\n", legacy))
	}
	sb.WriteString(fmt.Sprintf("  Base URL:  %s", baseURL))
	ctx.AddMessage("system", sb.String())
	return nil
}

func maskKey(key string) string {
	if key == "" {
		return "(not set)"
	}
	if len(key) > 8 {
		return key[:4] + "…" + key[len(key)-4:]
	}
	return "****"
}

func configSet(ctx *SlashContext, key, value string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	switch key {
	case "api_key":
		cfg.APIKey = value
	case "composio_api_key":
		cfg.ComposioAPIKey = value
	case "action_provider":
		ap := config.NormalizeActionProvider(value)
		if strings.TrimSpace(value) == "" {
			cfg.ActionProvider = ""
			break
		}
		if ap == "" {
			ctx.AddMessage("system", "Invalid value for action_provider. Valid: auto, one, composio")
			return nil
		}
		cfg.ActionProvider = ap
	case "web_search_provider":
		provider := config.NormalizeWebSearchProvider(value)
		if strings.TrimSpace(value) == "" {
			cfg.WebSearchProvider = ""
			break
		}
		if provider == "" {
			ctx.AddMessage("system", "Invalid value for web_search_provider. Valid: none, brave")
			return nil
		}
		cfg.WebSearchProvider = provider
	case "workspace_id":
		cfg.WorkspaceID = value
	case "workspace_slug":
		cfg.WorkspaceSlug = value
	case "memory_backend":
		normalized := config.NormalizeMemoryBackend(value)
		if normalized == "" {
			ctx.AddMessage("system", "Unsupported memory backend. Valid value: none")
			return nil
		}
		cfg.MemoryBackend = normalized
	case "cloud_backup_provider":
		normalized := config.NormalizeCloudBackupProvider(value)
		if normalized == "" {
			ctx.AddMessage("system", "Unsupported cloud backup provider. Valid values: gcs, none")
			return nil
		}
		cfg.CloudBackupProvider = normalized
	case "cloud_backup_bucket":
		cfg.CloudBackupBucket = value
	case "cloud_backup_prefix":
		cfg.CloudBackupPrefix = strings.Trim(strings.ReplaceAll(value, "\\", "/"), "/")
	case "llm_provider":
		cfg.LLMProvider = value
	case "gemini_api_key":
		cfg.GeminiAPIKey = value
	case "anthropic_api_key":
		cfg.AnthropicAPIKey = value
	case "openai_api_key":
		cfg.OpenAIAPIKey = value
	case "minimax_api_key":
		cfg.MinimaxAPIKey = value
	case "brave_api_key":
		cfg.BraveAPIKey = value
	case "blueprint", "template", "operation_template", "pack":
		cfg.SetActiveBlueprint(value)
	case "team_lead_slug":
		cfg.TeamLeadSlug = value
	case "dev_url":
		cfg.DevURL = value
	case "default_format":
		cfg.DefaultFormat = value
	case "company_name":
		cfg.CompanyName = value
	case "company_description":
		cfg.CompanyDescription = value
	case "company_goals":
		cfg.CompanyGoals = value
	case "company_size":
		cfg.CompanySize = value
	case "company_priority":
		cfg.CompanyPriority = value
	default:
		ctx.AddMessage("system", "Unknown config key: "+key+
			"\nValid keys: api_key, composio_api_key, action_provider, web_search_provider, workspace_id, workspace_slug, memory_backend, cloud_backup_provider, cloud_backup_bucket, cloud_backup_prefix, llm_provider, gemini_api_key, anthropic_api_key, openai_api_key, minimax_api_key, brave_api_key, blueprint, template, operation_template, pack (legacy alias), team_lead_slug, dev_url, default_format, company_name, company_description, company_goals, company_size, company_priority")
		return nil
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	ctx.AddMessage("system", fmt.Sprintf("Set %s = %s", key, value))
	return nil
}

// cmdDetect checks for installed AI platform CLIs.
func cmdDetect(ctx *SlashContext, args string) error {
	platforms := []struct {
		name string
		cmd  string
	}{
		{"Claude", "claude"},
		{"Cursor", "cursor"},
		{"Windsurf", "windsurf"},
		{"VS Code", "code"},
		{"Cline", "cline"},
		{"Aider", "aider"},
	}

	var sb strings.Builder
	sb.WriteString("AI platform detection:\n")
	found := 0
	for _, p := range platforms {
		path, err := exec.LookPath(p.cmd)
		if err == nil {
			sb.WriteString(fmt.Sprintf("  ✓ %s — %s\n", p.name, path))
			found++
		} else {
			sb.WriteString(fmt.Sprintf("  ✗ %s — not found\n", p.name))
		}
	}
	sb.WriteString(fmt.Sprintf("\n%d of %d platforms detected.", found, len(platforms)))
	ctx.AddMessage("system", sb.String())
	return nil
}

// cmdSession handles session management subcommands.
func cmdSession(ctx *SlashContext, args string) error {
	positional, _ := parseFlags(args)

	sub := "list"
	if len(positional) > 0 {
		sub = positional[0]
	}

	switch sub {
	case "list":
		ctx.AddMessage("system", "Session management — coming soon.")
	case "clear":
		ctx.AddMessage("system", "Sessions cleared.")
	default:
		ctx.AddMessage("system", "Unknown subcommand: "+sub+"\nUsage: /session [list|clear]")
	}
	return nil
}
