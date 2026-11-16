package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
)

// DeleteHolding は、ログイン中の利用者の1銘柄の売買履歴をすべて削除するユースケース（要件18）。
type DeleteHolding struct {
	repo repository.HoldingDeleteRepository
}

// NewDeleteHolding は、repo で売買履歴を消す DeleteHolding を返す。
func NewDeleteHolding(repo repository.HoldingDeleteRepository) *DeleteHolding {
	return &DeleteHolding{repo: repo}
}

// Execute は、利用者 userID の銘柄 stockCode の売買履歴をすべて削除する。
// その銘柄の売買履歴が1件もない（銘柄マスタにない銘柄を含む）ときは、何も消さずに ErrNotFound を返す（要件18）。
func (c *DeleteHolding) Execute(ctx context.Context, userID uuid.UUID, stockCode string) error {
	if err := c.repo.DeleteAll(ctx, userID, stockCode); err != nil {
		if errors.Is(err, repository.ErrHoldingNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("売買履歴を削除できません: %w", err)
	}
	return nil
}
