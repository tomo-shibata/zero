package command_test

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// 仕様: 価格の変動の通知の要件（docs/requirements/price-alerts.md）の要件21（判定に失敗しても、ほかの条件・銘柄の判定は続ける）と、
// プラン（docs/plans/price-alerts/plan.md）4.1・4.3（利用者ごとの計算の失敗）、4.8 の PR① の境界（command.AlertEvaluationStore・NewEvaluatePriceAlerts・
// EvaluatePriceAlerts.Execute）、5.2 の evaluate_price_alerts_test.go の行。
// store は、読み取りと書き込みの境界の外側の fake（DB の代わり）。渡された引数と、書き込みに渡された結果を覚える（呼んだかではなく、何を書いたかを見る）。
// 判定そのもの（EvaluateAlerts）は fake にせず本物を使う（入力から通知ができることで確かめる）。

// fakeAlertStore は、command.AlertEvaluationStore を実装したテスト用の store。
type fakeAlertStore struct {
	mu         sync.Mutex
	inputs     map[string]dto.AlertEvaluationInput // 銘柄コード → 読み取りが返す入力
	loadErrors map[string]error                    // 銘柄コード → 読み取りが返すエラー
	saveErrors map[string]error                    // 銘柄コード（結果の通知か判定の銘柄）→ 書き込みが返すエラー
	loads      []alertLoadCall
	saved      []dto.AlertEvaluationResult
}

// alertLoadCall は、LoadEvaluationInput に渡された引数。
type alertLoadCall struct {
	code            string
	tradingDate     string
	closeHundredths int64
}

func (s *fakeAlertStore) LoadEvaluationInput(_ context.Context, code, tradingDate string, closeHundredths int64) (dto.AlertEvaluationInput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loads = append(s.loads, alertLoadCall{code, tradingDate, closeHundredths})
	if err, ok := s.loadErrors[code]; ok {
		return dto.AlertEvaluationInput{}, err
	}
	return s.inputs[code], nil
}

func (s *fakeAlertStore) SaveEvaluation(_ context.Context, result dto.AlertEvaluationResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved = append(s.saved, result)
	for _, j := range result.Judgments {
		if err, ok := s.saveErrors[j.StockCode]; ok {
			return err
		}
	}
	return nil
}

// savedCodes は、書き込みに渡された結果の銘柄コード（判定の結果の銘柄）を、渡された順に返す。
func (s *fakeAlertStore) savedCodes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var codes []string
	for _, r := range s.saved {
		for _, j := range r.Judgments {
			codes = append(codes, j.StockCode)
		}
	}
	return codes
}

// savedNotificationsOf は、書き込みに渡された、銘柄 code の通知を返す。
func (s *fakeAlertStore) savedNotificationsOf(code string) []dto.AlertNotification {
	s.mu.Lock()
	defer s.mu.Unlock()
	var got []dto.AlertNotification
	for _, r := range s.saved {
		for _, n := range r.Notifications {
			if n.StockCode == code {
				got = append(got, n)
			}
		}
	}
	return got
}

var (
	alertUserA = uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	alertUserB = uuid.MustParse("00000000-0000-0000-0000-00000000000b")
)

// costDownInput は、銘柄 code（円）の、2026-11-10・終値 1,950円の入力。Aが「買 2,500円 100株」で、全銘柄・取得価格から 20% 下がったら（-22.0%。当てはまる。条件 ID 1）。
func costDownInput(code string) dto.AlertEvaluationInput {
	return dto.AlertEvaluationInput{
		StockCode:            code,
		Currency:             dto.CurrencyJPY,
		TradingDate:          "2026-11-10",
		ClosePriceHundredths: 195000,
		LatestTradingDate:    "2026-11-10",
		Conditions: []dto.AlertConditionRow{{
			ID: 1, UserID: alertUserA, Kind: "cost", Scope: "all", Direction: "down", ThresholdHundredths: 2000, AddedOn: "2026-11-01",
		}},
		TradesByUser: map[uuid.UUID][]dto.TradeRow{alertUserA: {{
			ID: 1, Code: code, Name: "テスト銘柄" + code, Currency: dto.CurrencyJPY, Side: dto.TradeSideBuy,
			Quantity: 100, PriceHundredths: 250000, TradeDate: "2026-01-10",
		}}},
		PreviousJudgments: map[int64]dto.AlertJudgmentRow{},
	}
}

// evaluatedPrice は、銘柄 code の 2026-11-10・1,950円の終値。
func evaluatedPrice(code string) model.ClosingPrice {
	return model.ClosingPrice{StockCode: code, TradingDate: "2026-11-10", PriceHundredths: 195000}
}

