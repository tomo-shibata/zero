// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeAll, beforeEach, expect, test, vi, type Mock } from 'vitest'
import { HoldingsPage } from '../../../src/features/holdings'

// 仕様: おまかせ運用の要件 docs/requirements/managed-account.md の要件9・要件15〜要件20・要件22〜要件24、基準2・基準12・基準13・基準14 の単体の部分。
//   （基準2・基準13・基準14 の通しの確認は E2E（managed-account.spec.ts）。ここは HoldingsPage に API の応答を渡したときの画面の単体。）
// 境界: プラン docs/plans/managed-account/plan.md 4.6・4.8 の PR③
//   - HoldingsPage（変える）: 0件の判定を「銘柄も口座もない」にする。並べ方は、合計の欄 → おまかせ運用のカード → 更新のボタン・並びの保存の失敗・株式のカード
//     （この3つは銘柄があるときだけ）。削除の対象は1つの state で持ち、株式か口座かの印を付ける（閉じた後のフォーカスの effect を口座にもそのまま使う）。
//   - 口座の並びは list「おまかせ運用」、株式の並びは list「保有株式」、合計の欄は region「合計」（円は region「円」）。
//   - 口座の削除の確認は alertdialog「<名前>を削除しますか？この口座の記録はすべて削除されます。」、「キャンセル」「削除する」。
// テスト計画: プラン 5.1 の基準12・基準13 の PR③ の単体の行と、5.2 の PR③ の HoldingsPage.managed.test.tsx の行。
// mock は境界の外側（ネットワーク = グローバルの fetch）だけに使う。jsdom には <dialog> の showModal・close がないので、
//   ブラウザと同じ動き（open 属性・close イベント）の代用を差し込む（StockHistoryPage.test.tsx と同じ）。

const STORAGE_KEY = 'zero.holdings.totalsView'
const UNPRICED_NOTE = '価格を取得できない銘柄を含みません'
const DELETE_MESSAGE = 'WealthNaviを削除しますか？この口座の記録はすべて削除されます。'

const WEALTHNAVI = {
  name: 'WealthNavi',
  valuation: '4000000',
  principal: '3000000',
  unrealizedGain: '1000000',
  unrealizedGainRate: '33.3',
  valuationDate: '2026-11-16',
}
const ALPHA = {
  name: 'Alpha',
  valuation: '500000',
  principal: '500000',
  unrealizedGain: '0',
  unrealizedGainRate: '0.0',
  valuationDate: '2026-11-16',
}
const TOYOTA = {
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
}
const MSFT_UNPRICED = {
  code: 'MSFT',
  name: 'Microsoft',
  currency: 'USD',
  quantity: 5,
  acquisitionPrice: '400.00',
  currentPrice: null,
  priceDate: null,
  valuation: null,
  unrealizedGain: null,
  unrealizedGainRate: null,
}

/** 円の合計（口座の WealthNavi だけ。評価額 4,000,000円・含み損益 +1,000,000円）。 */
const YEN_TOTAL_ACCOUNT_ONLY = {
  currency: 'JPY',
  valuation: '4000000',
  excludesUnpriced: false,
  unrealizedGain: '1000000',
  unrealizedGainRate: '33.3',
}

/** 基準13: 売買履歴が0件で、記録が WealthNavi の1件。合計は円の部分だけ。 */
const ACCOUNT_ONLY_BODY = {
  holdings: [],
  totals: [YEN_TOTAL_ACCOUNT_ONLY],
  combinedTotal: null,
  managedAccounts: [WEALTHNAVI],
}

/**
 * 基準2: 7203（取得額 250,000円・評価額 300,000円・含み損益 +50,000円。価格あり）と WealthNavi。
 * 円の合計は、評価額 4,300,000円、含み損益 +1,050,000円、率 1,050,000 ÷（250,000 + 3,000,000）＝ 32.3…%（要件15・要件16。画面は文字列を出すだけ）。
 */
