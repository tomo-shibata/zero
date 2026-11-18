package integration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/write"
	"github.com/tomo-shibata/zero/backend/internal/interface/managedcsv"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: おまかせ運用の要件（docs/requirements/managed-account.md）の要件1・要件2・要件25〜要件31・非機能2 と「CSV の誤りの文言」の表、
// 基準19〜25・基準28 と、プラン（docs/plans/managed-account/plan.md）4.5（取り込み）、4.8 の PR① の境界
// （repository.ManagedAccountImportRepository・command.ImportManagedAccounts・managedcsv.Run・write.NewManagedAccountRepository）、
// 5.1 の PR① の行、5.2 の「Run と取り込み（統合）」。
// 基準22（a）と基準26（task を通した取り込み）は managed_import_task_test.go。ここは managedcsv.Run を testdb で直接呼ぶ。
// 補助は managed_accounts_fixtures_test.go と、売買履歴の取り込みの補助（importOutcome・assertImportSucceeded・assertImportFailed。
// 先頭の行「取り込めませんでした。何も変えていません。」は売買履歴の CSV と同じ文言）。
// 「今日」は 2026-11-16（日本時間。importToday）。Aは seedUserA、Bは seedUserB。

const (
	// managedHeader は、おまかせ運用の CSV の見出し（要件28）。
	managedHeader = "名前,日付,評価額,損益"

	// 要件定義の「CSV の誤りの文言」の表のうち、複数のテストが使うもの。「元本（評価額 − 損益）」の「−」は U+2212。
	managedHeaderMessage    = "1行目の見出しを「名前,日付,評価額,損益」にしてください"
	managedQuoteTailMessage = "ダブルクォートの使い方が正しくありません"
	managedValuationZeroMsg = "評価額は1以上にしてください"
	managedDateMessage      = "日付は YYYY-MM-DD か YYYY/M/D の実在する日付にしてください"
	managedGainMessage      = "損益は半角数字だけの整数で書いてください（マイナスは先頭に -）"

	// Shift_JIS で書いた CSV（名前に漢字を含む）。見出し「名前,日付,評価額,損益」と、1行「ロボアド,2026-01-10,500000,1000」の実際の Shift_JIS のバイト列。
	managedShiftJISHeader = "\x96\xbc\x91\x4f\x2c\x93\xfa\x95\x74\x2c\x95\x5d\x89\xbf\x8a\x7a\x2c\x91\xb9\x89\x76\x0d\x0a"
	// 見出し「名前,評価額,日付,損益」（順番違い）の Shift_JIS のバイト列。
	managedShiftJISHeaderSwapped = "\x96\xbc\x91\x4f\x2c\x95\x5d\x89\xbf\x8a\x7a\x2c\x93\xfa\x95\x74\x2c\x91\xb9\x89\x76\x0d\x0a"
	managedShiftJISRow           = "\x83\x8d\x83\x7b\x83\x41\x83\x68\x2c\x32\x30\x32\x36\x2d\x30\x31\x2d\x31\x30\x2c\x35\x30\x30\x30\x30\x30\x2c\x31\x30\x30\x30\x0d\x0a"
)

// managedRegisteredMessage は、成功の表示（要件31）。
func managedRegisteredMessage(count int) string {
	return fmt.Sprintf("%d件の記録を取り込みました。", count)
}

// newManagedImporter は、pool の DB を使うおまかせ運用の取り込みのユースケース（4.8）を組み立てる。
func newManagedImporter(pool *pgxpool.Pool) *command.ImportManagedAccounts {
	return command.NewImportManagedAccounts(write.NewManagedAccountRepository(pool))
}

// runManagedImport は、今日が 2026-11-16 のときに、CSV のパス csvPath と利用者 userID（どちらも Run に渡す文字列そのまま）で managedcsv.Run を呼ぶ。
func runManagedImport(t *testing.T, pool *pgxpool.Pool, csvPath, userID string) importOutcome {
	t.Helper()
	var stdout, stderr strings.Builder
	code := managedcsv.Run(t.Context(), managedcsv.RunInput{
		CSVPath:  csvPath,
		UserID:   userID,
		Today:    importToday,
		Importer: newManagedImporter(pool),
	}, &stdout, &stderr)
	return importOutcome{code: code, stdout: splitOutputLines(stdout.String()), stderr: splitOutputLines(stderr.String())}
}

