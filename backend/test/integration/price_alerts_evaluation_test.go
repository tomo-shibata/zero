package integration

import (
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/write"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 価格の変動の通知の要件（docs/requirements/price-alerts.md）の基準1〜基準17・基準20・基準21（後半）・基準23と、
// プラン（docs/plans/price-alerts/plan.md）5章の冒頭、5.1 の判定の統合の行、5.2 の判定の統合の補助の行。
// 基準18・基準19・基準22 は price_alerts_refresh_test.go、基準56 は price_alerts_performance_test.go。
// 組み立ては price_alerts_fixtures_test.go のとおり（テスト用の取得元を注入した FetchClosingPrices に判定をつなぎ、notifications を SQL で読む）。
// 売買履歴の取引日を書いていないものは 2026-01-10（buyDate）、足した日を書いていない条件は 2026-11-01。「今日」は 2026-11-18。
// 期待値は要件の基準とプランの値をそのまま使い、テストの中で計算しない。金額は 0.01 単位の整数（2,500円は 250000、180.00ドルは 18000）。

// 基準1（要件11・要件16）: Aが 7203「買 2,500円 100株」、条件: 全銘柄・取得価格から 20% 以上 下がったら
// → 2,002円・2026-11-10 の保存で通知はなく（-19.9%）、2,001円・2026-11-11 の保存で取引日 2026-11-11 の通知が1件できる（-20.0%）。
func Test基準1_取得価格から20パーセント下がったとき初めて通知ができる(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	condID := insertAllCondition(t, pool, seedUserA, "cost", "down", "20")

	saveClose(t, pool, "7203", "2026-11-10", 200200)
	assertNotifications(t, pool)

	saveClose(t, pool, "7203", "2026-11-11", 200100)
	assertNotifications(t, pool, wantNotification{
		conditionID: condID, stockCode: "7203", tradingDate: "2026-11-11", closeHundredths: 200100,
		details: `{"rateTenths":-200,"basePriceHundredths":250000}`,
		kind:    "cost", direction: "down", thresholdHundredths: 2000,
	})
}

// 基準2（要件16）: 基準1 と同じ売買と条件で、7203 の終値を 2026-11-10 1,990円、11-11 1,950円、11-12 2,050円、11-13 1,980円の順に保存する
// → 通知は 2026-11-10（-20.4%）と 2026-11-13（-20.8%）の2件だけ（11-11 は前の判定も当てはまり、11-12 は当てはまらない）。
func Test基準2_当てはまらなかった後に当てはまった日だけ通知ができる(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	condID := insertAllCondition(t, pool, seedUserA, "cost", "down", "20")

	saveClose(t, pool, "7203", "2026-11-10", 199000)
	saveClose(t, pool, "7203", "2026-11-11", 195000)
	saveClose(t, pool, "7203", "2026-11-12", 205000)
	saveClose(t, pool, "7203", "2026-11-13", 198000)

	assertNotifications(t, pool,
		wantNotification{conditionID: condID, stockCode: "7203", tradingDate: "2026-11-10", closeHundredths: 199000, details: `{"rateTenths":-204,"basePriceHundredths":250000}`},
		wantNotification{conditionID: condID, stockCode: "7203", tradingDate: "2026-11-13", closeHundredths: 198000, details: `{"rateTenths":-208,"basePriceHundredths":250000}`},
	)
}

// 基準3（要件11）: Aが AAPL「買 150.00ドル 10株」、条件: 全銘柄・取得価格から 20% 以上 上がったら
// → AAPL の終値 180.00ドル・2026-11-10 の保存で通知が1件できる（+20.0%。ドルのまま計算する）。
func Test基準3_ドルの銘柄はドルのまま計算して通知ができる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStock(t, pool, "AAPL", "Apple", currencyUSD)
	insertBuy(t, pool, seedUserA, "AAPL", 10, "150.00")
	condID := insertAllCondition(t, pool, seedUserA, "cost", "up", "20")

	saveClose(t, pool, "AAPL", "2026-11-10", 18000)

	assertNotifications(t, pool, wantNotification{
		conditionID: condID, stockCode: "AAPL", tradingDate: "2026-11-10", closeHundredths: 18000,
		details: `{"rateTenths":200,"basePriceHundredths":15000}`,
	})
}

// 基準4（要件11・要件16）: Aが 7203「買 1,000円 100株」、条件: 全銘柄・取得価格から 20% 以上 下がったら
// → 800円・2026-11-10 の保存で通知が1件（-20.0%）。「買 800円 100株 2026-11-11」を登録して 800円・2026-11-11 を保存しても増えない（取得価格 900円、-11.1%）。
// 720円・2026-11-12 の保存で 2026-11-12 の通知ができ、「取得価格 900円 → 終値 720円」（-20.0%。合わせて2件）。
func Test基準4_買い増しで取得価格が変わったら変わった後の取得価格で判定する(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserA, "7203", 100, "1000")
	condID := insertAllCondition(t, pool, seedUserA, "cost", "down", "20")

	saveClose(t, pool, "7203", "2026-11-10", 80000)
	first := wantNotification{
		conditionID: condID, stockCode: "7203", tradingDate: "2026-11-10", closeHundredths: 80000,
		details: `{"rateTenths":-200,"basePriceHundredths":100000}`,
	}
	assertNotifications(t, pool, first)

	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "800", "2026-11-11")
	saveClose(t, pool, "7203", "2026-11-11", 80000)
	assertNotifications(t, pool, first)

	saveClose(t, pool, "7203", "2026-11-12", 72000)
	assertNotifications(t, pool, first, wantNotification{
		conditionID: condID, stockCode: "7203", tradingDate: "2026-11-12", closeHundredths: 72000,
		details: `{"rateTenths":-200,"basePriceHundredths":90000}`,
	})
}

