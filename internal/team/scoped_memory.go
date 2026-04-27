package team

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	privateMemoryScope = "private"
	sharedMemoryScope  = "shared"
	channelMemoryScope = "channel"

	channelMemoryEntryLimit = 48
	channelMemoryBriefLimit = 3
)

type privateMemoryNote struct {
	Key       string `json:"key"`
	Title     string `json:"title,omitempty"`
	Content   string `json:"content"`
	Author    string `json:"author,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type brokerMemoryEntry struct {
	Key       string `json:"key"`
	Title     string `json:"title,omitempty"`
	Content   string `json:"content"`
	Author    string `json:"author,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func privateMemoryNamespace(slug string) string {
	return "agent/" + strings.TrimSpace(slug)
}

func channelMemoryNamespace(channel string) string {
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		channel = "general"
	}
	return channelMemoryScope + "/" + channel
}

func encodePrivateMemoryNote(note privateMemoryNote) string {
	note.Key = strings.TrimSpace(note.Key)
	note.Title = strings.TrimSpace(note.Title)
	note.Content = strings.TrimSpace(note.Content)
	note.Author = strings.TrimSpace(note.Author)
	now := time.Now().UTC().Format(time.RFC3339)
	if strings.TrimSpace(note.CreatedAt) == "" {
		note.CreatedAt = now
	}
	if strings.TrimSpace(note.UpdatedAt) == "" {
		note.UpdatedAt = note.CreatedAt
	}
	data, err := json.Marshal(note)
	if err != nil {
		return note.Content
	}
	return string(data)
}

func decodePrivateMemoryNote(key string, raw string) privateMemoryNote {
	key = strings.TrimSpace(key)
	raw = strings.TrimSpace(raw)
	note := privateMemoryNote{
		Key:     key,
		Content: raw,
		Title:   key,
	}
	if raw == "" {
		return note
	}
	var decoded privateMemoryNote
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return note
	}
	if strings.TrimSpace(decoded.Key) == "" {
		decoded.Key = key
	}
	if strings.TrimSpace(decoded.Title) == "" {
		decoded.Title = decoded.Key
	}
	if strings.TrimSpace(decoded.Content) == "" {
		decoded.Content = raw
	}
	return decoded
}

func brokerEntryFromNote(note privateMemoryNote) brokerMemoryEntry {
	return brokerMemoryEntry(note)
}

func searchPrivateMemory(entries map[string]string, query string, limit int) []privateMemoryNote {
	if limit <= 0 {
		limit = 5
	}
	query = strings.TrimSpace(strings.ToLower(query))
	type scoredNote struct {
		note  privateMemoryNote
		score int
	}
	notes := make([]scoredNote, 0, len(entries))
	for key, raw := range entries {
		note := decodePrivateMemoryNote(key, raw)
		haystack := normalizeMemorySearchText(strings.Join([]string{note.Key, note.Title, note.Content}, "\n"))
		score := privateMemoryMatchScore(haystack, query)
		if query != "" && score == 0 {
			continue
		}
		notes = append(notes, scoredNote{note: note, score: score})
	}
	sort.Slice(notes, func(i, j int) bool {
		if notes[i].score != notes[j].score {
			return notes[i].score > notes[j].score
		}
		return noteTimestamp(notes[i].note).After(noteTimestamp(notes[j].note))
	})
	if len(notes) > limit {
		notes = notes[:limit]
	}
	out := make([]privateMemoryNote, 0, len(notes))
	for _, item := range notes {
		out = append(out, item.note)
	}
	return out
}

