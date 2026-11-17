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

// HoldingEditRepository は、利用者の1銘柄の売買履歴を書き換える repository.HoldingDeleteRepository の実装。
// 編集の保存は保留にした（値段の更新の要件3〜12。売買履歴の要件定義の「既存の要件の置き換え」）。
type HoldingEditRepository struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
}

// HoldingEditRepository が repository.HoldingDeleteRepository を満たすことを、コンパイル時に確かめる。
var _ repository.HoldingDeleteRepository = (*HoldingEditRepository)(nil)

// NewHoldingEditRepository は、pool の DB の売買履歴を書き換える HoldingEditRepository を返す。
func NewHoldingEditRepository(pool *pgxpool.Pool) *HoldingEditRepository {
	return &HoldingEditRepository{pool: pool, queries: sqlcgen.New(pool)}
}

// DeleteAll は、利用者 userID の銘柄 stockCode の売買履歴をすべて消し、その銘柄の並びの行も消す（カード表示の要件28。プラン 4.3「消すとき」）。
// 1つのトランザクションで、最初に利用者の行をロックする（並びの保存とぶつからないよう、判断6）。
// 1件も消えなければ repository.ErrHoldingNotFound を返す（何も変えない）。
// 消す条件は1つの DELETE（db/query/trades.sql の DeleteTradesOf）で決まるので、消えた件数が本当に消した件数になる。
func (r *HoldingEditRepository) DeleteAll(ctx context.Context, userID uuid.UUID, stockCode string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("トランザクションを始められません: %w", err)
	}
	// 取り込みと同じく、Rollback と Commit には ctx の取り消しを引き継がない（コミットされたか分からないまま返さないため）。
	finishCtx := context.WithoutCancel(ctx)
	defer func() { _ = tx.Rollback(finishCtx) }()

	q := r.queries.WithTx(tx)
	owner := pgtype.UUID{Bytes: userID, Valid: true}
	if err := q.LockUser(ctx, owner); err != nil {
		return fmt.Errorf("利用者の行のロックに失敗しました: %w", err)
	}
	deleted, err := q.DeleteTradesOf(ctx, sqlcgen.DeleteTradesOfParams{UserID: owner, StockCode: stockCode})
	if err != nil {
		return fmt.Errorf("売買履歴の削除に失敗しました: %w", err)
	}
	if deleted == 0 {
		return repository.ErrHoldingNotFound
	}
	if err := q.DeleteHoldingOrderOf(ctx, sqlcgen.DeleteHoldingOrderOfParams{UserID: owner, StockCode: stockCode}); err != nil {
		return fmt.Errorf("並びの行の削除に失敗しました: %w", err)
	}
	if err := tx.Commit(finishCtx); err != nil {
		return fmt.Errorf("コミットに失敗しました: %w", err)
	}
	return nil
}
