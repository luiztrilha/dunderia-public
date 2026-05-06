package team

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/nex-crm/wuphf/internal/commands"
)

type commandManifestResponse struct {
	GeneratedAt string                 `json:"generated_at"`
	Persisted   bool                   `json:"persisted"`
	Summary     map[string]int         `json:"summary"`
	Commands    []commandManifestEntry `json:"commands"`
}

type commandManifestEntry = commands.ManifestEntry

type commandManifestDriftResponse struct {
	GeneratedAt string                     `json:"generated_at"`
	Persisted   bool                       `json:"persisted"`
	Status      string                     `json:"status"`
	Summary     map[string]int             `json:"summary"`
	Items       []commandManifestDriftItem `json:"items"`
}

type commandManifestDriftItem struct {
	Command  string `json:"command"`
	Category string `json:"category,omitempty"`
	Severity string `json:"severity"`
	Kind     string `json:"kind"`
	Summary  string `json:"summary"`
}

func (b *Broker) handleCommandManifest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	category := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("category")))
	surface := commands.NormalizeManifestSurface(r.URL.Query().Get("surface"))
	if surface == "" {
		http.Error(w, "unknown surface", http.StatusBadRequest)
		return
	}
	mutatingOnly := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("mutating")), "true")
	manifest := commands.FilterCommandManifest(commands.BuildCommandManifest(), surface)
	filtered := make([]commandManifestEntry, 0, len(manifest))
	for _, command := range manifest {
		if category != "" && !strings.EqualFold(command.Category, category) {
			continue
		}
		if mutatingOnly && !command.Mutating {
			continue
		}
		if query != "" && !commandManifestEntryMatches(command, query) {
			continue
		}
		filtered = append(filtered, command)
	}
	summary := map[string]int{"total": len(filtered)}
	for _, command := range filtered {
		summary["category_"+command.Category]++
		for _, commandSurface := range strings.Split(command.Surface, ",") {
			commandSurface = strings.TrimSpace(commandSurface)
			if commandSurface != "" {
				summary["surface_"+commandSurface]++
			}
		}
		if command.Mutating {
			summary["mutating"]++
		} else {
			summary["read_only"]++
		}
		if command.RequiresConfirmation {
			summary["requires_confirmation"]++
		}
		if command.TopologySensitive {
			summary["topology_sensitive"]++
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(commandManifestResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Persisted:   false,
		Summary:     summary,
		Commands:    filtered,
	})
}

