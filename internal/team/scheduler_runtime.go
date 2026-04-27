package team

import (
	"fmt"
	"strings"
	"time"
)

func (b *Broker) DueSchedulerJobs() []schedulerJob {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pruneSchedulerJobsLocked(time.Now().UTC())
	return append([]schedulerJob(nil), b.dueSchedulerJobsLocked(time.Now().UTC())...)
}

func (b *Broker) UpdateSchedulerJobState(slug string, nextRun time.Time, status string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now().UTC()
	b.pruneSchedulerJobsLocked(now)
	for i := range b.scheduler {
		if strings.TrimSpace(b.scheduler[i].Slug) != strings.TrimSpace(slug) {
			continue
		}
		nextState, err := resolveSchedulerJobState(b.scheduler[i], status, nextRun, now)
		if err != nil {
			return err
		}
		b.scheduler[i] = nextState
		return b.saveLocked()
	}
	return fmt.Errorf("scheduler job %q not found", slug)
}

func (b *Broker) TaskByID(taskID string) (teamTask, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if idx := b.findTaskIndexByIDLocked(taskID); idx >= 0 {
		return b.tasks[idx], true
	}
	return teamTask{}, false
}

func (b *Broker) linkTaskToSourceMessage(taskID, sourceMessageID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	idx := b.findTaskIndexByIDLocked(taskID)
	if idx < 0 {
		return false
	}
	threadRoot := firstNonEmpty(b.threadRootFromMessageIDLocked(strings.TrimSpace(sourceMessageID)), strings.TrimSpace(sourceMessageID))
	if threadRoot == "" {
		return false
	}

	task := &b.tasks[idx]
	taskThreadRoot := firstNonEmpty(b.threadRootFromMessageIDLocked(strings.TrimSpace(task.ThreadID)), strings.TrimSpace(task.ThreadID))
	sourceThreadRoot := firstNonEmpty(b.threadRootFromMessageIDLocked(strings.TrimSpace(task.SourceMessageID)), strings.TrimSpace(task.SourceMessageID))
	if taskThreadRoot == threadRoot || sourceThreadRoot == threadRoot {
		return false
	}
	if strings.TrimSpace(task.SourceMessageID) != "" {
		return false
	}

	task.SourceMessageID = threadRoot
	if err := b.saveLocked(); err != nil {
		task.SourceMessageID = ""
		return false
	}
	return true
}

func (b *Broker) FindTask(channel, taskID string) (teamTask, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	channel = normalizeTaskIndexChannel(channel)
	if idx := b.findTaskIndexByIDLocked(taskID); idx >= 0 {
		task := b.tasks[idx]
		if normalizeTaskIndexChannel(task.Channel) == channel {
			return task, true
		}
	}
	return teamTask{}, false
}

func (b *Broker) ActiveTaskByOwner(owner string, channel ...string) (teamTask, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	targetChannel := ""
	if len(channel) > 0 {
		targetChannel = channel[0]
	}
	return b.activeTaskByOwnerLocked(owner, targetChannel)
}

func (b *Broker) TasksByType(taskType string) []teamTask {
	b.mu.Lock()
	defer b.mu.Unlock()
	return coalesceTaskView(b.tasksByTypeLocked(taskType))
}

func (b *Broker) pruneSchedulerJobsLocked(now time.Time) {
	if len(b.scheduler) == 0 {
		return
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	seen := make(map[string]struct{}, len(b.scheduler))
	pruned := make([]schedulerJob, 0, len(b.scheduler))
	for i := len(b.scheduler) - 1; i >= 0; i-- {
		job := normalizeSchedulerJob(b.scheduler[i])
		if schedulerJobIsTerminal(job) {
			lastRun := strings.TrimSpace(job.LastRun)
			if lastRun != "" {
				if parsed, err := time.Parse(time.RFC3339, lastRun); err == nil && now.Sub(parsed) > schedulerTerminalRetention {
					continue
				}
			}
		}
		key := schedulerJobSemanticKey(job)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		pruned = append(pruned, job)
	}
	for left, right := 0, len(pruned)-1; left < right; left, right = left+1, right-1 {
		pruned[left], pruned[right] = pruned[right], pruned[left]
	}
	b.scheduler = pruned
}

func (b *Broker) FindRequest(channel, requestID string) (humanInterview, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		channel = "general"
	}
	for _, req := range b.requests {
		reqChannel := normalizeChannelSlug(req.Channel)
		if reqChannel == "" {
			reqChannel = "general"
		}
		if reqChannel != channel {
			continue
		}
		if strings.TrimSpace(req.ID) == strings.TrimSpace(requestID) {
			return req, true
		}
	}
	return humanInterview{}, false
}

func (b *Broker) UpdateSkillExecutionByWorkflowKey(workflowKey, status string, when time.Time) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.skills {
		if strings.TrimSpace(b.skills[i].WorkflowKey) != strings.TrimSpace(workflowKey) {
			continue
		}
		if !when.IsZero() {
			b.skills[i].LastExecutionAt = when.UTC().Format(time.RFC3339)
		}
		b.skills[i].LastExecutionStatus = strings.TrimSpace(status)
		b.skills[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		return b.saveLocked()
	}
	return nil
}
