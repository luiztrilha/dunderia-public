package team

import (
	"fmt"
	"strings"
)

type taskDemandSeed struct {
	Channel           string
	ExecutionKey      string
	Title             string
	Details           string
	Owner             string
	CreatedBy         string
	TaskType          string
	ExecutionMode     string
	WorkspacePath     string
	SourceSignalID    string
	SourceDecisionID  string
	PublicationPolicy *taskGitHubPublicationPolicy
	DerivedFrom       *taskDerivedDemandRef
}

func (b *Broker) deriveMarkedDemandFollowUpsLocked(sourceTask *teamTask, actor string, handoff *taskHandoffRecord, now string) ([]string, error) {
	if b == nil || sourceTask == nil || handoff == nil {
		return nil, nil
	}
	seeds := collectTaskDemandSeeds(sourceTask, actor, handoff)
	if len(seeds) == 0 {
		return nil, nil
	}
	issueTaskIDs := make([]string, 0, len(seeds))
	for _, seed := range seeds {
		created, issueQueued, err := b.ensureDerivedDemandTaskLocked(seed, now)
		if err != nil {
			return issueTaskIDs, err
		}
		if issueQueued {
			issueTaskIDs = append(issueTaskIDs, created.ID)
		}
	}
	return compactStringList(issueTaskIDs), nil
}

func collectTaskDemandSeeds(sourceTask *teamTask, actor string, handoff *taskHandoffRecord) []taskDemandSeed {
	if sourceTask == nil || handoff == nil {
		return nil
	}
	owner := firstNonEmpty(strings.TrimSpace(sourceTask.Owner), strings.TrimSpace(actor), strings.TrimSpace(sourceTask.CreatedBy))
	executionMode := strings.TrimSpace(sourceTask.ExecutionMode)
	workspacePath := ""
	if strings.EqualFold(executionMode, "external_workspace") {
		workspacePath = strings.TrimSpace(sourceTask.WorkspacePath)
	}
	seeds := make([]taskDemandSeed, 0, len(handoff.Blockers)+len(handoff.ReviewFindings))
	for _, blocker := range handoff.Blockers {
		if !blocker.NewDemand {
			continue
		}
		title := truncateSummary(firstNonEmpty(strings.TrimSpace(blocker.Need), strings.TrimSpace(blocker.Question), "Follow-up demand"), 120)
		seeds = append(seeds, taskDemandSeed{
			Channel:      normalizeChannelSlug(sourceTask.Channel),
			ExecutionKey: fmt.Sprintf("%s|demand|%s|blocker|%s", normalizeChannelSlug(sourceTask.Channel), strings.TrimSpace(sourceTask.ID), strings.TrimSpace(blocker.ID)),
			Title:        "Follow-up: " + title,
			Details: strings.TrimSpace(strings.Join([]string{
				fmt.Sprintf("Nova demanda derivada da task `%s`.", strings.TrimSpace(sourceTask.ID)),
				fmt.Sprintf("Origem: blocker `%s`.", strings.TrimSpace(blocker.ID)),
				"Pergunta: " + strings.TrimSpace(blocker.Question),
				"Necessidade: " + strings.TrimSpace(blocker.Need),
				"Contexto: " + strings.TrimSpace(blocker.Context),
			}, "\n\n")),
			Owner:            owner,
			CreatedBy:        firstNonEmpty(strings.TrimSpace(actor), strings.TrimSpace(sourceTask.CreatedBy), "system"),
			TaskType:         "follow_up",
			ExecutionMode:    executionMode,
			WorkspacePath:    workspacePath,
			SourceSignalID:   strings.TrimSpace(sourceTask.SourceSignalID),
			SourceDecisionID: strings.TrimSpace(sourceTask.SourceDecisionID),
			PublicationPolicy: &taskGitHubPublicationPolicy{
				Enabled:   "auto",
				IssueMode: "immediate",
				PRMode:    "auto",
				SyncMode:  "auto",
				Labels:    []string{"new-demand", "blocker"},
			},
			DerivedFrom: &taskDerivedDemandRef{
				SourceTaskID: strings.TrimSpace(sourceTask.ID),
				SourceKind:   "blocker",
				SourceItemID: strings.TrimSpace(blocker.ID),
			},
		})
	}
	for _, finding := range handoff.ReviewFindings {
		if !finding.NewDemand {
			continue
		}
		title := truncateSummary(firstNonEmpty(strings.TrimSpace(finding.Description), strings.TrimSpace(finding.Location), "Follow-up demand"), 120)
		seeds = append(seeds, taskDemandSeed{
			Channel:      normalizeChannelSlug(sourceTask.Channel),
			ExecutionKey: fmt.Sprintf("%s|demand|%s|finding|%s", normalizeChannelSlug(sourceTask.Channel), strings.TrimSpace(sourceTask.ID), strings.TrimSpace(finding.ID)),
			Title:        "Follow-up: " + title,
			Details: strings.TrimSpace(strings.Join([]string{
				fmt.Sprintf("Nova demanda derivada da task `%s`.", strings.TrimSpace(sourceTask.ID)),
				fmt.Sprintf("Origem: review finding `%s`.", strings.TrimSpace(finding.ID)),
				"Severidade: " + strings.TrimSpace(finding.Severity),
				"Local: " + strings.TrimSpace(finding.Location),
				"Descricao: " + strings.TrimSpace(finding.Description),
				"Guidance: " + strings.TrimSpace(finding.Guidance),
			}, "\n\n")),
			Owner:            owner,
			CreatedBy:        firstNonEmpty(strings.TrimSpace(actor), strings.TrimSpace(sourceTask.CreatedBy), "system"),
			TaskType:         "follow_up",
			ExecutionMode:    executionMode,
			WorkspacePath:    workspacePath,
			SourceSignalID:   strings.TrimSpace(sourceTask.SourceSignalID),
			SourceDecisionID: strings.TrimSpace(sourceTask.SourceDecisionID),
			PublicationPolicy: &taskGitHubPublicationPolicy{
				Enabled:   "auto",
				IssueMode: "immediate",
				PRMode:    "auto",
				SyncMode:  "auto",
				Labels:    []string{"new-demand", "review-finding"},
			},
			DerivedFrom: &taskDerivedDemandRef{
				SourceTaskID: strings.TrimSpace(sourceTask.ID),
				SourceKind:   "review_finding",
				SourceItemID: strings.TrimSpace(finding.ID),
			},
		})
	}
	return compactTaskDemandSeeds(seeds)
}

