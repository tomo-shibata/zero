import { afterEach, describe, expect, test, vi } from 'vitest'
import { deleteManagedAccount } from '../../../src/features/holdings/api/deleteManagedAccount'

// 仕様: おまかせ運用の要件 docs/requirements/managed-account.md の要件24（削除の要求。失敗したら画面は削除の失敗として扱う）。
// 境界: プラン docs/plans/managed-account/plan.md 4.8 の PR③
//   deleteManagedAccount(name: string): Promise<void>（features/holdings/api/deleteManagedAccount.ts）。
//   口座 name を削除する。204 以外・通信の失敗・10秒たっても応答がないときは reject する（deleteHolding と同じ）。
// テスト計画: プラン 5.2 の PR③ の deleteManagedAccount.test.ts の行
//   「fetch に渡した URL の pathname が /api/managed-accounts、searchParams.get('name') が渡した名前（ロボアド B・a&b=c・50%）と同じ、
//   method が DELETE。204 で resolve、404・500・通信の失敗・10秒の時間切れで reject」。
// mock は境界の外側（ネットワーク = グローバルの fetch）だけに使い、結果（resolve した値か reject か）と、送った要求（メソッドと URL）を確かめる。

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

/** 届いた要求の URL の文字列を、パスとクエリに分けて読む（相対のパスでも読めるように、起点を付ける）。 */
function parsed(url: string): URL {
  return new URL(url, 'http://localhost')
}

/** グローバルの fetch を差し替える。どんな要求にも makeResponse の応答を返し、届いた要求を requests に残す。 */
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

/** deleteManagedAccount の結果が resolve か reject かを返す（reject の値の型は仕様にないので問わない）。 */
async function settle(name: string): Promise<'resolved' | 'rejected'> {
  return deleteManagedAccount(name).then(
    () => 'resolved' as const,
    () => 'rejected' as const,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('成功（204）', () => {
  test('補助（要件24）: WealthNavi を渡すと DELETE /api/managed-accounts?name=WealthNavi を送り、204 で resolve する（値は返さない）', async () => {
    const requests = stubFetch(() => new Response(null, { status: 204 }))

    await expect(deleteManagedAccount('WealthNavi')).resolves.toBeUndefined()

    expect(requests).toHaveLength(1)
    expect(requests[0].method).toBe('DELETE')
    const url = parsed(requests[0].url)
    expect(url.pathname).toBe('/api/managed-accounts')
    expect(url.searchParams.get('name')).toBe('WealthNavi')
  })

  // 名前はクエリに入れる。「&」「%」「空白」「日本語」を含んでも、サーバーが同じ名前に戻せる形で送る（プラン 5.2 の PR②、判断3）。
  test.each(['ロボアド B', 'a&b=c', '50%', 'x/y', 'a+b', 'a#b', ' WealthNavi', 'WealthNavi '])(
    '補助（要件24）: 名前 %j は、クエリの name を読むと同じ文字列に戻る（ほかのクエリ項目は増えない）',
    async (name) => {
      const requests = stubFetch(() => new Response(null, { status: 204 }))

      await expect(deleteManagedAccount(name)).resolves.toBeUndefined()

      expect(requests).toHaveLength(1)
      expect(requests[0].method).toBe('DELETE')
      const url = parsed(requests[0].url)
      expect(url.pathname).toBe('/api/managed-accounts')
      expect(url.searchParams.get('name')).toBe(name)
      expect([...url.searchParams.keys()]).toEqual(['name'])
    },
  )
})

describe('reject する応答（要件24: 削除の要求そのものの失敗）', () => {
  test('補助（要件24）: HTTP 404（持っていない口座。{"error":"not_found"}）は reject する', async () => {
    stubFetch(
      () =>
        new Response(JSON.stringify({ error: 'not_found' }), {
          status: 404,
          headers: { 'Content-Type': 'application/json' },
        }),
    )

    expect(await settle('WealthNavi')).toBe('rejected')
  })

  test('補助（要件24）: HTTP 500（{"error":"internal_error"}）は reject する', async () => {
    stubFetch(
      () =>
        new Response(JSON.stringify({ error: 'internal_error' }), {
          status: 500,
          headers: { 'Content-Type': 'application/json' },
        }),
    )

    expect(await settle('WealthNavi')).toBe('rejected')
  })

  test('補助（要件24）: HTTP 403（別のサイトからの書き込みとして拒否された。{"error":"forbidden"}）は reject する', async () => {
    stubFetch(
      () =>
        new Response(JSON.stringify({ error: 'forbidden' }), {
          status: 403,
          headers: { 'Content-Type': 'application/json' },
        }),
    )

    expect(await settle('WealthNavi')).toBe('rejected')
  })

  test('補助（要件24）: 204 以外の成功（200 の本文あり）も reject する', async () => {
    stubFetch(
      () =>
        new Response(JSON.stringify({}), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
    )

    expect(await settle('WealthNavi')).toBe('rejected')
  })

  test('補助（要件24）: 通信の失敗（fetch が reject する）は reject する', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof fetch>(() => Promise.reject(new TypeError('Failed to fetch'))),
    )

    expect(await settle('WealthNavi')).toBe('rejected')
  })
})

describe('10秒で中断する（deleteHolding と同じ。応答が返らずに確認のダイアログから出られなくならないため）', () => {
  const TIMEOUT_MS = 10_000

  afterEach(() => {
    vi.useRealTimers()
  })

  /** 応答が返らない fetch は、渡された signal が中断されたときだけ reject する（本物の fetch と同じ）。 */
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

  test('補助（要件24）: 応答が返らないとき、9,999ms たってもまだ決まらず、10,000ms で reject する', async () => {
    vi.useFakeTimers()
    stubNeverRespondingFetch()
    let state: 'pending' | 'resolved' | 'rejected' = 'pending'
    void deleteManagedAccount('WealthNavi').then(
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

  test('補助（要件24）: 応答が10秒より前に返れば、その後に時計を進めても結果は変わらない（resolve のまま）で、タイマーも残らない', async () => {
    vi.useFakeTimers()
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof fetch>(async () => new Response(null, { status: 204 })),
    )

    await deleteManagedAccount('WealthNavi')

    expect(vi.getTimerCount()).toBe(0)
    await vi.advanceTimersByTimeAsync(TIMEOUT_MS)
  })

  test('補助（要件24）: HTTP 500 で reject した後も、10秒のタイマーが残っていない（タイマーが 0 個）', async () => {
    vi.useFakeTimers()
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof fetch>(async () => new Response(null, { status: 500 })),
    )

    await expect(deleteManagedAccount('WealthNavi')).rejects.toThrow()

    expect(vi.getTimerCount()).toBe(0)
  })
})