// writeManagedCSV は、content を t.TempDir() の中の CSV ファイルに書いて、そのパスを返す。
func writeManagedCSV(t *testing.T, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "managed.csv")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("CSV を書けない: %v", err)
	}
	return path
}

// runManagedImportCSV は、content を CSV ファイルにして、利用者 userID に取り込む。
func runManagedImportCSV(t *testing.T, pool *pgxpool.Pool, userID string, content []byte) importOutcome {
	t.Helper()
	return runManagedImport(t, pool, writeManagedCSV(t, content), userID)
}

// managedCSV は、見出しと lines の行を LF でつないだ CSV の中身にする。
func managedCSV(lines ...string) []byte {
	all := append([]string{managedHeader}, lines...)
	return []byte(strings.Join(all, "\n") + "\n")
}

// 基準19（要件1・要件2・要件25・要件26・要件31）: Aの口座は0件。見出しと「WealthNavi,2026-10-31,3950000,950000」「WealthNavi,2026-11-16,4000000,+1000000」
// 「wealthnavi,2026-11-16,100000,0」「Alpha,2026/11/1,500000,-20000」の CSV をAに取り込むと、終了コード0で「4件の記録を取り込みました。」の1行だけが標準出力に出て、
// 標準エラーは空。Aの記録は (Alpha, 2026-11-01, 500000, -20000)・(WealthNavi, 2026-10-31, 3950000, 950000)・(WealthNavi, 2026-11-16, 4000000, 1000000)・
// (wealthnavi, 2026-11-16, 100000, 0) のちょうど4行で、口座は Alpha・WealthNavi・wealthnavi の3つ（大文字と小文字が違う名前は別の口座）。
func Test基準19_CSVの記録を口座ごとに取り込める(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	if got := readManagedAccountNames(t, pool, seedUserA); len(got) != 0 {
		t.Fatalf("前提: Aの口座が0件のはずが %v", got)
	}

	out := runManagedImportCSV(t, pool, seedUserA, managedCSV(
		"WealthNavi,2026-10-31,3950000,950000",
		"WealthNavi,2026-11-16,4000000,+1000000",
		"wealthnavi,2026-11-16,100000,0",
		"Alpha,2026/11/1,500000,-20000",
	))

	assertImportSucceeded(t, out, managedRegisteredMessage(4))
	wantRows := []storedManagedValuation{
		{"Alpha", "2026-11-01", 500_000, -20_000},
		{"WealthNavi", "2026-10-31", 3_950_000, 950_000},
		{"WealthNavi", "2026-11-16", 4_000_000, 1_000_000},
		{"wealthnavi", "2026-11-16", 100_000, 0},
	}
	if got := readManagedValuations(t, pool, seedUserA); !slices.Equal(got, wantRows) {
		t.Errorf("Aの記録:\n got %+v\nwant %+v", got, wantRows)
	}
	wantNames := []string{"Alpha", "WealthNavi", "wealthnavi"}
	if got := readManagedAccountNames(t, pool, seedUserA); !slices.Equal(got, wantNames) {
		t.Errorf("Aの口座: got %q, want %q", got, wantNames)
	}
}

