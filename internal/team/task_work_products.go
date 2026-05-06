package team

import (
	"fmt"
	"strings"
	"time"
)

type taskArtifact struct {
	ID                string                 `json:"id,omitempty"`
	Kind              string                 `json:"kind,omitempty"`
	ResultRole        string                 `json:"result_role,omitempty"`
	Title             string                 `json:"title,omitempty"`
	Summary           string                 `json:"summary,omitempty"`
	Path              string                 `json:"path,omitempty"`
	URL               string                 `json:"url,omitempty"`
	PreviewURL        string                 `json:"preview_url,omitempty"`
	MIMEType          string                 `json:"mime_type,omitempty"`
	SizeBytes         int64                  `json:"size_bytes,omitempty"`
	Checksum          string                 `json:"checksum,omitempty"`
	State             string                 `json:"state,omitempty"`
	BrowserInspection *taskBrowserInspection `json:"browser_inspection,omitempty"`
	ValidatedBy       string                 `json:"validated_by,omitempty"`
	ValidatedAt       string                 `json:"validated_at,omitempty"`
	CreatedBy         string                 `json:"created_by,omitempty"`
	CreatedAt         string                 `json:"created_at,omitempty"`
	UpdatedAt         string                 `json:"updated_at,omitempty"`
}

type taskBrowserInspection struct {
	PageURL        string `json:"page_url,omitempty"`
	Selector       string `json:"selector,omitempty"`
	ElementText    string `json:"element_text,omitempty"`
	ScreenshotPath string `json:"screenshot_path,omitempty"`
	ViewportWidth  int    `json:"viewport_width,omitempty"`
	ViewportHeight int    `json:"viewport_height,omitempty"`
	Notes          string `json:"notes,omitempty"`
}

type taskExecutionLock struct {
	RunID       string `json:"run_id,omitempty"`
	Owner       string `json:"owner,omitempty"`
	Status      string `json:"status,omitempty"`
	AcquiredAt  string `json:"acquired_at,omitempty"`
	HeartbeatAt string `json:"heartbeat_at,omitempty"`
	ExpiresAt   string `json:"expires_at,omitempty"`
}

type taskPlanRevision struct {
	ID         string `json:"id,omitempty"`
	Version    int    `json:"version,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Content    string `json:"content,omitempty"`
	Status     string `json:"status,omitempty"`
	ApprovedBy string `json:"approved_by,omitempty"`
	ApprovedAt string `json:"approved_at,omitempty"`
	CreatedBy  string `json:"created_by,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
}

type taskLearningCandidate struct {
	Recommended bool   `json:"recommended,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Title       string `json:"title,omitempty"`
	Summary     string `json:"summary,omitempty"`
	SkillName   string `json:"skill_name,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type taskExecutionLimits struct {
	MaxAttempts       int    `json:"max_attempts,omitempty"`
	MaxRuntimeMinutes int    `json:"max_runtime_minutes,omitempty"`
	MaxCostCents      int    `json:"max_cost_cents,omitempty"`
	AttemptsUsed      int    `json:"attempts_used,omitempty"`
	RuntimeMsUsed     int64  `json:"runtime_ms_used,omitempty"`
	CostCentsUsed     int    `json:"cost_cents_used,omitempty"`
	LimitState        string `json:"limit_state,omitempty"`
	LastAttemptAt     string `json:"last_attempt_at,omitempty"`
	LastLimitReason   string `json:"last_limit_reason,omitempty"`
}

