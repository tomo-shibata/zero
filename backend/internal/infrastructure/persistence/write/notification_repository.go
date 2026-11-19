package write

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/sqlcgen"
)

// NotificationRepository は、PostgreSQL の notifications の見たかどうかを書く repository.NotificationSeenRepository の実装。
type NotificationRepository struct {
	queries *sqlcgen.Queries
}

// NotificationRepository が repository.NotificationSeenRepository を満たすことを、コンパイル時に確かめる。
var _ repository.NotificationSeenRepository = (*NotificationRepository)(nil)

// NewNotificationRepository は、pool の DB に書く NotificationRepository を返す。
func NewNotificationRepository(pool *pgxpool.Pool) *NotificationRepository {
	return &NotificationRepository{queries: sqlcgen.New(pool)}
}

// MarkSeenUpTo は、利用者 userID の、ID が upToID 以下で見ていない通知を、1つの SQL で見たにする。
func (r *NotificationRepository) MarkSeenUpTo(ctx context.Context, userID uuid.UUID, upToID int64) error {
	if err := r.queries.MarkNotificationsSeenUpTo(ctx, sqlcgen.MarkNotificationsSeenUpToParams{
		UserID: pgtype.UUID{Bytes: userID, Valid: true},
		UpToID: upToID,
	}); err != nil {
		return fmt.Errorf("notifications を更新できません: %w", err)
	}
	return nil
}
