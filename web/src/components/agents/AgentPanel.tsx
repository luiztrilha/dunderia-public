import { useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useAppStore } from '../../stores/app'
import { useOfficeMembers } from '../../hooks/useMembers'
import { useAgentStream } from '../../hooks/useAgentStream'
import { createDM, extractDMChannelSlug, formatProviderLabel, getAgentLogs, post, type PerAgentProviderKind } from '../../api/client'
import { PixelAvatar } from '../ui/PixelAvatar'
import { showNotice } from '../ui/Toast'
import { StreamLineView } from '../messages/StreamLineView'
import type { AgentLog, OfficeMember } from '../../api/client'

interface AgentPanelViewProps {
  agent: OfficeMember
  onClose: () => void
}

type RuntimeChoice = 'office-default' | PerAgentProviderKind

interface RuntimeOption {
  value: RuntimeChoice
  label: string
  i18nKey?: string
}

const RUNTIME_OPTIONS: RuntimeOption[] = [
  { value: 'office-default', label: '', i18nKey: 'apps.agentPanel.useOfficeDefault' },
  { value: 'claude-code', label: formatProviderLabel('claude-code') },
  { value: 'codex', label: formatProviderLabel('codex') },
  { value: 'gemini', label: formatProviderLabel('gemini') },
  { value: 'gemini-vertex', label: formatProviderLabel('gemini-vertex') },
  { value: 'ollama', label: formatProviderLabel('ollama') },
  { value: 'openclaude', label: formatProviderLabel('openclaude') },
]

function runtimeChoiceFromAgent(agent: OfficeMember): RuntimeChoice {
  switch (agent.provider?.kind) {
    case 'claude-code':
    case 'codex':
    case 'gemini':
    case 'gemini-vertex':
    case 'ollama':
    case 'openclaude':
      return agent.provider.kind
    default:
      return 'office-default'
  }
}

function runtimeModelFromAgent(agent: OfficeMember): string {
  return agent.provider?.model?.trim() ?? ''
}

function formatRuntimeSummary(agent: OfficeMember): string {
  const label = formatProviderLabel(agent.provider?.kind)
  const model = runtimeModelFromAgent(agent)
  if (!model) {
    return label
  }
  return `${label} · ${model}`
}

function StreamSection({ slug }: { slug: string }) {
  const { t } = useTranslation()
  const currentChannel = useAppStore((s) => s.currentChannel)
  const { lines, connected } = useAgentStream(slug, currentChannel)
  const scrollRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const el = scrollRef.current
    if (el) {
      el.scrollTop = el.scrollHeight
    }
  }, [lines])

  return (
    <div className="agent-panel-section">
      <div className="agent-panel-section-title">{t('apps.agentPanel.liveStream')}</div>
      <div className="agent-stream-status">
        <span className={`status-dot ${connected ? 'active pulse' : 'lurking'}`} />
        {connected ? t('apps.agentPanel.connected') : t('apps.agentPanel.disconnected')}
      </div>
      <div className="agent-stream-log" ref={scrollRef}>
        {lines.length === 0 ? (
          <div className="agent-stream-empty">{t('apps.agentPanel.noOutput')}</div>
        ) : (
          lines.map((line) => (
            <StreamLineView key={line.id} line={line} compact />
          ))
        )}
      </div>
    </div>
  )
}

