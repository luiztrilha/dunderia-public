import { useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  answerRequest,
  getOfficeMembers,
  requestRecommendation,
  reassignTask,
  updateTaskStatus,
  type InterviewOption,
  type OfficeMember,
  type Task,
  type TaskStatusAction,
} from '../../api/client'
import { formatRelativeTime } from '../../lib/format'
import { confirm } from '../ui/ConfirmDialog'
import { useAppStore } from '../../stores/app'

interface TaskDetailModalProps {
  task: Task
  onClose: () => void
}

const HUMAN_SLUG = 'human'

export function TaskDetailModal({ task, onClose }: TaskDetailModalProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const setCurrentApp = useAppStore((s) => s.setCurrentApp)
  const setCurrentChannel = useAppStore((s) => s.setCurrentChannel)
  const setActiveThreadId = useAppStore((s) => s.setActiveThreadId)
  const { data: memberData } = useQuery({
    queryKey: ['office-members'],
    queryFn: getOfficeMembers,
    staleTime: 30_000,
  })

  const currentOwner = (task.owner ?? '').trim()
  const currentStatus = (task.status ?? '').trim().toLowerCase()
  const [selectedOwner, setSelectedOwner] = useState<string>(currentOwner)
  const [submitting, setSubmitting] = useState(false)
  const [statusBusy, setStatusBusy] = useState<TaskStatusAction | null>(null)
  const [requestBusy, setRequestBusy] = useState(false)
  const [recommendationBusy, setRecommendationBusy] = useState(false)
  const [textMode, setTextMode] = useState<InterviewOption | null>(null)
  const [customText, setCustomText] = useState('')
  const [errorMsg, setErrorMsg] = useState<string | null>(null)

  useEffect(() => {
    setSelectedOwner((task.owner ?? '').trim())
    setTextMode(null)
    setCustomText('')
    setErrorMsg(null)
  }, [task.id, task.owner])

  useEffect(() => {
    function handleKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', handleKey)
    return () => document.removeEventListener('keydown', handleKey)
  }, [onClose])

  const assignableMembers = useMemo<OfficeMember[]>(() => {
    const members = memberData?.members ?? []
    return members.filter((m) => {
      const slug = m.slug?.trim().toLowerCase()
      return slug && slug !== 'human' && slug !== 'you'
    })
  }, [memberData])

  const isHumanAction = Boolean(task.awaiting_human) || (task.task_type ?? '') === 'human_action'
  const requestID = task.source_request_id?.trim() || ''
  const requestOptions = task.human_options ?? []
  const hasRecommendation = Boolean(task.recommendation_summary?.trim())
  const contextThreadID = task.thread_id?.trim() || task.source_message_id?.trim() || ''
  const humanActionPanelClass =
    'task-detail-human-action'
    + (task.awaiting_human ? ' task-detail-human-action-waiting' : '')
    + (currentStatus === 'blocked' ? ' task-detail-human-action-blocked' : '')

  async function runStatusAction(action: TaskStatusAction) {
    setStatusBusy(action)
    setErrorMsg(null)
    try {
      await updateTaskStatus(task.id, action, task.channel || 'general', HUMAN_SLUG)
      await queryClient.invalidateQueries({ queryKey: ['office-tasks'] })
      if (action === 'cancel' || action === 'complete') {
        onClose()
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : `${action} failed`
      setErrorMsg(message)
    } finally {
      setStatusBusy(null)
    }
  }

  function handleStatusAction(action: TaskStatusAction) {
    if (action === 'cancel') {
      confirm({
        title: t('apps.taskDetail.cancelTitle'),
        message: t('apps.taskDetail.cancelBody', { title: task.title || task.id }),
        confirmLabel: t('apps.taskDetail.cancelConfirm'),
        danger: true,
        onConfirm: () => runStatusAction(action),
      })
      return
    }
    void runStatusAction(action)
  }

  async function handleReassign() {
    const next = selectedOwner.trim()
    if (!next || next === currentOwner) return
    setSubmitting(true)
    setErrorMsg(null)
    try {
      await reassignTask(task.id, next, task.channel || 'general', HUMAN_SLUG)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['office-tasks'] }),
        queryClient.invalidateQueries({ queryKey: ['tasks'] }),
      ])
      onClose()
    } catch (err) {
      const message = err instanceof Error ? err.message : t('apps.taskDetail.reassignFailed')
      setErrorMsg(message)
    } finally {
      setSubmitting(false)
    }
  }

  function handleOverlayClick(e: React.MouseEvent<HTMLDivElement>) {
    if (e.target === e.currentTarget) onClose()
  }

  async function invalidateHumanActionViews() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['office-tasks'] }),
      queryClient.invalidateQueries({ queryKey: ['tasks'] }),
      queryClient.invalidateQueries({ queryKey: ['requests'] }),
      queryClient.invalidateQueries({ queryKey: ['deliveries'] }),
    ])
  }

  async function submitHumanAnswer(option: InterviewOption, text?: string) {
    if (!requestID || requestBusy) return
    setRequestBusy(true)
    setErrorMsg(null)
    try {
      await answerRequest(requestID, option.id, text)
      await invalidateHumanActionViews()
      onClose()
    } catch (err) {
      const message = err instanceof Error ? err.message : t('apps.taskDetail.humanAction.answerFailed')
      setErrorMsg(message)
    } finally {
      setRequestBusy(false)
    }
  }

  function handleHumanOption(option: InterviewOption) {
    if (option.requires_text) {
      setTextMode(option)
      setCustomText('')
      return
    }
    void submitHumanAnswer(option)
  }

  async function handleAskGameMaster() {
    if (!requestID || recommendationBusy) return
    setRecommendationBusy(true)
    setErrorMsg(null)
    try {
      await requestRecommendation(requestID, HUMAN_SLUG)
      await invalidateHumanActionViews()
    } catch (err) {
      const message = err instanceof Error ? err.message : t('apps.taskDetail.humanAction.recommendationFailed')
      setErrorMsg(message)
    } finally {
      setRecommendationBusy(false)
    }
  }

  function handleOpenContext() {
    setCurrentApp(null)
    setCurrentChannel(task.channel || 'general')
    if (contextThreadID) {
      setActiveThreadId(contextThreadID)
    }
    onClose()
  }

  const status = (task.status || '').replace(/_/g, ' ')
  const reviewState = (task.review_state || '').replace(/_/g, ' ')
  const description = task.description?.trim() || ''
  const details = task.details?.trim() || ''

  const metaRows: Array<[string, string | null | undefined]> = [
    [t('apps.taskDetail.meta.owner'), task.owner ? `@${task.owner}` : t('apps.taskDetail.unassigned')],
    [t('apps.taskDetail.meta.channel'), task.channel ? `#${task.channel}` : '—'],
    [t('apps.taskDetail.meta.status'), status || '—'],
    [t('apps.taskDetail.meta.reviewState'), reviewState || null],
    [t('apps.taskDetail.meta.taskType'), task.task_type || null],
    [t('apps.taskDetail.meta.executionMode'), task.execution_mode || null],
    [t('apps.taskDetail.meta.workspacePath'), task.workspace_path || null],
    [t('apps.taskDetail.meta.pipeline'), task.pipeline_id || null],
    [t('apps.taskDetail.meta.pipelineStage'), task.pipeline_stage || null],
    [t('apps.taskDetail.meta.worktreeBranch'), task.worktree_branch || null],
    [t('apps.taskDetail.meta.worktreePath'), task.worktree_path || null],
    [t('apps.taskDetail.meta.sourceSignal'), task.source_signal_id || null],
    [t('apps.taskDetail.meta.sourceDecision'), task.source_decision_id || null],
    [t('apps.taskDetail.meta.thread'), task.thread_id || null],
    [t('apps.taskDetail.meta.sourceRequest'), task.source_request_id || null],
    [t('apps.taskDetail.meta.sourceTask'), task.source_task_id || null],
    [t('apps.taskDetail.meta.sourceMessage'), task.source_message_id || null],
    [t('apps.taskDetail.meta.delivery'), task.delivery_id || null],
    [t('apps.taskDetail.meta.createdBy'), task.created_by ? `@${task.created_by}` : null],
    [t('apps.taskDetail.meta.created'), task.created_at ? formatRelativeTime(task.created_at) : null],
    [t('apps.taskDetail.meta.updated'), task.updated_at ? formatRelativeTime(task.updated_at) : null],
    [t('apps.taskDetail.meta.due'), task.due_at ? formatRelativeTime(task.due_at) : null],
    [t('apps.taskDetail.meta.followUp'), task.follow_up_at ? formatRelativeTime(task.follow_up_at) : null],
    [t('apps.taskDetail.meta.reminder'), task.reminder_at ? formatRelativeTime(task.reminder_at) : null],
    [t('apps.taskDetail.meta.recheck'), task.recheck_at ? formatRelativeTime(task.recheck_at) : null],
  ]

  const dependsOn = task.depends_on ?? []

  const ownerChanged = selectedOwner.trim() !== currentOwner && selectedOwner.trim() !== ''

  return (
    <div
      className="task-detail-overlay"
      onClick={handleOverlayClick}
      role="dialog"
      aria-modal="true"
      aria-label={`Task ${task.id}`}
    >
      <div className="task-detail-modal card">
        <header className="task-detail-header">
          <div>
            <div className="task-detail-id">#{task.id}</div>
            <h2 className="task-detail-title">{task.title || t('apps.taskDetail.untitled')}</h2>
          </div>
          <button
            type="button"
            className="task-detail-close"
            onClick={onClose}
            aria-label={t('apps.taskDetail.close')}
          >
            ×
          </button>
        </header>

        <section className="task-detail-section">
          <div className="task-detail-label">{t('apps.taskDetail.status')}</div>
          <div className="task-detail-status">
            <span className={`task-detail-status-badge status-${currentStatus || 'open'}`}>
              {currentStatus ? currentStatus.replace(/_/g, ' ') : 'open'}
            </span>
            {!isHumanAction && (
              <div className="task-detail-status-actions">
                <StatusButton
                  action="release"
                  label={t('apps.taskDetail.statusBtn.release')}
                  busy={statusBusy}
                  disabledFor={['open']}
                  currentStatus={currentStatus}
                  onClick={handleStatusAction}
                />
                <StatusButton
                  action="review"
                  label={t('apps.taskDetail.statusBtn.review')}
                  busy={statusBusy}
                  disabledFor={['review']}
                  currentStatus={currentStatus}
                  onClick={handleStatusAction}
                />
                <StatusButton
                  action="block"
                  label={t('apps.taskDetail.statusBtn.block')}
                  busy={statusBusy}
                  disabledFor={['blocked']}
                  currentStatus={currentStatus}
                  onClick={handleStatusAction}
                />
                <StatusButton
                  action="complete"
                  label={t('apps.taskDetail.statusBtn.complete')}
                  busy={statusBusy}
                  disabledFor={['done']}
                  currentStatus={currentStatus}
                  onClick={handleStatusAction}
                />
                <StatusButton
                  action="cancel"
                  label={t('apps.taskDetail.statusBtn.cancel')}
                  busy={statusBusy}
                  disabledFor={['canceled', 'cancelled']}
                  currentStatus={currentStatus}
                  onClick={handleStatusAction}
                  danger
                />
              </div>
            )}
          </div>
        </section>

        {!isHumanAction && (
          <section className="task-detail-section">
            <div className="task-detail-label">{t('apps.taskDetail.ownership')}</div>
            <div className="task-detail-ownership">
              <div className="task-detail-owner-current">
                <span className="task-detail-owner-badge">
                  {task.owner ? `@${task.owner}` : t('apps.taskDetail.unassigned')}
                </span>
                <span className="task-detail-hint">
                  {t('apps.taskDetail.reassignHint', { channel: task.channel || 'general' })}
                </span>
              </div>
              <div className="task-detail-owner-controls">
                <select
                  className="task-detail-select"
                  value={selectedOwner}
                  onChange={(e) => setSelectedOwner(e.target.value)}
                  disabled={submitting}
                >
                  <option value="">{t('apps.taskDetail.pickOwner')}</option>
                  {assignableMembers.map((m) => (
                    <option key={m.slug} value={m.slug}>
                      {m.name ? `${m.name} — @${m.slug}` : `@${m.slug}`}
                    </option>
                  ))}
                </select>
                <button
                  type="button"
                  className="btn btn-primary btn-sm"
                  onClick={handleReassign}
                  disabled={!ownerChanged || submitting}
                >
                  {submitting ? t('apps.taskDetail.reassigning') : t('apps.taskDetail.reassign')}
                </button>
              </div>
              {errorMsg && <div className="task-detail-error">{errorMsg}</div>}
            </div>
          </section>
        )}

        {isHumanAction && (
          <section className={`task-detail-section ${humanActionPanelClass}`}>
            <div className="task-detail-label">{t('apps.taskDetail.humanAction.title')}</div>
            <div style={{ display: 'grid', gap: 12 }}>
              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                <span className="badge badge-waiting">{t('apps.taskDetail.humanAction.title')}</span>
                {currentStatus === 'blocked' ? (
                  <span className="badge badge-attention">{t('apps.taskDetail.statusBtn.block')}</span>
                ) : null}
              </div>

              <div style={{ fontSize: 13, color: 'var(--text-secondary)', lineHeight: 1.5 }}>
                {task.awaiting_human_reason || t('apps.taskDetail.humanAction.subtitle')}
              </div>

              {typeof task.progress_percent === 'number' && (
                <div className="app-card-meta">
                  {t('apps.taskDetail.humanAction.progress', {
                    percent: task.progress_percent,
                    basis: task.progress_basis || t('apps.taskDetail.humanAction.noProgressBasis'),
                  })}
                </div>
              )}

              {task.awaiting_human_since && (
                <div className="app-card-meta">
                  {t('apps.taskDetail.humanAction.waitingSince', {
                    time: formatRelativeTime(task.awaiting_human_since),
                  })}
                </div>
              )}

              {hasRecommendation && (
                <div className="task-detail-human-note">
                  <div style={{ fontSize: 12, fontWeight: 700, marginBottom: 6 }}>
                    {t('apps.taskDetail.humanAction.recommendationReady')}
                  </div>
                  <div style={{ fontSize: 13, color: 'var(--text-secondary)', lineHeight: 1.5 }}>
                    {task.recommendation_summary}
                  </div>
                </div>
              )}

              {!hasRecommendation && task.recommendation_status === 'requested' && (
                <div className="app-card-meta">{t('apps.taskDetail.humanAction.recommendationRequested')}</div>
              )}

              {textMode ? (
                <div style={{ display: 'grid', gap: 8 }}>
                  <textarea
                    className="task-detail-select"
                    rows={4}
                    value={customText}
                    placeholder={textMode.text_hint || t('apps.taskDetail.humanAction.textPlaceholder')}
                    onChange={(e) => setCustomText(e.target.value)}
                  />
                  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                    <button
                      type="button"
                      className="btn btn-ghost btn-sm"
                      onClick={() => setTextMode(null)}
                      disabled={requestBusy}
                    >
                      {t('apps.taskDetail.humanAction.back')}
                    </button>
                    <button
                      type="button"
                      className="btn btn-primary btn-sm"
                      onClick={() => void submitHumanAnswer(textMode, customText.trim())}
                      disabled={requestBusy || !customText.trim()}
                    >
                      {requestBusy
                        ? t('apps.taskDetail.humanAction.answering')
                        : t('apps.taskDetail.humanAction.answerAs', { label: textMode.label })}
                    </button>
                  </div>
                </div>
              ) : requestOptions.length > 0 ? (
                <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                  {requestOptions.map((option) => (
                    <button
                      key={option.id}
                      type="button"
                      className={`btn btn-sm ${option.id === task.human_recommended_id ? 'btn-primary' : 'btn-ghost'}`}
                      onClick={() => handleHumanOption(option)}
                      disabled={requestBusy}
                      title={option.description}
                    >
                      {option.label}
                      {option.requires_text ? ` · ${t('apps.taskDetail.humanAction.typeHint')}` : ''}
                    </button>
                  ))}
                </div>
              ) : (
                <div className="app-card-meta">{t('apps.taskDetail.humanAction.noOptions')}</div>
              )}

              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                {requestID && (
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    onClick={() => void handleAskGameMaster()}
                    disabled={recommendationBusy}
                  >
                    {recommendationBusy
                      ? t('apps.taskDetail.humanAction.askingGameMaster')
                      : t('apps.taskDetail.humanAction.askGameMaster')}
                  </button>
                )}
                <button type="button" className="btn btn-ghost btn-sm" onClick={handleOpenContext}>
                  {t('apps.taskDetail.humanAction.openContext')}
                </button>
              </div>

              {errorMsg && <div className="task-detail-error">{errorMsg}</div>}
            </div>
          </section>
        )}

        {(description || details) && (
          <section className="task-detail-section">
            {description && (
              <>
                <div className="task-detail-label">{t('apps.taskDetail.description')}</div>
                <div className="task-detail-body">{description}</div>
              </>
            )}
            {details && (
              <>
                <div className="task-detail-label" style={{ marginTop: description ? 12 : 0 }}>
                  {t('apps.taskDetail.details')}
                </div>
                <div className="task-detail-body">{details}</div>
              </>
            )}
          </section>
        )}

        {dependsOn.length > 0 && (
          <section className="task-detail-section">
            <div className="task-detail-label">{t('apps.taskDetail.dependsOn')}</div>
            <ul className="task-detail-deps">
              {dependsOn.map((dep) => (
                <li key={dep}>#{dep}</li>
              ))}
            </ul>
          </section>
        )}

        <section className="task-detail-section">
          <div className="task-detail-label">{t('apps.taskDetail.metadata')}</div>
          <dl className="task-detail-meta">
            {metaRows
              .filter(([, value]) => value != null && value !== '')
              .map(([key, value]) => (
                <div key={key} className="task-detail-meta-row">
                  <dt>{key}</dt>
                  <dd>{value}</dd>
                </div>
              ))}
          </dl>
        </section>
      </div>
    </div>
  )
}

interface StatusButtonProps {
  action: TaskStatusAction
  label: string
  busy: TaskStatusAction | null
  disabledFor: string[]
  currentStatus: string
  onClick: (action: TaskStatusAction) => void
  danger?: boolean
}

function StatusButton({
  action,
  label,
  busy,
  disabledFor,
  currentStatus,
  onClick,
  danger,
}: StatusButtonProps) {
  const { t } = useTranslation()
  const isCurrent = disabledFor.includes(currentStatus)
  const isBusy = busy === action
  const anyBusy = busy !== null
  const className = 'btn btn-sm ' + (danger ? 'btn-ghost task-detail-status-btn-danger' : 'btn-ghost')
  return (
    <button
      type="button"
      className={className}
      onClick={() => onClick(action)}
      disabled={isCurrent || anyBusy}
      title={isCurrent ? t('apps.taskDetail.statusBtn.alreadyInState') : undefined}
    >
      {isBusy ? '...' : label}
    </button>
  )
}
