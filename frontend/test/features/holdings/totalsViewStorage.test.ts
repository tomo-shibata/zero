import { describe, expect, test, vi } from 'vitest'
import {
  readTotalsView,
  TOTALS_VIEW_STORAGE_KEY,
  writeTotalsView,
} from '../../../src/features/holdings/lib/totalsViewStorage'

// 仕様: 要件 docs/requirements/fx-total.md の要件16・要件18、基準17。
// 境界: プラン docs/plans/fx-total/plan.md 4.5・4.7（PR②）
//   - キーは 'zero.holdings.totalsView'（TOTALS_VIEW_STORAGE_KEY）、値は 'byCurrency' か 'combinedYen'。
//   - readTotalsView(storage: Pick<Storage, 'getItem'> | null): TotalsView
//       storage が null・読むと例外・知らない値なら 'byCurrency'。例外を外に出さない。
//   - writeTotalsView(storage: Pick<Storage, 'setItem'> | null, view: TotalsView): void
//       storage が null・書くと例外なら何もしない。例外を外に出さない。
// テスト計画: プラン 5.1 の基準17 の行（totalsViewStorage.test.ts）。Storage は引数で受け取るので、差し替えた偽物を渡す
//   （window.localStorage には触れない）。

/** getItem が value を返す（null は「覚えていない」）だけの Storage。 */
function storageReturning(value: string | null): Pick<Storage, 'getItem'> {
  return { getItem: () => value }
}

describe('readTotalsView（覚えた表示を読む。要件16・要件18）', () => {
  test('基準17（要件16）: キーは zero.holdings.totalsView で、その値を読む', () => {
    const getItem = vi.fn((key: string) => (key === 'zero.holdings.totalsView' ? 'combinedYen' : null))

    expect(TOTALS_VIEW_STORAGE_KEY).toBe('zero.holdings.totalsView')
    expect(readTotalsView({ getItem })).toBe('combinedYen')
    expect(getItem).toHaveBeenCalledWith('zero.holdings.totalsView')
  })

  test('基準17（要件16）: 値が "combinedYen" なら combinedYen を返す', () => {
    expect(readTotalsView(storageReturning('combinedYen'))).toBe('combinedYen')
  })

  test('基準17（要件16）: 値が "byCurrency" なら byCurrency を返す', () => {
    expect(readTotalsView(storageReturning('byCurrency'))).toBe('byCurrency')
  })

  // 要件16: 何も覚えていないときは「通貨ごと」で始める。
  test('基準17（要件16）: 値がない（getItem が null）なら byCurrency を返す', () => {
    expect(readTotalsView(storageReturning(null))).toBe('byCurrency')
  })

  // 要件18: 覚えている値が「通貨ごと」「円にまとめる」のどちらでもないときは「通貨ごと」。
  test.each(['abc', '', 'combinedyen', 'COMBINEDYEN', ' combinedYen', 'bycurrency', 'null', 'undefined'])(
    '基準17（要件18）: 知らない値 %j は byCurrency を返す',
    (value) => {
      expect(readTotalsView(storageReturning(value))).toBe('byCurrency')
    },
  )

  // 要件18: 保存が禁止されているとき（storage が取れない、読むと例外）は、エラーを出さずに「通貨ごと」。
  test('基準17（要件18）: storage が null なら byCurrency を返す', () => {
    expect(readTotalsView(null)).toBe('byCurrency')
  })

  test('基準17（要件18）: getItem が例外を投げても、例外を外に出さず byCurrency を返す', () => {
    const throwing: Pick<Storage, 'getItem'> = {
      getItem: () => {
        throw new DOMException('The operation is insecure.', 'SecurityError')
      },
    }

    expect(() => readTotalsView(throwing)).not.toThrow()
    expect(readTotalsView(throwing)).toBe('byCurrency')
  })
})

describe('writeTotalsView（表示を覚えさせる。要件16・要件18）', () => {
  test.each(['byCurrency', 'combinedYen'] as const)(
    '基準17（要件16）: %s を zero.holdings.totalsView に書く',
    (view) => {
      const setItem = vi.fn()

      writeTotalsView({ setItem }, view)

      expect(setItem).toHaveBeenCalledTimes(1)
      expect(setItem).toHaveBeenCalledWith('zero.holdings.totalsView', view)
    },
  )

  // 要件18: 書くと例外（保存が禁止、容量超過など）でも、エラーを出さない。
  test('基準17（要件18）: setItem が例外を投げても、例外を外に出さない', () => {
    const throwing: Pick<Storage, 'setItem'> = {
      setItem: () => {
        throw new DOMException('The quota has been exceeded.', 'QuotaExceededError')
      },
    }

    expect(() => writeTotalsView(throwing, 'combinedYen')).not.toThrow()
  })

  test('基準17（要件18）: storage が null でも、例外を出さず何もしない', () => {
    expect(() => writeTotalsView(null, 'combinedYen')).not.toThrow()
  })
})