const STOCK_AND_ACCOUNT_BODY = {
  holdings: [TOYOTA],
  totals: [
    { currency: 'JPY', valuation: '4300000', excludesUnpriced: false, unrealizedGain: '1050000', unrealizedGainRate: '32.3' },
  ],
  combinedTotal: null,
  managedAccounts: [WEALTHNAVI],
}

let queryClient: QueryClient
let fetchMock: Mock<typeof fetch>
/** 一覧の取得（GET /api/holdings）が返す本文。削除に成功したら、テストが変える。 */
let listBody: unknown
/** 削除の API（DELETE /api/managed-accounts）の応答の状態。 */
let deleteStatus = 204
/** 削除に成功したときに、一覧の本文をどうするか。 */
let bodyAfterDelete: unknown

beforeAll(() => {
  HTMLDialogElement.prototype.showModal = function showModal(this: HTMLDialogElement) {
    this.setAttribute('open', '')
  }
  HTMLDialogElement.prototype.close = function close(this: HTMLDialogElement) {
    this.removeAttribute('open')
    this.dispatchEvent(new Event('close'))
  }
})

function requestedUrl(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.href
  return input.url
}

function requestedMethod(input: RequestInfo | URL, init?: RequestInit): string {
  const method = init?.method ?? (input instanceof Request ? input.method : 'GET')
  return method.toUpperCase()
}

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

/** 届いた削除の要求（DELETE）の数。 */
function deleteRequestCount(): number {
  return fetchMock.mock.calls.filter(([input, init]) => requestedMethod(input, init) === 'DELETE').length
}

beforeEach(() => {
  queryClient = new QueryClient()
  deleteStatus = 204
  listBody = ACCOUNT_ONLY_BODY
  bodyAfterDelete = undefined
  fetchMock = vi.fn<typeof fetch>(async (input, init) => {
    const url = new URL(requestedUrl(input), 'http://localhost')
    const method = requestedMethod(input, init)
    if (url.pathname === '/api/holdings' && method === 'GET') return jsonResponse(listBody)
    if (url.pathname === '/api/managed-accounts' && method === 'DELETE') {
      if (deleteStatus !== 204) {
        return new Response(JSON.stringify({ error: 'internal_error' }), { status: deleteStatus, headers: { 'Content-Type': 'application/json' } })
      }
      if (bodyAfterDelete !== undefined) listBody = bodyAfterDelete
      return new Response(null, { status: 204 })
    }
    return new Response('not found', { status: 404 })
  })
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  vi.unstubAllGlobals()
  // jsdom の localStorage は同じファイルのテストの間で残るので、必ず空にする。
  window.localStorage.clear()
})

function renderHoldingsPage() {
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <HoldingsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

/** a が b より文書の中で前にあること。 */
function expectBefore(a: HTMLElement, b: HTMLElement, message: string): void {
  expect((a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0, message).toBe(true)
}

test('基準13（要件17・要件19・要件20）: 売買履歴が0件で口座が1つの応答のとき、「保有株式はありません」はなく、合計の欄（円の部分だけ）と WealthNavi のカードがあり、「価格を今すぐ更新」と list「保有株式」はない', async () => {
  listBody = ACCOUNT_ONLY_BODY

  renderHoldingsPage()

  const summary = await screen.findByRole('region', { name: '合計' })
  expect(screen.queryByText('保有株式はありません')).not.toBeInTheDocument()
  // 合計の欄は円の部分だけ（ドルの部分はない）。含み損益と評価額が出ている。
  const yen = within(summary).getByRole('region', { name: '円' })
  expect(within(yen).getByText('+1,000,000円（+33.3%）')).toBeInTheDocument()
  expect(within(yen).getByText('4,000,000円')).toBeInTheDocument()
  expect(within(summary).queryByRole('region', { name: 'ドル' })).not.toBeInTheDocument()
  // WealthNavi のカードと、口座の「⋯」（銘柄がなくても出す。要件22）。
  const accounts = screen.getByRole('list', { name: 'おまかせ運用' })
  expect(within(accounts).getAllByRole('listitem')).toHaveLength(1)
  expect(within(accounts).getByRole('button', { name: 'WealthNaviの操作' })).toBeInTheDocument()
  // 株式がないので、更新のボタンと株式の並びは出さない（要件20）。
  expect(screen.queryByRole('button', { name: '価格を今すぐ更新' })).not.toBeInTheDocument()
  expect(screen.queryByRole('list', { name: '保有株式' })).not.toBeInTheDocument()
})

test('基準14（要件19）: 売買履歴も口座もない応答のとき、「保有株式はありません」だけが出て、合計の欄と list「おまかせ運用」と見出し「おまかせ運用」は出ない', async () => {
  listBody = { holdings: [], totals: [], combinedTotal: null, managedAccounts: [] }

  renderHoldingsPage()

  expect(await screen.findByText('保有株式はありません')).toBeInTheDocument()
  expect(screen.queryByRole('region', { name: '合計' })).not.toBeInTheDocument()
  expect(screen.queryByRole('list', { name: 'おまかせ運用' })).not.toBeInTheDocument()
  expect(screen.queryByRole('heading', { name: 'おまかせ運用' })).not.toBeInTheDocument()
  expect(screen.queryByRole('list')).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: '価格を今すぐ更新' })).not.toBeInTheDocument()
})

