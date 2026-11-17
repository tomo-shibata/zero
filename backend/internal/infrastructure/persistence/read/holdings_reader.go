// Package read は、Query 側の読み取りの実装を持つ。sqlc の生成コードで SQL を実行し、application の dto を返す。
package read

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// ListStockSplits は、登録済みの分割をすべて返す（分割1件は1回だけ）。0件のときは空のスライスを返す。
// userID の売買履歴では絞らない。ListTradeRows とは別の読み取りなので、間に取り込みが入ると、売買履歴で絞った分割が
// 読んだ売買履歴と食い違い、一覧が誤りになるため。rows にない銘柄の分割は、CalculateHoldings が使わない。userID は使わない。
func (r *HoldingsReader) ListStockSplits(ctx context.Context, _ uuid.UUID) ([]dto.StockSplitRow, error) {
	dbRows, err := r.queries.ListAllStockSplits(ctx)
	if err != nil {
		return nil, fmt.Errorf("分割の問い合わせに失敗しました: %w", err)
	}
	splits := make([]dto.StockSplitRow, 0, len(dbRows))
	for _, d := range dbRows {
		splitDate, err := formatDate(d.SplitDate)
		if err != nil {
			return nil, fmt.Errorf("分割（銘柄 %q）の分割の日: %w", d.StockCode, err)
		}
		splits = append(splits, dto.StockSplitRow{Code: d.StockCode, SplitDate: splitDate, Ratio: int64(d.Ratio)})
	}
	return splits, nil
}

// ListHoldingOrder は、利用者 userID の保存した並び（銘柄コード）を、位置の順に返す。保存していなければ空のスライス。
func (r *HoldingsReader) ListHoldingOrder(ctx context.Context, userID uuid.UUID) ([]string, error) {
	codes, err := r.queries.ListHoldingOrder(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("保存した並びの問い合わせに失敗しました: %w", err)
	}
	if codes == nil {
		return []string{}, nil
	}
	return codes, nil
}

// LatestUsdJpyRate は、日付がいちばん新しいドル円のレートを返す（ドルを円にまとめる合計の要件14）。1件もなければ nil, nil。
// レートは 0.01円単位の整数にする（150.00円は 15000）。
func (r *HoldingsReader) LatestUsdJpyRate(ctx context.Context) (*dto.UsdJpyRate, error) {
	d, err := r.queries.GetLatestUsdJpyRate(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ドル円のレートの問い合わせに失敗しました: %w", err)
	}
	rateDate, err := formatDate(d.RateDate)
	if err != nil {
		return nil, fmt.Errorf("ドル円のレートの日付: %w", err)
	}
	hundredths, err := hundredthsFromNumeric(d.Rate)
	if err != nil {
		return nil, fmt.Errorf("ドル円のレート: %w", err)
	}
	return &dto.UsdJpyRate{RateDate: rateDate, RateHundredths: hundredths}, nil
}

// toTradeRow は、DB の1行を dto.TradeRow にする。価格は 0.01 単位の整数、取引日は "YYYY-MM-DD" にする。
func toTradeRow(d sqlcgen.ListTradeRowsRow) (dto.TradeRow, error) {
	row, err := newTradeRow(d.ID, d.StockCode, d.Name, d.Currency, d.Side, d.Quantity, d.Price, d.TradeDate)
	if err != nil {
		return dto.TradeRow{}, err
	}

	// 価格が一度も保存されていない銘柄は、終値と取引日がどちらも NULL（LEFT JOIN）。そのときは nil のまま返す。
	if !d.ClosePrice.Valid && !d.TradingDate.Valid {
		return row, nil
	}
	closePrice, err := hundredthsFromNumeric(d.ClosePrice)
	if err != nil {
		return dto.TradeRow{}, fmt.Errorf("終値: %w", err)
	}
	priceDate, err := formatDate(d.TradingDate)
	if err != nil {
		return dto.TradeRow{}, fmt.Errorf("終値の取引日: %w", err)
	}
	row.ClosePriceHundredths = &closePrice
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
