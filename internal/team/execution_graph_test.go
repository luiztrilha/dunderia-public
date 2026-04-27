package team

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPostMessageCreatesRootExecutionNodeForHumanRequest(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "ceo", "CEO")
	b.mu.Unlock()

	msg, err := b.PostMessage("you", "general", "Preciso de um plano claro para migrar o fluxo de convenio sem perder o historico do canal.", nil, "")
	if err != nil {
		t.Fatalf("post human message: %v", err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.executionNodes) != 1 {
		t.Fatalf("expected 1 execution node, got %+v", b.executionNodes)
	}
	node := b.executionNodes[0]
	if node.TriggerMessageID != msg.ID || node.RootMessageID != msg.ID {
		t.Fatalf("expected root node tied to %s, got %+v", msg.ID, node)
	}
	if node.OwnerAgent != "ceo" || node.Status != "pending" {
		t.Fatalf("expected pending CEO node, got %+v", node)
	}
}

func TestPostMessageCreatesChildExecutionNodeForTaggedAgent(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "ceo", "CEO")
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	b.mu.Unlock()

	root, err := b.PostMessage("you", "general", "Quero o corte minimo implementado sem perder a causalidade das respostas.", nil, "")
	if err != nil {
		t.Fatalf("post human message: %v", err)
	}
	ceoReply, err := b.PostMessage("ceo", "general", "@builder implemente o primeiro corte do broker causal e me devolva o recibo.", []string{"builder"}, root.ID)
	if err != nil {
		t.Fatalf("post CEO reply: %v", err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.executionNodes) != 2 {
		t.Fatalf("expected root + child execution nodes, got %+v", b.executionNodes)
	}
	rootNode := b.executionNodes[0]
	childNode := b.executionNodes[1]
	if rootNode.Status != "answered" || rootNode.ResolvedByMessageID != ceoReply.ID {
		t.Fatalf("expected root node answered by CEO reply, got %+v", rootNode)
	}
	if childNode.ParentNodeID != rootNode.ID {
		t.Fatalf("expected child parent %s, got %+v", rootNode.ID, childNode)
	}
	if childNode.OwnerAgent != "builder" || childNode.TriggerMessageID != ceoReply.ID || childNode.RootMessageID != root.ID {
		t.Fatalf("expected builder child node tied to CEO reply/root, got %+v", childNode)
	}
	if childNode.Status != "pending" {
		t.Fatalf("expected pending child node, got %+v", childNode)
	}
}

