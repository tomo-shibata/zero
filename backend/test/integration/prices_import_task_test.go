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

// 仕様: 要件（docs/requirements/price-import.md）の要件1（task prices:import CSV_FILE=<CSV のパス> で登録する。DATABASE_URL の DB に、
// 登録する前にマイグレーションを適用する。相対パスは task を実行したディレクトリからのパス）・要件3・要件10・基準7（a）と、
// プラン（docs/plans/price-import/plan.md）4.5（Taskfile の引数）、5.1 の基準7 の行、5.3 の「task の相対パス」「見本の CSV」。
//
// task prices:import を別プロセスで実際に動かす（prices_import_task_test.go の runPricesImportTask は trades_import_task_test.go の
// runTradesImportTask と同じ動かし方）。リポジトリのルートで実行し、DATABASE_URL に testdb の接続文字列を渡す（task は呼び出し側の DATABASE_URL を上書きしない）。
// task は先に db:migrate（→ db:up）を動かすので、docker の PostgreSQL と docker CLI が要る。
// task は、失敗したコマンドの終了コードを既定で 201 にして返すので、失敗は「0以外」で確かめる。task のログが出力に混ざるので、出力の行はちょうどでは比べない。

// pricesImportTaskTimeout は、task prices:import の1回の実行を待つ上限。固まったときにテストを止めるため（中で cmd をビルドすることがあるので長め）。
const pricesImportTaskTimeout = 5 * time.Minute

// examplePricesCSV は、見本の CSV（要件10）。リポジトリの直下からの相対パス（スラッシュ区切り）。
const examplePricesCSV = "backend/db/seed/prices.example.csv"

// 基準7（a）（要件3）: CSV_FILE を指定しないと、「登録できませんでした。何も変えていません。」と「CSV_FILE を指定してください」が出て、終了コードが0以外で、
// 終値は変わらない（Taskfile は requires を使わず、空のまま登録の本体に渡す。4.5）。
// 出力の行に先頭の行と「CSV_FILE を指定してください」があること、0以外の終了コード、DB が変わらないことだけを確かめる
// （ちょうど2行・stdout が空は、Run に空の CSVPath を渡す prices_import_test.go の基準7 の a で確かめる）。
func Test基準7a_taskコマンドでCSV_FILEを指定しないと登録せずに誤りを出す(t *testing.T) {
	t.Parallel()
	pool := givenPricePool(t)
	before := snapshotPriceDB(t, pool)

	code, out := runPricesImportTask(t, pool.Config().ConnString())

	if code == 0 {
		t.Errorf("task prices:import（CSV_FILE なし）の終了コード: got 0, want 0以外\n出力:\n%s", out)
	}
	lines := splitOutputLines(out)
	for _, want := range []string{priceFailedHeadline, priceMissingArgsMsg} {
		if !slices.Contains(lines, want) {
			t.Errorf("出力に行 %q がない\n出力:\n%s", want, out)
		}
	}
	assertPriceDBUnchanged(t, pool, before)
}

