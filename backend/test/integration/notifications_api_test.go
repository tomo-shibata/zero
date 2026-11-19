package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/read"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/write"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/router"
	"github.com/tomo-shibata/zero/backend/internal/interface/handler"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 価格の変動の通知の要件（docs/requirements/price-alerts.md）の基準27（要件52 のうち通知の分）・基準39（要件34）・基準54（非機能1 のうち通知を見たにする）と、
// プラン（docs/plans/price-alerts/plan.md）4.5「API の形」（通知の3つ）・4.8 の PR②・5.1 の基準27・基準39・基準54 の PR② の行・5.2 の
// notifications_api_test.go の行（並び・101件目・latestId、upToId の 400）。
// 組み立ては 4.8 の境界のとおり:
//
//	read.NewNotificationsReader(pool) → query.NewListNotifications・query.NewCountUnseenNotifications、
//	write.NewNotificationRepository(pool) → command.NewMarkNotificationsSeen
//	→ handler.NewNotificationsHandler → router.New(router.Deps{NotificationsHandler: …}) を httptest で呼ぶ。
//
// ログイン中の利用者は DevUserID で決まる。書き込み（POST /api/notifications/seen）には、アプリの画面と同じオリジンの組（sameOriginHeaders）を付ける。
// 通知は判定を通さず、notifications に直接入れる（要件定義の成功基準の冒頭の「通知（直接）」）。条件の補助は PR① の price_alerts_fixtures_test.go のものを使う。

const (
	notificationsPath        = "/api/notifications"
	unseenCountPath          = "/api/notifications/unseen-count"
	notificationsSeenPath    = "/api/notifications/seen"
	badRequestBodyJSON       = `{"error":"bad_request"}`
	notificationsMaxListSize = 100
)

// newNotificationsAPI は、pool の DB を使う通知の API（一覧・見ていない数・見たにする）を、4.8 の境界のとおりに組み立てる。ログイン中の利用者は userID。
func newNotificationsAPI(pool *pgxpool.Pool, userID string) http.Handler {
	reader := read.NewNotificationsReader(pool)
	return router.New(router.Deps{
		NotificationsHandler: handler.NewNotificationsHandler(
			query.NewListNotifications(reader),
			query.NewCountUnseenNotifications(reader),
			command.NewMarkNotificationsSeen(write.NewNotificationRepository(pool)),
		),
		DevUserID:    uuid.MustParse(userID),
		AllowedHosts: []string{allowedHost},
	})
}

// servePostJSON は、Host を allowedHost にし、headers と Content-Type: application/json を付けた POST path を、本文 body で h に送り、応答を返す。
func servePostJSON(t *testing.T, h http.Handler, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(body))
	req.Host = allowedHost
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// seenRequestBody は、通知を見たにする API の本文 {"upToId":"<upToID>"}。
func seenRequestBody(upToID string) string {
	b, err := json.Marshal(map[string]string{"upToId": upToID})
	if err != nil {
		panic(err) // map[string]string の JSON 化は失敗しない
	}
	return string(b)
}

// notifSeed は、notifications に直接入れる通知1件（要件定義の「通知（直接）」）。
type notifSeed struct {
	userID             string // 空なら利用者A
	conditionID        int64
	kind               string // 空なら "cost"
	scope              string // 空なら "all"
	conditionStockCode string // 全銘柄なら ""
	direction          string // 空なら "down"
	threshold          string // 空なら "20"
	stockCode          string // 空なら "7203"
	tradingDate        string
	closePrice         string // 空なら "1990"
	details            string // 空なら {"rateTenths":-204,"basePriceHundredths":250000}
	seen               bool
}

