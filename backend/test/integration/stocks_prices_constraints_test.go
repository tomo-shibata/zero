package integration

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: プラン（docs/plans/stock-holdings/plan.md）4.2 の stocks・stock_prices・holdings の行の制約と、
// 5.1「補助のテスト（バックエンド）」の FR-8・FR-19・FR-20・4.2（PR② の TC-04）の行。
//   - stocks: code と name は空文字不可（23514）、NULL 不可（23502）。
//   - stock_prices: close_price > 0・close_price = round(close_price, 1)・close_price < 'Infinity'・isfinite(trading_date)（23514）、
//     すべての列が NOT NULL（23502）、FK(stocks)（23503）。
//   - holdings: acquisition_price < 'Infinity'（'NaN'・'Infinity' を 23514 で拒否）。
//
// 形は AC-15（holdings_constraints_test.go）と同じ: 正しい行が保存できることを確かめてから、そこから1項目だけ変えた INSERT が
// 制約の種類の SQLSTATE で失敗することを確かめる。SQLSTATE の定数は holdings_constraints_test.go のもの。
//
// stocks と stock_prices は主キーを持つので、正しい行と1項目だけ変えた行が同じ主キーになる。
// 互いに影響しないよう、この2つの表への INSERT は、最後にロールバックするトランザクションの中で1件ずつ行う
// （insertInRolledBackTx）。遅延された制約もその場で検査させるので、INSERT が成功すれば保存できる行である。

// insertInRolledBackTx は、sql を、最後にロールバックするトランザクションの中で実行し、そのエラーを返す（成功なら nil）。
// 実行の後に SET CONSTRAINTS ALL IMMEDIATE で遅延された制約も検査させるので、nil ならコミットしても制約に違反しない行である。
func insertInRolledBackTx(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) error {
	t.Helper()
	ctx := t.Context()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("トランザクションを始められない: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "SET CONSTRAINTS ALL IMMEDIATE"); err != nil {
		return err
	}
	return nil
}

// assertSQLState は、err が SQLSTATE wantCode の PostgreSQL のエラーである（その行は保存できない）ことを確かめる。
// row は失敗のメッセージに出す行の説明。
func assertSQLState(t *testing.T, err error, row any, wantCode string) {
	t.Helper()
	if err == nil {
		t.Errorf("行 %+v が保存できてしまった（want SQLSTATE %s で保存できない）", row, wantCode)
		return
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Errorf("行 %+v: PostgreSQL のエラーではない（want SQLSTATE %s）: %v", row, wantCode, err)
		return
	}
	if pgErr.Code != wantCode {
		t.Errorf("行 %+v の SQLSTATE: got %s（%s）, want %s", row, pgErr.Code, pgErr.Message, wantCode)
	}
}

// ---- stocks ----

// stockValues は、stocks に入れる1行の値。nil の項目は NULL として入れる。
type stockValues struct {
	code any
	name any
}

// validStock は、銘柄マスタの正しい行。seed（7203・6758・9984）と主キーが重ならない銘柄コードにする。
// 130A は要件 FR-5 の例にある英字を含む銘柄コードで、銘柄名はテスト用の仮の名前。
func validStock() stockValues {
	return stockValues{code: "130A", name: "テスト銘柄130A"}
}

func (v stockValues) with(change func(*stockValues)) stockValues {
	change(&v)
	return v
}

// tryInsertStock は、v を stocks に INSERT し（ロールバックするトランザクションの中で）、そのエラーを返す。
func tryInsertStock(t *testing.T, pool *pgxpool.Pool, v stockValues) error {
	t.Helper()
	return insertInRolledBackTx(t, pool, "INSERT INTO stocks (code, name) VALUES ($1::text, $2::text)", v.code, v.name)
}

// 補助 FR-20・4.2（PR② の TC-04）: 銘柄マスタの code・name が空文字なら 23514、NULL なら 23502 で保存できない。
func TestFR20_PR2_TC04_銘柄マスタの銘柄コードと銘柄名は空文字やNULLでは保存できない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)

	valid := validStock()
	if err := tryInsertStock(t, pool, valid); err != nil {
		t.Fatalf("正しい行 %+v を保存できない（以降の「保存できない」の確かめが意味をなさない）: %v", valid, err)
	}

	cases := []struct {
		name     string
		v        stockValues
		wantCode string
	}{
		{"codeが空文字", valid.with(func(v *stockValues) { v.code = "" }), sqlStateCheckViolation},
		{"nameが空文字", valid.with(func(v *stockValues) { v.name = "" }), sqlStateCheckViolation},
		{"codeがNULL", valid.with(func(v *stockValues) { v.code = nil }), sqlStateNotNullViolation},
		{"nameがNULL", valid.with(func(v *stockValues) { v.name = nil }), sqlStateNotNullViolation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertSQLState(t, tryInsertStock(t, pool, tc.v), tc.v, tc.wantCode)
		})
	}
}

// ---- stock_prices ----

// stockPriceValues は、stock_prices に入れる1行の値。nil の項目は NULL として入れる。
// closePrice は円の文字列（例 "3000.1"、"NaN"）、tradingDate は日付の文字列（例 "2026-10-06"、"infinity"）、
// fetchedAt は時刻の文字列で、いずれも text から変換して入れる（書いた値のまま保存するため）。
type stockPriceValues struct {
	stockCode   any
	tradingDate any
	closePrice  any
	fetchedAt   any
}

