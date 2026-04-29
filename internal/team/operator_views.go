package team

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/nex-crm/wuphf/internal/orchestration"
)

type deliveryArtifact struct {
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Summary   string `json:"summary,omitempty"`
	State     string `json:"state,omitempty"`
	Path      string `json:"path,omitempty"`
	URL       string `json:"url,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	RelatedID string `json:"related_id,omitempty"`
}

type deliveryView struct {
	ID                      string             `json:"id"`
	Title                   string             `json:"title"`
	Summary                 string             `json:"summary,omitempty"`
	Status                  string             `json:"status"`
	Owner                   string             `json:"owner,omitempty"`
	Channel                 string             `json:"channel,omitempty"`
	WorkspacePath           string             `json:"workspace_path,omitempty"`
	ProgressPercent         int                `json:"progress_percent,omitempty"`
	ProgressBasis           string             `json:"progress_basis,omitempty"`
	LastSubstantiveUpdateAt string             `json:"last_substantive_update_at,omitempty"`
	LastSubstantiveUpdateBy string             `json:"last_substantive_update_by,omitempty"`
	LastSubstantiveSummary  string             `json:"last_substantive_summary,omitempty"`
	PendingHumanCount       int                `json:"pending_human_count,omitempty"`
	BlockerCount            int                `json:"blocker_count,omitempty"`
	TaskIDs                 []string           `json:"task_ids,omitempty"`
	RequestIDs              []string           `json:"request_ids,omitempty"`
	Artifacts               []deliveryArtifact `json:"artifacts,omitempty"`
}

type deliveryAccumulator struct {
	view           deliveryView
	taskIDs        map[string]struct{}
	requestIDs     map[string]struct{}
	artifactKeys   map[string]struct{}
	totalSteps     int
	completedSteps int
	hasInProgress  bool
	hasReview      bool
	hasBlocked     bool
	hasDone        bool
	workspaceScore int
	workspaceAt    string
}

var artifactPathPattern = regexp.MustCompile(`(?i)([A-Z]:\\[^\r\n"]+\.(?:md|txt|json|pdf|docx|xlsx|csv|tsv))`)

var deliveryWorkspaceRepoRootMarkers = map[string]struct{}{
	"repos":           {},
	"repositórios":    {},
	"repositorios":    {},
	"repository":      {},
	"repositories":    {},
	"workspace-repos": {},
}

func (b *Broker) handleDeliveries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	allChannels := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all_channels")), "true")
	includeDone := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_done")), "true")
	viewerSlug := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	if channel == "" && !allChannels {
		channel = "general"
	}

	b.mu.Lock()
	if !allChannels && !b.canAccessChannelLocked(viewerSlug, channel) {
		b.mu.Unlock()
		http.Error(w, "channel access denied", http.StatusForbidden)
		return
	}
	deliveries := b.buildDeliveriesLocked(channel, allChannels, includeDone)
	b.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"channel":    channel,
		"deliveries": deliveries,
	})
}

func (b *Broker) buildOperatorTasksLocked(channel string, allChannels, includeDone bool, statusFilter, mySlug string) []teamTask {
	progressByDelivery := b.deliveryProgressIndexLocked(channel, allChannels, true)
	result := make([]teamTask, 0, len(b.tasks)+len(b.requests)+len(b.executionNodes))
	requestRoots := make(map[string]struct{}, len(b.requests))

	for _, task := range b.tasks {
		if !allChannels && normalizeChannelSlug(task.Channel) != channel {
			continue
		}
		if taskIsTerminal(&task) && !includeDone && statusFilter == "" {
			continue
		}
		if statusFilter != "" && strings.TrimSpace(task.Status) != statusFilter {
			continue
		}
		if mySlug != "" && strings.TrimSpace(task.Owner) != "" && strings.TrimSpace(task.Owner) != mySlug {
			continue
		}
		copyTask := task
		copyTask.SourceTaskID = firstNonEmpty(copyTask.SourceTaskID, derivedSourceTaskID(copyTask))
		copyTask.DeliveryID = b.deliveryKeyForTaskLocked(copyTask)
		applyDeliveryProgress(&copyTask, progressByDelivery)
		result = append(result, copyTask)
	}

	includeHuman := mySlug == "" || mySlug == "human" || mySlug == "you"
	if !includeHuman {
		return result
	}

	for _, req := range b.requests {
		if !requestIsActive(req) {
			continue
		}
		reqChannel := normalizeChannelSlug(req.Channel)
		if reqChannel == "" {
			reqChannel = "general"
		}
		if !allChannels && reqChannel != channel {
			continue
		}
		viewTask := b.humanActionTaskFromRequestLocked(req)
		if viewTask.ID == "" {
			continue
		}
		if statusFilter != "" && strings.TrimSpace(viewTask.Status) != statusFilter {
			continue
		}
		applyDeliveryProgress(&viewTask, progressByDelivery)
		result = append(result, viewTask)
		threadRoot := b.threadKeyForRequestLocked(req)
		if threadRoot != "" {
			requestRoots[reqChannel+"|"+threadRoot] = struct{}{}
		}
	}

	for _, node := range b.executionNodes {
		if !node.AwaitingHumanInput || !executionNodeIsOpen(node) {
			continue
		}
		nodeChannel := normalizeChannelSlug(node.Channel)
		if nodeChannel == "" {
			nodeChannel = "general"
		}
		if !allChannels && nodeChannel != channel {
			continue
		}
		threadRoot := firstNonEmpty(strings.TrimSpace(node.RootMessageID), strings.TrimSpace(node.TriggerMessageID))
		if threadRoot != "" {
			if _, ok := requestRoots[nodeChannel+"|"+threadRoot]; ok {
				continue
			}
		}
		viewTask := b.humanActionTaskFromExecutionNodeLocked(node)
		if viewTask.ID == "" {
			continue
		}
		if statusFilter != "" && strings.TrimSpace(viewTask.Status) != statusFilter {
			continue
		}
		applyDeliveryProgress(&viewTask, progressByDelivery)
		result = append(result, viewTask)
	}

	return result
}

