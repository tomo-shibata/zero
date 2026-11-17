package query

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// ErrStockHistoryNotFound は、利用者がその銘柄の売買履歴を1件も持たないこと（要件20）。
// 一度も売買していない・銘柄マスタにない・他の利用者だけが売買している、のどれでも同じエラーにする（違いを外に出さない）。
var ErrStockHistoryNotFound = errors.New("その銘柄の売買履歴がありません")

// StockHistoryReader は、銘柄ごとの売買履歴の画面が使う、売買・分割・終値を読む。実装は infrastructure（persistence/read）に置き、
// Query 側はこの IF だけに依存する（依存の向きを内側にするため）。
type StockHistoryReader interface {
	// ListTradesOfStock は、利用者 userID の、銘柄 code の売買履歴を返す（銘柄名・通貨つき。ClosePriceHundredths・PriceDate は nil）。並びは決めない。
	ListTradesOfStock(ctx context.Context, userID uuid.UUID, code string) ([]dto.TradeRow, error)
	// ListStockSplitsOfStock は、銘柄 code の分割を返す。並びは決めない。
	ListStockSplitsOfStock(ctx context.Context, code string) ([]dto.StockSplitRow, error)
	// ListClosingPricesSince は、銘柄 code の、取引日が from と同じかそれより後の終値を、取引日の昇順に返す。
	ListClosingPricesSince(ctx context.Context, code, from string) ([]dto.ClosingPriceRow, error)
}
