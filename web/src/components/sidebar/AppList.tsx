import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { SECONDARY_APPS } from '../../lib/constants'
import { useAppStore } from '../../stores/app'
import { getDeliveries, type Delivery, type Task } from '../../api/client'
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
  const tasksQuery = useOfficeTasks({ includeDone: false, fallbackMs: 10_000 })
  const deliveriesQuery = useQuery({
    queryKey: deliveriesKey(false),
    queryFn: () => getDeliveries({ includeDone: false }),
    refetchInterval,
    staleTime: 30_000,
  })

  const waitingOnHumanCount = (tasksQuery.data?.tasks ?? []).filter(isHumanActionTask).length
  const deliveries = deliveriesQuery.data?.deliveries ?? []
  const waitingDeliveriesCount = deliveries.filter((delivery) => (delivery.pending_human_count ?? 0) > 0).length
  const blockedDeliveriesCount = deliveries.filter((delivery) => delivery.status === 'blocked').length
  const attentionDeliveriesCount = countAttentionDeliveries(deliveries)

  return (
    <div className={`sidebar-apps${compact ? ' sidebar-apps-compact' : ''}`}>
      {SECONDARY_APPS.map((app) => {
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
        }
        return (
          <button
            key={app.id}
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
      })}
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
