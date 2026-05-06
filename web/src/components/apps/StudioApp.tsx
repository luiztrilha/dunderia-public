import { useCallback, useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  applyPreview,
  getAgentSessions,
  getAdapterConfigChecks,
  getAdapterEnvironmentChecks,
  getBehaviorEvals,
  getBrowserInspectionHandoffPreview,
  getCompanyControlPlanePreview,
  getDesktopIDEPreview,
  getExecutionTrace,
  getFileContextHandoffPreview,
  getGovernanceRollbackPackages,
  getHumanPermissionsPreview,
  getIntakeQueues,
  getKnowledgeWikiPromotionPreview,
  getMarketplaceManifestPreview,
  getNoiseCleanupPreview,
  getOpenCoDesignStatus,
  getOperatorOverview,
  getOperatorRunbook,
  getOutcomes,
  getPluginRuntime,
  getPluginSandboxPreview,
  getProjectOverviewWidgetsPreview,
  getProviderCompatibilityPreview,
  getRemoteSandboxPreview,
  getReleaseReadiness,
  getSchedulerRevisionsPreview,
  getSkillCapabilityUpgradePreview,
  getSkillMetadataPreview,
  getStudioDevConsole,
  getStudioBootstrapPackage,
  getWikiEditorPreview,
  getWorkspaceInventory,
  generateStudioPackage,
  launchDesktopMode,
  launchOpenCoDesign,
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
import { StudioOperatorOverview } from './studio/StudioOperatorOverview'

type ActionState = 'idle' | 'working' | 'done'
type StudioView = 'operator' | 'summary' | 'context' | 'blockers'
type PreviewKind = 'noise_cleanup' | 'skill_metadata'
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
  const operatorQuery = useQuery({
    queryKey: ['operator-overview'],
    queryFn: () => getOperatorOverview({ allChannels: true, viewerSlug: 'human' }),
    refetchInterval: devConsoleInterval,
  })
  const releaseReadinessQuery = useQuery({
    queryKey: ['release-readiness'],
    queryFn: () => getReleaseReadiness(),
    refetchInterval: bootstrapInterval,
  })
  const skillMetadataQuery = useQuery({
    queryKey: ['skill-metadata-preview'],
    queryFn: () => getSkillMetadataPreview(),
    refetchInterval: bootstrapInterval,
  })
  const skillCapabilityQuery = useQuery({
    queryKey: ['skill-capability-upgrade-preview'],
    queryFn: () => getSkillCapabilityUpgradePreview(),
    refetchInterval: bootstrapInterval,
  })
  const adapterChecksQuery = useQuery({
    queryKey: ['adapter-environment-checks'],
    queryFn: () => getAdapterEnvironmentChecks(),
    refetchInterval: bootstrapInterval,
  })
  const adapterConfigChecksQuery = useQuery({
    queryKey: ['adapter-config-checks'],
    queryFn: () => getAdapterConfigChecks(),
    refetchInterval: bootstrapInterval,
  })
  const behaviorEvalsQuery = useQuery({
    queryKey: ['behavior-evals'],
    queryFn: () => getBehaviorEvals(),
    refetchInterval: bootstrapInterval,
  })
  const pluginRuntimeQuery = useQuery({
    queryKey: ['plugin-runtime'],
    queryFn: () => getPluginRuntime(),
    refetchInterval: bootstrapInterval,
  })
  const pluginSandboxQuery = useQuery({
    queryKey: ['plugin-sandbox-preview'],
    queryFn: () => getPluginSandboxPreview(),
    refetchInterval: bootstrapInterval,
  })
  const marketplaceManifestQuery = useQuery({
    queryKey: ['marketplace-manifest-preview'],
    queryFn: () => getMarketplaceManifestPreview(),
    refetchInterval: bootstrapInterval,
  })
  const browserHandoffQuery = useQuery({
    queryKey: ['browser-inspection-handoff-preview'],
    queryFn: () => getBrowserInspectionHandoffPreview({ allChannels: true, viewerSlug: 'human' }),
    refetchInterval: bootstrapInterval,
  })
  const remoteSandboxQuery = useQuery({
    queryKey: ['remote-sandbox-preview'],
    queryFn: () => getRemoteSandboxPreview(),
    refetchInterval: bootstrapInterval,
  })
  const schedulerRevisionsQuery = useQuery({
    queryKey: ['scheduler-revisions-preview'],
    queryFn: () => getSchedulerRevisionsPreview({ allChannels: true, viewerSlug: 'human' }),
    refetchInterval: bootstrapInterval,
  })
  const wikiEditorQuery = useQuery({
    queryKey: ['wiki-editor-preview'],
    queryFn: () => getWikiEditorPreview(),
    refetchInterval: bootstrapInterval,
  })
  const providerCompatibilityQuery = useQuery({
    queryKey: ['provider-compatibility-preview'],
    queryFn: () => getProviderCompatibilityPreview(),
    refetchInterval: bootstrapInterval,
  })
  const projectOverviewWidgetsQuery = useQuery({
    queryKey: ['project-overview-widgets-preview'],
    queryFn: () => getProjectOverviewWidgetsPreview(),
    refetchInterval: bootstrapInterval,
  })
  const fileContextHandoffQuery = useQuery({
    queryKey: ['file-context-handoff-preview'],
    queryFn: () => getFileContextHandoffPreview({ allChannels: true, viewerSlug: 'human' }),
    refetchInterval: bootstrapInterval,
  })
  const desktopIDEQuery = useQuery({
    queryKey: ['desktop-ide-preview'],
    queryFn: () => getDesktopIDEPreview(),
    refetchInterval: bootstrapInterval,
  })
  const companyControlPlaneQuery = useQuery({
    queryKey: ['company-control-plane-preview'],
    queryFn: () => getCompanyControlPlanePreview(),
    refetchInterval: bootstrapInterval,
  })
  const wikiPromotionQuery = useQuery({
    queryKey: ['knowledge-wiki-promotion-preview'],
    queryFn: () => getKnowledgeWikiPromotionPreview({ allChannels: true, viewerSlug: 'human' }),
    refetchInterval: bootstrapInterval,
  })
  const workspaceInventoryQuery = useQuery({
    queryKey: ['workspace-inventory'],
    queryFn: () => getWorkspaceInventory({ allChannels: true, viewerSlug: 'human' }),
    refetchInterval: bootstrapInterval,
  })
  const outcomesQuery = useQuery({
    queryKey: ['outcomes'],
    queryFn: () => getOutcomes({ allChannels: true, viewerSlug: 'human' }),
    refetchInterval: bootstrapInterval,
  })
  const agentSessionsQuery = useQuery({
    queryKey: ['agent-sessions'],
    queryFn: () => getAgentSessions({ allChannels: true, viewerSlug: 'human' }),
    refetchInterval: devConsoleInterval,
  })
  const executionTraceQuery = useQuery({
    queryKey: ['execution-trace'],
    queryFn: () => getExecutionTrace({ allChannels: true, viewerSlug: 'human' }),
    refetchInterval: bootstrapInterval,
  })
  const rollbackPackagesQuery = useQuery({
    queryKey: ['governance-rollback-packages'],
    queryFn: () => getGovernanceRollbackPackages({ allChannels: true, viewerSlug: 'human' }),
    refetchInterval: bootstrapInterval,
  })
  const humanPermissionsQuery = useQuery({
    queryKey: ['human-permissions-preview'],
    queryFn: () => getHumanPermissionsPreview({ allChannels: true, viewerSlug: 'human' }),
    refetchInterval: bootstrapInterval,
  })
  const intakeQueuesQuery = useQuery({
    queryKey: ['intake-queues'],
    queryFn: () => getIntakeQueues({ allChannels: true, viewerSlug: 'human' }),
    refetchInterval: devConsoleInterval,
  })
  const noiseCleanupQuery = useQuery({
    queryKey: ['noise-cleanup-preview'],
    queryFn: () => getNoiseCleanupPreview({ allChannels: true, viewerSlug: 'human' }),
    refetchInterval: bootstrapInterval,
  })
  const operatorRunbookQuery = useQuery({
    queryKey: ['operator-runbook'],
    queryFn: () => getOperatorRunbook({ allChannels: true, viewerSlug: 'human' }),
    refetchInterval: bootstrapInterval,
  })
  const openCoDesignQuery = useQuery({
    queryKey: ['open-codesign-status'],
    queryFn: () => getOpenCoDesignStatus(),
    refetchInterval: bootstrapInterval,
  })

  const [genState, setGenState] = useState<ActionState>('idle')
  const [runState, setRunState] = useState<ActionState>('idle')
  const [openCoDesignState, setOpenCoDesignState] = useState<ActionState>('idle')
  const [desktopState, setDesktopState] = useState<ActionState>('idle')
  const [pendingActionKey, setPendingActionKey] = useState<string | null>(null)
  const [applyingPreview, setApplyingPreview] = useState<PreviewKind | null>(null)
  const [selectedWorkflowKey, setSelectedWorkflowKey] = useState('')
  const [activeView, setActiveView] = useState<StudioView>('operator')

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
    await Promise.all([
      studioQuery.refetch(),
      bootstrapQuery.refetch(),
      operatorQuery.refetch(),
      releaseReadinessQuery.refetch(),
      skillMetadataQuery.refetch(),
      skillCapabilityQuery.refetch(),
      adapterChecksQuery.refetch(),
      adapterConfigChecksQuery.refetch(),
      behaviorEvalsQuery.refetch(),
      pluginRuntimeQuery.refetch(),
      pluginSandboxQuery.refetch(),
      marketplaceManifestQuery.refetch(),
      browserHandoffQuery.refetch(),
      remoteSandboxQuery.refetch(),
      schedulerRevisionsQuery.refetch(),
      wikiEditorQuery.refetch(),
      providerCompatibilityQuery.refetch(),
      projectOverviewWidgetsQuery.refetch(),
      fileContextHandoffQuery.refetch(),
      desktopIDEQuery.refetch(),
      companyControlPlaneQuery.refetch(),
      wikiPromotionQuery.refetch(),
      workspaceInventoryQuery.refetch(),
      outcomesQuery.refetch(),
      agentSessionsQuery.refetch(),
      executionTraceQuery.refetch(),
      rollbackPackagesQuery.refetch(),
      humanPermissionsQuery.refetch(),
      intakeQueuesQuery.refetch(),
      noiseCleanupQuery.refetch(),
      operatorRunbookQuery.refetch(),
      openCoDesignQuery.refetch(),
    ])
  }, [adapterChecksQuery, adapterConfigChecksQuery, agentSessionsQuery, behaviorEvalsQuery, bootstrapQuery, browserHandoffQuery, companyControlPlaneQuery, desktopIDEQuery, executionTraceQuery, fileContextHandoffQuery, humanPermissionsQuery, intakeQueuesQuery, marketplaceManifestQuery, noiseCleanupQuery, openCoDesignQuery, operatorQuery, operatorRunbookQuery, outcomesQuery, pluginRuntimeQuery, pluginSandboxQuery, projectOverviewWidgetsQuery, providerCompatibilityQuery, releaseReadinessQuery, remoteSandboxQuery, rollbackPackagesQuery, schedulerRevisionsQuery, skillCapabilityQuery, skillMetadataQuery, studioQuery, wikiEditorQuery, wikiPromotionQuery, workspaceInventoryQuery])

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

  const handleOpenCoDesign = useCallback(() => {
    setOpenCoDesignState('working')
    launchOpenCoDesign({ prototype_dir: openCoDesignQuery.data?.prototype_dir })
      .then((response) => {
        setOpenCoDesignState(response.launched ? 'done' : 'idle')
        if (response.launched) {
          showNotice(t('apps.studio.openCoDesign.launched'), 'success')
          setTimeout(() => setOpenCoDesignState('idle'), 2000)
        } else {
          showNotice(response.message || t('apps.studio.openCoDesign.unavailable'), 'info')
        }
        void openCoDesignQuery.refetch()
      })
      .catch((e: Error) => {
        setOpenCoDesignState('idle')
        showNotice(t('apps.studio.openCoDesign.launchFailed', { error: e.message }), 'error')
      })
  }, [openCoDesignQuery, t])

  const handleOpenDesktopMode = useCallback(() => {
    setDesktopState('working')
    launchDesktopMode({ web_url: window.location.origin })
      .then((response) => {
        setDesktopState(response.launched ? 'done' : 'idle')
        if (response.launched) {
          showNotice(t('apps.studio.desktopMode.launched'), 'success')
          setTimeout(() => setDesktopState('idle'), 2000)
        } else {
          showNotice(response.message || t('apps.studio.desktopMode.launchFailed', { error: 'not launched' }), 'info')
        }
      })
      .catch((e: Error) => {
        setDesktopState('idle')
        showNotice(t('apps.studio.desktopMode.launchFailed', { error: e.message }), 'error')
      })
  }, [t])

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

  const handleApplyPreview = useCallback(
    async (preview: PreviewKind, itemIds: string[]) => {
      const ids = itemIds.filter(Boolean)
      if (ids.length === 0) {
        showNotice(t('apps.studio.previewEmpty'), 'info')
        return
      }
      const ok = window.confirm(t('apps.studio.previewConfirm', { count: ids.length }))
      if (!ok) return
      const reason = window.prompt(t('apps.studio.previewReasonPrompt'))
      if (!reason?.trim()) {
        showNotice(t('apps.studio.previewReasonRequired'), 'info')
        return
      }
      setApplyingPreview(preview)
      try {
        const response = await applyPreview({
          preview,
          item_ids: ids,
          actor: 'human',
          reason: reason.trim(),
          confirm: true,
          confirmation: 'APPLY_PREVIEW',
        })
        showNotice(t('apps.studio.previewApplied', { count: response.applied }), 'success')
        await refetchAll()
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error)
        showNotice(t('apps.studio.previewApplyFailed', { error: message }), 'error')
      } finally {
        setApplyingPreview(null)
      }
    },
    [refetchAll, t],
  )

  const blockerCount = studioQuery.data?.blockers.length ?? 0
  const viewTabs: Array<{ key: StudioView; label: string; count?: number }> = [
    { key: 'operator', label: t('apps.studio.tabs.operator') },
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

          {activeView === 'operator' && (
            <StudioOperatorOverview
              overview={operatorQuery.data}
              loading={operatorQuery.isLoading}
              releaseReadiness={releaseReadinessQuery.data}
              skillMetadataPreview={skillMetadataQuery.data}
              skillCapabilityPreview={skillCapabilityQuery.data}
              adapterChecks={adapterChecksQuery.data}
              adapterConfigChecks={adapterConfigChecksQuery.data}
              behaviorEvals={behaviorEvalsQuery.data}
              pluginRuntime={pluginRuntimeQuery.data}
              pluginSandboxPreview={pluginSandboxQuery.data}
              marketplaceManifestPreview={marketplaceManifestQuery.data}
              browserHandoffPreview={browserHandoffQuery.data}
              remoteSandboxPreview={remoteSandboxQuery.data}
              schedulerRevisionsPreview={schedulerRevisionsQuery.data}
              wikiEditorPreview={wikiEditorQuery.data}
              providerCompatibilityPreview={providerCompatibilityQuery.data}
              projectOverviewWidgetsPreview={projectOverviewWidgetsQuery.data}
              fileContextHandoffPreview={fileContextHandoffQuery.data}
              desktopIDEPreview={desktopIDEQuery.data}
              companyControlPlanePreview={companyControlPlaneQuery.data}
              wikiPromotionPreview={wikiPromotionQuery.data}
              workspaceInventory={workspaceInventoryQuery.data}
              outcomes={outcomesQuery.data}
              agentSessions={agentSessionsQuery.data}
              executionTrace={executionTraceQuery.data}
              rollbackPackages={rollbackPackagesQuery.data}
              humanPermissions={humanPermissionsQuery.data}
              intakeQueues={intakeQueuesQuery.data}
              noiseCleanupPreview={noiseCleanupQuery.data}
              runbook={operatorRunbookQuery.data}
              openCoDesign={openCoDesignQuery.data}
              openCoDesignLoading={openCoDesignQuery.isLoading}
              openingOpenCoDesign={openCoDesignState === 'working'}
              onOpenCoDesign={handleOpenCoDesign}
              showDesktopMode={!Boolean((window as Window & { dunderiaDesktop?: { isDesktop?: boolean } }).dunderiaDesktop?.isDesktop)}
              openingDesktopMode={desktopState === 'working'}
              onOpenDesktopMode={handleOpenDesktopMode}
              applyingPreview={applyingPreview}
              onApplyPreview={handleApplyPreview}
              onRefresh={() => void refetchAll()}
              t={t}
            />
          )}
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
