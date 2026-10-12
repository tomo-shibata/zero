package handler

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/interface/middleware"
	"github.com/tomo-shibata/zero/backend/internal/interface/response"
)

// priceRefreshTimeout は、今すぐ更新の1回にかける時間の上限（値段の更新のプラン 6章の判断8）。
// 本番の取得元が応答しないときに、要求が終わらなくならないため。超えたら、残りの銘柄は取得せずに failed に入れて 200 を返す。
const priceRefreshTimeout = 60 * time.Second

// PriceRefreshHandler は、POST /api/price-refresh（価格の今すぐ更新）を受け持つ。
type PriceRefreshHandler struct {
	refresh *command.RefreshPrices
}

// NewPriceRefreshHandler は、c で今すぐ更新を行う PriceRefreshHandler を返す。
func NewPriceRefreshHandler(c *command.RefreshPrices) *PriceRefreshHandler {
	return &PriceRefreshHandler{refresh: c}
}

// priceRefreshResponse は、POST /api/price-refresh の本文。どちらも銘柄コードの昇順。
type priceRefreshResponse struct {
	Saved  []string `json:"saved"`
	Failed []string `json:"failed"`
}

// ServeHTTP は、ログイン中の利用者の保有銘柄の終値を取得して保存し、結果を 200 で返す（値段の更新のプラン 4.4「API」）。
// 保有銘柄を読めないときは 500 と {"error":"internal_error"} を返す。内部のエラーの内容は応答に出すと外に漏れるので、ログにだけ出す。
// 上限（priceRefreshTimeout）を超えたときは、エラーにせず、取得できなかった銘柄を failed に入れて 200 を返す。
// ログにはリクエストのパスなどを入れず、固定の文言にする。クライアントが送った改行などで、偽のログの行を作らせないため。
func (h *PriceRefreshHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFrom(r.Context())
	if !ok {
		// router が必ず CurrentUser を通すので、ここに来るのは組み立ての誤りのときだけ。
		log.Print("api: POST /api/price-refresh: ログイン中の利用者が context にありません")
		response.Error(w, http.StatusInternalServerError, response.CodeInternalError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), priceRefreshTimeout)
	defer cancel()
	result, err := h.refresh.Execute(ctx, userID)
	if err != nil {
		log.Printf("api: POST /api/price-refresh: 価格を更新できません: %v", err)
		response.Error(w, http.StatusInternalServerError, response.CodeInternalError)
		return
	}
	response.JSON(w, http.StatusOK, newPriceRefreshResponse(result))
}

// newPriceRefreshResponse は、取得処理の結果を API の本文の形にする。
// 0件でも saved・failed を null ではなく [] にするため、nil ではないスライスにそろえる。
func newPriceRefreshResponse(result command.FetchResult) priceRefreshResponse {
	return priceRefreshResponse{
		Saved:  append([]string{}, result.Saved...),
		Failed: append([]string{}, result.Failed...),
	}
}
