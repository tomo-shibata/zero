// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi, type Mock } from 'vitest'
import type { HoldingsList } from '../../../src/entities/holding/model'
import { HOLDINGS_QUERY_KEY, useHoldings } from '../../../src/features/holdings/hooks/useHoldings'
import { useHoldingsOrder } from '../../../src/features/holdings/hooks/useHoldingsOrder'

// 仕様: カード表示の要件 docs/requirements/holdings-cards.md の要件22・要件23・要件29・要件30、基準23・基準24（単体の部分）・基準30。
// 境界: プラン docs/plans/holdings-cards/plan.md 4.4「並びと保存」・4.6 PR②・判断11
//   useHoldingsOrder(): { move(activeCode, overCode | null): void; saveFailed: boolean }
//   - QueryClientProvider の中で、HOLDINGS_QUERY_KEY に一覧が入っている状態で使う。
//   - move は、キャッシュの今の holdings の並びを元に新しい並びを作り、キャッシュに書いてから（すぐ表示）保存（PUT /api/holding-order）を送る。
//     元と同じ位置、またはどのカードの上でもないなら何もしない（fetch を呼ばない）。
//   - 保存は1つずつ送る。送っている間にまた並べ替えたら、最後の並びだけを覚えておき、前の保存が終わったら送る。
//   - 保存に失敗したら、待っている並びを捨て、キャッシュの holdings を、最後に保存できた並び（まだなければ読み込んだときの並び）にして、
//     saveFailed を true にする。次に並べ替えたら false に戻す。
//   - 保存の待ちがなくなり、最後の保存が成功したときだけ HOLDINGS_QUERY_KEY を invalidate して一覧を取り直す（GET /api/holdings が飛ぶ）。失敗のときは取り直さない。
//   - 並べ替えたとき、途中の一覧の取り直しは取り消す（古い応答で並びを上書きしないため）。
//   テストは queryClient.getQueryData(HOLDINGS_QUERY_KEY) の holdings の順で表示の並びを確かめる。
// テスト計画: プラン 5.1 の基準23（hook の部分）・24（hook の部分）・30、5.3 PR② の「useHoldingsOrder」の行。
// mock は境界の外側（ネットワーク = グローバルの fetch。小さなサーバーの代わり）だけに使う。
//   fake のサーバーは、GET /api/holdings には「その時点で最後に 204 を返した PUT の本文の順」（なければ最初の並び）で一覧を返す。
//   最初の GET だけはすぐ返し、2回目以降の GET は、応答の内容を要求を受けた時点で決めて、テストが放すまで保留する。
//   PUT は、テストが応答（204・500・通信の失敗）を決めるまで保留する。

const SONY = {
  code: '6758',
  name: 'ソニーグループ',
  currency: 'JPY',
  quantity: 10,
  acquisitionPrice: '4000.0',
  currentPrice: '3500.0',
  priceDate: '2026-10-06',
  valuation: '35000',
  unrealizedGain: '-5000',
  unrealizedGainRate: '-12.5',
} as const
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
} as const
const APPLE = {
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
} as const
const BY_CODE: Record<string, unknown> = { '6758': SONY, '7203': TOYOTA, AAPL: APPLE }
const TOTALS = [
  { currency: 'JPY', valuation: '335000', excludesUnpriced: false, unrealizedGain: '45000', unrealizedGainRate: '15.5' },
  { currency: 'USD', valuation: '1800.00', excludesUnpriced: false, unrealizedGain: '297.50', unrealizedGainRate: '19.8' },
]

const LOADED_ORDER = ['6758', '7203', 'AAPL']

/** 並び order の一覧の応答の本文。 */
function listBody(order: readonly string[]) {
  return { holdings: order.map((code) => BY_CODE[code]), totals: TOTALS, combinedTotal: null, managedAccounts: [] }
}

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

function requestedUrl(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.href
  return input.url
}

function requestedMethod(input: RequestInfo | URL, init?: RequestInit): string {
  const method = init?.method ?? (input instanceof Request ? input.method : 'GET')
  return method.toUpperCase()
}

