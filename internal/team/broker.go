package team

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	wuphf "github.com/nex-crm/wuphf"
	"github.com/nex-crm/wuphf/internal/action"
	"github.com/nex-crm/wuphf/internal/agent"
	"github.com/nex-crm/wuphf/internal/atomicfile"
	"github.com/nex-crm/wuphf/internal/backup"
	"github.com/nex-crm/wuphf/internal/brokeraddr"
	"github.com/nex-crm/wuphf/internal/buildinfo"
	"github.com/nex-crm/wuphf/internal/channel"
	"github.com/nex-crm/wuphf/internal/company"
	"github.com/nex-crm/wuphf/internal/config"
	"github.com/nex-crm/wuphf/internal/onboarding"
	"github.com/nex-crm/wuphf/internal/operations"
	"github.com/nex-crm/wuphf/internal/provider"
	"github.com/nex-crm/wuphf/internal/workspace"
)

const BrokerPort = brokeraddr.DefaultPort

// brokerTokenFilePath is the path where the broker writes its auth token on start.
// Tests can redirect this to a temp directory to avoid clobbering the live broker token.
var brokerTokenFilePath = brokeraddr.DefaultTokenFile

const defaultRateLimitRequestsPerWindow = 600
const defaultRateLimitWindow = time.Minute
const mutationAckNamespace = "mutation_acks"

var brokerStatePath = defaultBrokerStatePath
var atomicReplaceBrokerStateFile = atomicfile.Replace

var studioPackageGenerator = provider.RunCodexOneShot

var externalRetryAfterPattern = regexp.MustCompile(`(?i)retry after ([0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.+-]+Z?)`)
var brokerTextMentionPattern = regexp.MustCompile(`(^|[^A-Za-z0-9._-])@([a-z0-9][a-z0-9-]*)`)
var runBrokerCloudBackupAsync = func(fn func()) { go fn() }
var sleepBrokerCloudBackup = time.Sleep

type brokerCloudBackupPlan struct {
	settings       backup.Settings
	data           []byte
	historyName    string
	writeCurrent   bool
	writeSnapshot  bool
	writeHistory   bool
	deleteCurrent  bool
	deleteSnapshot bool
}

type brokerTaskIndexes struct {
	byID        map[string]int
	byChannel   map[string][]int
	byOwner     map[string][]int
	byStatus    map[string][]int
	byTaskType  map[string][]int
	counterSeen int
	sizeSeen    int
}

type brokerReadinessSnapshot struct {
	Live      bool     `json:"live"`
	Ready     bool     `json:"ready"`
	State     string   `json:"state,omitempty"`
	Stage     string   `json:"stage,omitempty"`
	Summary   string   `json:"summary,omitempty"`
	Signals   []string `json:"signals,omitempty"`
	CheckedAt string   `json:"checked_at,omitempty"`
}

type brokerHTTPMetric struct {
	Path            string `json:"path"`
	Requests        int    `json:"requests"`
	LastStatus      int    `json:"last_status,omitempty"`
	LastDurationMs  int64  `json:"last_duration_ms,omitempty"`
	TotalDurationMs int64  `json:"total_duration_ms,omitempty"`
	MaxDurationMs   int64  `json:"max_duration_ms,omitempty"`
	LastSeenAt      string `json:"last_seen_at,omitempty"`
}

type brokerCloudBackupRuntime struct {
	Pending             bool   `json:"pending"`
	LastAttemptAt       string `json:"last_attempt_at,omitempty"`
	LastSuccessAt       string `json:"last_success_at,omitempty"`
	LastError           string `json:"last_error,omitempty"`
	ConsecutiveFailures int    `json:"consecutive_failures,omitempty"`
}

type brokerDeferredCloudRestore struct {
	Pending     bool
	BaseCounter int
	BaseScore   int
}

type brokerStatusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *brokerStatusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *brokerStatusRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(data)
}

var (
	brokerCloudBackupQueueMu      sync.Mutex
	brokerCloudBackupPendingPlan  *brokerCloudBackupPlan
	brokerCloudBackupWorkerActive bool
	brokerCloudBackupState        brokerCloudBackupRuntime
)

const (
	brokerCloudBackupDebounceWindow = 750 * time.Millisecond
	schedulerTerminalRetention      = 2 * time.Hour
)

func brokerDebugHTTPTimingEnabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("WUPHF_DEBUG_HTTP_TIMING")))
	return raw == "1" || raw == "true" || raw == "yes"
}

func cloneBrokerReadinessSnapshot(snapshot brokerReadinessSnapshot) brokerReadinessSnapshot {
	cloned := snapshot
	if len(snapshot.Signals) > 0 {
		cloned.Signals = append([]string(nil), snapshot.Signals...)
	}
	return cloned
}

func (b *Broker) SetReadinessSnapshot(snapshot brokerReadinessSnapshot) {
	if b == nil {
		return
	}
	snapshot.State = strings.TrimSpace(snapshot.State)
	if snapshot.State == "" {
		if snapshot.Ready {
			snapshot.State = "ready"
		} else {
			snapshot.State = "starting"
		}
	}
	snapshot.Stage = strings.TrimSpace(snapshot.Stage)
	if snapshot.CheckedAt == "" {
		snapshot.CheckedAt = time.Now().UTC().Format(time.RFC3339)
	}
	b.observabilityMu.Lock()
	b.readiness = cloneBrokerReadinessSnapshot(snapshot)
	b.observabilityMu.Unlock()
}

func (b *Broker) readinessSnapshot() brokerReadinessSnapshot {
	if b == nil {
		return brokerReadinessSnapshot{Live: true, Ready: true, State: "ready", Stage: "broker"}
	}
	b.observabilityMu.Lock()
	defer b.observabilityMu.Unlock()
	return cloneBrokerReadinessSnapshot(b.readiness)
}

func (b *Broker) recordHTTPMetric(path string, status int, duration time.Duration) {
	if b == nil {
		return
	}
	path = strings.TrimSpace(path)
	if path == "" {
		path = "/"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	durationMs := duration.Milliseconds()
	b.observabilityMu.Lock()
	defer b.observabilityMu.Unlock()
	if b.httpMetrics == nil {
		b.httpMetrics = make(map[string]brokerHTTPMetric)
	}
	metric := b.httpMetrics[path]
	metric.Path = path
	metric.Requests++
	metric.LastStatus = status
	metric.LastDurationMs = durationMs
	metric.TotalDurationMs += durationMs
	if durationMs > metric.MaxDurationMs {
		metric.MaxDurationMs = durationMs
	}
	metric.LastSeenAt = now
	b.httpMetrics[path] = metric
}

func (b *Broker) httpMetricSnapshot(limit int) []brokerHTTPMetric {
	if b == nil || limit == 0 {
		return nil
	}
	b.observabilityMu.Lock()
	defer b.observabilityMu.Unlock()
	if len(b.httpMetrics) == 0 {
		return nil
	}
	out := make([]brokerHTTPMetric, 0, len(b.httpMetrics))
	for _, metric := range b.httpMetrics {
		out = append(out, metric)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MaxDurationMs == out[j].MaxDurationMs {
			if out[i].LastDurationMs == out[j].LastDurationMs {
				return out[i].Path < out[j].Path
			}
			return out[i].LastDurationMs > out[j].LastDurationMs
		}
		return out[i].MaxDurationMs > out[j].MaxDurationMs
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func cloneBrokerCloudBackupRuntime(runtime brokerCloudBackupRuntime) brokerCloudBackupRuntime {
	return runtime
}

func updateBrokerCloudBackupRuntime(update func(*brokerCloudBackupRuntime)) {
	brokerCloudBackupQueueMu.Lock()
	defer brokerCloudBackupQueueMu.Unlock()
	update(&brokerCloudBackupState)
}

func brokerCloudBackupRuntimeSnapshot() brokerCloudBackupRuntime {
	brokerCloudBackupQueueMu.Lock()
	defer brokerCloudBackupQueueMu.Unlock()
	return cloneBrokerCloudBackupRuntime(brokerCloudBackupState)
}

func (b *Broker) httpMetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/events") || strings.HasPrefix(r.URL.Path, "/agent-stream/") {
			next.ServeHTTP(w, r)
			return
		}
		recorder := &brokerStatusRecorder{ResponseWriter: w}
		startedAt := time.Now()
		next.ServeHTTP(recorder, r)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		b.recordHTTPMetric(r.URL.Path, status, time.Since(startedAt))
	})
}

func schedulerJobSemanticKey(job schedulerJob) string {
	job = normalizeSchedulerJob(job)
	if slug := strings.TrimSpace(job.Slug); slug != "" {
		return "slug:" + slug
	}
	return strings.Join([]string{
		"job",
		firstNonEmpty(job.Kind, "_"),
		firstNonEmpty(job.TargetType, "_"),
		firstNonEmpty(job.TargetID, "_"),
		firstNonEmpty(job.Channel, "_"),
		firstNonEmpty(job.Provider, "_"),
		firstNonEmpty(job.WorkflowKey, "_"),
		firstNonEmpty(job.SkillName, "_"),
	}, "|")
}

func schedulerJobIsTerminal(job schedulerJob) bool {
	return schedulerStatusIsTerminal(job.Status)
}

// agentStreamBuffer holds recent stdout/stderr lines from a headless agent
// process and fans them out to SSE subscribers in real time.
type agentStreamBuffer struct {
	mu     sync.Mutex
	lines  []string
	subs   map[int]chan string
	nextID int
}

func (s *agentStreamBuffer) Push(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = append(s.lines, line)
	if len(s.lines) > 2000 {
		s.lines = s.lines[len(s.lines)-2000:]
	}
	for _, ch := range s.subs {
		select {
		case ch <- line:
		default:
		}
	}
}

func (s *agentStreamBuffer) subscribe() (<-chan string, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextID
	s.nextID++
	ch := make(chan string, 128)
	s.subs[id] = ch
	return ch, func() {
		s.mu.Lock()
		delete(s.subs, id)
		s.mu.Unlock()
	}
}

func (s *agentStreamBuffer) recent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.lines))
	copy(out, s.lines)
	return out
}

type messageReaction struct {
	Emoji string `json:"emoji"`
	From  string `json:"from"`
}

type channelMessage struct {
	ID          string            `json:"id"`
	ClientID    string            `json:"client_id,omitempty"`
	From        string            `json:"from"`
	Channel     string            `json:"channel,omitempty"`
	CanDelete   bool              `json:"can_delete,omitempty"`
	Kind        string            `json:"kind,omitempty"`
	Source      string            `json:"source,omitempty"`
	SourceLabel string            `json:"source_label,omitempty"`
	EventID     string            `json:"event_id,omitempty"`
	Title       string            `json:"title,omitempty"`
	Content     string            `json:"content"`
	Tagged      []string          `json:"tagged"`
	ReplyTo     string            `json:"reply_to,omitempty"`
	Timestamp   string            `json:"timestamp"`
	Usage       *messageUsage     `json:"usage,omitempty"`
	Reactions   []messageReaction `json:"reactions,omitempty"`
}

type messageUsage struct {
	InputTokens         int `json:"input_tokens,omitempty"`
	OutputTokens        int `json:"output_tokens,omitempty"`
	CacheReadTokens     int `json:"cache_read_tokens,omitempty"`
	CacheCreationTokens int `json:"cache_creation_tokens,omitempty"`
	TotalTokens         int `json:"total_tokens,omitempty"`
}

type interviewOption struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Description  string `json:"description"`
	RequiresText bool   `json:"requires_text,omitempty"`
	TextHint     string `json:"text_hint,omitempty"`
}

type interviewAnswer struct {
	ChoiceID   string `json:"choice_id,omitempty"`
	ChoiceText string `json:"choice_text,omitempty"`
	CustomText string `json:"custom_text,omitempty"`
	AnsweredAt string `json:"answered_at,omitempty"`
}

type humanInterview struct {
	ID                        string            `json:"id"`
	Kind                      string            `json:"kind,omitempty"`
	Status                    string            `json:"status,omitempty"`
	From                      string            `json:"from"`
	Channel                   string            `json:"channel,omitempty"`
	Title                     string            `json:"title,omitempty"`
	Question                  string            `json:"question"`
	Context                   string            `json:"context,omitempty"`
	Options                   []interviewOption `json:"options,omitempty"`
	RecommendedID             string            `json:"recommended_id,omitempty"`
	Blocking                  bool              `json:"blocking,omitempty"`
	Required                  bool              `json:"required,omitempty"`
	Secret                    bool              `json:"secret,omitempty"`
	ReplyTo                   string            `json:"reply_to,omitempty"`
	DueAt                     string            `json:"due_at,omitempty"`
	FollowUpAt                string            `json:"follow_up_at,omitempty"`
	ReminderAt                string            `json:"reminder_at,omitempty"`
	RecheckAt                 string            `json:"recheck_at,omitempty"`
	SLAHours                  int               `json:"sla_hours,omitempty"`
	NextEscalationAt          string            `json:"next_escalation_at,omitempty"`
	LastEscalatedAt           string            `json:"last_escalated_at,omitempty"`
	EscalationCount           int               `json:"escalation_count,omitempty"`
	EscalationState           string            `json:"escalation_state,omitempty"`
	CreatedAt                 string            `json:"created_at"`
	UpdatedAt                 string            `json:"updated_at,omitempty"`
	SourceTaskID              string            `json:"source_task_id,omitempty"`
	SourceBlocker             string            `json:"source_blocker,omitempty"`
	RecommendationStatus      string            `json:"recommendation_status,omitempty"`
	RecommendationTaskID      string            `json:"recommendation_task_id,omitempty"`
	RecommendationRequestedAt string            `json:"recommendation_requested_at,omitempty"`
	Answered                  *interviewAnswer  `json:"answered,omitempty"`
}

type teamTask struct {
	ID                     string                       `json:"id"`
	Channel                string                       `json:"channel,omitempty"`
	ExecutionKey           string                       `json:"execution_key,omitempty"`
	Title                  string                       `json:"title"`
	Details                string                       `json:"details,omitempty"`
	Owner                  string                       `json:"owner,omitempty"`
	Status                 string                       `json:"status"`
	CreatedBy              string                       `json:"created_by"`
	ThreadID               string                       `json:"thread_id,omitempty"`
	TaskType               string                       `json:"task_type,omitempty"`
	PipelineID             string                       `json:"pipeline_id,omitempty"`
	PipelineStage          string                       `json:"pipeline_stage,omitempty"`
	ExecutionMode          string                       `json:"execution_mode,omitempty"`
	ReviewState            string                       `json:"review_state,omitempty"`
	SourceSignalID         string                       `json:"source_signal_id,omitempty"`
	SourceDecisionID       string                       `json:"source_decision_id,omitempty"`
	WorkspacePath          string                       `json:"workspace_path,omitempty"`
	WorktreePath           string                       `json:"worktree_path,omitempty"`
	WorktreeBranch         string                       `json:"worktree_branch,omitempty"`
	DependsOn              []string                     `json:"depends_on,omitempty"`
	Blocked                bool                         `json:"blocked,omitempty"`
	AckedAt                string                       `json:"acked_at,omitempty"`
	DueAt                  string                       `json:"due_at,omitempty"`
	FollowUpAt             string                       `json:"follow_up_at,omitempty"`
	ReminderAt             string                       `json:"reminder_at,omitempty"`
	RecheckAt              string                       `json:"recheck_at,omitempty"`
	CreatedAt              string                       `json:"created_at"`
	UpdatedAt              string                       `json:"updated_at"`
	HandoffStatus          string                       `json:"handoff_status,omitempty"`
	HandoffAcceptedAt      string                       `json:"handoff_accepted_at,omitempty"`
	BlockerRequestIDs      []string                     `json:"blocker_request_ids,omitempty"`
	ReviewFindings         []taskReviewFinding          `json:"review_findings,omitempty"`
	ReviewFindingHistory   []taskReviewFindingBatch     `json:"review_finding_history,omitempty"`
	LastHandoff            *taskHandoffRecord           `json:"last_handoff,omitempty"`
	Reconciliation         *taskReconciliationState     `json:"reconciliation,omitempty"`
	PublicationPolicy      *taskGitHubPublicationPolicy `json:"publication_policy,omitempty"`
	DerivedFrom            *taskDerivedDemandRef        `json:"derived_from,omitempty"`
	RepoContext            *taskGitHubRepoContext       `json:"repo_context,omitempty"`
	IssuePublication       *taskGitHubPublication       `json:"issue_publication,omitempty"`
	PRPublication          *taskGitHubPublication       `json:"pr_publication,omitempty"`
	AwaitingHuman          bool                         `json:"awaiting_human,omitempty"`
	AwaitingHumanSince     string                       `json:"awaiting_human_since,omitempty"`
	AwaitingHumanReason    string                       `json:"awaiting_human_reason,omitempty"`
	AwaitingHumanRequestID string                       `json:"awaiting_human_request_id,omitempty"`
	AwaitingHumanSource    string                       `json:"awaiting_human_source,omitempty"`
	RecommendedResponder   string                       `json:"recommended_responder,omitempty"`
	RecommendationStatus   string                       `json:"recommendation_status,omitempty"`
	RecommendationSummary  string                       `json:"recommendation_summary,omitempty"`
	RecommendationTaskID   string                       `json:"recommendation_task_id,omitempty"`
	SourceMessageID        string                       `json:"source_message_id,omitempty"`
	SourceRequestID        string                       `json:"source_request_id,omitempty"`
	SourceTaskID           string                       `json:"source_task_id,omitempty"`
	DeliveryID             string                       `json:"delivery_id,omitempty"`
	ProgressPercent        int                          `json:"progress_percent,omitempty"`
	ProgressBasis          string                       `json:"progress_basis,omitempty"`
	HumanOptions           []interviewOption            `json:"human_options,omitempty"`
	HumanRecommendedID     string                       `json:"human_recommended_id,omitempty"`
}

type channelSurface struct {
	Provider    string `json:"provider,omitempty"`
	RemoteID    string `json:"remote_id,omitempty"`
	RemoteTitle string `json:"remote_title,omitempty"`
	Mode        string `json:"mode,omitempty"`
	BotTokenEnv string `json:"bot_token_env,omitempty"`
	WebhookURL  string `json:"webhook_url,omitempty"`
}

type linkedRepoRef struct {
	RepoPath            string                       `json:"repo_path"`
	Source              string                       `json:"source,omitempty"`
	Primary             bool                         `json:"primary,omitempty"`
	PublicationDefaults *taskGitHubPublicationPolicy `json:"publication_defaults,omitempty"`
	CreatedAt           string                       `json:"created_at,omitempty"`
	UpdatedAt           string                       `json:"updated_at,omitempty"`
	CreatedBy           string                       `json:"created_by,omitempty"`
}

type teamChannel struct {
	Slug        string          `json:"slug"`
	Name        string          `json:"name"`
	Type        string          `json:"type,omitempty"` // "channel" (default) or "dm"
	Description string          `json:"description,omitempty"`
	Protected   bool            `json:"protected,omitempty"`
	Archived    bool            `json:"archived,omitempty"`
	Members     []string        `json:"members,omitempty"`
	Disabled    []string        `json:"disabled,omitempty"`
	LinkedRepos []linkedRepoRef `json:"linked_repos,omitempty"`
	Surface     *channelSurface `json:"surface,omitempty"`
	CreatedBy   string          `json:"created_by,omitempty"`
	CreatedAt   string          `json:"created_at,omitempty"`
	UpdatedAt   string          `json:"updated_at,omitempty"`
}

func shouldProtectUserChannel(slug string, createdBy string) bool {
	creator := strings.TrimSpace(createdBy)
	slug = normalizeChannelSlug(slug)
	if creator == "" || strings.EqualFold(creator, "wuphf") || strings.EqualFold(creator, "system") {
		return false
	}
	if slug == "" || slug == "general" || IsDMSlug(slug) {
		return false
	}
	return true
}

func normalizeRemovalConfirmToken(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.ToLower(raw)
	raw = strings.TrimPrefix(raw, "confirm:")
	raw = strings.TrimPrefix(raw, "delete:")
	return strings.TrimSpace(raw)
}

func teamChannelTypeFromStoreChannelType(t channel.ChannelType) string {
	switch t {
	case channel.ChannelTypeDirect, channel.ChannelTypeGroup:
		return "dm"
	default:
		return ""
	}
}

func (b *Broker) upsertPublicChannelInStoreLocked(ch teamChannel) {
	if b == nil || b.channelStore == nil {
		return
	}
	slug := normalizeChannelSlug(ch.Slug)
	if slug == "" || IsDMSlug(slug) {
		return
	}
	stored, ok := b.channelStore.GetBySlug(slug)
	if !ok || stored == nil {
		created, err := b.channelStore.Create(channel.Channel{
			Slug:        slug,
			Name:        firstNonEmpty(strings.TrimSpace(ch.Name), slug),
			Type:        channel.ChannelTypePublic,
			CreatedBy:   strings.TrimSpace(ch.CreatedBy),
			Description: strings.TrimSpace(ch.Description),
		})
		if err != nil {
			return
		}
		stored = created
	}
	if stored == nil {
		return
	}
	desiredMembers := uniqueSlugs(ch.Members)
	if len(desiredMembers) == 0 {
		desiredMembers = []string{"ceo"}
	}
	currentMembers := make(map[string]struct{})
	for _, member := range b.channelStore.Members(stored.ID) {
		if normalized := normalizeChannelSlug(member.Slug); normalized != "" {
			currentMembers[normalized] = struct{}{}
		}
	}
	for _, member := range desiredMembers {
		member = normalizeChannelSlug(member)
		if member == "" {
			continue
		}
		if _, ok := currentMembers[member]; !ok {
			_ = b.channelStore.AddMember(stored.ID, member, "all")
		}
		delete(currentMembers, member)
	}
	for member := range currentMembers {
		_ = b.channelStore.RemoveMember(stored.ID, member)
	}
}

func (b *Broker) removePublicChannelFromStoreLocked(slug string) {
	if b == nil || b.channelStore == nil {
		return
	}
	slug = normalizeChannelSlug(slug)
	if slug == "" || IsDMSlug(slug) {
		return
	}
	stored, ok := b.channelStore.GetBySlug(slug)
	if !ok || stored == nil {
		return
	}
	_ = b.channelStore.Delete(stored.ID)
}

func (b *Broker) recoverChannelsFromStoreLocked() int {
	if b == nil || b.channelStore == nil {
		return 0
	}
	existing := make(map[string]struct{}, len(b.channels))
	for _, ch := range b.channels {
		slug := normalizeChannelSlug(ch.Slug)
		if slug == "" {
			continue
		}
		existing[slug] = struct{}{}
	}
	recovered := 0
	for _, stored := range b.channelStore.List(channel.ChannelFilter{}) {
		slug := normalizeChannelSlug(stored.Slug)
		if slug == "" {
			continue
		}
		if _, ok := existing[slug]; ok {
			continue
		}
		members := b.channelStore.Members(stored.ID)
		memberSlugs := make([]string, 0, len(members))
		for _, member := range members {
			if normalized := normalizeChannelSlug(member.Slug); normalized != "" {
				memberSlugs = append(memberSlugs, normalized)
			}
		}
		tc := teamChannel{
			Slug:        slug,
			Name:        strings.TrimSpace(stored.Name),
			Type:        teamChannelTypeFromStoreChannelType(stored.Type),
			Description: strings.TrimSpace(stored.Description),
			Protected:   shouldProtectUserChannel(slug, stored.CreatedBy),
			Members:     uniqueSlugs(memberSlugs),
			CreatedBy:   strings.TrimSpace(stored.CreatedBy),
			CreatedAt:   strings.TrimSpace(stored.CreatedAt),
			UpdatedAt:   strings.TrimSpace(firstNonEmpty(stored.UpdatedAt, stored.CreatedAt)),
		}
		if tc.Name == "" {
			tc.Name = slug
		}
		if tc.Description == "" {
			tc.Description = defaultTeamChannelDescription(tc.Slug, tc.Name)
		}
		b.channels = append(b.channels, tc)
		existing[slug] = struct{}{}
		recovered++
	}
	return recovered
}

func (b *Broker) recoverChannelsFromReferencedStateLocked() int {
	if b == nil {
		return 0
	}
	existing := make(map[string]struct{}, len(b.channels))
	for _, ch := range b.channels {
		if slug := normalizeChannelSlug(ch.Slug); slug != "" {
			existing[slug] = struct{}{}
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	recovered := 0
	add := func(rawChannel string, createdBy string) {
		slug := normalizeChannelSlug(rawChannel)
		if slug == "" || IsDMSlug(slug) {
			return
		}
		if _, ok := existing[slug]; ok {
			return
		}
		ch := teamChannel{
			Slug:        slug,
			Name:        slug,
			Description: defaultTeamChannelDescription(slug, slug),
			Protected:   shouldProtectUserChannel(slug, createdBy),
			Members:     []string{"ceo"},
			CreatedBy:   strings.TrimSpace(createdBy),
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		b.channels = append(b.channels, ch)
		existing[slug] = struct{}{}
		recovered++
	}
	for namespace := range b.sharedMemory {
		if channel, ok := channelFromMemoryNamespace(namespace); ok {
			add(channel, "")
		}
	}
	for _, msg := range b.messages {
		add(msg.Channel, msg.From)
	}
	for _, task := range b.tasks {
		add(task.Channel, task.CreatedBy)
	}
	for _, req := range b.requests {
		add(req.Channel, req.From)
	}
	for _, action := range b.actions {
		add(action.Channel, action.Actor)
	}
	for _, signal := range b.signals {
		add(signal.Channel, signal.Owner)
	}
	for _, decision := range b.decisions {
		add(decision.Channel, decision.Owner)
	}
	for _, alert := range b.watchdogs {
		add(alert.Channel, alert.Owner)
	}
	for _, job := range b.scheduler {
		add(job.Channel, "")
	}
	for _, node := range b.executionNodes {
		add(node.Channel, node.OwnerAgent)
	}
	return recovered
}

func (b *Broker) recoverChannelsFromStateLocked() int {
	if b == nil {
		return 0
	}
	recovered := b.recoverChannelsFromStoreLocked()
	recovered += b.recoverChannelsFromReferencedStateLocked()
	return recovered
}

func (ch *teamChannel) isDM() bool {
	return ch.Type == "dm" || IsDMSlug(ch.Slug)
}

func (ch *teamChannel) primaryLinkedRepoPath() string {
	if ch == nil {
		return ""
	}
	for _, repo := range ch.LinkedRepos {
		if repo.Primary && strings.TrimSpace(repo.RepoPath) != "" {
			return strings.TrimSpace(repo.RepoPath)
		}
	}
	if len(ch.LinkedRepos) == 0 {
		return ""
	}
	return strings.TrimSpace(ch.LinkedRepos[0].RepoPath)
}

var linkedRepoPathMentionPattern = regexp.MustCompile(`(?i)[a-z]:[\\/][^\s"'` + "`" + `,;)\]]+`)

func normalizeLinkedRepoPath(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	target, ok := explicitWorkspaceTarget(raw)
	if !ok {
		return "", false
	}
	target = filepath.Clean(strings.TrimSpace(target))
	if !taskWorkspacePathLooksEligible(target) && !taskWorktreeSourceLooksUsable(target) {
		return "", false
	}
	return target, true
}

func shouldPromoteLinkedRepoSource(source string) bool {
	return strings.EqualFold(strings.TrimSpace(source), "human_message")
}

func extractLinkedRepoPathsFromContent(content string) []string {
	matches := linkedRepoPathMentionPattern.FindAllString(strings.TrimSpace(content), -1)
	if len(matches) == 0 {
		return nil
	}
	paths := make([]string, 0, len(matches))
	for _, match := range matches {
		if path, ok := normalizeLinkedRepoPath(match); ok {
			paths = append(paths, path)
		}
	}
	return compactStringList(paths)
}

func (b *Broker) upsertLinkedRepoForChannelLocked(channel string, repoPath string, source string, createdBy string) bool {
	ch := b.findChannelLocked(channel)
	if ch == nil {
		return false
	}
	repoPath, ok := normalizeLinkedRepoPath(repoPath)
	if !ok {
		return false
	}
	now := time.Now().UTC().Format(time.RFC3339)
	source = strings.TrimSpace(source)
	createdBy = strings.TrimSpace(createdBy)
	promote := shouldPromoteLinkedRepoSource(source)
	changed := false
	found := -1
	for i := range ch.LinkedRepos {
		if !sameCleanPath(ch.LinkedRepos[i].RepoPath, repoPath) {
			continue
		}
		found = i
		break
	}
	if found >= 0 {
		ref := &ch.LinkedRepos[found]
		if ref.Source != source && source != "" {
			ref.Source = source
			changed = true
		}
		if ref.CreatedBy == "" && createdBy != "" {
			ref.CreatedBy = createdBy
			changed = true
		}
		if ref.UpdatedAt != now {
			ref.UpdatedAt = now
			changed = true
		}
		if ref.CreatedAt == "" {
			ref.CreatedAt = now
			changed = true
		}
		if promote && !ref.Primary {
			ref.Primary = true
			changed = true
		}
	} else {
		ref := linkedRepoRef{
			RepoPath:  repoPath,
			Source:    source,
			Primary:   len(ch.LinkedRepos) == 0 || promote,
			CreatedAt: now,
			UpdatedAt: now,
			CreatedBy: createdBy,
		}
		ch.LinkedRepos = append(ch.LinkedRepos, ref)
		found = len(ch.LinkedRepos) - 1
		changed = true
	}
	if promote {
		for i := range ch.LinkedRepos {
			shouldPrimary := i == found
			if ch.LinkedRepos[i].Primary != shouldPrimary {
				ch.LinkedRepos[i].Primary = shouldPrimary
				changed = true
			}
		}
	} else {
		primaryIndex := -1
		for i := range ch.LinkedRepos {
			if ch.LinkedRepos[i].Primary {
				if primaryIndex == -1 {
					primaryIndex = i
					continue
				}
				ch.LinkedRepos[i].Primary = false
				changed = true
			}
		}
		if primaryIndex == -1 && found >= 0 {
			ch.LinkedRepos[found].Primary = true
			changed = true
		}
	}
	if changed {
		ch.UpdatedAt = now
	}
	return changed
}

func (b *Broker) inferLinkedReposFromHumanMessageLocked(from string, channel string, content string) bool {
	if b == nil || !isHumanLikeActor(from) {
		return false
	}
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		return false
	}
	changed := false
	for _, repoPath := range extractLinkedRepoPathsFromContent(content) {
		if b.upsertLinkedRepoForChannelLocked(channel, repoPath, "human_message", from) {
			changed = true
		}
	}
	return changed
}

func (b *Broker) linkTaskWorkspaceToChannelLocked(channel string, task *teamTask) bool {
	if b == nil || task == nil {
		return false
	}
	workspacePath := strings.TrimSpace(task.WorkspacePath)
	if workspacePath == "" {
		return false
	}
	return b.upsertLinkedRepoForChannelLocked(channel, workspacePath, "task_workspace", task.CreatedBy)
}

// IsDMSlug checks whether a channel slug represents a direct message.
func IsDMSlug(slug string) bool {
	slug = normalizeChannelSlug(slug)
	return strings.HasPrefix(slug, "dm-") || strings.Count(slug, "__") == 1
}

// DMSlugFor returns the DM channel slug for a given agent.
func DMSlugFor(agentSlug string) string {
	return "dm-" + agentSlug
}

func canonicalDMMembers(slug string) []string {
	members := []string{"human"}
	agentSlug := DMTargetAgent(slug)
	switch agentSlug {
	case "", "human", "you":
		return members
	default:
		return append(members, agentSlug)
	}
}

// DMTargetAgent extracts the agent slug from a DM channel slug.
// Returns "" if the slug is not a DM.
func DMTargetAgent(slug string) string {
	slug = normalizeChannelSlug(slug)
	if strings.Count(slug, "__") == 1 {
		parts := strings.SplitN(slug, "__", 2)
		left := normalizeActorSlug(parts[0])
		right := normalizeActorSlug(parts[1])
		switch {
		case left == "human" || left == "you":
			return right
		case right == "human" || right == "you":
			return left
		default:
			return right
		}
	}
	if !IsDMSlug(slug) {
		return ""
	}
	target := strings.TrimPrefix(slug, "dm-")
	target = strings.TrimPrefix(target, "human-")
	target = strings.TrimPrefix(target, "you-")
	return target
}

type officeMember struct {
	Slug           string                   `json:"slug"`
	Name           string                   `json:"name"`
	Role           string                   `json:"role,omitempty"`
	Expertise      []string                 `json:"expertise,omitempty"`
	Personality    string                   `json:"personality,omitempty"`
	PermissionMode string                   `json:"permission_mode,omitempty"`
	AllowedTools   []string                 `json:"allowed_tools,omitempty"`
	CreatedBy      string                   `json:"created_by,omitempty"`
	CreatedAt      string                   `json:"created_at,omitempty"`
	BuiltIn        bool                     `json:"built_in,omitempty"`
	Provider       provider.ProviderBinding `json:"provider,omitempty"`
}

type officeActionLog struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Source     string   `json:"source,omitempty"`
	Channel    string   `json:"channel,omitempty"`
	Actor      string   `json:"actor,omitempty"`
	Summary    string   `json:"summary"`
	RelatedID  string   `json:"related_id,omitempty"`
	SignalIDs  []string `json:"signal_ids,omitempty"`
	DecisionID string   `json:"decision_id,omitempty"`
	CreatedAt  string   `json:"created_at"`
}

type agentActivitySnapshot struct {
	Slug         string `json:"slug"`
	Channel      string `json:"channel,omitempty"`
	Status       string `json:"status,omitempty"`
	Activity     string `json:"activity,omitempty"`
	Detail       string `json:"detail,omitempty"`
	LastTime     string `json:"lastTime,omitempty"`
	TotalMs      int64  `json:"totalMs,omitempty"`
	FirstEventMs int64  `json:"firstEventMs,omitempty"`
	FirstTextMs  int64  `json:"firstTextMs,omitempty"`
	FirstToolMs  int64  `json:"firstToolMs,omitempty"`
}

type officeSignalRecord struct {
	ID            string `json:"id"`
	Source        string `json:"source"`
	SourceRef     string `json:"source_ref,omitempty"`
	Kind          string `json:"kind,omitempty"`
	Title         string `json:"title,omitempty"`
	Content       string `json:"content"`
	Channel       string `json:"channel,omitempty"`
	Owner         string `json:"owner,omitempty"`
	Confidence    string `json:"confidence,omitempty"`
	Urgency       string `json:"urgency,omitempty"`
	DedupeKey     string `json:"dedupe_key,omitempty"`
	RequiresHuman bool   `json:"requires_human,omitempty"`
	Blocking      bool   `json:"blocking,omitempty"`
	CreatedAt     string `json:"created_at"`
}

type officeDecisionRecord struct {
	ID            string   `json:"id"`
	Kind          string   `json:"kind"`
	Channel       string   `json:"channel,omitempty"`
	Summary       string   `json:"summary"`
	Reason        string   `json:"reason,omitempty"`
	Owner         string   `json:"owner,omitempty"`
	SignalIDs     []string `json:"signal_ids,omitempty"`
	RequiresHuman bool     `json:"requires_human,omitempty"`
	Blocking      bool     `json:"blocking,omitempty"`
	CreatedAt     string   `json:"created_at"`
}

type watchdogAlert struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Channel    string `json:"channel,omitempty"`
	TargetType string `json:"target_type,omitempty"`
	TargetID   string `json:"target_id,omitempty"`
	Owner      string `json:"owner,omitempty"`
	Status     string `json:"status,omitempty"`
	Summary    string `json:"summary"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

type schedulerJob struct {
	Slug            string `json:"slug"`
	Kind            string `json:"kind,omitempty"`
	Label           string `json:"label"`
	TargetType      string `json:"target_type,omitempty"`
	TargetID        string `json:"target_id,omitempty"`
	Channel         string `json:"channel,omitempty"`
	Provider        string `json:"provider,omitempty"`
	ScheduleExpr    string `json:"schedule_expr,omitempty"`
	WorkflowKey     string `json:"workflow_key,omitempty"`
	SkillName       string `json:"skill_name,omitempty"`
	IntervalMinutes int    `json:"interval_minutes"`
	DueAt           string `json:"due_at,omitempty"`
	NextRun         string `json:"next_run,omitempty"`
	LastRun         string `json:"last_run,omitempty"`
	Status          string `json:"status,omitempty"`
	Payload         string `json:"payload,omitempty"`
}

type teamSkill struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Title               string   `json:"title"`
	Description         string   `json:"description,omitempty"`
	Content             string   `json:"content"`
	CreatedBy           string   `json:"created_by"`
	Channel             string   `json:"channel,omitempty"`
	Tags                []string `json:"tags,omitempty"`
	Trigger             string   `json:"trigger,omitempty"`
	WorkflowProvider    string   `json:"workflow_provider,omitempty"`
	WorkflowKey         string   `json:"workflow_key,omitempty"`
	WorkflowDefinition  string   `json:"workflow_definition,omitempty"`
	WorkflowSchedule    string   `json:"workflow_schedule,omitempty"`
	RelayID             string   `json:"relay_id,omitempty"`
	RelayPlatform       string   `json:"relay_platform,omitempty"`
	RelayEventTypes     []string `json:"relay_event_types,omitempty"`
	LastExecutionAt     string   `json:"last_execution_at,omitempty"`
	LastExecutionStatus string   `json:"last_execution_status,omitempty"`
	UsageCount          int      `json:"usage_count"`
	Status              string   `json:"status"`
	CreatedAt           string   `json:"created_at"`
	UpdatedAt           string   `json:"updated_at"`
}

type brokerState struct {
	ChannelStore         json.RawMessage              `json:"channel_store,omitempty"`
	Messages             []channelMessage             `json:"messages"`
	MessageIndexSnapshot *brokerMessageIndexSnapshot  `json:"message_index_snapshot,omitempty"`
	Members              []officeMember               `json:"members,omitempty"`
	Channels             []teamChannel                `json:"channels,omitempty"`
	SessionMode          string                       `json:"session_mode,omitempty"`
	OneOnOneAgent        string                       `json:"one_on_one_agent,omitempty"`
	FocusMode            bool                         `json:"focus_mode,omitempty"`
	Tasks                []teamTask                   `json:"tasks,omitempty"`
	Requests             []humanInterview             `json:"requests,omitempty"`
	Actions              []officeActionLog            `json:"actions,omitempty"`
	Signals              []officeSignalRecord         `json:"signals,omitempty"`
	Decisions            []officeDecisionRecord       `json:"decisions,omitempty"`
	Watchdogs            []watchdogAlert              `json:"watchdogs,omitempty"`
	Scheduler            []schedulerJob               `json:"scheduler,omitempty"`
	Skills               []teamSkill                  `json:"skills,omitempty"`
	ExecutionNodes       []executionNode              `json:"execution_nodes,omitempty"`
	SharedMemory         map[string]map[string]string `json:"shared_memory,omitempty"`
	Counter              int                          `json:"counter"`
	NotificationSince    string                       `json:"notification_since,omitempty"`
	InsightsSince        string                       `json:"insights_since,omitempty"`
	PendingInterview     *humanInterview              `json:"pending_interview,omitempty"`
	Usage                teamUsageState               `json:"usage,omitempty"`
	Policies             []officePolicy               `json:"policies,omitempty"`
	ObservabilityHistory []brokerObservabilitySample  `json:"observability_history,omitempty"`
}

type usageTotals struct {
	InputTokens         int     `json:"input_tokens"`
	OutputTokens        int     `json:"output_tokens"`
	CacheReadTokens     int     `json:"cache_read_tokens"`
	CacheCreationTokens int     `json:"cache_creation_tokens"`
	TotalTokens         int     `json:"total_tokens"`
	CostUsd             float64 `json:"cost_usd"`
	Requests            int     `json:"requests"`
}

type teamUsageState struct {
	Session usageTotals            `json:"session,omitempty"`
	Total   usageTotals            `json:"total"`
	Agents  map[string]usageTotals `json:"agents,omitempty"`
	Since   string                 `json:"since,omitempty"`
}

type ipRateLimitBucket struct {
	timestamps []time.Time
}

// Broker is a lightweight HTTP message broker for the team channel.
// All agent MCP instances connect to this shared broker.
type Broker struct {
	channelStore               *channel.Store
	messages                   []channelMessage
	messageIndexesByChannel    map[string][]int // channel -> positions in messages; guarded by mu
	messageSearchIndexByToken  map[string][]int
	messageSearchIndexByAuthor map[string][]int
	messageIndexesByThread     map[string]map[string][]int
	messageThreadRootByID      map[string]string
	messageIndexSize           int
	messageIndexFirstID        string
	messageIndexLastID         string
	members                    []officeMember
	memberIndex                map[string]int // slug → index into members; guarded by mu
	channels                   []teamChannel
	sessionMode                string
	oneOnOneAgent              string
	focusMode                  bool
	tasks                      []teamTask
	taskIndexes                brokerTaskIndexes
	requests                   []humanInterview
	actions                    []officeActionLog
	signals                    []officeSignalRecord
	decisions                  []officeDecisionRecord
	watchdogs                  []watchdogAlert
	scheduler                  []schedulerJob
	skills                     []teamSkill
	executionNodes             []executionNode
	executionNodeSeq           int64
	sharedMemory               map[string]map[string]string // namespace → key → value
	lastTaggedAt               map[string]time.Time         // when each agent was last @mentioned
	lastPaneSnapshot           map[string]string            // last captured pane content per agent (for change detection)
	seenTelegramGroups         map[int64]string             // chat_id -> title, populated by transport
	counter                    int
	notificationSince          string
	insightsSince              string
	pendingInterview           *humanInterview
	usage                      teamUsageState
	externalDelivered          map[string]struct{} // message IDs already queued for external delivery
	messageSubscribers         map[int]chan channelMessage
	actionSubscribers          map[int]chan officeActionLog
	activity                   map[string]agentActivitySnapshot
	activitySubscribers        map[int]chan agentActivitySnapshot
	officeSubscribers          map[int]chan officeChangeEvent
	nextSubscriberID           int
	agentStreams               map[string]*agentStreamBuffer
	mu                         sync.Mutex
	server                     *http.Server
	token                      string   // shared secret for authenticating requests
	addr                       string   // actual listen address (useful when port=0)
	webUIOrigins               []string // allowed CORS origins for web UI (set by ServeWebUI)
	runtimeProvider            string   // "codex" or "claude" — set by launcher
	packSlug                   string   // active agent pack slug ("founding-team", "revops", ...) — set by launcher
	blankSlateLaunch           bool     // start without a saved blueprint and synthesize the first operation
	generateMemberFn           func(prompt string) (generatedMemberTemplate, error)
	generateChannelFn          func(prompt string) (generatedChannelTemplate, error)
	policies                   []officePolicy // active office operating rules
	rateLimitBuckets           map[string]ipRateLimitBucket
	rateLimitWindow            time.Duration
	rateLimitRequests          int
	lastRateLimitPrune         time.Time
	agentLogRoot               string // override for tests; empty means agent.DefaultTaskLogRoot()
	sessionObservabilityFn     func() SessionObservabilitySnapshot
	gitHubPublicationAudit     taskGitHubAuditState
	observabilityMu            sync.Mutex
	observabilityHistory       []brokerObservabilitySample
	readiness                  brokerReadinessSnapshot
	httpMetrics                map[string]brokerHTTPMetric
	deferredCloudRestore       brokerDeferredCloudRestore
}

type startupReconcileGuardSummary struct {
	MissingChannels []string
	MessageCount    int
	TaskCount       int
	RequestCount    int
	ActionCount     int
	NodeCount       int
}

func (s startupReconcileGuardSummary) active() bool {
	return len(s.MissingChannels) > 0 && (s.MessageCount > 0 || s.TaskCount > 0 || s.RequestCount > 0 || s.ActionCount > 0 || s.NodeCount > 0)
}

func (s startupReconcileGuardSummary) dedupeKey() string {
	if !s.active() {
		return ""
	}
	return fmt.Sprintf(
		"startup-reconcile-guard:%s:m=%d:t=%d:r=%d:a=%d:n=%d",
		strings.Join(s.MissingChannels, ","),
		s.MessageCount,
		s.TaskCount,
		s.RequestCount,
		s.ActionCount,
		s.NodeCount,
	)
}

func (s startupReconcileGuardSummary) detail() string {
	if !s.active() {
		return ""
	}
	return fmt.Sprintf(
		"missing_channels=%s; records(messages=%d,tasks=%d,requests=%d,actions=%d,execution_nodes=%d)",
		strings.Join(s.MissingChannels, ", "),
		s.MessageCount,
		s.TaskCount,
		s.RequestCount,
		s.ActionCount,
		s.NodeCount,
	)
}

func taskNeedsLocalWorktree(task *teamTask) bool {
	if task == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(task.ExecutionMode), "local_worktree") {
		return false
	}
	if strings.TrimSpace(task.Owner) == "" {
		return false
	}
	switch strings.TrimSpace(task.Status) {
	case "", "open":
		return false
	case "done":
		return strings.TrimSpace(task.WorktreePath) != "" || strings.TrimSpace(task.WorktreeBranch) != ""
	default:
		return true
	}
}

func taskBlockReasonLooksLikeWorkspaceWriteIssue(reason string) bool {
	reason = strings.ToLower(strings.TrimSpace(reason))
	if reason == "" {
		return false
	}
	markers := []string{
		"read-only",
		"read only",
		"writable workspace",
		"write access",
		"filesystem sandbox",
		"workspace sandbox",
		"operation not permitted",
		"permission denied",
	}
	for _, marker := range markers {
		if strings.Contains(reason, marker) {
			return true
		}
	}
	return false
}

func rejectFalseLocalWorktreeBlock(task *teamTask, reason string) error {
	if task == nil {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(task.ExecutionMode), "local_worktree") {
		return nil
	}
	if !taskBlockReasonLooksLikeWorkspaceWriteIssue(reason) {
		return nil
	}
	worktreePath := strings.TrimSpace(task.WorktreePath)
	if worktreePath == "" {
		return nil
	}
	if err := verifyTaskWorktreeWritable(worktreePath); err == nil {
		return fmt.Errorf("assigned local worktree is writable at %s; do not request writable-workspace approval; continue implementation in that worktree", worktreePath)
	}
	return nil
}

func taskRequiresExclusiveOwnerTurn(task *teamTask) bool {
	if task == nil {
		return false
	}
	if strings.TrimSpace(task.Owner) == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(task.ExecutionMode)) {
	case "local_worktree", "external_workspace", "live_external":
		return true
	default:
		return false
	}
}

func taskStatusConsumesExclusiveOwnerTurn(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "in_progress", "review":
		return true
	default:
		return false
	}
}

func stringSliceContainsFold(values []string, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}

func parseBrokerTimestamp(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return ts.UTC()
}

func taskChannelCandidateOwnerAllowed(ch *teamChannel, owner string) bool {
	if ch == nil {
		return false
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return true
	}
	return stringSliceContainsFold(ch.Members, owner) || strings.EqualFold(strings.TrimSpace(ch.CreatedBy), owner)
}

func (b *Broker) appendTaskDetailsWithRecovery(task *teamTask, note string) {
	if task == nil {
		return
	}
	note = strings.TrimSpace(note)
	if note == "" {
		return
	}
	existing := strings.TrimSpace(task.Details)
	switch {
	case existing == "":
		task.Details = note
	case !strings.Contains(existing, note):
		task.Details = existing + "\n\n" + note
	}
}

func (b *Broker) recoverTaskWorkspaceFromChannelLocked(task *teamTask, currentWorkspace string) string {
	if b == nil || task == nil {
		return ""
	}
	ch := b.findChannelLocked(normalizeChannelSlug(task.Channel))
	if ch == nil || len(ch.LinkedRepos) == 0 {
		return ""
	}

	ordered := make([]string, 0, len(ch.LinkedRepos))
	if primary := strings.TrimSpace(ch.primaryLinkedRepoPath()); primary != "" {
		ordered = append(ordered, primary)
	}
	for _, repo := range ch.LinkedRepos {
		path := strings.TrimSpace(repo.RepoPath)
		if path == "" {
			continue
		}
		duplicate := false
		for _, existing := range ordered {
			if sameCleanPath(existing, path) {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		ordered = append(ordered, path)
	}
	currentWorkspace = filepath.Clean(strings.TrimSpace(currentWorkspace))
	for _, workspacePath := range ordered {
		workspacePath = filepath.Clean(strings.TrimSpace(workspacePath))
		if workspacePath == "" || sameCleanPath(workspacePath, currentWorkspace) {
			continue
		}
		if taskWorkspacePathLooksEligible(workspacePath) && taskWorktreeSourceLooksUsable(workspacePath) {
			return workspacePath
		}
	}
	return ""
}

func (b *Broker) syncTaskWorktreeLocked(task *teamTask) error {
	if task == nil {
		return nil
	}
	// Automatically assign local_worktree mode when a coding agent claims a task.
	if task.ExecutionMode == "" && codingAgentSlugs[strings.TrimSpace(task.Owner)] {
		switch strings.TrimSpace(task.Status) {
		case "", "open", "done":
			// not yet in-progress; leave mode unset
		default:
			task.ExecutionMode = "local_worktree"
		}
	}
	if inferred := inferSiblingWorkspacePathForTask(task); inferred != "" {
		task.ExecutionMode = "external_workspace"
		task.WorkspacePath = inferred
	}
	if strings.EqualFold(strings.TrimSpace(task.ExecutionMode), "local_worktree") {
		if linkedWorkspace := b.recoverTaskWorkspaceFromChannelLocked(task, strings.TrimSpace(task.WorkspacePath)); linkedWorkspace != "" {
			task.ExecutionMode = "external_workspace"
			task.WorkspacePath = linkedWorkspace
			task.WorktreePath = ""
			task.WorktreeBranch = ""
			b.appendTaskDetailsWithRecovery(task, fmt.Sprintf("Recovered workspace path from linked channel repo: %s", linkedWorkspace))
		}
	}
	switch strings.ToLower(strings.TrimSpace(task.Status)) {
	case "done", "canceled", "cancelled", "failed":
		task.WorktreePath = ""
		task.WorktreeBranch = ""
		return nil
	}
	if taskNeedsLocalWorktree(task) {
		if strings.TrimSpace(task.WorktreePath) != "" && strings.TrimSpace(task.WorktreeBranch) != "" {
			if taskWorktreeSourceLooksUsable(task.WorktreePath) {
				return nil
			}
			if err := cleanupTaskWorktree(task.WorktreePath, task.WorktreeBranch); err != nil {
				return err
			}
			task.WorktreePath = ""
			task.WorktreeBranch = ""
		}
		if path, branch := b.reusableDependencyWorktreeLocked(task); path != "" && branch != "" {
			task.WorktreePath = path
			task.WorktreeBranch = branch
			return nil
		}
		path, branch, err := prepareTaskWorktree(task.ID)
		if err != nil {
			return err
		}
		task.WorktreePath = path
		task.WorktreeBranch = branch
		return nil
	}

	if taskUsesExternalWorkspace(task) {
		switch strings.ToLower(strings.TrimSpace(task.Status)) {
		case "canceled", "cancelled", "failed":
			task.WorkspacePath = ""
			task.WorktreePath = ""
			task.WorktreeBranch = ""
			return nil
		}
		if taskIsTerminal(task) {
			return nil
		}
		workspacePath := strings.TrimSpace(task.WorkspacePath)
		if workspacePath == "" {
			if fallbackPath := b.recoverTaskWorkspaceFromChannelLocked(task, workspacePath); fallbackPath != "" {
				task.WorkspacePath = fallbackPath
				b.appendTaskDetailsWithRecovery(task, fmt.Sprintf("Recovered workspace path for external mode from linked repo: %s", fallbackPath))
				workspacePath = fallbackPath
			} else {
				return fmt.Errorf("external workspace path required")
			}
		}
		if !taskWorktreeSourceLooksUsable(workspacePath) {
			if fallbackPath := b.recoverTaskWorkspaceFromChannelLocked(task, workspacePath); fallbackPath != "" {
				task.WorkspacePath = fallbackPath
				b.appendTaskDetailsWithRecovery(task, fmt.Sprintf("Recovered workspace path for external mode from linked repo: %s (previous path not usable: %s)", fallbackPath, workspacePath))
				workspacePath = fallbackPath
			} else {
				return fmt.Errorf("external workspace path %q is not a usable git workspace", workspacePath)
			}
		}
		if strings.TrimSpace(task.WorktreePath) == "" && strings.TrimSpace(task.WorktreeBranch) == "" {
			return nil
		}
		if err := cleanupTaskWorktree(task.WorktreePath, task.WorktreeBranch); err != nil {
			return err
		}
		task.WorktreePath = ""
		task.WorktreeBranch = ""
		return nil
	}

	if strings.TrimSpace(task.WorktreePath) == "" && strings.TrimSpace(task.WorktreeBranch) == "" {
		task.WorkspacePath = ""
		return nil
	}
	if err := cleanupTaskWorktree(task.WorktreePath, task.WorktreeBranch); err != nil {
		return err
	}
	task.WorktreePath = ""
	task.WorktreeBranch = ""
	task.WorkspacePath = ""
	return nil
}

func (b *Broker) reusableDependencyWorktreeLocked(task *teamTask) (string, string) {
	if b == nil || task == nil || len(task.DependsOn) == 0 {
		return "", ""
	}
	owner := strings.TrimSpace(task.Owner)
	var fallbackPath string
	var fallbackBranch string
	for _, depID := range task.DependsOn {
		depID = strings.TrimSpace(depID)
		if depID == "" {
			continue
		}
		for i := range b.tasks {
			dep := &b.tasks[i]
			if strings.TrimSpace(dep.ID) != depID {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(dep.ExecutionMode), "local_worktree") {
				continue
			}
			path := strings.TrimSpace(dep.WorktreePath)
			branch := strings.TrimSpace(dep.WorktreeBranch)
			if path == "" || branch == "" {
				continue
			}
			status := strings.ToLower(strings.TrimSpace(dep.Status))
			review := strings.ToLower(strings.TrimSpace(dep.ReviewState))
			if status != "review" && status != "done" && review != "ready_for_review" && review != "approved" {
				continue
			}
			if owner != "" && strings.TrimSpace(dep.Owner) == owner {
				return path, branch
			}
			if fallbackPath == "" && fallbackBranch == "" {
				fallbackPath = path
				fallbackBranch = branch
			}
		}
	}
	return fallbackPath, fallbackBranch
}

func (b *Broker) activeExclusiveOwnerTaskLocked(owner, excludeTaskID string) *teamTask {
	owner = strings.TrimSpace(owner)
	excludeTaskID = strings.TrimSpace(excludeTaskID)
	if b == nil || owner == "" {
		return nil
	}
	for i := range b.tasks {
		task := &b.tasks[i]
		if excludeTaskID != "" && strings.TrimSpace(task.ID) == excludeTaskID {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(task.Owner), owner) {
			continue
		}
		if !taskRequiresExclusiveOwnerTurn(task) {
			continue
		}
		if !taskStatusConsumesExclusiveOwnerTurn(task.Status) {
			continue
		}
		return task
	}
	return nil
}

func (b *Broker) queueTaskBehindActiveOwnerLaneLocked(task *teamTask) {
	if b == nil || task == nil {
		return
	}
	if !taskRequiresExclusiveOwnerTurn(task) {
		return
	}
	if !taskStatusConsumesExclusiveOwnerTurn(task.Status) {
		return
	}
	active := b.activeExclusiveOwnerTaskLocked(task.Owner, task.ID)
	if active == nil {
		return
	}
	if !stringSliceContainsFold(task.DependsOn, active.ID) {
		task.DependsOn = append(task.DependsOn, active.ID)
	}
	task.Blocked = true
	task.Status = "open"
	queueNote := fmt.Sprintf("Queued behind %s so @%s only carries one active %s lane at a time.", active.ID, strings.TrimSpace(task.Owner), strings.TrimSpace(task.ExecutionMode))
	switch existing := strings.TrimSpace(task.Details); {
	case existing == "":
		task.Details = queueNote
	case !strings.Contains(existing, queueNote):
		task.Details = existing + "\n\n" + queueNote
	}
}

func (b *Broker) preferredTaskChannelLocked(requestedChannel, createdBy, owner, title, details string) string {
	channel := normalizeChannelSlug(requestedChannel)
	if channel == "" {
		channel = "general"
	}
	if channel != "general" || b == nil {
		return channel
	}
	createdBy = strings.TrimSpace(createdBy)
	if createdBy == "" {
		return channel
	}
	probe := teamTask{
		Channel: channel,
		Owner:   strings.TrimSpace(owner),
		Title:   strings.TrimSpace(title),
		Details: strings.TrimSpace(details),
	}
	if !taskLooksLikeLiveBusinessObjective(&probe) {
		return channel
	}
	now := time.Now().UTC()
	var best *teamChannel
	var bestCreated time.Time
	for i := range b.channels {
		ch := &b.channels[i]
		slug := normalizeChannelSlug(ch.Slug)
		if slug == "" || slug == "general" || ch.isDM() {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(ch.CreatedBy), createdBy) {
			continue
		}
		if !taskChannelCandidateOwnerAllowed(ch, owner) {
			continue
		}
		createdAt := parseBrokerTimestamp(ch.CreatedAt)
		if !createdAt.IsZero() && now.Sub(createdAt) > 20*time.Minute {
			continue
		}
		if best == nil || (!createdAt.IsZero() && createdAt.After(bestCreated)) {
			best = ch
			bestCreated = createdAt
		}
	}
	if best == nil {
		return channel
	}
	return normalizeChannelSlug(best.Slug)
}

// generateToken returns a cryptographically random hex token.
func generateToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback: this should never happen on modern systems
		return fmt.Sprintf("wuphf-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// AgentStream returns (or lazily creates) the stream buffer for a given agent lane.
// It is safe to call concurrently.
func (b *Broker) AgentStream(slug string, channel ...string) *agentStreamBuffer {
	key := agentLaneKey(firstNonEmpty(channel...), slug)
	if key == "" {
		key = normalizeAgentLaneSlug(slug)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.agentStreams == nil {
		b.agentStreams = make(map[string]*agentStreamBuffer)
	}
	s, ok := b.agentStreams[key]
	if !ok {
		s = &agentStreamBuffer{subs: make(map[int]chan string)}
		b.agentStreams[key] = s
	}
	return s
}

// NewBroker creates a new channel broker with a random auth token.
func NewBroker() *Broker {
	b := &Broker{
		channelStore:               channel.NewStore(),
		token:                      generateToken(),
		messageIndexesByChannel:    make(map[string][]int),
		messageSearchIndexByToken:  make(map[string][]int),
		messageSearchIndexByAuthor: make(map[string][]int),
		messageIndexesByThread:     make(map[string]map[string][]int),
		messageThreadRootByID:      make(map[string]string),
		messageSubscribers:         make(map[int]chan channelMessage),
		actionSubscribers:          make(map[int]chan officeActionLog),
		activity:                   make(map[string]agentActivitySnapshot),
		activitySubscribers:        make(map[int]chan agentActivitySnapshot),
		officeSubscribers:          make(map[int]chan officeChangeEvent),
		agentStreams:               make(map[string]*agentStreamBuffer),
		rateLimitBuckets:           make(map[string]ipRateLimitBucket),
		rateLimitWindow:            defaultRateLimitWindow,
		rateLimitRequests:          defaultRateLimitRequestsPerWindow,
		httpMetrics:                make(map[string]brokerHTTPMetric),
		readiness: brokerReadinessSnapshot{
			Live:    true,
			Ready:   true,
			State:   "ready",
			Stage:   "broker",
			Summary: "Broker local pronto.",
		},
	}
	_ = b.loadState()
	b.mu.Lock()
	b.recoverChannelsFromStateLocked()
	b.recoverMessagesFromChannelMemoryLocked()
	b.recoverTasksFromChannelMemoryLocked()
	startupGuardTriggered := false
	if manifest, ok := runtimeManifestDefaults(); ok {
		startupGuardTriggered = b.reconcileStateToManifestLocked(manifest)
	}
	b.ensureDefaultOfficeMembersLocked()
	b.ensureDefaultChannelsLocked()
	b.normalizeLoadedStateLocked()
	b.ensureMessageIndexesLocked()
	b.rebuildTaskIndexesLocked()
	if startupGuardTriggered {
		if err := b.saveLocked(); err != nil {
			log.Printf("broker: persist startup reconcile guard: %v", err)
		}
	}
	b.mu.Unlock()
	return b
}

func normalizeTaskIndexChannel(channel string) string {
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		return "general"
	}
	return channel
}

func normalizeTaskIndexValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (b *Broker) rebuildTaskIndexesLocked() {
	indexes := brokerTaskIndexes{
		byID:        make(map[string]int, len(b.tasks)),
		byChannel:   make(map[string][]int),
		byOwner:     make(map[string][]int),
		byStatus:    make(map[string][]int),
		byTaskType:  make(map[string][]int),
		sizeSeen:    len(b.tasks),
		counterSeen: b.counter,
	}
	for i := range b.tasks {
		task := b.tasks[i]
		if id := strings.TrimSpace(task.ID); id != "" {
			indexes.byID[id] = i
		}
		channel := normalizeTaskIndexChannel(task.Channel)
		indexes.byChannel[channel] = append(indexes.byChannel[channel], i)
		if owner := normalizeTaskIndexValue(task.Owner); owner != "" {
			indexes.byOwner[owner] = append(indexes.byOwner[owner], i)
		}
		status := normalizeTaskIndexValue(task.Status)
		if status == "" {
			status = "open"
		}
		indexes.byStatus[status] = append(indexes.byStatus[status], i)
		taskType := normalizeTaskIndexValue(task.TaskType)
		if taskType != "" {
			indexes.byTaskType[taskType] = append(indexes.byTaskType[taskType], i)
		}
	}
	b.taskIndexes = indexes
}

func (b *Broker) ensureTaskIndexesLocked() {
	if len(b.tasks) == 0 {
		b.taskIndexes = brokerTaskIndexes{
			byID:        make(map[string]int),
			byChannel:   make(map[string][]int),
			byOwner:     make(map[string][]int),
			byStatus:    make(map[string][]int),
			byTaskType:  make(map[string][]int),
			sizeSeen:    0,
			counterSeen: b.counter,
		}
		return
	}
	if b.taskIndexes.byID == nil || b.taskIndexes.sizeSeen != len(b.tasks) || b.taskIndexes.counterSeen != b.counter {
		b.rebuildTaskIndexesLocked()
	}
}

func (b *Broker) copyIndexedTasksLocked(indexes []int) []teamTask {
	if len(indexes) == 0 {
		return nil
	}
	out := make([]teamTask, 0, len(indexes))
	for _, idx := range indexes {
		if idx < 0 || idx >= len(b.tasks) {
			continue
		}
		out = append(out, b.tasks[idx])
	}
	return out
}

func (b *Broker) findTaskIndexByIDLocked(taskID string) int {
	b.ensureTaskIndexesLocked()
	if idx, ok := b.taskIndexes.byID[strings.TrimSpace(taskID)]; ok && idx >= 0 && idx < len(b.tasks) {
		return idx
	}
	return -1
}

func (b *Broker) tasksByTypeLocked(taskType string) []teamTask {
	b.ensureTaskIndexesLocked()
	return b.copyIndexedTasksLocked(b.taskIndexes.byTaskType[normalizeTaskIndexValue(taskType)])
}

func (b *Broker) activeTaskByOwnerLocked(owner, channel string) (teamTask, bool) {
	b.ensureTaskIndexesLocked()
	owner = normalizeTaskIndexValue(owner)
	if owner == "" {
		return teamTask{}, false
	}
	targetChannel := normalizeChannelSlug(channel)
	var fallback teamTask
	for _, idx := range b.taskIndexes.byOwner[owner] {
		if idx < 0 || idx >= len(b.tasks) {
			continue
		}
		task := b.tasks[idx]
		if !strings.EqualFold(strings.TrimSpace(task.Status), "in_progress") {
			continue
		}
		if targetChannel != "" && normalizeChannelSlug(task.Channel) == targetChannel {
			return task, true
		}
		if strings.TrimSpace(fallback.ID) == "" {
			fallback = task
		}
	}
	if strings.TrimSpace(fallback.ID) != "" {
		return fallback, true
	}
	return teamTask{}, false
}

func (b *Broker) appendMessageLocked(msg channelMessage) {
	if b.messageIndexesByChannel == nil {
		b.messageIndexesByChannel = make(map[string][]int)
	}
	if b.messageSearchIndexByToken == nil {
		b.messageSearchIndexByToken = make(map[string][]int)
	}
	if b.messageSearchIndexByAuthor == nil {
		b.messageSearchIndexByAuthor = make(map[string][]int)
	}
	if b.messageIndexesByThread == nil {
		b.messageIndexesByThread = make(map[string]map[string][]int)
	}
	if b.messageThreadRootByID == nil {
		b.messageThreadRootByID = make(map[string]string)
	}
	channel := normalizeChannelSlug(msg.Channel)
	index := len(b.messages)
	b.messages = append(b.messages, msg)
	b.messageIndexesByChannel[channel] = append(b.messageIndexesByChannel[channel], index)
	b.indexMessageSearchLocked(index, msg)
	b.messageIndexSize = len(b.messages)
	if b.messageIndexSize == 1 {
		b.messageIndexFirstID = strings.TrimSpace(msg.ID)
	}
	b.messageIndexLastID = strings.TrimSpace(msg.ID)
	b.publishMessageLocked(msg)
}

func (b *Broker) replaceMessagesLocked(messages []channelMessage) {
	b.messages = messages
	b.rebuildMessageIndexesLocked()
}

func (b *Broker) rebuildMessageIndexesLocked() {
	if len(b.messages) == 0 {
		b.messageIndexesByChannel = make(map[string][]int)
		b.messageSearchIndexByToken = make(map[string][]int)
		b.messageSearchIndexByAuthor = make(map[string][]int)
		b.messageIndexesByThread = make(map[string]map[string][]int)
		b.messageThreadRootByID = make(map[string]string)
		b.messageIndexSize = 0
		b.messageIndexFirstID = ""
		b.messageIndexLastID = ""
		return
	}
	indexes := make(map[string][]int)
	tokenIndexes := make(map[string][]int)
	authorIndexes := make(map[string][]int)
	threadIndexes := make(map[string]map[string][]int)
	threadRoots := make(map[string]string)
	byID := make(map[string]channelMessage, len(b.messages))
	for i, msg := range b.messages {
		if msgID := strings.TrimSpace(msg.ID); msgID != "" {
			byID[msgID] = msg
		}
		channel := normalizeChannelSlug(msg.Channel)
		indexes[channel] = append(indexes[channel], i)
		if author := normalizeActorSlug(msg.From); author != "" {
			authorIndexes[author] = append(authorIndexes[author], i)
		}
		for _, token := range messageSearchTokensForIndex(msg.Title + "\n" + msg.Content) {
			tokenIndexes[token] = append(tokenIndexes[token], i)
		}
	}
	resolveRoot := func(startID string) string {
		current := strings.TrimSpace(startID)
		if current == "" {
			return ""
		}
		if cached := strings.TrimSpace(threadRoots[current]); cached != "" {
			return cached
		}
		visited := make([]string, 0, 8)
		for depth := 0; depth < 32 && current != ""; depth++ {
			if cached := strings.TrimSpace(threadRoots[current]); cached != "" {
				current = cached
				break
			}
			visited = append(visited, current)
			msg, ok := byID[current]
			if !ok {
				break
			}
			parent := strings.TrimSpace(msg.ReplyTo)
			if parent == "" {
				break
			}
			current = parent
		}
		root := strings.TrimSpace(current)
		for _, id := range visited {
			if id != "" {
				threadRoots[id] = root
			}
		}
		return root
	}
	for i, msg := range b.messages {
		channel := normalizeChannelSlug(msg.Channel)
		msgID := strings.TrimSpace(msg.ID)
		if msgID == "" {
			continue
		}
		threadRootID := msgID
		if replyTo := strings.TrimSpace(msg.ReplyTo); replyTo != "" {
			threadRootID = firstNonEmpty(resolveRoot(replyTo), replyTo)
		} else {
			threadRootID = resolveRoot(msgID)
		}
		if threadRootID == "" {
			threadRootID = msgID
		}
		threadRoots[msgID] = threadRootID
		if threadIndexes[channel] == nil {
			threadIndexes[channel] = make(map[string][]int)
		}
		threadIndexes[channel][threadRootID] = append(threadIndexes[channel][threadRootID], i)
	}
	b.messageIndexesByChannel = indexes
	b.messageSearchIndexByToken = tokenIndexes
	b.messageSearchIndexByAuthor = authorIndexes
	b.messageIndexesByThread = threadIndexes
	b.messageThreadRootByID = threadRoots
	b.messageIndexSize = len(b.messages)
	b.messageIndexFirstID = strings.TrimSpace(b.messages[0].ID)
	b.messageIndexLastID = strings.TrimSpace(b.messages[len(b.messages)-1].ID)
}

func (b *Broker) ensureMessageIndexesLocked() {
	if len(b.messages) == 0 {
		if b.messageIndexSize != 0 || len(b.messageIndexesByChannel) != 0 || len(b.messageSearchIndexByToken) != 0 || len(b.messageSearchIndexByAuthor) != 0 || len(b.messageIndexesByThread) != 0 || len(b.messageThreadRootByID) != 0 {
			b.rebuildMessageIndexesLocked()
		}
		return
	}
	if b.messageIndexSize != len(b.messages) {
		b.rebuildMessageIndexesLocked()
		return
	}
	if b.messageIndexesByThread == nil || b.messageSearchIndexByToken == nil || b.messageSearchIndexByAuthor == nil || b.messageThreadRootByID == nil {
		b.rebuildMessageIndexesLocked()
		return
	}
	if b.messageIndexFirstID != strings.TrimSpace(b.messages[0].ID) || b.messageIndexLastID != strings.TrimSpace(b.messages[len(b.messages)-1].ID) {
		b.rebuildMessageIndexesLocked()
	}
}

func messageSearchTokensForIndex(text string) []string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return nil
	}
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(parts) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(parts))
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len(part) < 2 {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		tokens = append(tokens, part)
	}
	return tokens
}

func indexSetFromIndexes(indexes []int) map[int]struct{} {
	set := make(map[int]struct{}, len(indexes))
	for _, idx := range indexes {
		set[idx] = struct{}{}
	}
	return set
}

func intersectIndexSets(left, right map[int]struct{}) map[int]struct{} {
	if len(left) == 0 || len(right) == 0 {
		return map[int]struct{}{}
	}
	if len(left) > len(right) {
		left, right = right, left
	}
	out := make(map[int]struct{}, len(left))
	for idx := range left {
		if _, ok := right[idx]; ok {
			out[idx] = struct{}{}
		}
	}
	return out
}

func (b *Broker) indexMessageSearchLocked(index int, msg channelMessage) {
	if b == nil {
		return
	}
	channel := normalizeChannelSlug(msg.Channel)
	if author := normalizeActorSlug(msg.From); author != "" {
		if b.messageSearchIndexByAuthor == nil {
			b.messageSearchIndexByAuthor = make(map[string][]int)
		}
		b.messageSearchIndexByAuthor[author] = append(b.messageSearchIndexByAuthor[author], index)
	}
	if msgID := strings.TrimSpace(msg.ID); msgID != "" {
		threadRootID := msgID
		if replyTo := strings.TrimSpace(msg.ReplyTo); replyTo != "" {
			threadRootID = firstNonEmpty(
				strings.TrimSpace(b.messageThreadRootByID[replyTo]),
				b.threadRootFromMessageIDLocked(replyTo),
				replyTo,
			)
		}
		if threadRootID != "" {
			if b.messageIndexesByThread == nil {
				b.messageIndexesByThread = make(map[string]map[string][]int)
			}
			if b.messageIndexesByThread[channel] == nil {
				b.messageIndexesByThread[channel] = make(map[string][]int)
			}
			b.messageIndexesByThread[channel][threadRootID] = append(b.messageIndexesByThread[channel][threadRootID], index)
			b.messageThreadRootByID[msgID] = threadRootID
		}
	}
	for _, token := range messageSearchTokensForIndex(msg.Title + "\n" + msg.Content) {
		if b.messageSearchIndexByToken == nil {
			b.messageSearchIndexByToken = make(map[string][]int)
		}
		b.messageSearchIndexByToken[token] = append(b.messageSearchIndexByToken[token], index)
	}
}

func (b *Broker) messageSearchTokenIndexesLocked(tokens []string) map[int]struct{} {
	var candidate map[int]struct{}
	for _, token := range tokens {
		postings := b.messageSearchIndexByToken[token]
		if len(postings) == 0 {
			continue
		}
		set := indexSetFromIndexes(postings)
		if candidate == nil {
			candidate = set
			continue
		}
		candidate = intersectIndexSets(candidate, set)
		if len(candidate) == 0 {
			return candidate
		}
	}
	return candidate
}

func (b *Broker) messageSearchThreadIndexesLocked(channelFilter, threadID string, threadOnly bool) map[int]struct{} {
	threadID = strings.TrimSpace(threadID)
	if threadID != "" {
		threadRootID := firstNonEmpty(strings.TrimSpace(b.messageThreadRootByID[threadID]), b.threadRootFromMessageIDLocked(threadID), threadID)
		if threadRootID == "" {
			return map[int]struct{}{}
		}
		if channelFilter != "" {
			if roots := b.messageIndexesByThread[channelFilter]; roots != nil {
				return indexSetFromIndexes(roots[threadRootID])
			}
			return map[int]struct{}{}
		}
		set := make(map[int]struct{})
		for _, roots := range b.messageIndexesByThread {
			if indexes := roots[threadRootID]; len(indexes) > 0 {
				for _, idx := range indexes {
					set[idx] = struct{}{}
				}
			}
		}
		return set
	}
	if !threadOnly {
		return nil
	}
	set := make(map[int]struct{})
	for channel, roots := range b.messageIndexesByThread {
		if channelFilter != "" && channel != channelFilter {
			continue
		}
		for _, indexes := range roots {
			if len(indexes) <= 1 {
				continue
			}
			for _, idx := range indexes {
				set[idx] = struct{}{}
			}
		}
	}
	return set
}

func (b *Broker) messageSearchCandidateSetLocked(queryTokens []string, channelFilter, authorFilter, threadID string, threadOnly bool) (map[int]struct{}, bool) {
	var candidate map[int]struct{}
	shortlisted := false
	add := func(set map[int]struct{}) {
		if set == nil {
			return
		}
		shortlisted = true
		if candidate == nil {
			candidate = set
			return
		}
		candidate = intersectIndexSets(candidate, set)
	}
	if channelFilter != "" {
		add(indexSetFromIndexes(b.messageIndexesByChannel[channelFilter]))
	}
	if authorFilter != "" {
		add(indexSetFromIndexes(b.messageSearchIndexByAuthor[authorFilter]))
	}
	if threadSet := b.messageSearchThreadIndexesLocked(channelFilter, threadID, threadOnly); threadSet != nil {
		add(threadSet)
	}
	if tokenSet := b.messageSearchTokenIndexesLocked(queryTokens); tokenSet != nil {
		add(tokenSet)
	}
	if !shortlisted {
		return nil, false
	}
	return candidate, true
}

func (b *Broker) findRecentRepeatedAgentStatusLocked(from, channel, replyTo, content string) *channelMessage {
	if b == nil {
		return nil
	}
	from = strings.TrimSpace(from)
	channel = normalizeChannelSlug(channel)
	replyTo = strings.TrimSpace(replyTo)
	if from == "" || from == "you" || from == "human" || from == "nex" {
		return nil
	}
	if !messageLooksLikeBlockedNoDeltaUpdate(content) {
		return nil
	}
	signature := normalizeBlockedNoDeltaSignature(content)
	if signature == "" {
		return nil
	}
	cutoff := time.Now().UTC().Add(-5 * time.Minute)
	for i := len(b.messages) - 1; i >= 0; i-- {
		msg := &b.messages[i]
		if strings.TrimSpace(msg.From) != from || normalizeChannelSlug(msg.Channel) != channel {
			continue
		}
		if strings.TrimSpace(msg.ReplyTo) != replyTo {
			continue
		}
		when, err := time.Parse(time.RFC3339, strings.TrimSpace(msg.Timestamp))
		if err == nil && when.Before(cutoff) {
			break
		}
		if !messageLooksLikeBlockedNoDeltaUpdate(msg.Content) {
			continue
		}
		if normalizeBlockedNoDeltaSignature(msg.Content) == signature {
			return msg
		}
	}
	return nil
}

func (b *Broker) findRecentRepeatedCoordinationGuidanceLocked(from, channel, content string) *channelMessage {
	if b == nil {
		return nil
	}
	from = strings.TrimSpace(from)
	channel = normalizeChannelSlug(channel)
	if from == "" || from == "you" || from == "human" || from == "nex" || from == "system" {
		return nil
	}
	if !messageLooksLikeCoordinationGuidance(content) {
		return nil
	}
	signature := normalizeCoordinationGuidanceSignature(content)
	if signature == "" {
		return nil
	}
	cutoff := time.Now().UTC().Add(-5 * time.Minute)
	for i := len(b.messages) - 1; i >= 0; i-- {
		msg := &b.messages[i]
		if strings.TrimSpace(msg.From) != from || normalizeChannelSlug(msg.Channel) != channel {
			continue
		}
		when, err := time.Parse(time.RFC3339, strings.TrimSpace(msg.Timestamp))
		if err == nil && when.Before(cutoff) {
			break
		}
		if !messageLooksLikeCoordinationGuidance(msg.Content) {
			continue
		}
		if normalizeCoordinationGuidanceSignature(msg.Content) == signature || coordinationGuidanceEquivalent(msg.Content, content) {
			return msg
		}
	}
	return nil
}

func (b *Broker) findRedundantOperationalRollCallLocked(from, channel, replyTo, content string, tagged []string) *channelMessage {
	if b == nil {
		return nil
	}
	from = strings.TrimSpace(from)
	channel = normalizeChannelSlug(channel)
	replyTo = strings.TrimSpace(replyTo)
	if from != "ceo" || channel == "" || replyTo == "" {
		return nil
	}
	if !messageLooksLikeOperationalRollCall(content) {
		return nil
	}
	rootID := firstNonEmpty(b.threadRootFromMessageIDLocked(replyTo), replyTo)
	if rootID == "" {
		return nil
	}
	pending := make(map[string]struct{})
	for _, node := range b.executionNodes {
		if normalizeChannelSlug(node.Channel) != channel {
			continue
		}
		if strings.TrimSpace(node.RootMessageID) != rootID || !executionNodeIsOpen(node) {
			continue
		}
		owner := strings.TrimSpace(node.OwnerAgent)
		if owner == "" || isHumanLikeActor(owner) || isSystemActor(owner) {
			continue
		}
		pending[owner] = struct{}{}
	}
	if len(pending) == 0 {
		return nil
	}
	currentCovered := make(map[string]struct{})
	for _, slug := range uniqueSlugs(tagged) {
		slug = strings.TrimSpace(slug)
		if _, ok := pending[slug]; ok {
			currentCovered[slug] = struct{}{}
		}
	}
	if len(currentCovered) == 0 {
		return nil
	}

	cutoff := time.Now().UTC().Add(-10 * time.Minute)
	for i := len(b.messages) - 1; i >= 0; i-- {
		msg := &b.messages[i]
		if strings.TrimSpace(msg.From) != from || normalizeChannelSlug(msg.Channel) != channel {
			continue
		}
		when, err := time.Parse(time.RFC3339, strings.TrimSpace(msg.Timestamp))
		if err == nil && when.Before(cutoff) {
			break
		}
		if !messageLooksLikeOperationalRollCall(msg.Content) {
			continue
		}
		msgRoot := firstNonEmpty(b.threadRootFromMessageIDLocked(strings.TrimSpace(msg.ReplyTo)), strings.TrimSpace(msg.ReplyTo), strings.TrimSpace(msg.ID))
		if msgRoot != rootID {
			continue
		}
		priorCoveredAll := true
		for slug := range currentCovered {
			if !containsString(msg.Tagged, slug) {
				priorCoveredAll = false
				break
			}
		}
		if priorCoveredAll {
			return msg
		}
	}
	return nil
}

func (b *Broker) validateTaskStateClaimLocked(channel, content string) error {
	claim := parseTaskStateClaim(content)
	if len(claim.TaskIDs) == 0 {
		return nil
	}
	channel = normalizeChannelSlug(channel)
	for _, taskID := range claim.TaskIDs {
		var task *teamTask
		for i := range b.tasks {
			if strings.EqualFold(strings.TrimSpace(b.tasks[i].ID), taskID) {
				if channel == "" || normalizeChannelSlug(b.tasks[i].Channel) == channel {
					task = &b.tasks[i]
					break
				}
				if task == nil {
					task = &b.tasks[i]
				}
			}
		}
		if task == nil {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(task.Status))
		if claim.ClaimsUnblocked && (task.Blocked || status == "blocked") {
			return fmt.Errorf("task state claim contradicts live state for %s: mutate the task before announcing it is unblocked", task.ID)
		}
		if claim.ClaimsInProgress && (task.Blocked || (status != "in_progress" && status != "review")) {
			return fmt.Errorf("task state claim contradicts live state for %s: mutate the task before announcing in_progress", task.ID)
		}
		if claim.ClaimsReviewReady && !(status == "review" || strings.EqualFold(strings.TrimSpace(task.ReviewState), "ready_for_review") || strings.EqualFold(strings.TrimSpace(task.ReviewState), "approved")) {
			return fmt.Errorf("task state claim contradicts live state for %s: mutate the task before announcing review-ready", task.ID)
		}
		if claim.ClaimsDone && status != "done" && status != "completed" {
			return fmt.Errorf("task state claim contradicts live state for %s: mutate the task before announcing done", task.ID)
		}
	}
	return nil
}

func normalizeExactAgentMessageSignature(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	return strings.Join(strings.Fields(content), " ")
}

func (b *Broker) findRecentExactAgentDuplicateLocked(from, channel, replyTo, content string) *channelMessage {
	if b == nil {
		return nil
	}
	from = strings.TrimSpace(from)
	channel = normalizeChannelSlug(channel)
	replyTo = strings.TrimSpace(replyTo)
	if from == "" || from == "you" || from == "human" || from == "nex" || from == "system" {
		return nil
	}
	signature := normalizeExactAgentMessageSignature(content)
	if signature == "" || !messageIsSubstantiveOfficeContent(signature) {
		return nil
	}
	cutoff := time.Now().UTC().Add(-5 * time.Minute)
	for i := len(b.messages) - 1; i >= 0; i-- {
		msg := &b.messages[i]
		if strings.TrimSpace(msg.From) != from || normalizeChannelSlug(msg.Channel) != channel {
			continue
		}
		if strings.TrimSpace(msg.ReplyTo) != replyTo {
			continue
		}
		when, err := time.Parse(time.RFC3339, strings.TrimSpace(msg.Timestamp))
		if err == nil && when.Before(cutoff) {
			break
		}
		if normalizeExactAgentMessageSignature(msg.Content) == signature {
			return msg
		}
	}
	return nil
}

func (b *Broker) findRecentAgentReplyInThreadLocked(from, channel, replyTo string) *channelMessage {
	if b == nil {
		return nil
	}
	from = strings.TrimSpace(from)
	channel = normalizeChannelSlug(channel)
	replyTo = strings.TrimSpace(replyTo)
	if from == "" || channel == "" || replyTo == "" || isHumanLikeActor(from) || isSystemActor(from) {
		return nil
	}
	cutoff := time.Now().UTC().Add(-10 * time.Minute)
	for i := len(b.messages) - 1; i >= 0; i-- {
		msg := &b.messages[i]
		if strings.TrimSpace(msg.From) != from || normalizeChannelSlug(msg.Channel) != channel {
			continue
		}
		if strings.TrimSpace(msg.ReplyTo) != replyTo {
			continue
		}
		when, err := time.Parse(time.RFC3339, strings.TrimSpace(msg.Timestamp))
		if err == nil && when.Before(cutoff) {
			break
		}
		if !messageIsSubstantiveOfficeContent(msg.Content) {
			continue
		}
		return msg
	}
	return nil
}

func (b *Broker) publishMessageLocked(msg channelMessage) {
	for _, ch := range b.messageSubscribers {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (b *Broker) publishActionLocked(action officeActionLog) {
	for _, ch := range b.actionSubscribers {
		select {
		case ch <- action:
		default:
		}
	}
}

func (b *Broker) publishActivityLocked(activity agentActivitySnapshot) {
	for _, ch := range b.activitySubscribers {
		select {
		case ch <- activity:
		default:
		}
	}
}

func (b *Broker) SubscribeMessages(buffer int) (<-chan channelMessage, func()) {
	if buffer <= 0 {
		buffer = 1
	}
	ch := make(chan channelMessage, buffer)

	b.mu.Lock()
	id := b.nextSubscriberID
	b.nextSubscriberID++
	b.messageSubscribers[id] = ch
	b.mu.Unlock()

	return ch, func() {
		b.mu.Lock()
		if existing, ok := b.messageSubscribers[id]; ok {
			delete(b.messageSubscribers, id)
			close(existing)
		}
		b.mu.Unlock()
	}
}

func (b *Broker) SubscribeActions(buffer int) (<-chan officeActionLog, func()) {
	if buffer <= 0 {
		buffer = 1
	}
	ch := make(chan officeActionLog, buffer)

	b.mu.Lock()
	id := b.nextSubscriberID
	b.nextSubscriberID++
	b.actionSubscribers[id] = ch
	b.mu.Unlock()

	return ch, func() {
		b.mu.Lock()
		if existing, ok := b.actionSubscribers[id]; ok {
			delete(b.actionSubscribers, id)
			close(existing)
		}
		b.mu.Unlock()
	}
}

func (b *Broker) SubscribeActivity(buffer int) (<-chan agentActivitySnapshot, func()) {
	if buffer <= 0 {
		buffer = 1
	}
	ch := make(chan agentActivitySnapshot, buffer)

	b.mu.Lock()
	id := b.nextSubscriberID
	b.nextSubscriberID++
	b.activitySubscribers[id] = ch
	b.mu.Unlock()

	return ch, func() {
		b.mu.Lock()
		if existing, ok := b.activitySubscribers[id]; ok {
			delete(b.activitySubscribers, id)
			close(existing)
		}
		b.mu.Unlock()
	}
}

type officeChangeEvent struct {
	Kind string `json:"kind"` // "member_created", "member_removed", "channel_created", "channel_removed"
	Slug string `json:"slug"`
}

func (b *Broker) SubscribeOfficeChanges(buffer int) (<-chan officeChangeEvent, func()) {
	if buffer <= 0 {
		buffer = 1
	}
	ch := make(chan officeChangeEvent, buffer)

	b.mu.Lock()
	id := b.nextSubscriberID
	b.nextSubscriberID++
	b.officeSubscribers[id] = ch
	b.mu.Unlock()

	return ch, func() {
		b.mu.Lock()
		if existing, ok := b.officeSubscribers[id]; ok {
			delete(b.officeSubscribers, id)
			close(existing)
		}
		b.mu.Unlock()
	}
}

func (b *Broker) publishOfficeChangeLocked(evt officeChangeEvent) {
	for _, ch := range b.officeSubscribers {
		select {
		case ch <- evt:
		default:
		}
	}
}

func (b *Broker) UpdateAgentActivity(update agentActivitySnapshot) {
	slug := normalizeAgentLaneSlug(update.Slug)
	if slug == "" {
		return
	}
	channel := normalizeChannelSlug(update.Channel)
	if update.LastTime == "" {
		update.LastTime = time.Now().UTC().Format(time.RFC3339)
	}
	update.Slug = slug
	update.Channel = channel
	key := agentLaneKey(channel, slug)

	b.mu.Lock()
	current := b.activity[key]
	current.Slug = slug
	current.Channel = channel
	if update.Status != "" {
		current.Status = update.Status
	}
	if update.Activity != "" {
		current.Activity = update.Activity
	}
	if update.Detail != "" {
		current.Detail = update.Detail
	}
	if update.LastTime != "" {
		current.LastTime = update.LastTime
	}
	if update.TotalMs > 0 {
		current.TotalMs = update.TotalMs
	}
	if update.FirstEventMs >= 0 {
		current.FirstEventMs = update.FirstEventMs
	}
	if update.FirstTextMs >= 0 {
		current.FirstTextMs = update.FirstTextMs
	}
	if update.FirstToolMs >= 0 {
		current.FirstToolMs = update.FirstToolMs
	}
	b.activity[key] = current
	b.publishActivityLocked(current)
	b.mu.Unlock()
}

// Token returns the shared secret that agents must include in requests.
func (b *Broker) Token() string {
	return b.token
}

// Addr returns the actual listen address (e.g. "127.0.0.1:7890").
func (b *Broker) Addr() string {
	return b.addr
}

// ChannelStore returns the channel store for DM type checks and member lookups.
func (b *Broker) ChannelStore() *channel.Store {
	return b.channelStore
}

// requireAuth wraps a handler to enforce Bearer token authentication.
// Accepts token via Authorization header or ?token= query parameter (for EventSource which can't set headers).
func (b *Broker) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if b.requestHasBrokerAuth(r) {
			next(w, r)
			return
		}
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	}
}

// Start launches the broker on the configured localhost port.
func (b *Broker) Start() error {
	return b.StartOnPort(brokeraddr.ResolvePort())
}

// StartOnPort launches the broker on the given port. Use 0 for an OS-assigned port.
func (b *Broker) StartOnPort(port int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", b.handleHealth) // no auth — used for liveness checks
	mux.HandleFunc("/ready", b.handleReady)
	mux.HandleFunc("/version", b.handleVersion)
	mux.HandleFunc("/session-mode", b.requireAuth(b.handleSessionMode))
	mux.HandleFunc("/focus-mode", b.requireAuth(b.handleFocusMode))
	mux.HandleFunc("/messages/search", b.requireAuth(b.handleSearchMessages))
	mux.HandleFunc("/messages/threads", b.requireAuth(b.handleGetMessageThreads))
	mux.HandleFunc("/messages", b.requireAuth(b.handleMessages))
	mux.HandleFunc("/reactions", b.requireAuth(b.handleReactions))
	mux.HandleFunc("/notifications/nex", b.requireAuth(b.handleNexNotifications))
	mux.HandleFunc("/office-members", b.requireAuth(b.handleOfficeMembers))
	mux.HandleFunc("/office-members/generate", b.requireAuth(b.handleGenerateMember))
	mux.HandleFunc("/channels", b.requireAuth(b.handleChannels))
	mux.HandleFunc("/channels/dm", b.requireAuth(b.handleCreateDM))
	mux.HandleFunc("/channels/generate", b.requireAuth(b.handleGenerateChannel))
	mux.HandleFunc("/channel-members", b.requireAuth(b.handleChannelMembers))
	mux.HandleFunc("/members", b.requireAuth(b.handleMembers))
	mux.HandleFunc("/tasks", b.requireAuth(b.handleTasks))
	mux.HandleFunc("/tasks/ack", b.requireAuth(b.handleTaskAck))
	mux.HandleFunc("/deliveries", b.requireAuth(b.handleDeliveries))
	mux.HandleFunc("/agent-logs", b.requireAuth(b.handleAgentLogs))
	mux.HandleFunc("/task-plan", b.requireAuth(b.handleTaskPlan))
	mux.HandleFunc("/memory", b.requireAuth(b.handleMemory))
	mux.HandleFunc("/studio/dev-console", b.requireAuth(b.handleStudioDevConsole))
	mux.HandleFunc("/studio/dev-console/action", b.requireAuth(b.handleStudioDevConsoleAction))
	mux.HandleFunc("/studio/generate-package", b.requireAuth(b.handleStudioGeneratePackage))
	mux.HandleFunc("/studio/bootstrap-package", b.requireAuth(b.handleOperationBootstrapPackage))
	mux.HandleFunc("/operations/bootstrap-package", b.requireAuth(b.handleOperationBootstrapPackage))
	mux.HandleFunc("/studio/run-workflow", b.requireAuth(b.handleStudioRunWorkflow))
	mux.HandleFunc("/requests", b.requireAuth(b.handleRequests))
	mux.HandleFunc("/requests/answer", b.requireAuth(b.handleRequestAnswer))
	mux.HandleFunc("/interview", b.requireAuth(b.handleInterview))
	mux.HandleFunc("/interview/answer", b.requireAuth(b.handleInterviewAnswer))
	mux.HandleFunc("/reset", b.requireAuth(b.handleReset))
	mux.HandleFunc("/reset-dm", b.requireAuth(b.handleResetDM))
	mux.HandleFunc("/channels/clear", b.requireAuth(b.handleClearChannel))
	mux.HandleFunc("/usage", b.requireAuth(b.handleUsage))
	mux.HandleFunc("/policies", b.requireAuth(b.handlePolicies))
	mux.HandleFunc("/signals", b.requireAuth(b.handleSignals))
	mux.HandleFunc("/decisions", b.requireAuth(b.handleDecisions))
	mux.HandleFunc("/watchdogs", b.requireAuth(b.handleWatchdogs))
	mux.HandleFunc("/actions", b.requireAuth(b.handleActions))
	mux.HandleFunc("/scheduler", b.requireAuth(b.handleScheduler))
	mux.HandleFunc("/skills", b.requireAuth(b.handleSkills))
	mux.HandleFunc("/skills/", b.requireAuth(b.handleSkillsSubpath))
	mux.HandleFunc("/github/webhook", b.handleGitHubWebhook)
	mux.HandleFunc("/telegram/groups", b.requireAuth(b.handleTelegramGroups))
	mux.HandleFunc("/bridges", b.requireAuth(b.handleBridge))
	mux.HandleFunc("/queue", b.requireAuth(b.handleQueue))
	mux.HandleFunc("/sessions", b.requireAuth(b.handleSessions))
	mux.HandleFunc("/company", b.requireAuth(b.handleCompany))
	mux.HandleFunc("/config", b.requireAuth(b.handleConfig))
	mux.HandleFunc("/v1/logs", b.requireAuth(b.handleOTLPLogs))
	mux.HandleFunc("/events", b.handleEvents)
	mux.HandleFunc("/agent-stream/", b.requireAuth(b.handleAgentStream))
	mux.HandleFunc("/web-token", b.handleWebToken)
	// Onboarding: state/progress/complete + prereqs/templates/validate-key + checklist.
	// completeFn posts the first task as a human message and seeds the team.
	onboarding.RegisterRoutes(mux, b.onboardingCompleteFn, b.packSlug, b.requireAuth)
	// Workspace maintenance: POST /workspace/reset (narrow) and the legacy
	// /workspace/shred compatibility route. Shred is state-preserving.
	// Auth-gated via requireAuth because workspace controls should not be
	// reachable without the broker token.
	workspace.RegisterRoutes(mux, b.requireAuth)

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	b.addr = ln.Addr().String()

	b.server = &http.Server{
		Addr:        addr,
		Handler:     b.httpMetricsMiddleware(b.corsMiddleware(b.rateLimitMiddleware(mux))),
		ReadTimeout: 5 * time.Second,
		// No WriteTimeout — SSE streams (agent-stream, events) are open-ended.
	}

	// Write token to a well-known path so tests and tools can authenticate.
	// Use /tmp directly (not os.TempDir which varies by OS).
	tokenFile := strings.TrimSpace(brokerTokenFilePath)
	if tokenFile == "" || tokenFile == brokeraddr.DefaultTokenFile {
		tokenFile = brokeraddr.ResolveTokenFile()
	}
	if tokenFile != "" {
		if dir := filepath.Dir(tokenFile); dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				log.Printf("broker token file: create dir %q: %v", dir, err)
			}
		}
		if err := os.WriteFile(tokenFile, []byte(b.token), 0o600); err != nil {
			log.Printf("broker token file: write %q: %v", tokenFile, err)
		}
	}

	go func() {
		_ = b.server.Serve(ln)
	}()
	return nil
}

// Stop shuts down the broker.
func (b *Broker) Stop() {
	if b.server != nil {
		_ = b.server.Close()
	}
}

func (b *Broker) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Exempt liveness and version checks from rate limiting
		if r.URL.Path == "/health" || r.URL.Path == "/ready" || r.URL.Path == "/version" || r.URL.Path == "/web-token" || b.requestHasBrokerAuth(r) {
			next.ServeHTTP(w, r)
			return
		}
		retryAfter, limited := b.consumeRateLimit(clientIPFromRequest(r))
		if limited {
			seconds := int((retryAfter + time.Second - 1) / time.Second)
			if seconds < 1 {
				seconds = 1
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":"rate_limited"}`)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (b *Broker) consumeRateLimit(clientIP string) (time.Duration, bool) {
	limit := b.rateLimitRequests
	if limit <= 0 {
		limit = defaultRateLimitRequestsPerWindow
	}
	window := b.rateLimitWindow
	if window <= 0 {
		window = defaultRateLimitWindow
	}

	now := time.Now()
	key := rateLimitKey(clientIP)
	cutoff := now.Add(-window)

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.rateLimitBuckets == nil {
		b.rateLimitBuckets = make(map[string]ipRateLimitBucket)
	}
	if b.lastRateLimitPrune.IsZero() || now.Sub(b.lastRateLimitPrune) >= window {
		for ip, bucket := range b.rateLimitBuckets {
			bucket.timestamps = pruneRateLimitEntries(bucket.timestamps, cutoff)
			if len(bucket.timestamps) == 0 {
				delete(b.rateLimitBuckets, ip)
				continue
			}
			b.rateLimitBuckets[ip] = bucket
		}
		b.lastRateLimitPrune = now
	}

	bucket := b.rateLimitBuckets[key]
	bucket.timestamps = pruneRateLimitEntries(bucket.timestamps, cutoff)
	if len(bucket.timestamps) >= limit {
		retryAfter := bucket.timestamps[0].Add(window).Sub(now)
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		b.rateLimitBuckets[key] = bucket
		return retryAfter, true
	}

	bucket.timestamps = append(bucket.timestamps, now)
	b.rateLimitBuckets[key] = bucket
	return 0, false
}

func externalWorkflowRetryAfter(err error, now time.Time) (time.Time, bool) {
	if err == nil {
		return time.Time{}, false
	}
	matches := externalRetryAfterPattern.FindStringSubmatch(err.Error())
	if len(matches) < 2 {
		return time.Time{}, false
	}
	retryAt, parseErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(matches[1]))
	if parseErr != nil {
		return time.Time{}, false
	}
	if retryAt.Before(now) {
		return now, true
	}
	return retryAt, true
}

func pruneRateLimitEntries(entries []time.Time, cutoff time.Time) []time.Time {
	keepIdx := 0
	for keepIdx < len(entries) && !entries[keepIdx].After(cutoff) {
		keepIdx++
	}
	if keepIdx == 0 {
		return entries
	}
	if keepIdx >= len(entries) {
		return nil
	}
	return entries[keepIdx:]
}

func rateLimitKey(remoteAddr string) string {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return "unknown"
	}
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil && strings.TrimSpace(host) != "" {
		return host
	}
	return remoteAddr
}

func clientIPFromRequest(r *http.Request) string {
	if r == nil {
		return "unknown"
	}
	if trustForwardedClientIP(r.RemoteAddr) {
		if forwarded := firstForwardedIP(r.Header.Get("X-Forwarded-For")); forwarded != "" {
			return forwarded
		}
		if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
			return rateLimitKey(realIP)
		}
	}
	return rateLimitKey(r.RemoteAddr)
}

func firstForwardedIP(value string) string {
	for _, part := range strings.Split(value, ",") {
		candidate := rateLimitKey(part)
		if candidate == "" || candidate == "unknown" {
			continue
		}
		if ip := net.ParseIP(candidate); ip != nil {
			return ip.String()
		}
	}
	return ""
}

func trustForwardedClientIP(remoteAddr string) bool {
	host := rateLimitKey(remoteAddr)
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func setProxyClientIPHeaders(header http.Header, remoteAddr string) {
	if header == nil {
		return
	}
	if clientIP := rateLimitKey(remoteAddr); clientIP != "unknown" {
		header.Set("X-Forwarded-For", clientIP)
		header.Set("X-Real-IP", clientIP)
	}
}

func (b *Broker) requestAuthToken(r *http.Request) string {
	if r == nil {
		return ""
	}
	if token := strings.TrimSpace(r.URL.Query().Get("token")); token != "" {
		return token
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return ""
}

func (b *Broker) requestHasBrokerAuth(r *http.Request) bool {
	return b.requestAuthToken(r) == b.token
}

// corsMiddleware adds CORS headers only for the web UI origin.
// If no web UI origins are configured, no CORS headers are set.
func (b *Broker) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if len(b.webUIOrigins) > 0 {
			matched := false
			for _, allowed := range b.webUIOrigins {
				if origin == allowed {
					w.Header().Set("Access-Control-Allow-Origin", allowed)
					matched = true
					break
				}
			}
			// Allow null/empty origin for localhost requests (headless browsers,
			// file:// loads, same-machine tools). Still safe because /web-token
			// restricts to 127.0.0.1 RemoteAddr.
			if !matched && (origin == "null" || origin == "") {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
			if w.Header().Get("Access-Control-Allow-Origin") != "" {
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			}
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleWebToken returns the broker token to localhost clients without requiring auth.
// This lets the web UI fetch the token to authenticate subsequent API calls.
func (b *Broker) handleWebToken(w http.ResponseWriter, r *http.Request) {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if host != "127.0.0.1" && host != "::1" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"token": b.token})
}

func (b *Broker) handleEvents(w http.ResponseWriter, r *http.Request) {
	if !b.requestHasBrokerAuth(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	messages, unsubscribeMessages := b.SubscribeMessages(256)
	defer unsubscribeMessages()
	actions, unsubscribeActions := b.SubscribeActions(256)
	defer unsubscribeActions()
	activity, unsubscribeActivity := b.SubscribeActivity(256)
	defer unsubscribeActivity()
	officeChanges, unsubscribeOffice := b.SubscribeOfficeChanges(64)
	defer unsubscribeOffice()

	writeEvent := func(name string, payload any) error {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	if err := writeEvent("ready", map[string]string{"status": "ok"}); err != nil {
		return
	}

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-messages:
			if !ok || writeEvent("message", map[string]any{"message": msg}) != nil {
				return
			}
		case action, ok := <-actions:
			if !ok || writeEvent("action", map[string]any{"action": action}) != nil {
				return
			}
		case snapshot, ok := <-activity:
			if !ok || writeEvent("activity", map[string]any{"activity": snapshot}) != nil {
				return
			}
		case evt, ok := <-officeChanges:
			if !ok || writeEvent("office_changed", evt) != nil {
				return
			}
		case <-heartbeat.C:
			if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// handleAgentStream serves a per-agent stdout SSE stream.
// Recent lines are replayed as initial history, then new lines are pushed live.
// Path: /agent-stream/{slug}?channel={channel}
func (b *Broker) handleAgentStream(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/agent-stream/")
	if slug == "" {
		http.Error(w, "missing agent slug", http.StatusBadRequest)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	stream := b.AgentStream(slug, r.URL.Query().Get("channel"))

	// Replay recent history so the client sees context immediately.
	history := stream.recent()
	for _, line := range history {
		if _, err := fmt.Fprintf(w, "data: %s\n\n", line); err != nil {
			return
		}
	}
	// If no history, send a connected event so the client knows the stream is live.
	if len(history) == 0 {
		if _, err := fmt.Fprintf(w, "data: [connected]\n\n"); err != nil {
			return
		}
	}
	flusher.Flush()

	lines, unsubscribe := stream.subscribe()
	defer unsubscribe()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case line, ok := <-lines:
			if !ok {
				return
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", line); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// ServeWebUI starts a static file server for the web UI on the given port.
func (b *Broker) ServeWebUI(port int) {
	b.webUIOrigins = []string{
		fmt.Sprintf("http://localhost:%d", port),
		fmt.Sprintf("http://127.0.0.1:%d", port),
	}

	// Resolution order for the web UI assets:
	//   1. filesystem web/dist/ (local dev after `npm run build`)
	//   2. filesystem web/ with index.legacy.html fallback (source checkout w/o React build)
	//   3. embedded FS (single-binary installs via curl | bash)
	exePath, _ := os.Executable()
	webDir := filepath.Join(filepath.Dir(exePath), "web")
	if _, err := os.Stat(webDir); os.IsNotExist(err) {
		webDir = "web"
	}
	var fileServer http.Handler
	serveLegacyFallback := false
	distDir := filepath.Join(webDir, "dist")
	distIndex := filepath.Join(distDir, "index.html")
	if _, err := os.Stat(distIndex); err == nil {
		// Real Vite build output on disk — use it.
		fileServer = http.FileServer(http.Dir(distDir))
	} else if embeddedFS, ok := wuphf.WebFS(); ok {
		// No on-disk build; use embedded assets.
		fileServer = http.FileServer(http.FS(embeddedFS))
	} else if _, err := os.Stat(filepath.Join(webDir, "index.legacy.html")); err == nil {
		// Source checkout without a React build — fall back to the legacy UI.
		fileServer = http.FileServer(http.Dir(webDir))
		serveLegacyFallback = true
	} else {
		// Nothing available; serve webDir as-is so 404s come from the actual FS.
		fileServer = http.FileServer(http.Dir(webDir))
	}
	mux := http.NewServeMux()
	brokerURL := brokeraddr.ResolveBaseURL()
	if addr := strings.TrimSpace(b.Addr()); addr != "" {
		brokerURL = "http://" + addr
	}
	// Same-origin proxy to the broker for app API routes and onboarding wizard routes.
	mux.Handle("/api/", b.webUIProxyHandler(brokerURL, "/api"))
	mux.Handle("/onboarding/", b.webUIProxyHandler(brokerURL, ""))
	// Token endpoint — no auth needed, same origin
	mux.HandleFunc("/api-token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"token":      b.token,
			"broker_url": brokerURL,
		})
	})
	if serveLegacyFallback {
		// Rewrite bare / and /index.html to /index.legacy.html so the legacy
		// vanilla-JS UI loads when the React build output is not present.
		mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" || r.URL.Path == "/index.html" {
				r.URL.Path = "/index.legacy.html"
			}
			fileServer.ServeHTTP(w, r)
		}))
	} else {
		mux.Handle("/", fileServer)
	}
	go func() {
		if err := http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", port), mux); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("broker web UI proxy: listen on :%d: %v", port, err)
		}
	}()
}

func (b *Broker) webUIProxyHandler(brokerURL, stripPrefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath := strings.TrimSpace(r.URL.Path)
		targetPath := r.URL.Path
		if stripPrefix != "" {
			targetPath = strings.TrimPrefix(targetPath, stripPrefix)
		}
		if targetPath == "" {
			targetPath = "/"
		}
		target := brokerURL + targetPath
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}

		proxyReq, err := http.NewRequest(r.Method, target, r.Body)
		if err != nil {
			http.Error(w, "proxy error", http.StatusBadGateway)
			return
		}
		setProxyClientIPHeaders(proxyReq.Header, r.RemoteAddr)
		proxyReq.Header.Set("Authorization", "Bearer "+b.token)
		proxyReq.Header.Set("Content-Type", r.Header.Get("Content-Type"))

		client := http.DefaultClient
		if r.Header.Get("Accept") == "text/event-stream" {
			client = &http.Client{Timeout: 0}
		}
		resp, err := client.Do(proxyReq)
		if err != nil {
			http.Error(w, "broker unreachable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		for k, v := range resp.Header {
			for _, vv := range v {
				w.Header().Add(k, vv)
			}
		}
		if proxiedResponseShouldDisableCache(requestPath, resp.Header.Get("Content-Type")) {
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		}
		w.WriteHeader(resp.StatusCode)

		if resp.Header.Get("Content-Type") == "text/event-stream" {
			flusher, canFlush := w.(http.Flusher)
			buf := make([]byte, 4096)
			for {
				n, readErr := resp.Body.Read(buf)
				if n > 0 {
					w.Write(buf[:n]) //nolint:errcheck
					if canFlush {
						flusher.Flush()
					}
				}
				if readErr != nil {
					break
				}
			}
			return
		}
		_, _ = io.Copy(w, resp.Body)
	})
}

func proxiedResponseShouldDisableCache(targetPath string, contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if strings.HasPrefix(contentType, "text/event-stream") {
		return false
	}
	targetPath = strings.TrimSpace(targetPath)
	return strings.HasPrefix(targetPath, "/api/") || strings.HasPrefix(targetPath, "/onboarding/")
}

// Messages returns all channel messages (for the Go TUI channel view).
func (b *Broker) Messages() []channelMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]channelMessage, len(b.messages))
	copy(out, b.messages)
	return out
}

func (b *Broker) HasPendingInterview() bool {
	return b.HasBlockingRequest()
}

func (b *Broker) HasBlockingRequest() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, req := range b.requests {
		if requestIsActive(req) && req.Blocking {
			return true
		}
	}
	return false
}

// HasRecentlyTaggedAgents returns true if any agent was @mentioned within
// the given duration and has not yet replied (i.e. is presumably "typing").
func (b *Broker) HasRecentlyTaggedAgents(within time.Duration) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.lastTaggedAt) == 0 {
		return false
	}
	cutoff := time.Now().Add(-within)
	for _, t := range b.lastTaggedAt {
		if t.After(cutoff) {
			return true
		}
	}
	return false
}

func (b *Broker) EnabledMembers(channel string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sessionMode == SessionModeOneOnOne {
		return []string{b.oneOnOneAgent}
	}
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		channel = "general"
	}
	if ch := b.findChannelLocked(channel); ch != nil {
		return b.enabledChannelMembersLocked(channel, ch.Members)
	}
	return nil
}

func (b *Broker) OfficeMembers() []officeMember {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]officeMember, len(b.members))
	copy(out, b.members)
	return out
}

func (b *Broker) ChannelMessages(channel string) []channelMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureMessageIndexesLocked()
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		channel = "general"
	}
	channelIndexes := b.messageIndexesByChannel[channel]
	out := make([]channelMessage, 0, len(channelIndexes))
	for _, idx := range channelIndexes {
		msg := b.messages[idx]
		if isHumanLikeActor(msg.From) || isSystemActor(msg.From) || !messageContentLooksLikeDisallowedAgentChannelContent(msg.Content) {
			out = append(out, msg)
		}
	}
	return out
}

// AllMessages returns a copy of all messages across all channels, ordered by
// creation time. Use this when the caller needs to search across channels rather
// than in a single known channel.
func (b *Broker) AllMessages() []channelMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]channelMessage, len(b.messages))
	copy(out, b.messages)
	return out
}

// SurfaceChannels returns all channels that have a surface configured for the given provider.
func (b *Broker) SurfaceChannels(provider string) []teamChannel {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []teamChannel
	for _, ch := range b.channels {
		if ch.Surface != nil && ch.Surface.Provider == provider {
			cp := ch
			cp.Members = append([]string(nil), ch.Members...)
			cp.Disabled = append([]string(nil), ch.Disabled...)
			s := *ch.Surface
			cp.Surface = &s
			out = append(out, cp)
		}
	}
	return out
}

// ExternalQueue returns messages that need to be sent to external surfaces
// for the given provider. Each message is returned at most once.
func (b *Broker) ExternalQueue(provider string) []channelMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.externalDelivered == nil {
		b.externalDelivered = make(map[string]struct{})
	}
	surfaceChannels := make(map[string]struct{})
	for _, ch := range b.channels {
		if ch.Surface != nil && ch.Surface.Provider == provider {
			surfaceChannels[ch.Slug] = struct{}{}
		}
	}
	var out []channelMessage
	for _, msg := range b.messages {
		ch := normalizeChannelSlug(msg.Channel)
		if _, ok := surfaceChannels[ch]; !ok {
			continue
		}
		if _, delivered := b.externalDelivered[msg.ID]; delivered {
			continue
		}
		b.externalDelivered[msg.ID] = struct{}{}
		out = append(out, msg)
	}
	return out
}

// EnsureBridgedMember registers a bridged external agent as an office member
// so it appears in the sidebar and can be @mentioned. Idempotent — calling with
// an existing slug is a no-op. CreatedBy tags the source (e.g. "telegram") so
// the UI can distinguish bridged agents from built-ins or user-generated ones.
func (b *Broker) EnsureBridgedMember(slug, name, createdBy string) error {
	slug = normalizeChannelSlug(slug)
	if slug == "" {
		return fmt.Errorf("slug required")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.findMemberLocked(slug) != nil {
		return nil
	}
	member := officeMember{
		Slug:      slug,
		Name:      strings.TrimSpace(name),
		Role:      "Bridged agent",
		CreatedBy: strings.TrimSpace(createdBy),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if member.Name == "" {
		member.Name = slug
	}
	applyOfficeMemberDefaults(&member)
	b.members = append(b.members, member)
	// Make sure the bridged agent shows up in #general so @mentions work.
	for i := range b.channels {
		if b.channels[i].Slug == "general" {
			if !containsString(b.channels[i].Members, slug) {
				b.channels[i].Members = append(b.channels[i].Members, slug)
			}
			break
		}
	}
	if err := b.saveLocked(); err != nil {
		return err
	}
	b.publishOfficeChangeLocked(officeChangeEvent{Kind: "member_created", Slug: slug})
	return nil
}

// EnsureDirectChannel opens (or returns) the 1:1 DM channel between the
// default human member and agentSlug. Returns the canonical channel slug
// (pair-sorted via channel.DirectSlug). Safe to call repeatedly; the DM row
// is upserted in both the channel store and the in-memory broker table so
// it shows up in the sidebar and findChannelLocked resolves it.
func (b *Broker) EnsureDirectChannel(agentSlug string) (string, error) {
	agentSlug = normalizeActorSlug(agentSlug)
	if agentSlug == "" {
		return "", fmt.Errorf("agent slug required")
	}
	if b.channelStore == nil {
		return "", fmt.Errorf("channel store not initialized")
	}
	ch, err := b.channelStore.GetOrCreateDirect("human", agentSlug)
	if err != nil {
		return "", fmt.Errorf("channel store GetOrCreateDirect: %w", err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.findChannelLocked(ch.Slug) == nil {
		now := time.Now().UTC().Format(time.RFC3339)
		b.channels = append(b.channels, teamChannel{
			Slug:        ch.Slug,
			Name:        ch.Slug,
			Type:        "dm",
			Description: "Direct messages with " + agentSlug,
			Members:     []string{"human", agentSlug},
			CreatedBy:   "wuphf",
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		if err := b.saveLocked(); err != nil {
			return "", err
		}
	}
	return ch.Slug, nil
}

// DMPartner returns the non-human member slug of a 1:1 DM channel. Returns
// "" if the channel is not a DM, does not exist, or is a group DM. Used by
// surface bridges (Telegram, Slack, etc.) to resolve "who is the human
// talking to" when routing DM posts to the right agent without requiring an
// @mention.
func (b *Broker) DMPartner(channelSlug string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := b.findChannelLocked(normalizeChannelSlug(channelSlug))
	if ch == nil || !ch.isDM() {
		return ""
	}
	if len(ch.Members) != 2 {
		return ""
	}
	for _, m := range ch.Members {
		if m != "human" && m != "you" {
			return m
		}
	}
	return ""
}

// PostInboundSurfaceMessage posts a message from an external surface into the broker channel.
func (b *Broker) PostInboundSurfaceMessage(from, channel, content, provider string) (channelMessage, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		return channelMessage{}, fmt.Errorf("channel required for surface message")
	}
	if b.findChannelLocked(channel) == nil {
		if IsDMSlug(channel) {
			b.ensureDMConversationLocked(channel)
		} else {
			return channelMessage{}, fmt.Errorf("channel not found: %s", channel)
		}
	}
	b.counter++
	msg := channelMessage{
		ID:          fmt.Sprintf("msg-%d", b.counter),
		From:        from,
		Channel:     channel,
		Kind:        "surface",
		Source:      provider,
		SourceLabel: provider,
		Content:     strings.TrimSpace(content),
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
	b.appendMessageLocked(msg)
	// Mark as already delivered so it doesn't bounce back to the same surface
	if b.externalDelivered == nil {
		b.externalDelivered = make(map[string]struct{})
	}
	b.externalDelivered[msg.ID] = struct{}{}
	if err := b.saveLocked(); err != nil {
		return channelMessage{}, err
	}
	return msg, nil
}

func (b *Broker) ChannelTasks(channel string) []teamTask {
	b.mu.Lock()
	b.ensureTaskIndexesLocked()
	out := b.copyIndexedTasksLocked(b.taskIndexes.byChannel[normalizeTaskIndexChannel(channel)])
	b.mu.Unlock()
	return coalesceTaskView(out)
}

// AllTasks returns a copy of all tasks across all channels. Use this when the
// caller needs to search across channels rather than in a single known channel.
func (b *Broker) AllTasks() []teamTask {
	b.mu.Lock()
	out := make([]teamTask, len(b.tasks))
	copy(out, b.tasks)
	b.mu.Unlock()
	return coalesceTaskView(out)
}

// InFlightTasks returns tasks that have an assigned owner and a non-terminal
// status (anything except "done", "completed", "canceled", or "cancelled").
func (b *Broker) InFlightTasks() []teamTask {
	b.mu.Lock()
	b.ensureTaskIndexesLocked()
	out := make([]teamTask, 0, len(b.tasks))
	for status, indexes := range b.taskIndexes.byStatus {
		switch status {
		case "done", "completed", "canceled", "cancelled":
			continue
		}
		for _, idx := range indexes {
			if idx < 0 || idx >= len(b.tasks) {
				continue
			}
			task := b.tasks[idx]
			if strings.TrimSpace(task.Owner) == "" {
				continue
			}
			out = append(out, task)
		}
	}
	b.mu.Unlock()
	return coalesceTaskView(out)
}

// RecentHumanMessages returns up to limit messages sent by a human or external
// sender ("you", "human", or "nex"). The returned slice contains the most
// recent messages in chronological order (earliest first).
func (b *Broker) RecentHumanMessages(limit int) []channelMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	var human []channelMessage
	for _, msg := range b.messages {
		f := strings.ToLower(strings.TrimSpace(msg.From))
		if f == "you" || f == "human" || f == "nex" {
			human = append(human, msg)
		}
	}
	if len(human) <= limit {
		return human
	}
	return human[len(human)-limit:]
}

// UnackedTasks returns in_progress tasks with an owner that have not been acked
// and were created more than the given duration ago.
func (b *Broker) UnackedTasks(timeout time.Duration) []teamTask {
	b.mu.Lock()
	defer b.mu.Unlock()
	cutoff := time.Now().UTC().Add(-timeout)
	out := make([]teamTask, 0)
	for _, task := range b.tasks {
		if task.Status != "in_progress" || task.Owner == "" || task.AckedAt != "" {
			continue
		}
		created, err := time.Parse(time.RFC3339, task.CreatedAt)
		if err != nil {
			continue
		}
		if created.Before(cutoff) {
			out = append(out, task)
		}
	}
	return out
}

func (b *Broker) Requests(channel string, includeResolved bool) []humanInterview {
	b.mu.Lock()
	defer b.mu.Unlock()
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		channel = "general"
	}
	out := make([]humanInterview, 0, len(b.requests))
	for _, req := range b.requests {
		reqChannel := normalizeChannelSlug(req.Channel)
		if reqChannel == "" {
			reqChannel = "general"
		}
		if reqChannel != channel {
			continue
		}
		if !includeResolved && !requestIsActive(req) {
			continue
		}
		out = append(out, req)
	}
	return out
}

func (b *Broker) Actions() []officeActionLog {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]officeActionLog, len(b.actions))
	copy(out, b.actions)
	return out
}

func (b *Broker) Signals() []officeSignalRecord {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]officeSignalRecord, len(b.signals))
	copy(out, b.signals)
	return out
}

func (b *Broker) Decisions() []officeDecisionRecord {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]officeDecisionRecord, len(b.decisions))
	copy(out, b.decisions)
	return out
}

func (b *Broker) Watchdogs() []watchdogAlert {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]watchdogAlert, len(b.watchdogs))
	copy(out, b.watchdogs)
	return out
}

func (b *Broker) Scheduler() []schedulerJob {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]schedulerJob, len(b.scheduler))
	copy(out, b.scheduler)
	return out
}

type queueSnapshot struct {
	Actions   []officeActionLog      `json:"actions"`
	Signals   []officeSignalRecord   `json:"signals,omitempty"`
	Decisions []officeDecisionRecord `json:"decisions,omitempty"`
	Watchdogs []watchdogAlert        `json:"watchdogs,omitempty"`
	Scheduler []schedulerJob         `json:"scheduler"`
	Due       []schedulerJob         `json:"due,omitempty"`
}

func (b *Broker) QueueSnapshot() queueSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pruneSchedulerJobsLocked(time.Now().UTC())
	return queueSnapshot{
		Actions:   append([]officeActionLog(nil), b.actions...),
		Signals:   append([]officeSignalRecord(nil), b.signals...),
		Decisions: append([]officeDecisionRecord(nil), b.decisions...),
		Watchdogs: append([]watchdogAlert(nil), b.watchdogs...),
		Scheduler: append([]schedulerJob(nil), b.scheduler...),
		Due:       append([]schedulerJob(nil), b.dueSchedulerJobsLocked(time.Now().UTC())...),
	}
}

func (b *Broker) SetSessionObservabilityFn(fn func() SessionObservabilitySnapshot) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sessionObservabilityFn = fn
}

func (b *Broker) dueSchedulerJobsLocked(now time.Time) []schedulerJob {
	now = now.UTC()
	var out []schedulerJob
	for _, job := range b.scheduler {
		if strings.EqualFold(strings.TrimSpace(job.Status), "done") || strings.EqualFold(strings.TrimSpace(job.Status), "canceled") {
			continue
		}
		target := strings.TrimSpace(job.NextRun)
		if target == "" {
			continue
		}
		dueAt, err := time.Parse(time.RFC3339, target)
		if err != nil {
			continue
		}
		if !dueAt.After(now) {
			out = append(out, job)
		}
	}
	return out
}

func (b *Broker) Reset() {
	b.mu.Lock()
	mode := b.sessionMode
	agent := b.oneOnOneAgent
	channels := append([]teamChannel(nil), b.channels...)
	members := append([]officeMember(nil), b.members...)
	b.replaceMessagesLocked(nil)
	b.members = members
	b.channels = channels
	b.sessionMode = mode
	b.oneOnOneAgent = agent
	b.tasks = nil
	b.requests = nil
	b.actions = nil
	b.signals = nil
	b.decisions = nil
	b.watchdogs = nil
	b.policies = nil
	b.scheduler = nil
	b.pendingInterview = nil
	b.activity = make(map[string]agentActivitySnapshot)
	b.counter = 0
	b.notificationSince = ""
	b.insightsSince = ""
	b.usage = teamUsageState{Agents: make(map[string]usageTotals)}
	b.ensureDefaultOfficeMembersLocked()
	b.ensureDefaultChannelsLocked()
	b.normalizeLoadedStateLocked()
	b.rebuildTaskIndexesLocked()
	// Restore session preferences after normalization: Reset() clears content but
	// should not re-validate the user's explicit 1:1 agent choice against the
	// current default member list (which may differ from the active pack).
	b.sessionMode = mode
	b.oneOnOneAgent = agent
	_ = b.saveLocked()
	_ = os.Remove(brokerStateSnapshotPath())
	b.mu.Unlock()
}

func defaultBrokerStatePath() string {
	// Env override lets probes and test harnesses isolate broker state from
	// the user's real ~/.wuphf/team/ dir without needing to remap HOME (which
	// breaks macOS keychain-backed auth for bundled CLIs like Claude Code).
	if p := strings.TrimSpace(os.Getenv("WUPHF_BROKER_STATE_PATH")); p != "" {
		return p
	}
	home := config.RuntimeHomeDir()
	if home == "" {
		return filepath.Join(".wuphf", "team", "broker-state.json")
	}
	return filepath.Join(home, ".wuphf", "team", "broker-state.json")
}

func brokerStateSnapshotPath() string {
	return brokerStatePath() + ".last-good"
}

func brokerStateHistoryDir(path string) string {
	return path + ".history"
}

func resolvedCloudBackupSettings() backup.Settings {
	return backup.Settings{
		Provider: config.ResolveCloudBackupProvider(),
		Bucket:   config.ResolveCloudBackupBucket(),
		Prefix:   config.ResolveCloudBackupPrefix(),
	}
}

func loadBrokerStateFile(path string) (brokerState, error) {
	var state brokerState
	data, err := os.ReadFile(path)
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	return state, nil
}

func loadBrokerStateFromBytes(data []byte) (brokerState, error) {
	var state brokerState
	if len(data) == 0 {
		return state, os.ErrNotExist
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	return state, nil
}

func persistBrokerStateLocal(path, snapshotPath string, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	if snapshotPath != "" {
		if err := os.WriteFile(snapshotPath, data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func loadBrokerStateCloudFallback(settings backup.Settings) (brokerState, []byte, error) {
	if !settings.Enabled() {
		return brokerState{}, nil, os.ErrNotExist
	}
	currentData, currentErr := backup.ReadBytes(nil, settings, "team/broker-state.json")
	lastGoodData, lastGoodErr := backup.ReadBytes(nil, settings, "team/broker-state.last-good.json")
	currentState, currentStateErr := loadBrokerStateFromBytes(currentData)
	lastGoodState, lastGoodStateErr := loadBrokerStateFromBytes(lastGoodData)

	bestState := brokerState{}
	var bestData []byte
	bestScore := -1
	if currentStateErr == nil {
		if score := brokerStateActivityScore(currentState); score > bestScore {
			bestScore = score
			bestState = currentState
			bestData = currentData
		}
	}
	if lastGoodStateErr == nil {
		if score := brokerStateActivityScore(lastGoodState); score > bestScore {
			bestScore = score
			bestState = lastGoodState
			bestData = lastGoodData
		}
	}
	if bestScore >= 0 {
		return bestState, bestData, nil
	}
	if currentErr != nil && !backup.IsNotFound(currentErr) {
		return brokerState{}, nil, currentErr
	}
	if lastGoodErr != nil && !backup.IsNotFound(lastGoodErr) {
		return brokerState{}, nil, lastGoodErr
	}
	return brokerState{}, nil, os.ErrNotExist
}

func (b *Broker) applyLoadedStateLocked(state brokerState) error {
	b.messages = append([]channelMessage(nil), state.Messages...)
	if !b.restoreMessageIndexSnapshotLocked(state.MessageIndexSnapshot) {
		b.rebuildMessageIndexesLocked()
	}
	b.members = state.Members
	b.channels = state.Channels
	b.sessionMode = state.SessionMode
	b.oneOnOneAgent = state.OneOnOneAgent
	b.focusMode = state.FocusMode
	b.tasks = state.Tasks
	b.requests = state.Requests
	b.actions = state.Actions
	b.signals = state.Signals
	b.decisions = state.Decisions
	b.watchdogs = state.Watchdogs
	b.policies = state.Policies
	b.scheduler = state.Scheduler
	b.skills = state.Skills
	b.executionNodes = state.ExecutionNodes
	b.sharedMemory = state.SharedMemory
	b.counter = state.Counter
	b.notificationSince = state.NotificationSince
	b.insightsSince = state.InsightsSince
	b.pendingInterview = state.PendingInterview
	b.usage = state.Usage
	if b.usage.Agents == nil {
		b.usage.Agents = make(map[string]usageTotals)
	}
	b.usage.Session = usageTotals{}
	if len(b.requests) == 0 && b.pendingInterview != nil {
		b.requests = []humanInterview{*b.pendingInterview}
	}
	b.observabilityMu.Lock()
	b.observabilityHistory = append([]brokerObservabilitySample(nil), state.ObservabilityHistory...)
	b.observabilityMu.Unlock()
	if len(state.ChannelStore) > 0 {
		if err := json.Unmarshal(state.ChannelStore, b.channelStore); err != nil {
			return fmt.Errorf("unmarshal channel_store: %w", err)
		}
		b.channelStore.MigrateLegacyDM()
	}
	for i := range b.messages {
		b.messages[i].Channel = channel.MigrateDMSlugString(b.messages[i].Channel)
	}
	for i := range b.tasks {
		b.tasks[i].Channel = channel.MigrateDMSlugString(b.tasks[i].Channel)
	}
	for i := range b.requests {
		b.requests[i].Channel = channel.MigrateDMSlugString(b.requests[i].Channel)
	}
	b.ensureDefaultChannelsLocked()
	b.ensureDefaultOfficeMembersLocked()
	b.normalizeLoadedStateLocked()
	b.rebuildTaskIndexesLocked()
	return nil
}

func brokerStateActivityScore(state brokerState) int {
	score := 0
	score += len(state.Messages) * 10
	score += len(state.Tasks) * 20
	score += len(activeRequests(state.Requests)) * 10
	score += len(state.Actions) * 4
	score += len(state.Signals) * 4
	score += len(state.Decisions) * 4
	score += len(state.Skills) * 2
	score += len(state.Policies)
	for _, ns := range state.SharedMemory {
		score += len(ns)
	}
	if state.PendingInterview != nil {
		score += 5
	}
	return score
}

func brokerStateShouldSnapshot(state brokerState) bool {
	return brokerStateActivityScore(state) > 0
}

func (b *Broker) stateLocked() brokerState {
	var channelStoreRaw json.RawMessage
	if b.channelStore != nil {
		if raw, err := json.Marshal(b.channelStore); err == nil {
			channelStoreRaw = raw
		}
	}
	messageIndexSnapshot := b.messageIndexSnapshotLocked()
	observabilityHistory := b.observabilityHistorySnapshot(brokerObservabilityHistoryLimit)
	return brokerState{
		ChannelStore:         channelStoreRaw,
		Messages:             b.messages,
		MessageIndexSnapshot: messageIndexSnapshot,
		Members:              b.members,
		Channels:             b.channels,
		SessionMode:          b.sessionMode,
		OneOnOneAgent:        b.oneOnOneAgent,
		FocusMode:            b.focusMode,
		Tasks:                b.tasks,
		Requests:             b.requests,
		Actions:              b.actions,
		Signals:              b.signals,
		Decisions:            b.decisions,
		Watchdogs:            b.watchdogs,
		Policies:             b.policies,
		Scheduler:            b.scheduler,
		Skills:               b.skills,
		ExecutionNodes:       b.executionNodes,
		SharedMemory:         b.sharedMemory,
		Counter:              b.counter,
		NotificationSince:    b.notificationSince,
		InsightsSince:        b.insightsSince,
		PendingInterview:     firstBlockingRequest(b.requests),
		Usage: func() teamUsageState {
			usage := b.usage
			usage.Session = usageTotals{}
			return usage
		}(),
		ObservabilityHistory: observabilityHistory,
	}
}

func (b *Broker) marshalStateLocked() (brokerState, []byte, error) {
	b.recordObservabilitySampleLocked(time.Now().UTC())
	state := b.stateLocked()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return brokerState{}, nil, err
	}
	return state, data, nil
}

func brokerStateStartupGuardDir(path string) string {
	return path + ".startup-guard"
}

func persistBrokerStateStartupGuard(path string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}
	dir := brokerStateStartupGuardDir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s.json", time.Now().UTC().Format("20060102T150405.000000000Z"))
	tmp := filepath.Join(dir, name+".tmp")
	finalPath := filepath.Join(dir, name)
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, finalPath); err != nil {
		return "", err
	}
	return finalPath, nil
}

func (b *Broker) collectStartupReconcileGuardLocked(validChannels map[string]struct{}) startupReconcileGuardSummary {
	if b == nil {
		return startupReconcileGuardSummary{}
	}
	missing := make(map[string]struct{})
	channelMissing := func(raw string) bool {
		slug := normalizeChannelSlug(raw)
		if slug == "" || IsDMSlug(slug) {
			return false
		}
		_, ok := validChannels[slug]
		if !ok {
			missing[slug] = struct{}{}
		}
		return !ok
	}
	summary := startupReconcileGuardSummary{}
	for _, msg := range b.messages {
		if channelMissing(msg.Channel) {
			summary.MessageCount++
		}
	}
	for _, task := range b.tasks {
		if channelMissing(task.Channel) {
			summary.TaskCount++
		}
	}
	for _, req := range b.requests {
		if channelMissing(req.Channel) {
			summary.RequestCount++
		}
	}
	for _, action := range b.actions {
		if channelMissing(action.Channel) {
			summary.ActionCount++
		}
	}
	for _, node := range b.executionNodes {
		if channelMissing(node.Channel) {
			summary.NodeCount++
		}
	}
	if len(missing) == 0 {
		return summary
	}
	summary.MissingChannels = make([]string, 0, len(missing))
	for slug := range missing {
		summary.MissingChannels = append(summary.MissingChannels, slug)
	}
	sort.Strings(summary.MissingChannels)
	return summary
}

func (b *Broker) hasSignalDedupeKeyLocked(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	for _, signal := range b.signals {
		if strings.TrimSpace(signal.DedupeKey) == key {
			return true
		}
	}
	return false
}

func (b *Broker) persistStartupReconcileGuardLocked(validChannels map[string]struct{}) bool {
	summary := b.collectStartupReconcileGuardLocked(validChannels)
	if !summary.active() {
		return false
	}
	dedupeKey := summary.dedupeKey()
	if b.hasSignalDedupeKeyLocked(dedupeKey) {
		return false
	}
	_, data, err := b.marshalStateLocked()
	if err != nil {
		log.Printf("broker: marshal startup reconcile guard state: %v", err)
		return false
	}
	snapshotPath, snapErr := persistBrokerStateStartupGuard(brokerStatePath(), data)
	if snapErr != nil {
		log.Printf("broker: persist startup reconcile guard snapshot: %v", snapErr)
	}
	b.counter++
	signalID := fmt.Sprintf("sig-%d", b.counter)
	now := time.Now().UTC().Format(time.RFC3339)
	content := fmt.Sprintf(
		"Startup manifest reconcile found records that would be discarded because their channels are not materialized in broker state. %s.",
		summary.detail(),
	)
	if snapshotPath != "" {
		content += fmt.Sprintf(" Raw pre-reconcile snapshot saved at %s.", snapshotPath)
	}
	if snapErr != nil {
		content += fmt.Sprintf(" Snapshot persistence failed: %v.", snapErr)
	}
	b.signals = append(b.signals, officeSignalRecord{
		ID:         signalID,
		Source:     "wuphf",
		SourceRef:  snapshotPath,
		Kind:       "startup_reconcile_guard",
		Title:      "Startup reconcile would discard channel records",
		Content:    content,
		Owner:      "ceo",
		Confidence: "high",
		Urgency:    "high",
		DedupeKey:  dedupeKey,
		Blocking:   false,
		CreatedAt:  now,
	})
	b.appendActionLocked("startup_reconcile_guard", "wuphf", "general", "wuphf", truncateSummary(summary.detail(), 200), signalID)
	return true
}

func (b *Broker) loadState() error {
	path := brokerStatePath()
	state, err := loadBrokerStateFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		state = brokerState{}
	}
	snapshotPath := brokerStateSnapshotPath()
	if snapshot, snapErr := loadBrokerStateFile(snapshotPath); snapErr == nil {
		if brokerStateActivityScore(snapshot) > brokerStateActivityScore(state) {
			state = snapshot
		}
	}
	localScore := brokerStateActivityScore(state)
	if err := b.applyLoadedStateLocked(state); err != nil {
		return err
	}
	b.deferredCloudRestore = brokerDeferredCloudRestore{
		Pending:     resolvedCloudBackupSettings().Enabled(),
		BaseCounter: b.counter,
		BaseScore:   localScore,
	}
	return nil
}

func (b *Broker) HasDeferredCloudRestore() bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.deferredCloudRestore.Pending
}

func (b *Broker) RestoreDeferredCloudState() (bool, error) {
	if b == nil {
		return false, nil
	}

	b.mu.Lock()
	restore := b.deferredCloudRestore
	b.mu.Unlock()
	if !restore.Pending {
		return false, nil
	}

	settings := resolvedCloudBackupSettings()
	if !settings.Enabled() {
		b.mu.Lock()
		b.deferredCloudRestore.Pending = false
		b.mu.Unlock()
		return false, nil
	}

	cloudState, cloudData, err := loadBrokerStateCloudFallback(settings)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			b.mu.Lock()
			b.deferredCloudRestore.Pending = false
			b.mu.Unlock()
			return false, nil
		}
		return false, err
	}

	cloudScore := brokerStateActivityScore(cloudState)
	path := brokerStatePath()
	snapshotPath := brokerStateSnapshotPath()

	b.mu.Lock()
	defer b.mu.Unlock()

	restore = b.deferredCloudRestore
	if !restore.Pending {
		return false, nil
	}
	currentScore := brokerStateActivityScore(b.stateLocked())
	if b.counter != restore.BaseCounter || currentScore != restore.BaseScore {
		b.deferredCloudRestore.Pending = false
		return false, nil
	}
	if cloudScore <= currentScore {
		b.deferredCloudRestore.Pending = false
		return false, nil
	}
	if err := b.applyLoadedStateLocked(cloudState); err != nil {
		return false, err
	}
	if len(cloudData) > 0 {
		_ = persistBrokerStateLocal(path, snapshotPath, cloudData)
	}
	b.deferredCloudRestore.Pending = false
	b.publishOfficeChangeLocked(officeChangeEvent{Kind: "state_restored", Slug: "broker"})
	if err := b.saveLocked(); err != nil {
		return false, err
	}
	return true, nil
}

func (b *Broker) saveLocked() error {
	b.suppressActiveFollowUpTasksWhenAuditCanceledLocked(time.Now().UTC().Format(time.RFC3339))
	b.rebuildTaskIndexesLocked()
	path := brokerStatePath()
	snapshotPath := brokerStateSnapshotPath()
	historyDir := brokerStateHistoryDir(path)
	if len(b.messages) == 0 && len(b.tasks) == 0 && len(activeRequests(b.requests)) == 0 && len(b.actions) == 0 && len(b.signals) == 0 && len(b.decisions) == 0 && len(b.watchdogs) == 0 && len(b.policies) == 0 && len(b.scheduler) == 0 && len(b.skills) == 0 && len(b.executionNodes) == 0 && len(b.sharedMemory) == 0 && isDefaultChannelState(b.channels) && isDefaultOfficeMemberState(b.members) && b.counter == 0 && b.notificationSince == "" && b.insightsSince == "" && usageStateIsZero(b.usage) && b.sessionMode == SessionModeOffice && b.oneOnOneAgent == DefaultOneOnOneAgent {
		settings := resolvedCloudBackupSettings()
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Remove(snapshotPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.RemoveAll(historyDir); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if settings.Enabled() {
			scheduleBrokerCloudBackupPlan(brokerCloudBackupPlan{
				settings:       settings,
				deleteCurrent:  true,
				deleteSnapshot: true,
			})
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	state, data, err := b.marshalStateLocked()
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := atomicReplaceBrokerStateFile(tmp, path); err != nil {
		return err
	}
	if brokerStateShouldSnapshot(state) {
		snapshotTmp := snapshotPath + ".tmp"
		if err := os.WriteFile(snapshotTmp, data, 0o600); err != nil {
			return err
		}
		if err := atomicReplaceBrokerStateFile(snapshotTmp, snapshotPath); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(historyDir, 0o700); err != nil {
		return err
	}
	historyName := fmt.Sprintf("%s-%06d.json", time.Now().UTC().Format("20060102T150405.000000000Z"), b.counter)
	historyTmp := filepath.Join(historyDir, historyName+".tmp")
	if err := os.WriteFile(historyTmp, data, 0o600); err != nil {
		return err
	}
	if err := atomicReplaceBrokerStateFile(historyTmp, filepath.Join(historyDir, historyName)); err != nil {
		return err
	}
	settings := resolvedCloudBackupSettings()
	if settings.Enabled() {
		scheduleBrokerCloudBackupPlan(brokerCloudBackupPlan{
			settings:      settings,
			data:          append([]byte(nil), data...),
			historyName:   historyName,
			writeCurrent:  true,
			writeSnapshot: brokerStateShouldSnapshot(state),
			writeHistory:  true,
		})
	}
	return nil
}

func scheduleBrokerCloudBackupPlan(plan brokerCloudBackupPlan) {
	if !plan.settings.Enabled() {
		return
	}
	updateBrokerCloudBackupRuntime(func(state *brokerCloudBackupRuntime) {
		state.Pending = true
	})
	brokerCloudBackupQueueMu.Lock()
	if brokerCloudBackupPendingPlan == nil {
		cloned := cloneBrokerCloudBackupPlan(plan)
		brokerCloudBackupPendingPlan = &cloned
	} else {
		merged := mergeBrokerCloudBackupPlans(*brokerCloudBackupPendingPlan, plan)
		brokerCloudBackupPendingPlan = &merged
	}
	shouldStart := !brokerCloudBackupWorkerActive
	if shouldStart {
		brokerCloudBackupWorkerActive = true
	}
	brokerCloudBackupQueueMu.Unlock()
	if shouldStart {
		runBrokerCloudBackupAsync(runBrokerCloudBackupWorker)
	}
}

func cloneBrokerCloudBackupPlan(plan brokerCloudBackupPlan) brokerCloudBackupPlan {
	cloned := plan
	if len(plan.data) > 0 {
		cloned.data = append([]byte(nil), plan.data...)
	}
	return cloned
}

func mergeBrokerCloudBackupPlans(current, next brokerCloudBackupPlan) brokerCloudBackupPlan {
	merged := cloneBrokerCloudBackupPlan(current)
	if next.settings.Enabled() {
		merged.settings = next.settings
	}
	if len(next.data) > 0 {
		merged.data = append([]byte(nil), next.data...)
	}
	if next.writeCurrent || next.deleteCurrent {
		merged.writeCurrent = next.writeCurrent
		merged.deleteCurrent = next.deleteCurrent
	}
	if next.writeSnapshot || next.deleteSnapshot {
		merged.writeSnapshot = next.writeSnapshot
		merged.deleteSnapshot = next.deleteSnapshot
	}
	if next.writeHistory {
		merged.writeHistory = true
		merged.historyName = next.historyName
	}
	return merged
}

func brokerCloudBackupRetryDelay(failures int) time.Duration {
	if failures <= 0 {
		return 0
	}
	if failures > 6 {
		failures = 6
	}
	return time.Second * time.Duration(1<<(failures-1))
}

func runBrokerCloudBackupWorker() {
	failures := 0
	for {
		brokerCloudBackupQueueMu.Lock()
		if brokerCloudBackupPendingPlan == nil {
			brokerCloudBackupWorkerActive = false
			brokerCloudBackupState.Pending = false
			brokerCloudBackupQueueMu.Unlock()
			return
		}
		plan := cloneBrokerCloudBackupPlan(*brokerCloudBackupPendingPlan)
		brokerCloudBackupPendingPlan = nil
		brokerCloudBackupQueueMu.Unlock()

		if brokerCloudBackupDebounceWindow > 0 {
			sleepBrokerCloudBackup(brokerCloudBackupDebounceWindow)
			brokerCloudBackupQueueMu.Lock()
			if brokerCloudBackupPendingPlan != nil {
				merged := mergeBrokerCloudBackupPlans(plan, *brokerCloudBackupPendingPlan)
				plan = merged
				brokerCloudBackupPendingPlan = nil
			}
			brokerCloudBackupQueueMu.Unlock()
		}

		updateBrokerCloudBackupRuntime(func(state *brokerCloudBackupRuntime) {
			state.Pending = true
			state.LastAttemptAt = time.Now().UTC().Format(time.RFC3339)
		})

		if err := persistBrokerCloudBackupPlan(plan); err != nil {
			failures++
			delay := brokerCloudBackupRetryDelay(failures)
			log.Printf("broker: cloud backup mirror failed (retry in %s): %v", delay, err)
			updateBrokerCloudBackupRuntime(func(state *brokerCloudBackupRuntime) {
				state.Pending = true
				state.LastError = truncate(err.Error(), 200)
				state.ConsecutiveFailures = failures
			})

			brokerCloudBackupQueueMu.Lock()
			if brokerCloudBackupPendingPlan == nil {
				cloned := cloneBrokerCloudBackupPlan(plan)
				brokerCloudBackupPendingPlan = &cloned
			} else {
				merged := mergeBrokerCloudBackupPlans(plan, *brokerCloudBackupPendingPlan)
				brokerCloudBackupPendingPlan = &merged
			}
			brokerCloudBackupQueueMu.Unlock()

			sleepBrokerCloudBackup(delay)
			continue
		}

		failures = 0
		updateBrokerCloudBackupRuntime(func(state *brokerCloudBackupRuntime) {
			state.Pending = false
			state.LastSuccessAt = time.Now().UTC().Format(time.RFC3339)
			state.LastError = ""
			state.ConsecutiveFailures = 0
		})
	}
}

func persistBrokerCloudBackupPlan(plan brokerCloudBackupPlan) error {
	sink, err := backup.Open(nil, plan.settings)
	if err != nil {
		return err
	}
	if sink == nil {
		return nil
	}
	defer sink.Close()

	if plan.deleteCurrent {
		if err := sink.Delete(nil, plan.settings.ObjectKey("team/broker-state.json")); err != nil && !backup.IsNotFound(err) {
			return err
		}
	}
	if plan.deleteSnapshot {
		if err := sink.Delete(nil, plan.settings.ObjectKey("team/broker-state.last-good.json")); err != nil && !backup.IsNotFound(err) {
			return err
		}
	}
	if plan.writeCurrent {
		if err := sink.Put(nil, plan.settings.ObjectKey("team/broker-state.json"), plan.data, "application/json"); err != nil {
			return err
		}
	}
	if plan.writeSnapshot {
		if err := sink.Put(nil, plan.settings.ObjectKey("team/broker-state.last-good.json"), plan.data, "application/json"); err != nil {
			return err
		}
	}
	if plan.writeHistory && strings.TrimSpace(plan.historyName) != "" {
		if err := sink.Put(nil, plan.settings.ObjectKey(filepath.ToSlash(filepath.Join("team", "history", plan.historyName))), plan.data, "application/json"); err != nil {
			return err
		}
	}
	return nil
}

func defaultOfficeMembers() []officeMember {
	now := time.Now().UTC().Format(time.RFC3339)
	manifest, ok := runtimeManifestDefaults()
	if !ok {
		manifest = company.DefaultManifest()
	}
	members := make([]officeMember, 0, len(manifest.Members))
	for _, cfg := range manifest.Members {
		builtIn := cfg.System || cfg.Slug == manifest.Lead || cfg.Slug == "ceo"
		members = append(members, memberFromSpec(cfg, "wuphf", now, builtIn))
	}
	return members
}

func defaultOfficeMemberSlugs() []string {
	members := defaultOfficeMembers()
	slugs := make([]string, 0, len(members))
	for _, member := range members {
		slugs = append(slugs, member.Slug)
	}
	return slugs
}

func defaultTeamChannels() []teamChannel {
	now := time.Now().UTC().Format(time.RFC3339)
	manifest, ok := runtimeManifestDefaults()
	if !ok {
		manifest = company.DefaultManifest()
	}
	channels := make([]teamChannel, 0, len(manifest.Channels))
	for _, channel := range manifest.Channels {
		tc := teamChannel{
			Slug:        channel.Slug,
			Name:        channel.Name,
			Description: channel.Description,
			Members:     append([]string(nil), channel.Members...),
			Disabled:    append([]string(nil), channel.Disabled...),
			CreatedBy:   "wuphf",
			CreatedAt:   now,
			UpdatedAt:   now,
			Protected:   true,
		}
		if channel.Surface != nil {
			tc.Surface = &channelSurface{
				Provider:    channel.Surface.Provider,
				RemoteID:    channel.Surface.RemoteID,
				RemoteTitle: channel.Surface.RemoteTitle,
				Mode:        channel.Surface.Mode,
				BotTokenEnv: channel.Surface.BotTokenEnv,
			}
		}
		channels = append(channels, tc)
	}
	return channels
}

func repoRootForRuntimeDefaults() string {
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "."
}

func runtimeManifestDefaults() (company.Manifest, bool) {
	manifest, err := company.LoadRuntimeManifest(repoRootForRuntimeDefaults())
	if err != nil {
		return company.Manifest{}, false
	}
	if len(manifest.Members) == 0 || len(manifest.Channels) == 0 {
		return company.Manifest{}, false
	}
	return manifest, true
}

func isDefaultChannelState(channels []teamChannel) bool {
	defaults := defaultTeamChannels()
	if len(channels) != len(defaults) {
		return false
	}
	for i := range defaults {
		if channels[i].Slug != defaults[i].Slug || channels[i].Name != defaults[i].Name || channels[i].Description != defaults[i].Description {
			return false
		}
		if strings.Join(channels[i].Members, ",") != strings.Join(defaults[i].Members, ",") {
			return false
		}
		if strings.Join(channels[i].Disabled, ",") != strings.Join(defaults[i].Disabled, ",") {
			return false
		}
	}
	return true
}

func isDefaultOfficeMemberState(members []officeMember) bool {
	defaults := defaultOfficeMembers()
	if len(members) != len(defaults) {
		return false
	}
	for i := range defaults {
		if members[i].Slug != defaults[i].Slug || members[i].Name != defaults[i].Name || members[i].Role != defaults[i].Role {
			return false
		}
		if strings.TrimSpace(members[i].Personality) != strings.TrimSpace(defaults[i].Personality) {
			return false
		}
		if strings.TrimSpace(members[i].PermissionMode) != strings.TrimSpace(defaults[i].PermissionMode) {
			return false
		}
		if members[i].Provider != defaults[i].Provider {
			return false
		}
		if !sameNormalizedStringSlice(members[i].Expertise, defaults[i].Expertise) {
			return false
		}
		if !sameNormalizedStringSlice(members[i].AllowedTools, defaults[i].AllowedTools) {
			return false
		}
	}
	return true
}

func sameNormalizedStringSlice(left, right []string) bool {
	left = normalizeStringList(left)
	right = normalizeStringList(right)
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func normalizeChannelSlug(slug string) string {
	slug = strings.ToLower(strings.TrimSpace(slug))
	slug = strings.TrimLeft(slug, "#")
	slug = strings.ReplaceAll(slug, " ", "-")
	// Preserve "__" (DM slug separator) before replacing single underscores.
	const placeholder = "\x00"
	slug = strings.ReplaceAll(slug, "__", placeholder)
	slug = strings.ReplaceAll(slug, "_", "-")
	slug = strings.ReplaceAll(slug, placeholder, "__")
	if slug == "" {
		return "general"
	}
	return slug
}

const globalSkillChannel = "*"

func isGlobalSkillChannel(channel string) bool {
	return strings.TrimSpace(channel) == globalSkillChannel
}

func skillVisibleInChannel(skillChannel, requestedChannel string) bool {
	if isGlobalSkillChannel(skillChannel) {
		return true
	}
	if requestedChannel == "" {
		return true
	}
	return normalizeChannelSlug(skillChannel) == requestedChannel
}

func skillMutationChannel(channel string) string {
	if isGlobalSkillChannel(channel) {
		return "general"
	}
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		return "general"
	}
	return channel
}

func normalizeActorSlug(slug string) string {
	slug = strings.ToLower(strings.TrimSpace(slug))
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")
	return slug
}

func (b *Broker) ensureDefaultChannelsLocked() {
	if len(b.channels) == 0 {
		b.channels = defaultTeamChannels()
		return
	}
	hasGeneral := false
	for _, ch := range b.channels {
		if ch.Slug == "general" {
			hasGeneral = true
			break
		}
	}
	if !hasGeneral {
		b.channels = append(defaultTeamChannels(), b.channels...)
		return
	}
	// Merge surface metadata from manifest into existing channels
	// (handles case where state was saved without surfaces by an older binary)
	defaults := defaultTeamChannels()
	for _, def := range defaults {
		if def.Surface == nil {
			continue
		}
		found := false
		for i := range b.channels {
			if b.channels[i].Slug == def.Slug {
				if b.channels[i].Surface == nil {
					b.channels[i].Surface = def.Surface
				}
				found = true
				break
			}
		}
		if !found {
			b.channels = append(b.channels, def)
		}
	}
}

func (b *Broker) ensureDefaultOfficeMembersLocked() {
	if len(b.members) == 0 {
		b.members = defaultOfficeMembers()
		return
	}
	defaults := defaultOfficeMembers()
	for _, member := range defaults {
		if b.findMemberLocked(member.Slug) == nil {
			b.members = append(b.members, member)
		}
	}
}

func (b *Broker) normalizeLoadedStateLocked() {
	b.sessionMode = NormalizeSessionMode(b.sessionMode)
	b.oneOnOneAgent = NormalizeOneOnOneAgent(b.oneOnOneAgent)
	if b.findMemberLocked(b.oneOnOneAgent) == nil {
		b.oneOnOneAgent = DefaultOneOnOneAgent
	}
	seenMembers := make(map[string]struct{}, len(b.members))
	normalizedMembers := make([]officeMember, 0, len(b.members))
	for _, member := range b.members {
		member.Slug = normalizeChannelSlug(member.Slug)
		if member.Slug == "" {
			continue
		}
		if _, ok := seenMembers[member.Slug]; ok {
			continue
		}
		seenMembers[member.Slug] = struct{}{}
		member.Name = strings.TrimSpace(member.Name)
		if member.Name == "" {
			member.Name = humanizeSlug(member.Slug)
		}
		member.Role = strings.TrimSpace(member.Role)
		if member.Role == "" {
			member.Role = member.Name
		}
		member.BuiltIn = member.Slug == "ceo"
		member.Expertise = normalizeStringList(member.Expertise)
		member.AllowedTools = normalizeStringList(member.AllowedTools)
		normalizedMembers = append(normalizedMembers, member)
	}
	b.members = normalizedMembers
	b.recoverChannelsFromStateLocked()
	b.channels = normalizeDuplicateChannels(b.channels)
	for i := range b.channels {
		b.channels[i].Slug = normalizeChannelSlug(b.channels[i].Slug)
		if IsDMSlug(b.channels[i].Slug) {
			b.channels[i].Type = "dm"
		}
		if strings.TrimSpace(b.channels[i].Name) == "" {
			b.channels[i].Name = b.channels[i].Slug
		}
		if b.channels[i].isDM() {
			agentSlug := DMTargetAgent(b.channels[i].Slug)
			if strings.TrimSpace(b.channels[i].Description) == "" {
				if agentSlug == "" {
					b.channels[i].Description = "Direct messages."
				} else {
					b.channels[i].Description = "Direct messages with " + agentSlug
				}
			}
			b.channels[i].Members = canonicalDMMembers(b.channels[i].Slug)
			filteredDisabled := make([]string, 0, len(b.channels[i].Disabled))
			for _, slug := range uniqueSlugs(b.channels[i].Disabled) {
				if containsString(b.channels[i].Members, slug) {
					filteredDisabled = append(filteredDisabled, slug)
				}
			}
			b.channels[i].Disabled = filteredDisabled
			continue
		}
		if !b.channels[i].Protected {
			b.channels[i].Protected = shouldProtectUserChannel(b.channels[i].Slug, b.channels[i].CreatedBy)
		}
		if strings.TrimSpace(b.channels[i].Description) == "" {
			b.channels[i].Description = defaultTeamChannelDescription(b.channels[i].Slug, b.channels[i].Name)
		}
		if b.channels[i].Slug == "general" && len(b.channels[i].Members) < len(b.members) {
			// Re-populate general channel with all office members.
			// This fixes stale state where only CEO survived a previous normalization.
			allSlugs := make([]string, 0, len(b.members))
			for _, m := range b.members {
				allSlugs = append(allSlugs, m.Slug)
			}
			b.channels[i].Members = allSlugs
		}
		filteredMembers := make([]string, 0, len(b.channels[i].Members))
		for _, slug := range uniqueSlugs(b.channels[i].Members) {
			if b.findMemberLocked(slug) != nil {
				filteredMembers = append(filteredMembers, slug)
			}
		}
		b.channels[i].Members = uniqueSlugs(append([]string{"ceo"}, filteredMembers...))
		filteredDisabled := make([]string, 0, len(b.channels[i].Disabled))
		for _, slug := range uniqueSlugs(b.channels[i].Disabled) {
			if slug == "ceo" {
				continue
			}
			if b.findMemberLocked(slug) != nil && containsString(b.channels[i].Members, slug) {
				filteredDisabled = append(filteredDisabled, slug)
			}
		}
		b.channels[i].Disabled = filteredDisabled
	}
	messageCountBeforeRecover := len(b.messages)
	b.recoverMessagesFromChannelMemoryLocked()
	messagesMutated := len(b.messages) != messageCountBeforeRecover
	for i := range b.messages {
		if strings.TrimSpace(b.messages[i].Channel) == "" {
			b.messages[i].Channel = "general"
			messagesMutated = true
		}
	}
	b.normalizeExecutionNodesLocked()
	b.normalizeActionIDsLocked()
	b.recoverTasksFromChannelMemoryLocked()
	for i := range b.tasks {
		if strings.TrimSpace(b.tasks[i].Channel) == "" {
			b.tasks[i].Channel = "general"
		}
	}
	for i := range b.requests {
		if strings.TrimSpace(b.requests[i].Channel) == "" {
			b.requests[i].Channel = "general"
		}
		b.requests[i] = normalizeRequestRecord(b.requests[i])
		if strings.TrimSpace(b.requests[i].UpdatedAt) == "" {
			b.requests[i].UpdatedAt = b.requests[i].CreatedAt
		}
		b.scheduleRequestLifecycleLocked(&b.requests[i])
	}
	for i := range b.watchdogs {
		b.watchdogs[i].Status = normalizeWatchdogStatus(b.watchdogs[i].Status)
		if b.watchdogs[i].Status == "" {
			b.watchdogs[i].Status = "active"
		}
		if strings.TrimSpace(b.watchdogs[i].UpdatedAt) == "" {
			b.watchdogs[i].UpdatedAt = b.watchdogs[i].CreatedAt
		}
	}
	for i := range b.scheduler {
		b.scheduler[i] = normalizeSchedulerJob(b.scheduler[i])
	}
	b.suppressActiveFollowUpTasksWhenAuditCanceledLocked(time.Now().UTC().Format(time.RFC3339))
	for i := range b.tasks {
		if strings.TrimSpace(b.tasks[i].Channel) == "" {
			b.tasks[i].Channel = "general"
		}
		normalizeTaskPlan(&b.tasks[i])
		b.ensureTaskOwnerChannelMembershipLocked(b.tasks[i].Channel, b.tasks[i].Owner)
		b.queueTaskBehindActiveOwnerLaneLocked(&b.tasks[i])
		b.scheduleTaskLifecycleLocked(&b.tasks[i])
		_ = b.syncTaskWorktreeLocked(&b.tasks[i])
	}
	b.resolveOrphanedTaskArtifactsLocked(time.Now().UTC().Format(time.RFC3339))
	b.reconcileChannelMessageNotesLocked()
	b.pendingInterview = firstBlockingRequest(b.requests)
	if messagesMutated {
		b.rebuildMessageIndexesLocked()
	} else {
		b.ensureMessageIndexesLocked()
	}
}

func (b *Broker) reconcileStateToManifestLocked(manifest company.Manifest) bool {
	now := time.Now().UTC().Format(time.RFC3339)
	existingMembers := make(map[string]officeMember, len(b.members))
	existingMemberOrder := make([]string, 0, len(b.members))
	for _, member := range b.members {
		slug := normalizeChannelSlug(member.Slug)
		if slug == "" {
			continue
		}
		if _, seen := existingMembers[slug]; !seen {
			existingMemberOrder = append(existingMemberOrder, slug)
		}
		existingMembers[slug] = member
	}

	manifestMembers := make([]officeMember, 0, len(manifest.Members)+len(existingMembers))
	validMembers := make(map[string]struct{}, len(manifest.Members)+len(existingMembers))
	for _, spec := range manifest.Members {
		slug := normalizeChannelSlug(spec.Slug)
		if slug == "" {
			continue
		}
		existing := existingMembers[slug]
		createdBy := firstNonEmpty(strings.TrimSpace(existing.CreatedBy), "wuphf")
		createdAt := firstNonEmpty(strings.TrimSpace(existing.CreatedAt), now)
		builtIn := spec.System || slug == normalizeChannelSlug(manifest.Lead) || slug == "ceo"
		member := memberFromSpec(spec, createdBy, createdAt, builtIn)
		member.Slug = slug
		if existing.Provider != (provider.ProviderBinding{}) {
			member.Provider = existing.Provider
		}
		manifestMembers = append(manifestMembers, member)
		validMembers[slug] = struct{}{}
	}
	for _, slug := range existingMemberOrder {
		if _, ok := validMembers[slug]; ok {
			continue
		}
		member := existingMembers[slug]
		member.Slug = slug
		manifestMembers = append(manifestMembers, member)
		validMembers[slug] = struct{}{}
	}
	b.members = manifestMembers

	existingChannels := normalizeDuplicateChannels(b.channels)
	existingBySlug := make(map[string]teamChannel, len(existingChannels))
	dmChannels := make([]teamChannel, 0, len(existingChannels))
	for _, ch := range existingChannels {
		slug := normalizeChannelSlug(ch.Slug)
		if slug == "" {
			continue
		}
		if IsDMSlug(slug) {
			target := normalizeChannelSlug(DMTargetAgent(slug))
			if _, ok := validMembers[target]; ok {
				dmChannels = append(dmChannels, ch)
			}
			continue
		}
		existingBySlug[slug] = ch
	}

	manifestChannels := make([]teamChannel, 0, len(manifest.Channels)+len(existingChannels))
	validChannels := make(map[string]struct{}, len(manifest.Channels)+len(existingChannels))
	for _, spec := range manifest.Channels {
		slug := normalizeChannelSlug(spec.Slug)
		if slug == "" {
			continue
		}
		existing := existingBySlug[slug]
		ch := teamChannel{
			Slug:        slug,
			Name:        firstNonEmpty(strings.TrimSpace(spec.Name), strings.TrimSpace(existing.Name), slug),
			Type:        strings.TrimSpace(existing.Type),
			Description: firstNonEmpty(strings.TrimSpace(spec.Description), strings.TrimSpace(existing.Description)),
			Members:     uniqueSlugs(spec.Members),
			Disabled:    uniqueSlugs(spec.Disabled),
			LinkedRepos: append([]linkedRepoRef(nil), existing.LinkedRepos...),
			CreatedBy:   firstNonEmpty(strings.TrimSpace(existing.CreatedBy), "wuphf"),
			CreatedAt:   firstNonEmpty(strings.TrimSpace(existing.CreatedAt), now),
			UpdatedAt:   firstNonEmpty(strings.TrimSpace(existing.UpdatedAt), now),
			Protected:   true,
		}
		if ch.Description == "" {
			ch.Description = defaultTeamChannelDescription(ch.Slug, ch.Name)
		}
		if spec.Surface != nil {
			ch.Surface = &channelSurface{
				Provider:    spec.Surface.Provider,
				RemoteID:    spec.Surface.RemoteID,
				RemoteTitle: spec.Surface.RemoteTitle,
				Mode:        spec.Surface.Mode,
				BotTokenEnv: spec.Surface.BotTokenEnv,
			}
		} else if existing.Surface != nil {
			cp := *existing.Surface
			ch.Surface = &cp
		}
		manifestChannels = append(manifestChannels, ch)
		validChannels[slug] = struct{}{}
	}
	for _, ch := range existingChannels {
		slug := normalizeChannelSlug(ch.Slug)
		if slug == "" || IsDMSlug(slug) {
			continue
		}
		if _, ok := validChannels[slug]; ok {
			continue
		}
		validChannels[slug] = struct{}{}
		manifestChannels = append(manifestChannels, ch)
	}
	for _, ch := range dmChannels {
		slug := normalizeChannelSlug(ch.Slug)
		validChannels[slug] = struct{}{}
		manifestChannels = append(manifestChannels, ch)
	}
	startupGuardTriggered := b.persistStartupReconcileGuardLocked(validChannels)
	b.channels = manifestChannels
	b.reconcileRecordsToManifestLocked(validMembers, validChannels)
	return startupGuardTriggered
}

func (b *Broker) reconcileRecordsToManifestLocked(validMembers, validChannels map[string]struct{}) {
	if b == nil {
		return
	}
	memberAllowed := func(slug string) bool {
		slug = normalizeChannelSlug(slug)
		if slug == "" {
			return true
		}
		switch slug {
		case "you", "human", "nex", "system":
			return true
		default:
			_, ok := validMembers[slug]
			return ok
		}
	}
	channelAllowed := func(slug string) bool {
		slug = normalizeChannelSlug(slug)
		if slug == "" {
			return true
		}
		_, ok := validChannels[slug]
		return ok
	}

	filteredMessages := make([]channelMessage, 0, len(b.messages))
	for _, msg := range b.messages {
		if !channelAllowed(msg.Channel) || !memberAllowed(msg.From) {
			continue
		}
		msg.Tagged = filterSlugsWithPolicy(msg.Tagged, memberAllowed)
		filteredMessages = append(filteredMessages, msg)
	}
	b.replaceMessagesLocked(filteredMessages)

	filteredTasks := make([]teamTask, 0, len(b.tasks))
	for _, task := range b.tasks {
		if !channelAllowed(task.Channel) || !memberAllowed(task.CreatedBy) || (strings.TrimSpace(task.Owner) != "" && !memberAllowed(task.Owner)) {
			continue
		}
		filteredTasks = append(filteredTasks, task)
	}
	b.tasks = filteredTasks

	filteredRequests := make([]humanInterview, 0, len(b.requests))
	for _, req := range b.requests {
		if !channelAllowed(req.Channel) || !memberAllowed(req.From) {
			continue
		}
		filteredRequests = append(filteredRequests, req)
	}
	b.requests = filteredRequests

	filteredActions := make([]officeActionLog, 0, len(b.actions))
	for _, action := range b.actions {
		if !channelAllowed(action.Channel) || !memberAllowed(action.Actor) {
			continue
		}
		filteredActions = append(filteredActions, action)
	}
	b.actions = filteredActions

	filteredNodes := make([]executionNode, 0, len(b.executionNodes))
	for _, node := range b.executionNodes {
		if !channelAllowed(node.Channel) || !memberAllowed(node.OwnerAgent) {
			continue
		}
		node.ExpectedFrom = filterSlugsWithPolicy(node.ExpectedFrom, memberAllowed)
		if len(node.ExpectedFrom) == 0 && strings.TrimSpace(node.OwnerAgent) != "" {
			node.ExpectedFrom = []string{node.OwnerAgent}
		}
		filteredNodes = append(filteredNodes, node)
	}
	b.executionNodes = filteredNodes
}

func filterSlugsWithPolicy(items []string, allow func(string) bool) []string {
	out := make([]string, 0, len(items))
	for _, item := range uniqueSlugs(items) {
		if allow(item) {
			out = append(out, item)
		}
	}
	return out
}

func normalizeDuplicateChannels(channels []teamChannel) []teamChannel {
	if len(channels) <= 1 {
		return channels
	}
	seen := make(map[string]int, len(channels))
	normalized := make([]teamChannel, 0, len(channels))
	for _, ch := range channels {
		ch.Slug = normalizeChannelSlug(ch.Slug)
		if ch.Slug == "" {
			continue
		}
		if idx, ok := seen[ch.Slug]; ok {
			mergeChannelState(&normalized[idx], ch)
			continue
		}
		seen[ch.Slug] = len(normalized)
		normalized = append(normalized, ch)
	}
	return normalized
}

func mergeChannelState(dst *teamChannel, src teamChannel) {
	if dst == nil {
		return
	}
	src.Slug = normalizeChannelSlug(src.Slug)
	if dst.Slug == "" {
		dst.Slug = src.Slug
	}
	if strings.TrimSpace(src.Name) != "" {
		if strings.TrimSpace(dst.Name) == "" || parseBrokerTimestamp(src.UpdatedAt).After(parseBrokerTimestamp(dst.UpdatedAt)) {
			dst.Name = strings.TrimSpace(src.Name)
		}
	}
	if strings.TrimSpace(src.Description) != "" {
		if strings.TrimSpace(dst.Description) == "" || parseBrokerTimestamp(src.UpdatedAt).After(parseBrokerTimestamp(dst.UpdatedAt)) {
			dst.Description = strings.TrimSpace(src.Description)
		}
	}
	if strings.TrimSpace(dst.Type) == "" && strings.TrimSpace(src.Type) != "" {
		dst.Type = strings.TrimSpace(src.Type)
	}
	if src.Protected {
		dst.Protected = true
	}
	dst.Members = uniqueSlugs(append(dst.Members, src.Members...))
	dst.Disabled = uniqueSlugs(append(dst.Disabled, src.Disabled...))
	if dst.Surface == nil && src.Surface != nil {
		cp := *src.Surface
		dst.Surface = &cp
	}
	if strings.TrimSpace(dst.CreatedBy) == "" && strings.TrimSpace(src.CreatedBy) != "" {
		dst.CreatedBy = strings.TrimSpace(src.CreatedBy)
	}
	dstCreatedAt := parseBrokerTimestamp(dst.CreatedAt)
	srcCreatedAt := parseBrokerTimestamp(src.CreatedAt)
	switch {
	case dstCreatedAt.IsZero() && !srcCreatedAt.IsZero():
		dst.CreatedAt = src.CreatedAt
	case !srcCreatedAt.IsZero() && srcCreatedAt.Before(dstCreatedAt):
		dst.CreatedAt = src.CreatedAt
	}
	dstUpdatedAt := parseBrokerTimestamp(dst.UpdatedAt)
	srcUpdatedAt := parseBrokerTimestamp(src.UpdatedAt)
	switch {
	case dstUpdatedAt.IsZero() && !srcUpdatedAt.IsZero():
		dst.UpdatedAt = src.UpdatedAt
	case !srcUpdatedAt.IsZero() && srcUpdatedAt.After(dstUpdatedAt):
		dst.UpdatedAt = src.UpdatedAt
	}
}

func (b *Broker) normalizeActionIDsLocked() {
	if len(b.actions) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(b.actions))
	maxOrdinal := 0
	for _, action := range b.actions {
		if n, ok := parseSequentialRecordOrdinal(action.ID, "action-"); ok && n > maxOrdinal {
			maxOrdinal = n
		}
	}
	nextOrdinal := maxOrdinal + 1
	if nextOrdinal < 1 {
		nextOrdinal = 1
	}
	for i := range b.actions {
		id := strings.TrimSpace(b.actions[i].ID)
		_, ok := parseSequentialRecordOrdinal(id, "action-")
		if id == "" || !ok {
			b.actions[i].ID = fmt.Sprintf("action-%d", nextOrdinal)
			nextOrdinal++
			continue
		}
		if _, exists := seen[id]; exists {
			b.actions[i].ID = fmt.Sprintf("action-%d", nextOrdinal)
			nextOrdinal++
			continue
		}
		seen[id] = struct{}{}
	}
}

func (b *Broker) SessionModeState() (string, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sessionMode, b.oneOnOneAgent
}

func (b *Broker) SetSessionMode(mode, agent string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sessionMode = NormalizeSessionMode(mode)
	b.oneOnOneAgent = NormalizeOneOnOneAgent(agent)
	if b.findMemberLocked(b.oneOnOneAgent) == nil {
		b.oneOnOneAgent = DefaultOneOnOneAgent
	}
	return b.saveLocked()
}

func (b *Broker) SetFocusMode(enabled bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.focusMode = enabled
	return b.saveLocked()
}

func (b *Broker) SetGenerateMemberFn(fn func(string) (generatedMemberTemplate, error)) {
	b.generateMemberFn = fn
}

func (b *Broker) SetGenerateChannelFn(fn func(string) (generatedChannelTemplate, error)) {
	b.generateChannelFn = fn
}

// SetAgentLogRoot overrides where /agent-logs reads task JSONL from.
// Used by tests; production uses agent.DefaultTaskLogRoot().
func (b *Broker) SetAgentLogRoot(root string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.agentLogRoot = root
}

func (b *Broker) FocusModeEnabled() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.focusMode
}

func (b *Broker) findChannelLocked(slug string) *teamChannel {
	slug = normalizeChannelSlug(slug)
	for i := range b.channels {
		if b.channels[i].Slug == slug {
			return &b.channels[i]
		}
	}
	return nil
}

func (b *Broker) channelLinkedRepoPaths(channel string) []string {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := b.findChannelLocked(channel)
	if ch == nil || len(ch.LinkedRepos) == 0 {
		return nil
	}
	paths := make([]string, 0, len(ch.LinkedRepos))
	if primary := strings.TrimSpace(ch.primaryLinkedRepoPath()); primary != "" {
		paths = append(paths, primary)
	}
	for _, repo := range ch.LinkedRepos {
		path := strings.TrimSpace(repo.RepoPath)
		if path == "" || (len(paths) > 0 && sameCleanPath(path, paths[0])) {
			continue
		}
		paths = append(paths, path)
	}
	return compactStringList(paths)
}

func (b *Broker) channelPrimaryLinkedRepoPath(channel string) string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := b.findChannelLocked(channel)
	if ch == nil {
		return ""
	}
	return strings.TrimSpace(ch.primaryLinkedRepoPath())
}

// ensureDMConversationLocked returns the DM conversation for the given slug,
// creating it on the fly if it doesn't exist. Mirrors Slack's conversations.open.
// It delegates creation to channelStore so DM channels have proper types and members.
func (b *Broker) ensureDMConversationLocked(slug string) *teamChannel {
	if ch := b.findChannelLocked(slug); ch != nil {
		return ch
	}
	if !IsDMSlug(slug) {
		return nil
	}
	agentSlug := DMTargetAgent(slug)
	now := time.Now().UTC().Format(time.RFC3339)
	// Register in channelStore for proper type-based DM detection.
	if b.channelStore != nil {
		newSlug := channel.DirectSlug("human", agentSlug)
		if _, err := b.channelStore.GetOrCreateDirect("human", agentSlug); err == nil {
			// Update slug in broker to the new deterministic format if different.
			if newSlug != slug {
				slug = newSlug
			}
		}
	}
	b.channels = append(b.channels, teamChannel{
		Slug:        slug,
		Name:        slug,
		Type:        "dm",
		Description: "Direct messages with " + agentSlug,
		Members:     canonicalDMMembers(slug),
		CreatedBy:   "wuphf",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	return &b.channels[len(b.channels)-1]
}

func (b *Broker) findMemberLocked(slug string) *officeMember {
	slug = normalizeChannelSlug(slug)
	if len(b.memberIndex) != len(b.members) {
		b.rebuildMemberIndexLocked()
	}
	if i, ok := b.memberIndex[slug]; ok && i < len(b.members) && b.members[i].Slug == slug {
		return &b.members[i]
	}
	return nil
}

// rebuildMemberIndexLocked rebuilds memberIndex from b.members. Callers must
// hold b.mu. Called on load and after any structural mutation (remove, reorder)
// to keep the map in sync with the slice. Appends and in-place updates are
// handled by findMemberLocked's length-check lazy rebuild.
func (b *Broker) rebuildMemberIndexLocked() {
	b.memberIndex = make(map[string]int, len(b.members))
	for i, m := range b.members {
		b.memberIndex[m.Slug] = i
	}
}

// SetMemberProvider attaches or replaces the ProviderBinding on the given
// office member and persists broker state. Returns an error if the member
// doesn't exist; callers should ensure the member exists first.
func (b *Broker) SetMemberProvider(slug string, binding provider.ProviderBinding) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	m := b.findMemberLocked(slug)
	if m == nil {
		return fmt.Errorf("set member provider: unknown slug %q", slug)
	}
	m.Provider = binding
	return b.saveLocked()
}

// MemberProviderBinding returns the per-agent provider binding for slug, or
// the zero value if the member does not exist. Safe to call from outside the
// broker; takes the mutex internally.
func (b *Broker) MemberProviderBinding(slug string) provider.ProviderBinding {
	b.mu.Lock()
	defer b.mu.Unlock()
	m := b.findMemberLocked(slug)
	if m == nil {
		return provider.ProviderBinding{}
	}
	return m.Provider
}

func normalizeProviderBinding(binding provider.ProviderBinding) provider.ProviderBinding {
	binding.Kind = strings.TrimSpace(binding.Kind)
	binding.Model = strings.TrimSpace(binding.Model)
	return binding
}

func providerBindingChanged(before, after provider.ProviderBinding) bool {
	return normalizeProviderBinding(before) != normalizeProviderBinding(after)
}

func persistManifestMemberProvider(slug string, binding provider.ProviderBinding) error {
	slug = normalizeChannelSlug(slug)
	if slug == "" {
		return nil
	}
	manifest, err := company.LoadManifest()
	if err != nil {
		return err
	}
	updated := false
	binding = normalizeProviderBinding(binding)
	for i := range manifest.Members {
		if normalizeChannelSlug(manifest.Members[i].Slug) != slug {
			continue
		}
		manifest.Members[i].Provider = binding
		updated = true
		break
	}
	if !updated {
		return nil
	}
	return company.SaveManifest(manifest)
}

func providerChangeLooksOperational(details string) bool {
	if isAutomaticRecoveryBlock(details) {
		return true
	}
	if _, ok := detectAgentOperationalIssue(details); ok {
		return true
	}
	detail := strings.ToLower(strings.TrimSpace(details))
	return strings.Contains(detail, "provider cooldown") ||
		strings.Contains(detail, "resource_exhausted") ||
		strings.Contains(detail, "rate limit") ||
		strings.Contains(detail, "retry after")
}

func (b *Broker) shouldResumeTaskAfterProviderChangeLocked(task *teamTask, owner string) bool {
	if task == nil {
		return false
	}
	if strings.TrimSpace(task.Owner) != strings.TrimSpace(owner) {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(task.Status))
	if status != "blocked" && !task.Blocked {
		return false
	}
	if providerChangeLooksOperational(task.Details) {
		return true
	}
	node := b.findExecutionNodeForTaskLocked(task, owner)
	if node == nil {
		return false
	}
	switch normalizeExecutionNodeStatus(node.Status) {
	case "timed_out", "fallback_dispatched", "blocked":
		return true
	default:
		return providerChangeLooksOperational(node.LastError)
	}
}

func (b *Broker) clearOperationalBlocksForProviderChangeLocked(slug, actor string) error {
	if b == nil {
		return nil
	}
	slug = normalizeChannelSlug(slug)
	if slug == "" {
		return nil
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "studio"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for i := range b.tasks {
		task := &b.tasks[i]
		if !b.shouldResumeTaskAfterProviderChangeLocked(task, slug) {
			continue
		}
		if _, err := b.resumeTaskLocked(task, actor, "Agent runtime changed; clearing operational block for retry.", now); err != nil {
			return err
		}
	}
	b.resolveAgentRuntimeAlertsLocked(slug)
	return nil
}

// MemberProviderKind returns the effective runtime kind for the given slug,
// falling back to the global runtime when the member has no explicit binding.
// Used by the launcher's dispatch switch so each agent can run on its own
// provider (e.g., one Codex agent + one Claude Code agent in the same team).
func (b *Broker) MemberProviderKind(slug string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	m := b.findMemberLocked(slug)
	if m == nil {
		return ""
	}
	return m.Provider.Kind
}

// memberFromSpec builds an officeMember from a manifest MemberSpec, threading
// Provider through. Used by defaultOfficeMembers and by HTTP create paths so
// field-copy logic lives in one place.
func memberFromSpec(spec company.MemberSpec, createdBy, createdAt string, builtIn bool) officeMember {
	return officeMember{
		Slug:           spec.Slug,
		Name:           spec.Name,
		Role:           spec.Role,
		Expertise:      append([]string(nil), spec.Expertise...),
		Personality:    spec.Personality,
		PermissionMode: spec.PermissionMode,
		AllowedTools:   append([]string(nil), spec.AllowedTools...),
		CreatedBy:      createdBy,
		CreatedAt:      createdAt,
		BuiltIn:        builtIn,
		Provider:       spec.Provider,
	}
}

func uniqueSlugs(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = normalizeChannelSlug(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func normalizeStringList(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func requestIsActive(req humanInterview) bool {
	req = normalizeRequestRecord(req)
	return !requestStatusIsTerminal(req.Status)
}

func defaultRequestSLAHours() int {
	minutes := config.ResolveTaskFollowUpInterval()
	if minutes < config.ResolveTaskReminderInterval()*2 {
		minutes = config.ResolveTaskReminderInterval() * 2
	}
	if minutes < 60 {
		minutes = 60
	}
	hours := minutes / 60
	if minutes%60 != 0 {
		hours++
	}
	if hours < 1 {
		hours = 1
	}
	return hours
}

func normalizeRequestEscalationState(value string, count int) string {
	if count >= 2 {
		return "critical"
	}
	if count >= 1 {
		return "escalated"
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical":
		return "critical"
	case "escalated":
		return "escalated"
	default:
		return "watching"
	}
}

func requestNeedsHumanDecision(req humanInterview) bool {
	switch strings.TrimSpace(req.Kind) {
	case "interview", "approval", "confirm", "choice", "epistemic_check":
		return true
	default:
		return req.Required
	}
}

func requestOptionDefaults(kind string) ([]interviewOption, string) {
	switch normalizeRequestKind(kind) {
	case "approval":
		return []interviewOption{
			{ID: "approve", Label: "Approve", Description: "Green-light this and let the team execute immediately."},
			{ID: "approve_with_note", Label: "Approve with note", Description: "Proceed, but attach explicit constraints or guardrails.", RequiresText: true, TextHint: "Type the conditions, constraints, or guardrails the team must follow."},
			{ID: "needs_more_info", Label: "Need more info", Description: "Gather more context before making the approval call."},
			{ID: "reject", Label: "Reject", Description: "Do not proceed with this."},
			{ID: "reject_with_steer", Label: "Reject with steer", Description: "Do not proceed as proposed. Redirect the team with clearer steering.", RequiresText: true, TextHint: "Type the steering, redirect, or rationale for rejecting this request."},
		}, "approve"
	case "confirm":
		return []interviewOption{
			{ID: "confirm_proceed", Label: "Confirm", Description: "Looks good. Proceed as planned."},
			{ID: "adjust", Label: "Adjust", Description: "Proceed only after applying the changes you specify.", RequiresText: true, TextHint: "Type the changes that must happen before proceeding."},
			{ID: "reassign", Label: "Reassign", Description: "Move this to a different owner or scope.", RequiresText: true, TextHint: "Type who should own this instead, or how the scope should change."},
			{ID: "hold", Label: "Hold", Description: "Do not act yet. Keep this pending for review."},
		}, "confirm_proceed"
	case "choice":
		return []interviewOption{
			{ID: "move_fast", Label: "Move fast", Description: "Bias toward speed. Ship now and iterate later."},
			{ID: "balanced", Label: "Balanced", Description: "Balance speed, risk, and quality."},
			{ID: "be_careful", Label: "Be careful", Description: "Bias toward caution and a tighter review loop."},
			{ID: "needs_more_info", Label: "Need more info", Description: "Gather more context before deciding.", RequiresText: true, TextHint: "Type what is missing or what should be investigated next."},
			{ID: "delegate", Label: "Delegate", Description: "Hand this to a specific owner for a closer call.", RequiresText: true, TextHint: "Type who should own this decision and any guidance for them."},
		}, "balanced"
	case "epistemic_check":
		return []interviewOption{
			{ID: "facts_verified", Label: "Facts verified", Description: "The observed facts and cited evidence are sufficient to proceed."},
			{ID: "safe_inference", Label: "Safe inference", Description: "Proceed, but treat part of this as inference and keep the next step narrow.", RequiresText: true, TextHint: "Type the inference being accepted and the narrow next step that remains safe."},
			{ID: "needs_more_evidence", Label: "Need more evidence", Description: "Do not proceed yet; gather the missing evidence first.", RequiresText: true, TextHint: "Type the missing evidence, file, command, or external check that must happen next."},
			{ID: "do_not_proceed", Label: "Do not proceed", Description: "The uncertainty is too high or the risk is unacceptable.", RequiresText: true, TextHint: "Type why this should stop here and what the safer fallback is."},
		}, "facts_verified"
	case "interview":
		return []interviewOption{
			{ID: "answer_directly", Label: "Answer directly", Description: "Respond in your own words below."},
			{ID: "need_more_context", Label: "Need more context", Description: "Ask the office to bring back more context before you decide.", RequiresText: true, TextHint: "Type what context is missing or what should be clarified next."},
		}, "answer_directly"
	case "freeform", "secret":
		return []interviewOption{
			{ID: "proceed", Label: "Proceed", Description: "Let the team handle it with their best judgment."},
			{ID: "give_direction", Label: "Give direction", Description: "Proceed, but only after you provide specific guidance.", RequiresText: true, TextHint: "Type the direction or constraints the team should follow."},
			{ID: "delegate", Label: "Delegate", Description: "Route this to a specific person.", RequiresText: true, TextHint: "Type who should own this and what they should do."},
			{ID: "hold", Label: "Hold", Description: "Pause until you review this further."},
		}, "proceed"
	default:
		return []interviewOption{
			{ID: "proceed", Label: "Proceed", Description: "Let the team handle it with their best judgment."},
			{ID: "give_direction", Label: "Give direction", Description: "Add specific guidance the team should follow.", RequiresText: true, TextHint: "Provide the direction or constraints the team should follow."},
			{ID: "delegate", Label: "Delegate", Description: "Route this to a specific person or role.", RequiresText: true, TextHint: "Name the person or role that should own the next call."},
			{ID: "hold", Label: "Hold", Description: "Pause until you review this further."},
		}, "proceed"
	}
}

func enrichRequestOptions(kind string, options []interviewOption) []interviewOption {
	if len(options) == 0 {
		defaults, _ := requestOptionDefaults(kind)
		return defaults
	}
	defaults, _ := requestOptionDefaults(kind)
	meta := make(map[string]interviewOption, len(defaults))
	for _, option := range defaults {
		meta[strings.TrimSpace(option.ID)] = option
	}
	out := make([]interviewOption, 0, len(options))
	for _, option := range options {
		id := strings.TrimSpace(option.ID)
		option.Label = strings.TrimSpace(option.Label)
		option.Description = strings.TrimSpace(option.Description)
		option.TextHint = strings.TrimSpace(option.TextHint)
		if id == "" && option.Label != "" {
			id = normalizeRequestOptionID(option.Label)
			option.ID = id
		}
		if base, ok := meta[id]; ok {
			if !option.RequiresText {
				option.RequiresText = base.RequiresText
			}
			if strings.TrimSpace(option.TextHint) == "" {
				option.TextHint = base.TextHint
			}
			if strings.TrimSpace(option.Label) == "" {
				option.Label = base.Label
			}
			if strings.TrimSpace(option.Description) == "" {
				option.Description = base.Description
			}
		}
		out = append(out, option)
	}
	return out
}

func normalizeRequestOptions(kind, recommendedID string, options []interviewOption) ([]interviewOption, string) {
	normalized := enrichRequestOptions(kind, options)
	recommendedID = strings.TrimSpace(recommendedID)
	if recommendedID != "" {
		for _, option := range normalized {
			if strings.TrimSpace(option.ID) == recommendedID {
				return normalized, recommendedID
			}
		}
	}
	_, fallback := requestOptionDefaults(kind)
	for _, option := range normalized {
		if strings.TrimSpace(option.ID) == fallback {
			return normalized, fallback
		}
	}
	if len(normalized) > 0 {
		return normalized, strings.TrimSpace(normalized[0].ID)
	}
	return normalized, fallback
}

func findRequestOption(req humanInterview, choiceID string) *interviewOption {
	choiceID = strings.TrimSpace(choiceID)
	if choiceID == "" {
		return nil
	}
	for i := range req.Options {
		if strings.TrimSpace(req.Options[i].ID) == choiceID {
			return &req.Options[i]
		}
	}
	return nil
}

func formatRequestAnswerMessage(req humanInterview, answer interviewAnswer) string {
	if req.Secret {
		return fmt.Sprintf("Answered @%s's request privately.", req.From)
	}
	custom := strings.TrimSpace(answer.CustomText)
	switch strings.TrimSpace(answer.ChoiceID) {
	case "approve":
		return fmt.Sprintf("Approved @%s's request.", req.From)
	case "approve_with_note":
		if custom != "" {
			return fmt.Sprintf("Approved @%s's request with note: %s", req.From, custom)
		}
		return fmt.Sprintf("Approved @%s's request with a note.", req.From)
	case "reject":
		return fmt.Sprintf("Rejected @%s's request.", req.From)
	case "reject_with_steer":
		if custom != "" {
			return fmt.Sprintf("Rejected @%s's request with steering: %s", req.From, custom)
		}
		return fmt.Sprintf("Rejected @%s's request with steering.", req.From)
	case "confirm_proceed":
		return fmt.Sprintf("Confirmed @%s's request.", req.From)
	case "adjust":
		if custom != "" {
			return fmt.Sprintf("Requested adjustments from @%s: %s", req.From, custom)
		}
		return fmt.Sprintf("Requested adjustments from @%s.", req.From)
	case "reassign":
		if custom != "" {
			return fmt.Sprintf("Reassigned @%s's request: %s", req.From, custom)
		}
		return fmt.Sprintf("Reassigned @%s's request.", req.From)
	case "hold":
		return fmt.Sprintf("Put @%s's request on hold.", req.From)
	case "delegate":
		if custom != "" {
			return fmt.Sprintf("Delegated @%s's request: %s", req.From, custom)
		}
		return fmt.Sprintf("Delegated @%s's request.", req.From)
	case "facts_verified":
		return fmt.Sprintf("Verified the evidence package for @%s's epistemic check.", req.From)
	case "safe_inference":
		if custom != "" {
			return fmt.Sprintf("Accepted a bounded inference for @%s: %s", req.From, custom)
		}
		return fmt.Sprintf("Accepted a bounded inference for @%s.", req.From)
	case "needs_more_evidence":
		if custom != "" {
			return fmt.Sprintf("Asked @%s for more evidence: %s", req.From, custom)
		}
		return fmt.Sprintf("Asked @%s for more evidence.", req.From)
	case "do_not_proceed":
		if custom != "" {
			return fmt.Sprintf("Stopped @%s's proposed action: %s", req.From, custom)
		}
		return fmt.Sprintf("Stopped @%s's proposed action.", req.From)
	case "needs_more_info":
		if custom != "" {
			return fmt.Sprintf("Asked @%s for more information: %s", req.From, custom)
		}
		return fmt.Sprintf("Asked @%s for more information.", req.From)
	}
	if custom != "" && strings.TrimSpace(answer.ChoiceText) != "" {
		return fmt.Sprintf("Answered @%s's request with %s: %s", req.From, answer.ChoiceText, custom)
	}
	if custom != "" {
		return fmt.Sprintf("Answered @%s's request: %s", req.From, custom)
	}
	if strings.TrimSpace(answer.ChoiceText) != "" {
		return fmt.Sprintf("Answered @%s's request: %s", req.From, answer.ChoiceText)
	}
	return fmt.Sprintf("Answered @%s's request.", req.From)
}

func activeRequests(requests []humanInterview) []humanInterview {
	out := make([]humanInterview, 0, len(requests))
	for _, req := range requests {
		if requestIsActive(req) {
			out = append(out, req)
		}
	}
	return out
}

func firstBlockingRequest(requests []humanInterview) *humanInterview {
	for i := range requests {
		if requestIsActive(requests[i]) && requests[i].Blocking {
			req := requests[i]
			return &req
		}
	}
	return nil
}

func normalizeRequestKind(kind string) string {
	kind = strings.TrimSpace(strings.ToLower(kind))
	if kind == "" {
		return "choice"
	}
	return kind
}

func normalizeRequestOptionID(label string) string {
	label = strings.TrimSpace(strings.ToLower(label))
	label = strings.ReplaceAll(label, "-", "_")
	label = strings.ReplaceAll(label, " ", "_")
	return label
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func humanizeSlug(slug string) string {
	parts := strings.Split(strings.ReplaceAll(strings.TrimSpace(slug), "-", " "), " ")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func defaultTeamChannelDescription(slug, name string) string {
	manifest, err := company.LoadManifest()
	if err == nil {
		for _, ch := range manifest.Channels {
			if normalizeChannelSlug(ch.Slug) == normalizeChannelSlug(slug) && strings.TrimSpace(ch.Description) != "" {
				return strings.TrimSpace(ch.Description)
			}
		}
	}
	if normalizeChannelSlug(slug) == "general" {
		return "The default company-wide room for top-level coordination, announcements, and cross-functional discussion."
	}
	label := strings.TrimSpace(name)
	if label == "" {
		label = humanizeSlug(slug)
	}
	return label + " focused work. Use this channel for discussion, decisions, and execution specific to that stream."
}

func (b *Broker) canAccessChannelLocked(slug, channel string) bool {
	slug = normalizeActorSlug(slug)
	channel = normalizeChannelSlug(channel)
	if b.sessionMode == SessionModeOneOnOne {
		if slug == "" || slug == "you" || slug == "human" {
			return true
		}
		return slug == b.oneOnOneAgent
	}
	if slug == "" || slug == "you" || slug == "human" || slug == "nex" {
		return true
	}
	if slug == "ceo" {
		return true
	}
	return b.channelHasMemberLocked(channel, slug)
}

func truncateSummary(s string, max int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= max {
		return s
	}
	runes := []rune(s)
	if max <= 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}

func applyOfficeMemberDefaults(member *officeMember) {
	if member == nil {
		return
	}
	if member.Name == "" {
		member.Name = humanizeSlug(member.Slug)
	}
	if member.Role == "" {
		member.Role = member.Name
	}
	if len(member.Expertise) == 0 {
		member.Expertise = inferOfficeExpertise(member.Slug, member.Role)
	}
	if member.Personality == "" {
		member.Personality = inferOfficePersonality(member.Slug, member.Role)
	}
	if member.PermissionMode == "" {
		member.PermissionMode = "plan"
	}
}

func inferOfficeExpertise(slug, role string) []string {
	text := strings.ToLower(strings.TrimSpace(slug + " " + role))
	switch {
	case strings.Contains(text, "front"), strings.Contains(text, "ui"), strings.Contains(text, "design eng"):
		return []string{"frontend", "UI", "interaction design", "components", "accessibility"}
	case strings.Contains(text, "back"), strings.Contains(text, "api"), strings.Contains(text, "infra"):
		return []string{"backend", "APIs", "systems", "infrastructure", "databases"}
	case strings.Contains(text, "ai"), strings.Contains(text, "ml"), strings.Contains(text, "llm"):
		return []string{"AI", "LLMs", "agents", "retrieval", "evaluations"}
	case strings.Contains(text, "market"), strings.Contains(text, "brand"), strings.Contains(text, "growth"):
		return []string{"marketing", "growth", "positioning", "campaigns", "brand"}
	case strings.Contains(text, "revenue"), strings.Contains(text, "sales"), strings.Contains(text, "cro"):
		return []string{"sales", "revenue", "pipeline", "partnerships", "closing"}
	case strings.Contains(text, "product"), strings.Contains(text, "pm"):
		return []string{"product", "roadmap", "requirements", "prioritization", "scope"}
	case strings.Contains(text, "design"):
		return []string{"design", "UX", "visual systems", "prototyping", "brand"}
	default:
		return []string{strings.ToLower(strings.TrimSpace(role))}
	}
}

func inferOfficePersonality(slug, role string) string {
	text := strings.ToLower(strings.TrimSpace(slug + " " + role))
	switch {
	case strings.Contains(text, "front"):
		return "Frontend specialist focused on polished user-facing work, clear implementation, and disciplined technical communication."
	case strings.Contains(text, "back"):
		return "Systems-minded engineer who controls complexity, protects reliability, and communicates in precise operational terms."
	case strings.Contains(text, "ai"), strings.Contains(text, "ml"), strings.Contains(text, "llm"):
		return "AI engineer focused on practical model behavior, evaluation, latency, and production reliability."
	case strings.Contains(text, "market"), strings.Contains(text, "brand"), strings.Contains(text, "growth"):
		return "Growth and positioning operator who translates product work into clear market execution without exaggeration."
	case strings.Contains(text, "revenue"), strings.Contains(text, "sales"):
		return "Commercial operator who thinks in demand, objections, and revenue consequences with direct communication."
	case strings.Contains(text, "product"), strings.Contains(text, "pm"):
		return "Product thinker who turns ambiguity into scope, sequencing, and clear tradeoffs."
	case strings.Contains(text, "design"):
		return "Designer focused on clarity, craft, usability, and direct articulation of product quality concerns."
	default:
		return "Professional teammate with clear domain ownership, direct communication, and disciplined technical judgment."
	}
}

func (b *Broker) channelHasMemberLocked(channel, slug string) bool {
	ch := b.findChannelLocked(channel)
	if ch == nil {
		// Fall back to channelStore for new-format channels (e.g. "eng__human")
		if b.channelStore != nil {
			return b.channelStore.IsMemberBySlug(channel, slug)
		}
		return false
	}
	for _, member := range ch.Members {
		if member == slug {
			return true
		}
	}
	return false
}

func (b *Broker) channelMemberEnabledLocked(channel, slug string) bool {
	if !b.channelHasMemberLocked(channel, slug) {
		return false
	}
	ch := b.findChannelLocked(channel)
	if ch == nil {
		return false
	}
	for _, disabled := range ch.Disabled {
		if disabled == slug {
			return false
		}
	}
	return true
}

func (b *Broker) enabledChannelMembersLocked(channel string, candidates []string) []string {
	var out []string
	for _, candidate := range candidates {
		if b.channelMemberEnabledLocked(channel, candidate) {
			out = append(out, candidate)
		}
	}
	return out
}

func (b *Broker) ensureTaskOwnerChannelMembershipLocked(channel, owner string) {
	channel = normalizeChannelSlug(channel)
	owner = normalizeChannelSlug(owner)
	if channel == "" || owner == "" {
		return
	}
	if b.findMemberLocked(owner) == nil {
		return
	}
	ch := b.findChannelLocked(channel)
	if ch == nil {
		return
	}
	if !containsString(ch.Members, owner) {
		ch.Members = uniqueSlugs(append(ch.Members, owner))
		ch.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if len(ch.Disabled) > 0 {
		filtered := ch.Disabled[:0]
		for _, disabled := range ch.Disabled {
			if disabled != owner {
				filtered = append(filtered, disabled)
			}
		}
		ch.Disabled = filtered
	}
}

func usageStateIsZero(state teamUsageState) bool {
	if state.Total.TotalTokens > 0 || state.Total.CostUsd > 0 || state.Total.Requests > 0 {
		return false
	}
	for _, totals := range state.Agents {
		if totals.TotalTokens > 0 || totals.CostUsd > 0 || totals.Requests > 0 {
			return false
		}
	}
	return true
}

func (b *Broker) appendActionLocked(kind, source, channel, actor, summary, relatedID string) {
	b.appendActionWithRefsLocked(kind, source, channel, actor, summary, relatedID, nil, "")
}

func (b *Broker) SetSchedulerJob(job schedulerJob) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	job = normalizeSchedulerJob(job)
	if job.Slug == "" {
		return fmt.Errorf("job slug required")
	}
	if err := b.scheduleJobLocked(job); err != nil {
		return err
	}
	return b.saveLocked()
}

func (b *Broker) ScheduleTaskFollowUp(taskID, channel, owner, label, payload string, when time.Time) error {
	return b.scheduleJob(schedulerJob{
		Slug:            normalizeSchedulerSlug("task_follow_up", channel, taskID),
		Kind:            "task_follow_up",
		Label:           label,
		TargetType:      "task",
		TargetID:        strings.TrimSpace(taskID),
		Channel:         normalizeChannelSlug(channel),
		IntervalMinutes: 0,
		DueAt:           when.UTC().Format(time.RFC3339),
		NextRun:         when.UTC().Format(time.RFC3339),
		Status:          "scheduled",
		Payload:         payload,
	})
}

func (b *Broker) ScheduleRequestFollowUp(requestID, channel, label, payload string, when time.Time) error {
	return b.scheduleJob(schedulerJob{
		Slug:            normalizeSchedulerSlug("request_follow_up", channel, requestID),
		Kind:            "request_follow_up",
		Label:           label,
		TargetType:      "request",
		TargetID:        strings.TrimSpace(requestID),
		Channel:         normalizeChannelSlug(channel),
		IntervalMinutes: 0,
		DueAt:           when.UTC().Format(time.RFC3339),
		NextRun:         when.UTC().Format(time.RFC3339),
		Status:          "scheduled",
		Payload:         payload,
	})
}

func (b *Broker) ScheduleRecheck(channel, targetType, targetID, label, payload string, when time.Time) error {
	return b.scheduleJob(schedulerJob{
		Slug:            normalizeSchedulerSlug("recheck", channel, targetType, targetID),
		Kind:            "recheck",
		Label:           label,
		TargetType:      strings.TrimSpace(targetType),
		TargetID:        strings.TrimSpace(targetID),
		Channel:         normalizeChannelSlug(channel),
		IntervalMinutes: 0,
		DueAt:           when.UTC().Format(time.RFC3339),
		NextRun:         when.UTC().Format(time.RFC3339),
		Status:          "scheduled",
		Payload:         payload,
	})
}

func (b *Broker) scheduleJob(job schedulerJob) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	job = normalizeSchedulerJob(job)
	if job.Slug == "" {
		return fmt.Errorf("job slug required")
	}
	if job.Channel == "" {
		job.Channel = "general"
	}
	if err := b.scheduleJobLocked(job); err != nil {
		return err
	}
	return b.saveLocked()
}

func (b *Broker) scheduleJobLocked(job schedulerJob) error {
	b.pruneSchedulerJobsLocked(time.Now().UTC())
	for i := range b.scheduler {
		if !schedulerJobMatches(b.scheduler[i], job) {
			continue
		}
		b.scheduler[i] = job
		return nil
	}
	b.scheduler = append(b.scheduler, job)
	return nil
}

func normalizeSchedulerSlug(parts ...string) string {
	var filtered []string
	for _, part := range parts {
		part = normalizeSlugPart(part)
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, ":")
}

func normalizeSlugPart(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	return s
}

func normalizeSchedulerJob(job schedulerJob) schedulerJob {
	job.Slug = strings.TrimSpace(job.Slug)
	job.Kind = strings.TrimSpace(job.Kind)
	job.Label = strings.TrimSpace(job.Label)
	job.TargetType = strings.TrimSpace(job.TargetType)
	job.TargetID = strings.TrimSpace(job.TargetID)
	job.Channel = normalizeChannelSlug(job.Channel)
	job.Provider = strings.TrimSpace(job.Provider)
	job.ScheduleExpr = strings.TrimSpace(job.ScheduleExpr)
	job.WorkflowKey = strings.TrimSpace(job.WorkflowKey)
	job.SkillName = strings.TrimSpace(job.SkillName)
	if job.Channel == "" {
		job.Channel = "general"
	}
	job.Payload = strings.TrimSpace(job.Payload)
	job.Status = normalizeSchedulerStatus(job.Status)
	if job.Status == "" {
		job.Status = "scheduled"
	}
	if schedulerStatusIsTerminal(job.Status) {
		job.DueAt = ""
		job.NextRun = ""
	}
	if job.IntervalMinutes < 0 {
		job.IntervalMinutes = 0
	}
	if job.DueAt == "" && job.NextRun != "" {
		job.DueAt = job.NextRun
	}
	if job.NextRun == "" && job.DueAt != "" {
		job.NextRun = job.DueAt
	}
	return job
}

func schedulerJobMatches(existing, candidate schedulerJob) bool {
	if existing.Slug != "" && candidate.Slug != "" && existing.Slug == candidate.Slug {
		return true
	}
	if existing.Kind != "" && candidate.Kind != "" && existing.Kind != candidate.Kind {
		return false
	}
	if existing.TargetType != "" && candidate.TargetType != "" && existing.TargetType != candidate.TargetType {
		return false
	}
	if existing.TargetID != "" && candidate.TargetID != "" && existing.TargetID != candidate.TargetID {
		return false
	}
	if existing.Channel != "" && candidate.Channel != "" && existing.Channel != candidate.Channel {
		return false
	}
	return existing.Kind != "" && existing.Kind == candidate.Kind && existing.TargetType == candidate.TargetType && existing.TargetID == candidate.TargetID && existing.Channel == candidate.Channel
}

func schedulerJobDue(job schedulerJob, now time.Time) bool {
	if schedulerStatusIsTerminal(job.Status) {
		return false
	}
	if job.DueAt != "" {
		if due, err := time.Parse(time.RFC3339, job.DueAt); err == nil && !due.After(now) {
			return true
		}
	}
	if job.NextRun != "" {
		if due, err := time.Parse(time.RFC3339, job.NextRun); err == nil && !due.After(now) {
			return true
		}
	}
	return false
}

func (b *Broker) completeSchedulerJobsLocked(targetType, targetID, channel string) {
	for i := range b.scheduler {
		job := &b.scheduler[i]
		if targetType != "" && job.TargetType != targetType {
			continue
		}
		if targetID != "" && job.TargetID != targetID {
			continue
		}
		if channel != "" && job.Channel != "" && normalizeChannelSlug(job.Channel) != normalizeChannelSlug(channel) {
			continue
		}
		job.Status = "done"
		job.DueAt = ""
		job.NextRun = ""
		job.LastRun = time.Now().UTC().Format(time.RFC3339)
	}
}

func (b *Broker) scheduleTaskLifecycleLocked(task *teamTask) {
	if task == nil {
		return
	}
	normalizeTaskPlan(task)
	taskChannel := normalizeChannelSlug(task.Channel)
	if taskChannel == "" {
		taskChannel = "general"
	}
	followUpMinutes := config.ResolveTaskFollowUpInterval()
	recheckMinutes := config.ResolveTaskRecheckInterval()
	reminderMinutes := config.ResolveTaskReminderInterval()
	now := time.Now().UTC()
	if strings.EqualFold(task.Status, "done") || strings.EqualFold(task.Status, "canceled") || strings.EqualFold(task.Status, "cancelled") {
		task.FollowUpAt = ""
		task.ReminderAt = ""
		task.RecheckAt = ""
		task.DueAt = ""
		b.completeSchedulerJobsLocked("task", task.ID, taskChannel)
		b.resolveWatchdogAlertsLocked("task", task.ID, taskChannel)
		return
	}
	switch strings.ToLower(strings.TrimSpace(task.Status)) {
	case "in_progress":
		due := now.Add(time.Duration(followUpMinutes) * time.Minute)
		task.FollowUpAt = due.Format(time.RFC3339)
		task.ReminderAt = due.Add(time.Duration(reminderMinutes) * time.Minute).Format(time.RFC3339)
		task.RecheckAt = due.Add(time.Duration(recheckMinutes) * time.Minute).Format(time.RFC3339)
		task.DueAt = task.FollowUpAt
		_ = b.scheduleJobLocked(normalizeSchedulerJob(schedulerJob{
			Slug:       normalizeSchedulerSlug("task_follow_up", taskChannel, task.ID),
			Kind:       "task_follow_up",
			Label:      "Follow up on " + task.Title,
			TargetType: "task",
			TargetID:   task.ID,
			Channel:    taskChannel,
			DueAt:      task.FollowUpAt,
			NextRun:    task.FollowUpAt,
			Status:     "scheduled",
			Payload:    task.Details,
		}))
	default:
		due := now.Add(time.Duration(recheckMinutes) * time.Minute)
		task.RecheckAt = due.Format(time.RFC3339)
		task.ReminderAt = due.Add(time.Duration(reminderMinutes) * time.Minute).Format(time.RFC3339)
		task.FollowUpAt = task.RecheckAt
		task.DueAt = task.RecheckAt
		_ = b.scheduleJobLocked(normalizeSchedulerJob(schedulerJob{
			Slug:       normalizeSchedulerSlug("recheck", taskChannel, "task", task.ID),
			Kind:       "recheck",
			Label:      "Recheck task " + truncateSummary(task.Title, 48),
			TargetType: "task",
			TargetID:   task.ID,
			Channel:    taskChannel,
			DueAt:      task.RecheckAt,
			NextRun:    task.RecheckAt,
			Status:     "scheduled",
			Payload:    task.Details,
		}))
	}
}

func (b *Broker) scheduleRequestLifecycleLocked(req *humanInterview) {
	if req == nil {
		return
	}
	*req = normalizeRequestRecord(*req)
	reqChannel := normalizeChannelSlug(req.Channel)
	if reqChannel == "" {
		reqChannel = "general"
	}
	reminderMinutes := config.ResolveTaskReminderInterval()
	followUpMinutes := config.ResolveTaskFollowUpInterval()
	now := time.Now().UTC()
	if requestStatusIsTerminal(req.Status) {
		clearRequestLifecycleFields(req)
		b.completeSchedulerJobsLocked("request", req.ID, reqChannel)
		b.resolveWatchdogAlertsLocked("request", req.ID, reqChannel)
		return
	}
	if req.SLAHours <= 0 {
		req.SLAHours = defaultRequestSLAHours()
	}
	req.EscalationState = normalizeRequestEscalationState(req.EscalationState, req.EscalationCount)
	due := now.Add(time.Duration(reminderMinutes) * time.Minute)
	req.ReminderAt = due.Format(time.RFC3339)
	req.FollowUpAt = due.Add(time.Duration(followUpMinutes) * time.Minute).Format(time.RFC3339)
	req.RecheckAt = req.ReminderAt
	req.DueAt = req.ReminderAt
	if strings.TrimSpace(req.NextEscalationAt) == "" {
		req.NextEscalationAt = now.Add(time.Duration(req.SLAHours) * time.Hour).Format(time.RFC3339)
	}
	_ = b.scheduleJobLocked(normalizeSchedulerJob(schedulerJob{
		Slug:       normalizeSchedulerSlug("request_follow_up", reqChannel, req.ID),
		Kind:       "request_follow_up",
		Label:      "Follow up on " + req.Title,
		TargetType: "request",
		TargetID:   req.ID,
		Channel:    reqChannel,
		DueAt:      req.ReminderAt,
		NextRun:    req.ReminderAt,
		Status:     "scheduled",
		Payload:    req.Question,
	}))
}

func (b *Broker) EscalateRequestIfDue(requestID string, now time.Time) (humanInterview, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return humanInterview{}, false, fmt.Errorf("request id required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	for i := range b.requests {
		req := &b.requests[i]
		if req.ID != requestID {
			continue
		}
		if !requestIsActive(*req) {
			return *req, false, nil
		}
		normalized, err := resolveRequestTransition(*req, "escalate", nil, now)
		if err != nil {
			return humanInterview{}, false, err
		}
		*req = normalized
		if req.SLAHours <= 0 {
			req.SLAHours = defaultRequestSLAHours()
		}
		nextEscalationAt := strings.TrimSpace(req.NextEscalationAt)
		if nextEscalationAt == "" {
			req.NextEscalationAt = now.Add(time.Duration(req.SLAHours) * time.Hour).Format(time.RFC3339)
			req.UpdatedAt = now.Format(time.RFC3339)
			if err := b.saveLocked(); err != nil {
				return humanInterview{}, false, err
			}
			return *req, false, nil
		}
		dueAt, err := time.Parse(time.RFC3339, nextEscalationAt)
		if err != nil || now.Before(dueAt) {
			return *req, false, nil
		}
		req.EscalationCount++
		req.EscalationState = normalizeRequestEscalationState(req.EscalationState, req.EscalationCount)
		req.LastEscalatedAt = now.Format(time.RFC3339)
		req.NextEscalationAt = now.Add(time.Duration(req.SLAHours) * time.Hour).Format(time.RFC3339)
		req.UpdatedAt = now.Format(time.RFC3339)
		if err := b.saveLocked(); err != nil {
			return humanInterview{}, false, err
		}
		return *req, true, nil
	}
	return humanInterview{}, false, fmt.Errorf("request not found")
}

func (b *Broker) handleHealth(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	lockStartedAt := time.Now()
	b.mu.Lock()
	lockWait := time.Since(lockStartedAt)
	mode := b.sessionMode
	agent := b.oneOnOneAgent
	focus := b.focusMode
	provider := b.runtimeProvider
	b.mu.Unlock()
	if strings.TrimSpace(provider) == "" {
		provider = config.ResolveLLMProvider("")
	}
	memoryStatus := ResolveMemoryBackendStatus()
	readiness := b.readinessSnapshot()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":                "ok",
		"session_mode":          mode,
		"one_on_one_agent":      agent,
		"focus_mode":            focus,
		"provider":              provider,
		"memory_backend":        memoryStatus.SelectedKind,
		"memory_backend_active": memoryStatus.ActiveKind,
		"memory_backend_ready":  memoryStatus.ActiveKind != config.MemoryBackendNone,
		"readiness":             readiness,
		"build":                 buildinfo.Current(),
	})
	if brokerDebugHTTPTimingEnabled() {
		log.Printf("broker http timing path=%s lock_wait=%s total=%s", r.URL.Path, lockWait, time.Since(startedAt))
	}
}

func (b *Broker) handleReady(w http.ResponseWriter, _ *http.Request) {
	readiness := b.readinessSnapshot()
	status := http.StatusOK
	if !readiness.Ready {
		status = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(readiness)
}

func (b *Broker) handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(buildinfo.Current())
}

func (b *Broker) handleSessionMode(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		mode, agent := b.SessionModeState()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"session_mode":     mode,
			"one_on_one_agent": agent,
		})
	case http.MethodPost:
		var body struct {
			Mode  string `json:"mode"`
			Agent string `json:"agent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if err := b.SetSessionMode(body.Mode, body.Agent); err != nil {
			http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
			return
		}
		mode, agent := b.SessionModeState()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"session_mode":     mode,
			"one_on_one_agent": agent,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *Broker) handleFocusMode(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"focus_mode": b.FocusModeEnabled(),
		})
	case http.MethodPost:
		var body struct {
			FocusMode bool `json:"focus_mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if err := b.SetFocusMode(body.FocusMode); err != nil {
			http.Error(w, "failed to persist", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"focus_mode": b.FocusModeEnabled(),
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *Broker) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.Reset()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (b *Broker) handleResetDM(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Agent   string `json:"agent"`
		Channel string `json:"channel"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	agent := strings.TrimSpace(body.Agent)
	channel := normalizeChannelSlug(body.Channel)
	if channel == "" {
		channel = "general"
	}

	b.mu.Lock()
	// Resetting a DM must clear the entire thread, including any polluted
	// third-party messages that slipped into the same channel.
	filtered := make([]channelMessage, 0, len(b.messages))
	removed := 0
	for _, msg := range b.messages {
		if normalizeChannelSlug(msg.Channel) != channel {
			filtered = append(filtered, msg)
			continue
		}
		removed++
	}
	b.replaceMessagesLocked(filtered)
	_ = b.saveLocked()
	b.mu.Unlock()

	// Respawn the agent's Claude Code session to clear its context
	go respawnAgentPane(agent)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "removed": removed})
}

func (b *Broker) clearChannelTranscriptLocked(channel string) (removedMessages, removedRequests, removedExecutionNodes int) {
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		channel = "general"
	}

	filteredMessages := b.messages[:0]
	for _, msg := range b.messages {
		if normalizeChannelSlug(msg.Channel) == channel {
			removedMessages++
			continue
		}
		filteredMessages = append(filteredMessages, msg)
	}
	b.replaceMessagesLocked(filteredMessages)

	filteredRequests := b.requests[:0]
	for _, req := range b.requests {
		if normalizeChannelSlug(req.Channel) == channel {
			removedRequests++
			continue
		}
		filteredRequests = append(filteredRequests, req)
	}
	b.requests = filteredRequests
	b.pendingInterview = firstBlockingRequest(b.requests)

	filteredNodes := b.executionNodes[:0]
	for _, node := range b.executionNodes {
		if normalizeChannelSlug(node.Channel) == channel {
			removedExecutionNodes++
			continue
		}
		filteredNodes = append(filteredNodes, node)
	}
	b.executionNodes = filteredNodes

	if b.sharedMemory != nil {
		delete(b.sharedMemory, channelMemoryNamespace(channel))
	}
	return
}

func (b *Broker) purgeChannelStateLocked(channel string) {
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		return
	}

	filteredMessages := b.messages[:0]
	for _, msg := range b.messages {
		if normalizeChannelSlug(msg.Channel) == channel {
			continue
		}
		filteredMessages = append(filteredMessages, msg)
	}
	b.replaceMessagesLocked(filteredMessages)

	filteredTasks := b.tasks[:0]
	for _, task := range b.tasks {
		if normalizeChannelSlug(task.Channel) == channel {
			continue
		}
		filteredTasks = append(filteredTasks, task)
	}
	b.tasks = filteredTasks

	filteredRequests := b.requests[:0]
	for _, req := range b.requests {
		if normalizeChannelSlug(req.Channel) == channel {
			continue
		}
		filteredRequests = append(filteredRequests, req)
	}
	b.requests = filteredRequests
	b.pendingInterview = firstBlockingRequest(b.requests)

	filteredActions := b.actions[:0]
	for _, action := range b.actions {
		if normalizeChannelSlug(action.Channel) == channel {
			continue
		}
		filteredActions = append(filteredActions, action)
	}
	b.actions = filteredActions

	filteredSignals := b.signals[:0]
	for _, signal := range b.signals {
		if normalizeChannelSlug(signal.Channel) == channel {
			continue
		}
		filteredSignals = append(filteredSignals, signal)
	}
	b.signals = filteredSignals

	filteredDecisions := b.decisions[:0]
	for _, decision := range b.decisions {
		if normalizeChannelSlug(decision.Channel) == channel {
			continue
		}
		filteredDecisions = append(filteredDecisions, decision)
	}
	b.decisions = filteredDecisions

	filteredWatchdogs := b.watchdogs[:0]
	for _, alert := range b.watchdogs {
		if normalizeChannelSlug(alert.Channel) == channel {
			continue
		}
		filteredWatchdogs = append(filteredWatchdogs, alert)
	}
	b.watchdogs = filteredWatchdogs

	filteredScheduler := b.scheduler[:0]
	for _, job := range b.scheduler {
		if normalizeChannelSlug(job.Channel) == channel {
			continue
		}
		filteredScheduler = append(filteredScheduler, job)
	}
	b.scheduler = filteredScheduler

	filteredNodes := b.executionNodes[:0]
	for _, node := range b.executionNodes {
		if normalizeChannelSlug(node.Channel) == channel {
			continue
		}
		filteredNodes = append(filteredNodes, node)
	}
	b.executionNodes = filteredNodes

	if b.sharedMemory != nil {
		delete(b.sharedMemory, channelMemoryNamespace(channel))
	}
}

func (b *Broker) handleClearChannel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Channel string `json:"channel"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	channel := normalizeChannelSlug(body.Channel)
	if channel == "" {
		channel = "general"
	}

	b.mu.Lock()
	if b.findChannelLocked(channel) == nil {
		b.mu.Unlock()
		http.Error(w, "channel not found", http.StatusNotFound)
		return
	}
	if !b.canAccessChannelLocked("human", channel) {
		b.mu.Unlock()
		http.Error(w, "channel access denied", http.StatusForbidden)
		return
	}
	removedMessages, removedRequests, removedExecutionNodes := b.clearChannelTranscriptLocked(channel)
	if err := b.saveLocked(); err != nil {
		b.mu.Unlock()
		http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
		return
	}
	dmAgent := ""
	if IsDMSlug(channel) {
		dmAgent = DMTargetAgent(channel)
	}
	b.mu.Unlock()

	if dmAgent != "" {
		go respawnAgentPane(dmAgent)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":                      true,
		"channel":                 channel,
		"removed_messages":        removedMessages,
		"removed_requests":        removedRequests,
		"removed_execution_nodes": removedExecutionNodes,
	})
}

// respawnAgentPane restarts an agent's Claude Code session in its tmux pane.
func respawnAgentPane(slug string) {
	manifest := company.DefaultManifest()
	loaded, err := company.LoadManifest()
	if err == nil && len(loaded.Members) > 0 {
		manifest = loaded
	}

	for i, agent := range manifest.Members {
		if agent.Slug == slug {
			paneIdx := i + 1 // pane 0 is channel view
			target := fmt.Sprintf("wuphf-team:team.%d", paneIdx)
			// Send Ctrl+C to interrupt, then exit to terminate
			_ = exec.Command("tmux", "-L", "wuphf", "send-keys", "-t", target, "C-c", "").Run()
			time.Sleep(500 * time.Millisecond)
			_ = exec.Command("tmux", "-L", "wuphf", "send-keys", "-t", target, "C-c", "").Run()
			time.Sleep(500 * time.Millisecond)
			// Respawn the pane with a fresh claude session
			_ = exec.Command("tmux", "-L", "wuphf", "respawn-pane", "-k", "-t", target).Run()
			return
		}
	}
}

func (b *Broker) handleUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.mu.Lock()
	usage := b.usage
	if usage.Agents == nil {
		usage.Agents = make(map[string]usageTotals)
	}
	b.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(usage)
}

// RecordPolicy adds a new active policy. Deduplicates by exact rule text.
func (b *Broker) RecordPolicy(source, rule string) (officePolicy, error) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return officePolicy{}, fmt.Errorf("rule cannot be empty")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	p, _, err := b.recordPolicyLocked(source, rule)
	if err != nil {
		return officePolicy{}, err
	}
	_ = b.saveLocked()
	return p, nil
}

func (b *Broker) recordPolicyLocked(source, rule string) (officePolicy, bool, error) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return officePolicy{}, false, fmt.Errorf("rule cannot be empty")
	}
	for i, p := range b.policies {
		if strings.EqualFold(p.Rule, rule) {
			wasActive := b.policies[i].Active
			b.policies[i].Active = true
			return b.policies[i], wasActive, nil
		}
	}
	p := newOfficePolicy(source, rule)
	b.policies = append(b.policies, p)
	return p, false, nil
}

// ListPolicies returns all active policies.
func (b *Broker) ListPolicies() []officePolicy {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]officePolicy, 0, len(b.policies))
	for _, p := range b.policies {
		if p.Active {
			out = append(out, p)
		}
	}
	return out
}

func (b *Broker) handlePolicies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		b.mu.Lock()
		out := make([]officePolicy, 0, len(b.policies))
		for _, p := range b.policies {
			if p.Active {
				out = append(out, p)
			}
		}
		b.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"policies": out})

	case http.MethodPost:
		var body struct {
			Source    string `json:"source"`
			Rule      string `json:"rule"`
			RequestID string `json:"request_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(body.Rule) == "" {
			http.Error(w, "rule is required", http.StatusBadRequest)
			return
		}
		requestID := strings.TrimSpace(body.RequestID)
		b.mu.Lock()
		if payload, ok := b.findMutationAckLocked("policy:create", requestID); ok {
			b.mu.Unlock()
			payload["duplicate"] = true
			b.respondPersistedMutation(w, payload)
			return
		}
		p, duplicate, err := b.recordPolicyLocked(body.Source, body.Rule)
		if err != nil {
			b.mu.Unlock()
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !duplicate {
			b.appendActionLocked("policy_created", "office", "general", "you", truncateSummary(p.Rule, 140), p.ID)
		}
		payload := map[string]any{
			"persisted": true,
			"duplicate": duplicate,
			"policy":    p,
		}
		if err := b.rememberMutationAckLocked("policy:create", requestID, payload); err != nil {
			b.mu.Unlock()
			http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
			return
		}
		if err := b.saveLocked(); err != nil {
			b.mu.Unlock()
			http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
			return
		}
		b.mu.Unlock()
		b.respondPersistedMutation(w, payload)

	case http.MethodDelete:
		id := strings.TrimPrefix(r.URL.Path, "/policies/")
		id = strings.TrimSpace(id)
		if id == "" || id == "/policies" {
			// Parse from body
			var body struct {
				ID        string `json:"id"`
				RequestID string `json:"request_id"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			id = strings.TrimSpace(body.ID)
			requestID := strings.TrimSpace(body.RequestID)
			b.mu.Lock()
			if payload, ok := b.findMutationAckLocked("policy:delete", requestID); ok {
				b.mu.Unlock()
				payload["duplicate"] = true
				b.respondPersistedMutation(w, payload)
				return
			}
			if id == "" {
				b.mu.Unlock()
				http.Error(w, "id required", http.StatusBadRequest)
				return
			}
			payload := map[string]any{
				"ok":        true,
				"persisted": true,
				"duplicate": false,
			}
			found := false
			for i, p := range b.policies {
				if p.ID != id {
					continue
				}
				found = true
				if !b.policies[i].Active {
					payload["duplicate"] = true
					payload["policy"] = b.policies[i]
					break
				}
				b.policies[i].Active = false
				payload["policy"] = b.policies[i]
				b.appendActionLocked("policy_deactivated", "office", "general", "you", truncateSummary(b.policies[i].Rule, 140), b.policies[i].ID)
				break
			}
			if !found {
				b.mu.Unlock()
				http.Error(w, "policy not found", http.StatusNotFound)
				return
			}
			if err := b.rememberMutationAckLocked("policy:delete", requestID, payload); err != nil {
				b.mu.Unlock()
				http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
				return
			}
			if err := b.saveLocked(); err != nil {
				b.mu.Unlock()
				http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
				return
			}
			b.mu.Unlock()
			b.respondPersistedMutation(w, payload)
			return
		}
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		b.mu.Lock()
		payload := map[string]any{
			"ok":        true,
			"persisted": true,
			"duplicate": false,
		}
		found := false
		for i, p := range b.policies {
			if p.ID != id {
				continue
			}
			found = true
			if !b.policies[i].Active {
				payload["duplicate"] = true
				payload["policy"] = b.policies[i]
				break
			}
			b.policies[i].Active = false
			payload["policy"] = b.policies[i]
			b.appendActionLocked("policy_deactivated", "office", "general", "you", truncateSummary(b.policies[i].Rule, 140), b.policies[i].ID)
			break
		}
		if !found {
			b.mu.Unlock()
			http.Error(w, "policy not found", http.StatusNotFound)
			return
		}
		if err := b.saveLocked(); err != nil {
			b.mu.Unlock()
			http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
			return
		}
		b.mu.Unlock()
		b.respondPersistedMutation(w, payload)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *Broker) handleSignals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.mu.Lock()
	signals := make([]officeSignalRecord, len(b.signals))
	copy(signals, b.signals)
	b.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"signals": signals})
}

func (b *Broker) handleDecisions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.mu.Lock()
	decisions := make([]officeDecisionRecord, len(b.decisions))
	copy(decisions, b.decisions)
	b.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"decisions": decisions})
}

func (b *Broker) handleWatchdogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.mu.Lock()
	alerts := make([]watchdogAlert, len(b.watchdogs))
	copy(alerts, b.watchdogs)
	b.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"watchdogs": alerts})
}

func (b *Broker) handleActions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		b.mu.Lock()
		actions := make([]officeActionLog, len(b.actions))
		copy(actions, b.actions)
		b.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"actions": actions})
	case http.MethodPost:
		var body struct {
			Kind       string   `json:"kind"`
			Source     string   `json:"source"`
			Channel    string   `json:"channel"`
			Actor      string   `json:"actor"`
			Summary    string   `json:"summary"`
			RelatedID  string   `json:"related_id"`
			SignalIDs  []string `json:"signal_ids"`
			DecisionID string   `json:"decision_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(body.Kind) == "" || strings.TrimSpace(body.Summary) == "" {
			http.Error(w, "kind and summary required", http.StatusBadRequest)
			return
		}
		if err := b.RecordAction(
			body.Kind,
			body.Source,
			body.Channel,
			body.Actor,
			body.Summary,
			body.RelatedID,
			body.SignalIDs,
			body.DecisionID,
		); err != nil {
			http.Error(w, "failed to persist action", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type studioGeneratedPackage map[string]map[string]any

type studioGeneratedArtifact struct {
	Kind  string         `json:"kind"`
	Title string         `json:"title,omitempty"`
	Data  map[string]any `json:"data,omitempty"`
}

type studioStubExecution struct {
	ID           string         `json:"id"`
	Provider     string         `json:"provider"`
	WorkflowKey  string         `json:"workflow_key"`
	Status       string         `json:"status"`
	Mode         string         `json:"mode"`
	Integrations []string       `json:"integrations,omitempty"`
	Summary      string         `json:"summary"`
	Input        map[string]any `json:"input,omitempty"`
	Output       map[string]any `json:"output,omitempty"`
}

func decodeStudioGeneratedPackage(raw string, requiredArtifacts []string) (studioGeneratedPackage, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("empty codex response")
	}
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) >= 3 && strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
			lines = lines[1:]
			if last := len(lines) - 1; last >= 0 && strings.HasPrefix(strings.TrimSpace(lines[last]), "```") {
				lines = lines[:last]
			}
			trimmed = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end >= start {
		trimmed = trimmed[start : end+1]
	}
	var pkg studioGeneratedPackage
	if err := json.Unmarshal([]byte(trimmed), &pkg); err != nil {
		return nil, err
	}
	for _, artifactID := range requiredArtifacts {
		if len(pkg[artifactID]) == 0 {
			return nil, fmt.Errorf("missing required artifact %q", artifactID)
		}
	}
	return pkg, nil
}

func buildStudioFollowUpStubExecutions(runTitle string, offers []any, pkg studioGeneratedPackage) []studioStubExecution {
	offerNames := extractStudioOfferNames(offers)
	artifactIDs := studioPackageArtifactIDs(pkg)
	primaryArtifactID, primaryArtifact := studioPrimaryPackageArtifact(pkg, artifactIDs)
	primarySummary := firstStudioString(
		primaryArtifact["summary"],
		primaryArtifact["objective"],
		primaryArtifact["title"],
		primaryArtifact["name"],
		runTitle,
	)
	return []studioStubExecution{
		{
			ID:           fmt.Sprintf("followup-review-%d", time.Now().UTC().UnixNano()),
			Provider:     "one",
			WorkflowKey:  "artifact-review-sync",
			Status:       "success",
			Mode:         "dry_run",
			Integrations: []string{"artifact-review"},
			Summary:      fmt.Sprintf("Prepared a review sync payload for %s.", runTitle),
			Input: map[string]any{
				"run_title":        runTitle,
				"artifact_ids":     artifactIDs,
				"primary_artifact": primaryArtifactID,
			},
			Output: map[string]any{
				"destination":  "review queue",
				"draft_status": "ready_for_review",
			},
		},
		{
			ID:           fmt.Sprintf("followup-offers-%d", time.Now().UTC().UnixNano()+1),
			Provider:     "one",
			WorkflowKey:  "offer-alignment-check",
			Status:       "success",
			Mode:         "dry_run",
			Integrations: []string{"offer-alignment"},
			Summary:      fmt.Sprintf("Prepared offer alignment notes for %s.", runTitle),
			Input: map[string]any{
				"run_title":    runTitle,
				"offer_names":  offerNames,
				"artifact_ids": artifactIDs,
			},
			Output: map[string]any{
				"destination":  "offer queue",
				"draft_status": "ready_for_review",
			},
		},
		{
			ID:           fmt.Sprintf("followup-approval-%d", time.Now().UTC().UnixNano()+2),
			Provider:     "one",
			WorkflowKey:  "approval-gate-review",
			Status:       "success",
			Mode:         "dry_run",
			Integrations: []string{"approval-gates"},
			Summary:      fmt.Sprintf("Prepared approval gates for %s.", runTitle),
			Input: map[string]any{
				"run_title":        runTitle,
				"primary_artifact": primaryArtifactID,
				"primary_summary":  primarySummary,
			},
			Output: map[string]any{
				"destination":  "approval queue",
				"draft_status": "ready_for_review",
			},
		},
	}
}

func studioDefaultArtifactDefinitions() []operations.ArtifactType {
	return []operations.ArtifactType{
		{ID: "objective_brief", Name: "Objective brief", Description: "Problem statement, constraints, and desired outcome for one run."},
		{ID: "execution_packet", Name: "Execution packet", Description: "Checklist, dependencies, outputs, and handoff details for one run."},
		{ID: "approval_checklist", Name: "Approval checklist", Description: "Review gates and required human approvals before live action."},
	}
}

func studioNormalizeArtifactDefinitions(defs []operations.ArtifactType) []operations.ArtifactType {
	normalized := make([]operations.ArtifactType, 0, len(defs))
	seen := make(map[string]struct{}, len(defs))
	for _, def := range defs {
		def.ID = strings.TrimSpace(def.ID)
		if def.ID == "" {
			continue
		}
		if _, ok := seen[def.ID]; ok {
			continue
		}
		seen[def.ID] = struct{}{}
		normalized = append(normalized, def)
	}
	if len(normalized) == 0 {
		return studioDefaultArtifactDefinitions()
	}
	return normalized
}

func studioArtifactIDs(defs []operations.ArtifactType) []string {
	ids := make([]string, 0, len(defs))
	for _, def := range defs {
		ids = append(ids, def.ID)
	}
	return ids
}

func buildStudioGeneratedArtifacts(runTitle string, pkg studioGeneratedPackage, defs []operations.ArtifactType) []studioGeneratedArtifact {
	artifacts := make([]studioGeneratedArtifact, 0, len(defs))
	for _, def := range studioNormalizeArtifactDefinitions(defs) {
		artifacts = append(artifacts, studioGeneratedArtifact{
			Kind:  def.ID,
			Title: runTitle,
			Data:  pkg[def.ID],
		})
	}
	return artifacts
}

func extractStudioOfferNames(offers []any) []string {
	names := make([]string, 0, len(offers))
	for _, item := range offers {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(fmt.Sprintf("%v", record["name"]))
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names
}

func extractStudioStringSlice(raw any) []string {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(fmt.Sprintf("%v", item))
		if text == "" || text == "<nil>" {
			continue
		}
		values = append(values, text)
	}
	return values
}

func firstStudioString(values ...any) string {
	for _, value := range values {
		text := strings.TrimSpace(fmt.Sprintf("%v", value))
		if text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}

func (b *Broker) handleStudioGeneratePackage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Channel   string                    `json:"channel"`
		Actor     string                    `json:"actor"`
		Workspace map[string]any            `json:"workspace"`
		Run       map[string]any            `json:"run"`
		Offers    []any                     `json:"offers"`
		Artifacts []operations.ArtifactType `json:"artifacts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	channel := normalizeChannelSlug(body.Channel)
	if channel == "" {
		channel = "general"
	}
	actor := strings.TrimSpace(body.Actor)
	if actor == "" {
		actor = "human"
	}
	runTitle := strings.TrimSpace(fmt.Sprintf("%v", body.Run["title"]))
	if runTitle == "" {
		http.Error(w, "run.title required", http.StatusBadRequest)
		return
	}

	artifactDefs := studioNormalizeArtifactDefinitions(body.Artifacts)
	systemPrompt := strings.TrimSpace(`You generate structured operation artifacts for a reusable workflow.
Return valid JSON only. No markdown fences. No prose outside JSON.
The top-level object must contain exactly:
` + strings.Join(func() []string {
		items := make([]string, 0, len(artifactDefs))
		for _, def := range artifactDefs {
			items = append(items, "- "+def.ID)
		}
		return items
	}(), "\n"))

	promptPayload, _ := json.Marshal(map[string]any{
		"workspace": body.Workspace,
		"run":       body.Run,
		"offers":    body.Offers,
		"artifacts": artifactDefs,
	})
	prompt := strings.TrimSpace(`Turn this run into a production-ready artifact bundle for the active operation.

Rules:
- Keep claims concrete and production-safe.
- Use short, scannable fields.
- For each requested artifact, use the provided id, name, and description to shape the object.
- Prefer compact objects with fields like summary, goals, checklist, dependencies, outputs, risks, approvals, notes, links, or tags when they fit the artifact purpose.
- Only return the requested artifact ids as top-level keys.

Input JSON:
` + string(promptPayload))

	raw, err := studioPackageGenerator(systemPrompt, prompt, "")
	if err != nil {
		http.Error(w, "package generation failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	pkg, err := decodeStudioGeneratedPackage(raw, studioArtifactIDs(artifactDefs))
	if err != nil {
		http.Error(w, "invalid codex package output: "+err.Error(), http.StatusBadGateway)
		return
	}
	stubExecutions := buildStudioFollowUpStubExecutions(runTitle, body.Offers, pkg)
	artifacts := buildStudioGeneratedArtifacts(runTitle, pkg, artifactDefs)
	summary := truncateSummary("Generated operation artifacts for "+runTitle, 140)
	if err := b.RecordAction("studio_package_generated", "studio", channel, actor, summary, runTitle, nil, ""); err != nil {
		http.Error(w, "failed to persist action", http.StatusInternalServerError)
		return
	}
	for _, execution := range stubExecutions {
		if err := b.RecordAction("studio_followup_stub_executed", "studio", channel, actor, truncateSummary(execution.Summary, 140), runTitle, nil, ""); err != nil {
			http.Error(w, "failed to persist follow-up stub action", http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":              true,
		"package":         pkg,
		"artifacts":       artifacts,
		"stub_executions": stubExecutions,
	})
}

func studioPackageArtifactIDs(pkg studioGeneratedPackage) []string {
	ids := make([]string, 0, len(pkg))
	for id := range pkg {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func studioPrimaryPackageArtifact(pkg studioGeneratedPackage, artifactIDs []string) (string, map[string]any) {
	for _, id := range artifactIDs {
		if item, ok := pkg[id]; ok && len(item) > 0 {
			return id, item
		}
	}
	for id, item := range pkg {
		if len(item) > 0 {
			return strings.TrimSpace(id), item
		}
	}
	return "", map[string]any{}
}

func normalizeStudioWorkflowDefinition(raw json.RawMessage) ([]byte, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var encoded string
		if err := json.Unmarshal(trimmed, &encoded); err != nil {
			return nil, err
		}
		trimmed = []byte(strings.TrimSpace(encoded))
	}
	if len(trimmed) == 0 {
		return nil, nil
	}
	return trimmed, nil
}

func studioWorkflowHints(definition []byte) (dryRun bool, mock bool, integrations []string) {
	var parsed struct {
		Steps []map[string]any `json:"steps"`
	}
	if err := json.Unmarshal(definition, &parsed); err != nil {
		return false, false, nil
	}
	seen := make(map[string]struct{})
	for _, step := range parsed.Steps {
		if v, ok := step["dry_run"].(bool); ok && v {
			dryRun = true
		}
		if v, ok := step["mock"].(bool); ok && v {
			mock = true
		}
		platform := strings.TrimSpace(fmt.Sprintf("%v", step["platform"]))
		if platform == "" || platform == "<nil>" {
			continue
		}
		if _, exists := seen[platform]; exists {
			continue
		}
		seen[platform] = struct{}{}
		integrations = append(integrations, platform)
	}
	return dryRun, mock, integrations
}

func workflowCreateConflict(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "already exists") ||
		strings.Contains(text, "duplicate") ||
		strings.Contains(text, "conflict")
}

func uniqueStrings(values ...[]string) []string {
	var out []string
	seen := make(map[string]struct{})
	for _, group := range values {
		for _, value := range group {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	return out
}

func workflowRunModeLabel(dryRun, mock bool) string {
	switch {
	case dryRun && mock:
		return "dry-run + mock"
	case dryRun:
		return "dry-run"
	case mock:
		return "mock"
	default:
		return "live"
	}
}

func mustMarshalStudioJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{"error":"marshal_failed"}`)
	}
	return json.RawMessage(data)
}

func executeStudioWorkflowStub(workflowKey string, definition []byte, inputs map[string]any, dryRun, mock bool) (action.WorkflowExecuteResult, error) {
	var parsed struct {
		Steps []map[string]any `json:"steps"`
	}
	if err := json.Unmarshal(definition, &parsed); err != nil {
		return action.WorkflowExecuteResult{}, err
	}
	now := time.Now().UTC()
	runID := fmt.Sprintf("studiowf_%d", now.UnixNano())
	stepLogs := make(map[string]json.RawMessage, len(parsed.Steps))
	events := make([]json.RawMessage, 0, len(parsed.Steps)+2)
	events = append(events, mustMarshalStudioJSON(map[string]any{
		"event":        "workflow_started",
		"provider":     "studio_stub",
		"workflow_key": workflowKey,
		"run_id":       runID,
	}))
	status := "success"
	for i, step := range parsed.Steps {
		stepID := strings.TrimSpace(fmt.Sprintf("%v", step["id"]))
		if stepID == "" {
			stepID = fmt.Sprintf("step-%d", i+1)
		}
		stepType := strings.TrimSpace(fmt.Sprintf("%v", step["kind"]))
		if stepType == "" || stepType == "<nil>" {
			stepType = strings.TrimSpace(fmt.Sprintf("%v", step["type"]))
		}
		if stepType == "" || stepType == "<nil>" {
			stepType = "action"
		}
		stepStatus := "completed"
		if dryRun {
			stepStatus = "planned"
		}
		if mock {
			stepStatus = "mocked"
		}
		payload := map[string]any{
			"id":       stepID,
			"type":     stepType,
			"status":   stepStatus,
			"platform": strings.TrimSpace(fmt.Sprintf("%v", step["platform"])),
			"action":   strings.TrimSpace(fmt.Sprintf("%v", step["action"])),
			"inputs":   inputs,
		}
		stepLogs[stepID] = mustMarshalStudioJSON(payload)
		events = append(events, mustMarshalStudioJSON(map[string]any{
			"event":   "workflow_step_completed",
			"step_id": stepID,
			"type":    stepType,
			"status":  stepStatus,
		}))
	}
	events = append(events, mustMarshalStudioJSON(map[string]any{
		"event":  "workflow_finished",
		"run_id": runID,
		"status": status,
	}))
	return action.WorkflowExecuteResult{
		RunID:  runID,
		Status: status,
		Steps:  stepLogs,
		Events: events,
	}, nil
}

func (b *Broker) recordStudioWorkflowExecution(channel, actor, skillName, workflowKey, providerName, title, status string, when time.Time) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	var skill *teamSkill
	if strings.TrimSpace(skillName) != "" {
		skill = b.findSkillByNameLocked(skillName)
	}
	if skill == nil && strings.TrimSpace(workflowKey) != "" {
		skill = b.findSkillByWorkflowKeyLocked(workflowKey)
	}
	if skill != nil {
		skill.UsageCount++
		skill.LastExecutionStatus = strings.TrimSpace(status)
		skill.LastExecutionAt = when.UTC().Format(time.RFC3339)
		skill.UpdatedAt = when.UTC().Format(time.RFC3339)
		if strings.TrimSpace(title) == "" {
			title = skill.Title
		}
	}
	if strings.TrimSpace(title) == "" {
		title = workflowKey
	}
	b.counter++
	b.appendMessageLocked(channelMessage{
		ID:        fmt.Sprintf("msg-%d", b.counter),
		From:      actor,
		Channel:   channel,
		Kind:      "skill_invocation",
		Title:     title,
		Content:   fmt.Sprintf("Workflow %q executed via %s (%s)", workflowKey, providerName, status),
		Timestamp: when.UTC().Format(time.RFC3339),
	})
	return b.saveLocked()
}

func (b *Broker) handleStudioRunWorkflow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Channel            string          `json:"channel"`
		Actor              string          `json:"actor"`
		SkillName          string          `json:"skill_name"`
		WorkflowKey        string          `json:"workflow_key"`
		WorkflowProvider   string          `json:"workflow_provider"`
		WorkflowDefinition json.RawMessage `json:"workflow_definition"`
		Inputs             map[string]any  `json:"inputs"`
		DryRun             *bool           `json:"dry_run"`
		Mock               *bool           `json:"mock"`
		AllowBash          bool            `json:"allow_bash"`
		Integrations       []string        `json:"integrations"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	channel := normalizeChannelSlug(body.Channel)
	if channel == "" {
		channel = "general"
	}
	actor := strings.TrimSpace(body.Actor)
	if actor == "" {
		actor = "human"
	}

	var (
		skillName          = strings.TrimSpace(body.SkillName)
		workflowKey        = strings.TrimSpace(body.WorkflowKey)
		workflowProvider   = strings.TrimSpace(body.WorkflowProvider)
		workflowDefinition []byte
		title              string
	)
	definition, err := normalizeStudioWorkflowDefinition(body.WorkflowDefinition)
	if err != nil {
		http.Error(w, "invalid workflow_definition: "+err.Error(), http.StatusBadRequest)
		return
	}
	workflowDefinition = definition

	b.mu.Lock()
	if skillName != "" || workflowKey != "" {
		var skill *teamSkill
		if skillName != "" {
			skill = b.findSkillByNameLocked(skillName)
		}
		if skill == nil && workflowKey != "" {
			skill = b.findSkillByWorkflowKeyLocked(workflowKey)
		}
		if skill != nil {
			if skillName == "" {
				skillName = strings.TrimSpace(skill.Name)
			}
			if workflowKey == "" {
				workflowKey = strings.TrimSpace(skill.WorkflowKey)
			}
			if workflowProvider == "" {
				workflowProvider = strings.TrimSpace(skill.WorkflowProvider)
			}
			if len(workflowDefinition) == 0 {
				workflowDefinition = []byte(strings.TrimSpace(skill.WorkflowDefinition))
			}
			title = strings.TrimSpace(skill.Title)
		}
	}
	b.mu.Unlock()

	if workflowKey == "" {
		http.Error(w, "workflow_key required", http.StatusBadRequest)
		return
	}
	if workflowProvider == "" {
		workflowProvider = "one"
	}
	if len(workflowDefinition) == 0 {
		http.Error(w, "workflow_definition required", http.StatusBadRequest)
		return
	}

	inferredDryRun, inferredMock, inferredIntegrations := studioWorkflowHints(workflowDefinition)
	dryRun := inferredDryRun
	if body.DryRun != nil {
		dryRun = *body.DryRun
	}
	mock := inferredMock
	if body.Mock != nil {
		mock = *body.Mock
	}
	integrations := uniqueStrings(body.Integrations, inferredIntegrations)

	providerLabel := workflowProvider
	registry := action.NewRegistryFromEnv()
	provider, err := registry.ProviderNamed(workflowProvider, action.CapabilityWorkflowExecute)
	var execution action.WorkflowExecuteResult
	if err != nil {
		if dryRun || mock {
			execution, err = executeStudioWorkflowStub(workflowKey, workflowDefinition, body.Inputs, dryRun, mock)
			if err != nil {
				http.Error(w, "workflow stub execution failed: "+err.Error(), http.StatusBadGateway)
				return
			}
		} else {
			http.Error(w, "workflow provider unavailable: "+err.Error(), http.StatusBadGateway)
			return
		}
	} else {
		providerLabel = provider.Name()
		if provider.Supports(action.CapabilityWorkflowCreate) {
			if _, err := provider.CreateWorkflow(r.Context(), action.WorkflowCreateRequest{
				Key:        workflowKey,
				Definition: workflowDefinition,
			}); err != nil && !workflowCreateConflict(err) {
				if dryRun || mock {
					execution, err = executeStudioWorkflowStub(workflowKey, workflowDefinition, body.Inputs, dryRun, mock)
					if err != nil {
						http.Error(w, "workflow stub execution failed: "+err.Error(), http.StatusBadGateway)
						return
					}
				} else {
					http.Error(w, "workflow registration failed: "+err.Error(), http.StatusBadGateway)
					return
				}
			}
		}
		if execution.RunID == "" {
			execution, err = provider.ExecuteWorkflow(r.Context(), action.WorkflowExecuteRequest{
				KeyOrPath: workflowKey,
				Inputs:    body.Inputs,
				DryRun:    dryRun,
				Mock:      mock,
				AllowBash: body.AllowBash,
			})
			if err != nil {
				if dryRun || mock {
					execution, err = executeStudioWorkflowStub(workflowKey, workflowDefinition, body.Inputs, dryRun, mock)
					if err != nil {
						http.Error(w, "workflow stub execution failed: "+err.Error(), http.StatusBadGateway)
						return
					}
				} else {
					now := time.Now().UTC()
					mode := workflowRunModeLabel(dryRun, mock)
					retryAt, rateLimited := externalWorkflowRetryAfter(err, now)
					failKind := "external_workflow_failed"
					failStatus := "failed"
					failSummary := truncateSummary(fmt.Sprintf("Studio workflow %s failed via %s (%s)", workflowKey, titleCaser.String(providerLabel), mode), 140)
					if rateLimited {
						failKind = "external_workflow_rate_limited"
						failStatus = "rate_limited"
						failSummary = truncateSummary(fmt.Sprintf("Studio workflow %s rate-limited via %s (%s)", workflowKey, titleCaser.String(providerLabel), mode), 140)
						retryDelay := time.Until(retryAt)
						if retryDelay < time.Second {
							retryDelay = time.Second
						}
						w.Header().Set("Retry-After", strconv.Itoa(int((retryDelay+time.Second-1)/time.Second)))
					}
					_ = b.RecordAction(failKind, providerLabel, channel, actor, failSummary, workflowKey, nil, "")
					_ = b.UpdateSkillExecutionByWorkflowKey(workflowKey, failStatus, now)
					if rateLimited {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusTooManyRequests)
						_ = json.NewEncoder(w).Encode(map[string]any{
							"ok":           false,
							"workflow_key": workflowKey,
							"provider":     providerLabel,
							"status":       "rate_limited",
							"error":        err.Error(),
							"retry_after":  retryAt.UTC().Format(time.RFC3339Nano),
						})
						return
					}
					http.Error(w, "workflow execution failed: "+err.Error(), http.StatusBadGateway)
					return
				}
			}
		}
	}
	now := time.Now().UTC()
	mode := workflowRunModeLabel(dryRun, mock)
	status := strings.TrimSpace(execution.Status)
	if status == "" {
		status = "completed"
	}
	summary := truncateSummary(fmt.Sprintf("Studio workflow %s ran via %s (%s)", workflowKey, titleCaser.String(providerLabel), mode), 140)
	if err := b.RecordAction("external_workflow_executed", providerLabel, channel, actor, summary, workflowKey, nil, ""); err != nil {
		http.Error(w, "failed to record workflow action", http.StatusInternalServerError)
		return
	}
	if err := b.recordStudioWorkflowExecution(channel, actor, skillName, workflowKey, providerLabel, title, status, now); err != nil {
		http.Error(w, "failed to persist workflow execution", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":           true,
		"skill_name":   skillName,
		"workflow_key": workflowKey,
		"provider":     providerLabel,
		"mode":         mode,
		"status":       status,
		"integrations": integrations,
		"execution": map[string]any{
			"run_id":   execution.RunID,
			"log_file": execution.LogFile,
			"status":   status,
			"steps":    execution.Steps,
			"events":   execution.Events,
		},
	})
}

func (b *Broker) handleScheduler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		b.mu.Lock()
		jobs := make([]schedulerJob, 0, len(b.scheduler))
		dueOnly := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("due_only")), "true")
		now := time.Now().UTC()
		for _, job := range b.scheduler {
			if dueOnly && !schedulerJobDue(job, now) {
				continue
			}
			jobs = append(jobs, job)
		}
		b.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jobs": jobs})
	case http.MethodPost:
		var body schedulerJob
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(body.Slug) == "" || strings.TrimSpace(body.Label) == "" {
			http.Error(w, "slug and label required", http.StatusBadRequest)
			return
		}
		if err := b.SetSchedulerJob(body); err != nil {
			http.Error(w, "failed to persist scheduler job", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *Broker) handleBridge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Actor         string   `json:"actor"`
		SourceChannel string   `json:"source_channel"`
		TargetChannel string   `json:"target_channel"`
		Summary       string   `json:"summary"`
		Tagged        []string `json:"tagged"`
		ReplyTo       string   `json:"reply_to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	actor := normalizeActorSlug(body.Actor)
	if actor != "ceo" {
		http.Error(w, "only the CEO can bridge channel context", http.StatusForbidden)
		return
	}
	source := normalizeChannelSlug(body.SourceChannel)
	target := normalizeChannelSlug(body.TargetChannel)
	if source == "" || target == "" {
		http.Error(w, "source_channel and target_channel required", http.StatusBadRequest)
		return
	}
	summary := strings.TrimSpace(body.Summary)
	if summary == "" {
		http.Error(w, "summary required", http.StatusBadRequest)
		return
	}

	b.mu.Lock()
	sourceExists := b.findChannelLocked(source) != nil
	targetExists := b.findChannelLocked(target) != nil
	b.mu.Unlock()
	if !sourceExists || !targetExists {
		http.Error(w, "channel not found", http.StatusNotFound)
		return
	}

	records, err := b.RecordSignals([]officeSignal{{
		ID:         fmt.Sprintf("bridge:%s:%s:%s", source, target, truncateSummary(strings.ToLower(summary), 48)),
		Source:     "channel_bridge",
		Kind:       "bridge",
		Title:      "Cross-channel bridge",
		Content:    fmt.Sprintf("CEO bridged context from #%s to #%s: %s", source, target, summary),
		Channel:    target,
		Owner:      "ceo",
		Confidence: "explicit",
		Urgency:    "normal",
	}})
	if err != nil {
		http.Error(w, "failed to record bridge signal", http.StatusInternalServerError)
		return
	}
	signalIDs := make([]string, 0, len(records))
	for _, record := range records {
		signalIDs = append(signalIDs, record.ID)
	}
	decision, err := b.RecordDecision(
		"bridge_channel",
		target,
		fmt.Sprintf("CEO bridged context from #%s to #%s.", source, target),
		"Relevant context existed in another channel, so the CEO carried it into this channel explicitly.",
		"ceo",
		signalIDs,
		false,
		false,
	)
	if err != nil {
		http.Error(w, "failed to record bridge decision", http.StatusInternalServerError)
		return
	}
	content := summary + fmt.Sprintf("\n\nCEO bridged this context from #%s to help #%s.", source, target)
	msg, _, err := b.PostAutomationMessage(
		"wuphf",
		target,
		"Bridge from #"+source,
		content,
		decision.ID,
		"ceo_bridge",
		"CEO bridge",
		uniqueSlugs(body.Tagged),
		strings.TrimSpace(body.ReplyTo),
	)
	if err != nil {
		http.Error(w, "failed to persist bridge message", http.StatusInternalServerError)
		return
	}
	if err := b.RecordAction("bridge_channel", "ceo_bridge", target, actor, truncateSummary(summary, 140), msg.ID, signalIDs, decision.ID); err != nil {
		http.Error(w, "failed to persist bridge action", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":          msg.ID,
		"decision_id": decision.ID,
		"signal_ids":  signalIDs,
	})
}

func (b *Broker) handleQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(b.QueueSnapshot())
}

func (b *Broker) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.mu.Lock()
	fn := b.sessionObservabilityFn
	b.mu.Unlock()

	snapshot := SessionObservabilitySnapshot{}
	if fn != nil {
		snapshot = fn()
	} else {
		mode, agent := b.SessionModeState()
		snapshot = SessionObservabilitySnapshot{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			SessionMode: mode,
			DirectAgent: agent,
			Provider:    config.ResolveLLMProvider(""),
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snapshot)
}

func logConfigPOST(r *http.Request, rawBody []byte) {
	if strings.TrimSpace(os.Getenv("WUPHF_DEBUG_CONFIG_POSTS")) == "" {
		return
	}
	logDir := filepath.Join(filepath.Dir(config.ConfigPath()), "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return
	}
	path := filepath.Join(logDir, "config-post-debug.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "\n[%s] remote=%s ua=%q\n%s\n", time.Now().Format(time.RFC3339Nano), r.RemoteAddr, r.UserAgent(), strings.TrimSpace(string(rawBody)))
}

func (b *Broker) handleCompany(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, _ := config.Load()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        cfg.CompanyName,
			"description": cfg.CompanyDescription,
			"goals":       cfg.CompanyGoals,
			"size":        cfg.CompanySize,
			"priority":    cfg.CompanyPriority,
		})
	case http.MethodPost:
		var body struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Goals       string `json:"goals"`
			Size        string `json:"size"`
			Priority    string `json:"priority"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		cfg, err := config.Load()
		if err != nil {
			http.Error(w, "failed to read config", http.StatusInternalServerError)
			return
		}
		if body.Name != "" {
			cfg.CompanyName = strings.TrimSpace(body.Name)
		}
		if body.Description != "" {
			cfg.CompanyDescription = strings.TrimSpace(body.Description)
		}
		if body.Goals != "" {
			cfg.CompanyGoals = strings.TrimSpace(body.Goals)
		}
		if body.Size != "" {
			cfg.CompanySize = strings.TrimSpace(body.Size)
		}
		if body.Priority != "" {
			cfg.CompanyPriority = strings.TrimSpace(body.Priority)
		}
		if err := config.Save(cfg); err != nil {
			http.Error(w, "save failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleConfig exposes GET/POST over ~/.wuphf/config.json for the web UI
// settings page and onboarding wizard. All POST fields are optional; clients
// can update one without touching the others. Secret fields (API keys, tokens)
// are returned as boolean flags on GET and accepted as plain values on POST.
func (b *Broker) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := config.Load()
		if err != nil {
			http.Error(w, "failed to read config", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			// Runtime
			"llm_provider":           config.ResolveLLMProvider(""),
			"memory_backend":         config.ResolveMemoryBackend(""),
			"action_provider":        config.ResolveActionProvider(),
			"web_search_provider":    config.ResolveWebSearchProvider(),
			"custom_mcp_config_path": config.ResolveCustomMCPConfigPath(),
			"cloud_backup_provider":  config.ResolveCloudBackupProvider(),
			"cloud_backup_bucket":    config.ResolveCloudBackupBucket(),
			"cloud_backup_prefix":    config.ResolveCloudBackupPrefix(),
			"team_lead_slug":         cfg.TeamLeadSlug,
			"max_concurrent_agents":  cfg.MaxConcurrent,
			"default_format":         config.ResolveFormat(""),
			"default_timeout":        config.ResolveTimeout(""),
			"blueprint":              cfg.ActiveBlueprint(),
			// Workspace
			"email":          cfg.Email,
			"workspace_id":   cfg.WorkspaceID,
			"workspace_slug": cfg.WorkspaceSlug,
			"dev_url":        cfg.DevURL,
			// Company
			"company_name":        cfg.CompanyName,
			"company_description": cfg.CompanyDescription,
			"company_goals":       cfg.CompanyGoals,
			"company_size":        cfg.CompanySize,
			"company_priority":    cfg.CompanyPriority,
			// Polling intervals
			"insights_poll_minutes":  config.ResolveInsightsPollInterval(),
			"task_follow_up_minutes": config.ResolveTaskFollowUpInterval(),
			"task_reminder_minutes":  config.ResolveTaskReminderInterval(),
			"task_recheck_minutes":   config.ResolveTaskRecheckInterval(),
			// Integrations — secret fields as booleans
			"api_key_set":        config.ResolveAPIKey("") != "",
			"openai_key_set":     config.ResolveOpenAIAPIKey() != "",
			"anthropic_key_set":  config.ResolveAnthropicAPIKey() != "",
			"gemini_key_set":     config.ResolveGeminiAPIKey() != "",
			"minimax_key_set":    config.ResolveMinimaxAPIKey() != "",
			"brave_key_set":      config.ResolveBraveAPIKey() != "",
			"one_key_set":        config.ResolveOneSecret() != "",
			"composio_key_set":   config.ResolveComposioAPIKey() != "",
			"telegram_token_set": config.ResolveTelegramBotToken() != "",
			// Config file path (informational)
			"config_path": config.ConfigPath(),
		})
	case http.MethodPost:
		rawBody, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		logConfigPOST(r, rawBody)
		var body struct {
			LLMProvider         *string `json:"llm_provider,omitempty"`
			MemoryBackend       *string `json:"memory_backend,omitempty"`
			ActionProvider      *string `json:"action_provider,omitempty"`
			WebSearchProvider   *string `json:"web_search_provider,omitempty"`
			CustomMCPConfig     *string `json:"custom_mcp_config_path,omitempty"`
			CloudBackupProvider *string `json:"cloud_backup_provider,omitempty"`
			CloudBackupBucket   *string `json:"cloud_backup_bucket,omitempty"`
			CloudBackupPrefix   *string `json:"cloud_backup_prefix,omitempty"`
			TeamLeadSlug        *string `json:"team_lead_slug,omitempty"`
			MaxConcurrent       *int    `json:"max_concurrent_agents,omitempty"`
			DefaultFormat       *string `json:"default_format,omitempty"`
			DefaultTimeout      *int    `json:"default_timeout,omitempty"`
			Blueprint           *string `json:"blueprint,omitempty"`
			Email               *string `json:"email,omitempty"`
			DevURL              *string `json:"dev_url,omitempty"`
			CompanyName         *string `json:"company_name,omitempty"`
			CompanyDesc         *string `json:"company_description,omitempty"`
			CompanyGoals        *string `json:"company_goals,omitempty"`
			CompanySize         *string `json:"company_size,omitempty"`
			CompanyPriority     *string `json:"company_priority,omitempty"`
			InsightsPoll        *int    `json:"insights_poll_minutes,omitempty"`
			TaskFollowUp        *int    `json:"task_follow_up_minutes,omitempty"`
			TaskReminder        *int    `json:"task_reminder_minutes,omitempty"`
			TaskRecheck         *int    `json:"task_recheck_minutes,omitempty"`
			// Secret fields
			APIKey          *string `json:"api_key,omitempty"`
			OpenAIAPIKey    *string `json:"openai_api_key,omitempty"`
			AnthropicAPIKey *string `json:"anthropic_api_key,omitempty"`
			GeminiAPIKey    *string `json:"gemini_api_key,omitempty"`
			MinimaxAPIKey   *string `json:"minimax_api_key,omitempty"`
			BraveAPIKey     *string `json:"brave_api_key,omitempty"`
			OneAPIKey       *string `json:"one_api_key,omitempty"`
			ComposioAPIKey  *string `json:"composio_api_key,omitempty"`
			TelegramToken   *string `json:"telegram_bot_token,omitempty"`
		}
		if err := json.NewDecoder(bytes.NewReader(rawBody)).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Validate enum fields before touching config.
		var provider string
		if body.LLMProvider != nil {
			provider = strings.TrimSpace(strings.ToLower(*body.LLMProvider))
			switch provider {
			case "claude-code", "codex", "gemini", "gemini-vertex", "ollama":
				// ok
			default:
				http.Error(w, "unsupported llm_provider", http.StatusBadRequest)
				return
			}
		}
		var memory string
		if body.MemoryBackend != nil {
			memory = config.NormalizeMemoryBackend(*body.MemoryBackend)
			if memory == "" {
				http.Error(w, "unsupported memory_backend", http.StatusBadRequest)
				return
			}
		}
		var webSearchProvider string
		if body.WebSearchProvider != nil {
			webSearchProvider = config.NormalizeWebSearchProvider(*body.WebSearchProvider)
			if strings.TrimSpace(*body.WebSearchProvider) != "" && webSearchProvider == "" {
				http.Error(w, "unsupported web_search_provider", http.StatusBadRequest)
				return
			}
		}
		var cloudBackupProvider string
		if body.CloudBackupProvider != nil {
			cloudBackupProvider = config.NormalizeCloudBackupProvider(*body.CloudBackupProvider)
			if cloudBackupProvider == "" {
				http.Error(w, "unsupported cloud_backup_provider", http.StatusBadRequest)
				return
			}
		}

		cfg, err := config.Load()
		if err != nil {
			http.Error(w, "failed to read config", http.StatusInternalServerError)
			return
		}
		changed := false

		// Enum/string fields
		if provider != "" {
			cfg.LLMProvider = provider
			changed = true
		}
		if body.MemoryBackend != nil {
			cfg.MemoryBackend = config.MemoryBackendNone
			changed = true
		}
		if body.ActionProvider != nil {
			apRaw := strings.TrimSpace(*body.ActionProvider)
			ap := config.NormalizeActionProvider(apRaw)
			switch {
			case apRaw == "":
				cfg.ActionProvider = ""
				changed = true
			case ap != "":
				cfg.ActionProvider = ap
				changed = true
			default:
				http.Error(w, "unsupported action_provider", http.StatusBadRequest)
				return
			}
		}
		if body.WebSearchProvider != nil {
			cfg.WebSearchProvider = webSearchProvider
			changed = true
		}
		if body.CustomMCPConfig != nil {
			cfg.CustomMCPConfig = strings.TrimSpace(*body.CustomMCPConfig)
			changed = true
		}
		if body.CloudBackupProvider != nil {
			cfg.CloudBackupProvider = cloudBackupProvider
			changed = true
		}
		if body.CloudBackupBucket != nil {
			cfg.CloudBackupBucket = strings.TrimSpace(*body.CloudBackupBucket)
			changed = true
		}
		if body.CloudBackupPrefix != nil {
			cfg.CloudBackupPrefix = strings.Trim(strings.ReplaceAll(strings.TrimSpace(*body.CloudBackupPrefix), "\\", "/"), "/")
			changed = true
		}
		if body.TeamLeadSlug != nil {
			cfg.TeamLeadSlug = strings.TrimSpace(*body.TeamLeadSlug)
			changed = true
		}
		if body.MaxConcurrent != nil {
			cfg.MaxConcurrent = *body.MaxConcurrent
			changed = true
		}
		if body.DefaultFormat != nil {
			cfg.DefaultFormat = strings.TrimSpace(*body.DefaultFormat)
			changed = true
		}
		if body.DefaultTimeout != nil {
			cfg.DefaultTimeout = *body.DefaultTimeout
			changed = true
		}
		if body.Blueprint != nil {
			cfg.SetActiveBlueprint(*body.Blueprint)
			changed = true
		}
		if body.Email != nil {
			cfg.Email = strings.TrimSpace(*body.Email)
			changed = true
		}
		if body.DevURL != nil {
			cfg.DevURL = strings.TrimSpace(*body.DevURL)
			changed = true
		}
		// Company
		if body.CompanyName != nil {
			cfg.CompanyName = strings.TrimSpace(*body.CompanyName)
			changed = true
		}
		if body.CompanyDesc != nil {
			cfg.CompanyDescription = strings.TrimSpace(*body.CompanyDesc)
			changed = true
		}
		if body.CompanyGoals != nil {
			cfg.CompanyGoals = strings.TrimSpace(*body.CompanyGoals)
			changed = true
		}
		if body.CompanySize != nil {
			cfg.CompanySize = strings.TrimSpace(*body.CompanySize)
			changed = true
		}
		if body.CompanyPriority != nil {
			cfg.CompanyPriority = strings.TrimSpace(*body.CompanyPriority)
			changed = true
		}
		// Polling intervals (minimum 2 minutes, matching resolve functions)
		if body.InsightsPoll != nil {
			if *body.InsightsPoll < 2 {
				http.Error(w, "insights_poll_minutes must be >= 2", http.StatusBadRequest)
				return
			}
			cfg.InsightsPollMinutes = *body.InsightsPoll
			changed = true
		}
		if body.TaskFollowUp != nil {
			if *body.TaskFollowUp < 2 {
				http.Error(w, "task_follow_up_minutes must be >= 2", http.StatusBadRequest)
				return
			}
			cfg.TaskFollowUpMinutes = *body.TaskFollowUp
			changed = true
		}
		if body.TaskReminder != nil {
			if *body.TaskReminder < 2 {
				http.Error(w, "task_reminder_minutes must be >= 2", http.StatusBadRequest)
				return
			}
			cfg.TaskReminderMinutes = *body.TaskReminder
			changed = true
		}
		if body.TaskRecheck != nil {
			if *body.TaskRecheck < 2 {
				http.Error(w, "task_recheck_minutes must be >= 2", http.StatusBadRequest)
				return
			}
			cfg.TaskRecheckMinutes = *body.TaskRecheck
			changed = true
		}
		// Secret fields
		if body.APIKey != nil {
			cfg.APIKey = strings.TrimSpace(*body.APIKey)
			changed = true
		}
		if body.OpenAIAPIKey != nil {
			cfg.OpenAIAPIKey = strings.TrimSpace(*body.OpenAIAPIKey)
			changed = true
		}
		if body.AnthropicAPIKey != nil {
			cfg.AnthropicAPIKey = strings.TrimSpace(*body.AnthropicAPIKey)
			changed = true
		}
		if body.GeminiAPIKey != nil {
			cfg.GeminiAPIKey = strings.TrimSpace(*body.GeminiAPIKey)
			changed = true
		}
		if body.MinimaxAPIKey != nil {
			cfg.MinimaxAPIKey = strings.TrimSpace(*body.MinimaxAPIKey)
			changed = true
		}
		if body.BraveAPIKey != nil {
			cfg.BraveAPIKey = strings.TrimSpace(*body.BraveAPIKey)
			changed = true
		}
		if body.OneAPIKey != nil {
			cfg.OneAPIKey = strings.TrimSpace(*body.OneAPIKey)
			changed = true
		}
		if body.ComposioAPIKey != nil {
			cfg.ComposioAPIKey = strings.TrimSpace(*body.ComposioAPIKey)
			changed = true
		}
		if body.TelegramToken != nil {
			cfg.TelegramBotToken = strings.TrimSpace(*body.TelegramToken)
			changed = true
		}

		if !changed {
			http.Error(w, "no fields to update", http.StatusBadRequest)
			return
		}

		if err := config.Save(cfg); err != nil {
			http.Error(w, "save failed", http.StatusInternalServerError)
			return
		}
		// Keep /health in sync for this process so the wizard choice is
		// reflected immediately without requiring a broker restart.
		if provider != "" {
			b.mu.Lock()
			b.runtimeProvider = provider
			b.mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *Broker) handleOfficeMembers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		b.mu.Lock()
		members := make([]officeMember, len(b.members))
		copy(members, b.members)
		b.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"members": members})
	case http.MethodPost:
		var body struct {
			Action         string                    `json:"action"`
			Slug           string                    `json:"slug"`
			Name           string                    `json:"name"`
			Role           string                    `json:"role"`
			Expertise      []string                  `json:"expertise"`
			Personality    string                    `json:"personality"`
			PermissionMode string                    `json:"permission_mode"`
			AllowedTools   []string                  `json:"allowed_tools"`
			CreatedBy      string                    `json:"created_by"`
			Provider       *provider.ProviderBinding `json:"provider,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		action := strings.TrimSpace(body.Action)
		slug := normalizeChannelSlug(body.Slug)
		if slug == "" {
			http.Error(w, "slug required", http.StatusBadRequest)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		actor := firstNonEmpty(strings.TrimSpace(body.CreatedBy), "studio")

		b.mu.Lock()
		defer b.mu.Unlock()
		switch action {
		case "create":
			if b.findMemberLocked(slug) != nil {
				http.Error(w, "member already exists", http.StatusConflict)
				return
			}
			if body.Provider != nil {
				if err := provider.ValidateKind(body.Provider.Kind); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			}
			member := officeMember{
				Slug:           slug,
				Name:           strings.TrimSpace(body.Name),
				Role:           strings.TrimSpace(body.Role),
				Expertise:      normalizeStringList(body.Expertise),
				Personality:    strings.TrimSpace(body.Personality),
				PermissionMode: strings.TrimSpace(body.PermissionMode),
				AllowedTools:   normalizeStringList(body.AllowedTools),
				CreatedBy:      strings.TrimSpace(body.CreatedBy),
				CreatedAt:      now,
			}
			if body.Provider != nil {
				member.Provider = *body.Provider
			}
			applyOfficeMemberDefaults(&member)

			b.members = append(b.members, member)
			b.memberIndex[member.Slug] = len(b.members) - 1
			if err := b.saveLocked(); err != nil {
				http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
				return
			}
			b.publishOfficeChangeLocked(officeChangeEvent{Kind: "member_created", Slug: slug})
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"member": member})
		case "update":
			member := b.findMemberLocked(slug)
			if member == nil {
				http.Error(w, "member not found", http.StatusNotFound)
				return
			}
			providerChanged := false
			if body.Name != "" {
				member.Name = strings.TrimSpace(body.Name)
			}
			if body.Role != "" {
				member.Role = strings.TrimSpace(body.Role)
			}
			if body.Expertise != nil {
				member.Expertise = normalizeStringList(body.Expertise)
			}
			if body.Personality != "" {
				member.Personality = strings.TrimSpace(body.Personality)
			}
			if body.PermissionMode != "" {
				member.PermissionMode = strings.TrimSpace(body.PermissionMode)
			}
			if body.AllowedTools != nil {
				member.AllowedTools = normalizeStringList(body.AllowedTools)
			}
			if body.Provider != nil {
				if err := provider.ValidateKind(body.Provider.Kind); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				nextBinding := normalizeProviderBinding(*body.Provider)
				providerChanged = providerBindingChanged(member.Provider, nextBinding)
				member.Provider = nextBinding
			}
			applyOfficeMemberDefaults(member)
			if providerChanged {
				if err := b.clearOperationalBlocksForProviderChangeLocked(slug, actor); err != nil {
					http.Error(w, "failed to clear runtime blocks", http.StatusInternalServerError)
					return
				}
			}
			if err := b.saveLocked(); err != nil {
				http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
				return
			}
			if providerChanged {
				if err := persistManifestMemberProvider(slug, member.Provider); err != nil {
					http.Error(w, "failed to persist manifest provider", http.StatusInternalServerError)
					return
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"member": member})
		case "remove":
			member := b.findMemberLocked(slug)
			if member == nil {
				http.Error(w, "member not found", http.StatusNotFound)
				return
			}
			if member.BuiltIn || slug == "ceo" {
				http.Error(w, "cannot remove built-in member", http.StatusBadRequest)
				return
			}
			filteredMembers := b.members[:0]
			for _, existing := range b.members {
				if existing.Slug != slug {
					filteredMembers = append(filteredMembers, existing)
				}
			}
			b.members = filteredMembers
			b.rebuildMemberIndexLocked()
			for i := range b.channels {
				nextMembers := b.channels[i].Members[:0]
				for _, existing := range b.channels[i].Members {
					if existing != slug {
						nextMembers = append(nextMembers, existing)
					}
				}
				b.channels[i].Members = nextMembers
				nextDisabled := b.channels[i].Disabled[:0]
				for _, existing := range b.channels[i].Disabled {
					if existing != slug {
						nextDisabled = append(nextDisabled, existing)
					}
				}
				b.channels[i].Disabled = nextDisabled
				b.channels[i].UpdatedAt = now
			}
			for i := range b.tasks {
				if b.tasks[i].Owner == slug {
					b.tasks[i].Owner = ""
					b.tasks[i].Status = "open"
					b.tasks[i].UpdatedAt = now
				}
			}
			if err := b.saveLocked(); err != nil {
				http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
				return
			}
			b.publishOfficeChangeLocked(officeChangeEvent{Kind: "member_removed", Slug: slug})
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.Error(w, "unknown action", http.StatusBadRequest)
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *Broker) handleGenerateMember(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if b.generateMemberFn == nil {
		http.Error(w, "generate not available", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	prompt := strings.TrimSpace(body.Prompt)
	if prompt == "" {
		http.Error(w, "prompt required", http.StatusBadRequest)
		return
	}
	tmpl, err := b.generateMemberFn(prompt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tmpl)
}

func (b *Broker) handleGenerateChannel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if b.generateChannelFn == nil {
		http.Error(w, "generate not available", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	prompt := strings.TrimSpace(body.Prompt)
	if prompt == "" {
		http.Error(w, "prompt required", http.StatusBadRequest)
		return
	}
	tmpl, err := b.generateChannelFn(prompt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tmpl)
}

func (b *Broker) handleChannels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		typeFilter := r.URL.Query().Get("type") // "dm" to see DMs, default excludes them
		includeArchived := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_archived")), "true")
		b.mu.Lock()
		channels := make([]teamChannel, 0, len(b.channels))
		for _, ch := range b.channels {
			if ch.Archived && !includeArchived {
				continue
			}
			if typeFilter == "dm" {
				if ch.isDM() {
					channels = append(channels, ch)
				}
			} else {
				// Default: only return real channels, never DMs
				if !ch.isDM() {
					channels = append(channels, ch)
				}
			}
		}
		b.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"channels": channels})
	case http.MethodPost:
		var body struct {
			Action      string          `json:"action"`
			Slug        string          `json:"slug"`
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Members     []string        `json:"members"`
			CreatedBy   string          `json:"created_by"`
			Archived    *bool           `json:"archived,omitempty"`
			Confirm     string          `json:"confirm,omitempty"`
			Force       bool            `json:"force,omitempty"`
			Purge       bool            `json:"purge,omitempty"`
			Surface     *channelSurface `json:"surface,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		action := strings.TrimSpace(body.Action)
		slug := normalizeChannelSlug(body.Slug)
		now := time.Now().UTC().Format(time.RFC3339)
		b.mu.Lock()
		defer b.mu.Unlock()
		validateMembers := func(members []string) ([]string, string) {
			members = uniqueSlugs(members)
			if len(members) == 0 {
				return nil, ""
			}
			validated := make([]string, 0, len(members))
			var missing []string
			for _, member := range members {
				if b.findMemberLocked(member) == nil {
					missing = append(missing, member)
					continue
				}
				validated = append(validated, member)
			}
			return validated, strings.Join(missing, ", ")
		}
		switch action {
		case "create":
			if slug == "" {
				http.Error(w, "slug required", http.StatusBadRequest)
				return
			}
			if b.findChannelLocked(slug) != nil {
				http.Error(w, "channel already exists", http.StatusConflict)
				return
			}
			members, missing := validateMembers(body.Members)
			if missing != "" {
				http.Error(w, "unknown members: "+missing, http.StatusNotFound)
				return
			}
			members = append([]string{"ceo"}, members...)
			if creator := normalizeChannelSlug(body.CreatedBy); creator != "" && creator != "ceo" && b.findMemberLocked(creator) != nil {
				members = append(members, creator)
			}
			ch := teamChannel{
				Slug:        slug,
				Name:        strings.TrimSpace(body.Name),
				Description: strings.TrimSpace(body.Description),
				Protected:   shouldProtectUserChannel(slug, body.CreatedBy),
				Members:     uniqueSlugs(members),
				Surface:     body.Surface,
				CreatedBy:   strings.TrimSpace(body.CreatedBy),
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if ch.Name == "" {
				ch.Name = slug
			}
			if ch.Description == "" {
				ch.Description = defaultTeamChannelDescription(ch.Slug, ch.Name)
			}
			b.channels = append(b.channels, ch)
			b.upsertPublicChannelInStoreLocked(ch)
			if err := b.saveLocked(); err != nil {
				http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
				return
			}
			b.publishOfficeChangeLocked(officeChangeEvent{Kind: "channel_created", Slug: slug})
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"channel": ch})
		case "update":
			if slug == "" {
				http.Error(w, "slug required", http.StatusBadRequest)
				return
			}
			ch := b.findChannelLocked(slug)
			if ch == nil {
				http.Error(w, "channel not found", http.StatusNotFound)
				return
			}
			if name := strings.TrimSpace(body.Name); name != "" {
				ch.Name = name
			}
			if description := strings.TrimSpace(body.Description); description != "" {
				ch.Description = description
			}
			if body.Surface != nil {
				ch.Surface = body.Surface
			}
			if body.Archived != nil {
				ch.Archived = *body.Archived
			}
			if body.Members != nil {
				if ch.isDM() {
					ch.Members = canonicalDMMembers(ch.Slug)
				} else {
					members, missing := validateMembers(body.Members)
					if missing != "" {
						http.Error(w, "unknown members: "+missing, http.StatusNotFound)
						return
					}
					ch.Members = uniqueSlugs(append([]string{"ceo"}, members...))
				}
				if len(ch.Disabled) > 0 {
					filtered := make([]string, 0, len(ch.Disabled))
					for _, disabled := range ch.Disabled {
						if !containsString(ch.Members, disabled) {
							filtered = append(filtered, disabled)
						}
					}
					ch.Disabled = filtered
				}
			}
			ch.UpdatedAt = now
			if ch.Archived {
				b.removePublicChannelFromStoreLocked(ch.Slug)
			} else {
				b.upsertPublicChannelInStoreLocked(*ch)
			}
			if err := b.saveLocked(); err != nil {
				http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
				return
			}
			b.publishOfficeChangeLocked(officeChangeEvent{Kind: "channel_updated", Slug: slug})
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"channel": ch})
		case "remove":
			if slug == "" || slug == "general" {
				http.Error(w, "cannot remove channel", http.StatusBadRequest)
				return
			}
			idx := -1
			for i := range b.channels {
				if b.channels[i].Slug == slug {
					idx = i
					break
				}
			}
			if idx == -1 {
				http.Error(w, "channel not found", http.StatusNotFound)
				return
			}
			if b.channels[idx].Protected && !body.Force {
				http.Error(w, "cannot remove protected channel", http.StatusForbidden)
				return
			}
			if b.channels[idx].Protected && normalizeRemovalConfirmToken(body.Confirm) != slug {
				http.Error(w, "protected channel removal requires explicit confirmation. Use confirm:<slug>", http.StatusBadRequest)
				return
			}
			if body.Purge {
				b.channels = append(b.channels[:idx], b.channels[idx+1:]...)
				b.purgeChannelStateLocked(slug)
				b.removePublicChannelFromStoreLocked(slug)
			} else {
				b.channels[idx].Archived = true
				b.channels[idx].UpdatedAt = now
				b.removePublicChannelFromStoreLocked(slug)
			}
			if err := b.saveLocked(); err != nil {
				http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
				return
			}
			if body.Purge {
				b.publishOfficeChangeLocked(officeChangeEvent{Kind: "channel_removed", Slug: slug})
			} else {
				b.publishOfficeChangeLocked(officeChangeEvent{Kind: "channel_updated", Slug: slug})
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"purged": body.Purge,
			})
		default:
			http.Error(w, "unknown action", http.StatusBadRequest)
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleCreateDM creates or returns an existing DM channel.
// POST /channels/dm — body: {members: ["human", "engineering"], type: "direct"|"group"}
func (b *Broker) handleCreateDM(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Members []string `json:"members"`
		Type    string   `json:"type"` // "direct" or "group"
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if len(body.Members) < 2 {
		http.Error(w, "at least 2 members required", http.StatusBadRequest)
		return
	}
	// Validate: at least one member must be "human" (no agent-to-agent DMs).
	hasHuman := false
	for _, m := range body.Members {
		if m == "human" || m == "you" {
			hasHuman = true
			break
		}
	}
	if !hasHuman {
		http.Error(w, "DM must include a human member; agent-to-agent DMs are not allowed", http.StatusBadRequest)
		return
	}

	if b.channelStore == nil {
		http.Error(w, "channel store not initialized", http.StatusInternalServerError)
		return
	}

	var (
		ch      *channel.Channel
		err     error
		created bool
	)
	dmType := strings.TrimSpace(strings.ToLower(body.Type))
	if dmType == "group" && len(body.Members) > 2 {
		existing, ok := b.channelStore.FindDirectByMembers(body.Members[0], body.Members[1])
		if !ok || existing == nil {
			created = true
		}
		ch, err = b.channelStore.GetOrCreateGroup(body.Members, "human")
	} else {
		// Default: direct (1:1). For >2 members use group.
		if len(body.Members) > 2 {
			existing, ok := b.channelStore.FindDirectByMembers(body.Members[0], body.Members[1])
			if !ok || existing == nil {
				created = true
			}
			ch, err = b.channelStore.GetOrCreateGroup(body.Members, "human")
		} else {
			// Normalize: find the non-human member for the slug.
			agentSlug := ""
			for _, m := range body.Members {
				if m != "human" && m != "you" {
					agentSlug = m
					break
				}
			}
			if agentSlug == "" {
				http.Error(w, "could not determine agent member", http.StatusBadRequest)
				return
			}
			_, exists := b.channelStore.FindDirectByMembers("human", agentSlug)
			created = !exists
			ch, err = b.channelStore.GetOrCreateDirect("human", agentSlug)
		}
	}
	if err != nil {
		http.Error(w, "failed to create DM: "+err.Error(), http.StatusInternalServerError)
		return
	}

	b.mu.Lock()
	_ = b.saveLocked()
	b.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":      ch.ID,
		"slug":    ch.Slug,
		"type":    ch.Type,
		"name":    ch.Name,
		"created": created,
	})
}

func (b *Broker) handleChannelMembers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
		b.mu.Lock()
		ch := b.findChannelLocked(channel)
		if ch == nil {
			b.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"channel": channel, "members": []map[string]any{}})
			return
		}
		memberInfo := make([]map[string]any, 0, len(ch.Members))
		for _, member := range ch.Members {
			memberInfo = append(memberInfo, map[string]any{
				"slug":     member,
				"disabled": !b.channelMemberEnabledLocked(channel, member),
			})
		}
		b.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"channel": channel, "members": memberInfo})
	case http.MethodPost:
		var body struct {
			Channel string `json:"channel"`
			Action  string `json:"action"`
			Slug    string `json:"slug"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		channel := normalizeChannelSlug(body.Channel)
		member := normalizeChannelSlug(body.Slug)
		action := strings.TrimSpace(body.Action)
		if member == "" {
			http.Error(w, "slug required", http.StatusBadRequest)
			return
		}
		b.mu.Lock()
		ch := b.findChannelLocked(channel)
		if ch == nil {
			b.mu.Unlock()
			http.Error(w, "channel not found", http.StatusNotFound)
			return
		}
		if b.findMemberLocked(member) == nil {
			b.mu.Unlock()
			http.Error(w, "member not found", http.StatusNotFound)
			return
		}
		if member == "ceo" && (action == "remove" || action == "disable") {
			b.mu.Unlock()
			http.Error(w, "cannot remove or disable CEO", http.StatusBadRequest)
			return
		}
		switch action {
		case "add":
			ch.Members = uniqueSlugs(append(ch.Members, member))
		case "remove":
			filtered := ch.Members[:0]
			for _, existing := range ch.Members {
				if existing != member {
					filtered = append(filtered, existing)
				}
			}
			ch.Members = filtered
			disabled := ch.Disabled[:0]
			for _, existing := range ch.Disabled {
				if existing != member {
					disabled = append(disabled, existing)
				}
			}
			ch.Disabled = disabled
		case "disable":
			if !b.channelHasMemberLocked(channel, member) {
				ch.Members = uniqueSlugs(append(ch.Members, member))
			}
			ch.Disabled = uniqueSlugs(append(ch.Disabled, member))
		case "enable":
			filtered := ch.Disabled[:0]
			for _, existing := range ch.Disabled {
				if existing != member {
					filtered = append(filtered, existing)
				}
			}
			ch.Disabled = filtered
		default:
			b.mu.Unlock()
			http.Error(w, "unknown action", http.StatusBadRequest)
			return
		}
		ch.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := b.saveLocked(); err != nil {
			b.mu.Unlock()
			http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
			return
		}
		state := map[string]any{
			"channel":  ch.Slug,
			"members":  ch.Members,
			"disabled": ch.Disabled,
		}
		b.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(state)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *Broker) NotificationCursor() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.notificationSince
}

func (b *Broker) SetNotificationCursor(cursor string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if cursor == "" || cursor == b.notificationSince {
		return nil
	}
	b.notificationSince = cursor
	return b.saveLocked()
}

func (b *Broker) InsightsCursor() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.insightsSince
}

func (b *Broker) SetInsightsCursor(cursor string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if cursor == "" || cursor == b.insightsSince {
		return nil
	}
	b.insightsSince = cursor
	return b.saveLocked()
}

func (b *Broker) handleMessages(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		b.handlePostMessage(w, r)
	case http.MethodGet:
		b.handleGetMessages(w, r)
	case http.MethodDelete:
		b.handleDeleteMessage(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *Broker) handleOTLPLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	events := parseOTLPUsageEvents(payload)
	b.mu.Lock()
	for _, event := range events {
		if strings.TrimSpace(event.AgentSlug) == "" {
			continue
		}
		b.recordUsageLocked(event)
	}
	if err := b.saveLocked(); err != nil {
		b.mu.Unlock()
		http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
		return
	}
	b.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"accepted": len(events)})
}

func (b *Broker) handleNexNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Channel     string   `json:"channel"`
		EventID     string   `json:"event_id"`
		Title       string   `json:"title"`
		Content     string   `json:"content"`
		Tagged      []string `json:"tagged"`
		ReplyTo     string   `json:"reply_to"`
		Source      string   `json:"source"`
		SourceLabel string   `json:"source_label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	msg, duplicate, err := b.PostAutomationMessage("nex", body.Channel, body.Title, body.Content, body.EventID, body.Source, body.SourceLabel, body.Tagged, body.ReplyTo)
	if err != nil {
		http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":        msg.ID,
		"duplicate": duplicate,
	})
}

type usageEvent struct {
	AgentSlug           string
	InputTokens         int
	OutputTokens        int
	CacheReadTokens     int
	CacheCreationTokens int
	CostUsd             float64
}

const messageUsageAttachMaxAge = 15 * time.Minute

func (b *Broker) recordUsageLocked(event usageEvent) {
	if b.usage.Agents == nil {
		b.usage.Agents = make(map[string]usageTotals)
	}
	if b.usage.Since == "" {
		b.usage.Since = time.Now().UTC().Format(time.RFC3339)
	}
	agentTotal := b.usage.Agents[event.AgentSlug]
	applyUsageEvent(&agentTotal, event)
	b.usage.Agents[event.AgentSlug] = agentTotal

	session := b.usage.Session
	applyUsageEvent(&session, event)
	b.usage.Session = session

	total := b.usage.Total
	applyUsageEvent(&total, event)
	b.usage.Total = total
	b.attachUsageToRecentMessagesLocked(event)
}

func applyUsageEvent(dst *usageTotals, event usageEvent) {
	dst.InputTokens += event.InputTokens
	dst.OutputTokens += event.OutputTokens
	dst.CacheReadTokens += event.CacheReadTokens
	dst.CacheCreationTokens += event.CacheCreationTokens
	dst.TotalTokens += event.InputTokens + event.OutputTokens + event.CacheReadTokens + event.CacheCreationTokens
	dst.CostUsd += event.CostUsd
	dst.Requests++
}

func usageEventToMessageUsage(event usageEvent) *messageUsage {
	total := event.InputTokens + event.OutputTokens + event.CacheReadTokens + event.CacheCreationTokens
	if total == 0 {
		return nil
	}
	return &messageUsage{
		InputTokens:         event.InputTokens,
		OutputTokens:        event.OutputTokens,
		CacheReadTokens:     event.CacheReadTokens,
		CacheCreationTokens: event.CacheCreationTokens,
		TotalTokens:         total,
	}
}

func cloneMessageUsage(src *messageUsage) *messageUsage {
	if src == nil {
		return nil
	}
	cp := *src
	return &cp
}

func messageIsWithinUsageAttachWindow(timestamp string, now time.Time) bool {
	ts := strings.TrimSpace(timestamp)
	if ts == "" {
		return true
	}
	parsed, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			return true
		}
	}
	return now.Sub(parsed) <= messageUsageAttachMaxAge
}

func (b *Broker) attachUsageToRecentMessagesLocked(event usageEvent) {
	usage := usageEventToMessageUsage(event)
	if usage == nil {
		return
	}
	slug := strings.TrimSpace(event.AgentSlug)
	if slug == "" {
		return
	}
	now := time.Now().UTC()
	for i := len(b.messages) - 1; i >= 0; i-- {
		msg := &b.messages[i]
		if strings.TrimSpace(msg.From) != slug {
			continue
		}
		if msg.Usage != nil {
			break
		}
		if !messageIsWithinUsageAttachWindow(msg.Timestamp, now) {
			break
		}
		msg.Usage = cloneMessageUsage(usage)
	}
}

// RecordAgentUsage records token usage from a provider stream result for a given agent.
func (b *Broker) RecordAgentUsage(slug, model string, usage provider.ClaudeUsage) {
	event := usageEvent{
		AgentSlug:           slug,
		InputTokens:         usage.InputTokens,
		OutputTokens:        usage.OutputTokens,
		CacheReadTokens:     usage.CacheReadTokens,
		CacheCreationTokens: usage.CacheCreationTokens,
		CostUsd:             usage.CostUSD,
	}
	b.mu.Lock()
	b.recordUsageLocked(event)
	_ = b.saveLocked()
	b.mu.Unlock()
}

func parseOTLPUsageEvents(payload map[string]any) []usageEvent {
	resourceLogs, _ := payload["resourceLogs"].([]any)
	var events []usageEvent
	for _, resourceLog := range resourceLogs {
		resourceMap, _ := resourceLog.(map[string]any)
		resourceAttrs := otlpAttributesMap(nestedMap(resourceMap, "resource"))
		scopeLogs, _ := resourceMap["scopeLogs"].([]any)
		for _, scopeLog := range scopeLogs {
			scopeMap, _ := scopeLog.(map[string]any)
			logRecords, _ := scopeMap["logRecords"].([]any)
			for _, logRecord := range logRecords {
				recordMap, _ := logRecord.(map[string]any)
				attrs := otlpAttributesMap(recordMap)
				for k, v := range resourceAttrs {
					if _, exists := attrs[k]; !exists {
						attrs[k] = v
					}
				}
				if attrs["event.name"] != "api_request" && attrs["event_name"] != "api_request" {
					continue
				}
				slug := attrs["agent.slug"]
				if slug == "" {
					slug = attrs["agent_slug"]
				}
				if slug == "" {
					continue
				}
				events = append(events, usageEvent{
					AgentSlug:           slug,
					InputTokens:         otlpIntValue(attrs["input_tokens"]),
					OutputTokens:        otlpIntValue(attrs["output_tokens"]),
					CacheReadTokens:     otlpIntValue(attrs["cache_read_tokens"]),
					CacheCreationTokens: otlpIntValue(attrs["cache_creation_tokens"]),
					CostUsd:             otlpFloatValue(attrs["cost_usd"]),
				})
			}
		}
	}
	return events
}

func nestedMap(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	child, _ := m[key].(map[string]any)
	return child
}

func otlpAttributesMap(record map[string]any) map[string]string {
	out := make(map[string]string)
	if record == nil {
		return out
	}
	attrs, _ := record["attributes"].([]any)
	for _, attr := range attrs {
		attrMap, _ := attr.(map[string]any)
		key, _ := attrMap["key"].(string)
		if key == "" {
			continue
		}
		out[key] = otlpAnyValue(attrMap["value"])
	}
	return out
}

func otlpAnyValue(raw any) string {
	valMap, _ := raw.(map[string]any)
	for _, key := range []string{"stringValue", "intValue", "doubleValue", "boolValue"} {
		if value, ok := valMap[key]; ok {
			return fmt.Sprintf("%v", value)
		}
	}
	return ""
}

func otlpIntValue(raw string) int {
	if raw == "" {
		return 0
	}
	n, _ := strconv.Atoi(raw)
	return n
}

func otlpFloatValue(raw string) float64 {
	if raw == "" {
		return 0
	}
	v, _ := strconv.ParseFloat(raw, 64)
	return v
}

func (b *Broker) handlePostMessage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From     string   `json:"from"`
		Channel  string   `json:"channel"`
		Kind     string   `json:"kind"`
		Title    string   `json:"title"`
		Content  string   `json:"content"`
		Tagged   []string `json:"tagged"`
		ReplyTo  string   `json:"reply_to"`
		ClientID string   `json:"client_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	b.mu.Lock()
	if firstBlockingRequest(b.requests) != nil {
		b.mu.Unlock()
		http.Error(w, "request pending; answer required before chat resumes", http.StatusConflict)
		return
	}

	b.counter++
	channel := normalizeChannelSlug(body.Channel)
	if channel == "" {
		channel = "general"
	}
	// Auto-create DM conversations on first message (like Slack's conversations.open)
	if b.findChannelLocked(channel) == nil {
		if IsDMSlug(channel) {
			b.ensureDMConversationLocked(channel)
		} else if b.channelStore != nil {
			if _, ok := b.channelStore.GetBySlug(channel); !ok {
				b.mu.Unlock()
				http.Error(w, "channel not found", http.StatusNotFound)
				return
			}
		} else {
			b.mu.Unlock()
			http.Error(w, "channel not found", http.StatusNotFound)
			return
		}
	}
	if !b.canAccessChannelLocked(body.From, channel) {
		b.mu.Unlock()
		http.Error(w, "channel access denied", http.StatusForbidden)
		return
	}
	if !isHumanLikeActor(body.From) && !isSystemActor(body.From) && messageContentLooksLikeDisallowedAgentChannelContent(body.Content) {
		b.mu.Unlock()
		http.Error(w, "non-substantive agent chatter is not allowed in channel messages; publish only the substantive result", http.StatusConflict)
		return
	}
	if err := b.validateTaskStateClaimLocked(channel, body.Content); err != nil {
		b.mu.Unlock()
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	tagged := b.normalizeTaggedSlugsLocked(body.From, body.Tagged, body.Content)
	for _, taggedSlug := range tagged {
		switch taggedSlug {
		case "you", "human", "system":
			continue
		}
		if b.findMemberLocked(taggedSlug) == nil {
			b.mu.Unlock()
			http.Error(w, "unknown tagged member", http.StatusBadRequest)
			return
		}
	}

	// Thread auto-tagging: when a HUMAN replies in a thread, notify all
	// other agents who have already participated. This keeps the team
	// aligned without requiring the human to re-tag on every reply.
	// Agent-to-agent auto-tagging is intentionally skipped: focus mode
	// routing (specialist → lead only) already handles that path, and
	// auto-tagging agent replies causes broadcast loops.
	replyTo := strings.TrimSpace(body.ReplyTo)
	clientID := strings.TrimSpace(body.ClientID)
	if duplicate := b.findMessageByClientIDLocked(body.From, channel, clientID); duplicate != nil {
		b.mu.Unlock()
		b.respondPostMessage(w, *duplicate, len(b.messages), true)
		return
	}
	isHumanSender := body.From == "you" || body.From == "human"
	if replyTo != "" && isHumanSender {
		threadRoot := replyTo
		threadParticipants := []string{}
		for _, existing := range b.messages {
			inThread := existing.ID == threadRoot || existing.ReplyTo == threadRoot
			if inThread && existing.From != body.From {
				// Include agents (skip "you"/"human" — they see via the web UI poll)
				if existing.From != "you" && existing.From != "human" && b.findMemberLocked(existing.From) != nil {
					threadParticipants = append(threadParticipants, existing.From)
				}
			}
		}
		tagged = uniqueSlugs(append(tagged, threadParticipants...))
	}

	if duplicate := b.findRecentExactAgentDuplicateLocked(body.From, channel, replyTo, body.Content); duplicate != nil {
		if b.lastTaggedAt != nil {
			delete(b.lastTaggedAt, agentLaneKey(channel, body.From))
		}
		if err := b.saveLocked(); err != nil {
			b.mu.Unlock()
			http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
			return
		}
		b.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":        duplicate.ID,
			"total":     len(b.messages),
			"duplicate": true,
		})
		return
	}

	if duplicate := b.findRecentRepeatedAgentStatusLocked(body.From, channel, replyTo, body.Content); duplicate != nil {
		if b.lastTaggedAt != nil {
			delete(b.lastTaggedAt, agentLaneKey(channel, body.From))
		}
		if err := b.saveLocked(); err != nil {
			b.mu.Unlock()
			http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
			return
		}
		b.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":        duplicate.ID,
			"total":     len(b.messages),
			"duplicate": true,
		})
		return
	}

	if duplicate := b.findRecentRepeatedCoordinationGuidanceLocked(body.From, channel, body.Content); duplicate != nil {
		if b.lastTaggedAt != nil {
			delete(b.lastTaggedAt, agentLaneKey(channel, body.From))
		}
		if err := b.saveLocked(); err != nil {
			b.mu.Unlock()
			http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
			return
		}
		b.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":        duplicate.ID,
			"total":     len(b.messages),
			"duplicate": true,
		})
		return
	}
	if duplicate := b.findRedundantOperationalRollCallLocked(body.From, channel, replyTo, body.Content, tagged); duplicate != nil {
		if b.lastTaggedAt != nil {
			delete(b.lastTaggedAt, agentLaneKey(channel, body.From))
		}
		if err := b.saveLocked(); err != nil {
			b.mu.Unlock()
			http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
			return
		}
		b.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":        duplicate.ID,
			"total":     len(b.messages),
			"duplicate": true,
		})
		return
	}
	if err := b.validateExecutionReplyLocked(body.From, channel, body.Content, replyTo); err != nil {
		b.mu.Unlock()
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	msg := channelMessage{
		ID:        fmt.Sprintf("msg-%d", b.counter),
		ClientID:  clientID,
		From:      body.From,
		Channel:   channel,
		Kind:      strings.TrimSpace(body.Kind),
		Title:     strings.TrimSpace(body.Title),
		Content:   body.Content,
		Tagged:    tagged,
		ReplyTo:   replyTo,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	b.ensureLeadTaggedMembersEnabledLocked(msg.From, channel, tagged)
	b.appendMessageLocked(msg)
	b.advanceExecutionGraphForMessageLocked(msg)
	b.recordChannelMemoryForMessageLocked(msg)
	b.inferLinkedReposFromHumanMessageLocked(msg.From, channel, msg.Content)
	total := len(b.messages)

	// Track which agents were tagged — they should show "typing" immediately
	if len(msg.Tagged) > 0 && (msg.From == "you" || msg.From == "human") {
		if b.lastTaggedAt == nil {
			b.lastTaggedAt = make(map[string]time.Time)
		}
		for _, slug := range msg.Tagged {
			b.lastTaggedAt[agentLaneKey(channel, slug)] = time.Now()
		}
	}

	// Clear typing indicator when an agent posts a reply
	if msg.From != "you" && msg.From != "human" && b.lastTaggedAt != nil {
		delete(b.lastTaggedAt, agentLaneKey(channel, msg.From))
	}

	// Auto-detect skill proposals from CEO messages
	b.parseSkillProposalLocked(msg)

	if err := b.saveLocked(); err != nil {
		b.mu.Unlock()
		http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
		return
	}
	b.mu.Unlock()

	b.respondPostMessage(w, msg, total, false)
}

func (b *Broker) respondPostMessage(w http.ResponseWriter, msg channelMessage, total int, duplicate bool) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":        msg.ID,
		"total":     total,
		"persisted": true,
		"duplicate": duplicate,
		"message":   msg,
	})
}

func (b *Broker) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID      string `json:"id"`
		Channel string `json:"channel"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	messageID := strings.TrimSpace(body.ID)
	if messageID == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	requestedChannel := normalizeChannelSlug(body.Channel)

	b.mu.Lock()
	b.ensureMessageIndexesLocked()
	index := -1
	msg := channelMessage{}
	for i := range b.messages {
		if strings.TrimSpace(b.messages[i].ID) != messageID {
			continue
		}
		index = i
		msg = b.messages[i]
		break
	}
	if index < 0 {
		b.mu.Unlock()
		http.Error(w, "message not found", http.StatusNotFound)
		return
	}

	channel := normalizeChannelSlug(msg.Channel)
	if channel == "" {
		channel = "general"
	}
	if requestedChannel != "" && requestedChannel != channel {
		b.mu.Unlock()
		http.Error(w, "message not found", http.StatusNotFound)
		return
	}
	if !b.messageCanBeDeletedLocked(msg) {
		b.mu.Unlock()
		http.Error(w, "only leaf messages outside active execution can be deleted", http.StatusConflict)
		return
	}
	threadID := firstNonEmpty(strings.TrimSpace(b.messageThreadRootByID[msg.ID]), b.threadRootFromMessageIDLocked(msg.ID), strings.TrimSpace(msg.ID))

	nextMessages := make([]channelMessage, 0, len(b.messages)-1)
	nextMessages = append(nextMessages, b.messages[:index]...)
	nextMessages = append(nextMessages, b.messages[index+1:]...)
	b.replaceMessagesLocked(nextMessages)
	b.appendActionLocked("message_deleted", "office", channel, "you", "Deleted message", messageID)
	total := len(b.messages)
	if err := b.saveLocked(); err != nil {
		b.mu.Unlock()
		http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
		return
	}
	b.mu.Unlock()

	b.respondPersistedMutation(w, map[string]any{
		"ok":        true,
		"id":        messageID,
		"channel":   channel,
		"thread_id": threadID,
		"total":     total,
	})
}

func (b *Broker) messageCanBeDeletedLocked(msg channelMessage) bool {
	messageID := strings.TrimSpace(msg.ID)
	if messageID == "" {
		return false
	}
	if b.messageHasRepliesLocked(messageID) {
		return false
	}
	if b.messageReferencedByExecutionNodeLocked(messageID) {
		return false
	}
	return true
}

func (b *Broker) messageHasRepliesLocked(messageID string) bool {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return false
	}
	for _, msg := range b.messages {
		if strings.TrimSpace(msg.ReplyTo) == messageID {
			return true
		}
	}
	return false
}

func (b *Broker) messageReferencedByExecutionNodeLocked(messageID string) bool {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return false
	}
	for _, node := range b.executionNodes {
		if strings.TrimSpace(node.RootMessageID) == messageID ||
			strings.TrimSpace(node.TriggerMessageID) == messageID ||
			strings.TrimSpace(node.ResolvedByMessageID) == messageID {
			return true
		}
	}
	return false
}

func (b *Broker) respondPersistedMutation(w http.ResponseWriter, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *Broker) findMutationAckLocked(scope, requestID string) (map[string]any, bool) {
	scope = strings.TrimSpace(scope)
	requestID = strings.TrimSpace(requestID)
	if scope == "" || requestID == "" || b.sharedMemory == nil {
		return nil, false
	}
	entries := b.sharedMemory[mutationAckNamespace]
	if len(entries) == 0 {
		return nil, false
	}
	raw := strings.TrimSpace(entries[scope+":"+requestID])
	if raw == "" {
		return nil, false
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, false
	}
	return payload, true
}

func (b *Broker) rememberMutationAckLocked(scope, requestID string, payload map[string]any) error {
	scope = strings.TrimSpace(scope)
	requestID = strings.TrimSpace(requestID)
	if scope == "" || requestID == "" {
		return nil
	}
	if b.sharedMemory == nil {
		b.sharedMemory = make(map[string]map[string]string)
	}
	if b.sharedMemory[mutationAckNamespace] == nil {
		b.sharedMemory[mutationAckNamespace] = make(map[string]string)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	b.sharedMemory[mutationAckNamespace][scope+":"+requestID] = string(data)
	return nil
}

func (b *Broker) findMessageByClientIDLocked(from, channel, clientID string) *channelMessage {
	from = strings.TrimSpace(from)
	channel = normalizeChannelSlug(channel)
	clientID = strings.TrimSpace(clientID)
	if from == "" || channel == "" || clientID == "" {
		return nil
	}
	for i := len(b.messages) - 1; i >= 0; i-- {
		msg := &b.messages[i]
		if strings.TrimSpace(msg.ClientID) != clientID {
			continue
		}
		if normalizeChannelSlug(msg.Channel) != channel {
			continue
		}
		if strings.TrimSpace(msg.From) != from {
			continue
		}
		return msg
	}
	return nil
}

func (b *Broker) handleReactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		MessageID string `json:"message_id"`
		Emoji     string `json:"emoji"`
		From      string `json:"from"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.MessageID == "" || body.Emoji == "" || body.From == "" {
		http.Error(w, "message_id, emoji, and from are required", http.StatusBadRequest)
		return
	}

	b.mu.Lock()
	found := false
	for i := range b.messages {
		if b.messages[i].ID == body.MessageID {
			// Don't duplicate: same emoji from same agent
			for _, r := range b.messages[i].Reactions {
				if r.Emoji == body.Emoji && r.From == body.From {
					b.mu.Unlock()
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "duplicate": true})
					return
				}
			}
			b.messages[i].Reactions = append(b.messages[i].Reactions, messageReaction{
				Emoji: body.Emoji,
				From:  body.From,
			})
			found = true
			break
		}
	}
	if !found {
		b.mu.Unlock()
		http.Error(w, "message not found", http.StatusNotFound)
		return
	}
	_ = b.saveLocked()
	b.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// RecordTelegramGroup saves a group chat ID and title seen by the transport.
func (b *Broker) RecordTelegramGroup(chatID int64, title string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.seenTelegramGroups == nil {
		b.seenTelegramGroups = make(map[int64]string)
	}
	b.seenTelegramGroups[chatID] = title
}

// SeenTelegramGroups returns all group chats the transport has seen.
func (b *Broker) SeenTelegramGroups() map[int64]string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.seenTelegramGroups == nil {
		return nil
	}
	out := make(map[int64]string, len(b.seenTelegramGroups))
	for k, v := range b.seenTelegramGroups {
		out[k] = v
	}
	return out
}

// PostSystemMessage posts a lightweight system message that shows progress without blocking.
func (b *Broker) PostSystemMessage(channel, content, kind string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.counter++
	if channel == "" {
		channel = "general"
	}
	msg := channelMessage{
		ID:        fmt.Sprintf("msg-%d", b.counter),
		From:      "system",
		Channel:   normalizeChannelSlug(channel),
		Kind:      kind,
		Content:   content,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	b.appendMessageLocked(msg)
}

func (b *Broker) PostMessage(from, channel, content string, tagged []string, replyTo string) (channelMessage, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if firstBlockingRequest(b.requests) != nil {
		return channelMessage{}, fmt.Errorf("request pending; answer required before chat resumes")
	}
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		channel = "general"
	}
	if IsDMSlug(channel) && b.findChannelLocked(channel) == nil {
		b.ensureDMConversationLocked(channel)
	}
	if b.findChannelLocked(channel) == nil {
		return channelMessage{}, fmt.Errorf("channel not found")
	}
	if !b.canAccessChannelLocked(from, channel) {
		return channelMessage{}, fmt.Errorf("channel access denied")
	}
	replyTo = strings.TrimSpace(replyTo)
	if !isHumanLikeActor(from) && !isSystemActor(from) && messageContentLooksLikeDisallowedAgentChannelContent(content) {
		return channelMessage{}, fmt.Errorf("non-substantive agent chatter is not allowed in channel messages; publish only the substantive result")
	}
	if err := b.validateTaskStateClaimLocked(channel, content); err != nil {
		return channelMessage{}, err
	}
	if duplicate := b.findRecentExactAgentDuplicateLocked(from, channel, replyTo, content); duplicate != nil {
		if b.lastTaggedAt != nil {
			delete(b.lastTaggedAt, agentLaneKey(channel, from))
		}
		if err := b.saveLocked(); err != nil {
			return channelMessage{}, err
		}
		return *duplicate, nil
	}
	if duplicate := b.findRecentRepeatedAgentStatusLocked(from, channel, replyTo, content); duplicate != nil {
		if b.lastTaggedAt != nil {
			delete(b.lastTaggedAt, agentLaneKey(channel, from))
		}
		if err := b.saveLocked(); err != nil {
			return channelMessage{}, err
		}
		return *duplicate, nil
	}
	if duplicate := b.findRecentRepeatedCoordinationGuidanceLocked(from, channel, content); duplicate != nil {
		if b.lastTaggedAt != nil {
			delete(b.lastTaggedAt, agentLaneKey(channel, from))
		}
		if err := b.saveLocked(); err != nil {
			return channelMessage{}, err
		}
		return *duplicate, nil
	}
	normalizedTagged := b.normalizeTaggedSlugsLocked(from, tagged, content)
	if duplicate := b.findRedundantOperationalRollCallLocked(from, channel, replyTo, content, normalizedTagged); duplicate != nil {
		if b.lastTaggedAt != nil {
			delete(b.lastTaggedAt, agentLaneKey(channel, from))
		}
		if err := b.saveLocked(); err != nil {
			return channelMessage{}, err
		}
		return *duplicate, nil
	}
	if err := b.validateExecutionReplyLocked(from, channel, content, replyTo); err != nil {
		return channelMessage{}, err
	}
	b.counter++
	msg := channelMessage{
		ID:        fmt.Sprintf("msg-%d", b.counter),
		From:      from,
		Channel:   channel,
		Kind:      "",
		Title:     "",
		Content:   strings.TrimSpace(content),
		Tagged:    normalizedTagged,
		ReplyTo:   replyTo,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	b.ensureLeadTaggedMembersEnabledLocked(from, channel, msg.Tagged)
	b.appendMessageLocked(msg)
	b.advanceExecutionGraphForMessageLocked(msg)
	b.recordChannelMemoryForMessageLocked(msg)
	b.inferLinkedReposFromHumanMessageLocked(from, channel, msg.Content)
	// Clear typing indicator — agent has replied
	if b.lastTaggedAt != nil {
		delete(b.lastTaggedAt, agentLaneKey(channel, msg.From))
	}
	b.appendActionLocked("automation", msg.Source, channel, msg.From, truncateSummary(msg.Title+" "+msg.Content, 140), msg.ID)
	if err := b.saveLocked(); err != nil {
		return channelMessage{}, err
	}
	return msg, nil
}

func (b *Broker) ensureLeadTaggedMembersEnabledLocked(from, channel string, tagged []string) {
	if b == nil || strings.TrimSpace(from) != "ceo" {
		return
	}
	ch := b.findChannelLocked(channel)
	if ch == nil || ch.isDM() {
		return
	}
	if len(tagged) == 0 {
		return
	}

	changed := false
	for _, slug := range uniqueSlugs(tagged) {
		slug = normalizeChannelSlug(slug)
		if slug == "" || slug == "ceo" {
			continue
		}
		if b.findMemberLocked(slug) == nil {
			continue
		}
		if !containsString(ch.Members, slug) {
			ch.Members = append(ch.Members, slug)
			changed = true
		}
	}
	if !changed && len(ch.Disabled) == 0 {
		return
	}
	filteredDisabled := make([]string, 0, len(ch.Disabled))
	for _, disabled := range ch.Disabled {
		if !containsString(tagged, disabled) {
			filteredDisabled = append(filteredDisabled, disabled)
		}
	}
	if len(filteredDisabled) != len(ch.Disabled) {
		changed = true
	}
	ch.Disabled = filteredDisabled
	if changed {
		ch.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
}

func (b *Broker) normalizeTaggedSlugsLocked(from string, tagged []string, content string) []string {
	normalized := uniqueSlugs(tagged)
	if len(normalized) > 0 {
		return normalized
	}
	if b == nil || strings.TrimSpace(from) != "ceo" {
		return normalized
	}
	inferred := inferTaggedSlugsFromContent(content)
	if len(inferred) == 0 {
		return normalized
	}
	filtered := make([]string, 0, len(inferred))
	for _, slug := range inferred {
		switch slug {
		case "", "ceo", "you", "human", "system", "all":
			continue
		}
		if b.findMemberLocked(slug) == nil {
			continue
		}
		filtered = append(filtered, slug)
	}
	return uniqueSlugs(filtered)
}

func inferTaggedSlugsFromContent(content string) []string {
	matches := brokerTextMentionPattern.FindAllStringSubmatch(strings.ToLower(strings.TrimSpace(content)), -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		slug := normalizeChannelSlug(match[2])
		if slug != "" {
			out = append(out, slug)
		}
	}
	return uniqueSlugs(out)
}

func (b *Broker) PostAutomationMessage(from, channel, title, content, eventID, source, sourceLabel string, tagged []string, replyTo string) (channelMessage, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if strings.TrimSpace(eventID) != "" {
		for _, existing := range b.messages {
			if existing.EventID != "" && existing.EventID == strings.TrimSpace(eventID) {
				return existing, true, nil
			}
		}
	}

	b.counter++
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		channel = "general"
	}
	msg := channelMessage{
		ID:          fmt.Sprintf("msg-%d", b.counter),
		From:        from,
		Channel:     channel,
		Kind:        "automation",
		Source:      strings.TrimSpace(source),
		SourceLabel: strings.TrimSpace(sourceLabel),
		EventID:     strings.TrimSpace(eventID),
		Title:       strings.TrimSpace(title),
		Content:     strings.TrimSpace(content),
		Tagged:      tagged,
		ReplyTo:     strings.TrimSpace(replyTo),
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
	if msg.Source == "" {
		msg.Source = "context_graph"
	}
	if msg.SourceLabel == "" {
		msg.SourceLabel = "Nex"
	}
	if msg.From == "" {
		msg.From = "nex"
	}

	b.appendMessageLocked(msg)
	if err := b.saveLocked(); err != nil {
		return channelMessage{}, false, err
	}
	return msg, false, nil
}

func (b *Broker) CreateRequest(req humanInterview) (humanInterview, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	channel := normalizeChannelSlug(req.Channel)
	if channel == "" {
		channel = "general"
	}
	if b.findChannelLocked(channel) == nil {
		return humanInterview{}, fmt.Errorf("channel not found")
	}
	b.counter++
	now := time.Now().UTC().Format(time.RFC3339)
	req.ID = fmt.Sprintf("request-%d", b.counter)
	req.Channel = channel
	req.CreatedAt = now
	req.UpdatedAt = now
	if strings.TrimSpace(req.Title) == "" {
		req.Title = "Request"
	}
	req = normalizeRequestRecord(req)
	normalized, err := resolveRequestTransition(req, "create", nil, time.Now().UTC())
	if err != nil {
		return humanInterview{}, err
	}
	req = normalized
	b.scheduleRequestLifecycleLocked(&req)
	b.requests = append(b.requests, req)
	b.pendingInterview = firstBlockingRequest(b.requests)
	b.appendActionLocked("request_created", "office", channel, req.From, truncateSummary(req.Title+" "+req.Question, 140), req.ID)
	if err := b.saveLocked(); err != nil {
		return humanInterview{}, err
	}
	return req, nil
}

func (b *Broker) handleGetMessages(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	q := r.URL.Query()
	limit := 10
	if l, err := strconv.Atoi(q.Get("limit")); err == nil && l > 0 {
		limit = l
	}
	if limit > 100 {
		limit = 100
	}

	sinceID := q.Get("since_id")
	beforeID := strings.TrimSpace(q.Get("before_id"))
	mySlug := strings.TrimSpace(q.Get("my_slug"))
	viewerSlug := strings.TrimSpace(q.Get("viewer_slug"))
	threadID := strings.TrimSpace(q.Get("thread_id"))
	if threadID == "" {
		threadID = strings.TrimSpace(q.Get("reply_to"))
	}
	scope := normalizeMessageScope(q.Get("scope"))
	if rawScope := strings.TrimSpace(q.Get("scope")); rawScope != "" && scope == "" {
		http.Error(w, "invalid message scope", http.StatusBadRequest)
		return
	}
	channel := normalizeChannelSlug(q.Get("channel"))
	if channel == "" {
		channel = "general"
	}
	accessSlug := mySlug
	if accessSlug == "" {
		accessSlug = viewerSlug
	}

	lockStartedAt := time.Now()
	b.mu.Lock()
	lockWait := time.Since(lockStartedAt)
	// Auto-create DM conversation on read (user opens DM before sending)
	if IsDMSlug(channel) && b.findChannelLocked(channel) == nil {
		b.ensureDMConversationLocked(channel)
	}
	if !b.canAccessChannelLocked(accessSlug, channel) {
		b.mu.Unlock()
		http.Error(w, "channel access denied", http.StatusForbidden)
		return
	}
	b.ensureMessageIndexesLocked()
	needsViewerScope := scope != "" && viewerSlug != ""
	needsThreadScan := threadID != ""
	window := newMessageWindow(limit, sinceID, beforeID)
	var messages []channelMessage
	var hasMore bool
	threadRootID := ""

	if needsViewerScope || needsThreadScan {
		sourceIndexes := b.messageIndexesByChannel[channel]
		if needsThreadScan {
			threadRootID = firstNonEmpty(strings.TrimSpace(b.messageThreadRootByID[threadID]), b.threadRootFromMessageIDLocked(threadID), threadID)
			if threadIndexesByRoot := b.messageIndexesByThread[channel]; threadRootID != "" && threadIndexesByRoot != nil {
				sourceIndexes = threadIndexesByRoot[threadRootID]
			} else {
				sourceIndexes = nil
			}
		}
		if !needsViewerScope {
			messages, hasMore = b.collectRecentIndexedMessagesLocked(sourceIndexes, sinceID, beforeID, limit)
		} else {
			needsMessageIndex := scope != "outbox"
			var messageIndex map[string]channelMessage
			if needsMessageIndex {
				messageIndex = make(map[string]channelMessage, len(sourceIndexes))
			}
			for _, idx := range sourceIndexes {
				msg := b.messages[idx]
				if needsMessageIndex {
					if id := strings.TrimSpace(msg.ID); id != "" {
						messageIndex[id] = msg
					}
				}
				if !b.messageAllowedForChannelReadLocked(msg) {
					continue
				}
				if !messageMatchesViewerScope(msg, viewerSlug, scope, messageIndex) {
					continue
				}
				if !window.Append(msg) {
					break
				}
			}
		}
	} else {
		messages, hasMore = b.collectRecentChannelMessagesLocked(channel, sinceID, beforeID, limit)
	}

	if messages == nil {
		messages, hasMore = window.Finalize()
	}
	if len(messages) == 0 && sinceID == "" && beforeID == "" && threadID == "" {
		synthetic := b.syntheticChannelMessagesLocked(channel)
		hasMore = len(synthetic) > limit
		if len(synthetic) > limit {
			synthetic = synthetic[len(synthetic)-limit:]
		}
		messages = synthetic
	}
	// Copy to avoid race
	result := make([]channelMessage, len(messages))
	copy(result, messages)
	executionNodes := make([]executionNode, 0, len(b.executionNodes))
	threadRootID = firstNonEmpty(threadRootID, b.threadRootFromMessageIDLocked(threadID), threadID)
	for i := range result {
		result[i].CanDelete = b.messageCanBeDeletedLocked(result[i])
	}
	for _, node := range b.executionNodes {
		if normalizeChannelSlug(node.Channel) != channel {
			continue
		}
		if threadRootID != "" && strings.TrimSpace(node.RootMessageID) != threadRootID {
			continue
		}
		copyNode := node
		copyNode.ExpectedFrom = append([]string(nil), node.ExpectedFrom...)
		executionNodes = append(executionNodes, copyNode)
	}
	b.mu.Unlock()

	taggedCount := 0
	taggedSlug := mySlug
	if taggedSlug == "" {
		taggedSlug = viewerSlug
	}
	if taggedSlug != "" {
		for _, m := range result {
			for _, t := range m.Tagged {
				if t == taggedSlug {
					taggedCount++
					break
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"channel":         channel,
		"messages":        result,
		"execution_nodes": coalesceLatestExecutionNodesByOwner(executionNodes),
		"tagged_count":    taggedCount,
		"has_more":        hasMore,
	})
	if brokerDebugHTTPTimingEnabled() {
		log.Printf("broker http timing path=%s channel=%s thread=%t scope=%q limit=%d lock_wait=%s total=%s messages=%d", r.URL.Path, channel, threadID != "", scope, limit, lockWait, time.Since(startedAt), len(result))
	}
}

func coalesceLatestExecutionNodesByOwner(nodes []executionNode) []executionNode {
	if len(nodes) == 0 {
		return nil
	}

	latestByOwner := make(map[string]executionNode, len(nodes))
	updatedByOwner := make(map[string]time.Time, len(nodes))

	for _, node := range nodes {
		owner := strings.TrimSpace(node.OwnerAgent)
		if owner == "" {
			continue
		}
		updatedAt := executionNodeUpdatedAt(node)
		existingUpdatedAt, ok := updatedByOwner[owner]
		if !ok {
			latestByOwner[owner] = node
			updatedByOwner[owner] = updatedAt
			continue
		}
		if updatedAt.IsZero() {
			continue
		}
		if !existingUpdatedAt.IsZero() && updatedAt.After(existingUpdatedAt) {
			latestByOwner[owner] = node
			updatedByOwner[owner] = updatedAt
			continue
		}
		if existingUpdatedAt.IsZero() {
			latestByOwner[owner] = node
			updatedByOwner[owner] = updatedAt
		}
	}

	out := make([]executionNode, 0, len(latestByOwner))
	for _, node := range latestByOwner {
		out = append(out, node)
	}
	sort.Slice(out, func(i, j int) bool {
		timestampI := executionNodeUpdatedAt(out[i])
		timestampJ := executionNodeUpdatedAt(out[j])
		if timestampI.Equal(timestampJ) {
			rootI := strings.TrimSpace(out[i].ParentNodeID) == ""
			rootJ := strings.TrimSpace(out[j].ParentNodeID) == ""
			if rootI != rootJ {
				return rootI
			}
			return strings.TrimSpace(out[i].OwnerAgent) < strings.TrimSpace(out[j].OwnerAgent)
		}
		if timestampI.IsZero() {
			return false
		}
		if timestampJ.IsZero() {
			return true
		}
		return timestampI.Before(timestampJ)
	})
	return out
}

func messageInThread(msg channelMessage, threadID string) bool {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return true
	}
	return strings.TrimSpace(msg.ID) == threadID || strings.TrimSpace(msg.ReplyTo) == threadID
}

func (b *Broker) messageAllowedForChannelReadLocked(msg channelMessage) bool {
	if b.sessionMode == SessionModeOneOnOne && !b.isOneOnOneDMMessage(msg) {
		return false
	}
	if !isHumanLikeActor(msg.From) && !isSystemActor(msg.From) && messageContentLooksLikeDisallowedAgentChannelContent(msg.Content) {
		return false
	}
	return true
}

func parseSearchBool(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	ok, err := strconv.ParseBool(raw)
	return err == nil && ok
}

type messageSearchHit struct {
	ID        string `json:"id"`
	Channel   string `json:"channel"`
	From      string `json:"from"`
	Title     string `json:"title,omitempty"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
	ReplyTo   string `json:"reply_to,omitempty"`
	ThreadID  string `json:"thread_id,omitempty"`
}

type messageThreadSummary struct {
	ThreadID    string         `json:"thread_id"`
	Channel     string         `json:"channel"`
	ReplyCount  int            `json:"reply_count"`
	LastReplyAt string         `json:"last_reply_at,omitempty"`
	Message     channelMessage `json:"message"`
	SortIndex   int            `json:"-"`
}

func (b *Broker) handleSearchMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if len(query) < 2 {
		http.Error(w, "query too short", http.StatusBadRequest)
		return
	}
	limit := 20
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = l
	}
	if limit > 100 {
		limit = 100
	}
	viewerSlug := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	channelFilter := normalizeChannelSlug(r.URL.Query().Get("channel"))
	authorFilter := normalizeActorSlug(r.URL.Query().Get("from"))
	threadID := strings.TrimSpace(r.URL.Query().Get("thread_id"))
	isThread := parseSearchBool(r.URL.Query().Get("is_thread"))

	b.mu.Lock()
	b.ensureMessageIndexesLocked()
	hits := make([]messageSearchHit, 0, limit)
	queryTokens := messageSearchTokensForIndex(query)
	candidateSet, shortlisted := b.messageSearchCandidateSetLocked(queryTokens, channelFilter, authorFilter, threadID, isThread)
	for i := len(b.messages) - 1; i >= 0 && len(hits) < limit; i-- {
		if shortlisted {
			if _, ok := candidateSet[i]; !ok {
				continue
			}
		}
		msg := b.messages[i]
		channel := normalizeChannelSlug(msg.Channel)
		if channel == "" {
			channel = "general"
		}
		if channelFilter != "" && channel != channelFilter {
			continue
		}
		if viewerSlug != "" && !b.canAccessChannelLocked(viewerSlug, channel) {
			continue
		}
		if !b.messageAllowedForChannelReadLocked(msg) {
			continue
		}
		haystack := strings.ToLower(strings.TrimSpace(msg.Title + "\n" + msg.Content))
		if !strings.Contains(haystack, query) {
			continue
		}
		threadID := firstNonEmpty(strings.TrimSpace(b.messageThreadRootByID[msg.ID]), b.threadRootFromMessageIDLocked(msg.ID), strings.TrimSpace(msg.ID))
		hits = append(hits, messageSearchHit{
			ID:        strings.TrimSpace(msg.ID),
			Channel:   channel,
			From:      strings.TrimSpace(msg.From),
			Title:     strings.TrimSpace(msg.Title),
			Content:   msg.Content,
			Timestamp: strings.TrimSpace(msg.Timestamp),
			ReplyTo:   strings.TrimSpace(msg.ReplyTo),
			ThreadID:  threadID,
		})
	}
	b.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"query": query,
		"hits":  hits,
	})
}

func (b *Broker) handleGetMessageThreads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := 50
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = l
	}
	if limit > 200 {
		limit = 200
	}
	viewerSlug := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	channelFilter := normalizeChannelSlug(r.URL.Query().Get("channel"))

	b.mu.Lock()
	b.ensureMessageIndexesLocked()
	summaries := make([]messageThreadSummary, 0, limit)
	for channel, roots := range b.messageIndexesByThread {
		if channelFilter != "" && channel != channelFilter {
			continue
		}
		if viewerSlug != "" && !b.canAccessChannelLocked(viewerSlug, channel) {
			continue
		}
		for rootID, indexes := range roots {
			if len(indexes) <= 1 {
				continue
			}
			rootFound := false
			rootMsg := channelMessage{}
			replyCount := 0
			lastReplyAt := ""
			for _, idx := range indexes {
				msg := b.messages[idx]
				if !b.messageAllowedForChannelReadLocked(msg) {
					continue
				}
				if strings.TrimSpace(msg.ID) == rootID {
					rootMsg = msg
					rootFound = true
					continue
				}
				replyCount++
				if ts := strings.TrimSpace(msg.Timestamp); ts != "" {
					lastReplyAt = ts
				}
			}
			if !rootFound || replyCount == 0 {
				continue
			}
			rootMsg.CanDelete = b.messageCanBeDeletedLocked(rootMsg)
			summaries = append(summaries, messageThreadSummary{
				ThreadID:    rootID,
				Channel:     channel,
				ReplyCount:  replyCount,
				LastReplyAt: lastReplyAt,
				Message:     rootMsg,
				SortIndex:   indexes[len(indexes)-1],
			})
		}
	}
	b.mu.Unlock()

	sort.SliceStable(summaries, func(i, j int) bool {
		leftTime := strings.TrimSpace(firstNonEmpty(summaries[i].LastReplyAt, summaries[i].Message.Timestamp))
		rightTime := strings.TrimSpace(firstNonEmpty(summaries[j].LastReplyAt, summaries[j].Message.Timestamp))
		if leftTime == rightTime {
			if summaries[i].SortIndex != summaries[j].SortIndex {
				return summaries[i].SortIndex > summaries[j].SortIndex
			}
			if summaries[i].ReplyCount == summaries[j].ReplyCount {
				if summaries[i].Channel == summaries[j].Channel {
					return summaries[i].ThreadID < summaries[j].ThreadID
				}
				return summaries[i].Channel < summaries[j].Channel
			}
			return summaries[i].ReplyCount > summaries[j].ReplyCount
		}
		return leftTime > rightTime
	})
	if len(summaries) > limit {
		summaries = summaries[:limit]
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"threads": summaries,
	})
}

type messageWindow struct {
	limit      int
	sinceID    string
	beforeID   string
	items      []channelMessage
	hasMore    bool
	sinceFound bool
}

func newMessageWindow(limit int, sinceID, beforeID string) *messageWindow {
	sinceID = strings.TrimSpace(sinceID)
	beforeID = strings.TrimSpace(beforeID)
	if sinceID != "" {
		beforeID = ""
	}
	capHint := limit + 1
	if capHint < 1 {
		capHint = 1
	}
	return &messageWindow{
		limit:    limit,
		sinceID:  sinceID,
		beforeID: beforeID,
		items:    make([]channelMessage, 0, capHint),
	}
}

func (w *messageWindow) Append(msg channelMessage) bool {
	msgID := strings.TrimSpace(msg.ID)
	switch {
	case w.sinceID != "":
		if msgID == w.sinceID {
			w.sinceFound = true
			w.items = w.items[:0]
			w.hasMore = false
			return true
		}
	case w.beforeID != "":
		if msgID == w.beforeID {
			return false
		}
	}

	w.items = append(w.items, msg)
	if len(w.items) > w.limit {
		w.hasMore = true
	}
	if len(w.items) > w.limit+1 {
		w.items = w.items[1:]
	}
	return true
}

func (w *messageWindow) Finalize() ([]channelMessage, bool) {
	if len(w.items) > w.limit {
		w.items = w.items[len(w.items)-w.limit:]
	}
	return w.items, w.hasMore
}

// collectRecentChannelMessagesLocked is the hot path for plain channel reads:
// no thread expansion and no viewer-scope filtering. It scans from the tail so
// initial loads and polling stop as soon as the newest useful window is known.
// Caller must hold b.mu.
func (b *Broker) collectRecentChannelMessagesLocked(channel, sinceID, beforeID string, limit int) ([]channelMessage, bool) {
	b.ensureMessageIndexesLocked()
	channel = normalizeChannelSlug(channel)
	return b.collectRecentIndexedMessagesLocked(b.messageIndexesByChannel[channel], sinceID, beforeID, limit)
}

// collectRecentIndexedMessagesLocked is the hot path for indexed message reads:
// plain channel windows and thread windows without viewer-scope filtering.
// Caller must hold b.mu.
func (b *Broker) collectRecentIndexedMessagesLocked(indexes []int, sinceID, beforeID string, limit int) ([]channelMessage, bool) {
	sinceID = strings.TrimSpace(sinceID)
	beforeID = strings.TrimSpace(beforeID)
	if sinceID != "" {
		beforeID = ""
	}

	collected := make([]channelMessage, 0, limit+1)
	collectingOlder := beforeID == ""

	for i := len(indexes) - 1; i >= 0; i-- {
		msg := b.messages[indexes[i]]
		if !b.messageAllowedForChannelReadLocked(msg) {
			continue
		}

		msgID := strings.TrimSpace(msg.ID)
		if !collectingOlder {
			if msgID == beforeID {
				collectingOlder = true
			}
			continue
		}
		if sinceID != "" && msgID == sinceID {
			break
		}

		collected = append(collected, msg)
		if len(collected) > limit {
			out := append([]channelMessage(nil), collected[:limit]...)
			reverseChannelMessages(out)
			return out, true
		}
	}

	reverseChannelMessages(collected)
	return collected, false
}

func reverseChannelMessages(messages []channelMessage) {
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
}

func collectThreadMessageIDs(messages []channelMessage, threadID string) map[string]struct{} {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil
	}
	ids := map[string]struct{}{threadID: {}}
	children := make(map[string][]string, len(messages))
	for _, msg := range messages {
		parent := strings.TrimSpace(msg.ReplyTo)
		id := strings.TrimSpace(msg.ID)
		if parent == "" || id == "" {
			continue
		}
		children[parent] = append(children[parent], id)
	}
	queue := []string{threadID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, child := range children[current] {
			if _, seen := ids[child]; seen {
				continue
			}
			ids[child] = struct{}{}
			queue = append(queue, child)
		}
	}
	return ids
}

func normalizeMessageScope(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "", "all", "channel":
		return ""
	case "agent", "inbox", "outbox":
		return strings.TrimSpace(strings.ToLower(value))
	default:
		return ""
	}
}

func messageMatchesViewerScope(msg channelMessage, viewerSlug, scope string, messagesByID map[string]channelMessage) bool {
	scope = normalizeMessageScope(scope)
	switch scope {
	case "inbox":
		return messageBelongsToViewerInbox(msg, viewerSlug, messagesByID)
	case "outbox":
		return messageBelongsToViewerOutbox(msg, viewerSlug)
	case "agent":
		return messageVisibleToViewer(msg, viewerSlug, messagesByID)
	default:
		return true
	}
}

func messageVisibleToViewer(msg channelMessage, viewerSlug string, messagesByID map[string]channelMessage) bool {
	return messageBelongsToViewerOutbox(msg, viewerSlug) || messageBelongsToViewerInbox(msg, viewerSlug, messagesByID)
}

func messageBelongsToViewerOutbox(msg channelMessage, viewerSlug string) bool {
	viewerSlug = strings.TrimSpace(viewerSlug)
	if viewerSlug == "" || viewerSlug == "ceo" {
		return true
	}
	return strings.TrimSpace(msg.From) == viewerSlug
}

func messageBelongsToViewerInbox(msg channelMessage, viewerSlug string, messagesByID map[string]channelMessage) bool {
	viewerSlug = strings.TrimSpace(viewerSlug)
	if viewerSlug == "" || viewerSlug == "ceo" {
		return true
	}
	from := strings.TrimSpace(msg.From)
	switch from {
	case viewerSlug:
		return false
	case "you", "human", "ceo":
		return true
	}
	for _, tagged := range msg.Tagged {
		tagged = strings.TrimSpace(tagged)
		if tagged == viewerSlug || tagged == "all" {
			return true
		}
	}
	return messageRepliesToViewerThread(msg, viewerSlug, messagesByID)
}

func messageRepliesToViewerThread(msg channelMessage, viewerSlug string, messagesByID map[string]channelMessage) bool {
	replyTo := strings.TrimSpace(msg.ReplyTo)
	if replyTo == "" || viewerSlug == "" {
		return false
	}
	seen := map[string]bool{}
	for replyTo != "" {
		if seen[replyTo] {
			return false
		}
		seen[replyTo] = true
		parent, ok := messagesByID[replyTo]
		if !ok {
			return false
		}
		if strings.TrimSpace(parent.From) == viewerSlug {
			return true
		}
		replyTo = strings.TrimSpace(parent.ReplyTo)
	}
	return false
}

// isOneOnOneDMMessage returns true if msg belongs in the 1:1 DM conversation.
// Only messages exclusively between the human and the 1:1 agent pass through.
// Caller must hold b.mu.
func (b *Broker) isOneOnOneDMMessage(msg channelMessage) bool {
	agent := b.oneOnOneAgent

	switch msg.From {
	case "you", "human":
		// Human messages: only if untagged (direct conversation) or
		// explicitly tagging the 1:1 agent.
		if len(msg.Tagged) == 0 {
			return true
		}
		for _, t := range msg.Tagged {
			if t == agent {
				return true
			}
		}
		return false

	case agent:
		// Agent messages: only if untagged (direct reply to human) or
		// explicitly tagging the human.
		if len(msg.Tagged) == 0 {
			return true
		}
		for _, t := range msg.Tagged {
			if t == "you" || t == "human" {
				return true
			}
		}
		return false

	case "system":
		// System messages: only if they mention the 1:1 agent or human,
		// or are general system announcements (no routing indicators).
		if msg.Kind == "routing" {
			return false
		}
		return true

	default:
		// Messages from any other agent do not belong in this DM.
		return false
	}
}

// capturePaneActivity captures tmux pane content for each agent and detects
// activity by comparing with the previous snapshot. If content changed,
// the agent is active and we return the last 5 non-empty lines as a stream.
// If content is the same as last time, agent is idle — return nothing.
func (b *Broker) capturePaneActivity(slugOverride string) map[string]string {
	result := make(map[string]string)

	type paneCheck struct {
		slug   string
		target string
	}

	var checks []paneCheck
	if slugOverride != "" {
		// 1:1 mode: only check pane 1
		checks = append(checks, paneCheck{slug: slugOverride, target: fmt.Sprintf("%s:team.1", SessionName)})
	} else {
		manifest := company.DefaultManifest()
		loaded, loadErr := company.LoadManifest()
		if loadErr == nil && len(loaded.Members) > 0 {
			manifest = loaded
		}
		for i, agent := range manifest.Members {
			checks = append(checks, paneCheck{
				slug:   agent.Slug,
				target: fmt.Sprintf("wuphf-team:team.%d", i+1),
			})
		}
	}

	b.mu.Lock()
	if b.lastPaneSnapshot == nil {
		b.lastPaneSnapshot = make(map[string]string)
	}
	b.mu.Unlock()

	for _, check := range checks {
		paneOut, err := exec.Command("tmux", "-L", "wuphf", "capture-pane",
			"-p", "-J",
			"-t", check.target).CombinedOutput()
		if err != nil {
			continue
		}

		content := string(paneOut)

		// Compare with previous snapshot
		b.mu.Lock()
		prev := b.lastPaneSnapshot[check.slug]
		b.lastPaneSnapshot[check.slug] = content
		b.mu.Unlock()

		if content == prev {
			// No change — agent is idle
			continue
		}

		// Content changed — agent is active. Extract last 5 meaningful lines.
		lines := strings.Split(content, "\n")
		var meaningful []string
		for i := len(lines) - 1; i >= 0 && len(meaningful) < 5; i-- {
			trimmed := strings.TrimSpace(lines[i])
			if trimmed == "" {
				continue
			}
			meaningful = append(meaningful, trimmed)
		}
		// Reverse to chronological order
		for i, j := 0, len(meaningful)-1; i < j; i, j = i+1, j-1 {
			meaningful[i], meaningful[j] = meaningful[j], meaningful[i]
		}
		if len(meaningful) > 0 {
			result[check.slug] = strings.Join(meaningful, "\n")
		}
	}
	return result
}

func (b *Broker) handleMembers(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	if channel == "" {
		channel = "general"
	}
	viewerSlug := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	if !b.canAccessChannelLocked(viewerSlug, channel) {
		b.mu.Unlock()
		http.Error(w, "channel access denied", http.StatusForbidden)
		return
	}
	type memberView struct {
		name        string
		role        string
		lastMessage string
		lastTime    string
		disabled    bool
	}
	members := make(map[string]memberView)
	if ch := b.findChannelLocked(channel); ch != nil {
		for _, member := range ch.Members {
			if b.sessionMode == SessionModeOneOnOne && member != b.oneOnOneAgent {
				continue
			}
			info := memberView{disabled: containsString(ch.Disabled, member)}
			if office := b.findMemberLocked(member); office != nil {
				info.name = office.Name
				info.role = office.Role
			}
			members[member] = info
		}
	}
	for _, msg := range b.messages {
		if normalizeChannelSlug(msg.Channel) != channel {
			continue
		}
		if b.sessionMode == SessionModeOneOnOne && msg.From != b.oneOnOneAgent {
			continue
		}
		if msg.Kind == "automation" || msg.From == "nex" {
			continue
		}
		content := msg.Content
		if len(content) > 80 {
			content = content[:80]
		}
		info := members[msg.From]
		info.lastMessage = content
		info.lastTime = msg.Timestamp
		if info.name == "" {
			if office := b.findMemberLocked(msg.From); office != nil {
				info.name = office.Name
				info.role = office.Role
			}
		}
		members[msg.From] = info
	}
	isOneOnOne := b.sessionMode == SessionModeOneOnOne
	oneOnOneSlug := b.oneOnOneAgent
	taggedAt := make(map[string]time.Time, len(b.lastTaggedAt))
	for laneKey, ts := range b.lastTaggedAt {
		if !agentLaneMatchesChannel(laneKey, channel) {
			continue
		}
		slug := agentLaneSlug(laneKey)
		if prior, ok := taggedAt[slug]; !ok || ts.After(prior) {
			taggedAt[slug] = ts
		}
	}
	activity := make(map[string]agentActivitySnapshot, len(b.activity))
	for laneKey, snapshot := range b.activity {
		if !agentLaneMatchesChannel(laneKey, channel) {
			continue
		}
		slug := firstNonEmpty(strings.TrimSpace(snapshot.Slug), agentLaneSlug(laneKey))
		if slug == "" {
			continue
		}
		snapshot.Slug = slug
		if strings.TrimSpace(snapshot.Channel) == "" {
			snapshot.Channel = agentLaneChannel(laneKey)
		}
		if prior, ok := activity[slug]; ok && strings.TrimSpace(prior.LastTime) >= strings.TrimSpace(snapshot.LastTime) {
			continue
		}
		activity[slug] = snapshot
	}
	b.mu.Unlock()

	type memberEntry struct {
		Slug         string `json:"slug"`
		Name         string `json:"name,omitempty"`
		Role         string `json:"role,omitempty"`
		Disabled     bool   `json:"disabled,omitempty"`
		LastMessage  string `json:"lastMessage"`
		LastTime     string `json:"lastTime"`
		LiveActivity string `json:"liveActivity,omitempty"`
		Status       string `json:"status,omitempty"`
		Activity     string `json:"activity,omitempty"`
		Detail       string `json:"detail,omitempty"`
		TotalMs      int64  `json:"totalMs,omitempty"`
		FirstEventMs int64  `json:"firstEventMs,omitempty"`
		FirstTextMs  int64  `json:"firstTextMs,omitempty"`
		FirstToolMs  int64  `json:"firstToolMs,omitempty"`
	}

	// Capture pane activity via diff detection.
	// If content changed since last poll, agent is active — return last 5 lines.
	var paneActivity map[string]string
	if isOneOnOne && oneOnOneSlug != "" {
		paneActivity = b.capturePaneActivity(oneOnOneSlug)
	} else {
		paneActivity = b.capturePaneActivity("")
	}

	var list []memberEntry
	for slug, info := range members {
		entry := memberEntry{
			Slug:        slug,
			Name:        info.name,
			Role:        info.role,
			Disabled:    info.disabled,
			LastMessage: info.lastMessage,
			LastTime:    info.lastTime,
		}
		if snapshot, ok := activity[slug]; ok {
			entry.Status = snapshot.Status
			entry.Activity = snapshot.Activity
			entry.Detail = snapshot.Detail
			entry.TotalMs = snapshot.TotalMs
			entry.FirstEventMs = snapshot.FirstEventMs
			entry.FirstTextMs = snapshot.FirstTextMs
			entry.FirstToolMs = snapshot.FirstToolMs
			if snapshot.LastTime != "" {
				entry.LastTime = snapshot.LastTime
			}
			if snapshot.Detail != "" {
				entry.LiveActivity = snapshot.Detail
			}
		}
		if live, ok := paneActivity[slug]; ok {
			entry.Status = "active"
			if entry.Activity == "" {
				entry.Activity = "text"
			}
			entry.LiveActivity = live
			entry.Detail = live
			if entry.LastTime == "" {
				entry.LastTime = time.Now().UTC().Format(time.RFC3339)
			}
		}
		// Also mark as active if tagged recently and hasn't replied yet
		if entry.LiveActivity == "" && taggedAt != nil {
			if t, ok := taggedAt[slug]; ok && time.Since(t) < 60*time.Second {
				entry.Status = "active"
				if entry.Activity == "" {
					entry.Activity = "queued"
				}
				entry.LiveActivity = "active"
			}
		}
		if entry.Status == "" {
			entry.Status = "idle"
		}
		if entry.Activity == "" {
			entry.Activity = "idle"
		}
		list = append(list, entry)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"channel": channel, "members": list})
}

func (b *Broker) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		b.handleGetTasks(w, r)
	case http.MethodPost:
		b.handlePostTask(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *Broker) handleAgentLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	b.mu.Lock()
	root := b.agentLogRoot
	b.mu.Unlock()
	if root == "" {
		root = agent.DefaultTaskLogRoot()
	}

	task := strings.TrimSpace(r.URL.Query().Get("task"))
	if task != "" {
		// Guard against path traversal — the task id is a single directory name.
		if strings.Contains(task, "..") || strings.ContainsAny(task, `/\`) {
			http.Error(w, "invalid task id", http.StatusBadRequest)
			return
		}
		entries, err := agent.ReadTaskLog(root, task)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				http.Error(w, "task not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"task":    task,
			"entries": entries,
		})
		return
	}

	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	tasks, err := agent.ListRecentTasks(root, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"tasks": tasks})
}

func (b *Broker) handleGetTasks(w http.ResponseWriter, r *http.Request) {
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	mySlug := strings.TrimSpace(r.URL.Query().Get("my_slug"))
	viewerSlug := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	allChannels := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all_channels")), "true")
	if channel == "" && !allChannels {
		channel = "general"
	}
	includeDone := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_done")), "true")

	b.mu.Lock()
	if !allChannels && !b.canAccessChannelLocked(viewerSlug, channel) {
		b.mu.Unlock()
		http.Error(w, "channel access denied", http.StatusForbidden)
		return
	}
	result := b.buildOperatorTasksLocked(channel, allChannels, includeDone, statusFilter, mySlug)
	b.mu.Unlock()
	result = coalesceTaskView(result)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"channel": channel, "tasks": result})
}

func (b *Broker) handlePostTask(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action           string   `json:"action"`
		Channel          string   `json:"channel"`
		ID               string   `json:"id"`
		ExecutionKey     string   `json:"execution_key"`
		Title            string   `json:"title"`
		Details          string   `json:"details"`
		Owner            string   `json:"owner"`
		CreatedBy        string   `json:"created_by"`
		ThreadID         string   `json:"thread_id"`
		TaskType         string   `json:"task_type"`
		PipelineID       string   `json:"pipeline_id"`
		ExecutionMode    string   `json:"execution_mode"`
		ReviewState      string   `json:"review_state"`
		SourceSignalID   string   `json:"source_signal_id"`
		SourceDecisionID string   `json:"source_decision_id"`
		WorkspacePath    string   `json:"workspace_path"`
		WorktreePath     string   `json:"worktree_path"`
		WorktreeBranch   string   `json:"worktree_branch"`
		DependsOn        []string `json:"depends_on"`
		Actor            string   `json:"actor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	action := strings.TrimSpace(body.Action)
	now := time.Now().UTC().Format(time.RFC3339)
	channel := normalizeChannelSlug(body.Channel)
	if channel == "" {
		channel = "general"
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.findChannelLocked(channel) == nil {
		http.Error(w, "channel not found", http.StatusNotFound)
		return
	}
	if !b.canAccessChannelLocked(body.CreatedBy, channel) {
		http.Error(w, "channel access denied", http.StatusForbidden)
		return
	}

	if action == "create" {
		if strings.TrimSpace(body.Title) == "" || strings.TrimSpace(body.CreatedBy) == "" {
			http.Error(w, "title and created_by required", http.StatusBadRequest)
			return
		}
		validated, err := b.validateStrictTaskPlanLocked(channel, strings.TrimSpace(body.CreatedBy), []plannedTaskSpec{{
			ExecutionKey:  body.ExecutionKey,
			Title:         body.Title,
			Assignee:      body.Owner,
			Details:       body.Details,
			TaskType:      body.TaskType,
			ExecutionMode: body.ExecutionMode,
			WorkspacePath: body.WorkspacePath,
			DependsOn:     append([]string(nil), body.DependsOn...),
		}})
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		item := validated[0]
		match := taskReuseMatch{
			Channel:          channel,
			ExecutionKey:     item.ExecutionKey,
			Title:            item.Title,
			Details:          item.Details,
			ThreadID:         strings.TrimSpace(body.ThreadID),
			Owner:            item.Owner,
			TaskType:         item.TaskType,
			PipelineID:       strings.TrimSpace(body.PipelineID),
			ExecutionMode:    item.ExecutionMode,
			WorkspacePath:    item.WorkspacePath,
			SourceSignalID:   strings.TrimSpace(body.SourceSignalID),
			SourceDecisionID: strings.TrimSpace(body.SourceDecisionID),
		}
		if existing := b.findReusableTaskLocked(match); existing != nil {
			previousStatus := existing.Status
			if details := item.Details; details != "" {
				existing.Details = details
			}
			if owner := item.Owner; owner != "" {
				existing.Owner = owner
				if !taskIsTerminal(existing) {
					existing.Status = "in_progress"
				}
			}
			if executionKey := item.ExecutionKey; executionKey != "" {
				existing.ExecutionKey = executionKey
			}
			if taskType := item.TaskType; taskType != "" {
				existing.TaskType = taskType
			}
			if pipelineID := strings.TrimSpace(body.PipelineID); pipelineID != "" {
				existing.PipelineID = pipelineID
			}
			if executionMode := item.ExecutionMode; executionMode != "" {
				existing.ExecutionMode = executionMode
			}
			if reviewState := strings.TrimSpace(body.ReviewState); reviewState != "" {
				existing.ReviewState = reviewState
			}
			if sourceSignalID := strings.TrimSpace(body.SourceSignalID); sourceSignalID != "" {
				existing.SourceSignalID = sourceSignalID
			}
			if sourceDecisionID := strings.TrimSpace(body.SourceDecisionID); sourceDecisionID != "" {
				existing.SourceDecisionID = sourceDecisionID
			}
			if workspacePath := item.WorkspacePath; workspacePath != "" {
				existing.WorkspacePath = workspacePath
			}
			if worktreePath := strings.TrimSpace(body.WorktreePath); worktreePath != "" {
				existing.WorktreePath = worktreePath
			}
			if worktreeBranch := strings.TrimSpace(body.WorktreeBranch); worktreeBranch != "" {
				existing.WorktreeBranch = worktreeBranch
			}
			if existing.ThreadID == "" && strings.TrimSpace(body.ThreadID) != "" {
				existing.ThreadID = strings.TrimSpace(body.ThreadID)
			}
			existing.DependsOn = append([]string(nil), item.ResolvedDepIDs...)
			b.ensureTaskOwnerChannelMembershipLocked(channel, existing.Owner)
			existing.UpdatedAt = now
			b.scheduleTaskLifecycleLocked(existing)
			if err := b.syncTaskWorktreeLocked(existing); err != nil {
				http.Error(w, "failed to manage task worktree", http.StatusInternalServerError)
				return
			}
			b.linkTaskWorkspaceToChannelLocked(channel, existing)
			issueQueued, prQueued := b.maybeQueueTaskGitHubPublicationLocked(existing, previousStatus, action, nil)
			b.appendActionLocked("task_updated", "office", channel, strings.TrimSpace(body.CreatedBy), truncateSummary(existing.Title+" ["+existing.Status+"]", 140), existing.ID)
			if err := b.saveLocked(); err != nil {
				http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
				return
			}
			if issueQueued {
				b.publishTaskIssueSoon(existing.ID)
			}
			if prQueued {
				b.publishTaskPRSoon(existing.ID)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"task": *existing})
			return
		}
		if recent := b.findRecentTerminalTaskLocked(match); recent != nil {
			http.Error(w, recentTaskConflict(recent).Error(), http.StatusConflict)
			return
		}
		b.counter++
		task := teamTask{
			ID:               fmt.Sprintf("task-%d", b.counter),
			Channel:          item.Channel,
			ExecutionKey:     item.ExecutionKey,
			Title:            item.Title,
			Details:          item.Details,
			Owner:            item.Owner,
			Status:           "open",
			CreatedBy:        strings.TrimSpace(body.CreatedBy),
			ThreadID:         strings.TrimSpace(body.ThreadID),
			TaskType:         item.TaskType,
			PipelineID:       strings.TrimSpace(body.PipelineID),
			ExecutionMode:    item.ExecutionMode,
			ReviewState:      firstNonEmpty(strings.TrimSpace(body.ReviewState), item.ReviewState),
			SourceSignalID:   strings.TrimSpace(body.SourceSignalID),
			SourceDecisionID: strings.TrimSpace(body.SourceDecisionID),
			WorkspacePath:    item.WorkspacePath,
			WorktreePath:     strings.TrimSpace(body.WorktreePath),
			WorktreeBranch:   strings.TrimSpace(body.WorktreeBranch),
			DependsOn:        append([]string(nil), item.ResolvedDepIDs...),
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if len(task.DependsOn) > 0 && b.hasUnresolvedDepsLocked(&task) {
			task.Blocked = true
		} else if task.Owner != "" {
			task.Status = "in_progress"
		}
		b.ensureTaskOwnerChannelMembershipLocked(channel, task.Owner)
		b.queueTaskBehindActiveOwnerLaneLocked(&task)
		if err := rejectTheaterTaskForLiveBusiness(&task); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		b.scheduleTaskLifecycleLocked(&task)
		if err := b.syncTaskWorktreeLocked(&task); err != nil {
			http.Error(w, "failed to manage task worktree", http.StatusInternalServerError)
			return
		}
		b.linkTaskWorkspaceToChannelLocked(channel, &task)
		issueQueued, prQueued := b.maybeQueueTaskGitHubPublicationLocked(&task, "", action, nil)
		b.tasks = append(b.tasks, task)
		b.appendActionLocked("task_created", "office", channel, task.CreatedBy, truncateSummary(task.Title, 140), task.ID)
		if err := b.saveLocked(); err != nil {
			http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
			return
		}
		if issueQueued {
			b.publishTaskIssueSoon(task.ID)
		}
		if prQueued {
			b.publishTaskPRSoon(task.ID)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"task": task})
		return
	}

	requestedID := strings.TrimSpace(body.ID)
	for i := range b.tasks {
		if b.tasks[i].ID != requestedID {
			continue
		}
		task := &b.tasks[i]
		taskChannel := normalizeChannelSlug(task.Channel)
		previousStatus := task.Status
		handoff, err := b.validateTaskMutationContractLocked(task, action, strings.TrimSpace(body.CreatedBy), body.Details)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if _, ok := taskTransitionRuleForAction(action); !ok {
			http.Error(w, "unknown action", http.StatusBadRequest)
			return
		}
		reassignPrevOwner := ""
		reassignTriggered := false
		cancelTriggered := false
		cancelPrevOwner := ""
		derivedIssueTaskIDs := make([]string, 0, 2)
		switch action {
		case "claim", "assign", "reassign":
			if strings.TrimSpace(body.Owner) == "" {
				http.Error(w, "owner required", http.StatusBadRequest)
				return
			}
		}
		if action == "block" {
			if err := rejectFalseLocalWorktreeBlock(task, body.Details); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
		}
		transition, err := resolveTaskTransition(task, action, handoff)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		switch action {
		case "claim", "assign":
			task.Owner = strings.TrimSpace(body.Owner)
		case "reassign":
			reassignPrevOwner = strings.TrimSpace(task.Owner)
			newOwner := strings.TrimSpace(body.Owner)
			task.Owner = newOwner
			reassignTriggered = reassignPrevOwner != newOwner
		case "release":
			task.Owner = ""
		case "cancel":
			cancelPrevOwner = strings.TrimSpace(task.Owner)
			cancelTriggered = true
		}
		if transition.ClearOwner {
			task.Owner = ""
		}
		if transition.ClearReconciliation {
			clearTaskReconciliationLocked(task)
		}
		if transition.ClearSchedule {
			task.FollowUpAt = ""
			task.ReminderAt = ""
			task.RecheckAt = ""
		}
		task.Status = transition.Status
		task.ReviewState = transition.ReviewState
		task.Blocked = transition.Blocked
		if strings.TrimSpace(body.Details) != "" {
			task.Details = strings.TrimSpace(body.Details)
		}
		if taskType := strings.TrimSpace(body.TaskType); taskType != "" {
			task.TaskType = taskType
		}
		if pipelineID := strings.TrimSpace(body.PipelineID); pipelineID != "" {
			task.PipelineID = pipelineID
		}
		if executionMode := strings.TrimSpace(body.ExecutionMode); executionMode != "" {
			task.ExecutionMode = executionMode
		}
		if sourceSignalID := strings.TrimSpace(body.SourceSignalID); sourceSignalID != "" {
			task.SourceSignalID = sourceSignalID
		}
		if sourceDecisionID := strings.TrimSpace(body.SourceDecisionID); sourceDecisionID != "" {
			task.SourceDecisionID = sourceDecisionID
		}
		if handoff != nil && taskActionResubmitsForReview(action) && taskHasBlockingReviewFindings(task) {
			resolveOpenTaskReviewFindingsLocked(task, strings.TrimSpace(body.CreatedBy), now)
		}
		if handoff != nil {
			b.acceptTaskHandoffLocked(task, strings.TrimSpace(body.CreatedBy), handoff, now)
		}
		if workspacePath := strings.TrimSpace(body.WorkspacePath); workspacePath != "" {
			task.WorkspacePath = workspacePath
		}
		if worktreePath := strings.TrimSpace(body.WorktreePath); worktreePath != "" {
			task.WorktreePath = worktreePath
		}
		if worktreeBranch := strings.TrimSpace(body.WorktreeBranch); worktreeBranch != "" {
			task.WorktreeBranch = worktreeBranch
		}
		if action == "block" && handoff != nil {
			requestIDs, err := b.createTaskBlockerRequestsLocked(task, strings.TrimSpace(body.CreatedBy), handoff, now)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			task.BlockerRequestIDs = append([]string(nil), requestIDs...)
		}
		if handoff != nil {
			var err error
			derivedIssueTaskIDs, err = b.deriveMarkedDemandFollowUpsLocked(task, strings.TrimSpace(body.CreatedBy), handoff, now)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		b.ensureTaskOwnerChannelMembershipLocked(taskChannel, task.Owner)
		b.queueTaskBehindActiveOwnerLaneLocked(task)
		task.UpdatedAt = now
		if err := rejectTheaterTaskForLiveBusiness(task); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if task.Status == "done" {
			b.unblockDependentsLocked(task.ID)
		}
		b.scheduleTaskLifecycleLocked(task)
		if task.Status == "open" {
			task.ReviewState = ""
		}
		if err := b.syncTaskWorktreeLocked(task); err != nil {
			http.Error(w, "failed to manage task worktree", http.StatusInternalServerError)
			return
		}
		b.linkTaskWorkspaceToChannelLocked(taskChannel, task)
		issueQueued, prQueued := b.maybeQueueTaskGitHubPublicationLocked(task, previousStatus, action, handoff)
		b.appendActionLocked("task_updated", "office", taskChannel, strings.TrimSpace(body.CreatedBy), truncateSummary(task.Title+" ["+task.Status+"]", 140), task.ID)
		if reassignTriggered {
			b.postTaskReassignNotificationsLocked(strings.TrimSpace(body.CreatedBy), task, reassignPrevOwner)
		}
		if cancelTriggered {
			b.postTaskCancelNotificationsLocked(strings.TrimSpace(body.CreatedBy), task, cancelPrevOwner)
		}
		if err := b.saveLocked(); err != nil {
			http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
			return
		}
		if issueQueued {
			b.publishTaskIssueSoon(task.ID)
		}
		for _, derivedTaskID := range derivedIssueTaskIDs {
			b.publishTaskIssueSoon(derivedTaskID)
		}
		if prQueued {
			b.publishTaskPRSoon(task.ID)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"task": *task})
		return
	}

	http.Error(w, "task not found", http.StatusNotFound)
}

// postTaskReassignNotificationsLocked posts the channel announcement plus DMs
// to the new owner and previous owner whenever a task ownership change happens.
// The CEO is tagged in the channel message rather than DM'd (CEO is the human
// user; human↔ceo self-DM is not a valid DM target).
//
// Must be called while b.mu is held for write.
func (b *Broker) postTaskReassignNotificationsLocked(actor string, task *teamTask, prevOwner string) {
	if task == nil {
		return
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "system"
	}
	newOwner := strings.TrimSpace(task.Owner)
	prevOwner = strings.TrimSpace(prevOwner)
	if newOwner == prevOwner {
		return
	}
	taskChannel := normalizeChannelSlug(task.Channel)
	if taskChannel == "" {
		taskChannel = "general"
	}
	title := strings.TrimSpace(task.Title)
	if title == "" {
		title = task.ID
	}
	now := time.Now().UTC().Format(time.RFC3339)

	newLabel := "(unassigned)"
	if newOwner != "" {
		newLabel = "@" + newOwner
	}
	prevLabel := "(unassigned)"
	if prevOwner != "" {
		prevLabel = "@" + prevOwner
	}

	b.counter++
	b.appendMessageLocked(channelMessage{
		ID:        fmt.Sprintf("msg-%d", b.counter),
		From:      actor,
		Channel:   taskChannel,
		Kind:      "task_reassigned",
		Title:     title,
		Content:   fmt.Sprintf("Task %q reassigned: %s → %s. (by @%s, cc @ceo)", title, prevLabel, newLabel, actor),
		Tagged:    dedupeReassignTags([]string{"ceo", newOwner, prevOwner}),
		Timestamp: now,
	})

	if isDMTargetSlug(newOwner) {
		b.postTaskDMLocked(actor, newOwner, "task_reassigned", title,
			fmt.Sprintf("Task %q is yours now. Details live in #%s.", title, taskChannel))
	}
	if isDMTargetSlug(prevOwner) && prevOwner != newOwner {
		b.postTaskDMLocked(actor, prevOwner, "task_reassigned", title,
			fmt.Sprintf("Task %q is off your plate — it moved to %s.", title, newLabel))
	}
}

// postTaskCancelNotificationsLocked posts a channel announcement plus a DM
// to the (previous) owner whenever a task is closed as "won't do".
// Must be called while b.mu is held for write.
func (b *Broker) postTaskCancelNotificationsLocked(actor string, task *teamTask, prevOwner string) {
	if task == nil {
		return
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "system"
	}
	prevOwner = strings.TrimSpace(prevOwner)
	taskChannel := normalizeChannelSlug(task.Channel)
	if taskChannel == "" {
		taskChannel = "general"
	}
	title := strings.TrimSpace(task.Title)
	if title == "" {
		title = task.ID
	}
	now := time.Now().UTC().Format(time.RFC3339)

	ownerLabel := "(no owner)"
	if prevOwner != "" {
		ownerLabel = "@" + prevOwner
	}

	b.counter++
	b.appendMessageLocked(channelMessage{
		ID:        fmt.Sprintf("msg-%d", b.counter),
		From:      actor,
		Channel:   taskChannel,
		Kind:      "task_canceled",
		Title:     title,
		Content:   fmt.Sprintf("Task %q closed as won't do. Owner was %s. (by @%s, cc @ceo)", title, ownerLabel, actor),
		Tagged:    dedupeReassignTags([]string{"ceo", prevOwner}),
		Timestamp: now,
	})

	if isDMTargetSlug(prevOwner) {
		b.postTaskDMLocked(actor, prevOwner, "task_canceled", title,
			fmt.Sprintf("Heads up — task %q was closed as won't do. Take it off your list.", title))
	}
}

// postTaskDMLocked appends a direct-message notification to the DM channel
// between "human" and targetSlug, creating the channel if necessary.
// Must be called while b.mu is held for write.
func (b *Broker) postTaskDMLocked(from, targetSlug, kind, title, content string) {
	targetSlug = strings.TrimSpace(targetSlug)
	if targetSlug == "" || b.channelStore == nil {
		return
	}
	ch, err := b.channelStore.GetOrCreateDirect("human", targetSlug)
	if err != nil {
		return
	}
	if b.findChannelLocked(ch.Slug) == nil {
		now := time.Now().UTC().Format(time.RFC3339)
		b.channels = append(b.channels, teamChannel{
			Slug:        ch.Slug,
			Name:        ch.Slug,
			Type:        "dm",
			Description: "Direct messages with " + targetSlug,
			Members:     []string{"human", targetSlug},
			CreatedBy:   "wuphf",
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}
	b.counter++
	b.appendMessageLocked(channelMessage{
		ID:        fmt.Sprintf("msg-%d", b.counter),
		From:      strings.TrimSpace(from),
		Channel:   ch.Slug,
		Kind:      strings.TrimSpace(kind),
		Title:     title,
		Content:   content,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// isDMTargetSlug reports whether slug is a valid recipient for a human-to-agent DM.
// The human user ("human"/"you") and the CEO seat ("ceo", which is the human)
// are excluded because they would create self-DMs.
func isDMTargetSlug(slug string) bool {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return false
	}
	switch slug {
	case "human", "you", "ceo":
		return false
	}
	return true
}

func dedupeReassignTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

func (b *Broker) BlockTask(taskID, actor, reason string) (teamTask, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := strings.TrimSpace(taskID)
	if id == "" {
		return teamTask{}, false, fmt.Errorf("task id required")
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "system"
	}
	reason = strings.TrimSpace(reason)
	now := time.Now().UTC().Format(time.RFC3339)

	for i := range b.tasks {
		task := &b.tasks[i]
		if task.ID != id {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(task.Status))
		if status == "done" || status == "completed" || status == "canceled" || status == "cancelled" {
			return *task, false, nil
		}
		if err := rejectFalseLocalWorktreeBlock(task, reason); err != nil {
			return *task, false, err
		}
		if reason != "" {
			switch existing := strings.TrimSpace(task.Details); {
			case existing == "":
				task.Details = reason
			case !strings.Contains(existing, reason):
				task.Details = existing + "\n\n" + reason
			}
		}
		task.Status = "blocked"
		task.Blocked = true
		task.UpdatedAt = now
		if err := rejectTheaterTaskForLiveBusiness(task); err != nil {
			return *task, false, err
		}
		b.scheduleTaskLifecycleLocked(task)
		if err := b.syncTaskWorktreeLocked(task); err != nil {
			return teamTask{}, false, err
		}
		b.appendActionLocked("task_updated", "office", normalizeChannelSlug(task.Channel), actor, truncateSummary(task.Title+" ["+task.Status+"]", 140), task.ID)
		if err := b.saveLocked(); err != nil {
			return teamTask{}, false, err
		}
		return *task, true, nil
	}

	return teamTask{}, false, fmt.Errorf("task not found")
}

func (b *Broker) ResumeTask(taskID, actor, reason string) (teamTask, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := strings.TrimSpace(taskID)
	if id == "" {
		return teamTask{}, false, fmt.Errorf("task id required")
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "system"
	}
	reason = strings.TrimSpace(reason)
	now := time.Now().UTC().Format(time.RFC3339)

	for i := range b.tasks {
		task := &b.tasks[i]
		if task.ID != id {
			continue
		}
		changed, err := b.resumeTaskLocked(task, actor, reason, now)
		if err != nil {
			return teamTask{}, false, err
		}
		if !changed {
			return *task, false, nil
		}
		if err := b.saveLocked(); err != nil {
			return teamTask{}, false, err
		}
		return *task, true, nil
	}

	return teamTask{}, false, fmt.Errorf("task not found")
}

func (b *Broker) resumeTaskLocked(task *teamTask, actor, reason, now string) (bool, error) {
	if task == nil {
		return false, fmt.Errorf("task required")
	}
	if taskHasPendingReconciliation(task) {
		return false, fmt.Errorf("task %s has reconciliation pending and cannot resume until it is reconciled", task.ID)
	}
	changed := false
	if task.Blocked {
		task.Blocked = false
		changed = true
	}
	if strings.EqualFold(strings.TrimSpace(task.Status), "blocked") {
		if strings.TrimSpace(task.Owner) != "" {
			task.Status = "in_progress"
		} else {
			task.Status = "open"
		}
		changed = true
	}
	if !changed {
		return false, nil
	}
	if reason != "" && !strings.Contains(task.Details, reason) {
		task.Details = strings.TrimSpace(task.Details)
		if task.Details != "" {
			task.Details += "\n\n"
		}
		task.Details += reason
	}
	b.ensureTaskOwnerChannelMembershipLocked(task.Channel, task.Owner)
	b.queueTaskBehindActiveOwnerLaneLocked(task)
	task.UpdatedAt = now
	b.reopenExecutionNodeForTaskLocked(task, task.Owner, now)
	b.resolveWatchdogAlertsLocked("task", task.ID, task.Channel)
	b.scheduleTaskLifecycleLocked(task)
	if err := b.syncTaskWorktreeLocked(task); err != nil {
		return false, err
	}
	b.appendActionLocked("task_unblocked", "office", normalizeChannelSlug(task.Channel), actor, truncateSummary(task.Title+" resumed", 140), task.ID)
	return true, nil
}

func (b *Broker) handleTaskPlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Channel   string `json:"channel"`
		CreatedBy string `json:"created_by"`
		Tasks     []struct {
			ExecutionKey  string   `json:"execution_key"`
			Title         string   `json:"title"`
			Assignee      string   `json:"assignee"`
			Details       string   `json:"details"`
			TaskType      string   `json:"task_type"`
			ExecutionMode string   `json:"execution_mode"`
			WorkspacePath string   `json:"workspace_path"`
			DependsOn     []string `json:"depends_on"`
		} `json:"tasks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	createdBy := strings.TrimSpace(body.CreatedBy)
	if createdBy == "" || len(body.Tasks) == 0 {
		http.Error(w, "created_by and tasks required", http.StatusBadRequest)
		return
	}
	channel := normalizeChannelSlug(body.Channel)
	if channel == "" {
		channel = "general"
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.findChannelLocked(channel) == nil {
		http.Error(w, "channel not found", http.StatusNotFound)
		return
	}

	specs := make([]plannedTaskSpec, 0, len(body.Tasks))
	for _, item := range body.Tasks {
		specs = append(specs, plannedTaskSpec{
			ExecutionKey:  item.ExecutionKey,
			Title:         item.Title,
			Assignee:      item.Assignee,
			Details:       item.Details,
			TaskType:      item.TaskType,
			ExecutionMode: item.ExecutionMode,
			WorkspacePath: item.WorkspacePath,
			DependsOn:     append([]string(nil), item.DependsOn...),
		})
	}
	validated, err := b.validateStrictTaskPlanLocked(channel, createdBy, specs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	finalIDs := make(map[string]string, len(validated))
	reused := make(map[string]*teamTask, len(validated))
	nextCounter := b.counter
	for _, item := range validated {
		match := taskReuseMatch{
			Channel:       item.Channel,
			ExecutionKey:  item.ExecutionKey,
			Title:         item.Title,
			Details:       item.Details,
			Owner:         item.Owner,
			TaskType:      item.TaskType,
			ExecutionMode: item.ExecutionMode,
			WorkspacePath: item.WorkspacePath,
		}
		if existing := b.findReusableTaskLocked(match); existing != nil {
			reused[item.PlannedID] = existing
			finalIDs[item.PlannedID] = existing.ID
			continue
		}
		if recent := b.findRecentTerminalTaskLocked(match); recent != nil {
			http.Error(w, recentTaskConflict(recent).Error(), http.StatusConflict)
			return
		}
		nextCounter++
		finalIDs[item.PlannedID] = fmt.Sprintf("task-%d", nextCounter)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	created := make([]teamTask, 0, len(validated))
	issueTaskIDs := make([]string, 0, len(validated))
	for _, item := range validated {
		resolvedDeps := make([]string, 0, len(item.ResolvedDepIDs))
		for _, depID := range item.ResolvedDepIDs {
			if mapped, ok := finalIDs[depID]; ok {
				resolvedDeps = append(resolvedDeps, mapped)
				continue
			}
			resolvedDeps = append(resolvedDeps, depID)
		}

		if existing := reused[item.PlannedID]; existing != nil {
			previousStatus := existing.Status
			if details := strings.TrimSpace(item.Details); details != "" {
				existing.Details = details
			}
			if existing.ExecutionKey == "" && item.ExecutionKey != "" {
				existing.ExecutionKey = item.ExecutionKey
			}
			if taskType := strings.TrimSpace(item.TaskType); taskType != "" {
				existing.TaskType = taskType
			}
			if pipelineID := strings.TrimSpace(item.PipelineID); pipelineID != "" {
				existing.PipelineID = pipelineID
			}
			if executionMode := strings.TrimSpace(item.ExecutionMode); executionMode != "" {
				existing.ExecutionMode = executionMode
			}
			if workspacePath := strings.TrimSpace(item.WorkspacePath); workspacePath != "" {
				existing.WorkspacePath = workspacePath
			}
			existing.DependsOn = resolvedDeps
			if len(existing.DependsOn) > 0 && b.hasUnresolvedDepsLocked(existing) {
				existing.Blocked = true
				existing.Status = "open"
			} else if strings.TrimSpace(existing.Owner) != "" {
				existing.Blocked = false
				existing.Status = "in_progress"
			}
			b.ensureTaskOwnerChannelMembershipLocked(item.Channel, existing.Owner)
			b.queueTaskBehindActiveOwnerLaneLocked(existing)
			existing.UpdatedAt = now
			if err := rejectTheaterTaskForLiveBusiness(existing); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			b.scheduleTaskLifecycleLocked(existing)
			if err := b.syncTaskWorktreeLocked(existing); err != nil {
				http.Error(w, "failed to manage task worktree", http.StatusInternalServerError)
				return
			}
			if issueQueued, _ := b.maybeQueueTaskGitHubPublicationLocked(existing, previousStatus, "create", nil); issueQueued {
				issueTaskIDs = append(issueTaskIDs, existing.ID)
			}
			b.appendActionLocked("task_updated", "office", item.Channel, createdBy, truncateSummary(existing.Title+" ["+existing.Status+"]", 140), existing.ID)
			created = append(created, *existing)
			continue
		}

		task := teamTask{
			ID:            finalIDs[item.PlannedID],
			Channel:       item.Channel,
			ExecutionKey:  item.ExecutionKey,
			Title:         item.Title,
			Details:       item.Details,
			Owner:         item.Owner,
			Status:        "open",
			CreatedBy:     createdBy,
			TaskType:      item.TaskType,
			PipelineID:    item.PipelineID,
			ExecutionMode: item.ExecutionMode,
			ReviewState:   item.ReviewState,
			WorkspacePath: item.WorkspacePath,
			DependsOn:     resolvedDeps,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if task.Owner != "" && len(resolvedDeps) == 0 {
			task.Status = "in_progress"
		}
		if len(resolvedDeps) > 0 && b.hasUnresolvedDepsLocked(&task) {
			task.Blocked = true
		}
		b.ensureTaskOwnerChannelMembershipLocked(item.Channel, task.Owner)
		b.queueTaskBehindActiveOwnerLaneLocked(&task)
		if err := rejectTheaterTaskForLiveBusiness(&task); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		b.scheduleTaskLifecycleLocked(&task)
		if err := b.syncTaskWorktreeLocked(&task); err != nil {
			http.Error(w, "failed to manage task worktree", http.StatusInternalServerError)
			return
		}
		if issueQueued, _ := b.maybeQueueTaskGitHubPublicationLocked(&task, "", "create", nil); issueQueued {
			issueTaskIDs = append(issueTaskIDs, task.ID)
		}
		b.tasks = append(b.tasks, task)
		b.appendActionLocked("task_created", "office", item.Channel, createdBy, truncateSummary(task.Title, 140), task.ID)
		created = append(created, task)
	}

	b.counter = nextCounter

	if err := b.saveLocked(); err != nil {
		http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
		return
	}
	for _, taskID := range compactStringList(issueTaskIDs) {
		b.publishTaskIssueSoon(taskID)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"tasks": created})
}

func (b *Broker) handleMemory(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		namespace := strings.TrimSpace(r.URL.Query().Get("namespace"))
		query := strings.TrimSpace(r.URL.Query().Get("query"))
		keyFilter := strings.TrimSpace(r.URL.Query().Get("key"))
		limit := 5
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		b.mu.Lock()
		mem := b.sharedMemory
		b.mu.Unlock()
		if mem == nil {
			mem = make(map[string]map[string]string)
		}
		w.Header().Set("Content-Type", "application/json")
		if namespace != "" {
			entries := mem[namespace]
			switch {
			case keyFilter != "":
				var payload []brokerMemoryEntry
				if raw, ok := entries[keyFilter]; ok {
					payload = append(payload, brokerEntryFromNote(decodePrivateMemoryNote(keyFilter, raw)))
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"namespace": namespace,
					"entries":   payload,
				})
				return
			case query != "":
				matches := searchPrivateMemory(entries, query, limit)
				payload := make([]brokerMemoryEntry, 0, len(matches))
				for _, note := range matches {
					payload = append(payload, brokerEntryFromNote(note))
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"namespace": namespace,
					"entries":   payload,
				})
				return
			default:
				matches := searchPrivateMemory(entries, "", len(entries))
				payload := make([]brokerMemoryEntry, 0, len(matches))
				for _, note := range matches {
					payload = append(payload, brokerEntryFromNote(note))
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"namespace": namespace,
					"entries":   payload,
				})
				return
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"memory": mem})
	case http.MethodPost:
		var body struct {
			Namespace string `json:"namespace"`
			Key       string `json:"key"`
			Value     any    `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		ns := strings.TrimSpace(body.Namespace)
		key := strings.TrimSpace(body.Key)
		if ns == "" || key == "" {
			http.Error(w, "namespace and key required", http.StatusBadRequest)
			return
		}
		b.mu.Lock()
		if b.sharedMemory == nil {
			b.sharedMemory = make(map[string]map[string]string)
		}
		if b.sharedMemory[ns] == nil {
			b.sharedMemory[ns] = make(map[string]string)
		}
		value := ""
		switch typed := body.Value.(type) {
		case string:
			value = typed
		default:
			data, err := json.Marshal(typed)
			if err != nil {
				b.mu.Unlock()
				http.Error(w, "invalid value", http.StatusBadRequest)
				return
			}
			value = string(data)
		}
		b.sharedMemory[ns][key] = value
		if err := b.saveLocked(); err != nil {
			b.mu.Unlock()
			http.Error(w, "failed to persist", http.StatusInternalServerError)
			return
		}
		b.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "namespace": ns, "key": key})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *Broker) handleTaskAck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ID      string `json:"id"`
		Channel string `json:"channel"`
		Slug    string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	taskID := strings.TrimSpace(body.ID)
	slug := strings.TrimSpace(body.Slug)
	channel := normalizeChannelSlug(body.Channel)
	if channel == "" {
		channel = "general"
	}
	if taskID == "" || slug == "" {
		http.Error(w, "id and slug required", http.StatusBadRequest)
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.tasks {
		if b.tasks[i].ID == taskID && normalizeChannelSlug(b.tasks[i].Channel) == channel {
			if b.tasks[i].Owner != slug {
				http.Error(w, "only the task owner can ack", http.StatusForbidden)
				return
			}
			now := time.Now().UTC().Format(time.RFC3339)
			b.tasks[i].AckedAt = now
			b.tasks[i].UpdatedAt = now
			if err := b.saveLocked(); err != nil {
				http.Error(w, "failed to persist", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"task": b.tasks[i]})
			return
		}
	}
	http.Error(w, "task not found", http.StatusNotFound)
}

func (b *Broker) EnsureTask(channel, title, details, owner, createdBy, threadID string, dependsOn ...string) (teamTask, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	channel = b.preferredTaskChannelLocked(channel, createdBy, owner, title, details)
	if b.findChannelLocked(channel) == nil {
		return teamTask{}, false, fmt.Errorf("channel not found")
	}
	if !b.canAccessChannelLocked(createdBy, channel) {
		return teamTask{}, false, fmt.Errorf("channel access denied")
	}
	createdBy = strings.TrimSpace(createdBy)
	title = strings.TrimSpace(title)
	details = strings.TrimSpace(details)
	owner = strings.TrimSpace(owner)
	threadID = strings.TrimSpace(threadID)
	validated, err := b.validateStrictTaskPlanLocked(channel, createdBy, []plannedTaskSpec{{
		Title:     title,
		Assignee:  owner,
		Details:   details,
		DependsOn: append([]string(nil), dependsOn...),
	}})
	if err != nil {
		return teamTask{}, false, err
	}
	item := validated[0]
	match := taskReuseMatch{
		Channel:  item.Channel,
		Title:    item.Title,
		Details:  item.Details,
		ThreadID: threadID,
		Owner:    item.Owner,
	}
	if existing := b.findReusableTaskLocked(match); existing != nil {
		previousStatus := existing.Status
		if existing.Details == "" && item.Details != "" {
			existing.Details = item.Details
		}
		if existing.Owner == "" && item.Owner != "" {
			existing.Owner = item.Owner
			if !existing.Blocked {
				existing.Status = "in_progress"
			}
		}
		if existing.TaskType == "" && item.TaskType != "" {
			existing.TaskType = item.TaskType
		}
		if existing.PipelineID == "" && item.PipelineID != "" {
			existing.PipelineID = item.PipelineID
		}
		if existing.ExecutionMode == "" && item.ExecutionMode != "" {
			existing.ExecutionMode = item.ExecutionMode
		}
		if existing.ReviewState == "" && item.ReviewState != "" {
			existing.ReviewState = item.ReviewState
		}
		if existing.ThreadID == "" && threadID != "" {
			existing.ThreadID = threadID
		}
		existing.DependsOn = append([]string(nil), item.ResolvedDepIDs...)
		b.ensureTaskOwnerChannelMembershipLocked(item.Channel, existing.Owner)
		existing.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		b.queueTaskBehindActiveOwnerLaneLocked(existing)
		if err := rejectTheaterTaskForLiveBusiness(existing); err != nil {
			return teamTask{}, false, err
		}
		b.scheduleTaskLifecycleLocked(existing)
		if err := b.syncTaskWorktreeLocked(existing); err != nil {
			return teamTask{}, false, err
		}
		b.linkTaskWorkspaceToChannelLocked(item.Channel, existing)
		issueQueued, prQueued := b.maybeQueueTaskGitHubPublicationLocked(existing, previousStatus, "create", nil)
		if err := b.saveLocked(); err != nil {
			return teamTask{}, false, err
		}
		if issueQueued {
			b.publishTaskIssueSoon(existing.ID)
		}
		if prQueued {
			b.publishTaskPRSoon(existing.ID)
		}
		return *existing, true, nil
	}
	if recent := b.findRecentTerminalTaskLocked(match); recent != nil {
		return teamTask{}, false, recentTaskConflict(recent)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	b.counter++
	task := teamTask{
		ID:            fmt.Sprintf("task-%d", b.counter),
		Channel:       item.Channel,
		Title:         item.Title,
		Details:       item.Details,
		Owner:         item.Owner,
		Status:        "open",
		CreatedBy:     createdBy,
		ThreadID:      threadID,
		TaskType:      item.TaskType,
		PipelineID:    item.PipelineID,
		ExecutionMode: item.ExecutionMode,
		ReviewState:   item.ReviewState,
		DependsOn:     append([]string(nil), item.ResolvedDepIDs...),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if len(task.DependsOn) > 0 && b.hasUnresolvedDepsLocked(&task) {
		task.Blocked = true
	} else if task.Owner != "" {
		task.Status = "in_progress"
	}
	b.ensureTaskOwnerChannelMembershipLocked(item.Channel, task.Owner)
	b.queueTaskBehindActiveOwnerLaneLocked(&task)
	if err := rejectTheaterTaskForLiveBusiness(&task); err != nil {
		return teamTask{}, false, err
	}
	b.scheduleTaskLifecycleLocked(&task)
	if err := b.syncTaskWorktreeLocked(&task); err != nil {
		return teamTask{}, false, err
	}
	b.linkTaskWorkspaceToChannelLocked(item.Channel, &task)
	issueQueued, prQueued := b.maybeQueueTaskGitHubPublicationLocked(&task, "", "create", nil)
	b.tasks = append(b.tasks, task)
	b.appendActionLocked("task_created", "office", channel, createdBy, truncateSummary(task.Title, 140), task.ID)
	if err := b.saveLocked(); err != nil {
		return teamTask{}, false, err
	}
	if issueQueued {
		b.publishTaskIssueSoon(task.ID)
	}
	if prQueued {
		b.publishTaskPRSoon(task.ID)
	}
	return task, false, nil
}

type plannedTaskInput struct {
	Channel          string
	ExecutionKey     string
	Title            string
	Details          string
	Owner            string
	CreatedBy        string
	ThreadID         string
	TaskType         string
	PipelineID       string
	ExecutionMode    string
	ReviewState      string
	SourceSignalID   string
	SourceDecisionID string
	WorkspacePath    string
	DependsOn        []string
}

func (b *Broker) EnsurePlannedTask(input plannedTaskInput) (teamTask, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	channel := b.preferredTaskChannelLocked(input.Channel, input.CreatedBy, input.Owner, input.Title, input.Details)
	if b.findChannelLocked(channel) == nil {
		return teamTask{}, false, fmt.Errorf("channel not found")
	}
	if !b.canAccessChannelLocked(input.CreatedBy, channel) {
		return teamTask{}, false, fmt.Errorf("channel access denied")
	}
	input.CreatedBy = strings.TrimSpace(input.CreatedBy)
	input.ThreadID = strings.TrimSpace(input.ThreadID)
	input.PipelineID = strings.TrimSpace(input.PipelineID)
	input.ReviewState = strings.TrimSpace(input.ReviewState)
	input.SourceSignalID = strings.TrimSpace(input.SourceSignalID)
	input.SourceDecisionID = strings.TrimSpace(input.SourceDecisionID)
	validated, err := b.validateStrictTaskPlanLocked(channel, input.CreatedBy, []plannedTaskSpec{{
		ExecutionKey:  input.ExecutionKey,
		Title:         input.Title,
		Assignee:      input.Owner,
		Details:       input.Details,
		TaskType:      input.TaskType,
		ExecutionMode: input.ExecutionMode,
		WorkspacePath: input.WorkspacePath,
		DependsOn:     append([]string(nil), input.DependsOn...),
	}})
	if err != nil {
		return teamTask{}, false, err
	}
	item := validated[0]
	match := taskReuseMatch{
		Channel:          item.Channel,
		ExecutionKey:     normalizeExecutionKey(input.ExecutionKey),
		Title:            item.Title,
		Details:          item.Details,
		ThreadID:         input.ThreadID,
		Owner:            item.Owner,
		TaskType:         item.TaskType,
		PipelineID:       input.PipelineID,
		ExecutionMode:    strings.TrimSpace(input.ExecutionMode),
		WorkspacePath:    item.WorkspacePath,
		SourceSignalID:   input.SourceSignalID,
		SourceDecisionID: input.SourceDecisionID,
	}
	if existing := b.findReusableTaskLocked(match); existing != nil {
		previousStatus := existing.Status
		if existing.Details == "" && item.Details != "" {
			existing.Details = item.Details
		}
		if existing.Owner == "" && item.Owner != "" {
			existing.Owner = item.Owner
			if !taskIsTerminal(existing) {
				existing.Status = "in_progress"
			}
		}
		if existing.ExecutionKey == "" && normalizeExecutionKey(input.ExecutionKey) != "" {
			existing.ExecutionKey = normalizeExecutionKey(input.ExecutionKey)
		}
		if existing.ThreadID == "" && input.ThreadID != "" {
			existing.ThreadID = input.ThreadID
		}
		if existing.TaskType == "" && item.TaskType != "" {
			existing.TaskType = item.TaskType
		}
		if existing.PipelineID == "" && item.PipelineID != "" {
			existing.PipelineID = item.PipelineID
		}
		if existing.ExecutionMode == "" && item.ExecutionMode != "" {
			existing.ExecutionMode = item.ExecutionMode
		}
		if existing.ReviewState == "" && firstNonEmpty(input.ReviewState, item.ReviewState) != "" {
			existing.ReviewState = firstNonEmpty(input.ReviewState, item.ReviewState)
		}
		if existing.SourceSignalID == "" && input.SourceSignalID != "" {
			existing.SourceSignalID = input.SourceSignalID
		}
		if existing.SourceDecisionID == "" && input.SourceDecisionID != "" {
			existing.SourceDecisionID = input.SourceDecisionID
		}
		if existing.ExecutionKey == "" {
			existing.ExecutionKey = deriveTaskExecutionKey(existing)
		}
		if existing.WorkspacePath == "" && item.WorkspacePath != "" {
			existing.WorkspacePath = item.WorkspacePath
		}
		existing.DependsOn = append([]string(nil), item.ResolvedDepIDs...)
		b.ensureTaskOwnerChannelMembershipLocked(item.Channel, existing.Owner)
		existing.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		b.queueTaskBehindActiveOwnerLaneLocked(existing)
		if err := rejectTheaterTaskForLiveBusiness(existing); err != nil {
			return teamTask{}, false, err
		}
		b.scheduleTaskLifecycleLocked(existing)
		if err := b.syncTaskWorktreeLocked(existing); err != nil {
			return teamTask{}, false, err
		}
		b.linkTaskWorkspaceToChannelLocked(item.Channel, existing)
		issueQueued, prQueued := b.maybeQueueTaskGitHubPublicationLocked(existing, previousStatus, "create", nil)
		if err := b.saveLocked(); err != nil {
			return teamTask{}, false, err
		}
		if issueQueued {
			b.publishTaskIssueSoon(existing.ID)
		}
		if prQueued {
			b.publishTaskPRSoon(existing.ID)
		}
		return *existing, true, nil
	}
	if recent := b.findRecentTerminalTaskLocked(match); recent != nil {
		return teamTask{}, false, recentTaskConflict(recent)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	b.counter++
	task := teamTask{
		ID:               fmt.Sprintf("task-%d", b.counter),
		Channel:          item.Channel,
		ExecutionKey:     normalizeExecutionKey(input.ExecutionKey),
		Title:            item.Title,
		Details:          item.Details,
		Owner:            item.Owner,
		Status:           "open",
		CreatedBy:        input.CreatedBy,
		ThreadID:         input.ThreadID,
		TaskType:         item.TaskType,
		PipelineID:       firstNonEmpty(input.PipelineID, item.PipelineID),
		ExecutionMode:    item.ExecutionMode,
		ReviewState:      firstNonEmpty(input.ReviewState, item.ReviewState),
		SourceSignalID:   input.SourceSignalID,
		SourceDecisionID: input.SourceDecisionID,
		WorkspacePath:    item.WorkspacePath,
		DependsOn:        append([]string(nil), item.ResolvedDepIDs...),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if task.ExecutionKey == "" {
		task.ExecutionKey = deriveTaskExecutionKey(&task)
	}
	if len(task.DependsOn) > 0 && b.hasUnresolvedDepsLocked(&task) {
		task.Blocked = true
	} else if task.Owner != "" {
		task.Status = "in_progress"
	}
	b.ensureTaskOwnerChannelMembershipLocked(item.Channel, task.Owner)
	b.queueTaskBehindActiveOwnerLaneLocked(&task)
	if err := rejectTheaterTaskForLiveBusiness(&task); err != nil {
		return teamTask{}, false, err
	}
	b.scheduleTaskLifecycleLocked(&task)
	if err := b.syncTaskWorktreeLocked(&task); err != nil {
		return teamTask{}, false, err
	}
	b.linkTaskWorkspaceToChannelLocked(item.Channel, &task)
	issueQueued, prQueued := b.maybeQueueTaskGitHubPublicationLocked(&task, "", "create", nil)
	b.tasks = append(b.tasks, task)
	b.appendActionWithRefsLocked("task_created", "office", channel, input.CreatedBy, truncateSummary(task.Title, 140), task.ID, compactStringList([]string{task.SourceSignalID}), task.SourceDecisionID)
	if err := b.saveLocked(); err != nil {
		return teamTask{}, false, err
	}
	if issueQueued {
		b.publishTaskIssueSoon(task.ID)
	}
	if prQueued {
		b.publishTaskPRSoon(task.ID)
	}
	return task, false, nil
}

// hasUnresolvedDepsLocked returns true if any of the task's dependencies are not done.
func (b *Broker) hasUnresolvedDepsLocked(task *teamTask) bool {
	for _, depID := range task.DependsOn {
		if requestIsResolvedLocked(b.requests, depID) {
			continue
		}
		found := false
		for j := range b.tasks {
			if b.tasks[j].ID == depID {
				found = true
				if b.tasks[j].Status != "done" {
					return true
				}
				break
			}
		}
		if !found {
			return true // dependency doesn't exist yet — treat as unresolved
		}
	}
	return false
}

// unblockDependentsLocked checks all blocked tasks and unblocks those whose
// dependencies are now resolved. For each newly unblocked task, it appends a
// "task_unblocked" action so the launcher can deliver a notification to the owner.
func (b *Broker) unblockDependentsLocked(completedTaskID string) {
	now := time.Now().UTC().Format(time.RFC3339)
	for i := range b.tasks {
		if !b.tasks[i].Blocked {
			continue
		}
		hasDep := false
		for _, depID := range b.tasks[i].DependsOn {
			if depID == completedTaskID {
				hasDep = true
				break
			}
		}
		if !hasDep {
			continue
		}
		if !b.hasUnresolvedDepsLocked(&b.tasks[i]) {
			b.tasks[i].Blocked = false
			if strings.TrimSpace(b.tasks[i].Owner) != "" {
				b.tasks[i].Status = "in_progress"
			} else {
				b.tasks[i].Status = "open"
			}
			b.queueTaskBehindActiveOwnerLaneLocked(&b.tasks[i])
			b.tasks[i].UpdatedAt = now
			b.scheduleTaskLifecycleLocked(&b.tasks[i])
			_ = b.syncTaskWorktreeLocked(&b.tasks[i])
			b.appendActionLocked(
				"task_unblocked",
				"office",
				normalizeChannelSlug(b.tasks[i].Channel),
				"system",
				truncateSummary(b.tasks[i].Title+" unblocked by "+completedTaskID, 140),
				b.tasks[i].ID,
			)
		}
	}
}

type taskReuseMatch struct {
	Channel          string
	ExecutionKey     string
	Title            string
	Details          string
	ThreadID         string
	Owner            string
	TaskType         string
	PipelineID       string
	ExecutionMode    string
	WorkspacePath    string
	SourceSignalID   string
	SourceDecisionID string
}

const recentTerminalTaskReuseWindow = 24 * time.Hour

type recentTaskConflictError struct {
	TaskID    string
	Status    string
	UpdatedAt string
}

func (e *recentTaskConflictError) Error() string {
	if e == nil {
		return "recent similar task exists"
	}
	taskID := strings.TrimSpace(e.TaskID)
	if taskID == "" {
		taskID = "unknown-task"
	}
	status := strings.TrimSpace(e.Status)
	if status == "" {
		status = "done"
	}
	if updatedAt := strings.TrimSpace(e.UpdatedAt); updatedAt != "" {
		return fmt.Sprintf("recent similar task exists: %s (%s, updated %s)", taskID, status, updatedAt)
	}
	return fmt.Sprintf("recent similar task exists: %s (%s)", taskID, status)
}

func (m taskReuseMatch) hasScopedIdentity() bool {
	return (!m.usesImplicitTitleExecutionKey() && strings.TrimSpace(m.ExecutionKey) != "") ||
		strings.TrimSpace(m.SourceSignalID) != "" ||
		strings.TrimSpace(m.SourceDecisionID) != ""
}

func (m taskReuseMatch) usesImplicitTitleExecutionKey() bool {
	executionKey := normalizeExecutionKey(m.ExecutionKey)
	if executionKey == "" {
		return false
	}
	probe := teamTask{
		Channel: m.Channel,
		Title:   m.Title,
		Owner:   m.Owner,
	}
	return executionKey == deriveImplicitTitleExecutionKey(&probe)
}

func semanticTaskLaneKey(task *teamTask) string {
	if task == nil {
		return ""
	}
	channel := normalizeChannelSlug(task.Channel)
	if channel == "" {
		channel = "general"
	}
	owner := normalizeExecutionKey(task.Owner)
	taskType := normalizeExecutionKey(task.TaskType)
	targetRepo := ""
	if workspacePath := strings.TrimSpace(task.WorkspacePath); workspacePath != "" {
		targetRepo = normalizeExecutionKey(filepath.Base(workspacePath))
	} else if inferred := inferSiblingWorkspacePathForTask(task); inferred != "" {
		targetRepo = normalizeExecutionKey(filepath.Base(inferred))
	}
	titleTokens := extractTaskLaneSignalTokens(task.Title)
	detailTokens := extractTaskLaneSignalTokens(strings.Join([]string{
		task.ExecutionKey,
		task.Details,
	}, "\n"))
	trimRepoTokens := func(tokens []string) []string {
		out := make([]string, 0, len(tokens))
		for _, token := range tokens {
			if targetRepo != "" && token == targetRepo {
				continue
			}
			out = append(out, token)
		}
		return out
	}
	anchors := trimRepoTokens(titleTokens)
	if len(anchors) == 0 {
		anchors = trimRepoTokens(detailTokens)
	}
	if len(anchors) == 0 && targetRepo == "" {
		return ""
	}
	if len(anchors) == 0 && len(titleTokens) == 0 {
		for _, token := range extractTaskLaneSignalTokens(strings.Join([]string{task.Title, task.Details}, "\n")) {
			if targetRepo != "" && token == targetRepo {
				continue
			}
			anchors = append(anchors, token)
		}
	}
	parts := []string{channel}
	if owner != "" {
		parts = append(parts, "owner", owner)
	}
	if taskType != "" {
		parts = append(parts, "type", taskType)
	}
	if len(anchors) > 0 {
		if len(anchors) > 2 {
			anchors = anchors[:2]
		}
		parts = append(parts, "anchor", strings.Join(anchors, ","))
	} else if targetRepo != "" {
		parts = append(parts, "repo", targetRepo)
	}
	return strings.Join(parts, "|")
}

func semanticTaskLaneKeyForMatch(match taskReuseMatch) string {
	probe := teamTask{
		Channel:       match.Channel,
		ExecutionKey:  match.ExecutionKey,
		Title:         match.Title,
		Details:       match.Details,
		Owner:         match.Owner,
		TaskType:      match.TaskType,
		ExecutionMode: match.ExecutionMode,
		WorkspacePath: match.WorkspacePath,
	}
	return semanticTaskLaneKey(&probe)
}

func coalesceTaskView(tasks []teamTask) []teamTask {
	if len(tasks) < 2 {
		return append([]teamTask(nil), tasks...)
	}
	out := make([]teamTask, 0, len(tasks))
	seen := make(map[string]int)
	for _, task := range tasks {
		key := taskCoalesceKey(&task)
		if key == "" {
			out = append(out, task)
			continue
		}
		if idx, ok := seen[key]; ok {
			current := out[idx]
			if chosen := selectCanonicalExecutionTask(&current, &task); chosen != nil && chosen.ID != current.ID {
				out[idx] = task
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, task)
	}
	return out
}

func taskCoalesceKey(task *teamTask) string {
	if task == nil {
		return ""
	}
	if hasScopedTaskIdentity(task) {
		if executionKey := normalizeExecutionKey(task.ExecutionKey); executionKey != "" {
			channel := normalizeChannelSlug(task.Channel)
			if channel == "" {
				channel = "general"
			}
			return "execution|" + channel + "|" + executionKey
		}
	}
	return semanticTaskLaneKey(task)
}

func (b *Broker) findCanonicalTaskBySemanticLaneLocked(channel, laneKey string) *teamTask {
	channel = normalizeChannelSlug(channel)
	laneKey = strings.TrimSpace(laneKey)
	if laneKey == "" {
		return nil
	}
	var canonical *teamTask
	for i := range b.tasks {
		task := &b.tasks[i]
		if normalizeChannelSlug(task.Channel) != channel {
			continue
		}
		if semanticTaskLaneKey(task) != laneKey {
			continue
		}
		canonical = selectCanonicalExecutionTask(canonical, task)
	}
	return canonical
}

func hasScopedTaskIdentity(task *teamTask) bool {
	if task == nil {
		return false
	}
	if strings.TrimSpace(task.SourceSignalID) != "" || strings.TrimSpace(task.SourceDecisionID) != "" {
		return true
	}
	executionKey := normalizeExecutionKey(task.ExecutionKey)
	if executionKey == "" {
		return false
	}
	return executionKey != deriveImplicitTitleExecutionKey(task)
}

func taskOwnerMatches(task *teamTask, owner string) bool {
	if task == nil {
		return false
	}
	taskOwner := strings.TrimSpace(task.Owner)
	return owner == "" || taskOwner == owner || taskOwner == ""
}

func scopedTaskIdentityMatches(task *teamTask, match taskReuseMatch) bool {
	if task == nil {
		return false
	}
	if match.ExecutionKey != "" && normalizeExecutionKey(task.ExecutionKey) != match.ExecutionKey {
		return false
	}
	if match.PipelineID != "" && strings.TrimSpace(task.PipelineID) != "" && strings.TrimSpace(task.PipelineID) != match.PipelineID {
		return false
	}
	if match.SourceSignalID != "" && strings.TrimSpace(task.SourceSignalID) != match.SourceSignalID {
		return false
	}
	if match.SourceDecisionID != "" && strings.TrimSpace(task.SourceDecisionID) != match.SourceDecisionID {
		return false
	}
	return true
}

func recentTaskConflict(task *teamTask) error {
	if task == nil {
		return nil
	}
	return &recentTaskConflictError{
		TaskID:    strings.TrimSpace(task.ID),
		Status:    strings.TrimSpace(task.Status),
		UpdatedAt: firstNonEmpty(strings.TrimSpace(task.UpdatedAt), strings.TrimSpace(task.CreatedAt)),
	}
}

func taskUpdatedWithinWindow(task *teamTask, cutoff time.Time) bool {
	if task == nil {
		return false
	}
	updatedEpoch := taskUpdatedEpoch(task)
	if updatedEpoch == 0 {
		return false
	}
	return updatedEpoch >= cutoff.Unix()
}

func (b *Broker) findReusableTaskByExecutionKeyLocked(channel, executionKey string) *teamTask {
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		channel = "general"
	}
	executionKey = normalizeExecutionKey(executionKey)
	if executionKey == "" {
		return nil
	}
	var canonical *teamTask
	for i := range b.tasks {
		task := &b.tasks[i]
		if normalizeChannelSlug(task.Channel) != channel {
			continue
		}
		if normalizeExecutionKey(task.ExecutionKey) != executionKey {
			continue
		}
		if taskIsTerminal(task) {
			continue
		}
		canonical = selectCanonicalExecutionTask(canonical, task)
	}
	return canonical
}

func (b *Broker) findReusableTaskBySemanticLaneLocked(channel, laneKey string) *teamTask {
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		channel = "general"
	}
	laneKey = strings.TrimSpace(laneKey)
	if laneKey == "" {
		return nil
	}
	var canonical *teamTask
	for i := range b.tasks {
		task := &b.tasks[i]
		if normalizeChannelSlug(task.Channel) != channel {
			continue
		}
		if semanticTaskLaneKey(task) != laneKey {
			continue
		}
		if taskIsTerminal(task) {
			continue
		}
		canonical = selectCanonicalExecutionTask(canonical, task)
	}
	return canonical
}

func (b *Broker) findRecentTerminalTaskByExecutionKeyLocked(channel, executionKey string, cutoff time.Time) *teamTask {
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		channel = "general"
	}
	executionKey = normalizeExecutionKey(executionKey)
	if executionKey == "" {
		return nil
	}
	var canonical *teamTask
	for i := range b.tasks {
		task := &b.tasks[i]
		if normalizeChannelSlug(task.Channel) != channel {
			continue
		}
		if normalizeExecutionKey(task.ExecutionKey) != executionKey {
			continue
		}
		if !taskIsTerminal(task) || !taskUpdatedWithinWindow(task, cutoff) {
			continue
		}
		canonical = selectCanonicalExecutionTask(canonical, task)
	}
	return canonical
}

func (b *Broker) findRecentTerminalTaskBySemanticLaneLocked(channel, laneKey string, cutoff time.Time) *teamTask {
	channel = normalizeChannelSlug(channel)
	if channel == "" {
		channel = "general"
	}
	laneKey = strings.TrimSpace(laneKey)
	if laneKey == "" {
		return nil
	}
	var canonical *teamTask
	for i := range b.tasks {
		task := &b.tasks[i]
		if normalizeChannelSlug(task.Channel) != channel {
			continue
		}
		if semanticTaskLaneKey(task) != laneKey {
			continue
		}
		if !taskIsTerminal(task) || !taskUpdatedWithinWindow(task, cutoff) {
			continue
		}
		canonical = selectCanonicalExecutionTask(canonical, task)
	}
	return canonical
}

func (b *Broker) findReusableTaskLocked(match taskReuseMatch) *teamTask {
	channel := normalizeChannelSlug(match.Channel)
	if channel == "" {
		channel = "general"
	}
	workspacePath := strings.TrimSpace(match.WorkspacePath)
	executionKey := normalizeExecutionKey(match.ExecutionKey)
	if executionKey != "" {
		if reusable := b.findReusableTaskByExecutionKeyLocked(channel, executionKey); reusable != nil {
			if !taskMatchesExplicitWorkspacePath(reusable, workspacePath) {
				return nil
			}
			return reusable
		}
		if !match.usesImplicitTitleExecutionKey() {
			return nil
		}
	}
	if laneKey := semanticTaskLaneKeyForMatch(match); laneKey != "" {
		if canonical := b.findReusableTaskBySemanticLaneLocked(channel, laneKey); canonical != nil {
			if !taskMatchesExplicitWorkspacePath(canonical, workspacePath) {
				return nil
			}
			taskHasScopedIdentity := hasScopedTaskIdentity(canonical)
			if match.hasScopedIdentity() || taskHasScopedIdentity {
				if scopedTaskIdentityMatches(canonical, match) {
					return canonical
				}
			} else {
				return canonical
			}
		}
	}
	title := strings.TrimSpace(match.Title)
	threadID := strings.TrimSpace(match.ThreadID)
	owner := strings.TrimSpace(match.Owner)
	scopedIdentity := match.hasScopedIdentity()
	for i := range b.tasks {
		task := &b.tasks[i]
		if normalizeChannelSlug(task.Channel) != channel {
			continue
		}
		if !taskMatchesExplicitWorkspacePath(task, workspacePath) {
			continue
		}
		if taskIsTerminal(task) {
			continue
		}
		sameTitle := title != "" && strings.EqualFold(strings.TrimSpace(task.Title), title)
		if threadID != "" && strings.TrimSpace(task.ThreadID) == threadID {
			if sameTitle && taskOwnerMatches(task, owner) {
				taskHasScopedIdentity := hasScopedTaskIdentity(task)
				if scopedIdentity || taskHasScopedIdentity {
					if !scopedIdentity || !taskHasScopedIdentity {
						continue
					}
					if scopedTaskIdentityMatches(task, match) {
						return task
					}
					continue
				}
				return task
			}
			continue
		}
		if !sameTitle || !taskOwnerMatches(task, owner) {
			continue
		}
		taskHasScopedIdentity := hasScopedTaskIdentity(task)
		if scopedIdentity || taskHasScopedIdentity {
			if !scopedIdentity || !taskHasScopedIdentity {
				continue
			}
			if scopedTaskIdentityMatches(task, match) {
				return task
			}
			continue
		}
		return task
	}
	return nil
}

func (b *Broker) findRecentTerminalTaskLocked(match taskReuseMatch) *teamTask {
	channel := normalizeChannelSlug(match.Channel)
	if channel == "" {
		channel = "general"
	}
	workspacePath := strings.TrimSpace(match.WorkspacePath)
	cutoff := time.Now().UTC().Add(-recentTerminalTaskReuseWindow)
	executionKey := normalizeExecutionKey(match.ExecutionKey)
	if executionKey != "" {
		if recent := b.findRecentTerminalTaskByExecutionKeyLocked(channel, executionKey, cutoff); recent != nil {
			if !taskMatchesExplicitWorkspacePath(recent, workspacePath) {
				return nil
			}
			return recent
		}
		if !match.usesImplicitTitleExecutionKey() {
			return nil
		}
	}
	if laneKey := semanticTaskLaneKeyForMatch(match); laneKey != "" {
		if canonical := b.findRecentTerminalTaskBySemanticLaneLocked(channel, laneKey, cutoff); canonical != nil {
			if !taskMatchesExplicitWorkspacePath(canonical, workspacePath) {
				return nil
			}
			taskHasScopedIdentity := hasScopedTaskIdentity(canonical)
			if match.hasScopedIdentity() || taskHasScopedIdentity {
				if scopedTaskIdentityMatches(canonical, match) {
					return canonical
				}
			} else {
				return canonical
			}
		}
	}
	title := strings.TrimSpace(match.Title)
	threadID := strings.TrimSpace(match.ThreadID)
	owner := strings.TrimSpace(match.Owner)
	scopedIdentity := match.hasScopedIdentity()
	var candidate *teamTask
	for i := range b.tasks {
		task := &b.tasks[i]
		if normalizeChannelSlug(task.Channel) != channel {
			continue
		}
		if !taskMatchesExplicitWorkspacePath(task, workspacePath) {
			continue
		}
		if !taskIsTerminal(task) || !taskUpdatedWithinWindow(task, cutoff) {
			continue
		}
		sameTitle := title != "" && strings.EqualFold(strings.TrimSpace(task.Title), title)
		if threadID != "" && strings.TrimSpace(task.ThreadID) == threadID {
			if sameTitle && taskOwnerMatches(task, owner) {
				taskHasScopedIdentity := hasScopedTaskIdentity(task)
				if scopedIdentity || taskHasScopedIdentity {
					if !scopedIdentity || !taskHasScopedIdentity {
						continue
					}
					if scopedTaskIdentityMatches(task, match) {
						candidate = selectCanonicalExecutionTask(candidate, task)
					}
					continue
				}
				candidate = selectCanonicalExecutionTask(candidate, task)
			}
			continue
		}
		if !sameTitle || !taskOwnerMatches(task, owner) {
			continue
		}
		taskHasScopedIdentity := hasScopedTaskIdentity(task)
		if scopedIdentity || taskHasScopedIdentity {
			if !scopedIdentity || !taskHasScopedIdentity {
				continue
			}
			if scopedTaskIdentityMatches(task, match) {
				candidate = selectCanonicalExecutionTask(candidate, task)
			}
			continue
		}
		candidate = selectCanonicalExecutionTask(candidate, task)
	}
	return candidate
}

func normalizeExecutionKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	return strings.Join(strings.Fields(value), " ")
}

func deriveImplicitTitleExecutionKey(task *teamTask) string {
	if task == nil {
		return ""
	}
	channel := normalizeChannelSlug(task.Channel)
	if channel == "" {
		channel = "general"
	}
	title := normalizeExecutionKey(task.Title)
	if title == "" {
		return ""
	}
	parts := []string{channel}
	if owner := normalizeExecutionKey(task.Owner); owner != "" {
		parts = append(parts, "owner", owner)
	}
	parts = append(parts, "title", title)
	return strings.Join(parts, "|")
}

func deriveTaskExecutionKey(task *teamTask) string {
	if task == nil {
		return ""
	}
	if v := normalizeExecutionKey(task.ExecutionKey); v != "" {
		return v
	}
	channel := normalizeChannelSlug(task.Channel)
	if channel == "" {
		channel = "general"
	}
	if sourceDecisionID := normalizeExecutionKey(task.SourceDecisionID); sourceDecisionID != "" {
		return channel + "|decision|" + sourceDecisionID
	}
	if sourceSignalID := normalizeExecutionKey(task.SourceSignalID); sourceSignalID != "" {
		return channel + "|signal|" + sourceSignalID
	}
	return deriveImplicitTitleExecutionKey(task)
}

func taskStateRank(task *teamTask) int {
	if task == nil {
		return -1
	}
	status := strings.ToLower(strings.TrimSpace(task.Status))
	score := 0
	switch status {
	case "done":
		score = 500
	case "review", "in_review":
		score = 400
	case "in_progress":
		score = 300
	case "open":
		score = 200
	case "blocked":
		score = 100
	case "canceled", "cancelled", "failed":
		score = 50
	default:
		score = 10
	}
	if strings.TrimSpace(task.ReviewState) != "" {
		score += 25
	}
	if task.Blocked {
		score -= 10
	}
	return score
}

func taskIsTerminal(task *teamTask) bool {
	if task == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(task.Status)) {
	case "done", "canceled", "cancelled", "failed":
		return true
	default:
		return false
	}
}

func taskUpdatedEpoch(task *teamTask) int64 {
	if task == nil {
		return 0
	}
	for _, value := range []string{task.UpdatedAt, task.CreatedAt} {
		if ts, err := time.Parse(time.RFC3339, strings.TrimSpace(value)); err == nil {
			return ts.Unix()
		}
	}
	return 0
}

func selectCanonicalExecutionTask(left, right *teamTask) *teamTask {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	if leftRank, rightRank := taskStateRank(left), taskStateRank(right); leftRank != rightRank {
		if rightRank > leftRank {
			return right
		}
		return left
	}
	if rightUpdated, leftUpdated := taskUpdatedEpoch(right), taskUpdatedEpoch(left); rightUpdated != leftUpdated {
		if rightUpdated > leftUpdated {
			return right
		}
		return left
	}
	if right.ID < left.ID {
		return right
	}
	return left
}

func (b *Broker) findCanonicalTaskByExecutionKeyLocked(channel, executionKey string) *teamTask {
	channel = normalizeChannelSlug(channel)
	executionKey = normalizeExecutionKey(executionKey)
	if executionKey == "" {
		return nil
	}
	var canonical *teamTask
	for i := range b.tasks {
		task := &b.tasks[i]
		if normalizeChannelSlug(task.Channel) != channel {
			continue
		}
		if normalizeExecutionKey(task.ExecutionKey) != executionKey {
			continue
		}
		canonical = selectCanonicalExecutionTask(canonical, task)
	}
	return canonical
}

func (b *Broker) handleRequests(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		b.handleGetRequests(w, r)
	case http.MethodPost:
		b.handlePostRequest(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *Broker) handleGetRequests(w http.ResponseWriter, r *http.Request) {
	channel := normalizeChannelSlug(r.URL.Query().Get("channel"))
	if channel == "" {
		channel = "general"
	}
	viewerSlug := strings.TrimSpace(r.URL.Query().Get("viewer_slug"))
	includeResolved := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_resolved")), "true")
	b.mu.Lock()
	if !b.canAccessChannelLocked(viewerSlug, channel) {
		b.mu.Unlock()
		http.Error(w, "channel access denied", http.StatusForbidden)
		return
	}
	requests := make([]humanInterview, 0, len(b.requests))
	for _, req := range b.requests {
		reqChannel := normalizeChannelSlug(req.Channel)
		if reqChannel == "" {
			reqChannel = "general"
		}
		if reqChannel != channel {
			continue
		}
		if !includeResolved && !requestIsActive(req) {
			continue
		}
		requests = append(requests, req)
	}
	pending := firstBlockingRequest(requests)
	b.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"channel":  channel,
		"requests": requests,
		"pending":  pending,
	})
}

func (b *Broker) handlePostRequest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action        string            `json:"action"`
		ID            string            `json:"id"`
		Actor         string            `json:"actor"`
		Kind          string            `json:"kind"`
		From          string            `json:"from"`
		Channel       string            `json:"channel"`
		Title         string            `json:"title"`
		Question      string            `json:"question"`
		Context       string            `json:"context"`
		Options       []interviewOption `json:"options"`
		RecommendedID string            `json:"recommended_id"`
		Blocking      bool              `json:"blocking"`
		Required      bool              `json:"required"`
		Secret        bool              `json:"secret"`
		ReplyTo       string            `json:"reply_to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	action := strings.TrimSpace(body.Action)
	if action == "" {
		action = "create"
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	switch action {
	case "create":
		if strings.TrimSpace(body.From) == "" || strings.TrimSpace(body.Question) == "" {
			http.Error(w, "from and question required", http.StatusBadRequest)
			return
		}
		channel := normalizeChannelSlug(body.Channel)
		if channel == "" {
			channel = "general"
		}
		if b.findChannelLocked(channel) == nil {
			http.Error(w, "channel not found", http.StatusNotFound)
			return
		}
		if !b.canAccessChannelLocked(body.From, channel) {
			http.Error(w, "channel access denied", http.StatusForbidden)
			return
		}
		b.counter++
		req := humanInterview{
			ID:            fmt.Sprintf("request-%d", b.counter),
			Kind:          normalizeRequestKind(body.Kind),
			Status:        "pending",
			From:          strings.TrimSpace(body.From),
			Channel:       channel,
			Title:         strings.TrimSpace(body.Title),
			Question:      strings.TrimSpace(body.Question),
			Context:       strings.TrimSpace(body.Context),
			Options:       body.Options,
			RecommendedID: "",
			Blocking:      body.Blocking,
			Required:      body.Required,
			Secret:        body.Secret,
			ReplyTo:       strings.TrimSpace(body.ReplyTo),
			CreatedAt:     time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		}
		req.RecommendedID = strings.TrimSpace(body.RecommendedID)
		if req.Title == "" {
			req.Title = "Request"
		}
		req = normalizeRequestRecord(req)
		normalized, err := resolveRequestTransition(req, "create", nil, time.Now().UTC())
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		req = normalized
		b.scheduleRequestLifecycleLocked(&req)
		b.requests = append(b.requests, req)
		b.pendingInterview = firstBlockingRequest(b.requests)
		b.appendActionLocked("request_created", "office", channel, req.From, truncateSummary(req.Title+" "+req.Question, 140), req.ID)
		if err := b.saveLocked(); err != nil {
			http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"request": req, "id": req.ID})
	case "recommend":
		id := strings.TrimSpace(body.ID)
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		actor := strings.TrimSpace(body.Actor)
		if actor == "" {
			actor = "human"
		}
		req, task, prompt, err := b.requestGameMasterRecommendationLocked(id, actor)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if err := b.saveLocked(); err != nil {
			http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"request":        req,
			"task":           task,
			"prompt_message": prompt,
		})
	case "cancel":
		id := strings.TrimSpace(body.ID)
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		for i := range b.requests {
			if b.requests[i].ID != id {
				continue
			}
			normalized, err := resolveRequestTransition(b.requests[i], "cancel", nil, time.Now().UTC())
			if err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			b.requests[i] = normalized
			b.scheduleRequestLifecycleLocked(&b.requests[i])
			b.pendingInterview = firstBlockingRequest(b.requests)
			b.appendActionLocked("request_canceled", "office", b.requests[i].Channel, b.requests[i].From, truncateSummary(b.requests[i].Title+" "+b.requests[i].Question, 140), b.requests[i].ID)
			if err := b.saveLocked(); err != nil {
				http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"request": b.requests[i]})
			return
		}
		http.Error(w, "request not found", http.StatusNotFound)
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
	}
}

func (b *Broker) handleRequestAnswer(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		b.handleGetRequestAnswer(w, r)
	case http.MethodPost:
		b.handlePostRequestAnswer(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *Broker) handleGetRequestAnswer(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	b.mu.Lock()
	defer b.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	for _, req := range b.requests {
		if req.ID == id && req.Answered != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"answered": req.Answered})
			return
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"answered": nil})
}

func (b *Broker) handlePostRequestAnswer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID         string `json:"id"`
		ChoiceID   string `json:"choice_id"`
		ChoiceText string `json:"choice_text"`
		CustomText string `json:"custom_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	b.mu.Lock()
	for i := range b.requests {
		if b.requests[i].ID != body.ID {
			continue
		}
		choiceID := strings.TrimSpace(body.ChoiceID)
		choiceText := strings.TrimSpace(body.ChoiceText)
		customText := strings.TrimSpace(body.CustomText)
		option := findRequestOption(b.requests[i], choiceID)
		if choiceID != "" && option == nil {
			b.mu.Unlock()
			http.Error(w, "unknown request option", http.StatusBadRequest)
			return
		}
		if option != nil {
			if choiceText == "" {
				choiceText = strings.TrimSpace(option.Label)
			}
			if option.RequiresText && customText == "" {
				hint := strings.TrimSpace(option.TextHint)
				if hint == "" {
					hint = "custom_text required for this response"
				}
				b.mu.Unlock()
				http.Error(w, hint, http.StatusBadRequest)
				return
			}
		}
		if choiceID == "" && choiceText == "" && customText == "" {
			b.mu.Unlock()
			http.Error(w, "choice_text or custom_text required", http.StatusBadRequest)
			return
		}
		answer := &interviewAnswer{
			ChoiceID:   choiceID,
			ChoiceText: choiceText,
			CustomText: customText,
			AnsweredAt: time.Now().UTC().Format(time.RFC3339),
		}
		normalized, err := resolveRequestTransition(b.requests[i], "answer", answer, time.Now().UTC())
		if err != nil {
			b.mu.Unlock()
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		b.requests[i] = normalized
		b.scheduleRequestLifecycleLocked(&b.requests[i])
		b.unblockDependentsLocked(b.requests[i].ID)
		b.pendingInterview = firstBlockingRequest(b.requests)
		b.unblockTasksForAnsweredRequestLocked(b.requests[i])

		// Skill proposal callback: accept activates the skill, reject archives it.
		if b.requests[i].Kind == "skill_proposal" {
			replyTo := strings.TrimSpace(b.requests[i].ReplyTo)
			for j := range b.skills {
				if b.skills[j].Name == replyTo && b.skills[j].Status != "archived" {
					activatedAt := time.Now().UTC().Format(time.RFC3339)
					if choiceID == "accept" {
						b.skills[j].Status = "active"
						b.skills[j].UpdatedAt = activatedAt
						b.counter++
						b.appendMessageLocked(channelMessage{
							ID:        fmt.Sprintf("msg-%d", b.counter),
							From:      "system",
							Channel:   normalizeChannelSlug(b.requests[i].Channel),
							Kind:      "skill_activated",
							Title:     "Skill Activated: " + b.skills[j].Title,
							Content:   fmt.Sprintf("Skill **%s** is now active and ready to use.", b.skills[j].Title),
							Timestamp: activatedAt,
						})
					} else {
						b.skills[j].Status = "archived"
						b.skills[j].UpdatedAt = activatedAt
					}
					break
				}
			}
		}

		b.counter++
		msg := channelMessage{
			ID:        fmt.Sprintf("msg-%d", b.counter),
			From:      "you",
			Channel:   normalizeChannelSlug(b.requests[i].Channel),
			Tagged:    []string{b.requests[i].From},
			ReplyTo:   strings.TrimSpace(b.requests[i].ReplyTo),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}
		msg.Content = formatRequestAnswerMessage(b.requests[i], *answer)
		b.appendMessageLocked(msg)
		b.appendActionLocked("request_answered", "office", b.requests[i].Channel, "you", truncateSummary(msg.Content, 140), b.requests[i].ID)
		if err := b.saveLocked(); err != nil {
			b.mu.Unlock()
			http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
			return
		}
		b.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		return
	}
	b.mu.Unlock()
	http.Error(w, "request not found", http.StatusNotFound)
}

func (b *Broker) unblockTasksForAnsweredRequestLocked(req humanInterview) {
	reqID := strings.TrimSpace(req.ID)
	if reqID == "" {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	answerText := strings.TrimSpace(reqAnswerSummary(req.Answered))
	for i := range b.tasks {
		task := &b.tasks[i]
		if !task.Blocked || strings.EqualFold(strings.TrimSpace(task.Status), "done") {
			continue
		}
		if sourceTaskID := strings.TrimSpace(req.SourceTaskID); sourceTaskID != "" {
			if strings.TrimSpace(task.ID) != sourceTaskID {
				continue
			}
			pendingOtherBlockers := false
			if len(task.BlockerRequestIDs) > 0 {
				remaining := make([]string, 0, len(task.BlockerRequestIDs))
				for _, blockerReqID := range task.BlockerRequestIDs {
					blockerReqID = strings.TrimSpace(blockerReqID)
					if blockerReqID == "" || blockerReqID == reqID {
						continue
					}
					remaining = append(remaining, blockerReqID)
					if !requestIsResolvedLocked(b.requests, blockerReqID) {
						pendingOtherBlockers = true
					}
				}
				task.BlockerRequestIDs = remaining
			}
			if pendingOtherBlockers {
				continue
			}
		} else {
			haystack := strings.ToLower(strings.TrimSpace(task.Title + "\n" + task.Details))
			if !strings.Contains(haystack, strings.ToLower(reqID)) {
				continue
			}
		}
		task.Blocked = false
		if strings.EqualFold(strings.TrimSpace(task.Status), "blocked") {
			if strings.TrimSpace(task.Owner) != "" {
				task.Status = "in_progress"
			} else {
				task.Status = "open"
			}
		}
		b.queueTaskBehindActiveOwnerLaneLocked(task)
		if answerText != "" && !strings.Contains(task.Details, answerText) {
			task.Details = strings.TrimSpace(task.Details)
			if task.Details != "" {
				task.Details += "\n\n"
			}
			task.Details += fmt.Sprintf("Human answer for %s: %s", reqID, answerText)
		}
		task.UpdatedAt = now
		b.appendActionLocked(
			"task_unblocked",
			"office",
			task.Channel,
			req.From,
			truncateSummary(task.Title+" unblocked by answered "+reqID, 140),
			task.ID,
		)
	}
}

func reqAnswerSummary(answer *interviewAnswer) string {
	if answer == nil {
		return ""
	}
	if text := strings.TrimSpace(answer.CustomText); text != "" {
		return text
	}
	if text := strings.TrimSpace(answer.ChoiceText); text != "" {
		return text
	}
	return strings.TrimSpace(answer.ChoiceID)
}

func (b *Broker) handleInterview(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		b.handleGetInterview(w, r)
	case http.MethodPost:
		b.handlePostInterview(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *Broker) handlePostInterview(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From          string            `json:"from"`
		Channel       string            `json:"channel"`
		Question      string            `json:"question"`
		Context       string            `json:"context"`
		Options       []interviewOption `json:"options"`
		RecommendedID string            `json:"recommended_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.From) == "" || strings.TrimSpace(body.Question) == "" {
		http.Error(w, "from and question required", http.StatusBadRequest)
		return
	}
	reqBody, _ := json.Marshal(map[string]any{
		"action":         "create",
		"kind":           "interview",
		"title":          "Human interview",
		"from":           body.From,
		"channel":        body.Channel,
		"question":       body.Question,
		"context":        body.Context,
		"options":        body.Options,
		"recommended_id": body.RecommendedID,
		"blocking":       true,
		"required":       true,
	})
	r2 := r.Clone(r.Context())
	r2.Body = io.NopCloser(bytes.NewReader(reqBody))
	b.handlePostRequest(w, r2)
}

func (b *Broker) handleGetInterview(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	pending := firstBlockingRequest(b.requests)
	if pending == nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"pending": nil})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"pending": pending})
}

func (b *Broker) handleInterviewAnswer(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		b.handleGetInterviewAnswer(w, r)
	case http.MethodPost:
		b.handlePostInterviewAnswer(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *Broker) handleGetInterviewAnswer(w http.ResponseWriter, r *http.Request) {
	b.handleGetRequestAnswer(w, r)
}

func (b *Broker) handlePostInterviewAnswer(w http.ResponseWriter, r *http.Request) {
	b.handlePostRequestAnswer(w, r)
}

// FormatChannelView returns a clean, Slack-style rendering of recent messages.
func FormatChannelView(messages []channelMessage) string {
	if len(messages) == 0 {
		return "  No messages yet. The team is getting set up..."
	}

	var sb strings.Builder
	for _, m := range messages {
		ts := m.Timestamp
		if len(ts) > 19 {
			ts = ts[11:19]
		}

		prefix := m.From
		if m.Kind == "automation" || m.From == "nex" {
			source := m.Source
			if source == "" {
				source = "context_graph"
			}
			title := m.Title
			if title != "" {
				title += ": "
			}
			sb.WriteString(fmt.Sprintf("  %s  Nex/%s: %s%s\n", ts, source, title, m.Content))
			continue
		}
		if strings.HasPrefix(m.Content, "[STATUS]") {
			sb.WriteString(fmt.Sprintf("  %s  @%s %s%s\n", ts, prefix, m.Content, formatMessageUsageSuffix(m.Usage)))
		} else {
			thread := ""
			if m.ReplyTo != "" {
				thread = fmt.Sprintf(" ↳ %s", m.ReplyTo)
			}
			sb.WriteString(fmt.Sprintf("  %s%s  @%s: %s%s\n", ts, thread, prefix, m.Content, formatMessageUsageSuffix(m.Usage)))
		}
	}
	return sb.String()
}

func formatMessageUsageSuffix(usage *messageUsage) string {
	if usage == nil {
		return ""
	}
	total := usage.TotalTokens
	if total == 0 {
		total = usage.InputTokens + usage.OutputTokens + usage.CacheReadTokens + usage.CacheCreationTokens
	}
	if total == 0 {
		return ""
	}
	return fmt.Sprintf(" [%d tok]", total)
}

// --------------- Skills ---------------

func (b *Broker) handleTelegramGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.mu.Lock()
	groups := make([]map[string]any, 0)
	for chatID, title := range b.seenTelegramGroups {
		groups = append(groups, map[string]any{"chat_id": chatID, "title": title})
	}
	b.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"groups": groups})
}

func (b *Broker) handleSkills(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		b.handleGetSkills(w, r)
	case http.MethodPost:
		b.handlePostSkill(w, r)
	case http.MethodPut:
		b.handlePutSkill(w, r)
	case http.MethodDelete:
		b.handleDeleteSkill(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *Broker) handleSkillsSubpath(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/skills/")
	if strings.HasSuffix(path, "/invoke") {
		b.handleInvokeSkill(w, r)
		return
	}
	http.Error(w, "not found", http.StatusNotFound)
}

func skillSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	return s
}

func (b *Broker) findSkillByNameLocked(name string) *teamSkill {
	slug := skillSlug(name)
	for i := range b.skills {
		if skillSlug(b.skills[i].Name) == slug && b.skills[i].Status != "archived" {
			return &b.skills[i]
		}
	}
	return nil
}

func (b *Broker) findSkillByWorkflowKeyLocked(key string) *teamSkill {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	for i := range b.skills {
		if strings.TrimSpace(b.skills[i].WorkflowKey) == key && b.skills[i].Status != "archived" {
			return &b.skills[i]
		}
	}
	return nil
}

func (b *Broker) handleGetSkills(w http.ResponseWriter, r *http.Request) {
	channelFilter := normalizeChannelSlug(r.URL.Query().Get("channel"))

	b.mu.Lock()
	result := make([]teamSkill, 0, len(b.skills))
	for _, sk := range b.skills {
		if sk.Status == "archived" {
			continue
		}
		if !skillVisibleInChannel(sk.Channel, channelFilter) {
			continue
		}
		result = append(result, sk)
	}
	b.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"skills": result})
}

func (b *Broker) handlePostSkill(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action              string   `json:"action"`
		Name                string   `json:"name"`
		Title               string   `json:"title"`
		Description         string   `json:"description"`
		Content             string   `json:"content"`
		CreatedBy           string   `json:"created_by"`
		Channel             string   `json:"channel"`
		Tags                []string `json:"tags"`
		Trigger             string   `json:"trigger"`
		WorkflowProvider    string   `json:"workflow_provider"`
		WorkflowKey         string   `json:"workflow_key"`
		WorkflowDefinition  string   `json:"workflow_definition"`
		WorkflowSchedule    string   `json:"workflow_schedule"`
		RelayID             string   `json:"relay_id"`
		RelayPlatform       string   `json:"relay_platform"`
		RelayEventTypes     []string `json:"relay_event_types"`
		LastExecutionAt     string   `json:"last_execution_at"`
		LastExecutionStatus string   `json:"last_execution_status"`
		RequestID           string   `json:"request_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	action := strings.TrimSpace(body.Action)
	if action == "" {
		action = "create"
	}
	if action != "create" && action != "propose" {
		http.Error(w, "action must be create or propose", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.Name) == "" || strings.TrimSpace(body.Content) == "" || strings.TrimSpace(body.CreatedBy) == "" {
		http.Error(w, "name, content, and created_by required", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	channel := normalizeChannelSlug(body.Channel)
	if channel == "" {
		channel = "general"
	}

	status := "active"
	msgKind := "skill_update"
	if action == "propose" {
		status = "proposed"
		msgKind = "skill_proposal"
	}

	b.mu.Lock()
	requestID := strings.TrimSpace(body.RequestID)
	if payload, ok := b.findMutationAckLocked("skill:create", requestID); ok {
		b.mu.Unlock()
		payload["duplicate"] = true
		b.respondPersistedMutation(w, payload)
		return
	}

	if existing := b.findSkillByNameLocked(body.Name); existing != nil {
		b.mu.Unlock()
		http.Error(w, "skill with this name already exists", http.StatusConflict)
		return
	}

	title := strings.TrimSpace(body.Title)
	if title == "" {
		title = strings.TrimSpace(body.Name)
	}

	b.counter++
	sk := teamSkill{
		ID:                  fmt.Sprintf("skill-%s", skillSlug(body.Name)),
		Name:                strings.TrimSpace(body.Name),
		Title:               title,
		Description:         strings.TrimSpace(body.Description),
		Content:             strings.TrimSpace(body.Content),
		CreatedBy:           strings.TrimSpace(body.CreatedBy),
		Channel:             channel,
		Tags:                body.Tags,
		Trigger:             strings.TrimSpace(body.Trigger),
		WorkflowProvider:    strings.TrimSpace(body.WorkflowProvider),
		WorkflowKey:         strings.TrimSpace(body.WorkflowKey),
		WorkflowDefinition:  strings.TrimSpace(body.WorkflowDefinition),
		WorkflowSchedule:    strings.TrimSpace(body.WorkflowSchedule),
		RelayID:             strings.TrimSpace(body.RelayID),
		RelayPlatform:       strings.TrimSpace(body.RelayPlatform),
		RelayEventTypes:     append([]string(nil), body.RelayEventTypes...),
		LastExecutionAt:     strings.TrimSpace(body.LastExecutionAt),
		LastExecutionStatus: strings.TrimSpace(body.LastExecutionStatus),
		UsageCount:          0,
		Status:              status,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	b.skills = append(b.skills, sk)

	eventChannel := skillMutationChannel(channel)
	b.appendMessageLocked(channelMessage{
		ID:        fmt.Sprintf("msg-%d", b.counter),
		From:      sk.CreatedBy,
		Channel:   eventChannel,
		Kind:      msgKind,
		Title:     sk.Title,
		Content:   fmt.Sprintf("Skill %q %sd by @%s", sk.Name, action, sk.CreatedBy),
		Timestamp: now,
	})
	b.appendActionLocked(msgKind, "office", eventChannel, sk.CreatedBy, truncateSummary(sk.Title, 140), sk.ID)

	payload := map[string]any{
		"persisted": true,
		"duplicate": false,
		"skill":     sk,
	}
	if err := b.rememberMutationAckLocked("skill:create", requestID, payload); err != nil {
		b.mu.Unlock()
		http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
		return
	}
	if err := b.saveLocked(); err != nil {
		b.mu.Unlock()
		http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
		return
	}
	b.mu.Unlock()

	b.respondPersistedMutation(w, payload)
}

func (b *Broker) handlePutSkill(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name                string   `json:"name"`
		Title               string   `json:"title"`
		Description         string   `json:"description"`
		Content             string   `json:"content"`
		Channel             string   `json:"channel"`
		Tags                []string `json:"tags"`
		Trigger             string   `json:"trigger"`
		Status              string   `json:"status"`
		WorkflowProvider    string   `json:"workflow_provider"`
		WorkflowKey         string   `json:"workflow_key"`
		WorkflowDefinition  string   `json:"workflow_definition"`
		WorkflowSchedule    string   `json:"workflow_schedule"`
		RelayID             string   `json:"relay_id"`
		RelayPlatform       string   `json:"relay_platform"`
		RelayEventTypes     []string `json:"relay_event_types"`
		LastExecutionAt     string   `json:"last_execution_at"`
		LastExecutionStatus string   `json:"last_execution_status"`
		RequestID           string   `json:"request_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.Name) == "" && strings.TrimSpace(body.WorkflowKey) == "" {
		http.Error(w, "name or workflow_key required", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	b.mu.Lock()
	requestID := strings.TrimSpace(body.RequestID)
	if payload, ok := b.findMutationAckLocked("skill:update", requestID); ok {
		b.mu.Unlock()
		payload["duplicate"] = true
		b.respondPersistedMutation(w, payload)
		return
	}

	sk := b.findSkillByNameLocked(body.Name)
	if sk == nil {
		sk = b.findSkillByWorkflowKeyLocked(body.WorkflowKey)
	}
	if sk == nil {
		b.mu.Unlock()
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}

	if t := strings.TrimSpace(body.Title); t != "" {
		sk.Title = t
	}
	if d := strings.TrimSpace(body.Description); d != "" {
		sk.Description = d
	}
	if c := strings.TrimSpace(body.Content); c != "" {
		sk.Content = c
	}
	if ch := normalizeChannelSlug(body.Channel); ch != "" {
		sk.Channel = ch
	}
	if body.Tags != nil {
		sk.Tags = body.Tags
	}
	if t := strings.TrimSpace(body.Trigger); t != "" {
		sk.Trigger = t
	}
	if p := strings.TrimSpace(body.WorkflowProvider); p != "" {
		sk.WorkflowProvider = p
	}
	if key := strings.TrimSpace(body.WorkflowKey); key != "" {
		sk.WorkflowKey = key
	}
	if def := strings.TrimSpace(body.WorkflowDefinition); def != "" {
		sk.WorkflowDefinition = def
	}
	if sched := strings.TrimSpace(body.WorkflowSchedule); sched != "" {
		sk.WorkflowSchedule = sched
	}
	if relayID := strings.TrimSpace(body.RelayID); relayID != "" {
		sk.RelayID = relayID
	}
	if relayPlatform := strings.TrimSpace(body.RelayPlatform); relayPlatform != "" {
		sk.RelayPlatform = relayPlatform
	}
	if body.RelayEventTypes != nil {
		sk.RelayEventTypes = append([]string(nil), body.RelayEventTypes...)
	}
	if ts := strings.TrimSpace(body.LastExecutionAt); ts != "" {
		sk.LastExecutionAt = ts
	}
	if status := strings.TrimSpace(body.LastExecutionStatus); status != "" {
		sk.LastExecutionStatus = status
	}
	if s := strings.TrimSpace(body.Status); s != "" {
		sk.Status = s
	}
	sk.UpdatedAt = now

	channel := skillMutationChannel(sk.Channel)

	b.counter++
	b.appendMessageLocked(channelMessage{
		ID:        fmt.Sprintf("msg-%d", b.counter),
		From:      sk.CreatedBy,
		Channel:   channel,
		Kind:      "skill_update",
		Title:     sk.Title,
		Content:   fmt.Sprintf("Skill %q updated", sk.Name),
		Timestamp: now,
	})
	b.appendActionLocked("skill_update", "office", channel, sk.CreatedBy, truncateSummary(sk.Title+" [updated]", 140), sk.ID)

	payload := map[string]any{
		"persisted": true,
		"duplicate": false,
		"skill":     *sk,
	}
	if err := b.rememberMutationAckLocked("skill:update", requestID, payload); err != nil {
		b.mu.Unlock()
		http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
		return
	}
	if err := b.saveLocked(); err != nil {
		b.mu.Unlock()
		http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
		return
	}
	b.mu.Unlock()

	b.respondPersistedMutation(w, payload)
}

func (b *Broker) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name      string `json:"name"`
		RequestID string `json:"request_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	b.mu.Lock()
	requestID := strings.TrimSpace(body.RequestID)
	if payload, ok := b.findMutationAckLocked("skill:delete", requestID); ok {
		b.mu.Unlock()
		payload["duplicate"] = true
		b.respondPersistedMutation(w, payload)
		return
	}

	sk := b.findSkillByNameLocked(body.Name)
	if sk == nil {
		b.mu.Unlock()
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}
	duplicate := sk.Status == "archived"

	sk.Status = "archived"
	sk.UpdatedAt = now

	channel := skillMutationChannel(sk.Channel)

	b.counter++
	b.appendMessageLocked(channelMessage{
		ID:        fmt.Sprintf("msg-%d", b.counter),
		From:      sk.CreatedBy,
		Channel:   channel,
		Kind:      "skill_update",
		Title:     sk.Title,
		Content:   fmt.Sprintf("Skill %q archived", sk.Name),
		Timestamp: now,
	})
	if !duplicate {
		b.appendActionLocked("skill_update", "office", channel, sk.CreatedBy, truncateSummary(sk.Title+" [archived]", 140), sk.ID)
	}

	payload := map[string]any{
		"ok":        true,
		"persisted": true,
		"duplicate": duplicate,
		"skill":     *sk,
	}
	if err := b.rememberMutationAckLocked("skill:delete", requestID, payload); err != nil {
		b.mu.Unlock()
		http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
		return
	}
	if err := b.saveLocked(); err != nil {
		b.mu.Unlock()
		http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
		return
	}
	b.mu.Unlock()

	b.respondPersistedMutation(w, payload)
}

func (b *Broker) handleInvokeSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract skill name from path: /skills/{name}/invoke
	path := strings.TrimPrefix(r.URL.Path, "/skills/")
	skillName := strings.TrimSuffix(path, "/invoke")
	if strings.TrimSpace(skillName) == "" {
		http.Error(w, "skill name required in path", http.StatusBadRequest)
		return
	}

	var body struct {
		InvokedBy string `json:"invoked_by"`
		Channel   string `json:"channel"`
		RequestID string `json:"request_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	b.mu.Lock()
	requestID := strings.TrimSpace(body.RequestID)
	if payload, ok := b.findMutationAckLocked("skill:invoke", requestID); ok {
		b.mu.Unlock()
		payload["duplicate"] = true
		b.respondPersistedMutation(w, payload)
		return
	}

	sk := b.findSkillByNameLocked(skillName)
	if sk == nil {
		b.mu.Unlock()
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}

	sk.UsageCount++
	sk.UpdatedAt = now

	channel := normalizeChannelSlug(body.Channel)
	if channel == "" {
		channel = normalizeChannelSlug(sk.Channel)
	}
	if channel == "" {
		channel = "general"
	}

	invoker := strings.TrimSpace(body.InvokedBy)
	if invoker == "" {
		invoker = "you"
	}
	sk.LastExecutionAt = now
	sk.LastExecutionStatus = "invoked"
	sk.UpdatedAt = now

	b.counter++
	b.appendMessageLocked(channelMessage{
		ID:        fmt.Sprintf("msg-%d", b.counter),
		From:      invoker,
		Channel:   channel,
		Kind:      "skill_invocation",
		Title:     sk.Title,
		Content:   fmt.Sprintf("Skill %q invoked by @%s (usage #%d)", sk.Name, invoker, sk.UsageCount),
		Timestamp: now,
	})
	b.appendActionLocked("skill_invocation", "office", channel, invoker, truncateSummary(sk.Title+" [invoked]", 140), sk.ID)

	payload := map[string]any{
		"persisted": true,
		"duplicate": false,
		"skill":     *sk,
		"channel":   channel,
	}
	if err := b.rememberMutationAckLocked("skill:invoke", requestID, payload); err != nil {
		b.mu.Unlock()
		http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
		return
	}
	if err := b.saveLocked(); err != nil {
		b.mu.Unlock()
		http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
		return
	}
	b.mu.Unlock()

	b.respondPersistedMutation(w, payload)
}

// parseSkillProposalLocked extracts a [SKILL PROPOSAL] block from a message
// and creates a proposed skill. Must be called with b.mu held.
func (b *Broker) parseSkillProposalLocked(msg channelMessage) {
	// Only the team lead (CEO) may propose skills via message blocks.
	// If no lead exists (empty office), reject all proposals to prevent injection.
	lead := officeLeadSlugFrom(b.members)
	if lead == "" || msg.From != lead {
		return
	}

	const startTag = "[SKILL PROPOSAL]"
	const endTag = "[/SKILL PROPOSAL]"

	channel := msg.Channel
	if channel == "" {
		channel = "general"
	}

	content := msg.Content
	searchFrom := 0
	for {
		startIdx := strings.Index(content[searchFrom:], startTag)
		if startIdx < 0 {
			return
		}
		startIdx += searchFrom
		blockStart := startIdx + len(startTag)
		endRel := strings.Index(content[blockStart:], endTag)
		if endRel < 0 {
			return
		}
		endIdx := blockStart + endRel
		block := strings.TrimSpace(content[blockStart:endIdx])
		searchFrom = endIdx + len(endTag)

		// Split on "---" separator between metadata and instructions.
		parts := strings.SplitN(block, "---", 2)
		if len(parts) < 2 {
			continue
		}

		meta := strings.TrimSpace(parts[0])
		instructions := strings.TrimSpace(parts[1])

		// Parse metadata fields.
		var name, title, description, trigger string
		var tags []string
		for _, line := range strings.Split(meta, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Name:") {
				name = strings.TrimSpace(strings.TrimPrefix(line, "Name:"))
			} else if strings.HasPrefix(line, "Title:") {
				title = strings.TrimSpace(strings.TrimPrefix(line, "Title:"))
			} else if strings.HasPrefix(line, "Description:") {
				description = strings.TrimSpace(strings.TrimPrefix(line, "Description:"))
			} else if strings.HasPrefix(line, "Trigger:") {
				trigger = strings.TrimSpace(strings.TrimPrefix(line, "Trigger:"))
			} else if strings.HasPrefix(line, "Tags:") {
				raw := strings.TrimSpace(strings.TrimPrefix(line, "Tags:"))
				for _, t := range strings.Split(raw, ",") {
					t = strings.TrimSpace(t)
					if t != "" {
						tags = append(tags, t)
					}
				}
			}
		}

		if name == "" || title == "" {
			continue
		}

		slug := skillSlug(name)

		// Check for duplicate (skip archived).
		duplicate := false
		for _, s := range b.skills {
			if s.Name == slug && s.Status != "archived" {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}

		now := time.Now().UTC().Format(time.RFC3339)
		skill := teamSkill{
			ID:          slug,
			Name:        slug,
			Title:       title,
			Description: description,
			Content:     instructions,
			CreatedBy:   msg.From,
			Channel:     channel,
			Tags:        tags,
			Trigger:     trigger,
			UsageCount:  0,
			Status:      "proposed",
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		b.skills = append(b.skills, skill)

		// Announce the proposal.
		b.counter++
		b.appendMessageLocked(channelMessage{
			ID:        fmt.Sprintf("msg-%d", b.counter),
			From:      "system",
			Channel:   channel,
			Kind:      "skill_proposal",
			Title:     "Skill Proposed: " + title,
			Content:   fmt.Sprintf("@%s proposed a new skill **%s**: %s. Use /skills to review and approve.", msg.From, title, description),
			Timestamp: now,
		})

		// Surface the proposal in the Requests panel as a non-blocking human decision.
		b.counter++
		interview := humanInterview{
			ID:        fmt.Sprintf("request-%d", b.counter),
			Kind:      "skill_proposal",
			Status:    "pending",
			From:      msg.From,
			Channel:   channel,
			Title:     "Approve skill: " + title,
			Question:  fmt.Sprintf("@%s proposed skill **%s**: %s\n\nActivate it?", msg.From, title, description),
			ReplyTo:   slug,
			Blocking:  false,
			CreatedAt: now,
			UpdatedAt: now,
		}
		interview.Options, interview.RecommendedID = normalizeRequestOptions(interview.Kind, "accept", []interviewOption{
			{ID: "accept", Label: "Accept"},
			{ID: "reject", Label: "Reject"},
		})
		b.requests = append(b.requests, interview)
	}
}

// SeedDefaultSkills pre-populates the broker with the pack's default skills.
// It is idempotent: skills whose name already exists (by slug) are skipped.
// Call this after broker.Start() from the Launcher so that the first time a
// pack is launched the team has its playbooks ready to reference.
func (b *Broker) SeedDefaultSkills(specs []agent.PackSkillSpec) {
	if len(specs) == 0 {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, spec := range specs {
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			continue
		}
		if b.findSkillByNameLocked(name) != nil {
			continue // already exists, skip
		}
		title := strings.TrimSpace(spec.Title)
		if title == "" {
			title = name
		}
		b.counter++
		sk := teamSkill{
			ID:          fmt.Sprintf("skill-%s", skillSlug(name)),
			Name:        name,
			Title:       title,
			Description: strings.TrimSpace(spec.Description),
			Content:     strings.TrimSpace(spec.Content),
			CreatedBy:   "system",
			Tags:        append([]string(nil), spec.Tags...),
			Trigger:     strings.TrimSpace(spec.Trigger),
			Status:      "active",
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		b.skills = append(b.skills, sk)
	}
	if err := b.saveLocked(); err != nil {
		log.Printf("broker: saveLocked after seeding skills: %v", err)
	}
}

// dirExists returns true if path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
