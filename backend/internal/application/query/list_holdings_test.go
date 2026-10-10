package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// 仕様: プラン（docs/plans/stock-holdings/plan.md）4.4「API」の
// 「計算（数量の合計、加重平均の途中の値 2S+Q と 2Q、評価額の途中の値 終値×数量、合計評価額）が int64 に収まらない保有データのときは、
// ListHoldings.Execute がエラーを返す」と、5.1「補助のテスト（バックエンド）」の
// 4.4 API のあふれ（PR② の EC-1）の行の単体の部分（PR② ステップ6 2ラウンド目の EC-8・TC-07 の (b)(d)、EC-9 の境界を含む）。
// 確かめるのは 4.4 の NewListHoldings・Execute。保有データは、境界の外側の HoldingsReader の fake から渡す。
// 期待値（エラーになる／ならない、対照と境界の値）はプランと要件の値をそのまま使い、テストの中で計算しない。

// fixedRowsReader は、どの利用者についても決まった保有データの行を返す HoldingsReader（境界の外側の fake）。
type fixedRowsReader struct {
	rows []dto.HoldingRow
}

func (r fixedRowsReader) ListHoldingRows(context.Context, uuid.UUID) ([]dto.HoldingRow, error) {
	return r.rows, nil
}

// userA は、要件の利用者A（プラン 4.4「テストデータ」）。fake は利用者で絞らないので、どの ID でもよい。
var userA = uuid.MustParse("00000000-0000-0000-0000-00000000000a")

// elevenStocksRows は、プラン 5.1 の (d) の行（PR② ステップ6 の EC-8・TC-07）:
// 銘柄コードの違う11行（1001〜1011）で、それぞれ数量 300000000000000・取得価格 1・終値 30000（単価は Tenths）。
// 各行の計算は収まり、合計評価額だけが11行目であふれる。fake から渡すので、銘柄マスタは要らない。
func elevenStocksRows() []dto.HoldingRow {
	stockCodes := []string{"1001", "1002", "1003", "1004", "1005", "1006", "1007", "1008", "1009", "1010", "1011"}
	rows := make([]dto.HoldingRow, 0, len(stockCodes))
	for _, code := range stockCodes {
		rows = append(rows, pricedRow(code, "テスト銘柄"+code, 300000000000000, 1, 30000, priceDate))
	}
	return rows
}

// assertExecuteFails は、rows を返す reader で Execute がエラーを返すことを確かめる。
func assertExecuteFails(t *testing.T, rows []dto.HoldingRow) {
	t.Helper()
	q := query.NewListHoldings(fixedRowsReader{rows: rows})

	got, err := q.Execute(t.Context(), userA)

	if err == nil {
		t.Errorf("Execute: err が nil（want エラー。計算が int64 に収まらない）\n返った一覧: %+v", got)
	}
}

// 4.4 API のあふれ（PR② の EC-1）: 計算が int64 に収まらない保有データのときは、Execute がエラーを返す
// （誤った値、たとえば負の評価額を、黙って返さない）。
// (a)(b)(d) の行と (c) の数量はプラン 5.1 の値。(c) の取得価格・終値はプランに指定がないので、
// 数量の合計のほかの途中の値ができるだけ小さくなるよう、最小の単価 0.1円（Tenths で 1）とした。
// (b) は PR② ステップ6 の EC-8・TC-07 で取得価格を 1 に直し（加重平均の途中の値で先に止まらず、評価額の途中の値
// 終値×数量だけがあふれる）、(d) を足した（合計評価額だけがあふれる）。
func TestPlan4_4_PR2_EC1_EC8_TC07_計算がint64に収まらない保有データならExecuteはエラーを返す(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		rows []dto.HoldingRow
	}{
		{
			// 加重平均の途中の値があふれる。
			name: "a_数量200000000000000_取得価格30000_終値30000",
			rows: []dto.HoldingRow{
				pricedRow(toyotaCode, toyotaName, 200000000000000, 30000, 30000, priceDate),
			},
		},
		{
			// PR2_EC8_TC07: 数量の合計・2S+Q・2Q は収まり、評価額の途中の値 終値×数量だけがあふれる。
			name: "b_PR2_EC8_TC07_数量400000000000000_取得価格1_終値30000",
			rows: []dto.HoldingRow{
				pricedRow(toyotaCode, toyotaName, 400000000000000, 1, 30000, priceDate),
			},
		},
		{
			// 同じ銘柄の2行の数量の合計があふれる。
			name: "c_同じ銘柄の2行がそれぞれ数量5000000000000000000",
			rows: []dto.HoldingRow{
				pricedRow(toyotaCode, toyotaName, 5000000000000000000, 1, 1, priceDate),
				pricedRow(toyotaCode, toyotaName, 5000000000000000000, 1, 1, priceDate),
			},
		},
		{
			// PR2_EC8_TC07: 各行の計算は収まり、合計評価額だけが11行目であふれる。
			name: "d_PR2_EC8_TC07_銘柄コードの違う11行がそれぞれ数量300000000000000_取得価格1_終値30000",
			rows: elevenStocksRows(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertExecuteFails(t, tc.rows)
		})
	}

	// 対照: 数量 100・取得価格 25000・終値 30000 ならエラーにならない
	// （上のエラーが、行の形などあふれ以外の理由でないことを確かめる）。
	// 値は要件 AC-1 のとおり（100株・2,500円・3,000円 → 評価額 300,000円）。
	t.Run("対照_数量100_取得価格25000_終値30000はエラーにならない", func(t *testing.T) {
		t.Parallel()
		q := query.NewListHoldings(fixedRowsReader{rows: []dto.HoldingRow{
			pricedRow(toyotaCode, toyotaName, 100, 25000, 30000, priceDate),
		}})

		got, err := q.Execute(t.Context(), userA)
		if err != nil {
			t.Fatalf("Execute: got err %v, want nil", err)
		}

		h := onlyHolding(t, got, toyotaCode)
		if h.Quantity != 100 {
			t.Errorf("保有数量: got %d, want 100", h.Quantity)
		}
		if h.AcquisitionPriceTenths != 25000 {
			t.Errorf("取得価格（0.1円単位）: got %d, want 25000", h.AcquisitionPriceTenths)
		}
		switch {
		case h.Valuation == nil:
			t.Error("評価額: got nil, want 300000（価格のある銘柄）")
		case *h.Valuation != 300000:
			t.Errorf("評価額: got %d, want 300000", *h.Valuation)
		}
		if got.TotalValuation != 300000 {
			t.Errorf("合計評価額: got %d, want 300000", got.TotalValuation)
		}
	})
}

