package team

import (
	"fmt"
	"strings"
	"time"
)

const (
	ceoConversationFollowUpInterval      = 10 * time.Minute
	ceoConversationFollowUpJobSlug       = "ceo-conversation-follow-up-audit"
	ceoConversationFollowUpJobKind       = "conversation_follow_up_audit"
	ceoConversationFollowUpJobTargetType = "conversation_follow_up"
	ceoConversationFollowUpTaskPrefix    = "ceo-conversation-follow-up"
)

type ceoConversationFollowUpCandidate struct {
	ExecutionKey string
	Channel      string
	ThreadID     string
	Title        string
	Details      string
}

func (b *Broker) inboundFollowUpDetailsLocked(channel string, msg channelMessage) string {
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		channel = "general"
	}
	latestContent := truncate(strings.TrimSpace(msg.Content), 280)
	threadRootID := firstNonEmpty(b.threadRootFromMessageIDLocked(strings.TrimSpace(msg.ID)), strings.TrimSpace(msg.ID))
	rootContent := ""
	if rootMsg := b.findMessageByIDLocked(threadRootID); rootMsg != nil {
		rootContent = truncate(strings.TrimSpace(rootMsg.Content), 280)
	}
	switch {
	case rootContent != "" && latestContent != "" && rootContent != latestContent:
		return fmt.Sprintf("A stale inbound thread still needs a CEO answer in #%s.\n\nOriginal ask: %s\n\nLatest human follow-up: %s", channel, rootContent, latestContent)
	case latestContent != "":
		return fmt.Sprintf("A stale inbound thread still needs a CEO answer in #%s.\n\nPending message: %s", channel, latestContent)
	default:
		return fmt.Sprintf("A stale inbound thread still needs a CEO answer in #%s.", channel)
	}
}

