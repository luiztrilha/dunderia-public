import { useQuery } from '@tanstack/react-query'
import { getOfficeTasks, getTasks } from '../api/client'
import { useBrokerRefetchInterval } from './useBrokerEvents'
import { channelTasksKey, officeTasksKey } from '../lib/queryKeys'

type TaskQueryOptions = {
  includeDone?: boolean
  fallbackMs?: number
  enabled?: boolean
  lite?: boolean
}

export function useOfficeTasks(options: TaskQueryOptions = {}) {
  const {
    includeDone = false,
    fallbackMs = 10_000,
    enabled = true,
    lite = false,
  } = options
  const refetchInterval = useBrokerRefetchInterval(fallbackMs)

  return useQuery({
    queryKey: officeTasksKey(includeDone, lite),
    queryFn: () => getOfficeTasks({ includeDone, lite }),
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
    lite = false,
  } = options
  const refetchInterval = useBrokerRefetchInterval(fallbackMs)

  return useQuery({
    queryKey: channelTasksKey(channel, includeDone, lite),
    queryFn: () => getTasks(channel, { includeDone, lite }),
    refetchInterval,
    staleTime: 30_000,
    enabled,
  })
}
