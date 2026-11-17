import { useId } from 'react'
import type { Holding } from '../../../entities/holding/model'
import { useCardReorder } from '../hooks/useCardReorder'
import { SCREEN_READER_INSTRUCTIONS } from '../lib/reorderAnnouncements'
import { HoldingCard } from './HoldingCard'
import './HoldingCardList.css'

/** カードの並びの見出し（画面には出さず、支援技術が読む。リストの名前にもなる）。 */
const LIST_LABEL = '保有株式'

/** カードの役割の説明（aria-roledescription。要件19）。 */
const CARD_ROLE_DESCRIPTION = '並べ替えできるカード'

type HoldingCardListProps = {
  /** 表示するカード（渡された順に並べる。API の並び）。 */
  holdings: readonly Holding[]
  /** カードのメニューの「削除」が選ばれたときに呼ばれる。確認のダイアログを開くのは呼び出し側。 */
  onDelete: (holding: Holding) => void
  /**
   * 置いて並びが変わるときだけ呼ばれる。overCode は置いた位置にあったカードの銘柄コード（null は渡さないが、型は境界のまま）。
   * 並びを変えて保存するのは呼び出し側（このコンポーネントは保存しない）。
   */
  onMove: (activeCode: string, overCode: string | null) => void
  /**
   * カードを押したとき（ドラッグせずに押して離した・長押しせずにタップした・Enter を押した）に呼ばれる。画面を移すのは呼び出し側。
   * 押したかどうかは、ドラッグを始めたかを知っている useCardReorder が決める（ブラウザの click は、ドラッグの後にも届くため使わない）。
   */
  onOpen: (code: string) => void
}

/**
 * 保有株式のカードの並び（要件1）。1銘柄1枚のカードを、画面の幅に応じた列数のグリッドに詰めて並べ、
 * マウス・タッチ・キーボードで並べ替えられるようにする（要件17〜22。操作は useCardReorder。カード表示のプラン 4.4「PR② 並べ替え」）。
 * 直前に、画面に出さない見出し h2「保有株式」を置き、リストの名前をその見出しから付ける
 * （見出しをたどる操作で、合計の欄 h2「合計」の次にカードの並びへ移れる）。
 * 読み上げ用のライブリージョンと操作の説明は、<ul> の外（<ul> の子は <li> だけ）に置く。
 * list-style: none だと Safari の VoiceOver がリストとして読まないことがあるので、role を明示する。
 */
export function HoldingCardList({ holdings, onDelete, onMove, onOpen }: HoldingCardListProps) {
  const titleId = useId()
  const instructionsId = useId()
  const reorder = useCardReorder({ holdings, onMove, onOpen })

  return (
    <>
      <h2 id={titleId} className="visually-hidden">
        {LIST_LABEL}
      </h2>
      {/* aria-label は、名前の付け方が変わっても属性で探すテスト（E2E の基準31）が使えるように、aria-labelledby と同じ文言で残す。 */}
      <ul role="list" aria-labelledby={titleId} aria-label={LIST_LABEL} className="holding-card-list" {...reorder.listProps}>
        {reorder.holdings.map((holding) => (
          <HoldingCard
            key={holding.code}
            holding={holding}
            onDelete={onDelete}
            liProps={{
              ...reorder.cardProps(holding.code),
              // role="button" は付けない。中に「⋯」のボタンがあり、ボタンの中にボタンを入れないため。
              tabIndex: 0,
              'aria-label': `${holding.code} ${holding.name}`,
              'aria-roledescription': CARD_ROLE_DESCRIPTION,
              'aria-describedby': instructionsId,
            }}
          />
        ))}
      </ul>
      {/* hidden 属性: aria-describedby からは読まれ、上から順に読むときには読まれない。 */}
      <p id={instructionsId} hidden>
        {SCREEN_READER_INSTRUCTIONS}
      </p>
      <div role="status" aria-live="assertive" aria-atomic="true" className="visually-hidden">
        {reorder.announcement}
      </div>
    </>
  )
}
