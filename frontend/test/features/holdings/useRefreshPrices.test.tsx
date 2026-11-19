// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi, type Mock } from 'vitest'
import { useRefreshPrices } from '../../../src/features/holdings/hooks/useRefreshPrices'
import { UNSEEN_NOTIFICATION_COUNT_QUERY_KEY } from '../../../src/shared/api/queryKeys'

// 仕様: 価格の変動の通知の要件 docs/requirements/price-alerts.md の要件27（バッジの数は、今すぐ更新が終わったとき（結果によらない）に取り直す）。
// 境界: プラン docs/plans/price-alerts/plan.md 4.6「今すぐ更新の後の取り直し」・4.7
//   features/holdings/hooks/useRefreshPrices.ts の onSettled で UNSEEN_NOTIFICATION_COUNT_QUERY_KEY（shared/api/queryKeys.ts）を無効にする（結果によらない）。
//   features/holdings と features/notifications は互いを import しないので、key は shared/api/queryKeys.ts に置く。
// テスト計画: プラン 5.2 の useRefreshPrices.test.tsx の行（今すぐ更新が成功しても失敗しても、見ていない数の key が無効になる）。
// mock は境界の外側（ネットワーク = グローバルの fetch）だけに使う。見ていない数を使う画面（ベル）は描かないので、取り直しは起きない。無効になったこと（isInvalidated）で確かめる。

let queryClient: QueryClient
let fetchMock: Mock<typeof fetch>

beforeEach(() => {
  queryClient = new QueryClient()
  // 見ていない数が取得済みの状態（ベルが数を持っている）を作る。
  queryClient.setQueryData(UNSEEN_NOTIFICATION_COUNT_QUERY_KEY, 3)
  fetchMock = vi.fn<typeof fetch>(() => Promise.resolve(new Response('not found', { status: 404 })))
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  vi.unstubAllGlobals()
})

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
}

describe('今すぐ更新の後に見ていない数を取り直す（要件27。結果によらない）', () => {
  test('要件27: 今すぐ更新が成功（200）したとき、見ていない数の key が無効になる', async () => {
    fetchMock.mockImplementation(() =>
      Promise.resolve(new Response(JSON.stringify({ saved: ['7203'], failed: [] }), { status: 200, headers: { 'Content-Type': 'application/json' } })),
    )
    const { result } = renderHook(() => useRefreshPrices(), { wrapper })
    expect(queryClient.getQueryState(UNSEEN_NOTIFICATION_COUNT_QUERY_KEY)?.isInvalidated).toBe(false)

    await act(async () => {
      result.current.mutate()
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(queryClient.getQueryState(UNSEEN_NOTIFICATION_COUNT_QUERY_KEY)?.isInvalidated).toBe(true)
  })

  test('要件27: 今すぐ更新が失敗（サーバーが 500）したときも、見ていない数の key が無効になる', async () => {
    fetchMock.mockImplementation(() => Promise.resolve(new Response(JSON.stringify({ error: 'internal_error' }), { status: 500 })))
    const { result } = renderHook(() => useRefreshPrices(), { wrapper })
    expect(queryClient.getQueryState(UNSEEN_NOTIFICATION_COUNT_QUERY_KEY)?.isInvalidated).toBe(false)

    await act(async () => {
      result.current.mutate()
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(queryClient.getQueryState(UNSEEN_NOTIFICATION_COUNT_QUERY_KEY)?.isInvalidated).toBe(true)
  })

  test('要件27: 今すぐ更新の通信が失敗（fetch が reject）したときも、見ていない数の key が無効になる', async () => {
    fetchMock.mockImplementation(() => Promise.reject(new TypeError('Failed to fetch')))
    const { result } = renderHook(() => useRefreshPrices(), { wrapper })

    await act(async () => {
      result.current.mutate()
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(queryClient.getQueryState(UNSEEN_NOTIFICATION_COUNT_QUERY_KEY)?.isInvalidated).toBe(true)
  })
})
