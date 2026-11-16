import { expect, test, type Locator, type Page, type Response } from '@playwright/test'
import {
  insertHoldingsData,
  resetHoldingsData,
  type StockPriceSeed,
  type StockSeed,
  type TradeSeed,
} from '../fixtures/db'
import { cardList, holdingCard, holdingCards, summaryRegion } from '../fixtures/holdings-page'

// 仕様: カード表示の要件 docs/requirements/holdings-cards.md の要件17〜23・要件29・要件30 と、成功基準の基準8（PR② の部分）・基準19〜21・基準23（E2E の補助）・基準31・基準35（E2E）。
// テスト計画: プラン docs/plans/holdings-cards/plan.md 5.1（E2E の共通の決まり）と 5.3 PR② の E2E の行。
//   - 「PUT が送られない」は、要求を最初から記録しておき、確かめた後に PUT が送られる操作（キーボードでの並べ替え）をもう1つ行って、
//     PUT がちょうど1回で本文がその後の並びであることで確かめる（proveNoPutSoFar）。
//   - 並べ替えた後に開き直すテストは、置く操作の前に page.waitForResponse（PUT /api/holding-order・204）を仕掛け、それを待ってから開き直す。
//     応答の要求の本文 {"codes":[...]} が期待した並びであることも確かめる。
//   - 並びは <ul aria-label="保有株式"> の <li> の DOM の順で読む（ドラッグ中だけは位置で読む）。
// 画面の構造: プラン 4.4（PR②）。読み上げの文は dnd-kit のライブリージョン（role="status" aria-live="assertive"）に出る
//   （価格の今すぐ更新の role="status" の領域と区別するため、aria-live も指定して探す）。保存の失敗は role="alert" で「並びの保存に失敗しました」をカードの並びの上に出す。
//   カード（<li>）は tabIndex=0 で、キーボードの始める・置くはスペース、取り消しは Esc。
// 実行の前提: stock-holdings のプラン 4.5「E2E」（zero_e2e、API は利用者A で起動、beforeEach でデータを入れ替える）。
// テストの名前の先頭は「カード表示の基準N」「カード表示の補助」（既存の保有株式一覧・売買履歴の基準と区別するため）。

test.describe.configure({ mode: 'default' })

test.beforeEach(async () => {
  await resetHoldingsData()
})

const HOLDINGS_PATH = '/holdings'
const SAVE_PATH = '/api/holding-order'
const SAVE_FAILED_MESSAGE = '並びの保存に失敗しました'
const PICKED_UP_APPLE = 'AAPL Appleを持ち上げました。矢印キーで動かし、スペースキーで置きます。'

// ---- テストデータ（要件定義「成功基準」の銘柄マスタとダミー価格。取引日は 2026-01-10 の買い） ----
const BUY_DATE = '2026-01-10'
const APPLE: StockSeed = { code: 'AAPL', name: 'Apple', currency: 'USD' }
const MICROSOFT: StockSeed = { code: 'MSFT', name: 'Microsoft', currency: 'USD' }
const TRADE_OF: Record<string, TradeSeed> = {
  '6758': { code: '6758', side: 'buy', quantity: 10, price: '4000.0', tradeDate: BUY_DATE },
  '7203': { code: '7203', side: 'buy', quantity: 100, price: '2500.0', tradeDate: BUY_DATE },
  '9984': { code: '9984', side: 'buy', quantity: 100, price: '4500.0', tradeDate: BUY_DATE },
  AAPL: { code: 'AAPL', side: 'buy', quantity: 10, price: '150.25', tradeDate: BUY_DATE },
  MSFT: { code: 'MSFT', side: 'buy', quantity: 5, price: '400.00', tradeDate: BUY_DATE },
}
const PRICES: StockPriceSeed[] = [
  { code: '7203', tradingDate: '2026-10-06', closePrice: '3000.0' },
  { code: '6758', tradingDate: '2026-10-06', closePrice: '3500.0' },
  { code: 'AAPL', tradingDate: '2026-10-06', closePrice: '180.00' },
]

/** 利用者Aが codes の銘柄を保有している状態にする（ドルの銘柄は銘柄マスタにも足す）。order があれば並びを保存した状態にする。 */
async function seedHoldings(codes: readonly string[], order?: string[]): Promise<void> {
  const stocks = [APPLE, MICROSOFT].filter((stock) => codes.includes(stock.code))
  await insertHoldingsData({
    stocks,
    trades: codes.map((code) => TRADE_OF[code]),
    prices: PRICES.filter((price) => codes.includes(price.code)),
    ...(order === undefined ? {} : { order }),
  })
}

// ---- 読む・操作する補助 ----

/** カードの並びを DOM の順に読む（カードの文字は銘柄コードから始まる。コードはすべて4文字）。 */
async function domOrder(page: Page): Promise<string[]> {
  return holdingCards(page).evaluateAll((items) => items.map((item) => (item.textContent ?? '').trim().slice(0, 4)))
}

async function expectOrder(page: Page, codes: readonly string[]): Promise<void> {
  await expect(holdingCards(page)).toHaveCount(codes.length)
  await expect.poll(() => domOrder(page), { message: 'カードの並び（DOM の順）' }).toEqual(codes)
}

