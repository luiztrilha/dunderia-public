package team

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
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
)

var scheduleTaskGitHubAsync = func(fn func()) { go fn() }
var runTaskGitHubWebhookReconcile = func(b *Broker) {
	if b == nil {
		return
	}
	_, _ = b.ReconcileTaskGitHubPublications(time.Now().UTC())
}
var runTaskGitHubWebhookRefreshPublication = func(b *Broker, taskID, kind string) {
	if b == nil {
		return
	}
	_, _, _ = b.refreshTaskGitHubPublicationNow(taskID, kind)
}

var taskGitHubPublicWindowsPathPattern = regexp.MustCompile(`(?i)\b[A-Z]:\\`)
var taskGitHubPublicUnixPathPattern = regexp.MustCompile(`(?i)(^|[\s(])/(users|home|var/folders|private/var|tmp|mnt|workspaces?)\b`)

const (
	taskGitHubAuditInterval   = 3 * time.Minute
	taskGitHubAuditJobSlug    = "github-publication-audit"
	taskGitHubAuditJobKind    = "github_publication_audit"
	taskGitHubAuditTargetType = "github_publication"
	taskGitHubMarkerPrefix    = "DUNDERIA-TASK-ID: "
)

var taskGitHubLookPath = exec.LookPath
var taskGitHubPublicationLocks sync.Map

var taskGitHubRunCommand = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = dir
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

var taskGitHubGitOutput = runGitOutput
var taskGitHubRunGit = runGit

func taskGitHubWebhookSecret() string {
	return strings.TrimSpace(os.Getenv("WUPHF_GITHUB_WEBHOOK_SECRET"))
}

type taskGitHubPublicationPolicy struct {
	Enabled   string   `json:"enabled,omitempty"`
	IssueMode string   `json:"issue_mode,omitempty"`
	PRMode    string   `json:"pr_mode,omitempty"`
	SyncMode  string   `json:"sync_mode,omitempty"`
	Labels    []string `json:"labels,omitempty"`
}

type taskDerivedDemandRef struct {
	SourceTaskID string `json:"source_task_id,omitempty"`
	SourceKind   string `json:"source_kind,omitempty"`
	SourceItemID string `json:"source_item_id,omitempty"`
}

type taskGitHubAuditState struct {
	LastRunAt     string `json:"last_run_at,omitempty"`
	LastSuccessAt string `json:"last_success_at,omitempty"`
	LastError     string `json:"last_error,omitempty"`
	PendingCount  int    `json:"pending_count,omitempty"`
	DeferredCount int    `json:"deferred_count,omitempty"`
	OpenedCount   int    `json:"opened_count,omitempty"`
	OpenedTotal   int    `json:"opened_total,omitempty"`
	UpdatedTotal  int    `json:"updated_total,omitempty"`
	ClosedTotal   int    `json:"closed_total,omitempty"`
	ReopenedTotal int    `json:"reopened_total,omitempty"`
	DeferredTotal int    `json:"deferred_total,omitempty"`
	RetriedTotal  int    `json:"retried_total,omitempty"`
	SyncedTotal   int    `json:"synced_total,omitempty"`
}

type taskGitHubRepoContext struct {
	RootPath            string                       `json:"root_path,omitempty"`
	RemoteURL           string                       `json:"remote_url,omitempty"`
	Owner               string                       `json:"owner,omitempty"`
	Repo                string                       `json:"repo,omitempty"`
	BaseBranch          string                       `json:"base_branch,omitempty"`
	Visibility          string                       `json:"visibility,omitempty"`
	Private             bool                         `json:"private,omitempty"`
	DefaultLabels       []string                     `json:"default_labels,omitempty"`
	PublicationDefaults *taskGitHubPublicationPolicy `json:"publication_defaults,omitempty"`
}

type taskGitHubPublication struct {
	Status               string   `json:"status,omitempty"`
	Number               int      `json:"number,omitempty"`
	URL                  string   `json:"url,omitempty"`
	Title                string   `json:"title,omitempty"`
	HeadBranch           string   `json:"head_branch,omitempty"`
	BaseBranch           string   `json:"base_branch,omitempty"`
	ExternalID           string   `json:"external_id,omitempty"`
	State                string   `json:"state,omitempty"`
	LastAttemptAt        string   `json:"last_attempt_at,omitempty"`
	LastSyncedAt         string   `json:"last_synced_at,omitempty"`
	LastError            string   `json:"last_error,omitempty"`
	RetryCount           int      `json:"retry_count,omitempty"`
	Draft                bool     `json:"draft,omitempty"`
	DesiredSignature     string   `json:"desired_signature,omitempty"`
	ReviewDecision       string   `json:"review_decision,omitempty"`
	ChecksState          string   `json:"checks_state,omitempty"`
	ChecksSummary        []string `json:"checks_summary,omitempty"`
	CommentCount         int      `json:"comment_count,omitempty"`
	LatestCommentAt      string   `json:"latest_comment_at,omitempty"`
	LatestCommentAuthor  string   `json:"latest_comment_author,omitempty"`
	LatestCommentSnippet string   `json:"latest_comment_snippet,omitempty"`
	MergedAt             string   `json:"merged_at,omitempty"`
}

type taskGitHubAPIResponse struct {
	Number      int                    `json:"number"`
	HTMLURL     string                 `json:"html_url"`
	NodeID      string                 `json:"node_id,omitempty"`
	State       string                 `json:"state,omitempty"`
	Draft       bool                   `json:"draft,omitempty"`
	MergedAt    string                 `json:"merged_at,omitempty"`
	Title       string                 `json:"title,omitempty"`
	Body        string                 `json:"body,omitempty"`
	Labels      []taskGitHubLabel      `json:"labels,omitempty"`
	Repository  *taskGitHubRepoPayload `json:"repository,omitempty"`
	PullRequest *taskGitHubPullRequest `json:"pull_request,omitempty"`
}

type taskGitHubPullRequest struct {
	MergedAt string `json:"merged_at,omitempty"`
}

type taskGitHubLabel struct {
	Name string `json:"name,omitempty"`
}

type taskGitHubRepoPayload struct {
	Private       bool   `json:"private,omitempty"`
	Visibility    string `json:"visibility,omitempty"`
	DefaultBranch string `json:"default_branch,omitempty"`
}

type taskGitHubSearchResponse struct {
	Items []taskGitHubAPIResponse `json:"items"`
}

type taskGitHubPRViewResponse struct {
	ReviewDecision    string                    `json:"reviewDecision,omitempty"`
	Reviews           []taskGitHubPRViewReview  `json:"reviews,omitempty"`
	Comments          []taskGitHubPRViewComment `json:"comments,omitempty"`
	StatusCheckRollup []map[string]any          `json:"statusCheckRollup,omitempty"`
}

type taskGitHubActor struct {
	Login string `json:"login,omitempty"`
}

type taskGitHubPRViewReview struct {
	State       string           `json:"state,omitempty"`
	Body        string           `json:"body,omitempty"`
	SubmittedAt string           `json:"submittedAt,omitempty"`
	Author      *taskGitHubActor `json:"author,omitempty"`
}

type taskGitHubPRViewComment struct {
	Body      string           `json:"body,omitempty"`
	CreatedAt string           `json:"createdAt,omitempty"`
	UpdatedAt string           `json:"updatedAt,omitempty"`
	Author    *taskGitHubActor `json:"author,omitempty"`
}

type taskGitHubRemoteReviewSnapshot struct {
	ReviewDecision       string
	ChecksState          string
	ChecksSummary        []string
	CommentCount         int
	LatestCommentAt      string
	LatestCommentAuthor  string
	LatestCommentSnippet string
}

type taskGitHubWebhookPayload struct {
	Action      string                         `json:"action,omitempty"`
	Ref         string                         `json:"ref,omitempty"`
	Repository  *taskGitHubWebhookRepository   `json:"repository,omitempty"`
	Issue       *taskGitHubWebhookIssuePayload `json:"issue,omitempty"`
	PullRequest *taskGitHubWebhookPRPayload    `json:"pull_request,omitempty"`
	CheckRun    *taskGitHubWebhookCheckPayload `json:"check_run,omitempty"`
	CheckSuite  *taskGitHubWebhookCheckPayload `json:"check_suite,omitempty"`
}

type taskGitHubWebhookRepository struct {
	FullName string `json:"full_name,omitempty"`
}

type taskGitHubWebhookIssuePayload struct {
	Number      int            `json:"number,omitempty"`
	HTMLURL     string         `json:"html_url,omitempty"`
	Body        string         `json:"body,omitempty"`
	PullRequest map[string]any `json:"pull_request,omitempty"`
}

type taskGitHubWebhookPRPayload struct {
	Number  int                         `json:"number,omitempty"`
	HTMLURL string                      `json:"html_url,omitempty"`
	Body    string                      `json:"body,omitempty"`
	Head    *taskGitHubWebhookBranchRef `json:"head,omitempty"`
}

type taskGitHubWebhookBranchRef struct {
	Ref string `json:"ref,omitempty"`
}

type taskGitHubWebhookCheckPayload struct {
	HeadBranch   string                         `json:"head_branch,omitempty"`
	PullRequests []taskGitHubWebhookPRNumberRef `json:"pull_requests,omitempty"`
}

type taskGitHubWebhookPRNumberRef struct {
	Number int `json:"number,omitempty"`
}

type taskGitHubWebhookOperation struct {
	TaskID string
	Kind   string
}

func normalizeTaskGitHubPublicationStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pending", "opened", "deferred", "failed":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func taskSupportsGitHubPublication(task *teamTask) bool {
	return task != nil && strings.TrimSpace(effectiveTeamTaskWorkspacePath(task)) != ""
}

func normalizeTaskGitHubPublicationPolicy(policy *taskGitHubPublicationPolicy) *taskGitHubPublicationPolicy {
	if policy == nil {
		return nil
	}
	copyPolicy := &taskGitHubPublicationPolicy{
		Enabled:   strings.ToLower(strings.TrimSpace(policy.Enabled)),
		IssueMode: strings.ToLower(strings.TrimSpace(policy.IssueMode)),
		PRMode:    strings.ToLower(strings.TrimSpace(policy.PRMode)),
		SyncMode:  strings.ToLower(strings.TrimSpace(policy.SyncMode)),
		Labels:    compactStringList(policy.Labels),
	}
	if copyPolicy.Enabled == "" {
		copyPolicy.Enabled = "auto"
	}
	switch copyPolicy.Enabled {
	case "auto", "disabled", "approved":
	default:
		copyPolicy.Enabled = "auto"
	}
	if copyPolicy.IssueMode == "" {
		copyPolicy.IssueMode = "auto"
	}
	switch copyPolicy.IssueMode {
	case "auto", "immediate", "skip":
	default:
		copyPolicy.IssueMode = "auto"
	}
	if copyPolicy.PRMode == "" {
		copyPolicy.PRMode = "auto"
	}
	switch copyPolicy.PRMode {
	case "auto", "draft", "skip":
	default:
		copyPolicy.PRMode = "auto"
	}
	if copyPolicy.SyncMode == "" {
		copyPolicy.SyncMode = "auto"
	}
	switch copyPolicy.SyncMode {
	case "auto", "manual":
	default:
		copyPolicy.SyncMode = "auto"
	}
	return copyPolicy
}

func cloneTaskGitHubPublicationPolicy(policy *taskGitHubPublicationPolicy) *taskGitHubPublicationPolicy {
	normalized := normalizeTaskGitHubPublicationPolicy(policy)
	if normalized == nil {
		return nil
	}
	copyPolicy := *normalized
	copyPolicy.Labels = append([]string(nil), normalized.Labels...)
	return &copyPolicy
}

