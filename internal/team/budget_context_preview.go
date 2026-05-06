package team

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"
)

type budgetContextPreviewResponse struct {
	GeneratedAt string                     `json:"generated_at"`
	Persisted   bool                       `json:"persisted"`
	Status      string                     `json:"status"`
	Summary     map[string]int             `json:"summary"`
	Usage       budgetContextUsagePreview  `json:"usage"`
	Items       []budgetContextPreviewItem `json:"items"`
}

type budgetContextUsagePreview struct {
	TotalTokens   int     `json:"total_tokens"`
	SessionTokens int     `json:"session_tokens"`
	Requests      int     `json:"requests"`
	CostUsd       float64 `json:"cost_usd"`
	AgentCount    int     `json:"agent_count"`
}

type budgetContextPreviewItem struct {
	TaskID       string                `json:"task_id"`
	Title        string                `json:"title"`
	Channel      string                `json:"channel,omitempty"`
	Owner        string                `json:"owner,omitempty"`
	Status       string                `json:"status,omitempty"`
	BudgetState  string                `json:"budget_state"`
	ContextState string                `json:"context_state"`
	WouldWarn    bool                  `json:"would_warn"`
	WouldBlock   bool                  `json:"would_block"`
	Reasons      []string              `json:"reasons,omitempty"`
	Metrics      []budgetContextMetric `json:"metrics,omitempty"`
	Context      budgetContextEstimate `json:"context"`
	UpdatedAt    string                `json:"updated_at,omitempty"`
}

type budgetContextMetric struct {
	Name    string `json:"name"`
	Used    int64  `json:"used"`
	Limit   int64  `json:"limit,omitempty"`
	Unit    string `json:"unit"`
	Percent int    `json:"percent,omitempty"`
	State   string `json:"state"`
}

type budgetContextEstimate struct {
	MessageCount  int      `json:"message_count"`
	ArtifactCount int      `json:"artifact_count"`
	PlanCount     int      `json:"plan_count"`
	LivenessCount int      `json:"liveness_count"`
	ApproxChars   int      `json:"approx_chars"`
	Signals       []string `json:"signals,omitempty"`
}

