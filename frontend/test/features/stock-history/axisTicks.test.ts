import { describe, expect, test } from 'vitest'
import { formatAxisPrice, priceTicks } from '../../../src/features/stock-history/lib/axisTicks'

// 仕様: 要件 docs/requirements/stock-history.md の要件6（縦軸の目盛りの金額は表示の書式で書く。円は保有株式一覧の FR-12、ドルは売買履歴の要件14）、基準10（単体）。
// 境界: プラン docs/plans/stock-history/plan.md 4.8「PR② 画面とカードからの移動」の features/stock-history/lib/axisTicks.ts
//   priceTicks(currency, min, max, maxTicks = 6): number[]
//     maxTicks（任意。既定 6）: 目盛りの数の上限。2 以上。低いチャートで縦軸の文字が重ならないよう、呼び出し側が描く範囲の高さから決める
//     （ステップ6 2ラウンド目のデザイン2-3）。省いたときは従来どおり（ふつうは3〜6個）。maxTicks が小さくても、1個以上で maxTicks 個以下。
//     昇順の切りのよい値で、例外の1個を除き、どれも min 以上 max 以下。ふつうは3〜6個。差が狭く3個取れないときは、範囲の中の倍数を1個以上
//     （範囲の中に倍数がなければ min を丸めた1個を返す。この1個だけは min〜max の外になりうる）。min < max は buildChartModel が保証する。
//     間隔は円は 0.1 の倍数、ドルは 0.01 の倍数に限る（分割で割った小さい値でも、表示の書式で正しく書けるように）。
//   formatAxisPrice(currency, value): string  … 円は「3,000円」「1,033.3円」、ドルは「180.00ドル」。
// テスト計画: プラン 5.1 の基準10 の axisTicks.test.ts の行と、5.2 の axisTicks.test.ts の行。

/** 値が unit（円 0.1・ドル 0.01）の倍数か。「10倍（ドルは100倍）した値と、それを Math.round した値の差が 1e-9 未満」で確かめる（1.1 * 10 は 11 ちょうどにならないため）。 */
function isMultipleOf(value: number, factor: number): boolean {
  const scaled = value * factor
  return Math.abs(scaled - Math.round(scaled)) < 1e-9
}

describe('formatAxisPrice（基準10。要件6）', () => {
  test('基準10: ドルの 180 は「180.00ドル」', () => {
    expect(formatAxisPrice('USD', 180)).toBe('180.00ドル')
  })

  test('基準10: 円の 3000 は「3,000円」', () => {
    expect(formatAxisPrice('JPY', 3000)).toBe('3,000円')
  })

  test('要件6: 円の 1033.3 は「1,033.3円」（小数第1位が0でないので小数を出す）。833.3 は「833.3円」', () => {
    expect(formatAxisPrice('JPY', 1033.3)).toBe('1,033.3円')
    expect(formatAxisPrice('JPY', 833.3)).toBe('833.3円')
  })

  test('要件6: ドルは小数を常に2桁。15 は「15.00ドル」、1502.5 は「1,502.50ドル」、0.07 は「0.07ドル」', () => {
    expect(formatAxisPrice('USD', 15)).toBe('15.00ドル')
    expect(formatAxisPrice('USD', 1502.5)).toBe('1,502.50ドル')
    expect(formatAxisPrice('USD', 0.07)).toBe('0.07ドル')
  })

  test('補助 要件6: 切りのよい値を足し算で作ったときの浮動小数点の誤差（0.1 + 0.2）があっても、円は「0.3円」、ドルは 0.1 + 0.2 で「0.30ドル」と書く', () => {
    expect(formatAxisPrice('JPY', 0.1 + 0.2)).toBe('0.3円')
    expect(formatAxisPrice('USD', 0.1 + 0.2)).toBe('0.30ドル')
  })

  test('補助 要件6: 円の整数は小数を付けない（1000 は「1,000円」）', () => {
    expect(formatAxisPrice('JPY', 1000)).toBe('1,000円')
  })
})

