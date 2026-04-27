package team

import (
	"sort"
	"strings"
	"time"
)

const (
	brokerObservabilityHistoryLimit      = 48
	brokerObservabilitySampleMinInterval = 20 * time.Second
)

func cloneIntSliceMap(src map[string][]int) map[string][]int {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string][]int, len(src))
	for key, values := range src {
		out[key] = append([]int(nil), values...)
	}
	return out
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func (b *Broker) messageIndexSnapshotLocked() *brokerMessageIndexSnapshot {
	if b == nil {
		return nil
	}
	b.ensureMessageIndexesLocked()
	if len(b.messages) == 0 {
		return nil
	}
	byThread := make(map[string][]int)
	for channel, roots := range b.messageIndexesByThread {
		for rootID, indexes := range roots {
			key := channel + "\x00" + rootID
			byThread[key] = append([]int(nil), indexes...)
		}
	}
	return &brokerMessageIndexSnapshot{
		Size:           b.messageIndexSize,
		FirstID:        strings.TrimSpace(b.messageIndexFirstID),
		LastID:         strings.TrimSpace(b.messageIndexLastID),
		ByChannel:      cloneIntSliceMap(b.messageIndexesByChannel),
		ByToken:        cloneIntSliceMap(b.messageSearchIndexByToken),
		ByAuthor:       cloneIntSliceMap(b.messageSearchIndexByAuthor),
		ByThread:       byThread,
		ThreadRootByID: cloneStringMap(b.messageThreadRootByID),
	}
}

func indexesWithinBounds(indexes []int, size int) bool {
	for _, idx := range indexes {
		if idx < 0 || idx >= size {
			return false
		}
	}
	return true
}

func (b *Broker) restoreMessageIndexSnapshotLocked(snapshot *brokerMessageIndexSnapshot) bool {
	if b == nil || snapshot == nil {
		return false
	}
	size := len(b.messages)
	if size == 0 {
		return false
	}
	if snapshot.Size != size {
		return false
	}
	if strings.TrimSpace(snapshot.FirstID) != strings.TrimSpace(b.messages[0].ID) {
		return false
	}
	if strings.TrimSpace(snapshot.LastID) != strings.TrimSpace(b.messages[size-1].ID) {
		return false
	}
	for _, indexes := range snapshot.ByChannel {
		if !indexesWithinBounds(indexes, size) {
			return false
		}
	}
	for _, indexes := range snapshot.ByToken {
		if !indexesWithinBounds(indexes, size) {
			return false
		}
	}
	for _, indexes := range snapshot.ByAuthor {
		if !indexesWithinBounds(indexes, size) {
			return false
		}
	}
	threadIndexes := make(map[string]map[string][]int)
	for key, indexes := range snapshot.ByThread {
		if !indexesWithinBounds(indexes, size) {
			return false
		}
		channel, rootID, ok := strings.Cut(key, "\x00")
		if !ok || strings.TrimSpace(rootID) == "" {
			return false
		}
		channel = normalizeChannelSlug(channel)
		if channel == "" {
			channel = "general"
		}
		if threadIndexes[channel] == nil {
			threadIndexes[channel] = make(map[string][]int)
		}
		threadIndexes[channel][rootID] = append([]int(nil), indexes...)
	}
	b.messageIndexesByChannel = cloneIntSliceMap(snapshot.ByChannel)
	b.messageSearchIndexByToken = cloneIntSliceMap(snapshot.ByToken)
	b.messageSearchIndexByAuthor = cloneIntSliceMap(snapshot.ByAuthor)
	b.messageIndexesByThread = threadIndexes
	b.messageThreadRootByID = cloneStringMap(snapshot.ThreadRootByID)
	b.messageIndexSize = snapshot.Size
	b.messageIndexFirstID = strings.TrimSpace(snapshot.FirstID)
	b.messageIndexLastID = strings.TrimSpace(snapshot.LastID)
	if b.messageIndexesByChannel == nil {
		b.messageIndexesByChannel = make(map[string][]int)
	}
	if b.messageSearchIndexByToken == nil {
		b.messageSearchIndexByToken = make(map[string][]int)
	}
	if b.messageSearchIndexByAuthor == nil {
		b.messageSearchIndexByAuthor = make(map[string][]int)
	}
	if b.messageIndexesByThread == nil {
		b.messageIndexesByThread = make(map[string]map[string][]int)
	}
	if b.messageThreadRootByID == nil {
		b.messageThreadRootByID = make(map[string]string)
	}
	return true
}

