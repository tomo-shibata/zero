import type { StockHistory, StockHistoryClose, StockHistoryTrade } from '../../../entities/stock-history/model'
import { fromDayNumber, toDayNumber } from './dayNumber'

// 売買履歴から、チャートに描く点・印・範囲を出す純粋関数（銘柄ごとの売買履歴の要件6〜要件9・要件12。プラン 4.6・4.8）。

/** 線・印の点。value は 価格 ÷ 割る数（丸めない。図の座標だけに使う。吹き出しの文字には使わない）。 */
export type ChartPoint = { date: string; value: number }
/** 線の点。吹き出しに要る元の終値を持つ。 */
export type ChartLinePoint = ChartPoint & { close: StockHistoryClose }
/** 売買の印。吹き出しに要る元の売買を持つ。 */
export type ChartMarker = ChartPoint & { side: 'buy' | 'sell'; trade: StockHistoryTrade }

export type ChartModel = {
  /** 横軸の範囲。from < to を保証する（同じ日だけなら前後に1日ずつ広げる）。 */
  xRange: { from: string; to: string }
  /** 取引日の昇順（API が返す並びのまま）。 */
  line: ChartLinePoint[]
  /** 売買1件に1つ。entries と同じ順（新しい順。同じ日は表と同じ順）。 */
  markers: ChartMarker[]
  /** 線と印がすべて入る範囲。min < max で、どちらも有限。 */
  yRange: { min: number; max: number }
  /** 線の点が0件（要件12）。 */
  hasNoPrices: boolean
}

/** 縦軸の範囲の、上下の余白（値の幅に対する割合。端の印が切れないように）。 */
const Y_PADDING_RATIO = 0.05
/** 値がすべて同じとき、上下に広げる幅（その値に対する割合）。 */
const FLAT_Y_SPREAD_RATIO = 0.1
/** 値がすべて 0 のとき、上下に広げる幅。 */
const FLAT_ZERO_SPREAD = 1

/** 割る数は桁が大きくなりうる（2^53 を超えうる整数の文字列）ので、Number にするのは座標の計算のここだけにする。 */
function adjustedValue(price: string, splitDivisor: string): number {
  return Number(price) / Number(splitDivisor)
}

/**
 * 売買履歴（API の応答）から、チャートのモデルを作る。history.entries には売買が1件以上ある（API は売買がなければ 404 にする）。
 * history.closes は、最初の売買の取引日と同じかそれより後のものだけ（API が絞る。ここでは絞らない）。
 */
export function buildChartModel(history: StockHistory): ChartModel {
  const trades = history.entries.filter((entry): entry is StockHistoryTrade => entry.kind === 'trade')
  const line: ChartLinePoint[] = history.closes.map((close) => ({
    date: close.date,
    value: adjustedValue(close.price, close.splitDivisor),
    close,
  }))
  const markers: ChartMarker[] = trades.map((trade) => ({
    date: trade.date,
    value: adjustedValue(trade.price, trade.splitDivisor),
    side: trade.side,
    trade,
  }))
  return { xRange: xRangeOf(trades, history.closes), line, markers, yRange: yRangeOf(line, markers), hasNoPrices: line.length === 0 }
}

/** 横軸: 最初の売買の取引日から、最後の売買の取引日と最後の終値の日の遅いほうまで（分割の行の日は使わない。要件6）。 */
function xRangeOf(trades: readonly StockHistoryTrade[], closes: readonly StockHistoryClose[]): { from: string; to: string } {
  // ISO 形式の日付は、文字列の順が日付の順と同じ。
  const tradeDates = trades.map((trade) => trade.date).sort()
  const from = tradeDates[0]
  const lastTradeDate = tradeDates[tradeDates.length - 1]
  const lastCloseDate = closes.length > 0 ? closes[closes.length - 1].date : lastTradeDate
  const to = lastCloseDate > lastTradeDate ? lastCloseDate : lastTradeDate
  if (from !== to) return { from, to }
  // 1日だけ: 描くとき 0 で割らないよう、前後に1日ずつ広げる。
  const day = toDayNumber(from)
  return { from: fromDayNumber(day - 1), to: fromDayNumber(day + 1) }
}

/** 縦軸: 線と印の最小・最大に、上下の余白を足す。すべて同じ値なら上下に広げる。価格は負にならないので、下は 0 までにする。 */
function yRangeOf(line: readonly ChartLinePoint[], markers: readonly ChartMarker[]): { min: number; max: number } {
  const values = [...line.map((point) => point.value), ...markers.map((marker) => marker.value)]
  let min = values[0]
  let max = values[0]
  for (const value of values) {
    if (value < min) min = value
    if (value > max) max = value
  }
  const spread = max > min ? (max - min) * Y_PADDING_RATIO : max === 0 ? FLAT_ZERO_SPREAD : Math.abs(max) * FLAT_Y_SPREAD_RATIO
  return { min: Math.max(0, min - spread), max: max + spread }
}