func (b *Broker) buildOperatorTasksLiteLocked(channel string, allChannels, includeDone bool, statusFilter, mySlug string) []teamTask {
	result := make([]teamTask, 0, len(b.tasks)+len(b.requests)+len(b.executionNodes))
	requestRoots := make(map[string]struct{}, len(b.requests))

	for _, task := range b.tasks {
		if !allChannels && normalizeChannelSlug(task.Channel) != channel {
			continue
		}
		if taskIsTerminal(&task) && !includeDone && statusFilter == "" {
			continue
		}
		if statusFilter != "" && strings.TrimSpace(task.Status) != statusFilter {
			continue
		}
		if mySlug != "" && strings.TrimSpace(task.Owner) != "" && strings.TrimSpace(task.Owner) != mySlug {
			continue
		}
		copyTask := task
		copyTask.SourceTaskID = firstNonEmpty(copyTask.SourceTaskID, derivedSourceTaskID(copyTask))
		result = append(result, copyTask)
	}

	includeHuman := mySlug == "" || mySlug == "human" || mySlug == "you"
	if !includeHuman {
		return result
	}

	for _, req := range b.requests {
		if !requestIsActive(req) {
			continue
		}
		reqChannel := normalizeChannelSlug(req.Channel)
		if reqChannel == "" {
			reqChannel = "general"
		}
		if !allChannels && reqChannel != channel {
			continue
		}
		viewTask := liteHumanActionTaskFromRequest(req)
		if viewTask.ID == "" {
			continue
		}
		if statusFilter != "" && strings.TrimSpace(viewTask.Status) != statusFilter {
			continue
		}
		result = append(result, viewTask)
		if threadRoot := firstNonEmpty(b.threadRootFromMessageIDLocked(strings.TrimSpace(req.ReplyTo)), strings.TrimSpace(req.ReplyTo)); threadRoot != "" {
			requestRoots[reqChannel+"|"+threadRoot] = struct{}{}
		}
	}

	for _, node := range b.executionNodes {
		if !node.AwaitingHumanInput || !executionNodeIsOpen(node) {
			continue
		}
		nodeChannel := normalizeChannelSlug(node.Channel)
		if nodeChannel == "" {
			nodeChannel = "general"
		}
		if !allChannels && nodeChannel != channel {
			continue
		}
		threadRoot := firstNonEmpty(strings.TrimSpace(node.RootMessageID), strings.TrimSpace(node.TriggerMessageID))
		if threadRoot != "" {
			if _, ok := requestRoots[nodeChannel+"|"+threadRoot]; ok {
				continue
			}
		}
		viewTask := liteHumanActionTaskFromExecutionNode(node)
		if viewTask.ID == "" {
			continue
		}
		if statusFilter != "" && strings.TrimSpace(viewTask.Status) != statusFilter {
			continue
		}
		result = append(result, viewTask)
	}

	return result
}

func liteHumanActionTaskFromRequest(req humanInterview) teamTask {
	channel := normalizeChannelSlug(req.Channel)
	if channel == "" {
		channel = "general"
	}
	title := strings.TrimSpace(req.Title)
	if title == "" || title == "Request" {
		title = truncateSummary(strings.TrimSpace(req.Question), 96)
	}
	if title == "" {
		title = "Human response needed"
	}
	status := strings.TrimSpace(req.RecommendationStatus)
	if status == "" {
		status = "none"
	}
	return teamTask{
		ID:                     "human-request-" + strings.TrimSpace(req.ID),
		Channel:                channel,
		ExecutionKey:           "human-request|" + strings.TrimSpace(req.ID),
		Title:                  title,
		Details:                strings.TrimSpace(req.Context),
		Owner:                  "human",
		Status:                 "pending",
		CreatedBy:              strings.TrimSpace(req.From),
		ThreadID:               strings.TrimSpace(req.ReplyTo),
		TaskType:               "human_action",
		Blocked:                req.Blocking,
		CreatedAt:              strings.TrimSpace(req.CreatedAt),
		UpdatedAt:              strings.TrimSpace(req.UpdatedAt),
		AwaitingHuman:          true,
		AwaitingHumanSince:     firstNonEmpty(strings.TrimSpace(req.UpdatedAt), strings.TrimSpace(req.CreatedAt)),
		AwaitingHumanReason:    strings.TrimSpace(req.Question),
		AwaitingHumanRequestID: strings.TrimSpace(req.ID),
		AwaitingHumanSource:    "request",
		RecommendedResponder:   "game-master",
		RecommendationStatus:   status,
		RecommendationTaskID:   strings.TrimSpace(req.RecommendationTaskID),
		SourceMessageID:        strings.TrimSpace(req.ReplyTo),
		SourceRequestID:        strings.TrimSpace(req.ID),
		SourceTaskID:           strings.TrimSpace(req.SourceTaskID),
		HumanOptions:           append([]interviewOption(nil), req.Options...),
		HumanRecommendedID:     strings.TrimSpace(req.RecommendedID),
	}
}

