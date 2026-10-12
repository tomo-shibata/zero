// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi, type Mock } from 'vitest'
import { HoldingsPage } from '../../../src/features/holdings'

// 仕様: 要件 docs/requirements/price-update.md の要件17、基準29。
//   要件17: 更新している間は、ボタンを押せなくし、ボタンの文言を「更新中…」にする。
//   基準29: 今すぐ更新の要求が完了していないとき、画面を表示すると、ボタンは押せず、文言は「更新中…」になっている。
// 境界: プラン docs/plans/price-update/plan.md 4.4「画面」の今すぐ更新（PR①）
//   - 合計の領域の直後に <section aria-label="価格の更新">。中に「価格を今すぐ更新」のボタン。
//   - 実行中は disabled で文言「更新中…」。
//   - 今すぐ更新の API は POST /api/price-refresh。一覧の API は GET /api/holdings。
//   - PriceRefreshPanel は HoldingsPage の中に置かれるので、HoldingsPage を描画して確かめる（プラン 5.1 の基準29 の行）。
// テスト計画: プラン 5.1 の基準29 の行「HoldingsPage を描画し、POST /api/price-refresh を終わらない fetch にする」。
// mock は境界の外側（ネットワーク = グローバルの fetch）だけに使う。

/** 一覧の API の応答（7203 の1行。保有が1件以上のときだけ、今すぐ更新のボタンが出る。要件1）。 */
const HOLDINGS_BODY = {
  holdings: [
    {
      code: '7203',
      name: 'トヨタ自動車',
      quantity: 100,
      acquisitionPrice: '2500.0',
      currentPrice: '3000.0',
      priceDate: '2026-10-06',
      valuation: 300000,
    },
  ],
  totalValuation: 300000,
  totalExcludesUnpriced: false,
}

const REFRESH_BUTTON = '価格を今すぐ更新'
const REFRESHING_BUTTON = '更新中…'

let queryClient: QueryClient
let fetchMock: Mock<typeof fetch>

/** fetch に渡された input から URL の文字列を取り出す。 */
function requestedUrl(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.href
  return input.url
}

/** fetch に渡された input と init から HTTP メソッドを取り出す（既定は GET）。 */
function requestedMethod(input: RequestInfo | URL, init?: RequestInit): string {
  const method = init?.method ?? (input instanceof Request ? input.method : 'GET')
  return method.toUpperCase()
}

/** POST /api/price-refresh の要求が fetch に渡されたか。 */
function refreshRequested(): boolean {
  return fetchMock.mock.calls.some(
    ([input, init]) =>
      requestedUrl(input) === '/api/price-refresh' && requestedMethod(input, init) === 'POST',
  )
}

