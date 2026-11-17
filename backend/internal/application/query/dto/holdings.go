// Package dto は、Query 側が返す読み取り専用のモデル（Read Model）を持つ。
// domain を通さずに DB から読んだ値を、そのまま運ぶための入れ物なので、振る舞いは持たない。
package dto

// 金額（価格・取得価格・終値・評価額）は、円もドルも 0.01 単位の整数（名前の末尾が Hundredths）で持つ。
// 例: 2,500円は 250000、150.25ドルは 15025。円の値は、価格（単価）は10の倍数、評価額（1円未満を切り捨てた値）は100の倍数になる。
// 金額に浮動小数点を使わないため（保有株式一覧のプラン 4.1、売買履歴のプラン 4.3・6章の判断1）。

// Currency は、銘柄の通貨。Query 側は domain を通さないので、dto に持つ（domain の model.Currency と同じ値）。
type Currency string

const (
	CurrencyJPY Currency = "JPY" // 円
	CurrencyUSD Currency = "USD" // ドル
)

// TradeSide は、売買の種類（買いか売り）。
type TradeSide string

const (
	TradeSideBuy  TradeSide = "buy"
	TradeSideSell TradeSide = "sell"
)

// TradeRow は、DB から読んだ売買履歴1件（要件2。計算する前）。
// 同じ銘柄の行は、同じ銘柄名・通貨と、同じ最新の終値・基準日を持つ（どれも銘柄ごとに決まる値なので）。
type TradeRow struct {
	ID                   int64 // 登録した順（同じ取引日はこの昇順にたどる。要件8）
	Code, Name           string
	Currency             Currency
	Side                 TradeSide
	Quantity             int64
	PriceHundredths      int64   // 価格（1株あたり。0.01 単位）
	TradeDate            string  // 取引日（"YYYY-MM-DD"）
	ClosePriceHundredths *int64  // 最新の終値（0.01 単位）。価格が一度も保存されていなければ nil
	PriceDate            *string // 終値の取引日（"YYYY-MM-DD"）。価格が一度も保存されていなければ nil
}

// StockSplitRow は、DB から読んだ株式分割1件（株式分割の要件1〜3。計算する前）。全利用者で共有するので、利用者は持たない。
type StockSplitRow struct {
	Code      string
	SplitDate string // 分割の日（"YYYY-MM-DD"）。分割の後の値段で売買が始まる日
	Ratio     int64  // 分割後の株数（1株が Ratio 株になる）
}

// Holding は、一覧の1行（同じ銘柄の売買履歴から計算した保有。要件6）。
type Holding struct {
	Code, Name                 string
	Currency                   Currency
	Quantity                   int64
	AcquisitionPriceHundredths int64   // 表示する取得価格（円は 0.1円、ドルは 0.01ドルに四捨五入した値。要件7）
	CurrentPriceHundredths     *int64  // 価格が一度も保存されていなければ nil
	PriceDate                  *string // "YYYY-MM-DD"。価格が一度も保存されていなければ nil
	ValuationHundredths        *int64  // 評価額。円は1円未満を切り捨てた値（100の倍数）、ドルはそのまま。価格が一度も保存されていなければ nil
	// 表示する含み損益（0.01 単位。円は100の倍数。カード表示の要件7・要件8）。価格が一度も保存されていなければ nil
	UnrealizedGainHundredths *int64
	// 含み損益の率（0.1% 単位。20.0% は 200、-12.5% は -125。要件9）。表示する含み損益が0なら0。価格が一度も保存されていなければ nil
	UnrealizedGainRateTenths *int64
}

// CurrencyTotal は、1つの通貨の合計評価額と含み損益の合計（要件12・13。通貨をまたいで足さない）。
type CurrencyTotal struct {
	Currency            Currency
	ValuationHundredths int64 // その通貨の、評価額のある行の合計
	ExcludesUnpriced    bool  // その通貨に、価格のない行が1件以上ある（合計評価額・含み損益の合計にその銘柄を含まない）
	// その通貨の、価格のある銘柄の、表示する含み損益の和（カード表示の要件13）。価格のある銘柄がなければ nil
	UnrealizedGainHundredths *int64
	// その通貨の、丸める前の含み損益の和 ÷ 取得額の和 × 100 を 0.1% 単位に丸めた値。含み損益の和が0なら0。価格のある銘柄がなければ nil
	UnrealizedGainRateTenths *int64
}

// HoldingsList は、利用者の保有株式一覧。
type HoldingsList struct {
	Holdings []Holding       // 銘柄コードの昇順（通貨によらない）。0件のときは空のスライス（nil ではない。API で null にしないため）
	Totals   []CurrencyTotal // 保有のある通貨だけ、JPY → USD の順。0件のときは空のスライス（nil ではない）
}
