import { expect, test, type Locator, type Page } from '@playwright/test'
import {
  insertHoldingsData,
  resetHoldingsData,
  type StockPriceSeed,
  type StockSeed,
  type TradeSeed,
} from '../fixtures/db'
import {
  GAIN_COLOR,
  LOSS_COLOR,
  UNAVAILABLE,
  UNPRICED_NOTE,
  cardCode,
  cardGain,
  cardList,
  cardValue,
  computedColor,
  computedColors,
  contrastRatios,
  currencyPart,
  expectCard,
  expectNoHorizontalScroll,
  expectTopToBottom,
  gainAndOtherFontSizes,
  holdingCard,
  holdingCards,
  partGain,
  refreshButton,
  summaryRegion,
} from '../fixtures/holdings-page'

// 仕様: カード表示の要件 docs/requirements/holdings-cards.md の基準1〜8（PR① の部分）・基準14・基準15・基準18・基準36（E2E）。
// テスト計画: プラン docs/plans/holdings-cards/plan.md 5.1（E2E の共通の決まり: 色は計算後の color をテストの中の定数と比べる、
//   「色が付かない」は同じカードの銘柄コードの文字の色と同じ、並びは <ul aria-label="保有株式"> の <li> の DOM の順）。
// 画面の構造: プラン 4.4（PR①）。カード（<li>）は 銘柄コード・「⋯」、銘柄名、「含み損益」の見出しと含み損益・率（data-tone）、<dl> の項目。
// 実行の前提: stock-holdings のプラン 4.5「E2E」（zero_e2e、API は利用者A で起動、beforeEach でデータを入れ替える）。
// テストの名前の先頭は「カード表示の基準N」（既存の保有株式一覧・売買履歴の基準と区別するため）。

test.describe.configure({ mode: 'default' })

test.beforeEach(async () => {
  await resetHoldingsData()
})

const HOLDINGS_PATH = '/holdings'

// ---- テストデータ（要件定義「成功基準」の銘柄マスタとダミー価格。取引日は 2026-01-10 の買い） ----
const BUY_DATE = '2026-01-10'
/** 7203 を100株、取得価格 2,500円。 */
const TOYOTA_100: TradeSeed = { code: '7203', side: 'buy', quantity: 100, price: '2500.0', tradeDate: BUY_DATE }
/** 7203 の保存済みの終値（3,000円・2026-10-06）。 */
const TOYOTA_PRICE: StockPriceSeed = { code: '7203', tradingDate: '2026-10-06', closePrice: '3000.0' }
/** 6758 を10株、取得価格 4,000円（現在の価格 3,500円なので -5,000円）。 */
const SONY_10: TradeSeed = { code: '6758', side: 'buy', quantity: 10, price: '4000.0', tradeDate: BUY_DATE }
/** 6758 の保存済みの終値（3,500円・2026-10-06）。 */
const SONY_PRICE: StockPriceSeed = { code: '6758', tradingDate: '2026-10-06', closePrice: '3500.0' }
/** 9984 を100株、取得価格 4,500円（価格は入れない）。 */
const SOFTBANK_100: TradeSeed = { code: '9984', side: 'buy', quantity: 100, price: '4500.0', tradeDate: BUY_DATE }
const APPLE: StockSeed = { code: 'AAPL', name: 'Apple', currency: 'USD' }
const MICROSOFT: StockSeed = { code: 'MSFT', name: 'Microsoft', currency: 'USD' }
/** AAPL を10株、取得価格 150.25ドル。 */
const APPLE_10: TradeSeed = { code: 'AAPL', side: 'buy', quantity: 10, price: '150.25', tradeDate: BUY_DATE }
/** AAPL の保存済みの終値（180.00ドル・2026-10-06）。 */
const APPLE_PRICE: StockPriceSeed = { code: 'AAPL', tradingDate: '2026-10-06', closePrice: '180.00' }
/** MSFT を5株、取得価格 400.00ドル（価格は入れない）。 */
const MICROSOFT_5: TradeSeed = { code: 'MSFT', side: 'buy', quantity: 5, price: '400.00', tradeDate: BUY_DATE }

/** 基準1 のカード: 7203 の「+50,000円（+20.0%）」ほか。 */
const TOYOTA_CARD = {
  code: '7203',
  name: 'トヨタ自動車',
  gain: '+50,000円（+20.0%）',
  valuation: '300,000円',
  quantity: '100株',
  acquisitionPrice: '2,500円',
  currentPrice: '3,000円',
  priceDate: '2026-10-06',
} as const

