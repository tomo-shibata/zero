package query_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// 仕様: おまかせ運用の要件（docs/requirements/managed-account.md）の基準3・基準7・基準9〜基準13（要件4・要件9・要件12・要件15〜要件19）と、
// プラン（docs/plans/managed-account/plan.md）4.3（口座のカードの値と円の合計）・4.8 の PR②の境界・5.1 の PR② の行・5.2 の PR② の「計算」。
// 確かめる関数は query.CalculateHoldings の4つ目の引数（口座ごとの最新の記録）と、返る dto.HoldingsList の ManagedAccounts・Totals・CombinedTotal。
// 金額は 0.01 単位の整数（円は100の倍数。4,000,000円は 400000000）、率は 0.1% 単位。
// 期待値は要件・プランに書かれた値をそのまま使い、テストの中で計算しない（計算し直した値は node で確かめて書いた）。
// 銘柄・売買履歴を作る補助（toyota・sony・apple・msft・softbank・jpyTotal・usdTotal・assertTotals・assertCombined など）は
// calculate_holdings_test.go と calculate_holdings_combined_test.go にある。

// managedRow は、口座 name の最新の記録（日付 2026-11-16。評価額 valuationYen円・損益 gainYen円）の行を返す。
func managedRow(name string, valuationYen, gainYen int64) dto.ManagedAccountRow {
	return dto.ManagedAccountRow{
		Name:                name,
		ValuationDate:       "2026-11-16",
		ValuationHundredths: valuationYen * 100,
		GainHundredths:      gainYen * 100,
	}
}

// calculateWithAccounts は、売買履歴 rows・レート rate・口座 accounts で CalculateHoldings を呼び、エラーならテストを止める（株式分割はなし）。
func calculateWithAccounts(t *testing.T, rows []dto.TradeRow, rate *dto.UsdJpyRate, accounts []dto.ManagedAccountRow) dto.HoldingsList {
	t.Helper()
	list, err := query.CalculateHoldings(rows, nil, rate, accounts)
	if err != nil {
		t.Fatalf("CalculateHoldings: got err %v, want nil", err)
	}
	return list
}

// accountNames は、一覧の口座の名前を並び順のまま返す。
func accountNames(list dto.HoldingsList) []string {
	got := make([]string, 0, len(list.ManagedAccounts))
	for _, a := range list.ManagedAccounts {
		got = append(got, a.Name)
	}
	return got
}

// basis9Rows は、基準9 の売買履歴: 7203「買 2,500円 100株」・6758「買 4,000円 10株」・AAPL「買 150.25ドル 10株」（どれも価格あり）。
func basis9Rows() []dto.TradeRow {
	return []dto.TradeRow{
		toyota().buy(1, 100, 250000, "2026-01-10"),
		sony().buy(2, 10, 400000, "2026-01-10"),
		apple().buy(3, 10, 15025, "2026-01-10"),
	}
}

// 基準3（要件9）: 口座が ロボアド B・WealthNavi・Alpha（この順に渡す）→ ManagedAccounts の名前が Alpha・WealthNavi・ロボアド B の順。
func Test基準3_口座は名前の昇順に並ぶ(t *testing.T) {
	t.Parallel()
	accounts := []dto.ManagedAccountRow{
		managedRow("ロボアド B", 500000, 0),
		managedRow("WealthNavi", 4000000, 1000000),
		managedRow("Alpha", 300000, 30000),
	}

	list := calculateWithAccounts(t, nil, nil, accounts)

	if got, want := accountNames(list), []string{"Alpha", "WealthNavi", "ロボアド B"}; !slices.Equal(got, want) {
		t.Errorf("口座の名前の並び: got %v, want %v", got, want)
	}
}

