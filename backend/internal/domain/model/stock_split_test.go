package model_test

import (
	"errors"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// 仕様: 株式分割の要件（docs/requirements/stock-split.md）の要件1〜3（銘柄コード・分割の日・分割後の株数。分割後の株数は2以上1,000以下、
// 分割の日は取り込んだ日（日本時間）以前）と、プラン（docs/plans/stock-split/plan.md）4.8 の PR① の境界
// （StockSplit・CheckSplitRatio・CheckSplitDate・NewStockSplit・ErrSplitRatioTooSmall・ErrSplitRatioTooLarge・ErrFutureSplitDate）、
// 5.2「domain の決まり」。「今日」は 2026-11-16（日本時間。trade_test.go の today）。

// 要件2: 分割後の株数は2以上1,000以下の整数。境界は 1・2・1000・1001。
func Test要件2_CheckSplitRatioは2以上1000以下なら通し範囲外はエラーを返す(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		ratio int64
		want  error
	}{
		{"負の値は小さすぎる", -2, model.ErrSplitRatioTooSmall},
		{"0は小さすぎる", 0, model.ErrSplitRatioTooSmall},
		{"1は小さすぎる", 1, model.ErrSplitRatioTooSmall},
		{"2は通る", 2, nil},
		{"1000は通る", 1000, nil},
		{"1001は大きすぎる", 1001, model.ErrSplitRatioTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := model.CheckSplitRatio(tc.ratio); !errors.Is(got, tc.want) {
				t.Errorf("CheckSplitRatio(%d): got %v, want %v", tc.ratio, got, tc.want)
			}
		})
	}
}

// 要件2: 分割の日は今日（日本時間）以前。今日 2026-11-16 は通り、翌日 2026-11-17 は ErrFutureSplitDate。
func Test要件2_CheckSplitDateは今日以前なら通し今日より後はエラーを返す(t *testing.T) {
	t.Parallel()
	if got := model.CheckSplitDate("2026-11-16", today); got != nil {
		t.Errorf("CheckSplitDate(2026-11-16, %s): got %v, want nil（今日は通る）", today, got)
	}
	if got := model.CheckSplitDate("2026-11-15", today); got != nil {
		t.Errorf("CheckSplitDate(2026-11-15, %s): got %v, want nil（昨日は通る）", today, got)
	}
	if got := model.CheckSplitDate("2026-11-17", today); !errors.Is(got, model.ErrFutureSplitDate) {
		t.Errorf("CheckSplitDate(2026-11-17, %s): got %v, want ErrFutureSplitDate", today, got)
	}
}

// 要件1〜3: 正しい値なら、銘柄コード・分割の日・分割後の株数を持つ分割を返す。境界の値（比率 2・1000、分割の日が今日）でも作れる。
func Test要件1_NewStockSplitは正しい値から分割を作る(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		code  string
		date  string
		ratio int64
	}{
		{"基本", "7203", "2026-03-31", 3},
		{"比率の下限2", "7203", "2026-03-31", 2},
		{"比率の上限1000", "7203", "2026-03-31", 1000},
		{"分割の日が今日", "NVDA", "2026-11-16", 10},
		{"銘柄コードに点を含む", "BRK.B", "2026-03-31", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := model.NewStockSplit(tc.code, tc.date, tc.ratio, today)
			if err != nil {
				t.Fatalf("NewStockSplit(%q, %q, %d, %q): got err %v, want nil", tc.code, tc.date, tc.ratio, today, err)
			}
			want := model.StockSplit{StockCode: tc.code, SplitDate: tc.date, Ratio: tc.ratio}
			if got != want {
				t.Errorf("NewStockSplit: got %+v, want %+v", got, want)
			}
		})
	}
}

// 要件2: 比率 1 は ErrSplitRatioTooSmall、1001 は ErrSplitRatioTooLarge、分割の日 2026-11-17（今日 2026-11-16）は ErrFutureSplitDate、
// 銘柄コード nvda（小文字）は銘柄コードの誤り（ErrInvalidStockCode）。誤りのとき、分割は零値。
func Test要件2_NewStockSplitは誤りの値をそれぞれのエラーにする(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		code  string
		date  string
		ratio int64
		want  error
	}{
		{"比率1", "7203", "2026-03-31", 1, model.ErrSplitRatioTooSmall},
		{"比率1001", "7203", "2026-03-31", 1001, model.ErrSplitRatioTooLarge},
		{"分割の日が明日", "7203", "2026-11-17", 3, model.ErrFutureSplitDate},
		{"銘柄コードが小文字", "nvda", "2026-03-31", 3, model.ErrInvalidStockCode},
		{"銘柄コードが空", "", "2026-03-31", 3, model.ErrInvalidStockCode},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := model.NewStockSplit(tc.code, tc.date, tc.ratio, today)
			if !errors.Is(err, tc.want) {
				t.Errorf("NewStockSplit(%q, %q, %d, %q): got err %v, want %v", tc.code, tc.date, tc.ratio, today, err, tc.want)
			}
			if got != (model.StockSplit{}) {
				t.Errorf("NewStockSplit: got %+v, want 零値（誤りのときは分割を返さない）", got)
			}
		})
	}
}

// 4.8: 確かめる順は、銘柄コード → 分割の日 → 比率（CSV の列の順）。最初に当たった誤りを返す。
// 比率 1・分割の日 2026-11-17・銘柄コード nvda を同時に与えると銘柄コードの誤り。比率 1・分割の日 2026-11-17 なら分割の日の誤り。
func Test要件2_NewStockSplitは銘柄コード_分割の日_比率の順に確かめ最初の誤りを返す(t *testing.T) {
	t.Parallel()

	_, err := model.NewStockSplit("nvda", "2026-11-17", 1, today)
	if !errors.Is(err, model.ErrInvalidStockCode) {
		t.Errorf("銘柄コード・分割の日・比率がすべて誤り: got err %v, want ErrInvalidStockCode（銘柄コードが先）", err)
	}

	_, err = model.NewStockSplit("7203", "2026-11-17", 1, today)
	if !errors.Is(err, model.ErrFutureSplitDate) {
		t.Errorf("分割の日と比率が誤り: got err %v, want ErrFutureSplitDate（分割の日が比率より先）", err)
	}
}
