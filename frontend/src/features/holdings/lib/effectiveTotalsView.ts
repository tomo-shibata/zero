import type { CombinedTotal } from '../../../entities/holding/model'
import type { TotalsView } from './totalsViewStorage'

/**
 * 実際に出す表示を決める（要件17）。combinedTotal が null（切り替えを出さない状態）なら、覚えた表示によらず 'byCurrency'。
 * 覚えた値は書き換えない（切り替えを出せる状態に戻ったら、覚えた表示で出す）。
 */
export function effectiveTotalsView(stored: TotalsView, combinedTotal: CombinedTotal | null): TotalsView {
  return combinedTotal === null ? 'byCurrency' : stored
}
