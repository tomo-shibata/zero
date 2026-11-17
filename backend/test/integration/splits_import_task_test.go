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

// 仕様: 株式分割の要件（docs/requirements/stock-split.md）の要件9（task splits:import CSV_FILE=<CSV のパス> で取り込む。DATABASE_URL の DB に、
// 取り込む前にマイグレーションを適用する。相対パスは task を実行したディレクトリからのパス）・要件13・要件18・基準17（a）・基準21 と、
// プラン（docs/plans/stock-split/plan.md）4.8 の PR② の末尾（Taskfile の引数）、5.1 の基準17・基準21 の行、5.2 の「見本の CSV」。
//
// task splits:import を別プロセスで実際に動かす（prices_import_task_test.go の runPricesImportTask と同じ動かし方）。リポジトリのルートで実行し、
// DATABASE_URL に testdb の接続文字列を渡す（task は呼び出し側の DATABASE_URL を上書きしない）。
// task は先に db:migrate（→ db:up）を動かすので、docker の PostgreSQL と docker CLI が要る。
// task は、失敗したコマンドの終了コードを既定で 201 にして返すので、失敗は「0以外」で確かめる。task のログが出力に混ざるので、出力の行はちょうどでは比べない。

// splitsImportTaskTimeout は、task splits:import の1回の実行を待つ上限。固まったときにテストを止めるため（中で cmd をビルドすることがあるので長め）。
const splitsImportTaskTimeout = 5 * time.Minute

// exampleSplitsCSV は、見本の分割の CSV（要件18）。リポジトリの直下からの相対パス（スラッシュ区切り）。
const exampleSplitsCSV = "backend/db/seed/splits.example.csv"

// 基準17（a）（要件9・要件13）: 分割は 7203（2026-03-31・3）の1件が登録済み。CSV_FILE を指定しないと、「取り込めませんでした。何も変えていません。」と
// 「CSV_FILE を指定してください」が出て、終了コードが0以外で、分割は変わらない（Taskfile は requires を使わず、空のまま取り込みの本体に渡す）。
// 出力の行に先頭の行と「CSV_FILE を指定してください」があること、0以外の終了コード、DB が変わらないことだけを確かめる
// （ちょうど2行・stdout が空は、Run に空の CSVPath を渡す splits_import_test.go の基準17 の a で確かめる）。
func Test基準17a_taskコマンドでCSV_FILEを指定しないと取り込まずに誤りを出す(t *testing.T) {
	t.Parallel()
	pool := givenSplitsPool(t, true)
	before := snapshotFiveTables(t, pool)

	code, out := runSplitsImportTask(t, pool.Config().ConnString())

	if code == 0 {
		t.Errorf("task splits:import（CSV_FILE なし）の終了コード: got 0, want 0以外\n出力:\n%s", out)
	}
	lines := splitOutputLines(out)
	for _, want := range []string{importFailedHeadline, splitMissingArgsMsg} {
		if !slices.Contains(lines, want) {
			t.Errorf("出力に行 %q がない\n出力:\n%s", want, out)
		}
	}
	assertFiveTablesUnchanged(t, pool, before)
}

// 基準21（要件18）: 分割は登録されておらず、開発用利用者の売買履歴は0件。リポジトリの直下から
// `task splits:import CSV_FILE=backend/db/seed/splits.example.csv`（相対パスの確かめも兼ねる）が終了コード0で、
// 続けて見本の売買履歴の CSV（backend/db/seed/trades.example.csv）を開発用利用者に取り込むと、これも成功する（見本の分割は見本の売買履歴に響かない）。
// stock_splits は 7203・6758 の2行（プラン 4.6 の見本）で、出力に「2件の分割を取り込みました。」の行がある。
func Test基準21_見本の分割のCSVと見本の売買履歴のCSVをどちらも取り込める(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	if _, err := pool.Exec(t.Context(), "INSERT INTO users (id) VALUES ($1::uuid)", devUserID); err != nil {
		t.Fatalf("開発用利用者を入れられない: %v", err)
	}
	if got := importTradesOf(t, pool, devUserID); len(got) != 0 {
		t.Fatalf("前提: 開発用利用者の売買履歴が0件のはずが %d 件", len(got))
	}

	code, out := runSplitsImportTask(t, pool.Config().ConnString(), "CSV_FILE="+exampleSplitsCSV)

	if code != 0 {
		t.Fatalf("task splits:import の終了コード: got %d, want 0\n出力:\n%s", code, out)
	}
	if !slices.Contains(splitOutputLines(out), splitRegisteredMessage(2)) {
		t.Errorf("出力に行 %q がない\n出力:\n%s", splitRegisteredMessage(2), out)
	}
	splits := readStockSplits(t, pool)
	if len(splits) != 2 {
		t.Fatalf("stock_splits: got %+v, want 7203 と 6758 の2行", splits)
	}
	codes := []string{splits[0].stockCode, splits[1].stockCode}
	if !slices.Equal(codes, []string{"6758", "7203"}) {
		t.Errorf("stock_splits の銘柄コード: got %v, want 6758 と 7203（Go の文字列比較の順）", codes)
	}

	example := filepath.Join(repositoryRoot(t), "backend", "db", "seed", "trades.example.csv")
	tradesOut := runImport(t, pool, example, devUserID)
	if tradesOut.code != 0 {
		t.Fatalf("見本の売買履歴の CSV の取り込み: 終了コード got %d, want 0\nstdout: %q\nstderr: %q", tradesOut.code, tradesOut.stdout, tradesOut.stderr)
	}
	if len(tradesOut.stderr) != 0 {
		t.Errorf("標準エラーの行: got %q, want なし", tradesOut.stderr)
	}
	if got := importTradesOf(t, pool, devUserID); len(got) == 0 {
		t.Error("開発用利用者の売買履歴が0件（見本の売買履歴の CSV から取り込めていない）")
	}
}

// runSplitsImportTask は、ほかの環境変数を引き継ぎつつ DATABASE_URL だけを databaseURL にして（CSV_FILE の環境変数は引き継がず）、
// リポジトリのルートを作業ディレクトリに `task splits:import <args>` を別プロセスで実行し、終了コードと出力（標準出力と標準エラー）を返す。
// 終了コードを得られない（起動できない・時間切れ）ときは、テストを止める。databaseURL にはパスワードが入るので、メッセージには出さない。
func runSplitsImportTask(t *testing.T, databaseURL string, args ...string) (exitCode int, output string) {
	t.Helper()
	root := repositoryRoot(t)
	taskCmd, err := exec.LookPath("task")
	if err != nil {
		t.Fatalf("task コマンドが見つからない（$(go env GOPATH)/bin を PATH に含める）: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), splitsImportTaskTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, taskCmd, append([]string{"splits:import"}, args...)...) //nolint:gosec // G204: テストが実行するコマンド。引数はこのテストが決める
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
	t.Fatalf("task splits:import の終了コードを得られない: %v\n出力:\n%s", err, out)
	return 0, ""
}

// 5.2「見本の CSV」（要件18）: backend/db/seed/splits.example.csv がリポジトリにあり（空でない）、git check-ignore で管理外にならない（コミットするため）。
// 対照: *.local.csv（backend/db/seed/splits.local.csv）は管理外になる（自分の分割の CSV を置く場所。見本が巻き込まれないことの対照）。
func Test要件18_見本の分割のCSVがリポジトリにあり管理外にならない(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	example := filepath.Join(root, "backend", "db", "seed", "splits.example.csv")
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
		{"backend/db/seed/splits.example.csv", false},
		{"backend/db/seed/splits.local.csv", true},
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
