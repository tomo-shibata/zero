import { useQuery } from '@tanstack/react-query'
import { UNSEEN_NOTIFICATION_COUNT_QUERY_KEY } from '../../../shared/api/queryKeys'
import { fetchUnseenNotificationCount } from '../api/fetchUnseenNotificationCount'

/**
 * 見ていない通知の数を取得する（サーバー状態。TanStack Query で管理する。ベルが使う。要件25・要件27）。取得の設定は useStockHistory と同じ。
 * - 失敗しても自動で再試行しない（失敗したらバッジを出さない。次に /holdings を開いたときに取り直す）。
 * - フォーカスの復帰や再接続で取り直さない。
 * - gcTime: 0 で、使う画面（ベル）がなくなったらキャッシュを捨てる。/notifications から戻ったときに、古い数が一瞬出ない。
 * - networkMode: 'always' で、ブラウザーがオフラインと判定していても取得を試みる。
 */
export function useUnseenNotificationCount() {
  return useQuery({
    queryKey: UNSEEN_NOTIFICATION_COUNT_QUERY_KEY,
    queryFn: () => fetchUnseenNotificationCount(),
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    gcTime: 0,
    networkMode: 'always',
  })
}
