import type { ManagedAccount } from '../../../entities/holding/model'
import { formatValuation } from '../../../shared/lib/format'
import { MenuButton } from '../../../shared/ui/MenuButton'
import { UnrealizedGainText } from './UnrealizedGainText'
import './HoldingCard.css'

/** メニューの「削除」の項目（おまかせ運用の要件22）。 */
const DELETE_MENU_LABEL = '削除'

/** 名前の下に出す、口座の種類（おまかせ運用の要件7）。aria-hidden は付けない（見える文字と読み上げをそろえる。プラン 4.6）。 */
const KIND_LABEL = 'おまかせ運用'

const GAIN_LABEL = '含み損益'

/** おまかせ運用の口座は、金額がすべて円（おまかせ運用のプラン 4.1）。 */
const CURRENCY = 'JPY'

type ManagedAccountCardProps = {
  account: ManagedAccount
  /** 「⋯」のメニューの「削除」が選ばれたときに呼ばれる。確認のダイアログを開くのは呼び出し側。 */
  onDelete: (account: ManagedAccount) => void
}

/**
 * おまかせ運用の口座1つのカード（おまかせ運用の要件7）。<ul> の中に置く <li>。見た目は株式のカード（HoldingCard）と同じ CSS を使う。
 * 上から、名前（同じ行の右端に「⋯」のメニュー）、「おまかせ運用」、含み損益と率（いちばん大きな文字）、見出し付きの評価額・元本・評価額の基準日。
 * 評価額の基準日は API の文字列をそのまま出す（Date に変換すると、タイムゾーンで日付がずれるため）。金額と率は API の文字列を整えて出すだけ（計算しない）。
 * 並べ替えも、押して移ることもないので、<li> に tabIndex・role・aria の属性は付けない（要件10・要件11）。
 */
export function ManagedAccountCard({ account, onDelete }: ManagedAccountCardProps) {
  return (
    <li className="holding-card">
      <div className="holding-card__header holding-card__header--managed">
        <p className="holding-card__code holding-card__code--wrap">{account.name}</p>
        <span className="holding-card__menu">
          <MenuButton
            label={`${account.name}の操作`}
            items={[{ label: DELETE_MENU_LABEL, onSelect: () => onDelete(account) }]}
          />
        </span>
      </div>
      <p className="holding-card__kind">{KIND_LABEL}</p>
      <p className="holding-card__gain-label">{GAIN_LABEL}</p>
      <UnrealizedGainText currency={CURRENCY} gain={account.unrealizedGain} rate={account.unrealizedGainRate} />
      <dl className="holding-card__details">
        <Detail label="評価額" value={formatValuation(CURRENCY, account.valuation)} />
        <Detail label="元本" value={formatValuation(CURRENCY, account.principal)} />
        <Detail label="評価額の基準日" value={account.valuationDate} />
      </dl>
    </li>
  )
}

/** 項目1つの見出し（dt）と値（dd）。 */
function Detail({ label, value }: { label: string; value: string }) {
  return (
    <>
      <dt className="holding-card__term">{label}</dt>
      <dd className="holding-card__value">{value}</dd>
    </>
  )
}
