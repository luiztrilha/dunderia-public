package team

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nex-crm/wuphf/internal/config"
)

type SessionAgentObservability struct {
	Slug                 string `json:"slug"`
	Channel              string `json:"channel,omitempty"`
	Status               string `json:"status,omitempty"`
	Activity             string `json:"activity,omitempty"`
	Detail               string `json:"detail,omitempty"`
	QueueDepth           int    `json:"queue_depth,omitempty"`
	QuickReplyQueueDepth int    `json:"quick_reply_queue_depth,omitempty"`
	ActiveTaskID         string `json:"active_task_id,omitempty"`
	ActiveSince          string `json:"active_since,omitempty"`
	WorkingDirectory     string `json:"working_directory,omitempty"`
}

type SessionObservabilitySnapshot struct {
	GeneratedAt            string                      `json:"generated_at,omitempty"`
	SessionMode            string                      `json:"session_mode,omitempty"`
	DirectAgent            string                      `json:"direct_agent,omitempty"`
	Provider               string                      `json:"provider,omitempty"`
	ReadinessReady         bool                        `json:"readiness_ready,omitempty"`
	ReadinessState         string                      `json:"readiness_state,omitempty"`
	ReadinessStage         string                      `json:"readiness_stage,omitempty"`
	ReadinessSummary       string                      `json:"readiness_summary,omitempty"`
	PendingRequests        int                         `json:"pending_requests,omitempty"`
	ActiveAgents           int                         `json:"active_agents,omitempty"`
	QueuedTurns            int                         `json:"queued_turns,omitempty"`
	SchedulerScheduled     int                         `json:"scheduler_scheduled,omitempty"`
	SchedulerDue           int                         `json:"scheduler_due,omitempty"`
	SchedulerTerminal      int                         `json:"scheduler_terminal,omitempty"`
	GitHubPending          int                         `json:"github_pending,omitempty"`
	GitHubDeferred         int                         `json:"github_deferred,omitempty"`
	GitHubOpened           int                         `json:"github_opened,omitempty"`
	GitHubLastAuditAt      string                      `json:"github_last_audit_at,omitempty"`
	GitHubLastAuditSuccess string                      `json:"github_last_audit_success,omitempty"`
	GitHubLastAuditError   string                      `json:"github_last_audit_error,omitempty"`
	GitHubOpenedTotal      int                         `json:"github_opened_total,omitempty"`
	GitHubUpdatedTotal     int                         `json:"github_updated_total,omitempty"`
	GitHubDeferredTotal    int                         `json:"github_deferred_total,omitempty"`
	GitHubClosedTotal      int                         `json:"github_closed_total,omitempty"`
	GitHubReopenedTotal    int                         `json:"github_reopened_total,omitempty"`
	GitHubRetriedTotal     int                         `json:"github_retried_total,omitempty"`
	GitHubSyncedTotal      int                         `json:"github_synced_total,omitempty"`
	CloudBackup            brokerCloudBackupRuntime    `json:"cloud_backup,omitempty"`
	HTTPHotPaths           []brokerHTTPMetric          `json:"http_hot_paths,omitempty"`
	History                []brokerObservabilitySample `json:"history,omitempty"`
	Agents                 []SessionAgentObservability `json:"agents,omitempty"`
}

func (s SessionObservabilitySnapshot) Available() bool {
	return len(s.Agents) > 0 || s.ActiveAgents > 0 || s.QueuedTurns > 0 || s.PendingRequests > 0 || s.SchedulerScheduled > 0 || s.SchedulerDue > 0 || s.SchedulerTerminal > 0 || strings.TrimSpace(s.ReadinessState) != "" || s.GitHubPending > 0 || s.GitHubDeferred > 0 || s.GitHubOpened > 0 || strings.TrimSpace(s.GitHubLastAuditAt) != "" || s.GitHubOpenedTotal > 0 || s.GitHubUpdatedTotal > 0 || s.GitHubDeferredTotal > 0 || s.GitHubClosedTotal > 0 || s.GitHubReopenedTotal > 0 || s.GitHubRetriedTotal > 0 || s.GitHubSyncedTotal > 0 || s.CloudBackup.Pending || strings.TrimSpace(s.CloudBackup.LastAttemptAt) != "" || len(s.HTTPHotPaths) > 0 || len(s.History) > 0
}

