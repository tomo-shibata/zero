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
//     中で DndContext を持つが、保存はしない（QueryClientProvider は要らない）。
//   - カード（<li>）: tabIndex=0、aria-label「<銘柄コード> <銘柄名>」、aria-roledescription「並べ替えできるカード」。role="button" は付けない。
//   - 読み上げの文は dnd-kit のライブリージョン（role="status"）に出る。操作の説明は
//     「スペースキーで持ち上げ、矢印キーで動かし、もう一度スペースキーで置きます。Esc で取り消します。」。
//   - 始める・置くはスペースキー、取り消しは Esc。「⋯」を包む要素（data-no-drag）の中で起きた mousedown・touchstart と、
//     カード自身以外が対象の keydown では、ドラッグを始めない。
// テスト計画: プラン 5.1 の基準22、5.3 PR② の「E2E: 「⋯」にフォーカスして Space を押すと…」の jsdom で確かめられる部分。
//   並びの先行表示（要件17）・当たり判定・1,280px の矢印キー・タッチは CSS・レイアウトが要るので E2E（holdings-reorder.spec.ts）。
// jsdom は座標をすべて 0 にするので、位置（矩形）に依存する動きは確かめない。ここで確かめるのは、読み上げ・props・取り消しなど座標に依らないこと。
//   （持ち上げた直後の ← が捨てられないこと（EagerKeyboardSensor）も、座標が 0 だと over が持ち上げた時点から変わらず区別できないので、E2E で確かめる。）

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
