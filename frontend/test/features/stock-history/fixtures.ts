// 銘柄ごとの売買履歴の画面のテストで使うデータの作り方（テストファイルではない。vitest は *.test.ts(x) だけを動かす）。
// 形は プラン docs/plans/stock-history/plan.md 4.8「PR② 画面とカードからの移動」の entities/stock-history/model.ts（API の応答と同じ形）。
// 金額の文字列の形は、円が小数1桁・ドルが小数2桁（プラン 4.4）。amount は「株数 × 価格」を同じ形で書いたもの。

import type {
  StockHistory,
  StockHistoryClose,
  StockHistoryEntry,
  StockHistorySplit,
  StockHistoryTrade,
} from '../../../src/entities/stock-history/model'
import type { Currency } from '../../../src/shared/lib/currency'

/** 小数点の後ろの桁数（円 1、ドル 2）。 */
const FRACTION_DIGITS: Record<Currency, number> = { JPY: 1, USD: 2 }

/** 売買1件。amount は株数 × 価格（テストの値は Number で正確に計算できる大きさ）。割る数と割った価格は、割らないなら 1 と価格のまま。 */
export function trade(
  side: 'buy' | 'sell',
  date: string,
  quantity: number,
  price: string,
  options: { currency?: Currency; splitDivisor?: string; adjustedPrice?: string } = {},
): StockHistoryTrade {
  const digits = FRACTION_DIGITS[options.currency ?? 'JPY']
  return {
    kind: 'trade',
    date,
    side,
    quantity,
    price,
    amount: (quantity * Number(price)).toFixed(digits),
    splitDivisor: options.splitDivisor ?? '1',
    adjustedPrice: options.adjustedPrice ?? price,
  }
}

/** 分割の行（分割の日と、分割後の株数。1→3 なら 3）。 */
export function split(date: string, ratio: number): StockHistorySplit {
  return { kind: 'split', date, ratio }
}

/** 終値1件。割る数と割った価格は、割らないなら 1 と価格のまま。 */
export function close(
  date: string,
  price: string,
  options: { splitDivisor?: string; adjustedPrice?: string } = {},
): StockHistoryClose {
  return { date, price, splitDivisor: options.splitDivisor ?? '1', adjustedPrice: options.adjustedPrice ?? price }
}

/** 7203 トヨタ自動車（円）の売買履歴。entries は表の並び（新しい順）で渡す。 */
export function toyotaHistory(entries: StockHistoryEntry[], closes: StockHistoryClose[]): StockHistory {
  return { stock: { code: '7203', name: 'トヨタ自動車', currency: 'JPY' }, entries, closes }
}

/** AAPL Apple（ドル）の売買履歴。 */
export function appleHistory(entries: StockHistoryEntry[], closes: StockHistoryClose[]): StockHistory {
  return { stock: { code: 'AAPL', name: 'Apple', currency: 'USD' }, entries, closes }
}

// ---- 成功基準のデータ ----

/**
 * 基準7 のデータ: 7203 の売買履歴「買 2,500円 100株 2026-01-10」「売 3,050円 50株 2026-03-02」と、終値。
 * 終値は API が返す形（最初の売買の取引日と同じかそれより後のものだけ。プラン 4.2）なので、2026-01-09 の終値は含まない。
 */
export function baseline7(): StockHistory {
  return toyotaHistory(
    [trade('sell', '2026-03-02', 50, '3050.0'), trade('buy', '2026-01-10', 100, '2500.0')],
    [close('2026-01-10', '2450.0'), close('2026-03-02', '3000.0'), close('2026-10-06', '3000.0')],
  )
}

/**
 * 基準11 のデータ（基準9 と同じ売買履歴）: 「買 2,500円 100株 2026-01-10」「買 2,520円 10株 2026-01-10」（この順に登録）「売 3,050円 50株 2026-03-02」。
 * entries は表の並び（新しい順。同じ日は後に登録したものが先）: 2026-03-02 の売、2026-01-10 の10株の買、2026-01-10 の100株の買。
 */
export function sameDayHistory(closes: StockHistoryClose[] = baseline7().closes): StockHistory {
  return toyotaHistory(
    [
      trade('sell', '2026-03-02', 50, '3050.0'),
      trade('buy', '2026-01-10', 10, '2520.0'),
      trade('buy', '2026-01-10', 100, '2500.0'),
    ],
    closes,
  )
}

/**
 * 基準12 のデータ: 7203 の分割「1→3 2026-03-31」、売買履歴「買 2,500円 100株 2026-01-10」「買 1,000円 30株 2026-03-31」、
 * 終値 2026-03-30 3,100円・2026-03-31 1,050円。割る数と割った価格は API が付ける値（プラン 4.8 の例）。
 */
export function splitHistory(): StockHistory {
  return toyotaHistory(
    [
      trade('buy', '2026-03-31', 30, '1000.0'),
      split('2026-03-31', 3),
      trade('buy', '2026-01-10', 100, '2500.0', { splitDivisor: '3', adjustedPrice: '833.3' }),
    ],
    [
      close('2026-03-30', '3100.0', { splitDivisor: '3', adjustedPrice: '1033.3' }),
      close('2026-03-31', '1050.0'),
    ],
  )
}
