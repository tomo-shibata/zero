package model_test

import (
	"slices"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// 仕様: 株式分割の要件（docs/requirements/stock-split.md）の基準1・4・5・6（要件4・要件6）、要件7（多すぎる売りは分割を反映した保有数量で判断する）と、
// プラン（docs/plans/stock-split/plan.md）4.2・4.3・4.8 の PR① の境界（FindOversells(trades, splits)・HeldStockCodes(trades, splits)）、
// 5.1 の基準1・4・5・6（判断2）の行と 5.2（要件6 の同じ日の順、桁あふれ、HeldStockCodes の途中で負になる売り）。
// Query 側（calculate_holdings_split_test.go）と同じ売買履歴と分割を domain に渡し、同じ保有数量になることを確かめる。
// 最後の保有数量 Q に対して、後の日付に「売 Q＋1株」を足すと多すぎる売り（Held が Q・Sold が Q＋1）、「売 Q株」を足すと売り切り。
// buy・sell は trade_test.go の補助（価格は結果に関係しないので固定）。

// splitOf は、銘柄 code の「分割 1→ratio 分割の日 date」を作る。
func splitOf(code, date string, ratio int64) model.StockSplit {
	return model.StockSplit{StockCode: code, SplitDate: date, Ratio: ratio}
}

// laterDate は、これらのテストの売買履歴のどの日付より後の日付（売りの日）。
const laterDate = "2026-11-01"

// splitCase は、保有の計算の例1つ分。trades と splits を渡したときの、最後の保有数量が wantQuantity になる。
type splitCase struct {
	name         string
	trades       []model.Trade
	splits       []model.StockSplit
	wantQuantity int64
}

// splitCases は、要件定義の基準1・4・5・6 の売買履歴と分割（7203。どれも円）。
func splitCases() []splitCase {
	return []splitCase{
		{
			// 200株を分割して 600株。
			name: "基準1_分割の前の2件の買い",
			trades: []model.Trade{
				buy("7203", 100, "2026-01-10"),
				buy("7203", 100, "2026-02-10"),
			},
			splits:       []model.StockSplit{splitOf("7203", "2026-03-31", 3)},
			wantQuantity: 600,
		},
		{
			// 分割で 600株、売 300株で 300株、買 300株で 600株。
			name: "基準1_さらに売りと買いを足す",
			trades: []model.Trade{
				buy("7203", 100, "2026-01-10"),
				buy("7203", 100, "2026-02-10"),
				sell("7203", 300, "2026-05-01"),
				buy("7203", 300, "2026-06-01"),
			},
			splits:       []model.StockSplit{splitOf("7203", "2026-03-31", 3)},
			wantQuantity: 600,
		},
		{
			// 3/30 の買いで 100株、3/31 は分割が先で 200株、その後の買いで 300株。
			name: "基準4_同じ日は分割が先",
			trades: []model.Trade{
				buy("7203", 100, "2026-03-30"),
				buy("7203", 100, "2026-03-31"),
			},
			splits:       []model.StockSplit{splitOf("7203", "2026-03-31", 2)},
			wantQuantity: 300,
		},
		{
			// 分割のときは保有数量0なので何も変わらない。その後の買いで 100株。
			name: "基準5_保有数量0のときの分割",
			trades: []model.Trade{
				buy("7203", 100, "2026-01-10"),
				sell("7203", 100, "2026-02-10"),
				buy("7203", 100, "2026-04-10"),
			},
			splits:       []model.StockSplit{splitOf("7203", "2026-03-31", 2)},
			wantQuantity: 100,
		},
		{
			// 1回目の分割で 200株、買いで 300株、2回目の分割で 1,500株。
			name: "基準6_分割が2回",
			trades: []model.Trade{
				buy("7203", 100, "2026-01-10"),
				buy("7203", 100, "2026-05-01"),
			},
			splits: []model.StockSplit{
				splitOf("7203", "2026-03-31", 2),
				splitOf("7203", "2026-08-01", 5),
			},
			wantQuantity: 1500,
		},
		{
			// 分割の並びが日付の逆順でも同じ値（splits の並びに頼らない）。
			name: "基準6_分割を逆の日付順で渡す",
			trades: []model.Trade{
				buy("7203", 100, "2026-01-10"),
				buy("7203", 100, "2026-05-01"),
			},
			splits: []model.StockSplit{
				splitOf("7203", "2026-08-01", 5),
				splitOf("7203", "2026-03-31", 2),
			},
			wantQuantity: 1500,
		},
		{
			// ほかの銘柄（6758）の分割は使わない。
			name: "基準6_ほかの銘柄の分割を混ぜても値は変わらない",
			trades: []model.Trade{
				buy("7203", 100, "2026-01-10"),
				buy("7203", 100, "2026-05-01"),
			},
			splits: []model.StockSplit{
				splitOf("6758", "2026-03-31", 2),
				splitOf("7203", "2026-08-01", 5),
				splitOf("7203", "2026-03-31", 2),
			},
			wantQuantity: 1500,
		},
	}
}

// withTrade は、trades の末尾に t を足した新しいスライスを返す（元は変えない）。
func withTrade(trades []model.Trade, t model.Trade) []model.Trade {
	return append(slices.Clone(trades), t)
}

// 基準1・4・5・6・要件7: 最後の保有数量 Q に対して、後の日付に「売 Q＋1株」を足すと、多すぎる売りが1件で Held が Q・Sold が Q＋1
// （Index は足した売りの添字）。Q より1株多いだけで検出できるので、分割を反映した保有数量が Q ちょうどであることが分かる。
func Test基準1_4_5_6_FindOversellsは分割を反映した保有数量より1株多い売りを検出する(t *testing.T) {
	t.Parallel()
	for _, tc := range splitCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			trades := withTrade(tc.trades, sell("7203", tc.wantQuantity+1, laterDate))

			got := model.FindOversells(trades, tc.splits)

			want := []model.Oversell{{Index: len(trades) - 1, Held: tc.wantQuantity, Sold: tc.wantQuantity + 1}}
			if !slices.Equal(got, want) {
				t.Errorf("FindOversells: got %+v, want %+v", got, want)
			}
		})
	}
}

