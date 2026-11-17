package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// UserTrades は、1人の利用者の売買履歴。Trades はその利用者の登録した順。
// 価格（PriceHundredths）は保有数量の計算に使わないので、読まなくてよい（0のまま）。
type UserTrades struct {
	UserID uuid.UUID
	Trades []model.Trade
}

// StockSplitImportRepository は、分割の CSV の取り込み（株式分割の要件9〜12）が使う、DB の読み書き。
type StockSplitImportRepository interface {
	// ReplaceStockSplits は、1つのトランザクションで、stock_splits を表ごとロックし、全利用者の売買履歴を読んで check に渡す（利用者の ID の昇順。売買履歴のある利用者だけでよい）。
	// check がエラーを返せば何も変えずにそのエラーを返す。nil なら、登録済みの分割をすべて消して splits を入れ、
	// 全利用者の並び（holding_orders）から、splits で計算して保有していない銘柄の行を消す。途中で失敗したら何も変えない。
	// 銘柄マスタ・売買履歴・終値は変えない。
	ReplaceStockSplits(ctx context.Context, splits []model.StockSplit, check func(all []UserTrades) error) error
}
