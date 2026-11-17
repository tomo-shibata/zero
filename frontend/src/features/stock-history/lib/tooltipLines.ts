import type { StockHistoryClose, StockHistoryTrade } from '../../../entities/stock-history/model'
import type { Currency } from '../../../shared/lib/currency'
import { formatShares, formatUnitPrice } from '../../../shared/lib/format'

// チャートの吹き出しに出す文（銘柄ごとの売買履歴の要件10・要件11）。

const SIDE_LABELS: Record<StockHistoryTrade['side'], string> = { buy: '買', sell: '売' }
const ADJUSTED_PRICE_LABEL = '分割を反映した価格'
/** 割る数がこの値なら、分割を反映していない（割る数の積が1）。 */
const NO_SPLIT_DIVISOR = '1'

/** 線の吹き出し: 「<取引日> <終値>」。終値は、分割で割った価格（API が丸めて返す adjustedPrice）で出す。 */
export function closeTooltip(currency: Currency, close: StockHistoryClose): string {
  return `${close.date} ${formatUnitPrice(currency, close.adjustedPrice)}`
}

/**
 * 印の吹き出し: 渡した順に「<取引日> <買 か 売> <株数> <価格>」。株数と価格は表と同じ実際の値。
 * 分割で割った売買（割る数が 1 でない）は、続けて「分割を反映した価格 <割った価格>」の行も出す。
 */
export function tradeTooltipLines(currency: Currency, trades: readonly StockHistoryTrade[]): string[] {
  return trades.flatMap((trade) => {
    const line = `${trade.date} ${SIDE_LABELS[trade.side]} ${formatShares(trade.quantity)} ${formatUnitPrice(currency, trade.price)}`
    if (trade.splitDivisor === NO_SPLIT_DIVISOR) return [line]
    return [line, `${ADJUSTED_PRICE_LABEL} ${formatUnitPrice(currency, trade.adjustedPrice)}`]
  })
}
