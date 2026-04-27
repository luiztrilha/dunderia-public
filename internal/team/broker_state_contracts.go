package team

import (
	"fmt"
	"strings"
	"time"
)

type brokerMessageIndexSnapshot struct {
	Size           int               `json:"size,omitempty"`
	FirstID        string            `json:"first_id,omitempty"`
	LastID         string            `json:"last_id,omitempty"`
	ByChannel      map[string][]int  `json:"by_channel,omitempty"`
	ByToken        map[string][]int  `json:"by_token,omitempty"`
	ByAuthor       map[string][]int  `json:"by_author,omitempty"`
	ByThread       map[string][]int  `json:"by_thread,omitempty"`
	ThreadRootByID map[string]string `json:"thread_root_by_id,omitempty"`
}

type brokerObservabilitySample struct {
	RecordedAt          string `json:"recorded_at,omitempty"`
	ReadinessState      string `json:"readiness_state,omitempty"`
	ReadinessStage      string `json:"readiness_stage,omitempty"`
	PendingRequests     int    `json:"pending_requests,omitempty"`
	ActiveWatchdogs     int    `json:"active_watchdogs,omitempty"`
	SchedulerScheduled  int    `json:"scheduler_scheduled,omitempty"`
	SchedulerDue        int    `json:"scheduler_due,omitempty"`
	GitHubPending       int    `json:"github_pending,omitempty"`
	GitHubDeferred      int    `json:"github_deferred,omitempty"`
	GitHubOpened        int    `json:"github_opened,omitempty"`
	CloudBackupPending  bool   `json:"cloud_backup_pending,omitempty"`
	CloudBackupFailures int    `json:"cloud_backup_failures,omitempty"`
	HotPath             string `json:"hot_path,omitempty"`
	HotPathMaxMs        int64  `json:"hot_path_max_ms,omitempty"`
}

type requestTransitionRule struct {
	allow func(req *humanInterview) bool
	apply func(req *humanInterview, answer *interviewAnswer, now string)
}

type watchdogTransitionRule struct {
	allow func(alert *watchdogAlert) bool
	apply func(alert *watchdogAlert, owner, summary, now string)
}

func normalizeRequestStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "pending":
		return "pending"
	case "open":
		return "open"
	case "answered":
		return "answered"
	case "canceled", "cancelled", "closed":
		return "canceled"
	default:
		return ""
	}
}

func requestStatusIsTerminal(status string) bool {
	switch normalizeRequestStatus(status) {
	case "answered", "canceled":
		return true
	default:
		return false
	}
}

func requestTransitionRules() map[string]requestTransitionRule {
	return map[string]requestTransitionRule{
		"create": {
			allow: func(req *humanInterview) bool { return req != nil },
			apply: func(req *humanInterview, _ *interviewAnswer, now string) {
				status := normalizeRequestStatus(req.Status)
				switch {
				case req.Answered != nil:
					req.Status = "answered"
				case status == "":
					req.Status = "pending"
				default:
					req.Status = status
				}
				if strings.TrimSpace(req.UpdatedAt) == "" {
					req.UpdatedAt = now
				}
			},
		},
		"answer": {
			allow: func(req *humanInterview) bool { return req != nil && requestIsActive(*req) },
			apply: func(req *humanInterview, answer *interviewAnswer, now string) {
				req.Answered = answer
				req.Status = "answered"
				req.UpdatedAt = now
				clearRequestLifecycleFields(req)
			},
		},
		"cancel": {
			allow: func(req *humanInterview) bool { return req != nil && !requestStatusIsTerminal(req.Status) },
			apply: func(req *humanInterview, _ *interviewAnswer, now string) {
				req.Status = "canceled"
				req.UpdatedAt = now
				clearRequestLifecycleFields(req)
			},
		},
		"escalate": {
			allow: func(req *humanInterview) bool { return req != nil && requestIsActive(*req) },
			apply: func(req *humanInterview, _ *interviewAnswer, now string) {
				status := normalizeRequestStatus(req.Status)
				if status == "" {
					status = "pending"
				}
				req.Status = status
				req.UpdatedAt = now
			},
		},
	}
}

func clearRequestLifecycleFields(req *humanInterview) {
	if req == nil {
		return
	}
	req.DueAt = ""
	req.ReminderAt = ""
	req.RecheckAt = ""
	req.FollowUpAt = ""
	req.NextEscalationAt = ""
}

