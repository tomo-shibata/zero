package model

import (
	"cmp"
	"math/big"
	"slices"
)

// replay は、trades を、splits も混ぜて銘柄ごとに「日付の昇順 → 同じ日は分割が先 → スライスの順」にたどり（要件6）、
// 銘柄ごとの最後の保有数量を返す。FindOversells と HeldStockCodes が同じ順でたどるための共通の部分。
//
// 買いでは保有数量に株数を足し、売りでは引き、分割では保有数量に分割後の株数を掛ける（要件4）。
// 売買は、足し引きする前に visit を呼ぶ。visit が false を返した売買は、保有数量に反映しない（多すぎる売りを数えないため）。
// 保有数量は math/big で数える（分割を重ねると int64 を超えうるため）。trades にない銘柄の分割は使わない。
// 戻り値には、trades にあるすべての銘柄を入れる（保有数量が負の銘柄も、そのまま）。
func replay(trades []Trade, splits []StockSplit, visit func(i int, t Trade, held *big.Int) bool) map[string]*big.Int {
	held := make(map[string]*big.Int)
	for _, t := range trades {
		if held[t.StockCode] == nil {
			held[t.StockCode] = new(big.Int)
		}
	}

	// events は、売買（trade が true。index は trades の添字）と分割（index は splits の添字）を並べてたどる順にしたもの。
	type event struct {
		date  string
		trade bool
		index int
	}
	events := make([]event, 0, len(trades)+len(splits))
	for i, t := range trades {
		events = append(events, event{date: t.TradeDate, trade: true, index: i})
	}
	for i, s := range splits {
		if held[s.StockCode] != nil {
			events = append(events, event{date: s.SplitDate, trade: false, index: i})
		}
	}
	// 日付は "YYYY-MM-DD" なので、文字列の昇順が日付の古い順になる。同じ日は分割（false）が先、同じ種類はスライスの順。
	slices.SortFunc(events, func(a, b event) int {
		return cmp.Or(cmp.Compare(a.date, b.date), compareBool(a.trade, b.trade), cmp.Compare(a.index, b.index))
	})

	for _, e := range events {
		if !e.trade {
			s := splits[e.index]
			held[s.StockCode].Mul(held[s.StockCode], big.NewInt(s.Ratio))
			continue
		}
		t := trades[e.index]
		q := held[t.StockCode]
		if !visit(e.index, t, q) {
			continue
		}
		switch t.Side {
		case TradeSideBuy:
			q.Add(q, big.NewInt(t.Quantity))
		case TradeSideSell:
			q.Sub(q, big.NewInt(t.Quantity))
		}
	}
	return held
}

// compareBool は、false を true より前にして比べる。
func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a:
		return -1
	default:
		return 1
	}
}
