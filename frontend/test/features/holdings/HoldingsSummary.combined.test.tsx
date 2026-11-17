// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import type { CombinedTotal, CurrencyTotal } from '../../../src/entities/holding/model'
import { HoldingsSummary } from '../../../src/features/holdings/components/HoldingsSummary'
import type { TotalsView } from '../../../src/features/holdings/lib/totalsViewStorage'

// 仕様: 要件 docs/requirements/fx-total.md の要件1〜要件4・要件9〜要件12・要件17、基準2・基準3・基準8・基準9・基準11・基準12・基準16（単体の部分）。
//   通貨ごとの表示（要件3）は、カード表示の要件12〜要件16（HoldingsSummary.test.tsx）と同じ。
// 境界: プラン docs/plans/fx-total/plan.md 4.5・4.7（PR②）
//   - HoldingsSummary の props: { totals, combinedTotal: CombinedTotal | null, view: TotalsView, onViewChange: (view: TotalsView) => void }。
//     view は覚えた表示で、実際に出す表示（combinedTotal が null なら通貨ごと）は HoldingsSummary の中で決める（要件17）。
//   - 切り替えは getByRole('group', { name: '合計の表示' }) の中の getByRole('button', { name: '通貨ごと' })・{ name: '円にまとめる' }。押されている状態は aria-pressed。
//   - 円にまとめた部分: 「円」「ドル」の見出し（h3）・region を出さない。含み損益（data-tone を持つ要素が1つ）、評価額、価格のない銘柄の注記、
//     レートの行「1ドル = <レート>円（<日付> のレート）で換算」の順。合計の欄は今と同じ region「合計」（h2）。
//   - CombinedSummary の props は実装担当が決める（境界にしない）ので、HoldingsSummary に combinedTotal と view='combinedYen' を渡して描く。
// テスト計画: プラン 5.1 の ② の行（基準2・基準3・基準8・基準9・基準11・基準12）と、5章冒頭の localStorage の後始末。
// mock は使わない（onViewChange のコールバックだけ、押されたことを確かめるための関数を渡す）。

const UNPRICED_NOTE = '価格を取得できない銘柄を含みません'
const UNAVAILABLE = '取得できません'

afterEach(() => {
  cleanup()
  window.localStorage.clear()
  vi.restoreAllMocks()
})

/** 基準1: 7203「買 2,500円 100株」・AAPL「買 150.25ドル 10株」（どちらも価格あり）。円は +50,000円（+20.0%）・300,000円、ドルは +297.50ドル（+19.8%）・1,800.00ドル。 */
const TOTALS: CurrencyTotal[] = [
  { currency: 'JPY', valuation: '300000', excludesUnpriced: false, unrealizedGain: '50000', unrealizedGainRate: '20.0' },
  { currency: 'USD', valuation: '1800.00', excludesUnpriced: false, unrealizedGain: '297.50', unrealizedGainRate: '19.8' },
]

/** 基準1: レート 2026-11-14 150.00円。円にまとめた値は +94,625円（+19.9%）・570,000円。 */
const COMBINED: CombinedTotal = {
  valuation: '570000',
  excludesUnpriced: false,
  unrealizedGain: '94625',
  unrealizedGainRate: '19.9',
  usdJpyRate: '150.00',
  usdJpyRateDate: '2026-11-14',
}

function renderSummary(
  props: {
    totals?: readonly CurrencyTotal[]
    combinedTotal?: CombinedTotal | null
    view?: TotalsView
    onViewChange?: (view: TotalsView) => void
  } = {},
) {
  return render(
    <HoldingsSummary
      totals={props.totals ?? TOTALS}
      combinedTotal={props.combinedTotal === undefined ? COMBINED : props.combinedTotal}
      view={props.view ?? 'byCurrency'}
      onViewChange={props.onViewChange ?? (() => {})}
    />,
  )
}

/** 合計の欄（region「合計」）。 */
function summaryRegion(): HTMLElement {
  return screen.getByRole('region', { name: '合計' })
}

/** 切り替えの組（合計の欄の中の role="group"、名前「合計の表示」）。 */
function switchGroup(): HTMLElement {
  return within(summaryRegion()).getByRole('group', { name: '合計の表示' })
}

