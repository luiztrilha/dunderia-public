import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getMessages } from '../api/client'
import { buildAgentRuntimeSummary } from '../lib/agentRuntime'
import { useChannelMembers, useOfficeMembers } from './useMembers'
import { useBrokerRefetchInterval } from './useBrokerEvents'
import { runtimeSummaryKey } from '../lib/queryKeys'
import { useChannelTasks } from './useTasks'

export function useAgentRuntimeSummary(channel: string) {
  const { data: officeMembers = [] } = useOfficeMembers()
  const { data: channelMembers = [] } = useChannelMembers(channel)
  const { data: tasksData } = useChannelTasks(channel, { includeDone: false, fallbackMs: 5000 })

  const { data: messagesData } = useQuery({
    queryKey: runtimeSummaryKey('messages', channel),
    queryFn: () => getMessages(channel, { limit: 1 }),
    refetchInterval: useBrokerRefetchInterval(5000),
    staleTime: 30_000,
  })

  return useMemo(() => buildAgentRuntimeSummary({
    officeMembers,
    channelMembers,
    tasks: tasksData?.tasks ?? [],
    executionNodes: messagesData?.execution_nodes ?? [],
  }), [channelMembers, messagesData?.execution_nodes, officeMembers, tasksData?.tasks])
}
