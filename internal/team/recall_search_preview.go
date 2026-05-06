package team

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"
)

type recallSearchPreviewResponse struct {
	GeneratedAt string               `json:"generated_at"`
	Persisted   bool                 `json:"persisted"`
	Query       string               `json:"query,omitempty"`
	Summary     map[string]int       `json:"summary"`
	Results     []recallSearchResult `json:"results"`
}

type recallSearchResult struct {
	ID             string         `json:"id"`
	Kind           string         `json:"kind"`
	Title          string         `json:"title"`
	Summary        string         `json:"summary,omitempty"`
	Channel        string         `json:"channel,omitempty"`
	Actor          string         `json:"actor,omitempty"`
	SourceID       string         `json:"source_id,omitempty"`
	TaskID         string         `json:"task_id,omitempty"`
	UpdatedAt      string         `json:"updated_at,omitempty"`
	Rank           int            `json:"rank"`
	RankSignals    []string       `json:"rank_signals,omitempty"`
	Quality        int            `json:"quality_score"`
	QualitySignals []string       `json:"quality_signals,omitempty"`
	RiskSignals    []string       `json:"risk_signals,omitempty"`
	Sources        []recallSource `json:"sources,omitempty"`
}

type recallSource struct {
	Kind    string `json:"kind"`
	ID      string `json:"id,omitempty"`
	Label   string `json:"label,omitempty"`
	Channel string `json:"channel,omitempty"`
	TaskID  string `json:"task_id,omitempty"`
	When    string `json:"when,omitempty"`
	Ref     string `json:"ref,omitempty"`
}

