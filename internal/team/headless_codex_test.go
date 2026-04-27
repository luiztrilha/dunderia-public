package team

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/nex-crm/wuphf/internal/agent"
	"github.com/nex-crm/wuphf/internal/config"
	"github.com/nex-crm/wuphf/internal/provider"
)

type headlessCodexRecord struct {
	Name   string   `json:"name"`
	Args   []string `json:"args"`
	Dir    string   `json:"dir"`
	Env    []string `json:"env"`
	Stdin  string   `json:"stdin"`
	Script string   `json:"script,omitempty"`
}

type processedTurn struct {
	notification string
	channel      string
}

func TestNewLauncherUsesCodexProviderFromConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WUPHF_BROKER_TOKEN", "")
	if err := config.Save(config.Config{LLMProvider: "codex"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	l, err := NewLauncher("founding-team")
	if err != nil {
		t.Fatalf("NewLauncher: %v", err)
	}
	if l.provider != "codex" {
		t.Fatalf("expected codex provider, got %q", l.provider)
	}
	if l.UsesTmuxRuntime() {
		t.Fatal("expected codex launcher to use headless runtime")
	}
}

func TestNewLauncherAcceptsOperationBlueprintID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WUPHF_BROKER_TOKEN", "")
	if err := config.Save(config.Config{LLMProvider: "codex"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	l, err := NewLauncher("multi-agent-workflow-consulting")
	if err != nil {
		t.Fatalf("NewLauncher: %v", err)
	}
	if got, want := l.packSlug, "multi-agent-workflow-consulting"; got != want {
		t.Fatalf("unexpected launcher blueprint id: got %q want %q", got, want)
	}
	if l.pack != nil {
		t.Fatalf("expected no static pack for operation blueprint launch, got %+v", l.pack)
	}
}

func TestBuildCodexOfficeConfigOverridesIncludesOfficeMCPEnv(t *testing.T) {
	oldExecutablePath := headlessCodexExecutablePath
	oldLookPath := headlessCodexLookPath
	headlessCodexExecutablePath = func() (string, error) { return "/tmp/wuphf", nil }
	headlessCodexLookPath = func(file string) (string, error) {
		return "", exec.ErrNotFound
	}
	defer func() {
		headlessCodexExecutablePath = oldExecutablePath
		headlessCodexLookPath = oldLookPath
	}()

	t.Setenv("WUPHF_NO_NEX", "1")

	broker := NewBroker()
	if err := broker.SetSessionMode(SessionModeOneOnOne, "pm"); err != nil {
		t.Fatalf("SetSessionMode: %v", err)
	}
	l := &Launcher{
		broker:      broker,
		pack:        agent.GetPack("founding-team"),
		sessionMode: SessionModeOneOnOne,
		oneOnOne:    "pm",
	}

	overrides, err := l.buildCodexOfficeConfigOverrides("pm")
	if err != nil {
		t.Fatalf("buildCodexOfficeConfigOverrides: %v", err)
	}
	joined := strings.Join(overrides, "\n")
	if !strings.Contains(joined, `mcp_servers.wuphf-office.command="/tmp/wuphf"`) {
		t.Fatalf("expected WUPHF MCP command override, got %q", joined)
	}
	if !strings.Contains(joined, `mcp_servers.wuphf-office.args=["mcp-team"]`) {
		t.Fatalf("expected WUPHF MCP args override, got %q", joined)
	}
	if !strings.Contains(joined, `mcp_servers.wuphf-office.env_vars=["WUPHF_AGENT_SLUG", "WUPHF_BROKER_TOKEN", "WUPHF_BROKER_BASE_URL", "WUPHF_NO_NEX", "WUPHF_ONE_ON_ONE", "WUPHF_ONE_ON_ONE_AGENT"]`) {
		t.Fatalf("expected office env var forwarding, got %q", joined)
	}
	if strings.Contains(joined, broker.Token()) {
		t.Fatalf("expected broker token value to stay out of args, got %q", joined)
	}
	if strings.Contains(joined, `mcp_servers.nex.command=`) {
		t.Fatalf("expected Nex MCP to stay disabled with WUPHF_NO_NEX, got %q", joined)
	}
}

func TestRunHeadlessCodexTurnUsesHeadlessOfficeRuntime(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	recordFile := filepath.Join(t.TempDir(), "headless-codex-record.jsonl")
	oldLookPath := headlessCodexLookPath
	oldExecutablePath := headlessCodexExecutablePath
	oldCommandContext := headlessCodexCommandContext
	headlessCodexLookPath = func(file string) (string, error) {
		switch file {
		case "codex":
			return "/usr/bin/codex", nil
		case "nex-mcp":
			return "/usr/bin/nex-mcp", nil
		default:
			return "", exec.ErrNotFound
		}
	}
	headlessCodexExecutablePath = func() (string, error) { return "/tmp/wuphf", nil }
	headlessCodexCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmdArgs := []string{"-test.run=TestHeadlessCodexHelperProcess", "--"}
		cmdArgs = append(cmdArgs, args...)
		return exec.CommandContext(ctx, os.Args[0], cmdArgs...)
	}
	defer func() {
		headlessCodexLookPath = oldLookPath
		headlessCodexExecutablePath = oldExecutablePath
		headlessCodexCommandContext = oldCommandContext
	}()

	t.Setenv("GO_WANT_HEADLESS_CODEX_HELPER_PROCESS", "1")
	t.Setenv("HEADLESS_CODEX_RECORD_FILE", recordFile)
	t.Setenv("WUPHF_API_KEY", "nex-secret-key")
	t.Setenv("WUPHF_OPENAI_API_KEY", "openai-secret-key")
	t.Setenv("WUPHF_ONE_SECRET", "one-secret-value")
	t.Setenv("WUPHF_ONE_IDENTITY", "founder@example.com")
	t.Setenv("WUPHF_ONE_IDENTITY_TYPE", "user")

	l := &Launcher{
		pack:        agent.GetPack("founding-team"),
		cwd:         t.TempDir(),
		broker:      NewBroker(),
		headlessCtx: context.Background(),
	}
	if err := l.broker.SetSessionMode(SessionModeOffice, "ceo"); err != nil {
		t.Fatalf("SetSessionMode: %v", err)
	}

	if err := l.runHeadlessCodexTurn(context.Background(), "ceo", "You have new work in #launch."); err != nil {
		t.Fatalf("runHeadlessCodexTurn: %v", err)
	}

	record := readHeadlessCodexRecord(t, recordFile)
	joinedArgs := strings.Join(record.Args, " ")
	if !strings.Contains(joinedArgs, "exec") || !strings.Contains(joinedArgs, "--ephemeral") {
		t.Fatalf("expected codex exec args, got %#v", record.Args)
	}
	if !strings.Contains(joinedArgs, "-a never") || !strings.Contains(joinedArgs, "-s workspace-write") {
		t.Fatalf("expected workspace-write sandbox for office turn, got %#v", record.Args)
	}
	if strings.Contains(joinedArgs, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("did not expect dangerous bypass for office turn, got %#v", record.Args)
	}
	if !strings.Contains(joinedArgs, "--disable plugins") {
		t.Fatalf("expected plugins feature to be disabled, got %#v", record.Args)
	}
	if !strings.Contains(joinedArgs, `mcp_servers.wuphf-office.command="/tmp/wuphf"`) {
		t.Fatalf("expected office MCP override, got %#v", record.Args)
	}
	if !strings.Contains(joinedArgs, `mcp_servers.wuphf-office.env_vars=["WUPHF_AGENT_SLUG", "WUPHF_BROKER_TOKEN", "WUPHF_BROKER_BASE_URL", "ONE_SECRET", "ONE_IDENTITY", "ONE_IDENTITY_TYPE"]`) {
		t.Fatalf("expected office env var forwarding, got %#v", record.Args)
	}
	if !strings.Contains(joinedArgs, `mcp_servers.nex.command="/usr/bin/nex-mcp"`) {
		t.Fatalf("expected nex MCP override, got %#v", record.Args)
	}
	if !strings.Contains(joinedArgs, `mcp_servers.nex.env_vars=["WUPHF_API_KEY", "NEX_API_KEY"]`) {
		t.Fatalf("expected nex env var forwarding, got %#v", record.Args)
	}
	if got := argValue(record.Args, "-C"); !samePath(got, l.cwd) {
		t.Fatalf("expected codex workspace root %q, got %q", l.cwd, got)
	}
	if !samePath(record.Dir, l.cwd) {
		t.Fatalf("expected command dir %q, got %q", l.cwd, record.Dir)
	}
	if !containsEnv(record.Env, "WUPHF_AGENT_SLUG=ceo") {
		t.Fatalf("expected agent env, got %#v", record.Env)
	}
	wantCodexHome := filepath.Join(os.Getenv("HOME"), ".wuphf", "codex-headless")
	if !containsEnv(record.Env, "HOME="+wantCodexHome) {
		t.Fatalf("expected isolated HOME env, got %#v", record.Env)
	}
	if !containsEnv(record.Env, "CODEX_HOME="+wantCodexHome) {
		t.Fatalf("expected absolute CODEX_HOME env, got %#v", record.Env)
	}
	if !containsEnv(record.Env, "WUPHF_HEADLESS_PROVIDER=codex") {
		t.Fatalf("expected headless provider env, got %#v", record.Env)
	}
	if got := envValue(record.Env, "GOCACHE"); !samePath(got, filepath.Join(l.cwd, ".wuphf", "cache", "go-build", "ceo")) {
		t.Fatalf("expected repo-local GOCACHE, got %#v", record.Env)
	}
	if got := envValue(record.Env, "GOTMPDIR"); !samePath(got, filepath.Join(l.cwd, ".wuphf", "cache", "go-tmp", "ceo")) {
		t.Fatalf("expected repo-local GOTMPDIR, got %#v", record.Env)
	}
	if !containsEnvPrefix(record.Env, "WUPHF_BROKER_TOKEN=") {
		t.Fatalf("expected broker token env, got %#v", record.Env)
	}
	if containsEnvPrefix(record.Env, "NEX_AGENT_SLUG=") {
		t.Fatalf("did not expect legacy agent slug env, got %#v", record.Env)
	}
	if containsEnvPrefix(record.Env, "NEX_BROKER_TOKEN=") {
		t.Fatalf("did not expect legacy broker token env, got %#v", record.Env)
	}
	if !containsEnv(record.Env, "WUPHF_API_KEY=nex-secret-key") || !containsEnv(record.Env, "NEX_API_KEY=nex-secret-key") {
		t.Fatalf("expected nex API env, got %#v", record.Env)
	}
	if !containsEnv(record.Env, "WUPHF_OPENAI_API_KEY=openai-secret-key") || !containsEnv(record.Env, "OPENAI_API_KEY=openai-secret-key") {
		t.Fatalf("expected openai API env, got %#v", record.Env)
	}
	if !containsEnv(record.Env, "ONE_SECRET=one-secret-value") {
		t.Fatalf("expected one secret env, got %#v", record.Env)
	}
	if strings.Contains(joinedArgs, l.broker.Token()) || strings.Contains(joinedArgs, "nex-secret-key") || strings.Contains(joinedArgs, "openai-secret-key") || strings.Contains(joinedArgs, "one-secret-value") {
		t.Fatalf("expected secret values to stay out of args, got %#v", record.Args)
	}
	if !strings.Contains(record.Stdin, "<system>") || !strings.Contains(record.Stdin, "You have new work in #launch.") {
		t.Fatalf("expected notification prompt in stdin, got %q", record.Stdin)
	}
	if got := l.broker.usage.Agents["ceo"].TotalTokens; got != 174 {
		t.Fatalf("expected recorded codex usage total 174, got %d", got)
	}
	if got := l.broker.usage.Agents["ceo"].InputTokens; got != 123 {
		t.Fatalf("expected recorded input tokens 123, got %d", got)
	}
	if got := l.broker.usage.Agents["ceo"].CacheReadTokens; got != 45 {
		t.Fatalf("expected recorded cached input tokens 45, got %d", got)
	}
	if got := l.broker.usage.Agents["ceo"].OutputTokens; got != 6 {
		t.Fatalf("expected recorded output tokens 6, got %d", got)
	}
}

func TestHeadlessCodexRunTurnDispatchesPerAgentProvider(t *testing.T) {
	type dispatchCall struct {
		runner   string
		provider string
		slug     string
		channel  string
	}

	oldCodexRunner := headlessCodexRuntimeRunTurn
	oldClaudeRunner := headlessClaudeRuntimeRunTurn
	oldGeminiRunner := headlessGeminiRuntimeRunTurn
	oldOllamaRunner := headlessOllamaRuntimeRunTurn
	oldOpenClaudeRunner := headlessOpenClaudeRuntimeRunTurn
	defer func() {
		headlessCodexRuntimeRunTurn = oldCodexRunner
		headlessClaudeRuntimeRunTurn = oldClaudeRunner
		headlessGeminiRuntimeRunTurn = oldGeminiRunner
		headlessOllamaRuntimeRunTurn = oldOllamaRunner
		headlessOpenClaudeRuntimeRunTurn = oldOpenClaudeRunner
	}()

	tests := []struct {
		name           string
		globalProvider string
		slug           string
		bindingKind    string
		channel        string
		wantRunner     string
		wantProvider   string
	}{
		{
			name:           "codex binding overrides claude default",
			globalProvider: provider.KindClaudeCode,
			slug:           "agent-codex",
			bindingKind:    provider.KindCodex,
			channel:        "launch",
			wantRunner:     "codex",
		},
		{
			name:           "claude binding overrides codex default",
			globalProvider: provider.KindCodex,
			slug:           "agent-claude",
			bindingKind:    provider.KindClaudeCode,
			channel:        "ops",
			wantRunner:     "claude",
		},
		{
			name:           "gemini binding uses provider runtime",
			globalProvider: provider.KindClaudeCode,
			slug:           "agent-gemini",
			bindingKind:    provider.KindGemini,
			channel:        "research",
			wantRunner:     "gemini-native",
			wantProvider:   provider.KindGemini,
		},
		{
			name:           "gemini-vertex binding uses provider runtime",
			globalProvider: provider.KindCodex,
			slug:           "agent-gemini-vertex",
			bindingKind:    provider.KindGeminiVertex,
			channel:        "writing",
			wantRunner:     "gemini-native",
			wantProvider:   provider.KindGeminiVertex,
		},
		{
			name:           "ollama binding uses provider runtime",
			globalProvider: provider.KindClaudeCode,
			slug:           "agent-ollama",
			bindingKind:    provider.KindOllama,
			channel:        "support",
			wantRunner:     "ollama",
		},
		{
			name:           "openclaude binding uses provider runtime",
			globalProvider: provider.KindCodex,
			slug:           "agent-openclaude",
			bindingKind:    provider.KindOpenclaude,
			channel:        "writing",
			wantRunner:     "openclaude-compatible",
			wantProvider:   provider.KindOpenclaude,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := make(chan dispatchCall, 1)
			headlessCodexRuntimeRunTurn = func(_ *Launcher, _ context.Context, slug, notification string, channel ...string) error {
				calls <- dispatchCall{runner: "codex", slug: slug, channel: firstNonEmpty(channel...)}
				return nil
			}
			headlessClaudeRuntimeRunTurn = func(_ *Launcher, _ context.Context, slug, notification string, channel ...string) error {
				calls <- dispatchCall{runner: "claude", slug: slug, channel: firstNonEmpty(channel...)}
				return nil
			}
			headlessGeminiRuntimeRunTurn = func(_ *Launcher, _ context.Context, slug, providerKind, notification string, channel ...string) error {
				calls <- dispatchCall{runner: "gemini-native", provider: providerKind, slug: slug, channel: firstNonEmpty(channel...)}
				return nil
			}
			headlessOllamaRuntimeRunTurn = func(_ *Launcher, _ context.Context, slug, notification string, channel ...string) error {
				calls <- dispatchCall{runner: "ollama", slug: slug, channel: firstNonEmpty(channel...)}
				return nil
			}
			headlessOpenClaudeRuntimeRunTurn = func(_ *Launcher, _ context.Context, slug, providerKind, notification string, channel ...string) error {
				calls <- dispatchCall{runner: "openclaude-compatible", provider: providerKind, slug: slug, channel: firstNonEmpty(channel...)}
				return nil
			}

			b := NewBroker()
			b.mu.Lock()
			b.members = append(b.members, officeMember{
				Slug:     tt.slug,
				Name:     tt.slug,
				Provider: provider.ProviderBinding{Kind: tt.bindingKind},
			})
			b.memberIndex = nil
			b.mu.Unlock()

			l := newHeadlessLauncherForTest()
			l.broker = b
			l.provider = tt.globalProvider

			if err := headlessCodexRunTurn(l, context.Background(), tt.slug, "owned task", tt.channel); err != nil {
				t.Fatalf("headlessCodexRunTurn: %v", err)
			}

			got := waitForDispatchCall(t, calls)
			if got.runner != tt.wantRunner {
				t.Fatalf("runner = %q, want %q (provider=%q)", got.runner, tt.wantRunner, tt.bindingKind)
			}
			if got.provider != tt.wantProvider {
				t.Fatalf("provider = %q, want %q", got.provider, tt.wantProvider)
			}
			if got.slug != tt.slug {
				t.Fatalf("slug = %q, want %q", got.slug, tt.slug)
			}
			if tt.wantRunner != "claude" && got.channel != tt.channel {
				t.Fatalf("channel = %q, want %q", got.channel, tt.channel)
			}
		})
	}
}

func TestHeadlessCodexRunTurnRejectsUnsupportedProvider(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	b.members = append(b.members, officeMember{
		Slug:     "agent-unsupported",
		Name:     "agent-unsupported",
		Provider: provider.ProviderBinding{Kind: "custom-runtime"},
	})
	b.memberIndex = nil
	b.mu.Unlock()

	l := newHeadlessLauncherForTest()
	l.broker = b
	l.provider = provider.KindClaudeCode

	err := headlessCodexRunTurn(l, context.Background(), "agent-unsupported", "owned task", "general")
	if err == nil {
		t.Fatal("expected unsupported provider dispatch to fail")
	}
	if !strings.Contains(err.Error(), "unsupported") || !strings.Contains(err.Error(), "custom-runtime") {
		t.Fatalf("expected unsupported provider error, got %v", err)
	}
}

