import type { TFunction } from 'i18next'
import type { StudioActiveContextSnapshot } from '../../../api/client'

interface StudioActiveContextProps {
  context: StudioActiveContextSnapshot
  onOpenChannel: (channel: string) => void
  onOpenTasks: () => void
  t: TFunction
}

function formatRelativeTime(raw?: string): string {
  if (!raw) return 'n/a'
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return raw
  const deltaMs = Date.now() - date.getTime()
  const deltaMinutes = Math.max(1, Math.round(deltaMs / 60_000))
  if (deltaMinutes < 60) return `${deltaMinutes}m ago`
  const deltaHours = Math.round(deltaMinutes / 60)
  if (deltaHours < 24) return `${deltaHours}h ago`
  return `${Math.round(deltaHours / 24)}d ago`
}

function formatLivenessLabel(raw?: string): string {
  return (raw || 'liveness').replace(/_/g, ' ')
}

function livenessTone(raw?: string): { bg: string; fg: string; dot: string } {
  switch ((raw || '').trim().toLowerCase()) {
    case 'advanced':
    case 'completed':
      return { bg: 'rgba(73, 127, 77, 0.12)', fg: '#3b7b54', dot: '#3b7b54' }
    case 'needs_followup':
    case 'blocked':
      return { bg: 'rgba(183, 112, 34, 0.12)', fg: '#9f651f', dot: '#b97022' }
    case 'failed':
    case 'empty_response':
    case 'plan_only':
      return { bg: 'rgba(198, 68, 68, 0.12)', fg: '#b24a4a', dot: '#b24a4a' }
    default:
      return { bg: 'rgba(111, 118, 132, 0.12)', fg: 'var(--text-secondary)', dot: 'var(--text-tertiary)' }
  }
}

