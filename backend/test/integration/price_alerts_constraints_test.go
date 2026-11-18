package integration

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: プラン（docs/plans/price-alerts/plan.md）4.2（alert_conditions・notifications の制約）と 5.2 の price_alerts_constraints_test.go の行（PR①）。
//   - alert_conditions: kind は cost・peak・prev・pnl・price、scope は all・stock、direction は種類ごとの組み合わせ（cost は up・down、peak は down、
//     prev は up・down・either、pnl は loss・gain、price は above・below）、kind が pnl・price なら scope は stock、
//     (scope = 'all') = (stock_code = '')、threshold は 0 より大きく、10億（1000000000）以下で、小数第2位まで（小数第3位は不可）、Infinity は不可。
//     一意制約: (user_id, kind, scope, stock_code, direction, threshold)（NUMERIC の比較なので 20 と 20.00 は同じ）。user_id の外部キー → users。
//   - notifications: 一意制約 (condition_id, stock_code, trading_date)。condition_id は外部キーにしない（条件を消しても通知を残すため）。
// 形は managed_accounts_constraints_test.go と同じ: 1件ずつロールバックするトランザクションの中で入れ（insertInRolledBackTx）、
// SQLSTATE が制約の種類と一致することで「保存できない」を確かめる。前提の行だけ先にコミットして入れる。SQLSTATE の定数は trades_constraints_test.go・
// holding_orders_constraints_test.go のもの。

const insertAlertConditionSQL = "INSERT INTO alert_conditions (user_id, kind, scope, stock_code, direction, threshold) " +
	"VALUES ($1::uuid, $2::text, $3::text, $4::text, $5::text, $6::text::numeric)"

// alertConditionValues は、alert_conditions に入れる1行の値。threshold は SQL の NUMERIC に読ませる文字列。
type alertConditionValues struct {
	userID, kind, scope, stockCode, direction, threshold string
}

// validAlertCondition は、前提の行と一意制約が重ならない正しい行（A・全銘柄・取得価格から・下がった・15）。
func validAlertCondition() alertConditionValues {
	return alertConditionValues{seedUserA, "cost", "all", "", "down", "15"}
}

// with は、v の一部だけを change で変えた行を返す（v 自体は変えない）。
func (v alertConditionValues) with(change func(*alertConditionValues)) alertConditionValues {
	change(&v)
	return v
}

// tryInsertAlertCondition は、v の行をロールバックするトランザクションの中で入れ、結果（エラー）を返す。
func tryInsertAlertCondition(t *testing.T, pool *pgxpool.Pool, v alertConditionValues) error {
	t.Helper()
	return insertInRolledBackTx(t, pool, insertAlertConditionSQL, v.userID, v.kind, v.scope, v.stockCode, v.direction, v.threshold)
}

// newAlertConstraintsDB は、前提の条件（Aの「全銘柄・取得価格から・下がった・20」）を1件コミットして入れた DB を返す。
func newAlertConstraintsDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testdb.New(t)
	insertAllCondition(t, pool, seedUserA, "cost", "down", "20")
	return pool
}

// 4.2: 正しい条件が保存できる。種類と向きの組み合わせ（表のとおり）、全銘柄と1銘柄、値の端（0.01・小数第2位・大きな値）、
// 別の利用者の同じ条件、同じ条件でも値が違うもの。
func Test4_2_正しい条件は保存できる(t *testing.T) {
	t.Parallel()
	pool := newAlertConstraintsDB(t)
	valid := validAlertCondition()
	cases := []struct {
		name string
		v    alertConditionValues
	}{
		{"取得価格から・全銘柄・下がった", valid},
		{"取得価格から・全銘柄・上がった", valid.with(func(v *alertConditionValues) { v.direction = "up" })},
		{"取得価格から・1銘柄", valid.with(func(v *alertConditionValues) { v.scope = "stock"; v.stockCode = "7203" })},
		{"最高値から・全銘柄・下がった", valid.with(func(v *alertConditionValues) { v.kind = "peak" })},
		{"前の取引日から・全銘柄・上がった", valid.with(func(v *alertConditionValues) { v.kind = "prev"; v.direction = "up" })},
		{"前の取引日から・全銘柄・下がった", valid.with(func(v *alertConditionValues) { v.kind = "prev" })},
		{"前の取引日から・全銘柄・動いた", valid.with(func(v *alertConditionValues) { v.kind = "prev"; v.direction = "either" })},
		{"含み損益の金額・1銘柄・含み損", valid.with(func(v *alertConditionValues) {
			v.kind = "pnl"
			v.scope = "stock"
			v.stockCode = "7203"
			v.direction = "loss"
			v.threshold = "100000"
		})},
		{"含み損益の金額・1銘柄・含み益", valid.with(func(v *alertConditionValues) {
			v.kind = "pnl"
			v.scope = "stock"
			v.stockCode = "AAPL"
			v.direction = "gain"
			v.threshold = "300.00"
		})},
		{"決めた価格・1銘柄・以上", valid.with(func(v *alertConditionValues) {
			v.kind = "price"
			v.scope = "stock"
			v.stockCode = "7203"
			v.direction = "above"
			v.threshold = "3000.2"
		})},
		{"決めた価格・1銘柄・以下（小数第2位）", valid.with(func(v *alertConditionValues) {
			v.kind = "price"
			v.scope = "stock"
			v.stockCode = "AAPL"
			v.direction = "below"
			v.threshold = "150.25"
		})},
		{"値が0.01", valid.with(func(v *alertConditionValues) { v.threshold = "0.01" })},
		{"値が大きい", valid.with(func(v *alertConditionValues) { v.threshold = "99999999.99" })},
		{"値が上限ちょうど（10億）", valid.with(func(v *alertConditionValues) { v.threshold = "1000000000" })},
		{"別の利用者の同じ条件", valid.with(func(v *alertConditionValues) { v.userID = seedUserB; v.threshold = "20" })},
		{"同じ種類・対象・向きで値が違う", valid.with(func(v *alertConditionValues) { v.threshold = "20.01" })},
	}
	for _, tc := range cases {
		if err := tryInsertAlertCondition(t, pool, tc.v); err != nil {
			t.Errorf("%s: 正しい行 %+v を保存できない: %v", tc.name, tc.v, err)
		}
	}
}