func TestHeadlessCodexRunTurnUsesActiveTurnProviderOverride(t *testing.T) {
	type dispatchCall struct {
		runner  string
		slug    string
		channel string
	}

	oldCodexRunner := headlessCodexRuntimeRunTurn
	oldClaudeRunner := headlessClaudeRuntimeRunTurn
	defer func() {
		headlessCodexRuntimeRunTurn = oldCodexRunner
		headlessClaudeRuntimeRunTurn = oldClaudeRunner
	}()

	calls := make(chan dispatchCall, 1)
	headlessCodexRuntimeRunTurn = func(_ *Launcher, _ context.Context, slug, notification string, channel ...string) error {
		calls <- dispatchCall{runner: "codex", slug: slug, channel: firstNonEmpty(channel...)}
		return nil
	}
	headlessClaudeRuntimeRunTurn = func(_ *Launcher, _ context.Context, slug, notification string, channel ...string) error {
		calls <- dispatchCall{runner: "claude", slug: slug, channel: firstNonEmpty(channel...)}
		return nil
	}

	l := newHeadlessLauncherForTest()
	l.provider = provider.KindCodex
	l.headlessActive["eng"] = &headlessCodexActiveTurn{
		Turn: headlessCodexTurn{
			Channel:          "general",
			ProviderOverride: provider.KindClaudeCode,
		},
	}

	if err := headlessCodexRunTurn(l, context.Background(), "eng", "owned task", "general"); err != nil {
		t.Fatalf("headlessCodexRunTurn: %v", err)
	}

	got := waitForDispatchCall(t, calls)
	if got.runner != "claude" {
		t.Fatalf("expected provider override to dispatch claude, got %+v", got)
	}
}

func TestHeadlessCodexRunTurnReroutesLocalWorktreeToCodex(t *testing.T) {
	type dispatchCall struct {
		runner  string
		slug    string
		channel string
	}

	oldCodexRunner := headlessCodexRuntimeRunTurn
	oldGeminiRunner := headlessGeminiRuntimeRunTurn
	defer func() {
		headlessCodexRuntimeRunTurn = oldCodexRunner
		headlessGeminiRuntimeRunTurn = oldGeminiRunner
	}()

	calls := make(chan dispatchCall, 1)
	headlessCodexRuntimeRunTurn = func(_ *Launcher, _ context.Context, slug, notification string, channel ...string) error {
		calls <- dispatchCall{runner: "codex", slug: slug, channel: firstNonEmpty(channel...)}
		return nil
	}
	headlessGeminiRuntimeRunTurn = func(_ *Launcher, _ context.Context, slug, providerKind, notification string, channel ...string) error {
		calls <- dispatchCall{runner: "gemini", slug: slug, channel: firstNonEmpty(channel...)}
		return nil
	}

	b := NewBroker()
	b.mu.Lock()
	b.members = append(b.members, officeMember{
		Slug:     "reviewer",
		Name:     "reviewer",
		Provider: provider.ProviderBinding{Kind: provider.KindGemini},
	})
	b.tasks = append(b.tasks, teamTask{
		ID:            "task-782",
		Channel:       "general",
		Title:         "Revisao tecnica priorizada da base DunderIA atual",
		Owner:         "reviewer",
		Status:        "in_progress",
		ExecutionMode: "local_worktree",
		WorktreePath:  `<USER_HOME>\.wuphf\task-worktrees\dunderia\wuphf-task-task-782`,
	})
	b.memberIndex = nil
	b.taskIndexes = brokerTaskIndexes{}
	b.mu.Unlock()

	l := newHeadlessLauncherForTest()
	l.broker = b
	l.provider = provider.KindClaudeCode

	if err := headlessCodexRunTurn(l, context.Background(), "reviewer", "owned task", "general"); err != nil {
		t.Fatalf("headlessCodexRunTurn: %v", err)
	}

	got := waitForDispatchCall(t, calls)
	if got.runner != "codex" {
		t.Fatalf("expected local_worktree task to reroute to codex, got %+v", got)
	}
}

func TestHeadlessCodexRunTurnIgnoresEmptyActiveTurnProviderOverride(t *testing.T) {
	type dispatchCall struct {
		runner  string
		slug    string
		channel string
	}

	oldCodexRunner := headlessCodexRuntimeRunTurn
	oldClaudeRunner := headlessClaudeRuntimeRunTurn
	defer func() {
		headlessCodexRuntimeRunTurn = oldCodexRunner
		headlessClaudeRuntimeRunTurn = oldClaudeRunner
	}()

	calls := make(chan dispatchCall, 1)
	headlessCodexRuntimeRunTurn = func(_ *Launcher, _ context.Context, slug, notification string, channel ...string) error {
		calls <- dispatchCall{runner: "codex", slug: slug, channel: firstNonEmpty(channel...)}
		return nil
	}
	headlessClaudeRuntimeRunTurn = func(_ *Launcher, _ context.Context, slug, notification string, channel ...string) error {
		calls <- dispatchCall{runner: "claude", slug: slug, channel: firstNonEmpty(channel...)}
		return nil
	}

	b := NewBroker()
	b.mu.Lock()
	b.members = append(b.members, officeMember{
		Slug:     "eng",
		Name:     "eng",
		Provider: provider.ProviderBinding{Kind: provider.KindCodex},
	})
	b.memberIndex = nil
	b.mu.Unlock()

	l := newHeadlessLauncherForTest()
	l.broker = b
	l.provider = provider.KindClaudeCode
	l.headlessActive["eng"] = &headlessCodexActiveTurn{
		Turn: headlessCodexTurn{
			Channel:          "general",
			ProviderOverride: "",
		},
	}

	if err := headlessCodexRunTurn(l, context.Background(), "eng", "owned task", "general"); err != nil {
		t.Fatalf("headlessCodexRunTurn: %v", err)
	}

	got := waitForDispatchCall(t, calls)
	if got.runner != "codex" {
		t.Fatalf("expected empty provider override to fall back to member provider, got %+v", got)
	}
}

func TestEnqueueHeadlessCodexTurnRecordClearsProviderOverride(t *testing.T) {
	l := newHeadlessLauncherForTest()

	l.enqueueHeadlessCodexTurnRecord("eng", headlessCodexTurn{
		Prompt:           "owned task",
		Channel:          "general",
		ProviderOverride: provider.KindClaudeCode,
	})

	queue := l.headlessQueues["eng"]
	if len(queue) != 1 {
		t.Fatalf("expected one queued turn, got %+v", queue)
	}
	if got := queue[0].ProviderOverride; got != "" {
		t.Fatalf("expected provider override to be cleared, got %q", got)
	}
}

func TestRunHeadlessOpenClaudeTurnUsesVertexProvider(t *testing.T) {
	testRunHeadlessOpenClaudeTurnUsesProvider(t, provider.KindOpenclaude, "vertex")
}

func TestRunHeadlessOpenClaudeTurnUsesGeminiProvider(t *testing.T) {
	testRunHeadlessOpenClaudeTurnUsesProvider(t, provider.KindGemini, "gemini")
}
func TestRunHeadlessCodexTurnUsesAssignedWorktreeForCodingAgents(t *testing.T) {
	recordFile := filepath.Join(t.TempDir(), "headless-codex-record.jsonl")
	worktreeDir := t.TempDir()
	repoRoot := t.TempDir()

	oldLookPath := headlessCodexLookPath
	oldExecutablePath := headlessCodexExecutablePath
	oldCommandContext := headlessCodexCommandContext
	oldPrepareTaskWorktree := prepareTaskWorktree
	headlessCodexLookPath = func(file string) (string, error) {
		switch file {
		case "codex":
			return "/usr/bin/codex", nil
		default:
			return "", exec.ErrNotFound
		}
	}
	headlessCodexExecutablePath = func() (string, error) { return "/tmp/wuphf", nil }
	headlessCodexCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmdArgs := []string{"-test.run=TestHeadlessCodexHelperProcess", "--"}
		cmdArgs = append(cmdArgs, args...)
		return exec.CommandContext(ctx, os.Args[0], cmdArgs...)
	}
	prepareTaskWorktree = func(taskID string) (string, string, error) {
		return worktreeDir, worktreeBranchName(taskID), nil
	}
	defer func() {
		headlessCodexLookPath = oldLookPath
		headlessCodexExecutablePath = oldExecutablePath
		headlessCodexCommandContext = oldCommandContext
		prepareTaskWorktree = oldPrepareTaskWorktree
	}()

	t.Setenv("GO_WANT_HEADLESS_CODEX_HELPER_PROCESS", "1")
	t.Setenv("HEADLESS_CODEX_RECORD_FILE", recordFile)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PWD", repoRoot)
	t.Setenv("OLDPWD", "/tmp/previous")
	t.Setenv("CODEX_THREAD_ID", "thread-from-controller")
	t.Setenv("CODEX_TUI_RECORD_SESSION", "1")
	t.Setenv("CODEX_TUI_SESSION_LOG_PATH", "/tmp/controller-session.jsonl")

	broker := NewBroker()
	ensureTestMemberAccess(broker, "general", "builder", "Builder")
	ensureTestMemberAccess(broker, "general", "operator", "Operator")
	task, _, err := broker.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Build the automation runtime",
		Details:       "Implement in the assigned worktree.",
		Owner:         "eng",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		PipelineID:    "feature",
		ExecutionMode: "local_worktree",
		ReviewState:   "pending_review",
	})
	if err != nil {
		t.Fatalf("EnsurePlannedTask: %v", err)
	}
	if task.WorktreePath != worktreeDir {
		t.Fatalf("expected assigned worktree %q, got %q", worktreeDir, task.WorktreePath)
	}

	l := &Launcher{
		pack:        agent.GetPack("founding-team"),
		cwd:         repoRoot,
		broker:      broker,
		headlessCtx: context.Background(),
	}

	if err := l.runHeadlessCodexTurn(context.Background(), "eng", "Ship the automation runtime."); err != nil {
		t.Fatalf("runHeadlessCodexTurn: %v", err)
	}

	record := readHeadlessCodexRecord(t, recordFile)
	joinedArgs := strings.Join(record.Args, " ")
	if got := argValue(record.Args, "-C"); !samePath(got, worktreeDir) {
		t.Fatalf("expected codex worktree %q, got %q", worktreeDir, got)
	}
	if !strings.Contains(joinedArgs, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("expected dangerous bypass for local worktree turn, got %#v", record.Args)
	}
	if strings.Contains(joinedArgs, "-s workspace-write") {
		t.Fatalf("did not expect workspace-write sandbox for local worktree turn, got %#v", record.Args)
	}
	if !strings.Contains(joinedArgs, "--disable plugins") {
		t.Fatalf("expected plugins feature to be disabled, got %#v", record.Args)
	}
	if !samePath(record.Dir, worktreeDir) {
		t.Fatalf("expected command dir %q, got %q", worktreeDir, record.Dir)
	}
	if got := envValue(record.Env, "WUPHF_WORKTREE_PATH"); !samePath(got, worktreeDir) {
		t.Fatalf("expected worktree env, got %#v", record.Env)
	}
	if got := envValue(record.Env, "PWD"); !samePath(got, worktreeDir) {
		t.Fatalf("expected PWD to match worktree, got %#v", record.Env)
	}
	wantCodexHome := filepath.Join(os.Getenv("HOME"), ".wuphf", "codex-headless")
	if !containsEnv(record.Env, "HOME="+wantCodexHome) {
		t.Fatalf("expected isolated HOME env, got %#v", record.Env)
	}
	if !containsEnv(record.Env, "CODEX_HOME="+wantCodexHome) {
		t.Fatalf("expected absolute CODEX_HOME env, got %#v", record.Env)
	}
	if got := envValue(record.Env, "GOCACHE"); !samePath(got, filepath.Join(worktreeDir, ".wuphf", "cache", "go-build", "eng")) {
		t.Fatalf("expected worktree-local GOCACHE, got %#v", record.Env)
	}
	if got := envValue(record.Env, "GOTMPDIR"); !samePath(got, filepath.Join(worktreeDir, ".wuphf", "cache", "go-tmp", "eng")) {
		t.Fatalf("expected worktree-local GOTMPDIR, got %#v", record.Env)
	}
	for _, forbiddenPrefix := range []string{
		"OLDPWD=",
		"CODEX_THREAD_ID=",
		"CODEX_TUI_RECORD_SESSION=",
		"CODEX_TUI_SESSION_LOG_PATH=",
	} {
		if containsEnvPrefix(record.Env, forbiddenPrefix) {
			t.Fatalf("expected %s to be stripped, got %#v", forbiddenPrefix, record.Env)
		}
	}
}

func TestRunHeadlessCodexTurnUsesAssignedWorktreeForLocalWorktreeBuilder(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	recordFile := filepath.Join(t.TempDir(), "headless-codex-record.jsonl")
	worktreeDir := t.TempDir()
	repoRoot := t.TempDir()

	oldLookPath := headlessCodexLookPath
	oldExecutablePath := headlessCodexExecutablePath
	oldCommandContext := headlessCodexCommandContext
	oldPrepareTaskWorktree := prepareTaskWorktree
	headlessCodexLookPath = func(file string) (string, error) {
		switch file {
		case "codex":
			return "/usr/bin/codex", nil
		default:
			return "", exec.ErrNotFound
		}
	}
	headlessCodexExecutablePath = func() (string, error) { return "/tmp/wuphf", nil }
	headlessCodexCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmdArgs := []string{"-test.run=TestHeadlessCodexHelperProcess", "--"}
		cmdArgs = append(cmdArgs, args...)
		return exec.CommandContext(ctx, os.Args[0], cmdArgs...)
	}
	prepareTaskWorktree = func(taskID string) (string, string, error) {
		return worktreeDir, worktreeBranchName(taskID), nil
	}
	defer func() {
		headlessCodexLookPath = oldLookPath
		headlessCodexExecutablePath = oldExecutablePath
		headlessCodexCommandContext = oldCommandContext
		prepareTaskWorktree = oldPrepareTaskWorktree
	}()

	t.Setenv("GO_WANT_HEADLESS_CODEX_HELPER_PROCESS", "1")
	t.Setenv("HEADLESS_CODEX_RECORD_FILE", recordFile)
	t.Setenv("PWD", repoRoot)

	broker := NewBroker()
	ensureTestMemberAccess(broker, "general", "builder", "Builder")
	task, _, err := broker.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Build the dry-run intake packet",
		Details:       "Implement in the assigned worktree.",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		PipelineID:    "feature",
		ExecutionMode: "local_worktree",
		ReviewState:   "pending_review",
	})
	if err != nil {
		t.Fatalf("EnsurePlannedTask: %v", err)
	}
	if task.WorktreePath != worktreeDir {
		t.Fatalf("expected assigned worktree %q, got %q", worktreeDir, task.WorktreePath)
	}

	l := &Launcher{
		pack:        agent.GetPack("founding-team"),
		cwd:         repoRoot,
		broker:      broker,
		headlessCtx: context.Background(),
	}

	if err := l.runHeadlessCodexTurn(context.Background(), "builder", "Ship the intake packet."); err != nil {
		t.Fatalf("runHeadlessCodexTurn: %v", err)
	}

	record := readHeadlessCodexRecord(t, recordFile)
	joinedArgs := strings.Join(record.Args, " ")
	if got := argValue(record.Args, "-C"); !samePath(got, worktreeDir) {
		t.Fatalf("expected codex worktree %q, got %q", worktreeDir, got)
	}
	if !strings.Contains(joinedArgs, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("expected dangerous bypass for local worktree turn, got %#v", record.Args)
	}
	if !samePath(record.Dir, worktreeDir) {
		t.Fatalf("expected command dir %q, got %q", worktreeDir, record.Dir)
	}
	if got := envValue(record.Env, "WUPHF_WORKTREE_PATH"); !samePath(got, worktreeDir) {
		t.Fatalf("expected worktree env, got %#v", record.Env)
	}
}

func TestRunHeadlessCodexTurnUsesAssignedExternalWorkspace(t *testing.T) {
	recordFile := filepath.Join(t.TempDir(), "headless-codex-record.jsonl")
	workspaceDir := t.TempDir()
	repoRoot := t.TempDir()

	oldLookPath := headlessCodexLookPath
	oldExecutablePath := headlessCodexExecutablePath
	oldCommandContext := headlessCodexCommandContext
	headlessCodexLookPath = func(file string) (string, error) {
		switch file {
		case "codex":
			return "/usr/bin/codex", nil
		default:
			return "", exec.ErrNotFound
		}
	}
	headlessCodexExecutablePath = func() (string, error) { return "/tmp/wuphf", nil }
	headlessCodexCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmdArgs := []string{"-test.run=TestHeadlessCodexHelperProcess", "--"}
		cmdArgs = append(cmdArgs, args...)
		return exec.CommandContext(ctx, os.Args[0], cmdArgs...)
	}
	defer func() {
		headlessCodexLookPath = oldLookPath
		headlessCodexExecutablePath = oldExecutablePath
		headlessCodexCommandContext = oldCommandContext
	}()

	t.Setenv("GO_WANT_HEADLESS_CODEX_HELPER_PROCESS", "1")
	t.Setenv("HEADLESS_CODEX_RECORD_FILE", recordFile)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PWD", repoRoot)

	broker := NewBroker()
	broker.tasks = []teamTask{{
		ID:            "task-91",
		Channel:       "general",
		Title:         "Inspect external repo",
		Owner:         "builder",
		Status:        "in_progress",
		CreatedBy:     "ceo",
		ExecutionMode: "external_workspace",
		WorkspacePath: workspaceDir,
	}}

	l := &Launcher{
		pack:        agent.GetPack("founding-team"),
		cwd:         repoRoot,
		broker:      broker,
		headlessCtx: context.Background(),
	}

	if err := l.runHeadlessCodexTurn(context.Background(), "builder", "Inspect the external workspace."); err != nil {
		t.Fatalf("runHeadlessCodexTurn: %v", err)
	}

	record := readHeadlessCodexRecord(t, recordFile)
	joinedArgs := strings.Join(record.Args, " ")
	if got := argValue(record.Args, "-C"); !samePath(got, workspaceDir) {
		t.Fatalf("expected codex workspace %q, got %q", workspaceDir, got)
	}
	if !strings.Contains(joinedArgs, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("expected dangerous bypass for external workspace turn, got %#v", record.Args)
	}
	if strings.Contains(joinedArgs, "-s workspace-write") {
		t.Fatalf("did not expect workspace-write sandbox for external workspace turn, got %#v", record.Args)
	}
	if !samePath(record.Dir, workspaceDir) {
		t.Fatalf("expected command dir %q, got %q", workspaceDir, record.Dir)
	}
	if got := envValue(record.Env, "WUPHF_WORKSPACE_PATH"); !samePath(got, workspaceDir) {
		t.Fatalf("expected workspace env, got %#v", record.Env)
	}
	if got := envValue(record.Env, "PWD"); !samePath(got, workspaceDir) {
		t.Fatalf("expected PWD to match external workspace, got %#v", record.Env)
	}
}

