import { describe, expect, test } from 'vitest'
import { buildChartModel } from '../../../src/features/stock-history/lib/chartModel'
import {
  appleHistory,
  baseline7,
  close,
  sameDayHistory,
  split,
  splitHistory,
  toyotaHistory,
  trade,
} from './fixtures'

// 仕様: 要件 docs/requirements/stock-history.md の要件6〜要件9・要件12、基準7〜基準9・基準12〜基準14（単体）。
// 境界: プラン docs/plans/stock-history/plan.md 4.8「PR② 画面とカードからの移動」の features/stock-history/lib/chartModel.ts
//   buildChartModel(history): ChartModel
//   - xRange: from は最初の売買の取引日、to は最後の売買の取引日と最後の終値の日の遅いほう（分割の行の日は使わない）。
//     from と to が同じなら前後に1日ずつ広げる（from < to を保証。例: 2026-11-16 だけなら { from: '2026-11-15', to: '2026-11-17' }）。
//   - line: 取引日の昇順の線の点。value は price ÷ splitDivisor（丸めない）。close は元の終値。
//   - markers: 売買1件に1つ。entries と同じ順（新しい順。同じ日は表と同じ順）。value は price ÷ splitDivisor。trade は元の売買。
//   - yRange: 線と印がすべて入る範囲。min < max で、どちらも有限の値。
//   - hasNoPrices: 線の点が0件。
//   history.closes は、API が返す形（最初の売買の取引日と同じかそれより後のものだけ。プラン 4.2）で渡す。
// テスト計画: プラン 5.1 の基準7・8・9・12・13・14 の ② の行と、5.2 の chartModel.test.ts の行。

/** 印の「日付・売買・値」。 */
function markersOf(history: ReturnType<typeof baseline7>): { date: string; side: string; value: number }[] {
  return buildChartModel(history).markers.map((marker) => ({ date: marker.date, side: marker.side, value: marker.value }))
}

describe('線と横軸（基準7・基準8。要件6・要件7）', () => {
  test('基準7: 終値が 2026-01-10 2,450円・2026-03-02 3,000円・2026-10-06 3,000円 のとき、line は この3点をこの順で、xRange は 2026-01-10 から 2026-10-06 まで', () => {
    const model = buildChartModel(baseline7())

    expect(model.line.map((point) => [point.date, point.value])).toEqual([
      ['2026-01-10', 2450],
      ['2026-03-02', 3000],
      ['2026-10-06', 3000],
    ])
    expect(model.xRange).toEqual({ from: '2026-01-10', to: '2026-10-06' })
    expect(model.hasNoPrices).toBe(false)
  })

  test('基準7（補助）: 線の点は、元の終値（close）を持つ（吹き出しに使う）', () => {
    const history = baseline7()
    const model = buildChartModel(history)

    expect(model.line.map((point) => point.close)).toEqual(history.closes)
  })

  test('基準8: 売買が 2026-01-10 と 2026-11-16 で、終値が 2026-10-06 だけのとき、xRange は 2026-01-10 から 2026-11-16 まで（最後の売買の日のほうが遅い）', () => {
    const history = toyotaHistory(
      [trade('buy', '2026-11-16', 10, '2800.0'), trade('buy', '2026-01-10', 100, '2500.0')],
      [close('2026-10-06', '3000.0')],
    )

    expect(buildChartModel(history).xRange).toEqual({ from: '2026-01-10', to: '2026-11-16' })
  })

  test('補助 要件6: 最後の終値の日のほうが最後の売買の日より遅いとき、xRange の to は最後の終値の日。from は最初の終値の日ではなく最初の売買の日', () => {
    const history = toyotaHistory(
      [trade('buy', '2026-01-10', 100, '2500.0')],
      [close('2026-03-02', '3000.0'), close('2026-10-06', '3000.0')],
    )

    expect(buildChartModel(history).xRange).toEqual({ from: '2026-01-10', to: '2026-10-06' })
  })

  test('補助 4.8 の xRange の決まり: 「買 2,500円 100株 2026-01-10」「売 2,600円 50株 2026-02-10」と分割 1→3 2026-03-31（終値なし）のとき、xRange は 2026-01-10 から 2026-02-10 まで（分割の行の日 2026-03-31 は使わない）', () => {
    const history = toyotaHistory(
      [split('2026-03-31', 3), trade('sell', '2026-02-10', 50, '2600.0'), trade('buy', '2026-01-10', 100, '2500.0')],
      [],
    )

    expect(buildChartModel(history).xRange).toEqual({ from: '2026-01-10', to: '2026-02-10' })
  })
})