func (b *Broker) handleCommandManifestDrift(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	manual, err := os.ReadFile("docs/MANUAL.md")
	if err != nil {
		http.Error(w, "manual not available", http.StatusInternalServerError)
		return
	}
	surface := commands.NormalizeManifestSurface(r.URL.Query().Get("surface"))
	if surface == "" {
		http.Error(w, "unknown surface", http.StatusBadRequest)
		return
	}
	manifest := commands.FilterCommandManifest(commands.BuildCommandManifest(), surface)
	payload := buildCommandManifestDrift(string(manual), manifest, time.Now().UTC())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func buildCommandManifestDrift(manual string, commands []commandManifestEntry, now time.Time) commandManifestDriftResponse {
	manifestByName := make(map[string]commandManifestEntry, len(commands))
	for _, command := range commands {
		manifestByName[strings.TrimSpace(command.Name)] = command
	}
	manualNames := extractManualSlashCommands(manual)
	manualSet := make(map[string]struct{}, len(manualNames))
	for _, name := range manualNames {
		manualSet[name] = struct{}{}
	}
	items := make([]commandManifestDriftItem, 0)
	for _, command := range commands {
		name := strings.TrimSpace(command.Name)
		if _, ok := manualSet[name]; !ok {
			items = append(items, commandManifestDriftItem{
				Command:  name,
				Category: command.Category,
				Severity: "warning",
				Kind:     "manifest_missing_manual",
				Summary:  "Command is exposed in the manifest but is not mentioned in the manual slash command section.",
			})
		}
	}
	for _, name := range manualNames {
		if _, ok := manifestByName[name]; !ok {
			items = append(items, commandManifestDriftItem{
				Command:  name,
				Severity: "warning",
				Kind:     "manual_missing_manifest",
				Summary:  "Manual mentions a slash command that is not present in the command manifest.",
			})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		return items[i].Command < items[j].Command
	})
	summary := map[string]int{"total": len(items)}
	status := "ok"
	for _, item := range items {
		summary["kind_"+item.Kind]++
		summary["severity_"+item.Severity]++
		if item.Severity == "warning" {
			status = "warning"
		}
	}
	return commandManifestDriftResponse{
		GeneratedAt: now.Format(time.RFC3339),
		Persisted:   false,
		Status:      status,
		Summary:     summary,
		Items:       items,
	}
}

func extractManualSlashCommands(manual string) []string {
	section := manual
	if idx := strings.Index(section, "## 10. Slash Commands"); idx >= 0 {
		section = section[idx:]
		if next := strings.Index(section[len("## 10. Slash Commands"):], "\n## "); next >= 0 {
			section = section[:len("## 10. Slash Commands")+next]
		}
	}
	re := regexp.MustCompile("`(/[A-Za-z0-9][A-Za-z0-9_-]*)(?:\\s[^`]*)?`")
	matches := re.FindAllStringSubmatch(section, -1)
	out := make([]string, 0, len(matches))
	seen := map[string]struct{}{}
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := strings.TrimSpace(match[1])
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func commandManifestEntryMatches(command commandManifestEntry, query string) bool {
	haystack := strings.ToLower(strings.Join(append([]string{
		command.Name,
		command.Category,
		command.Description,
		command.Surface,
		command.Route,
		command.Method,
		command.Args,
	}, command.Signals...), " "))
	return strings.Contains(haystack, query)
}

type executionEnvironmentPreviewResponse struct {
	GeneratedAt  string                        `json:"generated_at"`
	Persisted    bool                          `json:"persisted"`
	Summary      map[string]int                `json:"summary"`
	Environments []executionEnvironmentPreview `json:"environments"`
}

type executionEnvironmentPreview struct {
	ID               string                            `json:"id"`
	Kind             string                            `json:"kind"`
	Status           string                            `json:"status"`
	Readiness        string                            `json:"readiness"`
	Summary          string                            `json:"summary,omitempty"`
	Channels         []string                          `json:"channels,omitempty"`
	TaskIDs          []string                          `json:"task_ids,omitempty"`
	WorkspaceCount   int                               `json:"workspace_count,omitempty"`
	ActiveTaskCount  int                               `json:"active_task_count,omitempty"`
	RequiredPolicies []string                          `json:"required_policies,omitempty"`
	MissingPolicies  []string                          `json:"missing_policies,omitempty"`
	PolicyChecks     []executionEnvironmentPolicyCheck `json:"policy_checks,omitempty"`
	Signals          []string                          `json:"signals,omitempty"`
	NextStep         string                            `json:"next_step,omitempty"`
	RequiresReview   bool                              `json:"requires_review,omitempty"`
}

type executionEnvironmentPolicyCheck struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Summary  string `json:"summary"`
	NextStep string `json:"next_step,omitempty"`
}

func (b *Broker) handleExecutionEnvironmentsPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	allChannels := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all_channels")), "true")
	if channel == "" && !allChannels {
		channel = "general"
	}
	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind")))
	b.mu.RLock()
	payload := b.buildExecutionEnvironmentsPreviewLocked(viewer, channel, allChannels)
	b.mu.RUnlock()
	if kind != "" {
		filtered := payload.Environments[:0]
		for _, env := range payload.Environments {
			if strings.EqualFold(env.Kind, kind) {
				filtered = append(filtered, env)
			}
		}
		payload.Environments = filtered
		payload.Summary = executionEnvironmentSummary(filtered)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) buildExecutionEnvironmentsPreviewLocked(viewer, channel string, allChannels bool) executionEnvironmentPreviewResponse {
	workspaces := b.buildWorkspacesInventoryLocked(viewer, channel, allChannels).Workspaces
	buckets := map[string]*executionEnvironmentPreview{}
	ensure := func(kind string) *executionEnvironmentPreview {
		kind = strings.ToLower(strings.TrimSpace(kind))
		if kind == "" {
			kind = "office"
		}
		env := buckets[kind]
		if env == nil {
			env = &executionEnvironmentPreview{ID: "env:" + kind, Kind: kind, Status: "available", Readiness: "ready"}
			buckets[kind] = env
		}
		return env
	}
	for _, task := range b.tasks {
		taskChannel := normalizeChannelSlug(task.Channel)
		if !intakeChannelVisible(b, viewer, taskChannel, channel, allChannels) {
			continue
		}
		mode := strings.ToLower(strings.TrimSpace(firstNonEmpty(task.ExecutionMode, "office")))
		env := ensure(mode)
		env.Channels = appendUnique(env.Channels, taskChannel)
		env.TaskIDs = appendUnique(env.TaskIDs, task.ID)
		if !taskIsTerminal(&task) {
			env.ActiveTaskCount++
		}
		if mode == "external_workspace" {
			env.RequiresReview = true
			env.Signals = appendUnique(env.Signals, "explicit_workspace")
		}
		if mode == "live_external" {
			env.RequiresReview = true
			env.Signals = appendUnique(env.Signals, "external_system")
		}
	}
	for _, ws := range workspaces {
		kind := strings.ToLower(strings.TrimSpace(firstNonEmpty(ws.Kind, "workspace")))
		env := ensure(kind)
		env.WorkspaceCount++
		env.Channels = appendUnique(env.Channels, ws.Channel)
		env.TaskIDs = compactStringList(append(env.TaskIDs, ws.TaskIDs...))
		if ws.ActiveTaskCount > 0 {
			env.ActiveTaskCount += ws.ActiveTaskCount
		}
		if !ws.Healthy {
			env.Status = "degraded"
			env.Readiness = "review"
			env.RequiresReview = true
			env.Signals = appendUnique(env.Signals, "workspace_degraded")
		}
		if ws.GitDirtyCount > 0 {
			env.Signals = appendUnique(env.Signals, "dirty_workspace")
		}
	}
	for _, future := range futureExecutionEnvironmentAdapters() {
		if _, ok := buckets[future.Kind]; !ok {
			copyFuture := future
			buckets[future.Kind] = &copyFuture
		}
	}
	out := make([]executionEnvironmentPreview, 0, len(buckets))
	for _, env := range buckets {
		env.Channels = compactStringList(env.Channels)
		env.TaskIDs = compactStringList(env.TaskIDs)
		env.Signals = compactStringList(env.Signals)
		if env.Summary == "" {
			env.Summary = executionEnvironmentSummaryText(*env)
		}
		out = append(out, *env)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Readiness != out[j].Readiness {
			return executionEnvironmentReadinessRank(out[i].Readiness) > executionEnvironmentReadinessRank(out[j].Readiness)
		}
		return out[i].Kind < out[j].Kind
	})
	return executionEnvironmentPreviewResponse{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		Persisted:    false,
		Summary:      executionEnvironmentSummary(out),
		Environments: out,
	}
}

