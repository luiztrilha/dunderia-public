package team

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildOperatorTasksLockedIncludesHumanRequestRecommendation(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	b := NewBroker()
	b.mu.Lock()
	defer b.mu.Unlock()

	ensureTestMemberAccess(b, "general", "ceo", "CEO")
	ensureTestMemberAccess(b, "general", "game-master", "Game Master")

	b.appendMessageLocked(channelMessage{
		ID:        "msg-1",
		From:      "ceo",
		Channel:   "general",
		Content:   "Should we ship release R12?",
		Timestamp: "2026-04-23T12:00:00Z",
	})
	b.appendMessageLocked(channelMessage{
		ID:        "msg-2",
		From:      "game-master",
		Channel:   "general",
		ReplyTo:   "msg-1",
		Content:   "Suggested reply: approve the rollout and keep a rollback ready.",
		Timestamp: "2026-04-23T12:05:00Z",
	})

	b.tasks = []teamTask{{
		ID:        "task-1",
		Channel:   "general",
		Title:     "Ship release R12",
		Details:   "Prepare the release report.",
		Owner:     "builder",
		Status:    "in_progress",
		ThreadID:  "msg-1",
		CreatedAt: "2026-04-23T11:55:00Z",
		UpdatedAt: "2026-04-23T12:04:00Z",
	}}
	b.requests = []humanInterview{{
		ID:                        "request-1",
		From:                      "ceo",
		Channel:                   "general",
		Title:                     "Approve release",
		Question:                  "Should we deploy now?",
		Context:                   "QA is complete and the changelog is ready.",
		Status:                    "pending",
		ReplyTo:                   "msg-1",
		SourceTaskID:              "task-1",
		CreatedAt:                 "2026-04-23T12:01:00Z",
		UpdatedAt:                 "2026-04-23T12:02:00Z",
		RecommendationStatus:      "requested",
		RecommendationTaskID:      "task-9",
		RecommendationRequestedAt: "2026-04-23T12:03:00Z",
		Options: []interviewOption{
			{ID: "approve", Label: "Approve"},
			{ID: "reject", Label: "Reject"},
		},
	}}

	tasks := b.buildOperatorTasksLocked("general", false, true, "", "human")

	var humanTask *teamTask
	for i := range tasks {
		if tasks[i].ID == "human-request-request-1" {
			humanTask = &tasks[i]
			break
		}
	}
	if humanTask == nil {
		t.Fatalf("expected derived human task, got %#v", tasks)
	}
	if !humanTask.AwaitingHuman {
		t.Fatalf("expected awaiting_human=true, got %#v", humanTask)
	}
	if humanTask.RecommendationStatus != "ready" {
		t.Fatalf("expected recommendation status ready, got %q", humanTask.RecommendationStatus)
	}
	if !strings.Contains(humanTask.RecommendationSummary, "Suggested reply") {
		t.Fatalf("expected recommendation summary from game-master message, got %q", humanTask.RecommendationSummary)
	}
	if humanTask.DeliveryID != "delivery-task-task-1" {
		t.Fatalf("expected delivery-task-task-1, got %q", humanTask.DeliveryID)
	}
	if humanTask.ProgressPercent != 50 {
		t.Fatalf("expected 50%% progress, got %d", humanTask.ProgressPercent)
	}
	if humanTask.ProgressBasis != "2 of 4 milestones complete" {
		t.Fatalf("unexpected progress basis %q", humanTask.ProgressBasis)
	}
	if len(humanTask.HumanOptions) != 2 {
		t.Fatalf("expected request options copied into task, got %#v", humanTask.HumanOptions)
	}
}

