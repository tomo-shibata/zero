package handler

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/interface/middleware"
	"github.com/tomo-shibata/zero/backend/internal/interface/response"
)

// ManagedAccountDeleteHandler は、DELETE /api/managed-accounts?name=…（おまかせ運用の口座の削除）を受け持つ。
type ManagedAccountDeleteHandler struct {
	deleter *command.DeleteManagedAccount
}

// NewManagedAccountDeleteHandler は、c で口座を削除する ManagedAccountDeleteHandler を返す。
func NewManagedAccountDeleteHandler(c *command.DeleteManagedAccount) *ManagedAccountDeleteHandler {
	return &ManagedAccountDeleteHandler{deleter: c}
}

// ServeHTTP は、ログイン中の利用者の、クエリ name の口座を記録ごと削除し、204（本文なし）を返す（おまかせ運用のプラン 4.4・判断3）。
// name は、クエリの標準の読み方で戻した値を、書いたとおりに比べる。name がちょうど1つで空でなく、正しい UTF-8 で NUL を含まないときだけ消し、
// ない・空・2つ以上・正しくない文字のとき、またはその名前の口座がないときは、404 と {"error":"not_found"} を返す。
// それ以外の失敗は 500 と {"error":"internal_error"} で、内部のエラーの内容は応答に出すと外に漏れるので、ログにだけ出す。
// ログにはクエリや名前を入れず、固定の文言にする。クライアントが送った改行などで、偽のログの行を作らせないため。
func (h *ManagedAccountDeleteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFrom(r.Context())
	if !ok {
		// router が必ず CurrentUser を通すので、ここに来るのは組み立ての誤りのときだけ。
		log.Print("api: DELETE /api/managed-accounts: ログイン中の利用者が context にありません")
		response.Error(w, http.StatusInternalServerError, response.CodeInternalError)
		return
	}
	names := r.URL.Query()["name"]
	// 正しい UTF-8 でない・NUL を含む名前の口座は作れないので、DB には問い合わせず 404 にする。
	// NUL は PostgreSQL の TEXT に入らず、問い合わせが 500 になるため（株式の削除の pathStockCode と同じ）。
	if len(names) != 1 || names[0] == "" || !utf8.ValidString(names[0]) || strings.ContainsRune(names[0], 0) {
		response.Error(w, http.StatusNotFound, response.CodeNotFound)
		return
	}

	if err := h.deleter.Execute(r.Context(), userID, names[0]); err != nil {
		if errors.Is(err, command.ErrNotFound) {
			response.Error(w, http.StatusNotFound, response.CodeNotFound)
			return
		}
		log.Printf("api: DELETE /api/managed-accounts: 口座を削除できません: %v", err)
		response.Error(w, http.StatusInternalServerError, response.CodeInternalError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