/** 銘柄コードの順（DOM の順）でカードを読んだとき、各カードがその銘柄コードを持つこと（カードが codes の枚数ちょうどあること）。 */
async function expectCardOrder(page: Page, codes: readonly string[]): Promise<void> {
  await expect(holdingCards(page)).toHaveCount(codes.length)
  for (let i = 0; i < codes.length; i += 1) {
    await expect(holdingCards(page).nth(i)).toContainText(codes[i])
  }
}

test('カード表示の基準1（要件1・要件2・要件7・要件9・要件10）: 7203「買 2,500円 100株」で価格が保存済みのとき、7203 のカードに「7203 / トヨタ自動車 / +50,000円（+20.0%）/ 評価額 300,000円 / 保有数量 100株 / 取得価格 2,500円 / 現在の価格 3,000円 / 価格の基準日 2026-10-06」が上からこの順に表示され、表（table）は表示されない', async ({ page }) => {
  await insertHoldingsData({ trades: [TOYOTA_100], prices: [TOYOTA_PRICE] })

  await page.goto(HOLDINGS_PATH)

  const card = holdingCard(page, '7203')
  await expect(card).toBeVisible()
  await expectCard(card, TOYOTA_CARD)
  // 上からこの順（DOM の順でも、画面の上から下の順でも）。含み損益の見出しは、銘柄名と含み損益の間にある。
  await expectTopToBottom(
    [
      cardCode(card, '7203'),
      card.getByText('トヨタ自動車', { exact: true }),
      card.getByText('含み損益', { exact: true }),
      cardGain(card),
      card.locator('dt:text-is("評価額")'),
      card.locator('dt:text-is("保有数量")'),
      card.locator('dt:text-is("取得価格")'),
      card.locator('dt:text-is("現在の価格")'),
      card.locator('dt:text-is("価格の基準日")'),
    ],
    ['銘柄コード', '銘柄名', '含み損益の見出し', '含み損益と率', '評価額', '保有数量', '取得価格', '現在の価格', '価格の基準日'],
  )
  // 銘柄コードと同じ行（上端が近い）に「⋯」のボタンがある（要件2・要件6）。
  const codeBox = await cardCode(card, '7203').boundingBox()
  const menuBox = await card.getByRole('button', { name: 'トヨタ自動車の操作', exact: true }).boundingBox()
  if (codeBox === null || menuBox === null) throw new Error('銘柄コードか「⋯」の位置を取れない')
  expect(Math.abs(menuBox.y + menuBox.height / 2 - (codeBox.y + codeBox.height / 2))).toBeLessThan(codeBox.height)
  // 表は表示されない。
  await expect(page.getByRole('table')).toHaveCount(0)
})

test('カード表示の基準2（要件2・要件10）: AAPL「買 150.25ドル 10株」で価格が保存済みのとき、AAPL のカードに「+297.50ドル（+19.8%）/ 評価額 1,800.00ドル / 保有数量 10株 / 取得価格 150.25ドル / 現在の価格 180.00ドル」が表示される', async ({ page }) => {
  await insertHoldingsData({ stocks: [APPLE], trades: [APPLE_10], prices: [APPLE_PRICE] })

  await page.goto(HOLDINGS_PATH)

  await expectCard(holdingCard(page, 'AAPL'), {
    code: 'AAPL',
    name: 'Apple',
    gain: '+297.50ドル（+19.8%）',
    valuation: '1,800.00ドル',
    quantity: '10株',
    acquisitionPrice: '150.25ドル',
    currentPrice: '180.00ドル',
    priceDate: '2026-10-06',
  })
})

test('カード表示の基準3（要件4・要件10）: 7203「買 2,500円 100株」と 6758「買 4,000円 10株」で、どちらも価格が保存済みのとき、7203 のカードの「+50,000円（+20.0%）」は赤、6758 のカードの「-5,000円（-12.5%）」は青で表示される', async ({ page }) => {
  await insertHoldingsData({ trades: [TOYOTA_100, SONY_10], prices: [TOYOTA_PRICE, SONY_PRICE] })

  await page.goto(HOLDINGS_PATH)

  const toyotaGain = cardGain(holdingCard(page, '7203'))
  const sonyGain = cardGain(holdingCard(page, '6758'))
  await expect(toyotaGain).toHaveText('+50,000円（+20.0%）')
  await expect(sonyGain).toHaveText('-5,000円（-12.5%）')
  // 含み損益と率の文字を持つ要素と、その子孫のすべての文字の色（計算後）が、赤・青の定数と同じ。
  for (const color of await computedColors(toyotaGain)) expect(color).toBe(GAIN_COLOR)
  for (const color of await computedColors(sonyGain)) expect(color).toBe(LOSS_COLOR)
})

