package provider

import "fmt"

// Kind values for ProviderBinding.Kind. The empty string means "fall back to
// the install-wide default" (config.ResolveLLMProvider at dispatch time), which
// keeps manifests written before per-agent providers existed loading unchanged.
const (
	KindClaudeCode   = "claude-code"
	KindCodex        = "codex"
	KindGemini       = "gemini"
	KindGeminiVertex = "gemini-vertex"
	KindOllama       = "ollama"
	KindOpenclaude   = "openclaude"
)

// ProviderBinding is the per-agent runtime selection persisted on an office
// member and on company.MemberSpec. It captures which LLM runtime executes the
// agent's turns (Kind) and which model the runtime should use (Model, free
// form — validated by the runtime itself, not here).
type ProviderBinding struct {
	Kind  string `json:"kind,omitempty"`
	Model string `json:"model,omitempty"`
}

// ValidateKind reports whether s is an acceptable ProviderBinding.Kind value.
// The empty string is valid and means "use install-wide default."
func ValidateKind(s string) error {
	switch s {
	case "", KindClaudeCode, KindCodex, KindGemini, KindGeminiVertex, KindOllama, KindOpenclaude:
		return nil
	default:
		return fmt.Errorf(
			"unknown provider kind %q (valid: %s, %s, %s, %s, %s, %s, or empty)",
			s,
			KindClaudeCode,
			KindCodex,
			KindGemini,
			KindGeminiVertex,
			KindOllama,
			KindOpenclaude,
		)
	}
}

// ResolveKind returns the effective runtime kind for a binding. If the
// binding's Kind is empty, it falls back to global() — the caller provides
// this function so this package stays decoupled from config loading.
func ResolveKind(b ProviderBinding, global func() string) string {
	if b.Kind != "" {
		return b.Kind
	}
	if global == nil {
		return ""
	}
	return global()
}
