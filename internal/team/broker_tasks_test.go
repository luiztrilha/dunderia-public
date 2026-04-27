package team

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
)

func TestHandleTaskReassignClearsBlockedFlag(t *testing.T) {
	tmpDir := t.TempDir()
	oldPathFn := brokerStatePath
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	workspaceRoot := filepath.Join(tmpDir, "LegacySystemOld")
	initUsableGitWorktree(t, workspaceRoot)

	b := NewBroker()
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
