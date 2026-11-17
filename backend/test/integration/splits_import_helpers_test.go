package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/write"
	"github.com/tomo-shibata/zero/backend/internal/interface/splitcsv"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 分割の CSV の取り込み（docs/plans/stock-split/plan.md の PR②）の統合テストが使う補助。
// 組み立ては 4.8 の PR② の境界のとおり: write.NewStockSplitImportRepository(pool) → command.NewImportStockSplits → splitcsv.Run。
// 「今日」は 2026-11-16（日本時間）。Run には RunInput.Today に "2026-11-16" を渡す（importToday）。
// 取り込めたか・取り込めなかったかの確かめは、売買履歴の補助（assertImportSucceeded・assertImportFailed。trades_import_helpers_test.go）をそのまま使う
// （先頭の行「取り込めませんでした。何も変えていません。」は売買履歴の CSV と同じ文言）。
// 「何も変わらない」は、5つの表を比べる補助（snapshotFiveTables・assertFiveTablesUnchanged。stock_splits_fixtures_test.go）で確かめる。

const (
	// splitImportHeader は、分割の CSV の見出し（要件15）。
	splitImportHeader = "銘柄コード,分割の日,分割後の株数"

	// 要件定義の「分割の CSV の誤りの文言」の表のうち、複数のテストが使うもの。
	splitMissingArgsMsg   = "CSV_FILE を指定してください"
	splitHeaderMessage    = "1行目の見出しを「銘柄コード,分割の日,分割後の株数」にしてください"
	splitNoRowsMessage    = "取り込む行がありません"
	splitEncodingMessage  = "CSV の文字コードが UTF-8 ではありません"
	splitQuoteTailMessage = "ダブルクォートの使い方が正しくありません"
	splitRatioSmallMsg    = "分割後の株数は2以上の整数にしてください"
	splitCodeMessage      = "銘柄コードは半角の英大文字・数字・「.」・「-」で1〜10文字にしてください"
	splitDateMessage      = "分割の日は YYYY-MM-DD か YYYY/M/D の実在する日付にしてください"

	// Shift_JIS で書いた見出しの行（CRLF まで）。見出し「銘柄コード,分割の日,分割後の株数」の実際の Shift_JIS のバイト列。
	splitShiftJISHeader = "\x96\xc1\x95\xbf\x83\x52\x81\x5b\x83\x68\x2c\x95\xaa\x8a\x84\x82\xcc\x93\xfa\x2c\x95\xaa\x8a\x84\x8c\xe3\x82\xcc\x8a\x94\x90\x94\x0d\x0a"
)

// splitRegisteredMessage は、成功の表示（要件13）。
func splitRegisteredMessage(count int) string {
	return fmt.Sprintf("%d件の分割を取り込みました。", count)
}

// splitOversellMessage は、売買履歴との関係の誤りの文言（要件13の表）。取引日は "YYYY-MM-DD"。
func splitOversellMessage(userID, code, tradeDate string, sold, held int64) string {
	return fmt.Sprintf("利用者 %s の銘柄 %s の %s の売り（%d株）が、その時点の保有数量（%d株）より多くなります", userID, code, tradeDate, sold, held)
}

// newSplitImporter は、pool の DB を使う分割の取り込みのユースケース（4.8）を組み立てる。
func newSplitImporter(pool *pgxpool.Pool) *command.ImportStockSplits {
	return command.NewImportStockSplits(write.NewStockSplitImportRepository(pool))
}

// runSplitImport は、今日が 2026-11-16 のときに、CSV のパス csvPath（Run に渡す文字列そのまま）の CSV を取り込む。
func runSplitImport(t *testing.T, pool *pgxpool.Pool, csvPath string) importOutcome {
	t.Helper()
	var stdout, stderr strings.Builder
	code := splitcsv.Run(t.Context(), splitcsv.RunInput{
		CSVPath:  csvPath,
		Today:    importToday,
		Importer: newSplitImporter(pool),
	}, &stdout, &stderr)
	return importOutcome{code: code, stdout: splitOutputLines(stdout.String()), stderr: splitOutputLines(stderr.String())}
}

