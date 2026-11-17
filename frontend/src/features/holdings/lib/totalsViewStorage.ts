// 合計の欄で選んだ表示を、ブラウザーに覚えさせる（ドルを円でまとめる合計の要件16・要件18）。サーバーには保存しない。

/** 合計の欄の表示。通貨ごと（今までの表示）か、ドルを円にまとめた表示。 */
export type TotalsView = 'byCurrency' | 'combinedYen'

/** localStorage のキー。 */
export const TOTALS_VIEW_STORAGE_KEY = 'zero.holdings.totalsView'

/** 何も覚えていないとき、覚えている値が知らない値のとき、読めないときの表示（要件16・要件18）。 */
const DEFAULT_TOTALS_VIEW: TotalsView = 'byCurrency'

/**
 * 覚えた表示を読む。storage が null・読むと例外・知らない値なら 'byCurrency'（例外は外に出さない）。
 * Storage は引数で受け取り、テストで差し替えられるようにする。
 */
export function readTotalsView(storage: Pick<Storage, 'getItem'> | null): TotalsView {
  if (storage === null) return DEFAULT_TOTALS_VIEW
  try {
    const value = storage.getItem(TOTALS_VIEW_STORAGE_KEY)
    return value === 'combinedYen' ? 'combinedYen' : DEFAULT_TOTALS_VIEW
  } catch {
    // 保存が禁止されているブラウザーでは、読むだけで例外になることがある。
    return DEFAULT_TOTALS_VIEW
  }
}

/** 表示を覚えさせる。storage が null・書くと例外（保存が禁止、容量超過）なら何もしない（例外は外に出さない）。 */
export function writeTotalsView(storage: Pick<Storage, 'setItem'> | null, view: TotalsView): void {
  if (storage === null) return
  try {
    storage.setItem(TOTALS_VIEW_STORAGE_KEY, view)
  } catch {
    // 覚えさせられなくても、画面の表示は変わる（次に開いたときは「通貨ごと」で始まる）。
  }
}
