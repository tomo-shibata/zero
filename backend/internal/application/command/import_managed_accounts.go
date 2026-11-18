package command

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
)

// ImportManagedAccounts は、おまかせ運用の CSV を、利用者の口座と記録として取り込むユースケース（要件25〜要件31）。
type ImportManagedAccounts struct {
	repo repository.ManagedAccountImportRepository
}

// NewImportManagedAccounts は、repo で取り込む ImportManagedAccounts を返す。
func NewImportManagedAccounts(repo repository.ManagedAccountImportRepository) *ImportManagedAccounts {
	return &ImportManagedAccounts{repo: repo}
}

// CheckUser は、利用者 userID が DB にいるかを確かめる。いなければ ErrUserNotFound を返す。
func (u *ImportManagedAccounts) CheckUser(ctx context.Context, userID uuid.UUID) error {
	exists, err := u.repo.UserExists(ctx, userID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrUserNotFound
	}
	return nil
}

// Execute は、利用者 userID の口座と記録を valuations（CSV の行の順）で入れ替え、入れた記録の件数を返す（要件26）。
// valuations が空のときはエラーにして何も変えない（利用者の口座と記録を全部消すだけの取り込みにしないため。要件31 の「取り込む行がありません」）。
// 同じ名前・同じ日付の重なりは、呼び出し側が確かめてから渡す（重なっていれば DB の主キーで失敗し、何も変わらない）。
func (u *ImportManagedAccounts) Execute(ctx context.Context, userID uuid.UUID, valuations []model.ManagedValuation) (int, error) {
	if len(valuations) == 0 {
		return 0, errors.New("取り込む記録がありません")
	}
	if err := u.repo.ReplaceManagedAccounts(ctx, userID, valuations); err != nil {
		return 0, err
	}
	return len(valuations), nil
}
