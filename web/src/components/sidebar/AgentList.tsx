import { useTranslation } from 'react-i18next'
import { useAppStore } from '../../stores/app'
import { useAgentRuntimeSummary } from '../../hooks/useAgentRuntimeSummary'
import { PixelAvatar } from '../ui/PixelAvatar'
import { AgentWizard, useAgentWizard } from '../agents/AgentWizard'
import { createDM, extractDMChannelSlug, formatProviderLabel } from '../../api/client'
import type { AgentRuntimeMember } from '../../lib/agentRuntime'
import { showNotice } from '../ui/Toast'

function classifyActivity(member: AgentRuntimeMember | undefined): { state: 'working' | 'blocked' | 'waiting' | 'silent'; dotClass: string } {
  switch (member?.runtimeState) {
    case 'working':
      return { state: 'working', dotClass: 'runtime-working pulse' }
    case 'blocked':
      return { state: 'blocked', dotClass: 'runtime-blocked' }
    case 'waiting':
      return { state: 'waiting', dotClass: 'runtime-waiting' }
    default:
      return { state: 'silent', dotClass: 'runtime-silent' }
  }
}

export function AgentList() {
  const { t } = useTranslation()
  const primaryRail = useAppStore((s) => s.primaryRail)
  const currentChannel = useAppStore((s) => s.currentChannel)
  const enterDM = useAppStore((s) => s.enterDM)
  const setActiveAgentSlug = useAppStore((s) => s.setActiveAgentSlug)
  const channelMeta = useAppStore((s) => s.channelMeta)
  const wizard = useAgentWizard()
  const runtime = useAgentRuntimeSummary(currentChannel)

  const agents = runtime.members.filter((m) => m.slug && m.slug !== 'human')

  return (
    <>
      <div className="sidebar-agents">
        {agents.length === 0 ? (
          <div style={{ fontSize: 11, color: 'var(--text-tertiary)', padding: '4px 8px' }}>
            {t('sidebar.agents.empty')}
          </div>
        ) : (
          agents.map((agent) => {
            const ac = classifyActivity(agent)
            const meta = channelMeta[currentChannel]
            const isDMActive = meta?.type === 'D' && meta.agentSlug === agent.slug
            const runtimeLabel = formatProviderLabel(agent.provider?.kind)
            const stateLabel = t(`sidebar.runtime.${ac.state}`)

            return (
              <button
                key={agent.slug}
                data-agent-slug={agent.slug}
                className={`sidebar-agent${isDMActive ? ' active' : ''}`}
                title={t('sidebar.agents.titleTemplate', { name: agent.name, runtime: runtimeLabel, state: stateLabel })}
                onClick={async () => {
                  if (primaryRail === 'dms') {
                    try {
                      const result = await createDM(agent.slug)
                      enterDM(agent.slug, extractDMChannelSlug(result, agent.slug))
                    } catch (err) {
                      const message = err instanceof Error ? err.message : t('messages.commands.agentNotFound', { slug: agent.slug })
                      showNotice(message, 'error')
                    }
                    return
                  }
                  setActiveAgentSlug(agent.slug)
                }}
              >
                <span className="sidebar-agent-avatar">
                  <PixelAvatar
                    slug={agent.slug}
                    size={24}
                    className="pixel-avatar-sidebar"
                  />
                </span>
                <div className="sidebar-agent-wrap">
                  <div className="sidebar-agent-headline">
                    <span className="sidebar-agent-name">{agent.name || agent.slug}</span>
                    <span
                      className={`sidebar-agent-state sidebar-agent-state-${ac.state}`}
                      data-testid={`sidebar-agent-state-${agent.slug}`}
                    >
                      {stateLabel}
                    </span>
                  </div>
                  {agent.runtimeDetail && (
                    <span className="sidebar-agent-task">{agent.runtimeDetail}</span>
                  )}
                  <span
                    style={{
                      fontSize: 10,
                      color: 'var(--text-tertiary)',
                      lineHeight: 1.2,
                    }}
                  >
                    {runtimeLabel}
                  </span>
                </div>
                <span className={`status-dot ${ac.dotClass}`} />
              </button>
            )
          })
        )}
        <button
          className="sidebar-item sidebar-add-btn"
          onClick={wizard.show}
          title={t('sidebar.agents.newAgentTitle')}
        >
          <span style={{ fontSize: 14, width: 18, textAlign: 'center', flexShrink: 0 }}>+</span>
          <span>{t('sidebar.agents.newAgent')}</span>
        </button>
      </div>
      <AgentWizard open={wizard.open} onClose={wizard.hide} />
    </>
  )
}
