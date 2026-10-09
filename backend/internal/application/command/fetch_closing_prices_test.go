package command_test

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// 仕様: 要件（docs/requirements/stock-holdings.md）の AC-16（NFR-1「株価の取得元を差し替えられる。取得元ごとの実装は
// 共通のインターフェースを実装し…価格の保存処理…は変更しない」）と、
// プラン（docs/plans/stock-holdings/plan.md）5.1 の AC-16 の行
// （テスト用の取得元（9984 → 12345・2026-10-07、ダミーにない値）とメモリ上のリポジトリを注入する。その値が保存される）。
// 確かめる名前は 4.4 の command.PriceSource・ErrPriceUnavailable・NewFetchClosingPrices・Execute・FetchResult と、
// domain の model.ClosingPrice・NewClosingPrice。取得処理の流れは 4.4「株価取得の決まり」。
//
// mock は境界の外側（取得元と、DB の代わりのメモリ上のリポジトリ）だけに使い、保存された値（結果）を確かめる。
// 9984 の 12345（1,234.5円）・2026-10-07 は、ダミーの取得元（7203=30000・6758=35000、どちらも 2026-10-06。
// 9984 は定義なし）が返さない値なので、保存された値はこのテスト用の取得元から来たものだと分かる。

// fakeSourceCode・fakeTradingDate・fakePriceTenths は、プラン 5.1 の AC-16 の行のテスト用の取得元が返す値。
const (
	fakeSourceCode  = "9984"
	fakeTradingDate = "2026-10-07"
	fakePriceTenths = 12345 // 1,234.5円（0.1円単位。プラン 4.1）
)

// testPriceSource は、command.PriceSource を実装したテスト用の取得元（境界の外側の fake）。
// prices にある銘柄はその終値を返し、ない銘柄は command.ErrPriceUnavailable（4.4）を返す。通信はしない。
// 読むだけなので、Execute が銘柄ごとに並行して呼んでも構わない。
type testPriceSource struct {
	prices map[string]model.ClosingPrice
}

func (s testPriceSource) FetchClosingPrice(_ context.Context, stockCode string) (model.ClosingPrice, error) {
	p, ok := s.prices[stockCode]
	if !ok {
		return model.ClosingPrice{}, command.ErrPriceUnavailable
	}
	return p, nil
}

// memoryHoldingRepository は、repository.HoldingRepository を実装したメモリ上のリポジトリ（DB の代わり）。
// ListHeldStockCodes は、4.4 の決まり（全利用者の保有データにある銘柄コード。重複なし・昇順）のとおりの codes を返す。
// err が nil でなければ、codes と err を両方返す（一覧が読めないとき。fetch_closing_prices_failure_test.go が使う）。
// エラーと一緒に銘柄コードも返すのは、Execute がエラーのときに返った銘柄コードを使わないことを確かめるため
// （エラーのときに nil を返すと、それを使って取得・保存へ進む実装でも何も保存されず、見分けられない。PR③ ステップ6 2ラウンド目の TC-04）。
// err が nil のときは、codes と nil を返す（ほかのテストはこちら）。
type memoryHoldingRepository struct {
	codes []string
	err   error
}

func (r memoryHoldingRepository) ListHeldStockCodes(context.Context) ([]string, error) {
	return slices.Clone(r.codes), r.err
}

// memoryClosingPriceRepository は、repository.ClosingPriceRepository を実装したメモリ上のリポジトリ（DB の代わり）。
// 保存された終値を、銘柄・取引日ごとに1件だけ持つ（同じ銘柄・同じ取引日は上書き。4.4 の Save、FR-15）。
// saveErrors にある銘柄の Save は、何も保存せずにそのエラーを返す（保存の失敗。fetch_closing_prices_failure_test.go が使う）。
// Execute が並行して Save を呼んでも壊れないよう、mutex で守る。
type memoryClosingPriceRepository struct {
	mu         sync.Mutex
	saved      []model.ClosingPrice
	saveErrors map[string]error // 銘柄コード → Save が返すエラー。読むだけ
}

func (r *memoryClosingPriceRepository) Save(_ context.Context, p model.ClosingPrice) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err, ok := r.saveErrors[p.StockCode]; ok {
		return err
	}
	for i, s := range r.saved {
		if s.StockCode == p.StockCode && s.TradingDate == p.TradingDate {
			r.saved[i] = p
			return nil
		}
	}
	r.saved = append(r.saved, p)
	return nil
}

// all は、保存された終値をすべて返す。
func (r *memoryClosingPriceRepository) all() []model.ClosingPrice {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.saved)
}

// AC-16（NFR-1）: 共通インターフェース（command.PriceSource）を実装したテスト用の取得元を注入して、価格の取得処理を実行する
// → テスト用の取得元が返した価格（9984・2026-10-07・12345）が保存される。
// 取得処理（command.FetchClosingPrices）のコードは変更せず、NewFetchClosingPrices に渡す取得元を変えるだけで
// 取得元を差し替えられることを、このテストがダミー以外の取得元で動かすことで示す。
func TestAC16_共通インターフェースを実装したテスト用の取得元を注入するとその取得元が返した価格が保存される(t *testing.T) {
	t.Parallel()

	want, err := model.NewClosingPrice(fakeSourceCode, fakeTradingDate, fakePriceTenths)
	if err != nil {
		t.Fatalf("model.NewClosingPrice(%q, %q, %d): %v（0より大きい終値はエラーにならない。4.4）",
			fakeSourceCode, fakeTradingDate, fakePriceTenths, err)
	}
	// 変数の型を command.PriceSource にして、テスト用の取得元が共通インターフェースを実装していることを型で確かめる。
	var src command.PriceSource = testPriceSource{prices: map[string]model.ClosingPrice{fakeSourceCode: want}}
	prices := &memoryClosingPriceRepository{}
	fetch := command.NewFetchClosingPrices(src, memoryHoldingRepository{codes: []string{fakeSourceCode}}, prices)

	got, err := fetch.Execute(t.Context())

	// 4.4「株価取得の決まり」3: Execute がエラーを返すのは ListHeldStockCodes が失敗したときだけ。
	if err != nil {
		t.Fatalf("Execute: got err %v, want nil", err)
	}
	if !slices.Equal(got.Saved, []string{fakeSourceCode}) {
		t.Errorf("FetchResult.Saved: got %v, want [%s]", got.Saved, fakeSourceCode)
	}
	if len(got.Failed) != 0 {
		t.Errorf("FetchResult.Failed: got %v, want 空（テスト用の取得元は 9984 の価格を返す）", got.Failed)
	}

	saved := prices.all()
	if len(saved) != 1 {
		t.Fatalf("保存された終値: got %d件 %+v, want 1件（9984・2026-10-07・12345）", len(saved), saved)
	}
	p := saved[0]
	if p.StockCode != fakeSourceCode {
		t.Errorf("保存された終値の銘柄コード: got %q, want %q", p.StockCode, fakeSourceCode)
	}
	if p.TradingDate != fakeTradingDate {
		t.Errorf("保存された終値の取引日: got %q, want %q（テスト用の取得元が返した取引日）", p.TradingDate, fakeTradingDate)
	}
	if p.PriceTenths != fakePriceTenths {
		t.Errorf("保存された終値（0.1円単位）: got %d, want %d（テスト用の取得元が返した 1,234.5円）", p.PriceTenths, fakePriceTenths)
	}
}
