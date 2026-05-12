package team

import (
	"strings"
	"testing"
	"time"

	"github.com/nex-crm/wuphf/internal/agent"
)

func TestEnsureUnansweredMessageAuditJobRegistersRecurringJob(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	b := NewBroker()
	b.mu.Lock()
	b.messages = []channelMessage{{
		ID:        "msg-human",
		From:      "you",
		Channel:   "general",
		Content:   "@ceo preciso de resposta",
		Tagged:    []string{"ceo"},
		Timestamp: time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
	}}
	b.mu.Unlock()

	if err := b.EnsureUnansweredMessageAuditJob(); err != nil {
		t.Fatalf("EnsureUnansweredMessageAuditJob: %v", err)
	}

	var got schedulerJob
	for _, job := range b.Scheduler() {
		if job.Slug == unansweredMessageAuditJobSlug {
			got = job
			break
		}
	}
	if got.Slug == "" {
		t.Fatal("expected unanswered message audit scheduler job")
	}
	if got.Kind != unansweredMessageAuditJobKind || got.TargetType != unansweredMessageAuditTargetType {
		t.Fatalf("unexpected audit job %+v", got)
	}
	if got.IntervalMinutes != int(unansweredMessageAuditInterval/time.Minute) || got.Status != "scheduled" {
		t.Fatalf("unexpected audit job schedule %+v", got)
	}
}

func TestAuditUnansweredAgentMessagesCreatesWatchdogAndNudge(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	now := time.Date(2026, 5, 7, 15, 0, 0, 0, time.UTC)
	b := NewBroker()
	ensureTestMemberAccess(b, "pendencias-prioridades", "ceo", "CEO")
	b.mu.Lock()
	b.messages = []channelMessage{{
		ID:        "msg-human",
		From:      "you",
		Channel:   "pendencias-prioridades",
		Content:   "@ceo pode priorizar isso?",
		Tagged:    []string{"ceo"},
		Timestamp: now.Add(-unansweredMessageAuditMinAge - time.Minute).Format(time.RFC3339),
	}}
	b.mu.Unlock()

	l := &Launcher{broker: b, pack: agent.GetPack("founding-team")}
	report := l.auditUnansweredAgentMessages(now)

	if report.CreatedAlerts != 1 || report.NudgesPosted != 1 {
		t.Fatalf("expected one alert and one nudge, got %+v", report)
	}
	alerts := b.Watchdogs()
	if len(alerts) != 1 {
		t.Fatalf("expected one watchdog alert, got %+v", alerts)
	}
	if alerts[0].Kind != unansweredMessageWatchdogKind || alerts[0].Owner != "ceo" {
		t.Fatalf("unexpected watchdog alert %+v", alerts[0])
	}
	if alerts[0].TargetID != unansweredMessageWatchdogTargetID("ceo", "msg-human") {
		t.Fatalf("unexpected watchdog target id %q", alerts[0].TargetID)
	}

	foundNudge := false
	for _, msg := range b.AllMessages() {
		if msg.Kind != "automation" {
			continue
		}
		if msg.ReplyTo == "msg-human" && containsSlug(msg.Tagged, "ceo") && strings.Contains(msg.Content, "@ceo has not answered") {
			foundNudge = true
			break
		}
	}
	if !foundNudge {
		t.Fatalf("expected automation nudge tagged to ceo, got %+v", b.AllMessages())
	}
}