func noteTimestamp(note privateMemoryNote) time.Time {
	for _, candidate := range []string{note.UpdatedAt, note.CreatedAt} {
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(candidate)); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func slugify(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || unicode.IsSpace(r):
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func normalizeMemorySearchText(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastSpace := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			lastSpace = false
		default:
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func privateMemoryMatchScore(haystack string, query string) int {
	if query == "" {
		return 1
	}
	query = normalizeMemorySearchText(query)
	if query == "" {
		return 1
	}
	score := 0
	if strings.Contains(haystack, query) {
		score += 100
	}
	for _, token := range strings.Fields(query) {
		if strings.Contains(haystack, token) {
			score += 10
		}
	}
	return score
}

func formatPrivateMemoryBrief(slug string, entries map[string]string, query string) string {
	if strings.TrimSpace(slug) == "" || len(entries) == 0 {
		return ""
	}
	matches := searchPrivateMemory(entries, query, 2)
	if len(matches) == 0 {
		return ""
	}
	lines := []string{"== PRIVATE MEMORY =="}
	for _, note := range matches {
		title := strings.TrimSpace(note.Title)
		if title == "" {
			title = strings.TrimSpace(note.Key)
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", title, truncate(strings.TrimSpace(strings.ReplaceAll(note.Content, "\n", " ")), 220)))
	}
	lines = append(lines, "== END PRIVATE MEMORY ==")
	return strings.Join(lines, "\n")
}

func isSubstantiveChannelMemoryText(content string) bool {
	normalized := strings.TrimSpace(strings.ToLower(content))
	if normalized == "" {
		return false
	}
	normalized = strings.Join(strings.Fields(normalized), " ")
	switch normalized {
	case "ok", "okay", "ok.", "ok!", "thanks", "thank you", "obrigado", "obrigada", "valeu", "feito", "show", "fechado", "ping":
		return false
	}
	if strings.HasPrefix(normalized, "routing to @") {
		return false
	}
	meaningfulWords := 0
	for _, token := range strings.Fields(normalized) {
		if strings.HasPrefix(token, "@") {
			continue
		}
		meaningfulWords++
	}
	return meaningfulWords >= 4 || len(normalized) >= 28
}

func channelMemoryMessageNote(msg channelMessage) (string, privateMemoryNote, bool) {
	channel := normalizeChannelSlug(msg.Channel)
	if channel == "" {
		channel = "general"
	}
	body := strings.TrimSpace(msg.Content)
	if title := strings.TrimSpace(msg.Title); title != "" {
		body = strings.TrimSpace(title + ": " + body)
	}
	if !isSubstantiveChannelMemoryText(body) {
		return "", privateMemoryNote{}, false
	}

	title := ""
	switch strings.TrimSpace(msg.From) {
	case "you", "human":
		title = "Human request"
	case "ceo":
		if len(msg.Tagged) > 0 {
			title = "CEO handoff"
		} else {
			title = "CEO note"
		}
	case "system", "nex", "":
		return "", privateMemoryNote{}, false
	default:
		if len(msg.Tagged) == 0 {
			return "", privateMemoryNote{}, false
		}
		title = "Agent handoff"
	}

	return "msg:" + strings.TrimSpace(msg.ID), privateMemoryNote{
		Title:     title,
		Content:   body,
		Author:    strings.TrimSpace(msg.From),
		CreatedAt: strings.TrimSpace(msg.Timestamp),
		UpdatedAt: strings.TrimSpace(msg.Timestamp),
	}, true
}

func channelMemoryActionNote(action officeActionLog) (string, privateMemoryNote, bool) {
	kind := strings.TrimSpace(action.Kind)
	title := ""
	switch kind {
	case "task_created":
		title = "Task created"
	case "task_updated":
		title = "Task update"
	case "task_unblocked":
		title = "Task unblocked"
	case "request_created":
		title = "Human request"
	default:
		return "", privateMemoryNote{}, false
	}
	content := strings.TrimSpace(action.Summary)
	if content == "" {
		return "", privateMemoryNote{}, false
	}
	if actor := strings.TrimSpace(action.Actor); actor != "" {
		content = fmt.Sprintf("@%s %s", actor, content)
	}
	identity := strings.TrimSpace(action.RelatedID)
	if identity == "" {
		identity = strings.TrimSpace(action.ID)
	}
	if identity == "" {
		identity = slugify(content)
	}
	return "action:" + identity + ":" + kind, privateMemoryNote{
		Title:     title,
		Content:   content,
		Author:    strings.TrimSpace(action.Actor),
		CreatedAt: strings.TrimSpace(action.CreatedAt),
		UpdatedAt: strings.TrimSpace(action.CreatedAt),
	}, true
}

func channelMemoryDecisionNote(decision officeDecisionRecord) (string, privateMemoryNote, bool) {
	summary := strings.TrimSpace(decision.Summary)
	if summary == "" {
		return "", privateMemoryNote{}, false
	}
	content := summary
	if reason := strings.TrimSpace(decision.Reason); reason != "" {
		content = summary + " Reason: " + reason
	}
	return "decision:" + strings.TrimSpace(decision.ID), privateMemoryNote{
		Title:     "Decision",
		Content:   content,
		Author:    strings.TrimSpace(decision.Owner),
		CreatedAt: strings.TrimSpace(decision.CreatedAt),
		UpdatedAt: strings.TrimSpace(decision.CreatedAt),
	}, true
}

func (b *Broker) storeChannelMemoryNoteLocked(channel, key string, note privateMemoryNote) {
	if b == nil {
		return
	}
	key = strings.TrimSpace(key)
	note.Content = strings.TrimSpace(note.Content)
	if key == "" || note.Content == "" {
		return
	}
	if b.sharedMemory == nil {
		b.sharedMemory = make(map[string]map[string]string)
	}
	namespace := channelMemoryNamespace(channel)
	if b.sharedMemory[namespace] == nil {
		b.sharedMemory[namespace] = make(map[string]string)
	}
	note.Key = key
	b.sharedMemory[namespace][key] = encodePrivateMemoryNote(note)
	pruneChannelMemoryEntries(b.sharedMemory[namespace], channelMemoryEntryLimit)
}

func (b *Broker) recordChannelMemoryForMessageLocked(msg channelMessage) {
	key, note, ok := channelMemoryMessageNote(msg)
	if !ok {
		return
	}
	b.storeChannelMemoryNoteLocked(msg.Channel, key, note)
}

func (b *Broker) recordChannelMemoryForActionLocked(action officeActionLog) {
	key, note, ok := channelMemoryActionNote(action)
	if !ok {
		return
	}
	b.storeChannelMemoryNoteLocked(action.Channel, key, note)
}

func (b *Broker) recordChannelMemoryForDecisionLocked(decision officeDecisionRecord) {
	key, note, ok := channelMemoryDecisionNote(decision)
	if !ok {
		return
	}
	b.storeChannelMemoryNoteLocked(decision.Channel, key, note)
}

func pruneChannelMemoryEntries(entries map[string]string, keep int) {
	if len(entries) <= keep || keep <= 0 {
		return
	}
	notes := make([]privateMemoryNote, 0, len(entries))
	for key, raw := range entries {
		notes = append(notes, decodePrivateMemoryNote(key, raw))
	}
	sort.Slice(notes, func(i, j int) bool {
		return noteTimestamp(notes[i]).After(noteTimestamp(notes[j]))
	})
	keepSet := make(map[string]struct{}, keep)
	for i, note := range notes {
		if i >= keep {
			break
		}
		keepSet[note.Key] = struct{}{}
	}
	for key := range entries {
		if _, ok := keepSet[key]; ok {
			continue
		}
		delete(entries, key)
	}
}

func searchChannelMemory(entries map[string]string, query string, limit int) []privateMemoryNote {
	if len(entries) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = channelMemoryBriefLimit
	}
	all := searchPrivateMemory(entries, "", len(entries))
	var matches []privateMemoryNote
	seen := make(map[string]struct{}, limit)
	add := func(note privateMemoryNote) {
		if strings.TrimSpace(note.Key) == "" {
			return
		}
		if _, ok := seen[note.Key]; ok {
			return
		}
		seen[note.Key] = struct{}{}
		matches = append(matches, note)
	}
	for _, note := range pinnedChannelMemoryNotes(all) {
		add(note)
		if len(matches) >= limit {
			return matches[:limit]
		}
	}
	for _, note := range searchPrivateMemory(entries, query, limit) {
		add(note)
		if len(matches) >= limit {
			return matches[:limit]
		}
	}
	for _, note := range all {
		if _, ok := seen[note.Key]; ok {
			continue
		}
		seen[note.Key] = struct{}{}
		matches = append(matches, note)
		if len(matches) >= limit {
			break
		}
	}
	return matches
}

func pinnedChannelMemoryNotes(notes []privateMemoryNote) []privateMemoryNote {
	if len(notes) == 0 {
		return nil
	}
	var latestHuman *privateMemoryNote
	var latestCEO *privateMemoryNote
	for i := range notes {
		note := notes[i]
		title := strings.TrimSpace(note.Title)
		author := strings.TrimSpace(note.Author)
		switch {
		case latestHuman == nil && title == "Human request":
			cp := note
			latestHuman = &cp
		case latestCEO == nil && title == "CEO handoff":
			cp := note
			latestCEO = &cp
		case latestCEO == nil && author == "ceo":
			cp := note
			latestCEO = &cp
		}
		if latestHuman != nil && latestCEO != nil {
			break
		}
	}
	out := make([]privateMemoryNote, 0, 2)
	if latestHuman != nil {
		out = append(out, *latestHuman)
	}
	if latestCEO != nil {
		if latestHuman == nil || latestCEO.Key != latestHuman.Key {
			out = append(out, *latestCEO)
		}
	}
	return out
}

func formatChannelMemoryBrief(channel string, entries map[string]string, query string) string {
	if normalizeChannelSlug(channel) == "" || len(entries) == 0 {
		return ""
	}
	matches := searchChannelMemory(entries, query, channelMemoryBriefLimit)
	if len(matches) == 0 {
		return ""
	}
	lines := []string{"== CHANNEL MEMORY =="}
	for _, note := range matches {
		label := strings.TrimSpace(note.Title)
		if label == "" {
			label = "Note"
		}
		if author := strings.TrimSpace(note.Author); author != "" {
			label = fmt.Sprintf("%s (@%s)", label, author)
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", label, truncate(strings.TrimSpace(strings.ReplaceAll(note.Content, "\n", " ")), 220)))
	}
	lines = append(lines, "== END CHANNEL MEMORY ==")
	return strings.Join(lines, "\n")
}

func fetchChannelMemoryBrief(channel string, notification string, broker *Broker) string {
	if broker == nil {
		return ""
	}
	namespace := channelMemoryNamespace(channel)
	broker.mu.Lock()
	entries := map[string]string{}
	if broker.sharedMemory != nil {
		if stored := broker.sharedMemory[namespace]; stored != nil {
			entries = make(map[string]string, len(stored))
			for key, value := range stored {
				entries[key] = value
			}
		}
	}
	broker.mu.Unlock()
	return formatChannelMemoryBrief(channel, entries, notification)
}

func (b *Broker) reconcileChannelMessageNotesLocked() {
	if b == nil || len(b.sharedMemory) == 0 {
		return
	}
	expectedByNamespace := make(map[string]map[string]string)
	for _, msg := range b.messages {
		key, note, ok := channelMemoryMessageNote(msg)
		if !ok {
			continue
		}
		namespace := channelMemoryNamespace(msg.Channel)
		if expectedByNamespace[namespace] == nil {
			expectedByNamespace[namespace] = make(map[string]string)
		}
		expectedByNamespace[namespace][key] = encodePrivateMemoryNote(note)
	}
	for namespace, entries := range b.sharedMemory {
		if !strings.HasPrefix(namespace, channelMemoryScope+"/") || len(entries) == 0 {
			continue
		}
		for key := range entries {
			if !strings.HasPrefix(key, "msg:") {
				continue
			}
			expectedEntries := expectedByNamespace[namespace]
			expected, ok := expectedEntries[key]
			if ok {
				entries[key] = expected
				continue
			}
			delete(entries, key)
		}
		for key, value := range expectedByNamespace[namespace] {
			entries[key] = value
		}
		if len(entries) == 0 {
			delete(b.sharedMemory, namespace)
		}
	}
	for namespace, entries := range expectedByNamespace {
		if len(entries) == 0 {
			continue
		}
		if b.sharedMemory[namespace] == nil {
			b.sharedMemory[namespace] = make(map[string]string)
		}
		for key, value := range entries {
			b.sharedMemory[namespace][key] = value
		}
	}
}

func channelFromMemoryNamespace(namespace string) (string, bool) {
	if !strings.HasPrefix(strings.TrimSpace(namespace), channelMemoryScope+"/") {
		return "", false
	}
	channel := normalizeChannelSlug(strings.TrimPrefix(strings.TrimSpace(namespace), channelMemoryScope+"/"))
	if channel == "" {
		channel = "general"
	}
	return channel, true
}

func recoverMessageIDFromChannelMemoryKey(key string) string {
	if !strings.HasPrefix(strings.TrimSpace(key), "msg:") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(key), "msg:"))
}

func syntheticMessageIDFromChannelMemoryKey(channel string, key string) string {
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		channel = "general"
	}
	key = strings.TrimSpace(key)
	key = strings.NewReplacer(":", "-", "/", "-", "\\", "-", " ", "-").Replace(key)
	key = slugify(key)
	if key == "" {
		key = "note"
	}
	return "memory-" + channel + "-" + key
}

func isRecoverableChannelMemoryMessageKey(key string) bool {
	key = strings.TrimSpace(key)
	return strings.HasPrefix(key, "msg:") ||
		strings.HasPrefix(key, "action:") ||
		strings.HasPrefix(key, "decision:")
}

func syntheticChannelMessageFromNote(channel, key string, note privateMemoryNote) (channelMessage, bool) {
	if !isRecoverableChannelMemoryMessageKey(key) {
		return channelMessage{}, false
	}
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		channel = "general"
	}
	timestamp := strings.TrimSpace(firstNonEmpty(note.CreatedAt, note.UpdatedAt))
	if timestamp == "" {
		timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	id := recoverMessageIDFromChannelMemoryKey(key)
	kind := ""
	title := ""
	if id == "" {
		id = syntheticMessageIDFromChannelMemoryKey(channel, key)
		kind = "system"
		title = strings.TrimSpace(note.Title)
	}
	from := strings.TrimSpace(note.Author)
	if from == "" {
		from = "system"
	}
	content := strings.TrimSpace(note.Content)
	if content == "" {
		return channelMessage{}, false
	}
	return channelMessage{
		ID:        id,
		From:      from,
		Channel:   channel,
		Kind:      kind,
		Title:     title,
		Content:   content,
		Timestamp: timestamp,
	}, true
}

func (b *Broker) recoverMessagesFromChannelMemoryLocked() int {
	if b == nil || len(b.sharedMemory) == 0 {
		return 0
	}
	known := make(map[string]struct{}, len(b.messages))
	for _, msg := range b.messages {
		if id := strings.TrimSpace(msg.ID); id != "" {
			known[id] = struct{}{}
		}
	}
	recovered := make([]channelMessage, 0)
	for namespace, entries := range b.sharedMemory {
		channel, ok := channelFromMemoryNamespace(namespace)
		if !ok || len(entries) == 0 {
			continue
		}
		for key, raw := range entries {
			id := recoverMessageIDFromChannelMemoryKey(key)
			if id == "" {
				continue
			}
			if _, exists := known[id]; exists {
				continue
			}
			note := decodePrivateMemoryNote(key, raw)
			msg, ok := syntheticChannelMessageFromNote(channel, key, note)
			if !ok {
				continue
			}
			known[msg.ID] = struct{}{}
			recovered = append(recovered, msg)
		}
	}
	if len(recovered) == 0 {
		return 0
	}
	sort.Slice(recovered, func(i, j int) bool {
		left, right := recovered[i].Timestamp, recovered[j].Timestamp
		if left == right {
			return recovered[i].ID < recovered[j].ID
		}
		return left < right
	})
	b.messages = append(b.messages, recovered...)
	return len(recovered)
}

func parseRecoveredTaskActionKey(key string) (taskID string, actionKind string, ok bool) {
	parts := strings.Split(strings.TrimSpace(key), ":")
	if len(parts) != 3 || parts[0] != "action" {
		return "", "", false
	}
	taskID = strings.TrimSpace(parts[1])
	actionKind = strings.TrimSpace(parts[2])
	if taskID == "" || actionKind == "" {
		return "", "", false
	}
	switch actionKind {
	case "task_created", "task_updated", "task_unblocked":
		return taskID, actionKind, true
	default:
		return "", "", false
	}
}

func normalizeRecoveredTaskStatus(status string) string {
	status = normalizeChannelSlug(status)
	switch status {
	case "done", "completed":
		return "done"
	case "blocked":
		return "blocked"
	case "review":
		return "review"
	case "open":
		return "open"
	case "in-progress", "in_progress":
		return "in_progress"
	case "cancelled", "canceled":
		return "canceled"
	default:
		return ""
	}
}

func parseRecoveredTaskSummary(summary string) (owner string, title string, status string) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return "", "", ""
	}
	if strings.HasPrefix(summary, "@") {
		fields := strings.Fields(summary)
		if len(fields) > 0 {
			owner = normalizeChannelSlug(strings.TrimPrefix(fields[0], "@"))
			summary = strings.TrimSpace(strings.TrimPrefix(summary, fields[0]))
		}
	}
	if open := strings.LastIndex(summary, "["); open > 0 && strings.HasSuffix(summary, "]") {
		if normalized := normalizeRecoveredTaskStatus(summary[open+1 : len(summary)-1]); normalized != "" {
			status = normalized
			summary = strings.TrimSpace(summary[:open])
		}
	}
	title = strings.TrimSpace(summary)
	return owner, title, status
}

