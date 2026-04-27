import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useAppStore } from '../stores/app'
import { getMessages, getThreadMessages } from '../api/client'
import type { ExecutionNode, Message } from '../api/client'
import { useBrokerEventsState } from './useBrokerEvents'
import { subscribeChannelMessageDeleted, subscribeChannelMessagesRefresh } from '../lib/messageEvents'
import { usePageActivity } from '../lib/pageActivity'

const CHANNEL_PAGE_SIZE = 100

type ChannelCacheEntry = {
  messages: Message[]
  executionNodes: ExecutionNode[]
  hasOlder: boolean
}

type ThreadCacheEntry = {
  messages: Message[]
  executionNodes: ExecutionNode[]
}

type RefreshRequest = {
  indicate: boolean
  forceFull: boolean
}

const channelMessageCache = new Map<string, ChannelCacheEntry>()
const threadMessageCache = new Map<string, ThreadCacheEntry>()
const channelPrefetchInflight = new Map<string, Promise<void>>()
const threadPrefetchInflight = new Map<string, Promise<void>>()

function normalizeChannel(value: string): string {
  const trimmed = value.trim().toLowerCase()
  if (!trimmed) return ''
  return trimmed
    .replace(/^#/, '')
    .replace(/ /g, '-')
    .replace(/__/g, '\u0000')
    .replace(/_/g, '-')
    .replace(/\u0000/g, '__')
}

function mergeUniqueMessages(messages: Message[]): Message[] {
  const seen = new Set<string>()
  const out: Message[] = []
  for (const message of messages) {
    if (!message?.id || seen.has(message.id)) continue
    seen.add(message.id)
    out.push(message)
  }
  return out
}

function messageLoadError(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback
}

function mergeRefreshRequest(current: RefreshRequest | null, next: RefreshRequest): RefreshRequest {
  if (!current) return next
  return {
    indicate: current.indicate || next.indicate,
    forceFull: current.forceFull || next.forceFull,
  }
}

function channelCacheKey(channel: string): string {
  return normalizeChannel(channel || 'general')
}

function threadCacheKey(channel: string, threadId: string): string {
  return `${channelCacheKey(channel)}::${threadId.trim()}`
}

function readChannelCache(channel: string): ChannelCacheEntry | undefined {
  return channelMessageCache.get(channelCacheKey(channel))
}

function writeChannelCache(channel: string, entry: ChannelCacheEntry): void {
  channelMessageCache.set(channelCacheKey(channel), {
    messages: [...entry.messages],
    executionNodes: [...entry.executionNodes],
    hasOlder: entry.hasOlder,
  })
}

function readThreadCache(channel: string, threadId: string): ThreadCacheEntry | undefined {
  return threadMessageCache.get(threadCacheKey(channel, threadId))
}

function writeThreadCache(channel: string, threadId: string, entry: ThreadCacheEntry): void {
  threadMessageCache.set(threadCacheKey(channel, threadId), {
    messages: [...entry.messages],
    executionNodes: [...entry.executionNodes],
  })
}

export async function prefetchChannelMessages(channel: string): Promise<void> {
  const key = channelCacheKey(channel)
  if (channelMessageCache.has(key)) return
  const existing = channelPrefetchInflight.get(key)
  if (existing) return existing

  const request = (async () => {
    const data = await getMessages(channel, { limit: CHANNEL_PAGE_SIZE })
    const nextMessages = mergeUniqueMessages(
      (data.messages ?? []).filter((message) => normalizeChannel(message.channel || '') === key),
    )
    writeChannelCache(channel, {
      messages: nextMessages,
      executionNodes: data.execution_nodes ?? [],
      hasOlder: Boolean(data.has_more),
    })
  })()

  channelPrefetchInflight.set(key, request)
  try {
    await request
  } catch {
    // Hover/focus prefetch is opportunistic.
  } finally {
    if (channelPrefetchInflight.get(key) === request) {
      channelPrefetchInflight.delete(key)
    }
  }
}

export async function prefetchThreadMessages(channel: string, threadId: string): Promise<void> {
  const key = threadCacheKey(channel, threadId)
  if (threadMessageCache.has(key)) return
  const existing = threadPrefetchInflight.get(key)
  if (existing) return existing

  const request = (async () => {
    const data = await getThreadMessages(channel, threadId)
    const targetChannel = channelCacheKey(channel)
    const nextMessages = mergeUniqueMessages(
      (data.messages ?? []).filter((message) => normalizeChannel(message.channel || '') === targetChannel),
    )
    writeThreadCache(channel, threadId, {
      messages: nextMessages,
      executionNodes: data.execution_nodes ?? [],
    })
  })()

  threadPrefetchInflight.set(key, request)
  try {
    await request
  } catch {
    // Hover/focus prefetch is opportunistic.
  } finally {
    if (threadPrefetchInflight.get(key) === request) {
      threadPrefetchInflight.delete(key)
    }
  }
}

export function useMessages(channel: string) {
  const { isPageActive } = usePageActivity()
  const { connected: brokerEventsConnected } = useBrokerEventsState()
  const activeThreadId = useAppStore((s) => s.activeThreadId)
  const cachedState = readChannelCache(channel)
  const [messages, setMessages] = useState<Message[]>(() => cachedState?.messages ?? [])
  const [executionNodes, setExecutionNodes] = useState<ExecutionNode[]>(() => cachedState?.executionNodes ?? [])
  const [isLoading, setIsLoading] = useState(() => !cachedState)
  const [isRefreshing, setIsRefreshing] = useState(false)
  const [isFetchingOlder, setIsFetchingOlder] = useState(false)
  const [hasOlder, setHasOlder] = useState(() => Boolean(cachedState?.hasOlder))
  const [error, setError] = useState<string | null>(null)
  const generationRef = useRef(0)
  const latestMessageIdRef = useRef<string | null>(cachedState?.messages[cachedState.messages.length - 1]?.id ?? null)
  const mountedRef = useRef(true)
  const hasResolvedRef = useRef(Boolean(cachedState))
  const currentMessagesRef = useRef<Message[]>(cachedState?.messages ?? [])
  const currentExecutionNodesRef = useRef<ExecutionNode[]>(cachedState?.executionNodes ?? [])
  const hasOlderRef = useRef(Boolean(cachedState?.hasOlder))
  const refreshPromiseRef = useRef<Promise<void> | null>(null)
  const pendingRefreshRef = useRef<RefreshRequest | null>(null)
  const targetChannel = normalizeChannel(channel || 'general')
  const previousPageActiveRef = useRef(isPageActive)
  const previousBrokerEventsConnectedRef = useRef(brokerEventsConnected)

  const sanitizeMessages = useCallback(
    (incoming: Message[]) => {
      const filtered = incoming.filter((message) => normalizeChannel(message.channel || '') === targetChannel)
      return mergeUniqueMessages(filtered)
    },
    [targetChannel],
  )

  const storeChannelState = useCallback(
    (nextMessages: Message[], nextExecutionNodes: ExecutionNode[], nextHasOlder: boolean) => {
      currentMessagesRef.current = nextMessages
      currentExecutionNodesRef.current = nextExecutionNodes
      hasOlderRef.current = nextHasOlder
      latestMessageIdRef.current = nextMessages[nextMessages.length - 1]?.id ?? null
      hasResolvedRef.current = true
      writeChannelCache(channel, {
        messages: nextMessages,
        executionNodes: nextExecutionNodes,
        hasOlder: nextHasOlder,
      })
    },
    [channel],
  )

  const removeMessageLocally = useCallback(
    (messageId: string) => {
      const normalizedMessageId = messageId.trim()
      if (!normalizedMessageId) return
      const nextMessages = currentMessagesRef.current.filter((message) => message.id !== normalizedMessageId)
      if (nextMessages.length === currentMessagesRef.current.length) return
      setMessages(nextMessages)
      storeChannelState(nextMessages, currentExecutionNodesRef.current, hasOlderRef.current)
    },
    [storeChannelState],
  )

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      generationRef.current += 1
    }
  }, [])

  useEffect(() => {
    currentMessagesRef.current = messages
    latestMessageIdRef.current = messages[messages.length - 1]?.id ?? null
  }, [messages])

  useEffect(() => {
    currentExecutionNodesRef.current = executionNodes
  }, [executionNodes])

  useEffect(() => {
    hasOlderRef.current = hasOlder
  }, [hasOlder])

  const refreshLatest = useCallback(
    async (options?: { indicate?: boolean; forceFull?: boolean }) => {
      const requestOptions: RefreshRequest = {
        indicate: options?.indicate ?? true,
        forceFull: options?.forceFull ?? false,
      }
      if (refreshPromiseRef.current) {
        pendingRefreshRef.current = mergeRefreshRequest(pendingRefreshRef.current, requestOptions)
        return refreshPromiseRef.current
      }

      const request = (async () => {
        const generation = generationRef.current
        const { indicate, forceFull } = requestOptions
        const latestMessageID = latestMessageIdRef.current
        const canUseDelta = !forceFull && hasResolvedRef.current && latestMessageID != null && latestMessageID !== ''

        if (indicate) {
          if (hasResolvedRef.current) {
            setIsRefreshing(true)
          } else {
            setIsLoading(true)
          }
        }
        setError(null)

        try {
          const data = await getMessages(channel, {
            limit: CHANNEL_PAGE_SIZE,
            sinceId: canUseDelta ? latestMessageID : null,
          })
          if (!mountedRef.current || generation !== generationRef.current) return

          const nextExecutionNodes = data.execution_nodes ?? []
          const incomingMessages = sanitizeMessages(data.messages ?? [])

          if (canUseDelta) {
            const nextHasOlder = hasOlderRef.current
            setExecutionNodes(nextExecutionNodes)
            setHasOlder(nextHasOlder)
            if (incomingMessages.length === 0) {
              storeChannelState(currentMessagesRef.current, nextExecutionNodes, nextHasOlder)
            } else {
              const nextMessages = sanitizeMessages([...currentMessagesRef.current, ...incomingMessages])
              setMessages(nextMessages)
              storeChannelState(nextMessages, nextExecutionNodes, nextHasOlder)
            }
            setError(null)
            return
          }

          const nextHasOlder = Boolean(data.has_more)
          setMessages(incomingMessages)
          setExecutionNodes(nextExecutionNodes)
          setHasOlder(nextHasOlder)
          storeChannelState(incomingMessages, nextExecutionNodes, nextHasOlder)
          setError(null)
        } catch (nextError) {
          if (!mountedRef.current || generation !== generationRef.current) return
          setError(messageLoadError(nextError, 'Failed to load messages'))
        } finally {
          if (mountedRef.current && generation == generationRef.current) {
            setIsLoading(false)
            setIsRefreshing(false)
          }
        }
      })()

      refreshPromiseRef.current = request
      try {
        await request
      } finally {
        if (refreshPromiseRef.current === request) {
          refreshPromiseRef.current = null
        }
        const pending = pendingRefreshRef.current
        pendingRefreshRef.current = null
        if (pending && mountedRef.current) {
          void refreshLatest(pending)
        }
      }
    },
    [channel, sanitizeMessages, storeChannelState],
  )

  const loadInitial = useCallback(
    async (options?: { indicate?: boolean }) => {
      await refreshLatest({ indicate: options?.indicate, forceFull: true })
    },
    [refreshLatest],
  )

  const loadOlder = useCallback(async () => {
    if (isFetchingOlder || !hasOlderRef.current || currentMessagesRef.current.length === 0) return
    const oldestId = currentMessagesRef.current[0]?.id
    if (!oldestId) return

    const generation = generationRef.current
    setIsFetchingOlder(true)
    try {
      const data = await getMessages(channel, { beforeId: oldestId, limit: CHANNEL_PAGE_SIZE })
      if (!mountedRef.current || generation !== generationRef.current) return
      const older = data.messages ?? []
      const nextMessages = sanitizeMessages([...older, ...currentMessagesRef.current])
      const nextHasOlder = Boolean(data.has_more)
      setMessages(nextMessages)
      setHasOlder(nextHasOlder)
      storeChannelState(nextMessages, currentExecutionNodesRef.current, nextHasOlder)
      setError(null)
    } catch (nextError) {
      if (!mountedRef.current || generation !== generationRef.current) return
      setError(messageLoadError(nextError, 'Failed to load older messages'))
    } finally {
      if (mountedRef.current && generation === generationRef.current) {
        setIsFetchingOlder(false)
      }
    }
  }, [channel, isFetchingOlder, sanitizeMessages, storeChannelState])

  useEffect(() => {
    generationRef.current += 1
    refreshPromiseRef.current = null
    pendingRefreshRef.current = null
    const cached = readChannelCache(channel)
    const nextMessages = cached?.messages ?? []
    const nextExecutionNodes = cached?.executionNodes ?? []
    const nextHasOlder = Boolean(cached?.hasOlder)
    latestMessageIdRef.current = nextMessages[nextMessages.length - 1]?.id ?? null
    hasResolvedRef.current = Boolean(cached)
    currentMessagesRef.current = nextMessages
    currentExecutionNodesRef.current = nextExecutionNodes
    hasOlderRef.current = nextHasOlder
    setMessages(nextMessages)
    setExecutionNodes(nextExecutionNodes)
    setHasOlder(nextHasOlder)
    setError(null)
    setIsLoading(!cached)
    setIsRefreshing(Boolean(cached))
    void refreshLatest({ forceFull: !cached })
  }, [channel, refreshLatest])

  useEffect(
    () =>
      subscribeChannelMessagesRefresh(channel, (detail) => {
        if (!isPageActive) return
        void refreshLatest({ indicate: false, forceFull: detail.forceFull === true })
      }),
    [channel, isPageActive, refreshLatest],
  )

  useEffect(
    () =>
      subscribeChannelMessageDeleted(channel, (detail) => {
        removeMessageLocally(detail.messageId)
      }),
    [channel, removeMessageLocally],
  )

  useEffect(() => {
    if (!isPageActive && previousPageActiveRef.current === isPageActive) {
      return undefined
    }

    if (isPageActive && !previousPageActiveRef.current && hasResolvedRef.current) {
      void refreshLatest({ indicate: false })
    }
    previousPageActiveRef.current = isPageActive
    return undefined
  }, [isPageActive, refreshLatest])

  useEffect(() => {
    if (brokerEventsConnected && !previousBrokerEventsConnectedRef.current && hasResolvedRef.current) {
      void refreshLatest({ indicate: false })
    }
    previousBrokerEventsConnectedRef.current = brokerEventsConnected
  }, [brokerEventsConnected, refreshLatest])

  useEffect(() => {
    if (!isPageActive || brokerEventsConnected) return undefined

    const delay = activeThreadId ? 12000 : 6000
    let cancelled = false
    let timer: ReturnType<typeof window.setTimeout> | null = null

    const tick = async () => {
      if (cancelled) return
      try {
        await refreshLatest({ indicate: false })
      } finally {
        if (!cancelled) {
          timer = window.setTimeout(() => {
            void tick()
          }, delay)
        }
      }
    }

    timer = window.setTimeout(() => {
      void tick()
    }, delay)

    return () => {
      cancelled = true
      if (timer) {
        window.clearTimeout(timer)
      }
    }
  }, [activeThreadId, brokerEventsConnected, isPageActive, refreshLatest])

  return useMemo(
    () => ({
      data: messages,
      executionNodes,
      isLoading,
      isRefreshing,
      hasOlder,
      isFetchingOlder,
      error,
      loadOlder,
      refetch: loadInitial,
    }),
    [error, executionNodes, hasOlder, isFetchingOlder, isLoading, isRefreshing, loadInitial, loadOlder, messages],
  )
}

