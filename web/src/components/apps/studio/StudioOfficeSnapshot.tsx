import type { TFunction } from 'i18next'
import type {
  StudioBootstrapResponse,
  StudioEnvironmentSnapshot,
  StudioOfficeSnapshot,
} from '../../../api/client'
import type { AgentRuntimeSummary } from '../../../lib/agentRuntime'

interface StudioOfficeSnapshotProps {
  office: StudioOfficeSnapshot
  environment: StudioEnvironmentSnapshot
  runtimeSummary: AgentRuntimeSummary
  bootstrapPackage: StudioBootstrapResponse | undefined
  refreshing: boolean
  generating: boolean
  running: boolean
  selectedWorkflowKey: string
  onWorkflowChange: (workflowKey: string) => void
  onRefresh: () => void
  onGenerate: () => void
  onRun: () => void
  t: TFunction
}

function statusColor(status: string): { bg: string; fg: string } {
  switch (status) {
    case 'healthy':
    case 'ok':
      return { bg: 'rgba(73, 127, 77, 0.12)', fg: '#3b7b54' }
    case 'degraded':
      return { bg: 'rgba(183, 112, 34, 0.12)', fg: '#9f651f' }
    case 'blocked':
      return { bg: 'rgba(198, 68, 68, 0.12)', fg: '#b24a4a' }
    default:
      return { bg: 'rgba(111, 118, 132, 0.12)', fg: 'var(--text-secondary)' }
  }
}

function boolTone(ok: boolean): { bg: string; fg: string } {
  return ok
    ? { bg: 'rgba(73, 127, 77, 0.12)', fg: '#3b7b54' }
    : { bg: 'rgba(198, 68, 68, 0.12)', fg: '#b24a4a' }
}

function workflowOptions(bootstrapPackage: StudioBootstrapResponse | undefined) {
  return (bootstrapPackage?.package?.workflows ?? []).map((workflow) => ({
    key: workflow.workflow_key ?? workflow.id,
    label: workflow.name ?? workflow.workflow_key ?? workflow.id,
    status: workflow.status,
  }))
}

function formatPackageLabel(packageData: StudioBootstrapResponse['package'] | undefined): string {
  if (!packageData) return 'bootstrap-package'
  if (typeof packageData.name === 'string' && packageData.name.trim()) return packageData.name
  const blueprint = packageData.blueprint
  if (typeof blueprint === 'string' && blueprint.trim()) return blueprint
  if (blueprint && typeof blueprint === 'object') {
    const candidate = [
      (blueprint as Record<string, unknown>).name,
      (blueprint as Record<string, unknown>).id,
      (blueprint as Record<string, unknown>).kind,
    ].find((value) => typeof value === 'string' && value.trim())
    if (typeof candidate === 'string' && candidate.trim()) return candidate
  }
  return 'bootstrap-package'
}

