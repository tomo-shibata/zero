// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, test } from 'vitest'
import { TradeHistoryTable } from '../../../src/features/stock-history/components/TradeHistoryTable'
import type { StockHistoryEntry } from '../../../src/entities/stock-history/model'
import type { Currency } from '../../../src/shared/lib/currency'
import { split, trade } from './fixtures'

// 仕様: 要件 docs/requirements/stock-history.md の要件14〜要件18、基準17〜基準20（単体の部分。基準16・基準20 の画面全体は E2E）。
// 境界: プラン docs/plans/stock-history/plan.md 4.8「PR② 画面とカードからの移動」の features/stock-history/components/TradeHistoryTable.tsx
//   TradeHistoryTable({ currency, entries })。1行が1つの <tr>（見出しの行を除く）。列は「取引日・売買・株数・価格・金額」。
//   分割の行は、売買の欄に「分割」、株数の欄に「1株→3株」、価格と金額の欄は空の <td>（colSpan でまとめない）。
//   受け取った entries の順に行を出す（並べ替えは API。プラン 4.3）。価格と金額は実際の値（割らない。要件15）で、書式は formatUnitPrice。
// テスト計画: プラン 5.1 の基準17・基準18・基準19・基準20 の ② の行。

afterEach(() => {
  cleanup()
})

function renderTable(currency: Currency, entries: StockHistoryEntry[]) {
  return render(<TradeHistoryTable currency={currency} entries={entries} />)
}

/** 見出しの行を除いた行（見出しのセル（columnheader）を持たない行）。 */
function dataRows(): HTMLElement[] {
  return screen.getAllByRole('row').filter((row) => within(row).queryAllByRole('columnheader').length === 0)
}

/** 行のセル（取引日・売買・株数・価格・金額）の文字。td でも th でも、行の直下の要素を順に読む。 */
function cellsOf(row: HTMLElement): string[] {
  return Array.from(row.children).map((cell) => cell.textContent ?? '')
}

function rowTexts(): string[][] {
  return dataRows().map(cellsOf)
}

describe('列と行の並び（基準17。要件14・要件18）', () => {
  test('基準16・要件14: 見出しの行は「取引日 / 売買 / 株数 / 価格 / 金額」', () => {
    renderTable('JPY', [trade('buy', '2026-01-10', 100, '2500.0')])

    expect(screen.getAllByRole('columnheader').map((header) => header.textContent)).toEqual([
      '取引日',
      '売買',
      '株数',
      '価格',
      '金額',
    ])
  })

  test('基準17: 2026-03-02 の売、2026-01-10 の 10株の買、2026-01-10 の 100株の買の順に渡すと、行もこの順', () => {
    renderTable('JPY', [
      trade('sell', '2026-03-02', 50, '3050.0'),
      trade('buy', '2026-01-10', 10, '2520.0'),
      trade('buy', '2026-01-10', 100, '2500.0'),
    ])

    expect(rowTexts()).toEqual([
      ['2026-03-02', '売', '50株', '3,050円', '152,500円'],
      ['2026-01-10', '買', '10株', '2,520円', '25,200円'],
      ['2026-01-10', '買', '100株', '2,500円', '250,000円'],
    ])
  })

  test('基準17（受け取った順に出す）: 古い順に渡せば古い順に出す（表の中で並べ替えない）', () => {
    renderTable('JPY', [trade('buy', '2026-01-10', 100, '2500.0'), trade('sell', '2026-03-02', 50, '3050.0')])

    expect(rowTexts().map((cells) => cells[0])).toEqual(['2026-01-10', '2026-03-02'])
  })

  test('基準17（補助）: 1行が1つの <tr>。売買3件なら見出しの行を除いて3行', () => {
    renderTable('JPY', [
      trade('sell', '2026-03-02', 50, '3050.0'),
      trade('buy', '2026-01-10', 10, '2520.0'),
      trade('buy', '2026-01-10', 100, '2500.0'),
    ])

    expect(dataRows()).toHaveLength(3)
  })
})