function LogsSection({ slug }: { slug: string }) {
  const { t } = useTranslation()
  const [logs, setLogs] = useState<AgentLog[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    setLoading(true)

    getAgentLogs({ limit: 10 })
      .then((data) => {
        if (!cancelled) {
          const agentLogs = data.logs.filter((l) => l.agent === slug)
          setLogs(agentLogs.slice(0, 10))
          setLoading(false)
        }
      })
      .catch(() => {
        if (!cancelled) setLoading(false)
      })

    return () => { cancelled = true }
  }, [slug])

  function formatTime(timestamp: string | undefined): string {
    if (!timestamp) return ''
    try {
      const d = new Date(timestamp)
      return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
    } catch {
      return ''
    }
  }

  return (
    <div className="agent-panel-logs">
      <div className="agent-panel-section">
        <div className="agent-panel-section-title">{t('apps.agentPanel.recentActivity')}</div>
      </div>
      {loading ? (
        <div className="agent-log-empty">{t('apps.agentPanel.loading')}</div>
      ) : logs.length === 0 ? (
        <div className="agent-log-empty">{t('apps.agentPanel.noRecentActivity')}</div>
      ) : (
        logs.map((log) => (
          <div key={log.id} className="agent-log-item">
            {log.action && <div className="agent-log-action">{log.action}</div>}
            {log.content && <div className="agent-log-content">{log.content}</div>}
            <div className="agent-log-time">{formatTime(log.timestamp)}</div>
          </div>
        ))
      )}
    </div>
  )
}