// 4.2: 種類・対象・向き・銘柄コード・値が決まりに合わない条件は、23514（CHECK）で保存できない。
func Test4_2_決まりに合わない条件は保存できない(t *testing.T) {
	t.Parallel()
	pool := newAlertConstraintsDB(t)
	valid := validAlertCondition()
	cases := []struct {
		name string
		v    alertConditionValues
	}{
		{"知らない種類", valid.with(func(v *alertConditionValues) { v.kind = "volume" })},
		{"知らない対象", valid.with(func(v *alertConditionValues) { v.scope = "group" })},
		{"知らない向き", valid.with(func(v *alertConditionValues) { v.direction = "sideways" })},

		// 種類と向きの組み合わせ
		{"取得価格から・動いた", valid.with(func(v *alertConditionValues) { v.direction = "either" })},
		{"取得価格から・含み損", valid.with(func(v *alertConditionValues) { v.direction = "loss" })},
		{"最高値から・上がった", valid.with(func(v *alertConditionValues) { v.kind = "peak"; v.direction = "up" })},
		{"最高値から・動いた", valid.with(func(v *alertConditionValues) { v.kind = "peak"; v.direction = "either" })},
		{"前の取引日から・以上", valid.with(func(v *alertConditionValues) { v.kind = "prev"; v.direction = "above" })},
		{"含み損益の金額・上がった", valid.with(func(v *alertConditionValues) {
			v.kind = "pnl"
			v.scope = "stock"
			v.stockCode = "7203"
			v.direction = "up"
		})},
		{"含み損益の金額・以上", valid.with(func(v *alertConditionValues) {
			v.kind = "pnl"
			v.scope = "stock"
			v.stockCode = "7203"
			v.direction = "above"
		})},
		{"決めた価格・含み益", valid.with(func(v *alertConditionValues) {
			v.kind = "price"
			v.scope = "stock"
			v.stockCode = "7203"
			v.direction = "gain"
		})},
		{"決めた価格・下がった", valid.with(func(v *alertConditionValues) {
			v.kind = "price"
			v.scope = "stock"
			v.stockCode = "7203"
			v.direction = "down"
		})},

		// 含み損益の金額と決めた価格は、全銘柄にできない
		{"含み損益の金額・全銘柄", valid.with(func(v *alertConditionValues) { v.kind = "pnl"; v.direction = "loss" })},
		{"決めた価格・全銘柄", valid.with(func(v *alertConditionValues) { v.kind = "price"; v.direction = "above" })},

		// 対象と銘柄コードの組
		{"全銘柄なのに銘柄コードがある", valid.with(func(v *alertConditionValues) { v.stockCode = "7203" })},
		{"1銘柄なのに銘柄コードが空", valid.with(func(v *alertConditionValues) { v.scope = "stock" })},

		// 値
		{"値が0", valid.with(func(v *alertConditionValues) { v.threshold = "0" })},
		{"値が負", valid.with(func(v *alertConditionValues) { v.threshold = "-1" })},
		{"値が小数第3位まである", valid.with(func(v *alertConditionValues) { v.threshold = "20.005" })},
		{"値が Infinity", valid.with(func(v *alertConditionValues) { v.threshold = "Infinity" })},
		{"値が上限（10億）を超える", valid.with(func(v *alertConditionValues) { v.threshold = "1000000000.01" })},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tryInsertAlertCondition(t, pool, tc.v)
			assertSQLState(t, err, tc.v, sqlStateCheckViolation)
		})
	}
}

