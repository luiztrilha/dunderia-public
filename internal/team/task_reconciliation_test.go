package team

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecoverFailedHeadlessTurnMarksPendingReconciliationForLocalWorktree(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	b := NewBroker()
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Implement the reconciliation guard",
		Owner:         "eng",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	l := newHeadlessLauncherForTest()
	l.broker = b

	l.recoverFailedHeadlessTurn("eng", headlessCodexTurn{
		Prompt:   "Ship #task-" + task.ID,
		Channel:  "general",
		TaskID:   task.ID,
		Attempts: headlessCodexLocalWorktreeRetryLimit,
	}, time.Now().UTC().Add(-2*time.Second), "model failure")

	var updated teamTask
	for _, candidate := range b.AllTasks() {
		if candidate.ID == task.ID {
			updated = candidate
			break
		}
	}
	if updated.ID == "" {
		t.Fatalf("expected to find task %s", task.ID)
	}
	if updated.Reconciliation == nil || updated.Reconciliation.Status != "pending" || !updated.Reconciliation.Blocking {
		t.Fatalf("expected pending reconciliation, got %+v", updated.Reconciliation)
	}
}

func TestMarkTaskReconciliationPendingCapturesChangedAndUntrackedPaths(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	b := NewBroker()
	now := time.Now().UTC().Format(time.RFC3339)
	b.tasks = []teamTask{{
		ID:            "task-1",
		Channel:       "general",
		Title:         "Repair workspace drift",
		Owner:         "builder",
		Status:        "blocked",
		Blocked:       true,
		CreatedBy:     "ceo",
		WorkspacePath: "/tmp/repo",
		CreatedAt:     now,
		UpdatedAt:     now,
	}}

	updated, changed, err := b.MarkTaskReconciliationPending(
		"task-1",
		"/tmp/repo",
		" M internal/team/broker.go\x00?? internal/team/github_publication.go\x00",
		"workspace drift",
	)
	if err != nil || !changed {
		t.Fatalf("mark reconciliation pending: %v changed=%v", err, changed)
	}
	if updated.Reconciliation == nil {
		t.Fatal("expected reconciliation state")
	}
	if len(updated.Reconciliation.ChangedPaths) != 1 || updated.Reconciliation.ChangedPaths[0] != "internal/team/broker.go" {
		t.Fatalf("expected changed path summary, got %+v", updated.Reconciliation)
	}
	if len(updated.Reconciliation.UntrackedPaths) != 1 || updated.Reconciliation.UntrackedPaths[0] != "internal/team/github_publication.go" {
		t.Fatalf("expected untracked path summary, got %+v", updated.Reconciliation)
	}
}

func TestBrokerResumeTaskRejectsPendingReconciliation(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	now := time.Now().UTC().Format(time.RFC3339)
	b.tasks = []teamTask{{
		ID:        "task-1",
		Channel:   "general",
		Title:     "Reconcile workspace drift",
		Owner:     "builder",
		Status:    "blocked",
		Blocked:   true,
		CreatedBy: "ceo",
		CreatedAt: now,
		UpdatedAt: now,
		Reconciliation: &taskReconciliationState{
			Status:   "pending",
			Reason:   "Workspace changed before a valid handoff landed.",
			Blocking: true,
		},
	}}

	if _, changed, err := b.ResumeTask("task-1", "human", "Resume the lane."); err == nil {
		t.Fatal("expected resume to reject pending reconciliation")
	} else if changed {
		t.Fatalf("expected no task change on rejected resume, got changed=%v err=%v", changed, err)
	}
}

func TestBrokerTaskReconcileClearsPendingReconciliation(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Repair workspace drift",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
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
		b.tasks[i].Reconciliation = &taskReconciliationState{
			Status:        "pending",
			Reason:        "Workspace changed before a valid handoff landed.",
			WorkspacePath: b.tasks[i].WorktreePath,
			Blocking:      true,
			DetectedAt:    time.Now().UTC().Format(time.RFC3339),
		}
		break
	}
	if err := b.StartOnPort(0); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer b.Stop()

	base := fmt.Sprintf("http://%s", b.Addr())
	body, _ := json.Marshal(map[string]any{
		"action":     "reconcile",
		"channel":    "general",
		"id":         task.ID,
		"created_by": "builder",
		"details": structuredTaskHandoff(
			"reconcile",
			"Reviewed the workspace delta and reconciled the pending changes.",
			"Continue from the same task with the reconciled workspace state.",
			"",
			"",
		),
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/tasks", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+b.Token())
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("reconcile task request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected reconcile success, got %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Task teamTask `json:"task"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode task response: %v", err)
	}
	if result.Task.Reconciliation != nil {
		t.Fatalf("expected reconciliation to clear, got %+v", result.Task.Reconciliation)
	}
	if result.Task.Status != "in_progress" || result.Task.Blocked {
		t.Fatalf("expected reconciled task to resume, got %+v", result.Task)
	}
}

