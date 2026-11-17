package integration

import (
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 株式分割の要件（docs/requirements/stock-split.md）の基準7（要件1・要件4）・基準9（要件8）と、
// プラン（docs/plans/stock-split/plan.md）4.8 の PR① の境界（HoldingsReader.ListStockSplits）、5.1 の基準7・基準9 の行、5.2 の要件4 の補助。
// 組み立ては list_holdings_test.go の listHoldingsOf（read.NewHoldingsReader(pool) → query.NewListHoldings → Execute）。
// 分割は insertStockSplit（stock_splits_fixtures_test.go）、売買履歴・終値は SQL の INSERT で入れる。
// 金額は 0.01 単位の整数（66670 は 666.7円、100000 は 1,000円、30000000 は 300,000円）。

// 基準7（要件1・要件4）: 7203 の分割が「1→3 2026-03-31」。Aの売買履歴が 7203「買 2,000円 100株 2026-01-10」、
// Bの売買履歴が 7203「買 3,000円 10株 2026-02-10」→ Aの 7203 は 300株・666.7円、Bの 7203 は 30株・1,000円
// （分割は全利用者で共有し、ある銘柄の分割は、その銘柄を持つすべての利用者の計算に使う）。
func Test基準7_分割は全利用者の計算に使い保有数量と取得価格に反映する(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStockSplit(t, pool, "7203", "2026-03-31", 3)
	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "2000", "2026-01-10")
	insertTrade(t, pool, seedUserB, "7203", "buy", 10, "3000", "2026-02-10")

	gotA := onlyHoldingOf(t, listHoldingsOf(t, pool, seedUserA).Holdings, "7203")
	gotB := onlyHoldingOf(t, listHoldingsOf(t, pool, seedUserB).Holdings, "7203")

	if gotA.Quantity != 300 || gotA.AcquisitionPriceHundredths != 66670 {
		t.Errorf("Aの 7203: got %d株・%d, want 300株・66670（666.7円）", gotA.Quantity, gotA.AcquisitionPriceHundredths)
	}
	if gotB.Quantity != 30 || gotB.AcquisitionPriceHundredths != 100000 {
		t.Errorf("Bの 7203: got %d株・%d, want 30株・100000（1,000円）", gotB.Quantity, gotB.AcquisitionPriceHundredths)
	}
}

// 要件4 の補助（5.2）: Aの 7203 が「買 2,000円 100株 2026-01-10」「買 2,000円 100株 2026-02-10」、分割「1→3 2026-03-31」→ 600株・666.7円。
// ListStockSplits が分割を売買履歴の件数だけ重ねて返すと、分割を2回かけて 1,800株になってしまう（分割1件は1回だけ返す）。
func Test要件4_売買履歴が何件あっても分割は1回だけ反映する(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStockSplit(t, pool, "7203", "2026-03-31", 3)
	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "2000", "2026-01-10")
	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "2000", "2026-02-10")

	got := onlyHoldingOf(t, listHoldingsOf(t, pool, seedUserA).Holdings, "7203")

	if got.Quantity != 600 || got.AcquisitionPriceHundredths != 66670 {
		t.Errorf("Aの 7203: got %d株・%d, want 600株・66670（分割を重ねると 1,800株になる）", got.Quantity, got.AcquisitionPriceHundredths)
	}
}

// assertUnpricedHolding は、h の現在の価格・価格の基準日・評価額・含み損益・率がすべて nil であることを確かめる。
func assertUnpricedHolding(t *testing.T, h dto.Holding) {
	t.Helper()
	if h.CurrentPriceHundredths != nil || h.PriceDate != nil || h.ValuationHundredths != nil ||
		h.UnrealizedGainHundredths != nil || h.UnrealizedGainRateTenths != nil {
		t.Errorf("%s: 価格なしのはずが値がある: %+v", h.Code, h)
	}
}

// 基準9（要件8）: 7203 の分割が「1→3 2026-10-07」、Aの売買履歴が 7203「買 2,000円 100株 2026-01-10」で、
// 7203 の最新の終値が 3,000円（2026-10-06。分割の日より前）→ 7203 は 300株・666.7円で、現在の価格・基準日・評価額・含み損益は nil。
// 円の合計は評価額 0・価格を取得できない銘柄を含みません（7203 は円のただ1つの銘柄なので、古い終値で計算した 90000000 を合計に足す誤りを見分ける）。
// 続けて 7203 の終値 1,000円（2026-10-07。分割の日と同じ日）を保存して取り直すと、現在の価格 1,000円・基準日 2026-10-07・評価額 300,000円で、
// 円の合計は 300,000円・注記なし。
func Test基準9_最新の終値が分割の日より前なら価格なしとし分割の日と同じ日の終値からは使う(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStockSplit(t, pool, "7203", "2026-10-07", 3)
	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "2000", "2026-01-10")
	insertClosePrice(t, pool, "7203", "2026-10-06", "3000")

	before := listHoldingsOf(t, pool, seedUserA)

	h := onlyHoldingOf(t, before.Holdings, "7203")
	if h.Quantity != 300 || h.AcquisitionPriceHundredths != 66670 {
		t.Errorf("7203: got %d株・%d, want 300株・66670（666.7円）", h.Quantity, h.AcquisitionPriceHundredths)
	}
	assertUnpricedHolding(t, h)
	if len(before.Totals) != 1 {
		t.Fatalf("Totals の件数: got %d, want 1（円だけ）", len(before.Totals))
	}
	total := before.Totals[0]
	if total.Currency != dto.CurrencyJPY || total.ValuationHundredths != 0 || !total.ExcludesUnpriced ||
		total.UnrealizedGainHundredths != nil || total.UnrealizedGainRateTenths != nil {
		t.Errorf("Totals[0]: got %+v, want 円・評価額 0・注記あり・含み損益と率は nil", total)
	}

	insertClosePrice(t, pool, "7203", "2026-10-07", "1000")

	after := listHoldingsOf(t, pool, seedUserA)

	h = onlyHoldingOf(t, after.Holdings, "7203")
	if h.CurrentPriceHundredths == nil || *h.CurrentPriceHundredths != 100000 {
		t.Errorf("現在の価格: got %v, want 100000（1,000円）", h.CurrentPriceHundredths)
	}
	if h.PriceDate == nil || *h.PriceDate != "2026-10-07" {
		t.Errorf("価格の基準日: got %v, want 2026-10-07", h.PriceDate)
	}
	if h.ValuationHundredths == nil || *h.ValuationHundredths != 30000000 {
		t.Errorf("評価額: got %v, want 30000000（300,000円）", h.ValuationHundredths)
	}
	if len(after.Totals) != 1 {
		t.Fatalf("Totals の件数: got %d, want 1（円だけ）", len(after.Totals))
	}
	if tot := after.Totals[0]; tot.Currency != dto.CurrencyJPY || tot.ValuationHundredths != 30000000 || tot.ExcludesUnpriced {
		t.Errorf("Totals[0]: got %+v, want 円・評価額 30000000・注記なし", tot)
	}
}