func liteHumanActionTaskFromExecutionNode(node executionNode) teamTask {
	channel := normalizeChannelSlug(node.Channel)
	if channel == "" {
		channel = "general"
	}
	return teamTask{
		ID:                  "human-node-" + strings.TrimSpace(node.ID),
		Channel:             channel,
		ExecutionKey:        "human-node|" + strings.TrimSpace(node.ID),
		Title:               "Human follow-up needed",
		Details:             strings.TrimSpace(node.AwaitingHumanReason),
		Owner:               "human",
		Status:              "pending",
		CreatedBy:           firstNonEmpty(strings.TrimSpace(node.OwnerAgent), "system"),
		ThreadID:            firstNonEmpty(strings.TrimSpace(node.RootMessageID), strings.TrimSpace(node.TriggerMessageID)),
		TaskType:            "human_action",
		CreatedAt:           strings.TrimSpace(node.CreatedAt),
		UpdatedAt:           strings.TrimSpace(node.UpdatedAt),
		AwaitingHuman:       true,
		AwaitingHumanSince:  strings.TrimSpace(node.AwaitingHumanSince),
		AwaitingHumanReason: strings.TrimSpace(node.AwaitingHumanReason),
		AwaitingHumanSource: "execution_node",
		SourceMessageID:     firstNonEmpty(strings.TrimSpace(node.RootMessageID), strings.TrimSpace(node.TriggerMessageID)),
	}
}

func applyDeliveryProgress(task *teamTask, progressByDelivery map[string]deliveryView) {
	if task == nil {
		return
	}
	if task.DeliveryID == "" {
		return
	}
	delivery, ok := progressByDelivery[task.DeliveryID]
	if !ok {
		return
	}
	task.ProgressPercent = delivery.ProgressPercent
	task.ProgressBasis = delivery.ProgressBasis
}

func (b *Broker) humanActionTaskFromRequestLocked(req humanInterview) teamTask {
	channel := normalizeChannelSlug(req.Channel)
	if channel == "" {
		channel = "general"
	}
	title := strings.TrimSpace(req.Title)
	if title == "" || title == "Request" {
		title = truncateSummary(strings.TrimSpace(req.Question), 96)
	}
	if title == "" {
		title = "Human response needed"
	}
	recommendationStatus, recommendationSummary := b.requestRecommendationStateLocked(req)
	sourceTaskID := strings.TrimSpace(req.SourceTaskID)
	if sourceTaskID == "" {
		sourceTaskID = b.findSourceTaskIDForThreadLocked(channel, b.threadKeyForRequestLocked(req))
	}
	task := teamTask{
		ID:                     "human-request-" + strings.TrimSpace(req.ID),
		Channel:                channel,
		ExecutionKey:           "human-request|" + strings.TrimSpace(req.ID),
		Title:                  title,
		Details:                strings.TrimSpace(req.Context),
		Owner:                  "human",
		Status:                 "pending",
		CreatedBy:              strings.TrimSpace(req.From),
		ThreadID:               strings.TrimSpace(req.ReplyTo),
		TaskType:               "human_action",
		Blocked:                req.Blocking,
		CreatedAt:              strings.TrimSpace(req.CreatedAt),
		UpdatedAt:              strings.TrimSpace(req.UpdatedAt),
		AwaitingHuman:          true,
		AwaitingHumanSince:     firstNonEmpty(strings.TrimSpace(req.UpdatedAt), strings.TrimSpace(req.CreatedAt)),
		AwaitingHumanReason:    strings.TrimSpace(req.Question),
		AwaitingHumanRequestID: strings.TrimSpace(req.ID),
		AwaitingHumanSource:    "request",
		RecommendedResponder:   "game-master",
		RecommendationStatus:   recommendationStatus,
		RecommendationSummary:  recommendationSummary,
		RecommendationTaskID:   strings.TrimSpace(req.RecommendationTaskID),
		SourceMessageID:        strings.TrimSpace(req.ReplyTo),
		SourceRequestID:        strings.TrimSpace(req.ID),
		SourceTaskID:           sourceTaskID,
		DeliveryID:             b.deliveryKeyForRequestLocked(req),
		HumanOptions:           append([]interviewOption(nil), req.Options...),
		HumanRecommendedID:     strings.TrimSpace(req.RecommendedID),
	}
	return task
}

func (b *Broker) humanActionTaskFromExecutionNodeLocked(node executionNode) teamTask {
	channel := normalizeChannelSlug(node.Channel)
	if channel == "" {
		channel = "general"
	}
	sourceTaskID := b.findSourceTaskIDForThreadLocked(channel, firstNonEmpty(strings.TrimSpace(node.RootMessageID), strings.TrimSpace(node.TriggerMessageID)))
	title := "Human follow-up needed"
	if sourceTaskID != "" {
		if source := b.findTaskByIDLocked(sourceTaskID); source != nil && strings.TrimSpace(source.Title) != "" {
			title = "Reply needed: " + strings.TrimSpace(source.Title)
		}
	}
	return teamTask{
		ID:                  "human-node-" + strings.TrimSpace(node.ID),
		Channel:             channel,
		ExecutionKey:        "human-node|" + strings.TrimSpace(node.ID),
		Title:               title,
		Details:             strings.TrimSpace(node.AwaitingHumanReason),
		Owner:               "human",
		Status:              "pending",
		CreatedBy:           firstNonEmpty(strings.TrimSpace(node.OwnerAgent), "system"),
		ThreadID:            firstNonEmpty(strings.TrimSpace(node.RootMessageID), strings.TrimSpace(node.TriggerMessageID)),
		TaskType:            "human_action",
		CreatedAt:           strings.TrimSpace(node.CreatedAt),
		UpdatedAt:           strings.TrimSpace(node.UpdatedAt),
		AwaitingHuman:       true,
		AwaitingHumanSince:  strings.TrimSpace(node.AwaitingHumanSince),
		AwaitingHumanReason: strings.TrimSpace(node.AwaitingHumanReason),
		AwaitingHumanSource: "execution_node",
		SourceMessageID:     firstNonEmpty(strings.TrimSpace(node.RootMessageID), strings.TrimSpace(node.TriggerMessageID)),
		SourceTaskID:        sourceTaskID,
		DeliveryID:          b.deliveryKeyForExecutionNodeLocked(node),
	}
}

