package team

import (
	"strings"
	"testing"
	"time"
)

func TestLauncherSessionObservabilitySnapshotIncludesQueuesAndActivity(t *testing.T) {
	b := NewBroker()
	b.members = []officeMember{{Slug: "ceo"}, {Slug: "eng"}}
	b.requests = []humanInterview{{ID: "req-1", Status: "pending"}}
	b.SetReadinessSnapshot(brokerReadinessSnapshot{
		Live:    true,
		Ready:   false,
		State:   "starting",
		Stage:   "runtime_warmup",
		Summary: "warming runtime",
	})
	b.tasks = []teamTask{{
		ID:    "task-1",
		Title: "Publish GitHub issue",
		IssuePublication: &taskGitHubPublication{
			Status: "pending",
		},
		PRPublication: &taskGitHubPublication{
			Status: "deferred",
		},
	}}
	b.gitHubPublicationAudit = taskGitHubAuditState{
		LastRunAt:     "2026-04-22T18:00:00Z",
		LastSuccessAt: "2026-04-22T18:00:00Z",
		OpenedTotal:   5,
		UpdatedTotal:  3,
		DeferredTotal: 2,
	}
	b.activity = map[string]agentActivitySnapshot{
		agentLaneKey("engineering", "eng"): {Slug: "eng", Channel: "engineering", Status: "active", Activity: "tool_use", Detail: "running tests"},
	}
	b.scheduler = []schedulerJob{
		{Slug: "job-1", Kind: "task_follow_up", TargetType: "task", TargetID: "task-1", Channel: "general", NextRun: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339), DueAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339), Status: "scheduled"},
		{Slug: "job-2", Kind: "task_follow_up", TargetType: "task", TargetID: "task-2", Channel: "general", NextRun: time.Now().UTC().Add(time.Minute).Format(time.RFC3339), DueAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339), Status: "scheduled"},
		{Slug: "job-3", Kind: "task_follow_up", TargetType: "task", TargetID: "task-3", Channel: "general", Status: "done"},
	}
	updateBrokerCloudBackupRuntime(func(state *brokerCloudBackupRuntime) {
		*state = brokerCloudBackupRuntime{
			Pending:             true,
			LastAttemptAt:       "2026-04-23T12:00:00Z",
			LastSuccessAt:       "2026-04-23T11:59:00Z",
			LastError:           "temporary gcs 429",
			ConsecutiveFailures: 2,
		}
	})
	t.Cleanup(func() {
		updateBrokerCloudBackupRuntime(func(state *brokerCloudBackupRuntime) {
			*state = brokerCloudBackupRuntime{}
		})
	})
	b.recordHTTPMetric("/messages", 200, 25*time.Millisecond)
	b.observabilityHistory = []brokerObservabilitySample{{
		RecordedAt:      "2026-04-23T12:05:00Z",
		ReadinessState:  "starting",
		ReadinessStage:  "runtime_warmup",
		PendingRequests: 1,
		SchedulerDue:    1,
		GitHubPending:   1,
	}}

	l := &Launcher{
		provider:            "codex",
		broker:              b,
		headlessActive:      map[string]*headlessCodexActiveTurn{agentLaneKey("engineering", "eng"): {Turn: headlessCodexTurn{Channel: "engineering", TaskID: "task-9"}, StartedAt: time.Unix(1700000000, 0), WorkspaceDir: "/tmp/task-9"}},
		headlessQueues:      map[string][]headlessCodexTurn{agentLaneKey("engineering", "eng"): {{Channel: "engineering", TaskID: "task-10"}}},
		headlessQuickQueues: map[string][]headlessCodexTurn{"ceo": {{Prompt: "quick reply"}}},
	}

	got := l.SessionObservabilitySnapshot()
	if got.Provider != "codex" || got.PendingRequests != 1 {
		t.Fatalf("unexpected snapshot header: %+v", got)
	}
	if got.ActiveAgents != 1 || got.QueuedTurns != 2 {
		t.Fatalf("unexpected snapshot counts: %+v", got)
	}
	if got.ReadinessState != "starting" || got.ReadinessStage != "runtime_warmup" || got.ReadinessReady {
		t.Fatalf("unexpected readiness snapshot: %+v", got)
	}
	if got.SchedulerScheduled != 2 || got.SchedulerDue != 1 || got.SchedulerTerminal != 1 {
		t.Fatalf("unexpected scheduler snapshot: %+v", got)
	}
	if got.GitHubPending != 1 || got.GitHubDeferred != 1 || got.GitHubOpened != 0 {
		t.Fatalf("unexpected github publication counts: %+v", got)
	}
	if got.GitHubLastAuditAt == "" || got.GitHubLastAuditSuccess == "" {
		t.Fatalf("expected github audit metadata, got %+v", got)
	}
	if got.GitHubOpenedTotal != 5 || got.GitHubUpdatedTotal != 3 || got.GitHubDeferredTotal != 2 {
		t.Fatalf("expected github audit totals, got %+v", got)
	}
	if !got.CloudBackup.Pending || got.CloudBackup.ConsecutiveFailures != 2 {
		t.Fatalf("expected cloud backup runtime details, got %+v", got.CloudBackup)
	}
	if len(got.HTTPHotPaths) != 1 || got.HTTPHotPaths[0].Path != "/messages" {
		t.Fatalf("expected hot path metric, got %+v", got.HTTPHotPaths)
	}
	if len(got.History) != 1 || got.History[0].RecordedAt != "2026-04-23T12:05:00Z" {
		t.Fatalf("expected observability history, got %+v", got.History)
	}
	if len(got.Agents) != 2 {
		t.Fatalf("expected two agent entries, got %+v", got.Agents)
	}
	var eng SessionAgentObservability
	for _, agent := range got.Agents {
		if agent.Slug == "eng" {
			eng = agent
			break
		}
	}
	if eng.Channel != "engineering" || eng.ActiveTaskID != "task-9" || eng.QueueDepth != 1 || eng.WorkingDirectory != "/tmp/task-9" {
		t.Fatalf("unexpected eng observability: %+v", eng)
	}
}

