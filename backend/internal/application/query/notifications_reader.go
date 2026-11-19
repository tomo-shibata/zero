package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// NotificationsReader は、通知のリストと見ていない数が使う、通知の読み取り。実装は infrastructure（persistence/read）に置き、
// Query 側はこの IF だけに依存する（依存の向きを内側にするため）。
type NotificationsReader interface {
	// ListNotifications は、利用者 userID の通知を、取引日の新しい順（同じ取引日は銘柄コードの昇順、全銘柄の条件が先、条件 ID の昇順）に、
	// 100件まで返す。LatestID は、その利用者の通知の最大の ID（100件より古いものも含む。0件なら nil）。一覧と LatestID は同じスナップショットから読む。
	ListNotifications(ctx context.Context, userID uuid.UUID) (dto.NotificationList, error)
	// CountUnseenNotifications は、利用者 userID の、見ていない通知の数を返す。
	CountUnseenNotifications(ctx context.Context, userID uuid.UUID) (int64, error)
}
