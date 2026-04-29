package team

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPublishHeadlessFallbackReplySkipsInternalRuntimePayload(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	root, err := b.PostMessage("ceo", "general", "implemente o slice", []string{"builder"}, "")
	if err != nil {
		t.Fatalf("seed message: %v", err)
	}

	l := &Launcher{broker: b}
	notification := fmt.Sprintf(`Reply using team_broadcast with my_slug "builder" and channel "general" reply_to_id "%s". [WUPHF_REPLY_ROUTE channel="general" reply_to_id="%s"]`, root.ID, root.ID)
	before := len(b.AllMessages())

	l.publishHeadlessFallbackReply("builder", notification, `{"type":"item.completed","item":{"type":"mcp_tool_call","server":"filesystem","tool":"read_multiple_files"}}`, time.Now().UTC())

	if got := len(b.AllMessages()); got != before {
		t.Fatalf("expected internal runtime payload to be dropped, got %d messages want %d", got, before)
	}
}

func TestPublishHeadlessFallbackReplySkipsRawToolFailureText(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	root, err := b.PostMessage("ceo", "general", "implemente o slice", []string{"builder"}, "")
	if err != nil {
		t.Fatalf("seed message: %v", err)
	}

	l := &Launcher{broker: b}
	notification := fmt.Sprintf(`Reply using team_broadcast with my_slug "builder" and channel "general" reply_to_id "%s". [WUPHF_REPLY_ROUTE channel="general" reply_to_id="%s"]`, root.ID, root.ID)
	before := len(b.AllMessages())

	l.publishHeadlessFallbackReply("builder", notification, "read_mcp_resource\nErro", time.Now().UTC())

	if got := len(b.AllMessages()); got != before {
		t.Fatalf("expected raw tool failure text to be dropped, got %d messages want %d", got, before)
	}
}

func TestPublishHeadlessFallbackReplyKeepsHumanReadableToolFailureSummary(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	root, err := b.PostMessage("ceo", "general", "implemente o slice", []string{"builder"}, "")
	if err != nil {
		t.Fatalf("seed message: %v", err)
	}

	l := &Launcher{broker: b}
	notification := fmt.Sprintf(`Reply using team_broadcast with my_slug "builder" and channel "general" reply_to_id "%s". [WUPHF_REPLY_ROUTE channel="general" reply_to_id="%s"]`, root.ID, root.ID)
	before := len(b.AllMessages())

	l.publishHeadlessFallbackReply("builder", notification, "read_mcp_resource falhou no filesystem; segui por shell local e confirmei que o repo esta acessivel.", time.Now().UTC())

	if got := len(b.AllMessages()); got != before+1 {
		t.Fatalf("expected human-readable tool failure summary to be published, got %d messages want %d", got, before+1)
	}
	last := b.AllMessages()[len(b.AllMessages())-1]
	if last.From != "builder" || last.ReplyTo != root.ID {
		t.Fatalf("unexpected fallback reply %+v", last)
	}
}

func TestPublishHeadlessFallbackReplySkipsMixedRuntimePayloadAndProse(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	root, err := b.PostMessage("ceo", "general", "implemente o slice", []string{"builder"}, "")
	if err != nil {
		t.Fatalf("seed message: %v", err)
	}

	l := &Launcher{broker: b}
	notification := fmt.Sprintf(`Reply using team_broadcast with my_slug "builder" and channel "general" reply_to_id "%s". [WUPHF_REPLY_ROUTE channel="general" reply_to_id="%s"]`, root.ID, root.ID)
	before := len(b.AllMessages())

	mixed := "{\"channel\":\"general\",\"my_slug\":\"builder\",\"reply_to_id\":\"" + root.ID + "\",\"content\":\"payload bruto\"}\n\nNo further action needed here."
	l.publishHeadlessFallbackReply("builder", notification, mixed, time.Now().UTC())

	if got := len(b.AllMessages()); got != before {
		t.Fatalf("expected mixed runtime payload to be dropped, got %d messages want %d", got, before)
	}
}

