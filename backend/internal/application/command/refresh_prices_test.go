package command_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
)

// 仕様: 要件（docs/requirements/price-update.md）の要件16（ログイン中の利用者が保有する全銘柄の終値を取得して保存する。
// 取得と保存の決まりは定時の取得と同じ）、非機能2（定時実行と同じ取得処理を使う）と、
// プラン（docs/plans/price-update/plan.md）5.2 の PR① の次の2行。
//   - 「RefreshPrices: 保有銘柄を読めなければエラー（errors.Is で元のエラーとつながる）で、取得元を呼ばない」
//   - 「既存の fetch_closing_prices*_test.go を変えずに通す。ExecuteCodes に ["9984","7203"] を渡すと、その順に取得し、Saved・Failed もその順」
//     （既存のテストを変えずに通すことは、このファイルでは書かず、このパッケージの全テストの実行で確かめる）
//
// 確かめる名前は 4.4 の repository.HeldStockRepository（PR② で ListHeldStocksOf に変わる。通貨と一緒に返す）、command.NewRefreshPrices・RefreshPrices.Execute、
// FetchClosingPrices.ExecuteStocks（PR② で ExecuteCodes から変わる。4.6）、FetchResult。
// mock は境界の外側（取得元の fake と、DB の代わりのメモリ上のリポジトリ）だけに使い、戻り値と保存された終値（結果）を確かめる。
// 取得元を呼ばないことは、プランの基準そのもの（5.2）なので、呼ばれた銘柄の記録で確かめる。

// memoryHeldStockRepository は、repository.HeldStockRepository を実装したメモリ上のリポジトリ（DB の代わり）。
// 利用者ごとの保有銘柄 stocksByUser から、4.4 の決まり（重複なし・昇順）のとおりの銘柄（通貨付き）を返す。
// err が nil でなければ、銘柄と err を両方返す（エラーのときに返った銘柄コードを使って取得へ進む実装を見分けるため。
// fetch_closing_prices_test.go の memoryHoldingRepository と同じ考え方）。
type memoryHeldStockRepository struct {
	stocksByUser map[uuid.UUID][]repository.HeldStock
	err          error
}

func (r memoryHeldStockRepository) ListHeldStocksOf(_ context.Context, userID uuid.UUID) ([]repository.HeldStock, error) {
	return slices.Clone(r.stocksByUser[userID]), r.err
}

// recordingPriceSource は、command.PriceSource を実装したテスト用の取得元（境界の外側の fake）。
// prices にある銘柄はその終値を返し、ない銘柄は command.ErrPriceUnavailable を返す。通信はしない。
// 呼ばれた銘柄コードを、呼ばれた順に記録する（requested）。Execute が並行して呼んでも壊れないよう、mutex で守る。
type recordingPriceSource struct {
	mu     sync.Mutex
	prices map[string]model.ClosingPrice
	called []string
}

func (s *recordingPriceSource) FetchClosingPrice(_ context.Context, stockCode string) (model.ClosingPrice, error) {
	s.mu.Lock()
	s.called = append(s.called, stockCode)
	s.mu.Unlock()
	p, ok := s.prices[stockCode]
	if !ok {
		return model.ClosingPrice{}, command.ErrPriceUnavailable
	}
	return p, nil
}

// requested は、取得元が呼ばれた銘柄コードを、呼ばれた順に返す。
func (s *recordingPriceSource) requested() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.called)
}

var (
	refreshUserA = uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	refreshUserB = uuid.MustParse("00000000-0000-0000-0000-00000000000b")
)

