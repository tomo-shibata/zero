package query_test

import (
	"slices"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// 仕様: カード表示のプラン（docs/plans/holdings-cards/plan.md）4.3「読む」・4.6 PR②・5.3 の「ApplyHoldingOrder（単体）」
// （要件26・要件27）。ApplyHoldingOrder(holdings, order) は、銘柄コードの昇順の holdings を order の順に並べ直した新しいスライスを返す。
// order にない銘柄は後ろに銘柄コードの昇順で、order にあって holdings にない銘柄は無視する。holdings は変えない。

// holdingsOf は、銘柄コード codes の行（コードだけ入れた dto.Holding）を、その順に並べたスライスにする。
func holdingsOf(codes ...string) []dto.Holding {
	hs := make([]dto.Holding, 0, len(codes))
	for _, c := range codes {
		hs = append(hs, dto.Holding{Code: c})
	}
	return hs
}

// codesOfHoldings は、一覧の銘柄コードを、並びの順に返す。
func codesOfHoldings(hs []dto.Holding) []string {
	codes := make([]string, 0, len(hs))
	for _, h := range hs {
		codes = append(codes, h.Code)
	}
	return codes
}

// 銘柄コードの昇順（文字列として）は 6758、7203、9984、AAPL（数字が英字より先）。
func Testカード表示の補助_ApplyHoldingOrder_並びの決まり(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		order []string
		want  []string
	}{
		{"空の並びなら昇順のまま（要件26）", []string{}, []string{"6758", "7203", "9984", "AAPL"}},
		{"並びが nil でも昇順のまま", nil, []string{"6758", "7203", "9984", "AAPL"}},
		{"全部ある並びはその順になる", []string{"AAPL", "9984", "6758", "7203"}, []string{"AAPL", "9984", "6758", "7203"}},
		{"一部だけある並びは、その銘柄が先で、残りは昇順で後ろ（要件27）", []string{"AAPL"}, []string{"AAPL", "6758", "7203", "9984"}},
		{"新しい銘柄が2つ（6758・9984）あるときは、その中で昇順に末尾へ並べる（要件27）", []string{"7203", "AAPL"}, []string{"7203", "AAPL", "6758", "9984"}},
		{"並びにあって一覧にない銘柄は無視する", []string{"MSFT", "AAPL", "1301", "7203", "6758", "9984"}, []string{"AAPL", "7203", "6758", "9984"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in := holdingsOf("6758", "7203", "9984", "AAPL")

			got := codesOfHoldings(query.ApplyHoldingOrder(in, tc.order))

			if !slices.Equal(got, tc.want) {
				t.Errorf("並び: got %v, want %v", got, tc.want)
			}
		})
	}
}

// holdings は変えず、新しいスライスを返す。行の中身（コード以外の値）も、そのまま運ぶ。
func Testカード表示の補助_ApplyHoldingOrder_元のスライスを変えず行の中身を保つ(t *testing.T) {
	t.Parallel()
	in := []dto.Holding{
		{Code: "6758", Name: "ソニーグループ", Quantity: 10},
		{Code: "7203", Name: "トヨタ自動車", Quantity: 100},
		{Code: "AAPL", Name: "Apple", Quantity: 5},
	}

	got := query.ApplyHoldingOrder(in, []string{"AAPL", "6758", "7203"})

	if !slices.Equal(codesOfHoldings(in), []string{"6758", "7203", "AAPL"}) {
		t.Errorf("元のスライスの並び: got %v, want 変わらない [6758 7203 AAPL]", codesOfHoldings(in))
	}
	if len(got) != 3 || got[0].Code != "AAPL" || got[0].Name != "Apple" || got[0].Quantity != 5 {
		t.Fatalf("先頭の行: got %+v, want AAPL / Apple / 5株（行の中身は変えずに運ぶ）", got)
	}
	got[0].Name = "書き換え"
	if in[2].Name != "Apple" {
		t.Errorf("返したスライスを書き換えたら元のスライスも変わった: %+v", in[2])
	}
}

// ListHoldings.Execute は、読み取りの並びで一覧を並べ直す（要件26・要件27。4.3「読む」）。
// 売買履歴は 6758・7203・AAPL の買いで、保存した並びは AAPL、7203。→ AAPL、7203、6758。
func Testカード表示の補助_Executeは保存した並びで一覧を並べ直す(t *testing.T) {
	t.Parallel()
	reader := &fixedRowsReader{
		rows: []dto.TradeRow{
			sony().buy(1, 10, 400000, "2026-01-10"),
			toyota().buy(2, 100, 250000, "2026-01-10"),
			apple().buy(3, 10, 15025, "2026-01-10"),
		},
		order: []string{"AAPL", "7203"},
	}

	got, err := query.NewListHoldings(reader).Execute(t.Context(), userA)

	if err != nil {
		t.Fatalf("Execute: got err %v, want nil", err)
	}
	if want := []string{"AAPL", "7203", "6758"}; !slices.Equal(codesOfHoldings(got.Holdings), want) {
		t.Errorf("一覧の並び: got %v, want %v", codesOfHoldings(got.Holdings), want)
	}
}
