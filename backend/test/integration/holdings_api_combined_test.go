package integration

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: ドルを円にまとめる合計の要件（docs/requirements/fx-total.md）の基準1・基準4・基準13（要件2・要件6〜要件8・要件10・要件13・要件14）と、
// プラン（docs/plans/fx-total/plan.md）4.4（API の形）・4.7（PR① の境界）・5.1（PR① の行）・5.2（holdings_api_combined_test.go の行）。
// GET /api/holdings の本文の combinedTotal（キーは常にあり、値は null かオブジェクト）を確かめる。
// 組み立てと比べ方は holdings_api_test.go（getHoldingsFromDB・assertJSONResponse）と同じ。レートは SQL で直接入れる（要件15）。
//
// 売買履歴は要件定義の基準1 と同じ: Aの売買履歴が 7203「買 2,500円 100株」、AAPL「買 150.25ドル 10株」で、どちらも価格が保存済み（3,000円・180.00ドル）。
// （ドルの保有がないと combinedTotal が null になるので、基準13 などでもこの売買履歴を使う。）

// insertUsdJpyRate は、日付 rateDate（YYYY-MM-DD）のドル円のレート rate（例 "150"、"150.00"）を入れる。
// 書いた桁（scale）のまま保存させるため、text から変換する。呼んだ順が入れた順になる。
func insertUsdJpyRate(t *testing.T, pool *pgxpool.Pool, rateDate, rate string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO usd_jpy_rates (rate_date, rate) VALUES ($1::text::date, $2::text::numeric)",
		rateDate, rate,
	); err != nil {
		t.Fatalf("レート（%s・%s円）を入れられない: %v", rateDate, rate, err)
	}
}

// seedBasis1Holdings は、要件定義の基準1 の売買履歴と価格を入れる。
func seedBasis1Holdings(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	insertStock(t, pool, "AAPL", "Apple", currencyUSD)
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	insertBuy(t, pool, seedUserA, "AAPL", 10, "150.25")
	insertClosePrice(t, pool, "7203", "2026-10-06", "3000")
	insertClosePrice(t, pool, "AAPL", "2026-10-06", "180.00")
}

// basis1ResponseJSON は、基準1 の売買履歴に対する GET /api/holdings の本文で、combinedTotal だけを引数にしたもの。
// holdings と totals は今のとおり（カード表示の基準14 の 7203・AAPL の行と、通貨ごとの合計）。
func basis1ResponseJSON(combinedTotal string) string {
	return `{
		"holdings": [
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
			{"currency": "JPY", "valuation": "300000", "excludesUnpriced": false,
			 "unrealizedGain": "50000", "unrealizedGainRate": "20.0"},
			{"currency": "USD", "valuation": "1800.00", "excludesUnpriced": false,
			 "unrealizedGain": "297.50", "unrealizedGainRate": "19.8"}
		],
		"combinedTotal": ` + combinedTotal + `,
		"managedAccounts": []
	}`
}

// 円にまとめた合計が出る基準1 の値: レート 2026-11-14 150.00円。
// 評価額 570,000円・含み損益 +94,625円・率 +19.9%（300,000 ＋ 1,800.00 × 150、50,000 ＋ 297.50 × 150、94,625 ÷ (250,000 ＋ 1,502.50 × 150)）。
const basis1CombinedJSON = `{"valuation": "570000", "excludesUnpriced": false,
	"unrealizedGain": "94625", "unrealizedGainRate": "19.9",
	"usdJpyRate": "150.00", "usdJpyRateDate": "2026-11-14"}`

