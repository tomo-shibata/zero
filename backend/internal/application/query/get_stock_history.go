package query

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// GetStockHistory は、利用者の、1つの銘柄の売買履歴・分割・終値から、銘柄ごとの売買履歴の画面の内容を返すユースケース。
// 3つの読み取りは同じトランザクションにしない（読む間に取り込みが入れ替わっても、次に開けば正しくなる。プラン 判断5）。
type GetStockHistory struct {
	reader StockHistoryReader
}

// NewGetStockHistory は、r から読む GetStockHistory を返す。
func NewGetStockHistory(r StockHistoryReader) *GetStockHistory {
	return &GetStockHistory{reader: r}
}

// Execute は、利用者 userID の、銘柄 code の売買履歴を返す。他の利用者の売買履歴は含めない（要件21）。
// 売買履歴が1件もなければ ErrStockHistoryNotFound を返す（要件20）。
// 金額が int64 に収まらなければ、誤った値を返さずに ErrCalculationOverflow を（包んで）返す。
func (q *GetStockHistory) Execute(ctx context.Context, userID uuid.UUID, code string) (dto.StockHistory, error) {
	trades, err := q.reader.ListTradesOfStock(ctx, userID, code)
	if err != nil {
		return dto.StockHistory{}, fmt.Errorf("売買履歴を読めません: %w", err)
	}
	if len(trades) == 0 {
		return dto.StockHistory{}, ErrStockHistoryNotFound
	}
	splits, err := q.reader.ListStockSplitsOfStock(ctx, code)
	if err != nil {
		return dto.StockHistory{}, fmt.Errorf("分割を読めません: %w", err)
	}
	closes, err := q.reader.ListClosingPricesSince(ctx, code, firstTradeDate(trades))
	if err != nil {
		return dto.StockHistory{}, fmt.Errorf("終値を読めません: %w", err)
	}
	history, err := BuildStockHistory(trades, splits, closes)
	if err != nil {
		return dto.StockHistory{}, fmt.Errorf("売買履歴の画面の内容を計算できません: %w", err)
	}
	return history, nil
}
