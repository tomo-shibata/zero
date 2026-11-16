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

// 円の金額の丸め方の単位（0.01 単位の整数で数える）。
const (
	// yenPriceUnitHundredths は、円の価格（単価）の最小の単位 0.1円。取得価格は、これの倍数に四捨五入する（要件7）。
	yenPriceUnitHundredths = 10
	// yenValuationUnitHundredths は、円の評価額の最小の単位 1円。評価額は、これの倍数に切り捨てる（要件11）。
	yenValuationUnitHundredths = 100
	// usdUnitHundredths は、ドルの価格・評価額の最小の単位 0.01ドル。取得価格も評価額も、これ（丸めない単位）の倍数にする（要件7・要件11）。
	usdUnitHundredths = 1
)

// CalculateHoldings は、利用者の売買履歴 rows から、保有株式一覧を作る（要件4・6〜15。移動平均法）。
//
//  1. 銘柄ごとに、売買履歴を「取引日の昇順 → id の昇順」にたどる（要件8。rows の並びには頼らない）。
//  2. 保有数量 Q（整数）と取得価格 A（有理数）を持ち、買いでは A ← (Q×A ＋ 株数×価格) ÷ (Q ＋ 株数)、Q ← Q ＋ 株数、
//     売りでは Q ← Q − 株数（A は変えない）。途中では丸めない（要件6・要件7）。
//  3. 最後の Q が0の銘柄（売り切った銘柄）は一覧に出さず、合計評価額にも、価格なしの注記にも響かせない（要件10）。
//  4. 表示する取得価格は、A を、円は 0.1円、ドルは 0.01ドルに四捨五入した値（要件7）。
//     評価額は「現在の価格 × Q」で、円は1円未満を切り捨て、ドルはそのまま（要件11）。
//  5. 一覧は、通貨によらず銘柄コードの文字列としての昇順（要件15）。
//     合計評価額は、通貨ごとに、その通貨の評価額のある行を足す（要件12・13。為替の換算はしない）。
//     価格のない行がある通貨は、その通貨の ExcludesUnpriced を true にする。
//     Totals は、保有のある通貨だけを、円、ドルの順に入れる。
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
	totals := make(map[dto.Currency]*currencyTotal)
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
		t := totals[h.Currency]
		if t == nil {
			t = &currencyTotal{valuation: new(big.Int)}
			totals[h.Currency] = t
		}
		if valuation == nil {
			t.excludesUnpriced = true
			continue
		}
		t.valuation.Add(t.valuation, valuation)
		// 足すたびに確かめ、収まらなくなった銘柄をエラーに入れる。評価額は負にならないので、途中で収まらなければ最後の値も収まらない。
		if !t.valuation.IsInt64() {
			return dto.HoldingsList{}, fmt.Errorf("銘柄 %q までの%s の合計評価額: %w", code, h.Currency, ErrCalculationOverflow)
		}
	}
	list.Totals = buildTotals(totals)
	return list, nil
}

// buildTotals は、通貨ごとの合計 totals から、保有のある通貨だけを、円、ドルの順に並べた一覧を返す。
// 保有のある通貨がなければ、nil ではなく空のスライスを返す。
func buildTotals(totals map[dto.Currency]*currencyTotal) []dto.CurrencyTotal {
	result := []dto.CurrencyTotal{}
	for _, currency := range []dto.Currency{dto.CurrencyJPY, dto.CurrencyUSD} {
		if t := totals[currency]; t != nil {
			result = append(result, dto.CurrencyTotal{
				Currency:            currency,
				ValuationHundredths: t.valuation.Int64(),
				ExcludesUnpriced:    t.excludesUnpriced,
			})
		}
	}
	return result
}

// currencyTotal は、1つの通貨の合計評価額を足していく途中の値。
type currencyTotal struct {
	valuation        *big.Int // 評価額のある行の合計（0.01 単位）
	excludesUnpriced bool     // 価格のない行が1件以上ある
}

// calculateStock は、同じ銘柄の売買履歴 rows（1件以上）をたどって、一覧の1行を作る。
// 保有数量が0（売り切った銘柄）なら held が false で、ほかの戻り値は使わない。
// valuation は、評価額（0.01 単位。円は1円未満を切り捨てた値）。価格がなければ nil。
func calculateStock(rows []dto.TradeRow) (h dto.Holding, valuation *big.Int, held bool, err error) {
	rows = sortTrades(rows)
	quantity, acquisition, err := replayTrades(rows)
	if err != nil {
		return dto.Holding{}, nil, false, err
	}
	if quantity.Sign() == 0 {
		return dto.Holding{}, nil, false, nil
	}
	if !quantity.IsInt64() {
		return dto.Holding{}, nil, false, fmt.Errorf("保有数量: %w", ErrCalculationOverflow)
	}

	// 銘柄名・通貨と最新の終値・基準日は銘柄ごとに決まる値で、どの行も同じなので、並べた後の先頭の行から取る。
	first := rows[0]
	h = dto.Holding{Code: first.Code, Name: first.Name, Currency: first.Currency, Quantity: quantity.Int64()}

	priceUnit, valuationUnit, err := roundingUnits(first.Currency)
	if err != nil {
		return dto.Holding{}, nil, false, err
	}
	acquisitionHundredths := roundHalfUp(acquisition, priceUnit)
	if !acquisitionHundredths.IsInt64() {
		return dto.Holding{}, nil, false, fmt.Errorf("取得価格: %w", ErrCalculationOverflow)
	}
	h.AcquisitionPriceHundredths = acquisitionHundredths.Int64()

	valuation, err = applyClosePrice(&h, first, quantity, valuationUnit)
	if err != nil {
		return dto.Holding{}, nil, false, err
	}
	return h, valuation, true, nil
}

