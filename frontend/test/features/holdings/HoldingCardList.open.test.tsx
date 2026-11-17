// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import type { Holding } from '../../../src/entities/holding/model'
import { HoldingCardList } from '../../../src/features/holdings/components/HoldingCardList'

// 仕様: 要件 docs/requirements/stock-history.md の要件1（カードを押すと画面を移す。押すとは、マウスでドラッグせずに押して離すこと、
//   タッチで長押しせずに（0.25秒未満で）指を動かさずに離すこと）・要件2（Enter キー。スペースで持ち上げている間は移らない）・
//   要件3（「⋯」・ドラッグ・0.25秒以上の長押しでは移らない）、基準3（単体）。
// 境界: プラン docs/plans/stock-history/plan.md 4.5「カードを開く操作」と 4.8「PR② 画面とカードからの移動」
//   - <HoldingCardList holdings onDelete onMove onOpen />。onOpen: (code: string) => void（必須）。画面を移す（useNavigate）のは HoldingsPage で、ここでは呼ばれるかだけを確かめる。
//   - マウス: 待っている状態のまま、同じカードの上で左ボタンを離した（5px 未満しか動いていない）とき onOpen(code)。
//   - タッチ: 待っている状態のまま（0.25秒未満）、指を動かさずに離したとき onOpen(code)。0.25秒たってドラッグが始まった後に離しても呼ばない。
//     onOpen を呼ぶときは、その touchend で preventDefault() する（ブラウザがタップの後に送る互換の mousedown・mouseup でもう一度 onOpen が呼ばれないように）。
//   - キーボード: 何もしていないとき（idle）に、カード自身に Enter が来たとき。持ち上げている間の Enter は無視する。
//   - 「⋯」の中（data-no-drag）で起きたことでは呼ばない。
//   - 互換のマウスのイベントは jsdom が作らないので送らない（E2E の基準2 で確かめる）。タイマーは偽のタイマー。
// テスト計画: プラン 5.1 の基準3 の HoldingCardList.open.test.tsx の行と、5.2 の HoldingCardList.open.test.tsx の行。

const SONY: Holding = {
  code: '6758',
  name: 'ソニーグループ',
  currency: 'JPY',
  quantity: 10,
  acquisitionPrice: '4000.0',
  currentPrice: '3500.0',
  priceDate: '2026-10-06',
  valuation: '35000',
  unrealizedGain: '-5000',
  unrealizedGainRate: '-12.5',
}
const TOYOTA: Holding = {
  code: '7203',
  name: 'トヨタ自動車',
  currency: 'JPY',
  quantity: 100,
  acquisitionPrice: '2500.0',
  currentPrice: '3000.0',
  priceDate: '2026-10-06',
  valuation: '300000',
  unrealizedGain: '50000',
  unrealizedGainRate: '20.0',
}
/** 基準3 の「並びが 6758、7203 のカード」。 */
const TWO: readonly Holding[] = [SONY, TOYOTA]

const SPACE = { key: ' ', code: 'Space' }
const ENTER = { key: 'Enter', code: 'Enter' }
const ESCAPE = { key: 'Escape', code: 'Escape' }
const LIFTED = '持ち上げました'

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

function renderList(onOpen: (code: string) => void) {
  return render(<HoldingCardList holdings={TWO} onDelete={() => {}} onMove={() => {}} onOpen={onOpen} />)
}

/** カード holding の <li>（名前「<銘柄コード> <銘柄名>」）。 */
function card(holding: Holding): HTMLElement {
  return screen.getByLabelText(`${holding.code} ${holding.name}`, { selector: 'li' })
}

/** カード holding の本体（銘柄名。「⋯」を包む要素の外）。 */
function bodyOf(holding: Holding): HTMLElement {
  return within(card(holding)).getByText(holding.name, { selector: 'p' })
}

/** 読み上げ用のライブリージョン（role="status"）の文。 */
function announcement(): string {
  return screen.getByRole('status').textContent ?? ''
}

