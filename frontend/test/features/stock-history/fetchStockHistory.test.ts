import { afterEach, describe, expect, test, vi } from 'vitest'
import { fetchStockHistory } from '../../../src/features/stock-history/api/fetchStockHistory'

// 仕様: 要件 docs/requirements/stock-history.md の要件20（売買履歴がなければ「なし」）・要件23（取得に失敗したらダイアログ）。
// 境界: プラン docs/plans/stock-history/plan.md 4.4「API の形」と 4.8「PR② 画面とカードからの移動」の features/stock-history/api/fetchStockHistory.ts
//   - GET /api/holdings/{code}/history。応答は { stock: { code, name, currency }, entries, closes }。
//     entries の売買の行は kind・date・side・quantity・price・amount・splitDivisor・adjustedPrice の8つ、分割の行は kind・date・ratio だけ。
//     金額の文字列（price・amount・adjustedPrice）の形は一覧の単価と同じ（円は小数1桁・ドルは小数2桁）。
//     splitDivisor は整数の文字列（大きくなりうる）。closes は取引日の昇順で、0件のときは [] （null は形の誤り）。
//   - fetchStockHistory(code): Promise<StockHistoryResult>。404 は { kind: 'notFound' }（エラーにしない）。found は { kind: 'found', history }。
//     そのほかの失敗（2xx と 404 以外、通信の失敗）と形の誤りは reject。
// テスト計画: プラン 5.2 の fetchStockHistory.test.ts の行（404 で notFound、500 と形の誤り（円の価格に小数2桁、kind が知らない値、
//   quantity が安全な整数でない、closes が null）で throw）。
// mock は境界の外側（ネットワーク = グローバルの fetch）だけに使い、結果（resolve した値か reject か）を確かめる。

/** プラン 4.4 の API の例（そのまま。7203 の分割 1→3 2026-03-31）。 */
function apiExampleBody() {
  return {
    stock: { code: '7203', name: 'トヨタ自動車', currency: 'JPY' },
    entries: [
      {
        kind: 'trade',
        date: '2026-03-31',
        side: 'buy',
        quantity: 30,
        price: '1000.0',
        amount: '30000.0',
        splitDivisor: '1',
        adjustedPrice: '1000.0',
      },
      { kind: 'split', date: '2026-03-31', ratio: 3 },
      {
        kind: 'trade',
        date: '2026-01-10',
        side: 'buy',
        quantity: 100,
        price: '2500.0',
        amount: '250000.0',
        splitDivisor: '3',
        adjustedPrice: '833.3',
      },
    ],
    closes: [
      { date: '2026-03-30', price: '3100.0', splitDivisor: '3', adjustedPrice: '1033.3' },
      { date: '2026-03-31', price: '1050.0', splitDivisor: '1', adjustedPrice: '1050.0' },
    ],
  }
}

/** ドルの銘柄（AAPL）の応答。 */
function usdBody() {
  return {
    stock: { code: 'AAPL', name: 'Apple', currency: 'USD' },
    entries: [
      {
        kind: 'trade',
        date: '2026-01-10',
        side: 'buy',
        quantity: 10,
        price: '150.25',
        amount: '1502.50',
        splitDivisor: '1',
        adjustedPrice: '150.25',
      },
    ],
    closes: [{ date: '2026-10-06', price: '180.00', splitDivisor: '1', adjustedPrice: '180.00' }],
  }
}

/** 例の売買の行（entries[0]）を1か所だけ変えた本文。 */
function withTrade(change: (row: Record<string, unknown>) => void): unknown {
  const body = apiExampleBody()
  change(body.entries[0] as unknown as Record<string, unknown>)
  return body
}

/** 例の終値（closes[0]）を1か所だけ変えた本文。 */
function withClose(change: (row: Record<string, unknown>) => void): unknown {
  const body = apiExampleBody()
  change(body.closes[0] as unknown as Record<string, unknown>)
  return body
}

/** 例の最上位の項目を1か所だけ変えた本文。 */
function withTop(change: (body: Record<string, unknown>) => void): unknown {
  const body = apiExampleBody()
  change(body as unknown as Record<string, unknown>)
  return body
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

function requestedUrl(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.href
  return input.url
}

/** グローバルの fetch を差し替える。渡された URL を記録する。 */
function stubFetch(makeResponse: () => Response): string[] {
  const urls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn<typeof fetch>(async (input) => {
      urls.push(requestedUrl(input))
      return makeResponse()
    }),
  )
  return urls
}