// 基準5（要件16）: Aが 7203「買 2,500円 100株」、条件: 全銘柄・取得価格から 20% 以上 下がったら。終値 1,990円・2026-11-10 を保存して通知が1件ある
// → 「売 1,990円 100株 2026-11-10」「買 2,000円 100株 2026-11-12」を登録し、終値 1,590円・2026-11-13 を保存する
// → 2026-11-13 の通知ができる（-20.5%。買い直した 2026-11-12 より前の判定は前の判定に数えない）。
func Test基準5_売り切って買い直したら買い直す前の判定は前の判定に数えない(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	condID := insertAllCondition(t, pool, seedUserA, "cost", "down", "20")
	saveClose(t, pool, "7203", "2026-11-10", 199000)
	first := wantNotification{
		conditionID: condID, stockCode: "7203", tradingDate: "2026-11-10", closeHundredths: 199000,
		details: `{"rateTenths":-204,"basePriceHundredths":250000}`,
	}
	assertNotifications(t, pool, first)
	insertTrade(t, pool, seedUserA, "7203", "sell", 100, "1990", "2026-11-10")
	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "2000", "2026-11-12")

	saveClose(t, pool, "7203", "2026-11-13", 159000)

	assertNotifications(t, pool, first, wantNotification{
		conditionID: condID, stockCode: "7203", tradingDate: "2026-11-13", closeHundredths: 159000,
		details: `{"rateTenths":-205,"basePriceHundredths":200000}`,
	})
}

// 基準6（要件12）: Aが 6758「買 3,000円 100株 2026-03-02」「売 3,500円 100株 2026-07-01」「買 3,400円 100株 2026-09-01」。
// 6758 の終値（直接）: 2026-06-01 4,000円、2026-09-01 3,400円、2026-10-01 3,600円。条件: 全銘柄・最高値から 13% 以上 下がったら、全銘柄・最高値から 20% 以上 下がったら
// → 終値 3,100円・2026-11-10 の保存で 13% の条件の通知だけができ、最高値は買い直した 2026-09-01 から数える（3,600円・2026-10-01、-13.9%）。
func Test基準6_最高値は今の保有が始まった日から数える(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertTrade(t, pool, seedUserA, "6758", "buy", 100, "3000", "2026-03-02")
	insertTrade(t, pool, seedUserA, "6758", "sell", 100, "3500", "2026-07-01")
	insertTrade(t, pool, seedUserA, "6758", "buy", 100, "3400", "2026-09-01")
	insertClosePrice(t, pool, "6758", "2026-06-01", "4000")
	insertClosePrice(t, pool, "6758", "2026-09-01", "3400")
	insertClosePrice(t, pool, "6758", "2026-10-01", "3600")
	cond13 := insertAllCondition(t, pool, seedUserA, "peak", "down", "13")
	_ = insertAllCondition(t, pool, seedUserA, "peak", "down", "20")

	saveClose(t, pool, "6758", "2026-11-10", 310000)

	assertNotifications(t, pool, wantNotification{
		conditionID: cond13, stockCode: "6758", tradingDate: "2026-11-10", closeHundredths: 310000,
		details: `{"rateTenths":-139,"basePriceHundredths":360000,"baseDate":"2026-10-01"}`,
	})
}

// 基準7（要件12）: Aが 7203「買 2,500円 100株」。7203 の分割 1→2 2026-09-30。7203 の終値（直接）: 2026-09-29 4,000円、2026-10-01 1,900円。
// 条件: 全銘柄・最高値から 10% 以上 下がったら → 終値 1,750円・2026-11-10 の保存で通知が1件
// （分割の前の 4,000円を 2 で割った 2,000円（2026-09-29）が最高値で、-12.5%）。
func Test基準7_最高値は分割の前の終値を分割の株数で割って比べる(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	insertStockSplit(t, pool, "7203", "2026-09-30", 2)
	insertClosePrice(t, pool, "7203", "2026-09-29", "4000")
	insertClosePrice(t, pool, "7203", "2026-10-01", "1900")
	condID := insertAllCondition(t, pool, seedUserA, "peak", "down", "10")

	saveClose(t, pool, "7203", "2026-11-10", 175000)

	assertNotifications(t, pool, wantNotification{
		conditionID: condID, stockCode: "7203", tradingDate: "2026-11-10", closeHundredths: 175000,
		details: `{"rateTenths":-125,"basePriceHundredths":200000,"baseDate":"2026-09-29"}`,
	})
}

