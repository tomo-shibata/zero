import type { Holding } from '../../../entities/holding/model'
import { formatShares, formatUnitPrice, formatYen } from '../../../shared/lib/format'
import { MenuButton } from '../../../shared/ui/MenuButton'
import './HoldingsTable.css'

/** データの列見出し（要件 FR-3 の順の7列）。numeric は数字を出す列（価格の基準日を含む）で、右に揃えて数字の幅をそろえる。 */
const COLUMNS = [
  { label: '銘柄コード', numeric: false },
  { label: '銘柄名', numeric: false },
  { label: '保有数量', numeric: true },
  { label: '取得価格', numeric: true },
  { label: '現在の価格', numeric: true },
  { label: '価格の基準日', numeric: true },
  { label: '評価額', numeric: true },
] as const

/** 8列目（行ごとの「⋯」のメニュー）の見出し。画面には出さず、支援技術にだけ読ませる（値段の更新のプラン 4.4「画面」）。 */
const ACTIONS_COLUMN_LABEL = '操作'

/** メニューの「削除」の項目（要件2）。 */
const DELETE_MENU_LABEL = '削除'

/** 価格を一度も保存していない銘柄の、価格の欄に出す文言（要件 FR-10）。 */
const UNAVAILABLE = '取得できません'

const NAME_CELL_CLASS = 'holdings-table__cell holdings-table__cell--name'
const NUMERIC_CELL_CLASS = 'holdings-table__cell holdings-table__cell--numeric'
const TEXT_CELL_CLASS = 'holdings-table__cell'
const UNAVAILABLE_CELL_CLASS = `${NUMERIC_CELL_CLASS} holdings-table__cell--unavailable`
const ACTIONS_CELL_CLASS = 'holdings-table__cell holdings-table__cell--actions'

type HoldingsTableProps = {
  /** 表示する行（銘柄コードの昇順にそろったもの）。 */
  holdings: readonly Holding[]
  /** 表の名前にする要素（画面の見出し）の id。 */
  labelledBy: string
  /** 行のメニューの「削除」が選ばれたときに呼ばれる。確認のダイアログを開くのは呼び出し側。 */
  onDelete: (holding: Holding) => void
}

/**
 * 保有株式の一覧の表（要件 FR-3・FR-10〜14）。
 * 価格の基準日は API の文字列をそのまま出す（Date に変換すると、タイムゾーンで日付がずれるため）。
 *
 * 狭い画面では表を囲む領域が横にスクロールする。その領域を名前付きの region にして Tab で止まれるようにし、
 * キーボード（矢印キー）でも横にスクロールできるようにする。表にも同じ名前を付ける（PR④ ステップ6 の D-2）。
 */
export function HoldingsTable({ holdings, labelledBy, onDelete }: HoldingsTableProps) {
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
            <th scope="col" className={ACTIONS_CELL_CLASS}>
              <span className="visually-hidden">{ACTIONS_COLUMN_LABEL}</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {holdings.map((holding) => (
            <HoldingsTableRow key={holding.code} holding={holding} onDelete={onDelete} />
          ))}
        </tbody>
      </table>
    </div>
  )
}

/** 一覧の1行。セルは8つ（列見出しと同じ順。8つ目は「⋯」のメニュー）。 */
function HoldingsTableRow({
  holding,
  onDelete,
}: {
  holding: Holding
  onDelete: (holding: Holding) => void
}) {
  return (
    <tr>
      <td className={TEXT_CELL_CLASS}>{holding.code}</td>
      <td className={NAME_CELL_CLASS}>{holding.name}</td>
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
      <td className={ACTIONS_CELL_CLASS}>
        <MenuButton
          label={`${holding.name}の操作`}
          items={[{ label: DELETE_MENU_LABEL, onSelect: () => onDelete(holding) }]}
        />
      </td>
    </tr>
  )
}
