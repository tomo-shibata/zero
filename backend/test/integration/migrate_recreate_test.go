package integration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: プラン（docs/plans/stock-holdings/plan.md）5.1「補助のテスト（バックエンド）」の「判断10（TC-33、EC-2）」の行（PR①）。
// あわせて 4.3「cmd/migrate -recreate は、DATABASE_URL の DB 名が zero_e2e か zero_test_ で始まる場合だけ実行する
// （開発用 DB を消さないため）。DROP DATABASE … WITH (FORCE) → CREATE DATABASE の順に行う」と、6章の判断10。
// 要件の AC はない。確かめるのは次のとおり。
//   - 拒否する側: DB 名が zero_guard_<ランダム> の DB を指す DATABASE_URL を、
//     (a) path がその DB 名の URL、(b) path が zero_e2e で ?dbname= がその DB 名の URL、
//     (c) keyword/value 形式（dbname= がその DB 名）の3通りで渡し、cmd/migrate -recreate を別プロセスで実行する。
//     どれも終了コードが 0 以外で、その DB の pg_database.oid が変わらない（作り直されていない）。
//   - 許可する側: DB 名が zero_test_<ランダム> の DB を指す URL で同じく実行すると、
//     終了コードが 0 で、oid が変わる（作り直された）。
//
// 安全のため（ガードが壊れていたら、指した DB が消されるので）:
//   - 接続先は testdb が使うサーバー（pool.Config().ConnString()）で、DB 名だけを、このテストが作った使い捨ての DB に差し替える。
//     開発用 DB「zero」は決して指さない。実行の前に、その DATABASE_URL で実際につながる DB が使い捨ての DB であることを pgx で確かめる。
//   - 使い捨ての DB は t.Cleanup で必ず消す。

// guardDBPrefix・allowedDBPrefix は、このテストが作る使い捨ての DB の名前の頭。
// プラン 5.1: 拒否する側は zero_guard_、許可する側は zero_test_。
const (
	guardDBPrefix   = "zero_guard_"
	allowedDBPrefix = "zero_test_"
)

// migrateRunTimeout は、cmd/migrate -recreate の1回の実行を待つ上限。固まったときにテストを止めるため。
const migrateRunTimeout = 2 * time.Minute

// プラン 5.1「判断10（TC-33、EC-2）」: cmd/migrate -recreate を別プロセスで実行し、
// DB 名が zero_e2e でも zero_test_ で始まる名前でもない DB は作り直さず（3形式とも）、
// zero_test_ で始まる DB は作り直すことを、pg_database.oid で確かめる。
func Test判断10_TC33_EC2_migrate_recreateは許されないDB名では作り直さずzero_test_で始まるDB名では作り直す(t *testing.T) {
	t.Parallel()

	// 4回実行するので、ビルドは最初の1回だけにする。
	bin, backendDir := buildMigrate(t)
	pool := testdb.New(t)
	server := serverURL(t, pool)

	t.Run("拒否する側", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			dsn  func(db string) string
		}{
			{"a_pathがDB名のURL", func(db string) string { return urlWithPath(server, db) }},
			{"b_pathはzero_e2eでdbnameパラメータがDB名のURL", func(db string) string { return urlWithDBNameParam(server, "zero_e2e", db) }},
			{"c_keyword_value形式", func(db string) string { return keywordValueDSN(server, db) }},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				// 形式ごとに別の DB を使う（1つの形式で作り直されても、他の形式の確かめに影響しない）。
				db := createDisposableDB(t, pool, guardDBPrefix)
				dsn := tc.dsn(db)
				requireConnectsTo(t, dsn, db)
				before, ok := databaseOID(t, pool, db)
				if !ok {
					t.Fatalf("作ったばかりの DB %s が pg_database にない", db)
				}

				code, out := runMigrateRecreate(t, bin, backendDir, dsn)

				if code == 0 {
					t.Errorf("終了コード: got 0, want 0 以外（DB 名 %s は zero_e2e でも zero_test_ で始まる名前でもない）\n出力:\n%s", db, out)
				}
				after, ok := databaseOID(t, pool, db)
				switch {
				case !ok:
					t.Errorf("DB %s が消えている（-recreate が拒否せずに DROP した）\n出力:\n%s", db, out)
				case after != before:
					t.Errorf("oid: got %d, want %d（DB %s が作り直された）\n出力:\n%s", after, before, db, out)
				}
			})
		}
	})

	t.Run("許可する側", func(t *testing.T) {
		t.Parallel()
		db := createDisposableDB(t, pool, allowedDBPrefix)
		dsn := urlWithPath(server, db)
		requireConnectsTo(t, dsn, db)
		before, ok := databaseOID(t, pool, db)
		if !ok {
			t.Fatalf("作ったばかりの DB %s が pg_database にない", db)
		}

		code, out := runMigrateRecreate(t, bin, backendDir, dsn)

		if code != 0 {
			t.Errorf("終了コード: got %d, want 0（DB 名 %s は zero_test_ で始まる）\n出力:\n%s", code, db, out)
		}
		after, ok := databaseOID(t, pool, db)
		switch {
		case !ok:
			t.Errorf("DB %s がない（DROP の後に CREATE されていない）\n出力:\n%s", db, out)
		case after == before:
			t.Errorf("oid が %d のまま変わらない（DB %s が作り直されていない）\n出力:\n%s", before, db, out)
		}
	})
}

