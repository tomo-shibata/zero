// Package model は、Command 側（状態を変える）が通すモデルと、その整合性のルールを持つ。
// 保存する値は、ここのコンストラクタを通して作ることで、ルールに合わない値を保存しようとしない。
package model

import (
	"errors"
	"fmt"
	"time"
)

// ClosingPrice は、ある銘柄のある取引日の終値（要件 FR-8、FR-15）。
// 終値は 0.1円単位の整数で持つ。金額に浮動小数点を使わないため（プラン 4.1、6章の判断1）。
type ClosingPrice struct {
	StockCode   string // 銘柄コード
	TradingDate string // 取引日（YYYY-MM-DD）
	PriceTenths int64  // 終値（0.1円単位。例: 30000 は 3,000.0円）
}

// NewClosingPrice は、銘柄 code の取引日 tradingDate（YYYY-MM-DD）の終値 priceTenths（0.1円単位）を返す。
// 終値が0以下ならエラーにする（プラン 4.4。DB の stock_prices の CHECK（close_price > 0。プラン 4.2）と同じ決まり）。
//
// 銘柄コードが空のときと、取引日が YYYY-MM-DD の実在する日付でないときもエラーにする。
// 取得元（PriceSource）の実装はこれを通して終値を作るので、取得元が壊れた値を返しても、
// 保存する前にここで止められるようにするため（取引日を誤って別の日として保存しない）。
func NewClosingPrice(code, tradingDate string, priceTenths int64) (ClosingPrice, error) {
	if code == "" {
		return ClosingPrice{}, errors.New("終値の銘柄コードが空です")
	}
	if _, err := time.Parse(time.DateOnly, tradingDate); err != nil {
		return ClosingPrice{}, fmt.Errorf("終値の取引日 %q は YYYY-MM-DD の日付ではありません", tradingDate)
	}
	if priceTenths <= 0 {
		return ClosingPrice{}, fmt.Errorf("終値は0より大きくしてください（0.1円単位で %d）", priceTenths)
	}
	return ClosingPrice{StockCode: code, TradingDate: tradingDate, PriceTenths: priceTenths}, nil
}
