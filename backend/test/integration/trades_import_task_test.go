package integration

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 要件（docs/requirements/trade-history.md）の要件19（task trades:import CSV_FILE=… USER_ID=… で取り込む。DATABASE_URL の DB に、
// 取り込む前にマイグレーションを適用する）・要件21・基準21 と、プラン（docs/plans/trade-history/plan.md）4.4・4.7（Taskfile の引数・USER_ID の扱い）、
// 5.1 の基準21 の行、5.5 の「基準21」「task で USER_ID なし」。
//
// task trades:import を別プロセスで実際に動かす（prices_fetch_task_test.go の runPricesFetchTask と同じ動かし方）。
// リポジトリのルートで実行し、CSV_FILE はコミットした backend/test/integration/testdata/ のファイルを、リポジトリの直下からの相対パスで渡す。
// DATABASE_URL に testdb の接続文字列を渡す（task は呼び出し側の DATABASE_URL を上書きしない）。
// task は $(go env GOPATH)/bin にあり、PATH に含める。task は先に db:migrate（→ db:up）を動かすので、docker の PostgreSQL と docker CLI が要る。
// task は、失敗したコマンドの終了コードを既定で 201 にして返す（Taskfile の prices:fetch の summary）ので、失敗は「0以外」で確かめる。

// tradesImportTaskTimeout は、task trades:import の1回の実行を待つ上限。固まったときにテストを止めるため（中で cmd をビルドすることがあるので長め）。
const tradesImportTaskTimeout = 5 * time.Minute

// basicImportCSV は、基準21 の CSV（見出しと 7203・AAPL・MSFT の買い3件）。リポジトリの直下からの相対パス（スラッシュ区切り）。
const basicImportCSV = "backend/test/integration/testdata/trades_import_basic.csv"

// 基準21（要件19・要件20・要件21）: 銘柄マスタに 7203 があり、Aの売買履歴は0件。CSV は見出しと「7203,トヨタ自動車,JPY,買,2500,100,2026-01-10」
// 「AAPL,Apple,USD,買,150.25,10,2026-01-10」「MSFT,Microsoft,USD,買,400,5,2026-02-01」
// → task trades:import で Aに取り込むと、終了コード0で「3件の売買履歴を取り込みました。」「新しく登録した銘柄: AAPL, MSFT」が表示され、
// Aの売買履歴は3件、銘柄マスタに AAPL（Apple・USD）と MSFT（Microsoft・USD）が足される。
// 5.5: 取り込んだ後も stock_prices が空のままであること（価格は取得しない。要件20）も確かめる。Bの売買履歴は変わらない。
func Test基準21_taskコマンドでCSVを取り込める(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertTrade(t, pool, seedUserB, "6758", "buy", 5, "3000", "2026-01-05")
	bBefore := importTradesOf(t, pool, seedUserB)

	code, out := runTradesImportTask(t, pool.Config().ConnString(), "CSV_FILE="+basicImportCSV, "USER_ID="+seedUserA)

	if code != 0 {
		t.Fatalf("task trades:import の終了コード: got %d, want 0\n出力:\n%s", code, out)
	}
	lines := splitOutputLines(out)
	for _, want := range []string{"3件の売買履歴を取り込みました。", "新しく登録した銘柄: AAPL, MSFT"} {
		if !slices.Contains(lines, want) {
			t.Errorf("出力に行 %q がない\n出力:\n%s", want, out)
		}
	}
	// 件数の行と新しい銘柄の行は、この順に隣り合う（task のログが混ざるので添字で比べる）。
	if i := slices.Index(lines, "3件の売買履歴を取り込みました。"); i < 0 || slices.Index(lines, "新しく登録した銘柄: AAPL, MSFT") != i+1 {
		t.Errorf("「3件の売買履歴を取り込みました。」の次の行が「新しく登録した銘柄: AAPL, MSFT」でない\n出力:\n%s", out)
	}
	for _, l := range lines {
		if strings.HasPrefix(l, "銘柄名を変えた銘柄") {
			t.Errorf("出力に %q がある（銘柄名は変えていない）", l)
		}
	}

	gotA := importTradesOf(t, pool, seedUserA)
	if len(gotA) != 3 {
		t.Fatalf("Aの売買履歴の件数: got %d (%+v), want 3", len(gotA), gotA)
	}
	for i, wantCode := range []string{"7203", "AAPL", "MSFT"} {
		if gotA[i].stockCode != wantCode {
			t.Errorf("Aの売買履歴の %d 件目（登録した順）: got %s, want %s", i+1, gotA[i].stockCode, wantCode)
		}
	}
	for code, want := range map[string]importStockRow{
		"AAPL": {code: "AAPL", name: "Apple", currency: "USD"},
		"MSFT": {code: "MSFT", name: "Microsoft", currency: "USD"},
	} {
		if got, ok := importStockByCode(t, pool, code); !ok || got != want {
			t.Errorf("銘柄マスタの %s: got %+v (ok=%v), want %+v", code, got, ok, want)
		}
	}
	if got := importTradesOf(t, pool, seedUserB); !slices.Equal(got, bBefore) {
		t.Errorf("Bの売買履歴が変わった: before %+v, after %+v", bBefore, got)
	}
	assertStockPrices(t, pool, nil) // 価格は取得しない（stock_prices は空のまま）
}

