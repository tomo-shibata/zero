import { afterEach, describe, expect, test, vi } from 'vitest'
import { fetchHoldings } from '../../../src/features/holdings/api/fetchHoldings'

// 仕様: 要件 docs/requirements/stock-holdings.md の FR-22（取得に失敗したらダイアログ）。
// 境界: プラン docs/plans/stock-holdings/plan.md 4.4
//   - 「API」: GET /api/holdings の応答の形。単価は ^[0-9]+\.[0-9]$ の文字列、評価額と合計は円の整数、
//     価格のない行は currentPrice・priceDate・valuation が null、0件は {"holdings":[],"totalValuation":0,"totalExcludesUnpriced":false}。
//   - 「フロント（TypeScript）」の api/fetchHoldings.ts:
//     グローバルの fetch で "/api/holdings" を呼ぶ。2xx 以外と通信の失敗は reject。zod で検証して entities の型（API と同じ形）に変換。
//     検証（外れたら reject → 取得失敗 FR-22）: 単価は ^[0-9]+\.[0-9]$ の文字列。quantity は正の安全な整数（Number.isSafeInteger、判断16）。
//     valuation・totalValuation は0以上の安全な整数。行は「価格あり（3つがそろう）」か「価格なし（3つとも null）」。
//     priceDate は実在する YYYY-MM-DD。holdings は配列（null 不可）。JSON でない本文も reject。
// テスト計画: プラン 5.2 の「補助（4.4 の fetchHoldings の検証、PR④ の EC-2・EC-3・TC-02）」の行。
// mock は境界の外側（ネットワーク = グローバルの fetch）だけに使い、fetchHoldings の結果（resolve した値か reject か）を確かめる。

/** 一覧の API のパス（プラン 4.4）。 */
const HOLDINGS_API_PATH = '/api/holdings'

/** プラン 4.4「API」の応答の例（そのまま）。 */
function apiExampleBody() {
  return {
    holdings: [
      {
        code: '7203',
        name: 'トヨタ自動車',
        quantity: 100,
        acquisitionPrice: '2500.0',
        currentPrice: '3000.0',
        priceDate: '2026-10-06',
        valuation: 300000,
      },
    ],
    totalValuation: 300000,
    totalExcludesUnpriced: false,
  }
}

/** API の例の1行目（価格のある行）を1か所だけ変えた本文を作る。 */
function exampleWithRow(change: (row: Record<string, unknown>) => void): unknown {
  const body = apiExampleBody()
  change(body.holdings[0] as unknown as Record<string, unknown>)
  return body
}

/** API の例の最上位の項目を1か所だけ変えた本文を作る。 */
function exampleWithTop(change: (body: Record<string, unknown>) => void): unknown {
  const body = apiExampleBody()
  change(body as unknown as Record<string, unknown>)
  return body
}

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

/**
 * グローバルの fetch を差し替える。"/api/holdings" を要求されたときだけ makeResponse の応答を返し、
 * ほかのパスには 404 を返す（受け入れのテストで、fetchHoldings が "/api/holdings" を呼んでいることも確かめるため）。
 */
function stubFetch(makeResponse: () => Response): void {
  vi.stubGlobal(
    'fetch',
    vi.fn<typeof fetch>(async (input) =>
      requestedUrl(input) === HOLDINGS_API_PATH
        ? makeResponse()
        : new Response('not found', { status: 404 }),
    ),
  )
}

