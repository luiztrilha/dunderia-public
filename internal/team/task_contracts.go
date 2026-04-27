package team

import (
	"fmt"
	"strings"
)

type taskHandoffRecord struct {
	StatusClaim       string              `json:"status_claim,omitempty"`
	Summary           string              `json:"summary,omitempty"`
	Touched           []string            `json:"touched,omitempty"`
	Validation        []string            `json:"validation,omitempty"`
	Deviations        []string            `json:"deviations,omitempty"`
	DownstreamContext string              `json:"downstream_context,omitempty"`
	Blockers          []taskBlocker       `json:"blockers,omitempty"`
	ReviewFindings    []taskReviewFinding `json:"review_findings,omitempty"`
	AcceptedAt        string              `json:"accepted_at,omitempty"`
	AcceptedBy        string              `json:"accepted_by,omitempty"`
}

type taskBlocker struct {
	ID        string `json:"id,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Question  string `json:"question,omitempty"`
	WaitingOn string `json:"waiting_on,omitempty"`
	Need      string `json:"need,omitempty"`
	Context   string `json:"context,omitempty"`
	NewDemand bool   `json:"new_demand,omitempty"`
}

type taskReviewFinding struct {
	ID          string `json:"id,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Location    string `json:"location,omitempty"`
	Description string `json:"description,omitempty"`
	Guidance    string `json:"guidance,omitempty"`
	Status      string `json:"status,omitempty"`
	ResolvedAt  string `json:"resolved_at,omitempty"`
	ResolvedBy  string `json:"resolved_by,omitempty"`
	NewDemand   bool   `json:"new_demand,omitempty"`
}

type taskReviewFindingBatch struct {
	AcceptedAt string              `json:"accepted_at,omitempty"`
	AcceptedBy string              `json:"accepted_by,omitempty"`
	Findings   []taskReviewFinding `json:"findings,omitempty"`
}

type taskReconciliationState struct {
	Status         string   `json:"status,omitempty"`
	Reason         string   `json:"reason,omitempty"`
	WorkspacePath  string   `json:"workspace_path,omitempty"`
	ObservedDelta  string   `json:"observed_delta,omitempty"`
	ChangedPaths   []string `json:"changed_paths,omitempty"`
	UntrackedPaths []string `json:"untracked_paths,omitempty"`
	DetectedAt     string   `json:"detected_at,omitempty"`
	Blocking       bool     `json:"blocking,omitempty"`
}

type taskTransitionResult struct {
	Status              string
	ReviewState         string
	Blocked             bool
	ClearOwner          bool
	ClearReconciliation bool
	ClearSchedule       bool
}

type taskTransitionRule struct {
	allow func(task *teamTask) bool
	apply func(task *teamTask, handoff *taskHandoffRecord) taskTransitionResult
}

func agentRequiresStrictTaskHandoff(actor string) bool {
	return !isHumanLikeActor(actor) && !isSystemActor(actor)
}

func taskActionNeedsStrictHandoff(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "complete", "review", "block", "reconcile":
		return true
	default:
		return false
	}
}

func taskActionAcceptsReviewFindings(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "approve", "block", "review":
		return true
	default:
		return false
	}
}

func taskActionResubmitsForReview(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "complete", "review":
		return true
	default:
		return false
	}
}

func normalizeTaskStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "open", "in_progress", "review", "blocked", "done", "failed":
		return strings.ToLower(strings.TrimSpace(value))
	case "cancelled", "canceled":
		return "canceled"
	default:
		return ""
	}
}

func normalizeTaskReviewState(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pending_review", "ready_for_review", "changes_requested", "approved", "not_required":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func taskActiveReviewState(task *teamTask) string {
	if taskNeedsStructuredReview(task) {
		return "pending_review"
	}
	return "not_required"
}

func taskResolvedReviewState(task *teamTask) string {
	if taskNeedsStructuredReview(task) {
		return "approved"
	}
	return "not_required"
}

func taskCanAdvanceWhileNonTerminal(task *teamTask) bool {
	return task != nil && !taskIsTerminal(task)
}

func taskCanComplete(task *teamTask) bool {
	if task == nil {
		return false
	}
	status := normalizeTaskStatus(firstNonEmpty(task.Status, "open"))
	return status != "blocked" && status != "canceled" && status != "failed"
}

func taskCanMoveToReview(task *teamTask) bool {
	if task == nil {
		return false
	}
	return taskCanAdvanceWhileNonTerminal(task) && normalizeTaskStatus(task.Status) != "blocked"
}

