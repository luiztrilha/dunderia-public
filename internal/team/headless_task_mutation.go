package team

import (
	"fmt"
	"strings"
	"time"
)

type headlessTaskMutationInput struct {
	Action        string
	Channel       string
	ExecutionKey  string
	ID            string
	Title         string
	Details       string
	Owner         string
	CreatedBy     string
	TaskType      string
	ExecutionMode string
	WorkspacePath string
	DependsOn     []string
}

func (b *Broker) ApplyHeadlessTaskMutation(input headlessTaskMutationInput) (teamTask, error) {
	if b == nil {
		return teamTask{}, fmt.Errorf("broker unavailable")
	}
	action := strings.TrimSpace(input.Action)
	channel := normalizeChannelSlug(input.Channel)
	if channel == "" {
		channel = "general"
	}
	now := time.Now().UTC().Format(time.RFC3339)

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.findChannelLocked(channel) == nil {
		return teamTask{}, fmt.Errorf("channel not found")
	}

	if action == "create" {
		if strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.CreatedBy) == "" {
			return teamTask{}, fmt.Errorf("title and created_by required")
		}
		validated, err := b.validateStrictTaskPlanLocked(channel, strings.TrimSpace(input.CreatedBy), []plannedTaskSpec{{
			ExecutionKey:  input.ExecutionKey,
			Title:         input.Title,
			Assignee:      input.Owner,
			Details:       input.Details,
			TaskType:      input.TaskType,
			ExecutionMode: input.ExecutionMode,
			WorkspacePath: input.WorkspacePath,
			DependsOn:     append([]string(nil), input.DependsOn...),
		}})
		if err != nil {
			return teamTask{}, err
		}
		item := validated[0]
		b.counter++
		task := teamTask{
			ID:            fmt.Sprintf("task-%d", b.counter),
			Channel:       item.Channel,
			ExecutionKey:  item.ExecutionKey,
			Title:         item.Title,
			Details:       item.Details,
			Owner:         item.Owner,
			Status:        "open",
			CreatedBy:     strings.TrimSpace(input.CreatedBy),
			TaskType:      item.TaskType,
			PipelineID:    item.PipelineID,
			ExecutionMode: item.ExecutionMode,
			ReviewState:   item.ReviewState,
			WorkspacePath: item.WorkspacePath,
			DependsOn:     append([]string(nil), item.ResolvedDepIDs...),
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if task.Owner != "" {
			task.Status = "in_progress"
		}
		if len(task.DependsOn) > 0 && b.hasUnresolvedDepsLocked(&task) {
			task.Blocked = true
			task.Status = "open"
		}
		b.ensureTaskOwnerChannelMembershipLocked(task.Channel, task.Owner)
		b.queueTaskBehindActiveOwnerLaneLocked(&task)
		b.scheduleTaskLifecycleLocked(&task)
		if err := b.syncTaskWorktreeLocked(&task); err != nil {
			return teamTask{}, err
		}
		issueQueued, prQueued := b.maybeQueueTaskGitHubPublicationLocked(&task, "", action, nil)
		b.tasks = append(b.tasks, task)
		b.appendActionLocked("task_created", "office", task.Channel, task.CreatedBy, truncateSummary(task.Title, 140), task.ID)
		if err := b.saveLocked(); err != nil {
			return teamTask{}, err
		}
		if issueQueued {
			b.publishTaskIssueSoon(task.ID)
		}
		if prQueued {
			b.publishTaskPRSoon(task.ID)
		}
		return task, nil
	}

	requestedID := strings.TrimSpace(input.ID)
	for i := range b.tasks {
		if b.tasks[i].ID != requestedID {
			continue
		}
		task := &b.tasks[i]
		previousStatus := task.Status
		derivedIssueTaskIDs := make([]string, 0, 2)
		handoff, err := b.validateTaskMutationContractLocked(task, action, strings.TrimSpace(input.CreatedBy), input.Details)
		if err != nil {
			return teamTask{}, err
		}
		if _, ok := taskTransitionRuleForAction(action); !ok {
			return teamTask{}, fmt.Errorf("unknown action")
		}
		if action == "block" {
			if err := rejectFalseLocalWorktreeBlock(task, input.Details); err != nil {
				return teamTask{}, err
			}
		}
		transition, err := resolveTaskTransition(task, action, handoff)
		if err != nil {
			return teamTask{}, err
		}
		switch action {
		case "claim", "assign", "reassign":
			task.Owner = strings.TrimSpace(input.Owner)
		case "release":
			task.Owner = ""
		}
		if transition.ClearOwner {
			task.Owner = ""
		}
		if transition.ClearReconciliation {
			clearTaskReconciliationLocked(task)
		}
		if transition.ClearSchedule {
			task.FollowUpAt = ""
			task.ReminderAt = ""
			task.RecheckAt = ""
		}
		task.Status = transition.Status
		task.ReviewState = transition.ReviewState
		task.Blocked = transition.Blocked
		if strings.TrimSpace(input.Details) != "" {
			task.Details = strings.TrimSpace(input.Details)
		}
		if strings.TrimSpace(input.TaskType) != "" {
			task.TaskType = strings.TrimSpace(input.TaskType)
		}
		if strings.TrimSpace(input.ExecutionMode) != "" {
			task.ExecutionMode = strings.TrimSpace(input.ExecutionMode)
		}
		if handoff != nil && taskActionResubmitsForReview(action) && taskHasBlockingReviewFindings(task) {
			resolveOpenTaskReviewFindingsLocked(task, strings.TrimSpace(input.CreatedBy), now)
		}
		if action == "approve" && !taskHasBlockingReviewFindings(task) && (handoff == nil || len(handoff.ReviewFindings) == 0) {
			resolveOpenTaskReviewFindingsLocked(task, strings.TrimSpace(input.CreatedBy), now)
		}
		if handoff != nil {
			b.acceptTaskHandoffLocked(task, strings.TrimSpace(input.CreatedBy), handoff, now)
		}
		if strings.TrimSpace(input.WorkspacePath) != "" {
			task.WorkspacePath = strings.TrimSpace(input.WorkspacePath)
		}
		if len(input.DependsOn) > 0 {
			task.DependsOn = append([]string(nil), input.DependsOn...)
		}
		if action == "block" && handoff != nil {
			requestIDs, err := b.createTaskBlockerRequestsLocked(task, strings.TrimSpace(input.CreatedBy), handoff, now)
			if err != nil {
				return teamTask{}, err
			}
			task.BlockerRequestIDs = append([]string(nil), requestIDs...)
		}
		if handoff != nil {
			derivedIssueTaskIDs, err = b.deriveMarkedDemandFollowUpsLocked(task, strings.TrimSpace(input.CreatedBy), handoff, now)
			if err != nil {
				return teamTask{}, err
			}
		}
		b.ensureTaskOwnerChannelMembershipLocked(task.Channel, task.Owner)
		b.queueTaskBehindActiveOwnerLaneLocked(task)
		task.UpdatedAt = now
		if task.Status == "done" {
			b.unblockDependentsLocked(task.ID)
		}
		b.scheduleTaskLifecycleLocked(task)
		if err := b.syncTaskWorktreeLocked(task); err != nil {
			return teamTask{}, err
		}
		issueQueued, prQueued := b.maybeQueueTaskGitHubPublicationLocked(task, previousStatus, action, handoff)
		b.appendActionLocked("task_updated", "office", task.Channel, strings.TrimSpace(input.CreatedBy), truncateSummary(task.Title+" ["+task.Status+"]", 140), task.ID)
		if err := b.saveLocked(); err != nil {
			return teamTask{}, err
		}
		if issueQueued {
			b.publishTaskIssueSoon(task.ID)
		}
		for _, derivedTaskID := range derivedIssueTaskIDs {
			b.publishTaskIssueSoon(derivedTaskID)
		}
		if prQueued {
			b.publishTaskPRSoon(task.ID)
		}
		return *task, nil
	}

	return teamTask{}, fmt.Errorf("task not found")
}
