package integration

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: カード表示の要件定義（docs/requirements/holdings-cards.md）の要件24・要件26・要件31・非機能1 と、成功基準の基準25・基準32〜34、
// プラン（docs/plans/holdings-cards/plan.md）4.3「保存する」・5.1 の PR② の行・5.3 の PR②（統合）の行。
// 保存は PUT /api/holding-order、本文 {"codes":[...]}。成功は 204（本文なし）、不正は 400 と {"error":"bad_request"}、別のオリジンは 403。
// 銘柄マスタは test.sql の 7203・6758・9984 に、AAPL（Apple・USD）を足して使う（givenCardsPool）。
// 期待する並びは、要件・基準に書かれたものをそのまま書く。

// givenCardsPool は、銘柄マスタに AAPL（Apple・USD）を足した DB を作る。
func givenCardsPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testdb.New(t)
	insertStock(t, pool, "AAPL", "Apple", currencyUSD)
	return pool
}

// holdBuys は、利用者 userID が codes の銘柄を保有している状態（買い 10株 を1件ずつ）を作る。
func holdBuys(t *testing.T, pool *pgxpool.Pool, userID string, codes ...string) {
	t.Helper()
	for _, code := range codes {
		insertBuy(t, pool, userID, code, 10, "1000")
	}
}

// 基準25（要件24・要件26）: AとBがどちらも 6758、7203、AAPL を保有し、Aは並びを AAPL、7203、6758 に保存している
// → Bの一覧の並びは 6758、7203、AAPL（銘柄コードの昇順）。Aの一覧の並びは AAPL、7203、6758。
func Testカード表示の基準25_並びは利用者ごとで保存していない利用者は銘柄コードの昇順(t *testing.T) {
	t.Parallel()
	pool := givenCardsPool(t)
	holdBuys(t, pool, seedUserA, "6758", "7203", "AAPL")
	holdBuys(t, pool, seedUserB, "6758", "7203", "AAPL")
	insertHoldingOrder(t, pool, seedUserA, "AAPL", "7203", "6758")

	gotB := listedCodes(t, newHoldingsAPIFor(pool, seedUserB))
	gotA := listedCodes(t, newHoldingsAPIFor(pool, seedUserA))

	if want := []string{"6758", "7203", "AAPL"}; !slices.Equal(gotB, want) {
		t.Errorf("Bの一覧の並び: got %v, want %v（保存していないので銘柄コードの昇順）", gotB, want)
	}
	if want := []string{"AAPL", "7203", "6758"}; !slices.Equal(gotA, want) {
		t.Errorf("Aの一覧の並び: got %v, want %v（保存した並び）", gotA, want)
	}
}

// 基準32（要件31）: Aが 6758、7203、AAPL を保有し、並びを AAPL、7203、6758 に保存している
// → Aの並びとして (a) 7203、7203、6758、AAPL (b) 7203、6758 (c) 7203、6758、AAPL、9984 (d) 7203、6758、AAPL、XXXX（銘柄マスタにない）
// を保存しようとすると、どれも 400 と {"error":"bad_request"} で、Aの並びは AAPL、7203、6758 のまま（DB と一覧の両方）。
func Testカード表示の基準32_保有している銘柄の集まりと一致しない並びは400で何も変えない(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		codes []string
	}{
		{"a_重複がある", []string{"7203", "7203", "6758", "AAPL"}},
		{"b_足りない", []string{"7203", "6758"}},
		{"c_保有していない銘柄を含む", []string{"7203", "6758", "AAPL", "9984"}},
		{"d_銘柄マスタにない銘柄を含む", []string{"7203", "6758", "AAPL", "XXXX"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := givenCardsPool(t)
			holdBuys(t, pool, seedUserA, "6758", "7203", "AAPL")
			insertHoldingOrder(t, pool, seedUserA, "AAPL", "7203", "6758")
			h := newHoldingOrderAPI(pool, seedUserA)

			rec := servePutOrder(t, h, putOrderBody(tc.codes...), sameOriginHeaders())

			assertJSONResponse(t, rec, http.StatusBadRequest, badRequestJSON)
			if got, want := readHoldingOrder(t, pool, seedUserA), []string{"AAPL", "7203", "6758"}; !slices.Equal(got, want) {
				t.Errorf("DB の並び: got %v, want %v（何も変えない）", got, want)
			}
			if got, want := listedCodes(t, h), []string{"AAPL", "7203", "6758"}; !slices.Equal(got, want) {
				t.Errorf("一覧の並び: got %v, want %v", got, want)
			}
		})
	}
}

