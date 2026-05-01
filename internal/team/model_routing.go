package team

import (
	"fmt"
	"os"
	"strings"

	"github.com/nex-crm/wuphf/internal/config"
	"github.com/nex-crm/wuphf/internal/provider"
)

const (
	modelProfilePinned   = "pinned"
	modelProfileFast     = "fast"
	modelProfileBalanced = "balanced"
	modelProfileDeep     = "deep"
	modelProfilePremium  = "premium"
)

type modelRouteDecision struct {
	Provider string
	Profile  string
	Model    string
	Reasons  []string
}

func (d modelRouteDecision) summary() string {
	parts := []string{
		"provider=" + strings.TrimSpace(d.Provider),
		"profile=" + strings.TrimSpace(d.Profile),
	}
	if model := strings.TrimSpace(d.Model); model != "" {
		parts = append(parts, "model="+model)
	}
	if len(d.Reasons) > 0 {
		parts = append(parts, "reasons="+strings.Join(d.Reasons, ","))
	}
	return strings.Join(parts, " ")
}

func (d modelRouteDecision) progressDetail() string {
	profile := strings.TrimSpace(d.Profile)
	if profile == "" {
		profile = modelProfileBalanced
	}
	model := strings.TrimSpace(d.Model)
	if model == "" {
		return "model " + profile
	}
	return fmt.Sprintf("model %s - %s", profile, model)
}

func (l *Launcher) resolveHeadlessModelRoute(providerKind, slug, prompt string, channel ...string) modelRouteDecision {
	providerKind = normalizeProviderKind(providerKind)
	if providerKind == "" {
		providerKind = provider.KindClaudeCode
	}
	var turnChannel string
	var task *teamTask
	if l != nil {
		turnChannel = l.headlessTurnChannel(slug, channel...)
		task = l.headlessTaskForExecution(slug, turnChannel)
	}

	if model := l.explicitTaskModelForProvider(task, providerKind); model != "" {
		return modelRouteDecision{
			Provider: providerKind,
			Profile:  modelProfilePinned,
			Model:    model,
			Reasons:  []string{"task_runtime_model"},
		}
	}
	if model := l.explicitMemberModelForProvider(slug, providerKind); model != "" {
		return modelRouteDecision{
			Provider: providerKind,
			Profile:  modelProfilePinned,
			Model:    model,
			Reasons:  []string{"agent_provider_model"},
		}
	}

	profile, reasons := l.selectConservativeModelProfile(slug, prompt, task)
	if providerKind == provider.KindClaudeCode && l != nil && l.opusCEO && slug == l.officeLeadSlug() {
		profile = modelProfilePremium
		reasons = appendReason(reasons, "opus_ceo_flag")
	}
	model := modelForProviderProfile(providerKind, profile, func() string {
		if l != nil {
			return l.cwd
		}
		return ""
	}())
	return modelRouteDecision{
		Provider: providerKind,
		Profile:  profile,
		Model:    model,
		Reasons:  reasons,
	}
}

func (l *Launcher) explicitTaskModelForProvider(task *teamTask, providerKind string) string {
	if task == nil {
		return ""
	}
	model := strings.TrimSpace(task.RuntimeModel)
	if model == "" {
		return ""
	}
	explicitProvider := ""
	if strings.TrimSpace(task.RuntimeProvider) != "" {
		explicitProvider = normalizeProviderKind(task.RuntimeProvider)
	}
	inferredProvider := inferRuntimeProviderFromModel(model)
	switch providerKind {
	case provider.KindCodex:
		if explicitProvider == provider.KindCodex || (explicitProvider == "" && (inferredProvider == "" || inferredProvider == provider.KindCodex)) {
			return model
		}
	case provider.KindClaudeCode:
		if explicitProvider == provider.KindClaudeCode || (explicitProvider == "" && inferredProvider == provider.KindClaudeCode) {
			return model
		}
	case provider.KindGemini, provider.KindGeminiVertex:
		if explicitProvider == providerKind || (explicitProvider == "" && inferredProvider == provider.KindGemini) {
			return model
		}
	case provider.KindOllama:
		if explicitProvider == provider.KindOllama || (explicitProvider == "" && inferredProvider == "") {
			return model
		}
	}
	return ""
}

