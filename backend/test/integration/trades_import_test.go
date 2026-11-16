package integration

import (
	"errors"
	"fmt"
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
	"github.com/tomo-shibata/zero/backend/internal/interface/tradecsv"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 要件（docs/requirements/trade-history.md）の要件3・9・19〜26・非機能2 と「CSV の誤りの文言」の表、基準22〜30・29a・32 と、
// プラン（docs/plans/trade-history/plan.md）4.4（取り込みの流れ）、4.6・4.7（PR③の境界）、5.1 の PR③ の行、5.3・5.5 の PR③。
// 基準21（task を通した取り込み）は trades_import_task_test.go。ここは tradecsv.Run を testdb で直接呼ぶ（判断12）。
//
// Run の約束（4.6・4.7）: 終了コードは成功0・誤り1。成功の表示は標準出力、誤りの表示は標準エラーに1行ずつ書く。
// 誤りのときの標準エラーは、先頭が「取り込めませんでした。何も変えていません。」、続けて誤りの文言（要件21。ファイルの段階の誤りも同じ）。
// 「今日」は 2026-11-16（日本時間）。銘柄マスタは test.sql の 7203・6758・9984（どれも円）。AAPL・MSFT・GOOGL は入っていない。

// 基準22（要件20）: Aの売買履歴が 7203 の3件、Bの売買履歴が 6758 の1件
// → 「6758,ソニーグループ,JPY,買,3200,10,2026-01-10」だけの CSV をAに取り込むと、Aの売買履歴はその1件だけになり、Bの売買履歴は1件のまま。
func Test基準22_取り込みはその利用者の売買履歴だけを入れ替える(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "2000", "2026-01-10")
	insertTrade(t, pool, seedUserA, "7203", "buy", 50, "2600", "2026-02-10")
	insertTrade(t, pool, seedUserA, "7203", "sell", 20, "2700", "2026-03-10")
	insertTrade(t, pool, seedUserB, "6758", "buy", 5, "3000", "2026-01-05")
	bBefore := importTradesOf(t, pool, seedUserB)

	out := runImportCSV(t, pool, seedUserA, importCSV("6758,ソニーグループ,JPY,買,3200,10,2026-01-10"))

	assertImportSucceeded(t, out, "1件の売買履歴を取り込みました。")
	gotA := importTradesOf(t, pool, seedUserA)
	if len(gotA) != 1 {
		t.Fatalf("Aの売買履歴の件数: got %d (%+v), want 1", len(gotA), gotA)
	}
	if a := gotA[0]; a.stockCode != "6758" || a.side != "buy" || a.quantity != 10 || a.priceHundredths != 320000 || a.tradeDate != "2026-01-10" {
		t.Errorf("Aの売買履歴: got %+v, want 6758・buy・10株・320000（3,200円）・2026-01-10", a)
	}
	if gotB := importTradesOf(t, pool, seedUserB); !slices.Equal(gotB, bBefore) {
		t.Errorf("Bの売買履歴が変わった: before %+v, after %+v", bBefore, gotB)
	}
}

// 基準23（要件22）: BOM 付き・改行 CRLF の CSV で、銘柄名を `"Alphabet Inc., Class A"` と書いた GOOGL の行と、空の行を含む → 成功し、
// 銘柄マスタの GOOGL の銘柄名は `Alphabet Inc., Class A`（USD）になる。
func Test基準23_BOM付きCRLFで囲んだ銘柄名と空の行を含むCSVを取り込める(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	content := []byte("\ufeff" + importHeader + "\r\n" +
		`GOOGL,"Alphabet Inc., Class A",USD,買,150.5,10,2026-01-10` + "\r\n" +
		"\r\n" +
		"7203,トヨタ自動車,JPY,買,2500,100,2026-01-10\r\n" +
		"\r\n")

	out := runImportCSV(t, pool, seedUserA, content)

	assertImportSucceeded(t, out, "2件の売買履歴を取り込みました。", "新しく登録した銘柄: GOOGL")
	got, ok := importStockByCode(t, pool, "GOOGL")
	if !ok {
		t.Fatal("銘柄マスタに GOOGL がない")
	}
	if want := (importStockRow{code: "GOOGL", name: "Alphabet Inc., Class A", currency: "USD"}); got != want {
		t.Errorf("銘柄マスタの GOOGL: got %+v, want %+v", got, want)
	}
	// 売買履歴は CSV の行の順に2件（空の行は読み飛ばす）。
	gotA := importTradesOf(t, pool, seedUserA)
	if len(gotA) != 2 || gotA[0].stockCode != "GOOGL" || gotA[0].priceHundredths != 15050 || gotA[1].stockCode != "7203" {
		t.Errorf("Aの売買履歴: got %+v, want GOOGL（15050）→ 7203 の2件", gotA)
	}
}

