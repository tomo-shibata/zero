package integration

import (
	"net/http"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/infrastructure/pricesource"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 要件（docs/requirements/price-update.md）の非機能1・基準32（別のオリジンからの書き込みの要求は、何も保存・削除せず、取得元も呼ばずに拒否する。
// アプリの画面と同じオリジンからの同じ要求は受け付ける）と、プラン（docs/plans/price-update/plan.md）5.1 の基準32 の行（PR① は今すぐ更新だけ）、
// 5.2 の PR① の判断4 の行、6章の判断4（別のサイトからの書き込みの拒否は http.CrossOriginProtection。読み取りは対象外）。
// 組み立てと要求の付け方は price_refresh_api_test.go（4.4「統合テストの要求」: Host: 127.0.0.1:8080、受け付けの組は Origin: http://localhost:5173
// と Sec-Fetch-Site: same-origin の両方）。拒否は 403 と {"error":"forbidden"}（4.4 の API の共通）。
// 取得元は、ダミーを countingPriceSource で包み、取得元に要求が届いたかを確かめる。

// 基準32（非機能1）の PR① の部分: 今すぐ更新（POST /api/price-refresh）。
// Given: Aが 7203 を保有（ダミーの取得元は 7203 の価格を持つので、受け付ければ成功する同じ要求になる）。
// プラン 5.1: 拒否の (a) Origin: http://evil.example（Sec-Fetch-Site なし） (b) Sec-Fetch-Site: cross-site (c) Sec-Fetch-Site: same-site を先に送り、
// どれも 403・何も保存されない・取得元に要求が届かない。最後に受け付けの組を送り、200 で保存が起きる
// （拒否の後に受け付けを送るので、拒否のときに実際に保存していなければ、保存されたのは受け付けの要求だと分かる）。
// (b)(c) は、Origin を画面と同じ http://localhost:5173 にして、Sec-Fetch-Site だけで拒否されることを確かめる
// （Origin が正しくても、Sec-Fetch-Site が same-origin・none 以外なら拒む。判断4）。
func Test基準32_今すぐ更新は別のオリジンからの要求を拒否し取得元を呼ばず同じオリジンの同じ要求は受け付ける(t *testing.T) {
	// サブテストを並列にしない（拒否を先に、受け付けを最後に順に送る）ので、tparallel の決まりに合わせてこのテスト自体も並列にしない。
	pool := testdb.New(t)
	insertHolding(t, pool, seedUserA, "7203", 100, "2500")
	src := &countingPriceSource{inner: pricesource.NewDummyPriceSource()}
	h := newRefreshAPI(pool, seedUserA, src)

	rejected := []struct {
		name    string
		headers map[string]string
	}{
		{"拒否a_Originがevil_exampleでSec_Fetch_Siteなし", map[string]string{"Origin": evilOrigin}},
		{"拒否b_Sec_Fetch_Siteがcross_site", map[string]string{"Origin": appOrigin, secFetchSiteField: "cross-site"}},
		{"拒否c_Sec_Fetch_Siteがsame_site", map[string]string{"Origin": appOrigin, secFetchSiteField: "same-site"}},
	}
	// 順に実行する（t.Parallel() を付けない）。拒否のあとに受け付けを送るため。
	for _, tc := range rejected {
		t.Run(tc.name+"は403で何も保存せず取得元に要求が届かない", func(t *testing.T) {
			rec := servePost(t, h, priceRefreshPath, tc.headers)

			assertJSONResponse(t, rec, http.StatusForbidden, forbiddenJSON)
			assertStockPrices(t, pool, []storedClosePrice{})
			if got := src.requested(); len(got) != 0 {
				t.Errorf("取得元に要求された銘柄: got %v, want 届かない", got)
			}
		})
	}

	t.Run("受け付けの組_OriginがlocalhostでSec_Fetch_Siteがsame_originは200で保存される", func(t *testing.T) {
		rec := servePost(t, h, priceRefreshPath, sameOriginHeaders())

		assertJSONResponse(t, rec, http.StatusOK, `{"saved":["7203"],"failed":[]}`)
		assertStockPrices(t, pool, []storedClosePrice{
			{stockCode: "7203", tradingDate: "2026-10-06", priceTenths: 30000},
		})
	})
}

// 補助 判断4（5.2）: Sec-Fetch-Site も Origin もない書き込み（ブラウザ以外の要求）は通る（CSRF はブラウザからしか起きないため）。
// Aが 7203 を保有 → POST /api/price-refresh（ヘッダーなし）→ 200 で保存される。
func Test補助_判断4_Sec_Fetch_SiteもOriginもない書き込みは通る(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertHolding(t, pool, seedUserA, "7203", 100, "2500")
	h := newRefreshAPI(pool, seedUserA, pricesource.NewDummyPriceSource())

	rec := servePost(t, h, priceRefreshPath, nil)

	assertJSONResponse(t, rec, http.StatusOK, `{"saved":["7203"],"failed":[]}`)
	assertStockPrices(t, pool, []storedClosePrice{
		{stockCode: "7203", tradingDate: "2026-10-06", priceTenths: 30000},
	})
}

// 補助 判断4（5.2）: Origin: http://localhost:5173 だけ（Sec-Fetch-Site なし）の書き込みは 403。
// Sec-Fetch-Site がないときは Origin のホストと Host（127.0.0.1:8080）を比べ、違うので拒否する（古いブラウザが拒否されるのは許容。判断4）。
// 何も保存されず、取得元に要求が届かない。
func Test補助_判断4_OriginがlocalhostだけでSec_Fetch_Siteがない書き込みはHostと違うので403(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertHolding(t, pool, seedUserA, "7203", 100, "2500")
	src := &countingPriceSource{inner: pricesource.NewDummyPriceSource()}
	h := newRefreshAPI(pool, seedUserA, src)

	rec := servePost(t, h, priceRefreshPath, map[string]string{"Origin": appOrigin})

	assertJSONResponse(t, rec, http.StatusForbidden, forbiddenJSON)
	assertStockPrices(t, pool, []storedClosePrice{})
	if got := src.requested(); len(got) != 0 {
		t.Errorf("取得元に要求された銘柄: got %v, want 届かない", got)
	}
}

// 補助 判断4（5.2）: Origin: http://evil.example の GET /api/holdings は通る（読み取りは対象外。CSRF の対策は書き込みだけ）。
// Aが保有していない状態の一覧（0件）が 200 で返る。
func Test補助_判断4_Originがevil_exampleでも読み取りのGETは通る(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	h := newRefreshAPI(pool, seedUserA, pricesource.NewDummyPriceSource())

	rec := serveGetWithHeader(t, h, holdingsPath, allowedHost, "Origin", evilOrigin)

	assertJSONResponse(t, rec, http.StatusOK, emptyListJSON)
}

// 補助 判断4（5.2）: 拒否の 403 にも Cache-Control: no-store と X-Content-Type-Options: nosniff が付く
// （応答のヘッダーを付けるミドルウェアが、別のサイトからの書き込みを拒むミドルウェアより外側にあるため。4.4）。
func Test補助_判断4_拒否の403にもno_storeとnosniffが付く(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	h := newRefreshAPI(pool, seedUserA, pricesource.NewDummyPriceSource())

	rec := servePost(t, h, priceRefreshPath, map[string]string{"Origin": evilOrigin})

	// 前提: 拒否の応答になっている。
	if rec.Code != http.StatusForbidden {
		t.Fatalf("ステータス: got %d, want %d（このテストの前提を満たさない）\n本文: %s", rec.Code, http.StatusForbidden, rec.Body.Bytes())
	}
	assertSingleHeader(t, rec, "Cache-Control", "no-store")
	assertSingleHeader(t, rec, "X-Content-Type-Options", "nosniff")
}
