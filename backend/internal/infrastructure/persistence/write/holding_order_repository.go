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

// HoldingOrderRepository は、PostgreSQL への並びの保存（repository.HoldingOrderRepository）の実装。
type HoldingOrderRepository struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
}

// HoldingOrderRepository が repository.HoldingOrderRepository を満たすことを、コンパイル時に確かめる。
var _ repository.HoldingOrderRepository = (*HoldingOrderRepository)(nil)

// NewHoldingOrderRepository は、pool の DB に並びを保存する HoldingOrderRepository を返す。
func NewHoldingOrderRepository(pool *pgxpool.Pool) *HoldingOrderRepository {
	return &HoldingOrderRepository{pool: pool, queries: sqlcgen.New(pool)}
}

// ReplaceHoldingOrder は、1つのトランザクションで、利用者の行をロックし、保有数量が1以上の銘柄コードを読み、
// build が返した並びで、利用者の並びを入れ替える。途中で失敗したら、コミットせずに元に戻す（何も変わらない）。
func (r *HoldingOrderRepository) ReplaceHoldingOrder(ctx context.Context, userID uuid.UUID, build func(heldCodes []string) (model.HoldingOrder, error)) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("トランザクションを始められません: %w", err)
	}
	// 取り込みと同じく、Rollback と Commit には ctx の取り消しを引き継がない（コミットされたか分からないまま返さないため）。
	finishCtx := context.WithoutCancel(ctx)
	defer func() { _ = tx.Rollback(finishCtx) }()

	q := r.queries.WithTx(tx)
	owner := pgtype.UUID{Bytes: userID, Valid: true}
	// 銘柄の削除・取り込みと同時に動いても、消した銘柄の行が残らないよう、利用者の行をロックして順番に動かす（判断6）。
	if err := q.LockUser(ctx, owner); err != nil {
		return fmt.Errorf("利用者の行のロックに失敗しました: %w", err)
	}
	held, err := q.ListHeldStocksOf(ctx, owner)
	if err != nil {
		return fmt.Errorf("保有している銘柄の問い合わせに失敗しました: %w", err)
	}
	heldCodes := make([]string, 0, len(held))
	for _, h := range held {
		heldCodes = append(heldCodes, h.StockCode)
	}
	order, err := build(heldCodes)
	if err != nil {
		return fmt.Errorf("並びを決められません: %w", err)
	}
	if err := q.DeleteHoldingOrdersOf(ctx, owner); err != nil {
		return fmt.Errorf("並びの削除に失敗しました: %w", err)
	}
	if err := q.InsertHoldingOrders(ctx, sqlcgen.InsertHoldingOrdersParams{UserID: owner, Codes: order.Codes()}); err != nil {
		return fmt.Errorf("並びの登録に失敗しました: %w", err)
	}
	if err := tx.Commit(finishCtx); err != nil {
		return fmt.Errorf("コミットに失敗しました: %w", err)
	}
	return nil
}
