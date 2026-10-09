package database

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/tomo-shibata/zero/backend/db"
)

// Migrate は、db.Migrations のマイグレーションのうち、pool の DB にまだ適用していないものを適用する。
// 適用したマイグレーションを返す（すべて適用済みなら空）。
// 途中で失敗したときは、失敗する前に適用できたもの（DB に残っている）とエラーを返す。
//
// goose のグローバルな設定（SetDialect・SetBaseFS など）ではなく Provider を使う。
// テストは並列に別々の DB へマイグレーションを流すため、グローバルな状態を共有すると競合するから。
func Migrate(ctx context.Context, pool *pgxpool.Pool) ([]*goose.MigrationResult, error) {
	migrations, err := fs.Sub(db.Migrations, "migrations")
	if err != nil {
		return nil, fmt.Errorf("マイグレーションのファイルを読めません: %w", err)
	}

	// 同じ DB へ同時にマイグレーションを流す（task db:migrate を2つ動かすなど）と、同じ版を二重に適用しようとして片方が失敗する。
	// PostgreSQL のセッション単位のアドバイザリロックで、同じ DB には1つずつ流す。
	// アドバイザリロックは DB ごとに別なので、テストが並列に別々の DB へ流すときは互いを待たない。
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, fmt.Errorf("マイグレーションのロックを準備できません: %w", err)
	}

	// goose は database/sql で動くので、pgxpool を database/sql として包む。
	// これを閉じても pool は閉じない（pgx の stdlib.OpenDBFromPool の仕様）。
	// 閉じるときのエラーは、すでに終わったマイグレーションの結果を変えないので見ない。
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer func() { _ = sqlDB.Close() }()

	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations, goose.WithSessionLocker(locker))
	if err != nil {
		return nil, fmt.Errorf("マイグレーションを準備できません: %w", err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		// goose は途中で失敗すると結果を返さず、それまでに適用できたものを PartialError に入れて返す。
		// 適用できたものは DB に残るので、呼び出し側がログに出せるように取り出して返す。
		var partial *goose.PartialError
		if errors.As(err, &partial) {
			results = partial.Applied
		}
		return results, fmt.Errorf("マイグレーションを適用できません: %w", err)
	}
	return results, nil
}
