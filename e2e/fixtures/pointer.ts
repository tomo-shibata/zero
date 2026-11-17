import type { Locator, Page } from '@playwright/test'

// マウスとタッチの操作の補助（E2E。holdings-reorder.spec.ts・stock-history.spec.ts が使う）。
// 仕様: 銘柄ごとの売買履歴の画面のプラン docs/plans/stock-history/plan.md 5.3 の「e2e/fixtures/」の行
//   （holdings-reorder.spec.ts の中にあった centerOf・mouseDragTo と、タッチの補助 touchSession・touchMoveSteps を移した）。
// タッチは Chromium のタッチの模擬（CDP の Input.dispatchTouchEvent）で送る。touchStart・touchMove・touchEnd で画面のスクロールが起きることは確かめてある
//   （カード表示のステップ4）。タップ（touchStart と touchEnd）の後には、互換の mousedown・mouseup・click が送られる
//   （銘柄ごとの売買履歴の画面のステップ4 で確かめた。touchend を preventDefault すると送られない）。

/** locator の中心の座標。 */
export async function centerOf(locator: Locator): Promise<{ x: number; y: number }> {
  const box = await locator.boundingBox()
  if (box === null) throw new Error('要素の位置を取れない')
  return { x: box.x + box.width / 2, y: box.y + box.height / 2 }
}

/** マウスで from のカードの中心を押し、to の中心まで少しずつ動かす（離さない）。 */
export async function mouseDragTo(page: Page, from: Locator, to: { x: number; y: number }): Promise<void> {
  const start = await centerOf(from)
  await page.mouse.move(start.x, start.y)
  await page.mouse.down()
  await page.mouse.move(to.x, to.y, { steps: 20 })
}

/** CDP でタッチを送る補助（hasTouch のコンテキストで使う）。 */
export async function touchSession(page: Page) {
  const cdp = await page.context().newCDPSession(page)
  return {
    start: (x: number, y: number) => cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x, y }] }),
    move: (x: number, y: number) => cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x, y }] }),
    end: () => cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] }),
  }
}

/** (from) から (to) まで、steps 回に分けて touchMove する（各回の間に intervalMs 待つ）。 */
export async function touchMoveSteps(
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
