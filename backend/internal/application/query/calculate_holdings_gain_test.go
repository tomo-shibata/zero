package query_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// 仕様: カード表示の要件（docs/requirements/holdings-cards.md）の基準9〜13・基準16（要件7〜要件10・要件13）と、
// プラン（docs/plans/holdings-cards/plan.md）4.2（含み損益の計算）・4.6（PR① の境界）・5.1（9〜13・16）・5.3（PR① の Go の補助のテスト）。
// 確かめる関数は query.CalculateHoldings。行の dto.Holding と、通貨ごとの dto.CurrencyTotal の
// UnrealizedGainHundredths（表示する含み損益。0.01 単位。円は100の倍数）・UnrealizedGainRateTenths（率。0.1% 単位）。
// 期待値は要件・プランに書かれた値をそのまま使い、テストの中で計算しない。
// 銘柄・売買履歴を作る補助（toyota・sony・apple・msft・pricedAt・pricedUSDAt・assertTotals・assertGain など）は calculate_holdings_test.go にある。

// 基準9（要件7・要件8・要件9）: Aの 6758 の売買履歴が「買 2,000円 1株」「買 2,001円 2株」「売 2,100円 1株」で、現在の価格が 3,500円
// → 「+2,999円（+74.9%）」（2株・取得価格 2,000.666…円、取得額 4,001.333…円、評価額 7,000円。含み損益 2,998.666…円を四捨五入して 2,999円、
// 率は 2,998.666… ÷ 4,001.333… × 100 ＝ 74.94…% → 74.9%）。0.01 単位で 299900・749。
func Test基準9_含み損益は丸める前の取得額で計算し円は四捨五入し率は小数第1位にする(t *testing.T) {
	t.Parallel()
	s := sony()
	rows := []dto.TradeRow{
		s.buy(1, 1, 200000, "2026-01-10"),
		s.buy(2, 2, 200100, "2026-01-11"),
		s.sell(3, 1, 210000, "2026-01-12"),
	}

	h := onlyHolding(t, calculate(t, rows), sonyCode)

	assertGain(t, h, 299900, 749)
}

// 基準10（要件8・要件9）: Aの 6758 の売買履歴が「買 4,000円 1株」「買 4,001円 2株」「売 4,100円 2株」で、現在の価格が 3,500円
// → 「-501円（-12.5%）」（1株・取得額 4,000.666…円、評価額 3,500円。含み損益 -500.666…円を絶対値で四捨五入して -501円。
// 率は -500.666… ÷ 4,000.666… × 100 ＝ -12.51…% → -12.5%）。0.01 単位で -50100・-125。
func Test基準10_マイナスの含み損益は絶対値で四捨五入する(t *testing.T) {
	t.Parallel()
	s := sony()
	rows := []dto.TradeRow{
		s.buy(1, 1, 400000, "2026-01-10"),
		s.buy(2, 2, 400100, "2026-01-11"),
		s.sell(3, 2, 410000, "2026-01-12"),
	}

	h := onlyHolding(t, calculate(t, rows), sonyCode)

	assertGain(t, h, -50100, -125)
}

// 基準11（要件7・要件8）: Aの AAPL の売買履歴が「買 100.00ドル 1株」「買 100.01ドル 2株」「売 120.00ドル 1株」で、現在の価格が 180.00ドル
// → 「+159.99ドル（+80.0%）」（2株・取得額 200.01333…ドル、評価額 360.00ドル。含み損益 159.98666…ドルを小数第3位で四捨五入して 159.99ドル。
// 率は 159.98666… ÷ 200.01333… × 100 ＝ 79.99…% → 80.0%）。0.01 単位で 15999・800。
func Test基準11_ドルの含み損益は小数第3位を四捨五入する(t *testing.T) {
	t.Parallel()
	s := apple()
	rows := []dto.TradeRow{
		s.buy(1, 1, 10000, "2026-01-10"),
		s.buy(2, 2, 10001, "2026-01-11"),
		s.sell(3, 1, 12000, "2026-01-12"),
	}

	h := onlyHolding(t, calculate(t, rows), appleCode)

	assertGain(t, h, 15999, 800)
}

