import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import { answerRequest, type AgentRequest } from '../../api/client'
import { useRequests } from '../../hooks/useRequests'
import { showNotice } from '../ui/Toast'

/**
 * Global blocking-interview overlay. Always renders the first blocking pending
 * request from the broker, regardless of which app/channel the user is viewing.
 * Non-blocking requests get a one-time toast and stay in the Requests panel.
 */
export function HumanInterviewOverlay() {
  const { t } = useTranslation()
  const { blockingPending, pending } = useRequests()
  const queryClient = useQueryClient()
  const [submitting, setSubmitting] = useState(false)
  const seenNonBlockingIds = useRef<Set<string>>(new Set())

  // Toast non-blocking requests once each
  useEffect(() => {
    for (const req of pending) {
      if (req.blocking) continue
      if (!req.id || seenNonBlockingIds.current.has(req.id)) continue
      seenNonBlockingIds.current.add(req.id)
      showNotice(t('messages.overlay.toastAsked', { from: req.from || 'someone', question: req.question }), 'info')
    }
  }, [pending, t])

  if (!blockingPending) return null

  return (
    <BlockingInterview
      request={blockingPending}
      submitting={submitting}
      onAnswer={async (choiceId) => {
        if (submitting) return
        setSubmitting(true)
        try {
          await answerRequest(blockingPending.id, choiceId)
          await queryClient.invalidateQueries({ queryKey: ['requests'] })
        } catch (err: unknown) {
          const message = err instanceof Error ? err.message : t('messages.interview.failedAnswer')
          showNotice(message, 'error')
        } finally {
          setSubmitting(false)
        }
      }}
    />
  )
}

interface BlockingInterviewProps {
  request: AgentRequest
  submitting: boolean
  onAnswer: (choiceId: string) => void
}

function BlockingInterview({ request, submitting, onAnswer }: BlockingInterviewProps) {
  const { t } = useTranslation()
  const options = request.options ?? request.choices ?? []

  return (
    <div
      className={`interview-overlay${request.blocking ? ' interview-overlay-blocking' : ' interview-overlay-waiting'}`}
      role="dialog"
      aria-modal="true"
      aria-labelledby="interview-title"
    >
      <div className={`interview-card${request.blocking ? ' interview-card-blocking' : ' interview-card-waiting'}`}>
        <div className="interview-meta">
          <span className={request.blocking ? 'badge badge-attention' : 'badge badge-waiting'}>
            {request.blocking ? t('messages.interview.blocking') : t('messages.thread.humanAttentionBadge')}
          </span>
          <span className="interview-from">{t('messages.overlay.fromBadge', { from: request.from || 'agent' })}</span>
          {request.channel && <span className="interview-channel">{t('messages.interview.inChannel', { channel: request.channel })}</span>}
        </div>
        <h2 id="interview-title" className="interview-title">
          {request.title && request.title !== 'Request' ? request.title : t('messages.overlay.fallbackTitle')}
        </h2>
        <p className="interview-question">{request.question}</p>
        {request.context && (
          <p className="interview-context">{request.context}</p>
        )}
        {options.length > 0 ? (
          <div className="interview-actions">
            {options.map((opt) => (
              <button
                key={opt.id}
                type="button"
                className={`btn btn-sm ${opt.id === request.recommended_id ? 'btn-primary' : 'btn-ghost'}`}
                onClick={() => onAnswer(opt.id)}
                disabled={submitting}
                title={opt.description}
              >
                {opt.label}
              </button>
            ))}
          </div>
        ) : (
          <div className="interview-empty">
            {t('messages.overlay.noChoices')}
          </div>
        )}
      </div>
    </div>
  )
}
