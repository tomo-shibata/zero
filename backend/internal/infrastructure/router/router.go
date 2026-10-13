// Package router は、chi で HTTP のルーティングを組み立てる。
package router

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/interface/handler"
	"github.com/tomo-shibata/zero/backend/internal/interface/middleware"
)

// Deps は、ルーティングが使う依存。cmd/api が組み立てて渡す。
// 空の Deps でも New は panic しない（/healthz だけを使うテストが空の Deps で組み立てるため）。
type Deps struct {
	// QueryHandler は、GET /api/holdings を受け持つ。nil なら /api/holdings を登録しない。
	QueryHandler *handler.QueryHandler
	// PriceRefreshHandler は、POST /api/price-refresh（価格の今すぐ更新）を受け持つ。nil なら /api/price-refresh を登録しない。
	PriceRefreshHandler *handler.PriceRefreshHandler
	// DevUserID は、ログイン中の利用者として扱う開発用利用者（認証を作るまでの代わり。プラン 6章の判断5）。
	DevUserID uuid.UUID
	// AllowedHosts は、/api/ 以下で受け付ける Host ヘッダー（"127.0.0.1:<PORT>" など。プラン 6章の判断12）。
	// 空なら、/api/ 以下のルートはどの Host でも 403 になる（設定を忘れても開いたままにしない）。
	// /api/ 以下にルートが1つもない（QueryHandler と PriceRefreshHandler が nil）ときは、chi がミドルウェアを通さずに 404 を返す。
	AllowedHosts []string
}

// New は、アプリの HTTP ハンドラーを返す。
func New(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", healthz)

	// Host の許可リストは、データを返す /api/ 以下だけにかける。/healthz はデータを返さず、
	// 死活監視などが Host を気にせずに呼べるよう、対象にしない（プラン 4.4）。
	r.Route("/api", func(r chi.Router) {
		// 応答のヘッダーは、Host の許可リストより前（外側）で付ける。許可リストが返す 403 にも付けるため（プラン 4.4「API」）。
		r.Use(middleware.SecurityHeaders)
		r.Use(middleware.AllowedHosts(d.AllowedHosts))
		// 別のサイトからの書き込みの拒否は、Host の許可リストの内側、利用者を決める前に置く（値段の更新のプラン 4.4）。
		// 拒否の 403 にも、外側の SecurityHeaders のヘッダーが付く。
		r.Use(middleware.CrossOriginProtection())
		r.Use(middleware.CurrentUser(d.DevUserID))
		if d.QueryHandler != nil {
			r.Get("/holdings", d.QueryHandler.ListHoldings)
		}
		if d.PriceRefreshHandler != nil {
			r.Method(http.MethodPost, "/price-refresh", d.PriceRefreshHandler)
		}
	})
	return r
}

// healthz は、プロセスが HTTP に応答できることだけを返す（DB などの依存は確かめない）。
// 本文は改行なしの "ok" ちょうど（プラン 4.4）。
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "ok")
}
