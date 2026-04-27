package team

import (
	"fmt"
	"strings"

	"github.com/nex-crm/wuphf/internal/provider"
)

func normalizeTaskRuntimeOverrides(task *teamTask) error {
	if task == nil {
		return nil
	}
	providerKind, err := normalizeTaskRuntimeProvider(task.RuntimeProvider)
	if err != nil {
		return err
	}
	effort, err := normalizeReasoningEffort(task.ReasoningEffort)
	if err != nil {
		return err
	}
	task.RuntimeProvider = providerKind
	task.RuntimeModel = strings.TrimSpace(task.RuntimeModel)
	task.ReasoningEffort = effort
	return nil
}

func normalizeTaskRuntimeProvider(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	kind := normalizeProviderKind(raw)
	if err := provider.ValidateKind(kind); err != nil {
		return "", err
	}
	return kind, nil
}

func normalizeReasoningEffort(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "default":
		return "", nil
	case "low", "medium", "high", "xhigh":
		return strings.ToLower(strings.TrimSpace(raw)), nil
	default:
		return "", fmt.Errorf("invalid reasoning_effort %q (valid: default, low, medium, high, xhigh)", raw)
	}
}

func inferRuntimeProviderFromModel(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(model, "gpt-"):
		return provider.KindCodex
	case strings.HasPrefix(model, "o1"), strings.HasPrefix(model, "o3"), strings.HasPrefix(model, "o4"):
		return provider.KindCodex
	case strings.HasPrefix(model, "claude-"):
		return provider.KindClaudeCode
	case strings.HasPrefix(model, "gemini-"):
		return provider.KindGemini
	default:
		return ""
	}
}
