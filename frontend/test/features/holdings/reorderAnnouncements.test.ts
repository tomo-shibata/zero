import { describe, expect, test } from 'vitest'
import {
  CANCELLED_ANNOUNCEMENT,
  droppedAnnouncement,
  movedAnnouncement,
  pickedUpAnnouncement,
} from '../../../src/features/holdings/lib/reorderAnnouncements'

// 仕様: カード表示の要件 docs/requirements/holdings-cards.md の要件20（キーボードで並べ替えるとき、支援技術向けに読み上げさせる文）。
//   持ち上げたとき「<銘柄コード> <銘柄名>を持ち上げました。矢印キーで動かし、スペースキーで置きます。」、
//   動かしたとき「<n>番目に動かしました。」、置いたとき「<銘柄コード> <銘柄名>を<n>番目に置きました。」、
//   取り消したとき「並べ替えを取り消しました。」（n は先頭を1とした位置）。
// 境界: プラン docs/plans/holdings-cards/plan.md 4.6 PR②（features/holdings/lib/reorderAnnouncements.ts）。
// テスト計画: 5.3 PR② の「読み上げの4つの文の文字列」。

describe('読み上げの文（要件20）', () => {
  test('カード表示の基準21・22（要件20）: 持ち上げたとき: 「AAPL Appleを持ち上げました。矢印キーで動かし、スペースキーで置きます。」', () => {
    expect(pickedUpAnnouncement('AAPL', 'Apple')).toBe('AAPL Appleを持ち上げました。矢印キーで動かし、スペースキーで置きます。')
  })

  test('カード表示の基準21・22（要件20）: 持ち上げたとき: 銘柄コードと銘柄名はそのまま入る（7203 トヨタ自動車）', () => {
    expect(pickedUpAnnouncement('7203', 'トヨタ自動車')).toBe(
      '7203 トヨタ自動車を持ち上げました。矢印キーで動かし、スペースキーで置きます。',
    )
  })

  test('カード表示の基準21・22（要件20）: 動かしたとき: 位置は先頭を1とした番号で「<n>番目に動かしました。」', () => {
    expect(movedAnnouncement(1)).toBe('1番目に動かしました。')
    expect(movedAnnouncement(3)).toBe('3番目に動かしました。')
    expect(movedAnnouncement(12)).toBe('12番目に動かしました。')
  })

  test('カード表示の基準21・22（要件20）: 置いたとき: 「AAPL Appleを1番目に置きました。」（基準21 の文）', () => {
    expect(droppedAnnouncement('AAPL', 'Apple', 1)).toBe('AAPL Appleを1番目に置きました。')
  })

  test('カード表示の基準21・22（要件20）: 置いたとき: 銘柄コードと銘柄名と位置が入る（7203 トヨタ自動車を3番目に）', () => {
    expect(droppedAnnouncement('7203', 'トヨタ自動車', 3)).toBe('7203 トヨタ自動車を3番目に置きました。')
  })

  test('カード表示の基準21・22（要件20）: 取り消したとき: 「並べ替えを取り消しました。」（基準22 の文）', () => {
    expect(CANCELLED_ANNOUNCEMENT).toBe('並べ替えを取り消しました。')
  })
})