func (b *Broker) handleRecallSearchPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind")))
	limit := parsePositiveLimit(r.URL.Query().Get("limit"), 25)
	allChannels := channel == "" || strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all_channels")), "true")
	tokens := recallSearchTokens(query)

	b.mu.RLock()
	results := b.buildRecallSearchPreviewLocked(viewer, channel, allChannels, query, tokens)
	b.mu.RUnlock()

	filtered := results[:0]
	for _, result := range results {
		if kind != "" && !strings.EqualFold(result.Kind, kind) {
			continue
		}
		filtered = append(filtered, result)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Rank != filtered[j].Rank {
			return filtered[i].Rank > filtered[j].Rank
		}
		if filtered[i].UpdatedAt != filtered[j].UpdatedAt {
			return studioTimestampAfter(filtered[i].UpdatedAt, filtered[j].UpdatedAt)
		}
		return filtered[i].ID < filtered[j].ID
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	summary := map[string]int{"total": len(filtered)}
	for _, result := range filtered {
		summary["kind_"+result.Kind]++
		summary["quality_"+recallQualityBand(result.Quality)]++
		for _, signal := range result.RiskSignals {
			summary["risk_"+signal]++
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(recallSearchPreviewResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Persisted:   false,
		Query:       query,
		Summary:     summary,
		Results:     filtered,
	})
}

func (b *Broker) buildRecallSearchPreviewLocked(viewer, channel string, allChannels bool, query string, tokens []string) []recallSearchResult {
	results := make([]recallSearchResult, 0)
	add := func(result recallSearchResult, text string) {
		result.Channel = normalizeChannelSlug(result.Channel)
		if result.Channel == "" {
			result.Channel = "general"
		}
		if !allChannels && result.Channel != channel {
			return
		}
		if !b.canAccessChannelLocked(viewer, result.Channel) {
			return
		}
		score, signals := recallSearchScore(result, text, query, tokens)
		if query != "" && score == 0 {
			return
		}
		result.Rank = score
		result.RankSignals = compactStringList(signals)
		result.RiskSignals = recallSearchRiskSignals(result, text)
		result.Quality, result.QualitySignals = recallSearchQuality(result)
		if result.Title == "" {
			result.Title = truncateSummary(text, 120)
		}
		result.Summary = truncateSummary(result.Summary, 260)
		results = append(results, result)
	}
	for _, msg := range b.messages {
		if !b.messageAllowedForChannelReadLocked(msg) {
			continue
		}
		channel := normalizeChannelSlug(msg.Channel)
		if channel == "" {
			channel = "general"
		}
		text := strings.TrimSpace(msg.Title + "\n" + msg.Content)
		threadID := firstNonEmpty(strings.TrimSpace(b.messageThreadRootByID[msg.ID]), strings.TrimSpace(msg.ReplyTo), strings.TrimSpace(msg.ID))
		add(recallSearchResult{
			ID:        "message:" + strings.TrimSpace(msg.ID),
			Kind:      "message",
			Title:     truncateSummary(firstNonEmpty(msg.Title, msg.Content), 140),
			Summary:   truncateSummary(msg.Content, 260),
			Channel:   channel,
			Actor:     strings.TrimSpace(msg.From),
			SourceID:  strings.TrimSpace(msg.ID),
			UpdatedAt: strings.TrimSpace(msg.Timestamp),
			Sources: []recallSource{{
				Kind:    "message",
				ID:      strings.TrimSpace(msg.ID),
				Label:   firstNonEmpty(msg.Title, "Channel message"),
				Channel: channel,
				When:    strings.TrimSpace(msg.Timestamp),
				Ref:     "thread:" + threadID,
			}},
		}, text)
	}
	for _, task := range b.tasks {
		copyTask := task
		applyTaskOperatorContract(&copyTask, currentTaskGoalContext())
		channel := normalizeChannelSlug(copyTask.Channel)
		if channel == "" {
			channel = "general"
		}
		text := strings.TrimSpace(strings.Join([]string{
			copyTask.Title,
			copyTask.Details,
			copyTask.Outcome,
			copyTask.OutcomeEvidence,
			copyTask.LatestPlanSummary,
			copyTask.CompletionBlocker,
			copyTask.QueueReason,
		}, "\n"))
		add(recallSearchResult{
			ID:        "task:" + strings.TrimSpace(copyTask.ID),
			Kind:      "task",
			Title:     truncateSummary(copyTask.Title, 140),
			Summary:   truncateSummary(firstNonEmpty(copyTask.Outcome, copyTask.OutcomeEvidence, copyTask.Details, copyTask.LatestPlanSummary), 260),
			Channel:   channel,
			Actor:     strings.TrimSpace(firstNonEmpty(copyTask.Owner, copyTask.CreatedBy)),
			SourceID:  strings.TrimSpace(copyTask.ID),
			TaskID:    strings.TrimSpace(copyTask.ID),
			UpdatedAt: firstNonEmpty(copyTask.OutcomeVerifiedAt, copyTask.UpdatedAt, copyTask.CreatedAt),
			Sources: []recallSource{{
				Kind:    "task",
				ID:      strings.TrimSpace(copyTask.ID),
				Label:   copyTask.Title,
				Channel: channel,
				TaskID:  strings.TrimSpace(copyTask.ID),
				When:    firstNonEmpty(copyTask.UpdatedAt, copyTask.CreatedAt),
			}},
		}, text)
		for _, artifact := range copyTask.Artifacts {
			artifactText := strings.TrimSpace(strings.Join([]string{
				artifact.Title,
				artifact.Summary,
				artifact.Path,
				artifact.URL,
				artifact.PreviewURL,
				artifact.Kind,
				artifact.ResultRole,
			}, "\n"))
			artifactID := strings.TrimSpace(firstNonEmpty(artifact.ID, artifact.Path, artifact.URL, artifact.PreviewURL))
			add(recallSearchResult{
				ID:        "artifact:" + strings.TrimSpace(copyTask.ID) + ":" + artifactID,
				Kind:      "artifact",
				Title:     truncateSummary(firstNonEmpty(artifact.Title, artifact.Path, artifact.URL, artifact.Kind), 140),
				Summary:   truncateSummary(firstNonEmpty(artifact.Summary, artifact.Path, artifact.URL, artifact.PreviewURL), 260),
				Channel:   channel,
				Actor:     strings.TrimSpace(firstNonEmpty(artifact.CreatedBy, copyTask.Owner, copyTask.CreatedBy)),
				SourceID:  artifactID,
				TaskID:    strings.TrimSpace(copyTask.ID),
				UpdatedAt: firstNonEmpty(artifact.UpdatedAt, artifact.CreatedAt, copyTask.UpdatedAt, copyTask.CreatedAt),
				Sources: []recallSource{
					{Kind: "artifact", ID: artifactID, Label: firstNonEmpty(artifact.Title, artifact.Kind), Channel: channel, TaskID: strings.TrimSpace(copyTask.ID), When: firstNonEmpty(artifact.UpdatedAt, artifact.CreatedAt), Ref: firstNonEmpty(artifact.Path, artifact.URL, artifact.PreviewURL)},
					{Kind: "task", ID: strings.TrimSpace(copyTask.ID), Label: copyTask.Title, Channel: channel, TaskID: strings.TrimSpace(copyTask.ID), When: firstNonEmpty(copyTask.UpdatedAt, copyTask.CreatedAt)},
				},
			}, artifactText)
		}
	}
	for _, decision := range b.decisions {
		channel := normalizeChannelSlug(decision.Channel)
		if channel == "" {
			channel = "general"
		}
		text := strings.TrimSpace(strings.Join([]string{decision.Kind, decision.Summary, decision.Reason, decision.Owner}, "\n"))
		add(recallSearchResult{
			ID:        "decision:" + strings.TrimSpace(decision.ID),
			Kind:      "decision",
			Title:     truncateSummary(firstNonEmpty(decision.Summary, decision.Kind), 140),
			Summary:   truncateSummary(firstNonEmpty(decision.Reason, decision.Summary), 260),
			Channel:   channel,
			Actor:     strings.TrimSpace(decision.Owner),
			SourceID:  strings.TrimSpace(decision.ID),
			UpdatedAt: strings.TrimSpace(decision.CreatedAt),
			Sources: []recallSource{{
				Kind:    "decision",
				ID:      strings.TrimSpace(decision.ID),
				Label:   decision.Summary,
				Channel: channel,
				When:    strings.TrimSpace(decision.CreatedAt),
			}},
		}, text)
	}
	for _, skill := range b.skills {
		skillChannel := normalizeChannelSlug(skill.Channel)
		if skillChannel == "" {
			skillChannel = "general"
		}
		if !strings.EqualFold(strings.TrimSpace(skill.PluginID), "dunderia-learning") && !containsString(skill.Tags, "learning") {
			continue
		}
		text := strings.TrimSpace(strings.Join(append([]string{
			skill.Title,
			skill.Name,
			skill.Description,
			skill.Content,
			skill.WorkflowDefinition,
			skill.SourceRef,
		}, skill.Tags...), "\n"))
		skillID := strings.TrimSpace(firstNonEmpty(skill.ID, skill.Name))
		add(recallSearchResult{
			ID:        "knowledge:skill:" + skillID,
			Kind:      "knowledge",
			Title:     truncateSummary(firstNonEmpty(skill.Title, skill.Name), 140),
			Summary:   truncateSummary(firstNonEmpty(skill.Description, skill.Content), 260),
			Channel:   skillChannel,
			SourceID:  "skill:" + skillID,
			UpdatedAt: firstNonEmpty(skill.UpdatedAt, skill.CreatedAt),
			Sources: []recallSource{{
				Kind:    "learning",
				ID:      "skill:" + skillID,
				Label:   firstNonEmpty(skill.Title, skill.Name),
				Channel: skillChannel,
				When:    firstNonEmpty(skill.UpdatedAt, skill.CreatedAt),
				Ref:     firstNonEmpty(skill.SourceRef, "skill"),
			}},
		}, text)
	}
	return results
}

func recallSearchTokens(query string) []string {
	fields := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(query)), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && !(r >= 'à' && r <= 'ÿ')
	})
	return compactStringList(fields)
}

