package query

import (
	"cmp"
	"fmt"
	"math/big"
	"slices"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// 条件の種類・対象・向きの文字列。domain の model.AlertKind・AlertScope・AlertDirection と同じ値
// （Query 側は domain を通さないので、ここに持つ。DB の alert_conditions の CHECK とも同じ）。
const (
	alertKindCost  = "cost"
	alertKindPeak  = "peak"
	alertKindPrev  = "prev"
	alertKindPnl   = "pnl"
	alertKindPrice = "price"

	alertScopeAll = "all"

	alertDirectionUp     = "up"
	alertDirectionDown   = "down"
	alertDirectionEither = "either"
	alertDirectionLoss   = "loss"
	alertDirectionGain   = "gain"
	alertDirectionAbove  = "above"
	alertDirectionBelow  = "below"
)

// rateTenthsToThreshold は、率（0.1% 単位）を条件の値（0.01% 単位）の単位にそろえる倍率。
const rateTenthsToThreshold = 10

// alertOutcome は、1つの条件の判定の結果。
type alertOutcome struct {
	evaluated bool // 判定できた。前の取引日からの条件で前の終値がないときは false（要件13）
	satisfied bool
	details   dto.AlertNotificationDetails // 当てはまったときだけ使う
}

// evaluateCondition は、保有 uh の条件 cond が、D の終値で当てはまるかを出す（プラン 4.3 の比べ方の表。要件11〜要件15）。
func evaluateCondition(in dto.AlertEvaluationInput, divisors *splitDivisors, uh *userHolding, cond dto.AlertConditionRow) (alertOutcome, error) {
	switch cond.Kind {
	case alertKindCost:
		return evaluateCost(uh.holding, cond)
	case alertKindPeak:
		return evaluatePeak(in, divisors, uh, cond)
	case alertKindPrev:
		return evaluatePrev(in, divisors, cond)
	case alertKindPnl:
		return evaluatePnl(uh.holding, cond)
	case alertKindPrice:
		return evaluatePrice(in, divisors, cond)
	default:
		return alertOutcome{}, fmt.Errorf("知らない条件の種類 %q です", cond.Kind)
	}
}

// evaluateCost は、取得価格からの率（今のカードの率。0.1% 単位）が値に届いたかを出す（要件11）。
func evaluateCost(h dto.Holding, cond dto.AlertConditionRow) (alertOutcome, error) {
	if h.UnrealizedGainRateTenths == nil {
		return alertOutcome{}, fmt.Errorf("含み損益の率がありません")
	}
	rate := *h.UnrealizedGainRateTenths
	satisfied, err := rateReaches(rate, cond)
	if err != nil {
		return alertOutcome{}, err
	}
	return alertOutcome{evaluated: true, satisfied: satisfied, details: dto.AlertNotificationDetails{
		RateTenths: new(rate), BasePriceHundredths: new(h.AcquisitionPriceHundredths),
	}}, nil
}

// evaluatePeak は、今の保有が始まった日 S 以後 D 以前の終値（分割で割った値）の最高値からの率が、値に届いたかを出す（要件12）。
// 最高値が同じ値の日が複数あれば、新しい日を最高値の日にする。
func evaluatePeak(in dto.AlertEvaluationInput, divisors *splitDivisors, uh *userHolding, cond dto.AlertConditionRow) (alertOutcome, error) {
	// 最高値は利用者ごとに1つ（最高値の条件が何件あっても、終値の列を数えるのは1回）。
	if uh.peak == nil {
		peak := peakSince(in, divisors, uh.start)
		uh.peak = &peak
	}
	return evaluateRateFromBase(in, uh.peak.closeHundredths, uh.peak.divisor, uh.peak.date, cond)
}

// evaluateRateFromBase は、基準の日（最高値の日・前の取引日）の終値 baseClose（割る前）を、その日より後の分割の株数の積 divisor で割った価格からの率が、
// 条件 cond の値に届いたかを出す。通知の値には、率と、割った基準の価格（表示の丸め）と、基準の日 baseDate を持つ。
func evaluateRateFromBase(in dto.AlertEvaluationInput, baseClose int64, divisor *big.Int, baseDate string, cond dto.AlertConditionRow) (alertOutcome, error) {
	rate, err := rateTenthsFrom(in.ClosePriceHundredths, new(big.Rat).SetFrac(big.NewInt(baseClose), divisor))
	if err != nil {
		return alertOutcome{}, err
	}
	satisfied, err := rateReaches(rate, cond)
	if err != nil {
		return alertOutcome{}, err
	}
	priceUnit, _, err := roundingUnits(in.Currency)
	if err != nil {
		return alertOutcome{}, err
	}
	basePrice, err := adjustPrice(baseClose, divisor, priceUnit)
	if err != nil {
		return alertOutcome{}, err
	}
	return alertOutcome{evaluated: true, satisfied: satisfied, details: dto.AlertNotificationDetails{
		RateTenths: new(rate), BasePriceHundredths: new(basePrice), BaseDate: new(baseDate),
	}}, nil
}

// peakPoint は、最高値の日の、割る前の終値・取引日・その日より後の分割の株数の積（割る数）。
type peakPoint struct {
	closeHundredths int64
	date            string
	divisor         *big.Int
}

// peakSince は、S 以後 D 以前の終値を、その日より後の分割の株数の積で割った値のうち最大のものを返す。
// 入力の列（Closes）のうち D 以後の行は使わず（読んだ後に D の終値が保存し直されても食い違わないように）、D の値はいつも in.ClosePriceHundredths を数える。
func peakSince(in dto.AlertEvaluationInput, divisors *splitDivisors, start string) peakPoint {
	candidates := make([]dto.ClosingPriceRow, 0, len(in.Closes)+1)
	for _, c := range in.Closes {
		if c.TradingDate < start || c.TradingDate >= in.TradingDate {
			continue
		}
		candidates = append(candidates, c)
	}
	candidates = append(candidates, dto.ClosingPriceRow{TradingDate: in.TradingDate, ClosePriceHundredths: in.ClosePriceHundredths})
	slices.SortFunc(candidates, func(a, b dto.ClosingPriceRow) int { return cmp.Compare(a.TradingDate, b.TradingDate) })

	var best *big.Rat
	var peak peakPoint
	for _, c := range candidates {
		d := divisors.after(c.TradingDate)
		v := new(big.Rat).SetFrac(big.NewInt(c.ClosePriceHundredths), d)
		// 昇順に見て、同じ値なら後（新しい日）で置き換える。
		if best == nil || v.Cmp(best) >= 0 {
			best, peak = v, peakPoint{closeHundredths: c.ClosePriceHundredths, date: c.TradingDate, divisor: d}
		}
	}
	return peak
}

// evaluatePrev は、前の終値（D との間の分割で割った値）からの率が、向きごとに値に届いたかを出す（要件13）。前の終値がなければ判定しない。
func evaluatePrev(in dto.AlertEvaluationInput, divisors *splitDivisors, cond dto.AlertConditionRow) (alertOutcome, error) {
	if in.PreviousClose == nil {
		return alertOutcome{}, nil
	}
	prev := *in.PreviousClose
	return evaluateRateFromBase(in, prev.ClosePriceHundredths, divisors.after(prev.TradingDate), prev.TradingDate, cond)
}

// evaluatePnl は、今のカードの含み損益（表示する値）が値に届いたかを出す（要件14）。
func evaluatePnl(h dto.Holding, cond dto.AlertConditionRow) (alertOutcome, error) {
	if h.UnrealizedGainHundredths == nil {
		return alertOutcome{}, fmt.Errorf("含み損益がありません")
	}
	gain := *h.UnrealizedGainHundredths
	var satisfied bool
	switch cond.Direction {
	case alertDirectionLoss:
		satisfied = gain <= -cond.ThresholdHundredths
	case alertDirectionGain:
		satisfied = gain >= cond.ThresholdHundredths
	default:
		return alertOutcome{}, fmt.Errorf("含み損益の金額の向き %q が正しくありません", cond.Direction)
	}
	return alertOutcome{evaluated: true, satisfied: satisfied, details: dto.AlertNotificationDetails{
		UnrealizedGainHundredths: new(gain), BasePriceHundredths: new(h.AcquisitionPriceHundredths), Quantity: new(h.Quantity),
	}}, nil
}

// evaluatePrice は、終値が決めた価格に届いたかを出す（要件15）。
// 割る数は、条件を足した日（日本時間）より後で D 以前の分割の株数の積。終値 × 割る数を値と整数で比べ、丸めない。
// 割る数が1より大きいときだけ、割った値（表示の丸め）を通知の値に持つ。
func evaluatePrice(in dto.AlertEvaluationInput, divisors *splitDivisors, cond dto.AlertConditionRow) (alertOutcome, error) {
	divisor := divisors.after(cond.AddedOn)
	scaledClose := new(big.Int).Mul(big.NewInt(in.ClosePriceHundredths), divisor)
	threshold := big.NewInt(cond.ThresholdHundredths)
	var satisfied bool
	switch cond.Direction {
	case alertDirectionAbove:
		satisfied = scaledClose.Cmp(threshold) >= 0
	case alertDirectionBelow:
		satisfied = scaledClose.Cmp(threshold) <= 0
	default:
		return alertOutcome{}, fmt.Errorf("決めた価格の向き %q が正しくありません", cond.Direction)
	}
	outcome := alertOutcome{evaluated: true, satisfied: satisfied}
	if satisfied && divisor.Cmp(big.NewInt(1)) > 0 {
		priceUnit, _, err := roundingUnits(in.Currency)
		if err != nil {
			return alertOutcome{}, err
		}
		adjusted, err := adjustPrice(cond.ThresholdHundredths, divisor, priceUnit)
		if err != nil {
			return alertOutcome{}, err
		}
		outcome.details.AdjustedThresholdHundredths = new(adjusted)
	}
	return outcome, nil
}

// rateTenthsFrom は、終値 closeHundredths の、基準の価格 base（0.01 単位の正の有理数）からの率 (終値 − 基準) ÷ 基準 × 100（%）を、
// 0.1% 単位の整数に絶対値で四捨五入して返す（gainRateTenths と同じ丸め）。
func rateTenthsFrom(closeHundredths int64, base *big.Rat) (int64, error) {
	diff := new(big.Rat).Sub(new(big.Rat).SetInt64(closeHundredths), base)
	rate := diff.Mul(diff, big.NewRat(gainRatePerMille, 1))
	rate.Quo(rate, base)
	tenths := roundHalfUpAbs(rate, 1)
	if !tenths.IsInt64() {
		return 0, fmt.Errorf("率: %w", ErrCalculationOverflow)
	}
	return tenths.Int64(), nil
}

// rateReaches は、丸めた率 rateTenths（0.1% 単位）が、条件の向きと値（0.01% 単位）に届いたかを返す。
// up は 値以上、down は −値以下、either は絶対値が値以上。
func rateReaches(rateTenths int64, cond dto.AlertConditionRow) (bool, error) {
	// 率は 0.1% 単位、値は 0.01% 単位なので、率を10倍して比べる（率が大きくても int64 に収まる範囲しか来ないが、あふれないよう big で比べる）。
	scaled := new(big.Int).Mul(big.NewInt(rateTenths), big.NewInt(rateTenthsToThreshold))
	threshold := big.NewInt(cond.ThresholdHundredths)
	switch cond.Direction {
	case alertDirectionUp:
		return scaled.Cmp(threshold) >= 0, nil
	case alertDirectionDown:
		return scaled.Cmp(new(big.Int).Neg(threshold)) <= 0, nil
	case alertDirectionEither:
		return scaled.CmpAbs(threshold) >= 0, nil
	default:
		return false, fmt.Errorf("率の条件の向き %q が正しくありません", cond.Direction)
	}
}
