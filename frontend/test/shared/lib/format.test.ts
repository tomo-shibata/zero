import { describe, expect, test } from 'vitest'
import {
  formatShares,
  formatUnitPrice,
  formatUsdJpyRate,
  formatValuation,
} from '../../../src/shared/lib/format'

// 仕様: 要件 docs/requirements/stock-holdings.md の FR-11〜13、AC-12。
//   売買履歴の要件 docs/requirements/trade-history.md の要件14・基準13（ドルの金額の表示）。
// 境界: プラン docs/plans/stock-holdings/plan.md 4.4「フロント（TypeScript）」の shared/lib/format.ts を、
//   売買履歴のプラン docs/plans/trade-history/plan.md 4.6（PR②）の次の形にする。
//     formatUnitPrice(currency: Currency, price: string)      // "2500.0" → "2,500円"、"150.25" → "150.25ドル"
//     formatValuation(currency: Currency, valuation: string)  // "300000" → "300,000円"、"1800.00" → "1,800.00ドル"
//   formatYen と今の formatUnitPrice（通貨なし）は、この2つに置き換える。形が通貨に合わない文字列は throw する。
//   Currency 型は frontend/src/shared/lib/currency.ts（4.7）。
//   単価の文字列の形は、円が小数1桁（^[0-9]+\.[0-9]$）、ドルが小数2桁（^[0-9]+\.[0-9]{2}$）。
//   評価額の文字列の形は、円が整数（^[0-9]+$）、ドルが小数2桁。
// テスト計画: プラン 5.1 の基準13、5.5 の「基準13」の行（formatValuation('USD', …) に "1800.00"・"150.25"・"1234567.50"・"0.01"・"0.00"、
//   formatUnitPrice('USD', …) に "0.00" を除く4件）。円の期待値は PR② の前から変えない。

describe('formatUnitPrice（円の単価の表示。FR-12）', () => {
  // AC-12: 金額 1234.5 と 2500.0 を単価として整形すると、それぞれ 1,234.5円、2,500円 になる。
  // API の円の単価は小数第1位を1桁付けた文字列（売買履歴のプラン 4.3）なので、文字列で渡す。
  test('AC-12: ("JPY", "1234.5") は 3桁区切りで小数第1位を残し「1,234.5円」になる', () => {
    expect(formatUnitPrice('JPY', '1234.5')).toBe('1,234.5円')
  })

  test('AC-12: ("JPY", "2500.0") は小数第1位が0なので整数で「2,500円」になる', () => {
    expect(formatUnitPrice('JPY', '2500.0')).toBe('2,500円')
  })

  // 補助（FR-12、PR4_EC11）: 境界。整数部が0、ちょうど4桁で小数第1位が0、
  // Number の精度（2^53）を超える20桁の整数部（文字列のまま区切りを入れ、丸めない）。
  test('補助 FR-12（PR4_EC11）: ("JPY", "0.5") は整数部が0のまま「0.5円」になる', () => {
    expect(formatUnitPrice('JPY', '0.5')).toBe('0.5円')
  })

  test('補助 FR-12（PR4_EC11）: ("JPY", "1000.0") は小数第1位が0なので整数で「1,000円」になる', () => {
    expect(formatUnitPrice('JPY', '1000.0')).toBe('1,000円')
  })

  test('補助 FR-12（PR4_EC11）: 20桁の整数部 ("JPY", "12345678901234567890.5") は丸めずに「12,345,678,901,234,567,890.5円」になる', () => {
    expect(formatUnitPrice('JPY', '12345678901234567890.5')).toBe(
      '12,345,678,901,234,567,890.5円',
    )
  })
})

