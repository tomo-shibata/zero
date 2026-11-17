// 日付（"YYYY-MM-DD"）の計算。Date.UTC で通算の日数にするので、手元のタイムゾーンに左右されない（プラン 4.6）。

const MS_PER_DAY = 24 * 60 * 60 * 1000

/** "YYYY-MM-DD" を、1970-01-01 からの通算の日数にする。形は API の境界（zod の iso.date）で確かめ済み。 */
export function toDayNumber(date: string): number {
  const [year, month, day] = date.split('-').map(Number)
  return Date.UTC(year, month - 1, day) / MS_PER_DAY
}

/** 通算の日数（整数）を "YYYY-MM-DD" にする。 */
export function fromDayNumber(dayNumber: number): string {
  return new Date(dayNumber * MS_PER_DAY).toISOString().slice(0, 10)
}
