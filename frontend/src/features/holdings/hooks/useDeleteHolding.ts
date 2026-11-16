import { useMutation, useQueryClient } from '@tanstack/react-query'
import { deleteHolding } from '../api/deleteHolding'
import { HOLDINGS_QUERY_KEY } from './useHoldings'

/**
 * 削除が成功した後の一覧の取り直しを、待つ上限（ミリ秒）。過ぎたら削除した扱いにする（値段の更新のプラン 6章の判断37）。
 * 取り直しが返らないままだと、削除中は閉じられない確認のダイアログから出られなくなるため。
 */
const REFETCH_WAIT_LIMIT_MS = 10_000

/**
 * 銘柄の削除を行う（サーバー状態の書き込み。TanStack Query の useMutation で管理する）。mutate の引数は銘柄コード。
 * - 失敗しても自動で再試行しない（押し直すのは利用者。要件15）。
 * - networkMode: 'always' で、ブラウザがオフラインと判定していても要求を試みる（useRefreshPrices と同じ理由。
 *   値段の更新のプラン 6章の判断18）。
 * - 成功したら、一覧を取り直す。取り直している間は今の一覧を出したまま。取り直しが終わるまで実行中のままにして、削除の確認のダイアログを、
 *   取り直した一覧が手に入ってから閉じる（要件14「ダイアログを閉じて一覧と合計評価額を取得し直して表示する」）。
 *   ただし待つのは10秒まで。過ぎたら成功にしてダイアログを閉じる（取り直しは裏で続き、返ったら一覧が変わる）。
 *   一覧の取り直しの失敗は、削除の失敗にしない（一覧の取得失敗は、今までどおり一覧の側のダイアログで出る）。
 */
export function useDeleteHolding() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (code: string) => deleteHolding(code),
    retry: false,
    networkMode: 'always',
    onSuccess: async () => {
      let timer: ReturnType<typeof setTimeout> | undefined
      const waitLimit = new Promise<void>((resolve) => {
        timer = setTimeout(resolve, REFETCH_WAIT_LIMIT_MS)
      })
      try {
        // 取り直しと10秒の早い方まで待つ。取り直しは、待つのをやめても続く。
        await Promise.race([queryClient.invalidateQueries({ queryKey: HOLDINGS_QUERY_KEY }), waitLimit])
      } finally {
        clearTimeout(timer)
      }
    },
  })
}
