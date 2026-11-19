import { expect, test, type Locator, type Page, type Response } from '@playwright/test'
import {
  insertHoldingsData,
  resetHoldingsData,
  type NotificationSeed,
  type StockPriceSeed,
  type StockSeed,
  type TradeSeed,
} from '../fixtures/db'
import {
  GAIN_COLOR,
  LOSS_COLOR,
  cardList,
  computedColor,
  holdingCards,
  refreshButton,
} from '../fixtures/holdings-page'

// 仕様: 要件 docs/requirements/price-alerts.md の「ベルとバッジ」（基準29〜基準34）と「通知のリスト」（基準35〜基準38・基準40・基準41）、非機能2（基準55）の E2E。
//   「条件を決める」のリンク（要件29）と /alerts の画面は価格の変動の通知の PR③ で足すので、このファイルでは確かめない（基準35・基準41 の「条件を決める」の部分は PR③）。
// テスト計画: プラン docs/plans/price-alerts/plan.md 5章の冒頭の決まり・5.1 の PR② の E2E の行（notifications.spec.ts）。
// 画面の文言と構造: プラン 4.6「画面」・4.8 の PR②
//   - /holdings の見出し「保有株式一覧」と同じ行の右端にベルのリンク（<a href="/notifications">。名前は「通知（新着 n件）」か「通知」）。バッジは aria-hidden の要素で「3」か「99+」。
//   - /notifications: 左上に「← 保有株式一覧」のリンク、見出し「通知」、リストは <ul role="list">（aria-labelledby で名前は「通知」）。
//     1件が1つの <li>で、中は1つのリンク（/holdings/<銘柄コード>）が4行を全部包む。新着は「新着」の文字。率と含み損益の値の要素に data-tone="gain|loss"（色なしは属性なし）。
// 待ち方（プラン 5章の冒頭）:
//   - 「バッジがない」「新着の印がない」は、読み込み中・0件・失敗のどれでも同じに見える。ないことを確かめる前に page.waitForResponse で
//     GET /api/notifications/unseen-count を待ち、本文が {"count":0} であることも確かめる（失敗の基準は失敗を待つ）。
//   - 見たにした後に戻る基準は、POST /api/notifications/seen の 204 を待ってから戻る。「新着の印がない」は、リストの件数が出るのを待ってから（toHaveCount(n)）確かめる。
// 実行の前提: 前の機能の E2E と同じ（zero_e2e、API は利用者A で起動、beforeEach でデータを入れ替える、順に実行、America/Los_Angeles）。
//   今すぐ更新（基準32）は、E2E 用の API のダミーの取得元をそのまま使う（7203 は 2026-10-06 の終値 3,000円）。
// mock は境界の外側（ブラウザから見たネットワーク = page.route）にだけ使う。

// 1件失敗しても後続を止めずに、順に実行する（同じ DB を使うため）。
test.describe.configure({ mode: 'default' })

// 前の機能の E2E と同じく、UTC より遅いタイムゾーンで実行する（取引日が Date に変換されてずれないことも、あわせて確かめられる）。
test.use({ timezoneId: 'America/Los_Angeles' })

test.beforeEach(async () => {
  await resetHoldingsData()
})

const HOLDINGS_PATH = '/holdings'
const NOTIFICATIONS_PATH = '/notifications'
const UNSEEN_COUNT_API = '/api/notifications/unseen-count'
const NOTIFICATIONS_API = '/api/notifications'
const SEEN_API = '/api/notifications/seen'

// ---- 画面の文言（要件24〜要件37） ----
const HOLDINGS_TITLE = '保有株式一覧'
const EMPTY_HOLDINGS_MESSAGE = '保有株式はありません'
const BACK_LINK = '← 保有株式一覧'
const NOTIFICATIONS_TITLE = '通知'
const NEW_MARK = '新着'
const EMPTY_NOTIFICATIONS = '通知はまだありません。'
const EMPTY_NOTIFICATIONS_HINT = '条件に当てはまる終値が保存されると、ここに並びます。'
const NOTIFICATIONS_FETCH_ERROR = '通知を取得できませんでした。時間をおいて開き直してください。'

