package integration

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: カード表示のプラン（docs/plans/holdings-cards/plan.md）4.3 の表 holding_orders（00007）の制約。
// PRIMARY KEY (user_id, stock_code)・UNIQUE (user_id, position)・user_id と stock_code の外部キー・position >= 0。
// 「保存できない」は、INSERT の SQLSTATE が制約の種類と一致することで確かめる（trades_constraints_test.go と同じ形）。
// 各テストは、まず正しい行が保存できることを確かめてから、そこから1項目だけ変えた INSERT を試す。

const sqlStateUniqueViolation = "23505" // unique_violation（主キーも同じ）

// tryInsertOrderRow は、holding_orders に1行（利用者 userID・銘柄 code・位置 position）を入れ、そのエラーを返す（成功なら nil）。
func tryInsertOrderRow(t *testing.T, pool *pgxpool.Pool, userID, code string, position int) error {
	t.Helper()
	_, err := pool.Exec(t.Context(),
		"INSERT INTO holding_orders (user_id, stock_code, position) VALUES ($1::uuid, $2::text, $3::int)",
		userID, code, position)
	return err
}

// assertOrderRowRejected は、行の INSERT が SQLSTATE wantCode の PostgreSQL のエラーで失敗することを確かめる。
func assertOrderRowRejected(t *testing.T, pool *pgxpool.Pool, userID, code string, position int, wantCode string) {
	t.Helper()
	err := tryInsertOrderRow(t, pool, userID, code, position)
	if err == nil {
		t.Fatalf("行（%s・%s・位置 %d）を保存できてしまった（SQLSTATE %s で失敗するはず）", userID, code, position, wantCode)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("行（%s・%s・位置 %d）のエラーが PostgreSQL のエラーでない: %v", userID, code, position, err)
	}
	if pgErr.Code != wantCode {
		t.Errorf("行（%s・%s・位置 %d）の SQLSTATE: got %s, want %s（%s）", userID, code, position, pgErr.Code, wantCode, pgErr.Message)
	}
}

func Testカード表示の補助_holding_ordersは同じ利用者の同じ銘柄の2行目を保存できない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	if err := tryInsertOrderRow(t, pool, seedUserA, "7203", 0); err != nil {
		t.Fatalf("正しい行を保存できない: %v", err)
	}

	assertOrderRowRejected(t, pool, seedUserA, "7203", 1, sqlStateUniqueViolation)
}

func Testカード表示の補助_holding_ordersは同じ利用者の同じ位置の2行目を保存できない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	if err := tryInsertOrderRow(t, pool, seedUserA, "7203", 0); err != nil {
		t.Fatalf("正しい行を保存できない: %v", err)
	}

	assertOrderRowRejected(t, pool, seedUserA, "6758", 0, sqlStateUniqueViolation)
}

func Testカード表示の補助_holding_ordersは別の利用者なら同じ銘柄と同じ位置を保存できる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	if err := tryInsertOrderRow(t, pool, seedUserA, "7203", 0); err != nil {
		t.Fatalf("正しい行を保存できない: %v", err)
	}

	if err := tryInsertOrderRow(t, pool, seedUserB, "7203", 0); err != nil {
		t.Errorf("別の利用者の同じ銘柄・同じ位置の行を保存できない: %v", err)
	}
}

func Testカード表示の補助_holding_ordersは位置0以上で飛び番も保存できる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)

	for _, position := range []int{0, 5} {
		code := map[int]string{0: "7203", 5: "6758"}[position]
		if err := tryInsertOrderRow(t, pool, seedUserA, code, position); err != nil {
			t.Errorf("位置 %d の行を保存できない: %v", position, err)
		}
	}
	assertOrderRowRejected(t, pool, seedUserA, "9984", -1, sqlStateCheckViolation)
}

func Testカード表示の補助_holding_ordersは銘柄マスタにない銘柄といない利用者を保存できない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	if err := tryInsertOrderRow(t, pool, seedUserA, "7203", 0); err != nil {
		t.Fatalf("正しい行を保存できない: %v", err)
	}

	assertOrderRowRejected(t, pool, seedUserA, "XXXX", 1, sqlStateForeignKeyViolation)
	assertOrderRowRejected(t, pool, unknownUserID, "6758", 2, sqlStateForeignKeyViolation)
}
