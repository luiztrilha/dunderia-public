package team

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
)

func TestHandleActionsPostRecordsAction(t *testing.T) {
	tmpDir := isolateBrokerPersistenceEnv(t)
	oldPathFn := brokerStatePath
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	if err := b.StartOnPort(0); err != nil {
		t.Fatal(err)
	}
	defer b.Stop()

	body, _ := json.Marshal(map[string]any{
		"kind":    "external_action_executed",
		"source":  "one",
		"channel": "general",
		"actor":   "ceo",
		"summary": "Sent a Gmail draft via One",
	})
	req, _ := http.NewRequest(http.MethodPost, "http://"+b.Addr()+"/actions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+b.Token())
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	actions := b.Actions()
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].Kind != "external_action_executed" || actions[0].Source != "one" {
		t.Fatalf("unexpected action %+v", actions[0])
	}
}

func TestHandleSchedulerPostRecordsWorkflowJob(t *testing.T) {
	tmpDir := isolateBrokerPersistenceEnv(t)
	oldPathFn := brokerStatePath
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	if err := b.StartOnPort(0); err != nil {
		t.Fatal(err)
	}
	defer b.Stop()

	body, _ := json.Marshal(map[string]any{
		"slug":          "one-workflow:general:daily-digest",
		"kind":          "one_workflow",
		"label":         "Run Daily Digest",
		"target_type":   "workflow",
		"target_id":     "daily-digest",
		"channel":       "general",
		"provider":      "one",
		"schedule_expr": "daily",
		"workflow_key":  "daily-digest",
		"next_run":      "2026-03-29T09:00:00Z",
		"due_at":        "2026-03-29T09:00:00Z",
		"status":        "scheduled",
	})
	req, _ := http.NewRequest(http.MethodPost, "http://"+b.Addr()+"/scheduler", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+b.Token())
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	jobs := b.Scheduler()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Kind != "one_workflow" || jobs[0].Provider != "one" || jobs[0].WorkflowKey != "daily-digest" || jobs[0].ScheduleExpr != "daily" {
		t.Fatalf("unexpected scheduler job %+v", jobs[0])
	}
}

func TestRecordActionKeepsUniqueIDsAfterTrim(t *testing.T) {
	tmpDir := isolateBrokerPersistenceEnv(t)
	oldPathFn := brokerStatePath
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	for i := 0; i < 160; i++ {
		if err := b.RecordAction("automation", "office", "general", "ceo", "summary", "msg-1", nil, ""); err != nil {
			t.Fatalf("record action %d: %v", i, err)
		}
	}

	actions := b.Actions()
	if len(actions) != 150 {
		t.Fatalf("expected trimmed 150 actions, got %d", len(actions))
	}
	seen := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		if _, ok := seen[action.ID]; ok {
			t.Fatalf("duplicate action id detected after trim: %q", action.ID)
		}
		seen[action.ID] = struct{}{}
	}
	if actions[len(actions)-1].ID != "action-160" {
		t.Fatalf("expected last action id action-160, got %q", actions[len(actions)-1].ID)
	}
}

func TestNormalizeLoadedStateReassignsDuplicateActionIDs(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	b := NewBroker()
	b.mu.Lock()
	b.actions = []officeActionLog{
		{ID: "action-151", Kind: "automation", Channel: "general", Actor: "ceo", Summary: "first", CreatedAt: "2026-04-19T20:00:00Z"},
		{ID: "action-151", Kind: "automation", Channel: "general", Actor: "ceo", Summary: "second", CreatedAt: "2026-04-19T20:01:00Z"},
		{ID: "", Kind: "automation", Channel: "general", Actor: "ceo", Summary: "third", CreatedAt: "2026-04-19T20:02:00Z"},
	}
	b.normalizeLoadedStateLocked()
	actions := append([]officeActionLog(nil), b.actions...)
	b.mu.Unlock()

	seen := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		if action.ID == "" {
			t.Fatal("expected normalized action id, got blank")
		}
		if _, ok := seen[action.ID]; ok {
			t.Fatalf("duplicate action id remained after normalization: %q", action.ID)
		}
		seen[action.ID] = struct{}{}
	}
}
