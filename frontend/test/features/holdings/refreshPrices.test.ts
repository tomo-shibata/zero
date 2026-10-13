import { afterEach, describe, expect, test, vi } from 'vitest'
import { refreshPrices } from '../../../src/features/holdings/api/refreshPrices'

// 仕様: 要件 docs/requirements/price-update.md の要件16・要件18・要件19（今すぐ更新の要求が失敗したときの扱い）。
// 境界: プラン docs/plans/price-update/plan.md 4.4
//   - 「API」: POST /api/price-refresh（本文なし）→ 200 {"saved":["7203"],"failed":["9984"]}（どちらも銘柄コードの昇順。0件でも []）。
//   - 「フロント（TypeScript）」の api/refreshPrices.ts: refreshPrices(): Promise<PriceRefreshResult>
//     2xx 以外と通信の失敗は reject。zod で検証。
// テスト計画: プラン 5.2 の PR① の行
//   「refreshPrices: 受け入れる応答と、reject する応答（saved が null、要素が数、failed がない、500、200 の HTML の本文、通信の失敗）」。
// mock は境界の外側（ネットワーク = グローバルの fetch）だけに使い、refreshPrices の結果（resolve した値か reject か）を確かめる。

/** 今すぐ更新の API のパス（プラン 4.4）。 */
const PRICE_REFRESH_API_PATH = '/api/price-refresh'

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

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
 * グローバルの fetch を差し替える。POST "/api/price-refresh" を要求されたときだけ makeResponse の応答を返し、
 * それ以外の要求には 404 を返す（refreshPrices が、このパスをこのメソッドで呼ぶことも確かめるため）。
 */
function stubFetch(makeResponse: () => Response): void {
  vi.stubGlobal(
    'fetch',
    vi.fn<typeof fetch>(async (input, init) =>
      requestedUrl(input) === PRICE_REFRESH_API_PATH && requestedMethod(input, init) === 'POST'
        ? makeResponse()
        : new Response('not found', { status: 404 }),
    ),
  )
}

/** refreshPrices の結果が resolve か reject かを返す（reject の値の型は仕様にないので問わない）。 */
async function settle(): Promise<'resolved' | 'rejected'> {
  return refreshPrices().then(
    () => 'resolved' as const,
    () => 'rejected' as const,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('受け入れる応答（プラン 4.4「API」の形）', () => {
  test('補助 4.4: POST /api/price-refresh を呼び、200 {"saved":["7203"],"failed":["9984"]} は同じ形の値で resolve する', async () => {
    // stubFetch は POST /api/price-refresh 以外の要求に 404 を返すので、resolve することが、このパスをこのメソッドで呼んだことの確かめになる。
    stubFetch(() => jsonResponse({ saved: ['7203'], failed: ['9984'] }))

    await expect(refreshPrices()).resolves.toEqual({ saved: ['7203'], failed: ['9984'] })
  })

  test('補助 4.4: 0件の応答 {"saved":[],"failed":[]} は、空の配列の値で resolve する', async () => {
    stubFetch(() => jsonResponse({ saved: [], failed: [] }))

    await expect(refreshPrices()).resolves.toEqual({ saved: [], failed: [] })
  })

  test('補助 4.4: 保存だけ（failed が空）、失敗だけ（saved が空）の応答も、同じ形の値で resolve する', async () => {
    stubFetch(() => jsonResponse({ saved: ['6758', '7203'], failed: [] }))
    await expect(refreshPrices()).resolves.toEqual({ saved: ['6758', '7203'], failed: [] })

    stubFetch(() => jsonResponse({ saved: [], failed: ['1301', '9984'] }))
    await expect(refreshPrices()).resolves.toEqual({ saved: [], failed: ['1301', '9984'] })
  })
})

describe('reject する応答（要件19: 更新の要求そのものの失敗）', () => {
  const invalidBodies: { name: string; body: unknown }[] = [
    { name: 'saved が null', body: { saved: null, failed: [] } },
    { name: 'failed が null', body: { saved: [], failed: null } },
    { name: 'saved の要素が数（7203 の数）', body: { saved: [7203], failed: [] } },
    { name: 'failed の要素が数（9984 の数）', body: { saved: [], failed: [9984] } },
    { name: 'failed がない', body: { saved: ['7203'] } },
    { name: 'saved がない', body: { failed: ['9984'] } },
    { name: '空のオブジェクト', body: {} },
    { name: 'saved が配列でなく文字列', body: { saved: '7203', failed: [] } },
  ]

  test.each(invalidBodies)('補助 4.4: 200 でも $name の応答は reject する', async ({ body }) => {
    stubFetch(() => jsonResponse(body))

    expect(await settle()).toBe('rejected')
  })

  test('補助 4.4: HTTP 500（本文はエラーのコード）の応答は reject する', async () => {
    stubFetch(() => jsonResponse({ error: 'internal_error' }, 500))

    expect(await settle()).toBe('rejected')
  })

  test('補助 4.4: HTTP 500 でも、本文が正しい形（saved と failed が配列）なら reject する（2xx 以外は reject）', async () => {
    stubFetch(() => jsonResponse({ saved: ['7203'], failed: [] }, 500))

    expect(await settle()).toBe('rejected')
  })

  test('補助 4.4: HTTP 403（別のサイトからの書き込みとして拒否された）の応答は reject する', async () => {
    stubFetch(() => jsonResponse({ error: 'forbidden' }, 403))

    expect(await settle()).toBe('rejected')
  })

  test('補助 4.4: 200 でも本文が HTML（JSON でない）の応答は reject する', async () => {
    stubFetch(
      () =>
        new Response('<!doctype html><html><body>not json</body></html>', {
          status: 200,
          headers: { 'Content-Type': 'text/html' },
        }),
    )

    expect(await settle()).toBe('rejected')
  })

  test('補助 4.4: 通信の失敗（fetch が reject する）は reject する', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof fetch>(() => Promise.reject(new TypeError('Failed to fetch'))),
    )

    expect(await settle()).toBe('rejected')
  })
})
