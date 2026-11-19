package command

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
)

// MarkNotificationsSeen は、ログイン中の利用者の通知を、取得した時点までのものを見たにするユースケース（価格の変動の通知の要件34）。
type MarkNotificationsSeen struct {
	repo repository.NotificationSeenRepository
}

// NewMarkNotificationsSeen は、repo で見たにする MarkNotificationsSeen を返す。
func NewMarkNotificationsSeen(repo repository.NotificationSeenRepository) *MarkNotificationsSeen {
	return &MarkNotificationsSeen{repo: repo}
}

// Execute は、利用者 userID の、ID が upToID 以下で見ていない通知を、見たにする。
// 取得の後に作られた通知（ID が upToID より大きい）と、ほかの利用者の通知は変えない（要件34・要件52）。
func (c *MarkNotificationsSeen) Execute(ctx context.Context, userID uuid.UUID, upToID int64) error {
	if err := c.repo.MarkSeenUpTo(ctx, userID, upToID); err != nil {
		return fmt.Errorf("通知を見たにできません: %w", err)
	}
	return nil
}
