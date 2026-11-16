package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
)

// カード表示のプラン（docs/plans/holdings-cards/plan.md）4.6 PR② の「バックエンドの統合テストの補助」。
// 並びの保存（PUT /api/holding-order。基準25・32〜34 など）と、取り込み・削除の後の一覧（基準26〜29）の統合テストが使う。
// 組み立ては 4.6 の境界のとおり: read.NewHoldingsReader(pool) → query.NewListHoldings → handler.NewQueryHandler、
// write.NewHoldingOrderRepository(pool) → command.NewSaveHoldingOrder → handler.NewHoldingOrderHandler
// → router.New(router.Deps{QueryHandler, HoldingOrderHandler, …})。

const (
	// holdingOrderPath は、並びの保存の API のパス（判断13）。
	holdingOrderPath = "/api/holding-order"
	// badRequestJSON は、並びの保存の 400 の本文（4.3「保存する」）。
	badRequestJSON = `{"error":"bad_request"}`
)

// insertHoldingOrder は、利用者 userID の並びを、位置 0, 1, 2, … で holding_orders に入れる（4.6 PR② の補助）。
// 銘柄マスタに銘柄があることが前提（外部キー）。保有しているかは確かめない。
func insertHoldingOrder(t *testing.T, pool *pgxpool.Pool, userID string, codes ...string) {
	t.Helper()
	for position, code := range codes {
		if _, err := pool.Exec(t.Context(),
			"INSERT INTO holding_orders (user_id, stock_code, position) VALUES ($1::uuid, $2::text, $3::int)",
			userID, code, position,
		); err != nil {
			t.Fatalf("並び（利用者 %s・%s・位置 %d）を入れられない: %v", userID, code, position, err)
		}
	}
}

// readHoldingOrder は、利用者 userID の DB の並びを、位置の順に返す（4.6 PR② の補助）。行がなければ空のスライス。
func readHoldingOrder(t *testing.T, pool *pgxpool.Pool, userID string) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		"SELECT stock_code FROM holding_orders WHERE user_id = $1::uuid ORDER BY position", userID)
	if err != nil {
		t.Fatalf("利用者 %s の並びを読めない: %v", userID, err)
	}
	defer rows.Close()
	codes := []string{}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			t.Fatalf("並びの行を読めない: %v", err)
		}
		codes = append(codes, code)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("並びを読めない: %v", err)
	}
	return codes
}

// newHoldingsAPIFor は、pool の DB を読む、利用者 userID の API（GET /api/holdings。今の newHoldingsAPI は利用者A に固定のため。4.6 PR② の補助）。
func newHoldingsAPIFor(pool *pgxpool.Pool, userID string) http.Handler {
	return router.New(router.Deps{
		QueryHandler: handler.NewQueryHandler(query.NewListHoldings(read.NewHoldingsReader(pool))),
		DevUserID:    uuid.MustParse(userID),
		AllowedHosts: []string{allowedHost},
	})
}

// newHoldingOrderAPI は、pool の DB を使う、利用者 userID の並びの保存（PUT /api/holding-order）・一覧（GET /api/holdings）・
// 銘柄の削除（DELETE /api/holdings/{code}）の API を、4.6 の境界のとおりに組み立てる。
func newHoldingOrderAPI(pool *pgxpool.Pool, userID string) http.Handler {
	return router.New(router.Deps{
		QueryHandler:         handler.NewQueryHandler(query.NewListHoldings(read.NewHoldingsReader(pool))),
		HoldingOrderHandler:  handler.NewHoldingOrderHandler(command.NewSaveHoldingOrder(write.NewHoldingOrderRepository(pool))),
		HoldingDeleteHandler: handler.NewHoldingDeleteHandler(command.NewDeleteHolding(write.NewHoldingEditRepository(pool))),
		DevUserID:            uuid.MustParse(userID),
		AllowedHosts:         []string{allowedHost},
	})
}

// servePutOrder は、Host を allowedHost にし、headers と Content-Type: application/json を付けた PUT /api/holding-order を、本文 body で h に送り、応答を返す。
func servePutOrder(t *testing.T, h http.Handler, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, holdingOrderPath, strings.NewReader(body))
	req.Host = allowedHost
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// putOrderBody は、並び codes の本文 {"codes":[...]} を作る。
func putOrderBody(codes ...string) string {
	b, err := json.Marshal(map[string][]string{"codes": codes})
	if err != nil {
		panic(err) // []string の JSON 化は失敗しない
	}
	return string(b)
}

// assertNoContent は、応答が 204 で本文が空であることを確かめる。
func assertNoContent(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusNoContent {
		t.Errorf("ステータス: got %d, want %d\n本文: %s", rec.Code, http.StatusNoContent, rec.Body.Bytes())
	}
	if rec.Body.Len() != 0 {
		t.Errorf("本文: got %q, want 空", rec.Body.String())
	}
}

// listedCodes は、h の GET /api/holdings の応答の holdings の銘柄コードを、並びの順に返す。200 でなければテストを止める。
func listedCodes(t *testing.T, h http.Handler) []string {
	t.Helper()
	rec := serveGet(t, h, holdingsPath, allowedHost)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/holdings: got %d, want 200\n本文: %s", rec.Code, rec.Body.Bytes())
	}
	var body struct {
		Holdings []struct {
			Code string `json:"code"`
		} `json:"holdings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET /api/holdings の本文を読めない: %v\n本文: %s", err, rec.Body.Bytes())
	}
	codes := []string{}
	for _, row := range body.Holdings {
		codes = append(codes, row.Code)
	}
	return codes
}
