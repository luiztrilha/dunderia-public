package team

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectOpenSpecFindsProjectAndChanges(t *testing.T) {
	root := t.TempDir()
	openSpecDir := filepath.Join(root, "openspec")
	if err := os.MkdirAll(filepath.Join(openSpecDir, "changes", "ship-login"), 0o755); err != nil {
		t.Fatalf("mkdir openspec changes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(openSpecDir, "project.md"), []byte("# Product Spec\n\n"), 0o644); err != nil {
		t.Fatalf("write project.md: %v", err)
	}

	got := DetectOpenSpec(root)
	if !got.Available() {
		t.Fatal("expected openspec summary to be available")
	}
	if got.ProjectTitle != "Product Spec" {
		t.Fatalf("expected project title, got %+v", got)
	}
	if len(got.ActiveChanges) != 1 || got.ActiveChanges[0] != "ship-login" {
		t.Fatalf("expected active change, got %+v", got.ActiveChanges)
	}
}

func TestBuildTaskExecutionPacketIncludesOpenSpecGuidance(t *testing.T) {
	root := t.TempDir()
	openSpecDir := filepath.Join(root, "openspec")
	if err := os.MkdirAll(filepath.Join(openSpecDir, "changes", "ship-login"), 0o755); err != nil {
		t.Fatalf("mkdir openspec changes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(openSpecDir, "project.md"), []byte("# Product Spec\n"), 0o644); err != nil {
		t.Fatalf("write project.md: %v", err)
	}

	l := &Launcher{}
	got := l.buildTaskExecutionPacket("eng", officeActionLog{
		Kind:  "task_updated",
		Actor: "ceo",
	}, teamTask{
		ID:            "task-21",
		Channel:       "general",
		Title:         "Implement login slice",
		Owner:         "eng",
		Status:        "in_progress",
		ExecutionMode: "local_worktree",
		WorktreePath:  root,
	}, "Implement the smallest runnable slice.")
	if !strings.Contains(got, "OpenSpec rule: this workspace uses OpenSpec") {
		t.Fatalf("expected openspec guidance in packet: %q", got)
	}
	if !strings.Contains(got, "OpenSpec active changes: ship-login") {
		t.Fatalf("expected active change in packet: %q", got)
	}
}
