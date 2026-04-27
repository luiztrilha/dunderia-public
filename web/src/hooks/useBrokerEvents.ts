import { useEffect, useMemo, useSyncExternalStore } from 'react'
import type { QueryClient, QueryKey } from '@tanstack/react-query'
import { sseURL, type BrokerEventActionPayload, type BrokerEventActivityPayload, type BrokerEventMessagePayload, type BrokerEventType, type BrokerOfficeChangedPayload } from '../api/client'
import { dispatchChannelMessagesRefresh } from '../lib/messageEvents'
import { deliveriesKey, normalizeChannelKey, officeMembersKey, channelsKey, officeTasksKey, requestsKey, runtimeSummaryKey, schedulerKey, usageKey } from '../lib/queryKeys'
import { useAppStore } from '../stores/app'

type BrokerEventsSnapshot = {
  connected: boolean
  pageVisible: boolean
}

const initialSnapshot: BrokerEventsSnapshot = {
  connected: false,
  pageVisible: typeof document === 'undefined' ? true : document.visibilityState !== 'hidden',
}

let snapshot = initialSnapshot
const listeners = new Set<() => void>()

function emit(next: Partial<BrokerEventsSnapshot>) {
  snapshot = { ...snapshot, ...next }
  listeners.forEach((listener) => listener())
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

function getSnapshot() {
  return snapshot
}

function invalidateMany(queryClient: QueryClient, keys: QueryKey[]) {
  for (const key of keys) {
    queryClient.invalidateQueries({ queryKey: key })
  }
}

function invalidateRequestQueries(queryClient: QueryClient, channel: string | null) {
  invalidateMany(queryClient, [channel ? requestsKey(channel) : ['requests']])
}

function invalidateRuntimeSummaryQueries(
  queryClient: QueryClient,
  channel: string | null,
  scopes: Array<'tasks' | 'messages'>,
) {
  if (!channel) {
    invalidateMany(queryClient, [['runtime-summary']])
    return
  }
  invalidateMany(queryClient, scopes.map((scope) => runtimeSummaryKey(scope, channel)))
}

function invalidateTaskQueries(queryClient: QueryClient) {
  invalidateMany(queryClient, [
    officeTasksKey(false),
    officeTasksKey(true),
    deliveriesKey(false),
    deliveriesKey(true),
    ['tasks'],
  ])
}

function invalidateSchedulerQueries(queryClient: QueryClient) {
  invalidateMany(queryClient, [schedulerKey(false), schedulerKey(true)])
}

function invalidateUsageQueries(queryClient: QueryClient) {
  invalidateMany(queryClient, [usageKey()])
}

function invalidateOfficeMetadataQueries(queryClient: QueryClient) {
  invalidateMany(queryClient, [
    officeMembersKey(),
    channelsKey(),
  ])
}

function payloadText(...parts: Array<string | undefined>): string {
  return parts.filter((part): part is string => typeof part === 'string' && part.trim() !== '').join(' ').toLowerCase()
}

function classifyAction(payload: BrokerEventActionPayload) {
  const action = payload.action ?? payload
  const text = payloadText(
    typeof action.kind === 'string' ? action.kind : undefined,
    typeof action.type === 'string' ? action.type : undefined,
    typeof action.action === 'string' ? action.action : undefined,
    typeof action.summary === 'string' ? action.summary : undefined,
    typeof action.related_id === 'string' ? action.related_id : undefined,
  )

  return {
    task: /task|lane|review|block|claim|complete|reassign|cancel|resume|worktree/.test(text),
    request: /request|interview|question|prompt|answer/.test(text),
    scheduler: /schedule|scheduler|cron|workflow|timer|timeout/.test(text),
    usage: /usage|token|cost/.test(text),
    logs: /log|receipt|trace/.test(text),
    policies: /policy/.test(text),
    skills: /skill/.test(text),
    studio: /studio|bootstrap|console/.test(text),
    members: /member|channel|office/.test(text),
  }
}

function classifyActivity(payload: BrokerEventActivityPayload) {
  const activity = payload.activity ?? payload
  const text = payloadText(
    typeof activity.kind === 'string' ? activity.kind : undefined,
    typeof activity.type === 'string' ? activity.type : undefined,
    typeof activity.summary === 'string' ? activity.summary : undefined,
  )

  return {
    task: /task|lane|review|block|claim|complete|reassign|cancel|resume|worktree/.test(text),
    scheduler: /schedule|scheduler|cron|workflow|timer|timeout/.test(text),
    usage: /usage|token|cost/.test(text),
    logs: /log|receipt|trace/.test(text),
  }
}

function handleMessageEvent(queryClient: QueryClient, payload: BrokerEventMessagePayload) {
  const message = payload.message ?? payload
  const channel = typeof message.channel === 'string' ? normalizeChannelKey(message.channel) : null

  invalidateMany(queryClient, [
    ['message-threads'],
    usageKey(),
    ['activity-usage'],
  ])

  invalidateRuntimeSummaryQueries(queryClient, channel, ['messages'])

  if (channel) {
    invalidateMany(queryClient, [
      ['messages', channel],
    ])
    dispatchChannelMessagesRefresh(channel)
  }

  const from = typeof message.from === 'string' ? message.from : ''
  if (from === 'human' || from === 'you') {
    invalidateRequestQueries(queryClient, channel)
  }
}

function handleActionEvent(queryClient: QueryClient, payload: BrokerEventActionPayload) {
  const action = payload.action ?? payload
  const channel = typeof action.channel === 'string' ? normalizeChannelKey(action.channel) : null
  const scope = classifyAction(payload)

  invalidateMany(queryClient, [
    ['activity-actions'],
    ['activity-decisions'],
    ['activity-watchdogs'],
  ])

  if (scope.task) {
    invalidateTaskQueries(queryClient)
    invalidateMany(queryClient, [['activity-tasks']])
    invalidateRuntimeSummaryQueries(queryClient, channel, ['tasks'])
  }

  if (scope.request) {
    invalidateRequestQueries(queryClient, channel)
    invalidateRuntimeSummaryQueries(queryClient, channel, ['messages'])
  }

  if (scope.scheduler) {
    invalidateSchedulerQueries(queryClient)
    invalidateMany(queryClient, [['activity-scheduler']])
  }

  if (scope.usage) {
    invalidateUsageQueries(queryClient)
    invalidateMany(queryClient, [['activity-usage']])
  }

  if (scope.logs) {
    invalidateMany(queryClient, [['agent-logs']])
  }

  if (scope.policies) {
    invalidateMany(queryClient, [['policies']])
  }

  if (scope.skills) {
    invalidateMany(queryClient, [['skills']])
  }

  if (scope.studio) {
    invalidateMany(queryClient, [
      ['studio-dev-console'],
      ['studio-bootstrap'],
    ])
  }

  if (scope.members) {
    invalidateOfficeMetadataQueries(queryClient)
  }

  if (!scope.task && !scope.request && !scope.scheduler && !scope.usage && !scope.logs && !scope.policies && !scope.skills && !scope.studio && !scope.members) {
    invalidateTaskQueries(queryClient)
    invalidateRequestQueries(queryClient, channel)
    invalidateSchedulerQueries(queryClient)
    invalidateUsageQueries(queryClient)
    invalidateMany(queryClient, [
      ['activity-tasks'],
      deliveriesKey(false),
      deliveriesKey(true),
      ['activity-scheduler'],
      ['activity-usage'],
      ['agent-logs'],
      ['policies'],
      ['skills'],
      ['studio-dev-console'],
      ['studio-bootstrap'],
    ])
    invalidateRuntimeSummaryQueries(queryClient, channel, ['tasks', 'messages'])
  }

  if (channel) {
    invalidateMany(queryClient, [
      ['messages', channel],
    ])
    dispatchChannelMessagesRefresh(channel)
  }
}

function handleActivityEvent(queryClient: QueryClient, payload: BrokerEventActivityPayload) {
  const activity = payload.activity ?? payload
  const channel = typeof activity.channel === 'string' ? normalizeChannelKey(activity.channel) : null
  const scope = classifyActivity(payload)

  invalidateMany(queryClient, [
    ['activity-actions'],
    ['activity-decisions'],
    ['activity-watchdogs'],
  ])

  if (scope.task) {
    invalidateMany(queryClient, [['activity-tasks']])
    invalidateRuntimeSummaryQueries(queryClient, channel, ['tasks'])
  }

  if (scope.scheduler) {
    invalidateMany(queryClient, [['activity-scheduler']])
    invalidateSchedulerQueries(queryClient)
  }

  if (scope.usage) {
    invalidateMany(queryClient, [['activity-usage']])
    invalidateUsageQueries(queryClient)
  }

  if (scope.logs) {
    invalidateMany(queryClient, [['agent-logs']])
  }

  invalidateMany(queryClient, [['studio-dev-console']])
}

function handleOfficeChanged(queryClient: QueryClient) {
  invalidateTaskQueries(queryClient)
  invalidateRequestQueries(queryClient, null)
  invalidateRuntimeSummaryQueries(queryClient, null, ['tasks', 'messages'])
  invalidateSchedulerQueries(queryClient)
  invalidateUsageQueries(queryClient)
  invalidateMany(queryClient, [
    officeMembersKey(),
    ['channel-members'],
    channelsKey(),
    ['messages'],
    ['message-threads'],
    ['agent-logs'],
    ['activity-actions'],
    ['activity-members'],
    ['activity-tasks'],
    deliveriesKey(false),
    deliveriesKey(true),
    ['activity-scheduler'],
    ['activity-usage'],
    ['policies'],
    ['skills'],
    ['studio-dev-console'],
    ['studio-bootstrap'],
  ])
}

function handleOfficeChangedPayload(queryClient: QueryClient, payload: BrokerOfficeChangedPayload) {
  handleOfficeChanged(queryClient)
  const kind = typeof payload.kind === 'string' ? payload.kind.trim().toLowerCase() : ''
  if (kind === 'state_restored') {
    invalidateMany(queryClient, [
      ['requests'],
      ['runtime-summary'],
      ['tasks'],
      ['messages'],
      ['message-threads'],
    ])
  }
}

function handleBrokerEvent(queryClient: QueryClient, eventName: BrokerEventType, rawData: string) {
  let payload: Record<string, unknown> = {}
  try {
    payload = JSON.parse(rawData || '{}') as Record<string, unknown>
  } catch {
    payload = {}
  }

  switch (eventName) {
    case 'ready':
      emit({ connected: true })
      return
    case 'message':
      handleMessageEvent(queryClient, payload as BrokerEventMessagePayload)
      return
    case 'action':
      handleActionEvent(queryClient, payload as BrokerEventActionPayload)
      return
    case 'activity':
      handleActivityEvent(queryClient, payload as BrokerEventActivityPayload)
      return
    case 'office_changed':
      handleOfficeChangedPayload(queryClient, payload as BrokerOfficeChangedPayload)
      return
    default:
      return
  }
}

export function useBrokerEventsState() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

export function useBrokerRefetchInterval(fallbackMs: number) {
  const { connected, pageVisible } = useBrokerEventsState()
  return useMemo(() => {
    if (connected) return false
    if (!pageVisible) return Math.max(fallbackMs * 4, 30_000)
    return fallbackMs
  }, [connected, fallbackMs, pageVisible])
}

export function useBrokerEventBridge(queryClient: QueryClient) {
  const brokerConnected = useAppStore((s) => s.brokerConnected)

  useEffect(() => {
    if (typeof document === 'undefined') {
      emit({ pageVisible: true })
      return
    }

    const handleVisibility = () => {
      emit({ pageVisible: document.visibilityState !== 'hidden' })
    }

    handleVisibility()
    document.addEventListener('visibilitychange', handleVisibility)
    return () => {
      document.removeEventListener('visibilitychange', handleVisibility)
    }
  }, [])

  useEffect(() => {
    if (!brokerConnected || typeof window === 'undefined') {
      emit({ connected: false })
      return
    }

    const source = new EventSource(sseURL('/events'))

    const onReady = (event: MessageEvent<string>) => {
      handleBrokerEvent(queryClient, 'ready', event.data)
    }
    const onMessage = (event: MessageEvent<string>) => {
      handleBrokerEvent(queryClient, 'message', event.data)
    }
    const onAction = (event: MessageEvent<string>) => {
      handleBrokerEvent(queryClient, 'action', event.data)
    }
    const onActivity = (event: MessageEvent<string>) => {
      handleBrokerEvent(queryClient, 'activity', event.data)
    }
    const onOfficeChanged = (event: MessageEvent<string>) => {
      handleBrokerEvent(queryClient, 'office_changed', event.data)
    }

    source.addEventListener('ready', onReady as EventListener)
    source.addEventListener('message', onMessage as EventListener)
    source.addEventListener('action', onAction as EventListener)
    source.addEventListener('activity', onActivity as EventListener)
    source.addEventListener('office_changed', onOfficeChanged as EventListener)

    source.onopen = () => {
      emit({ connected: true })
    }
    source.onerror = () => {
      emit({ connected: false })
    }

    return () => {
      source.removeEventListener('ready', onReady as EventListener)
      source.removeEventListener('message', onMessage as EventListener)
      source.removeEventListener('action', onAction as EventListener)
      source.removeEventListener('activity', onActivity as EventListener)
      source.removeEventListener('office_changed', onOfficeChanged as EventListener)
      source.close()
      emit({ connected: false })
    }
  }, [brokerConnected, queryClient])
}
