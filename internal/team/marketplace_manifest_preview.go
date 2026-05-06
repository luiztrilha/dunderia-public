package team

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

type marketplaceManifestPreviewResponse struct {
	GeneratedAt string                       `json:"generated_at"`
	Persisted   bool                         `json:"persisted"`
	Status      string                       `json:"status"`
	Summary     map[string]int               `json:"summary"`
	Manifests   []marketplaceManifestPreview `json:"manifests"`
}

type marketplaceManifestPreview struct {
	ID                   string   `json:"id"`
	Kind                 string   `json:"kind"`
	Name                 string   `json:"name"`
	SourceType           string   `json:"source_type,omitempty"`
	SourceRef            string   `json:"source_ref,omitempty"`
	InstalledHash        string   `json:"installed_hash,omitempty"`
	ExpectedHash         string   `json:"expected_hash,omitempty"`
	ManifestID           string   `json:"manifest_id"`
	ManifestVersion      string   `json:"manifest_version"`
	ManifestSignature    string   `json:"manifest_signature"`
	SignatureStatus      string   `json:"signature_status"`
	TrustLevel           string   `json:"trust_level,omitempty"`
	TrustScore           int      `json:"trust_score,omitempty"`
	ManifestStatus       string   `json:"manifest_status"`
	DriftStatus          string   `json:"drift_status"`
	InstallEnabled       bool     `json:"install_enabled"`
	UpdateEnabled        bool     `json:"update_enabled"`
	Capabilities         []string `json:"capabilities,omitempty"`
	ProposedCapabilities []string `json:"proposed_capabilities,omitempty"`
	AddedCapabilities    []string `json:"added_capabilities,omitempty"`
	RequiredReviews      []string `json:"required_reviews,omitempty"`
	RequiredPolicies     []string `json:"required_policies,omitempty"`
	MissingPolicies      []string `json:"missing_policies,omitempty"`
	RiskSignals          []string `json:"risk_signals,omitempty"`
	RiskScore            int      `json:"risk_score,omitempty"`
	RiskLevel            string   `json:"risk_level,omitempty"`
	NextStep             string   `json:"next_step,omitempty"`
}

func (b *Broker) handleMarketplaceManifestPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.mu.RLock()
	payload := b.buildMarketplaceManifestPreviewLocked()
	b.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) buildMarketplaceManifestPreviewLocked() marketplaceManifestPreviewResponse {
	manifests := []marketplaceManifestPreview{marketplacePreviewFromNoopWorker()}
	for _, adapter := range mergedOfficeAdapters(b.adapters) {
		manifests = append(manifests, marketplacePreviewFromAdapter(adapter))
	}
	for _, skill := range b.skills {
		if skill.Status == "archived" {
			continue
		}
		manifests = append(manifests, marketplacePreviewFromSkill(skill))
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].ID < manifests[j].ID })
	status := "ok"
	summary := map[string]int{"total": len(manifests)}
	for _, manifest := range manifests {
		summary[manifest.Kind]++
		summary[manifest.ManifestStatus]++
		summary["drift_"+manifest.DriftStatus]++
		if manifest.SignatureStatus != "trusted_builtin" {
			summary["signature_review"]++
		}
		if manifest.ManifestStatus == "blocked" {
			status = "blocked"
		} else if manifest.ManifestStatus == "review" && status == "ok" {
			status = "review"
		}
	}
	return marketplaceManifestPreviewResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Persisted:   false,
		Status:      status,
		Summary:     summary,
		Manifests:   manifests,
	}
}

