package team

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	wuphf "github.com/nex-crm/wuphf"
	"github.com/nex-crm/wuphf/internal/config"
)

type runtimeDoctorSeverity string

const (
	runtimeDoctorOK   runtimeDoctorSeverity = "ok"
	runtimeDoctorWarn runtimeDoctorSeverity = "warn"
	runtimeDoctorFail runtimeDoctorSeverity = "fail"
	runtimeDoctorInfo runtimeDoctorSeverity = "info"
)

type runtimeDoctorCheck struct {
	ID       string                `json:"id"`
	Label    string                `json:"label"`
	Severity runtimeDoctorSeverity `json:"severity"`
	Summary  string                `json:"summary"`
	Detail   string                `json:"detail,omitempty"`
	NextStep string                `json:"next_step,omitempty"`
}

type runtimeDoctorProcess struct {
	PID         string `json:"pid,omitempty"`
	Kind        string `json:"kind,omitempty"`
	CommandLine string `json:"command_line,omitempty"`
}

type runtimeDoctorWebDist struct {
	Source       string `json:"source,omitempty"`
	IndexPath    string `json:"index_path,omitempty"`
	IndexHash    string `json:"index_hash,omitempty"`
	IndexModTime string `json:"index_mod_time,omitempty"`
	AssetCount   int    `json:"asset_count,omitempty"`
	Issue        string `json:"issue,omitempty"`
}

type runtimeDoctorQuarantineSignal struct {
	Kind     string   `json:"kind"`
	Severity string   `json:"severity"`
	Summary  string   `json:"summary"`
	TaskIDs  []string `json:"task_ids,omitempty"`
	Path     string   `json:"path,omitempty"`
	NextStep string   `json:"next_step,omitempty"`
}

type runtimeDoctorSnapshot struct {
	Status            string                          `json:"status"`
	GeneratedAt       string                          `json:"generated_at"`
	RuntimeHome       string                          `json:"runtime_home,omitempty"`
	WorkingDirectory  string                          `json:"working_directory,omitempty"`
	Executable        string                          `json:"executable,omitempty"`
	WebOrigins        []string                        `json:"web_origins,omitempty"`
	Processes         []runtimeDoctorProcess          `json:"processes,omitempty"`
	WebDist           runtimeDoctorWebDist            `json:"web_dist"`
	QuarantineSignals []runtimeDoctorQuarantineSignal `json:"quarantine_signals,omitempty"`
	RestartAdvice     runtimeRestartAdvice            `json:"restart_advice,omitempty"`
	SecretAudit       runtimeSecretAudit              `json:"secret_audit,omitempty"`
	BackupPolicy      runtimeBackupPolicy             `json:"backup_policy,omitempty"`
	Checks            []runtimeDoctorCheck            `json:"checks"`
}

var runtimeProcessListFn = detectRuntimeProcesses
var runtimeNowFn = time.Now
var runtimeStartedAt = time.Now().UTC()

func buildRuntimeDoctorSnapshot(state studioDevConsoleState) runtimeDoctorSnapshot {
	exe, _ := os.Executable()
	wd, _ := os.Getwd()
	snapshot := runtimeDoctorSnapshot{
		Status:           "ok",
		GeneratedAt:      runtimeNowFn().UTC().Format(time.RFC3339),
		RuntimeHome:      strings.TrimSpace(os.Getenv("WUPHF_RUNTIME_HOME")),
		WorkingDirectory: wd,
		Executable:       exe,
		WebOrigins:       append([]string(nil), state.WebUIOrigins...),
		Processes:        runtimeProcessListFn(exe),
		WebDist:          inspectRuntimeWebDist(exe),
	}
	snapshot.QuarantineSignals = runtimeQuarantineSignals(state, snapshot)
	snapshot.RestartAdvice = buildRuntimeRestartAdvice(snapshot)
	snapshot.SecretAudit = buildRuntimeSecretAudit()
	snapshot.BackupPolicy = buildRuntimeBackupPolicy()
	snapshot.Checks = runtimeDoctorChecks(snapshot)
	snapshot.Status = runtimeDoctorStatus(snapshot.Checks)
	return snapshot
}

func runtimeDoctorStatus(checks []runtimeDoctorCheck) string {
	status := "ok"
	for _, check := range checks {
		switch check.Severity {
		case runtimeDoctorFail:
			return "blocked"
		case runtimeDoctorWarn:
			status = "degraded"
		}
	}
	return status
}