func futureExecutionEnvironmentAdapters() []executionEnvironmentPreview {
	docker := futureExecutionEnvironmentAdapter("docker", []string{
		"docker_binary",
		"workspace_policy",
		"secret_policy",
		"network_policy",
		"cleanup_policy",
		"audit_policy",
		"approval_policy",
	})
	docker.Signals = appendUnique(docker.Signals, "future_adapter")
	docker.Summary = "Docker is a blocked future adapter. This preview only reports local readiness and missing governance policies."
	docker.NextStep = "Define workspace, secret, network, cleanup, audit, and approval policies before any Docker execution path is enabled."

	ssh := futureExecutionEnvironmentAdapter("ssh", []string{
		"ssh_binary",
		"host_policy",
		"key_policy",
		"workspace_policy",
		"secret_policy",
		"network_policy",
		"cleanup_policy",
		"audit_policy",
		"approval_policy",
	})
	ssh.Signals = appendUnique(appendUnique(ssh.Signals, "future_adapter"), "external_host")
	ssh.Summary = "SSH is a blocked future adapter. This preview only reports local readiness and missing governance policies."
	ssh.NextStep = "Define host allowlists, key policy, workspace, secret, network, cleanup, audit, and approval policies before any SSH execution path is enabled."
	return []executionEnvironmentPreview{docker, ssh}
}

