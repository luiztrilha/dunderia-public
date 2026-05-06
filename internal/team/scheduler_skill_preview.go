package team

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"
)

type schedulerSkillPreviewResponse struct {
	GeneratedAt string                     `json:"generated_at"`
	Persisted   bool                       `json:"persisted"`
	Summary     map[string]int             `json:"summary"`
	Jobs        []schedulerSkillPreviewJob `json:"jobs"`
}

type schedulerSkillPreviewJob struct {
	Slug         string                         `json:"slug,omitempty"`
	Label        string                         `json:"label,omitempty"`
	Kind         string                         `json:"kind,omitempty"`
	Channel      string                         `json:"channel,omitempty"`
	TargetType   string                         `json:"target_type,omitempty"`
	TargetID     string                         `json:"target_id,omitempty"`
	WorkflowKey  string                         `json:"workflow_key,omitempty"`
	ScheduleExpr string                         `json:"schedule_expr,omitempty"`
	Status       string                         `json:"status,omitempty"`
	SkillNames   []string                       `json:"skill_names,omitempty"`
	Readiness    string                         `json:"readiness"`
	RiskLevel    string                         `json:"risk_level"`
	Reasons      []string                       `json:"reasons,omitempty"`
	Skills       []schedulerSkillPreviewBinding `json:"skills,omitempty"`
}