// sortTrades は、同じ銘柄の売買履歴 rows を写して、たどる順（取引日の昇順 → id の昇順）に並べたものを返す。
// 呼び出し側の rows は並べ替えない。
func sortTrades(rows []dto.TradeRow) []dto.TradeRow {
	sorted := slices.Clone(rows)
	slices.SortFunc(sorted, func(a, b dto.TradeRow) int {
		// 取引日は "YYYY-MM-DD" なので、文字列の昇順が日付の古い順になる。
		return cmp.Or(cmp.Compare(a.TradeDate, b.TradeDate), cmp.Compare(a.ID, b.ID))
	})
	return sorted
}

// roundingUnits は、通貨 currency の、取得価格を四捨五入する単位 priceUnit と、評価額を切り捨てる単位 valuationUnit（どちらも 0.01 単位の整数）を返す。
// 円は取得価格を 0.1円、評価額を 1円の単位にし、ドルはどちらも 0.01ドル（丸めない単位）にする。知らない通貨はエラー。
func roundingUnits(currency dto.Currency) (priceUnit, valuationUnit int64, err error) {
	switch currency {
	case dto.CurrencyJPY:
		return yenPriceUnitHundredths, yenValuationUnitHundredths, nil
	case dto.CurrencyUSD:
		return usdUnitHundredths, usdUnitHundredths, nil
	default:
		return 0, 0, fmt.Errorf("知らない通貨 %q です", currency)
	}
}

// replayTrades は、たどる順に並べた売買履歴 rows（sortTrades の結果）を順にたどり、最後の保有数量 Q と取得価格 A（0.01 単位。丸めない）を返す。
// 売りで Q が負になれば ErrNegativeQuantity を返す。
func replayTrades(rows []dto.TradeRow) (quantity *big.Int, acquisition *big.Rat, err error) {
	quantity = new(big.Int)
	acquisition = new(big.Rat)
	for _, r := range rows {
		qty := big.NewInt(r.Quantity)
		switch r.Side {
		case dto.TradeSideBuy:
			// 保有数量が0のときは Q×A が0になり、A はこの買いの価格になる（要件6）。買った後の Q は1以上なので割れる。
			cost := new(big.Rat).Mul(new(big.Rat).SetInt(quantity), acquisition)
			cost.Add(cost, new(big.Rat).SetInt(new(big.Int).Mul(qty, big.NewInt(r.PriceHundredths))))
			quantity.Add(quantity, qty)
			acquisition = cost.Quo(cost, new(big.Rat).SetInt(quantity))
		case dto.TradeSideSell:
			quantity.Sub(quantity, qty)
			if quantity.Sign() < 0 {
				return nil, nil, ErrNegativeQuantity
			}
		default:
			return nil, nil, fmt.Errorf("知らない売買の種類 %q です", r.Side)
		}
	}
	return quantity, acquisition, nil
}

// applyClosePrice は、最新の終値 first があれば、h の現在の価格・基準日・評価額を埋め、評価額（0.01 単位）を返す。
// 価格がなければ、h の3つはどれも nil のまま、評価額も nil を返す（保有株式一覧の FR-10）。
// 値を写してから指すのは、返した一覧を書き換えても、呼び出し側の rows が変わらないようにするため。
func applyClosePrice(h *dto.Holding, first dto.TradeRow, quantity *big.Int, valuationUnit int64) (*big.Int, error) {
	if first.ClosePriceHundredths == nil || first.PriceDate == nil {
		return nil, nil
	}
	h.CurrentPriceHundredths = new(*first.ClosePriceHundredths)
	h.PriceDate = new(*first.PriceDate)
	// 評価額は、保有数量で1回だけ計算し、円は1円未満を切り捨てる（要件11。ドルは valuationUnit が1なので、そのまま）。
	// Quo は 0 の方向に切り捨てる（値は負にならない）。
	unit := big.NewInt(valuationUnit)
	valuation := new(big.Int).Mul(big.NewInt(*first.ClosePriceHundredths), quantity)
	valuation.Quo(valuation, unit)
	valuation.Mul(valuation, unit)
	if !valuation.IsInt64() {
		return nil, fmt.Errorf("評価額: %w", ErrCalculationOverflow)
	}
	h.ValuationHundredths = new(valuation.Int64())
	return valuation, nil
}

// roundHalfUp は、正の有理数 x を、unit（正の整数）の倍数に四捨五入した値を返す（ちょうど半分は切り上げる）。
// 浮動小数点を使わずに floor(x ÷ unit + 1/2) × unit を、floor((2n + u×d) ÷ 2u×d) × unit の整数の計算で求める（x = n/d、n・d・u は正）。
func roundHalfUp(x *big.Rat, unit int64) *big.Int {
	n, d := x.Num(), x.Denom()
	u := big.NewInt(unit)
	ud := new(big.Int).Mul(u, d)
	num := new(big.Int).Lsh(n, 1)
	num.Add(num, ud)
	den := new(big.Int).Lsh(ud, 1)
	num.Quo(num, den)
	return num.Mul(num, u)
}
