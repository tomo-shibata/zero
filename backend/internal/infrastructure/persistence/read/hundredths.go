package read

import (
	"errors"
	"math/big"

	"github.com/jackc/pgx/v5/pgtype"
)

var (
	errNullPrice          = errors.New("単価が NULL です")
	errNonFinitePrice     = errors.New("単価が有限の数ではありません（NaN か無限大）")
	errFinerThanHundredth = errors.New("単価に 0.01 より細かい桁があります")
	errPriceOutOfRange    = errors.New("単価が int64 の範囲を超えます")
)

// maxShiftDigits は、計算する前に断る 10 の乗数の上限。巨大な Exp で大きな数を作らないため、これより大きい shift は計算せずに断る。
// これ以下でも int64 に収まらなければ（0 でない値に 10^19 以上をかけると必ず収まらない）、後の IsInt64 で断る。
const maxShiftDigits = 20

// hundredthsFromNumeric は、PostgreSQL の NUMERIC の単価（円またはドル）を 0.01 単位の整数にする。
//
// 単価の列は桁数を指定しない NUMERIC なので、書いた桁（scale）のまま保存される（2000 と 2001.0 で scale が違う。プラン 4.2）。
// pgx はそれを「整数 Int × 10^Exp」で返し、同じ値でも Int と Exp の組は1通りに決まらないので、
// どの組でも 0.01 単位の値（Int × 10^(Exp+2)）を正しく求める。
// 0.01 で割り切れない値や int64 に収まらない値は、丸めたり切ったりせずにエラーにする（金額を黙って変えないため）。
// DB の CHECK 制約があるので、正しく保存された値ではエラーにならない。
// 円の単価が 0.1円で割り切れるか（10の倍数か）は、通貨を見ないと決まらないので、ここでは見ない（API の応答を作るときに確かめる）。
func hundredthsFromNumeric(n pgtype.Numeric) (int64, error) {
	if !n.Valid {
		return 0, errNullPrice
	}
	if n.NaN || n.InfinityModifier != pgtype.Finite || n.Int == nil {
		return 0, errNonFinitePrice
	}

	// 0 は Exp がどんな値でも 0。極端な Exp で大きな数を作らないよう、先に返す。
	if n.Int.Sign() == 0 {
		return 0, nil
	}

	hundredths := new(big.Int).Set(n.Int)
	if shift := int64(n.Exp) + 2; shift >= 0 {
		// 0 でない値に 10^20 以上をかけると、int64 に収まらない。巨大な桁の Exp で、大きな数を作らないために先に断る。
		if shift > maxShiftDigits {
			return 0, errPriceOutOfRange
		}
		hundredths.Mul(hundredths, pow10(shift))
	} else {
		// 割る桁数が |Int| の10進の桁数より大きいと、割り切れない（0 でない値は 10^桁数 より小さい）。
		// 巨大な桁の Exp で、大きな数を作らないために、pow10 を作る前に断る。
		if -shift > int64(len(new(big.Int).Abs(hundredths).String())) {
			return 0, errFinerThanHundredth
		}
		var rem big.Int
		hundredths.QuoRem(hundredths, pow10(-shift), &rem)
		if rem.Sign() != 0 {
			return 0, errFinerThanHundredth
		}
	}
	if !hundredths.IsInt64() {
		return 0, errPriceOutOfRange
	}
	return hundredths.Int64(), nil
}

// pow10 は、10 の n 乗（n >= 0）を返す。
func pow10(n int64) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(n), nil)
}
