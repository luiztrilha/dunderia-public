package team

import (
	"fmt"
	"strings"
	"time"
)

func (b *Broker) validateTaskMutationContractLocked(task *teamTask, action, actor, details string) (*taskHandoffRecord, error) {
	if task == nil {
		return nil, fmt.Errorf("task required")
	}
	action = strings.ToLower(strings.TrimSpace(action))
	if taskHasPendingReconciliation(task) && (action == "complete" || action == "review" || action == "approve") {
		return nil, fmt.Errorf("task %s has reconciliation pending and cannot advance until it is reconciled", task.ID)
	}
	trimmed := strings.TrimSpace(details)
	if taskActionNeedsStrictHandoff(action) && agentRequiresStrictTaskHandoff(actor) {
		if trimmed == "" {
			return nil, fmt.Errorf("task mutation requires a structured handoff in details")
		}
		handoff, err := parseStructuredTaskHandoff(trimmed)
		if err != nil {
			return nil, fmt.Errorf("task mutation requires a valid structured handoff: %w", err)
		}
		if action == "block" && len(handoff.Blockers) == 0 {
			return nil, fmt.Errorf("block action requires at least one structured blocker")
		}
		if action != "block" && len(handoff.Blockers) > 0 {
			return nil, fmt.Errorf("structured blockers are only allowed on block actions")
		}
		if _, ok := taskTransitionRuleForAction(action); ok {
			if _, err := resolveTaskTransition(task, action, handoff); err != nil {
				return nil, err
			}
		}
		return handoff, nil
	}
	if trimmed == "" || !taskActionAcceptsReviewFindings(action) {
		if _, ok := taskTransitionRuleForAction(action); ok {
			if _, err := resolveTaskTransition(task, action, nil); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	handoff, err := parseStructuredTaskHandoff(trimmed)
	if err != nil {
		return nil, fmt.Errorf("malformed structured handoff: %w", err)
	}
	if _, ok := taskTransitionRuleForAction(action); ok {
		if _, err := resolveTaskTransition(task, action, handoff); err != nil {
			return nil, err
		}
	}
	return handoff, nil
}

func (b *Broker) acceptTaskHandoffLocked(task *teamTask, actor string, handoff *taskHandoffRecord, now string) {
	if task == nil || handoff == nil {
		return
	}
	copyRecord := *handoff
	copyRecord.AcceptedAt = strings.TrimSpace(now)
	copyRecord.AcceptedBy = strings.TrimSpace(actor)
	task.LastHandoff = &copyRecord
	task.HandoffStatus = "accepted"
	task.HandoffAcceptedAt = strings.TrimSpace(now)
	if len(copyRecord.ReviewFindings) > 0 {
		normalized := normalizeTaskReviewFindings(copyRecord.ReviewFindings)
		task.ReviewFindings = mergeTaskReviewFindings(task.ReviewFindings, normalized)
		task.ReviewFindingHistory = append(task.ReviewFindingHistory, taskReviewFindingBatch{
			AcceptedAt: strings.TrimSpace(now),
			AcceptedBy: strings.TrimSpace(actor),
			Findings:   append([]taskReviewFinding(nil), normalized...),
		})
	}
}

func (b *Broker) createTaskBlockerRequestsLocked(task *teamTask, actor string, handoff *taskHandoffRecord, now string) ([]string, error) {
	if b == nil || task == nil || handoff == nil || len(handoff.Blockers) == 0 {
		return nil, nil
	}
	requestIDs := make([]string, 0, len(handoff.Blockers))
	for _, blocker := range handoff.Blockers {
		req := b.createRequestLocked(humanInterview{
			Kind:          requestKindForTaskBlocker(blocker.Kind),
			Status:        "pending",
			From:          strings.TrimSpace(actor),
			Channel:       normalizeChannelSlug(task.Channel),
			Title:         "Task blocker",
			Question:      blocker.Question,
			Context:       blocker.Context,
			Blocking:      true,
			Required:      true,
			SourceTaskID:  strings.TrimSpace(task.ID),
			SourceBlocker: blocker.ID,
		}, now)
		requestIDs = append(requestIDs, req.ID)
	}
	return requestIDs, nil
}

func (b *Broker) createRequestLocked(req humanInterview, now string) humanInterview {
	channel := normalizeChannelSlug(req.Channel)
	if channel == "" {
		channel = "general"
	}
	if strings.TrimSpace(now) == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	b.counter++
	req.ID = fmt.Sprintf("request-%d", b.counter)
	req.Channel = channel
	req.CreatedAt = now
	req.UpdatedAt = now
	req.Kind = normalizeRequestKind(req.Kind)
	req.Options, req.RecommendedID = normalizeRequestOptions(req.Kind, req.RecommendedID, req.Options)
	if requestNeedsHumanDecision(req) {
		req.Blocking = true
		req.Required = true
	}
	if strings.TrimSpace(req.Status) == "" {
		req.Status = "pending"
	}
	if strings.TrimSpace(req.Title) == "" {
		req.Title = "Request"
	}
	b.scheduleRequestLifecycleLocked(&req)
	b.requests = append(b.requests, req)
	b.pendingInterview = firstBlockingRequest(b.requests)
	b.appendActionLocked("request_created", "office", channel, req.From, truncateSummary(req.Title+" "+req.Question, 140), req.ID)
	return req
}

func (b *Broker) MarkTaskReconciliationPending(taskID, workspacePath, observedDelta, reason string) (teamTask, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return teamTask{}, false, fmt.Errorf("task id required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for i := range b.tasks {
		task := &b.tasks[i]
		if task.ID != taskID {
			continue
		}
		task.Reconciliation = &taskReconciliationState{
			Status:         "pending",
			Reason:         strings.TrimSpace(reason),
			WorkspacePath:  strings.TrimSpace(workspacePath),
			ObservedDelta:  strings.TrimSpace(observedDelta),
			ChangedPaths:   nil,
			UntrackedPaths: nil,
			DetectedAt:     now,
			Blocking:       true,
		}
		task.Reconciliation.ChangedPaths, task.Reconciliation.UntrackedPaths = summarizeObservedWorkspaceDelta(task.Reconciliation.ObservedDelta)
		task.HandoffStatus = "repair_required"
		task.UpdatedAt = now
		if err := b.saveLocked(); err != nil {
			return teamTask{}, false, err
		}
		return *task, true, nil
	}
	return teamTask{}, false, fmt.Errorf("task not found")
}

func clearTaskReconciliationLocked(task *teamTask) {
	if task == nil {
		return
	}
	task.Reconciliation = nil
}

func normalizeTaskReviewFindings(findings []taskReviewFinding) []taskReviewFinding {
	if len(findings) == 0 {
		return nil
	}
	out := make([]taskReviewFinding, 0, len(findings))
	for _, finding := range findings {
		copyFinding := finding
		if strings.TrimSpace(copyFinding.Status) == "" {
			copyFinding.Status = "open"
		}
		if copyFinding.Status == "open" {
			copyFinding.ResolvedAt = ""
			copyFinding.ResolvedBy = ""
		}
		out = append(out, copyFinding)
	}
	return out
}

func mergeTaskReviewFindings(existing, incoming []taskReviewFinding) []taskReviewFinding {
	if len(incoming) == 0 {
		return append([]taskReviewFinding(nil), existing...)
	}
	out := append([]taskReviewFinding(nil), existing...)
	index := make(map[string]int, len(out))
	for i, finding := range out {
		index[taskReviewFindingKey(finding)] = i
	}
	for _, finding := range incoming {
		key := taskReviewFindingKey(finding)
		if idx, ok := index[key]; ok {
			out[idx].Severity = finding.Severity
			out[idx].Location = finding.Location
			out[idx].Description = finding.Description
			out[idx].Guidance = finding.Guidance
			out[idx].NewDemand = finding.NewDemand
			out[idx].Status = "open"
			out[idx].ResolvedAt = ""
			out[idx].ResolvedBy = ""
			continue
		}
		out = append(out, finding)
		index[key] = len(out) - 1
	}
	return out
}

func taskReviewFindingKey(finding taskReviewFinding) string {
	return strings.ToLower(strings.TrimSpace(strings.Join([]string{
		finding.Severity,
		finding.Location,
		finding.Description,
	}, "|")))
}

func resolveOpenTaskReviewFindingsLocked(task *teamTask, actor, now string) {
	if task == nil {
		return
	}
	for i := range task.ReviewFindings {
		if strings.TrimSpace(task.ReviewFindings[i].Status) == "resolved" {
			continue
		}
		task.ReviewFindings[i].Status = "resolved"
		task.ReviewFindings[i].ResolvedAt = strings.TrimSpace(now)
		task.ReviewFindings[i].ResolvedBy = strings.TrimSpace(actor)
	}
}

func summarizeObservedWorkspaceDelta(observedDelta string) ([]string, []string) {
	if strings.TrimSpace(observedDelta) == "" {
		return nil, nil
	}
	entries := strings.Split(observedDelta, "\x00")
	changed := make([]string, 0, len(entries))
	untracked := make([]string, 0, len(entries))
	for _, entry := range entries {
		raw := entry
		if strings.TrimSpace(raw) == "" || len(raw) < 3 {
			continue
		}
		status := strings.TrimSpace(raw[:2])
		path := strings.TrimSpace(raw[2:])
		if path == "" {
			continue
		}
		if status == "??" {
			untracked = append(untracked, path)
			continue
		}
		changed = append(changed, path)
	}
	return compactStringList(changed), compactStringList(untracked)
}