func (s SessionObservabilitySnapshot) FormatLines() []string {
	if !s.Available() {
		return nil
	}
	lines := []string{
		"- Provider: " + firstNonEmpty(strings.TrimSpace(s.Provider), "unknown"),
		"- Readiness: " + firstNonEmpty(strings.TrimSpace(s.ReadinessState), "unknown") + " (" + firstNonEmpty(strings.TrimSpace(s.ReadinessStage), "broker") + ")",
		"- Active agents: " + itoa(s.ActiveAgents),
		"- Queued turns: " + itoa(s.QueuedTurns),
		"- Pending requests: " + itoa(s.PendingRequests),
		"- Scheduler: scheduled=" + itoa(s.SchedulerScheduled) + " due=" + itoa(s.SchedulerDue) + " terminal=" + itoa(s.SchedulerTerminal),
		"- GitHub publications: pending=" + itoa(s.GitHubPending) + " deferred=" + itoa(s.GitHubDeferred) + " opened=" + itoa(s.GitHubOpened),
	}
	if strings.TrimSpace(s.ReadinessSummary) != "" {
		lines = append(lines, "- Readiness summary: "+truncate(strings.TrimSpace(s.ReadinessSummary), 140))
	}
	if strings.TrimSpace(s.GitHubLastAuditAt) != "" {
		lines = append(lines, "- GitHub audit last run: "+strings.TrimSpace(s.GitHubLastAuditAt))
	}
	if strings.TrimSpace(s.GitHubLastAuditSuccess) != "" {
		lines = append(lines, "- GitHub audit last success: "+strings.TrimSpace(s.GitHubLastAuditSuccess))
	}
	if strings.TrimSpace(s.GitHubLastAuditError) != "" {
		lines = append(lines, "- GitHub audit error: "+truncate(strings.TrimSpace(s.GitHubLastAuditError), 120))
	}
	if s.GitHubOpenedTotal > 0 || s.GitHubUpdatedTotal > 0 || s.GitHubDeferredTotal > 0 || s.GitHubClosedTotal > 0 || s.GitHubReopenedTotal > 0 || s.GitHubRetriedTotal > 0 || s.GitHubSyncedTotal > 0 {
		lines = append(lines,
			"- GitHub totals: opened="+itoa(s.GitHubOpenedTotal)+
				" updated="+itoa(s.GitHubUpdatedTotal)+
				" deferred="+itoa(s.GitHubDeferredTotal)+
				" closed="+itoa(s.GitHubClosedTotal)+
				" reopened="+itoa(s.GitHubReopenedTotal)+
				" retried="+itoa(s.GitHubRetriedTotal)+
				" synced="+itoa(s.GitHubSyncedTotal),
		)
	}
	if s.CloudBackup.Pending || strings.TrimSpace(s.CloudBackup.LastAttemptAt) != "" || strings.TrimSpace(s.CloudBackup.LastSuccessAt) != "" || strings.TrimSpace(s.CloudBackup.LastError) != "" {
		lines = append(lines, "- Cloud backup: pending="+strconv.FormatBool(s.CloudBackup.Pending)+" failures="+itoa(s.CloudBackup.ConsecutiveFailures)+" last_attempt="+firstNonEmpty(strings.TrimSpace(s.CloudBackup.LastAttemptAt), "-")+" last_success="+firstNonEmpty(strings.TrimSpace(s.CloudBackup.LastSuccessAt), "-"))
		if strings.TrimSpace(s.CloudBackup.LastError) != "" {
			lines = append(lines, "- Cloud backup error: "+truncate(strings.TrimSpace(s.CloudBackup.LastError), 120))
		}
	}
	if len(s.History) > 0 {
		capacity := len(s.History)
		if capacity > 3 {
			capacity = 3
		}
		summaries := make([]string, 0, capacity)
		start := 0
		if len(s.History) > 3 {
			start = len(s.History) - 3
		}
		for _, sample := range s.History[start:] {
			summaries = append(summaries, strings.TrimSpace(sample.RecordedAt)+" due="+itoa(sample.SchedulerDue)+" req="+itoa(sample.PendingRequests)+" gh="+itoa(sample.GitHubPending+sample.GitHubDeferred+sample.GitHubOpened))
		}
		lines = append(lines, "- Observability history: "+strings.Join(summaries, " | "))
	}
	for _, metric := range s.HTTPHotPaths {
		lines = append(lines, "- HTTP hot path: "+metric.Path+" max="+itoa(int(metric.MaxDurationMs))+"ms last="+itoa(int(metric.LastDurationMs))+"ms requests="+itoa(metric.Requests))
	}
	for _, agent := range s.Agents {
		parts := []string{"- @" + agent.Slug}
		if channel := strings.TrimSpace(agent.Channel); channel != "" && channel != "general" {
			parts = append(parts, "#"+channel)
		}
		if strings.TrimSpace(agent.Status) != "" {
			parts = append(parts, "status="+agent.Status)
		}
		if strings.TrimSpace(agent.Activity) != "" {
			parts = append(parts, "activity="+agent.Activity)
		}
		if agent.QueueDepth > 0 {
			parts = append(parts, "queue="+itoa(agent.QueueDepth))
		}
		if agent.QuickReplyQueueDepth > 0 {
			parts = append(parts, "quick="+itoa(agent.QuickReplyQueueDepth))
		}
		if strings.TrimSpace(agent.ActiveTaskID) != "" {
			parts = append(parts, "task="+agent.ActiveTaskID)
		}
		if strings.TrimSpace(agent.WorkingDirectory) != "" {
			parts = append(parts, "wd="+agent.WorkingDirectory)
		}
		if strings.TrimSpace(agent.Detail) != "" {
			parts = append(parts, "detail="+truncate(agent.Detail, 120))
		}
		lines = append(lines, strings.Join(parts, " · "))
	}
	return lines
}

