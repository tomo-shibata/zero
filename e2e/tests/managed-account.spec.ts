import { expect, test, type Locator, type Page, type Response } from '@playwright/test'
import {
  insertHoldingsData,
  resetHoldingsData,
  type ManagedValuationSeed,
  type StockPriceSeed,
  type StockSeed,
  type TradeSeed,
  type UsdJpyRateSeed,
} from '../fixtures/db'
import {
  GAIN_COLOR,
  UNPRICED_NOTE,
  cardGain,
  cardList,
  cardValue,
  computedColors,
  currencyPart,
  expectNoHorizontalScroll,
  expectTopToBottom,
  gainAndOtherFontSizes,
  holdingCard,
  holdingCards,
  partGain,
  refreshButton,
  summaryRegion,
} from '../fixtures/holdings-page'
import { centerOf, mouseDragTo, touchMoveSteps, touchSession } from '../fixtures/pointer'

// 仕様: おまかせ運用の要件 docs/requirements/managed-account.md の要件6〜要件11・要件15〜要件20・要件22〜要件24・非機能1 と、
//   成功基準の基準1・基準2・基準4・基準5・基準9〜基準11・基準13・基準14・基準16・基準18・基準27（E2E）。
// テスト計画: プラン docs/plans/managed-account/plan.md 5.1 の PR③ の E2E の行と、5.2 の PR③ の E2E（managed-account.spec.ts）。
//   - 記録は SQL で入れる（insertHoldingsData の managedValuations。4.8 PR③）。
//   - 画面の要素（プラン 4.8 PR③）: 口座の並びは role="list" の「おまかせ運用」（ul[aria-label="おまかせ運用"]）の listitem。
//     カードの中の名前（getByText）、「⋯」（「<名前>の操作」）、文字「おまかせ運用」、見出し「含み損益」、含み損益と率（data-tone の要素が1つ）、
//     評価額・元本・評価額の基準日は dl の dt と次の dd（cardValue で読める）。削除の確認は alertdialog（名前は文言そのもの）。
//   - 「PUT が送られない」は、要求を最初から記録しておき、確かめた後に PUT が送られる操作（キーボードでの並べ替え）をもう1つ行って、
//     PUT がちょうど1回で本文がその後の並びであることで確かめる（holdings-reorder.spec.ts の proveNoPutSoFar と同じ）。
//   - 性能の測り方は売買履歴の基準31（holdings.spec.ts）・holdings-reorder.spec.ts の基準35 と同じ（ウォームアップの後、新しいコンテキストで performance.now()）。
// 実行の前提: stock-holdings のプラン 4.5「E2E」（zero_e2e、API は利用者A で起動、beforeEach でデータを入れ替える）。
// テストの名前の先頭は「おまかせ運用の基準N」「おまかせ運用の補助」（ほかの機能の基準と区別するため）。
// 「今日」は 2026-11-16（要件定義の成功基準の冒頭）。記録は SQL で直接入れるので、日付の検査は受けない。

test.describe.configure({ mode: 'default' })

test.beforeEach(async () => {
  await resetHoldingsData()
})

const HOLDINGS_PATH = '/holdings'
const SAVE_PATH = '/api/holding-order'
const ACCOUNT_LIST_LABEL = 'おまかせ運用'
const ACCOUNT_DELETE_PATH = '/api/managed-accounts'
const DELETE_MENU_ITEM = '削除'
const DELETE_CONFIRM_BUTTON = '削除する'
const DELETE_CANCEL_BUTTON = 'キャンセル'
const DELETE_ERROR_MESSAGE = '削除できませんでした'
const COMBINED_YEN = '円にまとめる'
const SWITCH_LABEL = '合計の表示'

// ---- テストデータ（要件定義「成功基準」の冒頭。銘柄マスタとダミー価格は _test-data.md。取引日は 2026-01-10 の買い） ----
const BUY_DATE = '2026-01-10'
const APPLE: StockSeed = { code: 'AAPL', name: 'Apple', currency: 'USD' }
/** 7203「買 2,500円 100株」（現在の価格 3,000円で、評価額 300,000円・含み損益 +50,000円）。 */
const TOYOTA_100: TradeSeed = { code: '7203', side: 'buy', quantity: 100, price: '2500.0', tradeDate: BUY_DATE }
/** 6758「買 3,200円 10株」（基準4）。 */
const SONY_10_3200: TradeSeed = { code: '6758', side: 'buy', quantity: 10, price: '3200.0', tradeDate: BUY_DATE }
/** 6758「買 4,000円 10株」（基準9。現在の価格 3,500円で、評価額 35,000円・含み損益 -5,000円）。 */
const SONY_10_4000: TradeSeed = { code: '6758', side: 'buy', quantity: 10, price: '4000.0', tradeDate: BUY_DATE }
/** 9984「買 4,500円 100株」（価格なし。基準10）。 */
const SOFTBANK_100: TradeSeed = { code: '9984', side: 'buy', quantity: 100, price: '4500.0', tradeDate: BUY_DATE }
/** AAPL「買 150.25ドル 10株」（現在の価格 180.00ドルで、評価額 1,800.00ドル・含み損益 +297.50ドル）。 */
const APPLE_10: TradeSeed = { code: 'AAPL', side: 'buy', quantity: 10, price: '150.25', tradeDate: BUY_DATE }
const TOYOTA_PRICE: StockPriceSeed = { code: '7203', tradingDate: '2026-10-06', closePrice: '3000.0' }
const SONY_PRICE: StockPriceSeed = { code: '6758', tradingDate: '2026-10-06', closePrice: '3500.0' }
const APPLE_PRICE: StockPriceSeed = { code: 'AAPL', tradingDate: '2026-10-06', closePrice: '180.00' }
/** レート: 2026-11-14 150.00円。 */
const RATE: UsdJpyRateSeed = { date: '2026-11-14', rate: '150.00' }

/** 「WealthNavi 2026-11-16 評価額 4,000,000円 損益 +1,000,000円」。 */
const WEALTHNAVI_LATEST: ManagedValuationSeed = { name: 'WealthNavi', date: '2026-11-16', valuation: 4000000, gain: 1000000 }
/** 「WealthNavi 2026-10-31 評価額 3,950,000円 損益 +950,000円」（最新でない記録。画面に出ない）。 */
const WEALTHNAVI_OLDER: ManagedValuationSeed = { name: 'WealthNavi', date: '2026-10-31', valuation: 3950000, gain: 950000 }

const WEALTHNAVI_DELETE_MESSAGE = 'WealthNaviを削除しますか？この口座の記録はすべて削除されます。'

// ---- 画面の要素 ----

/** 口座のカードの並び（ul[aria-label="おまかせ運用"]）。 */
function accountList(page: Page): Locator {
  return page.getByRole('list', { name: ACCOUNT_LIST_LABEL, exact: true })
}

