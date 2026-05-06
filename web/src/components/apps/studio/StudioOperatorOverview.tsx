import type { TFunction } from 'i18next'
import { type ReactNode, useState } from 'react'
import type {
  AdapterEnvironmentChecks,
  AdapterConfigChecks,
  AgentSessions,
  BehaviorEvalReport,
  BrowserInspectionHandoffPreview,
  CompanyControlPlanePreview,
  DesktopIDEPreview,
  ExecutionTrace,
  FileContextHandoffPreview,
  GovernanceRollbackPackages,
  HumanPermissionsPreview,
  IntakeQueues,
  KnowledgeWikiPromotionPreview,
  MarketplaceManifestPreview,
  NoiseCleanupPreview,
  OpenCoDesignStatus,
  OperatorOverview,
  OperatorRunbook,
  Outcomes,
  PluginRuntime,
  PluginSandboxPreview,
  ProjectOverviewWidgetsPreview,
  ProviderCompatibilityPreview,
  RemoteSandboxPreview,
  ReleaseReadiness,
  SchedulerRevisionsPreview,
  SkillCapabilityUpgradePreview,
  SkillMetadataPreview,
  WikiEditorPreview,
  WorkspaceInventory,
} from '../../../api/client'

type PreviewKind = 'noise_cleanup' | 'skill_metadata'
type OperatorMode = 'operation' | 'diagnostics'

interface StudioOperatorOverviewProps {
  overview: OperatorOverview | undefined
  loading: boolean
  releaseReadiness?: ReleaseReadiness
  skillMetadataPreview?: SkillMetadataPreview
  skillCapabilityPreview?: SkillCapabilityUpgradePreview
  adapterChecks?: AdapterEnvironmentChecks
  adapterConfigChecks?: AdapterConfigChecks
  behaviorEvals?: BehaviorEvalReport
  pluginRuntime?: PluginRuntime
  pluginSandboxPreview?: PluginSandboxPreview
  marketplaceManifestPreview?: MarketplaceManifestPreview
  browserHandoffPreview?: BrowserInspectionHandoffPreview
  remoteSandboxPreview?: RemoteSandboxPreview
  schedulerRevisionsPreview?: SchedulerRevisionsPreview
  wikiEditorPreview?: WikiEditorPreview
  providerCompatibilityPreview?: ProviderCompatibilityPreview
  projectOverviewWidgetsPreview?: ProjectOverviewWidgetsPreview
  fileContextHandoffPreview?: FileContextHandoffPreview
  desktopIDEPreview?: DesktopIDEPreview
  companyControlPlanePreview?: CompanyControlPlanePreview
  wikiPromotionPreview?: KnowledgeWikiPromotionPreview
  workspaceInventory?: WorkspaceInventory
  outcomes?: Outcomes
  agentSessions?: AgentSessions
  executionTrace?: ExecutionTrace
  rollbackPackages?: GovernanceRollbackPackages
  humanPermissions?: HumanPermissionsPreview
  intakeQueues?: IntakeQueues
  noiseCleanupPreview?: NoiseCleanupPreview
  runbook?: OperatorRunbook
  openCoDesign?: OpenCoDesignStatus
  openCoDesignLoading?: boolean
  openingOpenCoDesign?: boolean
  onOpenCoDesign?: () => void
  showDesktopMode?: boolean
  openingDesktopMode?: boolean
  onOpenDesktopMode?: () => void
  applyingPreview?: PreviewKind | null
  onApplyPreview?: (preview: PreviewKind, itemIds: string[]) => void
  onRefresh: () => void
  t: TFunction
}

function tone(status: string): { bg: string; fg: string; border: string } {
  switch (status) {
    case 'ok':
    case 'high':
      return { bg: 'rgba(73, 127, 77, 0.10)', fg: '#3f8359', border: 'rgba(73, 127, 77, 0.25)' }
    case 'blocked':
    case 'low':
      return { bg: 'rgba(198, 68, 68, 0.10)', fg: '#bd4b55', border: 'rgba(198, 68, 68, 0.24)' }
    default:
      return { bg: 'rgba(183, 112, 34, 0.10)', fg: '#a7681f', border: 'rgba(183, 112, 34, 0.24)' }
  }
}