// buildMigrate は、cmd/migrate を一時ディレクトリにビルドし、そのバイナリのパスと backend/ のパスを返す。
// go test はパッケージのディレクトリ（backend/test/integration）で動くので、backend/ はその2つ上。
func buildMigrate(t *testing.T) (bin, backendDir string) {
	t.Helper()
	backendDir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("backend/ の場所を決められない: %v", err)
	}
	if _, err := os.Stat(filepath.Join(backendDir, "cmd", "migrate")); err != nil {
		t.Fatalf("backend/cmd/migrate が見つからない（%s）: %v", backendDir, err)
	}
	goCmd, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go コマンドが見つからない: %v", err)
	}

	bin = filepath.Join(t.TempDir(), "migrate")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, goCmd, "build", "-o", bin, "./cmd/migrate") //nolint:gosec // G204: テストが自分でビルドするコマンド。引数はこのテストが決める
	cmd.Dir = backendDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cmd/migrate をビルドできない: %v\n出力:\n%s", err, out)
	}
	return bin, backendDir
}

// runMigrateRecreate は、ほかの環境変数を引き継ぎつつ DATABASE_URL だけを databaseURL にして、
// backend/ を作業ディレクトリに `migrate -recreate` を別プロセスで実行し、終了コードと出力（標準出力と標準エラー）を返す。
// 終了コードを得られない（起動できない・時間切れ）ときは、拒否と区別できないのでテストを止める。
func runMigrateRecreate(t *testing.T, bin, backendDir, databaseURL string) (exitCode int, output string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), migrateRunTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-recreate")
	cmd.Dir = backendDir
	// 同じ名前の環境変数が複数あるときは最後のものが使われる（os/exec の Cmd.Env の決まり）。
	cmd.Env = append(cmd.Environ(), "DATABASE_URL="+databaseURL)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && ctx.Err() == nil {
		return exitErr.ExitCode(), string(out)
	}
	t.Fatalf("migrate -recreate の終了コードを得られない: %v\n出力:\n%s", err, out)
	return 0, ""
}

// serverURL は、testdb の pool の接続文字列から、接続先（ホスト・ポート・利用者・パスワード）と sslmode だけを残した URL を返す。
// DB 名（path）は空にする。各形式の DATABASE_URL は、これに DB 名を入れて作る。
// プラン 4.3 では TEST_DATABASE_ADMIN_URL は postgres:// の URL なので、testdb の接続文字列も URL として読む。
func serverURL(t *testing.T, pool *pgxpool.Pool) *url.URL {
	t.Helper()
	// 接続文字列にはパスワードが入るので、メッセージには出さない。
	u, err := url.Parse(pool.Config().ConnString())
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" {
		t.Fatal("testdb の接続文字列を postgres:// の URL として読めない")
	}
	sslmode := u.Query().Get("sslmode")
	if sslmode == "" {
		sslmode = "disable"
	}
	return &url.URL{
		Scheme:   u.Scheme,
		User:     u.User,
		Host:     u.Host,
		RawQuery: url.Values{"sslmode": {sslmode}}.Encode(),
	}
}

// urlWithPath は、server の DB 名を path で db にした URL を返す（形式 a、許可する側）。
func urlWithPath(server *url.URL, db string) string {
	u := *server
	u.Path = "/" + db
	return u.String()
}

