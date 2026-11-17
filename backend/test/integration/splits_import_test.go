package integration

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/write"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 株式分割の要件（docs/requirements/stock-split.md）の要件9〜14・要件17・非機能2 と、成功基準の基準13〜17（a 以外）・基準19・基準20・基準23、
// プラン（docs/plans/stock-split/plan.md）4.4（取り込みの流れ）・4.8 の PR② の境界（repository.StockSplitImportRepository・command.ImportStockSplits・
// splitcsv.Run・write.NewStockSplitImportRepository）、5.1 の基準13〜17・19・20・23 の行、5.2 の「分割の取り込みの並びの掃除」と「分割の取り込み（PR②）」。
// 補助は splits_import_helpers_test.go。「今日」は 2026-11-16。銘柄マスタは test.sql の 7203・6758・9984（円）。AAPL・NVDA はない。

// splitFixtureOversellOnRemoval は、7203 の分割（givenSplitsPool(true) の「1→3 2026-03-31」）を取り込みでなくすと、Aの売りが多すぎになる形にする。
// Aの 7203 は、買 100株 2026-01-10（givenSplitsPool）に「売 850円 300株 2026-05-01」を足す（分割ありなら 300株を持っていて売り切り。分割なしなら保有 100株で 300株は売れない）。
func splitFixtureOversellOnRemoval(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := givenSplitsPool(t, true)
	insertTrade(t, pool, seedUserA, "7203", "sell", 300, "850", "2026-05-01")
	return pool
}

// 基準13（要件9・要件10・要件11・要件13）: 分割は登録されていない。「7203,2026-03-31,3」「AAPL,2026/4/1,4」の CSV を取り込むと、終了コード0で
// 「2件の分割を取り込みました。」の1行だけが標準出力に出て、標準エラーは空。分割は 7203（2026-03-31・3）と AAPL（2026-04-01・4）のちょうど2行。
// 銘柄マスタ・売買履歴・終値（と並び）は変わらない（AAPL は銘柄マスタにないが、誤りにならず、銘柄マスタにも足されない）。
func Test基準13_分割のCSVを取り込める(t *testing.T) {
	t.Parallel()
	pool := givenSplitsPool(t, false)
	before := snapshotFiveTables(t, pool)

	out := runSplitImportCSV(t, pool, splitCSV("7203,2026-03-31,3", "AAPL,2026/4/1,4"))

	assertImportSucceeded(t, out, splitRegisteredMessage(2))
	want := []storedSplit{{"7203", "2026-03-31", 3}, {"AAPL", "2026-04-01", 4}}
	if got := readStockSplits(t, pool); !slices.Equal(got, want) {
		t.Errorf("stock_splits:\n got %+v\nwant %+v", got, want)
	}
	assertSplitsImportedKeepingOthers(t, pool, before)
}

// 基準14（要件10）: 分割は 7203（2026-03-31・3）と 6758（2026-05-01・2）が登録済みで、どの利用者の売買履歴にも 6758 の分割の後の売りはない。
// 「7203,2026-03-31,3」「AAPL,2026-04-01,4」の CSV を取り込むと成功し、分割は 7203（2026-03-31・3）と AAPL（2026-04-01・4）の2件になる（6758 の分割は消える）。
// 7203 の行は、登録済みと同じ値でも重ならず1行になる（消してから入れる）。
func Test基準14_登録済みの分割はCSVの内容に入れ替わる(t *testing.T) {
	t.Parallel()
	pool := givenSplitsPool(t, true)
	insertStockSplit(t, pool, "6758", "2026-05-01", 2)
	before := snapshotFiveTables(t, pool)

	out := runSplitImportCSV(t, pool, splitCSV("7203,2026-03-31,3", "AAPL,2026-04-01,4"))

	assertImportSucceeded(t, out, splitRegisteredMessage(2))
	want := []storedSplit{{"7203", "2026-03-31", 3}, {"AAPL", "2026-04-01", 4}}
	if got := readStockSplits(t, pool); !slices.Equal(got, want) {
		t.Errorf("stock_splits:\n got %+v\nwant %+v", got, want)
	}
	assertSplitsImportedKeepingOthers(t, pool, before)
}