func (b *Broker) EnsureCEOConversationFollowUpAuditJob() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	nextRun := time.Now().UTC().Add(ceoConversationFollowUpInterval).Format(time.RFC3339)
	desired := normalizeSchedulerJob(schedulerJob{
		Slug:            ceoConversationFollowUpJobSlug,
		Kind:            ceoConversationFollowUpJobKind,
		Label:           "CEO conversation follow-up audit",
		TargetType:      ceoConversationFollowUpJobTargetType,
		TargetID:        "ceo",
		Channel:         "general",
		IntervalMinutes: int(ceoConversationFollowUpInterval / time.Minute),
		NextRun:         nextRun,
		DueAt:           nextRun,
		Status:          "scheduled",
	})

	changed := false
	for i := range b.scheduler {
		if strings.TrimSpace(b.scheduler[i].Slug) != ceoConversationFollowUpJobSlug {
			continue
		}
		current := normalizeSchedulerJob(b.scheduler[i])
		if current.Kind != desired.Kind {
			current.Kind = desired.Kind
			changed = true
		}
		if current.Label != desired.Label {
			current.Label = desired.Label
			changed = true
		}
		if current.TargetType != desired.TargetType {
			current.TargetType = desired.TargetType
			changed = true
		}
		if current.TargetID != desired.TargetID {
			current.TargetID = desired.TargetID
			changed = true
		}
		if current.Channel != desired.Channel {
			current.Channel = desired.Channel
			changed = true
		}
		if current.IntervalMinutes != desired.IntervalMinutes {
			current.IntervalMinutes = desired.IntervalMinutes
			changed = true
		}
		if strings.EqualFold(strings.TrimSpace(current.Status), "canceled") {
			return nil
		}
		if strings.EqualFold(strings.TrimSpace(current.Status), "done") {
			current.Status = "scheduled"
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

	b.scheduler = append(b.scheduler, desired)
	return b.saveLocked()
}

func (b *Broker) SyncCEOConversationFollowUpTasks(now time.Time) (createdOrUpdated int, resolved int, err error) {
	if b == nil {
		return 0, 0, nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	candidates := b.collectCEOConversationFollowUpCandidatesLocked(now.Add(-ceoConversationFollowUpInterval))
	active := make(map[string]ceoConversationFollowUpCandidate, len(candidates))
	nowText := now.UTC().Format(time.RFC3339)

	for _, candidate := range candidates {
		active[candidate.ExecutionKey] = candidate
		if b.upsertCEOConversationFollowUpTaskLocked(candidate, nowText) {
			createdOrUpdated++
		}
	}

	resolved += b.resolveDuplicateCEOConversationFollowUpTasksLocked(nowText)

	for i := range b.tasks {
		task := &b.tasks[i]
		executionKey := normalizeExecutionKey(task.ExecutionKey)
		if !strings.HasPrefix(executionKey, ceoConversationFollowUpTaskPrefix+"|") {
			continue
		}
		if _, ok := active[executionKey]; ok {
			continue
		}
		if b.markCEOConversationFollowUpTaskDoneLocked(task, nowText) {
			resolved++
		}
	}

	orphanJobsResolved, orphanAlertsResolved := b.resolveOrphanedTaskArtifactsLocked(nowText)

	if createdOrUpdated == 0 && resolved == 0 && orphanJobsResolved == 0 && orphanAlertsResolved == 0 {
		return 0, 0, nil
	}
	if err := b.saveLocked(); err != nil {
		return createdOrUpdated, resolved, err
	}
	return createdOrUpdated, resolved, nil
}

func (b *Broker) resolveDuplicateCEOConversationFollowUpTasksLocked(now string) int {
	if b == nil {
		return 0
	}
	canonicalByExecutionKey := make(map[string]*teamTask)
	for i := range b.tasks {
		task := &b.tasks[i]
		executionKey := normalizeExecutionKey(task.ExecutionKey)
		if !strings.HasPrefix(executionKey, ceoConversationFollowUpTaskPrefix+"|") {
			continue
		}
		channel := normalizeChannelSlug(task.Channel)
		if channel == "" {
			channel = "general"
		}
		scopedKey := channel + "|" + executionKey
		canonicalByExecutionKey[scopedKey] = selectCanonicalExecutionTask(canonicalByExecutionKey[scopedKey], task)
	}

	resolved := 0
	for i := range b.tasks {
		task := &b.tasks[i]
		executionKey := normalizeExecutionKey(task.ExecutionKey)
		if !strings.HasPrefix(executionKey, ceoConversationFollowUpTaskPrefix+"|") {
			continue
		}
		channel := normalizeChannelSlug(task.Channel)
		if channel == "" {
			channel = "general"
		}
		scopedKey := channel + "|" + executionKey
		canonical := canonicalByExecutionKey[scopedKey]
		if canonical == nil || strings.TrimSpace(canonical.ID) == strings.TrimSpace(task.ID) {
			continue
		}
		if b.markCEOConversationFollowUpTaskDoneLocked(task, now) {
			resolved++
		}
	}
	return resolved
}

func (b *Broker) markCEOConversationFollowUpTaskDoneLocked(task *teamTask, now string) bool {
	if b == nil || task == nil || taskIsTerminal(task) {
		return false
	}
	task.Status = "done"
	task.Blocked = false
	task.ReviewState = "not_required"
	task.UpdatedAt = now
	normalizeTaskPlan(task)
	b.scheduleTaskLifecycleLocked(task)
	b.appendActionLocked("task_updated", "watchdog", normalizeChannelSlug(task.Channel), "watchdog", truncateSummary(task.Title+" [done]", 140), task.ID)
	return true
}

func (b *Broker) resolveOrphanedTaskArtifactsLocked(now string) (jobsResolved int, alertsResolved int) {
	if b == nil {
		return 0, 0
	}
	if strings.TrimSpace(now) == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}

	taskIDs := make(map[string]struct{}, len(b.tasks))
	for _, task := range b.tasks {
		id := strings.TrimSpace(task.ID)
		if id == "" {
			continue
		}
		taskIDs[id] = struct{}{}
	}

	for i := range b.scheduler {
		job := &b.scheduler[i]
		if strings.TrimSpace(job.TargetType) != "task" {
			continue
		}
		targetID := strings.TrimSpace(job.TargetID)
		if targetID == "" {
			continue
		}
		if _, ok := taskIDs[targetID]; ok {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(job.Status))
		if status == "done" || status == "canceled" {
			continue
		}
		job.Status = "done"
		job.DueAt = ""
		job.NextRun = ""
		job.LastRun = now
		jobsResolved++
	}

	for i := range b.watchdogs {
		alert := &b.watchdogs[i]
		if strings.TrimSpace(alert.TargetType) != "task" {
			continue
		}
		targetID := strings.TrimSpace(alert.TargetID)
		if targetID == "" {
			continue
		}
		if _, ok := taskIDs[targetID]; ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(alert.Status), "resolved") {
			continue
		}
		alert.Status = "resolved"
		alert.UpdatedAt = now
		alertsResolved++
	}

	return jobsResolved, alertsResolved
}

