import { useEffect, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { getUsage } from '../../api/client'
import { formatUSD, formatTokens } from '../../lib/format'
import { useBrokerRefetchInterval } from '../../hooks/useBrokerEvents'
import { usageKey } from '../../lib/queryKeys'

type UsagePanelProps = {
  compact?: boolean
}

export function UsagePanel({ compact = false }: UsagePanelProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const rootRef = useRef<HTMLDivElement | null>(null)
  const panelRef = useRef<HTMLDivElement | null>(null)
  const [popoverStyle, setPopoverStyle] = useState<CSSProperties>()
  const refetchInterval = useBrokerRefetchInterval(open ? 5000 : 30_000)
  const { data: usage } = useQuery({
    queryKey: usageKey(),
    queryFn: () => getUsage(),
    refetchInterval,
    staleTime: 30_000,
  })

  const totalCost = usage?.total?.cost_usd ?? 0
  const agents = usage?.agents ?? {}
  const slugs = Object.keys(agents).sort()

  useEffect(() => {
    if (!compact || !open) return

    const handlePointerDown = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) {
        setOpen(false)
      }
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setOpen(false)
      }
    }

    document.addEventListener('pointerdown', handlePointerDown)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [compact, open])

  useEffect(() => {
    if (!compact || !open) return

    const updatePosition = () => {
      const triggerRect = rootRef.current?.getBoundingClientRect()
      const panelRect = panelRef.current?.getBoundingClientRect()
      if (!triggerRect || !panelRect) return

      const gutter = 8
      const maxWidth = Math.min(244, window.innerWidth - gutter * 2)
      const panelWidth = Math.min(panelRect.width || 244, maxWidth)
      const left = Math.min(
        Math.max(triggerRect.right - panelWidth, gutter),
        window.innerWidth - panelWidth - gutter,
      )

      setPopoverStyle({
        position: 'fixed',
        top: Math.round(triggerRect.bottom + gutter),
        left: Math.round(left),
        width: Math.round(panelWidth),
      })
    }

    updatePosition()
    window.addEventListener('resize', updatePosition)
    window.addEventListener('scroll', updatePosition, true)
    return () => {
      window.removeEventListener('resize', updatePosition)
      window.removeEventListener('scroll', updatePosition, true)
    }
  }, [compact, open, usage?.total?.cost_usd, slugs.length])

  const usageContent = slugs.length === 0 && totalCost === 0 ? (
    <p style={{ fontSize: 11, color: 'var(--text-tertiary)', padding: '4px 0' }}>
      {t('sidebar.usage.empty')}
    </p>
  ) : (
    <>
      <table className="usage-table">
        <thead>
          <tr>
            {(['agent', 'in', 'out', 'cache', 'cost'] as const).map((k) => (
              <th key={k}>{t(`sidebar.usage.headers.${k}`)}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {slugs.map((slug) => {
            const a = agents[slug]
            return (
              <tr key={slug}>
                <td>{slug}</td>
                <td>{formatTokens(a.input_tokens)}</td>
                <td>{formatTokens(a.output_tokens)}</td>
                <td>{formatTokens(a.cache_read_tokens)}</td>
                <td>{formatUSD(a.cost_usd)}</td>
              </tr>
            )
          })}
        </tbody>
      </table>
      <div className="usage-total">
        <span>{t('sidebar.usage.session', { tokens: formatTokens(usage?.session?.total_tokens ?? 0) })}</span>
        <span className="usage-total-cost">{formatUSD(totalCost)}</span>
      </div>
    </>
  )

  if (compact) {
    return (
      <div ref={rootRef} className="sidebar-utility-group">
        <button
          className={`sidebar-utility-button sidebar-usage-button${open ? ' open' : ''}`}
          onClick={() => setOpen((v) => !v)}
          aria-expanded={open}
        >
          <span className="sidebar-utility-button-text">
            <span className="sidebar-utility-label">{t('sidebar.usage.title')}</span>
            <span className="sidebar-utility-value">{formatUSD(totalCost)}</span>
          </span>
          <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="m9 18 6-6-6-6" />
          </svg>
        </button>
        {open && (
          <div ref={panelRef} className="usage-panel usage-panel-popover open" style={popoverStyle}>
            {usageContent}
          </div>
        )}
      </div>
    )
  }

  return (
    <>
      <button
        className={`usage-toggle${open ? ' open' : ''}`}
        onClick={() => setOpen((v) => !v)}
      >
        <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="m9 18 6-6-6-6" />
        </svg>
        {t('sidebar.usage.title')}
        <span style={{ marginLeft: 'auto', fontWeight: 400, color: 'var(--accent)' }}>
          {formatUSD(totalCost)}
        </span>
      </button>
      {open && (
        <div className="usage-panel open">
          {usageContent}
        </div>
      )}
    </>
  )
}
