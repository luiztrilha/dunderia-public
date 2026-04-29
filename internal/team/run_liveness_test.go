package team

import (
	"strings"
	"testing"
	"time"
)

func TestClassifyRunLivenessDetectsPlanOnlyOfficeTask(t *testing.T) {
	task := &teamTask{
		ID:            "task-1",
		TaskType:      "delivery",
		ExecutionMode: "office",
		Status:        "in_progress",
		Title:         "Publish the launch update",
	}

	result := classifyRunLiveness(runLivenessInput{
		RunSucceeded:       true,
		Task:               task,
		HadSubstantivePost: true,
		MessageContent:     "Vou revisar o contexto e em seguida preparo a resposta.",
	})

	if result.State != runLivenessPlanOnly {
		t.Fatalf("state = %q, want %q (%s)", result.State, runLivenessPlanOnly, result.Reason)
	}
}

func TestClassifyRunLivenessAcceptsResearchNarrativeProgress(t *testing.T) {
	task := &teamTask{
		ID:            "task-1",
		TaskType:      "research",
		ExecutionMode: "office",
		Status:        "in_progress",
		Title:         "Research competitor positioning",
	}

	result := classifyRunLiveness(runLivenessInput{
		RunSucceeded:       true,
		Task:               task,
		HadSubstantivePost: true,
		MessageContent:     "Vou comparar os tres concorrentes e consolidar os achados no canal.",
	})

	if result.State != runLivenessAdvanced {
		t.Fatalf("state = %q, want %q (%s)", result.State, runLivenessAdvanced, result.Reason)
	}
}

func TestClassifyRunLivenessDetectsEmptySuccessfulResponse(t *testing.T) {
	result := classifyRunLiveness(runLivenessInput{RunSucceeded: true})

	if result.State != runLivenessEmptyResponse {
		t.Fatalf("state = %q, want %q (%s)", result.State, runLivenessEmptyResponse, result.Reason)
	}
}

func TestClassifyRunLivenessAcceptsDurableCompletion(t *testing.T) {
	task := &teamTask{ID: "task-1", Status: "done", ReviewState: "not_required"}

	result := classifyRunLiveness(runLivenessInput{
		RunSucceeded: true,
		Task:         task,
	})

	if result.State != runLivenessCompleted {
		t.Fatalf("state = %q, want %q (%s)", result.State, runLivenessCompleted, result.Reason)
	}
}

func TestHeadlessTurnCompletedDurablyRejectsPlanOnlyOfficeTask(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "gtm", "GTM")
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Publish the launch update",
		Owner:         "gtm",
		CreatedBy:     "ceo",
		TaskType:      "delivery",
		ExecutionMode: "office",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	startedAt := time.Now().UTC().Add(-2 * time.Second)
	if _, err := b.PostMessage("gtm", "general", "Vou revisar o contexto e em seguida preparo a resposta.", nil, ""); err != nil {
		t.Fatalf("post message: %v", err)
	}

	l := newHeadlessLauncherForTest()
	l.broker = b

	ok, reason := l.headlessTurnCompletedDurably("gtm", &headlessCodexActiveTurn{
		Turn:      headlessCodexTurn{TaskID: task.ID},
		StartedAt: startedAt,
	})
	if ok {
		t.Fatal("expected plan-only office task to be rejected")
	}
	if !strings.Contains(reason, "future work") {
		t.Fatalf("expected plan-only failure reason, got %q", reason)
	}
	activity := b.activity[agentLaneKey("general", "gtm")]
	if activity.LivenessState != string(runLivenessPlanOnly) || activity.LivenessTaskID != task.ID {
		t.Fatalf("expected plan-only liveness activity, got %+v", activity)
	}
}

func TestHeadlessTurnCompletedDurablyAcceptsOfficeTaskMutation(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Publish the launch update",
		Owner:         "gtm",
		CreatedBy:     "ceo",
		TaskType:      "delivery",
		ExecutionMode: "office",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	startedAt := time.Now().UTC().Add(-2 * time.Second)
	b.appendActionLocked("task_updated", "office", "general", "gtm", "Publish the launch update [in_progress]", task.ID)

	l := newHeadlessLauncherForTest()
	l.broker = b

	ok, reason := l.headlessTurnCompletedDurably("gtm", &headlessCodexActiveTurn{
		Turn:      headlessCodexTurn{TaskID: task.ID},
		StartedAt: startedAt,
	})
	if !ok {
		t.Fatalf("expected task mutation to satisfy office liveness, got %q", reason)
	}
}

func TestHeadlessTurnCompletedDurablyAcceptsBlockedOfficeTask(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Publish the launch update",
		Owner:         "gtm",
		CreatedBy:     "ceo",
		TaskType:      "delivery",
		ExecutionMode: "office",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	for i := range b.tasks {
		if b.tasks[i].ID != task.ID {
			continue
		}
		b.tasks[i].Status = "blocked"
		b.tasks[i].Blocked = true
		break
	}

	l := newHeadlessLauncherForTest()
	l.broker = b

	ok, reason := l.headlessTurnCompletedDurably("gtm", &headlessCodexActiveTurn{
		Turn:      headlessCodexTurn{TaskID: task.ID},
		StartedAt: time.Now().UTC().Add(-2 * time.Second),
	})
	if !ok {
		t.Fatalf("expected blocked task to satisfy office liveness, got %q", reason)
	}
}