describe('売買の印（基準9。要件8）', () => {
  test('基準9: 2026-01-10 に買 2つ（2,500・2,520）、2026-03-02 に売 1つ（3,050）。markers の並びは entries と同じ（2026-03-02 の売、2026-01-10 の 10株の買 2,520、2026-01-10 の 100株の買 2,500）', () => {
    expect(markersOf(sameDayHistory())).toEqual([
      { date: '2026-03-02', side: 'sell', value: 3050 },
      { date: '2026-01-10', side: 'buy', value: 2520 },
      { date: '2026-01-10', side: 'buy', value: 2500 },
    ])
  })

  test('基準9（補助）: 印は、元の売買（trade）を持つ（吹き出しに使う）', () => {
    const history = sameDayHistory()
    const model = buildChartModel(history)

    expect(model.markers.map((marker) => marker.trade)).toEqual(history.entries)
  })

  test('補助 要件8: 分割の行は印にならない（印は売買1件に1つ）', () => {
    const history = splitHistory()

    expect(buildChartModel(history).markers).toHaveLength(2)
  })
})

describe('分割を反映した値（基準12・基準13。要件9）', () => {
  test('基準12: 分割 1→3 2026-03-31 のとき、線の点の値は 2026-03-30 が 3100÷3、2026-03-31 が 1050。印の値は 2026-03-31 が 1000（分割の日と同じ日なので割らない）、2026-01-10 が 2500÷3', () => {
    const history = splitHistory()
    const model = buildChartModel(history)

    expect(model.line.map((point) => point.date)).toEqual(['2026-03-30', '2026-03-31'])
    expect(model.line[0].value).toBeCloseTo(3100 / 3, 9)
    expect(model.line[1].value).toBe(1050)
    expect(model.markers.map((marker) => marker.date)).toEqual(['2026-03-31', '2026-01-10'])
    expect(model.markers[0].value).toBe(1000)
    expect(model.markers[1].value).toBeCloseTo(2500 / 3, 9)
  })

  test('基準12（要件9。丸めない）: 割った値は丸めない（2500÷3 は 833.3 ではなく 833.333…）', () => {
    const model = buildChartModel(splitHistory())

    expect(model.markers[1].value).not.toBe(833.3)
    expect(model.markers[1].value).toBeCloseTo(833.3333333333, 9)
  })

  test('基準13: AAPL の分割 1→2 2026-02-01・1→5 2026-04-01 で、2026-01-10 の 150.00ドルの印は 15（150÷(2×5)）、2026-03-01 の 40.00ドルの印は 8（40÷5）', () => {
    const history = appleHistory(
      [
        split('2026-04-01', 5),
        trade('buy', '2026-03-01', 10, '40.00', { currency: 'USD', splitDivisor: '5', adjustedPrice: '8.00' }),
        split('2026-02-01', 2),
        trade('buy', '2026-01-10', 10, '150.00', { currency: 'USD', splitDivisor: '10', adjustedPrice: '15.00' }),
      ],
      [],
    )

    expect(markersOf(history)).toEqual([
      { date: '2026-03-01', side: 'buy', value: 8 },
      { date: '2026-01-10', side: 'buy', value: 15 },
    ])
  })

  test('補助 4.4・4.8（splitDivisor は 2^53 を超えうる文字列）: 割る数が 1 の後ろに 0 が21個ある文字列でも、値は有限の数（NaN・Infinity にならない）で、yRange は min < max のどちらも有限', () => {
    const huge = `1${'0'.repeat(21)}`
    const history = toyotaHistory(
      [trade('buy', '2026-01-10', 100, '2500.0', { splitDivisor: huge, adjustedPrice: '0.0' })],
      [close('2026-01-10', '2450.0', { splitDivisor: huge, adjustedPrice: '0.0' })],
    )
    const model = buildChartModel(history)

    expect(Number.isFinite(model.line[0].value)).toBe(true)
    expect(Number.isFinite(model.markers[0].value)).toBe(true)
    expect(model.line[0].value).toBeCloseTo(2450 / 1e21, 30)
    expect(model.markers[0].value).toBeCloseTo(2500 / 1e21, 30)
    expect(Number.isFinite(model.yRange.min)).toBe(true)
    expect(Number.isFinite(model.yRange.max)).toBe(true)
    expect(model.yRange.min).toBeLessThan(model.yRange.max)
  })
})

