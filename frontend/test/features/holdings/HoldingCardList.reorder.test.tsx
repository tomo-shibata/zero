// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import type { Holding } from '../../../src/entities/holding/model'
import { HoldingCardList } from '../../../src/features/holdings/components/HoldingCardList'

// 仕様: カード表示の要件 docs/requirements/holdings-cards.md の要件19（キーボードで並べ替える。Esc で取り消す）・要件20（読み上げ）・
//   要件21（「⋯」でドラッグは始まらない）・要件22（取り消したときは元の並びに戻し、保存しない）、基準22（単体）。
// 境界: プラン docs/plans/holdings-cards/plan.md 4.4「PR② 並べ替え」・4.6 PR②
//   - HoldingCardList の props: { holdings; onDelete; onMove: (activeCode: string, overCode: string | null) => void }。
//     並べ替えはライブラリを使わず自前で作ってある（hook useCardReorder）が、保存はしない（QueryClientProvider は要らない）。
//   - カード（<li>）: tabIndex=0、aria-label「<銘柄コード> <銘柄名>」、aria-roledescription「並べ替えできるカード」。role="button" は付けない。
//   - 読み上げの文は HoldingCardList が <ul> の外に持つライブリージョン（role="status" aria-live="assertive"）に出る。操作の説明は
//     「スペースキーで持ち上げ、矢印キーで動かし、もう一度スペースキーで置きます。Esc で取り消します。」。
//   - 始める・置くはスペースキー、取り消しは Esc。「⋯」を包む要素（data-no-drag）の中で起きた mousedown・touchstart と、
//     カード自身以外が対象の keydown では、ドラッグを始めない。
// テスト計画: プラン 5.1 の基準22、5.3 PR② の「E2E: 「⋯」にフォーカスして Space を押すと…」の jsdom で確かめられる部分。
//   並びの先行表示（要件17）・当たり判定・1,280px の矢印キー・タッチは CSS・レイアウトが要るので E2E（holdings-reorder.spec.ts）。
// jsdom は座標をすべて 0 にするので、位置（矩形）に依存する動きは確かめない。ここで確かめるのは、読み上げ・props・取り消しなど座標に依らないこと。
//   （持ち上げた直後の ← が捨てられないこと（持ち上げた keydown そのものは document の見張りで読み飛ばし、次の keydown から扱う）も、座標が 0 だと置き場所が持ち上げた時点から変わらず区別できないので、E2E で確かめる。）

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
const APPLE: Holding = {
  code: 'AAPL',
  name: 'Apple',
  currency: 'USD',
  quantity: 10,
  acquisitionPrice: '150.25',
  currentPrice: '180.00',
  priceDate: '2026-10-06',
  valuation: '1800.00',
  unrealizedGain: '297.50',
  unrealizedGainRate: '19.8',
}
const THREE: readonly Holding[] = [SONY, TOYOTA, APPLE]

const INSTRUCTIONS = 'スペースキーで持ち上げ、矢印キーで動かし、もう一度スペースキーで置きます。Esc で取り消します。'
const SPACE = { key: ' ', code: 'Space' }
const ARROW_LEFT = { key: 'ArrowLeft', code: 'ArrowLeft' }
const ESCAPE = { key: 'Escape', code: 'Escape' }

let fetchMock: ReturnType<typeof vi.fn<typeof fetch>>

beforeEach(() => {
  // 保存は HoldingCardList の外（useHoldingsOrder）でする。ここでは fetch が呼ばれないことを確かめる。
  fetchMock = vi.fn<typeof fetch>(() => Promise.reject(new Error('この画面は fetch を呼ばない')))
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

/** カードの並び（<ul aria-label="保有株式">）の、カード（<li>）を DOM の順に返す。 */
function cards(): HTMLElement[] {
  return within(screen.getByRole('list', { name: '保有株式' })).getAllByRole('listitem')
}

/** DOM の順の銘柄コード。 */
function codes(): (string | null)[] {
  return cards().map((card) => within(card).getByText(/^(6758|7203|9984|AAPL)$/).textContent)
}

/** カード code の <li>（名前「<銘柄コード> <銘柄名>」）。 */
function card(holding: Holding): HTMLElement {
  return screen.getByLabelText(`${holding.code} ${holding.name}`, { selector: 'li' })
}

/** 読み上げ用のライブリージョン（role="status"）の文。 */
function announcement(): string {
  return screen.getByRole('status').textContent ?? ''
}

/** 少し待つ（キーボードの操作が受け付けられる状態になるのを待つ。「起きない」ことの確認の前にも使う）。 */
async function settle(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 30))
  })
}

