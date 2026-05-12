package team

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/nex-crm/wuphf/internal/config"
)

type taskPipelineTemplate struct {
	ID             string
	OpenStage      string
	ActiveStage    string
	ReviewStage    string
	DoneStage      string
	ReviewRequired bool
}

var taskPipelineTemplates = map[string]taskPipelineTemplate{
	"feature":   {ID: "feature", OpenStage: "triage", ActiveStage: "implement", ReviewStage: "review", DoneStage: "ship", ReviewRequired: true},
	"bugfix":    {ID: "bugfix", OpenStage: "triage", ActiveStage: "fix", ReviewStage: "review", DoneStage: "verify", ReviewRequired: true},
	"research":  {ID: "research", OpenStage: "question", ActiveStage: "investigate", ReviewStage: "synthesize", DoneStage: "recommend"},
	"launch":    {ID: "launch", OpenStage: "brief", ActiveStage: "execute", ReviewStage: "review", DoneStage: "ship"},
	"incident":  {ID: "incident", OpenStage: "assess", ActiveStage: "mitigate", ReviewStage: "verify", DoneStage: "postmortem"},
	"follow_up": {ID: "follow_up", OpenStage: "triage", ActiveStage: "act", ReviewStage: "verify", DoneStage: "done"},
}

func inferTaskType(owner, title, details string) string {
	text := strings.ToLower(strings.TrimSpace(owner + " " + title + " " + details))
	switch {
	case containsAnyTaskFragment(text, "bug", "fix", "regression", "broken", "error", "panic", "crash"):
		return "bugfix"
	case containsAnyTaskFragment(text, "incident", "outage", "sev", "mitigate", "hotfix"):
		return "incident"
	case containsAnyTaskFragment(text, "launch", "campaign", "announce", "rollout", "go to market"):
		return "launch"
	case containsAnyTaskFragment(text, "research", "investigate", "evaluate", "compare", "analyze", "audit", "thesis", "framework", "recommend"):
		return "research"
	case containsAnyTaskFragment(text, "feature", "build", "implement", "ship", "signup", "flow"):
		return "feature"
	default:
		return "follow_up"
	}
}

func pipelineTemplate(taskType string) taskPipelineTemplate {
	if template, ok := taskPipelineTemplates[strings.TrimSpace(taskType)]; ok {
		return template
	}
	return taskPipelineTemplates["follow_up"]
}

func taskNeedsStructuredReview(task *teamTask) bool {
	if task == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(task.ExecutionMode), "local_worktree") ||
		strings.EqualFold(strings.TrimSpace(task.ExecutionMode), "live_external") {
		return true
	}
	template := pipelineTemplate(task.TaskType)
	if !template.ReviewRequired {
		return false
	}
	return taskWorkRequiresLocalExecution(task.Owner, task.Title, task.Details)
}

func taskDefaultExecutionMode(owner, taskType, title, details string) string {
	task := &teamTask{Owner: owner, TaskType: taskType, Title: title, Details: details}
	if taskRequiresRealExternalExecution(task) {
		return "live_external"
	}
	switch strings.TrimSpace(strings.ToLower(taskType)) {
	case "feature", "bugfix", "incident":
		if taskWorkRequiresLocalExecution(owner, title, details) {
			return "local_worktree"
		}
	}
	return "office"
}

func taskStageForStatus(task *teamTask) string {
	template := pipelineTemplate(task.TaskType)
	switch strings.TrimSpace(task.Status) {
	case "in_progress":
		return template.ActiveStage
	case "review":
		return template.ReviewStage
	case "done":
		return template.DoneStage
	default:
		return template.OpenStage
	}
}