func (b *Broker) collectCEOConversationFollowUpCandidatesLocked(cutoff time.Time) []ceoConversationFollowUpCandidate {
	if b == nil {
		return nil
	}
	respondedByAgent := b.markAncestorRepliesLocked(func(msg channelMessage) bool {
		return !isHumanLikeActor(msg.From) && !isSystemActor(msg.From) && messageIsSubstantiveOfficeContent(msg.Content)
	})
	respondedByNonCEO := b.markAncestorRepliesLocked(func(msg channelMessage) bool {
		if strings.TrimSpace(msg.From) == "ceo" || isSystemActor(msg.From) {
			return false
		}
		if isHumanLikeActor(msg.From) {
			return strings.TrimSpace(msg.Content) != ""
		}
		return messageIsSubstantiveOfficeContent(msg.Content)
	})

	latestInbound := make(map[string]channelMessage)
	latestOutbound := make(map[string]channelMessage)
	latestAgentThreadActivity := make(map[string]time.Time)
	latestNonCEOThreadActivity := make(map[string]time.Time)
	latestNonCEOSemanticActivity := make(map[string]time.Time)

	for _, msg := range b.messages {
		when := parseBrokerTimestamp(msg.Timestamp)
		if when.IsZero() {
			continue
		}
		channel := normalizeChannelSlug(msg.Channel)
		if channel == "" {
			channel = "general"
		}
		rootID := firstNonEmpty(b.threadRootFromMessageIDLocked(msg.ID), strings.TrimSpace(msg.ID))
		threadKey := channel + "|" + rootID

		if !isHumanLikeActor(msg.From) && !isSystemActor(msg.From) && messageIsSubstantiveOfficeContent(msg.Content) {
			if prior, ok := latestAgentThreadActivity[threadKey]; !ok || prior.Before(when) {
				latestAgentThreadActivity[threadKey] = when
			}
		}
		if strings.TrimSpace(msg.From) != "ceo" && !isSystemActor(msg.From) {
			isSubstantive := false
			if isHumanLikeActor(msg.From) {
				isSubstantive = strings.TrimSpace(msg.Content) != ""
			} else {
				isSubstantive = messageIsSubstantiveOfficeContent(msg.Content)
			}
			if isSubstantive {
				if prior, ok := latestNonCEOThreadActivity[threadKey]; !ok || prior.Before(when) {
					latestNonCEOThreadActivity[threadKey] = when
				}
				if semanticKey := semanticFollowUpActivityKey(channel, msg.Content); semanticKey != "" {
					if prior, ok := latestNonCEOSemanticActivity[semanticKey]; !ok || prior.Before(when) {
						latestNonCEOSemanticActivity[semanticKey] = when
					}
				}
			}
		}
	}

	for _, msg := range b.messages {
		when := parseBrokerTimestamp(msg.Timestamp)
		if when.IsZero() || when.After(cutoff) {
			continue
		}
		channel := normalizeChannelSlug(msg.Channel)
		if channel == "" {
			channel = "general"
		}
		rootID := firstNonEmpty(b.threadRootFromMessageIDLocked(msg.ID), strings.TrimSpace(msg.ID))
		threadKey := channel + "|" + rootID

		if ceoNeedsToAnswerHumanMessage(msg) && ceoShouldHandleHumanMessageLocked(b, msg) {
			if latest, ok := latestAgentThreadActivity[threadKey]; ok && latest.After(when) {
				continue
			}
			if _, answered := respondedByAgent[strings.TrimSpace(msg.ID)]; !answered {
				if prior, ok := latestInbound[threadKey]; !ok || parseBrokerTimestamp(prior.Timestamp).Before(when) {
					latestInbound[threadKey] = msg
				}
			}
		}

		if ceoMessageNeedsReply(msg) {
			if latest, ok := latestNonCEOThreadActivity[threadKey]; ok && latest.After(when) {
				continue
			}
			if hasLaterSemanticFollowUpActivity(channel, msg.Content, when, latestNonCEOSemanticActivity) {
				continue
			}
			if b.hasOperationalTaskForThreadLocked(channel, rootID, false) {
				continue
			}
			if _, answered := respondedByNonCEO[strings.TrimSpace(msg.ID)]; !answered {
				key := b.outboundFollowUpSemanticKeyLocked(msg, latestOutbound)
				if prior, ok := latestOutbound[key]; !ok || parseBrokerTimestamp(prior.Timestamp).Before(when) {
					latestOutbound[key] = msg
				}
			}
		}
	}

	candidates := make([]ceoConversationFollowUpCandidate, 0, len(latestInbound)+len(latestOutbound))
	for _, msg := range latestInbound {
		channel := normalizeChannelSlug(msg.Channel)
		if channel == "" {
			channel = "general"
		}
		threadRootID := firstNonEmpty(b.threadRootFromMessageIDLocked(strings.TrimSpace(msg.ID)), strings.TrimSpace(msg.ID))
		candidates = append(candidates, ceoConversationFollowUpCandidate{
			ExecutionKey: normalizeExecutionKey(fmt.Sprintf("%s|incoming|%s|%s", ceoConversationFollowUpTaskPrefix, channel, msg.ID)),
			Channel:      channel,
			ThreadID:     threadRootID,
			Title:        fmt.Sprintf("Reply to pending message from @%s", firstNonEmpty(strings.TrimSpace(msg.From), "human")),
			Details:      b.inboundFollowUpDetailsLocked(channel, msg),
		})
	}
	for _, msg := range latestOutbound {
		channel := normalizeChannelSlug(msg.Channel)
		if channel == "" {
			channel = "general"
		}
		candidates = append(candidates, ceoConversationFollowUpCandidate{
			ExecutionKey: normalizeExecutionKey(fmt.Sprintf("%s|outgoing|%s|%s", ceoConversationFollowUpTaskPrefix, channel, msg.ID)),
			Channel:      channel,
			ThreadID:     strings.TrimSpace(msg.ID),
			Title:        "Validate unanswered CEO follow-up",
			Details:      fmt.Sprintf("A CEO follow-up is still waiting for a reply in #%s.\n\nPending message: %s", channel, truncate(strings.TrimSpace(msg.Content), 280)),
		})
	}
	return candidates
}

