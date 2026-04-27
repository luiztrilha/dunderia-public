package team

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreflightAcceptsCodexLB(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "codex-lb.cmd")
	script := "@echo off\r\necho codex-lb 0.1.0\r\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write codex-lb shim: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	l := &Launcher{provider: "codex"}
	if err := l.Preflight(); err != nil {
		t.Fatalf("expected preflight to accept codex-lb, got %v", err)
	}
}
