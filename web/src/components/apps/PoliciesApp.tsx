import { useState, useRef, useCallback, useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  getPolicies,
  createPolicy,
  deletePolicy,
  type Policy,
} from '../../api/client'
import { showNotice } from '../ui/Toast'
import { confirm } from '../ui/ConfirmDialog'
import { usePersistentDraft } from '../../hooks/usePersistentDraft'
import { useBrokerRefetchInterval } from '../../hooks/useBrokerEvents'

function makeRequestId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `policy-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

const SECTIONS = [
  { key: 'human_directed', i18nKey: 'apps.policies.sections.humanDirected', icon: '\uD83D\uDC64' },
  { key: 'auto_detected', i18nKey: 'apps.policies.sections.autoDetected', icon: '\uD83E\uDD16' },
] as const

export function PoliciesApp() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const refetchInterval = useBrokerRefetchInterval(15_000)
  const [formOpen, setFormOpen] = useState(false)
  const { value: ruleText, setValue: setRuleText, clear: clearRuleText } = usePersistentDraft(
    'dunderia.policies.ruleDraft.v1',
    '',
  )
  const [saveRequestId, setSaveRequestId] = useState<string | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const deleteRequestIds = useRef<Record<string, string>>({})

  const { data, isLoading, error } = useQuery({
    queryKey: ['policies'],
    queryFn: () => getPolicies(),
    refetchInterval,
  })

  const invalidate = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ['policies'] })
  }, [queryClient])

  useEffect(() => {
    if (ruleText.trim()) {
      setFormOpen(true)
    }
  }, [ruleText])

  const handleSave = useCallback(() => {
    const trimmed = ruleText.trim()
    if (!trimmed) return
    const nextRequestId = saveRequestId ?? makeRequestId()
    setSaveRequestId(nextRequestId)
    createPolicy('human_directed', trimmed, nextRequestId)
      .then((response) => {
        if (!response.persisted) {
          throw new Error('persisted ack missing')
        }
        setSaveRequestId(null)
        clearRuleText()
        setFormOpen(false)
        invalidate()
      })
      .catch((e: Error) => showNotice(t('apps.policies.saveFailed', { error: e.message }), 'error'))
  }, [ruleText, invalidate, saveRequestId, t])

  const handleDelete = useCallback(
    (id: string, rule: string) => {
      confirm({
        title: t('apps.policies.deactivateTitle'),
        message: rule ? t('apps.policies.deactivateBody', { rule }) : t('apps.policies.deactivateBodyNoRule'),
        confirmLabel: t('apps.policies.deactivate'),
        danger: true,
        onConfirm: () =>
          deletePolicy(id, deleteRequestIds.current[id] || (deleteRequestIds.current[id] = makeRequestId()))
            .then((response) => {
              if (!response.persisted) {
                throw new Error('persisted ack missing')
              }
              delete deleteRequestIds.current[id]
              invalidate()
            })
            .catch((e: Error) => showNotice(t('apps.policies.deleteFailed', { error: e.message }), 'error')),
      })
    },
    [invalidate, t],
  )

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter') handleSave()
      if (e.key === 'Escape') {
        setFormOpen(false)
        setSaveRequestId(null)
      }
    },
    [handleSave],
  )

  const activePolicies = (data?.policies ?? []).filter((p) => p.active !== false)

  return (
    <>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '16px 20px 8px' }}>
        <div style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
          {t('apps.policies.subtitle')}
        </div>
        <button
          className="btn btn-secondary btn-sm"
          onClick={() => {
            setFormOpen((v) => !v)
            setTimeout(() => inputRef.current?.focus(), 50)
          }}
        >
          {t('apps.policies.addRule')}
        </button>
      </div>

      {/* Inline add form */}
      {formOpen && (
        <div style={{ padding: '8px 20px 12px', borderBottom: '1px solid var(--border)' }}>
          <input
            ref={inputRef}
            className="input"
            type="text"
            placeholder={t('apps.policies.rulePlaceholder')}
            value={ruleText}
            onChange={(e) => {
              setRuleText(e.target.value)
              setSaveRequestId(null)
            }}
            onKeyDown={handleKeyDown}
            style={{ marginBottom: 8 }}
          />
          <div style={{ display: 'flex', gap: 8 }}>
            <button className="btn btn-primary btn-sm" onClick={handleSave}>
              {t('apps.policies.save')}
            </button>
            <button
              className="btn btn-secondary btn-sm"
              onClick={() => {
                setFormOpen(false)
                setSaveRequestId(null)
              }}
            >
              {t('apps.policies.cancel')}
            </button>
          </div>
        </div>
      )}

      {/* Policy list */}
      {isLoading && (
        <div style={{ padding: 20, textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
          {t('apps.policies.loading')}
        </div>
      )}

      {error && (
        <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
          {t('apps.policies.loadFailed')}
        </div>
      )}

      {!isLoading && !error && activePolicies.length === 0 && (
        <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
          {t('apps.policies.empty')}
        </div>
      )}

      {!isLoading && !error && activePolicies.length > 0 && (
        <div style={{ padding: '8px 0' }}>
          {SECTIONS.map((section) => {
            const sectionPolicies = activePolicies.filter((p) => p.source === section.key)
            if (sectionPolicies.length === 0) return null
            return (
              <div key={section.key}>
                <div style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 6,
                  padding: '6px 20px',
                  fontSize: 11,
                  fontWeight: 600,
                  color: 'var(--text-tertiary)',
                  textTransform: 'uppercase',
                  letterSpacing: '0.08em',
                }}>
                  <span>{section.icon}</span>
                  <span>{t(section.i18nKey)}</span>
                </div>
                {sectionPolicies.map((policy) => (
                  <PolicyRow key={policy.id} policy={policy} onDelete={handleDelete} />
                ))}
              </div>
            )
          })}
        </div>
      )}
    </>
  )
}

interface PolicyRowProps {
  policy: Policy
  onDelete: (id: string, rule: string) => void
}

function PolicyRow({ policy, onDelete }: PolicyRowProps) {
  const { t } = useTranslation()
  return (
    <div className="app-card" style={{ margin: '0 12px 6px', display: 'flex', alignItems: 'center', gap: 8 }}>
      <span style={{ fontSize: 14, flexShrink: 0 }}>{'\uD83D\uDCCB'}</span>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ fontWeight: 500, fontSize: 13 }}>{policy.rule}</div>
        <div className="app-card-meta">
          <span className="badge badge-green" style={{ fontSize: 10 }}>{t('apps.policies.active')}</span>
        </div>
      </div>
      <button
        style={{
          background: 'none',
          border: 'none',
          cursor: 'pointer',
          color: 'var(--text-tertiary)',
          fontSize: 16,
          padding: '0 4px',
          lineHeight: 1,
          flexShrink: 0,
        }}
        title={t('apps.policies.deactivate')}
        onClick={() => onDelete(policy.id, policy.rule || '')}
      >
        {'\u00D7'}
      </button>
    </div>
  )
}