// 5.3「task の相対パス」（要件1）: リポジトリの直下から `task prices:import CSV_FILE=backend/db/seed/prices.example.csv` を動かして終了コード0。
// task の前と後に、テストの中で `time.Now()` の日本時間の日付を取り（実装の関数は使わない）、7203・6758 の終値の取引日がそのどちらかで、
// 出力の行に「2件の価格を登録しました（取引日 <その日>）。」がある（日付を固定すると明日から落ち、0時をまたぐと日付が変わるため）。
func Test要件1_taskコマンドで相対パスのCSVを今日の取引日で登録できる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	japan := time.FixedZone("JST", 9*60*60)
	dayBefore := time.Now().In(japan).Format(time.DateOnly)

	code, out := runPricesImportTask(t, pool.Config().ConnString(), "CSV_FILE="+examplePricesCSV)

	dayAfter := time.Now().In(japan).Format(time.DateOnly)
	if code != 0 {
		t.Fatalf("task prices:import の終了コード: got %d, want 0\n出力:\n%s", code, out)
	}
	days := []string{dayBefore, dayAfter}
	lines := splitOutputLines(out)
	if !slices.ContainsFunc(days, func(d string) bool { return slices.Contains(lines, priceRegisteredMessage(2, d)) }) {
		t.Errorf("出力に行 %q（取引日は %v のどちらか）がない\n出力:\n%s", priceRegisteredMessage(2, "<その日>"), days, out)
	}
	got := readStockPrices(t, pool)
	if len(got) != 2 {
		t.Fatalf("stock_prices の行: got %+v, want 7203 と 6758 の2行", got)
	}
	for i, wantCode := range []string{"6758", "7203"} {
		if got[i].stockCode != wantCode {
			t.Errorf("stock_prices の %d 行目の銘柄コード: got %s, want %s", i+1, got[i].stockCode, wantCode)
		}
		if !slices.Contains(days, got[i].tradingDate) {
			t.Errorf("%s の取引日: got %s, want %v のどちらか", got[i].stockCode, got[i].tradingDate, days)
		}
	}
}

// runPricesImportTask は、ほかの環境変数を引き継ぎつつ DATABASE_URL だけを databaseURL にして（CSV_FILE の環境変数は引き継がず）、
// リポジトリのルートを作業ディレクトリに `task prices:import <args>` を別プロセスで実行し、終了コードと出力（標準出力と標準エラー）を返す。
// 終了コードを得られない（起動できない・時間切れ）ときは、テストを止める。databaseURL にはパスワードが入るので、メッセージには出さない。
func runPricesImportTask(t *testing.T, databaseURL string, args ...string) (exitCode int, output string) {
	t.Helper()
	root := repositoryRoot(t)
	taskCmd, err := exec.LookPath("task")
	if err != nil {
		t.Fatalf("task コマンドが見つからない（$(go env GOPATH)/bin を PATH に含める）: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), pricesImportTaskTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, taskCmd, append([]string{"prices:import"}, args...)...) //nolint:gosec // G204: テストが実行するコマンド。引数はこのテストが決める
	cmd.Dir = root
	// task は環境変数も変数として読むので、開発者の環境にある CSV_FILE を引き継がない（引数で渡したものだけにする）。
	env := make([]string, 0, len(cmd.Environ())+1)
	for _, kv := range cmd.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.EqualFold(name, "CSV_FILE") || strings.EqualFold(name, "DATABASE_URL") {
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
	t.Fatalf("task prices:import の終了コードを得られない: %v\n出力:\n%s", err, out)
	return 0, ""
}

// 5.3「見本の CSV」（要件10）: backend/db/seed/prices.example.csv がリポジトリにあり（空でない）、git check-ignore で管理外にならない（コミットするため）。
// 対照: *.local.csv（backend/db/seed/prices.local.csv）は管理外になる（同じ .gitignore の決まり。売買履歴のテストで確かめ済みだが、見本が巻き込まれないことの対照に使う）。
func Test要件10_見本のCSVがリポジトリにあり管理外にならない(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	example := filepath.Join(root, "backend", "db", "seed", "prices.example.csv")
	info, err := os.Stat(example)
	if err != nil {
		t.Fatalf("見本の CSV がない: %v", err)
	}
	if info.IsDir() || info.Size() == 0 {
		t.Errorf("見本の CSV が空かディレクトリ: %s", example)
	}

	gitCmd, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("git コマンドが見つからない: %v", err)
	}
	cases := []struct {
		path    string
		ignored bool
	}{
		{"backend/db/seed/prices.example.csv", false},
		{"backend/db/seed/prices.local.csv", true},
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
				t.Errorf("git check-ignore %s: 管理外になっていない（*.local.csv は管理外にする）", tc.path)
			}
		default:
			t.Fatalf("git check-ignore %s を実行できない: %v", tc.path, err)
		}
	}
}