func TestBuildDeliveriesLockedAggregatesArtifactsAndPendingHuman(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	b := NewBroker()
	b.mu.Lock()
	defer b.mu.Unlock()

	b.tasks = []teamTask{{
		ID:            "task-1",
		Channel:       "general",
		Title:         "Migration report",
		Details:       "Draft report at C:\\Reports\\migration.md",
		Owner:         "builder",
		Status:        "review",
		ThreadID:      "msg-1",
		WorkspacePath: "<REPOS_ROOT>\\ExampleDataRepo",
		WorktreePath:  "<REPOS_ROOT>\\worktrees\\migration-report",
		CreatedAt:     "2026-04-23T11:55:00Z",
		UpdatedAt:     "2026-04-23T12:04:00Z",
	}}
	b.requests = []humanInterview{{
		ID:           "request-1",
		From:         "ceo",
		Channel:      "general",
		Status:       "pending",
		Question:     "Pick the rollout window",
		ReplyTo:      "msg-1",
		SourceTaskID: "task-1",
		CreatedAt:    "2026-04-23T12:01:00Z",
		UpdatedAt:    "2026-04-23T12:06:00Z",
	}}

	deliveries := b.buildDeliveriesLocked("general", false, true)
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %#v", deliveries)
	}
	delivery := deliveries[0]
	if delivery.ID != "delivery-task-task-1" {
		t.Fatalf("unexpected delivery id %q", delivery.ID)
	}
	if delivery.Status != "awaiting_human" {
		t.Fatalf("expected awaiting_human status, got %q", delivery.Status)
	}
	if delivery.PendingHumanCount != 1 {
		t.Fatalf("expected 1 pending human item, got %d", delivery.PendingHumanCount)
	}
	if delivery.ProgressPercent != 75 {
		t.Fatalf("expected 75%% progress for review delivery, got %d", delivery.ProgressPercent)
	}
	if delivery.ProgressBasis != "3 of 4 milestones complete" {
		t.Fatalf("unexpected progress basis %q", delivery.ProgressBasis)
	}
	if delivery.WorkspacePath != "<REPOS_ROOT>\\ExampleDataRepo" {
		t.Fatalf("expected workspace path to prefer repo workspace, got %q", delivery.WorkspacePath)
	}
	if len(delivery.Artifacts) < 3 {
		t.Fatalf("expected workspace, worktree, and document artifacts, got %#v", delivery.Artifacts)
	}
}

func TestBuildDeliveriesLockedPrefersBusinessWorkspaceOverHelperReposAndDunderiaWorktrees(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	b := NewBroker()
	b.mu.Lock()
	defer b.mu.Unlock()

	b.tasks = []teamTask{
		{
			ID:            "task-1",
			Channel:       "ExampleWorkflow-web-legado",
			Title:         "Legacy bugfix",
			Owner:         "builder",
			Status:        "in_progress",
			PipelineID:    "bugfix",
			WorkspacePath: "<REPOS_ROOT>\\scripts",
			UpdatedAt:     "2026-04-24T13:26:54Z",
		},
		{
			ID:           "task-2",
			Channel:      "ExampleWorkflow-web-legado",
			Title:        "Legacy bugfix follow-up",
			Owner:        "builder",
			Status:       "in_progress",
			PipelineID:   "bugfix",
			WorktreePath: "<USER_HOME>\\.wuphf\\task-worktrees\\dunderia\\wuphf-task-task-946",
			UpdatedAt:    "2026-04-24T13:27:54Z",
		},
		{
			ID:            "task-3",
			Channel:       "ExampleWorkflow-web-legado",
			Title:         "Legacy bugfix canonical workspace",
			Owner:         "builder",
			Status:        "blocked",
			PipelineID:    "bugfix",
			WorkspacePath: "<REPOS_ROOT>\\LegacySystemOld",
			UpdatedAt:     "2026-04-24T12:26:54Z",
		},
	}

	deliveries := b.buildDeliveriesLocked("ExampleWorkflow-web-legado", false, true)
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery bucket, got %#v", deliveries)
	}
	if deliveries[0].WorkspacePath != "<REPOS_ROOT>\\LegacySystemOld" {
		t.Fatalf("expected business repo workspace to win, got %q", deliveries[0].WorkspacePath)
	}
}

