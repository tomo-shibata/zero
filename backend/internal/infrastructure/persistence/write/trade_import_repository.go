package write

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math/big"
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

// priceExponent は、0.01 単位の整数を NUMERIC にするときの指数（10^-2）。
const priceExponent = -2

// TradeImportRepository は、PostgreSQL への売買履歴の取り込み（repository.TradeImportRepository）の実装。
type TradeImportRepository struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
}

// TradeImportRepository が repository.TradeImportRepository を満たすことを、コンパイル時に確かめる。
var _ repository.TradeImportRepository = (*TradeImportRepository)(nil)

// NewTradeImportRepository は、pool の DB に売買履歴を取り込む TradeImportRepository を返す。
func NewTradeImportRepository(pool *pgxpool.Pool) *TradeImportRepository {
	return &TradeImportRepository{pool: pool, queries: sqlcgen.New(pool)}
}

// UserExists は、利用者 userID が users にいるかを返す。
func (r *TradeImportRepository) UserExists(ctx context.Context, userID uuid.UUID) (bool, error) {
	exists, err := r.queries.UserExists(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		return false, fmt.Errorf("利用者の問い合わせに失敗しました: %w", err)
	}
	return exists, nil
}

// FindStocks は、codes のうち銘柄マスタにある銘柄を、銘柄コードをキーにして返す。なければ空のマップを返す。
func (r *TradeImportRepository) FindStocks(ctx context.Context, codes []string) (map[string]model.Stock, error) {
	rows, err := r.queries.ListStocksByCodes(ctx, codes)
	if err != nil {
		return nil, fmt.Errorf("銘柄マスタの問い合わせに失敗しました: %w", err)
	}
	stocks := make(map[string]model.Stock, len(rows))
	for _, row := range rows {
		stocks[row.Code] = model.Stock{Code: row.Code, Name: row.Name, Currency: model.Currency(row.Currency)}
	}
	return stocks, nil
}

// ReplaceTrades は、1つのトランザクションで、利用者の売買履歴をすべて消し、newStocks を足し、renamed の銘柄名を変え、
// trades をスライスの順に COPY で入れる（COPY は送った順に行を入れるので、先の行ほど trades.id が小さい。要件8）。
// 途中で失敗したら、コミットせずに元に戻す（何も変わらない）。
func (r *TradeImportRepository) ReplaceTrades(ctx context.Context, userID uuid.UUID, newStocks, renamed []model.Stock, trades []model.Trade) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("トランザクションを始められません: %w", err)
	}
	// コミットした後の Rollback は ErrTxClosed を返すだけで何もしない。失敗の理由は元のエラーを返すので、取り消しのエラーは見ない。
	// Rollback と Commit には ctx の取り消しを引き継がない（Ctrl+C でコミットの返事を待たずに切って、
	// コミットされたかどうか分からないまま「何も変えていません」と出さないため）。
	finishCtx := context.WithoutCancel(ctx)
	defer func() { _ = tx.Rollback(finishCtx) }()

	if err := replaceTradesIn(ctx, tx, r.queries.WithTx(tx), userID, newStocks, renamed, trades); err != nil {
		return err
	}
	if err := tx.Commit(finishCtx); err != nil {
		return fmt.Errorf("コミットに失敗しました: %w", err)
	}
	return nil
}

