package integration

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/tomo-shibata/zero/backend/db"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/database"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: プラン（docs/plans/trade-history/plan.md）4.2（00006_add_currency.sql。stocks.currency を
// `TEXT NOT NULL CHECK (currency IN ('JPY', 'USD'))` で足す。今ある行は円なので、DEFAULT 'JPY' で足してから既定値を外す。
// stock_prices.close_price の CHECK を小数第2位までにする）と 5.3（PR②）の「00006（統合）: 既にある銘柄は JPY になる」。
// 通貨と終値の制約そのものは stocks_prices_constraints_test.go、取引日の範囲の制約（4.7）は trades_constraints_test.go。
// testdb.New は、マイグレーションをすべて適用した DB を返す。ここでは 00005 まで戻して銘柄を入れ、00006 を適用し直して確かめる。

// newProvider は、pool の DB だけに goose の Provider を作る（グローバルな設定を使わない。テストは並列に動くため）。
func newProvider(t *testing.T, pool *pgxpool.Pool) *goose.Provider {
	t.Helper()
	migrations, err := fs.Sub(db.Migrations, "migrations")
	if err != nil {
		t.Fatalf("マイグレーションのファイルを読めない: %v", err)
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = sqlDB.Close() })
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations)
	if err != nil {
		t.Fatalf("goose の Provider を作れない: %v", err)
	}
	return provider
}

// columnExists は、テーブル table に列 column があるかを返す。
func columnExists(t *testing.T, pool *pgxpool.Pool, table, column string) bool {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(t.Context(),
		"SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = $1::text AND column_name = $2::text)",
		table, column,
	).Scan(&exists); err != nil {
		t.Fatalf("列 %s.%s の有無を調べられない: %v", table, column, err)
	}
	return exists
}

// 4.2・5.3: 00006 の down は通貨の列を消し、00005 の状態に戻る。もう一度 up すると、既にある銘柄はすべて JPY になる
// （00005 の状態で足した銘柄も、test.sql の銘柄（7203・6758・9984）も）。up の後に、通貨を書かない銘柄は保存できない（既定値を外してある）。
// 手順: 00005 まで戻す（stocks.currency がなくなる）→ 銘柄 130A を通貨なしで足す → マイグレーションを適用し直す
// → 7203・6758・9984・130A の通貨がすべて JPY → 通貨を書かない INSERT は 23502（NOT NULL）で失敗する。
func Test補助_5_3_マイグレーション00006のdownの後にupし直すと既にある銘柄はJPYになる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	if !columnExists(t, pool, "stocks", "currency") {
		t.Fatal("00006 の up の後に stocks.currency がない")
	}
	provider := newProvider(t, pool)

	results, err := provider.DownTo(t.Context(), 5)
	if err != nil {
		t.Fatalf("00006 を戻せない（down が通らない）: %v", err)
	}
	// ドルを円でまとめる合計の PR① で 00009（usd_jpy_rates）が最新になったので、00009、00008、00007、00006 の順に戻る。
	if len(results) != 4 || results[0].Source.Version != 9 || results[1].Source.Version != 8 || results[2].Source.Version != 7 || results[3].Source.Version != 6 {
		t.Fatalf("戻したマイグレーション: got %+v, want 00009・00008・00007・00006 の4つ（この順）", results)
	}
	if columnExists(t, pool, "stocks", "currency") {
		t.Error("00006 を戻した後も stocks.currency が残っている")
	}
	if _, err := pool.Exec(t.Context(), "INSERT INTO stocks (code, name) VALUES ('130A', 'テスト銘柄130A')"); err != nil {
		t.Fatalf("00005 の状態の stocks に、通貨なしで銘柄を足せない: %v", err)
	}

	if _, err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("down の後にマイグレーションを適用し直せない: %v", err)
	}

	rows, err := pool.Query(t.Context(), "SELECT code, currency FROM stocks ORDER BY code")
	if err != nil {
		t.Fatalf("stocks を読めない: %v", err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var code, currency string
		if err := rows.Scan(&code, &currency); err != nil {
			t.Fatalf("stocks の行を読めない: %v", err)
		}
		got[code] = currency
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("stocks を読めない: %v", err)
	}
	for _, code := range []string{"130A", "6758", "7203", "9984"} {
		if got[code] != "JPY" {
			t.Errorf("銘柄 %s の通貨: got %q, want %q（既にある銘柄は JPY になる）", code, got[code], "JPY")
		}
	}

	// 既定値を外してあるので、通貨を書かない INSERT は NOT NULL 違反（23502）になる。
	_, err = pool.Exec(t.Context(), "INSERT INTO stocks (code, name) VALUES ('AAPL', 'Apple')")
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != sqlStateNotNullViolation {
		t.Errorf("通貨を書かない INSERT: got %v, want SQLSTATE %s（既定値は外してある）", err, sqlStateNotNullViolation)
	}
}
