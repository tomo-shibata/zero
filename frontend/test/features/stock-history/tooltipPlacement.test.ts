import { describe, expect, test } from 'vitest'
import { placeTooltip } from '../../../src/features/stock-history/lib/tooltipPlacement'

// 仕様: 要件 docs/requirements/stock-history.md の要件5（画面全体が横にスクロールしない）・要件10・要件11（吹き出し）。
//   ステップ6 1ラウンド目のデザイン1・エッジケース4（吹き出しがチャートの右・下からはみ出す）。
// 境界: 依頼で決めた純粋関数の境界（docs/plans/stock-history/reviews/pr2-step6-round1.md の「優先度1」デザイン1）
//   placeTooltip({ anchorX, anchorY, width, height, boxWidth, boxHeight }): { left, top }。座標はどれも CSS の px で、箱の左上が原点。
//   - 点から 12px 離し、4px の余白を残す。
//   - 横: 右（anchorX + 12）に置く。右端（boxWidth - 4）を越えるなら左（anchorX - 12 - width）に置く。それでも左端（4）より左なら
//     4 から boxWidth - width - 4 の範囲に丸める。
//   - 縦: 下（anchorY + 12）に置く。下端（boxHeight - 4）を越えるなら上（anchorY - 12 - height）に置く。それでも上端（4）より上なら同じように丸める。
//   - 吹き出しが箱より大きいときは left / top を 4 にする。
// 数はどれも手で計算した値（吹き出し 100x40、箱 300x200 を基本にする）。

const BASE = { anchorX: 0, anchorY: 0, width: 100, height: 40, boxWidth: 300, boxHeight: 200 }

describe('横', () => {
  test('右に入るなら、点の 12px 右（anchorX 50 なら left 62。右端は 62+100=162 で 296 以内）', () => {
    expect(placeTooltip({ ...BASE, anchorX: 50, anchorY: 50 }).left).toBe(62)
  })

  test('右端がちょうど boxWidth - 4（296）になるなら右に置く（anchorX 184 なら left 196。196+100=296）', () => {
    expect(placeTooltip({ ...BASE, anchorX: 184, anchorY: 50 }).left).toBe(196)
  })

  test('右端が 1px 越える（anchorX 185。197+100=297 > 296）なら左に置く（left = 185-12-100 = 73）', () => {
    expect(placeTooltip({ ...BASE, anchorX: 185, anchorY: 50 }).left).toBe(73)
  })

  test('右に入らないなら、点の 12px 左に吹き出しの右端が来る（anchorX 250 なら left 138）', () => {
    expect(placeTooltip({ ...BASE, anchorX: 250, anchorY: 50 }).left).toBe(138)
  })

  test('右にも左にも入らないときは、左の余白 4 に丸める（箱の幅 150、anchorX 60 なら右は 72+100=172 > 146、左は -52 なので 4）', () => {
    expect(placeTooltip({ ...BASE, boxWidth: 150, anchorX: 60, anchorY: 50 }).left).toBe(4)
  })

  test('左に置いた吹き出しの左端がちょうど 4 になるなら、そのまま 4（anchorX 116 なら 116-12-100=4。右は 128+100=228 > 146）', () => {
    expect(placeTooltip({ ...BASE, boxWidth: 150, anchorX: 116, anchorY: 50 }).left).toBe(4)
  })

  test('吹き出しが箱より広いとき（幅 400、箱 300）は left が 4', () => {
    expect(placeTooltip({ ...BASE, width: 400, anchorX: 50, anchorY: 50 }).left).toBe(4)
    expect(placeTooltip({ ...BASE, width: 400, anchorX: 250, anchorY: 50 }).left).toBe(4)
  })
})

describe('縦', () => {
  test('下に入るなら、点の 12px 下（anchorY 50 なら top 62。下端は 62+40=102 で 196 以内）', () => {
    expect(placeTooltip({ ...BASE, anchorX: 50, anchorY: 50 }).top).toBe(62)
  })

  test('下端がちょうど boxHeight - 4（196）になるなら下に置く（anchorY 144 なら top 156。156+40=196）', () => {
    expect(placeTooltip({ ...BASE, anchorX: 50, anchorY: 144 }).top).toBe(156)
  })

  test('下端が 1px 越える（anchorY 145。157+40=197 > 196）なら上に置く（top = 145-12-40 = 93）', () => {
    expect(placeTooltip({ ...BASE, anchorX: 50, anchorY: 145 }).top).toBe(93)
  })

  test('下に入らないなら、点の 12px 上に吹き出しの下端が来る（anchorY 170 なら top 118）', () => {
    expect(placeTooltip({ ...BASE, anchorX: 50, anchorY: 170 }).top).toBe(118)
  })

  test('下にも上にも入らないときは、上の余白 4 に丸める（箱の高さ 60、anchorY 30 なら下は 42+40=82 > 56、上は -22 なので 4）', () => {
    expect(placeTooltip({ ...BASE, boxHeight: 60, anchorX: 50, anchorY: 30 }).top).toBe(4)
  })

  test('吹き出しが箱より高いとき（高さ 300、箱 200）は top が 4', () => {
    expect(placeTooltip({ ...BASE, height: 300, anchorX: 50, anchorY: 50 }).top).toBe(4)
    expect(placeTooltip({ ...BASE, height: 300, anchorX: 50, anchorY: 170 }).top).toBe(4)
  })
})

describe('横と縦は別々に決まる', () => {
  test('右下の隅の点（anchorX 280、anchorY 180）では、左上に出す（left 168、top 128）', () => {
    expect(placeTooltip({ ...BASE, anchorX: 280, anchorY: 180 })).toEqual({ left: 168, top: 128 })
  })

  test('左上の隅の点（anchorX 10、anchorY 10）では、右下に出す（left 22、top 22）', () => {
    expect(placeTooltip({ ...BASE, anchorX: 10, anchorY: 10 })).toEqual({ left: 22, top: 22 })
  })

  test('右上の隅の点（anchorX 280、anchorY 10）では、左下に出す（left 168、top 22）', () => {
    expect(placeTooltip({ ...BASE, anchorX: 280, anchorY: 10 })).toEqual({ left: 168, top: 22 })
  })

  test('吹き出しの大きさ・箱の大きさを変えても結果は動かない（箱 390x300、吹き出し 150x60、点 (350, 250) なら left 188、top 178）', () => {
    expect(placeTooltip({ anchorX: 350, anchorY: 250, width: 150, height: 60, boxWidth: 390, boxHeight: 300 })).toEqual({
      left: 188,
      top: 178,
    })
  })
})
