package handler

import (
	"math"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// 仕様: 売買履歴の要件 docs/requirements/trade-history.md（要件11〜14）と、
// プラン docs/plans/trade-history/plan.md 4.3「一覧の API の形」・4.7（金額は文字列。円の単価は小数1桁・評価額は整数、ドルは小数2桁）。
// 内部の金額は 0.01 単位の整数（円もドルも同じ単位）。API の文字列には、浮動小数点を通さずに、末尾の0も省かずに変換する。

type formatCase struct {
	name      string
	currency  dto.Currency
	hundredth int64
	want      string
	wantErr   bool
}

func runFormatCases(t *testing.T, format func(dto.Currency, int64) (string, error), cases []formatCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := format(tc.currency, tc.hundredth)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("エラーになるはずが、値 %q が返った", got)
				}
				if got != "" {
					t.Errorf("エラーのときは空文字を返すはずが %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("エラーになるはずがない: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatUnitPrice(t *testing.T) {
	runFormatCases(t, formatUnitPrice, []formatCase{
		// 円: 小数第1位まで。末尾の0も省かない（"2500.0"）。
		{name: "円 2500円は2500.0", currency: dto.CurrencyJPY, hundredth: 250000, want: "2500.0"},
		{name: "円 0.1円は0.1", currency: dto.CurrencyJPY, hundredth: 10, want: "0.1"},
		{name: "円 1234.5円は1234.5", currency: dto.CurrencyJPY, hundredth: 123450, want: "1234.5"},
		{name: "円 0円は0.0", currency: dto.CurrencyJPY, hundredth: 0, want: "0.0"},
		// 円の単価が 0.1円（0.01 単位で10の倍数）で割り切れない値は、切り捨てずにエラー。
		{name: "円 0.05円はエラー", currency: dto.CurrencyJPY, hundredth: 5, wantErr: true},
		{name: "円 0.01円はエラー", currency: dto.CurrencyJPY, hundredth: 1, wantErr: true},
		{name: "円 3000.25円はエラー", currency: dto.CurrencyJPY, hundredth: 300025, wantErr: true},
		{name: "円 int64の最大値は10の倍数でないのでエラー", currency: dto.CurrencyJPY, hundredth: math.MaxInt64, wantErr: true},
		// ドル: 小数第2位まで。0.01 単位をそのまま使う。
		{name: "ドル 0.01ドルは0.01", currency: dto.CurrencyUSD, hundredth: 1, want: "0.01"},
		{name: "ドル 0.05ドルは0.05", currency: dto.CurrencyUSD, hundredth: 5, want: "0.05"},
		{name: "ドル 0ドルは0.00", currency: dto.CurrencyUSD, hundredth: 0, want: "0.00"},
		{name: "ドル 150.25ドルは150.25", currency: dto.CurrencyUSD, hundredth: 15025, want: "150.25"},
		{name: "ドル 180ドルは180.00", currency: dto.CurrencyUSD, hundredth: 18000, want: "180.00"},
		{name: "ドル int64の最大値は丸めずに92233720368547758.07", currency: dto.CurrencyUSD, hundredth: math.MaxInt64, want: "92233720368547758.07"},
		// 知らない通貨は、どの値でもエラー。
		{name: "知らない通貨はエラー", currency: dto.Currency("EUR"), hundredth: 10, wantErr: true},
		{name: "通貨が空はエラー", currency: dto.Currency(""), hundredth: 10, wantErr: true},
	})
}

func TestFormatValuation(t *testing.T) {
	runFormatCases(t, formatValuation, []formatCase{
		// 円: 整数。評価額は1円未満を切り捨てた値なので、0.01 単位で100の倍数。
		{name: "円 450000円は450000", currency: dto.CurrencyJPY, hundredth: 45000000, want: "450000"},
		{name: "円 1円は1", currency: dto.CurrencyJPY, hundredth: 100, want: "1"},
		{name: "円 0円は0", currency: dto.CurrencyJPY, hundredth: 0, want: "0"},
		// 100の倍数でない円の評価額は、切り捨てずにエラー。
		{name: "円 0.5円はエラー", currency: dto.CurrencyJPY, hundredth: 50, wantErr: true},
		{name: "円 1.01円はエラー", currency: dto.CurrencyJPY, hundredth: 101, wantErr: true},
		{name: "円 99はエラー", currency: dto.CurrencyJPY, hundredth: 99, wantErr: true},
		// ドル: 小数第2位まで。
		{name: "ドル 0.01ドルは0.01", currency: dto.CurrencyUSD, hundredth: 1, want: "0.01"},
		{name: "ドル 0.05ドルは0.05", currency: dto.CurrencyUSD, hundredth: 5, want: "0.05"},
		{name: "ドル 0ドルは0.00", currency: dto.CurrencyUSD, hundredth: 0, want: "0.00"},
		{name: "ドル 1800ドルは1800.00", currency: dto.CurrencyUSD, hundredth: 180000, want: "1800.00"},
		{name: "ドル int64の最大値は丸めずに92233720368547758.07", currency: dto.CurrencyUSD, hundredth: math.MaxInt64, want: "92233720368547758.07"},
		// 知らない通貨は、どの値でもエラー。
		{name: "知らない通貨はエラー", currency: dto.Currency("EUR"), hundredth: 100, wantErr: true},
		{name: "通貨が空はエラー", currency: dto.Currency(""), hundredth: 100, wantErr: true},
	})
}

// formatDecimal は、整数の10進の文字列の途中に小数点を入れる（浮動小数点を通さない）。
func TestFormatDecimal(t *testing.T) {
	tests := []struct {
		name     string
		units    int64
		decimals int
		want     string
	}{
		{name: "小数1桁 25000は2500.0", units: 25000, decimals: 1, want: "2500.0"},
		{name: "小数1桁 5は0.5", units: 5, decimals: 1, want: "0.5"},
		{name: "小数1桁 0は0.0", units: 0, decimals: 1, want: "0.0"},
		{name: "小数2桁 5は0.05", units: 5, decimals: 2, want: "0.05"},
		{name: "小数2桁 0は0.00", units: 0, decimals: 2, want: "0.00"},
		{name: "小数2桁 100は1.00", units: 100, decimals: 2, want: "1.00"},
		{name: "小数2桁 15025は150.25", units: 15025, decimals: 2, want: "150.25"},
		{name: "小数2桁 int64の最大値は92233720368547758.07", units: math.MaxInt64, decimals: 2, want: "92233720368547758.07"},
		{name: "小数2桁 負の値-5は-0.05", units: -5, decimals: 2, want: "-0.05"},
		{name: "小数2桁 負の値-15025は-150.25", units: -15025, decimals: 2, want: "-150.25"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatDecimal(tc.units, tc.decimals); got != tc.want {
				t.Errorf("formatDecimal(%d, %d) = %q, want %q", tc.units, tc.decimals, got, tc.want)
			}
		})
	}
}
