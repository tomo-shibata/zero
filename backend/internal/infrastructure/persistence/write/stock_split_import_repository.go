package write

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/sqlcgen"
)

// lockStockSplits は、stock_splits を、読み取り以外のすべてのロックと両立しない形でロックする文。
// 同時に2つの取り込みが、入れ替えを混ぜないため（読み取りは待たせない）。
const lockStockSplits = "LOCK TABLE stock_splits IN EXCLUSIVE MODE"

// StockSplitImportRepository は、PostgreSQL への分割の取り込み（repository.StockSplitImportRepository）の実装。
type StockSplitImportRepository struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
}

// StockSplitImportRepository が repository.StockSplitImportRepository を満たすことを、コンパイル時に確かめる。
var _ repository.StockSplitImportRepository = (*StockSplitImportRepository)(nil)

// NewStockSplitImportRepository は、pool の DB に分割を取り込む StockSplitImportRepository を返す。
func NewStockSplitImportRepository(pool *pgxpool.Pool) *StockSplitImportRepository {
	return &StockSplitImportRepository{pool: pool, queries: sqlcgen.New(pool)}
}

// ReplaceStockSplits は、1つのトランザクションで、stock_splits を表ごとロックし、全利用者の売買履歴を読んで check に渡し、
// check が nil を返したら、登録済みの分割をすべて消して splits を入れ、全利用者の並びから保有していない銘柄の行を消す。
// check がエラーを返したときと、途中で失敗したときは、コミットせずに元に戻す（何も変わらない）。
func (r *StockSplitImportRepository) ReplaceStockSplits(ctx context.Context, splits []model.StockSplit, check func(all []repository.UserTrades) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("トランザクションを始められません: %w", err)
	}
	// Rollback と Commit に ctx の取り消しを引き継がない理由は TradeImportRepository.ReplaceTrades と同じ。
	finishCtx := context.WithoutCancel(ctx)
	defer func() { _ = tx.Rollback(finishCtx) }()

	q := r.queries.WithTx(tx)
	if _, err := tx.Exec(ctx, lockStockSplits); err != nil {
		return fmt.Errorf("分割の表のロックに失敗しました: %w", err)
	}
	tradesByUser, err := readAllUserTrades(ctx, q)
	if err != nil {
		return err
	}
	if err := check(tradesByUser); err != nil {
		return err
	}
	if err := q.DeleteAllStockSplits(ctx); err != nil {
		return fmt.Errorf("分割の削除に失敗しました: %w", err)
	}
	if err := copyStockSplits(ctx, tx, splits); err != nil {
		return err
	}
	if err := sweepHoldingOrders(ctx, q, tradesByUser, splits); err != nil {
		return err
	}
	if err := tx.Commit(finishCtx); err != nil {
		return fmt.Errorf("コミットに失敗しました: %w", err)
	}
	return nil
}

// readAllUserTrades は、全利用者の売買履歴を、利用者の ID の昇順（バイト順）で返す。各利用者の売買履歴は登録した順（id の昇順）。
// 売買履歴のない利用者は含めない。
func readAllUserTrades(ctx context.Context, q *sqlcgen.Queries) ([]repository.UserTrades, error) {
	rows, err := q.ListTradesOfAllUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("売買履歴の問い合わせに失敗しました: %w", err)
	}
	byUser, err := groupTradesByUser(rows)
	if err != nil {
		return nil, err
	}
	all := make([]repository.UserTrades, 0, len(byUser))
	for _, id := range sortedUserIDs(byUser) {
		all = append(all, repository.UserTrades{UserID: id, Trades: byUser[id]})
	}
	return all, nil
}

// sortedUserIDs は、byUser の利用者の ID を昇順（バイト順）で返す（map の順に頼らず、毎回同じ順で動かすため）。
func sortedUserIDs(byUser map[uuid.UUID][]model.Trade) []uuid.UUID {
	return slices.SortedFunc(maps.Keys(byUser), func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
}

// copyStockSplits は、splits を COPY で stock_splits に入れる。
func copyStockSplits(ctx context.Context, tx pgx.Tx, splits []model.StockSplit) error {
	rows := make([][]any, 0, len(splits))
	for _, s := range splits {
		date, err := time.Parse(time.DateOnly, s.SplitDate)
		if err != nil {
			return fmt.Errorf("分割の日 %q を読めません: %w", s.SplitDate, err)
		}
		rows = append(rows, []any{s.StockCode, pgtype.Date{Time: date, Valid: true}, int32(s.Ratio)}) //nolint:gosec // G115: 分割後の株数は2〜1,000
	}
	copied, err := tx.CopyFrom(ctx, pgx.Identifier{"stock_splits"}, []string{"stock_code", "split_date", "ratio"}, pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("分割の登録に失敗しました: %w", err)
	}
	if copied != int64(len(splits)) {
		return fmt.Errorf("分割を %d 件登録するはずが %d 件でした", len(splits), copied)
	}
	return nil
}

// sweepHoldingOrders は、利用者ごとに、splits を混ぜた保有数量が1以上の銘柄だけを残して、並びの行を消す
// （分割をなくすと保有数量が0になる銘柄がありうるため。株式分割のプラン 4.3）。
// 対象は、売買履歴のある利用者と、並びの行のある利用者（売買履歴がなければ、並びの行はすべて消える）。
func sweepHoldingOrders(ctx context.Context, q *sqlcgen.Queries, all []repository.UserTrades, splits []model.StockSplit) error {
	tradesOf := make(map[uuid.UUID][]model.Trade, len(all))
	for _, u := range all {
		tradesOf[u.UserID] = u.Trades
	}
	orderUsers, err := q.ListHoldingOrderUserIDs(ctx)
	if err != nil {
		return fmt.Errorf("並びを持つ利用者の問い合わせに失敗しました: %w", err)
	}
	for _, id := range orderUsers {
		if _, ok := tradesOf[id.Bytes]; !ok {
			tradesOf[id.Bytes] = nil
		}
	}
	// 利用者の ID の昇順に消す。
	for _, id := range sortedUserIDs(tradesOf) {
		err := q.DeleteHoldingOrdersExcept(ctx, sqlcgen.DeleteHoldingOrdersExceptParams{
			UserID: pgtype.UUID{Bytes: id, Valid: true},
			Codes:  model.HeldStockCodes(tradesOf[id], splits),
		})
		if err != nil {
			return fmt.Errorf("並びの行の削除に失敗しました: %w", err)
		}
	}
	return nil
}
