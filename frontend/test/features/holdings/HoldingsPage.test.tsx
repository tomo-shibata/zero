// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
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