type schedulerSkillPreviewBinding struct {
	Name         string   `json:"name"`
	Found        bool     `json:"found"`
	Status       string   `json:"status,omitempty"`
	TrustLevel   string   `json:"trust_level,omitempty"`
	TrustScore   int      `json:"trust_score,omitempty"`
	SourceType   string   `json:"source_type,omitempty"`
	ScanStatus   string   `json:"scan_status,omitempty"`
	RiskLevel    string   `json:"risk_level"`
	Reasons      []string `json:"reasons,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

func (b *Broker) handleSchedulerSkillPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	readiness := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("readiness")))
	risk := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("risk")))
	limit := parsePositiveLimit(r.URL.Query().Get("limit"), 50)
	includeUnbound := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_unbound")), "true")
	includeTerminal := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_terminal")), "true")
	allChannels := channel == "" || strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all_channels")), "true")

	b.mu.RLock()
	jobs := b.buildSchedulerSkillPreviewLocked(viewer, channel, allChannels, includeUnbound, includeTerminal)
	b.mu.RUnlock()

	filtered := jobs[:0]
	for _, job := range jobs {
		if readiness != "" && !strings.EqualFold(job.Readiness, readiness) {
			continue
		}
		if risk != "" && !strings.EqualFold(job.RiskLevel, risk) {
			continue
		}
		if query != "" && !schedulerSkillPreviewJobMatches(job, query) {
			continue
		}
		filtered = append(filtered, job)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if schedulerPreviewReadinessRank(filtered[i].Readiness) != schedulerPreviewReadinessRank(filtered[j].Readiness) {
			return schedulerPreviewReadinessRank(filtered[i].Readiness) < schedulerPreviewReadinessRank(filtered[j].Readiness)
		}
		if filtered[i].Channel != filtered[j].Channel {
			return filtered[i].Channel < filtered[j].Channel
		}
		return filtered[i].Slug < filtered[j].Slug
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	summary := map[string]int{"total": len(filtered)}
	for _, job := range filtered {
		summary[job.Readiness]++
		summary["risk_"+job.RiskLevel]++
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(schedulerSkillPreviewResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Persisted:   false,
		Summary:     summary,
		Jobs:        filtered,
	})
}

func (b *Broker) buildSchedulerSkillPreviewLocked(viewer, channel string, allChannels, includeUnbound, includeTerminal bool) []schedulerSkillPreviewJob {
	out := make([]schedulerSkillPreviewJob, 0, len(b.scheduler))
	for _, raw := range b.scheduler {
		job := normalizeSchedulerJob(raw)
		if schedulerJobIsTerminal(job) && !includeTerminal {
			continue
		}
		jobChannel := normalizeChannelSlug(job.Channel)
		if jobChannel == "" {
			jobChannel = "general"
		}
		if !allChannels && jobChannel != channel {
			continue
		}
		if !b.canAccessChannelLocked(viewer, jobChannel) {
			continue
		}
		names := schedulerJobSkillNames(job)
		if len(names) == 0 && !includeUnbound {
			continue
		}
		out = append(out, b.schedulerSkillPreviewJobLocked(job, names))
	}
	return out
}

func (b *Broker) schedulerSkillPreviewJobLocked(job schedulerJob, names []string) schedulerSkillPreviewJob {
	preview := schedulerSkillPreviewJob{
		Slug:         strings.TrimSpace(job.Slug),
		Label:        strings.TrimSpace(job.Label),
		Kind:         strings.TrimSpace(job.Kind),
		Channel:      normalizeChannelSlug(job.Channel),
		TargetType:   strings.TrimSpace(job.TargetType),
		TargetID:     strings.TrimSpace(job.TargetID),
		WorkflowKey:  strings.TrimSpace(job.WorkflowKey),
		ScheduleExpr: strings.TrimSpace(job.ScheduleExpr),
		Status:       strings.TrimSpace(job.Status),
		SkillNames:   names,
		Readiness:    "ready",
		RiskLevel:    "low",
	}
	if preview.Channel == "" {
		preview.Channel = "general"
	}
	if len(names) == 0 {
		preview.Readiness = "warning"
		preview.RiskLevel = "medium"
		preview.Reasons = append(preview.Reasons, "no scheduler skills bound")
		return preview
	}
	countBySlug := make(map[string]int, len(names))
	for _, name := range names {
		countBySlug[skillSlug(name)]++
	}
	for _, name := range names {
		binding := b.schedulerSkillPreviewBindingLocked(name)
		if countBySlug[skillSlug(name)] > 1 {
			binding.RiskLevel = maxSchedulerRisk(binding.RiskLevel, "medium")
			binding.Reasons = append(binding.Reasons, "duplicate skill binding")
		}
		preview.Skills = append(preview.Skills, binding)
		switch binding.RiskLevel {
		case "high":
			preview.RiskLevel = maxSchedulerRisk(preview.RiskLevel, "high")
			preview.Readiness = maxSchedulerReadiness(preview.Readiness, "blocked")
		case "medium":
			preview.RiskLevel = maxSchedulerRisk(preview.RiskLevel, "medium")
			preview.Readiness = maxSchedulerReadiness(preview.Readiness, "warning")
		}
		preview.Reasons = append(preview.Reasons, binding.Reasons...)
	}
	preview.Reasons = compactStringList(preview.Reasons)
	return preview
}

func (b *Broker) schedulerSkillPreviewBindingLocked(name string) schedulerSkillPreviewBinding {
	name = strings.TrimSpace(name)
	binding := schedulerSkillPreviewBinding{Name: name, RiskLevel: "low"}
	skill := b.findSkillByNameLocked(name)
	if skill == nil {
		binding.Found = false
		binding.RiskLevel = "high"
		binding.Reasons = []string{"skill not found"}
		return binding
	}
	trust := buildSkillTrustRecord(*skill)
	binding.Found = true
	binding.Status = strings.TrimSpace(skill.Status)
	binding.TrustLevel = trust.Level
	binding.TrustScore = trust.Score
	binding.SourceType = strings.TrimSpace(skill.SourceType)
	binding.ScanStatus = firstNonEmpty(normalizeSkillScanStatus(skill.ScanStatus), "unknown")
	binding.Capabilities = normalizeSkillCapabilities(skill.Capabilities)
	binding.Reasons = append(binding.Reasons, trust.Reasons...)
	switch trust.Level {
	case "low":
		binding.RiskLevel = "high"
		binding.Reasons = append(binding.Reasons, "low trust skill")
	case "medium":
		binding.RiskLevel = maxSchedulerRisk(binding.RiskLevel, "medium")
		binding.Reasons = append(binding.Reasons, "medium trust skill")
	}
	if !skillCanInvoke(*skill) {
		binding.RiskLevel = maxSchedulerRisk(binding.RiskLevel, "high")
		binding.Reasons = append(binding.Reasons, "skill is not invokable")
	}
	if strings.EqualFold(strings.TrimSpace(skill.Status), "proposed") {
		binding.RiskLevel = maxSchedulerRisk(binding.RiskLevel, "medium")
		binding.Reasons = append(binding.Reasons, "skill is still proposed")
	}
	if skillReferencesSchedulerMutation(*skill) {
		binding.RiskLevel = maxSchedulerRisk(binding.RiskLevel, "medium")
		binding.Reasons = append(binding.Reasons, "skill references scheduler mutation")
	}
	binding.Reasons = compactStringList(binding.Reasons)
	return binding
}

func schedulerJobSkillNames(job schedulerJob) []string {
	var names []string
	if strings.TrimSpace(job.SkillName) != "" {
		names = append(names, strings.TrimSpace(job.SkillName))
	}
	names = append(names, job.SkillNames...)
	var payload struct {
		SkillName  string   `json:"skill_name"`
		SkillNames []string `json:"skill_names"`
	}
	if strings.TrimSpace(job.Payload) != "" && json.Unmarshal([]byte(job.Payload), &payload) == nil {
		if strings.TrimSpace(payload.SkillName) != "" {
			names = append(names, strings.TrimSpace(payload.SkillName))
		}
		names = append(names, payload.SkillNames...)
	}
	return uniqueSchedulerSkillNames(names)
}

func uniqueSchedulerSkillNames(names []string) []string {
	out := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		slug := skillSlug(name)
		if slug == "" {
			continue
		}
		if _, ok := seen[slug]; ok {
			continue
		}
		seen[slug] = struct{}{}
		out = append(out, name)
	}
	return out
}

func skillReferencesSchedulerMutation(skill teamSkill) bool {
	text := strings.ToLower(strings.Join([]string{
		skill.Title,
		skill.Description,
		skill.Content,
		skill.WorkflowDefinition,
		skill.Trigger,
	}, "\n"))
	for _, needle := range []string{"/scheduler", "set scheduler", "create scheduler", "schedule job", "cron job"} {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func maxSchedulerRisk(left, right string) string {
	if schedulerRiskRank(right) > schedulerRiskRank(left) {
		return right
	}
	return left
}

func schedulerRiskRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func maxSchedulerReadiness(left, right string) string {
	if schedulerPreviewReadinessRank(right) < schedulerPreviewReadinessRank(left) {
		return right
	}
	return left
}

func schedulerPreviewReadinessRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "blocked":
		return 0
	case "warning":
		return 1
	case "ready":
		return 2
	default:
		return 3
	}
}

func schedulerSkillPreviewJobMatches(job schedulerSkillPreviewJob, query string) bool {
	query = strings.TrimSpace(query)
	if query == "" {
		return true
	}
	parts := []string{
		job.Slug,
		job.Label,
		job.Kind,
		job.Channel,
		job.TargetType,
		job.TargetID,
		job.WorkflowKey,
		job.ScheduleExpr,
		job.Status,
		job.Readiness,
		job.RiskLevel,
	}
	parts = append(parts, job.SkillNames...)
	parts = append(parts, job.Reasons...)
	for _, skill := range job.Skills {
		parts = append(parts, skill.Name, skill.Status, skill.TrustLevel, skill.SourceType, skill.ScanStatus, skill.RiskLevel)
		parts = append(parts, skill.Reasons...)
		parts = append(parts, skill.Capabilities...)
	}
	return privateMemoryMatchScore(normalizeMemorySearchText(strings.Join(parts, "\n")), query) > 0
}
