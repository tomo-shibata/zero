package query_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// 仕様: ドルを円にまとめる合計の要件（docs/requirements/fx-total.md）の基準3・基準6〜基準8・基準10〜基準12（要件2・要件6〜要件8・要件11・要件12）と、
// プラン（docs/plans/fx-total/plan.md）4.3（円にまとめる計算）・4.7（PR① の境界）・5.1（PR① の行）・5.2（calculate_holdings_combined_test.go の行）。
// 確かめる関数は query.CalculateHoldings の3つ目の引数（レート）と、返る dto.HoldingsList の CombinedTotal。
// 金額は 0.01 単位の整数（評価額・含み損益は円なので 100 の倍数）、率は 0.1% 単位。レートも 0.01円単位（154.32円は 15432）。
// 期待値は要件・プランに書かれた値をそのまま使い、テストの中で計算しない。
// 銘柄・売買履歴を作る補助（toyota・apple・msft・softbank・pricedUSDAt・assertTotals など）は calculate_holdings_test.go にある。

// usdJpy は、日付 2026-11-14 のレート（0.01円単位。150.00円は 15000）を返す。
func usdJpy(hundredths int64) *dto.UsdJpyRate {
	return &dto.UsdJpyRate{RateDate: "2026-11-14", RateHundredths: hundredths}
}

// calculateWithRate は、売買履歴 rows とレート rate で CalculateHoldings を呼び、エラーならテストを止める（株式分割はなし）。
func calculateWithRate(t *testing.T, rows []dto.TradeRow, rate *dto.UsdJpyRate) dto.HoldingsList {
	t.Helper()
	list, err := query.CalculateHoldings(rows, nil, rate, nil)
	if err != nil {
		t.Fatalf("CalculateHoldings: got err %v, want nil", err)
	}
	return list
}

// assertCombined は、円にまとめた合計 got が、評価額・注記のフラグ・含み損益・率（want の nil は nil であること）と同じことを確かめる。
func assertCombined(t *testing.T, got *dto.CombinedTotal, wantValuation int64, wantExcludes bool, wantGain, wantRate *int64) {
	t.Helper()
	if got == nil {
		t.Fatalf("CombinedTotal: got nil, want 値あり（評価額 %d）", wantValuation)
	}
	if got.ValuationHundredths != wantValuation {
		t.Errorf("評価額（0.01 単位）: got %d, want %d", got.ValuationHundredths, wantValuation)
	}
	if got.ExcludesUnpriced != wantExcludes {
		t.Errorf("ExcludesUnpriced: got %v, want %v", got.ExcludesUnpriced, wantExcludes)
	}
	if !samePtr(got.UnrealizedGainHundredths, wantGain) {
		t.Errorf("含み損益（0.01 単位）: got %s, want %s", formatOptional(got.UnrealizedGainHundredths), formatOptional(wantGain))
	}
	if !samePtr(got.UnrealizedGainRateTenths, wantRate) {
		t.Errorf("率（0.1%% 単位）: got %s, want %s", formatOptional(got.UnrealizedGainRateTenths), formatOptional(wantRate))
	}
}

