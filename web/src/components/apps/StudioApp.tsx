import { useCallback, useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  getStudioDevConsole,
  getStudioBootstrapPackage,
  generateStudioPackage,
  reassignTask,
  runStudioDevConsoleAction,
  runStudioWorkflow,
  type StudioBlocker,
  type StudioBootstrapResponse,
  type StudioDevConsoleResponse,
} from '../../api/client'
import { useAppStore } from '../../stores/app'
import { useAgentRuntimeSummary } from '../../hooks/useAgentRuntimeSummary'
import { useBrokerRefetchInterval } from '../../hooks/useBrokerEvents'
import { showNotice } from '../ui/Toast'
import { StudioActiveContext } from './studio/StudioActiveContext'
import { StudioBlockerList } from './studio/StudioBlockerList'
import { StudioOfficeSnapshot } from './studio/StudioOfficeSnapshot'

type ActionState = 'idle' | 'working' | 'done'
type StudioView = 'summary' | 'context' | 'blockers'
const GAME_MASTER_SLUG = 'game-master'
const DELEGATE_GAME_MASTER_ACTION = 'delegate_game_master'

export function StudioApp() {
  const { t } = useTranslation()
  const setCurrentApp = useAppStore((s) => s.setCurrentApp)
  const setCurrentChannel = useAppStore((s) => s.setCurrentChannel)
  const currentChannel = useAppStore((s) => s.currentChannel)
  const devConsoleInterval = useBrokerRefetchInterval(15_000)
  const bootstrapInterval = useBrokerRefetchInterval(60_000)

  const studioQuery = useQuery({
    queryKey: ['studio-dev-console'],
    queryFn: () => getStudioDevConsole() as Promise<StudioDevConsoleResponse>,
    refetchInterval: devConsoleInterval,
  })
  const bootstrapQuery = useQuery({
    queryKey: ['studio-bootstrap'],
    queryFn: () => getStudioBootstrapPackage() as Promise<StudioBootstrapResponse>,
    refetchInterval: bootstrapInterval,
  })

  const [genState, setGenState] = useState<ActionState>('idle')
  const [runState, setRunState] = useState<ActionState>('idle')
  const [pendingActionKey, setPendingActionKey] = useState<string | null>(null)
  const [selectedWorkflowKey, setSelectedWorkflowKey] = useState('')
  const [activeView, setActiveView] = useState<StudioView>('summary')

  const actionDefinitions = useMemo(() => {
    return Object.fromEntries((studioQuery.data?.actions ?? []).map((action) => [action.action, action]))
  }, [studioQuery.data?.actions])

  const membersByChannel = useMemo(() => {
    return Object.fromEntries((studioQuery.data?.active_context.channels ?? []).map((channel) => [channel.slug, channel.members ?? []]))
  }, [studioQuery.data?.active_context.channels])
  const runtimeSummary = useAgentRuntimeSummary(studioQuery.data?.active_context.primary_channel || currentChannel)

  useEffect(() => {
    const workflows = bootstrapQuery.data?.package?.workflows ?? []
    if (workflows.length === 0) {
      setSelectedWorkflowKey('')
      return
    }
    setSelectedWorkflowKey((current) => {
      const stillExists = workflows.some((workflow) => (workflow.workflow_key ?? workflow.id) === current)
      return stillExists ? current : workflows[0]?.workflow_key ?? workflows[0]?.id ?? ''
    })
  }, [bootstrapQuery.data?.package?.workflows])

  const refetchAll = useCallback(async () => {
    await Promise.all([studioQuery.refetch(), bootstrapQuery.refetch()])
  }, [bootstrapQuery, studioQuery])

  const handleGenerate = useCallback(() => {
    setGenState('working')
    generateStudioPackage()
      .then(() => {
        setGenState('done')
        showNotice(t('apps.studio.generated'), 'success')
        setTimeout(() => setGenState('idle'), 2000)
        void refetchAll()
      })
      .catch((e: Error) => {
        setGenState('idle')
        showNotice(t('apps.studio.generateFailed', { error: e.message }), 'error')
      })
  }, [t, refetchAll])

  const handleRun = useCallback(() => {
    if (!selectedWorkflowKey) {
      showNotice(t('apps.studio.workflowRequired'), 'info')
      return
    }
    setRunState('working')
    runStudioWorkflow({ workflow_key: selectedWorkflowKey })
      .then(() => {
        setRunState('done')
        showNotice(t('apps.studio.ran'), 'success')
        setTimeout(() => setRunState('idle'), 2000)
        void refetchAll()
      })
      .catch((e: Error) => {
        setRunState('idle')
        showNotice(t('apps.studio.runFailed', { error: e.message }), 'error')
      })
  }, [selectedWorkflowKey, t, refetchAll])

  const handleAction = useCallback(
    async (action: string, blocker: StudioBlocker, extras?: { owner?: string }) => {
      const key = `${blocker.id}:${action}`
      const definition = actionDefinitions[action]
      setPendingActionKey(key)
      try {
        if (action === DELEGATE_GAME_MASTER_ACTION) {
          if (!blocker.task_id) {
            showNotice(t('apps.studio.delegateGameMasterMissingTask'), 'error')
            return
          }
          await reassignTask(blocker.task_id, GAME_MASTER_SLUG, blocker.channel || currentChannel, 'human')
          showNotice(t('apps.studio.delegatedGameMaster', { task: blocker.task_id }), 'success')
          await refetchAll()
          return
        }

        if (definition?.frontend_handled) {
          switch (action) {
            case 'inspect_task':
              setCurrentApp('tasks')
              showNotice(t('apps.studio.openedTasks'), 'info')
              break
            case 'inspect_channel': {
              const targetChannel = blocker.channel || studioQuery.data?.active_context.primary_channel || currentChannel
              setCurrentApp(null)
              setCurrentChannel(targetChannel)
              showNotice(t('apps.studio.openedChannel', { channel: targetChannel }), 'info')
              break
            }
            case 'create_task': {
              const targetChannel = blocker.channel || studioQuery.data?.active_context.primary_channel || currentChannel
              setCurrentApp(null)
              setCurrentChannel(targetChannel)
              showNotice(t('apps.studio.createTaskHint', { channel: targetChannel }), 'info')
              break
            }
            case 'refresh_snapshot':
              await refetchAll()
              showNotice(t('apps.studio.refreshed'), 'success')
              break
            default:
              break
          }
          return
        }

        const response = await runStudioDevConsoleAction({
          action,
          task_id: blocker.task_id,
          channel: blocker.channel,
          owner: extras?.owner,
          actor: 'human',
        })
        showNotice(response.message || t('apps.studio.actionSucceeded', { action: definition?.label || action }), 'success')
        await refetchAll()
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error)
        showNotice(t('apps.studio.actionFailed', { action: definition?.label || action, error: message }), 'error')
      } finally {
        setPendingActionKey(null)
      }
    },
    [actionDefinitions, currentChannel, refetchAll, setCurrentApp, setCurrentChannel, studioQuery.data?.active_context.primary_channel, t],
  )

  const blockerCount = studioQuery.data?.blockers.length ?? 0
  const viewTabs: Array<{ key: StudioView; label: string; count?: number }> = [
    { key: 'summary', label: t('apps.studio.tabs.summary') },
    { key: 'context', label: t('apps.studio.tabs.context') },
    { key: 'blockers', label: t('apps.studio.tabs.blockers'), count: blockerCount },
  ]

  return (
    <div
      data-testid="studio-dev-console"
      data-home-surface="studio"
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: 16,
        paddingBottom: 20,
      }}
    >
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        <div
          style={{
            fontSize: 11,
            textTransform: 'uppercase',
            letterSpacing: '0.08em',
            color: 'var(--text-tertiary)',
          }}
        >
          {t('apps.studio.eyebrow')}
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 16, flexWrap: 'wrap' }}>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            <h3 style={{ fontSize: 18, fontWeight: 700, margin: 0 }}>{t('apps.studio.title')}</h3>
            <div style={{ color: 'var(--text-secondary)', fontSize: 13, maxWidth: 760, lineHeight: 1.5 }}>
              {t('apps.studio.subtitle')}
            </div>
          </div>
          <div
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: 8,
              padding: '6px 10px',
              borderRadius: 999,
              background: blockerCount > 0 ? 'rgba(183, 112, 34, 0.12)' : 'rgba(73, 127, 77, 0.12)',
              color: blockerCount > 0 ? '#9f651f' : '#3b7b54',
              fontSize: 11,
              fontWeight: 700,
              textTransform: 'uppercase',
              letterSpacing: '0.05em',
            }}
          >
            {t('apps.studio.blockersCount', { count: blockerCount })}
          </div>
        </div>
      </div>

      {studioQuery.isLoading && !studioQuery.data && (
        <div style={{ padding: '32px 20px', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
          {t('apps.studio.loading')}
        </div>
      )}

      {studioQuery.error && !studioQuery.data && (
        <div className="app-card" style={{ padding: '24px 18px', display: 'flex', flexDirection: 'column', gap: 12 }}>
          <div style={{ color: 'var(--text-secondary)', fontSize: 14 }}>{t('apps.studio.loadFailed')}</div>
          <div>
            <button className="btn btn-secondary btn-sm" onClick={() => void studioQuery.refetch()}>
              {t('apps.studio.refresh')}
            </button>
          </div>
        </div>
      )}

      {studioQuery.data && (
        <>
          <div className="studio-view-tabs" role="tablist" aria-label={t('apps.studio.tabs.label')}>
            {viewTabs.map((tab) => (
              <button
                key={tab.key}
                type="button"
                role="tab"
                aria-selected={activeView === tab.key}
                className={`studio-view-tab${activeView === tab.key ? ' active' : ''}`}
                onClick={() => setActiveView(tab.key)}
              >
                <span>{tab.label}</span>
                {typeof tab.count === 'number' && <span className="studio-tab-count">{tab.count}</span>}
              </button>
            ))}
          </div>

          {activeView === 'summary' && (
            <StudioOfficeSnapshot
              office={studioQuery.data.office}
              environment={studioQuery.data.environment}
              runtimeSummary={runtimeSummary}
              bootstrapPackage={bootstrapQuery.data}
              refreshing={studioQuery.isFetching || bootstrapQuery.isFetching}
              generating={genState === 'working'}
              running={runState === 'working'}
              selectedWorkflowKey={selectedWorkflowKey}
              onWorkflowChange={setSelectedWorkflowKey}
              onRefresh={() => void refetchAll()}
              onGenerate={handleGenerate}
              onRun={handleRun}
              t={t}
            />
          )}
          {activeView === 'context' && (
            <StudioActiveContext
              context={studioQuery.data.active_context}
              onOpenChannel={(channel) => {
                setCurrentApp(null)
                setCurrentChannel(channel)
              }}
              onOpenTasks={() => setCurrentApp('tasks')}
              t={t}
            />
          )}
          {activeView === 'blockers' && (
            <StudioBlockerList
              blockers={studioQuery.data.blockers}
              actionDefinitions={actionDefinitions}
              membersByChannel={membersByChannel}
              pendingKey={pendingActionKey}
              onAction={(action, blocker, extras) => {
                void handleAction(action, blocker, extras)
              }}
              t={t}
            />
          )}
        </>
      )}
    </div>
  )
}