func TestRunHeadlessCodexTurnDisablesCodexLBWebsocketsForNonASCIIWorkspace(t *testing.T) {
	recordFile := filepath.Join(t.TempDir(), "headless-codex-record.jsonl")
	workspaceDir := filepath.Join(t.TempDir(), "Repositórios", "LegacySystemNew")
	repoRoot := t.TempDir()
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll workspace: %v", err)
	}

	oldLookPath := headlessCodexLookPath
	oldExecutablePath := headlessCodexExecutablePath
	oldCommandContext := headlessCodexCommandContext
	headlessCodexLookPath = func(file string) (string, error) {
		switch file {
		case "codex":
			return "/usr/bin/codex", nil
		default:
			return "", exec.ErrNotFound
		}
	}
	headlessCodexExecutablePath = func() (string, error) { return "/tmp/wuphf", nil }
	headlessCodexCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmdArgs := []string{"-test.run=TestHeadlessCodexHelperProcess", "--"}
		cmdArgs = append(cmdArgs, args...)
		return exec.CommandContext(ctx, os.Args[0], cmdArgs...)
	}
	defer func() {
		headlessCodexLookPath = oldLookPath
		headlessCodexExecutablePath = oldExecutablePath
		headlessCodexCommandContext = oldCommandContext
	}()

	t.Setenv("GO_WANT_HEADLESS_CODEX_HELPER_PROCESS", "1")
	t.Setenv("HEADLESS_CODEX_RECORD_FILE", recordFile)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PWD", repoRoot)

	broker := NewBroker()
	broker.tasks = []teamTask{{
		ID:            "task-utf8-header",
		Channel:       "general",
		Title:         "Inspect external repo",
		Owner:         "builder",
		Status:        "in_progress",
		CreatedBy:     "ceo",
		ExecutionMode: "external_workspace",
		WorkspacePath: workspaceDir,
	}}

	l := &Launcher{
		pack:        agent.GetPack("founding-team"),
		cwd:         repoRoot,
		broker:      broker,
		headlessCtx: context.Background(),
	}

	if err := l.runHeadlessCodexTurn(context.Background(), "builder", "Inspect the external workspace."); err != nil {
		t.Fatalf("runHeadlessCodexTurn: %v", err)
	}

	record := readHeadlessCodexRecord(t, recordFile)
	joinedArgs := strings.Join(record.Args, " ")
	if !strings.Contains(joinedArgs, `model_providers.codex-lb.supports_websockets=false`) {
		t.Fatalf("expected non-ASCII workspace to disable codex-lb websockets, got %#v", record.Args)
	}
}

func TestRunHeadlessCodexTurnGameMasterUsesDangerousBypassWithoutTask(t *testing.T) {
	recordFile := filepath.Join(t.TempDir(), "headless-codex-record.jsonl")
	repoRoot := t.TempDir()

	oldLookPath := headlessCodexLookPath
	oldExecutablePath := headlessCodexExecutablePath
	oldCommandContext := headlessCodexCommandContext
	headlessCodexLookPath = func(file string) (string, error) {
		switch file {
		case "codex":
			return "/usr/bin/codex", nil
		default:
			return "", exec.ErrNotFound
		}
	}
	headlessCodexExecutablePath = func() (string, error) { return "/tmp/wuphf", nil }
	headlessCodexCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmdArgs := []string{"-test.run=TestHeadlessCodexHelperProcess", "--"}
		cmdArgs = append(cmdArgs, args...)
		return exec.CommandContext(ctx, os.Args[0], cmdArgs...)
	}
	defer func() {
		headlessCodexLookPath = oldLookPath
		headlessCodexExecutablePath = oldExecutablePath
		headlessCodexCommandContext = oldCommandContext
	}()

	t.Setenv("GO_WANT_HEADLESS_CODEX_HELPER_PROCESS", "1")
	t.Setenv("HEADLESS_CODEX_RECORD_FILE", recordFile)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PWD", repoRoot)

	broker := NewBroker()
	ensureTestMemberAccess(broker, "general", "game-master", "Game Master")

	l := &Launcher{
		pack: &agent.PackDefinition{
			LeadSlug: "ceo",
			Agents: []agent.AgentConfig{
				{Slug: "ceo", Name: "CEO"},
				{Slug: "game-master", Name: "Game Master"},
			},
		},
		cwd:         repoRoot,
		broker:      broker,
		headlessCtx: context.Background(),
	}

	if err := l.runHeadlessCodexTurn(context.Background(), "game-master", "Inspect and fix directly."); err != nil {
		t.Fatalf("runHeadlessCodexTurn: %v", err)
	}

	record := readHeadlessCodexRecord(t, recordFile)
	joinedArgs := strings.Join(record.Args, " ")
	if !strings.Contains(joinedArgs, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("expected dangerous bypass for game-master turn, got %#v", record.Args)
	}
	if strings.Contains(joinedArgs, "-s workspace-write") {
		t.Fatalf("did not expect workspace-write sandbox for game-master turn, got %#v", record.Args)
	}
}

func TestRunHeadlessCodexTurnDisablesWebsocketsWhenCodexConfigHasNonASCIITrustedProjectPath(t *testing.T) {
	recordFile := filepath.Join(t.TempDir(), "headless-codex-record.jsonl")
	repoRoot := filepath.Join(t.TempDir(), "Repos", "dunderia")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll repo root: %v", err)
	}
	sourceHome := t.TempDir()
	runtimeHome := t.TempDir()
	sourceCodexHome := filepath.Join(sourceHome, ".codex")
	if err := os.MkdirAll(sourceCodexHome, 0o755); err != nil {
		t.Fatalf("MkdirAll source codex home: %v", err)
	}
	configText := strings.Join([]string{
		`model_provider = "codex-lb"`,
		``,
		`[model_providers.codex-lb]`,
		`supports_websockets = true`,
		``,
		`[projects.'<REPOS_ROOT>\LegacySystemNew']`,
		`trust_level = "trusted"`,
		``,
	}, "\n")
	if err := os.WriteFile(filepath.Join(sourceCodexHome, "config.toml"), []byte(configText), 0o600); err != nil {
		t.Fatalf("write source config: %v", err)
	}

	oldLookPath := headlessCodexLookPath
	oldExecutablePath := headlessCodexExecutablePath
	oldCommandContext := headlessCodexCommandContext
	headlessCodexLookPath = func(file string) (string, error) {
		switch file {
		case "codex":
			return "/usr/bin/codex", nil
		default:
			return "", exec.ErrNotFound
		}
	}
	headlessCodexExecutablePath = func() (string, error) { return "/tmp/wuphf", nil }
	headlessCodexCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmdArgs := []string{"-test.run=TestHeadlessCodexHelperProcess", "--"}
		cmdArgs = append(cmdArgs, args...)
		return exec.CommandContext(ctx, os.Args[0], cmdArgs...)
	}
	defer func() {
		headlessCodexLookPath = oldLookPath
		headlessCodexExecutablePath = oldExecutablePath
		headlessCodexCommandContext = oldCommandContext
	}()

	t.Setenv("GO_WANT_HEADLESS_CODEX_HELPER_PROCESS", "1")
	t.Setenv("HEADLESS_CODEX_RECORD_FILE", recordFile)
	t.Setenv("HOME", runtimeHome)
	t.Setenv("USERPROFILE", runtimeHome)
	t.Setenv("WUPHF_GLOBAL_HOME", sourceHome)
	t.Setenv("PWD", repoRoot)

	broker := NewBroker()
	l := &Launcher{
		pack:        agent.GetPack("founding-team"),
		cwd:         repoRoot,
		broker:      broker,
		headlessCtx: context.Background(),
	}

	if err := l.runHeadlessCodexTurn(context.Background(), "ceo", "Inspect the office state."); err != nil {
		t.Fatalf("runHeadlessCodexTurn: %v", err)
	}

	record := readHeadlessCodexRecord(t, recordFile)
	joinedArgs := strings.Join(record.Args, " ")
	if !strings.Contains(joinedArgs, `model_providers.codex-lb.supports_websockets=false`) {
		t.Fatalf("expected non-ASCII trusted project path to disable codex-lb websockets, got %#v", record.Args)
	}
}

func TestRunHeadlessCodexTurnUsesActiveTurnExternalWorkspaceWhenTaskIsNotInProgress(t *testing.T) {
	recordFile := filepath.Join(t.TempDir(), "headless-codex-record.jsonl")
	workspaceDir := t.TempDir()
	repoRoot := t.TempDir()

	oldLookPath := headlessCodexLookPath
	oldExecutablePath := headlessCodexExecutablePath
	oldCommandContext := headlessCodexCommandContext
	headlessCodexLookPath = func(file string) (string, error) {
		switch file {
		case "codex":
			return "/usr/bin/codex", nil
		default:
			return "", exec.ErrNotFound
		}
	}
	headlessCodexExecutablePath = func() (string, error) { return "/tmp/wuphf", nil }
	headlessCodexCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmdArgs := []string{"-test.run=TestHeadlessCodexHelperProcess", "--"}
		cmdArgs = append(cmdArgs, args...)
		return exec.CommandContext(ctx, os.Args[0], cmdArgs...)
	}
	defer func() {
		headlessCodexLookPath = oldLookPath
		headlessCodexExecutablePath = oldExecutablePath
		headlessCodexCommandContext = oldCommandContext
	}()

	t.Setenv("GO_WANT_HEADLESS_CODEX_HELPER_PROCESS", "1")
	t.Setenv("HEADLESS_CODEX_RECORD_FILE", recordFile)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PWD", repoRoot)

	broker := NewBroker()
	broker.tasks = []teamTask{{
		ID:            "task-1682",
		Channel:       "legado-para-novo",
		Title:         "Inspect external repo",
		Owner:         "builder",
		Status:        "blocked",
		CreatedBy:     "ceo",
		ExecutionMode: "external_workspace",
		WorkspacePath: workspaceDir,
	}}

	l := &Launcher{
		pack:        agent.GetPack("founding-team"),
		cwd:         repoRoot,
		broker:      broker,
		headlessCtx: context.Background(),
		headlessActive: map[string]*headlessCodexActiveTurn{
			"builder": {Turn: headlessCodexTurn{TaskID: "task-1682"}},
		},
	}

	if err := l.runHeadlessCodexTurn(context.Background(), "builder", "Inspect the external workspace."); err != nil {
		t.Fatalf("runHeadlessCodexTurn: %v", err)
	}

	record := readHeadlessCodexRecord(t, recordFile)
	joinedArgs := strings.Join(record.Args, " ")
	if got := argValue(record.Args, "-C"); !samePath(got, workspaceDir) {
		t.Fatalf("expected codex workspace %q, got %q", workspaceDir, got)
	}
	if !strings.Contains(joinedArgs, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("expected dangerous bypass for external workspace turn, got %#v", record.Args)
	}
	if !samePath(record.Dir, workspaceDir) {
		t.Fatalf("expected command dir %q, got %q", workspaceDir, record.Dir)
	}
	if got := envValue(record.Env, "WUPHF_WORKSPACE_PATH"); !samePath(got, workspaceDir) {
		t.Fatalf("expected workspace env, got %#v", record.Env)
	}
}

func TestRunHeadlessCodexTurnPassesScopedChannelEnv(t *testing.T) {
	recordFile := filepath.Join(t.TempDir(), "headless-codex-record.jsonl")
	oldLookPath := headlessCodexLookPath
	oldExecutablePath := headlessCodexExecutablePath
	oldCommandContext := headlessCodexCommandContext
	headlessCodexLookPath = func(file string) (string, error) {
		switch file {
		case "codex":
			return "/usr/bin/codex", nil
		default:
			return "", exec.ErrNotFound
		}
	}
	headlessCodexExecutablePath = func() (string, error) { return "/tmp/wuphf", nil }
	headlessCodexCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmdArgs := []string{"-test.run=TestHeadlessCodexHelperProcess", "--"}
		cmdArgs = append(cmdArgs, args...)
		return exec.CommandContext(ctx, os.Args[0], cmdArgs...)
	}
	defer func() {
		headlessCodexLookPath = oldLookPath
		headlessCodexExecutablePath = oldExecutablePath
		headlessCodexCommandContext = oldCommandContext
	}()

	t.Setenv("GO_WANT_HEADLESS_CODEX_HELPER_PROCESS", "1")
	t.Setenv("HEADLESS_CODEX_RECORD_FILE", recordFile)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("WUPHF_CHANNEL", "general")

	l := &Launcher{
		pack:        agent.GetPack("founding-team"),
		cwd:         t.TempDir(),
		broker:      NewBroker(),
		headlessCtx: context.Background(),
	}

	if err := l.runHeadlessCodexTurn(context.Background(), "eng", "Work the owned task.", "launch-ops"); err != nil {
		t.Fatalf("runHeadlessCodexTurn: %v", err)
	}

	record := readHeadlessCodexRecord(t, recordFile)
	if !containsEnv(record.Env, "WUPHF_CHANNEL=launch-ops") {
		t.Fatalf("expected scoped channel env, got %#v", record.Env)
	}
}

func TestRunHeadlessCodexTurnUsesChannelPrimaryLinkedRepoWorkspace(t *testing.T) {
	recordFile := filepath.Join(t.TempDir(), "headless-codex-record.jsonl")
	channelWorkspace := t.TempDir()
	repoRoot := t.TempDir()
	initScopedTestGitWorktree(t, channelWorkspace)

	oldLookPath := headlessCodexLookPath
	oldExecutablePath := headlessCodexExecutablePath
	oldCommandContext := headlessCodexCommandContext
	headlessCodexLookPath = func(file string) (string, error) {
		switch file {
		case "codex":
			return "/usr/bin/codex", nil
		default:
			return "", exec.ErrNotFound
		}
	}
	headlessCodexExecutablePath = func() (string, error) { return "/tmp/wuphf", nil }
	headlessCodexCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmdArgs := []string{"-test.run=TestHeadlessCodexHelperProcess", "--"}
		cmdArgs = append(cmdArgs, args...)
		return exec.CommandContext(ctx, os.Args[0], cmdArgs...)
	}
	defer func() {
		headlessCodexLookPath = oldLookPath
		headlessCodexExecutablePath = oldExecutablePath
		headlessCodexCommandContext = oldCommandContext
	}()

	t.Setenv("GO_WANT_HEADLESS_CODEX_HELPER_PROCESS", "1")
	t.Setenv("HEADLESS_CODEX_RECORD_FILE", recordFile)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PWD", repoRoot)

	broker := NewBroker()
	broker.mu.Lock()
	broker.channels = []teamChannel{{
		Slug:    "launch-ops",
		Name:    "Launch Ops",
		Members: []string{"eng"},
	}}
	if !broker.upsertLinkedRepoForChannelLocked("launch-ops", channelWorkspace, "human_message", "you") {
		broker.mu.Unlock()
		t.Fatal("expected linked repo upsert")
	}
	broker.mu.Unlock()

	l := &Launcher{
		pack:        agent.GetPack("founding-team"),
		cwd:         repoRoot,
		broker:      broker,
		headlessCtx: context.Background(),
	}

	if err := l.runHeadlessCodexTurn(context.Background(), "eng", "Inspect the linked repo.", "launch-ops"); err != nil {
		t.Fatalf("runHeadlessCodexTurn: %v", err)
	}

	record := readHeadlessCodexRecord(t, recordFile)
	joinedArgs := strings.Join(record.Args, " ")
	if got := argValue(record.Args, "-C"); !samePath(got, channelWorkspace) {
		t.Fatalf("expected codex workspace %q, got %q", channelWorkspace, got)
	}
	if !strings.Contains(joinedArgs, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("expected dangerous bypass for linked repo workspace, got %#v", record.Args)
	}
	if strings.Contains(joinedArgs, "-s workspace-write") {
		t.Fatalf("did not expect workspace-write sandbox for linked repo workspace, got %#v", record.Args)
	}
	if !samePath(record.Dir, channelWorkspace) {
		t.Fatalf("expected command dir %q, got %q", channelWorkspace, record.Dir)
	}
	if got := envValue(record.Env, "WUPHF_WORKSPACE_PATH"); !samePath(got, channelWorkspace) {
		t.Fatalf("expected workspace env %q, got %#v", channelWorkspace, record.Env)
	}
	if !containsEnv(record.Env, "WUPHF_CHANNEL=launch-ops") {
		t.Fatalf("expected scoped channel env, got %#v", record.Env)
	}
}

func TestRunHeadlessCodexTurnPrefersActiveTaskRepoContextOverChannelLink(t *testing.T) {
	recordFile := filepath.Join(t.TempDir(), "headless-codex-record.jsonl")
	repoRoot := t.TempDir()
	activeWorkspace := t.TempDir()
	channelWorkspace := t.TempDir()
	initScopedTestGitWorktree(t, repoRoot)
	initScopedTestGitWorktree(t, activeWorkspace)
	initScopedTestGitWorktree(t, channelWorkspace)

	oldLookPath := headlessCodexLookPath
	oldExecutablePath := headlessCodexExecutablePath
	oldCommandContext := headlessCodexCommandContext
	headlessCodexLookPath = func(file string) (string, error) {
		switch file {
		case "codex":
			return "/usr/bin/codex", nil
		default:
			return "", exec.ErrNotFound
		}
	}
	headlessCodexExecutablePath = func() (string, error) { return "/tmp/wuphf", nil }
	headlessCodexCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmdArgs := []string{"-test.run=TestHeadlessCodexHelperProcess", "--"}
		cmdArgs = append(cmdArgs, args...)
		return exec.CommandContext(ctx, os.Args[0], cmdArgs...)
	}
	defer func() {
		headlessCodexLookPath = oldLookPath
		headlessCodexExecutablePath = oldExecutablePath
		headlessCodexCommandContext = oldCommandContext
	}()

	t.Setenv("GO_WANT_HEADLESS_CODEX_HELPER_PROCESS", "1")
	t.Setenv("HEADLESS_CODEX_RECORD_FILE", recordFile)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PWD", repoRoot)

	broker := NewBroker()
	broker.mu.Lock()
	broker.channels = []teamChannel{{
		Slug:    "launch-ops",
		Name:    "Launch Ops",
		Members: []string{"ceo", "eng"},
	}}
	if !broker.upsertLinkedRepoForChannelLocked("launch-ops", channelWorkspace, "human_message", "you") {
		broker.mu.Unlock()
		t.Fatal("expected linked repo upsert")
	}
	broker.tasks = []teamTask{
		{
			ID:            "task-1",
			Channel:       "launch-ops",
			Owner:         "eng",
			Status:        "in_progress",
			ExecutionMode: "external_workspace",
			RepoContext: &taskGitHubRepoContext{
				RootPath: activeWorkspace,
			},
		},
	}
	broker.mu.Unlock()

	l := &Launcher{
		pack:        agent.GetPack("founding-team"),
		cwd:         repoRoot,
		broker:      broker,
		headlessCtx: context.Background(),
	}

	if err := l.runHeadlessCodexTurn(context.Background(), "eng", "Inspect the active task workspace.", "launch-ops"); err != nil {
		t.Fatalf("runHeadlessCodexTurn: %v", err)
	}

	record := readHeadlessCodexRecord(t, recordFile)
	joinedArgs := strings.Join(record.Args, " ")
	if got := argValue(record.Args, "-C"); !samePath(got, activeWorkspace) {
		t.Fatalf("expected active task workspace %q, got %q", activeWorkspace, got)
	}
	if !strings.Contains(joinedArgs, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("expected dangerous bypass for active task workspace, got %#v", record.Args)
	}
	if !samePath(record.Dir, activeWorkspace) {
		t.Fatalf("expected command dir %q, got %q", activeWorkspace, record.Dir)
	}
	if got := envValue(record.Env, "WUPHF_WORKSPACE_PATH"); !samePath(got, activeWorkspace) {
		t.Fatalf("expected workspace env %q, got %#v", activeWorkspace, record.Env)
	}
	if !containsEnv(record.Env, "WUPHF_CHANNEL=launch-ops") {
		t.Fatalf("expected scoped channel env, got %#v", record.Env)
	}
}

