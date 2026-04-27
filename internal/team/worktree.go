package team

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nex-crm/wuphf/internal/config"
)

var prepareTaskWorktree = defaultPrepareTaskWorktree
var cleanupTaskWorktree = defaultCleanupTaskWorktree
var taskWorktreeRootDir = defaultTaskWorktreeRootDir
var verifyTaskWorktreeWritable = defaultVerifyTaskWorktreeWritable
var currentTaskRepoRoot = defaultCurrentTaskRepoRoot

var (
	defaultCurrentTaskRepoRootOnce sync.Once
	defaultCurrentTaskRepoRootPath string
	defaultCurrentTaskRepoRootErr  error
)

var overlaySourceWorkspaceSkipExact = map[string]struct{}{
	".playwright-cli": {},
	".playwright-mcp": {},
}

var overlaySourceWorkspaceSkipPrefixes = []string{
	".playwright-cli/",
	".playwright-mcp/",
	".wuphf/",
}

const (
	taskWorktreeLeaseVersion  = 1
	taskWorktreeLeaseTTL      = 24 * time.Hour
	taskWorktreeLeaseFileName = "task-worktree-lease.json"
)

type taskWorktreeLease struct {
	Version      int    `json:"version,omitempty"`
	TaskID       string `json:"task_id,omitempty"`
	WorktreePath string `json:"worktree_path,omitempty"`
	Branch       string `json:"branch,omitempty"`
	LeasedAt     string `json:"leased_at,omitempty"`
	RenewedAt    string `json:"renewed_at,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
}

func defaultPrepareTaskWorktree(taskID string) (string, string, error) {
	repoRoot, err := currentTaskRepoRoot()
	if err != nil {
		return "", "", err
	}

	branch := worktreeBranchNameForRepo(taskID, repoRoot)
	root := taskWorktreeRootDir(repoRoot)
	if strings.TrimSpace(root) == "" {
		root = filepath.Join(os.TempDir(), "wuphf-task-worktrees")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", "", fmt.Errorf("prepare task worktree root: %w", err)
	}
	path := filepath.Join(root, "wuphf-task-"+sanitizeWorktreeToken(taskID))
	_ = runGit(repoRoot, "worktree", "prune")
	_ = cleanupTaskWorktreeAtRepoRoot(repoRoot, path, branch)
	_ = clearStaleTaskBranch(repoRoot, branch)
	finish := func(path, branch string) (string, string, error) {
		if err := overlaySourceWorkspace(repoRoot, path); err != nil {
			_ = cleanupTaskWorktreeAtRepoRoot(repoRoot, path, branch)
			return "", "", fmt.Errorf("overlay source workspace: %w", err)
		}
		if err := overlayPersistedTaskWorktrees(path, taskID); err != nil {
			_ = cleanupTaskWorktreeAtRepoRoot(repoRoot, path, branch)
			return "", "", fmt.Errorf("overlay prior task worktrees: %w", err)
		}
		if err := writeTaskWorktreeLease(path, taskID, branch, time.Now().UTC()); err != nil {
			_ = cleanupTaskWorktreeAtRepoRoot(repoRoot, path, branch)
			return "", "", fmt.Errorf("write task worktree lease: %w", err)
		}
		return path, branch, nil
	}
	firstErr := runGit(repoRoot, "worktree", "add", "-b", branch, path, "HEAD")
	if firstErr == nil {
		return finish(path, branch)
	}
	_ = runGit(repoRoot, "worktree", "prune")
	_ = cleanupTaskWorktreeAtRepoRoot(repoRoot, path, branch)
	_ = clearStaleTaskBranch(repoRoot, branch)
	if err := runGit(repoRoot, "worktree", "add", "-b", branch, path, "HEAD"); err == nil {
		return finish(path, branch)
	}
	if err := runGit(repoRoot, "worktree", "add", path, branch); err == nil {
		return finish(path, branch)
	}

	return "", "", fmt.Errorf("create git worktree for %s: %w", taskID, firstErr)
}

func clearStaleTaskBranch(repoRoot, branch string) error {
	branch = strings.TrimSpace(branch)
	if branch == "" || !gitRefExists(repoRoot, "refs/heads/"+branch) {
		return nil
	}
	if err := runGit(repoRoot, "branch", "-D", branch); err == nil {
		return nil
	}
	_ = runGit(repoRoot, "worktree", "prune")
	if !gitRefExists(repoRoot, "refs/heads/"+branch) {
		return nil
	}
	return runGit(repoRoot, "branch", "-D", branch)
}

func defaultCleanupTaskWorktree(path, branch string) error {
	repoRoot, err := gitRepoRoot()
	if err != nil {
		return err
	}
	return cleanupTaskWorktreeAtRepoRoot(repoRoot, path, branch)
}

func defaultVerifyTaskWorktreeWritable(path string) error {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return fmt.Errorf("task worktree path required")
	}
	if !worktreePathLooksSafe(path) {
		return fmt.Errorf("unsafe task worktree path %q", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat task worktree: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("task worktree is not a directory: %q", path)
	}
	probe, err := os.CreateTemp(path, ".wuphf-write-probe-*")
	if err != nil {
		return fmt.Errorf("write probe failed: %w", err)
	}
	probePath := probe.Name()
	if closeErr := probe.Close(); closeErr != nil {
		_ = os.Remove(probePath)
		return fmt.Errorf("close write probe: %w", closeErr)
	}
	if err := os.Remove(probePath); err != nil {
		return fmt.Errorf("cleanup write probe: %w", err)
	}
	return nil
}

func cleanupTaskWorktreeAtRepoRoot(repoRoot, path, branch string) error {
	var failures []string
	if strings.TrimSpace(path) != "" {
		if err := runGit(repoRoot, "worktree", "remove", "--force", path); err != nil {
			if _, statErr := os.Stat(path); statErr == nil {
				if worktreePathLooksSafe(path) {
					if rmErr := os.RemoveAll(path); rmErr != nil {
						failures = append(failures, rmErr.Error())
					}
				} else {
					failures = append(failures, err.Error())
				}
			}
		}
	}
	if strings.TrimSpace(branch) != "" {
		if !worktreeBranchLooksManaged(branch) {
			return nil
		}
		if gitRefExists(repoRoot, "refs/heads/"+branch) {
			if err := runGit(repoRoot, "branch", "-D", branch); err != nil {
				failures = append(failures, err.Error())
			}
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return nil
}

func worktreeBranchLooksManaged(branch string) bool {
	branch = strings.TrimSpace(branch)
	if branch == "" || strings.ContainsAny(branch, "\\~^:?*[") || strings.Contains(branch, "..") {
		return false
	}
	if strings.HasPrefix(branch, "-") || strings.HasSuffix(branch, ".") || strings.HasSuffix(branch, "/") {
		return false
	}
	parts := strings.Split(branch, "/")
	for _, part := range parts {
		if part == "" || strings.HasSuffix(part, ".lock") {
			return false
		}
	}
	if !strings.HasPrefix(branch, "wuphf-") {
		return false
	}
	token := strings.TrimPrefix(branch, "wuphf-")
	if token == "" {
		return false
	}
	for _, r := range token {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return true
}

func defaultCurrentTaskRepoRoot() (string, error) {
	defaultCurrentTaskRepoRootOnce.Do(func() {
		if repoRoot, err := gitRepoRoot(); err == nil && strings.TrimSpace(repoRoot) != "" {
			defaultCurrentTaskRepoRootPath = repoRoot
			return
		}
		exePath, err := os.Executable()
		if err != nil {
			defaultCurrentTaskRepoRootErr = fmt.Errorf("resolve repo root from process cwd and executable: %w", err)
			return
		}
		exeDir := strings.TrimSpace(filepath.Dir(exePath))
		if exeDir == "" {
			defaultCurrentTaskRepoRootErr = fmt.Errorf("resolve repo root from executable: executable dir empty")
			return
		}
		defaultCurrentTaskRepoRootPath, defaultCurrentTaskRepoRootErr = gitRepoRootFromDir(exeDir)
	})
	return defaultCurrentTaskRepoRootPath, defaultCurrentTaskRepoRootErr
}

func gitRepoRoot() (string, error) {
	return gitRepoRootFromDir("")
}

func gitRepoRootFromDir(dir string) (string, error) {
	cmdArgs := []string{"rev-parse", "--show-toplevel"}
	if strings.TrimSpace(dir) != "" {
		cmdArgs = append([]string{"-C", dir}, cmdArgs...)
	}
	cmd := gitCommand(dir, cmdArgs...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.TrimSpace(dir) != "" {
			return "", fmt.Errorf("resolve repo root from %s: %w: %s", dir, err, strings.TrimSpace(stderr.String()))
		}
		return "", fmt.Errorf("resolve repo root: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func defaultTaskWorktreeRootDir(repoRoot string) string {
	repoToken := sanitizeWorktreeToken(filepath.Base(strings.TrimSpace(repoRoot)))
	if repoToken == "" {
		repoToken = "workspace"
	}

	if home := config.RuntimeHomeDir(); strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".wuphf", "task-worktrees", repoToken)
	}
	return filepath.Join(os.TempDir(), "wuphf-task-worktrees", repoToken)
}

func taskWorktreeLeasePath(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return ""
	}
	return filepath.Join(path, ".wuphf", taskWorktreeLeaseFileName)
}

func readTaskWorktreeLease(path string) (taskWorktreeLease, error) {
	leasePath := taskWorktreeLeasePath(path)
	if leasePath == "" {
		return taskWorktreeLease{}, os.ErrNotExist
	}
	raw, err := os.ReadFile(leasePath)
	if err != nil {
		return taskWorktreeLease{}, err
	}
	var lease taskWorktreeLease
	if err := json.Unmarshal(raw, &lease); err != nil {
		return taskWorktreeLease{}, err
	}
	return lease, nil
}

func writeTaskWorktreeLease(path, taskID, branch string, now time.Time) error {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return fmt.Errorf("task worktree path required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	leasePath := taskWorktreeLeasePath(path)
	if leasePath == "" {
		return fmt.Errorf("task worktree lease path required")
	}
	if err := os.MkdirAll(filepath.Dir(leasePath), 0o755); err != nil {
		return fmt.Errorf("mkdir task worktree lease dir: %w", err)
	}
	lease := taskWorktreeLease{
		Version:      taskWorktreeLeaseVersion,
		TaskID:       strings.TrimSpace(taskID),
		WorktreePath: path,
		Branch:       strings.TrimSpace(branch),
		LeasedAt:     now.Format(time.RFC3339),
		RenewedAt:    now.Format(time.RFC3339),
		ExpiresAt:    now.Add(taskWorktreeLeaseTTL).Format(time.RFC3339),
	}
	if existing, err := readTaskWorktreeLease(path); err == nil {
		if strings.TrimSpace(existing.LeasedAt) != "" {
			lease.LeasedAt = strings.TrimSpace(existing.LeasedAt)
		}
		if strings.TrimSpace(existing.TaskID) != "" && lease.TaskID == "" {
			lease.TaskID = strings.TrimSpace(existing.TaskID)
		}
		if strings.TrimSpace(existing.Branch) != "" && lease.Branch == "" {
			lease.Branch = strings.TrimSpace(existing.Branch)
		}
	}
	raw, err := json.MarshalIndent(lease, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal task worktree lease: %w", err)
	}
	if err := os.WriteFile(leasePath, raw, 0o600); err != nil {
		return fmt.Errorf("write task worktree lease: %w", err)
	}
	return nil
}

func taskWorktreeLeaseExpiry(path string) (time.Time, bool) {
	if lease, err := readTaskWorktreeLease(path); err == nil {
		if expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(lease.ExpiresAt)); err == nil {
			return expiresAt.UTC(), true
		}
	}
	for _, candidate := range []string{taskWorktreeLeasePath(path), filepath.Clean(strings.TrimSpace(path))} {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil {
			return info.ModTime().UTC().Add(taskWorktreeLeaseTTL), true
		}
	}
	return time.Time{}, false
}

func taskWorktreeLeaseExpired(path string, now time.Time) bool {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	expiresAt, ok := taskWorktreeLeaseExpiry(path)
	if !ok {
		return false
	}
	return !expiresAt.After(now)
}

func runGit(dir string, args ...string) error {
	cmd := gitCommand(dir, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func runGitOutput(dir string, args ...string) ([]byte, error) {
	cmd := gitCommand(dir, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func overlaySourceWorkspace(repoRoot, worktreePath string) error {
	return overlayWorkspaceChanges(repoRoot, worktreePath)
}

func overlayWorkspaceChanges(sourceRoot, worktreePath string) error {
	changed, err := runGitOutput(sourceRoot, "diff", "--name-only", "-z", "HEAD", "--")
	if err != nil {
		return err
	}
	untracked, err := runGitOutput(sourceRoot, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return err
	}
	// `git ls-files --others` is occasionally incomplete in worktree-heavy Windows/CI
	// environments. Fall back to porcelain status for `??` entries so overlaying
	// sibling task workspaces does not depend on that single plumbing command.
	status, err := runGitOutput(sourceRoot, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return err
	}
	seen := map[string]struct{}{}
	rawPaths := append(bytes.Split(changed, []byte{0}), bytes.Split(untracked, []byte{0})...)
	rawPaths = append(rawPaths, parsePorcelainUntrackedPaths(status)...)
	for _, raw := range rawPaths {
		rel := strings.TrimSpace(string(raw))
		if rel == "" {
			continue
		}
		if !shouldOverlaySourceWorkspacePath(rel) {
			continue
		}
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		src := filepath.Join(sourceRoot, filepath.FromSlash(rel))
		dst := filepath.Join(worktreePath, filepath.FromSlash(rel))
		info, statErr := os.Lstat(src)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				if err := os.RemoveAll(dst); err != nil && !os.IsNotExist(err) {
					return err
				}
				continue
			}
			return statErr
		}
		if err := copyWorkspacePath(src, dst, info); err != nil {
			return err
		}
	}
	return nil
}

func parsePorcelainUntrackedPaths(status []byte) [][]byte {
	entries := bytes.Split(status, []byte{0})
	out := make([][]byte, 0, len(entries))
	for _, entry := range entries {
		if len(entry) < 4 {
			continue
		}
		if entry[0] != '?' || entry[1] != '?' || entry[2] != ' ' {
			continue
		}
		path := bytes.TrimSpace(entry[3:])
		if len(path) == 0 {
			continue
		}
		out = append(out, append([]byte(nil), path...))
	}
	return out
}

func overlayPersistedTaskWorktrees(worktreePath string, currentTaskID string) error {
	raw, err := os.ReadFile(brokerStatePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var state struct {
		Tasks []teamTask `json:"tasks"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return err
	}

	seenSources := make(map[string]struct{})
	for _, task := range state.Tasks {
		if strings.TrimSpace(task.ID) == strings.TrimSpace(currentTaskID) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(task.ExecutionMode), "local_worktree") {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(task.Status))
		if status != "done" && status != "review" {
			continue
		}
		sourcePath := strings.TrimSpace(task.WorktreePath)
		if sourcePath == "" {
			continue
		}
		if sameCleanPath(sourcePath, worktreePath) {
			continue
		}
		sourceKey := filepath.Clean(sourcePath)
		if _, seen := seenSources[sourceKey]; seen {
			continue
		}
		seenSources[sourceKey] = struct{}{}
		if !taskWorktreeSourceLooksUsable(sourcePath) {
			continue
		}
		if taskWorktreeLeaseExpired(sourcePath, time.Now().UTC()) {
			continue
		}
		if !sameGitWorkspaceRoot(sourcePath, worktreePath) {
			continue
		}
		if err := overlayWorkspaceChanges(sourcePath, worktreePath); err != nil {
			return fmt.Errorf("%s: %w", task.ID, err)
		}
	}
	return nil
}