// 基準33（要件24・要件31）: AとBがどちらも 6758、7203 を保有し、Bは並びを 7203、6758 に保存している
// → Aが自分の並びを 6758、7203 で保存すると 204 で、Bの並びは 7203、6758 のまま。Aの並びは送った 6758、7203 になる（5.1）。
func Testカード表示の基準33_Aの保存はBの並びを変えない(t *testing.T) {
	t.Parallel()
	pool := givenCardsPool(t)
	holdBuys(t, pool, seedUserA, "6758", "7203")
	holdBuys(t, pool, seedUserB, "6758", "7203")
	insertHoldingOrder(t, pool, seedUserB, "7203", "6758")

	rec := servePutOrder(t, newHoldingOrderAPI(pool, seedUserA), putOrderBody("6758", "7203"), sameOriginHeaders())

	assertNoContent(t, rec)
	if got, want := readHoldingOrder(t, pool, seedUserB), []string{"7203", "6758"}; !slices.Equal(got, want) {
		t.Errorf("Bの並び: got %v, want %v（Aの保存で変わらない）", got, want)
	}
	if got, want := readHoldingOrder(t, pool, seedUserA), []string{"6758", "7203"}; !slices.Equal(got, want) {
		t.Errorf("Aの並び: got %v, want %v（送った並び）", got, want)
	}
}

// 基準33 の補助（要件31）: Aが、Bだけが保有している銘柄（Aは保有していない）を含む並びを送ると 400 で、AもBも変わらない。
// 他の利用者の保有で集まりが一致してしまわない（ログイン中の利用者の保有で比べる）ことを確かめる。
func Testカード表示の基準33補助_Aが保有していない銘柄をBが保有していてもAの並びとしては400(t *testing.T) {
	t.Parallel()
	pool := givenCardsPool(t)
	holdBuys(t, pool, seedUserA, "6758", "7203")
	holdBuys(t, pool, seedUserB, "6758", "7203", "AAPL")
	insertHoldingOrder(t, pool, seedUserA, "7203", "6758")
	insertHoldingOrder(t, pool, seedUserB, "AAPL", "7203", "6758")

	rec := servePutOrder(t, newHoldingOrderAPI(pool, seedUserA), putOrderBody("AAPL", "7203", "6758"), sameOriginHeaders())

	assertJSONResponse(t, rec, http.StatusBadRequest, badRequestJSON)
	if got, want := readHoldingOrder(t, pool, seedUserA), []string{"7203", "6758"}; !slices.Equal(got, want) {
		t.Errorf("Aの並び: got %v, want %v", got, want)
	}
	if got, want := readHoldingOrder(t, pool, seedUserB), []string{"AAPL", "7203", "6758"}; !slices.Equal(got, want) {
		t.Errorf("Bの並び: got %v, want %v", got, want)
	}
}

