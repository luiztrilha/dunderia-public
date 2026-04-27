package team

import (
	"testing"
	"time"
)

func TestResolveRequestTransitionAnswerClearsLifecycle(t *testing.T) {
	now := time.Date(2026, 4, 23, 15, 4, 5, 0, time.UTC)
	req := humanInterview{
		ID:               "request-1",
		Kind:             "approval",
		Status:           "pending",
		Question:         "Ship now?",
		ReminderAt:       now.Add(time.Minute).Format(time.RFC3339),
		FollowUpAt:       now.Add(2 * time.Minute).Format(time.RFC3339),
		RecheckAt:        now.Add(3 * time.Minute).Format(time.RFC3339),
		DueAt:            now.Add(time.Minute).Format(time.RFC3339),
		NextEscalationAt: now.Add(time.Hour).Format(time.RFC3339),
	}
	answer := &interviewAnswer{ChoiceID: "approve", ChoiceText: "Approve", AnsweredAt: now.Format(time.RFC3339)}

	got, err := resolveRequestTransition(req, "answer", answer, now)
	if err != nil {
		t.Fatalf("resolve answer transition: %v", err)
	}
	if got.Status != "answered" || got.Answered == nil || got.Answered.ChoiceID != "approve" {
		t.Fatalf("unexpected answered request: %+v", got)
	}
	if got.DueAt != "" || got.ReminderAt != "" || got.FollowUpAt != "" || got.RecheckAt != "" || got.NextEscalationAt != "" {
		t.Fatalf("expected answered transition to clear lifecycle fields, got %+v", got)
	}
	if _, err := resolveRequestTransition(got, "cancel", nil, now); err == nil {
		t.Fatal("expected answered request to reject cancel transition")
	}
}

func TestResolveWatchdogTransitionRaiseAndResolve(t *testing.T) {
	now := time.Date(2026, 4, 23, 16, 0, 0, 0, time.UTC)
	alert, err := resolveWatchdogTransition(watchdogAlert{ID: "watchdog-1"}, "raise", "ceo", "runtime blocked", now)
	if err != nil {
		t.Fatalf("raise watchdog: %v", err)
	}
	if alert.Status != "active" || alert.Owner != "ceo" || alert.Summary != "runtime blocked" {
		t.Fatalf("unexpected raised alert: %+v", alert)
	}
	resolved, err := resolveWatchdogTransition(alert, "resolve", "", "", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("resolve watchdog: %v", err)
	}
	if resolved.Status != "resolved" || resolved.UpdatedAt == "" {
		t.Fatalf("unexpected resolved alert: %+v", resolved)
	}
}

func TestResolveSchedulerJobStateNormalizesTerminalLifecycle(t *testing.T) {
	now := time.Date(2026, 4, 23, 17, 0, 0, 0, time.UTC)
	job := schedulerJob{Slug: "job-1", Status: "scheduled", NextRun: now.Add(time.Minute).Format(time.RFC3339), DueAt: now.Add(time.Minute).Format(time.RFC3339)}

	done, err := resolveSchedulerJobState(job, "done", time.Time{}, now)
	if err != nil {
		t.Fatalf("resolve done state: %v", err)
	}
	if done.Status != "done" || done.NextRun != "" || done.DueAt != "" || done.LastRun == "" {
		t.Fatalf("unexpected done job: %+v", done)
	}

	rescheduled, err := resolveSchedulerJobState(job, "scheduled", now.Add(5*time.Minute), now)
	if err != nil {
		t.Fatalf("resolve scheduled state: %v", err)
	}
	if rescheduled.Status != "scheduled" || rescheduled.NextRun == "" || rescheduled.DueAt == "" {
		t.Fatalf("unexpected rescheduled job: %+v", rescheduled)
	}
}
