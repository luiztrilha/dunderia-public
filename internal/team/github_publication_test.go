package team

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func initGitHubPublicationRepo(t *testing.T) (string, string) {
	t.Helper()

	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
		return strings.TrimSpace(string(out))
	}

	run("init", "-b", "main")
	run("config", "user.email", "dunderia@example.com")
	run("config", "user.name", "DunderIA Tests")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# repo\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	run("add", "README.md")
	run("commit", "-m", "initial")
	run("remote", "add", "origin", "git@github.com:acme/dunderia-test.git")
	run("checkout", "-b", "feature/strict-contracts")
	if err := os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatalf("write feature: %v", err)
	}
	run("add", "feature.txt")
	run("commit", "-m", "feature")
	return repo, "feature/strict-contracts"
}

func setChannelLinkedRepoDefaults(t *testing.T, b *Broker, channel, repo string, defaults *taskGitHubPublicationPolicy, primary bool) {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := b.findChannelLocked(channel)
	if ch == nil {
		t.Fatalf("channel %q not found", channel)
	}
	found := -1
	for i := range ch.LinkedRepos {
		if sameCleanPath(ch.LinkedRepos[i].RepoPath, repo) {
			found = i
			break
		}
	}
	if found < 0 {
		ch.LinkedRepos = append(ch.LinkedRepos, linkedRepoRef{
			RepoPath:            repo,
			Primary:             primary || len(ch.LinkedRepos) == 0,
			PublicationDefaults: cloneTaskGitHubPublicationPolicy(defaults),
		})
		found = len(ch.LinkedRepos) - 1
	} else {
		ch.LinkedRepos[found].PublicationDefaults = cloneTaskGitHubPublicationPolicy(defaults)
		if primary {
			ch.LinkedRepos[found].Primary = true
		}
	}
	if primary {
		for i := range ch.LinkedRepos {
			ch.LinkedRepos[i].Primary = i == found
		}
	}
}

func TestEnsurePlannedTaskQueuesGitHubIssuePublicationForExecution(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	oldAsync := scheduleTaskGitHubAsync
	scheduleTaskGitHubAsync = func(fn func()) {}
	defer func() { scheduleTaskGitHubAsync = oldAsync }()

	oldRun := taskGitHubRunCommand
	taskGitHubRunCommand = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		switch {
		case name == "gh" && len(args) == 2 && args[0] == "auth" && args[1] == "status":
			return []byte("ok"), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test":
			return []byte(`{"visibility":"private","private":true,"default_branch":"main"}`), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "search/issues":
			return []byte(`{"items":[]}`), nil
		case name == "gh" && len(args) >= 4 && args[0] == "api" && args[1] == "repos/acme/dunderia-test/issues":
			return []byte(`{"number":42,"html_url":"https://github.com/acme/dunderia-test/issues/42","node_id":"I_kwDOTEST42","state":"open"}`), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
		}
	}
	defer func() { taskGitHubRunCommand = oldRun }()

	repo, _ := initGitHubPublicationRepo(t)
	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")

	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Ship the broker contract",
		Details:       "Implement the strict execution slice.",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "external_workspace",
		WorkspacePath: repo,
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	if task.IssuePublication == nil || task.IssuePublication.Status != "pending" {
		t.Fatalf("expected pending issue publication after execution start, got %+v", task.IssuePublication)
	}

	updated, changed, err := b.publishTaskIssueNow(task.ID)
	if err != nil || !changed {
		t.Fatalf("publish issue: %v changed=%v", err, changed)
	}
	if updated.IssuePublication == nil || updated.IssuePublication.Status != "opened" {
		t.Fatalf("expected opened issue publication, got %+v", updated.IssuePublication)
	}
	if updated.IssuePublication.Number != 42 || !strings.Contains(updated.IssuePublication.URL, "/issues/42") {
		t.Fatalf("expected persisted issue metadata, got %+v", updated.IssuePublication)
	}
}

func TestPublishTaskIssueDefersPublicRepoWhenRedactionRiskDetected(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	oldAsync := scheduleTaskGitHubAsync
	scheduleTaskGitHubAsync = func(fn func()) {}
	defer func() { scheduleTaskGitHubAsync = oldAsync }()

	oldRun := taskGitHubRunCommand
	taskGitHubRunCommand = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		switch {
		case name == "gh" && len(args) == 2 && args[0] == "auth" && args[1] == "status":
			return []byte("ok"), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test":
			return []byte(`{"visibility":"public","private":false,"default_branch":"main"}`), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "search/issues":
			return []byte(`{"items":[]}`), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
		}
	}
	defer func() { taskGitHubRunCommand = oldRun }()

	repo, _ := initGitHubPublicationRepo(t)
	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")

	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Protect public repo publication",
		Details:       "Internal path D:\\Repos\\dunderia\\secret.txt should never leave the office.",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "external_workspace",
		WorkspacePath: repo,
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	for i := range b.tasks {
		if b.tasks[i].ID == task.ID {
			b.tasks[i].PublicationPolicy = &taskGitHubPublicationPolicy{Enabled: "approved"}
			break
		}
	}

	updated, changed, err := b.publishTaskIssueNow(task.ID)
	if err != nil || !changed {
		t.Fatalf("publish issue: %v changed=%v", err, changed)
	}
	if updated.IssuePublication == nil || updated.IssuePublication.Status != "deferred" {
		t.Fatalf("expected deferred issue publication for redaction risk, got %+v", updated.IssuePublication)
	}
	if !strings.Contains(updated.IssuePublication.LastError, "redaction_required") {
		t.Fatalf("expected explicit redaction blocker, got %+v", updated.IssuePublication)
	}
}

