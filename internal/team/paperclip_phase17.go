package team

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type pluginRuntimeResponse struct {
	GeneratedAt string              `json:"generated_at"`
	Summary     map[string]int      `json:"summary"`
	Plugins     []pluginRuntimeItem `json:"plugins"`
	Jobs        []pluginRuntimeJob  `json:"jobs,omitempty"`
	Runs        []pluginRuntimeRun  `json:"runs,omitempty"`
}

type pluginRuntimeItem struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Name         string   `json:"name"`
	Status       string   `json:"status,omitempty"`
	HealthStatus string   `json:"health_status,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	ConfigRef    string   `json:"config_ref,omitempty"`
	Source       string   `json:"source,omitempty"`
	UpdatedAt    string   `json:"updated_at,omitempty"`
}

type pluginSandboxPreviewResponse struct {
	GeneratedAt string                          `json:"generated_at"`
	Persisted   bool                            `json:"persisted"`
	Status      string                          `json:"status"`
	Summary     map[string]int                  `json:"summary"`
	Candidates  []pluginSandboxPreviewCandidate `json:"candidates"`
}

type pluginSandboxPreviewCandidate struct {
	ID                string   `json:"id"`
	Kind              string   `json:"kind"`
	Name              string   `json:"name"`
	WorkerClass       string   `json:"worker_class,omitempty"`
	ManifestID        string   `json:"manifest_id,omitempty"`
	ManifestSignature string   `json:"manifest_signature,omitempty"`
	RuntimeStatus     string   `json:"runtime_status,omitempty"`
	SandboxStatus     string   `json:"sandbox_status"`
	Capabilities      []string `json:"capabilities,omitempty"`
	RequiredPolicies  []string `json:"required_policies,omitempty"`
	MissingPolicies   []string `json:"missing_policies,omitempty"`
	FilesystemScope   []string `json:"filesystem_scope,omitempty"`
	NetworkPolicy     string   `json:"network_policy,omitempty"`
	SecretRefs        []string `json:"secret_refs,omitempty"`
	HealthCheck       string   `json:"health_check,omitempty"`
	ConfigRef         string   `json:"config_ref,omitempty"`
	RiskSignals       []string `json:"risk_signals,omitempty"`
	NextStep          string   `json:"next_step,omitempty"`
}

type pluginRuntimeJob struct {
	ID             string `json:"id"`
	PluginID       string `json:"plugin_id,omitempty"`
	Kind           string `json:"kind,omitempty"`
	Status         string `json:"status,omitempty"`
	Channel        string `json:"channel,omitempty"`
	Schedule       string `json:"schedule,omitempty"`
	NextRun        string `json:"next_run,omitempty"`
	LastStartedAt  string `json:"last_started_at,omitempty"`
	LastFinishedAt string `json:"last_finished_at,omitempty"`
	LastSummary    string `json:"last_summary,omitempty"`
}

type pluginRuntimeRun struct {
	ID        string `json:"id"`
	PluginID  string `json:"plugin_id,omitempty"`
	Action    string `json:"action"`
	Status    string `json:"status"`
	Actor     string `json:"actor,omitempty"`
	ActorType string `json:"actor_type,omitempty"`
	Summary   string `json:"summary,omitempty"`
	RelatedID string `json:"related_id,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