// ---- テストデータ（要件定義の成功基準の銘柄マスタとダミー価格） ----
const BUY_DATE = '2026-01-10'
const TOYOTA_100: TradeSeed = { code: '7203', side: 'buy', quantity: 100, price: '2500.0', tradeDate: BUY_DATE }
const TOYOTA_PRICE: StockPriceSeed = { code: '7203', tradingDate: '2026-10-06', closePrice: '3000.0' }
const APPLE: StockSeed = { code: 'AAPL', name: 'Apple', currency: 'USD' }

/** 通知（直接）の既定の値。7203・取得価格から・全銘柄・下がった・20%・終値 1,990円。 */
function note(overrides: Partial<NotificationSeed> & Pick<NotificationSeed, 'tradingDate'>): NotificationSeed {
  return {
    stockCode: '7203',
    kind: 'cost',
    scope: 'all',
    direction: 'down',
    threshold: '20',
    closePrice: '1990',
    details: { rateTenths: -204, basePriceHundredths: 250000 },
    seen: false,
    ...overrides,
  }
}

/**
 * 基準35〜基準37 の通知（直接）5件（要件定義の「通知のリスト」の冒頭）。一覧の並びは取引日の新しい順
 * （7203・AAPL・9984・6758（11-10）・6758（11-04））。入れる順は表示の順とずらす（6758（11-04）・9984・AAPL・6758（11-10）・7203 の順）ので、
 * id の昇順でも降順でも表示の順にならない（id で並べる実装を通さない）。
 */
function fiveNotifications(): NotificationSeed[] {
  return [
    // 2026-11-04 6758 含み損益の金額（取得価格 4,000円 → 終値 3,480円、200株、含み損益 -104,000円。条件: 6758・含み損が 100,000円 以上になったら）（見た）
    note({
      tradingDate: '2026-11-04',
      stockCode: '6758',
      kind: 'pnl',
      scope: 'stock',
      conditionStockCode: '6758',
      direction: 'loss',
      threshold: '100000',
      closePrice: '3480',
      details: { unrealizedGainHundredths: -10400000, basePriceHundredths: 400000, quantity: 200 },
      seen: true,
    }),
    // 2026-11-13 9984 決めた価格（終値 8,120円、決めた価格 8,000円 以上。条件: 9984・8,000円 以上になったら）（見ていない）
    note({
      tradingDate: '2026-11-13',
      stockCode: '9984',
      kind: 'price',
      scope: 'stock',
      conditionStockCode: '9984',
      direction: 'above',
      threshold: '8000',
      closePrice: '8120',
      details: {},
    }),
    // 2026-11-14 AAPL 前の取引日から（前の終値 180.25ドル（2026-11-13）→ 終値 191.24ドル、+6.1%）（見ていない）
    note({
      tradingDate: '2026-11-14',
      stockCode: 'AAPL',
      kind: 'prev',
      direction: 'either',
      threshold: '5',
      closePrice: '191.24',
      details: { rateTenths: 61, basePriceHundredths: 18025, baseDate: '2026-11-13' },
    }),
    // 2026-11-10 6758 最高値から（最高値 4,000円（2026-09-30）→ 終値 3,392円、-15.2%）（見た）
    note({
      tradingDate: '2026-11-10',
      stockCode: '6758',
      kind: 'peak',
      threshold: '15',
      closePrice: '3392',
      details: { rateTenths: -152, basePriceHundredths: 400000, baseDate: '2026-09-30' },
      seen: true,
    }),
    // 2026-11-14 7203 取得価格から（取得価格 2,500円 → 終値 1,990円、-20.4%）（見ていない）
    note({ tradingDate: '2026-11-14', stockCode: '7203' }),
  ]
}

/** 日付 base（"YYYY-MM-DD"）の days 日後の日付（"YYYY-MM-DD"。タイムゾーンに左右されない）。 */
function addDays(base: string, days: number): string {
  const [year, month, day] = base.split('-').map(Number)
  return new Date(Date.UTC(year, month - 1, day + days)).toISOString().slice(0, 10)
}