func hasLaterSemanticFollowUpActivity(channel, content string, when time.Time, activity map[string]time.Time) bool {
	semanticKey := semanticFollowUpActivityKey(channel, content)
	if semanticKey != "" {
		if latest, ok := activity[semanticKey]; ok && latest.After(when) {
			return true
		}
	}

	channel = normalizeChannelSlug(channel)
	if channel == "" {
		channel = "general"
	}
	contentTokens := extractTaskLaneSignalTokens(content)
	if len(contentTokens) == 0 {
		return false
	}
	for key, latest := range activity {
		if !latest.After(when) {
			continue
		}
		keyChannel, keyTokens := parseSemanticFollowUpActivityKey(key)
		if keyChannel != channel {
			continue
		}
		if followUpSemanticTokensEquivalent(contentTokens, keyTokens) {
			return true
		}
	}
	return false
}

func parseSemanticFollowUpActivityKey(key string) (channel string, tokens []string) {
	parts := strings.SplitN(strings.TrimSpace(key), "|semantic-activity|", 2)
	if len(parts) != 2 {
		return "", nil
	}
	channel = normalizeChannelSlug(parts[0])
	if strings.TrimSpace(parts[1]) == "" {
		return channel, nil
	}
	return channel, strings.Split(parts[1], ",")
}