describe('カードの属性（4.4）', () => {
  test('カード表示の補助 4.4: カード（<li>）は tabIndex 0・名前「<銘柄コード> <銘柄名>」・aria-roledescription「並べ替えできるカード」で、role="button" は付かない', () => {
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={() => {}} />)

    for (const holding of THREE) {
      const li = card(holding)
      expect(li.tagName).toBe('LI')
      expect(li).toHaveAttribute('tabindex', '0')
      expect(li).toHaveAttribute('aria-roledescription', '並べ替えできるカード')
      expect(li).not.toHaveAttribute('role', 'button')
    }
    // 「⋯」のボタンだけがボタン（カードがボタンになると、ボタンの中にボタンが入る）。
    expect(screen.getAllByRole('button')).toHaveLength(3)
  })

  test('カード表示の補助 4.4: 操作の説明（aria-describedby が指す文）は「スペースキーで持ち上げ、矢印キーで動かし、もう一度スペースキーで置きます。Esc で取り消します。」', () => {
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={() => {}} />)

    const describedBy = card(APPLE).getAttribute('aria-describedby')
    expect(describedBy).not.toBeNull()
    expect(document.getElementById(describedBy ?? '')).toHaveTextContent(INSTRUCTIONS)
  })
})

describe('キーボードでの取り消し（基準22・要件19・要件20・要件22）', () => {
  test('カード表示の基準22: AAPL のカードで スペース、←、Esc の順に押すと、並びは変わらず、「並べ替えを取り消しました。」が読み上げ用に出て、onMove は並べ替えの形で呼ばれず、fetch は呼ばれない', async () => {
    const onMove = vi.fn()
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={onMove} />)
    const apple = card(APPLE)
    apple.focus()

    // スペースで持ち上げる（要件20: 「AAPL Appleを持ち上げました。矢印キーで動かし、スペースキーで置きます。」）。
    fireEvent.keyDown(apple, SPACE)
    await waitFor(() => expect(announcement()).toBe('AAPL Appleを持ち上げました。矢印キーで動かし、スペースキーで置きます。'))
    // ← で1つ前へ動かす（持ち上げている間に動かす）。
    fireEvent.keyDown(apple, ARROW_LEFT)
    await settle()
    // Esc で取り消す。
    fireEvent.keyDown(apple, ESCAPE)

    await waitFor(() => expect(announcement()).toBe('並べ替えを取り消しました。'))
    expect(codes()).toEqual(['6758', '7203', 'AAPL'])
    // 並べ替えの形（over が null でない）で呼ばれていない。呼ばれるなら over は null。
    for (const [, over] of onMove.mock.calls) {
      expect(over).toBeNull()
    }
    expect(fetchMock).not.toHaveBeenCalled()
  })

  test('カード表示の補助 要件19: スペースで持ち上げてすぐ Esc で取り消しても、並びは変わらず onMove は並べ替えの形で呼ばれない', async () => {
    const onMove = vi.fn()
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={onMove} />)
    const sony = card(SONY)
    sony.focus()

    fireEvent.keyDown(sony, SPACE)
    await waitFor(() => expect(announcement()).toContain('6758 ソニーグループを持ち上げました。'))
    fireEvent.keyDown(sony, ESCAPE)

    await waitFor(() => expect(announcement()).toBe('並べ替えを取り消しました。'))
    expect(codes()).toEqual(['6758', '7203', 'AAPL'])
    for (const [, over] of onMove.mock.calls) {
      expect(over).toBeNull()
    }
  })

  test('カード表示の補助 判断9: スペースで持ち上げ、Tab を押すと取り消され、「並べ替えを取り消しました。」が出て、並びは変わらず、onMove は並べ替えの形で呼ばれず、フォーカスはそのカードのまま', async () => {
    const onMove = vi.fn()
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={onMove} />)
    const apple = card(APPLE)
    apple.focus()

    fireEvent.keyDown(apple, SPACE)
    await waitFor(() => expect(announcement()).toContain('AAPL Appleを持ち上げました。'))
    fireEvent.keyDown(apple, { key: 'Tab', code: 'Tab' })

    await waitFor(() => expect(announcement()).toBe('並べ替えを取り消しました。'))
    expect(codes()).toEqual(['6758', '7203', 'AAPL'])
    for (const [, over] of onMove.mock.calls) {
      expect(over).toBeNull()
    }
    expect(apple).toHaveFocus()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  test('カード表示の補助 要件19: Enter では持ち上がらない（始める・置くはスペースキーだけ）', async () => {
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={() => {}} />)
    const apple = card(APPLE)
    apple.focus()

    fireEvent.keyDown(apple, { key: 'Enter', code: 'Enter' })
    await settle()

    expect(announcement()).toBe('')
  })
})