// 基準20（要件26）: Aが WealthNavi（記録2件）と Alpha（記録1件）と、売買履歴 7203 の1件（終値・並びも）を持ち、Bが WealthNavi（記録1件）を持つ。
// 「WealthNavi,2026-11-16,4100000,1100000」だけの CSV をAに取り込むと成功し、Aの記録はこの1行だけ、Aの口座は WealthNavi だけ（Alpha はなくなる）。
// Bの口座と記録（作成日時を含む）と、5つの表（Aの 7203 の売買履歴・終値・並びを含む）は前後で同じ。
func Test基準20_取り込みはその利用者の口座と記録だけを入れ替える(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-10-31", 3_950_000, 950_000)
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-11-16", 4_000_000, 1_000_000)
	insertManagedValuation(t, pool, seedUserA, "Alpha", "2026-11-16", 500_000, 0)
	insertManagedValuation(t, pool, seedUserB, "WealthNavi", "2026-11-16", 9_000_000, 1_000_000)
	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "2000", "2026-01-10")
	insertClosePrice(t, pool, "7203", "2026-10-06", "3000")
	insertHoldingOrder(t, pool, seedUserA, "7203")
	bBefore := snapshotManagedTablesOf(t, pool, seedUserB)
	fiveBefore := snapshotFiveTables(t, pool)

	out := runManagedImportCSV(t, pool, seedUserA, managedCSV("WealthNavi,2026-11-16,4100000,1100000"))

	assertImportSucceeded(t, out, managedRegisteredMessage(1))
	wantRows := []storedManagedValuation{{"WealthNavi", "2026-11-16", 4_100_000, 1_100_000}}
	if got := readManagedValuations(t, pool, seedUserA); !slices.Equal(got, wantRows) {
		t.Errorf("Aの記録:\n got %+v\nwant %+v", got, wantRows)
	}
	if got := readManagedAccountNames(t, pool, seedUserA); !slices.Equal(got, []string{"WealthNavi"}) {
		t.Errorf("Aの口座: got %q, want [WealthNavi]（Alpha はなくなる）", got)
	}
	if got := snapshotManagedTablesOf(t, pool, seedUserB); got != bBefore {
		t.Errorf("Bの口座と記録が変わった:\n before %+v\n after  %+v", bBefore, got)
	}
	assertFiveTablesUnchanged(t, pool, fiveBefore)
}

// 基準21（要件27）: BOM 付き・改行 CRLF で、名前を `"Wealth, Navi"` と書いた行と、空の行（その後にもう1行ある）を含む CSV をAに取り込むと成功し、
// 口座の名前は `Alpha` と `Wealth, Navi` の2つ、記録は空の行の後の行も入った2件になる。
func Test基準21_BOM付きCRLFで囲んだ名前と空の行を含むCSVを取り込める(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	content := []byte("\xef\xbb\xbf" + managedHeader + "\r\n" +
		`"Wealth, Navi",2026-11-16,4000000,1000000` + "\r\n" +
		"\r\n" +
		"Alpha,2026-11-16,500000,0" + "\r\n")

	out := runManagedImportCSV(t, pool, seedUserA, content)

	assertImportSucceeded(t, out, managedRegisteredMessage(2))
	if got := readManagedAccountNames(t, pool, seedUserA); !slices.Equal(got, []string{"Alpha", "Wealth, Navi"}) {
		t.Errorf("Aの口座: got %q, want [Alpha, Wealth, Navi]", got)
	}
	wantRows := []storedManagedValuation{
		{"Alpha", "2026-11-16", 500_000, 0},
		{"Wealth, Navi", "2026-11-16", 4_000_000, 1_000_000},
	}
	if got := readManagedValuations(t, pool, seedUserA); !slices.Equal(got, wantRows) {
		t.Errorf("Aの記録:\n got %+v\nwant %+v", got, wantRows)
	}
}

// 基準22（a'〜g）（要件25・要件28・要件31）: Aが WealthNavi（記録1件）を持つ。次のどれかで取り込むと、標準エラーが
// 「取り込めませんでした。何も変えていません。」とファイルの段階の文言1つの2行、標準出力は空、終了コード1、Aの記録は1件のまま（口座の2つの表と5つの表は変わらない）。
// (a') Run に空の UserID（task を通す (a) は managed_import_task_test.go） (b) 存在しないファイル (c) DB にいない利用者の ID (d) Shift_JIS で書いた CSV
// (e) 見出しが `名前,評価額,日付,損益`（順番違い） (f) 見出しだけの CSV (g) 3行目の名前が `"Wealth` で閉じていない CSV。
// 加えて、Run に空の CSVPath も「CSV_FILE と USER_ID を指定してください」になる。
func Test基準22_ファイルの段階の誤りは文言1つだけ出して何も変えない(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "no-such-managed.csv")
	cases := []struct {
		name   string
		path   string // body が nil のとき、Run にそのまま渡すパス
		body   []byte // nil でなければ、これをファイルにして、そのパスを渡す
		userID string
		want   string
	}{
		{"a_USER_IDなし", "", managedCSV("WealthNavi,2026-11-16,4000000,1000000"), "", importMissingArgsMsg},
		{"a_CSV_FILEなし", "", nil, seedUserA, importMissingArgsMsg},
		{"b_存在しないファイル", missing, nil, seedUserA, "ファイル " + missing + " を開けません"},
		{"c_DBにいない利用者", "", managedCSV("WealthNavi,2026-11-16,4000000,1000000"), unknownUserID, "利用者 " + unknownUserID + " が見つかりません"},
		{"d_Shift_JIS", "", []byte(managedShiftJISHeader + managedShiftJISRow), seedUserA, importEncodingMsg},
		{"e_見出しの順番違い", "", []byte("名前,評価額,日付,損益\nWealthNavi,4000000,2026-11-16,1000000\n"), seedUserA, managedHeaderMessage},
		{"f_見出しだけ", "", []byte(managedHeader + "\n"), seedUserA, importNoRowsMessage},
		{"g_閉じていないダブルクォート", "", managedCSV("WealthNavi,2026-11-16,4000000,1000000", `"Wealth,2026-11-16,4000000,1000000`),
			seedUserA, "3行目: " + managedQuoteTailMessage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := givenManagedPool(t)
			before := snapshotManagedAndFive(t, pool)
			path := tc.path
			if tc.body != nil {
				path = writeManagedCSV(t, tc.body)
			}

			out := runManagedImport(t, pool, path, tc.userID)

			assertImportFailed(t, out, tc.want)
			wantA := []storedManagedValuation{{"WealthNavi", "2026-11-15", 4_000_000, 1_000_000}}
			if got := readManagedValuations(t, pool, seedUserA); !slices.Equal(got, wantA) {
				t.Errorf("Aの記録:\n got %+v\nwant %+v（1件のまま）", got, wantA)
			}
			assertManagedAndFiveUnchanged(t, pool, before)
		})
	}
}

