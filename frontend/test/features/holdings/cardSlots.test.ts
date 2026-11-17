import { describe, expect, test } from 'vitest'
import { edgeScrollDelta, slotAtPoint, verticalSlot, type Rect } from '../../../src/features/holdings/lib/cardSlots'

// 仕様: カード表示の要件 docs/requirements/holdings-cards.md の要件19（↑ ↓ で上下のカードへ動かす）・要件22（置き場所は離した所のカード。
//   カードの外で離したら取り消し）、基準22。
// 境界・決まり: プラン docs/plans/holdings-cards/plan.md 4.4「PR② 並べ替え」の「ポインターの置き場所」「キーボード」「画面の端での自動スクロール」と、
//   判断9・10。座標はすべてページの座標。
//   - slotAtPoint: ポインターが <ul> の矩形の外なら置き場所なし。中なら、入っているマス。どのマスにも入っていなければ中心がいちばん近いマス。
//   - verticalSlot: 左端の差がマスの幅の半分より小さいマスのうち、上（下）にあって top がいちばん近いもの。なければ動かない。
//   - edgeScrollDelta: 画面の上端・下端から 48px 以内で上（負）・下（正）にスクロール。それ以外は 0。
// テストの名前の先頭は、プランの対応する決まり（slotAtPoint: 判断10、verticalSlot: 判断9、edgeScrollDelta: 4.4）。

/** 列 col・行 row のマス（幅 300・高さ 100、列の間 20・行の間 20）。 */
function cell(col: number, row: number): Rect {
  return { left: col * 320, top: row * 120, width: 300, height: 100 }
}

// 3列×2行。マスの番号は 0 1 2 / 3 4 5。<ul> は全体を含む（0,0 から 940×220）。
const GRID_3X2: Rect[] = [cell(0, 0), cell(1, 0), cell(2, 0), cell(0, 1), cell(1, 1), cell(2, 1)]
const GRID_3X2_LIST: Rect = { left: 0, top: 0, width: 940, height: 220 }

