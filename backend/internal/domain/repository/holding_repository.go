// Package repository は、Command 側が使う書き込み用リポジトリの IF を持つ。
// 実装は infrastructure（persistence/write）に置き、domain と application はこの IF だけに依存する（依存の向きを内側にするため）。
package repository

import "context"

// HoldingRepository は、保有している銘柄を通貨と一緒に読む（売買履歴から求める。保有数量が1以上の銘柄）。
type HoldingRepository interface {
	// ListHeldStocks は、誰かの保有数量が1以上の銘柄を、重複なし・銘柄コードの昇順（Go の文字列比較と同じバイト順）で返す。売り切った銘柄は含めない。
	// 価格の取得処理が、どの銘柄の終値を取得するかを決め、取得した終値の精度を通貨で確かめるために使う（要件5・要件16）。
	ListHeldStocks(ctx context.Context) ([]HeldStock, error)
}
