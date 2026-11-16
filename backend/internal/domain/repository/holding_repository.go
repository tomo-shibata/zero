// Package repository は、Command 側が使う書き込み用リポジトリの IF を持つ。
// 実装は infrastructure（persistence/write）に置き、domain と application はこの IF だけに依存する（依存の向きを内側にするため）。
package repository

import "context"

// HoldingRepository は、保有している銘柄を読む（売買履歴から求める。保有数量が1以上の銘柄）。
type HoldingRepository interface {
	// ListHeldStockCodes は、誰かの保有数量が1以上の銘柄コードを、重複なし・昇順（Go の文字列比較）で返す。売り切った銘柄は含めない。
	// 価格の取得処理が、どの銘柄の終値を取得するかを決めるために使う（要件16）。
	ListHeldStockCodes(ctx context.Context) ([]string, error)
}
