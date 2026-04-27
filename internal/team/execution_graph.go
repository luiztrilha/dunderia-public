package team

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const executionNodeDefaultTimeout = 4 * time.Minute
const executionNodeHistoryRetention = 24 * time.Hour

type executionNode struct {
	ID                   string   `json:"id"`
	Channel              string   `json:"channel,omitempty"`
	RootMessageID        string   `json:"root_message_id,omitempty"`
	ParentNodeID         string   `json:"parent_node_id,omitempty"`
	TriggerMessageID     string   `json:"trigger_message_id,omitempty"`
	OwnerAgent           string   `json:"owner_agent,omitempty"`
	Status               string   `json:"status,omitempty"`
	ExpectedResponseKind string   `json:"expected_response_kind,omitempty"`
	ExpectedFrom         []string `json:"expected_from,omitempty"`
	TimeoutAt            string   `json:"timeout_at,omitempty"`
	AttemptCount         int      `json:"attempt_count,omitempty"`
	ResolvedByMessageID  string   `json:"resolved_by_message_id,omitempty"`
	ResolvedByAgent      string   `json:"resolved_by_agent,omitempty"`
	SupersedesNodeID     string   `json:"supersedes_node_id,omitempty"`
	AwaitingHumanInput   bool     `json:"awaiting_human_input,omitempty"`
	AwaitingHumanSince   string   `json:"awaiting_human_since,omitempty"`
	AwaitingHumanReason  string   `json:"awaiting_human_reason,omitempty"`
	LastError            string   `json:"last_error,omitempty"`
	CreatedAt            string   `json:"created_at,omitempty"`
	UpdatedAt            string   `json:"updated_at,omitempty"`
}

func normalizeExecutionNodeStatus(status string) string {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "pending", "answered", "needs_correction", "timed_out", "fallback_dispatched", "blocked", "closed", "cancelled", "canceled":
		return strings.TrimSpace(strings.ToLower(status))
	default:
		return "pending"
	}
}

func executionNodeIsOpen(node executionNode) bool {
	switch normalizeExecutionNodeStatus(node.Status) {
	case "pending", "needs_correction", "fallback_dispatched":
		return true
	default:
		return false
	}
}

func isHumanLikeActor(slug string) bool {
	switch strings.TrimSpace(slug) {
	case "", "you", "human", "nex":
		return true
	default:
		return false
	}
}

func isSystemActor(slug string) bool {
	return strings.TrimSpace(slug) == "system"
}

func expectsExecutionFollowup(msg channelMessage) bool {
	if messageKindSuppressesOfficeWake(msg.Kind) {
		return false
	}
	return isSubstantiveChannelMemoryText(strings.TrimSpace(msg.Title + ": " + msg.Content))
}

func parseExecutionNodeSequence(id string) int64 {
	id = strings.TrimSpace(id)
	if !strings.HasPrefix(id, "exec-") {
		return 0
	}
	seq, err := strconv.ParseInt(strings.TrimPrefix(id, "exec-"), 10, 64)
	if err != nil || seq < 1 {
		return 0
	}
	return seq
}

func (b *Broker) syncExecutionNodeCounterLocked() {
	for _, node := range b.executionNodes {
		if seq := parseExecutionNodeSequence(node.ID); seq > b.executionNodeSeq {
			b.executionNodeSeq = seq
		}
	}
}

func (b *Broker) nextExecutionNodeIDLocked() string {
	b.syncExecutionNodeCounterLocked()
	b.executionNodeSeq++
	return fmt.Sprintf("exec-%d", b.executionNodeSeq)
}

func (b *Broker) findMessageByIDLocked(id string) *channelMessage {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	for i := len(b.messages) - 1; i >= 0; i-- {
		if strings.TrimSpace(b.messages[i].ID) == id {
			return &b.messages[i]
		}
	}
	return nil
}

