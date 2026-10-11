import { useQuery } from '@tanstack/react-query'
import { fetchHoldings } from '../api/fetchHoldings'

const HOLDINGS_QUERY_KEY = ['holdings'] as const

/**
 * 保有株式一覧を取得する（サーバー状態。TanStack Query で管理する）。
 * - 失敗しても自動で再試行しない。再試行は画面の再読み込みで行う（要件 FR-22）。
 * - フォーカスの復帰や再接続で取り直さない（表示中の一覧が勝手に読み込み中や取得失敗に変わらないように）。
 * - gcTime: 0 で、画面を離れたらキャッシュを捨てる。/holdings を開くたびに「読み込み中」から始まる（要件 FR-21）。
 * - networkMode: 'always' で、ブラウザがオフラインと判定していても取得を試みる。既定の 'online' では、オフラインの間は
 *   取得を始めずに待つので、読み込み中のまま止まり、取得失敗のダイアログ（要件 FR-22）が出ない。
 *   API は 127.0.0.1 にあるので、オフラインと判定されていても取得できる場合もある（プラン 6章の判断22）。
 */
export function useHoldings() {
  return useQuery({
    queryKey: HOLDINGS_QUERY_KEY,
    queryFn: () => fetchHoldings(),
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    gcTime: 0,
    networkMode: 'always',
  })
}