// 補助 要件16・4.4: 保有銘柄を読めなければ、Execute はエラーを返し（errors.Is で元のエラーとつながる）、
// 取得元を呼ばず、何も保存しない。保有銘柄の読み取りは、エラーと一緒に銘柄コード（7203）も返す
// （エラーを無視して、返った銘柄コードで取得へ進む実装なら、取得元が呼ばれ、価格が保存されて落ちる）。
func TestRefreshPrices_要件16_保有銘柄を読めなければエラーを返し取得元を呼ばず何も保存しない(t *testing.T) {
	t.Parallel()
	readErr := errors.New("fake: 保有銘柄を読めない")
	src := &recordingPriceSource{prices: map[string]model.ClosingPrice{
		"7203": closingPrice(t, "7203", "2026-10-06", 300000),
	}}
	prices := &memoryClosingPriceRepository{}
	held := memoryHeldStockRepository{
		stocksByUser: map[uuid.UUID][]repository.HeldStock{refreshUserA: heldYen("7203")},
		err:          readErr,
	}
	// 取得処理（定時実行と同じ FetchClosingPrices）は、全利用者の保有銘柄の IF とは別に、今すぐ更新の保有銘柄の IF だけを使う。
	fetch := command.NewFetchClosingPrices(src, memoryHoldingRepository{stocks: heldYen("7203")}, prices)
	refresh := command.NewRefreshPrices(fetch, held)

	_, err := refresh.Execute(t.Context(), refreshUserA)

	if err == nil {
		t.Fatalf("Execute: got nil, want エラー（保有銘柄を読めないとき）")
	}
	if !errors.Is(err, readErr) {
		t.Errorf("Execute のエラー: got %v, want errors.Is で元のエラー（%v）とつながるもの", err, readErr)
	}
	if got := src.requested(); len(got) != 0 {
		t.Errorf("取得元に要求された銘柄: got %v, want 呼ばれない（保有銘柄が分からないので取得しない）", got)
	}
	assertNothingSaved(t, prices)
}

// 要件16: 今すぐ更新は、渡された利用者の保有銘柄だけを取得して保存する（他の利用者だけが保有する銘柄は取得しない）。
// 利用者A は 7203、利用者B は 6758 を保有し、取得元は両方の価格を持つ。A として実行すると、7203 だけが取得・保存される
// （結果は Saved が ["7203"]、Failed が空。取得元が呼ばれたのも 7203 だけ）。
func TestRefreshPrices_要件16_渡した利用者の保有銘柄だけを取得して保存する(t *testing.T) {
	t.Parallel()
	toyota := closingPrice(t, "7203", "2026-10-06", 300000)
	sony := closingPrice(t, "6758", "2026-10-06", 350000)
	src := &recordingPriceSource{prices: map[string]model.ClosingPrice{"7203": toyota, "6758": sony}}
	prices := &memoryClosingPriceRepository{}
	held := memoryHeldStockRepository{stocksByUser: map[uuid.UUID][]repository.HeldStock{
		refreshUserA: heldYen("7203"),
		refreshUserB: heldYen("6758"),
	}}
	// 全利用者の保有銘柄（6758・7203）を返す IF を渡しても、今すぐ更新はそれを使わない。
	fetch := command.NewFetchClosingPrices(src, memoryHoldingRepository{stocks: heldYen("6758", "7203")}, prices)
	refresh := command.NewRefreshPrices(fetch, held)

	got, err := refresh.Execute(t.Context(), refreshUserA)

	if err != nil {
		t.Fatalf("Execute: got err %v, want nil", err)
	}
	if want := []string{"7203"}; !slices.Equal(got.Saved, want) {
		t.Errorf("FetchResult.Saved: got %v, want %v", got.Saved, want)
	}
	if len(got.Failed) != 0 {
		t.Errorf("FetchResult.Failed: got %v, want 空", got.Failed)
	}
	if want := []string{"7203"}; !slices.Equal(src.requested(), want) {
		t.Errorf("取得元に要求された銘柄: got %v, want %v（Bだけが保有する 6758 は取得しない）", src.requested(), want)
	}
	assertSavedPrices(t, prices, []model.ClosingPrice{toyota})
}

