import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  post,
  generateAgent,
  formatProviderLabel,
  type PerAgentProviderKind,
} from '../../api/client'
import { useQueryClient } from '@tanstack/react-query'
import { usePersistentDraft } from '../../hooks/usePersistentDraft'

type WizardMode = 'describe' | 'manual'
type RuntimeChoice = 'office-default' | PerAgentProviderKind

interface AgentFormData {
  name: string
  slug: string
  role: string
  emoji: string
  provider: RuntimeChoice
  expertise: string
}

interface AgentWizardDraft {
  mode: WizardMode
  prompt: string
  form: AgentFormData
  slugEdited: boolean
}

const INITIAL_FORM: AgentFormData = {
  name: '',
  slug: '',
  role: '',
  emoji: '',
  provider: 'office-default',
  expertise: '',
}

const INITIAL_DRAFT: AgentWizardDraft = {
  mode: 'describe',
  prompt: '',
  form: INITIAL_FORM,
  slugEdited: false,
}

interface ProviderChoice {
  value: RuntimeChoice
  label: string
  i18nKey?: string
}

const PROVIDERS: ProviderChoice[] = [
  { value: 'office-default', label: 'Use office default', i18nKey: 'wizards.agent.providers.officeDefault' },
  { value: 'claude-code', label: formatProviderLabel('claude-code') },
  { value: 'codex', label: formatProviderLabel('codex') },
  { value: 'gemini', label: formatProviderLabel('gemini') },
  { value: 'ollama', label: formatProviderLabel('ollama') },
  { value: 'gemini-vertex', label: formatProviderLabel('gemini-vertex') },
  { value: 'openclaude', label: formatProviderLabel('openclaude') },
]

