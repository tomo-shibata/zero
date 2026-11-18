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
// カード表示（要件 docs/requirements/holdings-cards.md、プラン docs/plans/holdings-cards/plan.md 4.6 の PR①・5.2）: 行と合計に
//   unrealizedGain（円 /^-?[0-9]+$/・ドル /^-?[0-9]+\.[0-9]{2}$/）・unrealizedGainRate（/^-?[0-9]+\.[0-9]$/）を足す。
//   行: currentPrice が null なら両方 null、null でなければ両方文字列。合計: 両方 null か両方文字列。外れたら reject。
// ドルを円にまとめる合計（要件 docs/requirements/fx-total.md、プラン docs/plans/fx-total/plan.md 4.4・4.7 の PR②・5.2・5.3）: 応答に combinedTotal
//   （キーは常にあり、値は null か { valuation, excludesUnpriced, unrealizedGain, unrealizedGainRate, usdJpyRate, usdJpyRateDate }）を足す。
//   円の整数・符号付きの整数・符号付きの小数1桁・小数2桁・実在する日付に外れたら reject。既存の本文には combinedTotal: null を足した。
// おまかせ運用（プラン docs/plans/managed-account/plan.md 4.8 の PR③・5.2・5.3）: 応答に managedAccounts（キーは常にあり、口座がなければ空の配列。
//   口座は { name, valuation, principal, unrealizedGain, unrealizedGainRate, valuationDate }）を足す。円の整数・符号付きの整数（「+」なし）・
//   符号付きの小数1桁・実在する日付に外れたら reject。既存の本文には managedAccounts: [] を足した。
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
        unrealizedGain: '75000',
        unrealizedGainRate: '20.0',
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
        unrealizedGain: '297.50',
        unrealizedGainRate: '19.8',
      },
    ],
    totals: [
      { currency: 'JPY', valuation: '450000', excludesUnpriced: true, unrealizedGain: '75000', unrealizedGainRate: '20.0' },
      { currency: 'USD', valuation: '1800.00', excludesUnpriced: false, unrealizedGain: '297.50', unrealizedGainRate: '19.8' },
    ],
    // ドルを円にまとめる合計のプラン 4.4: キーは常にあり、値は null かオブジェクト（5.3 の PR②）。
    combinedTotal: null as unknown,
    // おまかせ運用のプラン 4.4・4.8 PR③: キーは常にあり、口座がなければ空の配列。
    managedAccounts: [] as unknown[],
  }
}

/** おまかせ運用の口座1つ（プラン 4.8 PR③ の ManagedAccount。要件定義の基準1 の WealthNavi）。 */
function managedAccountExample(): Record<string, unknown> {
  return {
    name: 'WealthNavi',
    valuation: '4000000',
    principal: '3000000',
    unrealizedGain: '1000000',
    unrealizedGainRate: '33.3',
    valuationDate: '2026-11-16',
  }
}

/** API の例に口座を1つ足し、その口座を1か所だけ変えた本文を作る。 */
function exampleWithManagedAccount(change: (account: Record<string, unknown>) => void): unknown {
  const body = apiExampleBody()
  const account = managedAccountExample()
  change(account)
  body.managedAccounts = [account]
  return body
}

/** ドルを円にまとめる合計のプラン 4.4 の例（基準1 の値）。 */
function combinedTotalExample(): Record<string, unknown> {
  return {
    valuation: '570000',
    excludesUnpriced: false,
    unrealizedGain: '94625',
    unrealizedGainRate: '19.9',
    usdJpyRate: '150.00',
    usdJpyRateDate: '2026-11-14',
  }
}

