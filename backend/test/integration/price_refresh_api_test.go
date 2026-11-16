package integration

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/read"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/write"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/pricesource"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/router"
	"github.com/tomo-shibata/zero/backend/internal/interface/handler"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 要件（docs/requirements/price-update.md）の基準26・基準27・基準31（価格の今すぐ更新の API 部分）と、
// プラン（docs/plans/price-update/plan.md）5.1 の基準26・基準27・31 の行、5.2 の PR① の「RefreshPrices の router を通すと 500・internal_error」。
// 組み立ては 4.4 の境界のとおり:
//   command.NewFetchClosingPrices(取得元, write.NewHoldingRepository(pool), write.NewClosingPriceRepository(pool))
//   → command.NewRefreshPrices(fetch, write.NewHoldingRepository(pool)) → handler.NewPriceRefreshHandler
//   → router.New(router.Deps{PriceRefreshHandler: …}) を httptest で呼ぶ。
// 取得元は、基準26 がダミー（pricesource.NewDummyPriceSource）、基準27・31 がテスト用の取得元（境界の外側の fake）。
// ログイン中の利用者は DevUserID で決まる（利用者Bとしての要求は、同じ DB で DevUserID を B にした router を作る）。
// 要求には、4.4 のとおり Host: 127.0.0.1:8080 と、アプリの画面と同じオリジンの組（Origin と Sec-Fetch-Site の両方）を付ける。

// 4.4「統合テストの要求」: アプリの画面と同じオリジンの組。ブラウザが Vite の proxy 経由で送る形。
const (
	appOrigin         = "http://localhost:5173"
	sameOriginFetch   = "same-origin"
	priceRefreshPath  = "/api/price-refresh"
	evilOrigin        = "http://evil.example"
	secFetchSiteField = "Sec-Fetch-Site"
)

// sameOriginHeaders は、アプリの画面と同じオリジンから送られた書き込みの要求のヘッダー（4.4）。
func sameOriginHeaders() map[string]string {
	return map[string]string{"Origin": appOrigin, secFetchSiteField: sameOriginFetch}
}

// servePost は、Host を allowedHost にし、headers を付けた、本文のない POST path を h に送り、応答を返す。
func servePost(t *testing.T, h http.Handler, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, nil)
	req.Host = allowedHost
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// countingPriceSource は、inner に処理を任せ、要求された銘柄コードを、要求された順に記録する取得元（境界の外側の fake）。
// 「取得元に要求が届かない」（基準32）と「取得元を呼ばない」（5.2）を確かめるために使う。並行して呼ばれても壊れないよう mutex で守る。
type countingPriceSource struct {
	inner command.PriceSource
	mu    sync.Mutex
	codes []string
}

func (s *countingPriceSource) FetchClosingPrice(ctx context.Context, stockCode string) (model.ClosingPrice, error) {
	s.mu.Lock()
	s.codes = append(s.codes, stockCode)
	s.mu.Unlock()
	return s.inner.FetchClosingPrice(ctx, stockCode)
}

// requested は、取得元に要求された銘柄コードを、要求された順に返す。
func (s *countingPriceSource) requested() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.codes)
}

// newRefreshAPI は、pool の DB と取得元 src を使う、今すぐ更新（POST /api/price-refresh）と一覧（GET /api/holdings）の API を、
// 4.4 の境界のとおりに組み立てる。ログイン中の利用者は userID。
func newRefreshAPI(pool *pgxpool.Pool, userID string, src command.PriceSource) http.Handler {
	return newRefreshAPIWithHeld(pool, userID, src, write.NewHoldingRepository(pool))
}

// newRefreshAPIWithHeld は、newRefreshAPI と同じ組み立てで、利用者の保有銘柄を読む IF だけを held にしたもの。
func newRefreshAPIWithHeld(pool *pgxpool.Pool, userID string, src command.PriceSource, held repository.HeldStockRepository) http.Handler {
	fetch := newFetchClosingPrices(src, pool)
	return router.New(router.Deps{
		QueryHandler:        handler.NewQueryHandler(query.NewListHoldings(read.NewHoldingsReader(pool))),
		PriceRefreshHandler: handler.NewPriceRefreshHandler(command.NewRefreshPrices(fetch, held)),
		DevUserID:           uuid.MustParse(userID),
		AllowedHosts:        []string{allowedHost},
	})
}

