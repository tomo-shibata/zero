import { useId, useRef, useState } from 'react'
import type { HoldingsList } from '../../../entities/holding/model'
import { AlertDialog } from '../../../shared/ui/AlertDialog'
import { Spinner } from '../../../shared/ui/Spinner'
import { useHoldings } from '../hooks/useHoldings'
import { HoldingsTable } from './HoldingsTable'
import { HoldingsTotal } from './HoldingsTotal'
import './HoldingsPage.css'

const PAGE_TITLE = '保有株式一覧'
const LOADING_LABEL = '読み込み中'
const FETCH_ERROR_MESSAGE = '保有株式の取得に失敗しました'
const EMPTY_MESSAGE = '保有株式はありません'

/**
 * 保有株式一覧の画面（/holdings）。取得の状態（useHoldings）に応じて、表示するコンポーネントを選ぶコンテナ。
 * QueryClientProvider と Router は持たない（app 層で包む）。
 *
 * 状態は「読み込み中 → 取得失敗 → 成功（0件／1件以上）」の順に判定する（プラン 4.4「画面の状態ごとの表示」）。
 */
export function HoldingsPage() {
  const holdingsQuery = useHoldings()
  // 取得失敗のダイアログを開いているか（クライアント状態）。閉じた後は見出しだけを残す（要件 FR-22）。
  const [isErrorDialogOpen, setIsErrorDialogOpen] = useState(true)
  // 見出しの id。表とそれを囲む横スクロールの領域の名前にも使う（PR④ ステップ6 の D-2）。
  const titleId = useId()
  const titleRef = useRef<HTMLHeadingElement>(null)

  // 読み込み中はアイコンだけ（見出しも出さない。要件 FR-21、プラン 6章の判断8）。
  if (holdingsQuery.isPending) {
    return (
      <main className="holdings-page holdings-page--loading">
        <Spinner label={LOADING_LABEL} />
      </main>
    )
  }

  const handleErrorDialogClose = () => {
    setIsErrorDialogOpen(false)
    // 閉じたダイアログの中にあったフォーカスを、残る見出しに移す（body に落として、画面のどこにいるか分からなくしない。
    // PR④ ステップ6 の D-5 (b)）。見出しは tabIndex={-1} なので、Tab の順には入らずにフォーカスだけ受け取れる。
    titleRef.current?.focus()
  }

  return (
    <main className="holdings-page">
      <h1 id={titleId} ref={titleRef} tabIndex={-1} className="holdings-page__title">
        {PAGE_TITLE}
      </h1>
      {holdingsQuery.isError ? (
        isErrorDialogOpen && (
          <AlertDialog message={FETCH_ERROR_MESSAGE} onClose={handleErrorDialogClose} />
        )
      ) : (
        <HoldingsContent list={holdingsQuery.data} titleId={titleId} />
      )}
    </main>
  )
}

type HoldingsContentProps = {
  list: HoldingsList
  /** 画面の見出しの id（表の名前に使う）。 */
  titleId: string
}

/** 取得できた一覧の中身。0件なら空のメッセージだけ、1件以上なら合計の領域と表（要件 FR-6・FR-7）。 */
function HoldingsContent({ list, titleId }: HoldingsContentProps) {
  if (list.holdings.length === 0) {
    return <p className="holdings-page__empty">{EMPTY_MESSAGE}</p>
  }
  return (
    <>
      <HoldingsTotal
        totalValuation={list.totalValuation}
        excludesUnpriced={list.totalExcludesUnpriced}
      />
      <HoldingsTable holdings={list.holdings} labelledBy={titleId} />
    </>
  )
}
