package query_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// 仕様: 株式分割の要件（docs/requirements/stock-split.md）の基準1〜6（要件4〜6）と要件4・要件5・要件8、
// プラン（docs/plans/stock-split/plan.md）4.2・4.8 の PR① の境界（query.CalculateHoldings(rows, splits)・dto.StockSplitRow）、
// 5.1 の基準1〜6 の行と 5.2「Query 側の計算」。
// 金額は 0.01 単位の整数（例: 833.3円は 83330、37.56ドルは 3756）。期待値は要件・プランに書かれた値をそのまま使い、テストの中で計算しない。
// 銘柄マスタとダミー価格は calculate_holdings_test.go の toyota（3,000円・2026-10-06）・apple（180.00ドル・2026-10-06）を使う。
// 分割の日はどれも最新の終値の取引日（2026-10-06）より前にする（そうしないと要件8 で価格なしになる。要件8 のテストだけが後にする）。

// splitRow は、銘柄 code の「分割 1→ratio 分割の日 date」を作る。
func splitRow(code, date string, ratio int64) dto.StockSplitRow {
	return dto.StockSplitRow{Code: code, SplitDate: date, Ratio: ratio}
}

// calculateWithSplits は、分割 splits も渡して CalculateHoldings を呼び、エラーならテストを止める。
func calculateWithSplits(t *testing.T, rows []dto.TradeRow, splits []dto.StockSplitRow) dto.HoldingsList {
	t.Helper()
	list, err := query.CalculateHoldings(rows, splits)
	if err != nil {
		t.Fatalf("CalculateHoldings: got err %v, want nil", err)
	}
	return list
}

// 基準1（要件4・要件5）: 7203 の分割「1→3 2026-03-31」、Aの売買履歴「買 2,000円 100株 2026-01-10」「買 3,000円 100株 2026-02-10」
// → 600株・833.3円（200株・2,500円を分割して 600株・833.33…円）。
// さらに「売 850円 300株 2026-05-01」「買 900円 300株 2026-06-01」を足すと 600株・866.7円（(300×833.33…＋300×900)÷600＝866.66…円）。
func Test基準1_分割で保有数量を掛け取得価格を割り分割の後の売買も続けて計算する(t *testing.T) {
	t.Parallel()
	s := toyota()
	splits := []dto.StockSplitRow{splitRow("7203", "2026-03-31", 3)}
	rows := []dto.TradeRow{
		s.buy(1, 100, 200000, "2026-01-10"),
		s.buy(2, 100, 300000, "2026-02-10"),
	}

	got := onlyHolding(t, calculateWithSplits(t, rows, splits), "7203")

	assertQuantityAndAcquisition(t, got, 600, 83330)

	rows = append(rows,
		s.sell(3, 300, 85000, "2026-05-01"),
		s.buy(4, 300, 90000, "2026-06-01"),
	)

	got = onlyHolding(t, calculateWithSplits(t, rows, splits), "7203")

	assertQuantityAndAcquisition(t, got, 600, 86670)
}

// 基準2（要件5）: 7203 の分割「1→7 2026-03-31」、Aの売買履歴「買 1,000円 2株 2026-01-10」「買 300円 1株 2026-04-01」
// → 15株・153.3円（分割で 14株・142.857…円。(14×142.857…＋300)÷15＝153.33…円。分割の後で 142.9円に丸めてから使うと 153.4円になってしまう）。
func Test基準2_分割で割った取得価格は計算の途中で丸めない(t *testing.T) {
	t.Parallel()
	s := toyota()
	splits := []dto.StockSplitRow{splitRow("7203", "2026-03-31", 7)}
	rows := []dto.TradeRow{
		s.buy(1, 2, 100000, "2026-01-10"),
		s.buy(2, 1, 30000, "2026-04-01"),
	}

	got := onlyHolding(t, calculateWithSplits(t, rows, splits), "7203")

	assertQuantityAndAcquisition(t, got, 15, 15330)
}

