import type { ChartLinePoint } from './chartModel'
import { toDayNumber } from './dayNumber'

/**
 * 線の点のうち、日付 date にいちばん近いもの（要件10）。同じ近さなら古いほう。line が空なら null。
 * 日付は通算の日数にして比べる（"YYYYMMDD" の数の差では、月をまたぐと近さが合わない）。
 */
export function nearestClose(line: readonly ChartLinePoint[], date: string): ChartLinePoint | null {
  const target = toDayNumber(date)
  let best: ChartLinePoint | null = null
  let bestDistance = Infinity
  let bestDay = Infinity
  for (const point of line) {
    const day = toDayNumber(point.date)
    const distance = Math.abs(day - target)
    if (distance < bestDistance || (distance === bestDistance && day < bestDay)) {
      best = point
      bestDistance = distance
      bestDay = day
    }
  }
  return best
}
