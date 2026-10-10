import { formatYen } from '../../../shared/lib/format'
import './HoldingsTotal.css'

const TOTAL_LABEL = '合計評価額'
const UNPRICED_NOTE = '価格を取得できない銘柄を含みません'

type HoldingsTotalProps = {
  /** 合計評価額（円の整数）。 */
  totalValuation: number
  /** 価格のない銘柄が1件以上あるか。 */
  excludesUnpriced: boolean
}

/**
 * 保有株式全体の合計評価額を表示する（要件 FR-6）。
 * 価格のない銘柄があれば、合計評価額に含まないことを添える。
 */
export function HoldingsTotal({ totalValuation, excludesUnpriced }: HoldingsTotalProps) {
  return (
    <section aria-label={TOTAL_LABEL} className="holdings-total">
      <p className="holdings-total__label">{TOTAL_LABEL}</p>
      <p className="holdings-total__amount">{formatYen(totalValuation)}</p>
      {excludesUnpriced && <p className="holdings-total__note">{UNPRICED_NOTE}</p>}
    </section>
  )
}
