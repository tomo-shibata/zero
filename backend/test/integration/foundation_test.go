package integration

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/infrastructure/router"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: プラン（docs/plans/stock-holdings/plan.md）5.1「補助のテスト（バックエンド）」の「基盤」の行（PR①）。
// 要件の AC はない。確かめるのは次のとおり。
//   - GET /healthz が 200 と本文 "ok" を返す（4.4 の router.New）。
//   - testdb.New が DB「zero_test_<ランダム>」を作り、マイグレーション → db/seed/test.sql を流す（4.4 の testdb.New）。
//   - testdb で作った2つの DB に並列で users を書き込んでも、互いに見えない
//     （テストルール 4「テストごとに DB を独立させ、並列に動いても干渉させない」）。

// db/seed/test.sql が入れる利用者（プラン 4.4「テストデータ」）。
const (
	seedUserA = "00000000-0000-0000-0000-00000000000a"
	seedUserB = "00000000-0000-0000-0000-00000000000b"
)

// プラン 4.4: router.New は GET /healthz に 200・本文 "ok" を返す。
// PR① では Deps の項目がまだないので、空の Deps で組み立てる。
func Test基盤_GET_healthzは200と本文okを返す(t *testing.T) {
	t.Parallel()

	h := router.New(router.Deps{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("ステータス: got %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "ok" {
		t.Errorf("本文: got %q, want %q", got, "ok")
	}
}

// プラン 4.4: testdb.New は DB「zero_test_<ランダム>」を作る。
func Test基盤_testdbのDB名はzero_test_で始まる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)

	var name string
	if err := pool.QueryRow(t.Context(), "SELECT current_database()").Scan(&name); err != nil {
		t.Fatalf("DB 名を読めない: %v", err)
	}
	const prefix = "zero_test_"
	if !strings.HasPrefix(name, prefix) || len(name) == len(prefix) {
		t.Errorf("DB 名: got %q, want %q の後にランダムな部分が続く名前", name, prefix)
	}
}

// プラン 4.4: testdb.New はマイグレーションの後に db/seed/test.sql を流す。
// 「テストデータ」の表のとおり、test.sql が入れる利用者は A・B の2人だけ。
func Test基盤_testdbのDBにはseedの利用者AとBが入っている(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)

	got := userIDs(t, pool)
	want := []string{seedUserA, seedUserB}
	if !slices.Equal(got, want) {
		t.Errorf("users の id: got %v, want %v", got, want)
	}
}

// プラン 5.1「基盤」: testdb で作った2つの DB に並列で users を書き込み、互いに見えない。
//
// 2つのサブテストは、両方の INSERT が済むまで待ち合わせてから確かめる。
// こうすると、確かめる時点で2つの DB が同時にあり、もう一方の INSERT も済んでいる
// （片方が終わってからもう片方が動く、という順になって確認が素通りすることがない）。
func Test基盤_testdbで並列に作った2つのDBは互いに見えない(t *testing.T) {
	t.Parallel()

	// 利用者C・D は seed にない利用者。サブテストごとに片方だけを INSERT する。
	writers := []*parallelWriter{
		{name: "利用者Cを書き込むDB", userID: "00000000-0000-0000-0000-00000000000c", inserted: make(chan struct{})},
		{name: "利用者Dを書き込むDB", userID: "00000000-0000-0000-0000-00000000000d", inserted: make(chan struct{})},
	}
	for i := range writers {
		me, other := writers[i], writers[1-i]
		t.Run(me.name, func(t *testing.T) {
			t.Parallel()
			// 途中で失敗して抜けても、相手を待たせ続けない。
			defer me.markInserted()

			pool := testdb.New(t)
			if _, err := pool.Exec(t.Context(), "INSERT INTO users (id) VALUES ($1)", me.userID); err != nil {
				t.Fatalf("利用者 %s を INSERT できない: %v", me.userID, err)
			}
			me.markInserted()
			other.waitInserted(t)

			got := userIDs(t, pool)
			if !slices.Contains(got, me.userID) {
				t.Errorf("自分が入れた利用者 %s が見えない。users の id: %v", me.userID, got)
			}
			if slices.Contains(got, other.userID) {
				t.Errorf("もう一方の DB に入れた利用者 %s が見えている（DB が独立していない）。users の id: %v", other.userID, got)
			}
		})
	}
}

// parallelWriter は、並列のサブテストの1つが書き込む利用者と、その INSERT が済んだことの知らせを持つ。
type parallelWriter struct {
	name     string
	userID   string
	inserted chan struct{}
	once     sync.Once
}

// parallelWaitTimeout は、相手のサブテストの INSERT を待つ上限。
// -parallel 1 だと2つのサブテストが同時に動けず待ち続けるので、時間で区切って失敗させる。
const parallelWaitTimeout = time.Minute

func (w *parallelWriter) markInserted() {
	w.once.Do(func() { close(w.inserted) })
}

// waitInserted は、w のサブテストが INSERT を終える（または失敗して抜ける）まで待つ。
func (w *parallelWriter) waitInserted(t *testing.T) {
	t.Helper()
	select {
	case <-w.inserted:
	case <-time.After(parallelWaitTimeout):
		t.Fatalf("「%s」の INSERT を %v 待っても済まない（-parallel が 1 だと2つのサブテストが同時に動けない）", w.name, parallelWaitTimeout)
	}
}

// userIDs は users の id を、昇順の文字列（小文字の UUID）で返す。
func userIDs(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(), "SELECT id::text FROM users ORDER BY id")
	if err != nil {
		t.Fatalf("users を読めない: %v", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("users を読めない: %v", err)
	}
	return ids
}