test('カード表示の基準4（要件4・要件10）: 7203「買 3,000円 10株」で現在の価格が 3,000円のとき、含み損益は「0円（0.0%）」で、赤・青は付かない（銘柄コードの文字と同じ色）', async ({ page }) => {
  await insertHoldingsData({
    trades: [{ code: '7203', side: 'buy', quantity: 10, price: '3000.0', tradeDate: BUY_DATE }],
    prices: [TOYOTA_PRICE],
  })

  await page.goto(HOLDINGS_PATH)

  const card = holdingCard(page, '7203')
  await expect(cardGain(card)).toHaveText('0円（0.0%）')
  const codeColor = await computedColor(cardCode(card, '7203'))
  for (const color of await computedColors(cardGain(card))) expect(color).toBe(codeColor)
  expect(codeColor).not.toBe(GAIN_COLOR)
  expect(codeColor).not.toBe(LOSS_COLOR)
})

test('カード表示の基準5（要件5）: 9984「買 4,500円 100株」で価格が一度も保存されていないとき、9984 のカードの含み損益は「取得できません」だけで、率は表示されず、赤・青は付かない。現在の価格・価格の基準日・評価額も「取得できません」と表示される', async ({ page }) => {
  await insertHoldingsData({ trades: [SOFTBANK_100] })

  await page.goto(HOLDINGS_PATH)

  const card = holdingCard(page, '9984')
  await expect(card).toBeVisible()
  await expect(cardGain(card)).toHaveText(UNAVAILABLE)
  await expect(card).not.toContainText('%')
  await expect(card).not.toContainText('+')
  const codeColor = await computedColor(cardCode(card, '9984'))
  for (const color of await computedColors(cardGain(card))) expect(color).toBe(codeColor)
  await expect(cardValue(card, '現在の価格')).toHaveText(UNAVAILABLE)
  await expect(cardValue(card, '価格の基準日')).toHaveText(UNAVAILABLE)
  await expect(cardValue(card, '評価額')).toHaveText(UNAVAILABLE)
  // 保有数量と取得価格は、価格がなくても表示する。
  await expect(cardValue(card, '保有数量')).toHaveText('100株')
  await expect(cardValue(card, '取得価格')).toHaveText('4,500円')
})

/** 基準14 のデータ: 7203「買 2,500円 100株」、6758「買 4,000円 10株」、AAPL「買 150.25ドル 10株」。どれも価格が保存済み。 */
async function insertThreeHoldings(): Promise<void> {
  await insertHoldingsData({
    stocks: [APPLE],
    trades: [TOYOTA_100, SONY_10, APPLE_10],
    prices: [TOYOTA_PRICE, SONY_PRICE, APPLE_PRICE],
  })
}

test('カード表示の基準6（要件3・要件14）: 基準1 のカードと基準14 の合計の欄（円とドル）で、含み損益と率の文字が、同じカード・合計の欄の中のほかのすべての文字（銘柄コード・銘柄名・見出し・値・「⋯」・注記）より厳密に大きい。注記が出る合計の欄（基準15 のデータ）でも同じ', async ({ page }) => {
  await insertThreeHoldings()

  await page.goto(HOLDINGS_PATH)

  const card = holdingCard(page, '7203')
  await expectCard(card, TOYOTA_CARD)
  const cardSizes = await gainAndOtherFontSizes(card)
  expect(cardSizes.gainMin, `カードの含み損益と率の文字（最小 ${cardSizes.gainMin}px）が、ほかの文字（最大 ${cardSizes.othersMax}px）より大きい`).toBeGreaterThan(cardSizes.othersMax)

  const summary = summaryRegion(page)
  await expect(currencyPart(page, '円')).toBeVisible()
  await expect(currencyPart(page, 'ドル')).toBeVisible()
  // 合計の欄は、円とドルの両方の含み損益（data-tone を持つ要素が2つ）をまとめて見る。
  await expect(summary.locator('[data-tone]')).toHaveCount(2)
  const summarySizes = await gainAndOtherFontSizes(summary)
  expect(summarySizes.gainMin, `合計の欄の含み損益と率の文字（最小 ${summarySizes.gainMin}px）が、ほかの文字（見出し h2・h3 を含む。最大 ${summarySizes.othersMax}px）より大きい`).toBeGreaterThan(summarySizes.othersMax)

  // 注記が出る合計の欄（基準15 のデータ。注記は円とドルに1つずつ）でも、含み損益と率の文字が、注記を含むほかのすべての文字より大きい。
  await resetHoldingsData()
  await insertHoldingsData({
    stocks: [MICROSOFT],
    trades: [TOYOTA_100, SOFTBANK_100, MICROSOFT_5],
    prices: [TOYOTA_PRICE],
  })
  await page.goto(HOLDINGS_PATH)

  await expect(summary.getByText(UNPRICED_NOTE)).toHaveCount(2)
  await expect(summary.locator('[data-tone]')).toHaveCount(2)
  const noteSizes = await gainAndOtherFontSizes(summary)
  // 注記の要素が、比べる「ほかの文字」に入っている（円とドルで2つ）。
  expect(noteSizes.othersTexts.filter((text) => text === UNPRICED_NOTE), '比べに入った注記の要素').toHaveLength(2)
  expect(noteSizes.gainMin, `注記がある合計の欄の含み損益と率の文字（最小 ${noteSizes.gainMin}px）が、注記を含むほかの文字（最大 ${noteSizes.othersMax}px）より大きい`).toBeGreaterThan(noteSizes.othersMax)
})

