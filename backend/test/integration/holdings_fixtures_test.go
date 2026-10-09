package integration

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// 保有株式一覧の統合テスト（PR②）が使う、保有データと価格を入れるヘルパー。
// プラン（docs/plans/stock-holdings/plan.md）4.4「テストデータ」: 保有データ・価格は seed に入れず、各テストで入れる。
// 列はプラン 4.2 のとおり。holdings は user_id・stock_code・quantity・acquisition_price を入れ、
// stock_prices は stock_code・trading_date・close_price を入れる（id・created_at・fetched_at は DEFAULT に任せる）。
//
// 単価（acquisition_price・close_price）と取引日は、SQL に書く値のまま文字列で渡し、PostgreSQL の text から変換させる。
// 桁数を指定しない NUMERIC は書いた桁（scale）のまま保存されるので（プラン 4.2）、
// "2000"（scale 0）と "2001.0"（scale 1）のような違いを、そのまま DB に入れられるようにするため。

// insertHolding は、利用者 userID の保有データ1件を入れる。acquisitionPrice は円（例 "2500"、"2001.0"）。
func insertHolding(t *testing.T, pool *pgxpool.Pool, userID, stockCode string, quantity int64, acquisitionPrice string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO holdings (user_id, stock_code, quantity, acquisition_price) VALUES ($1::uuid, $2::text, $3::bigint, $4::text::numeric)",
		userID, stockCode, quantity, acquisitionPrice,
	); err != nil {
		t.Fatalf("保有データ（利用者 %s・%s・%d株・%s円）を入れられない: %v", userID, stockCode, quantity, acquisitionPrice, err)
	}
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