// 基準8（要件13・要件17）: Aが AAPL「買 150.00ドル 10株」。AAPL の終値（直接）: 2026-11-12 180.25ドル。条件: 全銘柄・前の取引日から 5% 以上 動いたら（上がっても下がっても）
// → AAPL の終値を 2026-11-13 191.24ドル、11-16 202.80ドル、11-17 190.00ドル の順に保存する → 通知は3件（+6.1%、+6.0%、-6.3%）。続けて当てはまっても毎回できる。
func Test基準8_前の取引日からは当てはまった取引日ごとに通知ができる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStock(t, pool, "AAPL", "Apple", currencyUSD)
	insertBuy(t, pool, seedUserA, "AAPL", 10, "150.00")
	insertClosePrice(t, pool, "AAPL", "2026-11-12", "180.25")
	condID := insertAllCondition(t, pool, seedUserA, "prev", "either", "5")

	saveClose(t, pool, "AAPL", "2026-11-13", 19124)
	saveClose(t, pool, "AAPL", "2026-11-16", 20280)
	saveClose(t, pool, "AAPL", "2026-11-17", 19000)

	assertNotifications(t, pool,
		wantNotification{
			conditionID: condID, stockCode: "AAPL", tradingDate: "2026-11-13", closeHundredths: 19124,
			details: `{"rateTenths":61,"basePriceHundredths":18025,"baseDate":"2026-11-12"}`,
		},
		wantNotification{
			conditionID: condID, stockCode: "AAPL", tradingDate: "2026-11-16", closeHundredths: 20280,
			details: `{"rateTenths":60,"basePriceHundredths":19124,"baseDate":"2026-11-13"}`,
		},
		wantNotification{
			conditionID: condID, stockCode: "AAPL", tradingDate: "2026-11-17", closeHundredths: 19000,
			details: `{"rateTenths":-63,"basePriceHundredths":20280,"baseDate":"2026-11-16"}`,
		},
	)
}

// 基準9（要件13）: Aが 6758「買 3,000円 100株」。6758 の終値（直接）: 2026-11-06 3,500円。条件: 全銘柄・前の取引日から 5% 以上 上がったら
// → 終値 3,700円・2026-11-13 の保存で通知が1件（+5.7%。日が空いていても前の終値と比べる）。
func Test基準9_前の取引日からは日が空いていても前の終値と比べる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserA, "6758", 100, "3000")
	insertClosePrice(t, pool, "6758", "2026-11-06", "3500")
	condID := insertAllCondition(t, pool, seedUserA, "prev", "up", "5")

	saveClose(t, pool, "6758", "2026-11-13", 370000)

	assertNotifications(t, pool, wantNotification{
		conditionID: condID, stockCode: "6758", tradingDate: "2026-11-13", closeHundredths: 370000,
		details: `{"rateTenths":57,"basePriceHundredths":350000,"baseDate":"2026-11-06"}`,
	})
}

// 基準10（要件13）: Aが 7203「買 2,500円 100株」。7203 の分割 1→2 2026-11-13。7203 の終値（直接）: 2026-11-12 3,000円。
// 条件: 全銘柄・前の取引日から 5% 以上 動いたら → 終値 1,500円・2026-11-13 の保存で通知はない（前の終値を 2 で割って 1,500円と比べ、0.0%）。
func Test基準10_前の終値を分割で割って比べるので分割だけでは通知が出ない(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	insertStockSplit(t, pool, "7203", "2026-11-13", 2)
	insertClosePrice(t, pool, "7203", "2026-11-12", "3000")
	insertAllCondition(t, pool, seedUserA, "prev", "either", "5")

	saveClose(t, pool, "7203", "2026-11-13", 150000)

	assertNotifications(t, pool)
}

// 基準11（要件14）: Aが 6758「買 4,000円 200株」、条件: 6758・含み損が 100,000円 以上になったら
// → 終値 3,505円・2026-11-10 の保存で通知はなく（含み損益 -99,000円）、終値 3,500円・2026-11-11 の保存で通知が1件（含み損益 -100,000円）。
func Test基準11_含み損が値に届いたとき通知ができる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserA, "6758", 200, "4000")
	condID := insertStockCondition(t, pool, seedUserA, "pnl", "6758", "loss", "100000")

	saveClose(t, pool, "6758", "2026-11-10", 350500)
	assertNotifications(t, pool)

	saveClose(t, pool, "6758", "2026-11-11", 350000)
	assertNotifications(t, pool, wantNotification{
		conditionID: condID, stockCode: "6758", tradingDate: "2026-11-11", closeHundredths: 350000,
		details: `{"unrealizedGainHundredths":-10000000,"basePriceHundredths":400000,"quantity":200}`,
	})
}

// 基準12（要件14）: Aが AAPL「買 150.00ドル 10株」、条件: AAPL・含み益が 300.00ドル 以上になったら
// → 終値 180.00ドル・2026-11-10 の保存で通知が1件（含み損益 +300.00ドル）。
func Test基準12_ドルの含み益が値に届いたとき通知ができる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStock(t, pool, "AAPL", "Apple", currencyUSD)
	insertBuy(t, pool, seedUserA, "AAPL", 10, "150.00")
	condID := insertStockCondition(t, pool, seedUserA, "pnl", "AAPL", "gain", "300.00")

	saveClose(t, pool, "AAPL", "2026-11-10", 18000)

	assertNotifications(t, pool, wantNotification{
		conditionID: condID, stockCode: "AAPL", tradingDate: "2026-11-10", closeHundredths: 18000,
		details: `{"unrealizedGainHundredths":30000,"basePriceHundredths":15000,"quantity":10}`,
	})
}

