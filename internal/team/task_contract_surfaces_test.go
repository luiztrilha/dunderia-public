package team

import (
	"strings"
	"testing"
	"time"
)

func TestBuildRuntimeSnapshotHighlightsContractRepairNextSteps(t *testing.T) {
	snapshot := BuildRuntimeSnapshot(RuntimeSnapshotInput{
		Channel: "general",
		Tasks: []RuntimeTask{{
			ID:                     "task-1",
			Title:                  "Repair workspace drift",
			Owner:                  "builder",
			Status:                 "blocked",
			PipelineStage:          "implement",
			ExecutionMode:          "local_worktree",
			WorktreePath:           "/tmp/wuphf-task-1",
			HandoffStatus:          "repair_required",
			BlockerRequestIDs:      []string{"request-1"},
			BlockingReviewFindings: 1,
			ReconciliationPending:  true,
		}},
		Requests: []RuntimeRequest{{
			ID:       "request-1",
			Title:    "Need workspace confirmation",
			From:     "builder",
			Status:   "pending",
			Blocking: true,
		}},
		Now: time.Unix(100, 0),
	})

	joined := strings.Join(snapshot.Recovery.NextSteps, "\n")
	for _, want := range []string{
		"reconcile",
		"structured handoff",
		"blocking review finding",
		"blocker request",
	} {
		if !strings.Contains(strings.ToLower(joined), want) {
			t.Fatalf("expected %q in recovery next steps, got %q", want, joined)
		}
	}
}

func TestBuildRuntimeSnapshotHighlightsDeferredGitHubPublicationNextSteps(t *testing.T) {
	snapshot := BuildRuntimeSnapshot(RuntimeSnapshotInput{
		Channel: "general",
		Tasks: []RuntimeTask{{
			ID:                     "task-1",
			Title:                  "Publish GitHub artifacts",
			Owner:                  "builder",
			Status:                 "in_progress",
			PipelineStage:          "implement",
			ExecutionMode:          "external_workspace",
			WorkspacePath:          "/tmp/repo",
			IssuePublicationStatus: "deferred",
			PRPublicationStatus:    "deferred",
		}},
		Now: time.Unix(100, 0),
	})

	joined := strings.ToLower(strings.Join(snapshot.Recovery.NextSteps, "\n"))
	for _, want := range []string{
		"retry github issue publication",
		"retry github pr publication",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in recovery next steps, got %q", want, joined)
		}
	}
}

func TestBuildSessionMemorySnapshotFromOfficeStateCarriesContractFields(t *testing.T) {
	snapshot := BuildSessionMemorySnapshotFromOfficeState(
		SessionModeOffice,
		"",
		[]teamTask{{
			ID:                "task-1",
			Title:             "Repair workspace drift",
			Owner:             "builder",
			Status:            "blocked",
			Blocked:           true,
			BlockerRequestIDs: []string{"request-1"},
			HandoffStatus:     "repair_required",
			ReviewFindings: []taskReviewFinding{{
				ID:       "finding-1",
				Severity: "major",
				Status:   "open",
			}},
			Reconciliation: &taskReconciliationState{
				Status:   "pending",
				Blocking: true,
			},
		}},
		nil,
		nil,
		nil,
	)

	if len(snapshot.Tasks) != 1 {
		t.Fatalf("expected one task summary, got %+v", snapshot.Tasks)
	}
	task := snapshot.Tasks[0]
	if task.HandoffStatus != "repair_required" || !task.ReconciliationPending {
		t.Fatalf("expected contract fields in session memory task summary, got %+v", task)
	}
	if task.BlockingReviewFindings != 1 || len(task.BlockerRequestIDs) != 1 {
		t.Fatalf("expected blocker/finding summary in session memory task summary, got %+v", task)
	}
}

func TestBuildSessionMemorySnapshotFromOfficeStateCarriesGitHubPublicationFields(t *testing.T) {
	snapshot := BuildSessionMemorySnapshotFromOfficeState(
		SessionModeOffice,
		"",
		[]teamTask{{
			ID:     "task-1",
			Title:  "Publish GitHub artifacts",
			Owner:  "builder",
			Status: "review",
			IssuePublication: &taskGitHubPublication{
				Status: "opened",
				URL:    "https://github.com/acme/dunderia/issues/42",
			},
			PRPublication: &taskGitHubPublication{
				Status: "deferred",
				URL:    "https://github.com/acme/dunderia/pull/7",
			},
		}},
		nil,
		nil,
		nil,
	)

	if len(snapshot.Tasks) != 1 {
		t.Fatalf("expected one task summary, got %+v", snapshot.Tasks)
	}
	task := snapshot.Tasks[0]
	if task.IssuePublicationStatus != "opened" || !strings.Contains(task.IssueURL, "/issues/42") {
		t.Fatalf("expected carried issue publication fields, got %+v", task)
	}
	if task.PRPublicationStatus != "deferred" || !strings.Contains(task.PRURL, "/pull/7") {
		t.Fatalf("expected carried pr publication fields, got %+v", task)
	}
}

func TestBuildStudioBlockersIncludesTaskReconciliationPending(t *testing.T) {
	blockers := buildStudioBlockersFromState(studioDevConsoleState{
		Tasks: []teamTask{{
			ID:      "task-1",
			Channel: "general",
			Title:   "Repair workspace drift",
			Owner:   "builder",
			Status:  "blocked",
			Blocked: true,
			Reconciliation: &taskReconciliationState{
				Status:        "pending",
				Reason:        "Workspace changed before a valid handoff landed.",
				WorkspacePath: "D:\\Repos\\dunderia",
				Blocking:      true,
			},
		}},
	})

	found := false
	for _, blocker := range blockers {
		if blocker.Kind == "task_reconciliation_pending" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected reconciliation blocker, got %+v", blockers)
	}
}

func TestBuildStudioBlockersIncludesDeferredGitHubPublication(t *testing.T) {
	blockers := buildStudioBlockersFromState(studioDevConsoleState{
		Tasks: []teamTask{{
			ID:      "task-1",
			Channel: "general",
			Title:   "Publish GitHub artifacts",
			Owner:   "builder",
			Status:  "review",
			IssuePublication: &taskGitHubPublication{
				Status:    "deferred",
				LastError: "gh auth missing",
			},
			PRPublication: &taskGitHubPublication{
				Status:    "deferred",
				LastError: "push failed",
			},
		}},
	})

	foundIssue := false
	foundPR := false
	for _, blocker := range blockers {
		switch blocker.Kind {
		case "github_issue_publication_deferred":
			foundIssue = blocker.RecommendedAction == "retry_issue_publication"
		case "github_pr_publication_deferred":
			foundPR = blocker.RecommendedAction == "retry_pr_publication"
		}
	}
	if !foundIssue || !foundPR {
		t.Fatalf("expected issue and pr publication blockers, got %+v", blockers)
	}
}
