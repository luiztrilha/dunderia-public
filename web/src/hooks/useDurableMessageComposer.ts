import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { extractTaggedSlugs } from '../components/messages/mentions'
import { postMessage, type Message, type PostMessageResponse } from '../api/client'

const DRAFT_STORAGE_KEY = 'dunderia.messageDrafts.v1'
const OUTBOX_STORAGE_KEY = 'dunderia.messageOutbox.v1'

type DeliveryKind = 'idle' | 'draft' | 'sending' | 'persisted' | 'failed'

export interface ComposerDeliveryState {
  kind: DeliveryKind
  messageId?: string
  error?: string
}

interface OutboxEntry {
  clientId: string
  scopeKey: string
  channel: string
  replyTo?: string
  content: string
  tagged: string[]
  status: 'sending' | 'failed'
  attempts: number
  createdAt: string
  updatedAt: string
  lastError?: string
}

function canUseStorage(): boolean {
  return typeof window !== 'undefined' && typeof window.localStorage !== 'undefined'
}

function readDrafts(): Record<string, string> {
  if (!canUseStorage()) return {}
  try {
    const raw = window.localStorage.getItem(DRAFT_STORAGE_KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw)
    return parsed && typeof parsed === 'object' ? parsed as Record<string, string> : {}
  } catch {
    return {}
  }
}

function writeDrafts(next: Record<string, string>): void {
  if (!canUseStorage()) return
  window.localStorage.setItem(DRAFT_STORAGE_KEY, JSON.stringify(next))
}