func taskWorktreeSourceLooksUsable(path string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	out, err := runGitOutput(path, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(string(out)), "true")
}

func gitWorkspaceIdentity(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return ""
	}
	out, err := runGitOutput(path, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return ""
	}
	return filepath.Clean(strings.TrimSpace(string(out)))
}

func shouldOverlaySourceWorkspacePath(rel string) bool {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" {
		return false
	}
	if _, skip := overlaySourceWorkspaceSkipExact[rel]; skip {
		return false
	}
	for _, prefix := range overlaySourceWorkspaceSkipPrefixes {
		if strings.HasPrefix(rel, prefix) {
			return false
		}
	}
	return true
}

func copyWorkspacePath(src, dst string, info os.FileInfo) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		_ = os.RemoveAll(dst)
		return os.Symlink(target, dst)
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, info.Mode().Perm()); err != nil {
		return err
	}
	return nil
}

func gitRefExists(dir, ref string) bool {
	cmd := gitCommand(dir, "show-ref", "--verify", "--quiet", ref)
	return cmd.Run() == nil
}

func gitCommand(dir string, args ...string) *exec.Cmd {
	workingDir := strings.TrimSpace(dir)
	if workingDir == "" {
		if cwd, err := os.Getwd(); err == nil {
			workingDir = cwd
		}
	}

	cmdArgs := make([]string, 0, len(args)+2)
	if safeDir := gitSafeDirectory(workingDir); safeDir != "" {
		cmdArgs = append(cmdArgs, "-c", "safe.directory="+safeDir)
	}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.Command("git", cmdArgs...)
	if workingDir != "" {
		cmd.Dir = workingDir
	}
	return cmd
}

