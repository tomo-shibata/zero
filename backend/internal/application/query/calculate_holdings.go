package query

import (
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
	// ErrCalculationOverflow は、一覧の最後の値（保有数量・取得価格・評価額・合計評価額・含み損益・率とその合計）が int64 に収まらないこと。
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

// CalculateHoldings は、利用者の売買履歴 rows と分割 splits から、保有株式一覧を作る（要件4・6〜15。移動平均法）。
//
//  1. 銘柄ごとに、売買履歴と分割を「日付の昇順 → 同じ日は分割が先 → id の昇順」にたどる（要件8、株式分割の要件6。rows・splits の並びには頼らない）。
//     rows にない銘柄の分割は使わない。
//  2. 保有数量 Q（整数）と取得価格 A（有理数）を持ち、買いでは A ← (Q×A ＋ 株数×価格) ÷ (Q ＋ 株数)、Q ← Q ＋ 株数、
//     売りでは Q ← Q − 株数（A は変えない）、分割では Q ← Q × 分割後の株数、A ← A ÷ 分割後の株数。途中では丸めない（要件6・要件7、株式分割の要件5）。
//  3. 最後の Q が0の銘柄（売り切った銘柄）は一覧に出さず、合計評価額にも、価格なしの注記にも響かせない（要件10）。
//  4. 表示する取得価格は、A を、円は 0.1円、ドルは 0.01ドルに四捨五入した値（要件7）。
//     評価額は「現在の価格 × Q」で、円は1円未満を切り捨て、ドルはそのまま（要件11）。
//  5. 一覧は、通貨によらず銘柄コードの文字列としての昇順（要件15）。
//     合計評価額は、通貨ごとに、その通貨の評価額のある行を足す（要件12・13。為替の換算はしない）。
//     価格のない行がある通貨は、その通貨の ExcludesUnpriced を true にする。
//     Totals は、保有のある通貨だけを、円、ドルの順に入れる。
//  6. 価格のある行には、含み損益と率を付ける（unrealized_gain.go。カード表示の要件7〜10）。
//     通貨ごとの含み損益の合計は、価格のある行の表示する含み損益の和、率は丸める前の値の和から出す（要件13）。
//  7. その銘柄の最も新しい分割の日が、最新の終値の取引日より後なら、その銘柄は価格なしとして扱う（株式分割の要件8。
//     終値が分割の前の値段かもしれないため）。合計評価額の注記と含み損益も、価格のない銘柄と同じになる。
//  8. 保有数量が1以上のドルの銘柄があり、かつレート rate があれば、通貨ごとの合計を円にまとめた CombinedTotal を付ける（combined_total.go。
//     ドルを円にまとめる合計の要件2・要件6〜要件8）。どちらかが欠けるときは nil（rate が nil なら、今までと同じ結果になる）。
//  9. おまかせ運用の口座 accounts（口座ごとの最新の記録。並びは決めない）があれば、カードの値を名前の昇順で ManagedAccounts に入れ、
//     円の合計（評価額・含み損益・取得額）に足す（managed_account_values.go。おまかせ運用の要件12・要件15〜要件18）。
//     口座が1つ以上あれば Totals に円が必ず入り、円にまとめた合計にも口座が含まれる。accounts が nil・空なら、ManagedAccounts は空のスライスで、ほかは今までと同じ結果になる。
//
// 計算は math/big で行い、桁あふれしないようにする。保有数量が途中で負になる銘柄があれば ErrNegativeQuantity、
// 最後の値が int64 に収まらなければ ErrCalculationOverflow を（包んで）返す。値そのものは資産の額なので、エラー（ログ）には入れない。
// rows が nil でも空でも、Holdings は空のスライス（nil ではない）にする。
func CalculateHoldings(rows []dto.TradeRow, splits []dto.StockSplitRow, rate *dto.UsdJpyRate, accounts []dto.ManagedAccountRow) (dto.HoldingsList, error) {
	byCode := make(map[string][]dto.TradeRow)
	for _, r := range rows {
		byCode[r.Code] = append(byCode[r.Code], r)
	}
	splitsOf := splitsByCode(splits)

	list := dto.HoldingsList{Holdings: make([]dto.Holding, 0, len(byCode))}
	totals := make(map[dto.Currency]*currencyTotal)
	// slices.Sorted は文字列を < で比べるので、Go の文字列としての昇順（"1301" < "130A" < "7203"）になる。
	// あふれる銘柄が複数あっても、毎回同じ銘柄をエラーに入れるため、エラーもこの順に見つける。
	for _, code := range slices.Sorted(maps.Keys(byCode)) {
		s, held, err := calculateStock(byCode[code], splitsOf[code])
		if err != nil {
			return dto.HoldingsList{}, fmt.Errorf("銘柄 %q: %w", code, err)
		}
		if !held {
			continue
		}
		h := s.holding
		list.Holdings = append(list.Holdings, h)
		t := totals[h.Currency]
		if t == nil {
			t = &currencyTotal{valuation: new(big.Int), gain: newGainTotal()}
			totals[h.Currency] = t
		}
		if s.valuation == nil {
			t.excludesUnpriced = true
			continue
		}
		t.valuation.Add(t.valuation, s.valuation)
		t.hasPriced = true
		t.gain.add(s.gain)
		// 足すたびに確かめ、収まらなくなった銘柄をエラーに入れる。評価額は負にならないので、途中で収まらなければ最後の値も収まらない。
		// （含み損益の合計は符号が変わるので、途中ではなく最後に確かめる。buildTotals）
		if !t.valuation.IsInt64() {
			return dto.HoldingsList{}, fmt.Errorf("銘柄 %q までの%s の合計評価額: %w", code, h.Currency, ErrCalculationOverflow)
		}
	}
	var err error
	// 口座の分を円の途中の値に足してから、通貨ごとの合計と円にまとめた合計を作る（どちらも口座を含んだ円の値を使う）。
	list.ManagedAccounts, err = buildManagedAccounts(accounts, totals)
	if err != nil {
		return dto.HoldingsList{}, err
	}
	list.Totals, err = buildTotals(totals)
	if err != nil {
		return dto.HoldingsList{}, err
	}
	// 通貨ごとの途中の値（丸める前の含み損益の和・取得額の和）を使うので、ここから呼ぶ（プラン 判断2）。
	list.CombinedTotal, err = combineInYen(totals, rate)
	if err != nil {
		return dto.HoldingsList{}, err
	}
	return list, nil
}

// buildTotals は、通貨ごとの合計 totals から、保有のある通貨だけを、円、ドルの順に並べた一覧を返す。
// 保有のある通貨がなければ、nil ではなく空のスライスを返す。
// 価格のある銘柄がない通貨は、含み損益の合計と率を nil にする（カード表示の要件16）。含み損益の合計が int64 に収まらなければ ErrCalculationOverflow を返す。
func buildTotals(totals map[dto.Currency]*currencyTotal) ([]dto.CurrencyTotal, error) {
	result := []dto.CurrencyTotal{}
	for _, currency := range []dto.Currency{dto.CurrencyJPY, dto.CurrencyUSD} {
		t := totals[currency]
		if t == nil {
			continue
		}
		total := dto.CurrencyTotal{
			Currency:            currency,
			ValuationHundredths: t.valuation.Int64(),
			ExcludesUnpriced:    t.excludesUnpriced,
		}
		if t.hasPriced {
			if err := t.gain.apply(&total); err != nil {
				return nil, err
			}
		}
		result = append(result, total)
	}
	return result, nil
}

// currencyTotal は、1つの通貨の合計評価額と含み損益の合計を足していく途中の値。
type currencyTotal struct {
	valuation        *big.Int   // 評価額のある行の合計（0.01 単位）
	gain             *gainTotal // 価格のある行の含み損益の合計
	hasPriced        bool       // 価格のある行が1件以上ある
	excludesUnpriced bool       // 価格のない行が1件以上ある
}

// calculatedStock は、1銘柄の計算の結果。
type calculatedStock struct {
	holding   dto.Holding
	valuation *big.Int   // 評価額（0.01 単位。円は1円未満を切り捨てた値）。価格がなければ nil
	gain      *stockGain // 丸める前の含み損益と取得額（合計に足す元）。価格がなければ nil
}

// calculateStock は、同じ銘柄の売買履歴 rows（1件以上）と、その銘柄の分割 splits をたどって、一覧の1行を作る。
// 保有数量が0（売り切った銘柄）なら held が false で、ほかの戻り値は使わない。
func calculateStock(rows []dto.TradeRow, splits []dto.StockSplitRow) (s calculatedStock, held bool, err error) {
	quantity, acquisition, err := replayTrades(traceEvents(rows, splits))
	if err != nil {
		return calculatedStock{}, false, err
	}
	if quantity.Sign() == 0 {
		return calculatedStock{}, false, nil
	}
	if !quantity.IsInt64() {
		return calculatedStock{}, false, fmt.Errorf("保有数量: %w", ErrCalculationOverflow)
	}

	// 銘柄名・通貨と最新の終値・基準日は銘柄ごとに決まる値で、どの行も同じなので、先頭の行から取る。
	first := rows[0]
	h := dto.Holding{Code: first.Code, Name: first.Name, Currency: first.Currency, Quantity: quantity.Int64()}

	priceUnit, valuationUnit, err := roundingUnits(first.Currency)
	if err != nil {
		return calculatedStock{}, false, err
	}
	acquisitionHundredths := roundHalfUp(acquisition, priceUnit)
	if !acquisitionHundredths.IsInt64() {
		return calculatedStock{}, false, fmt.Errorf("取得価格: %w", ErrCalculationOverflow)
	}
	h.AcquisitionPriceHundredths = acquisitionHundredths.Int64()

	// 最も新しい分割の日が最新の終値の取引日より後なら、終値が分割の前の値段かもしれないので、価格なしにする（株式分割の要件8）。
	var valuation *big.Int
	if !closePriceBeforeSplit(first, splits) {
		valuation, err = applyClosePrice(&h, first, quantity, valuationUnit)
		if err != nil {
			return calculatedStock{}, false, err
		}
	}
	s = calculatedStock{holding: h, valuation: valuation}
	if valuation != nil {
		// 含み損益の丸めの単位は、評価額の単位と同じ（円は1円、ドルは0.01ドル。カード表示の要件8）。
		s.gain, err = applyUnrealizedGain(&s.holding, valuation, acquisition, quantity, valuationUnit)
		if err != nil {
			return calculatedStock{}, false, err
		}
	}
	return s, true, nil
}

// closePriceBeforeSplit は、その銘柄の最新の終値の取引日が、最も新しい分割の日より前なら true を返す（株式分割の要件8）。
// 同じ日なら false（分割の日の終値は分割の後の値段）。終値がない、または分割がなければ false。
func closePriceBeforeSplit(first dto.TradeRow, splits []dto.StockSplitRow) bool {
	if first.PriceDate == nil {
		return false
	}
	latest, ok := latestSplitDate(splits)
	return ok && latest > *first.PriceDate
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

// replayTrades は、たどる順に並べた売買履歴と分割 events（traceEvents の結果）を順にたどり、最後の保有数量 Q と取得価格 A（0.01 単位。丸めない）を返す。
// 分割では Q に分割後の株数を掛け、A をその株数で割る（保有数量が0のときは A は次の買いで置き換わるので、割っても響かない）。
// 売りで Q が負になれば ErrNegativeQuantity を返す。
func replayTrades(events []traceEvent) (quantity *big.Int, acquisition *big.Rat, err error) {
	quantity = new(big.Int)
	acquisition = new(big.Rat)
	for _, e := range events {
		if !e.isTrade {
			ratio := big.NewInt(e.ratio)
			quantity.Mul(quantity, ratio)
			acquisition = new(big.Rat).Quo(acquisition, new(big.Rat).SetInt(ratio))
			continue
		}
		r := e.trade
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
