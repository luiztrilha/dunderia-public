import { type CSSProperties, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import type { Message } from '../../api/client'
import { formatTime, formatTokens } from '../../lib/format'
import { formatMarkdown } from '../../lib/markdown'
import { prefetchThreadMessages } from '../../hooks/useMessages'
import { dispatchChannelMessageDeleted, dispatchChannelMessagesRefresh } from '../../lib/messageEvents'
import { useAppStore } from '../../stores/app'
import { deleteMessage, toggleReaction } from '../../api/client'
import { PixelAvatar } from '../ui/PixelAvatar'
import { confirm } from '../ui/ConfirmDialog'
import { showNotice } from '../ui/Toast'
import { getThreadRole } from '../../lib/messageThreads'

interface MessageBubbleMember {
  slug: string
  name?: string
  role?: string
}

interface MessageBubbleProps {
  message: Message
  members: MessageBubbleMember[]
  grouped?: boolean
  onThreadClick?: (id: string) => void
  onReply?: (id: string) => void
  threadDepth?: number
  threadParentLabel?: string
  attentionLabel?: string
  canDelete?: boolean
  onDeleted?: (message: Message) => void | Promise<void>
}

function compactRoleLabel(role?: string | null): string | null {
  const trimmed = role?.trim()
  if (!trimmed) return null
  const firstSegment = trimmed.split(/[,.;\n]/)[0]?.trim() || trimmed
  if (firstSegment.length <= 44) return firstSegment
  return `${firstSegment.slice(0, 41).trimEnd()}...`
}

export function MessageBubble({
  message,
  members,
  grouped = false,
  onThreadClick,
  onReply,
  threadDepth,
  threadParentLabel,
  attentionLabel,
  canDelete = false,
  onDeleted,
}: MessageBubbleProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentChannel = useAppStore((s) => s.currentChannel)
  const isHuman = message.from === 'you' || message.from === 'human'
  const agent = members.find((m) => m.slug === message.from)
  const compactRole = compactRoleLabel(agent?.role)
  const [isDeleting, setIsDeleting] = useState(false)

  const requestDelete = () => {
    if (isDeleting) return
    confirm({
      title: t('messages.bubble.deleteConfirmTitle'),
      message: t('messages.bubble.deleteConfirmBody'),
      confirmLabel: t('messages.bubble.deleteAction'),
      danger: true,
      onConfirm: async () => {
        setIsDeleting(true)
        try {
          const response = await deleteMessage(message.id, targetChannel)
          dispatchChannelMessageDeleted(targetChannel, message.id, { threadId: response.thread_id })
          await queryClient.invalidateQueries({ queryKey: ['message-threads'] })
          dispatchChannelMessagesRefresh(targetChannel, { forceFull: true })
          await onDeleted?.(message)
          showNotice(t('messages.bubble.deleteSuccess'), 'success')
        } catch (e) {
          const error = e instanceof Error ? e.message : t('messages.bubble.deleteFailedFallback')
          showNotice(t('messages.bubble.deleteFailed', { error }), 'error')
        } finally {
          setIsDeleting(false)
        }
      },
    })
  }

  // Status messages — compact
  if (message.content?.startsWith('[STATUS]')) {
    const statusText = message.content.replace(/^\[STATUS\]\s*/, '')
    return <div className="message-status animate-fade">{statusText}</div>
  }

  const usageTotal = message.usage
    ? (message.usage.total_tokens ?? (
        (message.usage.input_tokens ?? 0) +
        (message.usage.output_tokens ?? 0) +
        (message.usage.cache_read_tokens ?? 0) +
        (message.usage.cache_creation_tokens ?? 0)
      ))
    : 0

  const reactions = message.reactions
    ? (Array.isArray(message.reactions)
        ? message.reactions as Array<{ emoji: string; count?: number }>
        : Object.entries(message.reactions).map(([emoji, users]) => ({
            emoji,
            count: Array.isArray(users) ? users.length : 1,
          })))
    : []

  // Agent markdown is HTML-escaped and link schemes are sanitized before render.
  // Human input continues to render as plain text.
  const renderedHtml = !isHuman ? formatMarkdown(message.content || '') : ''
  const depth = threadDepth ?? 0
  const clampedDepth = Math.min(depth, 8)
  const nestedStyle =
    depth > 0
      ? ({
          '--thread-depth': String(clampedDepth),
          '--thread-rail-offset': `${Math.max(0, clampedDepth - 1) * 18}px`,
        } as CSSProperties)
      : undefined
  const hasThreadLink = (message.thread_count ?? 0) > 0 && onThreadClick
  const hasReplyAction = !!onReply
  const hasDeleteAction = canDelete
  const threadRole = getThreadRole(message)
  const threadId = message.thread_id ?? message.id
  const targetChannel = message.channel || currentChannel

  return (
    <div
      className={`message animate-fade${grouped ? ' message-grouped' : ''}${threadDepth ? ' message-threaded' : ''}${attentionLabel ? ' message-needs-human-attention' : ''} message-thread-role-${threadRole}`}
      data-msg-id={message.id}
      style={nestedStyle}
    >
      {/* Avatar */}
      <div
        className="message-avatar"
      >
        <PixelAvatar slug={isHuman ? 'you' : message.from} size={36} />
      </div>

      {/* Content */}
      <div className={`message-content${hasDeleteAction ? ' message-content-has-visual-action' : ''}`}>
        {hasDeleteAction ? (
          <div className="message-visual-actions">
            <button
              type="button"
              className="message-visual-action message-visual-action-delete"
              onClick={requestDelete}
              aria-label={t('messages.bubble.deleteAria')}
              title={t('messages.bubble.deleteAction')}
              disabled={isDeleting}
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M3 6h18" />
                <path d="M8 6V4h8v2" />
                <path d="m19 6-1 14H6L5 6" />
                <path d="M10 11v6" />
                <path d="M14 11v6" />
              </svg>
            </button>
          </div>
        ) : null}
        {threadParentLabel ? (
          <div className="message-thread-link">
            <span className="message-thread-link-dot" />
            <span>{threadParentLabel}</span>
          </div>
        ) : null}

        {/* Header */}
        <div className="message-header">
          <span className="message-author">
            {isHuman ? t('messages.bubble.you') : (agent?.name || message.from)}
          </span>
          {isHuman ? (
            <span className="badge badge-yellow">{t('messages.bubble.human')}</span>
          ) : compactRole ? (
            <span className="badge badge-green message-role-badge" title={agent?.role}>
              {compactRole}
            </span>
          ) : null}
          {attentionLabel ? (
            <span className="message-attention-badge" title={attentionLabel}>
              {attentionLabel}
            </span>
          ) : null}
          <span className="message-time">{formatTime(message.timestamp)}</span>
          {usageTotal > 0 && (
            <span className="message-token-badge">{formatTokens(usageTotal)} tok</span>
          )}
        </div>

        {/* Text — human messages as plain text, agent messages as formatted markdown */}
        {isHuman ? (
          <div className="message-text message-text-plain">{message.content}</div>
        ) : (
          <div
            className="message-text"
            dangerouslySetInnerHTML={{ __html: renderedHtml }}
          />
        )}

        {/* Reactions */}
        {reactions.length > 0 && (
          <div className="message-reactions">
            {reactions.map((r) => (
              <button
                key={r.emoji}
                className="reaction-pill"
                onClick={() => {
                  toggleReaction(message.id, r.emoji, currentChannel).catch((e: Error) =>
                    showNotice(t('messages.bubble.reactionFailed', { error: e.message }), 'error'),
                  )
                }}
              >
                <span>{r.emoji}</span>
                <span className="reaction-pill-count">{r.count ?? 1}</span>
              </button>
            ))}
          </div>
        )}

        {(hasThreadLink || hasReplyAction) ? (
          <div className="message-action-row">
            {hasThreadLink && (
              <button
                className="inline-thread-btn inline-thread-toggle"
                onClick={() => onThreadClick?.(message.id)}
                onMouseEnter={() => void prefetchThreadMessages(currentChannel, threadId)}
                onFocus={() => void prefetchThreadMessages(currentChannel, threadId)}
                onPointerDown={() => void prefetchThreadMessages(currentChannel, threadId)}
                title={t('messages.bubble.reply', { count: message.thread_count })}
              >
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="m9 18 6-6-6-6" />
                </svg>
                {t('messages.bubble.reply', { count: message.thread_count })}
              </button>
            )}
            {hasReplyAction && (
              <button
                className="inline-thread-btn inline-thread-reply"
                onClick={() => onReply?.(message.id)}
                title={t('messages.bubble.replyAction')}
              >
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M18 15h-6l-1.5 4-6-4H2" />
                  <path d="M18 8h-6l-1.5 4-6-4H2" />
                </svg>
                {t('messages.bubble.replyAction')}
              </button>
            )}
          </div>
        ) : null}
      </div>
    </div>
  )
}
