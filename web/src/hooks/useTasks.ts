import { useQuery } from '@tanstack/react-query'
import { getOfficeTasks, getTasks } from '../api/client'
import { useBrokerRefetchInterval } from './useBrokerEvents'
import { channelTasksKey, officeTasksKey } from '../lib/queryKeys'

type TaskQueryOptions = {
  includeDone?: boolean
  fallbackMs?: number
  enabled?: boolean
}

export function useOfficeTasks(options: TaskQueryOptions = {}) {
  const {
    includeDone = false,
    fallbackMs = 10_000,
    enabled = true,
  } = options
  const refetchInterval = useBrokerRefetchInterval(fallbackMs)

  return useQuery({
    queryKey: officeTasksKey(includeDone),
    queryFn: () => getOfficeTasks({ includeDone }),
    refetchInterval,
    staleTime: 30_000,
    enabled,
  })
}

export function useChannelTasks(channel: string, options: TaskQueryOptions = {}) {
  const {
    includeDone = false,
    fallbackMs = 5_000,
    enabled = true,
  } = options
  const refetchInterval = useBrokerRefetchInterval(fallbackMs)

  return useQuery({
    queryKey: channelTasksKey(channel, includeDone),
    queryFn: () => getTasks(channel, { includeDone }),
    refetchInterval,
    staleTime: 30_000,
    enabled,
  })
}
