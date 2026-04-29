package team

import (
	"fmt"
	"strings"
	"time"
)

type runLivenessState string

const (
	runLivenessAdvanced      runLivenessState = "advanced"
	runLivenessCompleted     runLivenessState = "completed"
	runLivenessBlocked       runLivenessState = "blocked"
	runLivenessFailed        runLivenessState = "failed"
	runLivenessEmptyResponse runLivenessState = "empty_response"
	runLivenessPlanOnly      runLivenessState = "plan_only"
	runLivenessNeedsFollowUp runLivenessState = "needs_followup"
)

type runLivenessInput struct {
	RunSucceeded       bool
	Task               *teamTask
	MessageContent     string
	HadSubstantivePost bool
	HadTaskMutation    bool
	HadExternalAttempt bool
	HadExternalSuccess bool
	WorkspaceChanged   bool
	Error              string
}

type runLivenessResult struct {
	State  runLivenessState
	Reason string
}

func classifyRunLiveness(input runLivenessInput) runLivenessResult {
	if !input.RunSucceeded {
		reason := strings.TrimSpace(input.Error)
		if reason == "" {
			reason = "runtime turn failed"
		}
		return runLivenessResult{State: runLivenessFailed, Reason: reason}
	}
	if input.Task != nil && taskHasDurableCompletionState(input.Task) {
		status := strings.ToLower(strings.TrimSpace(input.Task.Status))
		if status == "blocked" {
			return runLivenessResult{State: runLivenessBlocked, Reason: fmt.Sprintf("task %s is blocked", input.Task.ID)}
		}
		return runLivenessResult{State: runLivenessCompleted, Reason: fmt.Sprintf("task %s reached durable state", input.Task.ID)}
	}
	if input.HadExternalSuccess {
		return runLivenessResult{State: runLivenessCompleted, Reason: "external workflow/action execution was recorded"}
	}
	if input.HadExternalAttempt {
		return runLivenessResult{State: runLivenessBlocked, Reason: "external workflow/action attempt was recorded"}
	}
	if input.HadTaskMutation {
		return runLivenessResult{State: runLivenessAdvanced, Reason: "task state changed during turn"}
	}
	if input.WorkspaceChanged {
		return runLivenessResult{State: runLivenessAdvanced, Reason: "workspace changed during turn"}
	}
	if !input.HadSubstantivePost || strings.TrimSpace(input.MessageContent) == "" {
		return runLivenessResult{State: runLivenessEmptyResponse, Reason: "runtime turn completed without substantive office output or evidence"}
	}
	if messageLooksLikeBlockedNoDeltaUpdate(input.MessageContent) || messageLooksLikeAwaitingHumanInput(input.MessageContent) {
		return runLivenessResult{State: runLivenessBlocked, Reason: "agent reported a blocker without durable task mutation"}
	}
	if runLivenessTaskAllowsNarrativeProgress(input.Task) {
		return runLivenessResult{State: runLivenessAdvanced, Reason: "narrative progress is acceptable for this task type"}
	}
	if messageLooksLikePlanOnly(input.MessageContent) {
		return runLivenessResult{State: runLivenessPlanOnly, Reason: "agent only described future work without durable task progress"}
	}
	return runLivenessResult{State: runLivenessAdvanced, Reason: "agent posted substantive office output"}
}

func (l *Launcher) classifyHeadlessOfficeTurnLiveness(slug string, active *headlessCodexActiveTurn, task *teamTask) runLivenessResult {
	if active == nil {
		return runLivenessResult{State: runLivenessAdvanced, Reason: "no active turn to classify"}
	}
	message, hadMessage := l.latestAgentSubstantiveMessageSince(slug, active.StartedAt)
	executed, attempted := l.taskHasExternalWorkflowEvidenceSince(task, active.StartedAt)
	return classifyRunLiveness(runLivenessInput{
		RunSucceeded:       true,
		Task:               task,
		MessageContent:     message.Content,
		HadSubstantivePost: hadMessage,
		HadTaskMutation:    l.agentRecordedTaskProgressSince(taskIDForLiveness(task), slug, active.StartedAt),
		HadExternalAttempt: attempted,
		HadExternalSuccess: executed,
		WorkspaceChanged:   l.headlessWorkspaceChanged(active),
	})
}

