package command

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
)

// RefreshPrices は、ログイン中の利用者が保有する銘柄の終値を、今すぐ取得して保存するユースケース（要件16）。
// 取得と保存は、定時実行と同じ FetchClosingPrices を使う（非機能2、値段の更新のプラン 6章の判断8）。
type RefreshPrices struct {
	fetch *FetchClosingPrices
	held  repository.HeldStockRepository
}

// NewRefreshPrices は、利用者の保有銘柄を held から読み、その終値を fetch で取得して保存する RefreshPrices を返す。
func NewRefreshPrices(fetch *FetchClosingPrices, held repository.HeldStockRepository) *RefreshPrices {
	return &RefreshPrices{fetch: fetch, held: held}
}

// Execute は、利用者 userID の保有銘柄の終値を取得して保存する。他の利用者だけが保有する銘柄は取得しない（要件16）。
// エラーを返すのは、保有銘柄を読めなかったときだけで、そのときは取得元を呼ばない（どの銘柄を取得すればよいか分からないため）。
// 1銘柄の失敗や ctx の取り消しは、エラーにせず FetchResult の Failed に入る（ExecuteCodes の決まり）。
func (c *RefreshPrices) Execute(ctx context.Context, userID uuid.UUID) (FetchResult, error) {
	codes, err := c.held.ListHeldStockCodesOf(ctx, userID)
	if err != nil {
		return FetchResult{}, fmt.Errorf("利用者の保有している銘柄を読めません: %w", err)
	}
	return c.fetch.ExecuteCodes(ctx, codes), nil
}
