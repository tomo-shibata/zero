// Package write は、Command 側の書き込みの実装（domain/repository の IF の実装）を持つ。sqlc の生成コードで SQL を実行する。
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

// HoldingRepository は、PostgreSQL の保有データを扱う repository.HoldingRepository と repository.HeldStockRepository の実装。
type HoldingRepository struct {
	queries *sqlcgen.Queries
}

// HoldingRepository が repository.HoldingRepository と repository.HeldStockRepository を満たすことを、コンパイル時に確かめる。
var (
	_ repository.HoldingRepository   = (*HoldingRepository)(nil)
	_ repository.HeldStockRepository = (*HoldingRepository)(nil)
)

// NewHoldingRepository は、pool の DB の保有データを扱う HoldingRepository を返す。
func NewHoldingRepository(pool *pgxpool.Pool) *HoldingRepository {
	return &HoldingRepository{queries: sqlcgen.New(pool)}
}

// ListHeldStockCodes は、全利用者の保有データにある銘柄コードを、重複なし・昇順（Go の文字列比較と同じバイト順）で返す。
// まとめと並べ替えは SQL（db/query/holdings.sql の ListHeldStockCodes）で行う。
func (r *HoldingRepository) ListHeldStockCodes(ctx context.Context) ([]string, error) {
	codes, err := r.queries.ListHeldStockCodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("保有されている銘柄コードの問い合わせに失敗しました: %w", err)
	}
	return codes, nil
}

// ListHeldStockCodesOf は、利用者 userID の保有データにある銘柄コードを、重複なし・昇順（COLLATE "C"）で返す。
// まとめと並べ替えは SQL（db/query/holdings.sql の ListHeldStockCodesOf）で行う。0件なら nil ではなく空のスライスを返す。
func (r *HoldingRepository) ListHeldStockCodesOf(ctx context.Context, userID uuid.UUID) ([]string, error) {
	codes, err := r.queries.ListHeldStockCodesOf(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("利用者の保有している銘柄コードの問い合わせに失敗しました: %w", err)
	}
	if codes == nil {
		// sqlc は0件のとき nil を返す。IF の約束（空のスライス）に合わせる。
		codes = []string{}
	}
	return codes, nil
}