func gitSafeDirectory(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return ""
	}

	for {
		gitPath := filepath.Join(path, ".git")
		if info, err := os.Stat(gitPath); err == nil {
			if info.IsDir() || !info.IsDir() {
				return path
			}
		}
		parent := filepath.Dir(path)
		if parent == path {
			return ""
		}
		path = parent
	}
}

func worktreeBranchName(taskID string) string {
	repoRoot, err := gitRepoRoot()
	if err != nil {
		return "wuphf-" + sanitizeWorktreeToken(taskID)
	}
	return worktreeBranchNameForRepo(taskID, repoRoot)
}

func worktreeBranchNameForRepo(taskID string, repoRoot string) string {
	taskToken := sanitizeWorktreeToken(taskID)
	if taskToken == "" {
		taskToken = "task"
	}
	if namespace := worktreeNamespaceToken(repoRoot); namespace != "" {
		return "wuphf-" + namespace + "-" + taskToken
	}
	return "wuphf-" + taskToken
}

func worktreeNamespaceToken(repoRoot string) string {
	root := strings.TrimSpace(taskWorktreeRootDir(repoRoot))
	if root == "" {
		root = strings.TrimSpace(repoRoot)
	}
	if root == "" {
		return ""
	}
	sum := fnv.New32a()
	_, _ = sum.Write([]byte(filepath.Clean(root)))
	return fmt.Sprintf("%08x", sum.Sum32())
}