/** 各カードの画面上の位置から読んだ並び（上端が近いものは同じ行として、左から右へ。ドラッグ中の並びを読むために使う）。 */
async function visualOrder(page: Page): Promise<string[]> {
  const boxes = await holdingCards(page).evaluateAll((items) =>
    items.map((item) => {
      const rect = item.getBoundingClientRect()
      return { code: (item.textContent ?? '').trim().slice(0, 4), top: rect.top + rect.height / 2, left: rect.left + rect.width / 2 }
    }),
  )
  const ROW_TOLERANCE_PX = 40
  return [...boxes]
    .sort((a, b) => (Math.abs(a.top - b.top) > ROW_TOLERANCE_PX ? a.top - b.top : a.left - b.left))
    .map((box) => box.code)
}

/** 読み上げ用のライブリージョン（dnd-kit。価格の今すぐ更新の role="status" とは aria-live で区別する）。 */
function liveRegion(page: Page): Locator {
  return page.locator('[role="status"][aria-live="assertive"]')
}

/** 並びの保存（PUT /api/holding-order）の応答を待つ。置く操作の前に呼んで、置いた後に await する。 */
function waitForSave(page: Page): Promise<Response> {
  const response = page.waitForResponse((candidate) => {
    return candidate.request().method() === 'PUT' && new URL(candidate.url()).pathname === SAVE_PATH
  })
  // テストが途中で失敗して await されないまま終わったときの「Test ended」を、本当の失敗の原因に混ぜない（await したときの reject は残る）。
  response.catch(() => {})
  return response
}