func TestHeadlessCodexHomeDirNormalizesRelativeEnv(t *testing.T) {
	wd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(wd); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()

	t.Setenv("CODEX_HOME", ".codex-relative")

	got := headlessCodexHomeDir()
	want := filepath.Join(wd, ".codex-relative")
	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if !samePath(got, want) {
		t.Fatalf("expected absolute CODEX_HOME %q, got %q", want, got)
	}
}

func TestPrepareHeadlessCodexHomeUsesDedicatedRuntimeHomeAndCopiesAuth(t *testing.T) {
	sourceHome := t.TempDir()
	runtimeHome := t.TempDir()
	t.Setenv("HOME", runtimeHome)
	t.Setenv("USERPROFILE", runtimeHome)
	t.Setenv("WUPHF_GLOBAL_HOME", sourceHome)

	sourceCodexHome := filepath.Join(sourceHome, ".codex")
	if err := os.MkdirAll(sourceCodexHome, 0o755); err != nil {
		t.Fatalf("MkdirAll source home: %v", err)
	}
	wantAuth := []byte(`{"access_token":"test-token"}`)
	if err := os.WriteFile(filepath.Join(sourceCodexHome, "auth.json"), wantAuth, 0o600); err != nil {
		t.Fatalf("write source auth: %v", err)
	}
	wantConfig := []byte("model_provider = \"codex-lb\"\n")
	if err := os.WriteFile(filepath.Join(sourceCodexHome, "config.toml"), wantConfig, 0o600); err != nil {
		t.Fatalf("write source config: %v", err)
	}
	oneDir := filepath.Join(sourceHome, ".one")
	if err := os.MkdirAll(oneDir, 0o755); err != nil {
		t.Fatalf("MkdirAll one dir: %v", err)
	}
	wantOneConfig := []byte(`{"session":"one-test"}`)
	if err := os.WriteFile(filepath.Join(oneDir, "config.json"), wantOneConfig, 0o600); err != nil {
		t.Fatalf("write source one config: %v", err)
	}
	wantOneUpdate := []byte(`{"last_check":"2026-04-15T00:00:00Z"}`)
	if err := os.WriteFile(filepath.Join(oneDir, "update-check.json"), wantOneUpdate, 0o600); err != nil {
		t.Fatalf("write source one update check: %v", err)
	}

	got := prepareHeadlessCodexHome()
	want := filepath.Join(runtimeHome, ".wuphf", "codex-headless")
	if !samePath(got, want) {
		t.Fatalf("expected runtime headless home %q, got %q", want, got)
	}
	authCopy, err := os.ReadFile(filepath.Join(want, "auth.json"))
	if err != nil {
		t.Fatalf("read copied auth: %v", err)
	}
	if string(authCopy) != string(wantAuth) {
		t.Fatalf("expected copied auth %q, got %q", string(wantAuth), string(authCopy))
	}
	configCopy, err := os.ReadFile(filepath.Join(want, "config.toml"))
	if err != nil {
		t.Fatalf("read copied config: %v", err)
	}
	if string(configCopy) != string(wantConfig) {
		t.Fatalf("expected copied config %q, got %q", string(wantConfig), string(configCopy))
	}
	oneConfigCopy, err := os.ReadFile(filepath.Join(want, ".one", "config.json"))
	if err != nil {
		t.Fatalf("read copied one config: %v", err)
	}
	if string(oneConfigCopy) != string(wantOneConfig) {
		t.Fatalf("expected copied one config %q, got %q", string(wantOneConfig), string(oneConfigCopy))
	}
	oneUpdateCopy, err := os.ReadFile(filepath.Join(want, ".one", "update-check.json"))
	if err != nil {
		t.Fatalf("read copied one update check: %v", err)
	}
	if string(oneUpdateCopy) != string(wantOneUpdate) {
		t.Fatalf("expected copied one update check %q, got %q", string(wantOneUpdate), string(oneUpdateCopy))
	}
}

func TestPrepareHeadlessCodexHomeRemovesStaleAuthWhenSourceHasNoAuth(t *testing.T) {
	sourceHome := t.TempDir()
	runtimeHome := t.TempDir()
	t.Setenv("HOME", runtimeHome)
	t.Setenv("USERPROFILE", runtimeHome)
	t.Setenv("WUPHF_GLOBAL_HOME", sourceHome)

	sourceCodexHome := filepath.Join(sourceHome, ".codex")
	if err := os.MkdirAll(sourceCodexHome, 0o755); err != nil {
		t.Fatalf("MkdirAll source home: %v", err)
	}
	wantConfig := []byte("model_provider = \"codex-lb\"\n")
	if err := os.WriteFile(filepath.Join(sourceCodexHome, "config.toml"), wantConfig, 0o600); err != nil {
		t.Fatalf("write source config: %v", err)
	}

	runtimeCodexHome := filepath.Join(runtimeHome, ".wuphf", "codex-headless")
	if err := os.MkdirAll(runtimeCodexHome, 0o755); err != nil {
		t.Fatalf("MkdirAll runtime home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runtimeCodexHome, "auth.json"), []byte(`{"access_token":"stale-test-token"}`), 0o600); err != nil {
		t.Fatalf("write stale auth: %v", err)
	}

	got := prepareHeadlessCodexHome()
	if !samePath(got, runtimeCodexHome) {
		t.Fatalf("expected runtime headless home %q, got %q", runtimeCodexHome, got)
	}
	if _, err := os.Stat(filepath.Join(runtimeCodexHome, "auth.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected stale auth.json to be removed, got err=%v", err)
	}
	configCopy, err := os.ReadFile(filepath.Join(runtimeCodexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read copied config: %v", err)
	}
	if string(configCopy) != string(wantConfig) {
		t.Fatalf("expected copied config %q, got %q", string(wantConfig), string(configCopy))
	}
}

func TestSyncHeadlessCodexRuntimeConfigRewritesManagedMCPServers(t *testing.T) {
	sourceHome := t.TempDir()
	runtimeHome := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("HOME", runtimeHome)
	t.Setenv("USERPROFILE", runtimeHome)
	t.Setenv("WUPHF_GLOBAL_HOME", sourceHome)
	t.Setenv("WUPHF_CONFIG_PATH", filepath.Join(configHome, "config.json"))

	customPath := filepath.Join(t.TempDir(), "mcp.json")
	customJSON := `{
  "mcpServers": {
    "playwright": {
      "command": "C:/Windows/System32/WindowsPowerShell/v1.0/powershell.exe",
      "args": ["-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "D:/Repos/dunderia/scripts/launch_playwright_mcp.ps1"],
      "startup_timeout_sec": 90,
      "tools": {
        "browser_navigate": { "approval_mode": "approve" }
      }
    }
  }
}`
	if err := os.WriteFile(customPath, []byte(customJSON), 0o600); err != nil {
		t.Fatalf("write custom mcp json: %v", err)
	}
	if err := config.Save(config.Config{CustomMCPConfig: customPath}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	sourceCodexHome := filepath.Join(sourceHome, ".codex")
	if err := os.MkdirAll(sourceCodexHome, 0o755); err != nil {
		t.Fatalf("MkdirAll source home: %v", err)
	}
	oldConfig := strings.Join([]string{
		`model_provider = "codex-lb"`,
		``,
		`[mcp_servers.playwright]`,
		`command = "C:/Windows/System32/WindowsPowerShell/v1.0/powershell.exe"`,
		`args = ["-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "D:/Repos/.openclaw/scripts/launch_playwright_mcp.ps1"]`,
		`startup_timeout_sec = 90`,
		``,
		`[notice]`,
		`hide_full_access_warning = true`,
		``,
	}, "\n")
	if err := os.WriteFile(filepath.Join(sourceCodexHome, "config.toml"), []byte(oldConfig), 0o600); err != nil {
		t.Fatalf("write source config: %v", err)
	}

	runtimeCodexHome := prepareHeadlessCodexHome()
	l := newHeadlessLauncherForTest()
	if err := l.syncHeadlessCodexRuntimeConfig(runtimeCodexHome, "", "", ""); err != nil {
		t.Fatalf("syncHeadlessCodexRuntimeConfig: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(runtimeCodexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	text := string(got)
	if !strings.Contains(text, `D:/Repos/dunderia/scripts/launch_playwright_mcp.ps1`) {
		t.Fatalf("expected runtime config to include DunderIA playwright launcher, got:\n%s", text)
	}
	if strings.Contains(text, `D:/Repos/.openclaw/scripts/launch_playwright_mcp.ps1`) {
		t.Fatalf("expected runtime config to drop legacy OpenClaw playwright launcher, got:\n%s", text)
	}
	if !strings.Contains(text, `[mcp_servers."playwright".tools."browser_navigate"]`) || !strings.Contains(text, `approval_mode = "approve"`) {
		t.Fatalf("expected runtime config to include playwright tool approval, got:\n%s", text)
	}
	if !strings.Contains(text, `[notice]`) {
		t.Fatalf("expected non-MCP sections to be preserved, got:\n%s", text)
	}
}

func TestEnqueueHeadlessCodexTurnProcessesDistinctTaskTurnsFIFO(t *testing.T) {
	oldRunTurn := headlessCodexRunTurn
	processed := make(chan string, 4)
	headlessCodexRunTurn = func(_ *Launcher, _ context.Context, _ string, notification string, channel ...string) error {
		processed <- notification
		return nil
	}
	defer func() { headlessCodexRunTurn = oldRunTurn }()

	l := newHeadlessLauncherForTest()

	// Distinct task turns should still keep FIFO order; only obsolete queued
	// non-task notifications are coalesced.
	l.enqueueHeadlessCodexTurnRecord("fe", headlessCodexTurn{
		Prompt:     "first task packet",
		TaskID:     "task-1",
		EnqueuedAt: time.Now(),
	})
	l.enqueueHeadlessCodexTurnRecord("fe", headlessCodexTurn{
		Prompt:     "second task packet",
		TaskID:     "task-2",
		EnqueuedAt: time.Now(),
	})

	first := waitForString(t, processed)
	second := waitForString(t, processed)
	if first != "first task packet" || second != "second task packet" {
		t.Fatalf("expected FIFO order, got %q then %q", first, second)
	}
}

func TestSendTaskUpdatePassesTaskChannelToHeadlessTurn(t *testing.T) {
	oldRunTurn := headlessCodexRunTurn
	processed := make(chan processedTurn, 1)
	headlessCodexRunTurn = func(_ *Launcher, _ context.Context, _ string, notification string, channel ...string) error {
		processed <- processedTurn{
			notification: notification,
			channel:      firstNonEmpty(channel...),
		}
		return nil
	}
	defer func() { headlessCodexRunTurn = oldRunTurn }()

	l := newHeadlessLauncherForTest()
	l.provider = "codex"
	l.pack = agent.GetPack("founding-team")

	l.sendTaskUpdate(notificationTarget{Slug: "eng"}, officeActionLog{
		Kind:    "task_updated",
		Actor:   "ceo",
		Channel: "launch-ops",
	}, teamTask{
		ID:      "task-3",
		Channel: "launch-ops",
		Title:   "Build the faceless content factory MVP in-repo",
		Owner:   "eng",
		Status:  "in_progress",
	}, "Continue shipping the owned build.")

	got := waitForProcessedTurn(t, processed)
	if got.channel != "launch-ops" {
		t.Fatalf("expected task update to preserve channel, got %+v", got)
	}
	if !strings.Contains(got.notification, "#launch-ops") {
		t.Fatalf("expected notification to reference launch-ops, got %+v", got)
	}
}

func TestEnqueueHeadlessCodexTurnCancelsStaleTurn(t *testing.T) {
	oldRunTurn := headlessCodexRunTurn
	oldTimeout := headlessCodexTurnTimeout
	oldStale := headlessCodexStaleCancelAfter
	headlessCodexTurnTimeout = 5 * time.Second
	headlessCodexStaleCancelAfter = 20 * time.Millisecond
	defer func() {
		headlessCodexRunTurn = oldRunTurn
		headlessCodexTurnTimeout = oldTimeout
		headlessCodexStaleCancelAfter = oldStale
	}()

	started := make(chan struct{}, 1)
	cancelled := make(chan struct{}, 1)
	processed := make(chan string, 4)
	headlessCodexRunTurn = func(_ *Launcher, ctx context.Context, _ string, notification string, channel ...string) error {
		if notification == "first" {
			select {
			case started <- struct{}{}:
			default:
			}
			<-ctx.Done()
			select {
			case cancelled <- struct{}{}:
			default:
			}
			return ctx.Err()
		}
		processed <- notification
		return nil
	}

	l := newHeadlessLauncherForTest()
	l.enqueueHeadlessCodexTurn("ceo", "first")
	waitForSignal(t, started)
	time.Sleep(35 * time.Millisecond)
	l.enqueueHeadlessCodexTurn("ceo", "second")

	waitForSignal(t, cancelled)
	if got := waitForString(t, processed); got != "second" {
		t.Fatalf("expected queued turn to run after cancellation, got %q", got)
	}
}

func TestHeadlessCodexHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HEADLESS_CODEX_HELPER_PROCESS") != "1" {
		return
	}

	args := os.Args
	doubleDash := 0
	for i, arg := range args {
		if arg == "--" {
			doubleDash = i
			break
		}
	}
	codexArgs := append([]string(nil), args[doubleDash+1:]...)
	stdin, _ := io.ReadAll(os.Stdin)

	record := headlessCodexRecord{
		Name:  "codex",
		Args:  codexArgs,
		Dir:   mustGetwd(t),
		Env:   os.Environ(),
		Stdin: string(stdin),
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal helper record: %v", err)
	}
	recordPath := os.Getenv("HEADLESS_CODEX_RECORD_FILE")
	if err := os.WriteFile(recordPath, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write helper record: %v", err)
	}

	if !containsArg(codexArgs, "--json") {
		t.Fatalf("missing --json arg: %#v", codexArgs)
	}
	_, _ = os.Stdout.WriteString("{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"codex office reply\"}}\n")
	_, _ = os.Stdout.WriteString("{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":123,\"cached_input_tokens\":45,\"output_tokens\":6}}\n")
	os.Exit(0)
}

func TestHeadlessCodexNeedsWebsocketHeaderWorkaround(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("WUPHF_GLOBAL_HOME", home)

	asciiWorkspace := filepath.Join(t.TempDir(), "Repos", "LegacySystemNew")
	if got := headlessCodexNeedsWebsocketHeaderWorkaround(asciiWorkspace); got {
		t.Fatalf("expected ASCII workspace path to skip workaround, got %t", got)
	}

	nonASCIIWorkspace := filepath.Join(t.TempDir(), "Repositórios", "LegacySystemNew")
	if got := headlessCodexNeedsWebsocketHeaderWorkaround(nonASCIIWorkspace); !got {
		t.Fatalf("expected non-ASCII workspace path to require workaround, got %t", got)
	}
}

func TestHeadlessClaudeCompatibleHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HEADLESS_CLAUDE_COMPAT_HELPER_PROCESS") != "1" {
		return
	}

	args := os.Args
	doubleDash := 0
	for i, arg := range args {
		if arg == "--" {
			doubleDash = i
			break
		}
	}
	compatArgs := append([]string(nil), args[doubleDash+1:]...)
	if len(compatArgs) == 0 {
		t.Fatal("missing command name")
	}
	commandName := compatArgs[0]
	compatArgs = compatArgs[1:]
	stdin, _ := io.ReadAll(os.Stdin)

	record := headlessCodexRecord{
		Name:  commandName,
		Args:  compatArgs,
		Dir:   mustGetwd(t),
		Env:   os.Environ(),
		Stdin: string(stdin),
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal helper record: %v", err)
	}
	recordPath := os.Getenv("HEADLESS_CODEX_RECORD_FILE")
	if err := os.WriteFile(recordPath, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write helper record: %v", err)
	}

	_, _ = os.Stdout.WriteString("{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"openclaude office reply\"}]}}\n")
	_, _ = os.Stdout.WriteString("{\"type\":\"result\",\"result\":\"openclaude office reply\",\"usage\":{\"input_tokens\":22,\"output_tokens\":7,\"cache_creation_input_tokens\":0,\"cache_read_input_tokens\":0}}\n")
	os.Exit(0)
}

func readHeadlessCodexRecord(t *testing.T, path string) headlessCodexRecord {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read record file: %v", err)
	}
	var record headlessCodexRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatalf("unmarshal record: %v", err)
	}
	return record
}

