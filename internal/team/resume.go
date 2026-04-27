package team

import (
	"fmt"
	"strings"
)

// recentHumanMessageLimit is the number of recent human messages to consider
// when building resume packets. The spec requires the last 50 messages.
const recentHumanMessageLimit = 50

// isHumanOrSystemSender reports whether a message sender is a human or system
// source (not an agent). Only agent replies count as "answers".
func isHumanOrSystemSender(from string) bool {
	f := strings.ToLower(strings.TrimSpace(from))
	return f == "you" || f == "human" || f == "nex" || f == "system" || f == ""
}

// findUnansweredMessages returns the subset of humanMsgs that have received no
// agent reply in allMessages. A human message is considered "answered" only when
// at least one AGENT message (not human/nex/system) in allMessages has ReplyTo
// set to that human message's ID.
func findUnansweredMessages(humanMsgs, allMessages []channelMessage) []channelMessage {
	// Build a set of human message IDs that have been replied to by agents.
	// Skip replies from human/nex/system senders — only agent replies count.
	replied := make(map[string]struct{})
	for _, msg := range allMessages {
		if msg.ReplyTo == "" {
			continue
		}
		if isHumanOrSystemSender(msg.From) {
			continue
		}
		replied[msg.ReplyTo] = struct{}{}
	}

	var out []channelMessage
	for _, hm := range humanMsgs {
		if _, ok := replied[hm.ID]; !ok {
			out = append(out, hm)
		}
	}
	return out
}

func threadRootMessageIDFromMap(msg channelMessage, messageByID map[string]channelMessage) string {
	rootID := strings.TrimSpace(msg.ID)
	parentID := strings.TrimSpace(msg.ReplyTo)
	seen := map[string]struct{}{}
	for parentID != "" {
		if _, ok := seen[parentID]; ok {
			rootID = parentID
			break
		}
		seen[parentID] = struct{}{}
		parent, ok := messageByID[parentID]
		if !ok {
			rootID = parentID
			break
		}
		rootID = strings.TrimSpace(parent.ID)
		parentID = strings.TrimSpace(parent.ReplyTo)
	}
	if rootID == "" {
		rootID = strings.TrimSpace(msg.ID)
	}
	return rootID
}

func collapseUnansweredMessagesByThread(msgs, allMessages []channelMessage) []channelMessage {
	if len(msgs) <= 1 {
		return msgs
	}
	messageByID := make(map[string]channelMessage, len(allMessages))
	for _, msg := range allMessages {
		id := strings.TrimSpace(msg.ID)
		if id == "" {
			continue
		}
		messageByID[id] = msg
	}
	threadKeyFor := func(msg channelMessage) string {
		channel := normalizeChannelSlug(msg.Channel)
		if channel == "" {
			channel = "general"
		}
		rootID := threadRootMessageIDFromMap(msg, messageByID)
		return channel + "|" + rootID
	}

	latestByThread := make(map[string]channelMessage, len(msgs))
	for _, msg := range msgs {
		threadKey := threadKeyFor(msg)
		prior, ok := latestByThread[threadKey]
		if !ok || strings.TrimSpace(prior.Timestamp) < strings.TrimSpace(msg.Timestamp) {
			latestByThread[threadKey] = msg
		}
	}

	out := make([]channelMessage, 0, len(latestByThread))
	seenIDs := make(map[string]struct{}, len(latestByThread))
	for _, msg := range msgs {
		threadKey := threadKeyFor(msg)
		latest := latestByThread[threadKey]
		if strings.TrimSpace(latest.ID) != strings.TrimSpace(msg.ID) {
			continue
		}
		id := strings.TrimSpace(msg.ID)
		if _, ok := seenIDs[id]; ok {
			continue
		}
		seenIDs[id] = struct{}{}
		out = append(out, msg)
	}
	return out
}

func routeResumeMessagesToThreadRoot(msgs, allMessages []channelMessage) []channelMessage {
	if len(msgs) == 0 {
		return nil
	}
	messageByID := make(map[string]channelMessage, len(allMessages))
	for _, msg := range allMessages {
		id := strings.TrimSpace(msg.ID)
		if id == "" {
			continue
		}
		messageByID[id] = msg
	}
	out := make([]channelMessage, 0, len(msgs))
	for _, msg := range msgs {
		replyTargetID := threadRootMessageIDFromMap(msg, messageByID)
		if replyTargetID == "" || replyTargetID == strings.TrimSpace(msg.ID) {
			out = append(out, msg)
			continue
		}
		routed := msg
		routed.ID = replyTargetID
		if rootMsg, ok := messageByID[replyTargetID]; ok {
			rootContent := strings.TrimSpace(rootMsg.Content)
			latestContent := strings.TrimSpace(msg.Content)
			switch {
			case rootContent != "" && latestContent != "" && rootContent != latestContent:
				routed.Content = fmt.Sprintf("Original ask: %s | Latest human follow-up: %s", rootContent, latestContent)
			case rootContent != "":
				routed.Content = rootContent
			}
		}
		out = append(out, routed)
	}
	return out
}

