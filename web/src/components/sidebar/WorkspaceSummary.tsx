import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useOfficeMembers } from '../../hooks/useMembers'
import { getUsage } from '../../api/client'
import { useBrokerRefetchInterval } from '../../hooks/useBrokerEvents'
import { useOfficeTasks } from '../../hooks/useTasks'
import { usageKey } from '../../lib/queryKeys'

function formatTokens(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'k'
  return String(n)
}

/**
 * Small status line at the bottom of the sidebar. Mirrors the legacy
 * `renderWorkspaceSummary` output: active agents, open tasks, total tokens.
 */
export function WorkspaceSummary() {
  const { t } = useTranslation()
  const { data: members = [] } = useOfficeMembers()
  const usageInterval = useBrokerRefetchInterval(30_000)
  const { data: tasksData } = useOfficeTasks({ includeDone: false, fallbackMs: 30_000 })
  const { data: usage } = useQuery({
    queryKey: usageKey(),
    queryFn: () => getUsage(),
    refetchInterval: usageInterval,
    staleTime: 30_000,
  })

  const activeAgents = members.filter((m) => {
    if (!m.slug || m.slug === 'human' || m.slug === 'you') return false
    return (m.status || '').toLowerCase() === 'active'
  }).length

  const openTasks = (tasksData?.tasks ?? []).filter((t) => {
    const s = (t.status || '').toLowerCase()
    return s && s !== 'done' && s !== 'completed'
  }).length

  const parts: string[] = [
    t('sidebar.workspace.agentActive', { count: activeAgents }),
    t('sidebar.workspace.taskOpen', { count: openTasks }),
  ]
  const total = usage?.total?.total_tokens ?? 0
  if (total > 0) parts.push(t('sidebar.workspace.tokens', { tokens: formatTokens(total) }))

  const hint = openTasks > 0
    ? t('sidebar.workspace.taskInProgress', { count: openTasks })
    : t('sidebar.workspace.typeSlash')

  return (
    <>
      <div className="sidebar-summary">{parts.join(', ')}</div>
      <div className="sidebar-hint">{hint}</div>
    </>
  )
}
