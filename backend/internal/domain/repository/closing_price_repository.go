package repository

import (
	"context"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// ClosingPriceRepository は、終値を保存する。
type ClosingPriceRepository interface {
	// Save は、終値 p を保存する。同じ銘柄・同じ取引日の終値がすでにあれば、終値と取得日時を上書きする
	// （再取得した値を正とし、成功として扱う。要件 FR-15）。
	Save(ctx context.Context, p model.ClosingPrice) error
}
