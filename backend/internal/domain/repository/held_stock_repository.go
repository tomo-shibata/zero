package repository

import (
	"context"

	"github.com/google/uuid"
)

// HeldStockRepository は、利用者が保有している銘柄を読む。
// 利用者 ID の型に uuid.UUID を使うのは、Query 側・ミドルウェアと同じ型なら変換が要らないため（値段の更新のプラン 6章の判断7）。
type HeldStockRepository interface {
	// ListHeldStockCodesOf は、利用者 userID の保有データにある銘柄コードを、重複なし・昇順（COLLATE "C"。Go の文字列比較と同じバイト順）で返す。
	// 0件なら nil ではなく空のスライスを返す。
	// 価格の今すぐ更新が、ログイン中の利用者の保有銘柄だけの終値を取得するために使う（要件16）。
	ListHeldStockCodesOf(ctx context.Context, userID uuid.UUID) ([]string, error)
}
