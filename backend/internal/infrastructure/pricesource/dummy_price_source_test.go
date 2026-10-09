package pricesource_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/pricesource"
)

// 仕様: 要件（docs/requirements/stock-holdings.md）の AC-17（NFR-2「当面の取得元はダミーとする。ダミーは外部と通信せず、
// 銘柄コードごとに定義した固定の終値と取引日を返し、定義のない銘柄は『取得できない』を返す」）と、
// プラン（docs/plans/stock-holdings/plan.md）5.1 の AC-17 の行
// （http.DefaultTransport を「呼ばれたらテストを失敗させる」ものに差し替え、取り消し済みの context で 7203・9984 を要求する。
// 7203 は 30000・2026-10-06、9984 は ErrPriceUnavailable。このテストは t.Parallel() にしない）。
// 確かめる名前は 4.4 の pricesource.NewDummyPriceSource（7203=30000・6758=35000（Tenths、どちらも 2026-10-06）。
// それ以外は ErrPriceUnavailable。通信しない。ctx の取り消しを見ない（取り消し済みでも定義どおりの値を返す））と、
// command.PriceSource・ErrPriceUnavailable。
//
// 取り消し済みの context を渡すのは、ダミーが通信しようとしても（context を使う通信なら）すぐに失敗するようにして、
// 通信に頼らずに定義どおりの値を返すことを確かめるため（4.4: ctx の取り消しを見ない）。

// failingTransport は、呼ばれたらテストを失敗させる http.RoundTripper。
// ダミーが http.DefaultTransport を使う通信（http.Get・http.DefaultClient など）をしたら、ここで分かる。
type failingTransport struct {
	t *testing.T
}

func (f failingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// 別の goroutine から呼ばれることもあるので、Fatal ではなく Error にする。
	f.t.Errorf("外部への通信が発生した: %s %s（ダミーの取得元は外部と通信しない。NFR-2）", req.Method, req.URL)
	return nil, errors.New("このテストでは外部への通信を禁じている")
}

// replaceDefaultTransport は、テストの間だけ http.DefaultTransport を failingTransport に差し替える。
// パッケージの変数を書き換えるので、これを使うテストは t.Parallel() にしない（プラン 5.1 の AC-17 の行）。
func replaceDefaultTransport(t *testing.T) {
	t.Helper()
	original := http.DefaultTransport
	http.DefaultTransport = failingTransport{t: t}
	t.Cleanup(func() { http.DefaultTransport = original })
}

// AC-17（NFR-2）: ダミーの取得元に 7203 と 9984 の価格を要求する
// → 7203 は3,000円（0.1円単位で 30000）・2026-10-06 を返し、9984 は「取得できない」（command.ErrPriceUnavailable）を返す。
// 外部への通信は発生しない。
//
// t.Parallel() にしない（サブテストも）: http.DefaultTransport（パッケージの変数）を差し替えるため（プラン 5.1 の AC-17 の行）。
func TestAC17_ダミーの取得元は通信せずに7203は定義どおりの終値を返し9984は取得できないを返す(t *testing.T) {
	replaceDefaultTransport(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // 取り消し済みの context で要求する

	// 変数の型を command.PriceSource にして、ダミーが共通インターフェース（NFR-1）を実装していることを型で確かめる。
	var src command.PriceSource = pricesource.NewDummyPriceSource()

	t.Run("7203は3000円_2026-10-06を返す", func(t *testing.T) {
		got, err := src.FetchClosingPrice(ctx, "7203")
		if err != nil {
			t.Fatalf("FetchClosingPrice(7203): got err %v, want nil（ダミーに定義のある銘柄。取り消し済みの context でも定義どおりの値を返す）", err)
		}
		if got.StockCode != "7203" {
			t.Errorf("銘柄コード: got %q, want %q", got.StockCode, "7203")
		}
		if got.TradingDate != "2026-10-06" {
			t.Errorf("取引日: got %q, want %q", got.TradingDate, "2026-10-06")
		}
		if got.PriceTenths != 30000 {
			t.Errorf("終値（0.1円単位）: got %d, want 30000（3,000円）", got.PriceTenths)
		}
	})

	t.Run("9984は取得できないを返す", func(t *testing.T) {
		got, err := src.FetchClosingPrice(ctx, "9984")
		if !errors.Is(err, command.ErrPriceUnavailable) {
			t.Errorf("FetchClosingPrice(9984): got (%+v, %v), want command.ErrPriceUnavailable（ダミーに定義のない銘柄）", got, err)
		}
	})
}