func TestPublishHeadlessFallbackReplySkipsAgentSelfTalk(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	root, err := b.PostMessage("ceo", "general", "implemente o slice", []string{"builder"}, "")
	if err != nil {
		t.Fatalf("seed message: %v", err)
	}

	l := &Launcher{broker: b}
	notification := fmt.Sprintf(`Reply using team_broadcast with my_slug "builder" and channel "general" reply_to_id "%s". [WUPHF_REPLY_ROUTE channel="general" reply_to_id="%s"]`, root.ID, root.ID)
	before := len(b.AllMessages())

	selfTalk := "Vou abrir os arquivos agora.\n\nVou aplicar o patch em seguida.\n\nVou validar todos os pontos depois.\n\nVou registrar o andamento no canal."
	l.publishHeadlessFallbackReply("builder", notification, selfTalk, time.Now().UTC())

	if got := len(b.AllMessages()); got != before {
		t.Fatalf("expected agent self-talk to be dropped, got %d messages want %d", got, before)
	}
}

func TestPublishHeadlessFallbackReplyPostsTaskFollowUpReplyFromExecutionPacket(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	ensureTestMemberAccess(b, "convenios-web-azure", "ceo", "CEO")
	root, err := b.PostMessage("you", "convenios-web-azure", "@ceo onde está o arquivo gerado?", []string{"ceo"}, "")
	if err != nil {
		t.Fatalf("seed message: %v", err)
	}

	l := &Launcher{broker: b}
	notification := l.buildTaskExecutionPacket("ceo", officeActionLog{
		Kind:  "watchdog_alert",
		Actor: "watchdog",
	}, teamTask{
		ID:         "task-855",
		Channel:    "convenios-web-azure",
		Title:      "Reply to pending message from @you",
		Owner:      "ceo",
		Status:     "in_progress",
		ThreadID:   root.ID,
		TaskType:   "follow_up",
		PipelineID: "follow_up",
	}, "Watchdog reminder")
	before := len(b.AllMessages())

	l.publishHeadlessFallbackReply("ceo", notification, "Preciso do diretório de saída para localizar o arquivo gerado.", time.Now().UTC())

	if got := len(b.AllMessages()); got != before+1 {
		t.Fatalf("expected follow-up reply to be published, got %d messages want %d", got, before+1)
	}
	last := b.AllMessages()[len(b.AllMessages())-1]
	if last.From != "ceo" || last.Channel != "convenios-web-azure" || last.ReplyTo != root.ID {
		t.Fatalf("unexpected follow-up reply %+v", last)
	}
}

func TestPublishHeadlessFallbackReplyPostsSingleResumePacketReply(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	ensureTestMemberAccess(b, "convenios-web-azure", "ceo", "CEO")
	root, err := b.PostMessage("you", "convenios-web-azure", "@ceo onde está o arquivo gerado?", []string{"ceo"}, "")
	if err != nil {
		t.Fatalf("seed message: %v", err)
	}

	l := &Launcher{broker: b}
	packet := buildResumePacket("ceo", nil, []channelMessage{root})
	before := len(b.AllMessages())

	l.publishHeadlessFallbackReply("ceo", packet, "Preciso do diretório de saída para localizar o arquivo gerado.", time.Now().UTC())

	if got := len(b.AllMessages()); got != before+1 {
		t.Fatalf("expected single-message resume reply to be published, got %d messages want %d", got, before+1)
	}
	last := b.AllMessages()[len(b.AllMessages())-1]
	if last.From != "ceo" || last.Channel != "convenios-web-azure" || last.ReplyTo != root.ID {
		t.Fatalf("unexpected resume fallback reply %+v", last)
	}
}

