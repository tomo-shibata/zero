package model

import (
	"cmp"
	"slices"
)

// Oversell は、多すぎる売り（要件9）。Index は FindOversells に渡したスライスの添字。
type Oversell struct {
	Index      int
	Held, Sold int64 // 売りの直前の保有数量と、売る株数
}

// FindOversells は、trades を銘柄ごとに「取引日の昇順 → スライスの順」にたどり（要件8）、
// 売る株数が直前の保有数量より多い売りを、銘柄ごとに最初の1件だけ、Index の昇順で返す（要件9）。
// 多すぎる売りがなければ空（nil）を返す。
func FindOversells(trades []Trade) []Oversell {
	order := make([]int, len(trades))
	for i := range order {
		order[i] = i
	}
	// 同じ取引日はスライスの順（登録した順）のままにするため、添字でも比べる。
	slices.SortFunc(order, func(a, b int) int {
		return cmp.Or(cmp.Compare(trades[a].TradeDate, trades[b].TradeDate), cmp.Compare(a, b))
	})

	held := make(map[string]int64)
	reported := make(map[string]bool)
	var oversells []Oversell
	for _, i := range order {
		t := trades[i]
		switch t.Side {
		case TradeSideBuy:
			held[t.StockCode] += t.Quantity
		case TradeSideSell:
			if reported[t.StockCode] {
				continue
			}
			if t.Quantity > held[t.StockCode] {
				reported[t.StockCode] = true
				oversells = append(oversells, Oversell{Index: i, Held: held[t.StockCode], Sold: t.Quantity})
				continue
			}
			held[t.StockCode] -= t.Quantity
		}
	}
	slices.SortFunc(oversells, func(a, b Oversell) int { return cmp.Compare(a.Index, b.Index) })
	return oversells
}