func futureExecutionEnvironmentAdapter(kind string, policies []string) executionEnvironmentPreview {
	env := executionEnvironmentPreview{
		ID:               "env:" + kind,
		Kind:             kind,
		Status:           "planned",
		Readiness:        "blocked",
		RequiresReview:   true,
		RequiredPolicies: compactStringList(policies),
		Signals:          []string{"requires_policy"},
	}
	for _, policy := range policies {
		check := executionEnvironmentPolicyCheck{ID: policy}
		switch policy {
		case "docker_binary":
			check = executionEnvironmentBinaryPolicyCheck("docker", "Install Docker or keep the adapter disabled.")
		case "ssh_binary":
			check = executionEnvironmentBinaryPolicyCheck("ssh", "Install OpenSSH client or keep the adapter disabled.")
		default:
			check.Status = "missing"
			check.Summary = "Governance policy is not declared in this local office."
			check.NextStep = "Add an explicit, reviewed policy before enabling this adapter."
		}
		env.PolicyChecks = append(env.PolicyChecks, check)
		if check.Status != "ok" {
			env.MissingPolicies = appendUnique(env.MissingPolicies, policy)
		}
		if check.Status == "missing" {
			env.Signals = appendUnique(env.Signals, "missing_"+policy)
		}
	}
	env.MissingPolicies = compactStringList(env.MissingPolicies)
	env.PolicyChecks = compactExecutionEnvironmentPolicyChecks(env.PolicyChecks)
	return env
}

func executionEnvironmentBinaryPolicyCheck(binary, nextStep string) executionEnvironmentPolicyCheck {
	check := executionEnvironmentPolicyCheck{ID: binary + "_binary"}
	if _, err := exec.LookPath(binary); err != nil {
		check.Status = "missing"
		check.Summary = binary + " binary was not found on PATH."
		check.NextStep = nextStep
		return check
	}
	check.Status = "ok"
	check.Summary = binary + " binary is available on PATH. Adapter execution remains blocked by governance policy."
	return check
}

func compactExecutionEnvironmentPolicyChecks(checks []executionEnvironmentPolicyCheck) []executionEnvironmentPolicyCheck {
	out := make([]executionEnvironmentPolicyCheck, 0, len(checks))
	seen := map[string]struct{}{}
	for _, check := range checks {
		check.ID = strings.TrimSpace(check.ID)
		check.Status = strings.TrimSpace(check.Status)
		check.Summary = strings.TrimSpace(check.Summary)
		check.NextStep = strings.TrimSpace(check.NextStep)
		if check.ID == "" {
			continue
		}
		if _, ok := seen[check.ID]; ok {
			continue
		}
		seen[check.ID] = struct{}{}
		out = append(out, check)
	}
	return out
}

func executionEnvironmentSummary(environments []executionEnvironmentPreview) map[string]int {
	summary := map[string]int{"total": len(environments)}
	for _, env := range environments {
		summary["kind_"+env.Kind]++
		summary["readiness_"+env.Readiness]++
		if len(env.MissingPolicies) > 0 {
			summary["missing_policy"] += len(env.MissingPolicies)
		}
		if env.RequiresReview {
			summary["requires_review"]++
		}
	}
	return summary
}

func executionEnvironmentSummaryText(env executionEnvironmentPreview) string {
	switch env.Kind {
	case "office":
		return "Broker-mediated office execution without a dedicated filesystem workspace."
	case "local_worktree":
		return "Managed local worktree execution remains the default governed workspace path."
	case "external_workspace":
		return "Explicit local workspace path supplied by a task; review ownership and health before reuse."
	case "live_external":
		return "Connected external-system work; use only through governed adapters and task contracts."
	default:
		return "Execution environment preview."
	}
}

func executionEnvironmentReadinessRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "blocked":
		return 4
	case "review":
		return 3
	case "warning":
		return 2
	default:
		return 1
	}
}

type skillFilePreviewResponse struct {
	GeneratedAt string             `json:"generated_at"`
	Persisted   bool               `json:"persisted"`
	SkillName   string             `json:"skill_name"`
	Channel     string             `json:"channel,omitempty"`
	Selected    string             `json:"selected,omitempty"`
	Files       []skillFilePreview `json:"files"`
	Summary     map[string]int     `json:"summary"`
}

type skillFilePreview struct {
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Title       string   `json:"title,omitempty"`
	Summary     string   `json:"summary,omitempty"`
	Size        int      `json:"size"`
	Available   bool     `json:"available"`
	Content     string   `json:"content,omitempty"`
	RiskSignals []string `json:"risk_signals,omitempty"`
}