/** 保存の応答が 204 で、要求の本文が {"codes": codes} であることを確かめる。 */
async function expectSaved(response: Response, codes: readonly string[]): Promise<void> {
  expect(response.status(), '並びの保存の HTTP ステータス').toBe(204)
  expect(JSON.parse(response.request().postData() ?? 'null') as unknown, '並びの保存の本文').toEqual({ codes })
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

/**
 * カード（<li>）にフォーカスを当て、スペースで持ち上げ、keys を順に押し、置く前の見た目の並び（preview）になるのを待ってから、スペースで置く。
 * preview は、置く前（動かした後）の位置から読んだ並び。動かないことを確かめるテストは、今の並びを渡す。
 * 固定の時間は待たない（動かした結果が反映される前にスペースが届いて、元の位置に置いてしまうのを、見た目の並びで確かめて防ぐ）。
 */
async function liftMoveDrop(page: Page, code: string, keys: readonly string[], preview: readonly string[]): Promise<void> {
  await holdingCard(page, code).focus()
  await page.keyboard.press('Space')
  await expect(liveRegion(page)).toContainText('持ち上げました')
  for (const key of keys) {
    await page.keyboard.press(key)
  }
  // 動く場合は、動かした読み上げが出るのも待つ。直前に取り消したドラッグのカードが元の位置へ戻る動きの途中だと、
  // 位置から読んだ並びがたまたま preview と同じになり、矢印キーが反映される前にスペースが届くことがあるため。
  if (!(await domOrder(page)).every((code, index) => code === preview[index])) {
    await expect(liveRegion(page)).toContainText('番目に動かしました')
  }
  await expect.poll(() => visualOrder(page), { message: '置く前の位置から読んだ並び' }).toEqual(preview)
  await page.keyboard.press('Space')
}

/**
 * 「PUT が送られない」の確かめ（プラン 5.1 の共通の決まり）。ここまでに PUT が1回も送られていないことを確かめた後、
 * PUT が送られる操作（いちばん後ろのカードを Space・←・Space で1つ前へ）を行い、PUT がちょうど1回で、本文がその後の並びであることを確かめる。
 * order は、今のカードの並び。
 */
async function proveNoPutSoFar(page: Page, saves: string[][], order: readonly string[]): Promise<void> {
  expect(saves, 'ここまでに送られた並びの保存の要求').toEqual([])
  const last = order[order.length - 1]
  const expected = [...order.slice(0, -2), last, order[order.length - 2]]
  const saved = waitForSave(page)
  await liftMoveDrop(page, last, ['ArrowLeft'], expected)
  await expectSaved(await saved, expected)
  expect(saves, '操作の後に送られた並びの保存の要求（ちょうど1回）').toEqual([expected])
  await expectOrder(page, expected)
}

/** locator の中心の座標。 */
async function centerOf(locator: Locator): Promise<{ x: number; y: number }> {
  const box = await locator.boundingBox()
  if (box === null) throw new Error('要素の位置を取れない')
  return { x: box.x + box.width / 2, y: box.y + box.height / 2 }
}

/** マウスで from のカードの中心を押し、to の中心まで少しずつ動かす（離さない）。 */
async function mouseDragTo(page: Page, from: Locator, to: { x: number; y: number }): Promise<void> {
  const start = await centerOf(from)
  await page.mouse.move(start.x, start.y)
  await page.mouse.down()
  await page.mouse.move(to.x, to.y, { steps: 20 })
}

// =====================================================================================================
// マウス（基準19）
// =====================================================================================================

test('カード表示の基準19（要件17・要件23・要件24・要件26）: 6758・7203・AAPL を保有し一度も並べ替えていないとき、並びは 6758、7203、AAPL。AAPL のカードをマウスでドラッグし 6758 のカードの位置で離すと並びは AAPL、6758、7203 になり、画面を開き直しても同じ', async ({ page }) => {
  await seedHoldings(['6758', '7203', 'AAPL'])
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', '7203', 'AAPL'])

  const saved = waitForSave(page)
  await mouseDragTo(page, holdingCard(page, 'AAPL'), await centerOf(holdingCard(page, '6758')))
  await page.mouse.up()

  await expectOrder(page, ['AAPL', '6758', '7203'])
  await expectSaved(await saved, ['AAPL', '6758', '7203'])
  await page.reload()
  await expectOrder(page, ['AAPL', '6758', '7203'])
})

test('カード表示の補助（要件17）: ドラッグしている間（離す前）に、各カードの位置から読んだ並びが、離したときの並びになっている', async ({ page }) => {
  await seedHoldings(['6758', '7203', 'AAPL'])
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', '7203', 'AAPL'])

  await mouseDragTo(page, holdingCard(page, 'AAPL'), await centerOf(holdingCard(page, '6758')))

  // 離す前。dnd-kit は DOM の順を変えず、位置（変形）で動かすので、位置から読む。
  await expect.poll(() => visualOrder(page), { message: 'ドラッグ中の位置から読んだ並び' }).toEqual(['AAPL', '6758', '7203'])
  expect(await domOrder(page), 'ドラッグ中の DOM の順は、置くまで変わらない').toEqual(['6758', '7203', 'AAPL'])
  await page.mouse.up()
  await expectOrder(page, ['AAPL', '6758', '7203'])
})

test('カード表示の補助（要件22・基準23 の画面での確かめ）: カードの並びの外（合計の欄の上）でマウスを離すと、並びは同じで、PUT は送られない', async ({ page }) => {
  const saves = recordSaves(page)
  await seedHoldings(['6758', '7203', '9984', 'AAPL'])
  // 2行目の AAPL の中心が画面（既定の高さ 720px）の外になると、マウスを押しても何も起きない（ドラッグが始まらず、確かめたいことを確かめられない）ので、画面を高くする。
  await page.setViewportSize({ width: 1280, height: 1200 })
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', '7203', '9984', 'AAPL'])

  const apple = await centerOf(holdingCard(page, 'AAPL'))
  const summary = await centerOf(summaryRegion(page))
  await page.mouse.move(apple.x, apple.y)
  await page.mouse.down()
  await page.mouse.move(apple.x + 10, apple.y + 10, { steps: 3 })
  // ドラッグが始まっていたこと（始まっていないと、離しても何も起きないので、確かめたいことを確かめられない）。
  // 並びの外へ出ると読み上げは消えるので、動かし始めの直後に確かめる。
  await expect(liveRegion(page)).toContainText('持ち上げました')
  await page.mouse.move(summary.x, summary.y, { steps: 20 })
  await page.mouse.up()

  await expectOrder(page, ['6758', '7203', '9984', 'AAPL'])
  await proveNoPutSoFar(page, saves, ['6758', '7203', '9984', 'AAPL'])
})

// =====================================================================================================
// キーボード（基準21・補助）
// =====================================================================================================

test('カード表示の基準21（要件19・要件20・要件23）: 6758・7203・9984・AAPL を保有し、1,280px で AAPL が2行目の1列目にあるとき、AAPL のカードにフォーカスを当てスペース、↑、スペースの順に押すと、並びは AAPL、6758、7203、9984 になり、「AAPL Appleを1番目に置きました。」が読み上げ用に出る（途中は「持ち上げました」「1番目に動かしました」）。画面を開き直しても同じ', async ({ page }) => {
  await seedHoldings(['6758', '7203', '9984', 'AAPL'])
  await page.setViewportSize({ width: 1280, height: 720 })
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', '7203', '9984', 'AAPL'])
  // 前提: AAPL は2行目の1列目（1行目の1枚目の真下）にある。
  const sonyBox = await holdingCard(page, '6758').boundingBox()
  const appleBox = await holdingCard(page, 'AAPL').boundingBox()
  if (sonyBox === null || appleBox === null) throw new Error('カードの位置を取れない')
  expect(appleBox.y, 'AAPL は2行目').toBeGreaterThan(sonyBox.y + 1)
  expect(Math.abs(appleBox.x - sonyBox.x), 'AAPL は1列目').toBeLessThan(1)

  await holdingCard(page, 'AAPL').focus()
  await page.keyboard.press('Space')
  await expect(liveRegion(page)).toHaveText(PICKED_UP_APPLE)
  await page.keyboard.press('ArrowUp')
  await expect(liveRegion(page)).toHaveText('1番目に動かしました。')
  const saved = waitForSave(page)
  await page.keyboard.press('Space')

  await expect(liveRegion(page)).toHaveText('AAPL Appleを1番目に置きました。')
  await expectOrder(page, ['AAPL', '6758', '7203', '9984'])
  await expectSaved(await saved, ['AAPL', '6758', '7203', '9984'])
  await page.reload()
  await expectOrder(page, ['AAPL', '6758', '7203', '9984'])
})

test('カード表示の補助（要件19）: 並び 6758、7203、9984、AAPL（1,280px）で、6758 を スペース、→、スペース の順に押すと 7203、6758、9984、AAPL になる', async ({ page }) => {
  await seedHoldings(['6758', '7203', '9984', 'AAPL'])
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', '7203', '9984', 'AAPL'])

  const saved = waitForSave(page)
  await liftMoveDrop(page, '6758', ['ArrowRight'], ['7203', '6758', '9984', 'AAPL'])

  await expectOrder(page, ['7203', '6758', '9984', 'AAPL'])
  await expectSaved(await saved, ['7203', '6758', '9984', 'AAPL'])
})

test('カード表示の補助（要件19）: 5枚（6758、7203、9984、AAPL、MSFT。1,280px で1行目3枚・2行目2枚）で、7203 を スペース、↓、スペース の順に押すと、同じ列の1行下（MSFT の位置）へ動いて 6758、9984、AAPL、MSFT、7203 になる', async ({ page }) => {
  await seedHoldings(['6758', '7203', '9984', 'AAPL', 'MSFT'])
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', '7203', '9984', 'AAPL', 'MSFT'])

  const saved = waitForSave(page)
  await liftMoveDrop(page, '7203', ['ArrowDown'], ['6758', '9984', 'AAPL', 'MSFT', '7203'])

  await expectOrder(page, ['6758', '9984', 'AAPL', 'MSFT', '7203'])
  await expectSaved(await saved, ['6758', '9984', 'AAPL', 'MSFT', '7203'])
})

test('カード表示の補助（要件19。← → は並びの順、↑ ↓ は同じ列）: 5枚で、AAPL を スペース、→（表示は 6758、7203、9984 / MSFT、AAPL）、↑、スペース の順に押すと、並びは 6758、AAPL、7203、9984、MSFT になる（↑ で別の列へずれていない）', async ({ page }) => {
  await seedHoldings(['6758', '7203', '9984', 'AAPL', 'MSFT'])
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', '7203', '9984', 'AAPL', 'MSFT'])

  const saved = waitForSave(page)
  await holdingCard(page, 'AAPL').focus()
  await page.keyboard.press('Space')
  await expect(liveRegion(page)).toContainText('持ち上げました')
  await page.keyboard.press('ArrowRight')
  // → の後の表示: 1行目 6758、7203、9984 / 2行目 MSFT、AAPL。
  await expect(liveRegion(page)).toHaveText('5番目に動かしました。')
  await expect.poll(() => visualOrder(page), { message: '→ の後の位置から読んだ並び' }).toEqual(['6758', '7203', '9984', 'MSFT', 'AAPL'])
  await page.keyboard.press('ArrowUp')
  // ↑ の後の表示: 1行目 6758、AAPL、7203 / 2行目 9984、MSFT。
  await expect(liveRegion(page)).toHaveText('2番目に動かしました。')
  await expect.poll(() => visualOrder(page), { message: '↑ の後の位置から読んだ並び' }).toEqual(['6758', 'AAPL', '7203', '9984', 'MSFT'])
  await page.keyboard.press('Space')

  await expectOrder(page, ['6758', 'AAPL', '7203', '9984', 'MSFT'])
  await expectSaved(await saved, ['6758', 'AAPL', '7203', '9984', 'MSFT'])
})

test('カード表示の補助（要件19）: 幅 390px（1列）で、6758 を スペース、→、スペース の順に押すと、1つ後ろへ動いて 7203、6758、9984、AAPL になる', async ({ page }) => {
  await seedHoldings(['6758', '7203', '9984', 'AAPL'])
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', '7203', '9984', 'AAPL'])

  const saved = waitForSave(page)
  await liftMoveDrop(page, '6758', ['ArrowRight'], ['7203', '6758', '9984', 'AAPL'])

  await expectOrder(page, ['7203', '6758', '9984', 'AAPL'])
  await expectSaved(await saved, ['7203', '6758', '9984', 'AAPL'])
})

test('カード表示の補助（要件19）: 3列で、2行目の先頭の AAPL を スペース、←、スペース の順に押すと、前の行の末尾へ動いて 6758、7203、AAPL、9984 になる（行をまたぐ）', async ({ page }) => {
  await seedHoldings(['6758', '7203', '9984', 'AAPL'])
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', '7203', '9984', 'AAPL'])

  const saved = waitForSave(page)
  await liftMoveDrop(page, 'AAPL', ['ArrowLeft'], ['6758', '7203', 'AAPL', '9984'])

  await expectOrder(page, ['6758', '7203', 'AAPL', '9984'])
  await expectSaved(await saved, ['6758', '7203', 'AAPL', '9984'])
})

test('カード表示の補助（要件19）: 先頭のカードで スペース、←、スペース、末尾のカードで スペース、→、スペース としても、並びは変わらず PUT も送られない', async ({ page }) => {
  const saves = recordSaves(page)
  await seedHoldings(['6758', '7203', '9984', 'AAPL'])
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', '7203', '9984', 'AAPL'])

  await liftMoveDrop(page, '6758', ['ArrowLeft'], ['6758', '7203', '9984', 'AAPL'])
  await expectOrder(page, ['6758', '7203', '9984', 'AAPL'])
  await liftMoveDrop(page, 'AAPL', ['ArrowRight'], ['6758', '7203', '9984', 'AAPL'])
  await page.waitForTimeout(300)

  await expectOrder(page, ['6758', '7203', '9984', 'AAPL'])
  await proveNoPutSoFar(page, saves, ['6758', '7203', '9984', 'AAPL'])
})

test('カード表示の補助（要件19・要件22）: スペースで持ち上げ、←、Esc で取り消すと、並びは元のままで「並べ替えを取り消しました。」が出て、PUT は送られない', async ({ page }) => {
  const saves = recordSaves(page)
  await seedHoldings(['6758', '7203', 'AAPL'])
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', '7203', 'AAPL'])

  await holdingCard(page, 'AAPL').focus()
  await page.keyboard.press('Space')
  await expect(liveRegion(page)).toHaveText(PICKED_UP_APPLE)
  await page.keyboard.press('ArrowLeft')
  // ← が反映されてから Esc を押す（反映の前に Esc が届くと、動かさずに取り消しただけになり、確かめたいことを確かめられない）。
  await expect(liveRegion(page)).toHaveText('2番目に動かしました。')
  await expect.poll(() => visualOrder(page), { message: '← の後の位置から読んだ並び' }).toEqual(['6758', 'AAPL', '7203'])
  await page.keyboard.press('Escape')

  await expect(liveRegion(page)).toHaveText('並べ替えを取り消しました。')
  await expectOrder(page, ['6758', '7203', 'AAPL'])
  await proveNoPutSoFar(page, saves, ['6758', '7203', 'AAPL'])
})

test('カード表示の補助（要件19・要件22。判断9）: スペースで持ち上げ、←、Tab を押すと取り消され、並びは元のままで「並べ替えを取り消しました。」が出て、フォーカスは持ち上げたカードのままで、PUT は送られない', async ({ page }) => {
  const saves = recordSaves(page)
  await seedHoldings(['6758', '7203', 'AAPL'])
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', '7203', 'AAPL'])

  await holdingCard(page, 'AAPL').focus()
  await page.keyboard.press('Space')
  await expect(liveRegion(page)).toHaveText(PICKED_UP_APPLE)
  await page.keyboard.press('ArrowLeft')
  await expect(liveRegion(page)).toHaveText('2番目に動かしました。')
  await expect.poll(() => visualOrder(page), { message: '← の後の位置から読んだ並び' }).toEqual(['6758', 'AAPL', '7203'])
  await page.keyboard.press('Tab')

  await expect(liveRegion(page)).toHaveText('並べ替えを取り消しました。')
  await expect.poll(() => visualOrder(page), { message: '取り消した後の位置から読んだ並び' }).toEqual(['6758', '7203', 'AAPL'])
  await expectOrder(page, ['6758', '7203', 'AAPL'])
  await expect(holdingCard(page, 'AAPL')).toBeFocused()
  await proveNoPutSoFar(page, saves, ['6758', '7203', 'AAPL'])
})

test('カード表示の補助（要件19・基準21。置く操作の直後の矢印キー）: スペースで持ち上げた直後に、待たずに ← を押し、スペースで置くと、並びが 6758、AAPL、7203 になる（持ち上げた直後の矢印キーが捨てられない）', async ({ page }) => {
  await seedHoldings(['6758', '7203', 'AAPL'])
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', '7203', 'AAPL'])

  await holdingCard(page, 'AAPL').focus()
  // Space と ← の間に、ライブリージョンの確かめなどの待ちを入れない。
  await page.keyboard.press('Space')
  await page.keyboard.press('ArrowLeft')
  await expect.poll(() => visualOrder(page), { message: '← の後の位置から読んだ並び' }).toEqual(['6758', 'AAPL', '7203'])
  const saved = waitForSave(page)
  await page.keyboard.press('Space')

  await expectOrder(page, ['6758', 'AAPL', '7203'])
  await expectSaved(await saved, ['6758', 'AAPL', '7203'])
})

// =====================================================================================================
// 「⋯」（基準8 の PR② の部分・要件21・要件6）
// =====================================================================================================

test('カード表示の基準8（要件21。PR② の部分）: 6758・7203・9984・AAPL を保有しているとき、7203 のカードの「⋯」（「トヨタ自動車の操作」）を押すと「削除」のメニューが開き、並びは変わらず、並びの保存の要求は送られない', async ({ page }) => {
  const saves = recordSaves(page)
  await seedHoldings(['6758', '7203', '9984', 'AAPL'])
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', '7203', '9984', 'AAPL'])

  await holdingCard(page, '7203').getByRole('button', { name: 'トヨタ自動車の操作', exact: true }).click()

  await expect(page.getByRole('menuitem', { name: '削除', exact: true })).toBeVisible()
  await expectOrder(page, ['6758', '7203', '9984', 'AAPL'])
  await page.keyboard.press('Escape')
  await expect(page.getByRole('menuitem')).toHaveCount(0)
  await proveNoPutSoFar(page, saves, ['6758', '7203', '9984', 'AAPL'])
})

test('カード表示の補助（要件21）: 「⋯」にフォーカスして スペース を押すとメニューが開き、ドラッグは始まらない（ライブリージョンに「持ち上げました」が出ない）。メニューの項目で Esc を押すとメニューが閉じる', async ({ page }) => {
  const saves = recordSaves(page)
  await seedHoldings(['6758', '7203', 'AAPL'])
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', '7203', 'AAPL'])

  await holdingCard(page, 'AAPL').getByRole('button', { name: 'Appleの操作', exact: true }).focus()
  await page.keyboard.press('Space')

  await expect(page.getByRole('menuitem', { name: '削除', exact: true })).toBeVisible()
  await page.waitForTimeout(200)
  await expect(liveRegion(page)).not.toContainText('持ち上げました')
  // メニューの項目（フォーカスはメニューの項目に移っている）で Esc を押すと、メニューが閉じる。
  await expect(page.getByRole('menuitem', { name: '削除', exact: true })).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('menuitem')).toHaveCount(0)
  await expect(liveRegion(page)).not.toContainText('並べ替えを取り消しました')
  await expectOrder(page, ['6758', '7203', 'AAPL'])
  await proveNoPutSoFar(page, saves, ['6758', '7203', 'AAPL'])
})

