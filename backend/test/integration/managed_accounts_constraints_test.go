package integration

import (
	"testing"

	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: プラン（docs/plans/managed-account/plan.md）4.2（managed_accounts・managed_account_valuations の制約）と 5.2「テーブル（統合）」（PR①）。
// 要件定義の要件1（利用者ごと・名前で見分ける・同じ名前は1つだけ・書いたとおりに比べる）・要件2（同じ口座の同じ日付の記録は1件だけ）・
// 要件3（評価額は1以上1兆未満、損益は絶対値が1兆未満、元本は1円以上）が、DB の制約でも守られる。
//   - managed_accounts: 主キー (user_id, name)・name <> ''・user_id の外部キー → users・すべての列が NOT NULL。
//   - managed_account_valuations: 主キー (user_id, name, valuation_date)・(user_id, name) の外部キー → managed_accounts（ON DELETE CASCADE）・
//     valuation BETWEEN 1 AND 999999999999・gain BETWEEN -999999999999 AND 999999999999・valuation - gain >= 1・
//     isfinite(valuation_date) と valuation_date BETWEEN '0001-01-01' AND '9999-12-31'・すべての列が NOT NULL。
//
// 形は stocks・stock_prices の制約のテスト（stocks_prices_constraints_test.go）と同じ: 主キーを持つ表なので、INSERT は最後にロールバックする
// トランザクションの中で1件ずつ行い（insertInRolledBackTx）、SQLSTATE が制約の種類と一致することで「保存できない」を確かめる。
// 前提の行（Aの WealthNavi の口座と、2026-01-10 の記録）だけは、先にコミットして入れる。SQLSTATE の定数は trades_constraints_test.go・
// holding_orders_constraints_test.go のもの。

const insertManagedAccountSQL = "INSERT INTO managed_accounts (user_id, name) VALUES ($1::uuid, $2::text)"

const insertManagedValuationSQL = "INSERT INTO managed_account_valuations (user_id, name, valuation_date, valuation, gain) " +
	"VALUES ($1::uuid, $2::text, $3::text::date, $4::bigint, $5::bigint)"

// managedValuationValues は、managed_account_valuations に入れる1行の値。nil の項目は NULL として入れる。
type managedValuationValues struct {
	userID    any
	name      any
	date      any
	valuation any
	gain      any
}

// validManagedValuation は、前提の記録と主キーが重ならない正しい行（A・WealthNavi・2026-02-01・評価額 1,000円・損益 0円）。
func validManagedValuation() managedValuationValues {
	return managedValuationValues{userID: seedUserA, name: "WealthNavi", date: "2026-02-01", valuation: int64(1000), gain: int64(0)}
}

// with は、v の1項目だけを change で変えた行を返す（v 自体は変えない）。
func (v managedValuationValues) with(change func(*managedValuationValues)) managedValuationValues {
	change(&v)
	return v
}

// 5.2: 正しい行が保存できる。評価額 1・999999999999、損益 -999999999999（評価額1）・999999999998（評価額 999999999999）、
// 評価額 − 損益が 1（1000, 999）、日付 0001-01-01 と 9999-12-31。別の利用者の同じ名前、大文字と小文字だけが違う名前、前後の空白だけが違う名前の口座（要件1）。
func Test要件1_おまかせ運用の正しい行は保存できる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-01-10", 1000, 0)

	valid := validManagedValuation()
	validCases := []struct {
		name string
		v    managedValuationValues
	}{
		{"正しい行", valid},
		{"評価額1", valid.with(func(v *managedValuationValues) { v.valuation = int64(1) })},
		{"評価額999999999999", valid.with(func(v *managedValuationValues) { v.valuation = int64(999_999_999_999) })},
		{"損益-999999999999（評価額1）", valid.with(func(v *managedValuationValues) { v.valuation = int64(1); v.gain = int64(-999_999_999_999) })},
		{"損益999999999998（評価額999999999999。元本1）", valid.with(func(v *managedValuationValues) {
			v.valuation = int64(999_999_999_999)
			v.gain = int64(999_999_999_998)
		})},
		{"元本1（評価額1000・損益999）", valid.with(func(v *managedValuationValues) { v.gain = int64(999) })},
		{"日付0001-01-01", valid.with(func(v *managedValuationValues) { v.date = "0001-01-01" })},
		{"日付9999-12-31", valid.with(func(v *managedValuationValues) { v.date = "9999-12-31" })},
	}
	for _, tc := range validCases {
		if err := insertInRolledBackTx(t, pool, insertManagedValuationSQL, tc.v.userID, tc.v.name, tc.v.date, tc.v.valuation, tc.v.gain); err != nil {
			t.Errorf("%s: 正しい行 %+v を保存できない: %v", tc.name, tc.v, err)
		}
	}

	accountCases := []struct {
		name   string
		userID string
		acct   string
	}{
		{"別の利用者の同じ名前", seedUserB, "WealthNavi"},
		{"大文字と小文字だけが違う名前", seedUserA, "wealthnavi"},
		{"前後の空白だけが違う名前", seedUserA, "WealthNavi "},
		{"全角の空白だけの名前（DB は空文字だけを拒む）", seedUserA, "　"},
	}
	for _, tc := range accountCases {
		if err := insertInRolledBackTx(t, pool, insertManagedAccountSQL, tc.userID, tc.acct); err != nil {
			t.Errorf("%s: 口座（%s・%q）を保存できない: %v", tc.name, tc.userID, tc.acct, err)
		}
	}
}