// 補助（要件9・プラン 5.2）: 名前の並びは文字コード（バイト）の昇順。Zeta・alpha・Alpha の順に渡すと Alpha・Zeta・alpha。
// 受け取った順を逆にするだけなら Alpha・alpha・Zeta、大文字と小文字を区別しない並べ方なら alpha・Alpha・Zeta、降順なら alpha・Zeta・Alpha になる。
func Test補助_口座の並びは大文字と小文字を区別した文字コードの昇順(t *testing.T) {
	t.Parallel()
	accounts := []dto.ManagedAccountRow{
		managedRow("Zeta", 100000, 0),
		managedRow("alpha", 100000, 0),
		managedRow("Alpha", 100000, 0),
	}

	list := calculateWithAccounts(t, nil, nil, accounts)

	if got, want := accountNames(list), []string{"Alpha", "Zeta", "alpha"}; !slices.Equal(got, want) {
		t.Errorf("口座の名前の並び: got %v, want %v", got, want)
	}
}

// 基準7（要件8・要件12・要件13）: 最新の記録の評価額と損益の組 (a)〜(e) を、それぞれ1つの口座で渡す
// → 元本（評価額 − 損益）・含み損益（損益そのまま）・率（0.1% 単位。小数第2位を絶対値で四捨五入）が
// (a) 300000000・-30000000・-100（-10.0%） (b) 300000000・0・0 (c) 100000000・23456700・235（23.4567%）
// (d) 100000000・-12345700・-123（-12.3457%） (e) 99900・100・1（0.1001…%）。名前・日付・評価額は渡したまま。
func Test基準7_口座のカードの元本と含み損益と率(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                    string
		valuationYen, gainYen   int64
		wantValuation           int64
		wantPrincipal, wantGain int64
		wantRateTenths          int64
	}{
		{name: "a_2700000円と-300000円", valuationYen: 2700000, gainYen: -300000, wantValuation: 270000000, wantPrincipal: 300000000, wantGain: -30000000, wantRateTenths: -100},
		{name: "b_3000000円と0円", valuationYen: 3000000, gainYen: 0, wantValuation: 300000000, wantPrincipal: 300000000, wantGain: 0, wantRateTenths: 0},
		{name: "c_1234567円と234567円", valuationYen: 1234567, gainYen: 234567, wantValuation: 123456700, wantPrincipal: 100000000, wantGain: 23456700, wantRateTenths: 235},
		{name: "d_876543円と-123457円", valuationYen: 876543, gainYen: -123457, wantValuation: 87654300, wantPrincipal: 100000000, wantGain: -12345700, wantRateTenths: -123},
		{name: "e_1000円と1円", valuationYen: 1000, gainYen: 1, wantValuation: 100000, wantPrincipal: 99900, wantGain: 100, wantRateTenths: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			list := calculateWithAccounts(t, nil, nil, []dto.ManagedAccountRow{managedRow("WealthNavi", tc.valuationYen, tc.gainYen)})

			want := []dto.ManagedAccount{{
				Name:                     "WealthNavi",
				ValuationDate:            "2026-11-16",
				ValuationHundredths:      tc.wantValuation,
				PrincipalHundredths:      tc.wantPrincipal,
				UnrealizedGainHundredths: tc.wantGain,
				UnrealizedGainRateTenths: tc.wantRateTenths,
			}}
			if !reflect.DeepEqual(list.ManagedAccounts, want) {
				t.Errorf("ManagedAccounts:\n got %+v\nwant %+v", list.ManagedAccounts, want)
			}
		})
	}
}

// 補助（要件12・プラン 5.2）: 率の「ちょうど半分」は絶対値で四捨五入する。
// 損益 +5・評価額 2,005円（元本 2,000円。0.25%）で 3、損益 -5・評価額 1,995円（-0.25%）で -3。
// 0に向かって切る・偶数に丸める誤りなら 2・-2 になる。
func Test補助_口座の率のちょうど半分は絶対値で四捨五入する(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                  string
		valuationYen, gainYen int64
		wantRateTenths        int64
	}{
		{"プラスの0_25パーセントは3", 2005, 5, 3},
		{"マイナスの-0_25パーセントは-3", 1995, -5, -3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			list := calculateWithAccounts(t, nil, nil, []dto.ManagedAccountRow{managedRow("WealthNavi", tc.valuationYen, tc.gainYen)})

			if len(list.ManagedAccounts) != 1 {
				t.Fatalf("ManagedAccounts の数: got %d, want 1", len(list.ManagedAccounts))
			}
			if got := list.ManagedAccounts[0].UnrealizedGainRateTenths; got != tc.wantRateTenths {
				t.Errorf("率（0.1%% 単位）: got %d, want %d", got, tc.wantRateTenths)
			}
			if got := list.ManagedAccounts[0].PrincipalHundredths; got != 200000 {
				t.Errorf("元本（0.01 単位）: got %d, want 200000（2,000円）", got)
			}
		})
	}
}

