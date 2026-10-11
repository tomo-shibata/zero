import { expect, test, type Locator, type Page } from '@playwright/test'
import {
  insertHoldingsData,
  resetHoldingsData,
  type HoldingSeed,
  type StockPriceSeed,
} from '../fixtures/db'

// 仕様: 要件 docs/requirements/stock-holdings.md（保有株式一覧）の E2E の AC と、補助のテスト。
// テスト計画: プラン docs/plans/stock-holdings/plan.md 5.3 の各行。
// 画面の文言と構造: プラン 4.4「フロント（TypeScript）」（画面の状態ごとの表示、合計の領域、表の構造、日付は変換しない）。
// 実行の前提: プラン 4.5「E2E」（zero_e2e、API は利用者A で起動、beforeEach でデータを入れ替える）。
// mock は境界の外側（ブラウザから見たネットワーク = page.route）にだけ使う。

// 1件失敗しても後続を止めずに、順に実行する（同じ DB を使うため。プラン 4.5）。
test.describe.configure({ mode: 'default' })

// 補助 FR-14（TC-25）: このファイル全体を UTC より遅いタイムゾーンで実行し、
// 価格の基準日（API の "YYYY-MM-DD"）が Date に変換されてずれないことを確かめる（プラン 4.4「日付は変換しない」、4.5）。
const BROWSER_TIMEZONE = 'America/Los_Angeles'
test.use({ timezoneId: BROWSER_TIMEZONE })

// 各テストの前に、保有データ・終値と test.sql にない銘柄マスタを消す（プラン 4.5）。
// そのテストのデータは、各テストの中で画面を開く前に入れる。
test.beforeEach(async () => {
  await resetHoldingsData()
})

const HOLDINGS_PATH = '/holdings'
const DASHBOARD_PATH = '/dashboard'
/** 一覧の API（プラン 4.4「API」）。ブラウザからは Vite の proxy 経由で同じオリジンの /api/holdings を呼ぶ。 */
const HOLDINGS_API = '**/api/holdings'

// ---- 画面の文言（要件 FR-6・7・10・21・22、プラン 4.4） ----
const PAGE_TITLE = '保有株式一覧'
const TOTAL_LABEL = '合計評価額'
const UNPRICED_NOTE = '価格を取得できない銘柄を含みません'
const EMPTY_MESSAGE = '保有株式はありません'
const UNAVAILABLE = '取得できません'
const LOADING_LABEL = '読み込み中'
const FETCH_ERROR_MESSAGE = '保有株式の取得に失敗しました'
const CLOSE_BUTTON = '閉じる'
/** 表の列見出し（要件 FR-3 の順。プラン 4.4「表」）。 */
const COLUMN_HEADERS = [
  '銘柄コード',
  '銘柄名',
  '保有数量',
  '取得価格',
  '現在の価格',
  '価格の基準日',
  '評価額',
]

// ---- テストデータ（要件「成功基準」の銘柄マスタとダミー価格） ----
// 価格はダミーの取得元を通さず、「保存済み」の状態として DB に直接入れる。
/** AC-1: 7203 を100株、取得価格2,500円。 */
const TOYOTA_100: HoldingSeed = { code: '7203', quantity: 100, acquisitionPrice: '2500.0' }
/** 7203 の保存済みの終値（3,000円・2026-10-06）。 */
const TOYOTA_PRICE: StockPriceSeed = { code: '7203', tradingDate: '2026-10-06', closePrice: '3000.0' }
/** AC-7: 6758 を10株（取得価格は AC にないので任意の値）。 */
const SONY_10: HoldingSeed = { code: '6758', quantity: 10, acquisitionPrice: '3200.0' }
/** 6758 の保存済みの終値（3,500円・2026-10-06）。 */
const SONY_PRICE: StockPriceSeed = { code: '6758', tradingDate: '2026-10-06', closePrice: '3500.0' }
/** 9984 を50株（数量・取得価格は AC にないので任意の値）。9984 の価格は入れない（一度も保存されていない）。 */
const SOFTBANK_50: HoldingSeed = { code: '9984', quantity: 50, acquisitionPrice: '4500.0' }

