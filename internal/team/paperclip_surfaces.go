package team

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type workQueueSnapshot struct {
	GeneratedAt string           `json:"generated_at"`
	Queues      []workQueueGroup `json:"queues"`
	Next        []workQueueItem  `json:"next,omitempty"`
}

type workQueueGroup struct {
	Key      string         `json:"key"`
	Label    string         `json:"label"`
	Reason   string         `json:"reason,omitempty"`
	Count    int            `json:"count"`
	High     int            `json:"high"`
	Medium   int            `json:"medium"`
	Owners   []string       `json:"owners,omitempty"`
	Channels []string       `json:"channels,omitempty"`
	Next     *workQueueItem `json:"next,omitempty"`
}

type workQueueItem struct {
	TaskID    string `json:"task_id"`
	Title     string `json:"title"`
	QueueKey  string `json:"queue_key,omitempty"`
	Channel   string `json:"channel,omitempty"`
	Owner     string `json:"owner,omitempty"`
	Status    string `json:"status,omitempty"`
	Priority  string `json:"priority,omitempty"`
	Reason    string `json:"reason,omitempty"`
	SLAAt     string `json:"sla_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func (b *Broker) handleWorkQueues(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	allChannels := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all_channels")), "true")
	if channel == "" && !allChannels {
		channel = "general"
	}

	b.mu.RLock()
	if !allChannels && !b.canAccessChannelLocked(viewer, channel) {
		b.mu.RUnlock()
		http.Error(w, "channel access denied", http.StatusForbidden)
		return
	}
	tasks := make([]teamTask, 0, len(b.tasks))
	for _, task := range b.tasks {
		taskChannel := normalizeChannelSlug(task.Channel)
		if !allChannels && taskChannel != channel {
			continue
		}
		if allChannels && !b.canAccessChannelLocked(viewer, taskChannel) {
			continue
		}
		if strings.TrimSpace(task.ArchivedAt) != "" {
			continue
		}
		tasks = append(tasks, task)
	}
	b.mu.RUnlock()

	payload := buildWorkQueueSnapshot(tasks, currentTaskGoalContext(), time.Now().UTC())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func buildWorkQueueSnapshot(tasks []teamTask, goalCtx taskGoalContext, now time.Time) workQueueSnapshot {
	groups := map[string]*workQueueGroup{}
	allItems := make([]workQueueItem, 0, len(tasks))
	for i := range tasks {
		task := tasks[i]
		applyTaskOperatorContract(&task, goalCtx)
		status := normalizeTaskStatus(task.Status)
		if status == "done" || status == "canceled" || status == "failed" {
			continue
		}
		key := normalizeTaskQueueKey(task.QueueKey)
		if key == "" {
			key = "active"
		}
		group := groups[key]
		if group == nil {
			group = &workQueueGroup{Key: key, Label: taskQueueLabel(key)}
			groups[key] = group
		}
		item := workQueueItem{
			TaskID:    strings.TrimSpace(task.ID),
			Title:     strings.TrimSpace(task.Title),
			QueueKey:  key,
			Channel:   normalizeChannelSlug(task.Channel),
			Owner:     strings.TrimSpace(task.Owner),
			Status:    status,
			Priority:  strings.TrimSpace(firstNonEmpty(task.QueuePriority, "normal")),
			Reason:    strings.TrimSpace(task.QueueReason),
			SLAAt:     strings.TrimSpace(task.QueueSLAAt),
			UpdatedAt: strings.TrimSpace(firstNonEmpty(task.UpdatedAt, task.CreatedAt)),
		}
		group.Count++
		if item.Priority == "high" {
			group.High++
		}
		if item.Priority == "medium" {
			group.Medium++
		}
		group.Reason = firstNonEmpty(group.Reason, item.Reason)
		group.Owners = appendUnique(group.Owners, item.Owner)
		group.Channels = appendUnique(group.Channels, item.Channel)
		if group.Next == nil || workQueueItemLess(item, *group.Next, now) {
			copyItem := item
			group.Next = &copyItem
		}
		allItems = append(allItems, item)
	}

	out := workQueueSnapshot{GeneratedAt: now.Format(time.RFC3339)}
	for _, group := range groups {
		out.Queues = append(out.Queues, *group)
	}
	sort.Slice(out.Queues, func(i, j int) bool {
		if out.Queues[i].High != out.Queues[j].High {
			return out.Queues[i].High > out.Queues[j].High
		}
		if out.Queues[i].Count != out.Queues[j].Count {
			return out.Queues[i].Count > out.Queues[j].Count
		}
		return out.Queues[i].Key < out.Queues[j].Key
	})
	sort.Slice(allItems, func(i, j int) bool { return workQueueItemLess(allItems[i], allItems[j], now) })
	if len(allItems) > 8 {
		allItems = allItems[:8]
	}
	out.Next = allItems
	return out
}

func workQueueItemLess(left, right workQueueItem, now time.Time) bool {
	leftScore := workQueuePriorityScore(left.Priority)
	rightScore := workQueuePriorityScore(right.Priority)
	if leftScore != rightScore {
		return leftScore > rightScore
	}
	leftSLA := parseBrokerTimestamp(left.SLAAt)
	rightSLA := parseBrokerTimestamp(right.SLAAt)
	if !leftSLA.IsZero() || !rightSLA.IsZero() {
		if leftSLA.IsZero() {
			return false
		}
		if rightSLA.IsZero() {
			return true
		}
		leftOverdue := !leftSLA.After(now)
		rightOverdue := !rightSLA.After(now)
		if leftOverdue != rightOverdue {
			return leftOverdue
		}
		return leftSLA.Before(rightSLA)
	}
	if left.UpdatedAt != right.UpdatedAt {
		return studioTimestampAfter(left.UpdatedAt, right.UpdatedAt)
	}
	return left.TaskID < right.TaskID
}

func workQueuePriorityScore(priority string) int {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case "high":
		return 3
	case "medium":
		return 2
	default:
		return 1
	}
}

type knowledgeIndexResponse struct {
	GeneratedAt string           `json:"generated_at"`
	Entries     []knowledgeEntry `json:"entries"`
}

type knowledgeEntry struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Title     string   `json:"title"`
	Summary   string   `json:"summary,omitempty"`
	Channel   string   `json:"channel,omitempty"`
	TaskID    string   `json:"task_id,omitempty"`
	Source    string   `json:"source,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

type knowledgeWikiPreviewResponse struct {
	GeneratedAt string                 `json:"generated_at"`
	Persisted   bool                   `json:"persisted"`
	Articles    []knowledgeWikiArticle `json:"articles"`
	Summary     map[string]int         `json:"summary"`
}

type knowledgeWikiArticle struct {
	ID          string                `json:"id"`
	Slug        string                `json:"slug"`
	Title       string                `json:"title"`
	Kind        string                `json:"kind"`
	Channel     string                `json:"channel,omitempty"`
	Summary     string                `json:"summary,omitempty"`
	Markdown    string                `json:"markdown,omitempty"`
	Sources     []knowledgeWikiSource `json:"sources,omitempty"`
	Backlinks   []string              `json:"backlinks,omitempty"`
	Stale       bool                  `json:"stale"`
	RiskSignals []string              `json:"risk_signals,omitempty"`
	UpdatedAt   string                `json:"updated_at,omitempty"`
}

type knowledgeWikiSource struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	TaskID  string `json:"task_id,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type knowledgeWikiLintResponse struct {
	GeneratedAt string                     `json:"generated_at"`
	Persisted   bool                       `json:"persisted"`
	Status      string                     `json:"status"`
	Summary     map[string]int             `json:"summary"`
	Findings    []knowledgeWikiLintFinding `json:"findings"`
}

type knowledgeWikiLintFinding struct {
	ID       string `json:"id"`
	Article  string `json:"article"`
	Slug     string `json:"slug,omitempty"`
	Severity string `json:"severity"`
	Kind     string `json:"kind"`
	Summary  string `json:"summary"`
	SourceID string `json:"source_id,omitempty"`
	Channel  string `json:"channel,omitempty"`
}

type knowledgeWikiPromotionPreviewResponse struct {
	GeneratedAt string                           `json:"generated_at"`
	Persisted   bool                             `json:"persisted"`
	Status      string                           `json:"status"`
	Summary     map[string]int                   `json:"summary"`
	Proposals   []knowledgeWikiPromotionProposal `json:"proposals"`
}

type knowledgeWikiPromotionProposal struct {
	ID                 string                     `json:"id"`
	ArticleID          string                     `json:"article_id"`
	Slug               string                     `json:"slug"`
	Title              string                     `json:"title"`
	Channel            string                     `json:"channel,omitempty"`
	SourceID           string                     `json:"source_id,omitempty"`
	SourceKind         string                     `json:"source_kind,omitempty"`
	TargetPath         string                     `json:"target_path"`
	Action             string                     `json:"action"`
	CommitMessage      string                     `json:"commit_message"`
	Markdown           string                     `json:"markdown,omitempty"`
	Diff               string                     `json:"diff,omitempty"`
	ReviewedCommitOnly bool                       `json:"reviewed_commit_only"`
	RequiredReviews    []string                   `json:"required_reviews,omitempty"`
	RiskSignals        []string                   `json:"risk_signals,omitempty"`
	LintFindings       []knowledgeWikiLintFinding `json:"lint_findings,omitempty"`
	NextStep           string                     `json:"next_step,omitempty"`
}

func (b *Broker) handleKnowledgeIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind")))
	limit := parsePositiveLimit(r.URL.Query().Get("limit"), 25)
	allChannels := channel == "" || strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all_channels")), "true")

	b.mu.RLock()
	entries := b.buildKnowledgeIndexLocked(viewer, channel, allChannels)
	b.mu.RUnlock()
	filtered := entries[:0]
	for _, entry := range entries {
		if kind != "" && !strings.EqualFold(entry.Kind, kind) {
			continue
		}
		if query != "" && !knowledgeEntryMatches(entry, query) {
			continue
		}
		filtered = append(filtered, entry)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].UpdatedAt != filtered[j].UpdatedAt {
			return studioTimestampAfter(filtered[i].UpdatedAt, filtered[j].UpdatedAt)
		}
		return filtered[i].ID < filtered[j].ID
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(knowledgeIndexResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Entries:     filtered,
	})
}

func (b *Broker) handleKnowledgeWikiPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind")))
	limit := parsePositiveLimit(r.URL.Query().Get("limit"), 12)
	allChannels := channel == "" || strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all_channels")), "true")

	b.mu.RLock()
	entries := b.buildKnowledgeIndexLocked(viewer, channel, allChannels)
	b.mu.RUnlock()
	filtered := entries[:0]
	for _, entry := range entries {
		if kind != "" && !strings.EqualFold(entry.Kind, kind) {
			continue
		}
		if query != "" && !knowledgeEntryMatches(entry, query) {
			continue
		}
		filtered = append(filtered, entry)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].UpdatedAt != filtered[j].UpdatedAt {
			return studioTimestampAfter(filtered[i].UpdatedAt, filtered[j].UpdatedAt)
		}
		return filtered[i].ID < filtered[j].ID
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	articles := buildKnowledgeWikiArticles(filtered, time.Now().UTC())
	summary := map[string]int{"total": len(articles)}
	for _, article := range articles {
		summary["kind_"+article.Kind]++
		if article.Stale {
			summary["stale"]++
		}
		for _, signal := range article.RiskSignals {
			summary["risk_"+signal]++
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(knowledgeWikiPreviewResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Persisted:   false,
		Articles:    articles,
		Summary:     summary,
	})
}

func (b *Broker) handleKnowledgeWikiPromotionPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind")))
	taskID := strings.TrimSpace(r.URL.Query().Get("task_id"))
	limit := parsePositiveLimit(r.URL.Query().Get("limit"), 5)
	allChannels := channel == "" || strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all_channels")), "true")

	b.mu.RLock()
	entries := b.buildKnowledgeIndexLocked(viewer, channel, allChannels)
	taskIDs := make(map[string]struct{}, len(b.tasks))
	for _, task := range b.tasks {
		taskIDs[strings.TrimSpace(task.ID)] = struct{}{}
	}
	b.mu.RUnlock()
	filtered := entries[:0]
	for _, entry := range entries {
		if taskID != "" && strings.TrimSpace(entry.TaskID) != taskID {
			continue
		}
		if kind != "" && !strings.EqualFold(entry.Kind, kind) {
			continue
		}
		if query != "" && !knowledgeEntryMatches(entry, query) {
			continue
		}
		filtered = append(filtered, entry)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].UpdatedAt != filtered[j].UpdatedAt {
			return studioTimestampAfter(filtered[i].UpdatedAt, filtered[j].UpdatedAt)
		}
		return filtered[i].ID < filtered[j].ID
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	articles := buildKnowledgeWikiArticles(filtered, time.Now().UTC())
	findings := buildKnowledgeWikiLintFindings(articles, taskIDs)
	findingsByArticle := map[string][]knowledgeWikiLintFinding{}
	for _, finding := range findings {
		findingsByArticle[finding.Article] = append(findingsByArticle[finding.Article], finding)
	}
	proposals := make([]knowledgeWikiPromotionProposal, 0, len(articles))
	for i, article := range articles {
		proposals = append(proposals, buildKnowledgeWikiPromotionProposal(article, filtered[i], findingsByArticle[article.ID]))
	}
	status := "ok"
	summary := map[string]int{"total": len(proposals)}
	for _, proposal := range proposals {
		summary["action_"+proposal.Action]++
		if len(proposal.LintFindings) > 0 {
			summary["with_lint_findings"]++
		}
		for _, signal := range proposal.RiskSignals {
			summary["risk_"+signal]++
		}
		for _, finding := range proposal.LintFindings {
			summary["lint_"+finding.Severity]++
			if finding.Severity == "error" {
				status = "blocked"
			} else if status == "ok" && finding.Severity == "warning" {
				status = "review"
			}
		}
		if status == "ok" {
			status = "review"
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(knowledgeWikiPromotionPreviewResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Persisted:   false,
		Status:      status,
		Summary:     summary,
		Proposals:   proposals,
	})
}

func (b *Broker) handleKnowledgeWikiLint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind")))
	limit := parsePositiveLimit(r.URL.Query().Get("limit"), 50)
	allChannels := channel == "" || strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all_channels")), "true")

	b.mu.RLock()
	entries := b.buildKnowledgeIndexLocked(viewer, channel, allChannels)
	taskIDs := make(map[string]struct{}, len(b.tasks))
	for _, task := range b.tasks {
		taskIDs[strings.TrimSpace(task.ID)] = struct{}{}
	}
	b.mu.RUnlock()
	filtered := entries[:0]
	for _, entry := range entries {
		if kind != "" && !strings.EqualFold(entry.Kind, kind) {
			continue
		}
		if query != "" && !knowledgeEntryMatches(entry, query) {
			continue
		}
		filtered = append(filtered, entry)
	}
	articles := buildKnowledgeWikiArticles(filtered, time.Now().UTC())
	findings := buildKnowledgeWikiLintFindings(articles, taskIDs)
	if len(findings) > limit {
		findings = findings[:limit]
	}
	summary := map[string]int{"total": len(findings)}
	status := "ok"
	for _, finding := range findings {
		summary["kind_"+finding.Kind]++
		summary["severity_"+finding.Severity]++
		if finding.Severity == "error" {
			status = "error"
		} else if status == "ok" && finding.Severity == "warning" {
			status = "warning"
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(knowledgeWikiLintResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Persisted:   false,
		Status:      status,
		Summary:     summary,
		Findings:    findings,
	})
}

func buildKnowledgeWikiPromotionProposal(article knowledgeWikiArticle, entry knowledgeEntry, findings []knowledgeWikiLintFinding) knowledgeWikiPromotionProposal {
	targetPath := "wiki/" + article.Slug + ".md"
	source := knowledgeWikiSource{}
	if len(article.Sources) > 0 {
		source = article.Sources[0]
	}
	signals := compactStringList(append([]string{"reviewed_commit_required", "shared_memory_not_mutated"}, article.RiskSignals...))
	for _, finding := range findings {
		signals = appendUnique(signals, "lint_"+finding.Severity)
	}
	action := "propose_create"
	proposal := knowledgeWikiPromotionProposal{
		ID:                 "wiki-promotion:" + skillSlug(article.ID),
		ArticleID:          article.ID,
		Slug:               article.Slug,
		Title:              article.Title,
		Channel:            article.Channel,
		SourceID:           source.ID,
		SourceKind:         firstNonEmpty(source.Kind, entry.Source),
		TargetPath:         targetPath,
		Action:             action,
		CommitMessage:      "docs: promote " + article.Slug + " to team wiki",
		Markdown:           article.Markdown,
		Diff:               renderKnowledgeWikiPromotionDiff(targetPath, article.Markdown),
		ReviewedCommitOnly: true,
		RequiredReviews:    []string{"operator", "knowledge-owner"},
		RiskSignals:        compactStringList(signals),
		LintFindings:       findings,
		NextStep:           "Review lint findings and apply this markdown as a normal git commit only after operator approval.",
	}
	return proposal
}

func renderKnowledgeWikiPromotionDiff(targetPath, markdown string) string {
	lines := strings.Split(strings.TrimSpace(markdown), "\n")
	var sb strings.Builder
	sb.WriteString("diff --git a/" + targetPath + " b/" + targetPath + "\n")
	sb.WriteString("new file mode 100644\n")
	sb.WriteString("index 0000000..0000000\n")
	sb.WriteString("--- /dev/null\n")
	sb.WriteString("+++ b/" + targetPath + "\n")
	sb.WriteString(fmt.Sprintf("@@ -0,0 +1,%d @@\n", len(lines)))
	for _, line := range lines {
		sb.WriteString("+" + line + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (b *Broker) buildKnowledgeIndexLocked(viewer, channel string, allChannels bool) []knowledgeEntry {
	var entries []knowledgeEntry
	for _, task := range b.tasks {
		taskChannel := normalizeChannelSlug(task.Channel)
		if !allChannels && taskChannel != channel {
			continue
		}
		if !b.canAccessChannelLocked(viewer, taskChannel) {
			continue
		}
		copyTask := task
		applyTaskOperatorContract(&copyTask, currentTaskGoalContext())
		if copyTask.LearningCandidate == nil && strings.TrimSpace(copyTask.OutcomeEvidence) == "" && len(copyTask.Artifacts) == 0 && latestTaskPlanRevision(&copyTask) == nil {
			continue
		}
		entries = append(entries, knowledgeEntry{
			ID:        "task:" + strings.TrimSpace(copyTask.ID),
			Kind:      "task",
			Title:     truncateSummary(firstNonEmpty(copyTask.Outcome, copyTask.Title), 140),
			Summary:   truncateSummary(firstNonEmpty(copyTask.OutcomeEvidence, copyTask.Details, copyTask.LatestPlanSummary), 220),
			Channel:   taskChannel,
			TaskID:    strings.TrimSpace(copyTask.ID),
			Source:    "task",
			Tags:      compactStringList([]string{copyTask.TaskType, copyTask.QueueKey, copyTask.OutcomeStatus}),
			UpdatedAt: firstNonEmpty(copyTask.OutcomeVerifiedAt, copyTask.UpdatedAt, copyTask.CreatedAt),
		})
	}
	for _, skill := range b.skills {
		skillChannel := normalizeChannelSlug(skill.Channel)
		if skillChannel == "" {
			skillChannel = "general"
		}
		if !allChannels && skillChannel != channel {
			continue
		}
		if !b.canAccessChannelLocked(viewer, skillChannel) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(skill.PluginID), "dunderia-learning") && !containsString(skill.Tags, "learning") {
			continue
		}
		entries = append(entries, knowledgeEntry{
			ID:        "skill:" + strings.TrimSpace(skill.ID),
			Kind:      "learning",
			Title:     truncateSummary(firstNonEmpty(skill.Title, skill.Name), 140),
			Summary:   truncateSummary(firstNonEmpty(skill.Description, skill.Content), 220),
			Channel:   skillChannel,
			Source:    "skill",
			Tags:      compactStringList(skill.Tags),
			UpdatedAt: firstNonEmpty(skill.UpdatedAt, skill.CreatedAt),
		})
	}
	return entries
}

func buildKnowledgeWikiArticles(entries []knowledgeEntry, now time.Time) []knowledgeWikiArticle {
	articles := make([]knowledgeWikiArticle, 0, len(entries))
	for _, entry := range entries {
		article := knowledgeWikiArticle{
			ID:        "wiki:" + strings.TrimSpace(entry.ID),
			Slug:      "knowledge-" + skillSlug(strings.ReplaceAll(strings.TrimSpace(entry.ID), ":", "-")),
			Title:     firstNonEmpty(strings.TrimSpace(entry.Title), "Untitled knowledge"),
			Kind:      strings.TrimSpace(entry.Kind),
			Channel:   normalizeChannelSlug(entry.Channel),
			Summary:   strings.TrimSpace(entry.Summary),
			Sources:   []knowledgeWikiSource{{ID: strings.TrimSpace(entry.ID), Kind: strings.TrimSpace(entry.Source), TaskID: strings.TrimSpace(entry.TaskID), Summary: truncateSummary(entry.Summary, 180)}},
			Backlinks: compactStringList([]string{entry.TaskID}),
			UpdatedAt: strings.TrimSpace(entry.UpdatedAt),
		}
		if article.Kind == "" {
			article.Kind = "knowledge"
		}
		if article.Channel == "" {
			article.Channel = "general"
		}
		if contentLooksSecretBearing(article.Title + " " + article.Summary + " " + strings.Join(entry.Tags, " ")) {
			article.RiskSignals = appendUnique(article.RiskSignals, "secret_like_content")
		}
		if updated := parseBrokerTimestamp(article.UpdatedAt); !updated.IsZero() && now.Sub(updated) > 90*24*time.Hour {
			article.Stale = true
			article.RiskSignals = appendUnique(article.RiskSignals, "stale_source")
		}
		article.Markdown = renderKnowledgeWikiMarkdown(article, entry)
		article.RiskSignals = compactStringList(article.RiskSignals)
		articles = append(articles, article)
	}
	return articles
}

func renderKnowledgeWikiMarkdown(article knowledgeWikiArticle, entry knowledgeEntry) string {
	var sb strings.Builder
	sb.WriteString("# " + article.Title + "\n\n")
	if article.Summary != "" {
		sb.WriteString(article.Summary + "\n\n")
	}
	sb.WriteString("## Source\n")
	sb.WriteString("- " + strings.TrimSpace(entry.Source) + ": `" + strings.TrimSpace(entry.ID) + "`")
	if entry.TaskID != "" {
		sb.WriteString(" (task `" + strings.TrimSpace(entry.TaskID) + "`)")
	}
	sb.WriteString("\n")
	if len(entry.Tags) > 0 {
		sb.WriteString("\n## Tags\n")
		for _, tag := range compactStringList(entry.Tags) {
			sb.WriteString("- " + tag + "\n")
		}
	}
	if article.Stale || len(article.RiskSignals) > 0 {
		sb.WriteString("\n## Review Signals\n")
		if article.Stale {
			sb.WriteString("- stale_source\n")
		}
		for _, signal := range article.RiskSignals {
			if signal == "stale_source" && article.Stale {
				continue
			}
			sb.WriteString("- " + signal + "\n")
		}
	}
	return strings.TrimSpace(sb.String())
}

func buildKnowledgeWikiLintFindings(articles []knowledgeWikiArticle, taskIDs map[string]struct{}) []knowledgeWikiLintFinding {
	findings := make([]knowledgeWikiLintFinding, 0)
	add := func(article knowledgeWikiArticle, severity, kind, summary, sourceID string) {
		findings = append(findings, knowledgeWikiLintFinding{
			ID:       "wiki-lint:" + kind + ":" + skillSlug(article.ID+":"+sourceID),
			Article:  article.ID,
			Slug:     article.Slug,
			Severity: severity,
			Kind:     kind,
			Summary:  summary,
			SourceID: sourceID,
			Channel:  article.Channel,
		})
	}
	for _, article := range articles {
		if len(article.Sources) == 0 {
			add(article, "error", "missing_source", "Article has no source reference.", "")
		}
		if article.Stale {
			add(article, "warning", "stale_source", "Article source is older than the freshness threshold.", article.UpdatedAt)
		}
		if stringSliceContains(article.RiskSignals, "secret_like_content") || contentLooksSecretBearing(article.Markdown) {
			add(article, "error", "secret_like_content", "Article content or metadata looks like it may contain secret-bearing text.", article.ID)
		}
		for _, source := range article.Sources {
			if strings.TrimSpace(source.ID) == "" || strings.TrimSpace(source.Kind) == "" {
				add(article, "error", "incomplete_source", "Article source is missing an id or kind.", source.ID)
			}
			taskID := strings.TrimSpace(source.TaskID)
			if taskID == "" {
				continue
			}
			if _, ok := taskIDs[taskID]; !ok {
				add(article, "warning", "broken_backlink", "Article points to a task backlink that is not present in the current task index.", taskID)
			}
		}
		for _, backlink := range article.Backlinks {
			backlink = strings.TrimSpace(backlink)
			if backlink == "" {
				continue
			}
			if _, ok := taskIDs[backlink]; !ok {
				add(article, "warning", "broken_backlink", "Article backlink is not present in the current task index.", backlink)
			}
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		leftRank := knowledgeWikiLintSeverityRank(findings[i].Severity)
		rightRank := knowledgeWikiLintSeverityRank(findings[j].Severity)
		if leftRank != rightRank {
			return leftRank > rightRank
		}
		if findings[i].Kind != findings[j].Kind {
			return findings[i].Kind < findings[j].Kind
		}
		return findings[i].ID < findings[j].ID
	})
	return findings
}

func knowledgeWikiLintSeverityRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "error":
		return 3
	case "warning":
		return 2
	default:
		return 1
	}
}

func knowledgeEntryMatches(entry knowledgeEntry, query string) bool {
	haystack := strings.ToLower(strings.Join(append([]string{
		entry.ID,
		entry.Kind,
		entry.Title,
		entry.Summary,
		entry.Channel,
		entry.TaskID,
		entry.Source,
	}, entry.Tags...), " "))
	return strings.Contains(haystack, query)
}

type learningCandidatesResponse struct {
	GeneratedAt string                     `json:"generated_at"`
	Candidates  []learningCandidatePreview `json:"candidates"`
}

type learningCandidateDiffResponse struct {
	GeneratedAt   string                      `json:"generated_at"`
	Persisted     bool                        `json:"persisted"`
	TaskID        string                      `json:"task_id"`
	Channel       string                      `json:"channel"`
	Action        string                      `json:"action"`
	Duplicate     bool                        `json:"duplicate"`
	Candidate     learningCandidatePreview    `json:"candidate"`
	ProposedSkill teamSkill                   `json:"proposed_skill"`
	ExistingSkill *teamSkill                  `json:"existing_skill,omitempty"`
	Files         []learningCandidateDiffFile `json:"files"`
	RiskLevel     string                      `json:"risk_level"`
	RiskSignals   []string                    `json:"risk_signals,omitempty"`
	Summary       map[string]int              `json:"summary"`
}

type learningCandidateDiffFile struct {
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Status      string   `json:"status"`
	Summary     string   `json:"summary,omitempty"`
	BeforeSize  int      `json:"before_size,omitempty"`
	AfterSize   int      `json:"after_size,omitempty"`
	Before      string   `json:"before,omitempty"`
	After       string   `json:"after,omitempty"`
	RiskSignals []string `json:"risk_signals,omitempty"`
}

type learningCandidatePreview struct {
	ID              string                        `json:"id"`
	TaskID          string                        `json:"task_id"`
	Channel         string                        `json:"channel,omitempty"`
	Owner           string                        `json:"owner,omitempty"`
	Kind            string                        `json:"kind,omitempty"`
	Title           string                        `json:"title"`
	Summary         string                        `json:"summary,omitempty"`
	SkillName       string                        `json:"skill_name,omitempty"`
	Reason          string                        `json:"reason,omitempty"`
	Signals         []string                      `json:"signals,omitempty"`
	Provenance      []learningCandidateProvenance `json:"provenance,omitempty"`
	Promoted        bool                          `json:"promoted"`
	PromotedSkillID string                        `json:"promoted_skill_id,omitempty"`
	UpdatedAt       string                        `json:"updated_at,omitempty"`
}

type learningCandidateProvenance struct {
	Kind    string `json:"kind"`
	ID      string `json:"id,omitempty"`
	Summary string `json:"summary,omitempty"`
	When    string `json:"when,omitempty"`
}

type memoryCurationPreviewResponse struct {
	GeneratedAt string                    `json:"generated_at"`
	Persisted   bool                      `json:"persisted"`
	Summary     map[string]int            `json:"summary"`
	Candidates  []memoryCurationCandidate `json:"candidates"`
}

type memoryCurationCandidate struct {
	ID              string   `json:"id"`
	SourceType      string   `json:"source_type"`
	SourceID        string   `json:"source_id,omitempty"`
	Channel         string   `json:"channel,omitempty"`
	Actor           string   `json:"actor,omitempty"`
	Title           string   `json:"title,omitempty"`
	Summary         string   `json:"summary,omitempty"`
	Namespace       string   `json:"namespace"`
	Key             string   `json:"key,omitempty"`
	ProposedAction  string   `json:"proposed_action"`
	Confidence      string   `json:"confidence"`
	Score           int      `json:"score"`
	RiskLevel       string   `json:"risk_level"`
	RiskSignals     []string `json:"risk_signals,omitempty"`
	Signals         []string `json:"signals,omitempty"`
	AlreadyInMemory bool     `json:"already_in_memory"`
	CreatedAt       string   `json:"created_at,omitempty"`
}

func (b *Broker) handleMemoryCurationPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	action := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("action")))
	limit := parsePositiveLimit(r.URL.Query().Get("limit"), 25)
	includeDiscard := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_discard")), "true")
	allChannels := channel == "" || strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all_channels")), "true")

	b.mu.RLock()
	candidates := b.buildMemoryCurationPreviewLocked(viewer, channel, allChannels, includeDiscard)
	b.mu.RUnlock()

	filtered := candidates[:0]
	for _, candidate := range candidates {
		if action != "" && !strings.EqualFold(candidate.ProposedAction, action) {
			continue
		}
		if query != "" && !memoryCurationCandidateMatches(candidate, query) {
			continue
		}
		filtered = append(filtered, candidate)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].ProposedAction != filtered[j].ProposedAction {
			return memoryCurationActionRank(filtered[i].ProposedAction) < memoryCurationActionRank(filtered[j].ProposedAction)
		}
		if filtered[i].Score != filtered[j].Score {
			return filtered[i].Score > filtered[j].Score
		}
		if filtered[i].CreatedAt != filtered[j].CreatedAt {
			return studioTimestampAfter(filtered[i].CreatedAt, filtered[j].CreatedAt)
		}
		return filtered[i].ID < filtered[j].ID
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	summary := map[string]int{"total": len(filtered)}
	for _, candidate := range filtered {
		summary[candidate.ProposedAction]++
		if candidate.RiskLevel != "" {
			summary["risk_"+candidate.RiskLevel]++
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(memoryCurationPreviewResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Persisted:   false,
		Summary:     summary,
		Candidates:  filtered,
	})
}

func (b *Broker) buildMemoryCurationPreviewLocked(viewer, channel string, allChannels bool, includeDiscard bool) []memoryCurationCandidate {
	var candidates []memoryCurationCandidate
	add := func(candidate memoryCurationCandidate) {
		candidate.Channel = normalizeChannelSlug(candidate.Channel)
		if candidate.Channel == "" {
			candidate.Channel = "general"
		}
		if !allChannels && candidate.Channel != channel {
			return
		}
		if !b.canAccessChannelLocked(viewer, candidate.Channel) {
			return
		}
		if candidate.ProposedAction == "discard" && !includeDiscard {
			return
		}
		candidates = append(candidates, candidate)
	}
	for _, msg := range b.messages {
		add(b.memoryCurationCandidateFromMessageLocked(msg))
	}
	for _, task := range b.tasks {
		if candidate, ok := b.memoryCurationCandidateFromTaskLocked(task); ok {
			add(candidate)
		}
	}
	for _, decision := range b.decisions {
		if candidate, ok := b.memoryCurationCandidateFromDecisionLocked(decision); ok {
			add(candidate)
		}
	}
	return candidates
}

func (b *Broker) memoryCurationCandidateFromMessageLocked(msg channelMessage) memoryCurationCandidate {
	channel := normalizeChannelSlug(msg.Channel)
	if channel == "" {
		channel = "general"
	}
	key, note, ok := channelMemoryMessageNote(msg)
	if !ok {
		title := strings.TrimSpace(firstNonEmpty(msg.Title, "Message"))
		body := strings.TrimSpace(firstNonEmpty(msg.Content, msg.Title))
		return finalizeMemoryCurationCandidate(memoryCurationCandidate{
			ID:             "memory-preview:message:" + strings.TrimSpace(msg.ID),
			SourceType:     "message",
			SourceID:       strings.TrimSpace(msg.ID),
			Channel:        channel,
			Actor:          strings.TrimSpace(msg.From),
			Title:          title,
			Summary:        truncateSummary(body, 260),
			Namespace:      channelMemoryNamespace(channel),
			Key:            "msg:" + strings.TrimSpace(msg.ID),
			ProposedAction: "discard",
			Score:          5,
			Signals:        []string{"low_signal"},
			CreatedAt:      strings.TrimSpace(msg.Timestamp),
		})
	}
	candidate := memoryCurationCandidate{
		ID:         "memory-preview:message:" + strings.TrimSpace(msg.ID),
		SourceType: "message",
		SourceID:   strings.TrimSpace(msg.ID),
		Channel:    channel,
		Actor:      strings.TrimSpace(note.Author),
		Title:      strings.TrimSpace(note.Title),
		Summary:    truncateSummary(note.Content, 260),
		Namespace:  channelMemoryNamespace(channel),
		Key:        key,
		Score:      62,
		Signals:    []string{"substantive_message"},
		CreatedAt:  strings.TrimSpace(note.CreatedAt),
	}
	if len(msg.Tagged) > 0 {
		candidate.Score += 12
		candidate.Signals = append(candidate.Signals, "handoff")
	}
	return b.finalizeMemoryCurationCandidateLocked(candidate)
}

func (b *Broker) memoryCurationCandidateFromTaskLocked(task teamTask) (memoryCurationCandidate, bool) {
	channel := normalizeChannelSlug(task.Channel)
	if channel == "" {
		channel = "general"
	}
	summary := strings.TrimSpace(firstNonEmpty(task.OutcomeEvidence, task.Outcome, task.Details))
	if summary == "" {
		return memoryCurationCandidate{}, false
	}
	score := 50
	signals := []string{"task_context"}
	if strings.TrimSpace(task.OutcomeEvidence) != "" {
		score += 18
		signals = append(signals, "outcome_evidence")
	}
	if len(task.Artifacts) > 0 {
		score += 8
		signals = append(signals, "artifacts")
	}
	if len(task.Feedback) > 0 {
		score += 8
		signals = append(signals, "feedback")
	}
	if task.LearningCandidate != nil && task.LearningCandidate.Recommended {
		score += 10
		signals = append(signals, "learning_candidate")
	}
	candidate := memoryCurationCandidate{
		ID:         "memory-preview:task:" + strings.TrimSpace(task.ID),
		SourceType: "task",
		SourceID:   strings.TrimSpace(task.ID),
		Channel:    channel,
		Actor:      strings.TrimSpace(firstNonEmpty(task.Owner, task.CreatedBy)),
		Title:      truncateSummary(firstNonEmpty(task.Outcome, task.Title), 140),
		Summary:    truncateSummary(summary, 260),
		Namespace:  channelMemoryNamespace(channel),
		Key:        "task:" + strings.TrimSpace(task.ID) + ":outcome",
		Score:      score,
		Signals:    signals,
		CreatedAt:  firstNonEmpty(task.OutcomeVerifiedAt, task.UpdatedAt, task.CreatedAt),
	}
	return b.finalizeMemoryCurationCandidateLocked(candidate), true
}

func (b *Broker) memoryCurationCandidateFromDecisionLocked(decision officeDecisionRecord) (memoryCurationCandidate, bool) {
	key, note, ok := channelMemoryDecisionNote(decision)
	if !ok {
		return memoryCurationCandidate{}, false
	}
	channel := normalizeChannelSlug(decision.Channel)
	if channel == "" {
		channel = "general"
	}
	candidate := memoryCurationCandidate{
		ID:         "memory-preview:decision:" + strings.TrimSpace(decision.ID),
		SourceType: "decision",
		SourceID:   strings.TrimSpace(decision.ID),
		Channel:    channel,
		Actor:      strings.TrimSpace(firstNonEmpty(note.Author, decision.Owner)),
		Title:      strings.TrimSpace(note.Title),
		Summary:    truncateSummary(note.Content, 260),
		Namespace:  channelMemoryNamespace(channel),
		Key:        key,
		Score:      80,
		Signals:    []string{"decision"},
		CreatedAt:  strings.TrimSpace(note.CreatedAt),
	}
	return b.finalizeMemoryCurationCandidateLocked(candidate), true
}

func (b *Broker) finalizeMemoryCurationCandidateLocked(candidate memoryCurationCandidate) memoryCurationCandidate {
	if b != nil && b.sharedMemory != nil {
		if entries := b.sharedMemory[candidate.Namespace]; entries != nil {
			if _, ok := entries[candidate.Key]; ok && candidate.Key != "" {
				candidate.AlreadyInMemory = true
			}
		}
	}
	return finalizeMemoryCurationCandidate(candidate)
}

func finalizeMemoryCurationCandidate(candidate memoryCurationCandidate) memoryCurationCandidate {
	text := strings.TrimSpace(strings.Join([]string{candidate.Title, candidate.Summary}, "\n"))
	if contentLooksSecretBearing(text) {
		candidate.ProposedAction = "discard"
		candidate.RiskLevel = "high"
		candidate.RiskSignals = append(candidate.RiskSignals, "secret_like_content")
	} else if candidate.ProposedAction == "" {
		switch {
		case candidate.AlreadyInMemory:
			candidate.ProposedAction = "consolidate"
		case candidate.Score >= 60:
			candidate.ProposedAction = "remember"
		case candidate.Score >= 35:
			candidate.ProposedAction = "review"
		default:
			candidate.ProposedAction = "discard"
		}
	}
	if candidate.RiskLevel == "" {
		if candidate.Score < 35 {
			candidate.RiskLevel = "medium"
			candidate.RiskSignals = append(candidate.RiskSignals, "low_signal")
		} else {
			candidate.RiskLevel = "low"
		}
	}
	switch {
	case candidate.Score >= 75:
		candidate.Confidence = "high"
	case candidate.Score >= 45:
		candidate.Confidence = "medium"
	default:
		candidate.Confidence = "low"
	}
	candidate.Signals = compactStringList(candidate.Signals)
	candidate.RiskSignals = compactStringList(candidate.RiskSignals)
	return candidate
}

func memoryCurationActionRank(action string) int {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "remember":
		return 0
	case "consolidate":
		return 1
	case "review":
		return 2
	case "discard":
		return 3
	default:
		return 4
	}
}

func memoryCurationCandidateMatches(candidate memoryCurationCandidate, query string) bool {
	query = strings.TrimSpace(query)
	if query == "" {
		return true
	}
	haystack := normalizeMemorySearchText(strings.Join(append([]string{
		candidate.ID,
		candidate.SourceType,
		candidate.SourceID,
		candidate.Channel,
		candidate.Actor,
		candidate.Title,
		candidate.Summary,
		candidate.Namespace,
		candidate.Key,
		candidate.ProposedAction,
		candidate.Confidence,
		candidate.RiskLevel,
	}, append(candidate.Signals, candidate.RiskSignals...)...), "\n"))
	return privateMemoryMatchScore(haystack, query) > 0
}

func (b *Broker) handleLearningCandidateDiffPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	taskID := strings.TrimSpace(r.URL.Query().Get("task_id"))
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	selected := strings.TrimSpace(r.URL.Query().Get("file"))
	includeContent := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_content")), "true")
	if taskID == "" {
		http.Error(w, "task_id required", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	b.mu.RLock()
	task := b.findTaskByIDLocked(taskID)
	if task == nil {
		b.mu.RUnlock()
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	taskChannel := normalizeChannelSlug(task.Channel)
	if taskChannel == "" {
		taskChannel = "general"
	}
	if channel != "" && !skillVisibleInChannel(taskChannel, channel) {
		b.mu.RUnlock()
		http.Error(w, "task not visible in channel", http.StatusForbidden)
		return
	}
	if !b.canAccessChannelLocked(viewer, taskChannel) {
		b.mu.RUnlock()
		http.Error(w, "channel access denied", http.StatusForbidden)
		return
	}
	copyTask := *task
	applyTaskOperatorContract(&copyTask, currentTaskGoalContext())
	learningKind, learningTitle, learningSummary := "", "", ""
	if copyTask.LearningCandidate != nil {
		learningKind = copyTask.LearningCandidate.Kind
		learningTitle = copyTask.LearningCandidate.Title
		learningSummary = copyTask.LearningCandidate.Summary
	}
	actor := strings.TrimSpace(viewer)
	if actor == "" {
		actor = "human"
	}
	proposed, err := buildTaskLearningSkill(&copyTask, actor, learningKind, learningTitle, learningSummary, now)
	if err != nil {
		b.mu.RUnlock()
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	var existingSkill *teamSkill
	promotedSkillID := ""
	if existing := b.findSkillByNameLocked(proposed.Name); existing != nil {
		existingCopy := *existing
		existingSkill = &existingCopy
		promotedSkillID = strings.TrimSpace(existing.ID)
	}
	candidate := learningCandidatePreview{
		ID:              "learning:" + strings.TrimSpace(copyTask.ID),
		TaskID:          strings.TrimSpace(copyTask.ID),
		Channel:         taskChannel,
		Owner:           strings.TrimSpace(copyTask.Owner),
		Kind:            normalizeLearningKind(learningKind),
		Title:           truncateSummary(firstNonEmpty(learningTitle, copyTask.Outcome, copyTask.Title), 140),
		Summary:         truncateSummary(firstNonEmpty(learningSummary, copyTask.OutcomeEvidence, copyTask.Details), 260),
		SkillName:       proposed.Name,
		Reason:          "",
		Signals:         buildLearningCandidateSignals(copyTask),
		Provenance:      buildLearningCandidateProvenance(copyTask),
		Promoted:        promotedSkillID != "",
		PromotedSkillID: promotedSkillID,
		UpdatedAt:       firstNonEmpty(copyTask.OutcomeVerifiedAt, copyTask.UpdatedAt, copyTask.CreatedAt),
	}
	if copyTask.LearningCandidate != nil {
		candidate.Reason = strings.TrimSpace(copyTask.LearningCandidate.Reason)
	}
	b.mu.RUnlock()

	response := buildLearningCandidateDiffResponse(candidate, proposed, existingSkill, selected, includeContent)
	response.GeneratedAt = now
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (b *Broker) handleLearningCandidates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind")))
	limit := parsePositiveLimit(r.URL.Query().Get("limit"), 25)
	allChannels := channel == "" || strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all_channels")), "true")

	b.mu.RLock()
	candidates := b.buildLearningCandidatesLocked(viewer, channel, allChannels)
	b.mu.RUnlock()
	filtered := candidates[:0]
	for _, candidate := range candidates {
		if kind != "" && !strings.EqualFold(candidate.Kind, kind) {
			continue
		}
		if query != "" && !learningCandidateMatches(candidate, query) {
			continue
		}
		filtered = append(filtered, candidate)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Promoted != filtered[j].Promoted {
			return !filtered[i].Promoted
		}
		if filtered[i].UpdatedAt != filtered[j].UpdatedAt {
			return studioTimestampAfter(filtered[i].UpdatedAt, filtered[j].UpdatedAt)
		}
		return filtered[i].ID < filtered[j].ID
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(learningCandidatesResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Candidates:  filtered,
	})
}

func buildLearningCandidateDiffResponse(candidate learningCandidatePreview, proposed teamSkill, existing *teamSkill, selected string, includeContent bool) learningCandidateDiffResponse {
	action := "create"
	duplicate := false
	if existing != nil {
		duplicate = true
		action = "already_promoted"
	}
	files := buildLearningCandidateDiffFiles(proposed, existing, selected, includeContent)
	summary := map[string]int{"total": len(files)}
	riskSignals := make([]string, 0)
	for _, file := range files {
		summary["status_"+file.Status]++
		if file.Status != "unchanged" {
			summary["changed"]++
		}
		for _, signal := range file.RiskSignals {
			summary["risk_"+signal]++
			riskSignals = appendUnique(riskSignals, signal)
		}
	}
	if duplicate {
		riskSignals = appendUnique(riskSignals, "duplicate_skill_name")
	}
	if strings.EqualFold(proposed.ScanStatus, "warning") {
		riskSignals = appendUnique(riskSignals, "scan_warning")
	}
	riskLevel := "low"
	if stringSliceContains(riskSignals, "secret_like_content") || stringSliceContains(riskSignals, "scan_warning") {
		riskLevel = "high"
	} else if duplicate {
		riskLevel = "medium"
	}
	return learningCandidateDiffResponse{
		Persisted:     false,
		TaskID:        candidate.TaskID,
		Channel:       candidate.Channel,
		Action:        action,
		Duplicate:     duplicate,
		Candidate:     candidate,
		ProposedSkill: proposed,
		ExistingSkill: existing,
		Files:         files,
		RiskLevel:     riskLevel,
		RiskSignals:   compactStringList(riskSignals),
		Summary:       summary,
	}
}

type learningCandidateVirtualFile struct {
	name    string
	kind    string
	summary string
	content string
}

func buildLearningCandidateDiffFiles(proposed teamSkill, existing *teamSkill, selected string, includeContent bool) []learningCandidateDiffFile {
	afterFiles := learningCandidateVirtualFiles(proposed)
	beforeFiles := map[string]learningCandidateVirtualFile{}
	if existing != nil {
		for _, file := range learningCandidateVirtualFiles(*existing) {
			beforeFiles[file.name] = file
		}
	}
	selected = strings.TrimSpace(selected)
	out := make([]learningCandidateDiffFile, 0, len(afterFiles))
	for _, after := range afterFiles {
		if selected != "" && !strings.EqualFold(after.name, selected) {
			continue
		}
		before := beforeFiles[after.name]
		beforeContent := strings.TrimSpace(before.content)
		afterContent := strings.TrimSpace(after.content)
		if beforeContent == "" && afterContent == "" {
			continue
		}
		status := "create"
		if existing != nil {
			if beforeContent == afterContent {
				status = "unchanged"
			} else {
				status = "different_existing"
			}
		}
		file := learningCandidateDiffFile{
			Name:       after.name,
			Kind:       after.kind,
			Status:     status,
			Summary:    firstNonEmpty(after.summary, truncateSummary(afterContent, 180)),
			BeforeSize: len(beforeContent),
			AfterSize:  len(afterContent),
		}
		if contentLooksSecretBearing(beforeContent) || contentLooksSecretBearing(afterContent) {
			file.RiskSignals = appendUnique(file.RiskSignals, "secret_like_content")
		}
		if includeContent || strings.EqualFold(selected, after.name) {
			file.Before = beforeContent
			file.After = afterContent
		}
		out = append(out, file)
	}
	return out
}

func learningCandidateVirtualFiles(skill teamSkill) []learningCandidateVirtualFile {
	metadata, _ := json.MarshalIndent(map[string]any{
		"name":           skill.Name,
		"title":          skill.Title,
		"description":    skill.Description,
		"channel":        skill.Channel,
		"status":         skill.Status,
		"capabilities":   normalizeSkillCapabilities(skill.Capabilities),
		"plugin_id":      skill.PluginID,
		"plugin_kind":    skill.PluginKind,
		"source_type":    skill.SourceType,
		"source_ref":     skill.SourceRef,
		"source_hash":    skill.SourceHash,
		"scan_status":    skill.ScanStatus,
		"scan_summary":   skill.ScanSummary,
		"health_status":  skill.HealthStatus,
		"health_summary": skill.HealthSummary,
		"tags":           compactStringList(skill.Tags),
	}, "", "  ")
	provenance, _ := json.MarshalIndent(map[string]any{
		"source_type":     skill.SourceType,
		"source_ref":      skill.SourceRef,
		"source_hash":     skill.SourceHash,
		"installed_at":    skill.InstalledAt,
		"last_scanned_at": skill.LastScannedAt,
		"scan_status":     skill.ScanStatus,
		"scan_summary":    skill.ScanSummary,
		"created_by":      skill.CreatedBy,
		"created_at":      skill.CreatedAt,
		"updated_at":      skill.UpdatedAt,
	}, "", "  ")
	return []learningCandidateVirtualFile{
		{name: "metadata.json", kind: "metadata", summary: "Skill identity, routing, trust and capability metadata.", content: string(metadata)},
		{name: "content.md", kind: "instruction", summary: truncateSummary(skill.Content, 180), content: skill.Content},
		{name: "provenance.json", kind: "provenance", summary: "Source task and static scan provenance.", content: string(provenance)},
	}
}

func (b *Broker) buildLearningCandidatesLocked(viewer, channel string, allChannels bool) []learningCandidatePreview {
	var candidates []learningCandidatePreview
	for _, task := range b.tasks {
		taskChannel := normalizeChannelSlug(task.Channel)
		if !allChannels && taskChannel != channel {
			continue
		}
		if !b.canAccessChannelLocked(viewer, taskChannel) {
			continue
		}
		copyTask := task
		applyTaskOperatorContract(&copyTask, currentTaskGoalContext())
		if copyTask.LearningCandidate == nil || !copyTask.LearningCandidate.Recommended {
			continue
		}
		skillName := strings.TrimSpace(copyTask.LearningCandidate.SkillName)
		promotedSkillID := ""
		if skillName != "" {
			if existing := b.findSkillByNameLocked(skillName); existing != nil {
				promotedSkillID = strings.TrimSpace(existing.ID)
			}
		}
		candidates = append(candidates, learningCandidatePreview{
			ID:              "learning:" + strings.TrimSpace(copyTask.ID),
			TaskID:          strings.TrimSpace(copyTask.ID),
			Channel:         taskChannel,
			Owner:           strings.TrimSpace(copyTask.Owner),
			Kind:            normalizeLearningKind(copyTask.LearningCandidate.Kind),
			Title:           truncateSummary(firstNonEmpty(copyTask.LearningCandidate.Title, copyTask.Outcome, copyTask.Title), 140),
			Summary:         truncateSummary(firstNonEmpty(copyTask.LearningCandidate.Summary, copyTask.OutcomeEvidence, copyTask.Details), 260),
			SkillName:       skillName,
			Reason:          strings.TrimSpace(copyTask.LearningCandidate.Reason),
			Signals:         buildLearningCandidateSignals(copyTask),
			Provenance:      buildLearningCandidateProvenance(copyTask),
			Promoted:        promotedSkillID != "",
			PromotedSkillID: promotedSkillID,
			UpdatedAt:       firstNonEmpty(copyTask.OutcomeVerifiedAt, copyTask.UpdatedAt, copyTask.CreatedAt),
		})
	}
	return candidates
}

func buildLearningCandidateSignals(task teamTask) []string {
	var signals []string
	if strings.TrimSpace(task.OutcomeEvidence) != "" {
		signals = append(signals, "outcome_evidence")
	}
	if len(task.Artifacts) > 0 {
		signals = append(signals, "artifacts")
	}
	if latestTaskPlanRevision(&task) != nil {
		signals = append(signals, "plan_revision")
	}
	if len(task.Feedback) > 0 {
		signals = append(signals, "feedback")
	}
	if len(task.ReviewFindings) > 0 || len(task.ReviewFindingHistory) > 0 {
		signals = append(signals, "review_findings")
	}
	if len(task.Evals) > 0 {
		signals = append(signals, "evals")
	}
	if state := latestTaskLivenessState(task); state != "" {
		signals = append(signals, "liveness:"+state)
	}
	return compactStringList(signals)
}

func buildLearningCandidateProvenance(task teamTask) []learningCandidateProvenance {
	var provenance []learningCandidateProvenance
	if evidence := strings.TrimSpace(task.OutcomeEvidence); evidence != "" {
		provenance = append(provenance, learningCandidateProvenance{
			Kind:    "outcome_evidence",
			ID:      strings.TrimSpace(task.ID),
			Summary: truncateSummary(evidence, 220),
			When:    firstNonEmpty(task.OutcomeVerifiedAt, task.UpdatedAt, task.CreatedAt),
		})
	}
	if latest := latestTaskPlanRevision(&task); latest != nil {
		provenance = append(provenance, learningCandidateProvenance{
			Kind:    "plan_revision",
			ID:      strings.TrimSpace(latest.ID),
			Summary: truncateSummary(firstNonEmpty(latest.Summary, latest.Content), 220),
			When:    strings.TrimSpace(latest.CreatedAt),
		})
	}
	for _, artifact := range task.Artifacts {
		provenance = append(provenance, learningCandidateProvenance{
			Kind:    "artifact",
			ID:      strings.TrimSpace(artifact.ID),
			Summary: truncateSummary(firstNonEmpty(artifact.Title, artifact.Summary, artifact.Path, artifact.URL), 220),
			When:    firstNonEmpty(artifact.ValidatedAt, artifact.UpdatedAt, artifact.CreatedAt),
		})
		if len(provenance) >= 6 {
			return provenance
		}
	}
	for _, feedback := range task.Feedback {
		provenance = append(provenance, learningCandidateProvenance{
			Kind:    "feedback",
			ID:      strings.TrimSpace(feedback.ID),
			Summary: truncateSummary(firstNonEmpty(feedback.Comment, feedback.Rating), 220),
			When:    strings.TrimSpace(feedback.CreatedAt),
		})
		if len(provenance) >= 6 {
			return provenance
		}
	}
	return provenance
}

func learningCandidateMatches(candidate learningCandidatePreview, query string) bool {
	haystackParts := []string{
		candidate.ID,
		candidate.TaskID,
		candidate.Channel,
		candidate.Owner,
		candidate.Kind,
		candidate.Title,
		candidate.Summary,
		candidate.SkillName,
		candidate.Reason,
		candidate.PromotedSkillID,
	}
	haystackParts = append(haystackParts, candidate.Signals...)
	for _, source := range candidate.Provenance {
		haystackParts = append(haystackParts, source.Kind, source.ID, source.Summary)
	}
	return strings.Contains(strings.ToLower(strings.Join(haystackParts, " ")), query)
}

type deepPlanningPreview struct {
	Goal             string                  `json:"goal"`
	Outcome          string                  `json:"outcome,omitempty"`
	Channel          string                  `json:"channel"`
	Persisted        bool                    `json:"persisted"`
	RequiresApproval bool                    `json:"requires_approval"`
	Milestones       []deepPlanningMilestone `json:"milestones"`
	Tasks            []validatedPlannedTask  `json:"tasks"`
}

type deepPlanningMilestone struct {
	ID               string                 `json:"id"`
	Title            string                 `json:"title"`
	Outcome          string                 `json:"outcome,omitempty"`
	QueueKey         string                 `json:"queue_key,omitempty"`
	ReviewGate       string                 `json:"review_gate,omitempty"`
	EvidenceRequired []string               `json:"evidence_required,omitempty"`
	Tasks            []validatedPlannedTask `json:"tasks,omitempty"`
}

func (b *Broker) handleDeepPlanning(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Channel         string            `json:"channel"`
		CreatedBy       string            `json:"created_by"`
		Goal            string            `json:"goal"`
		Outcome         string            `json:"outcome"`
		DefaultAssignee string            `json:"default_assignee"`
		Constraints     []string          `json:"constraints"`
		Tasks           []plannedTaskSpec `json:"tasks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	createdBy := strings.TrimSpace(body.CreatedBy)
	goal := strings.TrimSpace(body.Goal)
	if createdBy == "" || goal == "" {
		http.Error(w, "created_by and goal required", http.StatusBadRequest)
		return
	}
	channel := normalizeChannelSlug(body.Channel)
	if channel == "" {
		channel = "general"
	}

	specs := body.Tasks
	if len(specs) == 0 {
		specs = defaultDeepPlanningTasks(goal, body.Outcome, body.DefaultAssignee, body.Constraints)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.canAccessChannelLocked(createdBy, channel) {
		http.Error(w, "channel access denied", http.StatusForbidden)
		return
	}
	if b.findChannelLocked(channel) == nil {
		http.Error(w, "channel not found", http.StatusNotFound)
		return
	}
	validated, err := b.validateStrictTaskPlanLocked(channel, createdBy, specs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	preview := deepPlanningPreview{
		Goal:             goal,
		Outcome:          strings.TrimSpace(body.Outcome),
		Channel:          channel,
		Persisted:        false,
		RequiresApproval: true,
		Tasks:            validated,
		Milestones:       buildDeepPlanningMilestones(validated, goal, body.Outcome),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(preview)
}

func defaultDeepPlanningTasks(goal, outcome, assignee string, constraints []string) []plannedTaskSpec {
	context := strings.TrimSpace(strings.Join(compactStringList(append([]string{outcome}, constraints...)), "\n"))
	if context != "" {
		context = "\nContexto:\n" + context
	}
	discoveryTitle := "Mapear escopo e riscos: " + truncateSummary(goal, 80)
	implementationTitle := "Executar entrega principal: " + truncateSummary(goal, 80)
	verificationTitle := "Validar e fechar: " + truncateSummary(goal, 80)
	return []plannedTaskSpec{
		{
			ExecutionKey:  "deep-plan-discovery",
			Title:         discoveryTitle,
			Assignee:      strings.TrimSpace(assignee),
			Details:       "Confirmar objetivo, restricoes, dependencias e evidencias necessarias." + context,
			TaskType:      "research",
			ExecutionMode: "office",
		},
		{
			ExecutionKey:  "deep-plan-implementation",
			Title:         implementationTitle,
			Assignee:      strings.TrimSpace(assignee),
			Details:       "Implementar a menor entrega completa, mantendo trilha de decisoes e artefatos.",
			TaskType:      "feature",
			ExecutionMode: "office",
			DependsOn:     []string{discoveryTitle},
		},
		{
			ExecutionKey:  "deep-plan-verification",
			Title:         verificationTitle,
			Assignee:      strings.TrimSpace(assignee),
			Details:       "Rodar verificacao, registrar evidencia, riscos restantes e proximo passo.",
			TaskType:      "follow_up",
			ExecutionMode: "office",
			DependsOn:     []string{implementationTitle},
		},
	}
}

func buildDeepPlanningMilestones(tasks []validatedPlannedTask, goal, outcome string) []deepPlanningMilestone {
	if len(tasks) == 0 {
		return nil
	}
	out := make([]deepPlanningMilestone, 0, len(tasks))
	for i, task := range tasks {
		queue := "active"
		probe := teamTask{
			Title:         task.Title,
			Details:       task.Details,
			Owner:         task.Owner,
			TaskType:      task.TaskType,
			ExecutionMode: task.ExecutionMode,
			Channel:       task.Channel,
			DependsOn:     task.ResolvedDepIDs,
			Outcome:       outcome,
			Status:        "open",
		}
		normalizeTaskPlan(&probe)
		applyTaskQueueContract(&probe)
		if probe.QueueKey != "" {
			queue = probe.QueueKey
		}
		out = append(out, deepPlanningMilestone{
			ID:               fmt.Sprintf("milestone-%d", i+1),
			Title:            task.Title,
			Outcome:          firstNonEmpty(outcome, goal),
			QueueKey:         queue,
			ReviewGate:       deepPlanningReviewGate(&probe),
			EvidenceRequired: deepPlanningEvidence(&probe),
			Tasks:            []validatedPlannedTask{task},
		})
	}
	return out
}

func deepPlanningReviewGate(task *teamTask) string {
	if taskNeedsStructuredReview(task) {
		return "structured_review_required"
	}
	if taskNeedsDeepPlan(task) {
		return "plan_review_recommended"
	}
	return "operator_check"
}

func deepPlanningEvidence(task *teamTask) []string {
	evidence := []string{"outcome_summary"}
	if taskNeedsStructuredReview(task) || strings.EqualFold(task.ExecutionMode, "local_worktree") {
		evidence = append(evidence, "test_or_build_output", "diff_or_artifact")
	}
	if strings.EqualFold(task.ExecutionMode, "external_workspace") || strings.EqualFold(task.ExecutionMode, "live_external") {
		evidence = append(evidence, "external_receipt")
	}
	return compactStringList(evidence)
}

type reviewChecklistResponse struct {
	TaskID        string                `json:"task_id"`
	Status        string                `json:"status"`
	BlockingCount int                   `json:"blocking_count"`
	Items         []reviewChecklistItem `json:"items"`
}

type reviewChecklistItem struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Status   string `json:"status"`
	Blocking bool   `json:"blocking,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

func (b *Broker) handleReviewChecklist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	taskID := strings.TrimSpace(r.URL.Query().Get("task_id"))
	viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	if taskID == "" {
		http.Error(w, "task_id required", http.StatusBadRequest)
		return
	}
	b.mu.RLock()
	var found *teamTask
	for i := range b.tasks {
		if strings.TrimSpace(b.tasks[i].ID) == taskID {
			copyTask := b.tasks[i]
			found = &copyTask
			break
		}
	}
	if found == nil {
		b.mu.RUnlock()
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	if !b.canAccessChannelLocked(viewer, found.Channel) {
		b.mu.RUnlock()
		http.Error(w, "channel access denied", http.StatusForbidden)
		return
	}
	requests := append([]humanInterview(nil), b.requests...)
	b.mu.RUnlock()

	applyTaskOperatorContract(found, currentTaskGoalContext())
	payload := buildReviewChecklist(*found, requests)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func buildReviewChecklist(task teamTask, requests []humanInterview) reviewChecklistResponse {
	items := []reviewChecklistItem{
		checklistItem("outcome_evidence", "Outcome evidence", task.CompletionEvidenceSatisfied || !task.CompletionEvidenceRequired, task.CompletionBlocker),
		checklistItem("deep_plan", "Deep plan", !task.PlanRequired || task.PlanStatus == "ready" || task.PlanStatus == "approved", task.PlanBlocker),
		checklistItem("review_findings", "Blocking review findings", !taskHasBlockingReviewFindings(&task), fmt.Sprintf("%d blocking finding(s)", countBlockingReviewFindings(task.ReviewFindings))),
		checklistItem("human_requests", "Human blockers", taskRequestsResolved(requests, task.BlockerRequestIDs), "Resolve pending blocker requests before approval."),
	}
	if taskNeedsStructuredReview(&task) {
		items = append(items, checklistItem("structured_review", "Structured review", task.ReviewState == "ready_for_review" || task.ReviewState == "approved", "Move task through review with structured handoff."))
	}
	blocking := 0
	for _, item := range items {
		if item.Blocking {
			blocking++
		}
	}
	status := "ready"
	if blocking > 0 {
		status = "blocked"
	}
	return reviewChecklistResponse{TaskID: task.ID, Status: status, BlockingCount: blocking, Items: items}
}

func checklistItem(id, label string, ok bool, detail string) reviewChecklistItem {
	status := "ok"
	blocking := false
	if !ok {
		status = "blocked"
		blocking = true
	}
	if ok {
		detail = ""
	}
	return reviewChecklistItem{ID: id, Label: label, Status: status, Blocking: blocking, Detail: strings.TrimSpace(detail)}
}

func taskRequestsResolved(requests []humanInterview, ids []string) bool {
	for _, id := range ids {
		if !requestIsResolvedLocked(requests, id) {
			return false
		}
	}
	return true
}

type templatePreviewResponse struct {
	Persisted                     bool                    `json:"persisted"`
	RequiresTopologyAuthorization bool                    `json:"requires_topology_authorization"`
	Changes                       []templatePreviewChange `json:"changes"`
	BlockedMutations              []string                `json:"blocked_mutations,omitempty"`
	Conflicts                     []templatePreviewIssue  `json:"conflicts,omitempty"`
	Warnings                      []templatePreviewIssue  `json:"warnings,omitempty"`
	SecretRefs                    []string                `json:"secret_refs,omitempty"`
	RequiredReviews               []string                `json:"required_reviews,omitempty"`
	RiskScore                     int                     `json:"risk_score"`
	RiskLevel                     string                  `json:"risk_level"`
	RollbackPlan                  string                  `json:"rollback_plan,omitempty"`
}

type templatePreviewChange struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Action string `json:"action"`
	Reason string `json:"reason,omitempty"`
}

type templatePreviewIssue struct {
	Kind    string `json:"kind"`
	ID      string `json:"id,omitempty"`
	Summary string `json:"summary"`
}

func (b *Broker) handleTemplatePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Title    string   `json:"title"`
		Summary  string   `json:"summary"`
		Agents   []string `json:"agents"`
		Channels []string `json:"channels"`
		Skills   []string `json:"skills"`
		Secrets  []string `json:"secrets"`
		Content  string   `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	payload := templatePreviewResponse{
		Persisted:       false,
		RequiredReviews: []string{"operator"},
		RollbackPlan:    "Preview only. No topology, skill, or broker-state mutation is applied; applying later must record a rollback note and broker-state backup reference.",
	}
	seenAgents := map[string]struct{}{}
	for _, agent := range compactStringList(body.Agents) {
		slug := normalizeActorSlug(agent)
		if slug == "" {
			payload.Conflicts = append(payload.Conflicts, templatePreviewIssue{Kind: "agent", ID: agent, Summary: "Agent slug normalizes to empty."})
			continue
		}
		if _, duplicate := seenAgents[slug]; duplicate {
			payload.Conflicts = append(payload.Conflicts, templatePreviewIssue{Kind: "agent", ID: slug, Summary: "Duplicate agent in template input."})
			continue
		}
		seenAgents[slug] = struct{}{}
		action := "create"
		if b.findMemberLocked(slug) != nil {
			action = "reuse"
		}
		payload.Changes = append(payload.Changes, templatePreviewChange{Kind: "agent", ID: slug, Action: action, Reason: "Template preview only."})
		if action == "create" {
			payload.RequiresTopologyAuthorization = true
			payload.BlockedMutations = append(payload.BlockedMutations, "agent:"+slug)
		}
	}
	seenChannels := map[string]struct{}{}
	for _, channel := range compactStringList(body.Channels) {
		slug := normalizeChannelSlug(channel)
		if slug == "" {
			payload.Conflicts = append(payload.Conflicts, templatePreviewIssue{Kind: "channel", ID: channel, Summary: "Channel slug normalizes to empty."})
			continue
		}
		if _, duplicate := seenChannels[slug]; duplicate {
			payload.Conflicts = append(payload.Conflicts, templatePreviewIssue{Kind: "channel", ID: slug, Summary: "Duplicate channel in template input."})
			continue
		}
		seenChannels[slug] = struct{}{}
		action := "create"
		if b.findChannelLocked(slug) != nil {
			action = "reuse"
		}
		payload.Changes = append(payload.Changes, templatePreviewChange{Kind: "channel", ID: slug, Action: action, Reason: "Template preview only."})
		if action == "create" {
			payload.RequiresTopologyAuthorization = true
			payload.BlockedMutations = append(payload.BlockedMutations, "channel:"+slug)
		}
	}
	seenSkills := map[string]struct{}{}
	for _, skill := range compactStringList(body.Skills) {
		name := skillSlug(skill)
		if name == "" {
			payload.Conflicts = append(payload.Conflicts, templatePreviewIssue{Kind: "skill", ID: skill, Summary: "Skill name normalizes to empty."})
			continue
		}
		if _, duplicate := seenSkills[name]; duplicate {
			payload.Conflicts = append(payload.Conflicts, templatePreviewIssue{Kind: "skill", ID: name, Summary: "Duplicate skill in template input."})
			continue
		}
		seenSkills[name] = struct{}{}
		action := "create"
		if b.findSkillByNameLocked(name) != nil {
			action = "reuse"
		}
		payload.Changes = append(payload.Changes, templatePreviewChange{Kind: "skill", ID: name, Action: action, Reason: firstNonEmpty(body.Title, body.Summary, "Template preview only.")})
	}
	payload.SecretRefs = compactStringList(body.Secrets)
	if contentLooksSecretBearing(body.Title + " " + body.Summary + " " + body.Content + " " + strings.Join(body.Skills, " ")) {
		payload.Warnings = append(payload.Warnings, templatePreviewIssue{Kind: "secret", Summary: "Template text mentions secret-like material; keep values scrubbed and use env/store references only."})
	}
	if len(payload.SecretRefs) > 0 {
		payload.RequiredReviews = appendUnique(payload.RequiredReviews, "secret-audit")
	}
	if payload.RequiresTopologyAuthorization {
		payload.RequiredReviews = appendUnique(payload.RequiredReviews, "topology-authorization")
	}
	payload.RiskScore = templatePreviewRiskScore(payload)
	payload.RiskLevel = templatePreviewRiskLevel(payload.RiskScore)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func templatePreviewRiskScore(payload templatePreviewResponse) int {
	score := len(payload.BlockedMutations)*20 + len(payload.Conflicts)*15 + len(payload.Warnings)*8 + len(payload.SecretRefs)*10
	if score > 100 {
		return 100
	}
	return score
}

func templatePreviewRiskLevel(score int) string {
	switch {
	case score >= 70:
		return "high"
	case score >= 30:
		return "medium"
	default:
		return "low"
	}
}

func parsePositiveLimit(raw string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || parsed <= 0 {
		return fallback
	}
	if parsed > 100 {
		return 100
	}
	return parsed
}