func runtimeDoctorChecks(snapshot runtimeDoctorSnapshot) []runtimeDoctorCheck {
	checks := make([]runtimeDoctorCheck, 0, 4)
	webProcesses := 0
	mcpProcesses := 0
	for _, proc := range snapshot.Processes {
		switch proc.Kind {
		case "web":
			webProcesses++
		case "mcp":
			mcpProcesses++
		}
	}
	processCheck := runtimeDoctorCheck{
		ID:       "managed-runtime-processes",
		Label:    "Managed runtime",
		Severity: runtimeDoctorOK,
		Summary:  "One office web process is running.",
	}
	switch {
	case webProcesses == 0:
		processCheck.Severity = runtimeDoctorWarn
		processCheck.Summary = "No office web process was detected."
		processCheck.NextStep = "Restart MaestrIA on the original web port."
	case webProcesses > 1:
		processCheck.Severity = runtimeDoctorFail
		processCheck.Summary = fmt.Sprintf("%d office web processes are running.", webProcesses)
		processCheck.NextStep = "Stop duplicate wuphf web processes before continuing."
	default:
		processCheck.Detail = fmt.Sprintf("%d MCP team process(es) detected.", mcpProcesses)
	}
	checks = append(checks, processCheck)

	webDistCheck := runtimeDoctorCheck{
		ID:       "web-dist",
		Label:    "Web build",
		Severity: runtimeDoctorOK,
		Summary:  "Compiled web assets are available.",
	}
	if snapshot.WebDist.Issue != "" {
		webDistCheck.Severity = runtimeDoctorFail
		webDistCheck.Summary = snapshot.WebDist.Issue
		webDistCheck.NextStep = "Run `npm --prefix web run build`, then restart MaestrIA."
	} else if snapshot.WebDist.Source == "embedded" {
		webDistCheck.Severity = runtimeDoctorInfo
		webDistCheck.Summary = "Using embedded web assets."
	}
	if snapshot.WebDist.IndexHash != "" {
		webDistCheck.Detail = "index sha256 " + snapshot.WebDist.IndexHash
	}
	checks = append(checks, webDistCheck)

	quarantineCheck := runtimeDoctorCheck{
		ID:       "instance-quarantine",
		Label:    "Instance guard",
		Severity: runtimeDoctorOK,
		Summary:  "Runtime is not running from a task worktree or cloned temp home.",
	}
	for _, signal := range snapshot.QuarantineSignals {
		if signal.Severity == "fail" {
			quarantineCheck.Severity = runtimeDoctorFail
			break
		}
		if signal.Severity == "warn" && quarantineCheck.Severity != runtimeDoctorFail {
			quarantineCheck.Severity = runtimeDoctorWarn
		}
	}
	if len(snapshot.QuarantineSignals) > 0 {
		quarantineCheck.Summary = snapshot.QuarantineSignals[0].Summary
		quarantineCheck.NextStep = snapshot.QuarantineSignals[0].NextStep
	}
	checks = append(checks, quarantineCheck)

	restartCheck := runtimeDoctorCheck{
		ID:       "restart-advice",
		Label:    "Restart advisory",
		Severity: runtimeDoctorOK,
		Summary:  "No restart is required by the current diagnostics.",
	}
	if snapshot.RestartAdvice.Required {
		restartCheck.Severity = runtimeDoctorWarn
		restartCheck.Summary = snapshot.RestartAdvice.Summary
		restartCheck.NextStep = snapshot.RestartAdvice.NextStep
	}
	checks = append(checks, restartCheck)

	secretCheck := runtimeDoctorCheck{
		ID:       "secret-strict-audit",
		Label:    "Secret strict audit",
		Severity: runtimeDoctorOK,
		Summary:  "No plaintext config secrets detected.",
	}
	if snapshot.SecretAudit.PlaintextConfigCount > 0 {
		secretCheck.Severity = runtimeDoctorWarn
		secretCheck.Summary = fmt.Sprintf("%d plaintext config secret(s) detected.", snapshot.SecretAudit.PlaintextConfigCount)
		secretCheck.NextStep = "Run `wuphf secret migrate-config --write`, then clear plaintext when ready."
	}
	if snapshot.SecretAudit.Strict && snapshot.SecretAudit.PlaintextConfigCount > 0 {
		secretCheck.Severity = runtimeDoctorFail
		secretCheck.NextStep = "Strict mode requires encrypted secret refs; migrate and clear plaintext config secrets."
	}
	checks = append(checks, secretCheck)
	return checks
}

type runtimeRestartAdvice struct {
	Required bool     `json:"required"`
	Summary  string   `json:"summary,omitempty"`
	Reasons  []string `json:"reasons,omitempty"`
	NextStep string   `json:"next_step,omitempty"`
}

