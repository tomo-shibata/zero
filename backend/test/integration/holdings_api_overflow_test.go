package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// 仕様: プラン（docs/plans/stock-holdings/plan.md）4.4「API」の
// 「計算（数量の合計、加重平均の途中の値 2S+Q と 2Q、評価額の途中の値 終値×数量、合計評価額）が int64 に収まらない保有データのときは、
// ListHoldings.Execute がエラーを返し、API は 500 と {"error":"internal_error"}」と、
// 5.1「補助のテスト（バックエンド）」の 4.4 API のあふれ（PR② の EC-1）の行の統合の部分（router を通すと 500。
// PR② ステップ6 2ラウンド目の EC-8・TC-07 の (b)(d) を含む）。
// 保有データは、境界の外側の HoldingsReader の fake から渡す（int64 の境界の数量は、DB に入れる手間をかけずに渡せるため。
// DB を使わないので、(d) の銘柄コード 1001〜1011 の銘柄マスタも要らない）。
// 単体の部分（EC-9 の境界を含む）は internal/application/query/list_holdings_test.go。

// fixedRowsHoldingsReader は、どの利用者についても決まった保有データの行を返す HoldingsReader（境界の外側の fake。EC-1 用）。
type fixedRowsHoldingsReader struct {
	rows []dto.HoldingRow
}

func (r fixedRowsHoldingsReader) ListHoldingRows(context.Context, uuid.UUID) ([]dto.HoldingRow, error) {
	return r.rows, nil
}

// pricedHoldingRow は、銘柄 code の、価格（2026-10-06）が保存済みの保有データ1件（集約前）を作る。単価は Tenths。
func pricedHoldingRow(code, name string, quantity, acquisitionTenths, closeTenths int64) dto.HoldingRow {
	return dto.HoldingRow{
		Code:                   code,
		Name:                   name,
		Quantity:               quantity,
		AcquisitionPriceTenths: acquisitionTenths,
		ClosePriceTenths:       new(closeTenths),
		PriceDate:              new("2026-10-06"),
	}
}

// toyotaRow は、7203（トヨタ自動車）の、価格（2026-10-06）が保存済みの保有データ1件（集約前）を作る。単価は Tenths。
func toyotaRow(quantity, acquisitionTenths, closeTenths int64) dto.HoldingRow {
	return pricedHoldingRow("7203", "トヨタ自動車", quantity, acquisitionTenths, closeTenths)
}

// elevenStocksHoldingRows は、プラン 5.1 の (d) の行（PR② ステップ6 の EC-8・TC-07）:
// 銘柄コードの違う11行（1001〜1011）で、それぞれ数量 300000000000000・取得価格 1・終値 30000（単価は Tenths）。
// 各行の計算は収まり、合計評価額だけが11行目であふれる。
func elevenStocksHoldingRows() []dto.HoldingRow {
	stockCodes := []string{"1001", "1002", "1003", "1004", "1005", "1006", "1007", "1008", "1009", "1010", "1011"}
	rows := make([]dto.HoldingRow, 0, len(stockCodes))
	for _, code := range stockCodes {
		rows = append(rows, pricedHoldingRow(code, "テスト銘柄"+code, 300000000000000, 1, 30000))
	}
	return rows
}

// 4.4 API のあふれ（PR② の EC-1）: 計算が int64 に収まらない保有データなら、GET /api/holdings は
// 500 と {"error":"internal_error"}（誤った値を 200 で黙って返さない）。
// (a)(b)(d) の行と (c) の数量はプラン 5.1 の値。(c) の取得価格・終値はプランに指定がないので、最小の単価 0.1円（Tenths で 1）とした。
// (b) は PR② ステップ6 の EC-8・TC-07 で取得価格を 1 に直し（評価額の途中の値 終値×数量だけがあふれる）、
// (d) を足した（合計評価額だけがあふれる）。
func TestPlan4_4_PR2_EC1_EC8_TC07_API_計算がint64に収まらない保有データなら500とinternal_errorを返す(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		rows []dto.HoldingRow
	}{
		{"a_数量200000000000000_取得価格30000_終値30000", []dto.HoldingRow{
			toyotaRow(200000000000000, 30000, 30000),
		}},
		// PR2_EC8_TC07: 数量の合計・2S+Q・2Q は収まり、評価額の途中の値 終値×数量だけがあふれる。
		{"b_PR2_EC8_TC07_数量400000000000000_取得価格1_終値30000", []dto.HoldingRow{
			toyotaRow(400000000000000, 1, 30000),
		}},
		{"c_同じ銘柄の2行がそれぞれ数量5000000000000000000", []dto.HoldingRow{
			toyotaRow(5000000000000000000, 1, 1),
			toyotaRow(5000000000000000000, 1, 1),
		}},
		// PR2_EC8_TC07: 各行の計算は収まり、合計評価額だけが11行目であふれる。
		{"d_PR2_EC8_TC07_銘柄コードの違う11行がそれぞれ数量300000000000000_取得価格1_終値30000", elevenStocksHoldingRows()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHoldingsAPI(fixedRowsHoldingsReader{rows: tc.rows})

			rec := serveGet(t, h, holdingsPath, allowedHost)

			assertJSONResponse(t, rec, http.StatusInternalServerError, `{"error":"internal_error"}`)
		})
	}

	// 対照: 数量 100・取得価格 25000・終値 30000 なら 200 で、要件 AC-1 の行を返す
	// （上の 500 が、fake の行の形などあふれ以外の理由でないことを確かめる）。
	t.Run("対照_数量100_取得価格25000_終値30000は200", func(t *testing.T) {
		t.Parallel()
		h := newHoldingsAPI(fixedRowsHoldingsReader{rows: []dto.HoldingRow{toyotaRow(100, 25000, 30000)}})

		rec := serveGet(t, h, holdingsPath, allowedHost)

		assertJSONResponse(t, rec, http.StatusOK, `{
			"holdings": [
				{"code": "7203", "name": "トヨタ自動車", "quantity": 100,
				 "acquisitionPrice": "2500.0", "currentPrice": "3000.0",
				 "priceDate": "2026-10-06", "valuation": 300000}
			],
			"totalValuation": 300000,
			"totalExcludesUnpriced": false
		}`)
	})
}
