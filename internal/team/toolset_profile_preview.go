package team

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"
)

type toolsetProfilePreviewResponse struct {
	GeneratedAt string                  `json:"generated_at"`
	Persisted   bool                    `json:"persisted"`
	Summary     map[string]int          `json:"summary"`
	Profiles    []toolsetProfilePreview `json:"profiles"`
}

type toolsetProfilePreview struct {
	ID              string                     `json:"id"`
	AgentSlug       string                     `json:"agent_slug"`
	AgentName       string                     `json:"agent_name,omitempty"`
	Channel         string                     `json:"channel,omitempty"`
	PermissionMode  string                     `json:"permission_mode,omitempty"`
	DeclaredTools   []string                   `json:"declared_tools,omitempty"`
	RuntimeToolsets []string                   `json:"runtime_toolsets,omitempty"`
	Capabilities    []toolsetCapabilityPreview `json:"capabilities,omitempty"`
	Drift           []string                   `json:"drift,omitempty"`
	RiskLevel       string                     `json:"risk_level"`
	SuggestedAction string                     `json:"suggested_action"`
	Signals         []string                   `json:"signals,omitempty"`
}

type toolsetCapabilityPreview struct {
	Name              string `json:"name"`
	Source            string `json:"source,omitempty"`
	Kind              string `json:"kind,omitempty"`
	Status            string `json:"status,omitempty"`
	Mutating          bool   `json:"mutating,omitempty"`
	External          bool   `json:"external,omitempty"`
	SecretBearing     bool   `json:"secret_bearing,omitempty"`
	SchedulerMutating bool   `json:"scheduler_mutating,omitempty"`
}

func (b *Broker) handleToolsetProfilePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewer := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	risk := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("risk")))
	action := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("action")))
	limit := parsePositiveLimit(r.URL.Query().Get("limit"), 50)
	allChannels := channel == "" || strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all_channels")), "true")

	b.mu.RLock()
	profiles := b.buildToolsetProfilePreviewLocked(viewer, channel, allChannels)
	b.mu.RUnlock()

	filtered := profiles[:0]
	for _, profile := range profiles {
		if risk != "" && !strings.EqualFold(profile.RiskLevel, risk) {
			continue
		}
		if action != "" && !strings.EqualFold(profile.SuggestedAction, action) {
			continue
		}
		if query != "" && !toolsetProfileMatches(profile, query) {
			continue
		}
		filtered = append(filtered, profile)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if toolsetRiskRank(filtered[i].RiskLevel) != toolsetRiskRank(filtered[j].RiskLevel) {
			return toolsetRiskRank(filtered[i].RiskLevel) > toolsetRiskRank(filtered[j].RiskLevel)
		}
		if filtered[i].Channel != filtered[j].Channel {
			return filtered[i].Channel < filtered[j].Channel
		}
		return filtered[i].AgentSlug < filtered[j].AgentSlug
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	summary := map[string]int{"total": len(filtered)}
	for _, profile := range filtered {
		summary["risk_"+profile.RiskLevel]++
		summary[profile.SuggestedAction]++
		for _, drift := range profile.Drift {
			summary["drift_"+drift]++
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toolsetProfilePreviewResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Persisted:   false,
		Summary:     summary,
		Profiles:    filtered,
	})
}

func (b *Broker) buildToolsetProfilePreviewLocked(viewer, channel string, allChannels bool) []toolsetProfilePreview {
	channels := b.visibleToolsetChannelsLocked(viewer, channel, allChannels)
	profiles := make([]toolsetProfilePreview, 0, len(channels)*len(b.members))
	for _, ch := range channels {
		for _, member := range b.members {
			if !b.channelMemberEnabledLocked(ch.Slug, member.Slug) {
				continue
			}
			profiles = append(profiles, b.buildToolsetProfileForMemberLocked(member, ch))
		}
	}
	return profiles
}

func (b *Broker) visibleToolsetChannelsLocked(viewer, channel string, allChannels bool) []teamChannel {
	var channels []teamChannel
	if !allChannels {
		ch := b.findChannelLocked(channel)
		if ch == nil {
			return nil
		}
		if !b.canAccessChannelLocked(viewer, ch.Slug) {
			return nil
		}
		return []teamChannel{*ch}
	}
	for _, ch := range b.channels {
		if ch.Archived {
			continue
		}
		if !b.canAccessChannelLocked(viewer, ch.Slug) {
			continue
		}
		channels = append(channels, ch)
	}
	return channels
}

