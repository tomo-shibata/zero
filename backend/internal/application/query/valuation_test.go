package query_test

import (
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
)

// 仕様: 要件（docs/requirements/stock-holdings.md）の AC-10（FR-9）と、
// プラン（docs/plans/stock-holdings/plan.md）5.1 の AC-10 の行（`Valuation(12345, 3) == 3703`）。
// 確かめる関数は 4.4 の Valuation（単価は 0.1円単位の整数、評価額は円の整数）。

// AC-10（FR-9）: 現在の価格 1,234.5円（Tenths で 12345）、保有数量3株
// → 評価額は 3,703円（3,703.5円の1円未満を切り捨て）。
func TestAC10_評価額は現在の価格と保有数量の積の1円未満を切り捨てる(t *testing.T) {
	t.Parallel()

	if got := query.Valuation(12345, 3); got != 3703 {
		t.Errorf("Valuation(12345, 3): got %d, want 3703", got)
	}
}