describe('formatUnitPrice（ドルの単価の表示。要件14、基準13）', () => {
  // 基準13: ドルの金額 1800、150.25、1234567.5、0.01 は、整数部を3桁区切りにし、小数は常に2桁にして「ドル」を付ける。
  // 単価の文字列はドルが小数2桁（"1800.00"・"1234567.50"）。"0.00" は単価にならない（価格は0より大きい）ので渡さない。
  test.each([
    ['1800.00', '1,800.00ドル'],
    ['150.25', '150.25ドル'],
    ['1234567.50', '1,234,567.50ドル'],
    ['0.01', '0.01ドル'],
  ])('基準13: ("USD", %j) は「%s」になる', (price, expected) => {
    expect(formatUnitPrice('USD', price)).toBe(expected)
  })

  test('ドルの単価は小数第2位が0でも省かない（"180.00" は「180.00ドル」。円の「2,500円」のように整数にしない）', () => {
    expect(formatUnitPrice('USD', '180.00')).toBe('180.00ドル')
  })
})

describe('formatUnitPrice（形が通貨に合わない文字列は throw する）', () => {
  test.each([
    { currency: 'JPY', price: '2500.00', reason: '円に小数2桁' },
    { currency: 'JPY', price: '2500', reason: '円に小数なし' },
    { currency: 'JPY', price: '-1.0', reason: '円に負の数' },
    { currency: 'JPY', price: 'abc', reason: '円に数でない文字列' },
    { currency: 'USD', price: '2500.0', reason: 'ドルに小数1桁' },
    { currency: 'USD', price: '150', reason: 'ドルに小数なし' },
    { currency: 'USD', price: '150.255', reason: 'ドルに小数3桁' },
    { currency: 'USD', price: '-1.00', reason: 'ドルに負の数' },
    { currency: 'USD', price: '', reason: 'ドルに空の文字列' },
  ] as const)('($currency, "$price") は $reason なので throw する', ({ currency, price }) => {
    expect(() => formatUnitPrice(currency, price)).toThrow()
  })
})

describe('formatValuation（円の評価額・合計評価額の表示。FR-11）', () => {
  // 補助（FR-11、TC-21）: 0 のとき（すべての行に価格がないときの合計。FR-6）と、
  // 3桁区切りが2か所入る桁数のとき。API の円の評価額は整数の文字列（売買履歴のプラン 4.3）。
  test('補助 FR-11（TC-21）: ("JPY", "0") は「0円」になる', () => {
    expect(formatValuation('JPY', '0')).toBe('0円')
  })

  test('補助 FR-11（TC-21）: ("JPY", "300000") は「300,000円」になる', () => {
    expect(formatValuation('JPY', '300000')).toBe('300,000円')
  })

  test('補助 FR-11（TC-21）: ("JPY", "1234567") は「1,234,567円」になる', () => {
    expect(formatValuation('JPY', '1234567')).toBe('1,234,567円')
  })

  // 補助（FR-11、PR4_EC13）: 桁の大きい金額の境界。評価額は文字列のまま区切るので、Number の精度（2^53）を超えても丸めない。
  // 2^53 - 1 はちょうど安全な整数の上限、2^53 + 1 は Number では 2^53 に丸まってしまう値（9,007,199,254,740,992円 にならない）。
  test('補助 FR-11（PR4_EC13）: ("JPY", "9007199254740991") は「9,007,199,254,740,991円」になる', () => {
    expect(formatValuation('JPY', '9007199254740991')).toBe(
      '9,007,199,254,740,991円',
    )
  })

  test('補助 FR-11（PR4_EC13）: ("JPY", "9007199254740993") は Number の精度（2^53）を超えても丸めず「9,007,199,254,740,993円」になる', () => {
    expect(formatValuation('JPY', '9007199254740993')).toBe(
      '9,007,199,254,740,993円',
    )
  })

  // 補助（FR-11、PR4_EC3）: 形が円に合わない文字列（ありえない値）は、誤った表示を出さずに throw する。
  test.each([
    { valuation: '1234.5', reason: '小数を含む' },
    { valuation: '300000.00', reason: '小数2桁（ドルの形）' },
    { valuation: 'NaN', reason: '数でない' },
    { valuation: '', reason: '空' },
    { valuation: '-1', reason: '負の数' },
  ])('補助 FR-11（PR4_EC3）: ("JPY", "$valuation") は $reason ので throw する', ({ valuation }) => {
    expect(() => formatValuation('JPY', valuation)).toThrow()
  })
})