func testRunHeadlessOpenClaudeTurnUsesProvider(t *testing.T, runtimeKind string, wantCLIProvider string) {
	t.Helper()

	recordFile := filepath.Join(t.TempDir(), "headless-openclaude-record.jsonl")
	openClaudeDir := t.TempDir()
	openClaudeCmd := filepath.Join(openClaudeDir, "openclaude.cmd")
	openClaudePs1 := filepath.Join(openClaudeDir, "openclaude.ps1")
	oldLookPath := headlessClaudeLookPath
	oldCommandContext := headlessClaudeCommandContext
	defer func() {
		headlessClaudeLookPath = oldLookPath
		headlessClaudeCommandContext = oldCommandContext
	}()

	if err := os.WriteFile(openClaudeCmd, []byte("@echo off\r\n"), 0o644); err != nil {
		t.Fatalf("write openclaude.cmd: %v", err)
	}
	if err := os.WriteFile(openClaudePs1, []byte("Write-Output 'noop'\n"), 0o644); err != nil {
		t.Fatalf("write openclaude.ps1: %v", err)
	}

	headlessClaudeLookPath = func(file string) (string, error) {
		if file == "openclaude" {
			return openClaudeCmd, nil
		}
		if file == "nex-mcp" {
			return "/usr/bin/nex-mcp", nil
		}
		return "", exec.ErrNotFound
	}
	headlessClaudeCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmdArgs := []string{"-test.run=TestHeadlessClaudeCompatibleHelperProcess", "--", name}
		cmdArgs = append(cmdArgs, args...)
		return exec.CommandContext(ctx, os.Args[0], cmdArgs...)
	}

	t.Setenv("GO_WANT_HEADLESS_CLAUDE_COMPAT_HELPER_PROCESS", "1")
	t.Setenv("HEADLESS_CODEX_RECORD_FILE", recordFile)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("WUPHF_API_KEY", "nex-secret-key")

	l := &Launcher{
		pack:        agent.GetPack("founding-team"),
		cwd:         t.TempDir(),
		broker:      NewBroker(),
		headlessCtx: context.Background(),
	}

	if err := l.runHeadlessOpenClaudeTurn(context.Background(), "ceo", runtimeKind, "You have new work in #launch."); err != nil {
		t.Fatalf("runHeadlessOpenClaudeTurn: %v", err)
	}

	record := readHeadlessCodexRecord(t, recordFile)
	if runtime.GOOS == "windows" {
		if record.Name != "pwsh" {
			t.Fatalf("expected pwsh wrapper, got %q", record.Name)
		}
		if len(record.Args) < 2 || record.Args[0] != "-File" || !samePath(record.Args[1], openClaudePs1) {
			t.Fatalf("expected pwsh -File %q wrapper args, got %#v", openClaudePs1, record.Args)
		}
	} else if record.Name != "openclaude" {
		t.Fatalf("expected direct openclaude binary, got %q", record.Name)
	}
	joinedArgs := strings.Join(record.Args, " ")
	if !strings.Contains(joinedArgs, "--provider "+wantCLIProvider) {
		t.Fatalf("expected openclaude provider %q, got %#v", wantCLIProvider, record.Args)
	}
	if !strings.Contains(joinedArgs, "--output-format stream-json") {
		t.Fatalf("expected stream-json output format, got %#v", record.Args)
	}
	if !strings.Contains(joinedArgs, "--append-system-prompt") {
		t.Fatalf("expected append-system-prompt, got %#v", record.Args)
	}
	if !strings.Contains(joinedArgs, "--mcp-config") || !strings.Contains(joinedArgs, "--strict-mcp-config") {
		t.Fatalf("expected MCP config args, got %#v", record.Args)
	}
	if !containsEnv(record.Env, "WUPHF_HEADLESS_PROVIDER="+runtimeKind) {
		t.Fatalf("expected runtime env %q, got %#v", runtimeKind, record.Env)
	}
	if !strings.Contains(record.Stdin, "You have new work in #launch.") {
		t.Fatalf("expected notification in stdin, got %q", record.Stdin)
	}
}

func TestRunHeadlessCodexTurnUsesTaskRuntimeOverrides(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	recordFile := filepath.Join(t.TempDir(), "headless-codex-record.jsonl")
	oldLookPath := headlessCodexLookPath
	oldExecutablePath := headlessCodexExecutablePath
	oldCommandContext := headlessCodexCommandContext
	headlessCodexLookPath = func(file string) (string, error) {
		if file == "codex" {
			return "/usr/bin/codex", nil
		}
		return "", exec.ErrNotFound
	}
	headlessCodexExecutablePath = func() (string, error) { return "/tmp/wuphf", nil }
	headlessCodexCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmdArgs := []string{"-test.run=TestHeadlessCodexHelperProcess", "--"}
		cmdArgs = append(cmdArgs, args...)
		return exec.CommandContext(ctx, os.Args[0], cmdArgs...)
	}
	defer func() {
		headlessCodexLookPath = oldLookPath
		headlessCodexExecutablePath = oldExecutablePath
		headlessCodexCommandContext = oldCommandContext
	}()

	t.Setenv("GO_WANT_HEADLESS_CODEX_HELPER_PROCESS", "1")
	t.Setenv("HEADLESS_CODEX_RECORD_FILE", recordFile)
	t.Setenv("HOME", t.TempDir())

	broker := NewBroker()
	broker.mu.Lock()
	broker.tasks = append(broker.tasks, teamTask{
		ID:              "task-runtime",
		Channel:         "general",
		Title:           "Hard task",
		Owner:           "eng",
		Status:          "in_progress",
		RuntimeProvider: "codex",
		RuntimeModel:    "gpt-5.5",
		ReasoningEffort: "high",
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:       time.Now().UTC().Format(time.RFC3339),
	})
	broker.mu.Unlock()

	l := &Launcher{
		pack:        agent.GetPack("founding-team"),
		cwd:         t.TempDir(),
		broker:      broker,
		headlessCtx: context.Background(),
	}

	if err := l.runHeadlessCodexTurn(context.Background(), "eng", "You have a difficult task.", "general"); err != nil {
		t.Fatalf("runHeadlessCodexTurn: %v", err)
	}

	record := readHeadlessCodexRecord(t, recordFile)
	if got := argValue(record.Args, "--model"); got != "gpt-5.5" {
		t.Fatalf("--model = %q, want gpt-5.5; args=%#v", got, record.Args)
	}
	joinedArgs := strings.Join(record.Args, " ")
	if !strings.Contains(joinedArgs, `model_reasoning_effort="high"`) {
		t.Fatalf("expected task reasoning effort override, got %#v", record.Args)
	}
}

func TestHeadlessTurnProviderKindInfersCodexFromTaskModel(t *testing.T) {
	broker := NewBroker()
	broker.mu.Lock()
	broker.tasks = append(broker.tasks, teamTask{
		ID:           "task-runtime",
		Channel:      "general",
		Title:        "Hard task",
		Owner:        "eng",
		Status:       "in_progress",
		RuntimeModel: "gpt-5.5",
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
	})
	broker.mu.Unlock()

	l := &Launcher{
		provider: provider.KindClaudeCode,
		broker:   broker,
	}

	if got := l.headlessTurnProviderKind("eng", "general"); got != provider.KindCodex {
		t.Fatalf("provider kind = %q, want %q", got, provider.KindCodex)
	}
}

func containsEnv(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsEnvPrefix(values []string, prefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func envValue(values []string, key string) string {
	prefix := strings.TrimSpace(key) + "="
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimPrefix(value, prefix)
		}
	}
	return ""
}

func containsArg(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func argValue(values []string, key string) string {
	for i := 0; i < len(values)-1; i++ {
		if values[i] == key {
			return values[i+1]
		}
	}
	return ""
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return wd
}

func samePath(a, b string) bool {
	return canonicalPath(a) == canonicalPath(b)
}

func canonicalPath(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return ""
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return filepath.Clean(resolved)
	}
	return path
}

func isolateBrokerStateForTest(t *testing.T) {
	t.Helper()

	homeDir := t.TempDir()
	oldPathFn := brokerStatePath
	brokerStatePath = func() string {
		return filepath.Join(homeDir, "broker-state.json")
	}
	t.Setenv("HOME", homeDir)
	t.Cleanup(func() {
		brokerStatePath = oldPathFn
	})
}

