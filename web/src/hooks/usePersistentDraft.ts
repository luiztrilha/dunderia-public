import { useCallback, useState, type Dispatch, type SetStateAction } from 'react'

function canUseStorage(): boolean {
  return typeof window !== 'undefined' && typeof window.localStorage !== 'undefined'
}

interface PersistentDraftOptions<T> {
  persist?: boolean
  serialize?: (value: T) => T
  hydrate?: (stored: T, fallback: T) => T
}

function readStoredValue<T>(key: string, fallback: T, hydrate?: (stored: T, fallback: T) => T): T {
  if (!canUseStorage()) return fallback
  try {
    const raw = window.localStorage.getItem(key)
    if (!raw) return fallback
    const stored = JSON.parse(raw) as T
    return hydrate ? hydrate(stored, fallback) : stored
  } catch {
    return fallback
  }
}

function writeStoredValue<T>(key: string, value: T, serialize?: (value: T) => T): void {
  if (!canUseStorage()) return
  try {
    const persisted = serialize ? serialize(value) : value
    window.localStorage.setItem(key, JSON.stringify(persisted))
  } catch {
    // Storage can be unavailable or quota-limited; drafts should degrade to memory-only.
  }
}

export function usePersistentDraft<T>(
  storageKey: string,
  initialValue: T,
  options?: PersistentDraftOptions<T>,
) {
  const persist = options?.persist ?? true

  const [value, setValue] = useState<T>(() =>
    persist ? readStoredValue(storageKey, initialValue, options?.hydrate) : initialValue,
  )

  const setPersistentValue = useCallback<Dispatch<SetStateAction<T>>>((next) => {
    setValue((current) => {
      const resolved = typeof next === 'function' ? (next as (prev: T) => T)(current) : next
      if (persist) {
        writeStoredValue(storageKey, resolved, options?.serialize)
      }
      return resolved
    })
  }, [options?.serialize, persist, storageKey])

  const clear = useCallback(() => {
    setValue(initialValue)
    if (!canUseStorage()) return
    try {
      window.localStorage.removeItem(storageKey)
    } catch {
      // Ignore storage cleanup failures.
    }
  }, [initialValue, storageKey])

  const clearStorage = useCallback(() => {
    if (!canUseStorage()) return
    try {
      window.localStorage.removeItem(storageKey)
    } catch {
      // Ignore storage cleanup failures.
    }
  }, [storageKey])

  return {
    value,
    setValue: setPersistentValue,
    clear,
    clearStorage,
  }
}