describe('formatValuation（ドルの評価額・合計評価額の表示。要件14、基準13）', () => {
  // 基準13: ドルの金額 1800、150.25、1234567.5、0.01、0 を表示用に整形すると、
  // それぞれ 1,800.00ドル、150.25ドル、1,234,567.50ドル、0.01ドル、0.00ドル になる。
  // API のドルの評価額は小数2桁の文字列（"1800.00"・"1234567.50"・"0.00"）。
  test.each([
    ['1800.00', '1,800.00ドル'],
    ['150.25', '150.25ドル'],
    ['1234567.50', '1,234,567.50ドル'],
    ['0.01', '0.01ドル'],
    ['0.00', '0.00ドル'],
  ])('基準13: ("USD", %j) は「%s」になる', (valuation, expected) => {
    expect(formatValuation('USD', valuation)).toBe(expected)
  })

  // 形がドルに合わない文字列（小数が2桁でない）は throw する。
  test.each([
    { valuation: '1800', reason: '小数なし（円の形）' },
    { valuation: '1800.0', reason: '小数1桁' },
    { valuation: '1800.000', reason: '小数3桁' },
    { valuation: 'abc', reason: '数でない' },
    { valuation: '', reason: '空' },
  ])('("USD", "$valuation") は $reason ので throw する', ({ valuation }) => {
    expect(() => formatValuation('USD', valuation)).toThrow()
  })
})

describe('formatShares（保有数量の表示。FR-13）', () => {
  // 補助（FR-13、TC-21）: 3桁区切りが入る桁数のとき。
  test('補助 FR-13（TC-21）: 1000 は「1,000株」になる', () => {
    expect(formatShares(1000)).toBe('1,000株')
  })

  // 補助（FR-13、PR4_EC3）: 安全な整数でない値（ありえない値）は、誤った表示を出さずに throw する。
  test.each([1234.5, Number.NaN])(
    '補助 FR-13（PR4_EC3）: 安全な整数でない %s を渡すと throw する',
    (quantity) => {
      expect(() => formatShares(quantity)).toThrow()
    },
  )

  // 補助（FR-13、PR4_EC13）: 安全な整数の上の境界。いちばん大きい安全な整数（2^53 - 1）は
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

// ドルを円にまとめる合計（要件 docs/requirements/fx-total.md の要件10・基準9、プラン docs/plans/fx-total/plan.md 4.7 の PR②・5.1・5.2）。
// 境界: formatUsdJpyRate(rate: string): string  // "1000.50" → "1,000.50"。整数部分を3桁区切りにし、小数2桁はそのまま。
describe('formatUsdJpyRate（ドル円のレートの表示。要件10）', () => {
  // 基準9: レート 150 円は「150.00」、1,000.5 円は「1,000.50」（API は小数2桁の文字列で渡す）。
  test.each([
    ['150.00', '150.00'],
    ['1000.50', '1,000.50'],
  ])('基準9: formatUsdJpyRate(%j) は %j になる', (rate, expected) => {
    expect(formatUsdJpyRate(rate)).toBe(expected)
  })

  // 補助（プラン 5.2）: 3桁区切りの境目。整数部が1桁、7桁。
  test.each([
    ['0.01', '0.01'],
    ['999.99', '999.99'],
    ['1234567.89', '1,234,567.89'],
  ])('補助（要件10）: formatUsdJpyRate(%j) は %j になる', (rate, expected) => {
    expect(formatUsdJpyRate(rate)).toBe(expected)
  })
})