// 基準15（要件11）: 銘柄マスタに NVDA がない。「NVDA,2024-06-10,10」の CSV を取り込むと成功し、NVDA（2024-06-10・10）の分割が登録される。銘柄マスタに NVDA は足されない。
func Test基準15_銘柄マスタにない銘柄の分割も取り込み銘柄マスタには足さない(t *testing.T) {
	t.Parallel()
	pool := givenSplitsPool(t, false)
	before := snapshotFiveTables(t, pool)

	out := runSplitImportCSV(t, pool, splitCSV("NVDA,2024-06-10,10"))

	assertImportSucceeded(t, out, splitRegisteredMessage(1))
	want := []storedSplit{{"NVDA", "2024-06-10", 10}}
	if got := readStockSplits(t, pool); !slices.Equal(got, want) {
		t.Errorf("stock_splits:\n got %+v\nwant %+v", got, want)
	}
	if got, ok := importStockByCode(t, pool, "NVDA"); ok {
		t.Errorf("銘柄マスタに NVDA が足された: %+v（分割の取り込みは銘柄マスタを変えない）", got)
	}
	assertSplitsImportedKeepingOthers(t, pool, before)
}

// 基準16（要件12・要件13）: 7203 の分割が「1→3 2026-03-31」で登録済み。Aの売買履歴が 7203「買 2,000円 100株 2026-01-10」「売 850円 300株 2026-05-01」。
// (a)「6758,2026-05-01,2」だけ（7203 の分割をなくす）(b)「7203,2026-06-01,3」だけ（分割の日を売りの後にずらす）で取り込むと、どちらも
// 「取り込めませんでした。何も変えていません。」と「利用者 <Aの ID> の銘柄 7203 の 2026-05-01 の売り（300株）が、その時点の保有数量（100株）より多くなります」の2行が標準エラーに出て、
// 終了コード1、標準出力は空。分割は 7203（2026-03-31・3）の1件のまま、5つの表は変わらない。
func Test基準16_分割の入れ替えで売りが多すぎになるなら誤りにして何も変えない(t *testing.T) {
	t.Parallel()
	for name, line := range map[string]string{
		"a_7203の分割をなくす":   "6758,2026-05-01,2",
		"b_分割の日を売りの後にずらす": "7203,2026-06-01,3",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pool := splitFixtureOversellOnRemoval(t)
			before := snapshotFiveTables(t, pool)

			out := runSplitImportCSV(t, pool, splitCSV(line))

			assertImportFailed(t, out, splitOversellMessage(seedUserA, "7203", "2026-05-01", 300, 100))
			want := []storedSplit{{"7203", "2026-03-31", 3}}
			if got := readStockSplits(t, pool); !slices.Equal(got, want) {
				t.Errorf("stock_splits:\n got %+v\nwant %+v（分割は1件のまま）", got, want)
			}
			assertFiveTablesUnchanged(t, pool, before)
		})
	}
}

// 基準17（a'〜f）（要件9・要件13・要件14・要件15）: 分割は 7203（2026-03-31・3）の1件が登録済み。次のどれかで取り込むと、
// 標準エラーが「取り込めませんでした。何も変えていません。」とファイルの段階の文言1つの2行、標準出力は空、終了コード1、分割は1件のまま（5つの表は変わらない）。
// (a') Run に空の CSVPath（task を通す (a) は splits_import_task_test.go） (b) 存在しないファイル (c) Shift_JIS で書いた CSV
// (d) 見出しが `銘柄コード,分割後の株数,分割の日`（順番違い） (e) 見出しだけの CSV (f) 2行目の銘柄コードが `"7203` で閉じていない CSV。
func Test基準17_ファイルの段階の誤りは文言1つだけ出して何も変えない(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "no-such-splits.csv")
	cases := []struct {
		name string
		path string // body が nil のとき、Run にそのまま渡すパス（空は CSV_FILE なし）
		body []byte // nil でなければ、これをファイルにして、そのパスを渡す
		want string
	}{
		{"a_CSV_FILEなし", "", nil, splitMissingArgsMsg},
		{"b_存在しないファイル", missing, nil, "ファイル " + missing + " を開けません"},
		{"c_Shift_JIS", "", []byte(splitShiftJISHeader + "7203,2026-03-31,3\r\n"), splitEncodingMessage},
		{"d_見出しの順番違い", "", []byte("銘柄コード,分割後の株数,分割の日\n7203,3,2026-03-31\n"), splitHeaderMessage},
		{"e_見出しだけ", "", []byte(splitImportHeader + "\n"), splitNoRowsMessage},
		{"f_閉じていないダブルクォート", "", splitCSV(`"7203,2026-03-31,3`), "2行目: " + splitQuoteTailMessage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := givenSplitsPool(t, true)
			before := snapshotFiveTables(t, pool)
			path := tc.path
			if tc.body != nil {
				path = writeSplitCSV(t, tc.body)
			}

			out := runSplitImport(t, pool, path)

			assertImportFailed(t, out, tc.want)
			want := []storedSplit{{"7203", "2026-03-31", 3}}
			if got := readStockSplits(t, pool); !slices.Equal(got, want) {
				t.Errorf("stock_splits:\n got %+v\nwant %+v（分割は1件のまま）", got, want)
			}
			assertFiveTablesUnchanged(t, pool, before)
		})
	}
}