func TestPostMessageDoesNotCreateDuplicateOpenExecutionNodeForSameOwnerInRootThread(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "ceo", "CEO")
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	b.mu.Unlock()

	root, err := b.PostMessage("you", "general", "Verifique quem está online.", nil, "")
	if err != nil {
		t.Fatalf("post human message: %v", err)
	}
	firstPing, err := b.PostMessage("ceo", "general", "@builder confirme seu status.", []string{"builder"}, root.ID)
	if err != nil {
		t.Fatalf("post CEO reply: %v", err)
	}
	if _, err := b.PostMessage("ceo", "general", "@builder reforcando a confirmacao de status.", []string{"builder"}, firstPing.ID); err != nil {
		t.Fatalf("post second CEO reply: %v", err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	count := 0
	for _, node := range b.executionNodes {
		if node.OwnerAgent == "builder" && node.RootMessageID == root.ID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected single open builder execution node for root thread, got %+v", b.executionNodes)
	}
}

func TestPostMessageRejectsAgentReplyOutsideExpectedExecutionThread(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "ceo", "CEO")
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	b.mu.Unlock()

	root, err := b.PostMessage("you", "general", "Preciso do patch causal no broker.", nil, "")
	if err != nil {
		t.Fatalf("post human message: %v", err)
	}
	if _, err := b.PostMessage("ceo", "general", "@builder responda nesta thread com o patch.", []string{"builder"}, root.ID); err != nil {
		t.Fatalf("post CEO reply: %v", err)
	}
	_, err = b.PostMessage("builder", "general", "Implementei o patch.", nil, "")
	if err == nil {
		t.Fatal("expected builder reply without reply_to to be rejected")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "reply_to") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPostMessageResolvesExecutionNodeWhenAgentRepliesInExpectedThread(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "ceo", "CEO")
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	b.mu.Unlock()

	root, err := b.PostMessage("you", "general", "Preciso da primeira fatia do grafo causal funcionando.", nil, "")
	if err != nil {
		t.Fatalf("post human message: %v", err)
	}
	ceoReply, err := b.PostMessage("ceo", "general", "@builder entregue a primeira fatia nesta thread.", []string{"builder"}, root.ID)
	if err != nil {
		t.Fatalf("post CEO reply: %v", err)
	}
	builderReply, err := b.PostMessage("builder", "general", "Primeira fatia pronta e validada.", nil, ceoReply.ID)
	if err != nil {
		t.Fatalf("post builder reply: %v", err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	var child executionNode
	for _, node := range b.executionNodes {
		if node.OwnerAgent == "builder" {
			child = node
			break
		}
	}
	if child.ID == "" {
		t.Fatalf("expected builder execution node, got %+v", b.executionNodes)
	}
	if child.Status != "answered" || child.ResolvedByMessageID != builderReply.ID || child.ResolvedByAgent != "builder" {
		t.Fatalf("expected builder node answered by builder reply, got %+v", child)
	}
}

func TestRecoverTimedOutHeadlessTurnMarksExecutionNodeFallbackDispatched(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "ceo", "CEO")
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	b.mu.Unlock()

	root, err := b.PostMessage("you", "general", "Preciso do slice causal rodando.", nil, "")
	if err != nil {
		t.Fatalf("post human message: %v", err)
	}
	ceoReply, err := b.PostMessage("ceo", "general", "@builder implemente o slice causal nesta thread.", []string{"builder"}, root.ID)
	if err != nil {
		t.Fatalf("post CEO reply: %v", err)
	}
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Implementar slice causal",
		Owner:         "builder",
		CreatedBy:     "ceo",
		ThreadID:      ceoReply.ID,
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	l := newHeadlessLauncherForTest()
	l.broker = b
	l.headlessWorkers["builder"] = true
	l.recoverTimedOutHeadlessTurn("builder", headlessCodexTurn{
		TaskID:   task.ID,
		Channel:  "general",
		Prompt:   "implement the causal slice",
		Attempts: 0,
	}, time.Now().UTC().Add(-2*time.Second), headlessCodexLocalWorktreeTurnTimeout)

	b.mu.Lock()
	defer b.mu.Unlock()
	var child executionNode
	for _, node := range b.executionNodes {
		if node.OwnerAgent == "builder" {
			child = node
			break
		}
	}
	if child.ID == "" {
		t.Fatalf("expected builder execution node, got %+v", b.executionNodes)
	}
	if child.Status != "fallback_dispatched" || child.AttemptCount != 1 {
		t.Fatalf("expected fallback-dispatched execution node after retry, got %+v", child)
	}
}

func TestExecutionNodeIDsRemainMonotonicAfterPrune(t *testing.T) {
	b := NewBroker()
	now := time.Now().UTC()

	b.mu.Lock()
	b.executionNodes = []executionNode{
		{
			ID:         "exec-1",
			Channel:    "general",
			OwnerAgent: "ceo",
			Status:     "answered",
			CreatedAt:  now.Add(-48 * time.Hour).Format(time.RFC3339),
			UpdatedAt:  now.Add(-48 * time.Hour).Format(time.RFC3339),
		},
		{
			ID:         "exec-2",
			Channel:    "general",
			OwnerAgent: "builder",
			Status:     "pending",
			CreatedAt:  now.Add(-1 * time.Hour).Format(time.RFC3339),
			UpdatedAt:  now.Add(-1 * time.Hour).Format(time.RFC3339),
		},
	}
	b.normalizeExecutionNodesLocked()
	msg := channelMessage{ID: "msg-3", Channel: "general", From: "you", Timestamp: now.Format(time.RFC3339)}
	node := b.createExecutionNodeLocked(msg, "ceo", "", "msg-3", "answer")
	b.mu.Unlock()

	if node == nil {
		t.Fatal("expected execution node")
	}
	if node.ID != "exec-3" {
		t.Fatalf("expected exec-3 after prune, got %q", node.ID)
	}
}
