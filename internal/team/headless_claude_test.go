package team

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nex-crm/wuphf/internal/agent"
	"github.com/nex-crm/wuphf/internal/provider"
)

func TestHeadlessReplyRouteParsesChannelReplyInstruction(t *testing.T) {
	tests := []struct {
		name         string
		notification string
		wantChannel  string
		wantReplyTo  string
	}{
		{
			name:         "one-on-one wording",
			notification: `Reply using team_broadcast with my_slug "ceo" and channel "general" reply_to_id "msg-1".`,
			wantChannel:  "general",
			wantReplyTo:  "msg-1",
		},
		{
			name:         "office packet wording",
			notification: `Just do the work and reply via team_broadcast with my_slug "ceo", channel "general", reply_to_id "msg-2".`,
			wantChannel:  "general",
			wantReplyTo:  "msg-2",
		},
		{
			name:         "explicit route marker wins over stale packet route",
			notification: "Old context reply via team_broadcast with my_slug \"ceo\", channel \"general\", reply_to_id \"msg-2\".\n[WUPHF_REPLY_ROUTE channel=\"general\" reply_to_id=\"msg-99\"]",
			wantChannel:  "general",
			wantReplyTo:  "msg-99",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel, replyTo, ok := headlessReplyRoute(tt.notification)
			if !ok {
				t.Fatalf("expected routing info from %q", tt.notification)
			}
			if channel != tt.wantChannel || replyTo != tt.wantReplyTo {
				t.Fatalf("route = (%q, %q), want (%q, %q)", channel, replyTo, tt.wantChannel, tt.wantReplyTo)
			}
		})
	}
}

// minimalLauncher builds a Launcher with a predictable two-member pack so
// officeLeadSlug() always returns "ceo".
func minimalLauncher(opusCEO bool) *Launcher {
	return &Launcher{
		pack: &agent.PackDefinition{
			LeadSlug: "ceo",
			Agents: []agent.AgentConfig{
				{Slug: "ceo", Name: "CEO"},
				{Slug: "eng", Name: "Engineer"},
				{Slug: "pm", Name: "Product Manager"},
			},
		},
		opusCEO:         opusCEO,
		headlessWorkers: make(map[string]bool),
		headlessActive:  make(map[string]*headlessCodexActiveTurn),
		headlessQueues:  make(map[string][]headlessCodexTurn),
	}
}

// ─── headlessClaudeModel ──────────────────────────────────────────────────

// TestHeadlessClaudeModel_SonnetByDefault verifies that every agent, including
// the lead, uses the Sonnet model when opusCEO is false.
func TestHeadlessClaudeModel_SonnetByDefault(t *testing.T) {
	l := minimalLauncher(false)
	for _, slug := range []string{"ceo", "eng", "pm"} {
		t.Run(slug, func(t *testing.T) {
			if got := l.headlessClaudeModel(slug); got != "claude-sonnet-4-6" {
				t.Fatalf("slug=%q opusCEO=false: want claude-sonnet-4-6, got %q", slug, got)
			}
		})
	}
}

// TestHeadlessClaudeModel_OpusForLeadOnly verifies that only the lead (CEO)
// gets upgraded to Opus when opusCEO is true; non-lead agents stay on Sonnet.
func TestHeadlessClaudeModel_OpusForLeadOnly(t *testing.T) {
	l := minimalLauncher(true)
	tests := []struct {
		slug string
		want string
	}{
		{"ceo", "claude-opus-4-6"},
		{"eng", "claude-sonnet-4-6"},
		{"pm", "claude-sonnet-4-6"},
	}
	for _, tc := range tests {
		t.Run(tc.slug, func(t *testing.T) {
			if got := l.headlessClaudeModel(tc.slug); got != tc.want {
				t.Fatalf("slug=%q opusCEO=true: want %q, got %q", tc.slug, tc.want, got)
			}
		})
	}
}

