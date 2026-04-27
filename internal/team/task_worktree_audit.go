package team

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	taskWorktreeAuditInterval   = 7 * time.Minute
	taskWorktreeAuditJobSlug    = "task-worktree-audit"
	taskWorktreeAuditJobKind    = "task_worktree_audit"
	taskWorktreeAuditTargetType = "task_worktree"
)

var taskWorktreeListGitWorktrees = defaultTaskWorktreeListGitWorktrees
var taskWorktreeCleanupAtRepoRoot = cleanupTaskWorktreeAtRepoRoot

type taskWorktreeAuditReport struct {
	LastRunAt        string `json:"last_run_at,omitempty"`
	ReconciledTasks  int    `json:"reconciled_tasks,omitempty"`
	RecoveredTasks   int    `json:"recovered_tasks,omitempty"`
	CleanedTasks     int    `json:"cleaned_tasks,omitempty"`
	CleanedOrphans   int    `json:"cleaned_orphans,omitempty"`
	FailedOperations int    `json:"failed_operations,omitempty"`
}

func (b *Broker) EnsureTaskWorktreeAuditJob() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	nextRun := time.Now().UTC().Add(taskWorktreeAuditInterval).Format(time.RFC3339)
	desired := normalizeSchedulerJob(schedulerJob{
		Slug:            taskWorktreeAuditJobSlug,
		Kind:            taskWorktreeAuditJobKind,
		Label:           "Task worktree audit",
		TargetType:      taskWorktreeAuditTargetType,
		TargetID:        "global",
		Channel:         "general",
		IntervalMinutes: int(taskWorktreeAuditInterval / time.Minute),
		NextRun:         nextRun,
		DueAt:           nextRun,
		Status:          "scheduled",
	})
	changed := false
	for i := range b.scheduler {
		if strings.TrimSpace(b.scheduler[i].Slug) != taskWorktreeAuditJobSlug {
			continue
		}
		current := normalizeSchedulerJob(b.scheduler[i])
		if current.Kind != desired.Kind || current.Label != desired.Label || current.TargetType != desired.TargetType ||
			current.TargetID != desired.TargetID || current.Channel != desired.Channel || current.IntervalMinutes != desired.IntervalMinutes {
			current.Kind = desired.Kind
			current.Label = desired.Label
			current.TargetType = desired.TargetType
			current.TargetID = desired.TargetID
			current.Channel = desired.Channel
			current.IntervalMinutes = desired.IntervalMinutes
			changed = true
		}
		if current.Status == "" || strings.EqualFold(current.Status, "done") || strings.EqualFold(current.Status, "canceled") {
			current.Status = "scheduled"
			current.NextRun = desired.NextRun
			current.DueAt = desired.DueAt
			changed = true
		}
		if changed {
			b.scheduler[i] = current
			return b.saveLocked()
		}
		return nil
	}
	if err := b.scheduleJobLocked(desired); err != nil {
		return err
	}
	return b.saveLocked()
}

