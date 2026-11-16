package model

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