// 基準1 の API 部分（要件6〜要件8・要件10・要件13）: 基準1 の売買履歴で、レートを SQL で小数なしの 150 と入れる（2026-11-14）
// → combinedTotal は {"valuation":"570000","excludesUnpriced":false,"unrealizedGain":"94625","unrealizedGainRate":"19.9",
// "usdJpyRate":"150.00","usdJpyRateDate":"2026-11-14"}。totals は今と同じ。
// 小数なしの 150 でも 0.01円単位（15000）に読み、"150.00" で返す（DB の桁数によらない）。
func Test基準1_API_円にまとめた合計は円の値とレートを文字列で返す(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	seedBasis1Holdings(t, pool)
	insertUsdJpyRate(t, pool, "2026-11-14", "150")

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, basis1ResponseJSON(basis1CombinedJSON))
}

// 基準4 の API 部分（要件2）: 基準1 の売買履歴で、DB にレートがない → "combinedTotal": null（holdings と totals は今と同じ）。
func Test基準4_API_レートがなければcombinedTotalはnullになる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	seedBasis1Holdings(t, pool)

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, basis1ResponseJSON("null"))
}

// 基準13（要件13・要件14）: 基準1 の売買履歴で、レートを 2026-11-14 150.00円、2026-11-13 149.00円の順に入れる
// → 使うレートは日付がいちばん新しい 2026-11-14 の 150.00円（usdJpyRate "150.00"・usdJpyRateDate "2026-11-14"）。
// 入れた順の最後を使う誤りを見分けるため、新しい日付を先に入れる。値は基準1 と同じ。
func Test基準13_API_使うレートは日付がいちばん新しいもの(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	seedBasis1Holdings(t, pool)
	insertUsdJpyRate(t, pool, "2026-11-14", "150.00")
	insertUsdJpyRate(t, pool, "2026-11-13", "149.00")

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, basis1ResponseJSON(basis1CombinedJSON))
}

// 補助（要件14。プラン 5.2）: 基準1 の売買履歴で、レートを 2026-11-13 151.00円 → 2026-11-14 150.00円 → 2026-11-12 149.00円の順に入れる
// → usdJpyRate が "150.00"、usdJpyRateDate が "2026-11-14"。基準13 のデータでは、並びを決めずに1件読む・最初に入れたものを使う・
// レートの高い順に読む誤りが通る。この並びなら、最初・最後に入れたもの、最も高い・低いレートのどれとも違う。値は基準1 と同じ。
func Test補助_API_使うレートは入れた順でもレートの大小でもなく日付がいちばん新しいもの(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	seedBasis1Holdings(t, pool)
	insertUsdJpyRate(t, pool, "2026-11-13", "151.00")
	insertUsdJpyRate(t, pool, "2026-11-14", "150.00")
	insertUsdJpyRate(t, pool, "2026-11-12", "149.00")

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, basis1ResponseJSON(basis1CombinedJSON))
}

// rateFailingHoldingsReader は、売買履歴・分割・並びは空で、レートを読むと必ず失敗する HoldingsReader（境界の外側の fake）。
type rateFailingHoldingsReader struct {
	emptyHoldingsReader
}

// rateFailingDetail は、fake が返すエラーの内容。応答に出てはいけない（今の一覧の失敗と同じ扱い）。
const rateFailingDetail = "fake: connection refused to db.internal:5432 (usd_jpy_rates)"

func (rateFailingHoldingsReader) LatestUsdJpyRate(context.Context) (*dto.UsdJpyRate, error) {
	return nil, errors.New(rateFailingDetail)
}

// 補助（プラン 5.2）: LatestUsdJpyRate が失敗する HoldingsReader を注入した router.New に GET /api/holdings
// → 500 と {"error":"internal_error"}（今の一覧の失敗と同じ扱い。内部のエラー内容は返さない）。
func Test補助_API_レートを読めないときは500とinternal_errorを返す(t *testing.T) {
	t.Parallel()
	h := newHoldingsAPI(rateFailingHoldingsReader{})

	rec := serveGet(t, h, holdingsPath, allowedHost)

	assertJSONResponse(t, rec, http.StatusInternalServerError, `{"error":"internal_error"}`)
}