func (l *Launcher) SessionObservabilitySnapshot() SessionObservabilitySnapshot {
	now := time.Now().UTC()
	snapshot := SessionObservabilitySnapshot{
		GeneratedAt: now.Format(time.RFC3339),
		SessionMode: SessionModeOffice,
	}
	if l == nil {
		snapshot.Provider = config.ResolveLLMProvider("")
		return snapshot
	}
	snapshot.Provider = strings.TrimSpace(l.provider)
	if strings.TrimSpace(snapshot.Provider) == "" {
		snapshot.Provider = config.ResolveLLMProvider("")
	}
	if l.broker != nil {
		snapshot.SessionMode, snapshot.DirectAgent = l.broker.SessionModeState()
		if strings.TrimSpace(snapshot.Provider) == "" {
			l.broker.mu.Lock()
			snapshot.Provider = strings.TrimSpace(l.broker.runtimeProvider)
			l.broker.mu.Unlock()
		}
		readiness := l.broker.readinessSnapshot()
		snapshot.ReadinessReady = readiness.Ready
		snapshot.ReadinessState = strings.TrimSpace(readiness.State)
		snapshot.ReadinessStage = strings.TrimSpace(readiness.Stage)
		snapshot.ReadinessSummary = strings.TrimSpace(readiness.Summary)
		snapshot.CloudBackup = brokerCloudBackupRuntimeSnapshot()
		snapshot.HTTPHotPaths = l.broker.httpMetricSnapshot(3)
		snapshot.History = l.broker.observabilityHistorySnapshot(8)
	}

	l.headlessMu.Lock()
	active := make(map[string]*headlessCodexActiveTurn, len(l.headlessActive))
	for slug, state := range l.headlessActive {
		active[slug] = state
	}
	queues := make(map[string][]headlessCodexTurn, len(l.headlessQueues))
	for slug, queue := range l.headlessQueues {
		queues[slug] = append([]headlessCodexTurn(nil), queue...)
	}
	quickQueues := make(map[string][]headlessCodexTurn, len(l.headlessQuickQueues))
	for slug, queue := range l.headlessQuickQueues {
		quickQueues[slug] = append([]headlessCodexTurn(nil), queue...)
	}
	l.headlessMu.Unlock()

	activity := make(map[string]agentActivitySnapshot)
	memberSlugs := make(map[string]struct{})
	if l.broker != nil {
		l.broker.mu.Lock()
		for _, req := range l.broker.requests {
			if requestIsActive(req) {
				snapshot.PendingRequests++
			}
		}
		for _, job := range l.broker.scheduler {
			if schedulerJobIsTerminal(job) {
				snapshot.SchedulerTerminal++
				continue
			}
			snapshot.SchedulerScheduled++
			if schedulerJobDue(job, now) {
				snapshot.SchedulerDue++
			}
		}
		for _, task := range l.broker.tasks {
			for _, publication := range []*taskGitHubPublication{task.IssuePublication, task.PRPublication} {
				if publication == nil {
					continue
				}
				switch normalizeTaskGitHubPublicationStatus(publication.Status) {
				case "pending":
					snapshot.GitHubPending++
				case "deferred":
					snapshot.GitHubDeferred++
				case "opened":
					snapshot.GitHubOpened++
				}
			}
		}
		snapshot.GitHubLastAuditAt = strings.TrimSpace(l.broker.gitHubPublicationAudit.LastRunAt)
		snapshot.GitHubLastAuditSuccess = strings.TrimSpace(l.broker.gitHubPublicationAudit.LastSuccessAt)
		snapshot.GitHubLastAuditError = strings.TrimSpace(l.broker.gitHubPublicationAudit.LastError)
		snapshot.GitHubOpenedTotal = l.broker.gitHubPublicationAudit.OpenedTotal
		snapshot.GitHubUpdatedTotal = l.broker.gitHubPublicationAudit.UpdatedTotal
		snapshot.GitHubDeferredTotal = l.broker.gitHubPublicationAudit.DeferredTotal
		snapshot.GitHubClosedTotal = l.broker.gitHubPublicationAudit.ClosedTotal
		snapshot.GitHubReopenedTotal = l.broker.gitHubPublicationAudit.ReopenedTotal
		snapshot.GitHubRetriedTotal = l.broker.gitHubPublicationAudit.RetriedTotal
		snapshot.GitHubSyncedTotal = l.broker.gitHubPublicationAudit.SyncedTotal
		for laneKey, state := range l.broker.activity {
			activity[laneKey] = state
		}
		for _, member := range l.broker.members {
			if member.Slug != "" {
				memberSlugs[member.Slug] = struct{}{}
			}
		}
		l.broker.mu.Unlock()
	}
	entryMap := make(map[string]SessionAgentObservability)
	touchEntry := func(key string) (SessionAgentObservability, bool) {
		key = strings.TrimSpace(key)
		if key == "" {
			return SessionAgentObservability{}, false
		}
		entry, ok := entryMap[key]
		if !ok {
			channel, slug := parseAgentLaneKey(key)
			entry = SessionAgentObservability{
				Slug:    slug,
				Channel: channel,
			}
		}
		return entry, true
	}

	for laneKey, state := range activity {
		entry, ok := entryMap[laneKey]
		if !ok {
			channel, slug := parseAgentLaneKey(laneKey)
			entry = SessionAgentObservability{Slug: slug, Channel: channel}
		}
		entry.Status = strings.TrimSpace(state.Status)
		entry.Activity = strings.TrimSpace(state.Activity)
		entry.Detail = strings.TrimSpace(state.Detail)
		entryMap[laneKey] = entry
	}
	for laneKey, queue := range queues {
		if entry, ok := touchEntry(laneKey); ok {
			entry.QueueDepth = len(queue)
			entryMap[laneKey] = entry
			snapshot.QueuedTurns += len(queue)
		}
	}
	for laneKey, queue := range quickQueues {
		if entry, ok := touchEntry(laneKey); ok {
			entry.QuickReplyQueueDepth = len(queue)
			entryMap[laneKey] = entry
			snapshot.QueuedTurns += len(queue)
		}
	}
	for laneKey, state := range active {
		if state == nil {
			continue
		}
		entry, ok := entryMap[laneKey]
		if !ok {
			channel, slug := parseAgentLaneKey(laneKey)
			entry = SessionAgentObservability{Slug: slug, Channel: channel}
		}
		entry.ActiveTaskID = strings.TrimSpace(state.Turn.TaskID)
		entry.ActiveSince = state.StartedAt.UTC().Format(time.RFC3339)
		entry.WorkingDirectory = strings.TrimSpace(state.WorkspaceDir)
		if entry.Status == "" {
			entry.Status = "active"
		}
		if entry.Activity == "" {
			entry.Activity = "running"
		}
		entryMap[laneKey] = entry
	}
	for slug := range memberSlugs {
		alreadyPresent := false
		for _, entry := range entryMap {
			if entry.Slug == slug {
				alreadyPresent = true
				break
			}
		}
		if alreadyPresent {
			continue
		}
		entryMap[agentLaneKey("", slug)] = SessionAgentObservability{
			Slug:    slug,
			Channel: "general",
		}
	}

	keys := make([]string, 0, len(entryMap))
	for key := range entryMap {
		if strings.TrimSpace(entryMap[key].Slug) != "" {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		left := entryMap[keys[i]]
		right := entryMap[keys[j]]
		if left.Slug == right.Slug {
			return left.Channel < right.Channel
		}
		return left.Slug < right.Slug
	})

	snapshot.Agents = make([]SessionAgentObservability, 0, len(keys))
	for _, key := range keys {
		entry := entryMap[key]
		if entry.Status == "" {
			entry.Status = "idle"
		}
		if entry.Activity == "" {
			entry.Activity = "idle"
		}
		if entry.Status == "active" {
			snapshot.ActiveAgents++
		}
		snapshot.Agents = append(snapshot.Agents, entry)
	}

	return snapshot
}

func itoa(v int) string {
	return strconv.Itoa(v)
}