func CleanupPersistedTaskWorktrees() error {
	path := brokerStatePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var state struct {
		Tasks []teamTask `json:"tasks"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	seen := make(map[string]struct{})
	var firstErr error
	for _, task := range state.Tasks {
		worktreePath := strings.TrimSpace(task.WorktreePath)
		worktreeBranch := strings.TrimSpace(task.WorktreeBranch)
		if worktreePath == "" && worktreeBranch == "" {
			continue
		}
		key := worktreePath + "\x00" + worktreeBranch
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if err := cleanupTaskWorktree(worktreePath, worktreeBranch); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func worktreePathLooksSafe(path string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return false
	}
	if !strings.Contains(filepath.Base(path), "wuphf-task-") {
		return false
	}
	for _, root := range managedWorktreeRoots() {
		if pathWithinRoot(path, root) {
			return true
		}
	}
	tempRoot := filepath.Clean(os.TempDir())
	return pathWithinRoot(path, tempRoot)
}

func managedWorktreeRoots() []string {
	roots := make([]string, 0, 2)
	if home := config.RuntimeHomeDir(); strings.TrimSpace(home) != "" {
		roots = append(roots, filepath.Join(home, ".wuphf", "task-worktrees"))
	}
	roots = append(roots, filepath.Join(os.TempDir(), "wuphf-task-worktrees"))
	return roots
}

func pathWithinRoot(path string, root string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	root = filepath.Clean(strings.TrimSpace(root))
	if path == "" || root == "" || root == "." {
		return false
	}
	return path == root || strings.HasPrefix(path, root+string(os.PathSeparator))
}

func sameCleanPath(a string, b string) bool {
	a = filepath.Clean(strings.TrimSpace(a))
	b = filepath.Clean(strings.TrimSpace(b))
	if a == "" || b == "" {
		return false
	}
	return a == b
}

func sanitizeWorktreeToken(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "task"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(strings.ReplaceAll(b.String(), "--", "-"), "-")
}