function slugify(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

interface AgentWizardProps {
  open: boolean
  onClose: () => void
  onCreated?: () => void
}

export function AgentWizard({ open, onClose, onCreated }: AgentWizardProps) {
  const { t } = useTranslation()
  const { value: draft, setValue: setDraft, clear: clearDraft } = usePersistentDraft<AgentWizardDraft>(
    'dunderia.agentWizardDraft.v1',
    INITIAL_DRAFT,
  )
  const [generating, setGenerating] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const queryClient = useQueryClient()
  const mode = draft.mode
  const prompt = draft.prompt
  const form = draft.form
  const slugEdited = draft.slugEdited

  async function handleGenerate() {
    const trimmed = prompt.trim()
    if (!trimmed) {
      setError(t('wizards.agent.describeEmpty'))
      return
    }
    setGenerating(true)
    setError(null)
    try {
      const tmpl = await generateAgent(trimmed)
      const generatedSlug = tmpl.slug || ''
      setDraft({
        ...draft,
        mode: 'manual',
        prompt: trimmed,
        form: {
        name: tmpl.name || '',
        slug: generatedSlug,
        role: tmpl.role || '',
        emoji: tmpl.emoji || '',
        provider: tmpl.provider ?? 'office-default',
        expertise: (tmpl.expertise || []).join(', '),
        },
        slugEdited: generatedSlug.length > 0,
      })
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : t('wizards.agent.generateFailed')
      setError(message)
    } finally {
      setGenerating(false)
    }
  }

  const updateField = useCallback(
    <K extends keyof AgentFormData>(field: K, value: AgentFormData[K]) => {
      setDraft((prev) => {
        const next = { ...prev.form, [field]: value }
        if (field === 'name' && !prev.slugEdited) {
          next.slug = slugify(value as string)
        }
        return { ...prev, form: next }
      })
      setError(null)
    },
    [setDraft],
  )

  const expertiseTags = useMemo(() => {
    return form.expertise
      .split(',')
      .map((t) => t.trim())
      .filter(Boolean)
  }, [form.expertise])

  const canSubmit = form.name.trim().length > 0 && form.slug.trim().length > 0

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()

    if (!canSubmit) return
    setSubmitting(true)
    setError(null)

    try {
      const provider =
        form.provider === 'office-default'
          ? undefined
          : { kind: form.provider }

      const body = {
        action: 'create',
        slug: form.slug,
        name: form.name,
        role: form.role || undefined,
        emoji: form.emoji || undefined,
        provider,
        expertise: expertiseTags.length > 0 ? expertiseTags : undefined,
      }

      await post('/office-members', body)
      await queryClient.invalidateQueries({ queryKey: ['office-members'] })

      clearDraft()
      onCreated?.()
      onClose()
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : t('wizards.agent.createFailed')
      setError(message)
    } finally {
      setSubmitting(false)
    }
  }

  function handleCancel() {
    setError(null)
    onClose()
  }

  function handleOverlayClick(e: React.MouseEvent) {
    if (e.target === e.currentTarget) {
      handleCancel()
    }
  }

  if (!open) return null

  return (
    <div className="agent-wizard-overlay" onClick={handleOverlayClick}>
      <div className="agent-wizard-modal card">
        <div className="agent-wizard-title">{t('wizards.agent.title')}</div>

        {/* Mode toggle */}
        <div className="channel-wizard-tabs" style={{ marginBottom: 16 }}>
          <button
            type="button"
            className={`channel-wizard-tab${mode === 'describe' ? ' active' : ''}`}
            onClick={() => { setDraft((prev) => ({ ...prev, mode: 'describe' })); setError(null) }}
          >
            {t('wizards.common.describeTab')}
          </button>
          <button
            type="button"
            className={`channel-wizard-tab${mode === 'manual' ? ' active' : ''}`}
            onClick={() => { setDraft((prev) => ({ ...prev, mode: 'manual' })); setError(null) }}
          >
            {t('wizards.common.manualTab')}
          </button>
        </div>

        {mode === 'describe' ? (
          <div className="agent-wizard-form">
            <div className="agent-wizard-field">
              <label className="label" htmlFor="agent-prompt">
                {t('wizards.agent.describeLabel')}
              </label>
              <textarea
                id="agent-prompt"
                className="input"
                placeholder={t('wizards.agent.describePlaceholder')}
                value={prompt}
                onChange={(e) => { setDraft((prev) => ({ ...prev, prompt: e.target.value })); setError(null) }}
                rows={3}
                style={{ minHeight: 80, resize: 'vertical', padding: '10px 12px', lineHeight: 1.5 }}
                autoFocus
              />
              <span style={{ fontSize: 11, color: 'var(--text-tertiary)', marginTop: 6, display: 'block' }}>
                {t('wizards.agent.describeHint')}
              </span>
            </div>

            {error && <div className="agent-wizard-error">{error}</div>}

            <div className="agent-wizard-footer">
              <button
                type="button"
                className="btn btn-ghost btn-sm"
                onClick={handleCancel}
                disabled={generating}
              >
                {t('wizards.common.cancel')}
              </button>
              <button
                type="button"
                className="btn btn-primary btn-sm"
                onClick={handleGenerate}
                disabled={generating || !prompt.trim()}
              >
                {generating ? t('wizards.common.generating') : t('wizards.common.generate')}
              </button>
            </div>
          </div>
        ) : (
        <form className="agent-wizard-form" onSubmit={handleSubmit}>
          {/* Name */}
          <div className="agent-wizard-field">
            <label className="label" htmlFor="agent-name">{t('wizards.agent.fields.name')}</label>
            <input
              id="agent-name"
              className="input"
              type="text"
              placeholder={t('wizards.agent.fields.namePlaceholder')}
              value={form.name}
              onChange={(e) => updateField('name', e.target.value)}
              autoFocus
            />
          </div>

          {/* Slug */}
          <div className="agent-wizard-field">
            <label className="label" htmlFor="agent-slug">{t('wizards.agent.fields.slug')}</label>
            <input
              id="agent-slug"
              className="input"
              type="text"
              placeholder={t('wizards.agent.fields.slugPlaceholder')}
              value={form.slug}
              onChange={(e) => {
                setDraft((prev) => ({ ...prev, slugEdited: true }))
                updateField('slug', e.target.value)
              }}
            />
          </div>

          {/* Role */}
          <div className="agent-wizard-field">
            <label className="label" htmlFor="agent-role">{t('wizards.agent.fields.role')}</label>
            <input
              id="agent-role"
              className="input"
              type="text"
              placeholder={t('wizards.agent.fields.rolePlaceholder')}
              value={form.role}
              onChange={(e) => updateField('role', e.target.value)}
            />
          </div>

          {/* Emoji */}
          <div className="agent-wizard-field">
            <label className="label" htmlFor="agent-emoji">{t('wizards.agent.fields.emoji')}</label>
            <input
              id="agent-emoji"
              className="input"
              type="text"
              placeholder={t('wizards.agent.fields.emojiPlaceholder')}
              value={form.emoji}
              onChange={(e) => updateField('emoji', e.target.value)}
              maxLength={4}
              style={{ width: 80 }}
            />
          </div>

          {/* Provider */}
          <div className="agent-wizard-field">
            <label className="label" htmlFor="agent-provider">{t('wizards.agent.fields.provider')}</label>
            <select
              id="agent-provider"
              className="input"
              value={form.provider}
              onChange={(e) => updateField('provider', e.target.value as RuntimeChoice)}
            >
              {PROVIDERS.map((p) => (
                <option key={p.value} value={p.value}>{p.i18nKey ? t(p.i18nKey) : p.label}</option>
              ))}
            </select>
            <span style={{ fontSize: 11, color: 'var(--text-tertiary)', marginTop: 6, display: 'block' }}>
              {t('wizards.agent.fields.providerHint')}
            </span>
          </div>

          {/* Expertise */}
          <div className="agent-wizard-field">
            <label className="label" htmlFor="agent-expertise">
              {t('wizards.agent.fields.expertise')} <span style={{ fontWeight: 400, color: 'var(--text-tertiary)' }}>{t('wizards.agent.fields.expertiseHint')}</span>
            </label>
            <input
              id="agent-expertise"
              className="input"
              type="text"
              placeholder={t('wizards.agent.fields.expertisePlaceholder')}
              value={form.expertise}
              onChange={(e) => updateField('expertise', e.target.value)}
            />
            {expertiseTags.length > 0 && (
              <div className="agent-panel-tags" style={{ marginTop: 6 }}>
                {expertiseTags.map((tag) => (
                  <span key={tag} className="agent-panel-tag">{tag}</span>
                ))}
              </div>
            )}
          </div>

          {error && <div className="agent-wizard-error">{error}</div>}

          {/* Footer */}
          <div className="agent-wizard-footer">
            <button
              type="button"
              className="btn btn-ghost btn-sm"
              onClick={handleCancel}
              disabled={submitting}
            >
              {t('wizards.common.cancel')}
            </button>
            <button
              type="submit"
              className="btn btn-primary btn-sm"
              disabled={!canSubmit || submitting}
            >
              {submitting ? t('wizards.common.creating') : t('wizards.common.create')}
            </button>
          </div>
        </form>
        )}
      </div>
    </div>
  )
}

/**
 * Hook to manage wizard open/close state from any component.
 * Usage:
 *   const { open, show, hide } = useAgentWizard()
 *   <button onClick={show}>New Agent</button>
 *   <AgentWizard open={open} onClose={hide} />
 */
export function useAgentWizard() {
  const [open, setOpen] = useState(false)
  const show = useCallback(() => setOpen(true), [])
  const hide = useCallback(() => setOpen(false), [])
  return { open, show, hide }
}