func (l *Launcher) explicitMemberModelForProvider(slug, providerKind string) string {
	if l == nil || l.broker == nil {
		return ""
	}
	binding := l.broker.MemberProviderBinding(slug)
	model := strings.TrimSpace(binding.Model)
	if model == "" {
		return ""
	}
	explicitProvider := ""
	if strings.TrimSpace(binding.Kind) != "" {
		explicitProvider = normalizeProviderKind(binding.Kind)
	}
	inferredProvider := inferRuntimeProviderFromModel(model)
	switch providerKind {
	case provider.KindCodex:
		if explicitProvider == provider.KindCodex || (explicitProvider == "" && inferredProvider == provider.KindCodex) {
			return model
		}
	case provider.KindClaudeCode:
		if explicitProvider == provider.KindClaudeCode || (explicitProvider == "" && (inferredProvider == "" || inferredProvider == provider.KindClaudeCode)) {
			return model
		}
	case provider.KindGemini, provider.KindGeminiVertex:
		if explicitProvider == providerKind || (explicitProvider == "" && inferredProvider == provider.KindGemini) {
			return model
		}
	case provider.KindOllama:
		if explicitProvider == provider.KindOllama || (explicitProvider == "" && inferredProvider == "") {
			return model
		}
	}
	return ""
}

func (l *Launcher) selectConservativeModelProfile(slug, prompt string, task *teamTask) (string, []string) {
	reasons := []string{"default_balanced"}
	profile := modelProfileBalanced
	var member officeMember
	if l != nil {
		member = l.officeMemberBySlug(slug)
	}
	requestText := strings.ToLower(strings.Join([]string{
		prompt,
		taskModelRoutingText(task),
	}, "\n"))
	text := strings.ToLower(strings.Join([]string{
		prompt,
		taskModelRoutingText(task),
		memberModelRoutingText(member),
	}, "\n"))

	if routeTextContainsAny(requestText, "premium model", "modelo premium", "modelo maximo", "opus", "best model", "strongest model") {
		return modelProfilePremium, []string{"human_requested_premium"}
	}
	if routeTextContainsAny(requestText, "modelo forte", "strong model", "deep model", "use deep", "mais forte", "raciocinio profundo") {
		profile = modelProfileDeep
		reasons = appendReason(reasons, "human_requested_deep")
	}
	if profile == modelProfileBalanced && task == nil && len([]rune(strings.TrimSpace(prompt))) <= 280 && routeTextContainsAny(strings.ToLower(prompt), "ping", "status?", "ok?", "ta online", "are you there") {
		return modelProfileFast, []string{"trivial_ping"}
	}

	if task != nil {
		if effort, err := normalizeReasoningEffort(task.ReasoningEffort); err == nil && (effort == "high" || effort == "xhigh") {
			profile = modelProfileDeep
			reasons = appendReason(reasons, "task_reasoning_effort")
		}
		if isHeadlessCodeWorkspaceExecution(task) {
			profile = modelProfileDeep
			reasons = appendReason(reasons, "workspace_execution")
		}
	}

	if len([]rune(strings.TrimSpace(prompt))) > 6000 {
		profile = modelProfileDeep
		reasons = appendReason(reasons, "large_context")
	}

	switch {
	case (l != nil && slug == l.officeLeadSlug()) || routeTextContainsAny(text, "ceo", "chief", "lead"):
		if routeTextContainsAny(requestText, "strategy", "estrateg", "decision", "decisao", "priorit", "trade-off", "roadmap", "conflict", "ambiguous", "ambigu") {
			profile = modelProfileDeep
			reasons = appendReason(reasons, "lead_strategy")
		}
	case slug == "pm" || routeTextContainsAny(text, "product manager", "requirements", "requisitos", "user story", "prioritization", "priorizacao"):
		if routeTextContainsAny(requestText, "requirements", "requisitos", "scope", "escopo", "priorit", "ambigu", "roadmap", "spec", "acceptance criteria") {
			profile = modelProfileDeep
			reasons = appendReason(reasons, "pm_requirements")
		}
	}

	if codingAgentSlugs[slug] || routeTextContainsAny(text, "engineer", "frontend", "backend", "qa", "developer") {
		if routeTextContainsAny(requestText, "debug", "failing test", "teste falhando", "regression", "regressao", "architecture", "arquitetura", "refactor", "migration", "migracao", "security", "cross-file", "multi-file", "varios arquivos") {
			profile = modelProfileDeep
			reasons = appendReason(reasons, "engineering_complexity")
		}
	}
	if slug == "designer" || routeTextContainsAny(text, "designer", "ui-ux", "visual", "branding", "prototype") {
		if routeTextContainsAny(requestText, "ux", "layout", "visual", "responsive", "accessibility", "acessibilidade", "design system", "prototype", "prototyp") {
			profile = modelProfileDeep
			reasons = appendReason(reasons, "design_complexity")
		}
	}

	return profile, reasons
}

