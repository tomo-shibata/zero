package scheduler_test

import (
	"testing"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/tomo-shibata/zero/backend/internal/infrastructure/scheduler"
)

// 仕様: 要件（docs/requirements/stock-holdings.md）の AC-14a（FR-15「毎日18:00（日本時間）に自動で…取得し…保存する」）と、
// プラン（docs/plans/stock-holdings/plan.md）5.1 の AC-14a の行
// （cron.ParseStandard(FetchClosingPricesSpec) に UTC の時刻を渡す。2026-10-06T08:59Z の次が 2026-10-06T09:00Z、
// 2026-10-09T09:00Z（金）の次が 2026-10-10T09:00Z（土））。
// 確かめる名前は 4.4 の scheduler.FetchClosingPricesSpec（cron.ParseStandard で解釈する）と、決定事項 T-5（robfig/cron）。
//
// 日本時間 18:00 は UTC の 09:00（日本は夏時間がなく、いつも UTC+9）。
// 渡す時刻と期待する時刻を UTC で書くので、テストを動かす PC のタイムゾーンに左右されない。
// 期待する時刻はプランの値をそのまま使い、テストの中で計算しない。

// AC-14a（FR-15）: 価格の取得処理の実行スケジュール設定を読む → 毎日18:00（日本時間）に実行される設定になっている。
//   - 2026-10-06T08:59Z（日本時間 17:59）の次は、同じ日の 2026-10-06T09:00Z（日本時間 18:00）。
//     UTC の 18:00 や、日本時間の別の時刻に実行される設定なら、ここで食い違う。
//   - 2026-10-09T09:00Z（金曜の日本時間 18:00 ちょうど）の次は、翌日の 2026-10-10T09:00Z（土曜の日本時間 18:00）。
//     18時台に何度も実行される設定（分の指定の誤り）や、平日だけ実行される設定（「毎日」でない）なら、ここで食い違う。
func TestAC14a_価格の取得処理は毎日18時日本時間に実行される設定になっている(t *testing.T) {
	t.Parallel()

	schedule, err := cron.ParseStandard(scheduler.FetchClosingPricesSpec)
	if err != nil {
		t.Fatalf("cron.ParseStandard(%q): %v", scheduler.FetchClosingPricesSpec, err)
	}

	cases := []struct {
		name string
		from time.Time
		want time.Time
	}{
		{
			name: "2026-10-06T08:59Zの次は同じ日の2026-10-06T09:00Z",
			from: time.Date(2026, 10, 6, 8, 59, 0, 0, time.UTC),
			want: time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC),
		},
		{
			name: "2026-10-09T09:00Z_金の次は翌日の2026-10-10T09:00Z_土",
			from: time.Date(2026, 10, 9, 9, 0, 0, 0, time.UTC),
			want: time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := schedule.Next(tc.from)

			// Next は設定のタイムゾーン（日本時間）の時刻を返すことがあるので、== ではなく Equal で同じ瞬間かを比べる。
			if !got.Equal(tc.want) {
				t.Errorf("%s の次の実行時刻: got %s, want %s",
					tc.from.Format(time.RFC3339), got.UTC().Format(time.RFC3339), tc.want.Format(time.RFC3339))
			}
		})
	}
}
