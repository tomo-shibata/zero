import { ConfirmDialog } from '../../../shared/ui/ConfirmDialog'
import { useDeleteHolding } from '../hooks/useDeleteHolding'

const CONFIRM_LABEL = '削除する'
const ERROR_MESSAGE = '削除できませんでした'

type DeleteHoldingDialogProps = {
  /** 削除する銘柄の銘柄コード。 */
  code: string
  /** 削除する銘柄の銘柄名。 */
  name: string
  /**
   * ダイアログを閉じるときに呼ばれる。削除に成功して閉じるなら deleted が true、「キャンセル」と Esc なら false。
   * 表示をやめるのは呼び出し側。閉じた後のフォーカスの移し先も、deleted を見て呼び出し側が決める
   * （削除したなら行がなくなるので画面の見出しへ、しなかったなら開いた行の「⋯」へ）。
   */
  onClose: (deleted: boolean) => void
}

/**
 * 銘柄の削除の確認のダイアログ（要件13〜15）。
 * 「削除する」を押すと、その銘柄の保有データをすべて削除し、一覧の取り直しが終わるか10秒たつまで待ってから閉じる（取り直しが返らなくても出られるように。判断37）。
 * 失敗したら開いたまま「削除できませんでした」を出し、押し直せる。一覧は変えない。
 */
export function DeleteHoldingDialog({ code, name, onClose }: DeleteHoldingDialogProps) {
  const deletion = useDeleteHolding()

  return (
    <ConfirmDialog
      message={`${code} ${name}を削除しますか？この銘柄の保有データはすべて削除されます。`}
      confirmLabel={CONFIRM_LABEL}
      pending={deletion.isPending}
      error={deletion.isError ? ERROR_MESSAGE : undefined}
      onConfirm={() => deletion.mutate(code, { onSuccess: () => onClose(true) })}
      onCancel={() => onClose(false)}
    />
  )
}
