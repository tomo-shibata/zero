import { afterEach, describe, expect, test, vi } from 'vitest'
import { deleteHolding } from '../../../src/features/holdings/api/deleteHolding'

// 仕様: 要件 docs/requirements/price-update.md の要件14・要件15（銘柄の削除と、削除の要求が失敗したときの扱い）。
// 境界: プラン docs/plans/price-update/plan.md 4.4
//   - 「API」: DELETE /api/holdings/{code}（本文なし）→ 204。保有していない銘柄 → 404。
//   - 「フロント（TypeScript）」の api/deleteHolding.ts: deleteHolding(code: string): Promise<void>
//     204 以外と通信の失敗は reject。銘柄コードは encodeURIComponent してパスに入れる。
// テスト計画: プラン 5.2 の PR② の行
//   「deleteHolding: DELETE /api/holdings/7203、銘柄コード "A/B" はパスで "A%2FB"、204 → resolve、404・500・通信の失敗 → reject」。
// mock は境界の外側（ネットワーク = グローバルの fetch）だけに使い、deleteHolding の結果（resolve した値か reject か）と、
// 送った要求（メソッドとパス）を確かめる。

type Requested = { url: string; method: string }

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

/**
 * グローバルの fetch を差し替える。どんな要求にも makeResponse の応答を返し、届いた要求（URL とメソッド）を requests に残す。
 * 要求の URL とメソッドは、deleteHolding を呼んだ後に requests で確かめる。
 */
function stubFetch(makeResponse: () => Response): Requested[] {
  const requests: Requested[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn<typeof fetch>(async (input, init) => {
      requests.push({ url: requestedUrl(input), method: requestedMethod(input, init) })
      return makeResponse()
    }),
  )
  return requests
}

