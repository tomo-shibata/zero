import type { Currency } from '../../../shared/lib/currency'
import { formatUnitPrice } from '../../../shared/lib/format'

// 縦軸（価格）の目盛り（銘柄ごとの売買履歴の要件6）。

/** 目盛りの間隔の最小の単位（円は 0.1、ドルは 0.01）の逆数。分割で割った小さい値でも、表示の書式（円は小数1桁、ドルは小数2桁）で正しく書けるようにする。 */
const UNITS_PER_ONE: Record<Currency, number> = { JPY: 10, USD: 100 }
/** 目盛りの個数の上限の既定（ふつうは3〜6個）。 */
const DEFAULT_MAX_TICKS = 6
/** 間隔の候補（最小の単位の何倍か）の、10のべき乗に掛ける数。2.5 は 10 倍以上のときだけ整数になる。 */
const STEP_MULTIPLIERS = [1, 2, 2.5, 5]
/** 間隔の候補の、10のべき乗の上限（これを超える範囲は価格として現れない）。 */
const MAX_STEP_EXPONENT = 15

/**
 * 縦軸の目盛りの値を、昇順で返す。切りのよい間隔（1・2・2.5・5 × 10のべき乗。円は 0.1、ドルは 0.01 の倍数）で、
 * maxTicks（既定 6。低いチャートで縦軸の文字が重ならないよう、呼び出し側が描く範囲の高さから決める）個以下になるいちばん細かい間隔を選ぶ。
 * どれも min 以上 max 以下。ふつうは3〜6個。
 * 範囲が狭くて間隔の倍数が少ないときも、範囲の中の倍数を1個以上返す。範囲の中に倍数が1つもなければ、min を丸めた1個を返す
 * （この1個だけは min〜max の外になりうる）。min < max は buildChartModel が保証する。
 */
export function priceTicks(currency: Currency, min: number, max: number, maxTicks = DEFAULT_MAX_TICKS): number[] {
  const scale = UNITS_PER_ONE[currency]
  const minUnits = min * scale
  const maxUnits = max * scale
  const rangeUnits = maxUnits - minUnits
  for (let exponent = 0; exponent <= MAX_STEP_EXPONENT; exponent += 1) {
    for (const multiplier of STEP_MULTIPLIERS) {
      const step = multiplier * 10 ** exponent
      if (!Number.isInteger(step)) continue
      // 間隔が細かすぎて、範囲の中に maxTicks + 1 個以上入る（数えるまでもなく多すぎる）なら、次の間隔へ。
      if (rangeUnits / step >= maxTicks + 1) continue
      const ticks = ticksInRange(min, max, scale, step)
      if (ticks.length > maxTicks) continue
      if (ticks.length > 0) return ticks
      // 最小の間隔でも範囲の中に倍数がない。
      if (step === 1) return [Math.round(minUnits) / scale]
    }
  }
  return [Math.round(minUnits) / scale]
}

/** 最小の単位の step 倍の倍数のうち、min 以上 max 以下のものを、昇順で返す。値は整数を scale で割って作る（足し算の誤差を溜めない）。 */
function ticksInRange(min: number, max: number, scale: number, step: number): number[] {
  const first = Math.floor((min * scale) / step) - 1
  const last = Math.ceil((max * scale) / step) + 1
  const ticks: number[] = []
  for (let index = first; index <= last; index += 1) {
    const tick = (index * step) / scale
    if (tick >= min && tick <= max) ticks.push(tick)
  }
  return ticks
}

/** 目盛りの金額を表示の書式で書く。円は「3,000円」「1,033.3円」、ドルは「180.00ドル」。値は円は小数第1位、ドルは小数第2位に丸めてから書く。 */
export function formatAxisPrice(currency: Currency, value: number): string {
  const digits = currency === 'JPY' ? 1 : 2
  return formatUnitPrice(currency, value.toFixed(digits))
}