/** 口座のカード（その並びの listitem）すべて。DOM の順。 */
function accountCards(page: Page): Locator {
  return accountList(page).getByRole('listitem')
}

/** 名前が name の口座のカード。 */
function accountCard(page: Page, name: string): Locator {
  return accountCards(page).filter({ has: page.getByText(name, { exact: true }) })
}

/** 口座の「⋯」のボタン（「<名前>の操作」）。 */
function accountMenuButton(page: Page, name: string): Locator {
  return page.getByRole('button', { name: `${name}の操作`, exact: true })
}

/** 口座のカードの並びを、「⋯」のボタンの名前から読んだ名前の順（DOM の順）にする。 */
async function accountNames(page: Page): Promise<string[]> {
  return accountCards(page).evaluateAll((items) =>
    items.map((item) => (item.querySelector('button')?.getAttribute('aria-label') ?? '').replace(/の操作$/, '')),
  )
}

/** 口座の削除の確認のダイアログ（role="alertdialog"。名前は文言そのもの）。 */
function deleteDialog(page: Page, message: string): Locator {
  return page.getByRole('alertdialog', { name: message, exact: true })
}

/** 口座 name の「⋯」→「削除」を選んで、削除の確認のダイアログが開くのを待つ。 */
async function openDeleteConfirm(page: Page, name: string, message: string): Promise<Locator> {
  await accountMenuButton(page, name).click()
  await page.getByRole('menuitem', { name: DELETE_MENU_ITEM, exact: true }).click()
  const dialog = deleteDialog(page, message)
  await expect(dialog).toBeVisible()
  return dialog
}

/** 合計の欄の「円にまとめる」のボタン。 */
function combinedYenButton(page: Page): Locator {
  return summaryRegion(page).getByRole('group', { name: SWITCH_LABEL, exact: true }).getByRole('button', { name: COMBINED_YEN, exact: true })
}

// ---- 並べ替えの補助（holdings-reorder.spec.ts と同じ決まり） ----

/** 株式のカードの並びを DOM の順に読む（カードの文字は銘柄コードから始まる。コードはすべて4文字）。 */
async function domOrder(page: Page): Promise<string[]> {
  return holdingCards(page).evaluateAll((items) => items.map((item) => (item.textContent ?? '').trim().slice(0, 4)))
}

async function expectOrder(page: Page, codes: readonly string[]): Promise<void> {
  await expect(holdingCards(page)).toHaveCount(codes.length)
  await expect.poll(() => domOrder(page), { message: '株式のカードの並び（DOM の順）' }).toEqual(codes)
}

/** 株式の各カードの画面上の位置から読んだ並び（上端が近いものは同じ行として、左から右へ）。 */
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

/** 株式の各カードの左上の位置（銘柄コードごと）。 */
async function cardPositions(page: Page): Promise<Record<string, { x: number; y: number }>> {
  const entries = await holdingCards(page).evaluateAll((items) =>
    items.map((item) => {
      const rect = item.getBoundingClientRect()
      return [(item.textContent ?? '').trim().slice(0, 4), { x: rect.x, y: rect.y }] as const
    }),
  )
  return Object.fromEntries(entries)
}

/** 持ち上げているカード（dragged）以外の株式のカードが、すべて before の位置にあることを待って確かめる。 */
async function expectOthersAtOriginalPositions(
  page: Page,
  before: Record<string, { x: number; y: number }>,
  dragged: string,
): Promise<void> {
  const others = Object.keys(before).filter((code) => code !== dragged)
  await expect
    .poll(
      async () => {
        const now = await cardPositions(page)
        return others.every((code) => Math.abs(now[code].x - before[code].x) < 1 && Math.abs(now[code].y - before[code].y) < 1)
      },
      { message: `${dragged} 以外のカードが、元の位置にある（先行表示が元の並びになっている）` },
    )
    .toBe(true)
}

/** 読み上げ用のライブリージョン（株式の並び HoldingCardList が持つ。価格の今すぐ更新の role="status" とは aria-live で区別する）。 */
function liveRegion(page: Page): Locator {
  return page.locator('[role="status"][aria-live="assertive"]')
}

/** 並びの保存（PUT /api/holding-order）の応答を待つ。置く操作の前に呼んで、置いた後に await する。 */
function waitForSave(page: Page): Promise<Response> {
  const response = page.waitForResponse((candidate) => {
    return candidate.request().method() === 'PUT' && new URL(candidate.url()).pathname === SAVE_PATH
  })
  response.catch(() => {})
  return response
}

/** 保存の応答が 204 で、要求の本文が {"codes": codes} であることを確かめる。 */
async function expectSaved(response: Response, codes: readonly string[]): Promise<void> {
  expect(response.status(), '並びの保存の HTTP ステータス').toBe(204)
  expect(JSON.parse(response.request().postData() ?? 'null') as unknown, '並びの保存の本文').toEqual({ codes })
}

/** 並びの保存の要求（PUT /api/holding-order）の本文を、最初から記録する（goto の前に呼ぶ）。 */
function recordSaves(page: Page): unknown[] {
  const saves: unknown[] = []
  page.on('request', (request) => {
    if (request.method() === 'PUT' && new URL(request.url()).pathname === SAVE_PATH) {
      saves.push(JSON.parse(request.postData() ?? 'null') as unknown)
    }
  })
  return saves
}

/**
 * 株式のカード（<li>）にフォーカスを当て、スペースで持ち上げ、keys を順に押し、置く前の見た目の並び（preview）になるのを待ってから、スペースで置く。
 * 固定の時間は待たない。
 */
async function liftMoveDrop(page: Page, code: string, keys: readonly string[], preview: readonly string[]): Promise<void> {
  await holdingCard(page, code).focus()
  await page.keyboard.press('Space')
  await expect(liveRegion(page)).toContainText('持ち上げました')
  for (const key of keys) {
    await page.keyboard.press(key)
  }
  if (!(await domOrder(page)).every((code, index) => code === preview[index])) {
    await expect(liveRegion(page)).toContainText('番目に動かしました')
  }
  await expect.poll(() => visualOrder(page), { message: '置く前の位置から読んだ並び' }).toEqual(preview)
  await page.keyboard.press('Space')
}

/**
 * 「PUT が送られない」の確かめ。ここまでに PUT が1回も送られていないことを確かめた後、PUT が送られる操作（いちばん後ろの株式のカードを
 * Space・←・Space で1つ前へ）を行い、PUT がちょうど1回で、本文が {"codes":[…]}（その後の並び。口座の名前を含まない。要件10）であることを確かめる。
 * order は、今の株式のカードの並び。
 */
