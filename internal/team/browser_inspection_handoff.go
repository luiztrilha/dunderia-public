package team

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

type browserInspectionHandoffPreviewResponse struct {
	GeneratedAt string                            `json:"generated_at"`
	Persisted   bool                              `json:"persisted"`
	Status      string                            `json:"status"`
	Summary     map[string]int                    `json:"summary"`
	Handoffs    []browserInspectionHandoffPreview `json:"handoffs"`
}

type browserInspectionHandoffPreview struct {
	ID             string   `json:"id"`
	TaskID         string   `json:"task_id"`
	TaskTitle      string   `json:"task_title,omitempty"`
	Channel        string   `json:"channel,omitempty"`
	Owner          string   `json:"owner,omitempty"`
	ArtifactID     string   `json:"artifact_id,omitempty"`
	ArtifactTitle  string   `json:"artifact_title,omitempty"`
	PageURL        string   `json:"page_url,omitempty"`
	Selector       string   `json:"selector,omitempty"`
	ElementText    string   `json:"element_text,omitempty"`
	ScreenshotPath string   `json:"screenshot_path,omitempty"`
	ViewportWidth  int      `json:"viewport_width,omitempty"`
	ViewportHeight int      `json:"viewport_height,omitempty"`
	Evidence       string   `json:"evidence,omitempty"`
	HandoffPrompt  string   `json:"handoff_prompt,omitempty"`
	Ready          bool     `json:"ready"`
	MissingFields  []string `json:"missing_fields,omitempty"`
	RiskSignals    []string `json:"risk_signals,omitempty"`
	NextStep       string   `json:"next_step,omitempty"`
	UpdatedAt      string   `json:"updated_at,omitempty"`
}

func (b *Broker) handleBrowserInspectionHandoffPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	taskID := strings.TrimSpace(r.URL.Query().Get("task_id"))
	limit := parsePositiveLimit(r.URL.Query().Get("limit"), 20)
	allChannels := channel == "" || strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all_channels")), "true")

	b.mu.RLock()
	handoffs := b.buildBrowserInspectionHandoffsLocked(viewer, channel, allChannels, taskID)
	b.mu.RUnlock()
	sort.Slice(handoffs, func(i, j int) bool {
		if handoffs[i].UpdatedAt != handoffs[j].UpdatedAt {
			return studioTimestampAfter(handoffs[i].UpdatedAt, handoffs[j].UpdatedAt)
		}
		return handoffs[i].ID < handoffs[j].ID
	})
	if len(handoffs) > limit {
		handoffs = handoffs[:limit]
	}
	summary := map[string]int{"total": len(handoffs)}
	status := "ok"
	for _, handoff := range handoffs {
		if handoff.Ready {
			summary["ready"]++
		} else {
			summary["review"]++
			if status == "ok" {
				status = "review"
			}
		}
		for _, signal := range handoff.RiskSignals {
			summary["risk_"+signal]++
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(browserInspectionHandoffPreviewResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Persisted:   false,
		Status:      status,
		Summary:     summary,
		Handoffs:    handoffs,
	})
}

func (b *Broker) buildBrowserInspectionHandoffsLocked(viewer, channel string, allChannels bool, taskID string) []browserInspectionHandoffPreview {
	handoffs := make([]browserInspectionHandoffPreview, 0)
	for _, task := range b.tasks {
		if taskID != "" && strings.TrimSpace(task.ID) != taskID {
			continue
		}
		taskChannel := normalizeChannelSlug(task.Channel)
		if !allChannels && taskChannel != channel {
			continue
		}
		if !b.canAccessChannelLocked(viewer, taskChannel) {
			continue
		}
		if strings.TrimSpace(task.ArchivedAt) != "" {
			continue
		}
		for _, artifact := range task.Artifacts {
			if artifact.BrowserInspection == nil && normalizeTaskArtifactKind(artifact.Kind) != "browser_inspection" {
				continue
			}
			handoffs = append(handoffs, buildBrowserInspectionHandoff(task, artifact))
		}
	}
	return handoffs
}

