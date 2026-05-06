package team

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBudgetContextPreviewShowsWouldBlockAndReadOnly(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{
		{
			ID:        "task-budget",
			Channel:   "general",
			Title:     "Budgeted run",
			Owner:     "eng",
			Status:    "in_progress",
			Details:   strings.Repeat("context ", 300),
			CreatedAt: "2026-05-04T10:00:00Z",
			UpdatedAt: "2026-05-04T10:02:00Z",
			Limits: taskExecutionLimits{
				MaxAttempts:   3,
				AttemptsUsed:  3,
				MaxCostCents:  200,
				CostCentsUsed: 150,
			},
		},
		{
			ID:              "task-private",
			Channel:         "private",
			Title:           "Private task",
			Status:          "in_progress",
			OutcomeEvidence: "private evidence",
			Limits:          taskExecutionLimits{MaxAttempts: 1, AttemptsUsed: 1},
		},
	}
	b.messages = []channelMessage{{
		ID:        "msg-budget",
		Channel:   "general",
		From:      "eng",
		Content:   "task-budget has a lot of execution context",
		Timestamp: "2026-05-04T10:01:00Z",
	}}
	b.usage = teamUsageState{
		Total:  usageTotals{TotalTokens: 1200, Requests: 3, CostUsd: 0.42},
		Agents: map[string]usageTotals{"eng": {TotalTokens: 1200, Requests: 3}},
	}
	beforeTasks := len(b.tasks)
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/budget/context-preview?channel=general&viewer_slug=human", nil)
	b.handleBudgetContextPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload budgetContextPreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Persisted || payload.Status != "blocked" || len(payload.Items) != 1 {
		t.Fatalf("expected one blocked read-only preview, got persisted=%v status=%q items=%+v", payload.Persisted, payload.Status, payload.Items)
	}
	item := payload.Items[0]
	if item.TaskID != "task-budget" || !item.WouldBlock || item.BudgetState != "exhausted" {
		t.Fatalf("expected exhausted budget item, got %+v", item)
	}
	if payload.Usage.TotalTokens != 1200 || payload.Usage.AgentCount != 1 {
		t.Fatalf("expected usage summary, got %+v", payload.Usage)
	}
	if item.Context.MessageCount == 0 || !stringSliceContains(item.Context.Signals, "recent_messages") {
		t.Fatalf("expected context estimate with recent messages, got %+v", item.Context)
	}
	b.mu.RLock()
	afterTasks := len(b.tasks)
	b.mu.RUnlock()
	if afterTasks != beforeTasks {
		t.Fatalf("budget preview mutated tasks: %d -> %d", beforeTasks, afterTasks)
	}
}

func TestBudgetContextPreviewWarnsNearLimitAndUnbounded(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{
		{
			ID:        "task-warning",
			Channel:   "general",
			Title:     "Nearly out of runtime",
			Status:    "in_progress",
			CreatedAt: "2026-05-04T10:00:00Z",
			UpdatedAt: "2026-05-04T10:02:00Z",
			Limits: taskExecutionLimits{
				MaxRuntimeMinutes: 10,
				RuntimeMsUsed:     8 * int64(timeMinuteMs()),
			},
		},
		{
			ID:        "task-unbounded",
			Channel:   "general",
			Title:     "No limits yet",
			Status:    "open",
			CreatedAt: "2026-05-04T10:00:00Z",
			UpdatedAt: "2026-05-04T10:01:00Z",
		},
	}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/budget/context-preview?channel=general&viewer_slug=human", nil)
	b.handleBudgetContextPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload budgetContextPreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Status != "warning" || len(payload.Items) != 2 {
		t.Fatalf("expected warning preview with two items, got status=%q items=%+v", payload.Status, payload.Items)
	}
	byID := map[string]budgetContextPreviewItem{}
	for _, item := range payload.Items {
		byID[item.TaskID] = item
	}
	if byID["task-warning"].BudgetState != "warning" || !byID["task-warning"].WouldWarn {
		t.Fatalf("expected near-limit warning, got %+v", byID["task-warning"])
	}
	if byID["task-unbounded"].BudgetState != "unbounded" || byID["task-unbounded"].WouldBlock {
		t.Fatalf("expected unbounded advisory without block, got %+v", byID["task-unbounded"])
	}
}