// 基準19（要件17）: 分割は登録されていない。2行目が「7203,2026-03-31,3」、3行目が「6758,2026-03-31,2」、4行目が「7203,2026/3/31,2」の CSV を取り込むと、
// 「4行目: 銘柄コード 7203 の 2026-03-31 の分割は 2行目にもあります」が表示され、何も変わらない。
func Test基準19_同じ銘柄と同じ分割の日の行があると誤りにして何も変えない(t *testing.T) {
	t.Parallel()
	pool := givenSplitsPool(t, false)
	before := snapshotFiveTables(t, pool)

	out := runSplitImportCSV(t, pool, splitCSV("7203,2026-03-31,3", "6758,2026-03-31,2", "7203,2026/3/31,2"))

	assertImportFailed(t, out, "4行目: 銘柄コード 7203 の 2026-03-31 の分割は 2行目にもあります")
	assertFiveTablesUnchanged(t, pool, before)
}

// 基準20（要件13）: 3行目の分割後の株数が `1`、5行目の分割の日が `2026-02-30` で銘柄コードが `nvda`、ほかの行は正しい CSV を取り込むと、
// 「取り込めませんでした。何も変えていません。」「3行目: 分割後の株数は2以上の整数にしてください」「5行目: 銘柄コードは…」「5行目: 分割の日は…」がこの順に表示され、終了コード1。
// 前提は、分割（7203）・売買履歴・終値・並びを1件ずつ入れて、5つの表が変わらないことを確かめる（要件定義の Given は「分割は登録されていない」だが、
// 各行の誤りの前に分割の表を消す誤りを見分けるため、分割を1件足す。表示は変わらない）。
func Test基準20_各行の誤りをすべて行の順に表示して何も変えない(t *testing.T) {
	t.Parallel()
	pool := givenSplitsPool(t, true)
	before := snapshotFiveTables(t, pool)

	out := runSplitImportCSV(t, pool, splitCSV(
		"7203,2026-03-31,3",
		"6758,2026-05-01,1",
		"9984,2026-06-01,2",
		"nvda,2026-02-30,2",
		"AAPL,2026/4/1,4",
	))

	assertImportFailed(t, out,
		"3行目: "+splitRatioSmallMsg,
		"5行目: "+splitCodeMessage,
		"5行目: "+splitDateMessage,
	)
	assertFiveTablesUnchanged(t, pool, before)
}

// 要件12: 多すぎる売りは、取り込んだ後の分割（CSV の分割）で確かめる（登録済みの分割ではない）。
// 7203 の分割が「1→3 2026-03-31」で登録済み、Aの売買履歴が 7203「買 100株 2026-01-10」「売 300株 2026-05-01」（登録済みの分割なら売り切り）。
// 「7203,2026-03-31,2」（分割後の株数を減らす）で取り込むと、新しい分割では売る時点の保有数量が 200株なので、誤りになり何も変わらない。
// 登録済みの分割で確かめる実装を見分けるため、保有数量は 200株（登録済みなら 300株）。
func Test要件12_登録済みでなく取り込む分割で売りの確かめをする_株数を減らすと誤り(t *testing.T) {
	t.Parallel()
	pool := splitFixtureOversellOnRemoval(t)
	before := snapshotFiveTables(t, pool)

	out := runSplitImportCSV(t, pool, splitCSV("7203,2026-03-31,2"))

	assertImportFailed(t, out, splitOversellMessage(seedUserA, "7203", "2026-05-01", 300, 200))
	assertFiveTablesUnchanged(t, pool, before)
}

