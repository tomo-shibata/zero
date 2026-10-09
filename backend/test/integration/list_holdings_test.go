package integration

import (
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/read"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 要件（docs/requirements/stock-holdings.md）の AC-2（FR-1、FR-17）と、
// プラン（docs/plans/stock-holdings/plan.md）5.1 の AC-2 の行
// （A に 7203、B に 6758 を保有させる。ListHoldings.Execute(A) の結果が 7203 の1行だけ）。
// 組み立ては 4.4 の read.NewHoldingsReader → query.NewListHoldings。

// AC-2（FR-1、FR-17）: 利用者Bが 6758 を保有し、利用者Aは 6758 を保有していない
// → 利用者Aの保有株式一覧に 6758 は含まれない（A の一覧は 7203 の1行だけ）。
func TestAC2_利用者Aの保有株式一覧に利用者Bの保有株式は含まれない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertHolding(t, pool, seedUserA, "7203", 100, "2500")
	insertHolding(t, pool, seedUserB, "6758", 10, "3200")
	q := query.NewListHoldings(read.NewHoldingsReader(pool))

	got, err := q.Execute(t.Context(), uuid.MustParse(seedUserA))
	if err != nil {
		t.Fatalf("利用者Aの一覧を取得できない: %v", err)
	}
	if codes := holdingCodes(got.Holdings); !slices.Equal(codes, []string{"7203"}) {
		t.Errorf("利用者Aの一覧の銘柄コード: got %v, want [7203]（利用者Bの 6758 を含めない）", codes)
	}

	// 対照: 同じ DB で利用者Bの一覧には 6758 が出る（6758 が A の一覧にないのは、
	// 利用者で絞っているからであって、6758 の行が読めないからではないことを確かめる）。
	gotB, err := q.Execute(t.Context(), uuid.MustParse(seedUserB))
	if err != nil {
		t.Fatalf("利用者Bの一覧を取得できない: %v", err)
	}
	if codes := holdingCodes(gotB.Holdings); !slices.Equal(codes, []string{"6758"}) {
		t.Errorf("利用者Bの一覧の銘柄コード: got %v, want [6758]", codes)
	}
}

// holdingCodes は、一覧の行の銘柄コードを並び順のまま返す。
func holdingCodes(holdings []dto.Holding) []string {
	codes := make([]string, 0, len(holdings))
	for _, h := range holdings {
		codes = append(codes, h.Code)
	}
	return codes
}
