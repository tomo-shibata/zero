/**
 * activeCode のカードを overCode のカードの位置に移した、新しい並び（銘柄コード）を返す（要件17）。
 * 間のカードは1つずつずれる（前から後ろへ移すと間のカードは1つ前へ、後ろから前へ移すと1つ後ろへ）。
 * 並べ替えない場合は null を返す: overCode が null（カードの並びの外で離した。要件22）、
 * activeCode と同じ（元の位置で離した。要件23）、または codes にない銘柄コードのとき。
 * codes は変えない。
 */
export function reorderCodes(codes: readonly string[], activeCode: string, overCode: string | null): string[] | null {
  if (overCode === null || overCode === activeCode) return null
  const from = codes.indexOf(activeCode)
  const to = codes.indexOf(overCode)
  if (from < 0 || to < 0) return null
  const next = [...codes]
  next.splice(from, 1)
  next.splice(to, 0, activeCode)
  return next
}
