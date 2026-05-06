package team

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHandleTaskReassignClearsBlockedFlag(t *testing.T) {
	tmpDir := t.TempDir()
	oldPathFn := brokerStatePath
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	workspaceRoot := filepath.Join(tmpDir, "ConveniosWebBNB_Antigo")
	initUsableGitWorktree(t, workspaceRoot)

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "human", "Human")
	if err := b.StartOnPort(0); err != nil {
		t.Fatal(err)
	}
	defer b.Stop()

	postJSON := func(path string, payload map[string]any) map[string]any {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		req, err := http.NewRequest(http.MethodPost, "http://"+b.Addr()+path, bytes.NewReader(body))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+b.Token())
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("post %s: %v", path, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("post %s status = %d", path, resp.StatusCode)
		}
		var out map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		return out
	}

	create := postJSON("/tasks", map[string]any{
		"action":         "create",
		"channel":        "general",
		"title":          "Retomar slice legado",
		"details":        "Task criada para validar desbloqueio por reassign.",
		"owner":          "builder",
		"created_by":     "ceo",
		"task_type":      "bugfix",
		"pipeline_id":    "bugfix",
		"execution_mode": "external_workspace",
		"workspace_path": workspaceRoot,
	})

	task := create["task"].(map[string]any)
	taskID := task["id"].(string)

	postJSON("/tasks", map[string]any{
		"action":     "block",
		"id":         taskID,
		"channel":    "general",
		"created_by": "ceo",
		"details": structuredTaskHandoff(
			"blocked",
			"Bloqueio temporario para validar resume por reassign.",
			"Retomar o mesmo task assim que o reassign recolocar a execucao em andamento.",
			`## Blockers
Kind: clarification
Question: Quem deve retomar a execucao deste slice agora?
Waiting On: office
Need: Confirmacao do reassign para continuar.
Context: Este bloqueio existe apenas para validar que o reassign limpa o flag blocked.
`,
			"",
		),
	})

	reassigned := postJSON("/tasks", map[string]any{
		"action":         "reassign",
		"id":             taskID,
		"channel":        "general",
		"owner":          "builder",
		"created_by":     "ceo",
		"execution_mode": "external_workspace",
		"workspace_path": workspaceRoot,
		"details":        "Reassign deve limpar o blocked para destravar a execucao.",
	})

	got := reassigned["task"].(map[string]any)
	if got["status"] != "in_progress" {
		t.Fatalf("status = %v, want in_progress", got["status"])
	}
	if blocked, _ := got["blocked"].(bool); blocked {
		t.Fatalf("blocked = true, want false after reassign")
	}
}

