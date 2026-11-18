package integration

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/write"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/router"
	"github.com/tomo-shibata/zero/backend/internal/interface/handler"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: おまかせ運用の要件（docs/requirements/managed-account.md）の要件1・要件24・非機能3 と、基準17・基準29、
// プラン（docs/plans/managed-account/plan.md）4.4（DELETE /api/managed-accounts?name=<名前>）・4.8 の PR②の境界
// （repository.ManagedAccountDeleteRepository・command.NewDeleteManagedAccount・handler.NewManagedAccountDeleteHandler・router.Deps）・
// 5.1 の基準17・基準29 の行・5.2 の PR② の「削除の API」。
// 組み立ては 4.8 の境界のとおり: write.NewManagedAccountRepository(pool) → command.NewDeleteManagedAccount → handler.NewManagedAccountDeleteHandler
// → router.New(router.Deps{…}) を httptest で呼ぶ。ログイン中の利用者は DevUserID で決まる。
// 要求には Host: 127.0.0.1:8080 と、アプリの画面と同じオリジンの組（sameOriginHeaders）を付ける（serveDelete。holding_delete_api_test.go）。
// 口座と記録は insertManagedValuation で SQL から入れ、確かめるときは readManagedValuations・readManagedAccountNames と、
// 2つの表の前後比較（snapshotManagedTables など。managed_accounts_fixtures_test.go）、5つの表の前後比較（snapshotFiveTables）を使う。

const managedAccountsPath = "/api/managed-accounts"

// managedDeletePath は、口座 name を削除する API のパス。クエリは url.Values で作る（名前の特別な文字をクエリの標準の形にする）。
func managedDeletePath(name string) string {
	return managedAccountsPath + "?" + url.Values{"name": {name}}.Encode()
}

// newManagedDeleteAPI は、pool の DB を使う、口座の削除（DELETE /api/managed-accounts）の API を、4.8 の境界のとおりに組み立てる。
// ログイン中の利用者は userID。
func newManagedDeleteAPI(pool *pgxpool.Pool, userID string) http.Handler {
	return router.New(router.Deps{
		ManagedAccountDeleteHandler: handler.NewManagedAccountDeleteHandler(command.NewDeleteManagedAccount(write.NewManagedAccountRepository(pool))),
		DevUserID:                   uuid.MustParse(userID),
		AllowedHosts:                []string{allowedHost},
	})
}

// insertDeleteGivenAccounts は、基準17 の Given を入れる。Aは WealthNavi（記録2件。2026-10-31・評価額 3,950,000円・損益 950,000円、
// 2026-11-16・4,000,000円・1,000,000円）と Alpha（記録1件。2026-11-01・500,000円・-20,000円）、Bは WealthNavi（記録1件。2026-11-16・9,000,000円・2,000,000円）。
// 値は基準17 に指定がないので、要件定義のほかの基準（基準1・基準6・基準19）の値にした。
func insertDeleteGivenAccounts(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-10-31", 3_950_000, 950_000)
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-11-16", 4_000_000, 1_000_000)
	insertManagedValuation(t, pool, seedUserA, "Alpha", "2026-11-01", 500_000, -20_000)
	insertManagedValuation(t, pool, seedUserB, "WealthNavi", "2026-11-16", 9_000_000, 2_000_000)
}

// 基準17: Aが WealthNavi（記録2件）と Alpha（記録1件）、Bが WealthNavi（記録1件）を持つ → Aが WealthNavi を削除する
// → 204（本文なし）。Aの WealthNavi の口座と記録はなくなり（readManagedValuations(A) は Alpha の1件だけ、managed_accounts に Aの WealthNavi がない）、
// Aの Alpha と Bの WealthNavi は残る（Bの行は作成日時を含めて前後で同じ）。
// 続けて Aが持っていない「ロボアド B」を削除すると 404 と {"error":"not_found"} になり、口座の2つの表（全利用者）は前後で同じ。
func Test基準17_Aが口座を削除するとAのその口座と記録だけが消えBの同じ名前の口座は残り持っていない名前は404(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertDeleteGivenAccounts(t, pool)
	h := newManagedDeleteAPI(pool, seedUserA)
	beforeB := snapshotManagedTablesOf(t, pool, seedUserB)

	rec := serveDelete(t, h, managedDeletePath("WealthNavi"), sameOriginHeaders())

	assertNoContent(t, rec)
	wantA := []storedManagedValuation{{name: "Alpha", date: "2026-11-01", valuation: 500_000, gain: -20_000}}
	if got := readManagedValuations(t, pool, seedUserA); !slices.Equal(got, wantA) {
		t.Errorf("Aの記録: got %+v, want %+v（WealthNavi の記録2件が消え、Alpha の1件が残る）", got, wantA)
	}
	if got, want := readManagedAccountNames(t, pool, seedUserA), []string{"Alpha"}; !slices.Equal(got, want) {
		t.Errorf("Aの口座: got %v, want %v（Aの WealthNavi の口座がなくなる）", got, want)
	}
	if afterB := snapshotManagedTablesOf(t, pool, seedUserB); afterB != beforeB {
		t.Errorf("Bの口座と記録が変わった:\n before %+v\n after  %+v", beforeB, afterB)
	}

	// 持っていない名前は 404 で、何も消えない。
	beforeAll := snapshotManagedTables(t, pool)
	rec = serveDelete(t, h, managedDeletePath("ロボアド B"), sameOriginHeaders())

	assertJSONResponse(t, rec, http.StatusNotFound, notFoundJSON)
	assertManagedTablesUnchanged(t, pool, beforeAll)
}