/** AC-1: 7203 の行の7つのセル（要件 AC-1 の値そのまま）。 */
const TOYOTA_ROW = ['7203', 'トヨタ自動車', '100株', '2,500円', '3,000円', '2026-10-06', '300,000円']

// ---- 画面の要素（プラン 4.4 の文言と構造） ----
function holdingsHeading(page: Page): Locator {
  return page.getByRole('heading', { name: PAGE_TITLE, exact: true })
}

/** 合計の領域（<section aria-label="合計評価額">）。 */
function totalRegion(page: Page): Locator {
  return page.getByRole('region', { name: TOTAL_LABEL, exact: true })
}

function holdingsTable(page: Page): Locator {
  return page.getByRole('table')
}

/** 銘柄コードのセルが code と一致するデータ行。 */
function holdingRow(page: Page, code: string): Locator {
  return holdingsTable(page)
    .getByRole('row')
    .filter({ has: page.getByRole('cell', { name: code, exact: true }) })
}

function errorDialog(page: Page): Locator {
  return page.getByRole('alertdialog', { name: FETCH_ERROR_MESSAGE, exact: true })
}

function loadingStatus(page: Page): Locator {
  return page.getByRole('status', { name: LOADING_LABEL, exact: true })
}

function holdingsLink(page: Page): Locator {
  return page.getByRole('link', { name: PAGE_TITLE, exact: true })
}

/**
 * 金額の表示（例 "335,000円"）が、前に数字や「,」が続かない形で現れることを表す正規表現。
 * "0円" が "300,000円" の一部に、"335,000円" が "1,335,000円" の一部に当たらないようにする。
 */
function amountPattern(amount: string): RegExp {
  const escaped = amount.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return new RegExp(`(^|[^0-9,])${escaped}`)
}

/**
 * 取得に失敗したときの表示（要件 FR-22、プラン 4.4 の状態の表の「取得失敗」と「ダイアログを閉じた後」）を確かめる。
 * AC-9b（HTTP 500）と補助 FR-22（TC-15、通信エラー）で共通。
 */
async function expectFetchErrorDialogAndNothingAfterClose(page: Page): Promise<void> {
  // 「保有株式の取得に失敗しました」と書いたダイアログが表示される。
  const dialog = errorDialog(page)
  await expect(dialog).toBeVisible()
  await expect(dialog).toContainText(FETCH_ERROR_MESSAGE)
  // ダイアログが出た（取得が終わった）ので、一覧と合計評価額が出ていないことを確かめる。
  await expect(holdingsTable(page)).toHaveCount(0)
  await expect(totalRegion(page)).toHaveCount(0)

  // 「閉じる」を押すとダイアログが閉じる。
  await dialog.getByRole('button', { name: CLOSE_BUTTON, exact: true }).click()
  await expect(dialog).toHaveCount(0)

  // 閉じた後は見出しだけ（プラン 4.4）。見出しが出ているのを待ってから、一覧と合計評価額がないことを確かめる。
  await expect(holdingsHeading(page)).toBeVisible()
  await expect(holdingsTable(page)).toHaveCount(0)
  await expect(totalRegion(page)).toHaveCount(0)
  await expect(page.getByText(TOTAL_LABEL)).toHaveCount(0)
}

test('AC-1・補助 FR-14（TC-25）: 7203 の行に「7203 / トヨタ自動車 / 100株 / 2,500円 / 3,000円 / 2026-10-06 / 300,000円」が表示される（America/Los_Angeles でも日付が変わらない）', async ({ page }) => {
  // Given: 利用者Aが 7203 を100株、取得価格2,500円で保有し、7203 の価格（3,000円・2026-10-06）が保存済み。
  await insertHoldingsData({ holdings: [TOYOTA_100], prices: [TOYOTA_PRICE] })

  await page.goto(HOLDINGS_PATH)

  // TC-25 の前提: ブラウザのタイムゾーンが UTC より遅い America/Los_Angeles になっている
  // （"2026-10-06" を Date に変換すると 2026-10-05 になるタイムゾーン）。
  expect(
    await page.evaluate(() => Intl.DateTimeFormat().resolvedOptions().timeZone),
  ).toBe(BROWSER_TIMEZONE)

  await expect(holdingsHeading(page)).toBeVisible()
  // 列の並び（要件 FR-3。プラン 4.4「表」の列見出し）。
  await expect(holdingsTable(page).getByRole('columnheader')).toHaveText(COLUMN_HEADERS)
  // 7203 の行の7つのセル（AC-1）。価格の基準日は 2026-10-06 のまま（TC-25）。
  await expect(holdingRow(page, '7203').getByRole('cell')).toHaveText(TOYOTA_ROW)
})

