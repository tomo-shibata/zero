import { useMutation, useQueryClient } from '@tanstack/react-query'
import { deleteManagedAccount } from '../api/deleteManagedAccount'
import { HOLDINGS_QUERY_KEY } from './useHoldings'

/**
 * 削除が成功した後の一覧の取り直しを、待つ上限（ミリ秒）。過ぎたら削除した扱いにする（useDeleteHolding と同じ）。
 * 取り直しが返らないままだと、削除中は閉じられない確認のダイアログから出られなくなるため。
 */
const REFETCH_WAIT_LIMIT_MS = 10_000

/**
 * おまかせ運用の口座の削除を行う（useMutation。mutate の引数は口座の名前）。useDeleteHolding と同じ形。
 * - 失敗しても自動で再試行しない（押し直すのは利用者）。
 * - networkMode: 'always' で、ブラウザがオフラインと判定していても要求を試みる。
 * - 成功したら一覧を取り直し、取り直しか10秒の早い方まで実行中のままにする。取り直しの失敗は、削除の失敗にしない。
 */
export function useDeleteManagedAccount() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => deleteManagedAccount(name),
    retry: false,
    networkMode: 'always',
    onSuccess: async () => {
      let timer: ReturnType<typeof setTimeout> | undefined
      const waitLimit = new Promise<void>((resolve) => {
        timer = setTimeout(resolve, REFETCH_WAIT_LIMIT_MS)
      })
      try {
        await Promise.race([queryClient.invalidateQueries({ queryKey: HOLDINGS_QUERY_KEY }), waitLimit])
      } finally {
        clearTimeout(timer)
      }
    },
  })
}
