package model

import (
	"errors"
	"slices"
)

// ErrInvalidHoldingOrder は、並びの銘柄が、保有している銘柄（保有数量が1以上の銘柄）とちょうど同じ集まりでないこと
// （重複、足りない、保有していない銘柄。要件31）。
var ErrInvalidHoldingOrder = errors.New("並びの銘柄が保有している銘柄と一致しません")

// HoldingOrder は、保有株式一覧の並び（要件23）。銘柄コードを、表示する順に持つ。
// NewHoldingOrder でだけ作るので、保有している銘柄とちょうど同じ集まり（重複なし）であることが保たれる。
type HoldingOrder struct {
	codes []string
}

// NewHoldingOrder は、codes が heldCodes（保有数量が1以上の銘柄コード）とちょうど同じ集まり（重複なし）のときだけ、
// codes の順の並びを返す。heldCodes の順は問わない。違えば ErrInvalidHoldingOrder を返す（要件31）。
func NewHoldingOrder(codes, heldCodes []string) (HoldingOrder, error) {
	if len(codes) != len(heldCodes) {
		return HoldingOrder{}, ErrInvalidHoldingOrder
	}
	// 同じ長さで、どちらにも重複がなければ、集まりが同じ＝並べ替えたら等しい。
	sortedCodes := slices.Sorted(slices.Values(codes))
	sortedHeld := slices.Sorted(slices.Values(heldCodes))
	if !slices.Equal(sortedCodes, sortedHeld) {
		return HoldingOrder{}, ErrInvalidHoldingOrder
	}
	// 並べ替えて等しくても、両方に同じ重複があれば（heldCodes は重複しない前提だが）通ってしまうので、重複を確かめる。
	if len(slices.Compact(sortedCodes)) != len(sortedCodes) {
		return HoldingOrder{}, ErrInvalidHoldingOrder
	}
	return HoldingOrder{codes: slices.Clone(codes)}, nil
}

// Codes は、並びの銘柄コードを、表示する順に返す（写し）。
func (o HoldingOrder) Codes() []string {
	return slices.Clone(o.codes)
}
