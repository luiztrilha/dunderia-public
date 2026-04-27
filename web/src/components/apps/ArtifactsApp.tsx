import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  getActions,
  getDecisions,
  getWatchdogs,
  getScheduler,
  getUsage,
  type Task,
  type OfficeMember,
} from '../../api/client'
import { formatTokens } from '../../lib/format'
import { InsightsList, type Insight } from '../activity/InsightsList'
import { Timeline, type TimelineEvent } from '../activity/Timeline'
import { useBrokerRefetchInterval } from '../../hooks/useBrokerEvents'
import { schedulerKey, usageKey } from '../../lib/queryKeys'
import { useOfficeMembers } from '../../hooks/useMembers'
import { useOfficeTasks } from '../../hooks/useTasks'

/** Minimal action/decision/watchdog shapes from the untyped endpoints. */
interface ActionRecord {
  summary?: string
  name?: string
  title?: string
  kind?: string
  type?: string
  channel?: string
  actor?: string
  source?: string
  created_at?: string
  related_id?: string
}

interface DecisionRecord {
  summary?: string
  kind?: string
  reason?: string
  channel?: string
  owner?: string
  created_at?: string
  requires_human?: boolean
  blocking?: boolean
}

interface WatchdogRecord {
  summary?: string
  kind?: string
  channel?: string
  owner?: string
  target_type?: string
  target_id?: string
  updated_at?: string
  created_at?: string
}

interface SchedulerJobRaw {
  id?: string
  label?: string
  slug?: string
  status?: string
  channel?: string
  provider?: string
  workflow_key?: string
  skill_name?: string
  kind?: string
  next_run?: string
  due_at?: string
}

function normalizeStatus(raw: string): string {
  const s = raw.toLowerCase().replace(/[\s-]+/g, '_')
  if (s === 'completed') return 'done'
  return s
}

function classifyMemberActivity(member: OfficeMember, t: TFunction): { state: string; label: string } {
  if (member.status === 'shipping' || member.task) return { state: 'shipping', label: t('apps.artifacts.labels.shipping') }
  if (member.status === 'plotting') return { state: 'plotting', label: t('apps.artifacts.labels.plotting') }
  return { state: 'lurking', label: t('apps.artifacts.labels.idle') }
}

