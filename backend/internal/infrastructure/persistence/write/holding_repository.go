// Package write は、Command 側の書き込みの実装（domain/repository の IF の実装）を持つ。sqlc の生成コードで SQL を実行する。
package write

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/sqlcgen"
)

// HoldingRepository は、PostgreSQL の売買履歴から保有している銘柄を通貨と一緒に読む repository.HoldingRepository と repository.HeldStockRepository の実装。
type HoldingRepository struct {
	queries *sqlcgen.Queries
}

// HoldingRepository が repository.HoldingRepository と repository.HeldStockRepository を満たすことを、コンパイル時に確かめる。
var (
	_ repository.HoldingRepository   = (*HoldingRepository)(nil)
	_ repository.HeldStockRepository = (*HoldingRepository)(nil)
)

// NewHoldingRepository は、pool の DB の売買履歴から保有している銘柄を読む HoldingRepository を返す。
func NewHoldingRepository(pool *pgxpool.Pool) *HoldingRepository {
	return &HoldingRepository{queries: sqlcgen.New(pool)}
}

// ListHeldStocks は、誰かの保有数量が1以上の銘柄を、通貨と一緒に、重複なし・銘柄コードの昇順（Go の文字列比較と同じバイト順）で返す。
// まとめと並べ替えは SQL（db/query/trades.sql の ListHeldStocks）で行う。
func (r *HoldingRepository) ListHeldStocks(ctx context.Context) ([]repository.HeldStock, error) {
	rows, err := r.queries.ListHeldStocks(ctx)
	if err != nil {
		return nil, fmt.Errorf("保有されている銘柄の問い合わせに失敗しました: %w", err)
	}
	stocks := make([]repository.HeldStock, 0, len(rows))
	for _, row := range rows {
		stocks = append(stocks, repository.HeldStock{Code: row.StockCode, Currency: model.Currency(row.Currency)})
	}
	return stocks, nil
}

// ListHeldStocksOf は、利用者 userID の保有数量が1以上の銘柄を、通貨と一緒に、重複なし・銘柄コードの昇順（COLLATE "C"）で返す。
// まとめと並べ替えは SQL（db/query/trades.sql の ListHeldStocksOf）で行う。0件なら nil ではなく空のスライスを返す。
func (r *HoldingRepository) ListHeldStocksOf(ctx context.Context, userID uuid.UUID) ([]repository.HeldStock, error) {
	rows, err := r.queries.ListHeldStocksOf(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("利用者の保有している銘柄の問い合わせに失敗しました: %w", err)
	}
	// sqlc は0件のとき nil を返す。IF の約束（空のスライス）に合わせるため、長さ0のスライスから始める。
	stocks := make([]repository.HeldStock, 0, len(rows))
	for _, row := range rows {
		stocks = append(stocks, repository.HeldStock{Code: row.StockCode, Currency: model.Currency(row.Currency)})
	}
	return stocks, nil
}
