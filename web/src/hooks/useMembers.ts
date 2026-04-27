import { useQuery } from '@tanstack/react-query'
import { getOfficeMembers, getMembers } from '../api/client'
import type { OfficeMember } from '../api/client'
import { useBrokerRefetchInterval } from './useBrokerEvents'
import { channelMembersKey, officeMembersKey } from '../lib/queryKeys'

export function useOfficeMembers() {
  const refetchInterval = useBrokerRefetchInterval(5000)
  return useQuery({
    queryKey: officeMembersKey(),
    queryFn: () => getOfficeMembers(),
    refetchInterval,
    staleTime: 30_000,
    select: (data) => data.members ?? [],
  })
}

export function useChannelMembers(channel: string) {
  const refetchInterval = useBrokerRefetchInterval(5000)
  return useQuery({
    queryKey: channelMembersKey(channel),
    queryFn: () => getMembers(channel),
    refetchInterval,
    staleTime: 30_000,
    select: (data) => data.members ?? [],
  })
}

export type { OfficeMember }
