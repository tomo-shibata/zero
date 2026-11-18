package command_test

import (
	"context"
	"errors"
	"math/big"
	"slices"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// 仕様: 要件（docs/requirements/price-import.md）の要件2・要件6・要件7・要件9・基準9（円・ドルの桁）と、
// プラン（docs/plans/price-import/plan.md）4.1・4.5 の command.ImportClosingPrices（Classify・Save）・PriceImportRow・PriceImportSkip・PriceTooFine・
// PriceImportPlan・repository.ClosingPriceImportRepository、5.1 の基準9（桁）の行、5.3 の「Classify（単体）」。
// 銘柄マスタは、通貨を返すだけのテスト用の repository（呼んだかどうかではなく、結果を見る）。
// 今日は 2026-11-16（日本時間）。Classify の tradingDate には "2026-11-16" を渡す。

const priceImportDate = "2026-11-16"

// priceImportRepo は、銘柄マスタの通貨を返し、SaveAll に渡された終値を覚えるテスト用の repository。
type priceImportRepo struct {
	currencies map[string]model.Currency
	findErr    error
	saveErr    error
	saveCalls  [][]model.ClosingPrice
}

func (r *priceImportRepo) FindCurrencies(_ context.Context, codes []string) (map[string]model.Currency, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	found := map[string]model.Currency{}
	for _, c := range codes {
		if cur, ok := r.currencies[c]; ok {
			found[c] = cur
		}
	}
	return found, nil
}

func (r *priceImportRepo) SaveAll(_ context.Context, prices []model.ClosingPrice) error {
	r.saveCalls = append(r.saveCalls, slices.Clone(prices))
	return r.saveErr
}

// priceImportRow は、Line・銘柄コード・終値（"2850.25" のような10進の文字列）の行を作る。
func priceImportRow(t *testing.T, line int, code, price string) command.PriceImportRow {
	t.Helper()
	r, ok := new(big.Rat).SetString(price)
	if !ok {
		t.Fatalf("終値 %q を読めない", price)
	}
	return command.PriceImportRow{Line: line, StockCode: code, Price: r}
}

// priceImportRepoWithMaster は、7203・6758（円）と AAPL（ドル）を銘柄マスタに持つ repository を返す（XYZ・ZZZ はない）。
func priceImportRepoWithMaster() *priceImportRepo {
	return &priceImportRepo{currencies: map[string]model.Currency{
		"7203": model.CurrencyJPY,
		"6758": model.CurrencyJPY,
		"AAPL": model.CurrencyUSD,
	}}
}

func priceOf(code string, hundredths int64) model.ClosingPrice {
	return model.ClosingPrice{StockCode: code, TradingDate: priceImportDate, PriceHundredths: hundredths}
}

// 基準9（桁）: 誤りにならない6つの値（円の 0.1・99999999.9・2850、ドルの 0.01・99999999.99・182.3）→ TooFine が空で、
// Prices が 0.01 単位で 10・9999999990・285000 と 1・9999999999・18230（取引日は tradingDate）。
func Test基準9_Classifyは円とドルの正しい桁を0_01単位にして登録する終値にする(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		code  string
		price string
		want  int64
	}{
		{"円_0.1", "7203", "0.1", 10},
		{"円_99999999.9", "7203", "99999999.9", 9999999990},
		{"円_2850", "7203", "2850", 285000},
		{"ドル_0.01", "AAPL", "0.01", 1},
		{"ドル_99999999.99", "AAPL", "99999999.99", 9999999999},
		{"ドル_182.3", "AAPL", "182.3", 18230},
		{"ドル_182.35", "AAPL", "182.35", 18235},
		// 値で決める（売買履歴と同じ）: 円の 2850.10（小数第2位が0）は誤りにならず 285010。
		{"円_2850.10", "7203", "2850.10", 285010},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			uc := command.NewImportClosingPrices(priceImportRepoWithMaster())

			plan, err := uc.Classify(t.Context(), []command.PriceImportRow{priceImportRow(t, 2, tc.code, tc.price)}, priceImportDate)

			if err != nil {
				t.Fatalf("Classify: got err %v, want nil", err)
			}
			if len(plan.TooFine) != 0 || len(plan.Skipped) != 0 {
				t.Errorf("TooFine=%+v Skipped=%+v, want どちらも空", plan.TooFine, plan.Skipped)
			}
			if want := []model.ClosingPrice{priceOf(tc.code, tc.want)}; !slices.Equal(plan.Prices, want) {
				t.Errorf("Prices:\n got %+v\nwant %+v", plan.Prices, want)
			}
		})
	}
}

