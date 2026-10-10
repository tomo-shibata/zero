package command_test

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// 仕様: 要件（docs/requirements/stock-holdings.md）の FR-16（取得元から価格を取得できなかった銘柄は、保存済みの価格をそのまま残す。
// 1銘柄の失敗で他の銘柄の取得を止めない）と、プラン（docs/plans/stock-holdings/plan.md）4.4「株価取得の決まり」2〜4、
// 5.1「補助のテスト（バックエンド）」の次の2行（PR③。背景は docs/plans/stock-holdings/reviews/pr3-step6.md）。
//   - FR-16・4.4 決まり2・3（PR③ の TC-01、TC-02、EC-P1-1）: 下の (a)〜(d)
//   - 4.4 決まり2・4（PR③ の EC-P2-1、EC-P2-3）: 下の (e)(f)(g)（(g) は PR③ ステップ6 2ラウンド目の EC-P2-9）
//
// 確かめる名前は 4.4 の command.PriceSource・ErrPriceUnavailable・NewFetchClosingPrices・Execute・FetchResult と、
// domain の model.ClosingPrice・NewClosingPrice。
// mock は境界の外側（取得元の fake と、DB の代わりのメモリ上のリポジトリ。リポジトリは fetch_closing_prices_test.go）だけに使い、
// Execute の戻り値（err・Saved・Failed）と、メモリ上のリポジトリに残った終値（結果）を確かめる。
// 期待値はプラン 5.1 の値をそのまま使い、テストの中で計算しない。

// sourceReply は、テスト用の取得元が1銘柄について返す応答。
type sourceReply struct {
	price  model.ClosingPrice
	err    error
	panics bool // true なら、値を返さずに panic する（4.4 の PriceSource の約束「panic しない」を破る取得元）
	// beforeReply は、nil でなければ、応答を返す（または panic する）前に呼ぶ。
	// 取得の途中で ctx を取り消す（取得元の応答を待っている間に Ctrl+C などが来た）ことを再現するのに使う（(g)）。
	beforeReply func()
}

// scriptedPriceSource は、command.PriceSource を実装したテスト用の取得元（境界の外側の fake）。
// 銘柄ごとに replies で決めた応答（終値、エラー、panic）を返す。ctx は見ない（取り消し済みでも決めた応答を返す。
// ダミーの取得元と同じ。4.4）。通信はしない。
// replies にない銘柄を要求されたら、テストの前提の誤りなので、テストを失敗させてから ErrPriceUnavailable を返す。
// 読むだけなので、Execute が銘柄ごとに並行して呼んでも構わない（失敗の知らせは Errorf で、別の goroutine からでもよい）。
type scriptedPriceSource struct {
	t       *testing.T
	replies map[string]sourceReply
}

func (s scriptedPriceSource) FetchClosingPrice(_ context.Context, stockCode string) (model.ClosingPrice, error) {
	r, ok := s.replies[stockCode]
	if !ok {
		s.t.Errorf("テスト用の取得元に、保有していない銘柄 %q が要求された（取得するのは ListHeldStockCodes の銘柄だけ。4.4 決まり1）", stockCode)
		return model.ClosingPrice{}, command.ErrPriceUnavailable
	}
	if r.beforeReply != nil {
		r.beforeReply()
	}
	if r.panics {
		panic("テスト用の取得元の panic（銘柄 " + stockCode + "）")
	}
	return r.price, r.err
}

// closingPrice は、4.4 の model.NewClosingPrice で終値を作る。作れなければテストを止める（0より大きい終値と実在する日付は作れる）。
func closingPrice(t *testing.T, code, tradingDate string, priceTenths int64) model.ClosingPrice {
	t.Helper()
	p, err := model.NewClosingPrice(code, tradingDate, priceTenths)
	if err != nil {
		t.Fatalf("model.NewClosingPrice(%q, %q, %d): %v", code, tradingDate, priceTenths, err)
	}
	return p
}

