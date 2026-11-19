package read

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/sqlcgen"
)

// NotificationsReader は、PostgreSQL から通知のリストと見ていない数を読む query.NotificationsReader の実装。
type NotificationsReader struct {
	queries *sqlcgen.Queries
}

// NotificationsReader が query.NotificationsReader を満たすことを、コンパイル時に確かめる。
var _ query.NotificationsReader = (*NotificationsReader)(nil)

// NewNotificationsReader は、pool の DB から読む NotificationsReader を返す。
func NewNotificationsReader(pool *pgxpool.Pool) *NotificationsReader {
	return &NotificationsReader{queries: sqlcgen.New(pool)}
}

// ListNotifications は、利用者 userID の通知を、100件まで返す。0件のときは Items が空のスライスで、LatestID は nil。
// 一覧と LatestID は、1つの SQL（同じスナップショット）で読む。
func (r *NotificationsReader) ListNotifications(ctx context.Context, userID uuid.UUID) (dto.NotificationList, error) {
	dbRows, err := r.queries.ListNotifications(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		return dto.NotificationList{}, fmt.Errorf("通知の問い合わせに失敗しました: %w", err)
	}
	items := make([]dto.NotificationItem, 0, len(dbRows))
	for _, d := range dbRows {
		var details dto.AlertNotificationDetails
		if err := json.Unmarshal(d.Details, &details); err != nil {
			return dto.NotificationList{}, fmt.Errorf("通知（id %d）の details を読めません: %w", d.ID, err)
		}
		items = append(items, dto.NotificationItem{
			ID:          d.ID,
			StockCode:   d.StockCode,
			StockName:   d.StockName,
			Currency:    dto.Currency(d.Currency),
			TradingDate: d.TradingDate,
			IsNew:       d.IsNew,
			Condition: dto.NotificationCondition{
				Kind:                d.Kind,
				Scope:               d.Scope,
				StockCode:           d.ConditionStockCode,
				Direction:           d.Direction,
				ThresholdHundredths: d.ThresholdHundredths,
			},
			ClosePriceHundredths: d.CloseHundredths,
			Details:              details,
		})
	}
	list := dto.NotificationList{Items: items}
	if len(dbRows) > 0 {
		// すべての行が同じ latest_id を持つ（その利用者の最大の ID）。
		latest := dbRows[0].LatestID
		list.LatestID = &latest
	}
	return list, nil
}

// CountUnseenNotifications は、利用者 userID の、見ていない通知の数を返す。
func (r *NotificationsReader) CountUnseenNotifications(ctx context.Context, userID uuid.UUID) (int64, error) {
	count, err := r.queries.CountUnseenNotifications(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		return 0, fmt.Errorf("見ていない通知の数の問い合わせに失敗しました: %w", err)
	}
	return count, nil
}
