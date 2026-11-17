import { useQuery } from '@tanstack/react-query'
import { fetchStockHistory } from '../api/fetchStockHistory'

/** 銘柄ごとの売買履歴の query key。銘柄コードを含める（銘柄ごとに別の取得にするため）。 */
const stockHistoryQueryKey = (code: string) => ['stock-history', code] as const

/**
 * 銘柄 code の売買履歴を取得する（サーバー状態。TanStack Query で管理する）。取得の設定は保有株式一覧（useHoldings）と同じ。
 * - 失敗しても自動で再試行しない（再試行は画面の再読み込みで行う。要件23）。
 * - フォーカスの復帰や再接続で取り直さない。
 * - gcTime: 0 で、画面を離れたらキャッシュを捨てる（開くたびに「読み込み中」から始まる。要件22）。
 * - networkMode: 'always' で、ブラウザがオフラインと判定していても取得を試みる（オフラインの間に読み込み中のまま止まらず、失敗のダイアログを出す）。
 */
export function useStockHistory(code: string) {
  return useQuery({
    queryKey: stockHistoryQueryKey(code),
    queryFn: () => fetchStockHistory(code),
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    gcTime: 0,
    networkMode: 'always',
  })
}
