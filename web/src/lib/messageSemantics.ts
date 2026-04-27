import type { Message } from '../api/client'

const AUTOMATION_ACTORS = new Set(['nex', 'watchdog', 'wuphf', 'scheduler'])
const NON_SUBSTANTIVE_ACTORS = new Set(['', 'system', ...AUTOMATION_ACTORS])

export function normalizeMessageActor(value?: string | null): string {
  return (value || '').trim().toLowerCase()
}

export function isAutomationMessage(message: Message): boolean {
  const kind = normalizeMessageActor(message.kind)
  if (kind === 'automation') return true
  return AUTOMATION_ACTORS.has(normalizeMessageActor(message.from))
}

export function isSubstantiveMessage(message: Message): boolean {
  const content = (message.content || '').trim()
  if (!content || content.startsWith('[STATUS]')) return false
  if (isAutomationMessage(message)) return false
  return !NON_SUBSTANTIVE_ACTORS.has(normalizeMessageActor(message.from))
}

export function automationSourceLabel(messages: Message[]): string {
  const latest = messages[messages.length - 1]
  const explicit = findLastMessageValue(messages, (message) => message.source_label) ||
    findLastMessageValue(messages, (message) => message.source) ||
    latest?.title ||
    'automation'
  return explicit.trim()
}

export function automationPreview(messages: Message[]): string {
  const latest = messages[messages.length - 1]
  const parts = [latest?.title, latest?.content].filter(Boolean)
  return truncateMessageText(parts.join(': '), 180)
}

export function truncateMessageText(value: string, maxLength: number): string {
  const trimmed = value.trim()
  if (trimmed.length <= maxLength) return trimmed
  return `${trimmed.slice(0, maxLength - 1).trimEnd()}\u2026`
}

function findLastMessageValue(messages: Message[], pick: (message: Message) => string | undefined): string {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const value = (pick(messages[index]) || '').trim()
    if (value) return value
  }
  return ''
}