func useStubTaskWorktreeProvisioning(t *testing.T) {
	t.Helper()

	oldPrepare := prepareTaskWorktree
	oldCleanup := cleanupTaskWorktree
	seq := 0

	prepareTaskWorktree = func(taskID string) (string, string, error) {
		seq++
		path := filepath.Join(t.TempDir(), fmt.Sprintf("task-worktree-%02d", seq))
		if err := os.MkdirAll(path, 0o755); err != nil {
			return "", "", err
		}
		return path, fmt.Sprintf("wuphf-test-%s-%02d", sanitizeWorktreeToken(taskID), seq), nil
	}
	cleanupTaskWorktree = func(path, branch string) error {
		path = strings.TrimSpace(path)
		if path == "" {
			return nil
		}
		if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	t.Cleanup(func() {
		prepareTaskWorktree = oldPrepare
		cleanupTaskWorktree = oldCleanup
	})
}

func newHeadlessLauncherForTest() *Launcher {
	return &Launcher{
		headlessCtx:          context.Background(),
		headlessWorkers:      make(map[string]bool),
		headlessActive:       make(map[string]*headlessCodexActiveTurn),
		headlessQueues:       make(map[string][]headlessCodexTurn),
		headlessQuickWorkers: make(map[string]bool),
		headlessQuickActive:  make(map[string]*headlessCodexActiveTurn),
		headlessQuickQueues:  make(map[string][]headlessCodexTurn),
		pack:                 &agent.PackDefinition{LeadSlug: "ceo"}, // deterministic lead; avoids reading global broker state
	}
}

func TestFinishHeadlessTurnWakesLeadWhenAllSpecialistsDone(t *testing.T) {
	woken := make(chan string, 4)
	oldWakeLead := headlessWakeLeadFn
	headlessWakeLeadFn = func(_ *Launcher, specialistSlug string) {
		woken <- specialistSlug
	}
	defer func() { headlessWakeLeadFn = oldWakeLead }()

	l := newHeadlessLauncherForTest()

	// Simulate "fe" finishing with no other specialists active.
	l.finishHeadlessTurn("fe", false)

	got := waitForString(t, woken)
	if got != "fe" {
		t.Fatalf("expected lead woken after fe finished, got %q", got)
	}
}

func TestFinishHeadlessTurnDoesNotWakeLeadWhenOtherSpecialistsActive(t *testing.T) {
	woken := make(chan string, 4)
	oldWakeLead := headlessWakeLeadFn
	headlessWakeLeadFn = func(_ *Launcher, specialistSlug string) {
		woken <- specialistSlug
	}
	defer func() { headlessWakeLeadFn = oldWakeLead }()

	l := newHeadlessLauncherForTest()
	// "be" is still active while "fe" finishes.
	l.headlessActive["be"] = &headlessCodexActiveTurn{}

	l.finishHeadlessTurn("fe", false)

	select {
	case got := <-woken:
		t.Fatalf("expected NO lead wake when other specialist still active, but got %q", got)
	case <-time.After(100 * time.Millisecond):
		// correct: lead not woken
	}
}

func TestFinishHeadlessTurnDoesNotWakeLeadWhenLeadFinishes(t *testing.T) {
	woken := make(chan string, 4)
	oldWakeLead := headlessWakeLeadFn
	headlessWakeLeadFn = func(_ *Launcher, specialistSlug string) {
		woken <- specialistSlug
	}
	defer func() { headlessWakeLeadFn = oldWakeLead }()

	l := newHeadlessLauncherForTest()
	// CEO finishes — should not self-wake.
	l.finishHeadlessTurn("ceo", false)

	select {
	case got := <-woken:
		t.Fatalf("expected NO lead wake when lead itself finishes, got %q", got)
	case <-time.After(100 * time.Millisecond):
		// correct: lead not self-woken
	}
}

func TestFinishHeadlessTurnDoesNotWakeLeadWhenLeadAlreadyQueued(t *testing.T) {
	woken := make(chan string, 4)
	oldWakeLead := headlessWakeLeadFn
	headlessWakeLeadFn = func(_ *Launcher, specialistSlug string) {
		woken <- specialistSlug
	}
	defer func() { headlessWakeLeadFn = oldWakeLead }()

	l := newHeadlessLauncherForTest()
	// CEO already has a pending turn.
	l.headlessQueues["ceo"] = []headlessCodexTurn{{Prompt: "pending work"}}

	l.finishHeadlessTurn("fe", false)

	select {
	case got := <-woken:
		t.Fatalf("expected NO lead wake when lead already has queued work, got %q", got)
	case <-time.After(100 * time.Millisecond):
		// correct: lead not woken again
	}
}

func TestEnqueueHeadlessCodexTurnRecordDropsDuplicateLeadTaskWhileActive(t *testing.T) {
	l := newHeadlessLauncherForTest()
	l.pack = &agent.PackDefinition{LeadSlug: "ceo"}
	l.headlessActive["ceo"] = &headlessCodexActiveTurn{
		Turn: headlessCodexTurn{
			Prompt: "first prompt about #task-3",
			TaskID: "task-3",
		},
		StartedAt: time.Now(),
	}

	l.enqueueHeadlessCodexTurnRecord("ceo", headlessCodexTurn{
		Prompt:     "second prompt about #task-3",
		TaskID:     "task-3",
		EnqueuedAt: time.Now(),
	})

	if got := len(l.headlessQueues["ceo"]); got != 0 {
		t.Fatalf("expected no queued duplicate lead turn for same task, got %d", got)
	}
}

func TestEnqueueHeadlessCodexTurnRecordQueuesUrgentLeadWakeForSameTask(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Review and advance the proof lane",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "follow_up",
		ExecutionMode: "office",
		ReviewState:   "ready_for_review",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	for i := range b.tasks {
		if b.tasks[i].ID == task.ID {
			b.tasks[i].Status = "review"
			b.tasks[i].ReviewState = "ready_for_review"
			break
		}
	}

	cancelled := false
	l := newHeadlessLauncherForTest()
	l.pack = &agent.PackDefinition{LeadSlug: "ceo"}
	l.broker = b
	l.headlessWorkers["ceo"] = true
	l.headlessActive["ceo"] = &headlessCodexActiveTurn{
		Turn: headlessCodexTurn{
			Prompt: "first prompt about #" + task.ID,
			TaskID: task.ID,
		},
		StartedAt: time.Now().Add(-2 * time.Minute),
		Cancel: func() {
			cancelled = true
		},
	}

	l.enqueueHeadlessCodexTurnRecord("ceo", headlessCodexTurn{
		Prompt:     "specialist handoff about #" + task.ID,
		TaskID:     task.ID,
		EnqueuedAt: time.Now(),
	})

	if got := len(l.headlessQueues["ceo"]); got != 1 {
		t.Fatalf("expected urgent lead wake to queue behind same task, got %d", got)
	}
	if !cancelled {
		t.Fatal("expected stale active lead turn to be cancelled for urgent same-task wake")
	}
}

func TestEnqueueHeadlessCodexTurnRecordDropsDuplicateAgentTaskWhileActive(t *testing.T) {
	l := newHeadlessLauncherForTest()
	l.pack = &agent.PackDefinition{LeadSlug: "ceo"}
	l.headlessActive["eng"] = &headlessCodexActiveTurn{
		Turn: headlessCodexTurn{
			Prompt: "first prompt about #task-11",
			TaskID: "task-11",
		},
		StartedAt: time.Now(),
	}

	l.enqueueHeadlessCodexTurnRecord("eng", headlessCodexTurn{
		Prompt:     "second prompt about #task-11",
		TaskID:     "task-11",
		EnqueuedAt: time.Now(),
	})

	if got := len(l.headlessQueues["eng"]); got != 0 {
		t.Fatalf("expected no queued duplicate agent turn for same task, got %d", got)
	}
}

func TestEnqueueHeadlessCodexTurnRecordReplacesPendingAgentTaskTurn(t *testing.T) {
	l := newHeadlessLauncherForTest()
	l.pack = &agent.PackDefinition{LeadSlug: "ceo"}
	laneKey := agentLaneKey("launch-ops", "eng")
	l.headlessWorkers[laneKey] = true
	l.headlessQueues[laneKey] = []headlessCodexTurn{{
		Prompt:     "older prompt about #task-11",
		Channel:    "launch-ops",
		TaskID:     "task-11",
		EnqueuedAt: time.Now().Add(-time.Minute),
	}}

	l.enqueueHeadlessCodexTurnRecord("eng", headlessCodexTurn{
		Prompt:     "newer prompt about #task-11",
		Channel:    "launch-ops",
		TaskID:     "task-11",
		EnqueuedAt: time.Now(),
	})

	queue := l.headlessQueues[laneKey]
	if got := len(queue); got != 1 {
		t.Fatalf("expected single queued agent turn for same task, got %d", got)
	}
	if got := queue[0].Prompt; got != "newer prompt about #task-11" {
		t.Fatalf("expected queued agent turn to be replaced, got %q", got)
	}
}

func TestEnqueueHeadlessCodexTurnRecordAllowsRetryBehindActiveAgentTask(t *testing.T) {
	l := newHeadlessLauncherForTest()
	l.pack = &agent.PackDefinition{LeadSlug: "ceo"}
	laneKey := agentLaneKey("launch-ops", "eng")
	l.headlessWorkers[laneKey] = true
	l.headlessActive[laneKey] = &headlessCodexActiveTurn{
		Turn: headlessCodexTurn{
			Prompt:   "first prompt about #task-11",
			Channel:  "launch-ops",
			TaskID:   "task-11",
			Attempts: 0,
		},
		StartedAt: time.Now(),
	}

	l.enqueueHeadlessCodexTurnRecord("eng", headlessCodexTurn{
		Prompt:     "retry prompt about #task-11",
		Channel:    "launch-ops",
		TaskID:     "task-11",
		Attempts:   1,
		EnqueuedAt: time.Now(),
	})

	queue := l.headlessQueues[laneKey]
	if got := len(queue); got != 1 {
		t.Fatalf("expected single queued retry turn for same task, got %d", got)
	}
	if got := queue[0].Prompt; got != "retry prompt about #task-11" {
		t.Fatalf("expected retry turn to be queued, got %q", got)
	}
	if got := queue[0].Attempts; got != 1 {
		t.Fatalf("expected retry attempt to be preserved, got %d", got)
	}
}

func TestEnqueueHeadlessCodexTurnRecordCoalescesQueuedNonTaskTurnsPerLane(t *testing.T) {
	l := newHeadlessLauncherForTest()
	l.pack = &agent.PackDefinition{LeadSlug: "ceo"}
	laneKey := agentLaneKey("launch-ops", "eng")
	l.headlessWorkers[laneKey] = true
	l.headlessQueues[laneKey] = []headlessCodexTurn{
		{
			Prompt:     "older status ping",
			Channel:    "launch-ops",
			EnqueuedAt: time.Now().Add(-2 * time.Minute),
		},
		{
			Prompt:     "older follow-up note",
			Channel:    "launch-ops",
			EnqueuedAt: time.Now().Add(-time.Minute),
		},
	}

	l.enqueueHeadlessCodexTurnRecord("eng", headlessCodexTurn{
		Prompt:     "latest lane note",
		Channel:    "launch-ops",
		EnqueuedAt: time.Now(),
	})

	queue := l.headlessQueues[laneKey]
	if got := len(queue); got != 1 {
		t.Fatalf("expected obsolete queued non-task turns to collapse to one latest turn, got %d", got)
	}
	if got := queue[0].Prompt; got != "latest lane note" {
		t.Fatalf("expected latest queued non-task turn to win, got %q", got)
	}
}

func TestEnqueueHeadlessCodexTurnRecordKeepsTaskTurnWhileCoalescingNonTaskTurns(t *testing.T) {
	l := newHeadlessLauncherForTest()
	l.pack = &agent.PackDefinition{LeadSlug: "ceo"}
	laneKey := agentLaneKey("launch-ops", "eng")
	l.headlessWorkers[laneKey] = true
	l.headlessQueues[laneKey] = []headlessCodexTurn{
		{
			Prompt:     "queued task work",
			Channel:    "launch-ops",
			TaskID:     "task-11",
			EnqueuedAt: time.Now().Add(-2 * time.Minute),
		},
		{
			Prompt:     "older lane status ping",
			Channel:    "launch-ops",
			EnqueuedAt: time.Now().Add(-time.Minute),
		},
	}

	l.enqueueHeadlessCodexTurnRecord("eng", headlessCodexTurn{
		Prompt:     "latest lane status ping",
		Channel:    "launch-ops",
		EnqueuedAt: time.Now(),
	})

	queue := l.headlessQueues[laneKey]
	if got := len(queue); got != 2 {
		t.Fatalf("expected task turn to remain while non-task turns coalesce, got %d", got)
	}
	if got := queue[0].TaskID; got != "task-11" {
		t.Fatalf("expected task turn to remain first in queue, got %+v", queue)
	}
	if got := queue[1].Prompt; got != "latest lane status ping" {
		t.Fatalf("expected latest queued non-task turn to replace older one, got %+v", queue)
	}
}

func TestWakeLeadAfterSpecialistFallsBackToCompletedTaskUpdateWhenNoBroadcast(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	oldRunTurn := headlessCodexRunTurn
	notifications := make(chan string, 1)
	headlessCodexRunTurn = func(_ *Launcher, _ context.Context, slug, notification string, channel ...string) error {
		if slug == "ceo" {
			notifications <- notification
		}
		return nil
	}
	defer func() { headlessCodexRunTurn = oldRunTurn }()

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	ensureTestMemberAccess(b, "general", "operator", "Operator")
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Lock the faceless content niche",
		Owner:         "gtm",
		CreatedBy:     "ceo",
		TaskType:      "launch",
		ExecutionMode: "office",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	b.mu.Lock()
	for i := range b.tasks {
		if b.tasks[i].ID != task.ID {
			continue
		}
		b.tasks[i].Status = "done"
		b.tasks[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		b.appendActionLocked("task_updated", "office", "general", "gtm", truncateSummary(b.tasks[i].Title+" ["+b.tasks[i].Status+"]", 140), task.ID)
		break
	}
	b.mu.Unlock()

	l := newHeadlessLauncherForTest()
	l.broker = b
	l.provider = "codex"
	l.sessionName = "test"

	l.wakeLeadAfterSpecialist("gtm")

	got := waitForString(t, notifications)
	if !strings.Contains(got, "[Task updated #"+task.ID+" on #general]") {
		t.Fatalf("expected CEO notification for completed task handoff, got %q", got)
	}
	if !strings.Contains(got, "status done") {
		t.Fatalf("expected completed task status in CEO notification, got %q", got)
	}
}

func TestWakeLeadAfterSpecialistDoesNotRequeueLeadForDirectMessage(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	b := NewBroker()
	root, err := b.PostMessage("human", "estagiario__human", "Me ajuda a analisar esse legado.", nil, "")
	if err != nil {
		t.Fatalf("post human dm: %v", err)
	}
	if _, err := b.PostMessage("estagiario", "estagiario__human", "Claro. Vou olhar o sistema legado agora.", nil, root.ID); err != nil {
		t.Fatalf("post specialist dm reply: %v", err)
	}

	l := newHeadlessLauncherForTest()
	l.broker = b
	l.provider = "codex"
	l.sessionName = "test"

	l.wakeLeadAfterSpecialist("estagiario")

	if got := len(l.headlessQueues["ceo"]); got != 0 {
		t.Fatalf("expected no lead requeue for specialist DM reply, got %d queued turns", got)
	}
	if got := len(l.headlessQuickQueues["ceo"]); got != 0 {
		t.Fatalf("expected no lead quick-reply wake for specialist DM reply, got %d queued turns", got)
	}
}

func TestEnqueueHeadlessCodexTurnScopesLeadDeferralToSameChannel(t *testing.T) {
	l := newHeadlessLauncherForTest()
	l.broker = NewBroker()
	l.headlessActive[agentLaneKey("general", "eng")] = &headlessCodexActiveTurn{}
	l.headlessWorkers[agentLaneKey("engineering", "ceo")] = true

	l.enqueueHeadlessCodexTurnRecord("ceo", headlessCodexTurn{
		Prompt:     "pending task handoff about #task-44",
		Channel:    "engineering",
		TaskID:     "task-44",
		EnqueuedAt: time.Now(),
	})

	laneKey := agentLaneKey("engineering", "ceo")
	if got := len(l.headlessQueues[laneKey]); got != 1 {
		t.Fatalf("expected lead work to queue immediately on the engineering lane, got %d pending turns", got)
	}
	if l.headlessDeferredLead[laneKey] != nil {
		t.Fatal("expected no deferred lead turn for a specialist active in another channel")
	}
}

func TestRecoverTimedOutHeadlessTurnBlocksTaskWithoutSubstantiveReply(t *testing.T) {
	isolateBrokerStateForTest(t)

	b := NewBroker()
	b.mu.Lock()
	b.members = append(b.members, officeMember{Slug: "cmo", Name: "Chief Marketing Officer"})
	for i := range b.channels {
		if b.channels[i].Slug == "general" {
			b.channels[i].Members = append(b.channels[i].Members, "cmo")
			break
		}
	}
	b.mu.Unlock()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Research the best faceless wedge",
		Owner:         "cmo",
		CreatedBy:     "ceo",
		TaskType:      "research",
		ExecutionMode: "office",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	if _, err := b.PostMessage("cmo", "general", "[STATUS] still researching", nil, task.ThreadID); err != nil {
		t.Fatalf("post status: %v", err)
	}

	l := newHeadlessLauncherForTest()
	l.broker = b
	l.recoverTimedOutHeadlessTurn("cmo", headlessCodexTurn{
		TaskID:   task.ID,
		Attempts: headlessCodexOfficeRetryLimit,
	}, time.Now().UTC().Add(-2*time.Second), headlessCodexTurnTimeout)

	var updated teamTask
	for _, candidate := range b.AllTasks() {
		if candidate.ID == task.ID {
			updated = candidate
			break
		}
	}
	if updated.ID == "" {
		t.Fatalf("expected to find task %s", task.ID)
	}
	if updated.Status != "blocked" || !updated.Blocked {
		t.Fatalf("expected task to be blocked after retry budget exhausted with only status chatter, got %+v", updated)
	}
	if !strings.Contains(updated.Details, "timed out") {
		t.Fatalf("expected timeout detail appended, got %+v", updated)
	}
}

func TestRecoverTimedOutHeadlessTurnLeavesTaskRunningAfterSubstantiveReply(t *testing.T) {
	isolateBrokerStateForTest(t)

	b := NewBroker()
	b.mu.Lock()
	b.members = append(b.members, officeMember{Slug: "cmo", Name: "Chief Marketing Officer"})
	for i := range b.channels {
		if b.channels[i].Slug == "general" {
			b.channels[i].Members = append(b.channels[i].Members, "cmo")
			break
		}
	}
	b.mu.Unlock()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Research the best faceless wedge",
		Owner:         "cmo",
		CreatedBy:     "ceo",
		TaskType:      "research",
		ExecutionMode: "office",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	startedAt := time.Now().UTC().Add(-2 * time.Second)
	if _, err := b.PostMessage("cmo", "general", "Best wedge is a high-volume historical facts channel with sponsor ladder.", nil, task.ThreadID); err != nil {
		t.Fatalf("post substantive message: %v", err)
	}

	l := newHeadlessLauncherForTest()
	l.broker = b
	l.recoverTimedOutHeadlessTurn("cmo", headlessCodexTurn{TaskID: task.ID}, startedAt, headlessCodexTurnTimeout)

	var updated teamTask
	for _, candidate := range b.AllTasks() {
		if candidate.ID == task.ID {
			updated = candidate
			break
		}
	}
	if updated.ID == "" {
		t.Fatalf("expected to find task %s", task.ID)
	}
	if updated.Status != "in_progress" || updated.Blocked {
		t.Fatalf("expected task to remain active after substantive reply, got %+v", updated)
	}
}

func TestRecoverTimedOutHeadlessTurnLeavesTaskRunningAfterTaskProgressUpdate(t *testing.T) {
	isolateBrokerStateForTest(t)

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	workspace := t.TempDir()
	initUsableGitWorktree(t, workspace)
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Backport the legacy upload hardening",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "bugfix",
		ExecutionMode: "external_workspace",
		WorkspacePath: workspace,
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	startedAt := time.Now().UTC().Add(-2 * time.Second)
	b.mu.Lock()
	for i := range b.tasks {
		if b.tasks[i].ID != task.ID {
			continue
		}
		b.tasks[i].Status = "open"
		b.tasks[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		b.appendActionLocked("task_updated", "office", b.tasks[i].Channel, "builder", truncateSummary(b.tasks[i].Title+" ["+b.tasks[i].Status+"]", 140), b.tasks[i].ID)
		break
	}
	b.mu.Unlock()

	l := newHeadlessLauncherForTest()
	l.broker = b
	l.recoverTimedOutHeadlessTurn("builder", headlessCodexTurn{TaskID: task.ID}, startedAt, headlessCodexTurnTimeout)

	var updated teamTask
	for _, candidate := range b.AllTasks() {
		if candidate.ID == task.ID {
			updated = candidate
			break
		}
	}
	if updated.ID == "" {
		t.Fatalf("expected to find task %s", task.ID)
	}
	if updated.Blocked {
		t.Fatalf("expected task progress update to avoid timeout reblock, got %+v", updated)
	}
}

func TestRecoverTimedOutHeadlessTurnRetriesLocalWorktreeOnceBeforeBlocking(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	useStubTaskWorktreeProvisioning(t)

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	ensureTestMemberAccess(b, "general", "operator", "Operator")
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Implement the studio build",
		Owner:         "eng",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	l := newHeadlessLauncherForTest()
	l.broker = b
	l.headlessWorkers["eng"] = true

	turn := headlessCodexTurn{
		Prompt:   "Build #task-" + strings.TrimPrefix(task.ID, "task-"),
		Channel:  "general",
		TaskID:   task.ID,
		Attempts: 0,
	}
	l.recoverTimedOutHeadlessTurn("eng", turn, time.Now().UTC().Add(-2*time.Second), headlessCodexLocalWorktreeTurnTimeout)

	if len(l.headlessQueues["eng"]) != 1 {
		t.Fatalf("expected one queued retry, got %+v", l.headlessQueues["eng"])
	}
	retry := l.headlessQueues["eng"][0]
	if retry.Attempts != 1 {
		t.Fatalf("expected retry attempt 1, got %+v", retry)
	}
	if !strings.Contains(retry.Prompt, "Previous attempt by @eng timed out") {
		t.Fatalf("expected retry prompt note, got %q", retry.Prompt)
	}

	var updated teamTask
	for _, candidate := range b.AllTasks() {
		if candidate.ID == task.ID {
			updated = candidate
			break
		}
	}
	if updated.ID == "" {
		t.Fatalf("expected to find task %s", task.ID)
	}
	if updated.Status != "in_progress" || updated.Blocked {
		t.Fatalf("expected task to remain active during retry, got %+v", updated)
	}
}

func TestRecoverFailedHeadlessTurnRetriesLocalWorktreeOnceBeforeBlocking(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	useStubTaskWorktreeProvisioning(t)

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Implement queue mode for the content factory",
		Owner:         "eng",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	l := newHeadlessLauncherForTest()
	l.broker = b
	l.headlessWorkers["eng"] = true

	turn := headlessCodexTurn{
		Prompt:   "Build #task-" + strings.TrimPrefix(task.ID, "task-"),
		Channel:  "general",
		TaskID:   task.ID,
		Attempts: 0,
	}
	l.recoverFailedHeadlessTurn("eng", turn, time.Now().UTC().Add(-2*time.Second), "failed to write patch to assigned workspace")

	if len(l.headlessQueues["eng"]) != 1 {
		t.Fatalf("expected one queued retry, got %+v", l.headlessQueues["eng"])
	}
	retry := l.headlessQueues["eng"][0]
	if retry.Attempts != 1 {
		t.Fatalf("expected retry attempt 1, got %+v", retry)
	}
	if !strings.Contains(retry.Prompt, "Previous attempt by @eng failed") {
		t.Fatalf("expected retry prompt note, got %q", retry.Prompt)
	}
	if !strings.Contains(retry.Prompt, "failed to write patch to assigned workspace") {
		t.Fatalf("expected retry prompt to carry failure detail, got %q", retry.Prompt)
	}

	var updated teamTask
	for _, candidate := range b.AllTasks() {
		if candidate.ID == task.ID {
			updated = candidate
			break
		}
	}
	if updated.ID == "" {
		t.Fatalf("expected to find task %s", task.ID)
	}
	if updated.Status != "in_progress" || updated.Blocked {
		t.Fatalf("expected task to remain active during retry, got %+v", updated)
	}
}

func TestRecoverTimedOutHeadlessTurnRetriesExternalWorkspaceBeforeBlocking(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	b := NewBroker()
	workspace := t.TempDir()
	initUsableGitWorktree(t, workspace)
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Implement the external workspace backend slice",
		Owner:         "backend",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "external_workspace",
		WorkspacePath: workspace,
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	l := newHeadlessLauncherForTest()
	l.broker = b
	l.headlessWorkers["backend"] = true

	turn := headlessCodexTurn{
		Prompt:   "Ship #task-" + strings.TrimPrefix(task.ID, "task-"),
		Channel:  "general",
		TaskID:   task.ID,
		Attempts: 0,
	}
	l.recoverTimedOutHeadlessTurn("backend", turn, time.Now().UTC().Add(-2*time.Second), headlessCodexTurnTimeout)

	if len(l.headlessQueues["backend"]) != 1 {
		t.Fatalf("expected one queued retry, got %+v", l.headlessQueues["backend"])
	}
	if l.headlessQueues["backend"][0].Attempts != 1 {
		t.Fatalf("expected retry attempt 1, got %+v", l.headlessQueues["backend"][0])
	}

	var updated teamTask
	for _, candidate := range b.AllTasks() {
		if candidate.ID == task.ID {
			updated = candidate
			break
		}
	}
	if updated.ID == "" {
		t.Fatalf("expected to find task %s", task.ID)
	}
	if updated.Status != "in_progress" || updated.Blocked {
		t.Fatalf("expected external_workspace task to remain active during retry, got %+v", updated)
	}
}

func TestRecoverTimedOutHeadlessTurnRetriesOfficeTaskOnceBeforeBlocking(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Map backend contracts for fluxo 1",
		Owner:         "backend",
		CreatedBy:     "ceo",
		TaskType:      "follow_up",
		ExecutionMode: "office",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	l := newHeadlessLauncherForTest()
	l.broker = b
	l.headlessWorkers["backend"] = true

	turn := headlessCodexTurn{
		Prompt:   "Resolve #task-" + strings.TrimPrefix(task.ID, "task-"),
		Channel:  "general",
		TaskID:   task.ID,
		Attempts: 0,
	}
	l.recoverTimedOutHeadlessTurn("backend", turn, time.Now().UTC().Add(-2*time.Second), headlessCodexTurnTimeout)

	if len(l.headlessQueues["backend"]) != 1 {
		t.Fatalf("expected one queued retry, got %+v", l.headlessQueues["backend"])
	}
	if l.headlessQueues["backend"][0].Attempts != 1 {
		t.Fatalf("expected retry attempt 1, got %+v", l.headlessQueues["backend"][0])
	}

	var updated teamTask
	for _, candidate := range b.AllTasks() {
		if candidate.ID == task.ID {
			updated = candidate
			break
		}
	}
	if updated.ID == "" {
		t.Fatalf("expected to find task %s", task.ID)
	}
	if updated.Status != "in_progress" || updated.Blocked {
		t.Fatalf("expected office task to remain active during retry, got %+v", updated)
	}
}

func TestHeadlessCodexTurnTimeoutForExternalWorkspaceUsesLocalWorktreeBudget(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	b := NewBroker()
	workspace := t.TempDir()
	initUsableGitWorktree(t, workspace)
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Implement the external workspace backend slice",
		Owner:         "backend",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "external_workspace",
		WorkspacePath: workspace,
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	l := newHeadlessLauncherForTest()
	l.broker = b

	timeout := l.headlessCodexTurnTimeoutForTurn(headlessCodexTurn{TaskID: task.ID})
	if timeout != headlessCodexLocalWorktreeTurnTimeout {
		t.Fatalf("expected external_workspace timeout %s, got %s", headlessCodexLocalWorktreeTurnTimeout, timeout)
	}
}

func TestNormalizeHeadlessPromptPayloadRepairsInvalidUTF8AndNul(t *testing.T) {
	normalized, changed := normalizeHeadlessPromptPayload(string([]byte{'o', 'k', 0xff, 0x00, 'x'}))
	if !changed {
		t.Fatal("expected payload normalization to report change")
	}
	if !utf8.ValidString(normalized) {
		t.Fatalf("expected normalized payload to be valid utf-8, got %q", normalized)
	}
	if strings.Contains(normalized, "\x00") {
		t.Fatalf("expected normalized payload to remove NUL bytes, got %q", normalized)
	}
	if !strings.Contains(normalized, headlessPromptNormalizationReplacement) {
		t.Fatalf("expected normalized payload to include replacement rune, got %q", normalized)
	}
}

func TestRecoverFailedHeadlessTurnRetriesPromptEncodingFailureOnceForOfficeTask(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Investigate blocked backend runtime",
		Owner:         "backend",
		CreatedBy:     "ceo",
		TaskType:      "investigation",
		ExecutionMode: "office",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	l := newHeadlessLauncherForTest()
	l.broker = b
	l.headlessWorkers["backend"] = true

	turn := headlessCodexTurn{
		Prompt:   string([]byte{'F', 'i', 'x', ' ', 0xff, '#', 't', 'a', 's', 'k', '-', '1'}),
		Channel:  "general",
		TaskID:   task.ID,
		Attempts: 0,
	}
	l.recoverFailedHeadlessTurn("backend", turn, time.Now().UTC().Add(-2*time.Second), "Failed to read prompt from stdin: input is not valid UTF-8 (invalid byte at offset 4)")

	if len(l.headlessQueues["backend"]) != 1 {
		t.Fatalf("expected one queued retry, got %+v", l.headlessQueues["backend"])
	}
	retry := l.headlessQueues["backend"][0]
	if retry.Attempts != 1 {
		t.Fatalf("expected retry attempt 1, got %+v", retry)
	}
	if !utf8.ValidString(retry.Prompt) {
		t.Fatalf("expected retried prompt to be valid utf-8, got %q", retry.Prompt)
	}
	if !strings.Contains(retry.Prompt, "Failed to read prompt from stdin") {
		t.Fatalf("expected retry prompt to carry encoding failure detail, got %q", retry.Prompt)
	}

	var updated teamTask
	for _, candidate := range b.AllTasks() {
		if candidate.ID == task.ID {
			updated = candidate
			break
		}
	}
	if updated.ID == "" {
		t.Fatalf("expected to find task %s", task.ID)
	}
	if updated.Status != "in_progress" || updated.Blocked {
		t.Fatalf("expected task to remain active during encoding retry, got %+v", updated)
	}
}

func TestRecoverTimedOutLocalWorktreeRetriesEvenAfterSubstantiveReplyIfTaskStillActive(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	useStubTaskWorktreeProvisioning(t)

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Implement the studio build",
		Owner:         "eng",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	startedAt := time.Now().UTC().Add(-2 * time.Second)
	b.messages = append(b.messages, channelMessage{
		ID:        "msg-test-eng-timeout",
		From:      "eng",
		Channel:   "general",
		Content:   "I found the right files and I am wiring the generator now.",
		ReplyTo:   task.ThreadID,
		Timestamp: startedAt.Add(time.Second).Format(time.RFC3339),
	})

	l := newHeadlessLauncherForTest()
	l.broker = b
	l.headlessWorkers["eng"] = true

	l.recoverTimedOutHeadlessTurn("eng", headlessCodexTurn{
		Prompt:   "Build #task-" + strings.TrimPrefix(task.ID, "task-"),
		Channel:  "general",
		TaskID:   task.ID,
		Attempts: 0,
	}, startedAt, headlessCodexLocalWorktreeTurnTimeout)

	if len(l.headlessQueues["eng"]) != 1 {
		t.Fatalf("expected one queued retry, got %+v", l.headlessQueues["eng"])
	}

	var updated teamTask
	for _, candidate := range b.AllTasks() {
		if candidate.ID == task.ID {
			updated = candidate
			break
		}
	}
	if updated.ID == "" {
		t.Fatalf("expected to find task %s", task.ID)
	}
	if updated.Status != "in_progress" || updated.Blocked {
		t.Fatalf("expected task to remain active during retry, got %+v", updated)
	}
}

func TestRecoverTimedOutLocalWorktreeLeavesReviewReadyTaskUnchanged(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	useStubTaskWorktreeProvisioning(t)

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Implement the studio build",
		Owner:         "eng",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	for i := range b.tasks {
		if b.tasks[i].ID != task.ID {
			continue
		}
		b.tasks[i].Status = "review"
		b.tasks[i].ReviewState = "ready_for_review"
		b.tasks[i].Details = "Artifact shipped and awaiting review."
		b.tasks[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		break
	}

	l := newHeadlessLauncherForTest()
	l.broker = b

	l.recoverTimedOutHeadlessTurn("eng", headlessCodexTurn{
		Prompt:   "Build #task-" + strings.TrimPrefix(task.ID, "task-"),
		Channel:  "general",
		TaskID:   task.ID,
		Attempts: 0,
	}, time.Now().UTC().Add(-2*time.Second), headlessCodexLocalWorktreeTurnTimeout)

	if len(l.headlessQueues["eng"]) != 0 {
		t.Fatalf("expected no retry queue for review-ready task, got %+v", l.headlessQueues["eng"])
	}

	var updated teamTask
	for _, candidate := range b.AllTasks() {
		if candidate.ID == task.ID {
			updated = candidate
			break
		}
	}
	if updated.ID == "" {
		t.Fatalf("expected to find task %s", task.ID)
	}
	if updated.Status != "review" || updated.ReviewState != "ready_for_review" {
		t.Fatalf("expected task to remain review-ready, got %+v", updated)
	}
}

func TestRecoverTimedOutHeadlessTurnBlocksLocalWorktreeAfterRetryExhausted(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	useStubTaskWorktreeProvisioning(t)

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Implement the studio build",
		Owner:         "eng",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	l := newHeadlessLauncherForTest()
	l.broker = b

	l.recoverTimedOutHeadlessTurn("eng", headlessCodexTurn{
		Prompt:   "Ship #task-" + strings.TrimPrefix(task.ID, "task-"),
		Channel:  "general",
		TaskID:   task.ID,
		Attempts: headlessCodexLocalWorktreeRetryLimit,
	}, time.Now().UTC().Add(-2*time.Second), headlessCodexLocalWorktreeTurnTimeout)

	var updated teamTask
	for _, candidate := range b.AllTasks() {
		if candidate.ID == task.ID {
			updated = candidate
			break
		}
	}
	if updated.ID == "" {
		t.Fatalf("expected to find task %s", task.ID)
	}
	if updated.Status != "blocked" || !updated.Blocked {
		t.Fatalf("expected task to be blocked after retry budget exhausted, got %+v", updated)
	}
}

func TestRecoverFailedHeadlessTurnBlocksLocalWorktreeAfterRetryExhausted(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	useStubTaskWorktreeProvisioning(t)

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Implement queue mode for the content factory",
		Owner:         "eng",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	l := newHeadlessLauncherForTest()
	l.broker = b

	l.recoverFailedHeadlessTurn("eng", headlessCodexTurn{
		Prompt:   "Ship #task-" + strings.TrimPrefix(task.ID, "task-"),
		Channel:  "general",
		TaskID:   task.ID,
		Attempts: headlessCodexLocalWorktreeRetryLimit,
	}, time.Now().UTC().Add(-2*time.Second), "failed to write patch to assigned workspace")

	var updated teamTask
	for _, candidate := range b.AllTasks() {
		if candidate.ID == task.ID {
			updated = candidate
			break
		}
	}
	if updated.ID == "" {
		t.Fatalf("expected to find task %s", task.ID)
	}
	if updated.Status != "blocked" || !updated.Blocked {
		t.Fatalf("expected task to be blocked after retry budget exhausted, got %+v", updated)
	}
	if !strings.Contains(updated.Details, "failed to write patch to assigned workspace") {
		t.Fatalf("expected failure detail appended, got %+v", updated)
	}
}

func TestRecoverFailedHeadlessTurnRequeuesExternalActionBeforeBlocking(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Send a live Slack kickoff update and pivot to Notion if needed (completed)",
		Details:       "Use the connected Slack target first. If it fails, pivot to the smallest useful live Notion action.",
		Owner:         "operator",
		CreatedBy:     "ceo",
		TaskType:      "follow_up",
		ExecutionMode: "office",
	})
	if err != nil {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	l := newHeadlessLauncherForTest()
	l.broker = b

	l.recoverFailedHeadlessTurn("operator", headlessCodexTurn{
		Prompt:   "Send #task-" + strings.TrimPrefix(task.ID, "task-"),
		Channel:  "general",
		TaskID:   task.ID,
		Attempts: 0,
	}, time.Now().UTC().Add(-2*time.Second), "channel_not_found")

	queue := l.headlessQueues["operator"]
	if len(queue) != 1 {
		t.Fatalf("expected one retry queued for external action, got %+v", queue)
	}
	if queue[0].Attempts != 1 {
		t.Fatalf("expected retry attempt count 1, got %+v", queue[0])
	}
	if !strings.Contains(queue[0].Prompt, "live external-action task") {
		t.Fatalf("expected external recovery prompt, got %q", queue[0].Prompt)
	}
	if !strings.Contains(queue[0].Prompt, "smallest useful live Notion or Drive action") {
		t.Fatalf("expected pivot guidance in retry prompt, got %q", queue[0].Prompt)
	}
}

func TestRecoverFailedHeadlessTurnLeavesCompletedExternalActionUnchanged(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Send a live Slack kickoff update and pivot to Notion if needed (completed)",
		Details:       "Already closed after the live follow-up landed.",
		Owner:         "operator",
		CreatedBy:     "ceo",
		TaskType:      "follow_up",
		ExecutionMode: "office",
	})
	if err != nil {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	for i := range b.tasks {
		if b.tasks[i].ID != task.ID {
			continue
		}
		b.tasks[i].Status = "done"
		b.tasks[i].ReviewState = "not_required"
		b.tasks[i].Details = "Closed with live evidence already captured."
		b.tasks[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		break
	}

	l := newHeadlessLauncherForTest()
	l.broker = b

	l.recoverFailedHeadlessTurn("operator", headlessCodexTurn{
		Prompt:   "Send #task-" + strings.TrimPrefix(task.ID, "task-"),
		Channel:  "general",
		TaskID:   task.ID,
		Attempts: 0,
	}, time.Now().UTC().Add(-2*time.Second), "channel_not_found")

	if len(l.headlessQueues["operator"]) != 0 {
		t.Fatalf("expected no retry queue for completed external action, got %+v", l.headlessQueues["operator"])
	}

	var updated teamTask
	for _, candidate := range b.AllTasks() {
		if candidate.ID == task.ID {
			updated = candidate
			break
		}
	}
	if updated.ID == "" {
		t.Fatalf("expected to find task %s", task.ID)
	}
	if updated.Status != "done" || updated.ReviewState != "not_required" {
		t.Fatalf("expected task to remain completed, got %+v", updated)
	}
}

func TestRecoverFailedHeadlessTurnLeavesCanceledExternalActionUnchanged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Send a live Slack kickoff update and pivot to Notion if needed (canceled)",
		Details:       "Canceled after a newer live path landed.",
		Owner:         "operator",
		CreatedBy:     "ceo",
		TaskType:      "follow_up",
		ExecutionMode: "office",
	})
	if err != nil {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	for i := range b.tasks {
		if b.tasks[i].ID != task.ID {
			continue
		}
		b.tasks[i].Status = "canceled"
		b.tasks[i].ReviewState = "not_required"
		b.tasks[i].Details = "Canceled after the office closed this lane."
		b.tasks[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		break
	}

	l := newHeadlessLauncherForTest()
	l.broker = b

	l.recoverFailedHeadlessTurn("operator", headlessCodexTurn{
		Prompt:   "Send #task-" + strings.TrimPrefix(task.ID, "task-"),
		Channel:  "general",
		TaskID:   task.ID,
		Attempts: 0,
	}, time.Now().UTC().Add(-2*time.Second), "channel_not_found")

	if len(l.headlessQueues["operator"]) != 0 {
		t.Fatalf("expected no retry queue for canceled external action, got %+v", l.headlessQueues["operator"])
	}

	var updated teamTask
	for _, candidate := range b.AllTasks() {
		if candidate.ID == task.ID {
			updated = candidate
			break
		}
	}
	if updated.ID == "" {
		t.Fatalf("expected to find task %s", task.ID)
	}
	if updated.Status != "canceled" || updated.ReviewState != "not_required" {
		t.Fatalf("expected task to remain canceled, got %+v", updated)
	}
}

func TestHeadlessTurnCompletedDurablyRejectsCodingTurnWithoutTaskStateOrEvidence(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	useStubTaskWorktreeProvisioning(t)

	oldSnapshot := headlessCodexWorkspaceStatusSnapshot
	headlessCodexWorkspaceStatusSnapshot = func(string) string {
		return "after-change"
	}
	defer func() { headlessCodexWorkspaceStatusSnapshot = oldSnapshot }()

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Implement the durable turn guard",
		Owner:         "eng",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	l := newHeadlessLauncherForTest()
	l.broker = b

	ok, reason := l.headlessTurnCompletedDurably("eng", &headlessCodexActiveTurn{
		Turn: headlessCodexTurn{
			TaskID: task.ID,
		},
		StartedAt:         time.Now().UTC().Add(-2 * time.Second),
		WorkspaceDir:      t.TempDir(),
		WorkspaceSnapshot: "before-change",
	})
	if ok {
		t.Fatal("expected coding turn without task closure or evidence to be rejected")
	}
	if !strings.Contains(reason, "without durable task state or completion evidence") && !strings.Contains(reason, "changed workspace") {
		t.Fatalf("expected durable completion failure reason, got %q", reason)
	}
}

func TestHeadlessTurnCompletedDurablyAcceptsReviewReadyTask(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	useStubTaskWorktreeProvisioning(t)

	oldSnapshot := headlessCodexWorkspaceStatusSnapshot
	headlessCodexWorkspaceStatusSnapshot = func(string) string {
		return "after-change"
	}
	defer func() { headlessCodexWorkspaceStatusSnapshot = oldSnapshot }()

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Implement the durable turn guard",
		Owner:         "eng",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	for i := range b.tasks {
		if b.tasks[i].ID != task.ID {
			continue
		}
		b.tasks[i].Status = "review"
		b.tasks[i].ReviewState = "ready_for_review"
		break
	}

	l := newHeadlessLauncherForTest()
	l.broker = b

	ok, reason := l.headlessTurnCompletedDurably("eng", &headlessCodexActiveTurn{
		Turn: headlessCodexTurn{
			TaskID: task.ID,
		},
		StartedAt:         time.Now().UTC().Add(-2 * time.Second),
		WorkspaceDir:      t.TempDir(),
		WorkspaceSnapshot: "before-change",
	})
	if !ok {
		t.Fatalf("expected review-ready task to satisfy durable completion, got %q", reason)
	}
}

func TestHeadlessTurnCompletedDurablyRejectsLocalWorktreeBuilderWithoutTaskStateOrEvidence(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	useStubTaskWorktreeProvisioning(t)

	oldSnapshot := headlessCodexWorkspaceStatusSnapshot
	headlessCodexWorkspaceStatusSnapshot = func(string) string {
		return "after-change"
	}
	defer func() { headlessCodexWorkspaceStatusSnapshot = oldSnapshot }()

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Build the dry-run intake packet",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	l := newHeadlessLauncherForTest()
	l.broker = b

	ok, reason := l.headlessTurnCompletedDurably("builder", &headlessCodexActiveTurn{
		Turn: headlessCodexTurn{
			TaskID: task.ID,
		},
		StartedAt:         time.Now().UTC().Add(-2 * time.Second),
		WorkspaceDir:      t.TempDir(),
		WorkspaceSnapshot: "before-change",
	})
	if ok {
		t.Fatal("expected local_worktree builder turn without task closure or evidence to be rejected")
	}
	if !strings.Contains(reason, "without durable task state or completion evidence") && !strings.Contains(reason, "changed workspace") {
		t.Fatalf("expected durable completion failure reason, got %q", reason)
	}
}

func TestHeadlessTurnCompletedDurablyRejectsExternalCompletionWithoutWorkflowEvidence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:     "general",
		Title:       "Create one new Notion client workspace page for the consulting engagement",
		Details:     "Use the connected Notion workspace and leave the new client-facing page link in channel.",
		Owner:       "builder",
		CreatedBy:   "ceo",
		TaskType:    "follow_up",
		ReviewState: "ready_for_review",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	for i := range b.tasks {
		if b.tasks[i].ID == task.ID {
			b.tasks[i].Status = "review"
			b.tasks[i].ReviewState = "ready_for_review"
			break
		}
	}

	l := newHeadlessLauncherForTest()
	l.broker = b

	ok, reason := l.headlessTurnCompletedDurably("builder", &headlessCodexActiveTurn{
		Turn:      headlessCodexTurn{TaskID: task.ID},
		StartedAt: time.Now().UTC().Add(-2 * time.Second),
	})
	if ok {
		t.Fatal("expected external completion without workflow evidence to be rejected")
	}
	if !strings.Contains(reason, "without recorded external execution evidence") {
		t.Fatalf("expected external evidence failure reason, got %q", reason)
	}
}

func TestHeadlessTurnCompletedDurablyAcceptsExternalCompletionWithWorkflowEvidence(t *testing.T) {
	isolateBrokerStateForTest(t)

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:     "general",
		Title:       "Create one new Notion client workspace page for the consulting engagement",
		Details:     "Use the connected Notion workspace and leave the new client-facing page link in channel.",
		Owner:       "builder",
		CreatedBy:   "ceo",
		TaskType:    "follow_up",
		ReviewState: "ready_for_review",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	for i := range b.tasks {
		if b.tasks[i].ID == task.ID {
			b.tasks[i].Status = "review"
			b.tasks[i].ReviewState = "ready_for_review"
			break
		}
	}
	if err := b.RecordAction("external_workflow_executed", "notion", "general", "builder", "Created client workspace page in Notion", "workflow-notion-client-page", nil, ""); err != nil {
		t.Fatalf("record action: %v", err)
	}

	l := newHeadlessLauncherForTest()
	l.broker = b

	ok, reason := l.headlessTurnCompletedDurably("builder", &headlessCodexActiveTurn{
		Turn:      headlessCodexTurn{TaskID: task.ID},
		StartedAt: time.Now().UTC().Add(-2 * time.Second),
	})
	if !ok {
		t.Fatalf("expected external completion with workflow evidence to be accepted, got %q", reason)
	}
}

func TestHeadlessTurnCompletedDurablyAcceptsExternalCompletionWithActionEvidence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:     "general",
		Title:       "Verify the new Notion client workspace page for the consulting engagement",
		Details:     "Use the connected Notion workspace and confirm the client-facing page is live.",
		Owner:       "reviewer",
		CreatedBy:   "ceo",
		TaskType:    "follow_up",
		ReviewState: "not_required",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	for i := range b.tasks {
		if b.tasks[i].ID == task.ID {
			b.tasks[i].Status = "done"
			b.tasks[i].ReviewState = "not_required"
			break
		}
	}
	if err := b.RecordAction("external_action_executed", "one", "general", "reviewer", "Verified client workspace page in Notion", "notion-client-page", nil, ""); err != nil {
		t.Fatalf("record action: %v", err)
	}

	l := newHeadlessLauncherForTest()
	l.broker = b

	ok, reason := l.headlessTurnCompletedDurably("reviewer", &headlessCodexActiveTurn{
		Turn:      headlessCodexTurn{TaskID: task.ID},
		StartedAt: time.Now().UTC().Add(-2 * time.Second),
	})
	if !ok {
		t.Fatalf("expected external completion with action evidence to be accepted, got %q", reason)
	}
}