// TestHeadlessClaudeModel_CustomLeadSlug verifies model selection when the
// pack defines a non-"ceo" lead slug.
// brokerStatePath is redirected to an empty temp dir so officeMembersSnapshot()
// falls through to the pack definition instead of loading live state.
func TestHeadlessClaudeModel_CustomLeadSlug(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	l := &Launcher{
		pack: &agent.PackDefinition{
			LeadSlug: "captain",
			Agents: []agent.AgentConfig{
				{Slug: "captain", Name: "Captain"},
				{Slug: "crew", Name: "Crew"},
			},
		},
		opusCEO:         true,
		headlessWorkers: make(map[string]bool),
		headlessActive:  make(map[string]*headlessCodexActiveTurn),
		headlessQueues:  make(map[string][]headlessCodexTurn),
	}

	tests := []struct {
		slug string
		want string
	}{
		{"captain", "claude-opus-4-6"},
		{"crew", "claude-sonnet-4-6"},
	}
	for _, tc := range tests {
		t.Run(tc.slug, func(t *testing.T) {
			if got := l.headlessClaudeModel(tc.slug); got != tc.want {
				t.Fatalf("slug=%q: want %q, got %q", tc.slug, tc.want, got)
			}
		})
	}
}

func TestHeadlessClaudeModel_UsesPerAgentProviderModelWhenPresent(t *testing.T) {
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

	if got := l.headlessClaudeModel("eng"); got != "claude-haiku-4-5" {
		t.Fatalf("expected per-agent provider model override, got %q", got)
	}
}

func TestHeadlessClaudeMaxTurns_UsesReducedBudgets(t *testing.T) {
	l := minimalLauncher(false)

	if got := l.headlessClaudeMaxTurns("ceo"); got != "12" {
		t.Fatalf("ceo max turns = %q, want 12", got)
	}
	for _, slug := range []string{"eng", "pm"} {
		if got := l.headlessClaudeMaxTurns(slug); got != "12" {
			t.Fatalf("%s max turns = %q, want 12", slug, got)
		}
	}
}

// ─── runHeadlessClaudeTurn: no --resume flag in fresh sessions ────────────

// TestRunHeadlessClaudeTurn_NoResumeFlag verifies that the command assembled
// for a fresh (non-resumed) session does NOT contain --resume.
//
// We intercept headlessClaudeCommandContext to record the argv before any
// process is started. The binary is pointed at /bin/true so the process exits
// cleanly; the function will fail at JSON parsing (no output), but the
// captured args are all we need.
func TestRunHeadlessClaudeTurn_NoResumeFlag(t *testing.T) {
	// Redirect broker state to an isolated temp dir.
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	origCommandContext := headlessClaudeCommandContext
	origLookPath := headlessClaudeLookPath
	defer func() {
		headlessClaudeCommandContext = origCommandContext
		headlessClaudeLookPath = origLookPath
	}()

	var capturedArgs []string

	// Simulate claude found on PATH.
	headlessClaudeLookPath = func(file string) (string, error) { return "/bin/true", nil }

	// Intercept command creation: record the args then delegate to a real
	// exec.Cmd pointing at /bin/true so Start()/Wait() succeed trivially.
	headlessClaudeCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		capturedArgs = append(capturedArgs, args...)
		return exec.CommandContext(ctx, "/bin/true")
	}

	b := NewBroker()
	l := minimalLauncher(false)
	l.broker = b
	l.cwd = tmpDir

	// Write a valid (empty) MCP config so ensureAgentMCPConfig succeeds.
	mcpPath := filepath.Join(tmpDir, "mcp.json")
	_ = os.WriteFile(mcpPath, []byte(`{"mcpServers":{}}`), 0o600)
	l.mcpConfig = mcpPath

	// The function returns a parse error because /bin/true produces no JSON.
	// That is expected; we only care about capturedArgs.
	_ = l.runHeadlessClaudeTurn(context.Background(), "eng", "do the thing")

	if len(capturedArgs) == 0 {
		t.Fatal("no args captured; headlessClaudeCommandContext hook was not called")
	}
	for _, arg := range capturedArgs {
		if arg == "--resume" {
			t.Fatalf("--resume must not appear in a fresh session, got args: %v", capturedArgs)
		}
	}
}

