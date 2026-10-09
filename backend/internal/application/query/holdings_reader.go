package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// HoldingsReader は、保有データを読む。実装は infrastructure（persistence/read）に置き、
// Query 側はこの IF だけに依存する（依存の向きを内側にするため）。
type HoldingsReader interface {
	// ListHoldingRows は、利用者 userID の保有データを、まとめずに1件ずつ返す（並び順は決めない）。
	// 各行には、銘柄マスタの銘柄名と、その銘柄の最新の終値・取引日（なければ nil）を付ける。
	ListHoldingRows(ctx context.Context, userID uuid.UUID) ([]dto.HoldingRow, error)
}
