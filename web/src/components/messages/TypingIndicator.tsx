import { useTranslation } from 'react-i18next'
import { useOfficeMembers } from '../../hooks/useMembers'

export function TypingIndicator() {
  const { t } = useTranslation()
  const { data: members = [] } = useOfficeMembers()

  // Show typing for any member with 'active' status
  const active = members.filter((m) => m.status === 'active' && m.slug !== 'human')

  if (active.length === 0) return null

  const names = active.map((m) => m.name || m.slug)
  const label = names.length === 1
    ? t('messages.typing.one', { name: names[0] })
    : names.length <= 3
      ? t('messages.typing.few', { names: names.join(', ') })
      : t('messages.typing.many', { count: names.length })

  return (
    <div className="typing-indicator" style={{ padding: '0 20px 8px' }}>
      <div className="typing-dots">
        <span className="typing-dot" />
        <span className="typing-dot" />
        <span className="typing-dot" />
      </div>
      <span>{label}</span>
    </div>
  )
}
