import { useCallback, useState, type DragEvent, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import i18n from '../../i18n/config'
import { post, type Task } from '../../api/client'
import { formatRelativeTime } from '../../lib/format'
import { TaskDetailModal } from './TaskDetailModal'
import { showNotice } from '../ui/Toast'
import { useOfficeTasks } from '../../hooks/useTasks'
import { useAppStore } from '../../stores/app'

const STATUS_ORDER = ['in_progress', 'open', 'review', 'pending', 'blocked', 'done', 'canceled'] as const

type StatusGroup = typeof STATUS_ORDER[number]

const DND_MIME = 'application/x-wuphf-task-id'
const HUMAN_SLUG = 'human'

type CreateTaskFormState = {
  title: string
  owner: string
  details: string
  executionMode: 'office' | 'local_worktree' | 'external_workspace'
  workspacePath: string
}

const EMPTY_CREATE_FORM: CreateTaskFormState = {
  title: '',
  owner: '',
  details: '',
  executionMode: 'office',
  workspacePath: '',
}

function columnLabel(status: StatusGroup): string {
  return i18n.t(`apps.tasks.columns.${status}`)
}

function normalizeStatus(raw: string): StatusGroup {
  const s = raw.toLowerCase().replace(/[\s-]+/g, '_')
  if (s === 'completed') return 'done'
  if (s === 'in_review') return 'review'
  if (s === 'cancelled') return 'canceled'
  if ((STATUS_ORDER as readonly string[]).includes(s)) return s as StatusGroup
  return 'open'
}

function statusBadgeClass(status: StatusGroup): string {
  if (status === 'done') return 'badge badge-green'
  if (status === 'in_progress' || status === 'review') return 'badge badge-accent'
  if (status === 'blocked') return 'badge badge-attention'
  if (status === 'canceled') return 'badge badge-muted'
  return 'badge badge-accent'
}

function groupTasks(tasks: Task[]): Record<StatusGroup, Task[]> {
  const groups: Record<StatusGroup, Task[]> = {
    in_progress: [],
    open: [],
    review: [],
    pending: [],
    blocked: [],
    done: [],
    canceled: [],
  }
  for (const task of tasks) {
    const status = normalizeStatus(task.status)
    groups[status].push(task)
  }
  return groups
}

function isHumanActionTask(task: Task): boolean {
  return Boolean(task.awaiting_human) || (task.task_type ?? '') === 'human_action'
}

/**
 * Map a target column (StatusGroup) to the backend action payload.
 * Returns null when the transition has no corresponding action (e.g. "pending").
 */
function buildMoveBody(task: Task, toStatus: StatusGroup): Record<string, string> | null {
  const base: Record<string, string> = {
    id: task.id,
    channel: task.channel || 'general',
    created_by: HUMAN_SLUG,
  }
  switch (toStatus) {
    case 'in_progress':
      return { ...base, action: 'claim', owner: HUMAN_SLUG }
    case 'open':
      return { ...base, action: 'release' }
    case 'review':
      return { ...base, action: 'review' }
    case 'done':
      return { ...base, action: 'complete' }
    case 'blocked':
      return { ...base, action: 'block' }
    case 'canceled':
      return { ...base, action: 'cancel' }
    case 'pending':
      // No direct "pending" action in the broker — punted.
      return null
  }
}

function useTaskMove() {
  const queryClient = useQueryClient()

  return useCallback(
    async (task: Task, toStatus: StatusGroup) => {
      const fromStatus = normalizeStatus(task.status)
      if (fromStatus === toStatus) return

      const body = buildMoveBody(task, toStatus)
      if (!body) return

      try {
        await post('/tasks', body)
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : i18n.t('apps.tasks.moveFailed')
        showNotice(message, 'error')
      } finally {
        await queryClient.invalidateQueries({ queryKey: ['office-tasks'] })
      }
    },
    [queryClient],
  )
}

export function TasksApp() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentChannel = useAppStore((s) => s.currentChannel)
  const { data, isLoading, error } = useOfficeTasks({ includeDone: true, fallbackMs: 10_000 })

  const moveTask = useTaskMove()
  const [draggingId, setDraggingId] = useState<string | null>(null)
  const [dragoverStatus, setDragoverStatus] = useState<StatusGroup | null>(null)
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null)
  const [createForm, setCreateForm] = useState<CreateTaskFormState>(EMPTY_CREATE_FORM)
  const [creating, setCreating] = useState(false)

  if (isLoading) {
    return (
      <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
        {t('apps.tasks.loading')}
      </div>
    )
  }

  if (error) {
    return (
      <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
        {t('apps.tasks.loadFailed')}
      </div>
    )
  }

  const tasks = data?.tasks ?? []
  const humanTasks = tasks
    .filter((task) => isHumanActionTask(task))
    .sort((a, b) => {
      const ar = a.recommendation_status === 'ready' ? 0 : a.recommendation_status === 'requested' ? 1 : 2
      const br = b.recommendation_status === 'ready' ? 0 : b.recommendation_status === 'requested' ? 1 : 2
      if (ar !== br) return ar - br
      return String(b.updated_at ?? b.created_at ?? '').localeCompare(String(a.updated_at ?? a.created_at ?? ''))
    })
  const normalTasks = tasks.filter((task) => !isHumanActionTask(task))

  const grouped = groupTasks(normalTasks)
  const tasksById = new Map(tasks.map((t) => [t.id, t]))
  const isDragging = draggingId !== null
  const selectedTask = selectedTaskId ? tasksById.get(selectedTaskId) ?? null : null

  async function handleCreateTask(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const title = createForm.title.trim()
    const owner = createForm.owner.trim()
    const details = createForm.details.trim()
    const workspacePath = createForm.workspacePath.trim()
    const targetChannel = currentChannel.trim() || 'general'

    if (!title) {
      showNotice(t('apps.tasks.createTitleRequired'), 'error')
      return
    }
    if (createForm.executionMode === 'external_workspace' && !workspacePath) {
      showNotice(t('apps.tasks.workspaceRequired'), 'error')
      return
    }

    setCreating(true)
    try {
      const body: Record<string, unknown> = {
        action: 'create',
        channel: targetChannel,
        created_by: HUMAN_SLUG,
        title,
        execution_mode: createForm.executionMode,
      }
      if (owner) body.owner = owner
      if (details) body.details = details
      if (createForm.executionMode === 'external_workspace') body.workspace_path = workspacePath
      await post('/tasks', body)
      setCreateForm(EMPTY_CREATE_FORM)
      await queryClient.invalidateQueries({ queryKey: ['office-tasks'] })
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : t('apps.tasks.createFailed')
      showNotice(message, 'error')
    } finally {
      setCreating(false)
    }
  }

  const handleDragStart = (taskId: string) => (event: DragEvent<HTMLDivElement>) => {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData(DND_MIME, taskId)
    // Fallback for browsers that restrict custom MIME reads during dragover.
    event.dataTransfer.setData('text/plain', taskId)
    setDraggingId(taskId)
  }

  const handleDragEnd = () => {
    setDraggingId(null)
    setDragoverStatus(null)
  }

  const handleColumnDragOver = (status: StatusGroup) => (event: DragEvent<HTMLDivElement>) => {
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
    if (dragoverStatus !== status) setDragoverStatus(status)
  }

  const handleColumnDragLeave = (status: StatusGroup) => (event: DragEvent<HTMLDivElement>) => {
    // Only clear when leaving the column itself, not a nested child.
    if (event.currentTarget.contains(event.relatedTarget as Node | null)) return
    if (dragoverStatus === status) setDragoverStatus(null)
  }

  const handleColumnDrop = (status: StatusGroup) => (event: DragEvent<HTMLDivElement>) => {
    event.preventDefault()
    const taskId = event.dataTransfer.getData(DND_MIME) || event.dataTransfer.getData('text/plain')
    setDraggingId(null)
    setDragoverStatus(null)
    if (!taskId) return
    const task = tasksById.get(taskId)
    if (!task) return
    void moveTask(task, status)
  }

  return (
    <>
      <div style={{ padding: '16px 20px 0', borderBottom: '1px solid var(--border)' }}>
        <h3 style={{ fontSize: 16, fontWeight: 700 }}>{t('apps.tasks.title')}</h3>
        <div style={{ fontSize: 12, color: 'var(--text-tertiary)', marginTop: 4, marginBottom: 12 }}>
          {t('apps.tasks.subtitle')}
        </div>
        <div className="app-card-meta" style={{ marginBottom: 10 }}>
          {t('apps.tasks.form.channelHint', { channel: currentChannel || 'general' })}
        </div>
        <form
          onSubmit={handleCreateTask}
          style={{
            display: 'grid',
            gridTemplateColumns: 'minmax(220px, 2fr) minmax(140px, 1fr) minmax(170px, 1fr)',
            gap: 10,
            paddingBottom: 16,
          }}
        >
          <input
            value={createForm.title}
            onChange={(event) => setCreateForm((current) => ({ ...current, title: event.target.value }))}
            placeholder={t('apps.tasks.form.titlePlaceholder')}
            aria-label={t('apps.tasks.form.title')}
            style={taskInputStyle}
          />
          <input
            value={createForm.owner}
            onChange={(event) => setCreateForm((current) => ({ ...current, owner: event.target.value }))}
            placeholder={t('apps.tasks.form.ownerPlaceholder')}
            aria-label={t('apps.tasks.form.owner')}
            style={taskInputStyle}
          />
          <select
            value={createForm.executionMode}
            onChange={(event) =>
              setCreateForm((current) => ({
                ...current,
                executionMode: event.target.value as CreateTaskFormState['executionMode'],
                workspacePath: event.target.value === 'external_workspace' ? current.workspacePath : '',
              }))
            }
            aria-label={t('apps.tasks.form.executionMode')}
            style={taskInputStyle}
          >
            <option value="office">{t('apps.tasks.form.executionModes.office')}</option>
            <option value="local_worktree">{t('apps.tasks.form.executionModes.local_worktree')}</option>
            <option value="external_workspace">{t('apps.tasks.form.executionModes.external_workspace')}</option>
          </select>
          <textarea
            value={createForm.details}
            onChange={(event) => setCreateForm((current) => ({ ...current, details: event.target.value }))}
            placeholder={t('apps.tasks.form.detailsPlaceholder')}
            aria-label={t('apps.tasks.form.details')}
            rows={3}
            style={{ ...taskInputStyle, gridColumn: '1 / span 2', resize: 'vertical', minHeight: 78 }}
          />
          <div style={{ display: 'grid', gap: 10 }}>
            {createForm.executionMode === 'external_workspace' && (
              <input
                value={createForm.workspacePath}
                onChange={(event) => setCreateForm((current) => ({ ...current, workspacePath: event.target.value }))}
                placeholder={t('apps.tasks.form.workspacePathPlaceholder')}
                aria-label={t('apps.tasks.form.workspacePath')}
                style={taskInputStyle}
              />
            )}
            <button
              type="submit"
              className="btn btn-primary"
              disabled={creating}
              style={{ justifySelf: 'start' }}
            >
              {creating ? t('apps.tasks.form.creating') : t('apps.tasks.form.create')}
            </button>
          </div>
        </form>
      </div>

      {tasks.length === 0 ? (
        <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
          {t('apps.tasks.empty')}
        </div>
      ) : (
        <>
          {humanTasks.length > 0 && (
            <div style={{ padding: '16px 20px 0' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 10 }}>
                <div style={{ fontSize: 14, fontWeight: 700 }}>{t('apps.tasks.humanQueue')}</div>
                <div className="app-card-meta">{t('apps.tasks.humanQueueCount', { count: humanTasks.length })}</div>
              </div>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(260px, 1fr))', gap: 10, marginBottom: 16 }}>
                {humanTasks.map((task) => (
                  <TaskCard
                    key={task.id}
                    task={task}
                    isDragging={false}
                    draggable={false}
                    onDragStart={noopDragHandler}
                    onDragEnd={noopDragHandler}
                    onOpen={() => setSelectedTaskId(task.id)}
                  />
                ))}
              </div>
            </div>
          )}

          <div style={{ padding: '8px 20px 0' }}>
            <div style={{ fontSize: 13, fontWeight: 700 }}>{t('apps.tasks.boardTitle')}</div>
            <div className="app-card-meta" style={{ marginTop: 4 }}>
              {t('apps.tasks.boardSubtitle')}
            </div>
          </div>

          <div className="task-board" style={{ paddingTop: 12 }}>
            {STATUS_ORDER.map((status) => {
              const column = grouped[status]
              if (
                !isDragging &&
                column.length === 0 &&
                (status === 'pending' || status === 'blocked' || status === 'canceled')
              ) {
                return null
              }
              const columnClass =
                'task-column' + (dragoverStatus === status ? ' dragover' : '')
              return (
                <div
                  className={columnClass}
                  key={status}
                  onDragOver={handleColumnDragOver(status)}
                  onDragLeave={handleColumnDragLeave(status)}
                  onDrop={handleColumnDrop(status)}
                >
                  <div className="task-column-header">
                    <span>{columnLabel(status)}</span>
                    <span className="task-column-count">{column.length}</span>
                  </div>
                  {column.map((task) => (
                    <TaskCard
                      key={task.id}
                      task={task}
                      isDragging={draggingId === task.id}
                      onDragStart={handleDragStart(task.id)}
                      onDragEnd={handleDragEnd}
                      onOpen={() => setSelectedTaskId(task.id)}
                    />
                  ))}
                </div>
              )
            })}
          </div>
        </>
      )}
      {selectedTask && (
        <TaskDetailModal
          task={selectedTask}
          onClose={() => setSelectedTaskId(null)}
        />
      )}
    </>
  )
}

