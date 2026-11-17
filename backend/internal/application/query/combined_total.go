package query

import (
	"fmt"
	"math/big"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// fxHundredths は、レート（0.01円単位の整数）と 0.01ドル単位の金額を掛けた値を、0.01円単位の金額にするための割る数。
// ドルの金額 x（0.01ドル単位）× レート R（0.01円単位）÷ 100 ＝ 円の金額（0.01円単位）。
const fxHundredths = 100

// combineInYen は、通貨ごとの合計 totals を、レート rate で円にまとめる（ドルを円にまとめる合計の要件2・要件6〜要件8・要件11・要件12。プラン 4.3）。
//
//   - 保有数量が1以上のドルの銘柄がない（totals にドルがない）か、rate が nil なら nil を返す（要件2）。
//   - 評価額は「円の評価額の合計 ＋ ドルの評価額の合計 × R」。ドルの分は合計に1回だけ R を掛け、1円未満を切り捨てる（要件6）。
//   - 含み損益は、価格のある通貨の分だけを足す（要件7・要件12）。円は表示する和そのまま、ドルは表示する和に R を掛けて
//     1円未満を絶対値で四捨五入する。どちらの通貨にも価格のある銘柄がなければ、含み損益と率は nil。
//   - 率は、丸める前の含み損益の和（ドルは R を掛けた値）÷ 取得額の和（同）を、通貨ごとの合計の率と同じ丸めにする（要件8）。
//   - ExcludesUnpriced は、円かドルのどちらかが true なら true（要件11）。
//
// 値が int64 に収まらなければ ErrCalculationOverflow を（包んで）返す。
func combineInYen(totals map[dto.Currency]*currencyTotal, rate *dto.UsdJpyRate) (*dto.CombinedTotal, error) {
	usd := totals[dto.CurrencyUSD]
	if usd == nil || rate == nil {
		return nil, nil
	}
	jpy := totals[dto.CurrencyJPY]
	r := big.NewInt(rate.RateHundredths)

	// 評価額（円は100の倍数）。ドルの分は 0.01円単位に直してから、1円（100）の倍数に切り捨てる。評価額は負にならないので、Quo は切り捨てと同じ。
	usdValuation := new(big.Int).Mul(usd.valuation, r)
	usdValuation.Quo(usdValuation, big.NewInt(fxHundredths*yenValuationUnitHundredths))
	usdValuation.Mul(usdValuation, big.NewInt(yenValuationUnitHundredths))
	valuation := new(big.Int).Add(usdValuation, jpyValuation(jpy))
	if !valuation.IsInt64() {
		return nil, fmt.Errorf("円にまとめた評価額: %w", ErrCalculationOverflow)
	}

	combined := &dto.CombinedTotal{
		ValuationHundredths: valuation.Int64(),
		ExcludesUnpriced:    usd.excludesUnpriced || (jpy != nil && jpy.excludesUnpriced),
		Rate:                *rate,
	}
	if err := applyCombinedGain(combined, jpy, usd, big.NewRat(rate.RateHundredths, fxHundredths)); err != nil {
		return nil, err
	}
	return combined, nil
}

// jpyValuation は、円の合計 jpy（なければ nil）の評価額（0.01 単位）を返す。円の保有がなければ0。
func jpyValuation(jpy *currencyTotal) *big.Int {
	if jpy == nil {
		return new(big.Int)
	}
	return jpy.valuation
}

// applyCombinedGain は、円にまとめた含み損益と率を combined に埋める。円・ドルのどちらにも価格のある銘柄がなければ、どちらも nil のまま。
// 価格のある通貨の分だけを足す（価格のない通貨は0）。ドルの値には、合計に1回だけ toYen（レート ÷ 100。0.01ドル単位を 0.01円単位にする倍率）を掛ける。
func applyCombinedGain(combined *dto.CombinedTotal, jpy, usd *currencyTotal, toYen *big.Rat) error {
	sum := newGainTotal()
	priced := false
	if jpy != nil && jpy.hasPriced {
		sum.add(&stockGain{gain: jpy.gain.gain, cost: jpy.gain.cost, displayed: jpy.gain.displayed})
		priced = true
	}
	if usd.hasPriced {
		// 表示する含み損益の和（ドル）は、円にして1円未満を絶対値で四捨五入する（要件7）。
		displayedYen := roundHalfUpAbs(new(big.Rat).Mul(new(big.Rat).SetInt(usd.gain.displayed), toYen), yenValuationUnitHundredths)
		sum.add(&stockGain{
			gain:      new(big.Rat).Mul(usd.gain.gain, toYen),
			cost:      new(big.Rat).Mul(usd.gain.cost, toYen),
			displayed: displayedYen,
		})
		priced = true
	}
	if !priced {
		return nil
	}
	if !sum.displayed.IsInt64() {
		return fmt.Errorf("円にまとめた含み損益: %w", ErrCalculationOverflow)
	}
	rate, err := sum.rateTenths()
	if err != nil {
		return fmt.Errorf("円にまとめた含み損益の率: %w", err)
	}
	combined.UnrealizedGainHundredths = new(sum.displayed.Int64())
	combined.UnrealizedGainRateTenths = new(rate)
	return nil
}
