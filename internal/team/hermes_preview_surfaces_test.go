package team

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHermesCommandManifestIsReadOnlyAndMarksMutatingCommands(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	beforeMessages := len(b.messages)
	beforeTasks := len(b.tasks)
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/commands/manifest?q=reset", nil)
	b.handleCommandManifest(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload commandManifestResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Persisted || len(payload.Commands) != 1 {
		t.Fatalf("expected one read-only reset command hit, got %+v", payload)
	}
	cmd := payload.Commands[0]
	if cmd.Name != "/reset" || !cmd.Mutating || !cmd.RequiresConfirmation || !cmd.TopologySensitive {
		t.Fatalf("expected reset command governance metadata, got %+v", cmd)
	}

	b.mu.RLock()
	afterMessages := len(b.messages)
	afterTasks := len(b.tasks)
	b.mu.RUnlock()
	if afterMessages != beforeMessages || afterTasks != beforeTasks {
		t.Fatalf("command manifest mutated state: messages %d -> %d, tasks %d -> %d", beforeMessages, afterMessages, beforeTasks, afterTasks)
	}
}

func TestHermesCommandManifestFiltersBySurface(t *testing.T) {
	b := NewBroker()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/commands/manifest?surface=tui&q=request", nil)
	b.handleCommandManifest(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload commandManifestResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	byName := map[string]commandManifestEntry{}
	for _, command := range payload.Commands {
		byName[command.Name] = command
	}
	if _, ok := byName["/requests"]; !ok {
		t.Fatalf("expected TUI manifest to include /requests, got %+v", payload.Commands)
	}
	if _, ok := byName["/request"]; !ok {
		t.Fatalf("expected TUI manifest to include /request, got %+v", payload.Commands)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/commands/manifest", nil)
	b.handleCommandManifest(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected default web manifest status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	payload = commandManifestResponse{}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode default response: %v", err)
	}
	for _, command := range payload.Commands {
		if command.Name == "/messages" {
			t.Fatalf("expected default manifest to stay web-only, got %+v", payload.Commands)
		}
	}
}

func TestHermesCommandManifestDriftReportsManualAndManifestGaps(t *testing.T) {
	now := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
	commands := []commandManifestEntry{
		{Name: "/known", Category: "navigation"},
		{Name: "/with-args", Category: "task"},
		{Name: "/missing-doc", Category: "runtime"},
	}
	manual := "## 10. Slash Commands Na Web\n\n| Comando | Uso |\n|---|---|\n| `/known` | ok |\n| `/manual-only` | old |\n\n## 11. Next\n`/not-in-section`\n"
	manual = strings.Replace(manual, "| `/known` | ok |", "| `/known` | ok |\n| `/with-args <id>` | ok |", 1)
	payload := buildCommandManifestDrift(manual, commands, now)
	if payload.Persisted || payload.Status != "warning" || len(payload.Items) != 2 {
		t.Fatalf("expected two warning drift items, got %+v", payload)
	}
	byKind := map[string]commandManifestDriftItem{}
	for _, item := range payload.Items {
		byKind[item.Kind] = item
	}
	if byKind["manifest_missing_manual"].Command != "/missing-doc" {
		t.Fatalf("expected manifest gap for /missing-doc, got %+v", payload.Items)
	}
	if byKind["manual_missing_manifest"].Command != "/manual-only" {
		t.Fatalf("expected manual gap for /manual-only, got %+v", payload.Items)
	}
}

func TestHermesExecutionEnvironmentPreviewReportsFutureAdaptersAndWorkspaceRisk(t *testing.T) {
	b := NewBroker()
	workspacePath := filepath.Join(t.TempDir(), "external-workspace")
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{{
		ID:            "task-ext",
		Channel:       "general",
		Title:         "Audit external workspace",
		Status:        "in_progress",
		Owner:         "eng",
		ExecutionMode: "external_workspace",
		WorkspacePath: workspacePath,
		CreatedAt:     "2026-05-04T10:00:00Z",
		UpdatedAt:     "2026-05-04T10:01:00Z",
	}}
	beforeTasks := len(b.tasks)
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/runtime/execution-environments-preview?channel=general&viewer_slug=human", nil)
	b.handleExecutionEnvironmentsPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload executionEnvironmentPreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	kinds := map[string]executionEnvironmentPreview{}
	for _, env := range payload.Environments {
		kinds[env.Kind] = env
	}
	if kinds["docker"].Readiness != "blocked" || kinds["ssh"].Readiness != "blocked" {
		t.Fatalf("expected future docker/ssh adapters to be blocked, got %+v", kinds)
	}
	docker := kinds["docker"]
	if !docker.RequiresReview || !stringSliceContains(docker.RequiredPolicies, "workspace_policy") || !stringSliceContains(docker.RequiredPolicies, "approval_policy") {
		t.Fatalf("expected docker preview to require governed execution policies, got %+v", docker)
	}
	if len(docker.PolicyChecks) == 0 || !stringSliceContains(docker.Signals, "requires_policy") || docker.NextStep == "" {
		t.Fatalf("expected docker preview to expose policy checks and next step, got %+v", docker)
	}
	ssh := kinds["ssh"]
	if !ssh.RequiresReview || !stringSliceContains(ssh.RequiredPolicies, "host_policy") || !stringSliceContains(ssh.Signals, "external_host") {
		t.Fatalf("expected ssh preview to require host policy and external host signal, got %+v", ssh)
	}
	if len(ssh.PolicyChecks) == 0 || !stringSliceContains(ssh.MissingPolicies, "approval_policy") || ssh.NextStep == "" {
		t.Fatalf("expected ssh preview to expose missing governed policies, got %+v", ssh)
	}
	external := kinds["external_workspace"]
	if !external.RequiresReview || !stringSliceContains(external.Signals, "explicit_workspace") {
		t.Fatalf("expected external workspace review signal, got %+v", external)
	}

	b.mu.RLock()
	afterTasks := len(b.tasks)
	b.mu.RUnlock()
	if afterTasks != beforeTasks {
		t.Fatalf("execution environment preview mutated tasks: %d -> %d", beforeTasks, afterTasks)
	}
}

func TestHermesSkillFilesPreviewListsAndSelectsVirtualFiles(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.skills = []teamSkill{{
		ID:                 "skill-release",
		Name:               "release-playbook",
		Title:              "Release Playbook",
		Description:        "Release procedure",
		Content:            "Run release checks before publishing. Do not paste API_TOKEN values.",
		WorkflowDefinition: `{"version":1,"steps":[{"id":"check","kind":"command"}]}`,
		Channel:            "general",
		Status:             "active",
		SourceType:         "starter_pack",
		SourceRef:          "default-skill:release-playbook",
		ScanSummary:        "Static scan passed.",
	}}
	beforeSkills := len(b.skills)
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/skills/release-playbook/files-preview?channel=general&viewer_slug=human", nil)
	b.handleSkillsSubpath(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var listPayload skillFilePreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&listPayload); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if listPayload.Persisted || len(listPayload.Files) == 0 {
		t.Fatalf("expected read-only virtual file list, got %+v", listPayload)
	}
	for _, file := range listPayload.Files {
		if file.Content != "" {
			t.Fatalf("default file list should not include content, got %+v", file)
		}
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/skills/release-playbook/files-preview?channel=general&viewer_slug=human&file=content.md", nil)
	b.handleSkillsSubpath(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 selecting content, got %d: %s", rec.Code, rec.Body.String())
	}
	var selected skillFilePreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&selected); err != nil {
		t.Fatalf("decode selected response: %v", err)
	}
	if len(selected.Files) != 1 || selected.Files[0].Name != "content.md" || selected.Files[0].Content == "" {
		t.Fatalf("expected selected content.md with content, got %+v", selected)
	}
	if !stringSliceContains(selected.Files[0].RiskSignals, "secret_like_content") {
		t.Fatalf("expected secret-like risk signal for content, got %+v", selected.Files[0])
	}

	b.mu.RLock()
	afterSkills := len(b.skills)
	b.mu.RUnlock()
	if afterSkills != beforeSkills {
		t.Fatalf("skill files preview mutated skills: %d -> %d", beforeSkills, afterSkills)
	}
}
