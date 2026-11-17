package query_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// 仕様: プラン（docs/plans/trade-history/plan.md）4.3 の「保有の計算」の 3・6（保有数量が途中で負になる売買履歴と、
// 最後の値が int64 に収まらない売買履歴は、誤った一覧を返さずにエラーにする）、4.6（PR①。PR② で金額は 0.01 単位・合計は通貨ごと）の
// 「HoldingsReader.ListTradeRows」と「ListHoldings.Execute は中で CalculateHoldings を使う」、5.2・5.3。
// 確かめるのは NewListHoldings・Execute。売買履歴は、境界の外側の HoldingsReader の fake から渡す。
// 計算そのものの値（基準1〜5 など）は calculate_holdings_test.go。ここは、Execute が読み取りの結果を計算に渡し、
// エラーを返すことを確かめる。

// fixedRowsReader は、どの利用者についても決まった売買履歴の行を返す HoldingsReader（境界の外側の fake）。
// 呼ばれた利用者 ID を記録する。
type fixedRowsReader struct {
	rows     []dto.TradeRow
	err      error
	gotUsers []uuid.UUID
	// order は、保存した並び（銘柄コード）。nil なら並びなし（カード表示のプラン 4.6 PR②: 既定の fake は空の並びを返す）。
	order []string
	// rate・rateErr は、LatestUsdJpyRate が返すレートとエラー（指定しなければ nil, nil。ドルを円にまとめるプラン 5.3 PR①）。
	rate    *dto.UsdJpyRate
	rateErr error
}

func (r *fixedRowsReader) LatestUsdJpyRate(context.Context) (*dto.UsdJpyRate, error) {
	return r.rate, r.rateErr
}

func (r *fixedRowsReader) ListTradeRows(_ context.Context, userID uuid.UUID) ([]dto.TradeRow, error) {
	r.gotUsers = append(r.gotUsers, userID)
	return r.rows, r.err
}

func (r *fixedRowsReader) ListStockSplits(context.Context, uuid.UUID) ([]dto.StockSplitRow, error) {
	return []dto.StockSplitRow{}, nil
}

func (r *fixedRowsReader) ListHoldingOrder(context.Context, uuid.UUID) ([]string, error) {
	if r.order == nil {
		return []string{}, nil
	}
	return r.order, nil
}

// userA は、要件の利用者A（テストデータ）。fake は利用者で絞らないので、どの ID でもよい。
var userA = uuid.MustParse("00000000-0000-0000-0000-00000000000a")

// Execute は、読み取りが返した売買履歴から一覧を作って返す。呼ぶ利用者 ID をそのまま読み取りに渡す。
// 値は要件の基準1 の前半（150株・2,500円 = 250000）と、7203 の終値 3,000円（評価額 450,000円 = 45000000。どちらも 0.01 単位）。
func Test補助_Executeは読み取りの売買履歴から一覧を作り利用者のIDをそのまま渡す(t *testing.T) {
	t.Parallel()
	s := toyota()
	reader := &fixedRowsReader{rows: []dto.TradeRow{
		s.buy(1, 100, 200000, "2026-01-10"),
		s.buy(2, 100, 300000, "2026-06-01"),
		s.sell(3, 50, 350000, "2026-07-01"),
	}}
	q := query.NewListHoldings(reader)

	got, err := q.Execute(t.Context(), userA)

	if err != nil {
		t.Fatalf("Execute: got err %v, want nil", err)
	}
	h := onlyHolding(t, got, toyotaCode)
	assertQuantityAndAcquisition(t, h, 150, 250000)
	if h.ValuationHundredths == nil || *h.ValuationHundredths != 45000000 {
		t.Errorf("評価額（0.01 単位）: got %v, want 45000000（150株 × 3,000円 = 450,000円）", h.ValuationHundredths)
	}
	// 取得額 375,000円（2,500円×150株）・評価額 450,000円 → +75,000円（7500000）・20.0%（200）
	assertTotals(t, got, []dto.CurrencyTotal{
		jpyTotal(45000000, false, ptr[int64](7500000), ptr[int64](200)),
	})
	if len(reader.gotUsers) != 1 || reader.gotUsers[0] != userA {
		t.Errorf("読み取りに渡された利用者 ID: got %v, want [%s]", reader.gotUsers, userA)
	}
}

// 売買履歴が0件でも、エラーにならず、Holdings・Totals は空のスライス（nil ではない。API で null にしないため）。
func Test補助_Executeは売買履歴が0件なら空のスライスを返す(t *testing.T) {
	t.Parallel()
	q := query.NewListHoldings(&fixedRowsReader{rows: []dto.TradeRow{}})

	got, err := q.Execute(t.Context(), userA)

	if err != nil {
		t.Fatalf("Execute: got err %v, want nil", err)
	}
	if got.Holdings == nil || len(got.Holdings) != 0 {
		t.Errorf("Holdings: got %v, want 空のスライス（nil ではない）", got.Holdings)
	}
	if got.Totals == nil || len(got.Totals) != 0 {
		t.Errorf("Totals: got %v, want 空のスライス（nil ではない）", got.Totals)
	}
}

