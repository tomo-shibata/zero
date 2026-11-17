import { useLayoutEffect, useRef, useState } from 'react'
import type { RefObject } from 'react'

export type ElementSize = { width: number; height: number }

/**
 * 要素の今の大きさ（px）を返す。要素の大きさが変わったら（画面の幅・高さの変化、向きの変更）取り直す。
 * 最初の測定の前と、ResizeObserver のない環境（テストの jsdom）では null。
 */
export function useElementSize<T extends HTMLElement>(): { ref: RefObject<T | null>; size: ElementSize | null } {
  const ref = useRef<T | null>(null)
  const [size, setSize] = useState<ElementSize | null>(null)

  useLayoutEffect(() => {
    const element = ref.current
    if (element === null || typeof ResizeObserver === 'undefined') return
    const measure = () => {
      const { width, height } = element.getBoundingClientRect()
      // 値が同じなら state を変えない（無駄な再描画をしない）。
      setSize((previous) => (previous?.width === width && previous.height === height ? previous : { width, height }))
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [])

  return { ref, size }
}
