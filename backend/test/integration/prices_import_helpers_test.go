package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/write"
	"github.com/tomo-shibata/zero/backend/internal/interface/pricecsv"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 価格の CSV の登録（docs/plans/price-import/plan.md）の統合テストが使う補助。
// 組み立ては 4.5 の境界のとおり: write.NewClosingPriceImportRepository(pool) → command.NewImportClosingPrices → pricecsv.Run。
// 「今日」は 2026-11-16（日本時間）。Run には RunInput.Today に "2026-11-16" を渡す（売買履歴と同じ importToday）。
// 売買履歴の補助（trades_import_helpers_test.go）は直さず、splitOutputLines・importOutcome・assertImportSucceeded・snapshotImportDB は同じものを使う。

const (
	// priceImportHeader は、CSV の見出し（要件5）。
	priceImportHeader = "銘柄コード,終値"

	// 要件定義の「CSV の誤りの文言」の表のうち、複数のテストが使うもの。
	priceFailedHeadline   = "登録できませんでした。何も変えていません。"
	priceMissingArgsMsg   = "CSV_FILE を指定してください"
	priceHeaderMessage    = "1行目の見出しを「銘柄コード,終値」にしてください"
	priceNoRowsMessage    = "登録する行がありません"
	priceEncodingMessage  = "CSV の文字コードが UTF-8 ではありません"
	priceFormatMessage    = "終値は半角数字と小数点で書いてください"
	priceNotPositiveMsg   = "終値は0より大きくしてください"
	priceTooLargeMessage  = "終値は1億未満にしてください"
	priceColumnsMessage   = "列の数を2にしてください"
	priceYenFineMessage   = "円の終値は小数第1位までにしてください"
	priceDollarFineMsg    = "ドルの終値は小数第2位までにしてください"
	priceQuoteTailMessage = "ダブルクォートの使い方が正しくありません"

	// Shift_JIS で書いた見出し「銘柄コード,終値」（実際の Shift_JIS のバイト列）。
	priceShiftJISHeader = "\x96\xc1\x95\xbf\x83\x52\x81\x5b\x83\x68\x2c\x8f\x49\x92\x6c\x0d\x0a"
)

// priceSkippedMessage は、銘柄マスタにない銘柄コードの行を飛ばしたときの表示（要件6）。
func priceSkippedMessage(line int, code string) string {
	return fmt.Sprintf("%d行目: 銘柄コード %s は銘柄マスタにないので飛ばしました", line, code)
}

// priceRegisteredMessage は、成功の表示（要件3）。
func priceRegisteredMessage(count int, tradingDate string) string {
	return fmt.Sprintf("%d件の価格を登録しました（取引日 %s）。", count, tradingDate)
}

// newPriceImporter は、pool の DB を使う価格の登録のユースケース（4.5）を組み立てる。
func newPriceImporter(pool *pgxpool.Pool) *command.ImportClosingPrices {
	return command.NewImportClosingPrices(write.NewClosingPriceImportRepository(pool))
}

// runPriceImportOn は、CSV のパス csvPath と今日 today（どちらも Run に渡す文字列そのまま）で pricecsv.Run を呼ぶ。
func runPriceImportOn(t *testing.T, pool *pgxpool.Pool, csvPath, today string) importOutcome {
	t.Helper()
	var stdout, stderr strings.Builder
	code := pricecsv.Run(t.Context(), pricecsv.RunInput{
		CSVPath:  csvPath,
		Today:    today,
		Importer: newPriceImporter(pool),
	}, &stdout, &stderr)
	return importOutcome{code: code, stdout: splitOutputLines(stdout.String()), stderr: splitOutputLines(stderr.String())}
}

// runPriceImport は、今日が 2026-11-16 のときに csvPath の CSV を登録する。
func runPriceImport(t *testing.T, pool *pgxpool.Pool, csvPath string) importOutcome {
	t.Helper()
	return runPriceImportOn(t, pool, csvPath, importToday)
}

// writePriceCSV は、content を t.TempDir() の中の CSV ファイルに書いて、そのパスを返す。
func writePriceCSV(t *testing.T, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "prices.csv")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("CSV を書けない: %v", err)
	}
	return path
}

// runPriceImportCSV は、content を CSV ファイルにして、今日が 2026-11-16 のときに登録する。
func runPriceImportCSV(t *testing.T, pool *pgxpool.Pool, content []byte) importOutcome {
	t.Helper()
	return runPriceImport(t, pool, writePriceCSV(t, content))
}

