import { expect, test, type Locator, type Page } from '@playwright/test'
import { insertHoldingsData, resetHoldingsData, type StockPriceSeed, type TradeSeed } from '../fixtures/db'
import { cardList, holdingCard, holdingCards } from '../fixtures/holdings-page'
import { centerOf, mouseDragTo, touchSession } from '../fixtures/pointer'

// 仕様: 銘柄ごとの売買履歴の画面の要件 docs/requirements/stock-history.md の要件1〜要件5・要件14〜要件20・要件22 と、成功基準の基準1・基準2・基準4・基準5・基準6・基準16・基準20・基準21・基準23・基準27（E2E）。
// テスト計画: プラン docs/plans/stock-history/plan.md 5.1（PR② の E2E の行）と 5.2。
//   - 画面の約束（プラン 4.6・4.8）: /holdings/<銘柄コード>。左上に「← 保有株式一覧」のリンク、その下に見出し「<銘柄コード> <銘柄名>」。
//     チャートは <svg role="img" aria-label="<銘柄コード> <銘柄名>の価格のチャート">（線は data-chart-part="line"）。表は <table>（見出しの行と、1行が1つの <tr>）。
//     売買履歴がなければ「この銘柄の売買履歴はありません」と「← 保有株式一覧」だけ。
//   - タッチは Chromium のタッチの模擬（CDP の Input.dispatchTouchEvent）で確かめる。
//     ステップ4 の最初に試した結果: タップ（touchStart と touchEnd）の後に、Chromium は互換の mousedown・mouseup・click を送る
//     （touchend を preventDefault すると送られない）。そのため、基準2 では、戻るを1回押すだけで /holdings に戻ること（履歴に同じ画面が2つ積まれていないこと）も確かめる。
// 実行の前提: stock-holdings のプラン 4.5「E2E」（zero_e2e、API は利用者A で起動、beforeEach でデータを入れ替える）。
// テストの名前の先頭は「銘柄ごとの売買履歴の基準N」「銘柄ごとの売買履歴の補助」（既存の要件の基準と区別するため）。

test.describe.configure({ mode: 'default' })

test.beforeEach(async () => {
  await resetHoldingsData()
})

const HOLDINGS_PATH = '/holdings'
const SAVE_PATH = '/api/holding-order'
const BACK_LINK = '← 保有株式一覧'
const NO_HISTORY_MESSAGE = 'この銘柄の売買履歴はありません'
const NO_PRICES_MESSAGE = 'この期間の価格はまだありません'

// ---- テストデータ（要件定義「成功基準」の銘柄マスタとダミー価格） ----
const BUY_DATE = '2026-01-10'
const TOYOTA_BUY: TradeSeed = { code: '7203', side: 'buy', quantity: 100, price: '2500.0', tradeDate: BUY_DATE }
const SONY_BUY: TradeSeed = { code: '6758', side: 'buy', quantity: 10, price: '3200.0', tradeDate: BUY_DATE }
const DUMMY_PRICES: StockPriceSeed[] = [
  { code: '7203', tradingDate: '2026-10-06', closePrice: '3000.0' },
  { code: '6758', tradingDate: '2026-10-06', closePrice: '3500.0' },
]

/** 基準1 の Given: Aが 7203「買 2,500円 100株 2026-01-10」と 6758「買 3,200円 10株 2026-01-10」を持つ。 */
async function seedTwoHoldings(): Promise<void> {
  await insertHoldingsData({ trades: [TOYOTA_BUY, SONY_BUY], prices: DUMMY_PRICES })
}

// ---- 読む・操作する補助 ----

function backLink(page: Page): Locator {
  return page.getByRole('link', { name: BACK_LINK, exact: true })
}

/** チャート（<svg role="img">）。 */
function chartOf(page: Page, label: string): Locator {
  return page.getByRole('img', { name: label, exact: true })
}

/** 見出しの行を含む、表のすべての行。 */
function tableRows(page: Page): Locator {
  return page.getByRole('row')
}

/** 行のセル（取引日・売買・株数・価格・金額。td でも th でも、行の直下の要素）。 */
function cellsOf(row: Locator): Locator {
  return row.locator(':scope > *')
}

/** 表の見出しの行が「取引日 / 売買 / 株数 / 価格 / 金額」で、その下の行が rows のとおり（この順で、行数も同じ）であることを確かめる。 */
async function expectTable(page: Page, rows: readonly (readonly string[])[]): Promise<void> {
  const all = tableRows(page)
  await expect(all).toHaveCount(rows.length + 1)
  await expect(cellsOf(all.nth(0))).toHaveText(['取引日', '売買', '株数', '価格', '金額'])
  for (const [index, cells] of rows.entries()) {
    await expect(cellsOf(all.nth(index + 1)), `${index + 1} 行目`).toHaveText([...cells])
  }
}

