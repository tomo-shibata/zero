// Package read は、Query 側の読み取りの実装を持つ。sqlc の生成コードで SQL を実行し、application の dto を返す。
package read

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/sqlcgen"
)

// HoldingsReader は、PostgreSQL から売買履歴を読む query.HoldingsReader の実装。
type HoldingsReader struct {
	queries *sqlcgen.Queries
}

// HoldingsReader が query.HoldingsReader を満たすことを、コンパイル時に確かめる。
var _ query.HoldingsReader = (*HoldingsReader)(nil)

// NewHoldingsReader は、pool の DB から売買履歴を読む HoldingsReader を返す。
func NewHoldingsReader(pool *pgxpool.Pool) *HoldingsReader {
	return &HoldingsReader{queries: sqlcgen.New(pool)}
}

// ListTradeRows は、利用者 userID の売買履歴を、銘柄名と最新の終値・取引日を付けて1件ずつ返す。
// 0件のときは空のスライスを返す。
func (r *HoldingsReader) ListTradeRows(ctx context.Context, userID uuid.UUID) ([]dto.TradeRow, error) {
	dbRows, err := r.queries.ListTradeRows(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("売買履歴の問い合わせに失敗しました: %w", err)
	}
	rows := make([]dto.TradeRow, 0, len(dbRows))
	for _, dbRow := range dbRows {
		row, err := toTradeRow(dbRow)
		if err != nil {
			return nil, fmt.Errorf("売買履歴（id %d・銘柄 %q）を読めません: %w", dbRow.ID, dbRow.StockCode, err)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// toTradeRow は、DB の1行を dto.TradeRow にする。価格は 0.1円単位の整数、取引日は "YYYY-MM-DD" にする。
func toTradeRow(d sqlcgen.ListTradeRowsRow) (dto.TradeRow, error) {
	side := dto.TradeSide(d.Side)
	if side != dto.TradeSideBuy && side != dto.TradeSideSell {
		return dto.TradeRow{}, fmt.Errorf("知らない売買の種類 %q です", d.Side)
	}
	price, err := tenthsFromNumeric(d.Price)
	if err != nil {
		return dto.TradeRow{}, fmt.Errorf("価格: %w", err)
	}
	tradeDate, err := formatDate(d.TradeDate)
	if err != nil {
		return dto.TradeRow{}, fmt.Errorf("取引日: %w", err)
	}
	row := dto.TradeRow{
		ID:          d.ID,
		Code:        d.StockCode,
		Name:        d.Name,
		Side:        side,
		Quantity:    d.Quantity,
		PriceTenths: price,
		TradeDate:   tradeDate,
	}

	// 価格が一度も保存されていない銘柄は、終値と取引日がどちらも NULL（LEFT JOIN）。そのときは nil のまま返す。
	if !d.ClosePrice.Valid && !d.TradingDate.Valid {
		return row, nil
	}
	closePrice, err := tenthsFromNumeric(d.ClosePrice)
	if err != nil {
		return dto.TradeRow{}, fmt.Errorf("終値: %w", err)
	}
	priceDate, err := formatDate(d.TradingDate)
	if err != nil {
		return dto.TradeRow{}, fmt.Errorf("終値の取引日: %w", err)
	}
	row.ClosePriceTenths = &closePrice
	row.PriceDate = &priceDate
	return row, nil
}

// formatDate は、DATE の値を "YYYY-MM-DD" にする。
// pgx は DATE を UTC の0時の time.Time で返すので、そのまま書式にすれば、サーバーのタイムゾーンで日付がずれない。
func formatDate(d pgtype.Date) (string, error) {
	if !d.Valid {
		return "", errors.New("日付が NULL です")
	}
	if d.InfinityModifier != pgtype.Finite {
		return "", errors.New("日付が無限大です")
	}
	return d.Time.Format(time.DateOnly), nil
}
