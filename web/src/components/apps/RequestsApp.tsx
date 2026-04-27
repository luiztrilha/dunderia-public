import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { answerRequest, type AgentRequest } from '../../api/client'
import { useRequests } from '../../hooks/useRequests'
import { formatRelativeTime } from '../../lib/format'
import { canSubmitRequestOption, normalizeRequestCustomText, requestOptionTextKey } from '../../lib/requestAnswers'
import { showNotice } from '../ui/Toast'

export function RequestsApp() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { all: allRequests, pending, isLoading, error } = useRequests()
  const [busyRequestId, setBusyRequestId] = useState<string | null>(null)
  const [customTextByOption, setCustomTextByOption] = useState<Record<string, string>>({})
  const answered = allRequests.filter((r) => r.status && r.status !== 'open' && r.status !== 'pending')

  async function handleAnswer(req: AgentRequest, choiceId: string, customText?: string) {
    setBusyRequestId(req.id)
    try {
      await answerRequest(req.id, choiceId, customText)
      await queryClient.invalidateQueries({ queryKey: ['requests'] })
    } catch (e) {
      const message = e instanceof Error ? e.message : t('apps.requests.answerFailedUnknown')
      showNotice(t('apps.requests.answerFailed', { error: message }), 'error')
    } finally {
      setBusyRequestId(null)
    }
  }

  if (isLoading) {
    return (
      <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
        {t('apps.requests.loading')}
      </div>
    )
  }

  if (error) {
    return (
      <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
        {t('apps.requests.loadFailed')}
      </div>
    )
  }

  if (allRequests.length === 0) {
    return (
      <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
        {t('apps.requests.empty')}
      </div>
    )
  }

  return (
    <>
      {pending.length > 0 && (
        <>
          <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-secondary)', padding: '8px 0 4px' }}>
            {t('apps.requests.pending', { count: pending.length })}
          </div>
          {pending.map((req) => (
            <RequestItem
              key={req.id}
              request={req}
              isPending
              busy={busyRequestId === req.id}
              customTextByOption={customTextByOption}
              onChangeCustomText={(optionId, value) => {
                setCustomTextByOption((current) => ({ ...current, [requestOptionTextKey(req.id, optionId)]: value }))
              }}
              onAnswer={(choiceId, customText) => void handleAnswer(req, choiceId, customText)}
            />
          ))}
        </>
      )}

      {answered.length > 0 && (
        <>
          <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-secondary)', padding: '12px 0 4px' }}>
            {t('apps.requests.answered', { count: answered.length })}
          </div>
          {answered.map((req) => (
            <RequestItem key={req.id} request={req} isPending={false} />
          ))}
        </>
      )}
    </>
  )
}

interface RequestItemProps {
  request: AgentRequest
  isPending: boolean
  busy?: boolean
  customTextByOption?: Record<string, string>
  onChangeCustomText?: (choiceId: string, value: string) => void
  onAnswer?: (choiceId: string, customText?: string) => void
}

function RequestItem({
  request,
  isPending,
  busy = false,
  customTextByOption = {},
  onChangeCustomText,
  onAnswer,
}: RequestItemProps) {
  const { t } = useTranslation()
  // Broker uses `options`; legacy used `choices`. Accept either.
  const options = request.options ?? request.choices ?? []
  const ts = request.updated_at ?? request.created_at ?? request.timestamp

  return (
    <div className="app-card" style={{ marginBottom: 8 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
        <span style={{ fontWeight: 600, fontSize: 13 }}>{request.from || t('apps.requests.unknown')}</span>
        {request.status && (
          <span className="badge badge-accent" style={{ fontSize: 10 }}>
            {request.status.toUpperCase()}
          </span>
        )}
        {request.blocking && (
          <span className="badge badge-yellow" style={{ fontSize: 10 }}>{t('apps.requests.blocking')}</span>
        )}
      </div>

      {request.title && request.title !== 'Request' && (
        <div style={{ fontSize: 13, fontWeight: 600, marginBottom: 4 }}>{request.title}</div>
      )}

      <div style={{ fontSize: 14, marginBottom: 8 }}>{request.question || ''}</div>

      {request.context && (
        <div style={{ fontSize: 12, color: 'var(--text-secondary)', marginBottom: 8, whiteSpace: 'pre-wrap' }}>
          {request.context}
        </div>
      )}

      {ts && (
        <div className="app-card-meta" style={{ marginBottom: 6 }}>
          {formatRelativeTime(ts)}
        </div>
      )}

      {isPending && options.length > 0 && (
        <div style={{ display: 'grid', gap: 8 }}>
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            {options.map((opt) => {
              const textKey = requestOptionTextKey(request.id, opt.id)
              const customText = customTextByOption[textKey] ?? ''
              const disabled = !canSubmitRequestOption({
                requiresText: opt.requires_text,
                customText,
                busy,
              })
              return (
                <div key={opt.id} style={{ display: 'grid', gap: 6, minWidth: opt.requires_text ? 220 : undefined }}>
                  {opt.requires_text && (
                    <textarea
                      className="input"
                      value={customText}
                      disabled={busy}
                      rows={2}
                      placeholder={opt.text_hint || t('apps.requests.customTextPlaceholder')}
                      aria-label={t('apps.requests.customTextAria', { option: opt.label })}
                      onChange={(event) => onChangeCustomText?.(opt.id, event.target.value)}
                      style={{ resize: 'vertical', minHeight: 58 }}
                    />
                  )}
                  <button
                    className={`btn btn-sm ${opt.id === request.recommended_id ? 'btn-primary' : 'btn-ghost'}`}
                    title={opt.description}
                    disabled={disabled}
                    onClick={() => onAnswer?.(opt.id, opt.requires_text ? normalizeRequestCustomText(customText) : undefined)}
                  >
                    {busy ? t('apps.requests.answering') : opt.label}
                  </button>
                </div>
              )
            })}
          </div>
        </div>
      )}

      {!isPending && (
        <div style={{ fontSize: 12, color: 'var(--green)', fontWeight: 500 }}>{t('apps.requests.answeredLabel')}</div>
      )}
    </div>
  )
}
