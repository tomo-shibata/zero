package query

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// ListNotifications は、ログイン中の利用者の通知のリストを返すユースケース（価格の変動の通知の要件30）。
type ListNotifications struct {
	reader NotificationsReader
}

// NewListNotifications は、r から読む ListNotifications を返す。
func NewListNotifications(r NotificationsReader) *ListNotifications {
	return &ListNotifications{reader: r}
}

// Execute は、利用者 userID の通知を、取引日の新しい順に100件まで返す。他の利用者の通知は含めない（要件52）。
// 一覧と LatestID は同じスナップショットから読む（一覧を読んだ後にできた通知の ID が LatestID に入り、見たにされてしまうのを防ぐ。要件34）。
func (q *ListNotifications) Execute(ctx context.Context, userID uuid.UUID) (dto.NotificationList, error) {
	list, err := q.reader.ListNotifications(ctx, userID)
	if err != nil {
		return dto.NotificationList{}, fmt.Errorf("通知を読めません: %w", err)
	}
	return list, nil
}