// execute は、fetch.Execute(ctx) を呼んで戻り値を返す。
// Execute が panic を外に出したら（取得元の panic を、その銘柄の失敗として扱わなかった）、テストを失敗させて止める
// （テストのプロセスごと落ちて、ほかのテストの結果が分からなくなるのを避けるため）。
// ただし Execute が別の goroutine で取得元を呼んでいて、そこで panic が外に出た場合は、ここでは受け止められない
// （そのときはテストのプロセスが落ちる。それも失敗として分かる）。
func execute(ctx context.Context, t *testing.T, fetch *command.FetchClosingPrices) (command.FetchResult, error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Execute が panic を外に出した: %v（取得元の panic は、その銘柄の失敗として Failed に入れて次の銘柄へ進む。4.4 決まり2）", r)
		}
	}()
	return fetch.Execute(ctx)
}

// savedPrices は、メモリ上のリポジトリに保存されている終値を、銘柄コード・取引日の昇順で返す
// （メモリ上のリポジトリは保存した順に持つので、Execute が銘柄を処理する順に左右されずに比べるため）。
func savedPrices(r *memoryClosingPriceRepository) []model.ClosingPrice {
	saved := r.all()
	slices.SortFunc(saved, func(a, b model.ClosingPrice) int {
		return cmp.Or(cmp.Compare(a.StockCode, b.StockCode), cmp.Compare(a.TradingDate, b.TradingDate))
	})
	return saved
}

// assertSavedPrices は、メモリ上のリポジトリに保存されている終値が want（銘柄コード・取引日の昇順）とちょうど同じであることを確かめる。
func assertSavedPrices(t *testing.T, r *memoryClosingPriceRepository, want []model.ClosingPrice) {
	t.Helper()
	if got := savedPrices(r); !slices.Equal(got, want) {
		t.Errorf("保存されている終値（銘柄コード・取引日・0.1円単位の終値）:\n got %+v\nwant %+v", got, want)
	}
}

// assertNothingSaved は、メモリ上のリポジトリに終値が1件も保存されていないことを確かめる。
func assertNothingSaved(t *testing.T, r *memoryClosingPriceRepository) {
	t.Helper()
	if got := savedPrices(r); len(got) != 0 {
		t.Errorf("保存されている終値: got %+v, want なし", got)
	}
}

// (a) FR-16・4.4 決まり2・3（PR③ の TC-01、EC-P1-1）: ErrPriceUnavailable 以外の取得元のエラーと、保存の失敗も、
// その銘柄を Failed に入れて次の銘柄へ進む。Execute はエラーを返さず、取得できなかった銘柄の保存済みの価格は残る。
// プラン 5.1: 保有 [6758, 7203, 9984]。取得元は 6758 で errors.New("通信エラー")（ErrPriceUnavailable を包まない）、
// 7203 は 30000・2026-10-06、9984 は 12345・2026-10-07。価格のリポジトリは 6758 の 29000・2026-10-05 を保存済みにし、
// 7203 の Save だけ失敗させる → err が nil、Failed が [6758, 7203]、Saved が [9984]、
// 保存されている価格は 6758（2026-10-05・29000）と 9984（2026-10-07・12345）だけ。
// 失敗する2銘柄（6758・7203）は昇順で 9984 より前にあるので、最初の失敗で止める実装では 9984 が保存されない。
func TestFR16_PR3_TC01_EC_P1_1_a_ErrPriceUnavailable以外の取得元のエラーや保存の失敗はその銘柄だけFailedにして次の銘柄へ進む(t *testing.T) {
	t.Parallel()
	src := scriptedPriceSource{t: t, replies: map[string]sourceReply{
		"6758": {err: errors.New("通信エラー")},
		"7203": {price: closingPrice(t, "7203", "2026-10-06", 30000)},
		"9984": {price: closingPrice(t, "9984", "2026-10-07", 12345)},
	}}
	prices := &memoryClosingPriceRepository{
		saved:      []model.ClosingPrice{closingPrice(t, "6758", "2026-10-05", 29000)},
		saveErrors: map[string]error{"7203": errors.New("保存の失敗（テスト用）")},
	}
	fetch := command.NewFetchClosingPrices(src, memoryHoldingRepository{codes: []string{"6758", "7203", "9984"}}, prices)

	got, err := execute(t.Context(), t, fetch)

	if err != nil {
		t.Fatalf("Execute: got err %v, want nil（Execute がエラーを返すのは ListHeldStockCodes が失敗したときだけ。4.4 決まり3）", err)
	}
	if want := []string{"6758", "7203"}; !slices.Equal(got.Failed, want) {
		t.Errorf("FetchResult.Failed: got %v, want %v（6758 は取得元のエラー、7203 は保存の失敗）", got.Failed, want)
	}
	if want := []string{"9984"}; !slices.Equal(got.Saved, want) {
		t.Errorf("FetchResult.Saved: got %v, want %v（6758・7203 の失敗の後も 9984 を取得して保存する）", got.Saved, want)
	}
	assertSavedPrices(t, prices, []model.ClosingPrice{
		{StockCode: "6758", TradingDate: "2026-10-05", PriceTenths: 29000}, // 保存済みの値のまま残る（FR-16）
		{StockCode: "9984", TradingDate: "2026-10-07", PriceTenths: 12345},
	})
}

