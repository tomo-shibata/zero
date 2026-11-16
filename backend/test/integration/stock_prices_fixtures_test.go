package integration

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// 株価取得の統合テスト（PR③）が使う、売買履歴を入れるヘルパーと、stock_prices を読んで確かめるヘルパー。
// プラン（docs/plans/stock-holdings/plan.md）4.4「テストデータ」: 売買履歴・価格は seed に入れず、各テストで入れる
// （入れるのは holdings_fixtures_test.go の insertTrade・insertBuy・insertClosePrice。売買履歴への作り変え後の形）。
// stock_prices の列はプラン 4.2 のとおり（stock_code・trading_date・close_price・fetched_at）。

// storedClosePrice は、stock_prices の1行のうち、テストで確かめる値。
// 終値は 0.01 単位の整数で持つ（売買履歴のプラン 4.3。例: 300000 は 3,000円、18025 は 180.25ドル）。
type storedClosePrice struct {
	stockCode       string
	tradingDate     string // YYYY-MM-DD
	priceHundredths int64
}

// readStockPrices は、stock_prices のすべての行を、銘柄コード・取引日の昇順で返す。
// 終値は close_price を100倍して整数にした値（0.01 単位）にする。close_price は CHECK（00006 で小数第2位まで）で
// 小数第2位までに限られる（売買履歴のプラン 4.2）ので、100倍した値は整数になり、bigint への変換で丸めは起きない。
// 取引日は DateStyle の設定に左右されないよう、to_char で YYYY-MM-DD にする。
func readStockPrices(t *testing.T, pool *pgxpool.Pool) []storedClosePrice {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT stock_code, to_char(trading_date, 'YYYY-MM-DD'), (close_price * 100)::bigint
		   FROM stock_prices
		  ORDER BY stock_code, trading_date`)
	if err != nil {
		t.Fatalf("stock_prices を読めない: %v", err)
	}
	prices, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (storedClosePrice, error) {
		var p storedClosePrice
		err := row.Scan(&p.stockCode, &p.tradingDate, &p.priceHundredths)
		return p, err
	})
	if err != nil {
		t.Fatalf("stock_prices を読めない: %v", err)
	}
	return prices
}

// assertStockPrices は、stock_prices の行が want（銘柄コード・取引日の昇順）とちょうど同じであることを確かめる。
// 行が多すぎる（同じ銘柄・同じ取引日が2行になった、取得できない銘柄の行が増えた）ことも、足りないことも、値の違いも失敗にする。
func assertStockPrices(t *testing.T, pool *pgxpool.Pool, want []storedClosePrice) {
	t.Helper()
	if got := readStockPrices(t, pool); !slices.Equal(got, want) {
		t.Errorf("stock_prices の行（銘柄コード・取引日・0.01 単位の終値）:\n got %+v\nwant %+v", got, want)
	}
}

// insertAC13Holdings は、要件 AC-13 の Given「7203 と 6758 が保有されていて、価格は未保存」の売買履歴（買い）を入れる。
// プラン 5.1 の AC-13 の行: A が 7203 を2件（数量違い）、B が 6758 を保有。
// 数量・価格はプランに指定がないので、元の AC-4 の値（7203 を100株・2,000円と300株・3,000円）と、
// 利用者Bの 6758（10株・3,200円）にした（取得の結果には関係しない）。
// AC-14b（prices_fetch_task_test.go）も「AC-13 と同じ」データとしてこれを使う。
func insertAC13Holdings(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	insertBuy(t, pool, seedUserA, "7203", 100, "2000")
	insertBuy(t, pool, seedUserA, "7203", 300, "3000")
	insertBuy(t, pool, seedUserB, "6758", 10, "3200")
}

// ac13WantPrices は、要件 AC-13 の Then「7203 は3,000円・2026-10-06、6758 は3,500円・2026-10-06 で保存される」を、
// stock_prices の行（銘柄コード・取引日の昇順。各1行）にしたもの。値は要件「成功基準」のダミー価格の表と、
// 売買履歴のプラン 4.6（PR②）の pricesource.NewDummyPriceSource（7203=300000・6758=350000・AAPL=18000（0.01 単位）、どれも 2026-10-06）のとおり。
func ac13WantPrices() []storedClosePrice {
	return []storedClosePrice{
		{stockCode: "6758", tradingDate: "2026-10-06", priceHundredths: 350000},
		{stockCode: "7203", tradingDate: "2026-10-06", priceHundredths: 300000},
	}
}

// insertTradeHistoryGiven は、売買履歴の要件定義の基準16・基準17 の Given の売買履歴を入れる。
// Aが 7203「買 2,500円 100株」と 6758「買 3,200円 10株」「売 3,500円 10株」（6758 は売り切り）、Bが AAPL「買 150.25ドル 10株」を持つ。
// どれも価格は未保存。AAPL は test.sql にない銘柄なので、銘柄マスタにドル（USD）の銘柄として入れる（プラン 4.7: AAPL・MSFT・GOOGL は各テストで入れる）。
// 取引日は、買いが buyDate、6758 の売りが 2026-02-10。
func insertTradeHistoryGiven(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	insertStock(t, pool, "AAPL", "Apple", currencyUSD)
	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "2500", buyDate)
	insertTrade(t, pool, seedUserA, "6758", "buy", 10, "3200", buyDate)
	insertTrade(t, pool, seedUserA, "6758", "sell", 10, "3500", "2026-02-10")
	insertTrade(t, pool, seedUserB, "AAPL", "buy", 10, "150.25", buyDate)
}
