package integration

import (
	"net/http"
	"slices"
	"testing"
)

// 仕様: カード表示の要件定義（docs/requirements/holdings-cards.md）の要件25〜28 と、成功基準の基準26〜29、
// プラン（docs/plans/holdings-cards/plan.md）4.3「消すとき」・5.1 の基準26〜29 の行・5.3 の PR②（統合）の「削除の後」「取り込みの後」の行。
// 取り込みは runImportCSV（売買履歴の CSV。全部入れ替え）、削除は DELETE /api/holdings/{code} の API。
// 並びは銘柄コードで覚える。取り込みの前後で保有数量が1以上の銘柄は並びを変えず、保有数量が0になって一覧から消えた銘柄は並びの行を消す
// （また保有したときは新しく出てきた銘柄と同じく末尾に、銘柄コードの昇順で出る）。

// CSV の行（要件定義の売買履歴の書き方。取引日は 2026-01-10 を基本にする）。名前は銘柄マスタ（test.sql・givenCardsPool）と同じ。
const (
	csvToyotaBuy = "7203,トヨタ自動車,JPY,買,2500,100,2026-01-10"
	csvSonyBuy   = "6758,ソニーグループ,JPY,買,4000,10,2026-01-10"
	csvSBGBuy    = "9984,ソフトバンクグループ,JPY,買,4500,100,2026-01-10"
	csvAppleBuy  = "AAPL,Apple,USD,買,150.25,10,2026-01-10"
	// 売り切る 7203（買 100株 → 売 100株）。
	csvToyotaSell = "7203,トヨタ自動車,JPY,売,2600,100,2026-02-10"
)

// 基準26（要件25）: Aが 6758、7203、AAPL を保有し、並びを AAPL、7203、6758 に保存している
// → 同じ売買履歴の CSV を取り込み直しても、Aの一覧の並びは AAPL、7203、6758 のまま（DB の並びの行も同じ）。
func Testカード表示の基準26_同じCSVを取り込み直しても並びは変わらない(t *testing.T) {
	t.Parallel()
	pool := givenCardsPool(t)
	csv := importCSV(csvSonyBuy, csvToyotaBuy, csvAppleBuy)
	// 保有は同じ CSV の取り込みで作る（売買履歴の全部入れ替えと、並びの行が結びつく、実際の流れ）。
	first := runImportCSV(t, pool, seedUserA, csv)
	if first.code != 0 {
		t.Fatalf("最初の取り込み: 終了コード got %d, want 0\nstderr: %q", first.code, first.stderr)
	}
	insertHoldingOrder(t, pool, seedUserA, "AAPL", "7203", "6758")

	out := runImportCSV(t, pool, seedUserA, csv)

	if out.code != 0 {
		t.Fatalf("取り込み直し: 終了コード got %d, want 0\nstderr: %q", out.code, out.stderr)
	}
	if got, want := listedCodes(t, newHoldingsAPIFor(pool, seedUserA)), []string{"AAPL", "7203", "6758"}; !slices.Equal(got, want) {
		t.Errorf("Aの一覧の並び: got %v, want %v", got, want)
	}
	if got, want := readHoldingOrder(t, pool, seedUserA), []string{"AAPL", "7203", "6758"}; !slices.Equal(got, want) {
		t.Errorf("DB の並び: got %v, want %v", got, want)
	}
}