func (b *Broker) requestRecommendationStateLocked(req humanInterview) (string, string) {
	status := strings.TrimSpace(req.RecommendationStatus)
	summary := b.latestGameMasterRecommendationLocked(req)
	switch {
	case summary != "":
		return "ready", summary
	case status != "":
		return status, ""
	default:
		return "none", ""
	}
}

func (b *Broker) latestGameMasterRecommendationLocked(req humanInterview) string {
	channel := normalizeChannelSlug(req.Channel)
	if channel == "" {
		channel = "general"
	}
	threadRoot := b.threadKeyForRequestLocked(req)
	if threadRoot == "" {
		return ""
	}
	cutoff := firstNonEmpty(strings.TrimSpace(req.RecommendationRequestedAt), strings.TrimSpace(req.UpdatedAt), strings.TrimSpace(req.CreatedAt))
	for i := len(b.messages) - 1; i >= 0; i-- {
		msg := b.messages[i]
		if normalizeChannelSlug(msg.Channel) != channel {
			continue
		}
		if strings.TrimSpace(msg.From) != "game-master" {
			continue
		}
		if cutoff != "" && strings.TrimSpace(msg.Timestamp) < cutoff {
			continue
		}
		if b.threadRootFromMessageIDLocked(strings.TrimSpace(msg.ID)) != threadRoot {
			continue
		}
		return truncateSummary(strings.TrimSpace(msg.Content), 280)
	}
	return ""
}

func (b *Broker) requestGameMasterRecommendationLocked(requestID, actor string) (humanInterview, teamTask, channelMessage, error) {
	requestID = strings.TrimSpace(requestID)
	actor = firstNonEmpty(strings.TrimSpace(actor), "human")
	for i := range b.requests {
		req := &b.requests[i]
		if strings.TrimSpace(req.ID) != requestID {
			continue
		}
		if !requestIsActive(*req) {
			return humanInterview{}, teamTask{}, channelMessage{}, errConflict("request is no longer active")
		}
		now := time.Now().UTC().Format(time.RFC3339)
		task := b.ensureRecommendationTaskLocked(*req, actor, now)
		req.RecommendationStatus = "requested"
		req.RecommendationTaskID = strings.TrimSpace(task.ID)
		if strings.TrimSpace(req.RecommendationRequestedAt) == "" {
			req.RecommendationRequestedAt = now
		}
		req.UpdatedAt = now
		prompt := b.postRecommendationPromptLocked(*req, actor, now)
		b.appendActionLocked("request_recommendation_requested", "office", normalizeChannelSlug(req.Channel), actor, truncateSummary("Requested @game-master recommendation for "+firstNonEmpty(req.Title, req.Question), 140), req.ID)
		return *req, task, prompt, nil
	}
	return humanInterview{}, teamTask{}, channelMessage{}, errNotFound("request not found")
}

func (b *Broker) ensureRecommendationTaskLocked(req humanInterview, actor, now string) teamTask {
	channel := normalizeChannelSlug(req.Channel)
	if channel == "" {
		channel = "general"
	}
	executionKey := channel + "|human-triage|" + strings.TrimSpace(req.ID)
	for i := range b.tasks {
		task := &b.tasks[i]
		if strings.TrimSpace(task.ExecutionKey) != executionKey {
			continue
		}
		if taskIsTerminal(task) {
			task.Status = "in_progress"
		}
		task.Owner = "game-master"
		task.UpdatedAt = now
		task.SourceRequestID = strings.TrimSpace(req.ID)
		task.SourceTaskID = strings.TrimSpace(req.SourceTaskID)
		task.ThreadID = firstNonEmpty(strings.TrimSpace(req.ReplyTo), task.ThreadID)
		task.RecommendationStatus = "requested"
		task.AwaitingHuman = false
		b.ensureTaskOwnerChannelMembershipLocked(channel, task.Owner)
		return *task
	}

	title := firstNonEmpty(strings.TrimSpace(req.Title), truncateSummary(strings.TrimSpace(req.Question), 72), "Human recommendation")
	task := teamTask{
		ID:                   "",
		Channel:              channel,
		ExecutionKey:         executionKey,
		Title:                "Recommend response: " + title,
		Details:              buildRecommendationTaskDetails(req),
		Owner:                "game-master",
		Status:               "in_progress",
		CreatedBy:            actor,
		ThreadID:             strings.TrimSpace(req.ReplyTo),
		TaskType:             "human_triage",
		CreatedAt:            now,
		UpdatedAt:            now,
		SourceRequestID:      strings.TrimSpace(req.ID),
		SourceTaskID:         strings.TrimSpace(req.SourceTaskID),
		RecommendationStatus: "requested",
	}
	b.counter++
	task.ID = "task-" + itoa(b.counter)
	b.ensureTaskOwnerChannelMembershipLocked(channel, task.Owner)
	b.queueTaskBehindActiveOwnerLaneLocked(&task)
	b.scheduleTaskLifecycleLocked(&task)
	b.tasks = append(b.tasks, task)
	b.appendActionLocked("task_created", "office", channel, actor, truncateSummary(task.Title, 140), task.ID)
	return task
}

func buildRecommendationTaskDetails(req humanInterview) string {
	parts := []string{
		"Prepare a short recommendation for the human to answer this request.",
		"Goal: return a ready-to-send reply without deciding in place of the human.",
		"Request ID: " + strings.TrimSpace(req.ID),
		"Question: " + strings.TrimSpace(req.Question),
	}
	if context := strings.TrimSpace(req.Context); context != "" {
		parts = append(parts, "Context: "+context)
	}
	if sourceTaskID := strings.TrimSpace(req.SourceTaskID); sourceTaskID != "" {
		parts = append(parts, "Related task: "+sourceTaskID)
	}
	return strings.Join(parts, "\n\n")
}