func (b *Broker) buildToolsetProfileForMemberLocked(member officeMember, ch teamChannel) toolsetProfilePreview {
	channel := normalizeChannelSlug(ch.Slug)
	if channel == "" {
		channel = "general"
	}
	profile := toolsetProfilePreview{
		ID:              normalizeChannelSlug(channel + ":" + member.Slug),
		AgentSlug:       normalizeActorSlug(member.Slug),
		AgentName:       strings.TrimSpace(member.Name),
		Channel:         channel,
		PermissionMode:  strings.TrimSpace(member.PermissionMode),
		DeclaredTools:   normalizeStringList(member.AllowedTools),
		RuntimeToolsets: inferRuntimeToolsetsForMember(member, ch),
		RiskLevel:       "low",
		SuggestedAction: "keep",
	}
	profile.Capabilities = append(profile.Capabilities, officeToolsetCapabilities(member, ch)...)
	profile.Capabilities = append(profile.Capabilities, b.adapterToolsetCapabilitiesLocked()...)
	profile.Capabilities = append(profile.Capabilities, b.skillToolsetCapabilitiesLocked(channel)...)
	profile.Capabilities = uniqueToolsetCapabilities(profile.Capabilities)
	profile.Drift = toolsetProfileDrift(profile)
	profile.Signals = toolsetProfileSignals(profile)
	profile.RiskLevel = toolsetProfileRiskLevel(profile)
	profile.SuggestedAction = toolsetProfileSuggestedAction(profile)
	return profile
}

func inferRuntimeToolsetsForMember(member officeMember, ch teamChannel) []string {
	var toolsets []string
	channel := normalizeChannelSlug(ch.Slug)
	toolsets = append(toolsets, "office", "memory", "skills")
	if IsDMSlug(channel) || ch.isDM() {
		toolsets = append(toolsets, "dm")
	} else {
		toolsets = append(toolsets, "tasks", "requests")
	}
	if normalizeActorSlug(member.Slug) == "ceo" {
		toolsets = append(toolsets, "office-admin", "topology")
	}
	if codingAgentSlugs[normalizeActorSlug(member.Slug)] {
		toolsets = append(toolsets, "scoped-mcp", "workspace")
	} else {
		toolsets = append(toolsets, "nex")
	}
	return compactStringList(toolsets)
}

func officeToolsetCapabilities(member officeMember, ch teamChannel) []toolsetCapabilityPreview {
	isDM := IsDMSlug(ch.Slug) || ch.isDM()
	isLead := normalizeActorSlug(member.Slug) == "ceo"
	caps := []toolsetCapabilityPreview{
		{Name: "team_poll", Source: "mcp:wuphf-office", Kind: "read", Status: "available"},
		{Name: "team_inbox", Source: "mcp:wuphf-office", Kind: "read", Status: "available"},
		{Name: "team_memory_query", Source: "mcp:wuphf-office", Kind: "read", Status: "available"},
		{Name: "team_skill_list", Source: "mcp:wuphf-office", Kind: "read", Status: "available"},
		{Name: "team_skill_view", Source: "mcp:wuphf-office", Kind: "read", Status: "available"},
		{Name: "team_broadcast", Source: "mcp:wuphf-office", Kind: "write", Status: "available", Mutating: true},
		{Name: "team_memory_write", Source: "mcp:wuphf-office", Kind: "write", Status: "available", Mutating: true, SecretBearing: true},
		{Name: "team_memory_promote", Source: "mcp:wuphf-office", Kind: "write", Status: "available", Mutating: true},
		{Name: "team_skill_run", Source: "mcp:wuphf-office", Kind: "write", Status: "available", Mutating: true},
		{Name: "human_message", Source: "mcp:wuphf-office", Kind: "write", Status: "available", Mutating: true, External: true},
		{Name: "human_interview", Source: "mcp:wuphf-office", Kind: "write", Status: "available", Mutating: true, External: true, SecretBearing: true},
	}
	if !isDM {
		caps = append(caps,
			toolsetCapabilityPreview{Name: "team_tasks", Source: "mcp:wuphf-office", Kind: "read", Status: "available"},
			toolsetCapabilityPreview{Name: "team_runtime_state", Source: "mcp:wuphf-office", Kind: "read", Status: "available"},
			toolsetCapabilityPreview{Name: "team_task", Source: "mcp:wuphf-office", Kind: "write", Status: "available", Mutating: true},
			toolsetCapabilityPreview{Name: "team_request", Source: "mcp:wuphf-office", Kind: "write", Status: "available", Mutating: true, SecretBearing: true},
			toolsetCapabilityPreview{Name: "adapter_action", Source: "mcp:wuphf-office", Kind: "write", Status: "available", Mutating: true, External: true},
		)
	}
	if isLead {
		caps = append(caps,
			toolsetCapabilityPreview{Name: "team_channel", Source: "mcp:wuphf-office", Kind: "write", Status: "available", Mutating: true},
			toolsetCapabilityPreview{Name: "team_channel_member", Source: "mcp:wuphf-office", Kind: "write", Status: "available", Mutating: true},
			toolsetCapabilityPreview{Name: "team_member", Source: "mcp:wuphf-office", Kind: "write", Status: "available", Mutating: true},
			toolsetCapabilityPreview{Name: "team_bridge", Source: "mcp:wuphf-office", Kind: "write", Status: "available", Mutating: true},
		)
	}
	return caps
}

