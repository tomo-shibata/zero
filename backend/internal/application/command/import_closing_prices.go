package command

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"slices"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
)

// PriceImportRow は、書き方を確かめ済みの CSV の行（終値は0より大きく1億未満）。Line は CSV のファイルの行番号（見出しが1行目）。
type PriceImportRow struct {
	Line      int
	StockCode string
	Price     *big.Rat
}

// PriceImportSkip は、銘柄マスタにない銘柄コードのため登録しない行（要件6）。
type PriceImportSkip struct {
	Line      int
	StockCode string
}

// PriceTooFine は、通貨の精度に合わない終値の行（要件7。円は小数第1位、ドルは小数第2位まで）。
type PriceTooFine struct {
	Line     int
	Currency model.Currency // その銘柄の、銘柄マスタの通貨
}

// PriceImportPlan は、Classify の結果。どれも行の順。
type PriceImportPlan struct {
	Prices  []model.ClosingPrice // 登録する終値（取引日は Classify の tradingDate）
	Skipped []PriceImportSkip    // 銘柄マスタにない行（要件6）
	TooFine []PriceTooFine       // 通貨の精度に合わない行（要件7）
}

// ImportClosingPrices は、価格の CSV の登録のユースケース。銘柄マスタを読んで行を分け（Classify）、終値をまとめて保存する（Save）。
type ImportClosingPrices struct {
	repo   repository.ClosingPriceImportRepository
	alerts PriceAlertEvaluator // 保存した終値の価格の変動の通知の判定。nil なら判定しない
}

// NewImportClosingPrices は、repo を使う ImportClosingPrices を返す。
func NewImportClosingPrices(repo repository.ClosingPriceImportRepository) *ImportClosingPrices {
	return &ImportClosingPrices{repo: repo}
}

// Classify は、銘柄マスタの通貨を読んで rows を分ける。tradingDate は "YYYY-MM-DD"。何も書き込まない。
// 銘柄マスタにない行は、通貨が分からないので桁を確かめずに Skipped にする。ある行は、通貨の精度に合えば Prices、合わなければ TooFine にする。
func (u *ImportClosingPrices) Classify(ctx context.Context, rows []PriceImportRow, tradingDate string) (PriceImportPlan, error) {
	codes := make([]string, 0, len(rows))
	for _, row := range rows {
		if !slices.Contains(codes, row.StockCode) {
			codes = append(codes, row.StockCode)
		}
	}
	currencies, err := u.repo.FindCurrencies(ctx, codes)
	if err != nil {
		return PriceImportPlan{}, err
	}

	var plan PriceImportPlan
	for _, row := range rows {
		currency, ok := currencies[row.StockCode]
		if !ok {
			plan.Skipped = append(plan.Skipped, PriceImportSkip{Line: row.Line, StockCode: row.StockCode})
			continue
		}
		hundredths, err := currency.PriceHundredths(row.Price)
		if errors.Is(err, model.ErrPriceTooFine) {
			plan.TooFine = append(plan.TooFine, PriceTooFine{Line: row.Line, Currency: currency})
			continue
		}
		if err != nil {
			return PriceImportPlan{}, fmt.Errorf("%d行目の終値を読めません: %w", row.Line, err)
		}
		price, err := model.NewClosingPrice(row.StockCode, tradingDate, hundredths)
		if err != nil {
			return PriceImportPlan{}, fmt.Errorf("%d行目の終値を作れません: %w", row.Line, err)
		}
		plan.Prices = append(plan.Prices, price)
	}
	return plan, nil
}

// WithAlertEvaluator は、保存した終値を価格の変動の通知の判定に渡す e をつなぐ（価格の変動の通知のプラン 4.4）。
// Save は、保存（SaveAll）のコミットが成功した後に、保存した終値をまとめて1回渡す。e が nil なら判定しない（今までと同じ）。
// 判定が失敗しても Save は成功のままで、エラーはログに出すだけ（要件21）。
func (u *ImportClosingPrices) WithAlertEvaluator(e PriceAlertEvaluator) *ImportClosingPrices {
	u.alerts = e
	return u
}

// Save は、prices を SaveAll で保存する。prices が0件なら何もしない（要件9）。
func (u *ImportClosingPrices) Save(ctx context.Context, prices []model.ClosingPrice) error {
	if len(prices) == 0 {
		return nil
	}
	if err := u.repo.SaveAll(ctx, prices); err != nil {
		return err
	}
	evaluateAlerts(ctx, u.alerts, prices)
	return nil
}