describe('slotAtPoint', () => {
  test('カード表示の補助 判断10: <ul> の矩形の外なら、どの方向でも置き場所なし（null）', () => {
    expect(slotAtPoint(GRID_3X2, GRID_3X2_LIST, { x: -1, y: 50 })).toBeNull()
    expect(slotAtPoint(GRID_3X2, GRID_3X2_LIST, { x: 941, y: 50 })).toBeNull()
    expect(slotAtPoint(GRID_3X2, GRID_3X2_LIST, { x: 150, y: -1 })).toBeNull()
    expect(slotAtPoint(GRID_3X2, GRID_3X2_LIST, { x: 150, y: 221 })).toBeNull()
  })

  test('カード表示の補助 判断10: <ul> の外なら、いちばん近いマスがあっても null（外のときは近いマスにしない）', () => {
    // マス 2 の右の外側。マス 2 がいちばん近いが、<ul> の外なので置き場所なし。
    expect(slotAtPoint(GRID_3X2, GRID_3X2_LIST, { x: 1000, y: 50 })).toBeNull()
  })

  test('カード表示の補助 判断10: どれかのマスに入っていれば、そのマス', () => {
    expect(slotAtPoint(GRID_3X2, GRID_3X2_LIST, { x: 150, y: 50 })).toBe(0)
    expect(slotAtPoint(GRID_3X2, GRID_3X2_LIST, { x: 470, y: 50 })).toBe(1)
    expect(slotAtPoint(GRID_3X2, GRID_3X2_LIST, { x: 790, y: 50 })).toBe(2)
    expect(slotAtPoint(GRID_3X2, GRID_3X2_LIST, { x: 150, y: 170 })).toBe(3)
    expect(slotAtPoint(GRID_3X2, GRID_3X2_LIST, { x: 470, y: 170 })).toBe(4)
    expect(slotAtPoint(GRID_3X2, GRID_3X2_LIST, { x: 790, y: 170 })).toBe(5)
  })

  test('カード表示の補助 判断10: カードの間の横のすき間では、中心がいちばん近いマス', () => {
    // マス 0（x 0-300）とマス 1（x 320-620）の間。中心の x は 150 と 470。
    expect(slotAtPoint(GRID_3X2, GRID_3X2_LIST, { x: 305, y: 50 })).toBe(0)
    expect(slotAtPoint(GRID_3X2, GRID_3X2_LIST, { x: 315, y: 50 })).toBe(1)
  })

  test('カード表示の補助 判断10: カードの間の縦のすき間では、中心がいちばん近いマス', () => {
    // マス 0（y 0-100）とマス 3（y 120-220）の間。中心の y は 50 と 170。
    expect(slotAtPoint(GRID_3X2, GRID_3X2_LIST, { x: 150, y: 105 })).toBe(0)
    expect(slotAtPoint(GRID_3X2, GRID_3X2_LIST, { x: 150, y: 115 })).toBe(3)
  })

  test('カード表示の補助 判断10: 4つのマスの角の間（横にも縦にもすき間）でも、中心がいちばん近いマス', () => {
    // マス 0・1・3・4 の真ん中の点 (310, 110) からは、4つの中心はどれも同じ遠さ。そこから少し左上・右下へずらす。
    expect(slotAtPoint(GRID_3X2, GRID_3X2_LIST, { x: 308, y: 108 })).toBe(0)
    expect(slotAtPoint(GRID_3X2, GRID_3X2_LIST, { x: 312, y: 112 })).toBe(4)
  })

  test('カード表示の補助 判断10: <ul> の中で、どのマスからも離れている所（<ul> の余白）でも、中心がいちばん近いマス', () => {
    const list: Rect = { left: 0, top: 0, width: 1000, height: 300 }
    // 右の余白（マス 2 の右。x 620-940 のうち 940 より右）と、下の余白。
    expect(slotAtPoint(GRID_3X2, list, { x: 990, y: 50 })).toBe(2)
    expect(slotAtPoint(GRID_3X2, list, { x: 150, y: 290 })).toBe(3)
  })

  test('カード表示の補助 判断10: 境界の点: マスのちょうど端（左上・右下の角）はそのマスの中', () => {
    expect(slotAtPoint(GRID_3X2, GRID_3X2_LIST, { x: 320, y: 0 })).toBe(1)
    expect(slotAtPoint(GRID_3X2, GRID_3X2_LIST, { x: 300, y: 100 })).toBe(0)
    expect(slotAtPoint(GRID_3X2, GRID_3X2_LIST, { x: 640, y: 120 })).toBe(5)
  })

  test('カード表示の補助 判断10: 境界の点: <ul> のちょうど端は、中', () => {
    expect(slotAtPoint(GRID_3X2, GRID_3X2_LIST, { x: 0, y: 0 })).toBe(0)
    expect(slotAtPoint(GRID_3X2, GRID_3X2_LIST, { x: 940, y: 220 })).toBe(5)
  })

  test('カード表示の補助 判断10: マスが1つのとき: 中なら 0、<ul> の中のすき間でも 0、<ul> の外なら null', () => {
    const slots: Rect[] = [{ left: 10, top: 10, width: 200, height: 100 }]
    const list: Rect = { left: 0, top: 0, width: 300, height: 200 }
    expect(slotAtPoint(slots, list, { x: 100, y: 50 })).toBe(0)
    expect(slotAtPoint(slots, list, { x: 250, y: 180 })).toBe(0)
    expect(slotAtPoint(slots, list, { x: 301, y: 50 })).toBeNull()
  })

  test('カード表示の補助 判断10: マスがなければ null', () => {
    expect(slotAtPoint([], { left: 0, top: 0, width: 300, height: 200 }, { x: 10, y: 10 })).toBeNull()
  })

  test('カード表示の補助 判断10: ページの座標（スクロールした後の大きな値）でも同じに決まる', () => {
    const offset = (rect: Rect): Rect => ({ ...rect, top: rect.top + 5000 })
    const slots = GRID_3X2.map(offset)
    const list = offset(GRID_3X2_LIST)
    expect(slotAtPoint(slots, list, { x: 470, y: 5170 })).toBe(4)
    expect(slotAtPoint(slots, list, { x: 470, y: 170 })).toBeNull()
  })
})