// 基準12（要件7）: 7203 の現在の価格が 1,234.5円、売買履歴が「買 1,000円 3株」
// → 「+703円（+23.4%）」（評価額は 3,703.5円を切り捨てた 3,703円を使い、3,703 − 3,000 ＝ 703円。率は 703 ÷ 3,000 × 100 ＝ 23.43…%）。
// 0.01 単位で 70300・234。
func Test基準12_含み損益の評価額は切り捨てた表示の値を使う(t *testing.T) {
	t.Parallel()
	s := pricedAt(toyotaCode, toyotaName, 123450)
	rows := []dto.TradeRow{s.buy(1, 3, 100000, "2026-01-10")}

	h := onlyHolding(t, calculate(t, rows), toyotaCode)

	assertGain(t, h, 70300, 234)
}

// 基準13（要件10）の Go の部分: 含み損益と取得額の組 (a)〜(f) になる売買履歴と価格から、丸めた含み損益と率（0.1% 単位）を確かめる。
// 文字列にする部分は handler の formatUnrealizedGain・formatGainRate（holdings_response_test.go）と、画面の formatUnrealizedGain。
//
//	(a) 円 75,000・取得額 375,000: 3,750円 100株、現在の価格 4,500円 → 率 20.0%
//	(b) 円 -30,000・取得額 420,000: 4,200円 100株、現在の価格 3,900円 → 率 -7.14…% → -7.1%
//	(c) 円 1,234,567・取得額 1,000,000: 1,000,000円 1株、現在の価格 2,234,567円 → 率 123.4567% → 123.5%
//	(d) ドル 297.5・取得額 1,502.5: 150.25ドル 10株、現在の価格 180.00ドル → 率 19.80…% → 19.8%
//	(e) ドル -0.01・取得額 100: 100.00ドル 1株、現在の価格 99.99ドル → 率 -0.01% → 絶対値で四捨五入して 0（-0.0%）
//	(f) 円 1・取得額 1,000,000: 1,000,000円 1株、現在の価格 1,000,001円 → 率 0.0001% → 0.0%
func Test基準13_含み損益と取得額の6つの組で丸めた含み損益と率になる(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		row          dto.TradeRow
		code         string
		wantGain     int64
		wantRateTens int64
	}{
		{"a_円_75000", pricedAt(toyotaCode, toyotaName, 450000).buy(1, 100, 375000, "2026-01-10"), toyotaCode, 7500000, 200},
		{"b_円_-30000", pricedAt(toyotaCode, toyotaName, 390000).buy(1, 100, 420000, "2026-01-10"), toyotaCode, -3000000, -71},
		{"c_円_1234567", pricedAt(toyotaCode, toyotaName, 223456700).buy(1, 1, 100000000, "2026-01-10"), toyotaCode, 123456700, 1235},
		{"d_ドル_297.50", pricedUSDAt(appleCode, appleName, 18000).buy(1, 10, 15025, "2026-01-10"), appleCode, 29750, 198},
		{"e_ドル_-0.01", pricedUSDAt(appleCode, appleName, 9999).buy(1, 1, 10000, "2026-01-10"), appleCode, -1, 0},
		{"f_円_1", pricedAt(toyotaCode, toyotaName, 100000100).buy(1, 1, 100000000, "2026-01-10"), toyotaCode, 100, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := onlyHolding(t, calculate(t, []dto.TradeRow{tc.row}), tc.code)

			assertGain(t, h, tc.wantGain, tc.wantRateTens)
		})
	}
}

