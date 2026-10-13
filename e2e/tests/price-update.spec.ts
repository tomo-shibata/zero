import { expect, test, type Locator, type Page } from '@playwright/test'
import {
  insertHoldingsData,
  resetHoldingsData,
  type HoldingSeed,
  type StockPriceSeed,
} from '../fixtures/db'

// 仕様: 要件 docs/requirements/price-update.md（値段の更新）の E2E の基準と、補助のテスト。
// テスト計画: プラン docs/plans/price-update/plan.md 5.1・5.2 の E2E の行（このファイルは PR① の分: 今すぐ更新、PR② の分: 「⋯」のメニューと銘柄の削除）。
// 画面の文言と構造: プラン 4.4「画面」の今すぐ更新（合計の領域の直後の <section aria-label="価格の更新">、
//   結果は role="status" の中に1文ずつ <p>、要求の失敗は role="alert"）。
//   PR② は 4.4「画面」の表の8列目（MenuButton。アクセシブルな名前は「<銘柄名>の操作」、項目は role="menuitem"）と削除の確認
//   （ConfirmDialog。role="alertdialog"、名前は文言そのもの、「キャンセル」「削除する」の順、失敗は role="alert" で「削除できませんでした」）、閉じた後のフォーカス。
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

// ---- 画面の文言（要件2・要件13〜15、プラン 4.4） ----
const DELETE_MENU_ITEM = '削除'
const DELETE_CONFIRM_BUTTON = '削除する'
const DELETE_CANCEL_BUTTON = 'キャンセル'
const DELETE_CONFIRM_MESSAGE =
  '7203 トヨタ自動車を削除しますか？この銘柄の保有データはすべて削除されます。'
const DELETE_ERROR_MESSAGE = '削除できませんでした'
/** 銘柄の削除の API（プラン 4.4「API」の DELETE /api/holdings/{code}）。 */
const HOLDING_DELETE_API = '**/api/holdings/*'

// ---- テストデータ（要件定義の成功基準の銘柄マスタとダミー価格） ----
/** 7203 を100株、取得価格2,500円（基準25 の「3,000円 / 2026-10-06 / 300,000円」の評価額 300,000円 は100株の値）。 */
const TOYOTA_100: HoldingSeed = { code: '7203', quantity: 100, acquisitionPrice: '2500.0' }
/** 9984 を50株（数量・取得価格は基準にないので任意の値）。9984 はダミーの取得元に定義がなく、「取得できない」になる。 */
const SOFTBANK_50: HoldingSeed = { code: '9984', quantity: 50, acquisitionPrice: '4500.0' }
/** 7203 の1件目（100株・2,000円）。基準21 の「7203 を2件」の1件目。 */
const TOYOTA_FIRST: HoldingSeed = { code: '7203', quantity: 100, acquisitionPrice: '2000.0' }
/** 7203 の2件目（300株・3,000円）。1件目と合わせて 400株・取得価格 2,750円（(100×2,000＋300×3,000)÷400。基準22 の「400株」）。 */
const TOYOTA_SECOND: HoldingSeed = { code: '7203', quantity: 300, acquisitionPrice: '3000.0' }
/** 6758 を50株（数量・取得価格は基準にないので任意の値）。価格は入れない。 */
const SONY_50: HoldingSeed = { code: '6758', quantity: 50, acquisitionPrice: '3200.0' }
/** 6758 の保存済みの終値（3,500円・2026-10-06）。 */
const SONY_PRICE: StockPriceSeed = { code: '6758', tradingDate: '2026-10-06', closePrice: '3500.0' }
/** 9984 を100株・取得価格4,500円（要件定義の「画面のイメージ」の 9984 の行。価格は入れない）。 */
const SOFTBANK_100: HoldingSeed = { code: '9984', quantity: 100, acquisitionPrice: '4500.0' }
/** 長い銘柄名の銘柄（銘柄マスタに足す。判断39 の折り返しを見分けるため）。 */
const LONG_NAME_CODE = '8306'
const LONG_NAME = '三菱ＵＦＪフィナンシャル・グループ'
/** 長いカタカナの名前の銘柄（銘柄マスタに足す。文節の間でしか折り返せない名前でも、表が領域の幅を超えないことを見分けるため。判断39）。 */
const KATAKANA_NAME_CODE = '6752'
const KATAKANA_NAME = 'パナソニックホールディングス'
/** 大きな数量の銘柄（銘柄マスタに足す。判断39 の幅の確かめで、10,000株・12,345.6円の行に使う）。 */
const BIG_QUANTITY_CODE = '4063'
const BIG_QUANTITY_NAME = '信越化学工業'
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

/** 行の「⋯」のボタン。アクセシブルな名前は「<銘柄名>の操作」（要件2）。 */
function operationButton(page: Page, name: string): Locator {
  return page.getByRole('button', { name: `${name}の操作`, exact: true })
}

/** 銘柄の削除の確認のダイアログ（role="alertdialog"。名前は文言そのもの。プラン 4.4「画面」）。 */
function deleteConfirmDialog(page: Page): Locator {
  return page.getByRole('alertdialog', { name: DELETE_CONFIRM_MESSAGE, exact: true })
}

/** 銘柄の行の「⋯」→「削除」を選んで、削除の確認のダイアログが開くのを待つ。 */
async function openDeleteConfirm(page: Page, name: string): Promise<Locator> {
  await operationButton(page, name).click()
  await page.getByRole('menuitem', { name: DELETE_MENU_ITEM, exact: true }).click()
  const dialog = deleteConfirmDialog(page)
  await expect(dialog).toBeVisible()
  return dialog
}

/** 7203 の2件（400株・2,750円）と終値3,000円・2026-10-06 をまとめた行のデータの7セル。 */
const TOYOTA_400_PRICED_ROW = ['7203', 'トヨタ自動車', '400株', '2,750円', '3,000円', '2026-10-06', '1,200,000円']
/** 6758 の50株・3,200円と終値3,500円・2026-10-06 の行のデータの7セル。 */
const SONY_50_PRICED_ROW = ['6758', 'ソニーグループ', '50株', '3,200円', '3,500円', '2026-10-06', '175,000円']

