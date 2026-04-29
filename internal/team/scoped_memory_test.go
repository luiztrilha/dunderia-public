package team

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEncodePrivateMemoryNotePreservesExplicitUpdatedAt(t *testing.T) {
	raw := encodePrivateMemoryNote(privateMemoryNote{
		Key:       "note-1",
		Title:     "Human request",
		Content:   "Keep the original timestamp during rebuild.",
		Author:    "you",
		CreatedAt: "2026-04-19T23:19:25Z",
		UpdatedAt: "2026-04-19T23:19:25Z",
	})
	note := decodePrivateMemoryNote("note-1", raw)
	if note.UpdatedAt != "2026-04-19T23:19:25Z" {
		t.Fatalf("expected updated_at preserved, got %+v", note)
	}
}

func TestRebuildChannelMemoryLockedRecoversHistoryAndPreservesScopedMemory(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	defer b.mu.Unlock()

	b.messages = []channelMessage{
		{
			ID:        "msg-1",
			From:      "you",
			Channel:   "delivery",
			Content:   "A partir de agora, foquem no endpoint de entrega e preservem o contrato antigo.",
			Timestamp: "2026-04-19T20:10:00Z",
		},
		{
			ID:        "msg-2",
			From:      "ceo",
			Channel:   "delivery",
			Tagged:    []string{"builder"},
			Content:   "Builder, feche a entrega sem mudar a assinatura do payload legado.",
			Timestamp: "2026-04-19T20:11:00Z",
		},
	}
	b.actions = []officeActionLog{
		{
			ID:        "action-1",
			Kind:      "task_updated",
			Actor:     "builder",
			Channel:   "delivery",
			RelatedID: "task-1",
			Summary:   "Implementar o endpoint de entrega [in_progress]",
			CreatedAt: "2026-04-19T20:12:00Z",
		},
	}
	b.decisions = []officeDecisionRecord{
		{
			ID:        "decision-1",
			Channel:   "delivery",
			Owner:     "ceo",
			Summary:   "A entrega deve preservar o contrato legado.",
			Reason:    "Consumidores antigos ainda dependem do payload atual.",
			CreatedAt: "2026-04-19T20:13:00Z",
		},
	}
	b.sharedMemory = map[string]map[string]string{
		channelMemoryNamespace("delivery"): {
			"stale": encodePrivateMemoryNote(privateMemoryNote{
				Key:       "stale",
				Title:     "Human request",
				Content:   "old note",
				Author:    "you",
				CreatedAt: "2026-04-19T18:00:00Z",
				UpdatedAt: "2026-04-19T18:00:00Z",
			}),
		},
		privateMemoryNamespace("builder"): {
			"private-1": encodePrivateMemoryNote(privateMemoryNote{
				Key:       "private-1",
				Title:     "Scratch",
				Content:   "keep private memory intact",
				Author:    "builder",
				CreatedAt: "2026-04-19T18:00:00Z",
				UpdatedAt: "2026-04-19T18:00:00Z",
			}),
		},
	}

	stats := b.rebuildChannelMemoryLocked(nil)
	var deliveryStat *ChannelMemoryRebuildStat
	for i := range stats {
		if stats[i].Channel == "delivery" {
			deliveryStat = &stats[i]
			break
		}
	}
	if deliveryStat == nil {
		t.Fatalf("expected delivery stats, got %+v", stats)
	}
	if deliveryStat.Before != 1 || deliveryStat.After != 5 {
		t.Fatalf("expected delivery 1->5 notes after rebuild, got %+v", deliveryStat)
	}
	privateEntries := b.sharedMemory[privateMemoryNamespace("builder")]
	if len(privateEntries) != 1 {
		t.Fatalf("expected private memory preserved, got %#v", privateEntries)
	}
	entries := b.sharedMemory[channelMemoryNamespace("delivery")]
	if len(entries) != 5 {
		t.Fatalf("expected rebuilt delivery channel memory, got %#v", entries)
	}
	if _, ok := entries["stale"]; !ok {
		t.Fatalf("expected pre-existing note preserved, got keys=%v", mapKeys(entries))
	}
	if _, ok := entries["msg:msg-1"]; !ok {
		t.Fatalf("expected human note in rebuilt entries, got keys=%v", mapKeys(entries))
	}
	if _, ok := entries["msg:msg-2"]; !ok {
		t.Fatalf("expected CEO handoff in rebuilt entries, got keys=%v", mapKeys(entries))
	}
	if _, ok := entries["action:task-1:task_updated"]; !ok {
		t.Fatalf("expected task update note in rebuilt entries, got keys=%v", mapKeys(entries))
	}
	if _, ok := entries["decision:decision-1"]; !ok {
		t.Fatalf("expected decision note in rebuilt entries, got keys=%v", mapKeys(entries))
	}
	decision := decodePrivateMemoryNote("decision:decision-1", entries["decision:decision-1"])
	if !strings.Contains(decision.Content, "Consumidores antigos") {
		t.Fatalf("expected decision reason preserved, got %+v", decision)
	}
}