test('基準2（要件9）: 7203 と WealthNavi のとき、上から、合計の欄、WealthNavi のカード（list「おまかせ運用」）、「価格を今すぐ更新」、7203 のカード（list「保有株式」）の順に並ぶ', async () => {
  listBody = STOCK_AND_ACCOUNT_BODY

  renderHoldingsPage()

  const summary = await screen.findByRole('region', { name: '合計' })
  const accounts = screen.getByRole('list', { name: 'おまかせ運用' })
  const refresh = screen.getByRole('button', { name: '価格を今すぐ更新' })
  const stocks = screen.getByRole('list', { name: '保有株式' })
  expectBefore(summary, accounts, '合計の欄の後に口座のカード')
  expectBefore(accounts, refresh, '口座のカードの後に更新のボタン')
  expectBefore(refresh, stocks, '更新のボタンの後に株式のカード')
  // 口座のカードは、株式の並びの中に入らない（別の並び）。
  expect(within(accounts).getAllByRole('listitem')).toHaveLength(1)
  expect(within(stocks).getAllByRole('listitem')).toHaveLength(1)
  expect(within(stocks).queryByText('WealthNavi')).not.toBeInTheDocument()
  expect(within(accounts).queryByText('7203')).not.toBeInTheDocument()
})

test('要件6・要件9: 口座が2つのとき、list「おまかせ運用」のカードは、応答の順（Alpha、WealthNavi）に並ぶ', async () => {
  listBody = { ...STOCK_AND_ACCOUNT_BODY, managedAccounts: [ALPHA, WEALTHNAVI] }

  renderHoldingsPage()

  const accounts = await screen.findByRole('list', { name: 'おまかせ運用' })
  const buttons = within(accounts).getAllByRole('button', { name: /の操作$/ })
  expect(buttons.map((button) => button.getAttribute('aria-label'))).toEqual(['Alphaの操作', 'WealthNaviの操作'])
})

test('補助（要件19・要件20）: 口座がなく、銘柄だけの応答のとき、今のとおり合計の欄・「価格を今すぐ更新」・株式のカードが出て、list「おまかせ運用」は出ない', async () => {
  listBody = {
    holdings: [TOYOTA],
    totals: [{ currency: 'JPY', valuation: '300000', excludesUnpriced: false, unrealizedGain: '50000', unrealizedGainRate: '20.0' }],
    combinedTotal: null,
    managedAccounts: [],
  }

  renderHoldingsPage()

  await screen.findByRole('list', { name: '保有株式' })
  expect(screen.getByRole('region', { name: '合計' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: '価格を今すぐ更新' })).toBeInTheDocument()
  expect(screen.queryByRole('list', { name: 'おまかせ運用' })).not.toBeInTheDocument()
  expect(screen.queryByRole('heading', { name: 'おまかせ運用' })).not.toBeInTheDocument()
})

