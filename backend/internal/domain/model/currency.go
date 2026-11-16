package model

import (
	"fmt"
	"math/big"
)

// Currency は、銘柄の通貨（要件1）。円（JPY）とドル（USD）だけを扱う。
// 値は、DB の stocks.currency の CHECK（'JPY'・'USD'。00006）と同じ文字列。
type Currency string

const (
	CurrencyJPY Currency = "JPY" // 円
	CurrencyUSD Currency = "USD" // ドル
)

// yenPriceUnitHundredths は、円の価格の最小の単位（0.1円）が 0.01 単位でいくつか。
const yenPriceUnitHundredths = 10

// AcceptsPrice は、0.01 単位の価格 hundredths が、この通貨の精度に合うかを返す（要件3・要件5）。
// 円は小数第1位（0.1円）まで、つまり 0.01 単位で10の倍数のときだけ真。ドルは小数第2位（0.01）までなので、すべて真。
// 円の精度は DB の CHECK では確かめられない（銘柄の通貨が別のテーブルにあるため。プラン 6章の判断4）ので、ここで確かめる。
// 知らない通貨は、どの価格も受け付けない。
func (c Currency) AcceptsPrice(hundredths int64) bool {
	switch c {
	case CurrencyJPY:
		return hundredths%yenPriceUnitHundredths == 0
	case CurrencyUSD:
		return true
	default:
		return false
	}
}

// PriceHundredths は、価格 price（1株あたり、この通貨の金額）を 0.01 単位の整数にする（要件3・要件24）。
// この通貨の精度（円は小数第1位、ドルは小数第2位）に合わなければ ErrPriceTooFine を返す。
// 大きさ（0より大きく1億未満）は確かめない（CheckPrice が先に確かめる）が、0.01 単位の int64 に収まらない値は ErrPriceTooLarge を返す。
// 知らない通貨はエラーにする。
func (c Currency) PriceHundredths(price *big.Rat) (int64, error) {
	if c != CurrencyJPY && c != CurrencyUSD {
		return 0, fmt.Errorf("知らない通貨 %q です", string(c))
	}
	scaled := new(big.Rat).Mul(price, new(big.Rat).SetInt64(hundredthsPerUnit))
	if !scaled.IsInt() {
		return 0, ErrPriceTooFine
	}
	if !scaled.Num().IsInt64() {
		return 0, ErrPriceTooLarge
	}
	hundredths := scaled.Num().Int64()
	if !c.AcceptsPrice(hundredths) {
		return 0, ErrPriceTooFine
	}
	return hundredths, nil
}
