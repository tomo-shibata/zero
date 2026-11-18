package integration

import (
	"fmt"
	"testing"
	"time"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 価格の変動の通知の要件（docs/requirements/price-alerts.md）の非機能3・基準56と、
// プラン（docs/plans/price-alerts/plan.md）5.1 の基準56 の行（price_alerts_performance_test.go）。
// Aの条件100件（全銘柄の最高値からを含む）、保有50銘柄、各銘柄の終値（直接）が 2016-11-18 からの約2,500日分のとき、
// 50銘柄の終値を保存するのに判定が増やす時間が、判定なしで保存したときと比べて2秒以内。
// 重い道を必ず通すため、全銘柄・最高値からを含め、50銘柄の買いの日を終値の始まりと同じ 2016-11-18 にする（終値の列を約2,500日分読む）。
// 判定なしと判定ありは、同じ DB で別の取引日（2026-11-17 と 2026-11-18）に順に測る。t.Parallel() は呼ばない（並行する他のテストが時間に響かないように）。
// 時間のかかるテストなので testing.Short() では飛ばす。

const (
	performanceStocks    = 50
	performanceDays      = 2500
	performanceFirstDay  = "2016-11-18"
	performanceBudget    = 2 * time.Second
	performanceBaseDate  = "2026-11-17" // 判定なしで保存する取引日
	performanceAlertDate = "2026-11-18" // 判定ありで保存する取引日
)

// performanceCode は、i 番目（1始まり）の銘柄の銘柄コード（P001〜P050）。
func performanceCode(i int) string { return fmt.Sprintf("P%03d", i) }

// performancePrices は、50銘柄の、取引日 tradingDate の終値（1,000円台。銘柄ごとに少しずつ違う値）。
func performancePrices(t *testing.T, tradingDate string) (map[string]model.ClosingPrice, []repository.HeldStock) {
	t.Helper()
	prices := map[string]model.ClosingPrice{}
	var stocks []repository.HeldStock
	for i := 1; i <= performanceStocks; i++ {
		code := performanceCode(i)
		prices[code] = mustClosingPrice(t, code, tradingDate, alertYen(int64(1000+i)))
		stocks = append(stocks, repository.HeldStock{Code: code, Currency: model.CurrencyJPY})
	}
	return prices, stocks
}

func Test基準56_条件100件_50銘柄_各2500日の終値でも判定が増やす時間は2秒以内(t *testing.T) {
	if testing.Short() {
		t.Skip("時間のかかるテストなので -short では飛ばす")
	}
	pool := testdb.New(t)
	ctx := t.Context()
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("前提のデータを入れられない（%s）: %v", sql, err)
		}
	}
	// 銘柄マスタ 50銘柄（円）。
	mustExec(`INSERT INTO stocks (code, name, currency)
	          SELECT 'P' || lpad(i::text, 3, '0'), 'テスト銘柄P' || i, 'JPY' FROM generate_series(1, $1::int) AS i`, performanceStocks)
	// Aが全銘柄を「買 1,000円 100株 2016-11-18」で保有。
	mustExec(`INSERT INTO trades (user_id, stock_code, side, quantity, price, trade_date)
	          SELECT $1::uuid, 'P' || lpad(i::text, 3, '0'), 'buy', 100, 1000, $2::text::date FROM generate_series(1, $3::int) AS i`,
		seedUserA, performanceFirstDay, performanceStocks)
	// 各銘柄の終値（直接）: 2016-11-18 から2,500日分。
	mustExec(`INSERT INTO stock_prices (stock_code, trading_date, close_price)
	          SELECT 'P' || lpad(i::text, 3, '0'), $2::text::date + d, 1000 + ((i + d) % 200)
	            FROM generate_series(1, $1::int) AS i, generate_series(0, $3::int - 1) AS d`,
		performanceStocks, performanceFirstDay, performanceDays)
	// Aの条件100件: 全銘柄・最高値から（1〜50%）50件、全銘柄・取得価格から（1〜25%）25件、全銘柄・前の取引日から動いた（1〜25%）25件。
	mustExec(`INSERT INTO alert_conditions (user_id, kind, scope, stock_code, direction, threshold)
	          SELECT $1::uuid, 'peak', 'all', '', 'down', i FROM generate_series(1, 50) AS i
	          UNION ALL
	          SELECT $1::uuid, 'cost', 'all', '', 'down', i FROM generate_series(1, 25) AS i
	          UNION ALL
	          SELECT $1::uuid, 'prev', 'all', '', 'either', i FROM generate_series(1, 25) AS i`, seedUserA)
	if n := countRows(t, pool, "SELECT count(*) FROM alert_conditions"); n != 100 {
		t.Fatalf("前提の条件の件数: got %d, want 100", n)
	}

	// 判定なしで50銘柄の終値を保存する時間。
	basePrices, stocks := performancePrices(t, performanceBaseDate)
	baseSrc := stubPriceSource{t: t, prices: basePrices}
	start := time.Now()
	baseResult := newFetchClosingPrices(baseSrc, pool).ExecuteStocks(ctx, stocks)
	withoutAlerts := time.Since(start)
	if len(baseResult.Failed) != 0 || len(baseResult.Saved) != performanceStocks {
		t.Fatalf("判定なしの保存: Saved %d件・Failed %v, want 50件・失敗なし", len(baseResult.Saved), baseResult.Failed)
	}

	// 判定ありで50銘柄の終値を保存する時間。
	alertPrices, _ := performancePrices(t, performanceAlertDate)
	alertSrc := stubPriceSource{t: t, prices: alertPrices}
	start = time.Now()
	alertResult := newFetchWithAlerts(alertSrc, pool).ExecuteStocks(ctx, stocks)
	withAlerts := time.Since(start)
	if len(alertResult.Failed) != 0 || len(alertResult.Saved) != performanceStocks {
		t.Fatalf("判定ありの保存: Saved %d件・Failed %v, want 50件・失敗なし", len(alertResult.Saved), alertResult.Failed)
	}

	// 判定が実際に動いた（保有50銘柄 × 判定の結果を書く条件75件 = 3,750件。前の取引日からの判定の結果は書かない）。
	if n := countRows(t, pool, "SELECT count(*) FROM alert_judgments"); n != 3750 {
		t.Errorf("alert_judgments の行数: got %d, want 3750（50銘柄 × 最高値から・取得価格からの75件。判定が動いていない）", n)
	}
	if added := withAlerts - withoutAlerts; added > performanceBudget {
		t.Errorf("判定が増やした時間: got %v（判定あり %v、判定なし %v）, want %v 以内", added, withAlerts, withoutAlerts, performanceBudget)
	}
	t.Logf("判定なし %v、判定あり %v", withoutAlerts, withAlerts)
}
