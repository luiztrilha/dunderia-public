package team

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nex-crm/wuphf/internal/agent"
)

func TestBuildHeadlessOllamaPromptIncludesJSONToolInstructions(t *testing.T) {
	l := newHeadlessLauncherForTest()
	prompt := l.buildHeadlessOllamaPrompt("pm")

	for _, fragment := range []string{
		"== OLLAMA HEADLESS RUNTIME OVERRIDE ==",
		"Use JSON tool calls when you need to act in the office.",
		`{"name":"team_broadcast","arguments":{...}}`,
		`{"name":"team_task","arguments":{...}}`,
		`{"name":"local_exec","arguments":{"command":"...","working_directory":"...","timeout_ms":20000}}`,
	} {
		if !strings.Contains(prompt, fragment) {
			t.Fatalf("expected prompt to contain %q, got %q", fragment, prompt)
		}
	}
}

func TestRunHeadlessOllamaTurnExecutesBroadcastToolAndPublishesReply(t *testing.T) {
	oldFactory := headlessOllamaStreamFactory
	defer func() { headlessOllamaStreamFactory = oldFactory }()
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	headlessOllamaStreamFactory = func(_ context.Context, baseURL, model string) agent.StreamFn {
		return func(msgs []agent.Message, tools []agent.AgentTool) <-chan agent.StreamChunk {
			ch := make(chan agent.StreamChunk, 2)
			go func() {
				defer close(ch)
				ch <- agent.StreamChunk{
					Type:     "tool_call",
					ToolName: "team_broadcast",
					ToolParams: map[string]any{
						"channel":     "general",
						"reply_to_id": "msg-1",
						"content":     "Plain broadcast reply",
					},
				}
			}()
			return ch
		}
	}

	l := newHeadlessLauncherForTest()
	l.broker = NewBroker()
	l.broker.requests = nil
	l.broker.pendingInterview = nil
	if err := l.broker.SetSessionMode(SessionModeOffice, "ceo"); err != nil {
		t.Fatalf("SetSessionMode: %v", err)
	}
	ensureTestMemberAccess(l.broker, "general", "pm", "PM")
	before := len(l.broker.AllMessages())
	if err := l.runHeadlessOllamaTurn(context.Background(), "pm", `Reply using team_broadcast with my_slug "pm" and channel "general" reply_to_id "msg-1".`); err != nil {
		t.Fatalf("runHeadlessOllamaTurn: %v", err)
	}
	msgs := l.broker.AllMessages()
	if len(msgs) != before+1 {
		t.Fatalf("expected one new message, got before=%d after=%d", before, len(msgs))
	}
	msg := msgs[len(msgs)-1]
	if msg.Content != "Plain broadcast reply" || msg.ReplyTo != "msg-1" {
		t.Fatalf("unexpected broadcast message %+v", msg)
	}
}

