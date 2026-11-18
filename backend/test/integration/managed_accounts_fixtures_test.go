package integration

import (
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// おまかせ運用（docs/plans/managed-account/plan.md）の統合テストが使う補助（PR① と PR②）。
// 5章の冒頭: 記録は SQL で入れる（insertManagedValuation）。確かめるときは readManagedValuations で読む。
// 「何も変わらない」は、口座の2つの表（managed_accounts・managed_account_valuations。全利用者の行。作成日時を含む）と、
// 今の5つの表（snapshotFiveTables）の前後を比べる。前後を比べるテストの前提には、Aと Bの口座と記録、Aの売買履歴・終値・並びを
// 少なくとも1件ずつ入れる（givenManagedPool。空の表どうしを比べると、行を消す誤りを見逃すため）。

// storedManagedValuation は、managed_account_valuations の1行のうち、テストで確かめる値（金額は円の整数）。
type storedManagedValuation struct {
	name      string
	date      string // YYYY-MM-DD
	valuation int64
	gain      int64
}

// insertManagedValuation は、利用者 userID の口座 name の、日付 date（"YYYY-MM-DD"）の記録（評価額 valuation・損益 gain。円の整数）を
// SQL で入れる。口座がなければ先に作る。
func insertManagedValuation(t *testing.T, pool *pgxpool.Pool, userID, name, date string, valuation, gain int64) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO managed_accounts (user_id, name) VALUES ($1::uuid, $2::text) ON CONFLICT DO NOTHING",
		userID, name,
	); err != nil {
		t.Fatalf("口座（利用者 %s・%q）を入れられない: %v", userID, name, err)
	}
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO managed_account_valuations (user_id, name, valuation_date, valuation, gain) VALUES ($1::uuid, $2::text, $3::text::date, $4::bigint, $5::bigint)",
		userID, name, date, valuation, gain,
	); err != nil {
		t.Fatalf("記録（利用者 %s・%q・%s・評価額 %d・損益 %d）を入れられない: %v", userID, name, date, valuation, gain, err)
	}
}

// readManagedValuations は、利用者 userID の記録を、名前のバイトの順（Go の文字列比較の順）→ 日付の順に返す。なければ空のスライス。
func readManagedValuations(t *testing.T, pool *pgxpool.Pool, userID string) []storedManagedValuation {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT name, to_char(valuation_date, 'YYYY-MM-DD'), valuation, gain
		   FROM managed_account_valuations
		  WHERE user_id = $1::uuid
		  ORDER BY name COLLATE "C", valuation_date`, userID)
	if err != nil {
		t.Fatalf("利用者 %s の記録を読めない: %v", userID, err)
	}
	got, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (storedManagedValuation, error) {
		var v storedManagedValuation
		err := row.Scan(&v.name, &v.date, &v.valuation, &v.gain)
		return v, err
	})
	if err != nil {
		t.Fatalf("利用者 %s の記録を読めない: %v", userID, err)
	}
	if got == nil {
		got = []storedManagedValuation{}
	}
	return got
}

// readManagedAccountNames は、利用者 userID の口座の名前を、名前のバイトの順で返す。なければ空のスライス。
func readManagedAccountNames(t *testing.T, pool *pgxpool.Pool, userID string) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT name FROM managed_accounts WHERE user_id = $1::uuid ORDER BY name COLLATE "C"`, userID)
	if err != nil {
		t.Fatalf("利用者 %s の口座を読めない: %v", userID, err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("利用者 %s の口座を読めない: %v", userID, err)
	}
	if names == nil {
		names = []string{}
	}
	return names
}

// managedTablesSnapshot は、managed_accounts・managed_account_valuations の中身を、行ごとの JSON にして並べた文字列。
// 作成日時 created_at を含めて漏れなく比べるため、各表を to_jsonb で読む。
type managedTablesSnapshot struct {
	accounts   string
	valuations string
}

// snapshotManagedTables は、口座の2つの表の全利用者のすべての行を、決まった並びで読む。
func snapshotManagedTables(t *testing.T, pool *pgxpool.Pool) managedTablesSnapshot {
	t.Helper()
	return readManagedTablesWhere(t, pool, "true", nil)
}

