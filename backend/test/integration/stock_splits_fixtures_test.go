package integration

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// 株式分割の統合テスト（プラン docs/plans/stock-split/plan.md）が使う補助。
// 5章の冒頭: 分割は SQL で入れる（insertStockSplit）。「何も変わらない」は、stock_splits（作成日時を含む）・trades・stocks・stock_prices・
// holding_orders の5つの表の前後を比べる（snapshotFiveTables・assertFiveTablesUnchanged）。
// 今の snapshotImportDB・assertDBUnchanged は trades と stocks しか比べないので、株式分割のテストはこちらを使う。
// 前後を比べるテストは、前提に分割・売買履歴・終値・並びを少なくとも1件ずつ入れておく（空の表どうしを比べると行を消す誤りを見逃すため）。

// insertStockSplit は、stock_splits に分割1件（銘柄 code の「分割 1→ratio 分割の日 splitDate」）を SQL で入れる。
// splitDate は "YYYY-MM-DD"。銘柄マスタにない銘柄コードも入れられる（外部キーなし。要件11）。
func insertStockSplit(t *testing.T, pool *pgxpool.Pool, code, splitDate string, ratio int64) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO stock_splits (stock_code, split_date, ratio) VALUES ($1::text, $2::text::date, $3::integer)",
		code, splitDate, ratio,
	); err != nil {
		t.Fatalf("分割（%s・%s・1→%d）を入れられない: %v", code, splitDate, ratio, err)
	}
}

// fiveTableSnapshot は、stock_splits・trades・stocks・stock_prices・holding_orders の中身を、行ごとの JSON にして並べた文字列。
// 列の値を漏れなく比べるため（stock_splits の作成日時 created_at、trades の id・created_at なども含む）、各表を to_jsonb で読む。
type fiveTableSnapshot struct {
	stockSplits   string
	trades        string
	stocks        string
	stockPrices   string
	holdingOrders string
}

// snapshotFiveTables は、5つの表のすべての行を、決まった並びで読む。
func snapshotFiveTables(t *testing.T, pool *pgxpool.Pool) fiveTableSnapshot {
	t.Helper()
	read := func(table, orderBy string) string {
		t.Helper()
		var s string
		query := "SELECT coalesce(jsonb_agg(to_jsonb(x) ORDER BY " + orderBy + "), '[]'::jsonb)::text FROM " + table + " x"
		if err := pool.QueryRow(t.Context(), query).Scan(&s); err != nil {
			t.Fatalf("%s を読めない: %v", table, err)
		}
		return s
	}
	return fiveTableSnapshot{
		stockSplits:   read("stock_splits", `x.stock_code COLLATE "C", x.split_date`),
		trades:        read("trades", "x.id"),
		stocks:        read("stocks", `x.code COLLATE "C"`),
		stockPrices:   read("stock_prices", `x.stock_code COLLATE "C", x.trading_date`),
		holdingOrders: read("holding_orders", "x.user_id, x.position"),
	}
}

// assertFiveTablesUnchanged は、5つの表（stock_splits・trades・stocks・stock_prices・holding_orders）が before から何も変わっていない
// （行も id も作成日時も値も）ことを確かめる。
func assertFiveTablesUnchanged(t *testing.T, pool *pgxpool.Pool, before fiveTableSnapshot) {
	t.Helper()
	after := snapshotFiveTables(t, pool)
	check := func(name, b, a string) {
		t.Helper()
		if a != b {
			t.Errorf("%s が変わった（取り込めなかったときは何も変えない）:\n before %s\n after  %s", name, b, a)
		}
	}
	check("stock_splits", before.stockSplits, after.stockSplits)
	check("trades", before.trades, after.trades)
	check("stocks", before.stocks, after.stocks)
	check("stock_prices", before.stockPrices, after.stockPrices)
	check("holding_orders", before.holdingOrders, after.holdingOrders)
}