/** 時間を進める（act の中で、タイマーを進める）。 */
function advance(ms: number): void {
  act(() => {
    vi.advanceTimersByTime(ms)
  })
}

type TouchPoint = { identifier: number; clientX: number; clientY: number }

/**
 * target にタッチのイベントを送る（jsdom の TouchEvent は touches を受け取らないので、イベントに足す）。
 * 戻り値は dispatchEvent の結果（preventDefault されたら false）。
 */
function sendTouch(
  target: Node | Window,
  type: 'touchstart' | 'touchmove' | 'touchend' | 'touchcancel',
  touches: readonly TouchPoint[],
  changedTouches: readonly TouchPoint[] = touches,
): boolean {
  const event = new Event(type, { bubbles: true, cancelable: true })
  Object.defineProperties(event, {
    touches: { value: touches },
    changedTouches: { value: changedTouches },
    targetTouches: { value: touches },
  })
  return fireEvent(target, event)
}

function finger(identifier: number, x: number, y: number): TouchPoint {
  return { identifier, clientX: x, clientY: y }
}

describe('キーボード（基準3。要件2）', () => {
  test('基準3: 並びが 6758、7203 のとき、7203 のカードにフォーカスを当てて Enter を押すと onOpen("7203") が1回呼ばれる', () => {
    const onOpen = vi.fn()
    renderList(onOpen)
    const toyota = card(TOYOTA)
    toyota.focus()

    fireEvent.keyDown(toyota, ENTER)

    expect(onOpen).toHaveBeenCalledTimes(1)
    expect(onOpen).toHaveBeenCalledWith('7203')
  })

  test('基準3: 6758 のカードで Enter を押せば onOpen("6758")（押したカードの銘柄コードで呼ぶ）', () => {
    const onOpen = vi.fn()
    renderList(onOpen)
    const sony = card(SONY)
    sony.focus()

    fireEvent.keyDown(sony, ENTER)

    expect(onOpen).toHaveBeenCalledTimes(1)
    expect(onOpen).toHaveBeenCalledWith('6758')
  })

  test('基準3: 7203 のカードでスペースを押して持ち上げてから Enter を押しても、onOpen は呼ばれない（持ち上げたまま）', () => {
    const onOpen = vi.fn()
    renderList(onOpen)
    const toyota = card(TOYOTA)
    toyota.focus()

    fireEvent.keyDown(toyota, SPACE)
    expect(announcement()).toContain(LIFTED)
    fireEvent.keyDown(toyota, ENTER)

    expect(onOpen).not.toHaveBeenCalled()
    expect(announcement()).toContain(LIFTED)
  })

  test('補助 要件3: 「⋯」のボタンにフォーカスして Enter を押しても、onOpen は呼ばれない（Enter は「⋯」のボタンの操作）', () => {
    const onOpen = vi.fn()
    renderList(onOpen)
    const menuButton = screen.getByRole('button', { name: 'トヨタ自動車の操作' })
    menuButton.focus()

    fireEvent.keyDown(menuButton, ENTER)

    expect(onOpen).not.toHaveBeenCalled()
  })

  test('補助 要件2: スペースで持ち上げて Esc で取り消した後（何もしていない状態に戻った後）の Enter では、onOpen が呼ばれる', () => {
    const onOpen = vi.fn()
    renderList(onOpen)
    const toyota = card(TOYOTA)
    toyota.focus()

    fireEvent.keyDown(toyota, SPACE)
    fireEvent.keyDown(toyota, ESCAPE)
    fireEvent.keyDown(toyota, ENTER)

    expect(onOpen).toHaveBeenCalledTimes(1)
    expect(onOpen).toHaveBeenCalledWith('7203')
  })
})

