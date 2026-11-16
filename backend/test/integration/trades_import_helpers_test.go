package integration

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/write"
	"github.com/tomo-shibata/zero/backend/internal/interface/tradecsv"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 売買履歴の CSV の取り込み（docs/plans/trade-history/plan.md の PR③）の統合テストが使う補助。
// 組み立ては 4.6・4.7 の境界のとおり: write.NewTradeImportRepository(pool) → command.NewImportTrades → tradecsv.Run。
// 「今日」は 2026-11-16（日本時間）。Run には RunInput.Today に "2026-11-16" を渡す（要件定義の成功基準の前置き）。

const (
	// importToday は、「今日」が 2026-11-16（日本時間）のテストの RunInput.Today。
	importToday = "2026-11-16"
	// devUserID は、開発用利用者（db/seed/dev.sql）。test.sql にはいないので、基準30 のテストが入れる。
	devUserID = "00000000-0000-0000-0000-000000000001"

	// importHeader は、CSV の見出し（要件23）。
	importHeader = "銘柄コード,銘柄名,通貨,売買,価格,株数,取引日"

	// 要件定義の「CSV の誤りの文言」の表のうち、複数のテストが使うもの。
	importFailedHeadline = "取り込めませんでした。何も変えていません。"
	importHeaderMessage  = "1行目の見出しを「銘柄コード,銘柄名,通貨,売買,価格,株数,取引日」にしてください"
	importNoRowsMessage  = "取り込む行がありません"
	importEncodingMsg    = "CSV の文字コードが UTF-8 ではありません"
	importMissingArgsMsg = "CSV_FILE と USER_ID を指定してください"

	// Shift_JIS で書いた CSV（銘柄名「トヨタ自動車」を含む）。実際の Shift_JIS のバイト列。
	shiftJISHeader        = "\x96\xc1\x95\xbf\x83\x52\x81\x5b\x83\x68\x2c\x96\xc1\x95\xbf\x96\xbc\x2c\x92\xca\x89\xdd\x2c\x94\x84\x94\x83\x2c\x89\xbf\x8a\x69\x2c\x8a\x94\x90\x94\x2c\x8e\xe6\x88\xf8\x93\xfa\x0d\x0a"
	shiftJISHeaderSwapped = "\x96\xc1\x95\xbf\x83\x52\x81\x5b\x83\x68\x2c\x96\xc1\x95\xbf\x96\xbc\x2c\x94\x84\x94\x83\x2c\x92\xca\x89\xdd\x2c\x89\xbf\x8a\x69\x2c\x8a\x94\x90\x94\x2c\x8e\xe6\x88\xf8\x93\xfa\x0d\x0a"
	shiftJISRow           = "\x37\x32\x30\x33\x2c\x83\x67\x83\x88\x83\x5e\x8e\xa9\x93\xae\x8e\xd4\x2c\x4a\x50\x59\x2c\x94\x83\x2c\x32\x35\x30\x30\x2c\x31\x30\x30\x2c\x32\x30\x32\x36\x2d\x30\x31\x2d\x31\x30\x0d\x0a"
)

// importOutcome は、Run の結果（終了コードと、標準出力・標準エラーの行）。
type importOutcome struct {
	code   int
	stdout []string
	stderr []string
}

// splitOutputLines は、出力を行に分ける（行末の CR を除き、末尾の改行による空の要素は作らない）。出力がなければ空のスライス。
func splitOutputLines(s string) []string {
	s = strings.TrimRight(s, "\r\n")
	if s == "" {
		return []string{}
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, "\r")
	}
	return lines
}

// newImporter は、pool の DB を使う取り込みのユースケース（4.6）を組み立てる。
func newImporter(pool *pgxpool.Pool) *command.ImportTrades {
	return command.NewImportTrades(write.NewTradeImportRepository(pool))
}

// runImport は、CSV のパス csvPath と利用者 userID（どちらも Run に渡す文字列そのまま）で tradecsv.Run を呼ぶ。
func runImport(t *testing.T, pool *pgxpool.Pool, csvPath, userID string) importOutcome {
	t.Helper()
	var stdout, stderr strings.Builder
	code := tradecsv.Run(t.Context(), tradecsv.RunInput{
		CSVPath:  csvPath,
		UserID:   userID,
		Today:    importToday,
		Importer: newImporter(pool),
	}, &stdout, &stderr)
	return importOutcome{code: code, stdout: splitOutputLines(stdout.String()), stderr: splitOutputLines(stderr.String())}
}

// writeImportCSV は、content を t.TempDir() の中の CSV ファイルに書いて、そのパスを返す。
func writeImportCSV(t *testing.T, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "trades.csv")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("CSV を書けない: %v", err)
	}
	return path
}

// runImportCSV は、content を CSV ファイルにして、利用者 userID に取り込む。
func runImportCSV(t *testing.T, pool *pgxpool.Pool, userID string, content []byte) importOutcome {
	t.Helper()
	return runImport(t, pool, writeImportCSV(t, content), userID)
}

// importCSV は、見出しと lines の行を LF でつないだ CSV の中身にする。
func importCSV(lines ...string) []byte {
	all := append([]string{importHeader}, lines...)
	return []byte(strings.Join(all, "\n") + "\n")
}