// 基準26（要件16）: 利用者Aが 7203、利用者Bが 6758 を保有し、どちらも価格は未保存 → Aが今すぐ更新を実行する
// → 7203 は3,000円・2026-10-06 で保存され、6758 は保存されない。
// プラン 5.1: ダミーの取得元で組み立てる。ダミーは 6758 の値（3,500円）を持つので、全利用者の銘柄を取得する誤った実装なら
// 6758 が保存されて落ちる。応答は 200 と {"saved":["7203"],"failed":[]}（4.4 の API）。
func Test基準26_今すぐ更新はログイン中の利用者の保有銘柄だけを取得して保存し他の利用者だけが保有する銘柄は保存しない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	// 数量・取得価格はプランに指定がないので、取得の結果に関係しない値にした。
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	insertBuy(t, pool, seedUserB, "6758", 10, "3200")
	h := newRefreshAPI(pool, seedUserA, pricesource.NewDummyPriceSource())

	rec := servePost(t, h, priceRefreshPath, sameOriginHeaders())

	assertJSONResponse(t, rec, http.StatusOK, `{"saved":["7203"],"failed":[]}`)
	assertStockPrices(t, pool, []storedClosePrice{
		{stockCode: "7203", tradingDate: "2026-10-06", priceHundredths: 300000},
	})
}

// 基準27（要件16）: Aが 7203 と 6758 を保有し、7203 の取得元の価格が2,900円・2026-10-05 で保存済み。取得元は 7203 に「取得できない」、
// 6758 に3,500円・2026-10-06 を返す → Aが今すぐ更新を実行する
// → 7203 は2,900円・2026-10-05 のまま残り、6758 は保存され、結果は「保存できた銘柄: 6758、取得できなかった銘柄: 7203」になる
// （応答は {"saved":["6758"],"failed":["7203"]}。4.4 の API）。
func Test基準27_取得できない銘柄は保存済みの価格を残し他の銘柄は保存して結果に分けて返す(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertClosePrice(t, pool, "7203", "2026-10-05", "2900")
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	insertBuy(t, pool, seedUserA, "6758", 10, "3200")
	src := stubPriceSource{
		t: t,
		prices: map[string]model.ClosingPrice{
			"6758": mustClosingPrice(t, "6758", "2026-10-06", 350000),
		},
		unavailable: []string{"7203"},
	}
	h := newRefreshAPI(pool, seedUserA, src)

	rec := servePost(t, h, priceRefreshPath, sameOriginHeaders())

	assertJSONResponse(t, rec, http.StatusOK, `{"saved":["6758"],"failed":["7203"]}`)
	assertStockPrices(t, pool, []storedClosePrice{
		{stockCode: "6758", tradingDate: "2026-10-06", priceHundredths: 350000},
		{stockCode: "7203", tradingDate: "2026-10-05", priceHundredths: 290000}, // 保存済みの2,900円・2026-10-05 のまま
	})
}

// 基準31（非機能2）: 共通インターフェース（command.PriceSource）を実装したテスト用の取得元 → それを注入して組み立てた API で、
// Aが今すぐ更新を実行する → テスト用の取得元が返した価格が保存される。
// プラン 5.1: 基準31 の値はダミーにないもの（9984 → 123450（0.01 単位）・2026-10-07）。ダミーは 9984 を「取得できない」にするので、
// 保存されたのがテスト用の取得元の値なら、取得元が差し替えられていると分かる。
func Test基準31_共通インターフェースを実装したテスト用の取得元を注入すると今すぐ更新でその取得元が返した価格が保存される(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserA, "9984", 100, "4500")
	// 変数の型を command.PriceSource にして、テスト用の取得元が共通インターフェースを実装していることを型で確かめる。
	var src command.PriceSource = stubPriceSource{
		t: t,
		prices: map[string]model.ClosingPrice{
			"9984": mustClosingPrice(t, "9984", "2026-10-07", 123450),
		},
	}
	h := newRefreshAPI(pool, seedUserA, src)

	rec := servePost(t, h, priceRefreshPath, sameOriginHeaders())

	assertJSONResponse(t, rec, http.StatusOK, `{"saved":["9984"],"failed":[]}`)
	assertStockPrices(t, pool, []storedClosePrice{
		{stockCode: "9984", tradingDate: "2026-10-07", priceHundredths: 123450},
	})
}