/** カードの並びを DOM の順に読む（カードの文字は銘柄コードから始まる。コードはすべて4文字）。 */
async function domOrder(page: Page): Promise<string[]> {
  return holdingCards(page).evaluateAll((items) => items.map((item) => (item.textContent ?? '').trim().slice(0, 4)))
}

async function expectOrder(page: Page, codes: readonly string[]): Promise<void> {
  await expect(holdingCards(page)).toHaveCount(codes.length)
  await expect.poll(() => domOrder(page), { message: 'カードの並び（DOM の順）' }).toEqual(codes)
}

/** 読み上げ用のライブリージョン（HoldingCardList が持つ）。 */
function liveRegion(page: Page): Locator {
  return page.locator('[role="status"][aria-live="assertive"]')
}

/** 並びの保存の要求（PUT /api/holding-order）の本文の codes を、最初から記録する（goto の前に呼ぶ）。 */
function recordSaves(page: Page): string[][] {
  const saves: string[][] = []
  page.on('request', (request) => {
    if (request.method() === 'PUT' && new URL(request.url()).pathname === SAVE_PATH) {
      saves.push((JSON.parse(request.postData() ?? '{}') as { codes: string[] }).codes)
    }
  })
  return saves
}

/** 日付 start（"YYYY-MM-DD"）の days 日後の日付（"YYYY-MM-DD"）。 */
function addDays(start: string, days: number): string {
  const date = new Date(`${start}T00:00:00Z`)
  date.setUTCDate(date.getUTCDate() + days)
  return date.toISOString().slice(0, 10)
}

// =====================================================================================================
// 画面の開き方と戻り方（基準1・基準2・基準4・基準5）
// =====================================================================================================

test('銘柄ごとの売買履歴の基準1（要件1・要件4）: Aが 7203「買 2,500円 100株 2026-01-10」と 6758「買 3,200円 10株 2026-01-10」を持ち /holdings を開いているとき、7203 のカードをクリックすると /holdings/7203 に移り、見出しに「7203 トヨタ自動車」が出る。「← 保有株式一覧」を押すと /holdings に戻る', async ({ page }) => {
  await seedTwoHoldings()
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', '7203'])

  await holdingCard(page, '7203').click()

  await expect(page).toHaveURL(/\/holdings\/7203$/)
  await expect(page.getByRole('heading', { name: '7203 トヨタ自動車', exact: true })).toBeVisible()

  await backLink(page).click()

  await expect(page).toHaveURL(/\/holdings$/)
  await expect(cardList(page)).toBeVisible()
  await expectOrder(page, ['6758', '7203'])
})

test.describe('タッチの画面（幅 390px）', () => {
  test.use({ hasTouch: true, viewport: { width: 390, height: 844 } })

  test('銘柄ごとの売買履歴の基準2（要件1）: 基準1 と同じで、幅 390px のタッチの画面で 6758 のカードをタップすると /holdings/6758 に移る。履歴に同じ画面が2つ積まれず、戻るを1回押すと /holdings に戻る', async ({ page }) => {
    await seedTwoHoldings()
    await page.goto(HOLDINGS_PATH)
    await expectOrder(page, ['6758', '7203'])
    const historyLengthBefore = await page.evaluate(() => window.history.length)

    await holdingCard(page, '6758').tap()

    await expect(page).toHaveURL(/\/holdings\/6758$/)
    await expect(page.getByRole('heading', { name: '6758 ソニーグループ', exact: true })).toBeVisible()
    // タップの後にブラウザが送る互換の mousedown・mouseup でも画面を移すと、同じ画面が履歴に2つ積まれる（ステップ4 で、互換のイベントが送られることを確かめた）。
    // 互換のイベントはタップの直後に届くので、少し待ってから数える。
    await page.waitForTimeout(300)
    expect(await page.evaluate(() => window.history.length), '履歴に積まれたのは1つだけ').toBe(historyLengthBefore + 1)

    await page.goBack()

    await expect(page).toHaveURL(/\/holdings$/)
    await expect(cardList(page)).toBeVisible()
  })

  test('銘柄ごとの売買履歴の基準5（要件3）: 基準1 と同じで、幅 390px のタッチの画面で 7203 のカードを0.25秒以上（400ms）押し続けてから、指を動かさずに離しても、/holdings のままで、並びも変わらず、並びの保存の要求も送られない', async ({ page }) => {
    const saves = recordSaves(page)
    await seedTwoHoldings()
    // 2枚のカードが同じ画面に入る高さにする（カード表示の E2E と同じ）。
    await page.setViewportSize({ width: 390, height: 1800 })
    await page.goto(HOLDINGS_PATH)
    await expectOrder(page, ['6758', '7203'])
    const touch = await touchSession(page)

    const toyota = await centerOf(holdingCard(page, '7203'))
    await touch.start(toyota.x, toyota.y)
    await page.waitForTimeout(400)
    // 長押しでドラッグが始まっていたこと（始まっていないと、離しても何も起きないので、確かめたいことを確かめられない）。
    await expect(liveRegion(page)).toContainText('持ち上げました')
    await touch.end()
    // 離した後に画面が移るなら、少し待てば移る。
    await page.waitForTimeout(300)

    await expect(page).toHaveURL(/\/holdings$/)
    await expect(page.getByRole('heading', { name: '7203 トヨタ自動車', exact: true })).toHaveCount(0)
    await expectOrder(page, ['6758', '7203'])
    expect(saves, '並びの保存の要求は送られない').toEqual([])
  })
})

