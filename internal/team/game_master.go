package team

import "strings"

const ownerOnlyCallableAgentSlug = "game-master"

func isOwnerOnlyCallableAgent(slug string) bool {
	return normalizeChannelSlug(slug) == ownerOnlyCallableAgentSlug
}

func isOwnerInvocationSender(from string) bool {
	return normalizeActorSlug(strings.TrimSpace(from)) == "you"
}

func isOwnerOnlyCallableDMTarget(channelSlug, targetSlug string, isChannelDM func(string) (bool, string)) bool {
	targetSlug = normalizeChannelSlug(targetSlug)
	if !isOwnerOnlyCallableAgent(targetSlug) {
		return false
	}
	channelSlug = normalizeChannelSlug(channelSlug)
	if IsDMSlug(channelSlug) {
		return normalizeChannelSlug(DMTargetAgent(channelSlug)) == targetSlug
	}
	if isChannelDM == nil {
		return false
	}
	isDM, dmTarget := isChannelDM(channelSlug)
	return isDM && normalizeChannelSlug(dmTarget) == targetSlug
}