// samePtr は、a・b がどちらも nil か、どちらも nil でなく値が同じかを返す。
func samePtr(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// 要件定義の基準1 の売買履歴: 7203「買 2,500円 100株」・AAPL「買 150.25ドル 10株」（どちらも価格あり）。
func basis1Rows() []dto.TradeRow {
	return []dto.TradeRow{
		toyota().buy(1, 100, 250000, "2026-01-10"),
		apple().buy(2, 10, 15025, "2026-01-10"),
	}
}

// 基準3（要件2）: Aの保有が 7203 だけで、レートが1件ある → 円にまとめた合計は nil（切り替えを出さない）。
// あわせて、AAPL を買って全部売った（保有数量0）＋レートありでも nil（保有数量が1以上のドルの銘柄がない）。
func Test基準3_保有数量が1以上のドルの銘柄がなければ円にまとめた合計はnilになる(t *testing.T) {
	t.Parallel()

	t.Run("7203だけ", func(t *testing.T) {
		t.Parallel()
		rows := []dto.TradeRow{toyota().buy(1, 100, 250000, "2026-01-10")}

		list := calculateWithRate(t, rows, usdJpy(15000))

		if list.CombinedTotal != nil {
			t.Errorf("CombinedTotal: got %+v, want nil", *list.CombinedTotal)
		}
	})

	t.Run("AAPLを買って全部売った", func(t *testing.T) {
		t.Parallel()
		rows := []dto.TradeRow{
			apple().buy(1, 10, 15025, "2026-01-10"),
			apple().sell(2, 10, 17000, "2026-02-10"),
		}

		list := calculateWithRate(t, rows, usdJpy(15000))

		if list.CombinedTotal != nil {
			t.Errorf("CombinedTotal: got %+v, want nil", *list.CombinedTotal)
		}
	})

	t.Run("7203を持ちAAPLを買って全部売った", func(t *testing.T) {
		t.Parallel()
		rows := []dto.TradeRow{
			toyota().buy(1, 100, 250000, "2026-01-10"),
			apple().buy(2, 10, 15025, "2026-01-10"),
			apple().sell(3, 10, 17000, "2026-02-10"),
		}

		list := calculateWithRate(t, rows, usdJpy(15000))

		if list.CombinedTotal != nil {
			t.Errorf("CombinedTotal: got %+v, want nil", *list.CombinedTotal)
		}
	})
}

// 基準6（要件6・要件7・要件8）: 7203「買 2,500円 100株」（現在の価格 3,000円）、AAPL「買 150.25ドル 3株」（現在の価格 180.00ドル）、レート 154.32円
// → 評価額 383,332円（540.00 × 154.32 ＝ 83,332.8 を切り捨てて 83,332、＋ 300,000）= 38333200、
// 含み損益 +63,773円（89.25 × 154.32 ＝ 13,773.06 を四捨五入して 13,773、＋ 50,000）= 6377300、
// 率 +20.0%（19.95…%）= 200。ExcludesUnpriced は false。Rate は渡したもの。
func Test基準6_円にまとめた評価額は切り捨て含み損益は四捨五入し率は丸める前の値で計算する(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		toyota().buy(1, 100, 250000, "2026-01-10"),
		apple().buy(2, 3, 15025, "2026-01-10"),
	}
	rate := usdJpy(15432)

	list := calculateWithRate(t, rows, rate)

	assertCombined(t, list.CombinedTotal, 38333200, false, ptr[int64](6377300), ptr[int64](200))
	if list.CombinedTotal != nil && list.CombinedTotal.Rate != *rate {
		t.Errorf("Rate: got %+v, want %+v（渡したレートをそのまま入れる）", list.CombinedTotal.Rate, *rate)
	}
}

// 基準7（要件6）: ドルの銘柄 P・Q（どちらも「買 90.00ドル 1株」、現在の価格 100.01ドル。評価額の合計 200.02ドル）、円の銘柄なし、レート 154.32円
// → 評価額 30,867円（200.02 × 154.32 ＝ 30,867.08… を切り捨て）= 3086700。銘柄ごとに切り捨てる誤りなら 3086600。
// あわせて（プラン 5.1）含み損益 3,089円（20.02 × 154.32 ＝ 3,089.4864 → 3,089円）= 308900、率 11.1%（3,089.4864 ÷ 27,777.6 ＝ 11.12%）= 111。
// 含み損益を銘柄ごとに円にする誤り（質問3）なら 1,545円 × 2 で 309000。
func Test基準7_ドルの評価額と含み損益は合計に1回だけレートを掛ける(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		pricedUSDAt("P", "P社", 10001).buy(1, 1, 9000, "2026-01-10"),
		pricedUSDAt("Q", "Q社", 10001).buy(2, 1, 9000, "2026-01-10"),
	}

	list := calculateWithRate(t, rows, usdJpy(15432))

	assertCombined(t, list.CombinedTotal, 3086700, false, ptr[int64](308900), ptr[int64](111))
}