// (b) 4.4 決まり3（PR③ の TC-02、ステップ6 2ラウンド目の TC-04）: ListHeldStockCodes が失敗したら、Execute はエラーを返し
// （errors.Is で元のエラーとつながる）、何も保存しない。
// プラン 5.1: ListHeldStockCodes が銘柄コード [7203] とエラーを両方返す → Execute がエラー（errors.Is で元のエラーとつながる）、
// 何も保存されない（エラーのときに返った銘柄コードを使わないこと）。
// 取得元は 7203 に正しい終値を返すようにしておくので、エラーを見ずに返った [7203] で取得・保存へ進む実装なら 7203 が保存される。
// 7203 の値はプランに指定がないので、ダミーの取得元と同じ 30000・2026-10-06 にした。
func TestPlan4_4_PR3_TC02_TC04_b_保有銘柄の一覧が読めなければExecuteはそのエラーを返し返った銘柄コードも使わず何も保存しない(t *testing.T) {
	t.Parallel()
	listErr := errors.New("保有銘柄の一覧が読めない（テスト用）")
	src := scriptedPriceSource{t: t, replies: map[string]sourceReply{
		"7203": {price: closingPrice(t, "7203", "2026-10-06", 30000)},
	}}
	prices := &memoryClosingPriceRepository{}
	fetch := command.NewFetchClosingPrices(src, memoryHoldingRepository{codes: []string{"7203"}, err: listErr}, prices)

	_, err := execute(t.Context(), t, fetch)

	switch {
	case err == nil:
		t.Error("Execute: err が nil（want ListHeldStockCodes のエラー。4.4 決まり3）")
	case !errors.Is(err, listErr):
		t.Errorf("Execute: got err %v, want ListHeldStockCodes のエラー %q とつながるエラー（errors.Is）", err, listErr)
	}
	assertNothingSaved(t, prices)
}

// (c) FR-16・4.4 決まり2（PR③ の TC-01、EC-P1-1）: 7203 を要求したのに取得元が StockCode 6758 の終値を返したら、
// 7203 を Failed に入れ、何も保存しない（返った 6758 の値を 7203 として保存しない。6758 の値としても保存しない）。
// 返る 6758 の終値の値はプランに指定がないので、ダミーの取得元の 6758 と同じ 35000・2026-10-06 にした（それ自体は正しい終値）。
func TestFR16_PR3_TC01_EC_P1_1_c_取得元が要求と別の銘柄の終値を返したらその銘柄はFailedにして保存しない(t *testing.T) {
	t.Parallel()
	src := scriptedPriceSource{t: t, replies: map[string]sourceReply{
		"7203": {price: closingPrice(t, "6758", "2026-10-06", 35000)},
	}}
	prices := &memoryClosingPriceRepository{}
	fetch := command.NewFetchClosingPrices(src, memoryHoldingRepository{codes: []string{"7203"}}, prices)

	got, err := execute(t.Context(), t, fetch)

	if err != nil {
		t.Fatalf("Execute: got err %v, want nil（4.4 決まり3）", err)
	}
	if want := []string{"7203"}; !slices.Equal(got.Failed, want) {
		t.Errorf("FetchResult.Failed: got %v, want %v（取得元が別の銘柄の終値を返した）", got.Failed, want)
	}
	if len(got.Saved) != 0 {
		t.Errorf("FetchResult.Saved: got %v, want 空", got.Saved)
	}
	assertNothingSaved(t, prices)
}

