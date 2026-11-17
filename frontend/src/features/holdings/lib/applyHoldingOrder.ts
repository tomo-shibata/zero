import type { Holding } from '../../../entities/holding/model'

/**
 * holdings を、order（銘柄コード）の順に並べ直す。order にない銘柄は後ろに銘柄コードの昇順で置く。
 * バックエンドの ApplyHoldingOrder と同じ決まり（保存に失敗したとき、配列を丸ごと昔のものに戻さずに、今の行を並べ直すため）。
 */
export function applyHoldingOrder(holdings: readonly Holding[], order: readonly string[]): Holding[] {
  const byCode = new Map(holdings.map((holding) => [holding.code, holding]))
  const ordered: Holding[] = []
  const placed = new Set<string>()
  for (const code of order) {
    const holding = byCode.get(code)
    if (holding === undefined || placed.has(code)) continue
    placed.add(code)
    ordered.push(holding)
  }
  const rest = holdings.filter((holding) => !placed.has(holding.code))
  // 銘柄コードの昇順は、バイトの順（Go の文字列の比較と同じ）。localeCompare は使わない。
  rest.sort((a, b) => (a.code < b.code ? -1 : a.code > b.code ? 1 : 0))
  return [...ordered, ...rest]
}
