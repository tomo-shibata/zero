package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/interface/middleware"
	"github.com/tomo-shibata/zero/backend/internal/interface/response"
)

// maxHoldingOrderBody は、並びの保存の本文の大きさの上限（64KiB。カード表示のプラン 4.3「保存する」）。
// 並びは銘柄コードの配列だけなので、これを超える本文は読まずに拒否する。
const maxHoldingOrderBody = 64 * 1024

// HoldingOrderHandler は、PUT /api/holding-order（並びの保存）を受け持つ。
type HoldingOrderHandler struct {
	saver *command.SaveHoldingOrder
}

// NewHoldingOrderHandler は、c で並びを保存する HoldingOrderHandler を返す。
func NewHoldingOrderHandler(c *command.SaveHoldingOrder) *HoldingOrderHandler {
	return &HoldingOrderHandler{saver: c}
}

// holdingOrderRequest は、PUT /api/holding-order の本文。codes がない・null のときは nil のままになるので、{"codes":[]} と見分けられる。
type holdingOrderRequest struct {
	Codes []string `json:"codes"`
}

// ServeHTTP は、ログイン中の利用者の並びを、本文の codes（銘柄コードを表示する順に）にして、204（本文なし）を返す（カード表示のプラン 4.3）。
// 本文が JSON でない・codes がない（null を含む）・配列でない・知らない項目がある・64KiB を超える、
// または codes が保有している銘柄（保有数量が1以上）とちょうど同じ集まりでないときは、何も変えずに 400 と {"error":"bad_request"} を返す（要件31）。
// それ以外の失敗は 500 と {"error":"internal_error"} で、内部のエラーの内容は応答に出すと外に漏れるので、ログにだけ出す。
// ログには本文やパスを入れず、固定の文言にする。クライアントが送った改行などで、偽のログの行を作らせないため。
func (h *HoldingOrderHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFrom(r.Context())
	if !ok {
		// router が必ず CurrentUser を通すので、ここに来るのは組み立ての誤りのときだけ。
		log.Print("api: PUT /api/holding-order: ログイン中の利用者が context にありません")
		response.Error(w, http.StatusInternalServerError, response.CodeInternalError)
		return
	}
	codes, ok := readHoldingOrderCodes(w, r)
	if !ok {
		response.Error(w, http.StatusBadRequest, response.CodeBadRequest)
		return
	}

	if err := h.saver.Execute(r.Context(), userID, codes); err != nil {
		if errors.Is(err, command.ErrInvalidOrder) {
			response.Error(w, http.StatusBadRequest, response.CodeBadRequest)
			return
		}
		log.Printf("api: PUT /api/holding-order: 並びを保存できません: %v", err)
		response.Error(w, http.StatusInternalServerError, response.CodeInternalError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// readHoldingOrderCodes は、本文を読んで codes を返す。本文が決まりに合わなければ ok を false にする。
func readHoldingOrderCodes(w http.ResponseWriter, r *http.Request) (codes []string, ok bool) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxHoldingOrderBody))
	dec.DisallowUnknownFields()
	var req holdingOrderRequest
	if err := dec.Decode(&req); err != nil {
		return nil, false
	}
	// JSON の値の後ろに、空白以外の何かがあるとき（2つ目の値、本文の大きさの超過）は誤り。
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, false
	}
	if req.Codes == nil {
		return nil, false
	}
	return req.Codes, true
}
