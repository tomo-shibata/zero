import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import type { Holding, HoldingsList } from '../../../entities/holding/model'
import { stockHistoryPath } from '../../../shared/lib/routes'
import { AlertDialog } from '../../../shared/ui/AlertDialog'
import { Spinner } from '../../../shared/ui/Spinner'
import { useHoldings } from '../hooks/useHoldings'
import { useHoldingsOrder } from '../hooks/useHoldingsOrder'
import { useTotalsView } from '../hooks/useTotalsView'
import { DeleteHoldingDialog } from './DeleteHoldingDialog'
import { HoldingCardList } from './HoldingCardList'
import { HoldingsSummary } from './HoldingsSummary'
import { PriceRefreshPanel } from './PriceRefreshPanel'
import './HoldingsPage.css'

const PAGE_TITLE = '保有株式一覧'
const LOADING_LABEL = '読み込み中'
const FETCH_ERROR_MESSAGE = '保有株式の取得に失敗しました'
const EMPTY_MESSAGE = '保有株式はありません'
const SAVE_FAILED_MESSAGE = '並びの保存に失敗しました'

type HoldingsPageProps = {
  /**
   * 見出しの行の右端に出す部品（価格の変動の通知の要件24。app 層がベルのリンクを渡す。features/holdings は features/notifications を import しない）。
   * 見出しを出すときは、保有が0件・取得失敗のときも出す。読み込み中は見出しごと出さない。
   */
  headerActions?: ReactNode
}

/**
 * 保有株式一覧の画面（/holdings）。取得の状態（useHoldings）に応じて、表示するコンポーネントを選ぶコンテナ。
 * QueryClientProvider と Router は持たない（app 層で包む）。カードを押すと、その銘柄の売買履歴の画面（/holdings/<銘柄コード>）へ移る。
 *
 * 状態は「読み込み中 → 取得失敗 → 成功（0件／1件以上）」の順に判定する（プラン 4.4「画面の状態ごとの表示」）。
 */
