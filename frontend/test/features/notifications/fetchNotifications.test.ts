import { afterEach, describe, expect, test, vi } from 'vitest'
import { fetchNotifications } from '../../../src/features/notifications/api/fetchNotifications'
import { fetchUnseenNotificationCount } from '../../../src/features/notifications/api/fetchUnseenNotificationCount'
import { markNotificationsSeen } from '../../../src/features/notifications/api/markNotificationsSeen'
import { fiveNotifications } from './fixtures'

// 仕様: 価格の変動の通知の要件 docs/requirements/price-alerts.md の要件25（見ていない通知の数）・要件34（見たにする）・要件37（取得に失敗したとき）。
// 境界: プラン docs/plans/price-alerts/plan.md 4.5「API の形」（通知の3つ）・4.8 の PR②「features/notifications/api」
//   - fetchNotifications(): Promise<PriceAlertNotificationList>  GET /api/notifications。失敗と形の誤りは throw（reject）
//   - fetchUnseenNotificationCount(): Promise<number>  GET /api/notifications/unseen-count → { "count": 3 }
//   - markNotificationsSeen(upToId: string): Promise<void>  POST /api/notifications/seen、本文 { "upToId": "15" } → 204
//   API の境界で形を確かめて内部に渡す（フロントのルールの5）。
// テスト計画: プラン 5.2 の fetchNotifications.test.ts の行（形の誤り: kind が知らない値、quantity が安全な整数でない、items が null で throw）。
// mock は境界の外側（ネットワーク = グローバルの fetch）だけに使い、結果（resolve した値か reject か）を確かめる。

const NOTIFICATIONS_API_PATH = '/api/notifications'
const UNSEEN_COUNT_API_PATH = '/api/notifications/unseen-count'
const SEEN_API_PATH = '/api/notifications/seen'

afterEach(() => {
  vi.unstubAllGlobals()
})

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

