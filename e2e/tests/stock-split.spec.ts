import { expect, test } from '@playwright/test'
import {
  insertHoldingsData,
  resetHoldingsData,
  type SplitSeed,
  type StockPriceSeed,
  type StockSeed,
  type TradeSeed,
} from '../fixtures/db'
import {
  SUMMARY_LABEL,
  expectCardData,
  holdingCard,
  holdingCards,
} from '../fixtures/holdings-page'

// 仕様: 株式分割の要件 docs/requirements/stock-split.md の基準8（要件4・要件8）と基準22（非機能1）。
// テスト計画: プラン docs/plans/stock-split/plan.md 5.1 の基準8・基準22 の行（PR①）、5.3 の e2e/fixtures/db.ts の行。
// 実行の前提: holdings.spec.ts と同じ（zero_e2e、API は利用者A で起動、beforeEach でデータを入れ替える）。
// 分割は stock_splits に直接入れる（fixtures/db.ts の HoldingsTestData.splits）。

// 1件失敗しても後続を止めずに、順に実行する（同じ DB を使うため）。
test.describe.configure({ mode: 'default' })

// 各テストの前に、売買履歴・終値・並び・分割と test.sql にない銘柄マスタを消す。
test.beforeEach(async () => {
  await resetHoldingsData()
})

const HOLDINGS_PATH = '/holdings'

test('基準8（要件4・要件8）: 7203 の分割が「1→3 2026-03-31」、売買履歴が「買 2,000円 100株 2026-01-10」、7203 の価格が保存済み（3,000円・2026-10-06）のとき、/holdings の 7203 のカードに保有数量「300株」・取得価格「666.7円」が表示され、評価額は 900,000円になる', async ({ page }) => {
  // Given: 分割は全利用者で共有する。Aの売買履歴は買い1件。価格は分割の日より後の取引日（2026-10-06）なので、要件8 で価格なしにならない。
  const split: SplitSeed = { code: '7203', splitDate: '2026-03-31', ratio: 3 }
  const trade: TradeSeed = { code: '7203', side: 'buy', quantity: 100, price: '2000.0', tradeDate: '2026-01-10' }
  const price: StockPriceSeed = { code: '7203', tradingDate: '2026-10-06', closePrice: '3000.0' }
  await insertHoldingsData({ trades: [trade], prices: [price], splits: [split] })

  await page.goto(HOLDINGS_PATH)

  // 7203 のカード: 保有数量 300株、取得価格 666.7円（2,000円 ÷ 3 = 666.66…円を四捨五入）、現在の価格 3,000円、基準日 2026-10-06、評価額 900,000円（3,000円 × 300株）。
  await expectCardData(holdingCard(page, '7203'), [
    '7203',
    'トヨタ自動車',
    '300株',
    '666.7円',
    '3,000円',
    '2026-10-06',
    '900,000円',
  ])
})

test('基準22（非機能1）: 100銘柄（円50・ドル50。すべて価格が保存済み）を売買履歴各20件・分割各2件（1→2）で保有しているとき、/holdings を開いてから2秒以内にカードが100枚表示される（計測値を annotation と console.log に残す）', async ({ page, browser }) => {
  // Given: 利用者Aが100銘柄（円50・ドル50）を売買履歴各20件（合計2,000件）で保有し、各銘柄に分割が2件（合計200件）あり、すべて価格が保存済み。
  // 売買履歴の作り方は holdings.spec.ts の基準31 と同じ（買い16件・売り4件。取引日は 2026-02-01〜2026-02-20）。
  // 分割の日は2件とも終値の取引日（2026-10-06）より前にする（そうしないと要件8 で全銘柄が価格なしになり、
  // 評価額と含み損益を計算しない道を測ってしまう）。比率は 1→2。分割は取引日の間（2026-02-10）と後（2026-03-01）に1件ずつ置く
  // （どちらも保有数量は増えるだけなので、売りで保有数量が負にならない）。
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
  const splitsOf = (code: string): SplitSeed[] => [
    { code, splitDate: '2026-02-10', ratio: 2 },
    { code, splitDate: '2026-03-01', ratio: 2 },
  ]
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
    splits: allCodes.flatMap((code) => splitsOf(code)),
  })

  // ウォームアップ: 一度 /holdings を開いて捨てる（dev サーバーの初回の変換などを計測に含めない）。
  await page.goto(HOLDINGS_PATH)
  await expect(holdingCards(page)).toHaveCount(allCodes.length)
  await page.close()

  // 計測: 新しいブラウザコンテキストで開き直す（ウォームアップのページのキャッシュを使わない）。
  const context = await browser.newContext()
  try {
    const measuredPage = await context.newPage()
    await measuredPage.goto(HOLDINGS_PATH)
    // カードが100枚そろった時点の performance.now()（このページを開き始めてからの経過ミリ秒）を、描画のフレームごとに確かめる。
    const shownAt = await measuredPage.waitForFunction(
      (cardCount) =>
        document.querySelectorAll('ul[aria-label="保有株式"] > li').length >= cardCount && performance.now(),
      allCodes.length,
      { polling: 'raf' },
    )
    const elapsedMs = await shownAt.jsonValue()

    // 計測値と 2000ms までの余裕が後から分かるよう、判定の前に annotation と標準出力に残す（失敗したときも残る）。
    const measuredMs = Number(elapsedMs)
    const summary = `${measuredMs.toFixed(1)}ms（上限 2000ms、余裕 ${(2000 - measuredMs).toFixed(1)}ms）`
    test.info().annotations.push({ type: '基準22 計測値', description: summary })
    console.log(`基準22 計測値: ${summary}`)

    expect(elapsedMs).toBeLessThanOrEqual(2000)
    // 出ていたのは入れた100銘柄の一覧（カードが余分に出ていない）。合計の欄に円とドルの部分が1つずつ出ている。
    await expect(holdingCards(measuredPage)).toHaveCount(allCodes.length)
    await expect(measuredPage.getByRole('region', { name: SUMMARY_LABEL, exact: true }).getByRole('region')).toHaveCount(2)
  } finally {
    await context.close()
  }
})
