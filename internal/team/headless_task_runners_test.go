package team

import (
	"reflect"
	"testing"
)

func TestParseHeadlessTaskRunnerProcesses(t *testing.T) {
	input := []byte(" 123 node /opt/homebrew/bin/codex -a never -s workspace-write exec -C /private/var/folders/x/T/wuphf-task-task-3 -c mcp_servers.wuphf-office.command=\"/tmp/wuphf\" -\n456 /usr/bin/other\n")
	got := parseHeadlessTaskRunnerProcesses(input)
	want := []headlessTaskRunnerProcess{
		{
			PID:     123,
			Command: "node /opt/homebrew/bin/codex -a never -s workspace-write exec -C /private/var/folders/x/T/wuphf-task-task-3 -c mcp_servers.wuphf-office.command=\"/tmp/wuphf\" -",
		},
		{
			PID:     456,
			Command: "/usr/bin/other",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseHeadlessTaskRunnerProcesses() = %#v, want %#v", got, want)
	}
}

func TestKillStaleHeadlessTaskRunnersKillsOnlyMatchingProcesses(t *testing.T) {
	oldList := listHeadlessTaskRunnerProcesses
	oldKill := killHeadlessTaskRunnerProcess
	listHeadlessTaskRunnerProcesses = func() ([]byte, error) {
		return []byte("123 node /opt/homebrew/bin/codex -a never -s workspace-write exec -C /private/var/folders/x/T/wuphf-task-task-3 -c mcp_servers.wuphf-office.command=\"/tmp/wuphf\" -\n124 /opt/homebrew/bin/codex exec -C /tmp/elsewhere -\n125 node /opt/homebrew/bin/codex -a never -s workspace-write exec -C /private/var/folders/x/T/wuphf-task-task-9 -c something_else=1 -\n"), nil
	}
	defer func() {
		listHeadlessTaskRunnerProcesses = oldList
		killHeadlessTaskRunnerProcess = oldKill
	}()

	var killed []int
	killHeadlessTaskRunnerProcess = func(pid int) {
		killed = append(killed, pid)
	}

	killStaleHeadlessTaskRunners()

	if !reflect.DeepEqual(killed, []int{123}) {
		t.Fatalf("killStaleHeadlessTaskRunners() killed %#v, want [123]", killed)
	}
}

func TestParseWindowsHeadlessTaskRunnerProcesses(t *testing.T) {
	input := []byte(`[{"ProcessId":123,"CommandLine":"node codex exec -C C:\\Users\\l.sousa\\.wuphf\\task-worktrees\\dunderia\\wuphf-task-task-3 -c mcp_servers.wuphf-office.command=\"D:\\Repos\\dunderia\\.tmp-wuphf-live2.exe\" -"},{"ProcessId":456,"CommandLine":"C:\\Windows\\System32\\notepad.exe"}]`)
	got := parseWindowsHeadlessTaskRunnerProcesses(input)
	want := []headlessTaskRunnerProcess{
		{
			PID:     123,
			Command: `node codex exec -C C:\Users\l.sousa\.wuphf\task-worktrees\dunderia\wuphf-task-task-3 -c mcp_servers.wuphf-office.command="D:\Repos\dunderia\.tmp-wuphf-live2.exe" -`,
		},
		{
			PID:     456,
			Command: `C:\Windows\System32\notepad.exe`,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseWindowsHeadlessTaskRunnerProcesses() = %#v, want %#v", got, want)
	}
}