export function HoldingsPage({ headerActions }: HoldingsPageProps) {
  const holdingsQuery = useHoldings()
  const navigate = useNavigate()
  // 取得失敗のダイアログを開いているか（クライアント状態）。閉じた後は見出しだけを残す（要件 FR-22）。
  const [isErrorDialogOpen, setIsErrorDialogOpen] = useState(true)
  const titleRef = useRef<HTMLHeadingElement>(null)
  // 削除の確認のダイアログの対象の銘柄（なければ閉じている）。クライアント状態なので、ここで持つ（値段の更新のプラン 4.4「画面」）。
  const [deleteTarget, setDeleteTarget] = useState<Pick<Holding, 'code' | 'name'> | null>(null)
  // 確認のダイアログを開いた「⋯」（メニューを選んだ直後にフォーカスがある要素。MenuButton が「⋯」に戻してから onSelect を呼ぶ）。
  const deleteOpenerRef = useRef<HTMLElement | null>(null)
  // 確認のダイアログを閉じた後に、フォーカスを移す先。閉じた後（ダイアログが画面から消えた後）の effect で移す。
  const focusAfterDeleteRef = useRef<'opener' | 'title' | null>(null)

  // 閉じている間にフォーカスを移すと、モーダルのダイアログが背後を操作できなくしているので移せない。
  // そのため、ダイアログを描画しなくなった後の effect で移す（プラン 4.4「閉じた後のフォーカス」）。
  useEffect(() => {
    if (deleteTarget !== null) return
    const destination = focusAfterDeleteRef.current
    focusAfterDeleteRef.current = null
    const opener = deleteOpenerRef.current
    // 戻り先の「⋯」がもう画面にない（行が消えた）ときは、見出しに移す（判断40）。
    if (destination === 'opener' && opener?.isConnected) opener.focus()
    else if (destination !== null) titleRef.current?.focus()
  }, [deleteTarget])

  // 読み込み中はアイコンだけ（見出しも出さない。要件 FR-21、プラン 6章の判断8）。
  if (holdingsQuery.isPending) {
    return (
      <main className="holdings-page holdings-page--loading">
        <Spinner label={LOADING_LABEL} />
      </main>
    )
  }

  const handleDelete = (holding: Holding) => {
    deleteOpenerRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
    setDeleteTarget({ code: holding.code, name: holding.name })
  }

  // 銘柄ごとの売買履歴の画面へ移る（features/stock-history を直接 import せず、パスは shared/lib/routes で作る）。
  const handleOpen = (code: string) => navigate(stockHistoryPath(code))

  const handleDeleteDialogClose = (deleted: boolean) => {
    // 削除したなら、開いた行はなくなる（一覧を取り直す）ので、残る見出しに移す。しなかったなら、開いた行の「⋯」に戻す。
    focusAfterDeleteRef.current = deleted ? 'title' : 'opener'
    setDeleteTarget(null)
  }

  const handleErrorDialogClose = () => {
    setIsErrorDialogOpen(false)
    // 閉じたダイアログの中にあったフォーカスを、残る見出しに移す（body に落として、画面のどこにいるか分からなくしない。
    // PR④ ステップ6 の D-5 (b)）。見出しは tabIndex={-1} なので、Tab の順には入らずにフォーカスだけ受け取れる。
    titleRef.current?.focus()
  }

  return (
    <main className="holdings-page">
      <div className="holdings-page__header">
        <h1 ref={titleRef} tabIndex={-1} className="holdings-page__title">
          {PAGE_TITLE}
        </h1>
        {headerActions !== undefined && <div className="holdings-page__actions">{headerActions}</div>}
      </div>
      {holdingsQuery.isError ? (
        isErrorDialogOpen && (
          <AlertDialog message={FETCH_ERROR_MESSAGE} onClose={handleErrorDialogClose} />
        )
      ) : (
        <HoldingsContent list={holdingsQuery.data} onDelete={handleDelete} onOpen={handleOpen} />
      )}
      {deleteTarget !== null && (
        <DeleteHoldingDialog
          code={deleteTarget.code}
          name={deleteTarget.name}
          onClose={handleDeleteDialogClose}
        />
      )}
    </main>
  )
}

type HoldingsContentProps = {
  list: HoldingsList
  /** カードのメニューの「削除」が選ばれたときに呼ばれる。 */
  onDelete: (holding: Holding) => void
  /** カードを押したときに呼ばれる。 */
  onOpen: (code: string) => void
}

/**
 * 取得できた一覧の中身。0件なら空のメッセージだけ、1件以上なら合計の欄と、価格の今すぐ更新と、カードの並び（要件 FR-6・FR-7、カード表示の要件11）。
 * 0件のときは今すぐ更新を出さない（値段の更新の要件1）。
 */
function HoldingsContent({ list, onDelete, onOpen }: HoldingsContentProps) {
  // 並べ替えと保存（カード表示の要件23・要件30）。保存に失敗したら、カードの並びの直前に知らせる。
  const { move, saveFailed } = useHoldingsOrder()
  // 合計の欄の表示（通貨ごと・円にまとめる）。ブラウザーに覚えさせた表示で始める（ドルを円でまとめる合計の要件16）。
  const { view, setView } = useTotalsView()
  if (list.holdings.length === 0) {
    return <p className="holdings-page__empty">{EMPTY_MESSAGE}</p>
  }
  return (
    <>
      {list.totals.length > 0 && (
        <HoldingsSummary
          totals={list.totals}
          combinedTotal={list.combinedTotal}
          view={view}
          onViewChange={setView}
        />
      )}
      <PriceRefreshPanel />
      {saveFailed && (
        <p role="alert" className="holdings-page__save-failed">
          {SAVE_FAILED_MESSAGE}
        </p>
      )}
      <HoldingCardList holdings={list.holdings} onDelete={onDelete} onMove={move} onOpen={onOpen} />
    </>
  )
}
