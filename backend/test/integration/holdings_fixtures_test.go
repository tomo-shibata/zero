package integration

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// 保有株式一覧の統合テストが使う、売買履歴と価格を入れるヘルパー。
// 売買履歴への作り変え（docs/plans/trade-history/plan.md 4.6・5章）で、保有データ（holdings）を入れる insertHolding を、
// 売買履歴（trades）を入れる insertTrade に替えた。「Aが 7203 を保有」のような前提は、売買履歴（買い）を入れて作る。
// trades は user_id・stock_code・side・quantity・price・trade_date を入れ（id・created_at は DEFAULT に任せる）、
// stock_prices は stock_code・trading_date・close_price を入れる。
//
// 価格（price・close_price）と取引日は、SQL に書く値のまま文字列で渡し、PostgreSQL の text から変換させる。
// 桁数を指定しない NUMERIC は書いた桁（scale）のまま保存されるので、
// "2000"（scale 0）と "2001.0"（scale 1）のような違いを、そのまま DB に入れられるようにするため。

// buyDate は、前提を作る買いの取引日。どれも今日（2026-11-16）より前の日付にする。
const buyDate = "2026-01-10"

// insertTrade は、利用者 userID の売買履歴1件を入れる。side は "buy" か "sell"、price は1株あたりの円（例 "2500"、"2001.0"）、
// tradeDate は "YYYY-MM-DD"。呼んだ順が登録した順（trades.id の昇順）になる。
func insertTrade(t *testing.T, pool *pgxpool.Pool, userID, stockCode, side string, quantity int64, price, tradeDate string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO trades (user_id, stock_code, side, quantity, price, trade_date) VALUES ($1::uuid, $2::text, $3::text, $4::bigint, $5::text::numeric, $6::text::date)",
		userID, stockCode, side, quantity, price, tradeDate,
	); err != nil {
		t.Fatalf("売買履歴（利用者 %s・%s・%s・%d株・%s円・%s）を入れられない: %v", userID, stockCode, side, quantity, price, tradeDate, err)
	}
}

// insertBuy は、利用者 userID の買い1件（取引日は buyDate）を入れる。保有を作るための前提に使う。
func insertBuy(t *testing.T, pool *pgxpool.Pool, userID, stockCode string, quantity int64, price string) {
	t.Helper()
	insertTrade(t, pool, userID, stockCode, "buy", quantity, price, buyDate)
}

// insertClosePrice は、銘柄 stockCode の取引日 tradingDate（YYYY-MM-DD）の終値 closePrice（円。例 "3000"、"1234.5"）を入れる。
func insertClosePrice(t *testing.T, pool *pgxpool.Pool, stockCode, tradingDate, closePrice string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO stock_prices (stock_code, trading_date, close_price) VALUES ($1::text, $2::text::date, $3::text::numeric)",
		stockCode, tradingDate, closePrice,
	); err != nil {
		t.Fatalf("終値（%s・%s・%s円）を入れられない: %v", stockCode, tradingDate, closePrice, err)
	}
}
