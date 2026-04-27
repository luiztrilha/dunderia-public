package team

import (
	"fmt"
	"strings"
	"time"
)

type headlessProgressMetrics struct {
	TotalMs      int64
	FirstEventMs int64
	FirstTextMs  int64
	FirstToolMs  int64
}

func (l *Launcher) updateHeadlessProgress(slug string, status string, activity string, detail string, metrics headlessProgressMetrics, channel ...string) {
	if l == nil || l.broker == nil {
		return
	}
	status = strings.TrimSpace(status)
	activity = strings.TrimSpace(activity)
	detail = strings.TrimSpace(detail)
	laneChannel := ""
	if len(channel) > 0 {
		laneChannel = normalizeChannelSlug(channel[0])
	}
	if laneChannel == "" {
		laneChannel = l.headlessTurnChannel(slug)
	}
	l.broker.UpdateAgentActivity(agentActivitySnapshot{
		Slug:         slug,
		Channel:      laneChannel,
		Status:       status,
		Activity:     activity,
		Detail:       detail,
		LastTime:     time.Now().UTC().Format(time.RFC3339),
		TotalMs:      metrics.TotalMs,
		FirstEventMs: metrics.FirstEventMs,
		FirstTextMs:  metrics.FirstTextMs,
		FirstToolMs:  metrics.FirstToolMs,
	})
	l.maybeHandleAgentOperationalIssue(slug, status, detail, laneChannel)
}

func formatHeadlessLatencySummary(metrics headlessProgressMetrics) string {
	var parts []string
	if metrics.FirstTextMs >= 0 {
		parts = append(parts, fmt.Sprintf("ttft %dms", metrics.FirstTextMs))
	} else if metrics.FirstEventMs >= 0 {
		parts = append(parts, fmt.Sprintf("first event %dms", metrics.FirstEventMs))
	}
	if metrics.FirstToolMs >= 0 {
		parts = append(parts, fmt.Sprintf("first tool %dms", metrics.FirstToolMs))
	}
	if metrics.TotalMs >= 0 {
		parts = append(parts, fmt.Sprintf("done %dms", metrics.TotalMs))
	}
	return strings.Join(parts, " · ")
}
