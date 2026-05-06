import { useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  addTaskArtifact,
  addTaskFeedback,
  answerRequest,
  approveTaskPlan,
  getLearningCandidateDiffPreview,
  getOfficeMembers,
  promoteTaskLearning,
  recordTaskAttempt,
  requestRecommendation,
  reassignTask,
  saveTaskPlanRevision,
  updateTaskInboxState,
  updateTaskLimits,
  updateTaskOutcome,
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
  const [outcomeDraft, setOutcomeDraft] = useState(task.outcome ?? '')
  const [evidenceDraft, setEvidenceDraft] = useState(task.outcome_evidence ?? '')
  const [outcomeBusy, setOutcomeBusy] = useState(false)
  const [artifactKind, setArtifactKind] = useState('document')
  const [artifactTitle, setArtifactTitle] = useState('')
  const [artifactTarget, setArtifactTarget] = useState('')
  const [artifactSummary, setArtifactSummary] = useState('')
  const [browserPageURL, setBrowserPageURL] = useState('')
  const [browserSelector, setBrowserSelector] = useState('')
  const [browserElementText, setBrowserElementText] = useState('')
  const [browserViewportWidth, setBrowserViewportWidth] = useState('')
  const [browserViewportHeight, setBrowserViewportHeight] = useState('')
  const [artifactBusy, setArtifactBusy] = useState(false)
  const latestPlan = useMemo(() => {
    const revisions = task.plan_revisions ?? []
    return revisions.reduce<(typeof revisions)[number] | null>((latest, revision) => {
      if (!latest) return revision
      return (revision.version ?? 0) > (latest.version ?? 0) ? revision : latest
    }, null)
  }, [task.plan_revisions])
  const [planDraft, setPlanDraft] = useState(latestPlan?.content ?? '')
  const [planSummaryDraft, setPlanSummaryDraft] = useState('')
  const [planBusy, setPlanBusy] = useState(false)
  const [planApprovalBusy, setPlanApprovalBusy] = useState(false)
  const [showLearningDiff, setShowLearningDiff] = useState(false)
  const [learningBusy, setLearningBusy] = useState(false)
  const [limitAttempts, setLimitAttempts] = useState(String(task.limits?.max_attempts ?? ''))
  const [limitRuntime, setLimitRuntime] = useState(String(task.limits?.max_runtime_minutes ?? ''))
  const [limitCost, setLimitCost] = useState(String(task.limits?.max_cost_cents ?? ''))
  const [limitsBusy, setLimitsBusy] = useState(false)
  const [feedbackRating, setFeedbackRating] = useState<'up' | 'neutral' | 'down'>('up')
  const [feedbackComment, setFeedbackComment] = useState('')
  const [feedbackBusy, setFeedbackBusy] = useState(false)
  const [inboxBusy, setInboxBusy] = useState(false)
  const [textMode, setTextMode] = useState<InterviewOption | null>(null)
  const [customText, setCustomText] = useState('')
  const [errorMsg, setErrorMsg] = useState<string | null>(null)

  useEffect(() => {
    setSelectedOwner((task.owner ?? '').trim())
    setTextMode(null)
    setCustomText('')
    setOutcomeDraft(task.outcome ?? '')
    setEvidenceDraft(task.outcome_evidence ?? '')
    setArtifactKind('document')
    setArtifactTitle('')
    setArtifactTarget('')
    setArtifactSummary('')
    setBrowserPageURL('')
    setBrowserSelector('')
    setBrowserElementText('')
    setBrowserViewportWidth('')
    setBrowserViewportHeight('')
    setPlanDraft(latestPlan?.content ?? '')
    setPlanSummaryDraft('')
    setShowLearningDiff(false)
    setLimitAttempts(String(task.limits?.max_attempts ?? ''))
    setLimitRuntime(String(task.limits?.max_runtime_minutes ?? ''))
    setLimitCost(String(task.limits?.max_cost_cents ?? ''))
    setFeedbackRating('up')
    setFeedbackComment('')
    setErrorMsg(null)
  }, [task.id, task.owner, task.outcome, task.outcome_evidence, latestPlan?.content, task.limits?.max_attempts, task.limits?.max_runtime_minutes, task.limits?.max_cost_cents])

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
  const learningCandidate = task.learning_candidate
  const requestID = task.source_request_id?.trim() || ''
  const requestOptions = task.human_options ?? []
  const hasRecommendation = Boolean(task.recommendation_summary?.trim())
  const contextThreadID = task.thread_id?.trim() || task.source_message_id?.trim() || ''
  const humanActionPanelClass =
    'task-detail-human-action'
    + (task.awaiting_human ? ' task-detail-human-action-waiting' : '')
    + (currentStatus === 'blocked' ? ' task-detail-human-action-blocked' : '')
  const { data: learningDiff, isFetching: learningDiffFetching, error: learningDiffError } = useQuery({
    queryKey: ['learning-candidate-diff', task.id, task.channel || 'general'],
    queryFn: () => getLearningCandidateDiffPreview(task.id, task.channel || 'general', HUMAN_SLUG),
    enabled: showLearningDiff && Boolean(task.id),
    staleTime: 10_000,
  })

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

  async function handleSaveOutcome() {
    const outcome = outcomeDraft.trim()
    const evidence = evidenceDraft.trim()
    if (!outcome && !evidence) return
    setOutcomeBusy(true)
    setErrorMsg(null)
    try {
      await updateTaskOutcome(task.id, task.channel || 'general', outcome, evidence, HUMAN_SLUG)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['office-tasks'] }),
        queryClient.invalidateQueries({ queryKey: ['tasks'] }),
        queryClient.invalidateQueries({ queryKey: ['studio-dev-console'] }),
      ])
    } catch (err) {
      const message = err instanceof Error ? err.message : t('apps.taskDetail.outcome.saveFailed')
      setErrorMsg(message)
    } finally {
      setOutcomeBusy(false)
    }
  }

  async function handleAddArtifact() {
    const target = artifactTarget.trim()
    const summary = artifactSummary.trim()
    const pageURL = browserPageURL.trim()
    const selector = browserSelector.trim()
    const elementText = browserElementText.trim()
    const isBrowserInspection = artifactKind === 'browser_inspection'
    if (!target && !summary && (!isBrowserInspection || (!pageURL && !selector && !elementText))) return
    setArtifactBusy(true)
    setErrorMsg(null)
    try {
      const isURL = /^https?:\/\//i.test(target)
      const viewportWidth = Number(browserViewportWidth) || undefined
      const viewportHeight = Number(browserViewportHeight) || undefined
      await addTaskArtifact(
        task.id,
        task.channel || 'general',
        {
          kind: artifactKind.trim() || 'document',
          result_role: isBrowserInspection ? 'evidence' : undefined,
          title: artifactTitle.trim() || (isBrowserInspection ? t('apps.taskDetail.artifacts.browser.defaultTitle') : ''),
          summary,
          path: isBrowserInspection ? target : (isURL ? '' : target),
          url: isBrowserInspection ? pageURL : (isURL ? target : ''),
          preview_url: isBrowserInspection ? pageURL : '',
          state: isBrowserInspection ? 'verified' : 'active',
          browser_inspection: isBrowserInspection
            ? {
                page_url: pageURL,
                selector,
                element_text: elementText,
                screenshot_path: target,
                viewport_width: viewportWidth,
                viewport_height: viewportHeight,
                notes: summary,
              }
            : undefined,
        },
        HUMAN_SLUG,
      )
      setArtifactTitle('')
      setArtifactTarget('')
      setArtifactSummary('')
      setBrowserPageURL('')
      setBrowserSelector('')
      setBrowserElementText('')
      setBrowserViewportWidth('')
      setBrowserViewportHeight('')
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['office-tasks'] }),
        queryClient.invalidateQueries({ queryKey: ['tasks'] }),
        queryClient.invalidateQueries({ queryKey: ['deliveries'] }),
        queryClient.invalidateQueries({ queryKey: ['studio-dev-console'] }),
      ])
    } catch (err) {
      const message = err instanceof Error ? err.message : t('apps.taskDetail.artifacts.addFailed')
      setErrorMsg(message)
    } finally {
      setArtifactBusy(false)
    }
  }

  async function handleSavePlanRevision() {
    const content = planDraft.trim()
    if (!content) return
    setPlanBusy(true)
    setErrorMsg(null)
    try {
      await saveTaskPlanRevision(task.id, task.channel || 'general', content, planSummaryDraft.trim(), HUMAN_SLUG)
      setPlanSummaryDraft('')
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['office-tasks'] }),
        queryClient.invalidateQueries({ queryKey: ['tasks'] }),
        queryClient.invalidateQueries({ queryKey: ['studio-dev-console'] }),
      ])
    } catch (err) {
      const message = err instanceof Error ? err.message : t('apps.taskDetail.plan.saveFailed')
      setErrorMsg(message)
    } finally {
      setPlanBusy(false)
    }
  }

  async function refreshTaskViews() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['office-tasks'] }),
      queryClient.invalidateQueries({ queryKey: ['tasks'] }),
      queryClient.invalidateQueries({ queryKey: ['deliveries'] }),
      queryClient.invalidateQueries({ queryKey: ['studio-dev-console'] }),
    ])
  }

  async function handleApprovePlan() {
    setPlanApprovalBusy(true)
    setErrorMsg(null)
    try {
      await approveTaskPlan(task.id, task.channel || 'general', HUMAN_SLUG)
      await refreshTaskViews()
    } catch (err) {
      const message = err instanceof Error ? err.message : t('apps.taskDetail.plan.approveFailed')
      setErrorMsg(message)
    } finally {
      setPlanApprovalBusy(false)
    }
  }

  async function handlePromoteLearning() {
    setLearningBusy(true)
    setErrorMsg(null)
    try {
      await promoteTaskLearning(task.id, task.channel || 'general', HUMAN_SLUG)
      await Promise.all([
        refreshTaskViews(),
        queryClient.invalidateQueries({ queryKey: ['skills'] }),
      ])
    } catch (err) {
      const message = err instanceof Error ? err.message : t('apps.taskDetail.learning.promoteFailed')
      setErrorMsg(message)
    } finally {
      setLearningBusy(false)
    }
  }

  async function handleInboxAction(action: 'mark_read' | 'archive_inbox') {
    setInboxBusy(true)
    setErrorMsg(null)
    try {
      await updateTaskInboxState(task.id, task.channel || 'general', action, HUMAN_SLUG)
      await refreshTaskViews()
      if (action === 'archive_inbox') onClose()
    } catch (err) {
      const message = err instanceof Error ? err.message : t('apps.taskDetail.inbox.actionFailed')
      setErrorMsg(message)
    } finally {
      setInboxBusy(false)
    }
  }

  async function handleSaveLimits() {
    setLimitsBusy(true)
    setErrorMsg(null)
    try {
      await updateTaskLimits(task.id, task.channel || 'general', {
        max_attempts: Number(limitAttempts) || undefined,
        max_runtime_minutes: Number(limitRuntime) || undefined,
        max_cost_cents: Number(limitCost) || undefined,
      }, HUMAN_SLUG)
      await refreshTaskViews()
    } catch (err) {
      const message = err instanceof Error ? err.message : t('apps.taskDetail.limits.saveFailed')
      setErrorMsg(message)
    } finally {
      setLimitsBusy(false)
    }
  }

  async function handleRecordAttempt() {
    setLimitsBusy(true)
    setErrorMsg(null)
    try {
      await recordTaskAttempt(task.id, task.channel || 'general', HUMAN_SLUG)
      await refreshTaskViews()
    } catch (err) {
      const message = err instanceof Error ? err.message : t('apps.taskDetail.limits.attemptFailed')
      setErrorMsg(message)
    } finally {
      setLimitsBusy(false)
    }
  }

  async function handleAddFeedback() {
    setFeedbackBusy(true)
    setErrorMsg(null)
    try {
      await addTaskFeedback(task.id, task.channel || 'general', feedbackRating, feedbackComment.trim(), HUMAN_SLUG)
      setFeedbackComment('')
      await refreshTaskViews()
    } catch (err) {
      const message = err instanceof Error ? err.message : t('apps.taskDetail.feedback.saveFailed')
      setErrorMsg(message)
    } finally {
      setFeedbackBusy(false)
    }
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
  const completionBlocked = Boolean(task.completion_evidence_required && !task.completion_evidence_satisfied)
  const completionBlocker = task.completion_blocker || t('apps.taskDetail.completion.blockedCopy')
  const queuePriority = task.queue_priority || 'normal'
  const planStatus = task.plan_status || latestPlan?.status || (task.plan_required ? 'missing' : 'not_required')
  const livenessHistory = task.liveness_history ?? []
  const isBrowserInspectionArtifact = artifactKind === 'browser_inspection'

  const metaRows: Array<[string, string | null | undefined]> = [
    [t('apps.taskDetail.meta.owner'), task.owner ? `@${task.owner}` : t('apps.taskDetail.unassigned')],
    [t('apps.taskDetail.meta.channel'), task.channel ? `#${task.channel}` : '—'],
    [t('apps.taskDetail.meta.status'), status || '—'],
    [t('apps.taskDetail.meta.reviewState'), reviewState || null],
    [t('apps.taskDetail.meta.queue'), task.queue_key || null],
    [t('apps.taskDetail.meta.outcomeStatus'), task.outcome_status || null],
    [t('apps.taskDetail.meta.liveness'), task.liveness_state || null],
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
                  disabled={completionBlocked}
                  disabledTitle={completionBlocker}
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
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 10 }}>
            <button
              type="button"
              className="btn btn-ghost btn-sm"
              onClick={() => void handleInboxAction('mark_read')}
              disabled={inboxBusy || Boolean(task.read_at)}
            >
              {task.read_at ? t('apps.taskDetail.inbox.read') : t('apps.taskDetail.inbox.markRead')}
            </button>
            <button
              type="button"
              className="btn btn-ghost btn-sm"
              onClick={() => void handleInboxAction('archive_inbox')}
              disabled={inboxBusy || Boolean(task.archived_at)}
            >
              {task.archived_at ? t('apps.taskDetail.inbox.archived') : t('apps.taskDetail.inbox.archive')}
            </button>
          </div>
          {!isHumanAction && task.completion_evidence_required && (
            <div className={`task-detail-completion-contract${completionBlocked ? ' task-detail-completion-contract-blocked' : ''}`}>
              <div className="task-detail-completion-contract-title">
                {completionBlocked ? t('apps.taskDetail.completion.blockedTitle') : t('apps.taskDetail.completion.readyTitle')}
              </div>
              <div className="task-detail-completion-contract-copy">
                {completionBlocked ? completionBlocker : t('apps.taskDetail.completion.readyCopy')}
              </div>
            </div>
          )}
          {!isHumanAction && task.queue_key && (
            <div className="task-detail-queue-contract">
              <div>
                <div className="task-detail-completion-contract-title">{task.queue_label || task.queue_key}</div>
                <div className="task-detail-completion-contract-copy">{task.queue_reason || t('apps.taskDetail.queue.defaultReason')}</div>
              </div>
              <div className="task-detail-queue-contract-meta">
                <span className={`badge ${queuePriority === 'high' ? 'badge-attention' : queuePriority === 'medium' ? 'badge-yellow' : 'badge-accent'}`}>
                  {t(`apps.taskDetail.queue.priority.${queuePriority}`, { defaultValue: queuePriority })}
                </span>
                {task.queue_sla_at && (
                  <span className="app-card-meta">{t('apps.taskDetail.queue.sla', { time: formatRelativeTime(task.queue_sla_at) })}</span>
                )}
              </div>
            </div>
          )}
        </section>

        {livenessHistory.length > 0 && (
          <section className="task-detail-section">
            <div className="task-detail-label">{t('apps.taskDetail.liveness.title')}</div>
            <div className="task-detail-liveness-list">
              {livenessHistory.slice(0, 5).map((item) => {
                const state = item.state || 'unknown'
                const key = `${item.created_at || ''}-${item.actor || ''}-${state}-${item.reason || ''}`
                return (
                  <div key={key} className="task-detail-liveness-item">
                    <span className={`task-detail-status-badge status-${state}`}>
                      {t(`apps.taskDetail.liveness.states.${state}`, { defaultValue: state.replace(/_/g, ' ') })}
                    </span>
                    <div>
                      <div className="task-detail-completion-contract-title">
                        {item.reason || t('apps.taskDetail.liveness.noReason')}
                      </div>
                      <div className="app-card-meta">
                        {item.actor ? `@${item.actor}` : t('apps.taskDetail.liveness.runtime')}
                        {item.created_at ? ` · ${formatRelativeTime(item.created_at)}` : ''}
                      </div>
                    </div>
                  </div>
                )
              })}
            </div>
          </section>
        )}

        {!isHumanAction && (task.goal_path ?? []).length > 0 && (
          <section className="task-detail-section">
            <div className="task-detail-label">{t('apps.taskDetail.goalPath.title')}</div>
            <ol className="task-detail-goal-path">
              {(task.goal_path ?? []).map((item, index) => (
                <li key={`${item}-${index}`}>{item}</li>
              ))}
            </ol>
          </section>
        )}

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

        {!isHumanAction && (
          <section className="task-detail-section">
            <div className="task-detail-label">{t('apps.taskDetail.limits.title')}</div>
            <div style={{ display: 'grid', gap: 10 }}>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(100px, 1fr))', gap: 8 }}>
                <input
                  className="task-detail-select"
                  value={limitAttempts}
                  inputMode="numeric"
                  placeholder={t('apps.taskDetail.limits.maxAttempts')}
                  onChange={(e) => setLimitAttempts(e.target.value)}
                />
                <input
                  className="task-detail-select"
                  value={limitRuntime}
                  inputMode="numeric"
                  placeholder={t('apps.taskDetail.limits.maxRuntime')}
                  onChange={(e) => setLimitRuntime(e.target.value)}
                />
                <input
                  className="task-detail-select"
                  value={limitCost}
                  inputMode="numeric"
                  placeholder={t('apps.taskDetail.limits.maxCost')}
                  onChange={(e) => setLimitCost(e.target.value)}
                />
              </div>
              <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
                <span className={`task-detail-status-badge status-${task.limits?.limit_state || 'ok'}`}>
                  {task.limits?.limit_state || 'ok'}
                </span>
                <span className="app-card-meta">
                  {t('apps.taskDetail.limits.attempts', {
                    used: task.limits?.attempts_used ?? 0,
                    max: task.limits?.max_attempts ?? '∞',
                  })}
                </span>
                {(task.limits?.runtime_ms_used || task.limits?.cost_cents_used) ? (
                  <span className="app-card-meta">
                    {[
                      task.limits?.runtime_ms_used ? t('apps.taskDetail.limits.runtimeUsed', { value: formatRuntimeMinutes(task.limits.runtime_ms_used) }) : '',
                      task.limits?.cost_cents_used ? t('apps.taskDetail.limits.costUsed', { value: formatCostCents(task.limits.cost_cents_used) }) : '',
                    ].filter(Boolean).join(' · ')}
                  </span>
                ) : null}
                <button type="button" className="btn btn-primary btn-sm" onClick={() => void handleSaveLimits()} disabled={limitsBusy}>
                  {limitsBusy ? t('apps.taskDetail.limits.saving') : t('apps.taskDetail.limits.save')}
                </button>
                <button type="button" className="btn btn-ghost btn-sm" onClick={() => void handleRecordAttempt()} disabled={limitsBusy}>
                  {t('apps.taskDetail.limits.recordAttempt')}
                </button>
              </div>
              {task.limits?.last_limit_reason && <div className="app-card-meta">{task.limits.last_limit_reason}</div>}
              {errorMsg && <div className="task-detail-error">{errorMsg}</div>}
            </div>
          </section>
        )}

        {!isHumanAction && (task.evals ?? []).length > 0 && (
          <section className="task-detail-section">
            <div className="task-detail-label">{t('apps.taskDetail.evals.title')}</div>
            <div style={{ display: 'grid', gap: 8 }}>
              {(task.evals ?? []).map((evalSignal) => (
                <div key={evalSignal.id || evalSignal.kind} className="task-detail-human-note">
                  <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap', marginBottom: 4 }}>
                    <span className={`badge ${evalSignal.severity === 'error' ? 'badge-attention' : evalSignal.severity === 'warning' ? 'badge-yellow' : 'badge-accent'}`}>
                      {evalSignal.severity || 'info'}
                    </span>
                    <span style={{ fontSize: 12, fontWeight: 700 }}>{evalSignal.kind?.replace(/_/g, ' ')}</span>
                  </div>
                  <div style={{ fontSize: 12, color: 'var(--text-secondary)', lineHeight: 1.45 }}>{evalSignal.summary}</div>
                </div>
              ))}
            </div>
          </section>
        )}

        {!isHumanAction && (
          <section className="task-detail-section">
            <div className="task-detail-label">{t('apps.taskDetail.outcome.title')}</div>
            <div style={{ display: 'grid', gap: 10 }}>
              <textarea
                className="task-detail-select"
                rows={3}
                value={outcomeDraft}
                placeholder={t('apps.taskDetail.outcome.placeholder')}
                onChange={(e) => setOutcomeDraft(e.target.value)}
                style={{ height: 'auto', minHeight: 76, paddingTop: 8, resize: 'vertical' }}
              />
              <textarea
                className="task-detail-select"
                rows={3}
                value={evidenceDraft}
                placeholder={t('apps.taskDetail.outcome.evidencePlaceholder')}
                onChange={(e) => setEvidenceDraft(e.target.value)}
                style={{ height: 'auto', minHeight: 76, paddingTop: 8, resize: 'vertical' }}
              />
              <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
                {task.outcome_status && (
                  <span className={`task-detail-status-badge status-${task.outcome_status}`}>
                    {task.outcome_status.replace(/_/g, ' ')}
                  </span>
                )}
                <button
                  type="button"
                  className="btn btn-primary btn-sm"
                  onClick={() => void handleSaveOutcome()}
                  disabled={outcomeBusy || (!outcomeDraft.trim() && !evidenceDraft.trim())}
                >
                  {outcomeBusy ? t('apps.taskDetail.outcome.saving') : t('apps.taskDetail.outcome.save')}
                </button>
              </div>
              {errorMsg && <div className="task-detail-error">{errorMsg}</div>}
            </div>
          </section>
        )}

        {!isHumanAction && (
          <section className="task-detail-section">
            <div className="task-detail-label">{t('apps.taskDetail.artifacts.title')}</div>
            <div style={{ display: 'grid', gap: 10 }}>
              {(task.artifacts ?? []).length > 0 ? (
                <div style={{ display: 'grid', gap: 8 }}>
                  {(task.artifacts ?? []).slice().reverse().slice(0, 4).map((artifact) => (
                    <div key={artifact.id || `${artifact.kind}-${artifact.path || artifact.url}`} className="task-detail-human-note">
                      <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap', marginBottom: 4 }}>
                        <span className="badge badge-accent">{artifact.kind || 'artifact'}</span>
                        <span style={{ fontSize: 12, fontWeight: 700 }}>{artifact.title || artifact.path || artifact.url}</span>
                      </div>
                      {artifact.summary && (
                        <div style={{ fontSize: 12, color: 'var(--text-secondary)', lineHeight: 1.45 }}>{artifact.summary}</div>
                      )}
                      {artifact.browser_inspection && (
                        <div className="app-card-meta">
                          {[
                            artifact.browser_inspection.selector,
                            artifact.browser_inspection.viewport_width && artifact.browser_inspection.viewport_height
                              ? `${artifact.browser_inspection.viewport_width}×${artifact.browser_inspection.viewport_height}`
                              : '',
                            artifact.browser_inspection.element_text,
                          ].filter(Boolean).join(' · ')}
                        </div>
                      )}
                      <div className="app-card-meta">
                        {[artifact.path || artifact.url || '', artifact.state || '', artifact.updated_at ? formatRelativeTime(artifact.updated_at) : ''].filter(Boolean).join(' · ')}
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="app-card-meta">{t('apps.taskDetail.artifacts.empty')}</div>
              )}
              <div style={{ display: 'grid', gridTemplateColumns: 'minmax(110px, 0.7fr) minmax(160px, 1fr)', gap: 8 }}>
                <select className="task-detail-select" value={artifactKind} onChange={(e) => setArtifactKind(e.target.value)}>
                  <option value="document">{t('apps.taskDetail.artifacts.kinds.document')}</option>
                  <option value="pull_request">{t('apps.taskDetail.artifacts.kinds.pullRequest')}</option>
                  <option value="build">{t('apps.taskDetail.artifacts.kinds.build')}</option>
                  <option value="decision">{t('apps.taskDetail.artifacts.kinds.decision')}</option>
                  <option value="link">{t('apps.taskDetail.artifacts.kinds.link')}</option>
                  <option value="browser_inspection">{t('apps.taskDetail.artifacts.kinds.browserInspection')}</option>
                </select>
                <input
                  className="task-detail-select"
                  value={artifactTitle}
                  placeholder={t('apps.taskDetail.artifacts.titlePlaceholder')}
                  onChange={(e) => setArtifactTitle(e.target.value)}
                />
              </div>
              {isBrowserInspectionArtifact && (
                <div style={{ display: 'grid', gap: 8 }}>
                  <input
                    className="task-detail-select"
                    value={browserPageURL}
                    placeholder={t('apps.taskDetail.artifacts.browser.pageUrlPlaceholder')}
                    onChange={(e) => setBrowserPageURL(e.target.value)}
                  />
                  <input
                    className="task-detail-select"
                    value={browserSelector}
                    placeholder={t('apps.taskDetail.artifacts.browser.selectorPlaceholder')}
                    onChange={(e) => setBrowserSelector(e.target.value)}
                  />
                  <input
                    className="task-detail-select"
                    value={browserElementText}
                    placeholder={t('apps.taskDetail.artifacts.browser.textPlaceholder')}
                    onChange={(e) => setBrowserElementText(e.target.value)}
                  />
                  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, minmax(90px, 1fr))', gap: 8 }}>
                    <input
                      className="task-detail-select"
                      value={browserViewportWidth}
                      inputMode="numeric"
                      placeholder={t('apps.taskDetail.artifacts.browser.viewportWidthPlaceholder')}
                      onChange={(e) => setBrowserViewportWidth(e.target.value)}
                    />
                    <input
                      className="task-detail-select"
                      value={browserViewportHeight}
                      inputMode="numeric"
                      placeholder={t('apps.taskDetail.artifacts.browser.viewportHeightPlaceholder')}
                      onChange={(e) => setBrowserViewportHeight(e.target.value)}
                    />
                  </div>
                </div>
              )}
              <input
                className="task-detail-select"
                value={artifactTarget}
                placeholder={isBrowserInspectionArtifact ? t('apps.taskDetail.artifacts.browser.screenshotPlaceholder') : t('apps.taskDetail.artifacts.targetPlaceholder')}
                onChange={(e) => setArtifactTarget(e.target.value)}
              />
              <textarea
                className="task-detail-select"
                rows={2}
                value={artifactSummary}
                placeholder={t('apps.taskDetail.artifacts.summaryPlaceholder')}
                onChange={(e) => setArtifactSummary(e.target.value)}
                style={{ height: 'auto', minHeight: 60, paddingTop: 8, resize: 'vertical' }}
              />
              <button
                type="button"
                className="btn btn-primary btn-sm"
                onClick={() => void handleAddArtifact()}
                disabled={artifactBusy || (!artifactTarget.trim() && !artifactSummary.trim() && (!isBrowserInspectionArtifact || (!browserPageURL.trim() && !browserSelector.trim() && !browserElementText.trim())))}
                style={{ justifySelf: 'start' }}
              >
                {artifactBusy ? t('apps.taskDetail.artifacts.adding') : t('apps.taskDetail.artifacts.add')}
              </button>
              {errorMsg && <div className="task-detail-error">{errorMsg}</div>}
            </div>
          </section>
        )}

        {!isHumanAction && (
          <section className="task-detail-section">
            <div className="task-detail-label">{t('apps.taskDetail.plan.title')}</div>
            <div style={{ display: 'grid', gap: 10 }}>
              <div className={`task-detail-plan-contract${task.plan_required ? ' task-detail-plan-contract-required' : ''}`}>
                <div>
                  <div className="task-detail-completion-contract-title">
                    {t(`apps.taskDetail.plan.status.${planStatus}`, { defaultValue: planStatus.replace(/_/g, ' ') })}
                  </div>
                  <div className="task-detail-completion-contract-copy">
                    {task.plan_blocker || t('apps.taskDetail.plan.contractCopy')}
                  </div>
                </div>
                {latestPlan && latestPlan.status !== 'approved' && (
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    onClick={() => void handleApprovePlan()}
                    disabled={planApprovalBusy}
                  >
                    {planApprovalBusy ? t('apps.taskDetail.plan.approving') : t('apps.taskDetail.plan.approve')}
                  </button>
                )}
              </div>
              {latestPlan && (
                <div className="app-card-meta">
                  {t('apps.taskDetail.plan.latest', {
                    version: latestPlan.version ?? (task.plan_revisions ?? []).length,
                    time: latestPlan.created_at ? formatRelativeTime(latestPlan.created_at) : 'n/a',
                  })}
                  {latestPlan.summary ? ` · ${latestPlan.summary}` : ''}
                  {latestPlan.approved_by ? ` · ${t('apps.taskDetail.plan.approvedBy', { actor: latestPlan.approved_by })}` : ''}
                </div>
              )}
              <input
                className="task-detail-select"
                value={planSummaryDraft}
                placeholder={t('apps.taskDetail.plan.summaryPlaceholder')}
                onChange={(e) => setPlanSummaryDraft(e.target.value)}
              />
              <textarea
                className="task-detail-select"
                rows={5}
                value={planDraft}
                placeholder={t('apps.taskDetail.plan.placeholder')}
                onChange={(e) => setPlanDraft(e.target.value)}
                style={{ height: 'auto', minHeight: 120, paddingTop: 8, resize: 'vertical' }}
              />
              <button
                type="button"
                className="btn btn-primary btn-sm"
                onClick={() => void handleSavePlanRevision()}
                disabled={planBusy || !planDraft.trim()}
                style={{ justifySelf: 'start' }}
              >
                {planBusy ? t('apps.taskDetail.plan.saving') : t('apps.taskDetail.plan.save')}
              </button>
              {errorMsg && <div className="task-detail-error">{errorMsg}</div>}
            </div>
          </section>
        )}

        {!isHumanAction && learningCandidate?.recommended && (
          <section className="task-detail-section">
            <div className="task-detail-label">{t('apps.taskDetail.learning.title')}</div>
            <div className="task-detail-learning-card">
              <div>
                <div className="task-detail-completion-contract-title">{learningCandidate.title || task.title}</div>
                <div className="task-detail-completion-contract-copy">
                  {learningCandidate.reason || learningCandidate.summary || t('apps.taskDetail.learning.copy')}
                </div>
                {learningCandidate.skill_name && (
                  <div className="app-card-meta" style={{ marginTop: 6 }}>{learningCandidate.skill_name}</div>
                )}
              </div>
              <div className="task-detail-learning-actions">
                <button
                  type="button"
                  className="btn btn-ghost btn-sm"
                  onClick={() => setShowLearningDiff((value) => !value)}
                  disabled={learningDiffFetching}
                >
                  {learningDiffFetching
                    ? t('apps.taskDetail.learning.previewing')
                    : showLearningDiff
                      ? t('apps.taskDetail.learning.hidePreview')
                      : t('apps.taskDetail.learning.preview')}
                </button>
                <button
                  type="button"
                  className="btn btn-primary btn-sm"
                  onClick={() => void handlePromoteLearning()}
                  disabled={learningBusy}
                >
                  {learningBusy ? t('apps.taskDetail.learning.promoting') : t('apps.taskDetail.learning.promote')}
                </button>
              </div>
            </div>
            {showLearningDiff && (
              <div className="task-detail-learning-diff">
                {learningDiffError ? (
                  <div className="task-detail-error">
                    {learningDiffError instanceof Error ? learningDiffError.message : t('apps.taskDetail.learning.previewFailed')}
                  </div>
                ) : learningDiff ? (
                  <>
                    <div className="task-detail-learning-diff-meta">
                      <span>{t(`apps.taskDetail.learning.actions.${learningDiff.action}`, { defaultValue: learningDiff.action.replace(/_/g, ' ') })}</span>
                      <span>{t('apps.taskDetail.learning.risk', { level: learningDiff.risk_level || 'low' })}</span>
                      <span>{learningDiff.persisted ? t('apps.taskDetail.learning.persisted') : t('apps.taskDetail.learning.notPersisted')}</span>
                    </div>
                    <div className="task-detail-learning-diff-files">
                      {(learningDiff.files ?? []).map((file) => (
                        <div key={file.name} className="task-detail-learning-diff-file">
                          <div>
                            <div className="task-detail-completion-contract-title">{file.name}</div>
                            <div className="task-detail-completion-contract-copy">
                              {t(`apps.taskDetail.learning.fileStatus.${file.status}`, { defaultValue: file.status.replace(/_/g, ' ') })}
                              {' · '}
                              {t('apps.taskDetail.learning.fileSizes', { before: file.before_size ?? 0, after: file.after_size ?? 0 })}
                            </div>
                          </div>
                          {file.after && (
                            <pre className="task-detail-learning-diff-code">{file.after}</pre>
                          )}
                        </div>
                      ))}
                    </div>
                  </>
                ) : (
                  <div className="app-card-meta">{t('apps.taskDetail.learning.previewLoading')}</div>
                )}
              </div>
            )}
            {errorMsg && <div className="task-detail-error">{errorMsg}</div>}
          </section>
        )}

        {!isHumanAction && (
          <section className="task-detail-section">
            <div className="task-detail-label">{t('apps.taskDetail.feedback.title')}</div>
            <div style={{ display: 'grid', gap: 10 }}>
              {(task.feedback ?? []).length > 0 ? (
                <div style={{ display: 'grid', gap: 6 }}>
                  {(task.feedback ?? []).slice().reverse().slice(0, 3).map((item) => (
                    <div key={item.id || `${item.rating}-${item.created_at}`} className="app-card-meta">
                      {item.rating || 'neutral'} · {item.comment || t('apps.taskDetail.feedback.noComment')}
                    </div>
                  ))}
                </div>
              ) : (
                <div className="app-card-meta">{t('apps.taskDetail.feedback.empty')}</div>
              )}
              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                {(['up', 'neutral', 'down'] as const).map((rating) => (
                  <button
                    key={rating}
                    type="button"
                    className={`btn btn-sm ${feedbackRating === rating ? 'btn-primary' : 'btn-ghost'}`}
                    onClick={() => setFeedbackRating(rating)}
                  >
                    {t(`apps.taskDetail.feedback.${rating}`)}
                  </button>
                ))}
              </div>
              <textarea
                className="task-detail-select"
                rows={2}
                value={feedbackComment}
                placeholder={t('apps.taskDetail.feedback.placeholder')}
                onChange={(e) => setFeedbackComment(e.target.value)}
                style={{ height: 'auto', minHeight: 60, paddingTop: 8, resize: 'vertical' }}
              />
              <button type="button" className="btn btn-primary btn-sm" onClick={() => void handleAddFeedback()} disabled={feedbackBusy} style={{ justifySelf: 'start' }}>
                {feedbackBusy ? t('apps.taskDetail.feedback.saving') : t('apps.taskDetail.feedback.save')}
              </button>
              {errorMsg && <div className="task-detail-error">{errorMsg}</div>}
            </div>
          </section>
        )}

        {isHumanAction && (
          <section className={`task-detail-section ${humanActionPanelClass}`}>
            <div className="task-detail-human-action-header">
              <div>
                <div className="task-detail-label">{t('apps.taskDetail.humanAction.title')}</div>
                <div className="task-detail-human-action-heading">
                  {t('apps.taskDetail.humanAction.decisionNeeded')}
                </div>
              </div>
              <div className="task-detail-human-action-badges">
                <span className="badge badge-waiting">{t('apps.taskDetail.humanAction.title')}</span>
                {currentStatus === 'blocked' ? (
                  <span className="badge badge-attention">{t('apps.taskDetail.statusBtn.block')}</span>
                ) : null}
              </div>
            </div>

            <div className="task-detail-human-action-content">
              <div className="task-detail-human-action-warning">
                {t('apps.taskDetail.humanAction.blockingNewMessages')}
              </div>

              <div className="task-detail-human-question">
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
                <div className="task-detail-human-note task-detail-human-recommendation">
                  <div className="task-detail-human-note-title">
                    {t('apps.taskDetail.humanAction.agentNote')}
                  </div>
                  <div className="task-detail-human-note-body">
                    {task.recommendation_summary}
                  </div>
                </div>
              )}

              {!hasRecommendation && task.recommendation_status === 'requested' && (
                <div className="app-card-meta">{t('apps.taskDetail.humanAction.recommendationRequested')}</div>
              )}

              {textMode ? (
                <div className="task-detail-human-text-mode">
                  <textarea
                    className="task-detail-select task-detail-human-textarea"
                    rows={4}
                    value={customText}
                    placeholder={textMode.text_hint || t('apps.taskDetail.humanAction.textPlaceholder')}
                    onChange={(e) => setCustomText(e.target.value)}
                  />
                  <div className="task-detail-human-actions">
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
                <div className="task-detail-human-options">
                  {requestOptions.map((option) => (
                    <button
                      key={option.id}
                      type="button"
                      className={`btn btn-sm ${option.id === task.human_recommended_id ? 'btn-primary' : 'btn-ghost'} task-detail-human-option-btn`}
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

              <div className="task-detail-human-secondary-actions">
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
  disabled?: boolean
  disabledTitle?: string
  danger?: boolean
}

function StatusButton({
  action,
  label,
  busy,
  disabledFor,
  currentStatus,
  onClick,
  disabled,
  disabledTitle,
  danger,
}: StatusButtonProps) {
  const { t } = useTranslation()
  const isCurrent = disabledFor.includes(currentStatus)
  const isBusy = busy === action
  const anyBusy = busy !== null
  const isDisabled = Boolean(disabled)
  const className = 'btn btn-sm ' + (danger ? 'btn-ghost task-detail-status-btn-danger' : 'btn-ghost')
  return (
    <button
      type="button"
      className={className}
      onClick={() => onClick(action)}
      disabled={isCurrent || anyBusy || isDisabled}
      title={isDisabled ? disabledTitle : isCurrent ? t('apps.taskDetail.statusBtn.alreadyInState') : undefined}
    >
      {isBusy ? '...' : label}
    </button>
  )
}

function formatRuntimeMinutes(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return '0m'
  return `${Math.max(1, Math.round(ms / 60_000))}m`
}

function formatCostCents(cents: number): string {
  if (!Number.isFinite(cents) || cents <= 0) return '$0.00'
  return `$${(cents / 100).toFixed(2)}`
}
