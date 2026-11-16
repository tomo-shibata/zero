// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, expect, test, vi, type Mock } from 'vitest'
import { useDeleteHolding } from '../../../src/features/holdings/hooks/useDeleteHolding'
import { useHoldings } from '../../../src/features/holdings/hooks/useHoldings'

// 仕様: 要件 docs/requirements/price-update.md の要件14（「削除する」を押すと、保有データを削除し、ダイアログを閉じて一覧と合計評価額を取得し直して表示する）。
// 境界: プラン docs/plans/price-update/plan.md 判断37（ステップ6 の 2ラウンド目で進行役が決めた）
//   削除が 204 で成功した後の一覧の取り直しが返らなくても、10秒たったらダイアログを閉じる（削除した扱い）。
//   ダイアログは useDeleteHolding の mutation が成功するまで閉じない（DeleteHoldingDialog は mutate に渡した onSuccess で閉じる）ので、
//   「閉じる」は、mutate に渡した onSuccess が呼ばれた時刻で確かめる（時刻の本体）。取り直しが後で返ったら、一覧は取り直した内容になる。
//   isPending・isSuccess は、TanStack Query が状態の変化を setTimeout(0) で画面に伝えるので、onSuccess が呼ばれた時刻ちょうどではなく、
//   その後の1拍（1ms）で切り替わる。そのため、状態は onSuccess の後に1拍待ってから確かめる（hook の作りに依らない確かめ方）。
// mock は境界の外側（ネットワーク = グローバルの fetch）だけに使う。時計は fake timers で進める。
// 一覧の取り直しが起きるのは、一覧を表示している画面（useHoldings の observer がある）のときなので、同じ QueryClient で useHoldings も動かす。

const TWO_HOLDINGS_BODY = {
  holdings: [
    { code: '6758', name: 'ソニーグループ', quantity: 50, acquisitionPrice: '3200.0', currentPrice: null, priceDate: null, valuation: null },
    { code: '7203', name: 'トヨタ自動車', quantity: 400, acquisitionPrice: '2750.0', currentPrice: null, priceDate: null, valuation: null },
  ],
  totalValuation: 0,
  totalExcludesUnpriced: true,
}
const ONE_HOLDING_BODY = {
  holdings: [TWO_HOLDINGS_BODY.holdings[0]],
  totalValuation: 0,
  totalExcludesUnpriced: true,
}

const TIMEOUT_MS = 10_000

let queryClient: QueryClient
let fetchMock: Mock<typeof fetch>
/** 削除の後の一覧の取り直し（2回目の GET /api/holdings）の応答を、テストが返す。 */
let respondToRefetch: (response: Response) => void
/** 削除の API（DELETE）が 204 を返すまでの時間（fake timers の ms）。0 ならすぐ返る。 */
let deleteDelayMs = 0

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

beforeEach(() => {
  deleteDelayMs = 0
  vi.useFakeTimers()
  queryClient = new QueryClient()
  let listRequests = 0
  fetchMock = vi.fn<typeof fetch>((input, init) => {
    const url = requestedUrl(input)
    const method = requestedMethod(input, init)
    if (url === '/api/holdings' && method === 'GET') {
      listRequests += 1
      // 1回目（画面を開いたとき）はすぐ返す。2回目（削除の後の取り直し）は、テストが返すまで返らない。
      if (listRequests === 1) return Promise.resolve(jsonResponse(TWO_HOLDINGS_BODY))
      return new Promise<Response>((resolve) => {
        respondToRefetch = resolve
      })
    }
    if (url === '/api/holdings/7203' && method === 'DELETE') {
      if (deleteDelayMs === 0) return Promise.resolve(new Response(null, { status: 204 }))
      return new Promise<Response>((resolve) => {
        setTimeout(() => resolve(new Response(null, { status: 204 })), deleteDelayMs)
      })
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

test('判断37・要件14: 削除が 204 で成功した後、一覧の取り直しが返らなくても、9,999ms まではまだ削除中で、10,000ms で成功になる（削除した扱い）。取り直しが後で返ると、一覧は取り直した内容になる', async () => {
  const { result } = renderHook(() => ({ deletion: useDeleteHolding(), list: useHoldings() }), { wrapper })
  // 画面を開いたときの一覧が出ている。
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0)
  })
  expect(result.current.list.data?.holdings.map((holding) => holding.code)).toEqual(['6758', '7203'])

  // 7203 の削除を始める。削除は 204 で成功し、一覧の取り直しが始まるが、その応答は返らない。
  const successes: string[] = []
  act(() => {
    result.current.deletion.mutate('7203', { onSuccess: () => successes.push('success') })
  })
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0)
  })
  expect(fetchMock.mock.calls.filter(([input, init]) => requestedUrl(input) === '/api/holdings' && requestedMethod(input, init) === 'GET')).toHaveLength(2)
  expect(result.current.deletion.isPending).toBe(true)
  expect(successes).toEqual([])

  // 9,999ms たってもまだ削除中（ダイアログは閉じない）。
  await act(async () => {
    await vi.advanceTimersByTimeAsync(TIMEOUT_MS - 1)
  })
  expect(result.current.deletion.isPending).toBe(true)
  expect(successes).toEqual([])

  // 10,000ms で、取り直しを待たずに onSuccess が呼ばれる（ダイアログが閉じる合図。判断37 の「10秒で閉じる」）。一覧はまだ古い内容のまま。
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1)
  })
  expect(successes).toEqual(['success'])
  expect(result.current.list.data?.holdings.map((holding) => holding.code)).toEqual(['6758', '7203'])

  // ライブラリが状態を伝える1拍の後に、削除中でなくなり、成功になっている。
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1)
  })
  expect(result.current.deletion.isPending).toBe(false)
  expect(result.current.deletion.isSuccess).toBe(true)
  expect(successes).toEqual(['success'])

  // 後で取り直しが返ると、7203 の行がなくなる。
  await act(async () => {
    respondToRefetch(jsonResponse(ONE_HOLDING_BODY))
    await vi.advanceTimersByTimeAsync(0)
  })
  expect(result.current.list.data?.holdings.map((holding) => holding.code)).toEqual(['6758'])
})