export function StudioOfficeSnapshot({
  office,
  environment,
  runtimeSummary,
  bootstrapPackage,
  refreshing,
  generating,
  running,
  selectedWorkflowKey,
  onWorkflowChange,
  onRefresh,
  onGenerate,
  onRun,
  t,
}: StudioOfficeSnapshotProps) {
  const officeTone = statusColor(office.status)
  const environmentTone = statusColor(environment.status)
  const packageData = bootstrapPackage?.package
  const workflows = workflowOptions(bootstrapPackage)
  const packageLabel = formatPackageLabel(packageData)

  return (
    <section
      data-testid="studio-office-snapshot"
      className="app-card"
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: 18,
        padding: 18,
        background: 'linear-gradient(180deg, var(--bg-card) 0%, color-mix(in srgb, var(--bg-card) 86%, transparent) 100%)',
      }}
    >
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 16, flexWrap: 'wrap', alignItems: 'flex-start' }}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
          <div style={{ fontSize: 16, fontWeight: 700 }}>{t('apps.studio.officeTitle')}</div>
          <div className="app-card-meta">{t('apps.studio.officeSummary')}</div>
        </div>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
          <button className="btn btn-secondary btn-sm" onClick={onRefresh} disabled={refreshing}>
            {refreshing ? t('apps.studio.refreshing') : t('apps.studio.refresh')}
          </button>
          <button className="btn btn-primary btn-sm" onClick={onGenerate} disabled={generating}>
            {generating ? t('apps.studio.generating') : t('apps.studio.generate')}
          </button>
        </div>
      </div>

      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
        {[
          { label: office.status, tone: officeTone },
          { label: office.focus_mode ? t('apps.studio.focusModeOn') : t('apps.studio.focusModeOff'), tone: statusColor(office.focus_mode ? 'healthy' : 'warning') },
          { label: `${t('apps.studio.provider')}: ${office.provider || 'n/a'}`, tone: statusColor('ok') },
          { label: `${t('apps.studio.memoryBackend')}: ${office.memory_backend || 'n/a'}`, tone: statusColor('ok') },
        ].map((pill) => (
          <span
            key={pill.label}
            style={{
              padding: '5px 10px',
              borderRadius: 999,
              background: pill.tone.bg,
              color: pill.tone.fg,
              fontSize: 11,
              fontWeight: 700,
              textTransform: 'uppercase',
              letterSpacing: '0.04em',
            }}
          >
            {pill.label}
          </span>
        ))}
      </div>

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))',
          gap: 12,
        }}
      >
        <SnapshotCard title={t('apps.studio.runtimeHealth')} tone={environmentTone}>
          <HealthGrid
            rows={[
              { label: t('apps.studio.broker'), ok: environment.broker_reachable },
              { label: t('apps.studio.api'), ok: environment.api_reachable },
              { label: t('apps.studio.web'), ok: environment.web_reachable },
            ]}
          />
          {(environment.signals ?? []).length > 0 && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
              <FactLabel>{t('apps.studio.signals')}</FactLabel>
              <ul style={{ margin: 0, paddingLeft: 18, color: 'var(--text-secondary)', fontSize: 12, lineHeight: 1.5 }}>
                {(environment.signals ?? []).slice(0, 4).map((signal) => (
                  <li key={signal}>{signal}</li>
                ))}
              </ul>
            </div>
          )}
        </SnapshotCard>

        <SnapshotCard title={t('apps.studio.bootstrap')} tone={officeTone}>
          <div style={{ fontSize: 13, lineHeight: 1.5, color: 'var(--text-secondary)' }}>{office.bootstrap.summary}</div>
          <MetricGrid
            metrics={[
              { label: t('apps.studio.members'), value: office.bootstrap.members ?? 0 },
              { label: t('apps.studio.channels'), value: office.bootstrap.channels ?? 0 },
              { label: t('apps.studio.tasks'), value: office.bootstrap.tasks ?? office.task_counts.total ?? 0 },
              { label: t('apps.studio.requests'), value: office.bootstrap.requests ?? 0 },
              { label: t('apps.studio.workspaces'), value: office.bootstrap.workspaces ?? 0 },
              { label: t('apps.studio.workflows'), value: office.bootstrap.workflows ?? 0 },
            ]}
          />
        </SnapshotCard>

        <SnapshotCard title={t('apps.studio.agentStatusTitle')} tone={statusColor(runtimeSummary.counts.blocked > 0 ? 'blocked' : runtimeSummary.counts.waiting > 0 ? 'degraded' : 'healthy')}>
          <div data-testid="studio-agent-status-card" style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            <StatusMetricGrid
              metrics={[
                { key: 'working', label: t('sidebar.runtime.working'), value: runtimeSummary.counts.working },
                { key: 'blocked', label: t('sidebar.runtime.blocked'), value: runtimeSummary.counts.blocked },
                { key: 'waiting', label: t('sidebar.runtime.waiting'), value: runtimeSummary.counts.waiting },
                { key: 'silent', label: t('sidebar.runtime.silent'), value: runtimeSummary.counts.silent },
              ]}
            />
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
              {runtimeSummary.members.slice(0, 4).map((member) => (
                <div
                  key={member.slug}
                  style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    gap: 12,
                    padding: '10px 12px',
                    borderRadius: 14,
                    background: 'var(--bg)',
                    border: '1px solid var(--border)',
                  }}
                >
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                    <span style={{ fontSize: 13, fontWeight: 600 }}>{member.name || member.slug}</span>
                    <span style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>{member.runtimeDetail}</span>
                  </div>
                  <span style={{ fontSize: 11, color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.04em' }}>
                    {t(`sidebar.runtime.${member.runtimeState}`)}
                  </span>
                </div>
              ))}
            </div>
          </div>
        </SnapshotCard>

        <SnapshotCard title={t('apps.studio.packageTools')} tone={statusColor(packageData?.status ?? 'warning')}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              <div style={{ fontSize: 13, color: 'var(--text-primary)', fontWeight: 600 }}>
                {packageLabel}
              </div>
            <div style={{ fontSize: 12, color: 'var(--text-secondary)', lineHeight: 1.5 }}>
              {packageData?.description || t('apps.studio.workflowEmptyState')}
            </div>
            {workflows.length > 0 ? (
              <>
                <FactLabel>{t('apps.studio.workflowLabel')}</FactLabel>
                <select
                  value={selectedWorkflowKey}
                  onChange={(event) => onWorkflowChange(event.target.value)}
                  style={{
                    width: '100%',
                    padding: '9px 12px',
                    borderRadius: 12,
                    border: '1px solid var(--border)',
                    background: 'var(--bg)',
                    color: 'var(--text-primary)',
                    fontSize: 12,
                  }}
                >
                  {workflows.map((workflow) => (
                    <option key={workflow.key} value={workflow.key}>
                      {workflow.label}
                      {workflow.status ? ` · ${workflow.status}` : ''}
                    </option>
                  ))}
                </select>
                <button className="btn btn-secondary btn-sm" onClick={onRun} disabled={running}>
                  {running ? t('apps.studio.running') : t('apps.studio.run')}
                </button>
              </>
            ) : (
              <div style={{ fontSize: 12, color: 'var(--text-tertiary)' }}>{t('apps.studio.noWorkflow')}</div>
            )}
          </div>
        </SnapshotCard>
      </div>
    </section>
  )
}