func buildRuntimeRestartAdvice(snapshot runtimeDoctorSnapshot) runtimeRestartAdvice {
	var reasons []string
	if strings.TrimSpace(snapshot.WebDist.Issue) != "" {
		reasons = append(reasons, snapshot.WebDist.Issue)
	}
	if parsed := parseBrokerTimestamp(snapshot.WebDist.IndexModTime); !parsed.IsZero() && parsed.After(runtimeStartedAt.Add(2*time.Second)) {
		reasons = append(reasons, "web build is newer than this runtime process")
	}
	webProcesses := 0
	for _, proc := range snapshot.Processes {
		if proc.Kind == "web" {
			webProcesses++
		}
	}
	if webProcesses > 1 {
		reasons = append(reasons, fmt.Sprintf("%d web runtime processes are active", webProcesses))
	}
	if len(reasons) == 0 {
		return runtimeRestartAdvice{}
	}
	return runtimeRestartAdvice{
		Required: true,
		Summary:  strings.Join(reasons, "; "),
		Reasons:  compactStringList(reasons),
		NextStep: "Rebuild if needed, then restart MaestrIA on the original broker/web ports after active agent turns settle.",
	}
}

type runtimeSecretAudit struct {
	Strict               bool     `json:"strict"`
	PlaintextConfigCount int      `json:"plaintext_config_count"`
	PlaintextConfigNames []string `json:"plaintext_config_names,omitempty"`
	SecretEnvCount       int      `json:"secret_env_count"`
	SecretEnvNames       []string `json:"secret_env_names,omitempty"`
	StorePath            string   `json:"store_path,omitempty"`
}

func buildRuntimeSecretAudit() runtimeSecretAudit {
	audit := runtimeSecretAudit{
		Strict:    parseSearchBool(os.Getenv("WUPHF_SECRET_STRICT")),
		StorePath: config.SecretStorePath(),
	}
	if cfg, err := config.Load(); err == nil {
		for _, candidate := range config.ConfigSecretCandidates(cfg) {
			if candidate.Present {
				audit.PlaintextConfigNames = append(audit.PlaintextConfigNames, candidate.Name)
			}
		}
	}
	for _, name := range knownSecretEnvNames() {
		if strings.TrimSpace(os.Getenv(name)) != "" {
			audit.SecretEnvNames = append(audit.SecretEnvNames, name)
		}
	}
	sort.Strings(audit.PlaintextConfigNames)
	sort.Strings(audit.SecretEnvNames)
	audit.PlaintextConfigCount = len(audit.PlaintextConfigNames)
	audit.SecretEnvCount = len(audit.SecretEnvNames)
	return audit
}

func knownSecretEnvNames() []string {
	return []string{
		"WUPHF_API_KEY", "NEX_API_KEY",
		"WUPHF_OPENAI_API_KEY", "OPENAI_API_KEY",
		"WUPHF_ANTHROPIC_API_KEY", "ANTHROPIC_API_KEY",
		"WUPHF_GEMINI_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY",
		"WUPHF_MINIMAX_API_KEY", "MINIMAX_API_KEY",
		"WUPHF_BRAVE_API_KEY", "BRAVE_API_KEY",
		"WUPHF_COMPOSIO_API_KEY", "COMPOSIO_API_KEY",
		"WUPHF_TELEGRAM_BOT_TOKEN", "TELEGRAM_BOT_TOKEN",
		"WUPHF_ONE_SECRET", "ONE_SECRET",
		"WUPHF_GITHUB_WEBHOOK_SECRET",
	}
}

type runtimeBackupPolicy struct {
	BrokerStatePath    string                   `json:"broker_state_path,omitempty"`
	HistoryDir         string                   `json:"history_dir,omitempty"`
	RetentionDays      int                      `json:"retention_days"`
	MaxSnapshots       int                      `json:"max_snapshots"`
	MaxMB              int                      `json:"max_mb"`
	LocalSnapshotCount int                      `json:"local_snapshot_count"`
	LocalSnapshotBytes int64                    `json:"local_snapshot_bytes,omitempty"`
	CloudProvider      string                   `json:"cloud_provider,omitempty"`
	CloudEnabled       bool                     `json:"cloud_enabled"`
	CloudPrefix        string                   `json:"cloud_prefix,omitempty"`
	Runtime            brokerCloudBackupRuntime `json:"runtime,omitempty"`
	NextStep           string                   `json:"next_step,omitempty"`
}

