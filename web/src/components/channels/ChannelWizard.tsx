import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { createChannel, generateChannel } from '../../api/client'
import { useAppStore } from '../../stores/app'
import { useQueryClient } from '@tanstack/react-query'
import { usePersistentDraft } from '../../hooks/usePersistentDraft'

type WizardMode = 'describe' | 'manual'

interface ManualFormData {
  slug: string
  name: string
  description: string
}

interface ChannelWizardDraft {
  mode: WizardMode
  prompt: string
  manual: ManualFormData
  slugEdited: boolean
}

const INITIAL_MANUAL: ManualFormData = {
  slug: '',
  name: '',
  description: '',
}

const INITIAL_DRAFT: ChannelWizardDraft = {
  mode: 'describe',
  prompt: '',
  manual: INITIAL_MANUAL,
  slugEdited: false,
}

function slugify(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

interface ChannelWizardProps {
  open: boolean
  onClose: () => void
}

export function ChannelWizard({ open, onClose }: ChannelWizardProps) {
  const { t } = useTranslation()
  const { value: draft, setValue: setDraft, clear: clearDraft } = usePersistentDraft<ChannelWizardDraft>(
    'dunderia.channelWizardDraft.v1',
    INITIAL_DRAFT,
  )
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const setCurrentChannel = useAppStore((s) => s.setCurrentChannel)
  const queryClient = useQueryClient()

  const mode = draft.mode
  const prompt = draft.prompt
  const manual = draft.manual
  const slugEdited = draft.slugEdited

  const updateManualField = useCallback(
    <K extends keyof ManualFormData>(field: K, value: ManualFormData[K]) => {
      setDraft((prev) => {
        const next = { ...prev.manual, [field]: value }
        if (field === 'name' && !prev.slugEdited) {
          next.slug = slugify(value as string)
        }
        return { ...prev, manual: next }
      })
      setError(null)
    },
    [setDraft],
  )

  function resetState() {
    clearDraft()
    setError(null)
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

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setSubmitting(true)
    setError(null)

    try {
      let newSlug: string

      if (mode === 'describe') {
        if (!prompt.trim()) {
          setError(t('wizards.channel.describeEmpty'))
          setSubmitting(false)
          return
        }
        const channel = await generateChannel(prompt.trim())
        await createChannel(channel.slug, channel.name || channel.slug, channel.description || '')
        newSlug = channel.slug
      } else {
        if (!manual.slug.trim() || !manual.name.trim()) {
          setError(t('wizards.channel.manualRequired'))
          setSubmitting(false)
          return
        }
        await createChannel(manual.slug, manual.name, manual.description)
        newSlug = manual.slug
      }

      await queryClient.invalidateQueries({ queryKey: ['channels'] })
      resetState()
      onClose()
      setCurrentChannel(newSlug)
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : t('wizards.channel.createFailed')
      setError(message)
    } finally {
      setSubmitting(false)
    }
  }

  if (!open) return null

  return (
    <div className="channel-wizard-overlay" onClick={handleOverlayClick}>
      <div className="channel-wizard-modal card">
        <div className="channel-wizard-title">{t('wizards.channel.title')}</div>

        {/* Mode toggle */}
        <div className="channel-wizard-tabs">
          <button
            type="button"
            className={`channel-wizard-tab${mode === 'describe' ? ' active' : ''}`}
            onClick={() => { setDraft({ ...draft, mode: 'describe' }); setError(null) }}
          >
            {t('wizards.common.describeTab')}
          </button>
          <button
            type="button"
            className={`channel-wizard-tab${mode === 'manual' ? ' active' : ''}`}
            onClick={() => { setDraft({ ...draft, mode: 'manual' }); setError(null) }}
          >
            {t('wizards.common.manualTab')}
          </button>
        </div>

        <form className="channel-wizard-form" onSubmit={handleSubmit}>
          {mode === 'describe' ? (
            <div className="channel-wizard-field">
              <label className="label" htmlFor="channel-prompt">
                {t('wizards.channel.describeLabel')}
              </label>
              <textarea
                id="channel-prompt"
                className="input channel-wizard-textarea"
                placeholder={t('wizards.channel.describePlaceholder')}
                value={prompt}
                onChange={(e) => { setDraft({ ...draft, prompt: e.target.value }); setError(null) }}
                rows={3}
                autoFocus
              />
              <span className="channel-wizard-hint">
                {t('wizards.channel.describeHint')}
              </span>
            </div>
          ) : (
            <>
              <div className="channel-wizard-field">
                <label className="label" htmlFor="channel-name">{t('wizards.channel.fields.name')}</label>
                <input
                  id="channel-name"
                  className="input"
                  type="text"
                  placeholder={t('wizards.channel.fields.namePlaceholder')}
                  value={manual.name}
                  onChange={(e) => updateManualField('name', e.target.value)}
                  autoFocus
                />
              </div>
              <div className="channel-wizard-field">
                <label className="label" htmlFor="channel-slug">{t('wizards.channel.fields.slug')}</label>
                <input
                  id="channel-slug"
                  className="input"
                  type="text"
                  placeholder={t('wizards.channel.fields.slugPlaceholder')}
                  value={manual.slug}
                  onChange={(e) => {
                    setDraft((prev) => ({ ...prev, slugEdited: true }))
                    updateManualField('slug', e.target.value)
                  }}
                />
              </div>
              <div className="channel-wizard-field">
                <label className="label" htmlFor="channel-description">{t('wizards.channel.fields.description')}</label>
                <input
                  id="channel-description"
                  className="input"
                  type="text"
                  placeholder={t('wizards.channel.fields.descriptionPlaceholder')}
                  value={manual.description}
                  onChange={(e) => updateManualField('description', e.target.value)}
                />
              </div>
            </>
          )}

          {error && <div className="channel-wizard-error">{error}</div>}

          <div className="channel-wizard-footer">
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
              disabled={submitting}
            >
              {submitting
                ? mode === 'describe' ? t('wizards.common.generating') : t('wizards.common.creating')
                : mode === 'describe' ? t('wizards.common.generate') : t('wizards.common.create')}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

/**
 * Hook to manage wizard open/close state from any component.
 * Usage:
 *   const { open, show, hide } = useChannelWizard()
 *   <button onClick={show}>New Channel</button>
 *   <ChannelWizard open={open} onClose={hide} />
 */
export function useChannelWizard() {
  const [open, setOpen] = useState(false)
  const show = useCallback(() => setOpen(true), [])
  const hide = useCallback(() => setOpen(false), [])
  return { open, show, hide }
}