// 要件31・「CSV の誤りの文言」の前置き: ファイルの段階の誤りが2つ重なったときは、表の上の誤りだけを出す
// （引数 → ファイルを開く → 利用者 → 文字コード → ダブルクォート → 見出しの順）。何も変わらない。
func Test要件31_ファイルの段階の誤りが重なると表の上の誤りだけを出して何も変えない(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "no-such-managed.csv")
	cases := []struct {
		name   string
		path   string // body が nil のとき、Run にそのまま渡すパス
		body   []byte // nil でなければ、これをファイルにして、そのパスを渡す
		userID string
		want   string
	}{
		{"引数の不足が、ファイルを開けないことより先", missing, nil, "", importMissingArgsMsg},
		{"ファイルを開けないことが、利用者がいないことより先", missing, nil, unknownUserID, "ファイル " + missing + " を開けません"},
		{"利用者がいないことが、文字コードより先", "", []byte(managedShiftJISHeader + managedShiftJISRow), unknownUserID, "利用者 " + unknownUserID + " が見つかりません"},
		{"文字コードが、見出しより先", "", []byte(managedShiftJISHeaderSwapped + managedShiftJISRow), seedUserA, importEncodingMsg},
		{"文字コードが、ダブルクォートより先", "", []byte(managedShiftJISHeader + `"` + managedShiftJISRow), seedUserA, importEncodingMsg},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := givenManagedPool(t)
			before := snapshotManagedAndFive(t, pool)
			path := tc.path
			if tc.body != nil {
				path = writeManagedCSV(t, tc.body)
			}

			out := runManagedImport(t, pool, path, tc.userID)

			assertImportFailed(t, out, tc.want)
			assertManagedAndFiveUnchanged(t, pool, before)
		})
	}
}

// 要件26: 名前に NUL（`Wealth\x00Navi`）を含む正しい CSV は、DB が受け付けないので、終了コード1・標準エラーの先頭が
// 「取り込めませんでした。何も変えていません。」で、何も変わらない。誤りの理由の文言は確かめない（DB のエラーの文のため）。
func Test要件26_名前にNULを含むCSVは取り込めず何も変えない(t *testing.T) {
	t.Parallel()
	pool := givenManagedPool(t)
	before := snapshotManagedAndFive(t, pool)

	out := runManagedImportCSV(t, pool, seedUserA, managedCSV("Wealth\x00Navi,2026-11-16,4000000,1000000"))

	if out.code != 1 {
		t.Errorf("終了コード: got %d, want 1\nstdout: %q\nstderr: %q", out.code, out.stdout, out.stderr)
	}
	if len(out.stderr) == 0 || out.stderr[0] != importFailedHeadline {
		t.Errorf("標準エラーの先頭: got %q, want %q", out.stderr, importFailedHeadline)
	}
	if len(out.stdout) != 0 {
		t.Errorf("標準出力の行: got %q, want なし", out.stdout)
	}
	assertManagedAndFiveUnchanged(t, pool, before)
}