/** 7203 の行のデータの7セルが、7203 の2件（400株・2,750円。価格は未保存）をまとめた値であること。 */
async function expectToyotaRowMerged(page: Page): Promise<void> {
  await expect(dataCells(holdingRow(page, '7203'))).toHaveText([
    '7203',
    'トヨタ自動車',
    '400株',
    '2,750円',
    UNAVAILABLE,
    UNAVAILABLE,
    UNAVAILABLE,
  ])
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

test('基準1（要件1）: 保有株式が1件もないとき、「保有株式はありません」が表示され、「価格を今すぐ更新」と「⋯」はどちらも表示されない', async ({ page }) => {
  // Given: 利用者Aの保有データなし（beforeEach で消したまま）。

  await page.goto(HOLDINGS_PATH)

  // 先に空のメッセージが出る（取得が終わった）のを待ってから、ないことを確かめる（前のプランの E2E の決まり）。
  await expect(page.getByText(EMPTY_MESSAGE)).toBeVisible()
  await expect(refreshButton(page)).toHaveCount(0)
  await expect(refreshRegion(page)).toHaveCount(0)
  // PR②: 「⋯」（アクセシブルな名前は「<銘柄名>の操作」）もない。見える文字の「⋯」もない。
  await expect(page.getByRole('button', { name: /の操作$/ })).toHaveCount(0)
  await expect(page.getByText('⋯')).toHaveCount(0)
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

// ======================================================================
// PR②: 「⋯」のメニューと銘柄の削除
// ======================================================================

test('補助 要件2（PR② の分。PR③ で基準2 に置き換える）: 6758・7203・9984 を保有し、真ん中の 7203 の「⋯」を押すとメニューが開いて項目は「削除」だけ。Esc でメニューが閉じ、フォーカスが「トヨタ自動車の操作」にある', async ({ page }) => {
  // Given: 利用者Aが 6758・7203・9984 の3行を保有（真ん中の 7203 から開く。先頭や最後の行にフォーカスを戻す誤りを見分けるため。プラン 5章）。
  await insertHoldingsData({ holdings: [SONY_50, TOYOTA_FIRST, SOFTBANK_50] })
  await page.goto(HOLDINGS_PATH)
  const button = operationButton(page, 'トヨタ自動車')
  await expect(button).toBeVisible()
  // 「⋯」は3行にそれぞれある（アクセシブルな名前は銘柄ごと）。
  await expect(page.getByRole('button', { name: /の操作$/ })).toHaveText(['⋯', '⋯', '⋯'])
  await expect(button).toHaveAttribute('aria-expanded', 'false')
  await expect(page.getByRole('menu')).toHaveCount(0)

  // When: 7203 の行の「⋯」を押す。
  await button.click()

  // Then: メニューが開き、項目は「削除」だけ（PR③ で「編集」が先頭に足される）。
  await expect(button).toHaveAttribute('aria-expanded', 'true')
  await expect(page.getByRole('menu')).toBeVisible()
  await expect(page.getByRole('menuitem')).toHaveText([DELETE_MENU_ITEM])

  // When: Esc を押す。
  await page.keyboard.press('Escape')

  // Then: メニューが閉じ、フォーカスが 7203 の行の「⋯」（「トヨタ自動車の操作」）にある。
  await expect(page.getByRole('menu')).toHaveCount(0)
  await expect(button).toHaveAttribute('aria-expanded', 'false')
  await expect(button).toBeFocused()
  // 削除の確認は開いていない（メニューを閉じただけで何も起きない）。
  await expect(page.getByRole('alertdialog')).toHaveCount(0)
  await expect(holdingRow(page, '7203')).toHaveCount(1)
})

test('基準21（要件13・要件14）: 7203 を2件、6758 を1件保有し、7203 の「⋯」→「削除」を選ぶと確認のダイアログが開き、「削除する」を押すとダイアログが閉じ、一覧に 7203 の行はなく 6758 の行だけが表示され、フォーカスは見出しにある', async ({ page }) => {
  // Given: 利用者Aが 7203 を2件（100株・2,000円、300株・3,000円）、6758 を1件（50株・3,200円）保有。
  //   7203 の終値は3,000円・2026-10-06、6758 の終値は3,500円・2026-10-06（合計評価額が取り直されることを確かめるため。
  //   400株×3,000円＋50株×3,500円＝1,375,000円）。
  await insertHoldingsData({
    holdings: [TOYOTA_FIRST, TOYOTA_SECOND, SONY_50],
    prices: [TOYOTA_PRICE, SONY_PRICE],
  })
  await page.goto(HOLDINGS_PATH)
  await expect(dataCells(holdingRow(page, '7203'))).toHaveText(TOYOTA_400_PRICED_ROW)
  await expect(totalRegion(page)).toContainText(amountPattern('1,375,000円'))

  // When: 7203 の「⋯」→「削除」を選ぶ。
  const dialog = await openDeleteConfirm(page, 'トヨタ自動車')

  // Then: 「7203 トヨタ自動車を削除しますか？この銘柄の保有データはすべて削除されます。」の確認のダイアログが開く。
  //   ボタンは「キャンセル」「削除する」の順（判断33）。まだ何も消えていない（7203 の行は残っている）。
  await expect(dialog).toContainText(DELETE_CONFIRM_MESSAGE)
  await expect(dialog.getByRole('button')).toHaveText([DELETE_CANCEL_BUTTON, DELETE_CONFIRM_BUTTON])
  await expect(holdingRow(page, '7203')).toHaveCount(1)

  // When: 「削除する」を押す。
  await dialog.getByRole('button', { name: DELETE_CONFIRM_BUTTON, exact: true }).click()

  // Then: ダイアログが閉じ、一覧に 7203 の行はなく、6758 の行だけが表示される（一覧を取得し直した結果）。
  await expect(dialog).toHaveCount(0)
  await expect(holdingRow(page, '7203')).toHaveCount(0)
  await expect(page.locator('tbody tr')).toHaveCount(1)
  await expect(dataCells(holdingRow(page, '6758'))).toHaveText(SONY_50_PRICED_ROW)
  // 合計評価額も取り直されている（7203 の 1,200,000円 が抜けて 175,000円 になる。1,375,000円 は残っていない）。
  await expect(totalRegion(page)).toContainText(amountPattern('175,000円'))
  await expect(totalRegion(page)).not.toContainText('1,375,000円')
  // 開いた行がなくなったので、フォーカスは画面の見出しに移る（プラン 4.4「閉じた後のフォーカス」）。
  await expect(holdingsHeading(page)).toBeFocused()

  // 画面の表示だけでなく、サーバーの保有データが消えている（開き直しても 7203 の行はない）。
  await page.reload()
  await expect(dataCells(holdingRow(page, '6758'))).toHaveText(SONY_50_PRICED_ROW)
  await expect(totalRegion(page)).toContainText(amountPattern('175,000円'))
  await expect(holdingRow(page, '7203')).toHaveCount(0)
  await expect(page.locator('tbody tr')).toHaveCount(1)
})

test('基準22（要件13）: 6758・7203（2件）・9984 を保有し、確認のダイアログで「キャンセル」を押すとダイアログが閉じ、7203 の行は「400株」のまま表示され、フォーカスは「トヨタ自動車の操作」に戻り、開き直しても 7203 は「400株」のままである', async ({ page }) => {
  // Given: 基準21 と同じ 7203 の2件（400株）に、6758 と 9984 を足した3行（真ん中の 7203 から開き、フォーカスの戻り先を見分けるため。プラン 5章）。
  await insertHoldingsData({ holdings: [SONY_50, TOYOTA_FIRST, TOYOTA_SECOND, SOFTBANK_50] })
  await page.goto(HOLDINGS_PATH)
  await expectToyotaRowMerged(page)
  const dialog = await openDeleteConfirm(page, 'トヨタ自動車')

  // When: 確認のダイアログで「キャンセル」を押す。
  await dialog.getByRole('button', { name: DELETE_CANCEL_BUTTON, exact: true }).click()

  // Then: ダイアログが閉じ、7203 の行は「400株」のまま表示されている。
  await expect(dialog).toHaveCount(0)
  await expectToyotaRowMerged(page)
  await expect(page.locator('tbody tr')).toHaveCount(3)
  // 閉じた後のフォーカスは、開いた行の「⋯」（先頭の 6758 や最後の 9984 ではない。プラン 4.4「閉じた後のフォーカス」）。
  await expect(operationButton(page, 'トヨタ自動車')).toBeFocused()

  // 裏で消していない（開き直しても 7203 の行は「400株」のまま）。
  await page.reload()
  await expectToyotaRowMerged(page)
  await expect(page.locator('tbody tr')).toHaveCount(3)
})

test('補助 判断22・要件13（基準22 の補足）: 確認のダイアログは Esc でもキャンセルと同じに閉じ（何も消えない）、フォーカスは「トヨタ自動車の操作」に戻る', async ({ page }) => {
  // Given: 基準22 と同じ3行（7203 は2件）。
  await insertHoldingsData({ holdings: [SONY_50, TOYOTA_FIRST, TOYOTA_SECOND, SOFTBANK_50] })
  await page.goto(HOLDINGS_PATH)
  const dialog = await openDeleteConfirm(page, 'トヨタ自動車')

  await page.keyboard.press('Escape')

  await expect(dialog).toHaveCount(0)
  await expectToyotaRowMerged(page)
  await expect(operationButton(page, 'トヨタ自動車')).toBeFocused()
  await page.reload()
  await expectToyotaRowMerged(page)
})

test('補助 判断33（基準21 の補足）: 確認のダイアログを開いた直後は、フォーカスが「キャンセル」にある。そのまま Enter を押しても何も消えず、ダイアログが閉じて 7203 の行は「400株」のままである', async ({ page }) => {
  // Given: 7203 を2件、6758 と 9984 を保有（3行）。
  await insertHoldingsData({ holdings: [SONY_50, TOYOTA_FIRST, TOYOTA_SECOND, SOFTBANK_50] })
  await page.goto(HOLDINGS_PATH)
  const dialog = await openDeleteConfirm(page, 'トヨタ自動車')

  // 取り消せない操作は、安全な側（キャンセル）が最初のフォーカス。
  await expect(dialog.getByRole('button', { name: DELETE_CANCEL_BUTTON, exact: true })).toBeFocused()
  await expect(dialog.getByRole('button', { name: DELETE_CONFIRM_BUTTON, exact: true })).not.toBeFocused()

  // 開いた直後に Enter を押しても、削除されない（キャンセルが押される）。
  await page.keyboard.press('Enter')

  await expect(dialog).toHaveCount(0)
  await expectToyotaRowMerged(page)
  await page.reload()
  await expectToyotaRowMerged(page)
  await expect(page.locator('tbody tr')).toHaveCount(3)
})

test('補助 判断13（要件14）: 削除の要求が終わるまで「削除する」は押せず、押し直そうとしても削除の要求は1回だけ送られ、終わるとダイアログが閉じて 7203 の行がなくなる', async ({ page }) => {
  // Given: 7203（2件）と 6758 を保有。削除の API は 800ms 遅れて実際の応答を返す（削除中の状態を確実に作るため。このテストの中だけ）。
  await insertHoldingsData({ holdings: [TOYOTA_FIRST, TOYOTA_SECOND, SONY_50] })
  let deleteRequests = 0
  await page.route(HOLDING_DELETE_API, async (route) => {
    if (route.request().method() !== 'DELETE') {
      await route.continue()
      return
    }
    deleteRequests += 1
    await new Promise((resolve) => setTimeout(resolve, 800))
    await route.continue()
  })
  await page.goto(HOLDINGS_PATH)
  const dialog = await openDeleteConfirm(page, 'トヨタ自動車')
  const confirm = dialog.getByRole('button', { name: DELETE_CONFIRM_BUTTON, exact: true })

  await confirm.click()

  // 削除中は「削除する」を押せない。押そうとしても（force で押しても）、削除の要求は増えない。
  await expect(confirm).toBeDisabled()
  await confirm.click({ force: true })

  // 終わるとダイアログが閉じ、7203 の行がなくなる。削除の要求は1回だけ。
  await expect(dialog).toHaveCount(0)
  await expect(holdingRow(page, '7203')).toHaveCount(0)
  expect(deleteRequests).toBe(1)
})

/**
 * 銘柄の削除の要求が失敗したときの表示（要件15、基準24）を確かめる。
 * 確認のダイアログは開いたまま、その中に role="alert" で「削除できませんでした」が出て、一覧の 7203 の行は残る。
 */
async function expectDeleteFailedAndDialogStillOpen(page: Page, dialog: Locator): Promise<void> {
  await expect(dialog.getByRole('alert')).toHaveText(DELETE_ERROR_MESSAGE)
  // ダイアログは開いたまま（確認の文言も「キャンセル」「削除する」もそのまま）。
  await expect(dialog).toBeVisible()
  await expect(dialog).toContainText(DELETE_CONFIRM_MESSAGE)
  await expect(dialog.getByRole('button')).toHaveText([DELETE_CANCEL_BUTTON, DELETE_CONFIRM_BUTTON])
  // 一覧は変えない（7203 の行は残っている）。
  await expectToyotaRowMerged(page)
  await expect(page.locator('tbody tr')).toHaveCount(2)
}

test('基準24（要件15）: 銘柄の削除の API が HTTP 500 を返すとき、「削除する」を押してもダイアログは開いたまま「削除できませんでした」と表示され、一覧の 7203 の行は残っている', async ({ page }) => {
  // Given: 利用者Aが 7203（2件）と 6758 を保有。削除の API がエラー（HTTP 500。本文は 4.4「API」の失敗時の形）を返す（DELETE だけ。ほかの要求はそのまま通す）。
  await insertHoldingsData({ holdings: [TOYOTA_FIRST, TOYOTA_SECOND, SONY_50] })
  await page.route(HOLDING_DELETE_API, (route) =>
    route.request().method() === 'DELETE'
      ? route.fulfill({
          status: 500,
          contentType: 'application/json',
          body: JSON.stringify({ error: 'internal_error' }),
        })
      : route.continue(),
  )
  await page.goto(HOLDINGS_PATH)
  const dialog = await openDeleteConfirm(page, 'トヨタ自動車')

  // When: 確認のダイアログで「削除する」を押す。
  await dialog.getByRole('button', { name: DELETE_CONFIRM_BUTTON, exact: true }).click()

  await expectDeleteFailedAndDialogStillOpen(page, dialog)
})

test('補助 要件15（基準24 と同じ表示）: 銘柄の削除の API への通信が失敗したとき（abort）、ダイアログは開いたまま「削除できませんでした」と表示され、一覧の 7203 の行は残っている', async ({ page }) => {
  await insertHoldingsData({ holdings: [TOYOTA_FIRST, TOYOTA_SECOND, SONY_50] })
  // Given: 削除の API への通信がエラーになる（応答が返らない）。DELETE だけ。ほかの要求はそのまま通す。
  await page.route(HOLDING_DELETE_API, (route) =>
    route.request().method() === 'DELETE' ? route.abort() : route.continue(),
  )
  await page.goto(HOLDINGS_PATH)
  const dialog = await openDeleteConfirm(page, 'トヨタ自動車')

  await dialog.getByRole('button', { name: DELETE_CONFIRM_BUTTON, exact: true }).click()

  await expectDeleteFailedAndDialogStillOpen(page, dialog)
})

// ======================================================================
// PR② ステップ6 1ラウンド目の指摘で足したテスト
// ======================================================================

/**
 * page の、method の API の要求を、テストが release() を呼ぶまで止める（時間では止めない。待つ時間の長短でテストが揺れないため）。
 * release() の後、止めていた要求は実際のサーバーに通る（status を渡したときは、サーバーに通さず、その HTTP ステータスのエラーを返す）。
 * method 以外の要求は、そのまま通す。requests は、止めた要求の数。
 */
async function holdRequests(
  page: Page,
  urlPattern: string,
  method: string,
  failWithStatus?: number,
): Promise<{ release: () => void; requests: () => number }> {
  let release: () => void = () => {}
  const gate = new Promise<void>((resolve) => {
    release = resolve
  })
  let requests = 0
  await page.route(urlPattern, async (route) => {
    if (route.request().method() !== method) {
      await route.continue()
      return
    }
    requests += 1
    await gate
    if (failWithStatus === undefined) {
      await route.continue()
      return
    }
    await route.fulfill({
      status: failWithStatus,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'internal_error' }),
    })
  })
  return { release, requests: () => requests }
}

/**
 * 削除中（応答を止めている間）の確認のダイアログが、「キャンセル」を押せず、Esc を3回続けて押しても閉じず、7203 の行も残っていることを確かめる。
 * 間にクリックをはさまない（Esc を2回続けて押すと、ブラウザは2回目から cancel を止められない場合があるため。判断37）。
 */
async function expectDialogHeldOpenAgainstEscape(page: Page, dialog: Locator): Promise<void> {
  const cancel = dialog.getByRole('button', { name: DELETE_CANCEL_BUTTON, exact: true })
  await expect(cancel).toBeDisabled()
  await page.keyboard.press('Escape')
  await page.keyboard.press('Escape')
  await page.keyboard.press('Escape')
  await page.waitForTimeout(150)
  await expect(dialog).toBeVisible()
  await expect(cancel).toBeDisabled()
  // 応答が返る前は、7203 の行も残っている。
  await expect(holdingRow(page, '7203')).toHaveCount(1)
}

test('補助 判断37（要件13）: マウスで「削除する」を押した後、削除中は「キャンセル」が押せず、Esc を続けて3回押してもダイアログは閉じない。応答の後にダイアログが閉じ、7203 の行がなくなり、フォーカスは見出しにある', async ({ page }) => {
  // Given: 7203（2件）と 6758 を保有。削除の API の要求は、テストが解放するまで止める。
  await insertHoldingsData({ holdings: [TOYOTA_FIRST, TOYOTA_SECOND, SONY_50] })
  const held = await holdRequests(page, HOLDING_DELETE_API, 'DELETE')
  await page.goto(HOLDINGS_PATH)
  const dialog = await openDeleteConfirm(page, 'トヨタ自動車')

  await dialog.getByRole('button', { name: DELETE_CONFIRM_BUTTON, exact: true }).click()

  await expectDialogHeldOpenAgainstEscape(page, dialog)
  expect(held.requests()).toBe(1)

  // 応答を解放すると、ダイアログが閉じ、7203 の行がなくなり、フォーカスは見出しにある。
  held.release()
  await expect(dialog).toHaveCount(0)
  await expect(holdingRow(page, '7203')).toHaveCount(0)
  await expect(page.locator('tbody tr')).toHaveCount(1)
  await expect(holdingsHeading(page)).toBeFocused()
})

test('補助 判断37（要件13）: キーボード（Tab で移って Enter）で「削除する」を押した後、削除中は「キャンセル」が押せず、Esc を続けて3回押してもダイアログは閉じない。応答の後にダイアログが閉じ、7203 の行がなくなり、フォーカスは見出しにある', async ({ page }) => {
  await insertHoldingsData({ holdings: [TOYOTA_FIRST, TOYOTA_SECOND, SONY_50] })
  const held = await holdRequests(page, HOLDING_DELETE_API, 'DELETE')
  await page.goto(HOLDINGS_PATH)
  const dialog = await openDeleteConfirm(page, 'トヨタ自動車')
  const confirm = dialog.getByRole('button', { name: DELETE_CONFIRM_BUTTON, exact: true })

  // 開いた直後は「キャンセル」にフォーカスがある（判断33）。Tab で「削除する」へ移り、Enter で押す。
  await page.keyboard.press('Tab')
  await expect(confirm).toBeFocused()
  await page.keyboard.press('Enter')

  await expectDialogHeldOpenAgainstEscape(page, dialog)
  expect(held.requests()).toBe(1)

  held.release()
  await expect(dialog).toHaveCount(0)
  await expect(holdingRow(page, '7203')).toHaveCount(0)
  await expect(page.locator('tbody tr')).toHaveCount(1)
  await expect(holdingsHeading(page)).toBeFocused()
})

test('補助 要件1・要件14: 7203 だけを保有し、7203 を削除すると、「保有株式はありません」が表示され、「価格を今すぐ更新」も「⋯」も表示されず、フォーカスは見出しにある', async ({ page }) => {
  // Given: 利用者Aが 7203 だけを保有（価格あり。一覧と今すぐ更新のボタンが出るデータ）。
  await insertHoldingsData({ holdings: [TOYOTA_FIRST, TOYOTA_SECOND], prices: [TOYOTA_PRICE] })
  await page.goto(HOLDINGS_PATH)
  await expect(refreshButton(page)).toBeVisible()
  const dialog = await openDeleteConfirm(page, 'トヨタ自動車')

  await dialog.getByRole('button', { name: DELETE_CONFIRM_BUTTON, exact: true }).click()

  // 最後の1銘柄を消して0件になる: 空のメッセージだけが出て、表も今すぐ更新も「⋯」も合計もない。
  await expect(dialog).toHaveCount(0)
  await expect(page.getByText(EMPTY_MESSAGE)).toBeVisible()
  await expect(holdingsTable(page)).toHaveCount(0)
  await expect(refreshButton(page)).toHaveCount(0)
  await expect(refreshRegion(page)).toHaveCount(0)
  await expect(page.getByRole('button', { name: /の操作$/ })).toHaveCount(0)
  await expect(totalRegion(page)).toHaveCount(0)
  // 開いた行がなくなったので、フォーカスは見出しにある。
  await expect(holdingsHeading(page)).toBeFocused()
})

test('補助 要件14・要件15（失敗の後に押し直す）: 1回目の削除が HTTP 500 で失敗しても、ダイアログを開いたまま「削除する」を押し直すと成功し、ダイアログが閉じて 7203 の行がなくなる', async ({ page }) => {
  // Given: 7203（2件）と 6758 を保有。1回目の DELETE だけ 500、2回目は実際の応答を返す。
  await insertHoldingsData({ holdings: [TOYOTA_FIRST, TOYOTA_SECOND, SONY_50] })
  let deleteRequests = 0
  await page.route(HOLDING_DELETE_API, async (route) => {
    if (route.request().method() !== 'DELETE') {
      await route.continue()
      return
    }
    deleteRequests += 1
    if (deleteRequests === 1) {
      await route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'internal_error' }),
      })
      return
    }
    await route.continue()
  })
  await page.goto(HOLDINGS_PATH)
  const dialog = await openDeleteConfirm(page, 'トヨタ自動車')
  const confirm = dialog.getByRole('button', { name: DELETE_CONFIRM_BUTTON, exact: true })

  await confirm.click()
  await expect(dialog.getByRole('alert')).toHaveText(DELETE_ERROR_MESSAGE)
  await expect(confirm).toBeEnabled()

  // 押し直すと成功する。失敗の文は消え、ダイアログが閉じ、7203 の行がなくなる。
  await confirm.click()
  await expect(dialog).toHaveCount(0)
  await expect(holdingRow(page, '7203')).toHaveCount(0)
  await expect(page.locator('tbody tr')).toHaveCount(1)
  expect(deleteRequests).toBe(2)
})