func followUpSemanticTokensEquivalent(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	rightSet := make(map[string]struct{}, len(right))
	for _, token := range right {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		rightSet[token] = struct{}{}
	}
	shared := 0
	for _, token := range left {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if _, ok := rightSet[token]; ok {
			shared++
		}
	}
	shorter := len(left)
	if len(right) < shorter {
		shorter = len(right)
	}
	if shorter == 0 {
		return false
	}
	return shared >= 2 || shared*10 >= shorter*7
}

func (b *Broker) outboundFollowUpSemanticKeyLocked(msg channelMessage, existing map[string]channelMessage) string {
	channel := normalizeChannelSlug(msg.Channel)
	if channel == "" {
		channel = "general"
	}
	recipientKey := outboundFollowUpRecipientKey(msg.Tagged)
	threadRootID := firstNonEmpty(b.threadRootFromMessageIDLocked(strings.TrimSpace(msg.ID)), strings.TrimSpace(msg.ID))
	for key, prior := range existing {
		if normalizeChannelSlug(prior.Channel) != channel {
			continue
		}
		priorRootID := firstNonEmpty(b.threadRootFromMessageIDLocked(strings.TrimSpace(prior.ID)), strings.TrimSpace(prior.ID))
		if priorRootID == threadRootID {
			return key
		}
	}
	anchors := extractTaskLaneSignalTokens(msg.Content)
	if len(anchors) > 0 {
		return channel + "|semantic|" + recipientKey + "|" + strings.Join(anchors, ",")
	}
	signature := normalizeCoordinationGuidanceSignature(msg.Content)
	if signature == "" {
		return outboundFollowUpThreadKey(msg, recipientKey)
	}
	for key, prior := range existing {
		if !strings.HasPrefix(key, channel+"|semantic|"+recipientKey+"|") {
			continue
		}
		if coordinationGuidanceEquivalent(prior.Content, msg.Content) {
			return key
		}
	}
	return channel + "|semantic|" + recipientKey + "|" + signature
}

func outboundFollowUpThreadKey(msg channelMessage, recipientKey string) string {
	channel := normalizeChannelSlug(msg.Channel)
	if channel == "" {
		channel = "general"
	}
	rootID := strings.TrimSpace(msg.ID)
	if replyTo := strings.TrimSpace(msg.ReplyTo); replyTo != "" {
		rootID = replyTo
	}
	return channel + "|thread|" + recipientKey + "|" + rootID
}

func outboundFollowUpRecipientKey(tagged []string) string {
	recipients := make([]string, 0, len(tagged))
	for _, taggedSlug := range tagged {
		slug := strings.TrimPrefix(strings.TrimSpace(taggedSlug), "@")
		if slug == "" || slug == "ceo" || isSystemActor(slug) {
			continue
		}
		recipients = append(recipients, slug)
	}
	if len(recipients) == 0 {
		return "thread"
	}
	return normalizeExecutionKey(strings.Join(uniqueSlugs(recipients), ","))
}