function SnapshotCard({
  title,
  tone,
  children,
}: {
  title: string
  tone: { bg: string; fg: string }
  children: React.ReactNode
}) {
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: 12,
        padding: '14px 15px',
        borderRadius: 18,
        border: '1px solid var(--border)',
        background: 'var(--bg)',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 10 }}>
        <div style={{ fontSize: 13, fontWeight: 700 }}>{title}</div>
        <span style={{ padding: '4px 8px', borderRadius: 999, background: tone.bg, color: tone.fg, fontSize: 10, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.04em' }}>
          {title}
        </span>
      </div>
      {children}
    </div>
  )
}

function FactLabel({ children }: { children: React.ReactNode }) {
  return (
    <span style={{ fontSize: 11, color: 'var(--text-tertiary)', textTransform: 'uppercase', letterSpacing: '0.04em' }}>
      {children}
    </span>
  )
}

function HealthGrid({ rows }: { rows: Array<{ label: string; ok: boolean }> }) {
  return (
    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: 8 }}>
      {rows.map((row) => {
        const tone = boolTone(row.ok)
        return (
          <div
            key={row.label}
            style={{
              display: 'flex',
              flexDirection: 'column',
              gap: 6,
              padding: '10px 12px',
              borderRadius: 14,
              background: tone.bg,
            }}
          >
            <span style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.04em', color: tone.fg }}>{row.label}</span>
            <span style={{ fontSize: 12, color: tone.fg, fontWeight: 700 }}>{row.ok ? 'OK' : 'OFF'}</span>
          </div>
        )
      })}
    </div>
  )
}

function MetricGrid({ metrics }: { metrics: Array<{ label: string; value: number }> }) {
  return (
    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: 8 }}>
      {metrics.map((metric) => (
        <div
          key={metric.label}
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 4,
            padding: '10px 12px',
            borderRadius: 14,
            background: 'color-mix(in srgb, var(--bg-card) 30%, transparent)',
          }}
        >
          <span style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.04em', color: 'var(--text-tertiary)' }}>
            {metric.label}
          </span>
          <span style={{ fontSize: 16, fontWeight: 700 }}>{metric.value}</span>
        </div>
      ))}
    </div>
  )
}

function StatusMetricGrid({ metrics }: { metrics: Array<{ key: string; label: string; value: number }> }) {
  return (
    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, minmax(0, 1fr))', gap: 8 }}>
      {metrics.map((metric) => (
        <div
          key={metric.key}
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 4,
            padding: '10px 12px',
            borderRadius: 14,
            background: 'color-mix(in srgb, var(--bg-card) 30%, transparent)',
          }}
        >
          <span style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.04em', color: 'var(--text-tertiary)' }}>
            {metric.label}
          </span>
          <span
            data-testid={`studio-agent-status-count-${metric.key}`}
            style={{ fontSize: 16, fontWeight: 700 }}
          >
            {metric.value}
          </span>
        </div>
      ))}
    </div>
  )
}