export function StudioOperatorOverview({
  overview,
  loading,
  releaseReadiness,
  skillMetadataPreview,
  skillCapabilityPreview,
  adapterChecks,
  adapterConfigChecks,
  behaviorEvals,
  pluginRuntime,
  pluginSandboxPreview,
  marketplaceManifestPreview,
  browserHandoffPreview,
  remoteSandboxPreview,
  schedulerRevisionsPreview,
  wikiEditorPreview,
  providerCompatibilityPreview,
  projectOverviewWidgetsPreview,
  fileContextHandoffPreview,
  desktopIDEPreview,
  companyControlPlanePreview,
  wikiPromotionPreview,
  workspaceInventory,
  outcomes,
  agentSessions,
  executionTrace,
  rollbackPackages,
  humanPermissions,
  intakeQueues,
  noiseCleanupPreview,
  runbook,
  openCoDesign,
  openCoDesignLoading = false,
  openingOpenCoDesign = false,
  onOpenCoDesign,
  showDesktopMode = false,
  openingDesktopMode = false,
  onOpenDesktopMode,
  applyingPreview,
  onApplyPreview,
  onRefresh,
  t,
}: StudioOperatorOverviewProps) {
  const [mode, setMode] = useState<OperatorMode>('operation')
  const statusTone = tone(overview?.status ?? 'degraded')
  const resumeTask = overview?.resume?.task
  const releaseTone = tone(releaseReadiness?.status === 'ready' ? 'ok' : releaseReadiness?.status === 'blocked' ? 'blocked' : 'warn')
  const metadataCount = skillMetadataPreview?.summary?.total ?? 0
  const capabilityCount = skillCapabilityPreview?.summary?.total ?? 0
  const adapterIssueCount = (adapterChecks?.summary?.warn ?? 0) + (adapterChecks?.summary?.fail ?? 0)
  const adapterConfigIssueCount = (adapterConfigChecks?.summary?.warn ?? 0) + (adapterConfigChecks?.summary?.fail ?? 0)
  const pluginIssueCount = (pluginRuntime?.plugins ?? []).filter((item) => ['blocked', 'degraded', 'fail', 'failed', 'missing'].includes((item.health_status || item.status || '').toLowerCase())).length
  const pluginSandboxBlockedCount = pluginSandboxPreview?.summary?.blocked ?? 0
  const pluginSandboxReadyWorkers = (pluginSandboxPreview?.candidates ?? []).filter((item) => item.worker_class && item.sandbox_status === 'ready')
  const marketplaceReviewCount = (marketplaceManifestPreview?.summary?.review ?? 0) + (marketplaceManifestPreview?.summary?.blocked ?? 0)
  const browserHandoffReviewCount = browserHandoffPreview?.summary?.review ?? 0
  const remoteSandboxBlockedCount = remoteSandboxPreview?.summary?.readiness_blocked ?? 0
  const schedulerRevisionBlockedCount = schedulerRevisionsPreview?.summary?.blocked ?? 0
  const wikiEditorMissingCount = wikiEditorPreview?.summary?.missing ?? 0
  const providerCompatMissingCount = providerCompatibilityPreview?.summary?.missing_tests ?? 0
  const projectWidgetReviewCount = projectOverviewWidgetsPreview?.summary?.review ?? 0
  const fileContextReviewCount = fileContextHandoffPreview?.summary?.review ?? 0
  const desktopIDEBlockedCount = desktopIDEPreview?.summary?.readiness_blocked ?? 0
  const companyControlPlaneBlockedCount = (companyControlPlanePreview?.summary?.blocked_mutations ?? 0) + (companyControlPlanePreview?.summary?.missing_policies ?? 0)
  const wikiPromotionIssueCount = (wikiPromotionPreview?.summary?.lint_error ?? 0) + (wikiPromotionPreview?.summary?.lint_warning ?? 0)
  const workspaceIssueCount = (workspaceInventory?.summary?.degraded ?? 0) + (workspaceInventory?.summary?.dirty ?? 0)
  const outcomeAcceptedCount = outcomes?.summary?.accepted ?? 0
  const activeSessionCount = agentSessions?.summary?.normalized_working
    ?? ((agentSessions?.summary?.active ?? 0) + (agentSessions?.summary?.running ?? 0) + (agentSessions?.summary?.executing ?? 0))
  const traceAttentionCount = executionTrace?.summary?.attention ?? 0
  const rollbackReviewCount = rollbackPackages?.summary?.requires_review ?? 0
  const humanTopologyBlockedCount = humanPermissions?.summary?.topology_blocked ?? 0
  const intakeCount = intakeQueues?.summary?.total ?? 0
  const noiseCount = noiseCleanupPreview?.summary?.total ?? 0
  const alerts = overview?.alerts ?? []
  const criticalAlertCount = alerts.filter((alert) => alert.severity === 'critical').length
  const inspectorSessions = (agentSessions?.sessions ?? []).filter((item) => item.status !== 'idle' || item.current_task_id || (item.liveness_history?.length ?? 0) > 0)
  const runbookSteps = runbook?.steps ?? []
  const safeNoiseItemIDs = (noiseCleanupPreview?.items ?? []).filter((item) => item.safe && !item.requires_review).map((item) => item.id)
  const metadataItemIDs = (skillMetadataPreview?.previews ?? []).map((item) => `skill:${item.name}`)
  const openCoDesignInstallCommand = openCoDesign?.install_commands?.[0] ?? ''

  if (loading && !overview) {
    return (
      <section className="app-card" style={{ padding: 28, color: 'var(--text-secondary)', fontSize: 13 }}>
        {t('apps.studio.operator.loading')}
      </section>
    )
  }

  return (
    <section
      data-testid="studio-operator-overview"
      className="app-card"
      style={{
        display: 'grid',
        gap: 16,
        padding: 18,
        background: 'var(--bg-card)',
      }}
    >
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 14, alignItems: 'flex-start', flexWrap: 'wrap' }}>
        <div style={{ display: 'grid', gap: 5 }}>
          <div style={{ fontSize: 17, fontWeight: 750 }}>{t('apps.studio.operator.title')}</div>
          <div style={{ color: 'var(--text-secondary)', fontSize: 13, lineHeight: 1.45, maxWidth: 760 }}>
            {t('apps.studio.operator.subtitle')}
          </div>
        </div>
        <button className="btn btn-secondary btn-sm" onClick={onRefresh}>
          {t('apps.studio.refresh')}
        </button>
      </div>

      <div className="studio-view-tabs" role="tablist" aria-label={t('apps.studio.operator.modeLabel')} style={{ margin: 0 }}>
        {(['operation', 'diagnostics'] as OperatorMode[]).map((key) => (
          <button
            key={key}
            type="button"
            role="tab"
            aria-selected={mode === key}
            className={`studio-view-tab${mode === key ? ' active' : ''}`}
            onClick={() => setMode(key)}
          >
            {t(`apps.studio.operator.modes.${key}`)}
          </button>
        ))}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(132px, 1fr))', gap: 10 }}>
        <CompactMetric label={t('apps.studio.operator.health')} value={(overview?.status ?? 'n/a').toUpperCase()} tone={statusTone} />
        <CompactMetric label={t('apps.studio.operator.openTasks')} value={overview?.counts.open_tasks ?? 0} />
        <CompactMetric label={t('apps.studio.operator.blockedTasks')} value={overview?.counts.blocked_tasks ?? 0} tone={tone((overview?.counts.blocked_tasks ?? 0) > 0 ? 'blocked' : 'ok')} />
        {mode === 'operation' ? (
          <>
            <CompactMetric label={t('apps.studio.operator.humanRequests')} value={overview?.counts.human_requests ?? 0} />
            <CompactMetric label={t('apps.studio.operator.alerts')} value={alerts.length} tone={tone(criticalAlertCount > 0 ? 'blocked' : alerts.length > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.intake')} value={intakeCount} tone={tone(intakeCount > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.runbook')} value={runbookSteps.length} tone={tone(runbookSteps.length > 0 ? 'warn' : 'ok')} />
          </>
        ) : (
          <>
            <CompactMetric label={t('apps.studio.operator.skillRisk')} value={overview?.skill_trust.low ?? 0} tone={tone((overview?.skill_trust.low ?? 0) > 0 ? 'low' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.release')} value={releaseReadiness ? `${releaseReadiness.score}/100` : 'n/a'} tone={releaseTone} />
            <CompactMetric label={t('apps.studio.operator.evals')} value={(behaviorEvals?.summary?.pass ?? 0) + '/' + (behaviorEvals?.summary?.total ?? 0)} tone={tone(behaviorEvals?.status === 'pass' ? 'ok' : 'warn')} />
            <CompactMetric label={t('apps.studio.operator.adapters')} value={adapterIssueCount} tone={tone(adapterIssueCount > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.configRefs')} value={adapterConfigIssueCount} tone={tone(adapterConfigIssueCount > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.plugins')} value={pluginRuntime?.summary?.plugins ?? 0} tone={tone(pluginIssueCount > 0 || pluginSandboxBlockedCount > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.marketplace')} value={marketplaceManifestPreview?.summary?.total ?? 0} tone={tone(marketplaceReviewCount > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.browser')} value={browserHandoffPreview?.summary?.total ?? 0} tone={tone(browserHandoffReviewCount > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.remoteSandbox')} value={remoteSandboxPreview?.summary?.total ?? 0} tone={tone(remoteSandboxBlockedCount > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.schedulerRevisions')} value={schedulerRevisionsPreview?.summary?.total ?? 0} tone={tone(schedulerRevisionBlockedCount > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.wikiEditor')} value={wikiEditorPreview?.summary?.checks ?? 0} tone={tone(wikiEditorMissingCount > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.providerCompat')} value={providerCompatibilityPreview?.summary?.total ?? 0} tone={tone(providerCompatMissingCount > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.projectWidgets')} value={projectOverviewWidgetsPreview?.summary?.total ?? 0} tone={tone(projectWidgetReviewCount > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.fileHandoff')} value={fileContextHandoffPreview?.summary?.total ?? 0} tone={tone(fileContextReviewCount > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.desktopPreview')} value={desktopIDEPreview?.summary?.total ?? 0} tone={tone(desktopIDEBlockedCount > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.companies')} value={companyControlPlanePreview?.summary?.companies ?? 0} tone={tone(companyControlPlaneBlockedCount > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.wiki')} value={wikiPromotionPreview?.summary?.total ?? 0} tone={tone(wikiPromotionIssueCount > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.sessions')} value={activeSessionCount + '/' + (agentSessions?.summary?.total ?? 0)} tone={tone(activeSessionCount > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.traces')} value={executionTrace?.summary?.total ?? 0} tone={tone(traceAttentionCount > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.rollback')} value={rollbackPackages?.summary?.total ?? 0} tone={tone(rollbackReviewCount > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.permissions')} value={humanPermissions?.summary?.total ?? 0} tone={tone(humanTopologyBlockedCount > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.workspaces')} value={workspaceInventory?.summary?.total ?? 0} tone={tone(workspaceIssueCount > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.outcomes')} value={outcomeAcceptedCount + '/' + (outcomes?.summary?.total ?? 0)} tone={tone((outcomes?.summary?.needs_evidence ?? 0) > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.capabilities')} value={capabilityCount} tone={tone(capabilityCount > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.metadata')} value={metadataCount} tone={tone(metadataCount > 0 ? 'warn' : 'ok')} />
            <CompactMetric label={t('apps.studio.operator.noise')} value={noiseCount} tone={tone(noiseCount > 0 ? 'warn' : 'ok')} />
          </>
        )}
      </div>

      {mode === 'operation' ? (
        <>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(min(260px, 100%), 1fr))', gap: 12 }}>
            <OperatorPanel title={t('apps.studio.operator.nextWork')}>
              {(overview?.next_work ?? []).length === 0 ? (
                <EmptyLine>{t('apps.studio.operator.empty')}</EmptyLine>
              ) : (
                <div style={{ display: 'grid', gap: 8 }}>
                  {(overview?.next_work ?? []).slice(0, 5).map((item) => (
                    <div key={item.task_id} style={{ display: 'grid', gap: 4, padding: '10px 0', borderBottom: '1px solid var(--border)' }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 10 }}>
                        <strong style={{ fontSize: 13, lineHeight: 1.35 }}>{item.title}</strong>
                        <span style={{ color: 'var(--text-tertiary)', fontSize: 11, whiteSpace: 'nowrap' }}>{item.owner || item.channel}</span>
                      </div>
                      <div style={{ color: 'var(--text-secondary)', fontSize: 11 }}>
                        {item.status} · {item.queue_key || 'active'} · {item.priority || 'normal'}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </OperatorPanel>

            <OperatorPanel title={t('apps.studio.operator.resumePack')}>
              {resumeTask ? (
                <div style={{ display: 'grid', gap: 10 }}>
                  <div style={{ display: 'grid', gap: 4 }}>
                    <strong style={{ fontSize: 14, lineHeight: 1.35 }}>{resumeTask.title}</strong>
                    <span style={{ color: 'var(--text-secondary)', fontSize: 11 }}>{resumeTask.id} · {resumeTask.owner || resumeTask.channel}</span>
                  </div>
                  <div style={{ display: 'grid', gap: 6 }}>
                    {(overview?.resume?.next_steps ?? []).slice(0, 3).map((step) => (
                      <div key={step} style={{ color: 'var(--text-secondary)', fontSize: 12, lineHeight: 1.45 }}>
                        {step}
                      </div>
                    ))}
                  </div>
                </div>
              ) : (
                <EmptyLine>{t('apps.studio.operator.noResume')}</EmptyLine>
              )}
            </OperatorPanel>
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 12 }}>
            {showDesktopMode ? (
              <OperatorPanel
                title={t('apps.studio.desktopMode.title')}
                action={onOpenDesktopMode ? (
                  <button
                    className="btn btn-primary btn-sm"
                    disabled={openingDesktopMode}
                    onClick={onOpenDesktopMode}
                    title={t('apps.studio.desktopMode.buttonTitle')}
                  >
                    {openingDesktopMode ? t('apps.studio.desktopMode.opening') : t('apps.studio.desktopMode.open')}
                  </button>
                ) : null}
              >
                <div style={{ display: 'grid', gap: 7 }}>
                  <div style={{ color: 'var(--text-secondary)', fontSize: 12, lineHeight: 1.45 }}>
                    {t('apps.studio.desktopMode.summary')}
                  </div>
                  <div style={{ color: 'var(--text-tertiary)', fontSize: 11, lineHeight: 1.45 }}>
                    {t('apps.studio.desktopMode.detail')}
                  </div>
                </div>
              </OperatorPanel>
            ) : null}
            <OperatorPanel title={t('apps.studio.operator.alerts')}>
              <TinyList
                items={alerts.map((item) => `${item.title}: ${item.action || item.summary}${item.channel ? ` · #${item.channel}` : ''}`)}
                empty={t('apps.studio.operator.noAlerts')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.sessionInspector')}>
              <TinyList
                items={inspectorSessions.map((item) => {
                  const usage = item.usage?.total_tokens ? ` · ${item.usage.total_tokens} tok` : ''
                  const liveness = item.liveness_state ? ` · ${item.liveness_state}` : ''
                  const status = item.normalized_status || item.status
                  const latest = item.liveness_history?.[0]?.reason || item.next_action || item.detail || item.status
                  return `@${item.slug}: ${item.current_task_title || item.activity || item.status} · ${status}${liveness}${usage} · ${latest}`
                })}
                empty={t('apps.studio.operator.noSessionInspector')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.blockers')}>
              <TinyList items={(overview?.blockers ?? []).map((item) => item.summary || item.title)} empty={t('apps.studio.operator.noBlockers')} />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.requests')}>
              <TinyList items={(overview?.requests ?? []).map((item) => item.question || item.title || item.id)} empty={t('apps.studio.operator.noRequests')} />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.runbook')}>
              <TinyList
                items={runbookSteps.map((step) => `${step.title}${step.command ? ` · ${step.command}` : step.endpoint ? ` · ${step.endpoint}` : ''}`)}
                empty={t('apps.studio.operator.noRunbook')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.intakeQueues')}>
              <TinyList
                items={(intakeQueues?.queues ?? []).map((queue) => `${queue.label}: ${queue.count}${queue.next?.title ? ` · ${queue.next.title}` : ''}`)}
                empty={t('apps.studio.operator.noIntake')}
              />
            </OperatorPanel>
            <OperatorPanel
              title={t('apps.studio.openCoDesign.title')}
              action={onOpenCoDesign ? (
                <button
                  className="btn btn-secondary btn-sm"
                  disabled={openingOpenCoDesign}
                  onClick={onOpenCoDesign}
                  title={t('apps.studio.openCoDesign.buttonTitle')}
                >
                  {openingOpenCoDesign ? t('apps.studio.openCoDesign.opening') : t('apps.studio.openCoDesign.open')}
                </button>
              ) : null}
            >
              <div style={{ display: 'grid', gap: 7 }}>
                <div style={{ color: openCoDesign?.available ? '#3f8359' : '#a7681f', fontSize: 12, fontWeight: 700 }}>
                  {openCoDesignLoading
                    ? t('apps.studio.openCoDesign.checking')
                    : openCoDesign?.available
                      ? t('apps.studio.openCoDesign.available')
                      : t('apps.studio.openCoDesign.unavailable')}
                </div>
                <div style={{ color: 'var(--text-secondary)', fontSize: 12, lineHeight: 1.45 }}>
                  {openCoDesign?.prototype_dir || t('apps.studio.openCoDesign.noHandoff')}
                </div>
                {!openCoDesign?.available && openCoDesignInstallCommand ? (
                  <div style={{ color: 'var(--text-tertiary)', fontSize: 11, lineHeight: 1.45 }}>
                    {openCoDesignInstallCommand}
                  </div>
                ) : null}
              </div>
            </OperatorPanel>
          </div>
        </>
      ) : (
        <>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 12 }}>
            <OperatorPanel title={t('apps.studio.operator.releaseReadiness')}>
              <TinyList
                items={(releaseReadiness?.checks ?? [])
                  .filter((check) => check.status !== 'ok')
                  .map((check) => `${check.id}: ${check.detail || check.next_step || check.summary}`)}
                empty={t('apps.studio.operator.releaseReady')}
              />
            </OperatorPanel>
            <OperatorPanel
              title={t('apps.studio.operator.skillMetadata')}
              action={metadataItemIDs.length > 0 && onApplyPreview ? (
                <button className="btn btn-secondary btn-sm" disabled={applyingPreview === 'skill_metadata'} onClick={() => onApplyPreview('skill_metadata', metadataItemIDs)}>
                  {applyingPreview === 'skill_metadata' ? t('apps.studio.applyingPreview') : t('apps.studio.applyPreview')}
                </button>
              ) : null}
            >
              <TinyList
                items={(skillMetadataPreview?.previews ?? []).map((item) => `${item.name}: ${(item.would_update ?? []).join(', ')}`)}
                empty={t('apps.studio.operator.noSkillMetadata')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.capabilityUpgrades')}>
              <TinyList
                items={(skillCapabilityPreview?.previews ?? []).map((item) => `${item.skill_name}: ${(item.added_capabilities ?? []).join(', ')} · ${item.risk_level}`)}
                empty={t('apps.studio.operator.noCapabilityUpgrades')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.adapterChecks')}>
              <TinyList
                items={(adapterChecks?.checks ?? []).filter((item) => item.status !== 'ok').map((item) => `${item.name}: ${item.next_step || item.summary}`)}
                empty={t('apps.studio.operator.adaptersReady')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.adapterConfigRefs')}>
              <TinyList
                items={(adapterConfigChecks?.checks ?? [])
                  .filter((item) => item.status !== 'ok')
                  .map((item) => `${item.name}: ${item.next_step || item.summary}`)}
                empty={t('apps.studio.operator.configRefsReady')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.pluginRuntime')}>
              <TinyList
                items={[
                  t('apps.studio.operator.pluginRuntimeSummary', {
                    plugins: pluginRuntime?.summary?.plugins ?? 0,
                    jobs: pluginRuntime?.summary?.jobs ?? 0,
                    runs: pluginRuntime?.summary?.runs ?? 0,
                  }),
                  ...(pluginRuntime?.runs ?? []).slice(0, 3).map((item) => `${item.plugin_id || item.actor || 'runtime'}: ${item.action} · ${item.status}`),
                ]}
                empty={t('apps.studio.operator.pluginsQuiet')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.pluginSandbox')}>
              <TinyList
                items={[
                  ...pluginSandboxReadyWorkers.map((item) => `${item.name}: ${item.sandbox_status} · ${item.worker_class} · ${item.health_check || item.runtime_status}`),
                  ...(pluginSandboxPreview?.candidates ?? [])
                    .filter((item) => item.sandbox_status !== 'ready')
                    .slice(0, 5)
                    .map((item) => `${item.name}: ${item.sandbox_status} · ${item.next_step || (item.missing_policies ?? []).join(', ')}`),
                ]}
                empty={t('apps.studio.operator.pluginSandboxReady')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.marketplaceManifest')}>
              <TinyList
                items={(marketplaceManifestPreview?.manifests ?? [])
                  .filter((item) => item.manifest_status !== 'ready' || item.drift_status !== 'none')
                  .slice(0, 5)
                  .map((item) => `${item.name}: ${item.manifest_status} · ${item.drift_status} · ${item.signature_status}`)}
                empty={t('apps.studio.operator.marketplaceReady')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.browserHandoff')}>
              <TinyList
                items={(browserHandoffPreview?.handoffs ?? [])
                  .slice(0, 5)
                  .map((item) => `${item.task_title || item.task_id}: ${item.ready ? 'ready' : 'review'} · ${item.selector || item.page_url || item.artifact_id}`)}
                empty={t('apps.studio.operator.noBrowserHandoff')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.remoteSandboxPreview')}>
              <TinyList
                items={(remoteSandboxPreview?.candidates ?? [])
                  .slice(0, 5)
                  .map((item) => `${item.provider}: ${item.readiness} · ${item.execution_enabled ? 'enabled' : 'disabled'} · ${item.install_command_enabled ? 'install enabled' : 'install disabled'} · ${(item.missing_policies ?? []).slice(0, 2).join(', ')}`)}
                empty={t('apps.studio.operator.noRemoteSandbox')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.schedulerRevisionPreview')}>
              <TinyList
                items={(schedulerRevisionsPreview?.jobs ?? [])
                  .slice(0, 5)
                  .map((item) => `${item.label || item.slug}: ${item.restore_readiness} · ${item.restore_enabled ? 'restore enabled' : 'restore disabled'} · ${(item.missing_policies ?? []).slice(0, 2).join(', ')}`)}
                empty={t('apps.studio.operator.noSchedulerRevisions')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.wikiEditorPreview')}>
              <TinyList
                items={[
                  ...(wikiEditorPreview?.modes ?? []).map((item) => `${item.label}: ${item.readiness} · ${item.editor_enabled ? 'enabled' : 'disabled'}`),
                  ...(wikiEditorPreview?.checks ?? []).filter((item) => item.status !== 'ok').slice(0, 3).map((item) => `${item.id}: ${item.status}`),
                ]}
                empty={t('apps.studio.operator.noWikiEditor')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.providerCompatibility')}>
              <TinyList
                items={(providerCompatibilityPreview?.providers ?? [])
                  .slice(0, 5)
                  .map((item) => `${item.provider}: ${item.readiness} · ${(item.missing_tests ?? []).length} missing tests`)}
                empty={t('apps.studio.operator.noProviderCompatibility')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.projectWidgetsPreview')}>
              <TinyList
                items={(projectOverviewWidgetsPreview?.widgets ?? [])
                  .slice(0, 5)
                  .map((item) => `${item.title}: ${item.readiness} · ${item.count ?? 0} · ${item.mutation_enabled ? 'mutable' : 'read-only'}`)}
                empty={t('apps.studio.operator.noProjectWidgets')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.fileContextHandoff')}>
              <TinyList
                items={(fileContextHandoffPreview?.items ?? [])
                  .slice(0, 5)
                  .map((item) => `${item.task_title || item.task_id}: ${item.source} · ${item.content_included ? 'content' : 'reference'} · ${(item.missing_policies ?? []).slice(0, 2).join(', ')}`)}
                empty={t('apps.studio.operator.noFileContextHandoff')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.desktopIDEPreview')}>
              <TinyList
                items={(desktopIDEPreview?.surfaces ?? [])
                  .slice(0, 5)
                  .map((item) => `${item.name}: ${item.readiness} · ${item.canonical_surface || item.kind} · ${(item.missing_checks ?? []).slice(0, 2).join(', ')}`)}
                empty={t('apps.studio.operator.noDesktopPreview')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.companyControlPlane')}>
              <TinyList
                items={[
                  companyControlPlanePreview?.current_company
                    ? `${companyControlPlanePreview.current_company.name}: ${companyControlPlanePreview.current_company.member_count} members · ${companyControlPlanePreview.current_company.channel_count} channels`
                    : '',
                  ...(companyControlPlanePreview?.isolation ?? []).slice(0, 3).map((item) => `${item.id}: ${item.status} · ${item.summary}`),
                ]}
                empty={t('apps.studio.operator.noCompanyControlPlane')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.wikiPromotion')}>
              <TinyList
                items={(wikiPromotionPreview?.proposals ?? [])
                  .slice(0, 5)
                  .map((item) => `${item.title}: ${item.action} · ${item.target_path} · ${item.lint_findings?.length ?? 0} lint`)}
                empty={t('apps.studio.operator.noWikiPromotion')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.agentSessions')}>
              <TinyList
                items={(agentSessions?.sessions ?? [])
                  .filter((item) => item.status !== 'idle' || item.current_task_id)
                  .map((item) => `@${item.slug}: ${item.normalized_status || item.status} · ${item.current_task_title || item.activity || item.status}${item.heartbeat_at ? ' · heartbeat' : ''}${item.liveness_history?.length ? ` · ${item.liveness_history.length} liveness` : ''}`)}
                empty={t('apps.studio.operator.noAgentSessions')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.executionTrace')}>
              <TinyList
                items={(executionTrace?.traces ?? []).slice(0, 4).map((item) => `${item.title}: ${item.normalized_status || item.status || 'waiting'} · ${item.steps.length} ${t('apps.studio.operator.traceSteps')}`)}
                empty={t('apps.studio.operator.noExecutionTrace')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.rollbackPackages')}>
              <TinyList
                items={(rollbackPackages?.packages ?? []).slice(0, 4).map((item) => `${item.event_kind}: ${item.changes.length} ${t('apps.studio.operator.rollbackSteps')}`)}
                empty={t('apps.studio.operator.noRollbackPackages')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.permissionsPreview')}>
              <TinyList
                items={(humanPermissions?.snapshots ?? []).map((item) => `${item.channel}: ${item.access_level} · ${item.can_mutate_topology ? t('apps.studio.operator.topologyAllowed') : t('apps.studio.operator.topologyBlocked')}`)}
                empty={t('apps.studio.operator.noPermissionsPreview')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.workspaceInventory')}>
              <TinyList
                items={(workspaceInventory?.workspaces ?? [])
                  .filter((item) => !item.healthy || (item.git_dirty_count ?? 0) > 0)
                  .map((item) => `${item.kind || 'workspace'}: ${item.issue || `${item.git_dirty_count ?? 0} dirty`} · ${item.channel || 'global'}`)}
                empty={t('apps.studio.operator.workspacesReady')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.outcomeTaxonomy')}>
              <TinyList
                items={(outcomes?.items ?? []).slice(0, 4).map((item) => `${item.kind}: ${item.title} · ${item.state}`)}
                empty={t('apps.studio.operator.noOutcomes')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.behaviorEvals')}>
              <TinyList
                items={(behaviorEvals?.cases ?? []).filter((item) => item.status !== 'pass').map((item) => `${item.id}: ${item.summary}`)}
                empty={t('apps.studio.operator.evalsPass')}
              />
            </OperatorPanel>
            <OperatorPanel
              title={t('apps.studio.operator.noiseCleanup')}
              action={safeNoiseItemIDs.length > 0 && onApplyPreview ? (
                <button className="btn btn-secondary btn-sm" disabled={applyingPreview === 'noise_cleanup'} onClick={() => onApplyPreview('noise_cleanup', safeNoiseItemIDs)}>
                  {applyingPreview === 'noise_cleanup' ? t('apps.studio.applyingPreview') : t('apps.studio.applyPreview')}
                </button>
              ) : null}
            >
              <TinyList
                items={(noiseCleanupPreview?.items ?? []).map((item) => `${item.would_action}: ${item.title}`)}
                empty={t('apps.studio.operator.noNoise')}
              />
            </OperatorPanel>
            <OperatorPanel title={t('apps.studio.operator.governance')}>
              <TinyList items={(overview?.governance ?? []).map((item) => item.summary)} empty={t('apps.studio.operator.noGovernance')} />
            </OperatorPanel>
          </div>
        </>
      )}
    </section>
  )
}

function CompactMetric({ label, value, tone: metricTone }: { label: string; value: string | number; tone?: { bg: string; fg: string; border: string } }) {
  const t = metricTone ?? tone('neutral')
  return (
    <div style={{ padding: '12px 13px', borderRadius: 8, background: t.bg, border: `1px solid ${t.border}`, minWidth: 0 }}>
      <div style={{ color: t.fg, fontSize: 10, fontWeight: 750, textTransform: 'uppercase', letterSpacing: '0.04em' }}>{label}</div>
      <div style={{ color: 'var(--text-primary)', fontSize: 18, fontWeight: 760, marginTop: 5, overflow: 'hidden', textOverflow: 'ellipsis' }}>{value}</div>
    </div>
  )
}

function OperatorPanel({ title, action, children }: { title: string; action?: ReactNode; children: ReactNode }) {
  return (
    <div style={{ border: '1px solid var(--border)', borderRadius: 8, padding: 14, minWidth: 0, background: 'var(--bg)' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8, marginBottom: 8 }}>
        <div style={{ fontSize: 12, fontWeight: 760 }}>{title}</div>
        {action}
      </div>
      {children}
    </div>
  )
}

function TinyList({ items, empty }: { items: string[]; empty: string }) {
  const visible = items.filter(Boolean).slice(0, 4)
  if (visible.length === 0) return <EmptyLine>{empty}</EmptyLine>
  return (
    <div style={{ display: 'grid', gap: 7 }}>
      {visible.map((item) => (
        <div key={item} style={{ color: 'var(--text-secondary)', fontSize: 12, lineHeight: 1.4 }}>
          {item}
        </div>
      ))}
    </div>
  )
}

function EmptyLine({ children }: { children: ReactNode }) {
  return <div style={{ color: 'var(--text-tertiary)', fontSize: 12, lineHeight: 1.45 }}>{children}</div>
}