func withTaskGitHubPublicationLock(taskID, kind string, fn func() (teamTask, bool, error)) (teamTask, bool, error) {
	key := strings.TrimSpace(taskID) + "|" + strings.ToLower(strings.TrimSpace(kind))
	if key == "|" {
		return fn()
	}
	lockValue, _ := taskGitHubPublicationLocks.LoadOrStore(key, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	return fn()
}

func (ch *teamChannel) linkedRepoPublicationDefaults(repoPath string) *taskGitHubPublicationPolicy {
	if ch == nil {
		return nil
	}
	repoPath = strings.TrimSpace(repoPath)
	if repoPath != "" {
		for i := range ch.LinkedRepos {
			if sameCleanPath(ch.LinkedRepos[i].RepoPath, repoPath) {
				return cloneTaskGitHubPublicationPolicy(ch.LinkedRepos[i].PublicationDefaults)
			}
		}
	}
	primaryPath := strings.TrimSpace(ch.primaryLinkedRepoPath())
	if primaryPath == "" {
		return nil
	}
	for i := range ch.LinkedRepos {
		if sameCleanPath(ch.LinkedRepos[i].RepoPath, primaryPath) {
			return cloneTaskGitHubPublicationPolicy(ch.LinkedRepos[i].PublicationDefaults)
		}
	}
	return nil
}

func (b *Broker) taskGitHubPublicationDefaultsForTask(task *teamTask, repoPath string) *taskGitHubPublicationPolicy {
	if b == nil || task == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := b.findChannelLocked(task.Channel)
	if ch == nil {
		return nil
	}
	return ch.linkedRepoPublicationDefaults(repoPath)
}

func effectiveTaskGitHubPublicationPolicy(task *teamTask, repoCtx *taskGitHubRepoContext) taskGitHubPublicationPolicy {
	base := normalizeTaskGitHubPublicationPolicy(&taskGitHubPublicationPolicy{
		Enabled:   "auto",
		IssueMode: "auto",
		PRMode:    "auto",
		SyncMode:  "auto",
		Labels:    []string{"dunderia"},
	})
	if repoCtx != nil && repoCtx.PublicationDefaults != nil {
		base = normalizeTaskGitHubPublicationPolicy(repoCtx.PublicationDefaults)
		if base == nil {
			base = &taskGitHubPublicationPolicy{Enabled: "auto", IssueMode: "auto", PRMode: "auto", SyncMode: "auto"}
		}
		base.Labels = compactStringList(append(append([]string(nil), repoCtx.DefaultLabels...), base.Labels...))
	}
	if task != nil {
		if task.TaskType != "" {
			base.Labels = compactStringList(append(base.Labels, sanitizeTaskGitHubLabel(task.TaskType)))
		}
		if task.Channel != "" {
			base.Labels = compactStringList(append(base.Labels, "channel-"+sanitizeTaskGitHubLabel(task.Channel)))
		}
		if task.PublicationPolicy != nil {
			override := normalizeTaskGitHubPublicationPolicy(task.PublicationPolicy)
			if override != nil {
				if override.Enabled != "" {
					base.Enabled = override.Enabled
				}
				if override.IssueMode != "" {
					base.IssueMode = override.IssueMode
				}
				if override.PRMode != "" {
					base.PRMode = override.PRMode
				}
				if override.SyncMode != "" {
					base.SyncMode = override.SyncMode
				}
				base.Labels = compactStringList(append(base.Labels, override.Labels...))
			}
		}
	}
	return *base
}

func sanitizeTaskGitHubLabel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "-", "_", "-", "/", "-", "\\", "-", ":", "-", ".", "-")
	value = replacer.Replace(value)
	value = strings.Trim(value, "-")
	return value
}

func taskGitHubIssueMode(task *teamTask, repoCtx *taskGitHubRepoContext) string {
	return effectiveTaskGitHubPublicationPolicy(task, repoCtx).IssueMode
}

func taskGitHubPRMode(task *teamTask, repoCtx *taskGitHubRepoContext) string {
	return effectiveTaskGitHubPublicationPolicy(task, repoCtx).PRMode
}

func taskGitHubSyncMode(task *teamTask, repoCtx *taskGitHubRepoContext) string {
	return effectiveTaskGitHubPublicationPolicy(task, repoCtx).SyncMode
}

type taskGitHubDesiredArtifactState struct {
	Title     string
	Body      string
	Labels    []string
	State     string
	Draft     bool
	Signature string
}

func taskGitHubIssueShouldPublish(task *teamTask, repoCtx *taskGitHubRepoContext) bool {
	if task == nil || !taskSupportsGitHubPublication(task) {
		return false
	}
	if taskIsTerminal(task) {
		return false
	}
	policy := effectiveTaskGitHubPublicationPolicy(task, repoCtx)
	if policy.Enabled == "disabled" || policy.IssueMode == "skip" {
		return false
	}
	if policy.IssueMode == "immediate" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(task.Status), "in_progress")
}

func taskGitHubPRShouldPublish(task *teamTask, repoCtx *taskGitHubRepoContext) bool {
	if task == nil || !taskSupportsGitHubPublication(task) {
		return false
	}
	if taskIsTerminal(task) {
		return false
	}
	policy := effectiveTaskGitHubPublicationPolicy(task, repoCtx)
	if policy.Enabled == "disabled" || policy.PRMode == "skip" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(task.Status), "review")
}

func taskGitHubShouldSync(task *teamTask, repoCtx *taskGitHubRepoContext) bool {
	if task == nil || !taskSupportsGitHubPublication(task) {
		return false
	}
	policy := effectiveTaskGitHubPublicationPolicy(task, repoCtx)
	return policy.Enabled != "disabled" && policy.SyncMode != "manual"
}

func taskGitHubPublicationApproved(task *teamTask, repoCtx *taskGitHubRepoContext) bool {
	if repoCtx == nil {
		return true
	}
	policy := effectiveTaskGitHubPublicationPolicy(task, repoCtx)
	if policy.Enabled == "disabled" {
		return false
	}
	if repoCtx.Private || strings.EqualFold(strings.TrimSpace(repoCtx.Visibility), "private") || strings.EqualFold(strings.TrimSpace(repoCtx.Visibility), "internal") {
		return true
	}
	return policy.Enabled == "approved"
}

func taskGitHubDesiredIssueState(task *teamTask, repoCtx *taskGitHubRepoContext) taskGitHubDesiredArtifactState {
	state := "open"
	if taskIsTerminal(task) {
		state = "closed"
	}
	desired := taskGitHubDesiredArtifactState{
		Title:  buildTaskGitHubIssueTitle(task),
		Body:   buildTaskGitHubIssueBody(task, repoCtx),
		Labels: taskGitHubLogicalLabels(task, repoCtx),
		State:  state,
	}
	desired.Signature = taskGitHubDesiredSignature(desired)
	return desired
}

func taskGitHubDesiredPRState(task *teamTask, repoCtx *taskGitHubRepoContext) taskGitHubDesiredArtifactState {
	state := "open"
	if taskIsTerminal(task) {
		state = "closed"
	}
	desired := taskGitHubDesiredArtifactState{
		Title:  buildTaskGitHubPRTitle(task),
		Body:   buildTaskGitHubPRBody(task),
		Labels: taskGitHubLogicalLabels(task, repoCtx),
		State:  state,
		Draft:  !strings.EqualFold(strings.TrimSpace(task.Status), "review"),
	}
	desired.Signature = taskGitHubDesiredSignature(desired)
	return desired
}

func taskGitHubDesiredSignature(desired taskGitHubDesiredArtifactState) string {
	labels := append([]string(nil), desired.Labels...)
	sort.Strings(labels)
	return strings.Join([]string{
		strings.TrimSpace(desired.Title),
		strings.TrimSpace(desired.Body),
		strings.Join(labels, ","),
		strings.TrimSpace(desired.State),
		strconv.FormatBool(desired.Draft),
	}, "\n---\n")
}

func normalizeTaskGitHubReviewDecision(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "approved":
		return "approved"
	case "changes_requested":
		return "changes_requested"
	case "review_required":
		return "review_required"
	default:
		return ""
	}
}

func normalizeTaskGitHubChecksState(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "passing":
		return "passing"
	case "pending":
		return "pending"
	case "failing":
		return "failing"
	default:
		return ""
	}
}

func taskGitHubPublicRedactionError(repoCtx *taskGitHubRepoContext, desired taskGitHubDesiredArtifactState) error {
	if repoCtx == nil || repoCtx.Private || strings.EqualFold(strings.TrimSpace(repoCtx.Visibility), "private") || strings.EqualFold(strings.TrimSpace(repoCtx.Visibility), "internal") {
		return nil
	}
	text := strings.ToLower(strings.TrimSpace(desired.Title + "\n" + desired.Body))
	if text == "" {
		return nil
	}
	fragments := []string{
		".wuphf",
		"broker-state.json",
		"company.json",
		"task-worktrees",
		"temporaryitems",
		"working_directory",
		"execution_mode=local_worktree",
		"wuphf_runtime_home",
	}
	for _, fragment := range fragments {
		if strings.Contains(text, fragment) {
			return fmt.Errorf("redaction_required: contains %s", fragment)
		}
	}
	if taskGitHubPublicWindowsPathPattern.MatchString(desired.Title) || taskGitHubPublicWindowsPathPattern.MatchString(desired.Body) {
		return fmt.Errorf("redaction_required: contains_windows_path")
	}
	if taskGitHubPublicUnixPathPattern.MatchString(desired.Title) || taskGitHubPublicUnixPathPattern.MatchString(desired.Body) {
		return fmt.Errorf("redaction_required: contains_unix_path")
	}
	return nil
}

func taskGitHubResponseLabels(response *taskGitHubAPIResponse) []string {
	if response == nil {
		return nil
	}
	labels := make([]string, 0, len(response.Labels))
	for _, label := range response.Labels {
		if value := strings.TrimSpace(label.Name); value != "" {
			labels = append(labels, value)
		}
	}
	return compactStringList(labels)
}

func taskGitHubArtifactNeedsContentSync(response *taskGitHubAPIResponse, desired taskGitHubDesiredArtifactState) bool {
	if response == nil {
		return true
	}
	if strings.TrimSpace(response.Title) != strings.TrimSpace(desired.Title) {
		return true
	}
	if strings.TrimSpace(response.Body) != strings.TrimSpace(desired.Body) {
		return true
	}
	currentLabels := taskGitHubResponseLabels(response)
	desiredLabels := append([]string(nil), desired.Labels...)
	sort.Strings(currentLabels)
	sort.Strings(desiredLabels)
	return strings.Join(currentLabels, ",") != strings.Join(desiredLabels, ",")
}

func taskGitHubMarker(taskID string) string {
	return taskGitHubMarkerPrefix + strings.TrimSpace(taskID)
}

func taskGitHubLogicalLabels(task *teamTask, repoCtx *taskGitHubRepoContext) []string {
	policy := effectiveTaskGitHubPublicationPolicy(task, repoCtx)
	return compactStringList(policy.Labels)
}

func mergeTaskGitHubPublicationPolicy(current, incoming *taskGitHubPublicationPolicy) *taskGitHubPublicationPolicy {
	if current == nil && incoming == nil {
		return nil
	}
	base := normalizeTaskGitHubPublicationPolicy(current)
	if base == nil {
		base = &taskGitHubPublicationPolicy{
			Enabled:   "auto",
			IssueMode: "auto",
			PRMode:    "auto",
			SyncMode:  "auto",
		}
	}
	override := normalizeTaskGitHubPublicationPolicy(incoming)
	if override == nil {
		return base
	}
	if override.Enabled != "" {
		base.Enabled = override.Enabled
	}
	if override.IssueMode != "" {
		base.IssueMode = override.IssueMode
	}
	if override.PRMode != "" {
		base.PRMode = override.PRMode
	}
	if override.SyncMode != "" {
		base.SyncMode = override.SyncMode
	}
	base.Labels = compactStringList(append(base.Labels, override.Labels...))
	return base
}

func buildTaskGitHubIssueTitle(task *teamTask) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.Title)
}

