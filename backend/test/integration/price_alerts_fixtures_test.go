package integration

import (
	"cmp"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/write"
	"github.com/tomo-shibata/zero/backend/internal/interface/pricecsv"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 価格の変動の通知の判定（docs/plans/price-alerts/plan.md の PR①）の統合テストが使う補助。
// プラン 5章の冒頭: 判定の基準は、DB に売買・終値（直接）・分割・条件を入れ、テスト用の取得元を注入した FetchClosingPrices に
// WithAlertEvaluator で判定をつないで終値を保存し、notifications の行を SQL で読んで確かめる。
// 条件は alert_conditions に created_at つきで直接入れる（日本時間の0時。UTC の日付で足した日を取る実装を基準14 で落とすため）。
// 通知（notifications）・判定の結果（alert_judgments）の列は 4.2 のとおり。金額は 0.01 単位の整数で比べる。

// alertYen は、円の金額 n を 0.01 単位にする（2,500円 → 250000）。
func alertYen(n int64) int64 { return n * 100 }

// addedDefault は、足した日を書いていない条件の足した日（要件定義の成功基準の冒頭）。
const addedDefault = "2026-11-01"

// insertAlertConditionOn は、利用者 userID の条件1件を、足した日 addedOn（"YYYY-MM-DD"。日本時間の0時）で入れ、その id を返す。
// kind・scope・direction は 4.2 の値（"cost"・"all"・"down" など）、stockCode は全銘柄なら ""、threshold は 20（%）・100000（円）・150.25（ドル）のような値そのまま。
func insertAlertConditionOn(t *testing.T, pool *pgxpool.Pool, userID, kind, scope, stockCode, direction, threshold, addedOn string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(t.Context(),
		`INSERT INTO alert_conditions (user_id, kind, scope, stock_code, direction, threshold, created_at)
		 VALUES ($1::uuid, $2::text, $3::text, $4::text, $5::text, $6::text::numeric, ($7::text || 'T00:00:00+09:00')::timestamptz)
		 RETURNING id`,
		userID, kind, scope, stockCode, direction, threshold, addedOn,
	).Scan(&id)
	if err != nil {
		t.Fatalf("条件（%s・%s・%s・%s・%s・%s・%s）を入れられない: %v", userID, kind, scope, stockCode, direction, threshold, addedOn, err)
	}
	return id
}

// insertAllCondition は、全銘柄の条件（足した日は 2026-11-01）を入れ、その id を返す。
func insertAllCondition(t *testing.T, pool *pgxpool.Pool, userID, kind, direction, threshold string) int64 {
	t.Helper()
	return insertAlertConditionOn(t, pool, userID, kind, "all", "", direction, threshold, addedDefault)
}

// insertStockCondition は、銘柄 stockCode の条件（足した日は 2026-11-01）を入れ、その id を返す。
func insertStockCondition(t *testing.T, pool *pgxpool.Pool, userID, kind, stockCode, direction, threshold string) int64 {
	t.Helper()
	return insertAlertConditionOn(t, pool, userID, kind, "stock", stockCode, direction, threshold, addedDefault)
}

// newFetchWithAlerts は、取得元 src と pool の DB を使う価格の取得処理に、判定（4.4 の組み立て）をつないだもの。
func newFetchWithAlerts(src command.PriceSource, pool *pgxpool.Pool) *command.FetchClosingPrices {
	return newFetchClosingPrices(src, pool).WithAlertEvaluator(newAlertEvaluator(pool))
}

// newAlertEvaluator は、pool の DB を使う判定（4.4: command.NewEvaluatePriceAlerts(write.NewAlertRepository(pool))）。
func newAlertEvaluator(pool *pgxpool.Pool) command.PriceAlertEvaluator {
	return command.NewEvaluatePriceAlerts(write.NewAlertRepository(pool))
}

// saveClosePrices は、prices（銘柄コードは重ならない）の終値を、テスト用の取得元から、判定をつないだ価格の取得処理（ExecuteStocks）で保存する。
// 通貨は、AAPL がドル、それ以外は円。保存できなかった銘柄があればテストを止める。
func saveClosePrices(t *testing.T, pool *pgxpool.Pool, prices ...model.ClosingPrice) {
	t.Helper()
	src := stubPriceSource{t: t, prices: map[string]model.ClosingPrice{}}
	var stocks []repository.HeldStock
	for _, p := range prices {
		src.prices[p.StockCode] = p
		currency := model.CurrencyJPY
		if p.StockCode == "AAPL" {
			currency = model.CurrencyUSD
		}
		stocks = append(stocks, repository.HeldStock{Code: p.StockCode, Currency: currency})
	}
	result := newFetchWithAlerts(src, pool).ExecuteStocks(t.Context(), stocks)
	if len(result.Failed) != 0 {
		t.Fatalf("終値を保存できなかった銘柄: %v（%+v）", result.Failed, prices)
	}
}

// saveClose は、1つの銘柄の終値（0.01 単位）を、判定をつないで保存する。
func saveClose(t *testing.T, pool *pgxpool.Pool, code, tradingDate string, priceHundredths int64) {
	t.Helper()
	saveClosePrices(t, pool, mustClosingPrice(t, code, tradingDate, priceHundredths))
}

// storedNotification は、notifications の1行のうち、テストで確かめる値。
type storedNotification struct {
	userID              string
	conditionID         int64
	kind                string
	scope               string
	conditionStockCode  string
	direction           string
	thresholdHundredths int64
	stockCode           string
	tradingDate         string
	closeHundredths     int64
	details             string // 並びを決めた JSON（キーの昇順）
	seen                bool
}

// normalizeJSON は、JSON の文字列を、キーの昇順の決まった形にする（jsonb の並びや空白に左右されずに比べるため）。
func normalizeJSON(t *testing.T, s string) string {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("JSON %q を読めない: %v", s, err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("JSON を作れない: %v", err)
	}
	return string(out)
}

// compareNotifications は、通知を（取引日、利用者、条件 ID、銘柄コード）の順に並べるための比べ方。
func compareNotifications(a, b storedNotification) int {
	return cmp.Or(
		cmp.Compare(a.tradingDate, b.tradingDate),
		strings.Compare(a.userID, b.userID),
		cmp.Compare(a.conditionID, b.conditionID),
		strings.Compare(a.stockCode, b.stockCode),
	)
}

// readNotifications は、notifications のすべての行を、compareNotifications の順で返す。
func readNotifications(t *testing.T, pool *pgxpool.Pool) []storedNotification {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT user_id::text, condition_id, kind, scope, condition_stock_code, direction, (threshold * 100)::bigint,
		        stock_code, to_char(trading_date, 'YYYY-MM-DD'), (close_price * 100)::bigint, details::text, seen
		   FROM notifications`)
	if err != nil {
		t.Fatalf("notifications を読めない: %v", err)
	}
	got, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (storedNotification, error) {
		var n storedNotification
		err := row.Scan(&n.userID, &n.conditionID, &n.kind, &n.scope, &n.conditionStockCode, &n.direction, &n.thresholdHundredths,
			&n.stockCode, &n.tradingDate, &n.closeHundredths, &n.details, &n.seen)
		return n, err
	})
	if err != nil {
		t.Fatalf("notifications を読めない: %v", err)
	}
	for i := range got {
		got[i].details = normalizeJSON(t, got[i].details)
	}
	slices.SortFunc(got, compareNotifications)
	return got
}

// wantNotification は、通知の1行について確かめる値。userID が空なら利用者A。kind・direction・thresholdHundredths は、書いたときだけ確かめる
// （thresholdHundredths は 0.01 単位で、20% は 2000）。details は JSON（キーの順は問わない）。
type wantNotification struct {
	userID              string
	conditionID         int64
	stockCode           string
	tradingDate         string
	closeHundredths     int64
	details             string
	kind                string
	direction           string
	thresholdHundredths int64
}

// assertNotifications は、notifications の行が want とちょうど同じ（行数も中身も。並びは問わない）で、すべて見ていない（seen が false）ことを確かめる。
func assertNotifications(t *testing.T, pool *pgxpool.Pool, want ...wantNotification) {
	t.Helper()
	got := readNotifications(t, pool)
	if len(got) != len(want) {
		t.Errorf("notifications の行数: got %d, want %d\n got %+v", len(got), len(want), got)
		return
	}
	wantSorted := make([]storedNotification, 0, len(want))
	for _, w := range want {
		userID := w.userID
		if userID == "" {
			userID = seedUserA
		}
		wantSorted = append(wantSorted, storedNotification{
			userID: userID, conditionID: w.conditionID, stockCode: w.stockCode, tradingDate: w.tradingDate,
			closeHundredths: w.closeHundredths, details: normalizeJSON(t, w.details),
		})
	}
	slices.SortFunc(wantSorted, compareNotifications)
	for i, w := range wantSorted {
		g := got[i]
		check := func(name string, gotV, wantV any) {
			t.Helper()
			if gotV != wantV {
				t.Errorf("%d行目（%s・%s・条件 %d）の %s: got %v, want %v", i+1, g.stockCode, g.tradingDate, g.conditionID, name, gotV, wantV)
			}
		}
		check("利用者", g.userID, w.userID)
		check("条件 ID", g.conditionID, w.conditionID)
		check("銘柄コード", g.stockCode, w.stockCode)
		check("取引日", g.tradingDate, w.tradingDate)
		check("終値（0.01 単位）", g.closeHundredths, w.closeHundredths)
		check("details", g.details, w.details)
		check("seen", g.seen, false)
	}
	// 条件の写し（kind・direction・threshold）は、書いたときだけ確かめる。want の並びは compareNotifications で並べ直したので、元の want から探す。
	for _, w := range want {
		if w.kind == "" && w.direction == "" && w.thresholdHundredths == 0 {
			continue
		}
		userID := w.userID
		if userID == "" {
			userID = seedUserA
		}
		for _, g := range got {
			if g.userID != userID || g.conditionID != w.conditionID || g.stockCode != w.stockCode || g.tradingDate != w.tradingDate {
				continue
			}
			if w.kind != "" && g.kind != w.kind {
				t.Errorf("通知（条件 %d・%s）の kind: got %q, want %q", w.conditionID, w.tradingDate, g.kind, w.kind)
			}
			if w.direction != "" && g.direction != w.direction {
				t.Errorf("通知（条件 %d・%s）の direction: got %q, want %q", w.conditionID, w.tradingDate, g.direction, w.direction)
			}
			if w.thresholdHundredths != 0 && g.thresholdHundredths != w.thresholdHundredths {
				t.Errorf("通知（条件 %d・%s）の threshold（0.01 単位）: got %d, want %d", w.conditionID, w.tradingDate, g.thresholdHundredths, w.thresholdHundredths)
			}
		}
	}
}

// countRows は、SQL の count(*) の結果を返す（args は SQL の引数）。
func countRows(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("行数を数えられない（%s）: %v", sql, err)
	}
	return n
}

// storedJudgment は、alert_judgments の1行。
type storedJudgment struct {
	conditionID int64
	stockCode   string
	tradingDate string
	satisfied   bool
}

// readJudgments は、条件 conditionID の判定の結果を、取引日の昇順で返す。
func readJudgments(t *testing.T, pool *pgxpool.Pool, conditionID int64) []storedJudgment {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT condition_id, stock_code, to_char(trading_date, 'YYYY-MM-DD'), satisfied
		   FROM alert_judgments WHERE condition_id = $1 ORDER BY stock_code, trading_date`, conditionID)
	if err != nil {
		t.Fatalf("alert_judgments を読めない: %v", err)
	}
	got, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (storedJudgment, error) {
		var j storedJudgment
		err := row.Scan(&j.conditionID, &j.stockCode, &j.tradingDate, &j.satisfied)
		return j, err
	})
	if err != nil {
		t.Fatalf("alert_judgments を読めない: %v", err)
	}
	return got
}

// runPriceImportWith は、価格の登録のユースケース importer（判定をつないだものなど）で、今日が 2026-11-16 のときに csvPath の CSV を登録する。
func runPriceImportWith(t *testing.T, importer *command.ImportClosingPrices, csvPath string) importOutcome {
	t.Helper()
	var stdout, stderr strings.Builder
	code := pricecsv.Run(t.Context(), pricecsv.RunInput{CSVPath: csvPath, Today: importToday, Importer: importer}, &stdout, &stderr)
	return importOutcome{code: code, stdout: splitOutputLines(stdout.String()), stderr: splitOutputLines(stderr.String())}
}

// newAlertDB は、利用者Aが 7203「買 2,500円 100株」（取引日は 2026-01-10）を持つ DB を作る。
func newAlertDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	return pool
}
