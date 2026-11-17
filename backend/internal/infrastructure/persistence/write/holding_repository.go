// Package write は、Command 側の書き込みの実装（domain/repository の IF の実装）を持つ。sqlc の生成コードで SQL を実行する。
package write

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/sqlcgen"
)

// HoldingRepository は、PostgreSQL の売買履歴と分割から保有している銘柄を通貨と一緒に読む repository.HoldingRepository と repository.HeldStockRepository の実装。
// 保有数量が1以上かどうかは、SQL ではなく domain の HeldStockCodes で判定する（分割を数えるため。プラン 4.3）。
type HoldingRepository struct {
	queries *sqlcgen.Queries
}

// HoldingRepository が repository.HoldingRepository と repository.HeldStockRepository を満たすことを、コンパイル時に確かめる。
var (
	_ repository.HoldingRepository   = (*HoldingRepository)(nil)
	_ repository.HeldStockRepository = (*HoldingRepository)(nil)
)

// NewHoldingRepository は、pool の DB の売買履歴と分割から保有している銘柄を読む HoldingRepository を返す。
func NewHoldingRepository(pool *pgxpool.Pool) *HoldingRepository {
	return &HoldingRepository{queries: sqlcgen.New(pool)}
}

// ListHeldStocks は、誰かの保有数量が1以上の銘柄を、通貨と一緒に、重複なし・銘柄コードの昇順（Go の文字列比較と同じバイト順）で返す。
// 全利用者の売買履歴と分割を読み、利用者ごとに domain の HeldStockCodes で保有数量が1以上の銘柄を選ぶ。0件なら nil ではなく空のスライスを返す。
func (r *HoldingRepository) ListHeldStocks(ctx context.Context) ([]repository.HeldStock, error) {
	tradeRows, err := r.queries.ListTradesOfAllUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("売買履歴の問い合わせに失敗しました: %w", err)
	}
	// tradeRows は登録した順（id の昇順）なので、利用者ごとに分けても、その利用者の売買履歴は登録した順のまま。
	tradesOf := make(map[[16]byte][]model.Trade)
	currencies := make(map[string]model.Currency)
	for _, row := range tradeRows {
		t, err := toTrade(row.StockCode, row.Side, row.Quantity, row.TradeDate)
		if err != nil {
			return nil, err
		}
		tradesOf[row.UserID.Bytes] = append(tradesOf[row.UserID.Bytes], t)
		currencies[row.StockCode] = model.Currency(row.Currency)
	}
	splitRows, err := r.queries.ListAllStockSplits(ctx)
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

	held := make(map[string]bool)
	for _, trades := range tradesOf {
		for _, code := range model.HeldStockCodes(trades, splits) {
			held[code] = true
		}
	}
	return heldStocksOf(held, currencies), nil
}

// ListHeldStocksOf は、利用者 userID の保有数量が1以上の銘柄を、通貨と一緒に、重複なし・銘柄コードの昇順（Go の文字列比較と同じバイト順）で返す。
// 利用者の売買履歴と分割を読み、domain の HeldStockCodes で保有数量が1以上の銘柄を選ぶ。0件なら nil ではなく空のスライスを返す。
func (r *HoldingRepository) ListHeldStocksOf(ctx context.Context, userID uuid.UUID) ([]repository.HeldStock, error) {
	codes, currencies, err := heldOfUser(ctx, r.queries, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("利用者の保有している銘柄の問い合わせに失敗しました: %w", err)
	}
	held := make(map[string]bool, len(codes))
	for _, code := range codes {
		held[code] = true
	}
	return heldStocksOf(held, currencies), nil
}

// heldStocksOf は、保有している銘柄コード held を、currencies の通貨を付けて、銘柄コードの昇順（Go の文字列比較）で返す。空でも nil にしない。
func heldStocksOf(held map[string]bool, currencies map[string]model.Currency) []repository.HeldStock {
	stocks := make([]repository.HeldStock, 0, len(held))
	for code := range held {
		stocks = append(stocks, repository.HeldStock{Code: code, Currency: currencies[code]})
	}
	slices.SortFunc(stocks, func(a, b repository.HeldStock) int { return cmp.Compare(a.Code, b.Code) })
	return stocks
}