// 基準1・4・5・6・要件7: 「売 Q株」（持っている株数ちょうどまで）を足すと、多すぎる売りはなく、売り切りなので HeldStockCodes は空のスライス（nil ではない）。
// 売りを足さなければ HeldStockCodes は ["7203"]。
func Test基準1_4_5_6_FindOversellsとHeldStockCodesは分割を反映した保有数量ちょうどの売りを売り切りとして扱う(t *testing.T) {
	t.Parallel()
	for _, tc := range splitCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			exact := withTrade(tc.trades, sell("7203", tc.wantQuantity, laterDate))
			if got := model.FindOversells(exact, tc.splits); len(got) != 0 {
				t.Errorf("FindOversells(売 %d株): got %+v, want なし", tc.wantQuantity, got)
			}
			soldOut := model.HeldStockCodes(exact, tc.splits)
			if soldOut == nil {
				t.Errorf("HeldStockCodes(売り切り): got nil, want 空のスライス（nil ではない）")
			}
			if len(soldOut) != 0 {
				t.Errorf("HeldStockCodes(売り切り): got %v, want 空", soldOut)
			}

			if got, want := model.HeldStockCodes(tc.trades, tc.splits), []string{"7203"}; !slices.Equal(got, want) {
				t.Errorf("HeldStockCodes(売りなし): got %v, want %v", got, want)
			}
		})
	}
}

