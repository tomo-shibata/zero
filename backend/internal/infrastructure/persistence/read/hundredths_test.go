package read

import (
	"math/big"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// 仕様: プラン（docs/plans/trade-history/plan.md）4.7（PR②）の
// 「hundredthsFromNumeric(n pgtype.Numeric) (int64, error) が tenthsFromNumeric を置き換える
// （0.01 より細かい値と int64 を超える値はエラー）」と、5.3（PR②）の「読み取りの hundredthsFromNumeric（単体）: 0.001 や int64 を超える値はエラー」。
// 価格の列は桁数を指定しない NUMERIC なので、書いた桁（scale）のまま保存される。pgx は「整数 Int × 10^Exp」で返し、
// 同じ値でも Int と Exp の組は1通りに決まらない。どの組でも 0.01 単位の値（Int × 10^(Exp+2)）を求める。
// 内部の関数なので、同じパッケージのテストにする。

// numeric は、整数 int × 10^exp の有限の NUMERIC を作る。
func numeric(t *testing.T, digits string, exp int32) pgtype.Numeric {
	t.Helper()
	n, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		t.Fatalf("テストの誤り: %q は整数ではない", digits)
	}
	return pgtype.Numeric{Int: n, Exp: exp, Valid: true}
}

// 0.01 単位に直せる値は、Int と Exp の組によらず同じ値になる。
func Test補助_hundredthsFromNumericはIntとExpの組によらず0_01単位の値を返す(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		digits string
		exp    int32
		want   int64
	}{
		{"2500（scale 0）", "2500", 0, 250000},
		{"2500.0（scale 1）", "25000", -1, 250000},
		{"2500.00（scale 2）", "250000", -2, 250000},
		{"150.25（scale 2）", "15025", -2, 15025},
		{"3000.10（末尾が0の scale 2）", "300010", -2, 300010},
		{"0.01（最小）", "1", -2, 1},
		{"0.10（0.1円）", "10", -2, 10},
		{"2500（Int 25・Exp 2）", "25", 2, 250000},
		{"0.1500（scale 4。末尾の0は細かい値ではない）", "1500", -4, 15},
		{"0", "0", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := hundredthsFromNumeric(numeric(t, tc.digits, tc.exp))

			if err != nil {
				t.Fatalf("hundredthsFromNumeric: got err %v, want nil", err)
			}
			if got != tc.want {
				t.Errorf("hundredthsFromNumeric: got %d, want %d", got, tc.want)
			}
		})
	}
}

// 5.3: 0.01 より細かい値（0.001 など）は、丸めたり切ったりせずにエラーにする。
func Test補助_hundredthsFromNumericは0_01より細かい値をエラーにする(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		digits string
		exp    int32
	}{
		{"0.001", "1", -3},
		{"2500.005", "2500005", -3},
		{"150.255", "150255", -3},
		{"0.0001", "1", -4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := hundredthsFromNumeric(numeric(t, tc.digits, tc.exp))

			if err == nil {
				t.Errorf("hundredthsFromNumeric: got (%d, nil), want エラー（0.01 より細かい値）", got)
			}
		})
	}
}

// 5.3: int64 を超える値はエラー。境界: 0.01 単位で 9223372036854775800 は収まり、9223372036854775900 は収まらない
// （Int 92233720368547758 と 92233720368547759。どちらも Exp 0 で、0.01 単位にすると 100 倍）。
func Test補助_hundredthsFromNumericはint64を超える値をエラーにする(t *testing.T) {
	t.Parallel()

	t.Run("収まる側の境界", func(t *testing.T) {
		t.Parallel()

		got, err := hundredthsFromNumeric(numeric(t, "92233720368547758", 0))

		if err != nil {
			t.Fatalf("hundredthsFromNumeric: got err %v, want nil", err)
		}
		if got != 9223372036854775800 {
			t.Errorf("hundredthsFromNumeric: got %d, want 9223372036854775800", got)
		}
	})

	cases := []struct {
		name   string
		digits string
		exp    int32
	}{
		{"境界の1つ上", "92233720368547759", 0},
		{"10の17乗", "100000000000000000", 0},
		{"Exp が大きい", "1", 30},
	}
	for _, tc := range cases {
		t.Run(tc.name+"はエラー", func(t *testing.T) {
			t.Parallel()

			got, err := hundredthsFromNumeric(numeric(t, tc.digits, tc.exp))

			if err == nil {
				t.Errorf("hundredthsFromNumeric: got (%d, nil), want エラー（int64 を超える）", got)
			}
		})
	}
}

// NULL・NaN・無限大はエラー（DB の CHECK があるので、正しく保存された値では起きない）。
func Test補助_hundredthsFromNumericはNULLとNaNと無限大をエラーにする(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		n    pgtype.Numeric
	}{
		{"NULL", pgtype.Numeric{Valid: false}},
		{"NaN", pgtype.Numeric{NaN: true, Valid: true}},
		{"Infinity", pgtype.Numeric{InfinityModifier: pgtype.Infinity, Valid: true}},
		{"-Infinity", pgtype.Numeric{InfinityModifier: pgtype.NegativeInfinity, Valid: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := hundredthsFromNumeric(tc.n)

			if err == nil {
				t.Errorf("hundredthsFromNumeric: got (%d, nil), want エラー", got)
			}
		})
	}
}