// 基準34（非機能1）: Aが 6758、7203、AAPL を保有し、並びを AAPL、7203、6758 に保存している。
// プラン 5.1: 拒否の (a) Origin: http://evil.example（Sec-Fetch-Site なし） (b) Sec-Fetch-Site: cross-site (c) Sec-Fetch-Site: same-site を先に送り、
// どれも 403 と {"error":"forbidden"}・並びは変わらない。最後に同じオリジンの組（Origin: http://localhost:5173 と Sec-Fetch-Site: same-origin）で
// 同じ要求を送ると 204 で、並びが送った並び（7203、AAPL、6758）になる（拒否の後に受け付けを送るので、拒否のときに保存していなければ、
// 変わったのは受け付けの要求だと分かる）。(b)(c) は Origin を画面と同じにして、Sec-Fetch-Site だけで拒否されることを確かめる。
func Testカード表示の基準34_並びの保存は別のオリジンからの要求を拒否し同じオリジンの同じ要求は受け付ける(t *testing.T) {
	// サブテストを並列にしない（拒否を先に、受け付けを最後に順に送る）ので、tparallel の決まりに合わせてこのテスト自体も並列にしない。
	pool := givenCardsPool(t)
	holdBuys(t, pool, seedUserA, "6758", "7203", "AAPL")
	insertHoldingOrder(t, pool, seedUserA, "AAPL", "7203", "6758")
	h := newHoldingOrderAPI(pool, seedUserA)
	body := putOrderBody("7203", "AAPL", "6758")

	rejected := []struct {
		name    string
		headers map[string]string
	}{
		{"拒否a_Originがevil_exampleでSec_Fetch_Siteなし", map[string]string{"Origin": evilOrigin}},
		{"拒否b_Sec_Fetch_Siteがcross_site", map[string]string{"Origin": appOrigin, secFetchSiteField: "cross-site"}},
		{"拒否c_Sec_Fetch_Siteがsame_site", map[string]string{"Origin": appOrigin, secFetchSiteField: "same-site"}},
	}
	for _, tc := range rejected {
		t.Run(tc.name+"は403で並びは変わらない", func(t *testing.T) {
			rec := servePutOrder(t, h, body, tc.headers)

			assertJSONResponse(t, rec, http.StatusForbidden, forbiddenJSON)
			if got, want := readHoldingOrder(t, pool, seedUserA), []string{"AAPL", "7203", "6758"}; !slices.Equal(got, want) {
				t.Errorf("DB の並び: got %v, want %v（拒否したので変わらない）", got, want)
			}
		})
	}

	t.Run("受け付けの組_OriginがlocalhostでSec_Fetch_Siteがsame_originは204で並びが送った並びになる", func(t *testing.T) {
		rec := servePutOrder(t, h, body, sameOriginHeaders())

		assertNoContent(t, rec)
		if got, want := readHoldingOrder(t, pool, seedUserA), []string{"7203", "AAPL", "6758"}; !slices.Equal(got, want) {
			t.Errorf("DB の並び: got %v, want %v", got, want)
		}
		if got, want := listedCodes(t, h), []string{"7203", "AAPL", "6758"}; !slices.Equal(got, want) {
			t.Errorf("一覧の並び: got %v, want %v", got, want)
		}
	})
}

