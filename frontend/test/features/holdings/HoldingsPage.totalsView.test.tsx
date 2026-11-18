// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, expect, test, vi, type Mock } from 'vitest'
import { HoldingsPage } from '../../../src/features/holdings'

// 仕様: 要件 docs/requirements/fx-total.md の要件1・要件2・要件5・要件16〜要件18、基準2・基準15（画面の部分）・基準16・基準17。
// 境界: プラン docs/plans/fx-total/plan.md 4.5・4.7（PR②）
//   - HoldingsPage は一覧の応答の combinedTotal を HoldingsSummary に渡し、覚えた表示（localStorage のキー 'zero.holdings.totalsView'）を view に渡す。
//   - 切り替えは getByRole('group', { name: '合計の表示' }) の中の「通貨ごと」「円にまとめる」。押されている状態は aria-pressed。
//   - 円にまとめた部分では「円」「ドル」の region を出さず、レートの行「1ドル = …円（… のレート）で換算」を出す。
//   - サーバーには何も送らない（切り替えで fetch が増えない。要件5）。
// テスト計画: プラン 5.1 の基準2・基準16・基準17 の行（HoldingsPage.totalsView.test.tsx）と、5章冒頭の localStorage の後始末
//   （afterEach で localStorage.clear()・vi.restoreAllMocks()）。
// mock は境界の外側（ネットワーク = グローバルの fetch）と、保存が禁止されたブラウザーを作る Storage だけに使う。

const STORAGE_KEY = 'zero.holdings.totalsView'
const UNPRICED_NOTE = '価格を取得できない銘柄を含みません'

/** 基準1: 7203「買 2,500円 100株」・AAPL「買 150.25ドル 10株」（どちらも価格あり）、レート 2026-11-14 150.00円 の応答。 */
const COMBINED_BODY = {
  holdings: [
    {
      code: '7203',
      name: 'トヨタ自動車',
      currency: 'JPY',
      quantity: 100,
      acquisitionPrice: '2500.0',
      currentPrice: '3000.0',
      priceDate: '2026-10-06',
      valuation: '300000',
      unrealizedGain: '50000',
      unrealizedGainRate: '20.0',
    },
    {
      code: 'AAPL',
      name: 'Apple',
      currency: 'USD',
      quantity: 10,
      acquisitionPrice: '150.25',
      currentPrice: '180.00',
      priceDate: '2026-10-06',
      valuation: '1800.00',
      unrealizedGain: '297.50',
      unrealizedGainRate: '19.8',
    },
  ],
  totals: [
    { currency: 'JPY', valuation: '300000', excludesUnpriced: false, unrealizedGain: '50000', unrealizedGainRate: '20.0' },
    { currency: 'USD', valuation: '1800.00', excludesUnpriced: false, unrealizedGain: '297.50', unrealizedGainRate: '19.8' },
  ],
  combinedTotal: {
    valuation: '570000',
    excludesUnpriced: false,
    unrealizedGain: '94625',
    unrealizedGainRate: '19.9',
    usdJpyRate: '150.00',
    usdJpyRateDate: '2026-11-14',
  },
  managedAccounts: [],
}

/** 基準16: Aの保有が 7203 だけ（円だけ。ドルの保有がないので combinedTotal は null）の応答。 */
const YEN_ONLY_BODY = {
  holdings: [COMBINED_BODY.holdings[0]],
  totals: [COMBINED_BODY.totals[0]],
  combinedTotal: null,
  managedAccounts: [],
}

let queryClient: QueryClient
let fetchMock: Mock<typeof fetch>

beforeEach(() => {
  queryClient = new QueryClient()
  fetchMock = vi.fn<typeof fetch>(() => new Promise<Response>(() => {}))
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  vi.unstubAllGlobals()
  // jsdom の localStorage は同じファイルのテストの間で残るので、必ず空にする（プラン 5章冒頭）。
  window.localStorage.clear()
  vi.restoreAllMocks()
})

