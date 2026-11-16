package repository

import (
	"context"

	"github.com/google/uuid"
)

// HeldStockRepository は、利用者が保有している銘柄（保有数量が1以上の銘柄）を通貨と一緒に読む。
// 利用者 ID の型に uuid.UUID を使うのは、Query 側・ミドルウェアと同じ型なら変換が要らないため（値段の更新のプラン 6章の判断7）。
type HeldStockRepository interface {
	// ListHeldStocksOf は、利用者 userID の保有数量が1以上の銘柄を、重複なし・銘柄コードの昇順（COLLATE "C"。Go の文字列比較と同じバイト順）で返す。
	// 売り切った銘柄は含めない。0件なら nil ではなく空のスライスを返す。
	// 価格の今すぐ更新が、ログイン中の利用者の保有銘柄だけの終値を取得するために使う（要件5・要件16）。
	ListHeldStocksOf(ctx context.Context, userID uuid.UUID) ([]HeldStock, error)
}