test('銘柄ごとの売買履歴の基準4（要件3）: 基準1 と同じで、7203 のカードの「⋯」（「トヨタ自動車の操作」）を押すと「削除」のメニューが開き /holdings のまま。7203 のカードをマウスでドラッグして 6758 のカードの位置で離すと、並びが 7203、6758 になり、/holdings のまま', async ({ page }) => {
  await seedTwoHoldings()
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', '7203'])

  await holdingCard(page, '7203').getByRole('button', { name: 'トヨタ自動車の操作', exact: true }).click()

  await expect(page.getByRole('menuitem', { name: '削除', exact: true })).toBeVisible()
  await expect(page).toHaveURL(/\/holdings$/)
  await page.keyboard.press('Escape')
  await expect(page.getByRole('menuitem', { name: '削除', exact: true })).toHaveCount(0)

  const saved = page.waitForResponse(
    (response) => response.request().method() === 'PUT' && new URL(response.url()).pathname === SAVE_PATH,
  )
  // テストが途中で失敗して await されないまま終わったときの「Test ended」を、本当の失敗の原因に混ぜない。
  saved.catch(() => {})
  await mouseDragTo(page, holdingCard(page, '7203'), await centerOf(holdingCard(page, '6758')))
  await page.mouse.up()

  await expectOrder(page, ['7203', '6758'])
  expect((await saved).status(), '並びの保存の HTTP ステータス').toBe(204)
  await expect(page).toHaveURL(/\/holdings$/)
  await expect(page.getByRole('heading', { name: '7203 トヨタ自動車', exact: true })).toHaveCount(0)
})

// =====================================================================================================
// 画面の配置（基準6）
// =====================================================================================================

test('銘柄ごとの売買履歴の基準6（要件5）: Aが 7203 の売買履歴を50件持つとき、幅 390px・高さ 844px の画面でも、幅 1,280px・高さ 800px の画面でも、/holdings/7203 を開いて表を最後までスクロールすると、表の最後の行（いちばん古い売買）が見え、チャートは画面の中に見えたままで、画面全体のスクロールの位置は0のまま', async ({ page }) => {
  // 取引日が 2026-01-01 から1日ずつ後の、50件の買い（いちばん古い売買は 2026-01-01）。
  const trades: TradeSeed[] = Array.from({ length: 50 }, (_, index) => ({
    code: '7203',
    side: 'buy',
    quantity: 10,
    price: '2500.0',
    tradeDate: addDays('2026-01-01', index),
  }))
  await insertHoldingsData({ trades, prices: DUMMY_PRICES })

  for (const viewport of [
    { width: 390, height: 844 },
    { width: 1280, height: 800 },
  ]) {
    await page.setViewportSize(viewport)
    await page.goto('/holdings/7203')
    const chart = chartOf(page, '7203 トヨタ自動車の価格のチャート')
    await expect(chart, `${viewport.width}×${viewport.height}: チャートがある`).toBeVisible()
    // 見出しの行と、50件の行。
    await expect(tableRows(page)).toHaveCount(51)

    // 表を最後までスクロールする（表を縦にスクロールさせる箱を、表から外へたどって探す）。
    await page.getByRole('table').evaluate((table) => {
      for (let node: Element | null = table; node !== null; node = node.parentElement) {
        const overflowY = getComputedStyle(node).overflowY
        if ((overflowY === 'auto' || overflowY === 'scroll') && node.scrollHeight > node.clientHeight) {
          node.scrollTop = node.scrollHeight
          return
        }
      }
      throw new Error('表を縦にスクロールする箱がない（表だけが下半分の中でスクロールするはず）')
    })

    const lastRow = tableRows(page).last()
    await expect(lastRow, `${viewport.width}×${viewport.height}: 最後の行はいちばん古い売買`).toContainText('2026-01-01')
    await expect(lastRow, `${viewport.width}×${viewport.height}: 最後の行が見える`).toBeInViewport({ ratio: 1 })
    await expect(chart, `${viewport.width}×${viewport.height}: チャートは画面の中に見えたまま`).toBeInViewport({ ratio: 1 })
    expect(await page.evaluate(() => window.scrollY), `${viewport.width}×${viewport.height}: 画面全体のスクロールの位置`).toBe(0)
    // 画面全体はスクロールさせない（要件5）。
    const heights = await page.evaluate(() => ({
      scrollHeight: document.documentElement.scrollHeight,
      innerHeight: window.innerHeight,
    }))
    expect(heights.scrollHeight, `${viewport.width}×${viewport.height}: 画面全体の高さが画面の高さを超えない`).toBeLessThanOrEqual(heights.innerHeight)
  }
})

