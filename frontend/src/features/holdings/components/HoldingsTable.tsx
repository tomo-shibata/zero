import type { Holding } from '../../../entities/holding/model'
import { formatShares, formatUnitPrice, formatYen } from '../../../shared/lib/format'
import './HoldingsTable.css'

/** 列見出し（要件 FR-3 の順）。numeric は数字を出す列（価格の基準日を含む）で、右に揃えて数字の幅をそろえる。 */
const COLUMNS = [
  { label: '銘柄コード', numeric: false },
  { label: '銘柄名', numeric: false },
  { label: '保有数量', numeric: true },
  { label: '取得価格', numeric: true },
  { label: '現在の価格', numeric: true },
  { label: '価格の基準日', numeric: true },
  { label: '評価額', numeric: true },
] as const

/** 価格を一度も保存していない銘柄の、価格の欄に出す文言（要件 FR-10）。 */
const UNAVAILABLE = '取得できません'

const NUMERIC_CELL_CLASS = 'holdings-table__cell holdings-table__cell--numeric'
const TEXT_CELL_CLASS = 'holdings-table__cell'
const UNAVAILABLE_CELL_CLASS = `${NUMERIC_CELL_CLASS} holdings-table__cell--unavailable`

type HoldingsTableProps = {
  /** 表示する行（銘柄コードの昇順にそろったもの）。 */
  holdings: readonly Holding[]
  /** 表の名前にする要素（画面の見出し）の id。 */
  labelledBy: string
}

/**
 * 保有株式の一覧の表（要件 FR-3・FR-10〜14）。
 * 価格の基準日は API の文字列をそのまま出す（Date に変換すると、タイムゾーンで日付がずれるため）。
 *
 * 狭い画面では表を囲む領域が横にスクロールする。その領域を名前付きの region にして Tab で止まれるようにし、
 * キーボード（矢印キー）でも横にスクロールできるようにする。表にも同じ名前を付ける（PR④ ステップ6 の D-2）。
 */
export function HoldingsTable({ holdings, labelledBy }: HoldingsTableProps) {
  return (
    <div
      role="region"
      aria-labelledby={labelledBy}
      tabIndex={0}
      className="holdings-table__scroll"
    >
      <table aria-labelledby={labelledBy} className="holdings-table">
        <thead>
          <tr>
            {COLUMNS.map((column) => (
              <th
                key={column.label}
                scope="col"
                className={column.numeric ? NUMERIC_CELL_CLASS : TEXT_CELL_CLASS}
              >
                {column.label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {holdings.map((holding) => (
            <HoldingsTableRow key={holding.code} holding={holding} />
          ))}
        </tbody>
      </table>
    </div>
  )
}

/** 一覧の1行。セルは7つ（列見出しと同じ順）。 */
function HoldingsTableRow({ holding }: { holding: Holding }) {
  return (
    <tr>
      <td className={TEXT_CELL_CLASS}>{holding.code}</td>
      <td className={TEXT_CELL_CLASS}>{holding.name}</td>
      <td className={NUMERIC_CELL_CLASS}>{formatShares(holding.quantity)}</td>
      <td className={NUMERIC_CELL_CLASS}>{formatUnitPrice(holding.acquisitionPrice)}</td>
      {holding.currentPrice === null ? (
        <>
          <td className={UNAVAILABLE_CELL_CLASS}>{UNAVAILABLE}</td>
          <td className={UNAVAILABLE_CELL_CLASS}>{UNAVAILABLE}</td>
          <td className={UNAVAILABLE_CELL_CLASS}>{UNAVAILABLE}</td>
        </>
      ) : (
        <>
          <td className={NUMERIC_CELL_CLASS}>{formatUnitPrice(holding.currentPrice)}</td>
          <td className={NUMERIC_CELL_CLASS}>{holding.priceDate}</td>
          <td className={NUMERIC_CELL_CLASS}>{formatYen(holding.valuation)}</td>
        </>
      )}
    </tr>
  )
}