func (b *Broker) adapterToolsetCapabilitiesLocked() []toolsetCapabilityPreview {
	adapters := mergedOfficeAdapters(b.adapters)
	caps := make([]toolsetCapabilityPreview, 0, len(adapters))
	for _, adapter := range adapters {
		status := firstNonEmpty(adapter.HealthStatus, adapter.Status)
		for _, capability := range adapter.Capabilities {
			caps = append(caps, toolsetCapabilityPreview{
				Name:              capability,
				Source:            "adapter:" + adapter.ID,
				Kind:              firstNonEmpty(adapter.Kind, "adapter"),
				Status:            status,
				Mutating:          capabilityLooksMutating(capability),
				External:          adapterLooksExternal(adapter),
				SecretBearing:     capabilityLooksSecretBearing(capability) || contentLooksSecretBearing(adapter.ConfigRef),
				SchedulerMutating: capabilityLooksSchedulerMutating(capability),
			})
		}
	}
	return caps
}

func (b *Broker) skillToolsetCapabilitiesLocked(channel string) []toolsetCapabilityPreview {
	caps := make([]toolsetCapabilityPreview, 0, len(b.skills))
	for _, skill := range b.skills {
		if skill.Status == "archived" || !skillVisibleInChannel(skill.Channel, channel) {
			continue
		}
		for _, capability := range normalizeSkillCapabilities(skill.Capabilities) {
			caps = append(caps, toolsetCapabilityPreview{
				Name:              capability,
				Source:            "skill:" + skillSlug(skill.Name),
				Kind:              firstNonEmpty(skill.PluginKind, "skill"),
				Status:            firstNonEmpty(skill.HealthStatus, skill.Status),
				Mutating:          capabilityLooksMutating(capability),
				External:          skillLooksExternal(skill),
				SecretBearing:     capabilityLooksSecretBearing(capability) || contentLooksSecretBearing(skill.Content+" "+skill.Description+" "+skill.WorkflowDefinition),
				SchedulerMutating: capabilityLooksSchedulerMutating(capability) || skillReferencesSchedulerMutation(skill),
			})
		}
	}
	return caps
}

