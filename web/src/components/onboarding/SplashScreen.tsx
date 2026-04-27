import { useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'

const AUTO_DISMISS_MS = 2000

interface SplashScreenProps {
  onDone: () => void
}

export function SplashScreen({ onDone }: SplashScreenProps) {
  const { t } = useTranslation()
  const dismiss = useCallback(() => {
    onDone()
  }, [onDone])

  useEffect(() => {
    const timer = setTimeout(dismiss, AUTO_DISMISS_MS)
    return () => clearTimeout(timer)
  }, [dismiss])

  return (
    <div
      className="launch-screen"
      onClick={dismiss}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') dismiss()
      }}
      aria-label={t('splash.dismissAria')}
    >
      <div className="launch-logo">DunderIA</div>
      <div className="launch-spinner" />
      <p className="launch-text">{t('splash.opening')}</p>
      <p className="launch-sub">{t('splash.subtitle')}</p>
    </div>
  )
}
