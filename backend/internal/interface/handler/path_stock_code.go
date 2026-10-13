package handler

import (
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
)

// pathStockCode は、パスの {code} から銘柄コードを取り出す。画面は encodeURIComponent して送る（値段の更新のプラン 4.4「API」）。
// r.URL.RawPath が空でないときだけ url.PathUnescape で戻す。Go は既定のエンコードと同じなら RawPath を空にし、
// そのとき chi は戻し済みの Path で照合するので、いつも戻すと "100%" は壊れ、"%41" は "A" に二重に戻ってしまうため。
// 戻せないとき、または戻した値が正しい UTF-8 でない・NUL を含むときは ok が false になる（呼び出し側は 404 にする）。
// 銘柄マスタにそのような銘柄は入らないので、DB には問い合わせない。NUL は PostgreSQL の TEXT に入らず、問い合わせが 500 になるため。
func pathStockCode(r *http.Request) (code string, ok bool) {
	code = chi.URLParam(r, "code")
	if r.URL.RawPath != "" {
		var err error
		code, err = url.PathUnescape(code)
		if err != nil {
			return "", false
		}
	}
	if !utf8.ValidString(code) || strings.ContainsRune(code, 0) {
		return "", false
	}
	return code, true
}