// assertImportSucceeded は、取り込みが成功した（終了コード0・標準出力が want の行とちょうど同じ・標準エラーが空）ことを確かめる。
func assertImportSucceeded(t *testing.T, out importOutcome, wantStdout ...string) {
	t.Helper()
	if out.code != 0 {
		t.Errorf("終了コード: got %d, want 0\nstdout: %q\nstderr: %q", out.code, out.stdout, out.stderr)
	}
	if !slices.Equal(out.stdout, wantStdout) {
		t.Errorf("標準出力の行:\n got %q\nwant %q", out.stdout, wantStdout)
	}
	if len(out.stderr) != 0 {
		t.Errorf("標準エラーの行: got %q, want なし（成功の表示は標準出力だけ）", out.stderr)
	}
}

// assertImportFailed は、取り込めなかった（終了コード1・標準エラーが「取り込めませんでした。何も変えていません。」と want の誤りの行がこの順・
// 標準出力が空）ことを確かめる。誤りの行は、要件定義の「CSV の誤りの文言」のとおり（「n行目: 」を含む）。
func assertImportFailed(t *testing.T, out importOutcome, wantProblems ...string) {
	t.Helper()
	if out.code != 1 {
		t.Errorf("終了コード: got %d, want 1\nstdout: %q\nstderr: %q", out.code, out.stdout, out.stderr)
	}
	want := append([]string{importFailedHeadline}, wantProblems...)
	if !slices.Equal(out.stderr, want) {
		t.Errorf("標準エラーの行:\n got %q\nwant %q", out.stderr, want)
	}
	if len(out.stdout) != 0 {
		t.Errorf("標準出力の行: got %q, want なし（誤りの表示は標準エラーだけ）", out.stdout)
	}
}

// importTradeRow は、trades の1行のうち、取り込みのテストで確かめる値。価格は 0.01 単位の整数。
type importTradeRow struct {
	id              int64
	userID          string
	stockCode       string
	side            string
	quantity        int64
	priceHundredths int64
	tradeDate       string // YYYY-MM-DD
}

// importStockRow は、銘柄マスタ（stocks）の1行。
type importStockRow struct {
	code     string
	name     string
	currency string
}

// importSnapshot は、取り込みで変わりうる DB の中身（trades のすべての行と、stocks のすべての行）。
type importSnapshot struct {
	trades []importTradeRow
	stocks []importStockRow
}

// snapshotImportDB は、pool の DB の trades（id の昇順）と stocks（銘柄コードの昇順）をすべて読む。
func snapshotImportDB(t *testing.T, pool *pgxpool.Pool) importSnapshot {
	t.Helper()
	tradeRows, err := pool.Query(t.Context(),
		`SELECT id, user_id::text, stock_code, side, quantity, (price * 100)::bigint, to_char(trade_date, 'YYYY-MM-DD')
		   FROM trades
		  ORDER BY id`)
	if err != nil {
		t.Fatalf("trades を読めない: %v", err)
	}
	trades, err := pgx.CollectRows(tradeRows, func(row pgx.CollectableRow) (importTradeRow, error) {
		var r importTradeRow
		err := row.Scan(&r.id, &r.userID, &r.stockCode, &r.side, &r.quantity, &r.priceHundredths, &r.tradeDate)
		return r, err
	})
	if err != nil {
		t.Fatalf("trades を読めない: %v", err)
	}

	stockRows, err := pool.Query(t.Context(), `SELECT code, name, currency FROM stocks ORDER BY code COLLATE "C"`)
	if err != nil {
		t.Fatalf("stocks を読めない: %v", err)
	}
	stocks, err := pgx.CollectRows(stockRows, func(row pgx.CollectableRow) (importStockRow, error) {
		var r importStockRow
		err := row.Scan(&r.code, &r.name, &r.currency)
		return r, err
	})
	if err != nil {
		t.Fatalf("stocks を読めない: %v", err)
	}
	return importSnapshot{trades: trades, stocks: stocks}
}

// assertDBUnchanged は、pool の DB の trades と stocks が before から何も変わっていない（行も id も値も）ことを確かめる。
func assertDBUnchanged(t *testing.T, pool *pgxpool.Pool, before importSnapshot) {
	t.Helper()
	after := snapshotImportDB(t, pool)
	if !slices.Equal(after.trades, before.trades) {
		t.Errorf("trades が変わった（取り込めなかったときは何も変えない）:\n before %+v\n after  %+v", before.trades, after.trades)
	}
	if !slices.Equal(after.stocks, before.stocks) {
		t.Errorf("stocks が変わった（取り込めなかったときは何も変えない）:\n before %+v\n after  %+v", before.stocks, after.stocks)
	}
}

// importTradesOf は、利用者 userID の売買履歴を登録した順（id の昇順）に返す。
func importTradesOf(t *testing.T, pool *pgxpool.Pool, userID string) []importTradeRow {
	t.Helper()
	var own []importTradeRow
	for _, r := range snapshotImportDB(t, pool).trades {
		if r.userID == userID {
			own = append(own, r)
		}
	}
	return own
}

// importStockByCode は、銘柄マスタの銘柄 code を返す。なければ ok が false。
func importStockByCode(t *testing.T, pool *pgxpool.Pool, code string) (importStockRow, bool) {
	t.Helper()
	for _, r := range snapshotImportDB(t, pool).stocks {
		if r.code == code {
			return r, true
		}
	}
	return importStockRow{}, false
}

// givenImportPool は、取り込みの前提にする DB を作る。Aが 7203 の売買履歴（買 10株・2,000円 2026-01-05）を1件持つ
// （取り込めなかったときに、これが残ることを確かめるため。5.5「基準27・29」）。銘柄マスタは test.sql のまま（7203・6758・9984。AAPL・MSFT はない）。
func givenImportPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testdb.New(t)
	insertTrade(t, pool, seedUserA, "7203", "buy", 10, "2000", "2026-01-05")
	return pool
}