describe('priceTicks（5.2）', () => {
  const ORDINARY: { name: string; currency: 'JPY' | 'USD'; min: number; max: number }[] = [
    { name: '円 2,400〜3,100', currency: 'JPY', min: 2400, max: 3100 },
    { name: '円 833.3〜1,050（分割で割った値を含む）', currency: 'JPY', min: 833.3333, max: 1050 },
    { name: 'ドル 8〜15', currency: 'USD', min: 8, max: 15 },
    { name: 'ドル 150.25〜180', currency: 'USD', min: 150.25, max: 180 },
  ]

  for (const { name, currency, min, max } of ORDINARY) {
    test(`補助 5.2: ${name} のとき、昇順で、どれも min 以上 max 以下で、3〜6個`, () => {
      const ticks = priceTicks(currency, min, max)

      expect(ticks.length).toBeGreaterThanOrEqual(3)
      expect(ticks.length).toBeLessThanOrEqual(6)
      for (let i = 0; i < ticks.length; i += 1) {
        expect(ticks[i]).toBeGreaterThanOrEqual(min)
        expect(ticks[i]).toBeLessThanOrEqual(max)
        if (i > 0) expect(ticks[i]).toBeGreaterThan(ticks[i - 1])
      }
    })
  }

  test('補助 5.2: 円の目盛りは 0.1 の倍数だけ。円 1.0〜1.2 の目盛りは、どれも 0.1 の倍数で 1個以上', () => {
    const ticks = priceTicks('JPY', 1.0, 1.2)

    expect(ticks.length).toBeGreaterThanOrEqual(1)
    for (const tick of ticks) {
      expect(isMultipleOf(tick, 10), `${tick} は 0.1 の倍数`).toBe(true)
    }
  })

  test('補助 5.2: 円は 0.1 の倍数だけ。ふつうの幅（2,400〜3,100）でも、分割で割った幅（833.3333〜1,050）でも', () => {
    for (const [min, max] of [
      [2400, 3100],
      [833.3333, 1050],
      [0.5, 5.5],
    ] as const) {
      for (const tick of priceTicks('JPY', min, max)) {
        expect(isMultipleOf(tick, 10), `${tick} は 0.1 の倍数（${min}〜${max}）`).toBe(true)
      }
    }
  })

  test('補助 5.2: ドルは 0.01 の倍数だけ（8〜15、150.25〜180、1.00〜1.20）', () => {
    for (const [min, max] of [
      [8, 15],
      [150.25, 180],
      [1.0, 1.2],
    ] as const) {
      for (const tick of priceTicks('USD', min, max)) {
        expect(isMultipleOf(tick, 100), `${tick} は 0.01 の倍数（${min}〜${max}）`).toBe(true)
      }
    }
  })

  // デザイン2-3（ステップ6 2ラウンド目）: 4つ目の任意の引数 maxTicks（既定 6）。低いチャートで縦軸の文字が重ならないよう、目盛りの数の上限を呼び出し側が下げる。
  for (const { name, currency, min, max } of [
    { name: '円 2,400〜3,100', currency: 'JPY' as const, min: 2400, max: 3100 },
    { name: 'ドル 150.25〜180', currency: 'USD' as const, min: 150.25, max: 180 },
  ]) {
    for (const maxTicks of [2, 3]) {
      test(`デザイン2-3: ${name} で maxTicks が ${maxTicks} のとき、目盛りは1個以上 ${maxTicks} 個以下で、どれも min 以上 max 以下`, () => {
        const ticks = priceTicks(currency, min, max, maxTicks)

        expect(ticks.length).toBeGreaterThanOrEqual(1)
        expect(ticks.length).toBeLessThanOrEqual(maxTicks)
        for (const tick of ticks) {
          expect(tick).toBeGreaterThanOrEqual(min)
          expect(tick).toBeLessThanOrEqual(max)
        }
      })
    }
  }

  test('補助 5.2: 差が狭い（円で 2,500.0〜2,500.1）ときも、範囲の中の倍数を1個以上返す', () => {
    const ticks = priceTicks('JPY', 2500.0, 2500.1)

    expect(ticks.length).toBeGreaterThanOrEqual(1)
    for (const tick of ticks) {
      expect(tick).toBeGreaterThanOrEqual(2500.0)
      expect(tick).toBeLessThanOrEqual(2500.1)
      expect(isMultipleOf(tick, 10)).toBe(true)
    }
  })

  test('補助 5.2: 範囲の中に倍数がない（円で 833.34〜833.36）ときは、min を丸めた1個 [833.3]', () => {
    expect(priceTicks('JPY', 833.34, 833.36)).toEqual([833.3])
  })

  test('補助 5.2（ドルの対応）: 範囲の中に 0.01 の倍数がない（ドルで 10.001〜10.004）ときは、min を丸めた1個（10.00）', () => {
    const ticks = priceTicks('USD', 10.001, 10.004)

    expect(ticks).toHaveLength(1)
    expect(ticks[0]).toBeCloseTo(10, 9)
  })
})
