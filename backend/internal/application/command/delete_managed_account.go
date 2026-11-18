package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
)

// DeleteManagedAccount は、ログイン中の利用者のおまかせ運用の口座を、記録ごと削除するユースケース（おまかせ運用の要件24）。
type DeleteManagedAccount struct {
	repo repository.ManagedAccountDeleteRepository
}

// NewDeleteManagedAccount は、repo で口座を消す DeleteManagedAccount を返す。
func NewDeleteManagedAccount(repo repository.ManagedAccountDeleteRepository) *DeleteManagedAccount {
	return &DeleteManagedAccount{repo: repo}
}

// Execute は、利用者 userID の名前 name の口座を、記録ごと削除する。
// その名前の口座がなければ、何も消さずに ErrNotFound を返す（要件24）。
func (c *DeleteManagedAccount) Execute(ctx context.Context, userID uuid.UUID, name string) error {
	if err := c.repo.DeleteManagedAccount(ctx, userID, name); err != nil {
		if errors.Is(err, repository.ErrManagedAccountNotFound) {
			return ErrNotFound
		}
		// 名前は利用者の入力なので、エラー（ログに出る）には入れない。
		return fmt.Errorf("口座を削除できません: %w", err)
	}
	return nil
}
