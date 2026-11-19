package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/interface/middleware"
	"github.com/tomo-shibata/zero/backend/internal/interface/response"
)

// maxNotificationsSeenBody は、通知を見たにする本文の大きさの上限（4KiB）。本文は { "upToId": "<整数>" } だけなので、これを超える本文は読まずに拒否する。
const maxNotificationsSeenBody = 4 * 1024

// NotificationsHandler は、通知の3つの API（GET /api/notifications、GET /api/notifications/unseen-count、POST /api/notifications/seen）を受け持つ。
type NotificationsHandler struct {
	list  *query.ListNotifications
	count *query.CountUnseenNotifications
	seen  *command.MarkNotificationsSeen
}

// NewNotificationsHandler は、list・count・seen で通知の3つの API に応える NotificationsHandler を返す。
func NewNotificationsHandler(list *query.ListNotifications, count *query.CountUnseenNotifications, seen *command.MarkNotificationsSeen) *NotificationsHandler {
	return &NotificationsHandler{list: list, count: count, seen: seen}
}

// List は、ログイン中の利用者の通知を、取引日の新しい順に100件まで返す（価格の変動の通知のプラン 4.5）。
// 失敗は 500 と {"error":"internal_error"} で、内部のエラーの内容は応答に出すと外に漏れるので、ログにだけ出す。
func (h *NotificationsHandler) List(w http.ResponseWriter, r *http.Request) {
	const route = "GET /api/notifications"
	userID, ok := currentUserID(w, r, route)
	if !ok {
		return
	}
	list, err := h.list.Execute(r.Context(), userID)
	if err != nil {
		log.Printf("api: %s: 通知を取得できません: %v", route, err)
		response.Error(w, http.StatusInternalServerError, response.CodeInternalError)
		return
	}
	body, err := newNotificationsResponse(list)
	if err != nil {
		// 円の値が 0.1円の単位でないなど、DB の値が決まりに合わないとき。黙って丸めず、誤った値を返さない。
		log.Printf("api: %s: 応答を作れません: %v", route, err)
		response.Error(w, http.StatusInternalServerError, response.CodeInternalError)
		return
	}
	response.JSON(w, http.StatusOK, body)
}

// unseenCountResponse は、GET /api/notifications/unseen-count の本文。
type unseenCountResponse struct {
	Count int64 `json:"count"`
}

// UnseenCount は、ログイン中の利用者の、見ていない通知の数を返す（プラン 4.5）。
func (h *NotificationsHandler) UnseenCount(w http.ResponseWriter, r *http.Request) {
	const route = "GET /api/notifications/unseen-count"
	userID, ok := currentUserID(w, r, route)
	if !ok {
		return
	}
	count, err := h.count.Execute(r.Context(), userID)
	if err != nil {
		log.Printf("api: %s: 見ていない数を取得できません: %v", route, err)
		response.Error(w, http.StatusInternalServerError, response.CodeInternalError)
		return
	}
	response.JSON(w, http.StatusOK, unseenCountResponse{Count: count})
}

// notificationsSeenRequest は、POST /api/notifications/seen の本文。upToId がないときは空の文字列になり、正の整数でないので 400 になる。
type notificationsSeenRequest struct {
	UpToID string `json:"upToId"`
}

// MarkSeen は、ログイン中の利用者の、ID が本文の upToId 以下で見ていない通知を、見たにして、204（本文なし）を返す（要件34）。
// 本文が JSON でない・知らない項目がある・大きすぎる、または upToId が正の整数の文字列でないときは、何も変えずに 400 と {"error":"bad_request"} を返す。
// それ以外の失敗は 500 と {"error":"internal_error"}（ログにだけ出す）。ログには本文を入れず、固定の文言にする。
func (h *NotificationsHandler) MarkSeen(w http.ResponseWriter, r *http.Request) {
	const route = "POST /api/notifications/seen"
	userID, ok := currentUserID(w, r, route)
	if !ok {
		return
	}
	upToID, ok := readNotificationsSeenUpToID(w, r)
	if !ok {
		response.Error(w, http.StatusBadRequest, response.CodeBadRequest)
		return
	}
	if err := h.seen.Execute(r.Context(), userID, upToID); err != nil {
		log.Printf("api: %s: 見たにできません: %v", route, err)
		response.Error(w, http.StatusInternalServerError, response.CodeInternalError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// readNotificationsSeenUpToID は、本文を読んで upToId（正の整数）を返す。本文が決まりに合わなければ ok を false にする。
func readNotificationsSeenUpToID(w http.ResponseWriter, r *http.Request) (upToID int64, ok bool) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxNotificationsSeenBody))
	dec.DisallowUnknownFields()
	var req notificationsSeenRequest
	if err := dec.Decode(&req); err != nil {
		return 0, false
	}
	// JSON の値の後ろに、空白以外の何かがあるとき（2つ目の値、本文の大きさの超過）は誤り。
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return 0, false
	}
	id, err := strconv.ParseInt(req.UpToID, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != req.UpToID {
		return 0, false
	}
	return id, true
}

// currentUserID は、context からログイン中の利用者を取り出す。ないときは 500 を書いて ok を false にする。
func currentUserID(w http.ResponseWriter, r *http.Request, route string) (userID uuid.UUID, ok bool) {
	userID, ok = middleware.UserIDFrom(r.Context())
	if !ok {
		// router が必ず CurrentUser を通すので、ここに来るのは組み立ての誤りのときだけ。
		log.Printf("api: %s: ログイン中の利用者が context にありません", route)
		response.Error(w, http.StatusInternalServerError, response.CodeInternalError)
	}
	return userID, ok
}
