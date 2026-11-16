// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi, type Mock } from 'vitest'
import { HoldingsPage } from '../../../src/features/holdings'

// 仕様: 要件 docs/requirements/stock-holdings.md の FR-21、AC-9a。
// 境界: プラン docs/plans/stock-holdings/plan.md 4.4「フロント（TypeScript）」
//   - HoldingsPage は features/holdings/index.ts から公開され、QueryClientProvider を持たず（app 層で包む）、Router に依存しない。
//   - fetchHoldings はグローバルの fetch で "/api/holdings" を呼ぶ。
//   - <Spinner label="読み込み中" /> は role="status" で、aria-label がラベル。
//   - 「画面の状態ごとの表示」の表の「読み込み中」の行: Spinner だけを表示し、
//     見出し・合計の領域（<section aria-label="合計評価額">）・表・空のメッセージ・ダイアログは表示しない。
// テスト計画: プラン 5.2 の AC-9a の行と、補助（FR-21、PR④ の TC-01）の行。
// mock は境界の外側（ネットワーク = グローバルの fetch）だけに使う。

let queryClient: QueryClient
let fetchMock: Mock<typeof fetch>

beforeEach(() => {
  // テストごとに新しい QueryClient で包む（前のテストのキャッシュを持ち込まない）。
  queryClient = new QueryClient()
  // 応答を返さない（終わらない）fetch に差し替えて、「一覧のデータの取得が完了していない」状態を作る。
  fetchMock = vi.fn<typeof fetch>(() => new Promise<Response>(() => {}))
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

test('AC-9a・補助 FR-21（PR4_TC01）: 取得が完了していない間は「読み込み中」のラベルが付いたローディングのアイコンだけが表示され（描画結果の文字は空）、一覧と合計評価額は表示されない', async () => {
  const { container } = renderHoldingsPage()

  // Given の確認: 一覧のデータの取得を始めていて、まだ終わっていない（fetch は応答を返さない）。
  await waitFor(() => expect(fetchMock).toHaveBeenCalled())

  // 「読み込み中」のラベルが付いたローディングのアイコンが表示される（FR-21）。
  expect(screen.getByRole('status', { name: '読み込み中' })).toBeInTheDocument()

  // アイコン「だけ」: 見出し・表・合計の領域・空のメッセージ・ダイアログは表示しない（プラン 4.4 の状態の表）。
  expect(screen.queryAllByRole('heading')).toHaveLength(0)
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('region', { name: '合計評価額' }),
  ).not.toBeInTheDocument()
  expect(screen.queryByText('合計評価額')).not.toBeInTheDocument()
  expect(screen.queryByText('保有株式はありません')).not.toBeInTheDocument()
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()

  // 補助 FR-21（PR④ の TC-01）: アイコン「だけ」を、決められた要素の否定に限らず確かめる。
  // ラベルは aria-label（プラン 4.4 の Spinner）なので、描画結果に文字は1つもない。
  expect(container.textContent).toBe('')
})

// ---- 通貨ごとの表示（売買履歴の要件 docs/requirements/trade-history.md の要件12〜14。プラン docs/plans/trade-history/plan.md 4.6・4.7 の PR②） ----
// 一覧の API の応答（行に currency、金額は文字列、合計は通貨ごとの totals）を表示する画面の単体。
// 基準9・基準10・基準11 の E2E（holdings.spec.ts）と同じ値を、HoldingsPage を描画して確かめる（円の期待値は変えない）。

/** API の応答に見せかけて fetch が返す一覧。7203（円）と AAPL（ドル）の2行（どちらも価格あり）と、円・ドルの順の合計。 */
const MIXED_BODY = {
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
    },
  ],
  totals: [
    { currency: 'JPY', valuation: '300000', excludesUnpriced: false },
    { currency: 'USD', valuation: '1800.00', excludesUnpriced: false },
  ],
}

function respondWithHoldings(body: unknown): void {
  fetchMock.mockImplementation(
    async () =>
      new Response(JSON.stringify(body), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
  )
}

/** 銘柄コードのセルが code と一致するデータ行の、先頭の7つのセルの文字。 */
function rowCellTexts(code: string): string[] {
  const row = screen
    .getAllByRole('row')
    .find((r) => within(r).queryByRole('cell', { name: code }) !== null)
  if (row === undefined) throw new Error(`銘柄 ${code} の行がない`)
  return within(row)
    .getAllByRole('cell')
    .slice(0, 7)
    .map((cell) => cell.textContent ?? '')
}

test('基準9（要件14）: ドルの銘柄の行は「AAPL / Apple / 10株 / 150.25ドル / 180.00ドル / 2026-10-06 / 1,800.00ドル」、円の銘柄の行は「2,500円 / 3,000円 / 300,000円」で表示される', async () => {
  respondWithHoldings(MIXED_BODY)

  renderHoldingsPage()

  await screen.findByRole('table')
  expect(rowCellTexts('AAPL')).toEqual([
    'AAPL',
    'Apple',
    '10株',
    '150.25ドル',
    '180.00ドル',
    '2026-10-06',
    '1,800.00ドル',
  ])
  expect(rowCellTexts('7203')).toEqual([
    '7203',
    'トヨタ自動車',
    '100株',
    '2,500円',
    '3,000円',
    '2026-10-06',
    '300,000円',
  ])
})

test('基準10: 合計評価額の領域に「300,000円」と「1,800.00ドル」がこの順に表示され、「価格を取得できない銘柄を含みません」は表示されない', async () => {
  respondWithHoldings(MIXED_BODY)

  renderHoldingsPage()

  const region = await screen.findByRole('region', { name: '合計評価額' })
  const rows = within(region).getAllByRole('listitem')
  expect(rows).toHaveLength(2)
  expect(rows[0]).toHaveTextContent('300,000円')
  expect(rows[1]).toHaveTextContent('1,800.00ドル')
  expect(screen.queryByText('価格を取得できない銘柄を含みません')).not.toBeInTheDocument()
})

test('基準7（要件10）: holdings と totals が空の応答のとき、「保有株式はありません」が表示され、合計評価額の領域・表は表示されない', async () => {
  respondWithHoldings({ holdings: [], totals: [] })

  renderHoldingsPage()

  expect(await screen.findByText('保有株式はありません')).toBeInTheDocument()
  expect(screen.queryByRole('region', { name: '合計評価額' })).not.toBeInTheDocument()
  expect(screen.queryByText('合計評価額')).not.toBeInTheDocument()
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
})

test('補助（プラン 4.7）: holdings が1件以上で totals が空の応答のとき、表は表示されるが、合計評価額の領域は表示されない', async () => {
  respondWithHoldings({ holdings: MIXED_BODY.holdings, totals: [] })

  renderHoldingsPage()

  // 取得が終わって表が出てから、合計の領域がないことを確かめる。
  await screen.findByRole('table')
  expect(rowCellTexts('7203')).toEqual([
    '7203',
    'トヨタ自動車',
    '100株',
    '2,500円',
    '3,000円',
    '2026-10-06',
    '300,000円',
  ])
  expect(screen.queryByRole('region', { name: '合計評価額' })).not.toBeInTheDocument()
  expect(screen.queryByText('合計評価額')).not.toBeInTheDocument()
})
