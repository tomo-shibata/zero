// @vitest-environment jsdom
import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { useTotalsView } from '../../../src/features/holdings/hooks/useTotalsView'

// 仕様: 要件 docs/requirements/fx-total.md の要件5・要件16・要件18、基準17。
// 境界: プラン docs/plans/fx-total/plan.md 4.5・4.7（PR②）
//   useTotalsView(): { view: TotalsView; setView: (view: TotalsView) => void }
//   - window.localStorage（キー 'zero.holdings.totalsView'、値 'byCurrency' か 'combinedYen'）で覚えた表示を読み書きする。
//   - 選び直したら state と localStorage の両方を変える。サーバーには何も送らない（要件5・要件16）。
//   - window.localStorage を取り出すだけで例外になる環境でも、例外を外に出さない（要件18）。
// テスト計画: プラン 5.2 の useTotalsView.test.tsx の行（localStorage の取り出しそのものが例外を投げるとき）。5章冒頭の後始末
//   （afterEach で localStorage.clear()・vi.restoreAllMocks()。getter を差し替えたら元の記述子に戻す）。
// mock は境界の外側（ブラウザーの Storage と、サーバーへの要求を数えるためのグローバルの fetch）だけに使う。

const STORAGE_KEY = 'zero.holdings.totalsView'

/** window.localStorage を差し替える前の状態に戻すための記述子（window 自身が持つ場合だけ。持たなければ、差し替えを消せば元に戻る）。 */
let originalDescriptor: PropertyDescriptor | undefined
let replaced = false

/** window.localStorage の取り出しそのものが例外を投げるようにする（ブラウザーによっては保存を禁止すると取り出すだけで例外になる）。 */
function makeLocalStorageGetterThrow(): void {
  originalDescriptor = Object.getOwnPropertyDescriptor(window, 'localStorage')
  replaced = true
  Object.defineProperty(window, 'localStorage', {
    configurable: true,
    get() {
      throw new DOMException('The operation is insecure.', 'SecurityError')
    },
  })
}

afterEach(() => {
  cleanup()
  if (replaced) {
    if (originalDescriptor !== undefined) {
      Object.defineProperty(window, 'localStorage', originalDescriptor)
    } else {
      // window 自身には持っていなかった（prototype 側にある）ので、差し替えを消せば元に戻る。
      Reflect.deleteProperty(window, 'localStorage')
    }
    replaced = false
    originalDescriptor = undefined
  }
  window.localStorage.clear()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('useTotalsView（覚えた表示。要件16）', () => {
  // 要件16: 何も覚えていないときは「通貨ごと」で始める。
  test('基準17（要件16）: 何も覚えていないとき、view は byCurrency で始まる', () => {
    const { result } = renderHook(() => useTotalsView())

    expect(result.current.view).toBe('byCurrency')
  })

  // 要件16: 再読み込みや次に開いたときに同じ表示で始める。
  test('基準15 の部品（要件16）: localStorage に combinedYen を入れてあるとき、view は combinedYen で始まる', () => {
    window.localStorage.setItem(STORAGE_KEY, 'combinedYen')

    const { result } = renderHook(() => useTotalsView())

    expect(result.current.view).toBe('combinedYen')
  })

  // 要件16: 選び直したら state と localStorage の両方を変える。
  test('基準15 の部品（要件16）: setView("combinedYen") で view が combinedYen になり、localStorage にも combinedYen が入る', () => {
    const { result } = renderHook(() => useTotalsView())

    act(() => {
      result.current.setView('combinedYen')
    })

    expect(result.current.view).toBe('combinedYen')
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe('combinedYen')
  })

  test('基準15 の部品（要件16）: combinedYen から setView("byCurrency") で、view が byCurrency になり、localStorage にも byCurrency が入る', () => {
    window.localStorage.setItem(STORAGE_KEY, 'combinedYen')
    const { result } = renderHook(() => useTotalsView())

    act(() => {
      result.current.setView('byCurrency')
    })

    expect(result.current.view).toBe('byCurrency')
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe('byCurrency')
  })

  // 要件5・要件16: サーバーには何も送らない。
  test('要件5・要件16: 読み込みと setView で、サーバーへの要求（fetch）は送らない', () => {
    const fetchMock = vi.fn<typeof fetch>()
    vi.stubGlobal('fetch', fetchMock)

    const { result } = renderHook(() => useTotalsView())
    act(() => {
      result.current.setView('combinedYen')
    })

    expect(fetchMock).not.toHaveBeenCalled()
  })

  // 要件18: 覚えている値がどちらでもないときは「通貨ごと」で始め、「円にまとめる」を選べる。
  test('基準17（要件18）: localStorage の値が "abc" のとき、view は byCurrency で始まり、setView で combinedYen になる', () => {
    window.localStorage.setItem(STORAGE_KEY, 'abc')
    const { result } = renderHook(() => useTotalsView())
    expect(result.current.view).toBe('byCurrency')

    act(() => {
      result.current.setView('combinedYen')
    })

    expect(result.current.view).toBe('combinedYen')
  })
})

describe('useTotalsView（保存が禁止されているとき。要件18）', () => {
  // プラン 5.2: window.localStorage の取り出しそのものが例外を投げるときも、描画が失敗せず 'byCurrency' で始まり、setView で表示が変わる。
  test('補助（要件18。プラン 5.2）: window.localStorage の取り出しが例外を投げても、描画は失敗せず view は byCurrency で始まり、setView で combinedYen に変わる', () => {
    makeLocalStorageGetterThrow()
    // Given の確認: 取り出すだけで例外になる。
    expect(() => window.localStorage).toThrow()

    const { result } = renderHook(() => useTotalsView())
    expect(result.current.view).toBe('byCurrency')

    expect(() => {
      act(() => {
        result.current.setView('combinedYen')
      })
    }).not.toThrow()

    expect(result.current.view).toBe('combinedYen')
  })

  // 基準17: ブラウザーへの保存が例外を投げる（getItem・setItem が例外）とき。
  test('基準17（要件18）: Storage の getItem・setItem が例外を投げても、view は byCurrency で始まり、setView で combinedYen に変わる', () => {
    // 差し替えの前に 'combinedYen' を保存しておく。getItem の例外を受け止めたなら 'byCurrency' で始まり、
    // 差し替えが効かずに getItem が普通に値を返すなら 'combinedYen' で始まる（Given を作れたことの確かめ）。
    window.localStorage.setItem(STORAGE_KEY, 'combinedYen')
    const getItemSpy = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new DOMException('The operation is insecure.', 'SecurityError')
    })
    const setItemSpy = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('The quota has been exceeded.', 'QuotaExceededError')
    })

    const { result } = renderHook(() => useTotalsView())
    expect(result.current.view).toBe('byCurrency')
    expect(getItemSpy).toHaveBeenCalled()
    expect(getItemSpy.mock.results.some((r) => r.type === 'throw')).toBe(true)

    expect(() => {
      act(() => {
        result.current.setView('combinedYen')
      })
    }).not.toThrow()

    expect(setItemSpy).toHaveBeenCalled()
    expect(setItemSpy.mock.results.some((r) => r.type === 'throw')).toBe(true)
    expect(result.current.view).toBe('combinedYen')
  })
})