func (b *Broker) AuditTaskWorktrees(now time.Time) (taskWorktreeAuditReport, error) {
	report := taskWorktreeAuditReport{}
	if b == nil {
		return report, nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	report.LastRunAt = now.Format(time.RFC3339)

	referencedPaths := make(map[string]struct{})
	var firstErr error

	b.mu.Lock()
	changed := false
	for i := range b.tasks {
		beforePath := strings.TrimSpace(b.tasks[i].WorktreePath)
		beforeBranch := strings.TrimSpace(b.tasks[i].WorktreeBranch)
		beforeWorkspace := strings.TrimSpace(b.tasks[i].WorkspacePath)

		if err := b.syncTaskWorktreeLocked(&b.tasks[i]); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			report.FailedOperations++
			continue
		}

		if strings.EqualFold(strings.TrimSpace(b.tasks[i].ExecutionMode), "local_worktree") {
			afterStatus := strings.ToLower(strings.TrimSpace(b.tasks[i].Status))
			afterPath := strings.TrimSpace(b.tasks[i].WorktreePath)
			afterBranch := strings.TrimSpace(b.tasks[i].WorktreeBranch)
			switch {
			case afterPath == "":
			case afterStatus == "done":
				if taskWorktreeLeaseExpired(afterPath, now) {
					b.tasks[i].WorktreePath = ""
					b.tasks[i].WorktreeBranch = ""
				}
			default:
				if err := writeTaskWorktreeLease(afterPath, b.tasks[i].ID, afterBranch, now); err != nil {
					if firstErr == nil {
						firstErr = err
					}
					report.FailedOperations++
				}
			}
		}

		afterPath := strings.TrimSpace(b.tasks[i].WorktreePath)
		afterBranch := strings.TrimSpace(b.tasks[i].WorktreeBranch)
		afterWorkspace := strings.TrimSpace(b.tasks[i].WorkspacePath)
		if beforePath != afterPath || beforeBranch != afterBranch || beforeWorkspace != afterWorkspace {
			changed = true
			report.ReconciledTasks++
			switch {
			case beforePath == "" && afterPath != "":
				report.RecoveredTasks++
			case beforePath != "" && afterPath == "":
				report.CleanedTasks++
			}
		}
		if key := taskWorktreeAuditKey(afterPath); key != "" {
			referencedPaths[key] = struct{}{}
		}
	}
	if changed {
		if err := b.saveLocked(); err != nil {
			b.mu.Unlock()
			return report, err
		}
	}
	b.mu.Unlock()

	repoRoot, err := currentTaskRepoRoot()
	if err != nil {
		if firstErr == nil {
			firstErr = err
		}
		return report, firstErr
	}

	cleanedOrphans, orphanErr := cleanupOrphanedManagedTaskWorktrees(repoRoot, referencedPaths, now)
	report.CleanedOrphans = cleanedOrphans
	if orphanErr != nil {
		report.FailedOperations++
		if firstErr == nil {
			firstErr = orphanErr
		}
	}
	return report, firstErr
}

func defaultTaskWorktreeListGitWorktrees(repoRoot string) (map[string]string, error) {
	raw, err := runGitOutput(repoRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	out := make(map[string]string)
	var currentPath string
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "worktree "):
			currentPath = strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
			key := taskWorktreeAuditKey(currentPath)
			if key != "" {
				out[key] = ""
			}
		case strings.HasPrefix(line, "branch ") && currentPath != "":
			key := taskWorktreeAuditKey(currentPath)
			if key == "" {
				continue
			}
			branch := strings.TrimSpace(strings.TrimPrefix(line, "branch "))
			branch = strings.TrimSpace(strings.TrimPrefix(branch, "refs/heads/"))
			out[key] = branch
		}
	}
	return out, nil
}

func cleanupOrphanedManagedTaskWorktrees(repoRoot string, referencedPaths map[string]struct{}, now time.Time) (int, error) {
	knownBranches, err := taskWorktreeListGitWorktrees(repoRoot)
	if err != nil {
		return 0, err
	}
	paths, err := findManagedTaskWorktreePaths()
	if err != nil {
		return 0, err
	}
	sort.Strings(paths)

	cleaned := 0
	var firstErr error
	for _, path := range paths {
		key := taskWorktreeAuditKey(path)
		if key == "" {
			continue
		}
		if _, keep := referencedPaths[key]; keep {
			continue
		}
		if !taskWorktreeLeaseExpired(path, now) {
			continue
		}
		if err := taskWorktreeCleanupAtRepoRoot(repoRoot, path, knownBranches[key]); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		cleaned++
	}
	return cleaned, firstErr
}

func findManagedTaskWorktreePaths() ([]string, error) {
	seen := make(map[string]struct{})
	paths := make([]string, 0)
	for _, root := range managedWorktreeRoots() {
		root = filepath.Clean(strings.TrimSpace(root))
		if root == "" {
			continue
		}
		if _, err := os.Stat(root); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if !d.IsDir() {
				return nil
			}
			if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(d.Name())), "wuphf-task-") {
				return nil
			}
			cleanPath := filepath.Clean(path)
			if _, exists := seen[cleanPath]; exists {
				return filepath.SkipDir
			}
			seen[cleanPath] = struct{}{}
			paths = append(paths, cleanPath)
			return filepath.SkipDir
		}); err != nil {
			return nil, err
		}
	}
	return paths, nil
}

func taskWorktreeAuditKey(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return ""
	}
	if os.PathSeparator == '\\' {
		return strings.ToLower(path)
	}
	return path
}