/**
 * 見ていない通知 count 件（7203 取得価格から）。取引日は 2026-07-01 から1日1件（count 件で 2026-07-01 から count 日分）。
 * 入れる順は取引日の順とずらす（i 番目に 7 日おきの日を入れる。count が 7 の倍数でなければ、すべての日を1回ずつ使う）ので、
 * id の昇順でも降順でも取引日の順にならない（id で並べる・id で100件に絞る実装を通さない）。
 * 逆にして、最後に入れる（id が最大の）通知を 2026-07-01（一番古く、100件の一覧に出ない日）にする。
 * 「表示した通知の最大の ID まで見たにする」実装では、出していない古い通知が見たにならずに残る（基準38）。
 */
function dailyNotifications(count: number): NotificationSeed[] {
  return Array.from({ length: count }, (_, i) => note({ tradingDate: addDays('2026-07-01', (i * 7) % count) })).reverse()
}

// ---- 画面の要素と待ち方 ----
function holdingsHeading(page: Page): Locator {
  return page.getByRole('heading', { name: HOLDINGS_TITLE, exact: true })
}

/** /holdings のベルのリンク。name は「通知」か「通知（新着 n件）」。 */
function bell(page: Page, name: string): Locator {
  return page.getByRole('link', { name, exact: true })
}

/** 「バッジがない」ことの確かめ。リンクの文字が空で、かつリンクの直下に SVG（アイコン）以外の要素がない（空のバッジの要素を描く実装を通さない）。 */
async function expectNoBadge(link: Locator): Promise<void> {
  await expect(link).toHaveText(/^$/)
  await expect(link.locator(':scope > :not(svg)')).toHaveCount(0)
}

/** 見ていない数の取得（GET）の応答を待つ。開く前（クリックの前）に仕掛けて、後で await する。 */
function waitForUnseenCount(page: Page): Promise<Response> {
  return page.waitForResponse(
    (response) => response.url().endsWith(UNSEEN_COUNT_API) && response.request().method() === 'GET',
  )
}

/** 見たにする（POST）の応答を待つ。 */
function waitForSeen(page: Page): Promise<Response> {
  return page.waitForResponse(
    (response) => response.url().endsWith(SEEN_API) && response.request().method() === 'POST',
  )
}

async function expectUnseenCount(response: Promise<Response>, count: number): Promise<void> {
  const resolved = await response
  expect(resolved.status()).toBe(200)
  expect(await resolved.json()).toEqual({ count })
}

/** 通知のリスト（<ul role="list">。名前は見出し「通知」）。 */
function notificationList(page: Page): Locator {
  return page.getByRole('list', { name: NOTIFICATIONS_TITLE, exact: true })
}

function notificationItems(page: Page): Locator {
  return notificationList(page).getByRole('listitem')
}

function newMarks(page: Page): Locator {
  return page.getByText(NEW_MARK, { exact: true })
}

// =====================================================================================================================
// ベルとバッジ（要件24〜要件28）
// =====================================================================================================================

test('基準29: /holdings の見出しと同じ行の右端にベルのリンクがあり、見ていない通知3件でバッジに「3」、読み上げ名は「通知（新着 3件）」', async ({ page }) => {
  // Given: Aが 7203「買 2,500円 100株」。通知（直接）: 見ていない3件、見た2件。
  await insertHoldingsData({ stocks: [APPLE], trades: [TOYOTA_100], prices: [TOYOTA_PRICE], notifications: fiveNotifications() })
  const counted = waitForUnseenCount(page)

  await page.goto(HOLDINGS_PATH)

  await expectUnseenCount(counted, 3)
  const link = bell(page, '通知（新着 3件）')
  await expect(link).toBeVisible()
  await expect(link).toHaveAttribute('href', NOTIFICATIONS_PATH)
  await expect(link).toHaveText('3')
  // 見出しの行の右端: 見出しより右で、同じ行（縦の中央がそろう）にあり、右端が一覧の右端にそろう。
  await expect(cardList(page)).toBeVisible()
  const headingBox = await holdingsHeading(page).boundingBox()
  const linkBox = await link.boundingBox()
  const listBox = await cardList(page).boundingBox()
  if (headingBox === null || linkBox === null || listBox === null) throw new Error('位置を取れない')
  expect(linkBox.x).toBeGreaterThanOrEqual(headingBox.x + headingBox.width)
  expect(Math.abs(linkBox.y + linkBox.height / 2 - (headingBox.y + headingBox.height / 2))).toBeLessThanOrEqual(4)
  expect(Math.abs(linkBox.x + linkBox.width - (listBox.x + listBox.width))).toBeLessThanOrEqual(4)
})

