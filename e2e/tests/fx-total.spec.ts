import { expect, test, type Locator, type Page } from '@playwright/test'
import {
  insertHoldingsData,
  resetHoldingsData,
  type StockPriceSeed,
  type StockSeed,
  type TradeSeed,
  type UsdJpyRateSeed,
} from '../fixtures/db'
import {
  UNPRICED_NOTE,
  cardGain,
  cardValue,
  currencyPart,
  holdingCard,
  holdingCards,
  partGain,
  summaryRegion,
} from '../fixtures/holdings-page'

// 仕様: ドルを円にまとめる合計の要件 docs/requirements/fx-total.md の要件1〜要件5・要件10・要件11・要件16・非機能1 と、成功基準の
//   基準1・基準4・基準5・基準10・基準15（E2E）・基準18（性能）。
// テスト計画: プラン docs/plans/fx-total/plan.md 5.1 の ② の E2E の行（fx-total.spec.ts）。
//   - レートは SQL で入れる（insertHoldingsData の usdJpyRates。レートの日付はそのまま使い、「今日」は使わない）。
//   - 画面の要素（プラン 4.7）: 切り替えは region「合計」の中の group「合計の表示」にある「通貨ごと」「円にまとめる」のボタン（aria-pressed）。
//     円にまとめた部分には「円」「ドル」の見出し（h3）・region がなく、レートの行「1ドル = <レート>円（<日付> のレート）で換算」がある。
//   - 覚えた表示は localStorage の 'zero.holdings.totalsView'（値 'byCurrency' か 'combinedYen'。プラン 4.5）。
//   - 性能の測り方は holdings.spec.ts の売買履歴の基準31 と同じ（ウォームアップの後、新しいコンテキストで performance.now()）。
// 実行の前提: stock-holdings のプラン 4.5「E2E」（zero_e2e、API は利用者A で起動、beforeEach でデータを入れ替える）。
// テストの名前の先頭は「ドルを円にまとめる基準N」（ほかの機能の基準と区別するため）。

test.describe.configure({ mode: 'default' })

test.beforeEach(async () => {
  await resetHoldingsData()
})

const HOLDINGS_PATH = '/holdings'
const STORAGE_KEY = 'zero.holdings.totalsView'
const SWITCH_LABEL = '合計の表示'
const BY_CURRENCY = '通貨ごと'
const COMBINED_YEN = '円にまとめる'

// ---- テストデータ（要件定義「成功基準」の冒頭。銘柄マスタとダミー価格は _test-data.md。取引日は 2026-01-10 の買い） ----
const BUY_DATE = '2026-01-10'
/** 7203「買 2,500円 100株」。 */
const TOYOTA_100: TradeSeed = { code: '7203', side: 'buy', quantity: 100, price: '2500.0', tradeDate: BUY_DATE }
/** 7203 の保存済みの終値（3,000円）。 */
const TOYOTA_PRICE: StockPriceSeed = { code: '7203', tradingDate: '2026-10-06', closePrice: '3000.0' }
const APPLE: StockSeed = { code: 'AAPL', name: 'Apple', currency: 'USD' }
const MICROSOFT: StockSeed = { code: 'MSFT', name: 'Microsoft', currency: 'USD' }
/** AAPL「買 150.25ドル 10株」。 */
const APPLE_10: TradeSeed = { code: 'AAPL', side: 'buy', quantity: 10, price: '150.25', tradeDate: BUY_DATE }
/** AAPL の保存済みの終値（180.00ドル）。 */
const APPLE_PRICE: StockPriceSeed = { code: 'AAPL', tradingDate: '2026-10-06', closePrice: '180.00' }
/** MSFT「買 400.00ドル 5株」（価格は入れない）。 */
const MICROSOFT_5: TradeSeed = { code: 'MSFT', side: 'buy', quantity: 5, price: '400.00', tradeDate: BUY_DATE }
/** レート: 2026-11-14 150.00円。 */
const RATE: UsdJpyRateSeed = { date: '2026-11-14', rate: '150.00' }