func TestHandleTaskAssignAllowsNonGitExternalWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	oldPathFn := brokerStatePath
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	workspaceRoot := filepath.Join(tmpDir, ".wuphf", "cache", "external-working-directory")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatalf("ensure external workspace dir: %v", err)
	}

	b := NewBroker()
	b.channels = []teamChannel{{Slug: "general", Members: []string{"ceo", "builder"}}}
	b.members = []officeMember{{Slug: "ceo", Name: "CEO"}, {Slug: "builder", Name: "Builder"}}
	b.tasks = append(b.tasks, teamTask{
		ID:            "task-1",
		Channel:       "general",
		Title:         "Audit external workspace preserve path",
		Status:        "open",
		CreatedBy:     "ceo",
		ExecutionMode: "external_workspace",
		WorkspacePath: workspaceRoot,
	})
	if err := b.syncTaskWorktreeLocked(&b.tasks[0]); err != nil {
		t.Fatalf("initial external workspace sync: %v", err)
	}

	body, err := json.Marshal(map[string]any{
		"action":     "assign",
		"id":         "task-1",
		"channel":    "general",
		"owner":      "builder",
		"created_by": "ceo",
	})
	if err != nil {
		t.Fatalf("marshal assign: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+b.Token())
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	b.requireAuth(b.handlePostTask)(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("assign status = %d: %s", resp.StatusCode, raw)
	}

	var out struct {
		Task teamTask `json:"task"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode assign: %v", err)
	}
	if out.Task.ExecutionMode != "external_workspace" {
		t.Fatalf("execution mode = %q, want external_workspace", out.Task.ExecutionMode)
	}
	if out.Task.WorkspacePath != workspaceRoot {
		t.Fatalf("workspace_path = %q, want %q", out.Task.WorkspacePath, workspaceRoot)
	}
	if out.Task.WorktreePath != "" || out.Task.WorktreeBranch != "" {
		t.Fatalf("expected no managed worktree for external workspace, got %+v", out.Task)
	}
	if out.Task.Status != "in_progress" || out.Task.Owner != "builder" {
		t.Fatalf("unexpected assigned task state: %+v", out.Task)
	}
}

func TestHandleTaskCreatePersistsRuntimeOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	oldPathFn := brokerStatePath
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	if err := b.StartOnPort(0); err != nil {
		t.Fatal(err)
	}
	defer b.Stop()

	post := func(payload map[string]any) (*http.Response, map[string]any) {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		req, err := http.NewRequest(http.MethodPost, "http://"+b.Addr()+"/tasks", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+b.Token())
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("post /tasks: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			return resp, nil
		}
		defer resp.Body.Close()
		var out map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode /tasks: %v", err)
		}
		return resp, out
	}

	resp, out := post(map[string]any{
		"action":           "create",
		"channel":          "general",
		"title":            "Implement runtime routing",
		"details":          "Validate task-scoped runtime selection.",
		"owner":            "builder",
		"created_by":       "ceo",
		"runtime_provider": "codex",
		"runtime_model":    "gpt-5.5",
		"reasoning_effort": "high",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create status = %d", resp.StatusCode)
	}
	task := out["task"].(map[string]any)
	if task["runtime_provider"] != "codex" {
		t.Fatalf("runtime_provider = %v, want codex", task["runtime_provider"])
	}
	if task["runtime_model"] != "gpt-5.5" {
		t.Fatalf("runtime_model = %v, want gpt-5.5", task["runtime_model"])
	}
	if task["reasoning_effort"] != "high" {
		t.Fatalf("reasoning_effort = %v, want high", task["reasoning_effort"])
	}

	b.mu.Lock()
	persisted := b.tasks[0]
	b.mu.Unlock()
	if persisted.RuntimeProvider != "codex" || persisted.RuntimeModel != "gpt-5.5" || persisted.ReasoningEffort != "high" {
		t.Fatalf("runtime overrides not persisted: %+v", persisted)
	}
}

func TestHandleTaskOutcomeAndQueueFields(t *testing.T) {
	tmpDir := t.TempDir()
	oldPathFn := brokerStatePath
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	if err := b.StartOnPort(0); err != nil {
		t.Fatal(err)
	}
	defer b.Stop()

	post := func(payload map[string]any) teamTask {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		req, err := http.NewRequest(http.MethodPost, "http://"+b.Addr()+"/tasks", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+b.Token())
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("post /tasks: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("post /tasks status = %d", resp.StatusCode)
		}
		var out struct {
			Task teamTask `json:"task"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode /tasks: %v", err)
		}
		return out.Task
	}

	created := post(map[string]any{
		"action":     "create",
		"channel":    "general",
		"title":      "Ship outcome contract",
		"details":    "Expose outcome metadata to the operator surfaces.",
		"owner":      "builder",
		"created_by": "ceo",
		"outcome":    "Operator sees the expected result before closing the task.",
	})
	if created.OutcomeStatus != "pending" {
		t.Fatalf("created outcome_status = %q, want pending", created.OutcomeStatus)
	}
	if created.QueueKey == "" {
		t.Fatalf("expected derived queue key on created task: %+v", created)
	}

	updated := post(map[string]any{
		"action":           "update_outcome",
		"channel":          "general",
		"id":               created.ID,
		"created_by":       "ceo",
		"outcome_evidence": "go test ./internal/team -run TestHandleTaskOutcomeAndQueueFields",
	})
	if updated.OutcomeStatus != "verified" {
		t.Fatalf("updated outcome_status = %q, want verified", updated.OutcomeStatus)
	}
	if updated.OutcomeEvidence == "" || updated.OutcomeVerifiedAt == "" {
		t.Fatalf("expected evidence and verified timestamp, got %+v", updated)
	}

	withArtifact := post(map[string]any{
		"action":           "add_artifact",
		"channel":          "general",
		"id":               created.ID,
		"created_by":       "ceo",
		"artifact_kind":    "document",
		"artifact_title":   "Outcome contract test",
		"artifact_path":    "D:\\Reports\\outcome-contract.md",
		"artifact_summary": "Shows the recorded work product for this task.",
	})
	if len(withArtifact.Artifacts) != 1 {
		t.Fatalf("expected artifact to be appended, got %+v", withArtifact.Artifacts)
	}
	if withArtifact.Artifacts[0].Kind != "document" || withArtifact.Artifacts[0].Path == "" {
		t.Fatalf("unexpected artifact payload: %+v", withArtifact.Artifacts[0])
	}
	withBrowserArtifact := post(map[string]any{
		"action":     "add_artifact",
		"channel":    "general",
		"id":         created.ID,
		"created_by": "ceo",
		"artifact": map[string]any{
			"kind":  "browser_inspection",
			"title": "Checkout inspection",
			"browser_inspection": map[string]any{
				"page_url":        "http://localhost:7891/#/channels/general",
				"selector":        "[data-testid=\"checkout\"]",
				"element_text":    "Finalizar",
				"screenshot_path": "D:\\tmp\\checkout.png",
				"viewport_width":  390,
				"viewport_height": 844,
			},
		},
	})
	if len(withBrowserArtifact.Artifacts) != 2 {
		t.Fatalf("expected browser artifact to be appended, got %+v", withBrowserArtifact.Artifacts)
	}
	browserArtifact := withBrowserArtifact.Artifacts[1]
	if browserArtifact.Kind != "browser_inspection" || browserArtifact.ResultRole != "evidence" || browserArtifact.BrowserInspection == nil {
		t.Fatalf("unexpected browser artifact payload: %+v", browserArtifact)
	}
	if browserArtifact.URL == "" || browserArtifact.Path == "" || !strings.Contains(browserArtifact.Summary, "selector=") {
		t.Fatalf("expected browser artifact to package url, screenshot, and selector summary: %+v", browserArtifact)
	}

	withPlan := post(map[string]any{
		"action":       "save_plan_revision",
		"channel":      "general",
		"id":           created.ID,
		"created_by":   "ceo",
		"plan_summary": "Add outcome contract",
		"plan_content": "1. Add task outcome metadata\n2. Surface the fields\n3. Validate the contract",
	})
	if len(withPlan.PlanRevisions) != 1 {
		t.Fatalf("expected plan revision to be appended, got %+v", withPlan.PlanRevisions)
	}
	if withPlan.PlanRevisions[0].Version != 1 || withPlan.PlanRevisions[0].Summary != "Add outcome contract" {
		t.Fatalf("unexpected plan revision: %+v", withPlan.PlanRevisions[0])
	}

	withLimits := post(map[string]any{
		"action":       "update_limits",
		"channel":      "general",
		"id":           created.ID,
		"created_by":   "ceo",
		"max_attempts": 1,
	})
	if withLimits.Limits.MaxAttempts != 1 || withLimits.Limits.LimitState != "ok" {
		t.Fatalf("expected saved task limits, got %+v", withLimits.Limits)
	}
	afterAttempt := post(map[string]any{
		"action":     "record_attempt",
		"channel":    "general",
		"id":         created.ID,
		"created_by": "ceo",
	})
	if afterAttempt.Limits.AttemptsUsed != 1 || afterAttempt.Limits.LimitState != "exhausted" || afterAttempt.Status != "blocked" {
		t.Fatalf("expected attempt to exhaust and block task, got status=%s limits=%+v", afterAttempt.Status, afterAttempt.Limits)
	}

	withFeedback := post(map[string]any{
		"action":           "add_feedback",
		"channel":          "general",
		"id":               created.ID,
		"created_by":       "ceo",
		"feedback_rating":  "up",
		"feedback_comment": "Outcome controls are useful.",
	})
	if len(withFeedback.Feedback) != 1 || withFeedback.Feedback[0].Rating != "up" {
		t.Fatalf("expected task feedback, got %+v", withFeedback.Feedback)
	}

	readTask := post(map[string]any{
		"action":     "mark_read",
		"channel":    "general",
		"id":         created.ID,
		"created_by": "ceo",
	})
	if readTask.ReadAt == "" {
		t.Fatalf("expected read_at after mark_read, got %+v", readTask)
	}
	archivedTask := post(map[string]any{
		"action":     "archive_inbox",
		"channel":    "general",
		"id":         created.ID,
		"created_by": "ceo",
	})
	if archivedTask.ArchivedAt == "" {
		t.Fatalf("expected archived_at after archive_inbox, got %+v", archivedTask)
	}

	b.mu.RLock()
	snapshots := studioTaskSnapshotsFromTasks(b.tasks)
	deliveries := b.buildDeliveriesLocked("general", false, true, true)
	b.mu.RUnlock()
	if len(snapshots) != 1 {
		t.Fatalf("expected one studio task snapshot, got %+v", snapshots)
	}
	if snapshots[0].OutcomeStatus != "verified" || snapshots[0].QueueKey == "" || snapshots[0].ArtifactCount != 2 || snapshots[0].PlanRevisionCount != 1 {
		t.Fatalf("expected studio snapshot outcome and queue fields, got %+v", snapshots[0])
	}
	foundArtifact := false
	for _, delivery := range deliveries {
		for _, artifact := range delivery.Artifacts {
			if artifact.Path == "D:\\Reports\\outcome-contract.md" {
				foundArtifact = true
			}
		}
	}
	if !foundArtifact {
		t.Fatalf("expected delivery view to include explicit task artifact, got %+v", deliveries)
	}
}

