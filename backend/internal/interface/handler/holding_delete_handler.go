package handler

import (
	"errors"
	"log"
	"net/http"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/interface/middleware"
	"github.com/tomo-shibata/zero/backend/internal/interface/response"
)

// HoldingDeleteHandler は、DELETE /api/holdings/{code}（銘柄ごとの削除）を受け持つ。
type HoldingDeleteHandler struct {
	deleter *command.DeleteHolding
}

// NewHoldingDeleteHandler は、c で銘柄を削除する HoldingDeleteHandler を返す。
func NewHoldingDeleteHandler(c *command.DeleteHolding) *HoldingDeleteHandler {
	return &HoldingDeleteHandler{deleter: c}
}

// ServeHTTP は、ログイン中の利用者の、パスの銘柄の保有データをすべて削除し、204（本文なし）を返す（値段の更新のプラン 4.4「API」）。
// その銘柄を1件も保有していない、またはパスの銘柄コードを戻せないときは 404 と {"error":"not_found"} を返す。
// それ以外の失敗は 500 と {"error":"internal_error"} で、内部のエラーの内容は応答に出すと外に漏れるので、ログにだけ出す。
// ログにはパスや銘柄コードを入れず、固定の文言にする。クライアントが送った改行などで、偽のログの行を作らせないため。
func (h *HoldingDeleteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFrom(r.Context())
	if !ok {
		// router が必ず CurrentUser を通すので、ここに来るのは組み立ての誤りのときだけ。
		log.Print("api: DELETE /api/holdings/{code}: ログイン中の利用者が context にありません")
		response.Error(w, http.StatusInternalServerError, response.CodeInternalError)
		return
	}
	code, ok := pathStockCode(r)
	if !ok {
		response.Error(w, http.StatusNotFound, response.CodeNotFound)
		return
	}

	if err := h.deleter.Execute(r.Context(), userID, code); err != nil {
		if errors.Is(err, command.ErrNotFound) {
			response.Error(w, http.StatusNotFound, response.CodeNotFound)
			return
		}
		log.Printf("api: DELETE /api/holdings/{code}: 保有データを削除できません: %v", err)
		response.Error(w, http.StatusInternalServerError, response.CodeInternalError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