// 5.5「task で USER_ID なし」（基準24 (a)）: USER_ID を指定しないと、「CSV_FILE と USER_ID を指定してください」が出て、終了コードが0以外で、
// Aの売買履歴は変わらない（Taskfile は requires を使わず、空のまま取り込みの本体に渡す。4.7）。
func Test基準24a_taskコマンドでUSER_IDを指定しないと取り込まずに誤りを出す(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertTrade(t, pool, seedUserA, "7203", "buy", 10, "2000", "2026-01-05")
	before := snapshotImportDB(t, pool)

	code, out := runTradesImportTask(t, pool.Config().ConnString(), "CSV_FILE="+basicImportCSV)

	if code == 0 {
		t.Errorf("task trades:import（USER_ID なし）の終了コード: got 0, want 0以外\n出力:\n%s", out)
	}
	if !slices.Contains(splitOutputLines(out), importMissingArgsMsg) {
		t.Errorf("出力に行 %q がない\n出力:\n%s", importMissingArgsMsg, out)
	}
	assertDBUnchanged(t, pool, before)
}

// runTradesImportTask は、ほかの環境変数を引き継ぎつつ DATABASE_URL だけを databaseURL にして（CSV_FILE・USER_ID の環境変数は引き継がず）、
// リポジトリのルートを作業ディレクトリに `task trades:import <args>` を別プロセスで実行し、終了コードと出力（標準出力と標準エラー）を返す。
// 終了コードを得られない（起動できない・時間切れ）ときは、テストを止める。databaseURL にはパスワードが入るので、メッセージには出さない。
func runTradesImportTask(t *testing.T, databaseURL string, args ...string) (exitCode int, output string) {
	t.Helper()
	root := repositoryRoot(t)
	taskCmd, err := exec.LookPath("task")
	if err != nil {
		t.Fatalf("task コマンドが見つからない（$(go env GOPATH)/bin を PATH に含める）: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), tradesImportTaskTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, taskCmd, append([]string{"trades:import"}, args...)...) //nolint:gosec // G204: テストが実行するコマンド。引数はこのテストが決める
	cmd.Dir = root
	// task は環境変数も変数として読むので、開発者の環境にある CSV_FILE・USER_ID を引き継がない（引数で渡したものだけにする）。
	env := make([]string, 0, len(cmd.Environ())+1)
	for _, kv := range cmd.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.EqualFold(name, "CSV_FILE") || strings.EqualFold(name, "USER_ID") || strings.EqualFold(name, "DATABASE_URL") {
			continue
		}
		env = append(env, kv)
	}
	cmd.Env = append(env, "DATABASE_URL="+databaseURL)
	// 時間切れで task を止めても、子プロセスが出力をつかんだままだと読み取りが終わらないので、止めてから待つ上限を決める。
	cmd.WaitDelay = 10 * time.Second

	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && ctx.Err() == nil {
		return exitErr.ExitCode(), string(out)
	}
	t.Fatalf("task trades:import の終了コードを得られない: %v\n出力:\n%s", err, out)
	return 0, ""
}

// 4.4・4.7: 見本の CSV（backend/db/seed/trades.example.csv。要件26）がリポジトリにある。
func Test要件26_見本のCSVがリポジトリにある(t *testing.T) {
	t.Parallel()
	example := filepath.Join(repositoryRoot(t), "backend", "db", "seed", "trades.example.csv")
	info, err := os.Stat(example)
	if err != nil {
		t.Fatalf("見本の CSV がない: %v", err)
	}
	if info.IsDir() || info.Size() == 0 {
		t.Errorf("見本の CSV が空かディレクトリ: %s", example)
	}
}

// 要件26・5.3・5.5: *.local.csv は git の管理外になる（git check-ignore）。backend/db/seed/x.local.csv と、リポジトリの直下の trades.local.csv。
// 対照: 見本の CSV（trades.example.csv）は管理外にならない（コミットする）。
func Test要件26_local_csvはgitの管理外で見本のCSVは管理外にならない(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	gitCmd, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("git コマンドが見つからない: %v", err)
	}
	cases := []struct {
		path    string
		ignored bool
	}{
		{"backend/db/seed/x.local.csv", true},
		{"trades.local.csv", true},
		{"backend/db/seed/trades.example.csv", false},
	}
	for _, tc := range cases {
		cmd := exec.CommandContext(t.Context(), gitCmd, "check-ignore", "--quiet", "--no-index", tc.path) //nolint:gosec // G204: テストが実行するコマンド。引数はこのテストが決める
		cmd.Dir = root
		err := cmd.Run()
		var exitErr *exec.ExitError
		switch {
		case err == nil: // 終了コード0: 管理外
			if !tc.ignored {
				t.Errorf("git check-ignore %s: 管理外になっている（コミットするファイルなので管理外にしない）", tc.path)
			}
		case errors.As(err, &exitErr) && exitErr.ExitCode() == 1: // 終了コード1: 管理外ではない
			if tc.ignored {
				t.Errorf("git check-ignore %s: 管理外になっていない（.gitignore に *.local.csv を足す）", tc.path)
			}
		default:
			t.Fatalf("git check-ignore %s を実行できない: %v", tc.path, err)
		}
	}
}
