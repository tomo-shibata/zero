package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: プラン（docs/plans/stock-split/plan.md）4.1（テーブル stock_splits）と 5.2「テーブル」（PR①）。
// 制約は 4.1 の表のとおり: stock_code は NOT NULL・空でない、split_date は NOT NULL・isfinite・0001-01-01〜9999-12-31、
// ratio は NOT NULL・2〜1000、created_at は NOT NULL・既定 now()、主キーは (stock_code, split_date)。銘柄マスタへの外部キーは付けない（要件11）。
//   - 保存できない: ratio が 1・1001、stock_code が空、split_date が infinity、同じ (stock_code, split_date) の2行目。
//   - 保存できる: 銘柄マスタにない銘柄コード、ratio の境界 2・1000。
//
// 「保存できない」は、INSERT が PostgreSQL のエラーになり、その SQLSTATE が制約の種類と一致することで確かめる
// （SQLSTATE の定数は trades_constraints_test.go・holding_orders_constraints_test.go）。
// 各テストは、まず正しい行が保存できることを確かめてから、そこから1項目だけ変えた INSERT を試す
// （失敗の理由が、列名の誤りなど制約以外のものでないことを示すため）。

// splitValues は、stock_splits に入れる1行の値。nil の項目は NULL として入れる。splitDate は "YYYY-MM-DD"（または "infinity"）で、text から変換して入れる。
type splitValues struct {
	stockCode any
	splitDate any
	ratio     any
}

// validSplit は、正しい行（7203・2026-03-31・ratio 3）。
func validSplit() splitValues {
	return splitValues{stockCode: "7203", splitDate: "2026-03-31", ratio: int32(3)}
}

func (v splitValues) with(change func(*splitValues)) splitValues {
	change(&v)
	return v
}

// tryInsertSplit は、v を stock_splits に INSERT し、そのエラーを返す（成功なら nil）。
func tryInsertSplit(ctx context.Context, pool *pgxpool.Pool, v splitValues) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO stock_splits (stock_code, split_date, ratio) VALUES ($1::text, $2::text::date, $3::integer)",
		v.stockCode, v.splitDate, v.ratio,
	)
	return err
}

// requireSplitSaved は、正しい行 v が保存できることを確かめる（対照）。
func requireSplitSaved(t *testing.T, pool *pgxpool.Pool, v splitValues) {
	t.Helper()
	if err := tryInsertSplit(t.Context(), pool, v); err != nil {
		t.Fatalf("正しい行 %+v を保存できない（以降の「保存できない」の確かめが意味をなさない）: %v", v, err)
	}
}

// assertSplitRejected は、v の INSERT が SQLSTATE wantCode の PostgreSQL のエラーで失敗する（保存できない）ことを確かめる。
func assertSplitRejected(t *testing.T, pool *pgxpool.Pool, v splitValues, wantCode string) {
	t.Helper()
	err := tryInsertSplit(t.Context(), pool, v)
	if err == nil {
		t.Errorf("行 %+v が保存できてしまった（want SQLSTATE %s で保存できない）", v, wantCode)
		return
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Errorf("行 %+v: PostgreSQL のエラーではない（want SQLSTATE %s）: %v", v, wantCode, err)
		return
	}
	if pgErr.Code != wantCode {
		t.Errorf("行 %+v の SQLSTATE: got %s（%s）, want %s", v, pgErr.Code, pgErr.Message, wantCode)
	}
}

// 5.2: 正しい行が保存できる。ratio の境界（2・1000）、銘柄マスタにない銘柄コード（NVDA。要件11）、同じ銘柄の別の分割の日も保存できる。
func Test補助_5_2_分割の正しい行は保存できる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)

	requireSplitSaved(t, pool, validSplit())
	requireSplitSaved(t, pool, validSplit().with(func(v *splitValues) { v.stockCode = "6758"; v.ratio = int32(2) }))
	requireSplitSaved(t, pool, validSplit().with(func(v *splitValues) { v.stockCode = "9984"; v.ratio = int32(1000) }))
	requireSplitSaved(t, pool, validSplit().with(func(v *splitValues) { v.stockCode = "NVDA"; v.splitDate = "2024-06-10" }))
	requireSplitSaved(t, pool, validSplit().with(func(v *splitValues) { v.splitDate = "2026-08-01" }))
}