func buildRuntimeBackupPolicy() runtimeBackupPolicy {
	statePath := brokerStatePath()
	historyDir := brokerStateHistoryDir(statePath)
	count, bytes := countBackupHistory(historyDir)
	settings := resolvedCloudBackupSettings()
	policy := runtimeBackupPolicy{
		BrokerStatePath:    statePath,
		HistoryDir:         historyDir,
		RetentionDays:      config.ResolveBrokerHistoryRetentionDays(),
		MaxSnapshots:       config.ResolveBrokerHistoryMaxSnapshots(),
		MaxMB:              config.ResolveBrokerHistoryMaxMB(),
		LocalSnapshotCount: count,
		LocalSnapshotBytes: bytes,
		CloudProvider:      settings.Provider,
		CloudEnabled:       settings.Enabled(),
		CloudPrefix:        settings.Prefix,
		Runtime:            brokerCloudBackupRuntimeSnapshot(),
	}
	if policy.LocalSnapshotCount == 0 {
		policy.NextStep = "Let the broker persist state once, then local history snapshots will appear here."
	}
	if !policy.CloudEnabled {
		policy.NextStep = firstNonEmpty(policy.NextStep, "Cloud backup is optional; configure provider/bucket/prefix only if this office needs machine migration.")
	}
	return policy
}

func countBackupHistory(dir string) (int, int64) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0
	}
	var count int
	var total int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		count++
		total += info.Size()
	}
	return count, total
}

func inspectRuntimeWebDist(exe string) runtimeDoctorWebDist {
	candidates := runtimeWebDistCandidates(exe)
	for _, distDir := range candidates {
		indexPath := filepath.Join(distDir, "index.html")
		info, err := os.Stat(indexPath)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(indexPath)
		if err != nil {
			return runtimeDoctorWebDist{Source: "filesystem", IndexPath: indexPath, Issue: "web index exists but cannot be read"}
		}
		hash := sha256.Sum256(data)
		dist := runtimeDoctorWebDist{
			Source:       "filesystem",
			IndexPath:    indexPath,
			IndexHash:    hex.EncodeToString(hash[:])[:12],
			IndexModTime: info.ModTime().UTC().Format(time.RFC3339),
			AssetCount:   countRuntimeAssets(filepath.Join(distDir, "assets")),
		}
		if dist.AssetCount == 0 && strings.Contains(string(data), "/assets/") {
			dist.Issue = "web index references compiled assets, but no assets were found"
		}
		return dist
	}
	if _, ok := wuphf.WebFS(); ok {
		return runtimeDoctorWebDist{Source: "embedded"}
	}
	return runtimeDoctorWebDist{Issue: "web UI build missing"}
}

func runtimeWebDistCandidates(exe string) []string {
	var candidates []string
	if exe != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "web", "dist"))
	}
	candidates = append(candidates, filepath.Join("web", "dist"))
	return uniqueRuntimePaths(candidates)
}

func countRuntimeAssets(assetDir string) int {
	entries, err := os.ReadDir(assetDir)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			count++
		}
	}
	return count
}

func uniqueRuntimePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		cleaned := filepath.Clean(strings.TrimSpace(path))
		key := strings.ToLower(cleaned)
		if cleaned == "." || key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, cleaned)
	}
	return out
}

func runtimeQuarantineSignals(state studioDevConsoleState, snapshot runtimeDoctorSnapshot) []runtimeDoctorQuarantineSignal {
	var signals []runtimeDoctorQuarantineSignal
	if pathLooksLikeTemporaryInstance(snapshot.WorkingDirectory) {
		signals = append(signals, runtimeDoctorQuarantineSignal{
			Kind:     "working_directory_in_task_worktree",
			Severity: "fail",
			Summary:  "MaestrIA appears to be running from a task worktree or temporary clone.",
			Path:     snapshot.WorkingDirectory,
			NextStep: "Restart from the canonical MaestrIA repo before running office routines.",
		})
	}
	if pathLooksLikeTemporaryInstance(snapshot.RuntimeHome) {
		signals = append(signals, runtimeDoctorQuarantineSignal{
			Kind:     "runtime_home_in_task_worktree",
			Severity: "fail",
			Summary:  "WUPHF_RUNTIME_HOME points at a task worktree or temporary clone.",
			Path:     snapshot.RuntimeHome,
			NextStep: "Use the canonical runtime home before allowing agents to resume.",
		})
	}
	signals = append(signals, duplicateActiveWorktreeSignals(state.Tasks)...)
	return signals
}

