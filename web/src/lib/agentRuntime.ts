import type { ExecutionNode, OfficeMember, Task } from '../api/client'

export type AgentRuntimeState = 'working' | 'blocked' | 'waiting' | 'silent'

export interface AgentRuntimeMember extends OfficeMember {
  runtimeState: AgentRuntimeState
  runtimeDetail: string
}

export interface AgentRuntimeSummary {
  members: AgentRuntimeMember[]
  counts: Record<AgentRuntimeState, number>
}

function isHumanSlug(slug: string | undefined): boolean {
  const normalized = (slug || '').trim().toLowerCase()
  return normalized === '' || normalized === 'human' || normalized === 'you'
}

function normalizeStatus(value: string | undefined): string {
  return (value || '').trim().toLowerCase()
}

function normalizeLiveness(value: string | undefined): string {
  return (value || '').trim().toLowerCase()
}

function livenessDetail(member: OfficeMember): string {
  return member.liveness_reason || member.detail || member.liveActivity || 'Runtime needs follow-up'
}

function buildBlockedTaskMap(tasks: Task[]): Map<string, Task> {
  const latestTaskByOwner = new Map<string, Task>()
  const latestTaskUpdatedAtByOwner = new Map<string, number>()

  for (const task of tasks) {
    const owner = (task.owner || '').trim()
    if (!owner) continue

    const updatedAt = parseTaskUpdatedAt(task)
    const currentTs = latestTaskUpdatedAtByOwner.get(owner)
    if (currentTs === undefined || updatedAt > currentTs) {
      latestTaskByOwner.set(owner, task)
      latestTaskUpdatedAtByOwner.set(owner, updatedAt)
    }
  }

  const blockedByOwner = new Map<string, Task>()
  for (const [owner, task] of latestTaskByOwner) {
    const status = normalizeStatus(task.status)
    if (task.blocked || status === 'blocked') {
      blockedByOwner.set(owner, task)
    }
  }

  return blockedByOwner
}

function buildWaitingNodeMap(nodes: ExecutionNode[]): Map<string, ExecutionNode> {
  const waitingByOwner = new Map<string, ExecutionNode>()
  const latestWaitingByOwner = new Map<string, number>()
  for (const node of nodes) {
    const owner = (node.owner_agent || '').trim()
    if (!owner) continue
    if (node.awaiting_human_input) {
      const ts = parseExecutionNodeUpdatedAt(node)
      if (!latestWaitingByOwner.has(owner) || ts > (latestWaitingByOwner.get(owner) || 0)) {
        waitingByOwner.set(owner, node)
        latestWaitingByOwner.set(owner, ts)
      }
    }
  }
  return waitingByOwner
}

function buildTimedOutNodeMap(nodes: ExecutionNode[]): Map<string, ExecutionNode> {
  const timedOutByOwner = new Map<string, ExecutionNode>()
  const latestTimedOutByOwner = new Map<string, number>()
  for (const node of nodes) {
    const owner = (node.owner_agent || '').trim()
    if (!owner) continue
    if (normalizeStatus(node.status) === 'timed_out') {
      const ts = parseExecutionNodeUpdatedAt(node)
      if (!latestTimedOutByOwner.has(owner) || ts > (latestTimedOutByOwner.get(owner) || 0)) {
        timedOutByOwner.set(owner, node)
        latestTimedOutByOwner.set(owner, ts)
      }
    }
  }
  return timedOutByOwner
}

function parseExecutionNodeUpdatedAt(node: ExecutionNode): number {
  const updated = ((node.updated_at || '').trim())
    || ((node.created_at || '').trim())
  if (!updated) return 0
  const ts = Date.parse(updated)
  return Number.isNaN(ts) ? 0 : ts
}

function parseTaskUpdatedAt(task: Task): number {
  const updated = ((task.updated_at || '').trim()) || ((task.created_at || '').trim())
  if (!updated) return 0
  const ts = Date.parse(updated)
  return Number.isNaN(ts) ? 0 : ts
}

function mergeOfficeAndChannelMembers(officeMembers: OfficeMember[], channelMembers: OfficeMember[]): OfficeMember[] {
  const runtimeBySlug = new Map<string, OfficeMember>()
  for (const member of channelMembers) {
    const slug = (member.slug || '').trim()
    if (!slug || isHumanSlug(slug)) continue
    runtimeBySlug.set(slug, member)
  }

  const merged: OfficeMember[] = []
  const seen = new Set<string>()

  for (const member of officeMembers) {
    const slug = (member.slug || '').trim()
    if (!slug || isHumanSlug(slug) || seen.has(slug)) continue
    seen.add(slug)
    merged.push({ ...member, ...runtimeBySlug.get(slug) })
  }

  for (const member of channelMembers) {
    const slug = (member.slug || '').trim()
    if (!slug || isHumanSlug(slug) || seen.has(slug)) continue
    seen.add(slug)
    merged.push(member)
  }

  return merged
}

export function buildAgentRuntimeSummary(input: {
  officeMembers: OfficeMember[]
  channelMembers: OfficeMember[]
  tasks: Task[]
  executionNodes: ExecutionNode[]
}): AgentRuntimeSummary {
  const mergedMembers = mergeOfficeAndChannelMembers(input.officeMembers, input.channelMembers)
  const blockedByOwner = buildBlockedTaskMap(input.tasks)
  const waitingByOwner = buildWaitingNodeMap(input.executionNodes)
  const timedOutByOwner = buildTimedOutNodeMap(input.executionNodes)

  const members: AgentRuntimeMember[] = mergedMembers.map((member) => {
    const slug = (member.slug || '').trim()
    const blockedTask = blockedByOwner.get(slug)
    const waitingNode = waitingByOwner.get(slug)
    const timedOutNode = timedOutByOwner.get(slug)
    const liveness = normalizeLiveness(member.liveness_state)

    if (blockedTask) {
      return {
        ...member,
        runtimeState: 'blocked',
        runtimeDetail: blockedTask.title || blockedTask.details || 'Blocked task',
      }
    }
    if (timedOutNode) {
      return {
        ...member,
        runtimeState: 'blocked',
        runtimeDetail: timedOutNode.last_error || 'Timed out waiting for update',
      }
    }
    if (waitingNode) {
      return {
        ...member,
        runtimeState: 'waiting',
        runtimeDetail: waitingNode.awaiting_human_reason || 'Waiting for your reply',
      }
    }
    if (liveness === 'plan_only' || liveness === 'empty_response' || liveness === 'failed') {
      return {
        ...member,
        runtimeState: 'blocked',
        runtimeDetail: livenessDetail(member),
      }
    }
    if (liveness === 'blocked') {
      return {
        ...member,
        runtimeState: 'waiting',
        runtimeDetail: livenessDetail(member),
      }
    }
    if (normalizeStatus(member.status) === 'active') {
      return {
        ...member,
        runtimeState: 'working',
        runtimeDetail: member.detail || member.liveActivity || member.task || 'Working now',
      }
    }
    return {
      ...member,
      runtimeState: 'silent',
      runtimeDetail: member.detail || member.liveActivity || 'Waiting for direction',
    }
  })

  const counts: Record<AgentRuntimeState, number> = {
    working: 0,
    blocked: 0,
    waiting: 0,
    silent: 0,
  }
  for (const member of members) {
    counts[member.runtimeState] += 1
  }

  return { members, counts }
}