// 基準8（要件7・要件9）: 7203「買 1,000円 100株」（現在の価格 900円。含み損益 -10,000円・取得額 100,000円）、
// AAPL「買 100.00ドル 1株」（現在の価格 99.99ドル。含み損益 -0.01ドル・取得額 100.00ドル）、レート 154.32円
// → 含み損益 -10,002円（-0.01 × 154.32 ＝ -1.5432 を絶対値で四捨五入して -2、＋ -10,000）= -1000200、
// 率 -8.7%（-10,001.5432 ÷ 115,432 × 100 ＝ -8.66…%）= -87。
// 評価額は 90,000 ＋ 99.99 × 154.32 ＝ 15,430.4568 を切り捨てた 15,430 で 105,430円（10543000。要件6）。
func Test基準8_マイナスの含み損益は絶対値で四捨五入し率は丸める前の値で計算する(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		pricedAt(toyotaCode, toyotaName, 90000).buy(1, 100, 100000, "2026-01-10"),
		pricedUSDAt(appleCode, appleName, 9999).buy(2, 1, 10000, "2026-01-10"),
	}

	list := calculateWithRate(t, rows, usdJpy(15432))

	assertCombined(t, list.CombinedTotal, 10543000, false, ptr[int64](-1000200), ptr[int64](-87))
}

// 基準1 のデータ（円にまとめた値の基準。基準10 の対照）: 7203・AAPL がどちらも価格あり、レート 150.00円
// → 評価額 570,000円（57000000）、含み損益 +94,625円（9462500）、率 +19.9%（199）。ExcludesUnpriced は false。
func Test補助_基準1のデータでは評価額570000円含み損益94625円率19_9パーセントで注記なし(t *testing.T) {
	t.Parallel()

	list := calculateWithRate(t, basis1Rows(), usdJpy(15000))

	assertCombined(t, list.CombinedTotal, 57000000, false, ptr[int64](9462500), ptr[int64](199))
}

// 基準10（要件11）: 基準1 のデータに MSFT「買 400.00ドル 5株」（価格なし）を足す
// → 値は基準1 と同じ（57000000・9462500・199）で、ExcludesUnpriced が true（価格のない銘柄は評価額にも含み損益にも率にも入れない）。
func Test基準10_価格のない銘柄は円にまとめた値に入れず注記のフラグがtrueになる(t *testing.T) {
	t.Parallel()
	rows := append(basis1Rows(), msft().buy(3, 5, 40000, "2026-01-10"))

	list := calculateWithRate(t, rows, usdJpy(15000))

	assertCombined(t, list.CombinedTotal, 57000000, true, ptr[int64](9462500), ptr[int64](199))
}

// 基準11（要件12）: 7203「買 2,500円 100株」（価格あり）、MSFT「買 400.00ドル 5株」（価格なし）、レート 150.00円
// → 評価額 300,000円（30000000）、含み損益 +50,000円（5000000）、率 +20.0%（200）、ExcludesUnpriced が true
// （ドルに価格のある銘柄がなければドルの分を0として、円の値だけで計算する）。
func Test基準11_ドルに価格のある銘柄がなければ円の値だけで計算する(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		toyota().buy(1, 100, 250000, "2026-01-10"),
		msft().buy(2, 5, 40000, "2026-01-10"),
	}

	list := calculateWithRate(t, rows, usdJpy(15000))

	assertCombined(t, list.CombinedTotal, 30000000, true, ptr[int64](5000000), ptr[int64](200))
}

// 基準12（要件12）: 9984「買 4,500円 100株」（価格なし）、MSFT「買 400.00ドル 5株」（価格なし）、レート 150.00円
// → 評価額 0、含み損益と率が nil、ExcludesUnpriced が true。
func Test基準12_円にもドルにも価格のある銘柄がなければ評価額0で含み損益と率はnilになる(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		softbank().buy(1, 100, 450000, "2026-01-10"),
		msft().buy(2, 5, 40000, "2026-01-10"),
	}

	list := calculateWithRate(t, rows, usdJpy(15000))

	assertCombined(t, list.CombinedTotal, 0, true, nil, nil)
}