// 要件12: 7203 の分割が「1→2 2026-03-31」で登録済み、Aの売買履歴が 7203「買 100株 2026-01-10」「売 150株 2026-05-01」（登録済みの分割でも保有数量 200株で売れる）。
// 「7203,2026-03-31,3」（分割後の株数を増やす）で取り込むと成功する（新しい分割なら 300株。分割を使わずに確かめる実装なら、150株は 100株より多いと誤る）。
func Test要件12_取り込む分割で売りが範囲内なら成功する_株数を増やす(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStockSplit(t, pool, "7203", "2026-03-31", 2)
	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "2000", "2026-01-10")
	insertTrade(t, pool, seedUserA, "7203", "sell", 150, "850", "2026-05-01")

	out := runSplitImportCSV(t, pool, splitCSV("7203,2026-03-31,3"))

	assertImportSucceeded(t, out, splitRegisteredMessage(1))
	want := []storedSplit{{"7203", "2026-03-31", 3}}
	if got := readStockSplits(t, pool); !slices.Equal(got, want) {
		t.Errorf("stock_splits:\n got %+v\nwant %+v", got, want)
	}
}

// 5.2「段階の順」: 各行の誤りがあれば、同じ銘柄・同じ日の誤りと売買履歴との関係は出さない。
// 前提は、7203 の分割をなくすとAの売りが多すぎになる形。CSV は、2行目に株数の誤り、3行目と4行目が同じ銘柄・同じ日（`2026-03-31` と `2026/3/31`）、7203 の分割なし
// → 2行目の誤りだけが出る。
func Test要件13_各行の誤りがあれば行どうしと売買履歴との関係の誤りは出さない(t *testing.T) {
	t.Parallel()
	pool := splitFixtureOversellOnRemoval(t)
	before := snapshotFiveTables(t, pool)

	out := runSplitImportCSV(t, pool, splitCSV("6758,2026-03-31,1", "9984,2026-03-31,2", "9984,2026/3/31,2"))

	assertImportFailed(t, out, "2行目: "+splitRatioSmallMsg)
	assertFiveTablesUnchanged(t, pool, before)
}

// 5.2「段階の順」: 同じ銘柄・同じ日の誤りがあれば、売買履歴との関係は出さない。
// 前提は、7203 の分割をなくすとAの売りが多すぎになる形。CSV は、2行目と3行目が同じ銘柄・同じ日（9984・2026-03-31）、7203 の分割なし → 3行目の誤りだけが出る。
func Test要件13_同じ銘柄と同じ日の誤りがあれば売買履歴との関係の誤りは出さない(t *testing.T) {
	t.Parallel()
	pool := splitFixtureOversellOnRemoval(t)
	before := snapshotFiveTables(t, pool)

	out := runSplitImportCSV(t, pool, splitCSV("9984,2026-03-31,2", "9984,2026-03-31,3"))

	assertImportFailed(t, out, "3行目: 銘柄コード 9984 の 2026-03-31 の分割は 2行目にもあります")
	assertFiveTablesUnchanged(t, pool, before)
}

// splitFixtureManyOversells は、分割を入れ替えると多すぎる売りが出る形を作る（5.2「売買履歴との関係の並び」）。
// 7203 と 6758 の分割「1→3 2026-03-31」が登録済み。並べ替えない実装を見分けるため、Bの売買履歴をAより先に入れ、各利用者の中では 7203 の多すぎる売りを
// 6758 の多すぎる売りより前の日付・前の順に入れる。分割をなくすと（CSV「9984,2026-03-31,2」）、どの売りも多すぎになる。分割ありの今は、どれも保有数量の範囲内。
//
//	Aの 7203: 買 100株 2026-01-10、売 120株 2026-06-01（先に登録）、売 150株 2026-05-01（後に登録）
//	          分割なしだと、どちらの売りも保有 100株より多い（1人の1銘柄に多すぎる売りが2件。日付も株数も違い、日付の遅いほうを先に登録している）。
//	          分割ありの今は 300→150（05-01）→30株（06-01）で、どちらも範囲内
//	Aの 6758: 買 10株 2026-01-11、売 15株 2026-05-02（分割なしだと保有 10株）
//	Bの 7203: 買 50株 2026-01-10、売 80株 2026-05-03（分割なしだと保有 50株）
//	Bの 6758: 買 20株 2026-01-11、売 40株 2026-05-04（分割なしだと保有 20株）
func splitFixtureManyOversells(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testdb.New(t)
	insertStockSplit(t, pool, "7203", "2026-03-31", 3)
	insertStockSplit(t, pool, "6758", "2026-03-31", 3)
	insertTrade(t, pool, seedUserB, "7203", "buy", 50, "2000", "2026-01-10")
	insertTrade(t, pool, seedUserB, "7203", "sell", 80, "850", "2026-05-03")
	insertTrade(t, pool, seedUserB, "6758", "buy", 20, "3000", "2026-01-11")
	insertTrade(t, pool, seedUserB, "6758", "sell", 40, "3300", "2026-05-04")
	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "2000", "2026-01-10")
	insertTrade(t, pool, seedUserA, "7203", "sell", 120, "900", "2026-06-01")
	insertTrade(t, pool, seedUserA, "7203", "sell", 150, "850", "2026-05-01")
	insertTrade(t, pool, seedUserA, "6758", "buy", 10, "3000", "2026-01-11")
	insertTrade(t, pool, seedUserA, "6758", "sell", 15, "3300", "2026-05-02")
	insertClosePrice(t, pool, "7203", "2026-10-06", "3000")
	insertHoldingOrder(t, pool, seedUserA, "7203")
	return pool
}

