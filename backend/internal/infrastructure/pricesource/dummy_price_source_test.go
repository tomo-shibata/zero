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
// 7203 は 3,000円・2026-10-06、9984 は ErrPriceUnavailable。このテストは t.Parallel() にしない）。
// 売買履歴のプラン（docs/plans/trade-history/plan.md）4.6・5.3（PR②）で、値は 0.01 単位にし、AAPL を足す: 7203=300000・6758=350000・AAPL=18000
// （どれも 2026-10-06）。MSFT・9984 は取得できない（ダミーの取得元の単体）。
// 確かめる名前は 4.4 の pricesource.NewDummyPriceSource（上の値を返し、それ以外は ErrPriceUnavailable。
// 通信しない。ctx の取り消しを見ない（取り消し済みでも定義どおりの値を返す））と、
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
// → 7203 は3,000円（0.01 単位で 300000）・2026-10-06 を返し、9984 は「取得できない」（command.ErrPriceUnavailable）を返す。
// PR②（売買履歴のプラン 4.6・5.3）: AAPL は 180.00ドル（18000）・2026-10-06 を返し、MSFT は「取得できない」を返す。
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
		if got.PriceHundredths != 300000 {
			t.Errorf("終値（0.01 単位）: got %d, want 300000（3,000円）", got.PriceHundredths)
		}
	})

	t.Run("6758は3500円_2026-10-06を返す", func(t *testing.T) {
		got, err := src.FetchClosingPrice(ctx, "6758")
		if err != nil {
			t.Fatalf("FetchClosingPrice(6758): got err %v, want nil", err)
		}
		if got.StockCode != "6758" || got.TradingDate != "2026-10-06" || got.PriceHundredths != 350000 {
			t.Errorf("FetchClosingPrice(6758): got %+v, want 6758・2026-10-06・350000（3,500円）", got)
		}
	})

	t.Run("AAPLは180ドル_2026-10-06を返す", func(t *testing.T) {
		got, err := src.FetchClosingPrice(ctx, "AAPL")
		if err != nil {
			t.Fatalf("FetchClosingPrice(AAPL): got err %v, want nil（ダミーに定義のある銘柄）", err)
		}
		if got.StockCode != "AAPL" || got.TradingDate != "2026-10-06" || got.PriceHundredths != 18000 {
			t.Errorf("FetchClosingPrice(AAPL): got %+v, want AAPL・2026-10-06・18000（180.00ドル）", got)
		}
	})

	t.Run("MSFTは取得できないを返す", func(t *testing.T) {
		got, err := src.FetchClosingPrice(ctx, "MSFT")
		if !errors.Is(err, command.ErrPriceUnavailable) {
			t.Errorf("FetchClosingPrice(MSFT): got (%+v, %v), want command.ErrPriceUnavailable（ダミーに定義のない銘柄）", got, err)
		}
	})

	t.Run("9984は取得できないを返す", func(t *testing.T) {
		got, err := src.FetchClosingPrice(ctx, "9984")
		if !errors.Is(err, command.ErrPriceUnavailable) {
			t.Errorf("FetchClosingPrice(9984): got (%+v, %v), want command.ErrPriceUnavailable（ダミーに定義のない銘柄）", got, err)
		}
	})
}
