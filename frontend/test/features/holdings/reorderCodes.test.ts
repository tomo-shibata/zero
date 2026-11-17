import { describe, expect, test } from 'vitest'
import { reorderCodes } from '../../../src/features/holdings/lib/reorderCodes'

// 仕様: カード表示の要件 docs/requirements/holdings-cards.md の要件17（別のカードの位置で離すと、離した位置に移し、間のカードは1つずつずれる）・
//   要件22・要件23（並びの外で離したとき、元と同じ位置に置いたときは並べ替えない）。基準23・基準24 の単体の部分。
// 境界: プラン docs/plans/holdings-cards/plan.md 4.6 PR②
//   reorderCodes(codes, activeCode, overCode): string[] | null
//   activeCode を overCode の位置に移した新しい並びを返す（間のカードは1つずつずれる）。
//   overCode が null（並びの外）か、activeCode と同じ（元の位置）なら null（並べ替えない）。

describe('reorderCodes（要件17・要件22・要件23）', () => {
  const THREE = ['6758', '7203', 'AAPL'] as const
  const FOUR = ['6758', '7203', '9984', 'AAPL'] as const

  test('カード表示の基準19: AAPL を 6758 の位置（先頭）に移すと AAPL、6758、7203 になる（後ろのカードは1つずつ後ろへずれる）', () => {
    expect(reorderCodes(THREE, 'AAPL', '6758')).toEqual(['AAPL', '6758', '7203'])
  })

  test('カード表示の補助: 前のカードを後ろの位置に移すと、間のカードは1つずつ前へずれる（6758 を AAPL の位置へ）', () => {
    expect(reorderCodes(THREE, '6758', 'AAPL')).toEqual(['7203', 'AAPL', '6758'])
  })

  test('カード表示の補助: 隣のカードの位置への移動（7203 を 6758 の位置へ、6758 を 7203 の位置へ）は、2枚の入れ替えになる', () => {
    expect(reorderCodes(THREE, '7203', '6758')).toEqual(['7203', '6758', 'AAPL'])
    expect(reorderCodes(THREE, '6758', '7203')).toEqual(['7203', '6758', 'AAPL'])
  })

  test('カード表示の補助: 真ん中への移動: 4枚の 9984 を 7203 の位置へ移すと 6758、9984、7203、AAPL', () => {
    expect(reorderCodes(FOUR, '9984', '7203')).toEqual(['6758', '9984', '7203', 'AAPL'])
  })

  test('カード表示の補助: 末尾への移動: 6758 を AAPL の位置へ移すと 7203、9984、AAPL、6758', () => {
    expect(reorderCodes(FOUR, '6758', 'AAPL')).toEqual(['7203', '9984', 'AAPL', '6758'])
  })

  test('カード表示の基準23: 離した先が並びの外（overCode が null）なら null（並べ替えない）', () => {
    expect(reorderCodes(THREE, 'AAPL', null)).toBeNull()
  })

  test('カード表示の基準24: 元の位置（activeCode と overCode が同じ）で離したら null（並べ替えない）', () => {
    expect(reorderCodes(THREE, '7203', '7203')).toBeNull()
  })

  test('カード表示の補助: 渡した配列は変えず、新しい配列を返す', () => {
    const codes = ['6758', '7203', 'AAPL']

    const result = reorderCodes(codes, 'AAPL', '6758')

    expect(codes).toEqual(['6758', '7203', 'AAPL'])
    expect(result).not.toBe(codes)
  })

  test('カード表示の補助: 1枚だけの並びでは、自分の位置で離すことしかできず null', () => {
    expect(reorderCodes(['7203'], '7203', '7203')).toBeNull()
    expect(reorderCodes(['7203'], '7203', null)).toBeNull()
  })
})
