import { expect, test, type Locator, type Page } from '@playwright/test'
import {
  insertHoldingsData,
  resetHoldingsData,
  type HoldingSeed,
  type StockPriceSeed,
} from '../fixtures/db'

// 仕様: 要件 docs/requirements/price-update.md（値段の更新）の E2E の基準と、補助のテスト。
// テスト計画: プラン docs/plans/price-update/plan.md 5.1・5.2 の E2E の行（このファイルは PR① の分: 今すぐ更新）。
// 画面の文言と構造: プラン 4.4「画面」の今すぐ更新（合計の領域の直後の <section aria-label="価格の更新">、
//   結果は role="status" の中に1文ずつ <p>、要求の失敗は role="alert"）。
// 実行の前提: プラン 4.5 と前のプラン（docs/plans/stock-holdings/plan.md）4.5「E2E」の決まり
//   （zero_e2e、API は利用者A で起動、beforeEach でデータを入れ替える、順に実行、America/Los_Angeles、「表示されない」の確かめ方）。
//   今すぐ更新は、E2E 用の API のダミーの取得元をそのまま使う（7203 と 6758 は 2026-10-06 の終値、それ以外は取得できない）。
// mock は境界の外側（ブラウザから見たネットワーク = page.route）にだけ使う。

// 1件失敗しても後続を止めずに、順に実行する（同じ DB を使うため）。
test.describe.configure({ mode: 'default' })

// 前の機能の E2E と同じく、UTC より遅いタイムゾーンで実行する（価格の基準日が Date に変換されてずれないことも、あわせて確かめられる）。
const BROWSER_TIMEZONE = 'America/Los_Angeles'
test.use({ timezoneId: BROWSER_TIMEZONE })

// 各テストの前に、保有データ・終値と test.sql にない銘柄マスタを消す。そのテストのデータは、各テストの中で画面を開く前に入れる。
test.beforeEach(async () => {
  await resetHoldingsData()
})

const HOLDINGS_PATH = '/holdings'
/** 今すぐ更新の API（プラン 4.4「API」）。ブラウザからは Vite の proxy 経由で同じオリジンの /api/price-refresh を呼ぶ。 */
const PRICE_REFRESH_API = '**/api/price-refresh'

// ---- 画面の文言（要件16〜19、プラン 4.4） ----
const PAGE_TITLE = '保有株式一覧'
const TOTAL_LABEL = '合計評価額'
const EMPTY_MESSAGE = '保有株式はありません'
const UNAVAILABLE = '取得できません'
const REFRESH_REGION = '価格の更新'
const REFRESH_BUTTON = '価格を今すぐ更新'
const REFRESH_SAVED_MESSAGE = '1銘柄の価格を更新しました'
const REFRESH_FAILED_MESSAGE = '価格を取得できなかった銘柄: 9984'
const REFRESH_REQUEST_ERROR_MESSAGE = '価格の更新に失敗しました'

// ---- テストデータ（要件定義の成功基準の銘柄マスタとダミー価格） ----
/** 7203 を100株、取得価格2,500円（基準25 の「3,000円 / 2026-10-06 / 300,000円」の評価額 300,000円 は100株の値）。 */
const TOYOTA_100: HoldingSeed = { code: '7203', quantity: 100, acquisitionPrice: '2500.0' }
/** 9984 を50株（数量・取得価格は基準にないので任意の値）。9984 はダミーの取得元に定義がなく、「取得できない」になる。 */
const SOFTBANK_50: HoldingSeed = { code: '9984', quantity: 50, acquisitionPrice: '4500.0' }
/** 7203 の保存済みの終値（3,000円・2026-10-06）。 */
const TOYOTA_PRICE: StockPriceSeed = { code: '7203', tradingDate: '2026-10-06', closePrice: '3000.0' }

// ---- 画面の要素 ----
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

/**
 * 行のデータの7セル（銘柄コード・銘柄名・保有数量・取得価格・現在の価格・価格の基準日・評価額）。
 * 後の PR で行の右端に「⋯」の列が足されても、同じ確かめが使えるよう、先頭の7つのセルだけを選ぶ。
 */
function dataCells(row: Locator): Locator {
  return row.locator('td:nth-child(-n+7)')
}

