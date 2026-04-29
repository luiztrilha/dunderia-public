package team

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nex-crm/wuphf/internal/agent"
)

func TestSchedulerCreatesAndResolvesCEOConversationFollowUpTasks(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	now := time.Now().UTC()
	b := NewBroker()
	b.mu.Lock()
	b.messages = nil
	b.tasks = nil
	b.scheduler = nil
	b.actions = nil
	b.watchdogs = nil
	b.requests = nil
	b.counter = 0
	b.messages = []channelMessage{
		{
			ID:        "msg-human-old",
			From:      "you",
			Channel:   "general",
			Content:   "Preciso da decisão final do canal.",
			Timestamp: now.Add(-25 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:        "msg-human-other-owner",
			From:      "you",
			Channel:   "general",
			Content:   "@pm assume essa frente?",
			Tagged:    []string{"pm"},
			Timestamp: now.Add(-25 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:        "msg-ceo-old",
			From:      "ceo",
			Channel:   "general",
			Content:   "@pm valide esse ponto e me responda na thread.",
			Tagged:    []string{"pm"},
			Timestamp: now.Add(-22 * time.Minute).Format(time.RFC3339),
		},
	}
	b.mu.Unlock()

	if err := b.EnsureCEOConversationFollowUpAuditJob(); err != nil {
		t.Fatalf("ensure audit job: %v", err)
	}
	if err := b.SetSchedulerJob(schedulerJob{
		Slug:            ceoConversationFollowUpJobSlug,
		Kind:            ceoConversationFollowUpJobKind,
		Label:           "CEO conversation follow-up audit",
		TargetType:      ceoConversationFollowUpJobTargetType,
		TargetID:        "ceo",
		Channel:         "general",
		IntervalMinutes: int(ceoConversationFollowUpInterval / time.Minute),
		NextRun:         now.Add(-1 * time.Minute).Format(time.RFC3339),
		DueAt:           now.Add(-1 * time.Minute).Format(time.RFC3339),
		Status:          "scheduled",
	}); err != nil {
		t.Fatalf("set due audit job: %v", err)
	}

	l := &Launcher{broker: b}
	l.processDueSchedulerJobs()

	tasks := b.AllTasks()
	if len(tasks) != 2 {
		t.Fatalf("expected 2 CEO follow-up tasks, got %d: %+v", len(tasks), tasks)
	}

	incomingFound := false
	outgoingFound := false
	for _, task := range tasks {
		if task.Owner != "ceo" {
			t.Fatalf("expected task owner ceo, got %+v", task)
		}
		if task.TaskType != "follow_up" || task.Status != "in_progress" {
			t.Fatalf("expected in-progress follow_up task, got %+v", task)
		}
		if !strings.HasPrefix(task.ExecutionKey, ceoConversationFollowUpTaskPrefix+"|") {
			t.Fatalf("expected ceo conversation execution key, got %+v", task)
		}
		switch task.ThreadID {
		case "msg-human-old":
			incomingFound = true
		case "msg-ceo-old":
			outgoingFound = true
		case "msg-human-other-owner":
			t.Fatalf("unexpected follow-up for non-CEO tagged human message: %+v", task)
		}
	}
	if !incomingFound || !outgoingFound {
		t.Fatalf("expected both inbound and outbound follow-up tasks, got %+v", tasks)
	}

	l.processDueSchedulerJobs()
	if got := len(b.AllTasks()); got != 2 {
		t.Fatalf("expected follow-up audit to dedupe task count at 2, got %d", got)
	}

	b.mu.Lock()
	b.messages = append(b.messages,
		channelMessage{
			ID:        "msg-ceo-answer",
			From:      "ceo",
			Channel:   "general",
			Content:   "Decisão final: seguimos com a primeira opção.",
			ReplyTo:   "msg-human-old",
			Timestamp: now.Add(-2 * time.Minute).Format(time.RFC3339),
		},
		channelMessage{
			ID:        "msg-pm-answer",
			From:      "pm",
			Channel:   "general",
			Content:   "Validado, pode seguir.",
			ReplyTo:   "msg-ceo-old",
			Timestamp: now.Add(-2 * time.Minute).Format(time.RFC3339),
		},
	)
	b.mu.Unlock()

	if err := b.SetSchedulerJob(schedulerJob{
		Slug:            ceoConversationFollowUpJobSlug,
		Kind:            ceoConversationFollowUpJobKind,
		Label:           "CEO conversation follow-up audit",
		TargetType:      ceoConversationFollowUpJobTargetType,
		TargetID:        "ceo",
		Channel:         "general",
		IntervalMinutes: int(ceoConversationFollowUpInterval / time.Minute),
		NextRun:         time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339),
		DueAt:           time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339),
		Status:          "scheduled",
	}); err != nil {
		t.Fatalf("refresh due audit job: %v", err)
	}

	l.processDueSchedulerJobs()

	resolvedTasks := b.AllTasks()
	for _, task := range resolvedTasks {
		if task.Status != "done" {
			t.Fatalf("expected follow-up task to resolve after reply, got %+v", task)
		}
	}
}

