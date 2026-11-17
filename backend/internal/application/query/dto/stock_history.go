package dto

import "math/big"

// ClosingPriceRow は、DB から読んだ終値1件（銘柄ごとの売買履歴のチャート。計算する前）。
type ClosingPriceRow struct {
	TradingDate          string // 取引日（"YYYY-MM-DD"）
	ClosePriceHundredths int64  // 終値（0.01 単位）
}

// StockHistoryEntryKind は、売買履歴の表の1行の種類（売買か分割）。
type StockHistoryEntryKind string

const (
	StockHistoryEntryTrade StockHistoryEntryKind = "trade"
	StockHistoryEntrySplit StockHistoryEntryKind = "split"
)

// StockHistoryEntry は、売買履歴の表の1行（売買1件か、分割1件。要件14〜要件18）。
type StockHistoryEntry struct {
	Kind StockHistoryEntryKind
	Date string // 取引日か分割の日
	// 売買のときだけ使う
	Side                    TradeSide
	Quantity                int64
	PriceHundredths         int64    // 実際に売買した価格（割らない）
	AmountHundredths        int64    // 株数 × 価格（要件16）
	SplitDivisor            *big.Int // その日より後の分割の、分割後の株数の積。分割がなければ 1（要件9）
	AdjustedPriceHundredths int64    // 価格 ÷ SplitDivisor を、円は10・ドルは1の単位に四捨五入（要件10・要件11）
	// 分割のときだけ使う
	Ratio int64 // 分割後の株数
}

// StockHistoryClose は、チャートの終値1件。割る数と割った価格は売買の行と同じ決め方（要件9〜要件11）。
type StockHistoryClose struct {
	Date                    string
	PriceHundredths         int64
	SplitDivisor            *big.Int
	AdjustedPriceHundredths int64
}

// StockHistory は、銘柄ごとの売買履歴の画面が使う、1つの銘柄の売買履歴・分割・終値。
type StockHistory struct {
	Code, Name string
	Currency   Currency
	Entries    []StockHistoryEntry // 表の並び（新しい順。要件18）。最初の売買の日より後の分割だけ（要件17）。0件にはならない
	Closes     []StockHistoryClose // 取引日の昇順。0件のときは空のスライス（nil ではない。API で [] にするため）
}