test('カード表示の基準7（要件1）: 6758・7203・9984・AAPL を保有しているとき、幅 1,280px では1行目に3枚・2行目に1枚のカードが並び、幅 390px では1行に1枚ずつ並んで画面は横にスクロールしない', async ({ page }) => {
  await insertHoldingsData({
    stocks: [APPLE],
    trades: [SONY_10, TOYOTA_100, SOFTBANK_100, APPLE_10],
    prices: [TOYOTA_PRICE, SONY_PRICE, APPLE_PRICE],
  })

  // 幅 1,280px: 上端が同じカードが1行目に3つ、2行目に1つ。2行目の左端は1行目の1枚目の左端と同じ（詰めて並べ、空きマスを作らない）。
  await page.setViewportSize({ width: 1280, height: 720 })
  await page.goto(HOLDINGS_PATH)
  await expectCardOrder(page, ['6758', '7203', '9984', 'AAPL'])
  const wide = await holdingCards(page).evaluateAll((items) =>
    items.map((item) => {
      const rect = item.getBoundingClientRect()
      return { top: rect.top, left: rect.left }
    }),
  )
  expect(wide, '4枚のカード').toHaveLength(4)
  expect(Math.abs(wide[1].top - wide[0].top), '2枚目は1枚目と同じ行').toBeLessThan(1)
  expect(Math.abs(wide[2].top - wide[0].top), '3枚目は1枚目と同じ行').toBeLessThan(1)
  expect(wide[3].top, '4枚目は2行目').toBeGreaterThan(wide[0].top + 1)
  expect(Math.abs(wide[3].left - wide[0].left), '4枚目の左端は1枚目の左端と同じ').toBeLessThan(1)
  expect(wide[1].left, '2枚目は1枚目の右').toBeGreaterThan(wide[0].left + 1)
  expect(wide[2].left, '3枚目は2枚目の右').toBeGreaterThan(wide[1].left + 1)
  await expectNoHorizontalScroll(page)

  // 幅 390px: 1行に1枚ずつ（上端が増えていき、左端がそろう）。画面は横にスクロールしない。
  await page.setViewportSize({ width: 390, height: 844 })
  await expectCardOrder(page, ['6758', '7203', '9984', 'AAPL'])
  const narrow = await holdingCards(page).evaluateAll((items) =>
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

test('カード表示の基準8（要件6。PR① の部分）: 7203 のカードの「⋯」（「トヨタ自動車の操作」）を押すと「削除」のメニューが開き、カードの並びは変わらない', async ({ page }) => {
  await insertHoldingsData({
    stocks: [APPLE],
    trades: [SONY_10, TOYOTA_100, SOFTBANK_100, APPLE_10],
    prices: [TOYOTA_PRICE, SONY_PRICE, APPLE_PRICE],
  })
  await page.goto(HOLDINGS_PATH)
  await expectCardOrder(page, ['6758', '7203', '9984', 'AAPL'])

  await holdingCard(page, '7203').getByRole('button', { name: 'トヨタ自動車の操作', exact: true }).click()

  await expect(page.getByRole('menuitem', { name: '削除', exact: true })).toBeVisible()
  await expect(page.getByRole('menuitem')).toHaveCount(1)
  await expectCardOrder(page, ['6758', '7203', '9984', 'AAPL'])
})

test('カード表示の基準14（要件11・要件12・要件13）: 7203「買 2,500円 100株」、6758「買 4,000円 10株」、AAPL「買 150.25ドル 10株」で、どれも価格が保存済みのとき、カードの並びの上の合計の欄に、円「含み損益 +45,000円（+15.5%）/ 評価額 335,000円」、ドル「含み損益 +297.50ドル（+19.8%）/ 評価額 1,800.00ドル」がこの順に表示され、「価格を取得できない銘柄を含みません」は表示されない。合計の欄は「価格を今すぐ更新」のボタンより上にある', async ({ page }) => {
  await insertThreeHoldings()

  await page.goto(HOLDINGS_PATH)

  const summary = summaryRegion(page)
  await expect(summary).toBeVisible()
  await expect(summary.getByRole('heading', { level: 2, name: '合計', exact: true })).toBeVisible()
  const yen = currencyPart(page, '円')
  const usd = currencyPart(page, 'ドル')
  await expect(summary.getByRole('region')).toHaveCount(2)
  // 円、ドルの順。各通貨の中は、見出し、「含み損益」、含み損益と率、「評価額」、評価額の順。
  await expectTopToBottom([yen, usd], ['円の部分', 'ドルの部分'])
  for (const [part, name, gain, valuation] of [
    [yen, '円', '+45,000円（+15.5%）', '335,000円'],
    [usd, 'ドル', '+297.50ドル（+19.8%）', '1,800.00ドル'],
  ] as const) {
    await expect(partGain(part)).toHaveText(gain)
    await expectTopToBottom(
      [
        part.getByRole('heading', { level: 3, name, exact: true }),
        part.getByText('含み損益', { exact: true }),
        partGain(part),
        part.getByText('評価額', { exact: true }),
        part.getByText(valuation, { exact: true }),
      ],
      [`${name}の見出し`, '含み損益の見出し', '含み損益と率', '評価額の見出し', '評価額'],
    )
  }
  await expect(page.getByText(UNPRICED_NOTE)).toHaveCount(0)

  // 合計の欄の下端 ≦ 「価格を今すぐ更新」のボタンの上端、ボタンの下端 ≦ カードの並びの上端。
  const summaryBox = await summary.boundingBox()
  const buttonBox = await refreshButton(page).boundingBox()
  const listBox = await cardList(page).boundingBox()
  if (summaryBox === null || buttonBox === null || listBox === null) throw new Error('合計の欄・ボタン・カードの並びの位置を取れない')
  expect(summaryBox.y + summaryBox.height, '合計の欄の下端が、ボタンの上端以上でない').toBeLessThanOrEqual(buttonBox.y + 0.5)
  expect(buttonBox.y + buttonBox.height, 'ボタンの下端が、カードの並びの上端以下').toBeLessThanOrEqual(listBox.y + 0.5)
})

test('カード表示の基準15（要件15・要件16）: 7203「買 2,500円 100株」（価格保存済み）、9984「買 4,500円 100株」（価格なし）、MSFT「買 400.00ドル 5株」（価格なし）のとき、合計の欄の円は「含み損益 +50,000円（+20.0%）/ 評価額 300,000円」と「価格を取得できない銘柄を含みません」、ドルは「含み損益 取得できません / 評価額 0.00ドル」と「価格を取得できない銘柄を含みません」になり、注記はページ全体で2つ', async ({ page }) => {
  await insertHoldingsData({
    stocks: [MICROSOFT],
    trades: [TOYOTA_100, SOFTBANK_100, MICROSOFT_5],
    prices: [TOYOTA_PRICE],
  })

  await page.goto(HOLDINGS_PATH)

  const yen = currencyPart(page, '円')
  const usd = currencyPart(page, 'ドル')
  await expect(partGain(yen)).toHaveText('+50,000円（+20.0%）')
  await expect(yen.getByText('300,000円', { exact: true })).toBeVisible()
  await expect(yen.getByText(UNPRICED_NOTE)).toHaveCount(1)
  await expect(partGain(usd)).toHaveText(UNAVAILABLE)
  await expect(usd).not.toContainText('%')
  await expect(usd.getByText('0.00ドル', { exact: true })).toBeVisible()
  await expect(usd.getByText(UNPRICED_NOTE)).toHaveCount(1)
  await expect(page.getByText(UNPRICED_NOTE)).toHaveCount(2)
  // 含み損益が「取得できません」の部分には、赤・青を付けない（同じ部分の見出しの色と同じ）。
  const headingColor = await computedColor(usd.getByRole('heading', { level: 3, name: 'ドル', exact: true }))
  for (const color of await computedColors(partGain(usd))) expect(color).toBe(headingColor)
})

test('カード表示の基準18（要件11）: 7203「買 2,000円 100株」「売 2,500円 100株」だけのとき、「保有株式はありません」と表示され、合計の欄もカードも表示されない', async ({ page }) => {
  await insertHoldingsData({
    trades: [
      { code: '7203', side: 'buy', quantity: 100, price: '2000.0', tradeDate: '2026-01-10' },
      { code: '7203', side: 'sell', quantity: 100, price: '2500.0', tradeDate: '2026-02-10' },
    ],
  })

  await page.goto(HOLDINGS_PATH)

  // 先に空のメッセージが出る（取得が終わった）のを待ってから、ないことを確かめる。
  await expect(page.getByText('保有株式はありません')).toBeVisible()
  await expect(summaryRegion(page)).toHaveCount(0)
  await expect(page.getByText('合計', { exact: true })).toHaveCount(0)
  await expect(cardList(page)).toHaveCount(0)
  await expect(holdingCards(page)).toHaveCount(0)
})

/** 要素 element と、その子孫のうち文字を持つすべての要素の、計算後のコントラスト比が 4.5 以上であること。 */
async function expectContrastAtLeast(element: Locator, name: string): Promise<void> {
  const ratios = await contrastRatios(element)
  expect(ratios.length, `${name} の文字を持つ要素がある`).toBeGreaterThan(0)
  for (const { text, ratio } of ratios) {
    expect(ratio, `${name} の「${text}」の文字と背景のコントラスト比（${ratio.toFixed(2)}）が 4.5 以上`).toBeGreaterThanOrEqual(4.5)
  }
}

test('カード表示の基準36（非機能3）: 基準3 と同じデータで開いたとき、赤と青の含み損益の文字の色と、その背景の色のコントラスト比が、どちらも 4.5:1 以上（カード2つと、合計の欄の gain・loss の要素）', async ({ page }) => {
  // 基準3 のデータ（7203 は +50,000円、6758 は -5,000円）に、AAPL「買 200.00ドル 10株」（現在の価格 180.00ドルで -200.00ドル）を足す。
  // 円の合計は +45,000円（赤）、ドルの合計は -200.00ドル（青）になり、合計の欄でも赤と青の両方を確かめられる。
  await insertHoldingsData({
    stocks: [APPLE],
    trades: [TOYOTA_100, SONY_10, { code: 'AAPL', side: 'buy', quantity: 10, price: '200.00', tradeDate: BUY_DATE }],
    prices: [TOYOTA_PRICE, SONY_PRICE, APPLE_PRICE],
  })

  await page.goto(HOLDINGS_PATH)

  const toyotaGain = cardGain(holdingCard(page, '7203'))
  const sonyGain = cardGain(holdingCard(page, '6758'))
  const yenTotal = partGain(currencyPart(page, '円'))
  const usdTotal = partGain(currencyPart(page, 'ドル'))
  await expect(toyotaGain).toHaveText('+50,000円（+20.0%）')
  await expect(sonyGain).toHaveText('-5,000円（-12.5%）')
  await expect(yenTotal).toHaveText('+45,000円（+15.5%）')
  await expect(usdTotal).toHaveText('-200.00ドル（-10.0%）')
  // 赤と青の色であること（テストの中の定数。ページの CSS 変数からは読まない）と、コントラスト比。
  // カードの2つと合計の欄の2つ、すべてで、要素とその子孫の色がすべて赤か青の定数で、文字を持つ各要素のコントラスト比が 4.5 以上。
  for (const [element, expected, name] of [
    [toyotaGain, GAIN_COLOR, '7203 のカードの含み損益（赤）'],
    [sonyGain, LOSS_COLOR, '6758 のカードの含み損益（青）'],
    [yenTotal, GAIN_COLOR, '合計の欄の円の含み損益（赤）'],
    [usdTotal, LOSS_COLOR, '合計の欄のドルの含み損益（青）'],
  ] as const) {
    for (const color of await computedColors(element)) expect(color, `${name} とその子孫の色`).toBe(expected)
    await expectContrastAtLeast(element, name)
  }
})
