import { useEffect, useMemo, useState } from 'react'
import type { TFunction } from 'i18next'
import type {
  StudioActionDefinition,
  StudioActionInvocation,
  StudioBlocker,
} from '../../../api/client'

interface StudioActionBarProps {
  blocker: StudioBlocker
  actionDefinitions: Record<string, StudioActionDefinition>
  membersByChannel: Record<string, string[]>
  pendingKey: string | null
  onAction: (action: string, blocker: StudioBlocker, extras?: { owner?: string }) => void
  t: TFunction
}

const DELEGATE_GAME_MASTER_ACTION = 'delegate_game_master'

function actionKey(blockerId: string, action: string): string {
  return `${blockerId}:${action}`
}

export function StudioActionBar({
  blocker,
  actionDefinitions,
  membersByChannel,
  pendingKey,
  onAction,
  t,
}: StudioActionBarProps) {
  const members = useMemo(() => {
    const pool = membersByChannel[blocker.channel ?? ''] ?? []
    return pool.filter((slug, index) => slug && slug !== blocker.owner && pool.indexOf(slug) === index)
  }, [blocker.channel, blocker.owner, membersByChannel])

  const actions = useMemo(() => {
    return (blocker.available_actions ?? []).filter((action) => actionDefinitions[action.action] != null)
  }, [actionDefinitions, blocker.available_actions])

  const [selectedOwner, setSelectedOwner] = useState<string>('')
  const canDelegateToGameMaster = Boolean(blocker.task_id) && blocker.owner !== 'game-master'
  const delegatingToGameMaster = pendingKey === actionKey(blocker.id, DELEGATE_GAME_MASTER_ACTION)

  useEffect(() => {
    setSelectedOwner((current) => {
      if (current && members.includes(current)) return current
      return members[0] ?? ''
    })
  }, [members])

  if (actions.length === 0 && !canDelegateToGameMaster) {
    return null
  }

  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, alignItems: 'center' }}>
      {canDelegateToGameMaster && (
        <button
          className="btn btn-sm btn-primary"
          disabled={delegatingToGameMaster}
          onClick={() => onAction(DELEGATE_GAME_MASTER_ACTION, blocker)}
          title={t('apps.studio.delegateGameMasterTitle')}
        >
          {delegatingToGameMaster ? t('apps.studio.delegatingGameMaster') : t('apps.studio.delegateGameMaster')}
        </button>
      )}
      {actions.map((action) => {
        if (action.action === 'reassign_task') {
          const disabled = members.length === 0 || !selectedOwner || pendingKey === actionKey(blocker.id, action.action)
          return (
            <div key={action.action} style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
              <select
                value={selectedOwner}
                onChange={(event) => setSelectedOwner(event.target.value)}
                disabled={members.length === 0}
                style={{
                  minWidth: 144,
                  padding: '7px 10px',
                  borderRadius: 999,
                  border: '1px solid var(--border)',
                  background: 'var(--bg)',
                  color: 'var(--text-primary)',
                  fontSize: 12,
                }}
              >
                {members.length === 0 ? (
                  <option value="">No owners</option>
                ) : (
                  members.map((member) => (
                    <option key={member} value={member}>
                      @{member}
                    </option>
                  ))
                )}
              </select>
              <button
                className={`btn btn-sm ${blocker.recommended_action === action.action ? 'btn-primary' : 'btn-secondary'}`}
                disabled={disabled}
                onClick={() => onAction(action.action, blocker, { owner: selectedOwner })}
              >
                {pendingKey === actionKey(blocker.id, action.action) ? 'Working...' : action.label}
              </button>
            </div>
          )
        }

        return (
          <button
            key={action.action}
            className={`btn btn-sm ${blocker.recommended_action === action.action ? 'btn-primary' : 'btn-secondary'}`}
            disabled={pendingKey === actionKey(blocker.id, action.action)}
            onClick={() => onAction(action.action, blocker)}
          >
            {pendingKey === actionKey(blocker.id, action.action) ? 'Working...' : action.label}
          </button>
        )
      })}
    </div>
  )
}