// failingHeldStockRepository は、保有銘柄を読むと必ず失敗する repository.HeldStockRepository（境界の外側の fake）。
type failingHeldStockRepository struct{}

// failingHeldDetail は、fake が返すエラーの内容。応答に出てはいけない（4.4: 内部のエラー内容は返さずログにだけ出す）。
const failingHeldDetail = "fake: connection refused to db.internal:5432"

func (failingHeldStockRepository) ListHeldStocksOf(context.Context, uuid.UUID) ([]repository.HeldStock, error) {
	return nil, errors.New(failingHeldDetail)
}

// 補助 要件16（5.2）: 保有銘柄を読めない RefreshPrices を router に通すと、POST /api/price-refresh は 500 と {"error":"internal_error"}
// （内部のエラーの内容は返さない）。取得元は呼ばれず、何も保存されない。
// Aは 7203 を保有し、取得元は 7203 の価格を持つ（保有銘柄を読めなくても取得へ進む実装なら、7203 が保存されて落ちる）。
func Test補助_要件16_保有銘柄を読めないときの今すぐ更新は500とinternal_errorを返し取得元を呼ばず何も保存しない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	src := &countingPriceSource{inner: pricesource.NewDummyPriceSource()}
	h := newRefreshAPIWithHeld(pool, seedUserA, src, failingHeldStockRepository{})

	rec := servePost(t, h, priceRefreshPath, sameOriginHeaders())

	assertJSONResponse(t, rec, http.StatusInternalServerError, `{"error":"internal_error"}`)
	if got := src.requested(); len(got) != 0 {
		t.Errorf("取得元に要求された銘柄: got %v, want 呼ばれない", got)
	}
	assertStockPrices(t, pool, []storedClosePrice{})
}

// 補助 4.4・要件18: Aが 9984 だけを保有（ダミーの取得元は 9984 を「取得できない」にする）→ 今すぐ更新
// → 200 と、本文がちょうど {"saved":[],"failed":["9984"]}（saved は null でも項目なしでもなく、空の配列。4.4 の API「0件でも []」）。
// stock_prices は空のまま。本文の JSON は assertJSONResponse で厳密に比べる（値の種類まで同じ。null と [] は別のもの）。
func Test補助_要件18_4_4_保存できた銘柄が0件の応答はsavedが空の配列でfailedに取得できなかった銘柄が入る(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserA, "9984", 100, "4500")
	h := newRefreshAPI(pool, seedUserA, pricesource.NewDummyPriceSource())

	rec := servePost(t, h, priceRefreshPath, sameOriginHeaders())

	assertJSONResponse(t, rec, http.StatusOK, `{"saved":[],"failed":["9984"]}`)
	assertStockPrices(t, pool, []storedClosePrice{})
}

// 補助 4.4・要件18: Aは保有なし、Bが 7203 を保有 → Aが今すぐ更新
// → 200 と、本文がちょうど {"saved":[],"failed":[]}（どちらも null でも項目なしでもなく、空の配列）。
// stock_prices は空のまま。取得元に要求が届かない（Bだけが保有する 7203 を取得しない。取得元は要求された銘柄を記録する）。
func Test補助_要件18_4_4_保有銘柄がない利用者の応答はsavedもfailedも空の配列で取得元を呼ばない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserB, "7203", 100, "2500")
	src := &countingPriceSource{inner: pricesource.NewDummyPriceSource()}
	h := newRefreshAPI(pool, seedUserA, src)

	rec := servePost(t, h, priceRefreshPath, sameOriginHeaders())

	assertJSONResponse(t, rec, http.StatusOK, `{"saved":[],"failed":[]}`)
	assertStockPrices(t, pool, []storedClosePrice{})
	if got := src.requested(); len(got) != 0 {
		t.Errorf("取得元に要求された銘柄: got %v, want 届かない（Aは何も保有していない）", got)
	}
}

