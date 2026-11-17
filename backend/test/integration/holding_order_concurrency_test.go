package integration

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/write"
)

// 仕様: カード表示のプラン（docs/plans/holdings-cards/plan.md）4.3「消すとき」と判断6
// （削除と保存は利用者の行をロックする。保存と削除が同時に動いても、消した銘柄の並びの行が残らないように）。
// 要件28（消した銘柄の前の位置を覚えない）が、同時に動くときにも成り立つことを確かめる（ステップ6 のエッジの指摘2）。
// 待ち合わせは、利用者の行のロックを待っている接続があること（pg_stat_activity の wait_event_type = 'Lock'）で確かめる（固定の時間は待たない）。

const (
	// lockWaitLimit は、ロック待ちの接続が現れるのを待つ上限。
	lockWaitLimit = 10 * time.Second
	// finishLimit は、待っていた操作が終わるのを待つ上限。
	finishLimit = 10 * time.Second
)

// waitForLockWaiter は、この DB の中に、ロックを待っている接続が現れるまで待つ。
// 接続プールの接続（テストの DB は2本まで）は、ロックを持つ側と待つ側で使い切るので、確かめには別の接続を張る。
func waitForLockWaiter(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	conn, err := pgx.ConnectConfig(t.Context(), pool.Config().ConnConfig)
	if err != nil {
		t.Fatalf("確かめ用の接続を張れない: %v", err)
	}
	defer func() { _ = conn.Close(t.Context()) }()

	deadline := time.Now().Add(lockWaitLimit)
	for {
		var waiting int
		if err := conn.QueryRow(t.Context(),
			"SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'",
		).Scan(&waiting); err != nil {
			t.Fatalf("ロック待ちの接続を数えられない: %v", err)
		}
		if waiting > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%v たってもロックを待つ接続が現れない（保存・削除が利用者の行をロックしていない）", lockWaitLimit)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// receiveRecorder は、ch から応答を limit まで待って受け取る。
func receiveRecorder(t *testing.T, ch <-chan *httptest.ResponseRecorder, what string) *httptest.ResponseRecorder {
	t.Helper()
	select {
	case rec := <-ch:
		return rec
	case <-time.After(finishLimit):
		t.Fatalf("%s が %v たっても終わらない", what, finishLimit)
		return nil
	}
}

// 補助（判断6）: 別のトランザクションが利用者の行をロックしている間に、7203 を含む並びの保存が始まって待たされ、
// その間に 7203 の売買履歴と並びの行が消えてコミットされると、保存は 400 になり、7203 の並びの行は残らない（保存が古い保有を元に並びを入れない）。
func Testカード表示の補助_保存が待っている間に銘柄が削除されたら保存は400で消した銘柄の並びの行は残らない(t *testing.T) {
	t.Parallel()
	pool := givenCardsPool(t)
	holdBuys(t, pool, seedUserA, "6758", "7203", "AAPL")
	insertHoldingOrder(t, pool, seedUserA, "AAPL", "7203", "6758")
	api := newHoldingOrderAPI(pool, seedUserA)

	// 別のトランザクション（削除の役）が、利用者の行をロックしたまま。
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("トランザクションを始められない: %v", err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	if _, err := tx.Exec(t.Context(), "SELECT 1 FROM users WHERE id = $1::uuid FOR UPDATE", seedUserA); err != nil {
		t.Fatalf("利用者の行をロックできない: %v", err)
	}

	// 7203 を含む並びの保存を始める。ロックを待たされる。
	saved := make(chan *httptest.ResponseRecorder, 1)
	go func() { saved <- servePutOrder(t, api, putOrderBody("7203", "6758", "AAPL"), sameOriginHeaders()) }()
	waitForLockWaiter(t, pool)
	select {
	case <-saved:
		t.Fatal("利用者の行をロックされている間に、保存が終わってしまった（ロックを待っていない）")
	default:
	}

	// 待っている間に、7203 の売買履歴と並びの行を消してコミットする（銘柄の削除と同じ変更）。
	if _, err := tx.Exec(t.Context(), "DELETE FROM trades WHERE user_id = $1::uuid AND stock_code = '7203'", seedUserA); err != nil {
		t.Fatalf("7203 の売買履歴を消せない: %v", err)
	}
	if _, err := tx.Exec(t.Context(), "DELETE FROM holding_orders WHERE user_id = $1::uuid AND stock_code = '7203'", seedUserA); err != nil {
		t.Fatalf("7203 の並びの行を消せない: %v", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("コミットできない: %v", err)
	}

	rec := receiveRecorder(t, saved, "保存")
	assertJSONResponse(t, rec, http.StatusBadRequest, badRequestJSON)
	if got := readHoldingOrder(t, pool, seedUserA); slices.Contains(got, "7203") {
		t.Errorf("DB の並び: got %v, 消した 7203 の行が残っている", got)
	}
	if got, want := readHoldingOrder(t, pool, seedUserA), []string{"AAPL", "6758"}; !slices.Equal(got, want) {
		t.Errorf("DB の並び: got %v, want %v（保存は何も変えない）", got, want)
	}
}

// 補助（判断6）: 逆に、保存が先に利用者の行のロックを取って並びを入れる途中（7203 を含む並びをまだ入れていない）に、7203 の削除が始まって待たされ、
// 保存がコミットした後に削除が進むと、7203 の並びの行は残らない（保存が入れた 7203 の行を、削除が消す）。
func Testカード表示の補助_削除が待っている間に保存が終わっても消した銘柄の並びの行は残らない(t *testing.T) {
	t.Parallel()
	pool := givenCardsPool(t)
	holdBuys(t, pool, seedUserA, "6758", "7203", "AAPL")
	insertHoldingOrder(t, pool, seedUserA, "AAPL", "7203", "6758")
	api := newHoldingOrderAPI(pool, seedUserA)
	repo := write.NewHoldingOrderRepository(pool)

	// 保存を、ロックを取った後の build の中で止める（本物の保存が、ロックを持ったまま待つ）。
	locked := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseSave := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseSave() // テストが途中で失敗しても、保存を止めたままにしない。
	saveErr := make(chan error, 1)
	go func() {
		saveErr <- repo.ReplaceHoldingOrder(t.Context(), uuid.MustParse(seedUserA), func(held []string) (model.HoldingOrder, error) {
			close(locked)
			<-release
			return model.NewHoldingOrder([]string{"7203", "6758", "AAPL"}, held)
		})
	}()
	select {
	case <-locked:
	case <-time.After(finishLimit):
		t.Fatalf("保存がロックを取って並びを決める段階に %v たっても来ない", finishLimit)
	}

	// 保存がロックを持っている間に、7203 の削除を始める。ロックを待たされる。
	deleted := make(chan *httptest.ResponseRecorder, 1)
	go func() { deleted <- serveDelete(t, api, holdingDeletePath("7203"), sameOriginHeaders()) }()
	waitForLockWaiter(t, pool)
	select {
	case <-deleted:
		t.Fatal("保存がロックを持っている間に、削除が終わってしまった（ロックを待っていない）")
	default:
	}

	// 保存を進めて終わらせる。続いて削除が進む。
	releaseSave()
	select {
	case err := <-saveErr:
		if err != nil {
			t.Fatalf("保存: %v", err)
		}
	case <-time.After(finishLimit):
		t.Fatalf("保存が %v たっても終わらない", finishLimit)
	}
	rec := receiveRecorder(t, deleted, "削除")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("削除: got %d, want 204\n本文: %s", rec.Code, rec.Body.Bytes())
	}

	if got, want := readHoldingOrder(t, pool, seedUserA), []string{"6758", "AAPL"}; !slices.Equal(got, want) {
		t.Errorf("DB の並び: got %v, want %v（削除した 7203 の行は残らない）", got, want)
	}
	if got, want := listedCodes(t, api), []string{"6758", "AAPL"}; !slices.Equal(got, want) {
		t.Errorf("一覧の並び: got %v, want %v", got, want)
	}
}
