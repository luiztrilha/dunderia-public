package team

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnsureTaskWorktreeAuditJobRegistersRecurringJob(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	b := NewBroker()
	if err := b.EnsureTaskWorktreeAuditJob(); err != nil {
		t.Fatalf("EnsureTaskWorktreeAuditJob: %v", err)
	}

	found := false
	for _, job := range b.scheduler {
		if job.Slug != taskWorktreeAuditJobSlug {
			continue
		}
		found = true
		if job.Kind != taskWorktreeAuditJobKind || job.TargetType != taskWorktreeAuditTargetType || job.IntervalMinutes <= 0 {
			t.Fatalf("unexpected worktree audit job: %+v", job)
		}
	}
	if !found {
		t.Fatalf("expected task worktree audit job to be registered")
	}
}

func TestCleanupOrphanedManagedTaskWorktreesRemovesOnlyUnreferencedPaths(t *testing.T) {
	runtimeHome := t.TempDir()
	t.Setenv("WUPHF_RUNTIME_HOME", runtimeHome)

	managedRoot := filepath.Join(runtimeHome, ".wuphf", "task-worktrees", "repo")
	keepPath := filepath.Join(managedRoot, "wuphf-task-keep")
	freshPath := filepath.Join(managedRoot, "wuphf-task-fresh")
	orphanPath := filepath.Join(managedRoot, "wuphf-task-orphan")
	for _, path := range []string{keepPath, freshPath, orphanPath} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}
	now := time.Now().UTC()
	if err := writeTaskWorktreeLease(keepPath, "task-keep", "wuphf-keep", now); err != nil {
		t.Fatalf("write keep lease: %v", err)
	}
	if err := writeTaskWorktreeLease(freshPath, "task-fresh", "wuphf-fresh", now); err != nil {
		t.Fatalf("write fresh lease: %v", err)
	}
	if err := writeTaskWorktreeLease(orphanPath, "task-orphan", "wuphf-orphan", now.Add(-2*taskWorktreeLeaseTTL)); err != nil {
		t.Fatalf("write orphan lease: %v", err)
	}

	oldList := taskWorktreeListGitWorktrees
	oldCleanup := taskWorktreeCleanupAtRepoRoot
	defer func() {
		taskWorktreeListGitWorktrees = oldList
		taskWorktreeCleanupAtRepoRoot = oldCleanup
	}()

	taskWorktreeListGitWorktrees = func(repoRoot string) (map[string]string, error) {
		return map[string]string{
			taskWorktreeAuditKey(keepPath):   "wuphf-keep",
			taskWorktreeAuditKey(freshPath):  "wuphf-fresh",
			taskWorktreeAuditKey(orphanPath): "wuphf-orphan",
		}, nil
	}

	var cleaned []string
	taskWorktreeCleanupAtRepoRoot = func(repoRoot, path, branch string) error {
		cleaned = append(cleaned, filepath.Clean(path)+"|"+branch)
		return nil
	}

	count, err := cleanupOrphanedManagedTaskWorktrees(filepath.Join(t.TempDir(), "repo"), map[string]struct{}{
		taskWorktreeAuditKey(keepPath): {},
	}, now)
	if err != nil {
		t.Fatalf("cleanupOrphanedManagedTaskWorktrees: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one orphan cleanup, got %d (%v)", count, cleaned)
	}
	if len(cleaned) != 1 || cleaned[0] != filepath.Clean(orphanPath)+"|wuphf-orphan" {
		t.Fatalf("expected orphan worktree cleanup only, got %v", cleaned)
	}
}

func TestAuditTaskWorktreesClearsDoneLeaseExpiredWorktree(t *testing.T) {
	isolateBrokerPersistenceEnv(t)

	runtimeHome := t.TempDir()
	t.Setenv("WUPHF_RUNTIME_HOME", runtimeHome)

	repoDir := filepath.Join(t.TempDir(), "repo")
	worktreePath := filepath.Join(runtimeHome, ".wuphf", "task-worktrees", "repo", "wuphf-task-task-1")
	for _, dir := range []string{repoDir, worktreePath} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	run(worktreePath, "git", "init", "-b", "main")
	run(worktreePath, "git", "config", "user.name", "WUPHF Test")
	run(worktreePath, "git", "config", "user.email", "wuphf@example.com")
	if err := os.WriteFile(filepath.Join(worktreePath, "tracked.txt"), []byte("done\n"), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	run(worktreePath, "git", "add", "tracked.txt")
	run(worktreePath, "git", "commit", "-m", "base")

	now := time.Now().UTC()
	if err := writeTaskWorktreeLease(worktreePath, "task-1", "wuphf-task-1", now.Add(-2*taskWorktreeLeaseTTL)); err != nil {
		t.Fatalf("write stale lease: %v", err)
	}

	oldRepoRoot := currentTaskRepoRoot
	oldList := taskWorktreeListGitWorktrees
	oldCleanup := taskWorktreeCleanupAtRepoRoot
	defer func() {
		currentTaskRepoRoot = oldRepoRoot
		taskWorktreeListGitWorktrees = oldList
		taskWorktreeCleanupAtRepoRoot = oldCleanup
	}()
	currentTaskRepoRoot = func() (string, error) { return repoDir, nil }
	taskWorktreeListGitWorktrees = func(repoRoot string) (map[string]string, error) {
		return map[string]string{taskWorktreeAuditKey(worktreePath): "wuphf-task-1"}, nil
	}
	var cleaned []string
	taskWorktreeCleanupAtRepoRoot = func(repoRoot, path, branch string) error {
		cleaned = append(cleaned, filepath.Clean(path)+"|"+branch)
		return nil
	}

	b := NewBroker()
	b.tasks = []teamTask{{
		ID:             "task-1",
		Owner:          "builder",
		Status:         "done",
		ExecutionMode:  "local_worktree",
		WorktreePath:   worktreePath,
		WorktreeBranch: "wuphf-task-1",
		CreatedAt:      now.Add(-48 * time.Hour).Format(time.RFC3339),
		UpdatedAt:      now.Add(-2 * time.Hour).Format(time.RFC3339),
	}}

	report, err := b.AuditTaskWorktrees(now)
	if err != nil {
		t.Fatalf("AuditTaskWorktrees: %v", err)
	}
	if report.CleanedTasks != 1 {
		t.Fatalf("expected one cleaned task, got %+v", report)
	}
	if report.CleanedOrphans != 1 {
		t.Fatalf("expected one cleaned orphan, got %+v", report)
	}
	if got := b.tasks[0].WorktreePath; got != "" {
		t.Fatalf("expected expired done worktree path to be cleared, got %q", got)
	}
	if got := b.tasks[0].WorktreeBranch; got != "" {
		t.Fatalf("expected expired done worktree branch to be cleared, got %q", got)
	}
	if len(cleaned) != 1 || cleaned[0] != filepath.Clean(worktreePath)+"|wuphf-task-1" {
		t.Fatalf("expected orphan cleanup for expired worktree, got %v", cleaned)
	}
}