// (d) FR-16・4.4 決まり1・2（PR③ の TC-01、EC-P1-1）: 取得元が model.NewClosingPrice を通さずに作った不正な終値
// （PriceTenths 0、または TradingDate "2026-02-30"）を返したら、Execute が NewClosingPrice で確かめ直して弾き、
// その銘柄を Failed に入れて保存しない。
// 不正にしない項目（銘柄コード 7203、取引日 2026-10-06、終値 30000）はプランに指定がないので、ダミーの取得元の 7203 と同じ値にした。
func TestFR16_PR3_TC01_EC_P1_1_d_取得元が終値0や実在しない取引日を返したらその銘柄はFailedにして保存しない(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		price model.ClosingPrice // NewClosingPrice を通さずに作る
	}{
		{
			name:  "PriceTenths0",
			price: model.ClosingPrice{StockCode: "7203", TradingDate: "2026-10-06", PriceTenths: 0},
		},
		{
			name:  "TradingDate2026-02-30",
			price: model.ClosingPrice{StockCode: "7203", TradingDate: "2026-02-30", PriceTenths: 30000},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := scriptedPriceSource{t: t, replies: map[string]sourceReply{"7203": {price: tc.price}}}
			prices := &memoryClosingPriceRepository{}
			fetch := command.NewFetchClosingPrices(src, memoryHoldingRepository{codes: []string{"7203"}}, prices)

			got, err := execute(t.Context(), t, fetch)

			if err != nil {
				t.Fatalf("Execute: got err %v, want nil（4.4 決まり3）", err)
			}
			if want := []string{"7203"}; !slices.Equal(got.Failed, want) {
				t.Errorf("FetchResult.Failed: got %v, want %v（取得元が不正な終値 %+v を返した）", got.Failed, want, tc.price)
			}
			if len(got.Saved) != 0 {
				t.Errorf("FetchResult.Saved: got %v, want 空", got.Saved)
			}
			assertNothingSaved(t, prices)
		})
	}
}

// (e) FR-16・4.4 決まり2（PR③ の EC-P2-3）: 取得元が panic した銘柄は、その銘柄の取得元のエラーとして Failed に入れ、
// 次の銘柄へ進む（panic で残りの銘柄の取得を止めない。Execute も panic しない）。
// プラン 5.1: 取得元が 6758 で panic し、7203 は 30000・2026-10-06 → Failed が [6758]、Saved が [7203]。
// 6758 は昇順で 7203 より前にあるので、panic で止まる実装では 7203 が保存されない。
func TestFR16_PR3_EC_P2_3_e_取得元がpanicした銘柄はFailedにして次の銘柄へ進む(t *testing.T) {
	t.Parallel()
	src := scriptedPriceSource{t: t, replies: map[string]sourceReply{
		"6758": {panics: true},
		"7203": {price: closingPrice(t, "7203", "2026-10-06", 30000)},
	}}
	prices := &memoryClosingPriceRepository{}
	fetch := command.NewFetchClosingPrices(src, memoryHoldingRepository{codes: []string{"6758", "7203"}}, prices)

	got, err := execute(t.Context(), t, fetch)

	if err != nil {
		t.Fatalf("Execute: got err %v, want nil（4.4 決まり3）", err)
	}
	if want := []string{"6758"}; !slices.Equal(got.Failed, want) {
		t.Errorf("FetchResult.Failed: got %v, want %v（6758 は取得元が panic した）", got.Failed, want)
	}
	if want := []string{"7203"}; !slices.Equal(got.Saved, want) {
		t.Errorf("FetchResult.Saved: got %v, want %v（6758 の panic の後も 7203 を取得して保存する）", got.Saved, want)
	}
	assertSavedPrices(t, prices, []model.ClosingPrice{
		{StockCode: "7203", TradingDate: "2026-10-06", PriceTenths: 30000},
	})
}

