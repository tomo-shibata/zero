package query

import (
	"maps"
	"slices"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// AggregateHoldings は、利用者の保有データ rows（まとめる前）から一覧を作る（要件 FR-4〜6）。
// プラン 4.4「集約の決まり」のとおり:
//  1. 同じ銘柄コードの行を1行にまとめる。保有数量は合計し、取得価格は WeightedAverageTenths。
//  2. 評価額は、まとめた後の行ごとに、合計した保有数量で1回だけ Valuation を計算する。
//  3. 合計評価額は、まとめた後の行の評価額を足したもの。価格のない行は足さずに TotalExcludesUnpriced を true にする。
//  4. 並び順は銘柄コードの文字列としての昇順。
//
// rows が nil でも空でも、Holdings は空のスライス（nil ではない）にする。
func AggregateHoldings(rows []dto.HoldingRow) dto.HoldingsList {
	byCode := make(map[string][]dto.HoldingRow)
	for _, r := range rows {
		byCode[r.Code] = append(byCode[r.Code], r)
	}

	list := dto.HoldingsList{Holdings: make([]dto.Holding, 0, len(byCode))}
	// slices.Sorted は文字列を < で比べるので、Go の文字列としての昇順（"1301" < "130A" < "7203"）になる。
	for _, code := range slices.Sorted(maps.Keys(byCode)) {
		h := aggregateStock(byCode[code])
		list.Holdings = append(list.Holdings, h)
		if h.Valuation == nil {
			list.TotalExcludesUnpriced = true
			continue
		}
		list.TotalValuation += *h.Valuation
	}
	return list
}

// aggregateStock は、同じ銘柄の保有データ rows（1件以上）を一覧の1行にまとめる。
// 銘柄名と最新の終値・基準日は銘柄ごとに決まる値で、どの行も同じなので、先頭の行から取る。
func aggregateStock(rows []dto.HoldingRow) dto.Holding {
	first := rows[0]
	h := dto.Holding{
		Code:                   first.Code,
		Name:                   first.Name,
		AcquisitionPriceTenths: WeightedAverageTenths(rows),
	}
	for _, r := range rows {
		h.Quantity += r.Quantity
	}
	// 価格がなければ、現在の価格・基準日・評価額はどれも nil のままにする（要件 FR-10）。
	// 値を写してから指すのは、返した一覧を書き換えても、呼び出し側の rows が変わらないようにするため。
	if hasPrice(first) {
		h.CurrentPriceTenths = new(*first.ClosePriceTenths)
		h.PriceDate = new(*first.PriceDate)
		h.Valuation = new(Valuation(*first.ClosePriceTenths, h.Quantity))
	}
	return h
}

// hasPrice は、保有データ r に終値と基準日の両方があるかを返す。銘柄の先頭の行に使うと、評価額を計算する銘柄かが分かる。
// aggregateStock と sumByStock（checkCalculationRange）で評価額を計算する銘柄をそろえるため、決まりを1か所に置く。
func hasPrice(r dto.HoldingRow) bool {
	return r.ClosePriceTenths != nil && r.PriceDate != nil
}