// 読み取りが失敗したら、その失敗を（包んで）返す。
func Test補助_Executeは読み取りが失敗したらエラーを返す(t *testing.T) {
	t.Parallel()
	errRead := errors.New("fake: 読み取りに失敗")
	q := query.NewListHoldings(&fixedRowsReader{err: errRead})

	_, err := q.Execute(t.Context(), userA)

	if !errors.Is(err, errRead) {
		t.Errorf("Execute: got err %v, want 読み取りのエラーを包んだもの", err)
	}
}

// 5.3・判断5: 最後の値が int64 に収まらない売買履歴なら、Execute は誤った値を返さず ErrCalculationOverflow を返す。
// 例は calculate_holdings_test.go の overflowCases と同じ（プラン 5.3 の3つと、境界の1つ上）。
func Test補助_計算がint64に収まらない売買履歴ならExecuteはErrCalculationOverflowを返す(t *testing.T) {
	t.Parallel()
	for _, tc := range overflowCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := query.NewListHoldings(&fixedRowsReader{rows: tc.rows})

			got, err := q.Execute(t.Context(), userA)

			if !errors.Is(err, query.ErrCalculationOverflow) {
				t.Errorf("Execute: got err %v, want ErrCalculationOverflow\n返った一覧: %+v", err, got)
			}
		})
	}
}

// 5.3・判断6: 途中で保有数量が負になる売買履歴なら、Execute は ErrNegativeQuantity を返す（誤った一覧を返さない）。
func Test補助_保有数量が途中で負になる売買履歴ならExecuteはErrNegativeQuantityを返す(t *testing.T) {
	t.Parallel()
	for _, tc := range negativeCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := query.NewListHoldings(&fixedRowsReader{rows: tc.rows})

			got, err := q.Execute(t.Context(), userA)

			if !errors.Is(err, query.ErrNegativeQuantity) {
				t.Errorf("Execute: got err %v, want ErrNegativeQuantity\n返った一覧: %+v", err, got)
			}
		})
	}
}

// ドルを円にまとめるプラン 5.2・4.1: 偽の reader のレートが CalculateHoldings に渡る（CombinedTotal.Rate が同じ）。
// 売買履歴は 7203「買 2,500円 100株」・AAPL「買 150.25ドル 10株」（どちらも価格あり）、レートは 2026-11-14 150.00円（15000）
// → 評価額は 570,000円（57000000。要件定義の基準1）。
func Test補助_Executeは読み取りのレートを円にまとめる計算に渡す(t *testing.T) {
	t.Parallel()
	rate := &dto.UsdJpyRate{RateDate: "2026-11-14", RateHundredths: 15000}
	reader := &fixedRowsReader{
		rows: []dto.TradeRow{
			toyota().buy(1, 100, 250000, "2026-01-10"),
			apple().buy(2, 10, 15025, "2026-01-10"),
		},
		rate: rate,
	}
	q := query.NewListHoldings(reader)

	got, err := q.Execute(t.Context(), userA)

	if err != nil {
		t.Fatalf("Execute: got err %v, want nil", err)
	}
	if got.CombinedTotal == nil {
		t.Fatalf("CombinedTotal: got nil, want 値あり")
	}
	if got.CombinedTotal.Rate != *rate {
		t.Errorf("CombinedTotal.Rate: got %+v, want %+v", got.CombinedTotal.Rate, *rate)
	}
	if got.CombinedTotal.ValuationHundredths != 57000000 {
		t.Errorf("CombinedTotal.ValuationHundredths: got %d, want 57000000", got.CombinedTotal.ValuationHundredths)
	}
}

// ドルを円にまとめるプラン 5.2・4.1: LatestUsdJpyRate がエラーなら、Execute もそのエラーを（包んで）返す。
func Test補助_Executeはレートの読み取りが失敗したらエラーを返す(t *testing.T) {
	t.Parallel()
	errRate := errors.New("fake: レートの読み取りに失敗")
	q := query.NewListHoldings(&fixedRowsReader{
		rows:    []dto.TradeRow{toyota().buy(1, 100, 250000, "2026-01-10")},
		rateErr: errRate,
	})

	_, err := q.Execute(t.Context(), userA)

	if !errors.Is(err, errRate) {
		t.Errorf("Execute: got err %v, want レートの読み取りのエラーを包んだもの", err)
	}
}
