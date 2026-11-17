package handler

import (
	"encoding/json"
	"math"
	"reflect"
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

// 仕様: カード表示の要件 docs/requirements/holdings-cards.md の基準13（要件10）と、
// プラン docs/plans/holdings-cards/plan.md 4.2「API の形」・4.6（PR① の境界）・5.1（基準13）・5.3（PR① の handler の補助のテスト）。

// 基準13（要件10）の handler の部分・補助: 0.01 単位の含み損益を API の文字列にする。
// 円は符号付きの整数（100の倍数でなければエラー）、ドルは符号付きの小数2桁（整数部が0の負の値は "-0.01"）。プラスに "+" は付けない。
func TestFormatUnrealizedGain(t *testing.T) {
	runFormatCases(t, formatUnrealizedGain, []formatCase{
		{name: "円 -501円は-501", currency: dto.CurrencyJPY, hundredth: -50100, want: "-501"},
		{name: "円 50000円は50000", currency: dto.CurrencyJPY, hundredth: 5000000, want: "50000"},
		{name: "円 0円は0", currency: dto.CurrencyJPY, hundredth: 0, want: "0"},
		{name: "円 1円は1（プラスに+を付けない）", currency: dto.CurrencyJPY, hundredth: 100, want: "1"},
		{name: "円 -1円は-1", currency: dto.CurrencyJPY, hundredth: -100, want: "-1"},
		{name: "円 1234567円は1234567", currency: dto.CurrencyJPY, hundredth: 123456700, want: "1234567"},
		{name: "ドル -0.01ドルは-0.01（整数部が0の負の値）", currency: dto.CurrencyUSD, hundredth: -1, want: "-0.01"},
		{name: "ドル -159.99ドルは-159.99", currency: dto.CurrencyUSD, hundredth: -15999, want: "-159.99"},
		{name: "ドル 0ドルは0.00", currency: dto.CurrencyUSD, hundredth: 0, want: "0.00"},
		{name: "ドル 297.50ドルは297.50", currency: dto.CurrencyUSD, hundredth: 29750, want: "297.50"},
		{name: "ドル 0.01ドルは0.01", currency: dto.CurrencyUSD, hundredth: 1, want: "0.01"},
		// 円で100の倍数でない値は、切り捨てずにエラー。
		{name: "円 0.5円（50）はエラー", currency: dto.CurrencyJPY, hundredth: 50, wantErr: true},
		{name: "円 -0.5円（-50）はエラー", currency: dto.CurrencyJPY, hundredth: -50, wantErr: true},
		{name: "円 1.01円（101）はエラー", currency: dto.CurrencyJPY, hundredth: 101, wantErr: true},
		// 知らない通貨は、どの値でもエラー。
		{name: "知らない通貨はエラー", currency: dto.Currency("EUR"), hundredth: 100, wantErr: true},
		{name: "通貨が空はエラー", currency: dto.Currency(""), hundredth: 100, wantErr: true},
	})
}

// 基準13（要件10）の handler の部分・補助: 0.1% 単位の率を、符号付きの小数1桁の文字列にする（整数部が0の負の値は "-0.5"）。
func TestFormatGainRate(t *testing.T) {
	tests := []struct {
		name   string
		tenths int64
		want   string
	}{
		{name: "-5は-0.5", tenths: -5, want: "-0.5"},
		{name: "-125は-12.5", tenths: -125, want: "-12.5"},
		{name: "0は0.0", tenths: 0, want: "0.0"},
		{name: "749は74.9", tenths: 749, want: "74.9"},
		{name: "200は20.0", tenths: 200, want: "20.0"},
		{name: "1は0.1", tenths: 1, want: "0.1"},
		{name: "-1は-0.1", tenths: -1, want: "-0.1"},
		{name: "1235は123.5", tenths: 1235, want: "123.5"},
		{name: "-71は-7.1", tenths: -71, want: "-7.1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatGainRate(tc.tenths); got != tc.want {
				t.Errorf("formatGainRate(%d) = %q, want %q", tc.tenths, got, tc.want)
			}
		})
	}
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

// ---- ドルを円にまとめる合計（docs/plans/fx-total/plan.md 4.4・4.7・5.1 の基準6・基準9。PR①） ----

// marshalCombinedTotal は、list を newHoldingsResponse に通して json.Marshal し、本文の combinedTotal を取り出す。
// キーがなければテストを止める（キーは常にあり、値は null かオブジェクト。4.7）。
func marshalCombinedTotal(t *testing.T, list dto.HoldingsList) any {
	t.Helper()
	body, err := newHoldingsResponse(list)
	if err != nil {
		t.Fatalf("newHoldingsResponse: got err %v, want nil", err)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("本文を JSON として読めない: %v\n本文: %s", err, raw)
	}
	got, ok := decoded["combinedTotal"]
	if !ok {
		t.Fatalf("本文に combinedTotal のキーがない（キーは常にある）\n本文: %s", raw)
	}
	return got
}