// 基準24（要件21・要件23）: Aの売買履歴が 7203 の1件 → (a) 〜 (g) のどれで取り込んでも、「CSV の誤りの文言」のファイルの段階の文言が1つだけ
// 標準エラーに出て、終了コードは0以外、Aの売買履歴は1件のまま、銘柄マスタも変わらない。
func Test基準24_ファイルの段階の誤りは文言を1つだけ出して何も変えない(t *testing.T) {
	t.Parallel()
	validCSV := importCSV("7203,トヨタ自動車,JPY,買,2500,100,2026-01-10")
	cases := []struct {
		name string
		// run は、取り込みを実行して結果と、期待する誤りの文言（「取り込めませんでした…」の後の行）を返す。
		run func(t *testing.T, pool *pgxpool.Pool) (importOutcome, string)
	}{
		{"a_USER_IDなし", func(t *testing.T, pool *pgxpool.Pool) (importOutcome, string) {
			t.Helper()
			return runImport(t, pool, writeImportCSV(t, validCSV), ""), importMissingArgsMsg
		}},
		{"a_CSV_FILEなし", func(t *testing.T, pool *pgxpool.Pool) (importOutcome, string) {
			t.Helper()
			return runImport(t, pool, "", seedUserA), importMissingArgsMsg
		}},
		{"a_どちらもなし", func(t *testing.T, pool *pgxpool.Pool) (importOutcome, string) {
			t.Helper()
			return runImport(t, pool, "", ""), importMissingArgsMsg
		}},
		{"b_存在しないファイル", func(t *testing.T, pool *pgxpool.Pool) (importOutcome, string) {
			t.Helper()
			missing := filepath.Join(t.TempDir(), "missing.csv")
			return runImport(t, pool, missing, seedUserA), fmt.Sprintf("ファイル %s を開けません", missing)
		}},
		{"c_DBにいない利用者", func(t *testing.T, pool *pgxpool.Pool) (importOutcome, string) {
			t.Helper()
			return runImport(t, pool, writeImportCSV(t, validCSV), unknownUserID), fmt.Sprintf("利用者 %s が見つかりません", unknownUserID)
		}},
		{"c_UUIDとして読めない利用者", func(t *testing.T, pool *pgxpool.Pool) (importOutcome, string) {
			t.Helper()
			return runImport(t, pool, writeImportCSV(t, validCSV), "not-a-uuid"), "利用者 not-a-uuid が見つかりません"
		}},
		{"d_Shift_JIS", func(t *testing.T, pool *pgxpool.Pool) (importOutcome, string) {
			t.Helper()
			return runImportCSV(t, pool, seedUserA, []byte(shiftJISHeader+shiftJISRow)), importEncodingMsg
		}},
		{"e_見出しの順番違い", func(t *testing.T, pool *pgxpool.Pool) (importOutcome, string) {
			t.Helper()
			content := []byte("銘柄コード,銘柄名,売買,通貨,価格,株数,取引日\n7203,トヨタ自動車,買,JPY,2500,100,2026-01-10\n")
			return runImportCSV(t, pool, seedUserA, content), importHeaderMessage
		}},
		{"f_見出しだけ", func(t *testing.T, pool *pgxpool.Pool) (importOutcome, string) {
			t.Helper()
			return runImportCSV(t, pool, seedUserA, importCSV()), importNoRowsMessage
		}},
		{"f_見出しと空の行だけ", func(t *testing.T, pool *pgxpool.Pool) (importOutcome, string) {
			t.Helper()
			return runImportCSV(t, pool, seedUserA, []byte(importHeader+"\r\n\r\n\r\n")), importNoRowsMessage
		}},
		{"g_3行目の銘柄名が閉じていない", func(t *testing.T, pool *pgxpool.Pool) (importOutcome, string) {
			t.Helper()
			content := importCSV(
				"7203,トヨタ自動車,JPY,買,2500,100,2026-01-10",
				`AAPL,"Apple,USD,買,150.25,10,2026-01-10`,
			)
			return runImportCSV(t, pool, seedUserA, content), "3行目: ダブルクォートの使い方が正しくありません"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := givenImportPool(t)
			before := snapshotImportDB(t, pool)

			out, wantMsg := tc.run(t, pool)

			assertImportFailed(t, out, wantMsg)
			assertDBUnchanged(t, pool, before)
			if got := importTradesOf(t, pool, seedUserA); len(got) != 1 {
				t.Errorf("Aの売買履歴の件数: got %d, want 1（取り込めなかったので元のまま）", len(got))
			}
		})
	}
}

// 5.5「ファイルの段階の順番」: 存在しないファイル＋いない利用者 → ファイルの文言だけ（表の上から順に確かめ、最初に当たった1つだけ）。
func Test基準24_ファイルを開けないことを利用者がいないことより先に確かめる(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	before := snapshotImportDB(t, pool)
	missing := filepath.Join(t.TempDir(), "missing.csv")

	out := runImport(t, pool, missing, unknownUserID)

	assertImportFailed(t, out, fmt.Sprintf("ファイル %s を開けません", missing))
	assertDBUnchanged(t, pool, before)
}

// 5.5「ファイルの段階の順番」: 利用者がいない＋Shift_JIS → 利用者の文言だけ（利用者の確かめは、文字コードより先）。
func Test基準24_利用者がいないことを文字コードより先に確かめる(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	before := snapshotImportDB(t, pool)

	out := runImportCSV(t, pool, unknownUserID, []byte(shiftJISHeader+shiftJISRow))

	assertImportFailed(t, out, fmt.Sprintf("利用者 %s が見つかりません", unknownUserID))
	assertDBUnchanged(t, pool, before)
}

// 5.5「ファイルの段階の順番」: Shift_JIS＋見出し違い → 文字コードの文言だけ。
func Test基準24_文字コードを見出しより先に確かめる(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	before := snapshotImportDB(t, pool)

	out := runImportCSV(t, pool, seedUserA, []byte(shiftJISHeaderSwapped+shiftJISRow))

	assertImportFailed(t, out, importEncodingMsg)
	assertDBUnchanged(t, pool, before)
}

// 5.5「ファイルの段階の順番」: 見出しが違い、見出しの後に行がない → 見出しの文言だけ（「取り込む行がありません」は出さない）。
func Test基準24_見出しを取り込む行がないことより先に確かめる(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	before := snapshotImportDB(t, pool)

	out := runImportCSV(t, pool, seedUserA, []byte("銘柄コード,銘柄名,売買,通貨,価格,株数,取引日\n"))

	assertImportFailed(t, out, importHeaderMessage)
	assertDBUnchanged(t, pool, before)
}

// 5.5「ファイルの段階の順番」: USER_ID が空で、存在しないファイル → 引数の文言だけ（引数の確かめが、ファイルを開くことより先）。
func Test基準24_引数の不足をファイルを開けないことより先に確かめる(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	before := snapshotImportDB(t, pool)
	missing := filepath.Join(t.TempDir(), "missing.csv")

	out := runImport(t, pool, missing, "")

	assertImportFailed(t, out, importMissingArgsMsg)
	assertDBUnchanged(t, pool, before)
}

// 5.5「ファイルの段階の順番」: Shift_JIS の行に閉じていない `"` がある CSV → 文字コードの文言だけ（文字コードの確かめが、ダブルクォートより先）。
func Test基準24_文字コードをダブルクォートより先に確かめる(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	before := snapshotImportDB(t, pool)

	out := runImportCSV(t, pool, seedUserA, []byte(shiftJISHeader+`"`+shiftJISRow))

	assertImportFailed(t, out, importEncodingMsg)
	assertDBUnchanged(t, pool, before)
}

