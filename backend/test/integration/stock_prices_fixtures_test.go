package integration

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// 株価取得の統合テスト（PR③）が使う、保有データを入れるヘルパーと、stock_prices を読んで確かめるヘルパー。
// プラン（docs/plans/stock-holdings/plan.md）4.4「テストデータ」: 保有データ・価格は seed に入れず、各テストで入れる
// （入れるのは holdings_fixtures_test.go の insertHolding・insertClosePrice）。
// stock_prices の列はプラン 4.2 のとおり（stock_code・trading_date・close_price・fetched_at）。

// storedClosePrice は、stock_prices の1行のうち、テストで確かめる値。
// 終値は 0.1円単位の整数で持つ（プラン 4.1。例: 30000 は 3,000.0円）。
type storedClosePrice struct {
	stockCode   string
	tradingDate string // YYYY-MM-DD
	priceTenths int64
}

// readStockPrices は、stock_prices のすべての行を、銘柄コード・取引日の昇順で返す。
// 終値は close_price を10倍して整数にした値（0.1円単位）にする。close_price は CHECK で小数第1位までに限られる
// （プラン 4.2）ので、10倍した値は整数になり、bigint への変換で丸めは起きない。
// 取引日は DateStyle の設定に左右されないよう、to_char で YYYY-MM-DD にする。
func readStockPrices(t *testing.T, pool *pgxpool.Pool) []storedClosePrice {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT stock_code, to_char(trading_date, 'YYYY-MM-DD'), (close_price * 10)::bigint
		   FROM stock_prices
		  ORDER BY stock_code, trading_date`)
	if err != nil {
		t.Fatalf("stock_prices を読めない: %v", err)
	}
	prices, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (storedClosePrice, error) {
		var p storedClosePrice
		err := row.Scan(&p.stockCode, &p.tradingDate, &p.priceTenths)
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
		t.Errorf("stock_prices の行（銘柄コード・取引日・0.1円単位の終値）:\n got %+v\nwant %+v", got, want)
	}
}

// insertAC13Holdings は、要件 AC-13 の Given「7203 と 6758 が保有されていて、価格は未保存」の保有データを入れる。
// プラン 5.1 の AC-13 の行: A が 7203 を2件（数量違い）、B が 6758 を保有。
// 数量・取得価格はプランに指定がないので、要件の AC-4（7203 を100株・2,000円と300株・3,000円）と、
// PR② のテストで使った利用者Bの 6758（10株・3,200円）の値にした（取得の結果には関係しない）。
// AC-14b（prices_fetch_task_test.go）も「AC-13 と同じ」データとしてこれを使う。
func insertAC13Holdings(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	insertHolding(t, pool, seedUserA, "7203", 100, "2000")
	insertHolding(t, pool, seedUserA, "7203", 300, "3000")
	insertHolding(t, pool, seedUserB, "6758", 10, "3200")
}

// ac13WantPrices は、要件 AC-13 の Then「7203 は3,000円・2026-10-06、6758 は3,500円・2026-10-06 で保存される」を、
// stock_prices の行（銘柄コード・取引日の昇順。各1行）にしたもの。値は要件「成功基準」のダミー価格の表と、
// プラン 4.4 の pricesource.NewDummyPriceSource（7203=30000・6758=35000、どちらも 2026-10-06）のとおり。
func ac13WantPrices() []storedClosePrice {
	return []storedClosePrice{
		{stockCode: "6758", tradingDate: "2026-10-06", priceTenths: 35000},
		{stockCode: "7203", tradingDate: "2026-10-06", priceTenths: 30000},
	}
}