func uniqueToolsetCapabilities(caps []toolsetCapabilityPreview) []toolsetCapabilityPreview {
	out := make([]toolsetCapabilityPreview, 0, len(caps))
	seen := make(map[string]int, len(caps))
	for _, cap := range caps {
		cap.Name = strings.TrimSpace(cap.Name)
		cap.Source = strings.TrimSpace(cap.Source)
		if cap.Name == "" {
			continue
		}
		key := strings.ToLower(cap.Source + "::" + cap.Name)
		if idx, ok := seen[key]; ok {
			out[idx].Mutating = out[idx].Mutating || cap.Mutating
			out[idx].External = out[idx].External || cap.External
			out[idx].SecretBearing = out[idx].SecretBearing || cap.SecretBearing
			out[idx].SchedulerMutating = out[idx].SchedulerMutating || cap.SchedulerMutating
			continue
		}
		seen[key] = len(out)
		out = append(out, cap)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func toolsetProfileDrift(profile toolsetProfilePreview) []string {
	var drift []string
	declared := stringSet(profile.DeclaredTools)
	if len(declared) == 0 {
		drift = append(drift, "missing_declaration")
		return drift
	}
	runtime := make(map[string]struct{}, len(profile.Capabilities)+len(profile.RuntimeToolsets))
	for _, toolset := range profile.RuntimeToolsets {
		runtime[normalizeExecutionKey(toolset)] = struct{}{}
	}
	for _, cap := range profile.Capabilities {
		runtime[normalizeExecutionKey(cap.Name)] = struct{}{}
	}
	for _, tool := range profile.DeclaredTools {
		if _, ok := runtime[normalizeExecutionKey(tool)]; !ok {
			drift = append(drift, "declared_missing_runtime")
			break
		}
	}
	mutatingDeclared := false
	for _, cap := range profile.Capabilities {
		if !cap.Mutating {
			continue
		}
		if _, ok := declared[cap.Name]; ok {
			mutatingDeclared = true
			break
		}
		if _, ok := declared[normalizeExecutionKey(cap.Name)]; ok {
			mutatingDeclared = true
			break
		}
	}
	if !mutatingDeclared {
		drift = append(drift, "mutating_runtime_not_declared")
	}
	return compactStringList(drift)
}

func toolsetProfileSignals(profile toolsetProfilePreview) []string {
	var signals []string
	if strings.EqualFold(profile.PermissionMode, "dangerously-skip-permissions") || strings.EqualFold(profile.PermissionMode, "bypass") {
		signals = append(signals, "permissive_mode")
	}
	for _, cap := range profile.Capabilities {
		if cap.Mutating {
			signals = append(signals, "mutating")
		}
		if cap.External {
			signals = append(signals, "external")
		}
		if cap.SecretBearing {
			signals = append(signals, "secret_bearing")
		}
		if cap.SchedulerMutating {
			signals = append(signals, "scheduler_mutating")
		}
	}
	signals = append(signals, profile.Drift...)
	return compactStringList(signals)
}

func toolsetProfileRiskLevel(profile toolsetProfilePreview) string {
	signals := stringSet(profile.Signals)
	switch {
	case setContains(signals, "permissive_mode") && setContains(signals, "secret_bearing"):
		return "high"
	case setContains(signals, "scheduler_mutating") && setContains(signals, "mutating"):
		return "high"
	case setContains(signals, "declared_missing_runtime"):
		return "medium"
	case setContains(signals, "mutating_runtime_not_declared"):
		return "medium"
	case setContains(signals, "external") || setContains(signals, "secret_bearing"):
		return "medium"
	default:
		return "low"
	}
}

func toolsetProfileSuggestedAction(profile toolsetProfilePreview) string {
	switch profile.RiskLevel {
	case "high":
		return "restrict"
	case "medium":
		return "review"
	default:
		if len(profile.Drift) > 0 {
			return "document"
		}
		return "keep"
	}
}

func toolsetRiskRank(value string) int {
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

func toolsetProfileMatches(profile toolsetProfilePreview, query string) bool {
	query = strings.TrimSpace(query)
	if query == "" {
		return true
	}
	parts := []string{
		profile.ID,
		profile.AgentSlug,
		profile.AgentName,
		profile.Channel,
		profile.PermissionMode,
		profile.RiskLevel,
		profile.SuggestedAction,
	}
	parts = append(parts, profile.DeclaredTools...)
	parts = append(parts, profile.RuntimeToolsets...)
	parts = append(parts, profile.Drift...)
	parts = append(parts, profile.Signals...)
	for _, cap := range profile.Capabilities {
		parts = append(parts, cap.Name, cap.Source, cap.Kind, cap.Status)
	}
	return privateMemoryMatchScore(normalizeMemorySearchText(strings.Join(parts, "\n")), query) > 0
}

func capabilityLooksMutating(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, needle := range []string{"write", "create", "delete", "remove", "update", "edit", "run", "execute", "invoke", "open", "restart", "promote", "sync", "route", "record", "audit"} {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func capabilityLooksSecretBearing(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, needle := range []string{"secret", "credential", "token", "auth", "password", "key"} {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func capabilityLooksSchedulerMutating(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, needle := range []string{"scheduler", "schedule", "cron", "timer"} {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func adapterLooksExternal(adapter officeAdapter) bool {
	provider := strings.ToLower(strings.TrimSpace(adapter.Provider))
	return provider != "" && provider != "dunderia" && provider != "local" && provider != "builtin"
}

func skillLooksExternal(skill teamSkill) bool {
	sourceType := strings.ToLower(strings.TrimSpace(skill.SourceType))
	provider := strings.ToLower(strings.TrimSpace(skill.WorkflowProvider))
	return sourceType == "external" || sourceType == "remote" || (provider != "" && provider != "dunderia" && provider != "local")
}