test('AC-3: ダッシュボードでリンク「保有株式一覧」を押すと /holdings が表示される', async ({ page }) => {
  await page.goto(DASHBOARD_PATH)

  await holdingsLink(page).click()

  await expect(page).toHaveURL(/\/holdings$/)
  await expect(holdingsHeading(page)).toBeVisible()
})

test('AC-7: 7203 を100株、6758 を10株保有し、両方の価格が保存済みのとき、合計評価額に 335,000円 が表示される', async ({ page }) => {
  // Given: 7203（100株 × 3,000円）と 6758（10株 × 3,500円）。合計 300,000円 + 35,000円 = 335,000円。
  await insertHoldingsData({
    holdings: [TOYOTA_100, SONY_10],
    prices: [TOYOTA_PRICE, SONY_PRICE],
  })

  await page.goto(HOLDINGS_PATH)

  const total = totalRegion(page)
  await expect(total).toContainText(amountPattern('335,000円'))
  // 合計の領域に「合計評価額」の文字がある（プラン 4.4「合計の領域」）。
  await expect(total).toContainText(TOTAL_LABEL)
})

test('AC-8: 7203（価格あり）と 9984（価格なし）を保有しているとき、合計評価額に 300,000円 と「価格を取得できない銘柄を含みません」が表示される', async ({ page }) => {
  // Given: 7203 を100株（価格保存済み）、9984（価格未保存）。
  await insertHoldingsData({
    holdings: [TOYOTA_100, SOFTBANK_50],
    prices: [TOYOTA_PRICE],
  })

  await page.goto(HOLDINGS_PATH)

  const total = totalRegion(page)
  await expect(total).toContainText(amountPattern('300,000円'))
  // 注記は合計評価額の近く（合計の領域の中。プラン 4.4）に出る。
  await expect(total).toContainText(UNPRICED_NOTE)
})

test('AC-8a: 全銘柄の価格が保存済みのとき、「価格を取得できない銘柄を含みません」は表示されない', async ({ page }) => {
  // Given: AC-7 と同じ。
  await insertHoldingsData({
    holdings: [TOYOTA_100, SONY_10],
    prices: [TOYOTA_PRICE, SONY_PRICE],
  })

  await page.goto(HOLDINGS_PATH)

  // 先に合計評価額が出る（取得が終わった）のを待ってから、注記がないことを確かめる（プラン 4.5）。
  await expect(totalRegion(page)).toContainText(amountPattern('335,000円'))
  await expect(page.getByText(UNPRICED_NOTE)).toHaveCount(0)
})

test('AC-9: 1件も保有していないとき、「保有株式はありません」が表示され、合計評価額は表示されない', async ({ page }) => {
  // Given: 利用者Aの保有データなし（beforeEach で消したまま）。

  await page.goto(HOLDINGS_PATH)

  // 先に空のメッセージが出る（取得が終わった）のを待ってから、合計評価額がないことを確かめる（プラン 4.5）。
  await expect(page.getByText(EMPTY_MESSAGE)).toBeVisible()
  await expect(totalRegion(page)).toHaveCount(0)
  await expect(page.getByText(TOTAL_LABEL)).toHaveCount(0)
  // 0件のときは表も出さない（プラン 4.4 の状態の表）。
  await expect(holdingsTable(page)).toHaveCount(0)
})

test('AC-9b: 一覧の API が HTTP 500 を返すとき、「保有株式の取得に失敗しました」のダイアログが表示され、「閉じる」を押すと閉じて一覧と合計評価額は表示されない', async ({ page }) => {
  // DB には保有データを入れておく（API が成功すれば一覧が出るデータにして、「表示されない」が空のデータのせいでないようにする）。
  await insertHoldingsData({ holdings: [TOYOTA_100], prices: [TOYOTA_PRICE] })
  // Given: 一覧の API がエラー（HTTP 500。本文はプラン 4.4「API」の失敗時の形）を返す。
  await page.route(HOLDINGS_API, (route) =>
    route.fulfill({
      status: 500,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'internal_error' }),
    }),
  )

  await page.goto(HOLDINGS_PATH)

  await expectFetchErrorDialogAndNothingAfterClose(page)
})

