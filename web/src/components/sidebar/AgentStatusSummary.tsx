import { useTranslation } from 'react-i18next'
import { useAppStore } from '../../stores/app'
import { useAgentRuntimeSummary } from '../../hooks/useAgentRuntimeSummary'

function namesForList(values: string[]): string {
  if (values.length === 0) return '—'
  return values.slice(0, 3).map((value) => `@${value}`).join(' · ')
}

export function AgentStatusSummary() {
  const { t } = useTranslation()
  const currentChannel = useAppStore((s) => s.currentChannel)
  const runtime = useAgentRuntimeSummary(currentChannel)

  const groups = [
    {
      key: 'working',
      label: t('sidebar.runtime.working'),
      count: runtime.counts.working,
      members: runtime.members.filter((member) => member.runtimeState === 'working').map((member) => member.slug),
    },
    {
      key: 'blocked',
      label: t('sidebar.runtime.blocked'),
      count: runtime.counts.blocked,
      members: runtime.members.filter((member) => member.runtimeState === 'blocked').map((member) => member.slug),
    },
    {
      key: 'waiting',
      label: t('sidebar.runtime.waiting'),
      count: runtime.counts.waiting,
      members: runtime.members.filter((member) => member.runtimeState === 'waiting').map((member) => member.slug),
    },
    {
      key: 'silent',
      label: t('sidebar.runtime.silent'),
      count: runtime.counts.silent,
      members: runtime.members.filter((member) => member.runtimeState === 'silent').map((member) => member.slug),
    },
  ]

  return (
    <div className="agent-status-summary" data-testid="agent-status-summary">
      <div className="agent-status-summary-title">{t('sidebar.runtime.title')}</div>
      <div className="agent-status-summary-list">
        {groups.map((group) => (
          <div key={group.key} className="agent-status-summary-row">
            <div className="agent-status-summary-row-head">
              <span className="agent-status-summary-label">{group.label}</span>
              <span
                className="agent-status-summary-count"
                data-testid={`agent-status-count-${group.key}`}
              >
                {group.count}
              </span>
            </div>
            <div className="agent-status-summary-members">{namesForList(group.members)}</div>
          </div>
        ))}
      </div>
    </div>
  )
}