// 基準6（API の形）: 評価額 38333200・含み損益 6377300・率 200・ExcludesUnpriced false・レート 15432（2026-11-14）の CombinedTotal
// → combinedTotal が "valuation":"383332"・"unrealizedGain":"63773"・"unrealizedGainRate":"20.0"・"usdJpyRate":"154.32"・
// "excludesUnpriced":false・"usdJpyRateDate":"2026-11-14"。
func Test基準6_円にまとめた合計は円の整数と率の文字列とレートの文字列にして返す(t *testing.T) {
	gain, rate := int64(6377300), int64(200)
	list := dto.HoldingsList{
		Holdings: []dto.Holding{},
		Totals:   []dto.CurrencyTotal{},
		CombinedTotal: &dto.CombinedTotal{
			ValuationHundredths:      38333200,
			ExcludesUnpriced:         false,
			UnrealizedGainHundredths: &gain,
			UnrealizedGainRateTenths: &rate,
			Rate:                     dto.UsdJpyRate{RateDate: "2026-11-14", RateHundredths: 15432},
		},
	}

	got := marshalCombinedTotal(t, list)

	want := map[string]any{
		"valuation":          "383332",
		"excludesUnpriced":   false,
		"unrealizedGain":     "63773",
		"unrealizedGainRate": "20.0",
		"usdJpyRate":         "154.32",
		"usdJpyRateDate":     "2026-11-14",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("combinedTotal:\n got %v\nwant %v", got, want)
	}
}

// 基準9（API の形）: レートの組 (a) 15000・2026-11-14 (b) 100050・2026-01-05 → "usdJpyRate" が (a) "150.00" (b) "1000.50"、
// "usdJpyRateDate" は日付のまま（3桁区切りは画面が付ける）。
func Test基準9_レートは小数2桁の文字列にして返す(t *testing.T) {
	cases := []struct {
		name         string
		rate         dto.UsdJpyRate
		wantRate     string
		wantRateDate string
	}{
		{"a_150円", dto.UsdJpyRate{RateDate: "2026-11-14", RateHundredths: 15000}, "150.00", "2026-11-14"},
		{"b_1000.5円", dto.UsdJpyRate{RateDate: "2026-01-05", RateHundredths: 100050}, "1000.50", "2026-01-05"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gain, rate := int64(0), int64(0)
			list := dto.HoldingsList{
				Holdings: []dto.Holding{},
				Totals:   []dto.CurrencyTotal{},
				CombinedTotal: &dto.CombinedTotal{
					UnrealizedGainHundredths: &gain,
					UnrealizedGainRateTenths: &rate,
					Rate:                     tc.rate,
				},
			}

			got, ok := marshalCombinedTotal(t, list).(map[string]any)

			if !ok {
				t.Fatalf("combinedTotal がオブジェクトでない")
			}
			if got["usdJpyRate"] != tc.wantRate {
				t.Errorf("usdJpyRate: got %v, want %q", got["usdJpyRate"], tc.wantRate)
			}
			if got["usdJpyRateDate"] != tc.wantRateDate {
				t.Errorf("usdJpyRateDate: got %v, want %q", got["usdJpyRateDate"], tc.wantRateDate)
			}
		})
	}
}

// 基準12（API の形。プラン 4.4「含み損益と率は、両方とも null か両方とも文字列」）: 評価額 0・含み損益と率が nil・ExcludesUnpriced true
// → "valuation":"0"・"unrealizedGain":null・"unrealizedGainRate":null・"excludesUnpriced":true。
func Test補助_円にまとめた含み損益と率がnilならどちらもnullで返す(t *testing.T) {
	list := dto.HoldingsList{
		Holdings: []dto.Holding{},
		Totals:   []dto.CurrencyTotal{},
		CombinedTotal: &dto.CombinedTotal{
			ValuationHundredths: 0,
			ExcludesUnpriced:    true,
			Rate:                dto.UsdJpyRate{RateDate: "2026-11-14", RateHundredths: 15000},
		},
	}

	got := marshalCombinedTotal(t, list)

	want := map[string]any{
		"valuation":          "0",
		"excludesUnpriced":   true,
		"unrealizedGain":     nil,
		"unrealizedGainRate": nil,
		"usdJpyRate":         "150.00",
		"usdJpyRateDate":     "2026-11-14",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("combinedTotal:\n got %v\nwant %v", got, want)
	}
}

// 要件2（API の形。プラン 4.7「キーは常にあり、値は null かオブジェクト」）: CombinedTotal が nil → 本文に "combinedTotal":null。
func Test補助_円にまとめた合計がnilなら本文のcombinedTotalはnullになる(t *testing.T) {
	list := dto.HoldingsList{Holdings: []dto.Holding{}, Totals: []dto.CurrencyTotal{}, CombinedTotal: nil}

	got := marshalCombinedTotal(t, list)

	if got != nil {
		t.Errorf("combinedTotal: got %v, want null", got)
	}
}