// 基準13（要件15）: Aが 7203「買 2,500円 100株」と 6758「買 3,000円 100株」、条件: 7203・3,000円 以上になったら、条件: 6758・3,500円 以下になったら
// → 7203 の終値 3,000円・2026-11-10 と、6758 の終値 3,500.1円・2026-11-10 を保存する → 通知は 7203 の1件だけ（「終値 3,000円」「決めた価格 3,000円 以上」）。
func Test基準13_決めた価格は届いた銘柄だけ通知ができる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	insertBuy(t, pool, seedUserA, "6758", 100, "3000")
	cond7203 := insertStockCondition(t, pool, seedUserA, "price", "7203", "above", "3000")
	_ = insertStockCondition(t, pool, seedUserA, "price", "6758", "below", "3500")

	saveClosePrices(t, pool,
		mustClosingPrice(t, "7203", "2026-11-10", 300000),
		mustClosingPrice(t, "6758", "2026-11-10", 350010),
	)

	assertNotifications(t, pool, wantNotification{
		conditionID: cond7203, stockCode: "7203", tradingDate: "2026-11-10", closeHundredths: 300000, details: `{}`,
		kind: "price", direction: "above", thresholdHundredths: 300000,
	})
}

// 基準14（要件15）: Aが 7203「買 2,500円 100株」、条件: 7203・8,000円 以上になったら（2026-11-02）、条件: 7203・8,200円 以上になったら（2026-11-10）。7203 の分割 1→2 2026-11-10
// → 終値 4,100円・2026-11-10 の保存で、2026-11-02 に足した条件の通知だけができ、決めた価格は 4,000円（分割に合わせた値。足したときは 8,000円）。
// 2026-11-10 に足した条件は、同じ日の分割では割らないので 8,200円のまま。
// 通知に写した条件の決めた価格（threshold）は、足したときの 8,000円のまま（割った 4,000円にしない）。
func Test基準14_決めた価格は足した日より後の分割で割り足した日と同じ日の分割では割らない(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	insertStockSplit(t, pool, "7203", "2026-11-10", 2)
	before := insertAlertConditionOn(t, pool, seedUserA, "price", "stock", "7203", "above", "8000", "2026-11-02")
	_ = insertAlertConditionOn(t, pool, seedUserA, "price", "stock", "7203", "above", "8200", "2026-11-10")

	saveClose(t, pool, "7203", "2026-11-10", 410000)

	assertNotifications(t, pool, wantNotification{
		conditionID: before, stockCode: "7203", tradingDate: "2026-11-10", closeHundredths: 410000,
		details: `{"adjustedThresholdHundredths":400000}`,
		kind:    "price", direction: "above", thresholdHundredths: 800000,
	})
}

// 基準15（要件3）: 条件: 全銘柄・取得価格から 20% 以上 下がったら（2026-11-01）。その後、Aが 6758「買 3,500円 100株 2026-11-05」
// → 6758 の終値 2,800円・2026-11-10 の保存で 6758 の通知が1件できる（-20.0%。条件を足した後に買った銘柄にも効く）。
func Test基準15_全銘柄の条件は足した後に買った銘柄にも効く(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	condID := insertAllCondition(t, pool, seedUserA, "cost", "down", "20")
	insertTrade(t, pool, seedUserA, "6758", "buy", 100, "3500", "2026-11-05")

	saveClose(t, pool, "6758", "2026-11-10", 280000)

	assertNotifications(t, pool, wantNotification{
		conditionID: condID, stockCode: "6758", tradingDate: "2026-11-10", closeHundredths: 280000,
		details: `{"rateTenths":-200,"basePriceHundredths":350000}`,
	})
}

// 基準16（要件19）: Aが 7203「買 2,500円 100株」、条件: 全銘柄・取得価格から 20% 以上 下がったら、条件: 7203・取得価格から 10% 以上 下がったら
// → 終値 2,250円・2026-11-10 の保存で 7203 の条件の通知が1件できる（-10.0%）。終値 1,950円・2026-11-11 の保存で全銘柄の条件の通知が1件増える
// （-22.0%。7203 の条件は前の判定も当てはまるので増えない）。
func Test基準16_全銘柄の条件と同じ銘柄の条件は別々に判定する(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	allCond := insertAllCondition(t, pool, seedUserA, "cost", "down", "20")
	stockCond := insertStockCondition(t, pool, seedUserA, "cost", "7203", "down", "10")

	saveClose(t, pool, "7203", "2026-11-10", 225000)
	first := wantNotification{
		conditionID: stockCond, stockCode: "7203", tradingDate: "2026-11-10", closeHundredths: 225000,
		details: `{"rateTenths":-100,"basePriceHundredths":250000}`,
	}
	assertNotifications(t, pool, first)

	saveClose(t, pool, "7203", "2026-11-11", 195000)
	assertNotifications(t, pool, first, wantNotification{
		conditionID: allCond, stockCode: "7203", tradingDate: "2026-11-11", closeHundredths: 195000,
		details: `{"rateTenths":-220,"basePriceHundredths":250000}`,
	})
}

// 基準17（要件10）: Aが 7203「買 2,500円 100株」「売 2,800円 100株 2026-11-02」、条件: 7203・3,000円 以上になったら。Bが 7203「買 3,000円 10株」
// → 7203 の終値 3,100円・2026-11-10 の保存で、Aの通知はない（Aは 7203 を保有していない）。
func Test基準17_保有していない銘柄の条件は判定しない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	insertTrade(t, pool, seedUserA, "7203", "sell", 100, "2800", "2026-11-02")
	insertStockCondition(t, pool, seedUserA, "price", "7203", "above", "3000")
	insertTrade(t, pool, seedUserB, "7203", "buy", 10, "3000", buyDate)

	saveClose(t, pool, "7203", "2026-11-10", 310000)

	assertNotifications(t, pool)
}