func TestRunHeadlessClaudeTurnUsesChannelPrimaryLinkedRepoWorkspaceAndScopedMCP(t *testing.T) {
	recordFile := filepath.Join(t.TempDir(), "headless-claude-record.jsonl")
	channelWorkspace := t.TempDir()
	repoRoot := t.TempDir()
	otherRepo := t.TempDir()
	initUsableGitWorktree(t, channelWorkspace)
	initUsableGitWorktree(t, otherRepo)

	customPath := filepath.Join(t.TempDir(), "custom-mcp.json")
	customConfig := `{"mcpServers":{
		"serena":{"command":"uvx","args":["serena"]},
		"filesystem":{"command":"npx","args":["-y","@modelcontextprotocol/server-filesystem","<REPOS_ROOT>"]},
		"megamemory-linked":{"command":"npx","args":["-y","megamemory"],"env":{"MEGAMEMORY_DB_PATH":"` + filepath.ToSlash(filepath.Join(channelWorkspace, ".megamemory", "knowledge.db")) + `"}},
		"megamemory-other":{"command":"npx","args":["-y","megamemory"],"env":{"MEGAMEMORY_DB_PATH":"` + filepath.ToSlash(filepath.Join(otherRepo, ".megamemory", "knowledge.db")) + `"}}
	}}`
	if err := os.WriteFile(customPath, []byte(customConfig), 0o600); err != nil {
		t.Fatalf("write custom MCP config: %v", err)
	}
	t.Setenv("WUPHF_CUSTOM_MCP_CONFIG_PATH", customPath)

	origCommandContext := headlessClaudeCommandContext
	origLookPath := headlessClaudeLookPath
	defer func() {
		headlessClaudeCommandContext = origCommandContext
		headlessClaudeLookPath = origLookPath
	}()
	headlessClaudeLookPath = func(file string) (string, error) { return "/usr/bin/claude", nil }
	headlessClaudeCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmdArgs := []string{"-test.run=TestHeadlessClaudeCompatibleHelperProcess", "--", name}
		cmdArgs = append(cmdArgs, args...)
		return exec.CommandContext(ctx, os.Args[0], cmdArgs...)
	}

	t.Setenv("GO_WANT_HEADLESS_CLAUDE_COMPAT_HELPER_PROCESS", "1")
	t.Setenv("HEADLESS_CODEX_RECORD_FILE", recordFile)
	t.Setenv("HOME", t.TempDir())

	b := NewBroker()
	ensureTestMemberAccess(b, "launch-ops", "eng", "Engineer")
	b.mu.Lock()
	if !b.upsertLinkedRepoForChannelLocked("launch-ops", channelWorkspace, "human_message", "you") {
		b.mu.Unlock()
		t.Fatal("expected linked repo upsert")
	}
	b.mu.Unlock()

	l := minimalLauncher(false)
	l.broker = b
	l.cwd = repoRoot

	if err := l.runHeadlessClaudeTurn(context.Background(), "eng", "do the thing", "launch-ops"); err != nil {
		t.Fatalf("runHeadlessClaudeTurn: %v", err)
	}

	record := readHeadlessCodexRecord(t, recordFile)
	if record.Name != "claude" {
		t.Fatalf("expected claude command, got %q", record.Name)
	}
	if !samePath(record.Dir, channelWorkspace) {
		t.Fatalf("expected command dir %q, got %q", channelWorkspace, record.Dir)
	}
	if !containsEnv(record.Env, "WUPHF_CHANNEL=launch-ops") {
		t.Fatalf("expected scoped channel env, got %#v", record.Env)
	}
	if got := envValue(record.Env, "WUPHF_WORKSPACE_PATH"); !samePath(got, channelWorkspace) {
		t.Fatalf("expected workspace env %q, got %#v", channelWorkspace, record.Env)
	}
	mcpConfigPath := argValue(record.Args, "--mcp-config")
	if strings.TrimSpace(mcpConfigPath) == "" {
		t.Fatalf("expected --mcp-config arg, got %#v", record.Args)
	}
	data, err := os.ReadFile(mcpConfigPath)
	if err != nil {
		t.Fatalf("read scoped MCP config: %v", err)
	}
	var cfg struct {
		MCPServers map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal scoped MCP config: %v", err)
	}
	if _, ok := cfg.MCPServers["megamemory-linked"]; !ok {
		t.Fatalf("expected linked megamemory server, got %v", mapKeys(cfg.MCPServers))
	}
	if _, ok := cfg.MCPServers["megamemory-other"]; ok {
		t.Fatalf("did not expect unrelated megamemory server, got %v", mapKeys(cfg.MCPServers))
	}
}

