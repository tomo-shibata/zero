package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// utf8BOM は、UTF-8 のバイト順マーク（U+FEFF）。
const utf8BOM = "\uFEFF"

// Seed は、seed ファイルの中身（DML）を pool の DB に流す。
//
// ファイル全体を引数なしの1回の Exec で送る。pgx は引数がないと simple protocol を使うので、
// 複数の文を `;` で自前に分割せずにそのまま実行できる（文字列の中の `;` で誤って分割しない）。
// 1回のメッセージで送った文は暗黙の1トランザクションになるため、途中で失敗すれば何も残らない。
// そのため seed には BEGIN・COMMIT を書かない決まりにする（途中に COMMIT があると、
// そこまでが確定してしまい、後の文が失敗しても半端なデータが残る）。
func Seed(ctx context.Context, pool *pgxpool.Pool, sql string) error {
	// Windows のエディタは UTF-8 のファイルの先頭に BOM を付けることがあり、
	// PostgreSQL は BOM を最初の文の一部として読んで構文の誤りにするので、取り除いてから流す。
	sql = strings.TrimPrefix(sql, utf8BOM)
	if _, err := pool.Exec(ctx, sql); err != nil {
		return fmt.Errorf("seed を流せません: %w", err)
	}
	return nil
}