func (b *Broker) findExecutionNodeByIDLocked(id string) *executionNode {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	for i := range b.executionNodes {
		if strings.TrimSpace(b.executionNodes[i].ID) == id {
			return &b.executionNodes[i]
		}
	}
	return nil
}

func (b *Broker) threadRootFromMessageIDLocked(startID string) string {
	root := strings.TrimSpace(startID)
	if root == "" {
		return ""
	}
	for depth := 0; depth < 16; depth++ {
		msg := b.findMessageByIDLocked(root)
		if msg == nil {
			break
		}
		parent := strings.TrimSpace(msg.ReplyTo)
		if parent == "" {
			break
		}
		root = parent
	}
	return root
}

func (b *Broker) expectedOwnerForHumanMessageLocked(channel string) string {
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		channel = "general"
	}
	if isDM, target := b.channelIsDMLocked(channel); isDM && strings.TrimSpace(target) != "" {
		return target
	}
	return "ceo"
}

func (b *Broker) channelIsDMLocked(channel string) (bool, string) {
	ch := b.findChannelLocked(channel)
	if ch == nil || !ch.isDM() {
		return false, ""
	}
	return true, DMTargetAgent(ch.Slug)
}

func (b *Broker) parentExecutionNodeForMessageLocked(msg channelMessage) *executionNode {
	replyTo := strings.TrimSpace(msg.ReplyTo)
	if replyTo == "" {
		return nil
	}
	for i := len(b.executionNodes) - 1; i >= 0; i-- {
		node := &b.executionNodes[i]
		if normalizeChannelSlug(node.Channel) != normalizeChannelSlug(msg.Channel) {
			continue
		}
		if strings.TrimSpace(node.TriggerMessageID) == replyTo || strings.TrimSpace(node.RootMessageID) == replyTo || strings.TrimSpace(node.ResolvedByMessageID) == replyTo {
			return node
		}
	}
	return nil
}

func (b *Broker) findExecutionAnchorNodeLocked(channel, triggerMsgID, threadRootID, owner string) *executionNode {
	channel = normalizeChannelSlug(channel)
	triggerMsgID = strings.TrimSpace(triggerMsgID)
	threadRootID = firstNonEmpty(strings.TrimSpace(threadRootID), b.threadRootFromMessageIDLocked(triggerMsgID))
	owner = strings.TrimSpace(owner)

	bestIdx := -1
	bestScore := 0
	for i := len(b.executionNodes) - 1; i >= 0; i-- {
		node := &b.executionNodes[i]
		if channel != "" && normalizeChannelSlug(node.Channel) != channel {
			continue
		}
		score := 0
		if owner != "" && strings.TrimSpace(node.OwnerAgent) == owner {
			score += 40
		}
		if triggerMsgID != "" {
			switch {
			case strings.TrimSpace(node.TriggerMessageID) == triggerMsgID:
				score += 30
			case strings.TrimSpace(node.ResolvedByMessageID) == triggerMsgID:
				score += 25
			case strings.TrimSpace(node.RootMessageID) == triggerMsgID:
				score += 20
			}
		}
		if threadRootID != "" && strings.TrimSpace(node.RootMessageID) == threadRootID {
			score += 10
		}
		if executionNodeIsOpen(*node) {
			score += 2
		}
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	if bestIdx < 0 || bestScore == 0 {
		return nil
	}
	return &b.executionNodes[bestIdx]
}

func (b *Broker) executionBranchMessageIDsLocked(channel, triggerMsgID, threadRootID, owner string) map[string]struct{} {
	result := make(map[string]struct{})
	node := b.findExecutionAnchorNodeLocked(channel, triggerMsgID, threadRootID, owner)
	if node == nil {
		return result
	}
	seenNodes := map[string]struct{}{}
	for current := node; current != nil; current = b.findExecutionNodeByIDLocked(current.ParentNodeID) {
		if _, seen := seenNodes[current.ID]; seen {
			break
		}
		seenNodes[current.ID] = struct{}{}
		for _, id := range []string{current.RootMessageID, current.TriggerMessageID, current.ResolvedByMessageID} {
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				result[trimmed] = struct{}{}
			}
		}
		if strings.TrimSpace(current.ParentNodeID) == "" {
			break
		}
	}
	return result
}

