import type { CurrencyTotal } from '../../../entities/holding/model'
import { formatValuation } from '../../../shared/lib/format'
import './HoldingsTotal.css'

const TOTAL_LABEL = '合計評価額'
const UNPRICED_NOTE = '価格を取得できない銘柄を含みません'

type HoldingsTotalProps = {
  /** 通貨ごとの合計評価額（保有のある通貨だけ。円、ドルの順）。空のときは、この部品を描画しない（呼び出し側が決める）。 */
  totals: readonly CurrencyTotal[]
}

/**
 * 保有株式全体の合計評価額を、通貨ごとに1行ずつ表示する（要件12・13・14）。為替の換算はしない。
 * 「合計評価額」の見出しは1回だけ出し、その下の一覧に通貨ごとの行を渡された順（円、ドル）に入れる。
 * 価格のない銘柄がある通貨の行には、その通貨の合計評価額に含まないことを添える。
 */
export function HoldingsTotal({ totals }: HoldingsTotalProps) {
  return (
    <section aria-label={TOTAL_LABEL} className="holdings-total">
      <p className="holdings-total__label">{TOTAL_LABEL}</p>
      {/* list-style: none だと Safari の VoiceOver がリストとして読まないことがあるので、role を明示する。 */}
      <ul role="list" className="holdings-total__list">
        {totals.map((total) => (
          <li key={total.currency} className="holdings-total__item">
            <p className="holdings-total__amount">{formatValuation(total.currency, total.valuation)}</p>
            {total.excludesUnpriced && <p className="holdings-total__note">{UNPRICED_NOTE}</p>}
          </li>
        ))}
      </ul>
    </section>
  )
}
