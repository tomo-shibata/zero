// カードの並べ替えで使う、マス（slot）の計算（React にも DOM にも依存しない純粋関数。カード表示のプラン 4.4「PR② 並べ替え」）。
// 座標は、マス・ポインターのどちらも、カードの並び（<ul>）の左上からの相対（スクロールや、並びより上の欄の高さに依らないため）。

/** 矩形。マス（ドラッグを始めたときのカードの位置）と、カードの並び（<ul>）の範囲に使う。 */
export type Rect = { left: number; top: number; width: number; height: number }

export type Point = { x: number; y: number }

/** 同じ行かどうかを見るときの、top の差の許容（px）。小数の丸めで同じ行がずれて見えないようにする。 */
const SAME_ROW_TOLERANCE_PX = 1

/**
 * ポインター（指）の置き場所を、何番目のマスかで返す（要件22。判断10）。
 * - カードの並び（list）の矩形の外なら null（離すと取り消し）。
 * - 中なら、ポインターが入っているマス。どのマスにも入っていなければ（カードの間のすき間）、中心がいちばん近いマス。
 * マスがなければ null。
 */
export function slotAtPoint(slots: readonly Rect[], list: Rect, point: Point): number | null {
  if (slots.length === 0 || !contains(list, point)) return null
  const inside = slots.findIndex((slot) => contains(slot, point))
  if (inside >= 0) return inside
  let nearest = 0
  let nearestDistance = Number.POSITIVE_INFINITY
  for (const [index, slot] of slots.entries()) {
    const distance = (centerX(slot) - point.x) ** 2 + (centerY(slot) - point.y) ** 2
    if (distance < nearestDistance) {
      nearest = index
      nearestDistance = distance
    }
  }
  return nearest
}

/**
 * ↑ ↓ で動かす先のマス（要件19。判断9）。current のマスと同じ列（左端の差がマスの幅の半分より小さい）にあり、
 * 上（down なら下）にあるマスのうち、top がいちばん近いもの。なければ null（動かさない）。
 */
export function verticalSlot(slots: readonly Rect[], current: number, direction: 'up' | 'down'): number | null {
  const from = slots[current]
  if (from === undefined) return null
  let best: number | null = null
  let bestGap = Number.POSITIVE_INFINITY
  let bestShift = Number.POSITIVE_INFINITY
  for (const [index, slot] of slots.entries()) {
    const shift = Math.abs(slot.left - from.left)
    if (shift >= from.width / 2) continue
    const gap = direction === 'up' ? from.top - slot.top : slot.top - from.top
    if (gap <= SAME_ROW_TOLERANCE_PX) continue
    if (gap < bestGap || (gap === bestGap && shift < bestShift)) {
      best = index
      bestGap = gap
      bestShift = shift
    }
  }
  return best
}

/**
 * 画面の端での自動スクロールの量（px。上へなら負、下へなら正、スクロールしないなら 0）。
 * ポインターが画面の上端・下端から edge 以内にあるとき、端に近いほど大きくする（最大 maxStep）。
 */
export function edgeScrollDelta(clientY: number, viewportHeight: number, edge = 48, maxStep = 16): number {
  if (clientY < edge) return -Math.ceil((Math.min(edge - clientY, edge) / edge) * maxStep)
  const fromBottom = viewportHeight - clientY
  if (fromBottom < edge) return Math.ceil((Math.min(edge - fromBottom, edge) / edge) * maxStep)
  return 0
}

function contains(rect: Rect, point: Point): boolean {
  return (
    point.x >= rect.left &&
    point.x <= rect.left + rect.width &&
    point.y >= rect.top &&
    point.y <= rect.top + rect.height
  )
}

function centerX(rect: Rect): number {
  return rect.left + rect.width / 2
}

function centerY(rect: Rect): number {
  return rect.top + rect.height / 2
}
