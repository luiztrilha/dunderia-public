import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useMessages } from '../../hooks/useMessages'
import { useAgentStream } from '../../hooks/useAgentStream'
import { useOfficeMembers } from '../../hooks/useMembers'
import { useAppStore } from '../../stores/app'
import { MessageBubble } from './MessageBubble'
import { Composer } from './Composer'
import { InterviewBar } from './InterviewBar'
import { StreamLineView } from './StreamLineView'

export function DMView() {
  const { t } = useTranslation()
  const currentChannel = useAppStore((s) => s.currentChannel)
  const dmAgentSlug = useAppStore((s) => s.dmAgentSlug)
  const exitDM = useAppStore((s) => s.exitDM)
  const { data: messages = [], isLoading, isRefreshing, error } = useMessages(currentChannel)
  const { data: officeMembers = [] } = useOfficeMembers()
  const { lines, connected } = useAgentStream(dmAgentSlug, currentChannel)
  const messagesRef = useRef<HTMLDivElement>(null)
  const streamRef = useRef<HTMLDivElement>(null)

  // Auto-scroll messages
  useEffect(() => {
    if (messagesRef.current) {
      messagesRef.current.scrollTop = messagesRef.current.scrollHeight
    }
  }, [messages.length])

  // Auto-scroll stream
  useEffect(() => {
    if (streamRef.current) {
      streamRef.current.scrollTop = streamRef.current.scrollHeight
    }
  }, [lines.length])

  return (
    <>
      {/* DM banner */}
      <div className="dm-banner active">
        <span>{t('messages.dm.banner', { agent: dmAgentSlug })}</span>
        <button className="dm-back-btn" onClick={exitDM}>
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M19 12H5" />
            <path d="m12 19-7-7 7-7" />
          </svg>
          {t('messages.dm.back')}
        </button>
      </div>

      {/* Split layout: messages left, live stream right */}
      <div style={{ flex: 1, display: 'flex', overflow: 'hidden' }}>
        {/* Left: Messages + Composer */}
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
          <div
            ref={messagesRef}
            className="messages"
          >
            {isRefreshing ? (
              <div className="messages-refresh-indicator" role="status" aria-live="polite">
                {t('messages.feed.refreshing')}
              </div>
            ) : null}
            {isLoading && messages.length === 0 ? (
              <div style={{ color: 'var(--text-tertiary)', padding: 12, fontSize: 13 }}>
                {t('messages.feed.loading')}
              </div>
            ) : error && messages.length === 0 ? (
              <div style={{ color: 'var(--red)', padding: 12, fontSize: 13 }}>
                {t('messages.feed.loadFailed', { error })}
              </div>
            ) : (
              <>
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
                {messages.map((msg) => (
                  <MessageBubble key={msg.id} message={msg} members={officeMembers} />
                ))}
              </>
            )}
          </div>
          <InterviewBar />
          <Composer />
        </div>

        {/* Right: Live stream */}
        <div style={{
          width: 320,
          flexShrink: 0,
          borderLeft: '1px solid var(--border)',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}>
          <div style={{
            padding: '8px 12px',
            borderBottom: '1px solid var(--border)',
            display: 'flex',
            alignItems: 'center',
            gap: 8,
            fontSize: 13,
            fontWeight: 600,
          }}>
            <span
              className={`status-dot ${connected ? 'active pulse' : 'lurking'}`}
            />
            <span>{t('messages.dm.liveOutput')}</span>
          </div>
          <div
            ref={streamRef}
            style={{
              flex: 1,
              overflowY: 'auto',
              padding: 8,
              fontFamily: 'var(--font-mono)',
              fontSize: 11,
              lineHeight: 1.5,
              color: 'var(--text-secondary)',
            }}
          >
            {lines.length === 0 ? (
              <div style={{ color: 'var(--text-tertiary)', padding: 8 }}>
                {connected ? t('messages.dm.waiting') : t('messages.dm.idle')}
              </div>
            ) : (
              lines.map((line) => (
                <StreamLineView key={line.id} line={line} compact />
              ))
            )}
          </div>
        </div>
      </div>
    </>
  )
}
