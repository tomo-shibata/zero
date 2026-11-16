package query

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"math/big"
	"slices"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

var (
	// ErrNegativeQuantity は、売買履歴をたどる途中で、保有数量が負になる銘柄があること（要件9 を破る売買履歴）。
	// 取り込みは多すぎる売りを入れないので、起きるのは seed や SQL で手で入れたときだけ。誤った一覧を返さないためにエラーにする。
	ErrNegativeQuantity = errors.New("保有数量が負になる売買履歴があります")
	// ErrCalculationOverflow は、一覧の最後の値（保有数量・取得価格・評価額・合計評価額）が int64 に収まらないこと。
	ErrCalculationOverflow = errors.New("int64 に収まりません")
)

// tenthsPerYen は、1円が 0.1円単位でいくつか。0.1円単位の値をこれで割ると円になる。
const tenthsPerYen = 10

// CalculateHoldings は、利用者の売買履歴 rows から、保有株式一覧を作る（要件4・6〜10。移動平均法）。
//
//  1. 銘柄ごとに、売買履歴を「取引日の昇順 → id の昇順」にたどる（要件8。rows の並びには頼らない）。
//  2. 保有数量 Q（整数）と取得価格 A（有理数）を持ち、買いでは A ← (Q×A ＋ 株数×価格) ÷ (Q ＋ 株数)、Q ← Q ＋ 株数、
//     売りでは Q ← Q − 株数（A は変えない）。途中では丸めない（要件6・要件7）。
//  3. 最後の Q が0の銘柄（売り切った銘柄）は一覧に出さず、合計評価額にも、価格なしの注記にも響かせない（要件10）。
//  4. 表示する取得価格は、A を 0.1円に四捨五入した値（要件7）。評価額は「現在の価格 × Q」の1円未満の切り捨て（要件11）。
//  5. 一覧は銘柄コードの文字列としての昇順。合計評価額は、評価額のある行の合計。価格のない行があれば TotalExcludesUnpriced を true にする。
//
// 計算は math/big で行い、桁あふれしないようにする。保有数量が途中で負になる銘柄があれば ErrNegativeQuantity、
// 最後の値が int64 に収まらなければ ErrCalculationOverflow を（包んで）返す。値そのものは資産の額なので、エラー（ログ）には入れない。
// rows が nil でも空でも、Holdings は空のスライス（nil ではない）にする。
func CalculateHoldings(rows []dto.TradeRow) (dto.HoldingsList, error) {
	byCode := make(map[string][]dto.TradeRow)
	for _, r := range rows {
		byCode[r.Code] = append(byCode[r.Code], r)
	}

	list := dto.HoldingsList{Holdings: make([]dto.Holding, 0, len(byCode))}
	total := new(big.Int)
	// slices.Sorted は文字列を < で比べるので、Go の文字列としての昇順（"1301" < "130A" < "7203"）になる。
	// あふれる銘柄が複数あっても、毎回同じ銘柄をエラーに入れるため、エラーもこの順に見つける。
	for _, code := range slices.Sorted(maps.Keys(byCode)) {
		h, valuation, held, err := calculateStock(byCode[code])
		if err != nil {
			return dto.HoldingsList{}, fmt.Errorf("銘柄 %q: %w", code, err)
		}
		if !held {
			continue
		}
		list.Holdings = append(list.Holdings, h)
		if valuation == nil {
			list.TotalExcludesUnpriced = true
			continue
		}
		total.Add(total, valuation)
		// 足すたびに確かめ、収まらなくなった銘柄をエラーに入れる。評価額は負にならないので、途中で収まらなければ最後の値も収まらない。
		if !total.IsInt64() {
			return dto.HoldingsList{}, fmt.Errorf("銘柄 %q までの合計評価額: %w", code, ErrCalculationOverflow)
		}
	}
	list.TotalValuation = total.Int64()
	return list, nil
}