test('補助 要件13・要件15（失敗の後にキャンセルする）: 削除が HTTP 500 で失敗した後に「キャンセル」を押すと、ダイアログが閉じ、フォーカスは「トヨタ自動車の操作」に戻り、一覧は変わらない', async ({ page }) => {
  // Given: 6758・7203（2件）・9984 の3行（真ん中の 7203 から開く）。削除の API は 500 を返す。
  await insertHoldingsData({ holdings: [SONY_50, TOYOTA_FIRST, TOYOTA_SECOND, SOFTBANK_50] })
  await page.route(HOLDING_DELETE_API, (route) =>
    route.request().method() === 'DELETE'
      ? route.fulfill({
          status: 500,
          contentType: 'application/json',
          body: JSON.stringify({ error: 'internal_error' }),
        })
      : route.continue(),
  )
  await page.goto(HOLDINGS_PATH)
  const dialog = await openDeleteConfirm(page, 'トヨタ自動車')
  await dialog.getByRole('button', { name: DELETE_CONFIRM_BUTTON, exact: true }).click()
  await expect(dialog.getByRole('alert')).toHaveText(DELETE_ERROR_MESSAGE)

  await dialog.getByRole('button', { name: DELETE_CANCEL_BUTTON, exact: true }).click()

  await expect(dialog).toHaveCount(0)
  await expect(operationButton(page, 'トヨタ自動車')).toBeFocused()
  await expectToyotaRowMerged(page)
  await expect(page.locator('tbody tr')).toHaveCount(3)
  // 裏でも消えていない。
  await page.reload()
  await expectToyotaRowMerged(page)
  await expect(page.locator('tbody tr')).toHaveCount(3)
})