test('基準30: 見ていない通知が0件のとき、バッジはなく（リンクの中に文字がない）、読み上げ名は「通知」', async ({ page }) => {
  await insertHoldingsData({ trades: [TOYOTA_100], prices: [TOYOTA_PRICE] })
  const counted = waitForUnseenCount(page)

  await page.goto(HOLDINGS_PATH)

  // 数の応答（{"count":0}）を待ってから、バッジがないことを確かめる（読み込み中・失敗と同じ見た目なので）。
  await expectUnseenCount(counted, 0)
  await expect(holdingCards(page)).toHaveCount(1)
  const link = bell(page, '通知')
  await expect(link).toBeVisible()
  await expectNoBadge(link)
})

test('基準30: 見ていない通知が120件のとき、バッジは「99+」、読み上げ名は「通知（新着 120件）」', async ({ page }) => {
  await insertHoldingsData({ trades: [TOYOTA_100], prices: [TOYOTA_PRICE], notifications: dailyNotifications(120) })
  const counted = waitForUnseenCount(page)

  await page.goto(HOLDINGS_PATH)

  await expectUnseenCount(counted, 120)
  const link = bell(page, '通知（新着 120件）')
  await expect(link).toBeVisible()
  await expect(link).toHaveText('99+')
})

test('基準31: 売買履歴が0件でも（「保有株式はありません」の画面でも）、ベルのリンクがある', async ({ page }) => {
  const counted = waitForUnseenCount(page)

  await page.goto(HOLDINGS_PATH)

  await expect(page.getByText(EMPTY_HOLDINGS_MESSAGE, { exact: true })).toBeVisible()
  await expectUnseenCount(counted, 0)
  await expect(holdingsHeading(page)).toBeVisible()
  await expect(bell(page, '通知')).toBeVisible()
})

test('基準32: 「価格を今すぐ更新」が終わると、見ていない数を取り直して、バッジに「1」が出る（今すぐ更新で判定して通知ができる）', async ({ page }) => {
  // Given: Aが 7203「買 2,500円 100株」、条件: 全銘柄・取得価格から 20% 以上 上がったら。通知は0件。ダミーの取得元は 7203 が 3,000円（+20.0%）。
  await insertHoldingsData({
    trades: [TOYOTA_100],
    alertConditions: [{ kind: 'cost', scope: 'all', direction: 'up', threshold: '20' }],
  })
  const counted = waitForUnseenCount(page)

  await page.goto(HOLDINGS_PATH)

  // 開いた時点（{"count":0}）はバッジなし。
  await expectUnseenCount(counted, 0)
  await expect(holdingCards(page)).toHaveCount(1)
  await expectNoBadge(bell(page, '通知'))

  // 今すぐ更新を押す。更新が終わった後に、見ていない数を取り直す。
  const recounted = waitForUnseenCount(page)
  await refreshButton(page).click()

  await expectUnseenCount(recounted, 1)
  const link = bell(page, '通知（新着 1件）')
  await expect(link).toBeVisible()
  await expect(link).toHaveText('1')
})