// 基準3（要件4・要件5）: AAPL の分割「1→4 2026-03-31」、Aの売買履歴「買 150.25ドル 10株 2026-01-10」
// → 40株・37.56ドル（150.25÷4＝37.5625ドルの小数第3位を四捨五入）。
func Test基準3_ドルの銘柄は分割で割った取得価格を小数第3位で四捨五入する(t *testing.T) {
	t.Parallel()
	s := apple()
	splits := []dto.StockSplitRow{splitRow("AAPL", "2026-03-31", 4)}
	rows := []dto.TradeRow{s.buy(1, 10, 15025, "2026-01-10")}

	got := onlyHolding(t, calculateWithSplits(t, rows, splits), "AAPL")

	assertQuantityAndAcquisition(t, got, 40, 3756)
}

// 基準4（要件6）: 7203 の分割「1→2 2026-03-31」、Aの売買履歴「買 3,000円 100株 2026-03-30」「買 1,600円 100株 2026-03-31」
// → 300株・1,533.3円（3/30 の買いで 100株・3,000円、3/31 は分割が先で 200株・1,500円、その後の買いで (200×1,500＋100×1,600)÷300＝1,533.33…円。
// 買いを先にたどると 400株・1,150円になる）。
func Test基準4_同じ日は分割が先で売買履歴はその後にたどる(t *testing.T) {
	t.Parallel()
	s := toyota()
	splits := []dto.StockSplitRow{splitRow("7203", "2026-03-31", 2)}
	rows := []dto.TradeRow{
		s.buy(1, 100, 300000, "2026-03-30"),
		s.buy(2, 100, 160000, "2026-03-31"),
	}

	got := onlyHolding(t, calculateWithSplits(t, rows, splits), "7203")

	assertQuantityAndAcquisition(t, got, 300, 153330)
}

// 要件6（プラン 5.2）: 同じ日に分割・買い・売りが並ぶときは、分割が先で、買いと売りは id の順。
// 7203 の分割「1→2 2026-03-31」、Aの売買履歴「買 3,000円 100株 2026-03-30（id 1）」「買 1,600円 100株 2026-03-31（id 2）」「売 1,600円 300株 2026-03-31（id 3）」
// → 3/30 の100株が分割で200株になり、買いで300株、売りで0株。7203 は一覧に出ない（0株）。
// 売りの id を買いより小さくした形（id 2 が売り、id 3 が買い）では、売りを先にたどって 200株から300株を売るので負になり、ErrNegativeQuantity になる。
func Test要件6_同じ日の分割と買いと売りは分割が先で買いと売りはidの順にたどる(t *testing.T) {
	t.Parallel()
	s := toyota()
	splits := []dto.StockSplitRow{splitRow("7203", "2026-03-31", 2)}

	t.Run("買いのidが小さければ0株になり一覧に出ない", func(t *testing.T) {
		t.Parallel()
		// 渡す並びは id の逆順にする（rows の並びに頼らず id でたどる）。
		rows := []dto.TradeRow{
			s.sell(3, 300, 160000, "2026-03-31"),
			s.buy(2, 100, 160000, "2026-03-31"),
			s.buy(1, 100, 300000, "2026-03-30"),
		}

		list := calculateWithSplits(t, rows, splits)

		if got := codes(list); len(got) != 0 {
			t.Errorf("一覧の銘柄コード: got %v, want なし（7203 は 0株）", got)
		}
	})

	t.Run("売りのidが小さければ負になりErrNegativeQuantity", func(t *testing.T) {
		t.Parallel()
		rows := []dto.TradeRow{
			s.buy(3, 100, 160000, "2026-03-31"),
			s.sell(2, 300, 160000, "2026-03-31"),
			s.buy(1, 100, 300000, "2026-03-30"),
		}

		_, err := query.CalculateHoldings(rows, splits)

		if !errors.Is(err, query.ErrNegativeQuantity) {
			t.Errorf("CalculateHoldings: got err %v, want ErrNegativeQuantity", err)
		}
	})
}

// 基準5（要件4）: 7203 の分割「1→2 2026-03-31」、Aの売買履歴「買 2,000円 100株 2026-01-10」「売 2,500円 100株 2026-02-10」「買 1,500円 100株 2026-04-10」
// → 100株・1,500円（分割のときは保有数量0なので何も変わらない）。
func Test基準5_保有数量0のときの分割は何も変えない(t *testing.T) {
	t.Parallel()
	s := toyota()
	splits := []dto.StockSplitRow{splitRow("7203", "2026-03-31", 2)}
	rows := []dto.TradeRow{
		s.buy(1, 100, 200000, "2026-01-10"),
		s.sell(2, 100, 250000, "2026-02-10"),
		s.buy(3, 100, 150000, "2026-04-10"),
	}

	got := onlyHolding(t, calculateWithSplits(t, rows, splits), "7203")

	assertQuantityAndAcquisition(t, got, 100, 150000)
}

