export const ALL_APPS = [
  { id: 'studio', icon: '\u25B6', nameKey: 'sidebar.apps.studio' },
  { id: 'tasks', icon: '\uD83D\uDCE5', nameKey: 'sidebar.apps.tasks' },
  { id: 'deliveries', icon: '\uD83D\uDCE6', nameKey: 'sidebar.apps.deliveries' },
  { id: 'requests', icon: '\uD83D\uDCCB', nameKey: 'sidebar.apps.requests' },
  { id: 'policies', icon: '\uD83D\uDEE1', nameKey: 'sidebar.apps.policies' },
  { id: 'calendar', icon: '\uD83D\uDCC5', nameKey: 'sidebar.apps.calendar' },
  { id: 'skills', icon: '\u26A1', nameKey: 'sidebar.apps.skills' },
  { id: 'activity', icon: '\uD83D\uDCE6', nameKey: 'sidebar.apps.activity' },
  { id: 'receipts', icon: '\uD83E\uDDFE', nameKey: 'sidebar.apps.receipts' },
  { id: 'health-check', icon: '\uD83D\uDD0D', nameKey: 'sidebar.apps.healthCheck' },
  { id: 'settings', icon: '\u2699', nameKey: 'sidebar.apps.settings' },
] as const

export const SECONDARY_APPS = ALL_APPS.filter((app) => app.id !== 'studio' && app.id !== 'activity')

export const HOME_APP_ID = 'studio'
export const ACTIVITY_APP_ID = 'activity'

export const DISCONNECT_THRESHOLD = 3
export const MESSAGE_POLL_INTERVAL = 2000
export const MEMBER_POLL_INTERVAL = 5000
export const REQUEST_POLL_INTERVAL = 3000