test('AC-11: 9984 の価格が一度も保存されていないとき、9984 の行の現在の価格・価格の基準日・評価額は「取得できません」で、7203 の行は AC-1 のとおり表示される', async ({ page }) => {
  // Given: 7203（100株・2,500円、価格保存済み）と 9984（50株・4,500円、価格は一度も保存されていない）。
  await insertHoldingsData({
    holdings: [TOYOTA_100, SOFTBANK_50],
    prices: [TOYOTA_PRICE],
  })

  await page.goto(HOLDINGS_PATH)

  // 9984 の行: 現在の価格・価格の基準日・評価額の3つのセルが「取得できません」（FR-10）。
  // ほかのセルは FR-3・12・13 のとおり（銘柄名は銘柄マスタから。FR-20）。
  await expect(holdingRow(page, '9984').getByRole('cell')).toHaveText([
    '9984',
    'ソフトバンクグループ',
    '50株',
    '4,500円',
    UNAVAILABLE,
    UNAVAILABLE,
    UNAVAILABLE,
  ])
  // 7203 の行は通常どおり（AC-1 のとおり）。
  await expect(holdingRow(page, '7203').getByRole('cell')).toHaveText(TOYOTA_ROW)
})

test('AC-18・補助 NFR-3（PR4_TC04）: 100銘柄（すべて価格が保存済み）を保有しているとき、/holdings を開いてから2秒以内に一覧が表示される（計測値を annotation に残す）', async ({ page, browser }) => {
  // Given: 利用者Aが100銘柄を保有し、すべて価格が保存済み。
  // test.sql にない銘柄なので、銘柄マスタにも入れる（次のテストの beforeEach で消える）。
  const codes = Array.from({ length: 100 }, (_, i) => String(1001 + i))
  await insertHoldingsData({
    stocks: codes.map((code) => ({ code, name: `E2E銘柄${code}` })),
    holdings: codes.map((code) => ({ code, quantity: 100, acquisitionPrice: '2500.0' })),
    prices: codes.map((code) => ({ code, tradingDate: '2026-10-06', closePrice: '3000.0' })),
  })

  // ウォームアップ: 一度 /holdings を開いて捨てる（dev サーバーの初回の変換などを計測に含めない。プラン 5.3）。
  await page.goto(HOLDINGS_PATH)
  await expect(page.locator('tbody tr')).toHaveCount(codes.length)
  await page.close()

  // 計測: 新しいブラウザコンテキストで開き直す（ウォームアップのページのキャッシュを使わない）。
  const context = await browser.newContext()
  try {
    const measuredPage = await context.newPage()
    await measuredPage.goto(HOLDINGS_PATH)
    // 一覧の行が100行そろった時点の performance.now()（このページを開き始めてからの経過ミリ秒）を、描画のフレームごとに確かめる。
    const shownAt = await measuredPage.waitForFunction(
      (rowCount) => document.querySelectorAll('tbody tr').length >= rowCount && performance.now(),
      codes.length,
      { polling: 'raf' },
    )
    const elapsedMs = await shownAt.jsonValue()

    // 補助 NFR-3（PR④ の TC-04）: 計測値と 2000ms までの余裕が後から分かるよう、判定の前に annotation に残す（失敗したときも残る）。
    const measuredMs = Number(elapsedMs)
    test.info().annotations.push({
      type: 'AC-18 計測値',
      description: `${measuredMs.toFixed(1)}ms（上限 2000ms、余裕 ${(2000 - measuredMs).toFixed(1)}ms）`,
    })

    expect(elapsedMs).toBeLessThanOrEqual(2000)
    // 出ていたのは入れた100銘柄の一覧（行が余分に出ていない）。
    await expect(measuredPage.locator('tbody tr')).toHaveCount(codes.length)
  } finally {
    await context.close()
  }
})

