// Package write は、Command 側の書き込みの実装（domain/repository の IF の実装）を持つ。sqlc の生成コードで SQL を実行する。
package write

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/sqlcgen"
)

// HoldingRepository は、PostgreSQL の保有データを扱う repository.HoldingRepository の実装。
type HoldingRepository struct {
	queries *sqlcgen.Queries
}

// HoldingRepository が repository.HoldingRepository を満たすことを、コンパイル時に確かめる。
var _ repository.HoldingRepository = (*HoldingRepository)(nil)

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