func (b *Broker) findMatchingOpenExecutionNodeForReplyLocked(channel, owner, replyTo string) *executionNode {
	channel = normalizeChannelSlug(channel)
	owner = strings.TrimSpace(owner)
	replyTo = strings.TrimSpace(replyTo)
	if channel == "" || owner == "" || replyTo == "" {
		return nil
	}
	for i := len(b.executionNodes) - 1; i >= 0; i-- {
		node := &b.executionNodes[i]
		if normalizeChannelSlug(node.Channel) != channel || strings.TrimSpace(node.OwnerAgent) != owner {
			continue
		}
		if !executionNodeIsOpen(*node) {
			continue
		}
		if strings.TrimSpace(node.TriggerMessageID) == replyTo || strings.TrimSpace(node.RootMessageID) == replyTo {
			return node
		}
	}
	return nil
}

func (b *Broker) openExecutionNodesForOwnerLocked(channel, owner string) []executionNode {
	channel = normalizeChannelSlug(channel)
	owner = strings.TrimSpace(owner)
	if channel == "" || owner == "" {
		return nil
	}
	out := make([]executionNode, 0)
	for _, node := range b.executionNodes {
		if normalizeChannelSlug(node.Channel) != channel || strings.TrimSpace(node.OwnerAgent) != owner {
			continue
		}
		if executionNodeIsOpen(node) {
			out = append(out, node)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out
}

func (b *Broker) findOpenExecutionNodeForRootOwnerLocked(channel, rootID, owner string) *executionNode {
	channel = normalizeChannelSlug(channel)
	rootID = strings.TrimSpace(rootID)
	owner = strings.TrimSpace(owner)
	if channel == "" || rootID == "" || owner == "" {
		return nil
	}
	for i := len(b.executionNodes) - 1; i >= 0; i-- {
		node := &b.executionNodes[i]
		if normalizeChannelSlug(node.Channel) != channel || strings.TrimSpace(node.OwnerAgent) != owner {
			continue
		}
		if strings.TrimSpace(node.RootMessageID) != rootID {
			continue
		}
		if !executionNodeIsOpen(*node) {
			continue
		}
		return node
	}
	return nil
}

func (b *Broker) impliedExecutionNodeForOwnerLocked(channel, owner string) *executionNode {
	open := b.openExecutionNodesForOwnerLocked(channel, owner)
	if len(open) != 1 {
		return nil
	}
	triggerMsg := b.findMessageByIDLocked(strings.TrimSpace(open[0].TriggerMessageID))
	if triggerMsg != nil && strings.TrimSpace(triggerMsg.ReplyTo) != "" {
		return nil
	}
	for i := range b.executionNodes {
		if b.executionNodes[i].ID == open[0].ID {
			return &b.executionNodes[i]
		}
	}
	return nil
}

func (b *Broker) answerExecutionNodeLocked(node *executionNode, msg channelMessage) {
	if node == nil {
		return
	}
	node.Status = "answered"
	node.ResolvedByMessageID = strings.TrimSpace(msg.ID)
	node.ResolvedByAgent = strings.TrimSpace(msg.From)
	node.UpdatedAt = strings.TrimSpace(msg.Timestamp)
	node.LastError = ""
}

func (b *Broker) markExecutionBranchAwaitingHumanLocked(msg channelMessage) {
	anchor := b.parentExecutionNodeForMessageLocked(msg)
	if anchor == nil {
		return
	}
	now := strings.TrimSpace(msg.Timestamp)
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	reason := truncateSummary(strings.TrimSpace(msg.Content), 200)
	for current := anchor; current != nil; current = b.findExecutionNodeByIDLocked(current.ParentNodeID) {
		current.AwaitingHumanInput = true
		current.AwaitingHumanSince = now
		current.AwaitingHumanReason = reason
		current.UpdatedAt = now
		if strings.TrimSpace(current.ParentNodeID) == "" {
			break
		}
	}
}

func (b *Broker) clearExecutionAwaitingHumanLocked(channel, threadRootID string) {
	channel = normalizeChannelSlug(channel)
	threadRootID = strings.TrimSpace(threadRootID)
	if channel == "" || threadRootID == "" {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for i := range b.executionNodes {
		node := &b.executionNodes[i]
		if normalizeChannelSlug(node.Channel) != channel {
			continue
		}
		if strings.TrimSpace(node.RootMessageID) != threadRootID {
			continue
		}
		node.AwaitingHumanInput = false
		node.AwaitingHumanSince = ""
		node.AwaitingHumanReason = ""
		node.UpdatedAt = now
	}
}

func (b *Broker) findAwaitingHumanExecutionNodeLocked(channel, replyTo string) *executionNode {
	channel = normalizeChannelSlug(channel)
	replyTo = strings.TrimSpace(replyTo)
	if channel == "" || replyTo == "" {
		return nil
	}
	threadRootID := firstNonEmpty(b.threadRootFromMessageIDLocked(replyTo), replyTo)
	for i := len(b.executionNodes) - 1; i >= 0; i-- {
		node := &b.executionNodes[i]
		if normalizeChannelSlug(node.Channel) != channel || !node.AwaitingHumanInput {
			continue
		}
		if strings.TrimSpace(node.RootMessageID) == threadRootID ||
			strings.TrimSpace(node.TriggerMessageID) == replyTo ||
			strings.TrimSpace(node.ResolvedByMessageID) == replyTo {
			return node
		}
	}
	return nil
}

func (b *Broker) createExecutionNodeLocked(msg channelMessage, owner, parentNodeID, rootID, expectedKind string) *executionNode {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return nil
	}
	now := strings.TrimSpace(msg.Timestamp)
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	rootID = strings.TrimSpace(rootID)
	if rootID == "" {
		rootID = strings.TrimSpace(msg.ID)
	}
	node := executionNode{
		ID:                   b.nextExecutionNodeIDLocked(),
		Channel:              normalizeChannelSlug(msg.Channel),
		RootMessageID:        rootID,
		ParentNodeID:         strings.TrimSpace(parentNodeID),
		TriggerMessageID:     strings.TrimSpace(msg.ID),
		OwnerAgent:           owner,
		Status:               "pending",
		ExpectedResponseKind: strings.TrimSpace(expectedKind),
		ExpectedFrom:         []string{owner},
		TimeoutAt:            time.Now().UTC().Add(executionNodeDefaultTimeout).Format(time.RFC3339),
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	b.executionNodes = append(b.executionNodes, node)
	return &b.executionNodes[len(b.executionNodes)-1]
}

func (b *Broker) advanceExecutionGraphForMessageLocked(msg channelMessage) {
	if b == nil {
		return
	}
	now := strings.TrimSpace(msg.Timestamp)
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	if isHumanLikeActor(msg.From) {
		threadRootID := firstNonEmpty(
			b.threadRootFromMessageIDLocked(strings.TrimSpace(msg.ID)),
			b.threadRootFromMessageIDLocked(strings.TrimSpace(msg.ReplyTo)),
			strings.TrimSpace(msg.ID),
			strings.TrimSpace(msg.ReplyTo),
		)
		b.clearExecutionAwaitingHumanLocked(msg.Channel, threadRootID)
	}
	parent := b.findMatchingOpenExecutionNodeForReplyLocked(msg.Channel, msg.From, msg.ReplyTo)
	if parent == nil && strings.TrimSpace(msg.ReplyTo) == "" {
		parent = b.impliedExecutionNodeForOwnerLocked(msg.Channel, msg.From)
	}
	if parent != nil {
		b.answerExecutionNodeLocked(parent, msg)
	}
	if !isHumanLikeActor(msg.From) && messageLooksLikeAwaitingHumanInput(msg.Content) {
		b.markExecutionBranchAwaitingHumanLocked(msg)
	}
	if !expectsExecutionFollowup(msg) {
		return
	}
	switch strings.TrimSpace(msg.From) {
	case "you", "human":
		owner := b.expectedOwnerForHumanMessageLocked(msg.Channel)
		parentNodeID := ""
		rootID := strings.TrimSpace(msg.ID)
		if ancestor := b.parentExecutionNodeForMessageLocked(msg); ancestor != nil {
			parentNodeID = ancestor.ID
			rootID = firstNonEmpty(strings.TrimSpace(ancestor.RootMessageID), b.threadRootFromMessageIDLocked(strings.TrimSpace(msg.ReplyTo)), strings.TrimSpace(msg.ID))
		}
		b.createExecutionNodeLocked(msg, owner, parentNodeID, rootID, "answer")
	case "ceo":
		if len(msg.Tagged) == 0 {
			return
		}
		parentNodeID := ""
		rootID := firstNonEmpty(b.threadRootFromMessageIDLocked(strings.TrimSpace(msg.ReplyTo)), strings.TrimSpace(msg.ID))
		if ancestor := b.parentExecutionNodeForMessageLocked(msg); ancestor != nil {
			parentNodeID = ancestor.ID
			rootID = firstNonEmpty(strings.TrimSpace(ancestor.RootMessageID), rootID, strings.TrimSpace(msg.ID))
		}
		for _, slug := range uniqueSlugs(msg.Tagged) {
			if isHumanLikeActor(slug) || isSystemActor(slug) {
				continue
			}
			if existing := b.findOpenExecutionNodeForRootOwnerLocked(msg.Channel, rootID, slug); existing != nil {
				existing.UpdatedAt = now
				continue
			}
			b.createExecutionNodeLocked(msg, slug, parentNodeID, rootID, "reply")
		}
	}
}

func (b *Broker) validateExecutionReplyLocked(from, channel, content, replyTo string) error {
	if b == nil || isHumanLikeActor(from) || isSystemActor(from) {
		return nil
	}
	replyTo = strings.TrimSpace(replyTo)
	if awaiting := b.findAwaitingHumanExecutionNodeLocked(channel, replyTo); awaiting != nil {
		return fmt.Errorf("thread awaiting human input: @%s must wait for a human reply before posting again in #%s", from, normalizeChannelSlug(channel))
	}
	open := b.openExecutionNodesForOwnerLocked(channel, from)
	if len(open) == 0 {
		if replyTo != "" {
			if prior := b.findRecentAgentReplyInThreadLocked(from, channel, replyTo); prior != nil {
				parent := b.findMessageByIDLocked(replyTo)
				if parent == nil || !isHumanLikeActor(parent.From) {
					return fmt.Errorf("already replied in thread %s: @%s must wait for a new human or lead turn before posting again in #%s", replyTo, from, normalizeChannelSlug(channel))
				}
			}
		}
		return nil
	}
	if replyTo == "" {
		if implied := b.impliedExecutionNodeForOwnerLocked(channel, from); implied != nil {
			return nil
		}
		expected := strings.TrimSpace(open[0].TriggerMessageID)
		if expected == "" {
			expected = strings.TrimSpace(open[0].RootMessageID)
		}
		return fmt.Errorf("reply_to required: @%s has an open execution node in #%s and must reply in thread %s", from, normalizeChannelSlug(channel), expected)
	}
	for _, node := range open {
		if strings.TrimSpace(node.TriggerMessageID) == replyTo || strings.TrimSpace(node.RootMessageID) == replyTo {
			return nil
		}
	}
	expected := strings.TrimSpace(open[0].TriggerMessageID)
	if expected == "" {
		expected = strings.TrimSpace(open[0].RootMessageID)
	}
	return fmt.Errorf("reply_to mismatch: @%s has an open execution node in #%s and must reply to %s", from, normalizeChannelSlug(channel), expected)
}

func (b *Broker) findExecutionNodeForTaskLocked(task *teamTask, owner string) *executionNode {
	if b == nil || task == nil {
		return nil
	}
	channel := normalizeChannelSlug(task.Channel)
	threadID := strings.TrimSpace(task.ThreadID)
	owner = strings.TrimSpace(owner)
	for i := len(b.executionNodes) - 1; i >= 0; i-- {
		node := &b.executionNodes[i]
		if normalizeChannelSlug(node.Channel) != channel || strings.TrimSpace(node.OwnerAgent) != owner {
			continue
		}
		if threadID != "" && (strings.TrimSpace(node.TriggerMessageID) == threadID || strings.TrimSpace(node.RootMessageID) == threadID) {
			return node
		}
	}
	return nil
}

func (b *Broker) markExecutionNodeFallbackLocked(task *teamTask, owner, detail string, attempts int) {
	node := b.findExecutionNodeForTaskLocked(task, owner)
	if node == nil {
		return
	}
	node.Status = "fallback_dispatched"
	if attempts > node.AttemptCount {
		node.AttemptCount = attempts
	}
	node.LastError = strings.TrimSpace(detail)
	node.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
}

func (b *Broker) markExecutionNodeTimedOutLocked(task *teamTask, owner, detail string) {
	node := b.findExecutionNodeForTaskLocked(task, owner)
	if node == nil {
		return
	}
	node.Status = "timed_out"
	node.LastError = strings.TrimSpace(detail)
	node.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
}

func (b *Broker) reopenExecutionNodeForTaskLocked(task *teamTask, owner, now string) {
	node := b.findExecutionNodeForTaskLocked(task, owner)
	if node == nil {
		return
	}
	switch normalizeExecutionNodeStatus(node.Status) {
	case "timed_out", "fallback_dispatched", "blocked":
	default:
		return
	}
	now = strings.TrimSpace(now)
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	node.Status = "pending"
	node.TimeoutAt = time.Now().UTC().Add(executionNodeDefaultTimeout).Format(time.RFC3339)
	node.UpdatedAt = now
	node.LastError = ""
	node.ResolvedByMessageID = ""
	node.ResolvedByAgent = ""
	node.AwaitingHumanInput = false
	node.AwaitingHumanSince = ""
	node.AwaitingHumanReason = ""
	if strings.TrimSpace(owner) != "" {
		node.ExpectedFrom = []string{strings.TrimSpace(owner)}
	}
}

func (b *Broker) normalizeExecutionNodesLocked() {
	if len(b.executionNodes) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(b.executionNodes))
	normalized := make([]executionNode, 0, len(b.executionNodes))
	for _, node := range b.executionNodes {
		node.ID = strings.TrimSpace(node.ID)
		if node.ID == "" {
			node.ID = b.nextExecutionNodeIDLocked()
		}
		if _, ok := seen[node.ID]; ok {
			continue
		}
		seen[node.ID] = struct{}{}
		node.Channel = normalizeChannelSlug(node.Channel)
		if node.Channel == "" {
			node.Channel = "general"
		}
		node.RootMessageID = strings.TrimSpace(node.RootMessageID)
		node.ParentNodeID = strings.TrimSpace(node.ParentNodeID)
		node.TriggerMessageID = strings.TrimSpace(node.TriggerMessageID)
		node.OwnerAgent = strings.TrimSpace(node.OwnerAgent)
		node.Status = normalizeExecutionNodeStatus(node.Status)
		node.ExpectedResponseKind = strings.TrimSpace(node.ExpectedResponseKind)
		node.ExpectedFrom = uniqueSlugs(node.ExpectedFrom)
		node.ResolvedByMessageID = strings.TrimSpace(node.ResolvedByMessageID)
		node.ResolvedByAgent = strings.TrimSpace(node.ResolvedByAgent)
		node.SupersedesNodeID = strings.TrimSpace(node.SupersedesNodeID)
		node.AwaitingHumanSince = strings.TrimSpace(node.AwaitingHumanSince)
		node.AwaitingHumanReason = strings.TrimSpace(node.AwaitingHumanReason)
		node.LastError = strings.TrimSpace(node.LastError)
		if strings.TrimSpace(node.CreatedAt) == "" {
			node.CreatedAt = time.Now().UTC().Format(time.RFC3339)
		}
		if strings.TrimSpace(node.UpdatedAt) == "" {
			node.UpdatedAt = node.CreatedAt
		}
		normalized = append(normalized, node)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].CreatedAt < normalized[j].CreatedAt
	})
	normalized = pruneOldExecutionNodes(normalized)
	b.executionNodes = normalized
	b.syncExecutionNodeCounterLocked()
}