func compactTaskDemandSeeds(seeds []taskDemandSeed) []taskDemandSeed {
	if len(seeds) == 0 {
		return nil
	}
	out := make([]taskDemandSeed, 0, len(seeds))
	indexByKey := make(map[string]int, len(seeds))
	for _, seed := range seeds {
		key := canonicalTaskDemandSeedKey(seed)
		seed.ExecutionKey = key
		if existingIdx, ok := indexByKey[key]; ok {
			out[existingIdx].PublicationPolicy = mergeTaskGitHubPublicationPolicy(out[existingIdx].PublicationPolicy, seed.PublicationPolicy)
			continue
		}
		indexByKey[key] = len(out)
		out = append(out, seed)
	}
	return out
}

func canonicalTaskDemandSeedKey(seed taskDemandSeed) string {
	channel := normalizeChannelSlug(firstNonEmpty(seed.Channel, "general"))
	sourceTaskID := ""
	sourceKind := ""
	if seed.DerivedFrom != nil {
		sourceTaskID = strings.TrimSpace(seed.DerivedFrom.SourceTaskID)
		sourceKind = sanitizeTaskGitHubLabel(seed.DerivedFrom.SourceKind)
	}
	titleKey := sanitizeTaskGitHubLabel(strings.TrimSpace(strings.TrimPrefix(seed.Title, "Follow-up:")))
	if titleKey == "" {
		titleKey = sanitizeTaskGitHubLabel(seed.ExecutionKey)
	}
	return fmt.Sprintf("%s|demand|%s|%s|%s", channel, sourceTaskID, sourceKind, titleKey)
}