// buildResumePacket constructs a context string that an agent can use to resume
// in-flight work. It combines the agent's assigned tasks (with worktree paths)
// and any unanswered human messages (with channel/reply_to routing instructions).
// Returns an empty string when there is nothing to resume.
func buildResumePacket(slug string, tasks []teamTask, msgs []channelMessage) string {
	if len(tasks) == 0 && len(msgs) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("[Session resumed — picking up where you left off]\n\n")

	if len(tasks) > 0 {
		sb.WriteString("Active tasks:\n")
		for _, task := range tasks {
			runtimeTask := runtimeTaskFromTeamTask(task)
			sb.WriteString(fmt.Sprintf("- [%s] %s (status: %s)\n", task.ID, task.Title, task.Status))
			if task.Details != "" {
				sb.WriteString(fmt.Sprintf("  %s\n", task.Details))
			}
			if path := effectiveTeamTaskWorkspacePath(&task); path != "" {
				sb.WriteString(fmt.Sprintf("  Working directory: %s\n", path))
			}
			if next := runtimeTaskNextAction(runtimeTask); next != "" {
				sb.WriteString(fmt.Sprintf("  Next action: %s\n", next))
			}
			if len(runtimeTask.ChangedPaths) > 0 {
				sb.WriteString(fmt.Sprintf("  Changed paths: %s\n", strings.Join(runtimeTask.ChangedPaths, ", ")))
			}
			if len(runtimeTask.UntrackedPaths) > 0 {
				sb.WriteString(fmt.Sprintf("  Untracked paths: %s\n", strings.Join(runtimeTask.UntrackedPaths, ", ")))
			}
			if blockers := runtimeTaskResumeBlockers(runtimeTask); len(blockers) > 0 {
				sb.WriteString("  Relevant blockers:\n")
				for _, blocker := range blockers {
					sb.WriteString(fmt.Sprintf("  - %s\n", blocker))
				}
			}
			if route := formatResumeRouteInstruction(slug, runtimeTaskReplyChannel(runtimeTask), runtimeTaskReplyTo(runtimeTask), "Route updates"); route != "" {
				sb.WriteString("  " + route + "\n")
			}
		}
		sb.WriteString("\n")
	}

	if len(msgs) > 0 {
		sb.WriteString("Unanswered messages:\n")
		for _, msg := range msgs {
			channel := msg.Channel
			if channel == "" {
				channel = "general"
			}
			sb.WriteString(fmt.Sprintf("- @%s (channel: %q, reply_to_id: %q): %s\n", msg.From, channel, msg.ID, msg.Content))
			if route := formatResumeRouteInstruction(slug, channel, msg.ID, "Route reply"); route != "" {
				sb.WriteString("  " + route + "\n")
			}
		}
		sb.WriteString("\n")
		if len(msgs) == 1 {
			channel := msgs[0].Channel
			if channel == "" {
				channel = "general"
			}
			sb.WriteString(fmt.Sprintf("Reply using team_broadcast with my_slug %q and the exact channel/reply_to_id route shown under the message.\n", slug))
			sb.WriteString(fmt.Sprintf("[WUPHF_REPLY_ROUTE channel=%q reply_to_id=%q]\n", channel, msgs[0].ID))
		} else {
			sb.WriteString(fmt.Sprintf("Reply using team_broadcast with my_slug %q and the exact channel/reply_to_id route shown under each message.\n", slug))
		}
	}

	sb.WriteString("Please pick up where you left off.\n")
	return sb.String()
}

func formatResumeRouteInstruction(slug, channel, replyTo, prefix string) string {
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		channel = "general"
	}
	slug = strings.TrimSpace(slug)
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "Route reply"
	}
	replyTo = strings.TrimSpace(replyTo)
	if replyTo == "" {
		return fmt.Sprintf("%s: team_broadcast using my_slug %q and channel %q.", prefix, slug, channel)
	}
	return fmt.Sprintf("%s: team_broadcast using my_slug %q, channel %q, reply_to_id %q.", prefix, slug, channel, replyTo)
}