func normalizeRequestRecord(req humanInterview) humanInterview {
	req.Kind = normalizeRequestKind(req.Kind)
	req.Options, req.RecommendedID = normalizeRequestOptions(req.Kind, req.RecommendedID, req.Options)
	req.Status = normalizeRequestStatus(req.Status)
	if req.Answered != nil {
		req.Status = "answered"
	}
	if req.Status == "" {
		req.Status = "pending"
	}
	if requestNeedsHumanDecision(req) {
		req.Blocking = true
		req.Required = true
	}
	if requestStatusIsTerminal(req.Status) {
		clearRequestLifecycleFields(&req)
	}
	req.EscalationState = normalizeRequestEscalationState(req.EscalationState, req.EscalationCount)
	return req
}

func resolveRequestTransition(req humanInterview, action string, answer *interviewAnswer, now time.Time) (humanInterview, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	rule, ok := requestTransitionRules()[action]
	if !ok {
		return humanInterview{}, fmt.Errorf("unknown request transition %q", action)
	}
	req = normalizeRequestRecord(req)
	if !rule.allow(&req) {
		return humanInterview{}, fmt.Errorf("request transition %q is not allowed from status %q", action, req.Status)
	}
	if action == "answer" && answer == nil {
		return humanInterview{}, fmt.Errorf("request transition %q requires an answer", action)
	}
	stamp := ""
	if !now.IsZero() {
		stamp = now.UTC().Format(time.RFC3339)
	}
	rule.apply(&req, answer, stamp)
	return normalizeRequestRecord(req), nil
}

func normalizeWatchdogStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "active":
		return "active"
	case "resolved":
		return "resolved"
	default:
		return ""
	}
}

func watchdogTransitionRules() map[string]watchdogTransitionRule {
	return map[string]watchdogTransitionRule{
		"raise": {
			allow: func(_ *watchdogAlert) bool { return true },
			apply: func(alert *watchdogAlert, owner, summary, now string) {
				alert.Owner = strings.TrimSpace(owner)
				alert.Summary = strings.TrimSpace(summary)
				alert.Status = "active"
				if strings.TrimSpace(alert.CreatedAt) == "" {
					alert.CreatedAt = now
				}
				alert.UpdatedAt = now
			},
		},
		"resolve": {
			allow: func(alert *watchdogAlert) bool {
				return alert != nil && normalizeWatchdogStatus(alert.Status) != "resolved"
			},
			apply: func(alert *watchdogAlert, _, _ string, now string) {
				alert.Status = "resolved"
				if strings.TrimSpace(alert.CreatedAt) == "" {
					alert.CreatedAt = now
				}
				alert.UpdatedAt = now
			},
		},
	}
}

func resolveWatchdogTransition(alert watchdogAlert, action, owner, summary string, now time.Time) (watchdogAlert, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	rule, ok := watchdogTransitionRules()[action]
	if !ok {
		return watchdogAlert{}, fmt.Errorf("unknown watchdog transition %q", action)
	}
	alert.Status = normalizeWatchdogStatus(alert.Status)
	if alert.Status == "" {
		alert.Status = "active"
	}
	if !rule.allow(&alert) {
		return watchdogAlert{}, fmt.Errorf("watchdog transition %q is not allowed from status %q", action, alert.Status)
	}
	stamp := ""
	if !now.IsZero() {
		stamp = now.UTC().Format(time.RFC3339)
	}
	rule.apply(&alert, owner, summary, stamp)
	if alert.Status == "" {
		alert.Status = "active"
	}
	return alert, nil
}

func normalizeSchedulerStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "scheduled":
		return "scheduled"
	case "running":
		return "running"
	case "deferred":
		return "deferred"
	case "done":
		return "done"
	case "failed":
		return "failed"
	case "canceled", "cancelled":
		return "canceled"
	default:
		return ""
	}
}

func schedulerStatusIsTerminal(status string) bool {
	switch normalizeSchedulerStatus(status) {
	case "done", "failed", "canceled":
		return true
	default:
		return false
	}
}

func resolveSchedulerJobState(job schedulerJob, status string, nextRun, now time.Time) (schedulerJob, error) {
	job = normalizeSchedulerJob(job)
	nextStatus := normalizeSchedulerStatus(firstNonEmpty(status, job.Status))
	if nextStatus == "" {
		return schedulerJob{}, fmt.Errorf("invalid scheduler job status %q", status)
	}
	job.Status = nextStatus
	if !now.IsZero() {
		job.LastRun = now.UTC().Format(time.RFC3339)
	}
	if !nextRun.IsZero() {
		job.NextRun = nextRun.UTC().Format(time.RFC3339)
		job.DueAt = job.NextRun
		return job, nil
	}
	if schedulerStatusIsTerminal(job.Status) || status != "" {
		job.NextRun = ""
		job.DueAt = ""
	}
	return job, nil
}