func recallSearchScore(result recallSearchResult, text, query string, tokens []string) (int, []string) {
	if strings.TrimSpace(query) == "" {
		return 1, []string{"recent_context"}
	}
	haystack := strings.ToLower(strings.Join([]string{
		result.ID,
		result.Kind,
		result.Title,
		result.Summary,
		result.Channel,
		result.Actor,
		result.SourceID,
		result.TaskID,
		text,
	}, " "))
	title := strings.ToLower(result.Title)
	summary := strings.ToLower(result.Summary)
	queryLower := strings.ToLower(strings.TrimSpace(query))
	score := 0
	var signals []string
	if strings.Contains(haystack, queryLower) {
		score += 30
		signals = append(signals, "phrase_match")
	}
	for _, token := range tokens {
		if token == "" {
			continue
		}
		switch {
		case strings.Contains(title, token):
			score += 12
			signals = append(signals, "title_match")
		case strings.Contains(summary, token):
			score += 8
			signals = append(signals, "summary_match")
		case strings.Contains(haystack, token):
			score += 4
			signals = append(signals, "body_match")
		}
	}
	if score == 0 {
		return 0, nil
	}
	if result.TaskID != "" {
		score += 2
		signals = append(signals, "task_link")
	}
	if len(result.Sources) > 1 {
		score += len(result.Sources)
		signals = append(signals, "source_context")
	}
	return score, signals
}

