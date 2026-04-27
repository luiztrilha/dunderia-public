import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getStudioDevConsole } from '../../api/client'
import { useBrokerRefetchInterval } from '../../hooks/useBrokerEvents'
import { useAppStore } from '../../stores/app'
import { useChannels } from '../../hooks/useChannels'
import { ALL_APPS, ACTIVITY_APP_ID, HOME_APP_ID } from '../../lib/constants'

export function ChannelHeader() {
  const { t } = useTranslation()
  const currentChannel = useAppStore((s) => s.currentChannel)
  const currentApp = useAppStore((s) => s.currentApp)
  const setSearchOpen = useAppStore((s) => s.setSearchOpen)
  const { data: channels = [] } = useChannels(currentApp === null)
  const studioRefetchInterval = useBrokerRefetchInterval(15_000)
  const studioQuery = useQuery({
    queryKey: ['studio-dev-console'],
    queryFn: () => getStudioDevConsole(),
    refetchInterval: studioRefetchInterval,
    staleTime: 30_000,
    enabled: currentApp === null,
  })

  const channel = channels.find((c) => c.slug === currentChannel)
  const channelSnapshot = currentApp
    ? undefined
    : (studioQuery.data?.active_context.channels ?? []).find((item) => item.slug === currentChannel)
  const appEntry = currentApp ? ALL_APPS.find((a) => a.id === currentApp) : undefined
  const title = appEntry
    ? currentApp === HOME_APP_ID
      ? t('layout.rail.home')
      : currentApp === ACTIVITY_APP_ID
        ? t('layout.rail.activity')
        : t(appEntry.nameKey)
    : currentApp
      ? currentApp.charAt(0).toUpperCase() + currentApp.slice(1)
      : `# ${currentChannel}`
  const desc = currentApp === HOME_APP_ID
    ? t('layout.channelHeader.homeSubtitle')
    : currentApp === ACTIVITY_APP_ID
      ? t('layout.channelHeader.activitySubtitle')
      : currentApp
        ? ''
        : channel?.description || ''

  return (
    <div className="channel-header">
      <div className="channel-header-copy">
        <div className="channel-header-title-row">
          <span className="channel-title">{title}</span>
          {desc && <span className="channel-desc">{desc}</span>}
        </div>
        {channelSnapshot ? (
          <div
            className={
              'channel-header-meta'
              + ((channelSnapshot.task_counts.blocked ?? 0) > 0 ? ' channel-header-meta-attention' : '')
              + ((channelSnapshot.task_counts.blocked ?? 0) === 0 && (channelSnapshot.waiting_human_count ?? 0) > 0 ? ' channel-header-meta-waiting' : '')
            }
          >
            <div className="channel-header-pills">
              {(channelSnapshot.attention_count ?? 0) > 0 ? (
                <span className="sidebar-badge sidebar-badge-attention-soft">
                  {t('layout.channelHeader.attentionCount', { count: channelSnapshot.attention_count ?? 0 })}
                </span>
              ) : null}
              {(channelSnapshot.waiting_human_count ?? 0) > 0 ? (
                <span className="sidebar-badge sidebar-badge-waiting">
                  {t('layout.channelHeader.waitingHumanCount', { count: channelSnapshot.waiting_human_count ?? 0 })}
                </span>
              ) : null}
              {(channelSnapshot.task_counts.blocked ?? 0) > 0 ? (
                <span className="sidebar-badge sidebar-badge-attention">
                  {t('layout.channelHeader.blockedCount', { count: channelSnapshot.task_counts.blocked ?? 0 })}
                </span>
              ) : null}
              {(channelSnapshot.active_owner_count ?? 0) > 0 ? (
                <span className="channel-header-pill-neutral">
                  {t('layout.channelHeader.activeOwnersCount', { count: channelSnapshot.active_owner_count ?? 0 })}
                </span>
              ) : null}
            </div>
            {channelSnapshot.last_substantive_update_by ? (
              <span className="channel-header-note" title={channelSnapshot.last_substantive_preview || undefined}>
                {t('layout.channelHeader.lastUpdateBy', { agent: channelSnapshot.last_substantive_update_by })}
                {channelSnapshot.last_substantive_preview ? ` · ${channelSnapshot.last_substantive_preview}` : ''}
              </span>
            ) : null}
            {channelSnapshot.last_decision_summary ? (
              <span className="channel-header-note">
                {t('layout.channelHeader.lastDecisionLabel')}
                {`: ${channelSnapshot.last_decision_summary}`}
              </span>
            ) : null}
          </div>
        ) : null}
      </div>
      <div className="channel-actions">
        <button
          className="sidebar-btn"
          title={t('layout.channelHeader.searchTitle')}
          aria-label={t('layout.channelHeader.searchAria')}
          onClick={() => setSearchOpen(true)}
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="11" cy="11" r="8" />
            <path d="m21 21-4.3-4.3" />
          </svg>
        </button>
      </div>
    </div>
  )
}