// 基準9（要件15・要件16）: 7203「買 2,500円 100株」・6758「買 4,000円 10株」・AAPL「買 150.25ドル 10株」（どれも価格あり）と
// WealthNavi（評価額 4,000,000円・損益 +1,000,000円）
// → 円の合計が 433500000（4,335,000円）・含み損益 104500000（+1,045,000円）・率 318（31.8%）・ExcludesUnpriced false。
// ドルの合計は口座がないときと同じ（180000・29750・198）。
// （円の評価額 300,000 ＋ 35,000 ＋ 4,000,000。含み損益 50,000 − 5,000 ＋ 1,000,000。率 1,045,000 ÷（250,000 ＋ 40,000 ＋ 3,000,000）× 100 ＝ 31.76…%）
func Test基準9_円の合計に口座の評価額と損益と元本を含める(t *testing.T) {
	t.Parallel()

	list := calculateWithAccounts(t, basis9Rows(), nil, []dto.ManagedAccountRow{managedRow("WealthNavi", 4000000, 1000000)})

	assertTotals(t, list, []dto.CurrencyTotal{
		jpyTotal(433500000, false, ptr[int64](104500000), ptr[int64](318)),
		usdTotal(180000, false, ptr[int64](29750), ptr[int64](198)),
	})
}

// 基準10（要件17）: 9984（価格なし）「買 4,500円 100株」と WealthNavi（4,000,000円・+1,000,000円）
// → 円の合計が 400000000・100000000・333（33.3%）・ExcludesUnpriced true（価格のない円の銘柄があるときだけ注記）。
func Test基準10_円の銘柄が価格なしだけでも口座があれば円の合計は口座の値で注記のフラグはtrue(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{softbank().buy(1, 100, 450000, "2026-01-10")}

	list := calculateWithAccounts(t, rows, nil, []dto.ManagedAccountRow{managedRow("WealthNavi", 4000000, 1000000)})

	assertTotals(t, list, []dto.CurrencyTotal{
		jpyTotal(400000000, true, ptr[int64](100000000), ptr[int64](333)),
	})
}

// 基準11（要件18）: 基準9 のデータとレート 150.00円（15000）で、円にまとめた合計が 460500000（4,605,000円）・
// 含み損益 108962500（+1,089,625円）・率 310（31.0%）・ExcludesUnpriced false。
// （評価額 4,335,000 ＋ 1,800.00 × 150。含み損益 1,045,000 ＋ 297.50 × 150。
// 率（45,000 ＋ 1,000,000 ＋ 297.50 × 150）÷（290,000 ＋ 3,000,000 ＋ 1,502.50 × 150）× 100 ＝ 30.99…%）
func Test基準11_円にまとめた合計にも口座を含める(t *testing.T) {
	t.Parallel()
	rate := usdJpy(15000)

	list := calculateWithAccounts(t, basis9Rows(), rate, []dto.ManagedAccountRow{managedRow("WealthNavi", 4000000, 1000000)})

	assertCombined(t, list.CombinedTotal, 460500000, false, ptr[int64](108962500), ptr[int64](310))
	if list.CombinedTotal != nil && list.CombinedTotal.Rate != *rate {
		t.Errorf("Rate: got %+v, want %+v（渡したレートをそのまま入れる）", list.CombinedTotal.Rate, *rate)
	}
}