func TestBuildDeliveriesLockedPrefersDunderiaWorktreeOverHelperWorkspaceWhenNoBusinessRepoExists(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	b := NewBroker()
	b.mu.Lock()
	defer b.mu.Unlock()

	b.tasks = []teamTask{
		{
			ID:            "task-1",
			Channel:       "ExampleWorkflow-web-azure",
			Title:         "Runtime helper lane",
			Owner:         "watchdog",
			Status:        "canceled",
			PipelineID:    "bugfix",
			WorkspacePath: "<REPOS_ROOT>\\scripts",
			UpdatedAt:     "2026-04-24T14:54:58Z",
		},
		{
			ID:           "task-2",
			Channel:      "ExampleWorkflow-web-azure",
			Title:        "Dunderia worktree lane",
			Owner:        "ceo",
			Status:       "canceled",
			PipelineID:   "bugfix",
			WorktreePath: "<USER_HOME>\\.wuphf\\task-worktrees\\dunderia\\wuphf-task-task-2478",
			UpdatedAt:    "2026-04-24T13:55:41Z",
		},
	}

	deliveries := b.buildDeliveriesLocked("ExampleWorkflow-web-azure", false, true)
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery bucket, got %#v", deliveries)
	}
	if deliveries[0].WorkspacePath != "<USER_HOME>\\.wuphf\\task-worktrees\\dunderia\\wuphf-task-task-2478" {
		t.Fatalf("expected dunderia worktree to win over helper workspace, got %q", deliveries[0].WorkspacePath)
	}
}

func TestBuildDeliveriesLockedUsesChannelLinkedRepoWhenTaskHasNoRepository(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	b := NewBroker()
	b.mu.Lock()
	defer b.mu.Unlock()

	b.channels = []teamChannel{
		{
			Slug: "ExampleWorkflow-web-azure",
			Name: "Fluxo Exemplo Web Azure",
			LinkedRepos: []linkedRepoRef{
				{
					RepoPath: "<REPOS_ROOT>\\ExampleAzureRepo",
					Primary:  true,
				},
			},
		},
	}
	b.tasks = []teamTask{
		{
			ID:           "task-1",
			Channel:      "ExampleWorkflow-web-azure",
			Title:        "Azure deployment lane",
			Owner:        "builder",
			Status:       "blocked",
			PipelineID:   "launch",
			WorktreePath: "<USER_HOME>\\.wuphf\\task-worktrees\\dunderia\\wuphf-task-task-3059",
			UpdatedAt:    "2026-04-24T18:29:14Z",
		},
	}

	deliveries := b.buildDeliveriesLocked("ExampleWorkflow-web-azure", false, true)
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery bucket, got %#v", deliveries)
	}
	if deliveries[0].WorkspacePath != "<REPOS_ROOT>\\ExampleAzureRepo" {
		t.Fatalf("expected channel linked repo to win, got %q", deliveries[0].WorkspacePath)
	}
}

func TestBuildDeliveriesLockedUsesMentionedWorkspaceForTerminalSmokeHistory(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	repoPath, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("repo path: %v", err)
	}
	repoPath = filepath.Clean(repoPath)

	b := NewBroker()
	b.mu.Lock()
	defer b.mu.Unlock()

	b.tasks = []teamTask{
		{
			ID:            "task-263",
			Channel:       "general",
			Title:         "Smoke test startup em " + repoPath,
			Details:       "Smoke local executado em " + repoPath + ". Target identificado: ./cmd/wuphf.",
			Status:        "canceled",
			PipelineID:    "feature",
			ExecutionMode: "local_worktree",
			UpdatedAt:     "2026-04-20T17:24:17Z",
		},
	}

	deliveries := b.buildDeliveriesLocked("general", false, true)
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery bucket, got %#v", deliveries)
	}
	if deliveries[0].Status != "done" {
		t.Fatalf("expected terminal smoke history to be done, got %q", deliveries[0].Status)
	}
	if filepath.Clean(deliveries[0].WorkspacePath) != repoPath {
		t.Fatalf("expected mentioned repo workspace to win, got %q", deliveries[0].WorkspacePath)
	}
}

