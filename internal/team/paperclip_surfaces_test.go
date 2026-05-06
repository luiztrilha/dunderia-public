package team

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPaperclipPhase10WorkQueuesPrioritizeBlockingWork(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{
		{
			ID:        "task-1",
			Channel:   "general",
			Title:     "Normal work",
			Status:    "in_progress",
			TaskType:  "follow_up",
			CreatedAt: "2026-04-30T10:00:00Z",
			UpdatedAt: "2026-04-30T10:00:00Z",
		},
		{
			ID:        "task-2",
			Channel:   "general",
			Title:     "Blocked work",
			Status:    "blocked",
			Blocked:   true,
			TaskType:  "bugfix",
			CreatedAt: "2026-04-30T10:01:00Z",
			UpdatedAt: "2026-04-30T10:01:00Z",
		},
	}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/work-queues?channel=general&viewer_slug=human", nil)
	b.handleWorkQueues(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload workQueueSnapshot
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Queues) == 0 || payload.Queues[0].Key != "blocked" || payload.Queues[0].High != 1 {
		t.Fatalf("expected blocked high queue first, got %+v", payload.Queues)
	}
	if len(payload.Next) == 0 || payload.Next[0].TaskID != "task-2" {
		t.Fatalf("expected blocked task first, got %+v", payload.Next)
	}
}