func marketplacePreviewFromSkill(skill teamSkill) marketplaceManifestPreview {
	trust := buildSkillTrustRecord(skill)
	upgrade := buildSkillCapabilityUpgradePreview(skill)
	installedHash := strings.TrimSpace(skill.SourceHash)
	expectedHash := skillSourceHash(skill)
	signature := marketplacePreviewSignature(map[string]any{
		"id":           firstNonEmpty(skill.PluginID, "legacy-"+skillSlug(skill.Name)),
		"kind":         firstNonEmpty(skill.PluginKind, inferSkillPluginKind(skill)),
		"name":         skill.Name,
		"source_type":  firstNonEmpty(skill.SourceType, inferSkillSourceType(skill)),
		"source_ref":   firstNonEmpty(skill.SourceRef, inferSkillSourceRef(skill)),
		"source_hash":  firstNonEmpty(installedHash, expectedHash),
		"capabilities": upgrade.ProposedCapabilities,
	})
	preview := marketplaceManifestPreview{
		ID:                   "skill:" + skillSlug(skill.Name),
		Kind:                 "skill",
		Name:                 firstNonEmpty(skill.Title, skill.Name),
		SourceType:           firstNonEmpty(skill.SourceType, inferSkillSourceType(skill)),
		SourceRef:            firstNonEmpty(skill.SourceRef, inferSkillSourceRef(skill)),
		InstalledHash:        installedHash,
		ExpectedHash:         expectedHash,
		ManifestID:           firstNonEmpty(skill.PluginID, "legacy-"+skillSlug(skill.Name)),
		ManifestVersion:      "preview.v1",
		ManifestSignature:    signature,
		SignatureStatus:      "preview_only",
		TrustLevel:           trust.Level,
		TrustScore:           trust.Score,
		InstallEnabled:       false,
		UpdateEnabled:        false,
		Capabilities:         normalizeSkillCapabilities(skill.Capabilities),
		ProposedCapabilities: upgrade.ProposedCapabilities,
		AddedCapabilities:    upgrade.AddedCapabilities,
		RequiredReviews:      []string{"operator", "skill-owner"},
		RequiredPolicies:     []string{"trusted_signature", "source_hash", "security_scan", "capability_review"},
		RiskScore:            upgrade.RiskScore,
		RiskLevel:            upgrade.RiskLevel,
	}
	preview.MissingPolicies = marketplaceMissingPoliciesForSkill(skill, preview)
	preview.RiskSignals = marketplaceRiskSignalsForSkill(skill, preview)
	preview.DriftStatus = marketplaceDriftStatus(preview)
	preview.ManifestStatus, preview.NextStep = marketplaceManifestStatus(preview)
	return preview
}

func marketplacePreviewFromAdapter(adapter officeAdapter) marketplaceManifestPreview {
	configCheck := checkAdapterConfigRef(adapter)
	expectedHash := marketplaceAdapterHash(adapter)
	signature := marketplacePreviewSignature(map[string]any{
		"id":           adapter.ID,
		"kind":         firstNonEmpty(adapter.Kind, "adapter"),
		"name":         firstNonEmpty(adapter.Name, adapter.ID),
		"capabilities": compactStringList(adapter.Capabilities),
		"config_ref":   configCheck.ConfigRef,
	})
	preview := marketplaceManifestPreview{
		ID:                   "adapter:" + adapter.ID,
		Kind:                 firstNonEmpty(adapter.Kind, "adapter"),
		Name:                 firstNonEmpty(adapter.Name, adapter.ID),
		SourceType:           firstNonEmpty(adapter.Source, "adapter"),
		SourceRef:            adapter.ID,
		ExpectedHash:         expectedHash,
		ManifestID:           "adapter." + normalizeExecutionKey(adapter.ID),
		ManifestVersion:      "preview.v1",
		ManifestSignature:    signature,
		SignatureStatus:      "preview_only",
		ManifestStatus:       "review",
		DriftStatus:          "missing_installed_hash",
		InstallEnabled:       false,
		UpdateEnabled:        false,
		Capabilities:         compactStringList(adapter.Capabilities),
		ProposedCapabilities: compactStringList(adapter.Capabilities),
		RequiredReviews:      []string{"operator", "security"},
		RequiredPolicies:     []string{"trusted_signature", "source_hash", "security_scan", "capability_review", "config_ref"},
		MissingPolicies:      []string{"trusted_signature", "source_hash", "security_scan"},
		RiskScore:            marketplaceAdapterRiskScore(adapter, configCheck),
	}
	preview.RiskLevel = templatePreviewRiskLevel(preview.RiskScore)
	preview.RiskSignals = []string{"preview_signature_only", "install_disabled", "update_disabled"}
	if configCheck.Status == "fail" {
		preview.MissingPolicies = appendUnique(preview.MissingPolicies, "config_ref")
		preview.RiskSignals = appendUnique(preview.RiskSignals, "config_ref_blocked")
		preview.ManifestStatus = "blocked"
	}
	preview.NextStep = "Keep marketplace install/update disabled until this adapter has a trusted signature, source hash, security scan, and reviewed config references."
	return preview
}

