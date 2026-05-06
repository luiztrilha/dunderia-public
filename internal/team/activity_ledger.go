package team

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type officeActivityEvent struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Kind      string `json:"kind,omitempty"`
	Source    string `json:"source,omitempty"`
	Channel   string `json:"channel,omitempty"`
	Actor     string `json:"actor,omitempty"`
	ActorType string `json:"actor_type,omitempty"`
	Title     string `json:"title,omitempty"`
	Summary   string `json:"summary,omitempty"`
	RelatedID string `json:"related_id,omitempty"`
	Severity  string `json:"severity,omitempty"`
	Timestamp string `json:"timestamp"`
}

func activityEventTime(value string) time.Time {
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value)); err == nil {
		return parsed
	}
	return time.Time{}
}

func appendActivityEvent(events []officeActivityEvent, event officeActivityEvent) []officeActivityEvent {
	event.ID = strings.TrimSpace(event.ID)
	event.Type = strings.TrimSpace(event.Type)
	event.Timestamp = strings.TrimSpace(event.Timestamp)
	if event.ID == "" || event.Type == "" || event.Timestamp == "" {
		return events
	}
	event.Channel = normalizeChannelSlug(event.Channel)
	event.Actor = strings.TrimSpace(event.Actor)
	event.ActorType = normalizeActivityActorType(event.ActorType)
	if event.ActorType == "" {
		event.ActorType = actorTypeForActivity(event.Actor, event.Source, event.Kind)
	}
	event.Kind = strings.TrimSpace(event.Kind)
	event.Source = strings.TrimSpace(event.Source)
	event.Title = strings.TrimSpace(event.Title)
	event.Summary = strings.TrimSpace(event.Summary)
	event.RelatedID = strings.TrimSpace(event.RelatedID)
	event.Severity = strings.TrimSpace(event.Severity)
	return append(events, event)
}

func normalizeActivityActorType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "human", "agent", "system", "adapter":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func actorTypeForActivity(actor, source, kind string) string {
	actor = normalizeActorSlug(actor)
	text := strings.ToLower(strings.Join([]string{source, kind}, " "))
	switch {
	case actor != "" && isHumanLikeActor(actor):
		return "human"
	case actor == "scheduler" || actor == "watchdog" || actor == "wuphf" || isSystemActor(actor):
		return "system"
	case strings.Contains(text, "adapter") || strings.Contains(text, "plugin") || strings.Contains(text, "workflow") || strings.Contains(text, "external") || strings.Contains(text, "github"):
		return "adapter"
	case actor != "":
		return "agent"
	default:
		return "system"
	}
}

func (b *Broker) activityEventsLocked(limit int) []officeActivityEvent {
	events := make([]officeActivityEvent, 0, len(b.actions)+len(b.decisions)+len(b.signals)+len(b.watchdogs)+len(b.scheduler))
	for _, action := range b.actions {
		events = appendActivityEvent(events, officeActivityEvent{
			ID:        action.ID,
			Type:      "action",
			Kind:      action.Kind,
			Source:    action.Source,
			Channel:   action.Channel,
			Actor:     action.Actor,
			Summary:   action.Summary,
			RelatedID: action.RelatedID,
			Severity:  action.GovernanceSeverity,
			Timestamp: action.CreatedAt,
		})
	}
	for _, decision := range b.decisions {
		severity := ""
		if decision.Blocking {
			severity = "warning"
		}
		events = appendActivityEvent(events, officeActivityEvent{
			ID:        decision.ID,
			Type:      "decision",
			Kind:      decision.Kind,
			Channel:   decision.Channel,
			Actor:     decision.Owner,
			Summary:   firstNonEmpty(decision.Summary, decision.Reason),
			RelatedID: strings.Join(decision.SignalIDs, ","),
			Severity:  severity,
			Timestamp: decision.CreatedAt,
		})
	}
	for _, signal := range b.signals {
		severity := ""
		if signal.Blocking || signal.RequiresHuman {
			severity = "warning"
		}
		events = appendActivityEvent(events, officeActivityEvent{
			ID:        signal.ID,
			Type:      "signal",
			Kind:      signal.Kind,
			Source:    signal.Source,
			Channel:   signal.Channel,
			Actor:     signal.Owner,
			Title:     signal.Title,
			Summary:   signal.Content,
			RelatedID: signal.SourceRef,
			Severity:  severity,
			Timestamp: signal.CreatedAt,
		})
	}
	for _, alert := range b.watchdogs {
		events = appendActivityEvent(events, officeActivityEvent{
			ID:        alert.ID,
			Type:      "watchdog",
			Kind:      alert.Kind,
			Channel:   alert.Channel,
			Actor:     alert.Owner,
			Summary:   alert.Summary,
			RelatedID: firstNonEmpty(alert.TargetID, alert.TargetType),
			Severity:  "warning",
			Timestamp: firstNonEmpty(alert.UpdatedAt, alert.CreatedAt),
		})
	}
	for _, job := range b.scheduler {
		events = appendActivityEvent(events, officeActivityEvent{
			ID:        firstNonEmpty(job.Slug, job.TargetID),
			Type:      "routine",
			Kind:      job.Kind,
			Source:    job.Provider,
			Channel:   job.Channel,
			Title:     job.Label,
			Summary:   firstNonEmpty(job.LastSummary, job.WorkflowKey, job.SkillName, job.Payload),
			RelatedID: firstNonEmpty(job.TargetID, job.WorkflowKey),
			Severity:  "",
			Timestamp: firstNonEmpty(job.LastFinishedAt, job.LastStartedAt, job.LastRun, job.NextRun, job.DueAt),
		})
	}
	for _, task := range b.tasks {
		for _, artifact := range task.Artifacts {
			events = appendActivityEvent(events, officeActivityEvent{
				ID:        firstNonEmpty(artifact.ID, task.ID+"-"+artifact.Kind),
				Type:      "work_product",
				Kind:      firstNonEmpty(artifact.ResultRole, artifact.Kind),
				Channel:   task.Channel,
				Actor:     firstNonEmpty(artifact.CreatedBy, task.Owner, task.CreatedBy),
				Title:     artifact.Title,
				Summary:   firstNonEmpty(artifact.Summary, artifact.URL, artifact.Path, artifact.PreviewURL),
				RelatedID: task.ID,
				Severity:  artifact.State,
				Timestamp: firstNonEmpty(artifact.UpdatedAt, artifact.CreatedAt, task.UpdatedAt),
			})
		}
	}
	sort.SliceStable(events, func(i, j int) bool {
		left := activityEventTime(events[i].Timestamp)
		right := activityEventTime(events[j].Timestamp)
		if left.Equal(right) {
			return events[i].ID > events[j].ID
		}
		return left.After(right)
	})
	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}
	return events
}

func (b *Broker) handleActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}
	eventType := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type")))
	channel := ""
	if rawChannel := strings.TrimSpace(r.URL.Query().Get("channel")); rawChannel != "" {
		channel = normalizeChannelSlug(rawChannel)
	}
	b.mu.Lock()
	events := b.activityEventsLocked(0)
	b.mu.Unlock()
	if eventType != "" || channel != "" {
		filtered := events[:0]
		for _, event := range events {
			if eventType != "" && event.Type != eventType {
				continue
			}
			if channel != "" && normalizeChannelSlug(event.Channel) != channel {
				continue
			}
			filtered = append(filtered, event)
		}
		events = filtered
	}
	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"events": events})
}
