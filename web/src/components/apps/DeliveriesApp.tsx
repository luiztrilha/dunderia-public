import { useMemo, useState, type Dispatch, type SetStateAction } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getDeliveries, type Delivery } from '../../api/client'
import { useBrokerRefetchInterval } from '../../hooks/useBrokerEvents'
import { deliveriesKey } from '../../lib/queryKeys'

type DeliveryRepositoryGroup = {
  key: string
  label: string
  basePath?: string
  deliveries: Delivery[]
  latestTimestamp: number
  blockedCount: number
  waitingHumanCount: number
}

type DeliverySection = {
  groups: DeliveryRepositoryGroup[]
  orderedDeliveries: Delivery[]
}

const repositoryFamilyPattern = /^(LegacyWeb|legacyticketweb|publicportalweb|sharedsystemswebforms|integrationapi|dunderia|superpowers|relatorios|memory|scripts|codex-lb|vibeyard)/i

function normalizeStatus(status?: string): string {
  return (status || 'in_progress').replace(/_/g, ' ')
}

function formatTime(raw?: string): string {
  if (!raw) return 'n/a'
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return raw
  return date.toLocaleString()
}

function timestampValue(raw?: string): number {
  if (!raw) return 0
  const date = new Date(raw)
  return Number.isNaN(date.getTime()) ? 0 : date.getTime()
}

function deliveryActivityTimestamp(delivery: Delivery): number {
  return timestampValue(delivery.last_substantive_update_at)
}