func taskCanApprove(task *teamTask) bool {
	if task == nil {
		return false
	}
	switch normalizeTaskStatus(task.Status) {
	case "review":
		return true
	}
	switch normalizeTaskReviewState(task.ReviewState) {
	case "ready_for_review", "changes_requested":
		return true
	default:
		return false
	}
}

func taskTransitionRules() map[string]taskTransitionRule {
	return map[string]taskTransitionRule{
		"claim": {
			allow: taskCanAdvanceWhileNonTerminal,
			apply: func(task *teamTask, _ *taskHandoffRecord) taskTransitionResult {
				return taskTransitionResult{
					Status:      "in_progress",
					ReviewState: taskActiveReviewState(task),
					Blocked:     false,
				}
			},
		},
		"assign": {
			allow: taskCanAdvanceWhileNonTerminal,
			apply: func(task *teamTask, _ *taskHandoffRecord) taskTransitionResult {
				return taskTransitionResult{
					Status:      "in_progress",
					ReviewState: taskActiveReviewState(task),
					Blocked:     false,
				}
			},
		},
		"reassign": {
			allow: taskCanAdvanceWhileNonTerminal,
			apply: func(task *teamTask, _ *taskHandoffRecord) taskTransitionResult {
				switch normalizeTaskStatus(task.Status) {
				case "review":
					reviewState := normalizeTaskReviewState(task.ReviewState)
					if reviewState == "" || reviewState == "pending_review" {
						reviewState = "ready_for_review"
					}
					return taskTransitionResult{
						Status:      "review",
						ReviewState: reviewState,
						Blocked:     false,
					}
				case "done":
					return taskTransitionResult{
						Status:      "done",
						ReviewState: taskResolvedReviewState(task),
						Blocked:     false,
					}
				default:
					return taskTransitionResult{
						Status:      "in_progress",
						ReviewState: taskActiveReviewState(task),
						Blocked:     false,
					}
				}
			},
		},
		"complete": {
			allow: taskCanComplete,
			apply: func(task *teamTask, _ *taskHandoffRecord) taskTransitionResult {
				status := normalizeTaskStatus(task.Status)
				reviewState := normalizeTaskReviewState(task.ReviewState)
				switch {
				case status == "done":
					return taskTransitionResult{
						Status:      "done",
						ReviewState: taskResolvedReviewState(task),
						Blocked:     false,
					}
				case status == "review" || reviewState == "ready_for_review":
					return taskTransitionResult{
						Status:      "done",
						ReviewState: taskResolvedReviewState(task),
						Blocked:     false,
					}
				case taskNeedsStructuredReview(task):
					return taskTransitionResult{
						Status:      "review",
						ReviewState: "ready_for_review",
						Blocked:     false,
					}
				default:
					return taskTransitionResult{
						Status:      "done",
						ReviewState: "not_required",
						Blocked:     false,
					}
				}
			},
		},
		"review": {
			allow: taskCanMoveToReview,
			apply: func(task *teamTask, _ *taskHandoffRecord) taskTransitionResult {
				return taskTransitionResult{
					Status:      "review",
					ReviewState: "ready_for_review",
					Blocked:     false,
				}
			},
		},
		"approve": {
			allow: taskCanApprove,
			apply: func(task *teamTask, handoff *taskHandoffRecord) taskTransitionResult {
				if handoff != nil && len(handoff.ReviewFindings) > 0 {
					return taskTransitionResult{
						Status:      "review",
						ReviewState: "changes_requested",
						Blocked:     false,
					}
				}
				if taskHasBlockingReviewFindings(task) {
					return taskTransitionResult{
						Status:      "review",
						ReviewState: "changes_requested",
						Blocked:     false,
					}
				}
				return taskTransitionResult{
					Status:      "done",
					ReviewState: taskResolvedReviewState(task),
					Blocked:     false,
				}
			},
		},
		"block": {
			allow: taskCanAdvanceWhileNonTerminal,
			apply: func(task *teamTask, _ *taskHandoffRecord) taskTransitionResult {
				return taskTransitionResult{
					Status:      "blocked",
					ReviewState: normalizeTaskReviewState(task.ReviewState),
					Blocked:     true,
				}
			},
		},
		"release": {
			allow: taskCanAdvanceWhileNonTerminal,
			apply: func(task *teamTask, _ *taskHandoffRecord) taskTransitionResult {
				return taskTransitionResult{
					Status:      "open",
					ReviewState: "",
					Blocked:     false,
					ClearOwner:  true,
				}
			},
		},
		"cancel": {
			allow: taskCanAdvanceWhileNonTerminal,
			apply: func(task *teamTask, _ *taskHandoffRecord) taskTransitionResult {
				return taskTransitionResult{
					Status:        "canceled",
					ReviewState:   "",
					Blocked:       false,
					ClearSchedule: true,
				}
			},
		},
		"reconcile": {
			allow: taskHasPendingReconciliation,
			apply: func(task *teamTask, _ *taskHandoffRecord) taskTransitionResult {
				reviewState := ""
				status := "open"
				if strings.TrimSpace(task.Owner) != "" {
					status = "in_progress"
					reviewState = normalizeTaskReviewState(task.ReviewState)
					if reviewState == "" {
						reviewState = taskActiveReviewState(task)
					}
				}
				return taskTransitionResult{
					Status:              status,
					ReviewState:         reviewState,
					Blocked:             false,
					ClearReconciliation: true,
				}
			},
		},
	}
}