// 補助 要件10: 基準17 の続き。判定の後も条件は残り（対象がその銘柄だけの条件も残す）、続けてAが「買 3,000円 10株 2026-11-11」を登録し、
// 2026-11-12 に 3,200円を保存すると、Aに 2026-11-12 の通知が1件できる（買い直したらまた判定する。保有がなかった間の判定の結果は書かないので前の判定はない）。
func Test補助_要件10_保有していない間も条件は残り買い直したらまた判定する(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	insertTrade(t, pool, seedUserA, "7203", "sell", 100, "2800", "2026-11-02")
	condID := insertStockCondition(t, pool, seedUserA, "price", "7203", "above", "3000")
	insertTrade(t, pool, seedUserB, "7203", "buy", 10, "3000", buyDate)
	saveClose(t, pool, "7203", "2026-11-10", 310000)
	assertNotifications(t, pool)
	if n := countRows(t, pool, "SELECT count(*) FROM alert_conditions WHERE id = $1", condID); n != 1 {
		t.Fatalf("判定の後の条件の行数: got %d, want 1（条件は残す）", n)
	}

	insertTrade(t, pool, seedUserA, "7203", "buy", 10, "3000", "2026-11-11")
	saveClose(t, pool, "7203", "2026-11-12", 320000)

	assertNotifications(t, pool, wantNotification{
		conditionID: condID, stockCode: "7203", tradingDate: "2026-11-12", closeHundredths: 320000, details: `{}`,
	})
}

// 基準20（要件9）: Aが 7203「買 2,500円 100株」、条件: 7203・3,000円 以下になったら。7203 の終値（直接）: 2026-11-13 3,500円
// → 7203 の終値 2,900円・2026-11-12 を保存し直しても通知はない（最新の取引日ではない）。
// 続けて 7203 の分割 1→2 2026-11-17 を登録し、終値 2,900円・2026-11-16 を保存しても通知はない（分割の後の価格がまだないので、価格がないものとして扱う）。
func Test基準20_最新の取引日でない終値と分割の後の価格がない銘柄は判定しない(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	insertStockCondition(t, pool, seedUserA, "price", "7203", "below", "3000")
	insertClosePrice(t, pool, "7203", "2026-11-13", "3500")

	saveClose(t, pool, "7203", "2026-11-12", 290000)
	assertNotifications(t, pool)

	insertStockSplit(t, pool, "7203", "2026-11-17", 2)
	saveClose(t, pool, "7203", "2026-11-16", 290000)
	assertNotifications(t, pool)
}

// 基準21（要件20・要件16。後半。前半は alert_conditions_api_test.go）: Aが 7203「買 2,500円 100株」。7203 の終値（直接）: 2026-11-10 1,875円。
// 条件: 全銘柄・取得価格から 20% 以上 下がったら（2026-11-11）を直接入れた後、終値 1,870円・2026-11-11 の保存で通知が1件できる
// （-25.2%。初めての判定なので、当てはまれば作る）。
func Test基準21_条件を足した後の初めての判定で当てはまれば通知ができる(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	insertClosePrice(t, pool, "7203", "2026-11-10", "1875")
	condID := insertAlertConditionOn(t, pool, seedUserA, "cost", "all", "", "down", "20", "2026-11-11")
	assertNotifications(t, pool)

	saveClose(t, pool, "7203", "2026-11-11", 187000)

	assertNotifications(t, pool, wantNotification{
		conditionID: condID, stockCode: "7203", tradingDate: "2026-11-11", closeHundredths: 187000,
		details: `{"rateTenths":-252,"basePriceHundredths":250000}`,
	})
}

// 基準23（要件22）: 基準1 の通知（取得価格 2,500円 → 終値 2,001円、-20.0%）がある
// → 「買 1,500円 100株 2026-11-12」を登録し、条件を削除しても、その通知の行（kind・direction・threshold・close_price・details など）は変わらない。
func Test基準23_売買を足し条件を削除しても作った通知の中身は変わらない(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	condID := insertAllCondition(t, pool, seedUserA, "cost", "down", "20")
	saveClose(t, pool, "7203", "2026-11-11", 200100)
	want := wantNotification{
		conditionID: condID, stockCode: "7203", tradingDate: "2026-11-11", closeHundredths: 200100,
		details: `{"rateTenths":-200,"basePriceHundredths":250000}`,
		kind:    "cost", direction: "down", thresholdHundredths: 2000,
	}
	assertNotifications(t, pool, want)
	before := readNotifications(t, pool)

	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "1500", "2026-11-12")
	if _, err := pool.Exec(t.Context(), "DELETE FROM alert_conditions WHERE id = $1", condID); err != nil {
		t.Fatalf("条件を削除できない: %v", err)
	}

	assertNotifications(t, pool, want)
	if after := readNotifications(t, pool); !slices.Equal(after, before) {
		t.Errorf("通知の行が変わった（作ったときの値を残す）:\n before %+v\n after  %+v", before, after)
	}
}

// 補助 要件8（3つ目の道）: 価格の CSV の登録（ImportClosingPrices に判定をつなぐ）でも通知ができる。
// Aが 7203「買 2,500円 100株」、条件: 7203・3,000円 以上になったら。「7203,3000」の CSV を 2026-11-16 に登録すると、7203・2026-11-16 の通知が1件できる。
func Test補助_要件8_価格のCSVの登録でも通知ができる(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	condID := insertStockCondition(t, pool, seedUserA, "price", "7203", "above", "3000")
	importer := newPriceImporter(pool).WithAlertEvaluator(newAlertEvaluator(pool))

	out := runPriceImportWith(t, importer, writePriceCSV(t, priceCSV("7203,3000")))

	assertImportSucceeded(t, out, priceRegisteredMessage(1, importToday))
	assertNotifications(t, pool, wantNotification{
		conditionID: condID, stockCode: "7203", tradingDate: importToday, closeHundredths: 300000, details: `{}`,
	})
}

