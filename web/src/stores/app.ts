import { create } from 'zustand'
import { ACTIVITY_APP_ID, HOME_APP_ID } from '../lib/constants'

export type Theme = 'slack' | 'slack-dark'
export type PrimaryRail = 'home' | 'dms' | 'activity' | 'channels' | 'more'

const THEME_STORAGE_KEY = 'dunderia.theme'
const VALID_THEMES: readonly Theme[] = ['slack', 'slack-dark']

function railForApp(app: string | null): PrimaryRail {
  if (app === HOME_APP_ID) return 'home'
  if (app === ACTIVITY_APP_ID) return 'activity'
  return 'more'
}

function loadInitialTheme(): Theme {
  if (typeof window === 'undefined') return 'slack'
  const stored = window.localStorage.getItem(THEME_STORAGE_KEY)
  if (stored && (VALID_THEMES as readonly string[]).includes(stored)) {
    document.documentElement.setAttribute('data-theme', stored)
    return stored as Theme
  }
  document.documentElement.setAttribute('data-theme', 'slack')
  return 'slack'
}

export interface ChannelMeta {
  type: 'O' | 'D' | 'G'
  name?: string
  members?: string[]
  agentSlug?: string
}

export interface AppStore {
  // Connection
  brokerConnected: boolean
  setBrokerConnected: (v: boolean) => void

  // Navigation
  currentChannel: string
  setCurrentChannel: (ch: string) => void
  currentApp: string | null // null = messages view
  setCurrentApp: (app: string | null) => void

  // Channel metadata (DM info, etc.)
  channelMeta: Record<string, ChannelMeta>
  setChannelMeta: (slug: string, meta: ChannelMeta) => void

  // Theme
  theme: Theme
  setTheme: (t: Theme) => void

  // Primary rail
  primaryRail: PrimaryRail
  setPrimaryRail: (rail: PrimaryRail) => void

  // Sidebar
  sidebarAgentsOpen: boolean
  toggleSidebarAgents: () => void

  // Thread panel
  activeThreadId: string | null
  setActiveThreadId: (id: string | null) => void
  activeThreadReplyTo: string | null
  setActiveThreadReplyTo: (id: string | null) => void

  // DM mode
  dmMode: boolean
  dmAgentSlug: string | null
  enterDM: (slug: string, channel: string) => void
  exitDM: () => void

  // Message polling state
  lastMessageId: string | null
  setLastMessageId: (id: string | null) => void

  // Agent panel
  activeAgentSlug: string | null
  setActiveAgentSlug: (slug: string | null) => void

  // Search
  searchOpen: boolean
  setSearchOpen: (v: boolean) => void

  // Onboarding
  onboardingComplete: boolean
  setOnboardingComplete: (v: boolean) => void
}

export const useAppStore = create<AppStore>((set) => ({
  brokerConnected: false,
  setBrokerConnected: (v) => set({ brokerConnected: v }),

  currentChannel: 'general',
  setCurrentChannel: (ch) =>
    set({
      currentChannel: ch,
      currentApp: null,
      primaryRail: 'channels',
      dmMode: false,
      dmAgentSlug: null,
      activeThreadId: null,
      activeThreadReplyTo: null,
    }),
  currentApp: HOME_APP_ID,
  setCurrentApp: (app) =>
    set({
      currentApp: app,
      primaryRail: app ? railForApp(app) : 'channels',
      dmMode: false,
      dmAgentSlug: null,
    }),

  channelMeta: {},
  setChannelMeta: (slug, meta) =>
    set((s) => ({ channelMeta: { ...s.channelMeta, [slug]: meta } })),

  theme: loadInitialTheme(),
  setTheme: (t) => {
    document.documentElement.setAttribute('data-theme', t)
    if (typeof window !== 'undefined') {
      window.localStorage.setItem(THEME_STORAGE_KEY, t)
    }
    set({ theme: t })
  },

  primaryRail: 'home',
  setPrimaryRail: (primaryRail) => set({ primaryRail }),

  sidebarAgentsOpen: true,
  toggleSidebarAgents: () => set((s) => ({ sidebarAgentsOpen: !s.sidebarAgentsOpen })),

  activeThreadId: null,
  setActiveThreadId: (id) => set({ activeThreadId: id }),
  activeThreadReplyTo: null,
  setActiveThreadReplyTo: (id) => set({ activeThreadReplyTo: id }),

  dmMode: false,
  dmAgentSlug: null,
  enterDM: (slug, channel) =>
    set({
      dmMode: true,
      dmAgentSlug: slug,
      primaryRail: 'dms',
      currentChannel: channel,
      currentApp: null,
      activeThreadId: null,
      activeThreadReplyTo: null,
    }),
  exitDM: () =>
    set({
      dmMode: false,
      dmAgentSlug: null,
      primaryRail: 'channels',
      currentChannel: 'general',
      activeThreadId: null,
      activeThreadReplyTo: null,
    }),

  lastMessageId: null,
  setLastMessageId: (id) => set({ lastMessageId: id }),

  activeAgentSlug: null,
  setActiveAgentSlug: (slug) => set({ activeAgentSlug: slug }),

  searchOpen: false,
  setSearchOpen: (v) => set({ searchOpen: v }),

  onboardingComplete: false,
  setOnboardingComplete: (v) => set({ onboardingComplete: v }),
}))
