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

export function deleteMessage(messageId: string, channel: string) {
  return del<DeleteMessageResponse>('/messages', {
    id: messageId,
    channel: channel || 'general',
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
  | 'gemini-vertex'
  | 'openclaude'

export type PerAgentProviderKind = ProviderKind
export type GlobalLLMProvider = Exclude<ProviderKind, 'openclaude'>

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
}

export function getRequests(channel: string) {
  return get<{ requests: AgentRequest[] }>('/requests', {
    channel: channel || 'general',
    viewer_slug: 'human',
  })
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

export function getTasks(channel: string, opts?: { includeDone?: boolean; status?: string; mySlug?: string }) {
  const params: Record<string, string> = { viewer_slug: 'human', channel: channel || 'general' }
  if (opts?.includeDone) params.include_done = 'true'
  if (opts?.status) params.status = opts.status
  if (opts?.mySlug) params.my_slug = opts.mySlug
  return get<{ tasks: Task[] }>('/tasks', params)
}

export function getOfficeTasks(opts?: { includeDone?: boolean; status?: string; mySlug?: string }) {
  const params: Record<string, string> = { viewer_slug: 'human', all_channels: 'true' }
  if (opts?.includeDone) params.include_done = 'true'
  if (opts?.status) params.status = opts.status
  if (opts?.mySlug) params.my_slug = opts.mySlug
  return get<{ tasks: Task[] }>('/tasks', params)
}

// ── Signals / Decisions / Watchdogs / Actions ──

export function getSignals() { return get('/signals') }
export function getDecisions() { return get('/decisions') }
export function getWatchdogs() { return get('/watchdogs') }
export function getActions() { return get('/actions') }

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
  next_run?: string
  last_run?: string
  due_at?: string
  status?: string
}

export function getScheduler(opts?: { dueOnly?: boolean }) {
  const params: Record<string, string> = {}
  if (opts?.dueOnly) params.due_only = 'true'
  return get<{ jobs: SchedulerJob[] }>('/scheduler', params)
}

// ── Skills ──

export interface Skill {
  id?: string
  name: string
  title?: string
  description?: string
  source?: string
  channel?: string
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

// ── Usage ──

export interface AgentUsage {
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cost_usd: number
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
  workflow_key?: string
  pipeline_id?: string
  workspace_path?: string
  worktree_path?: string
  worktree_branch?: string
  depends_on?: string[]
  updated_at?: string
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

export function getStudioDevConsole() {
  return get<StudioDevConsoleResponse>('/studio/dev-console')
}

export function runStudioDevConsoleAction(payload: StudioDevConsoleActionRequest) {
  return post<StudioDevConsoleActionResponse>('/studio/dev-console/action', payload)
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
  'gemini-vertex': 'Gemini (Vertex Credits)',
  openclaude: 'OpenClaude (Vertex)',
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