func normalizeTaskPlan(task *teamTask) {
	if task == nil {
		return
	}
	if strings.TrimSpace(task.TaskType) == "" {
		task.TaskType = inferTaskType(task.Owner, task.Title, task.Details)
	}
	if strings.TrimSpace(task.PipelineID) == "" {
		task.PipelineID = task.TaskType
	}
	if strings.TrimSpace(task.ExecutionMode) == "" {
		task.ExecutionMode = taskDefaultExecutionMode(task.Owner, task.TaskType, task.Title, task.Details)
	}
	if strings.TrimSpace(task.ReviewState) == "" {
		if taskNeedsStructuredReview(task) {
			task.ReviewState = "pending_review"
		} else {
			task.ReviewState = "not_required"
		}
	}
	if strings.TrimSpace(task.Status) == "review" &&
		!strings.EqualFold(strings.TrimSpace(task.ReviewState), "changes_requested") {
		task.ReviewState = "ready_for_review"
	}
	if strings.TrimSpace(task.Status) == "done" &&
		(task.ReviewState == "pending_review" || task.ReviewState == "ready_for_review") {
		task.ReviewState = "approved"
	}
	task.PipelineStage = taskStageForStatus(task)
	normalizeTaskOutcomeAndQueue(task)
	normalizeTaskLimits(task)
	evaluateTaskSignals(task, firstNonEmpty(task.UpdatedAt, task.CreatedAt))
}

func normalizeTaskOutcomeStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pending", "needs_evidence", "verified", "waived":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizeTaskQueueKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "-", "_")
	return value
}

func applyTaskOutcomeInput(task *teamTask, outcome, outcomeStatus, evidence, queueKey, now string) {
	if task == nil {
		return
	}
	if next := strings.TrimSpace(outcome); next != "" {
		task.Outcome = next
	}
	if next := strings.TrimSpace(evidence); next != "" {
		task.OutcomeEvidence = next
		task.OutcomeVerifiedAt = strings.TrimSpace(now)
		if normalizeTaskOutcomeStatus(outcomeStatus) == "" {
			task.OutcomeStatus = "verified"
		}
	}
	if next := normalizeTaskOutcomeStatus(outcomeStatus); next != "" {
		task.OutcomeStatus = next
		if next == "verified" && strings.TrimSpace(task.OutcomeVerifiedAt) == "" {
			task.OutcomeVerifiedAt = strings.TrimSpace(now)
		}
	}
	if next := normalizeTaskQueueKey(queueKey); next != "" {
		task.QueueKey = next
	}
}

func normalizeTaskOutcomeAndQueue(task *teamTask) {
	if task == nil {
		return
	}
	task.Outcome = strings.TrimSpace(task.Outcome)
	task.OutcomeEvidence = strings.TrimSpace(task.OutcomeEvidence)
	task.OutcomeStatus = normalizeTaskOutcomeStatus(task.OutcomeStatus)
	if task.Outcome != "" {
		switch {
		case task.OutcomeStatus == "":
			if strings.TrimSpace(task.OutcomeEvidence) != "" {
				task.OutcomeStatus = "verified"
			} else if normalizeTaskStatus(task.Status) == "done" {
				task.OutcomeStatus = "needs_evidence"
			} else {
				task.OutcomeStatus = "pending"
			}
		case task.OutcomeStatus == "verified" && strings.TrimSpace(task.OutcomeVerifiedAt) == "" && strings.TrimSpace(task.OutcomeEvidence) != "":
			task.OutcomeVerifiedAt = strings.TrimSpace(task.UpdatedAt)
		}
	}
	task.QueueKey = deriveTaskQueueKey(task)
}

type taskGoalContext struct {
	Company  string
	Goals    string
	Priority string
}

func currentTaskGoalContext() taskGoalContext {
	data, err := os.ReadFile(config.ConfigPath())
	if err != nil {
		return taskGoalContext{}
	}
	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return taskGoalContext{}
	}
	return taskGoalContext{
		Company:  strings.TrimSpace(cfg.CompanyName),
		Goals:    strings.TrimSpace(cfg.CompanyGoals),
		Priority: strings.TrimSpace(cfg.CompanyPriority),
	}
}

func applyTaskOperatorContract(task *teamTask, goalCtx taskGoalContext) {
	if task == nil {
		return
	}
	applyTaskCompletionContract(task)
	applyTaskGoalPath(task, goalCtx)
	applyTaskQueueContract(task)
	applyTaskPlanningContract(task)
	applyTaskLearningCandidate(task)
}

func applyTaskCompletionContract(task *teamTask) {
	if task == nil {
		return
	}
	required := taskCompletionEvidenceRequired(task)
	satisfied := taskCompletionEvidenceSatisfied(task)
	task.CompletionEvidenceRequired = required
	task.CompletionEvidenceSatisfied = satisfied
	task.CompletionBlocker = ""
	if required && !satisfied {
		task.CompletionBlocker = "Completion requires outcome evidence or a durable artifact before this task can be marked done."
	}
}

