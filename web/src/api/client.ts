/**
 * Typed WuphfAPI client.
 * Mirrors every method from the legacy IIFE in index.legacy.html.
 */

let apiBase = '/api'
let brokerDirect = 'http://localhost:7890'
let useProxy = true
let token: string | null = null

// ── Init ──

interface ApiTokenResponse {
  token?: string
  broker_url?: string
}

async function fetchInitToken(url: string): Promise<ApiTokenResponse> {
  const response = await fetch(url)
  if (!response.ok) {
    throw new Error(`${response.status} ${response.statusText}`.trim())
  }
  const data = (await response.json()) as ApiTokenResponse
  if (!data.token) {
    throw new Error(`Missing API token from ${url}`)
  }
  return data
}

export async function initApi(): Promise<void> {
  token = null
  try {
    const data = await fetchInitToken('/api-token')
    token = data.token ?? null
    if (data.broker_url) {
      brokerDirect = String(data.broker_url).replace(/\/+$/, '')
    }
    useProxy = true
  } catch {
    useProxy = false
    try {
      const data = await fetchInitToken(brokerDirect + '/web-token')
      token = data.token ?? null
    } catch (error) {
      token = null
      throw error instanceof Error ? error : new Error('Broker unavailable')
    }
  }
}

export async function connectToBroker(): Promise<void> {
  await initApi()
  await getHealth()
}

// ── Internal helpers ──

function baseURL(): string {
  return useProxy ? apiBase : brokerDirect
}

function authHeaders(): Record<string, string> {
  const h: Record<string, string> = { 'Content-Type': 'application/json' }
  if (!useProxy && token) h['Authorization'] = `Bearer ${token}`
  return h
}