// 5.5「ダブルクォートの誤りの順番」: 見出しが違い、3行目のダブルクォートが閉じていない → ダブルクォートの文言だけ（表の順。ダブルクォートが先）。
func Test質問9_ダブルクォートの誤りを見出しの誤りより先に出す(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	before := snapshotImportDB(t, pool)
	content := []byte("銘柄コード,銘柄名,売買,通貨,価格,株数,取引日\n" +
		"7203,トヨタ自動車,買,JPY,2500,100,2026-01-10\n" +
		`AAPL,"Apple,買,USD,150.25,10,2026-01-10` + "\n")

	out := runImportCSV(t, pool, seedUserA, content)

	assertImportFailed(t, out, "3行目: ダブルクォートの使い方が正しくありません")
	assertDBUnchanged(t, pool, before)
}

// 5.5: 1行目の見出しのダブルクォートが閉じていない → 見出しの文言ではなく「1行目: ダブルクォートの使い方が正しくありません」だけ。
func Test質問9_見出しのダブルクォートの誤りは見出しの文言にしない(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	before := snapshotImportDB(t, pool)
	content := []byte(`"銘柄コード,銘柄名,通貨,売買,価格,株数,取引日` + "\n" + "7203,トヨタ自動車,JPY,買,2500,100,2026-01-10\n")

	out := runImportCSV(t, pool, seedUserA, content)

	assertImportFailed(t, out, "1行目: ダブルクォートの使い方が正しくありません")
	assertDBUnchanged(t, pool, before)
}

// 5.5: 2行目の価格が abc、4行目の銘柄名が `"Apple`（閉じていない）→「4行目: ダブルクォートの使い方が正しくありません」だけ
// （ダブルクォートの誤りは段階1なので、段階2の各行の誤りは出さない）。
func Test質問9_ダブルクォートの誤りがあれば各行の誤りは出さない(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	before := snapshotImportDB(t, pool)
	content := importCSV(
		"7203,トヨタ自動車,JPY,買,abc,100,2026-01-10",   // 2行目: 価格が abc
		"6758,ソニーグループ,JPY,買,3200,10,2026-01-10",  // 3行目
		`AAPL,"Apple,USD,買,150.25,10,2026-01-10`, // 4行目: 閉じていない
	)

	out := runImportCSV(t, pool, seedUserA, content)

	assertImportFailed(t, out, "4行目: ダブルクォートの使い方が正しくありません")
	assertDBUnchanged(t, pool, before)
}

// 5.5「売買の行が始まる行」: 2行目の銘柄名が `"Apple` と改行と `Inc."` の2行にまたがり、同じ売買の3行目の後ろの列に囲んでいない `"` がある CSV
// → 「2行目: ダブルクォートの使い方が正しくありません」だけ（売買の行が始まる行）。
func Test質問9_ダブルクォートの誤りは売買の行が始まる行で出す(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	before := snapshotImportDB(t, pool)
	content := []byte(importHeader + "\n" +
		`AAPL,"Apple` + "\n" +
		`Inc.",USD,買,150.25,10,20"26-01-10` + "\n")

	out := runImportCSV(t, pool, seedUserA, content)

	assertImportFailed(t, out, "2行目: ダブルクォートの使い方が正しくありません")
	assertDBUnchanged(t, pool, before)
}

// 基準26（要件21）: Aの売買履歴が 7203 の1件。CSV の3行目は株数が `0`、5行目は取引日が `2026-02-30` で価格が `abc`、ほかの行は正しい
// → 標準エラーに「取り込めませんでした。何も変えていません。」「3行目: 株数は…」「5行目: 価格は…」「5行目: 取引日は…」がこの順に出て、
// 終了コードは0以外、Aの売買履歴は1件のまま。
func Test基準26_各行の誤りをすべて行番号の順に出して何も変えない(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	before := snapshotImportDB(t, pool)
	content := importCSV(
		"7203,トヨタ自動車,JPY,買,2500,100,2026-01-10", // 2行目: 正しい
		"6758,ソニーグループ,JPY,買,3200,0,2026-01-10",  // 3行目: 株数が 0
		"AAPL,Apple,USD,買,150.25,10,2026-01-10", // 4行目: 正しい
		"MSFT,Microsoft,USD,買,abc,5,2026-02-30", // 5行目: 価格が abc・取引日が実在しない
		"7203,トヨタ自動車,JPY,買,2600,50,2026-01-12",  // 6行目: 正しい
	)

	out := runImportCSV(t, pool, seedUserA, content)

	assertImportFailed(t, out,
		"3行目: 株数は1以上の整数にしてください",
		"5行目: 価格は半角数字と小数点で書いてください",
		"5行目: 取引日は YYYY-MM-DD か YYYY/M/D の実在する日付にしてください",
	)
	assertDBUnchanged(t, pool, before)
	if got := importTradesOf(t, pool, seedUserA); len(got) != 1 {
		t.Errorf("Aの売買履歴の件数: got %d, want 1", len(got))
	}
}

// 5.5「段階2 があれば段階3 を出さない」: 3行目が株数 `0`、4行目が多すぎる売りの CSV → 標準エラーは「3行目: 株数は…」だけ。
func Test基準26_各行の誤りがあれば行どうしの誤りは出さない(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	before := snapshotImportDB(t, pool)
	content := importCSV(
		"7203,トヨタ自動車,JPY,買,2500,100,2026-01-10", // 2行目
		"7203,トヨタ自動車,JPY,買,2500,0,2026-01-11",   // 3行目: 株数が 0
		"7203,トヨタ自動車,JPY,売,3000,150,2026-02-10", // 4行目: 多すぎる売り
	)

	out := runImportCSV(t, pool, seedUserA, content)

	assertImportFailed(t, out, "3行目: 株数は1以上の整数にしてください")
	assertDBUnchanged(t, pool, before)
}