// validStockPrice は、終値の正しい行（7203・2026-10-06・3,000.1円）。小数第1位までの単価は保存できる（4.2）。
// fetched_at は、NULL にするケースのために列を明示して入れる（値は要件の取得時刻 18:00 の日本時間）。
func validStockPrice() stockPriceValues {
	return stockPriceValues{stockCode: "7203", tradingDate: "2026-10-06", closePrice: "3000.1", fetchedAt: "2026-10-06T18:00:00+09:00"}
}

func (v stockPriceValues) with(change func(*stockPriceValues)) stockPriceValues {
	change(&v)
	return v
}

// tryInsertStockPrice は、v を stock_prices に INSERT し（ロールバックするトランザクションの中で）、そのエラーを返す。
func tryInsertStockPrice(t *testing.T, pool *pgxpool.Pool, v stockPriceValues) error {
	t.Helper()
	return insertInRolledBackTx(t, pool,
		"INSERT INTO stock_prices (stock_code, trading_date, close_price, fetched_at) VALUES ($1::text, $2::text::date, $3::text::numeric, $4::text::timestamptz)",
		v.stockCode, v.tradingDate, v.closePrice, v.fetchedAt,
	)
}

// 補助 FR-8・FR-19・FR-20・4.2（PR② の TC-04）: 終値の正しい行（3,000.1円と、下限の 0.1円）が保存できる。
// そこから1項目だけ変えた行は保存できない。close_price が 0・3000.05・'NaN'・'Infinity' と、trading_date が 'infinity' は 23514。
// 各列が NULL は 23502。銘柄が銘柄マスタにない 1301 は 23503。
func TestFR8_FR19_FR20_PR2_TC04_終値は0や小数第2位やNaNや無限大や無限の取引日やNULLや銘柄マスタにない銘柄では保存できない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)

	valid := validStockPrice()
	validMin := valid.with(func(v *stockPriceValues) { v.closePrice = "0.1" })
	for _, v := range []stockPriceValues{valid, validMin} {
		if err := tryInsertStockPrice(t, pool, v); err != nil {
			t.Fatalf("正しい行 %+v を保存できない（以降の「保存できない」の確かめが意味をなさない）: %v", v, err)
		}
	}

	cases := []struct {
		name     string
		v        stockPriceValues
		wantCode string
	}{
		{"close_priceが0", valid.with(func(v *stockPriceValues) { v.closePrice = "0" }), sqlStateCheckViolation},
		{"close_priceが小数第2位を含む_3000.05", valid.with(func(v *stockPriceValues) { v.closePrice = "3000.05" }), sqlStateCheckViolation},
		{"close_priceがNaN", valid.with(func(v *stockPriceValues) { v.closePrice = "NaN" }), sqlStateCheckViolation},
		{"close_priceがInfinity", valid.with(func(v *stockPriceValues) { v.closePrice = "Infinity" }), sqlStateCheckViolation},
		{"trading_dateがinfinity", valid.with(func(v *stockPriceValues) { v.tradingDate = "infinity" }), sqlStateCheckViolation},
		{"stock_codeがNULL", valid.with(func(v *stockPriceValues) { v.stockCode = nil }), sqlStateNotNullViolation},
		{"trading_dateがNULL", valid.with(func(v *stockPriceValues) { v.tradingDate = nil }), sqlStateNotNullViolation},
		{"close_priceがNULL", valid.with(func(v *stockPriceValues) { v.closePrice = nil }), sqlStateNotNullViolation},
		{"fetched_atがNULL", valid.with(func(v *stockPriceValues) { v.fetchedAt = nil }), sqlStateNotNullViolation},
		{"銘柄マスタにない銘柄1301", valid.with(func(v *stockPriceValues) { v.stockCode = "1301" }), sqlStateForeignKeyViolation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertSQLState(t, tryInsertStockPrice(t, pool, tc.v), tc.v, tc.wantCode)
		})
	}
}

// ---- holdings ----

// 補助 FR-19・4.2（PR② の TC-04）: AC-15 の正しい行から、取得価格だけを 'NaN'・'Infinity' にした保有データは 23514 で保存できない
// （PostgreSQL では NaN・Infinity が「> 0」と「= round(…, 1)」を通ってしまうため、4.2 で「< 'Infinity'」を足した）。
// holdings は主キーが行ごとの連番で重ならないので、AC-15 と同じヘルパー（コミットする INSERT）を使う。
func TestFR19_PR2_TC04_取得価格がNaNや無限大の保有データは保存できない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)

	valid := validHolding()
	requireSaved(t, pool, valid)

	cases := []struct {
		name string
		v    holdingValues
	}{
		{"acquisition_priceがNaN", valid.with(func(v *holdingValues) { v.acquisitionPrice = "NaN" })},
		{"acquisition_priceがInfinity", valid.with(func(v *holdingValues) { v.acquisitionPrice = "Infinity" })},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertRejected(t, pool, tc.v, sqlStateCheckViolation)
		})
	}
}
