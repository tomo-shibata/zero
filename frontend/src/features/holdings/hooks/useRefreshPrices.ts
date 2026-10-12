import { useMutation, useQueryClient } from '@tanstack/react-query'
import { refreshPrices } from '../api/refreshPrices'
import { HOLDINGS_QUERY_KEY } from './useHoldings'

/**
 * 価格の今すぐ更新を行う（サーバー状態の書き込み。TanStack Query の useMutation で管理する）。
 * - 失敗しても自動で再試行しない（押し直すのは利用者。要件19）。
 * - networkMode: 'always' で、ブラウザがオフラインと判定していても要求を試みる（useHoldings と同じ理由。
 *   既定の 'online' では、オフラインの間は要求を始めずに待つので、更新中のまま止まり、失敗の表示が出ない。
 *   値段の更新のプラン 6章の判断18）。
 * - 成功したら、一覧を取り直す。取り直している間は今の一覧を出したまま。取り直しが終わるまで更新中のままにして、
 *   結果の文言を、取り直した一覧と同時に出す（要件18「一覧と合計評価額を取得し直して表示し、…結果を表示する」）。
 */
export function useRefreshPrices() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => refreshPrices(),
    retry: false,
    networkMode: 'always',
    onSuccess: () => queryClient.invalidateQueries({ queryKey: HOLDINGS_QUERY_KEY }),
  })
}
