package team

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

var listHeadlessTaskRunnerProcesses = func() ([]byte, error) {
	if runtime.GOOS == "windows" {
		return exec.Command(
			"powershell",
			"-NoProfile",
			"-Command",
			`Get-CimInstance Win32_Process | Select-Object ProcessId, CommandLine | ConvertTo-Json -Compress`,
		).Output()
	}
	return exec.Command("ps", "-axo", "pid=,command=").Output()
}

var killHeadlessTaskRunnerProcess = func(pid int) {
	terminateHeadlessProcessPID(pid)
}

type headlessTaskRunnerProcess struct {
	PID     int
	Command string
}

func killStaleHeadlessTaskRunners() {
	output, err := listHeadlessTaskRunnerProcesses()
	if err != nil {
		return
	}
	seen := map[int]struct{}{}
	for _, proc := range parseHeadlessTaskRunnerProcesses(output) {
		if !isHeadlessTaskRunnerCommand(proc.Command) {
			continue
		}
		if _, ok := seen[proc.PID]; ok {
			continue
		}
		seen[proc.PID] = struct{}{}
		killHeadlessTaskRunnerProcess(proc.PID)
	}
}

func parseHeadlessTaskRunnerProcesses(output []byte) []headlessTaskRunnerProcess {
	if runtime.GOOS == "windows" {
		if processes := parseWindowsHeadlessTaskRunnerProcesses(output); len(processes) > 0 {
			return processes
		}
	}
	lines := bytes.Split(output, []byte{'\n'})
	processes := make([]headlessTaskRunnerProcess, 0, len(lines))
	for _, raw := range lines {
		line := strings.TrimSpace(string(raw))
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 0 {
			continue
		}
		command := strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
		if command == "" {
			continue
		}
		processes = append(processes, headlessTaskRunnerProcess{
			PID:     pid,
			Command: command,
		})
	}
	return processes
}

func parseWindowsHeadlessTaskRunnerProcesses(output []byte) []headlessTaskRunnerProcess {
	type processRecord struct {
		ProcessID   int    `json:"ProcessId"`
		CommandLine string `json:"CommandLine"`
	}
	var many []processRecord
	if err := json.Unmarshal(output, &many); err == nil {
		processes := make([]headlessTaskRunnerProcess, 0, len(many))
		for _, proc := range many {
			if proc.ProcessID <= 0 || strings.TrimSpace(proc.CommandLine) == "" {
				continue
			}
			processes = append(processes, headlessTaskRunnerProcess{
				PID:     proc.ProcessID,
				Command: strings.TrimSpace(proc.CommandLine),
			})
		}
		return processes
	}
	var one processRecord
	if err := json.Unmarshal(output, &one); err == nil && one.ProcessID > 0 && strings.TrimSpace(one.CommandLine) != "" {
		return []headlessTaskRunnerProcess{{
			PID:     one.ProcessID,
			Command: strings.TrimSpace(one.CommandLine),
		}}
	}
	return nil
}

func isHeadlessTaskRunnerCommand(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	return strings.Contains(command, "codex") &&
		strings.Contains(command, "wuphf-task-") &&
		strings.Contains(command, "mcp_servers.wuphf-office.command=")
}
