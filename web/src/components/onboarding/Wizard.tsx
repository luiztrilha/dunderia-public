import { useState, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { useAppStore } from '../../stores/app'
import { get, post } from '../../api/client'
import '../../styles/onboarding.css'

/* ═══════════════════════════════════════════
   Types
   ═══════════════════════════════════════════ */

interface BlueprintTemplate {
  id: string
  name: string
  description: string
  emoji?: string
  agents?: BlueprintAgent[]
}

interface BlueprintAgent {
  slug: string
  name: string
  role: string
  emoji?: string
  checked?: boolean
}

interface TaskTemplate {
  id: string
  name: string
  description: string
  emoji?: string
  prompt?: string
}

type WizardStep = 'welcome' | 'templates' | 'identity' | 'team' | 'setup' | 'task'

const STEP_ORDER: readonly WizardStep[] = [
  'welcome',
  'templates',
  'identity',
  'team',
  'setup',
  'task',
] as const

const RUNTIME_OPTIONS = ['Claude Code', 'Codex', 'Cursor', 'Windsurf', 'Other'] as const

const API_KEY_FIELDS = [
  { key: 'ANTHROPIC_API_KEY', i18n: 'anthropic' },
  { key: 'OPENAI_API_KEY', i18n: 'openai' },
  { key: 'GOOGLE_API_KEY', i18n: 'google' },
] as const

type MemoryBackend = 'none'

/* ═══════════════════════════════════════════
   Arrow icon reused across buttons
   ═══════════════════════════════════════════ */

function ArrowIcon() {
  return (
    <svg
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M5 12h14" />
      <path d="m12 5 7 7-7 7" />
    </svg>
  )
}

function CheckIcon() {
  return (
    <svg
      width="12"
      height="12"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="3"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <polyline points="20 6 9 17 4 12" />
    </svg>
  )
}

/* ═══════════════════════════════════════════
   Sub-components
   ═══════════════════════════════════════════ */

function ProgressDots({ current }: { current: WizardStep }) {
  return (
    <div className="wizard-progress">
      {STEP_ORDER.map((step) => (
        <div
          key={step}
          className={`wizard-progress-dot ${step === current ? 'active' : 'inactive'}`}
        />
      ))}
    </div>
  )
}

/* ─── Step 1: Welcome ─── */

interface WelcomeStepProps {
  onNext: () => void
}

function WelcomeStep({ onNext }: WelcomeStepProps) {
  const { t } = useTranslation()
  return (
    <div className="wizard-step">
      <div className="wizard-hero">
        <div className="wizard-eyebrow">
          <span className="status-dot active pulse" />
          {t('wizards.onboarding.ready')}
        </div>
        <h1 className="wizard-headline">{t('wizards.onboarding.copy.step1Headline')}</h1>
        <p className="wizard-subhead">{t('wizards.onboarding.copy.step1Subhead')}</p>
      </div>
      <div style={{ display: 'flex', justifyContent: 'center' }}>
        <button className="btn btn-primary btn-lg" onClick={onNext}>
          {t('wizards.onboarding.copy.step1Cta')}
          <ArrowIcon />
        </button>
      </div>
    </div>
  )
}

/* ─── Step 2: Templates ─── */

interface TemplatesStepProps {
  templates: BlueprintTemplate[]
  loading: boolean
  selected: string | null
  onSelect: (id: string | null) => void
  onNext: () => void
  onBack: () => void
}

function TemplatesStep({
  templates,
  loading,
  selected,
  onSelect,
  onNext,
  onBack,
}: TemplatesStepProps) {
  const { t: tr } = useTranslation()
  return (
    <div className="wizard-step">
      <div className="wizard-hero">
        <div className="wizard-eyebrow">
          <span className="status-dot active pulse" />
          {tr('wizards.onboarding.templates.eyebrow')}
        </div>
        <h1 className="wizard-headline">{tr('wizards.onboarding.templates.headline')}</h1>
        <p className="wizard-subhead">
          {tr('wizards.onboarding.templates.subhead')}
        </p>
      </div>

      <div className="wizard-panel">
        {loading ? (
          <div style={{ color: 'var(--text-tertiary)', fontSize: 13, textAlign: 'center', padding: 20 }}>
            {tr('wizards.onboarding.templates.loading')}
          </div>
        ) : (
          <div className="template-grid">
            <button
              className={`template-card ${selected === null ? 'selected' : ''}`}
              onClick={() => onSelect(null)}
              type="button"
            >
              <div className="template-card-emoji">&#x1F4DD;</div>
              <div className="template-card-name">{tr('wizards.onboarding.templates.scratchName')}</div>
              <div className="template-card-desc">
                {tr('wizards.onboarding.templates.scratchDesc')}
              </div>
            </button>
            {templates.map((t) => (
              <button
                key={t.id}
                className={`template-card ${selected === t.id ? 'selected' : ''}`}
                onClick={() => onSelect(t.id)}
                type="button"
              >
                {t.emoji && <div className="template-card-emoji">{t.emoji}</div>}
                <div className="template-card-name">{t.name}</div>
                <div className="template-card-desc">{t.description}</div>
              </button>
            ))}
          </div>
        )}
      </div>

      <div className="wizard-nav">
        <button className="btn btn-ghost" onClick={onBack} type="button">
          {tr('wizards.common.back')}
        </button>
        <button className="btn btn-primary" onClick={onNext} type="button">
          {tr('wizards.common.continue')}
          <ArrowIcon />
        </button>
      </div>
    </div>
  )
}

/* ─── Step 3: Identity ─── */

interface IdentityStepProps {
  company: string
  description: string
  priority: string
  onChangeCompany: (v: string) => void
  onChangeDescription: (v: string) => void
  onChangePriority: (v: string) => void
  onNext: () => void
  onBack: () => void
}

function IdentityStep({
  company,
  description,
  priority,
  onChangeCompany,
  onChangeDescription,
  onChangePriority,
  onNext,
  onBack,
}: IdentityStepProps) {
  const { t } = useTranslation()
  const canContinue = company.trim().length > 0 && description.trim().length > 0

  return (
    <div className="wizard-step">
      <div className="wizard-panel">
        <p className="wizard-panel-title">{t('wizards.onboarding.identity.panelTitle')}</p>
        <div className="form-group">
          <label className="label" htmlFor="wiz-company">
            {t('wizards.onboarding.identity.companyLabel')} <span style={{ color: 'var(--red)' }}>*</span>
          </label>
          <input
            className="input"
            id="wiz-company"
            placeholder={t('wizards.onboarding.identity.companyPlaceholder')}
            autoComplete="organization"
            value={company}
            onChange={(e) => onChangeCompany(e.target.value)}
          />
        </div>
        <div className="form-group">
          <label className="label" htmlFor="wiz-description">
            {t('wizards.onboarding.identity.descriptionLabel')} <span style={{ color: 'var(--red)' }}>*</span>
          </label>
          <input
            className="input"
            id="wiz-description"
            placeholder={t('wizards.onboarding.identity.descriptionPlaceholder')}
            value={description}
            onChange={(e) => onChangeDescription(e.target.value)}
          />
        </div>
        <div className="form-group">
          <label className="label" htmlFor="wiz-priority">
            {t('wizards.onboarding.identity.priorityLabel')}
          </label>
          <input
            className="input"
            id="wiz-priority"
            placeholder={t('wizards.onboarding.identity.priorityPlaceholder')}
            value={priority}
            onChange={(e) => onChangePriority(e.target.value)}
          />
        </div>
      </div>

      <div className="wizard-nav">
        <button className="btn btn-ghost" onClick={onBack} type="button">
          {t('wizards.common.back')}
        </button>
        <button
          className="btn btn-primary"
          onClick={onNext}
          disabled={!canContinue}
          type="button"
        >
          {t('wizards.onboarding.identity.reviewTeam')}
          <ArrowIcon />
        </button>
      </div>
    </div>
  )
}

/* ─── Step 4: Team Review ─── */

interface TeamStepProps {
  agents: BlueprintAgent[]
  onToggle: (slug: string) => void
  onNext: () => void
  onBack: () => void
}

function TeamStep({ agents, onToggle, onNext, onBack }: TeamStepProps) {
  const { t } = useTranslation()
  return (
    <div className="wizard-step">
      <div className="wizard-panel">
        <p className="wizard-panel-title">{t('wizards.onboarding.team.panelTitle')}</p>
        <p style={{ fontSize: 12, color: 'var(--text-secondary)', margin: '-8px 0 12px 0' }}>
          {t('wizards.onboarding.team.subhead')}
        </p>

        {agents.length === 0 ? (
          <div className="wiz-team-empty">
            {t('wizards.onboarding.team.empty')}
          </div>
        ) : (
          <div className="wiz-team-grid">
            {agents.map((a) => (
              <button
                key={a.slug}
                className={`wiz-team-tile ${a.checked ? 'selected' : ''}`}
                onClick={() => onToggle(a.slug)}
                type="button"
              >
                <div className="wiz-team-check">
                  {a.checked && <CheckIcon />}
                </div>
                <div>
                  {a.emoji && (
                    <span style={{ marginRight: 6 }}>{a.emoji}</span>
                  )}
                  <span className="wiz-team-name">{a.name}</span>
                  {a.role && <div className="wiz-team-role">{a.role}</div>}
                </div>
              </button>
            ))}
          </div>
        )}
      </div>

      <div className="wizard-nav">
        <button className="btn btn-ghost" onClick={onBack} type="button">
          {t('wizards.common.back')}
        </button>
        <button className="btn btn-primary" onClick={onNext} type="button">
          {t('wizards.common.continue')}
          <ArrowIcon />
        </button>
      </div>
    </div>
  )
}

/* ─── Step 5: Setup ─── */

interface SetupStepProps {
  runtime: string
  onChangeRuntime: (v: string) => void
  apiKeys: Record<string, string>
  onChangeApiKey: (key: string, value: string) => void
  memoryBackend: MemoryBackend
  onNext: () => void
  onBack: () => void
}

function SetupStep({
  runtime,
  onChangeRuntime,
  apiKeys,
  onChangeApiKey,
  memoryBackend,
  onNext,
  onBack,
}: SetupStepProps) {
  const { t } = useTranslation()

  return (
    <div className="wizard-step">
      <div className="wizard-panel">
        <p className="wizard-panel-title">{t('wizards.onboarding.copy.step2PrereqsTitle')}</p>
        <p style={{ fontSize: 12, color: 'var(--text-secondary)', margin: '-8px 0 12px 0' }}>
          {t('wizards.onboarding.setup.runtimeHint')}
        </p>
        <div className="runtime-grid">
          {RUNTIME_OPTIONS.map((opt) => (
            <button
              key={opt}
              className={`runtime-tile ${runtime === opt ? 'selected' : ''}`}
              onClick={() => onChangeRuntime(opt)}
              type="button"
            >
              {opt}
            </button>
          ))}
        </div>
      </div>

      <div className="wizard-panel">
        <p className="wizard-panel-title">{t('wizards.onboarding.copy.step2KeysTitle')}</p>
        <p style={{ fontSize: 12, color: 'var(--text-secondary)', margin: '-8px 0 12px 0' }}>
          {t('wizards.onboarding.setup.keysHint')}
        </p>
        {API_KEY_FIELDS.map((field) => (
          <div className="key-row" key={field.key}>
            <div className="key-label-wrap">
              <span className="key-label">{t(`wizards.onboarding.setup.apiKeys.${field.i18n}.label`)}</span>
              <span className="key-hint">{t(`wizards.onboarding.setup.apiKeys.${field.i18n}.hint`)}</span>
            </div>
            <div className="key-input-wrap">
              <input
                className="input"
                type="password"
                placeholder={field.key}
                value={apiKeys[field.key] ?? ''}
                onChange={(e) => onChangeApiKey(field.key, e.target.value)}
                autoComplete="off"
              />
            </div>
          </div>
        ))}
      </div>

      <div className="wizard-panel">
        <p className="wizard-panel-title">{t('wizards.onboarding.setup.memoryTitle')}</p>
        <p style={{ fontSize: 12, color: 'var(--text-secondary)', margin: '-8px 0 12px 0' }}>
          {t('wizards.onboarding.setup.memoryHint')}{' '}
          <code>--memory-backend</code>.
        </p>
        <div className="runtime-grid">
          <div className="runtime-tile selected" title={t('wizards.onboarding.setup.memoryOptions.none.hint')}>
            <div style={{ fontWeight: 600 }}>{t('wizards.onboarding.setup.memoryOptions.none.label')}</div>
            <div
              style={{
                fontSize: 11,
                color: 'var(--text-tertiary)',
                marginTop: 4,
                fontWeight: 400,
              }}
            >
              {t('wizards.onboarding.setup.memoryOptions.none.hint')}
            </div>
          </div>
        </div>
      </div>

      <div className="wizard-nav">
        <button className="btn btn-ghost" onClick={onBack} type="button">
          {t('wizards.common.back')}
        </button>
        <button
          className="btn btn-primary"
          onClick={onNext}
          type="button"
        >
          {t('wizards.onboarding.copy.step2Cta')}
          <ArrowIcon />
        </button>
      </div>
    </div>
  )
}

/* ─── Step 6: First Task ─── */

interface TaskStepProps {
  taskTemplates: TaskTemplate[]
  selectedTaskTemplate: string | null
  onSelectTaskTemplate: (id: string | null) => void
  taskText: string
  onChangeTaskText: (v: string) => void
  onSkip: () => void
  onSubmit: () => void
  onBack: () => void
  submitting: boolean
  submissionError: string | null
}

function TaskStep({
  taskTemplates,
  selectedTaskTemplate,
  onSelectTaskTemplate,
  taskText,
  onChangeTaskText,
  onSkip,
  onSubmit,
  onBack,
  submitting,
  submissionError,
}: TaskStepProps) {
  const { t: tr } = useTranslation()
  return (
    <div className="wizard-step">
      <div>
        <h2
          style={{
            fontSize: 18,
            fontWeight: 700,
            textAlign: 'left',
            marginBottom: 4,
          }}
        >
          {tr('wizards.onboarding.copy.step3Title')}
        </h2>
      </div>

      {taskTemplates.length > 0 && (
        <div className="template-grid">
          {taskTemplates.map((t) => (
            <button
              key={t.id}
              className={`template-card ${selectedTaskTemplate === t.id ? 'selected' : ''}`}
              onClick={() => {
                onSelectTaskTemplate(selectedTaskTemplate === t.id ? null : t.id)
                if (t.prompt) onChangeTaskText(t.prompt)
              }}
              type="button"
            >
              {t.emoji && <div className="template-card-emoji">{t.emoji}</div>}
              <div className="template-card-name">{t.name}</div>
              <div className="template-card-desc">{t.description}</div>
            </button>
          ))}
        </div>
      )}

      <div>
        <label
          className="label"
          htmlFor="wiz-task-input"
          style={{ marginBottom: 8, display: 'block' }}
        >
          {tr('wizards.onboarding.task.describeLabel')}
        </label>
        <textarea
          className="task-textarea"
          id="wiz-task-input"
          placeholder={tr('wizards.onboarding.copy.step3Placeholder')}
          value={taskText}
          onChange={(e) => onChangeTaskText(e.target.value)}
        />
      </div>

      {submissionError && (
        <div className="wizard-inline-error" role="alert">
          {tr('wizards.onboarding.task.completeFailed', { error: submissionError })}
        </div>
      )}

      <div className="wizard-nav">
        <button className="btn btn-ghost" onClick={onBack} type="button">
          {tr('wizards.common.back')}
        </button>
        <div className="wizard-nav-right">
          <button
            className="task-skip"
            onClick={onSkip}
            disabled={submitting}
            type="button"
          >
            {tr('wizards.onboarding.copy.step3Skip')}
          </button>
          <button
            className="btn btn-primary"
            onClick={onSubmit}
            disabled={submitting || taskText.trim().length === 0}
            type="button"
          >
            {submitting ? tr('wizards.onboarding.task.starting') : tr('wizards.onboarding.copy.step3Cta')}
          </button>
        </div>
      </div>
    </div>
  )
}

/* ═══════════════════════════════════════════
   Main Wizard
   ═══════════════════════════════════════════ */

interface WizardProps {
  onComplete?: () => void
}

export function Wizard({ onComplete }: WizardProps) {
  const setOnboardingComplete = useAppStore((s) => s.setOnboardingComplete)

  // Navigation
  const [step, setStep] = useState<WizardStep>('welcome')

  // Step 2: templates
  const [blueprints, setBlueprints] = useState<BlueprintTemplate[]>([])
  const [blueprintsLoading, setBlueprintsLoading] = useState(true)
  const [selectedBlueprint, setSelectedBlueprint] = useState<string | null>(null)

  // Step 3: identity
  const [company, setCompany] = useState('')
  const [description, setDescription] = useState('')
  const [priority, setPriority] = useState('')

  // Step 4: team
  const [agents, setAgents] = useState<BlueprintAgent[]>([])

  // Step 5: setup
  const [runtime, setRuntime] = useState<string>(RUNTIME_OPTIONS[0])
  const [apiKeys, setApiKeys] = useState<Record<string, string>>({})
  const [memoryBackend] = useState<MemoryBackend>('none')

  // Step 6: first task
  const [taskTemplates, setTaskTemplates] = useState<TaskTemplate[]>([])
  const [selectedTaskTemplate, setSelectedTaskTemplate] = useState<string | null>(null)
  const [taskText, setTaskText] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [submissionError, setSubmissionError] = useState<string | null>(null)

  // Fetch blueprints on mount
  useEffect(() => {
    let cancelled = false
    setBlueprintsLoading(true)

    get<{ templates?: BlueprintTemplate[] }>('/onboarding/blueprints')
      .then((data) => {
        if (cancelled) return
        const tpls = data.templates ?? []
        setBlueprints(tpls)

        // Also extract task templates if present
        const tasks: TaskTemplate[] = []
        for (const t of tpls) {
          if ((t as unknown as Record<string, unknown>).tasks) {
            const arr = (t as unknown as Record<string, TaskTemplate[]>).tasks
            tasks.push(...arr)
          }
        }
        if (tasks.length > 0) {
          setTaskTemplates(tasks)
        }
      })
      .catch(() => {
        // Endpoint may not exist yet; continue with empty list
      })
      .finally(() => {
        if (!cancelled) setBlueprintsLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [])

  // When a blueprint is selected, populate agents
  useEffect(() => {
    if (selectedBlueprint === null) {
      setAgents([])
      return
    }
    const bp = blueprints.find((b) => b.id === selectedBlueprint)
    if (bp?.agents) {
      setAgents(
        bp.agents.map((a) => ({
          ...a,
          checked: a.checked !== false,
        })),
      )
    } else {
      setAgents([])
    }
  }, [selectedBlueprint, blueprints])

  // Navigation helpers
  const goTo = useCallback((target: WizardStep) => {
    setStep(target)
  }, [])

  const nextStep = useCallback(() => {
    const idx = STEP_ORDER.indexOf(step)
    if (idx < STEP_ORDER.length - 1) {
      setStep(STEP_ORDER[idx + 1])
    }
  }, [step])

  const prevStep = useCallback(() => {
    const idx = STEP_ORDER.indexOf(step)
    if (idx > 0) {
      setStep(STEP_ORDER[idx - 1])
    }
  }, [step])

  // Toggle agent selection
  const toggleAgent = useCallback((slug: string) => {
    setAgents((prev) =>
      prev.map((a) =>
        a.slug === slug ? { ...a, checked: !a.checked } : a,
      ),
    )
  }, [])

  // API key handler
  const handleApiKeyChange = useCallback((key: string, value: string) => {
    setApiKeys((prev) => ({ ...prev, [key]: value }))
  }, [])

  // Complete onboarding
  const finishOnboarding = useCallback(
    async (skipTask: boolean) => {
      setSubmitting(true)
      setSubmissionError(null)
      try {
        const providedApiKeys = Object.fromEntries(
          Object.entries(apiKeys).filter(([, value]) => value.trim().length > 0),
        )
        // Persist memory backend selection first so the broker reads it on
        // next launch. Fire-and-forget — a failure here should not block
        // completing onboarding.
        post('/config', { memory_backend: 'none' }).catch(() => {})

        // Post the onboarding payload. Body shape is historical; the broker
        // currently only acts on {task, skip_task} but the extra fields are
        // forward-compatible.
        await post('/onboarding/complete', {
          company,
          description,
          priority,
          runtime,
          memory_backend: 'none',
          blueprint: selectedBlueprint,
          agents: agents.filter((a) => a.checked).map((a) => a.slug),
          api_keys: providedApiKeys,
          task: skipTask ? '' : taskText.trim(),
          skip_task: skipTask,
        })
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Failed to complete onboarding'
        setSubmissionError(message)
        setSubmitting(false)
        return
      }

      setSubmitting(false)
      setOnboardingComplete(true)
      onComplete?.()
    },
    [
      company,
      description,
      priority,
      runtime,
      memoryBackend,
      selectedBlueprint,
      agents,
      apiKeys,
      taskText,
      setSubmissionError,
      setOnboardingComplete,
      onComplete,
    ],
  )

  return (
    <div className="wizard-container">
      <div className="wizard-body">
        <ProgressDots current={step} />

        {step === 'welcome' && (
          <WelcomeStep onNext={() => goTo('templates')} />
        )}

        {step === 'templates' && (
          <TemplatesStep
            templates={blueprints}
            loading={blueprintsLoading}
            selected={selectedBlueprint}
            onSelect={setSelectedBlueprint}
            onNext={nextStep}
            onBack={prevStep}
          />
        )}

        {step === 'identity' && (
          <IdentityStep
            company={company}
            description={description}
            priority={priority}
            onChangeCompany={setCompany}
            onChangeDescription={setDescription}
            onChangePriority={setPriority}
            onNext={nextStep}
            onBack={prevStep}
          />
        )}

        {step === 'team' && (
          <TeamStep
            agents={agents}
            onToggle={toggleAgent}
            onNext={nextStep}
            onBack={prevStep}
          />
        )}

        {step === 'setup' && (
          <SetupStep
            runtime={runtime}
            onChangeRuntime={setRuntime}
            apiKeys={apiKeys}
            onChangeApiKey={handleApiKeyChange}
            memoryBackend={memoryBackend}
            onNext={nextStep}
            onBack={prevStep}
          />
        )}

        {step === 'task' && (
          <TaskStep
            taskTemplates={taskTemplates}
            selectedTaskTemplate={selectedTaskTemplate}
            onSelectTaskTemplate={setSelectedTaskTemplate}
            taskText={taskText}
            onChangeTaskText={setTaskText}
            onSkip={() => finishOnboarding(true)}
            onSubmit={() => finishOnboarding(false)}
            onBack={prevStep}
            submitting={submitting}
            submissionError={submissionError}
          />
        )}
      </div>
    </div>
  )
}