test('基準33: 見ていない数の取得が失敗するとき、ベルのリンクはあり、バッジも失敗の文もない。カードは出る', async ({ page }) => {
  // Given: 見ていない通知が3件で、見ていない数の取得が失敗する（通信を失敗させる）。
  await insertHoldingsData({ stocks: [APPLE], trades: [TOYOTA_100], prices: [TOYOTA_PRICE], notifications: fiveNotifications() })
  await page.route(`**${UNSEEN_COUNT_API}`, (route) => route.abort())
  const failed = page.waitForEvent('requestfailed', (request) => request.url().endsWith(UNSEEN_COUNT_API))

  await page.goto(HOLDINGS_PATH)

  await failed
  // 失敗の通信が終わっても、アプリが失敗を処理する（TanStack Query が状態を伝える）のは次のタスク以降。
  // ベルは一覧（カード）が出てからでないと描かれないので、一覧の応答を数の失敗の後に回すことはできない（ベルが出る前に数の取得は始まらず、待ち合わせが終わらない）。
  // 代わりに、描画のフレームを2回進めて、失敗の処理が済んだ後に「ないこと」を確かめる。
  await page.evaluate(
    () => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))),
  )
  await expect(holdingCards(page)).toHaveCount(1)
  const link = bell(page, '通知')
  await expect(link).toBeVisible()
  await expectNoBadge(link)
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expect(page.getByRole('alertdialog')).toHaveCount(0)
  await expect(page.getByText(/失敗|取得できません|エラー/)).toHaveCount(0)
})

test('基準34: /holdings のベルのリンクを押すと、/notifications に移る', async ({ page }) => {
  await insertHoldingsData({ trades: [TOYOTA_100], prices: [TOYOTA_PRICE] })
  await page.goto(HOLDINGS_PATH)

  await bell(page, '通知').click()

  await expect(page).toHaveURL(new RegExp(`${NOTIFICATIONS_PATH}$`))
  await expect(page.getByRole('heading', { name: NOTIFICATIONS_TITLE, exact: true })).toBeVisible()
})

// =====================================================================================================================
// 通知のリスト（要件29〜要件37）
// =====================================================================================================================

test('基準35: 「← 保有株式一覧」と見出し「通知」があり、5件が 7203・AAPL・9984・6758（11-10）・6758（11-04）の順に、4行の文字と色つきで並ぶ', async ({ page }) => {
  await insertHoldingsData({ stocks: [APPLE], notifications: fiveNotifications() })

  await page.goto(NOTIFICATIONS_PATH)

  await expect(page.getByRole('link', { name: BACK_LINK, exact: true })).toHaveAttribute('href', HOLDINGS_PATH)
  await expect(page.getByRole('heading', { name: NOTIFICATIONS_TITLE, exact: true })).toBeVisible()
  const items = notificationItems(page)
  await expect(items).toHaveCount(5)

  const expected: Array<{ texts: string[]; tone: 'gain' | 'loss' | null; toneText: string | null; color: string | null }> = [
    {
      texts: ['7203 トヨタ自動車', '2026-11-14', '取得価格から -20.4%', '取得価格 2,500円 → 終値 1,990円', '条件: 全銘柄・取得価格から 20% 以上 下がったら'],
      tone: 'loss',
      toneText: '-20.4%',
      color: LOSS_COLOR,
    },
    {
      texts: [
        'AAPL Apple',
        '2026-11-14',
        '前の取引日から +6.1%',
        '前の終値 180.25ドル（2026-11-13）→ 終値 191.24ドル',
        '条件: 全銘柄・前の取引日から 5% 以上 動いたら（上がっても下がっても）',
      ],
      tone: 'gain',
      toneText: '+6.1%',
      color: GAIN_COLOR,
    },
    {
      texts: ['9984 ソフトバンクグループ', '2026-11-13', '終値 8,120円', '決めた価格 8,000円 以上', '条件: 9984・8,000円 以上になったら'],
      tone: null,
      toneText: null,
      color: null,
    },
    {
      texts: ['6758 ソニーグループ', '2026-11-10', '最高値から -15.2%', '最高値 4,000円（2026-09-30）→ 終値 3,392円', '条件: 全銘柄・最高値から 15% 以上 下がったら'],
      tone: 'loss',
      toneText: '-15.2%',
      color: LOSS_COLOR,
    },
    {
      texts: ['6758 ソニーグループ', '2026-11-04', '含み損益 -104,000円', '取得価格 4,000円 → 終値 3,480円（200株）', '条件: 6758・含み損が 100,000円 以上になったら'],
      tone: 'loss',
      toneText: '-104,000円',
      color: LOSS_COLOR,
    },
  ]
  for (const [index, row] of expected.entries()) {
    const item = items.nth(index)
    for (const text of row.texts) {
      await expect(item, `${index + 1}件目に「${text}」`).toContainText(text)
    }
    const tone = item.locator('[data-tone]')
    if (row.tone === null) {
      // 色なし: data-tone の要素がない（決めた価格の通知の終値には色を付けない）。
      await expect(tone, `${index + 1}件目は色なし`).toHaveCount(0)
    } else {
      await expect(tone, `${index + 1}件目の色の要素`).toHaveCount(1)
      await expect(tone).toHaveAttribute('data-tone', row.tone)
      await expect(tone).toHaveText(row.toneText ?? '')
      expect(await computedColor(tone), `${index + 1}件目の色`).toBe(row.color)
    }
  }
})

