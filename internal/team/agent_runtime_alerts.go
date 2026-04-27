package team

import (
	"fmt"
	"os"
	"strings"

	"github.com/nex-crm/wuphf/internal/provider"
)

type agentOperationalIssue struct {
	Summary  string
	Blocking bool
}

func detectAgentOperationalIssue(detail string) (agentOperationalIssue, bool) {
	normalized := strings.ToLower(strings.TrimSpace(detail))
	if normalized == "" {
		return agentOperationalIssue{}, false
	}

	switch {
	case agentOperationalFilesystemMCPFailure(normalized):
		return agentOperationalIssue{Summary: "filesystem MCP resource access failed", Blocking: false}, true
	case agentOperationalProviderFailure(normalized):
		return agentOperationalIssue{Summary: "runtime/provider failed before durable progress", Blocking: false}, true
	case strings.Contains(normalized, "blocked by policy"):
		return agentOperationalIssue{Summary: "tool call blocked by runtime policy", Blocking: true}, true
	case strings.Contains(normalized, "safe.directory"),
		strings.Contains(normalized, "dubious ownership"):
		return agentOperationalIssue{Summary: "git safe.directory blocked workspace inspection", Blocking: true}, true
	case strings.Contains(normalized, "read-only"),
		strings.Contains(normalized, "filesystem sandbox"),
		strings.Contains(normalized, "writable workspace"):
		return agentOperationalIssue{Summary: "workspace is not writable in this sandbox", Blocking: true}, true
	default:
		return agentOperationalIssue{}, false
	}
}

func agentOperationalProviderFailure(normalized string) bool {
	return isHeadlessRuntimeProviderFailure(normalized)
}

func agentOperationalFilesystemMCPFailure(normalized string) bool {
	if normalized == "" || !strings.Contains(normalized, "filesystem") {
		return false
	}
	switch {
	case strings.Contains(normalized, "resources/list failed"),
		strings.Contains(normalized, "resources/read failed"),
		strings.Contains(normalized, "read_mcp_resource"),
		strings.Contains(normalized, "unknown mcp server"),
		strings.Contains(normalized, "method not found"):
		return true
	default:
		return false
	}
}

func agentRuntimeAlertTargetID(slug, channel string) string {
	slug = strings.TrimSpace(slug)
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		channel = "general"
	}
	return slug + "|" + channel
}

func (l *Launcher) maybeHandleAgentOperationalIssue(slug string, status string, detail string, channel ...string) {
	if l == nil || l.broker == nil {
		return
	}
	slug = strings.TrimSpace(slug)
	status = strings.ToLower(strings.TrimSpace(status))
	detail = strings.TrimSpace(detail)
	if slug == "" {
		return
	}

	if issue, ok := detectAgentOperationalIssue(detail); ok {
		normalized := strings.ToLower(strings.TrimSpace(detail))
		if !issue.Blocking {
			if agentOperationalFilesystemMCPFailure(normalized) && l.agentCanFallbackWithoutFilesystemMCP(slug, channel...) {
				_ = l.broker.ResolveAgentRuntimeAlerts(agentRuntimeAlertTargetID(slug, firstNonEmpty(channel...)))
				return
			}
			if agentOperationalProviderFailure(normalized) && l.agentCanFallbackFromProviderFailure(slug, channel...) {
				_ = l.broker.ResolveAgentRuntimeAlerts(agentRuntimeAlertTargetID(slug, firstNonEmpty(channel...)))
				return
			}
		}
		l.reportAgentOperationalIssue(slug, issue, detail, channel...)
		return
	}

	if agentRuntimeAlertShouldResolve(status) {
		_ = l.broker.ResolveAgentRuntimeAlerts(agentRuntimeAlertTargetID(slug, firstNonEmpty(channel...)))
	}
}

func agentRuntimeAlertShouldResolve(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "idle":
		return true
	default:
		return false
	}
}

func (l *Launcher) agentCanFallbackWithoutFilesystemMCP(slug string, channel ...string) bool {
	if l == nil {
		return false
	}
	laneChannel := normalizeChannelSlug(firstNonEmpty(channel...))
	workspaceDir := normalizeHeadlessWorkspaceDir(l.headlessWorkspaceDirForExecution(slug, laneChannel))
	if workspaceDir == "" {
		workspaceDir = normalizeHeadlessWorkspaceDir(strings.TrimSpace(l.cwd))
	}
	if workspaceDir == "" {
		return false
	}
	info, err := os.Stat(workspaceDir)
	return err == nil && info.IsDir()
}

func (l *Launcher) agentCanFallbackFromProviderFailure(slug string, channel ...string) bool {
	if l == nil {
		return false
	}
	current := normalizeProviderKind(l.memberEffectiveProviderKind(slug))
	for _, candidate := range []string{
		provider.KindClaudeCode,
		provider.KindCodex,
		provider.KindGemini,
		provider.KindGeminiVertex,
		provider.KindOpenclaude,
	} {
		if normalizeProviderKind(candidate) == current {
			continue
		}
		if headlessRuntimeProviderAvailable(candidate) {
			return true
		}
	}
	return false
}

func (l *Launcher) reportAgentOperationalIssue(slug string, issue agentOperationalIssue, rawDetail string, channel ...string) {
	if l == nil || l.broker == nil {
		return
	}

	laneChannel := normalizeChannelSlug(firstNonEmpty(channel...))
	task := l.headlessTaskForExecution(slug, laneChannel)
	alertChannel := normalizeChannelSlug(firstNonEmpty(
		laneChannel,
		func() string {
			if task != nil {
				return task.Channel
			}
			return ""
		}(),
		l.headlessTurnChannel(slug),
	))
	if alertChannel == "" {
		alertChannel = "general"
	}
	lead := strings.TrimSpace(l.officeLeadSlug())
	if lead == "" {
		lead = "ceo"
	}

	targetID := agentRuntimeAlertTargetID(slug, alertChannel)
	summary := fmt.Sprintf("@%s hit an operational runtime block: %s", slug, issue.Summary)
	alert, existing, err := l.broker.CreateWatchdogAlert("agent_runtime_blocked", alertChannel, "agent_runtime", targetID, lead, summary)
	if err != nil {
		return
	}
	if existing {
		return
	}

	_ = l.broker.RecordAction("watchdog_alert", "watchdog", alertChannel, "watchdog", truncate(summary, 140), alert.ID, nil, "")

	lines := []string{summary + "."}
	if task != nil {
		lines = append(lines, fmt.Sprintf("Task: %s (`%s`).", strings.TrimSpace(task.Title), strings.TrimSpace(task.ID)))
	}
	if detail := strings.TrimSpace(rawDetail); detail != "" {
		lines = append(lines, "Evidence: "+truncate(detail, 220))
	}
	lines = append(lines, "@ceo inspect the lane, decide whether this is a real repo blocker or a runtime limitation, and keep the human informed.")

	replyTo := ""
	if task != nil {
		replyTo = strings.TrimSpace(task.ThreadID)
	}
	_, _, _ = l.broker.PostAutomationMessage(
		"wuphf",
		alertChannel,
		"Agent runtime issue",
		strings.Join(lines, "\n\n"),
		"watchdog-agent-runtime-"+targetID,
		"watchdog",
		"Office watchdog",
		[]string{lead},
		replyTo,
	)
}