// 基準12 の単体: MSFT（価格なし）と WealthNavi、レート 150.00円。円にまとめた値は「+1,000,000円（+33.3%）」「4,000,000円」と注記1つで、
// 含み損益は「取得できません」にならない（要件18）。期待値は要件定義の基準12 から（口座の損益 1,000,000 ÷ 元本 3,000,000 ＝ 33.3%）。
test('基準12（要件18）: localStorage に combinedYen を入れ、MSFT（価格なし）と WealthNavi の応答を描くと、合計の欄が「+1,000,000円（+33.3%）」「4,000,000円」と注記1つで、「取得できません」がない', async () => {
  listBody = {
    holdings: [MSFT_UNPRICED],
    totals: [
      YEN_TOTAL_ACCOUNT_ONLY,
      { currency: 'USD', valuation: '0.00', excludesUnpriced: true, unrealizedGain: null, unrealizedGainRate: null },
    ],
    combinedTotal: {
      valuation: '4000000',
      excludesUnpriced: true,
      unrealizedGain: '1000000',
      unrealizedGainRate: '33.3',
      usdJpyRate: '150.00',
      usdJpyRateDate: '2026-11-14',
    },
    managedAccounts: [WEALTHNAVI],
  }
  window.localStorage.setItem(STORAGE_KEY, 'combinedYen')

  renderHoldingsPage()

  const summary = await screen.findByRole('region', { name: '合計' })
  expect(summary.querySelector('[data-tone]')?.textContent?.trim()).toBe('+1,000,000円（+33.3%）')
  expect(within(summary).getByText('4,000,000円')).toBeInTheDocument()
  expect(within(summary).getByText('1ドル = 150.00円（2026-11-14 のレート）で換算')).toBeInTheDocument()
  expect(within(summary).getAllByText(UNPRICED_NOTE)).toHaveLength(1)
  expect(screen.getAllByText(UNPRICED_NOTE)).toHaveLength(1)
  expect(within(summary).queryByText('取得できません')).not.toBeInTheDocument()
})

// ---- 口座の削除（要件22〜要件24）。閉じた後のフォーカスは株式の削除と同じ（キャンセルなら「⋯」、削除したなら見出し）。 ----

test('要件23（株式の削除と同じ）: WealthNavi の「⋯」から「削除」を選ぶと確認のダイアログが開き、「キャンセル」を押すと、削除の要求は送られず、フォーカスが「WealthNaviの操作」の「⋯」に戻り、カードは残る', async () => {
  listBody = STOCK_AND_ACCOUNT_BODY

  renderHoldingsPage()

  await screen.findByRole('list', { name: 'おまかせ運用' })
  fireEvent.click(screen.getByRole('button', { name: 'WealthNaviの操作' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: '削除' }))
  expect(await screen.findByRole('alertdialog', { name: DELETE_MESSAGE })).toBeInTheDocument()

  fireEvent.click(screen.getByRole('button', { name: 'キャンセル' }))

  await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
  expect(deleteRequestCount()).toBe(0)
  const opener = screen.getByRole('button', { name: 'WealthNaviの操作' })
  await waitFor(() => expect(opener).toHaveFocus())
  expect(within(screen.getByRole('list', { name: 'おまかせ運用' })).getAllByRole('listitem')).toHaveLength(1)
})

