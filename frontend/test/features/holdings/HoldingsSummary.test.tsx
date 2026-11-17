// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, test } from 'vitest'
import type { CurrencyTotal } from '../../../src/entities/holding/model'
import { HoldingsSummary } from '../../../src/features/holdings/components/HoldingsSummary'

// 仕様: カード表示の要件 docs/requirements/holdings-cards.md の要件11〜要件16、基準14・基準15・基準17（単体の部分）。
//   売買履歴の要件12・要件13（合計評価額。計算は変えない）の「評価額の合計と注記の決まり」は、元の HoldingsTotal.test.tsx から引き継いだ。
//   合計の欄の位置（「価格を今すぐ更新」のボタンより上）・文字の大きさ・色は E2E（holdings-cards.spec.ts）。
// 境界: プラン docs/plans/holdings-cards/plan.md 4.4（PR①）・4.6（PR①）。HoldingsTotal を置き換える。
//   - HoldingsSummary: props { totals: readonly CurrencyTotal[] }
//   - region「合計」（見出し h2「合計」）の中に、通貨ごとの region「円」「ドル」（見出し h3）を totals の順（円、ドル）に置く。
//   - 各通貨の中は「含み損益」「<含み損益と率 または 取得できません>」「評価額」「<評価額>」「<注記>」の順。含み損益の要素に data-tone。
//   - 含み損益の合計が null なら「取得できません」（率なし、色なし）。excludesUnpriced なら「価格を取得できない銘柄を含みません」。
//   - 金額の整形は shared/lib/format.ts の formatValuation・formatUnrealizedGain。
//   - totals が空なら、呼び出し側が HoldingsSummary を描画しない（0件の画面は HoldingsPage.test.tsx）。

//   ドルを円にまとめる合計（プラン docs/plans/fx-total/plan.md 4.7 の PR②・5.3）: props に combinedTotal・view・onViewChange を足した。
//   このファイルは通貨ごとの表示（combinedTotal: null）の既存の確かめなので、Summary で combinedTotal: null・view="byCurrency" を渡す。
//   円にまとめた表示と切り替えは HoldingsSummary.combined.test.tsx。

const UNPRICED_NOTE = '価格を取得できない銘柄を含みません'
const UNAVAILABLE = '取得できません'

/** 通貨ごとの表示（切り替えなし。combinedTotal: null）で HoldingsSummary を描く。 */
function Summary({ totals }: { totals: readonly CurrencyTotal[] }) {
  return <HoldingsSummary totals={totals} combinedTotal={null} view="byCurrency" onViewChange={() => {}} />
}

afterEach(() => {
  cleanup()
})

/** 合計の欄（region「合計」）。 */
function summaryRegion(): HTMLElement {
  return screen.getByRole('region', { name: '合計' })
}

/** 通貨ごとの部分（region「円」・「ドル」。合計の欄の中）。 */
function currencyRegion(name: '円' | 'ドル'): HTMLElement {
  return within(summaryRegion()).getByRole('region', { name })
}

/** 含み損益と率を持つ要素（data-tone を持つ要素。通貨の部分に1つ）。 */
function gainElement(region: HTMLElement): HTMLElement {
  const elements = region.querySelectorAll<HTMLElement>('[data-tone]')
  expect(elements).toHaveLength(1)
  return elements[0]
}