func schedulerJobRelevantAt(job schedulerJob) string {
	return strings.TrimSpace(firstNonEmpty(job.LastRun, job.NextRun, job.DueAt))
}

func (b *Broker) conversationFollowUpAuditCanceledLocked() bool {
	if b == nil {
		return false
	}
	for _, job := range b.scheduler {
		job = normalizeSchedulerJob(job)
		if strings.TrimSpace(job.Slug) == ceoConversationFollowUpJobSlug ||
			strings.TrimSpace(job.Kind) == ceoConversationFollowUpJobKind ||
			strings.TrimSpace(job.TargetType) == ceoConversationFollowUpJobTargetType {
			return strings.EqualFold(strings.TrimSpace(job.Status), "canceled")
		}
	}
	return false
}

func (b *Broker) suppressActiveFollowUpTasksWhenAuditCanceledLocked(now string) int {
	if b == nil || !b.conversationFollowUpAuditCanceledLocked() {
		return 0
	}
	if strings.TrimSpace(now) == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	changed := 0
	for i := range b.tasks {
		task := &b.tasks[i]
		if taskIsTerminal(task) {
			continue
		}
		title := strings.TrimSpace(task.Title)
		if !strings.EqualFold(strings.TrimSpace(task.TaskType), "follow_up") &&
			!strings.EqualFold(strings.TrimSpace(task.PipelineID), "follow_up") &&
			!strings.HasPrefix(title, "Validate unanswered CEO follow-up") &&
			!strings.HasPrefix(title, "Reply to pending message from @human") {
			continue
		}
		task.Status = "done"
		task.Blocked = false
		task.ReviewState = "not_required"
		task.UpdatedAt = now
		changed++
	}
	return changed
}

