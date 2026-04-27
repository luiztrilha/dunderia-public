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
