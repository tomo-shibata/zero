import { describe, expect, test } from 'vitest'
import type { ChartLinePoint } from '../../../src/features/stock-history/lib/chartModel'
import { nearestClose } from '../../../src/features/stock-history/lib/nearestClose'
import { close } from './fixtures'

// 仕様: 要件 docs/requirements/stock-history.md の要件10（線の上にポインターを乗せると、いちばん近い終値を出す）。
// 境界: プラン docs/plans/stock-history/plan.md 4.8「PR② 画面とカードからの移動」の features/stock-history/lib/nearestClose.ts
//   nearestClose(line, date): ChartLinePoint | null  … date にいちばん近い点。同じ近さなら古いほう。line が空なら null。
//   日付の計算は "YYYY-MM-DD" の文字列か Date.UTC で日数を出す（手元のタイムゾーンに左右されない。4.6）。
// テスト計画: プラン 5.2 の nearestClose.test.ts の行。

/** 線の点（value は使わない。close は日付に合わせた終値）。 */
function point(date: string, value = 1000): ChartLinePoint {
  return { date, value, close: close(date, `${value}.0`) }
}

describe('nearestClose（要件10）', () => {
  const line = [point('2026-01-10', 2450), point('2026-03-02', 3000), point('2026-10-06', 3000)]

  test('補助 要件10: いちばん近い点を返す。2026-01-20 は 2026-01-10（10日）、2026-02-20 は 2026-03-02（10日。2026-01-10 は41日）', () => {
    expect(nearestClose(line, '2026-01-20')).toBe(line[0])
    expect(nearestClose(line, '2026-02-20')).toBe(line[1])
    expect(nearestClose(line, '2026-09-01')).toBe(line[2])
  })

  test('補助 要件10: 点の日付そのものなら、その点を返す', () => {
    expect(nearestClose(line, '2026-03-02')).toBe(line[1])
  })

  test('補助 要件10: 最初の点より前の日付は最初の点、最後の点より後の日付は最後の点', () => {
    expect(nearestClose(line, '2025-12-01')).toBe(line[0])
    expect(nearestClose(line, '2027-01-01')).toBe(line[2])
  })

  test('補助 要件10: 同じ近さなら古いほう。2026-03-01 と 2026-03-03 の点で 2026-03-02 を渡すと 2026-03-01', () => {
    const tie = [point('2026-03-01'), point('2026-03-03')]

    expect(nearestClose(tie, '2026-03-02')).toBe(tie[0])
  })

  test('補助 要件10: 同じ近さなら古いほう（月をまたぐ）。2026-01-31 と 2026-02-02 の点で 2026-02-01 を渡すと 2026-01-31', () => {
    const tie = [point('2026-01-31'), point('2026-02-02')]

    expect(nearestClose(tie, '2026-02-01')).toBe(tie[0])
  })

  test('補助 要件10（日数で比べる）: 2026-01-20 と 2026-02-10 の点で 2026-01-31 を渡すと、2026-02-10（10日）。2026-01-20 は11日。数字（YYYYMMDD）の差で比べると誤る', () => {
    const points = [point('2026-01-20'), point('2026-02-10')]

    expect(nearestClose(points, '2026-01-31')).toBe(points[1])
  })

  test('補助 要件10: 点が1つだけなら、どの日付でもその点', () => {
    const single = [point('2026-10-06')]

    expect(nearestClose(single, '2020-01-01')).toBe(single[0])
    expect(nearestClose(single, '2030-01-01')).toBe(single[0])
  })

  test('補助 要件10: 線が空なら null', () => {
    expect(nearestClose([], '2026-01-10')).toBeNull()
  })
})