func TestSessionObservabilityFormatLinesIncludesQueueAndTask(t *testing.T) {
	lines := SessionObservabilitySnapshot{
		Provider:               "codex",
		ReadinessState:         "starting",
		ReadinessStage:         "runtime_warmup",
		ReadinessSummary:       "warming runtime",
		ActiveAgents:           1,
		QueuedTurns:            2,
		PendingRequests:        1,
		SchedulerScheduled:     2,
		SchedulerDue:           1,
		SchedulerTerminal:      1,
		GitHubPending:          1,
		GitHubDeferred:         1,
		GitHubOpened:           2,
		GitHubLastAuditAt:      "2026-04-22T18:00:00Z",
		GitHubLastAuditSuccess: "2026-04-22T18:01:00Z",
		GitHubLastAuditError:   "gh auth unavailable",
		GitHubOpenedTotal:      5,
		GitHubUpdatedTotal:     3,
		GitHubDeferredTotal:    2,
		CloudBackup: brokerCloudBackupRuntime{
			Pending:             true,
			LastAttemptAt:       "2026-04-23T12:00:00Z",
			LastSuccessAt:       "2026-04-23T11:59:00Z",
			LastError:           "temporary gcs 429",
			ConsecutiveFailures: 2,
		},
		HTTPHotPaths: []brokerHTTPMetric{{
			Path:           "/messages",
			Requests:       4,
			LastDurationMs: 25,
			MaxDurationMs:  40,
		}},
		History: []brokerObservabilitySample{{
			RecordedAt:      "2026-04-23T12:05:00Z",
			PendingRequests: 1,
			SchedulerDue:    1,
			GitHubPending:   3,
		}},
		Agents: []SessionAgentObservability{{
			Slug:                 "eng",
			Channel:              "engineering",
			Status:               "active",
			Activity:             "running",
			QueueDepth:           1,
			QuickReplyQueueDepth: 1,
			ActiveTaskID:         "task-9",
			WorkingDirectory:     "/tmp/task-9",
			Detail:               "running tests",
		}},
	}.FormatLines()
	text := strings.Join(lines, "\n")
	for _, want := range []string{
		"Provider: codex",
		"Readiness: starting (runtime_warmup)",
		"Readiness summary: warming runtime",
		"Active agents: 1",
		"Queued turns: 2",
		"Pending requests: 1",
		"Scheduler: scheduled=2 due=1 terminal=1",
		"GitHub publications: pending=1 deferred=1 opened=2",
		"GitHub audit last run: 2026-04-22T18:00:00Z",
		"GitHub audit last success: 2026-04-22T18:01:00Z",
		"GitHub audit error: gh auth unavailable",
		"GitHub totals: opened=5 updated=3 deferred=2",
		"Cloud backup: pending=true failures=2 last_attempt=2026-04-23T12:00:00Z last_success=2026-04-23T11:59:00Z",
		"Cloud backup error: temporary gcs 429",
		"Observability history: 2026-04-23T12:05:00Z due=1 req=1 gh=3",
		"HTTP hot path: /messages max=40ms last=25ms requests=4",
		"@eng · #engineering · status=active · activity=running · queue=1 · quick=1 · task=task-9 · wd=/tmp/task-9 · detail=running tests",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in %q", want, text)
		}
	}
}