// ---- 補助のテスト（プラン 5.2。基準にないもの） ----

// 5.2: 7203「買 1,000円 1株」（現在の価格 1,002円、含み損益 +2円）と AAPL「買 100.01ドル 1株」（現在の価格 100.00ドル、-0.01ドル）、レート 150.00円
// → ドルの分 -1.5円が絶対値で四捨五入されて -2円、含み損益 0、率 0（0に向かって切る誤りなら -1円で含み損益 +1円）。
// 評価額は 1,002 ＋ 100.00 × 150 ＝ 15,000 で 16,002円（1600200）。
func Test補助_ドルの含み損益がちょうど半分のとき絶対値で四捨五入する(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		pricedAt(toyotaCode, toyotaName, 100200).buy(1, 1, 100000, "2026-01-10"),
		pricedUSDAt(appleCode, appleName, 10000).buy(2, 1, 10001, "2026-01-10"),
	}

	list := calculateWithRate(t, rows, usdJpy(15000))

	assertCombined(t, list.CombinedTotal, 1600200, false, ptr[int64](0), ptr[int64](0))
}

// 5.2: 7203「買 1,000円 1株」（現在の価格 1,000円）と AAPL「買 1.00ドル 1株」（現在の価格 1.01ドル）、レート 154.32円
// → 含み損益 2円（0.01 × 154.32 ＝ 1.5432 → 2円。切り捨てる誤りなら 100）= 200、率 0.1%（1.5432 ÷ 1,154.32 ＝ 0.134%）= 1
// （丸めた含み損益から率を出す誤りなら率 2）。評価額は 1,000 ＋ 1.01 × 154.32 ＝ 155.8632 を切り捨てた 155 で 1,155円（115500）。
func Test補助_ドルの含み損益は四捨五入し率は丸める前の含み損益から出す(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		pricedAt(toyotaCode, toyotaName, 100000).buy(1, 1, 100000, "2026-01-10"),
		pricedUSDAt(appleCode, appleName, 101).buy(2, 1, 100, "2026-01-10"),
	}

	list := calculateWithRate(t, rows, usdJpy(15432))

	assertCombined(t, list.CombinedTotal, 115500, false, ptr[int64](200), ptr[int64](1))
}

// 5.2: 7203「買 100.4円 1株」（現在の価格 100.0円。丸める前の含み損益 -0.4円、表示は0円）と AAPL「買 1.00ドル 1株」（現在の価格 1.00ドル）、レート 154.32円
// → 含み損益 0、率 0（カード表示の要件10: 表示する含み損益が0なら率も0。0にしない誤りなら率 -2。-0.4 ÷ 254.72 ＝ -0.157%）。
// 評価額は 100 ＋ 154.32 を切り捨てた 154 で 254円（25400）。
func Test補助_表示する含み損益が0のとき率も0にする(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		pricedAt(toyotaCode, toyotaName, 10000).buy(1, 1, 10040, "2026-01-10"),
		pricedUSDAt(appleCode, appleName, 100).buy(2, 1, 100, "2026-01-10"),
	}

	list := calculateWithRate(t, rows, usdJpy(15432))

	assertCombined(t, list.CombinedTotal, 25400, false, ptr[int64](0), ptr[int64](0))
}

