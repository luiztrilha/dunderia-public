package team

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var explicitWorkspacePathPattern = regexp.MustCompile(`(?i)[a-z]:\\[^\s"'` + "`" + `,;)\]]+`)

func explicitWorkspacePathFromText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	matches := explicitWorkspacePathPattern.FindAllString(text, -1)
	if len(matches) == 0 {
		return ""
	}

	roots := make(map[string]struct{}, len(matches))
	for _, raw := range matches {
		candidate, ok := explicitWorkspaceTarget(strings.TrimSpace(raw))
		if !ok || !taskWorkspacePathLooksEligible(candidate) {
			continue
		}
		roots[candidate] = struct{}{}
	}
	if len(roots) != 1 {
		return ""
	}
	for root := range roots {
		return root
	}
	return ""
}

func explicitWorkspacePathForMessage(msg channelMessage) string {
	return explicitWorkspacePathFromText(strings.TrimSpace(strings.Join([]string{
		msg.Channel,
		msg.Title,
		msg.Content,
	}, "\n")))
}

func taskMatchesExplicitWorkspacePath(task *teamTask, workspacePath string) bool {
	workspacePath = filepath.Clean(strings.TrimSpace(workspacePath))
	if workspacePath == "" {
		return true
	}
	if task == nil {
		return false
	}

	candidates := []string{
		strings.TrimSpace(task.WorkspacePath),
		strings.TrimSpace(task.WorktreePath),
		inferExplicitWorkspacePathForTask(task),
		inferSiblingWorkspacePathForTask(task),
	}
	for _, candidate := range candidates {
		candidate = filepath.Clean(strings.TrimSpace(candidate))
		if candidate == "" {
			continue
		}
		if sameCleanPath(candidate, workspacePath) || sameGitWorkspaceRoot(candidate, workspacePath) {
			return true
		}
	}
	return false
}

func effectiveTeamTaskWorkspacePath(task *teamTask) string {
	if task == nil {
		return ""
	}
	if path := strings.TrimSpace(task.WorkspacePath); path != "" {
		return path
	}
	return strings.TrimSpace(task.WorktreePath)
}

func effectiveRuntimeTaskWorkspacePath(task RuntimeTask) string {
	if path := strings.TrimSpace(task.WorkspacePath); path != "" {
		return path
	}
	return strings.TrimSpace(task.WorktreePath)
}

func effectiveSessionMemoryTaskWorkspacePath(task SessionMemoryTaskSummary) string {
	if path := strings.TrimSpace(task.WorkspacePath); path != "" {
		return path
	}
	return strings.TrimSpace(task.WorktreePath)
}

func taskUsesExternalWorkspace(task *teamTask) bool {
	return task != nil && strings.EqualFold(strings.TrimSpace(task.ExecutionMode), "external_workspace")
}

func inferSiblingWorkspacePathForTask(task *teamTask) string {
	if task == nil {
		return ""
	}
	if strings.TrimSpace(task.WorkspacePath) != "" {
		return ""
	}
	if explicit := inferExplicitWorkspacePathForTask(task); explicit != "" {
		return explicit
	}
	if !strings.EqualFold(strings.TrimSpace(task.ExecutionMode), "local_worktree") {
		return ""
	}

	repoRoot, err := currentTaskRepoRoot()
	if err != nil {
		return ""
	}
	repoRoot = filepath.Clean(strings.TrimSpace(repoRoot))
	if repoRoot == "" {
		return ""
	}

	workspaceRoot := filepath.Dir(repoRoot)
	entries, err := os.ReadDir(workspaceRoot)
	if err != nil {
		return ""
	}

	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		task.Channel,
		task.ExecutionKey,
		task.Title,
		task.Details,
	}, "\n")))
	if text == "" {
		return ""
	}

	currentRepoName := strings.TrimSpace(filepath.Base(repoRoot))
	var match string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" || strings.EqualFold(name, currentRepoName) || strings.HasPrefix(name, ".") {
			continue
		}
		if !taskTextMentionsWorkspace(text, name) {
			continue
		}
		path := filepath.Join(workspaceRoot, name)
		if !taskWorkspacePathLooksEligible(path) {
			continue
		}
		if match != "" && !sameCleanPath(match, path) {
			return ""
		}
		match = path
	}
	return strings.TrimSpace(match)
}

