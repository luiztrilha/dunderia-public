package team

import (
	"regexp"
	"strings"
	"time"
)

var headlessReplyRoutePattern = regexp.MustCompile(`team_broadcast with my_slug "[^"]+"(?:,| and) channel "([^"]+)"(?:,)? reply_to_id "([^"]+)"`)
var headlessReplyRouteMarkerPattern = regexp.MustCompile(`WUPHF_REPLY_ROUTE channel="([^"]+)" reply_to_id="([^"]+)"`)
var headlessExpectedReplyTargetPattern = regexp.MustCompile(`must reply(?: in thread| to) ([A-Za-z0-9_-]+)`)

func headlessReplyRoute(notification string) (channel string, replyTo string, ok bool) {
	markerMatches := headlessReplyRouteMarkerPattern.FindAllStringSubmatch(notification, -1)
	if len(markerMatches) > 0 {
		last := markerMatches[len(markerMatches)-1]
		if len(last) >= 3 {
			channel = normalizeChannelSlug(strings.TrimSpace(last[1]))
			replyTo = strings.TrimSpace(last[2])
			if channel != "" && replyTo != "" {
				return channel, replyTo, true
			}
		}
	}
	matches := headlessReplyRoutePattern.FindAllStringSubmatch(notification, -1)
	if len(matches) == 0 {
		return "", "", false
	}
	last := matches[len(matches)-1]
	if len(last) < 3 {
		return "", "", false
	}
	channel = normalizeChannelSlug(strings.TrimSpace(last[1]))
	replyTo = strings.TrimSpace(last[2])
	if channel == "" || replyTo == "" {
		return "", "", false
	}
	return channel, replyTo, true
}

func (l *Launcher) publishHeadlessFallbackReply(slug string, notification string, text string, startedAt time.Time) {
	if l == nil || l.broker == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" || !messageIsSubstantiveOfficeContent(text) {
		return
	}
	channel, replyTo, ok := headlessReplyRoute(notification)
	if !ok {
		return
	}
	if l.agentPostedSubstantiveReplySince(slug, channel, replyTo, startedAt) {
		return
	}
	if _, err := l.broker.PostMessage(slug, channel, text, nil, replyTo); err == nil {
		return
	} else if expectedReplyTo := headlessExpectedReplyTarget(err); expectedReplyTo != "" && expectedReplyTo != replyTo {
		if l.agentPostedSubstantiveReplySince(slug, channel, expectedReplyTo, startedAt) {
			return
		}
		if _, retryErr := l.broker.PostMessage(slug, channel, text, nil, expectedReplyTo); retryErr == nil {
			l.appendHeadlessFallbackLog(slug, "fallback-post-retry: switched reply_to_id from "+replyTo+" to "+expectedReplyTo)
			return
		} else {
			l.appendHeadlessFallbackLog(slug, "fallback-post-error: "+strings.TrimSpace(retryErr.Error()))
			l.publishTaskStateClaimNeutralFallback(slug, channel, expectedReplyTo, text, retryErr)
			return
		}
	} else {
		l.appendHeadlessFallbackLog(slug, "fallback-post-error: "+strings.TrimSpace(err.Error()))
		l.publishTaskStateClaimNeutralFallback(slug, channel, replyTo, text, err)
	}
}

func (l *Launcher) publishTaskStateClaimNeutralFallback(slug, channel, replyTo, blockedText string, postErr error) {
	if l == nil || l.broker == nil || postErr == nil {
		return
	}
	if !strings.Contains(strings.ToLower(strings.TrimSpace(postErr.Error())), "task state claim contradicts live state") {
		return
	}
	text := l.taskStateClaimNeutralFallbackText(channel, blockedText)
	if text == "" {
		return
	}
	if _, err := l.broker.PostMessage(slug, channel, text, nil, replyTo); err != nil {
		l.appendHeadlessFallbackLog(slug, "fallback-neutral-post-error: "+strings.TrimSpace(err.Error()))
	}
}

func (l *Launcher) taskStateClaimNeutralFallbackText(channel, blockedText string) string {
	if l == nil || l.broker == nil {
		return ""
	}
	claim := parseTaskStateClaim(blockedText)
	if len(claim.TaskIDs) == 0 {
		return ""
	}
	channel = normalizeChannelSlug(channel)
	lines := []string{
		"A resposta automatica anterior foi bloqueada porque continha uma declaracao de estado sem a mutacao duravel correspondente.",
		"Estado observavel agora:",
	}
	added := 0
	for _, taskID := range claim.TaskIDs {
		task, ok := l.broker.TaskByID(taskID)
		if !ok {
			continue
		}
		if channel != "" && normalizeChannelSlug(task.Channel) != channel {
			continue
		}
		title := strings.TrimSpace(task.Title)
		if title == "" {
			title = "tarefa sem titulo"
		}
		lines = append(lines, "- "+task.ID+": "+title)
		added++
	}
	if added == 0 {
		return ""
	}
	lines = append(lines, "Nenhum estado de task foi alterado por esta resposta.")
	return strings.Join(lines, "\n")
}

func headlessExpectedReplyTarget(err error) string {
	if err == nil {
		return ""
	}
	matches := headlessExpectedReplyTargetPattern.FindStringSubmatch(strings.TrimSpace(err.Error()))
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func shouldPublishHeadlessErrorFallback(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	return !isHeadlessRuntimeProviderFailure(text)
}

func (l *Launcher) appendHeadlessFallbackLog(slug string, line string) {
	if l == nil {
		return
	}
	if strings.EqualFold(strings.TrimSpace(l.memberEffectiveProviderKind(slug)), "codex") {
		appendHeadlessCodexLog(slug, line)
		return
	}
	appendHeadlessClaudeLog(slug, line)
}

func (l *Launcher) agentPostedSubstantiveReplySince(slug string, channel string, replyTo string, startedAt time.Time) bool {
	if l == nil || l.broker == nil {
		return false
	}
	channel = normalizeChannelSlug(channel)
	replyTo = strings.TrimSpace(replyTo)
	for _, msg := range l.broker.AllMessages() {
		if msg.From != slug || normalizeChannelSlug(msg.Channel) != channel {
			continue
		}
		if replyTo != "" && strings.TrimSpace(msg.ReplyTo) != replyTo {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if !messageIsSubstantiveOfficeContent(content) {
			continue
		}
		when, err := time.Parse(time.RFC3339, msg.Timestamp)
		if err != nil {
			continue
		}
		if when.Add(time.Second).After(startedAt) {
			return true
		}
	}
	return false
}
