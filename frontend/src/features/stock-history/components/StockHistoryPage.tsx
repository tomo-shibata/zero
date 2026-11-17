import { useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { HOLDINGS_PATH } from '../../../shared/lib/routes'
import { AlertDialog } from '../../../shared/ui/AlertDialog'
import { Spinner } from '../../../shared/ui/Spinner'
import { useStockHistory } from '../hooks/useStockHistory'
import { PriceChart } from './PriceChart'
import { TradeHistoryTable } from './TradeHistoryTable'
import './StockHistoryPage.css'

const BACK_LINK_LABEL = '← 保有株式一覧'
const LOADING_LABEL = '読み込み中'
const FETCH_ERROR_MESSAGE = '売買履歴の取得に失敗しました'
const NOT_FOUND_MESSAGE = 'この銘柄の売買履歴はありません'
const TABLE_LABEL = '売買履歴'

/**
 * 銘柄ごとの売買履歴の画面（/holdings/:code。要件4〜要件5・要件19〜要件23）。取得の状態に応じて表示を選ぶコンテナ。
 * QueryClientProvider と Router は持たない（app 層で包む）。
 *
 * 状態は「読み込み中 → 取得失敗 → 売買履歴なし（404）→ あり」の順に判定する。どの状態でも、左上に「← 保有株式一覧」のリンクを出す。
 * ありのときは、見出しより下を上下に半分ずつ分け、上にチャート、下に表を置く（表だけが下半分の中でスクロールする）。
 */
export function StockHistoryPage() {
  const { code = '' } = useParams()
  const query = useStockHistory(code)
  // 取得失敗のダイアログを開いているか（クライアント状態）。閉じた後は戻るリンクだけを残す（要件23）。
  const [isErrorDialogOpen, setIsErrorDialogOpen] = useState(true)
  const backLinkRef = useRef<HTMLAnchorElement>(null)

  const handleErrorDialogClose = () => {
    setIsErrorDialogOpen(false)
    // 閉じたダイアログの中にあったフォーカスを、残る戻るリンクに移す（body に落として、画面のどこにいるか分からなくしない）。
    backLinkRef.current?.focus()
  }

  const backLink = (
    <Link ref={backLinkRef} to={HOLDINGS_PATH} className="stock-history-page__back">
      {BACK_LINK_LABEL}
    </Link>
  )

  if (query.isPending) {
    return (
      <main className="stock-history-page stock-history-page--loading">
        {backLink}
        <div className="stock-history-page__loading">
          <Spinner label={LOADING_LABEL} />
        </div>
      </main>
    )
  }

  if (query.isError) {
    return (
      <main className="stock-history-page">
        {backLink}
        {isErrorDialogOpen && <AlertDialog message={FETCH_ERROR_MESSAGE} onClose={handleErrorDialogClose} />}
      </main>
    )
  }

  if (query.data.kind === 'notFound') {
    return (
      <main className="stock-history-page">
        {backLink}
        <p className="stock-history-page__not-found">{NOT_FOUND_MESSAGE}</p>
      </main>
    )
  }

  const { history } = query.data
  return (
    <main className="stock-history-page stock-history-page--loaded">
      {backLink}
      <h1 className="stock-history-page__title">{`${history.stock.code} ${history.stock.name}`}</h1>
      <div className="stock-history-page__chart">
        <PriceChart history={history} />
      </div>
      {/* 表の箱だけをスクロールさせる。スクロールする箱はキーボードで動かせるよう、フォーカスを受け取れるようにする。 */}
      <div className="stock-history-page__table" role="region" aria-label={TABLE_LABEL} tabIndex={0}>
        <TradeHistoryTable currency={history.stock.currency} entries={history.entries} />
      </div>
    </main>
  )
}