/** 保留している PUT（保存の要求）。respond で応答を決める。 */
type HeldPut = {
  codes: string[]
  /** status の応答を返す。204 ならサーバーの並びが codes になる。 */
  respond: (status: number) => void
  /** 通信の失敗にする。 */
  fail: () => void
}

/** 保留している GET（一覧の取り直し）。release で、要求を受けた時点の並びの応答を返す。 */
type HeldGet = { order: string[]; release: () => void }

type FakeServer = {
  puts: HeldPut[]
  heldGets: HeldGet[]
  /** GET /api/holdings の要求の数（最初の1回を含む）。 */
  getCount: () => number
  /** PUT /api/holding-order の要求の数。 */
  putCount: () => number
}

let queryClient: QueryClient
let fetchMock: Mock<typeof fetch>
let server: FakeServer

/** fetch を fake のサーバーに差し替える。最初に読み込まれる並びは initialOrder。 */
function installServer(initialOrder: readonly string[]): FakeServer {
  let serverOrder = [...initialOrder]
  let gets = 0
  let putRequests = 0
  const fake: FakeServer = {
    puts: [],
    heldGets: [],
    getCount: () => gets,
    putCount: () => putRequests,
  }
  fetchMock = vi.fn<typeof fetch>((input, init) => {
    const url = requestedUrl(input)
    const method = requestedMethod(input, init)
    if (url === '/api/holdings' && method === 'GET') {
      gets += 1
      if (gets === 1) return Promise.resolve(jsonResponse(listBody(serverOrder)))
      // 要求を受けた時点のサーバーの並びで答える。応答を返す時刻は、テストが release で決める。
      const orderAtRequest = [...serverOrder]
      return new Promise<Response>((resolve) => {
        fake.heldGets.push({ order: orderAtRequest, release: () => resolve(jsonResponse(listBody(orderAtRequest))) })
      })
    }
    if (url === '/api/holding-order' && method === 'PUT') {
      putRequests += 1
      const body = JSON.parse(String(init?.body)) as { codes: string[] }
      return new Promise<Response>((resolve, reject) => {
        fake.puts.push({
          codes: body.codes,
          respond: (status) => {
            if (status === 204) serverOrder = [...body.codes]
            resolve(new Response(status === 204 ? null : JSON.stringify({ error: 'x' }), { status }))
          },
          fail: () => reject(new TypeError('Failed to fetch')),
        })
      })
    }
    return Promise.resolve(new Response('not found', { status: 404 }))
  })
  vi.stubGlobal('fetch', fetchMock)
  return fake
}

beforeEach(() => {
  queryClient = new QueryClient()
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  vi.unstubAllGlobals()
})

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
}

/** キャッシュの一覧の銘柄コード（表示の並び）。 */
function cachedOrder(): string[] | undefined {
  return queryClient.getQueryData<HoldingsList>(HOLDINGS_QUERY_KEY)?.holdings.map((holding) => holding.code)
}

/** 非同期の後始末（応答の処理・状態の伝わり）が終わるのを少し待つ。「起きない」ことを確かめる前に使う。 */
async function flush(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 30))
  })
}

/** 一覧を読み込み済み（キャッシュに並び LOADED_ORDER が入っている。GET は1回）の状態で useHoldingsOrder を使う。 */
async function renderWithLoadedList() {
  server = installServer(LOADED_ORDER)
  const hook = renderHook(() => ({ order: useHoldingsOrder(), list: useHoldings() }), { wrapper })
  await waitFor(() => expect(hook.result.current.list.data).toBeDefined())
  expect(cachedOrder()).toEqual(LOADED_ORDER)
  expect(server.getCount()).toBe(1)
  return hook
}

/** 一覧を取りに行かない、キャッシュに直接入れた状態で useHoldingsOrder だけを使う（fetch が1回も呼ばれないことを確かめるため）。 */
function renderWithSeededCache() {
  server = installServer(LOADED_ORDER)
  queryClient.setQueryData(HOLDINGS_QUERY_KEY, listBody(LOADED_ORDER))
  return renderHook(() => useHoldingsOrder(), { wrapper })
}