describe('終値がないとき（基準14。要件12）', () => {
  test('基準14: 9984 の「買 6,000円 100株 2026-01-10」「売 7,000円 50株 2026-02-10」で終値が1件もないとき、line は空・hasNoPrices は true・yRange は 6,000 と 7,000 を含み、markers は 2026-02-10 の売 7000、2026-01-10 の買 6000 の順', () => {
    const history = {
      stock: { code: '9984', name: 'ソフトバンクグループ', currency: 'JPY' as const },
      entries: [trade('sell', '2026-02-10', 50, '7000.0'), trade('buy', '2026-01-10', 100, '6000.0')],
      closes: [],
    }
    const model = buildChartModel(history)

    expect(model.line).toEqual([])
    expect(model.hasNoPrices).toBe(true)
    expect(model.yRange.min).toBeLessThanOrEqual(6000)
    expect(model.yRange.max).toBeGreaterThanOrEqual(7000)
    expect(model.markers.map((marker) => [marker.date, marker.side, marker.value])).toEqual([
      ['2026-02-10', 'sell', 7000],
      ['2026-01-10', 'buy', 6000],
    ])
  })
})

describe('範囲が1点のとき（要件6・要件12 の補助。プラン 4.6・5.2）', () => {
  test('補助: 「買 2,500円 100株 2026-11-16」だけで終値なしのとき、xRange は { from: 2026-11-15, to: 2026-11-17 }、yRange は min < max でどちらも有限で 2,500 を含み、印は1つ', () => {
    const history = toyotaHistory([trade('buy', '2026-11-16', 100, '2500.0')], [])
    const model = buildChartModel(history)

    expect(model.xRange).toEqual({ from: '2026-11-15', to: '2026-11-17' })
    expect(Number.isFinite(model.yRange.min)).toBe(true)
    expect(Number.isFinite(model.yRange.max)).toBe(true)
    expect(model.yRange.min).toBeLessThan(model.yRange.max)
    expect(model.yRange.min).toBeLessThanOrEqual(2500)
    expect(model.yRange.max).toBeGreaterThanOrEqual(2500)
    expect(model.markers).toHaveLength(1)
    expect(model.hasNoPrices).toBe(true)
  })

  test('補助: 終値が1点（2026-11-16）で売買も同じ日の1件のとき、line は1点で、xRange は { from: 2026-11-15, to: 2026-11-17 }。値がすべて同じ（2,500）でも yRange は min < max', () => {
    const history = toyotaHistory([trade('buy', '2026-11-16', 100, '2500.0')], [close('2026-11-16', '2500.0')])
    const model = buildChartModel(history)

    expect(model.line).toHaveLength(1)
    expect(model.hasNoPrices).toBe(false)
    expect(model.xRange).toEqual({ from: '2026-11-15', to: '2026-11-17' })
    expect(model.yRange.min).toBeLessThan(model.yRange.max)
    expect(model.yRange.min).toBeLessThanOrEqual(2500)
    expect(model.yRange.max).toBeGreaterThanOrEqual(2500)
  })

  test('補助（日付の計算）: 広げた日付は月・年をまたぐ。2026-12-01 だけなら { 2026-11-30, 2026-12-02 }、2026-01-01 だけなら { 2025-12-31, 2026-01-02 }、2028-03-01 だけなら { 2028-02-29, 2028-03-02 }', () => {
    const rangeOf = (date: string) =>
      buildChartModel(toyotaHistory([trade('buy', date, 100, '2500.0')], [])).xRange

    expect(rangeOf('2026-12-01')).toEqual({ from: '2026-11-30', to: '2026-12-02' })
    expect(rangeOf('2026-01-01')).toEqual({ from: '2025-12-31', to: '2026-01-02' })
    expect(rangeOf('2028-03-01')).toEqual({ from: '2028-02-29', to: '2028-03-02' })
  })
})

describe('縦軸の範囲（要件6。「線と印がすべて入る範囲」）', () => {
  test('補助: 基準12 のデータ（割った値 833.33…・1,033.33…・1,050・1,000）の線と印がすべて yRange に入り、min < max', () => {
    const model = buildChartModel(splitHistory())
    const values = [...model.line.map((point) => point.value), ...model.markers.map((marker) => marker.value)]

    expect(model.yRange.min).toBeLessThan(model.yRange.max)
    for (const value of values) {
      expect(value).toBeGreaterThanOrEqual(model.yRange.min)
      expect(value).toBeLessThanOrEqual(model.yRange.max)
    }
  })

  test('補助: 基準7 のデータで、yRange は 売買の価格 3,050 と終値 2,450 を含む', () => {
    const model = buildChartModel(baseline7())

    expect(model.yRange.min).toBeLessThanOrEqual(2450)
    expect(model.yRange.max).toBeGreaterThanOrEqual(3050)
  })
})
