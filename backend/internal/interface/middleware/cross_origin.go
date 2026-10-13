package middleware

import (
	"net/http"

	"github.com/tomo-shibata/zero/backend/internal/interface/response"
)

// CrossOriginProtection は、別のサイトのページから送られた書き込みの要求（GET・HEAD・OPTIONS 以外）を拒み、
// 403 と {"error":"forbidden"} を返すミドルウェアを返す（CSRF 対策。要件 非機能1、値段の更新のプラン 6章の判断4）。
//
// Go の標準の http.CrossOriginProtection を使う。Sec-Fetch-Site が same-origin・none ならば通し、それ以外の値なら拒む。
// Sec-Fetch-Site がなければ Origin のホストと Host を比べて違えば拒み、どちらのヘッダーもなければ通す（ブラウザ以外の要求。CSRF はブラウザからしか起きない）。
// 読み取り（GET）は対象外。信頼するオリジンも、バイパスの指定も足さない（足すと、別のサイトからの書き込みを通す穴になるため）。
// 拒否の応答は、エラーの本文の形を揃えるため、response.Error で書く。
func CrossOriginProtection() func(http.Handler) http.Handler {
	protection := http.NewCrossOriginProtection()
	protection.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		response.Error(w, http.StatusForbidden, response.CodeForbidden)
	}))
	return func(next http.Handler) http.Handler {
		return protection.Handler(next)
	}
}