test('補助 判断31 と同じ方法（デザインの指摘3）: キーボードで「削除する」を押して（Tab で移って Enter）削除が遅れて失敗した後、フォーカスは「削除する」にある', async ({ page }) => {
  // Given: 7203（2件）と 6758 を保有。削除の API は 300ms 遅れて 500 を返す（削除中の状態を確実に作るため。このテストの中だけ）。
  await insertHoldingsData({ holdings: [TOYOTA_FIRST, TOYOTA_SECOND, SONY_50] })
  await page.route(HOLDING_DELETE_API, async (route) => {
    if (route.request().method() !== 'DELETE') {
      await route.continue()
      return
    }
    await new Promise((resolve) => setTimeout(resolve, 300))
    await route.fulfill({
      status: 500,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'internal_error' }),
    })
  })
  await page.goto(HOLDINGS_PATH)
  const dialog = await openDeleteConfirm(page, 'トヨタ自動車')
  const confirm = dialog.getByRole('button', { name: DELETE_CONFIRM_BUTTON, exact: true })

  // 開いた直後は「キャンセル」にフォーカスがある（判断33）。Tab で「削除する」へ移り、Enter で押す。
  await page.keyboard.press('Tab')
  await expect(confirm).toBeFocused()
  await page.keyboard.press('Enter')

  // 失敗の文が出て、「削除する」が押せる状態に戻った後も、フォーカスは「削除する」にある（キーボードの利用者がそのまま押し直せる）。
  await expect(dialog.getByRole('alert')).toHaveText(DELETE_ERROR_MESSAGE)
  await expect(confirm).toBeEnabled()
  await expect(confirm).toBeFocused()
  // フォーカスが戻った「削除する」の説明が、失敗の文になっている（スクリーンリーダーが失敗の理由を読める。判断44）。
  await expect(confirm).toHaveAccessibleDescription(DELETE_ERROR_MESSAGE)
})