/** 今すぐ更新の領域（<section aria-label="価格の更新">）。 */
function refreshRegion(page: Page): Locator {
  return page.getByRole('region', { name: REFRESH_REGION, exact: true })
}

function refreshButton(page: Page): Locator {
  return page.getByRole('button', { name: REFRESH_BUTTON, exact: true })
}

/**
 * 金額の表示（例 "300,000円"）が、前に数字や「,」が続かない形で現れることを表す正規表現。
 * "0円" が "300,000円" の一部に当たらないようにする。
 */
function amountPattern(amount: string): RegExp {
  const escaped = amount.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return new RegExp(`(^|[^0-9,])${escaped}`)
}

test('基準1（今すぐ更新の部分・要件1）: 保有株式が1件もないとき、「保有株式はありません」が表示され、「価格を今すぐ更新」は表示されない', async ({ page }) => {
  // Given: 利用者Aの保有データなし（beforeEach で消したまま）。

  await page.goto(HOLDINGS_PATH)

  // 先に空のメッセージが出る（取得が終わった）のを待ってから、ないことを確かめる（前のプランの E2E の決まり）。
  await expect(page.getByText(EMPTY_MESSAGE)).toBeVisible()
  await expect(refreshButton(page)).toHaveCount(0)
  await expect(refreshRegion(page)).toHaveCount(0)
})

test('基準25（要件16・要件17・要件18）: 7203 と 9984 を保有し（価格は未保存）、「価格を今すぐ更新」を押すと、7203 の行に「3,000円 / 2026-10-06 / 300,000円」が表示され、「1銘柄の価格を更新しました」と「価格を取得できなかった銘柄: 9984」が表示され、ボタンは押せる状態に戻る', async ({ page }) => {
  // Given: 利用者Aが 7203 と 9984 を保有し、どちらも価格は未保存。
  await insertHoldingsData({ holdings: [TOYOTA_100, SOFTBANK_50] })

  await page.goto(HOLDINGS_PATH)

  // Given の確認: 更新の前は、7203 の現在の価格・価格の基準日・評価額は「取得できません」。
  await expect(dataCells(holdingRow(page, '7203'))).toHaveText([
    '7203',
    'トヨタ自動車',
    '100株',
    '2,500円',
    UNAVAILABLE,
    UNAVAILABLE,
    UNAVAILABLE,
  ])

  // When: 「価格を今すぐ更新」を押す。
  await refreshButton(page).click()

  // Then: 7203 の行に「3,000円 / 2026-10-06 / 300,000円」が表示される（一覧を取得し直した結果。基準日は America/Los_Angeles でもずれない）。
  await expect(dataCells(holdingRow(page, '7203'))).toHaveText([
    '7203',
    'トヨタ自動車',
    '100株',
    '2,500円',
    '3,000円',
    '2026-10-06',
    '300,000円',
  ])
  // 合計評価額も取得し直されている（要件18）。9984 は価格がないままなので、合計には含めない。
  await expect(totalRegion(page)).toContainText(amountPattern('300,000円'))
  // 9984 の行は取得できなかったので、価格のないまま。
  await expect(dataCells(holdingRow(page, '9984'))).toHaveText([
    '9984',
    'ソフトバンクグループ',
    '50株',
    '4,500円',
    UNAVAILABLE,
    UNAVAILABLE,
    UNAVAILABLE,
  ])
  // 結果の文言は、ボタンの近く（今すぐ更新の領域の中。role="status" に1文ずつ）に表示される。
  const status = refreshRegion(page).getByRole('status')
  await expect(status.locator('p')).toHaveText([REFRESH_SAVED_MESSAGE, REFRESH_FAILED_MESSAGE])
  // 終わった後、ボタンは押せる「価格を今すぐ更新」に戻っている（要件17）。「更新中…」のボタンは残っていない。
  await expect(refreshRegion(page).getByRole('button', { name: REFRESH_BUTTON, exact: true })).toBeEnabled()
  await expect(page.getByRole('button', { name: '更新中…', exact: true })).toHaveCount(0)
})

