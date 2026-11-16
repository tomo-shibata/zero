package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ErrHoldingNotFound は、対象の保有データが（その利用者に）1件もないこと。
// 保有データを消す・書き換える IF が、何も変えなかったことを知らせるために返す（%w で包んでもよい）。
var ErrHoldingNotFound = errors.New("holding not found")

// HoldingDeleteRepository は、利用者の1銘柄の保有データをすべて消す（値段の更新のプラン 4.4）。
// 利用者 ID の型に uuid.UUID を使う理由は HeldStockRepository と同じ（プラン 6章の判断7）。
type HoldingDeleteRepository interface {
	// DeleteAll は、利用者 userID の、銘柄 stockCode の保有データをすべて消す（要件14）。
	// 他の利用者の保有データと、その銘柄の手入力の価格は消さない。1件も消えなければ ErrHoldingNotFound を返す。
	DeleteAll(ctx context.Context, userID uuid.UUID, stockCode string) error
}