// 基準9（桁）: 円の 2850.25（小数第2位以下を含む）・ドルの 182.355（小数第3位以下を含む）は TooFine に入り、Currency がそれぞれ円・ドル。登録する終値には入れない。
func Test基準9_Classifyは通貨の精度に合わない終値をTooFineにする(t *testing.T) {
	t.Parallel()
	uc := command.NewImportClosingPrices(priceImportRepoWithMaster())
	rows := []command.PriceImportRow{
		priceImportRow(t, 2, "7203", "2850.25"),
		priceImportRow(t, 3, "AAPL", "182.355"),
	}

	plan, err := uc.Classify(t.Context(), rows, priceImportDate)

	if err != nil {
		t.Fatalf("Classify: got err %v, want nil", err)
	}
	want := []command.PriceTooFine{
		{Line: 2, Currency: model.CurrencyJPY},
		{Line: 3, Currency: model.CurrencyUSD},
	}
	if !slices.Equal(plan.TooFine, want) {
		t.Errorf("TooFine:\n got %+v\nwant %+v", plan.TooFine, want)
	}
	if len(plan.Prices) != 0 {
		t.Errorf("Prices: got %+v, want 空（精度に合わない行は登録しない）", plan.Prices)
	}
}

// 基準9（桁）: 境界の外側。円の 0.15（小数第2位）・ドルの 0.001（小数第3位）も TooFine。
func Test基準9_Classifyは桁の境界の外側をTooFineにする(t *testing.T) {
	t.Parallel()
	uc := command.NewImportClosingPrices(priceImportRepoWithMaster())
	rows := []command.PriceImportRow{
		priceImportRow(t, 2, "7203", "0.15"),
		priceImportRow(t, 3, "AAPL", "0.001"),
	}

	plan, err := uc.Classify(t.Context(), rows, priceImportDate)

	if err != nil {
		t.Fatalf("Classify: got err %v, want nil", err)
	}
	want := []command.PriceTooFine{
		{Line: 2, Currency: model.CurrencyJPY},
		{Line: 3, Currency: model.CurrencyUSD},
	}
	if !slices.Equal(plan.TooFine, want) {
		t.Errorf("TooFine:\n got %+v\nwant %+v", plan.TooFine, want)
	}
}

// 5.3「Classify」: Prices・Skipped・TooFine が行の順。Skipped は銘柄コードの順（ZZZ → XYZ）と行の順（2行目の ZZZ、5行目の XYZ）が違うデータ。
// Prices の取引日が tradingDate、終値が 0.01 単位（円の 2850 は 285000、ドルの 182.35 は 18235）。
// 銘柄マスタにない行は、桁に関わらず Skipped（通貨がないので桁は確かめない。要件6）。
func Test補助_Classifyは行の順にPricesとSkippedとTooFineに分ける(t *testing.T) {
	t.Parallel()
	uc := command.NewImportClosingPrices(priceImportRepoWithMaster())
	rows := []command.PriceImportRow{
		priceImportRow(t, 2, "ZZZ", "5"),
		priceImportRow(t, 3, "7203", "2850"),
		priceImportRow(t, 4, "AAPL", "182.355"),
		priceImportRow(t, 5, "XYZ", "10.555"),
		priceImportRow(t, 6, "AAPL", "182.35"),
		priceImportRow(t, 7, "6758", "3600.55"),
	}

	plan, err := uc.Classify(t.Context(), rows, "2026-11-13")

	if err != nil {
		t.Fatalf("Classify: got err %v, want nil", err)
	}
	wantPrices := []model.ClosingPrice{
		{StockCode: "7203", TradingDate: "2026-11-13", PriceHundredths: 285000},
		{StockCode: "AAPL", TradingDate: "2026-11-13", PriceHundredths: 18235},
	}
	if !slices.Equal(plan.Prices, wantPrices) {
		t.Errorf("Prices:\n got %+v\nwant %+v", plan.Prices, wantPrices)
	}
	wantSkipped := []command.PriceImportSkip{{Line: 2, StockCode: "ZZZ"}, {Line: 5, StockCode: "XYZ"}}
	if !slices.Equal(plan.Skipped, wantSkipped) {
		t.Errorf("Skipped:\n got %+v\nwant %+v", plan.Skipped, wantSkipped)
	}
	wantTooFine := []command.PriceTooFine{{Line: 4, Currency: model.CurrencyUSD}, {Line: 7, Currency: model.CurrencyJPY}}
	if !slices.Equal(plan.TooFine, wantTooFine) {
		t.Errorf("TooFine:\n got %+v\nwant %+v", plan.TooFine, wantTooFine)
	}
}