// 補助（要件24）: 同じ口座を続けて削除すると、2回目は口座がもうないので 404 になる（1回目は 204）。Bの WealthNavi は残る。
func Test補助_要件24_同じ口座を続けて削除すると2回目は404になる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertDeleteGivenAccounts(t, pool)
	h := newManagedDeleteAPI(pool, seedUserA)
	beforeB := snapshotManagedTablesOf(t, pool, seedUserB)

	first := serveDelete(t, h, managedDeletePath("WealthNavi"), sameOriginHeaders())
	second := serveDelete(t, h, managedDeletePath("WealthNavi"), sameOriginHeaders())

	if first.Code != http.StatusNoContent {
		t.Errorf("1回目のステータス: got %d, want %d", first.Code, http.StatusNoContent)
	}
	assertJSONResponse(t, second, http.StatusNotFound, notFoundJSON)
	if afterB := snapshotManagedTablesOf(t, pool, seedUserB); afterB != beforeB {
		t.Errorf("Bの口座と記録が変わった:\n before %+v\n after  %+v", beforeB, afterB)
	}
}

// 補助（4.4・プラン 5.2）: name がない・空・2つ（?name=WealthNavi&name=Alpha）・NUL を含む（?name=%00）・正しい UTF-8 でない（?name=%FF）は
// 404 と {"error":"not_found"} で、何も消えない（Aの WealthNavi も Alpha も残る）。
// NUL と正しくない UTF-8 は PostgreSQL の TEXT に入らず、DB に問い合わせると 500 になるので、DB に問い合わせずに 404 にする。
func Test補助_4_4_nameがない空か2つかNULか不正なUTF8の削除は404で何も消えない(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		path string
	}{
		{"nameがない", managedAccountsPath},
		{"nameが空", managedAccountsPath + "?name="},
		{"nameが2つ", managedAccountsPath + "?name=WealthNavi&name=Alpha"},
		{"nameがNUL", managedAccountsPath + "?name=%00"},
		{"nameが正しいUTF8でない", managedAccountsPath + "?name=%FF"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := testdb.New(t)
			insertDeleteGivenAccounts(t, pool)
			h := newManagedDeleteAPI(pool, seedUserA)
			before := snapshotManagedTables(t, pool)

			rec := serveDelete(t, h, tc.path, sameOriginHeaders())

			assertJSONResponse(t, rec, http.StatusNotFound, notFoundJSON)
			assertManagedTablesUnchanged(t, pool, before)
		})
	}
}

// 補助（要件24・プラン 5.2）: Bだけが持つ名前（Bの Alpha。記録1件）を Aが消すと 404 と {"error":"not_found"} で、
// 口座の2つの表（全利用者）は前後で同じ（Bの Alpha は消えない）。利用者で絞らずに口座の有無を確かめる誤りを見分ける。
// Aが Alpha を持たない必要があるので、insertDeleteGivenAccounts（Aが Alpha を持つ。ほかのテストの期待が変わる）は使わず、このテストの中で入れる:
// Aは WealthNavi（記録1件）、Bは WealthNavi（記録1件）と Alpha（記録1件）。
func Test補助_要件24_Bだけが持つ名前をAが消すと404でBの口座は消えない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-11-16", 4_000_000, 1_000_000)
	insertManagedValuation(t, pool, seedUserB, "WealthNavi", "2026-11-16", 9_000_000, 2_000_000)
	insertManagedValuation(t, pool, seedUserB, "Alpha", "2026-11-01", 700_000, 30_000)
	h := newManagedDeleteAPI(pool, seedUserA)
	before := snapshotManagedTables(t, pool)

	rec := serveDelete(t, h, managedDeletePath("Alpha"), sameOriginHeaders())

	assertJSONResponse(t, rec, http.StatusNotFound, notFoundJSON)
	assertManagedTablesUnchanged(t, pool, before)
}