test('基準30（要件19）: 今すぐ更新の API が HTTP 500 を返すとき、一覧と合計評価額は表示されたまま、「価格の更新に失敗しました」が表示される', async ({ page }) => {
  // 価格の保存済みの 7203 を保有（一覧と合計評価額が出るデータにして、「表示されたまま」を確かめられるようにする）。
  await insertHoldingsData({ holdings: [TOYOTA_100], prices: [TOYOTA_PRICE] })
  // Given: 今すぐ更新の API がエラー（HTTP 500。本文は 4.4「API」の失敗時の形）を返す（POST だけ。ほかの要求はそのまま通す）。
  await page.route(PRICE_REFRESH_API, (route) =>
    route.request().method() === 'POST'
      ? route.fulfill({
          status: 500,
          contentType: 'application/json',
          body: JSON.stringify({ error: 'internal_error' }),
        })
      : route.continue(),
  )
  await page.goto(HOLDINGS_PATH)
  await expect(holdingsTable(page)).toBeVisible()

  await refreshButton(page).click()

  await expectRefreshRequestFailedAndListStillShown(page)
})

test('補助 要件19（基準30 と同じ表示）: 今すぐ更新の API への通信が失敗したとき（abort）、一覧と合計評価額は表示されたまま、「価格の更新に失敗しました」が表示される', async ({ page }) => {
  await insertHoldingsData({ holdings: [TOYOTA_100], prices: [TOYOTA_PRICE] })
  // Given: 今すぐ更新の API への通信がエラーになる（応答が返らない）。POST だけ。ほかの要求はそのまま通す。
  await page.route(PRICE_REFRESH_API, (route) =>
    route.request().method() === 'POST' ? route.abort() : route.continue(),
  )
  await page.goto(HOLDINGS_PATH)
  await expect(holdingsTable(page)).toBeVisible()

  await refreshButton(page).click()

  await expectRefreshRequestFailedAndListStillShown(page)
})

/**
 * 今すぐ更新の要求が失敗したときの表示（要件19、基準30）を確かめる。
 * 「価格の更新に失敗しました」が role="alert" で出て、一覧（7203 の行）と合計評価額は表示されたまま、ボタンは押せる状態に戻る。
 */
async function expectRefreshRequestFailedAndListStillShown(page: Page): Promise<void> {
  // 「価格の更新に失敗しました」が、ボタンの近く（今すぐ更新の領域の中）に表示される。
  const alert = refreshRegion(page).getByRole('alert')
  await expect(alert).toHaveText(REFRESH_REQUEST_ERROR_MESSAGE)
  // 失敗の応答が返った（要求が終わった）後も、一覧と合計評価額は表示されたまま。
  await expect(holdingsHeading(page)).toBeVisible()
  await expect(dataCells(holdingRow(page, '7203'))).toHaveText([
    '7203',
    'トヨタ自動車',
    '100株',
    '2,500円',
    '3,000円',
    '2026-10-06',
    '300,000円',
  ])
  await expect(totalRegion(page)).toContainText(amountPattern('300,000円'))
  // 成功の結果の文言（「n銘柄の価格を更新しました」など）は出ていない。
  await expect(page.getByText('更新しました')).toHaveCount(0)
  // 失敗した後、ボタンは押せる状態に戻っている（押し直せる。要件17）。
  await expect(refreshRegion(page).getByRole('button', { name: REFRESH_BUTTON, exact: true })).toBeEnabled()
}

test('補助・判断31・要件17・要件18（キーボード操作のフォーカス）: ボタン「価格を今すぐ更新」にフォーカスを当てて Enter で押し、結果の文言が出てボタンが押せる状態に戻った後も、フォーカスはそのボタンにある', async ({ page }) => {
  // Given: 利用者Aが 7203 を保有（価格は未保存。ダミーの取得元は 7203 の価格を持つので、更新は成功する）。
  await insertHoldingsData({ holdings: [TOYOTA_100] })
  // 更新中の状態が1フレームより短いと、ボタンが disabled になってもフォーカスが外れず、フォーカスを戻す処理がなくても通ってしまう。
  // 応答を 300ms 遅らせて、更新中の状態を確実に作る（実際の API の応答はそのまま返す。このテストの中だけ）。
  await page.route(PRICE_REFRESH_API, async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 300))
    await route.continue()
  })
  await page.goto(HOLDINGS_PATH)
  const button = refreshButton(page)
  await expect(button).toBeVisible()

  // キーボードでボタンにフォーカスを当て、Enter で押す。
  await button.focus()
  await expect(button).toBeFocused()
  await page.keyboard.press('Enter')

  // 結果の文言が出て（更新と一覧の取り直しが終わった）、ボタンが押せる状態に戻る。
  await expect(refreshRegion(page).getByRole('status').locator('p')).toHaveText([REFRESH_SAVED_MESSAGE])
  await expect(button).toBeEnabled()
  // 更新中はボタンを押せなくなるので、そのままではフォーカスが画面から落ちる。終わったら、キーボードの利用者が続けて操作できるよう、ボタンに戻っている。
  await expect(button).toBeFocused()
})