func pruneOldExecutionNodes(nodes []executionNode) []executionNode {
	if len(nodes) == 0 {
		return nodes
	}
	keepAfter := time.Now().UTC().Add(-executionNodeHistoryRetention)
	out := make([]executionNode, 0, len(nodes))
	for _, node := range nodes {
		node.OwnerAgent = strings.TrimSpace(node.OwnerAgent)
		node.Channel = normalizeChannelSlug(node.Channel)
		if executionNodeIsOpen(node) {
			out = append(out, node)
			continue
		}
		t := executionNodeUpdatedAt(node)
		if t.IsZero() {
			continue
		}
		if t.Before(keepAfter) {
			continue
		}
		out = append(out, node)
	}
	if len(out) == len(nodes) {
		return out
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt < out[j].CreatedAt
	})
	return out
}

func executionNodeUpdatedAt(node executionNode) time.Time {
	ts := strings.TrimSpace(node.UpdatedAt)
	if ts == "" {
		ts = strings.TrimSpace(node.CreatedAt)
	}
	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		time.DateTime,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05.000000000",
		"2006-01-02 15:04:05.000000000-07:00",
	}
	for _, layout := range layouts {
		t, err := time.Parse(layout, ts)
		if err == nil {
			return t
		}
	}
	t, err := time.Parse(time.RFC3339Nano, strings.TrimSuffix(ts, "Z"))
	if err == nil {
		return t
	}
	t, err = time.ParseInLocation("2006-01-02T15:04:05", ts, time.Local)
	if err == nil {
		return t
	}
	t, err = time.ParseInLocation("2006-01-02T15:04:05.000000000", ts, time.Local)
	if err == nil {
		return t
	}
	return time.Time{}
}

func (b *Broker) ExecutionBranchMessageIDs(channel, triggerMsgID, threadRootID, owner string) map[string]struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	branch := b.executionBranchMessageIDsLocked(channel, triggerMsgID, threadRootID, owner)
	if len(branch) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(branch))
	for id := range branch {
		out[id] = struct{}{}
	}
	return out
}

func (b *Broker) ChannelExecutionNodes(channel, threadRootID string) []executionNode {
	b.mu.Lock()
	defer b.mu.Unlock()
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		channel = "general"
	}
	threadRootID = firstNonEmpty(strings.TrimSpace(threadRootID), b.threadRootFromMessageIDLocked(threadRootID))
	out := make([]executionNode, 0, len(b.executionNodes))
	for _, node := range b.executionNodes {
		if normalizeChannelSlug(node.Channel) != channel {
			continue
		}
		if threadRootID != "" && strings.TrimSpace(node.RootMessageID) != threadRootID {
			continue
		}
		copyNode := node
		copyNode.ExpectedFrom = append([]string(nil), node.ExpectedFrom...)
		out = append(out, copyNode)
	}
	return out
}