// =====================================================================================================
// 吹き出しがチャートの外へはみ出さない（要件5。ステップ6 1ラウンド目のデザイン1・エッジケース4）
// =====================================================================================================

test.describe('吹き出しの置き場所（幅 390px・高さ 844px のタッチの画面）', () => {
  test.use({ hasTouch: true, viewport: { width: 390, height: 844 } })

  /**
   * 印を1つずつタップして吹き出しを出し、そのたびに、画面全体が横にスクロールせず（scrollWidth <= clientWidth）、
   * 吹き出しの箱がチャートの箱（.price-chart__canvas）の中に収まっていることを確かめる。
   * 印は、チャートの左端から右端まで散らす（横の真ん中あたりの印は、吹き出しを右へ出すか左へ出すかの境目にあたる）。
   */
  async function expectTooltipsInsideChart(page: Page, label: string, markerCount: number): Promise<void> {
    const chart = chartOf(page, label)
    await expect(chart).toBeVisible()
    const markers = chart.locator('[data-chart-part="marker"]')
    await expect(markers).toHaveCount(markerCount)
    const canvas = page.locator('.price-chart__canvas')
    const tooltip = page.getByRole('tooltip')

    for (let index = 0; index < markerCount; index += 1) {
      const date = await markers.nth(index).getAttribute('data-date')
      // 同じ日の印は重なって描かれる。いちばん上（同じ日の最後）の印をタップする（下の印は、上の印に隠れてタップできない）。
      await chart.locator(`[data-chart-part="marker"][data-date="${date}"]`).last().tap()
      await expect(tooltip, `${date} の印をタップすると吹き出しが出る`).toBeVisible()

      const canvasBox = await canvas.boundingBox()
      const tooltipBox = await tooltip.boundingBox()
      if (canvasBox === null || tooltipBox === null) throw new Error('チャートか吹き出しの位置を取れない')
      const message = `${date} の吹き出し`
      expect(tooltipBox.x, `${message}: 左端がチャートの中`).toBeGreaterThanOrEqual(canvasBox.x - 0.5)
      expect(tooltipBox.y, `${message}: 上端がチャートの中`).toBeGreaterThanOrEqual(canvasBox.y - 0.5)
      expect(tooltipBox.x + tooltipBox.width, `${message}: 右端がチャートの中`).toBeLessThanOrEqual(canvasBox.x + canvasBox.width + 0.5)
      expect(tooltipBox.y + tooltipBox.height, `${message}: 下端がチャートの中`).toBeLessThanOrEqual(canvasBox.y + canvasBox.height + 0.5)
      const scroll = await page.evaluate(() => ({
        scrollWidth: document.documentElement.scrollWidth,
        clientWidth: document.documentElement.clientWidth,
      }))
      expect(scroll.scrollWidth, `${message}: 画面全体が横にスクロールしない`).toBeLessThanOrEqual(scroll.clientWidth)
    }
  }

  test('銘柄ごとの売買履歴の補助（要件5・要件11）: 円の銘柄 7203 で、2026-01-01 から 2026-03-02 に散らした7つの印（桁の多い株数で、吹き出しを長くする。価格は範囲の中でいちばん安いもの・いちばん高いものを混ぜ、チャートの下の端と上の端の近くにも印を置く）を順にタップすると、どの吹き出しも、画面全体を横にスクロールさせず、チャートの箱の中に収まる', async ({ page }) => {
    // 範囲は 60日（2026-01-01〜2026-03-02）。1/22 は35%、1/31 は50%、2/14 は73%、2/25 は92% あたり。
    // 価格は 12,000円（範囲でいちばん安い。下の端）と 13,000円（いちばん高い。上の端）を、横の位置の違う印に混ぜる。
    // いちばん安い印（2026-01-01）には、同じ日の2件目を足して、吹き出しを2行にする（1行だけだと、下の端の印の下にも収まってしまい、縦の置き場所を確かめられない）。
    const trades: TradeSeed[] = [
      { code: '7203', side: 'buy', quantity: 1000000, price: '12000.0', tradeDate: '2026-01-01' },
      { code: '7203', side: 'buy', quantity: 1000000, price: '12000.0', tradeDate: '2026-01-01' },
      { code: '7203', side: 'buy', quantity: 1000000, price: '13000.0', tradeDate: '2026-01-22' },
      { code: '7203', side: 'buy', quantity: 1000000, price: '12000.0', tradeDate: '2026-01-31' },
      { code: '7203', side: 'buy', quantity: 1000000, price: '13000.0', tradeDate: '2026-02-14' },
      { code: '7203', side: 'sell', quantity: 1000000, price: '12000.0', tradeDate: '2026-02-25' },
      { code: '7203', side: 'sell', quantity: 1000000, price: '13000.0', tradeDate: '2026-03-02' },
    ]
    await insertHoldingsData({
      trades,
      prices: [
        { code: '7203', tradingDate: '2026-01-01', closePrice: '12000.0' },
        { code: '7203', tradingDate: '2026-03-02', closePrice: '13000.0' },
      ],
    })

    await page.goto('/holdings/7203')

    await expectTooltipsInsideChart(page, '7203 トヨタ自動車の価格のチャート', trades.length)
  })

  test('銘柄ごとの売買履歴の補助（要件5・要件11）: ドルの銘柄 AAPL でも、価格をいちばん安いもの・いちばん高いものを混ぜた同じ7つの印を順にタップすると、どの吹き出しも、画面全体を横にスクロールさせず、チャートの箱の中に収まる', async ({ page }) => {
    const trades: TradeSeed[] = [
      { code: 'AAPL', side: 'buy', quantity: 1000000, price: '12000.00', tradeDate: '2026-01-01' },
      { code: 'AAPL', side: 'buy', quantity: 1000000, price: '12000.00', tradeDate: '2026-01-01' },
      { code: 'AAPL', side: 'buy', quantity: 1000000, price: '13000.00', tradeDate: '2026-01-22' },
      { code: 'AAPL', side: 'buy', quantity: 1000000, price: '12000.00', tradeDate: '2026-01-31' },
      { code: 'AAPL', side: 'buy', quantity: 1000000, price: '13000.00', tradeDate: '2026-02-14' },
      { code: 'AAPL', side: 'sell', quantity: 1000000, price: '12000.00', tradeDate: '2026-02-25' },
      { code: 'AAPL', side: 'sell', quantity: 1000000, price: '13000.00', tradeDate: '2026-03-02' },
    ]
    await insertHoldingsData({
      stocks: [{ code: 'AAPL', name: 'Apple', currency: 'USD' }],
      trades,
      prices: [
        { code: 'AAPL', tradingDate: '2026-01-01', closePrice: '12000.00' },
        { code: 'AAPL', tradingDate: '2026-03-02', closePrice: '13000.00' },
      ],
    })

    await page.goto('/holdings/AAPL')

    await expectTooltipsInsideChart(page, 'AAPL Appleの価格のチャート', trades.length)
  })

  test('銘柄ごとの売買履歴の補助（要件5・要件11。ステップ6 2ラウンド目のデザイン2-1・エッジケース2-3）: 円の銘柄 7203 で、同じ日（2026-02-01）の売買を10件入れ（その後に分割があり、吹き出しが20行になる）た印をタップすると、吹き出しがチャートの箱の中に（縦も）収まり、画面全体は縦にも横にもスクロールしない', async ({ page }) => {
    // 同じ日の10件（価格を少しずつ変えて、印が縦に散る）。10件だけの吹き出し（10行）は、この画面のチャートの箱（高さ約 326px）に収まってしまうので、
    // 分割（1→2、2026-03-01）を足し、10件のどれにも「分割を反映した価格」の行が付くようにする（吹き出しは20行で、箱より高い）。
    const sameDay: TradeSeed[] = Array.from({ length: 10 }, (_, index) => ({
      code: '7203',
      side: 'buy',
      quantity: 999999999,
      price: `${12000 + index * 100}.0`,
      tradeDate: '2026-02-01',
    }))
    await insertHoldingsData({
      trades: [{ code: '7203', side: 'buy', quantity: 1, price: '12000.0', tradeDate: '2026-01-01' }, ...sameDay],
      splits: [{ code: '7203', splitDate: '2026-03-01', ratio: 2 }],
      prices: [
        { code: '7203', tradingDate: '2026-01-01', closePrice: '12000.0' },
        { code: '7203', tradingDate: '2026-03-02', closePrice: '13000.0' },
      ],
    })

    await page.goto('/holdings/7203')

    const chart = chartOf(page, '7203 トヨタ自動車の価格のチャート')
    await expect(chart).toBeVisible()
    const sameDayMarkers = chart.locator('[data-chart-part="marker"][data-date="2026-02-01"]')
    await expect(sameDayMarkers).toHaveCount(10)
    // 印は重なって描かれる。いちばん上（DOM の最後）の印をタップする（ほかの印が受け取ってしまう場合を避ける）。
    await sameDayMarkers.last().tap()
    const tooltip = page.getByRole('tooltip')
    await expect(tooltip).toBeVisible()
    await expect(tooltip.locator('> *')).toHaveCount(20)

    const canvasBox = await page.locator('.price-chart__canvas').boundingBox()
    const tooltipBox = await tooltip.boundingBox()
    if (canvasBox === null || tooltipBox === null) throw new Error('チャートか吹き出しの位置を取れない')
    expect(tooltipBox.x, '左端がチャートの中').toBeGreaterThanOrEqual(canvasBox.x - 0.5)
    expect(tooltipBox.y, '上端がチャートの中').toBeGreaterThanOrEqual(canvasBox.y - 0.5)
    expect(tooltipBox.x + tooltipBox.width, '右端がチャートの中').toBeLessThanOrEqual(canvasBox.x + canvasBox.width + 0.5)
    expect(tooltipBox.y + tooltipBox.height, '下端がチャートの中').toBeLessThanOrEqual(canvasBox.y + canvasBox.height + 0.5)
    const scroll = await page.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
      scrollHeight: document.documentElement.scrollHeight,
      clientHeight: document.documentElement.clientHeight,
    }))
    expect(scroll.scrollWidth, '画面全体が横にスクロールしない').toBeLessThanOrEqual(scroll.clientWidth)
    expect(scroll.scrollHeight, '画面全体が縦にスクロールしない').toBeLessThanOrEqual(scroll.clientHeight)
  })
})