test('基準36: 取得した時点で見ていない3件に「新着」の印が出る。見たにしてから /holdings に戻るとバッジはなく、開き直すと「新着」の印はどこにもない', async ({ page }) => {
  await insertHoldingsData({ stocks: [APPLE], trades: [TOYOTA_100], prices: [TOYOTA_PRICE], notifications: fiveNotifications() })
  const seen = waitForSeen(page)

  await page.goto(NOTIFICATIONS_PATH)

  const items = notificationItems(page)
  await expect(items).toHaveCount(5)
  for (const index of [0, 1, 2]) {
    await expect(items.nth(index).getByText(NEW_MARK, { exact: true })).toHaveCount(1)
  }
  for (const index of [3, 4]) {
    await expect(items.nth(index).getByText(NEW_MARK, { exact: true })).toHaveCount(0)
  }
  await expect(newMarks(page)).toHaveCount(3)

  // 見たにする POST の 204 を待ってから戻る。
  expect((await seen).status()).toBe(204)
  const counted = waitForUnseenCount(page)
  await page.getByRole('link', { name: BACK_LINK, exact: true }).click()
  await expect(page).toHaveURL(new RegExp(`${HOLDINGS_PATH}$`))
  await expectUnseenCount(counted, 0)
  const link = bell(page, '通知')
  await expect(link).toBeVisible()
  await expectNoBadge(link)

  // もう一度 /notifications を開く: 5件の <li> が出るのを待ってから、「新着」の文字がどこにもないことを確かめる。
  await link.click()
  await expect(notificationItems(page)).toHaveCount(5)
  await expect(newMarks(page)).toHaveCount(0)
})

test('基準37: 7203 の通知を押すと、/holdings/7203 に移る', async ({ page }) => {
  await insertHoldingsData({ stocks: [APPLE], trades: [TOYOTA_100], prices: [TOYOTA_PRICE], notifications: fiveNotifications() })
  await page.goto(NOTIFICATIONS_PATH)
  await expect(notificationItems(page)).toHaveCount(5)

  await notificationItems(page).first().getByRole('link').click()

  await expect(page).toHaveURL(new RegExp('/holdings/7203$'))
})

test('基準38: 見ていない通知が120件のとき、2026-10-28 から 2026-07-21 までの100件が新しい順に並び、見たにする POST の後 /holdings に戻るとバッジはない（出していない古い20件も見たになる）', async ({ page }) => {
  // Given: 見ていない通知が120件（2026-07-01〜2026-10-28 に1日1件。7203 取得価格から）。
  await insertHoldingsData({ trades: [TOYOTA_100], prices: [TOYOTA_PRICE], notifications: dailyNotifications(120) })
  const seen = waitForSeen(page)

  await page.goto(NOTIFICATIONS_PATH)

  const items = notificationItems(page)
  await expect(items).toHaveCount(100)
  await expect(items.first()).toContainText('2026-10-28')
  await expect(items.last()).toContainText('2026-07-21')

  expect((await seen).status()).toBe(204)
  const counted = waitForUnseenCount(page)
  await page.getByRole('link', { name: BACK_LINK, exact: true }).click()

  // 表示した100件だけを見たにする実装なら、ここで 20 が返る。
  await expectUnseenCount(counted, 0)
  await expect(holdingCards(page)).toHaveCount(1)
  await expectNoBadge(bell(page, '通知'))
})

