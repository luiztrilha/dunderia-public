package team

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOperatorAlertsAggregateTaskSignalsAndScopeChannels(t *testing.T) {
	b := NewBroker()
	now := "2026-05-04T10:00:00Z"
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	ensureTestMemberAccess(b, "private-client", "private-agent", "Private Agent")
	b.tasks = []teamTask{{
		ID:                          "task-evidence",
		Channel:                     "general",
		Title:                       "Ship with evidence",
		Owner:                       "builder",
		Status:                      "review",
		CompletionEvidenceRequired:  true,
		CompletionEvidenceSatisfied: false,
		CompletionBlocker:           "Build output is missing.",
		CreatedAt:                   now,
		UpdatedAt:                   now,
	}, {
		ID:        "task-budget",
		Channel:   "general",
		Title:     "Retry bounded task",
		Owner:     "builder",
		Status:    "blocked",
		Blocked:   true,
		Limits:    taskExecutionLimits{LimitState: "exhausted", LastLimitReason: "attempt limit reached", LastAttemptAt: now},
		CreatedAt: now,
		UpdatedAt: now,
	}, {
		ID:                          "task-private",
		Channel:                     "private-client",
		Title:                       "Private task",
		Owner:                       "private-agent",
		Status:                      "review",
		CompletionEvidenceRequired:  true,
		CompletionEvidenceSatisfied: false,
		CreatedAt:                   now,
		UpdatedAt:                   now,
	}}
	b.requests = []humanInterview{{
		ID:        "request-1",
		Channel:   "general",
		Title:     "Approve run",
		Question:  "Can the run continue?",
		Status:    "pending",
		Blocking:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}}
	payload := b.buildOperatorAlertsLocked("human", "general", false, parseBrokerTimestamp(now))
	b.mu.Unlock()

	if payload.Persisted || payload.Summary["total"] == 0 || payload.Status == "ok" {
		t.Fatalf("expected read-only non-ok alerts, got %+v", payload)
	}
	ids := make([]string, 0, len(payload.Alerts))
	for _, alert := range payload.Alerts {
		ids = append(ids, alert.ID)
		if alert.RelatedID == "task-private" {
			t.Fatalf("private alert leaked to human: %+v", payload.Alerts)
		}
	}
	for _, want := range []string{"task:evidence:task-evidence", "task:budget:task-budget", "request:request-1"} {
		if !stringSliceContains(ids, want) {
			t.Fatalf("expected alert %q in ids %+v", want, ids)
		}
	}
}

func TestOperatorAlertsEndpointReturnsReadOnlyEnvelope(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{{
		ID:                          "task-evidence",
		Channel:                     "general",
		Title:                       "Ship with evidence",
		Owner:                       "builder",
		Status:                      "review",
		CompletionEvidenceRequired:  true,
		CompletionEvidenceSatisfied: false,
		CreatedAt:                   "2026-05-04T10:00:00Z",
		UpdatedAt:                   "2026-05-04T10:00:00Z",
	}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/operator/alerts?viewer_slug=human&channel=general", nil)
	b.handleOperatorAlerts(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload operatorAlertsResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Persisted || len(payload.Alerts) == 0 {
		t.Fatalf("expected read-only alerts envelope, got %+v", payload)
	}
}
