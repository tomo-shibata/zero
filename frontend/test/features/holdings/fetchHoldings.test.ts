import { afterEach, describe, expect, test, vi } from 'vitest'
import { fetchHoldings } from '../../../src/features/holdings/api/fetchHoldings'

// 仕様: 要件 docs/requirements/stock-holdings.md の FR-22（取得に失敗したらダイアログ）。
//   売買履歴の要件 docs/requirements/trade-history.md の要件1・要件12・要件14（通貨と、通貨ごとの合計評価額）。
// 境界: プラン docs/plans/trade-history/plan.md 4.3・4.6（PR②）の「API」と「画面」
//   - GET /api/holdings の応答の形: 行に currency（"JPY" か "USD"）。単価は円 ^[0-9]+\.[0-9]$・ドル ^[0-9]+\.[0-9]{2}$ の文字列、
//     評価額は円 ^[0-9]+$・ドル ^[0-9]+\.[0-9]{2}$ の文字列。価格のない行は currentPrice・priceDate・valuation が null。
//     合計は通貨ごとの配列 totals（{ currency, valuation（評価額と同じ形の文字列）, excludesUnpriced }）。0件は {"holdings":[],"totals":[]}。
//   - api/fetchHoldings.ts: グローバルの fetch で "/api/holdings" を呼ぶ。2xx 以外と通信の失敗は reject。zod で検証して entities の型
//     （Holding・CurrencyTotal・HoldingsList。API と同じ形）に変換。API の境界の zod で通貨ごとの形を確かめる。
//     検証（外れたら reject → 取得失敗 FR-22）: 通貨は "JPY" か "USD"（知らない通貨は reject）。円の単価に小数2桁・ドルの単価に小数1桁は reject。
//     quantity は正の安全な整数（Number.isSafeInteger、判断16）。行は「価格あり（3つがそろう）」か「価格なし（3つとも null）」。
//     priceDate は実在する YYYY-MM-DD。holdings・totals は配列（null 不可）。JSON でない本文も reject。
// テスト計画: プラン 5.5（PR②）の「5.2b 既存のテストの直し」（fetchHoldings.test.ts は totals の形にする）と、5.3 の
//   「fetchHoldings の zod（単体）: 円の単価に小数2桁、ドルの単価に小数1桁、知らない通貨は reject」。
// mock は境界の外側（ネットワーク = グローバルの fetch）だけに使い、fetchHoldings の結果（resolve した値か reject か）を確かめる。

/** 一覧の API のパス（プラン 4.4）。 */
const HOLDINGS_API_PATH = '/api/holdings'

/** 売買履歴のプラン 4.3 の API の例（そのまま）。7203（円・価格あり）と AAPL（ドル・価格あり）。 */
function apiExampleBody() {
  return {
    holdings: [
      {
        code: '7203',
        name: 'トヨタ自動車',
        currency: 'JPY',
        quantity: 150,
        acquisitionPrice: '2500.0',
        currentPrice: '3000.0',
        priceDate: '2026-10-06',
        valuation: '450000',
      },
      {
        code: 'AAPL',
        name: 'Apple',
        currency: 'USD',
        quantity: 10,
        acquisitionPrice: '150.25',
        currentPrice: '180.00',
        priceDate: '2026-10-06',
        valuation: '1800.00',
      },
    ],
    totals: [
      { currency: 'JPY', valuation: '450000', excludesUnpriced: true },
      { currency: 'USD', valuation: '1800.00', excludesUnpriced: false },
    ],
  }
}

/** API の例の円の行（1行目）を1か所だけ変えた本文を作る。 */
function exampleWithYenRow(change: (row: Record<string, unknown>) => void): unknown {
  const body = apiExampleBody()
  change(body.holdings[0] as unknown as Record<string, unknown>)
  return body
}

/** API の例のドルの行（2行目）を1か所だけ変えた本文を作る。 */
function exampleWithUsdRow(change: (row: Record<string, unknown>) => void): unknown {
  const body = apiExampleBody()
  change(body.holdings[1] as unknown as Record<string, unknown>)
  return body
}

