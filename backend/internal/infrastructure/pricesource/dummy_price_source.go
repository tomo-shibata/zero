// Package pricesource は、株価の取得元（command.PriceSource）の実装を持つ。
// 取得元を差し替えるときは、ここに新しい実装を足し、cmd/api と cmd/fetch-prices の組み立てを変える（要件 NFR-1、プラン 6章の判断4）。
package pricesource

import (
	"context"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// dummyTradingDate は、ダミーの終値の取引日（要件「成功基準」のダミー価格の表）。
const dummyTradingDate = "2026-10-06"

// DummyPriceSource は、外部と通信しない、開発・テスト用のダミーの取得元（要件 NFR-2）。
// 本番の取得元が決まるまで使う。銘柄コードごとに決めた固定の終値と取引日を返し、決めていない銘柄は
// command.ErrPriceUnavailable を返す。ダミーであることを名前で示す（NFR-2）。画面にはダミーであることを出さない。
type DummyPriceSource struct {
	// pricesTenths は、銘柄コードごとの固定の終値（0.1円単位）。取引日はどれも dummyTradingDate。
	pricesTenths map[string]int64
}

// DummyPriceSource が command.PriceSource を満たすことを、コンパイル時に確かめる。
var _ command.PriceSource = (*DummyPriceSource)(nil)

// NewDummyPriceSource は、要件「成功基準」のダミー価格の表のとおりの終値を返す DummyPriceSource を返す。
//   - 7203（トヨタ自動車）: 3,000円・2026-10-06
//   - 6758（ソニーグループ）: 3,500円・2026-10-06
//   - それ以外（9984 など）: 取得できない（command.ErrPriceUnavailable）
func NewDummyPriceSource() *DummyPriceSource {
	return &DummyPriceSource{pricesTenths: map[string]int64{
		"7203": 30000,
		"6758": 35000,
	}}
}

// FetchClosingPrice は、銘柄 stockCode の固定の終値を返す。決めていない銘柄なら command.ErrPriceUnavailable を返す。
// 通信も待ちもしないので、ctx は見ない（取り消し済みの ctx でも、決めたとおりの値を返す。プラン 4.4）。
// 読むだけなので、複数の goroutine から同時に呼んでもよい。
func (s *DummyPriceSource) FetchClosingPrice(_ context.Context, stockCode string) (model.ClosingPrice, error) {
	priceTenths, ok := s.pricesTenths[stockCode]
	if !ok {
		return model.ClosingPrice{}, command.ErrPriceUnavailable
	}
	return model.NewClosingPrice(stockCode, dummyTradingDate, priceTenths)
}
