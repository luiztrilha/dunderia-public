package team

import (
	"testing"

	"github.com/nex-crm/wuphf/internal/provider"
)

func TestNormalizeProviderKind(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want string
	}{
		{"", provider.KindClaudeCode},
		{"claude", provider.KindClaudeCode},
		{"Claude", provider.KindClaudeCode},
		{" codex ", provider.KindCodex},
		{"CODEX", provider.KindCodex},
		{"claude-code", provider.KindClaudeCode},
		{"gemini", provider.KindGemini},
		{"gemini-vertex", provider.KindGeminiVertex},
		{"ollama", provider.KindOllama},
		{"openclaude", provider.KindOpenclaude},
		{"custom-runtime", "custom-runtime"},
	}
	for _, tt := range tests {
		if got := normalizeProviderKind(tt.in); got != tt.want {
			t.Errorf("normalizeProviderKind(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMemberEffectiveProviderKind_PerAgentWins(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	b.members = append(b.members, officeMember{
		Slug:     "pm-codex",
		Name:     "PM Codex",
		Provider: provider.ProviderBinding{Kind: provider.KindCodex},
	})
	b.memberIndex = nil
	b.mu.Unlock()

	l := &Launcher{broker: b, provider: "claude-code"}
	if got := l.memberEffectiveProviderKind("pm-codex"); got != provider.KindCodex {
		t.Fatalf("per-agent should win over global, got %q", got)
	}
}

func TestMemberEffectiveProviderKind_PerAgentGeminiWins(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	b.members = append(b.members, officeMember{
		Slug:     "research-gemini",
		Name:     "Research Gemini",
		Provider: provider.ProviderBinding{Kind: provider.KindGemini},
	})
	b.memberIndex = nil
	b.mu.Unlock()

	l := &Launcher{broker: b, provider: provider.KindClaudeCode}
	if got := l.memberEffectiveProviderKind("research-gemini"); got != provider.KindGemini {
		t.Fatalf("per-agent gemini should win over global fallback, got %q", got)
	}
}

func TestMemberEffectiveProviderKind_PerAgentGeminiVertexWins(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	b.members = append(b.members, officeMember{
		Slug:     "research-vertex",
		Name:     "Research Vertex",
		Provider: provider.ProviderBinding{Kind: provider.KindGeminiVertex},
	})
	b.memberIndex = nil
	b.mu.Unlock()

	l := &Launcher{broker: b, provider: provider.KindCodex}
	if got := l.memberEffectiveProviderKind("research-vertex"); got != provider.KindGeminiVertex {
		t.Fatalf("per-agent gemini-vertex should win over global fallback, got %q", got)
	}
}

func TestMemberEffectiveProviderKind_PerAgentOllamaWins(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	b.members = append(b.members, officeMember{
		Slug:     "coder-ollama",
		Name:     "Coder Ollama",
		Provider: provider.ProviderBinding{Kind: provider.KindOllama, Model: provider.OllamaDefaultModel},
	})
	b.memberIndex = nil
	b.mu.Unlock()

	l := &Launcher{broker: b, provider: provider.KindCodex}
	if got := l.memberEffectiveProviderKind("coder-ollama"); got != provider.KindOllama {
		t.Fatalf("per-agent ollama should win over global fallback, got %q", got)
	}
}

func TestMemberEffectiveProviderKind_PerAgentOpenclaudeWins(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	b.members = append(b.members, officeMember{
		Slug:     "writer-openclaude",
		Name:     "Writer OpenClaude",
		Provider: provider.ProviderBinding{Kind: provider.KindOpenclaude},
	})
	b.memberIndex = nil
	b.mu.Unlock()

	l := &Launcher{broker: b, provider: provider.KindCodex}
	if got := l.memberEffectiveProviderKind("writer-openclaude"); got != provider.KindOpenclaude {
		t.Fatalf("per-agent openclaude should win over global fallback, got %q", got)
	}
}

func TestMemberEffectiveProviderKind_FallsBackToGlobal(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	b.members = append(b.members, officeMember{
		Slug: "no-binding",
		Name: "No Binding",
	})
	b.memberIndex = nil
	b.mu.Unlock()

	l := &Launcher{broker: b, provider: provider.KindCodex}
	if got := l.memberEffectiveProviderKind("no-binding"); got != provider.KindCodex {
		t.Fatalf("empty Kind should fall back to global=codex, got %q", got)
	}
	if got := l.memberEffectiveProviderKind("nobody"); got != provider.KindCodex {
		t.Fatalf("unknown slug should fall back to global=codex, got %q", got)
	}
}

func TestMemberEffectiveProviderKind_DefaultsToClaudeWhenAllEmpty(t *testing.T) {
	b := NewBroker()
	l := &Launcher{broker: b, provider: ""}
	if got := l.memberEffectiveProviderKind("anybody"); got != provider.KindClaudeCode {
		t.Fatalf("default fallback should be claude-code, got %q", got)
	}
}

func TestBrokerMemberProviderKind_Lookup(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	b.members = append(b.members, officeMember{
		Slug:     "research-vertex",
		Name:     "Research Vertex",
		Provider: provider.ProviderBinding{Kind: provider.KindGeminiVertex, Model: provider.GeminiVertexDefaultModel},
	})
	b.memberIndex = nil
	b.mu.Unlock()

	if got := b.MemberProviderKind("research-vertex"); got != provider.KindGeminiVertex {
		t.Fatalf("MemberProviderKind = %q, want %q", got, provider.KindGeminiVertex)
	}
	if binding := b.MemberProviderBinding("research-vertex"); binding.Model != provider.GeminiVertexDefaultModel {
		t.Fatalf("MemberProviderBinding lost model: %+v", binding)
	}
	if got := b.MemberProviderKind("missing"); got != "" {
		t.Fatalf("unknown slug should return empty, got %q", got)
	}
}

func TestIsHeadlessRuntimeProviderFailureRecognizesHitYourLimit(t *testing.T) {
	if !isHeadlessRuntimeProviderFailure("You've hit your limit · resets 12:30am (America/Sao_Paulo)") {
		t.Fatal("expected hit-your-limit provider failure to be recognized")
	}
}
