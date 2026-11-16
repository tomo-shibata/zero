package csvinput

import "time"

// japanOffsetSeconds は、日本時間の UTC からの差（9時間）を秒で表した値。
const japanOffsetSeconds = 9 * 60 * 60

// japanTime は、日本時間（UTC+9）。「今日」「取り込んだ日」は日本時間の日付とする（要件定義の前提）。
// time.LoadLocation("Asia/Tokyo") を使わないのは、タイムゾーンのデータがない環境（最小の Docker イメージなど）でも動くようにするため。
// 日本に夏時間はないので、固定の差で正しい。
var japanTime = time.FixedZone("JST", japanOffsetSeconds)

// TodayInJapan は、now の時点の日本時間の日付を "YYYY-MM-DD" で返す。cmd の main が Run の Today に渡す。
func TodayInJapan(now time.Time) string {
	return now.In(japanTime).Format(time.DateOnly)
}