func semanticFollowUpActivityKey(channel, content string) string {
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		channel = "general"
	}
	anchors := extractTaskLaneSignalTokens(content)
	if len(anchors) == 0 {
		return ""
	}
	return channel + "|semantic-activity|" + strings.Join(anchors, ",")
}

func (b *Broker) markAncestorRepliesLocked(predicate func(channelMessage) bool) map[string]struct{} {
	replied := make(map[string]struct{})
	if b == nil || predicate == nil {
		return replied
	}
	for _, msg := range b.messages {
		if !predicate(msg) {
			continue
		}
		parentID := strings.TrimSpace(msg.ReplyTo)
		seen := make(map[string]struct{})
		for parentID != "" {
			if _, loop := seen[parentID]; loop {
				break
			}
			seen[parentID] = struct{}{}
			replied[parentID] = struct{}{}
			parent := b.findMessageByIDLocked(parentID)
			if parent == nil {
				break
			}
			parentID = strings.TrimSpace(parent.ReplyTo)
		}
	}
	return replied
}

func ceoNeedsToAnswerHumanMessage(msg channelMessage) bool {
	switch strings.ToLower(strings.TrimSpace(msg.From)) {
	case "you", "human", "nex":
		return strings.TrimSpace(msg.ID) != "" && strings.TrimSpace(msg.Content) != ""
	default:
		return false
	}
}

func ceoShouldHandleHumanMessageLocked(b *Broker, msg channelMessage) bool {
	if b == nil {
		return false
	}
	channel := normalizeChannelSlug(msg.Channel)
	if channel == "" {
		channel = "general"
	}
	if isDM, target := b.channelIsDMLocked(channel); isDM {
		return strings.TrimSpace(target) == "ceo"
	}
	if len(msg.Tagged) == 0 {
		return true
	}
	for _, tagged := range msg.Tagged {
		slug := strings.TrimPrefix(strings.TrimSpace(tagged), "@")
		if slug == "ceo" {
			return true
		}
	}
	return false
}

func ceoMessageNeedsReply(msg channelMessage) bool {
	if strings.TrimSpace(msg.From) != "ceo" {
		return false
	}
	if strings.TrimSpace(msg.Kind) != "" {
		return false
	}
	if !messageIsSubstantiveOfficeContent(msg.Content) {
		return false
	}
	if ceoMessageExplicitlySuppressesFollowUp(msg.Content) {
		return false
	}
	for _, tagged := range msg.Tagged {
		slug := strings.TrimPrefix(strings.TrimSpace(tagged), "@")
		if slug != "" && slug != "ceo" && !isSystemActor(slug) {
			return true
		}
	}
	return strings.TrimSpace(msg.ReplyTo) != "" && messageLooksLikeCoordinationGuidance(msg.Content)
}

func ceoMessageExplicitlySuppressesFollowUp(content string) bool {
	normalized := strings.ToLower(strings.TrimSpace(content))
	if normalized == "" {
		return false
	}
	suppressors := []string{
		"nenhuma ação adicional",
		"nenhuma acao adicional",
		"sem nova resposta",
		"sem novo alerta",
		"nenhuma resposta imediata",
		"no further action",
		"no additional action",
		"no immediate reply",
		"won't do",
		"wont do",
	}
	for _, suppressor := range suppressors {
		if strings.Contains(normalized, suppressor) {
			return true
		}
	}
	return false
}

func (b *Broker) hasActiveOperationalTaskForThreadLocked(channel, threadRootID string) bool {
	return b.hasOperationalTaskForThreadLocked(channel, threadRootID, true)
}