test('補助 デザインの指摘1: 幅 375px で保有のある一覧を開いても、ページ全体は横にスクロールしない', async ({ page }) => {
  // Given: 4銘柄を保有（価格あり・なしの両方。8列の表が狭い画面の幅を超える。表の中は横にスクロールしてよい）。
  await insertHoldingsData({
    holdings: [TOYOTA_FIRST, TOYOTA_SECOND, SONY_50, SOFTBANK_100],
    prices: [TOYOTA_PRICE, SONY_PRICE],
  })
  await page.setViewportSize({ width: 375, height: 667 })

  await page.goto(HOLDINGS_PATH)

  await expect(holdingsTable(page)).toBeVisible()
  await expect(operationButton(page, 'トヨタ自動車')).toBeVisible()
  const widths = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }))
  expect(widths.scrollWidth).toBeLessThanOrEqual(widths.clientWidth)
})

test('補助 判断39（デザインの指摘2）: 幅 1280px（高さ 720px）で、画面のイメージのデータに長い銘柄名・小数つきの取得価格・大きな数量と価格の行を足して開くと、表の横スクロールの領域に横のスクロールが出ず、押す前から各行の「⋯」が領域の中にある', async ({ page }) => {
  // Given: 要件定義の「画面のイメージ」と同じ 7203 トヨタ自動車（400株・2,750円・価格あり）と、
  //   9984 ソフトバンクグループ（100株・4,500円・価格なしで価格の3列が「取得できません」）に、
  //   実際の銘柄名に多い長い名前の行を足す: 8306 三菱ＵＦＪフィナンシャル・グループ（16文字。100株・1,500円・価格なし）。
  //   銘柄名の列を折り返さないと、長い名前の行で表が領域の幅を超え、「⋯」が右にはみ出す（判断39）。
  //   さらに、よくある大きめの値の行を足す（人間が決めた。それより大きい値は表の横スクロールで見る）:
  //   - 6758 ソニーグループを 100株・2,700円 と 200株・2,800円 で保有（まとめると 300株・取得価格の加重平均が小数つきの 2,766.7円。現在の価格 3,500円）
  //   - 4063 信越化学工業を 10,000株・12,000円 で保有（現在の価格 12,345.6円・評価額 123,456,000円）
  await insertHoldingsData({
    stocks: [
      { code: LONG_NAME_CODE, name: LONG_NAME },
      { code: BIG_QUANTITY_CODE, name: BIG_QUANTITY_NAME },
      { code: KATAKANA_NAME_CODE, name: KATAKANA_NAME },
    ],
    holdings: [
      TOYOTA_FIRST,
      TOYOTA_SECOND,
      SOFTBANK_100,
      { code: LONG_NAME_CODE, quantity: 100, acquisitionPrice: '1500.0' },
      { code: '6758', quantity: 100, acquisitionPrice: '2700.0' },
      { code: '6758', quantity: 200, acquisitionPrice: '2800.0' },
      { code: BIG_QUANTITY_CODE, quantity: 10000, acquisitionPrice: '12000.0' },
      { code: KATAKANA_NAME_CODE, quantity: 100, acquisitionPrice: '1000.0' },
    ],
    prices: [
      TOYOTA_PRICE,
      SONY_PRICE,
      { code: BIG_QUANTITY_CODE, tradingDate: '2026-10-06', closePrice: '12345.6' },
    ],
  })
  await page.setViewportSize({ width: 1280, height: 720 })

  await page.goto(HOLDINGS_PATH)

  await expect(dataCells(holdingRow(page, '9984'))).toHaveText([
    '9984',
    'ソフトバンクグループ',
    '100株',
    '4,500円',
    UNAVAILABLE,
    UNAVAILABLE,
    UNAVAILABLE,
  ])
  // 長い銘柄名の行: 銘柄名のセルは2行以上に折り返し（切れず）、ほかのデータの6つのセルは1行のまま（判断39）。
  const longRowCells = dataCells(holdingRow(page, LONG_NAME_CODE))
  await expect(longRowCells).toHaveCount(7)
  const cellShapes = await longRowCells.evaluateAll((cells) =>
    cells.map((cell) => {
      const range = document.createRange()
      range.selectNodeContents(cell)
      const lineTops = new Set(Array.from(range.getClientRects()).map((rect) => Math.round(rect.top)))
      return { lines: lineTops.size, clipped: cell.scrollWidth > cell.clientWidth }
    }),
  )
  const NAME_CELL = 1
  expect(cellShapes[NAME_CELL]?.lines, '銘柄名のセルの行数').toBeGreaterThanOrEqual(2)
  expect(cellShapes[NAME_CELL]?.clipped, '銘柄名のセルが切れている').toBe(false)
  cellShapes.forEach((shape, index) => {
    if (index !== NAME_CELL) expect(shape.lines, `${index + 1}つ目のセルの行数`).toBe(1)
  })
  // 小数つきの取得価格の行と、大きな数量・価格の行が、期待どおりに表示されている（幅を測る行がこの値であることの確認）。
  await expect(dataCells(holdingRow(page, '6758'))).toHaveText([
    '6758',
    'ソニーグループ',
    '300株',
    '2,766.7円',
    '3,500円',
    '2026-10-06',
    '1,050,000円',
  ])
  await expect(dataCells(holdingRow(page, BIG_QUANTITY_CODE))).toHaveText([
    BIG_QUANTITY_CODE,
    BIG_QUANTITY_NAME,
    '10,000株',
    '12,000円',
    '12,345.6円',
    '2026-10-06',
    '123,456,000円',
  ])
  // 長いカタカナの名前の行（6752 パナソニックホールディングス）: 銘柄名のセルが切れていない。
  await expect(dataCells(holdingRow(page, KATAKANA_NAME_CODE))).toHaveText([
    KATAKANA_NAME_CODE,
    KATAKANA_NAME,
    '100株',
    '1,000円',
    UNAVAILABLE,
    UNAVAILABLE,
    UNAVAILABLE,
  ])
  const katakanaNameCell = dataCells(holdingRow(page, KATAKANA_NAME_CODE)).nth(1)
  const katakanaNameShape = await katakanaNameCell.evaluate((cell) => ({ scrollWidth: cell.scrollWidth, clientWidth: cell.clientWidth }))
  expect(katakanaNameShape.scrollWidth, '長いカタカナの銘柄名のセルが切れている').toBeLessThanOrEqual(katakanaNameShape.clientWidth)
  const region = page.getByRole('region', { name: PAGE_TITLE, exact: true })
  // 表の横スクロールの領域に、横のスクロールが出ない（表が領域の幅に収まっている）。
  const overflow = await region.evaluate((element) => ({
    scrollWidth: element.scrollWidth,
    clientWidth: element.clientWidth,
  }))
  expect(overflow.scrollWidth, '領域の横スクロールの幅（scrollWidth）').toBeLessThanOrEqual(overflow.clientWidth)
  const regionBox = await region.boundingBox()
  if (regionBox === null) throw new Error('表の横スクロールの領域の位置を取れない')
  // 押す前に、「⋯」のボタンの右端が領域の右端より内側にある（横にスクロールしないと見えない位置ではない）。
  for (const name of ['トヨタ自動車', 'ソフトバンクグループ', LONG_NAME, 'ソニーグループ', BIG_QUANTITY_NAME, KATAKANA_NAME]) {
    const box = await operationButton(page, name).boundingBox()
    if (box === null) throw new Error(`${name}の「⋯」の位置を取れない`)
    expect(box.x + box.width, `${name}の「⋯」の右端`).toBeLessThanOrEqual(regionBox.x + regionBox.width)
    expect(box.x, `${name}の「⋯」の左端`).toBeGreaterThanOrEqual(regionBox.x)
  }
})

