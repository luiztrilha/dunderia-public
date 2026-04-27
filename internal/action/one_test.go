package action

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nex-crm/wuphf/internal/config"
)

func fakeOneBinaryPath(dir, name string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(dir, name+".cmd")
	}
	return filepath.Join(dir, name)
}

func clearManagedOneEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"WUPHF_ONE_BIN", "WUPHF_ONE_SECRET", "ONE_SECRET",
		"WUPHF_ONE_IDENTITY", "ONE_IDENTITY",
		"WUPHF_ONE_IDENTITY_TYPE", "ONE_IDENTITY_TYPE",
	} {
		t.Setenv(key, "")
	}
}

func samePath(t *testing.T, left, right string) bool {
	t.Helper()
	leftInfo, err := os.Stat(left)
	if err != nil {
		t.Fatalf("stat %q: %v", left, err)
	}
	rightInfo, err := os.Stat(right)
	if err != nil {
		t.Fatalf("stat %q: %v", right, err)
	}
	return os.SameFile(leftInfo, rightInfo)
}

func writeFakeOne(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := fakeOneBinaryPath(dir, "one")
	script := `#!/bin/sh
if [ "$1" = "--agent" ]; then
  shift
fi

cmd1="$1"
cmd2="$1 $2"
cmd3="$1 $2 $3"

if [ "$cmd1" = "list" ]; then
  echo '{"total":1,"showing":1,"connections":[{"platform":"gmail","state":"operational","key":"live::gmail::default::abc123"}]}'
elif [ "$cmd3" = "actions search gmail" ]; then
  echo '{"actions":[{"actionId":"act-send","title":"Send Email","method":"POST","path":"/gmail/send"}]}'
elif [ "$cmd3" = "actions knowledge gmail" ]; then
  echo '{"knowledge":"Needs to, subject, body","method":"POST"}'
elif [ "$cmd3" = "actions execute gmail" ]; then
  echo '{"dryRun":true,"request":{"method":"POST","url":"https://api.withone.ai/send","headers":{"x-test":"1"},"data":{"to":"a@example.com"}}}'
elif [ "$cmd2" = "flow create" ]; then
  echo '{"created":true,"key":"'"$3"'","path":"/tmp/.one/flows/'"$3"'/flow.json"}'
elif [ "$cmd2" = "flow execute" ]; then
  echo '{"event":"step:start","stepId":"execute"}'
  echo '{"event":"workflow:result","runId":"run-1","logFile":"/tmp/run.log","status":"success","steps":{"execute":{"status":"success","response":{"ok":true,"posted":true,"channel":"#ops"}}}}'
elif [ "$cmd3" = "relay event-types gmail" ]; then
  echo '{"platform":"gmail","eventTypes":["message.received"]}'
elif [ "$cmd2" = "relay create" ]; then
  echo '{"id":"relay-1","url":"https://relay.example","active":false,"description":"mail relay","eventFilters":["message.received"]}'
elif [ "$cmd3" = "relay activate relay-1" ]; then
  echo '{"id":"relay-1","active":true,"actions":[{"type":"passthrough"}]}'
elif [ "$cmd2" = "relay events" ]; then
  echo '{"total":1,"showing":1,"events":[{"id":"evt-1","platform":"gmail","eventType":"message.received","timestamp":"2026-03-29T10:00:00Z"}]}'
elif [ "$cmd3" = "relay event evt-1" ]; then
  echo '{"id":"evt-1","platform":"gmail","eventType":"message.received","timestamp":"2026-03-29T10:00:00Z","payload":{"from":"a@example.com"}}'
else
  echo "unexpected args: $*" >&2
  exit 1
fi
`
	if runtime.GOOS == "windows" {
		script = `@echo off
if "%1"=="--agent" shift
if "%1"=="list" (
  echo {"total":1,"showing":1,"connections":[{"platform":"gmail","state":"operational","key":"live::gmail::default::abc123"}]}
  exit /b 0
)
if "%1 %2 %3"=="actions search gmail" (
  echo {"actions":[{"actionId":"act-send","title":"Send Email","method":"POST","path":"/gmail/send"}]}
  exit /b 0
)
if "%1 %2 %3"=="actions knowledge gmail" (
  echo {"knowledge":"Needs to, subject, body","method":"POST"}
  exit /b 0
)
if "%1 %2 %3"=="actions execute gmail" (
  echo {"dryRun":true,"request":{"method":"POST","url":"https://api.withone.ai/send","headers":{"x-test":"1"},"data":{"to":"a@example.com"}}}
  exit /b 0
)
if "%1 %2"=="flow create" (
  echo {"created":true,"key":"%3","path":"C:\\tmp\\.one\\flows\\%3\\flow.json"}
  exit /b 0
)
if "%1 %2"=="flow execute" (
  echo {"event":"step:start","stepId":"execute"}
  echo {"event":"workflow:result","runId":"run-1","logFile":"C:\\tmp\\run.log","status":"success","steps":{"execute":{"status":"success","response":{"ok":true,"posted":true,"channel":"#ops"}}}}
  exit /b 0
)
if "%1 %2 %3"=="relay event-types gmail" (
  echo {"platform":"gmail","eventTypes":["message.received"]}
  exit /b 0
)
if "%1 %2"=="relay create" (
  echo {"id":"relay-1","url":"https://relay.example","active":false,"description":"mail relay","eventFilters":["message.received"]}
  exit /b 0
)
if "%1 %2 %3"=="relay activate relay-1" (
  echo {"id":"relay-1","active":true,"actions":[{"type":"passthrough"}]}
  exit /b 0
)
if "%1 %2"=="relay events" (
  echo {"total":1,"showing":1,"events":[{"id":"evt-1","platform":"gmail","eventType":"message.received","timestamp":"2026-03-29T10:00:00Z"}]}
  exit /b 0
)
if "%1 %2 %3"=="relay event evt-1" (
  echo {"id":"evt-1","platform":"gmail","eventType":"message.received","timestamp":"2026-03-29T10:00:00Z","payload":{"from":"a@example.com"}}
  exit /b 0
)
echo unexpected args: %* 1>&2
exit /b 1
`
	}
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOneCLIHappyPath(t *testing.T) {
	oneBin := writeFakeOne(t)
	client := &OneCLI{Bin: oneBin, WorkDir: t.TempDir(), Env: []string{"ONE_SECRET=test-secret"}}
	ctx := context.Background()

	connections, err := client.ListConnections(ctx, ListConnectionsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(connections.Connections); got != 1 {
		t.Fatalf("expected 1 connection, got %d", got)
	}

	search, err := client.SearchActions(ctx, "gmail", "send email", "execute")
	if err != nil {
		t.Fatal(err)
	}
	if got := search.Actions[0].ActionID; got != "act-send" {
		t.Fatalf("unexpected action id %q", got)
	}

	knowledge, err := client.ActionKnowledge(ctx, "gmail", "act-send")
	if err != nil {
		t.Fatal(err)
	}
	if knowledge.Method != "POST" {
		t.Fatalf("unexpected method %q", knowledge.Method)
	}

	executed, err := client.ExecuteAction(ctx, ExecuteRequest{
		Platform:      "gmail",
		ActionID:      "act-send",
		ConnectionKey: "live::gmail::default::abc123",
		Data: map[string]any{
			"to": "a@example.com",
		},
		DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !executed.DryRun || executed.Request.Method != "POST" {
		t.Fatalf("unexpected execute result %+v", executed)
	}

	created, err := client.CreateWorkflow(ctx, WorkflowCreateRequest{
		Key:        "welcome-flow",
		Definition: []byte(`{"key":"welcome-flow","name":"Welcome","version":"1","inputs":{},"steps":[]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created.Created {
		t.Fatalf("expected created workflow, got %+v", created)
	}

	workflow, err := client.ExecuteWorkflow(ctx, WorkflowExecuteRequest{KeyOrPath: "welcome-flow"})
	if err != nil {
		t.Fatal(err)
	}
	if workflow.RunID != "run-1" || workflow.Status != "success" {
		t.Fatalf("unexpected workflow result %+v", workflow)
	}

	eventTypes, err := client.RelayEventTypes(ctx, "gmail")
	if err != nil {
		t.Fatal(err)
	}
	if len(eventTypes.EventTypes) != 1 {
		t.Fatalf("unexpected event types %+v", eventTypes)
	}

	relay, err := client.CreateRelay(ctx, RelayCreateRequest{
		ConnectionKey: "live::gmail::default::abc123",
		Description:   "mail relay",
		EventFilters:  []string{"message.received"},
		CreateWebhook: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if relay.ID != "relay-1" {
		t.Fatalf("unexpected relay %+v", relay)
	}

	relay, err = client.ActivateRelay(ctx, RelayActivateRequest{
		ID:      "relay-1",
		Actions: []byte(`[{"type":"passthrough"}]`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !relay.Active {
		t.Fatalf("expected active relay, got %+v", relay)
	}

	events, err := client.ListRelayEvents(ctx, RelayEventsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(events.Events); got != 1 {
		t.Fatalf("expected 1 relay event, got %d", got)
	}

	detail, err := client.GetRelayEvent(ctx, "evt-1")
	if err != nil {
		t.Fatal(err)
	}
	if detail.ID != "evt-1" {
		t.Fatalf("unexpected relay detail %+v", detail)
	}
}

func TestNewOneCLIFromEnvUsesManagedIdentity(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearManagedOneEnv(t)
	if err := config.Save(config.Config{
		APIKey:    "nex-key",
		OneAPIKey: "one-secret",
		Email:     "ceo@example.com",
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	client := NewOneCLIFromEnv()
	got := strings.Join(client.Env, " ")
	if !strings.Contains(got, "ONE_SECRET=one-secret") {
		t.Fatalf("expected ONE_SECRET env, got %q", got)
	}
	if !strings.Contains(got, "ONE_IDENTITY=ceo@example.com") {
		t.Fatalf("expected ONE_IDENTITY env, got %q", got)
	}
	if !strings.Contains(got, "ONE_IDENTITY_TYPE=user") {
		t.Fatalf("expected ONE_IDENTITY_TYPE env, got %q", got)
	}
}

func TestOneCLIRunsWithoutManagedProvisioning(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearManagedOneEnv(t)
	oneBin := writeFakeOne(t)
	client := &OneCLI{Bin: oneBin, WorkDir: t.TempDir()}
	result, err := client.ListConnections(context.Background(), ListConnectionsOptions{})
	if err != nil {
		t.Fatalf("expected local One config/bin fallback to run, got %v", err)
	}
	if got := len(result.Connections); got != 1 {
		t.Fatalf("expected 1 connection, got %d", got)
	}
}

func TestNewOneCLIFromEnvFallsBackToNpx(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearManagedOneEnv(t)
	dir := t.TempDir()
	npxPath := fakeOneBinaryPath(dir, "npx")
	script := `#!/bin/sh
if [ "$1" != "-y" ] || [ "$2" != "@withone/cli" ] || [ "$3" != "--agent" ]; then
  echo "unexpected prefix: $*" >&2
  exit 1
fi
shift 3
if [ "$1" = "list" ]; then
  echo '{"total":1,"showing":1,"connections":[{"platform":"gmail","state":"operational","key":"live::gmail::default::abc123"}]}'
  exit 0
fi
echo "unexpected args: $*" >&2
exit 1
`
	if runtime.GOOS == "windows" {
		script = `@echo off
if not "%1"=="-y" (
  echo unexpected prefix: %* 1>&2
  exit /b 1
)
if not "%2"=="@withone/cli" (
  echo unexpected prefix: %* 1>&2
  exit /b 1
)
if not "%3"=="--agent" (
  echo unexpected prefix: %* 1>&2
  exit /b 1
)
shift
shift
shift
if "%1"=="list" (
  echo {"total":1,"showing":1,"connections":[{"platform":"gmail","state":"operational","key":"live::gmail::default::abc123"}]}
  exit /b 0
)
echo unexpected args: %* 1>&2
exit /b 1
`
	}
	if err := os.WriteFile(npxPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	client := NewOneCLIFromEnv()
	if client.Bin != "npx" {
		t.Fatalf("expected npx fallback, got %q", client.Bin)
	}
	if got := strings.Join(client.ArgsPrefix, " "); got != "-y @withone/cli" {
		t.Fatalf("unexpected args prefix %q", got)
	}

	result, err := client.ListConnections(context.Background(), ListConnectionsOptions{})
	if err != nil {
		t.Fatalf("expected npx-backed one cli to run, got %v", err)
	}
	if got := len(result.Connections); got != 1 {
		t.Fatalf("expected 1 connection, got %d", got)
	}
}

func TestOneCLIListConnectionsUsesSafeActionWorkDir(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	workDir := t.TempDir()
	traceFile := filepath.Join(t.TempDir(), "pwd.txt")
	oneBin := fakeOneBinaryPath(t.TempDir(), "one")
	script := `#!/bin/sh
if [ "$1" = "--agent" ]; then
  shift
fi
pwd > "` + traceFile + `"
if [ "$1" = "list" ]; then
  echo '{"total":1,"showing":1,"connections":[{"platform":"notion","state":"operational","key":"live::notion::default::abc123"}]}'
  exit 0
fi
echo "unexpected args: $*" >&2
exit 1
`
	if runtime.GOOS == "windows" {
		script = `@echo off
if "%1"=="--agent" shift
cd > "` + traceFile + `"
if "%1"=="list" (
  echo {"total":1,"showing":1,"connections":[{"platform":"notion","state":"operational","key":"live::notion::default::abc123"}]}
  exit /b 0
)
echo unexpected args: %* 1>&2
exit /b 1
`
	}
	if err := os.WriteFile(oneBin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	client := &OneCLI{Bin: oneBin, WorkDir: workDir}

	result, err := client.ListConnections(context.Background(), ListConnectionsOptions{})
	if err != nil {
		t.Fatalf("ListConnections returned error: %v", err)
	}
	if len(result.Connections) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(result.Connections))
	}

	usedDirRaw, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatalf("read trace file: %v", err)
	}
	usedDir := strings.TrimSpace(string(usedDirRaw))
	expectedDir, err := filepath.EvalSymlinks(homeDir)
	if err != nil {
		t.Fatalf("resolve home dir: %v", err)
	}
	if !samePath(t, usedDir, expectedDir) {
		t.Fatalf("expected ListConnections to run from home dir %q, got %q", expectedDir, usedDir)
	}
}

func TestOneCLIExecuteWorkflowKeepsFlowWorkDir(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	workDir := t.TempDir()
	traceFile := filepath.Join(t.TempDir(), "workflow-pwd.txt")
	oneBin := fakeOneBinaryPath(t.TempDir(), "one")
	script := `#!/bin/sh
if [ "$1" = "--agent" ]; then
  shift
fi
pwd > "` + traceFile + `"
if [ "$1 $2 $3" = "flow execute welcome-flow" ]; then
  echo '{"event":"workflow:result","runId":"run-1","logFile":"/tmp/run.log","status":"success","steps":{"step-1":{"status":"success"}}}'
  exit 0
fi
echo "unexpected args: $*" >&2
exit 1
`
	if runtime.GOOS == "windows" {
		script = `@echo off
if "%1"=="--agent" shift
cd > "` + traceFile + `"
if "%1 %2 %3"=="flow execute welcome-flow" (
  echo {"event":"workflow:result","runId":"run-1","logFile":"C:\\tmp\\run.log","status":"success","steps":{"step-1":{"status":"success"}}}
  exit /b 0
)
echo unexpected args: %* 1>&2
exit /b 1
`
	}
	if err := os.WriteFile(oneBin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	client := &OneCLI{Bin: oneBin, WorkDir: workDir}

	workflow, err := client.ExecuteWorkflow(context.Background(), WorkflowExecuteRequest{KeyOrPath: "welcome-flow"})
	if err != nil {
		t.Fatalf("ExecuteWorkflow returned error: %v", err)
	}
	if workflow.RunID != "run-1" || workflow.Status != "success" {
		t.Fatalf("unexpected workflow result %+v", workflow)
	}

	usedDirRaw, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatalf("read trace file: %v", err)
	}
	usedDir := strings.TrimSpace(string(usedDirRaw))
	expectedDir, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		t.Fatalf("resolve workdir: %v", err)
	}
	if !samePath(t, usedDir, expectedDir) {
		t.Fatalf("expected ExecuteWorkflow to run from flow workdir %q, got %q", expectedDir, usedDir)
	}
}

func TestOneCLIExecuteActionAutoResolvesConnectionViaTempFlow(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	oneBin := writeFakeOne(t)
	client := &OneCLI{Bin: oneBin, WorkDir: t.TempDir()}

	result, err := client.ExecuteAction(context.Background(), ExecuteRequest{
		Platform: "slack",
		ActionID: "post-message",
		Data: map[string]any{
			"channel": "#ops",
			"text":    "hello",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteAction returned error: %v", err)
	}
	if result.DryRun {
		t.Fatalf("expected live temp-flow execution result, got dry-run %+v", result)
	}
	if !strings.Contains(string(result.Response), `"posted":true`) {
		t.Fatalf("expected flow response payload, got %s", string(result.Response))
	}
}