// 要件6: 同じ日に分割と売買履歴があるときは、分割を先にたどり、その後に売買履歴をスライスの順（登録した順）にたどる。
// 3/30 に 100株を買い、3/31 に分割 1→2・買 100株・売 300株: 分割で 200株 → 買いで 300株 → 売りで 0株（多すぎる売りはない）。
// 売りを買いより先に並べると、売りの時点で 200株しかないので多すぎる（Held 200・Sold 300）。
func Test要件6_FindOversellsは同じ日に分割を先にたどり売買履歴は渡した順にたどる(t *testing.T) {
	t.Parallel()
	splits := []model.StockSplit{splitOf("7203", "2026-03-31", 2)}

	buyThenSell := []model.Trade{
		buy("7203", 100, "2026-03-30"),
		buy("7203", 100, "2026-03-31"),
		sell("7203", 300, "2026-03-31"),
	}
	if got := model.FindOversells(buyThenSell, splits); len(got) != 0 {
		t.Errorf("FindOversells(分割の日に 買→売): got %+v, want なし（分割 → 買い → 売りで 300株 → 0株）", got)
	}

	sellThenBuy := []model.Trade{
		buy("7203", 100, "2026-03-30"),
		sell("7203", 300, "2026-03-31"),
		buy("7203", 100, "2026-03-31"),
	}
	want := []model.Oversell{{Index: 1, Held: 200, Sold: 300}}
	if got := model.FindOversells(sellThenBuy, splits); !slices.Equal(got, want) {
		t.Errorf("FindOversells(分割の日に 売→買): got %+v, want %+v（分割は先、売買履歴は渡した順）", got, want)
	}

	// 売りを分割の日の前日にすると、分割の前の保有数量（100株）で判断する。
	beforeSplit := []model.Trade{
		buy("7203", 100, "2026-03-30"),
		sell("7203", 101, "2026-03-30"),
	}
	wantBefore := []model.Oversell{{Index: 1, Held: 100, Sold: 101}}
	if got := model.FindOversells(beforeSplit, splits); !slices.Equal(got, wantBefore) {
		t.Errorf("FindOversells(分割の前の売り): got %+v, want %+v", got, wantBefore)
	}
}

// 要件4: 売買履歴にない銘柄の分割は使わない。splits に trades にない銘柄の分割だけがあっても、結果は分割なしと同じ。
func Test要件4_FindOversellsは売買履歴にない銘柄の分割を使わない(t *testing.T) {
	t.Parallel()
	trades := []model.Trade{
		buy("7203", 100, "2026-01-10"),
		sell("7203", 101, "2026-05-01"),
	}
	splits := []model.StockSplit{splitOf("9984", "2026-03-31", 2)}

	got := model.FindOversells(trades, splits)

	want := []model.Oversell{{Index: 1, Held: 100, Sold: 101}}
	if !slices.Equal(got, want) {
		t.Errorf("FindOversells: got %+v, want %+v（9984 の分割は 7203 に響かない）", got, want)
	}
	if got, want := model.HeldStockCodes(trades[:1], splits), []string{"7203"}; !slices.Equal(got, want) {
		t.Errorf("HeldStockCodes: got %v, want %v（trades にない 9984 は含めない）", got, want)
	}
}

// 4.2・5.2「桁あふれ」: 買 9,999,999株と分割 1→1,000 を4回（9,999,999 × 1,000^4 は int64 に収まらない）。
// 「売 1株」を足しても多すぎる売りは出ず、HeldStockCodes に入る（保有数量は math/big で数える）。
func Test桁あふれ_FindOversellsとHeldStockCodesは保有数量がint64を超えても正しく数える(t *testing.T) {
	t.Parallel()
	trades := []model.Trade{
		buy("7203", 9_999_999, "2026-01-10"),
		sell("7203", 1, "2026-06-01"),
	}
	splits := []model.StockSplit{
		splitOf("7203", "2026-02-01", 1000),
		splitOf("7203", "2026-02-02", 1000),
		splitOf("7203", "2026-02-03", 1000),
		splitOf("7203", "2026-02-04", 1000),
	}

	if got := model.FindOversells(trades, splits); len(got) != 0 {
		t.Errorf("FindOversells: got %+v, want なし（int64 であふれて負になり、多すぎる売りと誤らない）", got)
	}
	if got, want := model.HeldStockCodes(trades, splits), []string{"7203"}; !slices.Equal(got, want) {
		t.Errorf("HeldStockCodes: got %v, want %v", got, want)
	}
}