test('要件24（株式の削除と同じ）: 「削除する」で削除が成功して口座が消えると、ダイアログが閉じ、list「おまかせ運用」がなくなり、フォーカスが見出し「保有株式一覧」に移る。株式のカードは残る', async () => {
  listBody = STOCK_AND_ACCOUNT_BODY
  // 削除の後の一覧（WealthNavi がなくなり、円の合計は 7203 だけ）。
  bodyAfterDelete = {
    holdings: [TOYOTA],
    totals: [{ currency: 'JPY', valuation: '300000', excludesUnpriced: false, unrealizedGain: '50000', unrealizedGainRate: '20.0' }],
    combinedTotal: null,
    managedAccounts: [],
  }

  renderHoldingsPage()

  await screen.findByRole('list', { name: 'おまかせ運用' })
  fireEvent.click(screen.getByRole('button', { name: 'WealthNaviの操作' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: '削除' }))
  await screen.findByRole('alertdialog', { name: DELETE_MESSAGE })

  fireEvent.click(screen.getByRole('button', { name: '削除する' }))

  await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
  expect(deleteRequestCount()).toBe(1)
  await waitFor(() => expect(screen.queryByRole('list', { name: 'おまかせ運用' })).not.toBeInTheDocument())
  expect(screen.getByRole('heading', { level: 1, name: '保有株式一覧' })).toHaveFocus()
  expect(screen.getByRole('list', { name: '保有株式' })).toBeInTheDocument()
  const summary = screen.getByRole('region', { name: '合計' })
  expect(within(summary).getByText('+50,000円（+20.0%）')).toBeInTheDocument()
  expect(within(summary).getByText('300,000円')).toBeInTheDocument()
})

test('要件19・要件24: 唯一の口座（銘柄なし）を削除して一覧が空になると、「保有株式はありません」が出て、合計の欄は出ず、フォーカスは見出しに移る', async () => {
  listBody = ACCOUNT_ONLY_BODY
  bodyAfterDelete = { holdings: [], totals: [], combinedTotal: null, managedAccounts: [] }

  renderHoldingsPage()

  await screen.findByRole('list', { name: 'おまかせ運用' })
  fireEvent.click(screen.getByRole('button', { name: 'WealthNaviの操作' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: '削除' }))
  await screen.findByRole('alertdialog', { name: DELETE_MESSAGE })
  fireEvent.click(screen.getByRole('button', { name: '削除する' }))

  expect(await screen.findByText('保有株式はありません')).toBeInTheDocument()
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  expect(screen.queryByRole('region', { name: '合計' })).not.toBeInTheDocument()
  expect(screen.queryByRole('list', { name: 'おまかせ運用' })).not.toBeInTheDocument()
  expect(screen.getByRole('heading', { level: 1, name: '保有株式一覧' })).toHaveFocus()
})

test('基準18（要件24）: 削除が HTTP 500 で失敗すると、ダイアログは開いたまま「削除できませんでした」が出て、WealthNavi のカードは残り、一覧は変わらない', async () => {
  listBody = STOCK_AND_ACCOUNT_BODY
  deleteStatus = 500

  renderHoldingsPage()

  await screen.findByRole('list', { name: 'おまかせ運用' })
  fireEvent.click(screen.getByRole('button', { name: 'WealthNaviの操作' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: '削除' }))
  await screen.findByRole('alertdialog', { name: DELETE_MESSAGE })
  fireEvent.click(screen.getByRole('button', { name: '削除する' }))

  expect(await screen.findByRole('alert')).toHaveTextContent('削除できませんでした')
  expect(screen.getByRole('alertdialog', { name: DELETE_MESSAGE })).toBeInTheDocument()
  expect(within(screen.getByRole('list', { name: 'おまかせ運用' })).getByRole('button', { name: 'WealthNaviの操作' })).toBeInTheDocument()
})

test('要件23: 口座が2つのとき、確認のダイアログは「⋯」を押した方の口座（Alpha）の名前になる', async () => {
  listBody = { ...STOCK_AND_ACCOUNT_BODY, managedAccounts: [ALPHA, WEALTHNAVI] }

  renderHoldingsPage()

  await screen.findByRole('list', { name: 'おまかせ運用' })
  fireEvent.click(screen.getByRole('button', { name: 'Alphaの操作' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: '削除' }))

  expect(await screen.findByRole('alertdialog', { name: 'Alphaを削除しますか？この口座の記録はすべて削除されます。' })).toBeInTheDocument()
})