export function StudioActiveContext({
  context,
  onOpenChannel,
  onOpenTasks,
  t,
}: StudioActiveContextProps) {
  const channels = context.channels ?? []
  const flows = context.flows ?? []
  const tasks = context.tasks ?? []
  const workspaces = context.workspaces ?? []

  return (
    <section
      data-testid="studio-active-context"
      className="app-card"
      style={{ display: 'flex', flexDirection: 'column', gap: 16, padding: 18 }}
    >
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 16, flexWrap: 'wrap', alignItems: 'flex-start' }}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
          <div style={{ fontSize: 16, fontWeight: 700 }}>{t('apps.studio.activeTitle')}</div>
          <div className="app-card-meta">{t('apps.studio.activeSummary')}</div>
        </div>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
          {context.primary_channel ? (
            <button className="btn btn-secondary btn-sm" onClick={() => onOpenChannel(context.primary_channel ?? 'general')}>
              {t('apps.studio.openChannel')}
            </button>
          ) : null}
          <button className="btn btn-secondary btn-sm" onClick={onOpenTasks}>
            {t('apps.studio.openTasks')}
          </button>
        </div>
      </div>

      {!context.primary_channel && channels.length === 0 && flows.length === 0 && tasks.length === 0 && workspaces.length === 0 ? (
        <div
          style={{
            padding: '24px 18px',
            borderRadius: 16,
            border: '1px dashed var(--border)',
            background: 'var(--bg)',
            color: 'var(--text-secondary)',
            fontSize: 13,
          }}
        >
          {t('apps.studio.noContext')}
        </div>
      ) : (
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: 'minmax(260px, 1.1fr) minmax(280px, 1fr)',
            gap: 12,
          }}
        >
          <ContextPane title={t('apps.studio.activeFlow')}>
            <FactRow label={t('apps.studio.primaryChannel')} value={context.primary_channel ? `#${context.primary_channel}` : 'n/a'} />
            <FactRow label={t('apps.studio.sessionMode')} value={context.session_mode || 'n/a'} />
            <FactRow label={t('apps.studio.directAgent')} value={context.direct_agent ? `@${context.direct_agent}` : 'n/a'} />
            <FactRow label={t('apps.studio.status')} value={context.focus || 'n/a'} />
            {(context.next_steps ?? []).length > 0 && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                <span style={{ fontSize: 11, color: 'var(--text-tertiary)', textTransform: 'uppercase', letterSpacing: '0.04em' }}>
                  {t('apps.studio.nextSteps')}
                </span>
                <ul style={{ margin: 0, paddingLeft: 18, color: 'var(--text-secondary)', fontSize: 12, lineHeight: 1.6 }}>
                  {(context.next_steps ?? []).slice(0, 4).map((step) => (
                    <li key={step}>{step}</li>
                  ))}
                </ul>
              </div>
            )}
          </ContextPane>

          <ContextPane title={t('apps.studio.channelLoad')}>
            {(channels.length > 0 ? channels : []).slice(0, 4).map((channel) => (
              <div
                key={channel.slug}
                style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  gap: 12,
                  padding: '10px 12px',
                  borderRadius: 14,
                  background: 'var(--bg)',
                  border: '1px solid var(--border)',
                }}
              >
                <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  <span style={{ fontSize: 13, fontWeight: 600 }}>{channel.name || `#${channel.slug}`}</span>
                  <span style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>
                    {(channel.members ?? []).map((member) => `@${member}`).join(' · ') || '#'}
                  </span>
                  {(channel.last_substantive_update_by || channel.last_decision_summary) ? (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 4, marginTop: 2 }}>
                      {channel.last_substantive_update_by ? (
                        <span style={{ fontSize: 11, color: 'var(--text-secondary)', lineHeight: 1.4 }}>
                          {t('apps.studio.lastUpdateBy', { agent: channel.last_substantive_update_by })}
                          {channel.last_substantive_preview ? ` · ${channel.last_substantive_preview}` : ''}
                        </span>
                      ) : null}
                      {channel.last_decision_summary ? (
                        <span style={{ fontSize: 11, color: 'var(--text-tertiary)', lineHeight: 1.4 }}>
                          {t('apps.studio.lastDecisionLabel')}
                          {`: ${channel.last_decision_summary}`}
                        </span>
                      ) : null}
                    </div>
                  ) : null}
                </div>
                <div style={{ fontSize: 11, color: 'var(--text-secondary)', textAlign: 'right', display: 'flex', flexDirection: 'column', gap: 4 }}>
                  <div>{channel.task_counts.total} tasks</div>
                  <div>{channel.request_count ?? 0} requests</div>
                  <div>{channel.blockers?.length ?? 0} blockers</div>
                  <div style={{ display: 'inline-flex', flexWrap: 'wrap', gap: 4, justifyContent: 'flex-end' }}>
                    {(channel.attention_count ?? 0) > 0 ? (
                      <span style={pillStyle('rgba(73, 127, 77, 0.12)', '#3b7b54')}>
                        {t('apps.studio.attentionCount', { count: channel.attention_count ?? 0 })}
                      </span>
                    ) : null}
                    {(channel.waiting_human_count ?? 0) > 0 ? (
                      <span style={pillStyle('rgba(198, 68, 68, 0.12)', '#b24a4a')}>
                        {t('apps.studio.waitingHumanCount', { count: channel.waiting_human_count ?? 0 })}
                      </span>
                    ) : null}
                    {(channel.active_owner_count ?? 0) > 0 ? (
                      <span style={pillStyle('rgba(111, 118, 132, 0.12)', 'var(--text-secondary)')}>
                        {t('apps.studio.activeOwnersCount', { count: channel.active_owner_count ?? 0 })}
                      </span>
                    ) : null}
                  </div>
                </div>
              </div>
            ))}
          </ContextPane>

          <ContextPane title={t('apps.studio.hotTasks')}>
            {(tasks.length > 0 ? tasks : []).slice(0, 5).map((task) => (
              <div
                key={task.id}
                style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  gap: 12,
                  padding: '10px 12px',
                  borderRadius: 14,
                  background: 'var(--bg)',
                  border: '1px solid var(--border)',
                }}
              >
                <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  <span style={{ fontSize: 13, fontWeight: 600 }}>{task.title || task.id}</span>
                  <span style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>
                    {task.channel ? `#${task.channel}` : 'no-channel'}
                    {task.owner ? ` · @${task.owner}` : ''}
                    {task.workspace_path ? ` · ${task.workspace_path}` : ''}
                  </span>
                </div>
                <div style={{ fontSize: 11, color: 'var(--text-secondary)', textAlign: 'right' }}>
                  <div>{task.status || 'n/a'}</div>
                  {task.liveness_state ? (
                    <LivenessBadge
                      state={task.liveness_state}
                      reason={task.liveness_reason}
                      at={task.liveness_at}
                    />
                  ) : null}
                  <div>{formatRelativeTime(task.updated_at)}</div>
                </div>
              </div>
            ))}
          </ContextPane>

          <ContextPane title={t('apps.studio.flowLoad')}>
            {(flows.slice(0, 3).map((flow) => ({
              key: flow.id,
              title: flow.label,
              meta: [flow.channel ? `#${flow.channel}` : '', flow.owner ? `@${flow.owner}` : '', flow.workspace || ''].filter(Boolean).join(' · '),
              status: flow.status || `${flow.task_count} tasks`,
            })) as Array<{ key: string; title: string; meta: string; status: string }>).concat(
              workspaces.slice(0, 2).map((workspace) => ({
                key: workspace.path,
                title: workspace.path.split(/[\\/]/).filter(Boolean).pop() || workspace.path,
                meta: [workspace.channel ? `#${workspace.channel}` : '', workspace.owner ? `@${workspace.owner}` : '', workspace.branch || ''].filter(Boolean).join(' · '),
                status: workspace.healthy ? 'healthy' : workspace.issue || 'issue',
              })),
            ).map((row) => (
              <div
                key={row.key}
                style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  gap: 12,
                  padding: '10px 12px',
                  borderRadius: 14,
                  background: 'var(--bg)',
                  border: '1px solid var(--border)',
                }}
              >
                <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  <span style={{ fontSize: 13, fontWeight: 600 }}>{row.title}</span>
                  <span style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>{row.meta || 'dev-runtime'}</span>
                </div>
                <div style={{ fontSize: 11, color: 'var(--text-secondary)', textAlign: 'right' }}>{row.status}</div>
              </div>
            ))}
          </ContextPane>
        </div>
      )}
    </section>
  )
}