func TestHandleTaskCompleteRequiresOutcomeEvidence(t *testing.T) {
	tmpDir := t.TempDir()
	oldPathFn := brokerStatePath
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	if err := b.StartOnPort(0); err != nil {
		t.Fatal(err)
	}
	defer b.Stop()

	post := func(payload map[string]any) (*http.Response, []byte) {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		req, err := http.NewRequest(http.MethodPost, "http://"+b.Addr()+"/tasks", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+b.Token())
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("post /tasks: %v", err)
		}
		raw, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatalf("read /tasks response: %v", err)
		}
		return resp, raw
	}

	resp, raw := post(map[string]any{
		"action":         "create",
		"channel":        "general",
		"title":          "Research delivery contract",
		"details":        "Validate that completion requires evidence.",
		"owner":          "builder",
		"created_by":     "human",
		"task_type":      "research",
		"execution_mode": "office",
		"outcome":        "A decision note states whether the contract is ready.",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create status = %d: %s", resp.StatusCode, raw)
	}
	var created struct {
		Task teamTask `json:"task"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	resp, raw = post(map[string]any{
		"action":     "complete",
		"channel":    "general",
		"id":         created.Task.ID,
		"created_by": "human",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("complete without evidence status = %d, want 409: %s", resp.StatusCode, raw)
	}
	if !strings.Contains(string(raw), "Completion requires outcome evidence") {
		t.Fatalf("expected evidence guidance, got %s", raw)
	}

	foundBlockedAudit := false
	for _, action := range b.Actions() {
		if action.Kind == "task_completion_blocked" && action.RelatedID == created.Task.ID {
			foundBlockedAudit = true
			break
		}
	}
	if !foundBlockedAudit {
		t.Fatalf("expected task_completion_blocked audit event for %s", created.Task.ID)
	}

	resp, raw = post(map[string]any{
		"action":           "complete",
		"channel":          "general",
		"id":               created.Task.ID,
		"created_by":       "human",
		"outcome_evidence": "Reviewed research note and accepted the recommendation.",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("complete with evidence status = %d: %s", resp.StatusCode, raw)
	}
	var completed struct {
		Task teamTask `json:"task"`
	}
	if err := json.Unmarshal(raw, &completed); err != nil {
		t.Fatalf("decode complete response: %v", err)
	}
	if completed.Task.Status != "done" || completed.Task.OutcomeStatus != "verified" {
		t.Fatalf("expected verified done task, got %+v", completed.Task)
	}
	foundCompletedAudit := false
	for _, action := range b.Actions() {
		if action.Kind == "task_completed" && action.RelatedID == created.Task.ID {
			foundCompletedAudit = true
			break
		}
	}
	if !foundCompletedAudit {
		t.Fatalf("expected task_completed audit event for %s", created.Task.ID)
	}

	resp, raw = post(map[string]any{
		"action":       "save_plan_revision",
		"channel":      "general",
		"id":           created.Task.ID,
		"created_by":   "human",
		"plan_content": "1. Inspect the evidence\n2. Record the reusable decision\n3. Promote the pattern if it repeats",
		"plan_summary": "Capture reusable decision path",
		"plan_status":  "ready",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save plan status = %d: %s", resp.StatusCode, raw)
	}

	resp, raw = post(map[string]any{
		"action":     "approve_plan",
		"channel":    "general",
		"id":         created.Task.ID,
		"created_by": "human",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approve plan status = %d: %s", resp.StatusCode, raw)
	}
	var approved struct {
		PlanRevision taskPlanRevision `json:"plan_revision"`
	}
	if err := json.Unmarshal(raw, &approved); err != nil {
		t.Fatalf("decode approve plan response: %v", err)
	}
	if approved.PlanRevision.Status != "approved" || approved.PlanRevision.ApprovedBy != "human" {
		t.Fatalf("expected approved plan metadata, got %+v", approved.PlanRevision)
	}

	resp, raw = post(map[string]any{
		"action":     "promote_learning",
		"channel":    "general",
		"id":         created.Task.ID,
		"created_by": "human",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("promote learning status = %d: %s", resp.StatusCode, raw)
	}
	var promoted struct {
		Skill teamSkill `json:"skill"`
	}
	if err := json.Unmarshal(raw, &promoted); err != nil {
		t.Fatalf("decode promoted skill response: %v", err)
	}
	if promoted.Skill.PluginID != "dunderia-learning" || !stringSliceContains(promoted.Skill.Capabilities, "knowledge.reuse") {
		t.Fatalf("expected learning skill capabilities, got %+v", promoted.Skill)
	}

	req, err := http.NewRequest(http.MethodGet, "http://"+b.Addr()+"/tasks?channel=general&include_done=true", nil)
	if err != nil {
		t.Fatalf("new get request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+b.Token())
	getResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get /tasks: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get /tasks status = %d", getResp.StatusCode)
	}
	var listing struct {
		Tasks []teamTask `json:"tasks"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&listing); err != nil {
		t.Fatalf("decode /tasks listing: %v", err)
	}
	for _, task := range listing.Tasks {
		if task.ID != created.Task.ID {
			continue
		}
		if !task.CompletionEvidenceRequired || !task.CompletionEvidenceSatisfied {
			t.Fatalf("expected completion contract to be satisfied in operator listing, got %+v", task)
		}
		if task.QueueKey == "" || task.QueuePriority == "" || task.QueueReason == "" {
			t.Fatalf("expected queue contract in operator listing, got %+v", task)
		}
		if task.PlanStatus != "approved" || task.LatestPlanSummary == "" {
			t.Fatalf("expected approved planning contract in operator listing, got %+v", task)
		}
		if task.LearningCandidate == nil || !task.LearningCandidate.Recommended {
			t.Fatalf("expected learning candidate in operator listing, got %+v", task)
		}
		if len(task.GoalPath) == 0 || !strings.Contains(task.GoalSummary, "Resultado esperado") {
			t.Fatalf("expected goal path with outcome context, got path=%+v summary=%q", task.GoalPath, task.GoalSummary)
		}
		return
	}
	t.Fatalf("task %s not found in /tasks listing", created.Task.ID)
}