export function ArtifactsApp() {
  const { t } = useTranslation()
  const refetchInterval = useBrokerRefetchInterval(15_000)
  const tasks = useOfficeTasks({ includeDone: true, fallbackMs: 15_000 })

  const actions = useQuery({
    queryKey: ['activity-actions'],
    queryFn: () => getActions() as Promise<{ actions: ActionRecord[] }>,
    refetchInterval,
    staleTime: 30_000,
  })

  const decisions = useQuery({
    queryKey: ['activity-decisions'],
    queryFn: () => getDecisions() as Promise<{ decisions: DecisionRecord[] }>,
    refetchInterval,
    staleTime: 30_000,
  })

  const watchdogs = useQuery({
    queryKey: ['activity-watchdogs'],
    queryFn: () => getWatchdogs() as Promise<{ watchdogs: WatchdogRecord[] }>,
    refetchInterval,
    staleTime: 30_000,
  })

  const scheduler = useQuery({
    queryKey: schedulerKey(true),
    queryFn: () => getScheduler({ dueOnly: true }),
    refetchInterval,
    staleTime: 30_000,
  })

  const usage = useQuery({
    queryKey: usageKey(),
    queryFn: () => getUsage(),
    refetchInterval,
    staleTime: 30_000,
  })

  const members = useOfficeMembers()

  const isLoading =
    tasks.isLoading || actions.isLoading || decisions.isLoading ||
    watchdogs.isLoading || scheduler.isLoading || usage.isLoading || members.isLoading

  if (isLoading) {
    return (
      <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
        {t('apps.artifacts.loading')}
      </div>
    )
  }

  const allTasks = tasks.data?.tasks ?? []
  const allActions = ((actions.data as { actions?: ActionRecord[] })?.actions ?? []).slice()
  const allDecisions = ((decisions.data as { decisions?: DecisionRecord[] })?.decisions ?? []).slice()
  const allWatchdogs = ((watchdogs.data as { watchdogs?: WatchdogRecord[] })?.watchdogs ?? []).slice()
  const allJobs = (scheduler.data?.jobs ?? []) as unknown as SchedulerJobRaw[]
  const usageData = usage.data
  const allMembers = members.data ?? []

  const activeTasks = allTasks.filter((t) => {
    const s = normalizeStatus(t.status)
    return s === 'in_progress' || s === 'review' || s === 'open'
  })
  const blockedTasks = allTasks.filter((t) => normalizeStatus(t.status) === 'blocked')
  const liveAgents = allMembers.filter((m) => m.slug !== 'human' && m.slug !== 'you' && classifyMemberActivity(m, t).state !== 'lurking')

  allActions.sort((a, b) => String(b.created_at ?? '').localeCompare(String(a.created_at ?? '')))
  allDecisions.sort((a, b) => String(b.created_at ?? '').localeCompare(String(a.created_at ?? '')))

  const insights: Insight[] = [
    ...blockedTasks.map<Insight>((task) => ({
      priority: 'high',
      category: 'task',
      title: task.title || task.id || t('apps.artifacts.labels.blockedTask'),
      body: task.description,
      target: [task.channel ? `#${task.channel}` : '', task.owner ? `@${task.owner}` : ''].filter(Boolean).join(' · ') || undefined,
      time: task.updated_at ? new Date(task.updated_at).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' }) : undefined,
    })),
    ...allWatchdogs.map<Insight>((w) => ({
      priority: w.kind?.toLowerCase() === 'critical' ? 'critical' : 'high',
      category: w.kind || t('apps.artifacts.labels.watchdog'),
      title: w.summary || w.kind || t('apps.artifacts.labels.watchdogAlert'),
      body: w.target_type ? `${w.target_type}${w.target_id ? ' · ' + w.target_id : ''}` : undefined,
      target: w.channel ? `#${w.channel}` : undefined,
      time: (w.updated_at || w.created_at)
        ? new Date(w.updated_at || w.created_at || '').toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
        : undefined,
    })),
  ]

  const timelineEvents: TimelineEvent[] = [
    ...allDecisions
      .filter((d) => d.created_at)
      .map<TimelineEvent>((d) => ({
        type: d.blocking ? 'watchdog' : 'decision',
        timestamp: d.created_at || '',
        actor: d.owner,
        content: d.summary || d.reason || d.kind || t('apps.artifacts.labels.decision'),
        meta: [d.channel ? `#${d.channel}` : '', d.kind || ''].filter(Boolean).join(' · ') || undefined,
      })),
    ...allActions
      .filter((a) => a.created_at)
      .map<TimelineEvent>((a) => ({
        type: 'action',
        timestamp: a.created_at || '',
        actor: a.actor,
        content: a.summary || a.name || a.title || t('apps.artifacts.labels.action'),
        meta: [a.channel ? `#${a.channel}` : '', a.kind || a.type || ''].filter(Boolean).join(' · ') || undefined,
      })),
  ]

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      {/* Hero */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
        <div>
          <h3 style={{ fontSize: 18, fontWeight: 700 }}>{t('apps.artifacts.title')}</h3>
          <div style={{ fontSize: 13, color: 'var(--text-secondary)', marginTop: 4 }}>
            {t('apps.artifacts.subtitle')}
          </div>
        </div>
        <div style={{ fontSize: 12, color: 'var(--text-tertiary)', whiteSpace: 'nowrap' }}>
          {new Date().toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })}
        </div>
      </div>

      {/* Stat grid */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(180px, 1fr))', gap: 10 }}>
        <StatCard kicker={t('apps.artifacts.stats.activeLanes')} value={String(activeTasks.length)} copy={t('apps.artifacts.stats.activeLanesCopy')} />
        <StatCard kicker={t('apps.artifacts.stats.blocked')} value={String(blockedTasks.length + allWatchdogs.length)} copy={t('apps.artifacts.stats.blockedCopy')} />
        <StatCard kicker={t('apps.artifacts.stats.agentsInMotion')} value={String(liveAgents.length)} copy={t('apps.artifacts.stats.agentsInMotionCopy')} />
        <StatCard kicker={t('apps.artifacts.stats.recentActions')} value={String(allActions.length)} copy={t('apps.artifacts.stats.recentActionsCopy')} />
        <StatCard kicker={t('apps.artifacts.stats.dueAutomations')} value={String(allJobs.length)} copy={t('apps.artifacts.stats.dueAutomationsCopy')} />
        <StatCard kicker={t('apps.artifacts.stats.sessionTokens')} value={formatTokens(usageData?.session?.total_tokens ?? 0)} copy={t('apps.artifacts.stats.sessionTokensCopy')} />
      </div>

      {/* Two-column grid */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
        {/* Left column */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          <ActivitySection title={t('apps.artifacts.sections.activeLanes')} meta={t('apps.artifacts.sections.activeLanesMeta', { count: activeTasks.length })}>
            {activeTasks.length === 0 ? (
              <EmptyState>{t('apps.artifacts.sections.activeLanesEmpty')}</EmptyState>
            ) : (
              activeTasks.slice(0, 10).map((task) => (
                <ActivityItem
                  key={task.id}
                  title={task.title || task.id || t('apps.artifacts.labels.untitledTask')}
                  body={task.description ?? ''}
                  meta={[task.channel ? `#${task.channel}` : '', task.owner ? `@${task.owner}` : ''].filter(Boolean)}
                  kindLabel={normalizeStatus(task.status).replace(/_/g, ' ')}
                />
              ))
            )}
          </ActivitySection>

          <ActivitySection title={t('apps.artifacts.sections.agentPulse')} meta={t('apps.artifacts.sections.agentPulseMeta', { count: liveAgents.length })}>
            {liveAgents.length === 0 ? (
              <EmptyState>{t('apps.artifacts.sections.agentPulseEmpty')}</EmptyState>
            ) : (
              liveAgents.slice(0, 10).map((member) => {
                const activity = classifyMemberActivity(member, t)
                return (
                  <div key={member.slug} className="app-card" style={{ marginBottom: 6, display: 'flex', alignItems: 'center', gap: 8 }}>
                    <span className={`status-dot ${activity.state}`} />
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ fontWeight: 600, fontSize: 13 }}>{member.name || member.slug}</div>
                      <div className="app-card-meta">{member.task || activity.label}</div>
                    </div>
                  </div>
                )
              })
            )}
          </ActivitySection>

          <ActivitySection title={t('apps.artifacts.sections.recentActions')} meta={t('apps.artifacts.sections.recentActionsMeta', { count: allActions.length })}>
            {allActions.length === 0 ? (
              <EmptyState>{t('apps.artifacts.sections.recentActionsEmpty')}</EmptyState>
            ) : (
              allActions.slice(0, 12).map((action, i) => (
                <ActivityItem
                  key={i}
                  title={action.summary || action.name || action.title || t('apps.artifacts.labels.action')}
                  body={action.related_id ? t('apps.artifacts.labels.related', { id: action.related_id }) : ''}
                  meta={[
                    action.channel ? `#${action.channel}` : '',
                    action.actor ? `@${action.actor}` : '',
                    action.created_at ? new Date(action.created_at).toLocaleString() : '',
                  ].filter(Boolean)}
                  kindLabel={action.kind || action.type || t('apps.artifacts.labels.action').toLowerCase()}
                />
              ))
            )}
          </ActivitySection>
        </div>

        {/* Right column */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          <ActivitySection title={t('apps.artifacts.sections.needsAttention')} meta={t('apps.artifacts.sections.needsAttentionMeta', { count: insights.length })}>
            <InsightsList
              insights={insights}
              emptyLabel={t('apps.artifacts.sections.needsAttentionEmpty')}
              limit={12}
            />
          </ActivitySection>

          <ActivitySection title={t('apps.artifacts.sections.recentActivity')} meta={t('apps.artifacts.sections.recentActivityMeta', { count: timelineEvents.length })}>
            <Timeline
              events={timelineEvents}
              emptyLabel={t('apps.artifacts.sections.recentActivityEmpty')}
              limit={14}
            />
          </ActivitySection>

          <ActivitySection title={t('apps.artifacts.sections.dueAutomations')} meta={t('apps.artifacts.sections.dueAutomationsMeta', { count: allJobs.length })}>
            {allJobs.length === 0 ? (
              <EmptyState>{t('apps.artifacts.sections.dueAutomationsEmpty')}</EmptyState>
            ) : (
              allJobs.slice(0, 6).map((job, idx) => (
                <ActivityItem
                  key={job.slug ?? job.id ?? `due-${idx}`}
                  title={job.label || job.slug || t('apps.artifacts.labels.scheduledJob')}
                  body={job.workflow_key || job.skill_name || job.kind || ''}
                  meta={[
                    job.channel ? `#${job.channel}` : '',
                    job.provider ?? '',
                    (job.next_run || job.due_at) ? new Date(job.next_run || job.due_at || '').toLocaleString() : '',
                  ].filter(Boolean)}
                  kindLabel={job.status || t('apps.artifacts.labels.scheduled')}
                />
              ))
            )}
          </ActivitySection>
        </div>
      </div>
    </div>
  )
}

