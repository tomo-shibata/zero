package integration

import (
	"net/http"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/infrastructure/pricesource"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: おまかせ運用の要件（docs/requirements/managed-account.md）の要件21 と基準15、プラン（docs/plans/managed-account/plan.md）5.1 の基準15 の行。
// 毎日18:00 の価格の取得（定時の取得）と「価格を今すぐ更新」は、口座と記録を変えない。取得元に頼む銘柄にも口座は含まれない。
// 組み立ては、定時の取得が newFetchClosingPrices（fetch_closing_prices_test.go）、今すぐ更新が newRefreshAPI（price_refresh_api_test.go）。
// 取得元は、ダミー（pricesource.NewDummyPriceSource）を countingPriceSource で包んだもの（頼まれた銘柄を記録する）。
// 「口座の2つの表は前後で同じ」は snapshotManagedTables・assertManagedTablesUnchanged（managed_accounts_fixtures_test.go）。
// 「1銘柄の価格を更新しました」の文言は、今の refreshResultMessages のテスト（保存1件）が確かめているので、ここでは応答（saved・failed）まで。

// givenManagedPoolForPrices は、基準15 の Given の DB を作る。Aは 7203「買 2,500円 100株」（価格は未保存）と
// WealthNavi（記録1件。2026-11-16・評価額 4,000,000円・損益 1,000,000円）、Bは WealthNavi（記録1件）。
func givenManagedPoolForPrices(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-11-16", 4_000_000, 1_000_000)
	insertManagedValuation(t, pool, seedUserB, "WealthNavi", "2026-11-16", 9_000_000, 2_000_000)
	return pool
}

// 基準15: Aが 7203「買 2,500円 100株」（価格未保存）と、記録「WealthNavi 2026-11-16 評価額 4,000,000円 損益 +1,000,000円」を持つ
// → 定時の取得処理をダミーの取得元で実行する → 7203 の価格だけが保存され（stock_prices の 7203 の1行。ダミーは 3,000円・2026-10-06）、
// 取得元に頼まれた銘柄は 7203 だけで、口座の2つの表は前後で同じ。
// 続けて Aが今すぐ更新を実行する → 応答が {"saved":["7203"],"failed":[]}（1銘柄）で、取得元に頼まれたのは 7203 だけ、口座の2つの表は前後で同じ。
// 口座の2つの表が空だと行を消す誤りを見逃すので、Bの WealthNavi（記録1件）も入れておく（要件定義に指定がないので 2026-11-16・9,000,000円・2,000,000円とした）。
func Test基準15_定時の取得と今すぐ更新は口座と記録を変えず取得元にも頼まない(t *testing.T) {
	// 定時の取得のあとに今すぐ更新を順に実行するので、並列にしない。
	pool := givenManagedPoolForPrices(t)
	before := snapshotManagedTables(t, pool)

	t.Run("定時の取得", func(t *testing.T) {
		src := &countingPriceSource{inner: pricesource.NewDummyPriceSource()}

		got, err := newFetchClosingPrices(src, pool).Execute(t.Context())

		if err != nil {
			t.Fatalf("Execute: got err %v, want nil", err)
		}
		if want := []string{"7203"}; !slices.Equal(got.Saved, want) {
			t.Errorf("FetchResult.Saved: got %v, want %v", got.Saved, want)
		}
		if len(got.Failed) != 0 {
			t.Errorf("FetchResult.Failed: got %v, want 空", got.Failed)
		}
		if want := []string{"7203"}; !slices.Equal(src.requested(), want) {
			t.Errorf("取得元に頼まれた銘柄: got %v, want %v（口座は銘柄に数えない）", src.requested(), want)
		}
		assertStockPrices(t, pool, []storedClosePrice{
			{stockCode: "7203", tradingDate: "2026-10-06", priceHundredths: 300000},
		})
		assertManagedTablesUnchanged(t, pool, before)
	})

	t.Run("今すぐ更新", func(t *testing.T) {
		src := &countingPriceSource{inner: pricesource.NewDummyPriceSource()}
		h := newRefreshAPI(pool, seedUserA, src)

		rec := servePost(t, h, priceRefreshPath, sameOriginHeaders())

		assertJSONResponse(t, rec, http.StatusOK, `{"saved":["7203"],"failed":[]}`)
		if want := []string{"7203"}; !slices.Equal(src.requested(), want) {
			t.Errorf("取得元に頼まれた銘柄: got %v, want %v（口座は銘柄に数えない）", src.requested(), want)
		}
		assertStockPrices(t, pool, []storedClosePrice{
			{stockCode: "7203", tradingDate: "2026-10-06", priceHundredths: 300000},
		})
		assertManagedTablesUnchanged(t, pool, before)
	})
}