// 4.2: 同じ利用者の、種類・対象・向き・値がすべて同じ条件は、23505（一意制約）で保存できない。NUMERIC の比較なので 20 と 20.00 は同じ。
// 全銘柄の条件（銘柄コードが空）も、1銘柄の条件も同じ（銘柄コードも比べる）。
func Test4_2_同じ条件は値の書き方が違っても保存できない(t *testing.T) {
	t.Parallel()
	pool := newAlertConstraintsDB(t)
	insertStockCondition(t, pool, seedUserA, "price", "7203", "above", "3000")
	valid := validAlertCondition()
	cases := []struct {
		name string
		v    alertConditionValues
	}{
		{"全銘柄・取得価格から・下がった・20（前提と同じ）", valid.with(func(v *alertConditionValues) { v.threshold = "20" })},
		{"同じ値を20.00と書く", valid.with(func(v *alertConditionValues) { v.threshold = "20.00" })},
		{"1銘柄の決めた価格・3000.0", valid.with(func(v *alertConditionValues) {
			v.kind = "price"
			v.scope = "stock"
			v.stockCode = "7203"
			v.direction = "above"
			v.threshold = "3000.0"
		})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tryInsertAlertCondition(t, pool, tc.v)
			assertSQLState(t, err, tc.v, sqlStateUniqueViolation)
		})
	}
}

// 4.2: 存在しない利用者の条件は、23503（外部キー）で保存できない。
func Test4_2_存在しない利用者の条件は保存できない(t *testing.T) {
	t.Parallel()
	pool := newAlertConstraintsDB(t)
	v := validAlertCondition().with(func(v *alertConditionValues) { v.userID = "00000000-0000-0000-0000-0000000000ff" })

	err := tryInsertAlertCondition(t, pool, v)

	assertSQLState(t, err, v, sqlStateForeignKeyViolation)
}

const insertNotificationSQL = "INSERT INTO notifications (user_id, condition_id, kind, scope, condition_stock_code, direction, threshold, " +
	"stock_code, trading_date, close_price, details) " +
	"VALUES ($1::uuid, $2::bigint, 'cost', 'all', '', 'down', 20, $3::text, $4::text::date, 1950, '{}'::jsonb)"

const insertNotificationWithThresholdSQL = "INSERT INTO notifications (user_id, condition_id, kind, scope, condition_stock_code, direction, threshold, " +
	"stock_code, trading_date, close_price, details) " +
	"VALUES ($1::uuid, 9001, 'cost', 'all', '', 'down', $2::text::numeric, '7203', '2026-11-10'::date, 1950, '{}'::jsonb)"

// 4.2（番号9）: 通知の値（threshold）は、alert_conditions と同じく 10億（要件4 の最大。円の含み損益）ちょうどまで入り、10億0.01 は 23514（CHECK）で入らない。
func Test4_2_通知の値は10億ちょうどまで入り超えると保存できない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)

	if err := insertInRolledBackTx(t, pool, insertNotificationWithThresholdSQL, seedUserA, "1000000000"); err != nil {
		t.Errorf("値が 10億ちょうどの通知を入れられない: %v", err)
	}

	err := insertInRolledBackTx(t, pool, insertNotificationWithThresholdSQL, seedUserA, "1000000000.01")
	assertSQLState(t, err, "値が 10億0.01 の通知", sqlStateCheckViolation)
}

// 4.2: 通知は、同じ条件・同じ銘柄・同じ取引日の行を2件持てない（23505）。条件が違う・銘柄が違う・取引日が違う行は入れられる。
// condition_id は外部キーにしないので、alert_conditions にない条件 ID の通知も入れられる（条件を消しても通知を残すため）。
func Test4_2_通知は同じ条件と銘柄と取引日を2件持てない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	if _, err := pool.Exec(t.Context(), insertNotificationSQL, seedUserA, int64(9001), "7203", "2026-11-10"); err != nil {
		t.Fatalf("前提の通知を入れられない（condition_id 9001 は alert_conditions にない。外部キーにしない）: %v", err)
	}

	for _, tc := range []struct {
		name        string
		conditionID int64
		stockCode   string
		date        string
	}{
		{"条件が違う", 9002, "7203", "2026-11-10"},
		{"銘柄が違う", 9001, "6758", "2026-11-10"},
		{"取引日が違う", 9001, "7203", "2026-11-11"},
	} {
		if err := insertInRolledBackTx(t, pool, insertNotificationSQL, seedUserA, tc.conditionID, tc.stockCode, tc.date); err != nil {
			t.Errorf("%s: 通知を入れられない: %v", tc.name, err)
		}
	}

	err := insertInRolledBackTx(t, pool, insertNotificationSQL, seedUserA, int64(9001), "7203", "2026-11-10")
	assertSQLState(t, err, "同じ条件・銘柄・取引日の通知", sqlStateUniqueViolation)
}