// insertNotif は、通知 n を notifications に入れ、その id を返す。呼んだ順が id の昇順になる。
func insertNotif(t *testing.T, pool *pgxpool.Pool, n notifSeed) int64 {
	t.Helper()
	or := func(v, def string) string {
		if v == "" {
			return def
		}
		return v
	}
	var id int64
	err := pool.QueryRow(t.Context(),
		`INSERT INTO notifications (user_id, condition_id, kind, scope, condition_stock_code, direction, threshold,
		                            stock_code, trading_date, close_price, details, seen)
		 VALUES ($1::uuid, $2::bigint, $3::text, $4::text, $5::text, $6::text, $7::text::numeric,
		         $8::text, $9::text::date, $10::text::numeric, $11::text::jsonb, $12::boolean)
		 RETURNING id`,
		or(n.userID, seedUserA), n.conditionID, or(n.kind, "cost"), or(n.scope, "all"), n.conditionStockCode, or(n.direction, "down"), or(n.threshold, "20"),
		or(n.stockCode, "7203"), n.tradingDate, or(n.closePrice, "1990"), or(n.details, `{"rateTenths":-204,"basePriceHundredths":250000}`), n.seen,
	).Scan(&id)
	if err != nil {
		t.Fatalf("通知（%+v）を入れられない: %v", n, err)
	}
	return id
}

// seenFlags は、notifications のすべての行（全利用者）の seen を、id ごとに返す。
func seenFlags(t *testing.T, pool *pgxpool.Pool) map[int64]bool {
	t.Helper()
	rows, err := pool.Query(t.Context(), "SELECT id, seen FROM notifications")
	if err != nil {
		t.Fatalf("notifications を読めない: %v", err)
	}
	defer rows.Close()
	flags := map[int64]bool{}
	for rows.Next() {
		var id int64
		var seen bool
		if err := rows.Scan(&id, &seen); err != nil {
			t.Fatalf("notifications を読めない: %v", err)
		}
		flags[id] = seen
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("notifications を読めない: %v", err)
	}
	return flags
}

// listedNotification は、一覧の応答の1件のうち、並びと新着の確かめに使う項目。
type listedNotification struct {
	ID          string `json:"id"`
	StockCode   string `json:"stockCode"`
	TradingDate string `json:"tradingDate"`
	IsNew       bool   `json:"isNew"`
	Condition   struct {
		Scope string `json:"scope"`
	} `json:"condition"`
}

type listedNotifications struct {
	Items    []listedNotification `json:"items"`
	LatestID *string              `json:"latestId"`
}

// getNotificationList は、h の GET /api/notifications を呼び、200 の応答を読んで返す。
func getNotificationList(t *testing.T, h http.Handler) listedNotifications {
	t.Helper()
	rec := serveGet(t, h, notificationsPath, allowedHost)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s のステータス: got %d, want 200\n本文: %s", notificationsPath, rec.Code, rec.Body.Bytes())
	}
	var list listedNotifications
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("一覧の本文を読めない: %v\n本文: %s", err, rec.Body.Bytes())
	}
	return list
}

func idString(id int64) string { return strconv.FormatInt(id, 10) }