func TestBuildHeadlessClaudeEnvForRuntimeAddsGitBashOnWindowsWhenMissing(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("git-bash bootstrap only applies on Windows")
	}

	oldResolveGitBash := headlessClaudeResolveGitBash
	headlessClaudeResolveGitBash = func() string { return `C:\Program Files\Git\bin\bash.exe` }
	defer func() { headlessClaudeResolveGitBash = oldResolveGitBash }()

	oldGitBashPath, hadGitBashPath := os.LookupEnv("CLAUDE_CODE_GIT_BASH_PATH")
	_ = os.Unsetenv("CLAUDE_CODE_GIT_BASH_PATH")
	defer func() {
		if hadGitBashPath {
			_ = os.Setenv("CLAUDE_CODE_GIT_BASH_PATH", oldGitBashPath)
			return
		}
		_ = os.Unsetenv("CLAUDE_CODE_GIT_BASH_PATH")
	}()

	l := minimalLauncher(false)
	l.broker = NewBroker()
	env := l.buildHeadlessClaudeEnvForRuntime("eng", provider.KindClaudeCode, "", "")

	if got := envValue(env, "CLAUDE_CODE_GIT_BASH_PATH"); got != `C:\Program Files\Git\bin\bash.exe` {
		t.Fatalf("expected CLAUDE_CODE_GIT_BASH_PATH to be injected, got %#v", env)
	}
}

func TestBuildHeadlessClaudeEnvForRuntimeReplacesEmptyGitBashPathOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("git-bash bootstrap only applies on Windows")
	}

	oldResolveGitBash := headlessClaudeResolveGitBash
	headlessClaudeResolveGitBash = func() string { return `C:\Program Files\Git\bin\bash.exe` }
	defer func() { headlessClaudeResolveGitBash = oldResolveGitBash }()

	oldGitBashPath, hadGitBashPath := os.LookupEnv("CLAUDE_CODE_GIT_BASH_PATH")
	_ = os.Setenv("CLAUDE_CODE_GIT_BASH_PATH", "   ")
	defer func() {
		if hadGitBashPath {
			_ = os.Setenv("CLAUDE_CODE_GIT_BASH_PATH", oldGitBashPath)
			return
		}
		_ = os.Unsetenv("CLAUDE_CODE_GIT_BASH_PATH")
	}()

	l := minimalLauncher(false)
	l.broker = NewBroker()
	env := l.buildHeadlessClaudeEnvForRuntime("eng", provider.KindClaudeCode, "", "")

	if got := envValue(env, "CLAUDE_CODE_GIT_BASH_PATH"); got != `C:\Program Files\Git\bin\bash.exe` {
		t.Fatalf("expected empty CLAUDE_CODE_GIT_BASH_PATH to be replaced, got %#v", env)
	}
}

// ─── MCP manifest: no-nex mode ────────────────────────────────────────────

// TestBuildMCPServerMap_NoNexExcludesNexServer verifies that when
// WUPHF_NO_NEX=true the built server map contains no "nex" entry, even if a
// non-empty API key is present.
func TestBuildMCPServerMap_NoNexExcludesNexServer(t *testing.T) {
	t.Setenv("WUPHF_NO_NEX", "true")
	// Provide a non-empty API key so we would enter the nex branch if WUPHF_NO_NEX
	// were not checked.
	t.Setenv("WUPHF_API_KEY", "test-key-12345")

	l := minimalLauncher(false)
	servers, err := l.buildMCPServerMap()
	if err != nil {
		t.Fatalf("buildMCPServerMap: %v", err)
	}
	if _, ok := servers["nex"]; ok {
		t.Fatalf("'nex' server must be absent when WUPHF_NO_NEX=true, got servers: %v", mapKeys(servers))
	}
	// wuphf-office must always be present regardless of no-nex mode.
	if _, ok := servers["wuphf-office"]; !ok {
		t.Fatalf("'wuphf-office' server must always be present, got servers: %v", mapKeys(servers))
	}
}

