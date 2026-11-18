import { useId } from 'react'
import type { ManagedAccount } from '../../../entities/holding/model'
import { ManagedAccountCard } from './ManagedAccountCard'
import './HoldingCardList.css'

/** カードの並びの見出し（画面には出さず、支援技術が読む。リストの名前にもなる）。 */
const LIST_LABEL = 'おまかせ運用'

type ManagedAccountCardListProps = {
  /** 表示する口座。渡された順に並べる（並べ直さない。API の順 = 名前の順）。空なら何も描かない。 */
  accounts: readonly ManagedAccount[]
  /** カードのメニューの「削除」が選ばれたときに呼ばれる。確認のダイアログを開くのは呼び出し側。 */
  onDelete: (account: ManagedAccount) => void
}

/**
 * おまかせ運用の口座のカードの並び（おまかせ運用の要件9）。株式の並び（HoldingCardList）と同じ列の数・間隔（holding-card-list）で、
 * 並べ替え・移動・ライブリージョンはつながない（要件10・要件11）。口座が0件なら、見出しも含めて何も描かない。
 * 直前に画面に出さない見出し h2「おまかせ運用」を置き、リストの名前をその見出しから付ける。
 * list-style: none だと Safari の VoiceOver がリストとして読まないことがあるので、role を明示する（HoldingCardList と同じ）。
 * React の key は名前（取り直しで並びが変わっても、開いている「⋯」のメニューが別の口座のものにならないように）。
 */
export function ManagedAccountCardList({ accounts, onDelete }: ManagedAccountCardListProps) {
  const titleId = useId()
  if (accounts.length === 0) return null
  return (
    <>
      <h2 id={titleId} className="visually-hidden">
        {LIST_LABEL}
      </h2>
      <ul role="list" aria-labelledby={titleId} aria-label={LIST_LABEL} className="holding-card-list">
        {accounts.map((account) => (
          <ManagedAccountCard key={account.name} account={account} onDelete={onDelete} />
        ))}
      </ul>
    </>
  )
}