function normalizeLocalPath(raw?: string): string {
  return (raw || '').trim().replace(/^["'`]+|["'`]+$/g, '')
}

function isLikelyDunderiaWorktree(path?: string): boolean {
  const normalized = normalizeLocalPath(path).toLowerCase()
  return normalized.includes('\\task-worktrees\\dunderia\\') || normalized.includes('/task-worktrees/dunderia/')
}

function displayLocalPath(raw?: string): string {
  return normalizeLocalPath(raw).replace(/\//g, '\\')
}

function normalizeRepositoryToken(value: string): string {
  return value.toLowerCase().replace(/[^a-z0-9]+/g, '')
}

function repositoryPriority(label?: string): number {
  const token = normalizeRepositoryToken(label || '')
  if (!token) return 0
  if (
    token.startsWith('legacyweb')
    || token.startsWith('legacyticketweb')
    || token.startsWith('publicportalweb')
    || token.startsWith('sharedsystemswebforms')
    || token.startsWith('integrationapi')
  ) {
    return 300
  }
  if (token === 'dunderia' || token === 'superpowers' || token === 'codexlb' || token === 'vibeyard') {
    return 180
  }
  if (token === 'scripts' || token === 'temp' || token === 'memory' || token === 'relatorios') {
    return 40
  }
  return 120
}

function looksLikeRepositoryFamily(value: string): boolean {
  return repositoryFamilyPattern.test(value)
}

function repositoryFromRepoSegments(segments: string[], drivePrefix: string): { key: string; label: string; basePath?: string } | null {
  const trimmedSegments = segments.map((segment) => segment.trim()).filter(Boolean)
  if (trimmedSegments.length === 0) {
    return null
  }

  const operationalTailMarkers = new Set([
    'app',
    'app_data',
    'api',
    'backend',
    'bin',
    'ExampleBank',
    'ExampleBankinterno',
    'build',
    'debug',
    'dist',
    'frontend',
    'obj',
    'packages',
    'release',
    'src',
    'temp',
    'wwwroot',
    'wsconvenio',
  ])

  let end = trimmedSegments.length
  while (end > 1 && operationalTailMarkers.has(trimmedSegments[end - 1].toLowerCase())) {
    end -= 1
  }
  const candidates = trimmedSegments.slice(0, end)
  if (candidates.length === 0) {
    return null
  }

  const firstCandidate = candidates[0]
  let bestCandidate = firstCandidate
  const firstFamily = looksLikeRepositoryFamily(firstCandidate) ? firstCandidate.match(repositoryFamilyPattern)?.[0].toLowerCase() ?? '' : ''

  if (firstFamily) {
    for (const candidate of candidates.slice(1)) {
      if (candidate.toLowerCase().startsWith(firstFamily)) {
        bestCandidate = candidate
      }
    }
  }

  const bestIndex = candidates.findIndex((candidate) => candidate === bestCandidate)
  const baseSegments = bestIndex >= 0 ? candidates.slice(0, bestIndex + 1) : [bestCandidate]
  const basePath = displayLocalPath(`${drivePrefix}\\${baseSegments.join('\\')}`)

  return { key: `repo:${normalizeRepositoryToken(bestCandidate)}`, label: bestCandidate, basePath }
}

function repositoryFromPath(raw?: string): { key: string; label: string; basePath?: string } | null {
  const normalized = normalizeLocalPath(raw)
  if (!normalized || !/^[a-zA-Z]:[\\/]/.test(normalized)) {
    return null
  }

  const segments = normalized
    .split(/[\\/]+/)
    .map((segment) => segment.trim())
    .filter(Boolean)

  if (segments.length === 0) {
    return null
  }

  const normalizedSegments = segments.map((segment) => segment.toLowerCase())
  const taskWorktreesIndex = normalizedSegments.findIndex((segment) => segment === 'task-worktrees')
  if (taskWorktreesIndex >= 0 && segments[taskWorktreesIndex + 1]) {
    const repoName = segments[taskWorktreesIndex + 1]
    const basePath = displayLocalPath(segments.slice(0, taskWorktreesIndex + 2).join('\\'))
    return { key: `repo:${normalizeRepositoryToken(repoName)}`, label: repoName, basePath }
  }

  const repoRootMarkers = new Set(['repos', 'repositórios', 'repositorios', 'repository', 'repositories', 'workspace-repos'])
  const repoRootIndex = normalizedSegments.findIndex((segment) => repoRootMarkers.has(segment))
  if (repoRootIndex >= 0) {
    const drivePrefix = segments[0] || normalized.slice(0, 2)
    return repositoryFromRepoSegments(segments.slice(repoRootIndex + 1), drivePrefix)
  }

  return null
}

function resolveDeliveryRepository(delivery: Delivery, unknownLabel: string): { key: string; label: string; basePath?: string } {
  const candidates: Array<{ score: number; repository: { key: string; label: string; basePath?: string } }> = []
  const pushCandidate = (path: string | undefined, source: 'direct' | 'workspace' | 'worktree' | 'other') => {
    const repository = repositoryFromPath(path)
    if (!repository) {
      return
    }
    let score = repositoryPriority(repository.label)
    switch (source) {
      case 'direct':
        score += 1000
        break
      case 'workspace':
        score += 500
        break
      case 'worktree':
        score += isLikelyDunderiaWorktree(path) ? 420 : 120
        break
      default:
        score += 40
        break
    }
    candidates.push({ score, repository })
  }

  pushCandidate(delivery.workspace_path, 'direct')

  for (const artifact of delivery.artifacts ?? []) {
    const source =
      artifact.kind === 'workspace'
        ? 'workspace'
        : artifact.kind === 'worktree'
          ? 'worktree'
          : 'other'
    pushCandidate(artifact.path, source)
  }

  candidates.sort((left, right) => right.score - left.score || left.repository.label.localeCompare(right.repository.label))
  if (candidates[0]) {
    return candidates[0].repository
  }

  return { key: 'repo:unknown', label: unknownLabel }
}

function badgeClass(status?: string): string {
  switch ((status || '').toLowerCase()) {
    case 'done':
      return 'badge badge-green'
    case 'blocked':
      return 'badge badge-attention'
    case 'awaiting_human':
      return 'badge badge-waiting'
    default:
      return 'badge badge-accent'
  }
}

function deliveryCardClass(delivery: Delivery, active: boolean): string {
  let className = 'app-card delivery-card'
  if ((delivery.pending_human_count ?? 0) > 0) className += ' app-card-waiting'
  if (delivery.status === 'blocked' || (delivery.blocker_count ?? 0) > 0) className += ' app-card-attention'
  if (delivery.status === 'done' || delivery.status === 'canceled') className += ' delivery-card-done'
  if (active) className += ' app-card-active delivery-card-active'
  return className
}

function isHistoricalDelivery(delivery: Delivery): boolean {
  const status = (delivery.status || '').toLowerCase()
  return status === 'done' || status === 'canceled'
}

function groupDeliveriesByRepository(deliveries: Delivery[], unknownLabel: string): DeliverySection {
  const groups = new Map<string, DeliveryRepositoryGroup>()

  for (const delivery of deliveries) {
    const repository = resolveDeliveryRepository(delivery, unknownLabel)
    const existing = groups.get(repository.key)
    const deliveryTimestamp = deliveryActivityTimestamp(delivery)
    if (existing) {
      existing.deliveries.push(delivery)
      existing.latestTimestamp = Math.max(existing.latestTimestamp, deliveryTimestamp)
      if (delivery.status === 'blocked' || (delivery.blocker_count ?? 0) > 0) existing.blockedCount += 1
      if ((delivery.pending_human_count ?? 0) > 0) existing.waitingHumanCount += 1
      continue
    }
    groups.set(repository.key, {
      key: repository.key,
      label: repository.label,
      basePath: repository.basePath,
      deliveries: [delivery],
      latestTimestamp: deliveryTimestamp,
      blockedCount: delivery.status === 'blocked' || (delivery.blocker_count ?? 0) > 0 ? 1 : 0,
      waitingHumanCount: (delivery.pending_human_count ?? 0) > 0 ? 1 : 0,
    })
  }

  const normalizedGroups = Array.from(groups.values())
    .map((group) => ({
      ...group,
      deliveries: [...group.deliveries].sort((left, right) => {
        const timestampDelta = deliveryActivityTimestamp(right) - deliveryActivityTimestamp(left)
        if (timestampDelta !== 0) return timestampDelta
        return left.title.localeCompare(right.title) || left.id.localeCompare(right.id)
      }),
    }))
    .sort((left, right) => {
      const timestampDelta = right.latestTimestamp - left.latestTimestamp
      if (timestampDelta !== 0) return timestampDelta
      return left.label.localeCompare(right.label)
    })

  return {
    groups: normalizedGroups,
    orderedDeliveries: normalizedGroups.flatMap((group) => group.deliveries),
  }
}

function renderDeliveryGroups(
  groups: DeliveryRepositoryGroup[],
  selected: Delivery | null,
  collapsedGroups: Record<string, boolean>,
  setCollapsedGroups: Dispatch<SetStateAction<Record<string, boolean>>>,
  setSelectedId: Dispatch<SetStateAction<string | null>>,
  t: ReturnType<typeof useTranslation>['t'],
  keyPrefix = '',
) {
  return groups.map((group) => {
    const scopedKey = `${keyPrefix}${group.key}`
    const hasSelectedDelivery = group.deliveries.some((delivery) => delivery.id === selected?.id)
    const isCollapsed = collapsedGroups[scopedKey] ?? true
    const isExpanded = hasSelectedDelivery || !isCollapsed

    return (
      <section key={scopedKey} className="delivery-group">
        <button
          type="button"
          className={`app-card delivery-group-header${isExpanded ? ' delivery-group-header-open' : ''}`}
          onClick={() => setCollapsedGroups((current) => ({ ...current, [scopedKey]: !(current[scopedKey] ?? true) }))}
          aria-expanded={isExpanded}
        >
          <div style={{ display: 'flex', alignItems: 'flex-start', gap: 10, minWidth: 0 }}>
            <span className={`delivery-group-chevron${isExpanded ? ' open' : ''}`}>▸</span>
            <div className="delivery-group-copy">
              <div className="app-card-title" style={{ marginBottom: 0 }}>{group.label}</div>
              {group.basePath ? (
                <div className="delivery-group-path" title={group.basePath}>{group.basePath}</div>
              ) : null}
              <div className="delivery-group-meta">
                <span>{t('apps.deliveries.groups.deliveryCount', { count: group.deliveries.length })}</span>
                <span>{t('apps.deliveries.groups.latestUpdate', { time: formatTime(group.deliveries[0]?.last_substantive_update_at) })}</span>
              </div>
            </div>
          </div>
          <div className="delivery-group-badges">
            {group.waitingHumanCount > 0 && (
              <span className="badge badge-waiting">{t('apps.deliveries.labels.waitingHuman', { count: group.waitingHumanCount })}</span>
            )}
            {group.blockedCount > 0 && (
              <span className="badge badge-attention">{t('apps.deliveries.labels.blockers', { count: group.blockedCount })}</span>
            )}
          </div>
        </button>

        {isExpanded ? (
          <div className="delivery-group-list">
            {group.deliveries.map((delivery) => {
              const active = selected?.id === delivery.id
              return (
                <button
                  key={delivery.id}
                  type="button"
                  className={deliveryCardClass(delivery, active)}
                  onClick={() => setSelectedId(delivery.id)}
                  style={{ textAlign: 'left' }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6, flexWrap: 'wrap' }}>
                    <span className={badgeClass(delivery.status)}>{normalizeStatus(delivery.status)}</span>
                    {(delivery.pending_human_count ?? 0) > 0 && (
                      <span className="badge badge-waiting">{t('apps.deliveries.labels.waitingHuman', { count: delivery.pending_human_count ?? 0 })}</span>
                    )}
                    {(delivery.blocker_count ?? 0) > 0 && (
                      <span className="badge badge-attention">{t('apps.deliveries.labels.blockers', { count: delivery.blocker_count ?? 0 })}</span>
                    )}
                  </div>
                  <div className="app-card-title" style={{ marginBottom: 4 }}>{delivery.title}</div>
                  {delivery.summary && (
                    <div className="delivery-card-summary">
                      {delivery.summary}
                    </div>
                  )}
                  <div className="delivery-card-footer">
                    <div className="app-card-meta">
                      {[delivery.channel ? `#${delivery.channel}` : '', delivery.owner ? `@${delivery.owner}` : ''].filter(Boolean).join(' · ') || '—'}
                    </div>
                    <div className="delivery-card-progress">
                      <div style={{ fontSize: 13, fontWeight: 700 }}>{delivery.progress_percent ?? 0}%</div>
                      <div className="app-card-meta">{delivery.progress_basis || t('apps.deliveries.labels.noMilestones')}</div>
                    </div>
                  </div>
                </button>
              )
            })}
          </div>
        ) : null}
      </section>
    )
  })
}

export function DeliveriesApp() {
  const { t } = useTranslation()
  const refetchInterval = useBrokerRefetchInterval(15_000)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [collapsedGroups, setCollapsedGroups] = useState<Record<string, boolean>>({})
  const [historyCollapsed, setHistoryCollapsed] = useState(true)
  const query = useQuery({
    queryKey: deliveriesKey(true),
    queryFn: () => getDeliveries({ includeDone: true }),
    refetchInterval,
    staleTime: 30_000,
  })

  const deliveries = query.data?.deliveries ?? []
  const { activeSection, historySection } = useMemo(() => {
    const unknownLabel = t('apps.deliveries.groups.unknownRepository')
    const activeDeliveries = deliveries.filter((delivery) => !isHistoricalDelivery(delivery))
    const historicalDeliveries = deliveries.filter((delivery) => isHistoricalDelivery(delivery))
    return {
      activeSection: groupDeliveriesByRepository(activeDeliveries, unknownLabel),
      historySection: groupDeliveriesByRepository(historicalDeliveries, unknownLabel),
    }
  }, [deliveries, t])

  const orderedDeliveries = useMemo(
    () => [...activeSection.orderedDeliveries, ...historySection.orderedDeliveries],
    [activeSection.orderedDeliveries, historySection.orderedDeliveries],
  )
  const selected = useMemo(() => {
    if (orderedDeliveries.length === 0) return null
    if (selectedId) return orderedDeliveries.find((item) => item.id === selectedId) ?? orderedDeliveries[0]
    return activeSection.orderedDeliveries[0] ?? historySection.orderedDeliveries[0] ?? null
  }, [orderedDeliveries, selectedId])

  const activeCount = deliveries.filter((item) => item.status !== 'done' && item.status !== 'canceled').length
  const waitingHumanCount = deliveries.filter((item) => (item.pending_human_count ?? 0) > 0).length
  const blockedCount = deliveries.filter((item) => item.status === 'blocked' || (item.blocker_count ?? 0) > 0).length

  if (query.isLoading) {
    return (
      <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
        {t('apps.deliveries.loading')}
      </div>
    )
  }

  if (query.error) {
    return (
      <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
        {t('apps.deliveries.loadFailed')}
      </div>
    )
  }

  return (
    <div className="deliveries-app" style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 16, flexWrap: 'wrap' }}>
        <div>
          <h3 style={{ fontSize: 18, fontWeight: 700 }}>{t('apps.deliveries.title')}</h3>
          <div style={{ fontSize: 13, color: 'var(--text-secondary)', marginTop: 4 }}>
            {t('apps.deliveries.subtitle')}
          </div>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(120px, 1fr))', gap: 8, minWidth: 360 }}>
          <StatCard label={t('apps.deliveries.stats.active')} value={String(activeCount)} />
          <StatCard label={t('apps.deliveries.stats.waitingHuman')} value={String(waitingHumanCount)} tone="waiting" />
          <StatCard label={t('apps.deliveries.stats.blocked')} value={String(blockedCount)} tone="attention" />
        </div>
      </div>

      {deliveries.length === 0 ? (
        <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
          {t('apps.deliveries.empty')}
        </div>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'minmax(340px, 0.95fr) minmax(0, 1.05fr)', gap: 16 }}>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {activeSection.groups.length > 0 ? (
              <>
                <div className="delivery-section-header">
                  <div className="delivery-section-title">{t('apps.deliveries.sections.active')}</div>
                  <div className="app-card-meta">{t('apps.deliveries.groups.deliveryCount', { count: activeSection.orderedDeliveries.length })}</div>
                </div>
                {renderDeliveryGroups(activeSection.groups, selected, collapsedGroups, setCollapsedGroups, setSelectedId, t, 'active:')}
              </>
            ) : null}

            {historySection.groups.length > 0 ? (
              <section className="delivery-history">
                <button
                  type="button"
                  className="app-card delivery-history-toggle"
                  onClick={() => setHistoryCollapsed((value) => !value)}
                  aria-expanded={!historyCollapsed}
                >
                  <div style={{ display: 'flex', alignItems: 'center', gap: 10, minWidth: 0 }}>
                    <span className={`delivery-group-chevron${historyCollapsed ? '' : ' open'}`}>▸</span>
                    <div className="delivery-group-copy">
                      <div className="delivery-section-title">{t('apps.deliveries.sections.history')}</div>
                      <div className="delivery-group-meta">
                        <span>{t('apps.deliveries.groups.deliveryCount', { count: historySection.orderedDeliveries.length })}</span>
                        <span>{t('apps.deliveries.sections.historyHint')}</span>
                      </div>
                    </div>
                  </div>
                </button>
                {!historyCollapsed ? renderDeliveryGroups(historySection.groups, selected, collapsedGroups, setCollapsedGroups, setSelectedId, t, 'history:') : null}
              </section>
            ) : null}
          </div>

          <div className={`app-card delivery-detail-card${selected && ((selected.pending_human_count ?? 0) > 0) ? ' app-card-waiting' : ''}${selected && (selected.status === 'blocked' || (selected.blocker_count ?? 0) > 0) ? ' app-card-attention' : ''}`} style={{ minHeight: 420 }}>
            {selected ? <DeliveryDetail delivery={selected} /> : null}
          </div>
        </div>
      )}
    </div>
  )
}