describe('「⋯」ではドラッグを始めない（要件21）', () => {
  test('カード表示の補助 要件21: 「⋯」のボタンでスペースを押しても持ち上がらない（読み上げが出ず、メニューの操作として扱われる）', async () => {
    const onMove = vi.fn()
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={onMove} />)
    const menuButton = screen.getByRole('button', { name: 'Appleの操作' })
    menuButton.focus()

    fireEvent.keyDown(menuButton, SPACE)
    await settle()

    expect(announcement()).toBe('')
    expect(onMove).not.toHaveBeenCalled()
  })

  test('カード表示の補助 要件21: カードの本体でマウスを押して動かすとドラッグが始まり（読み上げが出る）、「⋯」の上でマウスを押して10px動かしても始まらない', async () => {
    const onMove = vi.fn()
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={onMove} />)

    // 「⋯」の上: 押して 10px 動かして離しても、ドラッグは始まらない。
    const menuButton = screen.getByRole('button', { name: 'トヨタ自動車の操作' })
    fireEvent.mouseDown(menuButton, { button: 0, clientX: 100, clientY: 100 })
    fireEvent.mouseMove(document, { clientX: 112, clientY: 112 })
    fireEvent.mouseUp(document, { clientX: 112, clientY: 112 })
    await settle()
    expect(announcement()).toBe('')
    expect(onMove).not.toHaveBeenCalled()

    // 対照: 同じカードの本体（銘柄名）で同じ操作をすると、ドラッグが始まる（jsdom でもドラッグが始まることの確認）。
    const body = within(card(TOYOTA)).getByText('トヨタ自動車', { selector: 'p' })
    fireEvent.mouseDown(body, { button: 0, clientX: 100, clientY: 100 })
    fireEvent.mouseMove(document, { clientX: 112, clientY: 112 })
    await waitFor(() => expect(announcement()).toContain('7203 トヨタ自動車を持ち上げました。'))
    fireEvent.mouseUp(document, { clientX: 112, clientY: 112 })
  })
})

// =====================================================================================================
// ステップ6の指摘で足したテスト（プラン 4.4「PR② 並べ替え」の座標・マウス・タッチ・取り消し・読み上げの決まり）。
// 持ち上げた後の置き場所は座標に依るので（jsdom は座標が 0）、ここでは「始まる・始まらない」「取り消す・取り消さない」「置かない」を、読み上げと onMove で確かめる。
// =====================================================================================================

const LIFTED = '持ち上げました'
const CANCELLED = '並べ替えを取り消しました。'

