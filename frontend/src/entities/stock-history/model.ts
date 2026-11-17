import type { Currency } from '../../shared/lib/currency'

// 銘柄ごとの売買履歴のモデル。GET /api/holdings/{code}/history の応答と同じ形（銘柄ごとの売買履歴のプラン 4.4・4.8）。
// 金額（price・amount・adjustedPrice）は、円は小数第1位を1桁・ドルは小数第2位を2桁付けた文字列。数値にしない（桁が大きくても誤差を出さないため）。

/** 売買1件の行。 */
export type StockHistoryTrade = {
  kind: 'trade'
  /** 取引日。"YYYY-MM-DD"。 */
  date: string
  side: 'buy' | 'sell'
  /** 株数。 */
  quantity: number
  /** 実際に売買した価格（分割で割らない）。 */
  price: string
  /** 株数 × 価格（丸めない）。 */
  amount: string
  /** その日より後の分割の、分割後の株数の積（分割がなければ "1"）。整数の文字列で、2^53 を超えうる。 */
  splitDivisor: string
  /** 価格 ÷ splitDivisor を、円は小数第2位・ドルは小数第3位で四捨五入した価格（吹き出しに出す）。 */
  adjustedPrice: string
}

/** 分割の行。ratio は分割後の株数（1→3 なら 3）。 */
export type StockHistorySplit = { kind: 'split'; date: string; ratio: number }

export type StockHistoryEntry = StockHistoryTrade | StockHistorySplit

/** 終値1件。splitDivisor・adjustedPrice は売買と同じ意味。 */
export type StockHistoryClose = { date: string; price: string; splitDivisor: string; adjustedPrice: string }

/** 銘柄の売買履歴（その利用者の分）と終値。 */
export type StockHistory = {
  stock: { code: string; name: string; currency: Currency }
  /** 表の並び（新しい順）。 */
  entries: StockHistoryEntry[]
  /** 取引日の昇順。最初の売買の取引日と同じかそれより後のものだけ。 */
  closes: StockHistoryClose[]
}