// 要件9: すべての行が銘柄マスタにないとき、Prices は空で Skipped に全部が入る。
func Test要件9_Classifyはすべての行が銘柄マスタにないとき登録する終値が空になる(t *testing.T) {
	t.Parallel()
	uc := command.NewImportClosingPrices(priceImportRepoWithMaster())

	plan, err := uc.Classify(t.Context(), []command.PriceImportRow{priceImportRow(t, 2, "XYZ", "10")}, priceImportDate)

	if err != nil {
		t.Fatalf("Classify: got err %v, want nil", err)
	}
	if len(plan.Prices) != 0 || len(plan.TooFine) != 0 {
		t.Errorf("Prices=%+v TooFine=%+v, want どちらも空", plan.Prices, plan.TooFine)
	}
	if want := []command.PriceImportSkip{{Line: 2, StockCode: "XYZ"}}; !slices.Equal(plan.Skipped, want) {
		t.Errorf("Skipped:\n got %+v\nwant %+v", plan.Skipped, want)
	}
}

// 4.1: Classify は何も書き込まない（SaveAll を呼ばない）。銘柄マスタが読めないときはエラーを返す。
func Test補助_Classifyは何も書き込まず銘柄マスタを読めなければエラーを返す(t *testing.T) {
	t.Parallel()
	repo := priceImportRepoWithMaster()
	uc := command.NewImportClosingPrices(repo)
	if _, err := uc.Classify(t.Context(), []command.PriceImportRow{priceImportRow(t, 2, "7203", "2850")}, priceImportDate); err != nil {
		t.Fatalf("Classify: got err %v, want nil", err)
	}
	if len(repo.saveCalls) != 0 {
		t.Errorf("Classify が SaveAll を呼んだ: %+v（何も書き込まない）", repo.saveCalls)
	}

	broken := &priceImportRepo{findErr: errors.New("db down")}
	_, err := command.NewImportClosingPrices(broken).Classify(t.Context(), []command.PriceImportRow{priceImportRow(t, 2, "7203", "2850")}, priceImportDate)
	if err == nil {
		t.Error("銘柄マスタを読めないとき: got err nil, want エラー")
	}
}

// Save: prices を保存する（渡した終値がそのまま保存の対象になる）。
func Test補助_Saveは渡した終値を保存する(t *testing.T) {
	t.Parallel()
	repo := priceImportRepoWithMaster()
	prices := []model.ClosingPrice{priceOf("7203", 285000), priceOf("AAPL", 18235)}

	if err := command.NewImportClosingPrices(repo).Save(t.Context(), prices); err != nil {
		t.Fatalf("Save: got err %v, want nil", err)
	}

	if len(repo.saveCalls) != 1 || !slices.Equal(repo.saveCalls[0], prices) {
		t.Errorf("保存された終値: got %+v, want 1回の保存で %+v", repo.saveCalls, prices)
	}
}

// 要件9: 0件なら何もしない（保存を呼ばず、エラーも返さない）。
func Test要件9_Saveは0件なら何もしない(t *testing.T) {
	t.Parallel()
	repo := priceImportRepoWithMaster()
	uc := command.NewImportClosingPrices(repo)

	for name, prices := range map[string][]model.ClosingPrice{"nil": nil, "空": {}} {
		if err := uc.Save(t.Context(), prices); err != nil {
			t.Errorf("Save(%s): got err %v, want nil", name, err)
		}
	}

	if len(repo.saveCalls) != 0 {
		t.Errorf("0件なのに保存を呼んだ: %+v", repo.saveCalls)
	}
}

// 要件2: 保存の失敗はエラーとして返す。
func Test補助_Saveは保存の失敗をエラーで返す(t *testing.T) {
	t.Parallel()
	repo := &priceImportRepo{saveErr: errors.New("db down")}

	if err := command.NewImportClosingPrices(repo).Save(t.Context(), []model.ClosingPrice{priceOf("7203", 285000)}); err == nil {
		t.Error("保存が失敗したとき: got err nil, want エラー")
	}
}

// importAlertEvaluator は、command.PriceAlertEvaluator を実装したテスト用の判定（境界の外側の fake）。
// Execute に渡された終値を覚え、呼ばれた時点で SaveAll が呼ばれていた回数も覚える。err が nil でなければ、それを返す。
type importAlertEvaluator struct {
	repo          *priceImportRepo
	err           error
	calls         [][]model.ClosingPrice
	saveCountThen []int
}

func (e *importAlertEvaluator) Execute(_ context.Context, prices []model.ClosingPrice) error {
	e.calls = append(e.calls, slices.Clone(prices))
	e.saveCountThen = append(e.saveCountThen, len(e.repo.saveCalls))
	return e.err
}

