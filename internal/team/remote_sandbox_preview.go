package team

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"
)

type remoteSandboxPreviewResponse struct {
	GeneratedAt string                          `json:"generated_at"`
	Persisted   bool                            `json:"persisted"`
	Status      string                          `json:"status"`
	Summary     map[string]int                  `json:"summary"`
	Candidates  []remoteSandboxPreviewCandidate `json:"candidates"`
}

type remoteSandboxPreviewCandidate struct {
	ID               string                            `json:"id"`
	Provider         string                            `json:"provider"`
	Kind             string                            `json:"kind"`
	Readiness        string                            `json:"readiness"`
	ExecutionEnabled bool                              `json:"execution_enabled"`
	HealthCheck      string                            `json:"health_check,omitempty"`
	InstallPolicy    string                            `json:"install_command_policy,omitempty"`
	InstallEnabled   bool                              `json:"install_command_enabled"`
	InstallPreview   string                            `json:"install_command_preview,omitempty"`
	RequiredPolicies []string                          `json:"required_policies,omitempty"`
	MissingPolicies  []string                          `json:"missing_policies,omitempty"`
	PolicyChecks     []executionEnvironmentPolicyCheck `json:"policy_checks,omitempty"`
	RiskSignals      []string                          `json:"risk_signals,omitempty"`
	NextStep         string                            `json:"next_step,omitempty"`
}

func (b *Broker) handleRemoteSandboxPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	payload := buildRemoteSandboxPreview()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func buildRemoteSandboxPreview() remoteSandboxPreviewResponse {
	candidates := []remoteSandboxPreviewCandidate{
		buildRemoteSandboxCandidate("remote-sandbox:docker", "docker", "container", []string{"docker_binary", "workspace_policy", "secret_policy", "network_policy", "cleanup_policy", "audit_policy", "approval_policy", "install_command_policy"}),
		buildRemoteSandboxCandidate("remote-sandbox:ssh", "ssh", "external_host", []string{"ssh_binary", "host_policy", "key_policy", "workspace_policy", "secret_policy", "network_policy", "cleanup_policy", "audit_policy", "approval_policy", "install_command_policy"}),
		buildRemoteSandboxCandidate("remote-sandbox:self-hosted-worker", "self_hosted_worker", "worker_pool", []string{"worker_manifest", "host_policy", "workspace_policy", "secret_policy", "network_policy", "cleanup_policy", "audit_policy", "approval_policy", "cost_policy", "install_command_policy"}),
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	status := "ok"
	summary := map[string]int{"total": len(candidates)}
	for _, candidate := range candidates {
		summary["readiness_"+candidate.Readiness]++
		summary["provider_"+candidate.Provider]++
		if !candidate.ExecutionEnabled {
			summary["execution_disabled"]++
		}
		if candidate.Readiness == "blocked" {
			status = "blocked"
		} else if candidate.Readiness == "review" && status == "ok" {
			status = "review"
		}
	}
	return remoteSandboxPreviewResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Persisted:   false,
		Status:      status,
		Summary:     summary,
		Candidates:  candidates,
	}
}

func buildRemoteSandboxCandidate(id, provider, kind string, policies []string) remoteSandboxPreviewCandidate {
	candidate := remoteSandboxPreviewCandidate{
		ID:               id,
		Provider:         provider,
		Kind:             kind,
		Readiness:        "blocked",
		ExecutionEnabled: false,
		HealthCheck:      "static_preview_only",
		InstallPolicy:    "blocked_until_adapter_manifest_approved",
		InstallEnabled:   false,
		InstallPreview:   "Adapter install commands must be declared in a reviewed manifest, cost/network scoped, and followed by resolvability and hello checks before execution is allowed.",
		RequiredPolicies: compactStringList(policies),
		RiskSignals:      []string{"remote_execution_disabled", "install_command_disabled", "requires_governance", "preview_only"},
		NextStep:         "Keep remote execution and install commands disabled until workspace, secrets, network, cleanup, audit, approval, cost, and install-command policies are reviewed.",
	}
	for _, policy := range policies {
		check := remoteSandboxPolicyCheck(policy)
		candidate.PolicyChecks = append(candidate.PolicyChecks, check)
		if check.Status != "ok" {
			candidate.MissingPolicies = appendUnique(candidate.MissingPolicies, policy)
		}
		if check.Status == "missing" {
			candidate.RiskSignals = appendUnique(candidate.RiskSignals, "missing_"+policy)
		}
	}
	if provider == "ssh" {
		candidate.RiskSignals = appendUnique(candidate.RiskSignals, "external_host")
	}
	candidate.MissingPolicies = compactStringList(candidate.MissingPolicies)
	candidate.PolicyChecks = compactExecutionEnvironmentPolicyChecks(candidate.PolicyChecks)
	return candidate
}

func remoteSandboxPolicyCheck(policy string) executionEnvironmentPolicyCheck {
	switch strings.TrimSpace(policy) {
	case "docker_binary":
		return executionEnvironmentBinaryPolicyCheck("docker", "Install Docker only when a governed container adapter is approved.")
	case "ssh_binary":
		return executionEnvironmentBinaryPolicyCheck("ssh", "Install OpenSSH only when a governed SSH adapter is approved.")
	default:
		return executionEnvironmentPolicyCheck{
			ID:       policy,
			Status:   "missing",
			Summary:  "Remote sandbox governance policy is not declared in this local office.",
			NextStep: "Define and review this policy before enabling any remote sandbox backend.",
		}
	}
}
