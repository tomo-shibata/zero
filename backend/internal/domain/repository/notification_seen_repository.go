package repository

import (
	"context"

	"github.com/google/uuid"
)

// NotificationSeenRepository は、通知を見たにする書き込み（価格の変動の通知の要件34）。
type NotificationSeenRepository interface {
	// MarkSeenUpTo は、利用者 userID の、ID が upToID 以下で見ていない通知を、見たにする。ほかの利用者の通知は変えない。
	MarkSeenUpTo(ctx context.Context, userID uuid.UUID, upToID int64) error
}
