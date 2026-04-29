import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { CONDITIONAL_APPS, CORE_APPS, TOOL_APPS, type AppDefinition } from '../../lib/constants'
import { useAppStore } from '../../stores/app'
import { getDeliveries, type Delivery, type Task } from '../../api/client'
import { useRequests } from '../../hooks/useRequests'
import { useOfficeTasks } from '../../hooks/useTasks'
import { useBrokerRefetchInterval } from '../../hooks/useBrokerEvents'
import { deliveriesKey } from '../../lib/queryKeys'

type AppListProps = {
  compact?: boolean
}

export function AppList({ compact = false }: AppListProps) {
  const { t } = useTranslation()
  const currentApp = useAppStore((s) => s.currentApp)
  const setCurrentApp = useAppStore((s) => s.setCurrentApp)
  const refetchInterval = useBrokerRefetchInterval(10_000)
  const tasksQuery = useOfficeTasks({ includeDone: false, fallbackMs: 10_000, lite: true })
  const deliveriesQuery = useQuery({
    queryKey: deliveriesKey(false),
    queryFn: () => getDeliveries({ includeDone: false }),
    refetchInterval,
    staleTime: 30_000,
  })
  const requestsState = useRequests()

  const waitingOnHumanCount = (tasksQuery.data?.tasks ?? []).filter(isHumanActionTask).length
  const deliveries = deliveriesQuery.data?.deliveries ?? []
  const waitingDeliveriesCount = deliveries.filter((delivery) => (delivery.pending_human_count ?? 0) > 0).length
  const blockedDeliveriesCount = deliveries.filter((delivery) => delivery.status === 'blocked').length
  const attentionDeliveriesCount = countAttentionDeliveries(deliveries)
  const pendingRequestCount = requestsState.pending.length
  const visibleConditionalApps = CONDITIONAL_APPS.filter((app) =>
    app.id === 'requests' ? pendingRequestCount > 0 || currentApp === app.id : true
  )
  const selectedToolVisible = TOOL_APPS.some((app) => app.id === currentApp)
  const [toolsOpen, setToolsOpen] = useState(!compact || selectedToolVisible)

  useEffect(() => {
    if (!compact || selectedToolVisible) {
      setToolsOpen(true)
    }
  }, [compact, selectedToolVisible])

  const renderAppButton = (app: AppDefinition) => {
    let badge: number | null = null
    let badgeAria: string | null = null
    let badgeTitle: string | undefined
    let badgeClassName = 'sidebar-badge'

    if (app.id === 'tasks' && waitingOnHumanCount > 0) {
      badge = waitingOnHumanCount
      badgeAria = t('sidebar.apps.waitingAria', { count: badge })
      badgeTitle = badgeAria
      badgeClassName += ' sidebar-badge-waiting'
    } else if (app.id === 'deliveries' && attentionDeliveriesCount > 0) {
      badge = attentionDeliveriesCount
      badgeAria = t('sidebar.apps.attentionAria', { count: badge })
      badgeTitle = t('sidebar.apps.deliveriesBadgeTitle', {
        count: badge,
        waiting: waitingDeliveriesCount,
        blocked: blockedDeliveriesCount,
      })
      badgeClassName += blockedDeliveriesCount > 0 ? ' sidebar-badge-attention' : ' sidebar-badge-attention-soft'
    } else if (app.id === 'requests' && pendingRequestCount > 0) {
      badge = pendingRequestCount
      badgeAria = t('sidebar.apps.requestsAria', { count: badge })
      badgeTitle = badgeAria
      badgeClassName += ' sidebar-badge-waiting'
    }

    return (
      <button
        key={app.id}
        type="button"
        className={`sidebar-item${currentApp === app.id ? ' active' : ''}`}
        onClick={() => setCurrentApp(app.id)}
      >
        <span className="sidebar-item-emoji">{app.icon}</span>
        <span style={{ flex: 1 }}>{t(app.nameKey)}</span>
        {badge !== null && (
          <span
            className={badgeClassName}
            aria-label={badgeAria ?? t('sidebar.apps.pendingAria', { count: badge })}
            title={badgeTitle}
          >
            {badge}
          </span>
        )}
      </button>
    )
  }

  return (
    <div className={`sidebar-apps${compact ? ' sidebar-apps-compact' : ''}`}>
      {CORE_APPS.map(renderAppButton)}
      {visibleConditionalApps.map(renderAppButton)}
      <details
        className="sidebar-app-group"
        open={toolsOpen}
        onToggle={(event) => setToolsOpen(event.currentTarget.open)}
      >
        <summary className={`sidebar-app-group-summary${selectedToolVisible ? ' active' : ''}`}>
          <span className="sidebar-app-group-chevron">›</span>
          <span style={{ flex: 1 }}>{t('sidebar.apps.moreTools')}</span>
          <span className="sidebar-app-group-count">{TOOL_APPS.length}</span>
        </summary>
        <div className="sidebar-app-group-body">
          {TOOL_APPS.map(renderAppButton)}
        </div>
      </details>
    </div>
  )
}

function isHumanActionTask(task: Task): boolean {
  return Boolean(task.awaiting_human) || (task.task_type ?? '') === 'human_action'
}

function countAttentionDeliveries(deliveries: Delivery[]): number {
  const ids = new Set<string>()
  for (const delivery of deliveries) {
    if ((delivery.pending_human_count ?? 0) > 0 || delivery.status === 'blocked') {
      ids.add(delivery.id)
    }
  }
  return ids.size
}
