import type { StockHistoryEntry } from '../../../entities/stock-history/model'
import type { Currency } from '../../../shared/lib/currency'
import { formatShares, formatUnitPrice } from '../../../shared/lib/format'
import './TradeHistoryTable.css'

const COLUMN_LABELS = ['取引日', '売買', '株数', '価格', '金額']
const SIDE_LABELS = { buy: '買', sell: '売' } as const
const SPLIT_LABEL = '分割'

type TradeHistoryTableProps = {
  currency: Currency
  /** 表の行（新しい順。並べ替えは API が行うので、受け取った順に出す）。 */
  entries: readonly StockHistoryEntry[]
}

/**
 * 売買履歴の表（要件14〜要件18）。列は「取引日・売買・株数・価格・金額」で、1件1行。
 * 価格と金額は実際の値（分割で割らない。要件15）で、金額は API が丸めずに計算した文字列をそのまま書式にする（要件16）。
 * 分割の行は、売買の欄に「分割」、株数の欄に「1株→<分割後の株数>株」、価格と金額の欄は空（要件17）。
 */
export function TradeHistoryTable({ currency, entries }: TradeHistoryTableProps) {
  return (
    <table className="trade-history-table">
      <thead>
        <tr>
          {COLUMN_LABELS.map((label) => (
            <th key={label} scope="col">
              {label}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {entries.map((entry, index) =>
          entry.kind === 'trade' ? (
            // 同じ日の同じ売買があってもよいので、行の key は並びの番号（行は入れ替わらない）。
            <tr key={index}>
              <td>{entry.date}</td>
              <td>{SIDE_LABELS[entry.side]}</td>
              <td>{formatShares(entry.quantity)}</td>
              <td>{formatUnitPrice(currency, entry.price)}</td>
              <td>{formatUnitPrice(currency, entry.amount)}</td>
            </tr>
          ) : (
            <tr key={index}>
              <td>{entry.date}</td>
              <td>{SPLIT_LABEL}</td>
              <td>{`1株→${formatShares(entry.ratio)}`}</td>
              <td />
              <td />
            </tr>
          ),
        )}
      </tbody>
    </table>
  )
}
