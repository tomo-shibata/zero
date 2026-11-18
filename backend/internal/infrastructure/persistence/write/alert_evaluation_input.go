package write

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/sqlcgen"
)

// LoadEvaluationInput は、銘柄 code の、取引日 tradingDate・終値 closeHundredths の判定の入力を読む（プラン 4.3 の入力）。
// D が最新の取引日でない、または最新の分割の日が D より後なら、判定は何もしないので、条件などは読まない
// （入力には、最新の取引日と分割だけを入れて返す）。効く条件が1件もなければ、売買や終値も読まない。
func (r *AlertRepository) LoadEvaluationInput(ctx context.Context, code, tradingDate string, closeHundredths int64) (dto.AlertEvaluationInput, error) {
	date, err := time.Parse(time.DateOnly, tradingDate)
	if err != nil {
		return dto.AlertEvaluationInput{}, fmt.Errorf("取引日 %q を読めません: %w", tradingDate, err)
	}
	currency, err := r.queries.GetAlertStockCurrency(ctx, code)
	if err != nil {
		return dto.AlertEvaluationInput{}, fmt.Errorf("銘柄の通貨を読めません: %w", err)
	}
	latestDate, err := r.queries.GetLatestTradingDate(ctx, code)
	if err != nil {
		return dto.AlertEvaluationInput{}, fmt.Errorf("最新の取引日を読めません: %w", err)
	}
	latest, err := dateString(latestDate)
	if err != nil {
		return dto.AlertEvaluationInput{}, fmt.Errorf("最新の取引日: %w", err)
	}
	splits, err := r.loadSplits(ctx, code)
	if err != nil {
		return dto.AlertEvaluationInput{}, err
	}

	in := dto.AlertEvaluationInput{
		StockCode:            code,
		Currency:             dto.Currency(currency),
		TradingDate:          tradingDate,
		ClosePriceHundredths: closeHundredths,
		LatestTradingDate:    latest,
		Splits:               splits,
		TradesByUser:         map[uuid.UUID][]dto.TradeRow{},
		PreviousJudgments:    map[int64]dto.AlertJudgmentRow{},
	}
	if latest != tradingDate || splitAfter(splits, tradingDate) {
		return in, nil
	}

	in.Conditions, err = r.loadConditions(ctx, code)
	if err != nil {
		return dto.AlertEvaluationInput{}, err
	}
	if len(in.Conditions) == 0 {
		return in, nil
	}
	if in.TradesByUser, err = r.loadTrades(ctx, code, in.Conditions); err != nil {
		return dto.AlertEvaluationInput{}, err
	}
	if in.PreviousClose, err = r.loadPreviousClose(ctx, code, date); err != nil {
		return dto.AlertEvaluationInput{}, err
	}
	if in.Closes, err = r.loadCloses(ctx, code, date, in.Conditions, in.TradesByUser); err != nil {
		return dto.AlertEvaluationInput{}, err
	}
	if in.PreviousJudgments, err = r.loadPreviousJudgments(ctx, code, date, in.Conditions); err != nil {
		return dto.AlertEvaluationInput{}, err
	}
	return in, nil
}

// splitAfter は、splits に、date より後の分割の日があるかを返す。
func splitAfter(splits []dto.StockSplitRow, date string) bool {
	return slices.ContainsFunc(splits, func(s dto.StockSplitRow) bool { return s.SplitDate > date })
}

func (r *AlertRepository) loadSplits(ctx context.Context, code string) ([]dto.StockSplitRow, error) {
	rows, err := r.queries.ListStockSplitsOfCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("分割を読めません: %w", err)
	}
	splits := make([]dto.StockSplitRow, 0, len(rows))
	for _, d := range rows {
		splitDate, err := dateString(d.SplitDate)
		if err != nil {
			return nil, fmt.Errorf("分割の日: %w", err)
		}
		splits = append(splits, dto.StockSplitRow{Code: d.StockCode, SplitDate: splitDate, Ratio: int64(d.Ratio)})
	}
	return splits, nil
}

func (r *AlertRepository) loadConditions(ctx context.Context, code string) ([]dto.AlertConditionRow, error) {
	rows, err := r.queries.ListAlertConditionsOfStock(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("条件を読めません: %w", err)
	}
	conditions := make([]dto.AlertConditionRow, 0, len(rows))
	for _, d := range rows {
		conditions = append(conditions, dto.AlertConditionRow{
			ID: d.ID, UserID: uuid.UUID(d.UserID.Bytes), Kind: d.Kind, Scope: d.Scope, StockCode: d.StockCode,
			Direction: d.Direction, ThresholdHundredths: d.ThresholdHundredths, AddedOn: d.AddedOn,
		})
	}
	return conditions, nil
}