// buildResumePackets scans the broker for in-flight tasks and unanswered
// human messages, then builds a resume packet per agent. Routing:
//   - tasks: routed to their owner slug
//   - tagged messages: each tagged agent receives the message
//   - untagged messages: the pack lead receives the message
//
// Only agents in the current pack receive packets. Agents not in the pack
// (e.g. removed members with leftover tasks) are silently skipped.
//
// Returns a map of agent slug → resume packet (empty strings are omitted).
func (l *Launcher) buildResumePackets() map[string]string {
	if l.broker == nil {
		return nil
	}

	// Build the set of valid office agent slugs from the live broker roster, not
	// the original launch pack. Dynamically created specialists must resume after
	// restart just like built-in members.
	officeSlugs := make(map[string]struct{})
	for _, member := range l.officeMembersSnapshot() {
		officeSlugs[member.Slug] = struct{}{}
	}
	inPack := func(slug string) bool {
		if len(officeSlugs) == 0 {
			return true // no roster loaded — allow all (nil-roster safety)
		}
		_, ok := officeSlugs[slug]
		return ok
	}

	// Determine office lead slug.
	lead := l.officeLeadSlug()

	// Collect in-flight tasks per owner — skip owners not in the pack.
	tasksByAgent := make(map[string][]teamTask)
	for _, task := range l.broker.InFlightTasks() {
		if !l.shouldWakeTargetForTask(task.Owner) {
			continue
		}
		if !inPack(task.Owner) {
			continue
		}
		tasksByAgent[task.Owner] = append(tasksByAgent[task.Owner], task)
	}

	// Collect unanswered human messages.
	humanMsgs := l.broker.RecentHumanMessages(recentHumanMessageLimit)
	allMsgs := l.broker.AllMessages()
	unanswered := findUnansweredMessages(humanMsgs, allMsgs)
	unanswered = collapseUnansweredMessagesByThread(unanswered, allMsgs)
	unanswered = routeResumeMessagesToThreadRoot(unanswered, allMsgs)

	// Route unanswered messages: explicit tags → tagged agents; untagged → lead.
	// Skip agents not in the current pack.
	msgsByAgent := make(map[string][]channelMessage)
	for _, msg := range unanswered {
		channel := normalizeChannelSlug(msg.Channel)
		if IsDMSlug(channel) {
			target := DMTargetAgent(channel)
			if target != "" && !isHumanOrSystemSender(target) && inPack(target) && l.messageCanWakeTarget(msg, target) {
				msgsByAgent[target] = append(msgsByAgent[target], msg)
				continue
			}
		}
		if isDM, target := l.isChannelDM(channel); isDM {
			if target != "" && !isHumanOrSystemSender(target) && inPack(target) && l.messageCanWakeTarget(msg, target) {
				msgsByAgent[target] = append(msgsByAgent[target], msg)
				continue
			}
		}
		if len(msg.Tagged) > 0 {
			for _, tag := range msg.Tagged {
				slug := strings.TrimPrefix(tag, "@")
				// Skip human/you tags — those are not agents.
				if isHumanOrSystemSender(slug) {
					continue
				}
				if !inPack(slug) {
					continue
				}
				if !l.messageCanWakeTarget(msg, slug) {
					continue
				}
				msgsByAgent[slug] = append(msgsByAgent[slug], msg)
			}
		} else {
			if lead != "" && inPack(lead) {
				msgsByAgent[lead] = append(msgsByAgent[lead], msg)
			}
		}
	}

	// Build packets — include an agent only if they have tasks or messages.
	allSlugs := make(map[string]struct{})
	for slug := range tasksByAgent {
		allSlugs[slug] = struct{}{}
	}
	for slug := range msgsByAgent {
		allSlugs[slug] = struct{}{}
	}

	packets := make(map[string]string)
	for slug := range allSlugs {
		packet := buildResumePacket(slug, tasksByAgent[slug], msgsByAgent[slug])
		if packet != "" {
			packets[slug] = packet
		}
	}
	return packets
}

