package team

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveTaskTransitionRejectsApproveOutsideReview(t *testing.T) {
	task := &teamTask{
		ID:            "task-1",
		Title:         "Implement broker contract",
		Status:        "in_progress",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	}

	_, err := resolveTaskTransition(task, "approve", nil)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "cannot approve") {
		t.Fatalf("expected approve outside review to fail, got %v", err)
	}
}

func TestResolveTaskTransitionRejectsReconcileWithoutPendingState(t *testing.T) {
	task := &teamTask{
		ID:            "task-2",
		Title:         "Repair drift",
		Status:        "blocked",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	}

	_, err := resolveTaskTransition(task, "reconcile", nil)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "cannot reconcile") {
		t.Fatalf("expected reconcile without pending state to fail, got %v", err)
	}
}

func TestResolveTaskTransitionCompletePromotesStructuredTaskToReview(t *testing.T) {
	task := &teamTask{
		ID:            "task-3",
		Title:         "Implement broker contract",
		Status:        "in_progress",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	}

	result, err := resolveTaskTransition(task, "complete", nil)
	if err != nil {
		t.Fatalf("resolveTaskTransition complete: %v", err)
	}
	if result.Status != "review" || result.ReviewState != "ready_for_review" || result.Blocked {
		t.Fatalf("expected review-ready transition, got %+v", result)
	}
}

func TestResolveTaskTransitionApproveWithFindingsRequestsChanges(t *testing.T) {
	task := &teamTask{
		ID:            "task-4",
		Title:         "Review broker contract",
		Status:        "review",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
		ReviewState:   "ready_for_review",
	}
	handoff := &taskHandoffRecord{
		ReviewFindings: []taskReviewFinding{{
			ID:          "finding-1",
			Severity:    "major",
			Location:    "internal/team/broker.go:1",
			Description: "Missing guard",
			Guidance:    "Add a guard",
			Status:      "open",
		}},
	}

	result, err := resolveTaskTransition(task, "approve", handoff)
	if err != nil {
		t.Fatalf("resolveTaskTransition approve: %v", err)
	}
	if result.Status != "review" || result.ReviewState != "changes_requested" || result.Blocked {
		t.Fatalf("expected changes requested transition, got %+v", result)
	}
}

func TestResolveTaskTransitionRejectsClaimOnTerminalTask(t *testing.T) {
	task := &teamTask{
		ID:            "task-5",
		Title:         "Closed task",
		Status:        "done",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	}

	_, err := resolveTaskTransition(task, "claim", nil)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "cannot claim") {
		t.Fatalf("expected claim on terminal task to fail, got %v", err)
	}
}