// 基準27（要件9・要件21）: 2行目が「7203,トヨタ自動車,JPY,買,2500,100,2026-01-10」、3行目が「7203,トヨタ自動車,JPY,売,3000,150,2026-02-10」
// → 「3行目: 売る株数（150株）が、その時点の保有数量（100株）より多いです」が出て、何も変わらない（5.5: 前提のAの 7203 の1件と、銘柄マスタが元のまま）。
func Test基準27_持っている株数より多い売りは誤りにして何も変えない(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	before := snapshotImportDB(t, pool)
	content := importCSV(
		"7203,トヨタ自動車,JPY,買,2500,100,2026-01-10",
		"7203,トヨタ自動車,JPY,売,3000,150,2026-02-10",
	)

	out := runImportCSV(t, pool, seedUserA, content)

	assertImportFailed(t, out, "3行目: 売る株数（150株）が、その時点の保有数量（100株）より多いです")
	assertDBUnchanged(t, pool, before)
	if _, ok := importStockByCode(t, pool, "AAPL"); ok {
		t.Error("銘柄マスタに AAPL が足された（取り込めなかったので何も変えない）")
	}
}

// 4.7「多すぎる売りの文言の株数は、3桁区切りを入れない数字のまま」: 売る株数が 1500株・保有数量が 1000株のとき、
// 「売る株数（1500株）が、その時点の保有数量（1000株）より多いです」。
func Test基準27_多すぎる売りの株数は3桁区切りを入れない(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	content := importCSV(
		"7203,トヨタ自動車,JPY,買,2500,1000,2026-01-10",
		"7203,トヨタ自動車,JPY,売,3000,1500,2026-02-10",
	)

	out := runImportCSV(t, pool, seedUserA, content)

	assertImportFailed(t, out, "3行目: 売る株数（1500株）が、その時点の保有数量（1000株）より多いです")
}

// 基準28（要件8・要件9）: 2行目が売り、3行目が買い（取引日の順と行の順が逆）→ 成功する（取引日の順にたどるので、売りの時点で100株持っている）。
// 売買履歴は CSV の行の順に登録される（行の順が trades.id の順）。
func Test基準28_取引日の順にたどるので行の順が逆でも取り込める(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	content := importCSV(
		"7203,トヨタ自動車,JPY,売,3000,100,2026-02-10",
		"7203,トヨタ自動車,JPY,買,2500,100,2026-01-10",
	)

	out := runImportCSV(t, pool, seedUserA, content)

	assertImportSucceeded(t, out, "2件の売買履歴を取り込みました。")
	got := importTradesOf(t, pool, seedUserA)
	if len(got) != 2 || got[0].side != "sell" || got[1].side != "buy" {
		t.Errorf("Aの売買履歴（登録した順）: got %+v, want 売り → 買い（CSV の行の順）", got)
	}
}

// 基準29（要件25）(a): 銘柄マスタに 7203（トヨタ自動車・JPY）がある。2行目が「AAPL,Apple,USD,…」、4行目が「AAPL,Apple Inc.,USD,…」
// → 「4行目: 銘柄コード AAPL の銘柄名か通貨が、2行目と違います」が出て、何も変わらない。
func Test基準29_a_同じ銘柄コードの銘柄名が行どうしで違えば誤りにする(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	before := snapshotImportDB(t, pool)
	content := importCSV(
		"AAPL,Apple,USD,買,150.25,10,2026-01-10",
		"7203,トヨタ自動車,JPY,買,2500,100,2026-01-10",
		"AAPL,Apple Inc.,USD,買,160.00,10,2026-01-11",
	)

	out := runImportCSV(t, pool, seedUserA, content)

	assertImportFailed(t, out, "4行目: 銘柄コード AAPL の銘柄名か通貨が、2行目と違います")
	assertDBUnchanged(t, pool, before)
	if _, ok := importStockByCode(t, pool, "AAPL"); ok {
		t.Error("銘柄マスタに AAPL が足された（取り込めなかったので何も変えない）")
	}
}

// 基準29（要件25）: CSV の中で通貨だけが違う行（2行目 USD、3行目 JPY。銘柄名は同じ）も、銘柄名か通貨が違う行として誤りにする。何も変わらない。
func Test基準29_a_同じ銘柄コードの通貨が行どうしで違えば誤りにする(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	before := snapshotImportDB(t, pool)
	content := importCSV(
		"AAPL,Apple,USD,買,150,10,2026-01-05",
		"AAPL,Apple,JPY,買,150,10,2026-01-06",
	)

	out := runImportCSV(t, pool, seedUserA, content)

	assertImportFailed(t, out, "3行目: 銘柄コード AAPL の銘柄名か通貨が、2行目と違います")
	assertDBUnchanged(t, pool, before)
}

// 要件3・要件21（今日の決め方）: Run は RunInput.Today を「今日」として使う。Today を 2026-01-01 にすると、取引日 2026-06-01 は未来の取引日になり、何も変わらない。
func Test要件3_RunはRunInputのTodayを今日として使う(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	before := snapshotImportDB(t, pool)
	path := writeImportCSV(t, importCSV("7203,トヨタ自動車,JPY,買,2500,100,2026-06-01"))

	var stdout, stderr strings.Builder
	code := tradecsv.Run(t.Context(), tradecsv.RunInput{
		CSVPath:  path,
		UserID:   seedUserA,
		Today:    "2026-01-01",
		Importer: newImporter(pool),
	}, &stdout, &stderr)
	out := importOutcome{code: code, stdout: splitOutputLines(stdout.String()), stderr: splitOutputLines(stderr.String())}

	assertImportFailed(t, out, "2行目: 未来の取引日は取り込めません")
	assertDBUnchanged(t, pool, before)
}

// 基準29（要件25）(b): 銘柄マスタに 7203（トヨタ自動車・JPY）がある。2行目が「7203,トヨタ自動車,USD,…」
// → 「2行目: 銘柄コード 7203 は銘柄マスタに JPY で登録されています」が出て、何も変わらない（銘柄マスタの 7203 は JPY のまま）。
func Test基準29_b_銘柄マスタと通貨が違えば誤りにする(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	before := snapshotImportDB(t, pool)
	content := importCSV("7203,トヨタ自動車,USD,買,150,10,2026-01-10")

	out := runImportCSV(t, pool, seedUserA, content)

	assertImportFailed(t, out, "2行目: 銘柄コード 7203 は銘柄マスタに JPY で登録されています")
	assertDBUnchanged(t, pool, before)
	got, ok := importStockByCode(t, pool, "7203")
	if !ok || got.currency != "JPY" || got.name != "トヨタ自動車" {
		t.Errorf("銘柄マスタの 7203: got %+v (ok=%v), want トヨタ自動車・JPY のまま", got, ok)
	}
}

