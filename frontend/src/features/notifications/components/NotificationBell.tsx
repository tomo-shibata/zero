import { Link } from 'react-router-dom'
import { NOTIFICATIONS_PATH } from '../../../shared/lib/routes'
import { useUnseenNotificationCount } from '../hooks/useUnseenNotificationCount'
import './NotificationBell.css'

const BELL_LABEL = '通知'
/** バッジに出す数の上限。これ以上は「99+」と出す（要件25）。 */
const BADGE_MAX = 99
const BADGE_OVERFLOW_TEXT = `${BADGE_MAX}+`

/** 読み上げ名（要件26）。見ていない通知があれば「通知（新着 n件）」（n は 100件以上でも実際の数）、なければ「通知」。 */
function accessibleName(count: number): string {
  return count > 0 ? `${BELL_LABEL}（新着 ${count}件）` : BELL_LABEL
}

/**
 * ベルのリンク（要件24〜要件28）。押すと通知のリスト（/notifications）に移る（別の画面に移るのでボタンにしない）。
 * 右上のバッジに、見ていない通知の数を出す（0件のときは出さない。100件以上は「99+」）。数の取得中と失敗のときもバッジは出さず、失敗の文も出さない（要件27）。
 * QueryClientProvider と Router は持たない（app 層で包む）。
 */
export function NotificationBell() {
  const { data, isSuccess } = useUnseenNotificationCount()
  // 読み込み中と失敗（前に数が出ていても、取り直しが失敗したとき）は数を使わず、バッジの要素を作らない。
  // 取り直しの最中は success のままなので、前の数を出す。
  const count = isSuccess ? data : 0

  return (
    <Link to={NOTIFICATIONS_PATH} className="notification-bell" aria-label={accessibleName(count)}>
      <svg
        className="notification-bell__icon"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
        focusable="false"
      >
        <path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9" />
        <path d="M10.3 21a1.94 1.94 0 0 0 3.4 0" />
      </svg>
      {count > 0 && (
        <span className="notification-bell__badge" aria-hidden="true">
          {count > BADGE_MAX ? BADGE_OVERFLOW_TEXT : count}
        </span>
      )}
    </Link>
  )
}