describe('マウス（要件1・要件3）', () => {
  test('補助 要件1: 7203 のカードで左ボタンを押し、5px 未満（2px）動いて同じカードの上で離すと、onOpen("7203") が1回呼ばれる', () => {
    const onOpen = vi.fn()
    renderList(onOpen)

    fireEvent.mouseDown(bodyOf(TOYOTA), { button: 0, buttons: 1, clientX: 100, clientY: 100 })
    fireEvent.mouseMove(document, { buttons: 1, clientX: 102, clientY: 100 })
    fireEvent.mouseUp(bodyOf(TOYOTA), { button: 0, buttons: 0, clientX: 102, clientY: 100 })

    expect(onOpen).toHaveBeenCalledTimes(1)
    expect(onOpen).toHaveBeenCalledWith('7203')
  })

  test('補助 要件1: 動かさずに押して離しても onOpen("7203")（カードの <li> そのものを押した場合）', () => {
    const onOpen = vi.fn()
    renderList(onOpen)

    fireEvent.mouseDown(card(TOYOTA), { button: 0, clientX: 100, clientY: 100 })
    fireEvent.mouseUp(card(TOYOTA), { button: 0, clientX: 100, clientY: 100 })

    expect(onOpen).toHaveBeenCalledTimes(1)
    expect(onOpen).toHaveBeenCalledWith('7203')
  })

  test('補助 要件3: 5px 以上（12px）動かしてから離すと（ドラッグ）、onOpen は呼ばれない', () => {
    const onOpen = vi.fn()
    renderList(onOpen)

    fireEvent.mouseDown(bodyOf(TOYOTA), { button: 0, buttons: 1, clientX: 100, clientY: 100 })
    fireEvent.mouseMove(document, { buttons: 1, clientX: 112, clientY: 112 })
    expect(announcement()).toContain(LIFTED)
    fireEvent.mouseUp(bodyOf(TOYOTA), { button: 0, buttons: 0, clientX: 112, clientY: 112 })

    expect(onOpen).not.toHaveBeenCalled()
  })

  test('補助 要件3: 5px 以上動かして、押した位置に戻って離しても（ドラッグを始めた後）、onOpen は呼ばれない', () => {
    const onOpen = vi.fn()
    renderList(onOpen)

    fireEvent.mouseDown(bodyOf(TOYOTA), { button: 0, buttons: 1, clientX: 100, clientY: 100 })
    fireEvent.mouseMove(document, { buttons: 1, clientX: 112, clientY: 112 })
    fireEvent.mouseMove(document, { buttons: 1, clientX: 100, clientY: 100 })
    fireEvent.mouseUp(bodyOf(TOYOTA), { button: 0, buttons: 0, clientX: 100, clientY: 100 })

    expect(onOpen).not.toHaveBeenCalled()
  })

  test('補助 要件1（同じカードの上で離す）: 7203 のカードで押して、別のカード（6758）の上で離しても、onOpen は呼ばれない', () => {
    const onOpen = vi.fn()
    renderList(onOpen)

    fireEvent.mouseDown(bodyOf(TOYOTA), { button: 0, clientX: 100, clientY: 100 })
    fireEvent.mouseUp(bodyOf(SONY), { button: 0, clientX: 100, clientY: 100 })

    expect(onOpen).not.toHaveBeenCalled()
  })

  test('補助 要件1（左ボタン）: 右ボタン（button: 2）で押して離しても、onOpen は呼ばれない', () => {
    const onOpen = vi.fn()
    renderList(onOpen)

    fireEvent.mouseDown(bodyOf(TOYOTA), { button: 2, buttons: 2, clientX: 100, clientY: 100 })
    fireEvent.mouseUp(bodyOf(TOYOTA), { button: 2, clientX: 100, clientY: 100 })

    expect(onOpen).not.toHaveBeenCalled()
  })

  test('補助 要件3: 「⋯」の上で押して離しても（click も）、onOpen は呼ばれない', () => {
    const onOpen = vi.fn()
    renderList(onOpen)
    const menuButton = screen.getByRole('button', { name: 'トヨタ自動車の操作' })

    fireEvent.mouseDown(menuButton, { button: 0, buttons: 1, clientX: 100, clientY: 100 })
    fireEvent.mouseUp(menuButton, { button: 0, buttons: 0, clientX: 100, clientY: 100 })
    fireEvent.click(menuButton)

    expect(onOpen).not.toHaveBeenCalled()
  })

  test('補助 要件3（エッジケースの網羅）: 7203 のカードの本体で押して、同じカードの「⋯」の上で離しても、onOpen は呼ばれない', () => {
    const onOpen = vi.fn()
    renderList(onOpen)
    const menuButton = screen.getByRole('button', { name: 'トヨタ自動車の操作' })

    fireEvent.mouseDown(bodyOf(TOYOTA), { button: 0, buttons: 1, clientX: 100, clientY: 100 })
    fireEvent.mouseUp(menuButton, { button: 0, buttons: 0, clientX: 100, clientY: 100 })

    expect(onOpen).not.toHaveBeenCalled()
  })

  test('補助 要件1: 押して離したあと、続けて別のカードを押して離すと、それぞれ onOpen（7203、6758 の順）。1回の操作で2回呼ばれない', () => {
    const onOpen = vi.fn()
    renderList(onOpen)

    fireEvent.mouseDown(bodyOf(TOYOTA), { button: 0, clientX: 100, clientY: 100 })
    fireEvent.mouseUp(bodyOf(TOYOTA), { button: 0, clientX: 100, clientY: 100 })
    fireEvent.mouseDown(bodyOf(SONY), { button: 0, clientX: 100, clientY: 300 })
    fireEvent.mouseUp(bodyOf(SONY), { button: 0, clientX: 100, clientY: 300 })

    expect(onOpen.mock.calls).toEqual([['7203'], ['6758']])
  })
})