// 補助 要件21: 正常なときは、渡された終値の銘柄ごとに、読み取りに銘柄コード・取引日・終値を渡し、判定の結果を書き込みに渡し、エラーを返さない。
func Test補助_要件21_すべて成功すれば銘柄ごとに読んで判定して書きエラーを返さない(t *testing.T) {
	t.Parallel()
	store := &fakeAlertStore{inputs: map[string]dto.AlertEvaluationInput{
		"7203": costDownInput("7203"),
		"6758": costDownInput("6758"),
	}}
	var evaluator command.PriceAlertEvaluator = command.NewEvaluatePriceAlerts(store)

	err := evaluator.Execute(t.Context(), []model.ClosingPrice{evaluatedPrice("7203"), evaluatedPrice("6758")})

	if err != nil {
		t.Fatalf("Execute: got err %v, want nil", err)
	}
	// 並びは決めない（銘柄コードの昇順にして比べる）。
	wantLoads := []alertLoadCall{{"6758", "2026-11-10", 195000}, {"7203", "2026-11-10", 195000}}
	gotLoads := slices.Clone(store.loads)
	slices.SortFunc(gotLoads, func(a, b alertLoadCall) int { return cmp.Compare(a.code, b.code) })
	if !slices.Equal(gotLoads, wantLoads) {
		t.Errorf("LoadEvaluationInput に渡した引数: got %+v, want %+v（銘柄コード・取引日・終値）", store.loads, wantLoads)
	}
	for _, code := range []string{"7203", "6758"} {
		if got := store.savedNotificationsOf(code); len(got) != 1 {
			t.Errorf("%s の書き込みに渡した通知: got %d件 %+v, want 1件（-22.0%% で当てはまる）", code, len(got), got)
		}
	}
}

// 補助 要件21: 1銘柄目の読み取りが失敗しても、2銘柄目を判定して書き、戻り値が error になる。
// 7203 の読み取りが失敗し、6758 は読める。1銘柄目の失敗で止めると 6758 の通知が書かれず落ちる。
func Test補助_要件21_1銘柄目の読み取りが失敗しても2銘柄目を判定して書きエラーを返す(t *testing.T) {
	t.Parallel()
	loadErr := errors.New("fake: connection reset")
	store := &fakeAlertStore{
		inputs:     map[string]dto.AlertEvaluationInput{"6758": costDownInput("6758")},
		loadErrors: map[string]error{"7203": loadErr},
	}
	evaluator := command.NewEvaluatePriceAlerts(store)

	err := evaluator.Execute(t.Context(), []model.ClosingPrice{evaluatedPrice("7203"), evaluatedPrice("6758")})

	if err == nil {
		t.Error("Execute: got nil, want error（1銘柄の失敗はエラーで返す）")
	}
	if got := store.savedNotificationsOf("6758"); len(got) != 1 {
		t.Errorf("6758 の書き込みに渡した通知: got %d件 %+v, want 1件（1銘柄目の失敗で止めない）", len(got), got)
	}
	if got := store.savedNotificationsOf("7203"); len(got) != 0 {
		t.Errorf("7203 の書き込みに渡した通知: got %+v, want なし（読み取りに失敗した銘柄は書かない）", got)
	}
}

// 補助 要件21: 書き込みが失敗した銘柄があっても、次の銘柄を判定して書き、戻り値が error になる。
func Test補助_要件21_1銘柄目の書き込みが失敗しても2銘柄目を判定して書きエラーを返す(t *testing.T) {
	t.Parallel()
	store := &fakeAlertStore{
		inputs:     map[string]dto.AlertEvaluationInput{"7203": costDownInput("7203"), "6758": costDownInput("6758")},
		saveErrors: map[string]error{"7203": errors.New("fake: deadlock detected")},
	}
	evaluator := command.NewEvaluatePriceAlerts(store)

	err := evaluator.Execute(t.Context(), []model.ClosingPrice{evaluatedPrice("7203"), evaluatedPrice("6758")})

	if err == nil {
		t.Error("Execute: got nil, want error（書き込みの失敗はエラーで返す）")
	}
	codes := store.savedCodes()
	if !slices.Contains(codes, "6758") {
		t.Errorf("書き込みに渡した銘柄: got %v, want 6758 を含む（7203 の書き込みが失敗しても続ける）", codes)
	}
}

// 補助 要件21（4.3）: 1人の利用者の計算が失敗する入力（売りすぎ）でも、ほかの利用者の通知を SaveEvaluation に渡し、戻り値が error になる。
// Aは正常（-22.0% で当てはまる）、Bの売買履歴は「買 100株」「売 200株」。EvaluateAlerts がエラーと一緒に返した結果も書く。
func Test補助_要件21_ある利用者の計算が失敗してもほかの利用者の通知を書きエラーを返す(t *testing.T) {
	t.Parallel()
	in := costDownInput("7203")
	in.Conditions = append(in.Conditions, dto.AlertConditionRow{
		ID: 2, UserID: alertUserB, Kind: "cost", Scope: "all", Direction: "down", ThresholdHundredths: 2000, AddedOn: "2026-11-01",
	})
	in.TradesByUser[alertUserB] = []dto.TradeRow{
		{ID: 2, Code: "7203", Name: "テスト銘柄7203", Currency: dto.CurrencyJPY, Side: dto.TradeSideBuy, Quantity: 100, PriceHundredths: 250000, TradeDate: "2026-01-10"},
		{ID: 3, Code: "7203", Name: "テスト銘柄7203", Currency: dto.CurrencyJPY, Side: dto.TradeSideSell, Quantity: 200, PriceHundredths: 260000, TradeDate: "2026-02-10"},
	}
	store := &fakeAlertStore{inputs: map[string]dto.AlertEvaluationInput{"7203": in}}
	evaluator := command.NewEvaluatePriceAlerts(store)

	err := evaluator.Execute(t.Context(), []model.ClosingPrice{evaluatedPrice("7203")})

	if err == nil {
		t.Error("Execute: got nil, want error（Bの計算の失敗はエラーで返す）")
	}
	got := store.savedNotificationsOf("7203")
	if len(got) != 1 || got[0].UserID != alertUserA {
		t.Errorf("書き込みに渡した通知: got %+v, want Aの1件だけ（Bの条件は飛ばし、Aの結果は書く）", got)
	}
}