/** API の例の合計の i 件目を1か所だけ変えた本文を作る。 */
function exampleWithTotal(index: number, change: (total: Record<string, unknown>) => void): unknown {
  const body = apiExampleBody()
  change(body.totals[index] as unknown as Record<string, unknown>)
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

describe('受け入れる応答（売買履歴のプラン 4.3「API」の形）', () => {
  test('補助 FR-22（PR4_EC2・PR4_TC02）: プラン 4.3 の API の例の応答（円とドルの行と、通貨ごとの合計）は、同じ形の値で resolve する', async () => {
    stubFetch(() => jsonResponse(apiExampleBody()))

    await expect(fetchHoldings()).resolves.toEqual(apiExampleBody())
  })

  test('補助 FR-22・FR-7（PR4_EC2・PR4_TC02）: 0件の応答（holdings と totals が空の配列）は、同じ形の値で resolve する', async () => {
    const emptyBody = { holdings: [], totals: [] }
    stubFetch(() => jsonResponse(emptyBody))

    await expect(fetchHoldings()).resolves.toEqual(emptyBody)
  })

  test('補助 FR-22・FR-10（PR4_EC2・PR4_TC02）: 価格のない行（currentPrice・priceDate・valuation の3つとも null。円もドルも）を含む応答は、同じ形の値で resolve する', async () => {
    // API の例の 7203 の行に、価格のない 9984（円）と MSFT（ドル）の行を足した応答（価格のない行は合計に含めない。FR-6）。
    // ドルの合計は、価格のある行がなければ "0.00"（要件13）。
    const body = {
      holdings: [
        apiExampleBody().holdings[0],
        {
          code: '9984',
          name: 'ソフトバンクグループ',
          currency: 'JPY',
          quantity: 50,
          acquisitionPrice: '4500.0',
          currentPrice: null,
          priceDate: null,
          valuation: null,
        },
        {
          code: 'MSFT',
          name: 'Microsoft',
          currency: 'USD',
          quantity: 5,
          acquisitionPrice: '400.00',
          currentPrice: null,
          priceDate: null,
          valuation: null,
        },
      ],
      totals: [
        { currency: 'JPY', valuation: '450000', excludesUnpriced: true },
        { currency: 'USD', valuation: '0.00', excludesUnpriced: true },
      ],
    }
    stubFetch(() => jsonResponse(body))

    await expect(fetchHoldings()).resolves.toEqual(body)
  })

  test('円だけの応答（ドルの合計の行がない）は、同じ形の値で resolve する（基準12）', async () => {
    const body = {
      holdings: [apiExampleBody().holdings[0]],
      totals: [{ currency: 'JPY', valuation: '450000', excludesUnpriced: false }],
    }
    stubFetch(() => jsonResponse(body))

    await expect(fetchHoldings()).resolves.toEqual(body)
  })

  test('ドルの最小の単価と評価額（"0.01"）の応答は、同じ形の値で resolve する', async () => {
    const body = {
      holdings: [
        {
          code: 'AAPL',
          name: 'Apple',
          currency: 'USD',
          quantity: 1,
          acquisitionPrice: '0.01',
          currentPrice: '0.01',
          priceDate: '2026-10-06',
          valuation: '0.01',
        },
      ],
      totals: [{ currency: 'USD', valuation: '0.01', excludesUnpriced: false }],
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
      body: exampleWithYenRow((row) => {
        row.quantity = MAX_SAFE_INTEGER_PLUS_ONE
      }),
    },
    {
      name: 'quantity が 0（正でない）',
      body: exampleWithYenRow((row) => {
        row.quantity = 0
      }),
    },
    // 通貨: "JPY" か "USD" でなければ reject（5.3）。
    {
      name: 'currency が "EUR"（知らない通貨）',
      body: exampleWithYenRow((row) => {
        row.currency = 'EUR'
      }),
    },
    {
      name: 'currency が小文字の "jpy"',
      body: exampleWithYenRow((row) => {
        row.currency = 'jpy'
      }),
    },
    {
      name: 'currency がない行',
      body: exampleWithYenRow((row) => {
        delete row.currency
      }),
    },
    // 評価額: 円は整数の文字列、ドルは小数2桁の文字列でなければ reject。数のままも reject。
    {
      name: '円の valuation が "-1"（負）',
      body: exampleWithYenRow((row) => {
        row.valuation = '-1'
      }),
    },
    {
      name: '円の valuation が数の 450000（文字列でない）',
      body: exampleWithYenRow((row) => {
        row.valuation = 450000
      }),
    },
    {
      name: '円の valuation が "450000.00"（小数がある。ドルの形）',
      body: exampleWithYenRow((row) => {
        row.valuation = '450000.00'
      }),
    },
    {
      name: 'ドルの valuation が "1800"（小数がない。円の形）',
      body: exampleWithUsdRow((row) => {
        row.valuation = '1800'
      }),
    },
    {
      name: 'ドルの valuation が "1800.0"（小数が1桁）',
      body: exampleWithUsdRow((row) => {
        row.valuation = '1800.0'
      }),
    },
    {
      name: 'ドルの valuation が数の 1800（文字列でない）',
      body: exampleWithUsdRow((row) => {
        row.valuation = 1800
      }),
    },
    // 単価（取得価格・現在の価格）: 円は ^[0-9]+\.[0-9]$、ドルは ^[0-9]+\.[0-9]{2}$ の文字列でなければ reject（5.3）。
    ...(['acquisitionPrice', 'currentPrice'] as const).flatMap((field) => [
      {
        name: `円の ${field} が "2500"（小数第1位がない）`,
        body: exampleWithYenRow((row) => {
          row[field] = '2500'
        }),
      },
      {
        name: `円の ${field} が "2500.00"（小数が2桁。ドルの形）`,
        body: exampleWithYenRow((row) => {
          row[field] = '2500.00'
        }),
      },
      {
        name: `円の ${field} が "-1.0"（負）`,
        body: exampleWithYenRow((row) => {
          row[field] = '-1.0'
        }),
      },
      {
        name: `円の ${field} が数の 2500（文字列でない）`,
        body: exampleWithYenRow((row) => {
          row[field] = 2500
        }),
      },
      {
        name: `ドルの ${field} が "150"（小数がない）`,
        body: exampleWithUsdRow((row) => {
          row[field] = '150'
        }),
      },
      {
        name: `ドルの ${field} が "150.2"（小数が1桁。円の形）`,
        body: exampleWithUsdRow((row) => {
          row[field] = '150.2'
        }),
      },
      {
        name: `ドルの ${field} が "150.255"（小数が3桁）`,
        body: exampleWithUsdRow((row) => {
          row[field] = '150.255'
        }),
      },
      {
        name: `ドルの ${field} が "-1.00"（負）`,
        body: exampleWithUsdRow((row) => {
          row[field] = '-1.00'
        }),
      },
    ]),
    // 行の形: 「価格あり（3つがそろう）」か「価格なし（3つとも null）」のどちらでもなければ reject。
    {
      name: 'currentPrice があって priceDate が null の行',
      body: exampleWithYenRow((row) => {
        row.priceDate = null
      }),
    },
    // 価格の基準日: 実在する YYYY-MM-DD でなければ reject。
    {
      name: 'priceDate が "2026-02-30"（実在しない日付）',
      body: exampleWithYenRow((row) => {
        row.priceDate = '2026-02-30'
      }),
    },
    // holdings・totals: 配列でなければ（null も、ない場合も）reject。
    {
      name: 'holdings が null',
      body: exampleWithTop((body) => {
        body.holdings = null
      }),
    },
    {
      name: 'totals が null',
      body: exampleWithTop((body) => {
        body.totals = null
      }),
    },
    {
      name: 'totals がない（PR① の形の totalValuation だけ）',
      body: exampleWithTop((body) => {
        delete body.totals
        body.totalValuation = 450000
        body.totalExcludesUnpriced = false
      }),
    },
    // 合計の1件: 通貨は "JPY" か "USD"、valuation は通貨ごとの形の文字列、excludesUnpriced は真偽値でなければ reject。
    {
      name: 'totals の currency が "EUR"（知らない通貨）',
      body: exampleWithTotal(0, (total) => {
        total.currency = 'EUR'
      }),
    },
    {
      name: '円の totals の valuation が "450000.00"（小数がある）',
      body: exampleWithTotal(0, (total) => {
        total.valuation = '450000.00'
      }),
    },
    {
      name: '円の totals の valuation が数の 450000（文字列でない）',
      body: exampleWithTotal(0, (total) => {
        total.valuation = 450000
      }),
    },
    {
      name: 'ドルの totals の valuation が "1800"（小数がない）',
      body: exampleWithTotal(1, (total) => {
        total.valuation = '1800'
      }),
    },
    {
      name: 'totals の excludesUnpriced が文字列の "false"（真偽値でない）',
      body: exampleWithTotal(0, (total) => {
        total.excludesUnpriced = 'false'
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