func buildTaskGitHubIssueBody(task *teamTask, repoCtx *taskGitHubRepoContext) string {
	if task == nil {
		return ""
	}
	lines := []string{
		taskGitHubMarker(strings.TrimSpace(task.ID)),
		"DUNDERIA-ARTIFACT: issue",
		fmt.Sprintf("Imported from DunderIA task `%s`.", strings.TrimSpace(task.ID)),
	}
	if channel := normalizeChannelSlug(task.Channel); channel != "" {
		lines = append(lines, "DUNDERIA-CHANNEL: "+channel)
		lines = append(lines, fmt.Sprintf("- Channel: `#%s`", channel))
	}
	if owner := strings.TrimSpace(task.Owner); owner != "" {
		lines = append(lines, fmt.Sprintf("- Owner: `@%s`", owner))
	}
	if mode := strings.TrimSpace(task.ExecutionMode); mode != "" {
		lines = append(lines, fmt.Sprintf("- Execution mode: `%s`", mode))
	}
	if repoCtx != nil && strings.TrimSpace(repoCtx.BaseBranch) != "" {
		lines = append(lines, fmt.Sprintf("- Base branch: `%s`", repoCtx.BaseBranch))
	}
	if labels := taskGitHubLogicalLabels(task, repoCtx); len(labels) > 0 {
		lines = append(lines, fmt.Sprintf("- Labels: `%s`", strings.Join(labels, "`, `")))
	}
	if sourceSignalID := strings.TrimSpace(task.SourceSignalID); sourceSignalID != "" {
		lines = append(lines, fmt.Sprintf("- Source signal: `%s`", sourceSignalID))
	}
	if sourceDecisionID := strings.TrimSpace(task.SourceDecisionID); sourceDecisionID != "" {
		lines = append(lines, fmt.Sprintf("- Source decision: `%s`", sourceDecisionID))
	}
	if task.DerivedFrom != nil {
		lines = append(lines, fmt.Sprintf("- Derived from task: `%s`", strings.TrimSpace(task.DerivedFrom.SourceTaskID)))
		lines = append(lines, fmt.Sprintf("- Demand source: `%s/%s`", strings.TrimSpace(task.DerivedFrom.SourceKind), strings.TrimSpace(task.DerivedFrom.SourceItemID)))
	}
	if details := strings.TrimSpace(task.Details); details != "" {
		lines = append(lines, "", "## Details", "", details)
	}
	if task.Reconciliation != nil {
		if len(task.Reconciliation.ChangedPaths) > 0 {
			lines = append(lines, "", "## Changed Paths")
			for _, path := range task.Reconciliation.ChangedPaths {
				lines = append(lines, "- "+strings.TrimSpace(path))
			}
		}
		if len(task.Reconciliation.UntrackedPaths) > 0 {
			lines = append(lines, "", "## Untracked Paths")
			for _, path := range task.Reconciliation.UntrackedPaths {
				lines = append(lines, "- "+strings.TrimSpace(path))
			}
		}
	}
	return strings.Join(lines, "\n")
}

func buildTaskGitHubPRTitle(task *teamTask) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.Title)
}

func buildTaskGitHubPRBody(task *teamTask) string {
	if task == nil {
		return ""
	}
	lines := make([]string, 0, 16)
	if task.IssuePublication != nil && task.IssuePublication.Number > 0 {
		lines = append(lines, fmt.Sprintf("Closes #%d", task.IssuePublication.Number), "")
	}
	lines = append(lines,
		taskGitHubMarker(strings.TrimSpace(task.ID)),
		"DUNDERIA-ARTIFACT: pr",
		fmt.Sprintf("Imported from DunderIA task `%s`.", strings.TrimSpace(task.ID)),
	)
	if channel := normalizeChannelSlug(task.Channel); channel != "" {
		lines = append(lines, "DUNDERIA-CHANNEL: "+channel)
	}
	metadata := make([]string, 0, 8)
	if owner := strings.TrimSpace(task.Owner); owner != "" {
		metadata = append(metadata, fmt.Sprintf("- Owner: `@%s`", owner))
	}
	if mode := strings.TrimSpace(task.ExecutionMode); mode != "" {
		metadata = append(metadata, fmt.Sprintf("- Execution mode: `%s`", mode))
	}
	if sourceSignalID := strings.TrimSpace(task.SourceSignalID); sourceSignalID != "" {
		metadata = append(metadata, fmt.Sprintf("- Source signal: `%s`", sourceSignalID))
	}
	if sourceDecisionID := strings.TrimSpace(task.SourceDecisionID); sourceDecisionID != "" {
		metadata = append(metadata, fmt.Sprintf("- Source decision: `%s`", sourceDecisionID))
	}
	if task.DerivedFrom != nil {
		metadata = append(metadata, fmt.Sprintf("- Derived from task: `%s`", strings.TrimSpace(task.DerivedFrom.SourceTaskID)))
		metadata = append(metadata, fmt.Sprintf("- Demand source: `%s/%s`", strings.TrimSpace(task.DerivedFrom.SourceKind), strings.TrimSpace(task.DerivedFrom.SourceItemID)))
	}
	if len(metadata) > 0 {
		lines = append(lines, "", "## Metadata")
		lines = append(lines, metadata...)
	}
	if labels := taskGitHubLogicalLabels(task, nil); len(labels) > 0 {
		lines = append(lines, "", "## Logical Labels", "", strings.Join(labels, ", "))
	}
	if handoff := task.LastHandoff; handoff != nil {
		lines = append(lines, "", "## Task Report", "", strings.TrimSpace(handoff.Summary))
		if len(handoff.Validation) > 0 {
			lines = append(lines, "", "## Validation")
			for _, item := range handoff.Validation {
				lines = append(lines, "- "+strings.TrimSpace(item))
			}
		}
		if downstream := strings.TrimSpace(handoff.DownstreamContext); downstream != "" {
			lines = append(lines, "", "## Downstream Context", "", downstream)
		}
	} else if details := strings.TrimSpace(task.Details); details != "" {
		lines = append(lines, "", "## Context", "", details)
	}
	if task.Reconciliation != nil {
		if len(task.Reconciliation.ChangedPaths) > 0 {
			lines = append(lines, "", "## Changed Paths")
			for _, path := range task.Reconciliation.ChangedPaths {
				lines = append(lines, "- "+strings.TrimSpace(path))
			}
		}
		if len(task.Reconciliation.UntrackedPaths) > 0 {
			lines = append(lines, "", "## Untracked Paths")
			for _, path := range task.Reconciliation.UntrackedPaths {
				lines = append(lines, "- "+strings.TrimSpace(path))
			}
		}
	}
	return strings.Join(lines, "\n")
}

func (b *Broker) taskGitHubRepoContextForPolicyLocked(task *teamTask) *taskGitHubRepoContext {
	if task == nil {
		return nil
	}
	var repoCtx *taskGitHubRepoContext
	if task.RepoContext != nil {
		copyCtx := *task.RepoContext
		copyCtx.DefaultLabels = append([]string(nil), task.RepoContext.DefaultLabels...)
		copyCtx.PublicationDefaults = cloneTaskGitHubPublicationPolicy(task.RepoContext.PublicationDefaults)
		repoCtx = &copyCtx
	}
	ch := b.findChannelLocked(task.Channel)
	if ch == nil {
		return repoCtx
	}
	repoPath := strings.TrimSpace(effectiveTeamTaskWorkspacePath(task))
	defaults := ch.linkedRepoPublicationDefaults(repoPath)
	if defaults == nil {
		return repoCtx
	}
	if repoCtx == nil {
		repoCtx = &taskGitHubRepoContext{}
	}
	repoCtx.PublicationDefaults = defaults
	return repoCtx
}

func (b *Broker) queueTaskIssuePublicationLocked(task *teamTask) bool {
	if task == nil {
		return false
	}
	repoCtx := b.taskGitHubRepoContextForPolicyLocked(task)
	if !taskGitHubIssueShouldPublish(task, repoCtx) {
		return false
	}
	if task.IssuePublication == nil {
		task.IssuePublication = &taskGitHubPublication{}
	}
	switch normalizeTaskGitHubPublicationStatus(task.IssuePublication.Status) {
	case "opened", "pending":
		return false
	}
	task.IssuePublication.Status = "pending"
	task.IssuePublication.Title = buildTaskGitHubIssueTitle(task)
	task.IssuePublication.LastError = ""
	b.appendActionLocked("github_issue_open_requested", "office", normalizeChannelSlug(task.Channel), firstNonEmpty(strings.TrimSpace(task.Owner), strings.TrimSpace(task.CreatedBy), "system"), truncateSummary(task.Title, 140), task.ID)
	return true
}

func (b *Broker) queueTaskPRPublicationLocked(task *teamTask, handoff *taskHandoffRecord) bool {
	if task == nil || handoff == nil {
		return false
	}
	repoCtx := b.taskGitHubRepoContextForPolicyLocked(task)
	if !taskGitHubPRShouldPublish(task, repoCtx) {
		return false
	}
	if task.PRPublication == nil {
		task.PRPublication = &taskGitHubPublication{}
	}
	switch normalizeTaskGitHubPublicationStatus(task.PRPublication.Status) {
	case "opened", "pending":
		return false
	}
	task.PRPublication.Status = "pending"
	task.PRPublication.Title = buildTaskGitHubPRTitle(task)
	task.PRPublication.LastError = ""
	b.appendActionLocked("github_pr_open_requested", "office", normalizeChannelSlug(task.Channel), firstNonEmpty(strings.TrimSpace(task.Owner), strings.TrimSpace(task.CreatedBy), "system"), truncateSummary(task.Title, 140), task.ID)
	return true
}

func (b *Broker) maybeQueueTaskGitHubPublicationLocked(task *teamTask, previousStatus, action string, handoff *taskHandoffRecord) (bool, bool) {
	if task == nil {
		return false, false
	}
	previousStatus = strings.ToLower(strings.TrimSpace(previousStatus))
	currentStatus := strings.ToLower(strings.TrimSpace(task.Status))
	issueQueued := false
	prQueued := false
	repoCtx := b.taskGitHubRepoContextForPolicyLocked(task)
	if repoCtx != nil && task.RepoContext == nil {
		task.RepoContext = repoCtx
	}
	if taskGitHubIssueMode(task, repoCtx) == "immediate" && strings.EqualFold(strings.TrimSpace(action), "create") {
		issueQueued = b.queueTaskIssuePublicationLocked(task)
	} else if currentStatus == "in_progress" && previousStatus != "in_progress" {
		issueQueued = b.queueTaskIssuePublicationLocked(task)
	}
	if currentStatus == "review" && previousStatus != "review" {
		switch strings.ToLower(strings.TrimSpace(action)) {
		case "review", "complete":
			prQueued = b.queueTaskPRPublicationLocked(task, handoff)
		}
	}
	return issueQueued, prQueued
}

func (b *Broker) publishTaskIssueSoon(taskID string) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return
	}
	scheduleTaskGitHubAsync(func() {
		_, _, _ = b.publishTaskIssueNow(taskID)
	})
}

func (b *Broker) publishTaskPRSoon(taskID string) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return
	}
	scheduleTaskGitHubAsync(func() {
		_, _, _ = b.publishTaskPRNow(taskID)
	})
}

func (b *Broker) publishTaskIssueNow(taskID string) (teamTask, bool, error) {
	return withTaskGitHubPublicationLock(taskID, "issue", func() (teamTask, bool, error) {
		return b.publishTaskIssueNowLocked(taskID)
	})
}

func (b *Broker) publishTaskIssueNowLocked(taskID string) (teamTask, bool, error) {
	task, err := b.snapshotTaskForGitHubPublication(taskID, "issue")
	if err != nil {
		return teamTask{}, false, err
	}
	repoCtx, err := b.resolveTaskGitHubRepoContext(task)
	if err != nil {
		return b.deferTaskGitHubPublication(taskID, "issue", nil, "", err)
	}
	if err := ensureTaskGitHubCLI(); err != nil {
		return b.deferTaskGitHubPublication(taskID, "issue", repoCtx, "", err)
	}
	repoCtx, err = enrichTaskGitHubRepoContext(repoCtx)
	if err != nil {
		return b.deferTaskGitHubPublication(taskID, "issue", repoCtx, "", err)
	}
	if !taskGitHubPublicationApproved(task, repoCtx) {
		return b.deferTaskGitHubPublication(taskID, "issue", repoCtx, "", fmt.Errorf("public_repo_requires_approval"))
	}
	desired := taskGitHubDesiredIssueState(task, repoCtx)
	if err := taskGitHubPublicRedactionError(repoCtx, desired); err != nil {
		return b.deferTaskGitHubPublication(taskID, "issue", repoCtx, "", err)
	}
	if existing, err := findTaskGitHubIssueByMarker(repoCtx, task.ID); err != nil {
		return b.deferTaskGitHubPublication(taskID, "issue", repoCtx, "", err)
	} else if existing != nil {
		return b.adoptTaskGitHubPublication(taskID, "issue", repoCtx, existing, "", repoCtx.BaseBranch, buildTaskGitHubIssueTitle(task))
	}
	resp, err := createTaskGitHubIssue(repoCtx, desired.Title, desired.Body)
	if err != nil {
		return b.deferTaskGitHubPublication(taskID, "issue", repoCtx, "", err)
	}
	return b.completeTaskGitHubPublication(taskID, "issue", repoCtx, resp, "", repoCtx.BaseBranch, desired.Title)
}

func (b *Broker) publishTaskPRNow(taskID string) (teamTask, bool, error) {
	return withTaskGitHubPublicationLock(taskID, "pr", func() (teamTask, bool, error) {
		return b.publishTaskPRNowLocked(taskID)
	})
}