// writeSplitCSV は、content を t.TempDir() の中の CSV ファイルに書いて、そのパスを返す。
func writeSplitCSV(t *testing.T, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "splits.csv")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("CSV を書けない: %v", err)
	}
	return path
}

// runSplitImportCSV は、content を CSV ファイルにして、今日が 2026-11-16 のときに取り込む。
func runSplitImportCSV(t *testing.T, pool *pgxpool.Pool, content []byte) importOutcome {
	t.Helper()
	return runSplitImport(t, pool, writeSplitCSV(t, content))
}

// splitCSV は、見出しと lines の行を LF でつないだ CSV の中身にする。
func splitCSV(lines ...string) []byte {
	all := append([]string{splitImportHeader}, lines...)
	return []byte(strings.Join(all, "\n") + "\n")
}

// storedSplit は、stock_splits の1行のうち、取り込みのテストで確かめる値。
type storedSplit struct {
	stockCode string
	splitDate string // YYYY-MM-DD
	ratio     int64
}

// readStockSplits は、stock_splits のすべての行を、銘柄コード（Go の文字列比較の順）・分割の日の昇順で返す。なければ空のスライス。
func readStockSplits(t *testing.T, pool *pgxpool.Pool) []storedSplit {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT stock_code, to_char(split_date, 'YYYY-MM-DD'), ratio::bigint
		   FROM stock_splits
		  ORDER BY stock_code COLLATE "C", split_date`)
	if err != nil {
		t.Fatalf("stock_splits を読めない: %v", err)
	}
	splits, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (storedSplit, error) {
		var s storedSplit
		err := row.Scan(&s.stockCode, &s.splitDate, &s.ratio)
		return s, err
	})
	if err != nil {
		t.Fatalf("stock_splits を読めない: %v", err)
	}
	if splits == nil {
		splits = []storedSplit{}
	}
	return splits
}

// givenSplitsPool は、分割の取り込みの前提にする DB を作る。Aの売買履歴は 7203「買 2,000円 100株 2026-01-10」、7203 の終値は 3,000円（2026-10-06）、
// Aの並びは「7203」を1件ずつ入れる（取り込めなかったときに、これらが残ることを確かめるため。空の表どうしを比べて行を消す誤りを見逃さないため。5章の冒頭）。
// withSplit が true なら、7203 の分割「1→3 2026-03-31」を登録済みにする。銘柄マスタは test.sql のまま（7203・6758・9984。AAPL・NVDA はない）。
func givenSplitsPool(t *testing.T, withSplit bool) *pgxpool.Pool {
	t.Helper()
	pool := testdb.New(t)
	if withSplit {
		insertStockSplit(t, pool, "7203", "2026-03-31", 3)
	}
	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "2000", "2026-01-10")
	insertClosePrice(t, pool, "7203", "2026-10-06", "3000")
	insertHoldingOrder(t, pool, seedUserA, "7203")
	return pool
}

// assertSplitsImportedKeepingOthers は、取り込みが stock_splits 以外の表（trades・stocks・stock_prices・holding_orders）を変えていないことを確かめる。
// 取り込みで stock_splits は変わるので、5つの表のうち stock_splits だけを除く。
func assertSplitsImportedKeepingOthers(t *testing.T, pool *pgxpool.Pool, before fiveTableSnapshot) {
	t.Helper()
	after := snapshotFiveTables(t, pool)
	for _, c := range []struct{ name, before, after string }{
		{"trades", before.trades, after.trades},
		{"stocks", before.stocks, after.stocks},
		{"stock_prices", before.stockPrices, after.stockPrices},
		{"holding_orders", before.holdingOrders, after.holdingOrders},
	} {
		if c.before != c.after {
			t.Errorf("%s が変わった（分割の取り込みは変えない）:\n before %s\n after  %s", c.name, c.before, c.after)
		}
	}
}