func (b *Broker) handlePluginRuntime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.mu.RLock()
	payload := b.buildPluginRuntimeLocked()
	b.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) handlePluginSandboxPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.mu.RLock()
	payload := b.buildPluginSandboxPreviewLocked()
	b.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) buildPluginRuntimeLocked() pluginRuntimeResponse {
	plugins := make([]pluginRuntimeItem, 0)
	for _, adapter := range mergedOfficeAdapters(b.adapters) {
		plugins = append(plugins, pluginRuntimeItem{
			ID:           "adapter:" + adapter.ID,
			Kind:         firstNonEmpty(adapter.Kind, "adapter"),
			Name:         firstNonEmpty(adapter.Name, adapter.ID),
			Status:       adapter.Status,
			HealthStatus: adapter.HealthStatus,
			Capabilities: compactStringList(adapter.Capabilities),
			ConfigRef:    adapter.ConfigRef,
			Source:       firstNonEmpty(adapter.Source, "adapter"),
			UpdatedAt:    adapter.UpdatedAt,
		})
	}
	for _, skill := range b.skills {
		if skill.Status == "archived" {
			continue
		}
		pluginID := firstNonEmpty(skill.PluginID, "legacy-"+skillSlug(skill.Name))
		plugins = append(plugins, pluginRuntimeItem{
			ID:           "skill:" + pluginID,
			Kind:         firstNonEmpty(skill.PluginKind, "skill"),
			Name:         firstNonEmpty(skill.Title, skill.Name),
			Status:       skill.Status,
			HealthStatus: firstNonEmpty(skill.HealthStatus, "unknown"),
			Capabilities: compactStringList(skill.Capabilities),
			Source:       "skill",
			UpdatedAt:    firstNonEmpty(skill.UpdatedAt, skill.CreatedAt),
		})
	}
	jobs := make([]pluginRuntimeJob, 0, len(b.scheduler))
	for _, job := range b.scheduler {
		jobs = append(jobs, pluginRuntimeJob{
			ID:             firstNonEmpty(job.Slug, job.TargetID, job.WorkflowKey),
			PluginID:       pluginIDForSchedulerJob(job),
			Kind:           job.Kind,
			Status:         job.Status,
			Channel:        normalizeChannelSlug(job.Channel),
			Schedule:       firstNonEmpty(job.ScheduleExpr, fmt.Sprintf("%dm", job.IntervalMinutes)),
			NextRun:        firstNonEmpty(job.NextRun, job.DueAt),
			LastStartedAt:  job.LastStartedAt,
			LastFinishedAt: job.LastFinishedAt,
			LastSummary:    job.LastSummary,
		})
	}
	runs := make([]pluginRuntimeRun, 0)
	for _, action := range b.actions {
		if !actionLooksPluginRuntimeRelated(action) {
			continue
		}
		runs = append(runs, pluginRuntimeRun{
			ID:        action.ID,
			PluginID:  pluginIDForAction(action),
			Action:    action.Kind,
			Status:    actionStatusFromKind(action.Kind),
			Actor:     action.Actor,
			ActorType: actorTypeForActivity(action.Actor, action.Source, action.Kind),
			Summary:   action.Summary,
			RelatedID: action.RelatedID,
			CreatedAt: action.CreatedAt,
		})
	}
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].ID < plugins[j].ID })
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].ID < jobs[j].ID })
	sort.Slice(runs, func(i, j int) bool { return studioTimestampAfter(runs[i].CreatedAt, runs[j].CreatedAt) })
	if len(runs) > 40 {
		runs = runs[:40]
	}
	summary := map[string]int{"plugins": len(plugins), "jobs": len(jobs), "runs": len(runs)}
	for _, item := range plugins {
		summary[item.Kind]++
	}
	return pluginRuntimeResponse{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Summary: summary, Plugins: plugins, Jobs: jobs, Runs: runs}
}

func (b *Broker) buildPluginSandboxPreviewLocked() pluginSandboxPreviewResponse {
	candidates := make([]pluginSandboxPreviewCandidate, 0)
	candidates = append(candidates, pluginSandboxNoopWorkerCandidate())
	for _, adapter := range mergedOfficeAdapters(b.adapters) {
		configCheck := checkAdapterConfigRef(adapter)
		candidate := pluginSandboxPreviewCandidate{
			ID:            "adapter:" + adapter.ID,
			Kind:          firstNonEmpty(adapter.Kind, "adapter"),
			Name:          firstNonEmpty(adapter.Name, adapter.ID),
			RuntimeStatus: firstNonEmpty(adapter.HealthStatus, adapter.Status),
			Capabilities:  compactStringList(adapter.Capabilities),
			ConfigRef:     configCheck.ConfigRef,
		}
		candidate.RequiredPolicies = pluginSandboxRequiredPolicies(candidate.Kind, candidate.Capabilities)
		candidate.MissingPolicies = pluginSandboxMissingPolicies(candidate, true, configCheck)
		candidate.RiskSignals = pluginSandboxRiskSignals(candidate, configCheck)
		candidate.SandboxStatus, candidate.NextStep = pluginSandboxStatus(candidate)
		candidates = append(candidates, candidate)
	}
	for _, skill := range b.skills {
		if skill.Status == "archived" {
			continue
		}
		pluginID := firstNonEmpty(skill.PluginID, "legacy-"+skillSlug(skill.Name))
		candidate := pluginSandboxPreviewCandidate{
			ID:            "skill:" + pluginID,
			Kind:          firstNonEmpty(skill.PluginKind, "skill"),
			Name:          firstNonEmpty(skill.Title, skill.Name),
			RuntimeStatus: firstNonEmpty(skill.HealthStatus, skill.Status, "unknown"),
			Capabilities:  compactStringList(skill.Capabilities),
		}
		candidate.RequiredPolicies = pluginSandboxRequiredPolicies(candidate.Kind, candidate.Capabilities)
		candidate.MissingPolicies = pluginSandboxMissingPolicies(candidate, strings.TrimSpace(skill.PluginID) != "", adapterConfigCheck{Status: "ok"})
		candidate.RiskSignals = pluginSandboxRiskSignals(candidate, adapterConfigCheck{Status: "ok"})
		candidate.SandboxStatus, candidate.NextStep = pluginSandboxStatus(candidate)
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	summary := map[string]int{"total": len(candidates)}
	status := "ok"
	for _, candidate := range candidates {
		summary[candidate.SandboxStatus]++
		if candidate.WorkerClass != "" {
			summary["workers"]++
		}
		if candidate.SandboxStatus == "blocked" {
			status = "blocked"
		} else if candidate.SandboxStatus == "review" && status == "ok" {
			status = "review"
		}
	}
	return pluginSandboxPreviewResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Persisted:   false,
		Status:      status,
		Summary:     summary,
		Candidates:  candidates,
	}
}