// 基準27（要件52 のうち通知の分）: Aの条件: 全銘柄・取得価格から 20% 以上 下がったら、Bの条件: 全銘柄・取得価格から 30% 以上 下がったら。
// Aの通知1件（見ていない）、Bの通知2件（見ていない）。
// → Aが通知の一覧・見ていない数を取得すると、Aの通知1件、数は1だけが返る（4.5 の例のとおりの形）。
// Aが通知を見たにする（upToId はBの通知より大きい ID）と、Bの2件は見ていないまま（Aの1件だけが見たになる）。Bの数は2のまま。
func Test基準27_通知は利用者ごとで他の利用者の通知は一覧にも数にも出ず見たにもならない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	condA := insertAllCondition(t, pool, seedUserA, "cost", "down", "20")
	condB := insertAllCondition(t, pool, seedUserB, "cost", "down", "30")
	// Bの通知を先に入れる（Aの通知の id がBの通知の id より大きくなる）。
	idB1 := insertNotif(t, pool, notifSeed{userID: seedUserB, conditionID: condB, threshold: "30", tradingDate: "2026-11-12", closePrice: "1700"})
	idB2 := insertNotif(t, pool, notifSeed{userID: seedUserB, conditionID: condB, threshold: "30", tradingDate: "2026-11-13", closePrice: "1700"})
	idA := insertNotif(t, pool, notifSeed{conditionID: condA, tradingDate: "2026-11-14"})
	hA := newNotificationsAPI(pool, seedUserA)
	hB := newNotificationsAPI(pool, seedUserB)

	list := serveGet(t, hA, notificationsPath, allowedHost)
	count := serveGet(t, hA, unseenCountPath, allowedHost)

	assertJSONResponse(t, list, http.StatusOK, fmt.Sprintf(`{
		"items": [{
			"id": %q, "stockCode": "7203", "stockName": "トヨタ自動車", "currency": "JPY",
			"tradingDate": "2026-11-14", "isNew": true,
			"condition": {"kind": "cost", "scope": "all", "stockCode": null, "direction": "down", "threshold": "20.0"},
			"close": "1990.0", "rate": "-20.4", "basePrice": "2500.0", "baseDate": null,
			"unrealizedGain": null, "quantity": null, "adjustedThreshold": null
		}],
		"latestId": %q
	}`, idString(idA), idString(idA)))
	assertJSONResponse(t, count, http.StatusOK, `{"count":1}`)

	seen := servePostJSON(t, hA, notificationsSeenPath, seenRequestBody(idString(idA)), sameOriginHeaders())

	assertNoContent(t, seen)
	got := seenFlags(t, pool)
	if got[idB1] || got[idB2] {
		t.Errorf("Bの通知の seen: got %v, want どちらも false（Aが見たにしても、Bの通知は見たにならない）", got)
	}
	if !got[idA] {
		t.Errorf("Aの通知の seen: got false, want true")
	}
	assertJSONResponse(t, serveGet(t, hA, unseenCountPath, allowedHost), http.StatusOK, `{"count":0}`)
	assertJSONResponse(t, serveGet(t, hB, unseenCountPath, allowedHost), http.StatusOK, `{"count":2}`)
	if bList := getNotificationList(t, hB); len(bList.Items) != 2 || !bList.Items[0].IsNew || !bList.Items[1].IsNew {
		t.Errorf("Bの一覧: got %+v, want 見ていない2件", bList)
	}
}

// 基準39（要件34）: Aの見ていない通知が2件。Aが通知の一覧を取得し、その後に新しい通知が1件でき、続けて取得した時点までの通知
// （取得した latestId）を見たにする → 見ていない通知は、後からできた1件だけ（数は1）。
func Test基準39_見たにするのは取得した時点までの通知で後からできた通知は見ていないまま(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	n1 := insertNotif(t, pool, notifSeed{conditionID: 1, tradingDate: "2026-11-12"})
	n2 := insertNotif(t, pool, notifSeed{conditionID: 1, tradingDate: "2026-11-13"})
	h := newNotificationsAPI(pool, seedUserA)

	list := getNotificationList(t, h)
	n3 := insertNotif(t, pool, notifSeed{conditionID: 1, tradingDate: "2026-11-14"})

	if len(list.Items) != 2 || !list.Items[0].IsNew || !list.Items[1].IsNew {
		t.Fatalf("取得した一覧: got %+v, want 見ていない（isNew）2件", list)
	}
	if list.LatestID == nil || *list.LatestID != idString(n2) {
		t.Fatalf("latestId: got %v, want %q（取得した時点の最大の ID）", list.LatestID, idString(n2))
	}
	rec := servePostJSON(t, h, notificationsSeenPath, seenRequestBody(*list.LatestID), sameOriginHeaders())

	assertNoContent(t, rec)
	assertJSONResponse(t, serveGet(t, h, unseenCountPath, allowedHost), http.StatusOK, `{"count":1}`)
	want := map[int64]bool{n1: true, n2: true, n3: false}
	if got := seenFlags(t, pool); !mapsEqual(got, want) {
		t.Errorf("seen: got %v, want %v（取得した時点までの2件だけ見たにする）", got, want)
	}
}

