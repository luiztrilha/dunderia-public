package team

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMarketplaceManifestPreviewIsReadOnlyAndReportsSkillDrift(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	b.skills = []teamSkill{{
		ID:           "skill-legacy",
		Name:         "repo-audit",
		Title:        "Repo audit",
		Description:  "Audit repos and report findings.",
		Content:      "Use repository context and produce a report.",
		Status:       "active",
		HealthStatus: "unknown",
		CreatedAt:    "2026-05-05T00:00:00Z",
		UpdatedAt:    "2026-05-05T00:00:00Z",
	}}
	beforeHash := b.skills[0].SourceHash
	beforeCaps := append([]string(nil), b.skills[0].Capabilities...)
	b.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/marketplace/manifest-preview", nil)
	rec := httptest.NewRecorder()
	b.handleMarketplaceManifestPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload marketplaceManifestPreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Persisted {
		t.Fatalf("marketplace preview must be read-only: %+v", payload)
	}
	var found marketplaceManifestPreview
	for _, manifest := range payload.Manifests {
		if manifest.ID == "skill:repo-audit" {
			found = manifest
			break
		}
	}
	if found.ID == "" {
		t.Fatalf("expected repo-audit manifest, got %+v", payload.Manifests)
	}
	if found.InstallEnabled || found.UpdateEnabled {
		t.Fatalf("preview must not enable install/update: %+v", found)
	}
	if found.ManifestSignature == "" || found.SignatureStatus != "preview_only" {
		t.Fatalf("expected preview signature metadata, got %+v", found)
	}
	if found.DriftStatus != "missing_installed_hash" || !stringSliceContains(found.MissingPolicies, "trusted_signature") {
		t.Fatalf("expected missing hash/signature drift, got %+v", found)
	}
	if !stringSliceContains(found.AddedCapabilities, "repo.context") {
		t.Fatalf("expected inferred repo capability drift, got %+v", found)
	}
	b.mu.RLock()
	after := b.skills[0]
	b.mu.RUnlock()
	if after.SourceHash != beforeHash || len(after.Capabilities) != len(beforeCaps) {
		t.Fatalf("preview mutated skill state: before_hash=%q before_caps=%+v after=%+v", beforeHash, beforeCaps, after)
	}
}

func TestMarketplaceManifestPreviewIncludesTrustedNoopWorkerContract(t *testing.T) {
	b := NewBroker()
	req := httptest.NewRequest(http.MethodGet, "/marketplace/manifest-preview", nil)
	rec := httptest.NewRecorder()
	b.handleMarketplaceManifestPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload marketplaceManifestPreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var found marketplaceManifestPreview
	for _, manifest := range payload.Manifests {
		if manifest.ID == "worker:noop-health" {
			found = manifest
			break
		}
	}
	if found.ID == "" {
		t.Fatalf("expected noop worker marketplace contract, got %+v", payload.Manifests)
	}
	if found.SignatureStatus != "trusted_builtin" || found.ManifestStatus != "ready" || found.DriftStatus != "none" {
		t.Fatalf("expected trusted built-in health-only contract, got %+v", found)
	}
	if found.InstallEnabled || found.UpdateEnabled {
		t.Fatalf("noop worker contract must remain preview-only for marketplace install/update: %+v", found)
	}
	if !stringSliceContains(found.RiskSignals, "health_only_worker") {
		t.Fatalf("expected health-only signal, got %+v", found)
	}
}