func (b *Broker) recoverTasksFromChannelMemoryLocked() int {
	if b == nil || len(b.sharedMemory) == 0 {
		return 0
	}
	existing := make(map[string]int, len(b.tasks))
	for i := range b.tasks {
		if id := strings.TrimSpace(b.tasks[i].ID); id != "" {
			existing[id] = i
		}
	}
	recovered := make(map[string]*teamTask)
	for namespace, entries := range b.sharedMemory {
		channel, ok := channelFromMemoryNamespace(namespace)
		if !ok || len(entries) == 0 {
			continue
		}
		for key, raw := range entries {
			taskID, actionKind, ok := parseRecoveredTaskActionKey(key)
			if !ok {
				continue
			}
			note := decodePrivateMemoryNote(key, raw)
			owner, title, status := parseRecoveredTaskSummary(note.Content)
			if status == "" {
				switch actionKind {
				case "task_unblocked":
					status = "in_progress"
				case "task_created":
					status = "open"
				}
			}
			target, ok := recovered[taskID]
			if !ok {
				if idx, exists := existing[taskID]; exists {
					target = &b.tasks[idx]
				} else {
					target = &teamTask{
						ID:        taskID,
						Channel:   channel,
						Status:    "open",
						CreatedAt: strings.TrimSpace(firstNonEmpty(note.CreatedAt, note.UpdatedAt)),
						UpdatedAt: strings.TrimSpace(firstNonEmpty(note.UpdatedAt, note.CreatedAt)),
					}
					recovered[taskID] = target
				}
			}
			if taskIsTerminal(target) {
				continue
			}
			if strings.TrimSpace(target.Channel) == "" {
				target.Channel = channel
			}
			if strings.TrimSpace(target.Title) == "" && title != "" {
				target.Title = title
			}
			if strings.TrimSpace(target.Owner) == "" && owner != "" {
				target.Owner = owner
			}
			if strings.TrimSpace(target.CreatedBy) == "" && strings.TrimSpace(note.Author) != "" {
				target.CreatedBy = strings.TrimSpace(note.Author)
			}
			if createdAt := strings.TrimSpace(note.CreatedAt); createdAt != "" && (strings.TrimSpace(target.CreatedAt) == "" || createdAt < target.CreatedAt) {
				target.CreatedAt = createdAt
			}
			if updatedAt := strings.TrimSpace(firstNonEmpty(note.UpdatedAt, note.CreatedAt)); updatedAt != "" && updatedAt >= strings.TrimSpace(target.UpdatedAt) {
				target.UpdatedAt = updatedAt
				if title != "" {
					target.Title = title
				}
				if owner != "" {
					target.Owner = owner
				}
				if status != "" {
					target.Status = status
				}
				target.Blocked = target.Status == "blocked"
			}
			if actionKind == "task_unblocked" {
				target.Blocked = false
				if strings.TrimSpace(target.Status) == "" || target.Status == "blocked" || target.Status == "open" {
					target.Status = "in_progress"
				}
			}
			if strings.TrimSpace(target.Status) == "" {
				target.Status = "open"
			}
		}
	}
	if len(recovered) == 0 {
		return 0
	}
	for _, job := range b.scheduler {
		if strings.TrimSpace(job.TargetType) != "task" {
			continue
		}
		taskID := strings.TrimSpace(job.TargetID)
		target := recovered[taskID]
		if target == nil {
			continue
		}
		if strings.TrimSpace(target.Channel) == "" && strings.TrimSpace(job.Channel) != "" {
			target.Channel = normalizeChannelSlug(job.Channel)
		}
		if strings.TrimSpace(target.Title) == "" && strings.HasPrefix(strings.TrimSpace(job.Label), "Follow up on ") {
			target.Title = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(job.Label), "Follow up on "))
		}
		if payload := strings.TrimSpace(job.Payload); payload != "" {
			relevantAt := schedulerJobRelevantAt(job)
			if strings.TrimSpace(target.Details) == "" || (relevantAt != "" && relevantAt >= strings.TrimSpace(target.UpdatedAt)) {
				target.Details = payload
				if relevantAt != "" && relevantAt >= strings.TrimSpace(target.UpdatedAt) {
					target.UpdatedAt = relevantAt
				}
			}
		}
		switch strings.TrimSpace(job.Kind) {
		case "task_follow_up":
			if next := strings.TrimSpace(firstNonEmpty(job.NextRun, job.DueAt)); next != "" {
				target.FollowUpAt = next
			}
		case "recheck":
			if next := strings.TrimSpace(firstNonEmpty(job.NextRun, job.DueAt)); next != "" {
				target.RecheckAt = next
			}
		}
	}
	recoveredList := make([]teamTask, 0, len(recovered))
	for taskID, task := range recovered {
		if _, exists := existing[taskID]; exists {
			continue
		}
		if strings.TrimSpace(task.Channel) == "" {
			task.Channel = "general"
		}
		if strings.TrimSpace(task.Title) == "" {
			task.Title = taskID
		}
		if strings.TrimSpace(task.CreatedAt) == "" {
			task.CreatedAt = strings.TrimSpace(firstNonEmpty(task.UpdatedAt, time.Now().UTC().Format(time.RFC3339)))
		}
		if strings.TrimSpace(task.UpdatedAt) == "" {
			task.UpdatedAt = task.CreatedAt
		}
		if strings.TrimSpace(task.Status) == "" {
			task.Status = "open"
		}
		task.Blocked = task.Status == "blocked"
		recoveredList = append(recoveredList, *task)
	}
	if len(recoveredList) == 0 {
		return 0
	}
	sort.Slice(recoveredList, func(i, j int) bool {
		if recoveredList[i].CreatedAt == recoveredList[j].CreatedAt {
			return recoveredList[i].ID < recoveredList[j].ID
		}
		return recoveredList[i].CreatedAt < recoveredList[j].CreatedAt
	})
	b.tasks = append(b.tasks, recoveredList...)
	return len(recoveredList)
}

