package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// TestDatabasePrefix は、テスト用の DB 名の先頭。
// testdb はこの名前で DB を作っては消し、cmd/migrate -recreate はこの名前の DB なら消してよいとみなす。
// 1か所で決めるのは、片方だけを変えて、テスト用の DB を作り直せなくなったり、
// テスト用ではない DB を消せるようになったりしないため。
// Taskfile の db:clean-test も、この名前で始まる DB を消す（変えるときはあわせて変える）。
const TestDatabasePrefix = "zero_test_"

// CreateDatabase は、admin で接続したサーバーに、空の DB name を作る。
// admin は、作る DB とは別の DB（postgres など）に接続する設定にすること。
func CreateDatabase(ctx context.Context, admin *pgx.ConnConfig, name string) error {
	return execOnAdmin(ctx, admin, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize())
}

// DropDatabase は、admin で接続したサーバーから DB name を消す。なければ何もしない。
// WITH (FORCE) にするのは、閉じ忘れた接続や落ちたプロセスの接続が残っていても確実に消すため。
func DropDatabase(ctx context.Context, admin *pgx.ConnConfig, name string) error {
	return execOnAdmin(ctx, admin, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
}

// execOnAdmin は、admin に1本だけ接続して sql を実行する。
// 1回しか使わない接続なので、プールは作らない。
func execOnAdmin(ctx context.Context, admin *pgx.ConnConfig, sql string) error {
	conn, err := pgx.ConnectConfig(ctx, admin)
	if err != nil {
		return fmt.Errorf("DB を作る・消すための接続ができません: %w", err)
	}
	// 閉じるときのエラーは、すでに終わった sql の結果を変えないので見ない。
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, sql); err != nil {
		return fmt.Errorf("%q を実行できません: %w", sql, err)
	}
	return nil
}