// 5.2: UUID として読めない USER_ID（`abc`）は「利用者 abc が見つかりません」。何も変わらない。
func Test要件31_UUIDとして読めないUSER_IDは利用者が見つからない誤りにする(t *testing.T) {
	t.Parallel()
	pool := givenManagedPool(t)
	before := snapshotManagedAndFive(t, pool)
	path := writeManagedCSV(t, managedCSV("WealthNavi,2026-11-16,4000000,1000000"))

	out := runManagedImport(t, pool, path, "abc")

	assertImportFailed(t, out, "利用者 abc が見つかりません")
	assertManagedAndFiveUnchanged(t, pool, before)
}

// 基準24（要件31）: Aが WealthNavi（記録1件）を持つ。CSV の3行目は評価額が `0`、5行目は日付が `2026-02-30` で損益が `abc`、ほかの行は正しい
// → 標準エラーは「取り込めませんでした。何も変えていません。」「3行目: 評価額は1以上にしてください」「5行目: 日付は…」「5行目: 損益は…」がこの順、
// 標準出力は空、終了コード1。Aの記録は1件のまま（口座の2つの表と5つの表は変わらない）。
func Test基準24_すべての行のすべての誤りを行番号の順に出して何も変えない(t *testing.T) {
	t.Parallel()
	pool := givenManagedPool(t)
	before := snapshotManagedAndFive(t, pool)

	out := runManagedImportCSV(t, pool, seedUserA, managedCSV(
		"WealthNavi,2026-11-16,4000000,1000000", // 2行目
		"WealthNavi,2026-11-15,0,0",             // 3行目: 評価額 0
		"WealthNavi,2026-11-14,4000000,1000000", // 4行目
		"WealthNavi,2026-02-30,4000000,abc",     // 5行目: 日付と損益
		"WealthNavi,2026-11-13,4000000,1000000", // 6行目
	))

	assertImportFailed(t, out,
		"3行目: "+managedValuationZeroMsg,
		"5行目: "+managedDateMessage,
		"5行目: "+managedGainMessage,
	)
	wantA := []storedManagedValuation{{"WealthNavi", "2026-11-15", 4_000_000, 1_000_000}}
	if got := readManagedValuations(t, pool, seedUserA); !slices.Equal(got, wantA) {
		t.Errorf("Aの記録:\n got %+v\nwant %+v（1件のまま）", got, wantA)
	}
	assertManagedAndFiveUnchanged(t, pool, before)
}

// 基準25（要件30）: CSV の2行目が「WealthNavi,2026-11-16,4000000,1000000」、3行目は別の正しい行、4行目が「WealthNavi,2026/11/16,4100000,1100000」
// → 標準エラーは先頭の行と「4行目: WealthNavi の 2026-11-16 の記録が、2行目と重なっています」の2行、終了コード1、何も変わらない。
func Test基準25_同じ名前と同じ日付の行が重なるときは誤りにして何も変えない(t *testing.T) {
	t.Parallel()
	pool := givenManagedPool(t)
	before := snapshotManagedAndFive(t, pool)

	out := runManagedImportCSV(t, pool, seedUserA, managedCSV(
		"WealthNavi,2026-11-16,4000000,1000000",
		"WealthNavi,2026-10-31,3950000,950000",
		"WealthNavi,2026/11/16,4100000,1100000",
	))

	assertImportFailed(t, out, "4行目: WealthNavi の 2026-11-16 の記録が、2行目と重なっています")
	assertManagedAndFiveUnchanged(t, pool, before)
}

// 5.2・「CSV の誤りの文言」の前置き: 各行の誤りと重なりが両方ある CSV は、各行の誤りだけを出す（前の段階に誤りがあれば次の段階は確かめない）。
func Test要件31_各行の誤りがあるときは重なりの誤りを出さない(t *testing.T) {
	t.Parallel()
	pool := givenManagedPool(t)
	before := snapshotManagedAndFive(t, pool)

	out := runManagedImportCSV(t, pool, seedUserA, managedCSV(
		"WealthNavi,2026-11-16,4000000,1000000", // 2行目
		"WealthNavi,2026/11/16,4100000,1100000", // 3行目: 2行目と重なる
		"Alpha,2026-11-16,0,0",                  // 4行目: 評価額 0
	))

	assertImportFailed(t, out, "4行目: "+managedValuationZeroMsg)
	assertManagedAndFiveUnchanged(t, pool, before)
}