/* ── Shared sub-components ── */

function StatCard({ kicker, value, copy }: { kicker: string; value: string; copy: string }) {
  return (
    <div className="app-card" style={{ padding: '12px 14px' }}>
      <div style={{ fontSize: 10, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-tertiary)' }}>
        {kicker}
      </div>
      <div style={{ fontSize: 22, fontWeight: 700, margin: '4px 0 2px' }}>{value}</div>
      <div style={{ fontSize: 11, color: 'var(--text-secondary)' }}>{copy}</div>
    </div>
  )
}

function ActivitySection({ title, meta, children }: { title: string; meta?: string; children: React.ReactNode }) {
  return (
    <section>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 8 }}>
        <div style={{ fontSize: 14, fontWeight: 600 }}>{title}</div>
        {meta && <div className="app-card-meta">{meta}</div>}
      </div>
      {children}
    </section>
  )
}

function ActivityItem({ title, body, meta, kindLabel }: { title: string; body: string; meta: string[]; kindLabel: string }) {
  return (
    <div className="app-card" style={{ marginBottom: 6 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 2 }}>
        <span className="badge badge-accent" style={{ fontSize: 10 }}>{kindLabel}</span>
        <span className="app-card-title" style={{ marginBottom: 0 }}>{title}</span>
      </div>
      {body && <div style={{ fontSize: 12, color: 'var(--text-secondary)', marginBottom: 4 }}>{body}</div>}
      {meta.length > 0 && (
        <div className="app-card-meta">{meta.join(' \u2022 ')}</div>
      )}
    </div>
  )
}

function EmptyState({ children }: { children: React.ReactNode }) {
  return (
    <div style={{ padding: '20px 0', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 13 }}>
      {children}
    </div>
  )
}