test('カード表示の補助（要件21）: 「⋯」の上でマウスを押し、別のカードの上まで動かして離しても、ライブリージョンに「持ち上げました」が出ず、並びが変わらず、PUT も送られない', async ({ page }) => {
  const saves = recordSaves(page)
  await seedHoldings(['6758', '7203', '9984', 'AAPL'])
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', '7203', '9984', 'AAPL'])

  const menuButton = holdingCard(page, '6758').getByRole('button', { name: 'ソニーグループの操作', exact: true })
  const from = await centerOf(menuButton)
  const to = await centerOf(holdingCard(page, '9984'))
  await page.mouse.move(from.x, from.y)
  await page.mouse.down()
  await page.mouse.move(to.x, to.y, { steps: 20 })
  await page.waitForTimeout(200)
  await expect(liveRegion(page)).not.toContainText('持ち上げました')
  await page.mouse.up()

  await expectOrder(page, ['6758', '7203', '9984', 'AAPL'])
  await expect(liveRegion(page)).not.toContainText('持ち上げました')
  await proveNoPutSoFar(page, saves, ['6758', '7203', '9984', 'AAPL'])
})

test('カード表示の補助（要件6）: 「⋯」の箱は、カードの右端に寄っていて、銘柄コードと同じ行にある', async ({ page }) => {
  await seedHoldings(['7203'])
  await page.goto(HOLDINGS_PATH)

  const card = holdingCard(page, '7203')
  const cardBox = await card.boundingBox()
  const menuBox = await card.getByRole('button', { name: 'トヨタ自動車の操作', exact: true }).boundingBox()
  const codeBox = await card.getByText('7203', { exact: true }).boundingBox()
  if (cardBox === null || menuBox === null || codeBox === null) throw new Error('カード・「⋯」・銘柄コードの位置を取れない')

  const RIGHT_EDGE_TOLERANCE_PX = 40 // カードの内側の余白（1.25rem）に少し余裕を足した値
  expect(cardBox.x + cardBox.width - (menuBox.x + menuBox.width), '「⋯」の右端とカードの右端の間').toBeLessThanOrEqual(RIGHT_EDGE_TOLERANCE_PX)
  expect(menuBox.x, '「⋯」は銘柄コードより右').toBeGreaterThan(codeBox.x + codeBox.width)
  expect(Math.abs(menuBox.y + menuBox.height / 2 - (codeBox.y + codeBox.height / 2)), '「⋯」と銘柄コードの縦の中心の差').toBeLessThan(codeBox.height)
})

