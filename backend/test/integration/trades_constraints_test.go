package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: プラン（docs/plans/trade-history/plan.md）4.2（00005 の trades の制約）と 5.3「trades の DB の制約」（PR①）。
// 元の保有株式一覧の AC-15・AC-15a・FR-17〜20（NULL）と、holdings の NaN・Infinity のテストは、5.2 のとおりこのファイルに置き換えた。
// 制約は 4.2 の trades の行: FK(users)・FK(stocks)・side IN ('buy', 'sell')・quantity >= 1・price > 0・price = round(price, 2)・
// price < 'Infinity'・isfinite(trade_date)・すべての列が NOT NULL。
//   - 保存できない: side が hold、株数0、価格 0・NaN・Infinity・0.001、取引日 infinity、銘柄マスタにない銘柄、いない利用者。
//   - 保存できる: 価格 0.01（円の「小数第1位まで」は銘柄の通貨を見ないと決まらないので、DB では確かめない。判断4）。
//
// 「保存できない」は、INSERT が PostgreSQL のエラーになり、その SQLSTATE が制約の種類と一致することで確かめる。
// 各テストは、まず正しい行が保存できることを確かめてから、そこから1項目だけ変えた INSERT を試す
// （失敗の理由が、列名の誤りなど制約以外のものでないことを示すため）。
// SQLSTATE の定数は、stocks_prices_constraints_test.go も使う。

// 制約違反の SQLSTATE（PostgreSQL の Appendix A「PostgreSQL Error Codes」）。
const (
	sqlStateNotNullViolation    = "23502" // not_null_violation
	sqlStateForeignKeyViolation = "23503" // foreign_key_violation
	sqlStateCheckViolation      = "23514" // check_violation
)

// unknownUserID は、users にいない利用者の ID（test.sql の利用者は A・B だけ）。
const unknownUserID = "00000000-0000-0000-0000-0000000000ff"

// tradeValues は、trades に入れる1行の値。nil の項目は NULL として入れる。
// price は円の文字列（例 "0.01"、"2500.05"）、tradeDate は "YYYY-MM-DD"（または "infinity"）で、text から変換して入れる（書いた桁のまま保存するため）。
type tradeValues struct {
	userID    any
	stockCode any
	side      any
	quantity  any
	price     any
	tradeDate any
}

// validTrade は、正しい行（利用者A・7203・買い・1株・価格 0.01・2026-01-10）。
// 株数と価格は、DB が許す値の下限（株数1以上、価格は0より大きく小数第2位まで）。
func validTrade() tradeValues {
	return tradeValues{userID: seedUserA, stockCode: "7203", side: "buy", quantity: int64(1), price: "0.01", tradeDate: "2026-01-10"}
}

// with は、v の1項目だけを change で変えた行を返す（v 自体は変えない）。
func (v tradeValues) with(change func(*tradeValues)) tradeValues {
	change(&v)
	return v
}

// tryInsertTrade は、v を trades に INSERT し、そのエラーを返す（成功なら nil）。
func tryInsertTrade(ctx context.Context, pool *pgxpool.Pool, v tradeValues) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO trades (user_id, stock_code, side, quantity, price, trade_date) VALUES ($1::uuid, $2::text, $3::text, $4::bigint, $5::text::numeric, $6::text::date)",
		v.userID, v.stockCode, v.side, v.quantity, v.price, v.tradeDate,
	)
	return err
}

// requireTradeSaved は、正しい行 v が保存できることを確かめる（対照）。
func requireTradeSaved(t *testing.T, pool *pgxpool.Pool, v tradeValues) {
	t.Helper()
	if err := tryInsertTrade(t.Context(), pool, v); err != nil {
		t.Fatalf("正しい行 %+v を保存できない（以降の「保存できない」の確かめが意味をなさない）: %v", v, err)
	}
}

// assertTradeRejected は、v の INSERT が SQLSTATE wantCode の PostgreSQL のエラーで失敗する（保存できない）ことを確かめる。
func assertTradeRejected(t *testing.T, pool *pgxpool.Pool, v tradeValues, wantCode string) {
	t.Helper()
	err := tryInsertTrade(t.Context(), pool, v)
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

// 5.3: 正しい行（A・7203・買い・1株・価格 0.01）が保存できる。売り（sell）、価格 2500.05（小数第2位。円の精度は DB では見ない。判断4）、
// 株数 9999999 も保存できる。
func Test補助_5_3_売買履歴の正しい行は保存できる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)

	requireTradeSaved(t, pool, validTrade())
	requireTradeSaved(t, pool, validTrade().with(func(v *tradeValues) { v.side = "sell" }))
	requireTradeSaved(t, pool, validTrade().with(func(v *tradeValues) { v.price = "2500.05" }))
	requireTradeSaved(t, pool, validTrade().with(func(v *tradeValues) { v.quantity = int64(9999999) }))
}