/** カード holding の本体（銘柄名。「⋯」を包む要素の外）。 */
function bodyOf(holding: Holding): HTMLElement {
  return within(card(holding)).getByText(holding.name, { selector: 'p' })
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

/** 指 id が (x, y) にある。 */
function finger(identifier: number, x: number, y: number): TouchPoint {
  return { identifier, clientX: x, clientY: y }
}

/** window の error（イベントの見張りが例外を投げたとき、jsdom はここへ報告する）を集める。 */
function collectWindowErrors(): { errors: ErrorEvent[]; stop: () => void } {
  const errors: ErrorEvent[] = []
  const onError = (event: ErrorEvent) => {
    event.preventDefault()
    errors.push(event)
  }
  window.addEventListener('error', onError)
  return { errors, stop: () => window.removeEventListener('error', onError) }
}

describe('マウス（プラン 4.4 判断9）', () => {
  test('カード表示の補助 判断9: 5px に満たない動き（4px・斜めに 3px と 3px）ではドラッグが始まらず、読み上げは空のまま。離しても onMove は呼ばれない', async () => {
    const onMove = vi.fn()
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={onMove} />)

    fireEvent.mouseDown(bodyOf(APPLE), { button: 0, clientX: 100, clientY: 100 })
    fireEvent.mouseMove(document, { clientX: 104, clientY: 100 })
    fireEvent.mouseMove(document, { clientX: 103, clientY: 103 })
    expect(announcement()).toBe('')
    fireEvent.mouseUp(document, { button: 0, clientX: 103, clientY: 103 })
    await settle()

    expect(announcement()).toBe('')
    expect(onMove).not.toHaveBeenCalled()
  })

  test('カード表示の補助 判断9: 右ボタン（button: 2）の mousedown からは、動かしてもドラッグが始まらない', async () => {
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={() => {}} />)

    fireEvent.mouseDown(bodyOf(APPLE), { button: 2, buttons: 2, clientX: 100, clientY: 100 })
    fireEvent.mouseMove(document, { buttons: 2, clientX: 140, clientY: 140 })
    await settle()

    expect(announcement()).toBe('')
  })

  test('カード表示の補助 判断9: マウスのドラッグ中に、左以外のボタンの mouseup では置かず（まだ持ち上げている）、左ボタンの mouseup で終わる', () => {
    const onMove = vi.fn()
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={onMove} />)
    fireEvent.mouseDown(bodyOf(APPLE), { button: 0, clientX: 100, clientY: 100 })
    fireEvent.mouseMove(document, { clientX: 112, clientY: 112 })
    expect(announcement()).toContain(LIFTED)

    fireEvent.mouseUp(document, { button: 2, clientX: 112, clientY: 112 })

    expect(announcement()).toContain(LIFTED)
    expect(announcement()).not.toContain(CANCELLED)
    // 対照: 左ボタンの mouseup でドラッグは終わる（jsdom は座標が 0 で、離した所はどのカードの上でもないので取り消し）。
    fireEvent.mouseUp(document, { button: 0, clientX: 112, clientY: 112 })
    expect(announcement()).toBe(CANCELLED)
    expect(onMove).not.toHaveBeenCalled()
  })

  test('カード表示の補助 判断9: buttons: 1 で mousedown した後、buttons: 0 の mousemove が来たら待つのをやめ、その後 buttons: 1 で動かしてもドラッグは始まらない', async () => {
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={() => {}} />)

    fireEvent.mouseDown(bodyOf(APPLE), { button: 0, buttons: 1, clientX: 100, clientY: 100 })
    // 左ボタンが離されたのに mouseup が届かなかった（ブラウザの文字のドラッグの後など）。
    fireEvent.mouseMove(document, { buttons: 0, clientX: 102, clientY: 100 })
    fireEvent.mouseMove(document, { buttons: 1, clientX: 130, clientY: 130 })
    await settle()

    expect(announcement()).toBe('')
  })

  test('カード表示の補助 判断9: buttons: 1 で mousedown し、buttons: 1 のまま 12px 動かすとドラッグが始まる（対照）。buttons を付けない mousedown（合成のイベント）では、buttons: 0 の mousemove でも始まる', () => {
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={() => {}} />)

    fireEvent.mouseDown(bodyOf(APPLE), { button: 0, buttons: 1, clientX: 100, clientY: 100 })
    fireEvent.mouseMove(document, { buttons: 1, clientX: 112, clientY: 112 })
    expect(announcement()).toContain(LIFTED)
    fireEvent.mouseUp(document, { button: 0, buttons: 0, clientX: 112, clientY: 112 })
    expect(announcement()).toBe(CANCELLED)

    // buttons を持たない mousedown（今のテストの形。jsdom の mousemove は buttons が 0）。
    fireEvent.mouseDown(bodyOf(TOYOTA), { button: 0, clientX: 100, clientY: 100 })
    fireEvent.mouseMove(document, { clientX: 112, clientY: 112 })
    expect(announcement()).toContain('7203 トヨタ自動車を持ち上げました。')
  })

  test('カード表示の補助 判断9: buttons: 1 で始めたドラッグの途中で、左ボタンが押されていない mousemove（buttons: 0）が来たら、その場で取り消す', () => {
    const onMove = vi.fn()
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={onMove} />)
    fireEvent.mouseDown(bodyOf(APPLE), { button: 0, buttons: 1, clientX: 100, clientY: 100 })
    fireEvent.mouseMove(document, { buttons: 1, clientX: 112, clientY: 112 })
    expect(announcement()).toContain(LIFTED)

    fireEvent.mouseMove(document, { buttons: 0, clientX: 120, clientY: 120 })

    expect(announcement()).toBe(CANCELLED)
    expect(onMove).not.toHaveBeenCalled()
  })
})

