package team

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPostMessageDeduplicatesExactRepeatedAgentReply(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	b.mu.Lock()
	b.channels = append(b.channels, teamChannel{
		Slug:    "convenios-legacy",
		Name:    "Convenios Legacy",
		Members: []string{"reviewer"},
	})
	b.mu.Unlock()

	content := "Sem patch novo para reavaliar; o parecer continua em vermelho até existir diff real no worktree."
	first, err := b.PostMessage("reviewer", "convenios-legacy", content, nil, "msg-2048")
	if err != nil {
		t.Fatalf("first PostMessage: %v", err)
	}
	second, err := b.PostMessage("reviewer", "convenios-legacy", content, []string{"builder"}, "msg-2048")
	if err != nil {
		t.Fatalf("second PostMessage: %v", err)
	}

	if second.ID != first.ID {
		t.Fatalf("duplicate message returned %q, want existing %q", second.ID, first.ID)
	}
	if got := len(b.ChannelMessages("convenios-legacy")); got != 1 {
		t.Fatalf("expected 1 stored message in channel after duplicate post, got %d", got)
	}
}

func TestPostMessageDeduplicatesRepeatedCoordinationGuidance(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	b.mu.Lock()
	b.channels = append(b.channels, teamChannel{
		Slug:    "convenios-legacy",
		Name:    "Convenios Legacy",
		Members: []string{"reviewer"},
	})
	b.mu.Unlock()

	firstContent := "Gate de revisao do segundo arquivo: troque DATALENGTH(Anexo) por TArquivoStorageCompat, mantenha a assinatura publica, degrade qualquer falha para false e preserve BNBInterno."
	first, err := b.PostMessage("reviewer", "convenios-legacy", firstContent, nil, "msg-3000")
	if err != nil {
		t.Fatalf("first PostMessage: %v", err)
	}
	secondContent := "Gate de revisao do segundo arquivo permanece: troque DATALENGTH(Anexo) por TArquivoStorageCompat, mantenha assinatura publica, degrade qualquer falha para false e preserve BNBInterno."
	second, err := b.PostMessage("reviewer", "convenios-legacy", secondContent, []string{"builder"}, "msg-3001")
	if err != nil {
		t.Fatalf("second PostMessage: %v", err)
	}

	if second.ID != first.ID {
		t.Fatalf("duplicate guidance returned %q, want existing %q", second.ID, first.ID)
	}
	if got := len(b.ChannelMessages("convenios-legacy")); got != 1 {
		t.Fatalf("expected 1 stored message in channel after repeated guidance, got %d", got)
	}
}

func TestPostMessageRejectsContradictoryTaskUnblockedClaim(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	b.mu.Lock()
	b.channels = append(b.channels, teamChannel{
		Slug:    "convenios-legacy",
		Name:    "Convenios Legacy",
		Members: []string{"ceo", "operator", "builder"},
	})
	b.tasks = append(b.tasks, teamTask{
		ID:        "task-2",
		Channel:   "convenios-legacy",
		Title:     "Retomar slice legado",
		Owner:     "builder",
		Status:    "blocked",
		Blocked:   true,
		CreatedBy: "ceo",
		CreatedAt: time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	b.mu.Unlock()

	_, err := b.PostMessage("ceo", "convenios-legacy", "@builder a task-2 esta oficialmente destravada e de volta para in_progress.", nil, "")
	if err == nil {
		t.Fatal("expected contradictory task-state claim to be rejected")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "mutate the task before announcing") {
		t.Fatalf("unexpected error: %v", err)
	}
}
