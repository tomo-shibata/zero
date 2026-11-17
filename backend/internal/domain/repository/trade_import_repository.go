package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// TradeImportRepository は、売買履歴の CSV の取り込み（要件19〜25）が使う、DB の読み書き。
// 利用者 ID の型に uuid.UUID を使う理由は HeldStockRepository と同じ。
type TradeImportRepository interface {
	// UserExists は、利用者 userID が DB にいるかを返す。
	UserExists(ctx context.Context, userID uuid.UUID) (bool, error)
	// FindStocks は、codes のうち銘柄マスタにある銘柄を、銘柄コードをキーにして返す。なければ空のマップを返す。
	FindStocks(ctx context.Context, codes []string) (map[string]model.Stock, error)
	// ReplaceTrades は、1つのトランザクションで、利用者の売買履歴をすべて消し、newStocks を足し、renamed の銘柄名を変え、
	// trades をスライスの順に入れる（先の行ほど trades.id が小さい。要件8）。途中で失敗したら何も変えない。
	// 売買履歴を入れた後に、その利用者の並びの行のうち、保有していない銘柄の行を消す（カード表示の要件28）。
	// 他の利用者の売買履歴・並びと、銘柄マスタの通貨は変えない（renamed は銘柄名だけを変える）。
	ReplaceTrades(ctx context.Context, userID uuid.UUID, newStocks, renamed []model.Stock, trades []model.Trade) error
}