// 4.4 API のあふれの境界（PR② ステップ6 の EC-9）: ぎりぎり収まる値ならエラーにならず、1つ上の値ならエラーになる。
// 値はプラン 5.1 の 4.4 API のあふれ（PR② の EC-1）の行の「境界」のとおり（単価は Tenths）。
//   - 価格のない1行・取得価格 1: 数量 3074457345618258602 ならエラーにならず（数量 3074457345618258602・取得価格 1）、
//     3074457345618258603 ならエラー（加重平均の途中の値 2S+Q の境界）。
//   - 価格のある1行・取得価格 1・終値 10: 数量 922337203685477580 ならエラーにならず（評価額 922337203685477580）、
//     922337203685477581 ならエラー（評価額の途中の値 終値×数量の境界）。
//
// エラーにならない側は、返る一覧の値も確かめる（境界の値で黙って桁あふれした値を返さないこと）。
// プランに書かれていない一覧の値は、要件・プラン 4.4 から決まる値を使う: 1行だけの銘柄の保有数量と取得価格は
// その行の値のまま（FR-4）、価格のない銘柄の評価額は nil（4.4）、合計評価額は nil 以外の評価額の合計（4.4）。
func TestPlan4_4_PR2_EC9_あふれの境界はぎりぎり収まる値ならエラーにならず1つ上の値ならエラーになる(t *testing.T) {
	t.Parallel()

	t.Run("PR2_EC9_価格のない1行_取得価格1_数量3074457345618258602はエラーにならない", func(t *testing.T) {
		t.Parallel()
		q := query.NewListHoldings(fixedRowsReader{rows: []dto.HoldingRow{
			unpricedRow(toyotaCode, toyotaName, 3074457345618258602, 1),
		}})

		got, err := q.Execute(t.Context(), userA)
		if err != nil {
			t.Fatalf("Execute: got err %v, want nil（ぎりぎり収まる値）", err)
		}

		h := onlyHolding(t, got, toyotaCode)
		if h.Quantity != 3074457345618258602 {
			t.Errorf("保有数量: got %d, want 3074457345618258602", h.Quantity)
		}
		if h.AcquisitionPriceTenths != 1 {
			t.Errorf("取得価格（0.1円単位）: got %d, want 1", h.AcquisitionPriceTenths)
		}
		if h.Valuation != nil {
			t.Errorf("評価額: got %d, want nil（価格のない銘柄）", *h.Valuation)
		}
		if got.TotalValuation != 0 {
			t.Errorf("合計評価額: got %d, want 0（評価額のある行がない）", got.TotalValuation)
		}
	})

	t.Run("PR2_EC9_価格のない1行_取得価格1_数量3074457345618258603はエラー", func(t *testing.T) {
		t.Parallel()
		assertExecuteFails(t, []dto.HoldingRow{
			unpricedRow(toyotaCode, toyotaName, 3074457345618258603, 1),
		})
	})

	t.Run("PR2_EC9_価格のある1行_取得価格1_終値10_数量922337203685477580はエラーにならない", func(t *testing.T) {
		t.Parallel()
		q := query.NewListHoldings(fixedRowsReader{rows: []dto.HoldingRow{
			pricedRow(toyotaCode, toyotaName, 922337203685477580, 1, 10, priceDate),
		}})

		got, err := q.Execute(t.Context(), userA)
		if err != nil {
			t.Fatalf("Execute: got err %v, want nil（ぎりぎり収まる値）", err)
		}

		h := onlyHolding(t, got, toyotaCode)
		if h.Quantity != 922337203685477580 {
			t.Errorf("保有数量: got %d, want 922337203685477580", h.Quantity)
		}
		if h.AcquisitionPriceTenths != 1 {
			t.Errorf("取得価格（0.1円単位）: got %d, want 1", h.AcquisitionPriceTenths)
		}
		switch {
		case h.Valuation == nil:
			t.Error("評価額: got nil, want 922337203685477580（価格のある銘柄）")
		case *h.Valuation != 922337203685477580:
			t.Errorf("評価額: got %d, want 922337203685477580", *h.Valuation)
		}
		if got.TotalValuation != 922337203685477580 {
			t.Errorf("合計評価額: got %d, want 922337203685477580（1行だけなのでその評価額）", got.TotalValuation)
		}
	})

	t.Run("PR2_EC9_価格のある1行_取得価格1_終値10_数量922337203685477581はエラー", func(t *testing.T) {
		t.Parallel()
		assertExecuteFails(t, []dto.HoldingRow{
			pricedRow(toyotaCode, toyotaName, 922337203685477581, 1, 10, priceDate),
		})
	})
}
