package command

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// PriceAlertEvaluator は、保存した終値を価格の変動の通知の条件で判定する（価格の変動の通知のプラン 4.4）。
// 3つの保存の道（定時の取得・今すぐ更新・価格の CSV の登録）が、保存の後に呼ぶ。
type PriceAlertEvaluator interface {
	Execute(ctx context.Context, prices []model.ClosingPrice) error
}

// AlertEvaluationStore は、判定の入力の読み取りと、判定の結果・通知の書き込み（実装は infrastructure の write）。
type AlertEvaluationStore interface {
	// LoadEvaluationInput は、銘柄 code の、取引日 tradingDate・終値 closeHundredths の判定の入力を読む。
	LoadEvaluationInput(ctx context.Context, code, tradingDate string, closeHundredths int64) (dto.AlertEvaluationInput, error)
	// SaveEvaluation は、判定の結果の書き込みと通知の追加を、1つのトランザクションで行う。
	// 通知は同じ（条件・銘柄・取引日）があれば追加しない。判定の結果は上書きし、組ごとに新しい2件だけを残す。
	SaveEvaluation(ctx context.Context, result dto.AlertEvaluationResult) error
}

// EvaluatePriceAlerts は、保存した終値をまとめて判定し、判定の結果と通知を書くユースケース（要件8〜要件22）。
type EvaluatePriceAlerts struct {
	store AlertEvaluationStore
}

// NewEvaluatePriceAlerts は、store を使う EvaluatePriceAlerts（PriceAlertEvaluator）を返す。
func NewEvaluatePriceAlerts(store AlertEvaluationStore) *EvaluatePriceAlerts {
	return &EvaluatePriceAlerts{store: store}
}

// Execute は、終値 prices の銘柄ごとに、入力を読み、判定し（query.EvaluateAlerts）、結果を書く。
// 1つの銘柄の読み取り・判定・書き込みが失敗しても止めずに次の銘柄へ進み、最後に errors.Join で返す（要件21）。
// 判定が利用者ごとの失敗のエラーと一緒に結果を返したときも、その結果は書く（ほかの利用者の判定は続ける）。
func (u *EvaluatePriceAlerts) Execute(ctx context.Context, prices []model.ClosingPrice) error {
	var errs []error
	for _, p := range prices {
		if err := u.evaluateOne(ctx, p); err != nil {
			errs = append(errs, fmt.Errorf("銘柄 %q: %w", p.StockCode, err))
		}
	}
	return errors.Join(errs...)
}

// evaluateOne は、1つの終値の判定の入力を読み、判定して書く。判定のエラーがあっても、返ってきた結果は書いてから、そのエラーを返す。
func (u *EvaluatePriceAlerts) evaluateOne(ctx context.Context, p model.ClosingPrice) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("取り消されたので判定しません: %w", err)
	}
	input, err := u.store.LoadEvaluationInput(ctx, p.StockCode, p.TradingDate, p.PriceHundredths)
	if err != nil {
		return fmt.Errorf("判定の入力を読めません: %w", err)
	}
	result, evalErr := query.EvaluateAlerts(input)
	if len(result.Judgments) > 0 || len(result.Notifications) > 0 {
		if err := u.store.SaveEvaluation(ctx, result); err != nil {
			return errors.Join(evalErr, fmt.Errorf("判定の結果を書けません: %w", err))
		}
	}
	return evalErr
}

// evaluateAlerts は、保存した終値 saved を e に渡す。e が nil か、saved が空なら何もしない。
// 判定の失敗は終値の保存の結果に響かせないので、エラーはログに出すだけにする（要件21）。
// エラーの文字列は DB の値を含みうるので %q で出す（FetchClosingPrices の失敗のログと同じ理由）。
func evaluateAlerts(ctx context.Context, e PriceAlertEvaluator, saved []model.ClosingPrice) {
	if e == nil || len(saved) == 0 {
		return
	}
	if err := e.Execute(ctx, saved); err != nil {
		log.Printf("価格の変動の通知の判定: %q", err)
	}
}
