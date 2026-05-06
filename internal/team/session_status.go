package team

import "strings"

func normalizeRuntimeStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "active", "advanced", "executing", "in_progress", "running", "tool_use", "working":
		return "working"
	case "open", "pending", "queued", "scheduled", "waiting":
		return "waiting"
	case "awaiting_human", "human_input", "input_needed", "in_review", "permission_needed", "ready_for_review", "review":
		return "input_needed"
	case "blocked", "degraded", "needs_correction":
		return "blocked"
	case "accepted", "answered", "approved", "closed", "complete", "completed", "done", "ok", "success", "verified":
		return "completed"
	case "empty_response", "error", "fail", "failed":
		return "failed"
	case "cancelled", "canceled", "interrupted", "released":
		return "interrupted"
	case "expired", "fallback_dispatched", "needs_followup", "plan_only", "stale", "timed_out", "timeout":
		return "stale"
	case "", "idle", "info", "unknown":
		return "idle"
	default:
		return "waiting"
	}
}

func normalizeRuntimeLivenessStatus(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "advanced":
		return "working"
	case "completed":
		return "completed"
	case "blocked":
		return "blocked"
	case "empty_response", "failed":
		return "failed"
	case "needs_followup", "plan_only", "timed_out":
		return "stale"
	default:
		return ""
	}
}

func normalizeTaskRuntimeStatus(task teamTask) string {
	if task.Blocked || normalizeTaskStatus(task.Status) == "blocked" {
		return "blocked"
	}
	if task.AwaitingHuman || len(task.BlockerRequestIDs) > 0 {
		return "input_needed"
	}
	if task.PlanRequired && task.PlanStatus != "" && task.PlanStatus != "approved" {
		return "input_needed"
	}
	if task.CompletionEvidenceRequired && !task.CompletionEvidenceSatisfied && normalizeTaskStatus(task.Status) == "done" {
		return "input_needed"
	}
	if normalized := normalizeRuntimeLivenessStatus(task.LivenessState); normalized != "" {
		return normalized
	}
	switch normalizeTaskStatus(task.Status) {
	case "in_progress":
		return "working"
	case "review":
		return "input_needed"
	case "done":
		return "completed"
	case "failed":
		return "failed"
	case "canceled":
		return "interrupted"
	case "open", "":
		return "waiting"
	default:
		return normalizeRuntimeStatus(task.Status)
	}
}

func normalizeAgentSessionRuntimeStatus(session agentSessionSnapshot) string {
	if normalized := normalizeRuntimeLivenessStatus(session.LivenessState); normalized != "" {
		return normalized
	}
	if session.Status == "idle" && (session.QueuedNodeCount > 0 || session.OpenTaskCount > 0 || strings.TrimSpace(session.CurrentTaskID) != "") {
		return "waiting"
	}
	return normalizeRuntimeStatus(session.Status)
}

func normalizeSessionAgentRuntimeStatus(agent SessionAgentObservability) string {
	if normalized := normalizeRuntimeLivenessStatus(agent.LivenessState); normalized != "" {
		return normalized
	}
	if agent.Status == "idle" && (agent.QueueDepth > 0 || agent.QuickReplyQueueDepth > 0 || strings.TrimSpace(agent.ActiveTaskID) != "") {
		return "waiting"
	}
	return normalizeRuntimeStatus(agent.Status)
}

func normalizeExecutionTraceStepRuntimeStatus(step executionTraceStep) string {
	if step.Kind == "liveness" {
		if normalized := normalizeRuntimeLivenessStatus(step.Status); normalized != "" {
			return normalized
		}
	}
	if step.Kind == "message" && strings.TrimSpace(step.Status) == "" {
		return "completed"
	}
	return normalizeRuntimeStatus(step.Status)
}