func TestRepairChannelMemoryPersistsRebuiltState(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	b.mu.Lock()
	b.channels = append(b.channels, teamChannel{Slug: "delivery", Name: "Delivery"})
	b.messages = []channelMessage{
		{
			ID:        "msg-1",
			From:      "you",
			Channel:   "delivery",
			Content:   "O contrato de entrega precisa continuar compatível com o legado.",
			Timestamp: "2026-04-19T20:10:00Z",
		},
	}
	if err := b.saveLocked(); err != nil {
		t.Fatalf("save initial broker state: %v", err)
	}
	b.mu.Unlock()

	stats, err := RepairChannelMemory("delivery")
	if err != nil {
		t.Fatalf("repair channel memory: %v", err)
	}
	if len(stats) != 1 || stats[0].Channel != "delivery" || stats[0].After != 1 {
		t.Fatalf("unexpected repair stats: %+v", stats)
	}

	reloaded := NewBroker()
	reloaded.mu.Lock()
	defer reloaded.mu.Unlock()
	entries := reloaded.sharedMemory[channelMemoryNamespace("delivery")]
	if len(entries) != 1 {
		t.Fatalf("expected persisted rebuilt memory, got %#v", entries)
	}
}

func TestNormalizeLoadedStateRecoversMessagesFromChannelMessageNotes(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	defer b.mu.Unlock()

	b.messages = []channelMessage{
		{
			ID:        "msg-1",
			From:      "you",
			Channel:   "general",
			Content:   "Mensagem ainda presente no canal.",
			Timestamp: "2026-04-19T20:10:00Z",
		},
	}
	b.sharedMemory = map[string]map[string]string{
		channelMemoryNamespace("general"): {
			"msg:msg-1": encodePrivateMemoryNote(privateMemoryNote{
				Key:       "msg:msg-1",
				Title:     "Human request",
				Content:   "Versao antiga e incorreta da mensagem.",
				Author:    "you",
				CreatedAt: "2026-04-19T20:10:00Z",
				UpdatedAt: "2026-04-19T20:10:00Z",
			}),
			"msg:msg-missing": encodePrivateMemoryNote(privateMemoryNote{
				Key:       "msg:msg-missing",
				Title:     "Human request",
				Content:   "Mensagem que sumiu do canal.",
				Author:    "you",
				CreatedAt: "2026-04-19T19:55:00Z",
				UpdatedAt: "2026-04-19T19:55:00Z",
			}),
			"stale": encodePrivateMemoryNote(privateMemoryNote{
				Key:       "stale",
				Title:     "Pinned note",
				Content:   "Nota manual sem amarracao a uma mensagem.",
				Author:    "ceo",
				CreatedAt: "2026-04-19T19:40:00Z",
				UpdatedAt: "2026-04-19T19:40:00Z",
			}),
		},
	}

	b.normalizeLoadedStateLocked()

	entries := b.sharedMemory[channelMemoryNamespace("general")]
	raw, ok := entries["msg:msg-1"]
	if !ok {
		t.Fatalf("expected live message note preserved, got keys=%v", mapKeys(entries))
	}
	note := decodePrivateMemoryNote("msg:msg-1", raw)
	if !strings.Contains(note.Content, "Mensagem ainda presente no canal") {
		t.Fatalf("expected live message note refreshed from current message, got %+v", note)
	}
	recoveredRaw, ok := entries["msg:msg-missing"]
	if !ok {
		t.Fatalf("expected missing message note to remain as recovery source, got keys=%v", mapKeys(entries))
	}
	recoveredNote := decodePrivateMemoryNote("msg:msg-missing", recoveredRaw)
	if !strings.Contains(recoveredNote.Content, "Mensagem que sumiu do canal") {
		t.Fatalf("expected recovered message note preserved, got %+v", recoveredNote)
	}
	foundRecoveredMessage := false
	for _, msg := range b.messages {
		if msg.ID != "msg-missing" {
			continue
		}
		foundRecoveredMessage = true
		if msg.Channel != "general" || !strings.Contains(msg.Content, "Mensagem que sumiu do canal") {
			t.Fatalf("expected msg-missing recovered into channel history, got %+v", msg)
		}
		break
	}
	if !foundRecoveredMessage {
		t.Fatalf("expected msg-missing recovered into broker messages, got %+v", b.messages)
	}
	if _, ok := entries["stale"]; !ok {
		t.Fatalf("expected non-message note preserved, got keys=%v", mapKeys(entries))
	}
}

