package team

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type OpenSpecSummary struct {
	Root          string   `json:"root,omitempty"`
	ProjectPath   string   `json:"project_path,omitempty"`
	ProjectTitle  string   `json:"project_title,omitempty"`
	ActiveChanges []string `json:"active_changes,omitempty"`
}

func (s OpenSpecSummary) Available() bool {
	return strings.TrimSpace(s.Root) != ""
}

func (s OpenSpecSummary) FormatLines() []string {
	if !s.Available() {
		return nil
	}
	lines := []string{
		"- OpenSpec detected in " + s.Root,
	}
	if strings.TrimSpace(s.ProjectPath) != "" {
		title := strings.TrimSpace(s.ProjectTitle)
		if title == "" {
			title = s.ProjectPath
		}
		lines = append(lines, "- Project spec: "+title)
	}
	if len(s.ActiveChanges) > 0 {
		lines = append(lines, "- Active changes: "+strings.Join(s.ActiveChanges, ", "))
	}
	lines = append(lines, "- Respect existing proposal/tasks/design files before broad implementation changes.")
	return lines
}

func (s OpenSpecSummary) TaskPacketLines() []string {
	if !s.Available() {
		return nil
	}
	lines := []string{
		"OpenSpec rule: this workspace uses OpenSpec. Check the existing proposal/tasks/design files before widening scope or inventing a parallel plan.",
	}
	if strings.TrimSpace(s.ProjectPath) != "" {
		lines = append(lines, "OpenSpec project file: "+s.ProjectPath)
	}
	if len(s.ActiveChanges) > 0 {
		lines = append(lines, "OpenSpec active changes: "+strings.Join(s.ActiveChanges, ", "))
	}
	return lines
}

func DetectOpenSpec(paths ...string) OpenSpecSummary {
	for _, raw := range paths {
		root := findOpenSpecRoot(raw)
		if root == "" {
			continue
		}
		return buildOpenSpecSummary(root)
	}
	return OpenSpecSummary{}
}

func detectOpenSpecForRuntimeTasks(tasks []RuntimeTask) OpenSpecSummary {
	paths := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if path := effectiveRuntimeTaskWorkspacePath(task); path != "" {
			paths = append(paths, path)
		}
	}
	return DetectOpenSpec(paths...)
}

func detectOpenSpecForTeamTask(task *teamTask) OpenSpecSummary {
	if task == nil {
		return OpenSpecSummary{}
	}
	return DetectOpenSpec(effectiveTeamTaskWorkspacePath(task))
}

func findOpenSpecRoot(start string) string {
	start = strings.TrimSpace(start)
	if start == "" {
		return ""
	}
	info, err := os.Stat(start)
	if err != nil {
		return ""
	}
	current := start
	if !info.IsDir() {
		current = filepath.Dir(start)
	}
	for {
		candidate := filepath.Join(current, "openspec")
		if dirExists(candidate) {
			return filepath.Clean(candidate)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func buildOpenSpecSummary(root string) OpenSpecSummary {
	summary := OpenSpecSummary{Root: filepath.Clean(root)}
	projectPath := filepath.Join(root, "project.md")
	if fileExists(projectPath) {
		summary.ProjectPath = projectPath
		summary.ProjectTitle = openSpecProjectTitle(projectPath)
	}
	changesDir := filepath.Join(root, "changes")
	entries, err := os.ReadDir(changesDir)
	if err != nil {
		return summary
	}
	changes := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		changes = append(changes, name)
	}
	sort.Strings(changes)
	if len(changes) > 5 {
		changes = changes[:5]
	}
	summary.ActiveChanges = changes
	return summary
}

func openSpecProjectTitle(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "#")
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
