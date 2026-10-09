package middleware

import "net/http"

// SecurityHeaders は、応答に Cache-Control: no-store と X-Content-Type-Options: nosniff を付けるミドルウェア
// （プラン 4.4「API」、PR② ステップ6 の S-2）。
//
// /api/ の応答は利用者の資産データなので、ブラウザや途中のキャッシュに残さないよう no-store にする。
// nosniff は、JSON の応答を、ブラウザが中身から HTML やスクリプトだと推測して扱わないようにするため。
// 後ろのミドルウェア（AllowedHosts の 403）やハンドラー（500）が返す応答にも付くよう、next を呼ぶ前に付ける。
// Add ではなく Set にするのは、後ろで同じ名前のヘッダーを付けても、2つに重ならないようにするため。
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Cache-Control", "no-store")
		h.Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}