// 基準6（要件4）: 7203 の分割「1→2 2026-03-31」と「1→5 2026-08-01」、Aの売買履歴「買 3,000円 100株 2026-01-10」「買 1,400円 100株 2026-05-01」
// → 1,500株・293.3円（1回目の分割で 200株・1,500円、買いで 300株・1,466.66…円、2回目の分割で 1,500株・293.33…円）。
// 分割を逆の日付順（[1→5 2026-08-01, 1→2 2026-03-31]）で渡しても同じ値になる（splits の並びに頼らない）。
func Test基準6_分割が2回あっても日付の順にたどり分割の並びに頼らない(t *testing.T) {
	t.Parallel()
	s := toyota()
	rows := []dto.TradeRow{
		s.buy(1, 100, 300000, "2026-01-10"),
		s.buy(2, 100, 140000, "2026-05-01"),
	}
	cases := []struct {
		name   string
		splits []dto.StockSplitRow
	}{
		{"古い順", []dto.StockSplitRow{splitRow("7203", "2026-03-31", 2), splitRow("7203", "2026-08-01", 5)}},
		{"新しい順", []dto.StockSplitRow{splitRow("7203", "2026-08-01", 5), splitRow("7203", "2026-03-31", 2)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := onlyHolding(t, calculateWithSplits(t, rows, tc.splits), "7203")

			assertQuantityAndAcquisition(t, got, 1500, 29330)
		})
	}
}

// 要件4: 売買履歴のない銘柄の分割（9984 の「分割 1→2 2026-10-07」。保有する銘柄の終値 2026-10-06 より後の日にして、要件8 の判断に混ざる誤りも見分ける）を
// 渡しても、一覧は分割なしと変わらない。
func Test要件4_売買履歴のない銘柄の分割は一覧を変えない(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		toyota().buy(1, 100, 250000, "2026-01-10"),
		sony().buy(2, 10, 320000, "2026-01-10"),
	}
	splits := []dto.StockSplitRow{splitRow("9984", "2026-10-07", 2)}

	got := calculateWithSplits(t, rows, splits)

	want := calculateWithSplits(t, rows, nil)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("売買履歴のない銘柄の分割を渡した一覧が、分割なしと違う:\n got  %+v\n want %+v", got, want)
	}
	// 7203 の評価額が出ている（9984 の分割が 7203 の価格なしの判断に混ざらない）。
	if len(got.Holdings) != 2 {
		t.Fatalf("一覧の件数: got %d, want 2", len(got.Holdings))
	}
	h := got.Holdings[1] // 銘柄コードの昇順で 6758、7203 の順
	if h.Code != "7203" || h.ValuationHundredths == nil || *h.ValuationHundredths != 30000000 {
		t.Errorf("7203 の評価額: got %s, want 30000000（300,000円）", formatOptional(h.ValuationHundredths))
	}
}

// 要件8: 分割の日が最新の終値の取引日より前（または同じ日）なら、その終値を使う。
// 分割 1→3 2026-10-05 と 2026-10-06（終値の取引日と同じ日）のどちらでも、7203（100株）は 300株で、現在の価格 3,000円・基準日 2026-10-06・評価額 900,000円。
func Test要件8_分割の日が終値の取引日以前ならその終値を使う(t *testing.T) {
	t.Parallel()
	for _, splitDate := range []string{"2026-10-05", "2026-10-06"} {
		t.Run(splitDate, func(t *testing.T) {
			t.Parallel()
			rows := []dto.TradeRow{toyota().buy(1, 100, 200000, "2026-01-10")}
			splits := []dto.StockSplitRow{splitRow("7203", splitDate, 3)}

			got := onlyHolding(t, calculateWithSplits(t, rows, splits), "7203")

			if got.Quantity != 300 {
				t.Errorf("保有数量: got %d, want 300", got.Quantity)
			}
			if got.CurrentPriceHundredths == nil || *got.CurrentPriceHundredths != 300000 {
				t.Errorf("現在の価格: got %v, want 300000（3,000円）", formatOptional(got.CurrentPriceHundredths))
			}
			if got.PriceDate == nil || *got.PriceDate != "2026-10-06" {
				t.Errorf("価格の基準日: got %v, want 2026-10-06", got.PriceDate)
			}
			if got.ValuationHundredths == nil || *got.ValuationHundredths != 90000000 {
				t.Errorf("評価額: got %v, want 90000000（900,000円）", formatOptional(got.ValuationHundredths))
			}
		})
	}
}