func (b *Broker) handleBudgetContextPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	taskID := strings.TrimSpace(r.URL.Query().Get("task_id"))
	limit := parsePositiveLimit(r.URL.Query().Get("limit"), 25)
	allChannels := channel == "" || strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all_channels")), "true")

	b.mu.RLock()
	payload := b.buildBudgetContextPreviewLocked(viewer, channel, allChannels, taskID, limit, time.Now().UTC())
	b.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) buildBudgetContextPreviewLocked(viewer, channel string, allChannels bool, taskID string, limit int, now time.Time) budgetContextPreviewResponse {
	usage := cloneTeamUsageState(b.usage)
	items := make([]budgetContextPreviewItem, 0)
	for _, task := range b.tasks {
		if taskID != "" && strings.TrimSpace(task.ID) != taskID {
			continue
		}
		taskChannel := normalizeChannelSlug(task.Channel)
		if taskChannel == "" {
			taskChannel = "general"
		}
		if !allChannels && channel != "" && taskChannel != channel {
			continue
		}
		if !b.canAccessChannelLocked(viewer, taskChannel) {
			continue
		}
		if taskID == "" && (taskIsTerminal(&task) || strings.TrimSpace(task.ArchivedAt) != "") {
			continue
		}
		copyTask := task
		normalizeTaskLimits(&copyTask)
		items = append(items, buildBudgetContextPreviewItem(copyTask, b.messages, now))
	}
	sort.Slice(items, func(i, j int) bool {
		leftRank := budgetContextStateRank(items[i])
		rightRank := budgetContextStateRank(items[j])
		if leftRank != rightRank {
			return leftRank > rightRank
		}
		if items[i].UpdatedAt != items[j].UpdatedAt {
			return studioTimestampAfter(items[i].UpdatedAt, items[j].UpdatedAt)
		}
		return items[i].TaskID < items[j].TaskID
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	summary := map[string]int{"total": len(items)}
	status := "ok"
	for _, item := range items {
		summary["budget_"+item.BudgetState]++
		summary["context_"+item.ContextState]++
		if item.WouldWarn {
			summary["would_warn"]++
		}
		if item.WouldBlock {
			summary["would_block"]++
			status = "blocked"
		}
	}
	if status == "ok" && summary["would_warn"] > 0 {
		status = "warning"
	}
	return budgetContextPreviewResponse{
		GeneratedAt: now.Format(time.RFC3339),
		Persisted:   false,
		Status:      status,
		Summary:     summary,
		Usage: budgetContextUsagePreview{
			TotalTokens:   usage.Total.TotalTokens,
			SessionTokens: usage.Session.TotalTokens,
			Requests:      usage.Total.Requests,
			CostUsd:       usage.Total.CostUsd,
			AgentCount:    len(usage.Agents),
		},
		Items: items,
	}
}

func buildBudgetContextPreviewItem(task teamTask, messages []channelMessage, now time.Time) budgetContextPreviewItem {
	metrics := buildBudgetContextMetrics(task.Limits)
	reasons := make([]string, 0)
	wouldBlock := false
	wouldWarn := false
	budgetState := "ok"
	if len(metrics) == 0 {
		budgetState = "unbounded"
		reasons = append(reasons, "no explicit task budget configured")
	}
	for _, metric := range metrics {
		switch metric.State {
		case "exhausted":
			wouldBlock = true
			budgetState = "exhausted"
			reasons = append(reasons, metric.Name+" limit reached")
		case "warning":
			if budgetState != "exhausted" {
				budgetState = "warning"
			}
			wouldWarn = true
			reasons = append(reasons, metric.Name+" usage is near its limit")
		}
	}
	if normalizeTaskLimitState(task.Limits.LimitState) == "paused" {
		wouldBlock = true
		budgetState = "paused"
		reasons = append(reasons, firstNonEmpty(task.Limits.LastLimitReason, "task budget is paused"))
	}
	context := buildBudgetContextEstimate(task, messages, now)
	contextState := "ok"
	if context.ApproxChars >= 24000 {
		contextState = "high"
		wouldWarn = true
		reasons = append(reasons, "context payload is large")
	} else if context.ApproxChars >= 12000 {
		contextState = "warning"
		wouldWarn = true
		reasons = append(reasons, "context payload is growing")
	}
	if state := latestTaskLivenessState(task); operatorLivenessNeedsAttention(state) {
		wouldWarn = true
		reasons = append(reasons, "latest liveness state needs review")
	}
	return budgetContextPreviewItem{
		TaskID:       strings.TrimSpace(task.ID),
		Title:        truncateSummary(firstNonEmpty(task.Title, task.Outcome), 140),
		Channel:      normalizeChannelSlug(task.Channel),
		Owner:        strings.TrimSpace(task.Owner),
		Status:       normalizeTaskStatus(task.Status),
		BudgetState:  budgetState,
		ContextState: contextState,
		WouldWarn:    wouldWarn,
		WouldBlock:   wouldBlock,
		Reasons:      compactStringList(reasons),
		Metrics:      metrics,
		Context:      context,
		UpdatedAt:    firstNonEmpty(task.UpdatedAt, task.CreatedAt),
	}
}

func buildBudgetContextMetrics(limits taskExecutionLimits) []budgetContextMetric {
	out := make([]budgetContextMetric, 0, 3)
	if limits.MaxAttempts > 0 {
		out = append(out, budgetContextMetricFor("attempts", int64(limits.AttemptsUsed), int64(limits.MaxAttempts), "count"))
	}
	if limits.MaxRuntimeMinutes > 0 {
		out = append(out, budgetContextMetricFor("runtime", limits.RuntimeMsUsed, int64(limits.MaxRuntimeMinutes)*int64(timeMinuteMs()), "ms"))
	}
	if limits.MaxCostCents > 0 {
		out = append(out, budgetContextMetricFor("cost", int64(limits.CostCentsUsed), int64(limits.MaxCostCents), "cents"))
	}
	return out
}

func budgetContextMetricFor(name string, used, limit int64, unit string) budgetContextMetric {
	percent := 0
	state := "ok"
	if limit > 0 {
		percent = int((used * 100) / limit)
		if used >= limit {
			state = "exhausted"
		} else if percent >= 80 {
			state = "warning"
		}
	}
	return budgetContextMetric{Name: name, Used: used, Limit: limit, Unit: unit, Percent: percent, State: state}
}

func buildBudgetContextEstimate(task teamTask, messages []channelMessage, now time.Time) budgetContextEstimate {
	channel := normalizeChannelSlug(task.Channel)
	if channel == "" {
		channel = "general"
	}
	estimate := budgetContextEstimate{
		ArtifactCount: len(task.Artifacts),
		PlanCount:     len(task.PlanRevisions),
	}
	if latestTaskLivenessState(task) != "" {
		estimate.LivenessCount = 1
	}
	estimate.ApproxChars += len(task.Title) + len(task.Details) + len(task.Outcome) + len(task.OutcomeEvidence)
	for _, artifact := range task.Artifacts {
		estimate.ApproxChars += len(artifact.Title) + len(artifact.Summary) + len(artifact.Path) + len(artifact.URL)
	}
	for _, revision := range task.PlanRevisions {
		estimate.ApproxChars += len(revision.Summary) + len(revision.Content)
	}
	cutoff := now.Add(-24 * time.Hour)
	for i := len(messages) - 1; i >= 0 && estimate.MessageCount < 50; i-- {
		msg := messages[i]
		if normalizeChannelSlug(msg.Channel) != channel {
			continue
		}
		msgTime := parseBrokerTimestamp(msg.Timestamp)
		if !msgTime.IsZero() && msgTime.Before(cutoff) {
			continue
		}
		if strings.Contains(strings.ToLower(msg.Content+" "+msg.Title), strings.ToLower(task.ID)) || estimate.MessageCount < 12 {
			estimate.MessageCount++
			estimate.ApproxChars += len(msg.Title) + len(msg.Content)
		}
	}
	if estimate.MessageCount > 0 {
		estimate.Signals = append(estimate.Signals, "recent_messages")
	}
	if estimate.ArtifactCount > 0 {
		estimate.Signals = append(estimate.Signals, "artifacts")
	}
	if estimate.PlanCount > 0 {
		estimate.Signals = append(estimate.Signals, "plan_revisions")
	}
	if estimate.LivenessCount > 0 {
		estimate.Signals = append(estimate.Signals, "liveness_history")
	}
	estimate.Signals = compactStringList(estimate.Signals)
	return estimate
}

func budgetContextStateRank(item budgetContextPreviewItem) int {
	if item.WouldBlock {
		return 4
	}
	if item.WouldWarn {
		return 3
	}
	if item.BudgetState == "unbounded" {
		return 2
	}
	return 1
}
