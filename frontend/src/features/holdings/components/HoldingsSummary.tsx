import { useId } from 'react'
import type { CombinedTotal, CurrencyTotal } from '../../../entities/holding/model'
import type { Currency } from '../../../shared/lib/currency'
import { formatValuation } from '../../../shared/lib/format'
import { effectiveTotalsView } from '../lib/effectiveTotalsView'
import type { TotalsView } from '../lib/totalsViewStorage'
import { CombinedSummary } from './CombinedSummary'
import { GAIN_LABEL, UNPRICED_NOTE, VALUATION_LABEL } from './summaryLabels'
import { TotalsViewSwitch } from './TotalsViewSwitch'
import { UnrealizedGainText } from './UnrealizedGainText'
import './HoldingsSummary.css'

const SUMMARY_TITLE = '合計'

/** 通貨ごとの部分の見出し。 */
const CURRENCY_NAMES: Record<Currency, string> = { JPY: '円', USD: 'ドル' }

type HoldingsSummaryProps = {
  /** 通貨ごとの合計（保有のある通貨だけ。円、ドルの順）。空のときは、この部品を描画しない（呼び出し側が決める）。 */
  totals: readonly CurrencyTotal[]
  /** 円にまとめた合計。null のときは、切り替えを出さず、通貨ごとの表示にする（要件2）。 */
  combinedTotal: CombinedTotal | null
  /** 覚えた表示。実際に出す表示は、combinedTotal が null なら通貨ごと（要件17。effectiveTotalsView）。 */
  view: TotalsView
  /** 切り替えのボタンが押されたときに、押された表示で呼ばれる。 */
  onViewChange: (view: TotalsView) => void
}

/**
 * 保有株式全体の合計の欄（カード表示の要件12〜16、ドルを円でまとめる合計の要件1〜要件5）。
 * 見出し「合計」（h2）の行の右に切り替え（combinedTotal があるときだけ）を置き、その下に、
 * 通貨ごとの部分を渡された順（円、ドル）に置くか、円にまとめた部分を置く。
 */
export function HoldingsSummary({ totals, combinedTotal, view, onViewChange }: HoldingsSummaryProps) {
  const titleId = useId()
  const shownView = effectiveTotalsView(view, combinedTotal)
  return (
    <section aria-labelledby={titleId} className="holdings-summary">
      <div className="holdings-summary__header">
        <h2 id={titleId} className="holdings-summary__title">
          {SUMMARY_TITLE}
        </h2>
        {combinedTotal !== null && <TotalsViewSwitch view={shownView} onChange={onViewChange} />}
      </div>
      {combinedTotal !== null && shownView === 'combinedYen' ? (
        <CombinedSummary combinedTotal={combinedTotal} />
      ) : (
        <div className="holdings-summary__currencies">
          {totals.map((total) => (
            <CurrencySummary key={total.currency} total={total} />
          ))}
        </div>
      )}
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
