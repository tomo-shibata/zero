package write

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/sqlcgen"
)

// ClosingPriceImportRepository は、PostgreSQL への価格の CSV の登録（repository.ClosingPriceImportRepository）の実装。
type ClosingPriceImportRepository struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
}

// ClosingPriceImportRepository が repository.ClosingPriceImportRepository を満たすことを、コンパイル時に確かめる。
var _ repository.ClosingPriceImportRepository = (*ClosingPriceImportRepository)(nil)

// NewClosingPriceImportRepository は、pool の DB に終値を登録する ClosingPriceImportRepository を返す。
func NewClosingPriceImportRepository(pool *pgxpool.Pool) *ClosingPriceImportRepository {
	return &ClosingPriceImportRepository{pool: pool, queries: sqlcgen.New(pool)}
}

// FindCurrencies は、codes のうち銘柄マスタにある銘柄の通貨を、銘柄コードをキーにして返す。なければ空のマップを返す。
func (r *ClosingPriceImportRepository) FindCurrencies(ctx context.Context, codes []string) (map[string]model.Currency, error) {
	rows, err := r.queries.ListStocksByCodes(ctx, codes)
	if err != nil {
		return nil, fmt.Errorf("銘柄マスタの問い合わせに失敗しました: %w", err)
	}
	currencies := make(map[string]model.Currency, len(rows))
	for _, row := range rows {
		currencies[row.Code] = model.Currency(row.Currency)
	}
	return currencies, nil
}

// SaveAll は、1つのトランザクションで prices を保存する。同じ銘柄・同じ取引日の終値があれば、終値と取得日時を上書きする
// （18:00 の取得と同じ SQL。ClosingPriceRepository.Save）。途中で失敗したら、コミットせずに元に戻す（何も変わらない）。
func (r *ClosingPriceImportRepository) SaveAll(ctx context.Context, prices []model.ClosingPrice) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("トランザクションを始められません: %w", err)
	}
	// Rollback と Commit には ctx の取り消しを引き継がない（TradeImportRepository.ReplaceTrades と同じ理由）。
	finishCtx := context.WithoutCancel(ctx)
	defer func() { _ = tx.Rollback(finishCtx) }()

	q := r.queries.WithTx(tx)
	for _, p := range prices {
		if err := upsertClosingPrice(ctx, q, p); err != nil {
			return err
		}
	}
	if err := tx.Commit(finishCtx); err != nil {
		return fmt.Errorf("コミットに失敗しました: %w", err)
	}
	return nil
}