// 5.2「売買履歴との関係の並び」: Aと Bにそれぞれ 7203 と 6758 の多すぎる売りが出る CSV → 誤りが A の 6758、A の 7203、B の 6758、B の 7203 の順
// （利用者の ID の昇順 → 銘柄コードの昇順。日付の順でも、入れた順でもない）。1人の1銘柄に多すぎる売りが2件あっても1件だけで、日付の早いほうの文言が出る（Aの 7203）。
func Test要件13_売買履歴との関係の誤りは利用者のIDと銘柄コードの昇順で1人の1銘柄に1件(t *testing.T) {
	t.Parallel()
	pool := splitFixtureManyOversells(t)
	before := snapshotFiveTables(t, pool)

	out := runSplitImportCSV(t, pool, splitCSV("9984,2026-03-31,2"))

	assertImportFailed(t, out,
		splitOversellMessage(seedUserA, "6758", "2026-05-02", 15, 10),
		splitOversellMessage(seedUserA, "7203", "2026-05-01", 150, 100),
		splitOversellMessage(seedUserB, "6758", "2026-05-04", 40, 20),
		splitOversellMessage(seedUserB, "7203", "2026-05-03", 80, 50),
	)
	assertFiveTablesUnchanged(t, pool, before)
}

// 4.8: ImportStockSplits.Execute は、多すぎる売りが出るなら *SplitValidationError（Oversells が利用者の ID の昇順 → 銘柄コードの昇順で、1人の1銘柄に1件）を返して、何も変えない。
// 件数は入れていないので 0。
func Test要件12_Executeは多すぎる売りをSplitValidationErrorで返し何も変えない(t *testing.T) {
	t.Parallel()
	pool := splitFixtureManyOversells(t)
	before := snapshotFiveTables(t, pool)

	n, err := newSplitImporter(pool).Execute(t.Context(), []model.StockSplit{{StockCode: "9984", SplitDate: "2026-03-31", Ratio: 2}})

	var verr *command.SplitValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("Execute の誤り: got %v (%T), want *command.SplitValidationError", err, err)
	}
	a, b := uuid.MustParse(seedUserA), uuid.MustParse(seedUserB)
	want := []command.SplitOversell{
		{UserID: a, StockCode: "6758", TradeDate: "2026-05-02", Held: 10, Sold: 15},
		{UserID: a, StockCode: "7203", TradeDate: "2026-05-01", Held: 100, Sold: 150},
		{UserID: b, StockCode: "6758", TradeDate: "2026-05-04", Held: 20, Sold: 40},
		{UserID: b, StockCode: "7203", TradeDate: "2026-05-03", Held: 50, Sold: 80},
	}
	if !slices.Equal(verr.Oversells, want) {
		t.Errorf("Oversells:\n got %+v\nwant %+v", verr.Oversells, want)
	}
	if n != 0 {
		t.Errorf("Execute の件数: got %d, want 0（何も入れていない）", n)
	}
	assertFiveTablesUnchanged(t, pool, before)
}

