package model

import (
	"errors"
	"fmt"
	"math/big"
)

// TradeSide は、売買の別（買いか売り）。値は、DB の trades.side の CHECK（'buy'・'sell'）と同じ文字列。
type TradeSide string

const (
	TradeSideBuy  TradeSide = "buy"  // 買い
	TradeSideSell TradeSide = "sell" // 売り
)

// Trade は、1件の売買（要件2）。利用者は持たない（取り込みが、取り込む先の利用者を別に受け取る）。
// 価格は 0.01 単位の整数で持つ（円もドルも。ClosingPrice と同じ。例: 2,500円は 250000、150.25ドルは 15025）。
type Trade struct {
	StockCode       string
	Side            TradeSide
	Quantity        int64
	PriceHundredths int64
	TradeDate       string // "YYYY-MM-DD"
}

var (
	// ErrQuantityNotPositive は、株数が1未満であること（要件3）。
	ErrQuantityNotPositive = errors.New("quantity not positive")
	// ErrQuantityTooLarge は、株数が1,000万株以上であること（要件3）。
	ErrQuantityTooLarge = errors.New("quantity too large")
	// ErrPriceNotPositive は、価格が0以下であること（要件3）。
	ErrPriceNotPositive = errors.New("price not positive")
	// ErrPriceTooLarge は、価格が1億以上であること（要件3）。
	ErrPriceTooLarge = errors.New("price too large")
	// ErrPriceTooFine は、価格が通貨の精度（円は小数第1位、ドルは小数第2位）に合わないこと（要件3）。
	ErrPriceTooFine = errors.New("price too fine")
	// ErrFutureTradeDate は、取引日が取り込んだ日（日本時間）より後であること（要件3）。
	ErrFutureTradeDate = errors.New("future trade date")
)

const (
	// maxQuantity は、株数の上限（要件3: 1,000万株未満）。
	maxQuantity = 9_999_999
	// priceLimit は、価格の上限（要件3: 1億未満。この値は含まない）。
	priceLimit = 100_000_000
	// hundredthsPerUnit は、価格の 0.01 単位が1円（1ドル）でいくつか。
	hundredthsPerUnit = 100
)

// CheckQuantity は、株数 q が1以上9,999,999以下なら nil、そうでなければ ErrQuantityNotPositive か ErrQuantityTooLarge を返す（要件3）。
func CheckQuantity(q int64) error {
	switch {
	case q < 1:
		return ErrQuantityNotPositive
	case q > maxQuantity:
		return ErrQuantityTooLarge
	default:
		return nil
	}
}

// CheckPrice は、価格 price が0より大きく1億未満なら nil、そうでなければ ErrPriceNotPositive か ErrPriceTooLarge を返す（要件3）。
// 通貨ごとの精度は確かめない（Currency.PriceHundredths が確かめる）。
func CheckPrice(price *big.Rat) error {
	switch {
	case price.Sign() <= 0:
		return ErrPriceNotPositive
	case price.Cmp(new(big.Rat).SetInt64(priceLimit)) >= 0:
		return ErrPriceTooLarge
	default:
		return nil
	}
}

// checkPriceHundredths は、0.01 単位の価格 hundredths について、CheckPrice と同じ範囲（0より大きく1億未満）を確かめる。
func checkPriceHundredths(hundredths int64) error {
	switch {
	case hundredths <= 0:
		return ErrPriceNotPositive
	case hundredths >= priceLimit*hundredthsPerUnit:
		return ErrPriceTooLarge
	default:
		return nil
	}
}

// CheckTradeDate は、取引日 date が今日 today 以前なら nil、今日より後なら ErrFutureTradeDate を返す（要件3）。
// date と today は "YYYY-MM-DD"（年は4桁）。同じ形の文字列は、文字列の比較が日付の比較と一致する。
// 日付として実在するかは確かめない（読み取りの側が、書き方と実在を確かめてから渡す）。
func CheckTradeDate(date, today string) error {
	if date > today {
		return ErrFutureTradeDate
	}
	return nil
}

// NewTrade は、銘柄 stock の売買を作る（要件2・要件3）。tradeDate と today は "YYYY-MM-DD"。
// 株数・価格（0.01 単位）・取引日の順に確かめ、最初に当たった誤りを返す。価格の精度は stock の通貨で確かめる。
func NewTrade(stock Stock, side TradeSide, quantity, priceHundredths int64, tradeDate, today string) (Trade, error) {
	if side != TradeSideBuy && side != TradeSideSell {
		return Trade{}, fmt.Errorf("売買の別 %q は buy か sell にしてください", side)
	}
	if err := CheckQuantity(quantity); err != nil {
		return Trade{}, err
	}
	if err := checkPriceHundredths(priceHundredths); err != nil {
		return Trade{}, err
	}
	if !stock.Currency.AcceptsPrice(priceHundredths) {
		return Trade{}, ErrPriceTooFine
	}
	if err := CheckTradeDate(tradeDate, today); err != nil {
		return Trade{}, err
	}
	return Trade{
		StockCode:       stock.Code,
		Side:            side,
		Quantity:        quantity,
		PriceHundredths: priceHundredths,
		TradeDate:       tradeDate,
	}, nil
}