func (b *Broker) handleSkillFilesPreview(w http.ResponseWriter, r *http.Request, skillName string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	selected := strings.TrimSpace(r.URL.Query().Get("file"))
	includeContent := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_content")), "true")

	b.mu.RLock()
	skill := b.findSkillByNameLocked(skillName)
	if skill == nil {
		b.mu.RUnlock()
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}
	skillCopy := *skill
	b.mu.RUnlock()

	skillChannel := normalizeChannelSlug(skillCopy.Channel)
	if skillChannel == "" {
		skillChannel = "general"
	}
	if channel != "" && !skillVisibleInChannel(skillChannel, channel) {
		http.Error(w, "skill not visible in channel", http.StatusForbidden)
		return
	}
	if viewer != "" && !b.canAccessChannel(viewer, skillChannel) {
		http.Error(w, "channel access denied", http.StatusForbidden)
		return
	}
	files := buildSkillFilePreviews(skillCopy, selected, includeContent)
	if selected != "" {
		filtered := files[:0]
		for _, file := range files {
			if strings.EqualFold(file.Name, selected) {
				filtered = append(filtered, file)
			}
		}
		files = filtered
		if len(files) == 0 {
			http.Error(w, "skill file not found", http.StatusNotFound)
			return
		}
	}
	summary := map[string]int{"total": len(files)}
	for _, file := range files {
		if file.Available {
			summary["available"]++
		}
		if file.Content != "" {
			summary["with_content"]++
		}
		for _, signal := range file.RiskSignals {
			summary["risk_"+signal]++
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(skillFilePreviewResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Persisted:   false,
		SkillName:   skillCopy.Name,
		Channel:     skillChannel,
		Selected:    selected,
		Files:       files,
		Summary:     summary,
	})
}

func buildSkillFilePreviews(skill teamSkill, selected string, includeContent bool) []skillFilePreview {
	type virtualFile struct {
		name    string
		kind    string
		title   string
		summary string
		content string
	}
	metadata, _ := json.MarshalIndent(map[string]any{
		"name":          skill.Name,
		"title":         skill.Title,
		"description":   skill.Description,
		"channel":       skill.Channel,
		"status":        skill.Status,
		"capabilities":  normalizeSkillCapabilities(skill.Capabilities),
		"source_type":   skill.SourceType,
		"source_ref":    skill.SourceRef,
		"source_hash":   skill.SourceHash,
		"scan_status":   skill.ScanStatus,
		"health_status": skill.HealthStatus,
		"tags":          compactStringList(skill.Tags),
	}, "", "  ")
	candidates := []virtualFile{
		{name: "metadata.json", kind: "metadata", title: "Skill metadata", summary: "Provenance, status, trust and capability metadata.", content: string(metadata)},
		{name: "content.md", kind: "instruction", title: "Skill instructions", summary: truncateSummary(skill.Content, 180), content: skill.Content},
		{name: "workflow.json", kind: "workflow", title: "Workflow definition", summary: "Structured workflow definition.", content: skill.WorkflowDefinition},
		{name: "trigger.txt", kind: "trigger", title: "Trigger", summary: truncateSummary(skill.Trigger, 180), content: skill.Trigger},
		{name: "scan.txt", kind: "scan", title: "Scan summary", summary: truncateSummary(skill.ScanSummary, 180), content: skill.ScanSummary},
	}
	files := make([]skillFilePreview, 0, len(candidates))
	for _, candidate := range candidates {
		content := strings.TrimSpace(candidate.content)
		file := skillFilePreview{
			Name:      candidate.name,
			Kind:      candidate.kind,
			Title:     candidate.title,
			Summary:   candidate.summary,
			Size:      len(content),
			Available: content != "",
		}
		if file.Summary == "" && file.Available {
			file.Summary = truncateSummary(content, 180)
		}
		if contentLooksSecretBearing(content) {
			file.RiskSignals = appendUnique(file.RiskSignals, "secret_like_content")
		}
		if file.Available && (includeContent || strings.EqualFold(selected, candidate.name)) {
			file.Content = content
		}
		files = append(files, file)
	}
	return files
}

func (b *Broker) canAccessChannel(viewer, channel string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.canAccessChannelLocked(viewer, channel)
}