function StatCard({ label, value, tone = 'neutral' }: { label: string; value: string; tone?: 'neutral' | 'waiting' | 'attention' }) {
  const className =
    'app-card delivery-stat-card'
    + (tone === 'waiting' ? ' app-card-waiting' : '')
    + (tone === 'attention' ? ' app-card-attention' : '')
  return (
    <div className={className} style={{ padding: '12px 14px' }}>
      <div style={{ fontSize: 10, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-tertiary)' }}>
        {label}
      </div>
      <div style={{ fontSize: 22, fontWeight: 700, marginTop: 4 }}>{value}</div>
    </div>
  )
}

function DeliveryDetail({ delivery }: { delivery: Delivery }) {
  const { t } = useTranslation()

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
        <span className={badgeClass(delivery.status)}>{normalizeStatus(delivery.status)}</span>
        <span className="app-card-title" style={{ marginBottom: 0 }}>{delivery.title}</span>
      </div>

      {delivery.summary && (
        <div style={{ fontSize: 13, color: 'var(--text-secondary)', lineHeight: 1.5 }}>
          {delivery.summary}
        </div>
      )}

      <section style={{ display: 'grid', gridTemplateColumns: 'repeat(2, minmax(0, 1fr))', gap: 12 }}>
        <Fact label={t('apps.deliveries.detail.owner')} value={delivery.owner ? `@${delivery.owner}` : '—'} />
        <Fact label={t('apps.deliveries.detail.channel')} value={delivery.channel ? `#${delivery.channel}` : '—'} />
        <Fact label={t('apps.deliveries.detail.progress')} value={`${delivery.progress_percent ?? 0}%`} />
        <Fact label={t('apps.deliveries.detail.basis')} value={delivery.progress_basis || t('apps.deliveries.labels.noMilestones')} />
        <Fact label={t('apps.deliveries.detail.lastUpdate')} value={formatTime(delivery.last_substantive_update_at)} />
        <Fact label={t('apps.deliveries.detail.lastBy')} value={delivery.last_substantive_update_by ? `@${delivery.last_substantive_update_by}` : '—'} />
      </section>

      {delivery.last_substantive_summary && (
        <section>
          <div style={{ fontSize: 12, fontWeight: 700, marginBottom: 6 }}>{t('apps.deliveries.detail.latestMovement')}</div>
          <div style={{ fontSize: 13, color: 'var(--text-secondary)', lineHeight: 1.5 }}>{delivery.last_substantive_summary}</div>
        </section>
      )}

      <section>
        <div style={{ fontSize: 12, fontWeight: 700, marginBottom: 6 }}>{t('apps.deliveries.detail.links')}</div>
        <div className="app-card-meta">
          {t('apps.deliveries.detail.tasksCount', { count: delivery.task_ids?.length ?? 0 })}
          {' · '}
          {t('apps.deliveries.detail.requestsCount', { count: delivery.request_ids?.length ?? 0 })}
          {' · '}
          {t('apps.deliveries.detail.pendingHumanCount', { count: delivery.pending_human_count ?? 0 })}
        </div>
      </section>

      <section>
        <div style={{ fontSize: 12, fontWeight: 700, marginBottom: 8 }}>{t('apps.deliveries.detail.artifacts')}</div>
        {(delivery.artifacts ?? []).length === 0 ? (
          <div className="app-card-meta">{t('apps.deliveries.detail.noArtifacts')}</div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {(delivery.artifacts ?? []).map((artifact, index) => (
              <div key={`${artifact.kind}-${artifact.path || artifact.url || index}`} className="app-card delivery-artifact-card" style={{ marginBottom: 0 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap', marginBottom: 4 }}>
                  <span className="badge badge-accent">{artifact.kind}</span>
                  <span className="app-card-title" style={{ marginBottom: 0 }}>{artifact.title}</span>
                </div>
                {artifact.summary && (
                  <div style={{ fontSize: 12, color: 'var(--text-secondary)', marginBottom: 4 }}>{artifact.summary}</div>
                )}
                <div className="app-card-meta">
                  {[artifact.path || artifact.url || '', artifact.state || '', artifact.updated_at ? formatTime(artifact.updated_at) : ''].filter(Boolean).join(' · ') || '—'}
                </div>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  )
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div className="delivery-fact">
      <div style={{ fontSize: 10, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.05em', color: 'var(--text-tertiary)', marginBottom: 4 }}>
        {label}
      </div>
      <div style={{ fontSize: 13, color: 'var(--text-primary)' }}>{value}</div>
    </div>
  )
}
