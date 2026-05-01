package team

import (
	"slices"
	"testing"

	"github.com/nex-crm/wuphf/internal/provider"
)

func TestResolveHeadlessModelRoute_DefaultClaudeBalanced(t *testing.T) {
	l := minimalLauncher(false)

	route := l.resolveHeadlessModelRoute(provider.KindClaudeCode, "ceo", "hello")

	if route.Profile != modelProfileBalanced {
		t.Fatalf("profile = %q, want %q", route.Profile, modelProfileBalanced)
	}
	if route.Model != "claude-sonnet-4-6" {
		t.Fatalf("model = %q, want claude-sonnet-4-6", route.Model)
	}
	if !slices.Contains(route.Reasons, "default_balanced") {
		t.Fatalf("reasons = %v, want default_balanced", route.Reasons)
	}
}

func TestResolveHeadlessModelRoute_OpusCEOLeadPremium(t *testing.T) {
	l := minimalLauncher(true)

	route := l.resolveHeadlessModelRoute(provider.KindClaudeCode, "ceo", "status")

	if route.Profile != modelProfilePremium {
		t.Fatalf("profile = %q, want %q", route.Profile, modelProfilePremium)
	}
	if route.Model != "claude-opus-4-6" {
		t.Fatalf("model = %q, want claude-opus-4-6", route.Model)
	}
	if !slices.Contains(route.Reasons, "opus_ceo_flag") {
		t.Fatalf("reasons = %v, want opus_ceo_flag", route.Reasons)
	}
}

func TestSelectConservativeModelProfile_WorkspaceTaskIsDeep(t *testing.T) {
	l := minimalLauncher(false)

	profile, reasons := l.selectConservativeModelProfile("eng", "implement the fix", &teamTask{
		ExecutionMode: "local_worktree",
	})

	if profile != modelProfileDeep {
		t.Fatalf("profile = %q, want %q", profile, modelProfileDeep)
	}
	if !slices.Contains(reasons, "workspace_execution") {
		t.Fatalf("reasons = %v, want workspace_execution", reasons)
	}
}

func TestExplicitTaskModelPinsWithinProviderFamily(t *testing.T) {
	l := minimalLauncher(false)
	task := &teamTask{RuntimeProvider: provider.KindCodex, RuntimeModel: "gpt-5.5"}

	route := modelRouteDecision{
		Provider: provider.KindCodex,
		Profile:  modelProfilePinned,
		Model:    l.explicitTaskModelForProvider(task, provider.KindCodex),
		Reasons:  []string{"task_runtime_model"},
	}

	if route.Model != "gpt-5.5" {
		t.Fatalf("model = %q, want gpt-5.5", route.Model)
	}
	if route.Profile != modelProfilePinned {
		t.Fatalf("profile = %q, want %q", route.Profile, modelProfilePinned)
	}
	if got := l.explicitTaskModelForProvider(task, provider.KindClaudeCode); got != "" {
		t.Fatalf("claude task model = %q, want empty cross-family result", got)
	}
}

func TestResolveHeadlessModelRoute_UsesPerAgentProviderModel(t *testing.T) {
	l := minimalLauncher(false)
	b := NewBroker()
	b.mu.Lock()
	b.members = append(b.members, officeMember{
		Slug: "eng",
		Name: "Engineer",
		Provider: provider.ProviderBinding{
			Kind:  provider.KindCodex,
			Model: "gpt-5.5",
		},
	})
	b.memberIndex["eng"] = len(b.members) - 1
	b.mu.Unlock()
	l.broker = b

	route := l.resolveHeadlessModelRoute(provider.KindCodex, "eng", "review this")

	if route.Profile != modelProfilePinned {
		t.Fatalf("profile = %q, want %q", route.Profile, modelProfilePinned)
	}
	if route.Model != "gpt-5.5" {
		t.Fatalf("model = %q, want gpt-5.5", route.Model)
	}
	if !slices.Contains(route.Reasons, "agent_provider_model") {
		t.Fatalf("reasons = %v, want agent_provider_model", route.Reasons)
	}
}

func TestResolveHeadlessModelRoute_StaysInsideProviderFamily(t *testing.T) {
	l := minimalLauncher(false)
	b := NewBroker()
	b.mu.Lock()
	b.members = append(b.members, officeMember{
		Slug: "eng",
		Name: "Engineer",
		Provider: provider.ProviderBinding{
			Kind:  provider.KindClaudeCode,
			Model: "claude-haiku-4-5",
		},
	})
	b.memberIndex["eng"] = len(b.members) - 1
	b.mu.Unlock()
	l.broker = b

	route := l.resolveHeadlessModelRoute(provider.KindCodex, "eng", "debug this regression")

	if route.Provider != provider.KindCodex {
		t.Fatalf("provider = %q, want %q", route.Provider, provider.KindCodex)
	}
	if route.Model != "gpt-5.5" {
		t.Fatalf("model = %q, want codex deep model gpt-5.5", route.Model)
	}
	if route.Profile != modelProfileDeep {
		t.Fatalf("profile = %q, want %q", route.Profile, modelProfileDeep)
	}
}

func TestResolveHeadlessModelRoute_RejectsCrossFamilyUnscopedMemberModel(t *testing.T) {
	l := minimalLauncher(false)
	b := NewBroker()
	b.mu.Lock()
	b.members = append(b.members, officeMember{
		Slug: "eng",
		Name: "Engineer",
		Provider: provider.ProviderBinding{
			Model: "gpt-5.5",
		},
	})
	b.memberIndex["eng"] = len(b.members) - 1
	b.mu.Unlock()
	l.broker = b

	route := l.resolveHeadlessModelRoute(provider.KindClaudeCode, "eng", "plain update")

	if route.Model != "claude-sonnet-4-6" {
		t.Fatalf("model = %q, want claude-sonnet-4-6", route.Model)
	}
	if route.Profile == modelProfilePinned {
		t.Fatalf("profile = pinned, want automatic family-native fallback")
	}
}