// 基準29a（要件20・要件25）: 銘柄マスタに 7203（トヨタ自動車・JPY）があり、Bが 7203 を持つ。「7203,トヨタ,JPY,買,2500,100,2026-01-10」だけの CSV をAに取り込む
// → 成功し、「1件の売買履歴を取り込みました。」「銘柄名を変えた銘柄: 7203」が出て、銘柄マスタの 7203 は「トヨタ」・JPY になる
// （5.5: Bの一覧（ListHoldings を利用者Bで呼ぶ）の銘柄名も「トヨタ」になる）。
func Test基準29a_銘柄名が違うときは銘柄マスタの銘柄名をCSVの銘柄名に変える(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserB, "7203", 30, "3000")

	out := runImportCSV(t, pool, seedUserA, importCSV("7203,トヨタ,JPY,買,2500,100,2026-01-10"))

	assertImportSucceeded(t, out, "1件の売買履歴を取り込みました。", "銘柄名を変えた銘柄: 7203")
	got, ok := importStockByCode(t, pool, "7203")
	if !ok || got.name != "トヨタ" || got.currency != "JPY" {
		t.Errorf("銘柄マスタの 7203: got %+v (ok=%v), want トヨタ・JPY", got, ok)
	}
	// Bの売買履歴は変わらず、Bの一覧の銘柄名が「トヨタ」になる。
	gotB := listHoldingsOf(t, pool, seedUserB)
	h := onlyHoldingOf(t, gotB.Holdings, "7203")
	if h.Name != "トヨタ" {
		t.Errorf("Bの一覧の 7203 の銘柄名: got %q, want %q（銘柄マスタは全利用者で共有）", h.Name, "トヨタ")
	}
	if h.Quantity != 30 {
		t.Errorf("Bの 7203 の保有数量: got %d, want 30（Bの売買履歴は変えない）", h.Quantity)
	}
}

// 基準30（要件26）: 開発用利用者の売買履歴が0件。見本の CSV（backend/db/seed/trades.example.csv）を開発用利用者に取り込む → 成功する。
// 5.5: テストの中で開発用利用者（…0001）を入れてから取り込む。test.sql には AAPL・MSFT を入れない。
// 見本は「開発用の seed と同じ 7203・6758 の売買。売りを1行含める」（4.6）ので、売りを含み、取り込んだ後の一覧に出る。
func Test基準30_見本のCSVをそのまま取り込める(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	if _, err := pool.Exec(t.Context(), "INSERT INTO users (id) VALUES ($1::uuid)", devUserID); err != nil {
		t.Fatalf("開発用利用者を入れられない: %v", err)
	}
	example := filepath.Join(repositoryRoot(t), "backend", "db", "seed", "trades.example.csv")

	out := runImport(t, pool, example, devUserID)

	if out.code != 0 {
		t.Fatalf("見本の CSV の取り込み: 終了コード got %d, want 0\nstdout: %q\nstderr: %q", out.code, out.stdout, out.stderr)
	}
	if len(out.stderr) != 0 {
		t.Errorf("標準エラーの行: got %q, want なし", out.stderr)
	}
	trades := importTradesOf(t, pool, devUserID)
	if len(trades) == 0 {
		t.Fatal("開発用利用者の売買履歴が0件（見本の CSV から取り込めていない）")
	}
	wantFirst := fmt.Sprintf("%d件の売買履歴を取り込みました。", len(trades))
	if len(out.stdout) == 0 || out.stdout[0] != wantFirst {
		t.Errorf("標準出力の1行目: got %q, want %q", out.stdout, wantFirst)
	}
	hasSell := false
	for _, tr := range trades {
		if tr.stockCode != "7203" && tr.stockCode != "6758" {
			t.Errorf("見本の CSV の銘柄: got %s, want 7203 か 6758（開発用の seed と同じ銘柄）", tr.stockCode)
		}
		if tr.side == "sell" {
			hasSell = true
		}
	}
	if !hasSell {
		t.Error("見本の CSV に売りの行がない（売りを1行含める。4.6）")
	}
	// 取り込んだ売買履歴から一覧を作れる（保有が出る）。
	if got := listHoldingsOf(t, pool, devUserID); len(got.Holdings) == 0 {
		t.Error("取り込んだ後の開発用利用者の一覧が空（見本の CSV は保有が残る形にする）")
	}
}

// 基準32（非機能2）: 2,000行の正しい CSV をAに取り込むと、10秒以内に終わる。
// 5.5: 測るのは Run の呼び出しだけ（ビルドとマイグレーションは含めない）。測った値を t.Log に出す。t.Parallel は使わない（ほかのテストと時間を取り合わないため）。
func Test基準32_2000行のCSVの取り込みが10秒以内に終わる(t *testing.T) {
	pool := testdb.New(t)
	var lines []string
	for i := range 2000 {
		n := i % 100
		if n%2 == 0 {
			lines = append(lines, fmt.Sprintf("U%03d,米国銘柄U%03d,USD,買,100.55,10,2026-01-%02d", n, n, i%28+1))
		} else {
			lines = append(lines, fmt.Sprintf("J%03d,日本銘柄J%03d,JPY,買,1000.5,10,2026-01-%02d", n, n, i%28+1))
		}
	}
	path := writeImportCSV(t, importCSV(lines...))

	start := time.Now()
	out := runImport(t, pool, path, seedUserA)
	elapsed := time.Since(start)

	t.Logf("2,000行の取り込み（Run の呼び出しだけ）: %v", elapsed)
	if out.code != 0 {
		t.Fatalf("終了コード: got %d, want 0\nstderr: %q", out.code, out.stderr)
	}
	if len(out.stdout) == 0 || out.stdout[0] != "2000件の売買履歴を取り込みました。" {
		t.Errorf("標準出力の1行目: got %q, want %q", out.stdout, "2000件の売買履歴を取り込みました。")
	}
	if got := importTradesOf(t, pool, seedUserA); len(got) != 2000 {
		t.Errorf("Aの売買履歴の件数: got %d, want 2000", len(got))
	}
	if elapsed >= 10*time.Second {
		t.Errorf("2,000行の取り込みの時間: got %v, want 10秒未満", elapsed)
	}
}