// 5.2・要件3: 評価額 0・1000000000000、損益 -1000000000000（評価額1）・1000000000000、評価額 − 損益が 0（1000, 1000）・負（1000, 1500）の記録は、
// 23514（CHECK）で保存できない。
func Test要件3_評価額と損益と元本が範囲外の記録は保存できない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-01-10", 1000, 0)

	valid := validManagedValuation()
	cases := []struct {
		name string
		v    managedValuationValues
	}{
		// 元本（評価額 − 損益）は 1 以上にして、評価額の下限の CHECK だけに当てる。
		{"評価額0（損益-1。元本1）", valid.with(func(v *managedValuationValues) { v.valuation = int64(0); v.gain = int64(-1) })},
		{"評価額が負（損益-2。元本1）", valid.with(func(v *managedValuationValues) { v.valuation = int64(-1); v.gain = int64(-2) })},
		{"評価額1兆", valid.with(func(v *managedValuationValues) { v.valuation = int64(1_000_000_000_000) })},
		{"損益-1兆（評価額1）", valid.with(func(v *managedValuationValues) { v.valuation = int64(1); v.gain = int64(-1_000_000_000_000) })},
		{"損益1兆", valid.with(func(v *managedValuationValues) { v.gain = int64(1_000_000_000_000) })},
		{"元本0（1000と1000）", valid.with(func(v *managedValuationValues) { v.gain = int64(1000) })},
		{"元本が負（1000と1500）", valid.with(func(v *managedValuationValues) { v.gain = int64(1500) })},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := insertInRolledBackTx(t, pool, insertManagedValuationSQL, tc.v.userID, tc.v.name, tc.v.date, tc.v.valuation, tc.v.gain)
			assertSQLState(t, err, tc.v, sqlStateCheckViolation)
		})
	}
}

// 5.2・4.2: 日付 'infinity'・'-infinity'・'10000-01-01'・紀元前（0001-12-31 BC）の記録は、23514（CHECK）で保存できない。
func Test要件3_おまかせ運用の記録の日付が範囲外なら保存できない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-01-10", 1000, 0)

	valid := validManagedValuation()
	for _, date := range []string{"infinity", "-infinity", "10000-01-01", "0001-12-31 BC"} {
		t.Run(date, func(t *testing.T) {
			t.Parallel()
			v := valid.with(func(v *managedValuationValues) { v.date = date })
			err := insertInRolledBackTx(t, pool, insertManagedValuationSQL, v.userID, v.name, v.date, v.valuation, v.gain)
			assertSQLState(t, err, v, sqlStateCheckViolation)
		})
	}
}

// 5.2・要件1・要件2: 空の名前の口座は 23514（CHECK）、同じ（利用者、名前）の2つ目の口座は 23505（主キー）、いない利用者の口座は 23503（外部キー）、
// 同じ（利用者、名前、日付）の2つ目の記録は 23505、口座のない記録は 23503（外部キー）、いない利用者の記録も 23503 で、保存できない。
func Test要件1_口座と記録の主キーと外部キーと空の名前の制約(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-01-10", 1000, 0)

	valid := validManagedValuation()
	t.Run("口座_空の名前は保存できない", func(t *testing.T) {
		t.Parallel()
		err := insertInRolledBackTx(t, pool, insertManagedAccountSQL, seedUserA, "")
		assertSQLState(t, err, "口座(A, 空の名前)", sqlStateCheckViolation)
	})
	t.Run("口座_同じ利用者の同じ名前の2つ目は保存できない", func(t *testing.T) {
		t.Parallel()
		err := insertInRolledBackTx(t, pool, insertManagedAccountSQL, seedUserA, "WealthNavi")
		assertSQLState(t, err, "口座(A, WealthNavi)の2つ目", sqlStateUniqueViolation)
	})
	t.Run("口座_いない利用者の口座は保存できない", func(t *testing.T) {
		t.Parallel()
		err := insertInRolledBackTx(t, pool, insertManagedAccountSQL, unknownUserID, "WealthNavi")
		assertSQLState(t, err, "口座(いない利用者, WealthNavi)", sqlStateForeignKeyViolation)
	})
	t.Run("記録_同じ口座の同じ日付の2つ目は保存できない", func(t *testing.T) {
		t.Parallel()
		v := valid.with(func(v *managedValuationValues) { v.date = "2026-01-10" })
		err := insertInRolledBackTx(t, pool, insertManagedValuationSQL, v.userID, v.name, v.date, v.valuation, v.gain)
		assertSQLState(t, err, v, sqlStateUniqueViolation)
	})
	t.Run("記録_口座のない記録は保存できない", func(t *testing.T) {
		t.Parallel()
		v := valid.with(func(v *managedValuationValues) { v.name = "口座なし" })
		err := insertInRolledBackTx(t, pool, insertManagedValuationSQL, v.userID, v.name, v.date, v.valuation, v.gain)
		assertSQLState(t, err, v, sqlStateForeignKeyViolation)
	})
	t.Run("記録_Bに口座がない名前の記録は保存できない", func(t *testing.T) {
		t.Parallel()
		v := valid.with(func(v *managedValuationValues) { v.userID = seedUserB })
		err := insertInRolledBackTx(t, pool, insertManagedValuationSQL, v.userID, v.name, v.date, v.valuation, v.gain)
		assertSQLState(t, err, v, sqlStateForeignKeyViolation)
	})
	t.Run("記録_いない利用者の記録は保存できない", func(t *testing.T) {
		t.Parallel()
		v := valid.with(func(v *managedValuationValues) { v.userID = unknownUserID })
		err := insertInRolledBackTx(t, pool, insertManagedValuationSQL, v.userID, v.name, v.date, v.valuation, v.gain)
		assertSQLState(t, err, v, sqlStateForeignKeyViolation)
	})
}

