package team

import (
	"strings"
	"time"
)

type telegramHumanAttentionAlert struct {
	Task         teamTask
	AlertChannel string
	EventID      string
	Content      string
	ReplyTo      string
}

type telegramAlertDelivery struct {
	EventID   string `json:"event_id"`
	ChatID    string `json:"chat_id"`
	MessageID int64  `json:"message_id"`
	CreatedAt string `json:"created_at,omitempty"`
	DeletedAt string `json:"deleted_at,omitempty"`
}

// EmitTelegramHumanAttentionAlerts publishes one deduplicated automation
// message per open human-action task into a Telegram-backed channel.
func (b *Broker) EmitTelegramHumanAttentionAlerts() (int, error) {
	if b == nil {
		return 0, nil
	}

	b.mu.Lock()
	tasks := b.buildOperatorTasksLiteLocked("", true, false, "", "human")
	alerts := make([]telegramHumanAttentionAlert, 0, len(tasks))
	for _, task := range tasks {
		if !task.AwaitingHuman {
			continue
		}
		eventID := telegramHumanAttentionEventID(task)
		if eventID == "" || b.messageEventExistsLocked(eventID) {
			continue
		}
		alertChannel := b.telegramHumanAttentionChannelLocked(task.Channel)
		if alertChannel == "" {
			continue
		}
		alerts = append(alerts, telegramHumanAttentionAlert{
			Task:         task,
			AlertChannel: alertChannel,
			EventID:      eventID,
			Content:      formatTelegramHumanAttentionAlert(task),
			ReplyTo:      telegramHumanAttentionReplyTo(task, alertChannel),
		})
	}
	b.mu.Unlock()

	emitted := 0
	for _, alert := range alerts {
		_, duplicate, err := b.PostAutomationMessage(
			"wuphf",
			alert.AlertChannel,
			"Human input needed",
			alert.Content,
			alert.EventID,
			"telegram_alert",
			"Telegram alert",
			[]string{"human"},
			alert.ReplyTo,
		)
		if err != nil {
			return emitted, err
		}
		if !duplicate {
			emitted++
		}
	}
	return emitted, nil
}

func telegramHumanAttentionReplyTo(task teamTask, alertChannel string) string {
	if normalizeChannelSlug(task.Channel) != normalizeChannelSlug(alertChannel) {
		return ""
	}
	return strings.TrimSpace(task.ThreadID)
}

func telegramHumanAttentionEventID(task teamTask) string {
	id := strings.TrimSpace(task.ID)
	if id == "" {
		id = strings.TrimSpace(task.ExecutionKey)
	}
	if id == "" {
		return ""
	}
	return "telegram-human-attention:" + id
}

func (b *Broker) messageEventExistsLocked(eventID string) bool {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return false
	}
	for _, msg := range b.messages {
		if strings.TrimSpace(msg.EventID) == eventID {
			return true
		}
	}
	return false
}

func (b *Broker) telegramHumanAttentionChannelLocked(preferred string) string {
	preferred = normalizeChannelSlug(preferred)
	privateFallback := ""
	anyFallback := ""
	for _, ch := range b.channels {
		if ch.Surface == nil || ch.Surface.Provider != "telegram" {
			continue
		}
		slug := normalizeChannelSlug(ch.Slug)
		if slug == "" {
			continue
		}
		if slug == preferred {
			return slug
		}
		if privateFallback == "" && (ch.Surface.Mode == "private" || ch.Surface.RemoteID == "0") {
			privateFallback = slug
		}
		if anyFallback == "" {
			anyFallback = slug
		}
	}
	return firstNonEmpty(privateFallback, anyFallback)
}

func formatTelegramHumanAttentionAlert(task teamTask) string {
	channel := normalizeChannelSlug(task.Channel)
	if channel == "" {
		channel = "general"
	}
	title := strings.TrimSpace(task.Title)
	if title == "" {
		title = "Human response needed"
	}
	reason := strings.TrimSpace(task.AwaitingHumanReason)
	if reason == "" {
		reason = strings.TrimSpace(task.Details)
	}
	if reason == "" {
		reason = "A task is waiting for your input."
	}

	lines := []string{
		"MaestrIA needs your input.",
		"Task: " + truncateSummary(title, 120),
		"Channel: #" + channel,
		"Reason: " + truncateSummary(reason, 220),
	}
	if requestID := strings.TrimSpace(task.AwaitingHumanRequestID); requestID != "" {
		lines = append(lines, "Request: "+requestID)
	}
	return strings.Join(lines, "\n")
}

func (b *Broker) RecordTelegramAlertDelivery(eventID, chatID string, messageID int64) error {
	eventID = strings.TrimSpace(eventID)
	chatID = strings.TrimSpace(chatID)
	if b == nil || eventID == "" || chatID == "" || messageID == 0 {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.telegramAlertDeliveries == nil {
		b.telegramAlertDeliveries = make(map[string]telegramAlertDelivery)
	}
	b.telegramAlertDeliveries[eventID] = telegramAlertDelivery{
		EventID:   eventID,
		ChatID:    chatID,
		MessageID: messageID,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	return b.saveLocked()
}

func (b *Broker) PendingTelegramAlertDeletions() []telegramAlertDelivery {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	active := b.activeTelegramHumanAttentionEventIDsLocked()
	out := make([]telegramAlertDelivery, 0, len(b.telegramAlertDeliveries))
	for eventID, delivery := range b.telegramAlertDeliveries {
		if strings.TrimSpace(delivery.DeletedAt) != "" {
			continue
		}
		if _, ok := active[eventID]; ok {
			continue
		}
		out = append(out, delivery)
	}
	return out
}

func (b *Broker) MarkTelegramAlertDeleted(eventID string) error {
	eventID = strings.TrimSpace(eventID)
	if b == nil || eventID == "" {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.telegramAlertDeliveries == nil {
		return nil
	}
	delivery, ok := b.telegramAlertDeliveries[eventID]
	if !ok {
		return nil
	}
	delivery.DeletedAt = time.Now().UTC().Format(time.RFC3339)
	b.telegramAlertDeliveries[eventID] = delivery
	return b.saveLocked()
}

func (b *Broker) activeTelegramHumanAttentionEventIDsLocked() map[string]struct{} {
	tasks := b.buildOperatorTasksLiteLocked("", true, false, "", "human")
	active := make(map[string]struct{}, len(tasks))
	for _, task := range tasks {
		if !task.AwaitingHuman {
			continue
		}
		if eventID := telegramHumanAttentionEventID(task); eventID != "" {
			active[eventID] = struct{}{}
		}
	}
	return active
}

func cloneTelegramAlertDeliveries(in map[string]telegramAlertDelivery) map[string]telegramAlertDelivery {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]telegramAlertDelivery, len(in))
	for key, delivery := range in {
		out[key] = delivery
	}
	return out
}