// 5.5「行どうしの細かい決まり」: 2行目「AAPL,Apple,USD,…」、3行目・4行目「AAPL,Apple Inc.,USD,…」、5行目・6行目「7203,トヨタ自動車,USD,…」、
// 7行目に 6758 の多すぎる売り（3〜6行目はどれも買い・株数10。7行目の前に 6758 の行はない）
// → 標準エラーは「3行目: …2行目と違います」「4行目: …2行目と違います」「5行目: …JPY で登録されています」「7行目: 売る株数…」の順だけで、6行目は出ない
// （銘柄マスタとの通貨の食い違いは、その銘柄コードの最初の行だけ。CSV の中の食い違いは、最初に出た行と比べる）。
func Test補助_行どうしの誤りの細かい決まりに従って出す(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	before := snapshotImportDB(t, pool)
	content := importCSV(
		"AAPL,Apple,USD,買,150,10,2026-01-05",      // 2行目
		"AAPL,Apple Inc.,USD,買,150,10,2026-01-06", // 3行目: 2行目と銘柄名が違う
		"AAPL,Apple Inc.,USD,買,150,10,2026-01-07", // 4行目: 2行目と銘柄名が違う（3行目とは同じ。最初の行と比べる）
		"7203,トヨタ自動車,USD,買,150,10,2026-01-08",     // 5行目: 銘柄マスタの通貨（JPY）と違う
		"7203,トヨタ自動車,USD,買,150,10,2026-01-09",     // 6行目: 同じ銘柄コードの最初の行ではないので出さない
		"6758,ソニーグループ,JPY,売,3000,10,2026-01-10",   // 7行目: 多すぎる売り（直前の保有数量は0株）
	)

	out := runImportCSV(t, pool, seedUserA, content)

	assertImportFailed(t, out,
		"3行目: 銘柄コード AAPL の銘柄名か通貨が、2行目と違います",
		"4行目: 銘柄コード AAPL の銘柄名か通貨が、2行目と違います",
		"5行目: 銘柄コード 7203 は銘柄マスタに JPY で登録されています",
		"7行目: 売る株数（10株）が、その時点の保有数量（0株）より多いです",
	)
	assertDBUnchanged(t, pool, before)
}

// 4.7（網羅の指摘27）: 2行目「AAPL,Apple,USD,買,150,10,2026-01-05」、3行目は AAPL 以外の正しい買い、4行目「AAPL,Apple Inc.,USD,売,150,1500,2026-01-10」
// → 「4行目: …2行目と違います」「4行目: 売る株数（1500株）が、その時点の保有数量（10株）より多いです」の順（同じ行では表の上の行の順）。
func Test補助_同じ行の行どうしの誤りは表の順に出す(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	before := snapshotImportDB(t, pool)
	content := importCSV(
		"AAPL,Apple,USD,買,150,10,2026-01-05",
		"7203,トヨタ自動車,JPY,買,2500,100,2026-01-06",
		"AAPL,Apple Inc.,USD,売,150,1500,2026-01-10",
	)

	out := runImportCSV(t, pool, seedUserA, content)

	assertImportFailed(t, out,
		"4行目: 銘柄コード AAPL の銘柄名か通貨が、2行目と違います",
		"4行目: 売る株数（1500株）が、その時点の保有数量（10株）より多いです",
	)
	assertDBUnchanged(t, pool, before)
}

// 5.5「成功の表示の並び」: 銘柄マスタに 7203（トヨタ自動車）と 6758（ソニーグループ）があり、CSV が MSFT・6758（ソニー）・AAPL・7203（トヨタ）の順
// → 標準出力は「4件の売買履歴を取り込みました。」「新しく登録した銘柄: AAPL, MSFT」「銘柄名を変えた銘柄: 6758, 7203」（銘柄コードの昇順。CSV の順ではない）。
func Test補助_成功の表示は新しい銘柄と銘柄名を変えた銘柄を銘柄コードの昇順で出す(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	content := importCSV(
		"MSFT,Microsoft,USD,買,400,5,2026-01-10",
		"6758,ソニー,JPY,買,3200,10,2026-01-10",
		"AAPL,Apple,USD,買,150.25,10,2026-01-10",
		"7203,トヨタ,JPY,買,2500,100,2026-01-10",
	)

	out := runImportCSV(t, pool, seedUserA, content)

	assertImportSucceeded(t, out,
		"4件の売買履歴を取り込みました。",
		"新しく登録した銘柄: AAPL, MSFT",
		"銘柄名を変えた銘柄: 6758, 7203",
	)
	wantStocks := map[string]importStockRow{
		"AAPL": {code: "AAPL", name: "Apple", currency: "USD"},
		"MSFT": {code: "MSFT", name: "Microsoft", currency: "USD"},
		"6758": {code: "6758", name: "ソニー", currency: "JPY"},
		"7203": {code: "7203", name: "トヨタ", currency: "JPY"},
	}
	for code, want := range wantStocks {
		if got, ok := importStockByCode(t, pool, code); !ok || got != want {
			t.Errorf("銘柄マスタの %s: got %+v (ok=%v), want %+v", code, got, ok, want)
		}
	}
}

// 要件21: 新しい銘柄も名前を変えた銘柄もないときは、「n件の売買履歴を取り込みました。」の1行だけを出す。
func Test補助_銘柄マスタが変わらない取り込みは件数の1行だけを出す(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)

	out := runImportCSV(t, pool, seedUserA, importCSV(
		"7203,トヨタ自動車,JPY,買,2500,100,2026-01-10",
		"6758,ソニーグループ,JPY,買,3200,10,2026-01-10",
	))

	assertImportSucceeded(t, out, "2件の売買履歴を取り込みました。")
}