// 4.8: ImportStockSplits.Execute は、成功したら入れた件数を返し、分割を入れ替える（誤りは nil）。
func Test要件10_Executeは入れた件数を返し分割を入れ替える(t *testing.T) {
	t.Parallel()
	pool := givenSplitsPool(t, true)

	n, err := newSplitImporter(pool).Execute(t.Context(), []model.StockSplit{
		{StockCode: "AAPL", SplitDate: "2026-04-01", Ratio: 4},
		{StockCode: "9984", SplitDate: "2026-06-01", Ratio: 2},
	})

	if err != nil {
		t.Fatalf("Execute: got err %v, want nil", err)
	}
	if n != 2 {
		t.Errorf("Execute の件数: got %d, want 2", n)
	}
	want := []storedSplit{{"9984", "2026-06-01", 2}, {"AAPL", "2026-04-01", 4}}
	if got := readStockSplits(t, pool); !slices.Equal(got, want) {
		t.Errorf("stock_splits:\n got %+v\nwant %+v", got, want)
	}
}

// 5.2「分割の取り込みの並びの掃除」（カード表示の要件28）の前提。分割「1→2 2026-03-31」が 7203 に登録済み。
// Aは 7203「買 100株 2026-01-10」「売 100株 2026-04-10」（分割ありなら 100株を保有）と 9984「買 10株 2026-01-10」を持ち、並びは「7203, 9984」。
// Bも 7203 を「買 3,000円 10株 2026-01-10」で保有し、並びは「7203」。
// 利用者 C（splitSeedUserC）は売買履歴が0件で、並びの行だけ「7203」を持つ（SQL で直接入れる）。
func splitFixtureHoldingOrders(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testdb.New(t)
	if _, err := pool.Exec(t.Context(), "INSERT INTO users (id) VALUES ($1::uuid)", splitSeedUserC); err != nil {
		t.Fatalf("利用者 C を足せない: %v", err)
	}
	insertStockSplit(t, pool, "7203", "2026-03-31", 2)
	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "2000", "2026-01-10")
	insertTrade(t, pool, seedUserA, "7203", "sell", 100, "2500", "2026-04-10")
	insertTrade(t, pool, seedUserA, "9984", "buy", 10, "1000", "2026-01-10")
	insertTrade(t, pool, seedUserB, "7203", "buy", 10, "3000", "2026-01-10")
	insertHoldingOrder(t, pool, seedUserA, "7203", "9984")
	insertHoldingOrder(t, pool, seedUserB, "7203")
	insertHoldingOrder(t, pool, splitSeedUserC, "7203")
	return pool
}

// splitSeedUserC は、splitFixtureHoldingOrders が足す利用者C（売買履歴なし）の ID。test.sql の利用者（...0a・...0b）の決まりに合わせる。
const splitSeedUserC = "00000000-0000-0000-0000-00000000000c"

// 5.2「分割の取り込みの並びの掃除」: 7203 の分割を含まない CSV（「6758,2026-03-31,2」）を取り込むと、成功し（Aの売りは 100株で、買った 100株の範囲内。多すぎる売りにならない）、
// Aの 7203 は保有数量が 0 になるので、Aの並びから 7203 の行が消える（9984 は残る）。掃除は利用者ごと: Bの 7203 は保有を続けるので、Bの並びの 7203 は残る。
// 売買履歴が0件で並びの行だけある利用者Cの並びは、保有する銘柄がないので空になる。
func Testカード表示の要件28_分割をなくして保有がなくなった銘柄はその利用者の並びから消える(t *testing.T) {
	t.Parallel()
	pool := splitFixtureHoldingOrders(t)

	out := runSplitImportCSV(t, pool, splitCSV("6758,2026-03-31,2"))

	assertImportSucceeded(t, out, splitRegisteredMessage(1))
	if got, want := readHoldingOrder(t, pool, seedUserA), []string{"9984"}; !slices.Equal(got, want) {
		t.Errorf("Aの並び: got %v, want %v（保有がなくなった 7203 の行が消える）", got, want)
	}
	if got, want := readHoldingOrder(t, pool, seedUserB), []string{"7203"}; !slices.Equal(got, want) {
		t.Errorf("Bの並び: got %v, want %v（Bは 7203 を保有し続ける。掃除は利用者ごと）", got, want)
	}
	if got := readHoldingOrder(t, pool, splitSeedUserC); len(got) != 0 {
		t.Errorf("Cの並び: got %v, want 空（売買履歴が0件の利用者の並びの行も消える）", got)
	}
}

