import { describe, expect, test } from 'vitest'
import type { PriceRefreshResult } from '../../../src/entities/holding/model'
import { refreshResultMessages } from '../../../src/features/holdings/lib/refreshResultMessages'

// 仕様: 要件 docs/requirements/price-update.md の要件18、基準28（基準25 の文言も同じ決まり）。
// 境界: プラン docs/plans/price-update/plan.md 4.4「フロント（TypeScript）」
//   - entities/holding/model.ts の PriceRefreshResult = { saved: string[]; failed: string[] }
//   - features/holdings/lib/refreshResultMessages.ts の refreshResultMessages(result): string[]
//     要件18 の文言を表示する順に返す。failed は昇順に並べ直す。
// 要件18: 保存できた銘柄が1件以上なら「n銘柄の価格を更新しました」（n は保存できた銘柄の数）、
//   取得・保存できなかった銘柄が1件以上なら「価格を取得できなかった銘柄: 1301, 9984」（銘柄コードを昇順に「, 」でつなぐ）、
//   どちらも0件なら「更新する銘柄はありません」。
// テスト計画: プラン 5.1 の基準28 の行（(b) は失敗の銘柄を 9984・1301 の順で渡し、並べ直しを確かめる）。
// 純粋関数なので、mock は使わない。

describe('基準28（要件18）: 更新の結果の文言', () => {
  test('基準28 (a): 保存2件・失敗0件なら「2銘柄の価格を更新しました」だけ', () => {
    const result: PriceRefreshResult = { saved: ['6758', '7203'], failed: [] }

    expect(refreshResultMessages(result)).toEqual(['2銘柄の価格を更新しました'])
  })

  test('基準28 (b): 保存0件・失敗2件（9984、1301 の順で渡す）なら、昇順に並べ直した「価格を取得できなかった銘柄: 1301, 9984」だけ', () => {
    const result: PriceRefreshResult = { saved: [], failed: ['9984', '1301'] }

    expect(refreshResultMessages(result)).toEqual(['価格を取得できなかった銘柄: 1301, 9984'])
  })

  test('基準28 (c): 保存0件・失敗0件なら「更新する銘柄はありません」だけ', () => {
    const result: PriceRefreshResult = { saved: [], failed: [] }

    expect(refreshResultMessages(result)).toEqual(['更新する銘柄はありません'])
  })
})

describe('補助 要件18: 保存と失敗が両方あるときと、件数の数え方', () => {
  test('補助 要件18（基準25 の文言）: 保存1件・失敗1件なら、保存の文言、失敗の文言の順に2つ返す', () => {
    const result: PriceRefreshResult = { saved: ['7203'], failed: ['9984'] }

    expect(refreshResultMessages(result)).toEqual([
      '1銘柄の価格を更新しました',
      '価格を取得できなかった銘柄: 9984',
    ])
  })

  test('補助 要件18: 保存3件・失敗2件（失敗は昇順でない順で渡す）なら、n は保存できた銘柄の数（3）で、失敗は昇順につなぐ', () => {
    const result: PriceRefreshResult = {
      saved: ['6758', '7203', '8306'],
      failed: ['9984', '1301'],
    }

    expect(refreshResultMessages(result)).toEqual([
      '3銘柄の価格を更新しました',
      '価格を取得できなかった銘柄: 1301, 9984',
    ])
  })

  test('補助 要件18: 失敗が1件だけなら、区切りの「, 」を付けない', () => {
    const result: PriceRefreshResult = { saved: [], failed: ['1301'] }

    expect(refreshResultMessages(result)).toEqual(['価格を取得できなかった銘柄: 1301'])
  })

  test('補助 要件18: 渡した結果の failed の配列は、並べ直しで書き換えない', () => {
    const failed = ['9984', '1301']
    const result: PriceRefreshResult = { saved: [], failed }

    refreshResultMessages(result)

    expect(failed).toEqual(['9984', '1301'])
  })
})