func TestBeginHeadlessCodexTurnCapturesWorktreeForLocalWorktreeBuilder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	worktreeDir := t.TempDir()
	oldPrepareTaskWorktree := prepareTaskWorktree
	oldSnapshot := headlessCodexWorkspaceStatusSnapshot
	prepareTaskWorktree = func(taskID string) (string, string, error) {
		return worktreeDir, worktreeBranchName(taskID), nil
	}
	headlessCodexWorkspaceStatusSnapshot = func(path string) string {
		if !samePath(path, worktreeDir) {
			t.Fatalf("expected workspace snapshot to target %q, got %q", worktreeDir, path)
		}
		return "snapshot"
	}
	defer func() {
		prepareTaskWorktree = oldPrepareTaskWorktree
		headlessCodexWorkspaceStatusSnapshot = oldSnapshot
	}()

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Build the dry-run intake packet",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	l := newHeadlessLauncherForTest()
	l.broker = b
	l.headlessQueues["builder"] = []headlessCodexTurn{{TaskID: task.ID}}

	_, _, _, _, ok := l.beginHeadlessCodexTurn("builder", false)
	if !ok {
		t.Fatal("expected queued builder turn to begin")
	}
	active := l.headlessActive["builder"]
	if active == nil {
		t.Fatal("expected active builder turn")
	}
	if !samePath(active.WorkspaceDir, worktreeDir) {
		t.Fatalf("expected builder workspace %q, got %q", worktreeDir, active.WorkspaceDir)
	}
	if active.WorkspaceSnapshot != "snapshot" {
		t.Fatalf("expected workspace snapshot to be recorded, got %q", active.WorkspaceSnapshot)
	}
}

