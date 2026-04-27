import { type CSSProperties, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useThreadMessages } from '../../hooks/useMessages'
import { useOfficeMembers } from '../../hooks/useMembers'
import { useAppStore } from '../../stores/app'
import { type Message } from '../../api/client'
import { formatTime } from '../../lib/format'
import { showNotice } from '../ui/Toast'
import { MessageBubble } from './MessageBubble'
import {
  buildInlineThreadContextItems,
  buildInlineThreadDisplayNodes,
  buildThreadTree,
  flattenThreadTree,
} from '../../lib/messageThreads'
import { useDurableMessageComposer } from '../../hooks/useDurableMessageComposer'
import { getHumanAttentionNodes, getHumanAttentionReason } from '../../lib/threadAttention'
import { useVirtualWindow } from '../../lib/virtualWindow'

function getDisplayName(message: Message, members: { slug: string; name?: string }[]): string {
  if (message.from === 'you' || message.from === 'human') return message.from
  const member = members.find((m) => m.slug === message.from)
  return member?.name ?? message.from
}

export function InlineThread({ threadId }: { threadId: string }) {
  const { t } = useTranslation()
  const currentChannel = useAppStore((s) => s.currentChannel)
  const setActiveThreadId = useAppStore((s) => s.setActiveThreadId)
  const activeThreadReplyTo = useAppStore((s) => s.activeThreadReplyTo)
  const setActiveThreadReplyTo = useAppStore((s) => s.setActiveThreadReplyTo)
  const { data: officeMembers = [] } = useOfficeMembers()
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const messagesRef = useRef<HTMLDivElement>(null)
  const shouldStickToBottomRef = useRef(true)
  const [expandedAutomationGroups, setExpandedAutomationGroups] = useState<Record<string, boolean>>({})
  const {
    data: replies = [],
    executionNodes = [],
    isLoading,
    isRefreshing,
    error,
    refetch,
  } = useThreadMessages(currentChannel, threadId)

  const messagesById = useMemo(
    () => new Map<string, Message>(replies.map((message) => [message.id, message])),
    [replies],
  )
  const replyTarget = activeThreadReplyTo ? messagesById.get(activeThreadReplyTo) : null

  const getAuthorLabel = (message: Message): string =>
    message.from === 'you' || message.from === 'human'
      ? t('messages.bubble.you')
      : getDisplayName(message, officeMembers)

  const threadContextLabel = activeThreadReplyTo && replyTarget
    ? t('messages.thread.replyingTo', { author: getAuthorLabel(replyTarget) })
    : t('messages.thread.threadRoot')

  const threadTree = useMemo(() => buildThreadTree(replies), [replies])
  const flatNodes = useMemo(
    () => flattenThreadTree(threadTree, messagesById, getAuthorLabel, t),
    [getAuthorLabel, messagesById, t, threadTree],
  )
  const displayNodes = useMemo(() => buildInlineThreadDisplayNodes(flatNodes), [flatNodes])
  const recentThreadContext = useMemo(
    () => buildInlineThreadContextItems(replies, threadId, getAuthorLabel),
    [getAuthorLabel, replies, threadId],
  )
  const humanAttentionNodes = useMemo(() => getHumanAttentionNodes(executionNodes), [executionNodes])
  const humanAttentionReason = useMemo(() => getHumanAttentionReason(executionNodes), [executionNodes])
  const memberSlugs = useMemo(() => officeMembers.map((member) => member.slug), [officeMembers])

  const {
    text,
    setText,
    retryLastFailed,
    sendMessage,
    delivery,
    isSending,
  } = useDurableMessageComposer({
    channel: currentChannel,
    replyTo: activeThreadReplyTo ?? threadId,
    memberSlugs,
    onPersisted: () => {
      shouldStickToBottomRef.current = true
      setActiveThreadReplyTo(null)
      void refetch()
    },
  })

  const {
    startIndex,
    endIndex,
    totalHeight,
    offsets,
    registerItem,
  } = useVirtualWindow({
    items: displayNodes,
    containerRef: messagesRef,
    getKey: (item) => item.key,
    estimateSize: (item) =>
      item.type === 'automation-group'
        ? (expandedAutomationGroups[item.key] ? 96 + item.nodes.length * 108 : 96)
        : 84 + item.threadDepth * 16,
    overscan: 8,
  })

  useEffect(() => {
    setExpandedAutomationGroups({})
  }, [threadId])

  useEffect(() => {
    if (messagesRef.current && shouldStickToBottomRef.current) {
      messagesRef.current.scrollTop = messagesRef.current.scrollHeight
    }
  }, [displayNodes.length, expandedAutomationGroups])

  const handleSend = async () => {
    const trimmed = text.trim()
    if (!trimmed || isSending) return
    try {
      await sendMessage()
    } catch (err) {
      const message = err instanceof Error ? err.message : t('messages.thread.failedSend')
      showNotice(message, 'error')
    }
  }

  return (
    <div className="inline-thread-panel" data-thread-id={threadId}>
      <div className="inline-thread-header">
        <div className="thread-panel-context">
          <span className="thread-panel-context-chip">{threadContextLabel}</span>
          {activeThreadReplyTo ? (
            <button className="thread-panel-context-btn" onClick={() => setActiveThreadReplyTo(null)}>
              {t('messages.thread.replyToRoot')}
            </button>
          ) : null}
        </div>
        <button
          className="thread-panel-close inline-thread-close"
          onClick={() => {
            setActiveThreadId(null)
            setActiveThreadReplyTo(null)
          }}
          aria-label={t('messages.thread.closeAria')}
        >
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <path d="M18 6 6 18" />
            <path d="m6 6 12 12" />
          </svg>
        </button>
      </div>

      {humanAttentionNodes.length > 0 ? (
        <div className="thread-human-attention-banner" role="status" aria-live="polite">
          <span className="thread-human-attention-eyebrow">{t('messages.thread.humanAttentionBadge')}</span>
          <span>{humanAttentionReason || t('messages.thread.humanAttentionBody')}</span>
        </div>
      ) : null}

      <div
        className="inline-thread-body"
        ref={messagesRef}
        onScroll={() => {
          const node = messagesRef.current
          if (!node) return
          const distanceFromBottom = node.scrollHeight - node.scrollTop - node.clientHeight
          shouldStickToBottomRef.current = distanceFromBottom < 48
        }}
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: 0 }}>
          {isRefreshing ? (
            <div className="inline-thread-status" role="status" aria-live="polite">
              {t('messages.thread.refreshing')}
            </div>
          ) : null}
          {recentThreadContext.length > 0 ? (
            <section className="message-feed-context inline-thread-context" data-testid="inline-thread-context">
              <div className="message-feed-context-head">
                <div className="message-feed-context-title">{t('messages.feed.recentContextTitle')}</div>
                <div className="message-feed-context-copy">{t('messages.feed.recentContextBody')}</div>
              </div>
              <div className="message-feed-context-list">
                {recentThreadContext.map((item) => (
                  <div key={item.key} className="message-feed-context-item">
                    <div className="message-feed-context-meta">
                      <strong>{item.author}</strong>
                      <span>{formatTime(item.timestamp)}</span>
                    </div>
                    <div className="message-feed-context-preview">{item.preview}</div>
                  </div>
                ))}
              </div>
            </section>
          ) : null}
          {error && displayNodes.length === 0 ? (
            <div className="inline-thread-empty">{t('messages.thread.loadFailed', { error })}</div>
          ) : isLoading && replies.length === 0 ? (
            <div className="inline-thread-empty">{t('messages.thread.loading')}</div>
          ) : displayNodes.length === 0 ? (
            <div className="inline-thread-empty">{t('messages.thread.empty')}</div>
          ) : (
            <>
              {error ? (
                <div
                  style={{
                    marginBottom: 12,
                    padding: '8px 10px',
                    borderRadius: 'var(--radius-sm)',
                    border: '1px solid color-mix(in srgb, var(--red) 35%, transparent)',
                    background: 'color-mix(in srgb, var(--red) 8%, var(--bg-card))',
                    color: 'var(--text)',
                    fontSize: 12,
                  }}
                >
                  {t('messages.thread.loadFailed', { error })}
                </div>
              ) : null}
              <div style={{ height: offsets[startIndex] ?? 0 }} />
              {displayNodes.slice(startIndex, endIndex).map((node, sliceIndex) => {
                const absoluteIndex = startIndex + sliceIndex
                return (
                  <div
                    key={node.key}
                    className={`thread-tree-node ${node.threadDepth === 0 ? 'thread-tree-node-root' : ''}`}
                    style={{
                      '--thread-tree-indent': `${Math.min(node.threadDepth, 6) * 18}px`,
                    } as CSSProperties}
                    role="listitem"
                    ref={registerItem(absoluteIndex)}
                  >
                    {node.type === 'automation-group' ? (
                      <div
                        className="message-automation-group"
                        data-testid="inline-thread-automation-group"
                      >
                        <div className="message-automation-group-summary">
                          <div className="message-automation-group-copy">
                            <div className="message-automation-group-title">
                              {t('messages.feed.automationBurstLabel', {
                                count: node.count,
                                source: node.sourceLabel,
                              })}
                            </div>
                            <div className="message-automation-group-preview">{node.preview}</div>
                          </div>
                          <button
                            className="message-automation-group-toggle"
                            onClick={() =>
                              setExpandedAutomationGroups((current) => ({
                                ...current,
                                [node.key]: !current[node.key],
                              }))
                            }
                          >
                            {expandedAutomationGroups[node.key]
                              ? t('messages.feed.hideAutomationBurst')
                              : t('messages.feed.showAutomationBurst', { count: node.count })}
                          </button>
                        </div>
                        {expandedAutomationGroups[node.key] ? (
                          <div className="message-automation-group-list">
                            {node.nodes.map((entry) => (
                              <MessageBubble
                                key={entry.key}
                                message={entry.message}
                                members={officeMembers}
                                canDelete={entry.message.can_delete === true}
                                threadDepth={0}
                                threadParentLabel={entry.threadParentLabel}
                                onDeleted={(deleted) => {
                                  if (activeThreadReplyTo === deleted.id) {
                                    setActiveThreadReplyTo(null)
                                  }
                                  if (threadId === deleted.id) {
                                    setActiveThreadId(null)
                                  }
                                }}
                                onThreadClick={(messageId) => {
                                  const messageThreadId = entry.message.thread_id ?? messageId
                                  setActiveThreadId(messageThreadId)
                                  setActiveThreadReplyTo(null)
                                }}
                                onReply={(messageId) => {
                                  setActiveThreadReplyTo(messageId)
                                }}
                              />
                            ))}
                          </div>
                        ) : null}
                      </div>
                    ) : (
                      <div className="thread-tree-node-main">
                        <MessageBubble
                          message={node.message}
                          members={officeMembers}
                          canDelete={node.message.can_delete === true}
                          threadDepth={0}
                          threadParentLabel={
                            node.message.reply_to
                              ? messagesById.get(node.message.reply_to)
                                ? t('messages.thread.replyTo', {
                                    author: getAuthorLabel(messagesById.get(node.message.reply_to)!),
                                  })
                                : t('messages.thread.replyToMissing')
                              : undefined
                          }
                          onDeleted={(deleted) => {
                            if (activeThreadReplyTo === deleted.id) {
                              setActiveThreadReplyTo(null)
                            }
                            if (threadId === deleted.id) {
                              setActiveThreadId(null)
                            }
                          }}
                          onThreadClick={(messageId) => {
                            const messageThreadId = node.message.thread_id ?? messageId
                            setActiveThreadId(messageThreadId)
                            setActiveThreadReplyTo(null)
                          }}
                          onReply={(messageId) => {
                            setActiveThreadReplyTo(messageId)
                          }}
                        />
                      </div>
                    )}
                  </div>
                )
              })}
              <div style={{ height: Math.max(0, totalHeight - (offsets[endIndex] ?? totalHeight)) }} />
            </>
          )}
        </div>
      </div>

      <div className="composer inline-thread-composer">
        <div className="composer-context-hint">
          {activeThreadReplyTo
            ? t('messages.thread.composerReplyHint', {
                author: replyTarget ? getAuthorLabel(replyTarget) : t('messages.thread.threadRoot'),
              })
            : t('messages.thread.composerRootHint')}
        </div>
        <div className="composer-inner">
          <textarea
            ref={textareaRef}
            className="composer-input"
            placeholder={t('messages.thread.replyPlaceholder')}
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                handleSend()
              }
            }}
            rows={1}
          />
          <button
            className="composer-send"
            disabled={!text.trim() || isSending}
            onClick={() => void handleSend()}
            aria-label={t('messages.thread.sendAria')}
          >
            <svg
              width="16"
              height="16"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <path d="m22 2-7 20-4-9-9-4Z" />
              <path d="M22 2 11 13" />
            </svg>
          </button>
        </div>
        {delivery.kind !== 'idle' ? (
          <div className={`composer-status composer-status--${delivery.kind}`}>
            <span>
              {delivery.kind === 'draft' ? t('messages.composer.statusDraft') : null}
              {delivery.kind === 'sending' ? t('messages.composer.statusSending') : null}
              {delivery.kind === 'persisted' ? t('messages.composer.statusPersisted') : null}
              {delivery.kind === 'failed' ? t('messages.composer.statusFailed') : null}
            </span>
            {delivery.kind === 'failed' ? (
              <button className="composer-status-action" onClick={() => void retryLastFailed()}>
                {t('messages.composer.retryPersist')}
              </button>
            ) : null}
          </div>
        ) : null}
      </div>
    </div>
  )
}
