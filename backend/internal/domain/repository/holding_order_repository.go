package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// HoldingOrderRepository は、保有株式一覧の並びの保存（カード表示の要件23・要件31）が使う、DB の読み書き。
// 利用者 ID の型に uuid.UUID を使う理由は HeldStockRepository と同じ。
type HoldingOrderRepository interface {
	// ReplaceHoldingOrder は、1つのトランザクションで、利用者の行をロックし、その利用者の保有数量が1以上の銘柄コードを読み、
	// build(それ) が返した並びで、その利用者の並びを入れ替える（銘柄の削除・取り込みとの同時実行で、消した銘柄の行が残らないようにする。判断6）。
	// build がエラーを返したら何も変えずに、そのエラーを（包んで）返す。
	ReplaceHoldingOrder(ctx context.Context, userID uuid.UUID, build func(heldCodes []string) (model.HoldingOrder, error)) error
}
