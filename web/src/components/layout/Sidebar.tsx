import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useAppStore } from '../../stores/app'
import { AgentList } from '../sidebar/AgentList'
import { AgentStatusSummary } from '../sidebar/AgentStatusSummary'
import { ChannelList } from '../sidebar/ChannelList'
import { AppList } from '../sidebar/AppList'
import { UsagePanel } from '../sidebar/UsagePanel'
import { WorkspaceSummary } from '../sidebar/WorkspaceSummary'
import type { Theme } from '../../stores/app'
import { SUPPORTED_LANGUAGES } from '../../i18n/config'
import { ACTIVITY_APP_ID, HOME_APP_ID } from '../../lib/constants'

const LANGUAGE_LABELS: Record<string, string> = {
  en: 'EN',
  'pt-BR': 'PT-BR',
}

const THEME_LABELS: Record<Theme, string> = {
  slack: 'Slack',
  'slack-dark': 'Dark',
}

function SidebarShortcut({
  active = false,
  onClick,
  icon,
  children,
  subtle = false,
}: {
  active?: boolean
  onClick: () => void
  icon?: ReactNode
  children: ReactNode
  subtle?: boolean
}) {
  return (
    <button
      type="button"
      className={`sidebar-item${active ? ' active' : ''}${subtle ? ' sidebar-item-subtle' : ''}`}
      onClick={onClick}
    >
      {icon ? <span className="sidebar-item-emoji">{icon}</span> : null}
      <span style={{ flex: 1 }}>{children}</span>
    </button>
  )
}

