package team

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHandleStudioDevConsoleReturnsSnapshot(t *testing.T) {
	b, _ := newStudioDevConsoleFixture(t)

	req := httptest.NewRequest(http.MethodGet, "/studio/dev-console", nil)
	rec := httptest.NewRecorder()

	b.handleStudioDevConsole(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp studioDevConsoleResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Office.Status != "degraded" {
		t.Fatalf("expected degraded office snapshot, got %+v", resp.Office)
	}
	if !resp.Office.Health.Degraded || resp.Office.Bootstrap.Ready {
		t.Fatalf("expected health to be degraded with bootstrap not ready, got %+v", resp.Office)
	}
	if resp.Office.Bootstrap.Summary == "" || resp.Office.TaskCounts.Total != 5 {
		t.Fatalf("unexpected office bootstrap/counts: %+v", resp.Office)
	}
	if resp.Environment.Status != "degraded" || !resp.Environment.BrokerReachable || !resp.Environment.APIReachable || !resp.Environment.WebReachable {
		t.Fatalf("unexpected environment snapshot: %+v", resp.Environment)
	}
	if len(resp.Environment.Signals) == 0 {
		t.Fatalf("expected degradation signals, got %+v", resp.Environment)
	}
	if resp.ActiveContext.Focus == "" || resp.ActiveContext.PrimaryChannel == "" || len(resp.ActiveContext.Channels) == 0 || len(resp.ActiveContext.Flows) == 0 || len(resp.ActiveContext.Workspaces) == 0 {
		t.Fatalf("expected active context to be populated, got %+v", resp.ActiveContext)
	}

	gotBlockers := map[string]bool{}
	for _, blocker := range resp.Blockers {
		gotBlockers[blocker.Kind] = true
	}
	for _, kind := range []string{
		"task_timeout_no_substantive_update",
		"task_blocked_by_dependency",
		"owner_not_in_channel",
		"direction_without_task",
		"workflow_missing_required_input",
		"degraded_local_environment",
	} {
		if !gotBlockers[kind] {
			t.Fatalf("expected blocker %q, got %+v", kind, resp.Blockers)
		}
	}

	byAction := studioActionDefinitionsByName(resp.Actions)
	if len(byAction) != len(resp.Actions) {
		t.Fatalf("expected unique studio actions, got %+v", resp.Actions)
	}
	for _, action := range []string{
		"retry_task",
		"retry_issue_publication",
		"retry_pr_publication",
		"reassign_task",
		"wake_agents",
		"inspect_task",
		"inspect_channel",
		"create_task",
		"refresh_snapshot",
	} {
		if _, ok := byAction[action]; !ok {
			t.Fatalf("expected studio action %q, got %+v", action, resp.Actions)
		}
	}
	if !byAction["retry_task"].Mutating || !byAction["reassign_task"].Mutating || !byAction["wake_agents"].Mutating {
		t.Fatalf("expected mutating actions to be marked mutating, got %+v", resp.Actions)
	}
	if !byAction["retry_issue_publication"].Mutating || !byAction["retry_issue_publication"].RequiresTaskID {
		t.Fatalf("expected retry_issue_publication to require task context, got %+v", byAction["retry_issue_publication"])
	}
	if !byAction["retry_pr_publication"].Mutating || !byAction["retry_pr_publication"].RequiresTaskID {
		t.Fatalf("expected retry_pr_publication to require task context, got %+v", byAction["retry_pr_publication"])
	}
	if !byAction["inspect_task"].FrontendHandled || !byAction["inspect_channel"].FrontendHandled || !byAction["create_task"].FrontendHandled || !byAction["refresh_snapshot"].FrontendHandled {
		t.Fatalf("expected frontend-handled actions to be flagged, got %+v", resp.Actions)
	}
	if !byAction["wake_agents"].RequiresTaskID || byAction["wake_agents"].RequiresAgent {
		t.Fatalf("expected wake_agents to require only task_id, got %+v", byAction["wake_agents"])
	}
}

func TestBuildStudioBlockersDetectsHeuristicSet(t *testing.T) {
	b, _ := newStudioDevConsoleFixture(t)

	blockers := buildStudioBlockers(b)
	got := make(map[string]bool, len(blockers))
	for _, blocker := range blockers {
		got[blocker.Kind] = true
	}

	for _, kind := range []string{
		"task_timeout_no_substantive_update",
		"task_blocked_by_dependency",
		"owner_not_in_channel",
		"direction_without_task",
		"workflow_missing_required_input",
		"degraded_local_environment",
	} {
		if !got[kind] {
			t.Fatalf("expected blocker kind %q, got %+v", kind, blockers)
		}
	}

	timeout := findStudioBlockerByKind(blockers, "task_timeout_no_substantive_update")
	if timeout == nil || timeout.RecommendedAction != "retry_task" || len(timeout.AvailableActions) < 2 {
		t.Fatalf("unexpected timeout blocker: %+v", timeout)
	}
}

func TestBuildStudioDevConsoleSnapshotAggregatesChannelAttention(t *testing.T) {
	restore := useStudioDevConsoleStatePath(t)
	defer restore()

	b := NewBroker()
	now := time.Now().UTC()

	b.mu.Lock()
	b.members = []officeMember{
		{Slug: "ceo", Name: "CEO"},
		{Slug: "builder", Name: "Builder"},
		{Slug: "watchdog", Name: "Watchdog"},
	}
	b.channels = []teamChannel{
		{Slug: "general", Name: "General", Members: []string{"ceo", "builder", "watchdog"}},
		{Slug: "ops", Name: "Ops", Members: []string{"ceo"}},
	}
	b.tasks = []teamTask{
		{
			ID:           "task-follow-up-1",
			Channel:      "general",
			ExecutionKey: "ceo-conversation-follow-up|incoming|general|msg-1",
			Title:        "Validate unanswered CEO follow-up",
			Owner:        "watchdog",
			Status:       "open",
			TaskType:     "follow_up",
			CreatedAt:    now.Add(-12 * time.Minute).Format(time.RFC3339),
			UpdatedAt:    now.Add(-5 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:           "task-follow-up-2",
			Channel:      "general",
			ExecutionKey: "ceo-conversation-follow-up|outgoing|general|msg-2",
			Title:        "Validate unanswered CEO follow-up",
			Owner:        "watchdog",
			Status:       "open",
			TaskType:     "follow_up",
			CreatedAt:    now.Add(-11 * time.Minute).Format(time.RFC3339),
			UpdatedAt:    now.Add(-2 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:        "task-timeout",
			Channel:   "general",
			Title:     "Slice backend",
			Owner:     "builder",
			Status:    "blocked",
			Blocked:   true,
			Details:   "Automatic timeout recovery: @builder timed out after 4m0s before posting a substantive update.",
			CreatedAt: now.Add(-10 * time.Minute).Format(time.RFC3339),
			UpdatedAt: now.Add(-6 * time.Minute).Format(time.RFC3339),
		},
	}
	b.requests = []humanInterview{
		{
			ID:        "req-1",
			Kind:      "freeform",
			Status:    "pending",
			From:      "human",
			Channel:   "general",
			Title:     "Need approval",
			Question:  "Can the operator approve the next step?",
			Blocking:  true,
			Required:  true,
			CreatedAt: now.Add(-6 * time.Minute).Format(time.RFC3339),
			UpdatedAt: now.Add(-6 * time.Minute).Format(time.RFC3339),
		},
	}
	b.decisions = []officeDecisionRecord{
		{
			ID:        "decision-1",
			Kind:      "remind_owner",
			Channel:   "general",
			Owner:     "ceo",
			Summary:   "@ceo still needs to move Validate unanswered CEO follow-up in #general.",
			CreatedAt: now.Add(-90 * time.Second).Format(time.RFC3339),
		},
	}
	b.watchdogs = []watchdogAlert{
		{
			ID:         "watchdog-1",
			Kind:       "task_stalled",
			Status:     "active",
			Channel:    "general",
			TargetType: "task",
			TargetID:   "task-follow-up-1",
			Owner:      "watchdog",
			Summary:    "@watchdog still needs to move Validate unanswered CEO follow-up in #general.",
			CreatedAt:  now.Add(-3 * time.Minute).Format(time.RFC3339),
			UpdatedAt:  now.Add(-60 * time.Second).Format(time.RFC3339),
		},
	}
	b.executionNodes = []executionNode{
		{
			ID:                  "exec-1",
			Channel:             "general",
			OwnerAgent:          "ceo",
			Status:              "answered",
			AwaitingHumanInput:  true,
			AwaitingHumanReason: "Need approval to continue rollout.",
			CreatedAt:           now.Add(-4 * time.Minute).Format(time.RFC3339),
			UpdatedAt:           now.Add(-45 * time.Second).Format(time.RFC3339),
		},
	}
	b.messages = []channelMessage{
		{
			ID:        "msg-1",
			From:      "builder",
			Channel:   "general",
			Content:   "Applied the backend patch and ran the focused test slice locally.",
			Timestamp: now.Add(-75 * time.Second).Format(time.RFC3339),
		},
		{
			ID:        "msg-2",
			From:      "builder",
			Channel:   "general",
			Content:   "[STATUS] still running local verification",
			Timestamp: now.Add(-30 * time.Second).Format(time.RFC3339),
		},
	}
	b.webUIOrigins = []string{"http://localhost:7891"}
	b.mu.Unlock()

	resp := buildStudioDevConsoleSnapshot(b)
	channel := findStudioChannelBySlug(resp.ActiveContext.Channels, "general")
	if channel == nil {
		t.Fatalf("expected general channel snapshot, got %+v", resp.ActiveContext.Channels)
	}
	if channel.AttentionCount != 3 {
		t.Fatalf("expected 3 deduped attention groups, got %+v", channel)
	}
	if channel.WaitingHumanCount != 1 {
		t.Fatalf("expected deduped visible human queue count, got %+v", channel)
	}
	if channel.ActiveOwnerCount != 2 {
		t.Fatalf("expected active owners watchdog and builder, got %+v", channel)
	}
	if channel.LastSubstantiveUpdateBy != "builder" {
		t.Fatalf("expected builder as latest substantive updater, got %+v", channel)
	}
	if !strings.Contains(channel.LastSubstantivePreview, "focused test slice") {
		t.Fatalf("expected substantive preview from non-status message, got %+v", channel)
	}
	if channel.LastDecisionSummary == "" || !strings.Contains(channel.LastDecisionSummary, "still needs to move") {
		t.Fatalf("expected latest decision summary, got %+v", channel)
	}
	followUp := findStudioAttentionByKind(channel.Attention, "follow_up_queue")
	if followUp == nil || followUp.Count != 2 {
		t.Fatalf("expected deduped follow-up attention group, got %+v", channel.Attention)
	}
}

func TestStudioChannelSnapshotsSortByAttentionBeforeTaskLoad(t *testing.T) {
	now := time.Now().UTC()
	state := studioDevConsoleState{
		Channels: []teamChannel{
			{Slug: "general", Name: "General", Members: []string{"ceo", "watchdog"}},
			{Slug: "ops", Name: "Ops", Members: []string{"ceo", "builder"}},
		},
		Tasks: []teamTask{
			{
				ID:           "task-follow-up-1",
				Channel:      "general",
				ExecutionKey: "ceo-conversation-follow-up|incoming|general|msg-1",
				Title:        "Validate unanswered CEO follow-up",
				Owner:        "watchdog",
				Status:       "open",
				TaskType:     "follow_up",
				CreatedAt:    now.Add(-5 * time.Minute).Format(time.RFC3339),
				UpdatedAt:    now.Add(-4 * time.Minute).Format(time.RFC3339),
			},
			{
				ID:           "task-follow-up-2",
				Channel:      "general",
				ExecutionKey: "ceo-conversation-follow-up|outgoing|general|msg-2",
				Title:        "Validate unanswered CEO follow-up",
				Owner:        "watchdog",
				Status:       "open",
				TaskType:     "follow_up",
				CreatedAt:    now.Add(-4 * time.Minute).Format(time.RFC3339),
				UpdatedAt:    now.Add(-3 * time.Minute).Format(time.RFC3339),
			},
			{
				ID:        "task-ops",
				Channel:   "ops",
				Title:     "Ship ops slice",
				Owner:     "builder",
				Status:    "in_progress",
				CreatedAt: now.Add(-6 * time.Minute).Format(time.RFC3339),
				UpdatedAt: now.Add(-2 * time.Minute).Format(time.RFC3339),
			},
		},
	}

	tasks := studioTaskSnapshotsFromTasks(state.Tasks)
	channels := studioChannelSnapshotsFromState(state, tasks, nil, nil)
	if len(channels) < 2 {
		t.Fatalf("expected two channel snapshots, got %+v", channels)
	}
	if channels[0].Slug != "general" {
		t.Fatalf("expected attention-heavy channel to sort first, got %+v", channels)
	}
	if channels[0].AttentionCount != 1 {
		t.Fatalf("expected deduped follow-up queue to count as one attention group, got %+v", channels[0])
	}
}

func TestStudioChannelSnapshotsIgnoreTerminalTaskBlockersForAttention(t *testing.T) {
	now := time.Now().UTC()
	state := studioDevConsoleState{
		Channels: []teamChannel{
			{Slug: "general", Name: "General", Members: []string{"ceo", "builder"}},
		},
		Tasks: []teamTask{
			{
				ID:            "task-done-owner",
				Channel:       "general",
				Title:         "Closed owner mismatch",
				Owner:         "human",
				Status:        "done",
				ExecutionMode: "external_workspace",
				WorkspacePath: "Z:\\missing\\workspace",
				CreatedAt:     now.Add(-5 * time.Minute).Format(time.RFC3339),
				UpdatedAt:     now.Add(-4 * time.Minute).Format(time.RFC3339),
			},
			{
				ID:            "task-canceled-env",
				Channel:       "general",
				Title:         "Canceled degraded workspace",
				Owner:         "human",
				Status:        "canceled",
				ExecutionMode: "external_workspace",
				WorkspacePath: "Z:\\missing\\workspace",
				CreatedAt:     now.Add(-3 * time.Minute).Format(time.RFC3339),
				UpdatedAt:     now.Add(-2 * time.Minute).Format(time.RFC3339),
			},
		},
	}

	tasks := studioTaskSnapshotsFromTasks(state.Tasks)
	blockers := buildStudioBlockersFromState(state)
	channels := studioChannelSnapshotsFromState(state, tasks, nil, blockers)
	general := findStudioChannelBySlug(channels, "general")
	if general == nil {
		t.Fatalf("expected general channel snapshot, got %+v", channels)
	}
	if general.AttentionCount != 0 || len(general.Attention) != 0 || len(general.Blockers) != 0 {
		t.Fatalf("expected terminal task blockers to be hidden from channel attention, got %+v", general)
	}
}

func TestHandleStudioDevConsoleActionRetriesTask(t *testing.T) {
	restore := useStudioDevConsoleStatePath(t)
	defer restore()

	b := NewBroker()
	now := time.Now().UTC()
	b.tasks = []teamTask{{
		ID:        "task-1",
		Channel:   "general",
		Title:     "Slice backend",
		Owner:     "builder",
		Status:    "blocked",
		Blocked:   true,
		Details:   "Automatic timeout recovery: @builder timed out after 4m0s before posting a substantive update.",
		CreatedAt: now.Add(-15 * time.Minute).Format(time.RFC3339),
		UpdatedAt: now.Add(-10 * time.Minute).Format(time.RFC3339),
	}}

	body := bytes.NewBufferString(`{"action":"retry_task","task_id":"task-1","actor":"human"}`)
	req := httptest.NewRequest(http.MethodPost, "/studio/dev-console/action", body)
	rec := httptest.NewRecorder()

	b.handleStudioDevConsoleAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp studioDevConsoleActionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK || resp.Action != "retry_task" || resp.TaskID != "task-1" {
		t.Fatalf("unexpected retry response: %+v", resp)
	}
	if resp.Task == nil || resp.Task.Blocked || resp.Task.Status != "in_progress" {
		t.Fatalf("expected task to resume, got %+v", resp.Task)
	}

	task, ok := b.FindTask("general", "task-1")
	if !ok {
		t.Fatal("expected task to remain findable")
	}
	if task.Blocked || task.Status != "in_progress" {
		t.Fatalf("expected broker task to be resumed, got %+v", task)
	}

	actions := b.Actions()
	if len(actions) == 0 || actions[len(actions)-1].Kind != "task_unblocked" {
		t.Fatalf("expected task_unblocked action, got %+v", actions)
	}
}

func TestHandleStudioDevConsoleActionReassignsTask(t *testing.T) {
	restore := useStudioDevConsoleStatePath(t)
	defer restore()

	b := NewBroker()
	b.tasks = []teamTask{{
		ID:      "task-1",
		Channel: "general",
		Title:   "Slice backend",
		Owner:   "builder",
		Status:  "blocked",
		Blocked: true,
	}}

	body := bytes.NewBufferString(`{"action":"reassign_task","task_id":"task-1","owner":"ops","actor":"human"}`)
	req := httptest.NewRequest(http.MethodPost, "/studio/dev-console/action", body)
	rec := httptest.NewRecorder()

	b.handleStudioDevConsoleAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp studioDevConsoleActionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK || resp.Action != "reassign_task" || resp.TaskID != "task-1" {
		t.Fatalf("unexpected reassign response: %+v", resp)
	}
	if resp.Task == nil || resp.Task.Owner != "ops" || resp.Task.Status != "in_progress" {
		t.Fatalf("expected task to be reassigned, got %+v", resp.Task)
	}
}

func TestHandleStudioDevConsoleActionWakesAgents(t *testing.T) {
	restore := useStudioDevConsoleStatePath(t)
	defer restore()

	b := NewBroker()
	b.tasks = []teamTask{{
		ID:      "task-1",
		Channel: "general",
		Title:   "Slice backend",
		Owner:   "builder",
		Status:  "blocked",
		Blocked: true,
	}}

	body := bytes.NewBufferString(`{"action":"wake_agents","task_id":"task-1","actor":"human"}`)
	req := httptest.NewRequest(http.MethodPost, "/studio/dev-console/action", body)
	rec := httptest.NewRecorder()

	b.handleStudioDevConsoleAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp studioDevConsoleActionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK || resp.Action != "wake_agents" || resp.TaskID != "task-1" {
		t.Fatalf("unexpected wake response: %+v", resp)
	}
	if resp.Alert == nil || resp.Alert.Kind != "studio_wake_agents" {
		t.Fatalf("expected watchdog alert, got %+v", resp.Alert)
	}

	alerts := b.Watchdogs()
	if len(alerts) != 1 || alerts[0].Kind != "studio_wake_agents" || alerts[0].TargetID != "task-1" {
		t.Fatalf("expected persisted watchdog alert, got %+v", alerts)
	}
	actions := b.Actions()
	if len(actions) == 0 || actions[len(actions)-1].Kind != "watchdog_alert" {
		t.Fatalf("expected watchdog action log, got %+v", actions)
	}
}

func TestHandleStudioDevConsoleActionRejectsWakeAgentsWithoutTaskID(t *testing.T) {
	b := NewBroker()
	req := httptest.NewRequest(http.MethodPost, "/studio/dev-console/action", bytes.NewBufferString(`{"action":"wake_agents","channel":"general","agent":"builder","actor":"human"}`))
	rec := httptest.NewRecorder()

	b.handleStudioDevConsoleAction(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "task_id required") {
		t.Fatalf("unexpected error message: %s", rec.Body.String())
	}
}

func TestStudioDevConsoleActionsCatalogIncludesBackendAndFrontendHandledActions(t *testing.T) {
	actions := studioDevConsoleActions()
	defs := studioActionDefinitionsByName(actions)

	for _, action := range []string{"retry_task", "reassign_task", "wake_agents", "inspect_task", "inspect_channel", "create_task", "refresh_snapshot"} {
		if _, ok := defs[action]; !ok {
			t.Fatalf("expected action %q in catalog, got %+v", action, actions)
		}
	}
	if !defs["retry_task"].Mutating || !defs["reassign_task"].Mutating || !defs["wake_agents"].Mutating {
		t.Fatalf("expected backend mutating actions to be marked mutating, got %+v", actions)
	}
	if !defs["inspect_task"].FrontendHandled || !defs["inspect_channel"].FrontendHandled || !defs["create_task"].FrontendHandled || !defs["refresh_snapshot"].FrontendHandled {
		t.Fatalf("expected frontend-handled actions to be flagged, got %+v", actions)
	}
	if !defs["wake_agents"].RequiresTaskID || defs["wake_agents"].RequiresAgent {
		t.Fatalf("expected wake_agents to require only task_id, got %+v", defs["wake_agents"])
	}
}

func TestStudioDevConsoleActionRejectsUnknownAction(t *testing.T) {
	b := NewBroker()
	req := httptest.NewRequest(http.MethodPost, "/studio/dev-console/action", bytes.NewBufferString(`{"action":"noop"}`))
	rec := httptest.NewRecorder()

	b.handleStudioDevConsoleAction(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unsupported studio action") {
		t.Fatalf("unexpected error message: %s", rec.Body.String())
	}
}

func useStudioDevConsoleStatePath(t *testing.T) func() {
	t.Helper()
	prev := brokerStatePath
	prevPrepare := prepareTaskWorktree
	prevCleanup := cleanupTaskWorktree
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	prepareTaskWorktree = func(taskID string) (string, string, error) {
		path := filepath.Join(tmpDir, "task-worktrees", taskID)
		if err := os.MkdirAll(path, 0o755); err != nil {
			return "", "", err
		}
		return path, "wuphf-test-" + taskID, nil
	}
	cleanupTaskWorktree = func(path, branch string) error { return nil }
	return func() {
		brokerStatePath = prev
		prepareTaskWorktree = prevPrepare
		cleanupTaskWorktree = prevCleanup
	}
}

func newStudioDevConsoleFixture(t *testing.T) (*Broker, time.Time) {
	t.Helper()
	restore := useStudioDevConsoleStatePath(t)
	t.Cleanup(restore)

	b := NewBroker()
	now := time.Now().UTC()
	b.mu.Lock()
	b.runtimeProvider = "codex"
	b.sessionMode = "studio"
	b.oneOnOneAgent = "builder"
	b.focusMode = true
	b.webUIOrigins = []string{"http://localhost:7891"}
	b.members = []officeMember{
		{Slug: "ceo", Name: "CEO"},
		{Slug: "builder", Name: "Builder"},
		{Slug: "qa", Name: "QA"},
	}
	b.channels = []teamChannel{
		{Slug: "general", Name: "General", Members: []string{"ceo", "builder"}},
		{Slug: "ops", Name: "Ops", Members: []string{"ceo"}},
		{Slug: "qa", Name: "QA", Members: []string{"ceo", "qa"}},
		{Slug: "sales", Name: "Sales", Members: []string{"ceo"}},
	}
	b.tasks = []teamTask{
		{
			ID:        "task-1",
			Channel:   "general",
			Title:     "Slice backend",
			Owner:     "builder",
			Status:    "blocked",
			Blocked:   true,
			Details:   "Automatic timeout recovery: @builder timed out after 4m0s before posting a substantive update.",
			CreatedAt: now.Add(-15 * time.Minute).Format(time.RFC3339),
			UpdatedAt: now.Add(-10 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:        "task-2",
			Channel:   "general",
			Title:     "Dependency cleanup",
			Owner:     "builder",
			Status:    "blocked",
			Blocked:   true,
			DependsOn: []string{"task-missing"},
			CreatedAt: now.Add(-14 * time.Minute).Format(time.RFC3339),
			UpdatedAt: now.Add(-11 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:        "task-3",
			Channel:   "ops",
			Title:     "Reassign owner cleanup",
			Owner:     "builder",
			Status:    "in_progress",
			CreatedAt: now.Add(-13 * time.Minute).Format(time.RFC3339),
			UpdatedAt: now.Add(-9 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:           "task-4",
			Channel:      "qa",
			Title:        "Workflow needs input",
			Owner:        "qa",
			Status:       "in_progress",
			TaskType:     "workflow",
			ExecutionKey: "wf-4",
			Details:      "Missing required input: connection_key.",
			CreatedAt:    now.Add(-12 * time.Minute).Format(time.RFC3339),
			UpdatedAt:    now.Add(-8 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:            "task-5",
			Channel:       "qa",
			Title:         "Local environment cleanup",
			Owner:         "qa",
			Status:        "in_progress",
			ExecutionMode: "local_worktree",
			WorkspacePath: "<DUNDERIA_REPO>",
			CreatedAt:     now.Add(-11 * time.Minute).Format(time.RFC3339),
			UpdatedAt:     now.Add(-7 * time.Minute).Format(time.RFC3339),
		},
	}
	b.requests = []humanInterview{
		{
			ID:        "req-1",
			Kind:      "freeform",
			Status:    "pending",
			From:      "human",
			Channel:   "sales",
			Title:     "Need direction",
			Question:  "Need direction and next step for sales.",
			Blocking:  true,
			CreatedAt: now.Add(-3 * time.Minute).Format(time.RFC3339),
			UpdatedAt: now.Add(-3 * time.Minute).Format(time.RFC3339),
		},
	}
	b.messages = []channelMessage{
		{ID: "msg-1", From: "human", Channel: "general", Content: "Need a checkpoint.", Timestamp: now.Add(-2 * time.Minute).Format(time.RFC3339)},
	}
	b.actions = []officeActionLog{
		{ID: "action-1", Kind: "task_updated", Source: "office", Channel: "general", Actor: "builder", Summary: "Slice backend [blocked]", RelatedID: "task-1", CreatedAt: now.Add(-11 * time.Minute).Format(time.RFC3339)},
	}
	b.mu.Unlock()
	return b, now
}

func studioActionDefinitionsByName(defs []studioActionDefinition) map[string]studioActionDefinition {
	out := make(map[string]studioActionDefinition, len(defs))
	for _, def := range defs {
		out[def.Action] = def
	}
	return out
}

func findStudioBlockerByKind(blockers []studioBlocker, kind string) *studioBlocker {
	for i := range blockers {
		if blockers[i].Kind == kind {
			return &blockers[i]
		}
	}
	return nil
}

func findStudioChannelBySlug(channels []studioChannelSnapshot, slug string) *studioChannelSnapshot {
	for i := range channels {
		if channels[i].Slug == slug {
			return &channels[i]
		}
	}
	return nil
}

func findStudioAttentionByKind(values []studioAttentionGroup, kind string) *studioAttentionGroup {
	for i := range values {
		if values[i].Kind == kind {
			return &values[i]
		}
	}
	return nil
}