// 補助 要件8: CSV に誤りがあれば何も保存せず、判定も呼ばれない（通知0件）。誤りのある行（終値が数字でない）と、条件に当てはまる行が同じ CSV にある。
func Test補助_要件8_価格のCSVに誤りがあれば何も保存せず通知もできない(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	insertStockCondition(t, pool, seedUserA, "price", "7203", "above", "3000")
	importer := newPriceImporter(pool).WithAlertEvaluator(newAlertEvaluator(pool))

	out := runPriceImportWith(t, importer, writePriceCSV(t, priceCSV("7203,3000", "6758,abc")))

	if out.code == 0 {
		t.Errorf("終了コード: got 0, want 0以外（CSV に誤りがある）")
	}
	assertStockPrices(t, pool, []storedClosePrice{})
	assertNotifications(t, pool)
}

// 補助 4.2: 判定の結果の行は、条件と銘柄の組ごとに新しい2件だけ残る。4日続けて保存した後（1,990円・1,950円・2,050円・1,980円）に数えると2件
// （2026-11-12 は当てはまらない、2026-11-13 は当てはまる）。
func Test補助_4_2_判定の結果は条件と銘柄の組ごとに新しい2件だけ残す(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	condID := insertAllCondition(t, pool, seedUserA, "cost", "down", "20")

	saveClose(t, pool, "7203", "2026-11-10", 199000)
	saveClose(t, pool, "7203", "2026-11-11", 195000)
	saveClose(t, pool, "7203", "2026-11-12", 205000)
	saveClose(t, pool, "7203", "2026-11-13", 198000)

	want := []storedJudgment{
		{conditionID: condID, stockCode: "7203", tradingDate: "2026-11-12", satisfied: false},
		{conditionID: condID, stockCode: "7203", tradingDate: "2026-11-13", satisfied: true},
	}
	if got := readJudgments(t, pool, condID); !slices.Equal(got, want) {
		t.Errorf("alert_judgments の行:\n got %+v\nwant %+v（新しい2件だけ）", got, want)
	}
}

// 補助 4.2: 条件を消すと、その条件の判定の結果の行も消える（ON DELETE CASCADE）。消す前に1件ある。
func Test補助_4_2_条件を消すとその条件の判定の結果も消える(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	condID := insertAllCondition(t, pool, seedUserA, "cost", "down", "20")
	saveClose(t, pool, "7203", "2026-11-10", 200200)
	if got := readJudgments(t, pool, condID); len(got) != 1 {
		t.Fatalf("消す前の判定の結果: got %+v, want 1件", got)
	}

	if _, err := pool.Exec(t.Context(), "DELETE FROM alert_conditions WHERE id = $1", condID); err != nil {
		t.Fatalf("条件を削除できない: %v", err)
	}

	if got := readJudgments(t, pool, condID); len(got) != 0 {
		t.Errorf("条件を消した後の判定の結果: got %+v, want 0件", got)
	}
}

// 補助 要件18: 基準1 の売買と条件で、2026-11-10 に 1,990円を保存（1件、-20.4%）。続けて同じ日を 1,980円（-20.8%）と 2,100円（-16.0%）で保存し直す
// → 通知は1件のままで、close_price 1990、rateTenths -204（作った通知は消さず、書き換えない）。
func Test補助_要件18_同じ取引日を保存し直しても作った通知は消さず書き換えない(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	condID := insertAllCondition(t, pool, seedUserA, "cost", "down", "20")
	want := wantNotification{
		conditionID: condID, stockCode: "7203", tradingDate: "2026-11-10", closeHundredths: 199000,
		details: `{"rateTenths":-204,"basePriceHundredths":250000}`,
	}

	saveClose(t, pool, "7203", "2026-11-10", 199000)
	assertNotifications(t, pool, want)
	saveClose(t, pool, "7203", "2026-11-10", 198000)
	assertNotifications(t, pool, want)
	saveClose(t, pool, "7203", "2026-11-10", 210000)
	assertNotifications(t, pool, want)
}

// 補助 要件7: 基準1 の売買と条件で、2026-11-10 に 2,100円（当てはまらない）を保存し、条件を消してから 2026-11-11 に 1,990円を保存する → 通知0件（消した後は判定しない）。
func Test補助_要件7_条件を消した後は判定しない(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	condID := insertAllCondition(t, pool, seedUserA, "cost", "down", "20")
	saveClose(t, pool, "7203", "2026-11-10", 210000)
	if _, err := pool.Exec(t.Context(), "DELETE FROM alert_conditions WHERE id = $1", condID); err != nil {
		t.Fatalf("条件を削除できない: %v", err)
	}

	saveClose(t, pool, "7203", "2026-11-11", 199000)

	assertNotifications(t, pool)
}

