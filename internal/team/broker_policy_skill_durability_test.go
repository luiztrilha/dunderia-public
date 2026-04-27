package team

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestHandlePoliciesCreateDeduplicatesByRequestID(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	body := []byte(`{"source":"human_directed","rule":"never lose legacy incidents","request_id":"policy-create-1"}`)

	post := func() map[string]any {
		req := httptest.NewRequest(http.MethodPost, "/policies", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		b.handlePolicies(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return payload
	}

	first := post()
	second := post()
	if first["persisted"] != true {
		t.Fatalf("expected first response persisted=true, got %#v", first)
	}
	if second["persisted"] != true || second["duplicate"] != true {
		t.Fatalf("expected duplicate persisted response on retry, got %#v", second)
	}
	if got := len(b.ListPolicies()); got != 1 {
		t.Fatalf("expected one active policy, got %d", got)
	}
}

func TestHandlePostSkillDeduplicatesByRequestID(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	body := []byte(`{"name":"persist-skill","title":"Persist Skill","content":"keep durable audit trail","created_by":"you","request_id":"skill-create-1"}`)

	post := func() map[string]any {
		req := httptest.NewRequest(http.MethodPost, "/skills", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		b.handlePostSkill(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return payload
	}

	first := post()
	second := post()
	if first["persisted"] != true {
		t.Fatalf("expected first response persisted=true, got %#v", first)
	}
	if second["persisted"] != true || second["duplicate"] != true {
		t.Fatalf("expected duplicate persisted response on retry, got %#v", second)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.skills) != 1 {
		t.Fatalf("expected one skill after duplicate create retry, got %d", len(b.skills))
	}
}

func TestHandleInvokeSkillDeduplicatesByRequestID(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	b.mu.Lock()
	b.skills = append(b.skills, teamSkill{
		ID:        "skill-launch-ops-bootstrap",
		Name:      "launch-ops-bootstrap",
		Title:     "Bootstrap Automated Content Factory",
		Status:    "active",
		Channel:   "general",
		CreatedBy: "ceo",
	})
	b.mu.Unlock()

	body := []byte(`{"invoked_by":"you","channel":"general","request_id":"skill-invoke-1"}`)
	post := func() map[string]any {
		req := httptest.NewRequest(http.MethodPost, "/skills/launch-ops-bootstrap/invoke", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		b.handleInvokeSkill(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return payload
	}

	first := post()
	second := post()
	if first["persisted"] != true {
		t.Fatalf("expected first response persisted=true, got %#v", first)
	}
	if second["persisted"] != true || second["duplicate"] != true {
		t.Fatalf("expected duplicate persisted response on retry, got %#v", second)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.skills[0].UsageCount != 1 {
		t.Fatalf("expected usage count 1 after duplicate retry, got %d", b.skills[0].UsageCount)
	}
}
