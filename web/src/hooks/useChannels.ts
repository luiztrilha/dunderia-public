import { useQuery } from '@tanstack/react-query'
import { getChannels } from '../api/client'
import type { Channel } from '../api/client'
import { useBrokerRefetchInterval } from './useBrokerEvents'
import { channelsKey } from '../lib/queryKeys'

export function useChannels(enabled = true) {
  const refetchInterval = useBrokerRefetchInterval(10_000)
  return useQuery({
    queryKey: channelsKey(),
    queryFn: () => getChannels(),
    enabled,
    refetchInterval,
    staleTime: 30_000,
    select: (data) => data.channels ?? [],
  })
}

export type { Channel }
