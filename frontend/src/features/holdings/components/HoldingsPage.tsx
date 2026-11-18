import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import type { Holding, HoldingsList, ManagedAccount } from '../../../entities/holding/model'
import { stockHistoryPath } from '../../../shared/lib/routes'
import { AlertDialog } from '../../../shared/ui/AlertDialog'
import { Spinner } from '../../../shared/ui/Spinner'
import { useHoldings } from '../hooks/useHoldings'
import { useHoldingsOrder } from '../hooks/useHoldingsOrder'
import { useTotalsView } from '../hooks/useTotalsView'
import { DeleteHoldingDialog } from './DeleteHoldingDialog'
import { DeleteManagedAccountDialog } from './DeleteManagedAccountDialog'
import { HoldingCardList } from './HoldingCardList'
import { HoldingsSummary } from './HoldingsSummary'
import { ManagedAccountCardList } from './ManagedAccountCardList'
import { PriceRefreshPanel } from './PriceRefreshPanel'
import './HoldingsPage.css'

/** 削除の確認のダイアログの対象。株式（銘柄コードと名前）か、おまかせ運用の口座（名前）。 */
type DeleteTarget = { kind: 'holding'; code: string; name: string } | { kind: 'account'; name: string }

const PAGE_TITLE = '保有株式一覧'
const LOADING_LABEL = '読み込み中'
const FETCH_ERROR_MESSAGE = '保有株式の取得に失敗しました'
const EMPTY_MESSAGE = '保有株式はありません'
const SAVE_FAILED_MESSAGE = '並びの保存に失敗しました'

/**
 * 保有株式一覧の画面（/holdings）。取得の状態（useHoldings）に応じて、表示するコンポーネントを選ぶコンテナ。
 * QueryClientProvider と Router は持たない（app 層で包む）。カードを押すと、その銘柄の売買履歴の画面（/holdings/<銘柄コード>）へ移る。
 *
 * 状態は「読み込み中 → 取得失敗 → 成功（0件／1件以上）」の順に判定する（プラン 4.4「画面の状態ごとの表示」）。
 */
export function HoldingsPage() {
  const holdingsQuery = useHoldings()
  const navigate = useNavigate()
  // 取得失敗のダイアログを開いているか（クライアント状態）。閉じた後は見出しだけを残す（要件 FR-22）。
  const [isErrorDialogOpen, setIsErrorDialogOpen] = useState(true)
  const titleRef = useRef<HTMLHeadingElement>(null)
  // 削除の確認のダイアログの対象（なければ閉じている）。株式か口座かの印（kind）を付けて1つの state で持ち、
  // 閉じた後のフォーカスの移し先は、どちらの削除にも同じ effect を使う。クライアント状態なので、ここで持つ（値段の更新のプラン 4.4「画面」）。
  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget | null>(null)
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

  // 確認のダイアログを開く前に、開いた「⋯」を覚えてから、削除の対象を決める（株式も口座も同じ）。
  const openDeleteDialog = (target: DeleteTarget) => {
    deleteOpenerRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
    setDeleteTarget(target)
  }
  const handleDelete = (holding: Holding) =>
    openDeleteDialog({ kind: 'holding', code: holding.code, name: holding.name })
  const handleDeleteAccount = (account: ManagedAccount) =>
    openDeleteDialog({ kind: 'account', name: account.name })

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
      <h1 ref={titleRef} tabIndex={-1} className="holdings-page__title">
        {PAGE_TITLE}
      </h1>
      {holdingsQuery.isError ? (
        isErrorDialogOpen && (
          <AlertDialog message={FETCH_ERROR_MESSAGE} onClose={handleErrorDialogClose} />
        )
      ) : (
        <HoldingsContent
          list={holdingsQuery.data}
          onDelete={handleDelete}
          onDeleteAccount={handleDeleteAccount}
          onOpen={handleOpen}
        />
      )}
      {deleteTarget?.kind === 'holding' && (
        <DeleteHoldingDialog
          code={deleteTarget.code}
          name={deleteTarget.name}
          onClose={handleDeleteDialogClose}
        />
      )}
      {deleteTarget?.kind === 'account' && (
        <DeleteManagedAccountDialog name={deleteTarget.name} onClose={handleDeleteDialogClose} />
      )}
    </main>
  )
}

type HoldingsContentProps = {
  list: HoldingsList
  /** カードのメニューの「削除」が選ばれたときに呼ばれる。 */
  onDelete: (holding: Holding) => void
  /** 口座のカードのメニューの「削除」が選ばれたときに呼ばれる。 */
  onDeleteAccount: (account: ManagedAccount) => void
  /** カードを押したときに呼ばれる。 */
  onOpen: (code: string) => void
}

/**
 * 取得できた一覧の中身。銘柄も口座もなければ空のメッセージだけ（おまかせ運用の要件19）。
 * あれば、上から、合計の欄 → おまかせ運用のカード → 価格の今すぐ更新・並びの保存の失敗・株式のカードの並び（要件 FR-6・FR-7、カード表示の要件11、
 * おまかせ運用の要件9）。今すぐ更新・並びの保存の失敗・株式のカードは、銘柄があるときだけ出す（値段の更新の要件1、おまかせ運用の要件20）。
 */
function HoldingsContent({ list, onDelete, onDeleteAccount, onOpen }: HoldingsContentProps) {
  // 並べ替えと保存（カード表示の要件23・要件30）。保存に失敗したら、カードの並びの直前に知らせる。
  const { move, saveFailed } = useHoldingsOrder()
  // 合計の欄の表示（通貨ごと・円にまとめる）。ブラウザーに覚えさせた表示で始める（ドルを円でまとめる合計の要件16）。
  const { view, setView } = useTotalsView()
  const hasHoldings = list.holdings.length > 0
  if (!hasHoldings && list.managedAccounts.length === 0) {
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
      <ManagedAccountCardList accounts={list.managedAccounts} onDelete={onDeleteAccount} />
      {hasHoldings && (
        <>
          <PriceRefreshPanel />
          {saveFailed && (
            <p role="alert" className="holdings-page__save-failed">
              {SAVE_FAILED_MESSAGE}
            </p>
          )}
          <HoldingCardList holdings={list.holdings} onDelete={onDelete} onMove={move} onOpen={onOpen} />
        </>
      )}
    </>
  )
}