func taskTransitionConflict(task *teamTask, action string) error {
	taskID := ""
	status := ""
	reviewState := ""
	if task != nil {
		taskID = strings.TrimSpace(task.ID)
		status = normalizeTaskStatus(task.Status)
		reviewState = normalizeTaskReviewState(task.ReviewState)
	}
	if taskID == "" {
		return fmt.Errorf("task cannot %s from status %q with review_state %q", action, status, reviewState)
	}
	return fmt.Errorf("task %s cannot %s from status %q with review_state %q", taskID, action, status, reviewState)
}

func taskTransitionRuleForAction(action string) (taskTransitionRule, bool) {
	action = strings.ToLower(strings.TrimSpace(action))
	rule, ok := taskTransitionRules()[action]
	return rule, ok
}

func resolveTaskTransition(task *teamTask, action string, handoff *taskHandoffRecord) (taskTransitionResult, error) {
	if task == nil {
		return taskTransitionResult{}, fmt.Errorf("task required")
	}
	rule, ok := taskTransitionRuleForAction(action)
	if !ok {
		return taskTransitionResult{}, fmt.Errorf("unknown action")
	}
	if rule.allow != nil && !rule.allow(task) {
		return taskTransitionResult{}, taskTransitionConflict(task, action)
	}
	result := rule.apply(task, handoff)
	result.Status = normalizeTaskStatus(result.Status)
	result.ReviewState = normalizeTaskReviewState(result.ReviewState)
	return result, nil
}

func parseStructuredTaskHandoff(details string) (*taskHandoffRecord, error) {
	sections := splitStructuredTaskSections(details)
	taskReport := strings.TrimSpace(sections["task report"])
	if taskReport == "" {
		return nil, fmt.Errorf("structured handoff requires a ## Task Report section")
	}
	downstream := strings.TrimSpace(sections["downstream context"])
	if downstream == "" {
		return nil, fmt.Errorf("structured handoff requires a ## Downstream Context section")
	}

	reportFields := parseSimpleSectionFields(taskReport)
	statusClaim := strings.ToLower(strings.TrimSpace(firstSectionValue(reportFields["status"])))
	summary := strings.TrimSpace(firstSectionValue(reportFields["summary"]))
	if statusClaim == "" {
		return nil, fmt.Errorf("structured handoff task report requires Status")
	}
	if summary == "" {
		return nil, fmt.Errorf("structured handoff task report requires Summary")
	}

	handoff := &taskHandoffRecord{
		StatusClaim:       statusClaim,
		Summary:           summary,
		Touched:           splitSectionList(reportFields["touched"]),
		Validation:        splitSectionList(reportFields["validation"]),
		Deviations:        splitSectionList(reportFields["deviations"]),
		DownstreamContext: downstream,
	}

	if blockersSection := strings.TrimSpace(sections["blockers"]); blockersSection != "" {
		blockers, err := parseTaskBlockersSection(blockersSection)
		if err != nil {
			return nil, err
		}
		handoff.Blockers = blockers
	}
	if findingsSection := strings.TrimSpace(sections["review findings"]); findingsSection != "" {
		findings, err := parseTaskReviewFindingsSection(findingsSection)
		if err != nil {
			return nil, err
		}
		handoff.ReviewFindings = findings
	}

	return handoff, nil
}

func splitStructuredTaskSections(details string) map[string]string {
	sections := map[string]string{}
	lines := strings.Split(strings.ReplaceAll(details, "\r\n", "\n"), "\n")
	current := ""
	var body []string
	flush := func() {
		if current == "" {
			return
		}
		sections[current] = strings.TrimSpace(strings.Join(body, "\n"))
	}
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "## ") {
			flush()
			current = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "## ")))
			body = body[:0]
			continue
		}
		if current == "" {
			continue
		}
		body = append(body, raw)
	}
	flush()
	return sections
}