func taskCompletionEvidenceRequired(task *teamTask) bool {
	if task == nil {
		return false
	}
	if task.AwaitingHuman || strings.EqualFold(strings.TrimSpace(task.TaskType), "human_action") {
		return false
	}
	if strings.TrimSpace(task.Outcome) != "" {
		return true
	}
	if taskNeedsStructuredReview(task) {
		return true
	}
	switch strings.TrimSpace(task.ExecutionMode) {
	case "local_worktree", "external_workspace", "live_external":
		return true
	}
	return false
}

func taskCompletionEvidenceSatisfied(task *teamTask) bool {
	if task == nil {
		return false
	}
	if strings.TrimSpace(task.OutcomeEvidence) != "" {
		return true
	}
	if task.LastHandoff != nil && strings.TrimSpace(task.LastHandoff.Summary) != "" && strings.EqualFold(strings.TrimSpace(task.HandoffStatus), "accepted") {
		return true
	}
	return taskHasExternalPublication(task)
}

func applyTaskGoalPath(task *teamTask, goalCtx taskGoalContext) {
	if task == nil {
		return
	}
	var path []string
	add := func(label, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		path = append(path, label+": "+truncateSummary(value, 140))
	}
	add("Empresa", goalCtx.Company)
	add("Objetivo", goalCtx.Goals)
	add("Prioridade", goalCtx.Priority)
	add("Entrega", task.DeliveryID)
	add("Canal", normalizeChannelSlug(task.Channel))
	add("Fluxo", task.PipelineID)
	add("Etapa", task.PipelineStage)
	if len(task.DependsOn) > 0 {
		add("Depende de", strings.Join(task.DependsOn, ", "))
	}
	add("Resultado esperado", task.Outcome)
	task.GoalPath = uniqueStrings(path)
	task.GoalSummary = strings.Join(task.GoalPath, " -> ")
}

func deriveTaskQueueKey(task *teamTask) string {
	if task == nil {
		return "active"
	}
	status := normalizeTaskStatus(task.Status)
	switch {
	case task.AwaitingHuman || strings.EqualFold(strings.TrimSpace(task.TaskType), "human_action"):
		return "human"
	case task.Blocked || status == "blocked":
		return "blocked"
	case status == "review" || strings.EqualFold(strings.TrimSpace(task.ReviewState), "ready_for_review"):
		return "review"
	case status == "done" || status == "canceled" || status == "failed":
		return "history"
	case status == "open" && strings.TrimSpace(task.Owner) == "":
		return "intake"
	}
	if explicit := normalizeTaskQueueKey(task.QueueKey); explicit != "" {
		return explicit
	}
	switch strings.TrimSpace(task.ExecutionMode) {
	case "local_worktree", "external_workspace":
		return "workspace"
	}
	if taskType := normalizeTaskQueueKey(task.TaskType); taskType != "" {
		return taskType
	}
	return "active"
}

func applyTaskQueueContract(task *teamTask) {
	if task == nil {
		return
	}
	task.QueueKey = deriveTaskQueueKey(task)
	task.QueueLabel = taskQueueLabel(task.QueueKey)
	task.QueuePriority = taskQueuePriority(task)
	task.QueueReason = taskQueueReason(task)
	task.QueueSLAAt = firstNonEmpty(task.DueAt, task.RecheckAt, task.ReminderAt, task.FollowUpAt)
}

func taskQueueLabel(queueKey string) string {
	switch normalizeTaskQueueKey(queueKey) {
	case "human":
		return "Human decision"
	case "blocked":
		return "Blocked"
	case "review":
		return "Review"
	case "workspace":
		return "Workspace"
	case "history":
		return "History"
	case "intake":
		return "Intake"
	case "feature":
		return "Feature"
	case "bugfix":
		return "Bugfix"
	case "incident":
		return "Incident"
	default:
		return "Active"
	}
}