// 4.8・5.2: HeldStockCodes は、最後の保有数量が1以上の銘柄コードを、重複なし・昇順（Go の文字列比較）で返す。
// 売り切った銘柄は含めず、trades にない銘柄の分割は使わない。結果が空なら nil ではなく空のスライス。
func Test補助_HeldStockCodesは保有数量が1以上の銘柄を重複なしで昇順に返す(t *testing.T) {
	t.Parallel()

	t.Run("昇順で重複なし_売り切りは含めない", func(t *testing.T) {
		t.Parallel()
		trades := []model.Trade{
			buy("7203", 100, "2026-01-10"),
			buy("7203", 50, "2026-02-10"),
			buy("130A", 10, "2026-01-10"),
			buy("1301", 10, "2026-01-10"),
			buy("6758", 10, "2026-01-10"),
			sell("6758", 10, "2026-02-10"),
		}
		got := model.HeldStockCodes(trades, nil)
		if want := []string{"1301", "130A", "7203"}; !slices.Equal(got, want) {
			t.Errorf("HeldStockCodes: got %v, want %v（昇順・重複なし・6758 は売り切り）", got, want)
		}
	})

	t.Run("分割を含めた保有数量で決める", func(t *testing.T) {
		t.Parallel()
		// 7203: 買 100株、分割 1→3（300株）、売 200株 → 100株を保有（分割を無視した「買 − 売」は −100）。
		// 6758: 買 100株、分割 1→3（300株）、売 300株 → 売り切り。
		trades := []model.Trade{
			buy("7203", 100, "2026-01-10"),
			sell("7203", 200, "2026-05-01"),
			buy("6758", 100, "2026-01-10"),
			sell("6758", 300, "2026-05-01"),
		}
		splits := []model.StockSplit{
			splitOf("7203", "2026-03-31", 3),
			splitOf("6758", "2026-03-31", 3),
		}
		got := model.HeldStockCodes(trades, splits)
		if want := []string{"7203"}; !slices.Equal(got, want) {
			t.Errorf("HeldStockCodes: got %v, want %v", got, want)
		}
	})

	t.Run("売買履歴が0件なら空のスライスでnilではない", func(t *testing.T) {
		t.Parallel()
		got := model.HeldStockCodes(nil, nil)
		if got == nil {
			t.Errorf("HeldStockCodes(nil, nil): got nil, want 空のスライス")
		}
		if len(got) != 0 {
			t.Errorf("HeldStockCodes(nil, nil): got %v, want 空", got)
		}
	})
}

// 4.8・5.2: HeldStockCodes は、途中で保有数量が負になる売り（取り込みが入れないもの）があっても止めず、最後の値で決める。
// 日付の早い売りで −100株になった後、買 150株で 50株に戻れば含める。買 100株なら 0株なので含めない
// （売りで負になったときに 0 に直す実装では 100株になって誤って含めてしまう）。
func Test補助_HeldStockCodesは途中で保有数量が負になっても止めず最後の値で決める(t *testing.T) {
	t.Parallel()

	back := []model.Trade{
		sell("7203", 100, "2026-01-05"),
		buy("7203", 150, "2026-01-10"),
	}
	if got, want := model.HeldStockCodes(back, nil), []string{"7203"}; !slices.Equal(got, want) {
		t.Errorf("HeldStockCodes(−100株 → 買 150株): got %v, want %v（50株）", got, want)
	}

	zero := []model.Trade{
		sell("7203", 100, "2026-01-05"),
		buy("7203", 100, "2026-01-10"),
	}
	if got := model.HeldStockCodes(zero, nil); len(got) != 0 {
		t.Errorf("HeldStockCodes(−100株 → 買 100株): got %v, want 空（最後は0株）", got)
	}
}
