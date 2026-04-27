import { useEffect, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useOfficeMembers } from '../../hooks/useMembers'

export interface AutocompleteItem {
  /** Token to insert (e.g. "/clear" or "@ceo"). */
  insert: string
  /** Primary label shown in the panel. */
  label: string
  /** Secondary description. */
  desc?: string
  /** Leading glyph. */
  icon?: string
}

export interface SlashCommand {
  name: string
  descKey: string
  icon: string
}

export const SLASH_COMMANDS: SlashCommand[] = [
  { name: '/clear', descKey: 'messages.slash.clear', icon: '\uD83E\uDDF9' },
  { name: '/help', descKey: 'messages.slash.help', icon: '\u2753' },
  { name: '/reset', descKey: 'messages.slash.reset', icon: '\uD83D\uDD04' },
  { name: '/search', descKey: 'messages.slash.search', icon: '\uD83D\uDD0E' },
  { name: '/tasks', descKey: 'messages.slash.tasks', icon: '\uD83D\uDCCB' },
  { name: '/requests', descKey: 'messages.slash.requests', icon: '\uD83D\uDD14' },
  { name: '/recover', descKey: 'messages.slash.recover', icon: '\uD83D\uDD01' },
  { name: '/1o1', descKey: 'messages.slash.oneOnOne', icon: '\uD83D\uDCAC' },
  { name: '/task', descKey: 'messages.slash.task', icon: '\u2705' },
  { name: '/cancel', descKey: 'messages.slash.cancel', icon: '\u274C' },
  { name: '/policies', descKey: 'messages.slash.policies', icon: '\uD83D\uDCDC' },
  { name: '/calendar', descKey: 'messages.slash.calendar', icon: '\uD83D\uDCC5' },
  { name: '/skills', descKey: 'messages.slash.skills', icon: '\u26A1' },
  { name: '/focus', descKey: 'messages.slash.focus', icon: '\uD83C\uDFAF' },
  { name: '/collab', descKey: 'messages.slash.collab', icon: '\uD83E\uDD1D' },
  { name: '/pause', descKey: 'messages.slash.pause', icon: '\u23F8' },
  { name: '/resume', descKey: 'messages.slash.resume', icon: '\u25B6' },
  { name: '/threads', descKey: 'messages.slash.threads', icon: '\uD83E\uDDF5' },
  { name: '/provider', descKey: 'messages.slash.provider', icon: '\u2699' },
]

interface AutocompleteProps {
  /** Current composer text. */
  value: string
  /** Caret position in the textarea (0-based). */
  caret: number
  /** Currently highlighted item index, set by parent. */
  selectedIdx: number
  /** Notify parent of total visible items so it can clamp selectedIdx. */
  onItems: (items: AutocompleteItem[]) => void
  /** Pick an item: parent rewrites the text. */
  onPick: (item: AutocompleteItem) => void
}

/**
 * Renders the autocomplete panel above the composer. The parent owns
 * keyboard handling (up/down/enter/tab/escape) — this component only
 * paints, calculates the items list, and reports it.
 */
export function Autocomplete({ value, caret, selectedIdx, onItems, onPick }: AutocompleteProps) {
  const { t } = useTranslation()
  const { data: members = [] } = useOfficeMembers()
  const listRef = useRef<HTMLDivElement>(null)

  const items = useMemo<AutocompleteItem[]>(() => {
    const trigger = currentTrigger(value, caret)
    if (!trigger) return []
    if (trigger.kind === 'slash') {
      const q = trigger.query.toLowerCase()
      return SLASH_COMMANDS
        .filter((c) => c.name.slice(1).toLowerCase().startsWith(q))
        .slice(0, 8)
        .map((c) => ({ insert: c.name, label: c.name, desc: t(c.descKey), icon: c.icon }))
    }
    const q = trigger.query.toLowerCase()
    return members
      .filter((m) => m.slug && m.slug !== 'human' && m.slug !== 'you')
      .filter((m) => {
        if (!q) return true
        return (
          (m.slug || '').toLowerCase().includes(q) ||
          (m.name || '').toLowerCase().includes(q)
        )
      })
      .slice(0, 8)
      .map((m) => ({
        insert: '@' + m.slug,
        label: '@' + m.slug,
        desc: m.name,
        icon: m.emoji || '\uD83E\uDD16',
      }))
  }, [value, caret, members, t])

  useEffect(() => {
    onItems(items)
  }, [items, onItems])

  // Scroll selected into view
  useEffect(() => {
    const el = listRef.current?.children[selectedIdx] as HTMLElement | undefined
    el?.scrollIntoView({ block: 'nearest' })
  }, [selectedIdx])

  if (items.length === 0) return null

  return (
    <div ref={listRef} className="autocomplete open">
      {items.map((item, idx) => (
        <button
          key={item.insert}
          type="button"
          className={`autocomplete-item${idx === selectedIdx ? ' selected' : ''}`}
          onMouseDown={(e) => {
            // Prevent textarea blur before the click registers
            e.preventDefault()
            onPick(item)
          }}
        >
          <span className="autocomplete-item-icon">{item.icon}</span>
          <span className="autocomplete-item-label">{item.label}</span>
          {item.desc && <span className="autocomplete-item-desc">{item.desc}</span>}
        </button>
      ))}
    </div>
  )
}

/**
 * Inspect the text up to the caret and return the active trigger token.
 *
 * Returns:
 *  - {kind: 'slash', query, start} when the input starts with `/` and no space yet
 *  - {kind: 'mention', query, start} when the caret is inside an `@token` (no whitespace)
 *  - null otherwise
 */
export function currentTrigger(
  value: string,
  caret: number,
): { kind: 'slash' | 'mention'; query: string; start: number } | null {
  const before = value.slice(0, caret)
  // Slash must be at the very start of input (legacy behavior)
  if (/^\/\S*$/.test(value) && caret > 0) {
    return { kind: 'slash', query: value.slice(1, caret), start: 0 }
  }
  // @-mention: find the last @ that is preceded by start-of-string or whitespace,
  // and has no whitespace between it and the caret.
  const atIdx = before.lastIndexOf('@')
  if (atIdx === -1) return null
  const prevChar = atIdx === 0 ? '' : before[atIdx - 1]
  if (prevChar !== '' && !/\s/.test(prevChar)) return null
  const tail = before.slice(atIdx + 1)
  if (/\s/.test(tail)) return null
  return { kind: 'mention', query: tail, start: atIdx }
}

/**
 * Replace the active trigger token in `value` with `insert + ' '`.
 * Returns the new text and the new caret position.
 */
export function applyAutocomplete(
  value: string,
  caret: number,
  item: AutocompleteItem,
): { text: string; caret: number } {
  const trigger = currentTrigger(value, caret)
  if (!trigger) return { text: value, caret }
  const before = value.slice(0, trigger.start)
  const after = value.slice(caret)
  const insert = item.insert + ' '
  return { text: before + insert + after, caret: before.length + insert.length }
}