func TestBeginHeadlessCodexTurnUsesQueuedExternalWorkspaceTaskWhenNoActiveTask(t *testing.T) {
	workspaceDir := t.TempDir()
	oldSnapshot := headlessCodexWorkspaceStatusSnapshot
	headlessCodexWorkspaceStatusSnapshot = func(path string) string {
		if !samePath(path, workspaceDir) {
			t.Fatalf("expected workspace snapshot to target %q, got %q", workspaceDir, path)
		}
		return "snapshot"
	}
	defer func() {
		headlessCodexWorkspaceStatusSnapshot = oldSnapshot
	}()

	b := NewBroker()
	b.tasks = []teamTask{{
		ID:            "task-1682",
		Channel:       "legado-para-novo",
		Title:         "Fix onboarding in repo novo",
		Owner:         "builder",
		Status:        "blocked",
		CreatedBy:     "ceo",
		ExecutionMode: "external_workspace",
		WorkspacePath: workspaceDir,
	}}

	l := newHeadlessLauncherForTest()
	l.broker = b
	l.headlessQueues["builder"] = []headlessCodexTurn{{TaskID: "task-1682"}}

	_, _, _, _, ok := l.beginHeadlessCodexTurn("builder", false)
	if !ok {
		t.Fatal("expected queued builder turn to begin")
	}
	active := l.headlessActive["builder"]
	if active == nil {
		t.Fatal("expected active builder turn")
	}
	if !samePath(active.WorkspaceDir, workspaceDir) {
		t.Fatalf("expected builder workspace %q, got %q", workspaceDir, active.WorkspaceDir)
	}
	if active.WorkspaceSnapshot != "snapshot" {
		t.Fatalf("expected workspace snapshot to be recorded, got %q", active.WorkspaceSnapshot)
	}
}

func TestRunHeadlessCodexQueueRetriesLocalWorktreeAfterGenericError(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	useStubTaskWorktreeProvisioning(t)
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Implement queue mode for the content factory",
		Owner:         "eng",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	oldRunTurn := headlessCodexRunTurn
	defer func() { headlessCodexRunTurn = oldRunTurn }()

	processed := make(chan string, 2)
	attempt := 0
	headlessCodexRunTurn = func(_ *Launcher, _ context.Context, _ string, notification string, channel ...string) error {
		attempt++
		processed <- notification
		if attempt == 1 {
			return fmt.Errorf("failed to write patch to assigned workspace")
		}
		return nil
	}

	l := newHeadlessLauncherForTest()
	l.broker = b
	l.headlessWorkers["eng"] = true
	l.headlessQueues["eng"] = []headlessCodexTurn{{
		Prompt:     "Build #task-" + strings.TrimPrefix(task.ID, "task-"),
		Channel:    "general",
		TaskID:     task.ID,
		Attempts:   0,
		EnqueuedAt: time.Now(),
	}}

	done := make(chan struct{})
	go func() {
		l.runHeadlessCodexQueue("eng")
		close(done)
	}()

	first := waitForString(t, processed)
	second := waitForString(t, processed)
	if first == second {
		t.Fatalf("expected retry prompt to differ from the original prompt, got %q", first)
	}
	if !strings.Contains(second, "Previous attempt by @eng failed") {
		t.Fatalf("expected retry prompt note, got %q", second)
	}
	if !strings.Contains(second, "failed to write patch to assigned workspace") {
		t.Fatalf("expected retry prompt to include provider failure, got %q", second)
	}
	waitForSignal(t, done)
}

func TestHeadlessCodexTurnTimeoutForLocalWorktreeTask(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	useStubTaskWorktreeProvisioning(t)

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Implement the studio build",
		Owner:         "eng",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	l := newHeadlessLauncherForTest()
	l.broker = b

	if got := l.headlessCodexTurnTimeoutForTurn(headlessCodexTurn{TaskID: task.ID}); got != headlessCodexLocalWorktreeTurnTimeout {
		t.Fatalf("expected local worktree timeout %s, got %s", headlessCodexLocalWorktreeTurnTimeout, got)
	}
	if got := l.headlessCodexStaleCancelAfterForTurn(headlessCodexTurn{TaskID: task.ID}); got != headlessCodexLocalWorktreeTurnTimeout {
		t.Fatalf("expected local worktree stale cancel threshold %s, got %s", headlessCodexLocalWorktreeTurnTimeout, got)
	}
}

func TestHeadlessCodexTurnTimeoutForOfficeLaunchTask(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Produce the launch assets and operating pack",
		Owner:         "gtm",
		CreatedBy:     "ceo",
		TaskType:      "launch",
		ExecutionMode: "office",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	l := newHeadlessLauncherForTest()
	l.broker = b

	if got := l.headlessCodexTurnTimeoutForTurn(headlessCodexTurn{TaskID: task.ID}); got != headlessCodexOfficeLaunchTurnTimeout {
		t.Fatalf("expected office launch timeout %s, got %s", headlessCodexOfficeLaunchTurnTimeout, got)
	}
	if got := l.headlessCodexStaleCancelAfterForTurn(headlessCodexTurn{TaskID: task.ID}); got != headlessCodexOfficeLaunchTurnTimeout {
		t.Fatalf("expected office launch stale cancel threshold %s, got %s", headlessCodexOfficeLaunchTurnTimeout, got)
	}
}

func TestEnqueueHeadlessCodexTurnDefersLeadUntilSpecialistFinishes(t *testing.T) {
	oldRunTurn := headlessCodexRunTurn
	processed := make(chan string, 2)
	headlessCodexRunTurn = func(_ *Launcher, _ context.Context, _ string, notification string, channel ...string) error {
		processed <- notification
		return nil
	}
	defer func() { headlessCodexRunTurn = oldRunTurn }()

	l := newHeadlessLauncherForTest()
	l.headlessActive["eng"] = &headlessCodexActiveTurn{}

	l.enqueueHeadlessCodexTurn("ceo", "task-5 blocked after timeout")
	if l.headlessDeferredLead["ceo"] == nil {
		t.Fatal("expected lead work to be deferred while specialist is active")
	}

	l.finishHeadlessTurn("eng", false)

	if got := waitForString(t, processed); got != "task-5 blocked after timeout" {
		t.Fatalf("expected deferred lead notification to replay after specialist finished, got %q", got)
	}
}

func TestEnqueueHeadlessCodexTurnDoesNotDeferLeadForNonTaskPing(t *testing.T) {
	oldRunTurn := headlessCodexRunTurn
	processed := make(chan string, 1)
	headlessCodexRunTurn = func(_ *Launcher, _ context.Context, _ string, notification string, channel ...string) error {
		processed <- notification
		return nil
	}
	defer func() { headlessCodexRunTurn = oldRunTurn }()

	l := newHeadlessLauncherForTest()
	l.headlessActive["eng"] = &headlessCodexActiveTurn{}

	l.enqueueHeadlessCodexTurn("ceo", "estamos online?")

	if len(l.headlessDeferredLead) != 0 {
		t.Fatal("expected non-task lead ping to bypass specialist deferral")
	}
	if got := waitForString(t, processed); got != "estamos online?" {
		t.Fatalf("expected immediate lead notification for non-task ping, got %q", got)
	}
}

func TestEnqueueHeadlessCodexTurnDoesNotDeferLeadForFollowUpTask(t *testing.T) {
	oldRunTurn := headlessCodexRunTurn
	processed := make(chan string, 1)
	headlessCodexRunTurn = func(_ *Launcher, _ context.Context, _ string, notification string, channel ...string) error {
		processed <- notification
		return nil
	}
	defer func() { headlessCodexRunTurn = oldRunTurn }()

	b := NewBroker()
	b.mu.Lock()
	b.tasks = []teamTask{{
		ID:            "task-886",
		Channel:       "exampleworkflow-web-azure",
		Owner:         "ceo",
		Status:        "in_progress",
		ThreadID:      "msg-814",
		TaskType:      "follow_up",
		PipelineID:    "follow_up",
		ExecutionMode: "office",
	}}
	b.mu.Unlock()

	l := newHeadlessLauncherForTest()
	l.broker = b
	l.headlessActive[agentLaneKey("exampleworkflow-web-azure", "frontend")] = &headlessCodexActiveTurn{}

	l.enqueueHeadlessCodexTurnRecord("ceo", headlessCodexTurn{
		Prompt:     "[Task update #task-886 on #exampleworkflow-web-azure]: Reply to pending message from @you",
		Channel:    "exampleworkflow-web-azure",
		TaskID:     "task-886",
		EnqueuedAt: time.Now(),
	})

	if len(l.headlessDeferredLead) != 0 {
		t.Fatal("expected lead follow-up task to bypass specialist deferral")
	}
	if got := waitForString(t, processed); !strings.Contains(got, "#task-886") {
		t.Fatalf("expected immediate lead notification for follow-up task, got %q", got)
	}
}

func TestEnqueueHeadlessCodexTurnKeepsNonTaskLeadPingWhenQueueAtCap(t *testing.T) {
	l := newHeadlessLauncherForTest()
	l.headlessWorkers["ceo"] = true
	l.headlessQueues["ceo"] = []headlessCodexTurn{{
		Prompt:     "pending task handoff about #task-9",
		TaskID:     "task-9",
		EnqueuedAt: time.Now().Add(-time.Minute),
	}}

	l.enqueueHeadlessCodexTurn("ceo", "estamos online?")

	queue := l.headlessQueues["ceo"]
	if got := len(queue); got != 1 {
		t.Fatalf("expected lead queue to keep a single pending turn, got %d", got)
	}
	if got := queue[0].Prompt; got != "estamos online?" {
		t.Fatalf("expected non-task lead ping to replace capped queue entry, got %q", got)
	}
}

func TestEnqueueHeadlessCodexTurnRecordRoutesLeadQuickReplyToDedicatedLane(t *testing.T) {
	l := newHeadlessLauncherForTest()
	cancelled := false
	l.headlessWorkers["ceo"] = true
	l.headlessQueues["ceo"] = []headlessCodexTurn{{
		Prompt:     "pending task handoff about #task-9",
		TaskID:     "task-9",
		EnqueuedAt: time.Now().Add(-time.Minute),
	}}
	l.headlessActive["ceo"] = &headlessCodexActiveTurn{
		Turn:   headlessCodexTurn{Prompt: "background task", TaskID: "task-9"},
		Cancel: func() { cancelled = true },
	}

	l.enqueueHeadlessCodexTurnRecord("ceo", headlessCodexTurn{
		Prompt:     "estamos online?",
		Channel:    "general",
		EnqueuedAt: time.Now(),
		QuickReply: true,
	})

	if cancelled {
		t.Fatal("expected quick-reply lane to avoid cancelling the active lead task turn")
	}
	if got := len(l.headlessQueues["ceo"]); got != 1 {
		t.Fatalf("expected normal lead queue unchanged, got %d pending turns", got)
	}
	if got := l.headlessQueues["ceo"][0].Prompt; got != "pending task handoff about #task-9" {
		t.Fatalf("expected normal lead queue to keep background task, got %q", got)
	}
	if got := len(l.headlessQuickQueues["ceo"]); got != 1 {
		t.Fatalf("expected dedicated quick-reply queue to hold the human ping, got %d", got)
	}
	if got := l.headlessQuickQueues["ceo"][0].Prompt; got != "estamos online?" {
		t.Fatalf("expected quick-reply queue to hold human ping, got %q", got)
	}
}

func TestLeadQuickReplyLaneRunsWhileNormalLeadTurnIsActive(t *testing.T) {
	oldRunTurn := headlessCodexRunTurn
	processed := make(chan string, 1)
	headlessCodexRunTurn = func(_ *Launcher, _ context.Context, _ string, notification string, channel ...string) error {
		processed <- notification
		return nil
	}
	defer func() { headlessCodexRunTurn = oldRunTurn }()

	l := newHeadlessLauncherForTest()
	l.headlessActive["ceo"] = &headlessCodexActiveTurn{
		Turn:      headlessCodexTurn{Prompt: "background task", TaskID: "task-9"},
		StartedAt: time.Now().Add(-5 * time.Second),
	}

	l.enqueueHeadlessCodexTurnRecord("ceo", headlessCodexTurn{
		Prompt:     "estamos online?",
		Channel:    "general",
		EnqueuedAt: time.Now(),
		QuickReply: true,
	})

	if got := waitForString(t, processed); got != "estamos online?" {
		t.Fatalf("expected quick-reply lane to run immediately beside the background turn, got %q", got)
	}
	if active := l.headlessActive["ceo"]; active == nil || active.Turn.TaskID != "task-9" {
		t.Fatalf("expected background lead turn to remain active, got %+v", active)
	}
}

func TestSendChannelUpdateRoutesUntaggedGeneralHumanMessageToLeadQuickReplyLane(t *testing.T) {
	l := newHeadlessLauncherForTest()
	l.webMode = true
	l.broker = NewBroker()
	l.headlessQuickWorkers["ceo"] = true

	l.sendChannelUpdate(notificationTarget{Slug: "ceo"}, channelMessage{
		ID:      "msg-quick-1",
		From:    "you",
		Channel: "general",
		Content: "estamos online?",
	})

	if got := len(l.headlessQueues["ceo"]); got != 0 {
		t.Fatalf("expected normal lead queue untouched, got %d pending turns", got)
	}
	if got := len(l.headlessQuickQueues["ceo"]); got != 1 {
		t.Fatalf("expected quick-reply queue to receive the human general ping, got %d", got)
	}
	if !l.headlessQuickQueues["ceo"][0].QuickReply {
		t.Fatal("expected quick-reply turn to be marked as dedicated quick reply")
	}
	if got := l.headlessQuickQueues["ceo"][0].TaskID; got != "" {
		t.Fatalf("expected quick-reply lane to suppress task id inference, got %q", got)
	}
}

func TestSendChannelUpdateRoutesTaggedLeadGeneralHumanMessageToLeadQuickReplyLane(t *testing.T) {
	l := newHeadlessLauncherForTest()
	l.webMode = true
	l.broker = NewBroker()
	l.headlessQuickWorkers["ceo"] = true

	l.sendChannelUpdate(notificationTarget{Slug: "ceo"}, channelMessage{
		ID:      "msg-quick-2",
		From:    "you",
		Channel: "general",
		Content: "@ceo responde rapido",
		Tagged:  []string{"ceo"},
	})

	if got := len(l.headlessQueues["ceo"]); got != 0 {
		t.Fatalf("expected normal lead queue untouched, got %d pending turns", got)
	}
	if got := len(l.headlessQuickQueues["ceo"]); got != 1 {
		t.Fatalf("expected quick-reply queue to receive the tagged CEO ping, got %d", got)
	}
	if !l.headlessQuickQueues["ceo"][0].QuickReply {
		t.Fatal("expected tagged CEO ping to be marked as dedicated quick reply")
	}
	if got := l.headlessQuickQueues["ceo"][0].TaskID; got != "" {
		t.Fatalf("expected quick-reply lane to suppress task id inference, got %q", got)
	}
}

func TestSendChannelUpdateRoutesTaggedLeadHumanMessageFromScopedChannelToLeadQuickReplyLane(t *testing.T) {
	l := newHeadlessLauncherForTest()
	l.webMode = true
	l.broker = NewBroker()
	laneKey := agentLaneKey("legado-para-novo", "ceo")
	l.headlessQuickWorkers[laneKey] = true

	l.sendChannelUpdate(notificationTarget{Slug: "ceo"}, channelMessage{
		ID:      "msg-quick-3",
		From:    "you",
		Channel: "legado-para-novo",
		Content: "@ceo responda aqui",
		Tagged:  []string{"ceo"},
	})

	if got := len(l.headlessQueues["ceo"]); got != 0 {
		t.Fatalf("expected normal lead queue untouched, got %d pending turns", got)
	}
	if got := len(l.headlessQuickQueues[laneKey]); got != 1 {
		t.Fatalf("expected quick-reply queue to receive the scoped CEO ping, got %d", got)
	}
	if !l.headlessQuickQueues[laneKey][0].QuickReply {
		t.Fatal("expected scoped CEO ping to be marked as dedicated quick reply")
	}
	if got := l.headlessQuickQueues[laneKey][0].TaskID; got != "" {
		t.Fatalf("expected quick-reply lane to suppress task id inference, got %q", got)
	}
}

func TestSendChannelUpdateDoesNotInferTaskIDFromFreeTextMessage(t *testing.T) {
	l := newHeadlessLauncherForTest()
	l.webMode = true
	l.broker = NewBroker()
	laneKey := agentLaneKey("migration-exampleworkflow", "backend")
	l.headlessWorkers[laneKey] = true

	l.sendChannelUpdate(notificationTarget{Slug: "backend"}, channelMessage{
		ID:      "msg-task-mention",
		From:    "ceo",
		Channel: "migration-exampleworkflow",
		Content: "@backend assuma a task-2775 e investigue o erro da task-1629.",
		Tagged:  []string{"backend"},
	})

	if got := len(l.headlessQueues[laneKey]); got != 1 {
		t.Fatalf("expected backend lane to receive one queued message turn, got %d", got)
	}
	if got := l.headlessQueues[laneKey][0].TaskID; got != "" {
		t.Fatalf("expected free-text message turn not to infer active task id, got %q", got)
	}
}

func TestEnqueueHeadlessCodexTurnBypassesLeadHoldForReviewReadyTask(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	oldRunTurn := headlessCodexRunTurn
	processed := make(chan string, 1)
	headlessCodexRunTurn = func(_ *Launcher, _ context.Context, _ string, notification string, channel ...string) error {
		processed <- notification
		return nil
	}
	defer func() { headlessCodexRunTurn = oldRunTurn }()

	oldStatePath := brokerStatePath
	stateDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(stateDir, "broker-state.json") }
	defer func() { brokerStatePath = oldStatePath }()

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Define channel thesis and monetization system",
		Owner:         "gtm",
		CreatedBy:     "ceo",
		TaskType:      "launch",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	b.mu.Lock()
	for i := range b.tasks {
		if b.tasks[i].ID != task.ID {
			continue
		}
		b.tasks[i].Status = "review"
		b.tasks[i].ReviewState = "ready_for_review"
		task = b.tasks[i]
		break
	}
	b.mu.Unlock()

	l := newHeadlessLauncherForTest()
	l.broker = b
	l.headlessActive["eng"] = &headlessCodexActiveTurn{}

	action := officeActionLog{
		Kind:      "task_updated",
		Actor:     "gtm",
		Channel:   "general",
		RelatedID: task.ID,
	}
	content := l.taskNotificationContent(action, task)
	packet := l.buildTaskExecutionPacket("ceo", action, task, content)

	l.enqueueHeadlessCodexTurn("ceo", packet)

	if len(l.headlessDeferredLead) != 0 {
		t.Fatal("expected review-ready task notification to bypass lead deferral")
	}
	got := waitForString(t, processed)
	if !strings.Contains(got, "#"+task.ID) {
		t.Fatalf("expected immediate lead packet for %s, got %q", task.ID, got)
	}
}

func waitForSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for signal")
	}
}

func waitForString(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for string")
		return ""
	}
}

func waitForDispatchCall[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for dispatch call")
		var zero T
		return zero
	}
}

func waitForProcessedTurn(t *testing.T, ch <-chan processedTurn) processedTurn {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for processed turn")
		return processedTurn{}
	}
}
