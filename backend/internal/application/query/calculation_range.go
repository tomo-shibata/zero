package query

import (
	"errors"
	"fmt"
	"maps"
	"math/big"
	"slices"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// errCalculationOverflow は、一覧を作る計算の値が int64 に収まらないことを表す。
var errCalculationOverflow = errors.New("int64 に収まりません")

// checkCalculationRange は、rows から一覧を作る計算（AggregateHoldings）の値が、すべて int64 に収まるかを確かめる。
// 収まらなければ、どの銘柄のどの値かを入れたエラーを返す。値そのものは資産の額なので、エラー（ログ）には入れない。
//
// AggregateHoldings・WeightedAverageTenths・Valuation は int64 で計算するので、収まらないと黙って桁あふれして、
// 誤った値（負の評価額など）を返してしまう。現実の株価と数量では起きないが、seed は手で書くので桁の誤りが入りうる
// （プラン 6章の判断14）。境界の関数の形（エラーを返さない）を変えずに防ぐため、集約の前に同じ計算を、
// 桁あふれしない math/big でやってみて確かめる。
//
// 確かめるのは、割り算の前の値と、最後に返す値（プラン 4.4「API」）:
//   - 銘柄ごとの保有数量の合計 Q
//   - 取得価格の加重平均の割られる数 2S+Q と割る数 2Q（S = Σ(数量×取得価格)。WeightedAverageTenths）
//   - 評価額の割られる数 終値×Q（Valuation）
//   - 合計評価額
//
// 銘柄ごとの足し算と掛け算だけでできた途中の値（S や、足している途中の Q）は確かめない。Go の整数は 2^64 を法として
// 計算するので、途中で桁あふれしても、最後の値が int64 に収まれば正しい値になるため。割り算は法の計算と合わないので、その前で確かめる。
func checkCalculationRange(rows []dto.HoldingRow) error {
	sums := sumByStock(rows)
	total := new(big.Int) // 合計評価額（円）
	// AggregateHoldings と同じく銘柄コードの昇順に確かめる。あふれる銘柄が複数あっても、毎回同じ銘柄をエラーに入れるため。
	for _, code := range slices.Sorted(maps.Keys(sums)) {
		valuation, err := sums[code].checkRange()
		if err != nil {
			return fmt.Errorf("銘柄 %s の%w", code, err)
		}
		total.Add(total, valuation)
		// 合計評価額は、最後の値だけでなく足すたびに確かめ、収まらなくなった銘柄をエラーに入れる。
		// 評価額は負にならない（DB の CHECK で数量と終値は正）ので、途中で収まらなければ最後の値も収まらず、結果は同じになる。
		if !total.IsInt64() {
			return fmt.Errorf("銘柄 %s までの合計評価額が %w", code, errCalculationOverflow)
		}
	}
	return nil
}

// stockSums は、1銘柄の保有データを、桁あふれしない整数で足し合わせたもの。
type stockSums struct {
	quantity    *big.Int // Q = Σ数量
	cost        *big.Int // S = Σ(数量×取得価格の Tenths)
	closeTenths *int64   // 評価額の計算に使う終値（Tenths）。価格がなければ nil
}

// sumByStock は、rows を銘柄コードごとに足し合わせる。
func sumByStock(rows []dto.HoldingRow) map[string]*stockSums {
	sums := make(map[string]*stockSums)
	for _, r := range rows {
		s, ok := sums[r.Code]
		if !ok {
			s = &stockSums{quantity: new(big.Int), cost: new(big.Int)}
			// 終値は aggregateStock と同じく、その銘柄の先頭の行に終値と基準日の両方があるとき（hasPrice）だけ使う。
			// 評価額を計算する銘柄をそろえないと、計算しない評価額であふれを見つけたり、計算する評価額を見落としたりするため。
			if hasPrice(r) {
				s.closeTenths = r.ClosePriceTenths
			}
			sums[r.Code] = s
		}
		quantity := big.NewInt(r.Quantity)
		s.quantity.Add(s.quantity, quantity)
		s.cost.Add(s.cost, new(big.Int).Mul(quantity, big.NewInt(r.AcquisitionPriceTenths)))
	}
	return sums
}

// checkRange は、この銘柄の Q、2S+Q と 2Q、終値×Q が int64 に収まるかを確かめ、評価額（円）を返す。
// 価格がない銘柄は評価額を計算しない（合計にも足さない）ので、0 を返す。
func (s *stockSums) checkRange() (*big.Int, error) {
	if !s.quantity.IsInt64() {
		return nil, fmt.Errorf("保有数量の合計が %w", errCalculationOverflow)
	}
	two := big.NewInt(2)
	dividend := new(big.Int).Mul(two, s.cost)
	dividend.Add(dividend, s.quantity)
	divisor := new(big.Int).Mul(two, s.quantity)
	if !dividend.IsInt64() || !divisor.IsInt64() {
		return nil, fmt.Errorf("取得価格の加重平均の途中の値（2S+Q か 2Q）が %w", errCalculationOverflow)
	}

	if s.closeTenths == nil {
		return new(big.Int), nil
	}
	product := new(big.Int).Mul(big.NewInt(*s.closeTenths), s.quantity)
	if !product.IsInt64() {
		return nil, fmt.Errorf("評価額の途中の値（終値×保有数量）が %w", errCalculationOverflow)
	}
	// Valuation と同じく 0.1円単位から円にし、1円未満を切り捨てる（Quo は Go の / と同じく 0 の方向に切り捨てる）。
	return product.Quo(product, big.NewInt(tenthsPerYen)), nil
}
