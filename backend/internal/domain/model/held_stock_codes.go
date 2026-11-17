package model

import (
	"maps"
	"math/big"
	"slices"
)

// HeldStockCodes は、FindOversells と同じ順に trades と splits をたどり（trade_replay.go）、最後の保有数量が1以上の銘柄コードを、
// 重複なし・昇順（Go の文字列比較）で返す。保有している銘柄の判定の正（プラン 4.3。DB の「買い − 売り」では分割を数えられない）。
// 途中で保有数量が負になる売り（取り込みが入れないもの）があっても止めず、最後の値で決める。なければ空のスライス（nil ではない）。
func HeldStockCodes(trades []Trade, splits []StockSplit) []string {
	held := replay(trades, splits, func(int, Trade, *big.Int) bool { return true })
	codes := []string{}
	for _, code := range slices.Sorted(maps.Keys(held)) {
		if held[code].Sign() > 0 {
			codes = append(codes, code)
		}
	}
	return codes
}
