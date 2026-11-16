package integration

import (
	"slices"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 要件（docs/requirements/stock-holdings.md）の FR-15（保有されている全銘柄の終値を取得元から取得し、取引日とともに保存する）と、
// 売買履歴の要件定義（docs/requirements/trade-history.md）の要件5（終値は円が小数第1位まで、ドルが小数第2位まで）と、
// プラン（docs/plans/stock-holdings/plan.md）4.2（stock_prices.close_price は NUMERIC）、
// 売買履歴のプラン（docs/plans/trade-history/plan.md）4.3・5.5（PR②。終値は 0.01 単位の整数。0.1円の端数のテストは、
// 0.01 の端数（ドル）と、円の 0.01 の端数が Failed になること（基準15）に分ける）。
// 背景は docs/plans/stock-holdings/reviews/pr3-step6.md の EC-P2-7（端数を DB に保存するケースがない）。
// 組み立ては fetch_closing_prices_test.go の newFetchClosingPrices（4.4 の境界のとおり）、取得元は同じファイルの stubPriceSource。
// 円の 0.01 の端数が Failed になること（基準15）は fetch_closing_prices_test.go。

// FR-15・4.2（PR③ の EC-P2-7）: 取得元が 0.1円の端数のある円の終値（123450 = 1,234.5円）を返す → stock_prices に 1,234.5円 のまま保存される
// （1,234円や 12,345円に変わらない）。
// 保有データ（A が 7203 を 100株・2,500円）はプランに指定がないので、取得の結果に関係しない値にした。
func TestFR15_PR3_EC_P2_7_0_1円の端数のある終値はそのままDBに保存される(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	src := stubPriceSource{
		t:      t,
		prices: map[string]model.ClosingPrice{"7203": mustClosingPrice(t, "7203", "2026-10-06", 123450)},
	}

	got, err := newFetchClosingPrices(src, pool).Execute(t.Context())

	if err != nil {
		t.Fatalf("Execute: got err %v, want nil", err)
	}
	if len(got.Failed) != 0 {
		t.Errorf("FetchResult.Failed: got %v, want 空（1,234.5円は円の精度（小数第1位まで）に合う）", got.Failed)
	}
	if want := []string{"7203"}; !slices.Equal(got.Saved, want) {
		t.Errorf("FetchResult.Saved: got %v, want %v", got.Saved, want)
	}
	// 1,234.5円（0.01 単位で 123450）。readStockPrices は close_price を100倍した整数で読む。
	assertStockPrices(t, pool, []storedClosePrice{
		{stockCode: "7203", tradingDate: "2026-10-06", priceHundredths: 123450},
	})
}

// 要件5・FR-15（PR②）: 取得元がドルの 0.01 の端数のある終値（15025 = 150.25ドル）と、0.01 ドルの最小の終値（1）を返す
// → stock_prices に 150.25 と 0.01 のまま保存される（切り捨てたり丸めたりしない）。
// Aが AAPL・MSFT（どちらもドル。銘柄マスタにドルで入れる）を保有する。
func TestRequirement5_0_01ドルの端数のある終値はそのままDBに保存される(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStock(t, pool, "AAPL", "Apple", currencyUSD)
	insertStock(t, pool, "MSFT", "Microsoft", currencyUSD)
	insertBuy(t, pool, seedUserA, "AAPL", 10, "150.25")
	insertBuy(t, pool, seedUserA, "MSFT", 5, "400.00")
	src := stubPriceSource{
		t: t,
		prices: map[string]model.ClosingPrice{
			"AAPL": mustClosingPrice(t, "AAPL", "2026-10-06", 15025),
			"MSFT": mustClosingPrice(t, "MSFT", "2026-10-06", 1),
		},
	}

	got, err := newFetchClosingPrices(src, pool).Execute(t.Context())

	if err != nil {
		t.Fatalf("Execute: got err %v, want nil", err)
	}
	if len(got.Failed) != 0 {
		t.Errorf("FetchResult.Failed: got %v, want 空（ドルは 0.01ドルまでの終値を保存できる）", got.Failed)
	}
	if want := []string{"AAPL", "MSFT"}; !slices.Equal(got.Saved, want) {
		t.Errorf("FetchResult.Saved: got %v, want %v", got.Saved, want)
	}
	assertStockPrices(t, pool, []storedClosePrice{
		{stockCode: "AAPL", tradingDate: "2026-10-06", priceHundredths: 15025},
		{stockCode: "MSFT", tradingDate: "2026-10-06", priceHundredths: 1},
	})
}