// =====================================================================================================
// 売買履歴の表（基準16・基準20）
// =====================================================================================================

test('銘柄ごとの売買履歴の基準16（要件14・要件15・要件18）: Aの 7203 の売買履歴が「買 2,480円 100株 2024-04-01」「買 2,600円 50株 2025-06-10」「売 3,050円 50株 2026-03-02」のとき（登録は 2025-06-10、2026-03-02、2024-04-01 の順）、Aが /holdings/7203 を開くと、表の見出しは「取引日 / 売買 / 株数 / 価格 / 金額」で、行は上から「2026-03-02 / 売 / 50株 / 3,050円 / 152,500円」「2025-06-10 / 買 / 50株 / 2,600円 / 130,000円」「2024-04-01 / 買 / 100株 / 2,480円 / 248,000円」', async ({ page }) => {
  // 登録した順と日付の順を変える（id の順で並べる誤りを見分けるため）。
  await insertHoldingsData({
    trades: [
      { code: '7203', side: 'buy', quantity: 50, price: '2600.0', tradeDate: '2025-06-10' },
      { code: '7203', side: 'sell', quantity: 50, price: '3050.0', tradeDate: '2026-03-02' },
      { code: '7203', side: 'buy', quantity: 100, price: '2480.0', tradeDate: '2024-04-01' },
    ],
  })

  await page.goto('/holdings/7203')

  await expectTable(page, [
    ['2026-03-02', '売', '50株', '3,050円', '152,500円'],
    ['2025-06-10', '買', '50株', '2,600円', '130,000円'],
    ['2024-04-01', '買', '100株', '2,480円', '248,000円'],
  ])
})