// 補助（要件1・プラン 5.2）: 名前は書いたとおりに比べる。Aが WealthNavi を持つとき、wealthnavi（小文字）と「WealthNavi 」（後ろに空白）の削除は
// 404 と {"error":"not_found"} で、何も消えない。
func Test補助_要件1_名前は書いたとおりに比べ大文字小文字や空白が違えば404で何も消えない(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		accountName string
	}{
		{"小文字のwealthnavi", "wealthnavi"},
		{"後ろに空白のあるWealthNavi", "WealthNavi "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := testdb.New(t)
			insertDeleteGivenAccounts(t, pool)
			h := newManagedDeleteAPI(pool, seedUserA)
			before := snapshotManagedTables(t, pool)

			rec := serveDelete(t, h, managedDeletePath(tc.accountName), sameOriginHeaders())

			assertJSONResponse(t, rec, http.StatusNotFound, notFoundJSON)
			assertManagedTablesUnchanged(t, pool, before)
		})
	}
}

// 補助（判断3・プラン 5.2）: クエリで読み違えやすい名前（a&b=c・a+b・50%・x/y・ロボアド B）の口座を SQL で入れ、url.Values で作ったクエリで消すと
// 204 で、その口座だけが消える（ほかの口座は残る。name の値はクエリの標準の読み方で戻す）。
func Test補助_判断3_クエリで読み違えやすい名前の口座も名前のとおりに削除できる(t *testing.T) {
	t.Parallel()
	tricky := []string{"a&b=c", "a+b", "50%", "x/y", "ロボアド B"}
	for i, target := range tricky {
		// サブテストの名前に口座の名前を入れない（x/y の「/」が -run の区切りと読まれるため）。一覧の番号にし、名前はエラーの文に出す。
		t.Run(fmt.Sprintf("口座%d", i), func(t *testing.T) {
			t.Parallel()
			pool := testdb.New(t)
			insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-11-16", 4_000_000, 1_000_000)
			for _, name := range tricky {
				insertManagedValuation(t, pool, seedUserA, name, "2026-11-16", 500_000, 0)
			}
			h := newManagedDeleteAPI(pool, seedUserA)

			rec := serveDelete(t, h, managedDeletePath(target), sameOriginHeaders())

			assertNoContent(t, rec)
			want := []string{"WealthNavi"}
			for _, name := range tricky {
				if name != target {
					want = append(want, name)
				}
			}
			slices.Sort(want)
			if got := readManagedAccountNames(t, pool, seedUserA); !slices.Equal(got, want) {
				t.Errorf("Aの口座: got %v, want %v（%q だけが消える）", got, want, target)
			}
			if got := len(readManagedValuations(t, pool, seedUserA)); got != len(want) {
				t.Errorf("Aの記録の数: got %d, want %d（消した口座の記録も消える）", got, len(want))
			}
		})
	}
}

// 補助（要件24・プラン 5.2）: 消しても、5つの表（stock_splits・trades・stocks・stock_prices・holding_orders）は前後で同じ。
// 前提は givenManagedPool（Aの WealthNavi・Bの WealthNavi と、Aの売買履歴・終値・並び）に Aの Alpha を足したもの。
func Test補助_要件24_口座を消しても5つの表は変わらない(t *testing.T) {
	t.Parallel()
	pool := givenManagedPool(t)
	insertManagedValuation(t, pool, seedUserA, "Alpha", "2026-11-01", 500_000, -20_000)
	h := newManagedDeleteAPI(pool, seedUserA)
	before := snapshotFiveTables(t, pool)

	rec := serveDelete(t, h, managedDeletePath("WealthNavi"), sameOriginHeaders())

	assertNoContent(t, rec)
	assertFiveTablesUnchanged(t, pool, before)
}