// 基準16（要件13）: Aの 6758 の売買履歴が「買 2,000円 1株」「買 2,001円 2株」「売 2,100円 1株」（現在の価格 3,500円、カードは +2,999円）、
// 7203 の売買履歴が同じ（現在の価格 3,000円、カードは +1,999円）
// → 円の含み損益の合計は「+4,998円（+62.4%）」（金額はカードの 2,999 ＋ 1,999。率は丸める前の 4,997.333… ÷ 8,002.666… × 100 ＝ 62.44…%）。
// 0.01 単位で 499800・624。評価額の合計は 7,000円 ＋ 6,000円 ＝ 13,000円（1300000）。
func Test基準16_合計の含み損益はカードの含み損益の和で率は丸める前の値で計算する(t *testing.T) {
	t.Parallel()
	sonyRows := func(s stockInfo, firstID int64) []dto.TradeRow {
		return []dto.TradeRow{
			s.buy(firstID, 1, 200000, "2026-01-10"),
			s.buy(firstID+1, 2, 200100, "2026-01-11"),
			s.sell(firstID+2, 1, 210000, "2026-01-12"),
		}
	}
	rows := append(sonyRows(sony(), 1), sonyRows(toyota(), 4)...)

	list := calculate(t, rows)

	if got, want := codes(list), []string{sonyCode, toyotaCode}; !slices.Equal(got, want) {
		t.Fatalf("銘柄コード: got %v, want %v", got, want)
	}
	assertGain(t, list.Holdings[0], 299900, 749) // 6758: +2,999円
	assertGain(t, list.Holdings[1], 199900, 500) // 7203: +1,999円。率は 1,998.666… ÷ 4,001.333… × 100 ＝ 49.95…% → 50.0%
	assertTotals(t, list, []dto.CurrencyTotal{
		jpyTotal(1300000, false, ptr[int64](499800), ptr[int64](624)),
	})
}

// ---- 補助のテスト（プラン 5.3。PR①） ----

// 5.3: 円の含み損益がちょうど半分のとき、絶対値で四捨五入する。
//   - 取得額 100.5円・評価額 101円 → G = +0.5円 → +1円（100）。率は 0.4975…% → 0.5%（5）
//   - 取得額 100.5円・評価額 100円 → G = -0.5円 → -1円（-100）。率は -0.4975…% → -0.5%（-5）
func Test補助_円の含み損益がちょうど半分なら絶対値で四捨五入する(t *testing.T) {
	t.Parallel()

	t.Run("プラス0.5円は+1円", func(t *testing.T) {
		t.Parallel()
		rows := []dto.TradeRow{pricedAt(toyotaCode, toyotaName, 10100).buy(1, 1, 10050, "2026-01-10")}

		h := onlyHolding(t, calculate(t, rows), toyotaCode)

		assertGain(t, h, 100, 5)
	})

	t.Run("マイナス0.5円は-1円", func(t *testing.T) {
		t.Parallel()
		rows := []dto.TradeRow{pricedAt(toyotaCode, toyotaName, 10000).buy(1, 1, 10050, "2026-01-10")}

		h := onlyHolding(t, calculate(t, rows), toyotaCode)

		assertGain(t, h, -100, -5)
	})
}

// 5.3: 率がちょうど 0.05% のとき 0.1%（1）、-0.05% のとき -0.1%（-1）。
// 取得額 10,000円・評価額 10,005円 → +5円（500）・0.05% ／ 評価額 9,995円 → -5円（-500）・-0.05%。
func Test補助_率がちょうど0_05パーセントなら絶対値で四捨五入して0_1パーセントになる(t *testing.T) {
	t.Parallel()

	t.Run("プラス0.05%は0.1%", func(t *testing.T) {
		t.Parallel()
		rows := []dto.TradeRow{pricedAt(toyotaCode, toyotaName, 1000500).buy(1, 1, 1000000, "2026-01-10")}

		h := onlyHolding(t, calculate(t, rows), toyotaCode)

		assertGain(t, h, 500, 1)
	})

	t.Run("マイナス0.05%は-0.1%", func(t *testing.T) {
		t.Parallel()
		rows := []dto.TradeRow{pricedAt(toyotaCode, toyotaName, 999500).buy(1, 1, 1000000, "2026-01-10")}

		h := onlyHolding(t, calculate(t, rows), toyotaCode)

		assertGain(t, h, -500, -1)
	})
}

// 5.3: 表示する含み損益が0で、丸める前は0でないとき、率も0（要件10「表示する含み損益が0のときは 0円（0.0%）」）。
// 「買 100.4円 1株」「買 100.0円 2株」・現在の価格 100.0円 → 3株・取得額 300.4円・評価額 300円 → G = -0.4円 → 0円。
// 率をそのまま丸めると -0.133…% → -0.1% になるが、0（0.0%）にする。
func Test補助_表示する含み損益が0なら率も0にする(t *testing.T) {
	t.Parallel()
	s := pricedAt(toyotaCode, toyotaName, 10000)
	rows := []dto.TradeRow{
		s.buy(1, 1, 10040, "2026-01-10"),
		s.buy(2, 2, 10000, "2026-01-11"),
	}

	h := onlyHolding(t, calculate(t, rows), toyotaCode)

	assertGain(t, h, 0, 0)
}