func pluginSandboxNoopWorkerCandidate() pluginSandboxPreviewCandidate {
	manifest := map[string]any{
		"id":               "builtin.noop-health-worker",
		"worker_class":     "noop_health",
		"capabilities":     []string{"health.report"},
		"filesystem_scope": []string{"none"},
		"network_policy":   "none",
		"secret_refs":      []string{},
		"health_check":     "static",
	}
	raw, _ := json.Marshal(manifest)
	digest := sha256.Sum256(raw)
	return pluginSandboxPreviewCandidate{
		ID:                "worker:noop-health",
		Kind:              "worker",
		Name:              "No-op health worker",
		WorkerClass:       "noop_health",
		ManifestID:        "builtin.noop-health-worker",
		ManifestSignature: fmt.Sprintf("sha256:%x", digest),
		RuntimeStatus:     "ok",
		SandboxStatus:     "ready",
		Capabilities:      []string{"health.report"},
		RequiredPolicies:  []string{"manifest", "capabilities", "health_check", "filesystem_scope", "network_policy", "secret_refs"},
		FilesystemScope:   []string{"none"},
		NetworkPolicy:     "none",
		SecretRefs:        []string{},
		HealthCheck:       "static",
		NextStep:          "Health-only worker is ready for sandbox probes; it cannot execute plugin actions, shell commands, network calls, or filesystem writes.",
	}
}

func pluginSandboxRequiredPolicies(kind string, capabilities []string) []string {
	policies := []string{"manifest", "capabilities", "health_check", "filesystem_scope", "network_policy"}
	if pluginSandboxNeedsSecretPolicy(kind, capabilities) {
		policies = append(policies, "secret_refs")
	}
	return compactStringList(policies)
}