/** 基準1 のデータ: 7203・AAPL（どちらも価格が保存済み）とレート。 */
async function insertCriterion1Data(): Promise<void> {
  await insertHoldingsData({
    stocks: [APPLE],
    trades: [TOYOTA_100, APPLE_10],
    prices: [TOYOTA_PRICE, APPLE_PRICE],
    usdJpyRates: [RATE],
  })
}

// ---- 画面の要素 ----
function switchGroup(page: Page): Locator {
  return summaryRegion(page).getByRole('group', { name: SWITCH_LABEL, exact: true })
}

function switchButton(page: Page, name: typeof BY_CURRENCY | typeof COMBINED_YEN): Locator {
  return switchGroup(page).getByRole('button', { name, exact: true })
}

/** 基準1 の円にまとめた表示（「含み損益 +94,625円（+19.9%）/ 評価額 570,000円 / 1ドル = 150.00円（2026-11-14 のレート）で換算」）が出ている。 */
async function expectCombinedYenShown(page: Page): Promise<void> {
  const summary = summaryRegion(page)
  await expect(summary.locator('[data-tone]')).toHaveCount(1)
  await expect(summary.locator('[data-tone]')).toHaveText('+94,625円（+19.9%）')
  await expect(summary.getByText('570,000円', { exact: true })).toHaveCount(1)
  await expect(summary.getByText('1ドル = 150.00円（2026-11-14 のレート）で換算', { exact: true })).toHaveCount(1)
  // 「円」「ドル」の見出しも、通貨ごとの部分もない。
  await expect(summary.getByRole('heading', { level: 3 })).toHaveCount(0)
  await expect(currencyPart(page, '円')).toHaveCount(0)
  await expect(currencyPart(page, 'ドル')).toHaveCount(0)
}

/** 通貨ごとの表示（カード表示の基準14。円「+50,000円（+20.0%）/ 300,000円」、ドル「+297.50ドル（+19.8%）/ 1,800.00ドル」）が出ている。 */
async function expectByCurrencyShown(page: Page): Promise<void> {
  const yen = currencyPart(page, '円')
  const usd = currencyPart(page, 'ドル')
  await expect(yen).toHaveCount(1)
  await expect(usd).toHaveCount(1)
  await expect(partGain(yen)).toHaveText('+50,000円（+20.0%）')
  await expect(yen.getByText('300,000円', { exact: true })).toHaveCount(1)
  await expect(partGain(usd)).toHaveText('+297.50ドル（+19.8%）')
  await expect(usd.getByText('1,800.00ドル', { exact: true })).toHaveCount(1)
  await expect(summaryRegion(page).getByText('で換算')).toHaveCount(0)
}

test('ドルを円にまとめる基準1（要件1・要件3・要件4・要件6〜要件10）: 7203・AAPL を保有し、レートが 2026-11-14 150.00円のとき、「円にまとめる」を押すと合計の欄が「+94,625円（+19.9%）/ 570,000円 / 1ドル = 150.00円（2026-11-14 のレート）で換算」になり「円」「ドル」の見出しはなく、「通貨ごと」を押すと通貨ごとの表示に戻る', async ({ page }) => {
  await insertCriterion1Data()

  await page.goto(HOLDINGS_PATH)

  // 最初は「通貨ごと」（何も覚えていない）。切り替えの2つのボタンがあり、「通貨ごと」だけが押されている。
  await expect(switchGroup(page)).toHaveCount(1)
  await expect(switchGroup(page).getByRole('button')).toHaveCount(2)
  await expect(switchButton(page, BY_CURRENCY)).toHaveAttribute('aria-pressed', 'true')
  await expect(switchButton(page, COMBINED_YEN)).toHaveAttribute('aria-pressed', 'false')
  await expectByCurrencyShown(page)

  // 「円にまとめる」を押す。
  await switchButton(page, COMBINED_YEN).click()
  await expect(switchButton(page, COMBINED_YEN)).toHaveAttribute('aria-pressed', 'true')
  await expect(switchButton(page, BY_CURRENCY)).toHaveAttribute('aria-pressed', 'false')
  await expectCombinedYenShown(page)
  // 価格のない銘柄がないので、注記は出ない。
  await expect(summaryRegion(page).getByText(UNPRICED_NOTE)).toHaveCount(0)

  // 「通貨ごと」を押すと、通貨ごとの表示に戻る。
  await switchButton(page, BY_CURRENCY).click()
  await expect(switchButton(page, BY_CURRENCY)).toHaveAttribute('aria-pressed', 'true')
  await expectByCurrencyShown(page)
})

