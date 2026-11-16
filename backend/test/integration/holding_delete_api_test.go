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

// 仕様: 要件（docs/requirements/price-update.md）の基準23（銘柄ごとの削除。PR② の分）と要件14、
// プラン（docs/plans/price-update/plan.md）5.1 の基準23 の行（PR② では「Bの 7203 とAの 6758 が残る」と 404 を確かめる。
// 手入力の価格が残ることは manual_stock_prices ができる PR③ で足す）、4.4 の API の表（DELETE /api/holdings/{code}: 204、
// ログイン中の利用者がその銘柄を1件も保有していない → 404 と {"error":"not_found"}）と
// 「パスの {code}」の決まり（r.URL.RawPath が空でないときだけ url.PathUnescape する）。
// 組み立ては 4.4 の境界のとおり:
//   command.NewDeleteHolding(write.NewHoldingEditRepository(pool)) → handler.NewHoldingDeleteHandler
//   → router.New(router.Deps{HoldingDeleteHandler: …}) を httptest で呼ぶ。
// ログイン中の利用者は DevUserID で決まる。要求には 4.4 のとおり Host: 127.0.0.1:8080 と、アプリの画面と同じオリジンの組を付ける。

const notFoundJSON = `{"error":"not_found"}`

// holdingDeletePath は、銘柄 code（URL のパスに入れる形のまま）を削除する API のパス。
func holdingDeletePath(code string) string {
	return holdingsPath + "/" + code
}

// newDeleteAPI は、pool の DB を使う、銘柄ごとの削除（DELETE /api/holdings/{code}）の API を、4.4 の境界のとおりに組み立てる。
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

// storedHolding は、holdings の1行のうち、テストで確かめる値。取得価格は 0.1円単位の整数（例: 20000 は 2,000.0円）。
type storedHolding struct {
	userID                 string
	stockCode              string
	quantity               int64
	acquisitionPriceTenths int64
}