// 補助 要件16（上書き）: 7203 の 2,900円・2026-10-06 が保存済みで、Aが 7203 を保有 → ダミーの取得元で今すぐ更新
// → 200 と {"saved":["7203"],"failed":[]}（同じ銘柄・同じ取引日の終値は上書きして成功とする。保有株式一覧の FR-15）。
// stock_prices は (7203, 2026-10-06, 300000) の1行だけ（2行にならず、値は取得した 3,000円。価格は 0.01 単位の整数）。
func Test補助_要件16_同じ銘柄と取引日の終値が保存済みなら今すぐ更新は上書きして成功とする(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertClosePrice(t, pool, "7203", "2026-10-06", "2900")
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	h := newRefreshAPI(pool, seedUserA, pricesource.NewDummyPriceSource())

	rec := servePost(t, h, priceRefreshPath, sameOriginHeaders())

	assertJSONResponse(t, rec, http.StatusOK, `{"saved":["7203"],"failed":[]}`)
	assertStockPrices(t, pool, []storedClosePrice{
		{stockCode: "7203", tradingDate: "2026-10-06", priceHundredths: 300000},
	})
}

// 基準17（要件16。売買履歴の要件定義）: 基準16 と同じ（Aが 7203「買 2,500円 100株」と 6758「買 3,200円 10株」「売 3,500円 10株」、
// Bが AAPL「買 150.25ドル 10株」を持ち、どれも価格は未保存）→ Aが今すぐ更新を実行する → 7203 だけが保存され、6758（売り切り）と
// AAPL（Bだけが保有）は保存されない。
// 取得元は、PR② でダミー（pricesource.NewDummyPriceSource。7203・6758・AAPL のすべてに価格を持つ）を、
// 要求された銘柄を数える countingPriceSource で包む（取得してはいけない銘柄も、要求されたら保存されてしまう形にして確かめる。プラン 5.1・5.5）。
// 取得元に要求された銘柄は 7203 だけ。応答は 200 と {"saved":["7203"],"failed":[]}。
func Test基準17_今すぐ更新はログイン中の利用者の保有数量が1以上の銘柄だけを取得し売り切った銘柄と他の利用者の銘柄は取得しない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertTradeHistoryGiven(t, pool)
	src := &countingPriceSource{inner: pricesource.NewDummyPriceSource()}
	h := newRefreshAPI(pool, seedUserA, src)

	rec := servePost(t, h, priceRefreshPath, sameOriginHeaders())

	assertJSONResponse(t, rec, http.StatusOK, `{"saved":["7203"],"failed":[]}`)
	assertStockPrices(t, pool, []storedClosePrice{
		{stockCode: "7203", tradingDate: "2026-10-06", priceHundredths: 300000},
	})
	if got := src.requested(); !slices.Equal(got, []string{"7203"}) {
		t.Errorf("取得元に要求された銘柄: got %v, want [7203]（売り切った 6758 と、Bだけが保有する AAPL は要求しない）", got)
	}
}

// 補助 要件5・4.6（PR②）: Aが 7203（円）と AAPL（ドル）を保有し、どちらも価格は未保存 → ダミーの取得元で今すぐ更新
// → 200 と {"saved":["7203","AAPL"],"failed":[]}。AAPL は 180.00ドル（0.01 単位で 18000）・2026-10-06 で保存される
// （ドルの銘柄もダミーの取得元から取得して保存する。保有している銘柄は銘柄マスタの通貨と一緒に読まれる）。
func Test補助_要件5_ドルの銘柄も今すぐ更新でダミーの取得元から取得して保存する(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStock(t, pool, "AAPL", "Apple", currencyUSD)
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	insertBuy(t, pool, seedUserA, "AAPL", 10, "150.25")
	h := newRefreshAPI(pool, seedUserA, pricesource.NewDummyPriceSource())

	rec := servePost(t, h, priceRefreshPath, sameOriginHeaders())

	assertJSONResponse(t, rec, http.StatusOK, `{"saved":["7203","AAPL"],"failed":[]}`)
	assertStockPrices(t, pool, []storedClosePrice{
		{stockCode: "7203", tradingDate: "2026-10-06", priceHundredths: 300000},
		{stockCode: "AAPL", tradingDate: "2026-10-06", priceHundredths: 18000},
	})
}