// 要件1・5.2: 名前の前後の空白は取り除かない。` WealthNavi` と `WealthNavi` の行は、別の口座になる。
func Test要件1_名前の前後の空白を取り除かず別の口座にする(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)

	out := runManagedImportCSV(t, pool, seedUserA, managedCSV(
		" WealthNavi,2026-11-16,100000,0",
		"WealthNavi,2026-11-16,200000,0",
		"WealthNavi ,2026-11-16,300000,0",
	))

	assertImportSucceeded(t, out, managedRegisteredMessage(3))
	wantNames := []string{" WealthNavi", "WealthNavi", "WealthNavi "}
	if got := readManagedAccountNames(t, pool, seedUserA); !slices.Equal(got, wantNames) {
		t.Errorf("Aの口座: got %q, want %q（前後の空白もそのまま別の名前）", got, wantNames)
	}
}

// 要件26・5.2: 入れ替えの途中で失敗したら何も変えない。ReplaceManagedAccounts に同じ名前・同じ日付の2件を直接渡す（Parse を通さない）と、
// エラーが返り、前からあったAの口座と記録（作成日時を含む）が残る。Bの口座と記録、5つの表も同じ。
func Test要件26_入れ替えの途中で失敗したら前の口座と記録が残る(t *testing.T) {
	t.Parallel()
	pool := givenManagedPool(t)
	before := snapshotManagedAndFive(t, pool)
	repo := write.NewManagedAccountRepository(pool)
	userA := uuid.MustParse(seedUserA)

	err := repo.ReplaceManagedAccounts(t.Context(), userA, []model.ManagedValuation{
		{AccountName: "Alpha", ValuationDate: "2026-11-01", Valuation: 500_000, Gain: -20_000},
		{AccountName: "Alpha", ValuationDate: "2026-11-01", Valuation: 600_000, Gain: 0}, // 同じ名前・同じ日付
	})

	if err == nil {
		t.Fatal("ReplaceManagedAccounts: got err nil, want エラー（同じ名前・同じ日付の2件）")
	}
	wantA := []storedManagedValuation{{"WealthNavi", "2026-11-15", 4_000_000, 1_000_000}}
	if got := readManagedValuations(t, pool, seedUserA); !slices.Equal(got, wantA) {
		t.Errorf("Aの記録:\n got %+v\nwant %+v（前からあった記録が残る）", got, wantA)
	}
	assertManagedAndFiveUnchanged(t, pool, before)
}

// 要件26・5.2: Execute に空の valuations を渡しても（nil も空のスライスも）、エラーで、前からあった口座と記録が残る（全部消すだけの取り込みにしない）。
func Test要件26_Executeに空の記録を渡すとエラーで前の口座と記録が残る(t *testing.T) {
	t.Parallel()
	pool := givenManagedPool(t)
	before := snapshotManagedAndFive(t, pool)
	importer := newManagedImporter(pool)
	userA := uuid.MustParse(seedUserA)

	for name, valuations := range map[string][]model.ManagedValuation{"nil": nil, "空のスライス": {}} {
		n, err := importer.Execute(t.Context(), userA, valuations)
		if err == nil {
			t.Errorf("Execute(%s): got err nil (件数 %d), want エラー", name, n)
		}
	}
	assertManagedAndFiveUnchanged(t, pool, before)
}