type taskFeedback struct {
	ID        string `json:"id,omitempty"`
	Rating    string `json:"rating,omitempty"`
	Comment   string `json:"comment,omitempty"`
	CreatedBy string `json:"created_by,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

type taskEvalSignal struct {
	ID        string `json:"id,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Severity  string `json:"severity,omitempty"`
	Summary   string `json:"summary,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

func normalizeTaskArtifactKind(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	kind = strings.ReplaceAll(kind, " ", "_")
	kind = strings.ReplaceAll(kind, "-", "_")
	if kind == "" {
		return "artifact"
	}
	return kind
}

func normalizeTaskArtifactRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	role = strings.ReplaceAll(role, " ", "_")
	role = strings.ReplaceAll(role, "-", "_")
	switch role {
	case "primary", "supporting", "evidence", "preview", "release", "log":
		return role
	default:
		return ""
	}
}

func normalizeTaskArtifact(input taskArtifact, actor, now string, idFactory func() string) (taskArtifact, error) {
	artifact := taskArtifact{
		ID:          strings.TrimSpace(input.ID),
		Kind:        normalizeTaskArtifactKind(input.Kind),
		ResultRole:  normalizeTaskArtifactRole(input.ResultRole),
		Title:       strings.TrimSpace(input.Title),
		Summary:     strings.TrimSpace(input.Summary),
		Path:        strings.TrimSpace(input.Path),
		URL:         strings.TrimSpace(input.URL),
		PreviewURL:  strings.TrimSpace(input.PreviewURL),
		MIMEType:    strings.TrimSpace(input.MIMEType),
		SizeBytes:   input.SizeBytes,
		Checksum:    strings.TrimSpace(input.Checksum),
		State:       strings.TrimSpace(input.State),
		ValidatedBy: strings.TrimSpace(input.ValidatedBy),
		ValidatedAt: strings.TrimSpace(input.ValidatedAt),
		CreatedBy:   strings.TrimSpace(firstNonEmpty(input.CreatedBy, actor)),
		CreatedAt:   strings.TrimSpace(firstNonEmpty(input.CreatedAt, now)),
		UpdatedAt:   strings.TrimSpace(firstNonEmpty(input.UpdatedAt, now)),
	}
	if input.BrowserInspection != nil {
		inspection := normalizeTaskBrowserInspection(input.BrowserInspection)
		if !taskBrowserInspectionHasEvidence(inspection) {
			return taskArtifact{}, fmt.Errorf("browser inspection requires page_url, selector, screenshot_path, notes, or element_text")
		}
		artifact.BrowserInspection = inspection
		if artifact.URL == "" {
			artifact.URL = inspection.PageURL
		}
		if artifact.PreviewURL == "" {
			artifact.PreviewURL = inspection.PageURL
		}
		if artifact.Path == "" {
			artifact.Path = inspection.ScreenshotPath
		}
		if artifact.Summary == "" {
			artifact.Summary = browserInspectionSummary(inspection)
		}
	}
	if artifact.Path == "" && artifact.URL == "" && artifact.PreviewURL == "" && artifact.Summary == "" {
		return taskArtifact{}, fmt.Errorf("artifact requires path, url, or summary")
	}
	if artifact.SizeBytes < 0 {
		artifact.SizeBytes = 0
	}
	if artifact.ID == "" && idFactory != nil {
		artifact.ID = idFactory()
	}
	if artifact.Title == "" {
		artifact.Title = firstNonEmpty(artifact.Path, artifact.URL, artifact.PreviewURL, artifact.Kind)
	}
	if artifact.ResultRole == "" {
		switch artifact.Kind {
		case "build", "pull_request", "release_note":
			artifact.ResultRole = "release"
		case "log":
			artifact.ResultRole = "log"
		case "browser_inspection":
			artifact.ResultRole = "evidence"
		default:
			artifact.ResultRole = "supporting"
		}
	}
	if artifact.ValidatedAt == "" && strings.EqualFold(artifact.State, "verified") {
		artifact.ValidatedAt = now
	}
	if artifact.ValidatedBy == "" && artifact.ValidatedAt != "" {
		artifact.ValidatedBy = artifact.CreatedBy
	}
	return artifact, nil
}

func normalizeTaskBrowserInspection(input *taskBrowserInspection) *taskBrowserInspection {
	if input == nil {
		return nil
	}
	out := &taskBrowserInspection{
		PageURL:        strings.TrimSpace(input.PageURL),
		Selector:       strings.TrimSpace(input.Selector),
		ElementText:    strings.TrimSpace(input.ElementText),
		ScreenshotPath: strings.TrimSpace(input.ScreenshotPath),
		ViewportWidth:  input.ViewportWidth,
		ViewportHeight: input.ViewportHeight,
		Notes:          strings.TrimSpace(input.Notes),
	}
	if out.ViewportWidth < 0 {
		out.ViewportWidth = 0
	}
	if out.ViewportHeight < 0 {
		out.ViewportHeight = 0
	}
	return out
}

func taskBrowserInspectionHasEvidence(input *taskBrowserInspection) bool {
	if input == nil {
		return false
	}
	return input.PageURL != "" || input.Selector != "" || input.ElementText != "" || input.ScreenshotPath != "" || input.Notes != ""
}

func browserInspectionSummary(input *taskBrowserInspection) string {
	if input == nil {
		return ""
	}
	parts := []string{}
	if input.PageURL != "" {
		parts = append(parts, "url="+input.PageURL)
	}
	if input.Selector != "" {
		parts = append(parts, "selector="+input.Selector)
	}
	if input.ViewportWidth > 0 && input.ViewportHeight > 0 {
		parts = append(parts, fmt.Sprintf("viewport=%dx%d", input.ViewportWidth, input.ViewportHeight))
	}
	if input.ElementText != "" {
		parts = append(parts, "text="+truncateSummary(input.ElementText, 120))
	}
	if input.Notes != "" {
		parts = append(parts, "notes="+truncateSummary(input.Notes, 120))
	}
	return strings.Join(parts, "; ")
}

func appendTaskArtifact(task *teamTask, artifact taskArtifact) {
	if task == nil || strings.TrimSpace(artifact.ID) == "" {
		return
	}
	for i := range task.Artifacts {
		if strings.TrimSpace(task.Artifacts[i].ID) == artifact.ID {
			task.Artifacts[i] = artifact
			return
		}
	}
	task.Artifacts = append(task.Artifacts, artifact)
}

func normalizeTaskExecutionLockStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active", "released", "expired":
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return "active"
	}
}

func taskExecutionLockIsActive(lock *taskExecutionLock, now time.Time) bool {
	if lock == nil || normalizeTaskExecutionLockStatus(lock.Status) != "active" {
		return false
	}
	expiresAt := strings.TrimSpace(lock.ExpiresAt)
	if expiresAt == "" {
		return true
	}
	parsed, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return true
	}
	return parsed.After(now.UTC())
}

func normalizeTaskExecutionLock(lock *taskExecutionLock, now time.Time) *taskExecutionLock {
	if lock == nil {
		return nil
	}
	out := *lock
	out.RunID = strings.TrimSpace(out.RunID)
	out.Owner = strings.TrimSpace(out.Owner)
	out.Status = normalizeTaskExecutionLockStatus(out.Status)
	out.AcquiredAt = strings.TrimSpace(out.AcquiredAt)
	out.HeartbeatAt = strings.TrimSpace(out.HeartbeatAt)
	out.ExpiresAt = strings.TrimSpace(out.ExpiresAt)
	if out.RunID == "" || out.Owner == "" {
		return nil
	}
	if out.AcquiredAt == "" {
		out.AcquiredAt = now.UTC().Format(time.RFC3339)
	}
	if out.HeartbeatAt == "" {
		out.HeartbeatAt = out.AcquiredAt
	}
	if out.Status == "active" && !taskExecutionLockIsActive(&out, now) {
		out.Status = "expired"
	}
	return &out
}

func latestTaskPlanRevision(task *teamTask) *taskPlanRevision {
	if task == nil || len(task.PlanRevisions) == 0 {
		return nil
	}
	latest := &task.PlanRevisions[0]
	for i := range task.PlanRevisions {
		if task.PlanRevisions[i].Version > latest.Version {
			latest = &task.PlanRevisions[i]
		}
	}
	return latest
}

func normalizeTaskPlanRevision(input taskPlanRevision, actor, now string, currentCount int, idFactory func() string) (taskPlanRevision, error) {
	revision := taskPlanRevision{
		ID:         strings.TrimSpace(input.ID),
		Version:    input.Version,
		Summary:    strings.TrimSpace(input.Summary),
		Content:    strings.TrimSpace(input.Content),
		Status:     normalizeTaskPlanRevisionStatus(input.Status),
		ApprovedBy: strings.TrimSpace(input.ApprovedBy),
		ApprovedAt: strings.TrimSpace(input.ApprovedAt),
		CreatedBy:  strings.TrimSpace(firstNonEmpty(input.CreatedBy, actor)),
		CreatedAt:  strings.TrimSpace(firstNonEmpty(input.CreatedAt, now)),
	}
	if revision.Content == "" {
		return taskPlanRevision{}, fmt.Errorf("plan revision requires content")
	}
	if revision.Status == "" {
		revision.Status = "draft"
	}
	if revision.Version <= 0 {
		revision.Version = currentCount + 1
	}
	if revision.ID == "" && idFactory != nil {
		revision.ID = idFactory()
	}
	if revision.Summary == "" {
		revision.Summary = truncateSummary(revision.Content, 120)
	}
	return revision, nil
}

func normalizeTaskPlanRevisionStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "draft", "ready", "approved", "superseded":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func appendTaskPlanRevision(task *teamTask, revision taskPlanRevision) {
	if task == nil || strings.TrimSpace(revision.ID) == "" {
		return
	}
	for i := range task.PlanRevisions {
		if strings.TrimSpace(task.PlanRevisions[i].ID) == revision.ID {
			task.PlanRevisions[i] = revision
			return
		}
	}
	task.PlanRevisions = append(task.PlanRevisions, revision)
}

func normalizeTaskLimitState(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ok", "warning", "exhausted", "paused":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func applyTaskLimits(task *teamTask, limits taskExecutionLimits) {
	if task == nil {
		return
	}
	if limits.MaxAttempts > 0 {
		task.Limits.MaxAttempts = limits.MaxAttempts
	}
	if limits.MaxRuntimeMinutes > 0 {
		task.Limits.MaxRuntimeMinutes = limits.MaxRuntimeMinutes
	}
	if limits.MaxCostCents > 0 {
		task.Limits.MaxCostCents = limits.MaxCostCents
	}
	if limits.AttemptsUsed >= 0 {
		task.Limits.AttemptsUsed = limits.AttemptsUsed
	}
	if limits.RuntimeMsUsed > 0 {
		task.Limits.RuntimeMsUsed = limits.RuntimeMsUsed
	}
	if limits.CostCentsUsed > 0 {
		task.Limits.CostCentsUsed = limits.CostCentsUsed
	}
	if state := normalizeTaskLimitState(limits.LimitState); state != "" {
		task.Limits.LimitState = state
	}
	normalizeTaskLimits(task)
}

func normalizeTaskLimits(task *teamTask) {
	if task == nil {
		return
	}
	task.Limits.LimitState = normalizeTaskLimitState(task.Limits.LimitState)
	if task.Limits.AttemptsUsed < 0 {
		task.Limits.AttemptsUsed = 0
	}
	if task.Limits.MaxAttempts > 0 && task.Limits.AttemptsUsed >= task.Limits.MaxAttempts {
		task.Limits.LimitState = "exhausted"
		task.Limits.LastLimitReason = "attempt limit reached"
	}
	if task.Limits.MaxRuntimeMinutes > 0 && task.Limits.RuntimeMsUsed >= int64(task.Limits.MaxRuntimeMinutes)*int64(timeMinuteMs()) {
		task.Limits.LimitState = "exhausted"
		task.Limits.LastLimitReason = "runtime limit reached"
	}
	if task.Limits.MaxCostCents > 0 && task.Limits.CostCentsUsed >= task.Limits.MaxCostCents {
		task.Limits.LimitState = "exhausted"
		task.Limits.LastLimitReason = "cost limit reached"
	}
	if task.Limits.LimitState == "" && (task.Limits.MaxAttempts > 0 || task.Limits.MaxRuntimeMinutes > 0 || task.Limits.MaxCostCents > 0) {
		task.Limits.LimitState = "ok"
	}
}

func taskLimitExhausted(task *teamTask) bool {
	if task == nil {
		return false
	}
	normalizeTaskLimits(task)
	return task.Limits.LimitState == "exhausted" || task.Limits.LimitState == "paused"
}

func normalizeTaskFeedbackRating(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "up", "down", "neutral":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizeTaskFeedback(input taskFeedback, actor, now string, idFactory func() string) (taskFeedback, error) {
	feedback := taskFeedback{
		ID:        strings.TrimSpace(input.ID),
		Rating:    normalizeTaskFeedbackRating(input.Rating),
		Comment:   strings.TrimSpace(input.Comment),
		CreatedBy: strings.TrimSpace(firstNonEmpty(input.CreatedBy, actor)),
		CreatedAt: strings.TrimSpace(firstNonEmpty(input.CreatedAt, now)),
	}
	if feedback.Rating == "" {
		return taskFeedback{}, fmt.Errorf("feedback rating required")
	}
	if feedback.ID == "" && idFactory != nil {
		feedback.ID = idFactory()
	}
	return feedback, nil
}

func appendTaskFeedback(task *teamTask, feedback taskFeedback) {
	if task == nil || strings.TrimSpace(feedback.ID) == "" {
		return
	}
	task.Feedback = append(task.Feedback, feedback)
}

func timeMinuteMs() int {
	return 60 * 1000
}

func normalizeTaskEvalSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "info", "warning", "error":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "info"
	}
}

func taskEvalID(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	kind = strings.ReplaceAll(kind, " ", "_")
	kind = strings.ReplaceAll(kind, "-", "_")
	return "eval-" + kind
}

func taskHasExternalPublication(task *teamTask) bool {
	if task == nil {
		return false
	}
	if task.IssuePublication != nil && strings.TrimSpace(task.IssuePublication.URL) != "" {
		return true
	}
	if task.PRPublication != nil && strings.TrimSpace(task.PRPublication.URL) != "" {
		return true
	}
	for _, artifact := range task.Artifacts {
		if strings.TrimSpace(artifact.Path) != "" || strings.TrimSpace(artifact.URL) != "" || strings.TrimSpace(artifact.Summary) != "" {
			return true
		}
	}
	return false
}

func evaluateTaskSignals(task *teamTask, now string) {
	if task == nil {
		return
	}
	now = strings.TrimSpace(now)
	if now == "" {
		now = strings.TrimSpace(firstNonEmpty(task.UpdatedAt, task.CreatedAt))
	}
	var evals []taskEvalSignal
	add := func(kind, severity, summary string) {
		kind = strings.TrimSpace(kind)
		summary = strings.TrimSpace(summary)
		if kind == "" || summary == "" {
			return
		}
		evals = append(evals, taskEvalSignal{
			ID:        taskEvalID(kind),
			Kind:      kind,
			Severity:  normalizeTaskEvalSeverity(severity),
			Summary:   summary,
			CreatedAt: now,
		})
	}
	status := normalizeTaskStatus(task.Status)
	if strings.TrimSpace(task.Outcome) != "" && status == "done" && strings.TrimSpace(task.OutcomeEvidence) == "" {
		add("outcome_missing_evidence", "warning", "Done task has an expected outcome but no verification evidence.")
	}
	if status == "done" && !taskHasExternalPublication(task) {
		add("done_without_artifact", "warning", "Done task has no artifact, publication, or evidence attached.")
	}
	if task.Limits.LimitState == "exhausted" || task.Limits.LimitState == "paused" {
		add("budget_guardrail", "error", firstNonEmpty(task.Limits.LastLimitReason, "Task budget is exhausted or paused."))
	}
	if taskNeedsStructuredReview(task) && status == "done" && strings.TrimSpace(task.ReviewState) != "approved" {
		add("review_not_approved", "warning", "Structured-review task is done without approved review state.")
	}
	task.Evals = evals
}
