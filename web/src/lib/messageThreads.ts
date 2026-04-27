import type { Message } from '../api/client'
import {
  automationPreview,
  automationSourceLabel,
  isAutomationMessage,
  isSubstantiveMessage,
  truncateMessageText,
} from './messageSemantics'

export type ThreadNode = {
  message: Message
  children: ThreadNode[]
}

export type ThreadRole = 'single' | 'root' | 'branch' | 'leaf'

export type FlattenedThreadNode = {
  type: 'message'
  key: string
  message: Message
  grouped: boolean
  threadDepth: number
  threadParentLabel?: string
}

export type InlineThreadDisplayNode =
  | FlattenedThreadNode
  | {
      type: 'automation-group'
      key: string
      nodes: FlattenedThreadNode[]
      count: number
      threadDepth: number
      sourceLabel: string
      preview: string
    }

export type ThreadContextItem = {
  key: string
  author: string
  timestamp: string
  preview: string
}

export function normalizeChannel(value: string): string {
  const trimmed = value.trim().toLowerCase()
  if (!trimmed) return ''
  return trimmed
    .replace(/^#/, '')
    .replace(/ /g, '-')
    .replace(/__/g, '\u0000')
    .replace(/_/g, '-')
    .replace(/\u0000/g, '__')
}

export function dateDayKey(ts: string): string {
  const d = new Date(ts)
  return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`
}

export function sortByTimeAsc(a: Message, b: Message): number {
  return new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime()
}

export function buildThreadTree(messages: Message[]): ThreadNode[] {
  const byId = new Map<string, Message>(messages.map((message) => [message.id, message]))
  const children = new Map<string, Message[]>()
  const rootMessages: Message[] = []

  for (const message of messages) {
    const parentId = message.reply_to
    if (parentId && byId.has(parentId)) {
      const bucket = children.get(parentId) ?? []
      bucket.push(message)
      children.set(parentId, bucket)
      continue
    }
    rootMessages.push(message)
  }

  for (const list of children.values()) {
    list.sort(sortByTimeAsc)
  }

  rootMessages.sort(sortByTimeAsc)

  const buildNode = (message: Message, seen: Set<string>): ThreadNode | null => {
    if (seen.has(message.id)) return null
    seen.add(message.id)

    const childNodes: ThreadNode[] = []
    for (const child of children.get(message.id) ?? []) {
      const childNode = buildNode(child, seen)
      if (childNode) childNodes.push(childNode)
    }

    return { message, children: childNodes }
  }

  const roots: ThreadNode[] = []
  const seen = new Set<string>()

  for (const rootMessage of rootMessages) {
    const node = buildNode(rootMessage, seen)
    if (node) roots.push(node)
  }

  for (const message of messages) {
    if (seen.has(message.id)) continue
    const node = buildNode(message, seen)
    if (node) roots.push(node)
  }

  return roots
}

export function flattenThreadTree(
  nodes: ThreadNode[],
  messagesById: Map<string, Message>,
  getAuthorLabel: (message: Message) => string,
  t: (key: string, data?: Record<string, unknown>) => string,
  depth = 0,
): FlattenedThreadNode[] {
  const entries: FlattenedThreadNode[] = []

  const walk = (currentNodes: ThreadNode[], currentDepth: number) => {
    for (const node of currentNodes) {
      let threadParentLabel: string | undefined
      if (node.message.reply_to) {
        const parent = messagesById.get(node.message.reply_to)
        threadParentLabel = parent
          ? t('messages.thread.replyTo', { author: getAuthorLabel(parent) })
          : t('messages.thread.replyToMissing')
      }

      entries.push({
        type: 'message',
        key: node.message.id,
        message: node.message,
        grouped: false,
        threadDepth: currentDepth,
        threadParentLabel,
      })
      walk(node.children, currentDepth + 1)
    }
  }

  walk(nodes, depth)
  return entries
}

export function buildInlineThreadDisplayNodes(nodes: FlattenedThreadNode[]): InlineThreadDisplayNode[] {
  const entries: InlineThreadDisplayNode[] = []

  for (let index = 0; index < nodes.length; index += 1) {
    const node = nodes[index]
    if (isAutomationMessage(node.message)) {
      const burst = [node]
      let cursor = index + 1
      while (cursor < nodes.length) {
        const candidate = nodes[cursor]
        if (!isAutomationMessage(candidate.message)) break
        if (candidate.threadDepth !== node.threadDepth) break
        if ((candidate.message.reply_to || '') !== (node.message.reply_to || '')) break
        burst.push(candidate)
        cursor += 1
      }
      if (burst.length >= 2) {
        entries.push({
          type: 'automation-group',
          key: `thread-automation-${burst[0].message.id}-${burst[burst.length - 1].message.id}`,
          nodes: burst,
          count: burst.length,
          threadDepth: node.threadDepth,
          sourceLabel: automationSourceLabel(burst.map((item) => item.message)),
          preview: automationPreview(burst.map((item) => item.message)),
        })
        index = cursor - 1
        continue
      }
    }
    entries.push(node)
  }

  return entries
}

export function buildInlineThreadContextItems(
  messages: Message[],
  threadId: string,
  getAuthorLabel: (message: Message) => string,
): ThreadContextItem[] {
  if (messages.length === 0) return []

  const threadRoot = messages.find((message) => message.id === threadId) ||
    messages.find((message) => !message.reply_to) ||
    messages[0]

  const items: Message[] = []
  if ((threadRoot.content || '').trim()) {
    items.push(threadRoot)
  }

  const latestSubstantiveReply = [...messages]
    .reverse()
    .find((message) => message.id !== threadRoot.id && isSubstantiveMessage(message))

  if (latestSubstantiveReply) {
    items.push(latestSubstantiveReply)
  }

  return items.map((message) => ({
    key: message.id,
    author: getAuthorLabel(message),
    timestamp: message.timestamp,
    preview: truncateMessageText(message.content || '', 180),
  }))
}

export function getThreadRole(message: Message): ThreadRole {
  const replies = message.thread_count ?? 0
  if (message.reply_to) {
    return replies > 0 ? 'branch' : 'leaf'
  }
  return replies > 0 ? 'root' : 'single'
}
