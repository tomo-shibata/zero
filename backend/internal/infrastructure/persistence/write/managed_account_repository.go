package write

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/sqlcgen"
)

// ManagedAccountRepository は、PostgreSQL へのおまかせ運用の口座と記録の書き込みの実装。
// 今は CSV の取り込み（repository.ManagedAccountImportRepository）だけ。口座の削除（PR②）はここに足す。
type ManagedAccountRepository struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
}

// ManagedAccountRepository が repository.ManagedAccountImportRepository を満たすことを、コンパイル時に確かめる。
var _ repository.ManagedAccountImportRepository = (*ManagedAccountRepository)(nil)

// NewManagedAccountRepository は、pool の DB に口座と記録を書き込む ManagedAccountRepository を返す。
func NewManagedAccountRepository(pool *pgxpool.Pool) *ManagedAccountRepository {
	return &ManagedAccountRepository{pool: pool, queries: sqlcgen.New(pool)}
}

// UserExists は、利用者 userID が users にいるかを返す。
func (r *ManagedAccountRepository) UserExists(ctx context.Context, userID uuid.UUID) (bool, error) {
	exists, err := r.queries.UserExists(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		return false, fmt.Errorf("利用者の問い合わせに失敗しました: %w", err)
	}
	return exists, nil
}

// ReplaceManagedAccounts は、1つのトランザクションで、利用者の行をロックし、その利用者の口座をすべて消し（記録も消える）、
// valuations に出てくる名前ごとに口座を作り、valuations をスライスの順に COPY で入れる。
// 途中で失敗したら、コミットせずに元に戻す（何も変わらない）。
func (r *ManagedAccountRepository) ReplaceManagedAccounts(ctx context.Context, userID uuid.UUID, valuations []model.ManagedValuation) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("トランザクションを始められません: %w", err)
	}
	// Rollback と Commit に ctx の取り消しを引き継がない理由は TradeImportRepository.ReplaceTrades と同じ。
	finishCtx := context.WithoutCancel(ctx)
	defer func() { _ = tx.Rollback(finishCtx) }()

	owner := pgtype.UUID{Bytes: userID, Valid: true}
	q := r.queries.WithTx(tx)
	// 同じ利用者への取り込みが同時に動いても、口座が入り混じらないよう、利用者の行をロックして順番に動かす（プラン 判断7）。
	if err := q.LockUser(ctx, owner); err != nil {
		return fmt.Errorf("利用者の行のロックに失敗しました: %w", err)
	}
	if _, err := q.DeleteAllManagedAccountsOf(ctx, owner); err != nil {
		return fmt.Errorf("口座の削除に失敗しました: %w", err)
	}
	if err := copyManagedAccounts(ctx, tx, owner, valuations); err != nil {
		return err
	}
	if err := copyManagedValuations(ctx, tx, owner, valuations); err != nil {
		return err
	}
	if err := tx.Commit(finishCtx); err != nil {
		return fmt.Errorf("コミットに失敗しました: %w", err)
	}
	return nil
}

// copyManagedAccounts は、valuations に出てくる名前ごとに口座を1つ、COPY で入れる（名前は書いたとおりの文字列で数える。最初に出た順）。
func copyManagedAccounts(ctx context.Context, tx pgx.Tx, owner pgtype.UUID, valuations []model.ManagedValuation) error {
	seen := make(map[string]struct{}, len(valuations))
	var rows [][]any
	for _, v := range valuations {
		if _, ok := seen[v.AccountName]; ok {
			continue
		}
		seen[v.AccountName] = struct{}{}
		rows = append(rows, []any{owner, v.AccountName})
	}
	copied, err := tx.CopyFrom(ctx, pgx.Identifier{"managed_accounts"}, []string{"user_id", "name"}, pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("口座の登録に失敗しました: %w", err)
	}
	if copied != int64(len(rows)) {
		return fmt.Errorf("口座を %d 件登録するはずが %d 件でした", len(rows), copied)
	}
	return nil
}

// copyManagedValuations は、valuations を利用者 owner の記録として COPY で入れる。
func copyManagedValuations(ctx context.Context, tx pgx.Tx, owner pgtype.UUID, valuations []model.ManagedValuation) error {
	rows := make([][]any, 0, len(valuations))
	for _, v := range valuations {
		date, err := time.Parse(time.DateOnly, v.ValuationDate)
		if err != nil {
			return fmt.Errorf("評価額の基準日 %q を読めません: %w", v.ValuationDate, err)
		}
		rows = append(rows, []any{owner, v.AccountName, pgtype.Date{Time: date, Valid: true}, v.Valuation, v.Gain})
	}
	columns := []string{"user_id", "name", "valuation_date", "valuation", "gain"}
	copied, err := tx.CopyFrom(ctx, pgx.Identifier{"managed_account_valuations"}, columns, pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("記録の登録に失敗しました: %w", err)
	}
	if copied != int64(len(valuations)) {
		return fmt.Errorf("記録を %d 件登録するはずが %d 件でした", len(valuations), copied)
	}
	return nil
}