func TestRunHeadlessOllamaTurnFailsOnEmptyFinalReply(t *testing.T) {
	oldFactory := headlessOllamaStreamFactory
	defer func() { headlessOllamaStreamFactory = oldFactory }()
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	headlessOllamaStreamFactory = func(_ context.Context, baseURL, model string) agent.StreamFn {
		return func(msgs []agent.Message, tools []agent.AgentTool) <-chan agent.StreamChunk {
			ch := make(chan agent.StreamChunk)
			close(ch)
			return ch
		}
	}

	l := newHeadlessLauncherForTest()
	l.broker = NewBroker()
	l.broker.requests = nil
	l.broker.pendingInterview = nil
	if err := l.broker.SetSessionMode(SessionModeOffice, "ceo"); err != nil {
		t.Fatalf("SetSessionMode: %v", err)
	}
	ensureTestMemberAccess(l.broker, "general", "pm", "PM")
	err := l.runHeadlessOllamaTurn(context.Background(), "pm", `Reply using team_broadcast with my_slug "pm" and channel "general" reply_to_id "msg-2".`)
	if err == nil {
		t.Fatal("expected empty ollama output to fail")
	}
	if !strings.Contains(err.Error(), "no plain-text reply") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestRunHeadlessOllamaTurnTaskToolCarriesExternalWorkspacePath(t *testing.T) {
	oldFactory := headlessOllamaStreamFactory
	defer func() { headlessOllamaStreamFactory = oldFactory }()
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	workspace := t.TempDir()
	initUsableGitWorktree(t, workspace)
	title := "Audit external repo task tool test"
	headlessOllamaStreamFactory = func(_ context.Context, baseURL, model string) agent.StreamFn {
		return func(msgs []agent.Message, tools []agent.AgentTool) <-chan agent.StreamChunk {
			ch := make(chan agent.StreamChunk, 2)
			go func() {
				defer close(ch)
				ch <- agent.StreamChunk{
					Type:     "tool_call",
					ToolName: "team_task",
					ToolParams: map[string]any{
						"action":         "create",
						"channel":        "general",
						"title":          title,
						"details":        "Inspect directly.",
						"owner":          "repo-auditor",
						"execution_mode": "external_workspace",
						"workspace_path": workspace,
					},
				}
				ch <- agent.StreamChunk{Type: "text", Content: "Task created."}
			}()
			return ch
		}
	}

	l := newHeadlessLauncherForTest()
	l.broker = NewBroker()
	l.broker.requests = nil
	l.broker.pendingInterview = nil
	if err := l.broker.SetSessionMode(SessionModeOffice, "ceo"); err != nil {
		t.Fatalf("SetSessionMode: %v", err)
	}
	ensureTestMemberAccess(l.broker, "general", "pm", "PM")
	if existing := l.broker.findMemberLocked("repo-auditor"); existing == nil {
		member := officeMember{Slug: "repo-auditor", Name: "Repo Auditor"}
		applyOfficeMemberDefaults(&member)
		l.broker.members = append(l.broker.members, member)
	}
	replyTo := fmt.Sprintf("msg-%d", 1)
	if err := l.runHeadlessOllamaTurn(context.Background(), "pm", fmt.Sprintf(`Reply using team_broadcast with my_slug "pm" and channel "general" reply_to_id "%s".`, replyTo)); err != nil {
		t.Fatalf("runHeadlessOllamaTurn: %v", err)
	}

	var task *teamTask
	for i := range l.broker.tasks {
		if l.broker.tasks[i].Title == title {
			task = &l.broker.tasks[i]
			break
		}
	}
	if task == nil {
		t.Fatalf("expected created task in %+v", l.broker.tasks)
	}
	if task.ExecutionMode != "external_workspace" || task.WorkspacePath != workspace {
		t.Fatalf("expected external workspace task, got %+v", *task)
	}
}

func TestRunHeadlessOllamaTurnLocalExecUsesLauncherCwdForGeneralTurn(t *testing.T) {
	oldFactory := headlessOllamaStreamFactory
	defer func() { headlessOllamaStreamFactory = oldFactory }()
	oldExec := headlessOllamaLocalExec
	defer func() { headlessOllamaLocalExec = oldExec }()
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	expectedCwd := t.TempDir()
	var gotWorkingDir string
	var gotCommand string
	headlessOllamaLocalExec = func(_ context.Context, workingDirectory, command string, timeout time.Duration) (headlessOllamaLocalExecResult, error) {
		gotWorkingDir = workingDirectory
		gotCommand = command
		return headlessOllamaLocalExecResult{
			Command:          command,
			WorkingDirectory: workingDirectory,
			ExitCode:         0,
			Stdout:           filepath.ToSlash(expectedCwd),
		}, nil
	}

	round := 0
	headlessOllamaStreamFactory = func(_ context.Context, baseURL, model string) agent.StreamFn {
		return func(msgs []agent.Message, tools []agent.AgentTool) <-chan agent.StreamChunk {
			hasLocalExec := false
			for _, tool := range tools {
				if tool.Name == "local_exec" {
					hasLocalExec = true
					break
				}
			}
			if !hasLocalExec {
				t.Fatal("expected local_exec tool to be exposed")
			}

			ch := make(chan agent.StreamChunk, 2)
			switch round {
			case 0:
				round++
				go func() {
					defer close(ch)
					ch <- agent.StreamChunk{
						Type:     "tool_call",
						ToolName: "local_exec",
						ToolParams: map[string]any{
							"command": "git rev-parse --show-toplevel",
						},
					}
				}()
			case 1:
				round++
				joined := joinOllamaMessagesForTest(msgs)
				if !strings.Contains(joined, `"tool":"local_exec"`) {
					t.Fatalf("expected follow-up round to include local_exec tool result, got %q", joined)
				}
				if !strings.Contains(joined, `"stdout":"`+filepath.ToSlash(expectedCwd)+`"`) {
					t.Fatalf("expected follow-up round to include stdout, got %q", joined)
				}
				go func() {
					defer close(ch)
					ch <- agent.StreamChunk{Type: "text", Content: "LOCAL_EXEC_GENERAL_OK"}
				}()
			default:
				t.Fatalf("unexpected round %d", round)
			}
			return ch
		}
	}

	l := newHeadlessLauncherForTest()
	l.cwd = expectedCwd
	l.broker = NewBroker()
	l.broker.requests = nil
	l.broker.pendingInterview = nil
	if err := l.broker.SetSessionMode(SessionModeOffice, "ceo"); err != nil {
		t.Fatalf("SetSessionMode: %v", err)
	}
	ensureTestMemberAccess(l.broker, "general", "pm", "PM")
	before := len(l.broker.AllMessages())
	if err := l.runHeadlessOllamaTurn(context.Background(), "pm", `Reply using team_broadcast with my_slug "pm" and channel "general" reply_to_id "msg-general".`); err != nil {
		t.Fatalf("runHeadlessOllamaTurn: %v", err)
	}
	if !samePathAllowWindowsAliasing(gotWorkingDir, expectedCwd) {
		t.Fatalf("expected local_exec cwd %q, got %q", expectedCwd, gotWorkingDir)
	}
	if gotCommand != "git rev-parse --show-toplevel" {
		t.Fatalf("unexpected command %q", gotCommand)
	}
	msgs := l.broker.AllMessages()
	if len(msgs) != before+1 {
		t.Fatalf("expected one new message, got before=%d after=%d", before, len(msgs))
	}
	if got := msgs[len(msgs)-1].Content; got != "LOCAL_EXEC_GENERAL_OK" {
		t.Fatalf("expected fallback reply to publish final text, got %q", got)
	}
}

func TestRunHeadlessOllamaTurnLocalExecUsesExternalWorkspaceForActiveTask(t *testing.T) {
	oldFactory := headlessOllamaStreamFactory
	defer func() { headlessOllamaStreamFactory = oldFactory }()
	oldExec := headlessOllamaLocalExec
	defer func() { headlessOllamaLocalExec = oldExec }()
	oldPathFn := brokerStatePath
	tmpDir := t.TempDir()
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	workspace := t.TempDir()
	var gotWorkingDir string
	headlessOllamaLocalExec = func(_ context.Context, workingDirectory, command string, timeout time.Duration) (headlessOllamaLocalExecResult, error) {
		gotWorkingDir = workingDirectory
		return headlessOllamaLocalExecResult{
			Command:          command,
			WorkingDirectory: workingDirectory,
			ExitCode:         0,
			Stdout:           filepath.ToSlash(workspace),
		}, nil
	}

	round := 0
	headlessOllamaStreamFactory = func(_ context.Context, baseURL, model string) agent.StreamFn {
		return func(msgs []agent.Message, tools []agent.AgentTool) <-chan agent.StreamChunk {
			ch := make(chan agent.StreamChunk, 2)
			switch round {
			case 0:
				round++
				go func() {
					defer close(ch)
					ch <- agent.StreamChunk{
						Type:     "tool_call",
						ToolName: "local_exec",
						ToolParams: map[string]any{
							"command": "git rev-parse --show-toplevel",
						},
					}
				}()
			case 1:
				round++
				joined := joinOllamaMessagesForTest(msgs)
				assignedWorkspace := ""
				for _, line := range strings.Split(joined, "\n") {
					if strings.HasPrefix(line, "Assigned working directory: ") {
						assignedWorkspace = strings.TrimSpace(strings.TrimPrefix(line, "Assigned working directory: "))
						break
					}
				}
				if assignedWorkspace == "" || !samePathAllowWindowsAliasing(assignedWorkspace, workspace) {
					t.Fatalf("expected follow-up round to carry assigned workspace %q, got %q in %q", workspace, assignedWorkspace, joined)
				}
				go func() {
					defer close(ch)
					ch <- agent.StreamChunk{Type: "text", Content: "LOCAL_EXEC_TASK_OK"}
				}()
			default:
				t.Fatalf("unexpected round %d", round)
			}
			return ch
		}
	}

	l := newHeadlessLauncherForTest()
	l.cwd = t.TempDir()
	l.broker = NewBroker()
	l.broker.requests = nil
	l.broker.pendingInterview = nil
	if err := l.broker.SetSessionMode(SessionModeOffice, "ceo"); err != nil {
		t.Fatalf("SetSessionMode: %v", err)
	}
	ensureTestMemberAccess(l.broker, "general", "pm", "PM")
	l.broker.tasks = append(l.broker.tasks, teamTask{
		ID:            "task-ext-1",
		Channel:       "general",
		Title:         "Inspect repo",
		Owner:         "pm",
		Status:        "in_progress",
		ExecutionMode: "external_workspace",
		WorkspacePath: workspace,
	})
	before := len(l.broker.AllMessages())
	if err := l.runHeadlessOllamaTurn(context.Background(), "pm", `Reply using team_broadcast with my_slug "pm" and channel "general" reply_to_id "msg-task".`); err != nil {
		t.Fatalf("runHeadlessOllamaTurn: %v", err)
	}
	if !samePathAllowWindowsAliasing(gotWorkingDir, workspace) {
		t.Fatalf("expected local_exec cwd %q, got %q", workspace, gotWorkingDir)
	}
	msgs := l.broker.AllMessages()
	if len(msgs) != before+1 {
		t.Fatalf("expected one new message, got before=%d after=%d", before, len(msgs))
	}
	if got := msgs[len(msgs)-1].Content; got != "LOCAL_EXEC_TASK_OK" {
		t.Fatalf("expected fallback reply to publish final text, got %q", got)
	}
}

func samePathAllowWindowsAliasing(a, b string) bool {
	if samePath(a, b) {
		return true
	}
	if runtime.GOOS != "windows" {
		return false
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func joinOllamaMessagesForTest(msgs []agent.Message) string {
	parts := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		parts = append(parts, msg.Role+": "+msg.Content)
	}
	return strings.Join(parts, "\n")
}