/** deleteHolding の結果が resolve か reject かを返す（reject の値の型は仕様にないので問わない）。 */
async function settle(code: string): Promise<'resolved' | 'rejected'> {
  return deleteHolding(code).then(
    () => 'resolved' as const,
    () => 'rejected' as const,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('成功（204）', () => {
  test('補助 4.4: 7203 を渡すと DELETE /api/holdings/7203 を送り、204 で resolve する（値は返さない）', async () => {
    const requests = stubFetch(() => new Response(null, { status: 204 }))

    await expect(deleteHolding('7203')).resolves.toBeUndefined()

    expect(requests).toEqual([{ url: '/api/holdings/7203', method: 'DELETE' }])
  })

  test('補助 4.4: 銘柄コード "A/B" は encodeURIComponent してパスに入れ、DELETE /api/holdings/A%2FB を送る', async () => {
    const requests = stubFetch(() => new Response(null, { status: 204 }))

    await expect(deleteHolding('A/B')).resolves.toBeUndefined()

    expect(requests).toEqual([{ url: '/api/holdings/A%2FB', method: 'DELETE' }])
  })

  test('補助 4.4: 銘柄コード "A%41" は DELETE /api/holdings/A%2541 を送る（"%" も戻される形にする）', async () => {
    const requests = stubFetch(() => new Response(null, { status: 204 }))

    await expect(deleteHolding('A%41')).resolves.toBeUndefined()

    expect(requests).toEqual([{ url: '/api/holdings/A%2541', method: 'DELETE' }])
  })
})

describe('reject する応答（要件15: 削除の要求そのものの失敗）', () => {
  test('補助 4.4: HTTP 404（保有していない銘柄。{"error":"not_found"}）は reject する', async () => {
    stubFetch(
      () =>
        new Response(JSON.stringify({ error: 'not_found' }), {
          status: 404,
          headers: { 'Content-Type': 'application/json' },
        }),
    )

    expect(await settle('7203')).toBe('rejected')
  })

  test('補助 4.4: HTTP 500（{"error":"internal_error"}）は reject する', async () => {
    stubFetch(
      () =>
        new Response(JSON.stringify({ error: 'internal_error' }), {
          status: 500,
          headers: { 'Content-Type': 'application/json' },
        }),
    )

    expect(await settle('7203')).toBe('rejected')
  })

  test('補助 4.4: HTTP 403（別のサイトからの書き込みとして拒否された。{"error":"forbidden"}）は reject する', async () => {
    stubFetch(
      () =>
        new Response(JSON.stringify({ error: 'forbidden' }), {
          status: 403,
          headers: { 'Content-Type': 'application/json' },
        }),
    )

    expect(await settle('7203')).toBe('rejected')
  })

  test('補助 4.4: 204 以外の成功（200 の本文あり）も reject する（204 以外は reject）', async () => {
    stubFetch(
      () =>
        new Response(JSON.stringify({}), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
    )

    expect(await settle('7203')).toBe('rejected')
  })

  test('補助 4.4: 通信の失敗（fetch が reject する）は reject する', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof fetch>(() => Promise.reject(new TypeError('Failed to fetch'))),
    )

    expect(await settle('7203')).toBe('rejected')
  })
})

describe('10秒で中断する（判断37: 応答が返らずに確認のダイアログから出られなくならないため）', () => {
  // 仕様: プラン 4.4 の deleteHolding「10秒たっても応答がなければ、要求を中断して reject（時計を進めて確かめるので、
  // setTimeout と AbortController で中断する）」。
  // 応答が返らない fetch は、渡された signal が中断されたときだけ reject する（本物の fetch と同じ。ネットワークの代わりの fake）。
  const TIMEOUT_MS = 10_000

  afterEach(() => {
    vi.useRealTimers()
  })

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

  test('判断37: 応答が返らないとき、9,999ms たってもまだ決まらず、10,000ms で reject する', async () => {
    vi.useFakeTimers()
    stubNeverRespondingFetch()
    let state: 'pending' | 'resolved' | 'rejected' = 'pending'
    void deleteHolding('7203').then(
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

  test('判断37: 応答が10秒より前に返れば、その後に時計を進めても結果は変わらない（resolve のまま）', async () => {
    vi.useFakeTimers()
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof fetch>(async () => new Response(null, { status: 204 })),
    )
    let state: 'pending' | 'resolved' | 'rejected' = 'pending'
    void deleteHolding('7203').then(
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

describe('中断の信号とタイマーの後始末（判断37・ステップ6 2ラウンド目のテストの網羅の指摘6・境界値と異常系の指摘5）', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  test('判断37: fetch に渡された signal は、9,999ms たってもまだ中断されておらず、10,000ms で中断される', async () => {
    vi.useFakeTimers()
    let receivedSignal: AbortSignal | null | undefined
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof fetch>((_input, init) => {
        receivedSignal = init?.signal
        return new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener('abort', () => {
            reject(new DOMException('The operation was aborted.', 'AbortError'))
          })
        })
      }),
    )
    const settled = deleteHolding('7203').catch(() => 'rejected')

    expect(receivedSignal).toBeInstanceOf(AbortSignal)
    await vi.advanceTimersByTimeAsync(9_999)
    expect(receivedSignal?.aborted).toBe(false)

    await vi.advanceTimersByTimeAsync(1)
    expect(receivedSignal?.aborted).toBe(true)
    await expect(settled).resolves.toBe('rejected')
  })

  test('判断37: 204 で resolve した後は、10秒のタイマーが残っていない（タイマーが 0 個）', async () => {
    vi.useFakeTimers()
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof fetch>(async () => new Response(null, { status: 204 })),
    )

    await deleteHolding('7203')

    expect(vi.getTimerCount()).toBe(0)
  })

  test('判断37: HTTP 500 で reject した後も、10秒のタイマーが残っていない（タイマーが 0 個）', async () => {
    vi.useFakeTimers()
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof fetch>(async () => new Response(null, { status: 500 })),
    )

    await expect(deleteHolding('7203')).rejects.toThrow()

    expect(vi.getTimerCount()).toBe(0)
  })

  test('判断37: 通信の失敗（fetch が reject）で reject した後も、10秒のタイマーが残っていない（タイマーが 0 個）', async () => {
    vi.useFakeTimers()
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof fetch>(() => Promise.reject(new TypeError('Failed to fetch'))),
    )

    await expect(deleteHolding('7203')).rejects.toThrow()

    expect(vi.getTimerCount()).toBe(0)
  })
})