// =====================================================================================================
// 保存の失敗（基準31）
// =====================================================================================================

test('カード表示の基準31（要件30）: 6758・7203・AAPL を保有し、並びの保存の API がエラー（HTTP 500）を返すとき、AAPL を先頭に置くと、並びは 6758、7203、AAPL に戻り、カードの並びの上に「並びの保存に失敗しました」が表示され、次に並べ替えるまで残る', async ({ page }) => {
  await seedHoldings(['6758', '7203', 'AAPL'])
  let failing = true
  await page.route(`**${SAVE_PATH}`, async (route) => {
    if (failing) {
      await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: 'internal_error' }) })
    } else {
      await route.continue()
    }
  })
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', '7203', 'AAPL'])

  const failedSave = waitForSave(page)
  await mouseDragTo(page, holdingCard(page, 'AAPL'), await centerOf(holdingCard(page, '6758')))
  await page.mouse.up()

  expect((await failedSave).status(), '保存の応答').toBe(500)
  const alert = page.getByRole('alert').filter({ hasText: SAVE_FAILED_MESSAGE })
  await expect(alert).toBeVisible()
  await expect(alert).toHaveText(SAVE_FAILED_MESSAGE)
  await expectOrder(page, ['6758', '7203', 'AAPL'])
  // カードの並びの上にある。
  const alertBox = await alert.boundingBox()
  const listBox = await cardList(page).boundingBox()
  if (alertBox === null || listBox === null) throw new Error('表示の位置を取れない')
  expect(alertBox.y + alertBox.height, '失敗の表示の下端は、カードの並びの上端より上').toBeLessThanOrEqual(listBox.y + 1)
  // 次に並べ替えるまで残る（待っても消えない）。
  await page.waitForTimeout(1000)
  await expect(alert).toBeVisible()
  await expectOrder(page, ['6758', '7203', 'AAPL'])

  // 次の並べ替え（今度は保存できる）で、表示が消える。
  failing = false
  const saved = waitForSave(page)
  await liftMoveDrop(page, 'AAPL', ['ArrowLeft'], ['6758', 'AAPL', '7203'])
  await expect(alert).toHaveCount(0)
  await expectSaved(await saved, ['6758', 'AAPL', '7203'])
  await expectOrder(page, ['6758', 'AAPL', '7203'])
})