func TestPaperclipInspiredTaskOpsContracts(t *testing.T) {
	tmpDir := t.TempDir()
	oldPathFn := brokerStatePath
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	if err := b.StartOnPort(0); err != nil {
		t.Fatal(err)
	}
	defer b.Stop()

	req, err := http.NewRequest(http.MethodGet, "http://"+b.Addr()+"/task-templates", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+b.Token())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get task templates: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("task templates status = %d", resp.StatusCode)
	}
	var templates struct {
		Templates []taskTemplate `json:"templates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&templates); err != nil {
		t.Fatalf("decode task templates: %v", err)
	}
	if len(templates.Templates) == 0 || templates.Templates[0].Outcome == "" {
		t.Fatalf("expected task templates with outcomes, got %+v", templates.Templates)
	}
	foundReleaseTemplate := false
	for _, template := range templates.Templates {
		if template.ID == "release-pr-checklist" && strings.Contains(template.PlanContent, "rollback") {
			foundReleaseTemplate = true
			break
		}
	}
	if !foundReleaseTemplate {
		t.Fatalf("expected release-pr-checklist template, got %+v", templates.Templates)
	}

	b.mu.Lock()
	b.tasks = append(b.tasks, teamTask{
		ID:        "task-budget",
		Channel:   "general",
		Title:     "Budgeted run",
		Owner:     "builder",
		Status:    "in_progress",
		CreatedBy: "ceo",
		CreatedAt: "2026-04-29T00:00:00Z",
		UpdatedAt: "2026-04-29T00:00:00Z",
		Limits: taskExecutionLimits{
			MaxRuntimeMinutes: 1,
		},
	})
	b.mu.Unlock()

	b.UpdateAgentActivity(agentActivitySnapshot{
		Slug:           "builder",
		Channel:        "general",
		TotalMs:        61000,
		LivenessTaskID: "task-budget",
		LivenessAt:     "2026-04-29T00:01:01Z",
	})
	b.mu.RLock()
	budgetTask := b.tasks[0]
	b.mu.RUnlock()
	if budgetTask.Limits.RuntimeMsUsed != 61000 || budgetTask.Limits.LimitState != "exhausted" || budgetTask.Status != "blocked" {
		t.Fatalf("expected runtime budget to exhaust task, got status=%s limits=%+v", budgetTask.Status, budgetTask.Limits)
	}
	if len(budgetTask.Evals) == 0 {
		t.Fatalf("expected budget eval signal, got %+v", budgetTask)
	}

	if err := b.RecordAction("external_workflow_executed", "notion", "general", "builder", "Created external page", "workflow-1", nil, ""); err != nil {
		t.Fatalf("record action: %v", err)
	}
	actions := b.Actions()
	if len(actions) == 0 || !actions[len(actions)-1].RequiresApproval {
		t.Fatalf("expected sensitive action to be marked for governance, got %+v", actions)
	}
}

func TestPaperclipInspiredAdapterOrgAndCEOContracts(t *testing.T) {
	tmpDir := t.TempDir()
	oldPathFn := brokerStatePath
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "human", "Human")
	if err := b.StartOnPort(0); err != nil {
		t.Fatal(err)
	}
	defer b.Stop()

	doJSON := func(method, path string, payload any, out any) int {
		t.Helper()
		var body io.Reader
		if payload != nil {
			data, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			body = bytes.NewReader(data)
		}
		req, err := http.NewRequest(method, "http://"+b.Addr()+path, body)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+b.Token())
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		if out != nil {
			if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
				t.Fatalf("decode %s %s: %v", method, path, err)
			}
		}
		return resp.StatusCode
	}

	var adapters struct {
		Adapters []officeAdapter `json:"adapters"`
	}
	if status := doJSON(http.MethodGet, "/adapters", nil, &adapters); status != http.StatusOK {
		t.Fatalf("get adapters status = %d", status)
	}
	foundBroker := false
	for _, adapter := range adapters.Adapters {
		if adapter.ID == "local-broker" && stringSliceContains(adapter.Capabilities, "task.create") {
			foundBroker = true
			break
		}
	}
	if !foundBroker {
		t.Fatalf("expected builtin local-broker adapter, got %+v", adapters.Adapters)
	}

	var upserted struct {
		Adapter officeAdapter `json:"adapter"`
		Updated bool          `json:"updated"`
	}
	status := doJSON(http.MethodPost, "/adapters", map[string]any{
		"id":            "paperclip-style-workflow",
		"name":          "Paperclip style workflow",
		"kind":          "workflow",
		"provider":      "dunderia",
		"capabilities":  []string{"workflow.invoke", "artifact.record"},
		"health_status": "ready",
		"created_by":    "human",
	}, &upserted)
	if status != http.StatusOK {
		t.Fatalf("post adapter status = %d", status)
	}
	if upserted.Adapter.ID != "paperclip-style-workflow" || upserted.Updated {
		t.Fatalf("unexpected adapter response %+v", upserted)
	}

	var filtered struct {
		Adapters []officeAdapter `json:"adapters"`
	}
	if status := doJSON(http.MethodGet, "/adapters?capability=workflow.invoke", nil, &filtered); status != http.StatusOK {
		t.Fatalf("filtered adapters status = %d", status)
	}
	if len(filtered.Adapters) != 1 || filtered.Adapters[0].ID != "paperclip-style-workflow" {
		t.Fatalf("expected filtered custom adapter, got %+v", filtered.Adapters)
	}

	b.mu.RLock()
	memberCount := len(b.members)
	channelCount := len(b.channels)
	b.mu.RUnlock()

	var proposed struct {
		Proposal orgProposal `json:"proposal"`
	}
	status = doJSON(http.MethodPost, "/org-proposals", map[string]any{
		"action":          "propose",
		"kind":            "channel",
		"title":           "Criar canal para releases",
		"summary":         "Separar conversas de release do fluxo diario.",
		"proposed_by":     "human",
		"channel":         "general",
		"proposed_change": "create channel releases",
	}, &proposed)
	if status != http.StatusOK {
		t.Fatalf("post org proposal status = %d", status)
	}
	if !proposed.Proposal.RequiresTopologyAuthorization {
		t.Fatalf("expected topology authorization flag, got %+v", proposed.Proposal)
	}
	status = doJSON(http.MethodPost, "/org-proposals", map[string]any{
		"action": "approve",
		"id":     proposed.Proposal.ID,
		"actor":  "human",
	}, &proposed)
	if status != http.StatusOK || proposed.Proposal.Status != "approved" {
		t.Fatalf("approve proposal status=%d proposal=%+v", status, proposed.Proposal)
	}
	b.mu.RLock()
	if len(b.members) != memberCount || len(b.channels) != channelCount {
		t.Fatalf("approval mutated protected topology: members %d->%d channels %d->%d", memberCount, len(b.members), channelCount, len(b.channels))
	}
	b.mu.RUnlock()

	var convertedDecision struct {
		Kind     string               `json:"kind"`
		Decision officeDecisionRecord `json:"decision"`
	}
	status = doJSON(http.MethodPost, "/ceo/convert", map[string]any{
		"kind":              "decision",
		"channel":           "general",
		"created_by":        "human",
		"source_message_id": "msg-ceo-1",
		"title":             "Usar checklist forte em releases",
		"details":           "Registrar riscos, rollback e validacao.",
	}, &convertedDecision)
	if status != http.StatusOK || convertedDecision.Decision.Kind != "ceo_conversation" {
		t.Fatalf("convert decision status=%d response=%+v", status, convertedDecision)
	}

	var convertedTask struct {
		Kind string   `json:"kind"`
		Task teamTask `json:"task"`
	}
	status = doJSON(http.MethodPost, "/ceo/convert", map[string]any{
		"kind":       "task",
		"channel":    "general",
		"created_by": "human",
		"title":      "Preparar nota operacional semanal",
		"details":    "Consolidar decisoes e pendencias em um resumo revisavel.",
	}, &convertedTask)
	if status != http.StatusOK || convertedTask.Task.ID == "" || convertedTask.Task.TaskType != "follow_up" {
		t.Fatalf("convert task status=%d response=%+v", status, convertedTask)
	}
}

func TestPaperclipInspiredAtomicResultRoutineAndActivityContracts(t *testing.T) {
	tmpDir := t.TempDir()
	oldPathFn := brokerStatePath
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "human", "Human")
	if err := b.StartOnPort(0); err != nil {
		t.Fatal(err)
	}
	defer b.Stop()

	doJSON := func(method, path string, payload any, out any) int {
		t.Helper()
		var body io.Reader
		if payload != nil {
			data, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			body = bytes.NewReader(data)
		}
		req, err := http.NewRequest(method, "http://"+b.Addr()+path, body)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+b.Token())
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		if out != nil && resp.StatusCode < 400 {
			if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
				t.Fatalf("decode %s %s: %v", method, path, err)
			}
		}
		return resp.StatusCode
	}

	var created struct {
		Task teamTask `json:"task"`
	}
	status := doJSON(http.MethodPost, "/tasks", map[string]any{
		"action":     "create",
		"channel":    "general",
		"title":      "Prepare atomic execution demo",
		"created_by": "human",
		"owner":      "ceo",
		"task_type":  "release",
	}, &created)
	if status != http.StatusOK || created.Task.ID == "" {
		t.Fatalf("create task status=%d task=%+v", status, created.Task)
	}

	var locked struct {
		Task          teamTask           `json:"task"`
		ExecutionLock *taskExecutionLock `json:"execution_lock"`
	}
	status = doJSON(http.MethodPost, "/tasks", map[string]any{
		"action":           "acquire_execution_lock",
		"id":               created.Task.ID,
		"channel":          "general",
		"created_by":       "ceo",
		"run_id":           "run-1",
		"lock_ttl_seconds": 60,
	}, &locked)
	if status != http.StatusOK || locked.ExecutionLock == nil || locked.ExecutionLock.RunID != "run-1" {
		t.Fatalf("acquire lock status=%d response=%+v", status, locked)
	}
	status = doJSON(http.MethodPost, "/tasks", map[string]any{
		"action":     "acquire_execution_lock",
		"id":         created.Task.ID,
		"channel":    "general",
		"created_by": "human",
		"run_id":     "run-2",
	}, nil)
	if status != http.StatusConflict {
		t.Fatalf("expected conflicting lock acquisition, got status=%d", status)
	}
	status = doJSON(http.MethodPost, "/tasks", map[string]any{
		"action":     "heartbeat_execution_lock",
		"id":         created.Task.ID,
		"channel":    "general",
		"created_by": "ceo",
		"run_id":     "run-1",
	}, &locked)
	if status != http.StatusOK || locked.ExecutionLock == nil || locked.ExecutionLock.HeartbeatAt == "" {
		t.Fatalf("heartbeat lock status=%d response=%+v", status, locked)
	}
	status = doJSON(http.MethodPost, "/tasks", map[string]any{
		"action":     "release_execution_lock",
		"id":         created.Task.ID,
		"channel":    "general",
		"created_by": "ceo",
		"run_id":     "run-1",
	}, &locked)
	if status != http.StatusOK || locked.ExecutionLock == nil || locked.ExecutionLock.Status != "released" {
		t.Fatalf("release lock status=%d response=%+v", status, locked)
	}

	var artifactResp struct {
		Task     teamTask     `json:"task"`
		Artifact taskArtifact `json:"artifact"`
	}
	status = doJSON(http.MethodPost, "/tasks", map[string]any{
		"action":     "add_artifact",
		"id":         created.Task.ID,
		"channel":    "general",
		"created_by": "ceo",
		"artifact": map[string]any{
			"kind":        "preview",
			"result_role": "primary",
			"title":       "Release preview",
			"preview_url": "http://localhost:7891/preview/demo",
			"state":       "verified",
		},
	}, &artifactResp)
	if status != http.StatusOK || artifactResp.Artifact.ResultRole != "primary" || artifactResp.Artifact.ValidatedAt == "" {
		t.Fatalf("add artifact status=%d response=%+v", status, artifactResp)
	}

	if err := b.SetSchedulerJob(schedulerJob{
		Slug:              "routine-release",
		Kind:              "workflow",
		Label:             "Release routine",
		TargetType:        "workflow",
		TargetID:          "release",
		Channel:           "general",
		NextRun:           time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
		ConcurrencyPolicy: "skip_if_running",
		CatchUpPolicy:     "run_once",
		MaxParallel:       1,
		Status:            "scheduled",
	}); err != nil {
		t.Fatalf("set scheduler job: %v", err)
	}
	if err := b.UpdateSchedulerJobState("routine-release", time.Time{}, "running"); err != nil {
		t.Fatalf("mark scheduler running: %v", err)
	}
	b.mu.RLock()
	var runningJob schedulerJob
	for _, job := range b.scheduler {
		if job.Slug == "routine-release" {
			runningJob = job
			break
		}
	}
	b.mu.RUnlock()
	if runningJob.RunningCount != 1 || runningJob.LastStartedAt == "" {
		t.Fatalf("expected running scheduler metadata, got %+v", runningJob)
	}
	if due := b.DueSchedulerJobs(); len(due) != 0 {
		t.Fatalf("expected skip_if_running routine to be withheld from due list, got %+v", due)
	}
	if err := b.UpdateSchedulerJobState("routine-release", time.Now().Add(time.Minute), "scheduled"); err != nil {
		t.Fatalf("reschedule job: %v", err)
	}
	b.mu.Lock()
	b.actions = append(b.actions, officeActionLog{
		ID:        "action-paperclip-lab",
		Kind:      "task_updated",
		Source:    "office",
		Channel:   "paperclip-lab",
		Actor:     "ceo",
		Summary:   "Newest non-general activity",
		CreatedAt: time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
	})
	b.mu.Unlock()

	var latestActivity struct {
		Events []officeActivityEvent `json:"events"`
	}
	status = doJSON(http.MethodGet, "/activity?limit=1", nil, &latestActivity)
	if status != http.StatusOK || len(latestActivity.Events) != 1 || latestActivity.Events[0].Channel != "paperclip-lab" {
		t.Fatalf("expected empty channel query not to imply general filter, status=%d events=%+v", status, latestActivity.Events)
	}
	var generalActivity struct {
		Events []officeActivityEvent `json:"events"`
	}
	status = doJSON(http.MethodGet, "/activity?limit=1&channel=general", nil, &generalActivity)
	if status != http.StatusOK || len(generalActivity.Events) != 1 || generalActivity.Events[0].Channel != "general" {
		t.Fatalf("expected explicit channel filter before limit, status=%d events=%+v", status, generalActivity.Events)
	}

	var activity struct {
		Events []officeActivityEvent `json:"events"`
	}
	status = doJSON(http.MethodGet, "/activity?limit=50", nil, &activity)
	if status != http.StatusOK {
		t.Fatalf("activity status=%d", status)
	}
	foundWorkProduct := false
	foundRoutine := false
	for _, event := range activity.Events {
		if event.Type == "work_product" && event.RelatedID == created.Task.ID {
			foundWorkProduct = true
		}
		if event.Type == "routine" && event.ID == "routine-release" {
			foundRoutine = true
		}
	}
	if !foundWorkProduct || !foundRoutine {
		t.Fatalf("expected work_product and routine activity events, got %+v", activity.Events)
	}
}