// 基準29（非機能3）: Aが WealthNavi を持つ → 削除の API に Origin: http://evil.example を付けて要求する → 拒否（HTTP 403）され、WealthNavi は消えない。
// プラン 5.1: 拒否の (a) Origin: http://evil.example（Sec-Fetch-Site なし） (b) Sec-Fetch-Site: cross-site (c) Sec-Fetch-Site: same-site が
// どれも 403 と {"error":"forbidden"} で、口座の2つの表は変わらない。最後に受け付けの組（sameOriginHeaders()）で 204 になり消える
// （拒否の後に受け付けを送るので、拒否のときに実際に消していなければ、消えたのは受け付けの要求だと分かる）。
// (b)(c) は、Origin を画面と同じ http://localhost:5173 にして、Sec-Fetch-Site だけで拒否されることを確かめる。
// Aの Alpha と Bの WealthNavi も入れ、受け付けの組で消えるのが Aの WealthNavi だけであることまで確かめる。
func Test基準29_口座の削除は別のオリジンからの要求を拒否し何も消さず同じオリジンの同じ要求は受け付ける(t *testing.T) {
	// サブテストを並列にしない（拒否を先に、受け付けを最後に順に送る）ので、tparallel の決まりに合わせてこのテスト自体も並列にしない。
	pool := testdb.New(t)
	insertDeleteGivenAccounts(t, pool)
	h := newManagedDeleteAPI(pool, seedUserA)
	before := snapshotManagedTables(t, pool)

	rejected := []struct {
		name    string
		headers map[string]string
	}{
		{"拒否a_Originがevil_exampleでSec_Fetch_Siteなし", map[string]string{"Origin": evilOrigin}},
		{"拒否b_Sec_Fetch_Siteがcross_site", map[string]string{"Origin": appOrigin, secFetchSiteField: "cross-site"}},
		{"拒否c_Sec_Fetch_Siteがsame_site", map[string]string{"Origin": appOrigin, secFetchSiteField: "same-site"}},
	}
	for _, tc := range rejected {
		t.Run(tc.name+"は403で何も消えない", func(t *testing.T) {
			rec := serveDelete(t, h, managedDeletePath("WealthNavi"), tc.headers)

			assertJSONResponse(t, rec, http.StatusForbidden, forbiddenJSON)
			assertManagedTablesUnchanged(t, pool, before)
		})
	}

	t.Run("受け付けの組_OriginがlocalhostでSec_Fetch_Siteがsame_originは204でAのWealthNaviが消える", func(t *testing.T) {
		rec := serveDelete(t, h, managedDeletePath("WealthNavi"), sameOriginHeaders())

		assertNoContent(t, rec)
		if got, want := readManagedAccountNames(t, pool, seedUserA), []string{"Alpha"}; !slices.Equal(got, want) {
			t.Errorf("Aの口座: got %v, want %v", got, want)
		}
		if got, want := readManagedAccountNames(t, pool, seedUserB), []string{"WealthNavi"}; !slices.Equal(got, want) {
			t.Errorf("Bの口座: got %v, want %v（Bの WealthNavi は消えない）", got, want)
		}
	})
}

// errorManagedAccountDeleteRepository は、削除しようとすると必ず失敗する repository.ManagedAccountDeleteRepository（境界の外側の fake）。
type errorManagedAccountDeleteRepository struct{}

// failingManagedDeleteDetail は、fake が返すエラーの内容。応答に出てはいけない（内部のエラーの内容は返さずログにだけ出す）。
const failingManagedDeleteDetail = "fake: connection refused to db.internal:5432 (managed_accounts)"

func (errorManagedAccountDeleteRepository) DeleteManagedAccount(context.Context, uuid.UUID, string) error {
	return errors.New(failingManagedDeleteDetail)
}

// 補助（4.8・プラン 5.2）: 削除の repository が失敗する偽物（ErrManagedAccountNotFound ではない一般のエラー）→ 500 と {"error":"internal_error"}
// （404 にならない。内部のエラーの内容は応答に出さない。assertJSONResponse が本文全体を比べるので、内容が混ざれば落ちる）。
func Test補助_4_8_口座を消せないときの削除は500とinternal_errorを返す(t *testing.T) {
	t.Parallel()
	h := router.New(router.Deps{
		ManagedAccountDeleteHandler: handler.NewManagedAccountDeleteHandler(command.NewDeleteManagedAccount(errorManagedAccountDeleteRepository{})),
		DevUserID:                   uuid.MustParse(seedUserA),
		AllowedHosts:                []string{allowedHost},
	})

	rec := serveDelete(t, h, managedDeletePath("WealthNavi"), sameOriginHeaders())

	assertJSONResponse(t, rec, http.StatusInternalServerError, `{"error":"internal_error"}`)
}
