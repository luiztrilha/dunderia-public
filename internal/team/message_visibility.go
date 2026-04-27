package team

import (
	"encoding/json"
	"strings"
)

var internalRuntimeToolNames = map[string]struct{}{
	"team_broadcast":  {},
	"team_task":       {},
	"team_plan":       {},
	"team_channel":    {},
	"team_member":     {},
	"team_bridge":     {},
	"human_message":   {},
	"human_interview": {},
	"local_exec":      {},
}

var nonCollaborativeMessageKinds = map[string]struct{}{
	"skill_update":          {},
	"skill_proposal":        {},
	"skill_invocation":      {},
	"routing":               {},
	"onboarding_origin":     {},
	"synthesized_blueprint": {},
	"from_scratch_contract": {},
}

func messageIsSubstantiveOfficeContent(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" || strings.HasPrefix(content, "[STATUS]") {
		return false
	}
	return !messageContentLooksLikeDisallowedAgentChannelContent(content)
}

func messageContentLooksLikeDisallowedAgentChannelContent(content string) bool {
	return messageContentLooksInternalRuntimePayload(content) ||
		messageContentLooksLikeRawRuntimeToolMarkup(content) ||
		messageContentLooksLikeRawRuntimeToolFailure(content) ||
		messageContentLooksLikeAgentSelfTalk(content)
}

func messageKindSuppressesOfficeWake(kind string) bool {
	_, ok := nonCollaborativeMessageKinds[strings.TrimSpace(strings.ToLower(kind))]
	return ok
}

func messageContentLooksLikeRawRuntimeToolMarkup(content string) bool {
	normalized := strings.ToLower(strings.TrimSpace(content))
	if normalized == "" {
		return false
	}
	switch {
	case strings.HasPrefix(normalized, "[team_broadcast"),
		strings.HasPrefix(normalized, "[team_task"),
		strings.HasPrefix(normalized, "[team_plan"),
		strings.HasPrefix(normalized, "[team_channel"),
		strings.HasPrefix(normalized, "[team_member"),
		strings.HasPrefix(normalized, "[human_message"),
		strings.HasPrefix(normalized, "[human_interview"):
		return true
	}
	if strings.Contains(normalized, "**team broadcast**") && strings.Contains(normalized, "reply_to_id:") {
		return true
	}
	return false
}

func messageContentLooksLikeRawRuntimeToolFailure(content string) bool {
	normalized := strings.ToLower(strings.TrimSpace(content))
	if normalized == "" {
		return false
	}

	lines := compactNonEmptyNormalizedLines(normalized)
	if len(lines) == 0 {
		return false
	}

	switch {
	case len(lines) <= 3 && rawRuntimeToolInvocationLine(lines[0]):
		if len(lines) == 1 {
			return false
		}
		for _, line := range lines[1:] {
			if !rawRuntimeFailureBoilerplateLine(line) {
				return false
			}
		}
		return true
	case len(lines) == 1 && (strings.HasPrefix(lines[0], "resources/read failed") ||
		strings.HasPrefix(lines[0], "resources/list failed") ||
		strings.HasPrefix(lines[0], "unknown mcp server")):
		return true
	}

	if len(lines) == 1 && rawRuntimeFailureBoilerplateLine(lines[0]) && strings.Contains(lines[0], "blocked by policy") {
		return true
	}
	return false
}

func compactNonEmptyNormalizedLines(content string) []string {
	rawLines := strings.Split(content, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func rawRuntimeToolInvocationLine(line string) bool {
	switch strings.TrimSpace(line) {
	case "read_mcp_resource", "list_mcp_resources", "list_mcp_resource_templates":
		return true
	default:
		return false
	}
}

func rawRuntimeFailureBoilerplateLine(line string) bool {
	line = strings.TrimSpace(line)
	switch {
	case line == "erro", line == "error", line == "mcp error":
		return true
	case strings.HasPrefix(line, "mcp error:"),
		strings.HasPrefix(line, "resources/read failed"),
		strings.HasPrefix(line, "resources/list failed"),
		strings.HasPrefix(line, "unknown mcp server"),
		strings.HasPrefix(line, "method not found"),
		strings.HasPrefix(line, "blocked by policy"):
		return true
	default:
		return false
	}
}

func messageContentLooksInternalRuntimePayload(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" || !strings.HasPrefix(content, "{") {
		return false
	}

	var raw map[string]any
	decoder := json.NewDecoder(strings.NewReader(content))
	if err := decoder.Decode(&raw); err != nil {
		return false
	}
	if runtimePayloadLooksInternal(raw) {
		return true
	}
	if _, err := decoder.Token(); err == nil {
		// Extra JSON tokens after the first object still indicate raw runtime payload chatter.
		return true
	}
	return false
}

func runtimePayloadLooksInternal(raw map[string]any) bool {
	if len(raw) == 0 {
		return false
	}

	if name := strings.ToLower(strings.TrimSpace(runtimePayloadString(raw["name"]))); name != "" {
		if _, ok := internalRuntimeToolNames[name]; ok {
			return true
		}
	}

	switch typ := strings.ToLower(strings.TrimSpace(runtimePayloadString(raw["type"]))); typ {
	case "item.completed", "item.started", "item.updated", "turn.completed", "turn.failed", "turn.started",
		"response.output_item.done", "response.function_call_arguments.delta", "response.function_call_arguments.done":
		return true
	case "mcp_tool_call", "function_call", "tool_call", "custom_tool_call", "computer_call":
		return true
	default:
		if _, ok := internalRuntimeToolNames[typ]; ok {
			return true
		}
	}

	if item, ok := raw["item"].(map[string]any); ok {
		switch itemType := strings.ToLower(strings.TrimSpace(runtimePayloadString(item["type"]))); itemType {
		case "mcp_tool_call", "function_call", "tool_call", "custom_tool_call", "computer_call":
			return true
		}
	}

	if hasRuntimePayloadRoutingShape(raw) {
		return true
	}

	return false
}

func runtimePayloadString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func hasRuntimePayloadRoutingShape(raw map[string]any) bool {
	if len(raw) == 0 {
		return false
	}
	required := []string{"channel", "content"}
	for _, key := range required {
		if strings.TrimSpace(runtimePayloadString(raw[key])) == "" {
			return false
		}
	}
	if strings.TrimSpace(runtimePayloadString(raw["my_slug"])) == "" &&
		strings.TrimSpace(runtimePayloadString(raw["slug"])) == "" {
		return false
	}
	replyTo := strings.TrimSpace(runtimePayloadString(raw["reply_to_id"]))
	if replyTo == "" {
		replyTo = strings.TrimSpace(runtimePayloadString(raw["reply_to"]))
	}
	return replyTo != ""
}

func messageContentLooksLikeAgentSelfTalk(content string) bool {
	lines := compactNonEmptyNormalizedLines(normalizeCoordinationText(content))
	if len(lines) < 4 {
		return false
	}

	selfTalkLines := 0
	for _, line := range lines {
		if agentSelfTalkLine(line) {
			selfTalkLines++
		}
	}
	return selfTalkLines >= 4 && selfTalkLines*2 >= len(lines)
}

func agentSelfTalkLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	prefixes := []string{
		"vou ",
		"agora vou ",
		"primeiro vou ",
		"tambem vou ",
		"tambem irei ",
		"encontrei ",
		"vou abrir ",
		"vou aplicar ",
		"vou validar ",
		"vou registrar ",
		"vou repassar ",
		"vou pausar ",
		"ja enviei ",
		"nao apliquei ",
		"status registrado ",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}