// replaceTradesIn は、ReplaceTrades の書き込みを、トランザクション tx の中で行う。
func replaceTradesIn(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, userID uuid.UUID, newStocks, renamed []model.Stock, trades []model.Trade) error {
	owner := pgtype.UUID{Bytes: userID, Valid: true}
	// 同じ利用者への取り込みが同時に動いても、売買履歴が二重にならないよう、利用者の行をロックして順番に動かす。
	if err := q.LockUser(ctx, owner); err != nil {
		return fmt.Errorf("利用者の行のロックに失敗しました: %w", err)
	}
	if _, err := q.DeleteAllTradesOf(ctx, owner); err != nil {
		return fmt.Errorf("売買履歴の削除に失敗しました: %w", err)
	}
	for _, s := range newStocks {
		if err := q.InsertStock(ctx, sqlcgen.InsertStockParams{Code: s.Code, Name: s.Name, Currency: string(s.Currency)}); err != nil {
			return fmt.Errorf("銘柄 %q の登録に失敗しました: %w", s.Code, err)
		}
	}
	for _, s := range renamed {
		changed, err := q.RenameStock(ctx, sqlcgen.RenameStockParams{Code: s.Code, Name: s.Name})
		if err != nil {
			return fmt.Errorf("銘柄 %q の銘柄名の変更に失敗しました: %w", s.Code, err)
		}
		if changed != 1 {
			return errors.New("銘柄名を変える銘柄が銘柄マスタにありません: " + s.Code)
		}
	}
	if err := copyTrades(ctx, tx, owner, trades); err != nil {
		return err
	}
	// 入れ直した後に保有していない銘柄（CSV から消えた、売り切った）の並びの行を消す。また保有したときは末尾に出る（要件28。プラン 4.3「消すとき」）。
	// 保有している銘柄は、入れた売買履歴とその銘柄の分割から、domain の HeldStockCodes で決める（株式分割のプラン 4.3。IF が分割を受け取らないので、ここで読み直す）。
	held, err := heldCodesOfImported(ctx, q, trades)
	if err != nil {
		return err
	}
	if err := q.DeleteHoldingOrdersExcept(ctx, sqlcgen.DeleteHoldingOrdersExceptParams{UserID: owner, Codes: held}); err != nil {
		return fmt.Errorf("並びの行の削除に失敗しました: %w", err)
	}
	return nil
}

// heldCodesOfImported は、取り込んだ売買履歴 trades とその銘柄の分割（DB から読み直す）から、保有数量が1以上の銘柄コードを返す。
func heldCodesOfImported(ctx context.Context, q *sqlcgen.Queries, trades []model.Trade) ([]string, error) {
	codes := make(map[string]struct{})
	for _, t := range trades {
		codes[t.StockCode] = struct{}{}
	}
	splitRows, err := q.ListStockSplitsByCodes(ctx, slices.Collect(maps.Keys(codes)))
	if err != nil {
		return nil, fmt.Errorf("分割の問い合わせに失敗しました: %w", err)
	}
	splits := make([]model.StockSplit, 0, len(splitRows))
	for _, row := range splitRows {
		s, err := toStockSplit(row.StockCode, row.SplitDate, row.Ratio)
		if err != nil {
			return nil, err
		}
		splits = append(splits, s)
	}
	return model.HeldStockCodes(trades, splits), nil
}

// FindStockSplits は、codes の銘柄の分割を返す（並び順は決めない）。銘柄マスタにない銘柄の分割も返す（銘柄マスタを見ない）。なければ空のスライスを返す。
func (r *TradeImportRepository) FindStockSplits(ctx context.Context, codes []string) ([]model.StockSplit, error) {
	rows, err := r.queries.ListStockSplitsByCodes(ctx, codes)
	if err != nil {
		return nil, fmt.Errorf("分割の問い合わせに失敗しました: %w", err)
	}
	splits := make([]model.StockSplit, 0, len(rows))
	for _, row := range rows {
		s, err := toStockSplit(row.StockCode, row.SplitDate, row.Ratio)
		if err != nil {
			return nil, err
		}
		splits = append(splits, s)
	}
	return splits, nil
}

// copyTrades は、trades をスライスの順に、利用者 owner の売買履歴として COPY で入れる。
func copyTrades(ctx context.Context, tx pgx.Tx, owner pgtype.UUID, trades []model.Trade) error {
	rows := make([][]any, 0, len(trades))
	for _, t := range trades {
		date, err := time.Parse(time.DateOnly, t.TradeDate)
		if err != nil {
			return fmt.Errorf("取引日 %q を読めません: %w", t.TradeDate, err)
		}
		rows = append(rows, []any{
			owner,
			t.StockCode,
			string(t.Side),
			t.Quantity,
			pgtype.Numeric{Int: big.NewInt(t.PriceHundredths), Exp: priceExponent, Valid: true},
			pgtype.Date{Time: date, Valid: true},
		})
	}
	columns := []string{"user_id", "stock_code", "side", "quantity", "price", "trade_date"}
	copied, err := tx.CopyFrom(ctx, pgx.Identifier{"trades"}, columns, pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("売買履歴の登録に失敗しました: %w", err)
	}
	if copied != int64(len(trades)) {
		return fmt.Errorf("売買履歴を %d 件登録するはずが %d 件でした", len(trades), copied)
	}
	return nil
}
