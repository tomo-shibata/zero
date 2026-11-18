package query

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// EvaluateAlerts は、1銘柄・1取引日 D の入力 in から、書く判定の結果と作る通知を決める純粋関数（価格の変動の通知のプラン 4.3）。
//
//  1. D がその銘柄の最新の取引日でない、または最新の分割の日が D より後なら、何もしない（要件9）。
//  2. 条件を持つ利用者ごとに、今の一覧と同じ計算（calculateStock）で保有数量・取得価格・含み損益・率を出す（終値は D の終値。要件10・要件11・要件14）。
//     保有数量が0の利用者の条件は飛ばす（判定の結果も書かない）。
//  3. 今の保有が始まった日 S を出す（HoldingStartDate。要件12・要件16）。S が D より後なら、保有なしと同じに扱って
//     その利用者のこの銘柄の条件をすべて飛ばす（判定の結果も通知も作らない。要件10・基準57）。
//  4. 条件ごとに当てはまるかを出す（alert_rules.go。要件11〜要件15）。
//  5. 前の取引日から以外は、前の判定が「S 以後の日付で当てはまった」のでなければ、当てはまったときに通知を作る。
//     前の取引日からは、当てはまったら作り、判定の結果は書かない（要件16・要件17）。
//
// 利用者ごとの計算が失敗したら、その利用者の条件だけを飛ばし、ほかの利用者の結果を返したうえで、飛ばした分のエラーを errors.Join で返す（要件21）。
// 前の取引日からの条件で前の終値がなければ、その条件だけを飛ばす（エラーにしない。要件13）。
// 判定の結果と通知は条件 ID の昇順で、0件でも nil ではなく空のスライス。
func EvaluateAlerts(in dto.AlertEvaluationInput) (dto.AlertEvaluationResult, error) {
	result := dto.AlertEvaluationResult{Judgments: []dto.AlertJudgment{}, Notifications: []dto.AlertNotification{}}
	if !isLatestPrice(in) {
		return result, nil
	}

	conditions := conditionsOfStock(in)
	byUser := make(map[uuid.UUID][]dto.AlertConditionRow)
	for _, c := range conditions {
		byUser[c.UserID] = append(byUser[c.UserID], c)
	}
	users := make([]uuid.UUID, 0, len(byUser))
	for u := range byUser {
		users = append(users, u)
	}
	// エラーの並びを毎回同じにするため、利用者は ID の文字列の昇順に見る。
	slices.SortFunc(users, func(a, b uuid.UUID) int { return cmp.Compare(a.String(), b.String()) })

	divisors := newSplitDivisors(in.Splits)
	var errs []error
	for _, user := range users {
		holding, held, err := alertHolding(in, user)
		if err != nil {
			errs = append(errs, fmt.Errorf("利用者 %s: %w", user, err))
			continue
		}
		if !held {
			continue
		}
		for _, cond := range byUser[user] {
			outcome, err := evaluateCondition(in, divisors, &holding, cond)
			if err != nil {
				errs = append(errs, fmt.Errorf("条件 %d: %w", cond.ID, err))
				continue
			}
			if !outcome.evaluated {
				continue
			}
			if cond.Kind != alertKindPrev {
				result.Judgments = append(result.Judgments, dto.AlertJudgment{
					ConditionID: cond.ID, StockCode: in.StockCode, TradingDate: in.TradingDate, Satisfied: outcome.satisfied,
				})
			}
			if outcome.satisfied && !alreadySatisfied(in, cond, holding.start) {
				result.Notifications = append(result.Notifications, dto.AlertNotification{
					UserID: user, Condition: cond, StockCode: in.StockCode, TradingDate: in.TradingDate,
					ClosePriceHundredths: in.ClosePriceHundredths, Details: outcome.details,
				})
			}
		}
	}
	slices.SortFunc(result.Judgments, func(a, b dto.AlertJudgment) int { return cmp.Compare(a.ConditionID, b.ConditionID) })
	slices.SortFunc(result.Notifications, func(a, b dto.AlertNotification) int {
		return cmp.Compare(a.Condition.ID, b.Condition.ID)
	})
	return result, errors.Join(errs...)
}

// isLatestPrice は、D がその銘柄の最新の取引日で、かつ最新の分割の日が D 以前なら true を返す（手順1。要件9）。
func isLatestPrice(in dto.AlertEvaluationInput) bool {
	if in.TradingDate != in.LatestTradingDate {
		return false
	}
	latest, ok := latestSplitDate(in.Splits)
	return !ok || latest <= in.TradingDate
}

// conditionsOfStock は、入力の条件のうち、全銘柄の条件とこの銘柄の条件だけを、条件 ID の昇順に返す。
func conditionsOfStock(in dto.AlertEvaluationInput) []dto.AlertConditionRow {
	var conditions []dto.AlertConditionRow
	for _, c := range in.Conditions {
		if c.Scope == alertScopeAll || c.StockCode == in.StockCode {
			conditions = append(conditions, c)
		}
	}
	slices.SortFunc(conditions, func(a, b dto.AlertConditionRow) int { return cmp.Compare(a.ID, b.ID) })
	return conditions
}

// userHolding は、1人の利用者のこの銘柄の、D の終値で計算した保有と、今の保有が始まった日。
type userHolding struct {
	holding dto.Holding
	start   string
	peak    *peakPoint // 最高値の条件を判定するときに、最初の1回だけ数えて入れる
}

// alertHolding は、利用者 user のこの銘柄の保有を、D の終値で計算する（手順2・手順3）。保有数量が0、または今の保有が始まった日が D より後なら held が false。
// 一覧と同じ計算を使うため、売買に D の終値を持たせてから calculateStock に渡す（呼び出し側の売買は書き換えない）。
func alertHolding(in dto.AlertEvaluationInput, user uuid.UUID) (h userHolding, held bool, err error) {
	trades := in.TradesByUser[user]
	if len(trades) == 0 {
		return userHolding{}, false, nil
	}
	priced := make([]dto.TradeRow, len(trades))
	for i, t := range trades {
		t.ClosePriceHundredths = new(in.ClosePriceHundredths)
		t.PriceDate = new(in.TradingDate)
		priced[i] = t
	}
	s, held, err := calculateStock(priced, in.Splits)
	if err != nil || !held {
		return userHolding{}, false, err
	}
	start, ok := HoldingStartDate(trades, in.Splits)
	// 今の保有が始まった日が D より後（例: D の翌日に買い直した）なら、D の時点では保有していないのと同じに扱う（要件10 の最後の一文・基準57）。
	// 日付は "YYYY-MM-DD" なので文字列で比べられる。同じ日（S = D）なら判定する。
	if !ok || start > in.TradingDate {
		return userHolding{}, false, nil
	}
	return userHolding{holding: s.holding, start: start}, true, nil
}

// alreadySatisfied は、前の判定が「今の保有が始まった日 start 以後の日付で当てはまった」なら true を返す（要件16）。
// 前の取引日からは前の判定を使わない（要件17）。
func alreadySatisfied(in dto.AlertEvaluationInput, cond dto.AlertConditionRow, start string) bool {
	if cond.Kind == alertKindPrev {
		return false
	}
	prev, ok := in.PreviousJudgments[cond.ID]
	return ok && prev.Satisfied && prev.TradingDate >= start
}
