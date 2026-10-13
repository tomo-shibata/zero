import { useEffect, useId, useRef } from 'react'
import type { SyntheticEvent } from 'react'
import { Button } from './Button'
import { useOpenAsModal } from './useOpenAsModal'
import './ConfirmDialog.css'

const CANCEL_BUTTON_LABEL = 'キャンセル'

type ConfirmDialogProps = {
  /** ダイアログに書く確認の文。ダイアログのアクセシブルな名前にもなる。 */
  message: string
  /** 確定のボタンの文言（例: 「削除する」）。 */
  confirmLabel: string
  /** 確定のボタンを押したときに呼ばれる。ダイアログを閉じるのは呼び出し側（このコンポーネントを描画しなくする）。 */
  onConfirm: () => void
  /**
   * ダイアログが閉じた後（「キャンセル」ボタンでも Esc キーでも）に1回呼ばれる。
   * 表示をやめるのは呼び出し側。閉じた後のフォーカスの移し先も呼び出し側が決める。
   */
  onCancel: () => void
  /**
   * 確定の処理の実行中か。true の間は、確定のボタンも「キャンセル」も押せず、Esc でも閉じない
   * （二重に送らないため、また、閉じても送った処理は止まらず、閉じた後に結果が変わってしまうため。値段の更新のプラン 6章の判断13・37）。
   */
  pending?: boolean
  /** 確定の処理の失敗の文。あれば、ダイアログの中に role="alert" で出す。 */
  error?: string
}

/**
 * 取り消せない操作の前に、確認を取るダイアログ（role="alertdialog"。値段の更新のプラン 6章の判断22）。
 * ネイティブの <dialog> をモーダルで開くので、開いている間は背後の画面を操作できない。
 * ボタンは「キャンセル」「確定」の順で、最初のフォーカスは「キャンセル」に移る（開いた直後に Enter を押しても、取り消せない操作は動かない。判断33）。
 * 失敗してもダイアログは開いたままで、確定のボタンを押し直せる。
 * キーボードで確定のボタンを押した場合は、実行中に押せなくなってフォーカスが外れるので、失敗して押せる状態に戻ったときにフォーカスを戻す
 * （PriceRefreshPanel と同じ方法。判断31）。
 */
export function ConfirmDialog({
  message,
  confirmLabel,
  onConfirm,
  onCancel,
  pending = false,
  error,
}: ConfirmDialogProps) {
  const messageId = useId()
  const errorId = useId()
  const dialogRef = useOpenAsModal()
  const confirmRef = useRef<HTMLButtonElement>(null)
  // 押した時点で確定のボタンにフォーカスがあったか（キーボード。ブラウザによってはマウスでも）。
  const hadFocusOnConfirm = useRef(false)

  // 実行中は確定のボタンが disabled になり、フォーカスが外れる。ボタンにフォーカスがあった状態で押したときは、
  // 実行が終わって（失敗して）押せる状態に戻ったら、ボタンにフォーカスを戻す。成功したときはダイアログごとなくなるので、ここは動かない。
  // ただし、実行中に利用者がほかの要素へフォーカスを移していたら、引き戻さない
  // （フォーカスが無い（null・body）場合に加え、ボタン自身を含めるのは、ブラウザによっては押せなくなったボタンに activeElement が残るため）。
  useEffect(() => {
    if (!pending && hadFocusOnConfirm.current) {
      hadFocusOnConfirm.current = false
      const active = document.activeElement
      if (active === null || active === document.body || active === confirmRef.current) {
        confirmRef.current?.focus()
      }
    }
  }, [pending])

  // 実行中の Esc は、閉じさせない。cancel（<dialog> が Esc で出すイベント）と keydown の両方で既定の動きを止める。
  // cancel だけだと、Chromium は続けて Esc を押したときに止めさせずに閉じる。
  // keydown は、実行中の間だけ document の捕捉の段階で見張る。ボタンが押せなくなるとフォーカスが body に外れ、
  // ダイアログの keydown には届かなくなるが、document には届くため。実行が終わったら外す。
  useEffect(() => {
    if (!pending) return
    const stopEscape = (event: globalThis.KeyboardEvent) => {
      if (event.key === 'Escape') event.preventDefault()
    }
    document.addEventListener('keydown', stopEscape, true)
    return () => document.removeEventListener('keydown', stopEscape, true)
  }, [pending])
  const handleCancelEvent = (event: SyntheticEvent<HTMLDialogElement>) => {
    if (pending) event.preventDefault()
  }

  return (
    <dialog
      ref={dialogRef}
      role="alertdialog"
      aria-labelledby={messageId}
      className="confirm-dialog"
      onCancel={handleCancelEvent}
      // 「キャンセル」ボタンも Esc キーも <dialog> の close を通るので、呼び出し側に知らせるのは close イベントのこの1か所だけにする
      // （閉じ方によって、onCancel の呼ばれ方が変わらないように。AlertDialog と同じ）。
      onClose={onCancel}
    >
      <p id={messageId} className="confirm-dialog__message">
        {message}
      </p>
      {error !== undefined && (
        <p id={errorId} role="alert" className="confirm-dialog__error">
          {error}
        </p>
      )}
      <div className="confirm-dialog__actions">
        <Button
          type="button"
          variant="secondary"
          disabled={pending}
          onClick={() => dialogRef.current?.close()}
        >
          {CANCEL_BUTTON_LABEL}
        </Button>
        <Button
          ref={confirmRef}
          type="button"
          disabled={pending}
          // 失敗の文を、確定のボタンの説明にする（失敗してフォーカスを戻したときに、失敗の文も読まれるように。判断44）。
          aria-describedby={error !== undefined ? errorId : undefined}
          onClick={() => {
            hadFocusOnConfirm.current = document.activeElement === confirmRef.current
            onConfirm()
          }}
        >
          {confirmLabel}
        </Button>
      </div>
    </dialog>
  )
}