/** fetchHoldings の結果が resolve か reject かを返す（reject の値の型は仕様にないので問わない）。 */
async function settle(): Promise<'resolved' | 'rejected'> {
  return fetchHoldings().then(
    () => 'resolved' as const,
    () => 'rejected' as const,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('受け入れる応答（プラン 4.4「API」の形）', () => {
  test('補助 FR-22（PR4_EC2・PR4_TC02）: プラン 4.4 の API の例の応答は、同じ形の値で resolve する', async () => {
    stubFetch(() => jsonResponse(apiExampleBody()))

    await expect(fetchHoldings()).resolves.toEqual(apiExampleBody())
  })

  test('補助 FR-22・FR-7（PR4_EC2・PR4_TC02）: 0件の応答（holdings が空の配列、totalValuation が 0）は、同じ形の値で resolve する', async () => {
    const emptyBody = { holdings: [], totalValuation: 0, totalExcludesUnpriced: false }
    stubFetch(() => jsonResponse(emptyBody))

    await expect(fetchHoldings()).resolves.toEqual(emptyBody)
  })

  test('補助 FR-22・FR-10（PR4_EC2・PR4_TC02）: 価格のない行（currentPrice・priceDate・valuation の3つとも null）を含む応答は、同じ形の値で resolve する', async () => {
    // API の例の 7203 の行に、価格のない 9984 の行を足した応答（価格のない行は合計に含めない。FR-6）。
    const body = {
      holdings: [
        ...apiExampleBody().holdings,
        {
          code: '9984',
          name: 'ソフトバンクグループ',
          quantity: 50,
          acquisitionPrice: '4500.0',
          currentPrice: null,
          priceDate: null,
          valuation: null,
        },
      ],
      totalValuation: 300000,
      totalExcludesUnpriced: true,
    }
    stubFetch(() => jsonResponse(body))

    await expect(fetchHoldings()).resolves.toEqual(body)
  })
})

describe('reject する応答（取得失敗 FR-22。PR4_EC2・PR4_EC3・PR4_TC02）', () => {
  const MAX_SAFE_INTEGER_PLUS_ONE = 9007199254740992 // 2^53。Number.isSafeInteger が false になる最小の正の整数（判断16）

  const invalidBodies: { name: string; body: unknown }[] = [
    // 保有数量: 正の安全な整数でなければ reject。
    {
      name: 'quantity が 9007199254740992（安全な整数を超える。判断16）',
      body: exampleWithRow((row) => {
        row.quantity = MAX_SAFE_INTEGER_PLUS_ONE
      }),
    },
    {
      name: 'quantity が 0（正でない）',
      body: exampleWithRow((row) => {
        row.quantity = 0
      }),
    },
    // 評価額・合計評価額: 0以上の安全な整数でなければ reject。
    {
      name: 'valuation が -1（0未満）',
      body: exampleWithRow((row) => {
        row.valuation = -1
      }),
    },
    {
      name: 'totalValuation が 9007199254740992（安全な整数を超える。判断16）',
      body: exampleWithTop((body) => {
        body.totalValuation = MAX_SAFE_INTEGER_PLUS_ONE
      }),
    },
    // 単価（取得価格・現在の価格）: ^[0-9]+\.[0-9]$ の文字列でなければ reject。
    ...(['acquisitionPrice', 'currentPrice'] as const).flatMap((field) => [
      {
        name: `${field} が "2500"（小数第1位がない）`,
        body: exampleWithRow((row) => {
          row[field] = '2500'
        }),
      },
      {
        name: `${field} が "2500.00"（小数が2桁）`,
        body: exampleWithRow((row) => {
          row[field] = '2500.00'
        }),
      },
      {
        name: `${field} が "-1.0"（負）`,
        body: exampleWithRow((row) => {
          row[field] = '-1.0'
        }),
      },
      {
        name: `${field} が数の 2500（文字列でない）`,
        body: exampleWithRow((row) => {
          row[field] = 2500
        }),
      },
    ]),
    // 行の形: 「価格あり（3つがそろう）」か「価格なし（3つとも null）」のどちらでもなければ reject。
    {
      name: 'currentPrice があって priceDate が null の行',
      body: exampleWithRow((row) => {
        row.priceDate = null
      }),
    },
    // 価格の基準日: 実在する YYYY-MM-DD でなければ reject。
    {
      name: 'priceDate が "2026-02-30"（実在しない日付）',
      body: exampleWithRow((row) => {
        row.priceDate = '2026-02-30'
      }),
    },
    // holdings: 配列でなければ（null も）reject。
    {
      name: 'holdings が null',
      body: exampleWithTop((body) => {
        body.holdings = null
      }),
    },
  ]

  test.each(invalidBodies)('$name の 200 の応答は reject する', async ({ body }) => {
    stubFetch(() => jsonResponse(body))

    expect(await settle()).toBe('rejected')
  })

  test('JSON でない本文（200 の HTML）は reject する', async () => {
    stubFetch(
      () =>
        new Response('<!doctype html><html><body>index</body></html>', {
          status: 200,
          headers: { 'Content-Type': 'text/html' },
        }),
    )

    expect(await settle()).toBe('rejected')
  })

  test('HTTP 500（プラン 4.4 の失敗時の本文 {"error":"internal_error"}）は reject する', async () => {
    stubFetch(() => jsonResponse({ error: 'internal_error' }, 500))

    expect(await settle()).toBe('rejected')
  })
})
