package integration

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/write"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/router"
	"github.com/tomo-shibata/zero/backend/internal/interface/handler"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 売買履歴の要件定義（docs/requirements/trade-history.md）の基準20（要件18）と、
// プラン（docs/plans/trade-history/plan.md）5.1 の基準20 の行・5.2・5.3「削除」（PR①）。
// 削除は、ログイン中の利用者のその銘柄の売買履歴（trades）をすべて消す。他の利用者の売買履歴と、同じ利用者の別の銘柄の売買履歴と、
// 銘柄マスタは消さない。ログイン中の利用者にその銘柄の売買履歴が1件もないときは、何も消さずに 404 と {"error":"not_found"}。
// 売り切った銘柄（売買履歴はある）の削除は 204 で売買履歴が消える（5.3）。
// 元の値段の更新の基準23（銘柄ごとの削除。要件14）のテストは、前提を売買履歴で作り、消えたかを trades で確かめるように直した（5.2）。
//
// 組み立ては 4.6 の境界のとおり（名前と IF は変えない。判断3）:
//
//	command.NewDeleteHolding(write.NewHoldingEditRepository(pool)) → handler.NewHoldingDeleteHandler
//	→ router.New(router.Deps{HoldingDeleteHandler: …}) を httptest で呼ぶ。
//
// 値段の更新のプラン（docs/plans/price-update/plan.md）4.4 の API の表（DELETE /api/holdings/{code}: 204）と
// 「パスの {code}」の決まり（r.URL.RawPath が空でないときだけ url.PathUnescape する）。
// ログイン中の利用者は DevUserID で決まる。要求には 4.4 のとおり Host: 127.0.0.1:8080 と、アプリの画面と同じオリジンの組を付ける。

const notFoundJSON = `{"error":"not_found"}`

// holdingDeletePath は、銘柄 code（URL のパスに入れる形のまま）を削除する API のパス。
func holdingDeletePath(code string) string {
	return holdingsPath + "/" + code
}

// newDeleteAPI は、pool の DB を使う、銘柄ごとの削除（DELETE /api/holdings/{code}）の API を、4.6 の境界のとおりに組み立てる。
// ログイン中の利用者は userID。
func newDeleteAPI(pool *pgxpool.Pool, userID string) http.Handler {
	return router.New(router.Deps{
		HoldingDeleteHandler: handler.NewHoldingDeleteHandler(command.NewDeleteHolding(write.NewHoldingEditRepository(pool))),
		DevUserID:            uuid.MustParse(userID),
		AllowedHosts:         []string{allowedHost},
	})
}

// serveDelete は、Host を allowedHost にし、headers を付けた、本文のない DELETE path を h に送り、応答を返す。
func serveDelete(t *testing.T, h http.Handler, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, path, nil)
	req.Host = allowedHost
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// storedTrade は、trades の1行のうち、テストで確かめる値。価格は 0.01 単位の整数（例: 200000 は 2,000円）。
type storedTrade struct {
	userID          string
	stockCode       string
	side            string
	quantity        int64
	priceHundredths int64
}

