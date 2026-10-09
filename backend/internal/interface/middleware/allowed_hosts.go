package middleware

import (
	"net/http"
	"strings"

	"github.com/tomo-shibata/zero/backend/internal/interface/response"
)

// AllowedHosts は、Host ヘッダーが hosts のどれかのリクエストだけを通し、それ以外には 403 と
// {"error":"forbidden"} を返すミドルウェアを返す（プラン 6章の判断12）。
//
// API には認証がないので、悪意のあるサイトが DNS リバインディング（自分のドメインを 127.0.0.1 に向け直す）で
// ブラウザから API を呼ぶと、同一オリジンとして応答を読めてしまう。そのときの Host ヘッダーは攻撃者のドメインになるので、
// このサーバー自身を指す Host（"127.0.0.1:<PORT>" など。ポートまで含めて比べる）だけを許して防ぐ。
// hosts が空なら、すべてのリクエストを拒む（設定を忘れたときに開いたままにしないため）。
// ホスト名は大文字と小文字を区別しない（RFC 3986）ので、小文字にそろえて比べる。
func AllowedHosts(hosts []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(hosts))
	for _, h := range hosts {
		allowed[strings.ToLower(h)] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := allowed[strings.ToLower(r.Host)]; !ok {
				response.Error(w, http.StatusForbidden, response.CodeForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