test('補助 要件2・判断21（キーボードだけで確認のダイアログを開く）: 「⋯」にフォーカスして Enter でメニューを開き、「削除」で Enter を押すと、確認のダイアログが開く（フォーカスは「キャンセル」）', async ({ page }) => {
  // Given: 6758・7203・9984 の3行（真ん中の 7203 から開く）。
  await insertHoldingsData({ holdings: [SONY_50, TOYOTA_FIRST, SOFTBANK_50] })
  await page.goto(HOLDINGS_PATH)
  const button = operationButton(page, 'トヨタ自動車')
  await expect(button).toBeVisible()

  await button.focus()
  await page.keyboard.press('Enter')

  // メニューが開き、先頭の項目「削除」にフォーカスがある。
  await expect(page.getByRole('menu')).toBeVisible()
  await expect(page.getByRole('menuitem', { name: DELETE_MENU_ITEM, exact: true })).toBeFocused()

  await page.keyboard.press('Enter')

  // 確認のダイアログが開き、メニューは閉じている。フォーカスは「キャンセル」（判断33）。
  const dialog = deleteConfirmDialog(page)
  await expect(dialog).toBeVisible()
  await expect(page.getByRole('menu')).toHaveCount(0)
  await expect(dialog.getByRole('button', { name: DELETE_CANCEL_BUTTON, exact: true })).toBeFocused()
  // 何も消えていない。Esc で閉じると、フォーカスは開いた「⋯」に戻る。
  await expect(holdingRow(page, '7203')).toHaveCount(1)
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  await expect(button).toBeFocused()
})

