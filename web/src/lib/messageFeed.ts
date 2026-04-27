import type { Message } from '../api/client'
import { formatDateLabel } from './format'
import { dateDayKey } from './messageThreads'
import {
  automationPreview,
  automationSourceLabel,
  isAutomationMessage,
  isSubstantiveMessage,
  normalizeMessageActor,
  truncateMessageText,
} from './messageSemantics'

export type MessageFeedElement =
  | { type: 'date'; key: string; label: string }
  | { type: 'message'; key: string; message: Message; grouped: boolean; threadDepth: number; threadParentLabel?: string }
  | { type: 'automation-group'; key: string; messages: Message[]; count: number; sourceLabel: string; preview: string }

export type MessageFeedContextItem = {
  key: string
  author: string
  timestamp: string
  preview: string
}

export function buildMessageFeedElements(rootMessages: Message[]): MessageFeedElement[] {
  const list: MessageFeedElement[] = []
  let lastDate = ''
  let previousMessage: Message | null = null

  for (let index = 0; index < rootMessages.length; index += 1) {
    const msg = rootMessages[index]
    if (msg.timestamp) {
      const dayKey = dateDayKey(msg.timestamp)
      if (dayKey !== lastDate) {
        list.push({ type: 'date', key: `date-${dayKey}`, label: formatDateLabel(msg.timestamp) })
        lastDate = dayKey
        previousMessage = null
      }
    }

    if (isAutomationFeedMessage(msg)) {
      const automationBurst = [msg]
      let cursor = index + 1
      while (cursor < rootMessages.length) {
        const candidate = rootMessages[cursor]
        if (!isAutomationFeedMessage(candidate)) break
        if (dateDayKey(candidate.timestamp) !== dateDayKey(msg.timestamp)) break
        automationBurst.push(candidate)
        cursor += 1
      }
      if (automationBurst.length >= 2) {
        list.push({
          type: 'automation-group',
          key: `automation-${automationBurst[0].id}-${automationBurst[automationBurst.length - 1].id}`,
          messages: automationBurst,
          count: automationBurst.length,
          sourceLabel: automationSourceLabel(automationBurst),
          preview: automationPreview(automationBurst),
        })
        index = cursor - 1
        previousMessage = null
        continue
      }
    }

    let grouped = false
    if (
      previousMessage &&
      previousMessage.from === msg.from &&
      msg.timestamp &&
      previousMessage.timestamp &&
      !previousMessage.reply_to &&
      !msg.reply_to
    ) {
      const delta = new Date(msg.timestamp).getTime() - new Date(previousMessage.timestamp).getTime()
      if (delta >= 0 && delta < 5 * 60 * 1000) grouped = true
    }

    list.push({
      type: 'message',
      key: msg.id,
      message: msg,
      grouped,
      threadDepth: 0,
      threadParentLabel: undefined,
    })
    previousMessage = msg
  }

  return list
}

export function buildRecentContextItems(rootMessages: Message[], limit = 2): MessageFeedContextItem[] {
  return [...rootMessages]
    .filter((message) => isSubstantiveFeedMessage(message))
    .slice(-limit)
    .reverse()
    .map((message) => ({
      key: message.id,
      author: normalizeActor(message.from) || message.from || 'unknown',
      timestamp: message.timestamp,
      preview: truncateMessageText(message.content, 180),
    }))
}

export function isAutomationFeedMessage(message: Message): boolean {
  return isAutomationMessage(message)
}

export function isSubstantiveFeedMessage(message: Message): boolean {
  return isSubstantiveMessage(message)
}

function normalizeActor(value?: string | null): string {
  return normalizeMessageActor(value)
}