// 5.3「登録した順」（要件8）: 同じ取引日に「売→買」と書いた CSV を取り込むと、一覧の取得価格が CSV の行の順で計算されている。
// 2026-01-10 に 買 100株・2,000円、2026-03-02 に（この順で）売 100株・2,000円、買 100株・3,000円 → 100株・3,000円（行の順）。
// 買いを先にたどると 200株→100株で 2,500円になってしまう。
func Test要件8_同じ取引日は取り込んだCSVの行の順に登録される(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	content := importCSV(
		"7203,トヨタ自動車,JPY,買,2000,100,2026-01-10",
		"7203,トヨタ自動車,JPY,売,2000,100,2026-03-02",
		"7203,トヨタ自動車,JPY,買,3000,100,2026-03-02",
	)

	out := runImportCSV(t, pool, seedUserA, content)

	assertImportSucceeded(t, out, "3件の売買履歴を取り込みました。")
	h := onlyHoldingOf(t, listHoldingsOf(t, pool, seedUserA).Holdings, "7203")
	if h.Quantity != 100 || h.AcquisitionPriceHundredths != 300000 {
		t.Errorf("Aの 7203: got %d株・%d（0.01 単位）, want 100株・300000（3,000円。CSV の行の順にたどる）", h.Quantity, h.AcquisitionPriceHundredths)
	}
}

// 要件24・要件3: 円の銘柄の価格に小数第2位があると取り込めず（「円の価格は小数第1位までにしてください」）、ドルの銘柄なら同じ価格を取り込める。
func Test基準25_円の価格の小数第2位は取り込めずドルは取り込める(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	before := snapshotImportDB(t, pool)

	out := runImportCSV(t, pool, seedUserA, importCSV("7203,トヨタ自動車,JPY,買,2000.25,100,2026-01-10"))
	assertImportFailed(t, out, "2行目: 円の価格は小数第1位までにしてください")
	assertDBUnchanged(t, pool, before)

	out = runImportCSV(t, pool, seedUserA, importCSV("AAPL,Apple,USD,買,2000.25,100,2026-01-10"))
	assertImportSucceeded(t, out, "1件の売買履歴を取り込みました。", "新しく登録した銘柄: AAPL")
}

// 4.7: USER_ID は UUID として読めない値だけが「利用者 <ID> が見つかりません」になる。
// 大文字の UUID は同じ利用者として取り込める。
func Test補助_USER_IDは大文字のUUIDでも同じ利用者として読める(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)

	out := runImportCSV(t, pool, strings.ToUpper(seedUserA), importCSV("7203,トヨタ自動車,JPY,買,2500,100,2026-01-10"))

	assertImportSucceeded(t, out, "1件の売買履歴を取り込みました。")
	if got := importTradesOf(t, pool, seedUserA); len(got) != 1 {
		t.Errorf("Aの売買履歴の件数: got %d, want 1", len(got))
	}
}

// 5.3「ReplaceTrades（統合）」（4.6）: 途中で失敗する入力（銘柄マスタにも newStocks にもない銘柄の売買）では、
// 売買履歴を消す・銘柄を足す・銘柄名を変えるのどれも残らない。
func Test補助_ReplaceTradesは途中で失敗したら何も変えない(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	insertTrade(t, pool, seedUserB, "6758", "buy", 5, "3000", "2026-01-05")
	before := snapshotImportDB(t, pool)
	repo := write.NewTradeImportRepository(pool)

	err := repo.ReplaceTrades(t.Context(), uuid.MustParse(seedUserA),
		[]model.Stock{{Code: "ZZZ1", Name: "新しい銘柄", Currency: model.CurrencyUSD}},
		[]model.Stock{{Code: "7203", Name: "改名後のトヨタ", Currency: model.CurrencyJPY}},
		[]model.Trade{
			{StockCode: "ZZZ1", Side: model.TradeSideBuy, Quantity: 1, PriceHundredths: 10000, TradeDate: "2026-01-10"},
			{StockCode: "NOSUCH", Side: model.TradeSideBuy, Quantity: 1, PriceHundredths: 10000, TradeDate: "2026-01-10"}, // 銘柄マスタにも newStocks にもない
		},
	)

	if err == nil {
		t.Fatal("ReplaceTrades: got nil, want エラー（銘柄マスタにない銘柄の売買）")
	}
	assertDBUnchanged(t, pool, before)
}

// 4.6 の ReplaceTrades の約束: 利用者の売買履歴をすべて消し、newStocks を足し、renamed の銘柄名を変え、trades をスライスの順に入れる（先の行ほど id が小さい）。
// 他の利用者の売買履歴と、銘柄マスタの通貨は変えない。
func Test補助_ReplaceTradesは売買履歴を入れ替え銘柄を足し銘柄名を変える(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	insertTrade(t, pool, seedUserB, "6758", "buy", 5, "3000", "2026-01-05")
	bBefore := importTradesOf(t, pool, seedUserB)
	repo := write.NewTradeImportRepository(pool)

	err := repo.ReplaceTrades(t.Context(), uuid.MustParse(seedUserA),
		[]model.Stock{{Code: "AAPL", Name: "Apple", Currency: model.CurrencyUSD}},
		[]model.Stock{{Code: "7203", Name: "トヨタ", Currency: model.CurrencyJPY}},
		[]model.Trade{
			{StockCode: "AAPL", Side: model.TradeSideBuy, Quantity: 10, PriceHundredths: 15025, TradeDate: "2026-01-10"},
			{StockCode: "7203", Side: model.TradeSideBuy, Quantity: 100, PriceHundredths: 250000, TradeDate: "2026-01-10"},
			{StockCode: "7203", Side: model.TradeSideSell, Quantity: 40, PriceHundredths: 300000, TradeDate: "2026-02-10"},
		},
	)

	if err != nil {
		t.Fatalf("ReplaceTrades: got err %v, want nil", err)
	}
	gotA := importTradesOf(t, pool, seedUserA)
	if len(gotA) != 3 {
		t.Fatalf("Aの売買履歴の件数: got %d (%+v), want 3（元の1件は消える）", len(gotA), gotA)
	}
	wantCodes := []string{"AAPL", "7203", "7203"}
	for i, tr := range gotA {
		if tr.stockCode != wantCodes[i] {
			t.Errorf("Aの売買履歴の %d 番目（登録した順）: got %s, want %s", i+1, tr.stockCode, wantCodes[i])
		}
	}
	if gotA[0].id >= gotA[1].id || gotA[1].id >= gotA[2].id {
		t.Errorf("trades.id がスライスの順に増えていない: %d, %d, %d", gotA[0].id, gotA[1].id, gotA[2].id)
	}
	if gotB := importTradesOf(t, pool, seedUserB); !slices.Equal(gotB, bBefore) {
		t.Errorf("Bの売買履歴が変わった: before %+v, after %+v", bBefore, gotB)
	}
	if s, ok := importStockByCode(t, pool, "AAPL"); !ok || s.name != "Apple" || s.currency != "USD" {
		t.Errorf("銘柄マスタの AAPL: got %+v (ok=%v), want Apple・USD", s, ok)
	}
	if s, ok := importStockByCode(t, pool, "7203"); !ok || s.name != "トヨタ" || s.currency != "JPY" {
		t.Errorf("銘柄マスタの 7203: got %+v (ok=%v), want トヨタ・JPY（銘柄名だけ変わる）", s, ok)
	}
}