test('銘柄ごとの売買履歴の基準20（要件15・要件17・要件18）: 7203 の分割が「分割 1→5 2021-09-29」「分割 1→3 2026-03-31」で、Aの 7203 の売買履歴が「買 2,500円 100株 2026-01-10」「買 1,000円 30株 2026-03-31」のとき、Aが /holdings/7203 を開くと、行は上から「2026-03-31 / 買 / 30株 / 1,000円 / 30,000円」「2026-03-31 / 分割 / 1株→3株 / （空） / （空）」「2026-01-10 / 買 / 100株 / 2,500円 / 250,000円」の3行で、2021-09-29 の分割の行は出ない', async ({ page }) => {
  await insertHoldingsData({
    trades: [
      { code: '7203', side: 'buy', quantity: 100, price: '2500.0', tradeDate: '2026-01-10' },
      { code: '7203', side: 'buy', quantity: 30, price: '1000.0', tradeDate: '2026-03-31' },
    ],
    splits: [
      { code: '7203', splitDate: '2021-09-29', ratio: 5 },
      { code: '7203', splitDate: '2026-03-31', ratio: 3 },
    ],
  })

  await page.goto('/holdings/7203')

  await expectTable(page, [
    ['2026-03-31', '買', '30株', '1,000円', '30,000円'],
    ['2026-03-31', '分割', '1株→3株', '', ''],
    ['2026-01-10', '買', '100株', '2,500円', '250,000円'],
  ])
})

