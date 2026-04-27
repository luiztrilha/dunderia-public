import { useEffect, useState } from 'react'

export function usePageActivity() {
  const [isPageVisible, setIsPageVisible] = useState(() =>
    typeof document === 'undefined' ? true : document.visibilityState !== 'hidden',
  )
  const [isWindowFocused, setIsWindowFocused] = useState(() =>
    typeof document === 'undefined' ? true : document.hasFocus(),
  )

  useEffect(() => {
    if (typeof document === 'undefined') return undefined

    const syncVisibility = () => {
      setIsPageVisible(document.visibilityState !== 'hidden')
    }
    const syncFocus = () => {
      setIsWindowFocused(document.hasFocus())
    }

    syncVisibility()
    syncFocus()

    document.addEventListener('visibilitychange', syncVisibility)
    window.addEventListener('focus', syncFocus)
    window.addEventListener('blur', syncFocus)

    return () => {
      document.removeEventListener('visibilitychange', syncVisibility)
      window.removeEventListener('focus', syncFocus)
      window.removeEventListener('blur', syncFocus)
    }
  }, [])

  return {
    isPageVisible,
    isWindowFocused,
    isPageActive: isPageVisible && isWindowFocused,
  }
}