// readHoldings は、holdings のすべての行（全利用者）を、利用者・銘柄コード・登録順（id の昇順）で返す。
func readHoldings(t *testing.T, pool *pgxpool.Pool) []storedHolding {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT user_id::text, stock_code, quantity, (acquisition_price * 10)::bigint
		   FROM holdings
		  ORDER BY user_id, stock_code, id`)
	if err != nil {
		t.Fatalf("holdings を読めない: %v", err)
	}
	holdings, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (storedHolding, error) {
		var h storedHolding
		err := row.Scan(&h.userID, &h.stockCode, &h.quantity, &h.acquisitionPriceTenths)
		return h, err
	})
	if err != nil {
		t.Fatalf("holdings を読めない: %v", err)
	}
	return holdings
}

// assertHoldings は、holdings の全行（全利用者。利用者・銘柄コード・登録順）が want とちょうど同じであることを確かめる。
// 消えるべきでない行が消えたことも、消えるべき行が残ったことも失敗にする。
func assertHoldings(t *testing.T, pool *pgxpool.Pool, want []storedHolding) {
	t.Helper()
	if got := readHoldings(t, pool); !slices.Equal(got, want) {
		t.Errorf("holdings の行（利用者・銘柄コード・数量・0.1円単位の取得価格）:\n got %+v\nwant %+v", got, want)
	}
}

// insertDeleteGivenHoldings は、基準23 の Given の保有データを入れる。
// Aが 7203 を2件（100株・2,000円、300株・3,000円）と 6758 を1件（50株・3,200円）、Bが 7203 を1件（10株・2,500円）。
// 数量・取得価格は基準23 に指定がないので、基準3 の 7203 の2件の値と、PR① のテストで使った値にした（削除の結果に関係しない）。
// この Given の holdings の全行（assertHoldings に渡す順: 利用者・銘柄コード・登録順）は givenHoldings。
func insertDeleteGivenHoldings(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	insertHolding(t, pool, seedUserA, "7203", 100, "2000")
	insertHolding(t, pool, seedUserA, "7203", 300, "3000")
	insertHolding(t, pool, seedUserA, "6758", 50, "3200")
	insertHolding(t, pool, seedUserB, "7203", 10, "2500")
}

// givenHoldings は、insertDeleteGivenHoldings が入れた holdings の全行。
func givenHoldings() []storedHolding {
	return []storedHolding{
		{seedUserA, "6758", 50, 32000},
		{seedUserA, "7203", 100, 20000},
		{seedUserA, "7203", 300, 30000},
		{seedUserB, "7203", 10, 25000},
	}
}

// 基準23（要件14）の PR② の部分: Aが 7203 を2件と 6758 を1件、Bが 7203 を1件保有 → Aが 7203 を削除する
// → 204（本文なし）。Aの 7203 の保有データは0件になり、Aの 6758 とBの 7203 は残る。
// Aの 7203 を2件にするのは、1件だけ消す実装を見分けるため。Bの 7203 を置くのは、利用者で絞らず銘柄だけで消す実装を、
// Aの 6758 を置くのは、利用者の保有データを全部消す実装（銘柄で絞らない）を見分けるため。
func Test基準23_Aが7203を削除するとAの7203の保有データだけが全件消えBの7203とAの6758は残る(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertDeleteGivenHoldings(t, pool)
	h := newDeleteAPI(pool, seedUserA)

	rec := serveDelete(t, h, holdingDeletePath("7203"), sameOriginHeaders())

	if rec.Code != http.StatusNoContent {
		t.Errorf("ステータス: got %d, want %d\n本文: %s", rec.Code, http.StatusNoContent, rec.Body.Bytes())
	}
	if rec.Body.Len() != 0 {
		t.Errorf("204 の本文: got %q, want 空", rec.Body.String())
	}
	assertHoldings(t, pool, []storedHolding{
		{seedUserA, "6758", 50, 32000},
		{seedUserB, "7203", 10, 25000},
	})
}

// 基準23（要件14）の PR② の部分: Aが保有していない銘柄を削除する → 404 と {"error":"not_found"} になり、何も消えない。
// Given は insertDeleteGivenHoldings の保有データ（Aは 7203 と 6758）に、Bの 9984 を足したもの。
//   - 9984: Bだけが保有。利用者で絞らずに銘柄だけで消す実装なら、Bの 9984 が消えて 204 になり落ちる。
//   - 1301: 銘柄マスタにない銘柄（要件14「ログイン中の利用者が保有していない銘柄の削除」）。
//   - 5000: 銘柄マスタにあるが、誰も保有していない銘柄（銘柄マスタに足す）。
func Test基準23_Aが保有していない銘柄の削除は404で何も消えない(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		code string
	}{
		{"Bだけが保有する9984", "9984"},
		{"銘柄マスタにない1301", "1301"},
		{"銘柄マスタにあるが誰も保有していない銘柄", "5000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := testdb.New(t)
			insertStock(t, pool, "5000")
			insertDeleteGivenHoldings(t, pool)
			insertHolding(t, pool, seedUserB, "9984", 20, "4500")
			h := newDeleteAPI(pool, seedUserA)

			rec := serveDelete(t, h, holdingDeletePath(tc.code), sameOriginHeaders())

			assertJSONResponse(t, rec, http.StatusNotFound, notFoundJSON)
			assertHoldings(t, pool, append(givenHoldings(), storedHolding{seedUserB, "9984", 20, 45000}))
		})
	}
}

// 基準23（要件14）の PR② の部分: 削除した後に同じ銘柄をもう一度削除すると、Aはもう保有していないので 404 になる。
// 1回目は 204、2回目は 404 と {"error":"not_found"}。Bの 7203 はどちらの後も残る。
func Test基準23_同じ銘柄を続けて削除すると2回目は404になる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertDeleteGivenHoldings(t, pool)
	h := newDeleteAPI(pool, seedUserA)

	first := serveDelete(t, h, holdingDeletePath("7203"), sameOriginHeaders())
	second := serveDelete(t, h, holdingDeletePath("7203"), sameOriginHeaders())

	if first.Code != http.StatusNoContent {
		t.Errorf("1回目のステータス: got %d, want %d", first.Code, http.StatusNoContent)
	}
	assertJSONResponse(t, second, http.StatusNotFound, notFoundJSON)
	assertHoldings(t, pool, []storedHolding{
		{seedUserA, "6758", 50, 32000},
		{seedUserB, "7203", 10, 25000},
	})
}

// 補助 4.4「パスの {code}」: 銘柄コードは URL のパスに入れて送る。r.URL.RawPath が空でないときだけ url.PathUnescape する。
// 銘柄マスタの銘柄コードは空文字以外なら何でも入る（00002_create_stocks.sql）ので、パスに特別な文字を含む銘柄を足して確かめる。
//   - "A/B": 画面は encodeURIComponent して "A%2FB" で送る。RawPath が空でないので戻して "A/B" の保有データが消える（戻さない実装は 404 になって落ちる）。
//   - "A%41": 画面は "A%2541" で送る。Go の既定のエンコードと同じなので RawPath は空で、戻さずに "A%41" を使う
//     （いつも戻す実装は "AA" になり、404 になって落ちる）。
//
// どちらの場合も、Aの 7203 とBの同じコードの保有データは消えない。
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
			insertHolding(t, pool, seedUserA, "7203", 100, "2000")
			insertHolding(t, pool, seedUserA, tc.stockCode, 30, "1500")
			insertHolding(t, pool, seedUserB, tc.stockCode, 40, "1600")
			h := newDeleteAPI(pool, seedUserA)

			rec := serveDelete(t, h, holdingDeletePath(tc.encodedPath), sameOriginHeaders())

			if rec.Code != http.StatusNoContent {
				t.Errorf("ステータス: got %d, want %d\n本文: %s", rec.Code, http.StatusNoContent, rec.Body.Bytes())
			}
			assertHoldings(t, pool, []storedHolding{
				{seedUserA, "7203", 100, 20000},
				{seedUserB, tc.stockCode, 40, 16000},
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

// 補助 4.4・要件15（PR② ステップ6 のテストの網羅の指摘5）: 削除の API の内部のエラー（保有データを消せない）は、
// 500 と、本文がちょうど {"error":"internal_error"} になる（ErrHoldingNotFound ではない一般のエラーは 404 にならない。
// 内部のエラーの内容は応答に出さない。assertJSONResponse が本文全体を比べるので、内容が混ざれば落ちる）。
// command.NewDeleteHolding にエラーを返す fake を渡して router を組み立てる。
func Test補助_4_4_要件15_保有データを消せないときの削除は500とinternal_errorを返す(t *testing.T) {
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
// Aは 7203 と 6758、Bは 7203 を保有する（どの保有データも消えないことを全行で確かめる）。
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
			insertDeleteGivenHoldings(t, pool)
			h := newDeleteAPI(pool, seedUserA)

			rec := serveDelete(t, h, holdingDeletePath(tc.encodedPath), sameOriginHeaders())

			assertJSONResponse(t, rec, http.StatusNotFound, notFoundJSON)
			assertHoldings(t, pool, givenHoldings())
		})
	}
}
