package team

import "strings"

const agentLaneSeparator = "|"

func agentLaneKey(channel, slug string) string {
	slug = normalizeAgentLaneSlug(slug)
	if slug == "" {
		return ""
	}
	channel = normalizeChannelSlug(channel)
	if channel == "" || channel == "general" {
		return slug
	}
	return channel + agentLaneSeparator + slug
}

func parseAgentLaneKey(key string) (channel string, slug string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "general", ""
	}
	channel, slug, ok := strings.Cut(key, agentLaneSeparator)
	if ok {
		return normalizeChannelSlug(channel), normalizeAgentLaneSlug(slug)
	}
	return "general", normalizeAgentLaneSlug(key)
}

func agentLaneChannel(key string) string {
	channel, _ := parseAgentLaneKey(key)
	return channel
}

func agentLaneSlug(key string) string {
	_, slug := parseAgentLaneKey(key)
	return slug
}

func agentLaneMatchesChannel(key, channel string) bool {
	return agentLaneChannel(key) == normalizeChannelSlug(channel)
}

func normalizeAgentLaneSlug(slug string) string {
	return normalizeChannelSlug(strings.TrimSpace(slug))
}
