package integration

import (
	"net/http"
	"testing"

	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: カード表示の要件（docs/requirements/holdings-cards.md）の基準14・基準15（要件11〜要件16）と、
// プラン（docs/plans/holdings-cards/plan.md）5.1（14・15 の統合の部分: API の totals の値）。
// 画面の部分（合計の欄の中身・順番・位置）は E2E（e2e/tests/holdings-cards.spec.ts）。
// 組み立てと比べ方は holdings_api_test.go（getHoldingsFromDB・assertJSONResponse）と同じ。

// 基準14 の API 部分（要件12・要件13）: Aの売買履歴が 7203「買 2,500円 100株」、6758「買 4,000円 10株」、AAPL「買 150.25ドル 10株」で、
// どれも価格が保存済み（3,000円・3,500円・180.00ドル）
// → totals の円は 含み損益 "45000"（7203 の +50,000円 と 6758 の -5,000円）・率 "15.5"（45,000 ÷ 290,000 × 100 ＝ 15.51…%）・評価額 "335000"、
// ドルは 含み損益 "297.50"・率 "19.8"・評価額 "1800.00"。どちらも excludesUnpriced は false。
// 行は 6758 が -5,000円（-12.5%）、7203 が +50,000円（+20.0%）、AAPL が +297.50ドル（+19.8%）。
func Testカード表示の基準14_API_合計は通貨ごとにカードの含み損益の和と丸める前の値で計算した率を返す(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStock(t, pool, "AAPL", "Apple", currencyUSD)
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	insertBuy(t, pool, seedUserA, "6758", 10, "4000")
	insertBuy(t, pool, seedUserA, "AAPL", 10, "150.25")
	insertClosePrice(t, pool, "7203", "2026-10-06", "3000")
	insertClosePrice(t, pool, "6758", "2026-10-06", "3500")
	insertClosePrice(t, pool, "AAPL", "2026-10-06", "180.00")

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, `{
		"holdings": [
			{"code": "6758", "name": "ソニーグループ", "currency": "JPY", "quantity": 10,
			 "acquisitionPrice": "4000.0", "currentPrice": "3500.0",
			 "priceDate": "2026-10-06", "valuation": "35000",
			 "unrealizedGain": "-5000", "unrealizedGainRate": "-12.5"},
			{"code": "7203", "name": "トヨタ自動車", "currency": "JPY", "quantity": 100,
			 "acquisitionPrice": "2500.0", "currentPrice": "3000.0",
			 "priceDate": "2026-10-06", "valuation": "300000",
			 "unrealizedGain": "50000", "unrealizedGainRate": "20.0"},
			{"code": "AAPL", "name": "Apple", "currency": "USD", "quantity": 10,
			 "acquisitionPrice": "150.25", "currentPrice": "180.00",
			 "priceDate": "2026-10-06", "valuation": "1800.00",
			 "unrealizedGain": "297.50", "unrealizedGainRate": "19.8"}
		],
		"totals": [
			{"currency": "JPY", "valuation": "335000", "excludesUnpriced": false,
			 "unrealizedGain": "45000", "unrealizedGainRate": "15.5"},
			{"currency": "USD", "valuation": "1800.00", "excludesUnpriced": false,
			 "unrealizedGain": "297.50", "unrealizedGainRate": "19.8"}
		],
		"combinedTotal": null, "managedAccounts": []
	}`)
}

// 基準15 の API 部分（要件15・要件16）: Aの売買履歴が 7203「買 2,500円 100株」（価格保存済み）、9984「買 4,500円 100株」（価格なし）、
// MSFT「買 400.00ドル 5株」（価格なし）
// → totals の円は 含み損益 "50000"・率 "20.0"（価格のない 9984 を入れない）・評価額 "300000"・excludesUnpriced true、
// ドルは 含み損益 null・率 null・評価額 "0.00"・excludesUnpriced true。価格のない行の含み損益と率も null。
func Testカード表示の基準15_API_価格のない銘柄は合計の含み損益に入れずドルがすべて価格なしなら含み損益はnullになる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStock(t, pool, "MSFT", "Microsoft", currencyUSD)
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	insertBuy(t, pool, seedUserA, "9984", 100, "4500")
	insertBuy(t, pool, seedUserA, "MSFT", 5, "400.00")
	insertClosePrice(t, pool, "7203", "2026-10-06", "3000")

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, `{
		"holdings": [
			{"code": "7203", "name": "トヨタ自動車", "currency": "JPY", "quantity": 100,
			 "acquisitionPrice": "2500.0", "currentPrice": "3000.0",
			 "priceDate": "2026-10-06", "valuation": "300000",
			 "unrealizedGain": "50000", "unrealizedGainRate": "20.0"},
			{"code": "9984", "name": "ソフトバンクグループ", "currency": "JPY", "quantity": 100,
			 "acquisitionPrice": "4500.0", "currentPrice": null,
			 "priceDate": null, "valuation": null,
			 "unrealizedGain": null, "unrealizedGainRate": null},
			{"code": "MSFT", "name": "Microsoft", "currency": "USD", "quantity": 5,
			 "acquisitionPrice": "400.00", "currentPrice": null,
			 "priceDate": null, "valuation": null,
			 "unrealizedGain": null, "unrealizedGainRate": null}
		],
		"totals": [
			{"currency": "JPY", "valuation": "300000", "excludesUnpriced": true,
			 "unrealizedGain": "50000", "unrealizedGainRate": "20.0"},
			{"currency": "USD", "valuation": "0.00", "excludesUnpriced": true,
			 "unrealizedGain": null, "unrealizedGainRate": null}
		],
		"combinedTotal": null, "managedAccounts": []
	}`)
}