// =====================================================================================================
// 状態・異常系・権限（基準21・基準23）
// =====================================================================================================

test('銘柄ごとの売買履歴の基準21（要件19）: Aの 6758 の売買履歴が「買 3,200円 10株 2026-01-10」「売 3,500円 10株 2026-02-10」（売り切った）のとき、Aが /holdings/6758 を開くと、見出し「6758 ソニーグループ」、チャート、2行の表が出る', async ({ page }) => {
  await insertHoldingsData({
    trades: [
      { code: '6758', side: 'buy', quantity: 10, price: '3200.0', tradeDate: '2026-01-10' },
      { code: '6758', side: 'sell', quantity: 10, price: '3500.0', tradeDate: '2026-02-10' },
    ],
    prices: DUMMY_PRICES,
  })

  await page.goto('/holdings/6758')

  await expect(page.getByRole('heading', { name: '6758 ソニーグループ', exact: true })).toBeVisible()
  await expect(chartOf(page, '6758 ソニーグループの価格のチャート')).toBeVisible()
  await expectTable(page, [
    ['2026-02-10', '売', '10株', '3,500円', '35,000円'],
    ['2026-01-10', '買', '10株', '3,200円', '32,000円'],
  ])
})

test('銘柄ごとの売買履歴の基準23（要件20）: Aは 7203 だけを売買し、Bが 6758「買 3,200円 10株 2026-01-10」を持つとき、Aが /holdings/6758 を開くと、「この銘柄の売買履歴はありません」と「← 保有株式一覧」だけが出て、チャートと表は出ない。誰も売買していない 9984・銘柄マスタにない XXXX でも、同じ表示（他の利用者が売買しているかどうかが分かる違いを出さない）', async ({ page }) => {
  await insertHoldingsData({
    trades: [TOYOTA_BUY],
    otherUserTrades: [{ code: '6758', side: 'buy', quantity: 10, price: '3200.0', tradeDate: BUY_DATE }],
    prices: DUMMY_PRICES,
  })

  const bodyTexts: string[] = []
  for (const code of ['6758', '9984', 'XXXX']) {
    await page.goto(`/holdings/${code}`)

    await expect(page.getByText(NO_HISTORY_MESSAGE, { exact: true }), `${code}: 売買履歴なしの文`).toBeVisible()
    await expect(backLink(page), `${code}: 戻るリンク`).toBeVisible()
    await expect(page.getByRole('table'), `${code}: 表は出ない`).toHaveCount(0)
    await expect(page.getByRole('img', { name: /の価格のチャート$/ }), `${code}: チャートは出ない`).toHaveCount(0)
    const bodyText = (await page.locator('body').innerText()).replace(/\s+/g, '')
    // 「だけ」: 戻るリンクと売買履歴なしの文のほかに、何も出ない。
    expect(bodyText, `${code}: 画面の文字`).toBe(`${BACK_LINK}${NO_HISTORY_MESSAGE}`.replace(/\s+/g, ''))
    bodyTexts.push(bodyText)
  }
  // 3つの画面の文字が同じ（他の利用者の売買の有無を示す違いがない）。
  expect(new Set(bodyTexts).size).toBe(1)
})