test('カード表示の基準31（要件30。キーボード版）: 6758・AAPL を保有し、並びの保存の API がエラー（HTTP 500）を返すとき、キーボード（スペース、←、スペース）で AAPL を先頭に置くと、並びは 6758、AAPL に戻り、「並びの保存に失敗しました」が表示され、フォーカスは AAPL のカードにある', async ({ page }) => {
  await seedHoldings(['6758', 'AAPL'])
  await page.route(`**${SAVE_PATH}`, (route) =>
    route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: 'internal_error' }) }),
  )
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', 'AAPL'])

  const failedSave = waitForSave(page)
  await liftMoveDrop(page, 'AAPL', ['ArrowLeft'], ['AAPL', '6758'])

  expect((await failedSave).status(), '保存の応答').toBe(500)
  await expect(page.getByRole('alert').filter({ hasText: SAVE_FAILED_MESSAGE })).toBeVisible()
  await expectOrder(page, ['6758', 'AAPL'])
  await expect(holdingCard(page, 'AAPL')).toBeFocused()
})

test('カード表示の補助（要件30）: 保存の失敗の表示は、画面を開き直すと消え、サーバーの並び（保存できていないので元の並び）で表示される', async ({ page }) => {
  await seedHoldings(['6758', '7203', 'AAPL'])
  await page.route(`**${SAVE_PATH}`, (route) =>
    route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: 'internal_error' }) }),
  )
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', '7203', 'AAPL'])
  const failedSave = waitForSave(page)
  await liftMoveDrop(page, 'AAPL', ['ArrowLeft'], ['6758', 'AAPL', '7203'])
  await failedSave
  await expect(page.getByRole('alert').filter({ hasText: SAVE_FAILED_MESSAGE })).toBeVisible()

  await page.unroute(`**${SAVE_PATH}`)
  await page.reload()

  await expect(page.getByRole('alert').filter({ hasText: SAVE_FAILED_MESSAGE })).toHaveCount(0)
  await expectOrder(page, ['6758', '7203', 'AAPL'])
})