func TestEnsureCEOConversationFollowUpAuditJobPreservesCanceledJob(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	now := time.Now().UTC()
	b := NewBroker()
	b.mu.Lock()
	b.scheduler = []schedulerJob{
		{
			Slug:            ceoConversationFollowUpJobSlug,
			Kind:            ceoConversationFollowUpJobKind,
			Label:           "CEO conversation follow-up audit",
			TargetType:      ceoConversationFollowUpJobTargetType,
			TargetID:        "ceo",
			Channel:         "general",
			IntervalMinutes: int(ceoConversationFollowUpInterval / time.Minute),
			NextRun:         now.Add(-1 * time.Minute).Format(time.RFC3339),
			DueAt:           now.Add(-1 * time.Minute).Format(time.RFC3339),
			Status:          "canceled",
		},
	}
	b.mu.Unlock()

	if err := b.EnsureCEOConversationFollowUpAuditJob(); err != nil {
		t.Fatalf("ensure audit job: %v", err)
	}
	queue := b.QueueSnapshot()
	if len(queue.Scheduler) != 1 {
		t.Fatalf("expected one scheduler job, got %+v", queue.Scheduler)
	}
	if got := queue.Scheduler[0].Status; got != "canceled" {
		t.Fatalf("expected canceled audit job to stay canceled, got %q", got)
	}
}

