import { useState, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getSkills, invokeSkill, type Skill } from '../../api/client'
import { showNotice } from '../ui/Toast'
import { useBrokerRefetchInterval } from '../../hooks/useBrokerEvents'

function makeRequestId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `skill-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

export function SkillsApp() {
  const { t } = useTranslation()
  const refetchInterval = useBrokerRefetchInterval(30_000)
  const { data, isLoading, error } = useQuery({
    queryKey: ['skills'],
    queryFn: () => getSkills(),
    refetchInterval,
  })

  if (isLoading) {
    return (
      <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
        {t('apps.skills.loading')}
      </div>
    )
  }

  if (error) {
    return (
      <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
        {t('apps.skills.loadFailed')}
      </div>
    )
  }

  const skills = data?.skills ?? []

  return (
    <>
      <div style={{ padding: '0 0 12px', borderBottom: '1px solid var(--border)', marginBottom: 12 }}>
        <h3 style={{ fontSize: 16, fontWeight: 600 }}>{t('apps.skills.title')}</h3>
      </div>

      {skills.length === 0 ? (
        <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
          {t('apps.skills.empty')}
        </div>
      ) : (
        skills.map((skill) => <SkillCard key={skill.name} skill={skill} />)
      )}
    </>
  )
}

function SkillCard({ skill }: { skill: Skill }) {
  const { t } = useTranslation()
  const [invokeState, setInvokeState] = useState<'idle' | 'invoking' | 'done'>('idle')
  const [requestId, setRequestId] = useState<string | null>(null)

  const handleInvoke = useCallback(() => {
    if (!skill.name) return
    const nextRequestId = requestId ?? makeRequestId()
    setRequestId(nextRequestId)
    setInvokeState('invoking')
    invokeSkill(skill.name, {}, nextRequestId)
      .then((response) => {
        if (!response.persisted) {
          throw new Error('persisted ack missing')
        }
        setInvokeState('done')
        setRequestId(null)
        setTimeout(() => setInvokeState('idle'), 1500)
      })
      .catch((e: Error) => {
        setInvokeState('idle')
        showNotice(t('apps.skills.invokeFailed', { error: e.message }), 'error')
      })
  }, [requestId, skill.name, t])

  const buttonLabel =
    invokeState === 'invoking' ? t('apps.skills.invoking') :
    invokeState === 'done' ? '\u2713 ' + t('apps.skills.invoked') :
    '\u26A1 ' + t('apps.skills.invoke')

  return (
    <div className="app-card" style={{ marginBottom: 8 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
        <span style={{ fontSize: 16 }}>{'\u26A1'}</span>
        <span className="app-card-title" style={{ marginBottom: 0 }}>
          {skill.name || t('apps.skills.untitled')}
        </span>
      </div>

      {skill.description && (
        <div style={{ fontSize: 13, color: 'var(--text-secondary)', marginBottom: 8, lineHeight: 1.45 }}>
          {skill.description}
        </div>
      )}

      {skill.source && (
        <div className="app-card-meta" style={{ marginBottom: 8 }}>
          {t('apps.skills.source', { source: skill.source })}
        </div>
      )}

      <div style={{ display: 'flex', gap: 8, marginTop: 10 }}>
        <button
          className="btn btn-primary btn-sm"
          disabled={invokeState !== 'idle'}
          onClick={handleInvoke}
        >
          {buttonLabel}
        </button>
      </div>
    </div>
  )
}
