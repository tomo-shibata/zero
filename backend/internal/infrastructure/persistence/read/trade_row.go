package read

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// newTradeRow は、DB から読んだ売買履歴1件の列から、終値を付けない dto.TradeRow を作る。
// 売買の種類と通貨は知っているものだけを通し、価格は 0.01 単位の整数、取引日は "YYYY-MM-DD" にする。
func newTradeRow(id int64, code, name, currencyText, sideText string, quantity int64, priceNumeric pgtype.Numeric, tradeDate pgtype.Date) (dto.TradeRow, error) {
	side := dto.TradeSide(sideText)
	if side != dto.TradeSideBuy && side != dto.TradeSideSell {
		return dto.TradeRow{}, fmt.Errorf("知らない売買の種類 %q です", sideText)
	}
	currency := dto.Currency(currencyText)
	if currency != dto.CurrencyJPY && currency != dto.CurrencyUSD {
		return dto.TradeRow{}, fmt.Errorf("知らない通貨 %q です", currencyText)
	}
	price, err := hundredthsFromNumeric(priceNumeric)
	if err != nil {
		return dto.TradeRow{}, fmt.Errorf("価格: %w", err)
	}
	date, err := formatDate(tradeDate)
	if err != nil {
		return dto.TradeRow{}, fmt.Errorf("取引日: %w", err)
	}
	return dto.TradeRow{
		ID:              id,
		Code:            code,
		Name:            name,
		Currency:        currency,
		Side:            side,
		Quantity:        quantity,
		PriceHundredths: price,
		TradeDate:       date,
	}, nil
}
