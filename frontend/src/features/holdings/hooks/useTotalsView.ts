import { useState } from 'react'
import { readTotalsView, type TotalsView, writeTotalsView } from '../lib/totalsViewStorage'

/**
 * window.localStorage を取り出す。保存を禁止したブラウザーでは、取り出すだけで例外になることがあるので、そのときは null（要件18）。
 */
function getLocalStorage(): Storage | null {
  try {
    return window.localStorage
  } catch {
    return null
  }
}

/**
 * 合計の欄の表示を、localStorage で覚えて読み書きする（要件16）。描画の直後から、覚えた表示で始める（state の初期化で読む）。
 * 選び直したら state と localStorage の両方を変える。サーバーには何も送らない（要件5）。
 * 保存できない環境でも例外を出さず、state だけで切り替わる（要件18）。
 */
export function useTotalsView(): { view: TotalsView; setView: (view: TotalsView) => void } {
  const [view, setViewState] = useState<TotalsView>(() => readTotalsView(getLocalStorage()))
  const setView = (next: TotalsView) => {
    setViewState(next)
    writeTotalsView(getLocalStorage(), next)
  }
  return { view, setView }
}