// snapshotManagedTablesOf は、口座の2つの表の、利用者 userID の行だけを読む。
func snapshotManagedTablesOf(t *testing.T, pool *pgxpool.Pool, userID string) managedTablesSnapshot {
	t.Helper()
	return readManagedTablesWhere(t, pool, "x.user_id = $1::uuid", []any{userID})
}

func readManagedTablesWhere(t *testing.T, pool *pgxpool.Pool, where string, args []any) managedTablesSnapshot {
	t.Helper()
	read := func(table, orderBy string) string {
		t.Helper()
		var s string
		query := "SELECT coalesce(jsonb_agg(to_jsonb(x) ORDER BY " + orderBy + "), '[]'::jsonb)::text FROM " + table + " x WHERE " + where
		if err := pool.QueryRow(t.Context(), query, args...).Scan(&s); err != nil {
			t.Fatalf("%s を読めない: %v", table, err)
		}
		return s
	}
	return managedTablesSnapshot{
		accounts:   read("managed_accounts", `x.user_id, x.name COLLATE "C"`),
		valuations: read("managed_account_valuations", `x.user_id, x.name COLLATE "C", x.valuation_date`),
	}
}

// assertManagedTablesUnchanged は、口座の2つの表（全利用者）が before から何も変わっていない（行も作成日時も値も）ことを確かめる。
func assertManagedTablesUnchanged(t *testing.T, pool *pgxpool.Pool, before managedTablesSnapshot) {
	t.Helper()
	after := snapshotManagedTables(t, pool)
	if after.accounts != before.accounts {
		t.Errorf("managed_accounts が変わった（取り込めなかったときは何も変えない）:\n before %s\n after  %s", before.accounts, after.accounts)
	}
	if after.valuations != before.valuations {
		t.Errorf("managed_account_valuations が変わった（取り込めなかったときは何も変えない）:\n before %s\n after  %s", before.valuations, after.valuations)
	}
}

// managedAndFiveSnapshot は、口座の2つの表と、今の5つの表の前後比較に使う、取り込みの前の中身。
type managedAndFiveSnapshot struct {
	managed managedTablesSnapshot
	five    fiveTableSnapshot
}

// snapshotManagedAndFive は、口座の2つの表と5つの表を読む。
func snapshotManagedAndFive(t *testing.T, pool *pgxpool.Pool) managedAndFiveSnapshot {
	t.Helper()
	return managedAndFiveSnapshot{managed: snapshotManagedTables(t, pool), five: snapshotFiveTables(t, pool)}
}

// assertManagedAndFiveUnchanged は、口座の2つの表と5つの表がすべて before から何も変わっていないことを確かめる。
func assertManagedAndFiveUnchanged(t *testing.T, pool *pgxpool.Pool, before managedAndFiveSnapshot) {
	t.Helper()
	assertManagedTablesUnchanged(t, pool, before.managed)
	assertFiveTablesUnchanged(t, pool, before.five)
}

// givenManagedPool は、取り込みの前提にする DB を作る。Aは WealthNavi（記録1件。2026-11-15・評価額 4,000,000円・損益 1,000,000円）、
// Bは WealthNavi（記録1件。2026-11-16・評価額 9,000,000円・損益 1,000,000円）を持つ。
// Aの売買履歴 7203「買 100株 2,000円 2026-01-10」・7203 の終値 3,000円（2026-10-06）・並び「7203」も入れる
// （取り込めなかったときに、これらが残ることを確かめるため。5章の冒頭）。銘柄マスタは test.sql のまま。
func givenManagedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testdb.New(t)
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-11-15", 4_000_000, 1_000_000)
	insertManagedValuation(t, pool, seedUserB, "WealthNavi", "2026-11-16", 9_000_000, 1_000_000)
	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "2000", "2026-01-10")
	insertClosePrice(t, pool, "7203", "2026-10-06", "3000")
	insertHoldingOrder(t, pool, seedUserA, "7203")
	return pool
}
