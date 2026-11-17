package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
)

// ErrInvalidOrder は、送られた並びが、保有している銘柄の集まりとちょうど同じでないこと（要件31）。API は 400 にする。
var ErrInvalidOrder = errors.New("invalid holding order")

// SaveHoldingOrder は、ログイン中の利用者の保有株式一覧の並びを保存するユースケース（要件23）。
type SaveHoldingOrder struct {
	repo repository.HoldingOrderRepository
}

// NewSaveHoldingOrder は、repo で並びを保存する SaveHoldingOrder を返す。
func NewSaveHoldingOrder(repo repository.HoldingOrderRepository) *SaveHoldingOrder {
	return &SaveHoldingOrder{repo: repo}
}

// Execute は、利用者 userID の並びを codes にする。codes が、その利用者の保有している銘柄（保有数量が1以上）と
// ちょうど同じ集まりでなければ、何も変えずに ErrInvalidOrder を返す（要件31）。
func (c *SaveHoldingOrder) Execute(ctx context.Context, userID uuid.UUID, codes []string) error {
	err := c.repo.ReplaceHoldingOrder(ctx, userID, func(heldCodes []string) (model.HoldingOrder, error) {
		return model.NewHoldingOrder(codes, heldCodes)
	})
	if err != nil {
		if errors.Is(err, model.ErrInvalidHoldingOrder) {
			return ErrInvalidOrder
		}
		return fmt.Errorf("並びを保存できません: %w", err)
	}
	return nil
}