async function proveNoPutSoFar(page: Page, saves: unknown[], order: readonly string[]): Promise<void> {
  expect(saves, 'ここまでに送られた並びの保存の要求').toEqual([])
  const last = order[order.length - 1]
  const expected = [...order.slice(0, -2), last, order[order.length - 2]]
  const saved = waitForSave(page)
  await liftMoveDrop(page, last, ['ArrowLeft'], expected)
  await expectSaved(await saved, expected)
  expect(saves, '操作の後に送られた並びの保存の要求（ちょうど1回。口座の名前を含まない）').toEqual([{ codes: expected }])
  await expectOrder(page, expected)
}

// =====================================================================================================
// カードの表示（基準1・基準2）
// =====================================================================================================

test('おまかせ運用の基準1（要件2・要件4・要件6〜要件8・要件12・要件13）: WealthNavi の記録が 2026-10-31（評価額 3,950,000円 損益 +950,000円）と 2026-11-16（評価額 4,000,000円 損益 +1,000,000円）のとき、WealthNavi のカードに「WealthNavi / おまかせ運用 / 含み損益 +1,000,000円（+33.3%）/ 評価額 4,000,000円 / 元本 3,000,000円 / 評価額の基準日 2026-11-16」が上からこの順に表示され、含み損益は赤で、カードの中でいちばん大きな文字。2026-10-31 の値は表示されない', async ({ page }) => {
  await insertHoldingsData({ managedValuations: [WEALTHNAVI_OLDER, WEALTHNAVI_LATEST] })

  await page.goto(HOLDINGS_PATH)

  await expect(accountCards(page)).toHaveCount(1)
  const card = accountCard(page, 'WealthNavi')
  await expect(card).toHaveCount(1)
  await expect(cardGain(card)).toHaveCount(1)
  await expect(cardGain(card)).toHaveText('+1,000,000円（+33.3%）')
  await expect(cardValue(card, '評価額')).toHaveText('4,000,000円')
  await expect(cardValue(card, '元本')).toHaveText('3,000,000円')
  await expect(cardValue(card, '評価額の基準日')).toHaveText('2026-11-16')
  await expectTopToBottom(
    [
      card.getByText('WealthNavi', { exact: true }),
      card.getByText('おまかせ運用', { exact: true }),
      card.getByText('含み損益', { exact: true }),
      cardGain(card),
      card.getByText('評価額', { exact: true }),
      card.getByText('4,000,000円', { exact: true }),
      card.getByText('元本', { exact: true }),
      card.getByText('3,000,000円', { exact: true }),
      card.getByText('評価額の基準日', { exact: true }),
      card.getByText('2026-11-16', { exact: true }),
    ],
    ['名前', 'おまかせ運用', '含み損益の見出し', '含み損益と率', '評価額の見出し', '評価額', '元本の見出し', '元本', '評価額の基準日の見出し', '評価額の基準日'],
  )
  // 含み損益は赤（テストの中の定数。要素とその子孫の色がすべて赤）。
  for (const color of await computedColors(cardGain(card))) expect(color, '含み損益の文字の色').toBe(GAIN_COLOR)
  // 含み損益と率の文字が、カードの中のほかのすべての文字より大きい（要件8）。
  const sizes = await gainAndOtherFontSizes(card)
  expect(sizes.gainMin, `含み損益と率の文字（最小 ${sizes.gainMin}px）が、ほかの文字（最大 ${sizes.othersMax}px）より大きい`).toBeGreaterThan(sizes.othersMax)
  // 最新でない記録の値は出ない。
  await expect(card).not.toContainText('3,950,000円')
  await expect(card).not.toContainText('2026-10-31')
  await expect(card).not.toContainText('950,000円')
  // 合計の欄の円も最新の記録の値（古い記録の値は出ない）。
  const yen = currencyPart(page, '円')
  await expect(partGain(yen)).toHaveText('+1,000,000円（+33.3%）')
  await expect(yen.getByText('4,000,000円', { exact: true })).toHaveCount(1)
  await expect(summaryRegion(page)).not.toContainText('3,950,000円')
  await expect(summaryRegion(page)).not.toContainText('+950,000円')
})

test('おまかせ運用の基準2（要件9・要件20）: 7203「買 2,500円 100株」（価格保存済み）と WealthNavi のとき、上から、合計の欄、WealthNavi のカード、「価格を今すぐ更新」、7203 のカードの順に並ぶ', async ({ page }) => {
  await insertHoldingsData({ trades: [TOYOTA_100], prices: [TOYOTA_PRICE], managedValuations: [WEALTHNAVI_LATEST] })

  await page.goto(HOLDINGS_PATH)

  await expectTopToBottom(
    [summaryRegion(page), accountCard(page, 'WealthNavi'), refreshButton(page), holdingCard(page, '7203')],
    ['合計の欄', 'WealthNavi のカード', '価格を今すぐ更新', '7203 のカード'],
  )
  // 口座のカードは、株式の並びの中に入らない（別の並び）。
  await expect(accountCards(page)).toHaveCount(1)
  await expect(holdingCards(page)).toHaveCount(1)
})

// =====================================================================================================
// 並べ替えない・押しても移らない（基準4・基準5。要件10・要件11）
// =====================================================================================================

/** 基準4・基準5 のデータ: 6758「買 3,200円 10株」・7203「買 2,500円 100株」（価格保存済み。並びは 6758、7203）と、WealthNavi の記録1件。 */
async function insertReorderData(): Promise<void> {
  await insertHoldingsData({
    trades: [SONY_10_3200, TOYOTA_100],
    prices: [TOYOTA_PRICE, SONY_PRICE],
    order: ['6758', '7203'],
    managedValuations: [WEALTHNAVI_LATEST],
  })
}