func taskTextMentionsWorkspace(text, workspaceName string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	workspaceName = strings.ToLower(strings.TrimSpace(workspaceName))
	if text == "" || workspaceName == "" {
		return false
	}
	pattern := `(^|[^a-z0-9_])` + regexp.QuoteMeta(workspaceName) + `($|[^a-z0-9_])`
	matched, err := regexp.MatchString(pattern, text)
	return err == nil && matched
}

func inferExplicitWorkspacePathForTask(task *teamTask) string {
	if task == nil {
		return ""
	}

	repoRoot, err := currentTaskRepoRoot()
	if err != nil {
		return ""
	}
	repoRoot = filepath.Clean(strings.TrimSpace(repoRoot))
	if repoRoot == "" {
		return ""
	}

	text := strings.TrimSpace(strings.Join([]string{
		task.Channel,
		task.ExecutionKey,
		task.Title,
		task.Details,
	}, "\n"))
	if text == "" {
		return ""
	}

	matches := explicitWorkspacePathPattern.FindAllString(text, -1)
	if len(matches) == 0 {
		return ""
	}

	roots := make(map[string]struct{}, len(matches))
	for _, raw := range matches {
		candidate, ok := explicitWorkspaceTarget(strings.TrimSpace(raw))
		if !ok {
			continue
		}
		if sameCleanPath(candidate, repoRoot) || sameGitWorkspaceRoot(candidate, repoRoot) || !taskWorkspacePathLooksEligible(candidate) {
			continue
		}
		roots[candidate] = struct{}{}
	}
	if len(roots) != 1 {
		return ""
	}
	for root := range roots {
		return root
	}
	return ""
}

func explicitWorkspaceTarget(path string) (string, bool) {
	path = filepath.Clean(strings.TrimRight(strings.TrimSpace(path), ".,:"))
	if path == "" || pathContainsHiddenComponent(path) {
		return "", false
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", false
	}
	if info.IsDir() {
		if _, ok := nearestGitWorkspaceRoot(path); ok {
			return path, true
		}
		return "", false
	}
	dir := filepath.Dir(path)
	if dir == "" || dir == "." || pathContainsHiddenComponent(dir) {
		return "", false
	}
	if _, ok := nearestGitWorkspaceRoot(dir); !ok {
		return "", false
	}
	return dir, true
}

func taskWorkspacePathLooksEligible(path string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || pathContainsHiddenComponent(path) {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	root, ok := nearestGitWorkspaceRoot(path)
	if !ok {
		return false
	}
	return strings.TrimSpace(root) != ""
}

func sameGitWorkspaceRoot(a string, b string) bool {
	aRoot := gitWorkspaceIdentity(a)
	bRoot := gitWorkspaceIdentity(b)
	return aRoot != "" && bRoot != "" && sameCleanPath(aRoot, bRoot)
}

func nearestGitWorkspaceRoot(path string) (string, bool) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return "", false
	}
	candidates := make([]string, 0, 8)
	seen := map[string]struct{}{}
	addCandidate := func(candidate string) {
		candidate = filepath.Clean(strings.TrimSpace(candidate))
		if candidate == "" {
			return
		}
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		candidates = append(candidates, candidate)
	}
	addCandidate(path)
	addCandidate(filepath.Dir(path))
	for current := filepath.Dir(path); current != "" && current != filepath.Dir(current); current = filepath.Dir(current) {
		addCandidate(current)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return len(candidates[i]) > len(candidates[j])
	})
	for _, candidate := range candidates {
		root, err := gitRepoRootFromDir(candidate)
		if err == nil && strings.TrimSpace(root) != "" {
			return filepath.Clean(strings.TrimSpace(root)), true
		}
	}
	return "", false
}

func pathContainsHiddenComponent(path string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return false
	}
	volume := filepath.VolumeName(path)
	trimmed := strings.TrimPrefix(path, volume)
	trimmed = strings.TrimLeft(trimmed, `\/`)
	if trimmed == "" {
		return false
	}
	for _, part := range strings.FieldsFunc(trimmed, func(r rune) bool { return r == '\\' || r == '/' }) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}
