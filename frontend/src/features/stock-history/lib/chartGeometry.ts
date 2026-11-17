import type { ChartModel } from './chartModel'
import { fromDayNumber, toDayNumber } from './dayNumber'

// チャートの座標の計算（純粋関数）。日付は通算の日数、価格は線形に、描く範囲（plot）の座標へ換える（プラン 4.6・4.8）。

/** 描く範囲（軸の文字の内側）の、SVG の中の位置と大きさ。 */
export type PlotBox = { left: number; top: number; width: number; height: number }

export type ChartScales = {
  /** 日付の x 座標。 */
  x: (date: string) => number
  /** 価格（割った値）の y 座標。範囲の外は範囲の端にそろえる。 */
  y: (value: number) => number
}

/** 横軸の目盛りの個数の上限と、目盛り1つに要る幅（px。"YYYY-MM-DD" の文字が重ならない幅）。 */
const MAX_DATE_TICKS = 6
const DATE_TICK_WIDTH_PX = 100

/** 横軸と縦軸の範囲を、描く範囲 box の座標に換える。 */
export function makeScales(model: Pick<ChartModel, 'xRange' | 'yRange'>, box: PlotBox): ChartScales {
  const fromDay = toDayNumber(model.xRange.from)
  const spanDays = toDayNumber(model.xRange.to) - fromDay
  const { min, max } = model.yRange
  return {
    x: (date) => box.left + ((toDayNumber(date) - fromDay) / spanDays) * box.width,
    y: (value) => {
      const ratio = Math.min(1, Math.max(0, (value - min) / (max - min)))
      return box.top + (1 - ratio) * box.height
    },
  }
}

/**
 * 受け取る領域の左右の位置（clientX を、領域の left〜right の中の割合にしたもの）から、横軸の日付を出す。
 * 範囲の外（0 未満・1 超）は端にそろえる。幅が0以下・割合が数でないときは null。
 */
export function dateAtPosition(xRange: ChartModel['xRange'], clientX: number, rect: { left: number; right: number }): string | null {
  const width = rect.right - rect.left
  if (!(width > 0) || !Number.isFinite(clientX)) return null
  const ratio = Math.min(1, Math.max(0, (clientX - rect.left) / width))
  const fromDay = toDayNumber(xRange.from)
  const toDay = toDayNumber(xRange.to)
  return fromDayNumber(Math.round(fromDay + ratio * (toDay - fromDay)))
}

/** 横軸の目盛りの日付。描く範囲の幅に合わせた個数を、日数で等間隔に取る（両端を含む。同じ日は1つにまとめる）。 */
export function dateTicks(xRange: ChartModel['xRange'], plotWidth: number): string[] {
  const fromDay = toDayNumber(xRange.from)
  const toDay = toDayNumber(xRange.to)
  const count = Math.min(MAX_DATE_TICKS, Math.max(2, Math.floor(plotWidth / DATE_TICK_WIDTH_PX)))
  const days = Array.from({ length: count }, (_, index) => Math.round(fromDay + ((toDay - fromDay) * index) / (count - 1)))
  return [...new Set(days)].map(fromDayNumber)
}
