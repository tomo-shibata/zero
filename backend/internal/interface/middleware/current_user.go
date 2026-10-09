// Package middleware は、HTTP のリクエストを、ハンドラーに渡す前に処理するミドルウェアを持つ。
package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// currentUserKey は、ログイン中の利用者の ID を context に入れるときのキー。
// 他のパッケージのキーとぶつからないよう、外に出さない型にする。
type currentUserKey struct{}

// CurrentUser は、userID をログイン中の利用者として context に入れるミドルウェアを返す。
// 認証を作るまでは、固定の開発用利用者（DEV_USER_ID）を入れる。認証を作るときは、このミドルウェアを差し替える
// （プラン 6章の判断5）。ハンドラーは UserIDFrom で取り出すので、差し替えてもハンドラーは変えなくて済む。
func CurrentUser(userID uuid.UUID) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), currentUserKey{}, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFrom は、CurrentUser が context に入れたログイン中の利用者の ID を返す。入っていなければ false を返す。
func UserIDFrom(ctx context.Context) (uuid.UUID, bool) {
	userID, ok := ctx.Value(currentUserKey{}).(uuid.UUID)
	return userID, ok
}