// 5.2: ratio が 1・1001（範囲外）、0・負の値、stock_code が空、split_date が infinity・-infinity・'10000-01-01'（年の範囲外）の行は、23514（CHECK）で保存できない。
func Test補助_5_2_分割後の株数と銘柄コードと分割の日が制約に合わなければ保存できない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	valid := validSplit()
	requireSplitSaved(t, pool, valid)

	cases := []struct {
		name string
		v    splitValues
	}{
		{"ratioが1", valid.with(func(v *splitValues) { v.stockCode = "6758"; v.ratio = int32(1) })},
		{"ratioが1001", valid.with(func(v *splitValues) { v.stockCode = "6758"; v.ratio = int32(1001) })},
		{"ratioが0", valid.with(func(v *splitValues) { v.stockCode = "6758"; v.ratio = int32(0) })},
		{"ratioが負", valid.with(func(v *splitValues) { v.stockCode = "6758"; v.ratio = int32(-2) })},
		{"stock_codeが空", valid.with(func(v *splitValues) { v.stockCode = "" })},
		{"split_dateがinfinity", valid.with(func(v *splitValues) { v.stockCode = "6758"; v.splitDate = "infinity" })},
		{"split_dateが-infinity", valid.with(func(v *splitValues) { v.stockCode = "6758"; v.splitDate = "-infinity" })},
		{"split_dateが10000-01-01", valid.with(func(v *splitValues) { v.stockCode = "6758"; v.splitDate = "10000-01-01" })},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertSplitRejected(t, pool, tc.v, sqlStateCheckViolation)
		})
	}
}

// 5.2・4.1: 同じ (stock_code, split_date) の2行目は、ratio が違っても保存できない（主キー。23505）。
// 同じ銘柄でも分割の日が違えば保存できる（上のテスト）。
func Test補助_5_2_同じ銘柄と分割の日の2行目は保存できない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	requireSplitSaved(t, pool, validSplit())

	assertSplitRejected(t, pool, validSplit(), sqlStateUniqueViolation)
	assertSplitRejected(t, pool, validSplit().with(func(v *splitValues) { v.ratio = int32(5) }), sqlStateUniqueViolation)
}

// 4.1: stock_code・split_date・ratio は NOT NULL（23502）。
func Test補助_4_1_分割のすべての列はNOT_NULLである(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	valid := validSplit()
	requireSplitSaved(t, pool, valid)

	cases := []struct {
		name string
		v    splitValues
	}{
		{"stock_codeがNULL", valid.with(func(v *splitValues) { v.stockCode = nil })},
		{"split_dateがNULL", valid.with(func(v *splitValues) { v.stockCode = "6758"; v.splitDate = nil })},
		{"ratioがNULL", valid.with(func(v *splitValues) { v.stockCode = "6758"; v.ratio = nil })},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertSplitRejected(t, pool, tc.v, sqlStateNotNullViolation)
		})
	}
}

// 4.1: 銘柄マスタにない銘柄コードも入る（銘柄マスタへの外部キーを付けない。要件11）。入れた行は、銘柄マスタに銘柄を足さずに読める。
func Test要件11_銘柄マスタにない銘柄コードの分割も保存できる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)

	insertStockSplit(t, pool, "NVDA", "2024-06-10", 10)

	var stockCount, splitCount int
	if err := pool.QueryRow(t.Context(),
		"SELECT (SELECT count(*) FROM stocks WHERE code = 'NVDA'), (SELECT count(*) FROM stock_splits WHERE stock_code = 'NVDA')",
	).Scan(&stockCount, &splitCount); err != nil {
		t.Fatalf("件数を読めない: %v", err)
	}
	if stockCount != 0 || splitCount != 1 {
		t.Errorf("NVDA の件数（銘柄マスタ・分割）: got %d・%d, want 0・1", stockCount, splitCount)
	}
}