describe('価格と金額の書式（基準18・基準19。要件15・要件16）', () => {
  test('基準18: 円の「買 1,234.5円 3株」は価格「1,234.5円」・金額「3,703.5円」、「買 2,500円 100株」は「2,500円」・「250,000円」', () => {
    renderTable('JPY', [trade('buy', '2026-01-10', 3, '1234.5'), trade('buy', '2026-01-11', 100, '2500.0')])

    expect(rowTexts()).toEqual([
      ['2026-01-10', '買', '3株', '1,234.5円', '3,703.5円'],
      ['2026-01-11', '買', '100株', '2,500円', '250,000円'],
    ])
  })

  test('基準18: ドルの「買 150.25ドル 10株」は価格「150.25ドル」・金額「1,502.50ドル」', () => {
    renderTable('USD', [trade('buy', '2026-01-10', 10, '150.25', { currency: 'USD' })])

    expect(rowTexts()).toEqual([['2026-01-10', '買', '10株', '150.25ドル', '1,502.50ドル']])
  })

  test('基準19: ドルの「買 99,999,999.99ドル 9,999,999株」の金額は「999,999,899,900,000.01ドル」（誤差なし。API が返した文字列をそのまま書式にする）', () => {
    renderTable('USD', [
      {
        kind: 'trade',
        date: '2026-01-10',
        side: 'buy',
        quantity: 9999999,
        price: '99999999.99',
        amount: '999999899900000.01',
        splitDivisor: '1',
        adjustedPrice: '99999999.99',
      },
    ])

    expect(rowTexts()).toEqual([['2026-01-10', '買', '9,999,999株', '99,999,999.99ドル', '999,999,899,900,000.01ドル']])
  })

  test('基準19: 円の「買 99,999,999.9円 9,999,999株」の金額は「999,999,899,000,000.1円」', () => {
    renderTable('JPY', [
      {
        kind: 'trade',
        date: '2026-01-10',
        side: 'buy',
        quantity: 9999999,
        price: '99999999.9',
        amount: '999999899000000.1',
        splitDivisor: '1',
        adjustedPrice: '99999999.9',
      },
    ])

    expect(rowTexts()).toEqual([['2026-01-10', '買', '9,999,999株', '99,999,999.9円', '999,999,899,000,000.1円']])
  })
})

describe('分割の行（基準20。要件15・要件17）', () => {
  test('基準20: 「2026-03-31 買 30株 1,000円」「2026-03-31 分割 1株→3株」「2026-01-10 買 100株 2,500円」の3行。分割の行は価格と金額が空', () => {
    renderTable('JPY', [
      trade('buy', '2026-03-31', 30, '1000.0'),
      split('2026-03-31', 3),
      trade('buy', '2026-01-10', 100, '2500.0', { splitDivisor: '3', adjustedPrice: '833.3' }),
    ])

    expect(rowTexts()).toEqual([
      ['2026-03-31', '買', '30株', '1,000円', '30,000円'],
      ['2026-03-31', '分割', '1株→3株', '', ''],
      ['2026-01-10', '買', '100株', '2,500円', '250,000円'],
    ])
  })

  test('基準20（要件15）: 分割があっても株数と価格は割らずにそのまま出す。2,500円・割る数 3・割った価格 833.3 の売買は「2,500円」で、833.3 は出ない', () => {
    renderTable('JPY', [trade('buy', '2026-01-10', 100, '2500.0', { splitDivisor: '3', adjustedPrice: '833.3' })])

    expect(rowTexts()).toEqual([['2026-01-10', '買', '100株', '2,500円', '250,000円']])
    expect(screen.queryByText(/833/)).not.toBeInTheDocument()
  })

  test('基準20（プラン 4.8）: 分割の行も5つのセルを持つ（価格と金額は colSpan でまとめない空の <td>）', () => {
    renderTable('JPY', [split('2026-03-31', 3), trade('buy', '2026-01-10', 100, '2500.0')])

    const [splitRow] = dataRows()
    expect(splitRow.children).toHaveLength(5)
    expect(Array.from(splitRow.children).every((cell) => !cell.hasAttribute('colspan'))).toBe(true)
    expect(splitRow.children[3].tagName).toBe('TD')
    expect(splitRow.children[4].tagName).toBe('TD')
  })

  test('補助 要件17: 分割の比率は分割後の株数。分割 1→5 は「1株→5株」', () => {
    renderTable('JPY', [split('2026-03-31', 5), trade('buy', '2026-01-10', 100, '2500.0')])

    expect(rowTexts()[0].slice(1, 3)).toEqual(['分割', '1株→5株'])
  })
})