/** API の例の combinedTotal をオブジェクトにして、その1か所だけ変えた本文を作る。 */
function exampleWithCombinedTotal(change: (combinedTotal: Record<string, unknown>) => void): unknown {
  const body = apiExampleBody()
  const combinedTotal = combinedTotalExample()
  change(combinedTotal)
  body.combinedTotal = combinedTotal
  return body
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
    const emptyBody = { holdings: [], totals: [], combinedTotal: null, managedAccounts: [] }
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
          unrealizedGain: null,
          unrealizedGainRate: null,
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
          unrealizedGain: null,
          unrealizedGainRate: null,
        },
      ],
      totals: [
        { currency: 'JPY', valuation: '450000', excludesUnpriced: true, unrealizedGain: '75000', unrealizedGainRate: '20.0' },
        { currency: 'USD', valuation: '0.00', excludesUnpriced: true, unrealizedGain: null, unrealizedGainRate: null },
      ],
      combinedTotal: null,
      managedAccounts: [],
    }
    stubFetch(() => jsonResponse(body))

    await expect(fetchHoldings()).resolves.toEqual(body)
  })

  test('円だけの応答（ドルの合計の行がない）は、同じ形の値で resolve する（基準12）', async () => {
    const body = {
      holdings: [apiExampleBody().holdings[0]],
      totals: [{ currency: 'JPY', valuation: '450000', excludesUnpriced: false, unrealizedGain: '75000', unrealizedGainRate: '20.0' }],
      combinedTotal: null,
      managedAccounts: [],
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
          unrealizedGain: '0.00',
          unrealizedGainRate: '0.0',
        },
      ],
      totals: [{ currency: 'USD', valuation: '0.01', excludesUnpriced: false, unrealizedGain: '0.00', unrealizedGainRate: '0.0' }],
      combinedTotal: null,
      managedAccounts: [],
    }
    stubFetch(() => jsonResponse(body))

    await expect(fetchHoldings()).resolves.toEqual(body)
  })

  // カード表示の要件 holdings-cards.md の要件10・基準13、プラン 4.6（PR①）の fetchHoldings の zod:
  // 負の含み損益と率（"-501"・"-12.5"。ドルの "-0.01"・"-0.0" は出ないが "-0.01" と "0.0" の組はある）も受け入れる。
  test('補助（カード表示 PR①）: マイナスの含み損益と率（円 "-501"・"-12.5"、ドル "-0.01"・"0.0"）を含む応答は、同じ形の値で resolve する', async () => {
    const body = {
      holdings: [
        { ...(apiExampleBody().holdings[0] as object), unrealizedGain: '-501', unrealizedGainRate: '-12.5' },
        { ...(apiExampleBody().holdings[1] as object), unrealizedGain: '-0.01', unrealizedGainRate: '0.0' },
      ],
      totals: [
        { currency: 'JPY', valuation: '450000', excludesUnpriced: false, unrealizedGain: '-501', unrealizedGainRate: '-12.5' },
        { currency: 'USD', valuation: '1800.00', excludesUnpriced: false, unrealizedGain: '-0.01', unrealizedGainRate: '0.0' },
      ],
      combinedTotal: null,
      managedAccounts: [],
    }
    stubFetch(() => jsonResponse(body))

    await expect(fetchHoldings()).resolves.toEqual(body)
  })

  // ドルを円にまとめる合計のプラン 5.2: combinedTotal が null とオブジェクトで通る（API の境界。フロントのルールの5）。
  test('補助（ドルを円にまとめる合計 PR②）: combinedTotal がオブジェクト（基準1 の値）の応答は、同じ形の値で resolve する', async () => {
    const body = exampleWithCombinedTotal(() => {})
    stubFetch(() => jsonResponse(body))

    await expect(fetchHoldings()).resolves.toEqual(body)
  })

  test('補助（ドルを円にまとめる合計 PR②）: 含み損益と率が null で excludesUnpriced が true のオブジェクト（基準12）、マイナスの含み損益（基準8）、4桁のレート "1000.50" も resolve する', async () => {
    const unavailable = exampleWithCombinedTotal((combinedTotal) => {
      combinedTotal.valuation = '0'
      combinedTotal.excludesUnpriced = true
      combinedTotal.unrealizedGain = null
      combinedTotal.unrealizedGainRate = null
    })
    const loss = exampleWithCombinedTotal((combinedTotal) => {
      combinedTotal.unrealizedGain = '-10002'
      combinedTotal.unrealizedGainRate = '-8.7'
      combinedTotal.usdJpyRate = '1000.50'
      combinedTotal.usdJpyRateDate = '2026-01-05'
    })

    stubFetch(() => jsonResponse(unavailable))
    await expect(fetchHoldings()).resolves.toEqual(unavailable)
    stubFetch(() => jsonResponse(loss))
    await expect(fetchHoldings()).resolves.toEqual(loss)
  })

  // おまかせ運用のプラン 5.2 の PR③: managedAccounts が [] と口座1つで通る（API の境界。フロントのルールの5）。
  test('補助（おまかせ運用 PR③）: managedAccounts が空の配列の応答は、同じ形の値で resolve する', async () => {
    const body = apiExampleBody()
    expect(body.managedAccounts).toEqual([])
    stubFetch(() => jsonResponse(body))

    await expect(fetchHoldings()).resolves.toEqual(body)
  })

  test('補助（おまかせ運用 PR③）: managedAccounts に口座1つ（基準1 の WealthNavi）の応答は、口座の6つの項目を落とさず同じ形の値で resolve する', async () => {
    const body = exampleWithManagedAccount(() => {})
    stubFetch(() => jsonResponse(body))

    await expect(fetchHoldings()).resolves.toEqual(body)
  })

  // 要件1: 名前は空白以外の文字を1つ以上含む。backend は Go の unicode.IsSpace で見るので U+FEFF（BOM）は空白でなく、
  // U+FEFF だけの名前も DB に入って返りうる。backend が受け入れる名前は画面も受け入れる（一覧全体が取得失敗にならない）。
  test('補助（おまかせ運用 PR③・要件1）: 名前が U+FEFF だけの口座（backend は空白と見ない）の応答は、名前をそのままに同じ形の値で resolve する', async () => {
    const body = exampleWithManagedAccount((account) => {
      account.name = '\uFEFF'
    })
    stubFetch(() => jsonResponse(body))

    const result = await fetchHoldings()

    expect(result).toEqual(body)
    expect(result.managedAccounts[0].name).toBe('\uFEFF')
  })

  test('補助（おまかせ運用 PR③）: マイナス・0の含み損益（"-300000"・"-10.0"、"0"・"0.0"）と、holdings が空で口座だけの応答も resolve する', async () => {
    const loss = exampleWithManagedAccount((account) => {
      account.valuation = '2700000'
      account.principal = '3000000'
      account.unrealizedGain = '-300000'
      account.unrealizedGainRate = '-10.0'
    })
    const even = exampleWithManagedAccount((account) => {
      account.unrealizedGain = '0'
      account.unrealizedGainRate = '0.0'
    })
    const onlyAccount = {
      holdings: [],
      totals: [{ currency: 'JPY', valuation: '4000000', excludesUnpriced: false, unrealizedGain: '1000000', unrealizedGainRate: '33.3' }],
      combinedTotal: null,
      managedAccounts: [managedAccountExample()],
    }

    for (const body of [loss, even, onlyAccount]) {
      stubFetch(() => jsonResponse(body))
      await expect(fetchHoldings()).resolves.toEqual(body)
    }
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
    // 含み損益・率（カード表示のプラン 4.6 PR①）: 円の含み損益 /^-?[0-9]+$/、ドル /^-?[0-9]+\.[0-9]{2}$/、率 /^-?[0-9]+\.[0-9]$/。外れたら reject。
    {
      name: '円の unrealizedGain が "75000.00"（小数がある。ドルの形）',
      body: exampleWithYenRow((row) => {
        row.unrealizedGain = '75000.00'
      }),
    },
    {
      name: '円の unrealizedGain が "+75000"（+ が付いている。API は + を付けない）',
      body: exampleWithYenRow((row) => {
        row.unrealizedGain = '+75000'
      }),
    },
    {
      name: '円の unrealizedGain が数の 75000（文字列でない）',
      body: exampleWithYenRow((row) => {
        row.unrealizedGain = 75000
      }),
    },
    {
      name: 'ドルの unrealizedGain が "297"（小数がない。円の形）',
      body: exampleWithUsdRow((row) => {
        row.unrealizedGain = '297'
      }),
    },
    {
      name: 'ドルの unrealizedGain が "297.5"（小数が1桁）',
      body: exampleWithUsdRow((row) => {
        row.unrealizedGain = '297.5'
      }),
    },
    {
      name: 'ドルの unrealizedGain が "297.505"（小数が3桁）',
      body: exampleWithUsdRow((row) => {
        row.unrealizedGain = '297.505'
      }),
    },
    {
      name: 'unrealizedGainRate が "20"（小数がない）',
      body: exampleWithYenRow((row) => {
        row.unrealizedGainRate = '20'
      }),
    },
    {
      name: 'unrealizedGainRate が "20.00"（小数が2桁）',
      body: exampleWithYenRow((row) => {
        row.unrealizedGainRate = '20.00'
      }),
    },
    {
      name: 'unrealizedGainRate が数の 20（文字列でない）',
      body: exampleWithYenRow((row) => {
        row.unrealizedGainRate = 20
      }),
    },
    {
      name: '行の unrealizedGain がない（キーごと欠けている）',
      body: exampleWithYenRow((row) => {
        delete row.unrealizedGain
      }),
    },
    {
      name: '行の unrealizedGainRate がない（キーごと欠けている）',
      body: exampleWithYenRow((row) => {
        delete row.unrealizedGainRate
      }),
    },
    // 行: currentPrice が null なら含み損益・率も null、null でなければ両方とも文字列。違えば reject。
    {
      name: '価格のある行（currentPrice が文字列）の unrealizedGain が null',
      body: exampleWithYenRow((row) => {
        row.unrealizedGain = null
      }),
    },
    {
      name: '価格のある行（currentPrice が文字列）の unrealizedGainRate が null',
      body: exampleWithYenRow((row) => {
        row.unrealizedGainRate = null
      }),
    },
    {
      name: '価格のない行（currentPrice・priceDate・valuation が null）の unrealizedGain が文字列',
      body: exampleWithYenRow((row) => {
        row.currentPrice = null
        row.priceDate = null
        row.valuation = null
        row.unrealizedGainRate = null
      }),
    },
    {
      name: '価格のない行（currentPrice・priceDate・valuation が null）の unrealizedGainRate が文字列',
      body: exampleWithYenRow((row) => {
        row.currentPrice = null
        row.priceDate = null
        row.valuation = null
        row.unrealizedGain = null
      }),
    },
    // 合計: unrealizedGain と unrealizedGainRate は、両方とも null か両方とも文字列。違えば reject。
    {
      name: 'totals の unrealizedGain が null で unrealizedGainRate が文字列',
      body: exampleWithTotal(0, (total) => {
        total.unrealizedGain = null
      }),
    },
    {
      name: 'totals の unrealizedGain が文字列で unrealizedGainRate が null',
      body: exampleWithTotal(0, (total) => {
        total.unrealizedGainRate = null
      }),
    },
    {
      name: '円の totals の unrealizedGain が "75000.00"（小数がある）',
      body: exampleWithTotal(0, (total) => {
        total.unrealizedGain = '75000.00'
      }),
    },
    {
      name: 'ドルの totals の unrealizedGain が "297"（小数がない）',
      body: exampleWithTotal(1, (total) => {
        total.unrealizedGain = '297'
      }),
    },
    {
      name: 'totals の unrealizedGainRate が "19.85"（小数が2桁）',
      body: exampleWithTotal(1, (total) => {
        total.unrealizedGainRate = '19.85'
      }),
    },
    {
      name: 'totals の unrealizedGain がない（キーごと欠けている）',
      body: exampleWithTotal(0, (total) => {
        delete total.unrealizedGain
      }),
    },
    {
      name: 'totals の unrealizedGainRate がない（キーごと欠けている）',
      body: exampleWithTotal(0, (total) => {
        delete total.unrealizedGainRate
      }),
    },
    // ドルを円にまとめる合計（プラン 4.4・5.2）: キーは常にあり、値は null かオブジェクト。valuation は円の整数、含み損益は円の符号付きの整数、
    // 率は符号付きの小数1桁（含み損益と同時に null）、usdJpyRate は小数2桁、usdJpyRateDate は実在する YYYY-MM-DD。外れたら reject。
    {
      name: 'combinedTotal のキーがない（キーは常にある）',
      body: exampleWithTop((body) => {
        delete body.combinedTotal
      }),
    },
    {
      name: 'combinedTotal の usdJpyRate が "150.0"（小数が1桁）',
      body: exampleWithCombinedTotal((combinedTotal) => {
        combinedTotal.usdJpyRate = '150.0'
      }),
    },
    {
      name: 'combinedTotal の usdJpyRate が "150"（小数がない）',
      body: exampleWithCombinedTotal((combinedTotal) => {
        combinedTotal.usdJpyRate = '150'
      }),
    },
    {
      name: 'combinedTotal の usdJpyRate が数の 150（文字列でない）',
      body: exampleWithCombinedTotal((combinedTotal) => {
        combinedTotal.usdJpyRate = 150
      }),
    },
    {
      name: 'combinedTotal の valuation が "570000.5"（小数がある。円は整数）',
      body: exampleWithCombinedTotal((combinedTotal) => {
        combinedTotal.valuation = '570000.5'
      }),
    },
    {
      name: 'combinedTotal の valuation が "-1"（負）',
      body: exampleWithCombinedTotal((combinedTotal) => {
        combinedTotal.valuation = '-1'
      }),
    },
    {
      name: 'combinedTotal の unrealizedGain が "94625.00"（小数がある。円は整数）',
      body: exampleWithCombinedTotal((combinedTotal) => {
        combinedTotal.unrealizedGain = '94625.00'
      }),
    },
    {
      name: 'combinedTotal の unrealizedGain だけ null（率は文字列）',
      body: exampleWithCombinedTotal((combinedTotal) => {
        combinedTotal.unrealizedGain = null
      }),
    },
    {
      name: 'combinedTotal の unrealizedGainRate だけ null（含み損益は文字列）',
      body: exampleWithCombinedTotal((combinedTotal) => {
        combinedTotal.unrealizedGainRate = null
      }),
    },
    {
      name: 'combinedTotal の unrealizedGainRate が "19.85"（小数が2桁）',
      body: exampleWithCombinedTotal((combinedTotal) => {
        combinedTotal.unrealizedGainRate = '19.85'
      }),
    },
    {
      name: 'combinedTotal の excludesUnpriced が文字列の "false"（真偽値でない）',
      body: exampleWithCombinedTotal((combinedTotal) => {
        combinedTotal.excludesUnpriced = 'false'
      }),
    },
    {
      name: 'combinedTotal の usdJpyRateDate が "2026-02-30"（実在しない日付）',
      body: exampleWithCombinedTotal((combinedTotal) => {
        combinedTotal.usdJpyRateDate = '2026-02-30'
      }),
    },
    {
      name: 'combinedTotal の usdJpyRateDate が "20261114"（YYYY-MM-DD でない）',
      body: exampleWithCombinedTotal((combinedTotal) => {
        combinedTotal.usdJpyRateDate = '20261114'
      }),
    },
    {
      name: 'combinedTotal の usdJpyRateDate がない（キーごと欠けている）',
      body: exampleWithCombinedTotal((combinedTotal) => {
        delete combinedTotal.usdJpyRateDate
      }),
    },
    // おまかせ運用（プラン 4.8 PR③・5.2）: managedAccounts のキーは常にある。金額は円の文字列（valuation は負でない整数、principal は
    // 整数、unrealizedGain は符号付きの整数で "+" を付けない）、率は符号付きの小数1桁、valuationDate は実在する YYYY-MM-DD。外れたら reject。
    {
      name: 'managedAccounts のキーがない（キーは常にある）',
      body: exampleWithTop((body) => {
        delete body.managedAccounts
      }),
    },
    {
      name: 'managedAccounts が null',
      body: exampleWithTop((body) => {
        body.managedAccounts = null
      }),
    },
    {
      name: 'managedAccounts の valuation が "4000000.5"（小数がある。円は整数）',
      body: exampleWithManagedAccount((account) => {
        account.valuation = '4000000.5'
      }),
    },
    {
      name: 'managedAccounts の valuation が "-1"（負）',
      body: exampleWithManagedAccount((account) => {
        account.valuation = '-1'
      }),
    },
    {
      name: 'managedAccounts の valuation が数の 4000000（文字列でない）',
      body: exampleWithManagedAccount((account) => {
        account.valuation = 4000000
      }),
    },
    {
      name: 'managedAccounts の principal が "3,000,000"（カンマ区切り）',
      body: exampleWithManagedAccount((account) => {
        account.principal = '3,000,000'
      }),
    },
    {
      name: 'managedAccounts の unrealizedGain が "+1000000"（プラスに「+」は付かない）',
      body: exampleWithManagedAccount((account) => {
        account.unrealizedGain = '+1000000'
      }),
    },
    {
      name: 'managedAccounts の unrealizedGain が null（口座は常に値がある）',
      body: exampleWithManagedAccount((account) => {
        account.unrealizedGain = null
      }),
    },
    {
      name: 'managedAccounts の unrealizedGainRate が "33.33"（小数が2桁）',
      body: exampleWithManagedAccount((account) => {
        account.unrealizedGainRate = '33.33'
      }),
    },
    {
      name: 'managedAccounts の valuationDate が "20261116"（YYYY-MM-DD でない）',
      body: exampleWithManagedAccount((account) => {
        account.valuationDate = '20261116'
      }),
    },
    {
      name: 'managedAccounts の valuationDate が "2026-02-30"（実在しない日付）',
      body: exampleWithManagedAccount((account) => {
        account.valuationDate = '2026-02-30'
      }),
    },
    {
      name: 'managedAccounts の name がない（キーごと欠けている）',
      body: exampleWithManagedAccount((account) => {
        delete account.name
      }),
    },
    {
      name: 'managedAccounts の name が ""（空。名前に空白でない文字が要る）',
      body: exampleWithManagedAccount((account) => {
        account.name = ''
      }),
    },
    {
      name: 'managedAccounts の name が "   "（空白だけ）',
      body: exampleWithManagedAccount((account) => {
        account.name = '   '
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