func (b *Broker) hasOperationalTaskForThreadLocked(channel, threadRootID string, activeOnly bool) bool {
	if b == nil {
		return false
	}
	channel = normalizeChannelSlug(channel)
	threadRootID = strings.TrimSpace(threadRootID)
	if channel == "" || threadRootID == "" {
		return false
	}
	for i := range b.tasks {
		task := &b.tasks[i]
		if normalizeChannelSlug(task.Channel) != channel {
			continue
		}
		if activeOnly && taskIsTerminal(task) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(task.TaskType), "follow_up") {
			continue
		}
		taskRoot := firstNonEmpty(
			b.threadRootFromMessageIDLocked(strings.TrimSpace(task.ThreadID)),
			strings.TrimSpace(task.ThreadID),
		)
		if taskRoot == threadRootID {
			return true
		}
	}
	return false
}

func (b *Broker) upsertCEOConversationFollowUpTaskLocked(candidate ceoConversationFollowUpCandidate, now string) bool {
	if b == nil {
		return false
	}
	channel := normalizeChannelSlug(candidate.Channel)
	if channel == "" {
		channel = "general"
	}
	if existing := b.findCanonicalTaskByExecutionKeyLocked(channel, candidate.ExecutionKey); existing != nil {
		changed := false
		if strings.TrimSpace(existing.Title) != strings.TrimSpace(candidate.Title) {
			existing.Title = strings.TrimSpace(candidate.Title)
			changed = true
		}
		if strings.TrimSpace(existing.Details) != strings.TrimSpace(candidate.Details) {
			existing.Details = strings.TrimSpace(candidate.Details)
			changed = true
		}
		if strings.TrimSpace(existing.Owner) != "ceo" {
			existing.Owner = "ceo"
			changed = true
		}
		if strings.TrimSpace(existing.ThreadID) != strings.TrimSpace(candidate.ThreadID) {
			existing.ThreadID = strings.TrimSpace(candidate.ThreadID)
			changed = true
		}
		if strings.TrimSpace(existing.TaskType) != "follow_up" {
			existing.TaskType = "follow_up"
			changed = true
		}
		if strings.TrimSpace(existing.PipelineID) != "follow_up" {
			existing.PipelineID = "follow_up"
			changed = true
		}
		if strings.TrimSpace(existing.ExecutionMode) != "office" {
			existing.ExecutionMode = "office"
			changed = true
		}
		if strings.TrimSpace(existing.ReviewState) != "not_required" {
			existing.ReviewState = "not_required"
			changed = true
		}
		if strings.TrimSpace(existing.Status) != "in_progress" || existing.Blocked {
			existing.Status = "in_progress"
			existing.Blocked = false
			changed = true
		}
		if !changed {
			return false
		}
		existing.UpdatedAt = now
		b.ensureTaskOwnerChannelMembershipLocked(channel, existing.Owner)
		b.queueTaskBehindActiveOwnerLaneLocked(existing)
		normalizeTaskPlan(existing)
		b.scheduleTaskLifecycleLocked(existing)
		b.appendActionLocked("task_updated", "watchdog", channel, "watchdog", truncateSummary(existing.Title+" ["+existing.Status+"]", 140), existing.ID)
		return true
	}

	task := teamTask{
		ID:            fmt.Sprintf("task-%d", b.counter+1),
		Channel:       channel,
		ExecutionKey:  candidate.ExecutionKey,
		Title:         strings.TrimSpace(candidate.Title),
		Details:       strings.TrimSpace(candidate.Details),
		Owner:         "ceo",
		Status:        "in_progress",
		CreatedBy:     "watchdog",
		ThreadID:      strings.TrimSpace(candidate.ThreadID),
		TaskType:      "follow_up",
		PipelineID:    "follow_up",
		ExecutionMode: "office",
		ReviewState:   "not_required",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	b.counter++
	b.ensureTaskOwnerChannelMembershipLocked(channel, task.Owner)
	b.queueTaskBehindActiveOwnerLaneLocked(&task)
	normalizeTaskPlan(&task)
	b.scheduleTaskLifecycleLocked(&task)
	b.tasks = append(b.tasks, task)
	b.appendActionLocked("task_created", "watchdog", channel, "watchdog", truncateSummary(task.Title, 140), task.ID)
	return true
}