/** 含み損益と率を持つ要素（data-tone を持つ要素。合計の欄に1つ）。 */
function gainElement(): HTMLElement {
  const elements = summaryRegion().querySelectorAll<HTMLElement>('[data-tone]')
  expect(elements).toHaveLength(1)
  return elements[0]
}

/** レートの行の文（要件10）。 */
function rateLine(rate: string, date: string): string {
  return `1ドル = ${rate}円（${date} のレート）で換算`
}

/** elements が、文書の中でこの順に並んでいること（上から下へ）。 */
function expectDocumentOrder(elements: readonly HTMLElement[], names: readonly string[]): void {
  for (let i = 1; i < elements.length; i += 1) {
    const follows = (elements[i - 1].compareDocumentPosition(elements[i]) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0
    expect(follows, `「${names[i - 1]}」の後に「${names[i]}」が来る`).toBe(true)
  }
}

describe('切り替え（要件1・要件2。基準2・基準3）', () => {
  // 基準2: 「合計の表示」という名前の組に2つのボタン。「通貨ごと」だけが押されている状態。
  test('基準2: combinedTotal があり view が byCurrency のとき、合計の欄の「合計の表示」の組に「通貨ごと」「円にまとめる」のボタンがあり、「通貨ごと」だけ aria-pressed="true"', () => {
    renderSummary({ view: 'byCurrency' })

    const group = switchGroup()
    expect(within(group).getAllByRole('button')).toHaveLength(2)
    expect(within(group).getByRole('button', { name: '通貨ごと' })).toHaveAttribute('aria-pressed', 'true')
    expect(within(group).getByRole('button', { name: '円にまとめる' })).toHaveAttribute('aria-pressed', 'false')
  })

  // 基準2: 押されている状態は view に従う（「円にまとめる」だけが押されている状態になる）。
  test('基準2: view が combinedYen のとき、「円にまとめる」だけ aria-pressed="true"', () => {
    renderSummary({ view: 'combinedYen' })

    const group = switchGroup()
    expect(within(group).getByRole('button', { name: '通貨ごと' })).toHaveAttribute('aria-pressed', 'false')
    expect(within(group).getByRole('button', { name: '円にまとめる' })).toHaveAttribute('aria-pressed', 'true')
  })

  // 基準2: 「円にまとめる」を押すと onViewChange('combinedYen')。
  test('基準2: view が byCurrency のとき、「円にまとめる」を押すと onViewChange("combinedYen") が1回呼ばれる', () => {
    const onViewChange = vi.fn()
    renderSummary({ view: 'byCurrency', onViewChange })

    fireEvent.click(within(switchGroup()).getByRole('button', { name: '円にまとめる' }))

    expect(onViewChange).toHaveBeenCalledTimes(1)
    expect(onViewChange).toHaveBeenCalledWith('combinedYen')
  })

  test('基準1 の部品: view が combinedYen のとき、「通貨ごと」を押すと onViewChange("byCurrency") が1回呼ばれる', () => {
    const onViewChange = vi.fn()
    renderSummary({ view: 'combinedYen', onViewChange })

    fireEvent.click(within(switchGroup()).getByRole('button', { name: '通貨ごと' }))

    expect(onViewChange).toHaveBeenCalledTimes(1)
    expect(onViewChange).toHaveBeenCalledWith('byCurrency')
  })

  // 要件1: 切り替えは見出し「合計」と同じ欄（region「合計」）の中にある。見出し（h2）は「合計」の1つだけ。
  test('要件1: 切り替えは region「合計」の中にあり、見出し h2 は「合計」の1つだけ', () => {
    renderSummary({ view: 'byCurrency' })

    expect(screen.getAllByRole('heading', { level: 2 }).map((heading) => heading.textContent)).toEqual(['合計'])
    expect(summaryRegion()).toContainElement(switchGroup())
  })

  // 基準3: combinedTotal が null → 切り替えは表示されず、円の部分だけが表示される。
  test('基準3: combinedTotal が null で、円だけの totals のとき、組「合計の表示」はなく、円の部分だけが表示される', () => {
    renderSummary({ totals: [TOTALS[0]], combinedTotal: null, view: 'byCurrency' })

    expect(within(summaryRegion()).queryByRole('group', { name: '合計の表示' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '通貨ごと' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '円にまとめる' })).not.toBeInTheDocument()
    expect(within(summaryRegion()).getByRole('region', { name: '円' })).toBeInTheDocument()
    expect(within(summaryRegion()).queryByRole('region', { name: 'ドル' })).not.toBeInTheDocument()
  })

  // 基準16 の部品（要件17）: 「円にまとめる」を覚えていても（view が combinedYen）、combinedTotal が null なら、切り替えは出ず、円の部分だけ。
  test('基準16（要件17）: view が combinedYen でも combinedTotal が null なら、組はなく、円の部分だけが表示され、円にまとめた表示（レートの行）は出ない', () => {
    renderSummary({ totals: [TOTALS[0]], combinedTotal: null, view: 'combinedYen' })

    expect(within(summaryRegion()).queryByRole('group', { name: '合計の表示' })).not.toBeInTheDocument()
    expect(within(summaryRegion()).getByRole('region', { name: '円' })).toBeInTheDocument()
    expect(within(summaryRegion()).queryByRole('region', { name: 'ドル' })).not.toBeInTheDocument()
    expect(summaryRegion()).not.toHaveTextContent('で換算')
  })
})

describe('通貨ごとの表示（要件3）', () => {
  // 要件3: 「通貨ごと」のときは、今の表示と同じ（円・ドルの部分。円にまとめた値・レートの行は出ない）。
  test('要件3: view が byCurrency のとき、円・ドルの見出し（h3）と部分がこの順に表示され、円にまとめた値とレートの行は表示されない', () => {
    renderSummary({ view: 'byCurrency' })

    expect(screen.getAllByRole('heading', { level: 3 }).map((heading) => heading.textContent)).toEqual(['円', 'ドル'])
    const yen = within(summaryRegion()).getByRole('region', { name: '円' })
    const usd = within(summaryRegion()).getByRole('region', { name: 'ドル' })
    expect(within(yen).getByText('+50,000円（+20.0%）')).toBeInTheDocument()
    expect(within(yen).getByText('300,000円')).toBeInTheDocument()
    expect(within(usd).getByText('+297.50ドル（+19.8%）')).toBeInTheDocument()
    expect(within(usd).getByText('1,800.00ドル')).toBeInTheDocument()
    expect(summaryRegion()).not.toHaveTextContent('94,625')
    expect(summaryRegion()).not.toHaveTextContent('570,000')
    expect(summaryRegion()).not.toHaveTextContent('で換算')
    expect(summaryRegion().querySelectorAll('[data-tone]')).toHaveLength(2)
  })
})

describe('円にまとめた表示（要件4・要件9・要件10。基準1・基準8・基準9）', () => {
  // 基準1 の部品: 「含み損益 +94,625円（+19.9%）/ 評価額 570,000円 / 1ドル = 150.00円（2026-11-14 のレート）で換算」。「円」「ドル」の見出しは表示されない。
  test('基準1: 円にまとめた合計の欄は「含み損益 +94,625円（+19.9%）/ 評価額 570,000円 / 1ドル = 150.00円（2026-11-14 のレート）で換算」で、「円」「ドル」の見出しと部分は表示されない', () => {
    renderSummary({ view: 'combinedYen' })

    const summary = summaryRegion()
    expect(gainElement().textContent?.trim()).toBe('+94,625円（+19.9%）')
    expect(within(summary).getByText('570,000円')).toBeInTheDocument()
    expect(within(summary).getByText(rateLine('150.00', '2026-11-14'))).toBeInTheDocument()
    // 「円」「ドル」の見出し（h3）も、通貨ごとの region も出さない。見出しは h2「合計」だけ。
    expect(screen.queryAllByRole('heading', { level: 3 })).toHaveLength(0)
    expect(within(summary).queryByRole('heading', { name: '円' })).not.toBeInTheDocument()
    expect(within(summary).queryByRole('heading', { name: 'ドル' })).not.toBeInTheDocument()
    expect(within(summary).queryByRole('region', { name: '円' })).not.toBeInTheDocument()
    expect(within(summary).queryByRole('region', { name: 'ドル' })).not.toBeInTheDocument()
    // 通貨ごとの値（ドルの含み損益・評価額）は出ない。
    expect(summary).not.toHaveTextContent('297.50')
    expect(summary).not.toHaveTextContent('1,800.00')
    expect(summary).not.toHaveTextContent('+50,000円')
    // 注記は出ない（価格のない銘柄がない）。
    expect(screen.queryByText(UNPRICED_NOTE)).not.toBeInTheDocument()
  })

  // 要件4: 見出し「含み損益」「評価額」、含み損益と率、評価額、その下に使ったレートの行の順。
  test('要件4: 「含み損益」、含み損益と率、「評価額」、評価額の値、レートの行の順に並ぶ', () => {
    renderSummary({ view: 'combinedYen' })

    const summary = summaryRegion()
    expectDocumentOrder(
      [
        within(summary).getByRole('heading', { level: 2, name: '合計' }),
        within(summary).getByText('含み損益'),
        gainElement(),
        within(summary).getByText('評価額'),
        within(summary).getByText('570,000円'),
        within(summary).getByText(rateLine('150.00', '2026-11-14')),
      ],
      ['見出し「合計」', '含み損益', '含み損益と率', '評価額', '評価額の値', 'レートの行'],
    )
  })

  // 要件9・基準8: 色は円の通貨ごとの表示と同じ（プラスは gain）。
  test('基準1（要件9）: プラスの含み損益の data-tone は gain になる', () => {
    renderSummary({ view: 'combinedYen' })

    expect(gainElement()).toHaveAttribute('data-tone', 'gain')
  })

  // 基準8: unrealizedGain "-10002"・unrealizedGainRate "-8.7" → 「-10,002円（-8.7%）」、損の色（loss）。
  test('基準8: 含み損益 "-10002"・率 "-8.7" は「-10,002円（-8.7%）」で表示され、data-tone は loss になる', () => {
    renderSummary({
      view: 'combinedYen',
      combinedTotal: { ...COMBINED, valuation: '123456', unrealizedGain: '-10002', unrealizedGainRate: '-8.7' },
    })

    expect(gainElement().textContent?.trim()).toBe('-10,002円（-8.7%）')
    expect(gainElement()).toHaveAttribute('data-tone', 'loss')
  })

  // 要件9: 含み損益が0なら「0円（0.0%）」で色なし（円の通貨ごとの表示と同じ。API の率が "-0.1" でも 0.0）。
  test('補助（要件9）: 含み損益 "0" なら「0円（0.0%）」で data-tone は even になる', () => {
    renderSummary({
      view: 'combinedYen',
      combinedTotal: { ...COMBINED, unrealizedGain: '0', unrealizedGainRate: '-0.1' },
    })

    expect(gainElement().textContent?.trim()).toBe('0円（0.0%）')
    expect(gainElement()).toHaveAttribute('data-tone', 'even')
  })

  // 要件9: 評価額の書式は円の通貨ごとの表示と同じ（3桁区切り）。
  test('補助（要件9）: 評価額 "1422500" は「1,422,500円」と3桁区切りで表示される', () => {
    renderSummary({
      view: 'combinedYen',
      combinedTotal: { ...COMBINED, valuation: '1422500', unrealizedGain: '89625', unrealizedGainRate: '6.7' },
    })

    expect(within(summaryRegion()).getByText('1,422,500円')).toBeInTheDocument()
    expect(gainElement().textContent?.trim()).toBe('+89,625円（+6.7%）')
  })

  // 基準9: レートの行（要件10）。レートは整数部分を3桁区切りにして小数を常に2桁、日付は YYYY-MM-DD。
  test.each([
    ['150.00', '2026-11-14', '1ドル = 150.00円（2026-11-14 のレート）で換算'],
    ['1000.50', '2026-01-05', '1ドル = 1,000.50円（2026-01-05 のレート）で換算'],
  ])('基準9: レート %s・日付 %s は「%s」と表示される', (usdJpyRate, usdJpyRateDate, expected) => {
    renderSummary({ view: 'combinedYen', combinedTotal: { ...COMBINED, usdJpyRate, usdJpyRateDate } })

    expect(within(summaryRegion()).getByText(expected)).toBeInTheDocument()
  })

  // 要件10: レートの日付がどれだけ古くても、そのまま使い、注意の文言は足さない（レートの行は、換算の文のほかに何も足さない）。
  test('要件10: 古い日付（2020-01-01）のレートでも、レートの行はそのまま表示され、注意の文言（古い・更新など）は足されない', () => {
    renderSummary({ view: 'combinedYen', combinedTotal: { ...COMBINED, usdJpyRateDate: '2020-01-01' } })

    const line = within(summaryRegion()).getByText(rateLine('150.00', '2020-01-01'))
    expect(line.textContent?.trim()).toBe(rateLine('150.00', '2020-01-01'))
    expect(summaryRegion()).not.toHaveTextContent(/古い|注意|更新されて|取得できません/)
  })
})

describe('価格のない銘柄（要件11・要件12。基準11・基準12）', () => {
  // 基準11: 7203（価格あり）・MSFT（価格なし）→ 「含み損益 +50,000円（+20.0%）/ 評価額 300,000円」と「価格を取得できない銘柄を含みません」が1つ。
  test('基準11: excludesUnpriced が true のとき、「+50,000円（+20.0%）」「300,000円」と「価格を取得できない銘柄を含みません」が1つ表示される', () => {
    renderSummary({
      view: 'combinedYen',
      combinedTotal: {
        valuation: '300000',
        excludesUnpriced: true,
        unrealizedGain: '50000',
        unrealizedGainRate: '20.0',
        usdJpyRate: '150.00',
        usdJpyRateDate: '2026-11-14',
      },
    })

    expect(gainElement().textContent?.trim()).toBe('+50,000円（+20.0%）')
    expect(within(summaryRegion()).getByText('300,000円')).toBeInTheDocument()
    expect(screen.getAllByText(UNPRICED_NOTE)).toHaveLength(1)
    expect(within(summaryRegion()).getByText(rateLine('150.00', '2026-11-14'))).toBeInTheDocument()
  })

  test('基準10 の部品: excludesUnpriced が false のとき、注記は表示されない', () => {
    renderSummary({ view: 'combinedYen', combinedTotal: { ...COMBINED, excludesUnpriced: false } })

    expect(screen.queryByText(UNPRICED_NOTE)).not.toBeInTheDocument()
  })

  // 基準12: すべて価格がない → 「含み損益 取得できません / 評価額 0円」と注記。率は出ず、赤・青は付かない。
  test('基準12: unrealizedGain が null で excludesUnpriced が true のとき、「取得できません」（率なし、data-tone は even）、「0円」、注記が1つ表示される', () => {
    renderSummary({
      view: 'combinedYen',
      combinedTotal: {
        valuation: '0',
        excludesUnpriced: true,
        unrealizedGain: null,
        unrealizedGainRate: null,
        usdJpyRate: '150.00',
        usdJpyRateDate: '2026-11-14',
      },
    })

    const summary = summaryRegion()
    expect(gainElement().textContent?.trim()).toBe(UNAVAILABLE)
    expect(gainElement()).toHaveAttribute('data-tone', 'even')
    expect(summary).not.toHaveTextContent('%')
    expect(within(summary).getByText('0円')).toBeInTheDocument()
    expect(screen.getAllByText(UNPRICED_NOTE)).toHaveLength(1)
  })

  // 要件11: 注記は、価格のない銘柄があるときに1つだけ表示する（円・ドルの部分ごとには出さない）。
  test('補助（要件11）: 通貨ごとの totals の両方が excludesUnpriced でも、円にまとめた表示の注記は1つだけ', () => {
    renderSummary({
      view: 'combinedYen',
      totals: TOTALS.map((total) => ({ ...total, excludesUnpriced: true })),
      combinedTotal: { ...COMBINED, excludesUnpriced: true },
    })

    expect(screen.getAllByText(UNPRICED_NOTE)).toHaveLength(1)
  })
})