// 5.3: 合計で、カードの +1円と -1円を足して0になるとき、合計の率も0。
// 7203: 取得額 99.4円・評価額 100円 → G = +0.6円 → +1円（100）・率 0.60…% → 0.6%（6）。
// 6758: 取得額 100.9円・評価額 100円 → G = -0.9円 → -1円（-100）・率 -0.89…% → -0.9%（-9）。
// 含み損益の合計は 0円。合計の率をそのまま丸めると -0.3 ÷ 200.3 × 100 ＝ -0.14…% → -0.1% になるが、0 にする。
func Test補助_カードの含み損益の和が0なら合計の率も0にする(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		pricedAt(toyotaCode, toyotaName, 10000).buy(1, 1, 9940, "2026-01-10"),
		pricedAt(sonyCode, sonyName, 10000).buy(2, 1, 10090, "2026-01-10"),
	}

	list := calculate(t, rows)

	// 並びは銘柄コードの昇順（6758 → 7203）。
	if got, want := codes(list), []string{sonyCode, toyotaCode}; !slices.Equal(got, want) {
		t.Fatalf("銘柄コード: got %v, want %v", got, want)
	}
	assertGain(t, list.Holdings[0], -100, -9) // 6758
	assertGain(t, list.Holdings[1], 100, 6)   // 7203
	assertTotals(t, list, []dto.CurrencyTotal{
		jpyTotal(20000, false, ptr[int64](0), ptr[int64](0)),
	})
}

// 判断15（プラン 4.2）の補助: 合計の含み損益が +1円なのに、丸める前の率が負になるときは、合計の率を0にする（符号が逆の率を見せない）。
// 価格はどれも 100.0円で、7203「買 99.5円 1株」、6758「買 99.5円 1株」、9984「買 101.4円 1株」。
// 行は 7203・6758 が +0.5円 → +1円（100）・率 0.50…% → 0.5%（5）、9984 が -1.4円 → -1円（-100）・率 -1.38…% → -1.4%（-14）。
// 合計の含み損益は +1円（100）。合計の率は -0.4 ÷ 300.4 × 100 ＝ -0.13…% で丸めると -0.1%（-1）になるが、
// 含み損益（+1円）と符号が逆なので 0 にする。
func Test補助_合計の含み損益がプラスで丸めた率がマイナスになるときは合計の率を0にする(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		pricedAt(toyotaCode, toyotaName, 10000).buy(1, 1, 9950, "2026-01-10"),
		pricedAt(sonyCode, sonyName, 10000).buy(2, 1, 9950, "2026-01-10"),
		pricedAt(sbgCode, sbgName, 10000).buy(3, 1, 10140, "2026-01-10"),
	}

	list := calculate(t, rows)

	// 並びは銘柄コードの昇順（6758 → 7203 → 9984）。
	if got, want := codes(list), []string{sonyCode, toyotaCode, sbgCode}; !slices.Equal(got, want) {
		t.Fatalf("銘柄コード: got %v, want %v", got, want)
	}
	assertGain(t, list.Holdings[0], 100, 5)    // 6758
	assertGain(t, list.Holdings[1], 100, 5)    // 7203
	assertGain(t, list.Holdings[2], -100, -14) // 9984
	assertTotals(t, list, []dto.CurrencyTotal{
		jpyTotal(30000, false, ptr[int64](100), ptr[int64](0)),
	})
}

