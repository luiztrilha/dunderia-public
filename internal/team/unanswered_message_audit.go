package team

import (
	"fmt"
	"strings"
	"time"
)

const (
	unansweredMessageAuditInterval   = 2 * time.Minute
	unansweredMessageAuditMinAge     = 2 * time.Minute
	unansweredMessageAuditJobSlug    = "unanswered-message-audit"
	unansweredMessageAuditJobKind    = "unanswered_message_audit"
	unansweredMessageAuditTargetType = "unanswered_message_audit"

	unansweredMessageWatchdogKind       = "agent_message_unanswered"
	unansweredMessageWatchdogTargetType = "message"
)

type unansweredMessageAuditReport struct {
	CreatedAlerts  int
	ResolvedAlerts int
	NudgesPosted   int
}

type unansweredMessageAuditTarget struct {
	Owner            string
	Channel          string
	PendingMessageID string
	ReplyToID        string
	Message          channelMessage
}

func (b *Broker) EnsureUnansweredMessageAuditJob() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.messages) == 0 {
		if b.schedulerJobCanceledLocked(unansweredMessageAuditJobSlug) {
			return nil
		}
		if b.removeSchedulerJobBySlugLocked(unansweredMessageAuditJobSlug) {
			return b.saveLocked()
		}
		return nil
	}

	nextRun := time.Now().UTC().Add(unansweredMessageAuditInterval).Format(time.RFC3339)
	desired := normalizeSchedulerJob(schedulerJob{
		Slug:            unansweredMessageAuditJobSlug,
		Kind:            unansweredMessageAuditJobKind,
		Label:           "Unanswered agent message audit",
		TargetType:      unansweredMessageAuditTargetType,
		TargetID:        "global",
		Channel:         "general",
		IntervalMinutes: int(unansweredMessageAuditInterval / time.Minute),
		NextRun:         nextRun,
		DueAt:           nextRun,
		Status:          "scheduled",
	})

	for i := range b.scheduler {
		if strings.TrimSpace(b.scheduler[i].Slug) != unansweredMessageAuditJobSlug {
			continue
		}
		current := normalizeSchedulerJob(b.scheduler[i])
		if strings.EqualFold(current.Status, "canceled") {
			return nil
		}
		changed := false
		if current.Kind != desired.Kind || current.Label != desired.Label || current.TargetType != desired.TargetType ||
			current.TargetID != desired.TargetID || current.Channel != desired.Channel || current.IntervalMinutes != desired.IntervalMinutes {
			current.Kind = desired.Kind
			current.Label = desired.Label
			current.TargetType = desired.TargetType
			current.TargetID = desired.TargetID
			current.Channel = desired.Channel
			current.IntervalMinutes = desired.IntervalMinutes
			changed = true
		}
		if current.Status == "" || strings.EqualFold(current.Status, "done") {
			current.Status = "scheduled"
			current.NextRun = desired.NextRun
			current.DueAt = desired.DueAt
			changed = true
		}
		if strings.TrimSpace(current.NextRun) == "" {
			current.NextRun = desired.NextRun
			changed = true
		}
		if strings.TrimSpace(current.DueAt) == "" {
			current.DueAt = current.NextRun
			changed = true
		}
		if changed {
			b.scheduler[i] = current
			return b.saveLocked()
		}
		return nil
	}

	if err := b.scheduleJobLocked(desired); err != nil {
		return err
	}
	return b.saveLocked()
}

func (l *Launcher) processDueUnansweredMessageAuditJob(job schedulerJob) {
	if l == nil || l.broker == nil {
		return
	}
	l.auditUnansweredAgentMessages(time.Now().UTC())
	interval := time.Duration(job.IntervalMinutes) * time.Minute
	if interval <= 0 {
		interval = unansweredMessageAuditInterval
	}
	nextRun := time.Now().UTC().Add(interval)
	_ = l.broker.UpdateSchedulerJobState(job.Slug, nextRun, "scheduled")
}