test('おまかせ運用の基準4（要件10。マウス）: 並び 6758、7203 のとき、WealthNavi のカードをマウスで 7203 の位置へ動かして離しても、持ち上げた表示は出ず、カードは動かず、並びは変わらず、並びの保存の要求は送られない。7203 のカードを WealthNavi のカードの上へ動かして離しても、並びは 6758、7203 のまま保存の要求は送られない（本物の並べ替えは、口座の名前を含まない {"codes":["7203","6758"]} を1回だけ送る）', async ({ page }) => {
  const saves = recordSaves(page)
  await insertReorderData()
  // 3枚（口座1・株式2）が同じ画面に入る高さにする（中心が画面の外だと、マウスを押しても何も起きないため）。
  await page.setViewportSize({ width: 1280, height: 1200 })
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', '7203'])
  await expect(accountCards(page)).toHaveCount(1)

  // (1) WealthNavi のカードを、マウスで 7203 のカードの位置へ動かす。離す前に、持ち上げた表示が出ず、WealthNavi のカードの位置が動いていない。
  const wealthnavi = accountCard(page, 'WealthNavi')
  const boxBefore = await wealthnavi.boundingBox()
  await mouseDragTo(page, wealthnavi, await centerOf(holdingCard(page, '7203')))
  await expect(liveRegion(page)).not.toContainText('持ち上げました')
  expect(await wealthnavi.boundingBox(), 'WealthNavi のカードの位置（離す前）').toEqual(boxBefore)
  await page.mouse.up()
  expect(await wealthnavi.boundingBox(), 'WealthNavi のカードの位置（離した後）').toEqual(boxBefore)
  await expectOrder(page, ['6758', '7203'])
  expect(saves, '(1) の後に送られた並びの保存の要求').toEqual([])

  // (2) 7203 のカードをマウスで WealthNavi のカードの上へ動かす。動かし始めた直後に「持ち上げました」が出る（ドラッグが始まっている）ことを確かめてから離す。
  const positionsBefore = await cardPositions(page)
  const toyota = await centerOf(holdingCard(page, '7203'))
  const target = await centerOf(wealthnavi)
  await page.mouse.move(toyota.x, toyota.y)
  await page.mouse.down()
  await page.mouse.move(toyota.x + 10, toyota.y + 10, { steps: 3 })
  // 並びの外へ出ると読み上げは消えるので、動かし始めの直後に確かめる。
  await expect(liveRegion(page)).toContainText('持ち上げました')
  await page.mouse.move(target.x, target.y, { steps: 20 })
  // 離す前に、ほかの株式のカードが元の位置にある（口座のカードの上は、並びの外と同じく元の並びに戻る）。
  await expectOthersAtOriginalPositions(page, positionsBefore, '7203')
  await page.mouse.up()

  await expectOrder(page, ['6758', '7203'])
  await expect.poll(() => visualOrder(page), { message: '離した後の位置から読んだ並び' }).toEqual(['6758', '7203'])
  expect(await wealthnavi.boundingBox(), 'WealthNavi のカードの位置（(2) の後）').toEqual(boxBefore)
  // 最後に本物の並べ替えを1回行い（キーボードで 6758 と 7203 を入れ替える）、保存がちょうど1件で口座の名前を含まないことを確かめる。
  await proveNoPutSoFar(page, saves, ['6758', '7203'])
})

test.describe('タッチの画面（幅 390px）', () => {
  test.use({ hasTouch: true, viewport: { width: 390, height: 844 } })

  test('おまかせ運用の基準4（要件10。タッチ）: 並び 6758、7203 のとき、幅 390px のタッチの画面で WealthNavi のカードを0.25秒以上押し続けてから 6758 のカードの位置まで動かして離しても、持ち上げた表示は出ず、並びは変わらず、並びの保存の要求は送られない（本物の並べ替えは {"codes":["7203","6758"]} を1回だけ送る）', async ({ page }) => {
    const saves = recordSaves(page)
    await insertReorderData()
    // 3枚が同じ画面に入る高さにする。
    await page.setViewportSize({ width: 390, height: 1800 })
    await page.goto(HOLDINGS_PATH)
    await expectOrder(page, ['6758', '7203'])
    await expect(accountCards(page)).toHaveCount(1)
    const touch = await touchSession(page)

    const wealthnavi = accountCard(page, 'WealthNavi')
    const boxBefore = await wealthnavi.boundingBox()
    const start = await centerOf(wealthnavi)
    // 前提: 指を置く座標にあるのは WealthNavi のカード（口座の並びの中のカード）。外れていると、別の場所の長押しを確かめてしまう。
    const atTouch = await page.evaluate(
      ({ x, y }) => {
        const item = document.elementFromPoint(x, y)?.closest('li')
        return { list: item?.parentElement?.getAttribute('aria-label') ?? null, text: (item?.textContent ?? '').trim() }
      },
      start,
    )
    expect(atTouch.list, '指を置く座標にあるカードが入っている並び').toBe(ACCOUNT_LIST_LABEL)
    expect(atTouch.text.startsWith('WealthNavi'), '指を置く座標にあるカード').toBe(true)
    const sonyTarget = await centerOf(holdingCard(page, '6758'))

    await touch.start(start.x, start.y)
    await page.waitForTimeout(400)
    await touchMoveSteps(page, touch, start, sonyTarget, 12, 20)
    // 離す前に、持ち上げた表示が出ていない。
    await expect(liveRegion(page)).not.toContainText('持ち上げました')
    await touch.end()
    await page.waitForTimeout(200)

    expect(await wealthnavi.boundingBox(), 'WealthNavi のカードの位置').toEqual(boxBefore)
    await expectOrder(page, ['6758', '7203'])
    expect(saves, '長押しで動かした後に送られた並びの保存の要求').toEqual([])

    // 最後に本物の並べ替えを1回行う: 7203 を0.25秒以上押し続けてから 6758 の位置まで動かして離す（タッチが届いていることの確かめも兼ねる）。
    const toyota = await centerOf(holdingCard(page, '7203'))
    const sonyNow = await centerOf(holdingCard(page, '6758'))
    const saved = waitForSave(page)
    await touch.start(toyota.x, toyota.y)
    await page.waitForTimeout(400)
    await touchMoveSteps(page, touch, toyota, sonyNow, 12, 20)
    await touch.end()

    await expectOrder(page, ['7203', '6758'])
    await expectSaved(await saved, ['7203', '6758'])
    expect(saves, '保存の要求は、本物の並べ替えの1回だけ（口座の名前を含まない）').toEqual([{ codes: ['7203', '6758'] }])
  })

  test('おまかせ運用の基準5（要件11。タップ）: 幅 390px のタッチの画面で WealthNavi のカードを tap() して 300ms 待っても、/holdings のまま（株式のカードのタップは移る）', async ({ page }) => {
    await insertReorderData()
    await page.goto(HOLDINGS_PATH)
    await expectOrder(page, ['6758', '7203'])
    await expect(accountCards(page)).toHaveCount(1)

    await accountCard(page, 'WealthNavi').tap()
    // 株式のカードはタップの後の互換のマウスのイベントでも移りうるので、少し待ってから確かめる。
    await page.waitForTimeout(300)
    await expect(page).toHaveURL(/\/holdings$/)

    // 対照: 同じ操作で株式のカードは移る（タップで移る操作が効いていることの確かめ）。
    await holdingCard(page, '6758').tap()
    await expect(page).toHaveURL(/\/holdings\/6758$/)
  })
})