func TestBrokerTaskCompleteRequiresStructuredHandoffForAgent(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	if err := b.StartOnPort(0); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer b.Stop()

	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Implement the strict broker slice",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	base := fmt.Sprintf("http://%s", b.Addr())
	body, _ := json.Marshal(map[string]any{
		"action":     "complete",
		"channel":    "general",
		"id":         task.ID,
		"created_by": "builder",
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/tasks", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+b.Token())
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("task complete request failed: %v", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected conflict for missing handoff, got %d: %s", resp.StatusCode, raw)
	}
	if !strings.Contains(strings.ToLower(string(raw)), "handoff") {
		t.Fatalf("expected handoff guidance, got %s", raw)
	}

	var updated teamTask
	for _, candidate := range b.AllTasks() {
		if candidate.ID == task.ID {
			updated = candidate
			break
		}
	}
	if updated.Status != "in_progress" || updated.ReviewState != "pending_review" {
		t.Fatalf("expected task to remain active after rejected complete, got %+v", updated)
	}
}

func TestBrokerTaskCompleteAcceptsStructuredHandoffAndStoresMetadata(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	if err := b.StartOnPort(0); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer b.Stop()

	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Implement the strict broker slice",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	base := fmt.Sprintf("http://%s", b.Addr())
	body, _ := json.Marshal(map[string]any{
		"action":     "complete",
		"channel":    "general",
		"id":         task.ID,
		"created_by": "builder",
		"details": structuredTaskHandoff(
			"review_ready",
			"Implemented the broker path and ran the focused task tests.",
			"Review the new strict contract path before approving.",
			"",
			"",
		),
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/tasks", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+b.Token())
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("task complete request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected complete success, got %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Task teamTask `json:"task"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode task response: %v", err)
	}
	if result.Task.Status != "review" || result.Task.ReviewState != "ready_for_review" {
		t.Fatalf("expected review-ready task, got %+v", result.Task)
	}
	if result.Task.HandoffStatus != "accepted" || result.Task.HandoffAcceptedAt == "" {
		t.Fatalf("expected accepted handoff metadata, got %+v", result.Task)
	}
	if result.Task.LastHandoff == nil || result.Task.LastHandoff.Summary == "" {
		t.Fatalf("expected stored handoff summary, got %+v", result.Task.LastHandoff)
	}
}

func TestBrokerTaskBlockWithStructuredBlockerCreatesLinkedRequestAndUnblocksOnAnswer(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()
	useStubTaskWorktreeProvisioning(t)

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	if err := b.StartOnPort(0); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer b.Stop()

	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Implement the blocker bridge",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}

	base := fmt.Sprintf("http://%s", b.Addr())
	blockBody, _ := json.Marshal(map[string]any{
		"action":     "block",
		"channel":    "general",
		"id":         task.ID,
		"created_by": "builder",
		"details": structuredTaskHandoff(
			"blocked",
			"Execution stopped because the live environment target is still undefined.",
			"Resume the same task after the human answers the environment question.",
			`## Blockers
Kind: environment
Question: Qual e o workspace correto para este slice?
Waiting On: human
Need: O caminho exato do repo para continuar.
Context: A task foi planejada sem um destino de workspace utilizavel.
`,
			"",
		),
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/tasks", bytes.NewReader(blockBody))
	req.Header.Set("Authorization", "Bearer "+b.Token())
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("task block request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected block success, got %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Task teamTask `json:"task"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode task response: %v", err)
	}
	if !result.Task.Blocked || result.Task.Status != "blocked" {
		t.Fatalf("expected blocked task, got %+v", result.Task)
	}
	if len(result.Task.BlockerRequestIDs) != 1 {
		t.Fatalf("expected one linked blocker request, got %+v", result.Task.BlockerRequestIDs)
	}

	req, _ = http.NewRequest(http.MethodGet, base+"/requests?channel=general", nil)
	req.Header.Set("Authorization", "Bearer "+b.Token())
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list requests failed: %v", err)
	}
	defer resp.Body.Close()
	var requests struct {
		Requests []humanInterview `json:"requests"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&requests); err != nil {
		t.Fatalf("decode requests: %v", err)
	}
	if len(requests.Requests) != 1 {
		t.Fatalf("expected one blocker request, got %+v", requests.Requests)
	}
	if requests.Requests[0].SourceTaskID != task.ID {
		t.Fatalf("expected blocker request linked to %s, got %+v", task.ID, requests.Requests[0])
	}

	answerBody, _ := json.Marshal(map[string]any{
		"id":          requests.Requests[0].ID,
		"choice_text": "Use <DUNDERIA_REPO>",
	})
	req, _ = http.NewRequest(http.MethodPost, base+"/requests/answer", bytes.NewReader(answerBody))
	req.Header.Set("Authorization", "Bearer "+b.Token())
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("answer request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected request answer success, got %d: %s", resp.StatusCode, raw)
	}

	var updated teamTask
	for _, candidate := range b.AllTasks() {
		if candidate.ID == task.ID {
			updated = candidate
			break
		}
	}
	if updated.Blocked || updated.Status != "in_progress" {
		t.Fatalf("expected answered blocker request to unblock task, got %+v", updated)
	}
}

func TestBrokerApproveKeepsReviewOpenWhenBlockingFindingsAreReported(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "reviewer", "Reviewer")
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Validate the strict review gate",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	for i := range b.tasks {
		if b.tasks[i].ID != task.ID {
			continue
		}
		b.tasks[i].Status = "review"
		b.tasks[i].ReviewState = "ready_for_review"
		break
	}
	if err := b.StartOnPort(0); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer b.Stop()

	base := fmt.Sprintf("http://%s", b.Addr())
	body, _ := json.Marshal(map[string]any{
		"action":     "approve",
		"channel":    "general",
		"id":         task.ID,
		"created_by": "reviewer",
		"details": structuredTaskHandoff(
			"review",
			"Reviewed the slice and found a blocking gap in the request linkage.",
			"Builder should wire the request-task linkage and resubmit for review.",
			"",
			`## Review Findings
Severity: major
Location: internal/team/broker.go
Description: Missing explicit linkage between blocker requests and source task state.
Guidance: Persist source_task_id on the request and use it when unblocking the task.
`,
		),
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/tasks", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+b.Token())
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("approve task request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected review gate response, got %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Task teamTask `json:"task"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode task response: %v", err)
	}
	if result.Task.Status != "review" || result.Task.ReviewState != "changes_requested" {
		t.Fatalf("expected blocking findings to keep task in review, got %+v", result.Task)
	}
	if len(result.Task.ReviewFindings) != 1 || result.Task.ReviewFindings[0].Severity != "major" {
		t.Fatalf("expected stored blocking finding, got %+v", result.Task.ReviewFindings)
	}
	if len(result.Task.ReviewFindingHistory) != 1 || len(result.Task.ReviewFindingHistory[0].Findings) != 1 {
		t.Fatalf("expected stored review finding history, got %+v", result.Task.ReviewFindingHistory)
	}
}

func TestBrokerTaskReviewResubmissionResolvesOpenFindings(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	ensureTestMemberAccess(b, "general", "reviewer", "Reviewer")
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Resolve review findings before approval",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	for i := range b.tasks {
		if b.tasks[i].ID != task.ID {
			continue
		}
		b.tasks[i].Status = "review"
		b.tasks[i].ReviewState = "ready_for_review"
		break
	}
	if err := b.StartOnPort(0); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer b.Stop()

	base := fmt.Sprintf("http://%s", b.Addr())
	reportFindingBody, _ := json.Marshal(map[string]any{
		"action":     "approve",
		"channel":    "general",
		"id":         task.ID,
		"created_by": "reviewer",
		"details": structuredTaskHandoff(
			"review",
			"Found a blocking issue in the broker path.",
			"Builder should address the broker linkage and resubmit for review.",
			"",
			`## Review Findings
Severity: major
Location: internal/team/broker.go
Description: Missing linkage update after request creation.
Guidance: Persist and reuse the blocker linkage before approval.
`,
		),
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/tasks", bytes.NewReader(reportFindingBody))
	req.Header.Set("Authorization", "Bearer "+b.Token())
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("approve task request failed: %v", err)
	}
	resp.Body.Close()

	resubmitBody, _ := json.Marshal(map[string]any{
		"action":     "review",
		"channel":    "general",
		"id":         task.ID,
		"created_by": "builder",
		"details": structuredTaskHandoff(
			"review_ready",
			"Applied the broker linkage fix and re-ran the focused tests.",
			"Review the updated broker linkage before approval.",
			"",
			"",
		),
	})
	req, _ = http.NewRequest(http.MethodPost, base+"/tasks", bytes.NewReader(resubmitBody))
	req.Header.Set("Authorization", "Bearer "+b.Token())
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("review resubmission failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected review resubmission success, got %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Task teamTask `json:"task"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode review response: %v", err)
	}
	if result.Task.Status != "review" || result.Task.ReviewState != "ready_for_review" {
		t.Fatalf("expected task ready for review after resubmission, got %+v", result.Task)
	}
	if len(result.Task.ReviewFindings) != 1 {
		t.Fatalf("expected persisted finding history entry, got %+v", result.Task.ReviewFindings)
	}
	finding := result.Task.ReviewFindings[0]
	if finding.Status != "resolved" || finding.ResolvedBy != "builder" || finding.ResolvedAt == "" {
		t.Fatalf("expected open finding to be resolved on resubmission, got %+v", finding)
	}
	if len(result.Task.ReviewFindingHistory) != 1 {
		t.Fatalf("expected original finding batch to remain in history, got %+v", result.Task.ReviewFindingHistory)
	}
}

func TestBrokerApproveWithNewDemandFindingCreatesDerivedFollowUpTask(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	oldAsync := scheduleTaskGitHubAsync
	scheduleTaskGitHubAsync = func(fn func()) {}
	defer func() { scheduleTaskGitHubAsync = oldAsync }()

	repo, _ := initGitHubPublicationRepo(t)

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	ensureTestMemberAccess(b, "general", "reviewer", "Reviewer")
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Review the repo-backed slice",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "external_workspace",
		WorkspacePath: repo,
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	for i := range b.tasks {
		if b.tasks[i].ID != task.ID {
			continue
		}
		b.tasks[i].Status = "review"
		b.tasks[i].ReviewState = "ready_for_review"
		break
	}
	if err := b.StartOnPort(0); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer b.Stop()

	base := fmt.Sprintf("http://%s", b.Addr())
	body, _ := json.Marshal(map[string]any{
		"action":     "approve",
		"channel":    "general",
		"id":         task.ID,
		"created_by": "reviewer",
		"details": structuredTaskHandoff(
			"review",
			"Found a separate follow-up demand that should not block the current merge forever.",
			"Builder should ship the current slice and track the extra demand separately.",
			"",
			`## Review Findings
Severity: major
Location: internal/team/github_publication.go
Description: Add a separate audit export for publication events.
Guidance: Track this as follow-up work in a dedicated task.
New Demand: yes
`,
		),
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/tasks", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+b.Token())
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("approve task request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected review gate response, got %d: %s", resp.StatusCode, raw)
	}

	var derived teamTask
	for _, candidate := range b.AllTasks() {
		if candidate.ID == task.ID {
			continue
		}
		if candidate.DerivedFrom == nil || candidate.DerivedFrom.SourceTaskID != task.ID {
			continue
		}
		derived = candidate
		break
	}
	if derived.ID == "" {
		t.Fatalf("expected derived follow-up task, got %+v", b.AllTasks())
	}
	if derived.PublicationPolicy == nil || derived.PublicationPolicy.IssueMode != "immediate" {
		t.Fatalf("expected immediate issue policy on derived task, got %+v", derived.PublicationPolicy)
	}
	if derived.IssuePublication == nil || derived.IssuePublication.Status != "pending" {
		t.Fatalf("expected derived task to queue issue publication immediately, got %+v", derived.IssuePublication)
	}
	if derived.DerivedFrom.SourceKind != "review_finding" {
		t.Fatalf("expected derived task source kind review_finding, got %+v", derived.DerivedFrom)
	}
}

func TestBrokerApproveDedupesRepeatedNewDemandFindingsIntoSingleFollowUpTask(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	oldAsync := scheduleTaskGitHubAsync
	scheduleTaskGitHubAsync = func(fn func()) {}
	defer func() { scheduleTaskGitHubAsync = oldAsync }()

	repo, _ := initGitHubPublicationRepo(t)

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	ensureTestMemberAccess(b, "general", "reviewer", "Reviewer")
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Review duplicate demand markers",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "external_workspace",
		WorkspacePath: repo,
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	for i := range b.tasks {
		if b.tasks[i].ID != task.ID {
			continue
		}
		b.tasks[i].Status = "review"
		b.tasks[i].ReviewState = "ready_for_review"
		break
	}
	if err := b.StartOnPort(0); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer b.Stop()

	base := fmt.Sprintf("http://%s", b.Addr())
	body, _ := json.Marshal(map[string]any{
		"action":     "approve",
		"channel":    "general",
		"id":         task.ID,
		"created_by": "reviewer",
		"details": structuredTaskHandoff(
			"review",
			"Found the same follow-up demand twice in review.",
			"Keep only one derived task for the repeated demand.",
			"",
			`## Review Findings
Severity: major
Location: internal/team/github_publication.go
Description: Export GitHub publication counters for the session snapshot.
Guidance: Track this as follow-up work in a dedicated task.
New Demand: yes

Severity: major
Location: internal/team/session_observability.go
Description: Export GitHub publication counters for the session snapshot.
Guidance: Track this as follow-up work in a dedicated task.
New Demand: yes
`,
		),
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/tasks", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+b.Token())
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("approve task request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected review gate response, got %d: %s", resp.StatusCode, raw)
	}

	derivedCount := 0
	for _, candidate := range b.AllTasks() {
		if candidate.DerivedFrom != nil && candidate.DerivedFrom.SourceTaskID == task.ID && candidate.TaskType == "follow_up" {
			derivedCount++
		}
	}
	if derivedCount != 1 {
		t.Fatalf("expected a single deduped follow-up task, got %d", derivedCount)
	}
}

func TestBrokerApproveRejectsTaskOutsideReviewFlow(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	ensureTestMemberAccess(b, "general", "reviewer", "Reviewer")
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Reject premature approval",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	if err := b.StartOnPort(0); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer b.Stop()

	base := fmt.Sprintf("http://%s", b.Addr())
	body, _ := json.Marshal(map[string]any{
		"action":     "approve",
		"channel":    "general",
		"id":         task.ID,
		"created_by": "reviewer",
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/tasks", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+b.Token())
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("approve task request failed: %v", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected conflict for premature approval, got %d: %s", resp.StatusCode, raw)
	}
	if !strings.Contains(strings.ToLower(string(raw)), "approve") {
		t.Fatalf("expected approval transition guidance, got %s", raw)
	}

	var updated teamTask
	for _, candidate := range b.AllTasks() {
		if candidate.ID == task.ID {
			updated = candidate
			break
		}
	}
	if updated.Status != "in_progress" || updated.ReviewState != "pending_review" {
		t.Fatalf("expected task to remain in progress pending review, got %+v", updated)
	}
}

func TestBrokerReconcileRejectsTaskWithoutPendingReconciliation(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Reject stray reconcile",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	if err := b.StartOnPort(0); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer b.Stop()

	base := fmt.Sprintf("http://%s", b.Addr())
	body, _ := json.Marshal(map[string]any{
		"action":     "reconcile",
		"channel":    "general",
		"id":         task.ID,
		"created_by": "builder",
		"details": structuredTaskHandoff(
			"reconcile",
			"Attempted to reconcile without any pending workspace drift.",
			"No-op reconcile should be rejected.",
			"",
			"",
		),
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/tasks", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+b.Token())
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("reconcile task request failed: %v", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected conflict for reconcile without pending state, got %d: %s", resp.StatusCode, raw)
	}
	if !strings.Contains(strings.ToLower(string(raw)), "reconcile") {
		t.Fatalf("expected reconcile transition guidance, got %s", raw)
	}

	var updated teamTask
	for _, candidate := range b.AllTasks() {
		if candidate.ID == task.ID {
			updated = candidate
			break
		}
	}
	if updated.Status != "in_progress" || updated.Reconciliation != nil {
		t.Fatalf("expected task to remain active without reconciliation state, got %+v", updated)
	}
}

func TestBrokerReleaseNormalizesReviewStateBackToOpenFlow(t *testing.T) {
	oldPathFn := brokerStatePath
	tmpDir := isolateBrokerPersistenceEnv(t)
	brokerStatePath = func() string { return filepath.Join(tmpDir, "broker-state.json") }
	defer func() { brokerStatePath = oldPathFn }()

	b := NewBroker()
	ensureTestMemberAccess(b, "general", "builder", "Builder")
	task, reused, err := b.EnsurePlannedTask(plannedTaskInput{
		Channel:       "general",
		Title:         "Normalize release state",
		Owner:         "builder",
		CreatedBy:     "ceo",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	})
	if err != nil || reused {
		t.Fatalf("ensure planned task: %v reused=%v", err, reused)
	}
	for i := range b.tasks {
		if b.tasks[i].ID != task.ID {
			continue
		}
		b.tasks[i].Status = "review"
		b.tasks[i].ReviewState = "changes_requested"
		break
	}
	if err := b.StartOnPort(0); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer b.Stop()

	base := fmt.Sprintf("http://%s", b.Addr())
	body, _ := json.Marshal(map[string]any{
		"action":     "release",
		"channel":    "general",
		"id":         task.ID,
		"created_by": "builder",
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/tasks", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+b.Token())
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("release task request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected release success, got %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Task teamTask `json:"task"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode release response: %v", err)
	}
	if result.Task.Status != "open" || result.Task.Owner != "" || result.Task.Blocked {
		t.Fatalf("expected released task to return to open/unowned flow, got %+v", result.Task)
	}
	if result.Task.ReviewState != "" {
		t.Fatalf("expected release to clear review state, got %+v", result.Task)
	}
}

func TestResolveTaskTransitionRejectsApproveOutsideReviewFlow(t *testing.T) {
	task := &teamTask{
		ID:            "task-42",
		Owner:         "builder",
		Status:        "in_progress",
		ReviewState:   "pending_review",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	}

	_, err := resolveTaskTransition(task, "approve", nil)
	if err == nil {
		t.Fatalf("expected approve outside review flow to fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "approve") {
		t.Fatalf("expected approval guidance, got %v", err)
	}
}

func TestResolveTaskTransitionReleaseClearsOpenFlowFields(t *testing.T) {
	task := &teamTask{
		ID:            "task-77",
		Owner:         "builder",
		Status:        "review",
		ReviewState:   "changes_requested",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	}

	result, err := resolveTaskTransition(task, "release", nil)
	if err != nil {
		t.Fatalf("resolve release transition: %v", err)
	}
	if result.Status != "open" || result.ReviewState != "" || !result.ClearOwner || result.Blocked {
		t.Fatalf("unexpected release transition: %+v", result)
	}
}

func TestResolveTaskTransitionReconcileRequiresPendingState(t *testing.T) {
	task := &teamTask{
		ID:            "task-88",
		Owner:         "builder",
		Status:        "blocked",
		ReviewState:   "pending_review",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
	}

	_, err := resolveTaskTransition(task, "reconcile", nil)
	if err == nil {
		t.Fatalf("expected reconcile without pending state to fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "reconcile") {
		t.Fatalf("expected reconcile guidance, got %v", err)
	}
}

func structuredTaskHandoff(status, summary, downstream, blockers, findings string) string {
	var b strings.Builder
	b.WriteString("## Task Report\n")
	b.WriteString("Status: " + status + "\n")
	b.WriteString("Summary: " + summary + "\n")
	b.WriteString("Touched: internal/team/broker.go\n")
	b.WriteString("Validation: go test ./internal/team -run FocusedContractTest\n")
	b.WriteString("\n")
	if strings.TrimSpace(blockers) != "" {
		b.WriteString(strings.TrimSpace(blockers))
		b.WriteString("\n\n")
	}
	if strings.TrimSpace(findings) != "" {
		b.WriteString(strings.TrimSpace(findings))
		b.WriteString("\n\n")
	}
	b.WriteString("## Downstream Context\n")
	b.WriteString(downstream)
	return b.String()
}