test('判断37・要件14: 一覧の取り直しが10秒より前に返れば、そのときに成功になる（10秒を待たない）', async () => {
  const { result } = renderHook(() => ({ deletion: useDeleteHolding(), list: useHoldings() }), { wrapper })
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0)
  })
  const successes: string[] = []
  act(() => {
    result.current.deletion.mutate('7203', { onSuccess: () => successes.push('success') })
  })
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0)
  })
  expect(result.current.deletion.isPending).toBe(true)

  await act(async () => {
    await vi.advanceTimersByTimeAsync(2_000)
    respondToRefetch(jsonResponse(ONE_HOLDING_BODY))
    await vi.advanceTimersByTimeAsync(0)
  })

  // 取り直しが返ったとき（10秒より前）に onSuccess が呼ばれている。状態は、1拍の後に成功になる。
  expect(successes).toEqual(['success'])
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1)
  })
  expect(result.current.deletion.isPending).toBe(false)
  expect(result.current.deletion.isSuccess).toBe(true)
  expect(result.current.list.data?.holdings.map((holding) => holding.code)).toEqual(['6758'])
})

test('境界値と異常系の指摘2（判断37・要件14）: 削除が 204 の後、一覧の取り直しが HTTP 500 で失敗しても、mutate に渡した onSuccess は呼ばれ、削除は成功のまま（isError は false）', async () => {
  const { result } = renderHook(() => ({ deletion: useDeleteHolding(), list: useHoldings() }), { wrapper })
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0)
  })
  const successes: string[] = []
  const errors: string[] = []
  act(() => {
    result.current.deletion.mutate('7203', {
      onSuccess: () => successes.push('success'),
      onError: () => errors.push('error'),
    })
  })
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0)
  })
  expect(successes).toEqual([])

  // 取り直しが 500 で失敗する。
  await act(async () => {
    respondToRefetch(new Response(JSON.stringify({ error: 'internal_error' }), { status: 500 }))
    await vi.advanceTimersByTimeAsync(0)
  })

  expect(successes).toEqual(['success'])
  expect(errors).toEqual([])
  // 状態が伝わる1拍の後も、削除は成功のまま。
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1)
  })
  expect(result.current.deletion.isError).toBe(false)
  expect(result.current.deletion.isPending).toBe(false)
  expect(result.current.deletion.isSuccess).toBe(true)
})

test('テストの網羅の指摘2（判断37）: 取り直しの10秒は、削除の応答の後から数える。削除が 5,000ms 後に 204 を返し、取り直しが返らないとき、14,999ms では onSuccess が呼ばれず、15,000ms で呼ばれる', async () => {
  deleteDelayMs = 5_000
  const { result } = renderHook(() => ({ deletion: useDeleteHolding(), list: useHoldings() }), { wrapper })
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0)
  })
  const successes: string[] = []
  act(() => {
    result.current.deletion.mutate('7203', { onSuccess: () => successes.push('success') })
  })

  // 削除の応答が返るまでの5,000ms と、その後の取り直しを待つ9,999ms（合わせて 14,999ms）では、まだ呼ばれない。
  await act(async () => {
    await vi.advanceTimersByTimeAsync(5_000)
  })
  expect(fetchMock.mock.calls.filter(([input, init]) => requestedUrl(input) === '/api/holdings' && requestedMethod(input, init) === 'GET')).toHaveLength(2)
  await act(async () => {
    await vi.advanceTimersByTimeAsync(TIMEOUT_MS - 1)
  })
  expect(successes).toEqual([])
  expect(result.current.deletion.isPending).toBe(true)

  // 15,000ms（削除の応答の10,000ms 後）で呼ばれる。
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1)
  })
  expect(successes).toEqual(['success'])
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1)
  })
  expect(result.current.deletion.isSuccess).toBe(true)
})