describe('並べ替えない場合（要件22・要件23）', () => {
  test('カード表示の基準23: どのカードの上でもない（over が null）なら、並びは変わらず fetch は呼ばれず、saveFailed は false のまま', async () => {
    const { result } = renderWithSeededCache()

    act(() => result.current.move('AAPL', null))
    await flush()

    expect(cachedOrder()).toEqual(LOADED_ORDER)
    expect(fetchMock).not.toHaveBeenCalled()
    expect(result.current.saveFailed).toBe(false)
  })

  test('カード表示の基準24: 元の位置（active と over が同じ）なら、並びは変わらず fetch は呼ばれない', async () => {
    const { result } = renderWithSeededCache()

    act(() => result.current.move('7203', '7203'))
    await flush()

    expect(cachedOrder()).toEqual(LOADED_ORDER)
    expect(fetchMock).not.toHaveBeenCalled()
    expect(result.current.saveFailed).toBe(false)
  })
})

describe('基準30（要件29）: 保存を待っている間に続けて並べ替える', () => {
  test('カード表示の基準30: 1回目の保存が終わる前に2回目を置く。キャッシュの並びはすぐ変わり、保存は1つずつ順に送られ、最後の並びが残る。取り直しの応答を保留している間も、キャッシュは最後の並びのまま', async () => {
    const { result } = await renderWithLoadedList()

    // 1回目: AAPL を先頭に置く（AAPL、6758、7203）。保存の応答はまだ返らない。要件23: 保存の応答を待たずに新しい並びを表示する。
    act(() => result.current.order.move('AAPL', '6758'))
    expect(cachedOrder()).toEqual(['AAPL', '6758', '7203'])
    await waitFor(() => expect(server.putCount()).toBe(1))
    expect(server.puts[0]?.codes).toEqual(['AAPL', '6758', '7203'])

    // 2回目: 1回目の保存が終わる前に、7203 を先頭に置く（7203、AAPL、6758）。今のキャッシュの並びが元になる。
    act(() => result.current.order.move('7203', 'AAPL'))
    expect(cachedOrder()).toEqual(['7203', 'AAPL', '6758'])
    // 保存は1つずつ。1回目が終わるまで、2回目は送られない。
    await flush()
    expect(server.putCount()).toBe(1)

    // 1回目の保存が（2回目より先に）終わる。キャッシュの並びは2回目のままで、2回目の保存が送られる。
    await act(async () => server.puts[0]?.respond(204))
    await waitFor(() => expect(server.putCount()).toBe(2))
    expect(server.puts[1]?.codes).toEqual(['7203', 'AAPL', '6758'])
    expect(cachedOrder()).toEqual(['7203', 'AAPL', '6758'])
    // 保存の待ちがまだあるので、一覧は取り直さない。
    expect(server.getCount()).toBe(1)

    // 2回目の保存が終わる。待ちがなくなり、最後の保存が成功したので、一覧を取り直す（GET /api/holdings が1回飛ぶ）。
    await act(async () => server.puts[1]?.respond(204))
    await waitFor(() => expect(server.getCount()).toBe(2))
    // 取り直しの応答を保留している間、キャッシュの並びは最後の並びのまま（hook が自分で並べた結果）。
    expect(cachedOrder()).toEqual(['7203', 'AAPL', '6758'])
    expect(server.heldGets).toHaveLength(1)
    expect(server.heldGets[0]?.order).toEqual(['7203', 'AAPL', '6758'])

    // 取り直しの応答（サーバーの並び＝最後に 204 を返した PUT の本文の順）を返す。キャッシュはその並びで、取り直しはちょうど1回。
    await act(async () => server.heldGets[0]?.release())
    await flush()
    expect(cachedOrder()).toEqual(['7203', 'AAPL', '6758'])
    expect(server.getCount()).toBe(2)
    expect(server.putCount()).toBe(2)
    expect(result.current.order.saveFailed).toBe(false)
  })
})

