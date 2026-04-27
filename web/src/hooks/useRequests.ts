import { useQuery } from '@tanstack/react-query'
import { getRequests, type AgentRequest } from '../api/client'
import { useAppStore } from '../stores/app'
import { useBrokerRefetchInterval } from './useBrokerEvents'
import { requestsKey } from '../lib/queryKeys'

export interface RequestsState {
  all: AgentRequest[]
  pending: AgentRequest[]
  blockingPending: AgentRequest | null
  isLoading: boolean
  error: unknown
}

export function useRequests(): RequestsState {
  const currentChannel = useAppStore((s) => s.currentChannel)
  const refetchInterval = useBrokerRefetchInterval(5000)
  const { data, isLoading, error } = useQuery({
    queryKey: requestsKey(currentChannel, true),
    queryFn: () => getRequests(currentChannel, true),
    refetchInterval,
    staleTime: 30_000,
  })

  const all = data?.requests ?? []
  const pending = all.filter((r) => !r.status || r.status === 'open' || r.status === 'pending')
  const blockingPending = pending.find((r) => r.blocking) ?? null

  return { all, pending, blockingPending, isLoading, error }
}
