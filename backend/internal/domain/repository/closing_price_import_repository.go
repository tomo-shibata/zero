package repository

import (
	"context"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// ClosingPriceImportRepository は、価格の CSV の登録（要件1〜10）が使う、DB の読み書き。
type ClosingPriceImportRepository interface {
	// FindCurrencies は、codes のうち銘柄マスタにある銘柄の通貨を、銘柄コードをキーにして返す。なければ空のマップを返す。
	FindCurrencies(ctx context.Context, codes []string) (map[string]model.Currency, error)
	// SaveAll は、1つのトランザクションで prices を保存する。同じ銘柄・同じ取引日の終値があれば、終値と取得日時を上書きする。
	// 途中で失敗したら何も変えない。
	SaveAll(ctx context.Context, prices []model.ClosingPrice) error
}