func (b *Broker) syntheticChannelMessagesLocked(channel string) []channelMessage {
	if b == nil || len(b.sharedMemory) == 0 {
		return nil
	}
	namespace := channelMemoryNamespace(channel)
	entries := b.sharedMemory[namespace]
	if len(entries) == 0 {
		return nil
	}
	messages := make([]channelMessage, 0, len(entries))
	for key, raw := range entries {
		note := decodePrivateMemoryNote(key, raw)
		msg, ok := syntheticChannelMessageFromNote(channel, key, note)
		if !ok {
			continue
		}
		messages = append(messages, msg)
	}
	if len(messages) == 0 {
		return nil
	}
	sort.Slice(messages, func(i, j int) bool {
		if messages[i].Timestamp == messages[j].Timestamp {
			return messages[i].ID < messages[j].ID
		}
		return messages[i].Timestamp < messages[j].Timestamp
	})
	return messages
}

type ChannelMemoryRebuildStat struct {
	Channel string
	Before  int
	After   int
}

func selectedChannelSet(channels []string) map[string]struct{} {
	if len(channels) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(channels))
	for _, channel := range channels {
		channel = normalizeChannelSlug(channel)
		if channel == "" {
			continue
		}
		set[channel] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

func (b *Broker) rebuildChannelMemoryLocked(channels []string) []ChannelMemoryRebuildStat {
	if b == nil {
		return nil
	}
	selected := selectedChannelSet(channels)
	beforeCounts := make(map[string]int)
	existingNotes := make(map[string]map[string]privateMemoryNote)
	channelOrder := make([]string, 0, len(b.channels))
	seenChannels := make(map[string]struct{}, len(b.channels))
	addChannel := func(channel string) {
		channel = normalizeChannelSlug(channel)
		if channel == "" {
			channel = "general"
		}
		if selected != nil {
			if _, ok := selected[channel]; !ok {
				return
			}
		}
		if _, ok := seenChannels[channel]; ok {
			return
		}
		seenChannels[channel] = struct{}{}
		channelOrder = append(channelOrder, channel)
	}
	for _, channel := range b.channels {
		addChannel(channel.Slug)
	}
	if b.sharedMemory == nil {
		b.sharedMemory = make(map[string]map[string]string)
	}
	for namespace, entries := range b.sharedMemory {
		if !strings.HasPrefix(namespace, channelMemoryScope+"/") {
			continue
		}
		channel := strings.TrimPrefix(namespace, channelMemoryScope+"/")
		if selected != nil {
			if _, ok := selected[channel]; !ok {
				continue
			}
		}
		beforeCounts[channel] = len(entries)
		addChannel(channel)
		if existingNotes[channel] == nil {
			existingNotes[channel] = make(map[string]privateMemoryNote, len(entries))
		}
		for key, raw := range entries {
			existingNotes[channel][key] = decodePrivateMemoryNote(key, raw)
		}
		delete(b.sharedMemory, namespace)
	}

	rebuilt := make(map[string]map[string]privateMemoryNote)
	upsert := func(channel, key string, note privateMemoryNote) {
		channel = normalizeChannelSlug(channel)
		if channel == "" {
			channel = "general"
		}
		if selected != nil {
			if _, ok := selected[channel]; !ok {
				return
			}
		}
		addChannel(channel)
		if rebuilt[channel] == nil {
			rebuilt[channel] = make(map[string]privateMemoryNote)
		}
		if existing, ok := rebuilt[channel][key]; ok {
			if noteTimestamp(existing).After(noteTimestamp(note)) {
				return
			}
		}
		note.Key = strings.TrimSpace(key)
		rebuilt[channel][key] = note
	}
	for _, msg := range b.messages {
		if key, note, ok := channelMemoryMessageNote(msg); ok {
			upsert(msg.Channel, key, note)
		}
	}
	for _, action := range b.actions {
		if key, note, ok := channelMemoryActionNote(action); ok {
			upsert(action.Channel, key, note)
		}
	}
	for _, decision := range b.decisions {
		if key, note, ok := channelMemoryDecisionNote(decision); ok {
			upsert(decision.Channel, key, note)
		}
	}
	stats := make([]ChannelMemoryRebuildStat, 0, len(channelOrder))
	for _, channel := range channelOrder {
		namespace := channelMemoryNamespace(channel)
		entries := make(map[string]string)
		for key, note := range existingNotes[channel] {
			if strings.HasPrefix(key, "msg:") {
				continue
			}
			if _, ok := rebuilt[channel][key]; ok {
				continue
			}
			entries[key] = encodePrivateMemoryNote(note)
		}
		for key, note := range rebuilt[channel] {
			entries[key] = encodePrivateMemoryNote(note)
		}
		pruneChannelMemoryEntries(entries, channelMemoryEntryLimit)
		if len(entries) > 0 {
			b.sharedMemory[namespace] = entries
		}
		stats = append(stats, ChannelMemoryRebuildStat{
			Channel: channel,
			Before:  beforeCounts[channel],
			After:   len(entries),
		})
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].After != stats[j].After {
			return stats[i].After > stats[j].After
		}
		return stats[i].Channel < stats[j].Channel
	})
	return stats
}

func RepairChannelMemory(channels ...string) ([]ChannelMemoryRebuildStat, error) {
	b := NewBroker()
	b.mu.Lock()
	defer b.mu.Unlock()
	stats := b.rebuildChannelMemoryLocked(channels)
	if err := b.saveLocked(); err != nil {
		return nil, err
	}
	return stats, nil
}

func fetchScopedMemoryBrief(ctx context.Context, slug string, notification string, broker *Broker) string {
	query := strings.TrimSpace(notification)
	if query == "" {
		return ""
	}
	var blocks []string
	if broker != nil {
		broker.mu.Lock()
		entries := map[string]string{}
		if broker.sharedMemory != nil {
			if stored := broker.sharedMemory[privateMemoryNamespace(slug)]; stored != nil {
				entries = make(map[string]string, len(stored))
				for key, value := range stored {
					entries[key] = value
				}
			}
		}
		broker.mu.Unlock()
		if brief := formatPrivateMemoryBrief(slug, entries, query); brief != "" {
			blocks = append(blocks, brief)
		}
	}
	if brief := fetchMemoryBrief(ctx, notification); brief != "" {
		blocks = append(blocks, brief)
	}
	return strings.Join(blocks, "\n\n")
}
