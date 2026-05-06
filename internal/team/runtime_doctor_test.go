package team

import (
	"net/http"
	"testing"
	"time"
)

func TestRuntimeDoctorDetectsDuplicateWebProcesses(t *testing.T) {
	oldProcessList := runtimeProcessListFn
	oldNow := runtimeNowFn
	runtimeProcessListFn = func(string) []runtimeDoctorProcess {
		return []runtimeDoctorProcess{
			{PID: "1", Kind: "web", CommandLine: "wuphf-current.exe --web-port 7891"},
			{PID: "2", Kind: "web", CommandLine: "wuphf-current.exe --web-port 7891"},
			{PID: "3", Kind: "mcp", CommandLine: "wuphf-current.exe mcp-team"},
			{PID: "4", Kind: "cli", CommandLine: "wuphf-current.exe doctor --json"},
		}
	}
	runtimeNowFn = func() time.Time { return time.Unix(0, 0).UTC() }
	defer func() {
		runtimeProcessListFn = oldProcessList
		runtimeNowFn = oldNow
	}()

	snapshot := buildRuntimeDoctorSnapshot(studioDevConsoleState{WebUIOrigins: []string{"http://localhost:7891"}})
	if snapshot.Status != "blocked" {
		t.Fatalf("expected blocked duplicate process snapshot, got %+v", snapshot)
	}
	var found bool
	for _, check := range snapshot.Checks {
		if check.ID == "managed-runtime-processes" && check.Severity == runtimeDoctorFail {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected failing managed runtime check, got %+v", snapshot.Checks)
	}
}

func TestRuntimeDoctorDetectsDuplicateActiveWorktree(t *testing.T) {
	oldProcessList := runtimeProcessListFn
	runtimeProcessListFn = func(string) []runtimeDoctorProcess {
		return []runtimeDoctorProcess{{PID: "1", Kind: "web", CommandLine: "wuphf-current.exe --web-port 7891"}}
	}
	defer func() { runtimeProcessListFn = oldProcessList }()

	state := studioDevConsoleState{
		WebUIOrigins: []string{"http://localhost:7891"},
		Tasks: []teamTask{
			{ID: "task-1", Status: "in_progress", WorktreePath: "D:/Repos/.worktrees/shared"},
			{ID: "task-2", Status: "blocked", WorktreePath: "D:/Repos/.worktrees/shared"},
			{ID: "task-3", Status: "done", WorktreePath: "D:/Repos/.worktrees/shared"},
		},
	}
	snapshot := buildRuntimeDoctorSnapshot(state)
	if len(snapshot.QuarantineSignals) != 1 {
		t.Fatalf("expected one duplicate worktree signal, got %+v", snapshot.QuarantineSignals)
	}
	if snapshot.QuarantineSignals[0].Kind != "duplicate_active_worktree" {
		t.Fatalf("unexpected quarantine signal: %+v", snapshot.QuarantineSignals[0])
	}
}

func TestRuntimeDoctorEndpointIsServed(t *testing.T) {
	oldProcessList := runtimeProcessListFn
	runtimeProcessListFn = func(string) []runtimeDoctorProcess {
		return []runtimeDoctorProcess{{PID: "1", Kind: "web", CommandLine: "wuphf-current.exe --web-port 7891"}}
	}
	defer func() { runtimeProcessListFn = oldProcessList }()

	b := NewBroker()
	if err := b.StartOnPort(0); err != nil {
		t.Fatalf("start broker: %v", err)
	}
	defer b.Stop()

	req, err := http.NewRequest(http.MethodGet, "http://"+b.Addr()+"/runtime/doctor", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+b.Token())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("runtime doctor request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
