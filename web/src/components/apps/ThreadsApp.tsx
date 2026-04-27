import { startTransition, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { getMessageThreads, getStudioDevConsole, type MessageThreadSummary, type StudioChannelSnapshot } from '../../api/client'
import { useBrokerRefetchInterval } from '../../hooks/useBrokerEvents'
import { useAppStore } from '../../stores/app'
import { useOfficeMembers } from '../../hooks/useMembers'
import { PixelAvatar } from '../ui/PixelAvatar'
import { formatRelativeTime } from '../../lib/format'
import { usePageActivity } from '../../lib/pageActivity'
import { useVirtualWindow } from '../../lib/virtualWindow'

/**
 * All-threads surface. Uses a broker summary instead of fetching the latest
 * window from every channel.
 */
export function ThreadsApp() {
  const { t } = useTranslation()
  const { isPageActive } = usePageActivity()
  const setCurrentApp = useAppStore((s) => s.setCurrentApp)
  const setCurrentChannel = useAppStore((s) => s.setCurrentChannel)
  const setActiveThreadId = useAppStore((s) => s.setActiveThreadId)
  const setLastMessageId = useAppStore((s) => s.setLastMessageId)
  const { data: members = [] } = useOfficeMembers()
  const listRef = useRef<HTMLDivElement>(null)
  const studioRefetchInterval = useBrokerRefetchInterval(15_000)

  const { data, isLoading } = useQuery({
    queryKey: ['message-threads'],
    queryFn: () => getMessageThreads({ limit: 100 }),
    refetchInterval: isPageActive ? 20_000 : false,
  })
  const studioQuery = useQuery({
    queryKey: ['studio-dev-console'],
    queryFn: () => getStudioDevConsole(),
    refetchInterval: studioRefetchInterval,
    staleTime: 30_000,
  })
  const threads = data?.threads ?? []
  const studioChannels = studioQuery.data?.active_context.channels ?? []

  const rows = useMemo(
    () =>
      threads
        .map((row, index) => {
          const agent = members.find((m) => m.slug === row.message.from)
          const preview = row.message.content && row.message.content.length > 120
            ? row.message.content.slice(0, 120) + '\u2026'
            : (row.message.content || t('apps.threads.noContent'))

          return {
            key: `${row.channel}-${row.thread_id}`,
            row,
            agent,
            preview,
            snapshot: studioChannels.find((channel) => channel.slug === row.channel),
            index,
          }
        })
        .sort((left, right) => compareThreadsByChannelPriority(left.snapshot, right.snapshot, left.index, right.index)),
    [members, studioChannels, t, threads],
  )

  const openThread = (row: MessageThreadSummary) => {
    startTransition(() => {
      setCurrentApp(null)
      setCurrentChannel(row.channel)
      setLastMessageId(null)
      setActiveThreadId(row.thread_id)
    })
  }

  const {
    startIndex,
    endIndex,
    totalHeight,
    offsets,
    registerItem,
  } = useVirtualWindow({
    items: rows,
    containerRef: listRef,
    getKey: (item) => item.key,
    estimateSize: () => 104,
    overscan: 8,
  })

  return (
    <div className="threads-view" style={{ overflowY: 'hidden' }}>
      <div className="threads-view-header">
        <span className="threads-view-title">{t('apps.threads.title')}</span>
        <span className="threads-view-count">
          {t('apps.threads.activeCount', { count: threads.length })}
        </span>
      </div>

      {isLoading && threads.length === 0 ? (
        <div className="threads-view-empty">{t('apps.threads.loading')}</div>
      ) : threads.length === 0 ? (
        <div className="threads-view-empty">{t('apps.threads.empty')}</div>
      ) : (
        <div className="threads-view-list" ref={listRef} style={{ flex: 1, overflowY: 'auto', gap: 0 }}>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 0 }}>
            <div style={{ height: offsets[startIndex] ?? 0 }} />
            {rows.slice(startIndex, endIndex).map((item, sliceIndex) => {
              const { row, agent, preview } = item
              const absoluteIndex = startIndex + sliceIndex
              return (
                <button
                  key={item.key}
                  type="button"
                  className="thread-list-item"
                  onClick={() => openThread(row)}
                  ref={registerItem(absoluteIndex)}
                >
                  <div className="thread-list-item-avatar">
                    {agent ? (
                      <PixelAvatar slug={agent.slug} size={32} />
                    ) : (
                      <span style={{ fontSize: 22 }}>{'\uD83D\uDCAC'}</span>
                    )}
                  </div>
                  <div className="thread-list-item-body">
                    <div className="thread-list-item-preview">{preview}</div>
                    <div className="thread-list-item-meta">
                      <span className="thread-list-item-replies">
                        {t('apps.threads.replies', { count: row.reply_count })}
                      </span>
                      {agent && <span>{agent.name}</span>}
                      <span>#{row.channel}</span>
                      {item.snapshot ? (
                        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4, flexWrap: 'wrap' }}>
                          {(item.snapshot.waiting_human_count ?? 0) > 0 ? (
                            <span
                              title={t('sidebar.channels.waitingTitle', { count: item.snapshot.waiting_human_count ?? 0 })}
                              style={badgeStyle('rgba(198, 68, 68, 0.12)', '#b24a4a')}
                            >
                              {t('sidebar.channels.waitingBadge', { count: item.snapshot.waiting_human_count ?? 0 })}
                            </span>
                          ) : null}
                          {(item.snapshot.task_counts.blocked ?? 0) > 0 ? (
                            <span
                              title={t('sidebar.channels.blockedTitle', { count: item.snapshot.task_counts.blocked ?? 0 })}
                              style={badgeStyle('rgba(183, 112, 34, 0.12)', '#9f651f')}
                            >
                              {t('sidebar.channels.blockedBadge', { count: item.snapshot.task_counts.blocked ?? 0 })}
                            </span>
                          ) : null}
                          {(item.snapshot.attention_count ?? 0) > 0 ? (
                            <span
                              title={t('sidebar.channels.attentionTitle', { count: item.snapshot.attention_count ?? 0 })}
                              style={badgeStyle('rgba(73, 127, 77, 0.12)', '#3b7b54')}
                            >
                              {t('sidebar.channels.attentionBadge', { count: item.snapshot.attention_count ?? 0 })}
                            </span>
                          ) : null}
                        </span>
                      ) : null}
                      {(row.last_reply_at || row.message.timestamp) && (
                        <span>{formatRelativeTime(row.last_reply_at || row.message.timestamp)}</span>
                      )}
                    </div>
                  </div>
                </button>
              )
            })}
            <div style={{ height: Math.max(0, totalHeight - (offsets[endIndex] ?? totalHeight)) }} />
          </div>
        </div>
      )}
    </div>
  )
}

function compareThreadsByChannelPriority(
  left: StudioChannelSnapshot | undefined,
  right: StudioChannelSnapshot | undefined,
  leftIndex: number,
  rightIndex: number,
) {
  const leftAttention = left?.attention_count ?? 0
  const rightAttention = right?.attention_count ?? 0
  if (leftAttention !== rightAttention) return rightAttention - leftAttention

  const leftWaiting = left?.waiting_human_count ?? 0
  const rightWaiting = right?.waiting_human_count ?? 0
  if (leftWaiting !== rightWaiting) return rightWaiting - leftWaiting

  const leftBlocked = left?.task_counts.blocked ?? 0
  const rightBlocked = right?.task_counts.blocked ?? 0
  if (leftBlocked !== rightBlocked) return rightBlocked - leftBlocked

  return leftIndex - rightIndex
}

function badgeStyle(background: string, color: string) {
  return {
    display: 'inline-flex',
    alignItems: 'center',
    padding: '2px 6px',
    borderRadius: 999,
    background,
    color,
    fontSize: 10,
    fontWeight: 700,
    letterSpacing: '0.02em',
    whiteSpace: 'nowrap' as const,
  }
}
