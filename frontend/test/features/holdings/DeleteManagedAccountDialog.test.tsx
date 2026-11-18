// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeAll, beforeEach, expect, test, vi, type Mock } from 'vitest'
import { DeleteManagedAccountDialog } from '../../../src/features/holdings/components/DeleteManagedAccountDialog'

// 仕様: おまかせ運用の要件 docs/requirements/managed-account.md の要件23・要件24（削除の確認のダイアログ）。
//   基準16・基準18 の通しは E2E（managed-account.spec.ts）。ここはダイアログ1つの振る舞い。
// 境界: プラン docs/plans/managed-account/plan.md 4.8 の PR③
//   - DeleteManagedAccountDialog: props { name: string; onClose: (deleted: boolean) => void }。
//     削除に成功して閉じるなら true、「キャンセル」と Esc なら false（DeleteHoldingDialog と同じ）。
//   - 削除の確認は今の ConfirmDialog（role="alertdialog"、名前は文言そのもの、「キャンセル」「削除する」、失敗は role="alert" で「削除できませんでした」）。
//   - 削除の要求は DELETE /api/managed-accounts?name=<名前>（4.8 の deleteManagedAccount）。
// テスト計画: プラン 5.2 の PR③ の DeleteManagedAccountDialog.test.tsx の行
//   「文言が『WealthNaviを削除しますか？この口座の記録はすべて削除されます。』。『キャンセル』で onClose(false) が呼ばれ、削除の API を呼ばない。
//   『削除する』で成功なら onClose(true)、失敗なら『削除できませんでした』が出て onClose を呼ばない（要件23・要件24）」。
// mock は境界の外側（ネットワーク = グローバルの fetch）だけに使う。jsdom には <dialog> の showModal・close がないので、ブラウザと同じ動き
//   （open 属性・close イベント）の代用を差し込む（StockHistoryPage.test.tsx と同じ）。

const MESSAGE = 'WealthNaviを削除しますか？この口座の記録はすべて削除されます。'

let queryClient: QueryClient
let fetchMock: Mock<typeof fetch>
/** 削除の API（DELETE）の応答の状態。 */
let deleteStatus = 204

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

/** 届いた削除の要求（DELETE /api/managed-accounts）の、名前の一覧。 */
function deletedNames(): (string | null)[] {
  return fetchMock.mock.calls
    .filter(([input, init]) => requestedMethod(input, init) === 'DELETE')
    .map(([input]) => {
      const url = new URL(requestedUrl(input), 'http://localhost')
      expect(url.pathname).toBe('/api/managed-accounts')
      return url.searchParams.get('name')
    })
}

beforeEach(() => {
  deleteStatus = 204
  queryClient = new QueryClient()
  fetchMock = vi.fn<typeof fetch>(async (input, init) => {
    if (requestedMethod(input, init) === 'DELETE') {
      return deleteStatus === 204
        ? new Response(null, { status: 204 })
        : new Response(JSON.stringify({ error: 'internal_error' }), { status: deleteStatus, headers: { 'Content-Type': 'application/json' } })
    }
    // 削除の後の一覧の取り直し（口座の削除は、一覧を取り直す）。
    return new Response(JSON.stringify({ holdings: [], totals: [], combinedTotal: null, managedAccounts: [] }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  })
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  vi.unstubAllGlobals()
})

function renderDialog(onClose: (deleted: boolean) => void) {
  return render(
    <QueryClientProvider client={queryClient}>
      <DeleteManagedAccountDialog name="WealthNavi" onClose={onClose} />
    </QueryClientProvider>,
  )
}

test('要件23: 確認のダイアログ（alertdialog）の名前と文言が「WealthNaviを削除しますか？この口座の記録はすべて削除されます。」で、「キャンセル」と「削除する」のボタンがある', async () => {
  renderDialog(() => {})

  const dialog = await screen.findByRole('alertdialog', { name: MESSAGE })
  expect(dialog).toHaveTextContent(MESSAGE)
  expect(screen.getByRole('button', { name: 'キャンセル' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: '削除する' })).toBeInTheDocument()
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  // 開いただけでは何も送らない。
  expect(fetchMock).not.toHaveBeenCalled()
})

test('要件23: 名前が違えば、文の先頭の名前も変わる（「ロボアド Bを削除しますか？この口座の記録はすべて削除されます。」）', async () => {
  render(
    <QueryClientProvider client={queryClient}>
      <DeleteManagedAccountDialog name="ロボアド B" onClose={() => {}} />
    </QueryClientProvider>,
  )

  expect(await screen.findByRole('alertdialog', { name: 'ロボアド Bを削除しますか？この口座の記録はすべて削除されます。' })).toBeInTheDocument()
})

test('要件23: 「キャンセル」を押すと onClose(false) が1回呼ばれ、削除の API は呼ばれない', async () => {
  const onClose = vi.fn()
  renderDialog(onClose)

  fireEvent.click(await screen.findByRole('button', { name: 'キャンセル' }))

  await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1))
  expect(onClose).toHaveBeenCalledWith(false)
  expect(fetchMock).not.toHaveBeenCalled()
})

test('要件24: 「削除する」を押して削除が 204 で成功すると、DELETE /api/managed-accounts?name=WealthNavi が1回送られ、onClose(true) が1回呼ばれる', async () => {
  const onClose = vi.fn()
  renderDialog(onClose)

  fireEvent.click(await screen.findByRole('button', { name: '削除する' }))

  await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1))
  expect(onClose).toHaveBeenCalledWith(true)
  expect(deletedNames()).toEqual(['WealthNavi'])
})

test('要件24: 削除が HTTP 500 で失敗すると、ダイアログは開いたまま role="alert" で「削除できませんでした」が出て、onClose は呼ばれない', async () => {
  deleteStatus = 500
  const onClose = vi.fn()
  renderDialog(onClose)

  fireEvent.click(await screen.findByRole('button', { name: '削除する' }))

  expect(await screen.findByRole('alert')).toHaveTextContent('削除できませんでした')
  expect(screen.getByRole('alertdialog', { name: MESSAGE })).toBeInTheDocument()
  expect(screen.getByRole('alertdialog')).toHaveAttribute('open')
  expect(onClose).not.toHaveBeenCalled()
  expect(deletedNames()).toEqual(['WealthNavi'])
})

test('要件24: 失敗した後に「削除する」を押し直せる。2回目が成功すると onClose(true) が1回だけ呼ばれる', async () => {
  deleteStatus = 500
  const onClose = vi.fn()
  renderDialog(onClose)
  fireEvent.click(await screen.findByRole('button', { name: '削除する' }))
  await screen.findByRole('alert')
  await waitFor(() => expect(screen.getByRole('button', { name: '削除する' })).toBeEnabled())

  deleteStatus = 204
  fireEvent.click(screen.getByRole('button', { name: '削除する' }))

  await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1))
  expect(onClose).toHaveBeenCalledWith(true)
  expect(deletedNames()).toEqual(['WealthNavi', 'WealthNavi'])
})