describe('click（支援技術・キーボードが作る click。要件1・要件2。ステップ6 1ラウンド目のデザイン2）', () => {
  // event.detail が 0 の click は、マウスを押したものではなく、支援技術（NVDA の閲覧モードの Enter・VoiceOver のダブルタップなど）が作るもの。
  // 何もしていないとき（idle）だけ、開く操作として受ける。マウスの click（detail が 1 以上）は、ドラッグの後にも届くので使わない（プラン 4.5）。
  test('デザイン2: 7203 のカードに detail 0 の click を送ると、onOpen("7203") が1回呼ばれる', () => {
    const onOpen = vi.fn()
    renderList(onOpen)

    fireEvent.click(card(TOYOTA), { detail: 0 })

    expect(onOpen).toHaveBeenCalledTimes(1)
    expect(onOpen).toHaveBeenCalledWith('7203')
  })

  test('デザイン2: カードの本体（銘柄名の文字）に送った detail 0 の click でも、押したカードの銘柄コードで呼ばれる（6758 なら onOpen("6758")）', () => {
    const onOpen = vi.fn()
    renderList(onOpen)

    fireEvent.click(bodyOf(SONY), { detail: 0 })

    expect(onOpen).toHaveBeenCalledTimes(1)
    expect(onOpen).toHaveBeenCalledWith('6758')
  })

  test('デザイン2: detail が 1 の click（マウスの click。ドラッグの後にも届く）では、onOpen は呼ばれない', () => {
    const onOpen = vi.fn()
    renderList(onOpen)

    fireEvent.click(card(TOYOTA), { detail: 1 })
    fireEvent.click(bodyOf(TOYOTA), { detail: 2 })

    expect(onOpen).not.toHaveBeenCalled()
  })

  test('デザイン2: スペースで持ち上げている間の detail 0 の click では、onOpen は呼ばれない（持ち上げたまま）', () => {
    const onOpen = vi.fn()
    renderList(onOpen)
    const toyota = card(TOYOTA)
    toyota.focus()

    fireEvent.keyDown(toyota, SPACE)
    expect(announcement()).toContain(LIFTED)
    fireEvent.click(toyota, { detail: 0 })

    expect(onOpen).not.toHaveBeenCalled()
    expect(announcement()).toContain(LIFTED)
  })

  test('デザイン2: 「⋯」のボタンに送った detail 0 の click では、onOpen は呼ばれない（「⋯」の操作）', () => {
    const onOpen = vi.fn()
    renderList(onOpen)
    const menuButton = screen.getByRole('button', { name: 'トヨタ自動車の操作' })

    fireEvent.click(menuButton, { detail: 0 })

    expect(onOpen).not.toHaveBeenCalled()
  })

  test('デザイン2: マウスで押して離した（onOpen が1回）あとに、続けて届く detail 1 の click では、2回目の onOpen は呼ばれない', () => {
    const onOpen = vi.fn()
    renderList(onOpen)

    fireEvent.mouseDown(bodyOf(TOYOTA), { button: 0, buttons: 1, clientX: 100, clientY: 100 })
    fireEvent.mouseUp(bodyOf(TOYOTA), { button: 0, buttons: 0, clientX: 100, clientY: 100 })
    expect(onOpen).toHaveBeenCalledTimes(1)
    fireEvent.click(bodyOf(TOYOTA), { detail: 1, button: 0, clientX: 100, clientY: 100 })

    expect(onOpen).toHaveBeenCalledTimes(1)
    expect(onOpen).toHaveBeenCalledWith('7203')
  })

  // 支援技術（Chromium の押す操作）は、作り物の mousedown → mouseup → click（detail 0）を続けて送る。mouseup で開いたら、続く click では開かない
  // （開くと、履歴に同じ画面が2つ積まれる。ステップ6 2ラウンド目のエッジケース1-1）。
  test('エッジケース1-1: 7203 のカードの本体に mouseDown（左ボタン）→ mouseUp → click（detail 0）を続けて送ると、onOpen("7203") は1回だけ呼ばれる（支援技術の押す操作）', () => {
    const onOpen = vi.fn()
    renderList(onOpen)

    fireEvent.mouseDown(bodyOf(TOYOTA), { button: 0, buttons: 1, clientX: 100, clientY: 100 })
    fireEvent.mouseUp(bodyOf(TOYOTA), { button: 0, buttons: 0, clientX: 100, clientY: 100 })
    fireEvent.click(bodyOf(TOYOTA), { detail: 0, button: 0, clientX: 100, clientY: 100 })

    expect(onOpen).toHaveBeenCalledTimes(1)
    expect(onOpen).toHaveBeenCalledWith('7203')
  })

  test('デザイン2: ドラッグ（12px 動かして離す）の後に続けて届く detail 1 の click でも、onOpen は呼ばれない', () => {
    const onOpen = vi.fn()
    renderList(onOpen)

    fireEvent.mouseDown(bodyOf(TOYOTA), { button: 0, buttons: 1, clientX: 100, clientY: 100 })
    fireEvent.mouseMove(document, { buttons: 1, clientX: 112, clientY: 112 })
    fireEvent.mouseUp(bodyOf(TOYOTA), { button: 0, buttons: 0, clientX: 112, clientY: 112 })
    fireEvent.click(bodyOf(TOYOTA), { detail: 1, button: 0, clientX: 112, clientY: 112 })

    expect(onOpen).not.toHaveBeenCalled()
  })
})

