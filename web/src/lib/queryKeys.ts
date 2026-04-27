import type { QueryKey } from '@tanstack/react-query'

export function normalizeChannelKey(channel?: string | null): string {
  const trimmed = channel?.trim()
  return trimmed ? trimmed : 'general'
}

export function officeTasksKey(includeDone = false): QueryKey {
  return ['office-tasks', includeDone ? 'include-done' : 'active-only']
}

export function channelTasksKey(channel: string, includeDone = false): QueryKey {
  return ['tasks', normalizeChannelKey(channel), includeDone ? 'include-done' : 'active-only']
}

export function requestsKey(channel?: string | null): QueryKey {
  return ['requests', normalizeChannelKey(channel)]
}

export function deliveriesKey(includeDone = false): QueryKey {
  return ['deliveries', includeDone ? 'include-done' : 'active-only']
}

export function runtimeSummaryKey(scope: 'tasks' | 'messages', channel?: string | null): QueryKey {
  return ['runtime-summary', scope, normalizeChannelKey(channel)]
}

export function officeMembersKey(): QueryKey {
  return ['office-members']
}

export function channelMembersKey(channel: string): QueryKey {
  return ['channel-members', normalizeChannelKey(channel)]
}

export function channelsKey(): QueryKey {
  return ['channels']
}

export function schedulerKey(dueOnly = false): QueryKey {
  return ['scheduler', dueOnly ? 'due-only' : 'all']
}

export function usageKey(): QueryKey {
  return ['usage']
}