test('補助 判断40（ステップ6 のテストの網羅の指摘3）: 今すぐ更新が終わらないうちに 7203 の確認のダイアログを開き、その間に 7203 が消えると、一覧が取り直されて 7203 の行がなくなり、「キャンセル」で閉じたとき、フォーカスは見出しに移る', async ({ page }) => {
  // Given: 6758・7203（2件）・9984 の3行を保有。今すぐ更新の要求は、テストが解放するまで止める。
  await insertHoldingsData({ holdings: [SONY_50, TOYOTA_FIRST, TOYOTA_SECOND, SOFTBANK_50] })
  const refresh = await holdRequests(page, PRICE_REFRESH_API, 'POST')
  await page.goto(HOLDINGS_PATH)
  await refreshButton(page).click()
  await expect(page.getByRole('button', { name: '更新中…', exact: true })).toBeDisabled()
  const dialog = await openDeleteConfirm(page, 'トヨタ自動車')

  // 確認のダイアログを開いている間に、DB から 7203 が消える（6758 と 9984 だけを入れ直す）。
  await resetHoldingsData()
  await insertHoldingsData({ holdings: [SONY_50, SOFTBANK_50] })
  // 今すぐ更新の応答を解放すると、一覧が取り直され、7203 の行が（背後で）なくなる。ダイアログは開いたまま。
  refresh.release()
  await expect(holdingRow(page, '7203')).toHaveCount(0)
  await expect(dialog).toBeVisible()

  // 「キャンセル」で閉じると、戻すはずの「トヨタ自動車の操作」はもう画面にないので、フォーカスは見出しに移る。
  await dialog.getByRole('button', { name: DELETE_CANCEL_BUTTON, exact: true }).click()
  await expect(dialog).toHaveCount(0)
  await expect(holdingsHeading(page)).toBeFocused()
})

