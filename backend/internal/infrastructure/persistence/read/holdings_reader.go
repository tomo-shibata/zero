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

// HoldingsReader は、PostgreSQL から保有データを読む query.HoldingsReader の実装。
type HoldingsReader struct {
	queries *sqlcgen.Queries
}

// HoldingsReader が query.HoldingsReader を満たすことを、コンパイル時に確かめる。
var _ query.HoldingsReader = (*HoldingsReader)(nil)

// NewHoldingsReader は、pool の DB から保有データを読む HoldingsReader を返す。
func NewHoldingsReader(pool *pgxpool.Pool) *HoldingsReader {
	return &HoldingsReader{queries: sqlcgen.New(pool)}
}

// ListHoldingRows は、利用者 userID の保有データを、銘柄名と最新の終値・取引日を付けて1件ずつ返す。
// 0件のときは空のスライスを返す。
func (r *HoldingsReader) ListHoldingRows(ctx context.Context, userID uuid.UUID) ([]dto.HoldingRow, error) {
	dbRows, err := r.queries.ListHoldingRows(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("保有データの問い合わせに失敗しました: %w", err)
	}
	rows := make([]dto.HoldingRow, 0, len(dbRows))
	for _, dbRow := range dbRows {
		row, err := toHoldingRow(dbRow)
		if err != nil {
			return nil, fmt.Errorf("保有データ（銘柄 %s）を読めません: %w", dbRow.StockCode, err)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// toHoldingRow は、DB の1行を dto.HoldingRow にする。単価は 0.1円単位の整数、取引日は "YYYY-MM-DD" にする。
func toHoldingRow(d sqlcgen.ListHoldingRowsRow) (dto.HoldingRow, error) {
	acquisition, err := tenthsFromNumeric(d.AcquisitionPrice)
	if err != nil {
		return dto.HoldingRow{}, fmt.Errorf("取得価格: %w", err)
	}
	row := dto.HoldingRow{
		Code:                   d.StockCode,
		Name:                   d.Name,
		Quantity:               d.Quantity,
		AcquisitionPriceTenths: acquisition,
	}

	// 価格が一度も保存されていない銘柄は、終値と取引日がどちらも NULL（LEFT JOIN）。そのときは nil のまま返す。
	if !d.ClosePrice.Valid && !d.TradingDate.Valid {
		return row, nil
	}
	closePrice, err := tenthsFromNumeric(d.ClosePrice)
	if err != nil {
		return dto.HoldingRow{}, fmt.Errorf("終値: %w", err)
	}
	priceDate, err := formatDate(d.TradingDate)
	if err != nil {
		return dto.HoldingRow{}, fmt.Errorf("終値の取引日: %w", err)
	}
	row.ClosePriceTenths = &closePrice
	row.PriceDate = &priceDate
	return row, nil
}

// formatDate は、DATE の値を "YYYY-MM-DD" にする。
// pgx は DATE を UTC の0時の time.Time で返すので、そのまま書式にすれば、サーバーのタイムゾーンで日付がずれない。
func formatDate(d pgtype.Date) (string, error) {
	if !d.Valid {
		return "", errors.New("取引日が NULL です")
	}
	if d.InfinityModifier != pgtype.Finite {
		return "", errors.New("取引日が無限大です")
	}
	return d.Time.Format(time.DateOnly), nil
}
