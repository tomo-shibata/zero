// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, test } from 'vitest'
import { HoldingsTotal } from '../../../src/features/holdings/components/HoldingsTotal'
import type { CurrencyTotal } from '../../../src/entities/holding/model'

// 仕様: 売買履歴の要件 docs/requirements/trade-history.md の要件12・要件13・要件14、基準12（合計評価額に円の行だけを表示し、ドルの行は表示しない）。
//   基準10・基準11（通貨ごとの並びと「価格を取得できない銘柄を含みません」の位置）は E2E（holdings.spec.ts）。ここは部品の単体。
// 境界: プラン docs/plans/trade-history/plan.md 4.6（PR②）と 4.7（PR②）
//   - HoldingsTotal の props は { totals: CurrencyTotal[] }。通貨ごとに1行（金額と、excludesUnpriced なら「価格を取得できない銘柄を含みません」）。
//   - 構造: <section aria-label="合計評価額"> の中に「合計評価額」の見出しを1回だけ出し、その下の <ul> に通貨ごとの <li> を円、ドルの順に入れる。
//     各 <li> は金額と、excludesUnpriced のときだけ「価格を取得できない銘柄を含みません」を持つ。
//   - 金額の整形は shared/lib/format.ts の formatValuation（"300000" → "300,000円"、"1800.00" → "1,800.00ドル"）。
//   - CurrencyTotal は { currency: 'JPY' | 'USD'; valuation: string; excludesUnpriced: boolean }（entities/holding/model.ts）。
//   - totals が空なら HoldingsTotal を描画しない（0件の画面は HoldingsPage.test.tsx）。
// テスト計画: プラン 5.1 の基準12（HoldingsTotal の単体。円の行だけ）と、5.5（ドルだけ: totals がドルだけなら円の行を出さない）。

const TOTAL_LABEL = '合計評価額'
const UNPRICED_NOTE = '価格を取得できない銘柄を含みません'

afterEach(() => {
  cleanup()
})

/** 合計の領域（<section aria-label="合計評価額">）。 */
function totalRegion(): HTMLElement {
  return screen.getByRole('region', { name: TOTAL_LABEL })
}

/** 合計の領域の中の、通貨ごとの行（<li>）。 */
function currencyRows(): HTMLElement[] {
  return within(totalRegion()).getAllByRole('listitem')
}

const YEN_300000: CurrencyTotal = { currency: 'JPY', valuation: '300000', excludesUnpriced: false }
const USD_1800: CurrencyTotal = { currency: 'USD', valuation: '1800.00', excludesUnpriced: false }

