import { useId } from 'react'
import type { Holding } from '../../../entities/holding/model'
import { HoldingCard } from './HoldingCard'
import './HoldingCardList.css'

/** カードの並びの見出し（画面には出さず、支援技術が読む。リストの名前にもなる）。 */
const LIST_LABEL = '保有株式'

type HoldingCardListProps = {
  /** 表示するカード（渡された順に並べる。API の並び）。 */
  holdings: readonly Holding[]
  /** カードのメニューの「削除」が選ばれたときに呼ばれる。確認のダイアログを開くのは呼び出し側。 */
  onDelete: (holding: Holding) => void
}

/**
 * 保有株式のカードの並び（要件1）。1銘柄1枚のカードを、画面の幅に応じた列数のグリッドに詰めて並べる。
 * 直前に、画面に出さない見出し h2「保有株式」を置き、リストの名前をその見出しから付ける
 * （見出しをたどる操作で、合計の欄 h2「合計」の次にカードの並びへ移れる）。
 * list-style: none だと Safari の VoiceOver がリストとして読まないことがあるので、role を明示する。
 */
export function HoldingCardList({ holdings, onDelete }: HoldingCardListProps) {
  const titleId = useId()
  return (
    <>
      <h2 id={titleId} className="visually-hidden">
        {LIST_LABEL}
      </h2>
      {/* aria-label は、名前の付け方が変わっても属性で探すテスト（E2E の基準31）が使えるように、aria-labelledby と同じ文言で残す。 */}
      <ul role="list" aria-labelledby={titleId} aria-label={LIST_LABEL} className="holding-card-list">
        {holdings.map((holding) => (
          <HoldingCard key={holding.code} holding={holding} onDelete={onDelete} />
        ))}
      </ul>
    </>
  )
}