describe('verticalSlot', () => {
  test('カード表示の補助 判断9: 3列×2行: 2行目の2列目から ↑ で、1行目の2列目', () => {
    expect(verticalSlot(GRID_3X2, 4, 'up')).toBe(1)
  })

  test('カード表示の補助 判断9: 3列×2行: 1行目の各列から ↓ で、2行目の同じ列', () => {
    expect(verticalSlot(GRID_3X2, 0, 'down')).toBe(3)
    expect(verticalSlot(GRID_3X2, 1, 'down')).toBe(4)
    expect(verticalSlot(GRID_3X2, 2, 'down')).toBe(5)
  })

  test('カード表示の補助 判断9: 3列×2行: 2行目の各列から ↑ で、1行目の同じ列', () => {
    expect(verticalSlot(GRID_3X2, 3, 'up')).toBe(0)
    expect(verticalSlot(GRID_3X2, 5, 'up')).toBe(2)
  })

  test('カード表示の補助 判断9: 3列×2行: 1行目から ↑・2行目から ↓ は動かない（null）', () => {
    expect(verticalSlot(GRID_3X2, 0, 'up')).toBeNull()
    expect(verticalSlot(GRID_3X2, 1, 'up')).toBeNull()
    expect(verticalSlot(GRID_3X2, 2, 'up')).toBeNull()
    expect(verticalSlot(GRID_3X2, 3, 'down')).toBeNull()
    expect(verticalSlot(GRID_3X2, 4, 'down')).toBeNull()
    expect(verticalSlot(GRID_3X2, 5, 'down')).toBeNull()
  })

  test('カード表示の補助 判断9: 5枚（3枚＋2枚）: 2行目が2枚のとき、3列目から ↓ は下にマスがないので動かない', () => {
    const slots = [cell(0, 0), cell(1, 0), cell(2, 0), cell(0, 1), cell(1, 1)]
    expect(verticalSlot(slots, 2, 'down')).toBeNull()
    expect(verticalSlot(slots, 1, 'down')).toBe(4)
    expect(verticalSlot(slots, 0, 'down')).toBe(3)
  })

  test('カード表示の補助 判断9: 5枚（3枚＋2枚）: 2行目の2枚から ↑ で、1行目の同じ列', () => {
    const slots = [cell(0, 0), cell(1, 0), cell(2, 0), cell(0, 1), cell(1, 1)]
    expect(verticalSlot(slots, 3, 'up')).toBe(0)
    expect(verticalSlot(slots, 4, 'up')).toBe(1)
  })

  test('カード表示の補助 判断9: 1列: ↑ ↓ で1つ上・1つ下。先頭の ↑・末尾の ↓ は動かない', () => {
    const slots = [cell(0, 0), cell(0, 1), cell(0, 2)]
    expect(verticalSlot(slots, 0, 'down')).toBe(1)
    expect(verticalSlot(slots, 1, 'down')).toBe(2)
    expect(verticalSlot(slots, 2, 'up')).toBe(1)
    expect(verticalSlot(slots, 1, 'up')).toBe(0)
    expect(verticalSlot(slots, 0, 'up')).toBeNull()
    expect(verticalSlot(slots, 2, 'down')).toBeNull()
  })

  test('カード表示の補助 判断9: 同じ列のマスが複数あるときは、top がいちばん近いもの（2行以上離れたものは選ばない）', () => {
    const slots = [cell(0, 0), cell(0, 1), cell(0, 2), cell(0, 3)]
    expect(verticalSlot(slots, 0, 'down')).toBe(1)
    expect(verticalSlot(slots, 3, 'up')).toBe(2)
  })

  test('カード表示の補助 判断9: 列の判定: 左端の差がマスの幅の半分（150）より小さければ同じ列、半分以上なら別の列', () => {
    const current: Rect = { left: 0, top: 0, width: 300, height: 100 }
    const sameColumn: Rect = { left: 149, top: 120, width: 300, height: 100 }
    const otherColumn: Rect = { left: 150, top: 120, width: 300, height: 100 }
    expect(verticalSlot([current, sameColumn], 0, 'down')).toBe(1)
    expect(verticalSlot([current, otherColumn], 0, 'down')).toBeNull()
  })

  test('カード表示の補助 判断9: 列の判定: 左側にずれたマスも、差が半分より小さければ同じ列', () => {
    const current: Rect = { left: 300, top: 120, width: 300, height: 100 }
    const sameColumn: Rect = { left: 151, top: 0, width: 300, height: 100 }
    const otherColumn: Rect = { left: 150, top: 0, width: 300, height: 100 }
    expect(verticalSlot([sameColumn, current], 1, 'up')).toBe(0)
    expect(verticalSlot([otherColumn, current], 1, 'up')).toBeNull()
  })

  test('カード表示の補助 判断9: 別の列のマスは、上にあっても選ばない（同じ列に上のマスがなければ動かない）', () => {
    // 1行目は左の列だけ。2行目の右の列から ↑ は、同じ列に上のマスがないので動かない。
    const slots = [cell(0, 0), cell(1, 1)]
    expect(verticalSlot(slots, 1, 'up')).toBeNull()
  })
})