func TestCEOConversationFollowUpIgnoresAdministrativeAndSupersededThreadMessages(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	now := time.Now().UTC()
	b := NewBroker()
	b.mu.Lock()
	b.messages = []channelMessage{
		{
			ID:        "msg-root",
			From:      "you",
			Channel:   "general",
			Content:   "Erro na rota X, preciso de ajuda.",
			Timestamp: now.Add(-30 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:        "msg-ceo-1",
			From:      "ceo",
			Channel:   "general",
			Content:   "@builder confira o arquivo e me responda.",
			Tagged:    []string{"builder"},
			ReplyTo:   "msg-root",
			Timestamp: now.Add(-29 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:        "msg-human-clarify",
			From:      "you",
			Channel:   "general",
			Content:   "O alvo parece ser BNB/RecursoHumano.Cadastro.aspx.",
			ReplyTo:   "msg-root",
			Timestamp: now.Add(-28 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:        "msg-ceo-admin",
			From:      "ceo",
			Channel:   "general",
			Kind:      "task_reassigned",
			Content:   "Task \"Corrigir X\" reassigned: (unassigned) → @builder.",
			Tagged:    []string{"builder"},
			Timestamp: now.Add(-27 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:        "msg-ceo-2",
			From:      "ceo",
			Channel:   "general",
			Content:   "@builder refine o diagnóstico e reporte no fio.",
			Tagged:    []string{"builder"},
			ReplyTo:   "msg-human-clarify",
			Timestamp: now.Add(-26 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:        "msg-builder-later",
			From:      "builder",
			Channel:   "general",
			Content:   "Inspeção concluída; deixei o patch pronto.",
			ReplyTo:   "msg-ceo-1",
			Timestamp: now.Add(-25 * time.Minute).Format(time.RFC3339),
		},
	}
	b.mu.Unlock()

	created, resolved, err := b.SyncCEOConversationFollowUpTasks(now)
	if err != nil {
		t.Fatalf("sync follow-up tasks: %v", err)
	}
	if created != 0 || resolved != 0 {
		t.Fatalf("expected no follow-up tasks, got created=%d resolved=%d tasks=%+v", created, resolved, b.AllTasks())
	}
	if got := len(b.AllTasks()); got != 0 {
		t.Fatalf("expected no tasks, got %d: %+v", got, b.AllTasks())
	}
}

func TestCEOConversationFollowUpSkipsThreadsWithActiveOperationalTask(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	now := time.Now().UTC()
	b := NewBroker()
	b.mu.Lock()
	b.messages = []channelMessage{
		{
			ID:        "msg-root",
			From:      "you",
			Channel:   "general",
			Content:   "Temos um erro de Base64 nessa página.",
			Timestamp: now.Add(-25 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:        "msg-ceo-follow-up",
			From:      "ceo",
			Channel:   "general",
			Content:   "@builder investigue a origem do valor inválido e reporte a causa raiz.",
			Tagged:    []string{"builder"},
			ReplyTo:   "msg-root",
			Timestamp: now.Add(-22 * time.Minute).Format(time.RFC3339),
		},
	}
	b.tasks = []teamTask{
		{
			ID:         "task-ops",
			Channel:    "general",
			Title:      "Investigar erro de Base64",
			Owner:      "builder",
			Status:     "in_progress",
			CreatedBy:  "ceo",
			ThreadID:   "msg-root",
			TaskType:   "bugfix",
			PipelineID: "bugfix",
			CreatedAt:  now.Add(-21 * time.Minute).Format(time.RFC3339),
			UpdatedAt:  now.Add(-21 * time.Minute).Format(time.RFC3339),
		},
	}
	b.mu.Unlock()

	created, resolved, err := b.SyncCEOConversationFollowUpTasks(now)
	if err != nil {
		t.Fatalf("sync follow-up tasks: %v", err)
	}
	if created != 0 || resolved != 0 {
		t.Fatalf("expected no follow-up tasks, got created=%d resolved=%d tasks=%+v", created, resolved, b.AllTasks())
	}
	tasks := b.AllTasks()
	if len(tasks) != 1 || tasks[0].ID != "task-ops" {
		t.Fatalf("expected only the operational task to remain, got %+v", tasks)
	}
}

func TestCEOConversationFollowUpSkipsThreadsWithCompletedOperationalTask(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	now := time.Now().UTC()
	b := NewBroker()
	b.mu.Lock()
	b.messages = []channelMessage{
		{
			ID:        "msg-root",
			From:      "you",
			Channel:   "general",
			Content:   "Temos um erro antigo nessa página.",
			Timestamp: now.Add(-35 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:        "msg-ceo-follow-up",
			From:      "ceo",
			Channel:   "general",
			Content:   "@builder revalide a correção e me responda na thread.",
			Tagged:    []string{"builder"},
			ReplyTo:   "msg-root",
			Timestamp: now.Add(-22 * time.Minute).Format(time.RFC3339),
		},
	}
	b.tasks = []teamTask{
		{
			ID:         "task-done",
			Channel:    "general",
			Title:      "Corrigir erro antigo",
			Owner:      "builder",
			Status:     "done",
			CreatedBy:  "ceo",
			ThreadID:   "msg-root",
			TaskType:   "bugfix",
			PipelineID: "bugfix",
			CreatedAt:  now.Add(-21 * time.Minute).Format(time.RFC3339),
			UpdatedAt:  now.Add(-20 * time.Minute).Format(time.RFC3339),
		},
	}
	b.mu.Unlock()

	created, resolved, err := b.SyncCEOConversationFollowUpTasks(now)
	if err != nil {
		t.Fatalf("sync follow-up tasks: %v", err)
	}
	if created != 0 || resolved != 0 {
		t.Fatalf("expected no follow-up tasks over completed operational thread, got created=%d resolved=%d tasks=%+v", created, resolved, b.AllTasks())
	}
	tasks := b.AllTasks()
	if len(tasks) != 1 || tasks[0].ID != "task-done" {
		t.Fatalf("expected only the completed operational task to remain, got %+v", tasks)
	}
}

func TestCEOConversationFollowUpCoalescesEquivalentGuidanceAcrossThreads(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	now := time.Now().UTC()
	b := NewBroker()
	b.mu.Lock()
	b.messages = []channelMessage{
		{
			ID:        "msg-root-a",
			From:      "you",
			Channel:   "convenios-web-legado",
			Content:   "Erro antigo em PrestacaoContas.",
			Timestamp: now.Add(-31 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:        "msg-ceo-a",
			From:      "ceo",
			Channel:   "convenios-web-legado",
			Content:   "@builder investigue `BNB/PrestacaoContas.Detalhes.aspx` e reporte o diagnóstico técnico na thread.",
			Tagged:    []string{"builder"},
			ReplyTo:   "msg-root-a",
			Timestamp: now.Add(-30 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:        "msg-root-b",
			From:      "you",
			Channel:   "convenios-web-legado",
			Content:   "Retomando a frente da PrestacaoContas.",
			Timestamp: now.Add(-21 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:        "msg-ceo-b",
			From:      "ceo",
			Channel:   "convenios-web-legado",
			Content:   "@builder retome a auditoria em `BNB/PrestacaoContas.Detalhes.aspx` e me devolva a causa raiz nesta thread.",
			Tagged:    []string{"builder"},
			ReplyTo:   "msg-root-b",
			Timestamp: now.Add(-20 * time.Minute).Format(time.RFC3339),
		},
	}
	b.mu.Unlock()

	created, resolved, err := b.SyncCEOConversationFollowUpTasks(now)
	if err != nil {
		t.Fatalf("sync follow-up tasks: %v", err)
	}
	if created != 1 || resolved != 0 {
		t.Fatalf("expected one coalesced follow-up task, got created=%d resolved=%d tasks=%+v", created, resolved, b.AllTasks())
	}
	tasks := b.AllTasks()
	if len(tasks) != 1 {
		t.Fatalf("expected one coalesced follow-up task, got %+v", tasks)
	}
	if tasks[0].ThreadID != "msg-ceo-b" {
		t.Fatalf("expected the latest equivalent CEO guidance to win, got %+v", tasks[0])
	}
}

func TestCEOConversationFollowUpSkipsSemanticLaneWithLaterChannelProgress(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	now := time.Now().UTC()
	b := NewBroker()
	b.mu.Lock()
	b.messages = []channelMessage{
		{
			ID:        "msg-root-a",
			From:      "you",
			Channel:   "convenios-web-legado",
			Content:   "Erro antigo de ddlExtrato.",
			Timestamp: now.Add(-31 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:        "msg-ceo-a",
			From:      "ceo",
			Channel:   "convenios-web-legado",
			Content:   "Humano, aplique manualmente a proteção defensiva de `ddlExtrato.SelectedValue` em `ConciliacaoBancaria.Cadastro.aspx.cs`.",
			ReplyTo:   "msg-root-a",
			Timestamp: now.Add(-30 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:        "msg-builder-progress",
			From:      "builder",
			Channel:   "convenios-web-legado",
			Content:   "Patch defensivo de `ddlExtrato.SelectedValue` aplicado em `ConciliacaoBancaria.Cadastro.aspx.cs` e build validada.",
			Timestamp: now.Add(-12 * time.Minute).Format(time.RFC3339),
		},
	}
	b.mu.Unlock()

	created, resolved, err := b.SyncCEOConversationFollowUpTasks(now)
	if err != nil {
		t.Fatalf("sync follow-up tasks: %v", err)
	}
	if created != 0 || resolved != 0 {
		t.Fatalf("expected later semantic progress to suppress stale CEO follow-up, got created=%d resolved=%d tasks=%+v", created, resolved, b.AllTasks())
	}
	if got := len(b.AllTasks()); got != 0 {
		t.Fatalf("expected no follow-up tasks, got %+v", b.AllTasks())
	}
}

func TestSyncCEOConversationFollowUpTasksResolvesOrphanedTaskArtifacts(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	now := time.Now().UTC()
	b := NewBroker()
	b.mu.Lock()
	b.messages = []channelMessage{
		{
			ID:        "msg-live",
			From:      "ceo",
			Channel:   "convenios-web-legado",
			Content:   "@builder valide o status dessa frente e me responda na thread.",
			Tagged:    []string{"builder"},
			ReplyTo:   "msg-root",
			Timestamp: now.Add(-15 * time.Minute).Format(time.RFC3339),
		},
	}
	b.tasks = []teamTask{
		{
			ID:            "task-live",
			Channel:       "convenios-web-legado",
			ExecutionKey:  "ceo-conversation-follow-up|outgoing|convenios-web-legado|msg-live",
			Title:         "Validate unanswered CEO follow-up",
			Owner:         "ceo",
			Status:        "in_progress",
			CreatedBy:     "watchdog",
			ThreadID:      "msg-live",
			TaskType:      "follow_up",
			PipelineID:    "follow_up",
			ExecutionMode: "office",
			ReviewState:   "not_required",
			CreatedAt:     now.Add(-15 * time.Minute).Format(time.RFC3339),
			UpdatedAt:     now.Add(-15 * time.Minute).Format(time.RFC3339),
		},
	}
	b.scheduler = []schedulerJob{
		{
			Slug:       "task-follow-up:convenios-web-legado:task-orphan",
			Kind:       "task_follow_up",
			TargetType: "task",
			TargetID:   "task-orphan",
			Channel:    "convenios-web-legado",
			Status:     "scheduled",
			DueAt:      now.Add(-1 * time.Minute).Format(time.RFC3339),
			NextRun:    now.Add(-1 * time.Minute).Format(time.RFC3339),
		},
	}
	b.watchdogs = []watchdogAlert{
		{
			ID:         "watchdog-orphan",
			Kind:       "task_stalled",
			Channel:    "convenios-web-legado",
			TargetType: "task",
			TargetID:   "task-orphan",
			Owner:      "ceo",
			Status:     "active",
			Summary:    "@ceo still needs to move Validate unanswered CEO follow-up in #convenios-web-legado.",
			CreatedAt:  now.Add(-1 * time.Minute).Format(time.RFC3339),
			UpdatedAt:  now.Add(-1 * time.Minute).Format(time.RFC3339),
		},
	}
	b.mu.Unlock()

	created, resolved, err := b.SyncCEOConversationFollowUpTasks(now)
	if err != nil {
		t.Fatalf("sync follow-up tasks: %v", err)
	}
	if resolved != 0 {
		t.Fatalf("expected live follow-up to remain active while orphan artifacts are cleaned, got created=%d resolved=%d", created, resolved)
	}

	queue := b.QueueSnapshot()
	if len(queue.Scheduler) != 2 {
		t.Fatalf("expected orphan cleanup plus the live follow-up schedule, got %+v", queue.Scheduler)
	}
	var orphanJob schedulerJob
	foundOrphanJob := false
	for _, job := range queue.Scheduler {
		if job.TargetID != "task-orphan" {
			continue
		}
		orphanJob = job
		foundOrphanJob = true
		break
	}
	if !foundOrphanJob {
		t.Fatalf("expected orphan scheduler job in queue snapshot, got %+v", queue.Scheduler)
	}
	if orphanJob.Status != "done" || orphanJob.NextRun != "" || orphanJob.DueAt != "" {
		t.Fatalf("expected orphan scheduler job resolved, got %+v", orphanJob)
	}
	if len(queue.Watchdogs) != 1 {
		t.Fatalf("expected one watchdog, got %+v", queue.Watchdogs)
	}
	if queue.Watchdogs[0].Status != "resolved" {
		t.Fatalf("expected orphan watchdog resolved, got %+v", queue.Watchdogs[0])
	}
	tasks := b.AllTasks()
	if len(tasks) != 1 || tasks[0].ID != "task-live" || tasks[0].Status != "in_progress" {
		t.Fatalf("expected live follow-up task to remain active, got %+v", tasks)
	}
}

func TestSyncCEOConversationFollowUpTasksResolvesDuplicateExecutionKeyTasks(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	now := time.Now().UTC()
	executionKey := "ceo-conversation-follow-up|outgoing|convenios-web-legado|msg-live"
	details := "A CEO follow-up is still waiting for a reply in #convenios-web-legado.\n\nPending message: @builder valide o status dessa frente e me responda na thread."

	b := NewBroker()
	b.mu.Lock()
	b.messages = []channelMessage{
		{
			ID:        "msg-live",
			From:      "ceo",
			Channel:   "convenios-web-legado",
			Content:   "@builder valide o status dessa frente e me responda na thread.",
			Tagged:    []string{"builder"},
			ReplyTo:   "msg-root",
			Timestamp: now.Add(-15 * time.Minute).Format(time.RFC3339),
		},
	}
	b.tasks = []teamTask{
		{
			ID:            "task-older",
			Channel:       "convenios-web-legado",
			ExecutionKey:  executionKey,
			Title:         "Validate unanswered CEO follow-up",
			Details:       details,
			Owner:         "ceo",
			Status:        "in_progress",
			CreatedBy:     "watchdog",
			ThreadID:      "msg-live",
			TaskType:      "follow_up",
			PipelineID:    "follow_up",
			ExecutionMode: "office",
			ReviewState:   "not_required",
			CreatedAt:     now.Add(-15 * time.Minute).Format(time.RFC3339),
			UpdatedAt:     now.Add(-15 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:            "task-newer",
			Channel:       "convenios-web-legado",
			ExecutionKey:  executionKey,
			Title:         "Validate unanswered CEO follow-up",
			Details:       details,
			Owner:         "ceo",
			Status:        "in_progress",
			CreatedBy:     "watchdog",
			ThreadID:      "msg-live",
			TaskType:      "follow_up",
			PipelineID:    "follow_up",
			ExecutionMode: "office",
			ReviewState:   "not_required",
			CreatedAt:     now.Add(-12 * time.Minute).Format(time.RFC3339),
			UpdatedAt:     now.Add(-10 * time.Minute).Format(time.RFC3339),
		},
	}
	b.scheduler = []schedulerJob{
		{
			Slug:       normalizeSchedulerSlug("task_follow_up", "convenios-web-legado", "task-older"),
			Kind:       "task_follow_up",
			TargetType: "task",
			TargetID:   "task-older",
			Channel:    "convenios-web-legado",
			Status:     "scheduled",
			DueAt:      now.Add(-1 * time.Minute).Format(time.RFC3339),
			NextRun:    now.Add(-1 * time.Minute).Format(time.RFC3339),
		},
		{
			Slug:       normalizeSchedulerSlug("task_follow_up", "convenios-web-legado", "task-newer"),
			Kind:       "task_follow_up",
			TargetType: "task",
			TargetID:   "task-newer",
			Channel:    "convenios-web-legado",
			Status:     "scheduled",
			DueAt:      now.Add(-1 * time.Minute).Format(time.RFC3339),
			NextRun:    now.Add(-1 * time.Minute).Format(time.RFC3339),
		},
	}
	b.watchdogs = []watchdogAlert{
		{
			ID:         "watchdog-older",
			Kind:       "task_stalled",
			Channel:    "convenios-web-legado",
			TargetType: "task",
			TargetID:   "task-older",
			Owner:      "ceo",
			Status:     "active",
			Summary:    "@ceo still needs to move Validate unanswered CEO follow-up in #convenios-web-legado.",
			CreatedAt:  now.Add(-1 * time.Minute).Format(time.RFC3339),
			UpdatedAt:  now.Add(-1 * time.Minute).Format(time.RFC3339),
		},
	}
	b.mu.Unlock()

	_, resolved, err := b.SyncCEOConversationFollowUpTasks(now)
	if err != nil {
		t.Fatalf("sync follow-up tasks: %v", err)
	}
	if resolved != 1 {
		t.Fatalf("expected one duplicate follow-up resolved, got %d", resolved)
	}

	b.mu.Lock()
	tasks := append([]teamTask(nil), b.tasks...)
	b.mu.Unlock()
	if len(tasks) != 2 {
		t.Fatalf("expected both raw task records to remain, got %+v", tasks)
	}
	var older, newer teamTask
	for _, task := range tasks {
		switch task.ID {
		case "task-older":
			older = task
		case "task-newer":
			newer = task
		}
	}
	if older.Status != "done" {
		t.Fatalf("expected older duplicate to be resolved, got %+v", older)
	}
	if newer.Status != "in_progress" {
		t.Fatalf("expected canonical follow-up to stay active, got %+v", newer)
	}

	queue := b.QueueSnapshot()
	foundOlderJob := false
	for _, job := range queue.Scheduler {
		if job.TargetID != "task-older" {
			continue
		}
		foundOlderJob = true
		if job.Status != "done" {
			t.Fatalf("expected duplicate task scheduler to be closed, got %+v", job)
		}
	}
	if !foundOlderJob {
		t.Fatalf("expected scheduler entry for duplicate task, got %+v", queue.Scheduler)
	}
	if len(queue.Watchdogs) != 1 || queue.Watchdogs[0].Status != "resolved" {
		t.Fatalf("expected duplicate watchdog resolved, got %+v", queue.Watchdogs)
	}
}

func TestProcessDueCEOConversationFollowUpJobNotifiesLeadAboutCreatedTask(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	now := time.Now().UTC()
	b := NewBroker()
	b.mu.Lock()
	b.messages = []channelMessage{
		{
			ID:        "msg-human-old",
			From:      "you",
			Channel:   "general",
			Content:   "@ceo onde está o arquivo gerado?",
			Tagged:    []string{"ceo"},
			Timestamp: now.Add(-25 * time.Minute).Format(time.RFC3339),
		},
	}
	b.channels = []teamChannel{{
		Slug:    "general",
		Name:    "general",
		Members: []string{"ceo"},
	}}
	b.members = []officeMember{{Slug: "ceo", Name: "CEO"}}
	b.mu.Unlock()

	if err := b.SetSchedulerJob(schedulerJob{
		Slug:            ceoConversationFollowUpJobSlug,
		Kind:            ceoConversationFollowUpJobKind,
		Label:           "CEO conversation follow-up audit",
		TargetType:      ceoConversationFollowUpJobTargetType,
		TargetID:        "ceo",
		Channel:         "general",
		IntervalMinutes: int(ceoConversationFollowUpInterval / time.Minute),
		NextRun:         now.Add(-1 * time.Minute).Format(time.RFC3339),
		DueAt:           now.Add(-1 * time.Minute).Format(time.RFC3339),
		Status:          "scheduled",
	}); err != nil {
		t.Fatalf("set due audit job: %v", err)
	}

	l := newHeadlessLauncherForTest()
	l.webMode = true
	l.broker = b
	l.pack = &agent.PackDefinition{
		LeadSlug: "ceo",
		Agents: []agent.AgentConfig{
			{Slug: "ceo", Name: "CEO"},
		},
	}
	laneKey := agentLaneKey("general", "ceo")
	l.headlessWorkers[laneKey] = true

	l.processDueSchedulerJobs()

	tasks := b.AllTasks()
	if len(tasks) != 1 {
		t.Fatalf("expected one created follow-up task, got %+v", tasks)
	}
	if got := len(l.headlessQueues[laneKey]); got != 1 {
		t.Fatalf("expected one queued CEO notification, got %d", got)
	}
	if got := l.headlessQueues[laneKey][0].TaskID; got != tasks[0].ID {
		t.Fatalf("expected queued notification for %s, got %q", tasks[0].ID, got)
	}
}

func TestProcessDueCEOConversationFollowUpJobRequeuesExistingActiveTask(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	now := time.Now().UTC()
	b := NewBroker()
	b.mu.Lock()
	b.messages = []channelMessage{
		{
			ID:        "msg-806",
			From:      "you",
			Channel:   "convenios-web-azure",
			Content:   "@ceo onde está o arquivo gerado?",
			Tagged:    []string{"ceo"},
			Timestamp: now.Add(-20 * time.Minute).Format(time.RFC3339),
		},
	}
	b.tasks = []teamTask{
		{
			ID:            "task-existing",
			Channel:       "convenios-web-azure",
			ExecutionKey:  "ceo-conversation-follow-up|incoming|convenios-web-azure|msg-806",
			Title:         "Reply to pending message from @you",
			Details:       "A stale inbound thread still needs a CEO answer in #convenios-web-azure.",
			Owner:         "ceo",
			Status:        "in_progress",
			CreatedBy:     "watchdog",
			ThreadID:      "msg-806",
			TaskType:      "follow_up",
			PipelineID:    "follow_up",
			ExecutionMode: "office",
			ReviewState:   "not_required",
			CreatedAt:     now.Add(-20 * time.Minute).Format(time.RFC3339),
			UpdatedAt:     now.Add(-15 * time.Minute).Format(time.RFC3339),
		},
	}
	b.channels = []teamChannel{{
		Slug:    "convenios-web-azure",
		Name:    "convenios-web-azure",
		Members: []string{"ceo"},
	}}
	b.members = []officeMember{{Slug: "ceo", Name: "CEO"}}
	b.mu.Unlock()

	if err := b.SetSchedulerJob(schedulerJob{
		Slug:            ceoConversationFollowUpJobSlug,
		Kind:            ceoConversationFollowUpJobKind,
		Label:           "CEO conversation follow-up audit",
		TargetType:      ceoConversationFollowUpJobTargetType,
		TargetID:        "ceo",
		Channel:         "general",
		IntervalMinutes: int(ceoConversationFollowUpInterval / time.Minute),
		NextRun:         now.Add(-1 * time.Minute).Format(time.RFC3339),
		DueAt:           now.Add(-1 * time.Minute).Format(time.RFC3339),
		Status:          "scheduled",
	}); err != nil {
		t.Fatalf("set due audit job: %v", err)
	}

	l := newHeadlessLauncherForTest()
	l.webMode = true
	l.broker = b
	l.pack = &agent.PackDefinition{
		LeadSlug: "ceo",
		Agents: []agent.AgentConfig{
			{Slug: "ceo", Name: "CEO"},
		},
	}
	laneKey := agentLaneKey("convenios-web-azure", "ceo")
	l.headlessWorkers[laneKey] = true

	l.processDueSchedulerJobs()

	if got := len(l.headlessQueues[laneKey]); got != 1 {
		t.Fatalf("expected existing active follow-up to be requeued, got %d", got)
	}
	if got := l.headlessQueues[laneKey][0].TaskID; got != "task-existing" {
		t.Fatalf("expected requeued notification for task-existing, got %q", got)
	}
}

func TestSyncCEOConversationFollowUpTasksUsesThreadRootForInboundReplyRoute(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	now := time.Now().UTC()
	b := NewBroker()
	b.mu.Lock()
	b.messages = []channelMessage{
		{
			ID:        "msg-806",
			From:      "you",
			Channel:   "convenios-web-azure",
			Content:   "@ceo onde está o arquivo gerado?",
			Tagged:    []string{"ceo"},
			Timestamp: now.Add(-20 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:        "msg-814",
			From:      "you",
			Channel:   "convenios-web-azure",
			Content:   "@ceo responda",
			Tagged:    []string{"ceo"},
			ReplyTo:   "msg-806",
			Timestamp: now.Add(-10 * time.Minute).Format(time.RFC3339),
		},
	}
	b.mu.Unlock()

	created, _, err := b.SyncCEOConversationFollowUpTasks(now)
	if err != nil {
		t.Fatalf("sync follow-up tasks: %v", err)
	}
	if created != 1 {
		t.Fatalf("expected one created follow-up task, got %d", created)
	}

	tasks := b.AllTasks()
	if len(tasks) != 1 {
		t.Fatalf("expected one follow-up task, got %+v", tasks)
	}
	if got := tasks[0].ExecutionKey; got != "ceo-conversation-follow-up|incoming|convenios-web-azure|msg-814" {
		t.Fatalf("expected execution key to remain anchored to the latest human follow-up, got %q", got)
	}
	if got := tasks[0].ThreadID; got != "msg-806" {
		t.Fatalf("expected task thread route to use root msg-806, got %q", got)
	}
	if !strings.Contains(tasks[0].Details, "@ceo onde está o arquivo gerado?") {
		t.Fatalf("expected task details to include original ask, got %q", tasks[0].Details)
	}
	if !strings.Contains(tasks[0].Details, "@ceo responda") {
		t.Fatalf("expected task details to include latest follow-up, got %q", tasks[0].Details)
	}
}

func TestReconcileStateResolvesOrphanedTaskArtifacts(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	now := time.Now().UTC()
	b := NewBroker()
	b.mu.Lock()
	b.tasks = []teamTask{
		{
			ID:            "task-live",
			Channel:       "general",
			ExecutionKey:  "ceo-conversation-follow-up|outgoing|general|msg-live",
			Title:         "Validate unanswered CEO follow-up",
			Owner:         "ceo",
			Status:        "in_progress",
			CreatedBy:     "watchdog",
			ThreadID:      "msg-live",
			TaskType:      "follow_up",
			PipelineID:    "follow_up",
			ExecutionMode: "office",
			ReviewState:   "not_required",
			CreatedAt:     now.Add(-15 * time.Minute).Format(time.RFC3339),
			UpdatedAt:     now.Add(-15 * time.Minute).Format(time.RFC3339),
		},
	}
	b.scheduler = []schedulerJob{
		{
			Slug:       "task-follow-up:general:task-orphan",
			Kind:       "task_follow_up",
			TargetType: "task",
			TargetID:   "task-orphan",
			Channel:    "general",
			Status:     "scheduled",
			DueAt:      now.Add(-1 * time.Minute).Format(time.RFC3339),
			NextRun:    now.Add(-1 * time.Minute).Format(time.RFC3339),
		},
	}
	b.watchdogs = []watchdogAlert{
		{
			ID:         "watchdog-orphan",
			Kind:       "task_stalled",
			Channel:    "general",
			TargetType: "task",
			TargetID:   "task-orphan",
			Owner:      "ceo",
			Status:     "active",
			Summary:    "@ceo still needs to move Validate unanswered CEO follow-up in #general.",
			CreatedAt:  now.Add(-1 * time.Minute).Format(time.RFC3339),
			UpdatedAt:  now.Add(-1 * time.Minute).Format(time.RFC3339),
		},
	}
	b.normalizeLoadedStateLocked()
	b.mu.Unlock()

	queue := b.QueueSnapshot()
	if len(queue.Scheduler) != 2 {
		t.Fatalf("expected task lifecycle plus orphan cleanup, got %+v", queue.Scheduler)
	}
	foundOrphan := false
	for _, job := range queue.Scheduler {
		if job.TargetID != "task-orphan" {
			continue
		}
		foundOrphan = true
		if job.Status != "done" || job.NextRun != "" || job.DueAt != "" {
			t.Fatalf("expected orphan scheduler job resolved during reconcile, got %+v", job)
		}
	}
	if !foundOrphan {
		t.Fatalf("expected orphan scheduler job to remain as done record, got %+v", queue.Scheduler)
	}
	if len(queue.Watchdogs) != 1 || queue.Watchdogs[0].Status != "resolved" {
		t.Fatalf("expected orphan watchdog resolved during reconcile, got %+v", queue.Watchdogs)
	}
}

func TestCEOConversationFollowUpSkipsExplicitNoActionDirective(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	now := time.Now().UTC()
	b := NewBroker()
	b.mu.Lock()
	b.members = []officeMember{
		{Slug: "ceo", Name: "CEO"},
		{Slug: "game-master", Name: "Game Master"},
	}
	b.messages = []channelMessage{
		{
			ID:        "msg-no-action",
			From:      "ceo",
			Channel:   "general",
			Content:   "@game-master, prossiga com a consolidação integral e reporte apenas na conclusão do lote. Nenhuma ação adicional requerida nesta thread.",
			Tagged:    []string{"@game-master"},
			Timestamp: now.Add(-20 * time.Minute).Format(time.RFC3339),
		},
	}
	b.mu.Unlock()

	created, resolved, err := b.SyncCEOConversationFollowUpTasks(now)
	if err != nil {
		t.Fatalf("sync follow-up tasks: %v", err)
	}
	if created != 0 || resolved != 0 {
		t.Fatalf("expected no follow-up churn, got created=%d resolved=%d", created, resolved)
	}
	if tasks := b.AllTasks(); len(tasks) != 0 {
		t.Fatalf("expected no follow-up task, got %+v", tasks)
	}
}