// loadTrades は、条件を持つ利用者ごとの、銘柄 code の売買履歴を返す。売買のない利用者は含めない。
func (r *AlertRepository) loadTrades(ctx context.Context, code string, conditions []dto.AlertConditionRow) (map[uuid.UUID][]dto.TradeRow, error) {
	users := map[uuid.UUID]bool{}
	for _, c := range conditions {
		users[c.UserID] = true
	}
	ids := make([]pgtype.UUID, 0, len(users))
	for u := range maps.Keys(users) {
		ids = append(ids, pgtype.UUID{Bytes: u, Valid: true})
	}
	rows, err := r.queries.ListTradesOfStockForUsers(ctx, sqlcgen.ListTradesOfStockForUsersParams{StockCode: code, UserIds: ids})
	if err != nil {
		return nil, fmt.Errorf("売買履歴を読めません: %w", err)
	}
	trades := map[uuid.UUID][]dto.TradeRow{}
	for _, d := range rows {
		tradeDate, err := dateString(d.TradeDate)
		if err != nil {
			return nil, fmt.Errorf("売買履歴（id %d）の取引日: %w", d.ID, err)
		}
		row := dto.TradeRow{
			ID: d.ID, Code: d.StockCode, Name: d.Name, Currency: dto.Currency(d.Currency), Side: dto.TradeSide(d.Side),
			Quantity: d.Quantity, PriceHundredths: d.PriceHundredths, TradeDate: tradeDate,
		}
		user := uuid.UUID(d.UserID.Bytes)
		trades[user] = append(trades[user], row)
	}
	return trades, nil
}

// loadPreviousClose は、D より前で最も新しい終値を返す。なければ nil。
func (r *AlertRepository) loadPreviousClose(ctx context.Context, code string, date time.Time) (*dto.ClosingPriceRow, error) {
	row, err := r.queries.GetPreviousClose(ctx, sqlcgen.GetPreviousCloseParams{StockCode: code, BeforeDate: pgtype.Date{Time: date, Valid: true}})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("前の終値を読めません: %w", err)
	}
	prevDate, err := dateString(row.TradingDate)
	if err != nil {
		return nil, fmt.Errorf("前の終値の取引日: %w", err)
	}
	return &dto.ClosingPriceRow{TradingDate: prevDate, ClosePriceHundredths: row.CloseHundredths}, nil
}

// loadCloses は、最高値の条件（model.AlertKindPeak）があるときだけ、D 以前の終値を取引日の昇順に返す。始まりは、最高値の条件を持つ利用者の最初の売買の日
// （今の保有が始まった日はそれ以後なので、それより前の終値は要らない）。最高値の条件がなければ、読まずに nil を返す。
func (r *AlertRepository) loadCloses(ctx context.Context, code string, date time.Time, conditions []dto.AlertConditionRow, trades map[uuid.UUID][]dto.TradeRow) ([]dto.ClosingPriceRow, error) {
	from := ""
	for _, c := range conditions {
		if c.Kind != string(model.AlertKindPeak) {
			continue
		}
		for _, t := range trades[c.UserID] {
			if from == "" || t.TradeDate < from {
				from = t.TradeDate
			}
		}
	}
	if from == "" {
		return nil, nil
	}
	fromDate, err := time.Parse(time.DateOnly, from)
	if err != nil {
		return nil, fmt.Errorf("終値の範囲の始まり %q を読めません: %w", from, err)
	}
	rows, err := r.queries.ListClosesBetween(ctx, sqlcgen.ListClosesBetweenParams{
		StockCode: code,
		FromDate:  pgtype.Date{Time: fromDate, Valid: true},
		ToDate:    pgtype.Date{Time: date, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("終値の列を読めません: %w", err)
	}
	closes := make([]dto.ClosingPriceRow, 0, len(rows))
	for _, d := range rows {
		tradingDate, err := dateString(d.TradingDate)
		if err != nil {
			return nil, fmt.Errorf("終値の取引日: %w", err)
		}
		closes = append(closes, dto.ClosingPriceRow{TradingDate: tradingDate, ClosePriceHundredths: d.CloseHundredths})
	}
	return closes, nil
}

// loadPreviousJudgments は、条件ごとの、D より前で最も新しい判定の結果を返す。
func (r *AlertRepository) loadPreviousJudgments(ctx context.Context, code string, date time.Time, conditions []dto.AlertConditionRow) (map[int64]dto.AlertJudgmentRow, error) {
	ids := make([]int64, 0, len(conditions))
	for _, c := range conditions {
		ids = append(ids, c.ID)
	}
	rows, err := r.queries.ListPreviousAlertJudgments(ctx, sqlcgen.ListPreviousAlertJudgmentsParams{
		StockCode: code, BeforeDate: pgtype.Date{Time: date, Valid: true}, ConditionIds: ids,
	})
	if err != nil {
		return nil, fmt.Errorf("前の判定を読めません: %w", err)
	}
	judgments := make(map[int64]dto.AlertJudgmentRow, len(rows))
	for _, d := range rows {
		tradingDate, err := dateString(d.TradingDate)
		if err != nil {
			return nil, fmt.Errorf("前の判定の取引日: %w", err)
		}
		judgments[d.ConditionID] = dto.AlertJudgmentRow{ConditionID: d.ConditionID, TradingDate: tradingDate, Satisfied: d.Satisfied}
	}
	return judgments, nil
}
