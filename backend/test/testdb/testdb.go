// Package testdb は、テストごとに新しい DB を作るヘルパーを持つ。
// テストが並列に動いても、互いのデータに干渉しないようにするため（テストルール 4）。
package testdb

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/infrastructure/database"
)

const (
	// adminURLEnv は、テスト用の DB を作る PostgreSQL への接続文字列（URL 形式）を指定する環境変数。
	// DB 名には、テスト用の DB とは別の DB（postgres など）を指定する。
	adminURLEnv = "TEST_DATABASE_ADMIN_URL"
	// defaultAdminURL は、docker-compose.yml の PostgreSQL。
	// ホストを localhost にしないのは、Windows で先に IPv6 の ::1 に解決され、接続が遅れたり別のサーバーにつながったりするため。
	defaultAdminURL = "postgres://zero:zero@127.0.0.1:5432/postgres?sslmode=disable" //nolint:gosec // docker-compose.yml のローカル専用の値で、秘密ではない

	// maxConns は、テスト用の DB 1つへの接続プールの接続数の上限。
	// pgxpool の既定（CPU の数と 4 の大きい方）のままだと、並列のテストの数 × CPU の数まで接続が増えて、
	// PostgreSQL の max_connections（既定 100）を超えることがあるため、小さく抑える。
	// 1 にしないのは、1本を使っている間に別の1本で問い合わせるテストも書けるようにするため。
	maxConns = 2
	// poolCloseTimeout は、テストの終了時に接続プールを閉じるのを待つ上限。
	// プールは、使用中の接続がすべて返されるまで閉じないので、テストが接続を返し忘れるとそこで止まり続けるため。
	poolCloseTimeout = 10 * time.Second
	// dropTimeout は、テストの終了時に DB を消すのを待つ上限。
	dropTimeout = 30 * time.Second
)

// New は、DB「zero_test_<ランダム>」を作り、マイグレーション → db/seed/test.sql の順に流して、
// その DB への接続プールを返す。DB はテストの終了時に消す。
// 接続文字列は pool.Config().ConnString() で得られ、別プロセスにもそのまま渡せる。
func New(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := t.Context()

	// seed は DB を作る前に読む。読めないときに、使わない DB を作ってから失敗しないように。
	seed, err := os.ReadFile(seedPath())
	if err != nil {
		t.Fatalf("db/seed/test.sql を読めません: %v", err)
	}

	adminURL := os.Getenv(adminURLEnv)
	if adminURL == "" {
		adminURL = defaultAdminURL
	}
	admin, err := pgx.ParseConfig(adminURL)
	if err != nil {
		t.Fatalf("%s を読めません: %v", adminURLEnv, err)
	}

	// 小文字にするのは、引用符なしの SQL（DROP DATABASE zero_test_… など）でも手で扱えるようにするため。
	name := database.TestDatabasePrefix + strings.ToLower(rand.Text())
	// 接続の設定は DB を作る前に組み立てて確かめる。設定の誤りで、使わない DB を作ってから失敗しないように。
	poolConfig := newPoolConfig(t, adminURL, name)

	if err := database.CreateDatabase(ctx, admin, name); err != nil {
		t.Fatalf("テスト用の DB を作れません（PostgreSQL が動いているか確かめてください。task db:up）: %v", err)
	}
	t.Cleanup(func() {
		// t.Context() はクリーンアップの前に取り消されるので、別の context で消す。
		ctx, cancel := context.WithTimeout(context.Background(), dropTimeout)
		defer cancel()
		if err := database.DropDatabase(ctx, admin, name); err != nil {
			t.Errorf("テスト用の DB %q を消せません: %v", name, err)
		}
	})

	pool, err := database.ConnectConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("テスト用の DB %q に接続できません: %v", name, err)
	}
	// クリーンアップは登録と逆の順に動くので、DB を消す前にプールを閉じる。
	t.Cleanup(func() { closePool(t, pool, name) })

	if _, err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("テスト用の DB %q: %v", name, err)
	}
	if err := database.Seed(ctx, pool, string(seed)); err != nil {
		t.Fatalf("テスト用の DB %q: %v", name, err)
	}
	return pool
}

// newPoolConfig は、URL 形式の接続文字列 adminURL の DB 名を name に差し替えた、接続プールの設定を返す。
func newPoolConfig(t *testing.T, adminURL, name string) *pgxpool.Config {
	t.Helper()
	connString, err := withDatabase(adminURL, name)
	if err != nil {
		t.Fatal(err)
	}
	// 接続数の上限は、接続文字列（pool_max_conns）ではなく設定の構造体で変える。
	// pool.Config().ConnString() は接続文字列をそのまま返すので、文字列に足すと、それを受け取った別プロセスの
	// pgx（プールではないもの）が pool_max_conns を PostgreSQL の設定として送ってしまい、接続できないため。
	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		t.Fatalf("%s を読めません: %v", adminURLEnv, err)
	}
	poolConfig.MaxConns = maxConns
	return poolConfig
}

// closePool は、テスト用の DB name への接続プール pool を閉じる。
// poolCloseTimeout を過ぎても閉じなければ、テストの失敗として知らせて先へ進む。
// 先へ進めば、この後の DROP DATABASE … WITH (FORCE) が残った接続を切って DB を消すので、テストが固まらない。
// そのときは閉じるのを待つ goroutine がテストのプロセスの終わりまで残るが、プロセスとともに消えるので構わない。
func closePool(t *testing.T, pool *pgxpool.Pool, name string) {
	t.Helper()
	closed := make(chan struct{})
	go func() {
		pool.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(poolCloseTimeout):
		t.Errorf("テスト用の DB %q の接続プールが %v 待っても閉じません。テストが接続を返していない（rows.Close の呼び忘れなど）おそれがあります。DB は WITH (FORCE) で消します", name, poolCloseTimeout)
	}
}

// withDatabase は、URL 形式の接続文字列 base の DB 名を name に差し替えた接続文字列を返す。
// プールの ConnString() を別プロセスへそのまま渡せるよう、設定の構造体ではなく文字列で作る。
func withDatabase(base, name string) (string, error) {
	u, err := url.Parse(base)
	// エラーの内容には URL（パスワードを含む）が入るので、出さない。
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return "", errors.New(adminURLEnv + " は postgres:// で始まる URL にしてください")
	}
	u.Path = "/" + name
	u.RawPath = ""
	connString := u.String()

	// ?dbname=… が付いていると path より優先されるので、pgx の解釈で本当に name に接続するかを確かめる。
	// 確かめないと、開発用 DB などにテストのデータを書き込んでしまう。
	cfg, err := pgx.ParseConfig(connString)
	if err != nil {
		return "", fmt.Errorf("%s を読めません: %w", adminURLEnv, err)
	}
	if cfg.Database != name {
		return "", fmt.Errorf("%s の DB 名は URL の path だけで指定してください（%q ではなく %q に接続してしまう）", adminURLEnv, name, cfg.Database)
	}
	return connString, nil
}

// seedPath は、db/seed/test.sql の場所を返す。
// go test はパッケージごとに作業ディレクトリが変わるので、相対パスではなくこのファイルの場所から決める。
func seedPath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "db", "seed", "test.sql")
}
