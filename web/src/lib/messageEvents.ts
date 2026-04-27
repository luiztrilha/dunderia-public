const MESSAGE_REFRESH_EVENT = 'dunderia:messages-refresh'
const MESSAGE_DELETED_EVENT = 'dunderia:message-deleted'

export type ChannelMessagesRefreshDetail = {
  channel?: string
  forceFull?: boolean
}

export type ChannelMessageDeletedDetail = {
  channel?: string
  messageId: string
  threadId?: string
}

export function dispatchChannelMessagesRefresh(channel: string, options?: { forceFull?: boolean }) {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent<ChannelMessagesRefreshDetail>(MESSAGE_REFRESH_EVENT, {
    detail: {
      channel,
      forceFull: options?.forceFull === true,
    },
  }))
}

export function dispatchChannelMessageDeleted(
  channel: string,
  messageId: string,
  options?: { threadId?: string },
) {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent<ChannelMessageDeletedDetail>(MESSAGE_DELETED_EVENT, {
    detail: {
      channel,
      messageId,
      threadId: options?.threadId,
    },
  }))
}

export function subscribeChannelMessagesRefresh(
  channel: string,
  onRefresh: (detail: ChannelMessagesRefreshDetail) => void,
) {
  if (typeof window === 'undefined') return () => {}
  const normalized = channel.trim().toLowerCase()
  const listener = (event: Event) => {
    const custom = event as CustomEvent<ChannelMessagesRefreshDetail>
    const target = custom.detail?.channel?.trim().toLowerCase()
    if (target && target !== normalized) return
    onRefresh(custom.detail ?? {})
  }
  window.addEventListener(MESSAGE_REFRESH_EVENT, listener as EventListener)
  return () => window.removeEventListener(MESSAGE_REFRESH_EVENT, listener as EventListener)
}

export function subscribeChannelMessageDeleted(
  channel: string,
  onDelete: (detail: ChannelMessageDeletedDetail) => void,
) {
  if (typeof window === 'undefined') return () => {}
  const normalized = channel.trim().toLowerCase()
  const listener = (event: Event) => {
    const custom = event as CustomEvent<ChannelMessageDeletedDetail>
    const target = custom.detail?.channel?.trim().toLowerCase()
    if (target && target !== normalized) return
    if (!custom.detail?.messageId?.trim()) return
    onDelete(custom.detail)
  }
  window.addEventListener(MESSAGE_DELETED_EVENT, listener as EventListener)
  return () => window.removeEventListener(MESSAGE_DELETED_EVENT, listener as EventListener)
}