// 基準27（要件27）: Aが 7203、AAPL を保有し、並びを AAPL、7203 に保存している
// → 9984 と 6758 の買いを足した CSV を取り込むと、Aの一覧の並びは AAPL、7203、6758、9984。
// プラン 5.1: CSV には 9984 を 6758 より先に書く（入れた順で並べる誤りと見分けるため）。
func Testカード表示の基準27_新しく出てきた銘柄は末尾にコードの昇順で並ぶ(t *testing.T) {
	t.Parallel()
	pool := givenCardsPool(t)
	holdBuys(t, pool, seedUserA, "7203", "AAPL")
	insertHoldingOrder(t, pool, seedUserA, "AAPL", "7203")

	out := runImportCSV(t, pool, seedUserA, importCSV(csvToyotaBuy, csvAppleBuy, csvSBGBuy, csvSonyBuy))

	if out.code != 0 {
		t.Fatalf("取り込み: 終了コード got %d, want 0\nstderr: %q", out.code, out.stderr)
	}
	if got, want := listedCodes(t, newHoldingsAPIFor(pool, seedUserA)), []string{"AAPL", "7203", "6758", "9984"}; !slices.Equal(got, want) {
		t.Errorf("Aの一覧の並び: got %v, want %v", got, want)
	}
}

// 基準28（要件28）: Aが 6758、7203、AAPL を保有し、並びを AAPL、7203、6758 に保存している
// → 7203 を売り切った CSV を取り込むと並びは AAPL、6758。続けて 7203 をまた買った CSV を取り込むと並びは AAPL、6758、7203。
// 7203 の前にいた位置（2番目）は覚えていない。売り切った 7203 の並びの行は取り込みの時点で消える（DB の並びで確かめる）。
func Testカード表示の基準28_売り切って消えた銘柄をまた保有すると末尾に出る(t *testing.T) {
	t.Parallel()
	pool := givenCardsPool(t)
	holdBuys(t, pool, seedUserA, "6758", "7203", "AAPL")
	insertHoldingOrder(t, pool, seedUserA, "AAPL", "7203", "6758")
	h := newHoldingsAPIFor(pool, seedUserA)

	sold := runImportCSV(t, pool, seedUserA, importCSV(csvSonyBuy, csvToyotaBuy, csvToyotaSell, csvAppleBuy))

	if sold.code != 0 {
		t.Fatalf("取り込み（7203 を売り切る）: 終了コード got %d, want 0\nstderr: %q", sold.code, sold.stderr)
	}
	if got, want := listedCodes(t, h), []string{"AAPL", "6758"}; !slices.Equal(got, want) {
		t.Errorf("売り切った後の一覧の並び: got %v, want %v", got, want)
	}
	if got, want := readHoldingOrder(t, pool, seedUserA), []string{"AAPL", "6758"}; !slices.Equal(got, want) {
		t.Errorf("売り切った後の DB の並び: got %v, want %v（7203 の行は消える）", got, want)
	}

	bought := runImportCSV(t, pool, seedUserA, importCSV(csvSonyBuy, csvToyotaBuy, csvAppleBuy))

	if bought.code != 0 {
		t.Fatalf("取り込み（7203 をまた買う）: 終了コード got %d, want 0\nstderr: %q", bought.code, bought.stderr)
	}
	if got, want := listedCodes(t, h), []string{"AAPL", "6758", "7203"}; !slices.Equal(got, want) {
		t.Errorf("また買った後の一覧の並び: got %v, want %v（前の位置ではなく末尾）", got, want)
	}
}

// 基準29（要件28）: 基準28 の最初と同じ（Aが 6758、7203、AAPL を保有し、並びを AAPL、7203、6758 に保存している）
// → Aが 7203 を削除してから、7203 の買いを含む CSV を取り込むと、並びは AAPL、6758、7203。
func Testカード表示の基準29_削除した銘柄をまた取り込むと末尾に出る(t *testing.T) {
	t.Parallel()
	pool := givenCardsPool(t)
	holdBuys(t, pool, seedUserA, "6758", "7203", "AAPL")
	insertHoldingOrder(t, pool, seedUserA, "AAPL", "7203", "6758")
	api := newHoldingOrderAPI(pool, seedUserA)

	deleted := serveDelete(t, api, holdingDeletePath("7203"), sameOriginHeaders())

	if deleted.Code != http.StatusNoContent {
		t.Fatalf("削除: got %d, want 204\n本文: %s", deleted.Code, deleted.Body.Bytes())
	}
	if got, want := listedCodes(t, api), []string{"AAPL", "6758"}; !slices.Equal(got, want) {
		t.Errorf("削除した後の一覧の並び: got %v, want %v", got, want)
	}

	out := runImportCSV(t, pool, seedUserA, importCSV(csvSonyBuy, csvToyotaBuy, csvAppleBuy))

	if out.code != 0 {
		t.Fatalf("取り込み: 終了コード got %d, want 0\nstderr: %q", out.code, out.stderr)
	}
	if got, want := listedCodes(t, api), []string{"AAPL", "6758", "7203"}; !slices.Equal(got, want) {
		t.Errorf("取り込んだ後の一覧の並び: got %v, want %v（前の位置ではなく末尾）", got, want)
	}
}