func (b *Broker) postRecommendationPromptLocked(req humanInterview, actor, now string) channelMessage {
	channel := normalizeChannelSlug(req.Channel)
	if channel == "" || b.findChannelLocked(channel) == nil {
		return channelMessage{}
	}
	replyTo := strings.TrimSpace(req.ReplyTo)
	msg := channelMessage{
		ID:        "",
		From:      actor,
		Channel:   channel,
		Content:   buildRecommendationPrompt(req),
		Tagged:    []string{"game-master"},
		ReplyTo:   replyTo,
		Timestamp: now,
	}
	b.counter++
	msg.ID = "msg-" + itoa(b.counter)
	b.ensureLeadTaggedMembersEnabledLocked(msg.From, channel, msg.Tagged)
	b.appendMessageLocked(msg)
	b.advanceExecutionGraphForMessageLocked(msg)
	b.recordChannelMemoryForMessageLocked(msg)
	return msg
}

func buildRecommendationPrompt(req humanInterview) string {
	title := firstNonEmpty(strings.TrimSpace(req.Title), "Human request")
	lines := []string{
		"@game-master prepare a short recommendation and suggested reply for the human.",
		"Title: " + title,
		"Question: " + strings.TrimSpace(req.Question),
	}
	if context := strings.TrimSpace(req.Context); context != "" {
		lines = append(lines, "Context: "+context)
	}
	return strings.Join(lines, "\n")
}

func (b *Broker) buildDeliveriesLocked(channel string, allChannels, includeDone bool) []deliveryView {
	allTasksByID := make(map[string]teamTask, len(b.tasks))
	for _, task := range b.tasks {
		allTasksByID[strings.TrimSpace(task.ID)] = task
	}

	buckets := map[string]*deliveryAccumulator{}
	ensureBucket := func(key string) *deliveryAccumulator {
		if existing, ok := buckets[key]; ok {
			return existing
		}
		bucket := &deliveryAccumulator{
			view: deliveryView{
				ID: key,
			},
			taskIDs:      map[string]struct{}{},
			requestIDs:   map[string]struct{}{},
			artifactKeys: map[string]struct{}{},
		}
		buckets[key] = bucket
		return bucket
	}

	for _, task := range b.tasks {
		taskChannel := normalizeChannelSlug(task.Channel)
		if taskChannel == "" {
			taskChannel = "general"
		}
		if !allChannels && taskChannel != channel {
			continue
		}
		if !includeDone && taskIsTerminal(&task) {
			continue
		}
		key := b.deliveryKeyForTaskLocked(task)
		bucket := ensureBucket(key)
		bucket.addTask(task)
	}

	for _, req := range b.requests {
		reqChannel := normalizeChannelSlug(req.Channel)
		if reqChannel == "" {
			reqChannel = "general"
		}
		if !allChannels && reqChannel != channel {
			continue
		}
		if !includeDone && !requestIsActive(req) {
			continue
		}
		key := b.deliveryKeyForRequestLocked(req)
		bucket := ensureBucket(key)
		bucket.addRequest(req)
	}

	for _, node := range b.executionNodes {
		if !node.AwaitingHumanInput || !executionNodeIsOpen(node) {
			continue
		}
		nodeChannel := normalizeChannelSlug(node.Channel)
		if nodeChannel == "" {
			nodeChannel = "general"
		}
		if !allChannels && nodeChannel != channel {
			continue
		}
		key := b.deliveryKeyForExecutionNodeLocked(node)
		bucket := ensureBucket(key)
		bucket.view.PendingHumanCount++
		if bucket.view.Status == "" {
			bucket.view.Status = "awaiting_human"
		}
		if bucket.view.Channel == "" {
			bucket.view.Channel = nodeChannel
		}
		if bucket.view.Title == "" {
			bucket.view.Title = "Human follow-up needed"
		}
		if bucket.view.LastSubstantiveUpdateAt == "" || studioTimestampAfter(strings.TrimSpace(node.UpdatedAt), bucket.view.LastSubstantiveUpdateAt) {
			bucket.view.LastSubstantiveUpdateAt = strings.TrimSpace(node.UpdatedAt)
			bucket.view.LastSubstantiveUpdateBy = "system"
			bucket.view.LastSubstantiveSummary = truncateSummary(strings.TrimSpace(node.AwaitingHumanReason), 160)
		}
	}

	out := make([]deliveryView, 0, len(buckets))
	for key, bucket := range buckets {
		bucket.finalize(key, allTasksByID)
		if bucket.view.Title == "" {
			continue
		}
		if ch := b.findChannelLocked(bucket.view.Channel); ch != nil {
			if repoPath := strings.TrimSpace(ch.primaryLinkedRepoPath()); repoPath != "" {
				bucket.considerWorkspacePath(repoPath, "channel_repo", bucket.view.LastSubstantiveUpdateAt)
			}
		}
		out = append(out, bucket.view)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PendingHumanCount != out[j].PendingHumanCount {
			return out[i].PendingHumanCount > out[j].PendingHumanCount
		}
		if studioTimestampAfter(out[i].LastSubstantiveUpdateAt, out[j].LastSubstantiveUpdateAt) {
			return true
		}
		if studioTimestampAfter(out[j].LastSubstantiveUpdateAt, out[i].LastSubstantiveUpdateAt) {
			return false
		}
		return out[i].Title < out[j].Title
	})
	return out
}

func (b *Broker) deliveryProgressIndexLocked(channel string, allChannels, includeDone bool) map[string]deliveryView {
	deliveries := b.buildDeliveriesLocked(channel, allChannels, includeDone)
	index := make(map[string]deliveryView, len(deliveries))
	for _, delivery := range deliveries {
		index[delivery.ID] = delivery
	}
	return index
}