function stubFetch(response: () => Promise<Response>) {
  const fetchMock = vi.fn<typeof fetch>(response)
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

/** 5件の通知の本文を、1か所だけ変えて返す。 */
function bodyWithFirstItem(change: (item: Record<string, unknown>) => void): unknown {
  const body = JSON.parse(JSON.stringify(fiveNotifications())) as { items: Record<string, unknown>[] }
  change(body.items[0])
  return body
}

describe('fetchNotifications（要件37）', () => {
  test('GET /api/notifications を呼び、通知の一覧と latestId を返す（API の形のとおり）', async () => {
    const body = fiveNotifications()
    const fetchMock = stubFetch(() => Promise.resolve(jsonResponse(body)))

    const result = await fetchNotifications()

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(String(fetchMock.mock.calls[0][0])).toBe(NOTIFICATIONS_API_PATH)
    expect(result).toEqual(body)
  })

  test('0件の応答（items が空で latestId が null）を返す', async () => {
    stubFetch(() => Promise.resolve(jsonResponse({ items: [], latestId: null })))

    await expect(fetchNotifications()).resolves.toEqual({ items: [], latestId: null })
  })

  test('通信の失敗は reject する', async () => {
    stubFetch(() => Promise.reject(new TypeError('Failed to fetch')))

    await expect(fetchNotifications()).rejects.toThrow()
  })

  test('2xx 以外の応答（500）は reject する', async () => {
    stubFetch(() => Promise.resolve(jsonResponse({ error: 'internal_error' }, 500)))

    await expect(fetchNotifications()).rejects.toThrow()
  })

  test('JSON でない本文は reject する', async () => {
    stubFetch(() => Promise.resolve(new Response('not json', { status: 200 })))

    await expect(fetchNotifications()).rejects.toThrow()
  })

  test('形の誤り: items が null のとき reject する', async () => {
    stubFetch(() => Promise.resolve(jsonResponse({ items: null, latestId: null })))

    await expect(fetchNotifications()).rejects.toThrow()
  })

  test('形の誤り: 条件の kind が知らない値のとき reject する', async () => {
    const body = bodyWithFirstItem((item) => {
      item.condition = { ...(item.condition as object), kind: 'unknown' }
    })
    stubFetch(() => Promise.resolve(jsonResponse(body)))

    await expect(fetchNotifications()).rejects.toThrow()
  })

  test('形の誤り: quantity が安全な整数でないとき（2^53）reject する', async () => {
    const body = bodyWithFirstItem((item) => {
      item.quantity = 2 ** 53
    })
    stubFetch(() => Promise.resolve(jsonResponse(body)))

    await expect(fetchNotifications()).rejects.toThrow()
  })

  test('形の誤り: quantity が小数のとき reject する', async () => {
    const body = bodyWithFirstItem((item) => {
      item.quantity = 1.5
    })
    stubFetch(() => Promise.resolve(jsonResponse(body)))

    await expect(fetchNotifications()).rejects.toThrow()
  })

  test('形の誤り: 通貨が知らない値（EUR）のとき reject する', async () => {
    const body = bodyWithFirstItem((item) => {
      item.currency = 'EUR'
    })
    stubFetch(() => Promise.resolve(jsonResponse(body)))

    await expect(fetchNotifications()).rejects.toThrow()
  })

  test('形の誤り: isNew が真偽値でないとき reject する', async () => {
    const body = bodyWithFirstItem((item) => {
      item.isNew = 'true'
    })
    stubFetch(() => Promise.resolve(jsonResponse(body)))

    await expect(fetchNotifications()).rejects.toThrow()
  })

  test('形の誤り: latestId が文字列でも null でもないとき reject する', async () => {
    stubFetch(() => Promise.resolve(jsonResponse({ items: [], latestId: 15 })))

    await expect(fetchNotifications()).rejects.toThrow()
  })
})

describe('fetchUnseenNotificationCount（要件25・要件27）', () => {
  test('GET /api/notifications/unseen-count を呼び、count を数で返す', async () => {
    const fetchMock = stubFetch(() => Promise.resolve(jsonResponse({ count: 3 })))

    await expect(fetchUnseenNotificationCount()).resolves.toBe(3)

    expect(String(fetchMock.mock.calls[0][0])).toBe(UNSEEN_COUNT_API_PATH)
  })

  test('0 を返す', async () => {
    stubFetch(() => Promise.resolve(jsonResponse({ count: 0 })))

    await expect(fetchUnseenNotificationCount()).resolves.toBe(0)
  })

  test('通信の失敗と 2xx 以外（500）は reject する', async () => {
    stubFetch(() => Promise.reject(new TypeError('Failed to fetch')))
    await expect(fetchUnseenNotificationCount()).rejects.toThrow()

    stubFetch(() => Promise.resolve(jsonResponse({ error: 'internal_error' }, 500)))
    await expect(fetchUnseenNotificationCount()).rejects.toThrow()
  })

  test('形の誤り: count が文字列・欠けている・小数のとき reject する', async () => {
    for (const body of [{ count: '3' }, {}, { count: 1.5 }]) {
      stubFetch(() => Promise.resolve(jsonResponse(body)))

      await expect(fetchUnseenNotificationCount()).rejects.toThrow()
    }
  })
})

describe('markNotificationsSeen（要件34）', () => {
  test('POST /api/notifications/seen を、本文 {"upToId":"15"} で呼び、204 で resolve する', async () => {
    const fetchMock = stubFetch(() => Promise.resolve(new Response(null, { status: 204 })))

    await expect(markNotificationsSeen('15')).resolves.toBeUndefined()

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [input, init] = fetchMock.mock.calls[0]
    expect(String(input)).toBe(SEEN_API_PATH)
    expect(init?.method).toBe('POST')
    expect(JSON.parse(String(init?.body))).toEqual({ upToId: '15' })
  })

  test('2xx 以外の応答（500）と通信の失敗は reject する', async () => {
    stubFetch(() => Promise.resolve(jsonResponse({ error: 'internal_error' }, 500)))
    await expect(markNotificationsSeen('15')).rejects.toThrow()

    stubFetch(() => Promise.reject(new TypeError('Failed to fetch')))
    await expect(markNotificationsSeen('15')).rejects.toThrow()
  })
})