func TestPublishHeadlessFallbackReplyRetriesAtThreadRootOnReplyToMismatch(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	ensureTestMemberAccess(b, "convenios-web-azure", "ceo", "CEO")
	root, err := b.PostMessage("you", "convenios-web-azure", "@ceo onde está o arquivo gerado?", []string{"ceo"}, "")
	if err != nil {
		t.Fatalf("seed root message: %v", err)
	}
	followUp, err := b.PostMessage("you", "convenios-web-azure", "@ceo responda", []string{"ceo"}, root.ID)
	if err != nil {
		t.Fatalf("seed follow-up message: %v", err)
	}

	l := &Launcher{broker: b}
	notification := fmt.Sprintf(`Reply using team_broadcast with my_slug "ceo" and channel "convenios-web-azure" reply_to_id "%s". [WUPHF_REPLY_ROUTE channel="convenios-web-azure" reply_to_id="%s"]`, followUp.ID, followUp.ID)
	before := len(b.AllMessages())

	l.publishHeadlessFallbackReply("ceo", notification, "O relatório da task-782 foi entregue e aprovado internamente.", time.Now().UTC())

	if got := len(b.AllMessages()); got != before+1 {
		t.Fatalf("expected mismatch retry reply to be published, got %d messages want %d", got, before+1)
	}
	last := b.AllMessages()[len(b.AllMessages())-1]
	if last.From != "ceo" || last.Channel != "convenios-web-azure" || last.ReplyTo != root.ID {
		t.Fatalf("expected retry to publish at thread root %+v", last)
	}
}

func TestPublishHeadlessFallbackReplyPostsNeutralFallbackOnTaskStateClaimRejection(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	ensureTestMemberAccess(b, "migracao-convenios", "ceo", "CEO")
	root, err := b.PostMessage("you", "migracao-convenios", "o que esta sendo feito agora?", nil, "")
	if err != nil {
		t.Fatalf("seed human message: %v", err)
	}
	task := teamTask{
		ID:      "task-2775",
		Channel: "migracao-convenios",
		Title:   "Diagnosticar 500 failed to manage task worktree da task-1629",
		Owner:   "ceo",
		Status:  "open",
	}
	b.mu.Lock()
	b.tasks = append(b.tasks, task)
	b.rebuildTaskIndexesLocked()
	b.mu.Unlock()

	l := &Launcher{broker: b}
	notification := fmt.Sprintf(`Reply using team_broadcast with my_slug "ceo" and channel "migracao-convenios" reply_to_id "%s". [WUPHF_REPLY_ROUTE channel="migracao-convenios" reply_to_id="%s"]`, root.ID, root.ID)
	before := len(b.AllMessages())

	l.publishHeadlessFallbackReply("ceo", notification, "A task-2775 esta concluida; agora vou seguir com a proxima etapa.", time.Now().UTC())

	if got := len(b.AllMessages()); got != before+1 {
		t.Fatalf("expected neutral fallback reply to be published, got %d messages want %d", got, before+1)
	}
	last := b.AllMessages()[len(b.AllMessages())-1]
	if last.From != "ceo" || last.Channel != "migracao-convenios" || last.ReplyTo != root.ID {
		t.Fatalf("unexpected neutral fallback reply %+v", last)
	}
	if !strings.Contains(last.Content, "A resposta automatica anterior foi bloqueada") {
		t.Fatalf("expected neutral fallback explanation, got %q", last.Content)
	}
	if !strings.Contains(last.Content, "task-2775: Diagnosticar 500 failed to manage task worktree da task-1629") {
		t.Fatalf("expected live task context, got %q", last.Content)
	}
}

func TestShouldPublishHeadlessErrorFallbackSuppressesProviderFailureText(t *testing.T) {
	if shouldPublishHeadlessErrorFallback("You've hit your limit · resets 12:30am (America/Sao_Paulo)") {
		t.Fatal("expected provider failure fallback to be suppressed")
	}
	if !shouldPublishHeadlessErrorFallback("Consegui validar o patch e o próximo passo é abrir o PR.") {
		t.Fatal("expected substantive fallback text to remain publishable")
	}
}
