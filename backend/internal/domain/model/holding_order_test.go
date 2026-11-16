package model_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// 仕様: カード表示の要件定義（docs/requirements/holdings-cards.md）の要件31 と、プラン（docs/plans/holdings-cards/plan.md）
// 4.6 PR②・5.3 の「NewHoldingOrder（単体）」。NewHoldingOrder(codes, heldCodes) は、codes が heldCodes（保有数量が1以上の銘柄コード）と
// ちょうど同じ集まり（重複なし）のときだけ並びを返し、違えば ErrInvalidHoldingOrder を返す。Codes() は並びの写し。

func Testカード表示の補助_NewHoldingOrder_保有している銘柄の集まりと同じなら並びを返す(t *testing.T) {
	t.Parallel()
	// heldCodes の順と違う順でも、集まりが同じなら受け付ける。並びは codes の順のまま。
	o, err := model.NewHoldingOrder([]string{"AAPL", "7203", "6758"}, []string{"6758", "7203", "AAPL"})

	if err != nil {
		t.Fatalf("NewHoldingOrder: got err %v, want nil", err)
	}
	if want := []string{"AAPL", "7203", "6758"}; !slices.Equal(o.Codes(), want) {
		t.Errorf("Codes: got %v, want %v", o.Codes(), want)
	}
}

func Testカード表示の補助_NewHoldingOrder_一致しない集まりはErrInvalidHoldingOrderで返す(t *testing.T) {
	t.Parallel()
	held := []string{"6758", "7203", "AAPL"}
	tests := []struct {
		name  string
		codes []string
	}{
		{"重複がある（長さは同じ）", []string{"7203", "7203", "AAPL"}},
		{"重複があり、足りない銘柄もある", []string{"7203", "7203", "6758", "AAPL"}},
		{"足りない", []string{"7203", "6758"}},
		{"余分な銘柄がある（保有していない銘柄）", []string{"7203", "6758", "AAPL", "9984"}},
		{"同じ数だが1つ違う銘柄に替わっている", []string{"7203", "6758", "9984"}},
		{"空", []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := model.NewHoldingOrder(tc.codes, held)

			if !errors.Is(err, model.ErrInvalidHoldingOrder) {
				t.Errorf("NewHoldingOrder(%v, %v): got err %v, want ErrInvalidHoldingOrder", tc.codes, held, err)
			}
		})
	}
}

func Testカード表示の補助_NewHoldingOrder_保有が0件で並びも空なら並びを返す(t *testing.T) {
	t.Parallel()

	o, err := model.NewHoldingOrder([]string{}, []string{})

	if err != nil {
		t.Fatalf("NewHoldingOrder: got err %v, want nil（両方空なら集まりが一致する）", err)
	}
	if len(o.Codes()) != 0 {
		t.Errorf("Codes: got %v, want 空", o.Codes())
	}
}

func Testカード表示の補助_NewHoldingOrder_保有が0件で銘柄が送られたらErrInvalidHoldingOrder(t *testing.T) {
	t.Parallel()

	_, err := model.NewHoldingOrder([]string{"7203"}, []string{})

	if !errors.Is(err, model.ErrInvalidHoldingOrder) {
		t.Errorf("got err %v, want ErrInvalidHoldingOrder", err)
	}
}

// Codes は並びの写し。返したスライスを書き換えても、並びは変わらない。
func Testカード表示の補助_HoldingOrderのCodesは写しを返す(t *testing.T) {
	t.Parallel()
	o, err := model.NewHoldingOrder([]string{"AAPL", "7203"}, []string{"7203", "AAPL"})
	if err != nil {
		t.Fatalf("NewHoldingOrder: %v", err)
	}

	got := o.Codes()
	if len(got) != 2 {
		t.Fatalf("Codes: got %v, want 2件", got)
	}
	got[0] = "書き換え"

	if want := []string{"AAPL", "7203"}; !slices.Equal(o.Codes(), want) {
		t.Errorf("Codes: got %v, want %v（返した写しの書き換えが並びに届いた）", o.Codes(), want)
	}
}