// 基準12（要件18）: MSFT「買 400.00ドル 5株」（価格なし）と WealthNavi（4,000,000円・+1,000,000円）、レート 150.00円
// → 円にまとめた合計が 400000000・100000000（含み損益が nil でない）・333・ExcludesUnpriced true。
// Totals は円（400000000・100000000・333・false）とドル（0・nil・nil・true）。
func Test基準12_円とドルのすべての銘柄が価格なしでも口座があれば円にまとめた含み損益は取得できる(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{msft().buy(1, 5, 40000, "2026-01-10")}

	list := calculateWithAccounts(t, rows, usdJpy(15000), []dto.ManagedAccountRow{managedRow("WealthNavi", 4000000, 1000000)})

	assertCombined(t, list.CombinedTotal, 400000000, true, ptr[int64](100000000), ptr[int64](333))
	assertTotals(t, list, []dto.CurrencyTotal{
		jpyTotal(400000000, false, ptr[int64](100000000), ptr[int64](333)),
		usdTotal(0, true, nil, nil),
	})
}

// 基準13（要件17・要件18・要件19）: 売買履歴なしと WealthNavi（4,000,000円・+1,000,000円）、レート 150.00円
// → Holdings は空（nil ではない）、Totals は円だけ（400000000・100000000・333・false）。
// CombinedTotal は nil（レートがあっても、ドルの銘柄がなければ作らない。要件18）。
func Test基準13_銘柄がなくても口座があれば合計は円だけでCombinedTotalはnil(t *testing.T) {
	t.Parallel()

	list := calculateWithAccounts(t, []dto.TradeRow{}, usdJpy(15000), []dto.ManagedAccountRow{managedRow("WealthNavi", 4000000, 1000000)})

	if list.Holdings == nil || len(list.Holdings) != 0 {
		t.Errorf("Holdings: got %v, want 空のスライス（nil ではない）", list.Holdings)
	}
	assertTotals(t, list, []dto.CurrencyTotal{
		jpyTotal(400000000, false, ptr[int64](100000000), ptr[int64](333)),
	})
	if list.CombinedTotal != nil {
		t.Errorf("CombinedTotal: got %+v, want nil（ドルの銘柄がない）", *list.CombinedTotal)
	}
}

// 補助（要件16・プラン 5.2）: 口座どうしの合計の率は、損益の和 ÷ 元本の和。
// WealthNavi（4,000,000円・+1,000,000円）と Alpha（500,000円・0円）→ 円の合計が 450000000・100000000・286（1,000,000 ÷ 3,500,000 ＝ 28.57%）。
// 口座ごとの率を平均する誤りなら 167。
func Test補助_口座どうしの合計の率は損益の和を元本の和で割る(t *testing.T) {
	t.Parallel()
	accounts := []dto.ManagedAccountRow{
		managedRow("WealthNavi", 4000000, 1000000),
		managedRow("Alpha", 500000, 0),
	}

	list := calculateWithAccounts(t, nil, nil, accounts)

	assertTotals(t, list, []dto.CurrencyTotal{
		jpyTotal(450000000, false, ptr[int64](100000000), ptr[int64](286)),
	})
}

// 補助（要件16・プラン 5.2）: 率は丸める前の値で出す。7203「買 100.4円 1株」（現在の価格 100.0円。丸める前の含み損益 -0.4円、表示は0円）と
// 口座（評価額 2円・損益 +1円。元本 1円）→ 円の含み損益 100（+1円）・率 6（0.6 ÷ 101.4 ＝ 0.59%）。
// 表示する含み損益の和から出す誤りなら 10（1 ÷ 101.4 ＝ 0.99%）。
func Test補助_合計の率は丸める前の含み損益の和から出す(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{pricedAt(toyotaCode, toyotaName, 10000).buy(1, 1, 10040, "2026-01-10")}

	list := calculateWithAccounts(t, rows, nil, []dto.ManagedAccountRow{managedRow("WealthNavi", 2, 1)})

	if len(list.Totals) != 1 || list.Totals[0].Currency != dto.CurrencyJPY {
		t.Fatalf("Totals: got %s, want 円の1件", formatTotals(list.Totals))
	}
	got := list.Totals[0]
	if !samePtr(got.UnrealizedGainHundredths, ptr[int64](100)) {
		t.Errorf("円の含み損益（0.01 単位）: got %s, want 100", formatOptional(got.UnrealizedGainHundredths))
	}
	if !samePtr(got.UnrealizedGainRateTenths, ptr[int64](6)) {
		t.Errorf("円の率（0.1%% 単位）: got %s, want 6", formatOptional(got.UnrealizedGainRateTenths))
	}
}

