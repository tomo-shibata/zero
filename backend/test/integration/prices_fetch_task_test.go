package integration

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 要件（docs/requirements/stock-holdings.md）の AC-14b（FR-15a「開発・テスト用に、FR-15 と同じ取得処理を
// 手動で実行する task コマンドを用意する」）と、プラン（docs/plans/stock-holdings/plan.md）5.1 の AC-14b の行
// （AC-13 と同じデータで、task prices:fetch を別プロセスで実行する（DATABASE_URL に testdb の接続文字列を渡す）。
// 終了コードが 0、stock_prices が 7203（30000・2026-10-06）と 6758（35000・2026-10-06）の各1行）。
// あわせて 4.5「task prices:fetch は、呼び出し側の DATABASE_URL を上書きしない（AC-14b でテスト用 DB を渡すため）」、
// 4.3 の cmd/fetch-prices の環境変数（必須は DATABASE_URL）、4.4「株価取得の決まり」4（終了コード）、
// 4.4 の testdb（接続文字列は pool.Config().ConnString() で得て、別プロセスに渡す）。
//
// task コマンドはリポジトリのルート（Taskfile.yml のある場所）で実行する。
// task は $(go env GOPATH)/bin にあり、PATH に含める（プラン 4.5）。
//
// 前提（PR③ ステップ6 の EC-P2-6）: task prices:fetch は先に db:migrate（→ db:up）を実行するので（プラン 4.5）、
// docker の PostgreSQL と、PATH の docker（docker CLI）が要る。testdb の DB だけがあっても、docker がなければこのテストは失敗する。

// pricesFetchTaskTimeout は、task prices:fetch の1回の実行を待つ上限。固まったときにテストを止めるため。
// task の中で cmd/fetch-prices をビルドすることがあるので、長めにする。
const pricesFetchTaskTimeout = 5 * time.Minute

// AC-14b（FR-15a）: AC-13 と同じ（7203 と 6758 が保有されていて、価格は未保存）→ 手動実行の task コマンドを実行する
// → AC-13 と同じ価格（7203 は3,000円・2026-10-06、6758 は3,500円・2026-10-06）が保存される。
// 別プロセスの FetchResult は見えないので、終了コードと DB で確かめる（プラン 10章の SR-19・TC-29）。
func TestAC14b_手動実行のtaskコマンドでもAC13と同じ価格が保存される(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertAC13Holdings(t, pool)

	code, out := runPricesFetchTask(t, pool.Config().ConnString())

	if code != 0 {
		t.Errorf("task prices:fetch の終了コード: got %d, want 0\n出力:\n%s", code, out)
	}
	assertStockPrices(t, pool, ac13WantPrices())
}

// runPricesFetchTask は、ほかの環境変数を引き継ぎつつ DATABASE_URL だけを databaseURL にして、
// リポジトリのルートを作業ディレクトリに `task prices:fetch` を別プロセスで実行し、終了コードと出力（標準出力と標準エラー）を返す。
// 終了コードを得られない（起動できない・時間切れ）ときは、テストを止める。
// databaseURL にはパスワードが入るので、メッセージには出さない。
func runPricesFetchTask(t *testing.T, databaseURL string) (exitCode int, output string) {
	t.Helper()
	root := repositoryRoot(t)
	taskCmd, err := exec.LookPath("task")
	if err != nil {
		t.Fatalf("task コマンドが見つからない（$(go env GOPATH)/bin を PATH に含める。プラン 4.5）: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), pricesFetchTaskTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, taskCmd, "prices:fetch") //nolint:gosec // G204: テストが実行するコマンド。引数はこのテストが決める
	cmd.Dir = root
	// 同じ名前の環境変数が複数あるときは最後のものが使われる（os/exec の Cmd.Env の決まり）。
	cmd.Env = append(cmd.Environ(), "DATABASE_URL="+databaseURL)
	// 時間切れで task を止めても、task が起動した子プロセスが出力をつかんだままだと、出力の読み取りが終わらない。
	// その場合も待ち続けないよう、止めてから待つ上限を決める。
	cmd.WaitDelay = 10 * time.Second

	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && ctx.Err() == nil {
		return exitErr.ExitCode(), string(out)
	}
	t.Fatalf("task prices:fetch の終了コードを得られない: %v\n出力:\n%s", err, out)
	return 0, ""
}

// repositoryRoot は、リポジトリのルート（Taskfile.yml のある場所）を返す。
// go test はパッケージのディレクトリ（backend/test/integration）で動くので、ルートはその3つ上。
func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("リポジトリのルートの場所を決められない: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "Taskfile.yml")); err != nil {
		t.Fatalf("リポジトリのルートに Taskfile.yml が見つからない（%s）: %v", root, err)
	}
	return root
}
