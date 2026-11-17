package integration

import (
	"net/http"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/infrastructure/pricesource"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: プラン（docs/plans/stock-split/plan.md）4.3（保有している銘柄の判定を Go に移す。判断1）と 5.2「保有している銘柄の判定」（PR①）。
// 売買履歴の要件16（価格の取得の対象は保有している銘柄）とカード表示の要件28（並びの保存と掃除）が、分割を含めた保有数量で判定されることを確かめる。
// データは「Aの 7203 が 買 2,000円 100株 2026-01-10、売 2,000円 200株 2026-05-01、分割 1→3 2026-03-31」
// （本当は 100株を保有。分割を無視した「買 − 売」では −100 になる）と、比べ役に
// 「Aの 6758 が 買 3,000円 100株 2026-01-10、売 3,000円 300株 2026-05-01、分割 1→3 2026-03-31」（売り切り）。
// 価格の取得はダミーの取得元（pricesource.NewDummyPriceSource。7203・6758 とも価格を持つので、6758 が保存されれば誤り）。

// givenSplitHeldPool は、上のデータを入れた DB を作る。
func givenSplitHeldPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testdb.New(t)
	insertStockSplit(t, pool, "7203", "2026-03-31", 3)
	insertStockSplit(t, pool, "6758", "2026-03-31", 3)
	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "2000", "2026-01-10")
	insertTrade(t, pool, seedUserA, "7203", "sell", 200, "2000", "2026-05-01")
	insertTrade(t, pool, seedUserA, "6758", "buy", 100, "3000", "2026-01-10")
	insertTrade(t, pool, seedUserA, "6758", "sell", 300, "3000", "2026-05-01")
	return pool
}

// 売買履歴の要件16（定時の取得）: 7203 の価格が保存され、6758（売り切り）は保存されない。
func Test要件16_定時の取得は分割を含めた保有数量で保有している銘柄だけを取得する(t *testing.T) {
	t.Parallel()
	pool := givenSplitHeldPool(t)

	got, err := newFetchClosingPrices(pricesource.NewDummyPriceSource(), pool).Execute(t.Context())

	if err != nil {
		t.Fatalf("Execute: got err %v, want nil", err)
	}
	if want := []string{"7203"}; !slices.Equal(got.Saved, want) {
		t.Errorf("FetchResult.Saved: got %v, want %v（6758 は売り切り）", got.Saved, want)
	}
	assertStockPrices(t, pool, []storedClosePrice{
		{stockCode: "7203", tradingDate: "2026-10-06", priceHundredths: 300000},
	})
}

// 売買履歴の要件16（今すぐ更新。RefreshPrices）: 同じく 7203 の価格が保存され、6758 は保存されない。
func Test要件16_今すぐ更新も分割を含めた保有数量で保有している銘柄だけを取得する(t *testing.T) {
	t.Parallel()
	pool := givenSplitHeldPool(t)
	h := newRefreshAPI(pool, seedUserA, pricesource.NewDummyPriceSource())

	rec := servePost(t, h, priceRefreshPath, sameOriginHeaders())

	assertJSONResponse(t, rec, http.StatusOK, `{"saved":["7203"],"failed":[]}`)
	assertStockPrices(t, pool, []storedClosePrice{
		{stockCode: "7203", tradingDate: "2026-10-06", priceHundredths: 300000},
	})
}

// カード表示の要件28（並びの保存）: Aが並び「7203」を保存できる（ReplaceHoldingOrder が 7203 を保有している銘柄として扱う）。
func Test要件28_並びの保存は分割を含めた保有数量で保有している銘柄として扱う(t *testing.T) {
	t.Parallel()
	pool := givenSplitHeldPool(t)
	api := newHoldingOrderAPI(pool, seedUserA)

	rec := servePutOrder(t, api, putOrderBody("7203"), sameOriginHeaders())

	assertNoContent(t, rec)
	if got, want := readHoldingOrder(t, pool, seedUserA), []string{"7203"}; !slices.Equal(got, want) {
		t.Errorf("Aの並び: got %v, want %v", got, want)
	}
}

