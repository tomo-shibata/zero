package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 要件（docs/requirements/stock-holdings.md）の AC-15（FR-18、FR-19）・AC-15a（FR-20）と、
// プラン（docs/plans/stock-holdings/plan.md）5.1 の AC-15・AC-15a の行、
// 「補助のテスト（バックエンド）」の FR-17〜20（SR-14、TC-31）の行。
// 制約はプラン 4.2 の holdings の行（FK(users)、FK(stocks)、quantity >= 1、acquisition_price > 0、
// acquisition_price = round(acquisition_price, 1)、すべての列が NOT NULL）。
//
// 「保存できない」は、INSERT が PostgreSQL のエラーになり、その SQLSTATE が制約の種類と一致することで確かめる。
// 各テストは、まず正しい行が保存できることを確かめてから、そこから1項目だけ変えた INSERT を試す
// （失敗の理由が、列名の誤りなど制約以外のものでないことを示すため）。

// 制約違反の SQLSTATE（PostgreSQL の Appendix A「PostgreSQL Error Codes」）。
const (
	sqlStateNotNullViolation    = "23502" // not_null_violation
	sqlStateForeignKeyViolation = "23503" // foreign_key_violation
	sqlStateCheckViolation      = "23514" // check_violation
)

// holdingValues は、holdings に入れる1行の値。nil の項目は NULL として入れる。
// acquisitionPrice は円の文字列（例 "0.1"、"2500.05"）で、text から numeric に変換して入れる（書いた桁のまま保存するため）。
type holdingValues struct {
	userID           any
	stockCode        any
	quantity         any
	acquisitionPrice any
}

// validHolding は、AC-15 の正しい行（利用者A・7203・数量1・取得価格 0.1円）。
// 数量と取得価格は、許される値の下限（FR-18 の1以上、FR-19 の0より大きく小数第1位まで）。
func validHolding() holdingValues {
	return holdingValues{userID: seedUserA, stockCode: "7203", quantity: int64(1), acquisitionPrice: "0.1"}
}

// with は、v の1項目だけを change で変えた行を返す（v 自体は変えない）。
func (v holdingValues) with(change func(*holdingValues)) holdingValues {
	change(&v)
	return v
}

// tryInsertHolding は、v を holdings に INSERT し、そのエラーを返す（成功なら nil）。
func tryInsertHolding(ctx context.Context, pool *pgxpool.Pool, v holdingValues) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO holdings (user_id, stock_code, quantity, acquisition_price) VALUES ($1::uuid, $2::text, $3::bigint, $4::text::numeric)",
		v.userID, v.stockCode, v.quantity, v.acquisitionPrice,
	)
	return err
}

// requireSaved は、正しい行 v が保存できることを確かめる（対照）。
func requireSaved(t *testing.T, pool *pgxpool.Pool, v holdingValues) {
	t.Helper()
	if err := tryInsertHolding(t.Context(), pool, v); err != nil {
		t.Fatalf("正しい行 %+v を保存できない（以降の「保存できない」の確かめが意味をなさない）: %v", v, err)
	}
}

// assertRejected は、v の INSERT が SQLSTATE wantCode の PostgreSQL のエラーで失敗する（保存できない）ことを確かめる。
func assertRejected(t *testing.T, pool *pgxpool.Pool, v holdingValues, wantCode string) {
	t.Helper()
	err := tryInsertHolding(t.Context(), pool, v)
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

// AC-15（FR-18、FR-19）: 保有数量0、または取得価格0、または取得価格が小数第2位を含む保有データは保存できない。
// プラン 5.1: 正しい行（A・7203・数量1・取得価格 0.1、および 2500.1）が保存できる。
// そこから1項目だけ変えた INSERT（数量0／取得価格0／取得価格 2500.05）が SQLSTATE 23514 で失敗する。
// 2500.05 は、列を NUMERIC(…,1) にすると黙って 2500.1 に丸められて保存されてしまう値（プラン 4.2）。
func TestAC15_保有数量0や取得価格0や小数第2位を含む取得価格の保有データは保存できない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)

	validMin := validHolding()
	validTenths := validHolding().with(func(v *holdingValues) { v.acquisitionPrice = "2500.1" })
	requireSaved(t, pool, validMin)
	requireSaved(t, pool, validTenths)

	cases := []struct {
		name string
		v    holdingValues
	}{
		{"保有数量0", validMin.with(func(v *holdingValues) { v.quantity = int64(0) })},
		{"取得価格0", validMin.with(func(v *holdingValues) { v.acquisitionPrice = "0" })},
		{"取得価格が小数第2位を含む_2500.05", validTenths.with(func(v *holdingValues) { v.acquisitionPrice = "2500.05" })},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertRejected(t, pool, tc.v, sqlStateCheckViolation)
		})
	}
}

// AC-15a（FR-20）: 銘柄マスタに 1301 が登録されていない → 銘柄コード 1301 の保有データは保存できない。
// プラン 5.1: 銘柄だけ 1301 に変えた INSERT が SQLSTATE 23503 で失敗する。
func TestAC15a_銘柄マスタにない銘柄コードの保有データは保存できない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)

	// Given の確かめ: 1301 は銘柄マスタにない（db/seed/test.sql が入れるのは 7203・6758・9984 だけ）。
	var registered bool
	if err := pool.QueryRow(t.Context(), "SELECT EXISTS (SELECT 1 FROM stocks WHERE code = '1301')").Scan(&registered); err != nil {
		t.Fatalf("銘柄マスタを読めない: %v", err)
	}
	if registered {
		t.Fatal("1301 が銘柄マスタに登録されている（AC-15a の前提を満たさない）")
	}

	valid := validHolding()
	requireSaved(t, pool, valid)

	assertRejected(t, pool, valid.with(func(v *holdingValues) { v.stockCode = "1301" }), sqlStateForeignKeyViolation)
}

// 補助 FR-17〜20（SR-14、TC-31）: AC-15 の正しい行から、user_id・stock_code・quantity・acquisition_price を
// 1つずつ NULL にした INSERT が、それぞれ SQLSTATE 23502 で失敗する。
// CHECK と FK は NULL を検査しないので、NOT NULL がないと NULL で FR-18〜20 を迂回できてしまう（プラン 4.2）。
func TestFR17から20_SR14_TC31_保有データの各列はNULLでは保存できない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)

	valid := validHolding()
	requireSaved(t, pool, valid)

	cases := []struct {
		name string
		v    holdingValues
	}{
		{"user_idがNULL", valid.with(func(v *holdingValues) { v.userID = nil })},
		{"stock_codeがNULL", valid.with(func(v *holdingValues) { v.stockCode = nil })},
		{"quantityがNULL", valid.with(func(v *holdingValues) { v.quantity = nil })},
		{"acquisition_priceがNULL", valid.with(func(v *holdingValues) { v.acquisitionPrice = nil })},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertRejected(t, pool, tc.v, sqlStateNotNullViolation)
		})
	}
}