/** fetchStockHistory('7203') が resolve か reject か。 */
async function settle(): Promise<'resolved' | 'rejected'> {
  return fetchStockHistory('7203').then(
    () => 'resolved' as const,
    () => 'rejected' as const,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('受け入れる応答（プラン 4.4）', () => {
  test('補助 要件19: プラン 4.4 の API の例は、同じ形の値で { kind: "found", history } として resolve する。GET /api/holdings/7203/history を呼ぶ', async () => {
    const urls = stubFetch(() => jsonResponse(apiExampleBody()))

    await expect(fetchStockHistory('7203')).resolves.toEqual({ kind: 'found', history: apiExampleBody() })
    expect(urls).toEqual(['/api/holdings/7203/history'])
  })

  test('補助 要件6: ドルの銘柄（価格は小数2桁）の応答も、同じ形の値で resolve する', async () => {
    stubFetch(() => jsonResponse(usdBody()))

    await expect(fetchStockHistory('AAPL')).resolves.toEqual({ kind: 'found', history: usdBody() })
  })

  test('補助 要件12・プラン 4.4: 終値が0件（closes が空の配列）の応答は resolve する', async () => {
    const body = { ...apiExampleBody(), closes: [] }
    stubFetch(() => jsonResponse(body))

    await expect(fetchStockHistory('7203')).resolves.toEqual({ kind: 'found', history: body })
  })

  test('補助 プラン 4.4・4.8（splitDivisor は 2^53 を超えうる）: 割る数が整数の文字列なら、桁が大きくても（1 の後ろに 0 が21個）そのままの文字列で resolve する', async () => {
    const huge = `1${'0'.repeat(21)}`
    const body = withClose((row) => {
      row.splitDivisor = huge
    })
    stubFetch(() => jsonResponse(body))

    const result = await fetchStockHistory('7203')

    expect(result).toEqual({ kind: 'found', history: body })
    if (result.kind !== 'found') throw new Error('found のはず')
    expect(result.history.closes[0].splitDivisor).toBe(huge)
  })
})

describe('404 と失敗', () => {
  test('補助 要件20（5.2）: 404 は reject せず { kind: "notFound" } で resolve する', async () => {
    stubFetch(() => jsonResponse({ error: 'not_found' }, 404))

    await expect(fetchStockHistory('7203')).resolves.toEqual({ kind: 'notFound' })
  })

  // 404 は、本文が {"error":"not_found"} のときだけ売買履歴なし。別の 404（存在しない URL・proxy のエラーなど）は失敗として扱う（要件23）。
  test('補助 要件23（エッジケース2）: 404 でも本文が「404 page not found」の文字なら reject する（売買履歴なしにしない）', async () => {
    stubFetch(() => new Response('404 page not found\n', { status: 404, headers: { 'Content-Type': 'text/plain' } }))

    await expect(settle()).resolves.toBe('rejected')
  })

  test('補助 要件23（エッジケース2）: 404 でも本文が別の JSON（{"error":"internal_error"}）なら reject する', async () => {
    stubFetch(() => jsonResponse({ error: 'internal_error' }, 404))

    await expect(settle()).resolves.toBe('rejected')
  })

  test('補助 要件23（エッジケース2）: 404 でも本文が {"message":"not_found"}（error の欄がない JSON）なら reject する', async () => {
    stubFetch(() => jsonResponse({ message: 'not_found' }, 404))

    await expect(settle()).resolves.toBe('rejected')
  })

  test('補助 要件23（エッジケース2）: 404 で本文が空なら reject する', async () => {
    stubFetch(() => new Response(null, { status: 404 }))

    await expect(settle()).resolves.toBe('rejected')
  })

  test('補助 要件23（5.2）: 500 は reject する', async () => {
    stubFetch(() => jsonResponse({ error: 'internal_error' }, 500))

    await expect(settle()).resolves.toBe('rejected')
  })

  test('補助 要件23: 通信の失敗（fetch が reject）は reject する', async () => {
    vi.stubGlobal('fetch', vi.fn<typeof fetch>(() => Promise.reject(new TypeError('Failed to fetch'))))

    await expect(settle()).resolves.toBe('rejected')
  })

  test('補助 要件23: JSON でない本文は reject する', async () => {
    stubFetch(() => new Response('<html></html>', { status: 200 }))

    await expect(settle()).resolves.toBe('rejected')
  })
})

describe('形の誤りは reject する（フロントのルールの5。要件23）', () => {
  const INVALID: { name: string; body: () => unknown }[] = [
    {
      name: '円の価格に小数2桁（"1000.00"）',
      body: () =>
        withTrade((row) => {
          row.price = '1000.00'
        }),
    },
    {
      name: '円の割った価格に小数2桁',
      body: () =>
        withTrade((row) => {
          row.adjustedPrice = '1000.00'
        }),
    },
    {
      name: '円の金額に小数2桁',
      body: () =>
        withTrade((row) => {
          row.amount = '30000.00'
        }),
    },
    {
      name: '円の終値に小数2桁',
      body: () =>
        withClose((row) => {
          row.price = '3100.00'
        }),
    },
    {
      name: 'ドルの価格に小数1桁',
      body: () => {
        const body = usdBody()
        body.entries[0].price = '150.2'
        return body
      },
    },
    {
      name: 'kind が知らない値',
      body: () =>
        withTrade((row) => {
          row.kind = 'dividend'
        }),
    },
    {
      name: 'side が知らない値',
      body: () =>
        withTrade((row) => {
          row.side = 'hold'
        }),
    },
    {
      name: 'quantity が小数',
      body: () =>
        withTrade((row) => {
          row.quantity = 1.5
        }),
    },
    {
      name: 'quantity が安全な整数でない（2^53）',
      body: () =>
        withTrade((row) => {
          row.quantity = 2 ** 53
        }),
    },
    {
      name: 'splitDivisor が整数の文字列でない（"3.5"）',
      body: () =>
        withClose((row) => {
          row.splitDivisor = '3.5'
        }),
    },
    {
      name: 'closes が null',
      body: () =>
        withTop((body) => {
          body.closes = null
        }),
    },
    {
      name: 'entries が null',
      body: () =>
        withTop((body) => {
          body.entries = null
        }),
    },
    {
      name: '知らない通貨',
      body: () =>
        withTop((body) => {
          body.stock = { code: '7203', name: 'トヨタ自動車', currency: 'EUR' }
        }),
    },
    {
      name: '日付が実在しない（"2026-13-01"）',
      body: () =>
        withClose((row) => {
          row.date = '2026-13-01'
        }),
    },
    {
      // 売買の行を残して、分割の行の ratio だけを欠かす（売買がないという別の誤りで reject しないように）。
      name: '分割の行に ratio がない（売買の行は残す）',
      body: () =>
        withTop((body) => {
          body.entries = [(body.entries as unknown[])[0], { kind: 'split', date: '2026-03-31' }]
        }),
    },
    {
      name: 'entries が空の配列（売買履歴が0件なら 404 になるので、found の応答にはない）',
      body: () =>
        withTop((body) => {
          body.entries = []
        }),
    },
    {
      name: 'entries が分割の行だけ（売買が1件もない）',
      body: () =>
        withTop((body) => {
          body.entries = [{ kind: 'split', date: '2026-03-31', ratio: 3 }]
        }),
    },
    {
      name: '売買の行の splitDivisor が "0"',
      body: () =>
        withTrade((row) => {
          row.splitDivisor = '0'
        }),
    },
    {
      name: '終値の行の splitDivisor が "0"',
      body: () =>
        withClose((row) => {
          row.splitDivisor = '0'
        }),
    },
    {
      name: '売買の行の splitDivisor が先頭0の "03"',
      body: () =>
        withTrade((row) => {
          row.splitDivisor = '03'
        }),
    },
    {
      name: '終値の行の splitDivisor が先頭0の "03"',
      body: () =>
        withClose((row) => {
          row.splitDivisor = '03'
        }),
    },
    {
      name: '売買の行の quantity が 0',
      body: () =>
        withTrade((row) => {
          row.quantity = 0
        }),
    },
  ]

  for (const { name, body } of INVALID) {
    test(`補助 5.2: ${name} は reject する`, async () => {
      stubFetch(() => jsonResponse(body()))

      await expect(settle()).resolves.toBe('rejected')
    })
  }
})
