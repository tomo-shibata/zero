// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeAll, beforeEach, describe, expect, test, vi, type Mock } from 'vitest'
import { StockHistoryPage } from '../../../src/features/stock-history'
import { baseline7 } from './fixtures'

// 仕様: 要件 docs/requirements/stock-history.md の要件4（戻るリンクと見出し）・要件19・要件20・要件22・要件23、基準25・基準26（単体）。
// 境界: プラン docs/plans/stock-history/plan.md 4.6「画面」と 4.8「PR② 画面とカードからの移動」
//   - StockHistoryPage は features/stock-history/index.ts から公開され、useParams の code を使う。QueryClientProvider と Router は持たない（app 層で包む）。
//   - 左上に「← 保有株式一覧」のリンク（/holdings）。その下に「<銘柄コード> <銘柄名>」の見出し。
//   - 読み込み中（要件22）: 「← 保有株式一覧」のリンクと、「読み込み中」のラベルを付けたローディングのアイコン（Spinner。role="status"）だけ。
//   - 取得失敗（要件23。通信エラーでも、サーバーのエラーでも）: 「売買履歴の取得に失敗しました」のダイアログ（AlertDialog。role="alertdialog"）。
//     「閉じる」で閉じた後は「← 保有株式一覧」のリンクだけ。
//   - 売買履歴なし（404。要件20）: 「この銘柄の売買履歴はありません」と「← 保有株式一覧」のリンクだけ（見出しの代わり。チャートも表も出さない）。
//   - 取得は GET /api/holdings/{code}/history（fetchStockHistory）。
// テスト計画: プラン 5.1 の基準25・基準26 と、5.2 の StockHistoryPage.test.tsx の行（fetch が reject する通信エラーも同じダイアログ）。
// mock は境界の外側（ネットワーク = グローバルの fetch）だけに使う。
// jsdom には <dialog> の showModal・close がないので、ダイアログの開閉だけ、ブラウザと同じ動き（open 属性・close イベント）の代用を差し込む。

let queryClient: QueryClient
let fetchMock: Mock<typeof fetch>

beforeAll(() => {
  // jsdom の HTMLDialogElement は showModal・close を持たない。AlertDialog（useOpenAsModal）が呼ぶため、open 属性と close イベントだけの代用を足す。
  HTMLDialogElement.prototype.showModal = function showModal(this: HTMLDialogElement) {
    this.setAttribute('open', '')
  }
  HTMLDialogElement.prototype.close = function close(this: HTMLDialogElement) {
    this.removeAttribute('open')
    this.dispatchEvent(new Event('close'))
  }
})

beforeEach(() => {
  queryClient = new QueryClient()
  // 応答を返さない（終わらない）fetch。「データの取得が完了していない」状態を作る。
  fetchMock = vi.fn<typeof fetch>(() => new Promise<Response>(() => {}))
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  vi.unstubAllGlobals()
})

function renderPage(path = '/holdings/7203') {
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/holdings/:code" element={<StockHistoryPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function respondWith(status: number, body: unknown): void {
  fetchMock.mockImplementation(
    async () => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }),
  )
}

function requestedUrl(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.href
  return input.url
}

const BACK_LINK = '← 保有株式一覧'
const FETCH_ERROR = '売買履歴の取得に失敗しました'

/** 「← 保有株式一覧」のリンク（/holdings に移る。要件4）。 */
function backLink(): HTMLElement {
  return screen.getByRole('link', { name: BACK_LINK })
}

/** チャートも表も見出しも、ダイアログも出していないこと（リンクだけ）。 */
function expectOnlyBackLink(container: HTMLElement): void {
  expect(backLink()).toHaveAttribute('href', '/holdings')
  expect(screen.queryByRole('img')).not.toBeInTheDocument()
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
  expect(screen.queryAllByRole('heading')).toHaveLength(0)
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  expect(screen.queryByRole('status')).not.toBeInTheDocument()
  expect(container.textContent).toBe(BACK_LINK)
}

describe('読み込み中（基準25。要件22）', () => {
  test('基準25: 取得が完了していない間は、「← 保有株式一覧」と「読み込み中」のラベルが付いたローディングのアイコンだけが出る（文字は戻るリンクだけ）', async () => {
    const { container } = renderPage()
    // Given の確認: 取得を始めていて、まだ終わっていない。
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())

    expect(backLink()).toHaveAttribute('href', '/holdings')
    expect(screen.getByRole('status', { name: '読み込み中' })).toBeInTheDocument()
    // 「だけ」: 見出し・チャート・表・ダイアログ・売買履歴なしの文は出ない。
    expect(screen.queryAllByRole('heading')).toHaveLength(0)
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    expect(screen.queryByText('この銘柄の売買履歴はありません')).not.toBeInTheDocument()
    expect(container.textContent).toBe(BACK_LINK)
  })
})