func (b *Broker) observabilityHistorySnapshot(limit int) []brokerObservabilitySample {
	if b == nil || limit == 0 {
		return nil
	}
	b.observabilityMu.Lock()
	defer b.observabilityMu.Unlock()
	if len(b.observabilityHistory) == 0 {
		return nil
	}
	out := append([]brokerObservabilitySample(nil), b.observabilityHistory...)
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func observabilitySamplesEquivalent(a, b brokerObservabilitySample) bool {
	return a.ReadinessState == b.ReadinessState &&
		a.ReadinessStage == b.ReadinessStage &&
		a.PendingRequests == b.PendingRequests &&
		a.ActiveWatchdogs == b.ActiveWatchdogs &&
		a.SchedulerScheduled == b.SchedulerScheduled &&
		a.SchedulerDue == b.SchedulerDue &&
		a.GitHubPending == b.GitHubPending &&
		a.GitHubDeferred == b.GitHubDeferred &&
		a.GitHubOpened == b.GitHubOpened &&
		a.CloudBackupPending == b.CloudBackupPending &&
		a.CloudBackupFailures == b.CloudBackupFailures &&
		a.HotPath == b.HotPath &&
		a.HotPathMaxMs == b.HotPathMaxMs
}

func (b *Broker) recordObservabilitySampleLocked(now time.Time) {
	if b == nil {
		return
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	readiness := b.readinessSnapshot()
	httpHotPaths := b.httpMetricSnapshot(1)
	cloud := brokerCloudBackupRuntimeSnapshot()
	sample := brokerObservabilitySample{
		RecordedAt:          now.UTC().Format(time.RFC3339),
		ReadinessState:      strings.TrimSpace(readiness.State),
		ReadinessStage:      strings.TrimSpace(readiness.Stage),
		CloudBackupPending:  cloud.Pending,
		CloudBackupFailures: cloud.ConsecutiveFailures,
	}
	for _, req := range b.requests {
		if requestIsActive(req) {
			sample.PendingRequests++
		}
	}
	for _, alert := range b.watchdogs {
		if normalizeWatchdogStatus(alert.Status) == "active" {
			sample.ActiveWatchdogs++
		}
	}
	for _, job := range b.scheduler {
		job = normalizeSchedulerJob(job)
		if schedulerStatusIsTerminal(job.Status) {
			continue
		}
		sample.SchedulerScheduled++
		if schedulerJobDue(job, now) {
			sample.SchedulerDue++
		}
	}
	for _, task := range b.tasks {
		for _, publication := range []*taskGitHubPublication{task.IssuePublication, task.PRPublication} {
			if publication == nil {
				continue
			}
			switch normalizeTaskGitHubPublicationStatus(publication.Status) {
			case "pending":
				sample.GitHubPending++
			case "deferred":
				sample.GitHubDeferred++
			case "opened":
				sample.GitHubOpened++
			}
		}
	}
	if len(httpHotPaths) > 0 {
		sample.HotPath = httpHotPaths[0].Path
		sample.HotPathMaxMs = httpHotPaths[0].MaxDurationMs
	}

	b.observabilityMu.Lock()
	defer b.observabilityMu.Unlock()
	if len(b.observabilityHistory) > 0 {
		last := b.observabilityHistory[len(b.observabilityHistory)-1]
		if observabilitySamplesEquivalent(last, sample) {
			lastAt, err := time.Parse(time.RFC3339, last.RecordedAt)
			if err == nil && now.Sub(lastAt) < brokerObservabilitySampleMinInterval {
				return
			}
		}
	}
	b.observabilityHistory = append(b.observabilityHistory, sample)
	if len(b.observabilityHistory) > brokerObservabilityHistoryLimit {
		b.observabilityHistory = append([]brokerObservabilitySample(nil), b.observabilityHistory[len(b.observabilityHistory)-brokerObservabilityHistoryLimit:]...)
	}
}

func (b *Broker) hottestObservedPaths(limit int) []brokerHTTPMetric {
	metrics := b.httpMetricSnapshot(limit)
	sort.Slice(metrics, func(i, j int) bool {
		if metrics[i].MaxDurationMs == metrics[j].MaxDurationMs {
			return metrics[i].Path < metrics[j].Path
		}
		return metrics[i].MaxDurationMs > metrics[j].MaxDurationMs
	})
	return metrics
}
