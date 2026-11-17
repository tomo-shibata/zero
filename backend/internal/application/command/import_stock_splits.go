package command

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
)

// SplitOversell は、分割を入れ替えると出る多すぎる売り1件（株式分割の要件12）。
type SplitOversell struct {
	UserID     uuid.UUID
	StockCode  string
	TradeDate  string // "YYYY-MM-DD"
	Held, Sold int64
}

// SplitValidationError は、売買履歴との関係の誤り（株式分割の要件12）。
// Oversells は利用者の ID（文字列）の昇順 → 銘柄コードの昇順で、1人の1銘柄に1件。
type SplitValidationError struct{ Oversells []SplitOversell }

// Error は、誤りの件数を返す。利用者に見せる文言は、interface 層が Oversells から作る。
func (e *SplitValidationError) Error() string {
	return fmt.Sprintf("分割と売買履歴の関係の誤りが %d 件あります", len(e.Oversells))
}

// ImportStockSplits は、分割の CSV で登録済みの分割を入れ替えるユースケース（株式分割の要件9〜12）。
type ImportStockSplits struct {
	repo repository.StockSplitImportRepository
}

// NewImportStockSplits は、repo で取り込む ImportStockSplits を返す。
func NewImportStockSplits(repo repository.StockSplitImportRepository) *ImportStockSplits {
	return &ImportStockSplits{repo: repo}
}

// Execute は、splits（1件以上）で登録済みの分割を入れ替え、入れた件数を返す。
// 入れ替えた後の分割で、どれかの利用者の売買履歴に多すぎる売りが出るなら、*SplitValidationError を返して何も変えない（件数は0）。
// splits が空のときはエラーにする（登録済みの分割を全部消すだけの取り込みにしないため。要件13 の「取り込む行がありません」）。
func (u *ImportStockSplits) Execute(ctx context.Context, splits []model.StockSplit) (int, error) {
	if len(splits) == 0 {
		return 0, errors.New("取り込む分割がありません")
	}
	check := func(all []repository.UserTrades) error {
		if oversells := findSplitOversells(all, splits); len(oversells) > 0 {
			return &SplitValidationError{Oversells: oversells}
		}
		return nil
	}
	if err := u.repo.ReplaceStockSplits(ctx, splits, check); err != nil {
		return 0, err
	}
	return len(splits), nil
}

// findSplitOversells は、利用者ごとに、splits を混ぜた保有数量で多すぎる売りを探し、利用者の ID（文字列）の昇順 → 銘柄コードの昇順で返す。
func findSplitOversells(all []repository.UserTrades, splits []model.StockSplit) []SplitOversell {
	var oversells []SplitOversell
	for _, user := range all {
		for _, o := range model.FindOversells(user.Trades, splits) {
			oversells = append(oversells, SplitOversell{
				UserID:    user.UserID,
				StockCode: user.Trades[o.Index].StockCode,
				TradeDate: user.Trades[o.Index].TradeDate,
				Held:      o.Held,
				Sold:      o.Sold,
			})
		}
	}
	slices.SortStableFunc(oversells, func(a, b SplitOversell) int {
		return cmp.Or(cmp.Compare(a.UserID.String(), b.UserID.String()), cmp.Compare(a.StockCode, b.StockCode))
	})
	return oversells
}