describe('HoldingsTotal（合計評価額。要件12・要件13）', () => {
  // 基準12: Aの保有が 7203 だけ（円だけ）→ 合計評価額は円の行だけを表示し、ドルの行は表示しない。
  test('基準12: totals が円だけなら、円の行（「300,000円」）が1行だけ表示され、ドルの行は表示されない', () => {
    render(<HoldingsTotal totals={[YEN_300000]} />)

    const rows = currencyRows()
    expect(rows).toHaveLength(1)
    expect(rows[0]).toHaveTextContent('300,000円')
    expect(totalRegion()).not.toHaveTextContent('ドル')
    expect(screen.queryByText(UNPRICED_NOTE)).not.toBeInTheDocument()
  })

  // 5.5: ドルだけ。totals がドルだけなら、ドルの行だけを表示し、円の行は表示しない。
  test('5.5: totals がドルだけなら、ドルの行（「1,800.00ドル」）が1行だけ表示され、円の行は表示されない', () => {
    render(<HoldingsTotal totals={[USD_1800]} />)

    const rows = currencyRows()
    expect(rows).toHaveLength(1)
    expect(rows[0]).toHaveTextContent('1,800.00ドル')
    expect(totalRegion()).not.toHaveTextContent('円')
  })

  // 基準10 の部品の部分: 円とドルの両方があれば、通貨ごとに1行ずつ、渡された順（円、ドル）に表示する。「価格を取得できない銘柄を含みません」はない。
  test('基準10: totals が円とドルなら、円の行、ドルの行の順に表示され、「価格を取得できない銘柄を含みません」は表示されない', () => {
    render(<HoldingsTotal totals={[YEN_300000, USD_1800]} />)

    const rows = currencyRows()
    expect(rows).toHaveLength(2)
    expect(rows[0]).toHaveTextContent('300,000円')
    expect(rows[1]).toHaveTextContent('1,800.00ドル')
    expect(screen.queryByText(UNPRICED_NOTE)).not.toBeInTheDocument()
  })

  // 基準11 の部品の部分: 円は注記なし、ドルは価格のない銘柄があるので「0.00ドル」と「価格を取得できない銘柄を含みません」。
  // 注記は、価格のない銘柄がある通貨の行の中にだけある（円の行にはない）。
  test('基準11: ドルだけ excludesUnpriced なら、ドルの行に「0.00ドル」と「価格を取得できない銘柄を含みません」があり、円の行には注記がない', () => {
    render(
      <HoldingsTotal
        totals={[YEN_300000, { currency: 'USD', valuation: '0.00', excludesUnpriced: true }]}
      />,
    )

    const [yenRow, usdRow] = currencyRows()
    expect(yenRow).toHaveTextContent('300,000円')
    expect(yenRow).not.toHaveTextContent(UNPRICED_NOTE)
    expect(usdRow).toHaveTextContent('0.00ドル')
    expect(usdRow).toHaveTextContent(UNPRICED_NOTE)
    expect(screen.getAllByText(UNPRICED_NOTE)).toHaveLength(1)
  })

  test('円だけ excludesUnpriced なら、円の行に「300,000円」と「価格を取得できない銘柄を含みません」があり、ドルの行には注記がない', () => {
    render(
      <HoldingsTotal
        totals={[{ currency: 'JPY', valuation: '300000', excludesUnpriced: true }, USD_1800]}
      />,
    )

    const [yenRow, usdRow] = currencyRows()
    expect(yenRow).toHaveTextContent('300,000円')
    expect(yenRow).toHaveTextContent(UNPRICED_NOTE)
    expect(usdRow).toHaveTextContent('1,800.00ドル')
    expect(usdRow).not.toHaveTextContent(UNPRICED_NOTE)
    expect(screen.getAllByText(UNPRICED_NOTE)).toHaveLength(1)
  })

  // 4.7: 「合計評価額」の見出しは、通貨が2つあっても領域の中に1回だけ。
  test('4.7: 「合計評価額」の文字は、円とドルの2行があっても合計の領域の中に1回だけ出る', () => {
    render(<HoldingsTotal totals={[YEN_300000, USD_1800]} />)

    expect(within(totalRegion()).getAllByText(TOTAL_LABEL)).toHaveLength(1)
  })

  test('4.7: 通貨ごとの行は <ul> の中の <li> で、<li> の数は totals の件数と同じ', () => {
    render(<HoldingsTotal totals={[YEN_300000, USD_1800]} />)

    const list = within(totalRegion()).getByRole('list')
    expect(within(list).getAllByRole('listitem')).toHaveLength(2)
  })

  // 要件14: 金額の整形（円は3桁区切り＋「円」、ドルは3桁区切り＋小数2桁＋「ドル」）。
  test('要件14: 大きな金額は3桁区切りで表示される（円「1,234,567円」・ドル「1,234,567.50ドル」）', () => {
    render(
      <HoldingsTotal
        totals={[
          { currency: 'JPY', valuation: '1234567', excludesUnpriced: false },
          { currency: 'USD', valuation: '1234567.50', excludesUnpriced: false },
        ]}
      />,
    )

    const [yenRow, usdRow] = currencyRows()
    expect(yenRow).toHaveTextContent('1,234,567円')
    expect(usdRow).toHaveTextContent('1,234,567.50ドル')
  })
})
