package team

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nex-crm/wuphf/internal/agent"
	"github.com/nex-crm/wuphf/internal/provider"
)

func TestBuildHeadlessCodexPromptIncludesRuntimeGuidance(t *testing.T) {
	got := buildHeadlessCodexPrompt("system prompt", "do the work")
	for _, want := range []string{
		"Do not use read_mcp_resource",
		"filesystem file tools first",
		"blocked by policy",
		"clean no-profile PowerShell environment",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected prompt to include %q, got %q", want, got)
		}
	}
}

func TestBuildPromptIncludesFilesystemFallbackGuidance(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	l := minimalLauncher(false)
	got := l.buildPrompt("builder")
	if !strings.Contains(got, "missing filesystem MCP server") {
		t.Fatalf("expected prompt to include filesystem fallback guidance, got %q", got)
	}
}

func TestUpdateHeadlessProgressCreatesOperationalWatchdogAlertAndAutomationNotice(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	b := NewBroker()
	ensureTestMemberAccess(b, "general", "ceo", "CEO")
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	b.mu.Lock()
	b.tasks = append(b.tasks, teamTask{
		ID:       "task-77",
		Channel:  "general",
		Title:    "Patch legacy lane",
		Owner:    "builder",
		Status:   "in_progress",
		ThreadID: "msg-1",
	})
	b.mu.Unlock()

	l := &Launcher{
		broker: b,
		pack:   agent.GetPack("founding-team"),
	}

	l.updateHeadlessProgress("builder", "error", "error", "blocked by policy", headlessProgressMetrics{})

	b.mu.Lock()
	alerts := append([]watchdogAlert(nil), b.watchdogs...)
	b.mu.Unlock()
	if len(alerts) != 1 {
		t.Fatalf("expected one watchdog alert, got %+v", alerts)
	}
	if alerts[0].Kind != "agent_runtime_blocked" || alerts[0].Owner != "ceo" {
		t.Fatalf("unexpected watchdog alert %+v", alerts[0])
	}

	msgs := b.AllMessages()
	foundAutomation := false
	for _, msg := range msgs {
		if msg.Kind != "automation" {
			continue
		}
		if strings.Contains(msg.Content, "@builder hit an operational runtime block") && containsSlug(msg.Tagged, "ceo") {
			foundAutomation = true
			break
		}
	}
	if !foundAutomation {
		t.Fatalf("expected automation notice to tag ceo, got %+v", msgs)
	}
}

func TestUpdateHeadlessProgressSkipsWatchdogForRecoverableFilesystemMCPIssue(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	b := NewBroker()
	ensureTestMemberAccess(b, "general", "ceo", "CEO")
	ensureTestMemberAccess(b, "general", "builder", "Builder")

	l := &Launcher{
		broker: b,
		pack:   agent.GetPack("founding-team"),
		cwd:    t.TempDir(),
	}

	l.updateHeadlessProgress("builder", "error", "error", "resources/list failed for `filesystem`: Mcp error: -32601: Method not found", headlessProgressMetrics{})

	b.mu.Lock()
	alerts := append([]watchdogAlert(nil), b.watchdogs...)
	b.mu.Unlock()
	if len(alerts) != 0 {
		t.Fatalf("expected no watchdog alert for recoverable filesystem MCP issue, got %+v", alerts)
	}
	for _, msg := range b.AllMessages() {
		if msg.Kind == "automation" && strings.Contains(msg.Content, "operational runtime block") {
			t.Fatalf("expected no automation watchdog notice, got %+v", msg)
		}
	}
}

func TestUpdateHeadlessProgressCreatesWatchdogForFilesystemMCPIssueWithoutWorkspaceFallback(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	b := NewBroker()
	ensureTestMemberAccess(b, "general", "ceo", "CEO")
	ensureTestMemberAccess(b, "general", "builder", "Builder")

	l := &Launcher{
		broker: b,
		pack:   agent.GetPack("founding-team"),
		cwd:    filepath.Join(t.TempDir(), "missing-workspace"),
	}

	l.updateHeadlessProgress("builder", "error", "error", "unknown mcp server: filesystem", headlessProgressMetrics{})

	b.mu.Lock()
	alerts := append([]watchdogAlert(nil), b.watchdogs...)
	b.mu.Unlock()
	if len(alerts) != 1 || alerts[0].Kind != "agent_runtime_blocked" {
		t.Fatalf("expected watchdog alert when no local fallback exists, got %+v", alerts)
	}
}

func TestUpdateHeadlessProgressSkipsWatchdogForRecoverableProviderFallbackIssue(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	b := NewBroker()
	ensureTestMemberAccess(b, "general", "ceo", "CEO")
	ensureTestMemberAccess(b, "general", "builder", "Builder")

	oldAvailable := headlessRuntimeProviderAvailable
	headlessRuntimeProviderAvailable = func(kind string) bool {
		return normalizeProviderKind(kind) == provider.KindClaudeCode
	}
	defer func() { headlessRuntimeProviderAvailable = oldAvailable }()

	l := &Launcher{
		broker: b,
		pack:   agent.GetPack("founding-team"),
		cwd:    t.TempDir(),
	}
	l.provider = provider.KindCodex

	l.updateHeadlessProgress("builder", "error", "error", `codex not found: exec: "codex": executable file not found in %PATH%`, headlessProgressMetrics{})

	b.mu.Lock()
	alerts := append([]watchdogAlert(nil), b.watchdogs...)
	b.mu.Unlock()
	if len(alerts) != 0 {
		t.Fatalf("expected no watchdog alert for recoverable provider failure, got %+v", alerts)
	}
	for _, msg := range b.AllMessages() {
		if msg.Kind == "automation" && strings.Contains(msg.Content, "operational runtime block") {
			t.Fatalf("expected no automation watchdog notice, got %+v", msg)
		}
	}
}

func TestUpdateHeadlessProgressResolvesOperationalWatchdogAlertWhenAgentRecovers(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	b := NewBroker()
	if _, _, err := b.CreateWatchdogAlert("agent_runtime_blocked", "general", "agent_runtime", "builder|general", "ceo", "runtime blocked"); err != nil {
		t.Fatalf("seed watchdog alert: %v", err)
	}

	l := &Launcher{broker: b}
	l.updateHeadlessProgress("builder", "idle", "idle", "reply ready", headlessProgressMetrics{TotalMs: 10})

	b.mu.Lock()
	alerts := append([]watchdogAlert(nil), b.watchdogs...)
	b.mu.Unlock()
	if len(alerts) != 1 || alerts[0].Status != "resolved" {
		t.Fatalf("expected watchdog alert to resolve, got %+v", alerts)
	}
}

func TestUpdateHeadlessProgressKeepsOperationalWatchdogAlertOpenDuringActiveRecoveryNoise(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	b := NewBroker()
	if _, _, err := b.CreateWatchdogAlert("agent_runtime_blocked", "general", "agent_runtime", "builder|general", "ceo", "runtime blocked"); err != nil {
		t.Fatalf("seed watchdog alert: %v", err)
	}

	l := &Launcher{broker: b}
	l.updateHeadlessProgress("builder", "active", "thinking", "planning next step", headlessProgressMetrics{})

	b.mu.Lock()
	alerts := append([]watchdogAlert(nil), b.watchdogs...)
	b.mu.Unlock()
	if len(alerts) != 1 || alerts[0].Status != "active" {
		t.Fatalf("expected watchdog alert to stay active during non-idle recovery noise, got %+v", alerts)
	}
}