describe('取得の失敗（基準26。要件23）', () => {
  test('基準26: サーバーが 500 を返すと「売買履歴の取得に失敗しました」のダイアログが出る。「閉じる」を押すと、「← 保有株式一覧」だけが出る', async () => {
    respondWith(500, { error: 'internal_error' })
    const { container } = renderPage()

    const dialog = await screen.findByRole('alertdialog', { name: FETCH_ERROR })
    expect(dialog).toHaveTextContent(FETCH_ERROR)
    expect(backLink()).toBeInTheDocument()

    fireEvent.click(within(dialog).getByRole('button', { name: '閉じる' }))

    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
    expectOnlyBackLink(container)
  })

  test('補助 要件23（5.2。通信エラー）: fetch が reject する（通信エラー）と、500 と同じダイアログが出て、「閉じる」の後は「← 保有株式一覧」だけが出る', async () => {
    fetchMock.mockImplementation(() => Promise.reject(new TypeError('Failed to fetch')))
    const { container } = renderPage()

    const dialog = await screen.findByRole('alertdialog', { name: FETCH_ERROR })
    fireEvent.click(within(dialog).getByRole('button', { name: '閉じる' }))

    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
    expectOnlyBackLink(container)
  })

  test('補助 要件23（形の誤り）: 応答の形が誤っている（200 だが closes が null）ときも、取得に失敗したダイアログが出る（売買履歴なしの表示にはならない）', async () => {
    respondWith(200, { ...baseline7(), closes: null })
    renderPage()

    await screen.findByRole('alertdialog', { name: FETCH_ERROR })
    expect(screen.queryByText('この銘柄の売買履歴はありません')).not.toBeInTheDocument()
  })
})

describe('売買履歴なし（要件20）', () => {
  test('補助 要件20: 404 のとき、「この銘柄の売買履歴はありません」と「← 保有株式一覧」だけが出て、チャート・表・ダイアログは出ない', async () => {
    respondWith(404, { error: 'not_found' })
    const { container } = renderPage()

    expect(await screen.findByText('この銘柄の売買履歴はありません')).toBeInTheDocument()
    expect(backLink()).toHaveAttribute('href', '/holdings')
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(container.textContent).toBe(`${BACK_LINK}この銘柄の売買履歴はありません`)
  })
})

describe('取得できたとき（要件4・要件19）', () => {
  test('補助 要件4: URL の銘柄コードで取得し（/holdings/6758 なら GET /api/holdings/6758/history）、「← 保有株式一覧」・見出し「7203 トヨタ自動車」・チャート・表を出す', async () => {
    respondWith(200, baseline7())
    renderPage('/holdings/7203')

    expect(await screen.findByRole('heading', { name: '7203 トヨタ自動車' })).toBeInTheDocument()
    expect(fetchMock.mock.calls.map(([input]) => requestedUrl(input))).toEqual(['/api/holdings/7203/history'])
    expect(backLink()).toHaveAttribute('href', '/holdings')
    expect(screen.getByRole('img', { name: '7203 トヨタ自動車の価格のチャート' })).toBeInTheDocument()
    expect(screen.getByRole('table')).toBeInTheDocument()
    // 見出しの行のほかに、売買2件の行。
    expect(screen.getAllByRole('row')).toHaveLength(3)
    expect(screen.queryByRole('status', { name: '読み込み中' })).not.toBeInTheDocument()
  })

  test('補助 要件4: URL が /holdings/6758 なら 6758 の履歴を取得する（useParams の code を使う）', async () => {
    respondWith(404, { error: 'not_found' })
    renderPage('/holdings/6758')

    await screen.findByText('この銘柄の売買履歴はありません')
    expect(fetchMock.mock.calls.map(([input]) => requestedUrl(input))).toEqual(['/api/holdings/6758/history'])
  })

  test('補助 要件19（エッジケースの網羅）: URL が /holdings/A%2FB（銘柄コード A/B）なら、GET /api/holdings/A%2FB/history を呼ぶ（スラッシュをエスケープしたまま）', async () => {
    respondWith(404, { error: 'not_found' })
    renderPage('/holdings/A%2FB')

    await screen.findByText('この銘柄の売買履歴はありません')
    expect(fetchMock.mock.calls.map(([input]) => requestedUrl(input))).toEqual(['/api/holdings/A%2FB/history'])
  })
})