func (b *Broker) deliveryKeyForTaskLocked(task teamTask) string {
	if sourceTaskID := firstNonEmpty(strings.TrimSpace(task.SourceTaskID), derivedSourceTaskID(task)); sourceTaskID != "" {
		return "delivery-task-" + sourceTaskID
	}
	if pipelineID := strings.TrimSpace(task.PipelineID); pipelineID != "" {
		// Pipeline IDs are reused across the office, so the fallback bucket must stay channel-local.
		channel := normalizeChannelSlug(task.Channel)
		if channel == "" {
			channel = "general"
		}
		return "delivery-channel-" + channel + "-pipeline-" + pipelineID
	}
	if taskID := strings.TrimSpace(task.ID); taskID != "" {
		return "delivery-task-" + taskID
	}
	if threadID := strings.TrimSpace(task.ThreadID); threadID != "" {
		return "delivery-thread-" + b.threadRootFromMessageIDLocked(threadID)
	}
	if executionKey := normalizeExecutionKey(task.ExecutionKey); executionKey != "" {
		return "delivery-execution-" + executionKey
	}
	return "delivery-task-unkeyed"
}

func (b *Broker) deliveryKeyForRequestLocked(req humanInterview) string {
	if sourceTaskID := strings.TrimSpace(req.SourceTaskID); sourceTaskID != "" {
		return "delivery-task-" + sourceTaskID
	}
	channel := normalizeChannelSlug(req.Channel)
	if channel == "" {
		channel = "general"
	}
	if sourceTaskID := b.findSourceTaskIDForThreadLocked(channel, b.threadKeyForRequestLocked(req)); sourceTaskID != "" {
		return "delivery-task-" + sourceTaskID
	}
	if threadRoot := b.threadKeyForRequestLocked(req); threadRoot != "" {
		return "delivery-thread-" + threadRoot
	}
	return "delivery-request-" + strings.TrimSpace(req.ID)
}

func (b *Broker) deliveryKeyForExecutionNodeLocked(node executionNode) string {
	sourceTaskID := b.findSourceTaskIDForThreadLocked(normalizeChannelSlug(node.Channel), firstNonEmpty(strings.TrimSpace(node.RootMessageID), strings.TrimSpace(node.TriggerMessageID)))
	if sourceTaskID != "" {
		return "delivery-task-" + sourceTaskID
	}
	if root := firstNonEmpty(strings.TrimSpace(node.RootMessageID), strings.TrimSpace(node.TriggerMessageID)); root != "" {
		return "delivery-thread-" + root
	}
	return "delivery-node-" + strings.TrimSpace(node.ID)
}

func (b *Broker) threadKeyForRequestLocked(req humanInterview) string {
	replyTo := strings.TrimSpace(req.ReplyTo)
	if replyTo == "" {
		return ""
	}
	return firstNonEmpty(b.threadRootFromMessageIDLocked(replyTo), replyTo)
}

func (b *Broker) findSourceTaskIDForThreadLocked(channel, threadRoot string) string {
	channel = normalizeChannelSlug(channel)
	threadRoot = strings.TrimSpace(threadRoot)
	if channel == "" || threadRoot == "" {
		return ""
	}
	rootMsg := b.findMessageByIDLocked(threadRoot)
	msgWorkspacePath := ""
	if rootMsg != nil {
		msgWorkspacePath = explicitWorkspacePathForMessage(*rootMsg)
	}
	var direct *teamTask
	for _, task := range b.tasks {
		if normalizeChannelSlug(task.Channel) != channel {
			continue
		}
		if !taskMatchesExplicitWorkspacePath(&task, msgWorkspacePath) {
			continue
		}
		taskThread := firstNonEmpty(b.threadRootFromMessageIDLocked(strings.TrimSpace(task.ThreadID)), strings.TrimSpace(task.ThreadID))
		sourceMessageThread := firstNonEmpty(b.threadRootFromMessageIDLocked(strings.TrimSpace(task.SourceMessageID)), strings.TrimSpace(task.SourceMessageID))
		if taskThread == threadRoot || sourceMessageThread == threadRoot {
			direct = selectCanonicalExecutionTask(direct, &task)
		}
	}
	if direct != nil {
		return strings.TrimSpace(direct.ID)
	}
	if rootMsg == nil {
		return ""
	}
	var best *teamTask
	bestScore := 0.0
	for i := range b.tasks {
		task := &b.tasks[i]
		if normalizeChannelSlug(task.Channel) != channel {
			continue
		}
		if !taskMatchesExplicitWorkspacePath(task, msgWorkspacePath) {
			continue
		}
		if taskIsTerminal(task) {
			continue
		}
		score := orchestration.ScoreMessageAgainstTerms(messageRoutingText(*rootMsg), taskRoutingTerms(*task))
		if score < officeRoutingMatchThreshold {
			continue
		}
		if best == nil || score > bestScore {
			best = task
			bestScore = score
			continue
		}
		if score == bestScore && selectCanonicalExecutionTask(best, task) == task {
			best = task
		}
	}
	if best != nil {
		return strings.TrimSpace(best.ID)
	}
	return ""
}

func (b *Broker) findTaskByIDLocked(taskID string) *teamTask {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil
	}
	for i := range b.tasks {
		if strings.TrimSpace(b.tasks[i].ID) == taskID {
			return &b.tasks[i]
		}
	}
	return nil
}

func derivedSourceTaskID(task teamTask) string {
	if task.DerivedFrom == nil {
		return ""
	}
	return strings.TrimSpace(task.DerivedFrom.SourceTaskID)
}