func (b *Broker) ensureDerivedDemandTaskLocked(seed taskDemandSeed, now string) (teamTask, bool, error) {
	validated, err := b.validateStrictTaskPlanLocked(normalizeChannelSlug(firstNonEmpty(seed.Channel, "general")), strings.TrimSpace(seed.CreatedBy), []plannedTaskSpec{{
		ExecutionKey:  seed.ExecutionKey,
		Title:         seed.Title,
		Assignee:      seed.Owner,
		Details:       seed.Details,
		TaskType:      seed.TaskType,
		ExecutionMode: seed.ExecutionMode,
		WorkspacePath: seed.WorkspacePath,
	}})
	if err != nil {
		return teamTask{}, false, err
	}
	item := validated[0]
	channel := item.Channel
	match := taskReuseMatch{
		Channel:          channel,
		ExecutionKey:     item.ExecutionKey,
		Title:            item.Title,
		Details:          item.Details,
		Owner:            item.Owner,
		TaskType:         item.TaskType,
		ExecutionMode:    item.ExecutionMode,
		WorkspacePath:    item.WorkspacePath,
		SourceSignalID:   seed.SourceSignalID,
		SourceDecisionID: seed.SourceDecisionID,
	}
	if existing := b.findReusableTaskLocked(match); existing != nil {
		previousStatus := existing.Status
		if existing.Details == "" && item.Details != "" {
			existing.Details = item.Details
		}
		if existing.Owner == "" && item.Owner != "" {
			existing.Owner = item.Owner
			if !taskIsTerminal(existing) {
				existing.Status = "in_progress"
			}
		}
		if existing.ExecutionKey == "" {
			existing.ExecutionKey = item.ExecutionKey
		}
		if existing.TaskType == "" && item.TaskType != "" {
			existing.TaskType = item.TaskType
		}
		if existing.ExecutionMode == "" && item.ExecutionMode != "" {
			existing.ExecutionMode = item.ExecutionMode
		}
		if existing.SourceSignalID == "" && seed.SourceSignalID != "" {
			existing.SourceSignalID = seed.SourceSignalID
		}
		if existing.SourceDecisionID == "" && seed.SourceDecisionID != "" {
			existing.SourceDecisionID = seed.SourceDecisionID
		}
		if existing.WorkspacePath == "" && item.WorkspacePath != "" {
			existing.WorkspacePath = item.WorkspacePath
		}
		existing.PublicationPolicy = mergeTaskGitHubPublicationPolicy(existing.PublicationPolicy, seed.PublicationPolicy)
		if existing.DerivedFrom == nil && seed.DerivedFrom != nil {
			copyRef := *seed.DerivedFrom
			existing.DerivedFrom = &copyRef
		}
		b.ensureTaskOwnerChannelMembershipLocked(channel, existing.Owner)
		existing.UpdatedAt = strings.TrimSpace(now)
		b.queueTaskBehindActiveOwnerLaneLocked(existing)
		if err := rejectTheaterTaskForLiveBusiness(existing); err != nil {
			return teamTask{}, false, err
		}
		b.scheduleTaskLifecycleLocked(existing)
		if err := b.syncTaskWorktreeLocked(existing); err != nil {
			return teamTask{}, false, err
		}
		b.linkTaskWorkspaceToChannelLocked(channel, existing)
		issueQueued, _ := b.maybeQueueTaskGitHubPublicationLocked(existing, previousStatus, "create", nil)
		b.appendActionLocked("task_updated", "office", channel, strings.TrimSpace(seed.CreatedBy), truncateSummary(existing.Title+" [derived-demand]", 140), existing.ID)
		return *existing, issueQueued, nil
	}
	if recent := b.findRecentTerminalTaskLocked(match); recent != nil {
		return teamTask{}, false, recentTaskConflict(recent)
	}

	b.counter++
	task := teamTask{
		ID:                fmt.Sprintf("task-%d", b.counter),
		Channel:           channel,
		ExecutionKey:      item.ExecutionKey,
		Title:             item.Title,
		Details:           item.Details,
		Owner:             item.Owner,
		Status:            "open",
		CreatedBy:         strings.TrimSpace(seed.CreatedBy),
		TaskType:          item.TaskType,
		PipelineID:        item.PipelineID,
		ExecutionMode:     item.ExecutionMode,
		ReviewState:       item.ReviewState,
		SourceSignalID:    strings.TrimSpace(seed.SourceSignalID),
		SourceDecisionID:  strings.TrimSpace(seed.SourceDecisionID),
		WorkspacePath:     item.WorkspacePath,
		PublicationPolicy: mergeTaskGitHubPublicationPolicy(nil, seed.PublicationPolicy),
		CreatedAt:         strings.TrimSpace(now),
		UpdatedAt:         strings.TrimSpace(now),
	}
	if seed.DerivedFrom != nil {
		copyRef := *seed.DerivedFrom
		task.DerivedFrom = &copyRef
	}
	if task.Owner != "" {
		task.Status = "in_progress"
	}
	b.ensureTaskOwnerChannelMembershipLocked(channel, task.Owner)
	b.queueTaskBehindActiveOwnerLaneLocked(&task)
	if err := rejectTheaterTaskForLiveBusiness(&task); err != nil {
		return teamTask{}, false, err
	}
	b.scheduleTaskLifecycleLocked(&task)
	if err := b.syncTaskWorktreeLocked(&task); err != nil {
		return teamTask{}, false, err
	}
	b.linkTaskWorkspaceToChannelLocked(channel, &task)
	issueQueued, _ := b.maybeQueueTaskGitHubPublicationLocked(&task, "", "create", nil)
	b.tasks = append(b.tasks, task)
	b.appendActionLocked("task_created", "office", channel, strings.TrimSpace(seed.CreatedBy), truncateSummary(task.Title+" [derived-demand]", 140), task.ID)
	return task, issueQueued, nil
}