// TestEnsureAgentMCPConfig_NoNexEntryInWrittenFile verifies that the per-agent
// MCP config file written to disk contains no "nex" key when WUPHF_NO_NEX=true.
func TestEnsureAgentMCPConfig_NoNexEntryInWrittenFile(t *testing.T) {
	t.Setenv("WUPHF_NO_NEX", "true")
	t.Setenv("WUPHF_API_KEY", "test-key-12345")

	l := minimalLauncher(false)
	path, err := l.ensureAgentMCPConfig("ceo")
	if err != nil {
		t.Fatalf("ensureAgentMCPConfig: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read MCP config file: %v", err)
	}
	var cfg struct {
		MCPServers map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse MCP config: %v", err)
	}
	if _, hasNex := cfg.MCPServers["nex"]; hasNex {
		t.Fatalf("'nex' server must be absent in written MCP config when WUPHF_NO_NEX=true, got servers: %v", mapKeys(cfg.MCPServers))
	}
}

// TestBuildMCPServerMap_DoesNotForwardLegacyMemoryCredentials verifies that
// the office MCP server no longer forwards Nex memory credentials.
func TestBuildMCPServerMap_DoesNotForwardLegacyMemoryCredentials(t *testing.T) {
	t.Setenv("WUPHF_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	t.Setenv("WUPHF_NO_NEX", "")
	t.Setenv("WUPHF_MEMORY_BACKEND", "nex")
	t.Setenv("WUPHF_API_KEY", "test-key-12345")

	l := minimalLauncher(false)
	servers, err := l.buildMCPServerMap()
	if err != nil {
		t.Fatalf("buildMCPServerMap: %v", err)
	}
	entry, ok := servers["wuphf-office"]
	if !ok {
		t.Fatalf("'wuphf-office' server must be present, got servers: %v", mapKeys(servers))
	}
	server, ok := entry.(map[string]any)
	if !ok {
		t.Fatalf("expected wuphf-office entry to be an object, got %T", entry)
	}
	env, ok := server["env"].(map[string]string)
	if !ok {
		t.Fatalf("expected office env map, got %#v", server["env"])
	}
	if _, ok := env["WUPHF_API_KEY"]; ok {
		t.Fatalf("did not expect WUPHF_API_KEY on office server, got %#v", env)
	}
	if _, ok := env["NEX_API_KEY"]; ok {
		t.Fatalf("did not expect NEX_API_KEY on office server, got %#v", env)
	}
}

func TestBuildMCPServerMap_DoesNotForwardGBrainCredentials(t *testing.T) {
	t.Setenv("WUPHF_MEMORY_BACKEND", "gbrain")
	t.Setenv("WUPHF_OPENAI_API_KEY", "openai-test-key")

	l := minimalLauncher(false)
	servers, err := l.buildMCPServerMap()
	if err != nil {
		t.Fatalf("buildMCPServerMap: %v", err)
	}
	entry, ok := servers["wuphf-office"]
	if !ok {
		t.Fatalf("'wuphf-office' server must be present when GBrain is selected, got servers: %v", mapKeys(servers))
	}
	server, ok := entry.(map[string]any)
	if !ok {
		t.Fatalf("expected wuphf-office entry to be an object, got %T", entry)
	}
	env, ok := server["env"].(map[string]string)
	if !ok {
		t.Fatalf("expected office env map, got %#v", server["env"])
	}
	if _, ok := env["OPENAI_API_KEY"]; ok {
		t.Fatalf("did not expect OPENAI_API_KEY on office server, got %#v", env)
	}
}

// mapKeys returns the keys of map[string]V for human-readable error messages.
func mapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
