package command_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
)

// 仕様: 要件（docs/requirements/stock-holdings.md）の AC-16（NFR-1「株価の取得元を差し替えられる。取得元ごとの実装は
// 共通のインターフェースを実装し…価格の保存処理…は変更しない」）と、
// プラン（docs/plans/stock-holdings/plan.md）5.1 の AC-16 の行
// （テスト用の取得元（9984 → 123450（0.01 単位。1,234.5円）・2026-10-07、ダミーにない値）とメモリ上のリポジトリを注入する。その値が保存される）。
// 確かめる名前は 4.4 の command.PriceSource・ErrPriceUnavailable・NewFetchClosingPrices・Execute・FetchResult と、
// domain の model.ClosingPrice・NewClosingPrice。取得処理の流れは 4.4「株価取得の決まり」。
//
// mock は境界の外側（取得元と、DB の代わりのメモリ上のリポジトリ）だけに使い、保存された値（結果）を確かめる。
// 9984 の 123450（1,234.5円。0.01 単位）・2026-10-07 は、ダミーの取得元（7203=300000・6758=350000・AAPL=18000、どれも 2026-10-06。
// 9984 は定義なし）が返さない値なので、保存された値はこのテスト用の取得元から来たものだと分かる。

// fakeSourceCode・fakeTradingDate・fakePriceHundredths は、プラン 5.1 の AC-16 の行のテスト用の取得元が返す値。
const (
	fakeSourceCode      = "9984"
	fakeTradingDate     = "2026-10-07"
	fakePriceHundredths = 123450 // 1,234.5円（0.01 単位。プラン 4.3）
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
// ListHeldStocks は、4.6（PR②）の決まり（誰かの保有数量が1以上の銘柄を通貨と一緒に。重複なし・コードの昇順）のとおりの stocks を返す。
// err が nil でなければ、stocks と err を両方返す（一覧が読めないとき。fetch_closing_prices_failure_test.go が使う）。
// エラーと一緒に銘柄コードも返すのは、Execute がエラーのときに返った銘柄コードを使わないことを確かめるため
// （エラーのときに nil を返すと、それを使って取得・保存へ進む実装でも何も保存されず、見分けられない。PR③ ステップ6 2ラウンド目の TC-04）。
// err が nil のときは、stocks と nil を返す（ほかのテストはこちら）。
type memoryHoldingRepository struct {
	stocks []repository.HeldStock
	err    error
}

func (r memoryHoldingRepository) ListHeldStocks(context.Context) ([]repository.HeldStock, error) {
	return slices.Clone(r.stocks), r.err
}

// heldYen は、円の銘柄 codes を、保有している銘柄（通貨は JPY）にする。
func heldYen(codes ...string) []repository.HeldStock {
	stocks := make([]repository.HeldStock, 0, len(codes))
	for _, code := range codes {
		stocks = append(stocks, repository.HeldStock{Code: code, Currency: model.CurrencyJPY})
	}
	return stocks
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
// → テスト用の取得元が返した価格（9984・2026-10-07・123450（0.01 単位。1,234.5円））が保存される。
// 取得処理（command.FetchClosingPrices）のコードは変更せず、NewFetchClosingPrices に渡す取得元を変えるだけで
// 取得元を差し替えられることを、このテストがダミー以外の取得元で動かすことで示す。
func TestAC16_共通インターフェースを実装したテスト用の取得元を注入するとその取得元が返した価格が保存される(t *testing.T) {
	t.Parallel()

	want, err := model.NewClosingPrice(fakeSourceCode, fakeTradingDate, fakePriceHundredths)
	if err != nil {
		t.Fatalf("model.NewClosingPrice(%q, %q, %d): %v（0より大きい終値はエラーにならない。4.4）",
			fakeSourceCode, fakeTradingDate, fakePriceHundredths, err)
	}
	// 変数の型を command.PriceSource にして、テスト用の取得元が共通インターフェースを実装していることを型で確かめる。
	var src command.PriceSource = testPriceSource{prices: map[string]model.ClosingPrice{fakeSourceCode: want}}
	prices := &memoryClosingPriceRepository{}
	fetch := command.NewFetchClosingPrices(src, memoryHoldingRepository{stocks: heldYen(fakeSourceCode)}, prices)

	got, err := fetch.Execute(t.Context())

	// 4.4「株価取得の決まり」3: Execute がエラーを返すのは ListHeldStocks が失敗したときだけ。
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
		t.Fatalf("保存された終値: got %d件 %+v, want 1件（9984・2026-10-07・123450）", len(saved), saved)
	}
	p := saved[0]
	if p.StockCode != fakeSourceCode {
		t.Errorf("保存された終値の銘柄コード: got %q, want %q", p.StockCode, fakeSourceCode)
	}
	if p.TradingDate != fakeTradingDate {
		t.Errorf("保存された終値の取引日: got %q, want %q（テスト用の取得元が返した取引日）", p.TradingDate, fakeTradingDate)
	}
	if p.PriceHundredths != fakePriceHundredths {
		t.Errorf("保存された終値（0.01 単位）: got %d, want %d（テスト用の取得元が返した 1,234.5円）", p.PriceHundredths, fakePriceHundredths)
	}
}

// recordingAlertEvaluator は、command.PriceAlertEvaluator を実装したテスト用の判定（境界の外側の fake）。
// Execute に渡された終値を、呼ばれた回ごとに覚え、呼ばれた時点で保存済みだった終値（repo。nil なら覚えない）も覚える。
// err が nil でなければ、それを返す。
type recordingAlertEvaluator struct {
	mu        sync.Mutex
	repo      *memoryClosingPriceRepository
	err       error
	calls     [][]model.ClosingPrice
	savedThen [][]model.ClosingPrice
}

func (e *recordingAlertEvaluator) Execute(_ context.Context, prices []model.ClosingPrice) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, slices.Clone(prices))
	if e.repo != nil {
		e.savedThen = append(e.savedThen, e.repo.all())
	}
	return e.err
}