function readOutbox(): OutboxEntry[] {
  if (!canUseStorage()) return []
  try {
    const raw = window.localStorage.getItem(OUTBOX_STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed as OutboxEntry[] : []
  } catch {
    return []
  }
}

function writeOutbox(next: OutboxEntry[]): void {
  if (!canUseStorage()) return
  window.localStorage.setItem(OUTBOX_STORAGE_KEY, JSON.stringify(next))
}

function upsertOutboxEntry(entry: OutboxEntry): void {
  const outbox = readOutbox().filter((item) => item.clientId !== entry.clientId)
  outbox.push(entry)
  writeOutbox(outbox)
}

function removeOutboxEntry(clientId: string): void {
  writeOutbox(readOutbox().filter((item) => item.clientId !== clientId))
}

function removeScopeOutbox(scopeKey: string): void {
  writeOutbox(readOutbox().filter((item) => item.scopeKey !== scopeKey))
}

function latestScopeOutbox(scopeKey: string): OutboxEntry | null {
  const matches = readOutbox()
    .filter((item) => item.scopeKey === scopeKey)
    .sort((a, b) => a.updatedAt.localeCompare(b.updatedAt))
  return matches[matches.length - 1] ?? null
}

function setDraft(scopeKey: string, text: string): void {
  const drafts = readDrafts()
  if (text) {
    drafts[scopeKey] = text
  } else {
    delete drafts[scopeKey]
  }
  writeDrafts(drafts)
}

function getDraft(scopeKey: string): string {
  return readDrafts()[scopeKey] ?? ''
}

function makeScopeKey(channel: string, replyTo?: string | null): string {
  return `${(channel || 'general').trim().toLowerCase()}::${(replyTo ?? 'root').trim().toLowerCase()}`
}

function makeClientId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `client-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

export function useDurableMessageComposer({
  channel,
  replyTo,
  memberSlugs,
  onPersisted,
}: {
  channel: string
  replyTo?: string | null
  memberSlugs: string[]
  onPersisted?: (response: PostMessageResponse) => void
}) {
  const scopeKey = useMemo(() => makeScopeKey(channel, replyTo), [channel, replyTo])
  const [text, setTextState] = useState('')
  const [delivery, setDelivery] = useState<ComposerDeliveryState>({ kind: 'idle' })
  const [isSending, setIsSending] = useState(false)
  const autoResumedScopeRef = useRef<string | null>(null)

  const setText = useCallback((value: string) => {
    setTextState(value)
    setDelivery((current) => {
      if (current.kind === 'sending') return current
      return value.trim() ? { kind: 'draft' } : { kind: 'idle' }
    })
  }, [])

  const clearComposer = useCallback(() => {
    setTextState('')
    setDraft(scopeKey, '')
    removeScopeOutbox(scopeKey)
    setDelivery({ kind: 'idle' })
  }, [scopeKey])

  const submitEntry = useCallback(async (entry: OutboxEntry): Promise<PostMessageResponse> => {
    setIsSending(true)
    const nextEntry: OutboxEntry = {
      ...entry,
      status: 'sending',
      attempts: entry.attempts + 1,
      updatedAt: new Date().toISOString(),
    }
    upsertOutboxEntry(nextEntry)
    setDelivery({ kind: 'sending' })
    try {
      const response = await postMessage(
        nextEntry.content,
        nextEntry.channel,
        nextEntry.replyTo,
        nextEntry.tagged,
        nextEntry.clientId,
      )
      removeOutboxEntry(nextEntry.clientId)
      removeScopeOutbox(nextEntry.scopeKey)
      setDraft(nextEntry.scopeKey, '')
      setTextState((current) => (current.trim() === nextEntry.content.trim() ? '' : current))
      setDelivery({ kind: 'persisted', messageId: response.message?.id ?? response.id })
      onPersisted?.(response)
      return response
    } catch (err) {
      const message = err instanceof Error ? err.message : 'failed to persist message'
      upsertOutboxEntry({
        ...nextEntry,
        status: 'failed',
        updatedAt: new Date().toISOString(),
        lastError: message,
      })
      setDelivery({ kind: 'failed', error: message })
      throw err
    } finally {
      setIsSending(false)
    }
  }, [onPersisted])

  const sendMessage = useCallback(async () => {
    const trimmed = text.trim()
    if (!trimmed || isSending) {
      return null
    }
    removeScopeOutbox(scopeKey)
    const entry: OutboxEntry = {
      clientId: makeClientId(),
      scopeKey,
      channel: channel || 'general',
      replyTo: replyTo ?? undefined,
      content: trimmed,
      tagged: extractTaggedSlugs(trimmed, memberSlugs),
      status: 'sending',
      attempts: 0,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    }
    return submitEntry(entry)
  }, [channel, isSending, memberSlugs, replyTo, scopeKey, submitEntry, text])

  const retryLastFailed = useCallback(async () => {
    const entry = latestScopeOutbox(scopeKey)
    if (!entry) {
      return sendMessage()
    }
    return submitEntry(entry)
  }, [scopeKey, sendMessage, submitEntry])

  useEffect(() => {
    const draft = getDraft(scopeKey)
    const entry = latestScopeOutbox(scopeKey)
    const nextText = draft || entry?.content || ''
    setTextState(nextText)
    if (entry?.status === 'failed') {
      setDelivery({ kind: 'failed', error: entry.lastError })
    } else if (entry?.status === 'sending') {
      setDelivery({ kind: 'sending' })
    } else if (nextText.trim()) {
      setDelivery({ kind: 'draft' })
    } else {
      setDelivery({ kind: 'idle' })
    }
    autoResumedScopeRef.current = null
  }, [scopeKey])

  useEffect(() => {
    setDraft(scopeKey, text)
  }, [scopeKey, text])

  useEffect(() => {
    const pending = latestScopeOutbox(scopeKey)
    if (!pending || pending.status !== 'sending' || isSending) {
      return
    }
    if (autoResumedScopeRef.current === scopeKey) {
      return
    }
    autoResumedScopeRef.current = scopeKey
    void submitEntry(pending).catch(() => {})
  }, [isSending, scopeKey, submitEntry])

  return {
    text,
    setText,
    clearComposer,
    sendMessage,
    retryLastFailed,
    delivery,
    isSending,
  }
}

export type { Message }
