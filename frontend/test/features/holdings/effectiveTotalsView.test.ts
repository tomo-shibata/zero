import { describe, expect, test } from 'vitest'
import type { CombinedTotal } from '../../../src/entities/holding/model'
import { effectiveTotalsView } from '../../../src/features/holdings/lib/effectiveTotalsView'

// 仕様: 要件 docs/requirements/fx-total.md の要件2・要件17、基準16。
// 境界: プラン docs/plans/fx-total/plan.md 4.7（PR②）
//   effectiveTotalsView(stored: TotalsView, combinedTotal: CombinedTotal | null): TotalsView
//     combinedTotal が null なら 'byCurrency'、そうでなければ stored。
// テスト計画: プラン 5.1 の基準16 の行（effectiveTotalsView.test.ts）。

/** 基準1 の円にまとめた合計（値は何でもよい。有無だけを見る）。 */
const COMBINED_TOTAL: CombinedTotal = {
  valuation: '570000',
  excludesUnpriced: false,
  unrealizedGain: '94625',
  unrealizedGainRate: '19.9',
  usdJpyRate: '150.00',
  usdJpyRateDate: '2026-11-14',
}

describe('effectiveTotalsView（実際に出す表示。要件17）', () => {
  // 基準16: 「円にまとめる」を覚えていても、切り替えを出さない状態（combinedTotal が null）のときは通貨ごと。
  test('基準16: 覚えた表示が combinedYen でも、combinedTotal が null なら byCurrency になる', () => {
    expect(effectiveTotalsView('combinedYen', null)).toBe('byCurrency')
  })

  // 基準16: 切り替えを出せる状態なら、覚えた表示のとおり。
  test('基準16: 覚えた表示が combinedYen で combinedTotal があれば combinedYen になる', () => {
    expect(effectiveTotalsView('combinedYen', COMBINED_TOTAL)).toBe('combinedYen')
  })

  test('基準16: 覚えた表示が byCurrency で combinedTotal があれば byCurrency になる', () => {
    expect(effectiveTotalsView('byCurrency', COMBINED_TOTAL)).toBe('byCurrency')
  })

  // 補助: 覚えた表示が byCurrency で combinedTotal が null のときも byCurrency（4通りのうちの残り1つ）。
  test('補助（要件17）: 覚えた表示が byCurrency で combinedTotal が null なら byCurrency になる', () => {
    expect(effectiveTotalsView('byCurrency', null)).toBe('byCurrency')
  })
})