// 補助 4.4: 判定をつなぐと、保存できた銘柄の終値だけを、保存の後に1回で渡す。
// 7203 と 1301 は保存でき、6758 は取得元が「取得できない」、9984 は保存が失敗する → 渡されるのは 7203 と 1301 の終値（stocks の順）。
// 判定が呼ばれた時点で、その2つはもう保存されている。FetchResult は判定をつながないときと同じ。
func Test補助_4_4_判定をつなぐと保存できた銘柄の終値だけを保存の後に1回で渡す(t *testing.T) {
	t.Parallel()
	p7203 := mustAlertPrice(t, "7203", "2026-10-06", 300000)
	p1301 := mustAlertPrice(t, "1301", "2026-10-06", 150000)
	src := testPriceSource{prices: map[string]model.ClosingPrice{
		"7203": p7203,
		"1301": p1301,
		"9984": mustAlertPrice(t, "9984", "2026-10-06", 100000),
	}}
	prices := &memoryClosingPriceRepository{saveErrors: map[string]error{"9984": errors.New("fake: disk full")}}
	evaluator := &recordingAlertEvaluator{repo: prices}
	fetch := command.NewFetchClosingPrices(src, memoryHoldingRepository{}, prices).WithAlertEvaluator(evaluator)

	got := fetch.ExecuteStocks(t.Context(), heldYen("7203", "6758", "9984", "1301"))

	if want := []string{"7203", "1301"}; !slices.Equal(got.Saved, want) {
		t.Errorf("FetchResult.Saved: got %v, want %v", got.Saved, want)
	}
	if want := []string{"6758", "9984"}; !slices.Equal(got.Failed, want) {
		t.Errorf("FetchResult.Failed: got %v, want %v", got.Failed, want)
	}
	if len(evaluator.calls) != 1 {
		t.Fatalf("判定の呼び出し: got %d回 %+v, want 1回（保存できた終値をまとめて）", len(evaluator.calls), evaluator.calls)
	}
	if want := []model.ClosingPrice{p7203, p1301}; !slices.Equal(evaluator.calls[0], want) {
		t.Errorf("判定に渡した終値:\n got %+v\nwant %+v（取得できなかった銘柄・保存できなかった銘柄は渡さない）", evaluator.calls[0], want)
	}
	for _, want := range []model.ClosingPrice{p7203, p1301} {
		if !slices.Contains(evaluator.savedThen[0], want) {
			t.Errorf("判定が呼ばれた時点の保存済みの終値 %+v に、%+v がない（保存の後に判定を呼ぶ）", evaluator.savedThen[0], want)
		}
	}
}