func TestBuildDeliveriesLockedKeepsPipelineBucketsScopedPerChannel(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	b := NewBroker()
	b.mu.Lock()
	defer b.mu.Unlock()

	b.tasks = []teamTask{
		{
			ID:         "task-1",
			Channel:    "general",
			Title:      "General follow-up",
			Owner:      "ceo",
			Status:     "in_progress",
			PipelineID: "follow_up",
			CreatedAt:  "2026-04-24T10:00:00Z",
			UpdatedAt:  "2026-04-24T10:05:00Z",
		},
		{
			ID:         "task-2",
			Channel:    "ExampleWorkflow-web-legado",
			Title:      "Legacy follow-up",
			Owner:      "reviewer",
			Status:     "in_progress",
			PipelineID: "follow_up",
			CreatedAt:  "2026-04-24T10:01:00Z",
			UpdatedAt:  "2026-04-24T10:06:00Z",
		},
	}

	deliveries := b.buildDeliveriesLocked("", true, true)
	if len(deliveries) != 2 {
		t.Fatalf("expected 2 deliveries split by channel, got %#v", deliveries)
	}

	byID := make(map[string]deliveryView, len(deliveries))
	for _, delivery := range deliveries {
		byID[delivery.ID] = delivery
	}

	generalDelivery, ok := byID["delivery-channel-general-pipeline-follow_up"]
	if !ok {
		t.Fatalf("expected general channel delivery bucket, got %#v", deliveries)
	}
	if generalDelivery.Channel != "general" {
		t.Fatalf("expected general delivery channel, got %#v", generalDelivery)
	}
	if len(generalDelivery.TaskIDs) != 1 || generalDelivery.TaskIDs[0] != "task-1" {
		t.Fatalf("expected only task-1 in general delivery, got %#v", generalDelivery.TaskIDs)
	}

	legacyDelivery, ok := byID["delivery-channel-ExampleWorkflow-web-legado-pipeline-follow_up"]
	if !ok {
		t.Fatalf("expected legacy channel delivery bucket, got %#v", deliveries)
	}
	if legacyDelivery.Channel != "ExampleWorkflow-web-legado" {
		t.Fatalf("expected legacy delivery channel, got %#v", legacyDelivery)
	}
	if len(legacyDelivery.TaskIDs) != 1 || legacyDelivery.TaskIDs[0] != "task-2" {
		t.Fatalf("expected only task-2 in legacy delivery, got %#v", legacyDelivery.TaskIDs)
	}
}

func TestBuildOperatorTasksLockedLinksHumanRequestToSemanticallyMatchedTask(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	b := NewBroker()
	b.mu.Lock()
	defer b.mu.Unlock()

	b.appendMessageLocked(channelMessage{
		ID:        "msg-1",
		From:      "you",
		Channel:   "ExampleWorkflow-web-azure",
		Content:   "@ceo Realize uma revisão abrangente do código da base. Produza um relatório .md priorizado.",
		Timestamp: "2026-04-24T01:58:07Z",
	})

	b.tasks = []teamTask{{
		ID:        "task-1",
		Channel:   "ExampleWorkflow-web-azure",
		Title:     "Revisao tecnica abrangente da base ExampleAzureRepo",
		Details:   "Produzir relatorio .md priorizado com achados P0-P3.",
		Owner:     "reviewer",
		Status:    "in_progress",
		TaskType:  "research",
		CreatedAt: "2026-04-24T01:58:18Z",
		UpdatedAt: "2026-04-24T01:58:18Z",
	}}
	b.requests = []humanInterview{{
		ID:        "request-1",
		From:      "ceo",
		Channel:   "ExampleWorkflow-web-azure",
		Status:    "pending",
		Question:  "Qual task cobre este pedido?",
		ReplyTo:   "msg-1",
		CreatedAt: "2026-04-24T01:58:18Z",
		UpdatedAt: "2026-04-24T01:58:29Z",
	}}

	tasks := b.buildOperatorTasksLocked("ExampleWorkflow-web-azure", false, true, "", "human")

	var humanTask *teamTask
	for i := range tasks {
		if tasks[i].ID == "human-request-request-1" {
			humanTask = &tasks[i]
			break
		}
	}
	if humanTask == nil {
		t.Fatalf("expected derived human task, got %#v", tasks)
	}
	if humanTask.SourceTaskID != "task-1" {
		t.Fatalf("expected request to link to task-1, got %#v", humanTask)
	}
	if humanTask.DeliveryID != "delivery-task-task-1" {
		t.Fatalf("expected delivery-task-task-1, got %#v", humanTask)
	}
}