func TestRecoverTasksFromChannelMemoryDoesNotReviveTerminalTask(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	defer b.mu.Unlock()

	b.tasks = []teamTask{
		{
			ID:        "task-1",
			Channel:   "general",
			Title:     "Validate unanswered CEO follow-up",
			Status:    "done",
			TaskType:  "follow_up",
			UpdatedAt: "2026-04-24T18:00:00Z",
		},
	}
	b.sharedMemory = map[string]map[string]string{
		channelMemoryNamespace("general"): {
			"action:task-1:task_created": encodePrivateMemoryNote(privateMemoryNote{
				Key:       "action:task-1:task_created",
				Title:     "Task event",
				Content:   "@watchdog Validate unanswered CEO follow-up [open]",
				Author:    "watchdog",
				CreatedAt: "2026-04-24T18:50:42Z",
				UpdatedAt: "2026-04-24T18:50:42Z",
			}),
		},
	}

	b.normalizeLoadedStateLocked()

	if len(b.tasks) != 1 {
		t.Fatalf("expected no duplicate recovered task, got %+v", b.tasks)
	}
	if got := b.tasks[0].Status; got != "done" {
		t.Fatalf("expected terminal task to stay done, got %q", got)
	}
}

func TestNormalizeLoadedStateSuppressesFollowUpsWhenAuditCanceled(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	defer b.mu.Unlock()

	b.tasks = []teamTask{
		{
			ID:         "task-1",
			Channel:    "general",
			Title:      "Reply to pending message from @human",
			Status:     "open",
			TaskType:   "follow_up",
			PipelineID: "follow_up",
			UpdatedAt:  "2026-04-24T18:00:00Z",
		},
		{
			ID:         "task-2",
			Channel:    "migracao-convenios",
			Title:      "Validate unanswered CEO follow-up",
			Status:     "open",
			TaskType:   "bugfix",
			PipelineID: "bugfix",
			UpdatedAt:  "2026-04-24T18:00:00Z",
		},
	}
	b.scheduler = []schedulerJob{
		{
			Slug:       ceoConversationFollowUpJobSlug,
			Kind:       ceoConversationFollowUpJobKind,
			TargetType: ceoConversationFollowUpJobTargetType,
			TargetID:   "ceo",
			Status:     "canceled",
		},
	}

	b.normalizeLoadedStateLocked()

	if got := b.tasks[0].Status; got != "done" {
		t.Fatalf("expected canceled audit to suppress active follow-up, got %q", got)
	}
	if got := b.tasks[1].Status; got != "done" {
		t.Fatalf("expected canceled audit to suppress legacy titled follow-up, got %q", got)
	}
	if b.tasks[0].Blocked {
		t.Fatalf("expected suppressed follow-up to be unblocked")
	}
}

func TestNormalizeLoadedStateClearsTerminalLocalWorktreeWithoutClearingWorkspace(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	defer b.mu.Unlock()

	b.tasks = []teamTask{
		{
			ID:             "task-1",
			Channel:        "convenios-web-azure",
			Title:          "Deploy Azure publish artifact",
			Owner:          "builder",
			Status:         "canceled",
			ExecutionMode:  "local_worktree",
			WorkspacePath:  "D:\\Repos\\ConveniosWebVSAzure_Default",
			WorktreePath:   "C:\\Users\\l.sousa\\.wuphf\\task-worktrees\\dunderia\\wuphf-task-task-3059",
			WorktreeBranch: "wuphf-221fdf9b-task-3059",
		},
		{
			ID:             "task-2",
			Channel:        "convenios-web-azure",
			Title:          "Finished local worktree task",
			Owner:          "builder",
			Status:         "done",
			ExecutionMode:  "local_worktree",
			WorkspacePath:  "D:\\Repos\\ConveniosWebVSAzure_Default",
			WorktreePath:   "C:\\Users\\l.sousa\\.wuphf\\task-worktrees\\dunderia\\wuphf-task-task-2478",
			WorktreeBranch: "wuphf-221fdf9b-task-2478",
		},
	}

	b.normalizeLoadedStateLocked()

	for i := range b.tasks {
		if got := b.tasks[i].WorkspacePath; got != "D:\\Repos\\ConveniosWebVSAzure_Default" {
			t.Fatalf("expected workspace path preserved, got %q", got)
		}
		if b.tasks[i].WorktreePath != "" || b.tasks[i].WorktreeBranch != "" {
			t.Fatalf("expected terminal local worktree fields cleared, got task=%s path=%q branch=%q", b.tasks[i].ID, b.tasks[i].WorktreePath, b.tasks[i].WorktreeBranch)
		}
	}
}
