/** 吹き出しを、指す点から離す距離（px）。 */
export const TOOLTIP_OFFSET_PX = 12
/** 吹き出しと箱の端の間に残す余白（px）。 */
export const TOOLTIP_MARGIN_PX = 4

type PlacementInput = {
  /** 指す点（箱の左上からの px）。 */
  anchorX: number
  anchorY: number
  /** 吹き出しの大きさ（px）。 */
  width: number
  height: number
  /** 吹き出しを収める箱の大きさ（px）。 */
  boxWidth: number
  boxHeight: number
}

/**
 * 1つの軸（横か縦）で、吹き出しの始まりの位置を決める。点の後ろ（右・下）に置き、箱に入らなければ前（左・上）に置く。
 * どちらにも入らなければ、余白の位置（始まり）にそろえる。
 */
function place(anchor: number, size: number, boxSize: number): number {
  const max = boxSize - size - TOOLTIP_MARGIN_PX
  const after = anchor + TOOLTIP_OFFSET_PX
  if (after <= max) return after
  const before = anchor - TOOLTIP_OFFSET_PX - size
  if (before >= TOOLTIP_MARGIN_PX) return before
  // 前に置くと左（上）の端を越える。後ろにも前にも入らないので、左（上）の余白にそろえる（箱より大きいときも同じ）。
  return TOOLTIP_MARGIN_PX
}

/**
 * 吹き出しの左上の位置（箱の左上からの px）を決める。点から12px 離し、箱の端に4px の余白を残して、箱の中に収める。
 * 横と縦は別々に決める。
 */
export function placeTooltip({ anchorX, anchorY, width, height, boxWidth, boxHeight }: PlacementInput): { left: number; top: number } {
  return { left: place(anchorX, width, boxWidth), top: place(anchorY, height, boxHeight) }
}