describe('画面から外したときの後始末（プラン 4.4「取り消しのほかの場面と後始末」）', () => {
  test('カード表示の補助 4.4: マウスでドラッグを始めるとページ（body）に class が付き、画面から外すと消える。その後 document に mousemove・mouseup を送っても例外は出ない', () => {
    const onMove = vi.fn()
    const { unmount } = render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={onMove} />)
    expect(document.body.classList.length, '前提: ドラッグの前は body に class がない').toBe(0)
    fireEvent.mouseDown(bodyOf(APPLE), { button: 0, clientX: 100, clientY: 100 })
    fireEvent.mouseMove(document, { clientX: 112, clientY: 112 })
    expect(document.body.classList.length, 'マウスでドラッグしている間は body に class が付く').toBeGreaterThan(0)

    const caught = collectWindowErrors()
    try {
      unmount()
      expect(document.body.classList.length, '画面から外したら class が消える').toBe(0)
      fireEvent.mouseMove(document, { clientX: 150, clientY: 150 })
      fireEvent.mouseUp(document, { button: 0, clientX: 150, clientY: 150 })
      fireEvent.keyDown(document, ESCAPE)
      fireEvent.blur(window)
    } finally {
      caught.stop()
    }

    expect(caught.errors.map((error) => error.message)).toEqual([])
    expect(onMove).not.toHaveBeenCalled()
  })

  test('カード表示の補助 4.4: マウスで待っている間（5px 動く前）に画面から外しても、その後の mousemove でドラッグが始まらず、例外も出ない', () => {
    const onMove = vi.fn()
    const { unmount } = render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={onMove} />)
    fireEvent.mouseDown(bodyOf(APPLE), { button: 0, clientX: 100, clientY: 100 })

    const caught = collectWindowErrors()
    try {
      unmount()
      fireEvent.mouseMove(document, { clientX: 140, clientY: 140 })
      fireEvent.mouseUp(document, { button: 0, clientX: 140, clientY: 140 })
    } finally {
      caught.stop()
    }

    expect(caught.errors.map((error) => error.message)).toEqual([])
    expect(document.body.classList.length).toBe(0)
    expect(onMove).not.toHaveBeenCalled()
  })
})

