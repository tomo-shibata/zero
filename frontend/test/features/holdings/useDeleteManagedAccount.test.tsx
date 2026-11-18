// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, expect, test, vi, type Mock } from 'vitest'
import { useDeleteManagedAccount } from '../../../src/features/holdings/hooks/useDeleteManagedAccount'
import { useHoldings } from '../../../src/features/holdings/hooks/useHoldings'

// 仕様: おまかせ運用の要件 docs/requirements/managed-account.md の要件24（「削除する」を押すと、その口座と記録をすべて削除し、ダイアログを閉じて
//   一覧と合計を取得し直す。失敗したときは、ダイアログを開いたまま「削除できませんでした」と出し、一覧は変えない）。
// 境界: プラン docs/plans/managed-account/plan.md 4.8 の PR③
//   useDeleteManagedAccount(): UseMutationResult<void, Error, string>（mutate の引数は名前）。再試行しない。成功したら一覧を取り直し、
//   取り直しか10秒の早い方まで待つ（useDeleteHolding と同じ。テストの書き方は useDeleteHolding.test.tsx と同じ）。
// テスト計画: プラン 5.2 の PR③ の useDeleteManagedAccount.test.tsx の行「成功したら一覧を取り直す。失敗しても再試行しない」。
// mock は境界の外側（ネットワーク = グローバルの fetch）だけに使う。時計は fake timers で進める。
// 一覧の取り直しが起きるのは、一覧を表示している画面（useHoldings の observer がある）のときなので、同じ QueryClient で useHoldings も動かす。

const TOTALS = [{ currency: 'JPY', valuation: '4500000', excludesUnpriced: false, unrealizedGain: '1000000', unrealizedGainRate: '28.6' }]
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
/** 口座が2つの一覧（名前の順）。 */
const TWO_ACCOUNTS_BODY = { holdings: [], totals: TOTALS, combinedTotal: null, managedAccounts: [ALPHA, WEALTHNAVI] }
/** WealthNavi を削除した後の一覧。 */
const ONE_ACCOUNT_BODY = { holdings: [], totals: TOTALS, combinedTotal: null, managedAccounts: [ALPHA] }

const TIMEOUT_MS = 10_000

let queryClient: QueryClient
let fetchMock: Mock<typeof fetch>
/** 削除の後の一覧の取り直し（2回目の GET /api/holdings）の応答を、テストが返す。 */
let respondToRefetch: (response: Response) => void
/** 削除の API の応答（既定は 204）。 */
let deleteStatus = 204

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

/** 一覧の取得（GET /api/holdings）の回数。 */
function listRequestCount(): number {
  return fetchMock.mock.calls.filter(([input, init]) => requestedUrl(input) === '/api/holdings' && requestedMethod(input, init) === 'GET').length
}

/** 削除の要求（DELETE /api/managed-accounts?name=...）の回数。 */
function deleteRequestCount(): number {
  return fetchMock.mock.calls.filter(
    ([input, init]) => new URL(requestedUrl(input), 'http://localhost').pathname === '/api/managed-accounts' && requestedMethod(input, init) === 'DELETE',
  ).length
}

beforeEach(() => {
  deleteStatus = 204
  vi.useFakeTimers()
  queryClient = new QueryClient()
  let listRequests = 0
  fetchMock = vi.fn<typeof fetch>((input, init) => {
    const url = new URL(requestedUrl(input), 'http://localhost')
    const method = requestedMethod(input, init)
    if (url.pathname === '/api/holdings' && method === 'GET') {
      listRequests += 1
      // 1回目（画面を開いたとき）はすぐ返す。2回目（削除の後の取り直し）は、テストが返すまで返らない。
      if (listRequests === 1) return Promise.resolve(jsonResponse(TWO_ACCOUNTS_BODY))
      return new Promise<Response>((resolve) => {
        respondToRefetch = resolve
      })
    }
    if (url.pathname === '/api/managed-accounts' && method === 'DELETE') {
      return Promise.resolve(new Response(deleteStatus === 204 ? null : JSON.stringify({ error: 'internal_error' }), { status: deleteStatus }))
    }
    return Promise.resolve(new Response('not found', { status: 404 }))
  })
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
}

/** 一覧に出ている口座の名前。 */
function accountNames(list: ReturnType<typeof useHoldings>): string[] | undefined {
  return list.data?.managedAccounts.map((account) => account.name)
}

test('要件24: 削除が 204 で成功すると、一覧を取り直す。取り直しが返ると、その時点で成功になり、一覧から WealthNavi がなくなる', async () => {
  const { result } = renderHook(() => ({ deletion: useDeleteManagedAccount(), list: useHoldings() }), { wrapper })
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0)
  })
  expect(accountNames(result.current.list)).toEqual(['Alpha', 'WealthNavi'])
  expect(listRequestCount()).toBe(1)

  const successes: string[] = []
  act(() => {
    result.current.deletion.mutate('WealthNavi', { onSuccess: () => successes.push('success') })
  })
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0)
  })

  // 削除の要求が1回送られ、一覧の取り直しが始まっている（返るまでは削除中で、onSuccess はまだ）。
  expect(deleteRequestCount()).toBe(1)
  expect(listRequestCount()).toBe(2)
  expect(result.current.deletion.isPending).toBe(true)
  expect(successes).toEqual([])

  // 取り直しが返ると、onSuccess が呼ばれ、一覧は取り直した内容になる。状態は、ライブラリが伝える1拍の後に成功になる。
  await act(async () => {
    respondToRefetch(jsonResponse(ONE_ACCOUNT_BODY))
    await vi.advanceTimersByTimeAsync(0)
  })
  expect(successes).toEqual(['success'])
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1)
  })
  expect(result.current.deletion.isSuccess).toBe(true)
  expect(accountNames(result.current.list)).toEqual(['Alpha'])
})

