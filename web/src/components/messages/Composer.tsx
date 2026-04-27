import { useRef, useState, useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import { useOfficeMembers } from '../../hooks/useMembers'
import { useAppStore } from '../../stores/app'
import { showNotice } from '../ui/Toast'
import { Autocomplete, applyAutocomplete, type AutocompleteItem } from './Autocomplete'
import { dispatchSlashCommand } from '../../lib/slashCommands'
import { useDurableMessageComposer } from '../../hooks/useDurableMessageComposer'
import { useRequests } from '../../hooks/useRequests'

export function Composer() {
  const { t } = useTranslation()
  const currentChannel = useAppStore((s) => s.currentChannel)
  const setCurrentApp = useAppStore((s) => s.setCurrentApp)
  const setActiveThreadId = useAppStore((s) => s.setActiveThreadId)
  const setActiveThreadReplyTo = useAppStore((s) => s.setActiveThreadReplyTo)
  const { data: officeMembers = [] } = useOfficeMembers()
  const [caret, setCaret] = useState(0)
  const [acItems, setAcItems] = useState<AutocompleteItem[]>([])
  const [acIdx, setAcIdx] = useState(0)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const queryClient = useQueryClient()
  const { blockingPending, pending } = useRequests()
  const memberSlugs = useMemo(() => officeMembers.map((member) => member.slug), [officeMembers])
  const {
    text,
    setText,
    clearComposer,
    sendMessage,
    retryLastFailed,
    delivery,
    isSending,
  } = useDurableMessageComposer({
    channel: currentChannel,
    memberSlugs,
    onPersisted: () => {
      if (textareaRef.current) {
        textareaRef.current.style.height = 'auto'
      }
      queryClient.invalidateQueries({ queryKey: ['messages', currentChannel] })
    },
  })

  const pickAutocomplete = useCallback((item: AutocompleteItem) => {
    const next = applyAutocomplete(text, caret, item)
    setText(next.text)
    requestAnimationFrame(() => {
      const el = textareaRef.current
      if (!el) return
      el.focus()
      el.setSelectionRange(next.caret, next.caret)
      setCaret(next.caret)
    })
  }, [text, caret])

  const handleSend = useCallback(async () => {
    const trimmed = text.trim()
    if (!trimmed || isSending) return
    if (blockingPending) {
      showNotice(t('messages.composer.interviewBlock'), 'info')
      return
    }

    // Handle slash commands
    if (trimmed.startsWith('/')) {
      if (dispatchSlashCommand(trimmed, {
        currentChannel,
        setCurrentApp,
        setCurrentChannel: useAppStore.getState().setCurrentChannel,
        setLastMessageId: useAppStore.getState().setLastMessageId,
        setSearchOpen: useAppStore.getState().setSearchOpen,
        enterDM: useAppStore.getState().enterDM,
        clearActiveThread: () => {
          setActiveThreadId(null)
          setActiveThreadReplyTo(null)
        },
        onChannelCleared: () => {
          queryClient.invalidateQueries({ queryKey: ['requests'] })
        },
        resetRequiresConfirm: true,
        helpMode: 'composer',
      })) {
        clearComposer()
        return
      }
    }

    try {
      await sendMessage()
    } catch (err) {
      const message = err instanceof Error ? err.message : t('messages.composer.failedSend')
      if (/request pending|answer required/i.test(message)) {
        showNotice(t('messages.composer.interviewBlock'), 'info')
        return
      }
      showNotice(message, 'error')
    }
  }, [text, isSending, blockingPending, currentChannel, setActiveThreadId, setActiveThreadReplyTo, queryClient, clearComposer, sendMessage, t])

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (acItems.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setAcIdx((i) => (i + 1) % acItems.length)
        return
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        setAcIdx((i) => (i - 1 + acItems.length) % acItems.length)
        return
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault()
        const pick = acItems[acIdx] ?? acItems[0]
        if (pick) pickAutocomplete(pick)
        return
      }
      if (e.key === 'Escape') {
        e.preventDefault()
        setAcItems([])
        return
      }
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }, [handleSend, acItems, acIdx, pickAutocomplete])

  const handleAcItems = useCallback((items: AutocompleteItem[]) => {
    setAcItems(items)
    setAcIdx((idx) => Math.min(idx, Math.max(items.length - 1, 0)))
  }, [])

  const syncCaret = useCallback(() => {
    const el = textareaRef.current
    if (el) setCaret(el.selectionStart ?? 0)
  }, [])

  const handleInput = useCallback(() => {
    const el = textareaRef.current
    if (el) {
      el.style.height = 'auto'
      el.style.height = Math.min(el.scrollHeight, 120) + 'px'
    }
  }, [])

  const composerInnerClassName =
    'composer-inner'
    + (blockingPending ? ' composer-inner-blocked' : '')
    + (!blockingPending && pending.length > 0 ? ' composer-inner-waiting' : '')

  return (
    <div className="composer">
      <Autocomplete
        value={text}
        caret={caret}
        selectedIdx={acIdx}
        onItems={handleAcItems}
        onPick={pickAutocomplete}
      />
      {blockingPending ? (
        <div className="composer-blocker" role="status">
          <span className="composer-blocker-label">{t('messages.overlay.actionRequired')}</span>
          <span className="composer-blocker-copy">{t('messages.composer.blockedByInterview')}</span>
        </div>
      ) : null}
      <div className={composerInnerClassName}>
        <textarea
          ref={textareaRef}
          className="composer-input"
          placeholder={blockingPending ? t('messages.composer.blockedPlaceholder') : t('messages.composer.placeholder', { channel: currentChannel })}
          value={text}
          onChange={(e) => { setText(e.target.value); setCaret(e.target.selectionStart ?? 0); handleInput() }}
          onKeyDown={handleKeyDown}
          onKeyUp={syncCaret}
          onClick={syncCaret}
          disabled={Boolean(blockingPending)}
          rows={1}
        />
        <button
          className="composer-send"
          disabled={!text.trim() || isSending || Boolean(blockingPending)}
          onClick={handleSend}
          aria-label={t('messages.composer.sendAria')}
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="m22 2-7 20-4-9-9-4Z" />
            <path d="M22 2 11 13" />
          </svg>
        </button>
      </div>
      {delivery.kind !== 'idle' ? (
        <div className={`composer-status composer-status--${delivery.kind}`}>
          <span>
            {delivery.kind === 'draft' ? t('messages.composer.statusDraft') : null}
            {delivery.kind === 'sending' ? t('messages.composer.statusSending') : null}
            {delivery.kind === 'persisted' ? t('messages.composer.statusPersisted') : null}
            {delivery.kind === 'failed' ? t('messages.composer.statusFailed') : null}
          </span>
          {delivery.kind === 'failed' ? (
            <button className="composer-status-action" onClick={() => void retryLastFailed()}>
              {t('messages.composer.retryPersist')}
            </button>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