func (l *Launcher) buildResumePacketsByLane() map[string]string {
	if l.broker == nil {
		return nil
	}

	officeSlugs := make(map[string]struct{})
	for _, member := range l.officeMembersSnapshot() {
		officeSlugs[member.Slug] = struct{}{}
	}
	inPack := func(slug string) bool {
		if len(officeSlugs) == 0 {
			return true
		}
		_, ok := officeSlugs[slug]
		return ok
	}

	lead := l.officeLeadSlug()
	tasksByLane := make(map[string][]teamTask)
	for _, task := range l.broker.InFlightTasks() {
		if !l.shouldWakeTargetForTask(task.Owner) {
			continue
		}
		if !inPack(task.Owner) {
			continue
		}
		laneKey := agentLaneKey(task.Channel, task.Owner)
		if laneKey == "" {
			continue
		}
		tasksByLane[laneKey] = append(tasksByLane[laneKey], task)
	}

	humanMsgs := l.broker.RecentHumanMessages(recentHumanMessageLimit)
	allMsgs := l.broker.AllMessages()
	unanswered := findUnansweredMessages(humanMsgs, allMsgs)
	unanswered = collapseUnansweredMessagesByThread(unanswered, allMsgs)
	unanswered = routeResumeMessagesToThreadRoot(unanswered, allMsgs)
	msgsByLane := make(map[string][]channelMessage)

	for _, msg := range unanswered {
		channel := normalizeChannelSlug(msg.Channel)
		if channel == "" {
			channel = "general"
		}
		addMessage := func(slug string) {
			laneKey := agentLaneKey(channel, slug)
			if laneKey == "" {
				return
			}
			msgsByLane[laneKey] = append(msgsByLane[laneKey], msg)
		}
		if IsDMSlug(channel) {
			target := DMTargetAgent(channel)
			if target != "" && !isHumanOrSystemSender(target) && inPack(target) && l.messageCanWakeTarget(msg, target) {
				addMessage(target)
				continue
			}
		}
		if isDM, target := l.isChannelDM(channel); isDM {
			if target != "" && !isHumanOrSystemSender(target) && inPack(target) && l.messageCanWakeTarget(msg, target) {
				addMessage(target)
				continue
			}
		}
		if len(msg.Tagged) > 0 {
			for _, tag := range msg.Tagged {
				slug := strings.TrimPrefix(tag, "@")
				if isHumanOrSystemSender(slug) {
					continue
				}
				if !inPack(slug) {
					continue
				}
				if !l.messageCanWakeTarget(msg, slug) {
					continue
				}
				addMessage(slug)
			}
			continue
		}
		if lead != "" && inPack(lead) {
			addMessage(lead)
		}
	}

	allLanes := make(map[string]struct{})
	for laneKey := range tasksByLane {
		allLanes[laneKey] = struct{}{}
	}
	for laneKey := range msgsByLane {
		allLanes[laneKey] = struct{}{}
	}

	packets := make(map[string]string)
	for laneKey := range allLanes {
		slug := agentLaneSlug(laneKey)
		packet := buildResumePacket(slug, tasksByLane[laneKey], msgsByLane[laneKey])
		if packet != "" {
			packets[laneKey] = packet
		}
	}
	return packets
}

// resumeInFlightWork builds resume packets for all agents with pending work and
// delivers them via the appropriate runtime:
//   - Headless (Codex / web mode): enqueueHeadlessCodexTurn
//   - tmux: sendNotificationToPane
//
// In headless mode the lead is enqueued FIRST to avoid the queue-hold guard:
// enqueueHeadlessCodexTurn suppresses lead notifications when any specialist
// queue is non-empty. Enqueuing the lead before specialists ensures the lead's
// resume packet is not silently dropped at startup.
func (l *Launcher) resumeInFlightWork() {
	packets := l.buildResumePacketsByLane()
	if len(packets) == 0 {
		return
	}

	if l.usesCodexRuntime() || l.webMode {
		lead := l.officeLeadSlug()
		// Enqueue lead first to bypass the queue-hold guard.
		for laneKey, packet := range packets {
			slug := agentLaneSlug(laneKey)
			if slug != lead {
				continue
			}
			l.enqueueHeadlessCodexTurn(slug, packet, agentLaneChannel(laneKey))
		}
		for laneKey, packet := range packets {
			slug := agentLaneSlug(laneKey)
			if slug == lead {
				continue
			}
			l.enqueueHeadlessCodexTurn(slug, packet, agentLaneChannel(laneKey))
		}
		return
	}

	// tmux path — need pane targets.
	paneTargets := l.agentPaneTargets()
	for laneKey, packet := range packets {
		slug := agentLaneSlug(laneKey)
		target, ok := paneTargets[slug]
		if !ok {
			continue
		}
		l.sendNotificationToPane(target.PaneTarget, packet)
	}
}
