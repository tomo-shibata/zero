package integration

import (
	"slices"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 要件（docs/requirements/stock-holdings.md）の FR-15（保有されている全銘柄の終値を取得元から取得し、取引日とともに保存する）と、
// プラン（docs/plans/stock-holdings/plan.md）4.1（単価は 0.1円単位の整数で扱う）・
// 4.2（stock_prices.close_price は NUMERIC で、小数第1位まで）、5.1「補助のテスト（バックエンド）」の
// FR-15・4.2（PR③ の EC-P2-7）の行
// （テスト用の取得元が 7203 で 12345・2026-10-06 を返す → stock_prices の 7203 は 1,234.5円（0.1円単位で 12345））。
// 背景は docs/plans/stock-holdings/reviews/pr3-step6.md の EC-P2-7（0.1円の端数を DB に保存するケースがない）。
// 組み立ては fetch_closing_prices_test.go の newFetchClosingPrices（4.4 の境界のとおり）、取得元は同じファイルの stubPriceSource。

// FR-15・4.2（PR③ の EC-P2-7）: 取得元が 0.1円の端数のある終値（12345 = 1,234.5円）を返す → stock_prices に 1,234.5円 のまま保存される
// （1,234円や 12,345円に変わらない）。
// 保有データ（A が 7203 を 100株・2,500円）はプランに指定がないので、取得の結果に関係しない値にした。
func TestFR15_PR3_EC_P2_7_0_1円の端数のある終値はそのままDBに保存される(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertHolding(t, pool, seedUserA, "7203", 100, "2500")
	src := stubPriceSource{
		t:      t,
		prices: map[string]model.ClosingPrice{"7203": mustClosingPrice(t, "7203", "2026-10-06", 12345)},
	}

	got, err := newFetchClosingPrices(src, pool).Execute(t.Context())

	if err != nil {
		t.Fatalf("Execute: got err %v, want nil", err)
	}
	if len(got.Failed) != 0 {
		t.Errorf("FetchResult.Failed: got %v, want 空（1,234.5円は 0.1円単位の正しい終値）", got.Failed)
	}
	if want := []string{"7203"}; !slices.Equal(got.Saved, want) {
		t.Errorf("FetchResult.Saved: got %v, want %v", got.Saved, want)
	}
	// 1,234.5円（0.1円単位で 12345）。readStockPrices は close_price を10倍した整数で読む。
	assertStockPrices(t, pool, []storedClosePrice{
		{stockCode: "7203", tradingDate: "2026-10-06", priceTenths: 12345},
	})
}