func taskQueuePriority(task *teamTask) string {
	if task == nil {
		return ""
	}
	if task.AwaitingHuman || strings.EqualFold(task.QueueKey, "human") || task.Blocked || strings.EqualFold(normalizeTaskStatus(task.Status), "blocked") {
		return "high"
	}
	if taskCompletionEvidenceRequired(task) && !taskCompletionEvidenceSatisfied(task) {
		return "medium"
	}
	if strings.EqualFold(normalizeTaskStatus(task.Status), "review") || strings.EqualFold(task.ReviewState, "ready_for_review") {
		return "medium"
	}
	return "normal"
}

func taskQueueReason(task *teamTask) string {
	if task == nil {
		return ""
	}
	switch normalizeTaskQueueKey(task.QueueKey) {
	case "human":
		return "Waiting for a human decision before agents continue."
	case "blocked":
		return firstNonEmpty(task.AwaitingHumanReason, task.CompletionBlocker, "Blocked until the blocker is resolved.")
	case "review":
		return "Ready for review or approval."
	case "workspace":
		return "Requires a concrete workspace or worktree execution path."
	case "history":
		return "Terminal work retained for audit and learning."
	case "intake":
		return "Needs ownership before execution starts."
	}
	if taskCompletionEvidenceRequired(task) && !taskCompletionEvidenceSatisfied(task) {
		return "Needs outcome evidence or an artifact before closure."
	}
	return "Ready for normal execution."
}

func applyTaskPlanningContract(task *teamTask) {
	if task == nil {
		return
	}
	task.PlanRequired = taskNeedsDeepPlan(task)
	task.PlanStatus = "not_required"
	task.PlanBlocker = ""
	task.LatestPlanSummary = ""
	if latest := latestTaskPlanRevision(task); latest != nil {
		task.PlanStatus = firstNonEmpty(normalizeTaskPlanRevisionStatus(latest.Status), "draft")
		task.LatestPlanSummary = strings.TrimSpace(latest.Summary)
		if task.PlanRequired && task.PlanStatus == "draft" {
			task.PlanBlocker = "Plan exists but is still draft; mark it ready or approve it before high-confidence execution."
		}
		return
	}
	if task.PlanRequired {
		task.PlanStatus = "missing"
		task.PlanBlocker = "Deep planning is recommended before execution because this task has code, workspace, review, or delivery risk."
	}
}

func taskNeedsDeepPlan(task *teamTask) bool {
	if task == nil {
		return false
	}
	if task.AwaitingHuman || strings.EqualFold(strings.TrimSpace(task.TaskType), "human_action") {
		return false
	}
	if taskNeedsStructuredReview(task) {
		return true
	}
	switch strings.TrimSpace(task.ExecutionMode) {
	case "local_worktree", "external_workspace", "live_external":
		return true
	}
	switch strings.TrimSpace(task.TaskType) {
	case "feature", "bugfix", "incident", "launch":
		return true
	}
	return len(task.DependsOn) > 0 || strings.TrimSpace(task.DeliveryID) != ""
}

func applyTaskLearningCandidate(task *teamTask) {
	if task == nil {
		return
	}
	task.LearningCandidate = nil
	if normalizeTaskStatus(task.Status) != "done" {
		return
	}
	evidence := strings.TrimSpace(task.OutcomeEvidence)
	if evidence == "" && len(task.Artifacts) == 0 && latestTaskPlanRevision(task) == nil {
		return
	}
	title := strings.TrimSpace(firstNonEmpty(task.Outcome, task.Title))
	if title == "" {
		return
	}
	task.LearningCandidate = &taskLearningCandidate{
		Recommended: true,
		Kind:        "playbook",
		Title:       truncateSummary(title, 120),
		Summary:     truncateSummary(firstNonEmpty(evidence, task.Details, task.Title), 220),
		SkillName:   "learned-" + strings.TrimSpace(task.ID),
		Reason:      "Completed work has evidence, artifacts, or a plan that can seed future execution.",
	}
}

func requestIsResolvedLocked(requests []humanInterview, requestID string) bool {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return false
	}
	for _, req := range requests {
		if strings.TrimSpace(req.ID) != requestID {
			continue
		}
		if req.Answered != nil {
			return true
		}
		status := strings.ToLower(strings.TrimSpace(req.Status))
		return status == "answered" || status == "canceled" || status == "cancelled"
	}
	return false
}

