package team

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nex-crm/wuphf/internal/agent"
)

func TestNotificationTargetsForSkillUpdateSkipsOfficeWake(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "ceo", "CEO")
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	b.mu.Unlock()

	l := &Launcher{
		broker: b,
		pack: &agent.PackDefinition{
			LeadSlug: "ceo",
			Agents: []agent.AgentConfig{
				{Slug: "ceo", Name: "CEO"},
				{Slug: "builder", Name: "Builder"},
			},
		},
	}

	immediate, delayed := l.notificationTargetsForMessage(channelMessage{
		ID:        "msg-1",
		From:      "system",
		Channel:   "general",
		Kind:      "skill_update",
		Title:     "Skill Update",
		Content:   `Skill "contexto-workspace" created by @system`,
		Timestamp: "2026-04-19T20:00:00Z",
	})
	if len(immediate) != 0 || len(delayed) != 0 {
		t.Fatalf("expected skill_update to skip office wake, got immediate=%+v delayed=%+v", immediate, delayed)
	}
}

func TestPostMessageRejectsRawBroadcastMarkupFromAgent(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "estagiario", "Estagiario")
	b.mu.Unlock()

	_, err := b.PostMessage("estagiario", "general", `[team_broadcast slug="estagiario" content="aguardando repo"]`, nil, "msg-10")
	if err == nil {
		t.Fatal("expected raw broadcast markup to be rejected")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "non-substantive agent chatter") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPostMessageRejectsSecondAgentReplyWithoutNewExecutionTurn(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "ceo", "CEO")
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	b.mu.Unlock()

	root, err := b.PostMessage("you", "general", "Implemente o primeiro corte do broker.", nil, "")
	if err != nil {
		t.Fatalf("post human message: %v", err)
	}
	ceoReply, err := b.PostMessage("ceo", "general", "@builder faça o corte mínimo e me devolva o resultado nesta thread.", []string{"builder"}, root.ID)
	if err != nil {
		t.Fatalf("post CEO reply: %v", err)
	}
	if _, err := b.PostMessage("builder", "general", "Primeiro corte pronto.", nil, ceoReply.ID); err != nil {
		t.Fatalf("first builder reply: %v", err)
	}
	_, err = b.PostMessage("builder", "general", "Complementando: segue o mesmo status.", nil, ceoReply.ID)
	if err == nil {
		t.Fatal("expected second agent reply without new execution turn to be rejected")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "already replied") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAwaitingHumanInputFreezesThreadUntilHumanReplies(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "ceo", "CEO")
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	b.mu.Unlock()

	root, err := b.PostMessage("you", "general", "Quero a refatoração do slice do broker.", nil, "")
	if err != nil {
		t.Fatalf("post human message: %v", err)
	}
	ceoReply, err := b.PostMessage("ceo", "general", "@builder execute o primeiro corte assim que tiver o alvo.", []string{"builder"}, root.ID)
	if err != nil {
		t.Fatalf("post CEO reply: %v", err)
	}
	builderReply, err := b.PostMessage("builder", "general", "Preciso do caminho do repositório ou do arquivo exato antes de mexer no código.", nil, ceoReply.ID)
	if err != nil {
		t.Fatalf("post builder reply: %v", err)
	}

	_, err = b.PostMessage("ceo", "general", "Time, aguardem o @human mandar o repo.", nil, builderReply.ID)
	if err == nil {
		t.Fatal("expected agent-to-agent reply during awaiting-human latch to be rejected")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "awaiting human") {
		t.Fatalf("unexpected error: %v", err)
	}

	humanReply, err := b.PostMessage("you", "general", "Repo em <DUNDERIA_REPO>, arquivo internal/team/broker.go.", nil, root.ID)
	if err != nil {
		t.Fatalf("post human reply: %v", err)
	}
	if _, err := b.PostMessage("ceo", "general", "@builder alvo recebido; faça o corte agora.", []string{"builder"}, humanReply.ID); err != nil {
		t.Fatalf("expected CEO reply after human input to succeed, got %v", err)
	}
}