test('おまかせ運用の基準5（要件10・要件11。マウスとキーボード）: WealthNavi のカードの中央をマウスで押して 500ms 待っても /holdings のまま。Tab で順にフォーカスを移すと、口座のカードそのもの（li）には一度も当たらず、「WealthNaviの操作」のボタンには当たる', async ({ page }) => {
  await insertReorderData()
  await page.setViewportSize({ width: 1280, height: 1200 })
  await page.goto(HOLDINGS_PATH)
  await expectOrder(page, ['6758', '7203'])
  await expect(accountCards(page)).toHaveCount(1)

  // Tab を、いちばん最後の株式のカードの「⋯」に当たるまで押し続け、押すたびの document.activeElement を集める。
  // 開いた直後（何も押していない状態）から始める。先にマウスで押すと、Chromium は押した場所から Tab の移動を始めるため、前の項目を見落とす。
  const visited: { tag: string; label: string | null; list: string | null }[] = []
  for (let presses = 0; presses < 30; presses += 1) {
    await page.keyboard.press('Tab')
    const active = await page.evaluate(() => {
      const element = document.activeElement
      return {
        tag: element?.tagName ?? '',
        label: element?.getAttribute('aria-label') ?? null,
        list: element?.tagName === 'LI' ? (element.parentElement?.getAttribute('aria-label') ?? null) : null,
      }
    })
    visited.push(active)
    if (active.label === 'トヨタ自動車の操作') break
  }
  const labels = visited.map((item) => item.label)
  expect(labels, '最後の株式のカードの「⋯」まで Tab で進めた（最後まで集めた）').toContain('トヨタ自動車の操作')
  expect(labels, '「WealthNaviの操作」のボタンに Tab で止まる').toContain('WealthNaviの操作')
  expect(
    visited.filter((item) => item.tag === 'LI' && item.list === ACCOUNT_LIST_LABEL),
    '口座のカード（li）に Tab で止まらない',
  ).toEqual([])
  // 対照: 株式のカード（li）には止まる（Tab で li に止まる集め方が効いていることの確かめ）。
  expect(visited.some((item) => item.tag === 'LI' && item.list === '保有株式'), '株式のカード（li）には Tab で止まる').toBe(true)

  // マウスで押す（クリック）。株式のカードはクリックで移るので、500ms 待っても移らないことで確かめる。
  const center = await centerOf(accountCard(page, 'WealthNavi'))
  await page.mouse.click(center.x, center.y)
  await page.waitForTimeout(500)
  await expect(page).toHaveURL(/\/holdings$/)

  // 対照: 株式のカードを押すと移る（クリックで移る操作が効いていることの確かめ）。
  await holdingCard(page, '7203').click()
  await expect(page).toHaveURL(/\/holdings\/7203$/)
})

// =====================================================================================================
// 合計の欄（基準9〜基準11）
// =====================================================================================================

/** 基準9 のデータ: 7203・6758・AAPL（どれも価格保存済み）と WealthNavi（評価額 4,000,000円 損益 +1,000,000円）。 */
async function insertCriterion9Data(withRate = false): Promise<void> {
  await insertHoldingsData({
    stocks: [APPLE],
    trades: [TOYOTA_100, SONY_10_4000, APPLE_10],
    prices: [TOYOTA_PRICE, SONY_PRICE, APPLE_PRICE],
    managedValuations: [WEALTHNAVI_LATEST],
    ...(withRate ? { usdJpyRates: [RATE] } : {}),
  })
}

test('おまかせ運用の基準9（要件15・要件16）: 7203・6758・AAPL（どれも価格保存済み）と WealthNavi（評価額 4,000,000円 損益 +1,000,000円）のとき、合計の欄の円は「+1,045,000円（+31.8%）/ 4,335,000円」、ドルは「+297.50ドル（+19.8%）/ 1,800.00ドル」。円とドルの2つだけで、株式だけ・口座だけの合計は出ない', async ({ page }) => {
  await insertCriterion9Data()

  await page.goto(HOLDINGS_PATH)

  const summary = summaryRegion(page)
  await expect(summary).toBeVisible()
  const yen = currencyPart(page, '円')
  const usd = currencyPart(page, 'ドル')
  await expect(partGain(yen)).toHaveText('+1,045,000円（+31.8%）')
  await expect(yen.getByText('4,335,000円', { exact: true })).toHaveCount(1)
  await expect(partGain(usd)).toHaveText('+297.50ドル（+19.8%）')
  await expect(usd.getByText('1,800.00ドル', { exact: true })).toHaveCount(1)
  // 合計の欄の中の region は「円」「ドル」の2つだけで、含み損益と率（data-tone）の要素も2つだけ（株式だけ・口座だけの合計を出さない。要件15）。
  await expect(summary.getByRole('region')).toHaveCount(2)
  await expect(summary.locator('[data-tone]')).toHaveCount(2)
  await expect(page.getByText(UNPRICED_NOTE)).toHaveCount(0)
})

test('おまかせ運用の基準10（要件17）: 9984「買 4,500円 100株」（価格なし）だけの売買履歴と WealthNavi のとき、合計の欄の円は「+1,000,000円（+33.3%）/ 4,000,000円」と「価格を取得できない銘柄を含みません」（1つ）になる', async ({ page }) => {
  await insertHoldingsData({ trades: [SOFTBANK_100], managedValuations: [WEALTHNAVI_LATEST] })

  await page.goto(HOLDINGS_PATH)

  const yen = currencyPart(page, '円')
  await expect(partGain(yen)).toHaveText('+1,000,000円（+33.3%）')
  await expect(yen.getByText('4,000,000円', { exact: true })).toHaveCount(1)
  await expect(yen.getByText(UNPRICED_NOTE)).toHaveCount(1)
  await expect(page.getByText(UNPRICED_NOTE)).toHaveCount(1)
  await expect(summaryRegion(page)).not.toContainText('取得できません')
})

test('おまかせ運用の基準11（要件18）: 基準9 と同じで、レートが 2026-11-14 150.00円のとき、「円にまとめる」を押すと、合計の欄が「+1,089,625円（+31.0%）/ 4,605,000円 / 1ドル = 150.00円（2026-11-14 のレート）で換算」になる', async ({ page }) => {
  await insertCriterion9Data(true)

  await page.goto(HOLDINGS_PATH)
  await expect(combinedYenButton(page)).toBeVisible()
  await combinedYenButton(page).click()

  const summary = summaryRegion(page)
  await expect(combinedYenButton(page)).toHaveAttribute('aria-pressed', 'true')
  await expect(summary.locator('[data-tone]')).toHaveCount(1)
  await expect(summary.locator('[data-tone]')).toHaveText('+1,089,625円（+31.0%）')
  await expect(summary.getByText('4,605,000円', { exact: true })).toHaveCount(1)
  await expect(summary.getByText('1ドル = 150.00円（2026-11-14 のレート）で換算', { exact: true })).toHaveCount(1)
  await expect(currencyPart(page, '円')).toHaveCount(0)
  await expect(currencyPart(page, 'ドル')).toHaveCount(0)
})

