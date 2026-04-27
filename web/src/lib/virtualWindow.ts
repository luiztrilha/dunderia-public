import { useEffect, useMemo, useRef, useState, type RefObject } from 'react'

type VirtualWindowOptions<T> = {
  items: T[]
  containerRef: RefObject<HTMLElement | null>
  getKey: (item: T, index: number) => string
  estimateSize: (item: T, index: number) => number
  overscan?: number
  enabled?: boolean
}

type VirtualWindowRange = {
  startIndex: number
  endIndex: number
  totalHeight: number
  offsets: number[]
  registerItem: (index: number) => (node: HTMLElement | null) => void
}

export function useVirtualWindow<T>({
  items,
  containerRef,
  getKey,
  estimateSize,
  overscan = 6,
  enabled = true,
}: VirtualWindowOptions<T>): VirtualWindowRange {
  const sizesByKeyRef = useRef(new Map<string, number>())
  const nodesByKeyRef = useRef(new Map<string, HTMLElement>())
  const observersByKeyRef = useRef(new Map<string, ResizeObserver>())
  const getKeyRef = useRef(getKey)
  const estimateRef = useRef(estimateSize)
  const [version, setVersion] = useState(0)
  const [range, setRange] = useState({ startIndex: 0, endIndex: items.length })

  useEffect(() => {
    getKeyRef.current = getKey
  }, [getKey])

  useEffect(() => {
    estimateRef.current = estimateSize
  }, [estimateSize])

  useEffect(() => {
    const currentKeys = new Set<string>()
    let changed = false

    for (let index = 0; index < items.length; index += 1) {
      const key = getKeyRef.current(items[index], index)
      currentKeys.add(key)
      if (!sizesByKeyRef.current.has(key)) {
        sizesByKeyRef.current.set(key, estimateRef.current(items[index], index))
        changed = true
      }
    }

    for (const key of Array.from(sizesByKeyRef.current.keys())) {
      if (currentKeys.has(key)) continue
      sizesByKeyRef.current.delete(key)
      nodesByKeyRef.current.delete(key)
      const observer = observersByKeyRef.current.get(key)
      if (observer) observer.disconnect()
      observersByKeyRef.current.delete(key)
      changed = true
    }

    if (changed) {
      setVersion((next) => next + 1)
    }
  }, [items])

  const offsets = useMemo(() => {
    const nextOffsets = [0]
    for (let index = 0; index < items.length; index += 1) {
      const key = getKeyRef.current(items[index], index)
      const size = sizesByKeyRef.current.get(key) ?? estimateRef.current(items[index], index)
      nextOffsets.push(nextOffsets[nextOffsets.length - 1] + size)
    }
    return nextOffsets
  }, [estimateSize, items, version])

  const totalHeight = offsets[offsets.length - 1] ?? 0

  useEffect(() => {
    if (!enabled) {
      setRange({ startIndex: 0, endIndex: items.length })
      return undefined
    }

    const node = containerRef.current
    if (!node) return undefined

    let raf = 0

    const updateRange = () => {
      if (!enabled) return
      const scrollTop = Math.max(0, node.scrollTop)
      const viewportHeight = Math.max(0, node.clientHeight)
      const lowerBound = scrollTop
      const upperBound = scrollTop + viewportHeight

      let startIndex = 0
      for (let index = 0; index < items.length; index += 1) {
        const itemBottom = offsets[index + 1] ?? offsets[index] ?? 0
        if (itemBottom > lowerBound) {
          startIndex = Math.max(0, index - overscan)
          break
        }
      }

      let endIndex = items.length
      for (let index = startIndex; index < items.length; index += 1) {
        const itemTop = offsets[index] ?? 0
        if (itemTop >= upperBound) {
          endIndex = Math.min(items.length, index + overscan)
          break
        }
      }

      setRange((current) => {
        if (current.startIndex === startIndex && current.endIndex === endIndex) {
          return current
        }
        return { startIndex, endIndex }
      })
    }

    const scheduleUpdate = () => {
      if (raf) return
      raf = window.requestAnimationFrame(() => {
        raf = 0
        updateRange()
      })
    }

    updateRange()
    node.addEventListener('scroll', scheduleUpdate, { passive: true })
    window.addEventListener('resize', scheduleUpdate)

    const observer = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(scheduleUpdate) : null
    observer?.observe(node)

    return () => {
      node.removeEventListener('scroll', scheduleUpdate)
      window.removeEventListener('resize', scheduleUpdate)
      observer?.disconnect()
      if (raf) window.cancelAnimationFrame(raf)
    }
  }, [containerRef, enabled, items.length, offsets, overscan])

  useEffect(() => {
    if (!enabled) return
    setRange((current) => {
      const nextEnd = Math.min(items.length, Math.max(current.endIndex, current.startIndex))
      return {
        startIndex: Math.min(current.startIndex, items.length),
        endIndex: Math.max(nextEnd, Math.min(items.length, current.endIndex)),
      }
    })
  }, [enabled, items.length, version])

  const registerItem = (index: number) => (node: HTMLElement | null) => {
    const item = items[index]
    if (!item) return

    const key = getKeyRef.current(item, index)
    const existingNode = nodesByKeyRef.current.get(key)
    if (existingNode === node) return

    const existingObserver = observersByKeyRef.current.get(key)
    if (existingObserver) {
      existingObserver.disconnect()
      observersByKeyRef.current.delete(key)
    }

    if (!node) {
      nodesByKeyRef.current.delete(key)
      return
    }

    nodesByKeyRef.current.set(key, node)

    const measure = () => {
      const nextSize = Math.ceil(node.getBoundingClientRect().height)
      if (!Number.isFinite(nextSize) || nextSize <= 0) return
      const currentSize = sizesByKeyRef.current.get(key)
      if (currentSize === nextSize) return
      sizesByKeyRef.current.set(key, nextSize)
      setVersion((next) => next + 1)
    }

    measure()

    if (typeof ResizeObserver !== 'undefined') {
      const observer = new ResizeObserver(measure)
      observer.observe(node)
      observersByKeyRef.current.set(key, observer)
    }
  }

  return {
    startIndex: range.startIndex,
    endIndex: range.endIndex,
    totalHeight,
    offsets,
    registerItem,
  }
}