test('ドルを円にまとめる基準1（要件1。幅 390px）: 幅 390px の画面で開くと、切り替えの組は見出し「合計」と同じ行にある（縦の範囲が重なる）', async ({ page }) => {
  await insertCriterion1Data()
  await page.setViewportSize({ width: 390, height: 844 })

  await page.goto(HOLDINGS_PATH)

  await expect(switchGroup(page)).toHaveCount(1)
  const heading = summaryRegion(page).getByRole('heading', { level: 2, name: '合計', exact: true })
  await expect(heading).toHaveCount(1)
  const headingBox = await heading.boundingBox()
  const groupBox = await switchGroup(page).boundingBox()
  if (headingBox === null || groupBox === null) throw new Error('見出しか切り替えの位置を取れない')
  // 縦の範囲が重なる（同じ行に収まる。折り返して下の行に落ちていない）。
  expect(groupBox.y, '切り替えの上端が見出しの下端より上').toBeLessThan(headingBox.y + headingBox.height)
  expect(headingBox.y, '見出しの上端が切り替えの下端より上').toBeLessThan(groupBox.y + groupBox.height)
  // 切り替えは画面の幅に収まる。
  expect(groupBox.x + groupBox.width, '切り替えの右端が画面の幅に収まる').toBeLessThanOrEqual(390)
})

test('ドルを円にまとめる基準4（要件2）: 7203・AAPL を保有していても DB にレートがないとき、/holdings を開くと切り替えはなく、合計の欄は円「+50,000円（+20.0%）/ 300,000円」とドル「+297.50ドル（+19.8%）/ 1,800.00ドル」の通貨ごとの表示になる', async ({ page }) => {
  await insertHoldingsData({
    stocks: [APPLE],
    trades: [TOYOTA_100, APPLE_10],
    prices: [TOYOTA_PRICE, APPLE_PRICE],
  })

  await page.goto(HOLDINGS_PATH)

  await expectByCurrencyShown(page)
  await expect(switchGroup(page)).toHaveCount(0)
  await expect(page.getByRole('button', { name: BY_CURRENCY, exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: COMBINED_YEN, exact: true })).toHaveCount(0)
})

test('ドルを円にまとめる基準5（要件5）: 「円にまとめる」を押しても、AAPL のカードは「+297.50ドル（+19.8%）」「1,800.00ドル」のままで、カードの並びは変わらず、押してから 500ms の間に /api/ への要求は送られない', async ({ page }) => {
  await insertCriterion1Data()
  await page.goto(HOLDINGS_PATH)
  await expect(holdingCards(page)).toHaveCount(2)
  const cardsBefore = await holdingCards(page).allInnerTexts()
  expect(cardsBefore, 'Given の確認: AAPL のカードは通貨ごとの表示でもドルで表示されている').toEqual(
    expect.arrayContaining([expect.stringContaining('+297.50ドル（+19.8%）')]),
  )

  // 押す前から要求を数える（押してから 500ms の間の /api/ への要求だけを数える）。
  const apiRequests: string[] = []
  page.on('request', (request) => {
    if (new URL(request.url()).pathname.startsWith('/api/')) apiRequests.push(request.url())
  })

  await switchButton(page, COMBINED_YEN).click()
  await expectCombinedYenShown(page)
  await page.waitForTimeout(500)

  const aapl = holdingCard(page, 'AAPL')
  await expect(cardGain(aapl)).toHaveText('+297.50ドル（+19.8%）')
  await expect(cardValue(aapl, '評価額')).toHaveText('1,800.00ドル')
  expect(await holdingCards(page).allInnerTexts(), 'カードの表示と並びは変わらない').toEqual(cardsBefore)
  expect(apiRequests, '切り替えはサーバーへの要求を送らない').toEqual([])
})

