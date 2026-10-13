package write

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/sqlcgen"
)

// HoldingEditRepository は、利用者の1銘柄の保有データを書き換える repository.HoldingDeleteRepository の実装。
// 編集の保存（repository.HoldingEditRepository）は、あとの PR で同じ型に足す（値段の更新のプラン 4.3）。
type HoldingEditRepository struct {
	queries *sqlcgen.Queries
}

// HoldingEditRepository が repository.HoldingDeleteRepository を満たすことを、コンパイル時に確かめる。
var _ repository.HoldingDeleteRepository = (*HoldingEditRepository)(nil)

// NewHoldingEditRepository は、pool の DB の保有データを書き換える HoldingEditRepository を返す。
func NewHoldingEditRepository(pool *pgxpool.Pool) *HoldingEditRepository {
	return &HoldingEditRepository{queries: sqlcgen.New(pool)}
}

// DeleteAll は、利用者 userID の銘柄 stockCode の保有データをすべて消す。1件も消えなければ repository.ErrHoldingNotFound を返す。
// 消す条件は1つの DELETE（db/query/holdings.sql の DeleteHoldingsOf）で決まるので、読んでから消す間に別の要求が割り込んでも、
// 消えた件数が本当に消した件数になる。
func (r *HoldingEditRepository) DeleteAll(ctx context.Context, userID uuid.UUID, stockCode string) error {
	deleted, err := r.queries.DeleteHoldingsOf(ctx, sqlcgen.DeleteHoldingsOfParams{
		UserID:    pgtype.UUID{Bytes: userID, Valid: true},
		StockCode: stockCode,
	})
	if err != nil {
		return fmt.Errorf("保有データの削除に失敗しました: %w", err)
	}
	if deleted == 0 {
		return repository.ErrHoldingNotFound
	}
	return nil
}