test('要件24（useDeleteHolding と同じ）: 取り直しが返らなくても、9,999ms まではまだ削除中で、10,000ms で成功になる（ダイアログを閉じる合図）。取り直しが後で返ると一覧は変わる', async () => {
  const { result } = renderHook(() => ({ deletion: useDeleteManagedAccount(), list: useHoldings() }), { wrapper })
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0)
  })
  const successes: string[] = []
  act(() => {
    result.current.deletion.mutate('WealthNavi', { onSuccess: () => successes.push('success') })
  })
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0)
  })
  expect(listRequestCount()).toBe(2)

  await act(async () => {
    await vi.advanceTimersByTimeAsync(TIMEOUT_MS - 1)
  })
  expect(result.current.deletion.isPending).toBe(true)
  expect(successes).toEqual([])

  await act(async () => {
    await vi.advanceTimersByTimeAsync(1)
  })
  expect(successes).toEqual(['success'])
  // 一覧はまだ古い内容のまま。
  expect(accountNames(result.current.list)).toEqual(['Alpha', 'WealthNavi'])
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1)
  })
  expect(result.current.deletion.isSuccess).toBe(true)

  await act(async () => {
    respondToRefetch(jsonResponse(ONE_ACCOUNT_BODY))
    await vi.advanceTimersByTimeAsync(0)
  })
  expect(accountNames(result.current.list)).toEqual(['Alpha'])
})

test('要件24: 削除が HTTP 500 で失敗しても再試行せず（削除の要求は1回だけ）、onError が呼ばれ、一覧は取り直さず変わらない', async () => {
  deleteStatus = 500
  const { result } = renderHook(() => ({ deletion: useDeleteManagedAccount(), list: useHoldings() }), { wrapper })
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0)
  })
  const successes: string[] = []
  const errors: string[] = []
  act(() => {
    result.current.deletion.mutate('WealthNavi', {
      onSuccess: () => successes.push('success'),
      onError: () => errors.push('error'),
    })
  })
  // 再試行があれば間隔をあけて続くので、十分に時計を進めてから数える。
  await act(async () => {
    await vi.advanceTimersByTimeAsync(TIMEOUT_MS * 3)
  })

  expect(deleteRequestCount()).toBe(1)
  expect(errors).toEqual(['error'])
  expect(successes).toEqual([])
  expect(result.current.deletion.isError).toBe(true)
  // 一覧は変えない（取り直しも始めない）。
  expect(listRequestCount()).toBe(1)
  expect(accountNames(result.current.list)).toEqual(['Alpha', 'WealthNavi'])
})

test('要件24: 削除が 204 の後、一覧の取り直しが HTTP 500 で失敗しても、削除は成功のまま（onSuccess は呼ばれ、isError は false）', async () => {
  const { result } = renderHook(() => ({ deletion: useDeleteManagedAccount(), list: useHoldings() }), { wrapper })
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0)
  })
  const successes: string[] = []
  const errors: string[] = []
  act(() => {
    result.current.deletion.mutate('WealthNavi', {
      onSuccess: () => successes.push('success'),
      onError: () => errors.push('error'),
    })
  })
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0)
  })

  await act(async () => {
    respondToRefetch(new Response(JSON.stringify({ error: 'internal_error' }), { status: 500 }))
    await vi.advanceTimersByTimeAsync(0)
  })
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1)
  })

  expect(successes).toEqual(['success'])
  expect(errors).toEqual([])
  expect(result.current.deletion.isError).toBe(false)
  expect(result.current.deletion.isSuccess).toBe(true)
})
