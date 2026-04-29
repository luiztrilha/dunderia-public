package agent

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestListRecentTasks_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	got, err := ListRecentTasks(dir, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %d entries", len(got))
	}
}

func TestListRecentTasks_OrdersByMtimeDesc(t *testing.T) {
	dir := t.TempDir()
	oldLog := filepath.Join(dir, "eng-100", "output.log")
	newLog := filepath.Join(dir, "ceo-200", "output.log")
	mustWriteLog(t, oldLog, `{"tool_name":"grep_search"}`+"\n")
	mustWriteLog(t, newLog, `{"tool_name":"send_message"}`+"\n")

	// Set mtimes explicitly so the test doesn't rely on wall-clock resolution.
	older := time.Now().Add(-time.Hour)
	newer := time.Now()
	if err := os.Chtimes(oldLog, older, older); err != nil {
		t.Fatalf("chtimes old: %v", err)
	}
	if err := os.Chtimes(newLog, newer, newer); err != nil {
		t.Fatalf("chtimes new: %v", err)
	}

	got, err := ListRecentTasks(dir, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(got))
	}
	if got[0].TaskID != "ceo-200" {
		t.Fatalf("expected ceo-200 first (newest), got %s", got[0].TaskID)
	}
	if got[0].AgentSlug != "ceo" {
		t.Fatalf("expected agent slug ceo, got %s", got[0].AgentSlug)
	}
	if got[0].ToolCallCount != 1 {
		t.Fatalf("expected 1 tool call, got %d", got[0].ToolCallCount)
	}
}

func TestReadTaskLog_ParsesJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eng-100", "output.log")
	mustWriteLog(t, path, `{"tool_name":"grep_search","agent_slug":"eng","started_at":1700000000000,"params":{"pattern":"svg"}}`+"\n"+
		`{"tool_name":"write_file","agent_slug":"eng","started_at":1700000001000,"params":{"path":"/tmp/x"}}`+"\n")

	entries, err := ReadTaskLog(dir, "eng-100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].ToolName != "grep_search" {
		t.Fatalf("first entry: want grep_search, got %s", entries[0].ToolName)
	}
	if entries[1].ToolName != "write_file" {
		t.Fatalf("second entry: want write_file, got %s", entries[1].ToolName)
	}
}

func TestReadTaskLog_SkipsCorruptLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eng-100", "output.log")
	mustWriteLog(t, path, `{"tool_name":"grep_search"}`+"\n"+
		"this is not json\n"+
		`{"tool_name":"write_file"}`+"\n")

	entries, err := ReadTaskLog(dir, "eng-100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 valid entries (corrupt line skipped), got %d", len(entries))
	}
}

func TestReadTaskLog_EmptyTaskID(t *testing.T) {
	_, err := ReadTaskLog(t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error for empty taskID")
	}
}

func TestReadTaskLog_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	mustWriteLog(t, filepath.Join(dir, "eng-100", "output.log"), "")
	entries, err := ReadTaskLog(dir, "eng-100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries from empty file, got %d", len(entries))
	}
}

func TestReadTaskLogRange_ReadsBoundedChunkWithHash(t *testing.T) {
	dir := t.TempDir()
	content := "first line\nsecond line\nthird line\n"
	mustWriteLog(t, filepath.Join(dir, "eng-100", "output.log"), content)

	got, err := ReadTaskLogRange(dir, "eng-100", 0, 12, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Content != "first line\ns" {
		t.Fatalf("unexpected content %q", got.Content)
	}
	if got.NextOffset != 12 {
		t.Fatalf("expected next offset 12, got %d", got.NextOffset)
	}
	sum := sha256.Sum256([]byte(content))
	if got.SHA256 != fmt.Sprintf("%x", sum[:]) {
		t.Fatalf("unexpected sha256 %q", got.SHA256)
	}
	if got.SizeBytes != int64(len(content)) {
		t.Fatalf("unexpected size %d", got.SizeBytes)
	}
}

func TestReadTaskLogRange_RejectsUnsafeTaskID(t *testing.T) {
	if _, err := ReadTaskLogRange(t.TempDir(), "../escape", 0, 10, false); err == nil {
		t.Fatal("expected error for unsafe task id")
	}
}

func mustWriteLog(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}
