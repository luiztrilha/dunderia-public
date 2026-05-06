package team

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHermesRecallSearchPreviewIsScopedAndReadOnly(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.messages = []channelMessage{
		{
			ID:        "msg-release",
			From:      "human",
			Channel:   "general",
			Title:     "Release reminder",
			Content:   "Remember that release checks must run before publishing the desktop package.",
			Timestamp: "2026-05-04T10:00:00Z",
		},
		{
			ID:        "msg-private",
			From:      "human",
			Channel:   "private",
			Content:   "Private release process should stay hidden.",
			Timestamp: "2026-05-04T10:01:00Z",
		},
	}
	b.tasks = []teamTask{
		{
			ID:              "task-release",
			Channel:         "general",
			Title:           "Stabilize release workflow",
			Status:          "done",
			Owner:           "eng",
			Outcome:         "Release workflow stabilized",
			OutcomeEvidence: "release-check passed after validating package freshness",
			Artifacts: []taskArtifact{{
				ID:        "artifact-log",
				Kind:      "log",
				Title:     "Release check output",
				Summary:   "release-check passed",
				Path:      "artifacts/release-check.txt",
				CreatedAt: "2026-05-04T10:02:00Z",
			}},
			CreatedAt: "2026-05-04T09:00:00Z",
			UpdatedAt: "2026-05-04T10:03:00Z",
		},
		{
			ID:        "task-unrelated",
			Channel:   "general",
			Title:     "Clean workspace cache",
			Status:    "done",
			Owner:     "eng",
			Details:   "Remove stale temporary files after local validation.",
			CreatedAt: "2026-05-04T08:00:00Z",
			UpdatedAt: "2026-05-04T08:30:00Z",
		},
	}
	b.decisions = []officeDecisionRecord{{
		ID:        "decision-release",
		Kind:      "release_gate",
		Channel:   "general",
		Summary:   "Run release checks before publishing",
		Reason:    "The package should not ship without freshness validation.",
		Owner:     "ceo",
		CreatedAt: "2026-05-04T10:04:00Z",
	}}
	beforeMessages := len(b.messages)
	beforeTasks := len(b.tasks)
	beforeDecisions := len(b.decisions)
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/recall/search-preview?channel=general&viewer_slug=human&q=release", nil)
	b.handleRecallSearchPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload recallSearchPreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Persisted || len(payload.Results) < 4 {
		t.Fatalf("expected read-only recall hits across sources, got %+v", payload)
	}
	kinds := map[string]bool{}
	for _, result := range payload.Results {
		if result.Channel != "general" {
			t.Fatalf("expected scoped general results only, got %+v", payload.Results)
		}
		if result.Summary == "Private release process should stay hidden." {
			t.Fatalf("private result leaked into recall: %+v", result)
		}
		if len(result.Sources) == 0 {
			t.Fatalf("expected source references for result: %+v", result)
		}
		if result.Quality <= 0 || len(result.QualitySignals) == 0 {
			t.Fatalf("expected quality scoring for result: %+v", result)
		}
		if result.ID == "task:task-unrelated" {
			t.Fatalf("unrelated task should not be returned for release query: %+v", result)
		}
		if !stringSliceContains(result.RankSignals, "phrase_match") && !stringSliceContains(result.RankSignals, "title_match") && !stringSliceContains(result.RankSignals, "summary_match") {
			t.Fatalf("expected rank signal on result: %+v", result)
		}
		kinds[result.Kind] = true
	}
	for _, kind := range []string{"message", "task", "artifact", "decision"} {
		if !kinds[kind] {
			t.Fatalf("expected %s recall result, got kinds %+v in %+v", kind, kinds, payload.Results)
		}
	}

	b.mu.RLock()
	afterMessages := len(b.messages)
	afterTasks := len(b.tasks)
	afterDecisions := len(b.decisions)
	b.mu.RUnlock()
	if afterMessages != beforeMessages || afterTasks != beforeTasks || afterDecisions != beforeDecisions {
		t.Fatalf("recall preview mutated state: messages %d -> %d, tasks %d -> %d, decisions %d -> %d", beforeMessages, afterMessages, beforeTasks, afterTasks, beforeDecisions, afterDecisions)
	}
}

func TestHermesRecallSearchPreviewSupportsKnowledgeAndRiskSignals(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.skills = []teamSkill{{
		ID:          "skill-release",
		Name:        "release-playbook",
		Title:       "Release Playbook",
		Description: "Use release checks before publishing.",
		Content:     "Never store API_TOKEN values in the release playbook.",
		PluginID:    "dunderia-learning",
		Channel:     "general",
		Tags:        []string{"learning", "release"},
		Status:      "active",
		CreatedAt:   "2026-05-04T10:00:00Z",
		UpdatedAt:   "2026-05-04T10:01:00Z",
	}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/recall/search-preview?channel=general&viewer_slug=human&q=release&kind=knowledge", nil)
	b.handleRecallSearchPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload recallSearchPreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Results) != 1 || payload.Results[0].Kind != "knowledge" {
		t.Fatalf("expected one knowledge recall result, got %+v", payload.Results)
	}
	if !stringSliceContains(payload.Results[0].RiskSignals, "secret_like_content") {
		t.Fatalf("expected secret-like risk signal, got %+v", payload.Results[0])
	}
	if payload.Results[0].Quality >= 75 || !stringSliceContains(payload.Results[0].QualitySignals, "secret_risk_penalty") {
		t.Fatalf("expected quality penalty for risky knowledge, got %+v", payload.Results[0])
	}
	if payload.Summary["kind_knowledge"] != 1 || payload.Summary["risk_secret_like_content"] != 1 || payload.Summary["quality_low"] != 1 {
		t.Fatalf("expected knowledge and risk summary counts, got %+v", payload.Summary)
	}
}