export function useThreadMessages(channel: string, threadId: string | null) {
  const { isPageActive } = usePageActivity()
  const { connected: brokerEventsConnected } = useBrokerEventsState()
  const cachedState = threadId ? readThreadCache(channel, threadId) : undefined
  const [messages, setMessages] = useState<Message[]>(() => cachedState?.messages ?? [])
  const [executionNodes, setExecutionNodes] = useState<ExecutionNode[]>(() => cachedState?.executionNodes ?? [])
  const [isLoading, setIsLoading] = useState(() => Boolean(threadId) && !cachedState)
  const [isRefreshing, setIsRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const generationRef = useRef(0)
  const mountedRef = useRef(true)
  const hasResolvedRef = useRef(Boolean(cachedState))
  const currentMessagesRef = useRef<Message[]>(cachedState?.messages ?? [])
  const currentExecutionNodesRef = useRef<ExecutionNode[]>(cachedState?.executionNodes ?? [])
  const latestMessageIdRef = useRef<string | null>(cachedState?.messages[cachedState.messages.length - 1]?.id ?? null)
  const refreshPromiseRef = useRef<Promise<void> | null>(null)
  const pendingRefreshRef = useRef<RefreshRequest | null>(null)
  const previousPageActiveRef = useRef(isPageActive)
  const previousBrokerEventsConnectedRef = useRef(brokerEventsConnected)
  const targetChannel = normalizeChannel(channel || 'general')

  const sanitizeMessages = useCallback(
    (incoming: Message[]) => {
      const filtered = incoming.filter(
        (message) => normalizeChannel(message.channel || '') === targetChannel,
      )
      return mergeUniqueMessages(filtered)
    },
    [targetChannel],
  )

  const storeThreadState = useCallback(
    (nextMessages: Message[], nextExecutionNodes: ExecutionNode[]) => {
      if (!threadId) return
      hasResolvedRef.current = true
      writeThreadCache(channel, threadId, {
        messages: nextMessages,
        executionNodes: nextExecutionNodes,
      })
    },
    [channel, threadId],
  )

  const removeThreadMessageLocally = useCallback(
    (messageId: string) => {
      const normalizedMessageId = messageId.trim()
      if (!normalizedMessageId) return
      const nextMessages = currentMessagesRef.current.filter((message) => message.id !== normalizedMessageId)
      if (nextMessages.length === currentMessagesRef.current.length) return
      setMessages(nextMessages)
      storeThreadState(nextMessages, currentExecutionNodesRef.current)
    },
    [storeThreadState],
  )

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      generationRef.current += 1
    }
  }, [])

  useEffect(() => {
    currentMessagesRef.current = messages
    latestMessageIdRef.current = messages[messages.length - 1]?.id ?? null
  }, [messages])

  useEffect(() => {
    currentExecutionNodesRef.current = executionNodes
  }, [executionNodes])

  const loadThread = useCallback(
    async (options?: { indicate?: boolean; forceFull?: boolean }) => {
      if (!threadId) {
        setMessages([])
        setExecutionNodes([])
        setError(null)
        setIsLoading(false)
        setIsRefreshing(false)
        return
      }

      const requestOptions: RefreshRequest = {
        indicate: options?.indicate ?? true,
        forceFull: options?.forceFull ?? false,
      }
      if (refreshPromiseRef.current) {
        pendingRefreshRef.current = mergeRefreshRequest(pendingRefreshRef.current, requestOptions)
        return refreshPromiseRef.current
      }

      const request = (async () => {
        const generation = generationRef.current
        const { indicate, forceFull } = requestOptions
        const latestMessageID = latestMessageIdRef.current
        const canUseDelta = !forceFull && hasResolvedRef.current && latestMessageID != null && latestMessageID !== ''

        if (indicate) {
          if (hasResolvedRef.current) {
            setIsRefreshing(true)
          } else {
            setIsLoading(true)
          }
        }
        setError(null)

        try {
          const data = await getThreadMessages(channel, threadId, {
            limit: 50,
            sinceId: canUseDelta ? latestMessageID : null,
          })
          if (!mountedRef.current || generation !== generationRef.current) return

          const nextExecutionNodes = data.execution_nodes ?? []
          const incomingMessages = sanitizeMessages(data.messages ?? [])

          if (canUseDelta) {
            setExecutionNodes(nextExecutionNodes)
            if (incomingMessages.length === 0) {
              storeThreadState(currentMessagesRef.current, nextExecutionNodes)
            } else {
              const nextMessages = sanitizeMessages([...currentMessagesRef.current, ...incomingMessages])
              setMessages(nextMessages)
              storeThreadState(nextMessages, nextExecutionNodes)
            }
            setError(null)
            return
          }

          setMessages(incomingMessages)
          setExecutionNodes(nextExecutionNodes)
          storeThreadState(incomingMessages, nextExecutionNodes)
          setError(null)
        } catch (nextError) {
          if (!mountedRef.current || generation !== generationRef.current) return
          setError(messageLoadError(nextError, 'Failed to load thread'))
        } finally {
          if (mountedRef.current && generation === generationRef.current) {
            setIsLoading(false)
            setIsRefreshing(false)
          }
        }
      })()

      refreshPromiseRef.current = request
      try {
        await request
      } finally {
        if (refreshPromiseRef.current === request) {
          refreshPromiseRef.current = null
        }
        const pending = pendingRefreshRef.current
        pendingRefreshRef.current = null
        if (pending && mountedRef.current) {
          void loadThread(pending)
        }
      }
    },
    [channel, sanitizeMessages, storeThreadState, threadId],
  )

  useEffect(() => {
    generationRef.current += 1
    refreshPromiseRef.current = null
    pendingRefreshRef.current = null
    if (!threadId) {
      hasResolvedRef.current = false
      currentMessagesRef.current = []
      currentExecutionNodesRef.current = []
      latestMessageIdRef.current = null
      setMessages([])
      setExecutionNodes([])
      setError(null)
      setIsLoading(false)
      setIsRefreshing(false)
      return
    }

    const cached = readThreadCache(channel, threadId)
    hasResolvedRef.current = Boolean(cached)
    currentMessagesRef.current = cached?.messages ?? []
    currentExecutionNodesRef.current = cached?.executionNodes ?? []
    latestMessageIdRef.current = cached?.messages[cached.messages.length - 1]?.id ?? null
    setMessages(cached?.messages ?? [])
    setExecutionNodes(cached?.executionNodes ?? [])
    setError(null)
    setIsLoading(!cached)
    setIsRefreshing(Boolean(cached))
    void loadThread({ forceFull: !cached })
  }, [channel, loadThread, threadId])

  useEffect(
    () =>
      subscribeChannelMessagesRefresh(channel, (detail) => {
        if (!isPageActive || !threadId) return
        void loadThread({ indicate: false, forceFull: detail.forceFull === true })
      }),
    [channel, isPageActive, loadThread, threadId],
  )

  useEffect(
    () =>
      subscribeChannelMessageDeleted(channel, (detail) => {
        if (!threadId) return
        removeThreadMessageLocally(detail.messageId)
      }),
    [channel, removeThreadMessageLocally, threadId],
  )

  useEffect(() => {
    if (!isPageActive && previousPageActiveRef.current === isPageActive) {
      return undefined
    }
    if (isPageActive && !previousPageActiveRef.current && threadId && hasResolvedRef.current) {
      void loadThread({ indicate: false })
    }
    previousPageActiveRef.current = isPageActive
    return undefined
  }, [isPageActive, loadThread, threadId])

  useEffect(() => {
    if (brokerEventsConnected && !previousBrokerEventsConnectedRef.current && threadId && hasResolvedRef.current) {
      void loadThread({ indicate: false })
    }
    previousBrokerEventsConnectedRef.current = brokerEventsConnected
  }, [brokerEventsConnected, loadThread, threadId])

  useEffect(() => {
    if (!threadId || !isPageActive || brokerEventsConnected) return undefined

    const timer = window.setInterval(() => {
      void loadThread({ indicate: false })
    }, 10000)

    return () => {
      window.clearInterval(timer)
    }
  }, [brokerEventsConnected, isPageActive, loadThread, threadId])

  return useMemo(
    () => ({
      data: messages,
      executionNodes,
      isLoading,
      isRefreshing,
      error,
      refetch: loadThread,
    }),
    [error, executionNodes, isLoading, isRefreshing, loadThread, messages],
  )
}

export type { Message }