// =====================================================================================================
// 株式がないとき（基準13・基準14）
// =====================================================================================================

test('おまかせ運用の基準13（要件17・要件19・要件20・要件22）: 売買履歴が0件で WealthNavi だけのとき、「保有株式はありません」は表示されず、合計の欄は円の部分だけ（+1,000,000円（+33.3%）/ 4,000,000円）で、WealthNavi のカードと「WealthNaviの操作」のボタン（1つ）が表示され、「価格を今すぐ更新」と株式のカードは表示されない', async ({ page }) => {
  await insertHoldingsData({ managedValuations: [WEALTHNAVI_LATEST] })

  await page.goto(HOLDINGS_PATH)

  // 先に口座のカードが出る（取得が終わった）のを待ってから、ないことを確かめる。
  await expect(accountCard(page, 'WealthNavi')).toHaveCount(1)
  await expect(page.getByText('保有株式はありません')).toHaveCount(0)
  const yen = currencyPart(page, '円')
  await expect(partGain(yen)).toHaveText('+1,000,000円（+33.3%）')
  await expect(yen.getByText('4,000,000円', { exact: true })).toHaveCount(1)
  await expect(currencyPart(page, 'ドル')).toHaveCount(0)
  await expect(summaryRegion(page).getByRole('region')).toHaveCount(1)
  await expect(accountMenuButton(page, 'WealthNavi')).toHaveCount(1)
  await expect(refreshButton(page)).toHaveCount(0)
  await expect(cardList(page)).toHaveCount(0)
  await expect(holdingCards(page)).toHaveCount(0)
})

test('おまかせ運用の基準14（要件19）: 売買履歴も口座も0件のとき、「保有株式はありません」が表示され、合計の欄と list「おまかせ運用」は表示されない', async ({ page }) => {
  await page.goto(HOLDINGS_PATH)

  await expect(page.getByText('保有株式はありません')).toBeVisible()
  await expect(summaryRegion(page)).toHaveCount(0)
  await expect(accountList(page)).toHaveCount(0)
  await expect(page.getByRole('heading', { name: ACCOUNT_LIST_LABEL, exact: true })).toHaveCount(0)
  await expect(refreshButton(page)).toHaveCount(0)
})

// =====================================================================================================
// 削除（基準16・基準18。要件22〜要件24）
// =====================================================================================================

test('おまかせ運用の基準16（要件22〜要件24）: 7203（価格保存済み）と WealthNavi の記録2件のとき、「WealthNaviの操作」を押すとメニューの項目は「削除」の1つだけ。「削除」を選ぶと「WealthNaviを削除しますか？この口座の記録はすべて削除されます。」の確認が開き、「キャンセル」で閉じて WealthNavi のカードは残る。もう一度「削除」→「削除する」で閉じて、list「おまかせ運用」がなくなり、合計の欄の円は「+50,000円（+20.0%）/ 300,000円」になる', async ({ page }) => {
  await insertHoldingsData({
    trades: [TOYOTA_100],
    prices: [TOYOTA_PRICE],
    managedValuations: [WEALTHNAVI_OLDER, WEALTHNAVI_LATEST],
  })
  await page.goto(HOLDINGS_PATH)
  await expect(accountCard(page, 'WealthNavi')).toHaveCount(1)
  await expect(partGain(currencyPart(page, '円'))).toHaveText('+1,050,000円（+32.3%）')

  // 「⋯」を押すと、「削除」だけのメニューが開く。
  await accountMenuButton(page, 'WealthNavi').click()
  await expect(page.getByRole('menu')).toBeVisible()
  await expect(page.getByRole('menuitem')).toHaveCount(1)
  await expect(page.getByRole('menuitem', { name: DELETE_MENU_ITEM, exact: true })).toBeVisible()

  // 「削除」を選ぶと、確認のダイアログが開く（文言と、ボタンは「キャンセル」「削除する」）。
  await page.getByRole('menuitem', { name: DELETE_MENU_ITEM, exact: true }).click()
  const dialog = deleteDialog(page, WEALTHNAVI_DELETE_MESSAGE)
  await expect(dialog).toBeVisible()
  await expect(dialog).toContainText(WEALTHNAVI_DELETE_MESSAGE)
  await expect(dialog.getByRole('button')).toHaveText([DELETE_CANCEL_BUTTON, DELETE_CONFIRM_BUTTON])

  // 「キャンセル」を押すと、閉じて WealthNavi のカードは残る（何も変わらない）。
  await dialog.getByRole('button', { name: DELETE_CANCEL_BUTTON, exact: true }).click()
  await expect(dialog).toHaveCount(0)
  await expect(accountCard(page, 'WealthNavi')).toHaveCount(1)
  await expect(partGain(currencyPart(page, '円'))).toHaveText('+1,050,000円（+32.3%）')

  // もう一度「削除」を選び、「削除する」を押すと、閉じて WealthNavi のカードはなくなり、合計の欄は株式だけの値になる。
  const dialogAgain = await openDeleteConfirm(page, 'WealthNavi', WEALTHNAVI_DELETE_MESSAGE)
  await dialogAgain.getByRole('button', { name: DELETE_CONFIRM_BUTTON, exact: true }).click()

  await expect(dialogAgain).toHaveCount(0)
  await expect(accountList(page)).toHaveCount(0)
  await expect(accountCards(page)).toHaveCount(0)
  const yen = currencyPart(page, '円')
  await expect(partGain(yen)).toHaveText('+50,000円（+20.0%）')
  await expect(yen.getByText('300,000円', { exact: true })).toHaveCount(1)
  // 株式のカードはそのまま。
  await expect(holdingCard(page, '7203')).toHaveCount(1)
})

test('おまかせ運用の基準18（要件24）: WealthNavi があり、削除の API がエラー（HTTP 500）を返すとき、「削除する」を押しても、ダイアログは開いたまま「削除できませんでした」と表示され、WealthNavi のカードは残る', async ({ page }) => {
  await insertHoldingsData({ trades: [TOYOTA_100], prices: [TOYOTA_PRICE], managedValuations: [WEALTHNAVI_LATEST] })
  let deleteRequests = 0
  await page.route(
    (url) => url.pathname === ACCOUNT_DELETE_PATH,
    async (route) => {
      if (route.request().method() !== 'DELETE') {
        await route.continue()
        return
      }
      deleteRequests += 1
      await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: 'internal_error' }) })
    },
  )
  await page.goto(HOLDINGS_PATH)
  await expect(accountCard(page, 'WealthNavi')).toHaveCount(1)
  const dialog = await openDeleteConfirm(page, 'WealthNavi', WEALTHNAVI_DELETE_MESSAGE)

  await dialog.getByRole('button', { name: DELETE_CONFIRM_BUTTON, exact: true }).click()

  await expect(dialog.getByRole('alert')).toHaveText(DELETE_ERROR_MESSAGE)
  await expect(dialog).toBeVisible()
  expect(deleteRequests, '削除の要求が送られた').toBe(1)
  await expect(accountCard(page, 'WealthNavi')).toHaveCount(1)
  await expect(partGain(currencyPart(page, '円'))).toHaveText('+1,050,000円（+32.3%）')
})