describe('タッチ（プラン 4.4 判断9・要件18）', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  /** 指を進める（act の中で、タイマーを進める）。 */
  function advance(ms: number): void {
    act(() => {
      vi.advanceTimersByTime(ms)
    })
  }

  test('カード表示の補助 判断9: 指1本で触れて 0.25 秒（250ms）たつとドラッグが始まり「持ち上げました」と読み上げられる。それより前（200ms）では始まらない', () => {
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={() => {}} />)

    sendTouch(bodyOf(APPLE), 'touchstart', [finger(0, 100, 100)])
    advance(200)
    expect(announcement()).toBe('')
    advance(50)

    expect(announcement()).toContain(LIFTED)
    expect(announcement()).toContain('AAPL Apple')
  })

  test('カード表示の補助 判断9: 0.25 秒たつ前に 5px より動いたら（6px）ドラッグは始まらない（ブラウザのスクロールに任せる）', () => {
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={() => {}} />)

    sendTouch(bodyOf(APPLE), 'touchstart', [finger(0, 100, 100)])
    advance(100)
    sendTouch(window, 'touchmove', [finger(0, 100, 106)])
    advance(500)

    expect(announcement()).toBe('')
  })

  test('カード表示の補助 判断9: 0.25 秒たつ前の動きが 5px 以内（3px）なら、待ち続けてドラッグが始まる', () => {
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={() => {}} />)

    sendTouch(bodyOf(APPLE), 'touchstart', [finger(0, 100, 100)])
    advance(100)
    sendTouch(window, 'touchmove', [finger(0, 102, 103)])
    advance(150)

    expect(announcement()).toContain(LIFTED)
  })

  test('カード表示の補助 判断9: 0.25 秒たつ前に指を離したら、ドラッグは始まらない', () => {
    const onMove = vi.fn()
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={onMove} />)

    sendTouch(bodyOf(APPLE), 'touchstart', [finger(0, 100, 100)])
    advance(100)
    sendTouch(window, 'touchend', [], [finger(0, 100, 100)])
    advance(500)

    expect(announcement()).toBe('')
    expect(onMove).not.toHaveBeenCalled()
  })

  test('カード表示の補助 判断9: 0.25 秒たつ前に2本目の指が触れたら（window に touchstart、touches が2本）、ドラッグは始まらない', () => {
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={() => {}} />)

    sendTouch(bodyOf(APPLE), 'touchstart', [finger(0, 100, 100)])
    advance(100)
    sendTouch(window, 'touchstart', [finger(0, 100, 100), finger(1, 200, 200)], [finger(1, 200, 200)])
    advance(500)

    expect(announcement()).toBe('')
  })

  test('カード表示の補助 判断9: 最初から指が2本の touchstart では、ドラッグは始まらない', () => {
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={() => {}} />)

    sendTouch(bodyOf(APPLE), 'touchstart', [finger(0, 100, 100), finger(1, 200, 200)], [finger(0, 100, 100), finger(1, 200, 200)])
    advance(500)

    expect(announcement()).toBe('')
  })

  test('カード表示の補助 判断9: ドラッグ中の touchcancel で取り消され、「並べ替えを取り消しました。」が出て、onMove は呼ばれない', () => {
    const onMove = vi.fn()
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={onMove} />)
    sendTouch(bodyOf(APPLE), 'touchstart', [finger(0, 100, 100)])
    advance(250)
    expect(announcement()).toContain(LIFTED)

    sendTouch(window, 'touchcancel', [], [finger(0, 100, 100)])

    expect(announcement()).toBe(CANCELLED)
    expect(onMove).not.toHaveBeenCalled()
  })

  test('カード表示の補助 判断9: ドラッグ中に2本目の指が触れたら取り消される', () => {
    const onMove = vi.fn()
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={onMove} />)
    sendTouch(bodyOf(APPLE), 'touchstart', [finger(0, 100, 100)])
    advance(250)
    expect(announcement()).toContain(LIFTED)

    sendTouch(window, 'touchstart', [finger(0, 100, 100), finger(1, 200, 200)], [finger(1, 200, 200)])

    expect(announcement()).toBe(CANCELLED)
    expect(onMove).not.toHaveBeenCalled()
  })

  test('カード表示の補助 判断9: window の touchmove は、ドラッグ中でないとき（何もしていない・待っている間）は preventDefault されず、ドラッグ中は preventDefault される', () => {
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={() => {}} />)

    // 何もしていないとき。
    expect(sendTouch(window, 'touchmove', [finger(0, 100, 120)]), '何もしていないときの touchmove は、続けて処理される（defaultPrevented でない）').toBe(true)

    // 待っている間（画面のスクロールを止めない）。
    sendTouch(bodyOf(APPLE), 'touchstart', [finger(0, 100, 100)])
    advance(100)
    expect(sendTouch(window, 'touchmove', [finger(0, 100, 102)]), '待っている間の touchmove').toBe(true)

    // ドラッグ中（画面のスクロールを止める）。
    advance(200)
    expect(announcement()).toContain(LIFTED)
    expect(sendTouch(window, 'touchmove', [finger(0, 100, 130)]), 'ドラッグ中の touchmove は preventDefault される').toBe(false)
  })

  test('カード表示の補助 4.4: 待っている間に画面から外しても、250ms 進めて何も起きない（例外も出ず、onMove も呼ばれず、タイマーが残らない）', () => {
    const onMove = vi.fn()
    const { unmount } = render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={onMove} />)
    sendTouch(bodyOf(APPLE), 'touchstart', [finger(0, 100, 100)])

    const caught = collectWindowErrors()
    try {
      unmount()
      expect(vi.getTimerCount(), '画面から外したら、0.25 秒のタイマーは残らない').toBe(0)
      advance(250)
    } finally {
      caught.stop()
    }

    expect(caught.errors.map((error) => error.message)).toEqual([])
    expect(onMove).not.toHaveBeenCalled()
  })
})

