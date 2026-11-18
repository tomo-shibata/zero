package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// HoldingsReader は、売買履歴と保存した並びを読む。実装は infrastructure（persistence/read）に置き、
// Query 側はこの IF だけに依存する（依存の向きを内側にするため）。
type HoldingsReader interface {
	// ListTradeRows は、利用者 userID の売買履歴を、1件ずつ返す（並び順は決めない）。
	// 各行には、銘柄マスタの銘柄名と、その銘柄の最新の終値・取引日（なければ nil）を付ける。
	ListTradeRows(ctx context.Context, userID uuid.UUID) ([]dto.TradeRow, error)
	// ListStockSplits は、利用者 userID が売買履歴を持つ銘柄の分割を少なくとも返す（ほかの銘柄の分割が混ざってもよい。
	// CalculateHoldings は rows にない銘柄の分割を使わない）。並び順は決めない。なければ空のスライス。
	// 分割1件は1回だけ返す（売買履歴が何件あっても重ねない）。銘柄マスタは見ない（株式分割の要件4・要件11）。
	// 実装は売買履歴で絞らない（ListTradeRows と別の読み取りなので、間に取り込みが入ると、絞った分割が読んだ売買履歴と食い違うため）。
	ListStockSplits(ctx context.Context, userID uuid.UUID) ([]dto.StockSplitRow, error)
	// ListHoldingOrder は、利用者 userID の保存した並び（銘柄コード）を、位置の順に返す。保存していなければ空のスライス（要件23・要件26）。
	ListHoldingOrder(ctx context.Context, userID uuid.UUID) ([]string, error)
	// LatestUsdJpyRate は、日付がいちばん新しいドル円のレートを返す（ドルを円にまとめる合計の要件14。全利用者で共有）。1件もなければ nil, nil。
	LatestUsdJpyRate(ctx context.Context) (*dto.UsdJpyRate, error)
	// ListManagedAccounts は、利用者 userID のおまかせ運用の口座ごとに、日付がいちばん新しい記録を1件ずつ返す（おまかせ運用の要件4・要件5）。
	// 並び順は決めない（名前の順にするのは CalculateHoldings）。口座がなければ空のスライス。
	ListManagedAccounts(ctx context.Context, userID uuid.UUID) ([]dto.ManagedAccountRow, error)
}