describe('edgeScrollDelta', () => {
  const VIEWPORT = 800

  test('カード表示の補助 4.4: 画面の上端から 48px 以内なら、上へ（負）', () => {
    expect(edgeScrollDelta(0, VIEWPORT)).toBeLessThan(0)
    expect(edgeScrollDelta(10, VIEWPORT)).toBeLessThan(0)
    expect(edgeScrollDelta(47, VIEWPORT)).toBeLessThan(0)
  })

  test('カード表示の補助 4.4: 画面の下端から 48px 以内なら、下へ（正）', () => {
    expect(edgeScrollDelta(VIEWPORT, VIEWPORT)).toBeGreaterThan(0)
    expect(edgeScrollDelta(VIEWPORT - 10, VIEWPORT)).toBeGreaterThan(0)
    expect(edgeScrollDelta(VIEWPORT - 47, VIEWPORT)).toBeGreaterThan(0)
  })

  test('カード表示の補助 4.4: 端から 48px より内側（真ん中、ちょうど 48px の位置、その少し内側）は 0', () => {
    expect(edgeScrollDelta(VIEWPORT / 2, VIEWPORT)).toBe(0)
    expect(edgeScrollDelta(48, VIEWPORT)).toBe(0)
    expect(edgeScrollDelta(49, VIEWPORT)).toBe(0)
    expect(edgeScrollDelta(VIEWPORT - 48, VIEWPORT)).toBe(0)
    expect(edgeScrollDelta(VIEWPORT - 49, VIEWPORT)).toBe(0)
  })

  test('カード表示の補助 4.4: 端に近いほど、スクロールの量は大きい（同じ向きで、減らない）', () => {
    expect(Math.abs(edgeScrollDelta(0, VIEWPORT))).toBeGreaterThanOrEqual(Math.abs(edgeScrollDelta(24, VIEWPORT)))
    expect(Math.abs(edgeScrollDelta(24, VIEWPORT))).toBeGreaterThanOrEqual(Math.abs(edgeScrollDelta(47, VIEWPORT)))
    expect(Math.abs(edgeScrollDelta(0, VIEWPORT))).toBeGreaterThan(Math.abs(edgeScrollDelta(47, VIEWPORT)))
    expect(edgeScrollDelta(VIEWPORT, VIEWPORT)).toBeGreaterThan(edgeScrollDelta(VIEWPORT - 47, VIEWPORT))
  })

  test('カード表示の補助 4.4: 上端のそばと下端のそばで、同じ近さなら量は同じで向きが逆', () => {
    expect(edgeScrollDelta(10, VIEWPORT)).toBe(-edgeScrollDelta(VIEWPORT - 10, VIEWPORT))
  })

  test('カード表示の補助 4.4: 端のそばなら量は 0 にならない（1px 以上動く）', () => {
    expect(Math.abs(edgeScrollDelta(47, VIEWPORT))).toBeGreaterThanOrEqual(1)
    expect(Math.abs(edgeScrollDelta(VIEWPORT - 47, VIEWPORT))).toBeGreaterThanOrEqual(1)
  })

  test('カード表示の補助 4.4: 画面が低くて上端の帯と下端の帯が重ならない高さ（96px 以上）でも、真ん中は 0', () => {
    expect(edgeScrollDelta(60, 120)).toBe(0)
  })
})