// 補助 非機能2・判断8: ExecuteStocks に ["9984","7203"]（どちらも円）を渡すと、その順（9984 → 7203）に取得し、
// Saved も その順（["9984","7203"]。昇順に並べ直さない）になる。両方とも取得元に価格がある場合。
func TestExecuteStocks_非機能2_判断8_渡した銘柄の順に取得しSavedもその順になる(t *testing.T) {
	t.Parallel()
	softbank := closingPrice(t, "9984", "2026-10-07", 123450)
	toyota := closingPrice(t, "7203", "2026-10-06", 300000)
	src := &recordingPriceSource{prices: map[string]model.ClosingPrice{"9984": softbank, "7203": toyota}}
	prices := &memoryClosingPriceRepository{}
	fetch := command.NewFetchClosingPrices(src, memoryHoldingRepository{}, prices)

	got := fetch.ExecuteStocks(t.Context(), heldYen("9984", "7203"))

	if want := []string{"9984", "7203"}; !slices.Equal(src.requested(), want) {
		t.Errorf("取得元に要求された銘柄の順: got %v, want %v（渡された順）", src.requested(), want)
	}
	if want := []string{"9984", "7203"}; !slices.Equal(got.Saved, want) {
		t.Errorf("FetchResult.Saved: got %v, want %v（渡された順）", got.Saved, want)
	}
	if len(got.Failed) != 0 {
		t.Errorf("FetchResult.Failed: got %v, want 空", got.Failed)
	}
	// 保存された終値の比べ方は、銘柄コード・取引日の昇順にそろえる（assertSavedPrices）。順の確かめは上の2つで行う。
	assertSavedPrices(t, prices, []model.ClosingPrice{toyota, softbank})
}

// 補助 非機能2・判断8: ExecuteStocks に ["9984","7203"]（どちらも円）を渡し、どちらも取得元に価格がないとき、
// Failed も渡された順（["9984","7203"]）になり、Saved は空で、何も保存されない。
func TestExecuteStocks_非機能2_判断8_取得できない銘柄のFailedも渡した銘柄の順になる(t *testing.T) {
	t.Parallel()
	src := &recordingPriceSource{prices: map[string]model.ClosingPrice{}}
	prices := &memoryClosingPriceRepository{}
	fetch := command.NewFetchClosingPrices(src, memoryHoldingRepository{}, prices)

	got := fetch.ExecuteStocks(t.Context(), heldYen("9984", "7203"))

	if want := []string{"9984", "7203"}; !slices.Equal(got.Failed, want) {
		t.Errorf("FetchResult.Failed: got %v, want %v（渡された順）", got.Failed, want)
	}
	if len(got.Saved) != 0 {
		t.Errorf("FetchResult.Saved: got %v, want 空", got.Saved)
	}
	assertNothingSaved(t, prices)
}

// 要件5・基準15（PR②）: 今すぐ更新も、保有銘柄の通貨（ListHeldStocksOf が返す）で終値の精度を確かめる。
// 利用者Aが 7203（円）と AAPL（ドル）を保有。取得元は 7203 で 3,000.25円（300025。円では細かすぎる）、AAPL で 180.25ドル（18025）を返す
// → Saved が ["AAPL"]、Failed が ["7203"]。保存されているのは AAPL の 18025 だけ。
func TestRefreshPrices_要件5_基準15_保有銘柄の通貨の精度に合わない終値はFailedになりドルの0_01の端数は保存する(t *testing.T) {
	t.Parallel()
	toyota := closingPrice(t, "7203", "2026-10-06", 300025)
	aapl := closingPrice(t, "AAPL", "2026-10-06", 18025)
	src := &recordingPriceSource{prices: map[string]model.ClosingPrice{"7203": toyota, "AAPL": aapl}}
	prices := &memoryClosingPriceRepository{}
	held := memoryHeldStockRepository{stocksByUser: map[uuid.UUID][]repository.HeldStock{
		refreshUserA: {
			{Code: "7203", Currency: model.CurrencyJPY},
			{Code: "AAPL", Currency: model.CurrencyUSD},
		},
	}}
	fetch := command.NewFetchClosingPrices(src, memoryHoldingRepository{}, prices)
	refresh := command.NewRefreshPrices(fetch, held)

	got, err := refresh.Execute(t.Context(), refreshUserA)

	if err != nil {
		t.Fatalf("Execute: got err %v, want nil", err)
	}
	if want := []string{"AAPL"}; !slices.Equal(got.Saved, want) {
		t.Errorf("FetchResult.Saved: got %v, want %v", got.Saved, want)
	}
	if want := []string{"7203"}; !slices.Equal(got.Failed, want) {
		t.Errorf("FetchResult.Failed: got %v, want %v（7203 は円で 0.1円より細かい終値）", got.Failed, want)
	}
	assertSavedPrices(t, prices, []model.ClosingPrice{aapl})
}
