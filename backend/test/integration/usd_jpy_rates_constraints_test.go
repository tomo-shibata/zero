package integration

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: ドルを円にまとめる合計の要件（docs/requirements/fx-total.md）の基準14（要件13）と、
// プラン（docs/plans/fx-total/plan.md）4.2（テーブル usd_jpy_rates）・5.1（基準14）・5.2（usd_jpy_rates_constraints_test.go の行）。PR①。
// 制約は 4.2 の表のとおり: rate_date は主キー・isfinite・0001-01-01〜9999-12-31、rate は NOT NULL・0より大きい・小数第2位まで（rate = round(rate, 2)）・有限、
// created_at は NOT NULL・既定 now()。
//   - 保存できない: 同じ日付の2件目（23505）、rate が 0・-1・150.255・NaN・Infinity（23514）、rate_date が infinity・-infinity・'10000-01-01'（23514）、
//     rate・rate_date が NULL（23502）。
//   - 保存できる: 150.25（小数第2位まで）、小数なしの 150。
//
// 形は stocks・stock_prices の制約のテスト（stocks_prices_constraints_test.go）と同じ: 正しい行が保存できることを確かめてから、
// そこから1項目だけ変えた INSERT が制約の種類の SQLSTATE で失敗することを確かめる（insertInRolledBackTx・assertSQLState）。

// usdJpyRateValues は、usd_jpy_rates に入れる1行の値。nil の項目は NULL として入れる。
// rateDate は日付の文字列（例 "2026-11-14"、"infinity"）、rate は数の文字列（例 "150.25"、"NaN"）で、いずれも text から変換して入れる（書いた値のまま保存するため）。
type usdJpyRateValues struct {
	rateDate any
	rate     any
}

// validUsdJpyRate は、正しい行（2026-11-14・150.00円）。
func validUsdJpyRate() usdJpyRateValues {
	return usdJpyRateValues{rateDate: "2026-11-14", rate: "150.00"}
}

func (v usdJpyRateValues) with(change func(*usdJpyRateValues)) usdJpyRateValues {
	change(&v)
	return v
}

const insertUsdJpyRateSQL = "INSERT INTO usd_jpy_rates (rate_date, rate) VALUES ($1::text::date, $2::text::numeric)"

// tryInsertUsdJpyRate は、v を usd_jpy_rates に INSERT し（ロールバックするトランザクションの中で）、そのエラーを返す。
func tryInsertUsdJpyRate(t *testing.T, pool *pgxpool.Pool, v usdJpyRateValues) error {
	t.Helper()
	return insertInRolledBackTx(t, pool, insertUsdJpyRateSQL, v.rateDate, v.rate)
}

// 基準14（要件13）: 2026-11-14 のレート 150.00円が入っている
// → 同じ日付のレートをもう1件入れる、または 0円・-1円のレートを入れると DB が拒否する。小数第3位がある 150.255円も拒否する（丸めて 150.26 として入ることはない）。
// 補助（5.2）: rate の NaN・Infinity、rate_date の infinity・-infinity・'10000-01-01'、rate・rate_date の NULL も拒否する。
// 対照: 150.25 と小数なしの 150 は、同じ日付でなければ入る。
func Test基準14_レートは同じ日付の2件目と0と負の値と小数第3位と特殊な値とNULLでは保存できない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)

	valid := validUsdJpyRate()
	if _, err := pool.Exec(t.Context(), insertUsdJpyRateSQL, valid.rateDate, valid.rate); err != nil {
		t.Fatalf("正しい行 %+v を保存できない（以降の「保存できない」の確かめが意味をなさない）: %v", valid, err)
	}
	// 同じ日付でなければ、小数第2位までの値も小数なしの値も保存できる。
	for _, v := range []usdJpyRateValues{
		valid.with(func(v *usdJpyRateValues) { v.rateDate = "2026-11-13"; v.rate = "150.25" }),
		valid.with(func(v *usdJpyRateValues) { v.rateDate = "2026-11-13"; v.rate = "150" }),
	} {
		if err := tryInsertUsdJpyRate(t, pool, v); err != nil {
			t.Fatalf("正しい行 %+v を保存できない（以降の「保存できない」の確かめが意味をなさない）: %v", v, err)
		}
	}

	otherDate := func(v *usdJpyRateValues) { v.rateDate = "2026-11-13" }
	cases := []struct {
		name     string
		v        usdJpyRateValues
		wantCode string
	}{
		{"同じ日付の2件目", valid, sqlStateUniqueViolation},
		{"同じ日付の2件目はレートが違っても保存できない", valid.with(func(v *usdJpyRateValues) { v.rate = "151.00" }), sqlStateUniqueViolation},
		{"rateが0", valid.with(otherDate).with(func(v *usdJpyRateValues) { v.rate = "0" }), sqlStateCheckViolation},
		{"rateが-1", valid.with(otherDate).with(func(v *usdJpyRateValues) { v.rate = "-1" }), sqlStateCheckViolation},
		{"rateが小数第3位を含む_150.255", valid.with(otherDate).with(func(v *usdJpyRateValues) { v.rate = "150.255" }), sqlStateCheckViolation},
		{"rateがNaN", valid.with(otherDate).with(func(v *usdJpyRateValues) { v.rate = "NaN" }), sqlStateCheckViolation},
		{"rateがInfinity", valid.with(otherDate).with(func(v *usdJpyRateValues) { v.rate = "Infinity" }), sqlStateCheckViolation},
		{"rate_dateがinfinity", valid.with(func(v *usdJpyRateValues) { v.rateDate = "infinity" }), sqlStateCheckViolation},
		{"rate_dateが-infinity", valid.with(func(v *usdJpyRateValues) { v.rateDate = "-infinity" }), sqlStateCheckViolation},
		{"rate_dateが10000-01-01", valid.with(func(v *usdJpyRateValues) { v.rateDate = "10000-01-01" }), sqlStateCheckViolation},
		{"rate_dateがNULL", valid.with(func(v *usdJpyRateValues) { v.rateDate = nil }), sqlStateNotNullViolation},
		{"rateがNULL", valid.with(otherDate).with(func(v *usdJpyRateValues) { v.rate = nil }), sqlStateNotNullViolation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertSQLState(t, tryInsertUsdJpyRate(t, pool, tc.v), tc.v, tc.wantCode)
		})
	}
}