func (b *Broker) publishTaskPRNowLocked(taskID string) (teamTask, bool, error) {
	task, err := b.snapshotTaskForGitHubPublication(taskID, "pr")
	if err != nil {
		return teamTask{}, false, err
	}
	repoCtx, err := b.resolveTaskGitHubRepoContext(task)
	if err != nil {
		return b.deferTaskGitHubPublication(taskID, "pr", nil, "", err)
	}
	if err := ensureTaskGitHubCLI(); err != nil {
		return b.deferTaskGitHubPublication(taskID, "pr", repoCtx, "", err)
	}
	repoCtx, err = enrichTaskGitHubRepoContext(repoCtx)
	if err != nil {
		return b.deferTaskGitHubPublication(taskID, "pr", repoCtx, "", err)
	}
	if !taskGitHubPublicationApproved(task, repoCtx) {
		return b.deferTaskGitHubPublication(taskID, "pr", repoCtx, "", fmt.Errorf("public_repo_requires_approval"))
	}
	desired := taskGitHubDesiredPRState(task, repoCtx)
	if err := taskGitHubPublicRedactionError(repoCtx, desired); err != nil {
		return b.deferTaskGitHubPublication(taskID, "pr", repoCtx, "", err)
	}
	if existing, err := findTaskGitHubPRByMarker(repoCtx, task.ID); err != nil {
		return b.deferTaskGitHubPublication(taskID, "pr", repoCtx, "", err)
	} else if existing != nil {
		headBranch := ""
		if task.PRPublication != nil {
			headBranch = strings.TrimSpace(task.PRPublication.HeadBranch)
		}
		return b.adoptTaskGitHubPublication(taskID, "pr", repoCtx, existing, headBranch, repoCtx.BaseBranch, buildTaskGitHubPRTitle(task))
	}
	headBranch, err := resolveTaskGitHubHeadBranch(task, repoCtx.RootPath)
	if err != nil {
		return b.deferTaskGitHubPublication(taskID, "pr", repoCtx, "", err)
	}
	hasCommits, err := taskGitHubBranchHasCommitsAhead(repoCtx.RootPath, repoCtx.BaseBranch, headBranch)
	if err != nil {
		return b.deferTaskGitHubPublication(taskID, "pr", repoCtx, headBranch, err)
	}
	if !hasCommits {
		return b.deferTaskGitHubPublication(taskID, "pr", repoCtx, headBranch, fmt.Errorf("no commits ahead of %s on %s", repoCtx.BaseBranch, headBranch))
	}
	if err := taskGitHubRunGit(repoCtx.RootPath, "push", "--set-upstream", "origin", headBranch); err != nil {
		return b.deferTaskGitHubPublication(taskID, "pr", repoCtx, headBranch, err)
	}
	resp, err := createTaskGitHubPR(repoCtx, headBranch, desired.Title, desired.Body, true)
	if err != nil {
		return b.deferTaskGitHubPublication(taskID, "pr", repoCtx, headBranch, err)
	}
	return b.completeTaskGitHubPublication(taskID, "pr", repoCtx, resp, headBranch, repoCtx.BaseBranch, desired.Title)
}

func (b *Broker) RetryTaskIssuePublication(taskID, actor string) (teamTask, bool, error) {
	return withTaskGitHubPublicationLock(taskID, "issue", func() (teamTask, bool, error) {
		return b.retryTaskGitHubPublicationLocked(taskID, actor, "issue")
	})
}

func (b *Broker) RetryTaskPRPublication(taskID, actor string) (teamTask, bool, error) {
	return withTaskGitHubPublicationLock(taskID, "pr", func() (teamTask, bool, error) {
		return b.retryTaskGitHubPublicationLocked(taskID, actor, "pr")
	})
}

func (b *Broker) retryTaskGitHubPublication(taskID, actor, kind string) (teamTask, bool, error) {
	return withTaskGitHubPublicationLock(taskID, kind, func() (teamTask, bool, error) {
		return b.retryTaskGitHubPublicationLocked(taskID, actor, kind)
	})
}

func (b *Broker) retryTaskGitHubPublicationLocked(taskID, actor, kind string) (teamTask, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return teamTask{}, false, fmt.Errorf("task id required")
	}
	actor = firstNonEmpty(strings.TrimSpace(actor), "system")
	for i := range b.tasks {
		task := &b.tasks[i]
		if task.ID != taskID {
			continue
		}
		if !taskSupportsGitHubPublication(task) {
			return teamTask{}, false, fmt.Errorf("task %s is not associated with a publishable workspace", task.ID)
		}
		target := taskGitHubPublicationForKind(task, kind)
		if target == nil {
			target = &taskGitHubPublication{}
			setTaskGitHubPublicationForKind(task, kind, target)
		}
		if normalizeTaskGitHubPublicationStatus(target.Status) == "opened" {
			return teamTask{}, false, fmt.Errorf("%s publication already opened for task %s", kind, task.ID)
		}
		target.Status = "pending"
		target.LastError = ""
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		b.gitHubPublicationAudit.RetriedTotal++
		b.appendActionLocked("github_"+kind+"_open_requested", "office", normalizeChannelSlug(task.Channel), actor, truncateSummary(task.Title, 140), task.ID)
		if err := b.saveLocked(); err != nil {
			return teamTask{}, false, err
		}
		return *task, true, nil
	}
	return teamTask{}, false, fmt.Errorf("task not found")
}

func (b *Broker) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	secret := taskGitHubWebhookSecret()
	if secret == "" {
		http.Error(w, "github webhook disabled", http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "failed to read webhook body", http.StatusBadRequest)
		return
	}
	if !taskGitHubWebhookSignatureValid(secret, body, r.Header.Get("X-Hub-Signature-256")) {
		http.Error(w, "invalid github webhook signature", http.StatusUnauthorized)
		return
	}
	event := strings.TrimSpace(r.Header.Get("X-GitHub-Event"))
	if event == "" {
		http.Error(w, "missing github webhook event", http.StatusBadRequest)
		return
	}
	if strings.EqualFold(event, "ping") {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "event": event, "queued": false})
		return
	}
	if !taskGitHubWebhookShouldReconcile(event) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "event": event, "queued": false, "ignored": true})
		return
	}
	ops, targeted := b.taskGitHubWebhookOperations(event, body)
	scheduleTaskGitHubAsync(func() {
		if targeted {
			for _, op := range ops {
				runTaskGitHubWebhookRefreshPublication(b, op.TaskID, op.Kind)
			}
			return
		}
		runTaskGitHubWebhookReconcile(b)
	})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "event": event, "queued": true, "targeted": targeted})
}

func taskGitHubWebhookShouldReconcile(event string) bool {
	switch strings.TrimSpace(strings.ToLower(event)) {
	case "issues", "issue_comment", "pull_request", "pull_request_review", "pull_request_review_comment", "check_run", "check_suite", "push":
		return true
	default:
		return false
	}
}

func taskGitHubWebhookSignatureValid(secret string, body []byte, provided string) bool {
	secret = strings.TrimSpace(secret)
	provided = strings.TrimSpace(provided)
	if secret == "" || provided == "" || !strings.HasPrefix(strings.ToLower(provided), "sha256=") {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	expected := "sha256=" + fmt.Sprintf("%x", mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(provided))
}

func (b *Broker) taskGitHubWebhookOperations(event string, body []byte) ([]taskGitHubWebhookOperation, bool) {
	if b == nil || len(body) == 0 {
		return nil, false
	}
	var payload taskGitHubWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, false
	}

	repoFullName := strings.ToLower(strings.TrimSpace(func() string {
		if payload.Repository != nil {
			return payload.Repository.FullName
		}
		return ""
	}()))
	seen := map[string]struct{}{}
	ops := make([]taskGitHubWebhookOperation, 0, 4)
	addMatches := func(matches []taskGitHubWebhookOperation) {
		for _, match := range matches {
			key := strings.TrimSpace(match.TaskID) + "|" + strings.TrimSpace(match.Kind)
			if key == "|" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			ops = append(ops, match)
		}
	}

	switch strings.ToLower(strings.TrimSpace(event)) {
	case "issues":
		if payload.Issue != nil && payload.Issue.Number > 0 {
			addMatches(b.findTaskGitHubWebhookOperationsByNumber(repoFullName, "issue", payload.Issue.Number))
		}
	case "issue_comment":
		if payload.Issue != nil && payload.Issue.Number > 0 {
			kind := "issue"
			if len(payload.Issue.PullRequest) > 0 {
				kind = "pr"
			}
			addMatches(b.findTaskGitHubWebhookOperationsByNumber(repoFullName, kind, payload.Issue.Number))
		}
	case "pull_request", "pull_request_review", "pull_request_review_comment":
		if payload.PullRequest != nil && payload.PullRequest.Number > 0 {
			addMatches(b.findTaskGitHubWebhookOperationsByNumber(repoFullName, "pr", payload.PullRequest.Number))
		}
	case "check_run":
		if payload.CheckRun != nil {
			for _, pr := range payload.CheckRun.PullRequests {
				if pr.Number > 0 {
					addMatches(b.findTaskGitHubWebhookOperationsByNumber(repoFullName, "pr", pr.Number))
				}
			}
			if len(ops) == 0 {
				addMatches(b.findTaskGitHubWebhookOperationsByBranch(repoFullName, payload.CheckRun.HeadBranch))
			}
		}
	case "check_suite":
		if payload.CheckSuite != nil {
			for _, pr := range payload.CheckSuite.PullRequests {
				if pr.Number > 0 {
					addMatches(b.findTaskGitHubWebhookOperationsByNumber(repoFullName, "pr", pr.Number))
				}
			}
			if len(ops) == 0 {
				addMatches(b.findTaskGitHubWebhookOperationsByBranch(repoFullName, payload.CheckSuite.HeadBranch))
			}
		}
	case "push":
		branch := strings.TrimSpace(strings.TrimPrefix(payload.Ref, "refs/heads/"))
		if branch != "" {
			addMatches(b.findTaskGitHubWebhookOperationsByBranch(repoFullName, branch))
		}
	}

	return ops, len(ops) > 0
}

func (b *Broker) findTaskGitHubWebhookOperationsByNumber(repoFullName, kind string, number int) []taskGitHubWebhookOperation {
	if b == nil || number <= 0 {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	matches := make([]taskGitHubWebhookOperation, 0, 2)
	for _, task := range b.tasks {
		target := taskGitHubPublicationForKind(&task, kind)
		if target == nil || target.Number != number || normalizeTaskGitHubPublicationStatus(target.Status) != "opened" {
			continue
		}
		if !taskGitHubWebhookTaskMatchesRepo(&task, repoFullName, target.URL) {
			continue
		}
		matches = append(matches, taskGitHubWebhookOperation{
			TaskID: strings.TrimSpace(task.ID),
			Kind:   strings.TrimSpace(kind),
		})
	}
	return matches
}

func (b *Broker) findTaskGitHubWebhookOperationsByBranch(repoFullName, branch string) []taskGitHubWebhookOperation {
	if b == nil || strings.TrimSpace(branch) == "" {
		return nil
	}
	branch = strings.TrimSpace(branch)
	b.mu.Lock()
	defer b.mu.Unlock()

	matches := make([]taskGitHubWebhookOperation, 0, 2)
	for _, task := range b.tasks {
		if task.PRPublication == nil || normalizeTaskGitHubPublicationStatus(task.PRPublication.Status) != "opened" {
			continue
		}
		headBranch := strings.TrimSpace(firstNonEmpty(task.PRPublication.HeadBranch, task.WorktreeBranch))
		if headBranch != branch {
			continue
		}
		if !taskGitHubWebhookTaskMatchesRepo(&task, repoFullName, task.PRPublication.URL) {
			continue
		}
		matches = append(matches, taskGitHubWebhookOperation{
			TaskID: strings.TrimSpace(task.ID),
			Kind:   "pr",
		})
	}
	return matches
}

func taskGitHubWebhookTaskMatchesRepo(task *teamTask, repoFullName string, artifactURL string) bool {
	repoFullName = strings.ToLower(strings.TrimSpace(repoFullName))
	if repoFullName == "" {
		return true
	}
	if task != nil && task.RepoContext != nil {
		fullName := strings.ToLower(strings.TrimSpace(task.RepoContext.Owner + "/" + task.RepoContext.Repo))
		if fullName == repoFullName {
			return true
		}
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(artifactURL)), "github.com/"+repoFullName+"/")
}

