// Package dto は、Query 側が返す読み取り専用のモデル（Read Model）を持つ。
// domain を通さずに DB から読んだ値を、そのまま運ぶための入れ物なので、振る舞いは持たない。
package dto

// 単価（価格・取得価格・終値）は 0.1円単位の整数（名前の末尾が Tenths）、評価額は円の整数で持つ。
// 金額に浮動小数点を使わないため（保有株式一覧のプラン 4.1、6章の判断1）。
// 0.01 単位への変更は、ドルの PR（売買履歴のプラン 6章の判断13）で行う。

// TradeSide は、売買の種類（買いか売り）。
type TradeSide string

const (
	TradeSideBuy  TradeSide = "buy"
	TradeSideSell TradeSide = "sell"
)

// TradeRow は、DB から読んだ売買履歴1件（要件2。計算する前）。
// 同じ銘柄の行は、同じ銘柄名と、同じ最新の終値・基準日を持つ（どれも銘柄ごとに決まる値なので）。
type TradeRow struct {
	ID               int64 // 登録した順（同じ取引日はこの昇順にたどる。要件8）
	Code, Name       string
	Side             TradeSide
	Quantity         int64
	PriceTenths      int64   // 価格（1株あたり。0.1円単位）
	TradeDate        string  // 取引日（"YYYY-MM-DD"）
	ClosePriceTenths *int64  // 最新の終値。価格が一度も保存されていなければ nil
	PriceDate        *string // 終値の取引日（"YYYY-MM-DD"）。価格が一度も保存されていなければ nil
}

// Holding は、一覧の1行（同じ銘柄の売買履歴から計算した保有。要件6）。
type Holding struct {
	Code, Name             string
	Quantity               int64
	AcquisitionPriceTenths int64   // 表示する取得価格（0.1円に四捨五入した値。要件7）
	CurrentPriceTenths     *int64  // 価格が一度も保存されていなければ nil
	PriceDate              *string // "YYYY-MM-DD"。価格が一度も保存されていなければ nil
	Valuation              *int64  // 評価額（円）。価格が一度も保存されていなければ nil
}

// HoldingsList は、利用者の保有株式一覧。
type HoldingsList struct {
	Holdings              []Holding // 銘柄コードの昇順。0件のときは空のスライス（nil ではない。API で null にしないため）
	TotalValuation        int64     // 各行の Valuation（nil 以外）の合計（円）
	TotalExcludesUnpriced bool      // 価格のない行が1件以上あれば true（合計評価額にその銘柄を含まない）
}