describe('保存が成功したとき（要件23・判断11）', () => {
  test('カード表示の補助 判断11: 1回の並べ替えが成功すると、一覧を1回だけ取り直す。取り直しの応答を保留している間もキャッシュは新しい並びのまま', async () => {
    const { result } = await renderWithLoadedList()

    act(() => result.current.order.move('AAPL', '6758'))
    await waitFor(() => expect(server.putCount()).toBe(1))
    expect(server.puts[0]?.codes).toEqual(['AAPL', '6758', '7203'])
    expect(server.getCount()).toBe(1)

    await act(async () => server.puts[0]?.respond(204))
    await waitFor(() => expect(server.getCount()).toBe(2))
    expect(cachedOrder()).toEqual(['AAPL', '6758', '7203'])

    await act(async () => server.heldGets[0]?.release())
    await flush()
    expect(cachedOrder()).toEqual(['AAPL', '6758', '7203'])
    expect(server.getCount()).toBe(2)
    expect(result.current.order.saveFailed).toBe(false)
  })
})

describe('保存に失敗したとき（要件30）', () => {
  test.each([
    ['HTTP 500', (put: HeldPut) => put.respond(500)],
    ['通信の失敗', (put: HeldPut) => put.fail()],
  ])('カード表示の補助 要件30: 保存が失敗（%s）すると、並べ替える前の並びに戻り、saveFailed が true になり、一覧は取り直さない', async (_name, failPut) => {
    const { result } = await renderWithLoadedList()

    act(() => result.current.order.move('AAPL', '6758'))
    expect(cachedOrder()).toEqual(['AAPL', '6758', '7203'])
    await waitFor(() => expect(server.putCount()).toBe(1))
    expect(result.current.order.saveFailed).toBe(false)

    await act(async () => failPut(server.puts[0]!))

    await waitFor(() => expect(result.current.order.saveFailed).toBe(true))
    expect(cachedOrder()).toEqual(LOADED_ORDER)
    await flush()
    // 失敗のときは取り直さない（取り直しも失敗して画面が取得失敗に変わり、失敗の表示が消えるため）。
    expect(server.getCount()).toBe(1)
    expect(server.putCount()).toBe(1)
  })

  test('カード表示の補助 判断11: 保存が HTTP 400（画面の一覧が古い）で失敗すると、並べ替える前の並びに戻り、saveFailed が true になり、そのあと一覧をちょうど1回取り直す', async () => {
    const { result } = await renderWithLoadedList()

    act(() => result.current.order.move('AAPL', '6758'))
    expect(cachedOrder()).toEqual(['AAPL', '6758', '7203'])
    await waitFor(() => expect(server.putCount()).toBe(1))
    expect(result.current.order.saveFailed).toBe(false)
    expect(server.getCount()).toBe(1)

    await act(async () => server.puts[0]?.respond(400))

    await waitFor(() => expect(result.current.order.saveFailed).toBe(true))
    expect(cachedOrder()).toEqual(LOADED_ORDER)
    // 400 のときは取り直す（開いたまま CSV を取り込んだなど、画面の一覧が古いと毎回 400 になるため）。
    await waitFor(() => expect(server.getCount()).toBe(2))
    await flush()
    expect(server.getCount()).toBe(2)
    expect(server.putCount()).toBe(1)
    // 取り直しの応答が返った後も、失敗の表示は残る（取り直しの成功は、並べ替えの成功ではない）。
    await act(async () => server.heldGets[0]?.release())
    await flush()
    expect(result.current.order.saveFailed).toBe(true)
    expect(server.getCount()).toBe(2)
  })

  test('カード表示の補助 要件30: 保存の失敗の後に並べ替えると、saveFailed が false に戻る', async () => {
    const { result } = await renderWithLoadedList()
    act(() => result.current.order.move('AAPL', '6758'))
    await waitFor(() => expect(server.putCount()).toBe(1))
    await act(async () => server.puts[0]?.respond(500))
    await waitFor(() => expect(result.current.order.saveFailed).toBe(true))
    expect(cachedOrder()).toEqual(LOADED_ORDER)

    // 次の並べ替え（失敗の後の、元の並びからの並べ替え）。
    act(() => result.current.order.move('7203', '6758'))

    expect(result.current.order.saveFailed).toBe(false)
    expect(cachedOrder()).toEqual(['7203', '6758', 'AAPL'])
    await waitFor(() => expect(server.putCount()).toBe(2))
    expect(server.puts[1]?.codes).toEqual(['7203', '6758', 'AAPL'])
  })

  test('カード表示の補助 判断11: 1回目の保存が失敗し、2回目が待っていたときは、2回目は送らずに、最後に保存できた並び（読み込んだときの並び）に戻す', async () => {
    const { result } = await renderWithLoadedList()
    act(() => result.current.order.move('AAPL', '6758'))
    act(() => result.current.order.move('7203', 'AAPL'))
    expect(cachedOrder()).toEqual(['7203', 'AAPL', '6758'])
    await waitFor(() => expect(server.putCount()).toBe(1))

    await act(async () => server.puts[0]?.respond(500))

    await waitFor(() => expect(result.current.order.saveFailed).toBe(true))
    expect(cachedOrder()).toEqual(LOADED_ORDER)
    await flush()
    expect(server.putCount()).toBe(1)
    expect(server.getCount()).toBe(1)
  })

  test('カード表示の補助 判断11: 1回目の保存が成功し、2回目が失敗したときは、1回目の並びに戻る。一覧は取り直さない', async () => {
    const { result } = await renderWithLoadedList()
    act(() => result.current.order.move('AAPL', '6758'))
    act(() => result.current.order.move('7203', 'AAPL'))
    await waitFor(() => expect(server.putCount()).toBe(1))
    await act(async () => server.puts[0]?.respond(204))
    await waitFor(() => expect(server.putCount()).toBe(2))
    expect(server.puts[1]?.codes).toEqual(['7203', 'AAPL', '6758'])

    await act(async () => server.puts[1]?.respond(500))

    await waitFor(() => expect(result.current.order.saveFailed).toBe(true))
    expect(cachedOrder()).toEqual(['AAPL', '6758', '7203'])
    await flush()
    expect(server.getCount()).toBe(1)
  })
})

