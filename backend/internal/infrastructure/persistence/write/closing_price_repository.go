package write

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/sqlcgen"
)

// ClosingPriceRepository は、PostgreSQL の stock_prices に終値を保存する repository.ClosingPriceRepository の実装。
type ClosingPriceRepository struct {
	queries *sqlcgen.Queries
}

// ClosingPriceRepository が repository.ClosingPriceRepository を満たすことを、コンパイル時に確かめる。
var _ repository.ClosingPriceRepository = (*ClosingPriceRepository)(nil)

// NewClosingPriceRepository は、pool の DB に終値を保存する ClosingPriceRepository を返す。
func NewClosingPriceRepository(pool *pgxpool.Pool) *ClosingPriceRepository {
	return &ClosingPriceRepository{queries: sqlcgen.New(pool)}
}

// Save は、終値 p を stock_prices に保存する。同じ銘柄・同じ取引日の行がすでにあれば、終値と取得日時を上書きする
// （1つの SQL の INSERT … ON CONFLICT … DO UPDATE で行う。読んでから書くと、その間に別の実行が同じ行を入れたときに失敗するため。FR-15）。
func (r *ClosingPriceRepository) Save(ctx context.Context, p model.ClosingPrice) error {
	// 取引日は文字列のまま SQL で date に変換させず、ここで YYYY-MM-DD として読む。
	// PostgreSQL は DateStyle の設定によって別の書き方の日付も受け付けるので、形の違う値を別の日として保存しないため。
	tradingDate, err := time.Parse(time.DateOnly, p.TradingDate)
	if err != nil {
		return fmt.Errorf("取引日 %q を YYYY-MM-DD の日付として読めません: %w", p.TradingDate, err)
	}
	if err := r.queries.UpsertClosingPrice(ctx, sqlcgen.UpsertClosingPriceParams{
		StockCode:   p.StockCode,
		TradingDate: pgtype.Date{Time: tradingDate, Valid: true},
		ClosePrice:  numericFromTenths(p.PriceTenths),
	}); err != nil {
		return fmt.Errorf("stock_prices に保存できません: %w", err)
	}
	return nil
}

// numericFromTenths は、0.1円単位の整数 tenths を、円の NUMERIC にする（tenths × 10^-1）。
// 浮動小数点を通さないので、値が変わらない（プラン 6章の判断1）。DB には小数第1位までの値（例: 3000.0）で保存される。
func numericFromTenths(tenths int64) pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(tenths), Exp: -1, Valid: true}
}