func buildBrowserInspectionHandoff(task teamTask, artifact taskArtifact) browserInspectionHandoffPreview {
	inspection := artifact.BrowserInspection
	if inspection == nil {
		inspection = &taskBrowserInspection{PageURL: artifact.URL, ScreenshotPath: artifact.Path, Notes: artifact.Summary}
	}
	handoff := browserInspectionHandoffPreview{
		ID:             "browser-handoff:" + strings.TrimSpace(task.ID) + ":" + strings.TrimSpace(artifact.ID),
		TaskID:         strings.TrimSpace(task.ID),
		TaskTitle:      strings.TrimSpace(task.Title),
		Channel:        normalizeChannelSlug(task.Channel),
		Owner:          strings.TrimSpace(task.Owner),
		ArtifactID:     strings.TrimSpace(artifact.ID),
		ArtifactTitle:  strings.TrimSpace(artifact.Title),
		PageURL:        strings.TrimSpace(firstNonEmpty(inspection.PageURL, artifact.URL, artifact.PreviewURL)),
		Selector:       strings.TrimSpace(inspection.Selector),
		ElementText:    strings.TrimSpace(inspection.ElementText),
		ScreenshotPath: strings.TrimSpace(firstNonEmpty(inspection.ScreenshotPath, artifact.Path)),
		ViewportWidth:  inspection.ViewportWidth,
		ViewportHeight: inspection.ViewportHeight,
		Evidence:       truncateSummary(firstNonEmpty(artifact.Summary, inspection.Notes, inspection.ElementText, artifact.URL, artifact.Path), 260),
		UpdatedAt:      strings.TrimSpace(firstNonEmpty(artifact.UpdatedAt, artifact.CreatedAt, task.UpdatedAt, task.CreatedAt)),
	}
	handoff.MissingFields = browserInspectionHandoffMissingFields(handoff)
	handoff.Ready = len(handoff.MissingFields) == 0
	handoff.RiskSignals = browserInspectionHandoffRiskSignals(handoff)
	handoff.HandoffPrompt = browserInspectionHandoffPrompt(handoff)
	if handoff.Ready {
		handoff.NextStep = "Attach this browser evidence to the next frontend turn; rerun the visual check after code changes and record a new browser_inspection artifact."
	} else {
		handoff.NextStep = "Complete the missing browser evidence before relying on this handoff for frontend work."
	}
	return handoff
}

func browserInspectionHandoffMissingFields(handoff browserInspectionHandoffPreview) []string {
	missing := make([]string, 0)
	if strings.TrimSpace(handoff.PageURL) == "" {
		missing = append(missing, "page_url")
	}
	if strings.TrimSpace(handoff.Selector) == "" {
		missing = append(missing, "selector")
	}
	if strings.TrimSpace(handoff.ElementText) == "" {
		missing = append(missing, "element_text")
	}
	if strings.TrimSpace(handoff.ScreenshotPath) == "" {
		missing = append(missing, "screenshot_path")
	}
	if handoff.ViewportWidth <= 0 || handoff.ViewportHeight <= 0 {
		missing = append(missing, "viewport")
	}
	return compactStringList(missing)
}

func browserInspectionHandoffRiskSignals(handoff browserInspectionHandoffPreview) []string {
	signals := make([]string, 0)
	if len(handoff.MissingFields) > 0 {
		signals = append(signals, "incomplete_browser_evidence")
	}
	if strings.TrimSpace(handoff.ScreenshotPath) == "" {
		signals = append(signals, "no_screenshot")
	}
	if strings.TrimSpace(handoff.Selector) == "" {
		signals = append(signals, "no_selector")
	}
	if strings.TrimSpace(handoff.PageURL) == "" {
		signals = append(signals, "no_page_url")
	}
	if contentLooksSecretBearing(handoff.ElementText + " " + handoff.Evidence) {
		signals = append(signals, "secret_like_content")
	}
	return compactStringList(signals)
}

func browserInspectionHandoffPrompt(handoff browserInspectionHandoffPreview) string {
	lines := []string{
		"Browser inspection handoff:",
		"- Task: " + strings.TrimSpace(firstNonEmpty(handoff.TaskTitle, handoff.TaskID)),
		"- Page URL: " + strings.TrimSpace(handoff.PageURL),
		"- Selector: " + strings.TrimSpace(handoff.Selector),
		"- Element text: " + truncateSummary(handoff.ElementText, 180),
		"- Viewport: " + browserInspectionViewportText(handoff),
		"- Screenshot: " + strings.TrimSpace(handoff.ScreenshotPath),
		"- Evidence: " + truncateSummary(handoff.Evidence, 220),
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func browserInspectionViewportText(handoff browserInspectionHandoffPreview) string {
	if handoff.ViewportWidth <= 0 || handoff.ViewportHeight <= 0 {
		return ""
	}
	return fmt.Sprintf("%dx%d", handoff.ViewportWidth, handoff.ViewportHeight)
}