func recallSearchRiskSignals(result recallSearchResult, text string) []string {
	var signals []string
	if contentLooksSecretBearing(text) {
		signals = append(signals, "secret_like_content")
	}
	if len(text) > 700 {
		signals = append(signals, "long_raw_source_summarized")
	}
	for _, source := range result.Sources {
		ref := strings.ToLower(strings.TrimSpace(source.Ref))
		if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
			signals = append(signals, "external_ref")
		}
	}
	return compactStringList(signals)
}

func recallSearchQuality(result recallSearchResult) (int, []string) {
	score := 40
	signals := []string{"indexed_source"}
	if strings.TrimSpace(result.TaskID) != "" || recallResultHasSourceKind(result, "task") {
		score += 15
		signals = append(signals, "task_backlink")
	}
	if len(result.Sources) > 1 {
		score += 10
		signals = append(signals, "multi_source")
	}
	if result.Kind == "artifact" || recallResultHasSourceKind(result, "artifact") {
		score += 10
		signals = append(signals, "artifact_evidence")
	}
	if result.Kind == "decision" || recallResultHasSourceKind(result, "decision") {
		score += 8
		signals = append(signals, "decision_record")
	}
	if strings.TrimSpace(result.Summary) != "" {
		score += 5
		signals = append(signals, "summarized")
	}
	if updated := parseBrokerTimestamp(result.UpdatedAt); !updated.IsZero() {
		if time.Since(updated) <= 90*24*time.Hour {
			score += 10
			signals = append(signals, "fresh_source")
		} else {
			score -= 15
			signals = append(signals, "stale_source")
		}
	}
	for _, signal := range result.RiskSignals {
		switch signal {
		case "secret_like_content":
			score -= 25
			signals = append(signals, "secret_risk_penalty")
		case "external_ref":
			score -= 5
			signals = append(signals, "external_ref_review")
		case "long_raw_source_summarized":
			score -= 5
			signals = append(signals, "summary_loss_review")
		}
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score, compactStringList(signals)
}

func recallResultHasSourceKind(result recallSearchResult, kind string) bool {
	for _, source := range result.Sources {
		if strings.EqualFold(strings.TrimSpace(source.Kind), kind) {
			return true
		}
	}
	return false
}

func recallQualityBand(score int) string {
	switch {
	case score >= 75:
		return "high"
	case score >= 50:
		return "medium"
	default:
		return "low"
	}
}