// =====================================================================================================
// 補助: 「⋯」の位置・1行の枚数・掴めるように見えないこと・長い名前（プラン 5.2 の PR③ の E2E）
// =====================================================================================================

/** 口座4つ（API の順は Alpha、Beta、WealthNavi、ロボアド B）。記録は1件ずつ。 */
const FOUR_ACCOUNTS: ManagedValuationSeed[] = [
  { name: 'WealthNavi', date: '2026-11-16', valuation: 4000000, gain: 1000000 },
  { name: 'Alpha', date: '2026-11-16', valuation: 500000, gain: 0 },
  { name: 'ロボアド B', date: '2026-11-16', valuation: 1000000, gain: -250000 },
  { name: 'Beta', date: '2026-11-16', valuation: 2000000, gain: 100000 },
]

test('おまかせ運用の補助（要件7）: 口座4つのとき、「WealthNaviの操作」の箱は、カードの右端から 40px 以内で、名前より右にあり、名前と同じ行にある', async ({ page }) => {
  await insertHoldingsData({ managedValuations: FOUR_ACCOUNTS })
  await page.goto(HOLDINGS_PATH)
  await expect(accountCards(page)).toHaveCount(4)

  const card = accountCard(page, 'WealthNavi')
  const cardBox = await card.boundingBox()
  const menuBox = await accountMenuButton(page, 'WealthNavi').boundingBox()
  const nameBox = await card.getByText('WealthNavi', { exact: true }).boundingBox()
  if (cardBox === null || menuBox === null || nameBox === null) throw new Error('カード・「⋯」・名前の位置を取れない')

  const RIGHT_EDGE_TOLERANCE_PX = 40
  expect(cardBox.x + cardBox.width - (menuBox.x + menuBox.width), '「⋯」の右端とカードの右端の間').toBeLessThanOrEqual(RIGHT_EDGE_TOLERANCE_PX)
  expect(cardBox.x + cardBox.width - (menuBox.x + menuBox.width), '「⋯」はカードの中（右端を超えない）').toBeGreaterThanOrEqual(0)
  expect(menuBox.x, '「⋯」は名前より右').toBeGreaterThan(nameBox.x + nameBox.width)
  expect(Math.abs(menuBox.y + menuBox.height / 2 - (nameBox.y + nameBox.height / 2)), '「⋯」と名前の縦の中心の差').toBeLessThan(nameBox.height)
})

test('おまかせ運用の補助（要件9）: 口座4つのとき、カードは名前の順（Alpha、Beta、WealthNavi、ロボアド B）で、幅 1,280px では1行目に3枚・2行目に1枚、幅 390px では1行に1枚ずつ並び、横にスクロールしない', async ({ page }) => {
  await insertHoldingsData({ managedValuations: FOUR_ACCOUNTS })

  // 幅 1,280px: 上端が同じカードが1行目に3つ、2行目に1つ。2行目の左端は1行目の1枚目の左端と同じ。
  await page.setViewportSize({ width: 1280, height: 720 })
  await page.goto(HOLDINGS_PATH)
  await expect(accountCards(page)).toHaveCount(4)
  expect(await accountNames(page), '口座のカードの順（名前の文字コードの順）').toEqual(['Alpha', 'Beta', 'WealthNavi', 'ロボアド B'])
  const wide = await accountCards(page).evaluateAll((items) =>
    items.map((item) => {
      const rect = item.getBoundingClientRect()
      return { top: rect.top, left: rect.left }
    }),
  )
  expect(Math.abs(wide[1].top - wide[0].top), '2枚目は1枚目と同じ行').toBeLessThan(1)
  expect(Math.abs(wide[2].top - wide[0].top), '3枚目は1枚目と同じ行').toBeLessThan(1)
  expect(wide[3].top, '4枚目は2行目').toBeGreaterThan(wide[0].top + 1)
  expect(Math.abs(wide[3].left - wide[0].left), '4枚目の左端は1枚目の左端と同じ').toBeLessThan(1)
  expect(wide[1].left, '2枚目は1枚目の右').toBeGreaterThan(wide[0].left + 1)
  expect(wide[2].left, '3枚目は2枚目の右').toBeGreaterThan(wide[1].left + 1)
  await expectNoHorizontalScroll(page)

  // 幅 390px: 1行に1枚ずつ（上端が増えていき、左端がそろう）。画面は横にスクロールしない。
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(accountCards(page)).toHaveCount(4)
  const narrow = await accountCards(page).evaluateAll((items) =>
    items.map((item) => {
      const rect = item.getBoundingClientRect()
      return { top: rect.top, left: rect.left, right: rect.right }
    }),
  )
  for (let i = 1; i < narrow.length; i += 1) {
    expect(narrow[i].top, `${i + 1}枚目は ${i} 枚目より下の行`).toBeGreaterThan(narrow[i - 1].top + 1)
    expect(Math.abs(narrow[i].left - narrow[0].left), `${i + 1}枚目の左端は1枚目と同じ`).toBeLessThan(1)
  }
  for (const rect of narrow) expect(rect.right, 'カードは画面の幅に収まる').toBeLessThanOrEqual(390)
  await expectNoHorizontalScroll(page)
})

test('おまかせ運用の補助（要件10。見た目）: 口座のカードの li の cursor は grab でなく、株式のカードの li は grab（掴めるように見えない）', async ({ page }) => {
  await insertHoldingsData({
    trades: [TOYOTA_100],
    prices: [TOYOTA_PRICE],
    managedValuations: [WEALTHNAVI_LATEST],
  })
  await page.goto(HOLDINGS_PATH)

  await expect(accountCard(page, 'WealthNavi')).toHaveCount(1)
  await expect(holdingCard(page, '7203')).toHaveCount(1)
  // 対照: 株式のカードは grab（並べ替えのための見た目。これが読めていれば、口座が grab でないことに意味がある）。
  await expect(holdingCard(page, '7203')).toHaveCSS('cursor', 'grab')
  await expect(accountCard(page, 'WealthNavi')).not.toHaveCSS('cursor', 'grab')
})

