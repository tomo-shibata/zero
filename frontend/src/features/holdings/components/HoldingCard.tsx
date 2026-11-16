import type { ComponentProps } from 'react'
import type { Holding } from '../../../entities/holding/model'
import { formatShares, formatUnitPrice, formatValuation } from '../../../shared/lib/format'
import { MenuButton } from '../../../shared/ui/MenuButton'
import { UnrealizedGainText } from './UnrealizedGainText'
import './HoldingCard.css'

/** メニューの「削除」の項目（売買履歴の要件17）。 */
const DELETE_MENU_LABEL = '削除'

/** 価格を一度も保存していない銘柄の、価格の欄に出す文言（要件 FR-10）。 */
const UNAVAILABLE = '取得できません'

const GAIN_LABEL = '含み損益'

type HoldingCardProps = {
  holding: Holding
  /** 「⋯」のメニューの「削除」が選ばれたときに呼ばれる。確認のダイアログを開くのは呼び出し側。 */
  onDelete: (holding: Holding) => void
  /** <li> に足す属性・ref・style。並べ替えの dnd-kit の属性を、HoldingCardList が渡す（カード自身は並べ替えを知らない）。 */
  liProps?: ComponentProps<'li'>
}

/**
 * 保有株式1銘柄のカード（要件2〜6）。<ul> の中に置く <li>。
 * 上から、銘柄コード（同じ行の右端に「⋯」のメニュー）、銘柄名、含み損益と率（いちばん大きな文字）、
 * 見出し付きの評価額・保有数量・取得価格・現在の価格・価格の基準日。
 * 価格の基準日は API の文字列をそのまま出す（Date に変換すると、タイムゾーンで日付がずれるため）。
 * 価格を一度も保存していない銘柄は、含み損益・評価額・現在の価格・価格の基準日が「取得できません」（要件5、要件 FR-10）。
 */
export function HoldingCard({ holding, onDelete, liProps }: HoldingCardProps) {
  const priced = holding.currentPrice !== null
  return (
    <li {...liProps} className={liProps?.className ? `holding-card ${liProps.className}` : 'holding-card'}>
      <div className="holding-card__header">
        <p className="holding-card__code">{holding.code}</p>
        {/* data-no-drag: この中で押したマウス・タッチでは、ドラッグを始めない（要件21。HoldingCardList が見る）。 */}
        <span data-no-drag className="holding-card__menu">
          <MenuButton
            label={`${holding.name}の操作`}
            items={[{ label: DELETE_MENU_LABEL, onSelect: () => onDelete(holding) }]}
          />
        </span>
      </div>
      <p className="holding-card__name">{holding.name}</p>
      <p className="holding-card__gain-label">{GAIN_LABEL}</p>
      <UnrealizedGainText
        currency={holding.currency}
        gain={holding.unrealizedGain}
        rate={holding.unrealizedGainRate}
      />
      <dl className="holding-card__details">
        <Detail label="評価額" value={priced ? formatValuation(holding.currency, holding.valuation) : null} />
        <Detail label="保有数量" value={formatShares(holding.quantity)} />
        <Detail
          label="取得価格"
          value={formatUnitPrice(holding.currency, holding.acquisitionPrice)}
        />
        <Detail
          label="現在の価格"
          value={priced ? formatUnitPrice(holding.currency, holding.currentPrice) : null}
        />
        <Detail label="価格の基準日" value={priced ? holding.priceDate : null} />
      </dl>
    </li>
  )
}

/** 項目1つの見出し（dt）と値（dd）。value が null なら「取得できません」（弱い色）。 */
function Detail({ label, value }: { label: string; value: string | null }) {
  return (
    <>
      <dt className="holding-card__term">{label}</dt>
      <dd className={value === null ? 'holding-card__value holding-card__value--unavailable' : 'holding-card__value'}>
        {value ?? UNAVAILABLE}
      </dd>
    </>
  )
}
