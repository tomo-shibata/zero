import { useEffect, useId, useRef } from 'react'
import { Button } from './Button'
import './AlertDialog.css'

const CLOSE_BUTTON_LABEL = '閉じる'

type AlertDialogProps = {
  /** ダイアログに書く文。ダイアログのアクセシブルな名前にもなる。 */
  message: string
  /**
   * ダイアログが閉じた後（「閉じる」ボタンでも Esc キーでも）に1回呼ばれる。
   * 表示をやめるのは呼び出し側（このコンポーネントを描画しなくする）。閉じた後のフォーカスの移し先も呼び出し側が決める。
   */
  onClose: () => void
}

/**
 * 利用者に知らせる文と「閉じる」ボタンだけを持つダイアログ（role="alertdialog"）。
 * ネイティブの <dialog> をモーダルで開くので、開いている間は背後の画面を操作できず、フォーカスは「閉じる」ボタンに移る。
 */
export function AlertDialog({ message, onClose }: AlertDialogProps) {
  const messageId = useId()
  const dialogRef = useOpenAsModal()

  return (
    <dialog
      ref={dialogRef}
      role="alertdialog"
      aria-labelledby={messageId}
      className="alert-dialog"
      // 「閉じる」ボタンも Esc キーも <dialog> の close を通るので、呼び出し側に知らせるのは close イベントのこの1か所だけにする
      // （閉じ方によって、フォーカスの戻り方や onClose の呼ばれ方が変わらないように。PR④ ステップ6 の EC-5）。
      onClose={onClose}
    >
      <p id={messageId} className="alert-dialog__message">
        {message}
      </p>
      <Button type="button" onClick={() => dialogRef.current?.close()}>
        {CLOSE_BUTTON_LABEL}
      </Button>
    </dialog>
  )
}

/**
 * 描画した <dialog> をモーダルで開く。
 * 開いているかを確かめてから開くのは、開発時の StrictMode で effect が2回動いても、開いたダイアログを開き直さないため。
 * 閉じるのは <dialog> の close（「閉じる」ボタンと Esc キー）。閉じた後に描画しなくするのは呼び出し側。
 */
function useOpenAsModal() {
  const dialogRef = useRef<HTMLDialogElement>(null)

  useEffect(() => {
    const dialog = dialogRef.current
    if (dialog && !dialog.open) {
      dialog.showModal()
    }
  }, [])

  return dialogRef
}
