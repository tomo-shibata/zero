package handler

import (
	"errors"
	"log"
	"net/http"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/interface/middleware"
	"github.com/tomo-shibata/zero/backend/internal/interface/response"
)

// StockHistoryHandler は、GET /api/holdings/{code}/history（銘柄ごとの売買履歴）を受け持つ。
type StockHistoryHandler struct {
	getStockHistory *query.GetStockHistory
}

// NewStockHistoryHandler は、q で銘柄ごとの売買履歴を返す StockHistoryHandler を返す。
func NewStockHistoryHandler(q *query.GetStockHistory) *StockHistoryHandler {
	return &StockHistoryHandler{getStockHistory: q}
}

// ServeHTTP は、ログイン中の利用者の、パスの銘柄の売買履歴・分割・終値を返す（銘柄ごとの売買履歴の画面のプラン 4.4）。
// その銘柄の売買履歴が1件もない、またはパスの銘柄コードを戻せないときは、404 と {"error":"not_found"} を返す（本文は同じ。要件20）。
// それ以外の失敗は 500 と {"error":"internal_error"} で、内部のエラーの内容は応答に出すと外に漏れるので、ログにだけ出す。
// ログにはパスや銘柄コードを入れず、固定の文言にする。クライアントが送った改行などで、偽のログの行を作らせないため。
func (h *StockHistoryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFrom(r.Context())
	if !ok {
		// router が必ず CurrentUser を通すので、ここに来るのは組み立ての誤りのときだけ。
		log.Print("api: GET /api/holdings/{code}/history: ログイン中の利用者が context にありません")
		response.Error(w, http.StatusInternalServerError, response.CodeInternalError)
		return
	}
	code, ok := pathStockCode(r)
	if !ok {
		response.Error(w, http.StatusNotFound, response.CodeNotFound)
		return
	}

	history, err := h.getStockHistory.Execute(r.Context(), userID, code)
	if err != nil {
		if errors.Is(err, query.ErrStockHistoryNotFound) {
			response.Error(w, http.StatusNotFound, response.CodeNotFound)
			return
		}
		log.Printf("api: GET /api/holdings/{code}/history: 売買履歴を取得できません: %v", err)
		response.Error(w, http.StatusInternalServerError, response.CodeInternalError)
		return
	}
	body, err := newStockHistoryResponse(history)
	if err != nil {
		// 円の値が 0.1円の単位でないなど、DB の値が決まりに合わないとき。黙って丸めず、誤った値を返さない。
		log.Printf("api: GET /api/holdings/{code}/history: 応答を作れません: %v", err)
		response.Error(w, http.StatusInternalServerError, response.CodeInternalError)
		return
	}
	response.JSON(w, http.StatusOK, body)
}
