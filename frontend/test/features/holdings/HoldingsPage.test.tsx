// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { afterEach, beforeEach, expect, test, vi, type Mock } from 'vitest'
import { HoldingsPage } from '../../../src/features/holdings'

// 仕様: 要件 docs/requirements/stock-holdings.md の FR-21、AC-9a。
//   カード表示の要件 docs/requirements/holdings-cards.md の要件1・要件2・要件11（基準1・基準2・基準14・基準17・基準18 の単体の部分）。
//   「表」「行」はカード、「合計評価額」は合計の欄の評価額に読み替える（holdings-cards.md の「既存の要件の置き換え」。確かめる値は変えない）。
// 境界: プラン docs/plans/stock-holdings/plan.md 4.4「フロント（TypeScript）」
//   - HoldingsPage は features/holdings/index.ts から公開され、QueryClientProvider を持たず（app 層で包む）、Router に依存しない。
//   - fetchHoldings はグローバルの fetch で "/api/holdings" を呼ぶ。
//   - <Spinner label="読み込み中" /> は role="status" で、aria-label がラベル。
//   - 「画面の状態ごとの表示」の表の「読み込み中」の行: Spinner だけを表示し、
//     見出し・合計の欄（region「合計」）・カードの並び（<ul aria-label="保有株式">）・空のメッセージ・ダイアログは表示しない。
//   カード表示のプラン docs/plans/holdings-cards/plan.md 4.4（PR①）: 取得できたら HoldingsSummary（region「合計」。totals が空なら出さない）、
//   PriceRefreshPanel、HoldingCardList（<ul aria-label="保有株式"> の <li>）の順。0件は「保有株式はありません」だけ。
// テスト計画: プラン stock-holdings 5.2 の AC-9a の行と、補助（FR-21、PR④ の TC-01）の行。カード表示の 5.2（HoldingsPage.test.tsx の「表」「行」をカードに）。
// 銘柄ごとの売買履歴の画面（要件 docs/requirements/stock-history.md の要件2、基準3。プラン docs/plans/stock-history/plan.md 5.1・5.3）:
//   HoldingsPage が useNavigate を使うので、MemoryRouter で包んで描画する（Router に依存する）。
//   カードで Enter を押すと、場所が /holdings/<銘柄コード>（stockHistoryPath）になる（Enter から画面が移るまでを通して確かめる）。
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
      <MemoryRouter>
        <HoldingsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

/** 今の場所（パス）を文字で出す画面（移った先の確認用）。 */
function LocationProbe() {
  return <p data-testid="location">{useLocation().pathname}</p>
}

