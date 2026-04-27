export type RefreshInterval = number | false

type RefreshPolicyContext = {
  brokerEventsConnected: boolean
  isPageVisible: boolean
  isWindowFocused: boolean
}

type ChannelFeedRefreshContext = RefreshPolicyContext & {
  hasActiveThread: boolean
}

function shouldDisableFallbackPolling({
  brokerEventsConnected,
  isPageVisible,
}: RefreshPolicyContext): boolean {
  return brokerEventsConnected || !isPageVisible
}

export function getChannelFeedRefreshInterval(context: ChannelFeedRefreshContext): RefreshInterval {
  if (shouldDisableFallbackPolling(context)) return false
  if (!context.isWindowFocused) return 30_000
  if (context.hasActiveThread) return 15_000
  return 6_000
}

export function getThreadPanelRefreshInterval(context: RefreshPolicyContext): RefreshInterval {
  if (shouldDisableFallbackPolling(context)) return false
  if (!context.isWindowFocused) return 20_000
  return 8_000
}

export function getThreadsAppRefreshInterval(context: RefreshPolicyContext): RefreshInterval {
  if (shouldDisableFallbackPolling(context)) return false
  if (!context.isWindowFocused) return 45_000
  return 20_000
}