func duplicateActiveWorktreeSignals(tasks []teamTask) []runtimeDoctorQuarantineSignal {
	byPath := make(map[string][]string)
	for _, task := range tasks {
		if taskIsTerminal(&task) {
			continue
		}
		path := strings.TrimSpace(firstNonEmpty(task.WorktreePath, task.WorkspacePath))
		if path == "" {
			continue
		}
		key := strings.ToLower(filepath.Clean(path))
		byPath[key] = append(byPath[key], strings.TrimSpace(task.ID))
	}
	var signals []runtimeDoctorQuarantineSignal
	for path, ids := range byPath {
		if len(ids) < 2 {
			continue
		}
		sort.Strings(ids)
		signals = append(signals, runtimeDoctorQuarantineSignal{
			Kind:     "duplicate_active_worktree",
			Severity: "warn",
			Summary:  fmt.Sprintf("%d active tasks share the same worktree.", len(ids)),
			TaskIDs:  ids,
			Path:     path,
			NextStep: "Assign each active execution lane to a distinct worktree before waking agents.",
		})
	}
	return signals
}

func pathLooksLikeTemporaryInstance(path string) bool {
	normalized := strings.ToLower(filepath.ToSlash(strings.TrimSpace(path)))
	if normalized == "" {
		return false
	}
	return strings.Contains(normalized, "/task-worktrees/") ||
		strings.Contains(normalized, "/.tmp-") ||
		strings.Contains(normalized, "/temp/dunderia") ||
		strings.Contains(normalized, "/tmp/dunderia")
}

func detectRuntimeProcesses(exe string) []runtimeDoctorProcess {
	switch runtime.GOOS {
	case "windows":
		return detectRuntimeProcessesWindows(exe)
	default:
		return detectRuntimeProcessesPOSIX(exe)
	}
}

func detectRuntimeProcessesWindows(exe string) []runtimeDoctorProcess {
	script := "Get-CimInstance Win32_Process -Filter \"name = 'wuphf-current.exe' or name = 'wuphf.exe'\" | Select-Object ProcessId,CommandLine | ConvertTo-Json -Compress"
	out, err := exec.Command("powershell", "-NoProfile", "-Command", script).CombinedOutput()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		return fallbackRuntimeProcess(exe)
	}
	return parseRuntimeProcessLines(string(out))
}

func detectRuntimeProcessesPOSIX(exe string) []runtimeDoctorProcess {
	name := filepath.Base(exe)
	if strings.TrimSpace(name) == "" {
		name = "wuphf"
	}
	out, err := exec.Command("pgrep", "-af", name).CombinedOutput()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		return fallbackRuntimeProcess(exe)
	}
	return parseRuntimeProcessLines(string(out))
}

func fallbackRuntimeProcess(exe string) []runtimeDoctorProcess {
	return []runtimeDoctorProcess{{
		PID:         fmt.Sprint(os.Getpid()),
		Kind:        "web",
		CommandLine: strings.TrimSpace(exe),
	}}
}

func parseRuntimeProcessLines(raw string) []runtimeDoctorProcess {
	if processes := parseRuntimeProcessesJSON(raw); len(processes) > 0 {
		return processes
	}
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	var processes []runtimeDoctorProcess
	for _, line := range lines {
		line = strings.TrimSpace(strings.Trim(line, "[]{}"))
		if line == "" {
			continue
		}
		processes = append(processes, runtimeDoctorProcess{
			Kind:        classifyRuntimeProcessKind(line),
			CommandLine: line,
		})
	}
	if len(processes) == 0 {
		return nil
	}
	return processes
}

func parseRuntimeProcessesJSON(raw string) []runtimeDoctorProcess {
	type processRow struct {
		ProcessID   any    `json:"ProcessId"`
		CommandLine string `json:"CommandLine"`
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || (!strings.HasPrefix(raw, "{") && !strings.HasPrefix(raw, "[")) {
		return nil
	}
	var rows []processRow
	if strings.HasPrefix(raw, "{") {
		var row processRow
		if err := json.Unmarshal([]byte(raw), &row); err != nil {
			return nil
		}
		rows = []processRow{row}
	} else if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil
	}
	processes := make([]runtimeDoctorProcess, 0, len(rows))
	for _, row := range rows {
		commandLine := strings.TrimSpace(row.CommandLine)
		processes = append(processes, runtimeDoctorProcess{
			PID:         fmt.Sprint(row.ProcessID),
			Kind:        classifyRuntimeProcessKind(commandLine),
			CommandLine: commandLine,
		})
	}
	return processes
}

func classifyRuntimeProcessKind(commandLine string) string {
	lower := strings.ToLower(strings.TrimSpace(commandLine))
	switch {
	case strings.Contains(lower, "mcp-team"):
		return "mcp"
	case strings.Contains(lower, " doctor"):
		return "cli"
	case strings.Contains(lower, " log"), strings.Contains(lower, " secret"), strings.Contains(lower, " import"):
		return "cli"
	default:
		return "web"
	}
}