func taskModelRoutingText(task *teamTask) string {
	if task == nil {
		return ""
	}
	return strings.Join([]string{
		task.Title,
		task.Details,
		task.TaskType,
		task.PipelineID,
		task.PipelineStage,
		task.ExecutionMode,
		task.ReviewState,
		task.ProgressBasis,
	}, "\n")
}

func memberModelRoutingText(member officeMember) string {
	return strings.Join([]string{
		member.Slug,
		member.Name,
		member.Role,
		strings.Join(member.Expertise, " "),
	}, "\n")
}

func modelForProviderProfile(providerKind, profile, cwd string) string {
	providerKind = normalizeProviderKind(providerKind)
	profile = strings.TrimSpace(strings.ToLower(profile))
	if override := modelRouteEnvOverride(providerKind, profile); override != "" {
		return override
	}
	switch providerKind {
	case provider.KindClaudeCode:
		switch profile {
		case modelProfileFast:
			return "claude-haiku-4-5"
		case modelProfileDeep, modelProfilePremium:
			return "claude-opus-4-6"
		default:
			return "claude-sonnet-4-6"
		}
	case provider.KindCodex:
		switch profile {
		case modelProfileFast:
			return "gpt-5.4-mini"
		case modelProfileDeep, modelProfilePremium:
			return "gpt-5.5"
		default:
			if model := strings.TrimSpace(config.ResolveCodexModel(cwd)); model != "" {
				return model
			}
			return "gpt-5.4"
		}
	case provider.KindGemini:
		switch profile {
		case modelProfileFast:
			return "gemini-3.1-flash-lite"
		default:
			return provider.GeminiDefaultModel
		}
	case provider.KindGeminiVertex:
		switch profile {
		case modelProfileFast:
			return "gemini-3.1-flash-lite"
		default:
			return provider.GeminiVertexDefaultModel
		}
	case provider.KindOllama:
		return provider.OllamaDefaultModel
	default:
		return ""
	}
}

func modelRouteEnvOverride(providerKind, profile string) string {
	providerKey := strings.NewReplacer("-", "_").Replace(strings.ToUpper(strings.TrimSpace(providerKind)))
	profileKey := strings.ToUpper(strings.TrimSpace(profile))
	if providerKey == "" || profileKey == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv("WUPHF_MODEL_ROUTE_" + providerKey + "_" + profileKey))
}

func appendReason(reasons []string, reason string) []string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return reasons
	}
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}

func routeTextContainsAny(text string, needles ...string) bool {
	text = strings.ToLower(text)
	for _, needle := range needles {
		if strings.Contains(text, strings.ToLower(strings.TrimSpace(needle))) {
			return true
		}
	}
	return false
}
