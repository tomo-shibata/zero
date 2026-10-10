// Package response は、API の応答を JSON で書く。
// エラーの本文の形（{"error":"<コード>"}）を、ハンドラーとミドルウェアで揃えるため（プラン 4.4「API」）。
package response

import (
	"encoding/json"
	"log"
	"net/http"
)

// エラーの本文の error に入れるコード（プラン 4.4「API」）。
const (
	CodeInternalError = "internal_error" // 500。内部のエラーの内容は返さず、ログに出す
	CodeForbidden     = "forbidden"      // 403
)

// errorBody は、エラーの応答の本文。
type errorBody struct {
	Error string `json:"error"`
}

// JSON は、status と、v を JSON にした本文を書く。
// 先に JSON にしてから書くのは、変換に失敗したときに、200 を送ってから途中で切れた本文を返さないため。
func JSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		log.Printf("api: 応答を JSON にできません: %v", err)
		status = http.StatusInternalServerError
		body = []byte(`{"error":"` + CodeInternalError + `"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// 書き込みの失敗は、クライアントが切断したときなどで、もう応答を返す先がないので見ない。
	_, _ = w.Write(body)
}

// Error は、status と、本文 {"error":"<code>"} を書く。
func Error(w http.ResponseWriter, status int, code string) {
	JSON(w, status, errorBody{Error: code})
}