func (l *Launcher) auditUnansweredAgentMessages(now time.Time) unansweredMessageAuditReport {
	report := unansweredMessageAuditReport{}
	if l == nil || l.broker == nil {
		return report
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	allMessages := l.broker.AllMessages()
	report.ResolvedAlerts = l.broker.resolveAnsweredUnansweredMessageAlerts(allMessages, now)

	targets := l.collectUnansweredMessageAuditTargets(now)
	for _, target := range targets {
		targetID := unansweredMessageWatchdogTargetID(target.Owner, target.PendingMessageID)
		summary := formatUnansweredMessageWatchdogSummary(target)
		alert, existing, err := l.broker.CreateWatchdogAlert(unansweredMessageWatchdogKind, target.Channel, unansweredMessageWatchdogTargetType, targetID, target.Owner, summary)
		if err != nil || existing {
			continue
		}
		report.CreatedAlerts++
		signalIDs, decisionID := l.recordWatchdogLedger(target.Channel, unansweredMessageWatchdogKind, alert.ID, target.Owner, summary, target.PendingMessageID)
		_ = l.broker.RecordAction("watchdog_alert", "watchdog", target.Channel, "watchdog", truncate(summary, 140), alert.ID, signalIDs, decisionID)
		_, duplicate, _ := l.broker.PostAutomationMessage(
			"wuphf",
			target.Channel,
			"Unanswered agent message",
			summary,
			"watchdog-unanswered-message-"+targetID,
			"watchdog",
			"Office watchdog",
			[]string{target.Owner},
			target.ReplyToID,
		)
		if !duplicate {
			report.NudgesPosted++
		}
	}
	return report
}

func (l *Launcher) collectUnansweredMessageAuditTargets(now time.Time) []unansweredMessageAuditTarget {
	if l == nil || l.broker == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	officeSlugs := make(map[string]struct{})
	for _, member := range l.officeMembersSnapshot() {
		if strings.TrimSpace(member.Slug) != "" {
			officeSlugs[member.Slug] = struct{}{}
		}
	}
	inPack := func(slug string) bool {
		if len(officeSlugs) == 0 {
			return true
		}
		_, ok := officeSlugs[slug]
		return ok
	}

	lead := l.officeLeadSlug()
	humanMessages := l.broker.RecentHumanMessages(recentHumanMessageLimit)
	allMessages := l.broker.AllMessages()
	unanswered := findUnansweredMessages(humanMessages, allMessages)
	unanswered = collapseUnansweredMessagesByThread(unanswered, allMessages)

	messageByID := make(map[string]channelMessage, len(allMessages))
	for _, msg := range allMessages {
		id := strings.TrimSpace(msg.ID)
		if id != "" {
			messageByID[id] = msg
		}
	}

	targets := make([]unansweredMessageAuditTarget, 0, len(unanswered))
	seen := make(map[string]struct{})
	for _, pending := range unanswered {
		if !messageIsOldEnoughForUnansweredAudit(pending, now) {
			continue
		}
		if isHumanRequestAnswerReceipt(pending) {
			continue
		}
		channel := normalizeChannelSlug(pending.Channel)
		if channel == "" {
			channel = "general"
		}
		replyToID := threadRootMessageIDFromMap(pending, messageByID)
		if replyToID == "" {
			replyToID = strings.TrimSpace(pending.ID)
		}
		routed := pending
		routed.Channel = channel
		routed.ID = replyToID
		if rootMsg, ok := messageByID[replyToID]; ok {
			rootContent := strings.TrimSpace(rootMsg.Content)
			latestContent := strings.TrimSpace(pending.Content)
			switch {
			case rootContent != "" && latestContent != "" && rootContent != latestContent:
				routed.Content = fmt.Sprintf("Original ask: %s | Latest human follow-up: %s", rootContent, latestContent)
			case rootContent != "":
				routed.Content = rootContent
			}
		}

		for _, owner := range l.unansweredMessageAuditOwners(pending, inPack, lead) {
			key := strings.Join([]string{channel, owner, strings.TrimSpace(pending.ID)}, "|")
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			targets = append(targets, unansweredMessageAuditTarget{
				Owner:            owner,
				Channel:          channel,
				PendingMessageID: strings.TrimSpace(pending.ID),
				ReplyToID:        replyToID,
				Message:          routed,
			})
		}
	}
	return targets
}

func (l *Launcher) unansweredMessageAuditOwners(msg channelMessage, inPack func(string) bool, lead string) []string {
	channel := normalizeChannelSlug(msg.Channel)
	owners := make([]string, 0, len(msg.Tagged)+1)
	add := func(slug string) {
		slug = strings.TrimPrefix(strings.TrimSpace(slug), "@")
		if slug == "" || isHumanOrSystemSender(slug) {
			return
		}
		if inPack != nil && !inPack(slug) {
			return
		}
		if !l.messageCanWakeTarget(msg, slug) {
			return
		}
		if containsSlug(owners, slug) {
			return
		}
		owners = append(owners, slug)
	}

	if IsDMSlug(channel) {
		if target := DMTargetAgent(channel); target != "" {
			add(target)
			return owners
		}
	}
	if isDM, target := l.isChannelDM(channel); isDM {
		if target != "" {
			add(target)
			return owners
		}
	}
	if len(msg.Tagged) > 0 {
		for _, tag := range msg.Tagged {
			add(tag)
		}
		return owners
	}
	if lead != "" {
		add(lead)
	}
	return owners
}

func (b *Broker) resolveAnsweredUnansweredMessageAlerts(allMessages []channelMessage, now time.Time) int {
	if b == nil {
		return 0
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	resolved := 0
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.watchdogs {
		alert := &b.watchdogs[i]
		if alert.Kind != unansweredMessageWatchdogKind || alert.TargetType != unansweredMessageWatchdogTargetType {
			continue
		}
		if normalizeWatchdogStatus(alert.Status) == "resolved" {
			continue
		}
		_, pendingID, ok := parseUnansweredMessageWatchdogTargetID(alert.TargetID)
		if !ok || !messageAnsweredByAgent(pendingID, allMessages) {
			continue
		}
		updated, err := resolveWatchdogTransition(*alert, "resolve", "", "", now)
		if err != nil {
			continue
		}
		*alert = updated
		resolved++
	}
	if resolved == 0 {
		return 0
	}
	if err := b.saveLocked(); err != nil {
		return 0
	}
	return resolved
}

func unansweredMessageWatchdogTargetID(owner, pendingMessageID string) string {
	return strings.TrimSpace(owner) + "|" + strings.TrimSpace(pendingMessageID)
}

func parseUnansweredMessageWatchdogTargetID(targetID string) (string, string, bool) {
	parts := strings.SplitN(strings.TrimSpace(targetID), "|", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func messageAnsweredByAgent(messageID string, allMessages []channelMessage) bool {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return false
	}
	messageByID := make(map[string]channelMessage, len(allMessages))
	for _, msg := range allMessages {
		id := strings.TrimSpace(msg.ID)
		if id != "" {
			messageByID[id] = msg
		}
	}
	pending, hasPending := messageByID[messageID]
	pendingRoot := ""
	pendingAt := time.Time{}
	if hasPending {
		pendingRoot = threadRootMessageIDFromMap(pending, messageByID)
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(pending.Timestamp)); err == nil {
			pendingAt = parsed
		}
	}
	for _, msg := range allMessages {
		if isHumanOrSystemSender(msg.From) {
			continue
		}
		replyTo := strings.TrimSpace(msg.ReplyTo)
		if replyTo == messageID {
			return true
		}
		if pendingRoot == "" || pendingAt.IsZero() || replyTo == "" {
			continue
		}
		replyAt, err := time.Parse(time.RFC3339, strings.TrimSpace(msg.Timestamp))
		if err != nil || !replyAt.After(pendingAt) {
			continue
		}
		if threadRootMessageIDFromMap(msg, messageByID) == pendingRoot {
			return true
		}
	}
	return false
}

func messageIsOldEnoughForUnansweredAudit(msg channelMessage, now time.Time) bool {
	timestamp := strings.TrimSpace(msg.Timestamp)
	if timestamp == "" {
		return false
	}
	createdAt, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return false
	}
	return !createdAt.After(now) && now.Sub(createdAt) >= unansweredMessageAuditMinAge
}

func isHumanRequestAnswerReceipt(msg channelMessage) bool {
	from := strings.ToLower(strings.TrimSpace(msg.From))
	if from != "you" && from != "human" {
		return false
	}
	content := strings.TrimSpace(msg.Content)
	return (strings.HasPrefix(content, "Answered @") && strings.Contains(content, "'s request")) ||
		(strings.HasPrefix(content, "Rejected @") && strings.Contains(content, "'s request")) ||
		strings.HasPrefix(content, "Human asked @") ||
		strings.HasPrefix(content, "Human replied to @") ||
		strings.HasPrefix(content, "Human chose to answer @")
}

func formatUnansweredMessageWatchdogSummary(target unansweredMessageAuditTarget) string {
	channel := normalizeChannelSlug(target.Channel)
	if channel == "" {
		channel = "general"
	}
	content := strings.TrimSpace(target.Message.Content)
	if content == "" {
		content = strings.TrimSpace(target.PendingMessageID)
	}
	return fmt.Sprintf("@%s has not answered a pending message in #%s: %s", target.Owner, channel, truncate(content, 180))
}