func TestBuildOperatorTasksLockedPrefersExplicitWorkspaceMatch(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	workspaceRoot := t.TempDir()
	targetRepo := filepath.Join(workspaceRoot, "ExampleAzureRepo")
	initUsableGitWorktree(t, targetRepo)

	b := NewBroker()
	b.mu.Lock()
	defer b.mu.Unlock()

	b.appendMessageLocked(channelMessage{
		ID:        "msg-1",
		From:      "you",
		Channel:   "ExampleWorkflow-web-azure",
		Content:   "@ceo Realize uma revisão abrangente da base. Faça isso limitado ao repositório " + targetRepo,
		Timestamp: "2026-04-24T01:58:07Z",
	})

	b.tasks = []teamTask{
		{
			ID:        "task-1",
			Channel:   "ExampleWorkflow-web-azure",
			Title:     "Revisao tecnica abrangente da base DunderIA",
			Details:   "Produzir relatorio .md priorizado.",
			Owner:     "reviewer",
			Status:    "in_progress",
			TaskType:  "research",
			CreatedAt: "2026-04-24T01:58:18Z",
			UpdatedAt: "2026-04-24T01:58:18Z",
		},
		{
			ID:            "task-2",
			Channel:       "ExampleWorkflow-web-azure",
			Title:         "Revisao tecnica abrangente da base ExampleAzureRepo",
			Details:       "Produzir relatorio .md priorizado.",
			Owner:         "reviewer",
			Status:        "in_progress",
			TaskType:      "research",
			ExecutionMode: "external_workspace",
			WorkspacePath: targetRepo,
			CreatedAt:     "2026-04-24T01:58:18Z",
			UpdatedAt:     "2026-04-24T01:58:18Z",
		},
	}
	b.requests = []humanInterview{{
		ID:        "request-1",
		From:      "ceo",
		Channel:   "ExampleWorkflow-web-azure",
		Status:    "pending",
		Question:  "Qual task cobre este pedido?",
		ReplyTo:   "msg-1",
		CreatedAt: "2026-04-24T01:58:18Z",
		UpdatedAt: "2026-04-24T01:58:29Z",
	}}

	tasks := b.buildOperatorTasksLocked("ExampleWorkflow-web-azure", false, true, "", "human")

	var humanTask *teamTask
	for i := range tasks {
		if tasks[i].ID == "human-request-request-1" {
			humanTask = &tasks[i]
			break
		}
	}
	if humanTask == nil {
		t.Fatalf("expected derived human task, got %#v", tasks)
	}
	if humanTask.SourceTaskID != "task-2" {
		t.Fatalf("expected request to link to explicit workspace task-2, got %#v", humanTask)
	}
}

func TestRequestGameMasterRecommendationLockedCreatesTaskAndPrompt(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	b := NewBroker()
	b.mu.Lock()
	defer b.mu.Unlock()

	ensureTestMemberAccess(b, "general", "ceo", "CEO")
	ensureTestMemberAccess(b, "general", "game-master", "Game Master")

	b.appendMessageLocked(channelMessage{
		ID:        "msg-1",
		From:      "ceo",
		Channel:   "general",
		Content:   "Need a human decision here.",
		Timestamp: "2026-04-23T12:00:00Z",
	})
	b.requests = []humanInterview{{
		ID:        "request-1",
		From:      "ceo",
		Channel:   "general",
		Title:     "Need reply",
		Question:  "What should the human answer?",
		Status:    "pending",
		ReplyTo:   "msg-1",
		CreatedAt: "2026-04-23T12:01:00Z",
		UpdatedAt: "2026-04-23T12:01:00Z",
	}}

	req, task, prompt, err := b.requestGameMasterRecommendationLocked("request-1", "human")
	if err != nil {
		t.Fatalf("requestGameMasterRecommendationLocked returned error: %v", err)
	}
	if req.RecommendationStatus != "requested" {
		t.Fatalf("expected request recommendation status requested, got %q", req.RecommendationStatus)
	}
	if task.Owner != "game-master" || task.TaskType != "human_triage" {
		t.Fatalf("expected triage task for game-master, got %#v", task)
	}
	if task.SourceRequestID != "request-1" {
		t.Fatalf("expected task to point back to request-1, got %#v", task)
	}
	if prompt.ReplyTo != "msg-1" {
		t.Fatalf("expected prompt to stay in the same thread, got %#v", prompt)
	}
	if !containsString(prompt.Tagged, "game-master") {
		t.Fatalf("expected prompt to tag game-master, got %#v", prompt)
	}
	if !strings.Contains(prompt.Content, "@game-master prepare a short recommendation") {
		t.Fatalf("unexpected prompt content %q", prompt.Content)
	}
	if len(b.tasks) != 1 {
		t.Fatalf("expected 1 recommendation task, got %#v", b.tasks)
	}
	if len(b.messages) != 2 {
		t.Fatalf("expected root message plus recommendation prompt, got %#v", b.messages)
	}
}
