import { type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ACTIVITY_APP_ID, HOME_APP_ID } from '../../lib/constants'
import { useAppStore, type PrimaryRail as PrimaryRailKey } from '../../stores/app'

type RailItem = {
  key: PrimaryRailKey
  labelKey: string
  testId: string
  icon: ReactNode
  onClick: () => void
}

export function PrimaryRail() {
  const { t } = useTranslation()
  const primaryRail = useAppStore((s) => s.primaryRail)
  const setPrimaryRail = useAppStore((s) => s.setPrimaryRail)
  const setCurrentApp = useAppStore((s) => s.setCurrentApp)
  const setCurrentChannel = useAppStore((s) => s.setCurrentChannel)
  const currentChannel = useAppStore((s) => s.currentChannel)

  const items: RailItem[] = [
    {
      key: 'home',
      labelKey: 'layout.rail.home',
      testId: 'rail-item-home',
      onClick: () => setCurrentApp(HOME_APP_ID),
      icon: (
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
          <path d="M3 10.5 12 3l9 7.5" />
          <path d="M5 9.8V20h14V9.8" />
          <path d="M9.5 20v-5h5v5" />
        </svg>
      ),
    },
    {
      key: 'dms',
      labelKey: 'layout.rail.dms',
      testId: 'rail-item-dms',
      onClick: () => setPrimaryRail('dms'),
      icon: (
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
          <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2Z" />
          <path d="M8 9h8" />
          <path d="M8 13h5" />
        </svg>
      ),
    },
    {
      key: 'activity',
      labelKey: 'layout.rail.activity',
      testId: 'rail-item-activity',
      onClick: () => setCurrentApp(ACTIVITY_APP_ID),
      icon: (
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
          <path d="M4 12h3l2.2-6 4.6 12 2.2-6H20" />
        </svg>
      ),
    },
    {
      key: 'channels',
      labelKey: 'layout.rail.channels',
      testId: 'rail-item-channels',
      onClick: () => setCurrentChannel(currentChannel || 'general'),
      icon: (
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
          <path d="M10 3 8 21" />
          <path d="M16 3l-2 18" />
          <path d="M4 9h18" />
          <path d="M3 15h18" />
        </svg>
      ),
    },
    {
      key: 'more',
      labelKey: 'layout.rail.more',
      testId: 'rail-item-more',
      onClick: () => setPrimaryRail('more'),
      icon: (
        <svg viewBox="0 0 24 24" fill="currentColor">
          <circle cx="5" cy="12" r="1.8" />
          <circle cx="12" cy="12" r="1.8" />
          <circle cx="19" cy="12" r="1.8" />
        </svg>
      ),
    },
  ]

  return (
    <aside className="primary-rail" data-testid="primary-rail" aria-label={t('layout.rail.workspace')}>
      <div className="primary-rail-top">
        <button
          type="button"
          className="primary-rail-workspace"
          onClick={() => setCurrentApp(HOME_APP_ID)}
          aria-label={t('layout.rail.workspace')}
          title={t('layout.rail.workspace')}
        >
          <span className="primary-rail-workspace-mark">D</span>
        </button>
      </div>

      <nav className="primary-rail-nav" aria-label={t('layout.rail.navLabel')}>
        {items.map((item) => {
          const active = primaryRail === item.key
          return (
            <button
              key={item.key}
              type="button"
              data-testid={item.testId}
              className={`primary-rail-item${active ? ' active' : ''}`}
              aria-pressed={active}
              aria-label={t(item.labelKey)}
              title={t(item.labelKey)}
              onClick={item.onClick}
            >
              <span className="primary-rail-item-icon" aria-hidden="true">
                {item.icon}
              </span>
              <span className="primary-rail-item-label">{t(item.labelKey)}</span>
            </button>
          )
        })}
      </nav>
    </aside>
  )
}