describe('タッチ（要件1・要件3）', () => {
  test('補助 要件1: 7203 のカードに指を置き、0.25秒未満（100ms）で、動かさずに離すと、onOpen("7203") が1回呼ばれる', () => {
    const onOpen = vi.fn()
    renderList(onOpen)

    sendTouch(bodyOf(TOYOTA), 'touchstart', [finger(0, 100, 100)])
    advance(100)
    sendTouch(window, 'touchend', [], [finger(0, 100, 100)])

    expect(onOpen).toHaveBeenCalledTimes(1)
    expect(onOpen).toHaveBeenCalledWith('7203')
  })

  test('補助 要件1（5px 以内の揺れ）: 指が 3px 動いただけで（0.25秒未満に）離しても、onOpen("7203")', () => {
    const onOpen = vi.fn()
    renderList(onOpen)

    sendTouch(bodyOf(TOYOTA), 'touchstart', [finger(0, 100, 100)])
    sendTouch(window, 'touchmove', [finger(0, 102, 103)])
    sendTouch(window, 'touchend', [], [finger(0, 102, 103)])

    expect(onOpen).toHaveBeenCalledTimes(1)
    expect(onOpen).toHaveBeenCalledWith('7203')
  })

  test('補助 4.5（二重に開かない）: onOpen を呼ぶときの touchend は preventDefault される（fireEvent の戻り値が false）。ブラウザがタップの後に送る互換の mousedown・mouseup で、もう一度 onOpen が呼ばれないため', () => {
    const onOpen = vi.fn()
    renderList(onOpen)

    sendTouch(bodyOf(TOYOTA), 'touchstart', [finger(0, 100, 100)])
    const notPrevented = sendTouch(window, 'touchend', [], [finger(0, 100, 100)])

    expect(onOpen).toHaveBeenCalledTimes(1)
    expect(notPrevented, 'touchend の既定の動作が止められている').toBe(false)
  })

  test('補助 要件3: 指を動かして（6px。スクロール）、0.25秒未満で離すと、onOpen は呼ばれない', () => {
    const onOpen = vi.fn()
    renderList(onOpen)

    sendTouch(bodyOf(TOYOTA), 'touchstart', [finger(0, 100, 100)])
    advance(100)
    sendTouch(window, 'touchmove', [finger(0, 100, 106)])
    sendTouch(window, 'touchend', [], [finger(0, 100, 106)])

    expect(onOpen).not.toHaveBeenCalled()
  })

  test('補助 要件3: 0.25秒以上（300ms）押し続けてドラッグが始まった後に、指を動かさずに離しても、onOpen は呼ばれない', () => {
    const onOpen = vi.fn()
    renderList(onOpen)

    sendTouch(bodyOf(TOYOTA), 'touchstart', [finger(0, 100, 100)])
    advance(300)
    expect(announcement()).toContain(LIFTED)
    const notPrevented = sendTouch(window, 'touchend', [], [finger(0, 100, 100)])

    expect(onOpen).not.toHaveBeenCalled()
    // 置くときの touchend は止められる（止めないと、ブラウザがタップの後に互換の mousedown・mouseup を送り、マウスの側でカードが開いてしまう）。
    expect(notPrevented, 'touchend の既定の動作が止められている').toBe(false)
  })

  test('補助 要件3（エッジケースの網羅）: 指を置いて、2本目の指が触れた後に、1本目の指を離しても、onOpen は呼ばれない（スクロールやピンチに任せる）', () => {
    const onOpen = vi.fn()
    renderList(onOpen)

    sendTouch(bodyOf(TOYOTA), 'touchstart', [finger(0, 100, 100)])
    advance(100)
    // 2本目の指（identifier 1）が触れる。touches は2本、changedTouches は新しく触れた2本目。
    sendTouch(bodyOf(TOYOTA), 'touchstart', [finger(0, 100, 100), finger(1, 200, 100)], [finger(1, 200, 100)])
    // 1本目の指を離す。残っている touches は2本目だけ。
    const firstUp = sendTouch(window, 'touchend', [finger(1, 200, 100)], [finger(0, 100, 100)])
    // 続けて2本目も離す。
    sendTouch(window, 'touchend', [], [finger(1, 200, 100)])
    advance(500)

    expect(onOpen).not.toHaveBeenCalled()
    expect(firstUp, '開かないので touchend は止めない').toBe(true)
    // 長押しのドラッグも始まっていない。
    expect(announcement()).not.toContain(LIFTED)
  })

  test('補助 要件3: 「⋯」の上でタッチして離しても、onOpen は呼ばれず、その touchend は止められない（「⋯」の click が届くように）', () => {
    const onOpen = vi.fn()
    renderList(onOpen)
    const menuButton = screen.getByRole('button', { name: 'トヨタ自動車の操作' })

    sendTouch(menuButton, 'touchstart', [finger(0, 100, 100)])
    advance(100)
    const notPrevented = sendTouch(window, 'touchend', [], [finger(0, 100, 100)])

    expect(onOpen).not.toHaveBeenCalled()
    expect(notPrevented).toBe(true)
  })
})