func TestTaskReviewQueuesGitHubPRPublicationAndRetryClearsDeferred(t *testing.T) {
	tmpDir := isolateBrokerPersistenceEnv(t)
	_ = tmpDir

	oldAsync := scheduleTaskGitHubAsync
	scheduleTaskGitHubAsync = func(fn func()) {}
	defer func() { scheduleTaskGitHubAsync = oldAsync }()

	ghFailures := 1
	oldRun := taskGitHubRunCommand
	taskGitHubRunCommand = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		switch {
		case name == "gh" && len(args) == 2 && args[0] == "auth" && args[1] == "status":
			return []byte("ok"), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test":
			return []byte(`{"visibility":"private","private":true,"default_branch":"main"}`), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "search/issues":
			return []byte(`{"items":[]}`), nil
		case name == "gh" && len(args) >= 4 && args[0] == "api" && args[1] == "repos/acme/dunderia-test/issues":
			return []byte(`{"number":42,"html_url":"https://github.com/acme/dunderia-test/issues/42","node_id":"I_kwDOTEST42","state":"open"}`), nil
		case name == "gh" && len(args) >= 4 && args[0] == "api" && args[1] == "repos/acme/dunderia-test/pulls":
			if ghFailures > 0 {
				ghFailures--
				return nil, fmt.Errorf("simulated gh failure")
			}
			return []byte(`{"number":7,"html_url":"https://github.com/acme/dunderia-test/pull/7","node_id":"PR_kwDOTEST7","state":"open"}`), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
		}
	}
	defer func() { taskGitHubRunCommand = oldRun }()

	oldGit := taskGitHubRunGit
	taskGitHubRunGit = func(dir string, args ...string) error { return nil }
	defer func() { taskGitHubRunGit = oldGit }()

	repo, branch := initGitHubPublicationRepo(t)
	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	if err := b.StartOnPort(0); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer b.Stop()

	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Ship the broker contract",
		Details:       "Implement the strict execution slice.",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "external_workspace",
		WorkspacePath: repo,
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	task.WorktreeBranch = branch
	for i := range b.tasks {
		if b.tasks[i].ID == task.ID {
			b.tasks[i].WorktreeBranch = branch
			break
		}
	}
	if _, _, err := b.publishTaskIssueNow(task.ID); err != nil {
		t.Fatalf("publish issue: %v", err)
	}

	base := fmt.Sprintf("http://%s", b.Addr())
	body, _ := json.Marshal(map[string]any{
		"action":     "review",
		"channel":    "general",
		"id":         task.ID,
		"created_by": "builder",
		"details": structuredTaskHandoff(
			"review_ready",
			"Implemented the change and prepared it for review.",
			"Review the branch and validate the focused tests.",
			"",
			"",
		),
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/tasks", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+b.Token())
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("task review request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected review success, got %d", resp.StatusCode)
	}

	var reviewResult struct {
		Task teamTask `json:"task"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reviewResult); err != nil {
		t.Fatalf("decode review response: %v", err)
	}
	if reviewResult.Task.PRPublication == nil || reviewResult.Task.PRPublication.Status != "pending" {
		t.Fatalf("expected pending pr publication after review, got %+v", reviewResult.Task.PRPublication)
	}

	updated, changed, err := b.publishTaskPRNow(task.ID)
	if err != nil || !changed {
		t.Fatalf("publish pr: %v changed=%v", err, changed)
	}
	if updated.PRPublication == nil || updated.PRPublication.Status != "deferred" {
		t.Fatalf("expected deferred pr publication after failure, got %+v", updated.PRPublication)
	}

	retried, changed, err := b.RetryTaskPRPublication(task.ID, "studio")
	if err != nil || !changed {
		t.Fatalf("retry pr publication: %v changed=%v", err, changed)
	}
	if retried.PRPublication == nil || retried.PRPublication.Status != "pending" {
		t.Fatalf("expected pending pr publication after retry, got %+v", retried.PRPublication)
	}

	updated, changed, err = b.publishTaskPRNow(task.ID)
	if err != nil || !changed {
		t.Fatalf("publish pr after retry: %v changed=%v", err, changed)
	}
	if updated.PRPublication == nil || updated.PRPublication.Status != "opened" {
		t.Fatalf("expected opened pr publication after retry, got %+v", updated.PRPublication)
	}
	if updated.PRPublication.Number != 7 || !strings.Contains(updated.PRPublication.URL, "/pull/7") {
		t.Fatalf("expected persisted pr metadata, got %+v", updated.PRPublication)
	}
}

func TestPublishTaskIssueAdoptsExistingGitHubArtifactByMarker(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	oldAsync := scheduleTaskGitHubAsync
	scheduleTaskGitHubAsync = func(fn func()) {}
	defer func() { scheduleTaskGitHubAsync = oldAsync }()

	oldRun := taskGitHubRunCommand
	taskGitHubRunCommand = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		switch {
		case name == "gh" && len(args) == 2 && args[0] == "auth" && args[1] == "status":
			return []byte("ok"), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test":
			return []byte(`{"visibility":"private","private":true,"default_branch":"main"}`), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "search/issues":
			return []byte(`{"items":[{"number":51,"html_url":"https://github.com/acme/dunderia-test/issues/51","node_id":"I_kwDOADOPT51","state":"closed"}]}`), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
		}
	}
	defer func() { taskGitHubRunCommand = oldRun }()

	repo, _ := initGitHubPublicationRepo(t)
	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")

	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Adopt existing issue",
		Details:       "Use the already-open GitHub demand.",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "external_workspace",
		WorkspacePath: repo,
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	updated, changed, err := b.publishTaskIssueNow(task.ID)
	if err != nil || !changed {
		t.Fatalf("publish issue: %v changed=%v", err, changed)
	}
	if updated.IssuePublication == nil || updated.IssuePublication.Number != 51 {
		t.Fatalf("expected adopted issue metadata, got %+v", updated.IssuePublication)
	}
	if updated.IssuePublication.State != "closed" || updated.IssuePublication.ExternalID != "I_kwDOADOPT51" {
		t.Fatalf("expected adopted remote state, got %+v", updated.IssuePublication)
	}
}

func TestReconcileTaskGitHubPublicationsRefreshesOpenedPRState(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	oldRun := taskGitHubRunCommand
	taskGitHubRunCommand = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		switch {
		case name == "gh" && len(args) == 2 && args[0] == "auth" && args[1] == "status":
			return []byte("ok"), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test":
			return []byte(`{"visibility":"private","private":true,"default_branch":"main"}`), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test/pulls/7":
			return []byte(`{"number":7,"html_url":"https://github.com/acme/dunderia-test/pull/7","node_id":"PR_kwDOOPEN7","state":"closed","draft":false,"merged_at":"2026-04-22T18:00:00Z"}`), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test/issues/7":
			return []byte(`{"number":7,"html_url":"https://github.com/acme/dunderia-test/pull/7","node_id":"PR_kwDOOPEN7","state":"closed","title":"Refresh PR state","body":"Review body","labels":[{"name":"dunderia"}]}`), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
		}
	}
	defer func() { taskGitHubRunCommand = oldRun }()

	repo, branch := initGitHubPublicationRepo(t)
	b := NewBroker()
	now := time.Now().UTC().Format(time.RFC3339)
	b.tasks = []teamTask{{
		ID:             "task-1",
		Channel:        "general",
		Title:          "Refresh PR state",
		Owner:          "builder",
		Status:         "review",
		CreatedBy:      "ceo",
		TaskType:       "feature",
		ExecutionMode:  "external_workspace",
		WorkspacePath:  repo,
		WorktreeBranch: branch,
		CreatedAt:      now,
		UpdatedAt:      now,
		PRPublication: &taskGitHubPublication{
			Status:     "opened",
			Number:     7,
			URL:        "https://github.com/acme/dunderia-test/pull/7",
			HeadBranch: branch,
			BaseBranch: "main",
			Title:      "Refresh PR state",
		},
	}}

	state, err := b.ReconcileTaskGitHubPublications(time.Now().UTC())
	if err != nil {
		t.Fatalf("reconcile publications: %v", err)
	}
	if state.OpenedCount != 1 || state.LastSuccessAt == "" {
		t.Fatalf("expected successful audit state, got %+v", state)
	}
	updated := b.AllTasks()[0]
	if updated.PRPublication == nil || updated.PRPublication.State != "merged" {
		t.Fatalf("expected merged PR state after refresh, got %+v", updated.PRPublication)
	}
	if updated.PRPublication.LastSyncedAt == "" || updated.PRPublication.ExternalID != "PR_kwDOOPEN7" {
		t.Fatalf("expected synced PR metadata, got %+v", updated.PRPublication)
	}
}

func TestRefreshTaskGitHubPublicationCleansMergedRemoteBranchForTerminalTask(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	repo, branch := initGitHubPublicationRepo(t)
	b := NewBroker()
	now := time.Now().UTC().Format(time.RFC3339)
	b.tasks = []teamTask{{
		ID:             "task-1",
		Channel:        "general",
		Title:          "Cleanup merged branch",
		Owner:          "builder",
		Status:         "done",
		ReviewState:    "approved",
		CreatedBy:      "ceo",
		TaskType:       "feature",
		ExecutionMode:  "external_workspace",
		WorkspacePath:  repo,
		WorktreeBranch: branch,
		CreatedAt:      now,
		UpdatedAt:      now,
		PRPublication: &taskGitHubPublication{
			Status:     "opened",
			Number:     7,
			URL:        "https://github.com/acme/dunderia-test/pull/7",
			HeadBranch: branch,
			BaseBranch: "main",
			Title:      "Cleanup merged branch",
		},
	}}

	oldRun := taskGitHubRunCommand
	taskGitHubRunCommand = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		switch {
		case name == "gh" && len(args) == 2 && args[0] == "auth" && args[1] == "status":
			return []byte("ok"), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test":
			return []byte(`{"visibility":"private","private":true,"default_branch":"main"}`), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test/pulls/7":
			return []byte(`{"number":7,"html_url":"https://github.com/acme/dunderia-test/pull/7","node_id":"PR_kwDOOPEN7","state":"closed","draft":false,"merged_at":"2026-04-22T18:00:00Z"}`), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test/issues/7":
			return []byte(`{"number":7,"html_url":"https://github.com/acme/dunderia-test/pull/7","node_id":"PR_kwDOOPEN7","state":"closed","title":"Cleanup merged branch","body":"Review body","labels":[{"name":"dunderia"}]}`), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
		}
	}
	defer func() { taskGitHubRunCommand = oldRun }()

	oldGitOutput := taskGitHubGitOutput
	taskGitHubGitOutput = func(dir string, args ...string) ([]byte, error) {
		if len(args) >= 4 && args[0] == "ls-remote" && args[1] == "--heads" && args[2] == "origin" && args[3] == branch {
			return []byte("abc123\trefs/heads/" + branch), nil
		}
		return oldGitOutput(dir, args...)
	}
	defer func() { taskGitHubGitOutput = oldGitOutput }()

	var deletedArgs []string
	oldGit := taskGitHubRunGit
	taskGitHubRunGit = func(dir string, args ...string) error {
		deletedArgs = append([]string(nil), args...)
		return nil
	}
	defer func() { taskGitHubRunGit = oldGit }()

	updated, changed, err := b.refreshTaskGitHubPublicationNow("task-1", "pr")
	if err != nil || !changed {
		t.Fatalf("refresh pr publication: %v changed=%v", err, changed)
	}
	if updated.PRPublication == nil || updated.PRPublication.State != "merged" {
		t.Fatalf("expected merged PR state after refresh, got %+v", updated.PRPublication)
	}
	if strings.Join(deletedArgs, " ") != fmt.Sprintf("push origin --delete %s", branch) {
		t.Fatalf("expected merged branch cleanup push delete, got %v", deletedArgs)
	}
}

func TestRefreshTaskGitHubPublicationAppliesRemoteReviewSignals(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	repo, branch := initGitHubPublicationRepo(t)
	b := NewBroker()
	now := time.Now().UTC().Format(time.RFC3339)
	b.tasks = []teamTask{{
		ID:             "task-1",
		Channel:        "general",
		Title:          "Refresh PR review signals",
		Details:        "Implement the strict execution slice.",
		Owner:          "builder",
		Status:         "in_progress",
		CreatedBy:      "ceo",
		TaskType:       "feature",
		ExecutionMode:  "external_workspace",
		WorkspacePath:  repo,
		WorktreeBranch: branch,
		CreatedAt:      now,
		UpdatedAt:      now,
		PRPublication: &taskGitHubPublication{
			Status: "opened",
			Number: 7,
			URL:    "https://github.com/acme/dunderia-test/pull/7",
			Title:  "Refresh PR review signals",
			State:  "open",
			Draft:  true,
		},
	}}
	expectedTitle := buildTaskGitHubPRTitle(&b.tasks[0])
	expectedBody := buildTaskGitHubPRBody(&b.tasks[0])

	oldRun := taskGitHubRunCommand
	taskGitHubRunCommand = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		switch {
		case name == "gh" && len(args) == 2 && args[0] == "auth" && args[1] == "status":
			return []byte("ok"), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test":
			return []byte(`{"visibility":"private","private":true,"default_branch":"main"}`), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test/pulls/7":
			return []byte(`{"number":7,"html_url":"https://github.com/acme/dunderia-test/pull/7","node_id":"PR_kwDOOPEN7","state":"open","draft":true}`), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test/issues/7":
			return []byte(fmt.Sprintf(`{"number":7,"html_url":"https://github.com/acme/dunderia-test/pull/7","node_id":"PR_kwDOOPEN7","state":"open","title":%q,"body":%q,"labels":[{"name":"dunderia"},{"name":"feature"},{"name":"channel-general"}]}`, expectedTitle, expectedBody)), nil
		case name == "gh" && len(args) >= 5 && args[0] == "pr" && args[1] == "view" && args[2] == "7":
			return []byte(`{"reviewDecision":"CHANGES_REQUESTED","statusCheckRollup":[{"name":"ci/test","conclusion":"FAILURE"}]}`), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
		}
	}
	defer func() { taskGitHubRunCommand = oldRun }()

	updated, changed, err := b.refreshTaskGitHubPublicationNow("task-1", "pr")
	if err != nil || !changed {
		t.Fatalf("refresh pr publication: %v changed=%v", err, changed)
	}
	if updated.PRPublication == nil {
		t.Fatalf("expected pr publication metadata, got nil")
	}
	if updated.PRPublication.ReviewDecision != "changes_requested" {
		t.Fatalf("expected review decision to sync, got %+v", updated.PRPublication)
	}
	if updated.PRPublication.ChecksState != "failing" {
		t.Fatalf("expected failing checks state, got %+v", updated.PRPublication)
	}
	if updated.Status != "review" || updated.ReviewState != "changes_requested" {
		t.Fatalf("expected task to move to review/changes_requested, got %+v", updated)
	}
}

func TestRefreshTaskGitHubPublicationCapturesRemoteCommentSummary(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	repo, branch := initGitHubPublicationRepo(t)
	b := NewBroker()
	now := time.Now().UTC().Format(time.RFC3339)
	b.tasks = []teamTask{{
		ID:             "task-1",
		Channel:        "general",
		Title:          "Refresh PR comment summary",
		Details:        "Sync remote review comments.",
		Owner:          "builder",
		Status:         "review",
		CreatedBy:      "ceo",
		TaskType:       "feature",
		ExecutionMode:  "external_workspace",
		WorkspacePath:  repo,
		WorktreeBranch: branch,
		CreatedAt:      now,
		UpdatedAt:      now,
		PRPublication: &taskGitHubPublication{
			Status: "opened",
			Number: 7,
			URL:    "https://github.com/acme/dunderia-test/pull/7",
			Title:  "Refresh PR comment summary",
			State:  "open",
		},
	}}
	expectedTitle := buildTaskGitHubPRTitle(&b.tasks[0])
	expectedBody := buildTaskGitHubPRBody(&b.tasks[0])

	oldRun := taskGitHubRunCommand
	taskGitHubRunCommand = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		switch {
		case name == "gh" && len(args) == 2 && args[0] == "auth" && args[1] == "status":
			return []byte("ok"), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test":
			return []byte(`{"visibility":"private","private":true,"default_branch":"main"}`), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test/pulls/7":
			return []byte(`{"number":7,"html_url":"https://github.com/acme/dunderia-test/pull/7","node_id":"PR_kwDOOPEN7","state":"open","draft":false}`), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test/issues/7":
			return []byte(fmt.Sprintf(`{"number":7,"html_url":"https://github.com/acme/dunderia-test/pull/7","node_id":"PR_kwDOOPEN7","state":"open","title":%q,"body":%q,"labels":[{"name":"dunderia"},{"name":"feature"},{"name":"channel-general"}]}`, expectedTitle, expectedBody)), nil
		case name == "gh" && len(args) >= 5 && args[0] == "pr" && args[1] == "view" && args[2] == "7":
			return []byte(`{"reviewDecision":"APPROVED","statusCheckRollup":[{"name":"ci/test","conclusion":"SUCCESS"}],"comments":[{"body":"Please rename this helper before merge.","createdAt":"2026-04-23T10:00:00Z","author":{"login":"reviewer-a"}},{"body":"Looks aligned after the latest push.","updatedAt":"2026-04-23T11:30:00Z","author":{"login":"reviewer-b"}}]}`), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
		}
	}
	defer func() { taskGitHubRunCommand = oldRun }()

	updated, changed, err := b.refreshTaskGitHubPublicationNow("task-1", "pr")
	if err != nil || !changed {
		t.Fatalf("refresh pr publication: %v changed=%v", err, changed)
	}
	if updated.PRPublication == nil {
		t.Fatalf("expected pr publication metadata, got nil")
	}
	if updated.PRPublication.CommentCount != 2 {
		t.Fatalf("expected comment count to sync, got %+v", updated.PRPublication)
	}
	if updated.PRPublication.LatestCommentAuthor != "reviewer-b" || updated.PRPublication.LatestCommentAt != "2026-04-23T11:30:00Z" {
		t.Fatalf("expected latest comment metadata, got %+v", updated.PRPublication)
	}
	if !strings.Contains(updated.PRPublication.LatestCommentSnippet, "latest push") {
		t.Fatalf("expected latest comment snippet, got %+v", updated.PRPublication)
	}
}

func TestStoreTaskGitHubPRReviewSnapshotNoopWhenUnchanged(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	b := NewBroker()
	now := time.Now().UTC().Format(time.RFC3339)
	b.tasks = []teamTask{{
		ID:          "task-1",
		Channel:     "general",
		Title:       "No-op review snapshot",
		Status:      "review",
		ReviewState: "approved",
		CreatedAt:   now,
		UpdatedAt:   now,
		PRPublication: &taskGitHubPublication{
			Status:               "opened",
			Number:               7,
			ReviewDecision:       "approved",
			ChecksState:          "passing",
			ChecksSummary:        []string{"ci/test: success"},
			CommentCount:         1,
			LatestCommentAt:      "2026-04-23T11:30:00Z",
			LatestCommentAuthor:  "reviewer-b",
			LatestCommentSnippet: "Looks aligned after the latest push.",
		},
	}}

	updated, changed, err := b.storeTaskGitHubPRReviewSnapshot("task-1", &taskGitHubRemoteReviewSnapshot{
		ReviewDecision:       "approved",
		ChecksState:          "passing",
		ChecksSummary:        []string{"ci/test: success"},
		CommentCount:         1,
		LatestCommentAt:      "2026-04-23T11:30:00Z",
		LatestCommentAuthor:  "reviewer-b",
		LatestCommentSnippet: "Looks aligned after the latest push.",
	})
	if err != nil {
		t.Fatalf("store review snapshot: %v", err)
	}
	if changed {
		t.Fatalf("expected unchanged review snapshot to be a no-op, got %+v", updated.PRPublication)
	}
}

func TestEnsureGitHubPublicationAuditJobRegistersRecurringJob(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	b := NewBroker()
	if err := b.EnsureGitHubPublicationAuditJob(); err != nil {
		t.Fatalf("ensure github audit job: %v", err)
	}
	jobs := b.Scheduler()
	found := false
	for _, job := range jobs {
		if job.Slug == taskGitHubAuditJobSlug {
			found = true
			if job.Kind != taskGitHubAuditJobKind || job.TargetType != taskGitHubAuditTargetType || job.IntervalMinutes <= 0 {
				t.Fatalf("unexpected github audit job: %+v", job)
			}
		}
	}
	if !found {
		t.Fatalf("expected github audit job in scheduler, got %+v", jobs)
	}
}

func TestPublishTaskIssueRequiresExplicitApprovalForPublicRepo(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	oldAsync := scheduleTaskGitHubAsync
	scheduleTaskGitHubAsync = func(fn func()) {}
	defer func() { scheduleTaskGitHubAsync = oldAsync }()

	allowCreate := false
	oldRun := taskGitHubRunCommand
	taskGitHubRunCommand = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		switch {
		case name == "gh" && len(args) == 2 && args[0] == "auth" && args[1] == "status":
			return []byte("ok"), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test":
			return []byte(`{"visibility":"public","private":false,"default_branch":"main"}`), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "search/issues":
			return []byte(`{"items":[]}`), nil
		case name == "gh" && len(args) >= 4 && args[0] == "api" && args[1] == "repos/acme/dunderia-test/issues":
			if !allowCreate {
				t.Fatalf("unexpected issue creation without explicit approval")
			}
			return []byte(`{"number":88,"html_url":"https://github.com/acme/dunderia-test/issues/88","node_id":"I_kwDOPUBLIC88","state":"open"}`), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
		}
	}
	defer func() { taskGitHubRunCommand = oldRun }()

	repo, _ := initGitHubPublicationRepo(t)
	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")

	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Protect public repo publication",
		Details:       "Do not publish without an explicit approval flag.",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "external_workspace",
		WorkspacePath: repo,
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	updated, changed, err := b.publishTaskIssueNow(task.ID)
	if err != nil || !changed {
		t.Fatalf("publish issue: %v changed=%v", err, changed)
	}
	if updated.IssuePublication == nil || updated.IssuePublication.Status != "deferred" {
		t.Fatalf("expected deferred issue publication for public repo without approval, got %+v", updated.IssuePublication)
	}
	if !strings.Contains(updated.IssuePublication.LastError, "public_repo_requires_approval") {
		t.Fatalf("expected explicit public repo approval blocker, got %+v", updated.IssuePublication)
	}

	for i := range b.tasks {
		if b.tasks[i].ID == task.ID {
			b.tasks[i].PublicationPolicy = &taskGitHubPublicationPolicy{Enabled: "approved"}
			break
		}
	}
	retried, changed, err := b.RetryTaskIssuePublication(task.ID, "studio")
	if err != nil || !changed {
		t.Fatalf("retry issue publication: %v changed=%v", err, changed)
	}
	if retried.IssuePublication == nil || retried.IssuePublication.Status != "pending" {
		t.Fatalf("expected pending issue publication after approval+retry, got %+v", retried.IssuePublication)
	}

	allowCreate = true
	updated, changed, err = b.publishTaskIssueNow(task.ID)
	if err != nil || !changed {
		t.Fatalf("publish approved issue: %v changed=%v", err, changed)
	}
	if updated.IssuePublication == nil || updated.IssuePublication.Status != "opened" || updated.IssuePublication.Number != 88 {
		t.Fatalf("expected opened issue after explicit approval, got %+v", updated.IssuePublication)
	}
}

func TestResolveTaskGitHubRepoContextUsesLinkedRepoPublicationDefaults(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	repo, _ := initGitHubPublicationRepo(t)
	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	setChannelLinkedRepoDefaults(t, b, "general", repo, &taskGitHubPublicationPolicy{
		Enabled:   "approved",
		IssueMode: "immediate",
		SyncMode:  "manual",
		Labels:    []string{"ops"},
	}, true)

	task := &teamTask{
		ID:            "task-1",
		Channel:       "general",
		Title:         "Repo defaults",
		ExecutionMode: "external_workspace",
		WorkspacePath: repo,
	}
	repoCtx, err := b.resolveTaskGitHubRepoContext(task)
	if err != nil {
		t.Fatalf("resolve repo context: %v", err)
	}
	if repoCtx == nil || repoCtx.PublicationDefaults == nil {
		t.Fatalf("expected repo publication defaults, got %+v", repoCtx)
	}
	if repoCtx.PublicationDefaults.Enabled != "approved" || repoCtx.PublicationDefaults.IssueMode != "immediate" || repoCtx.PublicationDefaults.SyncMode != "manual" {
		t.Fatalf("expected linked repo defaults to be preserved, got %+v", repoCtx.PublicationDefaults)
	}
	policy := effectiveTaskGitHubPublicationPolicy(task, repoCtx)
	if !slicesContains(policy.Labels, "ops") || !slicesContains(policy.Labels, "dunderia") {
		t.Fatalf("expected effective policy labels to include repo defaults and base label, got %+v", policy.Labels)
	}
}

func TestResolveTaskGitHubRepoContextFallsBackToPrimaryLinkedRepoDefaults(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	primaryRepo, _ := initGitHubPublicationRepo(t)
	secondaryRepo, _ := initGitHubPublicationRepo(t)
	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	setChannelLinkedRepoDefaults(t, b, "general", primaryRepo, &taskGitHubPublicationPolicy{
		Enabled: "approved",
		Labels:  []string{"primary-default"},
	}, true)

	task := &teamTask{
		ID:            "task-1",
		Channel:       "general",
		Title:         "Primary fallback",
		ExecutionMode: "external_workspace",
		WorkspacePath: secondaryRepo,
	}
	repoCtx, err := b.resolveTaskGitHubRepoContext(task)
	if err != nil {
		t.Fatalf("resolve repo context with primary fallback: %v", err)
	}
	if repoCtx == nil || repoCtx.PublicationDefaults == nil || repoCtx.PublicationDefaults.Enabled != "approved" {
		t.Fatalf("expected primary linked repo defaults to apply, got %+v", repoCtx)
	}
	if !slicesContains(repoCtx.PublicationDefaults.Labels, "primary-default") {
		t.Fatalf("expected primary fallback label, got %+v", repoCtx.PublicationDefaults)
	}
}

func TestPublishTaskIssueUsesLinkedRepoPublicationApprovalDefaults(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	oldAsync := scheduleTaskGitHubAsync
	scheduleTaskGitHubAsync = func(fn func()) {}
	defer func() { scheduleTaskGitHubAsync = oldAsync }()

	oldRun := taskGitHubRunCommand
	taskGitHubRunCommand = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		switch {
		case name == "gh" && len(args) == 2 && args[0] == "auth" && args[1] == "status":
			return []byte("ok"), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test":
			return []byte(`{"visibility":"public","private":false,"default_branch":"main"}`), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "search/issues":
			return []byte(`{"items":[]}`), nil
		case name == "gh" && len(args) >= 4 && args[0] == "api" && args[1] == "repos/acme/dunderia-test/issues":
			return []byte(`{"number":77,"html_url":"https://github.com/acme/dunderia-test/issues/77","node_id":"I_kwDODEFAULT77","state":"open"}`), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
		}
	}
	defer func() { taskGitHubRunCommand = oldRun }()

	repo, _ := initGitHubPublicationRepo(t)
	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	setChannelLinkedRepoDefaults(t, b, "general", repo, &taskGitHubPublicationPolicy{
		Enabled: "approved",
		Labels:  []string{"triage"},
	}, true)

	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Public repo with linked default approval",
		Details:       "Publish using repo-level approval defaults.",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "external_workspace",
		WorkspacePath: repo,
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	updated, changed, err := b.publishTaskIssueNow(task.ID)
	if err != nil || !changed {
		t.Fatalf("publish issue with linked defaults: %v changed=%v", err, changed)
	}
	if updated.IssuePublication == nil || updated.IssuePublication.Status != "opened" || updated.IssuePublication.Number != 77 {
		t.Fatalf("expected opened issue using repo-level approval defaults, got %+v", updated.IssuePublication)
	}
}

func TestReconcileTaskGitHubPublicationsUpdatesAndReopensIssueLifecycle(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	repo, _ := initGitHubPublicationRepo(t)
	b := NewBroker()
	now := time.Now().UTC().Format(time.RFC3339)
	b.tasks = []teamTask{{
		ID:            "task-1",
		Channel:       "general",
		Title:         "Ship the broker contract",
		Details:       "Implement the strict execution slice.",
		Owner:         "builder",
		Status:        "in_progress",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "external_workspace",
		WorkspacePath: repo,
		CreatedAt:     now,
		UpdatedAt:     now,
		IssuePublication: &taskGitHubPublication{
			Status: "opened",
			Number: 42,
			URL:    "https://github.com/acme/dunderia-test/issues/42",
			Title:  "Old title",
			State:  "open",
		},
	}}

	expectedTitle := "Ship the broker contract"
	expectedBody := ""
	expectedLabels := []string{"dunderia", "feature", "channel-general"}
	remoteTitle := "Old title"
	remoteBody := "Old body"
	remoteLabels := []string{"legacy"}
	remoteState := "open"
	issueEdits := 0
	issueCloses := 0
	issueReopens := 0

	oldRun := taskGitHubRunCommand
	taskGitHubRunCommand = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		switch {
		case name == "gh" && len(args) == 2 && args[0] == "auth" && args[1] == "status":
			return []byte("ok"), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test":
			return []byte(`{"visibility":"private","private":true,"default_branch":"main"}`), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test/issues/42":
			labelsJSON := `[]`
			if len(remoteLabels) > 0 {
				parts := make([]string, 0, len(remoteLabels))
				for _, label := range remoteLabels {
					parts = append(parts, fmt.Sprintf(`{"name":%q}`, label))
				}
				labelsJSON = "[" + strings.Join(parts, ",") + "]"
			}
			return []byte(fmt.Sprintf(`{"number":42,"html_url":"https://github.com/acme/dunderia-test/issues/42","node_id":"I_kwDOISSUE42","state":%q,"title":%q,"body":%q,"labels":%s}`, remoteState, remoteTitle, remoteBody, labelsJSON)), nil
		case name == "gh" && len(args) >= 3 && args[0] == "issue" && args[1] == "edit" && args[2] == "42":
			issueEdits++
			remoteTitle = expectedTitle
			remoteBody = expectedBody
			remoteLabels = append([]string(nil), expectedLabels...)
			return []byte("ok"), nil
		case name == "gh" && len(args) >= 3 && args[0] == "issue" && args[1] == "close" && args[2] == "42":
			issueCloses++
			remoteState = "closed"
			return []byte("ok"), nil
		case name == "gh" && len(args) >= 3 && args[0] == "issue" && args[1] == "reopen" && args[2] == "42":
			issueReopens++
			remoteState = "open"
			return []byte("ok"), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
		}
	}
	defer func() { taskGitHubRunCommand = oldRun }()

	repoCtx, err := resolveTaskGitHubRepoContext(&b.tasks[0])
	if err != nil {
		t.Fatalf("resolve repo context: %v", err)
	}
	expectedBody = buildTaskGitHubIssueBody(&b.tasks[0], repoCtx)

	_, err = b.ReconcileTaskGitHubPublications(time.Now().UTC())
	if err != nil {
		t.Fatalf("reconcile active issue drift: %v", err)
	}
	updated := b.AllTasks()[0]
	if issueEdits != 1 {
		t.Fatalf("expected one issue edit for drift correction, got edits=%d", issueEdits)
	}
	if updated.IssuePublication == nil || updated.IssuePublication.Title != expectedTitle || updated.IssuePublication.State != "open" {
		t.Fatalf("expected synced issue after edit, got %+v", updated.IssuePublication)
	}

	b.tasks[0].Status = "done"
	b.tasks[0].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_, err = b.ReconcileTaskGitHubPublications(time.Now().UTC())
	if err != nil {
		t.Fatalf("reconcile closed issue lifecycle: %v", err)
	}
	updated = b.AllTasks()[0]
	if issueCloses != 1 || updated.IssuePublication == nil || updated.IssuePublication.State != "closed" {
		t.Fatalf("expected issue close on terminal task, closes=%d publication=%+v", issueCloses, updated.IssuePublication)
	}

	b.tasks[0].Status = "in_progress"
	b.tasks[0].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_, err = b.ReconcileTaskGitHubPublications(time.Now().UTC())
	if err != nil {
		t.Fatalf("reconcile reopen issue lifecycle: %v", err)
	}
	updated = b.AllTasks()[0]
	if issueReopens != 1 || updated.IssuePublication == nil || updated.IssuePublication.State != "open" {
		t.Fatalf("expected issue reopen on active task, reopens=%d publication=%+v", issueReopens, updated.IssuePublication)
	}
}

func TestReconcileTaskGitHubPublicationsPromotesDraftPRWhenTaskIsReadyForReview(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	repo, branch := initGitHubPublicationRepo(t)
	b := NewBroker()
	now := time.Now().UTC().Format(time.RFC3339)
	b.tasks = []teamTask{{
		ID:             "task-1",
		Channel:        "general",
		Title:          "Promote draft PR",
		Details:        "Move the PR to ready for review when the task reaches review.",
		Owner:          "builder",
		Status:         "review",
		ReviewState:    "ready_for_review",
		CreatedBy:      "ceo",
		TaskType:       "feature",
		ExecutionMode:  "external_workspace",
		WorkspacePath:  repo,
		WorktreeBranch: branch,
		CreatedAt:      now,
		UpdatedAt:      now,
		PRPublication: &taskGitHubPublication{
			Status:     "opened",
			Number:     7,
			URL:        "https://github.com/acme/dunderia-test/pull/7",
			HeadBranch: branch,
			BaseBranch: "main",
			Title:      "Promote draft PR",
			State:      "open",
			Draft:      true,
		},
	}}

	remoteDraft := true
	readyCalls := 0
	oldRun := taskGitHubRunCommand
	taskGitHubRunCommand = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		switch {
		case name == "gh" && len(args) == 2 && args[0] == "auth" && args[1] == "status":
			return []byte("ok"), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test":
			return []byte(`{"visibility":"private","private":true,"default_branch":"main"}`), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test/pulls/7":
			return []byte(fmt.Sprintf(`{"number":7,"html_url":"https://github.com/acme/dunderia-test/pull/7","node_id":"PR_kwDODRAFT7","state":"open","draft":%t,"title":"Promote draft PR","body":"Old body"}`, remoteDraft)), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test/issues/7":
			return []byte(`{"number":7,"html_url":"https://github.com/acme/dunderia-test/pull/7","node_id":"PR_kwDODRAFT7","state":"open","title":"Promote draft PR","body":"Old body","labels":[{"name":"legacy"}]}`), nil
		case name == "gh" && len(args) >= 3 && args[0] == "pr" && args[1] == "ready" && args[2] == "7":
			readyCalls++
			remoteDraft = false
			return []byte("ok"), nil
		case name == "gh" && len(args) >= 3 && args[0] == "pr" && args[1] == "edit" && args[2] == "7":
			return []byte("ok"), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
		}
	}
	defer func() { taskGitHubRunCommand = oldRun }()

	_, err := b.ReconcileTaskGitHubPublications(time.Now().UTC())
	if err != nil {
		t.Fatalf("reconcile draft pr promotion: %v", err)
	}
	updated := b.AllTasks()[0]
	if readyCalls != 1 {
		t.Fatalf("expected one ready-for-review promotion, got %d", readyCalls)
	}
	if updated.PRPublication == nil || updated.PRPublication.Draft {
		t.Fatalf("expected promoted PR to stop being draft, got %+v", updated.PRPublication)
	}
}

func TestLauncherProcessDueGitHubPublicationJobReconcilesAndReschedules(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	oldRun := taskGitHubRunCommand
	taskGitHubRunCommand = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		switch {
		case name == "gh" && len(args) == 2 && args[0] == "auth" && args[1] == "status":
			return []byte("ok"), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test":
			return []byte(`{"visibility":"private","private":true,"default_branch":"main"}`), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "search/issues":
			return []byte(`{"items":[]}`), nil
		case name == "gh" && len(args) >= 4 && args[0] == "api" && args[1] == "repos/acme/dunderia-test/issues":
			return []byte(`{"number":63,"html_url":"https://github.com/acme/dunderia-test/issues/63","node_id":"I_kwDOAUDIT63","state":"open"}`), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
		}
	}
	defer func() { taskGitHubRunCommand = oldRun }()

	repo, _ := initGitHubPublicationRepo(t)
	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	_, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Audit scheduler reconcile",
		Details:       "Open the issue from the periodic reconciler.",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "external_workspace",
		WorkspacePath: repo,
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	due := time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339)
	if err := b.SetSchedulerJob(schedulerJob{
		Slug:            taskGitHubAuditJobSlug,
		Kind:            taskGitHubAuditJobKind,
		Label:           "GitHub publication audit",
		TargetType:      taskGitHubAuditTargetType,
		TargetID:        "global",
		Channel:         "general",
		IntervalMinutes: int(taskGitHubAuditInterval / time.Minute),
		NextRun:         due,
		DueAt:           due,
		Status:          "scheduled",
	}); err != nil {
		t.Fatalf("set due audit job: %v", err)
	}

	l := &Launcher{broker: b}
	l.processDueSchedulerJobs()

	updated := b.AllTasks()[0]
	if updated.IssuePublication == nil || updated.IssuePublication.Status != "opened" || updated.IssuePublication.Number != 63 {
		t.Fatalf("expected launcher scheduler loop to open issue, got %+v", updated.IssuePublication)
	}

	var job schedulerJob
	for _, candidate := range b.Scheduler() {
		if candidate.Slug == taskGitHubAuditJobSlug {
			job = candidate
			break
		}
	}
	if job.Slug == "" || strings.TrimSpace(job.NextRun) == "" || job.Status != "scheduled" {
		t.Fatalf("expected rescheduled github audit job, got %+v", job)
	}
}

func TestPublishTaskIssueNowIsSerializedAndIdempotent(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	oldAsync := scheduleTaskGitHubAsync
	scheduleTaskGitHubAsync = func(fn func()) {}
	defer func() { scheduleTaskGitHubAsync = oldAsync }()

	repo, _ := initGitHubPublicationRepo(t)
	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")

	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Serialized issue publication",
		Details:       "Only one GitHub issue should be created.",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "external_workspace",
		WorkspacePath: repo,
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	createStarted := make(chan struct{}, 1)
	releaseCreate := make(chan struct{})
	var createCalls atomic.Int32
	oldRun := taskGitHubRunCommand
	taskGitHubRunCommand = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		switch {
		case name == "gh" && len(args) == 2 && args[0] == "auth" && args[1] == "status":
			return []byte("ok"), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "repos/acme/dunderia-test":
			return []byte(`{"visibility":"private","private":true,"default_branch":"main"}`), nil
		case name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "search/issues":
			return []byte(`{"items":[]}`), nil
		case name == "gh" && len(args) >= 4 && args[0] == "api" && args[1] == "repos/acme/dunderia-test/issues":
			if createCalls.Add(1) == 1 {
				createStarted <- struct{}{}
				<-releaseCreate
			}
			return []byte(`{"number":91,"html_url":"https://github.com/acme/dunderia-test/issues/91","node_id":"I_kwDOSERIAL91","state":"open"}`), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
		}
	}
	defer func() { taskGitHubRunCommand = oldRun }()

	type publishResult struct {
		task    teamTask
		changed bool
		err     error
	}
	results := make(chan publishResult, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		updated, changed, err := b.publishTaskIssueNow(task.ID)
		results <- publishResult{task: updated, changed: changed, err: err}
	}()

	<-createStarted

	go func() {
		defer wg.Done()
		updated, changed, err := b.publishTaskIssueNow(task.ID)
		results <- publishResult{task: updated, changed: changed, err: err}
	}()

	close(releaseCreate)
	wg.Wait()
	close(results)

	successes := 0
	failures := 0
	for result := range results {
		if result.err == nil && result.changed {
			successes++
		} else {
			failures++
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("expected one successful and one rejected concurrent publish, got successes=%d failures=%d", successes, failures)
	}
	if createCalls.Load() != 1 {
		t.Fatalf("expected exactly one issue creation call, got %d", createCalls.Load())
	}
	updated := b.AllTasks()[0]
	if updated.IssuePublication == nil || updated.IssuePublication.Status != "opened" || updated.IssuePublication.Number != 91 {
		t.Fatalf("expected stable opened issue publication, got %+v", updated.IssuePublication)
	}
	if updated.IssuePublication.RetryCount != 1 {
		t.Fatalf("expected retry count to remain stable after concurrent publish, got %+v", updated.IssuePublication)
	}
}

func slicesContains(values []string, needle string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == strings.TrimSpace(needle) {
			return true
		}
	}
	return false
}