// 補助 要件8: 利用者ごとの結果。終値は全利用者で共有なので、1回の保存で、その銘柄に条件を持つ全員の判定をする。
// Aは 7203「買 2,500円 100株」、Bは 7203「買 2,000円 100株」。2人とも全銘柄・取得価格から 20% 以上 下がったら。終値 1,950円・2026-11-10 の保存で、
// Aだけ通知ができる（Aは -22.0%、Bは -2.5%）。
func Test補助_要件8_1回の保存で条件を持つ全員を判定し利用者ごとに結果を出す(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	insertBuy(t, pool, seedUserB, "7203", 100, "2000")
	condA := insertAllCondition(t, pool, seedUserA, "cost", "down", "20")
	_ = insertAllCondition(t, pool, seedUserB, "cost", "down", "20")

	saveClose(t, pool, "7203", "2026-11-10", 195000)

	assertNotifications(t, pool, wantNotification{
		conditionID: condA, stockCode: "7203", tradingDate: "2026-11-10", closeHundredths: 195000,
		details: `{"rateTenths":-220,"basePriceHundredths":250000}`,
	})
}

// 補助 4.2（番号2）: 判定の結果の「前の判定」と「新しい2件だけ残す」は、条件と銘柄の組ごと（全銘柄の条件が複数の銘柄に効くとき、銘柄をまたがない）。
// Aが 7203（2,500円 100株）と 6758（3,500円 100株）を保有、条件: 全銘柄・取得価格から 20% 以上 下がったら。
// 7203 を 2026-11-10 1,990円で保存して通知1件（-20.4%）、6758 を 11-11 2,800円で2件目（-20.0%、基準の価格 3,500円）、
// 6758 を 11-12 2,700円・11-13 2,600円、7203 を 11-14 1,980円で保存する → 通知は2件のまま（7203 の前の判定は 11-10 で当てはまっている）。
// 判定の結果は、7203 が 11-10 と 11-14 の2件、6758 が 11-12 と 11-13 の2件（7203 の 11-10 の行を、6758 の新しい行のために消さない）。
func Test補助_4_2_前の判定と2件だけ残す処理は条件と銘柄の組ごとに数える(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	insertBuy(t, pool, seedUserA, "6758", 100, "3500")
	condID := insertAllCondition(t, pool, seedUserA, "cost", "down", "20")

	saveClose(t, pool, "7203", "2026-11-10", 199000)
	saveClose(t, pool, "6758", "2026-11-11", 280000)
	saveClose(t, pool, "6758", "2026-11-12", 270000)
	saveClose(t, pool, "6758", "2026-11-13", 260000)
	saveClose(t, pool, "7203", "2026-11-14", 198000)

	assertNotifications(t, pool,
		wantNotification{conditionID: condID, stockCode: "7203", tradingDate: "2026-11-10", closeHundredths: 199000, details: `{"rateTenths":-204,"basePriceHundredths":250000}`},
		wantNotification{conditionID: condID, stockCode: "6758", tradingDate: "2026-11-11", closeHundredths: 280000, details: `{"rateTenths":-200,"basePriceHundredths":350000}`},
	)
	want := []storedJudgment{
		{conditionID: condID, stockCode: "6758", tradingDate: "2026-11-12", satisfied: true},
		{conditionID: condID, stockCode: "6758", tradingDate: "2026-11-13", satisfied: true},
		{conditionID: condID, stockCode: "7203", tradingDate: "2026-11-10", satisfied: true},
		{conditionID: condID, stockCode: "7203", tradingDate: "2026-11-14", satisfied: true},
	}
	if got := readJudgments(t, pool, condID); !slices.Equal(got, want) {
		t.Errorf("alert_judgments の行:\n got %+v\nwant %+v（銘柄ごとに新しい2件）", got, want)
	}
}

// 補助 要件16・要件18（番号3）: 同じ取引日を保存し直したとき、前の判定は、保存し直した取引日より前の判定（前の取引日の判定）を使う
// （保存し直す前の同じ日の判定を前の判定にしない）。基準1 の売買と、条件: 全銘柄・取得価格から 20% 以上 下がったら。
// 2026-11-10 に 1,990円（通知1件、-20.4%）、11-11 に 2,100円（当てはまらない）、11-11 を 1,990円で保存し直す
// → 通知は 11-10 の1件だけ（11-11 は前の判定 11-10 が当てはまっているので作らない）。判定の結果は 11-10 当てはまった・11-11 当てはまったの2行。
func Test補助_要件16_同じ取引日を保存し直したときの前の判定は前の取引日の判定を使う(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	condID := insertAllCondition(t, pool, seedUserA, "cost", "down", "20")

	saveClose(t, pool, "7203", "2026-11-10", 199000)
	saveClose(t, pool, "7203", "2026-11-11", 210000)
	saveClose(t, pool, "7203", "2026-11-11", 199000)

	assertNotifications(t, pool, wantNotification{
		conditionID: condID, stockCode: "7203", tradingDate: "2026-11-10", closeHundredths: 199000,
		details: `{"rateTenths":-204,"basePriceHundredths":250000}`,
	})
	want := []storedJudgment{
		{conditionID: condID, stockCode: "7203", tradingDate: "2026-11-10", satisfied: true},
		{conditionID: condID, stockCode: "7203", tradingDate: "2026-11-11", satisfied: true},
	}
	if got := readJudgments(t, pool, condID); !slices.Equal(got, want) {
		t.Errorf("alert_judgments の行:\n got %+v\nwant %+v", got, want)
	}
}

