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

// 仕様: おまかせ運用の要件（docs/requirements/managed-account.md）の要件25（task managed:import CSV_FILE=… USER_ID=… で取り込む。DATABASE_URL の DB に、
// 取り込む前にマイグレーションを適用する。CSV_FILE と USER_ID は必須）・要件31・要件32（見本の CSV と *.local.csv）・基準22（a）・基準26 と、
// プラン（docs/plans/managed-account/plan.md）4.8 の PR① の末尾（Taskfile の引数・見本の CSV）、5.1 の基準22・基準26 の行、5.2 の「見本の CSV」。
//
// task managed:import を別プロセスで実際に動かす（trades_import_task_test.go の runTradesImportTask と同じ動かし方）。リポジトリのルートで実行し、
// DATABASE_URL に testdb の接続文字列を渡す（task は呼び出し側の DATABASE_URL を上書きしない）。
// task は先に db:migrate（→ db:up）を動かすので、docker の PostgreSQL と docker CLI が要る。
// task は、失敗したコマンドの終了コードを既定で 201 にして返すので、失敗は「0以外」で確かめる。task のログが出力に混ざるので、出力の行はちょうどでは比べない。
// 基準26 は task を通すので、「今日」は本当の時計の今日（日本時間）になる（見本の CSV の日付は 2026-11-16 以前にしてある）。

// managedImportTaskTimeout は、task managed:import の1回の実行を待つ上限。固まったときにテストを止めるため（中で cmd をビルドすることがあるので長め）。
const managedImportTaskTimeout = 5 * time.Minute

// exampleManagedCSV は、見本のおまかせ運用の CSV（要件32）。リポジトリの直下からの相対パス（スラッシュ区切り）。
const exampleManagedCSV = "backend/db/seed/managed.example.csv"

// 基準22（a）（要件25・要件31）: Aが WealthNavi（記録1件）を持つ。USER_ID を指定しないと、「取り込めませんでした。何も変えていません。」と
// 「CSV_FILE と USER_ID を指定してください」が出て、終了コードが0以外で、Aの記録は変わらない（Taskfile は requires を使わず、空のまま取り込みの本体に渡す）。
// 出力の行に先頭の行と「CSV_FILE と USER_ID を指定してください」があること、0以外の終了コード、DB が変わらないことだけを確かめる
// （ちょうど2行・stdout が空は、Run に空の UserID を渡す managed_import_test.go の基準22 の a で確かめる）。
func Test基準22a_taskコマンドでUSER_IDを指定しないと取り込まずに誤りを出す(t *testing.T) {
	t.Parallel()
	pool := givenManagedPool(t)
	before := snapshotManagedAndFive(t, pool)

	code, out := runManagedImportTask(t, pool.Config().ConnString(), "CSV_FILE="+exampleManagedCSV)

	if code == 0 {
		t.Errorf("task managed:import（USER_ID なし）の終了コード: got 0, want 0以外\n出力:\n%s", out)
	}
	lines := splitOutputLines(out)
	for _, want := range []string{importFailedHeadline, importMissingArgsMsg} {
		if !slices.Contains(lines, want) {
			t.Errorf("出力に行 %q がない\n出力:\n%s", want, out)
		}
	}
	assertManagedAndFiveUnchanged(t, pool, before)
}

// 基準26（要件32）: 開発用利用者の口座が0件。リポジトリの直下から
// `task managed:import CSV_FILE=backend/db/seed/managed.example.csv USER_ID=00000000-0000-0000-0000-000000000001`（相対パスの確かめも兼ねる）が
// 終了コード0。開発用利用者の記録の件数が、見本の CSV の、見出しを除いた空でない行の数と同じ。
func Test基準26_見本のCSVをtaskコマンドで開発用利用者に取り込める(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	if _, err := pool.Exec(t.Context(), "INSERT INTO users (id) VALUES ($1::uuid)", devUserID); err != nil {
		t.Fatalf("開発用利用者を入れられない: %v", err)
	}
	if got := readManagedAccountNames(t, pool, devUserID); len(got) != 0 {
		t.Fatalf("前提: 開発用利用者の口座が0件のはずが %v", got)
	}
	wantCount := countExampleManagedRows(t)
	if wantCount == 0 {
		t.Fatal("見本の CSV に、見出しを除いた空でない行がない")
	}

	code, out := runManagedImportTask(t, pool.Config().ConnString(), "CSV_FILE="+exampleManagedCSV, "USER_ID="+devUserID)

	if code != 0 {
		t.Fatalf("task managed:import の終了コード: got %d, want 0\n出力:\n%s", code, out)
	}
	if got := readManagedValuations(t, pool, devUserID); len(got) != wantCount {
		t.Errorf("開発用利用者の記録の件数: got %d (%+v), want %d（見本の CSV の、見出しを除いた空でない行の数）", len(got), got, wantCount)
	}
	if got := readManagedAccountNames(t, pool, devUserID); len(got) == 0 {
		t.Error("開発用利用者の口座が0件（見本の CSV から取り込めていない）")
	}
}

// countExampleManagedRows は、見本の CSV の、見出し（1行目）を除いた空でない行の数を返す。BOM と CRLF を考える。
func countExampleManagedRows(t *testing.T) int {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repositoryRoot(t), filepath.FromSlash(exampleManagedCSV)))
	if err != nil {
		t.Fatalf("見本の CSV を読めない: %v", err)
	}
	lines := strings.Split(strings.TrimPrefix(string(content), "\xef\xbb\xbf"), "\n")
	count := 0
	for _, l := range lines[1:] {
		if strings.TrimRight(l, "\r") != "" {
			count++
		}
	}
	return count
}

// runManagedImportTask は、ほかの環境変数を引き継ぎつつ DATABASE_URL だけを databaseURL にして（CSV_FILE・USER_ID の環境変数は引き継がず）、
// リポジトリのルートを作業ディレクトリに `task managed:import <args>` を別プロセスで実行し、終了コードと出力（標準出力と標準エラー）を返す。
// 終了コードを得られない（起動できない・時間切れ）ときは、テストを止める。databaseURL にはパスワードが入るので、メッセージには出さない。
func runManagedImportTask(t *testing.T, databaseURL string, args ...string) (exitCode int, output string) {
	t.Helper()
	root := repositoryRoot(t)
	taskCmd, err := exec.LookPath("task")
	if err != nil {
		t.Fatalf("task コマンドが見つからない（$(go env GOPATH)/bin を PATH に含める）: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), managedImportTaskTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, taskCmd, append([]string{"managed:import"}, args...)...) //nolint:gosec // G204: テストが実行するコマンド。引数はこのテストが決める
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
	t.Fatalf("task managed:import の終了コードを得られない: %v\n出力:\n%s", err, out)
	return 0, ""
}

// 要件32: 見本の CSV（backend/db/seed/managed.example.csv）がリポジトリにあり（空でない）、*.local.csv は git の管理外になる（git check-ignore）。
// 対照: 見本の CSV は管理外にならない（コミットする）。
func Test要件32_見本のCSVがリポジトリにあり_local_csvはgitの管理外(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	example := filepath.Join(root, "backend", "db", "seed", "managed.example.csv")
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
		{"backend/db/seed/managed.example.csv", false},
		{"backend/db/seed/managed.local.csv", true},
		{"managed.local.csv", true},
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