export async function get<T = unknown>(
  path: string,
  params?: Record<string, string | number | boolean | null | undefined>,
): Promise<T> {
  let url = baseURL() + path
  if (params) {
    const qs = Object.entries(params)
      .filter(([, v]) => v != null)
      .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`)
      .join('&')
    if (qs) url += '?' + qs
  }
  const r = await fetch(url, { headers: authHeaders() })
  if (!r.ok) {
    const text = (await r.text().catch(() => '')).trim()
    throw new Error(text || `${r.status} ${r.statusText}`)
  }
  return r.json()
}

export async function post<T = unknown>(
  path: string,
  body?: unknown,
): Promise<T> {
  const r = await fetch(baseURL() + path, {
    method: 'POST',
    headers: authHeaders(),
    body: JSON.stringify(body),
  })
  if (!r.ok) {
    const text = (await r.text().catch(() => '')).trim()
    throw new Error(text || `${r.status} ${r.statusText}`)
  }
  return r.json()
}

export async function del<T = unknown>(
  path: string,
  body?: unknown,
): Promise<T> {
  const r = await fetch(baseURL() + path, {
    method: 'DELETE',
    headers: authHeaders(),
    body: JSON.stringify(body),
  })
  if (!r.ok) {
    const text = (await r.text().catch(() => '')).trim()
    throw new Error(text || `${r.status} ${r.statusText}`)
  }
  return r.json()
}

// ── SSE ──

export function sseURL(path: string): string {
  let url = baseURL() + path
  if (!useProxy && token) url += (url.includes('?') ? '&' : '?') + 'token=' + encodeURIComponent(token)
  return url
}

// ── Messages ──

export interface Message {
  id: string
  client_id?: string
  from: string
  channel: string
  can_delete?: boolean
  can_delete_thread?: boolean
  kind?: string
  source?: string
  source_label?: string
  event_id?: string
  title?: string
  content: string
  timestamp: string
  reply_to?: string
  thread_id?: string
  thread_count?: number
  reactions?: Record<string, string[]>
  tagged?: string[]
  usage?: TokenUsage
}

export interface ExecutionNode {
  id: string
  channel?: string
  root_message_id?: string
  parent_node_id?: string
  trigger_message_id?: string
  owner_agent?: string
  status?: string
  expected_response_kind?: string
  expected_from?: string[]
  timeout_at?: string
  resolved_by_message_id?: string
  resolved_by_agent?: string
  awaiting_human_input?: boolean
  awaiting_human_since?: string
  awaiting_human_reason?: string
  last_error?: string
  created_at?: string
  updated_at?: string
}

export interface TokenUsage {
  input_tokens?: number
  output_tokens?: number
  cache_read_tokens?: number
  cache_creation_tokens?: number
  total_tokens?: number
  cost_usd?: number
}

export interface GetMessagesOptions {
  sinceId?: string | null
  beforeId?: string | null
  limit?: number
  threadId?: string | null
}

export interface MessagesResponse {
  messages: Message[]
  execution_nodes?: ExecutionNode[]
  has_more?: boolean
}

export type BrokerEventType = 'ready' | 'message' | 'action' | 'activity' | 'office_changed'

export interface BrokerEventMessagePayload {
  message?: Message
  [key: string]: unknown
}

export interface BrokerEventActionPayload {
  action?: {
    channel?: string
    kind?: string
    id?: string
    related_id?: string
    actor?: string
    summary?: string
    [key: string]: unknown
  }
  [key: string]: unknown
}

export interface BrokerEventActivityPayload {
  activity?: {
    channel?: string
    id?: string
    kind?: string
    [key: string]: unknown
  }
  [key: string]: unknown
}

export interface BrokerOfficeChangedPayload {
  kind?: string
  slug?: string
  [key: string]: unknown
}

export interface MessageSearchHit {
  id: string
  channel: string
  from: string
  title?: string
  content: string
  timestamp: string
  reply_to?: string
  thread_id?: string
}

export interface MessageSearchResponse {
  query: string
  hits: MessageSearchHit[]
}

export interface MessageThreadSummary {
  thread_id: string
  channel: string
  reply_count: number
  last_reply_at?: string
  message: Message
}

export interface MessageThreadsResponse {
  threads: MessageThreadSummary[]
}

export interface PostMessageResponse {
  id: string
  total: number
  persisted: boolean
  duplicate?: boolean
  message?: Message
}

export interface DeleteMessageResponse {
  ok: boolean
  id: string
  channel: string
  thread_id?: string
  deleted_ids?: string[]
  deleted_count?: number
  total: number
}

export function getMessages(channel: string, options?: GetMessagesOptions) {
  return get<MessagesResponse>('/messages', {
    channel: channel || 'general',
    viewer_slug: 'human',
    since_id: options?.sinceId ?? null,
    before_id: options?.beforeId ?? null,
    limit: options?.limit ?? 50,
    thread_id: options?.threadId ?? null,
  })
}

export function postMessage(content: string, channel: string, replyTo?: string, tagged?: string[], clientId?: string) {
  const body: Record<string, string> = {
    from: 'you',
    channel: channel || 'general',
    content,
  }
  if (replyTo) body.reply_to = replyTo
  if (clientId) body.client_id = clientId
  if (tagged && tagged.length > 0) (body as Record<string, unknown>).tagged = tagged
  return post<PostMessageResponse>('/messages', body)
}

export function deleteMessage(messageId: string, channel: string, options?: { deleteThread?: boolean }) {
  return del<DeleteMessageResponse>('/messages', {
    id: messageId,
    channel: channel || 'general',
    delete_thread: options?.deleteThread === true,
  })
}

export function getThreadMessages(
  channel: string,
  threadId: string,
  options?: Omit<GetMessagesOptions, 'threadId'>,
) {
  return getMessages(channel, {
    threadId,
    sinceId: options?.sinceId ?? null,
    beforeId: options?.beforeId ?? null,
    limit: options?.limit ?? 50,
  })
}

export function searchMessages(query: string, options?: { limit?: number; channel?: string }) {
  return get<MessageSearchResponse>('/messages/search', {
    q: query,
    viewer_slug: 'human',
    limit: options?.limit ?? 8,
    channel: options?.channel ?? null,
  })
}

export function getMessageThreads(options?: { limit?: number; channel?: string }) {
  return get<MessageThreadsResponse>('/messages/threads', {
    viewer_slug: 'human',
    limit: options?.limit ?? 50,
    channel: options?.channel ?? null,
  })
}

export function clearChannel(channel: string) {
  return post<{ ok: boolean; channel: string; removed_messages?: number; removed_requests?: number; removed_execution_nodes?: number }>(
    '/channels/clear',
    { channel: channel || 'general' },
  )
}

export function toggleReaction(msgId: string, emoji: string, channel?: string) {
  return post('/reactions', {
    message_id: msgId,
    emoji,
    from: 'you',
  })
}

// ── Members ──

export type ProviderKind =
  | 'claude-code'
  | 'codex'
  | 'gemini'
  | 'ollama'

export type PerAgentProviderKind = ProviderKind
export type GlobalLLMProvider = ProviderKind

export interface ProviderBinding {
  kind?: ProviderKind
  model?: string
}

export interface OfficeMember {
  slug: string
  name: string
  role: string
  emoji?: string
  status?: string
  activity?: string
  detail?: string
  liveActivity?: string
  lastMessage?: string
  lastTime?: string
  totalMs?: number
  firstEventMs?: number
  firstTextMs?: number
  firstToolMs?: number
  task?: string
  channel?: string
  liveness_state?: string
  liveness_reason?: string
  liveness_task_id?: string
  liveness_at?: string
  provider?: ProviderBinding
}

export function getOfficeMembers() {
  return get<{ members: OfficeMember[] }>('/office-members')
}

export interface GeneratedAgentTemplate {
  slug?: string
  name?: string
  role?: string
  emoji?: string
  expertise?: string[]
  personality?: string
  provider?: PerAgentProviderKind
  model?: string
}

export function generateAgent(prompt: string) {
  return post<GeneratedAgentTemplate>('/office-members/generate', { prompt })
}

export function getMembers(channel: string) {
  return get<{ members: OfficeMember[] }>('/members', {
    channel: channel || 'general',
    viewer_slug: 'human',
  })
}

// ── Channels ──

export interface Channel {
  slug: string
  name: string
  description?: string
  type?: string
  created_by?: string
  members?: string[]
}

export interface CreateDMResponse {
  id?: string
  slug?: string
  name?: string
  type?: string
  channel?: {
    slug?: string
    name?: string
    type?: string
  }
}

export function getChannels() {
  return get<{ channels: Channel[] }>('/channels')
}

export function createChannel(slug: string, name: string, description: string) {
  return post('/channels', {
    action: 'create',
    slug,
    name: name || slug,
    description,
    created_by: 'you',
  })
}

export function generateChannel(prompt: string) {
  return post<Channel>('/channels/generate', { prompt })
}

export function createDM(agentSlug: string) {
  return post<CreateDMResponse>('/channels/dm', {
    members: ['human', agentSlug],
    type: 'direct',
  })
}

export function extractDMChannelSlug(result: CreateDMResponse | null | undefined, agentSlug: string): string {
  return result?.slug
    ?? result?.channel?.slug
    ?? `dm-${agentSlug}`
}

// ── Requests ──

export interface InterviewOption {
  id: string
  label: string
  description?: string
  requires_text?: boolean
  text_hint?: string
}

export interface AgentRequest {
  id: string
  from: string
  question: string
  /** Legacy field name; broker now returns `options`. Kept for compatibility. */
  choices?: InterviewOption[]
  options?: InterviewOption[]
  channel?: string
  title?: string
  context?: string
  kind?: string
  timestamp?: string
  status?: string
  blocking?: boolean
  required?: boolean
  recommended_id?: string
  created_at?: string
  updated_at?: string
  recommendation_status?: string
  recommendation_task_id?: string
  recommendation_requested_at?: string
  read_at?: string
  archived_at?: string
}

export function getRequests(channel: string, allChannels = false) {
  const params: Record<string, string> = {
    viewer_slug: 'human',
  }
  if (allChannels) {
    params.all_channels = 'true'
  } else {
    params.channel = channel || 'general'
  }
  return get<{ requests: AgentRequest[] }>('/requests', params)
}

export function answerRequest(id: string, choiceId: string, customText?: string) {
  const body: Record<string, string> = { id, choice_id: choiceId }
  if (customText) body.custom_text = customText
  return post('/requests/answer', body)
}

export function requestRecommendation(id: string, actor = 'human') {
  return post<{ request?: AgentRequest; task?: Task; prompt_message?: Message }>('/requests', {
    action: 'recommend',
    id,
    actor,
  })
}

// ── Health ──

export function getHealth() {
  return get<{ status: string; agents?: Record<string, unknown> }>('/health')
}

// ── Tasks ──

export interface Task {
  id: string
  title: string
  description?: string
  details?: string
  status: string
  owner?: string
  created_by?: string
  channel?: string
  thread_id?: string
  task_type?: string
  pipeline_id?: string
  pipeline_stage?: string
  execution_mode?: string
  review_state?: string
  outcome?: string
  outcome_status?: string
  outcome_evidence?: string
  outcome_verified_at?: string
  completion_evidence_required?: boolean
  completion_evidence_satisfied?: boolean
  completion_blocker?: string
  goal_path?: string[]
  goal_summary?: string
  queue_key?: string
  queue_label?: string
  queue_reason?: string
  queue_priority?: 'normal' | 'medium' | 'high' | string
  queue_sla_at?: string
  artifacts?: TaskArtifact[]
  plan_revisions?: TaskPlanRevision[]
  plan_required?: boolean
  plan_status?: string
  plan_blocker?: string
  latest_plan_summary?: string
  learning_candidate?: TaskLearningCandidate
  execution_lock?: TaskExecutionLock
  limits?: TaskExecutionLimits
  feedback?: TaskFeedback[]
  evals?: TaskEvalSignal[]
  read_at?: string
  archived_at?: string
  source_signal_id?: string
  source_decision_id?: string
  workspace_path?: string
  worktree_path?: string
  worktree_branch?: string
  depends_on?: string[]
  blocked?: boolean
  acked_at?: string
  due_at?: string
  follow_up_at?: string
  reminder_at?: string
  recheck_at?: string
  created_at?: string
  updated_at?: string
  awaiting_human?: boolean
  awaiting_human_since?: string
  awaiting_human_reason?: string
  awaiting_human_request_id?: string
  awaiting_human_source?: string
  recommended_responder?: string
  recommendation_status?: string
  recommendation_summary?: string
  recommendation_task_id?: string
  source_message_id?: string
  source_request_id?: string
  source_task_id?: string
  delivery_id?: string
  progress_percent?: number
  progress_basis?: string
  human_options?: InterviewOption[]
  human_recommended_id?: string
  liveness_state?: string
  liveness_reason?: string
  liveness_at?: string
  liveness_history?: LivenessEvent[]
}

export interface TaskExecutionLimits {
  max_attempts?: number
  max_runtime_minutes?: number
  max_cost_cents?: number
  attempts_used?: number
  runtime_ms_used?: number
  cost_cents_used?: number
  limit_state?: string
  last_attempt_at?: string
  last_limit_reason?: string
}

export interface TaskEvalSignal {
  id?: string
  kind?: string
  severity?: 'info' | 'warning' | 'error' | string
  summary?: string
  created_at?: string
}

export interface TaskFeedback {
  id?: string
  rating?: 'up' | 'down' | 'neutral' | string
  comment?: string
  created_by?: string
  created_at?: string
}

export interface TaskArtifact {
  id?: string
  kind?: string
  result_role?: string
  title?: string
  summary?: string
  path?: string
  url?: string
  preview_url?: string
  mime_type?: string
  size_bytes?: number
  checksum?: string
  state?: string
  browser_inspection?: TaskBrowserInspection
  validated_by?: string
  validated_at?: string
  created_by?: string
  created_at?: string
  updated_at?: string
}

export interface TaskBrowserInspection {
  page_url?: string
  selector?: string
  element_text?: string
  screenshot_path?: string
  viewport_width?: number
  viewport_height?: number
  notes?: string
}

export interface TaskExecutionLock {
  run_id?: string
  owner?: string
  status?: string
  acquired_at?: string
  heartbeat_at?: string
  expires_at?: string
}

export interface TaskPlanRevision {
  id?: string
  version?: number
  summary?: string
  content?: string
  status?: 'draft' | 'ready' | 'approved' | 'superseded' | string
  approved_by?: string
  approved_at?: string
  created_by?: string
  created_at?: string
}

export interface TaskLearningCandidate {
  recommended?: boolean
  kind?: string
  title?: string
  summary?: string
  skill_name?: string
  reason?: string
}

export interface LearningCandidateDiffFile {
  name: string
  kind: string
  status: string
  summary?: string
  before_size?: number
  after_size?: number
  before?: string
  after?: string
  risk_signals?: string[]
}

export interface LearningCandidateDiffPreview {
  generated_at: string
  persisted: boolean
  task_id: string
  channel: string
  action: string
  duplicate?: boolean
  candidate: {
    id: string
    task_id: string
    channel?: string
    owner?: string
    kind?: string
    title?: string
    summary?: string
    skill_name?: string
    reason?: string
    signals?: string[]
    promoted?: boolean
    promoted_skill_id?: string
    updated_at?: string
  }
  proposed_skill: Skill
  existing_skill?: Skill
  files: LearningCandidateDiffFile[]
  risk_level?: string
  risk_signals?: string[]
  summary?: Record<string, number>
}

export function reassignTask(taskId: string, newOwner: string, channel: string, actor = 'human') {
  return post<{ task: Task }>('/tasks', {
    action: 'reassign',
    id: taskId,
    owner: newOwner,
    channel: channel || 'general',
    created_by: actor,
  })
}

export type TaskStatusAction = 'release' | 'review' | 'block' | 'complete' | 'cancel'

export function updateTaskStatus(
  taskId: string,
  action: TaskStatusAction,
  channel: string,
  actor = 'human',
) {
  return post<{ task: Task }>('/tasks', {
    action,
    id: taskId,
    channel: channel || 'general',
    created_by: actor,
  })
}

export function updateTaskOutcome(
  taskId: string,
  channel: string,
  outcome: string,
  evidence: string,
  actor = 'human',
) {
  return post<{ task: Task }>('/tasks', {
    action: 'update_outcome',
    id: taskId,
    channel: channel || 'general',
    created_by: actor,
    outcome,
    outcome_evidence: evidence,
  })
}

export function updateTaskInboxState(
  taskId: string,
  channel: string,
  action: 'mark_read' | 'archive_inbox',
  actor = 'human',
) {
  return post<{ task: Task }>('/tasks', {
    action,
    id: taskId,
    channel: channel || 'general',
    created_by: actor,
  })
}

export function updateTaskLimits(
  taskId: string,
  channel: string,
  limits: Pick<TaskExecutionLimits, 'max_attempts' | 'max_runtime_minutes' | 'max_cost_cents'>,
  actor = 'human',
) {
  return post<{ task: Task }>('/tasks', {
    action: 'update_limits',
    id: taskId,
    channel: channel || 'general',
    created_by: actor,
    limits,
  })
}

export function recordTaskAttempt(taskId: string, channel: string, actor = 'human') {
  return post<{ task: Task }>('/tasks', {
    action: 'record_attempt',
    id: taskId,
    channel: channel || 'general',
    created_by: actor,
  })
}

export function addTaskFeedback(
  taskId: string,
  channel: string,
  rating: 'up' | 'down' | 'neutral',
  comment: string,
  actor = 'human',
) {
  return post<{ task: Task; feedback?: TaskFeedback }>('/tasks', {
    action: 'add_feedback',
    id: taskId,
    channel: channel || 'general',
    created_by: actor,
    feedback_rating: rating,
    feedback_comment: comment,
  })
}

export function addTaskArtifact(
  taskId: string,
  channel: string,
  artifact: Pick<TaskArtifact, 'kind' | 'result_role' | 'title' | 'summary' | 'path' | 'url' | 'preview_url' | 'state' | 'browser_inspection'>,
  actor = 'human',
) {
  return post<{ task: Task; artifact?: TaskArtifact }>('/tasks', {
    action: 'add_artifact',
    id: taskId,
    channel: channel || 'general',
    created_by: actor,
    artifact,
  })
}

export function acquireTaskExecutionLock(taskId: string, channel: string, actor = 'human', runId?: string, ttlSeconds?: number) {
  return post<{ task: Task; execution_lock?: TaskExecutionLock }>('/tasks', {
    action: 'acquire_execution_lock',
    id: taskId,
    channel: channel || 'general',
    created_by: actor,
    actor,
    run_id: runId,
    lock_ttl_seconds: ttlSeconds,
  })
}

export function heartbeatTaskExecutionLock(taskId: string, channel: string, runId: string, actor = 'human', ttlSeconds?: number) {
  return post<{ task: Task; execution_lock?: TaskExecutionLock }>('/tasks', {
    action: 'heartbeat_execution_lock',
    id: taskId,
    channel: channel || 'general',
    created_by: actor,
    actor,
    run_id: runId,
    lock_ttl_seconds: ttlSeconds,
  })
}

export function releaseTaskExecutionLock(taskId: string, channel: string, runId: string, actor = 'human') {
  return post<{ task: Task; execution_lock?: TaskExecutionLock }>('/tasks', {
    action: 'release_execution_lock',
    id: taskId,
    channel: channel || 'general',
    created_by: actor,
    actor,
    run_id: runId,
  })
}

export function saveTaskPlanRevision(
  taskId: string,
  channel: string,
  content: string,
  summary: string,
  actor = 'human',
) {
  return post<{ task: Task; plan_revision?: TaskPlanRevision }>('/tasks', {
    action: 'save_plan_revision',
    id: taskId,
    channel: channel || 'general',
    created_by: actor,
    plan_content: content,
    plan_summary: summary,
    plan_status: 'ready',
  })
}

export function approveTaskPlan(taskId: string, channel: string, actor = 'human') {
  return post<{ task: Task; plan_revision?: TaskPlanRevision }>('/tasks', {
    action: 'approve_plan',
    id: taskId,
    channel: channel || 'general',
    created_by: actor,
  })
}

export function promoteTaskLearning(taskId: string, channel: string, actor = 'human') {
  return post<{ task: Task; skill?: Skill; duplicate?: boolean }>('/tasks', {
    action: 'promote_learning',
    id: taskId,
    channel: channel || 'general',
    created_by: actor,
  })
}

export function getLearningCandidateDiffPreview(taskId: string, channel: string, actor = 'human') {
  const params = new URLSearchParams({
    task_id: taskId,
    viewer_slug: actor,
    include_content: 'true',
  })
  if (channel) params.set('channel', channel)
  return get<LearningCandidateDiffPreview>(`/learning/candidates/diff-preview?${params.toString()}`)
}

export interface TaskTemplate {
  id: string
  title: string
  description?: string
  task_type?: string
  execution_mode?: CreateTaskExecutionMode | string
  outcome?: string
  plan_content?: string
  artifact_kinds?: string[]
  max_attempts?: number
  max_runtime_minutes?: number
  max_cost_cents?: number
}

export type CreateTaskExecutionMode = 'office' | 'local_worktree' | 'external_workspace'

export function getTaskTemplates() {
  return get<{ templates: TaskTemplate[] }>('/task-templates')
}

export interface WorkQueueItem {
  task_id: string
  title: string
  queue_key?: string
  channel?: string
  owner?: string
  status?: string
  priority?: 'normal' | 'medium' | 'high' | string
  reason?: string
  sla_at?: string
  updated_at?: string
}

export interface WorkQueueGroup {
  key: string
  label: string
  reason?: string
  count: number
  high: number
  medium: number
  owners?: string[]
  channels?: string[]
  next?: WorkQueueItem
}

export interface WorkQueueSnapshot {
  generated_at: string
  queues: WorkQueueGroup[]
  next?: WorkQueueItem[]
}

export function getWorkQueues(opts?: { channel?: string; allChannels?: boolean; viewerSlug?: string }) {
  const params: Record<string, string> = { viewer_slug: opts?.viewerSlug || 'human' }
  if (opts?.allChannels) params.all_channels = 'true'
  if (opts?.channel) params.channel = opts.channel
  return get<WorkQueueSnapshot>('/work-queues', params)
}

export interface ResumePackFact {
  label: string
  value?: string
  detail?: string
  kind?: string
  when?: string
  source?: string
  related?: string
}

export interface ResumePack {
  generated_at: string
  task?: {
    id: string
    title: string
    channel?: string
    owner?: string
    status?: string
    queue?: string
    priority?: string
    goal_path?: string[]
    goal_summary?: string
    workspace_path?: string
    worktree_path?: string
    updated_at?: string
    liveness_state?: string
    liveness_reason?: string
  }
  context?: ResumePackFact[]
  evidence?: ResumePackFact[]
  next_steps?: string[]
  warnings?: string[]
  source?: string
}

export interface GovernanceEvent {
  id: string
  kind: string
  status?: string
  actor?: string
  channel?: string
  summary: string
  related_id?: string
  requires_topology_authorization?: boolean
  rollback_plan?: string
  created_at?: string
}

export interface SkillTrustSummary {
  total: number
  high: number
  medium: number
  low: number
}

export interface OperatorOverview {
  generated_at: string
  status: string
  counts: {
    open_tasks: number
    blocked_tasks: number
    human_requests: number
    governance_items: number
    next_work_items: number
  }
  alerts?: OperatorAlert[]
  next_work?: WorkQueueItem[]
  blockers?: StudioBlocker[]
  requests?: StudioRequestSnapshot[]
  governance?: GovernanceEvent[]
  skill_trust: SkillTrustSummary
  resume?: ResumePack
  health: StudioBrokerHealthSnapshot
}

export interface OperatorAlert {
  id: string
  severity: 'critical' | 'warning' | 'info' | string
  source: string
  title: string
  summary: string
  channel?: string
  related_type?: string
  related_id?: string
  action?: string
  endpoint?: string
  command?: string
  created_at?: string
  signals?: string[]
}

export interface OperatorAlerts {
  generated_at: string
  persisted: boolean
  status: string
  summary: Record<string, number>
  alerts: OperatorAlert[]
}

export interface NoiseCleanupPreviewItem {
  id: string
  kind: string
  task_id?: string
  watchdog_id?: string
  scheduler_slug?: string
  channel?: string
  title: string
  reason: string
  would_action: string
  safe: boolean
  requires_review?: boolean
}

export interface NoiseCleanupPreview {
  persisted: boolean
  generated_at: string
  summary: Record<string, number>
  items: NoiseCleanupPreviewItem[]
}

export interface ReleaseReadinessCheck {
  id: string
  status: string
  summary: string
  detail?: string
  next_step?: string
}

export interface ReleaseReadiness {
  generated_at: string
  status: string
  score: number
  checks: ReleaseReadinessCheck[]
  next_steps?: string[]
}

export interface SkillMetadataMigrationPreview {
  name: string
  title?: string
  current_score: number
  current_level: string
  plugin_id?: string
  plugin_kind?: string
  capabilities?: string[]
  health_status?: string
  would_update?: string[]
}

export interface SkillMetadataPreview {
  persisted: boolean
  generated_at: string
  summary: Record<string, number>
  previews: SkillMetadataMigrationPreview[]
}

export interface SkillCapabilityUpgradePreviewItem {
  id: string
  skill_name: string
  title?: string
  existing_capabilities?: string[]
  proposed_capabilities?: string[]
  added_capabilities?: string[]
  requires_approval: boolean
  required_reviews?: string[]
  risk_score: number
  risk_level: string
  reason?: string
}

export interface SkillCapabilityUpgradePreview {
  persisted: boolean
  generated_at: string
  summary: Record<string, number>
  previews: SkillCapabilityUpgradePreviewItem[]
}

export interface AdapterEnvironmentCheck {
  adapter_id: string
  name: string
  status: string
  summary: string
  checks?: string[]
  next_step?: string
}

export interface AdapterEnvironmentChecks {
  generated_at: string
  status: string
  summary: Record<string, number>
  checks: AdapterEnvironmentCheck[]
}

export interface AdapterConfigCheck {
  adapter_id: string
  name: string
  status: string
  config_ref?: string
  summary: string
  next_step?: string
}

export interface AdapterConfigChecks {
  generated_at: string
  status: string
  summary: Record<string, number>
  checks: AdapterConfigCheck[]
}

export interface BehaviorEvalResult {
  id: string
  status: string
  surface: string
  summary: string
  contract?: string
}

export interface BehaviorEvalReport {
  generated_at: string
  status: string
  summary: Record<string, number>
  cases: BehaviorEvalResult[]
}

export interface IntakeQueueItem {
  id: string
  kind: string
  queue: string
  title: string
  summary?: string
  channel?: string
  owner?: string
  priority?: string
  status?: string
  related_id?: string
  updated_at?: string
}

export interface IntakeQueueGroup {
  key: string
  label: string
  count: number
  high?: number
  channels?: string[]
  owners?: string[]
  next?: IntakeQueueItem
  summary?: Record<string, number>
  items?: IntakeQueueItem[]
}

export interface IntakeQueues {
  generated_at: string
  summary: Record<string, number>
  queues: IntakeQueueGroup[]
  next?: IntakeQueueItem[]
}

export interface PluginRuntimeItem {
  id: string
  kind: string
  name: string
  status?: string
  health_status?: string
  capabilities?: string[]
  config_ref?: string
  source?: string
  updated_at?: string
}

export interface PluginRuntimeJob {
  id: string
  plugin_id?: string
  kind?: string
  status?: string
  channel?: string
  schedule?: string
  next_run?: string
  last_started_at?: string
  last_finished_at?: string
  last_summary?: string
}

export interface PluginRuntimeRun {
  id: string
  plugin_id?: string
  action: string
  status: string
  actor?: string
  actor_type?: string
  summary?: string
  related_id?: string
  created_at?: string
}

export interface PluginRuntime {
  generated_at: string
  summary: Record<string, number>
  plugins: PluginRuntimeItem[]
  jobs?: PluginRuntimeJob[]
  runs?: PluginRuntimeRun[]
}

export interface PluginSandboxPreviewCandidate {
  id: string
  kind: string
  name: string
  worker_class?: string
  manifest_id?: string
  manifest_signature?: string
  runtime_status?: string
  sandbox_status: string
  capabilities?: string[]
  required_policies?: string[]
  missing_policies?: string[]
  filesystem_scope?: string[]
  network_policy?: string
  secret_refs?: string[]
  health_check?: string
  config_ref?: string
  risk_signals?: string[]
  next_step?: string
}

export interface PluginSandboxPreview {
  generated_at: string
  persisted: boolean
  status: string
  summary: Record<string, number>
  candidates: PluginSandboxPreviewCandidate[]
}

export interface MarketplaceManifestPreviewItem {
  id: string
  kind: string
  name: string
  source_type?: string
  source_ref?: string
  installed_hash?: string
  expected_hash?: string
  manifest_id: string
  manifest_version: string
  manifest_signature: string
  signature_status: string
  trust_level?: string
  trust_score?: number
  manifest_status: string
  drift_status: string
  install_enabled: boolean
  update_enabled: boolean
  capabilities?: string[]
  proposed_capabilities?: string[]
  added_capabilities?: string[]
  required_reviews?: string[]
  required_policies?: string[]
  missing_policies?: string[]
  risk_signals?: string[]
  risk_score?: number
  risk_level?: string
  next_step?: string
}

export interface MarketplaceManifestPreview {
  generated_at: string
  persisted: boolean
  status: string
  summary: Record<string, number>
  manifests: MarketplaceManifestPreviewItem[]
}

export interface KnowledgeWikiLintFinding {
  id: string
  article: string
  slug?: string
  severity: string
  kind: string
  summary: string
  source_id?: string
  channel?: string
}

export interface KnowledgeWikiPromotionProposal {
  id: string
  article_id: string
  slug: string
  title: string
  channel?: string
  source_id?: string
  source_kind?: string
  target_path: string
  action: string
  commit_message: string
  markdown?: string
  diff?: string
  reviewed_commit_only: boolean
  required_reviews?: string[]
  risk_signals?: string[]
  lint_findings?: KnowledgeWikiLintFinding[]
  next_step?: string
}

export interface KnowledgeWikiPromotionPreview {
  generated_at: string
  persisted: boolean
  status: string
  summary: Record<string, number>
  proposals: KnowledgeWikiPromotionProposal[]
}

export interface BrowserInspectionHandoff {
  id: string
  task_id: string
  task_title?: string
  channel?: string
  owner?: string
  artifact_id?: string
  artifact_title?: string
  page_url?: string
  selector?: string
  element_text?: string
  screenshot_path?: string
  viewport_width?: number
  viewport_height?: number
  evidence?: string
  handoff_prompt?: string
  ready: boolean
  missing_fields?: string[]
  risk_signals?: string[]
  next_step?: string
  updated_at?: string
}

export interface BrowserInspectionHandoffPreview {
  generated_at: string
  persisted: boolean
  status: string
  summary: Record<string, number>
  handoffs: BrowserInspectionHandoff[]
}

export interface RemoteSandboxCandidate {
  id: string
  provider: string
  kind: string
  readiness: string
  execution_enabled: boolean
  health_check?: string
  install_command_policy?: string
  install_command_enabled?: boolean
  install_command_preview?: string
  required_policies?: string[]
  missing_policies?: string[]
  policy_checks?: ExecutionEnvironmentPolicyCheck[]
  risk_signals?: string[]
  next_step?: string
}

export interface RemoteSandboxPreview {
  generated_at: string
  persisted: boolean
  status: string
  summary: Record<string, number>
  candidates: RemoteSandboxCandidate[]
}

export interface SchedulerRevisionsPreviewJob {
  slug?: string
  label?: string
  kind?: string
  channel?: string
  status?: string
  latest_revision_id?: string
  revision_count: number
  restore_enabled: boolean
  restore_readiness: string
  missing_policies?: string[]
  required_policies?: string[]
  risk_signals?: string[]
  next_step?: string
}

export interface SchedulerRevisionsPreview {
  generated_at: string
  persisted: boolean
  status: string
  summary: Record<string, number>
  jobs: SchedulerRevisionsPreviewJob[]
  policies?: string[]
  blocked_actions?: string[]
  next_step?: string
}

export interface WikiEditorPreviewMode {
  id: string
  label: string
  readiness: string
  editor_enabled: boolean
  risk_signals?: string[]
  next_step?: string
}

export interface WikiEditorPreviewCheck {
  id: string
  status: string
  summary: string
  contracts?: string[]
  next_step?: string
}

export interface WikiEditorPreview {
  generated_at: string
  persisted: boolean
  status: string
  summary: Record<string, number>
  modes: WikiEditorPreviewMode[]
  checks: WikiEditorPreviewCheck[]
  next_step?: string
}

export interface ProviderCompatibilityPreviewItem {
  provider: string
  readiness: string
  known_event_shapes?: string[]
  compatibility_checks?: string[]
  missing_tests?: string[]
  risk_signals?: string[]
  parser_change_sensitive: boolean
  next_step?: string
}

export interface ProviderCompatibilityPreview {
  generated_at: string
  persisted: boolean
  status: string
  summary: Record<string, number>
  providers: ProviderCompatibilityPreviewItem[]
  mutation_enabled: boolean
  next_step?: string
}

export interface ProjectOverviewWidgetPreview {
  id: string
  title: string
  kind: string
  readiness: string
  source?: string
  count?: number
  query_enabled: boolean
  mutation_enabled: boolean
  required_policies?: string[]
  missing_policies?: string[]
  risk_signals?: string[]
  next_step?: string
}

export interface ProjectOverviewWidgetsPreview {
  generated_at: string
  persisted: boolean
  status: string
  summary: Record<string, number>
  widgets: ProjectOverviewWidgetPreview[]
  mutation_enabled: boolean
  next_step?: string
}

export interface FileContextHandoffItem {
  id: string
  task_id: string
  task_title?: string
  channel?: string
  source: string
  path?: string
  url?: string
  summary?: string
  content_included: boolean
  missing_policies?: string[]
  risk_signals?: string[]
  next_step?: string
  updated_at?: string
}

export interface FileContextHandoffPreview {
  generated_at: string
  persisted: boolean
  status: string
  summary: Record<string, number>
  content_read_enabled: boolean
  blocked_actions?: string[]
  items: FileContextHandoffItem[]
  next_step?: string
}

export interface DesktopIDESurface {
  id: string
  name: string
  kind: string
  readiness: string
  required_checks?: string[]
  missing_checks?: string[]
  risk_signals?: string[]
  launch_endpoint?: string
  canonical_surface?: string
  next_step?: string
}

export interface DesktopIDEPreview {
  generated_at: string
  persisted: boolean
  status: string
  summary: Record<string, number>
  surfaces: DesktopIDESurface[]
}

export interface CompanyControlPlaneSnapshot {
  id: string
  name: string
  description?: string
  lead?: string
  manifest_source: string
  member_count: number
  channel_count: number
  task_count: number
  skill_count: number
  adapter_count: number
}

export interface CompanyControlPlaneExportItem {
  id: string
  label: string
  source: string
  count: number
  secret_scrubbed: boolean
  preview_only: boolean
  risk_signals?: string[]
}

export interface CompanyControlPlaneIsolation {
  id: string
  status: string
  summary: string
  next_step?: string
  contracts?: string[]
}

export interface CompanyControlPlanePreview {
  generated_at: string
  persisted: boolean
  status: string
  summary: Record<string, number>
  current_company: CompanyControlPlaneSnapshot
  export_items: CompanyControlPlaneExportItem[]
  isolation: CompanyControlPlaneIsolation[]
  blocked_mutations: string[]
  required_policies: string[]
  missing_policies: string[]
  risk_signals: string[]
  next_step: string
  apply_enabled: boolean
  topology_mutation_enabled: boolean
}

export interface WorkspaceInventoryEntry {
  id: string
  path: string
  kind?: string
  channel?: string
  owner?: string
  healthy: boolean
  issue?: string
  git_branch?: string
  git_dirty_count?: number
  active_task_count?: number
  task_ids?: string[]
  preview_urls?: string[]
  updated_at?: string
}

export interface WorkspaceInventory {
  generated_at: string
  summary: Record<string, number>
  workspaces: WorkspaceInventoryEntry[]
}

export interface OutcomeRecord {
  task_id: string
  title: string
  channel?: string
  owner?: string
  kind: string
  state: string
  evidence?: string
  artifact_id?: string
  updated_at?: string
}

export interface Outcomes {
  generated_at: string
  summary: Record<string, number>
  items: OutcomeRecord[]
}

export interface AgentSessionSnapshot {
  slug: string
  channel?: string
  status: string
  normalized_status?: string
  activity?: string
  detail?: string
  current_task_id?: string
  current_task_title?: string
  workspace_path?: string
  run_id?: string
  heartbeat_at?: string
  last_seen_at?: string
  open_task_count?: number
  queued_node_count?: number
  usage?: AgentUsage
  context_summary?: string
  next_action?: string
  liveness_state?: string
  liveness_reason?: string
  liveness_task_id?: string
  liveness_at?: string
  liveness_history?: LivenessEvent[]
  persistent_context: boolean
}

export interface LivenessEvent {
  state: string
  reason?: string
  task_id?: string
  actor?: string
  channel?: string
  created_at?: string
}

export interface AgentSessions {
  generated_at: string
  summary: Record<string, number>
  sessions: AgentSessionSnapshot[]
}

export interface ExecutionTraceStep {
  id: string
  kind: string
  actor?: string
  actor_type?: string
  status?: string
  normalized_status?: string
  summary?: string
  related_id?: string
  timestamp?: string
}

export interface ExecutionTraceEntry {
  task_id: string
  title: string
  channel?: string
  owner?: string
  status?: string
  normalized_status?: string
  started_at?: string
  updated_at?: string
  steps: ExecutionTraceStep[]
}

export interface ExecutionTrace {
  generated_at: string
  summary: Record<string, number>
  traces: ExecutionTraceEntry[]
}

export interface GovernanceRollbackChange {
  target: string
  field?: string
  action: string
  restore_to?: string
  reason?: string
  requires_manual?: boolean
}

export interface GovernanceRollbackPackage {
  id: string
  event_id: string
  event_kind: string
  event_summary: string
  status: string
  target_id?: string
  channel?: string
  required_confirmation: string
  required_reviews?: string[]
  rollback_plan: string
  changes: GovernanceRollbackChange[]
  snapshot_hint?: string
  created_at?: string
}

export interface GovernanceRollbackPackages {
  generated_at: string
  summary: Record<string, number>
  packages: GovernanceRollbackPackage[]
}

export interface OperatorRunbookStep {
  id: string
  title: string
  reason?: string
  endpoint?: string
  command?: string
  severity?: string
  dry_run: boolean
}

export interface OperatorRunbook {
  persisted: boolean
  generated_at: string
  summary: Record<string, number>
  steps: OperatorRunbookStep[]
}

export interface ApplyPreviewRequest {
  preview: 'noise_cleanup' | 'skill_metadata'
  item_ids: string[]
  actor?: string
  reason: string
  confirm: boolean
  confirmation: 'APPLY_PREVIEW'
}

export interface ApplyPreviewChange {
  kind: string
  id: string
  field: string
  before?: string
  after?: string
  reason?: string
}

export interface ApplyPreviewResponse {
  persisted: boolean
  preview: string
  applied: number
  skipped?: number
  required_confirmation?: string
  changes?: ApplyPreviewChange[]
  rollback_plan?: string
  message?: string
}

export function getOperatorOverview(opts?: { channel?: string; allChannels?: boolean; viewerSlug?: string }) {
  const params: Record<string, string> = { viewer_slug: opts?.viewerSlug || 'human' }
  if (opts?.allChannels) params.all_channels = 'true'
  if (opts?.channel) params.channel = opts.channel
  return get<OperatorOverview>('/operator/overview', params)
}

export function getOperatorAlerts(opts?: { channel?: string; allChannels?: boolean; viewerSlug?: string }) {
  const params: Record<string, string> = { viewer_slug: opts?.viewerSlug || 'human' }
  if (opts?.allChannels) params.all_channels = 'true'
  if (opts?.channel) params.channel = opts.channel
  return get<OperatorAlerts>('/operator/alerts', params)
}

export function getNoiseCleanupPreview(opts?: { channel?: string; allChannels?: boolean; viewerSlug?: string }) {
  const params: Record<string, string> = { viewer_slug: opts?.viewerSlug || 'human' }
  if (opts?.allChannels) params.all_channels = 'true'
  if (opts?.channel) params.channel = opts.channel
  return get<NoiseCleanupPreview>('/operator/noise-cleanup-preview', params)
}

export function getOperatorRunbook(opts?: { channel?: string; allChannels?: boolean; viewerSlug?: string }) {
  const params: Record<string, string> = { viewer_slug: opts?.viewerSlug || 'human' }
  if (opts?.allChannels) params.all_channels = 'true'
  if (opts?.channel) params.channel = opts.channel
  return get<OperatorRunbook>('/operator/runbook', params)
}

export function getReleaseReadiness() {
  return get<ReleaseReadiness>('/release/readiness')
}

export function getSkillMetadataPreview() {
  return get<SkillMetadataPreview>('/skills/metadata-preview')
}

export function getSkillCapabilityUpgradePreview() {
  return get<SkillCapabilityUpgradePreview>('/skills/capability-upgrade-preview')
}

export function getAdapterEnvironmentChecks() {
  return get<AdapterEnvironmentChecks>('/adapters/checks')
}

export function getAdapterConfigChecks() {
  return get<AdapterConfigChecks>('/adapters/config-checks')
}

export function getBehaviorEvals() {
  return get<BehaviorEvalReport>('/evals/behavior')
}

export function getIntakeQueues(opts?: { channel?: string; allChannels?: boolean; viewerSlug?: string }) {
  const params: Record<string, string> = { viewer_slug: opts?.viewerSlug || 'human' }
  if (opts?.allChannels) params.all_channels = 'true'
  if (opts?.channel) params.channel = opts.channel
  return get<IntakeQueues>('/intake/queues', params)
}

export function getPluginRuntime() {
  return get<PluginRuntime>('/plugins/runtime')
}

export function getPluginSandboxPreview() {
  return get<PluginSandboxPreview>('/plugins/sandbox-preview')
}

export function getMarketplaceManifestPreview() {
  return get<MarketplaceManifestPreview>('/marketplace/manifest-preview')
}

export function getKnowledgeWikiPromotionPreview(opts?: { channel?: string; allChannels?: boolean; viewerSlug?: string; taskID?: string }) {
  const params: Record<string, string> = { viewer_slug: opts?.viewerSlug || 'human' }
  if (opts?.allChannels) params.all_channels = 'true'
  if (opts?.channel) params.channel = opts.channel
  if (opts?.taskID) params.task_id = opts.taskID
  return get<KnowledgeWikiPromotionPreview>('/knowledge/wiki-promotion-preview', params)
}

export function getBrowserInspectionHandoffPreview(opts?: { channel?: string; allChannels?: boolean; viewerSlug?: string; taskID?: string }) {
  const params: Record<string, string> = { viewer_slug: opts?.viewerSlug || 'human' }
  if (opts?.allChannels) params.all_channels = 'true'
  if (opts?.channel) params.channel = opts.channel
  if (opts?.taskID) params.task_id = opts.taskID
  return get<BrowserInspectionHandoffPreview>('/browser/inspection-handoff-preview', params)
}

export function getRemoteSandboxPreview() {
  return get<RemoteSandboxPreview>('/runtime/remote-sandbox-preview')
}

export function getSchedulerRevisionsPreview(opts?: { channel?: string; allChannels?: boolean; viewerSlug?: string }) {
  const params: Record<string, string> = { viewer_slug: opts?.viewerSlug || 'human' }
  if (opts?.allChannels) params.all_channels = 'true'
  if (opts?.channel) params.channel = opts.channel
  return get<SchedulerRevisionsPreview>('/scheduler/revisions-preview', params)
}

export function getWikiEditorPreview() {
  return get<WikiEditorPreview>('/knowledge/wiki-editor-preview')
}

export function getProviderCompatibilityPreview() {
  return get<ProviderCompatibilityPreview>('/providers/compatibility-preview')
}

export function getProjectOverviewWidgetsPreview() {
  return get<ProjectOverviewWidgetsPreview>('/studio/project-overview-preview')
}

export function getFileContextHandoffPreview(opts?: { channel?: string; allChannels?: boolean; viewerSlug?: string; taskID?: string }) {
  const params: Record<string, string> = { viewer_slug: opts?.viewerSlug || 'human' }
  if (opts?.allChannels) params.all_channels = 'true'
  if (opts?.channel) params.channel = opts.channel
  if (opts?.taskID) params.task_id = opts.taskID
  return get<FileContextHandoffPreview>('/files/context-handoff-preview', params)
}

export function getDesktopIDEPreview() {
  return get<DesktopIDEPreview>('/integrations/desktop/preview')
}

export function getCompanyControlPlanePreview() {
  return get<CompanyControlPlanePreview>('/companies/control-plane-preview')
}

export function getWorkspaceInventory(opts?: { channel?: string; allChannels?: boolean; viewerSlug?: string }) {
  const params: Record<string, string> = { viewer_slug: opts?.viewerSlug || 'human' }
  if (opts?.allChannels) params.all_channels = 'true'
  if (opts?.channel) params.channel = opts.channel
  return get<WorkspaceInventory>('/workspaces', params)
}

export function getOutcomes(opts?: { channel?: string; allChannels?: boolean; viewerSlug?: string }) {
  const params: Record<string, string> = { viewer_slug: opts?.viewerSlug || 'human' }
  if (opts?.allChannels) params.all_channels = 'true'
  if (opts?.channel) params.channel = opts.channel
  return get<Outcomes>('/outcomes', params)
}

export function getAgentSessions(opts?: { channel?: string; allChannels?: boolean; viewerSlug?: string }) {
  const params: Record<string, string> = { viewer_slug: opts?.viewerSlug || 'human' }
  if (opts?.allChannels) params.all_channels = 'true'
  if (opts?.channel) params.channel = opts.channel
  return get<AgentSessions>('/agent-sessions', params)
}

export function getExecutionTrace(opts?: { channel?: string; allChannels?: boolean; viewerSlug?: string; taskID?: string }) {
  const params: Record<string, string> = { viewer_slug: opts?.viewerSlug || 'human' }
  if (opts?.allChannels) params.all_channels = 'true'
  if (opts?.channel) params.channel = opts.channel
  if (opts?.taskID) params.task_id = opts.taskID
  return get<ExecutionTrace>('/execution-trace', params)
}

export function getGovernanceRollbackPackages(opts?: { channel?: string; allChannels?: boolean; viewerSlug?: string; id?: string }) {
  const params: Record<string, string> = { viewer_slug: opts?.viewerSlug || 'human' }
  if (opts?.allChannels) params.all_channels = 'true'
  if (opts?.channel) params.channel = opts.channel
  if (opts?.id) params.id = opts.id
  return get<GovernanceRollbackPackages>('/governance/rollback-packages', params)
}

export function applyPreview(body: ApplyPreviewRequest) {
  return post<ApplyPreviewResponse>('/operator/apply-preview', body)
}

export function getTasks(channel: string, opts?: { includeDone?: boolean; status?: string; mySlug?: string; lite?: boolean }) {
  const params: Record<string, string> = { viewer_slug: 'human', channel: channel || 'general' }
  if (opts?.includeDone) params.include_done = 'true'
  if (opts?.status) params.status = opts.status
  if (opts?.mySlug) params.my_slug = opts.mySlug
  if (opts?.lite) params.lite = 'true'
  return get<{ tasks: Task[] }>('/tasks', params)
}

export function getOfficeTasks(opts?: { includeDone?: boolean; status?: string; mySlug?: string; lite?: boolean }) {
  const params: Record<string, string> = { viewer_slug: 'human', all_channels: 'true' }
  if (opts?.includeDone) params.include_done = 'true'
  if (opts?.status) params.status = opts.status
  if (opts?.mySlug) params.my_slug = opts.mySlug
  if (opts?.lite) params.lite = 'true'
  return get<{ tasks: Task[] }>('/tasks', params)
}

// ── Signals / Decisions / Watchdogs / Actions ──

export function getSignals() { return get('/signals') }
export function getDecisions() { return get('/decisions') }
export function getWatchdogs() { return get('/watchdogs') }
export function getActions() { return get('/actions') }
export interface ActivityEvent {
  id: string
  type: string
  kind?: string
  source?: string
  channel?: string
  actor?: string
  actor_type?: 'human' | 'agent' | 'system' | 'adapter'
  title?: string
  summary?: string
  related_id?: string
  severity?: string
  timestamp?: string
}
export function getActivity(opts?: { limit?: number; type?: string; channel?: string }) {
  return get<{ events: ActivityEvent[] }>('/activity', opts)
}

export interface DeliveryArtifact {
  kind: string
  title: string
  summary?: string
  state?: string
  path?: string
  url?: string
  updated_at?: string
  related_id?: string
}

export interface Delivery {
  id: string
  title: string
  summary?: string
  status: string
  owner?: string
  channel?: string
  workspace_path?: string
  progress_percent?: number
  progress_basis?: string
  last_substantive_update_at?: string
  last_substantive_update_by?: string
  last_substantive_summary?: string
  pending_human_count?: number
  blocker_count?: number
  task_ids?: string[]
  request_ids?: string[]
  artifacts?: DeliveryArtifact[]
}

export function getDeliveries(opts?: { includeDone?: boolean; channel?: string }) {
  const params: Record<string, string> = { viewer_slug: 'human', all_channels: 'true' }
  if (opts?.includeDone) params.include_done = 'true'
  if (opts?.channel) {
    delete params.all_channels
    params.channel = opts.channel
  }
  return get<{ deliveries: Delivery[] }>('/deliveries', params)
}

// ── Policies ──

export interface Policy {
  id: string
  source: string
  rule: string
  active?: boolean
}

export interface PolicyMutationResponse {
  ok?: boolean
  persisted: boolean
  duplicate?: boolean
  policy?: Policy
}

export function getPolicies() {
  return get<{ policies: Policy[] }>('/policies')
}

export function createPolicy(source: string, rule: string, requestId?: string) {
  return post<PolicyMutationResponse>('/policies', { source, rule, request_id: requestId })
}

export function deletePolicy(id: string, requestId?: string) {
  return del<PolicyMutationResponse>('/policies', { id, request_id: requestId })
}

// ── Scheduler ──

export interface SchedulerJob {
  id?: string
  slug?: string
  name?: string
  label?: string
  kind?: string
  cron?: string
  channel?: string
  target_type?: string
  target_id?: string
  workflow_key?: string
  schedule_expr?: string
  skill_name?: string
  skill_names?: string[]
  next_run?: string
  last_run?: string
  run_count?: number
  concurrency_policy?: string
  catch_up_policy?: string
  max_parallel?: number
  running_count?: number
  last_started_at?: string
  last_finished_at?: string
  last_status?: string
  last_summary?: string
  due_at?: string
  status?: string
}

export function getScheduler(opts?: { dueOnly?: boolean }) {
  const params: Record<string, string> = {}
  if (opts?.dueOnly) params.due_only = 'true'
  return get<{ jobs: SchedulerJob[] }>('/scheduler', params)
}

export interface SchedulerSkillPreviewBinding {
  name: string
  found: boolean
  status?: string
  trust_level?: string
  trust_score?: number
  source_type?: string
  scan_status?: string
  risk_level: string
  reasons?: string[]
  capabilities?: string[]
}

export interface SchedulerSkillPreviewJob {
  slug?: string
  label?: string
  kind?: string
  channel?: string
  target_type?: string
  target_id?: string
  workflow_key?: string
  schedule_expr?: string
  status?: string
  skill_names?: string[]
  readiness: string
  risk_level: string
  reasons?: string[]
  skills?: SchedulerSkillPreviewBinding[]
}

export interface SchedulerSkillPreviewResponse {
  generated_at: string
  persisted: boolean
  summary: Record<string, number>
  jobs: SchedulerSkillPreviewJob[]
}

export function getSchedulerSkillPreview(opts?: {
  channel?: string
  viewerSlug?: string
  q?: string
  readiness?: string
  risk?: string
  limit?: number
  includeUnbound?: boolean
  includeTerminal?: boolean
  allChannels?: boolean
}) {
  const params: Record<string, string> = {}
  if (opts?.channel) params.channel = opts.channel
  if (opts?.viewerSlug) params.viewer_slug = opts.viewerSlug
  if (opts?.q) params.q = opts.q
  if (opts?.readiness) params.readiness = opts.readiness
  if (opts?.risk) params.risk = opts.risk
  if (opts?.limit) params.limit = String(opts.limit)
  if (opts?.includeUnbound) params.include_unbound = 'true'
  if (opts?.includeTerminal) params.include_terminal = 'true'
  if (opts?.allChannels) params.all_channels = 'true'
  return get<SchedulerSkillPreviewResponse>('/scheduler/skill-preview', params)
}

export interface ToolsetCapabilityPreview {
  name: string
  source?: string
  kind?: string
  status?: string
  mutating?: boolean
  external?: boolean
  secret_bearing?: boolean
  scheduler_mutating?: boolean
}

export interface ToolsetProfilePreview {
  id: string
  agent_slug: string
  agent_name?: string
  channel?: string
  permission_mode?: string
  declared_tools?: string[]
  runtime_toolsets?: string[]
  capabilities?: ToolsetCapabilityPreview[]
  drift?: string[]
  risk_level: string
  suggested_action: string
  signals?: string[]
}

export interface ToolsetProfilePreviewResponse {
  generated_at: string
  persisted: boolean
  summary: Record<string, number>
  profiles: ToolsetProfilePreview[]
}

export function getToolsetProfilePreview(opts?: {
  channel?: string
  viewerSlug?: string
  q?: string
  risk?: string
  action?: string
  limit?: number
  allChannels?: boolean
}) {
  const params: Record<string, string> = {}
  if (opts?.channel) params.channel = opts.channel
  if (opts?.viewerSlug) params.viewer_slug = opts.viewerSlug
  if (opts?.q) params.q = opts.q
  if (opts?.risk) params.risk = opts.risk
  if (opts?.action) params.action = opts.action
  if (opts?.limit) params.limit = String(opts.limit)
  if (opts?.allChannels) params.all_channels = 'true'
  return get<ToolsetProfilePreviewResponse>('/toolsets/profile-preview', params)
}

export interface HumanPermissionCapability {
  name: string
  status: string
  scope?: string
  mutating?: boolean
  requires_review?: boolean
  reason?: string
}

export interface HumanPermissionChannelSnapshot {
  id: string
  viewer_slug: string
  channel: string
  access_level: string
  can_read: boolean
  can_answer_requests: boolean
  can_review_tasks: boolean
  can_approve_actions: boolean
  can_mutate_topology: boolean
  capabilities?: HumanPermissionCapability[]
  signals?: string[]
  next_step?: string
}

export interface HumanPermissionsPreview {
  generated_at: string
  persisted: boolean
  viewer_slug: string
  channel?: string
  summary: Record<string, number>
  snapshots: HumanPermissionChannelSnapshot[]
}

export function getHumanPermissionsPreview(opts?: {
  channel?: string
  viewerSlug?: string
  allChannels?: boolean
}) {
  const params: Record<string, string> = {}
  if (opts?.channel) params.channel = opts.channel
  if (opts?.viewerSlug) params.viewer_slug = opts.viewerSlug
  if (opts?.allChannels) params.all_channels = 'true'
  return get<HumanPermissionsPreview>('/humans/permissions-preview', params)
}

export interface RecallSource {
  kind: string
  id?: string
  label?: string
  channel?: string
  task_id?: string
  when?: string
  ref?: string
}

export interface RecallSearchResult {
  id: string
  kind: string
  title: string
  summary?: string
  channel?: string
  actor?: string
  source_id?: string
  task_id?: string
  updated_at?: string
  rank: number
  rank_signals?: string[]
  quality_score: number
  quality_signals?: string[]
  risk_signals?: string[]
  sources?: RecallSource[]
}

export interface RecallSearchPreviewResponse {
  generated_at: string
  persisted: boolean
  query?: string
  summary: Record<string, number>
  results: RecallSearchResult[]
}

export function getRecallSearchPreview(opts?: {
  channel?: string
  viewerSlug?: string
  q?: string
  kind?: string
  limit?: number
  allChannels?: boolean
}) {
  const params: Record<string, string> = {}
  if (opts?.channel) params.channel = opts.channel
  if (opts?.viewerSlug) params.viewer_slug = opts.viewerSlug
  if (opts?.q) params.q = opts.q
  if (opts?.kind) params.kind = opts.kind
  if (opts?.limit) params.limit = String(opts.limit)
  if (opts?.allChannels) params.all_channels = 'true'
  return get<RecallSearchPreviewResponse>('/recall/search-preview', params)
}

export interface CommandManifestEntry {
  name: string
  category: string
  description: string
  surface: string
  route?: string
  method?: string
  args?: string
  mutating: boolean
  requires_confirmation?: boolean
  topology_sensitive?: boolean
  signals?: string[]
}

export interface CommandManifestResponse {
  generated_at: string
  persisted: boolean
  summary: Record<string, number>
  commands: CommandManifestEntry[]
}

export function getCommandManifest(opts?: {
  q?: string
  category?: string
  mutating?: boolean
  surface?: 'web' | 'tui' | 'all'
}) {
  const params: Record<string, string> = {}
  if (opts?.q) params.q = opts.q
  if (opts?.category) params.category = opts.category
  if (opts?.mutating) params.mutating = 'true'
  if (opts?.surface) params.surface = opts.surface
  return get<CommandManifestResponse>('/commands/manifest', params)
}

export interface ExecutionEnvironmentPreview {
  id: string
  kind: string
  status: string
  readiness: string
  summary?: string
  channels?: string[]
  task_ids?: string[]
  workspace_count?: number
  active_task_count?: number
  required_policies?: string[]
  missing_policies?: string[]
  policy_checks?: ExecutionEnvironmentPolicyCheck[]
  signals?: string[]
  next_step?: string
  requires_review?: boolean
}

export interface ExecutionEnvironmentPolicyCheck {
  id: string
  status: string
  summary: string
  next_step?: string
}

export interface ExecutionEnvironmentPreviewResponse {
  generated_at: string
  persisted: boolean
  summary: Record<string, number>
  environments: ExecutionEnvironmentPreview[]
}

export function getExecutionEnvironmentsPreview(opts?: {
  channel?: string
  viewerSlug?: string
  kind?: string
  allChannels?: boolean
}) {
  const params: Record<string, string> = {}
  if (opts?.channel) params.channel = opts.channel
  if (opts?.viewerSlug) params.viewer_slug = opts.viewerSlug
  if (opts?.kind) params.kind = opts.kind
  if (opts?.allChannels) params.all_channels = 'true'
  return get<ExecutionEnvironmentPreviewResponse>('/runtime/execution-environments-preview', params)
}

export interface SkillFilePreview {
  name: string
  kind: string
  title?: string
  summary?: string
  size: number
  available: boolean
  content?: string
  risk_signals?: string[]
}

export interface SkillFilePreviewResponse {
  generated_at: string
  persisted: boolean
  skill_name: string
  channel?: string
  selected?: string
  files: SkillFilePreview[]
  summary: Record<string, number>
}

export function getSkillFilesPreview(name: string, opts?: {
  channel?: string
  viewerSlug?: string
  file?: string
  includeContent?: boolean
}) {
  const params: Record<string, string> = {}
  if (opts?.channel) params.channel = opts.channel
  if (opts?.viewerSlug) params.viewer_slug = opts.viewerSlug
  if (opts?.file) params.file = opts.file
  if (opts?.includeContent) params.include_content = 'true'
  return get<SkillFilePreviewResponse>(`/skills/${encodeURIComponent(name)}/files-preview`, params)
}

// ── Skills ──

export interface Skill {
  id?: string
  name: string
  title?: string
  description?: string
  source?: string
  channel?: string
  plugin_id?: string
  plugin_kind?: string
  capabilities?: string[]
  health_status?: string
  health_summary?: string
  status?: string
  usage_count?: number
  last_execution_at?: string
  last_execution_status?: string
  parameters?: unknown
}

export interface SkillMutationResponse {
  ok?: boolean
  persisted: boolean
  duplicate?: boolean
  skill?: Skill
  channel?: string
}

export function getSkills() {
  return get<{ skills: Skill[] }>('/skills')
}

export function invokeSkill(name: string, params?: Record<string, unknown>, requestId?: string) {
  return post<SkillMutationResponse>(`/skills/${encodeURIComponent(name)}/invoke`, { ...(params ?? {}), request_id: requestId })
}

export interface OfficeAdapter {
  id: string
  name: string
  kind?: string
  provider?: string
  description?: string
  capabilities?: string[]
  status?: string
  health_status?: string
  health_summary?: string
  config_ref?: string
  source?: string
  created_by?: string
  created_at?: string
  updated_at?: string
}

export interface OrgProposal {
  id: string
  kind?: string
  title: string
  summary?: string
  rationale?: string
  proposed_by?: string
  channel?: string
  target_type?: string
  target_id?: string
  proposed_change?: string
  status?: string
  requires_topology_authorization?: boolean
  source_task_id?: string
  source_message_id?: string
  decided_by?: string
  decided_at?: string
  decision_reason?: string
  created_at?: string
  updated_at?: string
}

export function getAdapters(opts?: { kind?: string; provider?: string; capability?: string; status?: string }) {
  return get<{ adapters: OfficeAdapter[] }>('/adapters', opts)
}

export function upsertAdapter(adapter: Partial<OfficeAdapter> & { id: string; name: string; created_by?: string }) {
  return post<{ adapter: OfficeAdapter; persisted?: boolean; updated?: boolean }>('/adapters', {
    ...adapter,
    created_by: adapter.created_by || 'human',
  })
}

export function getOrgProposals(opts?: { kind?: string; status?: string }) {
  return get<{ proposals: OrgProposal[] }>('/org-proposals', opts)
}

export function proposeOrgChange(proposal: Partial<OrgProposal> & { title: string; proposed_by?: string }) {
  return post<{ proposal: OrgProposal; persisted?: boolean }>('/org-proposals', {
    action: 'propose',
    ...proposal,
    proposed_by: proposal.proposed_by || 'human',
  })
}

export function decideOrgProposal(id: string, action: 'approve' | 'reject', actor = 'human', decisionReason?: string) {
  return post<{ proposal: OrgProposal; persisted?: boolean }>('/org-proposals', {
    action,
    id,
    actor,
    decision_reason: decisionReason,
  })
}

export function convertCeoConversation(input: {
  kind?: 'task' | 'decision' | 'request'
  channel?: string
  created_by?: string
  source_message_id?: string
  thread_id?: string
  title: string
  summary?: string
  details?: string
  outcome?: string
  owner?: string
  reason?: string
  blocking?: boolean
}) {
  return post('/ceo/convert', { created_by: 'human', ...input })
}

// ── Usage ──

export interface AgentUsage {
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_creation_tokens?: number
  total_tokens?: number
  cost_usd: number
  requests?: number
}

export interface UsageData {
  total?: { cost_usd: number; total_tokens?: number }
  session?: { total_tokens: number }
  agents?: Record<string, AgentUsage>
}

export function getUsage() {
  return get<UsageData>('/usage')
}

// ── Agent Logs ──

export interface AgentLog {
  id: string
  agent: string
  task?: string
  action?: string
  content?: string
  timestamp?: string
  usage?: TokenUsage
}

export function getAgentLogs(opts?: { limit?: number; task?: string }) {
  if (opts?.task) {
    return get<{ logs: AgentLog[] }>('/agent-logs', { task: opts.task })
  }
  const params: Record<string, string> = {}
  if (opts?.limit) params.limit = String(opts.limit)
  return get<{ logs: AgentLog[] }>('/agent-logs', params)
}

// ── Memory ──

export function getMemory(channel: string) {
  return get('/memory', { channel: channel || 'general' })
}

export function setMemory(namespace: string, key: string, value: string) {
  return post('/memory', { namespace, key, value })
}

// ── Studio ──

export interface StudioBootstrapWorkflow {
  id: string
  name?: string
  status?: string
  workflow_key?: string
}

export interface StudioBootstrapPackage {
  blueprint?: string
  bootstrap_config?: unknown
  starter?: unknown
  automation?: unknown
  integrations?: unknown[]
  connections?: unknown[]
  smoke_tests?: unknown[]
  workflow_drafts?: unknown[]
  workstream_seed?: unknown
  offers?: unknown[]
  status?: string
  name?: string
  description?: string
  workflows?: StudioBootstrapWorkflow[]
  created_at?: string
  updated_at?: string
}

export interface StudioBootstrapResponse {
  package: StudioBootstrapPackage
}

export interface StudioTaskCounts {
  total: number
  open?: number
  in_progress?: number
  blocked?: number
  review?: number
  done?: number
  canceled?: number
  other?: number
}

export interface StudioBrokerHealthSnapshot {
  broker_reachable: boolean
  api_reachable: boolean
  web_reachable: boolean
  degraded: boolean
  signals?: string[]
  build?: unknown
}

export interface StudioBootstrapSnapshot {
  ready: boolean
  summary: string
  members?: number
  channels?: number
  tasks?: number
  requests?: number
  workspaces?: number
  workflows?: number
}

export interface StudioOfficeSnapshot {
  status: string
  provider?: string
  focus_mode: boolean
  session_mode?: string
  memory_backend?: string
  health: StudioBrokerHealthSnapshot
  bootstrap: StudioBootstrapSnapshot
  task_counts: StudioTaskCounts
}

export interface StudioEnvironmentSnapshot {
  status: string
  broker_reachable: boolean
  api_reachable: boolean
  web_reachable: boolean
  memory_backend_selected?: string
  memory_backend_active?: string
  memory_backend_ready: boolean
  degraded: boolean
  signals?: string[]
  build?: unknown
  runtime_doctor?: RuntimeDoctorSnapshot
}

export interface RuntimeDoctorCheck {
  id: string
  label: string
  severity: 'ok' | 'warn' | 'fail' | 'info' | string
  summary: string
  detail?: string
  next_step?: string
}

export interface RuntimeDoctorSnapshot {
  status: 'ok' | 'degraded' | 'blocked' | string
  generated_at: string
  runtime_home?: string
  working_directory?: string
  executable?: string
  web_origins?: string[]
  processes?: Array<{ pid?: string; kind?: string; command_line?: string }>
  web_dist?: {
    source?: string
    index_path?: string
    index_hash?: string
    index_mod_time?: string
    asset_count?: number
    issue?: string
  }
  quarantine_signals?: Array<{ kind: string; severity: string; summary: string; task_ids?: string[]; path?: string; next_step?: string }>
  restart_advice?: {
    required: boolean
    summary?: string
    reasons?: string[]
    next_step?: string
  }
  secret_audit?: {
    strict: boolean
    plaintext_config_count: number
    plaintext_config_names?: string[]
    secret_env_count: number
    secret_env_names?: string[]
    store_path?: string
  }
  backup_policy?: {
    retention_days: number
    max_snapshots: number
    max_mb: number
    local_snapshot_count: number
    local_snapshot_bytes?: number
    cloud_provider?: string
    cloud_enabled: boolean
    cloud_prefix?: string
    next_step?: string
  }
  checks?: RuntimeDoctorCheck[]
}

export interface StudioAttentionGroup {
  key: string
  kind: string
  severity: string
  title: string
  summary: string
  count: number
  latest_at?: string
  task_ids?: string[]
}

export interface StudioChannelSnapshot {
  slug: string
  name?: string
  members?: string[]
  task_counts: StudioTaskCounts
  request_count?: number
  flow_count?: number
  workspace_count?: number
  blockers?: string[]
  attention_count?: number
  waiting_human_count?: number
  active_owner_count?: number
  last_substantive_update_at?: string
  last_substantive_update_by?: string
  last_substantive_preview?: string
  last_decision_at?: string
  last_decision_summary?: string
  attention?: StudioAttentionGroup[]
}

export interface StudioFlowSnapshot {
  id: string
  label: string
  channel?: string
  owner?: string
  status?: string
  execution_mode?: string
  workflow_key?: string
  pipeline_id?: string
  task_count: number
  blocked_count?: number
  workspace?: string
  task_ids?: string[]
}

export interface StudioWorkspaceSnapshot {
  path: string
  worktree_path?: string
  branch?: string
  channel?: string
  owner?: string
  healthy: boolean
  issue?: string
  task_counts: StudioTaskCounts
  task_ids?: string[]
}

export interface StudioTaskSnapshot {
  id: string
  channel?: string
  title?: string
  owner?: string
  status?: string
  blocked?: boolean
  task_type?: string
  execution_mode?: string
  outcome?: string
  outcome_status?: string
  outcome_evidence?: string
  queue_key?: string
  artifact_count?: number
  plan_revision_count?: number
  latest_plan_summary?: string
  eval_count?: number
  eval_severity?: string
  workflow_key?: string
  pipeline_id?: string
  workspace_path?: string
  worktree_path?: string
  worktree_branch?: string
  depends_on?: string[]
  updated_at?: string
  liveness_state?: string
  liveness_reason?: string
  liveness_at?: string
}

export interface StudioRequestSnapshot {
  id: string
  kind?: string
  status?: string
  channel?: string
  from?: string
  title?: string
  question?: string
  blocking?: boolean
  required?: boolean
  reply_to?: string
}

export interface StudioMessageSnapshot {
  id: string
  channel?: string
  from?: string
  title?: string
  content?: string
  reply_to?: string
  timestamp?: string
}

export interface StudioActiveContextSnapshot {
  session_mode?: string
  direct_agent?: string
  focus?: string
  next_steps?: string[]
  primary_channel?: string
  channels?: StudioChannelSnapshot[]
  flows?: StudioFlowSnapshot[]
  workspaces?: StudioWorkspaceSnapshot[]
  tasks?: StudioTaskSnapshot[]
  requests?: StudioRequestSnapshot[]
  messages?: StudioMessageSnapshot[]
}

export interface StudioActionDefinition {
  action: string
  label: string
  description?: string
  mutating?: boolean
  frontend_handled?: boolean
  requires_task_id?: boolean
  requires_channel?: boolean
  requires_owner?: boolean
  requires_agent?: boolean
}

export interface StudioActionInvocation extends StudioActionDefinition {
  task_id?: string
  channel?: string
  owner?: string
  agent?: string
}

export interface StudioBlocker {
  id: string
  kind: string
  severity: string
  title: string
  summary: string
  channel?: string
  task_id?: string
  owner?: string
  reason: string
  waiting_on?: string
  recommended_action?: string
  available_actions?: StudioActionInvocation[]
}

export interface StudioDevConsoleResponse {
  office: StudioOfficeSnapshot
  environment: StudioEnvironmentSnapshot
  active_context: StudioActiveContextSnapshot
  blockers: StudioBlocker[]
  actions: StudioActionDefinition[]
}

export interface StudioDevConsoleActionRequest {
  action: string
  task_id?: string
  channel?: string
  owner?: string
  actor?: string
  agent?: string
}

export interface StudioDevConsoleActionResponse {
  ok: boolean
  action: string
  task_id?: string
  channel?: string
  message?: string
  frontend_handled?: boolean
}

export interface OpenCoDesignStatus {
  available: boolean
  executable?: string
  prototype_dir: string
  message: string
  install_commands?: string[]
}

export interface OpenCoDesignLaunchResponse {
  ok: boolean
  available: boolean
  launched: boolean
  executable?: string
  prototype_dir: string
  message: string
}

export interface DesktopLaunchResponse {
  ok: boolean
  launched: boolean
  web_url: string
  desktop_dir?: string
  message: string
}

export function getStudioDevConsole() {
  return get<StudioDevConsoleResponse>('/studio/dev-console')
}

export function runStudioDevConsoleAction(payload: StudioDevConsoleActionRequest) {
  return post<StudioDevConsoleActionResponse>('/studio/dev-console/action', payload)
}

export function getOpenCoDesignStatus() {
  return get<OpenCoDesignStatus>('/integrations/open-codesign/status')
}

export function launchOpenCoDesign(payload?: { prototype_dir?: string }) {
  return post<OpenCoDesignLaunchResponse>('/integrations/open-codesign/launch', payload ?? {})
}

export function launchDesktopMode(payload?: { web_url?: string }) {
  return post<DesktopLaunchResponse>('/integrations/desktop/launch', payload ?? {})
}

export function getStudioBootstrapPackage() {
  return get<StudioBootstrapResponse>('/operations/bootstrap-package')
}

export function generateStudioPackage(payload?: unknown) {
  return post('/studio/generate-package', payload ?? {})
}

export function runStudioWorkflow(payload?: unknown) {
  return post('/studio/run-workflow', payload ?? {})
}

// ── Config (Settings) ──

export type LLMProvider = GlobalLLMProvider
export type MemoryBackend = 'none'
export type ActionProvider = 'auto' | 'composio' | 'one' | ''
export type WebSearchProvider = 'none' | 'brave' | ''

const PROVIDER_LABELS: Record<ProviderKind, string> = {
  'claude-code': 'Claude Code',
  codex: 'Codex',
  gemini: 'Gemini',
  ollama: 'Ollama',
}

export function formatProviderLabel(kind?: ProviderKind | null): string {
  if (!kind) return 'Office default'
  return PROVIDER_LABELS[kind] ?? kind
}

export interface ConfigSnapshot {
  // Runtime
  llm_provider?: LLMProvider
  memory_backend?: MemoryBackend
  action_provider?: ActionProvider
  web_search_provider?: WebSearchProvider
  custom_mcp_config_path?: string
  cloud_backup_provider?: string
  cloud_backup_bucket?: string
  cloud_backup_prefix?: string
  team_lead_slug?: string
  max_concurrent_agents?: number
  default_format?: string
  default_timeout?: number
  blueprint?: string
  // Workspace
  email?: string
  workspace_id?: string
  workspace_slug?: string
  dev_url?: string
  // Company
  company_name?: string
  company_description?: string
  company_goals?: string
  company_size?: string
  company_priority?: string
  // Polling
  insights_poll_minutes?: number
  task_follow_up_minutes?: number
  task_reminder_minutes?: number
  task_recheck_minutes?: number
  // Local history retention
  broker_history_retention_days?: number
  broker_history_max_snapshots?: number
  broker_history_max_mb?: number
  // Secret flags
  api_key_set?: boolean
  openai_key_set?: boolean
  anthropic_key_set?: boolean
  gemini_key_set?: boolean
  minimax_key_set?: boolean
  brave_key_set?: boolean
  one_key_set?: boolean
  composio_key_set?: boolean
  telegram_token_set?: boolean
  config_path?: string
}

export type ConfigUpdate = Partial<{
  llm_provider: LLMProvider
  memory_backend: MemoryBackend
  action_provider: ActionProvider
  web_search_provider: WebSearchProvider
  custom_mcp_config_path: string
  cloud_backup_provider: string
  cloud_backup_bucket: string
  cloud_backup_prefix: string
  team_lead_slug: string
  max_concurrent_agents: number
  default_format: string
  default_timeout: number
  blueprint: string
  email: string
  dev_url: string
  company_name: string
  company_description: string
  company_goals: string
  company_size: string
  company_priority: string
  insights_poll_minutes: number
  task_follow_up_minutes: number
  task_reminder_minutes: number
  task_recheck_minutes: number
  broker_history_retention_days: number
  broker_history_max_snapshots: number
  broker_history_max_mb: number
  // Secret-write fields — sent as plaintext on write, never returned on read
  api_key: string
  openai_api_key: string
  anthropic_api_key: string
  gemini_api_key: string
  minimax_api_key: string
  brave_api_key: string
  one_api_key: string
  composio_api_key: string
  telegram_bot_token: string
}>

export function getConfig() {
  return get<ConfigSnapshot>('/config')
}

export function updateConfig(patch: ConfigUpdate) {
  return post<{ status: string }>('/config', patch)
}

// -- Workspace maintenance --

// WorkspaceWipeResult shape mirrors internal/workspace.Result plus the flags
// the HTTP handler adds (restart_required, redirect). The legacy shred route
// is now state-preserving, so `removed` should normally be empty.
export interface WorkspaceWipeResult {
  ok: boolean
  restart_required?: boolean
  redirect?: string
  removed?: string[]
  errors?: string[]
  error?: string
}

// shredWorkspace is kept for older clients. The server treats it as a
// non-destructive compatibility call and preserves local office state.
export function shredWorkspace() {
  return post<WorkspaceWipeResult>('/workspace/shred', {})
}