// 5.3: side が hold、株数0（や負）、価格 0・負・NaN・Infinity・-Infinity・0.001（小数第3位）、取引日 infinity の売買履歴は、23514（CHECK）で保存できない。
// 0.001 は、列を NUMERIC(…, 2) にすると黙って 0.00 に丸められて保存されてしまう値なので、CHECK で拒む（4.2）。
func Test補助_5_3_売買履歴の側と株数と価格と取引日が制約に合わなければ保存できない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	valid := validTrade()
	requireTradeSaved(t, pool, valid)

	cases := []struct {
		name string
		v    tradeValues
	}{
		{"sideがhold", valid.with(func(v *tradeValues) { v.side = "hold" })},
		{"sideが空文字", valid.with(func(v *tradeValues) { v.side = "" })},
		{"株数0", valid.with(func(v *tradeValues) { v.quantity = int64(0) })},
		{"株数が負", valid.with(func(v *tradeValues) { v.quantity = int64(-1) })},
		{"価格0", valid.with(func(v *tradeValues) { v.price = "0" })},
		{"価格が負", valid.with(func(v *tradeValues) { v.price = "-1" })},
		{"価格がNaN", valid.with(func(v *tradeValues) { v.price = "NaN" })},
		{"価格がInfinity", valid.with(func(v *tradeValues) { v.price = "Infinity" })},
		{"価格が-Infinity", valid.with(func(v *tradeValues) { v.price = "-Infinity" })},
		{"価格が小数第3位を含む_0.001", valid.with(func(v *tradeValues) { v.price = "0.001" })},
		{"価格が小数第3位を含む_2500.005", valid.with(func(v *tradeValues) { v.price = "2500.005" })},
		{"取引日がinfinity", valid.with(func(v *tradeValues) { v.tradeDate = "infinity" })},
		{"取引日が-infinity", valid.with(func(v *tradeValues) { v.tradeDate = "-infinity" })},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertTradeRejected(t, pool, tc.v, sqlStateCheckViolation)
		})
	}
}

// 5.3: 銘柄マスタにない銘柄（1301）といない利用者の売買履歴は、23503（外部キー）で保存できない。
// 1301 は銘柄マスタにない（db/seed/test.sql が入れるのは 7203・6758・9984 だけ）。
func Test補助_5_3_銘柄マスタにない銘柄やいない利用者の売買履歴は保存できない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)

	// Given の確かめ: 1301 は銘柄マスタにない。
	var registered bool
	if err := pool.QueryRow(t.Context(), "SELECT EXISTS (SELECT 1 FROM stocks WHERE code = '1301')").Scan(&registered); err != nil {
		t.Fatalf("銘柄マスタを読めない: %v", err)
	}
	if registered {
		t.Fatal("1301 が銘柄マスタに登録されている（このテストの前提を満たさない）")
	}

	valid := validTrade()
	requireTradeSaved(t, pool, valid)

	assertTradeRejected(t, pool, valid.with(func(v *tradeValues) { v.stockCode = "1301" }), sqlStateForeignKeyViolation)
	assertTradeRejected(t, pool, valid.with(func(v *tradeValues) { v.userID = unknownUserID }), sqlStateForeignKeyViolation)
}

// 5.3（元の FR-17〜20）: 正しい行から、すべての列を1つずつ NULL にした INSERT が、それぞれ SQLSTATE 23502（NOT NULL）で失敗する。
// CHECK と FK は NULL を検査しないので、NOT NULL がないと NULL で決まりを迂回できてしまう（4.2）。
func Test補助_5_3_売買履歴の各列はNULLでは保存できない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	valid := validTrade()
	requireTradeSaved(t, pool, valid)

	cases := []struct {
		name string
		v    tradeValues
	}{
		{"user_idがNULL", valid.with(func(v *tradeValues) { v.userID = nil })},
		{"stock_codeがNULL", valid.with(func(v *tradeValues) { v.stockCode = nil })},
		{"sideがNULL", valid.with(func(v *tradeValues) { v.side = nil })},
		{"quantityがNULL", valid.with(func(v *tradeValues) { v.quantity = nil })},
		{"priceがNULL", valid.with(func(v *tradeValues) { v.price = nil })},
		{"trade_dateがNULL", valid.with(func(v *tradeValues) { v.tradeDate = nil })},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertTradeRejected(t, pool, tc.v, sqlStateNotNullViolation)
		})
	}
}

// 5.5（PR②。00006 で trades.trade_date に CHECK (trade_date BETWEEN '0001-01-01' AND '9999-12-31') を足す）:
// 取引日 0001-01-01 と 9999-12-31 は保存できる。10000-01-01（年が5桁）と紀元前の日付（0001-12-31 BC）は 23514（CHECK）で保存できない
// （年が4桁でない日付は、文字列の比較で並び順を誤るため。PR① のステップ6 のエッジケースの指摘2-1）。
func Test補助_5_5_売買履歴の取引日は0001_01_01から9999_12_31の範囲だけ保存できる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	valid := validTrade()

	requireTradeSaved(t, pool, valid.with(func(v *tradeValues) { v.tradeDate = "0001-01-01" }))
	requireTradeSaved(t, pool, valid.with(func(v *tradeValues) { v.tradeDate = "9999-12-31" }))

	cases := []struct {
		name string
		v    tradeValues
	}{
		{"取引日が10000-01-01（年が5桁）", valid.with(func(v *tradeValues) { v.tradeDate = "10000-01-01" })},
		{"取引日が紀元前の0001-12-31_BC", valid.with(func(v *tradeValues) { v.tradeDate = "0001-12-31 BC" })},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertTradeRejected(t, pool, tc.v, sqlStateCheckViolation)
		})
	}
}
