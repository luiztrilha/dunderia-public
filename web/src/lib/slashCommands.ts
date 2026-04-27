import i18n from '../i18n/config'
import { clearChannel, createDM, extractDMChannelSlug, post } from '../api/client'
import { dispatchChannelMessagesRefresh } from './messageEvents'
import { showNotice } from '../components/ui/Toast'
import { confirm } from '../components/ui/ConfirmDialog'
import { openProviderSwitcher } from '../components/ui/ProviderSwitcher'

export interface SlashCommandDeps {
  currentChannel: string
  setCurrentApp: (id: string | null) => void
  setCurrentChannel: (slug: string) => void
  setLastMessageId?: (id: string | null) => void
  setSearchOpen?: (open: boolean) => void
  enterDM?: (agentSlug: string, channelSlug: string) => void
  clearActiveThread?: () => void
  onChannelCleared?: () => void
  onReset?: () => void
  resetRequiresConfirm?: boolean
  helpMode?: 'composer' | 'palette'
}

export function dispatchSlashCommand(input: string, deps: SlashCommandDeps): boolean {
  const parts = input.split(/\s+/)
  const cmd = parts[0].toLowerCase()
  const args = parts.slice(1).join(' ')
  const t = i18n.t.bind(i18n)

  switch (cmd) {
    case '/clear':
      clearChannel(deps.currentChannel)
        .then(() => {
          deps.clearActiveThread?.()
          deps.onChannelCleared?.()
          dispatchChannelMessagesRefresh(deps.currentChannel)
          showNotice(t('messages.commands.cleared'), 'info')
        })
        .catch((e: Error) => showNotice(t('messages.commands.resetFailed', { error: e.message }), 'error'))
      return true
    case '/help':
      showNotice(
        deps.helpMode === 'palette' ? t('search.palette.help') : t('messages.commands.helpList'),
        'info',
      )
      return true
    case '/requests':
      deps.setCurrentApp('requests')
      return true
    case '/policies':
      deps.setCurrentApp('policies')
      return true
    case '/skills':
      deps.setCurrentApp('skills')
      return true
    case '/calendar':
      deps.setCurrentApp('calendar')
      return true
    case '/tasks':
      deps.setCurrentApp('tasks')
      return true
    case '/recover':
    case '/doctor':
      deps.setCurrentApp('health-check')
      return true
    case '/threads':
      deps.setCurrentApp('threads')
      return true
    case '/provider':
      openProviderSwitcher()
      return true
    case '/search':
      deps.setSearchOpen?.(true)
      return true
    case '/focus':
      post('/focus-mode', { focus_mode: true })
        .then(() => showNotice(t('messages.commands.focusOn'), 'success'))
        .catch(() => showNotice(t('messages.commands.focusFailed'), 'error'))
      return true
    case '/collab':
      post('/focus-mode', { focus_mode: false })
        .then(() => showNotice(t('messages.commands.focusOff'), 'success'))
        .catch(() => showNotice(t('messages.commands.focusFailed'), 'error'))
      return true
    case '/pause':
      post('/signals', { kind: 'pause', summary: 'Human paused all agents' })
        .then(() => showNotice(t('messages.commands.pauseOk'), 'success'))
        .catch((e: Error) => showNotice(t('messages.commands.pauseFailed', { error: e.message }), 'error'))
      return true
    case '/resume':
      post('/signals', { kind: 'resume', summary: 'Human resumed agents' })
        .then(() => showNotice(t('messages.commands.resumeOk'), 'success'))
        .catch((e: Error) => showNotice(t('messages.commands.resumeFailed', { error: e.message }), 'error'))
      return true
    case '/reset': {
      const runReset = () =>
        post('/reset', {})
          .then(() => {
            deps.setLastMessageId?.(null)
            deps.setCurrentChannel('general')
            deps.onReset?.()
            showNotice(t('messages.commands.resetOk'), 'success')
          })
          .catch((e: Error) => showNotice(t('messages.commands.resetFailed', { error: e.message }), 'error'))

      if (deps.resetRequiresConfirm) {
        confirm({
          title: t('messages.commands.resetTitle'),
          message: t('messages.commands.resetBody'),
          confirmLabel: t('messages.commands.resetConfirm'),
          danger: true,
          onConfirm: runReset,
        })
        return true
      }

      void runReset()
      return true
    }
    case '/1o1': {
      if (!args) {
        showNotice(t('messages.commands.oneOnOneUsage'), 'info')
        return true
      }
      if (!deps.enterDM) {
        showNotice(t('search.palette.requiresArgs', { name: cmd }), 'info')
        return true
      }
      const slug = args.trim().toLowerCase()
      createDM(slug)
        .then((data) => {
          const ch = extractDMChannelSlug(data, slug)
          deps.enterDM?.(slug, ch)
        })
        .catch(() => showNotice(t('messages.commands.agentNotFound', { slug: args.trim() }), 'error'))
      return true
    }
    case '/task': {
      const taskParts = args.split(/\s+/)
      const action = (taskParts[0] || '').toLowerCase()
      const taskId = taskParts[1] || ''
      const extra = taskParts.slice(2).join(' ')
      if (!action || !taskId) {
        showNotice(t('messages.commands.taskUsage'), 'info')
        return true
      }
      const body: Record<string, string> = { action, id: taskId, channel: deps.currentChannel }
      if (action === 'claim') body.owner = 'human'
      if (extra) body.details = extra
      post('/tasks', body)
        .then(() => showNotice(t('messages.commands.taskActionOk', { id: taskId, action }), 'success'))
        .catch((e: Error) => showNotice(t('messages.commands.taskActionFailed', { error: e.message }), 'error'))
      return true
    }
    case '/cancel':
      if (!args) {
        showNotice(t('messages.commands.cancelUsage'), 'info')
        return true
      }
      post('/tasks', { action: 'release', id: args.trim(), channel: deps.currentChannel })
        .then(() => showNotice(t('messages.commands.cancelOk', { id: args.trim() }), 'success'))
        .catch(() => showNotice(t('messages.commands.cancelFailed'), 'error'))
      return true
    default:
      return false
  }
}