// 補助 4.4・4.8（番号4）: 判定の途中で消された条件の分は、判定の結果も通知も書かない（SaveEvaluation は alert_conditions に残っている条件の分だけ書く）。
// AとBが 7203「買 2,500円 100株」、2人とも条件: 全銘柄・取得価格から 20% 以上 下がったら。7203 の終値 1,950円・2026-11-10 を保存済みにして、
// LoadEvaluationInput で2人の条件を読み、Bの条件（当てはまる）を消してから、EvaluateAlerts の結果を SaveEvaluation に渡す
// → Aの条件の判定の結果（当てはまった）と通知（-22.0%）は書かれ、Bの条件の分はどちらも書かれない。
func Test補助_4_4_判定の途中で消された条件の分は判定の結果も通知も書かない(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	insertBuy(t, pool, seedUserB, "7203", 100, "2500")
	condA := insertAllCondition(t, pool, seedUserA, "cost", "down", "20")
	condB := insertAllCondition(t, pool, seedUserB, "cost", "down", "20")
	insertClosePrice(t, pool, "7203", "2026-11-10", "1950")
	repo := write.NewAlertRepository(pool)
	in, err := repo.LoadEvaluationInput(t.Context(), "7203", "2026-11-10", 195000)
	if err != nil {
		t.Fatalf("LoadEvaluationInput: %v", err)
	}
	result, err := query.EvaluateAlerts(in)
	if err != nil {
		t.Fatalf("EvaluateAlerts: %v", err)
	}
	if len(result.Notifications) != 2 {
		t.Fatalf("消す前の判定の通知: got %d件, want 2件（AとBの条件）: %+v", len(result.Notifications), result.Notifications)
	}
	if _, err := pool.Exec(t.Context(), "DELETE FROM alert_conditions WHERE id = $1", condB); err != nil {
		t.Fatalf("条件を削除できない: %v", err)
	}

	if err := repo.SaveEvaluation(t.Context(), result); err != nil {
		t.Fatalf("SaveEvaluation: %v", err)
	}

	assertNotifications(t, pool, wantNotification{
		conditionID: condA, stockCode: "7203", tradingDate: "2026-11-10", closeHundredths: 195000,
		details: `{"rateTenths":-220,"basePriceHundredths":250000}`,
	})
	wantA := []storedJudgment{{conditionID: condA, stockCode: "7203", tradingDate: "2026-11-10", satisfied: true}}
	if got := readJudgments(t, pool, condA); !slices.Equal(got, wantA) {
		t.Errorf("Aの条件の alert_judgments の行:\n got %+v\nwant %+v", got, wantA)
	}
	if got := readJudgments(t, pool, condB); len(got) != 0 {
		t.Errorf("消したBの条件の alert_judgments の行: got %+v, want 0件", got)
	}
}

// 補助 4.8（事前2）: 通知の宛先（UserID）が、条件の持ち主と違えば書かない。Aの条件 ID（condA）で UserID がBの通知を手で作って SaveEvaluation に渡すと、
// その通知は書かれない（notifications は0件）。
func Test補助_4_8_通知の宛先が条件の持ち主と違えば書かない(t *testing.T) {
	t.Parallel()
	pool := newAlertDB(t)
	condA := insertAllCondition(t, pool, seedUserA, "cost", "down", "20")
	row := dto.AlertConditionRow{
		ID: condA, UserID: uuid.MustParse(seedUserA), Kind: "cost", Scope: "all", Direction: "down",
		ThresholdHundredths: 2000, AddedOn: addedDefault,
	}
	rate, base := int64(-220), int64(250000)
	result := dto.AlertEvaluationResult{
		Judgments: []dto.AlertJudgment{{ConditionID: condA, StockCode: "7203", TradingDate: "2026-11-10", Satisfied: true}},
		Notifications: []dto.AlertNotification{{
			UserID: uuid.MustParse(seedUserB), Condition: row, StockCode: "7203", TradingDate: "2026-11-10", ClosePriceHundredths: 195000,
			Details: dto.AlertNotificationDetails{RateTenths: &rate, BasePriceHundredths: &base},
		}},
	}

	if err := write.NewAlertRepository(pool).SaveEvaluation(t.Context(), result); err != nil {
		t.Fatalf("SaveEvaluation: %v", err)
	}

	assertNotifications(t, pool)
}

// 基準57（要件10・要件16）: Aが 7203「買 2,500円 100株 2026-11-12」、条件: 全銘柄・取得価格から 10% 以上 上がったら
// → 7203 の終値 2,800円・2026-11-11 を保存して通知はなく、判定の結果も書かれない（2026-11-11 は今の保有が始まった 2026-11-12 より前。判定すれば +12.0% で当てはまる）。
// 続けて 2,760円・2026-11-12 を保存すると、取引日 2026-11-12 の通知が1件できる（+10.4%。買った日と同じ日の終値は判定する。
// 2026-11-11 を判定していたら、この日はその判定を前の判定に数えないので通知が2件になる）。
func Test基準57_判定する取引日が今の保有が始まった日より前なら判定しない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "2500", "2026-11-12")
	condID := insertAllCondition(t, pool, seedUserA, "cost", "up", "10")

	saveClose(t, pool, "7203", "2026-11-11", 280000)

	assertNotifications(t, pool)
	if n := countRows(t, pool, "SELECT count(*) FROM alert_judgments"); n != 0 {
		t.Errorf("保有が始まる前の取引日を保存した後の alert_judgments の行数: got %d, want 0", n)
	}

	saveClose(t, pool, "7203", "2026-11-12", 276000)

	assertNotifications(t, pool, wantNotification{
		conditionID: condID, stockCode: "7203", tradingDate: "2026-11-12", closeHundredths: 276000,
		details: `{"rateTenths":104,"basePriceHundredths":250000}`,
	})
}