// 補助 4.4: 判定をつないだとき、保存が成功したら、保存した終値を1回で、保存の後に渡す。
func Test補助_4_4_Saveは保存が成功したら保存した終値を1回で判定に渡す(t *testing.T) {
	t.Parallel()
	repo := priceImportRepoWithMaster()
	evaluator := &importAlertEvaluator{repo: repo}
	uc := command.NewImportClosingPrices(repo).WithAlertEvaluator(evaluator)
	prices := []model.ClosingPrice{priceOf("7203", 285000), priceOf("AAPL", 18235)}

	if err := uc.Save(t.Context(), prices); err != nil {
		t.Fatalf("Save: got err %v, want nil", err)
	}

	if len(evaluator.calls) != 1 || !slices.Equal(evaluator.calls[0], prices) {
		t.Errorf("判定に渡した終値: got %+v, want %+v の1回", evaluator.calls, prices)
	}
	if len(evaluator.saveCountThen) == 1 && evaluator.saveCountThen[0] != 1 {
		t.Errorf("判定が呼ばれた時点の SaveAll の回数: got %d, want 1（コミットの後に判定を呼ぶ）", evaluator.saveCountThen[0])
	}
}

// 補助 4.4: SaveAll が失敗したときは、判定を呼ばない（コミットが成功した後だけ）。エラーは今までどおり返す。
func Test補助_4_4_Saveは保存が失敗したら判定を呼ばない(t *testing.T) {
	t.Parallel()
	repo := &priceImportRepo{saveErr: errors.New("db down")}
	evaluator := &importAlertEvaluator{repo: repo}
	uc := command.NewImportClosingPrices(repo).WithAlertEvaluator(evaluator)

	err := uc.Save(t.Context(), []model.ClosingPrice{priceOf("7203", 285000)})

	if err == nil {
		t.Error("保存が失敗したとき: got err nil, want エラー")
	}
	if len(evaluator.calls) != 0 {
		t.Errorf("保存が失敗したのに判定を呼んだ: %+v", evaluator.calls)
	}
}

// 補助 4.4: 0件のときは、保存せず、判定も呼ばない。
func Test補助_4_4_Saveは0件なら判定を呼ばない(t *testing.T) {
	t.Parallel()
	repo := &priceImportRepo{}
	evaluator := &importAlertEvaluator{repo: repo}
	uc := command.NewImportClosingPrices(repo).WithAlertEvaluator(evaluator)

	for name, prices := range map[string][]model.ClosingPrice{"nil": nil, "空": {}} {
		if err := uc.Save(t.Context(), prices); err != nil {
			t.Errorf("Save(%s): got err %v, want nil", name, err)
		}
	}

	if len(evaluator.calls) != 0 {
		t.Errorf("0件なのに判定を呼んだ: %+v", evaluator.calls)
	}
}

// 補助 4.4・要件21: 判定が失敗しても、Save は成功のまま（task の結果の表示は変えない）。保存した終値は残る。
func Test補助_要件21_Saveは判定が失敗してもエラーを返さない(t *testing.T) {
	t.Parallel()
	repo := priceImportRepoWithMaster()
	evaluator := &importAlertEvaluator{repo: repo, err: errors.New("fake: 判定に失敗")}
	uc := command.NewImportClosingPrices(repo).WithAlertEvaluator(evaluator)

	if err := uc.Save(t.Context(), []model.ClosingPrice{priceOf("7203", 285000)}); err != nil {
		t.Errorf("Save: got err %v, want nil（判定の失敗は結果に出さない）", err)
	}

	if len(repo.saveCalls) != 1 {
		t.Errorf("SaveAll の回数: got %d, want 1", len(repo.saveCalls))
	}
	if len(evaluator.calls) != 1 {
		t.Errorf("判定の呼び出し: got %d回, want 1回", len(evaluator.calls))
	}
}

// 補助 4.4: 判定が nil のときは、判定を呼ばず今と同じに動く（panic もしない）。
func Test補助_4_4_判定がnilのときSaveは今と同じに動く(t *testing.T) {
	t.Parallel()
	repo := priceImportRepoWithMaster()
	uc := command.NewImportClosingPrices(repo).WithAlertEvaluator(nil)

	if err := uc.Save(t.Context(), []model.ClosingPrice{priceOf("7203", 285000)}); err != nil {
		t.Errorf("Save: got err %v, want nil", err)
	}

	if len(repo.saveCalls) != 1 {
		t.Errorf("SaveAll の回数: got %d, want 1", len(repo.saveCalls))
	}
}