export function Sidebar() {
  const { t, i18n } = useTranslation()
  const theme = useAppStore((s) => s.theme)
  const setTheme = useAppStore((s) => s.setTheme)
  const primaryRail = useAppStore((s) => s.primaryRail)
  const setPrimaryRail = useAppStore((s) => s.setPrimaryRail)
  const setCurrentApp = useAppStore((s) => s.setCurrentApp)
  const currentApp = useAppStore((s) => s.currentApp)
  const sidebarAgentsOpen = useAppStore((s) => s.sidebarAgentsOpen)
  const toggleSidebarAgents = useAppStore((s) => s.toggleSidebarAgents)
  const [openMenu, setOpenMenu] = useState<'theme' | 'language' | null>(null)
  const menuRootRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!openMenu) return

    const handlePointerDown = (event: PointerEvent) => {
      if (!menuRootRef.current?.contains(event.target as Node)) {
        setOpenMenu(null)
      }
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setOpenMenu(null)
      }
    }

    document.addEventListener('pointerdown', handlePointerDown)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [openMenu])

  return (
    <aside className={`sidebar sidebar-mode-${primaryRail}`} data-testid="context-sidebar">
      <div className="sidebar-header">
        <div className="sidebar-brand-lockup">
          <span className="sidebar-brand-eyebrow">{t(`layout.rail.${primaryRail}`)}</span>
          <span className="sidebar-logo">DunderIA</span>
        </div>

        <div ref={menuRootRef} className="sidebar-toolbar">
          <UsagePanel compact />

          <div className="sidebar-utility-group">
            <button
              type="button"
              className={`sidebar-utility-icon-button${openMenu === 'theme' ? ' open' : ''}`}
              aria-label={`${t('sidebar.theme.label')}: ${THEME_LABELS[theme]}`}
              title={`${t('sidebar.theme.label')}: ${THEME_LABELS[theme]}`}
              aria-expanded={openMenu === 'theme'}
              onClick={() => setOpenMenu((current) => current === 'theme' ? null : 'theme')}
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
                <circle cx="13.5" cy="6.5" r="2.5" />
                <circle cx="17.5" cy="12" r="2" />
                <circle cx="14" cy="17.5" r="2.25" />
                <circle cx="7.75" cy="15.75" r="2.25" />
                <path d="M11.2 8.8 9.3 13.2" />
                <path d="M15.4 8.7 16.5 10.2" />
                <path d="M12.8 15.3 10 15.8" />
              </svg>
            </button>
            {openMenu === 'theme' && (
              <div className="sidebar-utility-menu" role="menu" aria-label={t('sidebar.theme.label')}>
                {(Object.entries(THEME_LABELS) as Array<[Theme, string]>).map(([value, label]) => (
                  <button
                    key={value}
                    type="button"
                    className={`sidebar-utility-menu-item${theme === value ? ' active' : ''}`}
                    onClick={() => {
                      setTheme(value)
                      setOpenMenu(null)
                    }}
                  >
                    <span>{label}</span>
                    {theme === value && <span className="sidebar-utility-menu-check">✓</span>}
                  </button>
                ))}
              </div>
            )}
          </div>

          <div className="sidebar-utility-group">
            <button
              type="button"
              className={`sidebar-utility-icon-button${openMenu === 'language' ? ' open' : ''}`}
              aria-label={`${t('sidebar.language.label')}: ${LANGUAGE_LABELS[i18n.resolvedLanguage || 'en'] ?? i18n.resolvedLanguage ?? 'en'}`}
              title={`${t('sidebar.language.label')}: ${LANGUAGE_LABELS[i18n.resolvedLanguage || 'en'] ?? i18n.resolvedLanguage ?? 'en'}`}
              aria-expanded={openMenu === 'language'}
              onClick={() => setOpenMenu((current) => current === 'language' ? null : 'language')}
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
                <path d="M3 12h18" />
                <path d="M12 3a15.3 15.3 0 0 1 4 9 15.3 15.3 0 0 1-4 9 15.3 15.3 0 0 1-4-9 15.3 15.3 0 0 1 4-9Z" />
                <circle cx="12" cy="12" r="9" />
              </svg>
            </button>
            {openMenu === 'language' && (
              <div className="sidebar-utility-menu" role="menu" aria-label={t('sidebar.language.label')}>
                {SUPPORTED_LANGUAGES.map((lng) => (
                  <button
                    key={lng}
                    type="button"
                    className={`sidebar-utility-menu-item${(i18n.resolvedLanguage || 'en') === lng ? ' active' : ''}`}
                    onClick={() => {
                      void i18n.changeLanguage(lng)
                      setOpenMenu(null)
                    }}
                  >
                    <span>{LANGUAGE_LABELS[lng] ?? lng}</span>
                    {(i18n.resolvedLanguage || 'en') === lng && <span className="sidebar-utility-menu-check">✓</span>}
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>

      <div className="sidebar-body">
        {primaryRail === 'home' ? (
          <>
            <div className="sidebar-section">
              <p className="sidebar-section-title">{t('sidebar.context.jumpBackIn')}</p>
              <SidebarShortcut
                active={currentApp === HOME_APP_ID}
                icon="⌂"
                onClick={() => setCurrentApp(HOME_APP_ID)}
              >
                {t('layout.rail.home')}
              </SidebarShortcut>
              <SidebarShortcut icon="⚡" onClick={() => setCurrentApp(ACTIVITY_APP_ID)}>
                {t('layout.rail.activity')}
              </SidebarShortcut>
              <SidebarShortcut icon="#" onClick={() => setPrimaryRail('channels')}>
                {t('layout.rail.channels')}
              </SidebarShortcut>
            </div>

            <div className="sidebar-section">
              <p className="sidebar-section-title">{t('sidebar.sections.team')}</p>
              <SidebarShortcut
                active={sidebarAgentsOpen}
                icon="◎"
                onClick={toggleSidebarAgents}
              >
                {t('sidebar.agents.toggle')}
              </SidebarShortcut>
            </div>
            <AgentStatusSummary />
            {sidebarAgentsOpen ? <AgentList /> : null}

            <div className="sidebar-section sidebar-section-divider">
              <p className="sidebar-section-title">{t('sidebar.sections.apps')}</p>
            </div>
            <AppList compact />
          </>
        ) : null}

        {primaryRail === 'dms' ? (
          <>
            <div className="sidebar-section">
              <p className="sidebar-section-title">{t('layout.rail.dms')}</p>
              <div className="sidebar-copy">{t('sidebar.context.dmsCopy')}</div>
            </div>
            <AgentStatusSummary />
            <AgentList />
          </>
        ) : null}

        {primaryRail === 'activity' ? (
          <>
            <div className="sidebar-section">
              <p className="sidebar-section-title">{t('layout.rail.activity')}</p>
              <SidebarShortcut
                active={currentApp === ACTIVITY_APP_ID}
                icon="◔"
                onClick={() => setCurrentApp(ACTIVITY_APP_ID)}
              >
                {t('sidebar.context.activityFeed')}
              </SidebarShortcut>
              <SidebarShortcut icon="📥" onClick={() => setCurrentApp('tasks')}>
                {t('sidebar.apps.tasks')}
              </SidebarShortcut>
              <SidebarShortcut icon="📦" onClick={() => setCurrentApp('deliveries')}>
                {t('sidebar.apps.deliveries')}
              </SidebarShortcut>
              <SidebarShortcut icon="↳" onClick={() => setCurrentApp('threads')}>
                {t('sidebar.context.threads')}
              </SidebarShortcut>
            </div>
            <div className="sidebar-section sidebar-section-divider">
              <p className="sidebar-section-title">{t('sidebar.context.signalSummary')}</p>
              <WorkspaceSummary />
            </div>
          </>
        ) : null}

        {primaryRail === 'channels' ? (
          <>
            <div className="sidebar-section">
              <p className="sidebar-section-title">{t('layout.rail.channels')}</p>
            </div>
            <ChannelList />
            <div className="sidebar-section sidebar-section-divider">
              <p className="sidebar-section-title">{t('layout.rail.dms')}</p>
            </div>
            <AgentStatusSummary />
            <AgentList />
          </>
        ) : null}

        {primaryRail === 'more' ? (
          <>
            <div className="sidebar-section">
              <p className="sidebar-section-title">{t('layout.rail.more')}</p>
              <div className="sidebar-copy">{t('sidebar.context.moreCopy')}</div>
            </div>
            <AppList />
          </>
        ) : null}
      </div>

      <div className="sidebar-footer">
        <WorkspaceSummary />
      </div>
    </aside>
  )
}