test('ドルを円にまとめる基準10（要件11）: 7203・AAPL（価格保存済み）と MSFT（価格なし）を保有しているとき、「円にまとめる」を押すと、基準1 と同じ値と「価格を取得できない銘柄を含みません」が1つ表示される', async ({ page }) => {
  await insertHoldingsData({
    stocks: [APPLE, MICROSOFT],
    trades: [TOYOTA_100, APPLE_10, MICROSOFT_5],
    prices: [TOYOTA_PRICE, APPLE_PRICE],
    usdJpyRates: [RATE],
  })

  await page.goto(HOLDINGS_PATH)
  await switchButton(page, COMBINED_YEN).click()

  await expectCombinedYenShown(page)
  await expect(summaryRegion(page).getByText(UNPRICED_NOTE, { exact: true })).toHaveCount(1)
  await expect(page.getByText(UNPRICED_NOTE, { exact: true })).toHaveCount(1)
})

test('ドルを円にまとめる基準15（要件16）: 「円にまとめる」を押して再読み込みすると、「円にまとめる」が押された状態で、合計の欄は基準1 の円にまとめた表示になる', async ({ page }) => {
  await insertCriterion1Data()
  await page.goto(HOLDINGS_PATH)
  await switchButton(page, COMBINED_YEN).click()
  await expectCombinedYenShown(page)

  await page.reload()

  await expect(switchButton(page, COMBINED_YEN)).toHaveAttribute('aria-pressed', 'true')
  await expect(switchButton(page, BY_CURRENCY)).toHaveAttribute('aria-pressed', 'false')
  await expectCombinedYenShown(page)
})

