package write

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/sqlcgen"
)

// AlertRepository は、PostgreSQL への価格の変動の通知の判定の入力の読み取りと、判定の結果・通知の書き込み
// （command.AlertEvaluationStore）の実装。読み取りは alert_evaluation_input.go。
type AlertRepository struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
}

// AlertRepository が command.AlertEvaluationStore を満たすことを、コンパイル時に確かめる。
var _ command.AlertEvaluationStore = (*AlertRepository)(nil)

// NewAlertRepository は、pool の DB を使う AlertRepository を返す。
func NewAlertRepository(pool *pgxpool.Pool) *AlertRepository {
	return &AlertRepository{pool: pool, queries: sqlcgen.New(pool)}
}

// SaveEvaluation は、1つのトランザクションで、判定の結果の書き込みと通知の追加を行う（プラン 4.1・4.2）。
//   - 判定の結果は、当てはまった条件と当てはまらなかった条件に分けて、同じ（条件・銘柄・取引日）の行があれば上書きする。
//     書いた後に、条件ごとに取引日の新しい順に2件だけ残し、それより古い行を消す。
//   - 通知は、同じ（条件・銘柄・取引日）の通知があれば追加しない（作った通知は書き換えない。要件18）。
//
// 判定の結果も通知も、判定の途中で消された条件の分は書かない（残っている条件の分だけ書く。通知は、条件の持ち主が通知の利用者と同じものだけ）。
// 残っている条件は SQL の中でロックするので、条件を消す側はこのトランザクションのコミットまで待つ。
//
// 途中で失敗したら、コミットせずに元に戻す。result は1つの銘柄・1つの取引日の分（判定の結果と通知は同じ銘柄・取引日）。0件なら何もしない。
func (r *AlertRepository) SaveEvaluation(ctx context.Context, result dto.AlertEvaluationResult) error {
	stockCode, tradingDate, ok := evaluationTarget(result)
	if !ok {
		return nil
	}
	date, err := time.Parse(time.DateOnly, tradingDate)
	if err != nil {
		return fmt.Errorf("取引日 %q を読めません: %w", tradingDate, err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("トランザクションを始められません: %w", err)
	}
	// Rollback と Commit には ctx の取り消しを引き継がない（ClosingPriceImportRepository.SaveAll と同じ理由）。
	finishCtx := context.WithoutCancel(ctx)
	defer func() { _ = tx.Rollback(finishCtx) }()
	q := r.queries.WithTx(tx)

	if err := saveJudgments(ctx, q, result.Judgments, stockCode, pgtype.Date{Time: date, Valid: true}); err != nil {
		return err
	}
	if err := insertNotifications(ctx, q, result.Notifications); err != nil {
		return err
	}
	if err := tx.Commit(finishCtx); err != nil {
		return fmt.Errorf("コミットに失敗しました: %w", err)
	}
	return nil
}

// evaluationTarget は、結果の銘柄コードと取引日を返す。結果は1つの銘柄・1つの取引日の分なので、最初の判定の結果（なければ最初の通知）から取る。
// 判定の結果も通知もなければ ok が false。
func evaluationTarget(result dto.AlertEvaluationResult) (stockCode, tradingDate string, ok bool) {
	switch {
	case len(result.Judgments) > 0:
		return result.Judgments[0].StockCode, result.Judgments[0].TradingDate, true
	case len(result.Notifications) > 0:
		return result.Notifications[0].StockCode, result.Notifications[0].TradingDate, true
	default:
		return "", "", false
	}
}

// saveJudgments は、判定の結果を書き（当てはまった条件と当てはまらなかった条件の2回）、条件ごとに新しい2件だけを残す。
func saveJudgments(ctx context.Context, q *sqlcgen.Queries, judgments []dto.AlertJudgment, stockCode string, date pgtype.Date) error {
	var satisfied, unsatisfied []int64
	for _, j := range judgments {
		if j.Satisfied {
			satisfied = append(satisfied, j.ConditionID)
		} else {
			unsatisfied = append(unsatisfied, j.ConditionID)
		}
	}
	for _, group := range []struct {
		ids       []int64
		satisfied bool
	}{{satisfied, true}, {unsatisfied, false}} {
		if len(group.ids) == 0 {
			continue
		}
		err := q.UpsertAlertJudgments(ctx, sqlcgen.UpsertAlertJudgmentsParams{
			StockCode: stockCode, TradingDate: date, Satisfied: group.satisfied, ConditionIds: group.ids,
		})
		if err != nil {
			return fmt.Errorf("判定の結果を書けません: %w", err)
		}
	}
	if len(judgments) == 0 {
		return nil
	}
	all := slices.Concat(satisfied, unsatisfied)
	if err := q.PruneAlertJudgments(ctx, sqlcgen.PruneAlertJudgmentsParams{StockCode: stockCode, ConditionIds: all}); err != nil {
		return fmt.Errorf("古い判定の結果を消せません: %w", err)
	}
	return nil
}

// notificationJSON は、InsertAlertNotifications に渡す JSON の配列の1要素（キーは db/query/price_alerts.sql の jsonb_to_recordset の列と同じ）。
type notificationJSON struct {
	UserID              uuid.UUID                    `json:"user_id"`
	ConditionID         int64                        `json:"condition_id"`
	Kind                string                       `json:"kind"`
	Scope               string                       `json:"scope"`
	ConditionStockCode  string                       `json:"condition_stock_code"`
	Direction           string                       `json:"direction"`
	ThresholdHundredths int64                        `json:"threshold_hundredths"`
	StockCode           string                       `json:"stock_code"`
	TradingDate         string                       `json:"trading_date"`
	CloseHundredths     int64                        `json:"close_hundredths"`
	Details             dto.AlertNotificationDetails `json:"details"`
}

// insertNotifications は、通知をまとめて1回の問い合わせで追加する（同じ条件・銘柄・取引日があれば何もしない）。0件なら何もしない。
// 通知の数だけ問い合わせると、条件が多い利用者の判定が遅くなるため（非機能3）。
func insertNotifications(ctx context.Context, q *sqlcgen.Queries, notifications []dto.AlertNotification) error {
	if len(notifications) == 0 {
		return nil
	}
	rows := make([]notificationJSON, 0, len(notifications))
	for _, n := range notifications {
		rows = append(rows, notificationJSON{
			UserID: n.UserID, ConditionID: n.Condition.ID, Kind: n.Condition.Kind, Scope: n.Condition.Scope,
			ConditionStockCode: n.Condition.StockCode, Direction: n.Condition.Direction,
			ThresholdHundredths: n.Condition.ThresholdHundredths, StockCode: n.StockCode, TradingDate: n.TradingDate,
			CloseHundredths: n.ClosePriceHundredths, Details: n.Details,
		})
	}
	payload, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("通知を JSON にできません: %w", err)
	}
	if err := q.InsertAlertNotifications(ctx, payload); err != nil {
		return fmt.Errorf("通知を追加できません: %w", err)
	}
	return nil
}
