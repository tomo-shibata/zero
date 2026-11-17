package query

import (
	"fmt"
	"math/big"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// gainRatePerMille は、率（%）を 0.1% 単位の整数にするための倍率（% の値 × 10 ＝ 比 × 100 × 10）。
const gainRatePerMille = 1000

// stockGain は、価格のある1銘柄の、丸める前の含み損益 gain と取得額 cost（どちらも 0.01 単位の有理数）と、表示する含み損益 displayed（0.01 単位）。
// 通貨ごとの合計は、displayed の和と、gain・cost それぞれの和から作る（カード表示の要件13）。
type stockGain struct {
	gain, cost *big.Rat
	displayed  *big.Int
}

// applyUnrealizedGain は、評価額 valuation（0.01 単位。表示する値）の銘柄 h に、含み損益と率を埋め、丸める前の値を返す（カード表示の要件7〜9）。
//
//	取得額 C ＝ 丸める前の取得価格 acquisition × 保有数量 quantity、含み損益 G ＝ 評価額 − C。
//	表示する含み損益 ＝ G を gainUnit（円は100、ドルは1）の倍数に絶対値で四捨五入した値。
//	率 ＝ G ÷ C × 100（%）を 0.1% 単位に絶対値で四捨五入した値。表示する含み損益が0なら、率も0にする（要件10）。
//
// 値が int64 に収まらなければ ErrCalculationOverflow を（包んで）返す。
func applyUnrealizedGain(h *dto.Holding, valuation *big.Int, acquisition *big.Rat, quantity *big.Int, gainUnit int64) (*stockGain, error) {
	cost := new(big.Rat).Mul(acquisition, new(big.Rat).SetInt(quantity))
	gain := new(big.Rat).Sub(new(big.Rat).SetInt(valuation), cost)

	displayed := roundHalfUpAbs(gain, gainUnit)
	if !displayed.IsInt64() {
		return nil, fmt.Errorf("含み損益: %w", ErrCalculationOverflow)
	}
	rate, err := gainRateTenths(gain, cost, displayed)
	if err != nil {
		return nil, err
	}
	h.UnrealizedGainHundredths = new(displayed.Int64())
	h.UnrealizedGainRateTenths = new(rate)
	return &stockGain{gain: gain, cost: cost, displayed: displayed}, nil
}

// gainRateTenths は、含み損益 gain ÷ 取得額 cost × 100（%）を、0.1% 単位の整数に絶対値で四捨五入した値を返す。
// 表示する含み損益 displayed が0なら、丸める前の率によらず0を返す（要件10。0円（-0.1%）のような表示にしないため）。
// 取得額は、価格が0より大きいという制約があるので正（0なら率は定まらないのでエラー）。
func gainRateTenths(gain, cost *big.Rat, displayed *big.Int) (int64, error) {
	if displayed.Sign() == 0 {
		return 0, nil
	}
	if cost.Sign() <= 0 {
		return 0, fmt.Errorf("取得額が0以下です")
	}
	rate := new(big.Rat).Mul(gain, big.NewRat(gainRatePerMille, 1))
	rate.Quo(rate, cost)
	tenths := roundHalfUpAbs(rate, 1)
	if !tenths.IsInt64() {
		return 0, fmt.Errorf("含み損益の率: %w", ErrCalculationOverflow)
	}
	return tenths.Int64(), nil
}

// roundHalfUpAbs は、有理数 x を、unit（正の整数）の倍数に、絶対値で四捨五入した値を返す（-500.666…円 → -501円。ちょうど半分は絶対値が大きい方へ）。
// 符号を外して roundHalfUp で丸め、符号を戻す。
func roundHalfUpAbs(x *big.Rat, unit int64) *big.Int {
	rounded := roundHalfUp(new(big.Rat).Abs(x), unit)
	if x.Sign() < 0 {
		rounded.Neg(rounded)
	}
	return rounded
}

// gainTotal は、1つの通貨の含み損益の合計を足していく途中の値（価格のある銘柄だけを足す。カード表示の要件13）。
type gainTotal struct {
	displayed  *big.Int // 表示する含み損益の和
	gain, cost *big.Rat // 丸める前の含み損益の和・取得額の和
}

func newGainTotal() *gainTotal {
	return &gainTotal{displayed: new(big.Int), gain: new(big.Rat), cost: new(big.Rat)}
}

func (t *gainTotal) add(g *stockGain) {
	t.displayed.Add(t.displayed, g.displayed)
	t.gain.Add(t.gain, g.gain)
	t.cost.Add(t.cost, g.cost)
}

// apply は、合計 total の含み損益と率を埋める。値が int64 に収まらなければ ErrCalculationOverflow を返す。
// 合計の率は、丸める前の含み損益の和 ÷ 取得額の和 × 100 を丸めた値（表示する含み損益の和が0なら0。丸めた率の符号が表示する含み損益の和と逆なら0）。
func (t *gainTotal) apply(total *dto.CurrencyTotal) error {
	if !t.displayed.IsInt64() {
		return fmt.Errorf("%s の含み損益の合計: %w", total.Currency, ErrCalculationOverflow)
	}
	rate, err := gainRateTenths(t.gain, t.cost, t.displayed)
	if err != nil {
		return fmt.Errorf("%s の含み損益の合計の率: %w", total.Currency, err)
	}
	// 丸めた率の符号が、表示する含み損益の和の符号と逆なら、率は0にする（プラン 判断15）。
	// 行ごとの丸めで和の符号が変わりうるので、要件10（率の符号は含み損益と同じ）と要件13（率は丸める前の和から）がここでだけぶつかる。
	if rate != 0 && (rate > 0) != (t.displayed.Sign() > 0) {
		rate = 0
	}
	total.UnrealizedGainHundredths = new(t.displayed.Int64())
	total.UnrealizedGainRateTenths = new(rate)
	return nil
}
