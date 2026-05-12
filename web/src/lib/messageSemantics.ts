import type { Message } from '../api/client'

const AUTOMATION_ACTORS = new Set(['nex', 'watchdog', 'wuphf', 'scheduler'])
const NON_SUBSTANTIVE_ACTORS = new Set(['', 'system', ...AUTOMATION_ACTORS])
const HUMAN_DECISION_RECEIPT_PATTERN = /^(Answered @[^ ]+'s request(?:\b|:| with\b)|Rejected @[^ ]+'s request(?:\b|:| with\b)|Human asked @|Human replied to @|Human chose to answer @)/i
const OPERATIONAL_NOISE_PATTERNS = [
  /did not acknowledge (?:a direct mention|this demand) in time/i,
  /tentei publicar a confirma[çc][aã]o no thread\b/i,
  /n[aã]o consegui publicar .*no thread\b/i,
  /resposta em thread n[aã]o foi aceita/i,
  /post no thread .*bloqueado/i,
  /broker (?:ainda )?(?:bloqueou|recusou|retornou)/i,
]

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
  if (isHumanDecisionReceipt(message)) return false
  if (isAutomationMessage(message)) return false
  return !NON_SUBSTANTIVE_ACTORS.has(normalizeMessageActor(message.from))
}

export function isHumanDecisionReceipt(message: Message): boolean {
  const actor = normalizeMessageActor(message.from)
  if (actor !== 'you' && actor !== 'human') return false
  return HUMAN_DECISION_RECEIPT_PATTERN.test((message.content || '').trim())
}

export function isUnansweredWatchdogMessage(message: Message): boolean {
  if (normalizeMessageActor(message.kind) !== 'automation') return false
  if (normalizeMessageActor(message.source) !== 'watchdog') return false
  return normalizeMessageActor(message.title) === 'unanswered agent message' ||
    normalizeMessageActor(message.event_id).startsWith('watchdog-unanswered-message-')
}

export function isChannelFeedNoise(message: Message): boolean {
  const content = (message.content || '').trim()
  if (content.startsWith('[STATUS]')) return true
  if (isHumanDecisionReceipt(message)) return true
  if (isUnansweredWatchdogMessage(message)) return true
  if (OPERATIONAL_NOISE_PATTERNS.some((pattern) => pattern.test(content))) return true
  return false
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
