import { useId } from 'react'
import { Link } from 'react-router-dom'
import { HOLDINGS_PATH } from '../../../shared/lib/routes'
import { Spinner } from '../../../shared/ui/Spinner'
import { useNotifications } from '../hooks/useNotifications'
import { NotificationItem } from './NotificationItem'
import './NotificationsPage.css'

const BACK_LINK_LABEL = '← 保有株式一覧'
const PAGE_TITLE = '通知'
const LOADING_LABEL = '読み込み中'
const EMPTY_TITLE = '通知はまだありません。'
const EMPTY_HINT = '条件に当てはまる終値が保存されると、ここに並びます。'
const FETCH_ERROR_MESSAGE = '通知を取得できませんでした。時間をおいて開き直してください。'

/**
 * 通知のリストの画面（/notifications。要件29〜要件37）。取得の状態に応じて表示を選ぶコンテナ。
 * QueryClientProvider と Router は持たない（app 層で包む）。
 *
 * 状態は「読み込み中 → 取得失敗 → 0件 → 1件以上」の順に判定する。どの状態でも、左上に「← 保有株式一覧」のリンクを出す。
 * 読み込み中は戻るリンクとローディングのアイコンだけで、見出しは出さない。表示できたら、取得した時点までの通知を見たにする（useNotifications）。
 * 一覧は API の順（取引日の新しい順）のまま出す（並べ替えない）。
 */
export function NotificationsPage() {
  const query = useNotifications()
  const titleId = useId()

  const backLink = (
    <Link to={HOLDINGS_PATH} className="notifications-page__back">
      {BACK_LINK_LABEL}
    </Link>
  )

  if (query.isPending) {
    return (
      <main className="notifications-page notifications-page--loading">
        {backLink}
        <div className="notifications-page__loading">
          <Spinner label={LOADING_LABEL} />
        </div>
      </main>
    )
  }

  return (
    <main className="notifications-page">
      {backLink}
      <div className="notifications-page__header">
        <h1 id={titleId} className="notifications-page__title">
          {PAGE_TITLE}
        </h1>
      </div>
      {query.isError ? (
        <p role="alert" className="notifications-page__message">
          {FETCH_ERROR_MESSAGE}
        </p>
      ) : query.data.items.length === 0 ? (
        <div className="notifications-page__message">
          <p className="notifications-page__empty-title">{EMPTY_TITLE}</p>
          <p className="notifications-page__empty-hint">{EMPTY_HINT}</p>
        </div>
      ) : (
        // list-style: none だと Safari の VoiceOver がリストと読まないので、role="list" を付ける（HoldingCardList と同じ）。
        <ul role="list" aria-labelledby={titleId} className="notifications-page__list">
          {query.data.items.map((notification) => (
            <NotificationItem key={notification.id} notification={notification} />
          ))}
        </ul>
      )}
    </main>
  )
}
