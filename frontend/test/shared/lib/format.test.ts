import { describe, expect, test } from 'vitest'
import {
  formatShares,
  formatUnitPrice,
  formatYen,
} from '../../../src/shared/lib/format'

// 仕様: 要件 docs/requirements/stock-holdings.md の FR-11〜13、AC-12。
// 境界: プラン docs/plans/stock-holdings/plan.md 4.4「フロント（TypeScript）」の shared/lib/format.ts。
// テスト計画: プラン 5.2 の AC-12、補助（FR-11・13、TC-21）、補助（4.4 の書式、PR④ の EC-3・EC-11）の行。

describe('formatUnitPrice（単価の表示。FR-12）', () => {
  // AC-12: 金額 1234.5 と 2500.0 を単価として整形すると、それぞれ 1,234.5円、2,500円 になる。
  // API の単価は小数第1位を1桁付けた文字列（プラン 4.4「API」）なので、文字列で渡す。
  test('AC-12: "1234.5" は 3桁区切りで小数第1位を残し「1,234.5円」になる', () => {
    expect(formatUnitPrice('1234.5')).toBe('1,234.5円')
  })

  test('AC-12: "2500.0" は小数第1位が0なので整数で「2,500円」になる', () => {
    expect(formatUnitPrice('2500.0')).toBe('2,500円')
  })

  // 補助（FR-12、PR④ の EC-11）: 境界。整数部が0、ちょうど4桁で小数第1位が0、
  // Number の精度（2^53）を超える20桁の整数部（文字列のまま区切りを入れ、丸めない）。
  test('補助 FR-12（PR4_EC11）: "0.5" は整数部が0のまま「0.5円」になる', () => {
    expect(formatUnitPrice('0.5')).toBe('0.5円')
  })

  test('補助 FR-12（PR4_EC11）: "1000.0" は小数第1位が0なので整数で「1,000円」になる', () => {
    expect(formatUnitPrice('1000.0')).toBe('1,000円')
  })

  test('補助 FR-12（PR4_EC11）: 20桁の整数部 "12345678901234567890.5" は丸めずに「12,345,678,901,234,567,890.5円」になる', () => {
    expect(formatUnitPrice('12345678901234567890.5')).toBe(
      '12,345,678,901,234,567,890.5円',
    )
  })
})

describe('formatYen（評価額・合計評価額の表示。FR-11）', () => {
  // 補助（FR-11、TC-21）: 0 のとき（すべての行に価格がないときの合計。FR-6）と、
  // 3桁区切りが2か所入る桁数のとき。
  test('補助 FR-11（TC-21）: 0 は「0円」になる', () => {
    expect(formatYen(0)).toBe('0円')
  })

  test('補助 FR-11（TC-21）: 1234567 は「1,234,567円」になる', () => {
    expect(formatYen(1234567)).toBe('1,234,567円')
  })

  // 補助（FR-11、PR④ の EC-3）: 安全な整数でない値（ありえない値）は、誤った表示を出さずに throw する。
  test.each([1234.5, Number.NaN])(
    '補助 FR-11（PR4_EC3）: 安全な整数でない %s を渡すと throw する',
    (amount) => {
      expect(() => formatYen(amount)).toThrow()
    },
  )

  // 補助（FR-11、PR④ の EC-13）: 安全な整数の上の境界。いちばん大きい安全な整数（2^53 - 1）は
  // 丸めずに区切りを入れて表示し、それより1大きい値（2^53。整数だが安全な整数でない）は throw する。
  test('補助 FR-11（PR4_EC13）: Number.MAX_SAFE_INTEGER は「9,007,199,254,740,991円」になる', () => {
    expect(formatYen(Number.MAX_SAFE_INTEGER)).toBe(
      '9,007,199,254,740,991円',
    )
  })

  test('補助 FR-11（PR4_EC13）: Number.MAX_SAFE_INTEGER + 1 を渡すと throw する', () => {
    expect(() => formatYen(Number.MAX_SAFE_INTEGER + 1)).toThrow()
  })
})

describe('formatShares（保有数量の表示。FR-13）', () => {
  // 補助（FR-13、TC-21）: 3桁区切りが入る桁数のとき。
  test('補助 FR-13（TC-21）: 1000 は「1,000株」になる', () => {
    expect(formatShares(1000)).toBe('1,000株')
  })

  // 補助（FR-13、PR④ の EC-3）: 安全な整数でない値（ありえない値）は、誤った表示を出さずに throw する。
  test.each([1234.5, Number.NaN])(
    '補助 FR-13（PR4_EC3）: 安全な整数でない %s を渡すと throw する',
    (quantity) => {
      expect(() => formatShares(quantity)).toThrow()
    },
  )

  // 補助（FR-13、PR④ の EC-13）: 安全な整数の上の境界。いちばん大きい安全な整数（2^53 - 1）は
  // 丸めずに区切りを入れて表示し、それより1大きい値（2^53。整数だが安全な整数でない）は throw する。
  test('補助 FR-13（PR4_EC13）: Number.MAX_SAFE_INTEGER は「9,007,199,254,740,991株」になる', () => {
    expect(formatShares(Number.MAX_SAFE_INTEGER)).toBe(
      '9,007,199,254,740,991株',
    )
  })

  test('補助 FR-13（PR4_EC13）: Number.MAX_SAFE_INTEGER + 1 を渡すと throw する', () => {
    expect(() => formatShares(Number.MAX_SAFE_INTEGER + 1)).toThrow()
  })
})
