package team

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/nex-crm/wuphf/internal/company"
)

type companyControlPlanePreviewResponse struct {
	GeneratedAt             string                          `json:"generated_at"`
	Persisted               bool                            `json:"persisted"`
	Status                  string                          `json:"status"`
	Summary                 map[string]int                  `json:"summary"`
	CurrentCompany          companyControlPlaneSnapshot     `json:"current_company"`
	ExportItems             []companyControlPlaneExportItem `json:"export_items"`
	Isolation               []companyControlPlaneIsolation  `json:"isolation"`
	BlockedMutations        []string                        `json:"blocked_mutations"`
	RequiredPolicies        []string                        `json:"required_policies"`
	MissingPolicies         []string                        `json:"missing_policies"`
	RiskSignals             []string                        `json:"risk_signals"`
	NextStep                string                          `json:"next_step"`
	ApplyEnabled            bool                            `json:"apply_enabled"`
	TopologyMutationEnabled bool                            `json:"topology_mutation_enabled"`
}

type companyControlPlaneSnapshot struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	Lead           string `json:"lead,omitempty"`
	ManifestSource string `json:"manifest_source"`
	MemberCount    int    `json:"member_count"`
	ChannelCount   int    `json:"channel_count"`
	TaskCount      int    `json:"task_count"`
	SkillCount     int    `json:"skill_count"`
	AdapterCount   int    `json:"adapter_count"`
}

type companyControlPlaneExportItem struct {
	ID             string   `json:"id"`
	Label          string   `json:"label"`
	Source         string   `json:"source"`
	Count          int      `json:"count"`
	SecretScrubbed bool     `json:"secret_scrubbed"`
	PreviewOnly    bool     `json:"preview_only"`
	RiskSignals    []string `json:"risk_signals,omitempty"`
}

type companyControlPlaneIsolation struct {
	ID        string   `json:"id"`
	Status    string   `json:"status"`
	Summary   string   `json:"summary"`
	NextStep  string   `json:"next_step,omitempty"`
	Contracts []string `json:"contracts,omitempty"`
}

func (b *Broker) handleCompanyControlPlanePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	manifest, err := company.LoadManifest()
	payload := b.buildCompanyControlPlanePreview(manifest, err)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) buildCompanyControlPlanePreview(manifest company.Manifest, manifestErr error) companyControlPlanePreviewResponse {
	b.mu.RLock()
	memberCount := len(b.members)
	channelCount := len(b.channels)
	taskCount := len(b.tasks)
	skillCount := len(b.skills)
	adapterCount := len(b.adapters)
	b.mu.RUnlock()

	manifestSource := company.ManifestPath()
	if manifestErr != nil {
		manifest = company.DefaultManifest()
		manifestSource = "default_after_manifest_error"
	}
	name := strings.TrimSpace(manifest.Name)
	if name == "" {
		name = "Local Office"
	}
	lead := strings.TrimSpace(manifest.Lead)
	current := companyControlPlaneSnapshot{
		ID:             companyPreviewID(name),
		Name:           name,
		Description:    strings.TrimSpace(manifest.Description),
		Lead:           lead,
		ManifestSource: manifestSource,
		MemberCount:    memberCount,
		ChannelCount:   channelCount,
		TaskCount:      taskCount,
		SkillCount:     skillCount,
		AdapterCount:   adapterCount,
	}

	requiredPolicies := []string{
		"company_id_policy",
		"state_root_policy",
		"secret_scrub_policy",
		"topology_authorization",
		"backup_policy",
		"collision_policy",
		"rollback_policy",
		"human_review_policy",
	}
	missingPolicies := append([]string(nil), requiredPolicies...)
	blockedMutations := []string{
		"create_company",
		"switch_company",
		"import_company",
		"delete_company",
		"topology_apply",
		"state_root_repoint",
	}
	exportItems := []companyControlPlaneExportItem{
		{ID: "company_manifest", Label: "Company manifest", Source: "company.json", Count: 1, SecretScrubbed: true, PreviewOnly: true, RiskSignals: []string{"protected_topology"}},
		{ID: "members", Label: "Members", Source: "broker_state", Count: memberCount, SecretScrubbed: true, PreviewOnly: true, RiskSignals: []string{"topology"}},
		{ID: "channels", Label: "Channels", Source: "broker_state", Count: channelCount, SecretScrubbed: true, PreviewOnly: true, RiskSignals: []string{"routing"}},
		{ID: "tasks", Label: "Tasks", Source: "broker_state", Count: taskCount, SecretScrubbed: true, PreviewOnly: true, RiskSignals: []string{"work_history"}},
		{ID: "skills", Label: "Skills", Source: "broker_state", Count: skillCount, SecretScrubbed: true, PreviewOnly: true, RiskSignals: []string{"behavior_change"}},
		{ID: "adapters", Label: "Adapters", Source: "broker_state", Count: adapterCount, SecretScrubbed: true, PreviewOnly: true, RiskSignals: []string{"external_capability"}},
	}
	isolation := []companyControlPlaneIsolation{
		{
			ID:        "state_roots",
			Status:    "blocked",
			Summary:   "Multiple companies need separate state roots before any switch/import flow exists.",
			NextStep:  "Design state-root selection, backup, restore, and conflict checks as dry-run-only surfaces first.",
			Contracts: []string{"no_shared_broker_state", "explicit_restore_only"},
		},
		{
			ID:        "topology_guard",
			Status:    "blocked",
			Summary:   "Company import/export cannot create, delete, rename, or reassign agents/channels without current human authorization.",
			NextStep:  "Keep import/export as a preview until a confirmation-gated topology apply contract is reviewed.",
			Contracts: []string{"protected_topology", "human_confirmation_required"},
		},
		{
			ID:        "secret_scrub",
			Status:    "blocked",
			Summary:   "Portable company packages must scrub secrets, raw sessions, private memory snapshots, and live broker state.",
			NextStep:  "Define a scrubbed export package schema before any package writer is enabled.",
			Contracts: []string{"secret_refs_only", "no_raw_sessions", "no_private_memory"},
		},
	}
	sort.Slice(exportItems, func(i, j int) bool { return exportItems[i].ID < exportItems[j].ID })
	sort.Strings(blockedMutations)
	sort.Strings(requiredPolicies)
	sort.Strings(missingPolicies)

	summary := map[string]int{
		"companies":         1,
		"members":           memberCount,
		"channels":          channelCount,
		"tasks":             taskCount,
		"skills":            skillCount,
		"adapters":          adapterCount,
		"export_items":      len(exportItems),
		"blocked_mutations": len(blockedMutations),
		"missing_policies":  len(missingPolicies),
	}
	if manifestErr != nil {
		summary["manifest_error"] = 1
	}

	return companyControlPlanePreviewResponse{
		GeneratedAt:             time.Now().UTC().Format(time.RFC3339),
		Persisted:               false,
		Status:                  "blocked",
		Summary:                 summary,
		CurrentCompany:          current,
		ExportItems:             exportItems,
		Isolation:               isolation,
		BlockedMutations:        blockedMutations,
		RequiredPolicies:        requiredPolicies,
		MissingPolicies:         missingPolicies,
		RiskSignals:             []string{"preview_only", "protected_topology", "secret_scrub_required", "no_company_switch_runtime"},
		NextStep:                "Keep multi-company work at export/isolation preview level until state roots, scrubbed packages, collision handling, backups, rollback, and human review policies are designed.",
		ApplyEnabled:            false,
		TopologyMutationEnabled: false,
	}
}

func companyPreviewID(name string) string {
	id := normalizeChannelSlug(name)
	id = strings.Trim(id, "-")
	if id == "" || id == "general" {
		return "local-office"
	}
	return id
}
