import type { TFunction } from 'i18next'
import type { StudioActionDefinition, StudioBlocker } from '../../../api/client'
import { StudioActionBar } from './StudioActionBar'

interface StudioBlockerCardProps {
  blocker: StudioBlocker
  actionDefinitions: Record<string, StudioActionDefinition>
  membersByChannel: Record<string, string[]>
  pendingKey: string | null
  onAction: (action: string, blocker: StudioBlocker, extras?: { owner?: string }) => void
  t: TFunction
}

function severityTone(severity: string): { border: string; badgeBg: string; badgeFg: string } {
  switch (severity) {
    case 'critical':
      return { border: 'rgba(198, 68, 68, 0.45)', badgeBg: 'rgba(198, 68, 68, 0.12)', badgeFg: '#b24a4a' }
    case 'high':
      return { border: 'rgba(183, 112, 34, 0.4)', badgeBg: 'rgba(183, 112, 34, 0.12)', badgeFg: '#9f651f' }
    default:
      return { border: 'rgba(111, 118, 132, 0.28)', badgeBg: 'rgba(111, 118, 132, 0.12)', badgeFg: 'var(--text-secondary)' }
  }
}

export function StudioBlockerCard({
  blocker,
  actionDefinitions,
  membersByChannel,
  pendingKey,
  onAction,
  t,
}: StudioBlockerCardProps) {
  const tone = severityTone(blocker.severity)
  const metadata = [
    blocker.channel ? `#${blocker.channel}` : '',
    blocker.owner ? `@${blocker.owner}` : '',
    blocker.task_id ?? '',
  ].filter(Boolean)

  return (
    <article
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: 14,
        padding: '16px 18px',
        borderRadius: 18,
        border: `1px solid ${tone.border}`,
        background: 'linear-gradient(180deg, var(--bg-card) 0%, color-mix(in srgb, var(--bg-card) 82%, transparent) 100%)',
        boxShadow: '0 10px 24px rgba(0,0,0,0.06)',
      }}
    >
      <div style={{ display: 'flex', gap: 12, alignItems: 'flex-start', justifyContent: 'space-between', flexWrap: 'wrap' }}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8, minWidth: 0 }}>
          <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
            <span
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                padding: '4px 9px',
                borderRadius: 999,
                background: tone.badgeBg,
                color: tone.badgeFg,
                fontSize: 11,
                fontWeight: 700,
                letterSpacing: '0.04em',
                textTransform: 'uppercase',
              }}
            >
              {blocker.severity}
            </span>
            {blocker.recommended_action && (
              <span style={{ fontSize: 11, color: 'var(--text-tertiary)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                {t('apps.studio.recommended')}
              </span>
            )}
          </div>
          <div style={{ fontSize: 16, fontWeight: 700, color: 'var(--text-primary)' }}>{blocker.title}</div>
          <div style={{ fontSize: 13, lineHeight: 1.55, color: 'var(--text-secondary)', maxWidth: 760 }}>{blocker.summary}</div>
        </div>
        {metadata.length > 0 && (
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, maxWidth: 320, justifyContent: 'flex-end' }}>
            {metadata.map((item) => (
              <span
                key={item}
                style={{
                  fontSize: 11,
                  color: 'var(--text-secondary)',
                  padding: '4px 8px',
                  borderRadius: 999,
                  background: 'var(--bg)',
                  border: '1px solid var(--border)',
                }}
              >
                {item}
              </span>
            ))}
          </div>
        )}
      </div>

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))',
          gap: 10,
        }}
      >
        <BlockerFact label={t('apps.studio.reason')} value={blocker.reason} />
        {blocker.waiting_on ? <BlockerFact label={t('apps.studio.waitingOn')} value={blocker.waiting_on} /> : null}
        {blocker.task_id ? <BlockerFact label={t('apps.studio.task')} value={blocker.task_id} /> : null}
        {blocker.owner ? <BlockerFact label={t('apps.studio.owner')} value={`@${blocker.owner}`} /> : null}
      </div>

      <StudioActionBar
        blocker={blocker}
        actionDefinitions={actionDefinitions}
        membersByChannel={membersByChannel}
        pendingKey={pendingKey}
        onAction={onAction}
        t={t}
      />
    </article>
  )
}

function BlockerFact({ label, value }: { label: string; value: string }) {
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: 6,
        padding: '10px 12px',
        borderRadius: 14,
        background: 'var(--bg)',
        border: '1px solid var(--border)',
      }}
    >
      <span style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.04em', color: 'var(--text-tertiary)' }}>
        {label}
      </span>
      <span style={{ fontSize: 13, lineHeight: 1.45, color: 'var(--text-primary)' }}>{value}</span>
    </div>
  )
}
