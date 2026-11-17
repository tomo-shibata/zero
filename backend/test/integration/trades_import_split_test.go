package integration

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 株式分割の要件（docs/requirements/stock-split.md）の基準10〜12（要件7）と要件11（「分割 → 売買履歴」の順）、
// プラン（docs/plans/stock-split/plan.md）4.8 の PR① の境界（TradeImportRepository.FindStockSplits。ImportTrades.Execute が CSV の銘柄の分割を読んで FindOversells に渡す）、
// 5.1 の基準10〜12 の行と 5.2 の「分割 → 売買履歴の順」。
// 売買履歴の取り込みのテストの補助（runImportCSV・assertImportSucceeded・assertImportFailed。trades_import_helpers_test.go）を使う。
// 「何も変わらない」は、今の snapshotImportDB・assertDBUnchanged（trades・stocks だけ）ではなく、5つの表を比べる補助
// （snapshotFiveTables・assertFiveTablesUnchanged。stock_splits_fixtures_test.go）で確かめる。
// 「今日」は 2026-11-16。銘柄マスタは test.sql の 7203・6758・9984（円）。

// 売買履歴の CSV の行（要件定義の書き方）。
const (
	csvToyota100Buy = "7203,トヨタ自動車,JPY,買,2000,100,2026-01-10"
	csvToyota300Sel = "7203,トヨタ自動車,JPY,売,850,300,2026-05-01"
	csvToyota301Sel = "7203,トヨタ自動車,JPY,売,850,301,2026-05-01"
)

// givenSplitImportPool は、基準11・基準12 の前提の DB を作る。Aの売買履歴は 7203「買 2,000円 100株 2026-01-10」（SQL で入れる）、
// 7203 の終値 3,000円（2026-10-06）と、Aの並び「7203」を1件ずつ入れる（取り込みが前後の表を変えないことを、空の表どうしでなく確かめるため）。
// withSplit が true なら、7203 の分割「1→3 2026-03-31」を登録済みにする（基準11）。false なら分割は登録されていない（基準12）。
func givenSplitImportPool(t *testing.T, withSplit bool) *pgxpool.Pool {
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

// 基準10（要件7）: 7203 の分割が「1→3 2026-03-31」で登録済み → 売買履歴の CSV「買 2,000円 100株 2026-01-10」「売 850円 300株 2026-05-01」を
// Aに取り込むと成功する（分割を反映して、売る時点で 300株持っている）。Aの売買履歴は CSV の2件になる。
func Test基準10_登録済みの分割を反映して売りの確かめをし成功する(t *testing.T) {
	t.Parallel()
	pool := givenSplitImportPool(t, true)

	out := runImportCSV(t, pool, seedUserA, importCSV(csvToyota100Buy, csvToyota300Sel))

	assertImportSucceeded(t, out, "2件の売買履歴を取り込みました。")
	gotA := importTradesOf(t, pool, seedUserA)
	if len(gotA) != 2 {
		t.Fatalf("Aの売買履歴の件数: got %d (%+v), want 2", len(gotA), gotA)
	}
	if a := gotA[0]; a.side != "buy" || a.quantity != 100 || a.tradeDate != "2026-01-10" {
		t.Errorf("Aの売買履歴の1件目: got %+v, want 買・100株・2026-01-10", a)
	}
	if a := gotA[1]; a.side != "sell" || a.quantity != 300 || a.tradeDate != "2026-05-01" {
		t.Errorf("Aの売買履歴の2件目: got %+v, want 売・300株・2026-05-01", a)
	}
}

// 基準11（要件7）: 7203 の分割が「1→3 2026-03-31」で登録済み → CSV の2行目が「買 2,000円 100株 2026-01-10」、3行目が「売 850円 301株 2026-05-01」で Aに取り込むと、
// 「3行目: 売る株数（301株）が、その時点の保有数量（300株）より多いです」が表示され（誤りの文言の保有数量も分割を反映した値）、何も変わらない。
func Test基準11_分割を反映した保有数量より多い売りは誤りになり何も変えない(t *testing.T) {
	t.Parallel()
	pool := givenSplitImportPool(t, true)
	before := snapshotFiveTables(t, pool)

	out := runImportCSV(t, pool, seedUserA, importCSV(csvToyota100Buy, csvToyota301Sel))

	assertImportFailed(t, out, "3行目: 売る株数（301株）が、その時点の保有数量（300株）より多いです")
	assertFiveTablesUnchanged(t, pool, before)
}

// 基準12（要件7）: 分割は登録されていない → 基準10 と同じ CSV を Aに取り込むと、
// 「3行目: 売る株数（300株）が、その時点の保有数量（100株）より多いです」が表示され、何も変わらない。
func Test基準12_分割がなければ基準10と同じCSVは多すぎる売りになり何も変えない(t *testing.T) {
	t.Parallel()
	pool := givenSplitImportPool(t, false)
	before := snapshotFiveTables(t, pool)

	out := runImportCSV(t, pool, seedUserA, importCSV(csvToyota100Buy, csvToyota300Sel))

	assertImportFailed(t, out, "3行目: 売る株数（300株）が、その時点の保有数量（100株）より多いです")
	assertFiveTablesUnchanged(t, pool, before)
}

// 要件11（5.2「分割 → 売買履歴の順」）: NVDA の「分割 1→10 2024-06-10」を SQL で入れ、銘柄マスタに NVDA がない状態で、
// NVDA の「買 1株 2024-06-03」「売 10株 2024-06-11」（ドル）の CSV を Aに取り込むと成功する
// （分割を読むときに銘柄マスタを見ると、新しい銘柄の分割を読めず、売りが多すぎると誤る。FindStockSplits は stocks を見ない）。
func Test要件11_銘柄マスタにない銘柄の分割も売買履歴の取り込みで使う(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStockSplit(t, pool, "NVDA", "2024-06-10", 10)
	if _, ok := importStockByCode(t, pool, "NVDA"); ok {
		t.Fatal("前提: 銘柄マスタに NVDA がないはずが、ある")
	}

	out := runImportCSV(t, pool, seedUserA, importCSV(
		"NVDA,NVIDIA,USD,買,1200.00,1,2024-06-03",
		"NVDA,NVIDIA,USD,売,120.00,10,2024-06-11",
	))

	assertImportSucceeded(t, out, "2件の売買履歴を取り込みました。", "新しく登録した銘柄: NVDA")
	if got := importTradesOf(t, pool, seedUserA); len(got) != 2 {
		t.Errorf("Aの売買履歴の件数: got %d (%+v), want 2", len(got), got)
	}
}
