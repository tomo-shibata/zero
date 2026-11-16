import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useRef, useState } from 'react'
import type { HoldingsList } from '../../../entities/holding/model'
import { saveHoldingsOrder, SaveHoldingsOrderError } from '../api/saveHoldingsOrder'
import { applyHoldingOrder } from '../lib/applyHoldingOrder'
import { reorderCodes } from '../lib/reorderCodes'
import { HOLDINGS_QUERY_KEY } from './useHoldings'

/** 保存が失敗したときの HTTP のステータスのうち、画面の一覧が古い証拠として一覧を取り直すもの（プラン 判断11）。 */
const BAD_REQUEST = 400

/**
 * 並べ替えと、並びの保存（カード表示の要件22・23・29・30。プラン 4.4「並びと保存」・判断11）。
 * QueryClientProvider の中で、HOLDINGS_QUERY_KEY に一覧が入っている状態で使う。
 *
 * - move は、一覧のキャッシュの今の並びを元に新しい並びを作り、キャッシュに書いてすぐ表示してから、保存を送る（保存の応答を待たない）。
 *   元と同じ位置、またはどのカードの上でもないなら何もしない。
 * - 並べ替えたときは、途中の一覧の取り直しを取り消す（古い並びの応答で上書きしないため）。
 * - 保存は1つずつ送る。送っている間にまた並べ替えたら、最後の並びだけを覚えておき、前の保存が終わったら送る。
 * - 保存に失敗したら、待っている並びを捨て、キャッシュを、最後に保存できた並び（まだなければ、並べ替え始めたときの並び）に戻して、
 *   saveFailed を true にする。次に並べ替えたら false に戻す。一覧は取り直さない（取り直しも失敗すると画面が取得失敗に変わり、失敗の表示が消えるため）。
 *   ただし失敗が HTTP 400 のときは、戻した後に取り直す（画面の一覧が古いと毎回 400 になるため。saveFailed は取り直しが返っても true のまま。判断11）。
 * - 保存の待ちがなくなり、最後の保存が成功したときだけ、一覧を取り直す（取り消した取り直しのやり直しと、サーバーの並びに合わせるため）。
 */
export function useHoldingsOrder(): {
  move: (activeCode: string, overCode: string | null) => void
  saveFailed: boolean
} {
  const queryClient = useQueryClient()
  const [saveFailed, setSaveFailed] = useState(false)
  // 保存を送っている最中か。true の間に並べ替えたら、送らずに pendingRef に覚える。
  const savingRef = useRef(false)
  // 送っている間に並べ替えた、最後の並び（次に送る並び）。
  const pendingRef = useRef<string[] | null>(null)
  // サーバーにあると分かっている並び（最後に保存できた並び。まだなければ、並べ替え始めたときの並び）。保存の失敗で、この並びに戻す。
  const confirmedRef = useRef<string[]>([])

  const drain = useCallback(async () => {
    savingRef.current = true
    try {
      while (pendingRef.current !== null) {
        const codes = pendingRef.current
        pendingRef.current = null
        try {
          await saveHoldingsOrder(codes)
        } catch (error) {
          // 待っている並びは捨てて、最後に保存できた並びに戻す。
          pendingRef.current = null
          const confirmed = confirmedRef.current
          queryClient.setQueryData<HoldingsList>(HOLDINGS_QUERY_KEY, (old) =>
            old === undefined ? old : { ...old, holdings: applyHoldingOrder(old.holdings, confirmed) },
          )
          setSaveFailed(true)
          // 400 は、画面の一覧が古い（開いたまま CSV を取り込んだ、など）証拠。取り直さないと毎回 400 になる（判断11）。
          if (error instanceof SaveHoldingsOrderError && error.status === BAD_REQUEST) {
            void queryClient.invalidateQueries({ queryKey: HOLDINGS_QUERY_KEY })
          }
          return
        }
        confirmedRef.current = codes
      }
    } finally {
      savingRef.current = false
    }
    // 待ちがなくなり、最後の保存が成功した。
    void queryClient.invalidateQueries({ queryKey: HOLDINGS_QUERY_KEY })
  }, [queryClient])

  const move = useCallback(
    (activeCode: string, overCode: string | null) => {
      const cached = queryClient.getQueryData<HoldingsList>(HOLDINGS_QUERY_KEY)
      if (cached === undefined) return
      const current = cached.holdings.map((holding) => holding.code)
      const next = reorderCodes(current, activeCode, overCode)
      if (next === null) return

      // 保存の最中でなければ、今の並びがサーバーにある並びと同じ。失敗したときに戻す先として覚える。
      if (!savingRef.current) confirmedRef.current = current
      setSaveFailed(false)
      // 取り消した取り直しが、取り消し前の（古い）並びでキャッシュを戻さないよう、先に取り消してから書く。
      void queryClient.cancelQueries({ queryKey: HOLDINGS_QUERY_KEY })
      queryClient.setQueryData<HoldingsList>(HOLDINGS_QUERY_KEY, { ...cached, holdings: applyHoldingOrder(cached.holdings, next) })
      pendingRef.current = next
      if (!savingRef.current) void drain()
    },
    [queryClient, drain],
  )

  return { move, saveFailed }
}