// assertUnpriced は、h の現在の価格・価格の基準日・評価額・含み損益・率がすべて nil（価格なし）であることを確かめる。
func assertUnpriced(t *testing.T, h dto.Holding) {
	t.Helper()
	if h.CurrentPriceHundredths != nil || h.PriceDate != nil || h.ValuationHundredths != nil ||
		h.UnrealizedGainHundredths != nil || h.UnrealizedGainRateTenths != nil {
		t.Errorf("%s: 価格なしのはずが値がある: 現在の価格 %v・基準日 %v・評価額 %v・含み損益 %v・率 %v", h.Code,
			formatOptional(h.CurrentPriceHundredths), h.PriceDate, formatOptional(h.ValuationHundredths),
			formatOptional(h.UnrealizedGainHundredths), formatOptional(h.UnrealizedGainRateTenths))
	}
}

// 要件8: 最新の終値の取引日（2026-10-06）が、その銘柄の最も新しい分割の日（2026-10-07）より前のとき、価格なしとして扱う。
// 保有数量・取得価格は分割を反映した値（300株・666.7円）で、現在の価格・基準日・評価額・含み損益は nil。
// 円の合計は、評価額 0・価格を取得できない銘柄を含みません（ExcludesUnpriced）・含み損益は nil（7203 は円のただ1つの銘柄）。
func Test要件8_最新の終値の取引日が分割の日より前なら価格なしとして扱う(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{toyota().buy(1, 100, 200000, "2026-01-10")}
	splits := []dto.StockSplitRow{splitRow("7203", "2026-10-07", 3)}

	list := calculateWithSplits(t, rows, splits)

	got := onlyHolding(t, list, "7203")
	assertQuantityAndAcquisition(t, got, 300, 66670)
	assertUnpriced(t, got)
	assertTotals(t, list, []dto.CurrencyTotal{jpyTotal(0, true, nil, nil)})
}

// 要件8: 分割は保有数量0のとき（基準5 の形）でも、最も新しい分割の日として要件8 に使う。
// 終値 2026-03-30、分割「1→2 2026-03-31」、Aの売買履歴「買 100株 2026-01-10」「売 100株 2026-02-10」（分割のとき保有数量0）「買 100株 2026-04-10」
// → 100株は持っているが、価格なし（終値の取引日が分割の日より前）。
func Test要件8_保有数量0のときの分割も最新の分割の日として価格なしの判断に使う(t *testing.T) {
	t.Parallel()
	s := stockInfo{code: toyotaCode, name: toyotaName, currency: dto.CurrencyJPY, closeHundredths: ptr(int64(300000)), priceDate: ptr("2026-03-30")}
	rows := []dto.TradeRow{
		s.buy(1, 100, 200000, "2026-01-10"),
		s.sell(2, 100, 250000, "2026-02-10"),
		s.buy(3, 100, 150000, "2026-04-10"),
	}
	splits := []dto.StockSplitRow{splitRow("7203", "2026-03-31", 2)}

	got := onlyHolding(t, calculateWithSplits(t, rows, splits), "7203")

	assertQuantityAndAcquisition(t, got, 100, 150000)
	assertUnpriced(t, got)
}

// 要件8（最も新しい分割）: 7203 に「分割 1→2 2026-03-31」と「分割 1→2 2026-10-07」、終値 2026-10-06 → 7203 は価格なし。
// 分割を古い順と新しい順の両方の並びで渡す（最も古い分割や先頭の分割で判断する誤りを見分ける）。
func Test要件8_分割が複数あるときは最も新しい分割の日で価格なしを判断する(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{toyota().buy(1, 100, 200000, "2026-01-10")}
	cases := []struct {
		name   string
		splits []dto.StockSplitRow
	}{
		{"古い順", []dto.StockSplitRow{splitRow("7203", "2026-03-31", 2), splitRow("7203", "2026-10-07", 2)}},
		{"新しい順", []dto.StockSplitRow{splitRow("7203", "2026-10-07", 2), splitRow("7203", "2026-03-31", 2)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := onlyHolding(t, calculateWithSplits(t, rows, tc.splits), "7203")

			assertQuantityAndAcquisition(t, got, 400, 50000)
			assertUnpriced(t, got)
		})
	}
}

