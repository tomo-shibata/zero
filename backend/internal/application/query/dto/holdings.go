// Package dto は、Query 側が返す読み取り専用のモデル（Read Model）を持つ。
// domain を通さずに DB から読んだ値を、そのまま運ぶための入れ物なので、振る舞いは持たない。
package dto

// 単価（取得価格・終値）は 0.1円単位の整数（名前の末尾が Tenths）、評価額は円の整数で持つ。
// 金額に浮動小数点を使わないため（プラン 4.1、6章の判断1）。

// HoldingRow は、DB から読んだ保有データ1件（まとめる前）。
// 同じ銘柄の行は、同じ銘柄名と、同じ最新の終値・基準日を持つ（どれも銘柄ごとに決まる値なので）。
type HoldingRow struct {
	Code, Name             string
	Quantity               int64
	AcquisitionPriceTenths int64
	ClosePriceTenths       *int64  // 価格が一度も保存されていなければ nil
	PriceDate              *string // 終値の取引日（"YYYY-MM-DD"）。価格が一度も保存されていなければ nil
}

// Holding は、一覧の1行（同じ銘柄の保有データをまとめた後）。
type Holding struct {
	Code, Name             string
	Quantity               int64
	AcquisitionPriceTenths int64
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
