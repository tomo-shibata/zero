package read

import (
	"errors"
	"math/big"

	"github.com/jackc/pgx/v5/pgtype"
)

var (
	errNullPrice       = errors.New("単価が NULL です")
	errNonFinitePrice  = errors.New("単価が有限の数ではありません（NaN か無限大）")
	errFinerThanTenths = errors.New("単価に 0.1円より細かい桁があります")
	errPriceOutOfRange = errors.New("単価が int64 の範囲を超えます")
)

// tenthsFromNumeric は、PostgreSQL の NUMERIC の単価（円）を 0.1円単位の整数にする。
//
// 単価の列は桁数を指定しない NUMERIC なので、書いた桁（scale）のまま保存される（2000 と 2001.0 で scale が違う。プラン 4.2）。
// pgx はそれを「整数 Int × 10^Exp」で返し、同じ値でも Int と Exp の組は1通りに決まらないので、
// どの組でも 0.1円単位の値（Int × 10^(Exp+1)）を正しく求める。
// 0.1円で割り切れない値や int64 に収まらない値は、丸めたり切ったりせずにエラーにする（金額を黙って変えないため）。
// DB の CHECK 制約があるので、正しく保存された値ではエラーにならない。
func tenthsFromNumeric(n pgtype.Numeric) (int64, error) {
	if !n.Valid {
		return 0, errNullPrice
	}
	if n.NaN || n.InfinityModifier != pgtype.Finite || n.Int == nil {
		return 0, errNonFinitePrice
	}

	tenths := new(big.Int).Set(n.Int)
	if shift := int64(n.Exp) + 1; shift >= 0 {
		tenths.Mul(tenths, pow10(shift))
	} else {
		var rem big.Int
		tenths.QuoRem(tenths, pow10(-shift), &rem)
		if rem.Sign() != 0 {
			return 0, errFinerThanTenths
		}
	}
	if !tenths.IsInt64() {
		return 0, errPriceOutOfRange
	}
	return tenths.Int64(), nil
}

// pow10 は、10 の n 乗（n >= 0）を返す。
func pow10(n int64) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(n), nil)
}