func (b *Broker) EnsureGitHubPublicationAuditJob() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	nextRun := time.Now().UTC().Add(taskGitHubAuditInterval).Format(time.RFC3339)
	desired := normalizeSchedulerJob(schedulerJob{
		Slug:            taskGitHubAuditJobSlug,
		Kind:            taskGitHubAuditJobKind,
		Label:           "GitHub publication audit",
		TargetType:      taskGitHubAuditTargetType,
		TargetID:        "global",
		Channel:         "general",
		IntervalMinutes: int(taskGitHubAuditInterval / time.Minute),
		NextRun:         nextRun,
		DueAt:           nextRun,
		Status:          "scheduled",
	})
	changed := false
	for i := range b.scheduler {
		if strings.TrimSpace(b.scheduler[i].Slug) != taskGitHubAuditJobSlug {
			continue
		}
		current := normalizeSchedulerJob(b.scheduler[i])
		if current.Kind != desired.Kind || current.Label != desired.Label || current.TargetType != desired.TargetType ||
			current.TargetID != desired.TargetID || current.Channel != desired.Channel || current.IntervalMinutes != desired.IntervalMinutes {
			current.Kind = desired.Kind
			current.Label = desired.Label
			current.TargetType = desired.TargetType
			current.TargetID = desired.TargetID
			current.Channel = desired.Channel
			current.IntervalMinutes = desired.IntervalMinutes
			changed = true
		}
		if current.Status == "" || strings.EqualFold(current.Status, "done") || strings.EqualFold(current.Status, "canceled") {
			current.Status = "scheduled"
			current.NextRun = desired.NextRun
			current.DueAt = desired.DueAt
			changed = true
		}
		if changed {
			b.scheduler[i] = current
			return b.saveLocked()
		}
		return nil
	}
	if err := b.scheduleJobLocked(desired); err != nil {
		return err
	}
	return b.saveLocked()
}

