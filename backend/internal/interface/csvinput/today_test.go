package csvinput_test

import (
	"testing"
	"time"

	"github.com/tomo-shibata/zero/backend/internal/interface/csvinput"
)

// 仕様: 要件（docs/requirements/trade-history.md）の前提「『今日』『取り込んだ日』は日本時間（Asia/Tokyo）の日付」・要件3・要件24 と、
// プラン（docs/plans/trade-history/plan.md）4.7 の TodayInJapan(now time.Time) string（日本時間の今日を "YYYY-MM-DD" で返す）、
// 5.5 の「TodayInJapan（単体）: 2026-11-16T14:59:59Z → "2026-11-16"、2026-11-16T15:00:00Z → "2026-11-17"」。
// 価格の CSV の登録のプラン（docs/plans/price-import/plan.md）4.3・5.2 で tradecsv から csvinput に移した（ケースの入力と期待値は変えていない）。

func Test要件24_TodayInJapanは日本時間の日付を返す(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		now  time.Time
		want string
	}{
		{"UTC の 14:59:59 は日本時間でまだ同じ日（23:59:59）", time.Date(2026, 11, 16, 14, 59, 59, 0, time.UTC), "2026-11-16"},
		{"UTC の 15:00:00 は日本時間で翌日の 0:00", time.Date(2026, 11, 16, 15, 0, 0, 0, time.UTC), "2026-11-17"},
		{"UTC の 0:00:00 は日本時間の 9:00", time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC), "2026-11-16"},
		{"年をまたぐ（UTC の 12-31 15:00 は日本時間の翌年 1-1 0:00）", time.Date(2026, 12, 31, 15, 0, 0, 0, time.UTC), "2027-01-01"},
		{"月をまたぐ（UTC の 11-30 15:00 は日本時間の 12-1 0:00）", time.Date(2026, 11, 30, 15, 0, 0, 0, time.UTC), "2026-12-01"},
		{"日本時間の時刻で渡しても同じ日", time.Date(2026, 11, 17, 0, 0, 0, 0, time.FixedZone("JST", 9*60*60)), "2026-11-17"},
		{"日本時間の 23:59:59", time.Date(2026, 11, 16, 23, 59, 59, 0, time.FixedZone("JST", 9*60*60)), "2026-11-16"},
		{"UTC-8 の 23:59:59（UTC では翌日の 7:59、日本時間では翌日の 16:59）", time.Date(2026, 11, 16, 23, 59, 59, 0, time.FixedZone("PST", -8*60*60)), "2026-11-17"},
		{"うるう日", time.Date(2028, 2, 28, 15, 0, 0, 0, time.UTC), "2028-02-29"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := csvinput.TodayInJapan(tc.now); got != tc.want {
				t.Errorf("TodayInJapan(%s): got %q, want %q", tc.now.Format(time.RFC3339), got, tc.want)
			}
		})
	}
}