// カード表示の要件28（並びの保存）: 6758（売り切り）を含む並び「7203, 6758」は、保有していない銘柄の誤り（400 と bad_request）で、並びは変わらない。
func Test要件28_売り切った銘柄を含む並びは保有していない銘柄の誤りになる(t *testing.T) {
	t.Parallel()
	pool := givenSplitHeldPool(t)
	api := newHoldingOrderAPI(pool, seedUserA)

	rec := servePutOrder(t, api, putOrderBody("7203", "6758"), sameOriginHeaders())

	assertJSONResponse(t, rec, http.StatusBadRequest, badRequestJSON)
	if got := readHoldingOrder(t, pool, seedUserA); len(got) != 0 {
		t.Errorf("Aの並び: got %v, want 空（何も変えない）", got)
	}
}

// カード表示の要件28（取り込みの後の掃除）: 並び「7203, 6758」を入れておき（6758 は売り切りの前に保存した形で SQL から入れる）、
// 同じ売買履歴の CSV を取り込み直すと、7203 の行は残り、6758 の行は消える（掃除が分割を含めた保有数量で判定する）。
func Test要件28_売買履歴の取り込み直しの後の並びの掃除は分割を含めた保有数量で判定する(t *testing.T) {
	t.Parallel()
	pool := givenSplitHeldPool(t)
	insertHoldingOrder(t, pool, seedUserA, "7203", "6758")

	out := runImportCSV(t, pool, seedUserA, importCSV(
		"7203,トヨタ自動車,JPY,買,2000,100,2026-01-10",
		"7203,トヨタ自動車,JPY,売,2000,200,2026-05-01",
		"6758,ソニーグループ,JPY,買,3000,100,2026-01-10",
		"6758,ソニーグループ,JPY,売,3000,300,2026-05-01",
	))

	assertImportSucceeded(t, out, "4件の売買履歴を取り込みました。")
	if got, want := readHoldingOrder(t, pool, seedUserA), []string{"7203"}; !slices.Equal(got, want) {
		t.Errorf("Aの並び: got %v, want %v（7203 は保有しているので残り、売り切った 6758 は消える）", got, want)
	}
}

// カード表示の要件28（取り込みの後の掃除。保有している銘柄が0件）: 取り込みの後に保有している銘柄が1つもなくなると、その利用者の並びはすべて消える。
// 分割「1→3 2026-03-31」を SQL で入れ、Aの並び「7203」とBの並び「7203」（Bも 7203 を「買 3,000円 10株 2026-01-10」で保有）がある状態で、
// 7203 の「買 100株 2026-01-10」「売 300株 2026-05-01」（分割で 300株になるので売り切り）の CSV をAに取り込むと、成功し、
// Aの並びは空になり、Bの並び「7203」は残る（掃除は利用者ごと）。
func Test要件28_取り込みの後に保有している銘柄が0件ならその利用者の並びはすべて消える(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStockSplit(t, pool, "7203", "2026-03-31", 3)
	insertTrade(t, pool, seedUserB, "7203", "buy", 10, "3000", "2026-01-10")
	insertHoldingOrder(t, pool, seedUserA, "7203")
	insertHoldingOrder(t, pool, seedUserB, "7203")

	out := runImportCSV(t, pool, seedUserA, importCSV(
		"7203,トヨタ自動車,JPY,買,2000,100,2026-01-10",
		"7203,トヨタ自動車,JPY,売,2000,300,2026-05-01",
	))

	assertImportSucceeded(t, out, "2件の売買履歴を取り込みました。")
	if got := readHoldingOrder(t, pool, seedUserA); len(got) != 0 {
		t.Errorf("Aの並び: got %v, want 空（保有している銘柄が0件）", got)
	}
	if got, want := readHoldingOrder(t, pool, seedUserB), []string{"7203"}; !slices.Equal(got, want) {
		t.Errorf("Bの並び: got %v, want %v（他の利用者の並びは消えない）", got, want)
	}
}