// =====================================================================================================
// 保存した並びの表示（基準35）
// =====================================================================================================

test('カード表示の基準35（非機能2）: 100銘柄（円50・ドル50。すべて価格が保存済み）を売買履歴各20件で保有し、並びを銘柄コードの降順に保存しているとき、/holdings を開いてから2秒以内に、保存した並びでカードが表示される（計測値を annotation と console.log に残す）', async ({ page, browser }) => {
  // Given: 売買履歴の基準31 と同じデータ（円50・ドル50、各20件、すべて価格が保存済み）に、並び（銘柄コードの降順）を足す。
  const yenCodes = Array.from({ length: 50 }, (_, i) => String(1001 + i))
  const usdCodes = Array.from({ length: 50 }, (_, i) => `US${String(i + 1).padStart(2, '0')}`)
  const stocks: StockSeed[] = [
    ...yenCodes.map((code) => ({ code, name: `E2E銘柄${code}`, currency: 'JPY' as const })),
    ...usdCodes.map((code) => ({ code, name: `E2E銘柄${code}`, currency: 'USD' as const })),
  ]
  const allCodes = [...yenCodes, ...usdCodes]
  // 銘柄コードの文字列としての降順（US50、…、US01、1050、…、1001）。
  const descending = [...allCodes].sort((a, b) => (a < b ? 1 : a > b ? -1 : 0))
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
    order: descending,
  })

  // ウォームアップ: 一度 /holdings を開いて捨てる（dev サーバーの初回の変換などを計測に含めない）。
  await page.goto(HOLDINGS_PATH)
  await expect(holdingCards(page)).toHaveCount(allCodes.length)
  await page.close()

  // 計測: 新しいブラウザコンテキストで開き直す。
  const context = await browser.newContext()
  try {
    const measuredPage = await context.newPage()
    await measuredPage.goto(HOLDINGS_PATH)
    // カードが100枚あり、かつ先頭と末尾が降順の先頭と末尾になった時点の performance.now() を、描画のフレームごとに確かめる。
    const shownAt = await measuredPage.waitForFunction(
      ({ count, first, last }) => {
        const items = document.querySelectorAll('ul[aria-label="保有株式"] > li')
        if (items.length < count) return false
        const textOf = (el: Element | undefined) => (el?.textContent ?? '').trim()
        return textOf(items[0]).startsWith(first) && textOf(items[items.length - 1]).startsWith(last) && performance.now()
      },
      { count: allCodes.length, first: descending[0], last: descending[descending.length - 1] },
      { polling: 'raf' },
    )
    const elapsedMs = await shownAt.jsonValue()

    const measuredMs = Number(elapsedMs)
    const summary = `${measuredMs.toFixed(1)}ms（上限 2000ms、余裕 ${(2000 - measuredMs).toFixed(1)}ms）`
    test.info().annotations.push({ type: '基準35 計測値', description: summary })
    console.log(`基準35 計測値: ${summary}`)

    expect(elapsedMs).toBeLessThanOrEqual(2000)
    // 判定の後に、100枚の並び全体が降順であることも確かめる。
    await expectOrder(measuredPage, descending)
  } finally {
    await context.close()
  }
})

// =====================================================================================================
// タッチ（基準20・要件18・要件21）。Chromium のタッチの模擬（CDP の Input.dispatchTouchEvent）で確かめる。
// ステップ4 の最初に試した結果: Input.dispatchTouchEvent の touchStart・touchMove・touchEnd で、画面のスクロールが起きた
// （window.scrollY が増えた）ので、Input.synthesizeScrollGesture には切り替えない。
// =====================================================================================================

