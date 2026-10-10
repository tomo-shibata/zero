package integration

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/router"
	"github.com/tomo-shibata/zero/backend/internal/interface/handler"
)

// 仕様: プラン（docs/plans/stock-holdings/plan.md）4.4「API」の Host の許可リストとヘッダーの決まり、
// 6章の判断12。確かめるのは、5.1「補助のテスト（バックエンド）」の次の行（PR② ステップ6 の指摘で足したもの）。
//   - 判断12（PR② の S-1、EC-5）: Host はポートまで含めて、大文字小文字を区別せずに完全一致で比べる。
//     許可リストが空なら /api/ はすべて 403。X-Forwarded-Host などのヘッダーは見ない。外れたら 403 と {"error":"forbidden"}。
//   - 4.4 API のヘッダー（PR② の S-2）: /api/ の応答には Cache-Control: no-store と X-Content-Type-Options: nosniff を付ける。
//
// 組み立ては holdings_api_test.go と同じ 4.4 の境界（router.New(router.Deps{…})）。保有データは境界の外側の fake から渡す。

// emptyListJSON は、保有データが0件のときの 200 の本文（4.4「API」）。
const emptyListJSON = `{"holdings":[],"totalValuation":0,"totalExcludesUnpriced":false}`

// forbiddenJSON は、Host が許可リストにないときの 403 の本文（4.4「API」、判断12）。
const forbiddenJSON = `{"error":"forbidden"}`

// newHoldingsAPIWithHosts は、newHoldingsAPI と同じ組み立てで、Deps.AllowedHosts を hosts にした API を返す。
func newHoldingsAPIWithHosts(reader query.HoldingsReader, hosts []string) http.Handler {
	return router.New(router.Deps{
		QueryHandler: handler.NewQueryHandler(query.NewListHoldings(reader)),
		DevUserID:    uuid.MustParse(seedUserA),
		AllowedHosts: hosts,
	})
}

// serveGetWithHeader は、Host ヘッダーを host にし、ヘッダー name に value を付けた GET path を h に送り、応答を返す。
func serveGetWithHeader(t *testing.T, h http.Handler, path, host, name, value string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	req.Host = host
	req.Header.Set(name, value)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// 補助 判断12（PR② の S-1、EC-5）: AllowedHosts が ["127.0.0.1:8080","localhost:8080"] の router で、
// Host が attacker.example:8080・127.0.0.1:9999・127.0.0.1（ポートなし）なら 403。
// Host が example.com で X-Forwarded-Host: 127.0.0.1:8080 を付けても 403。Host が LOCALHOST:8080 なら 200。
func Test判断12_PR2_S1_EC5_Hostはポートまで含め大文字小文字を区別せず完全一致で比べ転送のヘッダーは見ない(t *testing.T) {
	t.Parallel()
	h := newHoldingsAPIWithHosts(emptyHoldingsReader{}, []string{"127.0.0.1:8080", "localhost:8080"})

	forbidden := []struct {
		name string
		host string
	}{
		{"許可していないホストで許可したポート_attacker.example:8080", "attacker.example:8080"},
		{"許可したホストで許可していないポート_127.0.0.1:9999", "127.0.0.1:9999"},
		{"許可したホストでポートなし_127.0.0.1", "127.0.0.1"},
	}
	for _, tc := range forbidden {
		t.Run(tc.name+"は403", func(t *testing.T) {
			t.Parallel()
			rec := serveGet(t, h, holdingsPath, tc.host)
			assertJSONResponse(t, rec, http.StatusForbidden, forbiddenJSON)
		})
	}

	t.Run("Hostがexample.comならX-Forwarded-Hostに許可したHostを付けても403", func(t *testing.T) {
		t.Parallel()
		rec := serveGetWithHeader(t, h, holdingsPath, "example.com", "X-Forwarded-Host", "127.0.0.1:8080")
		assertJSONResponse(t, rec, http.StatusForbidden, forbiddenJSON)
	})

	t.Run("大文字のLOCALHOST:8080は200", func(t *testing.T) {
		t.Parallel()
		rec := serveGet(t, h, holdingsPath, "LOCALHOST:8080")
		assertJSONResponse(t, rec, http.StatusOK, emptyListJSON)
	})

	// 対照: 同じ router で、許可リストのとおりの Host なら 200（上の 403 が Host 以外の理由でないことを確かめる）。
	for _, host := range []string{"127.0.0.1:8080", "localhost:8080"} {
		t.Run("対照_"+host+"は200", func(t *testing.T) {
			t.Parallel()
			rec := serveGet(t, h, holdingsPath, host)
			assertJSONResponse(t, rec, http.StatusOK, emptyListJSON)
		})
	}
}

// 補助 判断12（PR② の S-1、EC-5）: AllowedHosts が空の router では、Host が 127.0.0.1:8080 でも 403。
// 空は、指定しない（nil）場合と長さ0のスライスの場合の両方で確かめる。
func Test判断12_PR2_S1_EC5_許可リストが空ならapiはHostが127_0_0_1_8080でも403(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		hosts []string
	}{
		{"AllowedHostsがnil", nil},
		{"AllowedHostsが長さ0のスライス", []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHoldingsAPIWithHosts(emptyHoldingsReader{}, tc.hosts)

			rec := serveGet(t, h, holdingsPath, "127.0.0.1:8080")

			assertJSONResponse(t, rec, http.StatusForbidden, forbiddenJSON)
		})
	}
}

// 補助 4.4 API のヘッダー（PR② の S-2）: GET /api/holdings の 200・500・403 の応答に、
// Cache-Control: no-store と X-Content-Type-Options: nosniff が付く（資産データをキャッシュに残さないため）。
// 500 は保有データを読めない fake（FR-22 と同じ）、403 は許可リストにない Host（判断12）で起こす。
func TestPlan4_4_PR2_S2_API_api_holdingsの応答は200でも500でも403でもno_storeとnosniffを付ける(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		reader     query.HoldingsReader
		host       string
		wantStatus int
	}{
		{"200_保有データを読めた", emptyHoldingsReader{}, allowedHost, http.StatusOK},
		{"500_保有データを読めない", failingHoldingsReader{}, allowedHost, http.StatusInternalServerError},
		{"403_許可リストにないHost", emptyHoldingsReader{}, "example.com", http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHoldingsAPI(tc.reader)

			rec := serveGet(t, h, holdingsPath, tc.host)

			// 前提: 確かめたいステータスの応答になっている。
			if rec.Code != tc.wantStatus {
				t.Fatalf("ステータス: got %d, want %d（このケースの前提を満たさない）\n本文: %s", rec.Code, tc.wantStatus, rec.Body.Bytes())
			}
			assertSingleHeader(t, rec, "Cache-Control", "no-store")
			assertSingleHeader(t, rec, "X-Content-Type-Options", "nosniff")
		})
	}
}

// assertSingleHeader は、応答のヘッダー name がちょうど1つあり、その値が want であることを確かめる。
func assertSingleHeader(t *testing.T, rec *httptest.ResponseRecorder, name, want string) {
	t.Helper()
	if got := rec.Header().Values(name); !slices.Equal(got, []string{want}) {
		t.Errorf("ヘッダー %s: got %q, want [%q]", name, got, want)
	}
}
