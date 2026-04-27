import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import i18n from '../../i18n/config'
import { getConfig, updateConfig, type LLMProvider } from '../../api/client'
import { showNotice } from './Toast'
import { confirm } from './ConfirmDialog'

let requestOpen: (() => void) | null = null

/** Imperatively open the provider switcher from anywhere. */
export function openProviderSwitcher() {
  if (!requestOpen) {
    showNotice(i18n.t('provider.unavailable'), 'error')
    return
  }
  requestOpen()
}

interface ProviderOption {
  id: LLMProvider
  nameKey: string
  descKey: string
}

const PROVIDERS: ProviderOption[] = [
  { id: 'claude-code', nameKey: 'provider.options.claudeCode.name', descKey: 'provider.options.claudeCode.desc' },
  { id: 'codex', nameKey: 'provider.options.codex.name', descKey: 'provider.options.codex.desc' },
  { id: 'gemini', nameKey: 'provider.options.gemini.name', descKey: 'provider.options.gemini.desc' },
  { id: 'ollama', nameKey: 'provider.options.ollama.name', descKey: 'provider.options.ollama.desc' },
  { id: 'gemini-vertex', nameKey: 'provider.options.geminiVertex.name', descKey: 'provider.options.geminiVertex.desc' },
]

export function ProviderSwitcherHost() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [current, setCurrent] = useState<LLMProvider | null>(null)
  const [loading, setLoading] = useState(false)
  const [pending, setPending] = useState<LLMProvider | null>(null)
  const queryClient = useQueryClient()

  useEffect(() => {
    requestOpen = () => {
      setOpen(true)
      setLoading(true)
      getConfig()
        .then((cfg) => setCurrent(cfg.llm_provider ?? 'claude-code'))
        .catch(() => setCurrent('claude-code'))
        .finally(() => setLoading(false))
    }
    return () => {
      if (requestOpen !== null) requestOpen = null
    }
  }, [])

  useEffect(() => {
    if (!open) return
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [open])

  if (!open) return null

  async function switchTo(p: ProviderOption) {
    if (!current || p.id === current) return
    const providerName = t(p.nameKey)
    confirm({
      title: t('provider.confirmTitle'),
      message: t('provider.confirmBody', { name: providerName }),
      confirmLabel: t('provider.confirmCta'),
      onConfirm: async () => {
        setPending(p.id)
        try {
          await updateConfig({ llm_provider: p.id })
          await queryClient.invalidateQueries({ queryKey: ['config'] })
          await queryClient.invalidateQueries({ queryKey: ['health'] })
          setCurrent(p.id)
          showNotice(t('provider.switched', { name: providerName }), 'success')
          setOpen(false)
        } catch (err: unknown) {
          const message = err instanceof Error ? err.message : t('provider.switchFailedFallback')
          showNotice(t('provider.switchFailed', { error: message }), 'error')
        } finally {
          setPending(null)
        }
      },
    })
  }

  return (
    <div
      className="provider-overlay"
      onClick={(e) => {
        if (e.target === e.currentTarget) setOpen(false)
      }}
      role="dialog"
      aria-modal="true"
      aria-labelledby="provider-title"
    >
      <div className="provider-panel card">
        <h3 id="provider-title" className="provider-title">{t('provider.title')}</h3>
        {loading ? (
          <p className="provider-loading">{t('provider.loading')}</p>
        ) : (
          <div className="provider-options">
            {PROVIDERS.map((p) => {
              const isActive = current === p.id
              const isPending = pending === p.id
              return (
                <button
                  key={p.id}
                  type="button"
                  className={`provider-option${isActive ? ' active' : ''}`}
                  onClick={() => switchTo(p)}
                  disabled={isActive || isPending}
                >
                  <div className="provider-option-text">
                    <div className="provider-option-name">{t(p.nameKey)}</div>
                    <div className="provider-option-desc">{t(p.descKey)}</div>
                  </div>
                  {isActive && <span className="provider-option-check">{'\u2713'}</span>}
                  {isPending && <span className="provider-option-check">...</span>}
                </button>
              )
            })}
          </div>
        )}
        <div className="provider-footer">
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => setOpen(false)}>
            {t('provider.close')}
          </button>
        </div>
      </div>
    </div>
  )
}