// 5.3: ドルがすべて価格なし・円は価格あり → ドルの合計の含み損益と率は nil（要件16）、円は値あり。
// 価格のない行（MSFT）の含み損益と率も nil（要件5）。円の 7203 は「買 2,500円 100株」・現在の価格 3,000円 → +50,000円・20.0%。
func Test補助_ドルがすべて価格なしでも円は価格があれば円の合計は値を持ちドルの合計はnilになる(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		toyota().buy(1, 100, 250000, "2026-01-10"),
		msft().buy(2, 5, 40000, "2026-01-10"),
	}

	list := calculate(t, rows)

	assertGain(t, list.Holdings[0], 5000000, 200)
	noPrice := list.Holdings[1]
	if noPrice.Code != msftCode {
		t.Fatalf("2行目の銘柄コード: got %q, want %q", noPrice.Code, msftCode)
	}
	if noPrice.UnrealizedGainHundredths != nil || noPrice.UnrealizedGainRateTenths != nil {
		t.Errorf("価格のない行の含み損益・率: got %v・%v, want どちらも nil",
			formatOptional(noPrice.UnrealizedGainHundredths), formatOptional(noPrice.UnrealizedGainRateTenths))
	}
	assertTotals(t, list, []dto.CurrencyTotal{
		jpyTotal(30000000, false, ptr[int64](5000000), ptr[int64](200)),
		usdTotal(0, true, nil, nil),
	})
}

// 要件13・要件15: 価格のない銘柄は、含み損益の合計にも率（取得額の合計）にも入れない。
// 7203（価格あり。取得額 250,000円・+50,000円）と 9984（価格なし。取得額 90,000円）→ 円の合計は +50,000円・20.0%（取得額に 9984 を入れると 14.7% になって誤り）。
func Test補助_価格のない銘柄は含み損益の合計にも率の取得額にも入れない(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		toyota().buy(1, 100, 250000, "2026-01-10"),
		softbank().buy(2, 10, 900000, "2026-01-10"),
	}

	list := calculate(t, rows)

	assertTotals(t, list, []dto.CurrencyTotal{
		jpyTotal(30000000, true, ptr[int64](5000000), ptr[int64](200)),
	})
}

// 5.3: 桁あふれで ErrCalculationOverflow（4.2。最後の値のうち、行の含み損益・率、合計の含み損益・率が int64 に収まらないとき）。
//  1. 行の含み損益があふれる: 1,000.00ドル（100000）× 10^15 株の買い・現在の価格 0.01ドル（取得額 10^20、評価額 10^15 → G ≈ -10^20）
//  2. 率だけがあふれる: 取得価格 0.01ドル・1株、現在の価格 5×10^18（0.01 単位）。G は収まるが、G ÷ 取得額 × 1000 が収まらない
//  3. 行ごとには収まり、合計の含み損益だけがあふれる: 1.00ドル（100）× 5×10^16 株の買い・現在の価格 0.01ドルの2銘柄
//     （1行は G ≈ -4.95×10^18 で収まり、2行の合計は ≈ -9.9×10^18 で収まらない。評価額の合計は 10^17 で収まる）
func Test補助_含み損益と率が桁あふれしたらErrCalculationOverflowになる(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		rows []dto.TradeRow
	}{
		{"1_行の含み損益があふれる", []dto.TradeRow{
			pricedUSDAt(appleCode, appleName, 1).buy(1, 1000000000000000, 100000, "2026-01-10"),
		}},
		{"2_率だけがあふれる", []dto.TradeRow{
			pricedUSDAt(appleCode, appleName, 5000000000000000000).buy(1, 1, 1, "2026-01-10"),
		}},
		{"3_行ごとには収まり合計の含み損益だけがあふれる", []dto.TradeRow{
			pricedUSDAt(appleCode, appleName, 1).buy(1, 50000000000000000, 100, "2026-01-10"),
			pricedUSDAt(msftCode, msftName, 1).buy(2, 50000000000000000, 100, "2026-01-10"),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := query.CalculateHoldings(tc.rows, nil, nil, nil)

			if !errors.Is(err, query.ErrCalculationOverflow) {
				t.Errorf("CalculateHoldings: got err %v, want ErrCalculationOverflow", err)
			}
		})
	}

	// 対照: 3 の1行だけなら収まり、エラーにならずに正しい値を返す（3 のエラーが、合計の含み損益のあふれによるものであることを確かめる）。
	// G = 評価額 5×10^16 − 取得額 5×10^18 = -4,950,000,000,000,000,000。率は -99.0%（-990）。
	t.Run("対照_3の1行だけなら収まる", func(t *testing.T) {
		t.Parallel()
		rows := []dto.TradeRow{pricedUSDAt(appleCode, appleName, 1).buy(1, 50000000000000000, 100, "2026-01-10")}

		h := onlyHolding(t, calculate(t, rows), appleCode)

		assertGain(t, h, -4950000000000000000, -990)
	})
}
