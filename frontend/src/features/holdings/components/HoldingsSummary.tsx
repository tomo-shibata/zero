import { useId } from 'react'
import type { CurrencyTotal } from '../../../entities/holding/model'
import type { Currency } from '../../../shared/lib/currency'
import { formatValuation } from '../../../shared/lib/format'
import { UnrealizedGainText } from './UnrealizedGainText'
import './HoldingsSummary.css'

const SUMMARY_TITLE = '合計'
const GAIN_LABEL = '含み損益'
const VALUATION_LABEL = '評価額'
const UNPRICED_NOTE = '価格を取得できない銘柄を含みません'

/** 通貨ごとの部分の見出し。 */
const CURRENCY_NAMES: Record<Currency, string> = { JPY: '円', USD: 'ドル' }

type HoldingsSummaryProps = {
  /** 通貨ごとの合計（保有のある通貨だけ。円、ドルの順）。空のときは、この部品を描画しない（呼び出し側が決める）。 */
  totals: readonly CurrencyTotal[]
}

/**
 * 保有株式全体の合計の欄（要件11〜16）。為替の換算はしない。
 * 見出し「合計」（h2）の下に、通貨ごとの部分を渡された順（円、ドル）に置く。
 */
export function HoldingsSummary({ totals }: HoldingsSummaryProps) {
  const titleId = useId()
  return (
    <section aria-labelledby={titleId} className="holdings-summary">
      <h2 id={titleId} className="holdings-summary__title">
        {SUMMARY_TITLE}
      </h2>
      <div className="holdings-summary__currencies">
        {totals.map((total) => (
          <CurrencySummary key={total.currency} total={total} />
        ))}
      </div>
    </section>
  )
}

/**
 * 1つの通貨の部分。見出し（h3）、含み損益の合計とその率（いちばん大きな文字）、評価額の合計、注記の順。
 * 含み損益の合計が null（その通貨に価格のある銘柄がない）なら、含み損益は「取得できません」（率なし、色なし。要件16）。
 * 価格のない銘柄がある通貨には、含み損益と評価額の合計にその銘柄を含まないことを添える（要件15）。
 */
function CurrencySummary({ total }: { total: CurrencyTotal }) {
  const titleId = useId()
  return (
    <section aria-labelledby={titleId} className="holdings-summary__currency">
      <h3 id={titleId} className="holdings-summary__currency-title">
        {CURRENCY_NAMES[total.currency]}
      </h3>
      <dl className="holdings-summary__details">
        <dt className="holdings-summary__term">{GAIN_LABEL}</dt>
        <dd className="holdings-summary__gain">
          <UnrealizedGainText
            currency={total.currency}
            gain={total.unrealizedGain}
            rate={total.unrealizedGainRate}
          />
        </dd>
        <dt className="holdings-summary__term">{VALUATION_LABEL}</dt>
        <dd className="holdings-summary__valuation">{formatValuation(total.currency, total.valuation)}</dd>
      </dl>
      {total.excludesUnpriced && <p className="holdings-summary__note">{UNPRICED_NOTE}</p>}
    </section>
  )
}