// 補助（5.3「削除の後」）: AとBがどちらも 6758、7203 を保有し、どちらも並びを保存している（Aは 7203、6758、Bは 6758、7203）
// → Aが 7203 を削除すると、Aの並びの行は 6758 だけになり、Bの並びの行は 6758、7203 のまま残る。
func Testカード表示の補助_削除の後その利用者のその銘柄の並びの行が消え他の利用者の行は残る(t *testing.T) {
	t.Parallel()
	pool := givenCardsPool(t)
	holdBuys(t, pool, seedUserA, "6758", "7203")
	holdBuys(t, pool, seedUserB, "6758", "7203")
	insertHoldingOrder(t, pool, seedUserA, "7203", "6758")
	insertHoldingOrder(t, pool, seedUserB, "6758", "7203")

	deleted := serveDelete(t, newHoldingOrderAPI(pool, seedUserA), holdingDeletePath("7203"), sameOriginHeaders())

	if deleted.Code != http.StatusNoContent {
		t.Fatalf("削除: got %d, want 204\n本文: %s", deleted.Code, deleted.Body.Bytes())
	}
	if got, want := readHoldingOrder(t, pool, seedUserA), []string{"6758"}; !slices.Equal(got, want) {
		t.Errorf("Aの並びの行: got %v, want %v（7203 の行は消える）", got, want)
	}
	if got, want := readHoldingOrder(t, pool, seedUserB), []string{"6758", "7203"}; !slices.Equal(got, want) {
		t.Errorf("Bの並びの行: got %v, want %v（他の利用者の行は残る）", got, want)
	}
}

// 補助（5.3「取り込みの後」）: Aが 6758、7203、AAPL を保有し並びを AAPL、7203、6758 に、Bが 6758、7203 を保有し並びを 6758、7203 に保存している
// → Aに、7203 を含まない CSV（6758 と AAPL だけ）を取り込むと、Aの並びの行は 7203 だけが消えて AAPL、6758 になり、Bの並びの行は変わらない。
func Testカード表示の補助_取り込みの後保有していない銘柄の並びの行だけが消え他の利用者の行は残る(t *testing.T) {
	t.Parallel()
	pool := givenCardsPool(t)
	holdBuys(t, pool, seedUserA, "6758", "7203", "AAPL")
	holdBuys(t, pool, seedUserB, "6758", "7203")
	insertHoldingOrder(t, pool, seedUserA, "AAPL", "7203", "6758")
	insertHoldingOrder(t, pool, seedUserB, "6758", "7203")

	out := runImportCSV(t, pool, seedUserA, importCSV(csvSonyBuy, csvAppleBuy))

	if out.code != 0 {
		t.Fatalf("取り込み: 終了コード got %d, want 0\nstderr: %q", out.code, out.stderr)
	}
	if got, want := readHoldingOrder(t, pool, seedUserA), []string{"AAPL", "6758"}; !slices.Equal(got, want) {
		t.Errorf("Aの並びの行: got %v, want %v（保有していない 7203 の行だけが消える）", got, want)
	}
	if got, want := readHoldingOrder(t, pool, seedUserB), []string{"6758", "7203"}; !slices.Equal(got, want) {
		t.Errorf("Bの並びの行: got %v, want %v（他の利用者の行は残る）", got, want)
	}
}