// 4.6 の TradeImportRepository の読み取り: UserExists は利用者がいるかを返す。FindStocks は codes のうち銘柄マスタにある銘柄だけを、銘柄コードをキーにして返す。
func Test補助_TradeImportRepositoryは利用者と銘柄マスタを読める(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	repo := write.NewTradeImportRepository(pool)

	for _, tc := range []struct {
		name string
		id   string
		want bool
	}{
		{"利用者A", seedUserA, true},
		{"利用者B", seedUserB, true},
		{"いない利用者", unknownUserID, false},
	} {
		got, err := repo.UserExists(t.Context(), uuid.MustParse(tc.id))
		if err != nil || got != tc.want {
			t.Errorf("UserExists(%s): got (%v, %v), want (%v, nil)", tc.name, got, err, tc.want)
		}
	}

	got, err := repo.FindStocks(t.Context(), []string{"7203", "AAPL", "6758"})
	if err != nil {
		t.Fatalf("FindStocks: got err %v, want nil", err)
	}
	want := map[string]model.Stock{
		"7203": {Code: "7203", Name: "トヨタ自動車", Currency: model.CurrencyJPY},
		"6758": {Code: "6758", Name: "ソニーグループ", Currency: model.CurrencyJPY},
	}
	if len(got) != len(want) || got["7203"] != want["7203"] || got["6758"] != want["6758"] {
		t.Errorf("FindStocks: got %+v, want %+v（AAPL は銘柄マスタにないので含めない）", got, want)
	}
}

// 4.6 の ImportTrades: CheckUser は、利用者がいなければ ErrUserNotFound を返す。
func Test補助_CheckUserは利用者がいなければErrUserNotFoundを返す(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	uc := newImporter(pool)

	if err := uc.CheckUser(t.Context(), uuid.MustParse(seedUserA)); err != nil {
		t.Errorf("CheckUser(利用者A): got %v, want nil", err)
	}
	if err := uc.CheckUser(t.Context(), uuid.MustParse(unknownUserID)); !errors.Is(err, command.ErrUserNotFound) {
		t.Errorf("CheckUser(いない利用者): got %v, want ErrUserNotFound", err)
	}
}

// 4.6・4.7 の ImportTrades.Execute: 段階3の誤りなら *ImportValidationError を返し（何も変えない）、Problems は行番号の昇順、同じ行では Kind の昇順
// （CSV の中の食い違い → 銘柄マスタとの通貨の食い違い → 多すぎる売り）。
// 3行目: 7203 が USD（銘柄マスタは JPY）。4行目: AAPL の銘柄名が2行目と違い、かつ売りが多すぎる。
func Test補助_Executeは行どうしの誤りを行番号の昇順とKindの昇順で返す(t *testing.T) {
	t.Parallel()
	pool := givenImportPool(t)
	before := snapshotImportDB(t, pool)
	usd := func(code, name string) model.Stock {
		return model.Stock{Code: code, Name: name, Currency: model.CurrencyUSD}
	}
	rows := []command.ImportRow{
		{Line: 2, Stock: usd("AAPL", "Apple"), Trade: model.Trade{StockCode: "AAPL", Side: model.TradeSideBuy, Quantity: 10, PriceHundredths: 15000, TradeDate: "2026-01-05"}},
		{Line: 3, Stock: usd("7203", "トヨタ自動車"), Trade: model.Trade{StockCode: "7203", Side: model.TradeSideBuy, Quantity: 10, PriceHundredths: 15000, TradeDate: "2026-01-06"}},
		{Line: 4, Stock: usd("AAPL", "Apple Inc."), Trade: model.Trade{StockCode: "AAPL", Side: model.TradeSideSell, Quantity: 1500, PriceHundredths: 15000, TradeDate: "2026-01-10"}},
	}

	_, err := newImporter(pool).Execute(t.Context(), uuid.MustParse(seedUserA), rows)

	var verr *command.ImportValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("Execute: got err %v (%T), want *ImportValidationError", err, err)
	}
	// 種類ごとに意味のある項目だけを比べる（ImportProblem の4.6 のコメントのとおり）。
	var got []command.ImportProblem
	for _, p := range verr.Problems {
		switch p.Kind {
		case command.ProblemStockMismatchInCSV:
			got = append(got, command.ImportProblem{Line: p.Line, Kind: p.Kind, StockCode: p.StockCode, OtherLine: p.OtherLine})
		case command.ProblemCurrencyMismatchMaster:
			got = append(got, command.ImportProblem{Line: p.Line, Kind: p.Kind, StockCode: p.StockCode, Currency: p.Currency})
		case command.ProblemOversell:
			got = append(got, command.ImportProblem{Line: p.Line, Kind: p.Kind, Held: p.Held, Sold: p.Sold})
		default:
			got = append(got, p)
		}
	}
	want := []command.ImportProblem{
		{Line: 3, Kind: command.ProblemCurrencyMismatchMaster, StockCode: "7203", Currency: model.CurrencyJPY},
		{Line: 4, Kind: command.ProblemStockMismatchInCSV, StockCode: "AAPL", OtherLine: 2},
		{Line: 4, Kind: command.ProblemOversell, Held: 10, Sold: 1500},
	}
	if !slices.Equal(got, want) {
		t.Errorf("Execute の Problems:\n got %+v\nwant %+v", got, want)
	}
	assertDBUnchanged(t, pool, before)
}
