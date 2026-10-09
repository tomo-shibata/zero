// Package router は、chi で HTTP のルーティングを組み立てる。
package router

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Deps は、ルーティングが使う依存。cmd/api が組み立てて渡す。
// PR① ではまだ項目がない（PR② で GET /api/holdings 用の QueryHandler と DevUserID を足す）。
type Deps struct{}

// New は、アプリの HTTP ハンドラーを返す。
// PR① では Deps の項目がないので受け取るだけにしている。
func New(_ Deps) http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", healthz)
	return r
}

// healthz は、プロセスが HTTP に応答できることだけを返す（DB などの依存は確かめない）。
// 本文は改行なしの "ok" ちょうど（プラン 4.4）。
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "ok")
}
