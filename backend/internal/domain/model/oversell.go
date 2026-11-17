package model

import (
	"cmp"
	"math/big"
	"slices"
)

// Oversell は、多すぎる売り（要件9）。Index は FindOversells に渡したスライスの添字。
type Oversell struct {
	Index      int
	Held, Sold int64 // 売りの直前の保有数量と、売る株数
}

// FindOversells は、trades（1人の利用者の売買。スライスの順が登録した順）を、splits も混ぜて銘柄ごとに
// 「日付の昇順 → 同じ日は分割が先 → スライスの順」にたどり（株式分割の要件6。trade_replay.go）、
// 売る株数が直前の保有数量より多い売りを、銘柄ごとに最初の1件だけ、Index の昇順で返す（要件9。株式分割の要件7）。
// splits は、どの銘柄のものが混ざっていてもよい（trades にない銘柄の分割は使わない）。
// 保有数量は math/big で数える。Oversell.Held は売る株数（9,999,999以下）より小さいので int64 に収まる。
// 多すぎる売りがなければ空（nil）を返す。
func FindOversells(trades []Trade, splits []StockSplit) []Oversell {
	reported := make(map[string]bool)
	var oversells []Oversell
	replay(trades, splits, func(i int, t Trade, held *big.Int) bool {
		if t.Side != TradeSideSell {
			return true
		}
		if reported[t.StockCode] {
			return false
		}
		if held.Cmp(big.NewInt(t.Quantity)) < 0 {
			reported[t.StockCode] = true
			oversells = append(oversells, Oversell{Index: i, Held: held.Int64(), Sold: t.Quantity})
			return false
		}
		return true
	})
	slices.SortFunc(oversells, func(a, b Oversell) int { return cmp.Compare(a.Index, b.Index) })
	return oversells
}