func TestAuditUnansweredAgentMessagesSkipsFreshMessage(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	now := time.Date(2026, 5, 7, 15, 0, 0, 0, time.UTC)
	b := NewBroker()
	ensureTestMemberAccess(b, "general", "ceo", "CEO")
	b.mu.Lock()
	b.messages = []channelMessage{{
		ID:        "msg-fresh",
		From:      "you",
		Channel:   "general",
		Content:   "@ceo acabou de chegar",
		Tagged:    []string{"ceo"},
		Timestamp: now.Add(-unansweredMessageAuditMinAge + time.Second).Format(time.RFC3339),
	}}
	b.mu.Unlock()

	l := &Launcher{broker: b, pack: agent.GetPack("founding-team")}
	report := l.auditUnansweredAgentMessages(now)

	if report.CreatedAlerts != 0 || report.NudgesPosted != 0 {
		t.Fatalf("expected fresh message to be skipped, got %+v", report)
	}
	if alerts := b.Watchdogs(); len(alerts) != 0 {
		t.Fatalf("expected no watchdog alerts, got %+v", alerts)
	}
}

func TestAuditUnansweredAgentMessagesSkipsHumanRequestAnswerReceipts(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	now := time.Date(2026, 5, 12, 14, 0, 0, 0, time.UTC)
	b := NewBroker()
	ensureTestMemberAccess(b, "convenios-web-vsazure", "ceo", "CEO")
	b.mu.Lock()
	b.messages = []channelMessage{
		{
			ID:        "msg-old-receipt",
			From:      "you",
			Channel:   "convenios-web-vsazure",
			Content:   "Answered @ceo's request: Answer directly",
			Tagged:    []string{"ceo"},
			Timestamp: now.Add(-10 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:        "msg-new-receipt",
			From:      "you",
			Channel:   "convenios-web-vsazure",
			Content:   "Human asked @ceo for more context: pause",
			Tagged:    []string{"ceo"},
			Timestamp: now.Add(-9 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:        "msg-rejected-receipt",
			From:      "you",
			Channel:   "convenios-web-vsazure",
			Content:   "Rejected @ceo's request.",
			Tagged:    []string{"ceo"},
			Timestamp: now.Add(-8 * time.Minute).Format(time.RFC3339),
		},
	}
	b.mu.Unlock()

	l := &Launcher{broker: b, pack: agent.GetPack("founding-team")}
	report := l.auditUnansweredAgentMessages(now)

	if report.CreatedAlerts != 0 || report.NudgesPosted != 0 {
		t.Fatalf("expected human decision receipts to be skipped, got %+v", report)
	}
	if alerts := b.Watchdogs(); len(alerts) != 0 {
		t.Fatalf("expected no watchdog alerts, got %+v", alerts)
	}
}

func TestAuditUnansweredAgentMessagesResolvesWhenAgentAnswers(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	now := time.Date(2026, 5, 7, 15, 0, 0, 0, time.UTC)
	b := NewBroker()
	ensureTestMemberAccess(b, "general", "ceo", "CEO")
	targetID := unansweredMessageWatchdogTargetID("ceo", "msg-human")
	if _, _, err := b.CreateWatchdogAlert(unansweredMessageWatchdogKind, "general", unansweredMessageWatchdogTargetType, targetID, "ceo", "pending"); err != nil {
		t.Fatalf("seed watchdog alert: %v", err)
	}
	b.mu.Lock()
	b.messages = []channelMessage{
		{
			ID:        "msg-human",
			From:      "you",
			Channel:   "general",
			Content:   "@ceo pode ver?",
			Tagged:    []string{"ceo"},
			Timestamp: now.Add(-10 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:        "msg-agent",
			From:      "ceo",
			Channel:   "general",
			Content:   "Estou vendo.",
			ReplyTo:   "msg-human",
			Timestamp: now.Add(-time.Minute).Format(time.RFC3339),
		},
	}
	b.mu.Unlock()

	l := &Launcher{broker: b, pack: agent.GetPack("founding-team")}
	report := l.auditUnansweredAgentMessages(now)

	if report.ResolvedAlerts != 1 {
		t.Fatalf("expected one resolved alert, got %+v", report)
	}
	alerts := b.Watchdogs()
	if len(alerts) != 1 || alerts[0].Status != "resolved" {
		t.Fatalf("expected watchdog alert to resolve, got %+v", alerts)
	}
}
