package integration

import (
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/write"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 要件（docs/requirements/price-update.md）の要件16（ログイン中の利用者が保有する全銘柄を取得する。他の利用者だけが保有する銘柄は含めない）と、
// プラン（docs/plans/price-update/plan.md）5.2 の PR① の ListHeldStockCodesOf の行
// 「Aの 7203 が2件でも1つ、130A と 1301 は 1301・130A の順（COLLATE "C"）、保有のない利用者は空のスライス（nil でない）」。
// 境界は 4.4: write.NewHoldingRepository(pool) の ListHeldStockCodesOf(ctx, userID)（重複なし・昇順（COLLATE "C"）。0件なら空のスライス）。
// 保有データは SQL の INSERT で入れ（holdings_fixtures_test.go）、結果は戻り値で確かめる。

// insertStock は、銘柄マスタ（stocks）に、test.sql にない銘柄を1件足す。銘柄名はテスト用の仮の名前。
func insertStock(t *testing.T, pool *pgxpool.Pool, code string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO stocks (code, name) VALUES ($1::text, $2::text)",
		code, "テスト銘柄"+code,
	); err != nil {
		t.Fatalf("銘柄マスタに %q を足せない: %v", code, err)
	}
}

// listHeldStockCodesOf は、pool の DB で、利用者 userID の保有銘柄を ListHeldStockCodesOf で読む。
func listHeldStockCodesOf(t *testing.T, pool *pgxpool.Pool, userID string) []string {
	t.Helper()
	got, err := write.NewHoldingRepository(pool).ListHeldStockCodesOf(t.Context(), uuid.MustParse(userID))
	if err != nil {
		t.Fatalf("ListHeldStockCodesOf(%s): got err %v, want nil", userID, err)
	}
	return got
}

// 補助 要件16（5.2）: ListHeldStockCodesOf の戻り値。
func Test補助_要件16_ListHeldStockCodesOfはその利用者の保有銘柄を重複なしCOLLATE_Cの昇順で返し保有がなければ空のスライスを返す(t *testing.T) {
	t.Parallel()

	// Aの 7203 が2件（数量違い）でも1つ。Bだけが保有する 6758 は含めない。
	t.Run("Aの7203が2件でも1つで他の利用者の銘柄は含めない", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertHolding(t, pool, seedUserA, "7203", 100, "2000")
		insertHolding(t, pool, seedUserA, "7203", 300, "3000")
		insertHolding(t, pool, seedUserB, "6758", 10, "3200")

		got := listHeldStockCodesOf(t, pool, seedUserA)

		if want := []string{"7203"}; !slices.Equal(got, want) {
			t.Errorf("ListHeldStockCodesOf(A): got %v, want %v", got, want)
		}
	})

	// 130A と 1301（どちらも銘柄マスタにない銘柄なので足す）。1301 より先に 130A を保有データに入れる（入れた順が昇順と逆）。
	t.Run("130Aと1301は1301_130Aの順", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertStock(t, pool, "130A")
		insertStock(t, pool, "1301")
		insertHolding(t, pool, seedUserA, "130A", 10, "1000")
		insertHolding(t, pool, seedUserA, "1301", 10, "1000")

		got := listHeldStockCodesOf(t, pool, seedUserA)

		if want := []string{"1301", "130A"}; !slices.Equal(got, want) {
			t.Errorf("ListHeldStockCodesOf(A): got %v, want %v", got, want)
		}
	})

	// 補助（プランの 130A・1301 に足したデータ）: COLLATE "C"（バイト順）では大文字 130A（0x41）が小文字 130a（0x61）より前。
	// DB の既定の照合順序（en_US.utf8）のままなら 130a が 130A より前に来る（db/query/holdings.sql の ListHeldStockCodes のコメント）。
	// プランの 130A と 1301 の組は、どちらの照合順序でも同じ順になるので、これを足して COLLATE "C" を見分ける。
	t.Run("大文字の130Aは小文字の130aより前（バイト順）", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertStock(t, pool, "130a")
		insertStock(t, pool, "130A")
		insertHolding(t, pool, seedUserA, "130a", 10, "1000")
		insertHolding(t, pool, seedUserA, "130A", 10, "1000")

		got := listHeldStockCodesOf(t, pool, seedUserA)

		if want := []string{"130A", "130a"}; !slices.Equal(got, want) {
			t.Errorf("ListHeldStockCodesOf(A): got %v, want %v", got, want)
		}
	})

	// 保有のない利用者（B）は空のスライス。nil ではない（0件のとき nil を返す実装を見分ける）。Aは保有している。
	t.Run("保有のない利用者は空のスライスでnilではない", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertHolding(t, pool, seedUserA, "7203", 100, "2000")

		got := listHeldStockCodesOf(t, pool, seedUserB)

		if got == nil {
			t.Errorf("ListHeldStockCodesOf(B): got nil, want 空のスライス")
		}
		if len(got) != 0 {
			t.Errorf("ListHeldStockCodesOf(B): got %v, want 空", got)
		}
	})
}
