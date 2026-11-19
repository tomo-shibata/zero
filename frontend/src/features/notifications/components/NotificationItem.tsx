import { Link } from 'react-router-dom'
import { conditionLabel } from '../../../entities/price-alert/conditionText'
import type { PriceAlertNotification } from '../../../entities/price-alert/model'
import { stockHistoryPath } from '../../../shared/lib/routes'
import { notificationDetail, notificationHeadline } from '../lib/notificationTexts'
import './NotificationItem.css'

const NEW_MARK_TEXT = '新着'
const CONDITION_PREFIX = '条件: '

type NotificationItemProps = {
  notification: PriceAlertNotification
}

/**
 * 通知1件（要件31）。1つの <li> の中は1つのリンクで、4行を全部包む（押す・Enter で、その銘柄の売買履歴の画面へ移る。要件35）。
 * 1行目: 左に「<銘柄コード> <銘柄名>」、右に新着の印（取得した時点で見ていなかった通知だけ。要件34）と取引日。
 * 2行目: 何が起きたか（大きい太字。率と含み損益の値に色を付ける）。3行目: 前後の値。4行目: 「条件: <対象>・<条件の文>」。
 * <li> にもリンクの中にも、onClick などのイベントは付けない（リンクの既定の動きで移る）。
 */
export function NotificationItem({ notification: n }: NotificationItemProps) {
  const headline = notificationHeadline(n)
  const linkClassName = n.isNew ? 'notification-item__link notification-item__link--new' : 'notification-item__link'
  return (
    <li className="notification-item">
      <Link to={stockHistoryPath(n.stockCode)} className={linkClassName}>
        <span className="notification-item__top">
          <span className="notification-item__stock">
            <span>{n.stockCode}</span> <span className="notification-item__stock-name">{n.stockName}</span>
          </span>
          <span className="notification-item__meta">
            {n.isNew && <span className="notification-item__new-mark">{NEW_MARK_TEXT}</span>}
            <time dateTime={n.tradingDate}>{n.tradingDate}</time>
          </span>
        </span>
        <span className="notification-item__headline">
          {headline.label}
          {headline.tone === 'none' ? headline.value : <span data-tone={headline.tone}>{headline.value}</span>}
        </span>
        <span className="notification-item__detail">{notificationDetail(n)}</span>
        <span className="notification-item__condition">{`${CONDITION_PREFIX}${conditionLabel(n.condition, n.currency)}`}</span>
      </Link>
    </li>
  )
}