func mapsEqual(a, b map[int64]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// 基準54（非機能1 のうち通知を見たにする）: Aの見ていない通知がある。
// 見たにするを、(a) Origin: http://evil.example（Sec-Fetch-Site なし） (b) Sec-Fetch-Site: cross-site (c) Sec-Fetch-Site: same-site で要求すると、
// どれも 403 と {"error":"forbidden"} で seen は変わらない。最後に、アプリの画面と同じオリジンの組で同じ要求をすると 204 で、見たになる。
func Test基準54_通知を見たにするは別のオリジンからの要求を拒否し同じオリジンの要求は受け付ける(t *testing.T) {
	// サブテストを並列にしない（拒否を先に、受け付けを最後に順に送る）ので、このテスト自体も並列にしない。
	pool := testdb.New(t)
	id := insertNotif(t, pool, notifSeed{conditionID: 1, tradingDate: "2026-11-14"})
	h := newNotificationsAPI(pool, seedUserA)
	body := seenRequestBody(idString(id))

	rejected := []struct {
		name    string
		headers map[string]string
	}{
		{"拒否a_Originがevil_exampleでSec_Fetch_Siteなし", map[string]string{"Origin": evilOrigin}},
		{"拒否b_Sec_Fetch_Siteがcross_site", map[string]string{"Origin": appOrigin, secFetchSiteField: "cross-site"}},
		{"拒否c_Sec_Fetch_Siteがsame_site", map[string]string{"Origin": appOrigin, secFetchSiteField: "same-site"}},
	}
	for _, tc := range rejected {
		t.Run(tc.name+"は403でseenは変わらない", func(t *testing.T) {
			rec := servePostJSON(t, h, notificationsSeenPath, body, tc.headers)

			assertJSONResponse(t, rec, http.StatusForbidden, forbiddenJSON)
			if got := seenFlags(t, pool); got[id] {
				t.Errorf("seen: got true, want false（拒否したので変わらない）")
			}
		})
	}

	t.Run("受け付けの組は204で見たになる", func(t *testing.T) {
		rec := servePostJSON(t, h, notificationsSeenPath, body, sameOriginHeaders())

		assertNoContent(t, rec)
		if got := seenFlags(t, pool); !got[id] {
			t.Errorf("seen: got false, want true")
		}
	})
}

// 5.2（4.5 の並び）: 取引日の降順 → 銘柄コードの昇順（COLLATE "C"。130A・130a・7203・AAPL）→ 全銘柄の条件が先 → condition_id の昇順。
// 通知は、並びの結果と入れた順（id）・条件 ID の順が重ならないように入れる。見た通知は isNew が false。
func Test補助_通知の並びは取引日の降順から銘柄コードの昇順から全銘柄の条件が先から条件IDの昇順(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStock(t, pool, "130A", "テスト銘柄130A", currencyJPY)
	insertStock(t, pool, "130a", "テスト銘柄130a", currencyJPY)
	insertStock(t, pool, "AAPL", "Apple", currencyUSD)
	// 入れた順に id が小さい。
	oldest := insertNotif(t, pool, notifSeed{conditionID: 21, scope: "stock", conditionStockCode: "7203", stockCode: "7203", tradingDate: "2026-11-13", seen: true})
	// 小文字の 130a は、大文字の 130A より先（id が小さい）に入れる。バイト順（COLLATE "C"）では 130A が 130a より前。DB の既定の照合順序では 130a が 130A より前になる。
	code130a := insertNotif(t, pool, notifSeed{conditionID: 25, stockCode: "130a", tradingDate: "2026-11-14"})
	aapl := insertNotif(t, pool, notifSeed{conditionID: 22, stockCode: "AAPL", closePrice: "190.00", tradingDate: "2026-11-14"})
	toyota := insertNotif(t, pool, notifSeed{conditionID: 23, stockCode: "7203", tradingDate: "2026-11-14"})
	code130A := insertNotif(t, pool, notifSeed{conditionID: 24, stockCode: "130A", tradingDate: "2026-11-14"})
	// 2026-11-15 の3件は、期待する並びが id の昇順とも降順とも違うように、32・1銘柄・33 の順に入れる。
	sonyAll32 := insertNotif(t, pool, notifSeed{conditionID: 32, stockCode: "6758", tradingDate: "2026-11-15"})
	sonyStock := insertNotif(t, pool, notifSeed{conditionID: 31, scope: "stock", conditionStockCode: "6758", stockCode: "6758", tradingDate: "2026-11-15"})
	sonyAll33 := insertNotif(t, pool, notifSeed{conditionID: 33, stockCode: "6758", tradingDate: "2026-11-15"})
	h := newNotificationsAPI(pool, seedUserA)

	list := getNotificationList(t, h)

	gotIDs := make([]string, len(list.Items))
	for i, item := range list.Items {
		gotIDs[i] = item.ID
	}
	wantIDs := []string{
		idString(sonyAll32), idString(sonyAll33), idString(sonyStock), // 2026-11-15 の 6758: 全銘柄の条件（条件 ID の昇順）→ 1銘柄の条件
		idString(code130A), idString(code130a), idString(toyota), idString(aapl), // 2026-11-14: 130A・130a・7203・AAPL（バイト順）
		idString(oldest), // 2026-11-13
	}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Errorf("通知の並び（id）:\n got %v\nwant %v", gotIDs, wantIDs)
	}
	for _, item := range list.Items {
		if wantNew := item.ID != idString(oldest); item.IsNew != wantNew {
			t.Errorf("通知 %s の isNew: got %v, want %v（見た通知だけ false）", item.ID, item.IsNew, wantNew)
		}
	}
}

