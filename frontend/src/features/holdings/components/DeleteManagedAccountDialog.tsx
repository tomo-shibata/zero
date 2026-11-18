import { ConfirmDialog } from '../../../shared/ui/ConfirmDialog'
import { useDeleteManagedAccount } from '../hooks/useDeleteManagedAccount'

const CONFIRM_LABEL = '削除する'
const ERROR_MESSAGE = '削除できませんでした'

type DeleteManagedAccountDialogProps = {
  /** 削除する口座の名前。 */
  name: string
  /**
   * ダイアログを閉じるときに呼ばれる。削除に成功して閉じるなら deleted が true、「キャンセル」と Esc なら false。
   * 表示をやめるのも、閉じた後のフォーカスの移し先を決めるのも呼び出し側（DeleteHoldingDialog と同じ）。
   */
  onClose: (deleted: boolean) => void
}

/**
 * おまかせ運用の口座の削除の確認のダイアログ（おまかせ運用の要件23・要件24）。
 * 「削除する」を押すと、その口座と記録をすべて削除し、一覧の取り直しが終わるか10秒たつまで待ってから閉じる。
 * 失敗したら開いたまま「削除できませんでした」を出し、押し直せる。一覧は変えない。
 */
export function DeleteManagedAccountDialog({ name, onClose }: DeleteManagedAccountDialogProps) {
  const deletion = useDeleteManagedAccount()

  return (
    <ConfirmDialog
      message={`${name}を削除しますか？この口座の記録はすべて削除されます。`}
      confirmLabel={CONFIRM_LABEL}
      pending={deletion.isPending}
      error={deletion.isError ? ERROR_MESSAGE : undefined}
      onConfirm={() => deletion.mutate(name, { onSuccess: () => onClose(true) })}
      onCancel={() => onClose(false)}
    />
  )
}
