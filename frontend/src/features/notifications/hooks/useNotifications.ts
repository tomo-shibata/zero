import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import { UNSEEN_NOTIFICATION_COUNT_QUERY_KEY } from '../../../shared/api/queryKeys'
import { fetchNotifications } from '../api/fetchNotifications'
import { markNotificationsSeen } from '../api/markNotificationsSeen'

/**
 * 通知の一覧の query key。見ていない数の key（['notifications', 'unseen-count']）と2つ目が違うので、
 * 見ていない数を無効にしても一覧は無効にならない（見たにした後に一覧を取り直すと、新着の印が表示の途中で消える。要件34）。
 */
export const NOTIFICATIONS_QUERY_KEY = ['notifications', 'list'] as const

/**
 * 通知の一覧を取得し、表示できた後に1回だけ、取得した時点までの通知を見たにする（サーバー状態。要件34）。取得の設定は useStockHistory と同じ。
 * - 失敗しても自動で再試行しない（開き直す。要件37）。
 * - フォーカスの復帰や再接続で取り直さない（見たにした後に取り直すと、新着の印が表示の途中で消える）。
 * - gcTime: 0 で、画面を離れたらキャッシュを捨てる（開くたびに「読み込み中」から始まり、古い一覧と新着の印が出ない）。
 * - networkMode: 'always' で、ブラウザーがオフラインと判定していても取得を試みる。
 * - 見たにする送信（取得した latestId まで）に失敗しても、文を出さず何もしない（次に開いたときに、また新着として出る）。
 *   送信が終わったら、見ていない数の key だけを無効にする（ベルを描いていれば取り直す。描いていなければ、次に開いたときに取り直す）。
 *   一覧の key は無効にしない。
 */
export function useNotifications() {
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: NOTIFICATIONS_QUERY_KEY,
    queryFn: () => fetchNotifications(),
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    gcTime: 0,
    networkMode: 'always',
  })

  // 見たにするのは、この画面を開いている間に1回だけ（StrictMode で effect が2回走っても、ref が残るので2回送らない）。
  const markedRef = useRef(false)
  const latestId = query.isSuccess ? query.data.latestId : undefined
  useEffect(() => {
    // undefined は、まだ表示できていない（読み込み中・失敗）。null は通知が0件（送る ID がない）。
    if (latestId === undefined || markedRef.current) return
    markedRef.current = true
    if (latestId === null) return
    void markNotificationsSeen(latestId)
      .catch(() => {
        // 失敗しても文を出さない（要件34）。
      })
      .then(() => queryClient.invalidateQueries({ queryKey: UNSEEN_NOTIFICATION_COUNT_QUERY_KEY }))
  }, [latestId, queryClient])

  return query
}