// 5.2: 通知が101件あるとき、一覧は100件まで（101件目＝いちばん古い取引日の通知は出ない）。latestId は、一覧に出ない101件目を含む、その利用者の最大の ID。
// 取引日が古いほど id が大きくなるように入れる（101件目が最大の id を持つ）。その latestId で見たにすると、出ていない1件も見たになる（見ていない数は0）。
func Test補助_通知は100件までで_latestIdは一覧に出ない101件目を含む最大のIDで見たにする(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	// 2026-04-11（i=100）から 2026-01-01（i=0）の順に入れる。i=0（2026-01-01）が最も古く、id は最大。
	for i := 100; i >= 0; i-- {
		insertNotifOnDayOffset(t, pool, i)
	}
	h := newNotificationsAPI(pool, seedUserA)

	list := getNotificationList(t, h)

	if len(list.Items) != notificationsMaxListSize {
		t.Fatalf("一覧の件数: got %d, want %d", len(list.Items), notificationsMaxListSize)
	}
	if got := list.Items[0].TradingDate; got != "2026-04-11" {
		t.Errorf("先頭の取引日: got %q, want 2026-04-11（新しい順）", got)
	}
	if got := list.Items[len(list.Items)-1].TradingDate; got != "2026-01-02" {
		t.Errorf("末尾の取引日: got %q, want 2026-01-02（2026-01-01 の101件目は出ない）", got)
	}
	var maxID int64
	if err := pool.QueryRow(t.Context(), "SELECT max(id) FROM notifications").Scan(&maxID); err != nil {
		t.Fatalf("最大の id を読めない: %v", err)
	}
	if list.LatestID == nil || *list.LatestID != idString(maxID) {
		t.Fatalf("latestId: got %v, want %q（一覧に出ない101件目を含む最大の ID）", list.LatestID, idString(maxID))
	}
	rec := servePostJSON(t, h, notificationsSeenPath, seenRequestBody(*list.LatestID), sameOriginHeaders())

	assertNoContent(t, rec)
	assertJSONResponse(t, serveGet(t, h, unseenCountPath, allowedHost), http.StatusOK, `{"count":0}`)
}

// insertNotifOnDayOffset は、Aの通知（7203・見ていない）を、取引日が 2026-01-01 の offset 日後のものとして入れる（条件 ID は offset+1 で、組が重ならない）。
func insertNotifOnDayOffset(t *testing.T, pool *pgxpool.Pool, offset int) {
	t.Helper()
	var date string
	if err := pool.QueryRow(t.Context(), "SELECT to_char('2026-01-01'::date + $1::int, 'YYYY-MM-DD')", offset).Scan(&date); err != nil {
		t.Fatalf("日付を作れない: %v", err)
	}
	insertNotif(t, pool, notifSeed{conditionID: int64(offset) + 1, tradingDate: date})
}