beforeEach(() => {
  // テストごとに新しい QueryClient で包む（前のテストのキャッシュを持ち込まない）。
  queryClient = new QueryClient()
  // 一覧の API（GET /api/holdings）には 7203 の1行を返し、今すぐ更新の API（POST /api/price-refresh）には応答を返さない（終わらない）。
  fetchMock = vi.fn<typeof fetch>((input, init) => {
    if (requestedUrl(input) === '/api/holdings' && requestedMethod(input, init) === 'GET') {
      return Promise.resolve(
        new Response(JSON.stringify(HOLDINGS_BODY), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    }
    return new Promise<Response>(() => {})
  })
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  vi.unstubAllGlobals()
})

function renderHoldingsPage() {
  return render(
    <QueryClientProvider client={queryClient}>
      <HoldingsPage />
    </QueryClientProvider>,
  )
}

test('基準29（要件17）: 今すぐ更新の要求が完了していない間は、ボタンは押せず、文言は「更新中…」になっている', async () => {
  renderHoldingsPage()

  // 一覧が出て、今すぐ更新のボタンが押せる状態で出ている（Given の前の状態）。
  const region = await screen.findByRole('region', { name: '価格の更新' })
  const button = screen.getByRole('button', { name: REFRESH_BUTTON })
  expect(region).toContainElement(button)
  expect(button).toBeEnabled()

  fireEvent.click(button)

  // Given の確認: 今すぐ更新の要求（POST /api/price-refresh）を送っていて、まだ完了していない（応答を返さない）。
  await waitFor(() => expect(refreshRequested()).toBe(true))

  // ボタンは押せず、文言は「更新中…」。元の文言「価格を今すぐ更新」のボタンはない。
  const refreshing = await screen.findByRole('button', { name: REFRESHING_BUTTON })
  expect(refreshing).toBeDisabled()
  expect(refreshing).toHaveTextContent(REFRESHING_BUTTON)
  expect(screen.queryByRole('button', { name: REFRESH_BUTTON })).not.toBeInTheDocument()
  // 同じ領域の中にある（領域が入れ替わっていない）。
  expect(screen.getByRole('region', { name: '価格の更新' })).toContainElement(refreshing)
})

/**
 * 一覧の API（GET /api/holdings）には 7203 の1行を返し、今すぐ更新の API（POST /api/price-refresh）には、
 * 押した順に replies の応答を返す（replies を使い切った後の要求は、応答を返さない＝終わらない）。
 */
function stubRefreshReplies(replies: (() => Response)[]): void {
  let postCount = 0
  fetchMock.mockImplementation((input, init) => {
    if (requestedUrl(input) === '/api/holdings' && requestedMethod(input, init) === 'GET') {
      return Promise.resolve(
        new Response(JSON.stringify(HOLDINGS_BODY), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    }
    const reply = replies[postCount]
    postCount += 1
    return reply === undefined ? new Promise<Response>(() => {}) : Promise.resolve(reply())
  })
}

/** 今すぐ更新の領域（<section aria-label="価格の更新">）の中の、結果の文（role="status" の中の <p>）の数。 */
function resultMessageCount(): number {
  const status = within(screen.getByRole('region', { name: '価格の更新' })).getByRole('status')
  return status.querySelectorAll('p').length
}

test('補助・判断19・要件19: 更新の要求が失敗して「価格の更新に失敗しました」が出た後に押し直すと、その失敗の表示は消えて「更新中…」になる', async () => {
  stubRefreshReplies([
    () => new Response(JSON.stringify({ error: 'internal_error' }), { status: 500 }),
  ])
  renderHoldingsPage()
  fireEvent.click(await screen.findByRole('button', { name: REFRESH_BUTTON }))
  // 1回目: 要求が失敗し、失敗の文言が出る。ボタンは押せる状態に戻る。
  expect(await screen.findByRole('alert')).toHaveTextContent('価格の更新に失敗しました')
  fireEvent.click(await screen.findByRole('button', { name: REFRESH_BUTTON }))

  // 2回目は終わらない: 「更新中…」になり、前の失敗の文言は消えている。結果の文も出ていない。
  expect(await screen.findByRole('button', { name: REFRESHING_BUTTON })).toBeDisabled()
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(resultMessageCount()).toBe(0)
})

test('補助・判断19・要件18: 更新が成功して「1銘柄の価格を更新しました」が出た後に押し直すと、前の結果の文は消えて「更新中…」になる', async () => {
  stubRefreshReplies([
    () =>
      new Response(JSON.stringify({ saved: ['7203'], failed: [] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
  ])
  renderHoldingsPage()
  fireEvent.click(await screen.findByRole('button', { name: REFRESH_BUTTON }))
  // 1回目: 成功し、結果の文が出る（一覧の取り直しが終わった後）。ボタンは押せる状態に戻る。
  expect(await screen.findByText('1銘柄の価格を更新しました')).toBeInTheDocument()
  fireEvent.click(await screen.findByRole('button', { name: REFRESH_BUTTON }))

  // 2回目は終わらない: 「更新中…」になり、前の結果の文は消えている。失敗の文言も出ていない。
  expect(await screen.findByRole('button', { name: REFRESHING_BUTTON })).toBeDisabled()
  expect(screen.queryByText('1銘柄の価格を更新しました')).not.toBeInTheDocument()
  expect(resultMessageCount()).toBe(0)
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})