func parseSimpleSectionFields(section string) map[string][]string {
	fields := make(map[string][]string)
	for _, raw := range strings.Split(section, "\n") {
		line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "-"))
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		fields[key] = append(fields[key], value)
	}
	return fields
}

func firstSectionValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func splitSectionList(values []string) []string {
	var out []string
	for _, value := range values {
		for _, part := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == ';'
		}) {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return compactStringList(out)
}

func parseTaskBlockersSection(section string) ([]taskBlocker, error) {
	rawBlocks := splitStructuredDataBlocks(section)
	if len(rawBlocks) == 0 {
		return nil, fmt.Errorf("structured handoff blockers section must not be empty")
	}
	blockers := make([]taskBlocker, 0, len(rawBlocks))
	for i, block := range rawBlocks {
		kind := normalizeTaskBlockerKind(firstSectionValue(block["kind"]))
		question := strings.TrimSpace(firstSectionValue(block["question"]))
		need := strings.TrimSpace(firstSectionValue(block["need"]))
		context := strings.TrimSpace(firstSectionValue(block["context"]))
		if kind == "" || question == "" || need == "" || context == "" {
			return nil, fmt.Errorf("structured handoff blocker %d requires Kind, Question, Need, and Context", i+1)
		}
		blockers = append(blockers, taskBlocker{
			ID:        fmt.Sprintf("blocker-%d", i+1),
			Kind:      kind,
			Question:  question,
			WaitingOn: strings.TrimSpace(firstSectionValue(block["waiting on"])),
			Need:      need,
			Context:   context,
			NewDemand: parseTaskDemandMarker(block),
		})
	}
	return blockers, nil
}

func parseTaskReviewFindingsSection(section string) ([]taskReviewFinding, error) {
	rawBlocks := splitStructuredDataBlocks(section)
	if len(rawBlocks) == 0 {
		return nil, fmt.Errorf("review findings section must not be empty")
	}
	findings := make([]taskReviewFinding, 0, len(rawBlocks))
	for i, block := range rawBlocks {
		severity := normalizeTaskReviewFindingSeverity(firstSectionValue(block["severity"]))
		location := strings.TrimSpace(firstSectionValue(block["location"]))
		description := strings.TrimSpace(firstSectionValue(block["description"]))
		guidance := strings.TrimSpace(firstSectionValue(block["guidance"]))
		if severity == "" || location == "" || description == "" || guidance == "" {
			return nil, fmt.Errorf("review finding %d requires Severity, Location, Description, and Guidance", i+1)
		}
		findings = append(findings, taskReviewFinding{
			ID:          fmt.Sprintf("finding-%d", i+1),
			Severity:    severity,
			Location:    location,
			Description: description,
			Guidance:    guidance,
			Status:      "open",
			NewDemand:   parseTaskDemandMarker(block),
		})
	}
	return findings, nil
}

func parseTaskDemandMarker(block map[string][]string) bool {
	for _, key := range []string{"new demand", "nova demanda"} {
		if values, ok := block[key]; ok && parseTaskDemandValues(values) {
			return true
		}
	}
	return false
}

func parseTaskDemandValues(values []string) bool {
	for _, value := range values {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "yes", "y", "true", "1", "sim", "s":
			return true
		}
	}
	return false
}

func splitStructuredDataBlocks(section string) []map[string][]string {
	var blocks []map[string][]string
	current := make(map[string][]string)
	flush := func() {
		if len(current) == 0 {
			return
		}
		blocks = append(blocks, current)
		current = make(map[string][]string)
	}
	for _, raw := range strings.Split(section, "\n") {
		line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "-"))
		if line == "" {
			flush()
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		current[key] = append(current[key], value)
	}
	flush()
	return blocks
}

func normalizeTaskBlockerKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "approval", "clarification", "environment":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizeTaskReviewFindingSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical", "major", "minor", "note":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func requestKindForTaskBlocker(kind string) string {
	switch normalizeTaskBlockerKind(kind) {
	case "approval":
		return "approval"
	case "clarification", "environment":
		return "interview"
	default:
		return "interview"
	}
}

func taskHasBlockingReviewFindings(task *teamTask) bool {
	if task == nil {
		return false
	}
	return countBlockingReviewFindings(task.ReviewFindings) > 0
}

func countBlockingReviewFindings(findings []taskReviewFinding) int {
	count := 0
	for _, finding := range findings {
		if strings.TrimSpace(finding.Status) == "resolved" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(finding.Severity)) {
		case "critical", "major":
			count++
		}
	}
	return count
}

func taskHasPendingReconciliation(task *teamTask) bool {
	if task == nil || task.Reconciliation == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(task.Reconciliation.Status), "pending") && task.Reconciliation.Blocking
}
