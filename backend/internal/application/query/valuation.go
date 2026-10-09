package query

import "github.com/tomo-shibata/zero/backend/internal/application/query/dto"

// 金額の計算は、浮動小数点の誤差を入れないよう、すべて整数で行う（プラン 6章の判断1）。
// 値の範囲は、現実の株価（1株あたり数百万円まで）と保有数量なら、途中の積や和も int64 に十分収まる。
// ここでは桁あふれを確かめない。収まらない保有データは、ListHoldings.Execute が集約の前に
// checkCalculationRange で見つけてエラーにする（プラン 6章の判断14）。

// Valuation は、単価 priceTenths（0.1円単位）の株を quantity 株持つときの評価額（円）を返す。
// 1円未満は切り捨てる（要件 FR-9）。単価と数量は正の数なので、整数の割り算の切り捨てがそのまま使える。
func Valuation(priceTenths, quantity int64) int64 {
	return priceTenths * quantity / 10
}

// WeightedAverageTenths は、rows の取得価格を保有数量で重み付けした平均（0.1円単位）を返す。
// 0.1円未満（円の小数第2位）は四捨五入する（要件 FR-4）。
//
// S = Σ(数量×単価)、Q = Σ数量 とすると、平均は S/Q。0.1円単位で四捨五入した値は floor(S/Q + 1/2) で、
// 分母を払うと (2S+Q)/(2Q) の切り捨てになる。浮動小数点を使わずに整数だけで正しく丸めるため、この形で計算する。
// 数量の合計が0以下（rows が空など）のときは、割り算ができないので 0 を返す。
func WeightedAverageTenths(rows []dto.HoldingRow) int64 {
	var sum, quantity int64
	for _, r := range rows {
		sum += r.Quantity * r.AcquisitionPriceTenths
		quantity += r.Quantity
	}
	if quantity <= 0 {
		return 0
	}
	return (2*sum + quantity) / (2 * quantity)
}