// 5.2（4.3 の率の行。丸めた率の符号が含み損益と逆なら0）: 円の銘柄5つ（J1〜J5。それぞれ「買 1.4円 1株」・現在の価格 1.0円。丸める前の含み損益 -0.4円、表示は0円）と
// AAPL「買 1.00ドル 1株」（現在の価格 1.01ドル。含み損益 +0.01ドル）、レート 154.32円
// → ドルの分 0.01 × 154.32 ＝ 1.5432円が四捨五入されて 2円で、表示する含み損益は 0 ＋ 2 ＝ +2円（200）。
// 丸める前の和は -0.4 × 5 ＋ 1.5432 ＝ -0.4568円、取得額の和は 1.4 × 5 ＋ 154.32 ＝ 161.32円なので、丸めた率は -0.2832…% → -0.3%（-3）。
// 表示する含み損益はプラスなので、符号が逆の率は0にする。決まりがない誤りなら率は -3（含み損益 +2円なのに率 -0.3%）。
// 評価額は 1 × 5 ＋ 1.01 × 154.32 ＝ 155.8632 を切り捨てた 155 で 160円（16000）。
func Test補助_丸めた率の符号が表示する含み損益と逆なら率は0にする_決まりがなければ率は負の0_3パーセント(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		pricedAt("J1", "J1社", 100).buy(1, 1, 140, "2026-01-10"),
		pricedAt("J2", "J2社", 100).buy(2, 1, 140, "2026-01-10"),
		pricedAt("J3", "J3社", 100).buy(3, 1, 140, "2026-01-10"),
		pricedAt("J4", "J4社", 100).buy(4, 1, 140, "2026-01-10"),
		pricedAt("J5", "J5社", 100).buy(5, 1, 140, "2026-01-10"),
		pricedUSDAt(appleCode, appleName, 101).buy(6, 1, 100, "2026-01-10"),
	}

	list := calculateWithRate(t, rows, usdJpy(15432))

	assertCombined(t, list.CombinedTotal, 16000, false, ptr[int64](200), ptr[int64](0))
}

// 5.2（要件6 の円の分が0）: ドルの銘柄だけ（AAPL「買 150.25ドル 10株」）＋レート 150.00円
// → 評価額 270,000円（27000000）、含み損益 44,625円（4462500）。率は 297.50 ÷ 1,502.50 ＝ 19.8%（198）。
func Test補助_ドルの銘柄だけでも円にまとめた値を計算できる(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{apple().buy(1, 10, 15025, "2026-01-10")}

	list := calculateWithRate(t, rows, usdJpy(15000))

	assertCombined(t, list.CombinedTotal, 27000000, false, ptr[int64](4462500), ptr[int64](198))
}

// 5.2（要件12 の後半。基準11 と逆の向き）: 円は価格なし（9984）、ドルは価格あり（AAPL「買 150.25ドル 10株」）、レート 150.00円
// → 含み損益はドルの分だけ（4462500）、ExcludesUnpriced が true。評価額は 27000000。率は円の取得額を入れず 198。
func Test補助_円に価格のある銘柄がなければドルの値だけで計算する(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		softbank().buy(1, 100, 450000, "2026-01-10"),
		apple().buy(2, 10, 15025, "2026-01-10"),
	}

	list := calculateWithRate(t, rows, usdJpy(15000))

	assertCombined(t, list.CombinedTotal, 27000000, true, ptr[int64](4462500), ptr[int64](198))
}

// 5.2（4.3 の桁あふれ）: ドルの評価額 × レートが int64 に収まらないとき ErrCalculationOverflow。
// AAPL 1株（取得価格・現在の価格とも 9×10^16（0.01 単位）。ドルの通貨ごとの値は収まる）、レート 150.00円で、ドルの評価額 × 150 が int64 を超える。
// 対照: 同じ売買履歴でレート 1.00円（100）なら収まり、エラーにならない。
func Test補助_ドルの評価額にレートを掛けた値がint64に収まらなければErrCalculationOverflowになる(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		pricedUSDAt(appleCode, appleName, 90000000000000000).buy(1, 1, 90000000000000000, "2026-01-10"),
	}

	t.Run("レート150円はあふれる", func(t *testing.T) {
		t.Parallel()

		_, err := query.CalculateHoldings(rows, nil, usdJpy(15000), nil)

		if !errors.Is(err, query.ErrCalculationOverflow) {
			t.Errorf("CalculateHoldings: got err %v, want ErrCalculationOverflow", err)
		}
	})

	t.Run("対照_レート1円なら収まる", func(t *testing.T) {
		t.Parallel()

		list, err := query.CalculateHoldings(rows, nil, usdJpy(100), nil)

		if err != nil {
			t.Fatalf("CalculateHoldings: got err %v, want nil", err)
		}
		if list.CombinedTotal == nil {
			t.Fatalf("CombinedTotal: got nil, want 値あり")
		}
	})
}