test('補助 FR-6（TC-13）: 価格のない 9984 だけを保有しているとき、合計評価額に 0円 と「価格を取得できない銘柄を含みません」が表示される', async ({ page }) => {
  // Given: すべての行に価格がない（プラン 4.4「合計の領域」: 0円 と注記を出す）。
  await insertHoldingsData({ holdings: [SOFTBANK_50] })

  await page.goto(HOLDINGS_PATH)

  const total = totalRegion(page)
  await expect(total).toContainText(amountPattern('0円'))
  await expect(total).toContainText(UNPRICED_NOTE)
})

test('補助 FR-22（TC-15）: 一覧の API への通信が失敗したとき、AC-9b と同じダイアログが表示され、閉じた後は一覧と合計評価額は表示されない', async ({ page }) => {
  await insertHoldingsData({ holdings: [TOYOTA_100], prices: [TOYOTA_PRICE] })
  // Given: 一覧の API への通信がエラーになる（応答が返らない）。
  await page.route(HOLDINGS_API, (route) => route.abort())

  await page.goto(HOLDINGS_PATH)

  await expectFetchErrorDialogAndNothingAfterClose(page)
})

test('補助 FR-21（SR-15、TC-32）: 一度表示した後にダッシュボードから開き直すと、キャッシュを出さずに「読み込み中」から始まる', async ({ page }) => {
  await insertHoldingsData({ holdings: [TOYOTA_100], prices: [TOYOTA_PRICE] })

  // 1回目: ダッシュボードからリンクで /holdings を開き、表が出るのを待つ。
  await page.goto(DASHBOARD_PATH)
  await holdingsLink(page).click()
  await expect(holdingsTable(page)).toBeVisible()

  // ダッシュボードに戻る。
  await page.goBack()
  await expect(page.getByRole('heading', { name: 'ダッシュボード', exact: true })).toBeVisible()

  // 2回目: 一覧の API の応答を保留してからリンクを押す（取得が終わらない間の表示を確かめる）。
  let releaseResponse: () => void = () => {}
  const responseReleased = new Promise<void>((resolve) => {
    releaseResponse = resolve
  })
  await page.route(HOLDINGS_API, async (route) => {
    await responseReleased
    await route.continue()
  })
  try {
    await holdingsLink(page).click()

    // 「読み込み中」のアイコンが出ていて（取得中）、1回目に取得した一覧（キャッシュ）は出ていない（プラン 4.4 の gcTime: 0）。
    await expect(loadingStatus(page)).toBeVisible()
    await expect(holdingsTable(page)).toHaveCount(0)
  } finally {
    // 保留を解いて、API の応答をそのまま返す。
    releaseResponse()
  }

  // 応答が返ると一覧が出て、「読み込み中」のアイコンは消える。
  await expect(holdingRow(page, '7203').getByRole('cell')).toHaveText(TOYOTA_ROW)
  await expect(loadingStatus(page)).toHaveCount(0)
})

test('補助 FR-5（PR4_TC03）: 9984・7203・6758 の順に保有データを入れても、表は銘柄コードの昇順（6758、7203、9984）で表示される', async ({ page }) => {
  // Given: 保有データを 9984 → 7203 → 6758 の順に入れる（入れた順が昇順と逆になるようにする）。
  // 価格はどの銘柄にも入れない（価格の有無で並びが変わる誤りと見分けがつかなくならないよう、条件をそろえる）。
  await insertHoldingsData({ holdings: [SOFTBANK_50, TOYOTA_100, SONY_10] })

  await page.goto(HOLDINGS_PATH)

  // 表の1列目（銘柄コード。プラン 4.4「表」: データ行は <tbody> の中に <td> を7つ）が昇順。
  await expect(
    holdingsTable(page).locator('tbody tr > td:first-child'),
  ).toHaveText(['6758', '7203', '9984'])
})