func TestBuildSessionMemorySnapshotFromOfficeStateCarriesPrescriptiveReconciliationContext(t *testing.T) {
	snapshot := BuildSessionMemorySnapshotFromOfficeState(SessionModeOffice, "", []teamTask{{
		ID:                "task-55",
		Channel:           "release-ops",
		Title:             "Repair workspace drift",
		Owner:             "builder",
		Status:            "blocked",
		Blocked:           true,
		ThreadID:          "msg-55",
		ExecutionMode:     "local_worktree",
		WorktreePath:      "/tmp/wuphf-task-55",
		BlockerRequestIDs: []string{"request-9"},
		LastHandoff: &taskHandoffRecord{
			Blockers: []taskBlocker{{
				ID:        "blocker-1",
				Need:      "production credentials",
				WaitingOn: "@ceo",
			}},
		},
		Reconciliation: &taskReconciliationState{
			Status:         "pending",
			Reason:         "Workspace changed before a valid handoff landed.",
			WorkspacePath:  "/tmp/wuphf-task-55",
			ChangedPaths:   []string{"internal/team/resume.go"},
			UntrackedPaths: []string{"notes/reconcile.txt"},
			Blocking:       true,
		},
	}}, nil, nil, nil)

	if len(snapshot.Tasks) != 1 {
		t.Fatalf("expected one task summary, got %+v", snapshot.Tasks)
	}
	task := snapshot.Tasks[0]
	if !strings.HasPrefix(task.NextAction, "Inspect the pending workspace delta") {
		t.Fatalf("expected prescriptive next action, got %+v", task)
	}
	if task.ReplyChannel != "release-ops" || task.ReplyTo != "msg-55" {
		t.Fatalf("expected explicit reply route, got %+v", task)
	}
	if len(task.ChangedPaths) != 1 || task.ChangedPaths[0] != "internal/team/resume.go" {
		t.Fatalf("expected changed path summary, got %+v", task)
	}
	if len(task.UntrackedPaths) != 1 || task.UntrackedPaths[0] != "notes/reconcile.txt" {
		t.Fatalf("expected untracked path summary, got %+v", task)
	}
	if len(task.RelevantBlockers) < 2 {
		t.Fatalf("expected relevant blockers, got %+v", task.RelevantBlockers)
	}
}

func TestBuildRuntimeSnapshotFormatTextIncludesResumeGuidance(t *testing.T) {
	snapshot := BuildRuntimeSnapshot(RuntimeSnapshotInput{
		Channel: "release-ops",
		Tasks: []RuntimeTask{{
			ID:                    "task-88",
			Title:                 "Repair workspace drift",
			Owner:                 "builder",
			Status:                "blocked",
			Blocked:               true,
			ExecutionMode:         "local_worktree",
			WorktreePath:          "/tmp/wuphf-task-88",
			ThreadID:              "msg-88",
			ReconciliationPending: true,
			NextAction:            "Inspect the pending workspace delta in /tmp/wuphf-task-88, reconcile the changed/untracked files, and submit a structured reconcile handoff before resuming task-88.",
			ReplyChannel:          "release-ops",
			ReplyTo:               "msg-88",
			RelevantBlockers:      []string{"Pending blocker request(s): request-10"},
			ChangedPaths:          []string{"internal/team/runtime_state.go"},
			UntrackedPaths:        []string{"notes/reconcile.txt"},
		}},
		Now: time.Unix(200, 0),
	})

	text := snapshot.FormatText()
	for _, want := range []string{
		"Resume guidance:",
		"task-88: Inspect the pending workspace delta in /tmp/wuphf-task-88, reconcile the changed/untracked files, and submit a structured reconcile handoff before resuming task-88.",
		"Changed paths: internal/team/runtime_state.go",
		"Untracked paths: notes/reconcile.txt",
		"Blockers: Pending blocker request(s): request-10",
		"Reply route: #release-ops -> msg-88",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in runtime snapshot text:\n%s", want, text)
		}
	}
}