// 5.2「分割の取り込みの並びの掃除」: 7203 の分割を含む CSV（「7203,2026-03-31,2」）なら、並びは残る（Aの 7203 は 100株を保有）。
func Testカード表示の要件28_分割を残すなら並びは変わらない(t *testing.T) {
	t.Parallel()
	pool := splitFixtureHoldingOrders(t)

	out := runSplitImportCSV(t, pool, splitCSV("7203,2026-03-31,2"))

	assertImportSucceeded(t, out, splitRegisteredMessage(1))
	if got, want := readHoldingOrder(t, pool, seedUserA), []string{"7203", "9984"}; !slices.Equal(got, want) {
		t.Errorf("Aの並び: got %v, want %v", got, want)
	}
	if got, want := readHoldingOrder(t, pool, seedUserB), []string{"7203"}; !slices.Equal(got, want) {
		t.Errorf("Bの並び: got %v, want %v", got, want)
	}
}

// 5.2 ReplaceStockSplits（4.8）: check がエラーを返すと、そのエラーが返り、DB は変わらない（登録済みの分割も残る）。
func Test要件12_ReplaceStockSplitsはcheckがエラーを返すとそのエラーを返し何も変えない(t *testing.T) {
	t.Parallel()
	pool := givenSplitsPool(t, true)
	before := snapshotFiveTables(t, pool)
	sentinel := errors.New("check の誤り")
	repo := write.NewStockSplitImportRepository(pool)

	err := repo.ReplaceStockSplits(t.Context(),
		[]model.StockSplit{{StockCode: "AAPL", SplitDate: "2026-04-01", Ratio: 4}},
		func([]repository.UserTrades) error { return sentinel })

	if !errors.Is(err, sentinel) {
		t.Errorf("ReplaceStockSplits の誤り: got %v, want check の誤り", err)
	}
	assertFiveTablesUnchanged(t, pool, before)
}

// 要件10: 途中で失敗したら何も変えない。check は nil を返すが、同じ（銘柄コード, 分割の日）を2件渡したので、入れるところで一意制約に当たって失敗する。
// エラーが返り、分割・売買履歴・終値・並び（と銘柄マスタ）は変わらない（先に消した登録済みの分割も元に戻る）。
func Test要件10_ReplaceStockSplitsは入れるところで失敗したらエラーを返し何も変えない(t *testing.T) {
	t.Parallel()
	pool := givenSplitsPool(t, true)
	before := snapshotFiveTables(t, pool)
	repo := write.NewStockSplitImportRepository(pool)

	err := repo.ReplaceStockSplits(t.Context(),
		[]model.StockSplit{
			{StockCode: "AAPL", SplitDate: "2026-04-01", Ratio: 4},
			{StockCode: "AAPL", SplitDate: "2026-04-01", Ratio: 2},
		},
		func([]repository.UserTrades) error { return nil })

	if err == nil {
		t.Fatal("ReplaceStockSplits: got nil, want 一意制約の誤り")
	}
	assertFiveTablesUnchanged(t, pool, before)
}