// 5.2: どの列の NULL も、23502（NOT NULL）で保存できない（口座の2列、記録の5列）。
func Test要件1_おまかせ運用の各列はNULLでは保存できない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-01-10", 1000, 0)

	t.Run("口座_user_idがNULL", func(t *testing.T) {
		t.Parallel()
		err := insertInRolledBackTx(t, pool, insertManagedAccountSQL, nil, "Alpha")
		assertSQLState(t, err, "口座(NULL, Alpha)", sqlStateNotNullViolation)
	})
	t.Run("口座_nameがNULL", func(t *testing.T) {
		t.Parallel()
		err := insertInRolledBackTx(t, pool, insertManagedAccountSQL, seedUserA, nil)
		assertSQLState(t, err, "口座(A, NULL)", sqlStateNotNullViolation)
	})

	valid := validManagedValuation()
	cases := []struct {
		name string
		v    managedValuationValues
	}{
		{"記録_user_idがNULL", valid.with(func(v *managedValuationValues) { v.userID = nil })},
		{"記録_nameがNULL", valid.with(func(v *managedValuationValues) { v.name = nil })},
		{"記録_valuation_dateがNULL", valid.with(func(v *managedValuationValues) { v.date = nil })},
		{"記録_valuationがNULL", valid.with(func(v *managedValuationValues) { v.valuation = nil })},
		{"記録_gainがNULL", valid.with(func(v *managedValuationValues) { v.gain = nil })},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := insertInRolledBackTx(t, pool, insertManagedValuationSQL, tc.v.userID, tc.v.name, tc.v.date, tc.v.valuation, tc.v.gain)
			assertSQLState(t, err, tc.v, sqlStateNotNullViolation)
		})
	}
}

// 4.2・判断4・5.2: 口座を消すと、その記録も消える（ON DELETE CASCADE）。同じ利用者のほかの口座、別の利用者の同じ名前の口座と記録は残る。
func Test要件26_口座を消すとその記録も消える(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-10-31", 3_950_000, 950_000)
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-11-16", 4_000_000, 1_000_000)
	insertManagedValuation(t, pool, seedUserA, "Alpha", "2026-11-16", 500_000, 0)
	insertManagedValuation(t, pool, seedUserB, "WealthNavi", "2026-11-16", 9_000_000, 1_000_000)

	if _, err := pool.Exec(t.Context(), "DELETE FROM managed_accounts WHERE user_id = $1::uuid AND name = $2::text", seedUserA, "WealthNavi"); err != nil {
		t.Fatalf("口座を消せない: %v", err)
	}

	wantA := []storedManagedValuation{{"Alpha", "2026-11-16", 500_000, 0}}
	if got := readManagedValuations(t, pool, seedUserA); len(got) != 1 || got[0] != wantA[0] {
		t.Errorf("Aの記録: got %+v, want %+v（消した口座の記録は消え、Alpha は残る）", got, wantA)
	}
	wantB := []storedManagedValuation{{"WealthNavi", "2026-11-16", 9_000_000, 1_000_000}}
	if got := readManagedValuations(t, pool, seedUserB); len(got) != 1 || got[0] != wantB[0] {
		t.Errorf("Bの記録: got %+v, want %+v（別の利用者の同じ名前の口座は残る）", got, wantB)
	}
}
