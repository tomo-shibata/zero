import { describe, expect, test } from 'vitest'
import { conditionLabel, conditionText, formatThresholdPercent } from '../../../src/entities/price-alert/conditionText'
import type { AlertConditionSummary } from '../../../src/entities/price-alert/model'

// 仕様: 価格の変動の通知の要件 docs/requirements/price-alerts.md の要件31（「条件: <対象>・<条件の文>」の対象と条件の文）・要件41（条件の文）と、
//   成功基準の基準35（通知のリストの「条件: …」の文字）。
// 境界: プラン docs/plans/price-alerts/plan.md 4.8 の PR②「entities/price-alert/conditionText.ts」
//   - formatThresholdPercent(threshold: string): string  "20.0" → "20%"、"2.5" → "2.5%"（小数第1位が0なら整数）
//   - conditionText(c: AlertConditionSummary, currency: Currency | null): string  要件41 の文。currency は金額と価格の種類だけで使う
//   - conditionLabel(c: AlertConditionSummary, currency: Currency | null): string  「全銘柄・<文>」か「<銘柄コード>・<文>」（要件31）
//   threshold の形は API の形（プラン 4.5）: % は小数1桁、円の金額は整数、ドルの金額は小数2桁、円の価格は小数1桁、ドルの価格は小数2桁。
//   円の価格は保有株式一覧の FR-12（小数第1位が0なら整数。3桁区切り）、ドルは売買履歴の要件14（小数第2位まで。3桁区切り）、円の金額は整数を3桁区切り。
// テスト計画: プラン 5.1 の基準35（単体）の行。

function condition(overrides: Partial<AlertConditionSummary>): AlertConditionSummary {
  return { kind: 'cost', scope: 'all', stockCode: null, direction: 'down', threshold: '20.0', ...overrides }
}

describe('formatThresholdPercent（要件41。% の値は小数第1位が0なら整数）', () => {
  test.each([
    { threshold: '20.0', expected: '20%' },
    { threshold: '2.5', expected: '2.5%' },
    { threshold: '0.1', expected: '0.1%' },
    { threshold: '99.9', expected: '99.9%' },
    { threshold: '5.0', expected: '5%' },
  ])('"$threshold" は「$expected」になる', ({ threshold, expected }) => {
    expect(formatThresholdPercent(threshold)).toBe(expected)
  })
})