function AgentPanelView({ agent, onClose }: AgentPanelViewProps) {
  const { t } = useTranslation()
  const enterDM = useAppStore((s) => s.enterDM)
  const setActiveAgentSlug = useAppStore((s) => s.setActiveAgentSlug)
  const queryClient = useQueryClient()
  const savedRuntimeChoice = runtimeChoiceFromAgent(agent)
  const savedRuntimeModel = runtimeModelFromAgent(agent)
  const [dmLoading, setDmLoading] = useState(false)
  const [runtimeSaving, setRuntimeSaving] = useState(false)
  const [runtimeChoice, setRuntimeChoice] = useState<RuntimeChoice>(savedRuntimeChoice)
  const [runtimeModel, setRuntimeModel] = useState(savedRuntimeModel)
  const [view, setView] = useState<'stream' | 'logs'>('stream')

  useEffect(() => {
    setRuntimeChoice(savedRuntimeChoice)
    setRuntimeModel(savedRuntimeModel)
  }, [savedRuntimeChoice, savedRuntimeModel, agent.slug])

  async function handleOpenDM() {
    setDmLoading(true)
    try {
      const result = await createDM(agent.slug)
      const channel = extractDMChannelSlug(result, agent.slug)
      enterDM(agent.slug, channel)
      setActiveAgentSlug(null)
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : t('apps.agentPanel.openDMFailed')
      showNotice(message, 'error')
    } finally {
      setDmLoading(false)
    }
  }

  async function handleSaveRuntime() {
    setRuntimeSaving(true)
    try {
      await post('/office-members', {
        action: 'update',
        slug: agent.slug,
        provider: runtimeChoice === 'office-default'
          ? { kind: '', model: '' }
          : { kind: runtimeChoice, model: runtimeModel.trim() },
      })
      await queryClient.invalidateQueries({ queryKey: ['office-members'] })
      showNotice(t('apps.agentPanel.runtimeUpdated', { name: agent.name || agent.slug }), 'success')
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : t('apps.agentPanel.runtimeUpdateFailed')
      showNotice(message, 'error')
    } finally {
      setRuntimeSaving(false)
    }
  }

  const statusClass = agent.status === 'active' ? 'active pulse' : 'lurking'
  const runtimeDirty = runtimeChoice !== savedRuntimeChoice || runtimeModel.trim() !== savedRuntimeModel

  return (
    <div className="agent-panel">
      <div className="agent-panel-header">
        <div className="agent-panel-identity">
          <div className="agent-panel-avatar">
            <PixelAvatar
              slug={agent.slug}
              size={56}
              className="pixel-avatar-panel"
            />
          </div>
          <div style={{ minWidth: 0, flex: 1 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <span className="agent-panel-name">{agent.name || agent.slug}</span>
              <span className={`status-dot ${statusClass}`} />
            </div>
            {agent.role && (
              <span className="badge badge-accent" style={{ marginTop: 2 }}>
                {agent.role}
              </span>
            )}
          </div>
        </div>
        <button
          className="agent-panel-close"
          onClick={onClose}
          aria-label={t('apps.agentPanel.close')}
        >
          <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
            <path d="M1 1l12 12M13 1L1 13" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
          </svg>
        </button>
      </div>

      <div className="agent-panel-section">
        <div className="agent-panel-info">
          <div className="agent-panel-info-row">
            <span className="agent-panel-info-label">{t('apps.agentPanel.slug')}</span>
            <span className="agent-panel-info-value">{agent.slug}</span>
          </div>
          <div className="agent-panel-info-row">
            <span className="agent-panel-info-label">{t('apps.agentPanel.runtime')}</span>
            <span className="agent-panel-info-value">{formatRuntimeSummary(agent)}</span>
          </div>
          <div className="agent-panel-info-row" style={{ alignItems: 'flex-start' }}>
            <span className="agent-panel-info-label">{t('apps.agentPanel.setRuntime')}</span>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8, minWidth: 0, flex: 1 }}>
              <select
                className="input"
                value={runtimeChoice}
                onChange={(e) => setRuntimeChoice(e.target.value as RuntimeChoice)}
                disabled={runtimeSaving}
              >
                {RUNTIME_OPTIONS.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.i18nKey ? t(option.i18nKey) : option.label}
                  </option>
                ))}
              </select>
              <input
                className="input"
                type="text"
                value={runtimeModel}
                onChange={(e) => setRuntimeModel(e.target.value)}
                placeholder={t('apps.agentPanel.modelPlaceholder')}
                disabled={runtimeSaving || runtimeChoice === 'office-default'}
              />
              <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
                <button
                  className="btn btn-ghost btn-sm"
                  onClick={handleSaveRuntime}
                  disabled={runtimeSaving || !runtimeDirty}
                >
                  {runtimeSaving ? t('apps.agentPanel.saving') : t('apps.agentPanel.save')}
                </button>
                <span style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>
                  {t('apps.agentPanel.runtimeHint')}
                </span>
              </div>
              <span style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>
                {t('apps.agentPanel.modelHint')}
              </span>
            </div>
          </div>
          {agent.status && (
            <div className="agent-panel-info-row">
              <span className="agent-panel-info-label">{t('apps.agentPanel.status')}</span>
              <span className="agent-panel-info-value">{agent.status}</span>
            </div>
          )}
          {agent.task && (
            <div className="agent-panel-info-row">
              <span className="agent-panel-info-label">{t('apps.agentPanel.task')}</span>
              <span className="agent-panel-info-value">{agent.task}</span>
            </div>
          )}
        </div>
      </div>

      <div className="agent-panel-actions">
        <button
          className="btn btn-primary btn-sm"
          onClick={handleOpenDM}
          disabled={dmLoading}
        >
          {dmLoading ? t('apps.agentPanel.opening') : t('apps.agentPanel.openDM')}
        </button>
        <button
          className="btn btn-ghost btn-sm"
          onClick={() => setView(view === 'logs' ? 'stream' : 'logs')}
        >
          {view === 'logs' ? t('apps.agentPanel.liveStream') : t('apps.agentPanel.viewLogs')}
        </button>
      </div>

      {view === 'stream' ? (
        <StreamSection slug={agent.slug} />
      ) : (
        <LogsSection slug={agent.slug} />
      )}
    </div>
  )
}

export function AgentPanel() {
  const activeAgentSlug = useAppStore((s) => s.activeAgentSlug)
  const setActiveAgentSlug = useAppStore((s) => s.setActiveAgentSlug)
  const { data: members = [] } = useOfficeMembers()

  if (!activeAgentSlug) return null

  const agent = members.find((m) => m.slug === activeAgentSlug)
  if (!agent) return null

  return (
    <AgentPanelView
      agent={agent}
      onClose={() => setActiveAgentSlug(null)}
    />
  )
}