func containsAnyTaskFragment(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func taskWorkRequiresLocalExecution(owner, title, details string) bool {
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{owner, title, details}, " ")))
	return containsAnyTaskFragment(text,
		"eng", "engineer", "developer",
		"repo", "repository", "worktree", "workspace", "filesystem",
		"code", "coding", "implement", "build", "ship",
		"frontend", "backend", "api", "database", "schema", "migration",
		"bug", "fix", "panic", "crash", "compile", "test",
	)
}

func taskRequiresRealExternalExecution(task *teamTask) bool {
	if task == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(task.ExecutionMode), "local_worktree") {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{task.Owner, task.Title, task.Details}, " ")))
	if text == "" {
		return false
	}
	if containsAnyTaskFragment(text,
		"mock preview", "preview only", "stub only",
		"local-only", "local only", "repo-only", "repo only",
		"no live write", "no external write", "do not post", "don't post",
		"do not create remotely", "don't create remotely",
	) {
		return false
	}
	if !containsAnyTaskFragment(text,
		"slack", "notion", "google drive", "drive", "discord",
		"calendar", "crm", "hubspot", "salesforce", "airtable",
		"linear", "jira", "confluence", "integration", "connected account",
		"external system", "external tool", "external workflow", "one action",
	) {
		return false
	}
	return containsAnyTaskFragment(text,
		"post", "create", "write", "publish", "send", "join", "search",
		"query", "read", "fetch", "sync", "run", "execute", "trigger",
		"handoff", "proof artifact", "page", "doc", "document", "message",
		"database", "workflow", "fan-out", "fanout",
	)
}

func taskHasMockPreviewStubTestingIntent(task *teamTask) bool {
	if task == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{task.Channel, task.Owner, task.Title, task.Details, task.TaskType, task.PipelineID, task.ExecutionMode}, " ")))
	if text == "" {
		return false
	}
	return containsAnyTaskFragment(text,
		"mock", "preview", "stub", "test", "testing",
		"dry run", "dry-run", "sandbox", "simulate", "simulation",
	)
}

func taskLooksLikeInternalTheater(task *teamTask) bool {
	if task == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{task.Channel, task.Owner, task.Title, task.Details, task.TaskType, task.PipelineID, task.ExecutionMode}, " ")))
	if text == "" {
		return false
	}
	return containsAnyTaskFragment(text,
		"proof artifact", "proof packet", "proof page", "review bundle", "review packet",
		"local artifact", "preview packet", "artifact theater", "eval", "evaluation",
		"blueprint-derived scaffolding", "blueprint derived scaffolding",
		"scaffolding", "scaffold", "rubric", "scorecard", "smoke test",
		"handoff packet", "delivery packet",
		"artifact path", "local path", "reviewable artifact", "reviewable bundle",
		"source-of-truth artifact", "source of truth artifact",
		"blueprint.yaml", "updated blueprint", "template review packet",
	)
}

func taskLooksLikeLiveBusinessObjective(task *teamTask) bool {
	if task == nil {
		return false
	}
	if taskRequiresRealExternalExecution(task) {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{task.Channel, task.Owner, task.Title, task.Details, task.TaskType, task.PipelineID, task.ExecutionMode}, " ")))
	if text == "" {
		return false
	}
	return containsAnyTaskFragment(text,
		"launch", "go live", "go-live",
		"end to end", "end-to-end",
		"client", "customer", "customer-facing", "client-facing",
		"revenue", "sales", "deliverable", "publish", "ship", "deploy",
		"production", "live external", "real external", "business objective",
		"client-delivery", "delivery", "fulfillment", "customer-success",
		"marketing", "growth", "publishing", "publish", "content",
		"website", "landing page", "offer", "video", "script",
	)
}

func rejectTheaterTaskForLiveBusiness(task *teamTask) error {
	if task == nil {
		return nil
	}
	if !taskLooksLikeLiveBusinessObjective(task) {
		return nil
	}
	if taskHasMockPreviewStubTestingIntent(task) {
		return nil
	}
	if !taskLooksLikeInternalTheater(task) {
		return nil
	}
	return fmt.Errorf("live business task cannot be framed as proof/test/review-bundle/local-artifact theater; mark it mock/preview/stub/testing if that is intentional")
}