test('銘柄ごとの売買履歴の補助（要件12）: 終値が1件もない 9984 の売買履歴を開くと、チャートの中に「この期間の価格はまだありません」が出て、売買の印は2つ描かれ、表は2行', async ({ page }) => {
  await insertHoldingsData({
    trades: [
      { code: '9984', side: 'buy', quantity: 100, price: '6000.0', tradeDate: '2026-01-10' },
      { code: '9984', side: 'sell', quantity: 50, price: '7000.0', tradeDate: '2026-02-10' },
    ],
  })

  await page.goto('/holdings/9984')

  const chart = chartOf(page, '9984 ソフトバンクグループの価格のチャート')
  await expect(chart).toBeVisible()
  await expect(page.getByText(NO_PRICES_MESSAGE, { exact: true })).toBeVisible()
  await expect(chart.locator('[data-chart-part="marker"]')).toHaveCount(2)
  await expect(chart.locator('[data-chart-part="line"]')).toHaveCount(0)
  await expectTable(page, [
    ['2026-02-10', '売', '50株', '7,000円', '350,000円'],
    ['2026-01-10', '買', '100株', '6,000円', '600,000円'],
  ])
})

// =====================================================================================================
// 非機能（基準27）
// =====================================================================================================

test('銘柄ごとの売買履歴の基準27（非機能1）: Aが 7203 の売買履歴を1,000件（2016-11-16 から 2026-11-16 に散らして）持ち、7203 の終値が 2016-11-16 から 2026-11-16 までの平日にすべてあるとき、ローカル開発環境で /holdings/7203 を開いてから、2秒以内にチャートと表（1,000行）が出る（計測値を annotation と console.log に残す）', async ({ page, browser }) => {
  test.setTimeout(90_000)
  // 売買履歴: 最初の売買は 2016-11-16（終値の最初の日と同じ）で、1,000件を 2016-11-16〜2026-11-16 に散らす
  // （最近の日付だけにすると、描く終値が10年分にならない）。10件に1件は売り。
  const FIRST_DAY = '2016-11-16'
  const SPAN_DAYS = 3652
  const COUNT = 1000
  const trades: TradeSeed[] = Array.from({ length: COUNT }, (_, index) => ({
    code: '7203',
    side: index % 10 === 9 ? 'sell' : 'buy',
    quantity: 10,
    price: `${2000 + (index % 500)}.0`,
    tradeDate: addDays(FIRST_DAY, Math.floor((index * SPAN_DAYS) / (COUNT - 1))),
  }))
  // 終値: 2016-11-16 から 2026-11-16 までの平日（土日を除く）すべて。
  const prices: StockPriceSeed[] = []
  for (let day = 0, weekday = 0; day <= SPAN_DAYS; day += 1) {
    const date = addDays(FIRST_DAY, day)
    const dayOfWeek = new Date(`${date}T00:00:00Z`).getUTCDay()
    if (dayOfWeek === 0 || dayOfWeek === 6) continue
    prices.push({ code: '7203', tradingDate: date, closePrice: `${2500 + (weekday % 700)}.0` })
    weekday += 1
  }
  await insertHoldingsData({ trades, prices })

  // ウォームアップ: 一度開いて捨てる（dev サーバーの初回の変換などを計測に含めない）。
  await page.goto('/holdings/7203')
  await expect(tableRows(page)).toHaveCount(COUNT + 1)
  await page.close()

  // 計測: 新しいブラウザコンテキストで開き直す。
  const context = await browser.newContext()
  try {
    const measuredPage = await context.newPage()
    await measuredPage.goto('/holdings/7203')
    // 表の行（td を持つ行）が1,000行そろい、かつチャートの線（data-chart-part="line"）がある時点の performance.now() を、描画のフレームごとに確かめる。
    const shownAt = await measuredPage.waitForFunction(
      (expectedRows) => {
        const rows = Array.from(document.querySelectorAll('tr')).filter((row) => row.querySelector('td') !== null)
        if (rows.length < expectedRows) return false
        return document.querySelector('[data-chart-part="line"]') !== null && performance.now()
      },
      COUNT,
      { polling: 'raf', timeout: 30_000 },
    )
    const measuredMs = Number(await shownAt.jsonValue())
    const summary = `${measuredMs.toFixed(1)}ms（上限 2000ms、余裕 ${(2000 - measuredMs).toFixed(1)}ms）`
    test.info().annotations.push({ type: '基準27 計測値', description: summary })
    console.log(`基準27 計測値: ${summary}`)

    expect(measuredMs).toBeLessThanOrEqual(2000)
    // 判定の後に、表がちょうど1,000行で、チャートが見えていることも確かめる。
    await expect(tableRows(measuredPage)).toHaveCount(COUNT + 1)
    await expect(chartOf(measuredPage, '7203 トヨタ自動車の価格のチャート')).toBeVisible()
  } finally {
    await context.close()
  }
})