func pluginSandboxNeedsSecretPolicy(kind string, capabilities []string) bool {
	text := strings.ToLower(kind + " " + strings.Join(capabilities, " "))
	for _, needle := range []string{"integration", "adapter", "secret", "token", "issue.", "review.", "external", "network", "publish"} {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func pluginSandboxMissingPolicies(candidate pluginSandboxPreviewCandidate, hasManifest bool, configCheck adapterConfigCheck) []string {
	missing := make([]string, 0)
	if !hasManifest {
		missing = append(missing, "manifest")
	}
	if len(candidate.Capabilities) == 0 {
		missing = append(missing, "capabilities")
	}
	if status := strings.ToLower(strings.TrimSpace(candidate.RuntimeStatus)); status == "" || status == "unknown" || status == "missing" || status == "failed" || status == "blocked" {
		missing = append(missing, "health_check")
	}
	missing = append(missing, "filesystem_scope", "network_policy")
	if stringSliceContains(candidate.RequiredPolicies, "secret_refs") && configCheck.Status != "ok" {
		missing = append(missing, "secret_refs")
	}
	return compactStringList(missing)
}

func pluginSandboxRiskSignals(candidate pluginSandboxPreviewCandidate, configCheck adapterConfigCheck) []string {
	signals := make([]string, 0)
	if len(candidate.MissingPolicies) > 0 {
		signals = append(signals, "missing_policy")
	}
	if configCheck.Status == "fail" {
		signals = append(signals, "config_ref_blocked")
	} else if configCheck.Status == "warn" {
		signals = append(signals, "config_ref_review")
	}
	if stringSliceContains(candidate.MissingPolicies, "network_policy") && pluginSandboxNeedsSecretPolicy(candidate.Kind, candidate.Capabilities) {
		signals = append(signals, "external_access_unscoped")
	}
	if stringSliceContains(candidate.MissingPolicies, "filesystem_scope") {
		signals = append(signals, "filesystem_unscoped")
	}
	return compactStringList(signals)
}

func pluginSandboxStatus(candidate pluginSandboxPreviewCandidate) (string, string) {
	if len(candidate.MissingPolicies) == 0 {
		return "ready", "Candidate has the minimum manifest, health, capability, filesystem, network, and secret-reference policies for a future sandbox runner."
	}
	blockers := stringSet(candidate.MissingPolicies)
	if setContains(blockers, "filesystem_scope") || setContains(blockers, "network_policy") || setContains(blockers, "secret_refs") {
		return "blocked", "Keep execution disabled until the missing sandbox policies are declared and reviewed."
	}
	return "review", "Review the missing metadata before this plugin can be considered for sandbox execution."
}

func pluginIDForSchedulerJob(job schedulerJob) string {
	if names := schedulerJobSkillNames(job); len(names) > 0 {
		return "skill:" + skillSlug(names[0])
	}
	if strings.TrimSpace(job.Provider) != "" {
		return "adapter:" + normalizeExecutionKey(job.Provider)
	}
	if strings.TrimSpace(job.WorkflowKey) != "" {
		return "workflow:" + normalizeExecutionKey(job.WorkflowKey)
	}
	return ""
}

func actionLooksPluginRuntimeRelated(action officeActionLog) bool {
	text := strings.ToLower(strings.Join([]string{action.Kind, action.Source, action.Summary}, " "))
	return strings.Contains(text, "skill") ||
		strings.Contains(text, "adapter") ||
		strings.Contains(text, "workflow") ||
		strings.Contains(text, "plugin") ||
		strings.Contains(text, "external")
}

func pluginIDForAction(action officeActionLog) string {
	source := normalizeExecutionKey(action.Source)
	if source != "" {
		source = strings.TrimPrefix(source, "adapter-")
		return "adapter:" + source
	}
	if strings.Contains(strings.ToLower(action.Kind), "skill") {
		return "skill:" + normalizeExecutionKey(action.RelatedID)
	}
	return ""
}

func actionStatusFromKind(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch {
	case strings.Contains(kind, "failed"), strings.Contains(kind, "error"):
		return "failed"
	case strings.Contains(kind, "requested"), strings.Contains(kind, "preview"):
		return "queued"
	default:
		return "succeeded"
	}
}

type adapterConfigCheckResponse struct {
	GeneratedAt string               `json:"generated_at"`
	Status      string               `json:"status"`
	Summary     map[string]int       `json:"summary"`
	Checks      []adapterConfigCheck `json:"checks"`
}

type adapterConfigCheck struct {
	AdapterID string `json:"adapter_id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	ConfigRef string `json:"config_ref,omitempty"`
	Summary   string `json:"summary"`
	NextStep  string `json:"next_step,omitempty"`
}

func (b *Broker) handleAdapterConfigChecks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.mu.Lock()
	adapters := mergedOfficeAdapters(b.adapters)
	b.mu.Unlock()
	payload := buildAdapterConfigChecks(adapters)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func buildAdapterConfigChecks(adapters []officeAdapter) adapterConfigCheckResponse {
	checks := make([]adapterConfigCheck, 0, len(adapters))
	for _, adapter := range adapters {
		checks = append(checks, checkAdapterConfigRef(adapter))
	}
	status := "ok"
	summary := map[string]int{"total": len(checks)}
	for _, check := range checks {
		summary[check.Status]++
		if check.Status == "fail" {
			status = "blocked"
		} else if check.Status == "warn" && status == "ok" {
			status = "review"
		}
	}
	return adapterConfigCheckResponse{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Status: status, Summary: summary, Checks: checks}
}

func checkAdapterConfigRef(adapter officeAdapter) adapterConfigCheck {
	ref := strings.TrimSpace(adapter.ConfigRef)
	check := adapterConfigCheck{AdapterID: adapter.ID, Name: firstNonEmpty(adapter.Name, adapter.ID), ConfigRef: scrubConfigRefForOutput(ref)}
	if ref == "" {
		switch adapter.ID {
		case "local-broker", "fresh-runner", "scoped-mcp", "learning-registry":
			check.Status = "ok"
			check.Summary = "No secret-backed config is required."
		default:
			check.Status = "warn"
			check.Summary = "No config_ref is declared."
			check.NextStep = "Use secret:, env:, or config: references instead of raw secret values."
		}
		return check
	}
	lower := strings.ToLower(ref)
	switch {
	case strings.HasPrefix(lower, "secret:"), strings.HasPrefix(lower, "config:"):
		check.Status = "ok"
		check.Summary = "Config uses a reference instead of a raw value."
	case strings.HasPrefix(lower, "env:"):
		name := strings.TrimSpace(strings.TrimPrefix(ref, "env:"))
		if name == "" {
			check.Status = "fail"
			check.Summary = "env: config reference is empty."
			check.NextStep = "Set config_ref to env:NAME."
		} else if strings.TrimSpace(os.Getenv(name)) == "" {
			check.Status = "warn"
			check.Summary = "Environment config reference is declared but not set."
			check.NextStep = "Set the referenced environment variable before using this adapter."
		} else {
			check.Status = "ok"
			check.Summary = "Environment config reference is present."
		}
	case contentLooksSecretBearing(ref):
		check.Status = "fail"
		check.Summary = "config_ref looks like a raw secret value."
		check.NextStep = "Move the value into the secret store or environment and keep only a reference here."
	default:
		check.Status = "warn"
		check.Summary = "Config reference should use an explicit secret:, env:, or config: scheme."
		check.NextStep = "Normalize this config_ref before enabling automated adapter actions."
	}
	return check
}

func scrubConfigRefForOutput(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	lower := strings.ToLower(ref)
	for _, prefix := range []string{"secret:", "env:", "config:"} {
		if strings.HasPrefix(lower, prefix) {
			return ref
		}
	}
	return "[redacted]"
}

type adapterActionRequest struct {
	AdapterID    string `json:"adapter_id"`
	Action       string `json:"action"`
	Actor        string `json:"actor,omitempty"`
	Reason       string `json:"reason,omitempty"`
	Confirm      bool   `json:"confirm,omitempty"`
	Confirmation string `json:"confirmation,omitempty"`
}

type adapterActionResponse struct {
	Persisted            bool                     `json:"persisted"`
	AdapterID            string                   `json:"adapter_id"`
	Action               string                   `json:"action"`
	Status               string                   `json:"status"`
	RequiredConfirmation string                   `json:"required_confirmation,omitempty"`
	RequiredCapabilities []string                 `json:"required_capabilities,omitempty"`
	MissingCapabilities  []string                 `json:"missing_capabilities,omitempty"`
	AuditActionID        string                   `json:"audit_action_id,omitempty"`
	Message              string                   `json:"message,omitempty"`
	ConfigCheck          *adapterConfigCheck      `json:"config_check,omitempty"`
	EnvironmentCheck     *adapterEnvironmentCheck `json:"environment_check,omitempty"`
}

func (b *Broker) handleAdapterActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req adapterActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	req.AdapterID = normalizeExecutionKey(req.AdapterID)
	req.Action = normalizeExecutionKey(req.Action)
	req.Actor = firstNonEmpty(strings.TrimSpace(req.Actor), "human")
	req.Reason = strings.TrimSpace(req.Reason)
	b.mu.Lock()
	payload, err := b.applyAdapterActionLocked(req)
	b.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) applyAdapterActionLocked(req adapterActionRequest) (adapterActionResponse, error) {
	adapter, ok := findAdapterByID(mergedOfficeAdapters(b.adapters), req.AdapterID)
	if !ok {
		return adapterActionResponse{}, fmt.Errorf("adapter not found")
	}
	required := adapterActionCapabilities(req.Action)
	missing := missingAdapterCapabilities(adapter, required)
	payload := adapterActionResponse{
		Persisted:            false,
		AdapterID:            adapter.ID,
		Action:               req.Action,
		Status:               "preview",
		RequiredConfirmation: "ADAPTER_ACTION",
		RequiredCapabilities: required,
		MissingCapabilities:  missing,
	}
	configCheck := checkAdapterConfigRef(adapter)
	envCheck := checkAdapterEnvironment(adapter)
	payload.ConfigCheck = &configCheck
	payload.EnvironmentCheck = &envCheck
	if req.Action == "validate_config" {
		payload.Status = configCheck.Status
		payload.Message = configCheck.Summary
		return payload, nil
	}
	if len(missing) > 0 {
		payload.Status = "blocked"
		payload.Message = "Adapter action is blocked by missing declared capabilities."
		return payload, nil
	}
	if !req.Confirm || req.Confirmation != "ADAPTER_ACTION" {
		payload.Message = "Set confirm=true and confirmation=ADAPTER_ACTION to record this adapter action request."
		return payload, nil
	}
	if req.Reason == "" {
		return adapterActionResponse{}, fmt.Errorf("reason required")
	}
	b.appendActionLocked("adapter_action_requested", "adapter:"+adapter.ID, "general", req.Actor, truncateSummary(req.Action+": "+req.Reason, 140), adapter.ID)
	action := b.actions[len(b.actions)-1]
	if err := b.saveLocked(); err != nil {
		return adapterActionResponse{}, fmt.Errorf("failed to persist adapter action: %w", err)
	}
	payload.Persisted = true
	payload.Status = "queued"
	payload.AuditActionID = action.ID
	payload.Message = "Adapter action request recorded for a governed runner; no arbitrary process was executed by the bridge."
	return payload, nil
}

func findAdapterByID(adapters []officeAdapter, id string) (officeAdapter, bool) {
	id = normalizeExecutionKey(id)
	for _, adapter := range adapters {
		if normalizeExecutionKey(adapter.ID) == id {
			return adapter, true
		}
	}
	return officeAdapter{}, false
}

func adapterActionCapabilities(action string) []string {
	switch normalizeExecutionKey(action) {
	case "validate_config":
		return nil
	case "resync":
		return []string{"review.sync"}
	case "open_issue":
		return []string{"issue.open"}
	case "restart_process":
		return []string{"process.restart"}
	default:
		return []string{"adapter.action"}
	}
}

func missingAdapterCapabilities(adapter officeAdapter, required []string) []string {
	if len(required) == 0 {
		return nil
	}
	have := stringSet(adapter.Capabilities)
	var missing []string
	for _, capability := range required {
		if !setContains(have, capability) {
			missing = append(missing, capability)
		}
	}
	return missing
}

type workspaceInventoryResponse struct {
	GeneratedAt string                    `json:"generated_at"`
	Summary     map[string]int            `json:"summary"`
	Workspaces  []workspaceInventoryEntry `json:"workspaces"`
}

type workspaceInventoryEntry struct {
	ID              string   `json:"id"`
	Path            string   `json:"path"`
	Kind            string   `json:"kind,omitempty"`
	Channel         string   `json:"channel,omitempty"`
	Owner           string   `json:"owner,omitempty"`
	Healthy         bool     `json:"healthy"`
	Issue           string   `json:"issue,omitempty"`
	GitBranch       string   `json:"git_branch,omitempty"`
	GitDirtyCount   int      `json:"git_dirty_count,omitempty"`
	ActiveTaskCount int      `json:"active_task_count,omitempty"`
	TaskIDs         []string `json:"task_ids,omitempty"`
	PreviewURLs     []string `json:"preview_urls,omitempty"`
	UpdatedAt       string   `json:"updated_at,omitempty"`
}

func (b *Broker) handleWorkspacesInventory(w http.ResponseWriter, r *http.Request) {
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
	b.mu.RLock()
	payload := b.buildWorkspacesInventoryLocked(viewer, channel, allChannels)
	b.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) buildWorkspacesInventoryLocked(viewer, channel string, allChannels bool) workspaceInventoryResponse {
	buckets := map[string]*workspaceInventoryEntry{}
	add := func(entry workspaceInventoryEntry) {
		entry.Path = strings.TrimSpace(entry.Path)
		if entry.Path == "" {
			return
		}
		if !intakeChannelVisible(b, viewer, normalizeChannelSlug(entry.Channel), channel, allChannels) {
			return
		}
		key := strings.ToLower(filepath.Clean(entry.Path))
		current := buckets[key]
		if current == nil {
			entry.ID = "workspace:" + normalizeExecutionKey(filepath.Base(entry.Path))
			entry.GitBranch, entry.GitDirtyCount = gitWorkspaceSummary(entry.Path)
			buckets[key] = &entry
			return
		}
		current.TaskIDs = compactStringList(append(current.TaskIDs, entry.TaskIDs...))
		current.PreviewURLs = compactStringList(append(current.PreviewURLs, entry.PreviewURLs...))
		current.ActiveTaskCount += entry.ActiveTaskCount
		if current.Owner == "" {
			current.Owner = entry.Owner
		}
		if current.Channel == "" {
			current.Channel = entry.Channel
		}
		if !entry.Healthy {
			current.Healthy = false
			current.Issue = firstNonEmpty(current.Issue, entry.Issue)
		}
		if studioTimestampAfter(entry.UpdatedAt, current.UpdatedAt) {
			current.UpdatedAt = entry.UpdatedAt
		}
	}
	for _, task := range b.tasks {
		path := studioTaskWorkspacePath(task)
		if path == "" || strings.TrimSpace(task.ArchivedAt) != "" {
			continue
		}
		healthy, issue := studioWorkspaceHealth(task)
		active := 0
		if !taskIsTerminal(&task) {
			active = 1
		}
		add(workspaceInventoryEntry{
			Path:            path,
			Kind:            firstNonEmpty(task.ExecutionMode, "task_workspace"),
			Channel:         normalizeChannelSlug(task.Channel),
			Owner:           task.Owner,
			Healthy:         healthy,
			Issue:           issue,
			ActiveTaskCount: active,
			TaskIDs:         []string{task.ID},
			PreviewURLs:     taskPreviewURLs(task),
			UpdatedAt:       firstNonEmpty(task.UpdatedAt, task.CreatedAt),
		})
	}
	for _, ch := range b.channels {
		for _, repo := range ch.LinkedRepos {
			add(workspaceInventoryEntry{
				Path:      repo.RepoPath,
				Kind:      "linked_repo",
				Channel:   normalizeChannelSlug(ch.Slug),
				Owner:     repo.CreatedBy,
				Healthy:   taskWorktreeSourceLooksUsable(repo.RepoPath),
				Issue:     linkedRepoIssue(repo.RepoPath),
				UpdatedAt: firstNonEmpty(repo.UpdatedAt, repo.CreatedAt),
			})
		}
	}
	out := make([]workspaceInventoryEntry, 0, len(buckets))
	for _, entry := range buckets {
		entry.TaskIDs = compactStringList(entry.TaskIDs)
		entry.PreviewURLs = compactStringList(entry.PreviewURLs)
		out = append(out, *entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Healthy != out[j].Healthy {
			return !out[i].Healthy
		}
		return out[i].Path < out[j].Path
	})
	summary := map[string]int{"total": len(out)}
	for _, ws := range out {
		if ws.Healthy {
			summary["healthy"]++
		} else {
			summary["degraded"]++
		}
		if ws.GitDirtyCount > 0 {
			summary["dirty"]++
		}
	}
	return workspaceInventoryResponse{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Summary: summary, Workspaces: out}
}

func taskPreviewURLs(task teamTask) []string {
	var out []string
	for _, artifact := range task.Artifacts {
		out = appendUnique(out, artifact.PreviewURL)
		out = appendUnique(out, artifact.URL)
	}
	return out
}

func linkedRepoIssue(path string) string {
	if taskWorktreeSourceLooksUsable(path) {
		return ""
	}
	return "Linked repository path is not a usable git workspace."
}

func gitWorkspaceSummary(path string) (string, int) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	branchCmd := exec.CommandContext(ctx, "git", "-C", path, "branch", "--show-current")
	branchRaw, _ := branchCmd.Output()
	branch := strings.TrimSpace(string(branchRaw))
	ctx2, cancel2 := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel2()
	statusCmd := exec.CommandContext(ctx2, "git", "-C", path, "status", "--short")
	statusRaw, err := statusCmd.Output()
	if err != nil {
		return branch, 0
	}
	return branch, len(compactStringList(strings.Split(string(statusRaw), "\n")))
}

type outcomesResponse struct {
	GeneratedAt string          `json:"generated_at"`
	Summary     map[string]int  `json:"summary"`
	Items       []outcomeRecord `json:"items"`
}

type outcomeRecord struct {
	TaskID     string `json:"task_id"`
	Title      string `json:"title"`
	Channel    string `json:"channel,omitempty"`
	Owner      string `json:"owner,omitempty"`
	Kind       string `json:"kind"`
	State      string `json:"state"`
	Evidence   string `json:"evidence,omitempty"`
	ArtifactID string `json:"artifact_id,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

func (b *Broker) handleOutcomes(w http.ResponseWriter, r *http.Request) {
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
	b.mu.RLock()
	payload := b.buildOutcomesLocked(viewer, channel, allChannels)
	b.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) buildOutcomesLocked(viewer, channel string, allChannels bool) outcomesResponse {
	var items []outcomeRecord
	for _, task := range b.tasks {
		taskChannel := normalizeChannelSlug(task.Channel)
		if !intakeChannelVisible(b, viewer, taskChannel, channel, allChannels) {
			continue
		}
		record := classifyTaskOutcome(task)
		if record.Kind == "" {
			continue
		}
		record.TaskID = task.ID
		record.Title = task.Title
		record.Channel = taskChannel
		record.Owner = task.Owner
		record.UpdatedAt = firstNonEmpty(task.OutcomeVerifiedAt, task.UpdatedAt, task.CreatedAt)
		items = append(items, record)
	}
	sort.Slice(items, func(i, j int) bool { return studioTimestampAfter(items[i].UpdatedAt, items[j].UpdatedAt) })
	summary := map[string]int{"total": len(items)}
	for _, item := range items {
		summary[item.Kind]++
		summary[item.State]++
	}
	return outcomesResponse{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Summary: summary, Items: items}
}

func classifyTaskOutcome(task teamTask) outcomeRecord {
	state := "pending"
	if normalizeTaskStatus(task.Status) == "done" {
		state = "accepted"
	}
	if task.OutcomeStatus == "needs_evidence" {
		state = "needs_evidence"
	}
	evidence := firstNonEmpty(task.OutcomeEvidence, task.Outcome)
	for _, artifact := range task.Artifacts {
		kindText := strings.ToLower(strings.Join([]string{artifact.Kind, artifact.ResultRole, artifact.Title, artifact.Path, artifact.URL}, " "))
		switch {
		case strings.Contains(kindText, "pull_request"), strings.Contains(kindText, "merge"), strings.Contains(kindText, "build"):
			return outcomeRecord{Kind: "merged_code", State: outcomeStateFromArtifact(state, artifact), Evidence: firstNonEmpty(artifact.Summary, artifact.URL, artifact.Path), ArtifactID: artifact.ID}
		case strings.Contains(kindText, "doc"), strings.Contains(kindText, "manual"), strings.Contains(kindText, "report"):
			return outcomeRecord{Kind: "published_doc", State: outcomeStateFromArtifact(state, artifact), Evidence: firstNonEmpty(artifact.Summary, artifact.URL, artifact.Path), ArtifactID: artifact.ID}
		case artifact.State == "accepted" || artifact.State == "verified" || artifact.ResultRole == "evidence":
			return outcomeRecord{Kind: "accepted_artifact", State: outcomeStateFromArtifact(state, artifact), Evidence: firstNonEmpty(artifact.Summary, artifact.URL, artifact.Path), ArtifactID: artifact.ID}
		}
	}
	text := strings.ToLower(strings.Join([]string{task.Title, task.Details, task.Outcome, task.OutcomeEvidence, task.TaskType}, " "))
	switch {
	case strings.Contains(text, "decision") || strings.TrimSpace(task.SourceDecisionID) != "":
		return outcomeRecord{Kind: "explicit_decision", State: state, Evidence: evidence}
	case strings.Contains(text, "request") || strings.TrimSpace(task.SourceRequestID) != "":
		return outcomeRecord{Kind: "answered_request", State: state, Evidence: evidence}
	case strings.TrimSpace(evidence) != "":
		return outcomeRecord{Kind: "accepted_artifact", State: state, Evidence: evidence}
	default:
		return outcomeRecord{}
	}
}

func outcomeStateFromArtifact(fallback string, artifact taskArtifact) string {
	switch strings.ToLower(strings.TrimSpace(artifact.State)) {
	case "accepted", "verified":
		return "accepted"
	case "rejected", "failed":
		return "rejected"
	case "draft", "pending":
		return "pending"
	default:
		return fallback
	}
}