test.describe('タッチの画面（幅 390px）', () => {
  test.use({ hasTouch: true, viewport: { width: 390, height: 844 } })

  /** CDP でタッチを送る補助。 */
  async function touchSession(page: Page) {
    const cdp = await page.context().newCDPSession(page)
    return {
      start: (x: number, y: number) => cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x, y }] }),
      move: (x: number, y: number) => cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x, y }] }),
      end: () => cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] }),
    }
  }

  /** (from) から (to) まで、steps 回に分けて touchMove する（各回の間に intervalMs 待つ）。 */
  async function touchMoveSteps(
    page: Page,
    touch: Awaited<ReturnType<typeof touchSession>>,
    from: { x: number; y: number },
    to: { x: number; y: number },
    steps: number,
    intervalMs: number,
  ): Promise<void> {
    for (let i = 1; i <= steps; i += 1) {
      await touch.move(from.x + ((to.x - from.x) * i) / steps, from.y + ((to.y - from.y) * i) / steps)
      await page.waitForTimeout(intervalMs)
    }
  }

  test('カード表示の基準20（要件18）: 6758・7203・AAPL を保有し、幅 390px のタッチの画面で開いているとき、押し続けずに 6758 のカードから上へ指を動かすと、並びは変わらずに画面がスクロールし、並びの保存の要求は送られない。AAPL のカードを0.25秒以上（400ms）押し続けてから 6758 のカードの位置まで指を動かして離すと、並びは AAPL、6758、7203 になる', async ({ page }) => {
    const saves = recordSaves(page)
    await seedHoldings(['6758', '7203', 'AAPL'])
    await page.goto(HOLDINGS_PATH)
    await expectOrder(page, ['6758', '7203', 'AAPL'])
    const touch = await touchSession(page)

    // 押し続けない側: 画面が縦に動く高さ（844px）で、スクロールしてから 6758 のカードの上に指を置き、すぐ（50ms 以内）上へ動かす。
    await page.evaluate(() => window.scrollTo(0, 400))
    const scrollBefore = await page.evaluate(() => window.scrollY)
    expect(scrollBefore, '前提: 画面は縦にスクロールできる').toBeGreaterThan(0)
    const sony = await centerOf(holdingCard(page, '6758'))
    // 前提: 指を置く座標にいるのは 6758 のカード（座標が外れていると、カードの上でない場所のスクロールを確かめてしまう）。
    const labelAtTouch = await page.evaluate(
      ({ x, y }) => document.elementFromPoint(x, y)?.closest('li')?.getAttribute('aria-label') ?? null,
      sony,
    )
    expect(labelAtTouch, '指を置く座標にあるカード').toBe('6758 ソニーグループ')
    await touch.start(sony.x, sony.y)
    await touchMoveSteps(page, touch, sony, { x: sony.x, y: sony.y - 250 }, 10, 5)
    await touch.end()
    await page.waitForTimeout(300)

    expect(await page.evaluate(() => window.scrollY), '画面がスクロールした（scrollY が増えた）').toBeGreaterThan(scrollBefore)
    expect(await domOrder(page), '押し続けずに動かしたので、並びは変わらない').toEqual(['6758', '7203', 'AAPL'])
    expect(saves, '並びの保存の要求は送られない').toEqual([])
    await expect(liveRegion(page)).not.toContainText('持ち上げました')

    // 押し続ける側: 2枚のカードが同じ画面に入る高さにして、AAPL を 400ms 押し続けてから、数回に分けて 6758 のカードまで動かして離す。
    await page.setViewportSize({ width: 390, height: 1800 })
    await page.evaluate(() => window.scrollTo(0, 0))
    const apple = await centerOf(holdingCard(page, 'AAPL'))
    const sonyTarget = await centerOf(holdingCard(page, '6758'))
    const saved = waitForSave(page)
    await touch.start(apple.x, apple.y)
    await page.waitForTimeout(400)
    await touchMoveSteps(page, touch, apple, sonyTarget, 12, 20)
    await touch.end()

    await expectOrder(page, ['AAPL', '6758', '7203'])
    await expectSaved(await saved, ['AAPL', '6758', '7203'])
    // 保存の要求は、長押しで並べ替えた1回だけ（スクロールのときには送られていない）。
    expect(saves).toEqual([['AAPL', '6758', '7203']])
  })

  test('カード表示の補助（要件21）: 「⋯」を 400ms 押し続けてから別のカードの上まで動かして離しても、ライブリージョンに「持ち上げました」が出ず、並びが変わらず、PUT も送られない', async ({ page }) => {
    const saves = recordSaves(page)
    await seedHoldings(['6758', '7203', 'AAPL'])
    await page.setViewportSize({ width: 390, height: 1800 })
    await page.goto(HOLDINGS_PATH)
    await expectOrder(page, ['6758', '7203', 'AAPL'])
    const touch = await touchSession(page)

    const menu = await centerOf(holdingCard(page, '6758').getByRole('button', { name: 'ソニーグループの操作', exact: true }))
    const target = await centerOf(holdingCard(page, '7203'))
    await touch.start(menu.x, menu.y)
    await page.waitForTimeout(400)
    await touchMoveSteps(page, touch, menu, target, 12, 20)
    await expect(liveRegion(page)).not.toContainText('持ち上げました')
    await touch.end()
    await page.waitForTimeout(200)

    await expectOrder(page, ['6758', '7203', 'AAPL'])
    await expect(liveRegion(page)).not.toContainText('持ち上げました')
    await proveNoPutSoFar(page, saves, ['6758', '7203', 'AAPL'])
  })
})
