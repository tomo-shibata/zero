import type { CombinedTotal } from '../../../entities/holding/model'
import { formatUsdJpyRate, formatValuation } from '../../../shared/lib/format'
import { GAIN_LABEL, UNPRICED_NOTE, VALUATION_LABEL } from './summaryLabels'
import { UnrealizedGainText } from './UnrealizedGainText'

type CombinedSummaryProps = {
  combinedTotal: CombinedTotal
}

/**
 * ドルを円にまとめた合計の部分（要件4・要件9〜要件11）。「円」「ドル」の見出しは出さず、
 * 含み損益の合計とその率（いちばん大きな文字）、評価額の合計、価格のない銘柄の注記（あるときだけ）、使ったレートの行の順に出す。
 * 含み損益が null（価格のある銘柄がない）なら、「取得できません」（率なし、色なし。要件12）。書式と色は円の通貨ごとの表示と同じ。
 */
export function CombinedSummary({ combinedTotal }: CombinedSummaryProps) {
  const rate = formatUsdJpyRate(combinedTotal.usdJpyRate)
  return (
    <div className="holdings-summary__combined">
      <dl className="holdings-summary__details">
        <dt className="holdings-summary__term">{GAIN_LABEL}</dt>
        <dd className="holdings-summary__gain">
          <UnrealizedGainText
            currency="JPY"
            gain={combinedTotal.unrealizedGain}
            rate={combinedTotal.unrealizedGainRate}
          />
        </dd>
        <dt className="holdings-summary__term">{VALUATION_LABEL}</dt>
        <dd className="holdings-summary__valuation">{formatValuation('JPY', combinedTotal.valuation)}</dd>
      </dl>
      {combinedTotal.excludesUnpriced && <p className="holdings-summary__note">{UNPRICED_NOTE}</p>}
      <p className="holdings-summary__rate">{`1ドル = ${rate}円（${combinedTotal.usdJpyRateDate} のレート）で換算`}</p>
    </div>
  )
}