func marketplacePreviewFromNoopWorker() marketplaceManifestPreview {
	worker := pluginSandboxNoopWorkerCandidate()
	return marketplaceManifestPreview{
		ID:                   worker.ID,
		Kind:                 "worker",
		Name:                 worker.Name,
		SourceType:           "builtin",
		SourceRef:            worker.ManifestID,
		ExpectedHash:         strings.TrimPrefix(worker.ManifestSignature, "sha256:"),
		ManifestID:           worker.ManifestID,
		ManifestVersion:      "preview.v1",
		ManifestSignature:    worker.ManifestSignature,
		SignatureStatus:      "trusted_builtin",
		ManifestStatus:       "ready",
		DriftStatus:          "none",
		InstallEnabled:       false,
		UpdateEnabled:        false,
		Capabilities:         worker.Capabilities,
		ProposedCapabilities: worker.Capabilities,
		RequiredReviews:      []string{"operator"},
		RequiredPolicies:     []string{"trusted_signature", "capability_review", "security_scan"},
		RiskSignals:          []string{"install_disabled", "update_disabled", "health_only_worker"},
		RiskScore:            5,
		RiskLevel:            "low",
		NextStep:             "Use this built-in health-only manifest as the marketplace contract shape; it does not enable plugin action execution.",
	}
}

func marketplaceMissingPoliciesForSkill(skill teamSkill, preview marketplaceManifestPreview) []string {
	missing := []string{"trusted_signature"}
	if strings.TrimSpace(preview.InstalledHash) == "" {
		missing = append(missing, "source_hash")
	}
	if normalizeSkillScanStatus(skill.ScanStatus) != "passed" {
		missing = append(missing, "security_scan")
	}
	if len(preview.AddedCapabilities) > 0 {
		missing = append(missing, "capability_review")
	}
	return compactStringList(missing)
}

func marketplaceRiskSignalsForSkill(skill teamSkill, preview marketplaceManifestPreview) []string {
	signals := []string{"preview_signature_only", "install_disabled", "update_disabled"}
	if strings.TrimSpace(preview.InstalledHash) == "" {
		signals = append(signals, "source_hash_missing")
	} else if preview.InstalledHash != preview.ExpectedHash {
		signals = append(signals, "source_hash_drift")
	}
	if len(preview.AddedCapabilities) > 0 {
		signals = append(signals, "capability_drift")
	}
	if trust := strings.ToLower(preview.TrustLevel); trust == "low" || trust == "medium" {
		signals = append(signals, "trust_"+trust)
	}
	switch normalizeSkillScanStatus(skill.ScanStatus) {
	case "warning":
		signals = append(signals, "scan_warning")
	case "blocked":
		signals = append(signals, "scan_blocked")
	case "":
		signals = append(signals, "scan_missing")
	}
	if contentLooksSecretBearing(skill.Content + " " + skill.Description + " " + skill.WorkflowDefinition) {
		signals = append(signals, "secret_like_content")
	}
	return compactStringList(signals)
}

func marketplaceDriftStatus(preview marketplaceManifestPreview) string {
	if strings.TrimSpace(preview.InstalledHash) == "" && preview.Kind != "worker" {
		return "missing_installed_hash"
	}
	if preview.InstalledHash != "" && preview.ExpectedHash != "" && preview.InstalledHash != preview.ExpectedHash {
		return "hash_drift"
	}
	if len(preview.AddedCapabilities) > 0 {
		return "capability_drift"
	}
	if preview.SignatureStatus != "trusted_builtin" {
		return "signature_unverified"
	}
	return "none"
}

func marketplaceManifestStatus(preview marketplaceManifestPreview) (string, string) {
	signals := stringSet(preview.RiskSignals)
	if setContains(signals, "scan_blocked") || setContains(signals, "secret_like_content") || setContains(signals, "config_ref_blocked") {
		return "blocked", "Keep marketplace install/update disabled; resolve blocked scan, secret-like content, or config reference findings first."
	}
	if len(preview.MissingPolicies) > 0 || preview.DriftStatus != "none" || preview.TrustLevel == "low" {
		return "review", "Review signature, source hash, scan result, and capability drift before any future marketplace install/update flow exists."
	}
	return "ready", "Manifest shape is complete for preview purposes; install/update remains disabled until an explicit marketplace apply path is designed."
}

func marketplacePreviewSignature(payload any) string {
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("sha256:%x", sum)
}

func marketplaceAdapterHash(adapter officeAdapter) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(adapter.ID),
		strings.TrimSpace(adapter.Name),
		strings.TrimSpace(adapter.Kind),
		strings.Join(compactStringList(adapter.Capabilities), ","),
		strings.TrimSpace(adapter.ConfigRef),
	}, "\n")))
	return hex.EncodeToString(sum[:])
}

func marketplaceAdapterRiskScore(adapter officeAdapter, configCheck adapterConfigCheck) int {
	score := 25 + capabilityUpgradeRiskScore(adapter.Capabilities)
	if configCheck.Status == "fail" {
		score += 35
	} else if configCheck.Status == "warn" {
		score += 15
	}
	if score > 100 {
		return 100
	}
	return score
}