test('補助・判断31・要件19（キーボード操作のフォーカス・失敗のとき）: ボタン「価格を今すぐ更新」にフォーカスを当てて Enter で押し、要求が失敗して「価格の更新に失敗しました」が出てボタンが押せる状態に戻った後も、フォーカスはそのボタンにある', async ({ page }) => {
  // Given: 利用者Aが 7203 を保有し、価格も保存済み（一覧が出るデータ）。今すぐ更新の API は 300ms 後に HTTP 500 を返す
  // （更新中の状態を確実に作るための遅延。成功のときのテストと同じ理由）。
  await insertHoldingsData({ holdings: [TOYOTA_100], prices: [TOYOTA_PRICE] })
  await page.route(PRICE_REFRESH_API, async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 300))
    await route.fulfill({
      status: 500,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'internal_error' }),
    })
  })
  await page.goto(HOLDINGS_PATH)
  const button = refreshButton(page)
  await expect(button).toBeVisible()

  // キーボードでボタンにフォーカスを当て、Enter で押す。
  await button.focus()
  await expect(button).toBeFocused()
  await page.keyboard.press('Enter')

  // 失敗の文言が出て、ボタンが押せる状態に戻る。
  await expect(refreshRegion(page).getByRole('alert')).toHaveText(REFRESH_REQUEST_ERROR_MESSAGE)
  await expect(button).toBeEnabled()
  // 失敗のときも、キーボードの利用者が続けて操作（押し直し）できるよう、ボタンにフォーカスが戻っている。
  await expect(button).toBeFocused()
})

test('補助・判断31（更新中に移したフォーカスは取り上げない）: Enter で押した後、更新中に Tab で表の横スクロールの領域へフォーカスを移すと、更新が終わって結果の文言が出た後も、フォーカスは表の領域にあり、ボタンに奪い返されない', async ({ page }) => {
  // Given: 利用者Aが 7203 を保有。今すぐ更新の API は 1500ms 後に、実際の応答をそのまま返す
  // （更新中に Tab を押して確かめる間、更新中の状態を保つための長めの遅延）。
  await insertHoldingsData({ holdings: [TOYOTA_100] })
  await page.route(PRICE_REFRESH_API, async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 1500))
    await route.continue()
  })
  await page.goto(HOLDINGS_PATH)
  const button = refreshButton(page)
  await expect(button).toBeVisible()
  // 表の横スクロールの領域（role="region"、名前は見出し「保有株式一覧」。Tab で止まれる）。
  const tableRegion = page.getByRole('region', { name: PAGE_TITLE, exact: true })

  // キーボードでボタンを押す。更新中になる（ボタンは押せなくなり、文言は「更新中…」）。
  await button.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('button', { name: '更新中…', exact: true })).toBeDisabled()

  // 更新中に Tab で、次に止まれる表の領域へフォーカスを移す。
  await page.keyboard.press('Tab')
  await expect(tableRegion).toBeFocused()

  // 更新が終わり（結果の文言が出て、ボタンが押せる状態に戻る）ても、フォーカスは表の領域のまま。ボタンに取り戻さない。
  await expect(refreshRegion(page).getByRole('status').locator('p')).toHaveText([REFRESH_SAVED_MESSAGE])
  await expect(button).toBeEnabled()
  await expect(tableRegion).toBeFocused()
  await expect(button).not.toBeFocused()
})
