package team

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"
)

type operatorAlertsResponse struct {
	GeneratedAt string          `json:"generated_at"`
	Persisted   bool            `json:"persisted"`
	Status      string          `json:"status"`
	Summary     map[string]int  `json:"summary"`
	Alerts      []operatorAlert `json:"alerts"`
}

type operatorAlert struct {
	ID          string   `json:"id"`
	Severity    string   `json:"severity"`
	Source      string   `json:"source"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Channel     string   `json:"channel,omitempty"`
	RelatedType string   `json:"related_type,omitempty"`
	RelatedID   string   `json:"related_id,omitempty"`
	Action      string   `json:"action,omitempty"`
	Endpoint    string   `json:"endpoint,omitempty"`
	Command     string   `json:"command,omitempty"`
	CreatedAt   string   `json:"created_at,omitempty"`
	Signals     []string `json:"signals,omitempty"`
}

func (b *Broker) handleOperatorAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	allChannels := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all_channels")), "true")
	if channel == "" && !allChannels {
		channel = "general"
	}
	b.mu.RLock()
	payload := b.buildOperatorAlertsLocked(viewer, channel, allChannels, time.Now().UTC())
	b.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) buildOperatorAlertsLocked(viewer, channel string, allChannels bool, now time.Time) operatorAlertsResponse {
	state := studioDevConsoleState{
		SessionMode:    b.sessionMode,
		DirectAgent:    b.oneOnOneAgent,
		FocusMode:      b.focusMode,
		Members:        append([]officeMember(nil), b.members...),
		Channels:       append([]teamChannel(nil), b.channels...),
		Tasks:          append([]teamTask(nil), b.tasks...),
		Requests:       append([]humanInterview(nil), b.requests...),
		Actions:        append([]officeActionLog(nil), b.actions...),
		Decisions:      append([]officeDecisionRecord(nil), b.decisions...),
		Watchdogs:      append([]watchdogAlert(nil), b.watchdogs...),
		ExecutionNodes: append([]executionNode(nil), b.executionNodes...),
		Messages:       append([]channelMessage(nil), b.messages...),
		Activity:       studioActivitySnapshotsFromMap(b.activity),
		WebUIOrigins:   append([]string(nil), b.webUIOrigins...),
		BrokerReady:    true,
	}
	return b.buildOperatorAlertsFromStateLocked(state, cloneTeamUsageState(b.usage), viewer, channel, allChannels, now)
}

func (b *Broker) buildOperatorAlertsFromStateLocked(state studioDevConsoleState, usage teamUsageState, viewer, channel string, allChannels bool, now time.Time) operatorAlertsResponse {
	alerts := make([]operatorAlert, 0)
	add := func(alert operatorAlert) {
		alert.ID = strings.TrimSpace(alert.ID)
		alert.Source = strings.TrimSpace(alert.Source)
		alert.Title = strings.TrimSpace(alert.Title)
		alert.Summary = truncateSummary(alert.Summary, 220)
		if alert.ID == "" || alert.Source == "" || alert.Title == "" {
			return
		}
		alert.Severity = normalizeOperatorAlertSeverity(alert.Severity)
		alert.Channel = normalizeChannelSlug(alert.Channel)
		if !b.operatorAlertVisibleLocked(viewer, alert.Channel, channel, allChannels) {
			return
		}
		alert.Signals = compactStringList(alert.Signals)
		alerts = append(alerts, alert)
	}

	doctor := buildRuntimeDoctorSnapshot(state)
	for _, check := range doctor.Checks {
		if check.Severity != runtimeDoctorWarn && check.Severity != runtimeDoctorFail {
			continue
		}
		add(operatorAlert{
			ID:        "runtime:" + check.ID,
			Severity:  operatorSeverityFromDoctor(check.Severity),
			Source:    "runtime_doctor",
			Title:     check.Label,
			Summary:   firstNonEmpty(check.Summary, check.Detail),
			Action:    check.NextStep,
			Endpoint:  "/runtime/doctor",
			CreatedAt: doctor.GeneratedAt,
			Signals:   []string{check.ID},
		})
	}

	for _, blocker := range operatorVisibleBlockers(buildStudioBlockersFromState(state), state.Tasks) {
		add(operatorAlert{
			ID:          "blocker:" + blocker.ID,
			Severity:    blocker.Severity,
			Source:      "studio_blocker",
			Title:       firstNonEmpty(blocker.Title, "Active blocker"),
			Summary:     firstNonEmpty(blocker.Summary, blocker.Reason),
			Channel:     blocker.Channel,
			RelatedType: "task",
			RelatedID:   blocker.TaskID,
			Action:      blocker.RecommendedAction,
			Endpoint:    "/studio/dev-console",
			Signals:     []string{blocker.Kind},
		})
	}

	for _, req := range state.Requests {
		if intakeRequestStatusResolved(req.Status) || !req.Blocking {
			continue
		}
		add(operatorAlert{
			ID:          "request:" + req.ID,
			Severity:    "warning",
			Source:      "human_request",
			Title:       firstNonEmpty(req.Title, "Human input required"),
			Summary:     firstNonEmpty(req.Question, req.Context, "A blocking human request is waiting."),
			Channel:     req.Channel,
			RelatedType: "request",
			RelatedID:   req.ID,
			Action:      "Answer the request before waking more work.",
			Endpoint:    "/requests",
			CreatedAt:   firstNonEmpty(req.UpdatedAt, req.CreatedAt),
			Signals:     []string{"blocking_request"},
		})
	}

	for _, task := range state.Tasks {
		if !b.operatorAlertVisibleLocked(viewer, task.Channel, channel, allChannels) || taskIsTerminal(&task) || strings.TrimSpace(task.ArchivedAt) != "" {
			continue
		}
		taskChannel := normalizeChannelSlug(task.Channel)
		if task.CompletionEvidenceRequired && !task.CompletionEvidenceSatisfied {
			add(operatorAlert{
				ID:          "task:evidence:" + task.ID,
				Severity:    "warning",
				Source:      "task_contract",
				Title:       "Outcome evidence missing",
				Summary:     firstNonEmpty(task.CompletionBlocker, "Task cannot be completed until durable evidence is attached."),
				Channel:     taskChannel,
				RelatedType: "task",
				RelatedID:   task.ID,
				Action:      "Attach outcome evidence or record why it is not available.",
				Endpoint:    "/tasks",
				CreatedAt:   firstNonEmpty(task.UpdatedAt, task.CreatedAt),
				Signals:     []string{"completion_evidence_required"},
			})
		}
		limitState := normalizeTaskLimitState(task.Limits.LimitState)
		if limitState == "exhausted" || limitState == "paused" {
			add(operatorAlert{
				ID:          "task:budget:" + task.ID,
				Severity:    "critical",
				Source:      "task_budget",
				Title:       "Task execution limit reached",
				Summary:     firstNonEmpty(task.Limits.LastLimitReason, "Task execution is paused by its attempt, runtime, or cost limit."),
				Channel:     taskChannel,
				RelatedType: "task",
				RelatedID:   task.ID,
				Action:      "Review task limits before another run.",
				Endpoint:    "/tasks",
				CreatedAt:   firstNonEmpty(task.Limits.LastAttemptAt, task.UpdatedAt, task.CreatedAt),
				Signals:     []string{"limit_state:" + limitState},
			})
		}
		if task.ExecutionLock != nil {
			lock := normalizeTaskExecutionLock(task.ExecutionLock, now)
			if lock != nil {
				switch {
				case lock.Status == "expired":
					add(operatorAlert{
						ID:          "task:lock-expired:" + task.ID,
						Severity:    "warning",
						Source:      "execution_lock",
						Title:       "Execution lock expired",
						Summary:     "The task has an expired execution lock that may need review before resuming.",
						Channel:     taskChannel,
						RelatedType: "task",
						RelatedID:   task.ID,
						Action:      "Inspect the task trace and resume pack before waking the owner.",
						Endpoint:    "/execution-trace",
						CreatedAt:   firstNonEmpty(lock.HeartbeatAt, lock.AcquiredAt, task.UpdatedAt),
						Signals:     []string{"execution_lock_expired", lock.Owner},
					})
				case lock.Status == "active" && operatorLockHeartbeatStale(lock, now):
					add(operatorAlert{
						ID:          "task:lock-stale:" + task.ID,
						Severity:    "warning",
						Source:      "execution_lock",
						Title:       "Execution heartbeat is stale",
						Summary:     "The task still has an active execution lock, but the latest heartbeat is old.",
						Channel:     taskChannel,
						RelatedType: "task",
						RelatedID:   task.ID,
						Action:      "Check whether the run is still alive before starting another one.",
						Endpoint:    "/agent-sessions",
						CreatedAt:   firstNonEmpty(lock.HeartbeatAt, lock.AcquiredAt, task.UpdatedAt),
						Signals:     []string{"execution_lock_stale", lock.Owner},
					})
				}
			}
		}
		if state := latestTaskLivenessState(task); operatorLivenessNeedsAttention(state) {
			add(operatorAlert{
				ID:          "task:liveness:" + task.ID,
				Severity:    operatorSeverityFromLiveness(state),
				Source:      "liveness",
				Title:       "Agent progress needs review",
				Summary:     firstNonEmpty(latestTaskLivenessReason(task), "Latest liveness signal indicates the task needs follow-up."),
				Channel:     taskChannel,
				RelatedType: "task",
				RelatedID:   task.ID,
				Action:      "Review the execution trace before accepting completion.",
				Endpoint:    "/execution-trace",
				CreatedAt:   firstNonEmpty(task.UpdatedAt, task.CreatedAt),
				Signals:     []string{"liveness:" + state},
			})
		}
	}

	if usage.Total.TotalTokens >= 100000 {
		add(operatorAlert{
			ID:       "usage:high-token-volume",
			Severity: "info",
			Source:   "usage",
			Title:    "High token volume",
			Summary:  "The current usage snapshot has crossed 100k total tokens.",
			Action:   "Review usage by agent before starting large runs.",
			Endpoint: "/usage",
			Signals:  []string{"tokens:" + itoa(usage.Total.TotalTokens)},
		})
	}

	alerts = dedupeOperatorAlerts(alerts)
	sort.Slice(alerts, func(i, j int) bool {
		if operatorAlertSeverityRank(alerts[i].Severity) != operatorAlertSeverityRank(alerts[j].Severity) {
			return operatorAlertSeverityRank(alerts[i].Severity) > operatorAlertSeverityRank(alerts[j].Severity)
		}
		if alerts[i].CreatedAt != alerts[j].CreatedAt {
			return studioTimestampAfter(alerts[i].CreatedAt, alerts[j].CreatedAt)
		}
		return alerts[i].ID < alerts[j].ID
	})
	if len(alerts) > 12 {
		alerts = alerts[:12]
	}
	summary := map[string]int{"total": len(alerts)}
	status := "ok"
	for _, alert := range alerts {
		summary[alert.Severity]++
		summary[alert.Source]++
		if alert.Severity == "critical" {
			status = "blocked"
		} else if status == "ok" && alert.Severity != "info" {
			status = "degraded"
		}
	}
	return operatorAlertsResponse{
		GeneratedAt: now.Format(time.RFC3339),
		Persisted:   false,
		Status:      status,
		Summary:     summary,
		Alerts:      alerts,
	}
}

func (b *Broker) operatorAlertVisibleLocked(viewer, itemChannel, requestedChannel string, allChannels bool) bool {
	itemChannel = normalizeChannelSlug(itemChannel)
	if itemChannel == "" {
		if requestedChannel == "" && !allChannels {
			requestedChannel = "general"
		}
		if requestedChannel == "" {
			return true
		}
		return b.canAccessChannelLocked(viewer, requestedChannel)
	}
	if !allChannels && requestedChannel != "" && itemChannel != requestedChannel {
		return false
	}
	return b.canAccessChannelLocked(viewer, itemChannel)
}

func cloneTeamUsageState(in teamUsageState) teamUsageState {
	out := in
	if in.Agents != nil {
		out.Agents = make(map[string]usageTotals, len(in.Agents))
		for slug, usage := range in.Agents {
			out.Agents[slug] = usage
		}
	}
	return out
}

func normalizeOperatorAlertSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical", "blocked", "fail", "failed", "error":
		return "critical"
	case "warning", "warn", "degraded", "blocked_human":
		return "warning"
	case "info", "ok":
		return "info"
	default:
		return "warning"
	}
}

func operatorSeverityFromDoctor(severity runtimeDoctorSeverity) string {
	if severity == runtimeDoctorFail {
		return "critical"
	}
	return "warning"
}

func operatorSeverityFromLiveness(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "failed", "empty_response":
		return "critical"
	default:
		return "warning"
	}
}

func operatorLivenessNeedsAttention(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "blocked", "failed", "empty_response", "plan_only", "needs_followup":
		return true
	default:
		return false
	}
}

func operatorLockHeartbeatStale(lock *taskExecutionLock, now time.Time) bool {
	if lock == nil {
		return false
	}
	heartbeat := parseBrokerTimestamp(firstNonEmpty(lock.HeartbeatAt, lock.AcquiredAt))
	if heartbeat.IsZero() {
		return false
	}
	return now.UTC().Sub(heartbeat) > 30*time.Minute
}

func operatorAlertSeverityRank(severity string) int {
	switch normalizeOperatorAlertSeverity(severity) {
	case "critical":
		return 3
	case "warning":
		return 2
	default:
		return 1
	}
}

func dedupeOperatorAlerts(alerts []operatorAlert) []operatorAlert {
	seen := make(map[string]bool, len(alerts))
	out := make([]operatorAlert, 0, len(alerts))
	for _, alert := range alerts {
		if seen[alert.ID] {
			continue
		}
		seen[alert.ID] = true
		out = append(out, alert)
	}
	return out
}