describe('一覧の取り直しが途中のときの並べ替え（判断11）', () => {
  test('カード表示の補助 判断11: 取り直しが途中のときに並べ替えても、古い取り直しの応答で並びが上書きされない。保存が成功した後の取り直しで、サーバーの並びになる', async () => {
    const { result } = await renderWithLoadedList()
    // 一覧の取り直し（今すぐ更新・削除の後と同じ）が始まる。応答は保留（要求を受けた時点の古い並び LOADED_ORDER で答える）。
    act(() => {
      void queryClient.invalidateQueries({ queryKey: HOLDINGS_QUERY_KEY })
    })
    await waitFor(() => expect(server.getCount()).toBe(2))
    expect(server.heldGets[0]?.order).toEqual(LOADED_ORDER)

    // 取り直しが返る前に並べ替える。
    act(() => result.current.order.move('AAPL', '6758'))
    expect(cachedOrder()).toEqual(['AAPL', '6758', '7203'])
    await waitFor(() => expect(server.putCount()).toBe(1))

    // 古い取り直しの応答が返っても、並びは上書きされない。
    await act(async () => server.heldGets[0]?.release())
    await flush()
    expect(cachedOrder()).toEqual(['AAPL', '6758', '7203'])

    // 保存が成功すると、一覧を取り直す。その応答の前後でも、並びは新しい並びのまま。
    await act(async () => server.puts[0]?.respond(204))
    await waitFor(() => expect(server.getCount()).toBe(3))
    expect(cachedOrder()).toEqual(['AAPL', '6758', '7203'])
    expect(server.heldGets[1]?.order).toEqual(['AAPL', '6758', '7203'])
    await act(async () => server.heldGets[1]?.release())
    await flush()
    expect(cachedOrder()).toEqual(['AAPL', '6758', '7203'])
    expect(server.getCount()).toBe(3)
  })
})
