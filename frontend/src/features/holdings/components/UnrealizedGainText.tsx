import type { Currency } from '../../../shared/lib/currency'
import { formatUnrealizedGain, unrealizedGainTone } from '../../../shared/lib/format'
import './UnrealizedGainText.css'

/** 含み損益が取得できないとき（価格のない銘柄、価格のある銘柄がない通貨）の文言（要件5・要件16）。 */
const UNAVAILABLE = '取得できません'

type UnrealizedGainTextProps = {
  currency: Currency
  /** 表示する含み損益。取得できないときは null。 */
  gain: string | null
  /** 含み損益の率。取得できないときは null（gain と同時に null）。 */
  rate: string | null
}

/**
 * 含み損益と率の文字（要件3・要件4・要件5）。「+75,000円（+20.0%）」のように1つの要素に出し、
 * data-tone（gain・loss・even）で色を付ける（プラスは赤、マイナスは青、0 と取得できないときは赤・青なし）。
 * 取得できないときは「取得できません」だけを出し、率は出さない。
 * 文字の大きさは CSS の変数 --unrealized-gain-size で、置く場所（カード・合計の欄）が決める。
 */
export function UnrealizedGainText({ currency, gain, rate }: UnrealizedGainTextProps) {
  if (gain === null || rate === null) {
    return (
      <p className="unrealized-gain" data-tone="even">
        {UNAVAILABLE}
      </p>
    )
  }
  const { amount, rateText } = splitAmountAndRate(formatUnrealizedGain(currency, gain, rate))
  return (
    <p className="unrealized-gain" data-tone={unrealizedGainTone(gain)}>
      {amount}
      <wbr />
      {rateText}
    </p>
  )
}

/**
 * 「+75,000円（+20.0%）」を、金額「+75,000円」と率「（+20.0%）」に分ける（最初の「（」の前と後ろ）。
 * 狭い幅で折り返すとき、改行を金額と率の間（<wbr />）だけにして、「ドル」の途中などで割れないようにするため（CSS の word-break: keep-all と合わせる）。
 * 金額と率は、1つの要素の直下の文字として並べる（2つを続けた文字列は元の文字列と同じ。読み上げ・textContent・文字での検索は変わらない）。
 */
function splitAmountAndRate(text: string): { amount: string; rateText: string } {
  const index = text.indexOf('（')
  if (index < 0) return { amount: text, rateText: '' }
  return { amount: text.slice(0, index), rateText: text.slice(index) }
}
