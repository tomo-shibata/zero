import { useEffect, useId, useRef } from 'react'
import { Button } from '../../../shared/ui/Button'
import { useRefreshPrices } from '../hooks/useRefreshPrices'
import { refreshResultMessages } from '../lib/refreshResultMessages'
import './PriceRefreshPanel.css'

const PANEL_LABEL = '価格の更新'
const REFRESH_LABEL = '価格を今すぐ更新'
const REFRESHING_LABEL = '更新中…'
const REQUEST_ERROR_MESSAGE = '価格の更新に失敗しました'

/**
 * 価格の今すぐ更新のボタンと、その結果を表示する（要件16〜19）。
 * - 更新している間は、ボタンを押せなくし、文言を「更新中…」にする（要件17）。
 * - 終わったら、結果を1文ずつ表示する（要件18）。要求そのものが失敗したら、失敗の文言を表示する（要件19）。
 *   結果は、押し直す（前の結果を消して更新中になる）か画面を離れるまで残す（値段の更新のプラン 6章の判断19）。
 * 一覧の取り直しは useRefreshPrices が行う。
 */
export function PriceRefreshPanel() {
  const refresh = useRefreshPrices()
  const buttonRef = useRef<HTMLButtonElement>(null)
  const resultId = useId()
  // 押した時点でボタンにフォーカスがあったか（キーボード。ブラウザによってはマウスでも）。
  const hadFocusOnPress = useRef(false)

  // 更新中はボタンが disabled になり、フォーカスが外れる。ボタンにフォーカスがあった状態で押したときは、
  // 更新が終わったら（成功・失敗とも）ボタンにフォーカスを戻す。disabled のままにするのは、二重に押させないため（要件17）。
  // ただし、更新中に利用者がほかの要素へフォーカスを移していたら、引き戻さない。
  // フォーカスが無い（null・body）場合に加え、ボタン自身を含めるのは、ブラウザによっては押せなくなったボタンに activeElement が残るため。
  useEffect(() => {
    if (!refresh.isPending && hadFocusOnPress.current) {
      hadFocusOnPress.current = false
      const active = document.activeElement
      if (active === null || active === document.body || active === buttonRef.current) {
        buttonRef.current?.focus()
      }
    }
  }, [refresh.isPending])

  return (
    <section aria-label={PANEL_LABEL} className="price-refresh-panel">
      <Button
        ref={buttonRef}
        type="button"
        variant="secondary"
        className="price-refresh-panel__button"
        disabled={refresh.isPending}
        aria-describedby={resultId}
        onClick={() => {
          hadFocusOnPress.current = document.activeElement === buttonRef.current
          refresh.mutate()
        }}
      >
        {refresh.isPending ? REFRESHING_LABEL : REFRESH_LABEL}
      </Button>
      {/* 結果の入れ物。role を持たない。文言の位置（ボタンとの間隔、縦の位置）を、成功と失敗で揃えるために1つにまとめる。 */}
      <div id={resultId} className="price-refresh-panel__result">
        {/* role="status" の領域は、結果が出る前から置いておく（後から現れる領域は、支援技術に読み上げられないことがあるため）。 */}
        <div role="status" className="price-refresh-panel__status">
          {refresh.isSuccess &&
            refreshResultMessages(refresh.data).map((message) => (
              <p key={message} className="price-refresh-panel__message">
                {message}
              </p>
            ))}
        </div>
        {refresh.isError && (
          <p role="alert" className="price-refresh-panel__message">
            {REQUEST_ERROR_MESSAGE}
          </p>
        )}
      </div>
    </section>
  )
}
