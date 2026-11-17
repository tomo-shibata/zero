package read

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/sqlcgen"
)

// StockHistoryReader は、PostgreSQL から銘柄ごとの売買履歴の画面の元になる値を読む query.StockHistoryReader の実装。
type StockHistoryReader struct {
	queries *sqlcgen.Queries
}

// StockHistoryReader が query.StockHistoryReader を満たすことを、コンパイル時に確かめる。
var _ query.StockHistoryReader = (*StockHistoryReader)(nil)

// NewStockHistoryReader は、pool の DB から読む StockHistoryReader を返す。
func NewStockHistoryReader(pool *pgxpool.Pool) *StockHistoryReader {
	return &StockHistoryReader{queries: sqlcgen.New(pool)}
}

// ListTradesOfStock は、利用者 userID の、銘柄 code の売買履歴を、銘柄名・通貨を付けて返す。0件のときは空のスライスを返す。
func (r *StockHistoryReader) ListTradesOfStock(ctx context.Context, userID uuid.UUID, code string) ([]dto.TradeRow, error) {
	dbRows, err := r.queries.ListTradesOfStock(ctx, sqlcgen.ListTradesOfStockParams{
		UserID:    pgtype.UUID{Bytes: userID, Valid: true},
		StockCode: code,
	})
	if err != nil {
		return nil, fmt.Errorf("銘柄の売買履歴の問い合わせに失敗しました: %w", err)
	}
	rows := make([]dto.TradeRow, 0, len(dbRows))
	for _, d := range dbRows {
		row, err := newTradeRow(d.ID, d.StockCode, d.Name, d.Currency, d.Side, d.Quantity, d.Price, d.TradeDate)
		if err != nil {
			return nil, fmt.Errorf("売買履歴（id %d）を読めません: %w", d.ID, err)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// ListStockSplitsOfStock は、銘柄 code の分割を返す。0件のときは空のスライスを返す。
func (r *StockHistoryReader) ListStockSplitsOfStock(ctx context.Context, code string) ([]dto.StockSplitRow, error) {
	dbRows, err := r.queries.ListStockSplitsOfCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("銘柄の分割の問い合わせに失敗しました: %w", err)
	}
	splits := make([]dto.StockSplitRow, 0, len(dbRows))
	for _, d := range dbRows {
		splitDate, err := formatDate(d.SplitDate)
		if err != nil {
			return nil, fmt.Errorf("分割の日: %w", err)
		}
		splits = append(splits, dto.StockSplitRow{Code: d.StockCode, SplitDate: splitDate, Ratio: int64(d.Ratio)})
	}
	return splits, nil
}

// ListClosingPricesSince は、銘柄 code の、取引日が from（"YYYY-MM-DD"）と同じかそれより後の終値を、取引日の昇順に返す。
// 0件のときは空のスライスを返す。
func (r *StockHistoryReader) ListClosingPricesSince(ctx context.Context, code, from string) ([]dto.ClosingPriceRow, error) {
	fromDate, err := time.Parse(time.DateOnly, from)
	if err != nil {
		return nil, fmt.Errorf("終値の範囲の始まりの日付を読めません: %w", err)
	}
	dbRows, err := r.queries.ListClosingPricesSince(ctx, sqlcgen.ListClosingPricesSinceParams{
		StockCode: code,
		FromDate:  pgtype.Date{Time: fromDate, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("銘柄の終値の問い合わせに失敗しました: %w", err)
	}
	closes := make([]dto.ClosingPriceRow, 0, len(dbRows))
	for _, d := range dbRows {
		tradingDate, err := formatDate(d.TradingDate)
		if err != nil {
			return nil, fmt.Errorf("終値の取引日: %w", err)
		}
		closePrice, err := hundredthsFromNumeric(d.ClosePrice)
		if err != nil {
			return nil, fmt.Errorf("終値（%s）: %w", tradingDate, err)
		}
		closes = append(closes, dto.ClosingPriceRow{TradingDate: tradingDate, ClosePriceHundredths: closePrice})
	}
	return closes, nil
}