func (b *deliveryAccumulator) addTask(task teamTask) {
	taskID := strings.TrimSpace(task.ID)
	if taskID != "" {
		if _, ok := b.taskIDs[taskID]; !ok {
			b.taskIDs[taskID] = struct{}{}
			b.view.TaskIDs = append(b.view.TaskIDs, taskID)
		}
	}
	if b.view.Channel == "" {
		b.view.Channel = normalizeChannelSlug(task.Channel)
	}
	if b.view.Owner == "" && strings.TrimSpace(task.Owner) != "" {
		b.view.Owner = strings.TrimSpace(task.Owner)
	}
	if b.view.Title == "" || (strings.TrimSpace(task.TaskType) != "human_triage" && strings.HasPrefix(b.view.Title, "Recommend response:")) {
		b.view.Title = strings.TrimSpace(task.Title)
	}
	if details := strings.TrimSpace(task.Details); details != "" && b.view.Summary == "" {
		b.view.Summary = truncateSummary(details, 180)
	}
	if b.view.LastSubstantiveUpdateAt == "" || studioTimestampAfter(strings.TrimSpace(task.UpdatedAt), b.view.LastSubstantiveUpdateAt) {
		b.view.LastSubstantiveUpdateAt = strings.TrimSpace(task.UpdatedAt)
		b.view.LastSubstantiveUpdateBy = firstNonEmpty(strings.TrimSpace(task.Owner), strings.TrimSpace(task.CreatedBy))
		b.view.LastSubstantiveSummary = truncateSummary(firstNonEmpty(strings.TrimSpace(task.Details), strings.TrimSpace(task.Title)), 180)
	}
	if task.Blocked || len(task.BlockerRequestIDs) > 0 {
		b.view.BlockerCount++
		b.hasBlocked = true
	}
	if strings.TrimSpace(task.TaskType) == "human_triage" {
		b.view.PendingHumanCount++
	}
	completed, total := taskMilestones(task)
	b.completedSteps += completed
	b.totalSteps += total
	switch strings.ToLower(strings.TrimSpace(task.Status)) {
	case "done", "completed", "canceled", "cancelled", "failed":
		b.hasDone = true
	case "review", "in_review":
		b.hasReview = true
	case "blocked":
		b.hasBlocked = true
	default:
		if !taskIsTerminal(&task) {
			b.hasInProgress = true
		}
	}
	b.addTaskArtifacts(task)
}

func (b *deliveryAccumulator) addRequest(req humanInterview) {
	requestID := strings.TrimSpace(req.ID)
	if requestID != "" {
		if _, ok := b.requestIDs[requestID]; !ok {
			b.requestIDs[requestID] = struct{}{}
			b.view.RequestIDs = append(b.view.RequestIDs, requestID)
		}
	}
	if b.view.Channel == "" {
		b.view.Channel = normalizeChannelSlug(req.Channel)
	}
	if b.view.Title == "" {
		b.view.Title = firstNonEmpty(strings.TrimSpace(req.Title), truncateSummary(strings.TrimSpace(req.Question), 96), "Delivery")
	}
	if b.view.Summary == "" {
		b.view.Summary = truncateSummary(firstNonEmpty(strings.TrimSpace(req.Context), strings.TrimSpace(req.Question)), 180)
	}
	if requestIsActive(req) {
		b.view.PendingHumanCount++
	}
	if b.view.LastSubstantiveUpdateAt == "" || studioTimestampAfter(strings.TrimSpace(req.UpdatedAt), b.view.LastSubstantiveUpdateAt) {
		b.view.LastSubstantiveUpdateAt = strings.TrimSpace(req.UpdatedAt)
		b.view.LastSubstantiveUpdateBy = strings.TrimSpace(req.From)
		b.view.LastSubstantiveSummary = truncateSummary(strings.TrimSpace(req.Question), 180)
	}
	if len(b.view.TaskIDs) == 0 {
		b.totalSteps++
	}
}

func deliveryWorkspaceCandidateScore(path, artifactKind string) int {
	path = strings.TrimSpace(path)
	if path == "" {
		return -1
	}
	score := deliveryRepositoryPriority(deliveryRepositoryToken(path))
	switch strings.ToLower(strings.TrimSpace(artifactKind)) {
	case "channel_repo":
		score += 700
	case "workspace":
		score += 500
	case "worktree":
		if isLikelyDunderiaTaskWorktree(path) {
			score += 420
		} else {
			score += 120
		}
	default:
		score += 40
	}
	return score
}

func deliveryRepositoryPriority(token string) int {
	switch {
	case strings.HasPrefix(token, "conveniosweb"),
		strings.HasPrefix(token, "chamadoweb"),
		strings.HasPrefix(token, "transparenciaweb"),
		strings.HasPrefix(token, "sistemascompartilhadoswebforms"),
		strings.HasPrefix(token, "tectrilhaapi"):
		return 300
	case token == "dunderia", token == "superpowers", token == "codexlb", token == "vibeyard":
		return 180
	case token == "scripts", token == "temp", token == "memory", token == "relatorios":
		return 40
	case token != "":
		return 120
	default:
		return 0
	}
}

