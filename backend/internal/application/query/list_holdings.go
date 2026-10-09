// Package query は、Query 側（状態を読むだけ）のユースケースと、読んだ値から一覧を作る純粋関数を持つ。
// domain を通さずに HoldingsReader で読み、dto の Read Model を返す（軽量 CQRS）。
package query

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// ListHoldings は、利用者の保有株式一覧を返すユースケース（要件 FR-1、FR-3〜10）。
type ListHoldings struct {
	reader HoldingsReader
}

// NewListHoldings は、r から保有データを読む ListHoldings を返す。
func NewListHoldings(r HoldingsReader) *ListHoldings {
	return &ListHoldings{reader: r}
}

// Execute は、利用者 userID の保有株式一覧を返す。他の利用者の保有データは含めない（FR-1）。
// 一覧を作る計算が int64 に収まらない保有データなら、誤った値を返さずにエラーを返す（プラン 4.4「API」、6章の判断14）。
func (q *ListHoldings) Execute(ctx context.Context, userID uuid.UUID) (dto.HoldingsList, error) {
	rows, err := q.reader.ListHoldingRows(ctx, userID)
	if err != nil {
		return dto.HoldingsList{}, fmt.Errorf("保有データを読めません: %w", err)
	}
	// AggregateHoldings は桁あふれを知らせられない（エラーを返さない）ので、集約の前に確かめる。
	if err := checkCalculationRange(rows); err != nil {
		return dto.HoldingsList{}, fmt.Errorf("保有データから一覧を計算できません: %w", err)
	}
	return AggregateHoldings(rows), nil
}
