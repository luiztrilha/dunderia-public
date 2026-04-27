import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getStudioDevConsole, type StudioChannelSnapshot } from '../../api/client'
import { useChannels } from '../../hooks/useChannels'
import { useBrokerRefetchInterval } from '../../hooks/useBrokerEvents'
import { prefetchChannelMessages } from '../../hooks/useMessages'
import { useAppStore } from '../../stores/app'
import { ChannelWizard, useChannelWizard } from '../channels/ChannelWizard'

export function ChannelList() {
  const { t } = useTranslation()
  const { data: channels = [] } = useChannels()
  const studioRefetchInterval = useBrokerRefetchInterval(15_000)
  const studioQuery = useQuery({
    queryKey: ['studio-dev-console'],
    queryFn: () => getStudioDevConsole(),
    refetchInterval: studioRefetchInterval,
    staleTime: 30_000,
  })
  const currentChannel = useAppStore((s) => s.currentChannel)
  const setCurrentChannel = useAppStore((s) => s.setCurrentChannel)
  const currentApp = useAppStore((s) => s.currentApp)
  const wizard = useChannelWizard()
  const studioChannels = studioQuery.data?.active_context.channels ?? []
  const orderedChannels = useMemo(
    () => sortChannelsByOperationalPriority(channels, studioChannels),
    [channels, studioChannels],
  )
  const snapshotsByChannel = useMemo(
    () => Object.fromEntries(studioChannels.map((channel) => [channel.slug, channel])),
    [studioChannels],
  )

  return (
    <>
      <div className="sidebar-channels">
        {orderedChannels.map((ch) => {
          const isActive = currentChannel === ch.slug && !currentApp
          const snapshot = snapshotsByChannel[ch.slug]
          const waitingCount = snapshot?.waiting_human_count ?? 0
          const blockedCount = snapshot?.task_counts.blocked ?? 0
          const attentionCount = snapshot?.attention_count ?? 0
          const itemClassName =
            `sidebar-item${isActive ? ' active' : ''}`
            + (blockedCount > 0 ? ' sidebar-item-attention' : '')
            + (blockedCount === 0 && waitingCount > 0 ? ' sidebar-item-waiting' : '')
          return (
            <button
              key={ch.slug}
              className={itemClassName}
              onClick={() => setCurrentChannel(ch.slug)}
              onMouseEnter={() => void prefetchChannelMessages(ch.slug)}
              onFocus={() => void prefetchChannelMessages(ch.slug)}
            >
              <span style={{ fontSize: 13, color: 'var(--text-tertiary)', width: 18, textAlign: 'center', flexShrink: 0 }}>
                #
              </span>
              <span style={{ display: 'flex', flexDirection: 'column', minWidth: 0, alignItems: 'flex-start', gap: 2 }}>
                <span>{ch.name || ch.slug}</span>
                {snapshot?.last_substantive_update_by ? (
                  <span
                    style={{
                      fontSize: 10,
                      color: 'var(--text-tertiary)',
                      whiteSpace: 'nowrap',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      maxWidth: '100%',
                    }}
                    title={snapshot.last_substantive_preview || undefined}
                  >
                    {t('sidebar.channels.updatedBy', { agent: snapshot.last_substantive_update_by })}
                  </span>
                ) : null}
              </span>
              {snapshot ? (
                <span className="sidebar-channel-badges">
                  {waitingCount > 0 ? (
                    <span title={t('sidebar.channels.waitingTitle', { count: waitingCount })} className="sidebar-badge sidebar-badge-waiting">
                      {t('sidebar.channels.waitingBadge', { count: waitingCount })}
                    </span>
                  ) : null}
                  {blockedCount > 0 ? (
                    <span title={t('sidebar.channels.blockedTitle', { count: blockedCount })} className="sidebar-badge sidebar-badge-attention">
                      {t('sidebar.channels.blockedBadge', { count: blockedCount })}
                    </span>
                  ) : null}
                  {attentionCount > 0 ? (
                    <span title={t('sidebar.channels.attentionTitle', { count: attentionCount })} className="sidebar-badge sidebar-badge-attention-soft">
                      {t('sidebar.channels.attentionBadge', { count: attentionCount })}
                    </span>
                  ) : null}
                </span>
              ) : null}
            </button>
          )
        })}
        <button
          className="sidebar-item sidebar-add-btn"
          onClick={wizard.show}
          title={t('sidebar.channels.newChannelTitle')}
        >
          <span style={{ fontSize: 14, width: 18, textAlign: 'center', flexShrink: 0 }}>+</span>
          <span>{t('sidebar.channels.newChannel')}</span>
        </button>
      </div>
      <ChannelWizard open={wizard.open} onClose={wizard.hide} />
    </>
  )
}

function sortChannelsByOperationalPriority<T extends { slug: string }>(
  channels: T[],
  studioChannels: StudioChannelSnapshot[],
) {
  const order = new Map(studioChannels.map((channel, index) => [channel.slug, index]))
  return [...channels]
    .map((channel, index) => ({ channel, index }))
    .sort((left, right) => {
      const leftOrder = order.get(left.channel.slug)
      const rightOrder = order.get(right.channel.slug)
      if (leftOrder !== undefined || rightOrder !== undefined) {
        if (leftOrder === undefined) return 1
        if (rightOrder === undefined) return -1
        if (leftOrder !== rightOrder) return leftOrder - rightOrder
      }
      return left.index - right.index
    })
    .map(({ channel }) => channel)
}
