package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// 仕様: プラン（docs/plans/trade-history/plan.md）4.3 の「保有の計算」の 6（最後の値が int64 に収まらなければエラー。判断5）と、
// 5.2（`holdings_api_overflow_test.go`・`calculation_range` のテストは、`ErrCalculationOverflow` を売買履歴で起こすテストに直す）、
// 5.3（例は3つ: 終値 3,000円で 10^18 株、5×10^18 株の買い2件、行ごとには収まり合計評価額だけがあふれる2銘柄）。
// PR②（4.6）で、金額は 0.01 単位（Hundredths）、通貨は円（JPY）。int64 に収まるかは 0.01 単位の値で見る。
// 元の保有株式一覧のプラン 4.4「API」の「計算が int64 に収まらない保有データのときは、API は 500 と {"error":"internal_error"}」と同じ形で、
// GET /api/holdings は 500 と {"error":"internal_error"}（誤った値を 200 で黙って返さない）。
// 売買履歴は、境界の外側の HoldingsReader の fake から渡す（int64 の境界の数量は、DB に入れる手間をかけずに渡せるため）。
// 単体の部分は internal/application/query/calculate_holdings_test.go・list_holdings_test.go。

// fixedRowsHoldingsReader は、どの利用者についても決まった売買履歴の行を返す HoldingsReader（境界の外側の fake）。
type fixedRowsHoldingsReader struct {
	rows []dto.TradeRow
}

func (r fixedRowsHoldingsReader) ListTradeRows(context.Context, uuid.UUID) ([]dto.TradeRow, error) {
	return r.rows, nil
}

// ListStockSplits は空を返す（株式分割のプラン 4.8 PR①。この fake は分割を使わない）。
func (r fixedRowsHoldingsReader) ListStockSplits(context.Context, uuid.UUID) ([]dto.StockSplitRow, error) {
	return []dto.StockSplitRow{}, nil
}

// ListHoldingOrder は空の並びを返す（カード表示のプラン 4.6 PR②。この fake は並びを使わない）。
func (r fixedRowsHoldingsReader) ListHoldingOrder(context.Context, uuid.UUID) ([]string, error) {
	return []string{}, nil
}

// LatestUsdJpyRate はレートなし（nil, nil）を返す（ドルを円にまとめるプラン 4.6 PR①。この fake はレートを使わない）。
func (r fixedRowsHoldingsReader) LatestUsdJpyRate(context.Context) (*dto.UsdJpyRate, error) {
	return nil, nil
}

// buyRow は、円の銘柄 code・name の買い1件（登録した順は id。取引日 2026-01-10）の売買履歴の行を作る。
// 銘柄の最新の終値は closeHundredths（0.01 単位。2026-10-06）。価格（1株あたり）は 0.1円（Hundredths で 10。円の最小の単位）。
func buyRow(id int64, code, name string, quantity, closeHundredths int64) dto.TradeRow {
	return dto.TradeRow{
		ID:                   id,
		Code:                 code,
		Name:                 name,
		Currency:             dto.CurrencyJPY,
		Side:                 dto.TradeSideBuy,
		Quantity:             quantity,
		PriceHundredths:      10,
		TradeDate:            "2026-01-10",
		ClosePriceHundredths: &closeHundredths,
		PriceDate:            new("2026-10-06"),
	}
}

// 4.3 の 6・5.3: 最後の値が int64 に収まらない売買履歴なら、GET /api/holdings は 500 と {"error":"internal_error"}。
//
//	1: 終値 3,000円で 10^18 株（評価額があふれる）
//	2: 5×10^18 株の買い2件（保有数量があふれる）
//	3: 行ごとには収まり、合計評価額だけがあふれる2銘柄（どちらも終値 0.1円・5×10^17 株で評価額 5×10^18（0.01 単位）。合計 10^19）
func TestPR1_4_3_API_計算がint64に収まらない売買履歴なら500とinternal_errorを返す(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		rows []dto.TradeRow
	}{
		{"1_終値3000円で10の18乗株は評価額があふれる", []dto.TradeRow{
			buyRow(1, "7203", "トヨタ自動車", 1000000000000000000, 300000),
		}},
		{"2_5かける10の18乗株の買い2件は保有数量があふれる", []dto.TradeRow{
			buyRow(1, "7203", "トヨタ自動車", 5000000000000000000, 300000),
			buyRow(2, "7203", "トヨタ自動車", 5000000000000000000, 300000),
		}},
		{"3_行ごとには収まり合計評価額だけがあふれる2銘柄", []dto.TradeRow{
			buyRow(1, "7203", "トヨタ自動車", 500000000000000000, 10),
			buyRow(2, "6758", "ソニーグループ", 500000000000000000, 10),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHoldingsAPI(fixedRowsHoldingsReader{rows: tc.rows})

			rec := serveGet(t, h, holdingsPath, allowedHost)

			assertJSONResponse(t, rec, http.StatusInternalServerError, `{"error":"internal_error"}`)
		})
	}

	// 対照: 100株・買 2,500円・終値 3,000円なら 200 で、要件の基準1・保有株式一覧の AC-1 と同じ行を返す
	// （上の 500 が、fake の行の形などあふれ以外の理由でないことを確かめる）。
	t.Run("対照_100株_終値3000円は200", func(t *testing.T) {
		t.Parallel()
		row := buyRow(1, "7203", "トヨタ自動車", 100, 300000)
		row.PriceHundredths = 250000
		h := newHoldingsAPI(fixedRowsHoldingsReader{rows: []dto.TradeRow{row}})

		rec := serveGet(t, h, holdingsPath, allowedHost)

		assertJSONResponse(t, rec, http.StatusOK, `{
			"holdings": [
				{"code": "7203", "name": "トヨタ自動車", "currency": "JPY", "quantity": 100,
				 "acquisitionPrice": "2500.0", "currentPrice": "3000.0",
				 "priceDate": "2026-10-06", "valuation": "300000",
				 "unrealizedGain": "50000", "unrealizedGainRate": "20.0"}
			],
			"totals": [{"currency": "JPY", "valuation": "300000", "excludesUnpriced": false,
			            "unrealizedGain": "50000", "unrealizedGainRate": "20.0"}],
			"combinedTotal": null
		}`)
	})
}
