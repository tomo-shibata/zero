package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ErrHoldingNotFound は、対象の売買履歴が（その利用者に）1件もないこと。
// 売買履歴を消す・書き換える IF が、何も変えなかったことを知らせるために返す（%w で包んでもよい）。
var ErrHoldingNotFound = errors.New("holding not found")

// HoldingDeleteRepository は、利用者の1銘柄の売買履歴をすべて消す（値段の更新のプラン 4.4。実装を売買履歴にした経緯は売買履歴のプラン 6章の判断3）。
// 利用者 ID の型に uuid.UUID を使う理由は HeldStockRepository と同じ（プラン 6章の判断7）。
type HoldingDeleteRepository interface {
	// DeleteAll は、利用者 userID の、銘柄 stockCode の売買履歴をすべて消す（要件18）。
	// 他の利用者の売買履歴と、銘柄マスタ・終値は消さない。1件も消えなければ ErrHoldingNotFound を返す。
	DeleteAll(ctx context.Context, userID uuid.UUID, stockCode string) error
}