func (b *Broker) ReconcileTaskGitHubPublications(now time.Time) (taskGitHubAuditState, error) {
	if b == nil {
		return taskGitHubAuditState{}, nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	candidates := b.collectTaskGitHubAuditCandidates()
	if len(candidates) == 0 {
		state := b.updateTaskGitHubAuditState(now, "", "")
		return state, nil
	}
	if err := ensureTaskGitHubCLI(); err != nil {
		state := b.updateTaskGitHubAuditState(now, "", err.Error())
		return state, err
	}
	var firstErr error
	for _, candidate := range candidates {
		switch candidate.status {
		case "pending", "deferred":
			if candidate.status == "deferred" {
				if _, _, err := b.retryTaskGitHubPublication(candidate.taskID, "scheduler", candidate.kind); err != nil && firstErr == nil {
					firstErr = err
					continue
				}
			}
			switch candidate.kind {
			case "issue":
				_, _, err := b.publishTaskIssueNow(candidate.taskID)
				if err != nil && firstErr == nil {
					firstErr = err
				}
			case "pr":
				_, _, err := b.publishTaskPRNow(candidate.taskID)
				if err != nil && firstErr == nil {
					firstErr = err
				}
			}
		case "opened":
			if _, _, err := b.refreshTaskGitHubPublicationNow(candidate.taskID, candidate.kind); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	lastSuccess := ""
	lastError := ""
	if firstErr == nil {
		lastSuccess = now.Format(time.RFC3339)
	} else {
		lastError = firstErr.Error()
	}
	state := b.updateTaskGitHubAuditState(now, lastSuccess, lastError)
	return state, firstErr
}

type taskGitHubAuditCandidate struct {
	taskID  string
	kind    string
	status  string
	channel string
}

func (b *Broker) collectTaskGitHubAuditCandidates() []taskGitHubAuditCandidate {
	b.mu.Lock()
	defer b.mu.Unlock()

	candidates := make([]taskGitHubAuditCandidate, 0, len(b.tasks)*2)
	for _, task := range b.tasks {
		if !taskGitHubShouldSync(&task, task.RepoContext) {
			continue
		}
		for _, kind := range []string{"issue", "pr"} {
			target := taskGitHubPublicationForKind(&task, kind)
			if target == nil {
				continue
			}
			status := normalizeTaskGitHubPublicationStatus(target.Status)
			if status != "pending" && status != "deferred" && status != "opened" {
				continue
			}
			candidates = append(candidates, taskGitHubAuditCandidate{
				taskID:  strings.TrimSpace(task.ID),
				kind:    kind,
				status:  status,
				channel: normalizeChannelSlug(task.Channel),
			})
		}
	}
	return candidates
}

func (b *Broker) updateTaskGitHubAuditState(now time.Time, lastSuccess, lastError string) taskGitHubAuditState {
	b.mu.Lock()
	defer b.mu.Unlock()

	pending := 0
	deferred := 0
	opened := 0
	for _, task := range b.tasks {
		for _, target := range []*taskGitHubPublication{task.IssuePublication, task.PRPublication} {
			if target == nil {
				continue
			}
			switch normalizeTaskGitHubPublicationStatus(target.Status) {
			case "pending":
				pending++
			case "deferred":
				deferred++
			case "opened":
				opened++
			}
		}
	}
	b.gitHubPublicationAudit.LastRunAt = now.UTC().Format(time.RFC3339)
	if strings.TrimSpace(lastSuccess) != "" {
		b.gitHubPublicationAudit.LastSuccessAt = strings.TrimSpace(lastSuccess)
		b.gitHubPublicationAudit.LastError = ""
	} else if strings.TrimSpace(lastError) != "" {
		b.gitHubPublicationAudit.LastError = strings.TrimSpace(lastError)
	}
	b.gitHubPublicationAudit.PendingCount = pending
	b.gitHubPublicationAudit.DeferredCount = deferred
	b.gitHubPublicationAudit.OpenedCount = opened
	return b.gitHubPublicationAudit
}

func (b *Broker) bumpTaskGitHubAuditCounterLocked(action string) {
	switch strings.TrimSpace(action) {
	case "opened", "adopted":
		b.gitHubPublicationAudit.OpenedTotal++
	case "updated":
		b.gitHubPublicationAudit.UpdatedTotal++
	case "closed":
		b.gitHubPublicationAudit.ClosedTotal++
	case "reopened":
		b.gitHubPublicationAudit.ReopenedTotal++
	case "synced":
		b.gitHubPublicationAudit.SyncedTotal++
	case "deferred":
		b.gitHubPublicationAudit.DeferredTotal++
	}
}

func (b *Broker) snapshotTaskForGitHubPublication(taskID, kind string) (*teamTask, error) {
	return b.snapshotTaskForGitHubStatus(taskID, kind, "pending")
}

func (b *Broker) snapshotTaskForGitHubSync(taskID, kind string) (*teamTask, error) {
	return b.snapshotTaskForGitHubStatus(taskID, kind, "opened")
}

func (b *Broker) snapshotTaskForGitHubStatus(taskID, kind string, allowed ...string) (*teamTask, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	taskID = strings.TrimSpace(taskID)
	for i := range b.tasks {
		task := &b.tasks[i]
		if task.ID != taskID {
			continue
		}
		target := taskGitHubPublicationForKind(task, kind)
		if target == nil {
			return nil, fmt.Errorf("%s publication is not tracked for task %s", kind, task.ID)
		}
		status := normalizeTaskGitHubPublicationStatus(target.Status)
		allowedStatus := false
		for _, candidate := range allowed {
			if status == strings.TrimSpace(candidate) {
				allowedStatus = true
				break
			}
		}
		if !allowedStatus {
			return nil, fmt.Errorf("%s publication is not %s for task %s", kind, strings.Join(allowed, " or "), task.ID)
		}
		copyTask := *task
		return &copyTask, nil
	}
	return nil, fmt.Errorf("task not found")
}

func (b *Broker) snapshotTaskForGitHubRecord(taskID string) (*teamTask, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	taskID = strings.TrimSpace(taskID)
	for i := range b.tasks {
		if b.tasks[i].ID != taskID {
			continue
		}
		copyTask := b.tasks[i]
		return &copyTask, nil
	}
	return nil, fmt.Errorf("task not found")
}

func (b *Broker) refreshTaskGitHubPublicationNow(taskID, kind string) (teamTask, bool, error) {
	return withTaskGitHubPublicationLock(taskID, kind, func() (teamTask, bool, error) {
		return b.refreshTaskGitHubPublicationNowLocked(taskID, kind)
	})
}

func (b *Broker) refreshTaskGitHubPublicationNowLocked(taskID, kind string) (teamTask, bool, error) {
	task, err := b.snapshotTaskForGitHubSync(taskID, kind)
	if err != nil {
		return teamTask{}, false, err
	}
	repoCtx, err := b.resolveTaskGitHubRepoContext(task)
	if err != nil {
		return b.deferTaskGitHubPublication(taskID, kind, nil, "", err)
	}
	repoCtx, err = enrichTaskGitHubRepoContext(repoCtx)
	if err != nil {
		return b.deferTaskGitHubPublication(taskID, kind, repoCtx, "", err)
	}
	if !taskGitHubPublicationApproved(task, repoCtx) {
		return b.deferTaskGitHubPublication(taskID, kind, repoCtx, "", fmt.Errorf("public_repo_requires_approval"))
	}
	target := taskGitHubPublicationForKind(task, kind)
	if target == nil {
		return teamTask{}, false, fmt.Errorf("%s publication is not tracked for task %s", kind, task.ID)
	}
	if target.Number <= 0 {
		var found *taskGitHubAPIResponse
		switch kind {
		case "issue":
			found, err = findTaskGitHubIssueByMarker(repoCtx, task.ID)
		case "pr":
			found, err = findTaskGitHubPRByMarker(repoCtx, task.ID)
		}
		if err != nil {
			return b.deferTaskGitHubPublication(taskID, kind, repoCtx, strings.TrimSpace(target.HeadBranch), err)
		}
		if found == nil {
			return b.deferTaskGitHubPublication(taskID, kind, repoCtx, strings.TrimSpace(target.HeadBranch), fmt.Errorf("github %s not found for task %s", kind, task.ID))
		}
		artifactTitle := buildTaskGitHubIssueTitle(task)
		if strings.EqualFold(kind, "pr") {
			artifactTitle = buildTaskGitHubPRTitle(task)
		}
		return b.syncTaskGitHubPublication(taskID, kind, repoCtx, found, strings.TrimSpace(target.HeadBranch), firstNonEmpty(strings.TrimSpace(target.BaseBranch), repoCtx.BaseBranch), artifactTitle)
	}
	var response *taskGitHubAPIResponse
	switch kind {
	case "issue":
		response, err = refreshTaskGitHubIssue(repoCtx, target.Number)
	case "pr":
		response, err = refreshTaskGitHubPR(repoCtx, target.Number)
	default:
		err = fmt.Errorf("unsupported github publication kind %q", kind)
	}
	if err != nil {
		return b.deferTaskGitHubPublication(taskID, kind, repoCtx, strings.TrimSpace(target.HeadBranch), err)
	}
	switch kind {
	case "issue":
		return b.applyTaskGitHubIssueDesiredState(taskID, repoCtx, response, strings.TrimSpace(target.HeadBranch), firstNonEmpty(strings.TrimSpace(target.BaseBranch), repoCtx.BaseBranch))
	case "pr":
		issueResponse, issueErr := refreshTaskGitHubIssue(repoCtx, target.Number)
		if issueErr != nil {
			return b.deferTaskGitHubPublication(taskID, kind, repoCtx, strings.TrimSpace(target.HeadBranch), issueErr)
		}
		return b.applyTaskGitHubPRDesiredState(taskID, repoCtx, issueResponse, response, strings.TrimSpace(target.HeadBranch), firstNonEmpty(strings.TrimSpace(target.BaseBranch), repoCtx.BaseBranch))
	default:
		artifactTitle := buildTaskGitHubIssueTitle(task)
		if strings.EqualFold(kind, "pr") {
			artifactTitle = buildTaskGitHubPRTitle(task)
		}
		return b.syncTaskGitHubPublication(taskID, kind, repoCtx, response, strings.TrimSpace(target.HeadBranch), firstNonEmpty(strings.TrimSpace(target.BaseBranch), repoCtx.BaseBranch), artifactTitle)
	}
}

func (b *Broker) applyTaskGitHubIssueDesiredState(taskID string, repoCtx *taskGitHubRepoContext, remote *taskGitHubAPIResponse, headBranch, baseBranch string) (teamTask, bool, error) {
	task, err := b.snapshotTaskForGitHubRecord(taskID)
	if err != nil {
		return teamTask{}, false, err
	}
	desired := taskGitHubDesiredIssueState(task, repoCtx)
	contentChanged := false
	if taskGitHubArtifactNeedsContentSync(remote, desired) {
		if err := taskGitHubPublicRedactionError(repoCtx, desired); err != nil {
			return b.deferTaskGitHubPublication(taskID, "issue", repoCtx, headBranch, err)
		}
		if err := updateTaskGitHubIssue(repoCtx, remote.Number, remote, desired); err != nil {
			return b.deferTaskGitHubPublication(taskID, "issue", repoCtx, headBranch, err)
		}
		remote, err = refreshTaskGitHubIssue(repoCtx, remote.Number)
		if err != nil {
			return b.deferTaskGitHubPublication(taskID, "issue", repoCtx, headBranch, err)
		}
		contentChanged = true
	}
	remoteState := taskGitHubRemoteState(remote, "issue")
	action := "synced"
	if desired.State == "closed" && remoteState != "closed" {
		if err := closeTaskGitHubIssue(repoCtx, remote.Number, strings.EqualFold(strings.TrimSpace(task.Status), "canceled")); err != nil {
			return b.deferTaskGitHubPublication(taskID, "issue", repoCtx, headBranch, err)
		}
		remote, err = refreshTaskGitHubIssue(repoCtx, remote.Number)
		if err != nil {
			return b.deferTaskGitHubPublication(taskID, "issue", repoCtx, headBranch, err)
		}
		action = "closed"
	} else if desired.State == "open" && remoteState == "closed" {
		if err := reopenTaskGitHubIssue(repoCtx, remote.Number); err != nil {
			return b.deferTaskGitHubPublication(taskID, "issue", repoCtx, headBranch, err)
		}
		remote, err = refreshTaskGitHubIssue(repoCtx, remote.Number)
		if err != nil {
			return b.deferTaskGitHubPublication(taskID, "issue", repoCtx, headBranch, err)
		}
		action = "reopened"
	} else if contentChanged {
		action = "updated"
	}
	return b.storeTaskGitHubPublication(taskID, "issue", repoCtx, remote, headBranch, baseBranch, desired.Title, action)
}

func (b *Broker) applyTaskGitHubPRDesiredState(taskID string, repoCtx *taskGitHubRepoContext, remoteIssue, remotePR *taskGitHubAPIResponse, headBranch, baseBranch string) (teamTask, bool, error) {
	task, err := b.snapshotTaskForGitHubRecord(taskID)
	if err != nil {
		return teamTask{}, false, err
	}
	desired := taskGitHubDesiredPRState(task, repoCtx)
	remoteState := taskGitHubRemoteState(remotePR, "pr")
	contentChanged := false
	if remoteState != "merged" && taskGitHubArtifactNeedsContentSync(remoteIssue, desired) {
		if err := taskGitHubPublicRedactionError(repoCtx, desired); err != nil {
			return b.deferTaskGitHubPublication(taskID, "pr", repoCtx, headBranch, err)
		}
		if err := updateTaskGitHubPR(repoCtx, remotePR.Number, remoteIssue, desired); err != nil {
			return b.deferTaskGitHubPublication(taskID, "pr", repoCtx, headBranch, err)
		}
		remoteIssue, err = refreshTaskGitHubIssue(repoCtx, remotePR.Number)
		if err != nil {
			return b.deferTaskGitHubPublication(taskID, "pr", repoCtx, headBranch, err)
		}
		contentChanged = true
	}
	action := "synced"
	if remoteState != "merged" && desired.State == "closed" && remoteState != "closed" {
		if err := closeTaskGitHubPR(repoCtx, remotePR.Number); err != nil {
			return b.deferTaskGitHubPublication(taskID, "pr", repoCtx, headBranch, err)
		}
		action = "closed"
	} else if remoteState != "merged" && desired.State == "open" && remoteState == "closed" {
		if err := reopenTaskGitHubPR(repoCtx, remotePR.Number); err != nil {
			return b.deferTaskGitHubPublication(taskID, "pr", repoCtx, headBranch, err)
		}
		action = "reopened"
	}
	if remoteState != "merged" && remotePR.Draft != desired.Draft {
		if desired.Draft {
			if err := convertTaskGitHubPRToDraft(repoCtx, remotePR.Number); err != nil {
				return b.deferTaskGitHubPublication(taskID, "pr", repoCtx, headBranch, err)
			}
		} else {
			if err := markTaskGitHubPRReady(repoCtx, remotePR.Number); err != nil {
				return b.deferTaskGitHubPublication(taskID, "pr", repoCtx, headBranch, err)
			}
		}
		if action == "synced" {
			action = "updated"
		}
	}
	remotePR, err = refreshTaskGitHubPR(repoCtx, remotePR.Number)
	if err != nil {
		return b.deferTaskGitHubPublication(taskID, "pr", repoCtx, headBranch, err)
	}
	remoteIssue, err = refreshTaskGitHubIssue(repoCtx, remotePR.Number)
	if err != nil {
		return b.deferTaskGitHubPublication(taskID, "pr", repoCtx, headBranch, err)
	}
	if action == "synced" && contentChanged {
		action = "updated"
	}
	mergedIssue := *remotePR
	if strings.TrimSpace(remoteIssue.Title) != "" {
		mergedIssue.Title = remoteIssue.Title
	}
	if strings.TrimSpace(remoteIssue.Body) != "" {
		mergedIssue.Body = remoteIssue.Body
	}
	if len(remoteIssue.Labels) > 0 {
		mergedIssue.Labels = append([]taskGitHubLabel(nil), remoteIssue.Labels...)
	}
	updatedTask, changed, err := b.storeTaskGitHubPublication(taskID, "pr", repoCtx, &mergedIssue, headBranch, baseBranch, desired.Title, action)
	if err != nil {
		return teamTask{}, false, err
	}
	if snapshot, snapErr := refreshTaskGitHubPRReviewSnapshot(repoCtx, remotePR.Number); snapErr == nil && snapshot != nil {
		reviewTask, reviewChanged, reviewErr := b.storeTaskGitHubPRReviewSnapshot(taskID, snapshot)
		if reviewErr == nil {
			if taskIsTerminal(task) && taskGitHubRemoteState(remotePR, "pr") == "merged" {
				_ = taskGitHubCleanupMergedRemoteBranch(repoCtx, headBranch, baseBranch)
			}
			if reviewChanged {
				return reviewTask, true, nil
			}
			if changed {
				return reviewTask, true, nil
			}
			return reviewTask, false, nil
		}
	}
	if taskIsTerminal(task) && taskGitHubRemoteState(remotePR, "pr") == "merged" {
		_ = taskGitHubCleanupMergedRemoteBranch(repoCtx, headBranch, baseBranch)
	}
	return updatedTask, changed, nil
}

func taskGitHubRemoteState(response *taskGitHubAPIResponse, kind string) string {
	if response == nil {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(kind), "pr") {
		mergedAt := strings.TrimSpace(response.MergedAt)
		if mergedAt == "" && response.PullRequest != nil {
			mergedAt = strings.TrimSpace(response.PullRequest.MergedAt)
		}
		if mergedAt != "" {
			return "merged"
		}
	}
	return strings.ToLower(strings.TrimSpace(response.State))
}

func taskGitHubCleanupMergedRemoteBranch(repoCtx *taskGitHubRepoContext, headBranch, baseBranch string) error {
	if repoCtx == nil {
		return nil
	}
	headBranch = strings.TrimSpace(headBranch)
	baseBranch = strings.TrimSpace(firstNonEmpty(baseBranch, repoCtx.BaseBranch))
	if headBranch == "" || headBranch == baseBranch {
		return nil
	}
	exists, err := taskGitHubRemoteBranchExists(repoCtx.RootPath, headBranch)
	if err != nil || !exists {
		return err
	}
	return taskGitHubRunGit(repoCtx.RootPath, "push", "origin", "--delete", headBranch)
}

func taskGitHubRemoteBranchExists(repoRoot, branch string) (bool, error) {
	out, err := taskGitHubGitOutput(repoRoot, "ls-remote", "--heads", "origin", strings.TrimSpace(branch))
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func (b *Broker) completeTaskGitHubPublication(taskID, kind string, repoCtx *taskGitHubRepoContext, response *taskGitHubAPIResponse, headBranch, baseBranch, title string) (teamTask, bool, error) {
	return b.storeTaskGitHubPublication(taskID, kind, repoCtx, response, headBranch, baseBranch, title, "opened")
}

func (b *Broker) adoptTaskGitHubPublication(taskID, kind string, repoCtx *taskGitHubRepoContext, response *taskGitHubAPIResponse, headBranch, baseBranch, title string) (teamTask, bool, error) {
	return b.storeTaskGitHubPublication(taskID, kind, repoCtx, response, headBranch, baseBranch, title, "adopted")
}

func (b *Broker) syncTaskGitHubPublication(taskID, kind string, repoCtx *taskGitHubRepoContext, response *taskGitHubAPIResponse, headBranch, baseBranch, title string) (teamTask, bool, error) {
	return b.storeTaskGitHubPublication(taskID, kind, repoCtx, response, headBranch, baseBranch, title, "synced")
}

func (b *Broker) storeTaskGitHubPublication(taskID, kind string, repoCtx *taskGitHubRepoContext, response *taskGitHubAPIResponse, headBranch, baseBranch, title, action string) (teamTask, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if response == nil {
		return teamTask{}, false, fmt.Errorf("github response required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for i := range b.tasks {
		task := &b.tasks[i]
		if task.ID != strings.TrimSpace(taskID) {
			continue
		}
		target := taskGitHubPublicationForKind(task, kind)
		if target == nil {
			target = &taskGitHubPublication{}
			setTaskGitHubPublicationForKind(task, kind, target)
		}
		task.RepoContext = repoCtx
		target.Status = "opened"
		target.Number = response.Number
		target.URL = strings.TrimSpace(response.HTMLURL)
		target.Title = strings.TrimSpace(title)
		target.HeadBranch = strings.TrimSpace(headBranch)
		target.BaseBranch = strings.TrimSpace(baseBranch)
		target.ExternalID = strings.TrimSpace(response.NodeID)
		target.State = taskGitHubRemoteState(response, kind)
		target.LastAttemptAt = now
		target.LastSyncedAt = now
		target.LastError = ""
		target.RetryCount++
		target.Draft = response.Draft
		target.MergedAt = strings.TrimSpace(firstNonEmpty(response.MergedAt, func() string {
			if response.PullRequest != nil {
				return response.PullRequest.MergedAt
			}
			return ""
		}()))
		switch strings.ToLower(strings.TrimSpace(kind)) {
		case "issue":
			target.DesiredSignature = taskGitHubDesiredIssueState(task, repoCtx).Signature
		case "pr":
			target.DesiredSignature = taskGitHubDesiredPRState(task, repoCtx).Signature
		}
		task.UpdatedAt = now
		if strings.TrimSpace(action) == "" {
			action = "opened"
		}
		b.bumpTaskGitHubAuditCounterLocked(action)
		b.appendActionLocked("github_"+kind+"_"+action, "gh", normalizeChannelSlug(task.Channel), firstNonEmpty(strings.TrimSpace(task.Owner), strings.TrimSpace(task.CreatedBy), "system"), truncateSummary(task.Title, 140), task.ID)
		if err := b.saveLocked(); err != nil {
			return teamTask{}, false, err
		}
		return *task, true, nil
	}
	return teamTask{}, false, fmt.Errorf("task not found")
}

func (b *Broker) storeTaskGitHubPRReviewSnapshot(taskID string, snapshot *taskGitHubRemoteReviewSnapshot) (teamTask, bool, error) {
	if snapshot == nil {
		return teamTask{}, false, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	for i := range b.tasks {
		task := &b.tasks[i]
		if task.ID != strings.TrimSpace(taskID) {
			continue
		}
		if task.PRPublication == nil {
			task.PRPublication = &taskGitHubPublication{}
		}
		target := task.PRPublication
		changed := false
		reviewDecision := normalizeTaskGitHubReviewDecision(snapshot.ReviewDecision)
		checksState := normalizeTaskGitHubChecksState(snapshot.ChecksState)
		checksSummary := compactStringList(snapshot.ChecksSummary)
		commentCount := snapshot.CommentCount
		latestCommentAt := strings.TrimSpace(snapshot.LatestCommentAt)
		latestCommentAuthor := strings.TrimSpace(snapshot.LatestCommentAuthor)
		latestCommentSnippet := normalizeTaskGitHubCommentSnippet(snapshot.LatestCommentSnippet)
		if target.ReviewDecision != reviewDecision {
			target.ReviewDecision = reviewDecision
			changed = true
		}
		if target.ChecksState != checksState {
			target.ChecksState = checksState
			changed = true
		}
		if strings.Join(target.ChecksSummary, "\n") != strings.Join(checksSummary, "\n") {
			target.ChecksSummary = checksSummary
			changed = true
		}
		if target.CommentCount != commentCount {
			target.CommentCount = commentCount
			changed = true
		}
		if target.LatestCommentAt != latestCommentAt {
			target.LatestCommentAt = latestCommentAt
			changed = true
		}
		if target.LatestCommentAuthor != latestCommentAuthor {
			target.LatestCommentAuthor = latestCommentAuthor
			changed = true
		}
		if target.LatestCommentSnippet != latestCommentSnippet {
			target.LatestCommentSnippet = latestCommentSnippet
			changed = true
		}
		if !taskIsTerminal(task) {
			nextStatus := ""
			nextReview := ""
			switch {
			case reviewDecision == "changes_requested" || checksState == "failing":
				nextStatus = "review"
				nextReview = "changes_requested"
			case reviewDecision == "approved" && !taskHasBlockingReviewFindings(task):
				nextStatus = "review"
				nextReview = "approved"
			}
			if nextStatus != "" && task.Status != nextStatus {
				task.Status = nextStatus
				changed = true
			}
			if nextReview != "" && task.ReviewState != nextReview {
				task.ReviewState = nextReview
				changed = true
			}
		}
		if !changed {
			return *task, false, nil
		}
		target.LastSyncedAt = now
		task.UpdatedAt = now
		b.appendActionLocked("github_pr_review_synced", "gh", normalizeChannelSlug(task.Channel), firstNonEmpty(strings.TrimSpace(task.Owner), strings.TrimSpace(task.CreatedBy), "system"), truncateSummary(task.Title, 140), task.ID)
		if err := b.saveLocked(); err != nil {
			return teamTask{}, false, err
		}
		return *task, true, nil
	}
	return teamTask{}, false, fmt.Errorf("task not found")
}

func (b *Broker) deferTaskGitHubPublication(taskID, kind string, repoCtx *taskGitHubRepoContext, headBranch string, cause error) (teamTask, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	for i := range b.tasks {
		task := &b.tasks[i]
		if task.ID != strings.TrimSpace(taskID) {
			continue
		}
		target := taskGitHubPublicationForKind(task, kind)
		if target == nil {
			target = &taskGitHubPublication{}
			setTaskGitHubPublicationForKind(task, kind, target)
		}
		if repoCtx != nil {
			task.RepoContext = repoCtx
			if target.BaseBranch == "" {
				target.BaseBranch = strings.TrimSpace(repoCtx.BaseBranch)
			}
		}
		target.Status = "deferred"
		target.HeadBranch = firstNonEmpty(strings.TrimSpace(headBranch), target.HeadBranch)
		target.LastAttemptAt = now
		errorText := "github publication deferred"
		if cause != nil {
			errorText = cause.Error()
		}
		target.LastError = truncateSummary(strings.TrimSpace(errorText), 500)
		target.RetryCount++
		task.UpdatedAt = now
		b.bumpTaskGitHubAuditCounterLocked("deferred")
		b.appendActionLocked("github_"+kind+"_deferred", "gh", normalizeChannelSlug(task.Channel), firstNonEmpty(strings.TrimSpace(task.Owner), strings.TrimSpace(task.CreatedBy), "system"), truncateSummary(target.LastError, 140), task.ID)
		if err := b.saveLocked(); err != nil {
			return teamTask{}, false, err
		}
		return *task, true, nil
	}
	return teamTask{}, false, fmt.Errorf("task not found")
}

func taskGitHubPublicationForKind(task *teamTask, kind string) *taskGitHubPublication {
	if task == nil {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "issue":
		return task.IssuePublication
	case "pr":
		return task.PRPublication
	default:
		return nil
	}
}

func setTaskGitHubPublicationForKind(task *teamTask, kind string, target *taskGitHubPublication) {
	if task == nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "issue":
		task.IssuePublication = target
	case "pr":
		task.PRPublication = target
	}
}

func findTaskGitHubIssueByMarker(repoCtx *taskGitHubRepoContext, taskID string) (*taskGitHubAPIResponse, error) {
	return findTaskGitHubArtifactByMarker(repoCtx, taskID, "issue")
}

func findTaskGitHubPRByMarker(repoCtx *taskGitHubRepoContext, taskID string) (*taskGitHubAPIResponse, error) {
	return findTaskGitHubArtifactByMarker(repoCtx, taskID, "pr")
}

func findTaskGitHubArtifactByMarker(repoCtx *taskGitHubRepoContext, taskID, kind string) (*taskGitHubAPIResponse, error) {
	if repoCtx == nil {
		return nil, fmt.Errorf("repo context required")
	}
	query := fmt.Sprintf("repo:%s/%s type:%s %q", repoCtx.Owner, repoCtx.Repo, strings.TrimSpace(kind), taskGitHubMarker(taskID))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := taskGitHubRunCommand(ctx, repoCtx.RootPath, "gh",
		"api",
		"search/issues",
		"-f", "q="+query,
		"-f", "per_page=1",
	)
	if err != nil {
		return nil, err
	}
	var response taskGitHubSearchResponse
	if err := json.Unmarshal(out, &response); err != nil {
		return nil, err
	}
	if len(response.Items) == 0 {
		return nil, nil
	}
	return &response.Items[0], nil
}

func refreshTaskGitHubIssue(repoCtx *taskGitHubRepoContext, number int) (*taskGitHubAPIResponse, error) {
	if repoCtx == nil {
		return nil, fmt.Errorf("repo context required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := taskGitHubRunCommand(ctx, repoCtx.RootPath, "gh",
		"api",
		fmt.Sprintf("repos/%s/%s/issues/%d", repoCtx.Owner, repoCtx.Repo, number),
	)
	if err != nil {
		return nil, err
	}
	return decodeTaskGitHubAPIResponse(out)
}

func refreshTaskGitHubPR(repoCtx *taskGitHubRepoContext, number int) (*taskGitHubAPIResponse, error) {
	if repoCtx == nil {
		return nil, fmt.Errorf("repo context required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := taskGitHubRunCommand(ctx, repoCtx.RootPath, "gh",
		"api",
		fmt.Sprintf("repos/%s/%s/pulls/%d", repoCtx.Owner, repoCtx.Repo, number),
	)
	if err != nil {
		return nil, err
	}
	return decodeTaskGitHubAPIResponse(out)
}

func refreshTaskGitHubPRReviewSnapshot(repoCtx *taskGitHubRepoContext, number int) (*taskGitHubRemoteReviewSnapshot, error) {
	if repoCtx == nil {
		return nil, fmt.Errorf("repo context required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := taskGitHubRunCommand(
		ctx,
		repoCtx.RootPath,
		"gh",
		"pr",
		"view",
		strconv.Itoa(number),
		"-R",
		repoCtx.Owner+"/"+repoCtx.Repo,
		"--json",
		"reviewDecision,statusCheckRollup,reviews,comments",
	)
	if err != nil {
		return nil, err
	}
	var response taskGitHubPRViewResponse
	if err := json.Unmarshal(out, &response); err != nil {
		return nil, err
	}
	reviewDecision := normalizeTaskGitHubReviewDecision(response.ReviewDecision)
	if reviewDecision == "" {
		for _, review := range response.Reviews {
			switch strings.ToLower(strings.TrimSpace(review.State)) {
			case "changes_requested":
				reviewDecision = "changes_requested"
			case "approved":
				if reviewDecision == "" {
					reviewDecision = "approved"
				}
			}
			if reviewDecision == "changes_requested" {
				break
			}
		}
	}
	checksState, checksSummary := taskGitHubClassifyStatusCheckRollup(response.StatusCheckRollup)
	commentCount, latestCommentAt, latestCommentAuthor, latestCommentSnippet := taskGitHubSummarizePRComments(response.Comments)
	return &taskGitHubRemoteReviewSnapshot{
		ReviewDecision:       reviewDecision,
		ChecksState:          checksState,
		ChecksSummary:        checksSummary,
		CommentCount:         commentCount,
		LatestCommentAt:      latestCommentAt,
		LatestCommentAuthor:  latestCommentAuthor,
		LatestCommentSnippet: latestCommentSnippet,
	}, nil
}

func taskGitHubClassifyStatusCheckRollup(items []map[string]any) (string, []string) {
	if len(items) == 0 {
		return "", nil
	}
	state := ""
	summary := make([]string, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(firstNonEmpty(anyToString(item["name"]), anyToString(item["context"]), anyToString(item["workflow"])))
		conclusion := strings.ToLower(strings.TrimSpace(firstNonEmpty(anyToString(item["conclusion"]), anyToString(item["status"]), anyToString(item["state"]))))
		if name == "" {
			name = "check"
		}
		switch conclusion {
		case "success", "succeeded", "completed", "neutral", "skipped":
			if state == "" {
				state = "passing"
			}
		case "pending", "queued", "in_progress", "requested", "waiting", "expected":
			if state != "failing" {
				state = "pending"
			}
			summary = append(summary, fmt.Sprintf("%s: %s", name, conclusion))
		case "failure", "failed", "timed_out", "action_required", "cancelled", "startup_failure":
			state = "failing"
			summary = append(summary, fmt.Sprintf("%s: %s", name, conclusion))
		default:
			if conclusion != "" {
				if state == "" {
					state = "pending"
				}
				summary = append(summary, fmt.Sprintf("%s: %s", name, conclusion))
			}
		}
	}
	if len(summary) > 6 {
		summary = summary[:6]
	}
	return normalizeTaskGitHubChecksState(state), summary
}

func taskGitHubSummarizePRComments(comments []taskGitHubPRViewComment) (int, string, string, string) {
	if len(comments) == 0 {
		return 0, "", "", ""
	}
	count := 0
	latestAt := ""
	latestAuthor := ""
	latestSnippet := ""
	latestTime := time.Time{}
	for _, comment := range comments {
		body := normalizeTaskGitHubCommentSnippet(comment.Body)
		if body == "" {
			continue
		}
		count++
		candidateAt := strings.TrimSpace(firstNonEmpty(comment.UpdatedAt, comment.CreatedAt))
		candidateTime := taskGitHubParseTimestamp(candidateAt)
		if latestAt == "" || (!candidateTime.IsZero() && candidateTime.After(latestTime)) {
			latestTime = candidateTime
			latestAt = candidateAt
			if comment.Author != nil {
				latestAuthor = strings.TrimSpace(comment.Author.Login)
			} else {
				latestAuthor = ""
			}
			latestSnippet = body
		}
	}
	return count, latestAt, latestAuthor, latestSnippet
}

func normalizeTaskGitHubCommentSnippet(text string) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if len(text) <= 180 {
		return text
	}
	return strings.TrimSpace(text[:177]) + "..."
}

func taskGitHubParseTimestamp(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func anyToString(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}

func ensureTaskGitHubCLI() error {
	if _, err := taskGitHubLookPath("gh"); err != nil {
		return fmt.Errorf("gh CLI not found in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := taskGitHubRunCommand(ctx, "", "gh", "auth", "status"); err != nil {
		return fmt.Errorf("gh auth unavailable: %w", err)
	}
	return nil
}

func enrichTaskGitHubRepoContext(repoCtx *taskGitHubRepoContext) (*taskGitHubRepoContext, error) {
	if repoCtx == nil {
		return nil, fmt.Errorf("repo context required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := taskGitHubRunCommand(ctx, repoCtx.RootPath, "gh",
		"api",
		fmt.Sprintf("repos/%s/%s", repoCtx.Owner, repoCtx.Repo),
	)
	if err != nil {
		return nil, err
	}
	var payload taskGitHubRepoPayload
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, err
	}
	copyCtx := *repoCtx
	copyCtx.Private = payload.Private
	copyCtx.Visibility = strings.TrimSpace(payload.Visibility)
	if strings.TrimSpace(payload.DefaultBranch) != "" {
		copyCtx.BaseBranch = strings.TrimSpace(payload.DefaultBranch)
	}
	return &copyCtx, nil
}

func (b *Broker) resolveTaskGitHubRepoContext(task *teamTask) (*taskGitHubRepoContext, error) {
	repoCtx, err := resolveTaskGitHubRepoContext(task)
	if err != nil {
		return nil, err
	}
	defaults := b.taskGitHubPublicationDefaultsForTask(task, repoCtx.RootPath)
	if defaults == nil && task != nil && task.RepoContext != nil {
		defaults = task.RepoContext.PublicationDefaults
	}
	if defaults != nil {
		repoCtx.PublicationDefaults = mergeTaskGitHubPublicationPolicy(repoCtx.PublicationDefaults, defaults)
	}
	return repoCtx, nil
}

func resolveTaskGitHubRepoContext(task *teamTask) (*taskGitHubRepoContext, error) {
	if task == nil {
		return nil, fmt.Errorf("task required")
	}
	repoRoot := strings.TrimSpace(effectiveTeamTaskWorkspacePath(task))
	if repoRoot == "" {
		return nil, fmt.Errorf("task %s does not expose a workspace or worktree path", task.ID)
	}
	out, err := taskGitHubGitOutput(repoRoot, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("resolve repo root: %w", err)
	}
	repoRoot = filepath.Clean(strings.TrimSpace(string(out)))
	remoteOut, err := taskGitHubGitOutput(repoRoot, "remote", "get-url", "origin")
	if err != nil {
		return nil, fmt.Errorf("resolve origin remote: %w", err)
	}
	remoteURL := strings.TrimSpace(string(remoteOut))
	owner, repo, err := parseTaskGitHubRemote(remoteURL)
	if err != nil {
		return nil, err
	}
	ctx := &taskGitHubRepoContext{
		RootPath:      repoRoot,
		RemoteURL:     remoteURL,
		Owner:         owner,
		Repo:          repo,
		BaseBranch:    resolveTaskGitHubBaseBranch(repoRoot),
		DefaultLabels: []string{"dunderia"},
		PublicationDefaults: &taskGitHubPublicationPolicy{
			Enabled:   "auto",
			IssueMode: "auto",
			PRMode:    "auto",
			SyncMode:  "auto",
		},
	}
	if task.RepoContext != nil {
		ctx.Visibility = strings.TrimSpace(task.RepoContext.Visibility)
		ctx.Private = task.RepoContext.Private
		if strings.TrimSpace(task.RepoContext.BaseBranch) != "" {
			ctx.BaseBranch = strings.TrimSpace(task.RepoContext.BaseBranch)
		}
	}
	return ctx, nil
}

func parseTaskGitHubRemote(remoteURL string) (string, string, error) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return "", "", fmt.Errorf("origin remote is empty")
	}
	normalized := strings.TrimSuffix(remoteURL, ".git")
	normalized = strings.TrimPrefix(normalized, "ssh://")
	normalized = strings.TrimPrefix(normalized, "git@")
	normalized = strings.TrimPrefix(normalized, "https://")
	normalized = strings.TrimPrefix(normalized, "http://")
	normalized = strings.TrimPrefix(normalized, "github.com:")
	normalized = strings.TrimPrefix(normalized, "github.com/")
	if strings.Contains(remoteURL, "github.com:") {
		normalized = strings.TrimPrefix(strings.TrimSuffix(remoteURL, ".git"), "git@github.com:")
	}
	if strings.Contains(remoteURL, "github.com/") {
		idx := strings.Index(remoteURL, "github.com/")
		normalized = strings.TrimPrefix(strings.TrimSuffix(remoteURL[idx:], ".git"), "github.com/")
	}
	parts := strings.Split(strings.TrimSpace(normalized), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("origin remote %q is not a supported GitHub remote", remoteURL)
	}
	owner := strings.TrimSpace(parts[len(parts)-2])
	repo := strings.TrimSpace(parts[len(parts)-1])
	if owner == "" || repo == "" {
		return "", "", fmt.Errorf("origin remote %q is not a supported GitHub remote", remoteURL)
	}
	return owner, repo, nil
}

func resolveTaskGitHubBaseBranch(repoRoot string) string {
	if out, err := taskGitHubGitOutput(repoRoot, "symbolic-ref", "refs/remotes/origin/HEAD"); err == nil {
		head := strings.TrimSpace(string(out))
		head = strings.TrimPrefix(head, "refs/remotes/origin/")
		if head != "" {
			return head
		}
	}
	if out, err := taskGitHubGitOutput(repoRoot, "rev-parse", "--abbrev-ref", "origin/HEAD"); err == nil {
		head := strings.TrimSpace(string(out))
		head = strings.TrimPrefix(head, "origin/")
		if head != "" && head != "HEAD" {
			return head
		}
	}
	return "main"
}

func resolveTaskGitHubHeadBranch(task *teamTask, repoRoot string) (string, error) {
	if task == nil {
		return "", fmt.Errorf("task required")
	}
	if branch := strings.TrimSpace(task.WorktreeBranch); branch != "" {
		return branch, nil
	}
	out, err := taskGitHubGitOutput(repoRoot, "branch", "--show-current")
	if err != nil {
		return "", fmt.Errorf("resolve current branch: %w", err)
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" {
		return "", fmt.Errorf("current branch is empty")
	}
	return branch, nil
}

func taskGitHubBranchHasCommitsAhead(repoRoot, baseBranch, headBranch string) (bool, error) {
	candidates := []string{
		fmt.Sprintf("origin/%s..%s", strings.TrimSpace(baseBranch), strings.TrimSpace(headBranch)),
		fmt.Sprintf("%s..%s", strings.TrimSpace(baseBranch), strings.TrimSpace(headBranch)),
	}
	var lastErr error
	for _, ref := range candidates {
		if strings.TrimSpace(ref) == "" {
			continue
		}
		out, err := taskGitHubGitOutput(repoRoot, "rev-list", "--count", ref)
		if err != nil {
			lastErr = err
			continue
		}
		count, convErr := strconv.Atoi(strings.TrimSpace(string(out)))
		if convErr != nil {
			return false, convErr
		}
		return count > 0, nil
	}
	if lastErr != nil {
		return false, lastErr
	}
	return false, fmt.Errorf("unable to compare %s against %s", headBranch, baseBranch)
}

func createTaskGitHubIssue(repoCtx *taskGitHubRepoContext, title, body string) (*taskGitHubAPIResponse, error) {
	if repoCtx == nil {
		return nil, fmt.Errorf("repo context required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := taskGitHubRunCommand(ctx, repoCtx.RootPath, "gh",
		"api",
		fmt.Sprintf("repos/%s/%s/issues", repoCtx.Owner, repoCtx.Repo),
		"--method", "POST",
		"-f", "title="+title,
		"-f", "body="+body,
	)
	if err != nil {
		return nil, err
	}
	return decodeTaskGitHubAPIResponse(out)
}

func createTaskGitHubPR(repoCtx *taskGitHubRepoContext, headBranch, title, body string, draft bool) (*taskGitHubAPIResponse, error) {
	if repoCtx == nil {
		return nil, fmt.Errorf("repo context required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := []string{
		"api",
		fmt.Sprintf("repos/%s/%s/pulls", repoCtx.Owner, repoCtx.Repo),
		"--method", "POST",
		"-f", "title=" + title,
		"-f", "body=" + body,
		"-f", "head=" + headBranch,
		"-f", "base=" + repoCtx.BaseBranch,
	}
	if draft {
		args = append(args, "-F", "draft=true")
	}
	out, err := taskGitHubRunCommand(ctx, repoCtx.RootPath, "gh", args...)
	if err != nil {
		return nil, err
	}
	return decodeTaskGitHubAPIResponse(out)
}

func updateTaskGitHubIssue(repoCtx *taskGitHubRepoContext, number int, current *taskGitHubAPIResponse, desired taskGitHubDesiredArtifactState) error {
	if repoCtx == nil {
		return fmt.Errorf("repo context required")
	}
	args := []string{"issue", "edit", strconv.Itoa(number), "-R", repoCtx.Owner + "/" + repoCtx.Repo, "--title", desired.Title, "--body", desired.Body}
	added, removed := taskGitHubLabelDelta(current, desired.Labels)
	if len(added) > 0 {
		args = append(args, "--add-label", strings.Join(added, ","))
	}
	if len(removed) > 0 {
		args = append(args, "--remove-label", strings.Join(removed, ","))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := taskGitHubRunCommand(ctx, repoCtx.RootPath, "gh", args...)
	return err
}

func updateTaskGitHubPR(repoCtx *taskGitHubRepoContext, number int, current *taskGitHubAPIResponse, desired taskGitHubDesiredArtifactState) error {
	if repoCtx == nil {
		return fmt.Errorf("repo context required")
	}
	args := []string{"pr", "edit", strconv.Itoa(number), "-R", repoCtx.Owner + "/" + repoCtx.Repo, "--title", desired.Title, "--body", desired.Body}
	added, removed := taskGitHubLabelDelta(current, desired.Labels)
	if len(added) > 0 {
		args = append(args, "--add-label", strings.Join(added, ","))
	}
	if len(removed) > 0 {
		args = append(args, "--remove-label", strings.Join(removed, ","))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := taskGitHubRunCommand(ctx, repoCtx.RootPath, "gh", args...)
	return err
}

func taskGitHubLabelDelta(current *taskGitHubAPIResponse, desired []string) ([]string, []string) {
	currentLabels := taskGitHubResponseLabels(current)
	currentSet := make(map[string]struct{}, len(currentLabels))
	for _, label := range currentLabels {
		currentSet[label] = struct{}{}
	}
	desiredSet := make(map[string]struct{}, len(desired))
	for _, label := range compactStringList(desired) {
		desiredSet[label] = struct{}{}
	}
	added := make([]string, 0)
	removed := make([]string, 0)
	for label := range desiredSet {
		if _, ok := currentSet[label]; !ok {
			added = append(added, label)
		}
	}
	for label := range currentSet {
		if _, ok := desiredSet[label]; !ok {
			removed = append(removed, label)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

func closeTaskGitHubIssue(repoCtx *taskGitHubRepoContext, number int, canceled bool) error {
	if repoCtx == nil {
		return fmt.Errorf("repo context required")
	}
	args := []string{"issue", "close", strconv.Itoa(number), "-R", repoCtx.Owner + "/" + repoCtx.Repo}
	if canceled {
		args = append(args, "--reason", "not planned")
	} else {
		args = append(args, "--reason", "completed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := taskGitHubRunCommand(ctx, repoCtx.RootPath, "gh", args...)
	return err
}

func reopenTaskGitHubIssue(repoCtx *taskGitHubRepoContext, number int) error {
	if repoCtx == nil {
		return fmt.Errorf("repo context required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := taskGitHubRunCommand(ctx, repoCtx.RootPath, "gh", "issue", "reopen", strconv.Itoa(number), "-R", repoCtx.Owner+"/"+repoCtx.Repo)
	return err
}

func closeTaskGitHubPR(repoCtx *taskGitHubRepoContext, number int) error {
	if repoCtx == nil {
		return fmt.Errorf("repo context required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := taskGitHubRunCommand(ctx, repoCtx.RootPath, "gh", "pr", "close", strconv.Itoa(number), "-R", repoCtx.Owner+"/"+repoCtx.Repo)
	return err
}

func reopenTaskGitHubPR(repoCtx *taskGitHubRepoContext, number int) error {
	if repoCtx == nil {
		return fmt.Errorf("repo context required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := taskGitHubRunCommand(ctx, repoCtx.RootPath, "gh", "pr", "reopen", strconv.Itoa(number), "-R", repoCtx.Owner+"/"+repoCtx.Repo)
	return err
}

func markTaskGitHubPRReady(repoCtx *taskGitHubRepoContext, number int) error {
	if repoCtx == nil {
		return fmt.Errorf("repo context required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := taskGitHubRunCommand(ctx, repoCtx.RootPath, "gh", "pr", "ready", strconv.Itoa(number), "-R", repoCtx.Owner+"/"+repoCtx.Repo)
	return err
}

func convertTaskGitHubPRToDraft(repoCtx *taskGitHubRepoContext, number int) error {
	if repoCtx == nil {
		return fmt.Errorf("repo context required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := taskGitHubRunCommand(ctx, repoCtx.RootPath, "gh", "pr", "ready", strconv.Itoa(number), "--undo", "-R", repoCtx.Owner+"/"+repoCtx.Repo)
	return err
}

func decodeTaskGitHubAPIResponse(raw []byte) (*taskGitHubAPIResponse, error) {
	var response taskGitHubAPIResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, err
	}
	if response.Number <= 0 || strings.TrimSpace(response.HTMLURL) == "" {
		return nil, fmt.Errorf("github response missing number or html_url")
	}
	return &response, nil
}
