package model_test

import (
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// 仕様: 要件（docs/requirements/trade-history.md）の要件1（通貨は円 JPY かドル USD）・要件3・要件5（価格の精度は円が小数第1位まで、
// ドルが小数第2位まで）と、プラン（docs/plans/trade-history/plan.md）4.6（PR②）の
// 「model.Currency・CurrencyJPY・CurrencyUSD・AcceptsPrice(hundredths)」、5.3（PR②）の「Currency.AcceptsPrice（単体）」
// （円 250000 は真・250010 は真・250001 は偽、ドルはすべて真）。
// 価格は 0.01 単位の整数（Hundredths）で渡す（250000 は 2,500円または 2,500.00ドル）。

// 通貨の値は、DB の stocks.currency の CHECK（'JPY'・'USD'）と同じ文字列（4.2）。
func Test補助_通貨の値はJPYとUSDの文字列である(t *testing.T) {
	t.Parallel()
	if got := string(model.CurrencyJPY); got != "JPY" {
		t.Errorf("CurrencyJPY: got %q, want %q", got, "JPY")
	}
	if got := string(model.CurrencyUSD); got != "USD" {
		t.Errorf("CurrencyUSD: got %q, want %q", got, "USD")
	}
}

// 5.3: AcceptsPrice は、0.01 単位の価格が通貨の精度に合うかを返す。
// 円は 0.1円で割り切れる（0.01 単位で10の倍数）ときだけ真。ドルはすべて真。
func Test補助_AcceptsPriceは円なら10の倍数だけを受け付けドルならすべて受け付ける(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		currency   model.Currency
		hundredths int64
		want       bool
	}{
		{"円_250000は2500円で真", model.CurrencyJPY, 250000, true},
		{"円_250010は2500.1円で真", model.CurrencyJPY, 250010, true},
		{"円_250001は2500.01円で偽", model.CurrencyJPY, 250001, false},
		{"円_250005は2500.05円で偽", model.CurrencyJPY, 250005, false},
		{"円_10は0.1円で真", model.CurrencyJPY, 10, true},
		{"円_1は0.01円で偽", model.CurrencyJPY, 1, false},
		{"ドル_250000は2500.00ドルで真", model.CurrencyUSD, 250000, true},
		{"ドル_250010は2500.10ドルで真", model.CurrencyUSD, 250010, true},
		{"ドル_250001は2500.01ドルで真", model.CurrencyUSD, 250001, true},
		{"ドル_15025は150.25ドルで真", model.CurrencyUSD, 15025, true},
		{"ドル_1は0.01ドルで真", model.CurrencyUSD, 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tc.currency.AcceptsPrice(tc.hundredths)

			if got != tc.want {
				t.Errorf("%s.AcceptsPrice(%d): got %v, want %v", tc.currency, tc.hundredths, got, tc.want)
			}
		})
	}
}