// 補助（要件16）: 損益の和が0になる口座の組（+100円と -100円）→ 円の含み損益 0・率 0。
func Test補助_損益の和が0になる口座の組なら含み損益も率も0(t *testing.T) {
	t.Parallel()
	accounts := []dto.ManagedAccountRow{
		managedRow("Alpha", 1100, 100),
		managedRow("WealthNavi", 900, -100),
	}

	list := calculateWithAccounts(t, nil, nil, accounts)

	assertTotals(t, list, []dto.CurrencyTotal{
		jpyTotal(200000, false, ptr[int64](0), ptr[int64](0)),
	})
}

// 補助（要件17・プラン 5.2）: 口座があり、株式はドルの銘柄（AAPL）だけ → Totals が円（口座の分）→ ドルの順。
// AAPL は「買 150.25ドル 10株」・現在の価格 180.00ドル（ドルの合計は 180000・29750・198）。
func Test補助_口座があり株式がドルだけなら合計は円口座の分が先でドルが後(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{apple().buy(1, 10, 15025, "2026-01-10")}

	list := calculateWithAccounts(t, rows, nil, []dto.ManagedAccountRow{managedRow("WealthNavi", 4000000, 1000000)})

	assertTotals(t, list, []dto.CurrencyTotal{
		jpyTotal(400000000, false, ptr[int64](100000000), ptr[int64](333)),
		usdTotal(180000, false, ptr[int64](29750), ptr[int64](198)),
	})
}

// 補助（4.3・プラン 5.2）: accounts が nil でも空のスライスでも、ManagedAccounts は空のスライス（nil ではない）で、ほかの値は口座がないときと同じ。
// 売買履歴は 7203「買 2,500円 100株」（現在の価格 3,000円）→ 円の合計は 30000000・5000000・200・false。
func Test補助_口座がnilでも空でもManagedAccountsは空のスライスでほかの値は変わらない(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		accounts []dto.ManagedAccountRow
	}{
		{"nil", nil},
		{"空のスライス", []dto.ManagedAccountRow{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rows := []dto.TradeRow{toyota().buy(1, 100, 250000, "2026-01-10")}

			list := calculateWithAccounts(t, rows, nil, tc.accounts)

			if list.ManagedAccounts == nil || len(list.ManagedAccounts) != 0 {
				t.Errorf("ManagedAccounts: got %#v, want 空のスライス（nil ではない）", list.ManagedAccounts)
			}
			assertTotals(t, list, []dto.CurrencyTotal{
				jpyTotal(30000000, false, ptr[int64](5000000), ptr[int64](200)),
			})
			if got := codes(list); !slices.Equal(got, []string{toyotaCode}) {
				t.Errorf("一覧の銘柄コード: got %v, want [%s]", got, toyotaCode)
			}
		})
	}
}

// 補助（4.3）: 円の銘柄の評価額の和が int64 の上限に近い rows に口座を足すと ErrCalculationOverflow。
// 7203 を 30,744,573,456,181 株（現在の価格 3,000円。評価額 9223372036854300000 = int64 の最大より 475807 小さい）持つと、
// 口座がなければ収まる（対照）。評価額 4,000,000円（400000000）の口座を足すと合計があふれる。
func Test補助_円の合計に口座を足してint64に収まらなければErrCalculationOverflowになる(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{toyota().buy(1, 30744573456181, 10, "2026-01-10")}

	t.Run("対照_口座なしなら収まる", func(t *testing.T) {
		t.Parallel()

		if _, err := query.CalculateHoldings(rows, nil, nil, nil); err != nil {
			t.Fatalf("CalculateHoldings: got err %v, want nil（この対照が通らないとテストの前提が違う）", err)
		}
	})

	t.Run("口座を足すとあふれる", func(t *testing.T) {
		t.Parallel()

		_, err := query.CalculateHoldings(rows, nil, nil, []dto.ManagedAccountRow{managedRow("WealthNavi", 4000000, 1000000)})

		if !errors.Is(err, query.ErrCalculationOverflow) {
			t.Errorf("CalculateHoldings: got err %v, want ErrCalculationOverflow", err)
		}
	})
}