// 4.8: Execute は、入れた記録の件数を返す。CheckUser は、DB にいる利用者なら nil、いない利用者なら今の ErrUserNotFound を返す。
func Test要件26_ExecuteとCheckUserの返り値(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	importer := newManagedImporter(pool)

	if err := importer.CheckUser(t.Context(), uuid.MustParse(seedUserA)); err != nil {
		t.Errorf("CheckUser(A): got %v, want nil", err)
	}
	if err := importer.CheckUser(t.Context(), uuid.MustParse(unknownUserID)); !errors.Is(err, command.ErrUserNotFound) {
		t.Errorf("CheckUser(いない利用者): got %v, want ErrUserNotFound", err)
	}

	n, err := importer.Execute(t.Context(), uuid.MustParse(seedUserA), []model.ManagedValuation{
		{AccountName: "WealthNavi", ValuationDate: "2026-10-31", Valuation: 3_950_000, Gain: 950_000},
		{AccountName: "WealthNavi", ValuationDate: "2026-11-16", Valuation: 4_000_000, Gain: 1_000_000},
		{AccountName: "Alpha", ValuationDate: "2026-11-01", Valuation: 500_000, Gain: -20_000},
	})
	if err != nil || n != 3 {
		t.Fatalf("Execute: got (%d, %v), want (3, nil)", n, err)
	}
	wantRows := []storedManagedValuation{
		{"Alpha", "2026-11-01", 500_000, -20_000},
		{"WealthNavi", "2026-10-31", 3_950_000, 950_000},
		{"WealthNavi", "2026-11-16", 4_000_000, 1_000_000},
	}
	if got := readManagedValuations(t, pool, seedUserA); !slices.Equal(got, wantRows) {
		t.Errorf("Aの記録:\n got %+v\nwant %+v", got, wantRows)
	}
}

// 基準28（非機能2）: Aの口座10・記録各120件（入れ替えの削除も測るため、前提に SQL で入れておく）。口座10・記録各120件（1,200行。月に1回で10年分）の正しい CSV の Run が、
// 10秒以内に終了コード0で終わり、記録が1,200件（前提の口座は入れ替わってなくなる）。
// 測るのは Run の呼び出しだけ（ビルドとマイグレーションは含めない）。測った値を t.Log に出す。t.Parallel は使わない（ほかのテストと時間を取り合わないため）。
func Test基準28_1200行のCSVの取り込みが10秒以内に終わる(t *testing.T) {
	pool := testdb.New(t)
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO managed_accounts (user_id, name) SELECT $1::uuid, 'Old' || lpad(a::text, 2, '0') FROM generate_series(1, 10) a", seedUserA); err != nil {
		t.Fatalf("前提の口座を入れられない: %v", err)
	}
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO managed_account_valuations (user_id, name, valuation_date, valuation, gain)
		 SELECT $1::uuid, 'Old' || lpad(a::text, 2, '0'), date '2016-12-01' + (m * interval '1 month'), 1000000 + m, m
		   FROM generate_series(1, 10) a, generate_series(0, 119) m`, seedUserA); err != nil {
		t.Fatalf("前提の記録を入れられない: %v", err)
	}
	if got := readManagedValuations(t, pool, seedUserA); len(got) != 1200 {
		t.Fatalf("前提: Aの記録が 1200 件のはずが %d 件", len(got))
	}

	// 2016-12-01 から月に1回、2026-11-01 までの120か月。
	var lines []string
	for a := range 10 {
		for m := range 120 {
			date := time.Date(2016, time.Month(12+m), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
			lines = append(lines, fmt.Sprintf("Acct%02d,%s,%d,%d", a, date, 1_000_000+m*1000, m*10-50))
		}
	}
	path := writeManagedCSV(t, managedCSV(lines...))

	start := time.Now()
	out := runManagedImport(t, pool, path, seedUserA)
	elapsed := time.Since(start)

	t.Logf("1,200行の取り込み（Run の呼び出しだけ）: %v", elapsed)
	if out.code != 0 {
		t.Fatalf("終了コード: got %d, want 0\nstderr: %q", out.code, out.stderr)
	}
	if len(out.stdout) == 0 || out.stdout[0] != managedRegisteredMessage(1200) {
		t.Errorf("標準出力の1行目: got %q, want %q", out.stdout, managedRegisteredMessage(1200))
	}
	if got := readManagedValuations(t, pool, seedUserA); len(got) != 1200 {
		t.Errorf("Aの記録の件数: got %d, want 1200", len(got))
	}
	if got := readManagedAccountNames(t, pool, seedUserA); len(got) != 10 || got[0] != "Acct00" {
		t.Errorf("Aの口座: got %q, want Acct00〜Acct09 の10個（前提の Old01〜Old10 は入れ替わってなくなる）", got)
	}
	if elapsed >= 10*time.Second {
		t.Errorf("1,200行の取り込みの時間: got %v, want 10秒未満", elapsed)
	}
}
