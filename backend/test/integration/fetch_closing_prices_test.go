package integration

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/write"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/pricesource"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 要件（docs/requirements/stock-holdings.md）の AC-13・AC-13a（FR-15）、AC-14（FR-16）と、
// プラン（docs/plans/stock-holdings/plan.md）5.1 の AC-13・AC-13a・AC-14 の行（PR③）。
// 組み立ては 4.4 の境界のとおり: command.NewFetchClosingPrices(取得元, write.NewHoldingRepository(pool),
// write.NewClosingPriceRepository(pool)) → Execute。取得元は AC-13・AC-13a がダミー（pricesource.NewDummyPriceSource）、
// AC-14 がテスト用の取得元（境界の外側の fake）。取得処理の流れは 4.4「株価取得の決まり」。
//
// 保有データと保存済みの価格は SQL の INSERT で入れ（holdings_fixtures_test.go）、結果は stock_prices を SQL で読んで確かめる
// （stock_prices_fixtures_test.go。終値は 0.1円単位の整数で比べる）。
// 期待値は要件の AC とプランの値をそのまま使い、テストの中で計算しない。

// newFetchClosingPrices は、取得元 src と pool の DB を使う価格の取得処理を、4.4 の境界のとおりに組み立てる。
func newFetchClosingPrices(src command.PriceSource, pool *pgxpool.Pool) *command.FetchClosingPrices {
	return command.NewFetchClosingPrices(src, write.NewHoldingRepository(pool), write.NewClosingPriceRepository(pool))
}

// AC-13（FR-15）: 7203 と 6758 が保有されていて、価格は未保存 → ダミーの取得元で価格の取得処理を実行する
// → 7203 は3,000円・2026-10-06、6758 は3,500円・2026-10-06 で保存される。
// プラン 5.1: A が 7203 を2件（数量違い）、B が 6758 を保有。err が nil・len(Failed) == 0・Saved が ["6758","7203"]。
// stock_prices が 7203（30000・2026-10-06）と 6758（35000・2026-10-06）の各1行
// （同じ銘柄を2件保有していても1行。他の利用者（B）の保有銘柄も取得する。FR-15「保有されている全銘柄」）。
func TestAC13_保有されている全銘柄の終値をダミーの取得元から取得して取引日とともに保存する(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertAC13Holdings(t, pool)

	got, err := newFetchClosingPrices(pricesource.NewDummyPriceSource(), pool).Execute(t.Context())

	if err != nil {
		t.Fatalf("Execute: got err %v, want nil", err)
	}
	if len(got.Failed) != 0 {
		t.Errorf("FetchResult.Failed: got %v, want 空（7203・6758 はどちらもダミーに定義がある）", got.Failed)
	}
	if want := []string{"6758", "7203"}; !slices.Equal(got.Saved, want) {
		t.Errorf("FetchResult.Saved: got %v, want %v（重複なし・昇順）", got.Saved, want)
	}
	assertStockPrices(t, pool, ac13WantPrices())
}

// AC-13a（FR-15）: 7203 が保有されていて、7203 の価格が2,900円・2026-10-06 で保存済み → ダミーの取得元で価格の取得処理を実行する
// → エラーにならず、7203 の 2026-10-06 の価格は1件のまま3,000円になる（同じ銘柄・同じ取引日は上書きし、成功として扱う）。
// プラン 5.1: A が 7203 を保有。err が nil・len(Failed) == 0・Saved が ["7203"]。7203 の 2026-10-06 は1行で 30000。
func TestAC13a_同じ銘柄と取引日の終値が保存済みなら取得した値で上書きして成功とする(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertHolding(t, pool, seedUserA, "7203", 100, "2500")
	insertClosePrice(t, pool, "7203", "2026-10-06", "2900")

	got, err := newFetchClosingPrices(pricesource.NewDummyPriceSource(), pool).Execute(t.Context())

	if err != nil {
		t.Fatalf("Execute: got err %v, want nil（保存済みの行は上書きし、エラーにしない）", err)
	}
	if len(got.Failed) != 0 {
		t.Errorf("FetchResult.Failed: got %v, want 空（上書きは成功として扱う）", got.Failed)
	}
	if want := []string{"7203"}; !slices.Equal(got.Saved, want) {
		t.Errorf("FetchResult.Saved: got %v, want %v", got.Saved, want)
	}
	assertStockPrices(t, pool, []storedClosePrice{
		{stockCode: "7203", tradingDate: "2026-10-06", priceTenths: 30000},
	})
}

