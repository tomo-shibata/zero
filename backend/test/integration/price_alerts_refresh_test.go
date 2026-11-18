package integration

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/read"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/write"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/pricesource"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/router"
	"github.com/tomo-shibata/zero/backend/internal/interface/handler"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 価格の変動の通知の要件（docs/requirements/price-alerts.md）の基準18・基準19・基準22（今すぐ更新を通る判定）と、
// プラン（docs/plans/price-alerts/plan.md）5.1 の基準18・基準19・基準22 の行（price_alerts_refresh_test.go）。
// 組み立ては price_refresh_api_test.go の newRefreshAPI と同じで、価格の取得処理に判定（WithAlertEvaluator）をつないだ router
// （POST /api/price-refresh）。取得元はダミー（pricesource.NewDummyPriceSource。7203=3,000円・6758=3,500円・AAPL=180.00ドル、どれも 2026-10-06）。

// newRefreshAPIWithAlerts は、pool の DB と取得元 src を使い、判定 evaluator をつないだ今すぐ更新の API を組み立てる。ログイン中の利用者は userID。
func newRefreshAPIWithAlerts(pool *pgxpool.Pool, userID string, src command.PriceSource, evaluator command.PriceAlertEvaluator) http.Handler {
	fetch := newFetchClosingPrices(src, pool).WithAlertEvaluator(evaluator)
	return router.New(router.Deps{
		QueryHandler:        handler.NewQueryHandler(query.NewListHoldings(read.NewHoldingsReader(pool))),
		PriceRefreshHandler: handler.NewPriceRefreshHandler(command.NewRefreshPrices(fetch, write.NewHoldingRepository(pool))),
		DevUserID:           uuid.MustParse(userID),
		AllowedHosts:        []string{allowedHost},
	})
}

// 基準18（要件8）: Aが 7203「買 2,500円 100株」、条件: 7203・3,000円 以上になったら。Bが 7203「買 3,000円 10株」
// → Bが今すぐ更新をダミーの取得元で実行すると、Aに 7203・取引日 2026-10-06 の通知が1件できる（「終値 3,000円」。終値は全利用者で共有なので、誰が更新してもその銘柄に条件を持つ全員の判定をする）。
func Test基準18_誰が今すぐ更新してもその銘柄に条件を持つ全員を判定する(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	condID := insertStockCondition(t, pool, seedUserA, "price", "7203", "above", "3000")
	insertTrade(t, pool, seedUserB, "7203", "buy", 10, "3000", buyDate)
	h := newRefreshAPIWithAlerts(pool, seedUserB, pricesource.NewDummyPriceSource(), newAlertEvaluator(pool))

	rec := servePost(t, h, priceRefreshPath, sameOriginHeaders())

	assertJSONResponse(t, rec, http.StatusOK, `{"saved":["7203"],"failed":[]}`)
	assertNotifications(t, pool, wantNotification{
		conditionID: condID, stockCode: "7203", tradingDate: "2026-10-06", closeHundredths: 300000, details: `{}`,
	})
}

// 基準19（要件18）: Aが 7203「買 2,500円 100株」。7203 の終値（直接）: 2026-10-05 2,800円。条件: 全銘柄・前の取引日から 5% 以上 上がったら
// → Aが今すぐ更新をダミーの取得元で2回実行すると、通知は 2026-10-06 の1件だけ（「前の取引日から +7.1%」。同じ取引日の終値を保存し直しても2件目はできない）。
func Test基準19_同じ取引日の終値を保存し直しても通知は1件だけ(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	insertClosePrice(t, pool, "7203", "2026-10-05", "2800")
	condID := insertAllCondition(t, pool, seedUserA, "prev", "up", "5")
	h := newRefreshAPIWithAlerts(pool, seedUserA, pricesource.NewDummyPriceSource(), newAlertEvaluator(pool))

	for range 2 {
		rec := servePost(t, h, priceRefreshPath, sameOriginHeaders())
		assertJSONResponse(t, rec, http.StatusOK, `{"saved":["7203"],"failed":[]}`)
	}

	assertNotifications(t, pool, wantNotification{
		conditionID: condID, stockCode: "7203", tradingDate: "2026-10-06", closeHundredths: 300000,
		details: `{"rateTenths":71,"basePriceHundredths":280000,"baseDate":"2026-10-05"}`,
	})
}

// failingAlertEvaluator は、command.PriceAlertEvaluator を実装した、必ずエラーを返すテスト用の判定（判定で必ずエラーになるようにする差し替え）。呼ばれた回数を数える。
type failingAlertEvaluator struct{ calls atomic.Int32 }

func (e *failingAlertEvaluator) Execute(context.Context, []model.ClosingPrice) error {
	e.calls.Add(1)
	return errors.New("fake: 判定に必ず失敗する")
}

// 基準22（要件21）: Aが 7203「買 2,500円 100株」、条件: 7203・3,000円 以上になったら。判定で必ずエラーになるようにする
// → Aが今すぐ更新をダミーの取得元で実行すると、7203 の終値 3,000円・2026-10-06 は保存され、結果は「1銘柄の価格を更新しました」（200 と saved がちょうど ["7203"]、failed が空）で、通知はない。
func Test基準22_判定が失敗しても終値の保存と今すぐ更新の結果は変わらない(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	insertStockCondition(t, pool, seedUserA, "price", "7203", "above", "3000")
	evaluator := &failingAlertEvaluator{}
	h := newRefreshAPIWithAlerts(pool, seedUserA, pricesource.NewDummyPriceSource(), evaluator)

	rec := servePost(t, h, priceRefreshPath, sameOriginHeaders())

	assertJSONResponse(t, rec, http.StatusOK, `{"saved":["7203"],"failed":[]}`)
	assertStockPrices(t, pool, []storedClosePrice{
		{stockCode: "7203", tradingDate: "2026-10-06", priceHundredths: 300000},
	})
	assertNotifications(t, pool)
	if got := evaluator.calls.Load(); got != 1 {
		t.Errorf("判定の呼び出し: got %d回, want 1回（保存の後に判定を呼ぶ）", got)
	}
}