/** elements が、文書の中でこの順に並んでいること（上から下へ）。 */
function expectDocumentOrder(elements: readonly HTMLElement[], names: readonly string[]): void {
  for (let i = 1; i < elements.length; i += 1) {
    const before = elements[i - 1]
    const after = elements[i]
    const follows = (before.compareDocumentPosition(after) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0
    expect(follows, `「${names[i - 1]}」の後に「${names[i]}」が来る`).toBe(true)
  }
}

/** 基準14: 円は含み損益 +45,000円（+15.5%）・評価額 335,000円、ドルは +297.50ドル（+19.8%）・評価額 1,800.00ドル（注記なし）。 */
const YEN_GAIN: CurrencyTotal = {
  currency: 'JPY',
  valuation: '335000',
  excludesUnpriced: false,
  unrealizedGain: '45000',
  unrealizedGainRate: '15.5',
}
const USD_GAIN: CurrencyTotal = {
  currency: 'USD',
  valuation: '1800.00',
  excludesUnpriced: false,
  unrealizedGain: '297.50',
  unrealizedGainRate: '19.8',
}

describe('HoldingsSummary（合計の欄。要件11・要件12）', () => {
  // 基準14 の部品の部分: 円・ドルの順に、含み損益の合計とその率、評価額の合計が表示され、「価格を取得できない銘柄を含みません」は表示されない。
  test('基準14: 円「含み損益 +45,000円（+15.5%）/ 評価額 335,000円」、ドル「含み損益 +297.50ドル（+19.8%）/ 評価額 1,800.00ドル」がこの順に表示され、注記は表示されない', () => {
    render(<Summary totals={[YEN_GAIN, USD_GAIN]} />)

    const yen = currencyRegion('円')
    const usd = currencyRegion('ドル')
    expectDocumentOrder([yen, usd], ['円', 'ドル'])
    expect(gainElement(yen).textContent?.trim()).toBe('+45,000円（+15.5%）')
    expect(within(yen).getByText('335,000円')).toBeInTheDocument()
    expect(gainElement(usd).textContent?.trim()).toBe('+297.50ドル（+19.8%）')
    expect(within(usd).getByText('1,800.00ドル')).toBeInTheDocument()
    expect(screen.queryByText(UNPRICED_NOTE)).not.toBeInTheDocument()
  })

  // 基準14: 見出しは「合計」（h2）の1つと、通貨ごとの「円」「ドル」（h3）。
  test('基準14: 見出しは h2「合計」が1つと、通貨ごとの h3「円」「ドル」で、見出しの名前は region の名前と同じ', () => {
    render(<Summary totals={[YEN_GAIN, USD_GAIN]} />)

    expect(screen.getAllByRole('heading', { level: 2 }).map((heading) => heading.textContent)).toEqual(['合計'])
    expect(screen.getAllByRole('heading', { level: 3 }).map((heading) => heading.textContent)).toEqual(['円', 'ドル'])
    expect(within(summaryRegion()).getAllByText('合計')).toHaveLength(1)
  })

  // 要件12・プラン 4.6: 各通貨の中は「含み損益」「含み損益と率」「評価額」「評価額の値」の順。
  test('基準14: 各通貨の部分は、見出し、「含み損益」、含み損益と率、「評価額」、評価額の値の順に並ぶ', () => {
    render(<Summary totals={[YEN_GAIN, USD_GAIN]} />)

    for (const [name, gain, valuation] of [
      ['円', '+45,000円（+15.5%）', '335,000円'],
      ['ドル', '+297.50ドル（+19.8%）', '1,800.00ドル'],
    ] as const) {
      const region = currencyRegion(name)
      expectDocumentOrder(
        [
          within(region).getByRole('heading', { level: 3, name }),
          within(region).getByText('含み損益'),
          gainElement(region),
          within(region).getByText('評価額'),
          within(region).getByText(valuation),
        ],
        [`${name} の見出し`, '含み損益', gain, '評価額', valuation],
      )
    }
  })

  // 基準17: Aの保有が 7203 だけ（円だけ）→ 円の部分だけを表示し、ドルの部分は表示しない。
  test('基準17: totals が円だけなら、円の部分だけが表示され、ドルの部分（region「ドル」）は表示されない', () => {
    render(<Summary totals={[YEN_GAIN]} />)

    expect(currencyRegion('円')).toBeInTheDocument()
    expect(within(summaryRegion()).queryByRole('region', { name: 'ドル' })).not.toBeInTheDocument()
    expect(within(summaryRegion()).queryByRole('heading', { name: 'ドル' })).not.toBeInTheDocument()
    expect(summaryRegion()).not.toHaveTextContent('ドル')
  })

  // 要件12: ドルだけなら、ドルの部分だけを表示し、円の部分は表示しない。
  test('補助（要件12）: totals がドルだけなら、ドルの部分だけが表示され、円の部分は表示されない', () => {
    render(<Summary totals={[USD_GAIN]} />)

    expect(currencyRegion('ドル')).toBeInTheDocument()
    expect(within(summaryRegion()).queryByRole('region', { name: '円' })).not.toBeInTheDocument()
    expect(summaryRegion()).not.toHaveTextContent('円')
  })

  // 要件12: 為替で換算した、円とドルを合わせた合計は出さない（通貨の部分は円とドルの2つだけ。合計の欄の中の金額は4つだけ）。
  test('補助（要件12）: 円とドルを合わせた合計は出さない（通貨の部分は2つだけで、含み損益の要素も2つだけ）', () => {
    render(<Summary totals={[YEN_GAIN, USD_GAIN]} />)

    expect(within(summaryRegion()).getAllByRole('region')).toHaveLength(2)
    expect(summaryRegion().querySelectorAll('[data-tone]')).toHaveLength(2)
  })
})

describe('HoldingsSummary の注記と「取得できません」（要件15・要件16）', () => {
  // 基準15 の部品の部分: 円は含み損益あり・注記あり、ドルは「含み損益 取得できません / 評価額 0.00ドル」と注記。注記は円とドルに1つずつ（全体で2つ）。
  test('基準15: 円は「+50,000円（+20.0%）」「300,000円」と注記、ドルは「取得できません」「0.00ドル」と注記。注記は円とドルに1つずつ（2つ）', () => {
    render(
      <Summary
        totals={[
          { currency: 'JPY', valuation: '300000', excludesUnpriced: true, unrealizedGain: '50000', unrealizedGainRate: '20.0' },
          { currency: 'USD', valuation: '0.00', excludesUnpriced: true, unrealizedGain: null, unrealizedGainRate: null },
        ]}
      />,
    )

    const yen = currencyRegion('円')
    const usd = currencyRegion('ドル')
    expect(gainElement(yen).textContent?.trim()).toBe('+50,000円（+20.0%）')
    expect(within(yen).getByText('300,000円')).toBeInTheDocument()
    expect(within(yen).getAllByText(UNPRICED_NOTE)).toHaveLength(1)
    expect(gainElement(usd).textContent?.trim()).toBe(UNAVAILABLE)
    expect(within(usd).getByText('0.00ドル')).toBeInTheDocument()
    expect(within(usd).getAllByText(UNPRICED_NOTE)).toHaveLength(1)
    expect(screen.getAllByText(UNPRICED_NOTE)).toHaveLength(2)
  })

  // 要件16: 含み損益の合計が null のとき、「取得できません」と表示して率は出さず、赤・青も付けない（data-tone が even）。
  test('基準15（要件16）: 含み損益の合計が null の通貨は、「取得できません」だけで率は出ず、data-tone が even になる', () => {
    render(
      <Summary
        totals={[{ currency: 'USD', valuation: '0.00', excludesUnpriced: true, unrealizedGain: null, unrealizedGainRate: null }]}
      />,
    )

    const usd = currencyRegion('ドル')
    expect(gainElement(usd)).toHaveAttribute('data-tone', 'even')
    expect(within(usd).queryByText(/%/)).not.toBeInTheDocument()
    expect(usd).not.toHaveTextContent('%')
  })

  // 要件15: 注記は、価格のない銘柄がある通貨の部分の中にだけある。注記は評価額の値の後（その通貨の部分の最後）。
  test('補助（要件15）: 円だけ excludesUnpriced なら、注記は円の部分にだけあり、ドルの部分にはない。注記は評価額の後に出る', () => {
    render(
      <Summary totals={[{ ...YEN_GAIN, valuation: '300000', excludesUnpriced: true }, USD_GAIN]} />,
    )

    const yen = currencyRegion('円')
    const usd = currencyRegion('ドル')
    expect(within(yen).getAllByText(UNPRICED_NOTE)).toHaveLength(1)
    expect(within(usd).queryByText(UNPRICED_NOTE)).not.toBeInTheDocument()
    expect(screen.getAllByText(UNPRICED_NOTE)).toHaveLength(1)
    expectDocumentOrder(
      [within(yen).getByText('評価額'), within(yen).getByText('300,000円'), within(yen).getByText(UNPRICED_NOTE)],
      ['評価額', '評価額の値', '注記'],
    )
  })

  test('補助（要件15）: ドルだけ excludesUnpriced なら、注記はドルの部分にだけあり、円の部分にはない', () => {
    render(
      <Summary
        totals={[YEN_GAIN, { ...USD_GAIN, valuation: '0.00', excludesUnpriced: true }]}
      />,
    )

    expect(within(currencyRegion('円')).queryByText(UNPRICED_NOTE)).not.toBeInTheDocument()
    expect(within(currencyRegion('ドル')).getAllByText(UNPRICED_NOTE)).toHaveLength(1)
    expect(screen.getAllByText(UNPRICED_NOTE)).toHaveLength(1)
  })
})

describe('HoldingsSummary の含み損益の色の種類（data-tone。要件4・要件13）', () => {
  test('基準3・基準4（要件13の「表示形式・色は要件4・要件10と同じ」）: プラスは gain、マイナスは loss、0 は even', () => {
    render(
      <Summary
        totals={[
          { currency: 'JPY', valuation: '335000', excludesUnpriced: false, unrealizedGain: '45000', unrealizedGainRate: '15.5' },
          { currency: 'USD', valuation: '1800.00', excludesUnpriced: false, unrealizedGain: '-0.01', unrealizedGainRate: '0.0' },
        ]}
      />,
    )

    expect(gainElement(currencyRegion('円'))).toHaveAttribute('data-tone', 'gain')
    expect(gainElement(currencyRegion('ドル'))).toHaveAttribute('data-tone', 'loss')
    expect(gainElement(currencyRegion('ドル')).textContent?.trim()).toBe('-0.01ドル（-0.0%）')
  })

  test('基準4: 含み損益の合計が0なら「0円（0.0%）」で data-tone が even（API の率が "-0.1" でも率は 0.0）', () => {
    render(
      <Summary
        totals={[
          { currency: 'JPY', valuation: '20000', excludesUnpriced: false, unrealizedGain: '0', unrealizedGainRate: '-0.1' },
        ]}
      />,
    )

    const yen = currencyRegion('円')
    expect(gainElement(yen).textContent?.trim()).toBe('0円（0.0%）')
    expect(gainElement(yen)).toHaveAttribute('data-tone', 'even')
  })
})

describe('HoldingsSummary の評価額の合計（売買履歴の要件12〜14。計算と表示は変えない）', () => {
  test('補助（売買履歴の要件14）: 大きな金額は3桁区切りで表示される（円「1,234,567円」・ドル「1,234,567.50ドル」）', () => {
    render(
      <Summary
        totals={[
          { currency: 'JPY', valuation: '1234567', excludesUnpriced: false, unrealizedGain: '34567', unrealizedGainRate: '2.9' },
          { currency: 'USD', valuation: '1234567.50', excludesUnpriced: false, unrealizedGain: '34567.50', unrealizedGainRate: '2.9' },
        ]}
      />,
    )

    expect(within(currencyRegion('円')).getByText('1,234,567円')).toBeInTheDocument()
    expect(within(currencyRegion('ドル')).getByText('1,234,567.50ドル')).toBeInTheDocument()
  })

  test('補助（売買履歴の要件13）: すべて価格がない通貨の評価額の合計は「0.00ドル」（円なら「0円」）と表示される', () => {
    render(
      <Summary
        totals={[
          { currency: 'JPY', valuation: '0', excludesUnpriced: true, unrealizedGain: null, unrealizedGainRate: null },
          { currency: 'USD', valuation: '0.00', excludesUnpriced: true, unrealizedGain: null, unrealizedGainRate: null },
        ]}
      />,
    )

    expect(within(currencyRegion('円')).getByText('0円')).toBeInTheDocument()
    expect(within(currencyRegion('ドル')).getByText('0.00ドル')).toBeInTheDocument()
  })
})
