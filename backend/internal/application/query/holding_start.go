package query

import (
	"math/big"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// HoldingStartDate は、同じ銘柄の売買履歴 trades と分割 splits から、今の保有が始まった日を返す（価格の変動の通知の要件12）。
// 売買履歴と分割を、たどる順（日付の昇順 → 同じ日は分割が先 → id の昇順）にたどり、保有数量が0から1以上になった最後の買いの取引日。
// 売り切って買い直したら、買い直した日。保有数量が0（売買履歴がない・売り切った）なら ok が false。
// trades・splits の並びには頼らない。
func HoldingStartDate(trades []dto.TradeRow, splits []dto.StockSplitRow) (date string, ok bool) {
	quantity := new(big.Int)
	for _, e := range traceEvents(trades, splits) {
		if !e.isTrade {
			quantity.Mul(quantity, big.NewInt(e.ratio))
			continue
		}
		qty := big.NewInt(e.trade.Quantity)
		switch e.trade.Side {
		case dto.TradeSideBuy:
			if quantity.Sign() == 0 {
				date = e.date
			}
			quantity.Add(quantity, qty)
		case dto.TradeSideSell:
			quantity.Sub(quantity, qty)
			if quantity.Sign() < 0 {
				// 売りすぎの売買履歴。保有数量が決まらないので、保有なしとして返す（計算の失敗は calculateStock が返す）。
				return "", false
			}
		}
	}
	if quantity.Sign() == 0 {
		return "", false
	}
	return date, true
}