// (f) 4.4 決まり4（PR③ の EC-P2-1）: ctx が取り消されたら、残りの銘柄は取得せずに Failed に入れる。Execute はエラーを返さない。
// プラン 5.1: 取り消し済みの ctx で Execute → err が nil、Failed が保有銘柄すべて、Saved が空、何も保存されない。
// 取得元とメモリ上のリポジトリは ctx を見ない（取り消し済みでも終値を返し、保存もできる）ので、
// Execute 自身が ctx の取り消しを見ないと、終値が保存されてしまう。
// 保有銘柄と終値はプランに指定がないので、ダミーの取得元と同じ 6758（35000・2026-10-06）と 7203（30000・2026-10-06）にした。
func TestPlan4_4_PR3_EC_P2_1_f_取り消し済みのctxなら取得せずに保有銘柄すべてをFailedにしてエラーは返さない(t *testing.T) {
	t.Parallel()
	src := scriptedPriceSource{t: t, replies: map[string]sourceReply{
		"6758": {price: closingPrice(t, "6758", "2026-10-06", 35000)},
		"7203": {price: closingPrice(t, "7203", "2026-10-06", 30000)},
	}}
	prices := &memoryClosingPriceRepository{}
	fetch := command.NewFetchClosingPrices(src, memoryHoldingRepository{codes: []string{"6758", "7203"}}, prices)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	got, err := execute(ctx, t, fetch)

	if err != nil {
		t.Fatalf("Execute: got err %v, want nil（ctx の取り消しでは Execute はエラーを返さない。4.4 決まり4）", err)
	}
	if want := []string{"6758", "7203"}; !slices.Equal(got.Failed, want) {
		t.Errorf("FetchResult.Failed: got %v, want %v（保有銘柄すべて）", got.Failed, want)
	}
	if len(got.Saved) != 0 {
		t.Errorf("FetchResult.Saved: got %v, want 空", got.Saved)
	}
	assertNothingSaved(t, prices)
}

// (g) 4.4 決まり4（PR③ ステップ6 2ラウンド目の EC-P2-9）: 取得の途中で ctx が取り消されたら、そこまでに取得した銘柄は保存し、
// 残りの銘柄は取得せずに Failed に入れる。Execute はエラーを返さない（途中の取り消しも銘柄ごとに見ること）。
// プラン 5.1: 保有 [6758, 7203] で、取得元の fake が 6758 の応答を返す前に ctx を取り消す → err が nil、Saved が [6758]、Failed が [7203]。
// fake は 6758 の取得の中で cancel を呼んでから、6758 の終値を返す。Execute が ctx を見るのが最初の1回（ループの前）だけだと、
// その時点ではまだ取り消されていないので、取得元とメモリ上のリポジトリが ctx を見ない以上 7203 も取得・保存されてしまう。
// 保有銘柄の終値はプランに指定がないので、(f) と同じくダミーの取得元と同じ 6758（35000・2026-10-06）と 7203（30000・2026-10-06）にした。
// 保存されている終値が 6758 の値だけであることも確かめる（Saved に入れた銘柄が実際に保存され、7203 は保存されないこと）。
func TestPlan4_4_PR3_EC_P2_9_g_取得の途中でctxが取り消されたら取得済みの銘柄は保存し残りの銘柄はFailedにしてエラーは返さない(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	src := scriptedPriceSource{t: t, replies: map[string]sourceReply{
		"6758": {price: closingPrice(t, "6758", "2026-10-06", 35000), beforeReply: cancel},
		"7203": {price: closingPrice(t, "7203", "2026-10-06", 30000)},
	}}
	prices := &memoryClosingPriceRepository{}
	fetch := command.NewFetchClosingPrices(src, memoryHoldingRepository{codes: []string{"6758", "7203"}}, prices)

	got, err := execute(ctx, t, fetch)

	if err != nil {
		t.Fatalf("Execute: got err %v, want nil（ctx の取り消しでは Execute はエラーを返さない。4.4 決まり4）", err)
	}
	if want := []string{"6758"}; !slices.Equal(got.Saved, want) {
		t.Errorf("FetchResult.Saved: got %v, want %v（6758 は取り消しの前に取得を始めた銘柄）", got.Saved, want)
	}
	if want := []string{"7203"}; !slices.Equal(got.Failed, want) {
		t.Errorf("FetchResult.Failed: got %v, want %v（7203 は取り消しの後の残りの銘柄）", got.Failed, want)
	}
	assertSavedPrices(t, prices, []model.ClosingPrice{
		{StockCode: "6758", TradingDate: "2026-10-06", PriceTenths: 35000},
	})
}
