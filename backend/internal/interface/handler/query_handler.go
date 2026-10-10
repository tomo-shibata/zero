// Package handler は、HTTP のリクエストを受けてユースケースを呼び、結果を応答にする。
package handler

import (
	"log"
	"net/http"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/interface/middleware"
	"github.com/tomo-shibata/zero/backend/internal/interface/response"
)

// QueryHandler は、状態を読むだけの API（GET）を受け持つ。
type QueryHandler struct {
	listHoldings *query.ListHoldings
}

// NewQueryHandler は、q で保有株式一覧を返す QueryHandler を返す。
func NewQueryHandler(q *query.ListHoldings) *QueryHandler {
	return &QueryHandler{listHoldings: q}
}

// ListHoldings は、GET /api/holdings に、ログイン中の利用者の保有株式一覧を返す（プラン 4.4「API」）。
// 失敗したときは 500 と {"error":"internal_error"} を返す。内部のエラーの内容（DB の接続先など）は、
// 応答に出すと外に漏れるので、ログにだけ出す。
// ログにはリクエストのパスなどを入れず、固定の文言にする。クライアントが送った改行などで、偽のログの行を作らせないため。
func (h *QueryHandler) ListHoldings(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFrom(r.Context())
	if !ok {
		// router が必ず CurrentUser を通すので、ここに来るのは組み立ての誤りのときだけ。
		log.Print("api: GET /api/holdings: ログイン中の利用者が context にありません")
		response.Error(w, http.StatusInternalServerError, response.CodeInternalError)
		return
	}

	list, err := h.listHoldings.Execute(r.Context(), userID)
	if err != nil {
		log.Printf("api: GET /api/holdings: 保有株式一覧を取得できません: %v", err)
		response.Error(w, http.StatusInternalServerError, response.CodeInternalError)
		return
	}
	response.JSON(w, http.StatusOK, newHoldingsResponse(list))
}