function LivenessBadge({ state, reason, at }: { state: string; reason?: string; at?: string }) {
  const tone = livenessTone(state)
  const title = [reason || state, at ? `Recorded ${formatRelativeTime(at)}` : ''].filter(Boolean).join(' · ')

  return (
    <span
      title={title}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'flex-end',
        gap: 5,
        maxWidth: 150,
        padding: '3px 8px',
        borderRadius: 999,
        background: tone.bg,
        color: tone.fg,
        fontSize: 10,
        fontWeight: 700,
        letterSpacing: 0,
        lineHeight: 1.2,
        textTransform: 'uppercase',
        whiteSpace: 'nowrap',
      }}
    >
      <span
        aria-hidden="true"
        style={{
          width: 6,
          height: 6,
          borderRadius: 999,
          background: tone.dot,
          flex: '0 0 auto',
        }}
      />
      <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>
        {formatLivenessLabel(state)}
      </span>
    </span>
  )
}

function ContextPane({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 10, padding: '14px 15px', borderRadius: 18, border: '1px solid var(--border)', background: 'var(--bg)' }}>
      <div style={{ fontSize: 13, fontWeight: 700 }}>{title}</div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>{children}</div>
    </div>
  )
}

function FactRow({ label, value }: { label: string; value: string }) {
  return (
    <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, paddingBottom: 8, borderBottom: '1px solid var(--border)' }}>
      <span style={{ fontSize: 11, color: 'var(--text-tertiary)', textTransform: 'uppercase', letterSpacing: '0.04em' }}>{label}</span>
      <span style={{ fontSize: 12, color: 'var(--text-primary)', textAlign: 'right' }}>{value}</span>
    </div>
  )
}

function pillStyle(background: string, color: string) {
  return {
    padding: '3px 8px',
    borderRadius: 999,
    background,
    color,
    fontSize: 10,
    fontWeight: 700,
    letterSpacing: '0.03em',
    textTransform: 'uppercase' as const,
  }
}