// 5.2 ReplaceStockSplits（4.8）: check が受け取る利用者は ID の昇順で、各利用者の売買は登録した順（id の順）。
// Bの売買履歴をAより先に入れる（入れた順の実装を見分ける）。同じ取引日の2件を逆の銘柄コードの順（7203 の後に 6758）に入れ、
// Aの 7203 は後に登録した買いのほうが取引日が早い（取引日・銘柄コードで並べ替える実装を見分ける）。check が nil を返すと、ReplaceStockSplits は nil を返し、分割が入れ替わる。
func Test要件12_ReplaceStockSplitsはcheckに利用者のIDの昇順と登録した順の売買履歴を渡す(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStockSplit(t, pool, "7203", "2026-03-31", 3)
	insertTrade(t, pool, seedUserB, "7203", "buy", 10, "3000", "2026-01-10")
	insertTrade(t, pool, seedUserB, "6758", "buy", 5, "3200", "2026-01-10")
	insertTrade(t, pool, seedUserA, "7203", "buy", 20, "2000", "2026-02-01")
	insertTrade(t, pool, seedUserA, "7203", "buy", 10, "1500", "2026-01-05")
	repo := write.NewStockSplitImportRepository(pool)

	var got []repository.UserTrades
	calls := 0
	err := repo.ReplaceStockSplits(t.Context(),
		[]model.StockSplit{{StockCode: "AAPL", SplitDate: "2026-04-01", Ratio: 4}},
		func(all []repository.UserTrades) error {
			calls++
			got = all
			return nil
		})

	if err != nil {
		t.Fatalf("ReplaceStockSplits: got err %v, want nil", err)
	}
	if calls != 1 {
		t.Errorf("check を呼んだ回数: got %d, want 1", calls)
	}
	// 売買履歴のない利用者が混ざっていてもよいので、売買履歴のある利用者だけを比べる。
	var withTrades []repository.UserTrades
	for _, u := range got {
		if len(u.Trades) > 0 {
			withTrades = append(withTrades, u)
		}
	}
	// 価格は保有数量の計算に使わないので、境界に書かれていない価格（PriceHundredths）は比べない。
	want := []repository.UserTrades{
		{UserID: uuid.MustParse(seedUserA), Trades: []model.Trade{
			{StockCode: "7203", Side: model.TradeSideBuy, Quantity: 20, TradeDate: "2026-02-01"},
			{StockCode: "7203", Side: model.TradeSideBuy, Quantity: 10, TradeDate: "2026-01-05"},
		}},
		{UserID: uuid.MustParse(seedUserB), Trades: []model.Trade{
			{StockCode: "7203", Side: model.TradeSideBuy, Quantity: 10, TradeDate: "2026-01-10"},
			{StockCode: "6758", Side: model.TradeSideBuy, Quantity: 5, TradeDate: "2026-01-10"},
		}},
	}
	if len(withTrades) != len(want) {
		t.Fatalf("check が受け取った売買履歴のある利用者: got %+v, want %+v", withTrades, want)
	}
	for i, w := range want {
		gotTrades := slices.Clone(withTrades[i].Trades)
		for j := range gotTrades {
			gotTrades[j].PriceHundredths = 0
		}
		if withTrades[i].UserID != w.UserID || !slices.Equal(gotTrades, w.Trades) {
			t.Errorf("check が受け取った %d 人目:\n got %+v\nwant %+v", i+1, withTrades[i], w)
		}
	}
	if gotSplits, wantSplits := readStockSplits(t, pool), []storedSplit{{"AAPL", "2026-04-01", 4}}; !slices.Equal(gotSplits, wantSplits) {
		t.Errorf("stock_splits:\n got %+v\nwant %+v（check が nil なら入れ替える）", gotSplits, wantSplits)
	}
}

// 基準23（非機能2）: 全利用者の売買履歴が合計2,000件（A・Bが1,000件ずつ。100銘柄に10件ずつ）。売買履歴のある100銘柄の分割の行を含む、100行の正しい CSV の取り込みが、
// 10秒以内に終わり、終了コード0（多すぎる売りの確かめが、実際に分割を当てる道を測る）。
// 測るのは Run の呼び出しだけ（ビルドとマイグレーションは含めない）。測った値を t.Log に出す。t.Parallel は使わない（ほかのテストと時間を取り合わないため）。
func Test基準23_全利用者の売買履歴2000件のとき100行の分割のCSVの取り込みが10秒以内に終わる(t *testing.T) {
	pool := testdb.New(t)
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO stocks (code, name, currency)
		 SELECT 'T' || lpad(i::text, 3, '0'), 'テスト銘柄' || i::text, 'JPY' FROM generate_series(0, 99) AS i`); err != nil {
		t.Fatalf("銘柄マスタに100銘柄を足せない: %v", err)
	}
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO trades (user_id, stock_code, side, quantity, price, trade_date)
		 SELECT (CASE WHEN i % 2 = 0 THEN $1::text ELSE $2::text END)::uuid,
		        'T' || lpad(((i / 2) % 100)::text, 3, '0'), 'buy', 10, 1000, DATE '2026-01-01' + (i % 28)
		   FROM generate_series(0, 1999) AS i
		  ORDER BY i`, seedUserA, seedUserB); err != nil {
		t.Fatalf("売買履歴2,000件を入れられない: %v", err)
	}
	var lines []string
	for i := range 100 {
		lines = append(lines, fmt.Sprintf("T%03d,2026-03-31,2", i))
	}
	path := writeSplitCSV(t, splitCSV(lines...))

	start := time.Now()
	out := runSplitImport(t, pool, path)
	elapsed := time.Since(start)

	t.Logf("100行の分割の取り込み（売買履歴2,000件）: %s", elapsed)
	assertImportSucceeded(t, out, splitRegisteredMessage(100))
	if elapsed > 10*time.Second {
		t.Errorf("取り込みにかかった時間: got %s, want 10秒以内", elapsed)
	}
	if got := readStockSplits(t, pool); len(got) != 100 {
		t.Errorf("stock_splits の件数: got %d, want 100", len(got))
	}
}
