package team

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nex-crm/wuphf/internal/company"
)

func TestCompanyControlPlanePreviewIsReadOnlyAndBlocksApply(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	b := NewBroker()
	b.tasks = append(b.tasks, teamTask{ID: "task-1", Title: "Ship", Status: "open", CreatedBy: "human"})
	beforeMembers := len(b.members)
	beforeChannels := len(b.channels)
	beforeTasks := len(b.tasks)

	req := httptest.NewRequest(http.MethodGet, "/companies/control-plane-preview", nil)
	rec := httptest.NewRecorder()
	b.handleCompanyControlPlanePreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload companyControlPlanePreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Persisted || payload.Status != "blocked" || payload.ApplyEnabled || payload.TopologyMutationEnabled {
		t.Fatalf("expected blocked read-only company preview, got %+v", payload)
	}
	if !stringSliceContains(payload.BlockedMutations, "topology_apply") || !stringSliceContains(payload.MissingPolicies, "secret_scrub_policy") {
		t.Fatalf("expected topology and scrub safeguards, got %+v", payload)
	}
	if len(b.members) != beforeMembers || len(b.channels) != beforeChannels || len(b.tasks) != beforeTasks {
		t.Fatalf("company preview mutated state: members %d->%d channels %d->%d tasks %d->%d", beforeMembers, len(b.members), beforeChannels, len(b.channels), beforeTasks, len(b.tasks))
	}
}

func TestCompanyControlPlanePreviewSummarizesExportAndIsolation(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	b := NewBroker()
	b.skills = append(b.skills, teamSkill{Name: "release-playbook", Title: "Release", CreatedBy: "human"})
	payload := b.buildCompanyControlPlanePreview(company.Manifest{
		Name:        "MaestrIA Local",
		Description: "Local office runtime",
		Lead:        "ceo",
	}, nil)
	if payload.CurrentCompany.ID != "maestria-local" || payload.CurrentCompany.Lead != "ceo" {
		t.Fatalf("expected current company identity, got %+v", payload.CurrentCompany)
	}
	if payload.Summary["export_items"] == 0 || len(payload.Isolation) < 3 {
		t.Fatalf("expected export and isolation preview, got %+v", payload)
	}
	var sawSkills bool
	for _, item := range payload.ExportItems {
		if item.ID == "skills" {
			sawSkills = true
			if !item.SecretScrubbed || !item.PreviewOnly || item.Count != 1 {
				t.Fatalf("expected scrubbed preview-only skills export item, got %+v", item)
			}
		}
	}
	if !sawSkills {
		t.Fatalf("expected skills export item, got %+v", payload.ExportItems)
	}
}
