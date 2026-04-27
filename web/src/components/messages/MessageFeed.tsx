import { Suspense, lazy, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useMessages } from '../../hooks/useMessages'
import { useAppStore } from '../../stores/app'
import { useOfficeMembers } from '../../hooks/useMembers'
import { MessageBubble } from './MessageBubble'
import { formatTime } from '../../lib/format'
import type { Message } from '../../api/client'
import { buildThreadTree, normalizeChannel } from '../../lib/messageThreads'
import { getHumanAttentionRootIds } from '../../lib/threadAttention'
import { useVirtualWindow } from '../../lib/virtualWindow'
import { buildMessageFeedElements, buildRecentContextItems } from '../../lib/messageFeed'

const LazyInlineThread = lazy(() =>
  import('./InlineThread').then((module) => ({ default: module.InlineThread })),
)

export function MessageFeed() {
  const { t } = useTranslation()
  const currentChannel = useAppStore((s) => s.currentChannel)
  const activeThreadId = useAppStore((s) => s.activeThreadId)
  const activeThreadReplyTo = useAppStore((s) => s.activeThreadReplyTo)
  const setActiveThreadId = useAppStore((s) => s.setActiveThreadId)
  const setActiveThreadReplyTo = useAppStore((s) => s.setActiveThreadReplyTo)
  const { data: officeMembers = [] } = useOfficeMembers()
  const containerRef = useRef<HTMLDivElement>(null)
  const prevLastVisibleIdRef = useRef<string | null>(null)
  const prependingHeightRef = useRef<number | null>(null)
  const [expandedAutomationGroups, setExpandedAutomationGroups] = useState<Record<string, boolean>>({})

  const {
    data: messages = [],
    executionNodes = [],
    isLoading,
    isRefreshing,
    error,
    hasOlder,
    isFetchingOlder,
    loadOlder,
  } = useMessages(currentChannel)

  const humanAttentionRoots = useMemo(() => getHumanAttentionRootIds(executionNodes), [executionNodes])

  const visibleMessages = useMemo(() => {
    const targetChannel = normalizeChannel(currentChannel)
    const replyCounts = new Map<string, number>()

    for (const message of messages) {
      if (!message.reply_to) continue
      const target = message.reply_to
      replyCounts.set(target, (replyCounts.get(target) ?? 0) + 1)
    }

    return messages
      .filter((msg) => msg.content?.startsWith('[STATUS]') !== true)
      .filter((msg) => normalizeChannel(msg.channel || '') === targetChannel)
      .map((message) => ({
        ...message,
        thread_count: message.thread_count ?? replyCounts.get(message.id) ?? 0,
      }))
  }, [messages, currentChannel])

  const rootMessages = useMemo(
    () => buildThreadTree(visibleMessages).map((node) => node.message),
    [visibleMessages],
  )

  const elements = useMemo(() => buildMessageFeedElements(rootMessages), [rootMessages])
  const recentContext = useMemo(() => buildRecentContextItems(rootMessages), [rootMessages])

  const activeThreadIndex = useMemo(() => {
    if (!activeThreadId) return -1
    return elements.findIndex(
      (item) => item.type === 'message' && activeThreadId === (item.message.thread_id ?? item.message.id),
    )
  }, [activeThreadId, elements])

  const {
    startIndex,
    endIndex,
    totalHeight,
    offsets,
    registerItem,
  } = useVirtualWindow({
    items: elements,
    containerRef,
    getKey: (item) => item.key,
    estimateSize: (item) => {
      if (item.type === 'date') return 28
      if (item.type === 'automation-group') {
        const expanded =
          expandedAutomationGroups[item.key] ||
          item.messages.some((message) => activeThreadId === (message.thread_id ?? message.id))
        return expanded ? 112 + item.messages.length * 150 : 96
      }
      if (activeThreadId && activeThreadId === (item.message.thread_id ?? item.message.id)) return 380
      return 128
    },
    overscan: 8,
  })

  const renderStart = activeThreadIndex >= 0 ? Math.min(startIndex, Math.max(0, activeThreadIndex - 1)) : startIndex
  const renderEnd = activeThreadIndex >= 0 ? Math.max(endIndex, Math.min(elements.length, activeThreadIndex + 2)) : endIndex
  const paddingTop = offsets[renderStart] ?? 0
  const paddingBottom = Math.max(0, totalHeight - (offsets[renderEnd] ?? totalHeight))
  const visibleSlice = elements.slice(renderStart, renderEnd)

  const lastVisibleMessageId = useMemo(() => {
    for (let i = elements.length - 1; i >= 0; i -= 1) {
      const item = elements[i]
      if (item?.type === 'message') {
        return item.message.id
      }
    }
    return null
  }, [elements])

  useEffect(() => {
    if (prependingHeightRef.current != null && !isFetchingOlder && containerRef.current) {
      const nextHeight = containerRef.current.scrollHeight
      containerRef.current.scrollTop = nextHeight - prependingHeightRef.current
      prependingHeightRef.current = null
      return
    }
    if (
      lastVisibleMessageId &&
      lastVisibleMessageId !== prevLastVisibleIdRef.current &&
      containerRef.current
    ) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight
    }
    prevLastVisibleIdRef.current = lastVisibleMessageId
  }, [lastVisibleMessageId, isFetchingOlder])

  useEffect(() => {
    const node = containerRef.current
    if (!node) return
    const onScroll = () => {
      if (node.scrollTop > 120 || !hasOlder || isFetchingOlder) return
      prependingHeightRef.current = node.scrollHeight
      void loadOlder()
    }
    node.addEventListener('scroll', onScroll)
    return () => node.removeEventListener('scroll', onScroll)
  }, [hasOlder, isFetchingOlder, loadOlder])

  useEffect(() => {
    setExpandedAutomationGroups({})
  }, [currentChannel])

  if (isLoading && visibleMessages.length === 0) {
    return (
      <div className="messages" style={{ alignItems: 'center', justifyContent: 'center' }}>
        <span style={{ color: 'var(--text-tertiary)', fontSize: 13 }}>{t('messages.feed.loading')}</span>
      </div>
    )
  }

  if (error && visibleMessages.length === 0) {
    return (
      <div className="messages" style={{ alignItems: 'center', justifyContent: 'center' }}>
        <span style={{ color: 'var(--red)', fontSize: 13 }}>
          {t('messages.feed.loadFailed', { error })}
        </span>
      </div>
    )
  }

  if (visibleMessages.length === 0) {
    return (
      <div className="messages">
        <div className="channel-empty-state">
          <span className="eyebrow">{t('messages.feed.emptyEyebrow')}</span>
          <span className="title">{t('messages.feed.emptyTitle', { channel: currentChannel })}</span>
          <span className="body">{t('messages.feed.emptyBody')}</span>
          <div className="channel-empty-hints">
            <div>{t('messages.feed.emptyHint1Prefix')}<code>{t('messages.feed.emptyHint1Code')}</code></div>
            <div>
              {t('messages.feed.emptyHint2Prefix')}
              <code>{t('messages.feed.emptyHint2Code1')}</code>
              {t('messages.feed.emptyHint2Mid')}
              <code>{t('messages.feed.emptyHint2Code2')}</code>
              {t('messages.feed.emptyHint2Suffix')}
            </div>
          </div>
          <span className="channel-empty-foot">{t('messages.feed.emptyFoot')}</span>
        </div>
      </div>
    )
  }

  return (
    <div className="messages" ref={containerRef}>
      {isRefreshing ? (
        <div className="messages-refresh-indicator" role="status" aria-live="polite">
          {t('messages.feed.refreshing')}
        </div>
      ) : null}
      {error ? (
        <div
          style={{
            margin: '8px 12px 0',
            padding: '8px 10px',
            borderRadius: 'var(--radius-sm)',
            border: '1px solid color-mix(in srgb, var(--red) 35%, transparent)',
            background: 'color-mix(in srgb, var(--red) 8%, var(--bg-card))',
            color: 'var(--text)',
            fontSize: 12,
          }}
        >
          {t('messages.feed.loadFailed', { error })}
        </div>
      ) : null}
      {recentContext.length > 0 ? (
        <section className="message-feed-context" data-testid="message-feed-context">
          <div className="message-feed-context-head">
            <span className="message-feed-context-title">{t('messages.feed.recentContextTitle')}</span>
            <span className="message-feed-context-copy">{t('messages.feed.recentContextBody')}</span>
          </div>
          <div className="message-feed-context-list">
            {recentContext.map((item) => {
              const member = officeMembers.find((candidate) => candidate.slug === item.author)
              const authorLabel =
                item.author === 'you' || item.author === 'human'
                  ? t('messages.bubble.you')
                  : (member?.name || item.author)
              return (
                <div key={item.key} className="message-feed-context-item">
                  <div className="message-feed-context-meta">
                    <span>{authorLabel}</span>
                    <span>{formatTime(item.timestamp)}</span>
                  </div>
                  <div className="message-feed-context-preview">{item.preview}</div>
                </div>
              )
            })}
          </div>
        </section>
      ) : null}
      {(hasOlder || isFetchingOlder) && (
        <div className="date-separator">
          <div className="date-separator-line" />
          <span className="date-separator-text">
            {isFetchingOlder ? t('messages.feed.loadingHistory') : t('messages.feed.scrollForHistory')}
          </span>
          <div className="date-separator-line" />
        </div>
      )}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 0 }}>
        <div style={{ height: paddingTop }} />
        {visibleSlice.map((el, sliceIndex) => {
          const absoluteIndex = renderStart + sliceIndex
          if (el.type === 'date') {
            return (
              <div key={el.key} className="date-separator" ref={registerItem(absoluteIndex)}>
                <div className="date-separator-line" />
                <span className="date-separator-text">{el.label}</span>
                <div className="date-separator-line" />
              </div>
            )
          }
          if (el.type === 'automation-group') {
            const expanded =
              expandedAutomationGroups[el.key] ||
              el.messages.some((message) => activeThreadId === (message.thread_id ?? message.id))
            return (
              <div
                key={el.key}
                className="message-automation-group"
                data-testid="message-feed-automation-group"
                ref={registerItem(absoluteIndex)}
              >
                <div className="message-automation-group-summary">
                  <div className="message-automation-group-copy">
                    <div className="message-automation-group-title">
                      {t('messages.feed.automationBurstLabel', { count: el.count, source: el.sourceLabel })}
                    </div>
                    <div className="message-automation-group-preview">{el.preview}</div>
                  </div>
                  <button
                    type="button"
                    className="message-automation-group-toggle"
                    onClick={() => {
                      setExpandedAutomationGroups((current) => ({
                        ...current,
                        [el.key]: !expanded,
                      }))
                    }}
                  >
                    {expanded
                      ? t('messages.feed.hideAutomationBurst')
                      : t('messages.feed.showAutomationBurst', { count: el.count })}
                  </button>
                </div>
                {expanded ? (
                  <div className="message-automation-group-list">
                    {el.messages.map((message) => (
                      <div key={message.id} className="message-thread-block">
                        <MessageBubble
                          message={message}
                          members={officeMembers}
                          canDelete={message.can_delete === true}
                          attentionLabel={
                            humanAttentionRoots.has(message.id)
                              ? t('messages.thread.humanAttentionBadge')
                              : undefined
                          }
                          onDeleted={(deleted) => {
                            if (activeThreadId === deleted.id) {
                              setActiveThreadId(null)
                            }
                            if (activeThreadReplyTo === deleted.id) {
                              setActiveThreadReplyTo(null)
                            }
                          }}
                          onThreadClick={(id) => {
                            const nextThreadId = message.thread_id ?? id
                            if (activeThreadId === nextThreadId) {
                              setActiveThreadId(null)
                              setActiveThreadReplyTo(null)
                              return
                            }
                            setActiveThreadId(nextThreadId)
                            setActiveThreadReplyTo(null)
                          }}
                          onReply={(id) => {
                            const nextThreadId = message.thread_id ?? id
                            setActiveThreadId(nextThreadId)
                            setActiveThreadReplyTo(id)
                          }}
                        />
                        {activeThreadId === (message.thread_id ?? message.id) ? (
                          <Suspense
                            fallback={
                              <div className="inline-thread-panel" style={{ marginTop: 10, minHeight: 180 }}>
                                <div className="inline-thread-empty">{t('messages.thread.loading')}</div>
                              </div>
                            }
                          >
                            <LazyInlineThread threadId={activeThreadId} />
                          </Suspense>
                        ) : null}
                      </div>
                    ))}
                  </div>
                ) : null}
              </div>
            )
          }
          return (
            <div key={el.key} className="message-thread-block" ref={registerItem(absoluteIndex)}>
              <MessageBubble
                message={el.message}
                members={officeMembers}
                grouped={el.grouped}
                threadDepth={el.threadDepth}
                threadParentLabel={el.threadParentLabel}
                canDelete={el.message.can_delete === true}
                attentionLabel={
                  humanAttentionRoots.has(el.message.id)
                    ? t('messages.thread.humanAttentionBadge')
                    : undefined
                }
                onDeleted={(deleted) => {
                  if (activeThreadId === deleted.id) {
                    setActiveThreadId(null)
                  }
                  if (activeThreadReplyTo === deleted.id) {
                    setActiveThreadReplyTo(null)
                  }
                }}
                onThreadClick={(id) => {
                  const nextThreadId = el.message.thread_id ?? id
                  if (activeThreadId === nextThreadId) {
                    setActiveThreadId(null)
                    setActiveThreadReplyTo(null)
                    return
                  }
                  setActiveThreadId(nextThreadId)
                  setActiveThreadReplyTo(null)
                }}
                onReply={(id) => {
                  const nextThreadId = el.message.thread_id ?? id
                  setActiveThreadId(nextThreadId)
                  setActiveThreadReplyTo(id)
                }}
              />
              {activeThreadId === (el.message.thread_id ?? el.message.id) ? (
                <Suspense
                  fallback={
                    <div className="inline-thread-panel" style={{ marginTop: 10, minHeight: 180 }}>
                      <div className="inline-thread-empty">{t('messages.thread.loading')}</div>
                    </div>
                  }
                >
                  <LazyInlineThread threadId={activeThreadId} />
                </Suspense>
              ) : null}
            </div>
          )
        })}
        <div style={{ height: paddingBottom }} />
      </div>
    </div>
  )
}