function respondWithHoldings(body: unknown): void {
  fetchMock.mockImplementation(
    async () =>
      new Response(JSON.stringify(body), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
  )
}

function renderHoldingsPage() {
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <HoldingsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

/** 合計の欄（取得が終わるまで待つ）。 */
async function findSummary(): Promise<HTMLElement> {
  return screen.findByRole('region', { name: '合計' })
}

function switchButton(summary: HTMLElement, name: '通貨ごと' | '円にまとめる'): HTMLElement {
  return within(within(summary).getByRole('group', { name: '合計の表示' })).getByRole('button', { name })
}

/** 円にまとめた表示（基準1）が出ている: 円にまとめた値とレートの行があり、円・ドルの部分はない。 */
function expectCombinedYenShown(summary: HTMLElement): void {
  expect(summary.querySelector('[data-tone]')?.textContent?.trim()).toBe('+94,625円（+19.9%）')
  expect(within(summary).getByText('570,000円')).toBeInTheDocument()
  expect(within(summary).getByText('1ドル = 150.00円（2026-11-14 のレート）で換算')).toBeInTheDocument()
  expect(within(summary).queryByRole('region', { name: '円' })).not.toBeInTheDocument()
  expect(within(summary).queryByRole('region', { name: 'ドル' })).not.toBeInTheDocument()
}

/** 通貨ごとの表示（基準1 の通貨ごと）が出ている: 円「+50,000円（+20.0%）/ 300,000円」とドル「+297.50ドル（+19.8%）/ 1,800.00ドル」。 */
function expectByCurrencyShown(summary: HTMLElement): void {
  const yen = within(summary).getByRole('region', { name: '円' })
  const usd = within(summary).getByRole('region', { name: 'ドル' })
  expect(within(yen).getByText('+50,000円（+20.0%）')).toBeInTheDocument()
  expect(within(yen).getByText('300,000円')).toBeInTheDocument()
  expect(within(usd).getByText('+297.50ドル（+19.8%）')).toBeInTheDocument()
  expect(within(usd).getByText('1,800.00ドル')).toBeInTheDocument()
  expect(summary).not.toHaveTextContent('で換算')
}

test('基準2（画面）: 基準1 の応答のとき、最初は「通貨ごと」だけが押された状態で円・ドルの部分が出て、「円にまとめる」を押すと「円にまとめる」だけが押された状態になり、円にまとめた部分が出る', async () => {
  respondWithHoldings(COMBINED_BODY)

  renderHoldingsPage()

  const summary = await findSummary()
  expect(switchButton(summary, '通貨ごと')).toHaveAttribute('aria-pressed', 'true')
  expect(switchButton(summary, '円にまとめる')).toHaveAttribute('aria-pressed', 'false')
  expectByCurrencyShown(summary)

  fireEvent.click(switchButton(summary, '円にまとめる'))

  expect(switchButton(summary, '通貨ごと')).toHaveAttribute('aria-pressed', 'false')
  expect(switchButton(summary, '円にまとめる')).toHaveAttribute('aria-pressed', 'true')
  expectCombinedYenShown(summary)
})

test('基準1 の部品（画面）: 「円にまとめる」の後に「通貨ごと」を押すと、通貨ごとの表示に戻る', async () => {
  respondWithHoldings(COMBINED_BODY)
  renderHoldingsPage()
  const summary = await findSummary()

  fireEvent.click(switchButton(summary, '円にまとめる'))
  fireEvent.click(switchButton(summary, '通貨ごと'))

  expect(switchButton(summary, '通貨ごと')).toHaveAttribute('aria-pressed', 'true')
  expect(switchButton(summary, '円にまとめる')).toHaveAttribute('aria-pressed', 'false')
  expectByCurrencyShown(summary)
})

test('基準5 の部品（要件5）: 切り替えても、カードの表示と並び（7203、AAPL）は変わらず、サーバーへの要求（fetch）は増えない', async () => {
  respondWithHoldings(COMBINED_BODY)
  renderHoldingsPage()
  const summary = await findSummary()
  const cards = screen.getByRole('list', { name: '保有株式' })
  const cardsBefore = cards.textContent
  const fetchCallsBefore = fetchMock.mock.calls.length

  fireEvent.click(switchButton(summary, '円にまとめる'))

  expect(within(cards).getAllByRole('listitem')).toHaveLength(2)
  expect(cards.textContent).toBe(cardsBefore)
  expect(cards).toHaveTextContent('+297.50ドル（+19.8%）')
  expect(cards).toHaveTextContent('1,800.00ドル')
  expect(fetchMock.mock.calls.length).toBe(fetchCallsBefore)
})

test('基準15 の部品（要件16）: 「円にまとめる」を押すと localStorage に combinedYen が入り、再び描画（再読み込みの代わり）すると、「円にまとめる」が押された状態で円にまとめた表示から始まる', async () => {
  respondWithHoldings(COMBINED_BODY)
  const first = renderHoldingsPage()
  const summary = await findSummary()

  fireEvent.click(switchButton(summary, '円にまとめる'))
  expect(window.localStorage.getItem(STORAGE_KEY)).toBe('combinedYen')

  // 再読み込みの代わり: 画面を作り直す（localStorage は残る）。
  first.unmount()
  queryClient.clear()
  renderHoldingsPage()

  const reopened = await findSummary()
  expect(switchButton(reopened, '円にまとめる')).toHaveAttribute('aria-pressed', 'true')
  expect(switchButton(reopened, '通貨ごと')).toHaveAttribute('aria-pressed', 'false')
  expectCombinedYenShown(reopened)
})

test('基準15 の部品（要件16）: localStorage に combinedYen を入れておくと、最初から「円にまとめる」が押された状態で円にまとめた表示になる', async () => {
  window.localStorage.setItem(STORAGE_KEY, 'combinedYen')
  respondWithHoldings(COMBINED_BODY)

  renderHoldingsPage()

  const summary = await findSummary()
  expect(switchButton(summary, '円にまとめる')).toHaveAttribute('aria-pressed', 'true')
  expectCombinedYenShown(summary)
})

test('要件16: 何も覚えていないとき（localStorage が空）は「通貨ごと」で始まる', async () => {
  respondWithHoldings(COMBINED_BODY)

  renderHoldingsPage()

  const summary = await findSummary()
  expect(switchButton(summary, '通貨ごと')).toHaveAttribute('aria-pressed', 'true')
  expectByCurrencyShown(summary)
})

test('基準16（要件17）: localStorage に combinedYen を入れ、7203 だけ（combinedTotal が null）の応答のとき、組「合計の表示」はなく円の部分だけが表示され、描いた後も localStorage の値は combinedYen のまま', async () => {
  window.localStorage.setItem(STORAGE_KEY, 'combinedYen')
  respondWithHoldings(YEN_ONLY_BODY)

  renderHoldingsPage()

  const summary = await findSummary()
  expect(within(summary).queryByRole('group', { name: '合計の表示' })).not.toBeInTheDocument()
  expect(within(summary).getByRole('region', { name: '円' })).toBeInTheDocument()
  expect(within(summary).queryByRole('region', { name: 'ドル' })).not.toBeInTheDocument()
  expect(summary).not.toHaveTextContent('で換算')
  expect(window.localStorage.getItem(STORAGE_KEY)).toBe('combinedYen')
})

test('基準16（要件17）: 切り替えを出せない状態から出せる状態に変わると、覚えていた「円にまとめる」で表示される（覚えた値は書き換わっていない）', async () => {
  window.localStorage.setItem(STORAGE_KEY, 'combinedYen')
  respondWithHoldings(YEN_ONLY_BODY)
  const first = renderHoldingsPage()
  await findSummary()
  expect(window.localStorage.getItem(STORAGE_KEY)).toBe('combinedYen')

  // 次に開いたとき、ドルの保有とレートがある（combinedTotal がある）。
  first.unmount()
  queryClient.clear()
  respondWithHoldings(COMBINED_BODY)
  renderHoldingsPage()

  const summary = await findSummary()
  expect(switchButton(summary, '円にまとめる')).toHaveAttribute('aria-pressed', 'true')
  expectCombinedYenShown(summary)
})

test('基準17（要件18）: localStorage の値が "abc" のとき、最初は「通貨ごと」で表示され、「円にまとめる」を押すと円にまとめた表示になり、エラーのダイアログは出ない', async () => {
  window.localStorage.setItem(STORAGE_KEY, 'abc')
  respondWithHoldings(COMBINED_BODY)

  renderHoldingsPage()

  const summary = await findSummary()
  expect(switchButton(summary, '通貨ごと')).toHaveAttribute('aria-pressed', 'true')
  expectByCurrencyShown(summary)

  fireEvent.click(switchButton(summary, '円にまとめる'))

  expect(switchButton(summary, '円にまとめる')).toHaveAttribute('aria-pressed', 'true')
  expectCombinedYenShown(summary)
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})

test('基準17（要件18）: Storage の getItem・setItem が例外を投げるとき、最初は「通貨ごと」で表示され、「円にまとめる」を押すと円にまとめた表示になり、エラーのダイアログは出ない', async () => {
  // 差し替えの前に「円にまとめる」を保存しておく。getItem の例外を受け止めたなら「通貨ごと」で始まり、
  // 差し替えが効かずに getItem が普通に値を返すなら「円にまとめる」で始まる（Given を作れたことの確かめ）。
  window.localStorage.setItem(STORAGE_KEY, 'combinedYen')
  const getItemSpy = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
    throw new DOMException('The operation is insecure.', 'SecurityError')
  })
  const setItemSpy = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new DOMException('The quota has been exceeded.', 'QuotaExceededError')
  })
  respondWithHoldings(COMBINED_BODY)

  renderHoldingsPage()

  const summary = await findSummary()
  expect(switchButton(summary, '通貨ごと')).toHaveAttribute('aria-pressed', 'true')
  expectByCurrencyShown(summary)
  expect(getItemSpy).toHaveBeenCalled()
  expect(getItemSpy.mock.results.some((r) => r.type === 'throw')).toBe(true)

  fireEvent.click(switchButton(summary, '円にまとめる'))

  expect(setItemSpy).toHaveBeenCalled()
  expect(setItemSpy.mock.results.some((r) => r.type === 'throw')).toBe(true)
  expect(switchButton(summary, '円にまとめる')).toHaveAttribute('aria-pressed', 'true')
  expectCombinedYenShown(summary)
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})

test('基準10 の部品（要件11）: 円にまとめる表示で、combinedTotal の excludesUnpriced が true なら、注記は1つだけ表示される', async () => {
  respondWithHoldings({
    ...COMBINED_BODY,
    combinedTotal: { ...COMBINED_BODY.combinedTotal, excludesUnpriced: true },
  })
  window.localStorage.setItem(STORAGE_KEY, 'combinedYen')

  renderHoldingsPage()

  await findSummary()
  expect(screen.getAllByText(UNPRICED_NOTE)).toHaveLength(1)
})