func deliveryRepositoryToken(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	normalized := strings.ToLower(strings.ReplaceAll(path, "/", `\`))
	parts := strings.FieldsFunc(normalized, func(r rune) bool { return r == '\\' || r == '/' })
	if len(parts) == 0 {
		return ""
	}
	for i, part := range parts {
		if part == "task-worktrees" && i+1 < len(parts) {
			return normalizeRepositoryTokenForDelivery(parts[i+1])
		}
		if _, ok := deliveryWorkspaceRepoRootMarkers[part]; ok && i+1 < len(parts) {
			return normalizeRepositoryTokenForDelivery(parts[i+1])
		}
	}
	return ""
}

func normalizeRepositoryTokenForDelivery(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func isLikelyDunderiaTaskWorktree(path string) bool {
	path = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(path, "/", `\`)))
	return strings.Contains(path, `\task-worktrees\dunderia\`)
}

func (b *deliveryAccumulator) considerWorkspacePath(path, artifactKind, updatedAt string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	score := deliveryWorkspaceCandidateScore(path, artifactKind)
	if score < 0 {
		return
	}
	if strings.TrimSpace(b.view.WorkspacePath) == "" || score > b.workspaceScore || (score == b.workspaceScore && studioTimestampAfter(strings.TrimSpace(updatedAt), b.workspaceAt)) {
		b.view.WorkspacePath = path
		b.workspaceScore = score
		b.workspaceAt = strings.TrimSpace(updatedAt)
	}
}

func (b *deliveryAccumulator) addTaskArtifacts(task teamTask) {
	appendArtifact := func(artifact deliveryArtifact) {
		key := strings.Join([]string{artifact.Kind, artifact.Path, artifact.URL, artifact.RelatedID}, "|")
		if _, ok := b.artifactKeys[key]; ok {
			return
		}
		b.artifactKeys[key] = struct{}{}
		b.view.Artifacts = append(b.view.Artifacts, artifact)
	}
	if path := strings.TrimSpace(task.WorkspacePath); path != "" {
		b.considerWorkspacePath(path, "workspace", strings.TrimSpace(task.UpdatedAt))
		appendArtifact(deliveryArtifact{Kind: "workspace", Title: "Workspace", Path: path, UpdatedAt: strings.TrimSpace(task.UpdatedAt), RelatedID: strings.TrimSpace(task.ID)})
	}
	if path := strings.TrimSpace(task.WorktreePath); path != "" {
		b.considerWorkspacePath(path, "worktree", strings.TrimSpace(task.UpdatedAt))
		appendArtifact(deliveryArtifact{Kind: "worktree", Title: "Worktree", Path: path, UpdatedAt: strings.TrimSpace(task.UpdatedAt), RelatedID: strings.TrimSpace(task.ID)})
	}
	if path := explicitWorkspacePathFromText(strings.TrimSpace(strings.Join([]string{task.Title, task.Details}, "\n"))); path != "" {
		b.considerWorkspacePath(path, "mentioned_workspace", strings.TrimSpace(task.UpdatedAt))
		appendArtifact(deliveryArtifact{Kind: "workspace", Title: "Workspace", Path: path, UpdatedAt: strings.TrimSpace(task.UpdatedAt), RelatedID: strings.TrimSpace(task.ID)})
	}
	if url := publicationURL(task.IssuePublication); url != "" {
		appendArtifact(deliveryArtifact{Kind: "issue", Title: "Issue", URL: url, State: publicationStatus(task.IssuePublication), UpdatedAt: strings.TrimSpace(task.UpdatedAt), RelatedID: strings.TrimSpace(task.ID)})
	}
	if url := publicationURL(task.PRPublication); url != "" {
		appendArtifact(deliveryArtifact{Kind: "pull_request", Title: "Pull request", URL: url, State: publicationStatus(task.PRPublication), UpdatedAt: strings.TrimSpace(task.UpdatedAt), RelatedID: strings.TrimSpace(task.ID)})
	}
	for _, match := range artifactPathPattern.FindAllString(strings.TrimSpace(task.Details), -1) {
		appendArtifact(deliveryArtifact{Kind: "document", Title: match, Path: match, UpdatedAt: strings.TrimSpace(task.UpdatedAt), RelatedID: strings.TrimSpace(task.ID)})
	}
}

func (b *deliveryAccumulator) finalize(key string, allTasksByID map[string]teamTask) {
	sort.Strings(b.view.TaskIDs)
	sort.Strings(b.view.RequestIDs)
	if b.view.Title == "" && strings.HasPrefix(key, "delivery-task-") {
		taskID := strings.TrimPrefix(key, "delivery-task-")
		if root, ok := allTasksByID[taskID]; ok {
			b.view.Title = strings.TrimSpace(root.Title)
			if b.view.Summary == "" {
				b.view.Summary = truncateSummary(strings.TrimSpace(root.Details), 180)
			}
			if b.view.Owner == "" {
				b.view.Owner = strings.TrimSpace(root.Owner)
			}
			if b.view.Channel == "" {
				b.view.Channel = normalizeChannelSlug(root.Channel)
			}
		}
	}
	if b.totalSteps > 0 {
		b.view.ProgressPercent = (b.completedSteps * 100) / b.totalSteps
		b.view.ProgressBasis = itoa(b.completedSteps) + " of " + itoa(b.totalSteps) + " milestones complete"
	}
	switch {
	case b.view.PendingHumanCount > 0:
		b.view.Status = "awaiting_human"
	case b.hasBlocked || b.view.BlockerCount > 0:
		b.view.Status = "blocked"
	case len(b.view.TaskIDs) > 0 && !b.hasInProgress && !b.hasReview && b.hasDone:
		b.view.Status = "done"
	case b.hasReview:
		b.view.Status = "review"
	default:
		b.view.Status = "in_progress"
	}
	sort.Slice(b.view.Artifacts, func(i, j int) bool {
		if studioTimestampAfter(b.view.Artifacts[i].UpdatedAt, b.view.Artifacts[j].UpdatedAt) {
			return true
		}
		if studioTimestampAfter(b.view.Artifacts[j].UpdatedAt, b.view.Artifacts[i].UpdatedAt) {
			return false
		}
		return b.view.Artifacts[i].Title < b.view.Artifacts[j].Title
	})
}

func taskMilestones(task teamTask) (completed int, total int) {
	total = 4
	switch strings.ToLower(strings.TrimSpace(task.Status)) {
	case "done", "completed":
		return 4, total
	case "review", "in_review":
		return 3, total
	case "in_progress":
		return 2, total
	case "blocked":
		return 1, total
	case "canceled", "cancelled":
		return 4, total
	default:
		return 1, total
	}
}

func errConflict(message string) error {
	return &operatorViewError{message: message}
}

func errNotFound(message string) error {
	return &operatorViewError{message: message}
}

type operatorViewError struct {
	message string
}

func (e *operatorViewError) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}