func (l *Launcher) recordHeadlessLiveness(slug string, active *headlessCodexActiveTurn, result runLivenessResult) {
	if l == nil || l.broker == nil || active == nil || strings.TrimSpace(slug) == "" || result.State == "" {
		return
	}
	l.broker.UpdateAgentActivity(agentActivitySnapshot{
		Slug:           slug,
		Channel:        normalizeChannelSlug(active.Turn.Channel),
		LivenessState:  string(result.State),
		LivenessReason: strings.TrimSpace(result.Reason),
		LivenessTaskID: strings.TrimSpace(active.Turn.TaskID),
		LivenessAt:     time.Now().UTC().Format(time.RFC3339),
	})
}

func taskIDForLiveness(task *teamTask) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.ID)
}

func (l *Launcher) headlessWorkspaceChanged(active *headlessCodexActiveTurn) bool {
	if active == nil {
		return false
	}
	workspaceDir := strings.TrimSpace(active.WorkspaceDir)
	before := strings.TrimSpace(active.WorkspaceSnapshot)
	if workspaceDir == "" || before == "" {
		return false
	}
	return headlessCodexWorkspaceStatusSnapshot(workspaceDir) != before
}

func (l *Launcher) latestAgentSubstantiveMessageSince(slug string, startedAt time.Time) (channelMessage, bool) {
	if l == nil || l.broker == nil {
		return channelMessage{}, false
	}
	msgs := l.broker.AllMessages()
	for i := len(msgs) - 1; i >= 0; i-- {
		msg := msgs[i]
		if msg.From != slug {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if !messageIsSubstantiveOfficeContent(content) {
			continue
		}
		when, err := time.Parse(time.RFC3339, strings.TrimSpace(msg.Timestamp))
		if err != nil {
			when, err = time.Parse(time.RFC3339Nano, strings.TrimSpace(msg.Timestamp))
		}
		if err != nil || !when.Add(time.Second).After(startedAt) {
			continue
		}
		return msg, true
	}
	return channelMessage{}, false
}

func runLivenessTaskAllowsNarrativeProgress(task *teamTask) bool {
	if task == nil {
		return true
	}
	structuredText := normalizeCoordinationText(strings.Join([]string{
		task.TaskType,
		task.PipelineID,
		task.ExecutionMode,
	}, " "))
	if containsAnyNormalizedFragment(structuredText,
		"research", "pesquisa", "discovery", "analysis", "analise",
		"planejamento", "planning", "plan", "documentacao", "docs", "doc",
		"review", "revisao", "follow_up",
	) {
		return true
	}
	descriptiveText := normalizeCoordinationText(strings.Join([]string{
		task.Title,
		task.Details,
	}, " "))
	if descriptiveText == "" {
		return false
	}
	return containsAnyNormalizedFragment(descriptiveText,
		"research", "pesquisa", "discovery", "investigar", "analysis", "analise", "analise",
		"planejamento", "planning", "documentacao", "documentacao", "docs",
		"review", "revisao", "follow up", "follow_up", "responder", "reply",
	)
}

func messageLooksLikePlanOnly(content string) bool {
	normalized := normalizeCoordinationText(content)
	if normalized == "" || !messageIsSubstantiveOfficeContent(content) {
		return false
	}
	if containsAnyNormalizedFragment(normalized,
		"conclui", "concluido", "feito", "entreguei", "implementei", "apliquei", "corrigi",
		"validado", "verificado", "resultado", "segue", "pronto", "done", "completed",
	) {
		return false
	}
	futureSignals := []string{
		"vou ",
		"irei ",
		"pretendo ",
		"planejo ",
		"agora vou ",
		"em seguida vou ",
		"next i will ",
		"i will ",
		"i'll ",
		"going to ",
		"i plan to ",
	}
	for _, signal := range futureSignals {
		if strings.Contains(normalized, signal) {
			return true
		}
	}
	return false
}