// 要件8（その銘柄の分割）: 7203 と 6758 を保有し、6758 だけに「分割 1→2 2026-10-07」、どちらも終値 2026-10-06
// → 7203 は価格あり、6758 は価格なし（銘柄を問わず分割全体の最新の日で判断する誤りを見分ける）。
func Test要件8_価格なしはその銘柄の分割だけで判断し他の銘柄の分割に影響されない(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		toyota().buy(1, 100, 200000, "2026-01-10"),
		sony().buy(2, 10, 320000, "2026-01-10"),
	}
	splits := []dto.StockSplitRow{splitRow("6758", "2026-10-07", 2)}

	list := calculateWithSplits(t, rows, splits)

	if got := codes(list); len(got) != 2 || got[0] != "6758" || got[1] != "7203" {
		t.Fatalf("一覧の銘柄コード: got %v, want [6758 7203]", got)
	}
	assertUnpriced(t, list.Holdings[0]) // 6758
	toyotaRow := list.Holdings[1]
	if toyotaRow.CurrentPriceHundredths == nil || *toyotaRow.CurrentPriceHundredths != 300000 ||
		toyotaRow.ValuationHundredths == nil || *toyotaRow.ValuationHundredths != 30000000 {
		t.Errorf("7203 の現在の価格・評価額: got %v・%v, want 300000・30000000（価格あり）",
			formatOptional(toyotaRow.CurrentPriceHundredths), formatOptional(toyotaRow.ValuationHundredths))
	}
	// 円の合計は 7203 の評価額だけで、価格のない 6758 がいるので注記が付く。
	if len(list.Totals) != 1 || list.Totals[0].ValuationHundredths != 30000000 || !list.Totals[0].ExcludesUnpriced {
		t.Errorf("Totals: got %s, want 円・評価額 30000000・注記あり", formatTotals(list.Totals))
	}
}

// 要件5（0.01円の切り捨て）: 「買 100円 1株 2026-01-10」「分割 1→3 2026-03-31」「買 20.2円 1株 2026-04-01」→ 4株・30.1円（3010）。
// (10000＋2020)÷4＝3005（0.01 単位）の四捨五入（0.1円の単位で 3010）。分割の後の取得価格を 0.01円で切り捨てると
// (9999＋2020)÷4＝3004.75 で 3000 になってしまう（分割で割った取得価格を丸めない）。
func Test要件5_分割で割った取得価格を0_01円の単位でも切り捨てない(t *testing.T) {
	t.Parallel()
	s := toyota()
	splits := []dto.StockSplitRow{splitRow("7203", "2026-03-31", 3)}
	rows := []dto.TradeRow{
		s.buy(1, 1, 10000, "2026-01-10"),
		s.buy(2, 1, 2020, "2026-04-01"),
	}

	got := onlyHolding(t, calculateWithSplits(t, rows, splits), "7203")

	assertQuantityAndAcquisition(t, got, 4, 3010)
}

// 5.2「桁あふれ」: 「買 9,999,999株」と分割 1→1,000 を4回（9,999,999 × 1,000^4 は int64 に収まらない）→ ErrCalculationOverflow（保有数量）。
// 計算は math/big で行うので途中では桁あふれせず、最後の保有数量が収まらないことをエラーにする（黙って桁あふれした値を返さない）。
func Test桁あふれ_分割を重ねて保有数量がint64に収まらなければErrCalculationOverflowになる(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{toyota().buy(1, 9999999, 10, "2026-01-10")}
	splits := []dto.StockSplitRow{
		splitRow("7203", "2026-02-01", 1000),
		splitRow("7203", "2026-02-02", 1000),
		splitRow("7203", "2026-02-03", 1000),
		splitRow("7203", "2026-02-04", 1000),
	}

	_, err := query.CalculateHoldings(rows, splits)

	if !errors.Is(err, query.ErrCalculationOverflow) {
		t.Errorf("CalculateHoldings: got err %v, want ErrCalculationOverflow", err)
	}
}
