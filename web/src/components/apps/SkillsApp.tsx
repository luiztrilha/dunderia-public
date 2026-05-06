import { useState, useCallback } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  decideOrgProposal,
  getAdapters,
  getOrgProposals,
  getSkills,
  invokeSkill,
  type OfficeAdapter,
  type OrgProposal,
  type Skill,
} from '../../api/client'
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
  const adaptersQuery = useQuery({
    queryKey: ['adapters'],
    queryFn: () => getAdapters(),
    refetchInterval,
  })
  const proposalsQuery = useQuery({
    queryKey: ['org-proposals'],
    queryFn: () => getOrgProposals(),
    refetchInterval,
  })

  if (isLoading || adaptersQuery.isLoading || proposalsQuery.isLoading) {
    return (
      <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
        {t('apps.skills.loading')}
      </div>
    )
  }

  if (error || adaptersQuery.error || proposalsQuery.error) {
    return (
      <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
        {t('apps.skills.loadFailed')}
      </div>
    )
  }

  const skills = data?.skills ?? []
  const adapters = adaptersQuery.data?.adapters ?? []
  const proposals = proposalsQuery.data?.proposals ?? []

  return (
    <>
      <div style={{ padding: '0 0 12px', borderBottom: '1px solid var(--border)', marginBottom: 12 }}>
        <h3 style={{ fontSize: 16, fontWeight: 600 }}>{t('apps.skills.title')}</h3>
        <div className="app-card-meta" style={{ marginTop: 4 }}>
          {skills.length} skills · {adapters.length} adapters · {proposals.filter((p) => p.status === 'proposed').length} propostas abertas
        </div>
      </div>

      <SectionTitle title="Adapters" meta="Capacidades registradas para agentes e rotinas." />
      {adapters.length === 0 ? (
        <EmptyLine text="Nenhum adapter registrado." />
      ) : (
        adapters.map((adapter) => <AdapterCard key={adapter.id} adapter={adapter} />)
      )}

      <SectionTitle title="Auto-organizacao" meta="Propostas aprovaveis; aprovar nao altera a topologia automaticamente." />
      {proposals.length === 0 ? (
        <EmptyLine text="Nenhuma proposta aberta." />
      ) : (
        proposals.map((proposal) => <OrgProposalCard key={proposal.id} proposal={proposal} />)
      )}

      <SectionTitle title="Skills" meta="Procedimentos reutilizaveis e invocaveis." />
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

function SectionTitle({ title, meta }: { title: string; meta: string }) {
  return (
    <div style={{ margin: '16px 0 8px' }}>
      <div style={{ fontSize: 13, fontWeight: 700 }}>{title}</div>
      <div className="app-card-meta">{meta}</div>
    </div>
  )
}

function EmptyLine({ text }: { text: string }) {
  return (
    <div className="app-card-meta" style={{ padding: '8px 2px 12px' }}>
      {text}
    </div>
  )
}

function AdapterCard({ adapter }: { adapter: OfficeAdapter }) {
  const health = adapter.health_status || 'unknown'
  return (
    <div className="app-card" style={{ marginBottom: 8 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8, marginBottom: 6 }}>
        <div className="app-card-title" style={{ marginBottom: 0 }}>{adapter.name || adapter.id}</div>
        <span className={`badge ${health === 'error' ? 'badge-attention' : health === 'warning' || health === 'unknown' ? 'badge-yellow' : 'badge-green'}`}>
          {health}
        </span>
      </div>
      {adapter.description && (
        <div style={{ fontSize: 13, color: 'var(--text-secondary)', marginBottom: 8, lineHeight: 1.45 }}>
          {adapter.description}
        </div>
      )}
      <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
        {adapter.kind && <span className="badge badge-accent">{adapter.kind}</span>}
        {adapter.provider && <span className="app-card-meta">{adapter.provider}</span>}
        {adapter.status && <span className="app-card-meta">{adapter.status}</span>}
        {(adapter.capabilities ?? []).slice(0, 6).map((capability) => (
          <span key={capability} className="app-card-meta">{capability}</span>
        ))}
      </div>
      {adapter.health_summary && (
        <div className="app-card-meta" style={{ marginTop: 8, lineHeight: 1.45 }}>
          {adapter.health_summary}
        </div>
      )}
    </div>
  )
}

function OrgProposalCard({ proposal }: { proposal: OrgProposal }) {
  const queryClient = useQueryClient()
  const [busyAction, setBusyAction] = useState<'approve' | 'reject' | null>(null)
  const mutation = useMutation({
    mutationFn: (action: 'approve' | 'reject') => decideOrgProposal(proposal.id, action),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['org-proposals'] })
    },
    onError: (e: Error) => {
      showNotice(e.message, 'error')
    },
    onSettled: () => setBusyAction(null),
  })
  const act = (action: 'approve' | 'reject') => {
    setBusyAction(action)
    mutation.mutate(action)
  }
  const status = proposal.status || 'proposed'
  return (
    <div className={`app-card ${proposal.requires_topology_authorization ? 'app-card-waiting' : ''}`} style={{ marginBottom: 8 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8, marginBottom: 6 }}>
        <div className="app-card-title" style={{ marginBottom: 0 }}>{proposal.title}</div>
        <span className={`badge ${status === 'approved' ? 'badge-green' : status === 'rejected' ? 'badge-attention' : 'badge-yellow'}`}>
          {status}
        </span>
      </div>
      {proposal.summary && (
        <div style={{ fontSize: 13, color: 'var(--text-secondary)', marginBottom: 8, lineHeight: 1.45 }}>
          {proposal.summary}
        </div>
      )}
      <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginBottom: status === 'proposed' ? 10 : 0 }}>
        {proposal.kind && <span className="badge badge-accent">{proposal.kind}</span>}
        {proposal.channel && <span className="app-card-meta">#{proposal.channel}</span>}
        {proposal.proposed_by && <span className="app-card-meta">@{proposal.proposed_by}</span>}
        {proposal.requires_topology_authorization && <span className="badge badge-yellow">topologia protegida</span>}
      </div>
      {proposal.proposed_change && (
        <div className="app-card-meta" style={{ marginBottom: 10, lineHeight: 1.45 }}>
          {proposal.proposed_change}
        </div>
      )}
      {status === 'proposed' && (
        <div style={{ display: 'flex', gap: 8 }}>
          <button type="button" className="btn btn-primary btn-sm" disabled={mutation.isPending} onClick={() => act('approve')}>
            {busyAction === 'approve' ? 'Aprovando...' : 'Aprovar'}
          </button>
          <button type="button" className="btn btn-ghost btn-sm" disabled={mutation.isPending} onClick={() => act('reject')}>
            {busyAction === 'reject' ? 'Rejeitando...' : 'Rejeitar'}
          </button>
        </div>
      )}
    </div>
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
      {(skill.plugin_id || skill.plugin_kind || skill.health_status) && (
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginBottom: 8 }}>
          {skill.plugin_id && <span className="badge badge-accent">{skill.plugin_id}</span>}
          {skill.plugin_kind && <span className="app-card-meta">{skill.plugin_kind}</span>}
          {skill.health_status && (
            <span className={`badge ${skill.health_status === 'error' ? 'badge-attention' : skill.health_status === 'warning' ? 'badge-yellow' : 'badge-green'}`}>
              {skill.health_status}
            </span>
          )}
        </div>
      )}
      {(skill.capabilities ?? []).length > 0 && (
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginBottom: 8 }}>
          {(skill.capabilities ?? []).slice(0, 5).map((capability) => (
            <span key={capability} className="app-card-meta">{capability}</span>
          ))}
        </div>
      )}
      {skill.health_summary && (
        <div style={{ fontSize: 12, color: 'var(--text-secondary)', marginBottom: 8, lineHeight: 1.45 }}>
          {skill.health_summary}
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
