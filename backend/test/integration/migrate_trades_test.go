package integration

import (
	"io/fs"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/tomo-shibata/zero/backend/db"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/database"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: プラン（docs/plans/trade-history/plan.md）4.2（00005_replace_holdings_with_trades.sql。up は trades を作って holdings を消し、
// down は holdings を作り直して trades を消す。中身は戻らない）と 5.3「マイグレーション」（PR①）:
// 00005 の up の後に holdings がなく trades がある。down → up が通る。
// testdb.New は、マイグレーションをすべて適用した DB を返す（PR② で最新の版が 00006 になる。00005 を戻すには、先に 00006 を戻す）。

// tableExists は、スキーマ public にテーブル name があるかを返す。
func tableExists(t *testing.T, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(t.Context(), "SELECT to_regclass('public.' || $1::text) IS NOT NULL", name).Scan(&exists); err != nil {
		t.Fatalf("テーブル %s の有無を調べられない: %v", name, err)
	}
	return exists
}

// assertTables は、holdings と trades の有無が want のとおりであることを確かめる。
func assertTables(t *testing.T, pool *pgxpool.Pool, wantHoldings, wantTrades bool) {
	t.Helper()
	if got := tableExists(t, pool, "holdings"); got != wantHoldings {
		t.Errorf("holdings テーブルの有無: got %v, want %v", got, wantHoldings)
	}
	if got := tableExists(t, pool, "trades"); got != wantTrades {
		t.Errorf("trades テーブルの有無: got %v, want %v", got, wantTrades)
	}
}

// 4.2・5.3: 00005 の up の後に、holdings がなく trades がある。
func Test補助_5_3_マイグレーション00005のupの後はholdingsがなくtradesがある(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)

	assertTables(t, pool, false, true)
}

// 4.2・5.3: 00005 の down は holdings を作り直して trades を消し（中身は戻らない）、もう一度 up を流すと trades ができて holdings が消える。
// 手順: trades に1行入れる → 最新のマイグレーション（00006。PR②）を1つ戻し、次に 00005 を1つ戻す → holdings があって trades がなく、holdings に行を入れられる
// → マイグレーションを適用し直す → holdings がなく trades がある（down の前の trades の行は戻らず、空）。
func Test補助_5_3_マイグレーション00005はdownの後にupし直せる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")

	migrations, err := fs.Sub(db.Migrations, "migrations")
	if err != nil {
		t.Fatalf("マイグレーションのファイルを読めない: %v", err)
	}
	// goose のグローバルな設定を使わず、この DB だけに Provider を作る（テストは並列に動くため）。
	sqlDB := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = sqlDB.Close() })
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations)
	if err != nil {
		t.Fatalf("goose の Provider を作れない: %v", err)
	}

	// 最新は 00008（分割。株式分割の PR①）。先にそれを戻す。
	result, err := provider.Down(t.Context())
	if err != nil {
		t.Fatalf("00008 を戻せない（down が通らない）: %v", err)
	}
	if result.Source.Version != 8 {
		t.Fatalf("戻したマイグレーションの版: got %d, want 8（00008_create_stock_splits.sql が最新）", result.Source.Version)
	}
	// 次は 00007（並び。カード表示の PR②）。
	result, err = provider.Down(t.Context())
	if err != nil {
		t.Fatalf("00007 を戻せない（down が通らない）: %v", err)
	}
	if result.Source.Version != 7 {
		t.Fatalf("戻したマイグレーションの版: got %d, want 7（00007_create_holding_orders.sql が最新）", result.Source.Version)
	}
	// 次は 00006（通貨と終値の精度。売買履歴の PR②）。
	result, err = provider.Down(t.Context())
	if err != nil {
		t.Fatalf("00006 を戻せない（down が通らない）: %v", err)
	}
	if result.Source.Version != 6 {
		t.Fatalf("戻したマイグレーションの版: got %d, want 6（00006_add_currency.sql）", result.Source.Version)
	}
	result, err = provider.Down(t.Context())
	if err != nil {
		t.Fatalf("00005 を戻せない（down が通らない）: %v", err)
	}
	if result.Source.Version != 5 {
		t.Fatalf("戻したマイグレーションの版: got %d, want 5（00005_replace_holdings_with_trades.sql）", result.Source.Version)
	}
	assertTables(t, pool, true, false)
	// 作り直した holdings に、もとの形（利用者・銘柄コード・保有数量・取得価格）で行を入れられる。
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO holdings (user_id, stock_code, quantity, acquisition_price) VALUES ($1::uuid, '7203', 100, 2500.0)", seedUserA,
	); err != nil {
		t.Errorf("down で作り直した holdings に行を入れられない: %v", err)
	}

	if _, err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("down の後にマイグレーションを適用し直せない: %v", err)
	}
	assertTables(t, pool, false, true)
	var count int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM trades").Scan(&count); err != nil {
		t.Fatalf("trades を数えられない: %v", err)
	}
	if count != 0 {
		t.Errorf("up し直した後の trades の行数: got %d, want 0（中身は戻らない）", count)
	}
}