test('補助 判断36・判断41（ステップ6 のテストの網羅の指摘4）: 高さ 480px の画面で、最後の行の「⋯」を画面の下端に寄せて押すと、メニューは画面の中に、「⋯」の上に出て、「削除」を押すと確認のダイアログが開く', async ({ page }) => {
  // Given: 12銘柄（1001〜1012）を保有。高さ 480px の画面で、ページの末尾の行が画面の下端に来るまでスクロールする。
  const codes = Array.from({ length: 12 }, (_, i) => String(1001 + i))
  await insertHoldingsData({
    stocks: codes.map((code) => ({ code, name: `E2E銘柄${code}` })),
    holdings: codes.map((code) => ({ code, quantity: 100, acquisitionPrice: '2500.0' })),
  })
  await page.setViewportSize({ width: 1280, height: 480 })
  await page.goto(HOLDINGS_PATH)
  const lastButton = operationButton(page, 'E2E銘柄1012')
  await expect(lastButton).toBeVisible()
  // 「⋯」の下端が画面の下端にぴったり合うようにスクロールする（下にメニューを出す余地がない）。
  await lastButton.evaluate((element) => element.scrollIntoView({ block: 'end' }))
  const buttonBox = await lastButton.boundingBox()
  if (buttonBox === null) throw new Error('「⋯」の位置を取れない')
  expect(480 - (buttonBox.y + buttonBox.height), '「⋯」の下の余白').toBeLessThanOrEqual(2)

  await lastButton.click()

  const menu = page.getByRole('menu')
  await expect(menu).toBeVisible()
  const menuBox = await menu.boundingBox()
  if (menuBox === null) throw new Error('メニューの位置を取れない')
  // メニューの箱は画面の中に収まり、「⋯」の上（メニューの下端が「⋯」の上端より上）に出る。
  expect(menuBox.x, 'メニューの左端').toBeGreaterThanOrEqual(0)
  expect(menuBox.y, 'メニューの上端').toBeGreaterThanOrEqual(0)
  expect(menuBox.x + menuBox.width, 'メニューの右端').toBeLessThanOrEqual(1280)
  expect(menuBox.y + menuBox.height, 'メニューの下端').toBeLessThanOrEqual(480)
  expect(menuBox.y + menuBox.height, 'メニューの下端と「⋯」の上端').toBeLessThanOrEqual(buttonBox.y)

  // 「削除」を押すと確認のダイアログが開く。
  await page.getByRole('menuitem', { name: DELETE_MENU_ITEM, exact: true }).click()
  const dialog = page.getByRole('alertdialog')
  await expect(dialog).toBeVisible()
  await expect(dialog).toContainText('1012 E2E銘柄1012を削除しますか？')
})

// ======================================================================
// PR② ステップ6 3ラウンド目の指摘で足したテスト
// ======================================================================

test('補助 判断37・判断22（ステップ6 3ラウンド目の優先度1）: 削除中に Esc を止める見張りは、削除が失敗して終わった後は外れている。失敗の後に Esc を1回押すとダイアログが閉じ、フォーカスは「トヨタ自動車の操作」に戻り、7203 は「400株」のままである', async ({ page }) => {
  // Given: 6758・7203（2件）・9984 の3行を保有。削除の API の要求は、テストが解放するまで止め、解放すると HTTP 500 を返す。
  await insertHoldingsData({ holdings: [SONY_50, TOYOTA_FIRST, TOYOTA_SECOND, SOFTBANK_50] })
  const held = await holdRequests(page, HOLDING_DELETE_API, 'DELETE', 500)
  await page.goto(HOLDINGS_PATH)
  const dialog = await openDeleteConfirm(page, 'トヨタ自動車')
  const cancel = dialog.getByRole('button', { name: DELETE_CANCEL_BUTTON, exact: true })

  await dialog.getByRole('button', { name: DELETE_CONFIRM_BUTTON, exact: true }).click()

  // 削除中は「キャンセル」を押せない。
  await expect(cancel).toBeDisabled()

  // 応答を解放する（HTTP 500）。「削除できませんでした」が出て、「キャンセル」が押せる状態に戻る。
  held.release()
  await expect(dialog.getByRole('alert')).toHaveText(DELETE_ERROR_MESSAGE)
  await expect(cancel).toBeEnabled()

  // 失敗の後は、Esc で閉じられる（削除中だけ Esc を止める見張りが、削除が終わった後も残っていないこと）。
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  await expect(operationButton(page, 'トヨタ自動車')).toBeFocused()
  await expectToyotaRowMerged(page)
  await expect(page.locator('tbody tr')).toHaveCount(3)
})

test('補助 判断41（ステップ6 3ラウンド目のテストの網羅の指摘3）: 幅 375px で表の領域を横にスクロールして「⋯」の右側が画面の外に出る位置にし、見えている部分を押しても、メニューは横にも画面の中（左端が0以上、右端が375以下）に出る', async ({ page }) => {
  // Given: 7203・6758 を保有（価格あり。8列の表が狭い画面の幅を超え、表の領域が横にスクロールする）。
  await insertHoldingsData({
    holdings: [TOYOTA_FIRST, TOYOTA_SECOND, SONY_50],
    prices: [TOYOTA_PRICE, SONY_PRICE],
  })
  const VIEWPORT_WIDTH = 375
  await page.setViewportSize({ width: VIEWPORT_WIDTH, height: 667 })
  await page.goto(HOLDINGS_PATH)
  const region = page.getByRole('region', { name: PAGE_TITLE, exact: true })
  const button = operationButton(page, 'トヨタ自動車')
  await expect(button).toBeAttached()

  // 表の領域を横にスクロールして、「⋯」の左の6pxだけが領域（と画面）の右端の内側に見える位置にする。
  // このとき「⋯」の右側は画面の外に出る。
  const regionBox = await region.boundingBox()
  if (regionBox === null) throw new Error('表の横スクロールの領域の位置を取れない')
  const regionRight = regionBox.x + regionBox.width
  const before = await button.boundingBox()
  if (before === null) throw new Error('「⋯」の位置を取れない')
  const VISIBLE_PART_PX = 6
  await region.evaluate((element, delta) => {
    element.scrollLeft += delta
  }, before.x - (regionRight - VISIBLE_PART_PX))
  const box = await button.boundingBox()
  if (box === null) throw new Error('「⋯」の位置を取れない')
  expect(box.x + box.width, 'スクロール後の「⋯」の右端（画面の外に出ている）').toBeGreaterThan(VIEWPORT_WIDTH)
  expect(box.x, 'スクロール後の「⋯」の左端（見えている）').toBeLessThan(regionRight)

  // 見えている部分を押す。
  await page.mouse.click(box.x + VISIBLE_PART_PX / 2, box.y + box.height / 2)

  const menu = page.getByRole('menu')
  await expect(menu).toBeVisible()
  const menuBox = await menu.boundingBox()
  if (menuBox === null) throw new Error('メニューの位置を取れない')
  expect(menuBox.x, 'メニューの左端').toBeGreaterThanOrEqual(0)
  expect(menuBox.x + menuBox.width, 'メニューの右端').toBeLessThanOrEqual(VIEWPORT_WIDTH)
})
