package integration

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: プラン（docs/plans/stock-holdings/plan.md）5.1「補助のテスト（バックエンド）」の「基盤（TC-35）」の行（PR①）。
// 要件の AC はない。確かめるのは次のとおり。
//   - testdb.New で作った DB は、そのテストが終わると pg_database に残っていない
//     （4.4 の testdb.New「終了時に消す」）。

// プラン 5.1「基盤（TC-35）」: 並列にしないサブテストの中で testdb.New を呼び、DB 名を控える。
// サブテストが終わった後、その DB が pg_database に残っていない。
//
// 並列にしないサブテストは、本体とクリーンアップ（t.Cleanup）が済んでから t.Run が戻る。
// なので t.Run の後に確かめれば、testdb の後始末が済んだ後の状態を見られる。
// サブテストを並列にしないので、tparallel の決まりに合わせてこのテスト自体も並列にしない。
func Test基盤_TC35_testdbのDBはテストが終わると消える(t *testing.T) {
	var name string
	t.Run("testdbでDBを作って名前を控える", func(t *testing.T) {
		pool := testdb.New(t)
		if err := pool.QueryRow(t.Context(), "SELECT current_database()").Scan(&name); err != nil {
			t.Fatalf("DB 名を読めない: %v", err)
		}
	})
	if name == "" {
		t.Fatal("サブテストで DB 名を控えられなかった")
	}

	// 確かめるのには、別の testdb の DB への接続を使う（どの DB からでも pg_database は見える）。
	pool := testdb.New(t)

	// 対照: 同じ問い合わせで、いま使っている DB は「ある」と出る（問い合わせが常に「ない」を返す誤りではない）。
	var self string
	if err := pool.QueryRow(t.Context(), "SELECT current_database()").Scan(&self); err != nil {
		t.Fatalf("DB 名を読めない: %v", err)
	}
	if !databaseExists(t, pool, self) {
		t.Fatalf("いま使っている DB %s が pg_database に見つからない（確かめ方が誤っている）", self)
	}

	if databaseExists(t, pool, name) {
		t.Errorf("サブテストが終わった後も DB %s が pg_database に残っている（testdb.New が終了時に DB を消していない）", name)
	}
}

// databaseExists は、名前が name の DB が pg_database にあるかを返す。
func databaseExists(t *testing.T, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(t.Context(),
		"SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", name,
	).Scan(&exists); err != nil {
		t.Fatalf("pg_database を読めない: %v", err)
	}
	return exists
}
