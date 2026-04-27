import type { TFunction } from 'i18next'
import type { StudioActionDefinition, StudioBlocker } from '../../../api/client'
import { StudioBlockerCard } from './StudioBlockerCard'

interface StudioBlockerListProps {
  blockers: StudioBlocker[]
  actionDefinitions: Record<string, StudioActionDefinition>
  membersByChannel: Record<string, string[]>
  pendingKey: string | null
  onAction: (action: string, blocker: StudioBlocker, extras?: { owner?: string }) => void
  t: TFunction
}

export function StudioBlockerList({
  blockers,
  actionDefinitions,
  membersByChannel,
  pendingKey,
  onAction,
  t,
}: StudioBlockerListProps) {
  return (
    <section
      data-testid="studio-blockers"
      className="app-card"
      style={{ display: 'flex', flexDirection: 'column', gap: 16, padding: 18 }}
    >
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap', alignItems: 'baseline' }}>
        <div>
          <div style={{ fontSize: 16, fontWeight: 700 }}>{t('apps.studio.blockersTitle')}</div>
          <div className="app-card-meta" style={{ marginTop: 4 }}>
            {t('apps.studio.blockersSummary')}
          </div>
        </div>
        <div
          style={{
            padding: '5px 10px',
            borderRadius: 999,
            background: blockers.length > 0 ? 'rgba(183, 112, 34, 0.12)' : 'rgba(73, 127, 77, 0.12)',
            color: blockers.length > 0 ? '#9f651f' : '#3b7b54',
            fontSize: 11,
            fontWeight: 700,
            textTransform: 'uppercase',
            letterSpacing: '0.05em',
          }}
        >
          {t('apps.studio.blockersCount', { count: blockers.length })}
        </div>
      </div>

      {blockers.length === 0 ? (
        <div
          style={{
            padding: '28px 18px',
            borderRadius: 16,
            border: '1px dashed var(--border)',
            color: 'var(--text-secondary)',
            background: 'var(--bg)',
            textAlign: 'center',
            fontSize: 13,
          }}
        >
          {t('apps.studio.noBlockers')}
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          {blockers.map((blocker) => (
            <StudioBlockerCard
              key={blocker.id}
              blocker={blocker}
              actionDefinitions={actionDefinitions}
              membersByChannel={membersByChannel}
              pendingKey={pendingKey}
              onAction={onAction}
              t={t}
            />
          ))}
        </div>
      )}
    </section>
  )
}