func TestPaperclipPhase10KnowledgeIndexIncludesTaskEvidenceAndLearning(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{{
		ID:              "task-knowledge",
		Channel:         "general",
		Title:           "Ship reusable workflow",
		Status:          "done",
		TaskType:        "feature",
		Outcome:         "Reusable workflow shipped",
		OutcomeEvidence: "go test ./internal/team",
		CreatedAt:       "2026-04-30T10:00:00Z",
		UpdatedAt:       "2026-04-30T10:01:00Z",
	}}
	b.skills = []teamSkill{{
		ID:          "skill-learned",
		Name:        "learned-task-knowledge",
		Title:       "Reusable workflow playbook",
		Description: "Captured from a completed task",
		PluginID:    "dunderia-learning",
		Channel:     "general",
		Tags:        []string{"learning", "playbook"},
		Status:      "active",
		CreatedAt:   "2026-04-30T10:02:00Z",
		UpdatedAt:   "2026-04-30T10:02:00Z",
	}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/knowledge?channel=general&viewer_slug=human&q=workflow", nil)
	b.handleKnowledgeIndex(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload knowledgeIndexResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Entries) != 2 {
		t.Fatalf("expected task and learning entries, got %+v", payload.Entries)
	}
}

func TestKnowledgeWikiPreviewProjectsScopedReadOnlyArticles(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{
		{
			ID:              "task-knowledge",
			Channel:         "general",
			Title:           "Ship reusable workflow",
			Status:          "done",
			TaskType:        "feature",
			Outcome:         "Reusable workflow shipped",
			OutcomeEvidence: "go test ./internal/team passed",
			CreatedAt:       "2026-04-30T10:00:00Z",
			UpdatedAt:       "2026-04-30T10:01:00Z",
		},
		{
			ID:              "task-private",
			Channel:         "private",
			Title:           "Private workflow",
			Status:          "done",
			OutcomeEvidence: "hidden evidence",
			CreatedAt:       "2026-04-30T10:00:00Z",
			UpdatedAt:       "2026-04-30T10:01:00Z",
		},
	}
	beforeTasks := len(b.tasks)
	beforeSkills := len(b.skills)
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/knowledge/wiki-preview?channel=general&viewer_slug=human&q=workflow", nil)
	b.handleKnowledgeWikiPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload knowledgeWikiPreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Persisted || len(payload.Articles) != 1 {
		t.Fatalf("expected one read-only article, got persisted=%v articles=%+v", payload.Persisted, payload.Articles)
	}
	article := payload.Articles[0]
	if article.Slug == "" || !strings.Contains(article.Markdown, "## Source") || !strings.Contains(article.Markdown, "task-knowledge") {
		t.Fatalf("expected sourced markdown article, got %+v", article)
	}
	if article.Channel != "general" || article.Sources[0].ID != "task:task-knowledge" {
		t.Fatalf("unexpected article scope/source: %+v", article)
	}
	b.mu.RLock()
	afterTasks := len(b.tasks)
	afterSkills := len(b.skills)
	b.mu.RUnlock()
	if afterTasks != beforeTasks || afterSkills != beforeSkills {
		t.Fatalf("wiki preview mutated state: tasks %d -> %d, skills %d -> %d", beforeTasks, afterTasks, beforeSkills, afterSkills)
	}
}

func TestKnowledgeWikiLintReportsRiskAndBrokenBacklinkReadOnly(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{{
		ID:              "task-secret",
		Channel:         "general",
		Title:           "Capture risky knowledge",
		Status:          "done",
		Outcome:         "Reusable credential workflow",
		OutcomeEvidence: "Never paste API_TOKEN into the release note.",
		CreatedAt:       "2026-01-01T10:00:00Z",
		UpdatedAt:       "2026-01-01T10:01:00Z",
	}}
	beforeTasks := len(b.tasks)
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/knowledge/wiki-lint?channel=general&viewer_slug=human&q=credential", nil)
	b.handleKnowledgeWikiLint(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload knowledgeWikiLintResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Persisted || payload.Status != "error" || len(payload.Findings) == 0 {
		t.Fatalf("expected read-only lint errors, got %+v", payload)
	}
	seen := map[string]bool{}
	for _, finding := range payload.Findings {
		seen[finding.Kind] = true
	}
	if !seen["secret_like_content"] || !seen["stale_source"] {
		t.Fatalf("expected secret and stale findings, got %+v", payload.Findings)
	}
	b.mu.RLock()
	afterTasks := len(b.tasks)
	b.mu.RUnlock()
	if afterTasks != beforeTasks {
		t.Fatalf("wiki lint mutated tasks: %d -> %d", beforeTasks, afterTasks)
	}

	articles := []knowledgeWikiArticle{{
		ID:        "wiki:manual",
		Slug:      "manual",
		Title:     "Manual backlink",
		Kind:      "task",
		Channel:   "general",
		Sources:   []knowledgeWikiSource{{ID: "task:missing", Kind: "task", TaskID: "missing"}},
		Backlinks: []string{"missing"},
	}}
	findings := buildKnowledgeWikiLintFindings(articles, map[string]struct{}{"other": {}})
	var broken bool
	for _, finding := range findings {
		if finding.Kind == "broken_backlink" {
			broken = true
		}
	}
	if !broken {
		t.Fatalf("expected broken backlink finding, got %+v", findings)
	}
}

func TestKnowledgeWikiPromotionPreviewBuildsReviewedDiffReadOnly(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{
		{
			ID:              "task-wiki",
			Channel:         "general",
			Title:           "Publish reusable wiki article",
			Status:          "done",
			TaskType:        "docs",
			Outcome:         "Reusable wiki article ready",
			OutcomeEvidence: "Reviewed source evidence exists.",
			CreatedAt:       "2026-05-05T10:00:00Z",
			UpdatedAt:       "2026-05-05T10:01:00Z",
		},
		{
			ID:              "task-private-wiki",
			Channel:         "private",
			Title:           "Private wiki article",
			Status:          "done",
			OutcomeEvidence: "hidden evidence",
			CreatedAt:       "2026-05-05T10:00:00Z",
			UpdatedAt:       "2026-05-05T10:01:00Z",
		},
	}
	beforeTasks := len(b.tasks)
	beforeSkills := len(b.skills)
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/knowledge/wiki-promotion-preview?channel=general&viewer_slug=human&task_id=task-wiki", nil)
	b.handleKnowledgeWikiPromotionPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload knowledgeWikiPromotionPreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Persisted || payload.Status != "review" || len(payload.Proposals) != 1 {
		t.Fatalf("expected one read-only review proposal, got %+v", payload)
	}
	proposal := payload.Proposals[0]
	if proposal.SourceID != "task:task-wiki" || proposal.TargetPath == "" || !strings.HasPrefix(proposal.Diff, "diff --git") {
		t.Fatalf("expected sourced git-style diff proposal, got %+v", proposal)
	}
	if !proposal.ReviewedCommitOnly || !stringSliceContains(proposal.RiskSignals, "shared_memory_not_mutated") {
		t.Fatalf("expected reviewed commit boundary without shared memory mutation, got %+v", proposal)
	}
	if strings.Contains(proposal.Diff, "task-private-wiki") {
		t.Fatalf("promotion preview leaked private task diff: %s", proposal.Diff)
	}
	b.mu.RLock()
	afterTasks := len(b.tasks)
	afterSkills := len(b.skills)
	b.mu.RUnlock()
	if afterTasks != beforeTasks || afterSkills != beforeSkills {
		t.Fatalf("wiki promotion preview mutated state: tasks %d -> %d, skills %d -> %d", beforeTasks, afterTasks, beforeSkills, afterSkills)
	}
}

func TestKnowledgeWikiPromotionPreviewBlocksSecretLikeArticle(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{{
		ID:              "task-secret-wiki",
		Channel:         "general",
		Title:           "Document credential handling",
		Status:          "done",
		Outcome:         "Credential article",
		OutcomeEvidence: "Never paste API_TOKEN into the wiki.",
		CreatedAt:       "2026-05-05T10:00:00Z",
		UpdatedAt:       "2026-05-05T10:01:00Z",
	}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/knowledge/wiki-promotion-preview?channel=general&viewer_slug=human&task_id=task-secret-wiki", nil)
	b.handleKnowledgeWikiPromotionPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload knowledgeWikiPromotionPreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Status != "blocked" || len(payload.Proposals) != 1 {
		t.Fatalf("expected blocked promotion proposal, got %+v", payload)
	}
	if !stringSliceContains(payload.Proposals[0].RiskSignals, "lint_error") || len(payload.Proposals[0].LintFindings) == 0 {
		t.Fatalf("expected lint error risk signals, got %+v", payload.Proposals[0])
	}
}

func TestHermesLearningCandidatesPreviewIsScopedAndReadOnly(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{
		{
			ID:              "task-learn",
			Channel:         "general",
			Title:           "Stabilize release workflow",
			Status:          "done",
			Owner:           "eng",
			TaskType:        "bugfix",
			Outcome:         "Release workflow stabilized",
			OutcomeEvidence: "wuphf release-check passed after fixing stale web dist detection",
			Artifacts: []taskArtifact{{
				ID:        "artifact-release",
				Kind:      "log",
				Title:     "Release check output",
				Summary:   "release-check passed",
				CreatedAt: "2026-05-04T10:02:00Z",
			}},
			PlanRevisions: []taskPlanRevision{{
				ID:        "plan-1",
				Version:   1,
				Summary:   "Use the release gate before publishing",
				Status:    "approved",
				CreatedAt: "2026-05-04T10:01:00Z",
			}},
			Feedback: []taskFeedback{{
				ID:        "feedback-1",
				Rating:    "up",
				Comment:   "This should become a reusable release playbook",
				CreatedAt: "2026-05-04T10:03:00Z",
			}},
			CreatedAt: "2026-05-04T10:00:00Z",
			UpdatedAt: "2026-05-04T10:04:00Z",
		},
		{
			ID:              "task-private",
			Channel:         "private",
			Title:           "Private reusable workflow",
			Status:          "done",
			OutcomeEvidence: "hidden evidence",
			CreatedAt:       "2026-05-04T10:00:00Z",
			UpdatedAt:       "2026-05-04T10:04:00Z",
		},
	}
	beforeTasks := len(b.tasks)
	beforeSkills := len(b.skills)
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/learning/candidates?channel=general&viewer_slug=human&q=release", nil)
	b.handleLearningCandidates(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload learningCandidatesResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Candidates) != 1 {
		t.Fatalf("expected one scoped learning candidate, got %+v", payload.Candidates)
	}
	candidate := payload.Candidates[0]
	if candidate.TaskID != "task-learn" || candidate.SkillName != "learned-task-learn" || candidate.Promoted {
		t.Fatalf("unexpected candidate identity/promoted state: %+v", candidate)
	}
	if !stringSliceContains(candidate.Signals, "outcome_evidence") || !stringSliceContains(candidate.Signals, "artifacts") || !stringSliceContains(candidate.Signals, "plan_revision") || !stringSliceContains(candidate.Signals, "feedback") {
		t.Fatalf("expected evidence/artifact/plan/feedback signals, got %+v", candidate.Signals)
	}
	if len(candidate.Provenance) < 3 {
		t.Fatalf("expected provenance from evidence, plan, and artifact, got %+v", candidate.Provenance)
	}

	b.mu.RLock()
	afterTasks := len(b.tasks)
	afterSkills := len(b.skills)
	b.mu.RUnlock()
	if afterTasks != beforeTasks || afterSkills != beforeSkills {
		t.Fatalf("learning candidate preview mutated state: tasks %d -> %d, skills %d -> %d", beforeTasks, afterTasks, beforeSkills, afterSkills)
	}
}

func TestHermesLearningCandidateDiffPreviewIsReadOnly(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{{
		ID:              "task-learn",
		Channel:         "general",
		Title:           "Stabilize release workflow",
		Status:          "done",
		Owner:           "eng",
		Outcome:         "Release workflow stabilized",
		OutcomeEvidence: "release-check passed after fixing stale web dist detection",
		Artifacts: []taskArtifact{{
			ID:        "artifact-release",
			Kind:      "log",
			Title:     "Release check output",
			Summary:   "release-check passed",
			CreatedAt: "2026-05-04T10:02:00Z",
		}},
		PlanRevisions: []taskPlanRevision{{
			ID:        "plan-1",
			Version:   1,
			Summary:   "Use the release gate before publishing",
			Content:   "Run release-check before publishing desktop packages.",
			Status:    "approved",
			CreatedAt: "2026-05-04T10:01:00Z",
		}},
		CreatedAt: "2026-05-04T10:00:00Z",
		UpdatedAt: "2026-05-04T10:04:00Z",
	}}
	beforeSkills := len(b.skills)
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/learning/candidates/diff-preview?task_id=task-learn&channel=general&viewer_slug=human&include_content=true", nil)
	b.handleLearningCandidateDiffPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload learningCandidateDiffResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Persisted || payload.Action != "create" || payload.Duplicate {
		t.Fatalf("expected read-only create preview, got persisted=%v action=%q duplicate=%v", payload.Persisted, payload.Action, payload.Duplicate)
	}
	if payload.ProposedSkill.Name != "learned-task-learn" || payload.ProposedSkill.SourceType != "task_learning" {
		t.Fatalf("unexpected proposed skill: %+v", payload.ProposedSkill)
	}
	if payload.Candidate.TaskID != "task-learn" || payload.Candidate.SkillName != "learned-task-learn" {
		t.Fatalf("unexpected candidate identity: %+v", payload.Candidate)
	}
	var sawContent bool
	for _, file := range payload.Files {
		if file.Name == "content.md" {
			sawContent = true
			if file.Status != "create" || !strings.Contains(file.After, "release-check passed") {
				t.Fatalf("unexpected content diff file: %+v", file)
			}
		}
	}
	if !sawContent {
		t.Fatalf("expected content.md diff file, got %+v", payload.Files)
	}
	b.mu.RLock()
	afterSkills := len(b.skills)
	b.mu.RUnlock()
	if afterSkills != beforeSkills {
		t.Fatalf("diff preview mutated skills: %d -> %d", beforeSkills, afterSkills)
	}
}

func TestHermesLearningCandidateDiffPreviewDetectsDuplicateSkill(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{{
		ID:              "task-learn",
		Channel:         "general",
		Title:           "Stabilize release workflow",
		Status:          "done",
		Outcome:         "Release workflow stabilized",
		OutcomeEvidence: "new release evidence",
		CreatedAt:       "2026-05-04T10:00:00Z",
		UpdatedAt:       "2026-05-04T10:04:00Z",
	}}
	b.skills = []teamSkill{{
		ID:         "skill-learned-task-learn",
		Name:       "learned-task-learn",
		Title:      "Old release workflow",
		Content:    "old instructions",
		SourceType: "task_learning",
		SourceRef:  "task-learn",
		CreatedBy:  "human",
		Channel:    "general",
		Status:     "active",
		CreatedAt:  "2026-05-04T09:00:00Z",
		UpdatedAt:  "2026-05-04T09:00:00Z",
	}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/learning/candidates/diff-preview?task_id=task-learn&channel=general&viewer_slug=human&include_content=true", nil)
	b.handleLearningCandidateDiffPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload learningCandidateDiffResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Duplicate || payload.Action != "already_promoted" || payload.RiskLevel != "medium" {
		t.Fatalf("expected duplicate medium-risk preview, got duplicate=%v action=%q risk=%q", payload.Duplicate, payload.Action, payload.RiskLevel)
	}
	if !stringSliceContains(payload.RiskSignals, "duplicate_skill_name") {
		t.Fatalf("expected duplicate risk signal, got %+v", payload.RiskSignals)
	}
	var sawDifferent bool
	for _, file := range payload.Files {
		if file.Name == "content.md" && file.Status == "different_existing" {
			sawDifferent = true
		}
	}
	if !sawDifferent {
		t.Fatalf("expected content.md to differ from existing skill, got %+v", payload.Files)
	}
}

func TestHermesMemoryCurationPreviewIsScopedAndReadOnly(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.messages = []channelMessage{
		{
			ID:        "msg-release",
			From:      "human",
			Channel:   "general",
			Content:   "Remember that release checks must run before publishing the desktop package.",
			Timestamp: "2026-05-04T10:00:00Z",
		},
		{
			ID:        "msg-private",
			From:      "human",
			Channel:   "private",
			Content:   "Remember this private release process too.",
			Timestamp: "2026-05-04T10:01:00Z",
		},
	}
	b.tasks = []teamTask{{
		ID:              "task-release",
		Channel:         "general",
		Title:           "Stabilize release workflow",
		Status:          "done",
		Owner:           "eng",
		Outcome:         "Release workflow stabilized",
		OutcomeEvidence: "release-check passed after validating package freshness",
		CreatedAt:       "2026-05-04T09:00:00Z",
		UpdatedAt:       "2026-05-04T10:02:00Z",
	}}
	beforeMessages := len(b.messages)
	beforeMemory := len(b.sharedMemory)
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/memory/curation-preview?channel=general&viewer_slug=human&q=release", nil)
	b.handleMemoryCurationPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload memoryCurationPreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Persisted || len(payload.Candidates) < 2 {
		t.Fatalf("expected read-only memory candidates, got %+v", payload)
	}
	for _, candidate := range payload.Candidates {
		if candidate.Channel != "general" {
			t.Fatalf("expected scoped general candidates only, got %+v", payload.Candidates)
		}
		if candidate.ProposedAction != "remember" {
			t.Fatalf("expected remember candidates by default, got %+v", candidate)
		}
	}

	b.mu.RLock()
	afterMessages := len(b.messages)
	afterMemory := len(b.sharedMemory)
	b.mu.RUnlock()
	if afterMessages != beforeMessages || afterMemory != beforeMemory {
		t.Fatalf("memory curation preview mutated state: messages %d -> %d, memory namespaces %d -> %d", beforeMessages, afterMessages, beforeMemory, afterMemory)
	}
}

func TestHermesMemoryCurationPreviewFlagsSecretLikeContentForDiscard(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.messages = []channelMessage{{
		ID:        "msg-secret",
		From:      "human",
		Channel:   "general",
		Content:   "Remember that API_TOKEN is stored in plaintext config for this local test.",
		Timestamp: "2026-05-04T10:00:00Z",
	}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/memory/curation-preview?channel=general&viewer_slug=human&include_discard=true", nil)
	b.handleMemoryCurationPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload memoryCurationPreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Candidates) != 1 {
		t.Fatalf("expected one discard candidate, got %+v", payload)
	}
	candidate := payload.Candidates[0]
	if candidate.ProposedAction != "discard" || candidate.RiskLevel != "high" || !stringSliceContains(candidate.RiskSignals, "secret_like_content") {
		t.Fatalf("expected high-risk discard candidate, got %+v", candidate)
	}
}

func TestPaperclipPhase10DeepPlanningPreviewDoesNotPersistTasks(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	before := len(b.tasks)
	b.mu.Unlock()

	body := bytes.NewBufferString(`{"channel":"general","created_by":"human","goal":"Absorver melhorias do Paperclip","outcome":"Plano validado"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/planning/deep", body)
	b.handleDeepPlanning(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload deepPlanningPreview
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Persisted || !payload.RequiresApproval || len(payload.Milestones) != 3 {
		t.Fatalf("unexpected preview payload: %+v", payload)
	}
	b.mu.RLock()
	after := len(b.tasks)
	b.mu.RUnlock()
	if after != before {
		t.Fatalf("deep planning preview persisted tasks: before=%d after=%d", before, after)
	}
}

func TestPaperclipPhase10ReviewChecklistBlocksOnFindings(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	ensureTestMemberAccess(b, "general", "human", "Human")
	b.tasks = []teamTask{{
		ID:        "task-review",
		Channel:   "general",
		Title:     "Review me",
		Status:    "review",
		TaskType:  "feature",
		CreatedAt: "2026-04-30T10:00:00Z",
		UpdatedAt: "2026-04-30T10:00:00Z",
		ReviewFindings: []taskReviewFinding{{
			ID:          "finding-1",
			Severity:    "major",
			Location:    "web/src/App.tsx",
			Description: "Regression risk",
			Guidance:    "Fix before approval",
			Status:      "open",
		}},
	}}
	b.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/review/checklist?task_id=task-review&viewer_slug=human", nil)
	b.handleReviewChecklist(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload reviewChecklistResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Status != "blocked" || payload.BlockingCount == 0 {
		t.Fatalf("expected blocked checklist, got %+v", payload)
	}
	foundFindingBlock := false
	for _, item := range payload.Items {
		if item.ID == "review_findings" && item.Blocking {
			foundFindingBlock = true
		}
	}
	if !foundFindingBlock {
		t.Fatalf("expected review_findings to block, got %+v", payload.Items)
	}
}

func TestPaperclipPhase10TemplatePreviewDoesNotMutateTopology(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	beforeMembers := len(b.members)
	beforeChannels := len(b.channels)
	b.mu.Unlock()

	body := bytes.NewBufferString(`{"title":"Starter kit","agents":["paperclip-agent"],"channels":["paperclip-channel"],"skills":["paperclip-skill"]}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/templates/preview", body)
	b.handleTemplatePreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload templatePreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Persisted || !payload.RequiresTopologyAuthorization || len(payload.BlockedMutations) < 2 {
		t.Fatalf("unexpected preview payload: %+v", payload)
	}
	if !strings.Contains(strings.Join(payload.BlockedMutations, ","), "agent:paperclip-agent") {
		t.Fatalf("expected agent mutation to be blocked, got %+v", payload.BlockedMutations)
	}
	b.mu.RLock()
	afterMembers := len(b.members)
	afterChannels := len(b.channels)
	b.mu.RUnlock()
	if afterMembers != beforeMembers || afterChannels != beforeChannels {
		t.Fatalf("template preview mutated topology: members %d->%d channels %d->%d", beforeMembers, afterMembers, beforeChannels, afterChannels)
	}
}