test('基準40: 通知が0件のとき、/notifications に「通知はまだありません。」と「条件に当てはまる終値が保存されると、ここに並びます。」が出る', async ({ page }) => {
  await page.goto(NOTIFICATIONS_PATH)

  await expect(page.getByText(EMPTY_NOTIFICATIONS, { exact: true })).toBeVisible()
  await expect(page.getByText(EMPTY_NOTIFICATIONS_HINT, { exact: true })).toBeVisible()
})

test('基準41: 通知の取得が失敗するとき、「← 保有株式一覧」・見出し「通知」と、「通知を取得できませんでした。時間をおいて開き直してください。」が出る', async ({ page }) => {
  await insertHoldingsData({ notifications: fiveNotifications(), stocks: [APPLE] })
  await page.route(`**${NOTIFICATIONS_API}`, (route) => route.abort())
  const failed = page.waitForEvent('requestfailed', (request) => request.url().endsWith(NOTIFICATIONS_API))

  await page.goto(NOTIFICATIONS_PATH)

  await failed
  await expect(page.getByText(NOTIFICATIONS_FETCH_ERROR, { exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: BACK_LINK, exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: NOTIFICATIONS_TITLE, exact: true })).toBeVisible()
  await expect(notificationItems(page)).toHaveCount(0)
})

// =====================================================================================================================
// 性能（非機能2）
// =====================================================================================================================

test('基準55: 通知が10,000件あるとき、/notifications を開いてから100件目の通知が出るまでが2秒以内', async ({ page, browser }) => {
  // Given: Aの通知が10,000件。一意制約（条件 ID・銘柄・取引日）があるので、条件 ID を1件ごとに変える。取引日は 700日の間に散らす。
  const notifications: NotificationSeed[] = Array.from({ length: 10_000 }, (_, i) =>
    note({ conditionId: i + 1, tradingDate: addDays('2025-01-01', i % 700) }),
  )
  await insertHoldingsData({ notifications })

  // ウォームアップ: 一度 /notifications を開いて捨てる（dev サーバーの初回の変換などを計測に含めない）。見たにする POST が済むのも待つ。
  const warmedSeen = waitForSeen(page)
  await page.goto(NOTIFICATIONS_PATH)
  await expect(notificationItems(page)).toHaveCount(100)
  await warmedSeen
  await page.close()

  // 計測: 新しいブラウザコンテキストで開き直す（ウォームアップのページのキャッシュを使わない）。
  const context = await browser.newContext({ timezoneId: 'America/Los_Angeles' })
  try {
    const measuredPage = await context.newPage()
    await measuredPage.goto(NOTIFICATIONS_PATH)
    // 100件目の通知が出た時点の performance.now()（このページを開き始めてからの経過ミリ秒）を、描画のフレームごとに確かめる。
    const shownAt = await measuredPage.waitForFunction(
      (count) => document.querySelectorAll('ul[role="list"] > li').length >= count && performance.now(),
      100,
      { polling: 'raf' },
    )
    const elapsedMs = Number(await shownAt.jsonValue())

    // 計測値と 2000ms までの余裕が後から分かるよう、判定の前に annotation と標準出力に残す（失敗したときも残る）。
    const summary = `${elapsedMs.toFixed(1)}ms（上限 2000ms、余裕 ${(2000 - elapsedMs).toFixed(1)}ms）`
    test.info().annotations.push({ type: '基準55 計測値', description: summary })
    console.log(`基準55 計測値: ${summary}`)

    expect(elapsedMs).toBeLessThanOrEqual(2000)
    // 出ていたのは100件（101件目以降は出さない）。
    await expect(notificationItems(measuredPage)).toHaveCount(100)
  } finally {
    await context.close()
  }
})
