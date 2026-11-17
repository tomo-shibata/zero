import { describe, expect, test } from 'vitest'
import { HOLDINGS_PATH, stockHistoryPath } from '../../../src/shared/lib/routes'

// 仕様: 要件 docs/requirements/stock-history.md の要件1・要件4（カードを押すと /holdings/<銘柄コード> に移る。「← 保有株式一覧」は /holdings に移る）。
//   銘柄コードに「.」「-」が使える（売買履歴の要件24）。
// 境界: プラン docs/plans/stock-history/plan.md 4.8「PR② 画面とカードからの移動」の shared/lib/routes.ts
//   - HOLDINGS_PATH = '/holdings'
//   - stockHistoryPath(code): `/holdings/${encodeURIComponent(code)}`
// テスト計画: プラン 5.2 の routes.test.ts の行。

describe('shared/lib/routes（プラン 4.8）', () => {
  test('補助 要件4: HOLDINGS_PATH は /holdings', () => {
    expect(HOLDINGS_PATH).toBe('/holdings')
  })

  test('補助 要件1: stockHistoryPath("7203") は /holdings/7203', () => {
    expect(stockHistoryPath('7203')).toBe('/holdings/7203')
  })

  test('補助 売買履歴の要件24: 銘柄コードの「.」「-」はそのまま。stockHistoryPath("BRK.B") は /holdings/BRK.B、"BF-B" は /holdings/BF-B', () => {
    expect(stockHistoryPath('BRK.B')).toBe('/holdings/BRK.B')
    expect(stockHistoryPath('BF-B')).toBe('/holdings/BF-B')
  })

  test('補助 プラン 4.8: 「/」を含む銘柄コードは符号化される。stockHistoryPath("A/B") は /holdings/A%2FB', () => {
    expect(stockHistoryPath('A/B')).toBe('/holdings/A%2FB')
  })
})