const taskInputStyle: React.CSSProperties = {
  width: '100%',
  borderRadius: 8,
  border: '1px solid var(--border)',
  background: 'var(--panel)',
  color: 'var(--text-primary)',
  padding: '10px 12px',
  fontSize: 13,
}

interface TaskCardProps {
  task: Task
  isDragging: boolean
  onDragStart: (event: DragEvent<HTMLDivElement>) => void
  onDragEnd: (event: DragEvent<HTMLDivElement>) => void
  onOpen: () => void
  draggable?: boolean
}

function noopDragHandler(_event: DragEvent<HTMLDivElement>) {}

function TaskCard({ task, isDragging, onDragStart, onDragEnd, onOpen, draggable = true }: TaskCardProps) {
  const { t } = useTranslation()
  const status = normalizeStatus(task.status)
  const timestamp = task.updated_at ?? task.created_at
  const className =
    'app-card task-card'
    + (isHumanActionTask(task) ? ' app-card-waiting' : '')
    + (status === 'blocked' ? ' app-card-attention' : '')
    + (isDragging ? ' dragging' : '')

  function handleKeyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      onOpen()
    }
  }

  return (
    <div
      className={className}
      draggable={draggable}
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      onClick={onOpen}
      onKeyDown={handleKeyDown}
      role="button"
      tabIndex={0}
      style={{ marginBottom: 8, cursor: 'pointer' }}
    >
      <div className="app-card-title">{task.title || t('apps.tasks.untitled')}</div>
      {task.description && (
        <div style={{ fontSize: 12, color: 'var(--text-secondary)', marginBottom: 8, lineHeight: 1.45 }}>
          {task.description.slice(0, 160)}
        </div>
      )}
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
        <span className={statusBadgeClass(status)}>
          {columnLabel(status)}
        </span>
        {task.owner && (
          <span className="app-card-meta">@{task.owner}</span>
        )}
        {task.channel && (
          <span className="app-card-meta">#{task.channel}</span>
        )}
        {task.awaiting_human && (
          <span className="badge badge-waiting">{t('apps.tasks.humanQueue')}</span>
        )}
        {typeof task.progress_percent === 'number' && task.progress_basis && (
          <span className="app-card-meta">{task.progress_percent}% · {task.progress_basis}</span>
        )}
        {timestamp && (
          <span className="app-card-meta">{formatRelativeTime(timestamp)}</span>
        )}
      </div>
      {task.awaiting_human_reason && (
        <div style={{ fontSize: 12, color: 'var(--text-secondary)', marginTop: 8, lineHeight: 1.45 }}>
          {task.awaiting_human_reason}
        </div>
      )}
      {task.recommendation_status === 'ready' && task.recommendation_summary && (
        <div style={{ fontSize: 12, color: 'var(--green)', marginTop: 8, lineHeight: 1.45 }}>
          {task.recommendation_summary}
        </div>
      )}
    </div>
  )
}
