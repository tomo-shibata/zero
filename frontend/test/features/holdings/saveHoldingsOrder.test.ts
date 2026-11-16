import { afterEach, describe, expect, test, vi } from 'vitest'
import { saveHoldingsOrder } from '../../../src/features/holdings/api/saveHoldingsOrder'

// 仕様: カード表示の要件 docs/requirements/holdings-cards.md の要件23・要件30（並びを保存する。保存に失敗したときは画面が失敗として扱う）。
// 境界: プラン docs/plans/holdings-cards/plan.md 4.3「保存する」・4.6 PR②・判断14
//   saveHoldingsOrder(codes: readonly string[]): Promise<void>
//   PUT /api/holding-order、本文 {"codes": codes}、Content-Type: application/json。
//   204 以外・通信の失敗・10秒たっても応答がないときは reject する（10秒で要求を中断する）。
// テスト計画: プラン 5.3 PR② の「saveHoldingsOrder: 本文と method・Content-Type、204 で resolve、400・500 と通信の失敗で reject、10秒で中断して reject」。
// mock は境界の外側（ネットワーク = グローバルの fetch）だけに使う。

type Requested = { url: string; method: string; contentType: string | null; body: unknown }

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

/** 送った本文を JSON として読む（本文がなければ undefined）。 */
function requestedBody(init?: RequestInit): unknown {
  if (typeof init?.body !== 'string') return undefined
  return JSON.parse(init.body)
}

/**
 * グローバルの fetch を差し替える。どんな要求にも makeResponse の応答を返し、届いた要求（URL・メソッド・Content-Type・本文）を requests に残す。
 */
function stubFetch(makeResponse: () => Response): Requested[] {
  const requests: Requested[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn<typeof fetch>(async (input, init) => {
      requests.push({
        url: requestedUrl(input),
        method: requestedMethod(input, init),
        contentType: new Headers(init?.headers).get('Content-Type'),
        body: requestedBody(init),
      })
      return makeResponse()
    }),
  )
  return requests
}

/** saveHoldingsOrder の結果が resolve か reject かを返す。 */
async function settle(codes: readonly string[]): Promise<'resolved' | 'rejected'> {
  return saveHoldingsOrder(codes).then(
    () => 'resolved',
    () => 'rejected',
  )
}

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('要求と、resolve する応答', () => {
  test('カード表示の補助 4.6: PUT /api/holding-order に、本文 {"codes":[…]}（並びの順）を Content-Type: application/json で送り、204 で resolve する', async () => {
    const requests = stubFetch(() => new Response(null, { status: 204 }))

    await expect(saveHoldingsOrder(['AAPL', '7203', '6758'])).resolves.toBeUndefined()

    expect(requests).toEqual([
      {
        url: '/api/holding-order',
        method: 'PUT',
        contentType: 'application/json',
        body: { codes: ['AAPL', '7203', '6758'] },
      },
    ])
  })

  test('カード表示の補助 4.6: 本文の codes は渡した順のまま（並べ替えたり、重複を除いたりしない）', async () => {
    const requests = stubFetch(() => new Response(null, { status: 204 }))

    await saveHoldingsOrder(['9984', '6758', 'AAPL', '7203'])

    expect(requests[0]?.body).toEqual({ codes: ['9984', '6758', 'AAPL', '7203'] })
  })
})

describe('reject する応答（要件30: 保存の失敗）', () => {
  test('カード表示の補助 4.3: HTTP 400（{"error":"bad_request"}）は reject する', async () => {
    stubFetch(() => jsonResponse(400, { error: 'bad_request' }))

    expect(await settle(['AAPL', '7203', '6758'])).toBe('rejected')
  })

  test('カード表示の補助 4.3: HTTP 500（{"error":"internal_error"}）は reject する', async () => {
    stubFetch(() => jsonResponse(500, { error: 'internal_error' }))

    expect(await settle(['AAPL', '7203', '6758'])).toBe('rejected')
  })

  test('カード表示の補助 4.3: HTTP 403（別のオリジンとして拒否された）は reject する', async () => {
    stubFetch(() => jsonResponse(403, { error: 'forbidden' }))

    expect(await settle(['AAPL', '7203', '6758'])).toBe('rejected')
  })

  test('カード表示の補助 4.6: 204 以外の成功（200）も reject する', async () => {
    stubFetch(() => jsonResponse(200, {}))

    expect(await settle(['AAPL', '7203', '6758'])).toBe('rejected')
  })

  test('カード表示の補助 4.6: 通信の失敗（fetch が reject する）は reject する', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof fetch>(() => Promise.reject(new TypeError('Failed to fetch'))),
    )

    expect(await settle(['AAPL', '7203', '6758'])).toBe('rejected')
  })
})

describe('10秒で中断する（判断14: 応答が返らないときに、保存の待ちが止まり続けないため）', () => {
  const TIMEOUT_MS = 10_000

  /** 応答が返らない fetch。渡された signal が中断されたときだけ reject する（本物の fetch と同じ。ネットワークの代わりの fake）。 */
  function stubNeverRespondingFetch(): void {
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof fetch>(
        (_input, init) =>
          new Promise<Response>((_resolve, reject) => {
            init?.signal?.addEventListener('abort', () => {
              reject(new DOMException('The operation was aborted.', 'AbortError'))
            })
          }),
      ),
    )
  }

  test('カード表示の補助 判断14: 応答が返らないとき、9,999ms たってもまだ決まらず、10,000ms で reject する', async () => {
    vi.useFakeTimers()
    stubNeverRespondingFetch()
    let state: 'pending' | 'resolved' | 'rejected' = 'pending'
    void saveHoldingsOrder(['AAPL', '7203', '6758']).then(
      () => {
        state = 'resolved'
      },
      () => {
        state = 'rejected'
      },
    )

    await vi.advanceTimersByTimeAsync(TIMEOUT_MS - 1)
    expect(state).toBe('pending')

    await vi.advanceTimersByTimeAsync(1)
    expect(state).toBe('rejected')
  })

  test('カード表示の補助 判断14: 応答が10秒より前に返れば、その後に時計を進めても結果は変わらない（resolve のまま）', async () => {
    vi.useFakeTimers()
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof fetch>(async () => new Response(null, { status: 204 })),
    )
    let state: 'pending' | 'resolved' | 'rejected' = 'pending'
    void saveHoldingsOrder(['AAPL', '7203', '6758']).then(
      () => {
        state = 'resolved'
      },
      () => {
        state = 'rejected'
      },
    )

    await vi.advanceTimersByTimeAsync(1)
    expect(state).toBe('resolved')
    await vi.advanceTimersByTimeAsync(TIMEOUT_MS)
    expect(state).toBe('resolved')
  })
})