test('おまかせ運用の補助（要件7・要件23）: 空白のない英字40文字の名前の口座は、幅 390px で横にスクロールせず、名前の文字がカードの箱の中に収まり、「⋯」の箱もカードの中にある。削除の確認を開くと、文がダイアログの箱の中に収まる', async ({ page }) => {
  const LONG_NAME = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmn'
  expect(LONG_NAME).toHaveLength(40)
  await insertHoldingsData({ managedValuations: [{ name: LONG_NAME, date: '2026-11-16', valuation: 4000000, gain: 1000000 }] })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto(HOLDINGS_PATH)

  const card = accountCard(page, LONG_NAME)
  await expect(card).toHaveCount(1)
  await expectNoHorizontalScroll(page)
  const cardBox = await card.boundingBox()
  const nameBox = await card.getByText(LONG_NAME, { exact: true }).boundingBox()
  const menuBox = await accountMenuButton(page, LONG_NAME).boundingBox()
  if (cardBox === null || nameBox === null || menuBox === null) throw new Error('カード・名前・「⋯」の位置を取れない')
  expect(nameBox.x, '名前の左端がカードの中').toBeGreaterThanOrEqual(cardBox.x)
  expect(nameBox.x + nameBox.width, '名前の右端がカードの中').toBeLessThanOrEqual(cardBox.x + cardBox.width)
  expect(menuBox.x, '「⋯」の左端がカードの中').toBeGreaterThanOrEqual(cardBox.x)
  expect(menuBox.x + menuBox.width, '「⋯」の右端がカードの中').toBeLessThanOrEqual(cardBox.x + cardBox.width)
  expect(menuBox.y, '「⋯」の上端がカードの中').toBeGreaterThanOrEqual(cardBox.y)
  expect(menuBox.y + menuBox.height, '「⋯」の下端がカードの中').toBeLessThanOrEqual(cardBox.y + cardBox.height)

  // 削除の確認の文が、ダイアログの箱の中に収まる。
  const message = `${LONG_NAME}を削除しますか？この口座の記録はすべて削除されます。`
  const dialog = await openDeleteConfirm(page, LONG_NAME, message)
  const dialogBox = await dialog.boundingBox()
  const messageBox = await dialog.getByText(message, { exact: true }).boundingBox()
  if (dialogBox === null || messageBox === null) throw new Error('ダイアログ・文の位置を取れない')
  expect(messageBox.x, '文の左端がダイアログの中').toBeGreaterThanOrEqual(dialogBox.x)
  expect(messageBox.x + messageBox.width, '文の右端がダイアログの中').toBeLessThanOrEqual(dialogBox.x + dialogBox.width)
  await expectNoHorizontalScroll(page)
})

// =====================================================================================================
// 非機能（基準27）
// =====================================================================================================

test('おまかせ運用の基準27（非機能1）: 100銘柄（円50・ドル50。各20件、すべて価格あり）と口座10・記録各120件のとき、ローカル開発環境で /holdings を開いてから、2秒以内に、合計の欄・口座のカード10枚・株式のカード100枚がそろう（計測値を annotation と console.log に残す）', async ({ page, browser }) => {
  test.setTimeout(120_000)
  // 売買履歴の基準31 と同じデータ（円50・ドル50、各20件、すべて価格が保存済み）。
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
      return { code, side: isSell ? ('sell' as const) : ('buy' as const), quantity: isSell ? 10 : 100, price, tradeDate: `2026-02-${day}` }
    })
  // 口座10・記録各120件（月に1回で10年分。2016-12-01 から 2026-11-01 まで）。
  const ACCOUNT_COUNT = 10
  const RECORDS_PER_ACCOUNT = 120
  const accountNameList = Array.from({ length: ACCOUNT_COUNT }, (_, i) => `E2E口座${String(i + 1).padStart(2, '0')}`)
  const managedValuations: ManagedValuationSeed[] = accountNameList.flatMap((name, accountIndex) =>
    Array.from({ length: RECORDS_PER_ACCOUNT }, (_, month) => {
      const date = new Date(Date.UTC(2016, 11 + month, 1)).toISOString().slice(0, 10)
      return { name, date, valuation: 2000000 + accountIndex * 100000 + month * 10000, gain: 50000 + month * 1000 }
    }),
  )
  await insertHoldingsData({
    stocks,
    trades: [...yenCodes.flatMap((code) => tradesOf(code, 'JPY')), ...usdCodes.flatMap((code) => tradesOf(code, 'USD'))],
    prices: [
      ...yenCodes.map((code) => ({ code, tradingDate: '2026-10-06', closePrice: '3000.0' })),
      ...usdCodes.map((code) => ({ code, tradingDate: '2026-10-06', closePrice: '180.00' })),
    ],
    managedValuations,
  })

  // ウォームアップ: 一度 /holdings を開いて捨てる（dev サーバーの初回の変換などを計測に含めない）。
  await page.goto(HOLDINGS_PATH)
  await expect(holdingCards(page)).toHaveCount(allCodes.length)
  await expect(accountCards(page)).toHaveCount(ACCOUNT_COUNT)
  await page.close()

  // 計測: 新しいブラウザコンテキストで開き直す。
  const context = await browser.newContext()
  try {
    const measuredPage = await context.newPage()
    await measuredPage.goto(HOLDINGS_PATH)
    // 合計の欄（h2「合計」）があり、口座のカードが10枚、株式のカードが100枚そろった時点の performance.now() を、描画のフレームごとに確かめる。
    const shownAt = await measuredPage.waitForFunction(
      ({ accounts, stocksCount }) => {
        const hasSummary = Array.from(document.querySelectorAll('h2')).some((heading) => (heading.textContent ?? '').trim() === '合計')
        if (!hasSummary) return false
        const accountItems = document.querySelectorAll('ul[aria-label="おまかせ運用"] > li')
        const stockItems = document.querySelectorAll('ul[aria-label="保有株式"] > li')
        return accountItems.length >= accounts && stockItems.length >= stocksCount && performance.now()
      },
      { accounts: ACCOUNT_COUNT, stocksCount: allCodes.length },
      { polling: 'raf', timeout: 30_000 },
    )
    const measuredMs = Number(await shownAt.jsonValue())
    const summary = `${measuredMs.toFixed(1)}ms（上限 2000ms、余裕 ${(2000 - measuredMs).toFixed(1)}ms）`
    test.info().annotations.push({ type: '基準27 計測値', description: summary })
    console.log(`おまかせ運用の基準27 計測値: ${summary}`)

    expect(measuredMs).toBeLessThanOrEqual(2000)
    // 判定の後に、合計の欄・口座のカード10枚・株式のカード100枚がそろっていることも確かめる。
    await expect(summaryRegion(measuredPage)).toBeVisible()
    await expect(accountCards(measuredPage)).toHaveCount(ACCOUNT_COUNT)
    await expect(holdingCards(measuredPage)).toHaveCount(allCodes.length)
  } finally {
    await context.close()
  }
})
