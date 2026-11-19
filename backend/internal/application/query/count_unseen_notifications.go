package query

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// CountUnseenNotifications は、ログイン中の利用者の、見ていない通知の数を返すユースケース（価格の変動の通知の要件25）。
type CountUnseenNotifications struct {
	reader NotificationsReader
}

// NewCountUnseenNotifications は、r から読む CountUnseenNotifications を返す。
func NewCountUnseenNotifications(r NotificationsReader) *CountUnseenNotifications {
	return &CountUnseenNotifications{reader: r}
}

// Execute は、利用者 userID の、見ていない通知の数を返す。他の利用者の通知は数えない（要件52）。
func (q *CountUnseenNotifications) Execute(ctx context.Context, userID uuid.UUID) (int64, error) {
	count, err := q.reader.CountUnseenNotifications(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("見ていない通知の数を読めません: %w", err)
	}
	return count, nil
}