// priceCSV は、見出しと lines の行を LF でつないだ CSV の中身にする。
func priceCSV(lines ...string) []byte {
	all := append([]string{priceImportHeader}, lines...)
	return []byte(strings.Join(all, "\n") + "\n")
}

// assertPriceImportFailed は、登録できなかった（終了コード1・標準エラーが「登録できませんでした。何も変えていません。」と
// want の誤りの行がこの順・標準出力が空）ことを確かめる。誤りの行は、要件定義の「CSV の誤りの文言」のとおり（「n行目: 」を含む）。
func assertPriceImportFailed(t *testing.T, out importOutcome, wantProblems ...string) {
	t.Helper()
	if out.code != 1 {
		t.Errorf("終了コード: got %d, want 1\nstdout: %q\nstderr: %q", out.code, out.stdout, out.stderr)
	}
	want := append([]string{priceFailedHeadline}, wantProblems...)
	if !slices.Equal(out.stderr, want) {
		t.Errorf("標準エラーの行:\n got %q\nwant %q", out.stderr, want)
	}
	if len(out.stdout) != 0 {
		t.Errorf("標準出力の行: got %q, want なし（誤りの表示は標準エラーだけ）", out.stdout)
	}
}

// storedPriceRow は、stock_prices の1行（取得日時を含む）。取得日時は、比べるためだけの文字列（UTC・マイクロ秒まで）。
type storedPriceRow struct {
	storedClosePrice
	fetchedAt string
}

// priceSnapshot は、価格の登録で変わりうる DB の中身（stock_prices のすべての行と、trades・stocks のすべての行）。
type priceSnapshot struct {
	base   importSnapshot
	prices []storedPriceRow
}

// snapshotPriceDB は、pool の DB の stock_prices（銘柄コード・取引日の昇順。取得日時を含む）と、trades・stocks をすべて読む。
func snapshotPriceDB(t *testing.T, pool *pgxpool.Pool) priceSnapshot {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT stock_code, to_char(trading_date, 'YYYY-MM-DD'), (close_price * 100)::bigint,
		        to_char(fetched_at AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS.US')
		   FROM stock_prices
		  ORDER BY stock_code COLLATE "C", trading_date`)
	if err != nil {
		t.Fatalf("stock_prices を読めない: %v", err)
	}
	prices, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (storedPriceRow, error) {
		var p storedPriceRow
		err := row.Scan(&p.stockCode, &p.tradingDate, &p.priceHundredths, &p.fetchedAt)
		return p, err
	})
	if err != nil {
		t.Fatalf("stock_prices を読めない: %v", err)
	}
	return priceSnapshot{base: snapshotImportDB(t, pool), prices: prices}
}

// assertPriceDBUnchanged は、pool の DB の stock_prices（取得日時を含む）・stocks・trades が before から何も変わっていないことを確かめる。
func assertPriceDBUnchanged(t *testing.T, pool *pgxpool.Pool, before priceSnapshot) {
	t.Helper()
	after := snapshotPriceDB(t, pool)
	if !slices.Equal(after.prices, before.prices) {
		t.Errorf("stock_prices が変わった（登録できなかったときは何も変えない）:\n before %+v\n after  %+v", before.prices, after.prices)
	}
	if !slices.Equal(after.base.trades, before.base.trades) {
		t.Errorf("trades が変わった:\n before %+v\n after  %+v", before.base.trades, after.base.trades)
	}
	if !slices.Equal(after.base.stocks, before.base.stocks) {
		t.Errorf("stocks が変わった:\n before %+v\n after  %+v", before.base.stocks, after.base.stocks)
	}
}

// givenPricePool は、価格の登録の前提にする DB を作る。Aが 7203 を「買 2,500円 100株 2026-01-10」で持ち、7203 の終値が 3,000円（2026-10-06）の1件
// （「何も変わらない」を確かめるとき、空の表どうしを比べて行を消す誤りを見逃さないため。5章の冒頭）。銘柄マスタは test.sql のまま（7203・6758・9984。AAPL はない）。
func givenPricePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testdb.New(t)
	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "2500", "2026-01-10")
	insertClosePrice(t, pool, "7203", "2026-10-06", "3000")
	return pool
}

// price7203Before は、givenPricePool が入れる 7203 の終値（3,000円・2026-10-06）。
var price7203Before = storedClosePrice{stockCode: "7203", tradingDate: "2026-10-06", priceHundredths: 300000}