/** /holdings に HoldingsPage、/holdings/:code に場所を出すだけの画面を置いて描画する（カードを開くと移ることを確かめる）。 */
function renderHoldingsPageWithStockRoute() {
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/holdings']}>
        <Routes>
          <Route path="/holdings" element={<HoldingsPage />} />
          <Route path="/holdings/:code" element={<LocationProbe />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

test('AC-9a・補助 FR-21（PR4_TC01）: 取得が完了していない間は「読み込み中」のラベルが付いたローディングのアイコンだけが表示され（描画結果の文字は空）、一覧と合計評価額は表示されない', async () => {
  const { container } = renderHoldingsPage()

  // Given の確認: 一覧のデータの取得を始めていて、まだ終わっていない（fetch は応答を返さない）。
  await waitFor(() => expect(fetchMock).toHaveBeenCalled())

  // 「読み込み中」のラベルが付いたローディングのアイコンが表示される（FR-21）。
  expect(screen.getByRole('status', { name: '読み込み中' })).toBeInTheDocument()

  // アイコン「だけ」: 見出し・カードの並び・合計の欄・空のメッセージ・ダイアログは表示しない（プラン 4.4 の状態の表）。
  expect(screen.queryAllByRole('heading')).toHaveLength(0)
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
  expect(screen.queryByRole('list', { name: '保有株式' })).not.toBeInTheDocument()
  expect(screen.queryByRole('region', { name: '合計' })).not.toBeInTheDocument()
  expect(screen.queryByText('合計')).not.toBeInTheDocument()
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
  combinedTotal: null,
  managedAccounts: [],
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

/** カードの並び（<ul aria-label="保有株式">）の中の、銘柄コードが code のカード（<li>）。 */
function cardOf(code: string): HTMLElement {
  const card = within(screen.getByRole('list', { name: '保有株式' }))
    .getAllByRole('listitem')
    .find((item) => within(item).queryByText(code) !== null)
  if (card === undefined) throw new Error(`銘柄 ${code} のカードがない`)
  return card
}

/** element の文字に、texts が（空白を問わず）この順で含まれていること。 */
function expectTextsInOrder(element: HTMLElement, texts: readonly string[]): void {
  const content = element.textContent ?? ''
  let from = 0
  for (const text of texts) {
    const index = content.indexOf(text, from)
    expect(index, `「${text}」が、前の項目より後に含まれる（${content}）`).toBeGreaterThanOrEqual(0)
    from = index + text.length
  }
}

test('売買履歴の基準9・カード表示の基準1・基準2（要件2・要件14）: 円の銘柄のカードは「7203 / トヨタ自動車 / +50,000円（+20.0%）/ 評価額 300,000円 / 保有数量 100株 / 取得価格 2,500円 / 現在の価格 3,000円 / 価格の基準日 2026-10-06」、ドルの銘柄のカードは「AAPL / Apple / +297.50ドル（+19.8%）/ 評価額 1,800.00ドル / 保有数量 10株 / 取得価格 150.25ドル / 現在の価格 180.00ドル / 価格の基準日 2026-10-06」で表示される', async () => {
  respondWithHoldings(MIXED_BODY)

  renderHoldingsPage()

  await screen.findByRole('list', { name: '保有株式' })
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
  expectTextsInOrder(cardOf('7203'), [
    '7203',
    'トヨタ自動車',
    '含み損益',
    '+50,000円（+20.0%）',
    '評価額',
    '300,000円',
    '保有数量',
    '100株',
    '取得価格',
    '2,500円',
    '現在の価格',
    '3,000円',
    '価格の基準日',
    '2026-10-06',
  ])
  expectTextsInOrder(cardOf('AAPL'), [
    'AAPL',
    'Apple',
    '含み損益',
    '+297.50ドル（+19.8%）',
    '評価額',
    '1,800.00ドル',
    '保有数量',
    '10株',
    '取得価格',
    '150.25ドル',
    '現在の価格',
    '180.00ドル',
    '価格の基準日',
    '2026-10-06',
  ])
  // カードは1銘柄1枚、銘柄コードの昇順（API の並びのまま）。
  expect(within(screen.getByRole('list', { name: '保有株式' })).getAllByRole('listitem')).toHaveLength(2)
})

test('売買履歴の基準10・カード表示の補助（要件12・要件13）: 合計の欄（region「合計」）の円に「+50,000円（+20.0%）」「300,000円」、ドルに「+297.50ドル（+19.8%）」「1,800.00ドル」がこの順に表示され、「価格を取得できない銘柄を含みません」は表示されない', async () => {
  respondWithHoldings(MIXED_BODY)

  renderHoldingsPage()

  const summary = await screen.findByRole('region', { name: '合計' })
  const yen = within(summary).getByRole('region', { name: '円' })
  const usd = within(summary).getByRole('region', { name: 'ドル' })
  expect(yen.compareDocumentPosition(usd) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expectTextsInOrder(yen, ['含み損益', '+50,000円（+20.0%）', '評価額', '300,000円'])
  expectTextsInOrder(usd, ['含み損益', '+297.50ドル（+19.8%）', '評価額', '1,800.00ドル'])
  expect(screen.queryByText('価格を取得できない銘柄を含みません')).not.toBeInTheDocument()
})

test('基準14（要件11）: 合計の欄は、「価格を今すぐ更新」のボタンより上、カードの並びより上にある', async () => {
  respondWithHoldings(MIXED_BODY)

  renderHoldingsPage()

  const summary = await screen.findByRole('region', { name: '合計' })
  const refresh = screen.getByRole('button', { name: '価格を今すぐ更新' })
  const cards = screen.getByRole('list', { name: '保有株式' })
  expect(summary.compareDocumentPosition(refresh) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(refresh.compareDocumentPosition(cards) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})

test('基準17: 保有が円の銘柄だけの応答のとき、合計の欄に円の部分だけが表示され、ドルの部分は表示されない', async () => {
  respondWithHoldings({ holdings: [MIXED_BODY.holdings[0]], totals: [MIXED_BODY.totals[0]], combinedTotal: null, managedAccounts: [] })

  renderHoldingsPage()

  const summary = await screen.findByRole('region', { name: '合計' })
  expect(within(summary).getByRole('region', { name: '円' })).toBeInTheDocument()
  expect(within(summary).queryByRole('region', { name: 'ドル' })).not.toBeInTheDocument()
})

test('売買履歴の基準7・カード表示の基準18（要件11）: holdings と totals が空の応答のとき、「保有株式はありません」が表示され、合計の欄・カードは表示されない', async () => {
  respondWithHoldings({ holdings: [], totals: [], combinedTotal: null, managedAccounts: [] })

  renderHoldingsPage()

  expect(await screen.findByText('保有株式はありません')).toBeInTheDocument()
  expect(screen.queryByRole('region', { name: '合計' })).not.toBeInTheDocument()
  expect(screen.queryByText('合計')).not.toBeInTheDocument()
  expect(screen.queryByRole('list', { name: '保有株式' })).not.toBeInTheDocument()
  expect(screen.queryAllByRole('listitem')).toHaveLength(0)
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
})

test('基準3（要件2。Enter から画面が移るまで）: 6758・7203 のカードが並んでいるとき、7203 のカードにフォーカスを当てて Enter を押すと、場所が /holdings/7203 になる（一覧は表示されなくなる）', async () => {
  respondWithHoldings({
    holdings: [
      { ...MIXED_BODY.holdings[0], code: '6758', name: 'ソニーグループ' },
      MIXED_BODY.holdings[0],
    ],
    totals: [MIXED_BODY.totals[0]],
    combinedTotal: null,
    managedAccounts: [],
  })

  renderHoldingsPageWithStockRoute()

  await screen.findByRole('list', { name: '保有株式' })
  const toyota = screen.getByLabelText('7203 トヨタ自動車', { selector: 'li' })
  toyota.focus()
  fireEvent.keyDown(toyota, { key: 'Enter', code: 'Enter' })

  expect(await screen.findByTestId('location')).toHaveTextContent('/holdings/7203')
  expect(screen.queryByRole('list', { name: '保有株式' })).not.toBeInTheDocument()
})

test('補助（要件3）: 一覧の画面で、スペースで持ち上げてから Enter を押しても、場所は /holdings のまま（画面は移らない）', async () => {
  respondWithHoldings({ holdings: [MIXED_BODY.holdings[0]], totals: [MIXED_BODY.totals[0]], combinedTotal: null, managedAccounts: [] })

  renderHoldingsPageWithStockRoute()

  await screen.findByRole('list', { name: '保有株式' })
  const toyota = screen.getByLabelText('7203 トヨタ自動車', { selector: 'li' })
  toyota.focus()
  fireEvent.keyDown(toyota, { key: ' ', code: 'Space' })
  fireEvent.keyDown(toyota, { key: 'Enter', code: 'Enter' })

  expect(screen.queryByTestId('location')).not.toBeInTheDocument()
  expect(screen.getByRole('list', { name: '保有株式' })).toBeInTheDocument()
})

test('補助（プラン 4.7）: holdings が1件以上で totals が空の応答のとき、カードは表示されるが、合計の欄は表示されない', async () => {
  respondWithHoldings({ holdings: MIXED_BODY.holdings, totals: [], combinedTotal: null, managedAccounts: [] })

  renderHoldingsPage()

  // 取得が終わってカードが出てから、合計の欄がないことを確かめる。
  await screen.findByRole('list', { name: '保有株式' })
  expectTextsInOrder(cardOf('7203'), ['7203', 'トヨタ自動車', '+50,000円（+20.0%）', '300,000円'])
  expect(screen.queryByRole('region', { name: '合計' })).not.toBeInTheDocument()
  expect(screen.queryByText('合計')).not.toBeInTheDocument()
})
