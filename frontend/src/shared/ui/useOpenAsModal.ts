import { useEffect, useRef } from 'react'

/**
 * 描画した <dialog> をモーダルで開く（ref を <dialog> に渡して使う）。
 * 開いているかを確かめてから開くのは、開発時の StrictMode で effect が2回動いても、開いたダイアログを開き直さないため。
 * 閉じるのは <dialog> の close（ボタンと Esc キー）。閉じた後に描画しなくするのは呼び出し側。
 */
export function useOpenAsModal() {
  const dialogRef = useRef<HTMLDialogElement>(null)

  useEffect(() => {
    const dialog = dialogRef.current
    if (dialog && !dialog.open) {
      dialog.showModal()
    }
  }, [])

  return dialogRef
}