// 補助 4.4: Execute（定時の取得）でも、保存できた終値を1回で判定に渡す。
func Test補助_4_4_定時の取得でも保存した終値を判定に渡す(t *testing.T) {
	t.Parallel()
	p7203 := mustAlertPrice(t, "7203", "2026-10-06", 300000)
	src := testPriceSource{prices: map[string]model.ClosingPrice{"7203": p7203}}
	evaluator := &recordingAlertEvaluator{}
	fetch := command.NewFetchClosingPrices(src, memoryHoldingRepository{stocks: heldYen("7203")}, &memoryClosingPriceRepository{}).
		WithAlertEvaluator(evaluator)

	if _, err := fetch.Execute(t.Context()); err != nil {
		t.Fatalf("Execute: got err %v, want nil", err)
	}

	if len(evaluator.calls) != 1 || !slices.Equal(evaluator.calls[0], []model.ClosingPrice{p7203}) {
		t.Errorf("判定に渡した終値: got %+v, want [%+v] の1回", evaluator.calls, p7203)
	}
}

// 補助 4.4・要件21: 判定が失敗しても、FetchResult は変えない（Saved に残り、Failed に入れない）。保存した終値も残る。
func Test補助_要件21_判定が失敗しても取得の結果と保存した終値は変わらない(t *testing.T) {
	t.Parallel()
	p7203 := mustAlertPrice(t, "7203", "2026-10-06", 300000)
	src := testPriceSource{prices: map[string]model.ClosingPrice{"7203": p7203}}
	prices := &memoryClosingPriceRepository{}
	evaluator := &recordingAlertEvaluator{err: errors.New("fake: 判定に失敗")}
	fetch := command.NewFetchClosingPrices(src, memoryHoldingRepository{}, prices).WithAlertEvaluator(evaluator)

	got := fetch.ExecuteStocks(t.Context(), heldYen("7203"))

	if !slices.Equal(got.Saved, []string{"7203"}) || len(got.Failed) != 0 {
		t.Errorf("FetchResult: got Saved=%v Failed=%v, want Saved=[7203] Failed=空（判定の失敗は結果に出さない）", got.Saved, got.Failed)
	}
	if saved := prices.all(); !slices.Equal(saved, []model.ClosingPrice{p7203}) {
		t.Errorf("保存された終値: got %+v, want [%+v]", saved, p7203)
	}
	if len(evaluator.calls) != 1 {
		t.Errorf("判定の呼び出し: got %d回, want 1回", len(evaluator.calls))
	}
}

// 補助 4.4: 判定をつながない（nil を渡す）ときは、今と同じ（呼ばず、panic もしない）。
func Test補助_4_4_判定がnilのときは判定を呼ばず今と同じに動く(t *testing.T) {
	t.Parallel()
	p7203 := mustAlertPrice(t, "7203", "2026-10-06", 300000)
	src := testPriceSource{prices: map[string]model.ClosingPrice{"7203": p7203}}
	prices := &memoryClosingPriceRepository{}
	fetch := command.NewFetchClosingPrices(src, memoryHoldingRepository{}, prices).WithAlertEvaluator(nil)

	got := fetch.ExecuteStocks(t.Context(), heldYen("7203"))

	if !slices.Equal(got.Saved, []string{"7203"}) || len(got.Failed) != 0 {
		t.Errorf("FetchResult: got Saved=%v Failed=%v, want Saved=[7203] Failed=空", got.Saved, got.Failed)
	}
	if saved := prices.all(); !slices.Equal(saved, []model.ClosingPrice{p7203}) {
		t.Errorf("保存された終値: got %+v, want [%+v]", saved, p7203)
	}
}

// mustAlertPrice は、model.NewClosingPrice で終値を作る。作れなければテストを止める。
func mustAlertPrice(t *testing.T, code, tradingDate string, priceHundredths int64) model.ClosingPrice {
	t.Helper()
	p, err := model.NewClosingPrice(code, tradingDate, priceHundredths)
	if err != nil {
		t.Fatalf("model.NewClosingPrice(%q, %q, %d): %v", code, tradingDate, priceHundredths, err)
	}
	return p
}