// calculateStock は、同じ銘柄の売買履歴 rows（1件以上）をたどって、一覧の1行を作る。
// 保有数量が0（売り切った銘柄）なら held が false で、ほかの戻り値は使わない。
// valuation は、評価額（円）。価格がなければ nil。
func calculateStock(rows []dto.TradeRow) (h dto.Holding, valuation *big.Int, held bool, err error) {
	// 呼び出し側の rows を並べ替えないよう、写してから並べる。
	rows = slices.Clone(rows)
	slices.SortFunc(rows, func(a, b dto.TradeRow) int {
		// 取引日は "YYYY-MM-DD" なので、文字列の昇順が日付の古い順になる。
		return cmp.Or(cmp.Compare(a.TradeDate, b.TradeDate), cmp.Compare(a.ID, b.ID))
	})

	quantity := new(big.Int)    // Q
	acquisition := new(big.Rat) // A（0.1円単位。丸めない）
	for _, r := range rows {
		qty := big.NewInt(r.Quantity)
		switch r.Side {
		case dto.TradeSideBuy:
			// 保有数量が0のときは Q×A が0になり、A はこの買いの価格になる（要件6）。買った後の Q は1以上なので割れる。
			cost := new(big.Rat).Mul(new(big.Rat).SetInt(quantity), acquisition)
			cost.Add(cost, new(big.Rat).SetInt(new(big.Int).Mul(qty, big.NewInt(r.PriceTenths))))
			quantity.Add(quantity, qty)
			acquisition = cost.Quo(cost, new(big.Rat).SetInt(quantity))
		case dto.TradeSideSell:
			quantity.Sub(quantity, qty)
			if quantity.Sign() < 0 {
				return dto.Holding{}, nil, false, ErrNegativeQuantity
			}
		default:
			return dto.Holding{}, nil, false, fmt.Errorf("知らない売買の種類 %q です", r.Side)
		}
	}
	if quantity.Sign() == 0 {
		return dto.Holding{}, nil, false, nil
	}
	if !quantity.IsInt64() {
		return dto.Holding{}, nil, false, fmt.Errorf("保有数量: %w", ErrCalculationOverflow)
	}

	// 銘柄名と最新の終値・基準日は銘柄ごとに決まる値で、どの行も同じなので、先頭の行から取る。
	first := rows[0]
	h = dto.Holding{Code: first.Code, Name: first.Name, Quantity: quantity.Int64()}

	acquisitionTenths := roundHalfUp(acquisition)
	if !acquisitionTenths.IsInt64() {
		return dto.Holding{}, nil, false, fmt.Errorf("取得価格: %w", ErrCalculationOverflow)
	}
	h.AcquisitionPriceTenths = acquisitionTenths.Int64()

	// 価格がなければ、現在の価格・基準日・評価額はどれも nil のままにする（保有株式一覧の FR-10）。
	// 値を写してから指すのは、返した一覧を書き換えても、呼び出し側の rows が変わらないようにするため。
	if first.ClosePriceTenths != nil && first.PriceDate != nil {
		h.CurrentPriceTenths = new(*first.ClosePriceTenths)
		h.PriceDate = new(*first.PriceDate)
		// 評価額は、保有数量で1回だけ計算し、1円未満を切り捨てる（要件11）。Quo は 0 の方向に切り捨てる（値は負にならない）。
		valuation = new(big.Int).Mul(big.NewInt(*first.ClosePriceTenths), quantity)
		valuation.Quo(valuation, big.NewInt(tenthsPerYen))
		if !valuation.IsInt64() {
			return dto.Holding{}, nil, false, fmt.Errorf("評価額: %w", ErrCalculationOverflow)
		}
		h.Valuation = new(valuation.Int64())
	}
	return h, valuation, true, nil
}

// roundHalfUp は、正の有理数 x を、小数第1位（整数の位）で四捨五入した整数を返す。
// 浮動小数点を使わずに floor(x + 1/2) を、(2n + d) ÷ 2d の切り捨てで求める（x = n/d、n と d は正）。
func roundHalfUp(x *big.Rat) *big.Int {
	n, d := x.Num(), x.Denom()
	num := new(big.Int).Lsh(n, 1)
	num.Add(num, d)
	den := new(big.Int).Lsh(d, 1)
	return num.Quo(num, den)
}