// stubPriceSource は、command.PriceSource を実装したテスト用の取得元（境界の外側の fake）。
// prices にある銘柄はその終値を返し、unavailable にある銘柄は command.ErrPriceUnavailable を返す。
// どちらにもない銘柄を要求されたら、テストの前提の誤りなので、テストを失敗させてから ErrPriceUnavailable を返す。
// 読むだけなので、Execute が銘柄ごとに並行して呼んでも構わない（失敗の知らせは Errorf で、別の goroutine からでもよい）。
type stubPriceSource struct {
	t           *testing.T
	prices      map[string]model.ClosingPrice
	unavailable []string
}

func (s stubPriceSource) FetchClosingPrice(_ context.Context, stockCode string) (model.ClosingPrice, error) {
	if p, ok := s.prices[stockCode]; ok {
		return p, nil
	}
	if !slices.Contains(s.unavailable, stockCode) {
		s.t.Errorf("テスト用の取得元に、保有していない銘柄 %q が要求された（ListHeldStockCodes は保有データにある銘柄だけを返す。4.4）", stockCode)
	}
	return model.ClosingPrice{}, command.ErrPriceUnavailable
}

// mustClosingPrice は、4.4 の model.NewClosingPrice で終値を作る。作れなければテストを止める（0より大きい終値は作れる）。
func mustClosingPrice(t *testing.T, code, tradingDate string, priceTenths int64) model.ClosingPrice {
	t.Helper()
	p, err := model.NewClosingPrice(code, tradingDate, priceTenths)
	if err != nil {
		t.Fatalf("model.NewClosingPrice(%q, %q, %d): %v", code, tradingDate, priceTenths, err)
	}
	return p
}

// AC-14（FR-16）: 7203 の価格が2,900円・2026-10-05 で保存済みで、取得元が 7203 について「取得できない」を返し、
// 6758 については3,500円・2026-10-06 を返す → 価格の取得処理を実行する
// → 7203 は2,900円・2026-10-05 のまま残り、6758 は保存される。
// プラン 5.1: 保有データを 7203 → 6758 → 9984 の順に INSERT。テスト用の取得元は 7203 だけ ErrPriceUnavailable、
// 6758 は 35000・2026-10-06、9984 は 10000・2026-10-06。7203 は元の1行のまま、6758 と 9984 が保存され、Failed は ["7203"]。
// 9984 は、昇順で 7203 の後に処理される銘柄で、1銘柄（7203）の失敗で他の銘柄の取得を止めない（FR-16）ことを確かめるために足したもの。
// あわせて 4.4「株価取得の決まり」3（Execute がエラーを返すのは ListHeldStockCodes が失敗したときだけ）と、
// FetchResult の Saved・Failed（どちらも昇順）も確かめる。
func TestAC14_取得できなかった銘柄は保存済みの価格を残し他の銘柄の取得は止めない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertClosePrice(t, pool, "7203", "2026-10-05", "2900")
	// 保有データの INSERT の順はプラン 5.1 のとおり（7203 → 6758 → 9984）。
	// 数量・取得価格はプランに指定がないので、取得の結果に関係しない値にした。
	insertHolding(t, pool, seedUserA, "7203", 100, "2500")
	insertHolding(t, pool, seedUserA, "6758", 10, "3200")
	insertHolding(t, pool, seedUserA, "9984", 10, "9000")
	src := stubPriceSource{
		t: t,
		prices: map[string]model.ClosingPrice{
			"6758": mustClosingPrice(t, "6758", "2026-10-06", 35000),
			"9984": mustClosingPrice(t, "9984", "2026-10-06", 10000),
		},
		unavailable: []string{"7203"},
	}

	got, err := newFetchClosingPrices(src, pool).Execute(t.Context())

	if err != nil {
		t.Fatalf("Execute: got err %v, want nil（取得元が「取得できない」を返した銘柄は Failed に入れ、Execute はエラーにしない）", err)
	}
	if want := []string{"7203"}; !slices.Equal(got.Failed, want) {
		t.Errorf("FetchResult.Failed: got %v, want %v", got.Failed, want)
	}
	if want := []string{"6758", "9984"}; !slices.Equal(got.Saved, want) {
		t.Errorf("FetchResult.Saved: got %v, want %v（7203 の失敗の後も 9984 を取得する）", got.Saved, want)
	}
	assertStockPrices(t, pool, []storedClosePrice{
		{stockCode: "6758", tradingDate: "2026-10-06", priceTenths: 35000},
		{stockCode: "7203", tradingDate: "2026-10-05", priceTenths: 29000}, // 保存済みの2,900円・2026-10-05 のまま
		{stockCode: "9984", tradingDate: "2026-10-06", priceTenths: 10000},
	})
}