// urlWithDBNameParam は、path を pathDB にし、?dbname= を db にした URL を返す（形式 b）。
// pgx（libpq と同じ）では ?dbname= が path より優先されるので、実際につながるのは db。
func urlWithDBNameParam(server *url.URL, pathDB, db string) string {
	u := *server
	u.Path = "/" + pathDB
	q := u.Query()
	q.Set("dbname", db)
	u.RawQuery = q.Encode()
	return u.String()
}

// keywordValueDSN は、server と同じ接続先で dbname= を db にした keyword/value 形式の接続文字列を返す（形式 c）。
// 例: host='127.0.0.1' port='5432' user='zero' password='…' dbname='zero_guard_…' sslmode='disable'
func keywordValueDSN(server *url.URL, db string) string {
	parts := []string{"host=" + quoteKeywordValue(server.Hostname())}
	if port := server.Port(); port != "" {
		parts = append(parts, "port="+quoteKeywordValue(port))
	}
	parts = append(parts, "user="+quoteKeywordValue(server.User.Username()))
	if password, ok := server.User.Password(); ok {
		parts = append(parts, "password="+quoteKeywordValue(password))
	}
	parts = append(parts,
		"dbname="+quoteKeywordValue(db),
		"sslmode="+quoteKeywordValue(server.Query().Get("sslmode")),
	)
	return strings.Join(parts, " ")
}

// quoteKeywordValue は、keyword/value 形式の値を単一引用符で囲む（中の \ と ' は \ でエスケープする）。
func quoteKeywordValue(v string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v) + "'"
}

// createDisposableDB は、pool（testdb の DB への接続）で、名前が prefix<ランダム> の空の DB を作って名前を返す。
// テストの終わりに、DROP DATABASE IF EXISTS … WITH (FORCE) で必ず消す。
func createDisposableDB(t *testing.T, pool *pgxpool.Pool, prefix string) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("乱数を作れない: %v", err)
	}
	// 小文字の16進にする（引用しない識別子として扱われても名前が変わらないように）。
	name := prefix + hex.EncodeToString(b)
	ident := pgx.Identifier{name}.Sanitize()

	// CREATE が途中で失敗しても残さないよう、先に後始末を登録する。
	// t.Context() はクリーンアップの前に取り消されるので、ここでは使わない。
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if _, err := pool.Exec(ctx, "DROP DATABASE IF EXISTS "+ident+" WITH (FORCE)"); err != nil {
			t.Errorf("使い捨ての DB %s を消せない: %v", name, err)
		}
	})
	if _, err := pool.Exec(t.Context(), "CREATE DATABASE "+ident); err != nil {
		t.Fatalf("使い捨ての DB %s を作れない: %v", name, err)
	}
	return name
}

// requireConnectsTo は、dsn で実際につながる DB が want であることを、実行の前に pgx で確かめる。
// DATABASE_URL が本当にこのテストの使い捨ての DB を指していて（開発用 DB などを指していない）、
// 接続文字列として正しい（拒否された理由が接続文字列の誤りではない）ことを保証するため。
func requireConnectsTo(t *testing.T, dsn, want string) {
	t.Helper()
	if !strings.HasPrefix(want, guardDBPrefix) && !strings.HasPrefix(want, allowedDBPrefix) {
		t.Fatalf("このテストが作った使い捨ての DB ではない DB %q を指そうとしている", want)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("DATABASE_URL で %s に接続できない（接続文字列の作り方が誤っている）: %v", want, err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	var got string
	if err := conn.QueryRow(ctx, "SELECT current_database()").Scan(&got); err != nil {
		t.Fatalf("DB 名を読めない: %v", err)
	}
	if got != want {
		t.Fatalf("DATABASE_URL でつながる DB: got %q, want %q", got, want)
	}
}

// databaseOID は、名前が name の DB の pg_database.oid を返す。DB がなければ ok が false。
func databaseOID(t *testing.T, pool *pgxpool.Pool, name string) (oid uint32, ok bool) {
	t.Helper()
	err := pool.QueryRow(t.Context(), "SELECT oid FROM pg_database WHERE datname = $1", name).Scan(&oid)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false
	}
	if err != nil {
		t.Fatalf("DB %s の oid を読めない: %v", name, err)
	}
	return oid, true
}