// 補助（5.3）PUT の 400: 本文が JSON でない・本文が空・codes が文字列・知らない項目・64KiB を超える本文は、どれも 400 と
// {"error":"bad_request"} で、並びは変わらない。Aが 6758、7203 を保有し、並びを 7203、6758 に保存している。
func Testカード表示の補助_PUTの本文の誤りは400で並びは変わらない(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
	}{
		{"JSONでない", "not json"},
		{"本文が空", ""},
		{"codesが文字列", `{"codes":"7203"}`},
		{"codesの要素が文字列でない", `{"codes":[7203,6758]}`},
		{"知らない項目がある", `{"codes":["6758","7203"],"extra":1}`},
		// 空白を本文の前に置く（JSON の値を読むために、空白をすべて読む必要があるようにする）。
		{"64KiBを超える本文", strings.Repeat(" ", 64*1024) + `{"codes":["6758","7203"]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := givenCardsPool(t)
			holdBuys(t, pool, seedUserA, "6758", "7203")
			insertHoldingOrder(t, pool, seedUserA, "7203", "6758")

			rec := servePutOrder(t, newHoldingOrderAPI(pool, seedUserA), tc.body, sameOriginHeaders())

			assertJSONResponse(t, rec, http.StatusBadRequest, badRequestJSON)
			if got, want := readHoldingOrder(t, pool, seedUserA), []string{"7203", "6758"}; !slices.Equal(got, want) {
				t.Errorf("DB の並び: got %v, want %v（何も変えない）", got, want)
			}
		})
	}
}

// 補助（5.3・4.3）保有が0件の利用者での本文の決まり。集まりの比べだけでは見分けられない誤り（codes がなくても空の並びとして受け付ける、
// 知らない項目を無視する、64KiB を超える本文を読む）を、保有0件の利用者（Aは何も保有していない）で確かめる:
//   - {"codes":[]} は集まりが一致するので 204（行は0件のまま）
//   - codes がない・null・知らない項目がある・64KiB を超える本文は、空の並びと読めても 400
//   - 本文がちょうど 64KiB（超えない）なら 204
func Testカード表示の補助_保有が0件の利用者の本文の決まり(t *testing.T) {
	t.Parallel()
	const limit = 64 * 1024
	// 空の並びの JSON を、前に空白を置いて目的の長さにそろえる（JSON の値を読むために、空白をすべて読む必要があるようにする）。
	padded := func(size int) string {
		core := `{"codes":[]}`
		return strings.Repeat(" ", size-len(core)) + core
	}
	cases := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"codesが空の配列は204", `{"codes":[]}`, http.StatusNoContent},
		{"本文がちょうど64KiBなら204", padded(limit), http.StatusNoContent},
		{"codesがない本文は400", `{}`, http.StatusBadRequest},
		{"codesがnullは400", `{"codes":null}`, http.StatusBadRequest},
		{"知らない項目がある本文は400", `{"codes":[],"extra":1}`, http.StatusBadRequest},
		{"64KiBを超える本文は400", padded(limit + 1), http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := givenCardsPool(t)

			rec := servePutOrder(t, newHoldingOrderAPI(pool, seedUserA), tc.body, sameOriginHeaders())

			if tc.wantStatus == http.StatusNoContent {
				assertNoContent(t, rec)
			} else {
				assertJSONResponse(t, rec, tc.wantStatus, badRequestJSON)
			}
			if got := readHoldingOrder(t, pool, seedUserA); len(got) != 0 {
				t.Errorf("DB の並び: got %v, want 空（行は0件のまま）", got)
			}
		})
	}
}

// 補助（4.3「保存する」）: 保存の後の一覧は送った並びで返る。置き直す（保存し直す）と、前の並びの行は残らず、送った並びだけになる。
// Aが 6758、7203、AAPL を保有 → AAPL、7203、6758 を保存 → 6758、AAPL、7203 を保存し直す。
func Testカード表示の補助_保存し直すと前の並びは残らず一覧は送った並びで返る(t *testing.T) {
	t.Parallel()
	pool := givenCardsPool(t)
	holdBuys(t, pool, seedUserA, "6758", "7203", "AAPL")
	h := newHoldingOrderAPI(pool, seedUserA)

	first := servePutOrder(t, h, putOrderBody("AAPL", "7203", "6758"), sameOriginHeaders())
	second := servePutOrder(t, h, putOrderBody("6758", "AAPL", "7203"), sameOriginHeaders())

	assertNoContent(t, first)
	assertNoContent(t, second)
	if got, want := readHoldingOrder(t, pool, seedUserA), []string{"6758", "AAPL", "7203"}; !slices.Equal(got, want) {
		t.Errorf("DB の並び: got %v, want %v", got, want)
	}
	if got, want := listedCodes(t, h), []string{"6758", "AAPL", "7203"}; !slices.Equal(got, want) {
		t.Errorf("一覧の並び: got %v, want %v", got, want)
	}
}