// readTrades は、trades のすべての行（全利用者）を、利用者・銘柄コード・登録順（id の昇順）で返す。
func readTrades(t *testing.T, pool *pgxpool.Pool) []storedTrade {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT user_id::text, stock_code, side, quantity, (price * 100)::bigint
		   FROM trades
		  ORDER BY user_id, stock_code, id`)
	if err != nil {
		t.Fatalf("trades を読めない: %v", err)
	}
	trades, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (storedTrade, error) {
		var st storedTrade
		err := row.Scan(&st.userID, &st.stockCode, &st.side, &st.quantity, &st.priceHundredths)
		return st, err
	})
	if err != nil {
		t.Fatalf("trades を読めない: %v", err)
	}
	return trades
}

// assertTrades は、trades の全行（全利用者。利用者・銘柄コード・登録順）が want とちょうど同じであることを確かめる。
// 消えるべきでない行が消えたことも、消えるべき行が残ったことも失敗にする。
func assertTrades(t *testing.T, pool *pgxpool.Pool, want []storedTrade) {
	t.Helper()
	if got := readTrades(t, pool); !slices.Equal(got, want) {
		t.Errorf("trades の行（利用者・銘柄コード・売買・株数・0.01 単位の価格）:\n got %+v\nwant %+v", got, want)
	}
}

// assertStockExists は、銘柄マスタに銘柄 code がある（消えていない）ことを確かめる。
func assertStockExists(t *testing.T, pool *pgxpool.Pool, code string) {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(t.Context(), "SELECT EXISTS (SELECT 1 FROM stocks WHERE code = $1::text)", code).Scan(&exists); err != nil {
		t.Fatalf("銘柄マスタを読めない: %v", err)
	}
	if !exists {
		t.Errorf("銘柄マスタの %s が消えた（削除は売買履歴だけを消し、銘柄マスタは消さない）", code)
	}
}

// insertDeleteGivenTrades は、基準20 の Given の売買履歴を入れる。
// Aが 7203 を売買履歴3件（買 100株・2,000円 2026-01-10、買 300株・3,000円 2026-01-10、売 100株・3,500円 2026-02-10）と、
// 6758 を1件（買 50株・3,200円 2026-01-10）、Bが 7203 を1件（買 10株・2,500円 2026-01-10）。
// 株数・価格は基準20 に指定がないので、元の基準23 の値（7203 の2件、6758、Bの 7203）にし、7203 の3件目に売りを足した（削除の結果に関係しない）。
// この Given の trades の全行（assertTrades に渡す順: 利用者・銘柄コード・登録順）は givenTrades。
func insertDeleteGivenTrades(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	insertBuy(t, pool, seedUserA, "7203", 100, "2000")
	insertBuy(t, pool, seedUserA, "7203", 300, "3000")
	insertTrade(t, pool, seedUserA, "7203", "sell", 100, "3500", "2026-02-10")
	insertBuy(t, pool, seedUserA, "6758", 50, "3200")
	insertBuy(t, pool, seedUserB, "7203", 10, "2500")
}

// givenTrades は、insertDeleteGivenTrades が入れた trades の全行。
func givenTrades() []storedTrade {
	return []storedTrade{
		{seedUserA, "6758", "buy", 50, 320000},
		{seedUserA, "7203", "buy", 100, 200000},
		{seedUserA, "7203", "buy", 300, 300000},
		{seedUserA, "7203", "sell", 100, 350000},
		{seedUserB, "7203", "buy", 10, 250000},
	}
}

// 基準20（要件18）: Aが 7203 を売買履歴3件と 6758 を1件、Bが 7203 を1件で持つ → Aが 7203 を削除する
// → 204（本文なし）。Aの 7203 の売買履歴は0件になり、Aの 6758 とBの 7203 の売買履歴と、銘柄マスタの 7203 は残る。
// Aの 7203 を3件にするのは、1件だけ消す実装を見分けるため。Bの 7203 を置くのは、利用者で絞らず銘柄だけで消す実装を、
// Aの 6758 を置くのは、利用者の売買履歴を全部消す実装（銘柄で絞らない）を、銘柄マスタの確かめは、銘柄マスタまで消す実装を見分けるため。
func Test基準20_Aが7203を削除するとAの7203の売買履歴だけが全件消えBの7203とAの6758と銘柄マスタは残る(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertDeleteGivenTrades(t, pool)
	h := newDeleteAPI(pool, seedUserA)

	rec := serveDelete(t, h, holdingDeletePath("7203"), sameOriginHeaders())

	if rec.Code != http.StatusNoContent {
		t.Errorf("ステータス: got %d, want %d\n本文: %s", rec.Code, http.StatusNoContent, rec.Body.Bytes())
	}
	if rec.Body.Len() != 0 {
		t.Errorf("204 の本文: got %q, want 空", rec.Body.String())
	}
	assertTrades(t, pool, []storedTrade{
		{seedUserA, "6758", "buy", 50, 320000},
		{seedUserB, "7203", "buy", 10, 250000},
	})
	assertStockExists(t, pool, "7203")
}

// 基準20（要件18）: Aが売買履歴のない銘柄を削除する → 404 と {"error":"not_found"} になり、何も消えない。
// Given は insertDeleteGivenTrades の売買履歴（Aは 7203 と 6758）に、Bの 9984 を足したもの。
//   - 9984: Bだけが売買履歴を持つ（Aは売買履歴のない 9984。利用者で絞らずに銘柄だけで消す実装なら、Bの 9984 が消えて 204 になり落ちる）。
//   - 1301: 銘柄マスタにない銘柄。
//   - 5000: 銘柄マスタにあるが、誰も売買履歴を持たない銘柄（銘柄マスタに足す）。
func Test基準20_Aが売買履歴のない銘柄の削除は404で何も消えない(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		code string
	}{
		{"Bだけが売買履歴を持つ9984", "9984"},
		{"銘柄マスタにない1301", "1301"},
		{"銘柄マスタにあるが誰も売買履歴を持たない銘柄", "5000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := testdb.New(t)
			insertStock(t, pool, "5000")
			insertDeleteGivenTrades(t, pool)
			insertBuy(t, pool, seedUserB, "9984", 20, "4500")
			h := newDeleteAPI(pool, seedUserA)

			rec := serveDelete(t, h, holdingDeletePath(tc.code), sameOriginHeaders())

			assertJSONResponse(t, rec, http.StatusNotFound, notFoundJSON)
			assertTrades(t, pool, append(givenTrades(), storedTrade{seedUserB, "9984", "buy", 20, 450000}))
		})
	}
}

// 5.3「削除」: 売り切った銘柄（売買履歴はある。保有数量0）を削除すると 204 で、その銘柄の売買履歴が消える。
// 一覧に出ない銘柄でも、売買履歴が残っていれば消せる。Aの他の銘柄（7203）の売買履歴は消えない。
func Test補助_5_3_売り切った銘柄を削除すると204で売買履歴が消える(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserA, "7203", 100, "2000")
	insertBuy(t, pool, seedUserA, "6758", 10, "3200")
	insertTrade(t, pool, seedUserA, "6758", "sell", 10, "3500", "2026-02-10")
	h := newDeleteAPI(pool, seedUserA)

	rec := serveDelete(t, h, holdingDeletePath("6758"), sameOriginHeaders())

	if rec.Code != http.StatusNoContent {
		t.Errorf("ステータス: got %d, want %d\n本文: %s", rec.Code, http.StatusNoContent, rec.Body.Bytes())
	}
	assertTrades(t, pool, []storedTrade{
		{seedUserA, "7203", "buy", 100, 200000},
	})
}

// 基準20（要件18）: 削除した後に同じ銘柄をもう一度削除すると、Aの売買履歴はもうないので 404 になる。
// 1回目は 204、2回目は 404 と {"error":"not_found"}。Bの 7203 はどちらの後も残る。
func Test基準20_同じ銘柄を続けて削除すると2回目は404になる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertDeleteGivenTrades(t, pool)
	h := newDeleteAPI(pool, seedUserA)

	first := serveDelete(t, h, holdingDeletePath("7203"), sameOriginHeaders())
	second := serveDelete(t, h, holdingDeletePath("7203"), sameOriginHeaders())

	if first.Code != http.StatusNoContent {
		t.Errorf("1回目のステータス: got %d, want %d", first.Code, http.StatusNoContent)
	}
	assertJSONResponse(t, second, http.StatusNotFound, notFoundJSON)
	assertTrades(t, pool, []storedTrade{
		{seedUserA, "6758", "buy", 50, 320000},
		{seedUserB, "7203", "buy", 10, 250000},
	})
}

// 補助 4.4「パスの {code}」: 銘柄コードは URL のパスに入れて送る。r.URL.RawPath が空でないときだけ url.PathUnescape する。
// 銘柄マスタの銘柄コードは空文字以外なら何でも入る（00002_create_stocks.sql）ので、パスに特別な文字を含む銘柄を足して確かめる。
//   - "A/B": 画面は encodeURIComponent して "A%2FB" で送る。RawPath が空でないので戻して "A/B" の売買履歴が消える（戻さない実装は 404 になって落ちる）。
//   - "A%41": 画面は "A%2541" で送る。Go の既定のエンコードと同じなので RawPath は空で、戻さずに "A%41" を使う
//     （いつも戻す実装は "AA" になり、404 になって落ちる）。
//
// どちらの場合も、Aの 7203 とBの同じコードの売買履歴は消えない。
func Test補助_4_4_パスの銘柄コードは画面がencodeURIComponentで送った形から戻して削除できる(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		stockCode   string
		encodedPath string
	}{
		{"スラッシュを含むA/B", "A/B", "A%2FB"},
		{"パーセントを含むA%41", "A%41", "A%2541"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := testdb.New(t)
			insertStock(t, pool, tc.stockCode)
			insertBuy(t, pool, seedUserA, "7203", 100, "2000")
			insertBuy(t, pool, seedUserA, tc.stockCode, 30, "1500")
			insertBuy(t, pool, seedUserB, tc.stockCode, 40, "1600")
			h := newDeleteAPI(pool, seedUserA)

			rec := serveDelete(t, h, holdingDeletePath(tc.encodedPath), sameOriginHeaders())

			if rec.Code != http.StatusNoContent {
				t.Errorf("ステータス: got %d, want %d\n本文: %s", rec.Code, http.StatusNoContent, rec.Body.Bytes())
			}
			assertTrades(t, pool, []storedTrade{
				{seedUserA, "7203", "buy", 100, 200000},
				{seedUserB, tc.stockCode, "buy", 40, 160000},
			})
		})
	}
}

// errorHoldingDeleteRepository は、削除しようとすると必ず失敗する repository.HoldingDeleteRepository（境界の外側の fake）。
type errorHoldingDeleteRepository struct{}

// failingDeleteDetail は、fake が返すエラーの内容。応答に出てはいけない（4.4: 内部のエラーの内容は返さずログにだけ出す）。
const failingDeleteDetail = "fake: connection refused to db.internal:5432"

func (errorHoldingDeleteRepository) DeleteAll(context.Context, uuid.UUID, string) error {
	return errors.New(failingDeleteDetail)
}

// 補助 4.4・要件15（PR② ステップ6 のテストの網羅の指摘5）: 削除の API の内部のエラー（売買履歴を消せない）は、
// 500 と、本文がちょうど {"error":"internal_error"} になる（ErrHoldingNotFound ではない一般のエラーは 404 にならない。
// 内部のエラーの内容は応答に出さない。assertJSONResponse が本文全体を比べるので、内容が混ざれば落ちる）。
// command.NewDeleteHolding にエラーを返す fake を渡して router を組み立てる。
func Test補助_4_4_要件15_売買履歴を消せないときの削除は500とinternal_errorを返す(t *testing.T) {
	t.Parallel()
	h := router.New(router.Deps{
		HoldingDeleteHandler: handler.NewHoldingDeleteHandler(command.NewDeleteHolding(errorHoldingDeleteRepository{})),
		DevUserID:            uuid.MustParse(seedUserA),
		AllowedHosts:         []string{allowedHost},
	})

	rec := serveDelete(t, h, holdingDeletePath("7203"), sameOriginHeaders())

	assertJSONResponse(t, rec, http.StatusInternalServerError, `{"error":"internal_error"}`)
}

// 補助 4.4（PR② ステップ6 の境界値と異常系の指摘6、2ラウンド目の指摘5）: パスの銘柄コードが正しくない UTF-8（%FF、途中で切れた %E3%81）か NUL（%00）を含む（RawPath があって戻した後の値が正しくないものも）なら、
// 404 と {"error":"not_found"} になり（500 にならない）、何も消えない。
// Aは 7203 と 6758、Bは 7203 の売買履歴を持つ（どの売買履歴も消えないことを全行で確かめる）。
func Test補助_4_4_パスの銘柄コードが正しくないUTF8かNULを含む削除は404で何も消えない(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		encodedPath string
	}{
		{"正しくないUTF8の_FF", "%FF"},
		{"途中で切れたUTF8の_E3_81", "%E3%81"},
		{"NULの_00", "%00"},
		// RawPath が空でない経路（"/" を表す %2F を含むので、Go は既定のエンコードと同じにならず RawPath を持つ）。戻した後の値が正しくない。
		{"RawPathあり_2F_のあとに正しくないUTF8の_FF", "%2F%FF"},
		{"RawPathあり_A_2F_のあとにNULの_00", "A%2F%00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := testdb.New(t)
			insertDeleteGivenTrades(t, pool)
			h := newDeleteAPI(pool, seedUserA)

			rec := serveDelete(t, h, holdingDeletePath(tc.encodedPath), sameOriginHeaders())

			assertJSONResponse(t, rec, http.StatusNotFound, notFoundJSON)
			assertTrades(t, pool, givenTrades())
		})
	}
}