describe('conditionText（要件41 の文）', () => {
  test.each([
    { name: '取得価格から・上がった', c: condition({ kind: 'cost', direction: 'up', threshold: '20.0' }), currency: null, expected: '取得価格から 20% 以上 上がったら' },
    { name: '取得価格から・下がった', c: condition({ kind: 'cost', direction: 'down', threshold: '20.0' }), currency: null, expected: '取得価格から 20% 以上 下がったら' },
    { name: '取得価格から・小数の値', c: condition({ kind: 'cost', direction: 'down', threshold: '2.5' }), currency: null, expected: '取得価格から 2.5% 以上 下がったら' },
    { name: '最高値から', c: condition({ kind: 'peak', direction: 'down', threshold: '15.0' }), currency: null, expected: '最高値から 15% 以上 下がったら' },
    { name: '前の取引日から・上がった', c: condition({ kind: 'prev', direction: 'up', threshold: '5.0' }), currency: null, expected: '前の取引日から 5% 以上 上がったら' },
    { name: '前の取引日から・下がった', c: condition({ kind: 'prev', direction: 'down', threshold: '5.0' }), currency: null, expected: '前の取引日から 5% 以上 下がったら' },
    {
      name: '前の取引日から・動いた',
      c: condition({ kind: 'prev', direction: 'either', threshold: '5.0' }),
      currency: null,
      expected: '前の取引日から 5% 以上 動いたら（上がっても下がっても）',
    },
    {
      name: '前の取引日から・動いた・小数の値（基準35 の 2.5%）',
      c: condition({ kind: 'prev', direction: 'either', threshold: '2.5' }),
      currency: null,
      expected: '前の取引日から 2.5% 以上 動いたら（上がっても下がっても）',
    },
    {
      name: '含み損益の金額・含み損・円（整数を3桁区切り）',
      c: condition({ kind: 'pnl', scope: 'stock', stockCode: '6758', direction: 'loss', threshold: '100000' }),
      currency: 'JPY' as const,
      expected: '含み損が 100,000円 以上になったら',
    },
    {
      name: '含み損益の金額・含み益・ドル（小数2桁）',
      c: condition({ kind: 'pnl', scope: 'stock', stockCode: 'AAPL', direction: 'gain', threshold: '300.00' }),
      currency: 'USD' as const,
      expected: '含み益が 300.00ドル 以上になったら',
    },
    {
      name: '含み損益の金額・含み益・ドル（3桁区切り）',
      c: condition({ kind: 'pnl', scope: 'stock', stockCode: 'AAPL', direction: 'gain', threshold: '1234.50' }),
      currency: 'USD' as const,
      expected: '含み益が 1,234.50ドル 以上になったら',
    },
    {
      name: '決めた価格・以上・円（小数第1位が0なら整数）',
      c: condition({ kind: 'price', scope: 'stock', stockCode: '7203', direction: 'above', threshold: '3500.0' }),
      currency: 'JPY' as const,
      expected: '3,500円 以上になったら',
    },
    {
      name: '決めた価格・以上・円（小数第1位を出す）',
      c: condition({ kind: 'price', scope: 'stock', stockCode: '7203', direction: 'above', threshold: '3000.5' }),
      currency: 'JPY' as const,
      expected: '3,000.5円 以上になったら',
    },
    {
      name: '決めた価格・以下・ドル',
      c: condition({ kind: 'price', scope: 'stock', stockCode: 'AAPL', direction: 'below', threshold: '150.25' }),
      currency: 'USD' as const,
      expected: '150.25ドル 以下になったら',
    },
    {
      name: '決めた価格・以上・円（基準35 の 9984）',
      c: condition({ kind: 'price', scope: 'stock', stockCode: '9984', direction: 'above', threshold: '8000.0' }),
      currency: 'JPY' as const,
      expected: '8,000円 以上になったら',
    },
  ])('$name は「$expected」になる', ({ c, currency, expected }) => {
    expect(conditionText(c, currency)).toBe(expected)
  })
})

describe('conditionLabel（要件31 の「<対象>・<条件の文>」。対象は「全銘柄」か銘柄コード）', () => {
  test('基準35: 全銘柄の条件は「全銘柄・取得価格から 20% 以上 下がったら」', () => {
    expect(conditionLabel(condition({ kind: 'cost', direction: 'down', threshold: '20.0' }), null)).toBe('全銘柄・取得価格から 20% 以上 下がったら')
  })

  test('基準35: 全銘柄・前の取引日から 5% 以上 動いたら は「全銘柄・前の取引日から 5% 以上 動いたら（上がっても下がっても）」', () => {
    expect(conditionLabel(condition({ kind: 'prev', direction: 'either', threshold: '5.0' }), null)).toBe(
      '全銘柄・前の取引日から 5% 以上 動いたら（上がっても下がっても）',
    )
  })

  test('基準35: 全銘柄・最高値から 15% は「全銘柄・最高値から 15% 以上 下がったら」', () => {
    expect(conditionLabel(condition({ kind: 'peak', direction: 'down', threshold: '15.0' }), null)).toBe('全銘柄・最高値から 15% 以上 下がったら')
  })

  test('基準35: 1銘柄の条件は、銘柄コードで「9984・8,000円 以上になったら」', () => {
    expect(
      conditionLabel(condition({ kind: 'price', scope: 'stock', stockCode: '9984', direction: 'above', threshold: '8000.0' }), 'JPY'),
    ).toBe('9984・8,000円 以上になったら')
  })

  test('基準35: 6758・含み損が 100,000円 以上になったら', () => {
    expect(
      conditionLabel(condition({ kind: 'pnl', scope: 'stock', stockCode: '6758', direction: 'loss', threshold: '100000' }), 'JPY'),
    ).toBe('6758・含み損が 100,000円 以上になったら')
  })
})