test('補助 FR-22（PR4_SR1・PR4_EC1）: ブラウザがオフラインと判定しているときにダッシュボードからリンクで開くと、読み込み中のまま止まらずに取得失敗のダイアログが表示される', async ({ page }) => {
  // DB には保有データを入れておく（「表示されない」が空のデータのせいでないようにする）。
  await insertHoldingsData({ holdings: [TOYOTA_100], prices: [TOYOTA_PRICE] })

  // ダッシュボードを開き、画面が出てから（読み込みが終わってから）オフラインにする。
  await page.goto(DASHBOARD_PATH)
  await expect(holdingsLink(page)).toBeVisible()

  const context = page.context()
  await context.setOffline(true)
  try {
    // Given の確認: ブラウザがオフラインと判定している。
    await expect.poll(() => page.evaluate(() => navigator.onLine)).toBe(false)

    await holdingsLink(page).click()
    await expect(page).toHaveURL(/\/holdings$/)

    // 取得を試みて通信できないので、取得失敗のダイアログが出る（プラン 4.4 の networkMode: 'always'、判断22）。
    const dialog = errorDialog(page)
    await expect(dialog).toBeVisible()
    await expect(dialog).toContainText(FETCH_ERROR_MESSAGE)
    // 読み込み中のまま止まっていない。一覧と合計評価額も出ていない。
    await expect(loadingStatus(page)).toHaveCount(0)
    await expect(holdingsTable(page)).toHaveCount(0)
    await expect(totalRegion(page)).toHaveCount(0)
  } finally {
    // 後続のテストに影響しないよう、オンラインに戻す。
    await context.setOffline(false)
  }
})

/** 一覧の API を HTTP 500（本文はプラン 4.4「API」の失敗時の形）にして /holdings を開き、取得失敗のダイアログが出るのを待つ。 */
async function openHoldingsWithServerError(page: Page): Promise<Locator> {
  // DB には保有データを入れておく（「表示されない」が空のデータのせいでないようにする）。
  await insertHoldingsData({ holdings: [TOYOTA_100], prices: [TOYOTA_PRICE] })
  await page.route(HOLDINGS_API, (route) =>
    route.fulfill({
      status: 500,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'internal_error' }),
    }),
  )

  await page.goto(HOLDINGS_PATH)

  const dialog = errorDialog(page)
  await expect(dialog).toBeVisible()
  await expect(dialog).toContainText(FETCH_ERROR_MESSAGE)
  return dialog
}

test('補助 FR-22（PR4_EC5・PR4_D5b）: 取得失敗のダイアログは Esc でも閉じ、閉じた後は見出しだけが表示されてフォーカスが見出しにある', async ({ page }) => {
  const dialog = await openHoldingsWithServerError(page)

  await page.keyboard.press('Escape')

  // ダイアログが消え、見出しだけになる（プラン 4.4 の状態の表「取得失敗でダイアログを閉じた後」）。
  await expect(dialog).toHaveCount(0)
  await expect(holdingsHeading(page)).toBeVisible()
  await expect(holdingsTable(page)).toHaveCount(0)
  await expect(totalRegion(page)).toHaveCount(0)
  // 閉じた後のフォーカスは見出し（プラン 4.4 の HoldingsPage。body に落ちない）。
  await expect(holdingsHeading(page)).toBeFocused()
})

test('補助 FR-22（PR4_D5b）: 取得失敗のダイアログを「閉じる」ボタンで閉じたときも、フォーカスが見出しにある', async ({ page }) => {
  const dialog = await openHoldingsWithServerError(page)

  await dialog.getByRole('button', { name: CLOSE_BUTTON, exact: true }).click()

  await expect(dialog).toHaveCount(0)
  await expect(holdingsHeading(page)).toBeVisible()
  await expect(holdingsHeading(page)).toBeFocused()
})

test('補助 判断21・4.5 の CORS（PR4_S1）: 別の localhost のオリジン（http://localhost:3000）を付けて dev サーバーの /api/holdings を要求しても、応答に access-control-allow-origin が付かない', async ({ request }) => {
  // baseURL（フロントの dev サーバー）の /api/holdings に、別のオリジンからの要求として Origin を付けて送る。
  const response = await request.get('/api/holdings', {
    headers: { Origin: 'http://localhost:3000' },
  })

  // 先に、要求が dev サーバーの /api の転送を通って API に届いたこと（200。プラン 4.4「API」）を確かめる。
  expect(response.status()).toBe(200)
  // 別のオリジンに読むことを許すヘッダーが付かない（プラン 4.5 の server.cors: false、判断21）。
  expect(response.headers()['access-control-allow-origin']).toBeUndefined()
})
