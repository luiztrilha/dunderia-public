import { useState, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getAgentLogs, type AgentLog } from '../../api/client'
import { formatRelativeTime, formatTokens, formatUSD } from '../../lib/format'
import { useBrokerRefetchInterval } from '../../hooks/useBrokerEvents'

export function ReceiptsApp() {
  const [selectedTask, setSelectedTask] = useState<string | null>(null)

  if (selectedTask) {
    return <ReceiptDetail taskId={selectedTask} onBack={() => setSelectedTask(null)} />
  }

  return <ReceiptList onSelectTask={setSelectedTask} />
}

function ReceiptList({ onSelectTask }: { onSelectTask: (taskId: string) => void }) {
  const { t } = useTranslation()
  const refetchInterval = useBrokerRefetchInterval(10_000)
  const { data, isLoading, error } = useQuery({
    queryKey: ['agent-logs'],
    queryFn: () => getAgentLogs({ limit: 100 }),
    refetchInterval,
  })

  return (
    <>
      <div style={{ padding: '16px 20px', borderBottom: '1px solid var(--border)' }}>
        <h3 style={{ fontSize: 16, fontWeight: 600 }}>{t('apps.receipts.title')}</h3>
        <div style={{ fontSize: 12, color: 'var(--text-tertiary)', marginTop: 4 }}>
          {t('apps.receipts.subtitle')}
        </div>
      </div>

      {isLoading && (
        <div style={{ padding: 20, color: 'var(--text-tertiary)' }}>{t('apps.receipts.loading')}</div>
      )}

      {error && (
        <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
          {t('apps.receipts.loadFailed')}
        </div>
      )}

      {!isLoading && !error && <LogTable logs={data?.logs ?? []} onSelectTask={onSelectTask} />}
    </>
  )
}

function LogTable({ logs, onSelectTask }: { logs: AgentLog[]; onSelectTask: (taskId: string) => void }) {
  const { t } = useTranslation()
  if (logs.length === 0) {
    return (
      <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
        {t('apps.receipts.empty')}
      </div>
    )
  }

  return (
    <div style={{ overflow: 'auto', flex: 1 }}>
      <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
        <thead>
          <tr style={{ textAlign: 'left', color: 'var(--text-tertiary)', fontSize: 11, textTransform: 'uppercase' }}>
            <th style={{ padding: '8px 20px' }}>{t('apps.receipts.table.agent')}</th>
            <th style={{ padding: '8px 12px' }}>{t('apps.receipts.table.action')}</th>
            <th style={{ padding: '8px 12px' }}>{t('apps.receipts.table.time')}</th>
            <th style={{ padding: '8px 12px', textAlign: 'right' }}>{t('apps.receipts.table.tokens')}</th>
            <th style={{ padding: '8px 12px', textAlign: 'right' }}>{t('apps.receipts.table.cost')}</th>
          </tr>
        </thead>
        <tbody>
          {logs.map((log) => {
            const totalTokens = log.usage?.total_tokens ?? 0
            const cost = log.usage?.cost_usd ?? 0
            return (
              <tr
                key={log.id}
                style={{ borderTop: '1px solid var(--border-light)', cursor: log.task ? 'pointer' : 'default' }}
                onClick={() => log.task && onSelectTask(log.task)}
              >
                <td style={{ padding: '10px 20px', fontWeight: 500 }}>{log.agent || '\u2014'}</td>
                <td style={{ padding: '10px 12px', color: 'var(--text-secondary)' }}>{log.action || log.content?.slice(0, 60) || '\u2014'}</td>
                <td style={{ padding: '10px 12px', color: 'var(--text-secondary)' }}>
                  {log.timestamp ? formatRelativeTime(log.timestamp) : '\u2014'}
                </td>
                <td style={{ padding: '10px 12px', textAlign: 'right', fontFamily: 'var(--font-mono)', fontSize: 12 }}>
                  {totalTokens > 0 ? formatTokens(totalTokens) : '\u2014'}
                </td>
                <td style={{ padding: '10px 12px', textAlign: 'right', fontFamily: 'var(--font-mono)', fontSize: 12 }}>
                  {cost > 0 ? formatUSD(cost) : '\u2014'}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

function ReceiptDetail({ taskId, onBack }: { taskId: string; onBack: () => void }) {
  const { t } = useTranslation()
  const refetchInterval = useBrokerRefetchInterval(10_000)
  const { data, isLoading, error } = useQuery({
    queryKey: ['agent-logs', taskId],
    queryFn: () => getAgentLogs({ task: taskId }),
    refetchInterval,
  })

  const logs = data?.logs ?? []

  return (
    <>
      <button
        className="btn btn-secondary btn-sm"
        style={{ margin: '12px 20px 0' }}
        onClick={onBack}
      >
        {t('apps.receipts.detail.back')}
      </button>

      <div style={{ padding: '12px 20px 8px' }}>
        <h3 style={{ fontSize: 15, fontWeight: 600, fontFamily: 'var(--font-mono)' }}>{taskId}</h3>
        <div style={{ fontSize: 12, color: 'var(--text-tertiary)', marginTop: 4 }}>
          {t('apps.receipts.detail.subtitle')}
        </div>
      </div>

      {isLoading && (
        <div style={{ padding: '16px 20px', color: 'var(--text-tertiary)' }}>{t('apps.receipts.detail.loading')}</div>
      )}

      {error && (
        <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
          {t('apps.receipts.detail.loadFailed')}
        </div>
      )}

      {!isLoading && !error && logs.length === 0 && (
        <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
          {t('apps.receipts.detail.empty')}
        </div>
      )}

      {!isLoading && !error && logs.length > 0 && (
        <div style={{ overflow: 'auto', flex: 1, padding: '0 20px 20px' }}>
          {logs.map((entry, i) => (
            <div
              key={entry.id}
              style={{
                padding: '10px 0',
                borderBottom: '1px solid var(--border-light)',
                fontSize: 13,
                display: 'flex',
                flexDirection: 'column',
                gap: 4,
              }}
            >
              <div style={{ display: 'flex', gap: 12, alignItems: 'baseline' }}>
                <span style={{ color: 'var(--text-tertiary)', fontSize: 11, minWidth: 64 }}>
                  #{i + 1} {entry.timestamp ? new Date(entry.timestamp).toLocaleTimeString() : '\u2014'}
                </span>
                <span style={{ fontWeight: 600, fontFamily: 'var(--font-mono)' }}>
                  {entry.action || t('apps.receipts.detail.unknown')}
                </span>
                {entry.agent && (
                  <span style={{ fontSize: 11, color: 'var(--text-secondary)' }}>@{entry.agent}</span>
                )}
              </div>
              {entry.content && (
                <div style={{ fontSize: 12, color: 'var(--text-secondary)', paddingLeft: 76 }}>
                  {entry.content.slice(0, 200)}
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </>
  )
}