test('ドルを円にまとめる基準18（非機能1）: 100銘柄（円50・ドル50。各20件、すべて価格が保存済み）とレート1,000件があり「円にまとめる」を覚えているとき、/holdings を開いてからカード100枚と円にまとめた合計の欄（「で換算」の文）が表示されるまで2秒以内（計測値を annotation と console.log に残す）', async ({ page, browser }) => {
  // Given: 売買履歴の非機能1 のデータ（holdings.spec.ts の基準31 と同じ。100銘柄、各20件、すべて価格が保存済み）に、レートを1,000件足す。
  // 買い16件（各100株）と売り4件（各10株）。取引日の昇順・同じ日は買いが先（途中で保有数量が負にならない）。保有数量は 1,560株。
  const yenCodes = Array.from({ length: 50 }, (_, i) => String(1001 + i))
  const usdCodes = Array.from({ length: 50 }, (_, i) => `US${String(i + 1).padStart(2, '0')}`)
  const stocks: StockSeed[] = [
    ...yenCodes.map((code) => ({ code, name: `E2E銘柄${code}`, currency: 'JPY' as const })),
    ...usdCodes.map((code) => ({ code, name: `E2E銘柄${code}`, currency: 'USD' as const })),
  ]
  const allCodes = [...yenCodes, ...usdCodes]
  const tradesOf = (code: string, currency: 'JPY' | 'USD'): TradeSeed[] =>
    Array.from({ length: 20 }, (_, n) => {
      const isSell = n >= 16
      const day = String(n + 1).padStart(2, '0')
      const price = currency === 'JPY' ? `${2400 + n}.0` : `${140 + n}.25`
      return {
        code,
        side: isSell ? ('sell' as const) : ('buy' as const),
        quantity: isSell ? 10 : 100,
        price,
        tradeDate: `2026-02-${day}`,
      }
    })
  // レート1,000件: 2023-01-01 から1日ずつ。日付ごとに1件で、レートは 100.00 から 0.01 円ずつ上がる（最後に入れた、いちばん新しい日付のレートは 109.99円）。
  const usdJpyRates: UsdJpyRateSeed[] = Array.from({ length: 1000 }, (_, i) => ({
    date: new Date(Date.UTC(2023, 0, 1) + i * 24 * 60 * 60 * 1000).toISOString().slice(0, 10),
    rate: (100 + i / 100).toFixed(2),
  }))
  await insertHoldingsData({
    stocks,
    trades: [
      ...yenCodes.flatMap((code) => tradesOf(code, 'JPY')),
      ...usdCodes.flatMap((code) => tradesOf(code, 'USD')),
    ],
    prices: [
      ...yenCodes.map((code) => ({ code, tradingDate: '2026-10-06', closePrice: '3000.0' })),
      ...usdCodes.map((code) => ({ code, tradingDate: '2026-10-06', closePrice: '180.00' })),
    ],
    usdJpyRates,
  })
  // 「円にまとめる」を覚えている（ページのスクリプトより前に localStorage に入れる）。
  // about:blank などの localStorage を読めない文書でも例外にしない（読めたときだけ入れる）。
  const rememberCombinedYen = (key: string): void => {
    try {
      window.localStorage.setItem(key, 'combinedYen')
    } catch {
      // 読めない文書。実際の /holdings の文書では読める。
    }
  }

  // ウォームアップ: 一度 /holdings を開いて捨てる（dev サーバーの初回の変換などを計測に含めない）。
  await page.addInitScript(rememberCombinedYen, STORAGE_KEY)
  await page.goto(HOLDINGS_PATH)
  await expect(holdingCards(page)).toHaveCount(allCodes.length)
  await page.close()

  // 計測: 新しいブラウザコンテキストで開き直す（ウォームアップのページのキャッシュを使わない）。
  const context = await browser.newContext()
  try {
    await context.addInitScript(rememberCombinedYen, STORAGE_KEY)
    const measuredPage = await context.newPage()
    await measuredPage.goto(HOLDINGS_PATH)
    // カードが100枚あり、かつ合計の欄に円にまとめた表示のレートの行（「で換算」の文）がある時点の performance.now()
    // （このページを開き始めてからの経過ミリ秒）を、描画のフレームごとに確かめる。
    const shownAt = await measuredPage.waitForFunction(
      (cardCount) =>
        document.querySelectorAll('ul[aria-label="保有株式"] > li').length >= cardCount &&
        Array.from(document.querySelectorAll('section')).some((section) => (section.textContent ?? '').includes('で換算')) &&
        performance.now(),
      allCodes.length,
      { polling: 'raf', timeout: 10_000 },
    )
    const elapsedMs = await shownAt.jsonValue()

    // 計測値と 2000ms までの余裕が後から分かるよう、判定の前に annotation と標準出力に残す（失敗したときも残る）。
    const measuredMs = Number(elapsedMs)
    const summaryText = `${measuredMs.toFixed(1)}ms（上限 2000ms、余裕 ${(2000 - measuredMs).toFixed(1)}ms）`
    test.info().annotations.push({ type: '基準18 計測値', description: summaryText })
    console.log(`基準18 計測値: ${summaryText}`)

    expect(elapsedMs).toBeLessThanOrEqual(2000)
    // 出ていたのは入れた100銘柄の一覧と、いちばん新しい日付（最後に入れた 109.99円）のレートの円にまとめた表示。
    const latest = usdJpyRates[usdJpyRates.length - 1]
    expect(latest.rate).toBe('109.99')
    await expect(holdingCards(measuredPage)).toHaveCount(allCodes.length)
    await expect(
      summaryRegion(measuredPage).getByText(`1ドル = 109.99円（${latest.date} のレート）で換算`, { exact: true }),
    ).toHaveCount(1)
    await expect(measuredPage.getByRole('region', { name: '合計', exact: true }).getByRole('region')).toHaveCount(0)
  } finally {
    await context.close()
  }
})
