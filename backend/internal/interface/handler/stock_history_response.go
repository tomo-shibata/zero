package handler

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

var errNilSplitDivisor = errors.New("割る数がありません")

// stockHistoryResponse は、GET /api/holdings/{code}/history の本文（銘柄ごとの売買履歴の画面のプラン 4.4）。
// 金額（価格・金額・割った価格）は、一覧の単価と同じ形の文字列にする（円は小数1桁、ドルは小数2桁。amount も同じ）。
// 割る数（splitDivisor）は、大きくなりうるので整数の文字列にする。
type stockHistoryResponse struct {
	Stock stockResponse `json:"stock"`
	// Entries は表の並び（新しい順）。売買の行は tradeEntryResponse、分割の行は splitEntryResponse（使わない欄を空の値で出さない）。
	Entries []any                       `json:"entries"`
	Closes  []stockHistoryCloseResponse `json:"closes"`
}

type stockResponse struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
}

// tradeEntryResponse は、表の売買の行（8つの欄だけ）。
type tradeEntryResponse struct {
	Kind          string `json:"kind"`
	Date          string `json:"date"`
	Side          string `json:"side"`
	Quantity      int64  `json:"quantity"`
	Price         string `json:"price"`
	Amount        string `json:"amount"`
	SplitDivisor  string `json:"splitDivisor"`
	AdjustedPrice string `json:"adjustedPrice"`
}

// splitEntryResponse は、表の分割の行（kind・date・ratio だけ）。
type splitEntryResponse struct {
	Kind  string `json:"kind"`
	Date  string `json:"date"`
	Ratio int64  `json:"ratio"`
}

// stockHistoryCloseResponse は、チャートの終値1件。
type stockHistoryCloseResponse struct {
	Date          string `json:"date"`
	Price         string `json:"price"`
	SplitDivisor  string `json:"splitDivisor"`
	AdjustedPrice string `json:"adjustedPrice"`
}

// newStockHistoryResponse は、売買履歴の Read Model を API の本文の形にする。
// 金額の書式にできないとき（円の値が 0.1円の単位でない、知らない通貨）は、黙って切り捨てずにエラーにする（newHoldingsResponse と同じ）。
// 終値が0件でも closes を null ではなく [] にする。
func newStockHistoryResponse(h dto.StockHistory) (stockHistoryResponse, error) {
	entries := make([]any, 0, len(h.Entries))
	for _, e := range h.Entries {
		switch e.Kind {
		case dto.StockHistoryEntrySplit:
			entries = append(entries, splitEntryResponse{Kind: string(e.Kind), Date: e.Date, Ratio: e.Ratio})
		case dto.StockHistoryEntryTrade:
			row, err := newTradeEntryResponse(h.Currency, e)
			if err != nil {
				return stockHistoryResponse{}, fmt.Errorf("売買（%s）: %w", e.Date, err)
			}
			entries = append(entries, row)
		default:
			return stockHistoryResponse{}, fmt.Errorf("知らない行の種類 %q です", e.Kind)
		}
	}

	closes := make([]stockHistoryCloseResponse, 0, len(h.Closes))
	for _, c := range h.Closes {
		price, adjusted, divisor, err := formatPriceAndDivisor(h.Currency, c.PriceHundredths, c.AdjustedPriceHundredths, c.SplitDivisor)
		if err != nil {
			return stockHistoryResponse{}, fmt.Errorf("終値（%s）: %w", c.Date, err)
		}
		closes = append(closes, stockHistoryCloseResponse{Date: c.Date, Price: price, SplitDivisor: divisor, AdjustedPrice: adjusted})
	}

	return stockHistoryResponse{
		Stock:   stockResponse{Code: h.Code, Name: h.Name, Currency: string(h.Currency)},
		Entries: entries,
		Closes:  closes,
	}, nil
}

// newTradeEntryResponse は、表の売買の行 e を API の本文の形にする。
func newTradeEntryResponse(currency dto.Currency, e dto.StockHistoryEntry) (tradeEntryResponse, error) {
	price, adjusted, divisor, err := formatPriceAndDivisor(currency, e.PriceHundredths, e.AdjustedPriceHundredths, e.SplitDivisor)
	if err != nil {
		return tradeEntryResponse{}, err
	}
	amount, err := formatUnitPrice(currency, e.AmountHundredths)
	if err != nil {
		return tradeEntryResponse{}, fmt.Errorf("金額: %w", err)
	}
	return tradeEntryResponse{
		Kind: string(e.Kind), Date: e.Date, Side: string(e.Side), Quantity: e.Quantity,
		Price: price, Amount: amount, SplitDivisor: divisor, AdjustedPrice: adjusted,
	}, nil
}

// formatPriceAndDivisor は、価格・割った価格（0.01 単位）を単価の形の文字列に、割る数を整数の文字列にする。
func formatPriceAndDivisor(currency dto.Currency, price, adjusted int64, divisor *big.Int) (priceText, adjustedText, divisorText string, err error) {
	if divisor == nil {
		return "", "", "", errNilSplitDivisor
	}
	priceText, err = formatUnitPrice(currency, price)
	if err != nil {
		return "", "", "", fmt.Errorf("価格: %w", err)
	}
	adjustedText, err = formatUnitPrice(currency, adjusted)
	if err != nil {
		return "", "", "", fmt.Errorf("割った価格: %w", err)
	}
	return priceText, adjustedText, divisor.String(), nil
}