describe('キーボードの取り消しのほかの場面と置く操作（プラン 4.4 キーボード・取り消し）', () => {
  test('カード表示の補助 4.4: キーボードで持ち上げた後、holdings の銘柄が減ったら取り消され、その後スペースを押しても onMove は呼ばれない', () => {
    const onMove = vi.fn()
    const { rerender } = render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={onMove} />)
    const apple = card(APPLE)
    apple.focus()
    fireEvent.keyDown(apple, SPACE)
    expect(announcement()).toContain(LIFTED)

    rerender(<HoldingCardList holdings={[SONY, APPLE]} onDelete={() => {}} onMove={onMove} />)

    expect(announcement()).toBe(CANCELLED)
    fireEvent.keyDown(document, SPACE)
    expect(onMove).not.toHaveBeenCalled()
    expect(announcement()).toBe(CANCELLED)
  })

  test('カード表示の補助 4.4: キーボードで持ち上げた後、値段だけ変えた holdings（並びは同じ）で描き直しても取り消されず、まだ持ち上げている（スペースで置ける）', () => {
    const onMove = vi.fn()
    const { rerender } = render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={onMove} />)
    const apple = card(APPLE)
    apple.focus()
    fireEvent.keyDown(apple, SPACE)
    expect(announcement()).toContain(LIFTED)

    const appleRepriced: Holding = {
      code: 'AAPL',
      name: 'Apple',
      currency: 'USD',
      quantity: 10,
      acquisitionPrice: '150.25',
      currentPrice: '190.00',
      priceDate: '2026-10-06',
      valuation: '1900.00',
      unrealizedGain: '397.50',
      unrealizedGainRate: '26.4',
    }
    const repriced: readonly Holding[] = [SONY, TOYOTA, appleRepriced]
    rerender(<HoldingCardList holdings={repriced} onDelete={() => {}} onMove={onMove} />)

    expect(announcement()).toContain(LIFTED)
    expect(announcement()).not.toContain(CANCELLED)
    // まだ持ち上げているので、スペースで置ける（元のマスに置くので並びは変わらず onMove は呼ばれない）。
    fireEvent.keyDown(document, SPACE)
    expect(announcement()).toContain('置きました')
    expect(onMove).not.toHaveBeenCalled()
  })

  test('カード表示の補助 4.4: キーボードで持ち上げている間に window の blur が来たら取り消される', () => {
    const onMove = vi.fn()
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={onMove} />)
    const apple = card(APPLE)
    apple.focus()
    fireEvent.keyDown(apple, SPACE)
    expect(announcement()).toContain(LIFTED)

    fireEvent.blur(window)

    expect(announcement()).toBe(CANCELLED)
    expect(onMove).not.toHaveBeenCalled()
  })

  test('カード表示の補助 4.4: スペースを押し続けたとき（2回目の keydown が repeat: true）は置かず、押し直したとき（repeat でない keydown）に置く', () => {
    render(<HoldingCardList holdings={THREE} onDelete={() => {}} onMove={() => {}} />)
    const apple = card(APPLE)
    apple.focus()
    fireEvent.keyDown(apple, SPACE)
    expect(announcement()).toContain(LIFTED)

    fireEvent.keyDown(apple, { ...SPACE, repeat: true })
    expect(announcement(), '押し続けている間は、置かずに持ち上げたまま').toContain(LIFTED)
    expect(announcement()).not.toContain('置きました')

    fireEvent.keyDown(apple, SPACE)
    expect(announcement()).toContain('AAPL Appleを3番目に置きました。')
  })
})