// 5.2: 通知が0件のとき、一覧は {"items":[],"latestId":null}（items は null でなく空の配列）、見ていない数は 0。
func Test補助_通知が0件のとき一覧は空の配列でlatestIdはnullで数は0(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	h := newNotificationsAPI(pool, seedUserA)

	assertJSONResponse(t, serveGet(t, h, notificationsPath, allowedHost), http.StatusOK, `{"items":[],"latestId":null}`)
	assertJSONResponse(t, serveGet(t, h, unseenCountPath, allowedHost), http.StatusOK, `{"count":0}`)
}

// プラン 6章の判断14: details の形が誤っている行（配列・値が文字列）が1件でもあるとき、一覧は 500 と {"error":"internal_error"} で、
// 正しい行があっても途中までの本文を返さない（保有株式一覧と同じ。1件でも形が誤りなら全体を失敗にする）。種類は取得価格からで、率（rateTenths）が要る。
func Test補助_判断14_通知のdetailsの形が誤っている行があると一覧は500でinternal_errorを返す(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		details string
	}{
		{"detailsが配列", `[]`},
		{"rateTenthsが文字列", `{"rateTenths":"x"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := testdb.New(t)
			insertNotif(t, pool, notifSeed{conditionID: 1, tradingDate: "2026-11-13"})
			insertNotif(t, pool, notifSeed{conditionID: 2, tradingDate: "2026-11-14", details: tc.details})
			h := newNotificationsAPI(pool, seedUserA)

			rec := serveGet(t, h, notificationsPath, allowedHost)

			assertJSONResponse(t, rec, http.StatusInternalServerError, `{"error":"internal_error"}`)
		})
	}
}

// 5.2（4.8 の MarkSeen）: upToId が空・"abc"・"0"・負の数のとき、400 と {"error":"bad_request"} で、通知は見たにならない。
// 本文に upToId がないとき（{}）も空と同じ。先頭が 0・+・空白の文字、int64 を超える数、JSON でない本文、知らない項目、2つ目の値、4KiB を超える本文も 400
// （ステップ6の1ラウンド目の指摘 13。キーの大文字・小文字を区別しないのは害がないので許す＝表に入れない）。
func Test補助_見たにするのupToIdが正の整数の文字列でなければ400で何も変えない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	id := insertNotif(t, pool, notifSeed{conditionID: 1, tradingDate: "2026-11-14"})
	h := newNotificationsAPI(pool, seedUserA)

	for _, tc := range []struct {
		name string
		body string
	}{
		{"空", seenRequestBody("")},
		{"abc", seenRequestBody("abc")},
		{"0", seenRequestBody("0")},
		{"負の数", seenRequestBody("-1")},
		{"小数", seenRequestBody("1.5")},
		{"upToIdがない", `{}`},
		{"先頭が0の015", seenRequestBody("015")},
		{"先頭が+の+15", seenRequestBody("+15")},
		{"先頭に空白のある 15", seenRequestBody(" 15")},
		{"int64を超える2の63乗", seenRequestBody("9223372036854775808")},
		{"JSONでない本文", `not json`},
		{"知らない項目がある", `{"upToId":"1","x":1}`},
		{"2つ目の値が続く", `{"upToId":"1"}{"upToId":"2"}`},
		{"4KiBを超える本文", `{"upToId":"1"` + strings.Repeat(" ", 4096+1) + `}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := servePostJSON(t, h, notificationsSeenPath, tc.body, sameOriginHeaders())

			assertJSONResponse(t, rec, http.StatusBadRequest, badRequestBodyJSON)
			if got := seenFlags(t, pool); got[id] {
				t.Errorf("seen: got true, want false（400 のときは見たにしない）")
			}
		})
	}
}