// 5.2（4.3 の桁あふれ。含み損益の道）: ドルの含み損益に R を掛けた値が int64 に収まらないとき ErrCalculationOverflow。
// AAPL 1株（取得価格 9×10^16（0.01 単位）、現在の価格 0.01ドル）、レート 150.00円で、評価額（1 × 150 ÷ 100 → 0円）は収まるが、
// 含み損益 -(9×10^16 - 1) × 150 ÷ 100 × 100（0.01円単位にして1円単位に丸めた値）が約 -1.35×10^19 となり int64（約 9.22×10^18）を超える。
// ドルの通貨ごとの値（含み損益 -(9×10^16 - 1)・取得額 9×10^16）は収まる。
// 対照: 同じ売買履歴でレート 1.00円（100）なら含み損益は約 -9×10^16（0.01円単位）で収まり、エラーにならない。
func Test補助_ドルの含み損益にレートを掛けた値がint64に収まらなければErrCalculationOverflowになる(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		pricedUSDAt(appleCode, appleName, 1).buy(1, 1, 90000000000000000, "2026-01-10"),
	}

	t.Run("レート150円はあふれる", func(t *testing.T) {
		t.Parallel()

		_, err := query.CalculateHoldings(rows, nil, usdJpy(15000), nil)

		if !errors.Is(err, query.ErrCalculationOverflow) {
			t.Errorf("CalculateHoldings: got err %v, want ErrCalculationOverflow", err)
		}
	})

	t.Run("対照_レート1円なら収まる", func(t *testing.T) {
		t.Parallel()

		list, err := query.CalculateHoldings(rows, nil, usdJpy(100), nil)

		if err != nil {
			t.Fatalf("CalculateHoldings: got err %v, want nil", err)
		}
		if list.CombinedTotal == nil {
			t.Fatalf("CombinedTotal: got nil, want 値あり")
		}
	})
}

// 5.2: rate が nil で CombinedTotal が nil、Holdings・Totals は今と同じ（基準1 のデータ。円 +50,000円・+20.0%、ドル +297.50ドル・+19.8%）。
// レートを渡しても Holdings・Totals は同じ（円にまとめる計算は通貨ごとの値を変えない）。
func Test補助_レートがnilなら円にまとめた合計はnilで通貨ごとの値は変わらない(t *testing.T) {
	t.Parallel()
	wantTotals := []dto.CurrencyTotal{
		jpyTotal(30000000, false, ptr[int64](5000000), ptr[int64](200)),
		usdTotal(180000, false, ptr[int64](29750), ptr[int64](198)),
	}

	t.Run("レートなし", func(t *testing.T) {
		t.Parallel()

		list := calculateWithRate(t, basis1Rows(), nil)

		if list.CombinedTotal != nil {
			t.Errorf("CombinedTotal: got %+v, want nil", *list.CombinedTotal)
		}
		if got, want := codes(list), []string{toyotaCode, appleCode}; !slices.Equal(got, want) {
			t.Errorf("銘柄コード: got %v, want %v", got, want)
		}
		assertGain(t, list.Holdings[0], 5000000, 200)
		assertGain(t, list.Holdings[1], 29750, 198)
		assertTotals(t, list, wantTotals)
	})

	t.Run("レートあり", func(t *testing.T) {
		t.Parallel()

		list := calculateWithRate(t, basis1Rows(), usdJpy(15000))

		assertGain(t, list.Holdings[0], 5000000, 200)
		assertGain(t, list.Holdings[1], 29750, 198)
		assertTotals(t, list, wantTotals)
	})
}
