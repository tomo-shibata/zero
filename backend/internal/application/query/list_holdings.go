// Package query は、Query 側（状態を読むだけ）のユースケースと、読んだ値から一覧を作る純粋関数を持つ。
// domain を通さずに HoldingsReader で読み、dto の Read Model を返す（軽量 CQRS）。
package query

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// ListHoldings は、利用者の保有株式一覧を返すユースケース（保有株式一覧の FR-1、要件4・6〜10）。
type ListHoldings struct {
	reader HoldingsReader
}

// NewListHoldings は、r から売買履歴を読む ListHoldings を返す。
func NewListHoldings(r HoldingsReader) *ListHoldings {
	return &ListHoldings{reader: r}
}

// Execute は、利用者 userID の保有株式一覧を、その利用者の売買履歴から計算して返す。他の利用者の売買履歴は含めない（保有株式一覧の FR-1）。
// 保有数量が途中で負になる売買履歴や、計算が int64 に収まらない売買履歴なら、誤った値を返さずにエラーを返す
// （CalculateHoldings の ErrNegativeQuantity・ErrCalculationOverflow。売買履歴のプラン 6章の判断5・6）。
func (q *ListHoldings) Execute(ctx context.Context, userID uuid.UUID) (dto.HoldingsList, error) {
	rows, err := q.reader.ListTradeRows(ctx, userID)
	if err != nil {
		return dto.HoldingsList{}, fmt.Errorf("売買履歴を読めません: %w", err)
	}
	list, err := CalculateHoldings(rows)
	if err != nil {
		return dto.HoldingsList{}, fmt.Errorf("売買履歴から一覧を計算できません: %w", err)
	}
	return list, nil
}
