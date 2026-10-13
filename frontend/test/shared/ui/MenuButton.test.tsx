// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { MenuButton } from '../../../src/shared/ui/MenuButton'

// 仕様: 要件 docs/requirements/price-update.md の要件2（一覧の各行の右端の「⋯」。押すとメニューを開く。メニューは、項目を選ぶ、
// メニューの外を押す、Esc を押す、のどれかで閉じる。Esc で閉じたときは、フォーカスを「⋯」に戻す）。
// 境界: プラン docs/plans/price-update/plan.md 4.4
//   <MenuButton label="…" items={[{ label: '編集', onSelect }, …]} />  // 「⋯」のボタンとメニュー。開閉の状態は MenuButton が持つ
//   ボタンは aria-haspopup="menu"・aria-expanded・aria-label=label。開いたメニューは role="menu"、項目は role="menuitem"。
//   判断21: キーボードは、開くと先頭の項目にフォーカス、上下の矢印で移る、Esc で閉じて「⋯」に戻る、Tab で閉じる。外を押すと閉じる。
// テスト計画: プラン 5.2 の PR② の行
//   「role（menu・menuitem）と aria-expanded、開くと先頭の項目にフォーカス、上下の矢印で移る、Esc で閉じて「⋯」にフォーカス、
//    Tab で閉じる、外を押すと閉じる、項目を選ぶと閉じて onSelect が動く」。
// onSelect は呼び出された項目の名前を selected に残す関数にする（項目を選んだ結果＝どの項目の処理が動いたかを確かめるため）。
// 項目の数は、矢印で順に移れるよう3つにする（本番の使い方は PR② で「削除」だけ、PR③ で「編集」「削除」）。
// 後から足したもの（ステップ6 1ラウンド目の指摘、判断41）: 上下の矢印の端での折り返しと Home・End、「⋯」で矢印を押して開く、
//   項目が空なら開かない、フォーカスが外へ出たときと document の Esc で閉じる、スクロール（「⋯」の位置が変わったときだけ閉じる。
//   メニュー自身のスクロールでは閉じない）と画面の大きさの変化で閉じる（閉じたときフォーカスがメニューの中なら「⋯」に戻る）。
// 単体テストで確かめないもの: メニューを出す位置（画面の中に収める。jsdom は位置を計算しない）。位置は E2E（price-update.spec.ts）で確かめる。
// フォーカスの移動（focus・blur）はブラウザの動きで React の状態を変えるので、act で包んでから確かめる。

const BUTTON_NAME = 'トヨタ自動車の操作'
const OUTSIDE_BUTTON = '外のボタン'

let selected: string[] = []

afterEach(() => {
  cleanup()
  selected = []
})

function renderMenuButton() {
  selected = []
  return render(
    <>
      <button type="button">{OUTSIDE_BUTTON}</button>
      <MenuButton
        label={BUTTON_NAME}
        items={[
          { label: '編集', onSelect: () => selected.push('編集') },
          { label: '複製', onSelect: () => selected.push('複製') },
          { label: '削除', onSelect: () => selected.push('削除') },
        ]}
      />
    </>,
  )
}

function triggerButton(): HTMLElement {
  return screen.getByRole('button', { name: BUTTON_NAME })
}

/** メニューの項目（表示された順）。開いていなければ空。 */
function menuItems(): HTMLElement[] {
  return screen.queryAllByRole('menuitem')
}

/** 今フォーカスがある要素にキーを押す（キーボードの操作は、フォーカスのある要素に届く）。 */
function pressKey(key: string): void {
  fireEvent.keyDown(document.activeElement ?? document.body, { key })
}

/** 実際にポインターで押したときに起きる一連のイベントを、target に起こす。 */
function pointerPress(target: Element): void {
  fireEvent.pointerDown(target)
  fireEvent.mouseDown(target)
  fireEvent.pointerUp(target)
  fireEvent.mouseUp(target)
  fireEvent.click(target)
}

/** 「⋯」を押してメニューを開き、先頭の項目にフォーカスが移るのを待つ。 */
async function openMenu(): Promise<void> {
  fireEvent.click(triggerButton())
  await waitFor(() => expect(menuItems()[0]).toHaveFocus())
}

describe('閉じているとき', () => {
  test('補助 判断21・要件2: 「⋯」のボタンは aria-label が label、aria-haspopup="menu"、aria-expanded="false" で、メニューも項目も出ていない', () => {
    renderMenuButton()

    const button = triggerButton()
    expect(button).toHaveAttribute('aria-haspopup', 'menu')
    expect(button).toHaveAttribute('aria-expanded', 'false')
    expect(button).toHaveTextContent('⋯')
    expect(screen.queryByRole('menu')).not.toBeInTheDocument()
    expect(menuItems()).toHaveLength(0)
  })
})

describe('開く', () => {
  test('補助 判断21・要件2: 「⋯」を押すと、aria-expanded が "true" になり、role="menu" のメニューに role="menuitem" の項目が渡した順に出る', async () => {
    renderMenuButton()

    await openMenu()

    expect(triggerButton()).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByRole('menu')).toBeInTheDocument()
    expect(menuItems().map((item) => item.textContent)).toEqual(['編集', '複製', '削除'])
    // 開いただけでは、どの項目の処理も動かない。
    expect(selected).toEqual([])
  })

  test('補助 判断21: 開くと先頭の項目「編集」にフォーカスが移る（真ん中や最後の項目ではない）', async () => {
    renderMenuButton()

    await openMenu()

    expect(screen.getByRole('menuitem', { name: '編集' })).toHaveFocus()
    expect(screen.getByRole('menuitem', { name: '複製' })).not.toHaveFocus()
    expect(screen.getByRole('menuitem', { name: '削除' })).not.toHaveFocus()
  })
})

describe('矢印キー', () => {
  test('補助 判断21: 下矢印で 編集 → 複製 → 削除 の順にフォーカスが移り、上矢印で 削除 → 複製 → 編集 の順に戻る', async () => {
    renderMenuButton()
    await openMenu()

    pressKey('ArrowDown')
    expect(screen.getByRole('menuitem', { name: '複製' })).toHaveFocus()
    pressKey('ArrowDown')
    expect(screen.getByRole('menuitem', { name: '削除' })).toHaveFocus()
    pressKey('ArrowUp')
    expect(screen.getByRole('menuitem', { name: '複製' })).toHaveFocus()
    pressKey('ArrowUp')
    expect(screen.getByRole('menuitem', { name: '編集' })).toHaveFocus()

    // 矢印キーで移っただけでは閉じず、どの項目の処理も動かない。
    expect(screen.getByRole('menu')).toBeInTheDocument()
    expect(triggerButton()).toHaveAttribute('aria-expanded', 'true')
    expect(selected).toEqual([])
  })
})

describe('閉じる', () => {
  test('補助 判断21・要件2: Esc を押すとメニューが閉じ、aria-expanded が "false" になり、フォーカスが「⋯」に戻る（項目の処理は動かない）', async () => {
    renderMenuButton()
    await openMenu()

    pressKey('Escape')

    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
    expect(triggerButton()).toHaveAttribute('aria-expanded', 'false')
    await waitFor(() => expect(triggerButton()).toHaveFocus())
    expect(selected).toEqual([])
  })

  test('補助 判断21・要件2: 矢印で別の項目に移した後に Esc を押しても、メニューが閉じてフォーカスが「⋯」に戻る', async () => {
    renderMenuButton()
    await openMenu()
    pressKey('ArrowDown')
    expect(screen.getByRole('menuitem', { name: '複製' })).toHaveFocus()

    pressKey('Escape')

    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
    await waitFor(() => expect(triggerButton()).toHaveFocus())
    expect(selected).toEqual([])
  })

  test('補助 判断21: Tab を押すとメニューが閉じる（項目の処理は動かない）', async () => {
    renderMenuButton()
    await openMenu()

    pressKey('Tab')

    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
    expect(triggerButton()).toHaveAttribute('aria-expanded', 'false')
    expect(selected).toEqual([])
  })

  test('補助 要件2: メニューの外を押すとメニューが閉じる（項目の処理は動かない）', async () => {
    renderMenuButton()
    await openMenu()

    pointerPress(screen.getByRole('button', { name: OUTSIDE_BUTTON }))

    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
    expect(triggerButton()).toHaveAttribute('aria-expanded', 'false')
    expect(selected).toEqual([])
  })

  test('補助 要件2: ボタンやメニュー以外の画面の余白（body）を押しても閉じる', async () => {
    renderMenuButton()
    await openMenu()

    pointerPress(document.body)

    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
    expect(selected).toEqual([])
  })
})

describe('項目を選ぶ', () => {
  test('補助 要件2: 真ん中の項目「複製」を押すと、その項目の onSelect だけが動き、メニューが閉じる', async () => {
    renderMenuButton()
    await openMenu()

    pointerPress(screen.getByRole('menuitem', { name: '複製' }))

    expect(selected).toEqual(['複製'])
    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
    expect(triggerButton()).toHaveAttribute('aria-expanded', 'false')
  })

  test('補助 要件2: 最後の項目「削除」を押すと、その項目の onSelect だけが動く', async () => {
    renderMenuButton()
    await openMenu()

    pointerPress(screen.getByRole('menuitem', { name: '削除' }))

    expect(selected).toEqual(['削除'])
    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
  })

  test('補助 要件2: 閉じた後にもう一度「⋯」を押すと、メニューがまた開き、先頭の項目にフォーカスが移る', async () => {
    renderMenuButton()
    await openMenu()
    pointerPress(screen.getByRole('menuitem', { name: '削除' }))
    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())

    await openMenu()

    expect(screen.getByRole('menu')).toBeInTheDocument()
    expect(menuItems().map((item) => item.textContent)).toEqual(['編集', '複製', '削除'])
  })
})

// ======================================================================
// PR② ステップ6 1ラウンド目の指摘で足したテスト（判断41、テストの網羅の指摘3、境界値と異常系の指摘3〜5・9・10、デザインの指摘7・8）
// ======================================================================

describe('矢印キーの端と Home・End（判断21・テストの網羅の指摘3）', () => {
  test('補助 判断21: 先頭で上矢印を押すと最後へ、最後で下矢印を押すと先頭へ、フォーカスが折り返す', async () => {
    renderMenuButton()
    await openMenu()

    pressKey('ArrowUp')
    expect(screen.getByRole('menuitem', { name: '削除' })).toHaveFocus()
    pressKey('ArrowDown')
    expect(screen.getByRole('menuitem', { name: '編集' })).toHaveFocus()
  })

  test('補助 判断21: Home で先頭の項目、End で最後の項目にフォーカスが移る（真ん中の項目からでも）', async () => {
    renderMenuButton()
    await openMenu()
    pressKey('ArrowDown')
    expect(screen.getByRole('menuitem', { name: '複製' })).toHaveFocus()

    pressKey('End')
    expect(screen.getByRole('menuitem', { name: '削除' })).toHaveFocus()
    pressKey('ArrowUp')
    expect(screen.getByRole('menuitem', { name: '複製' })).toHaveFocus()
    pressKey('Home')
    expect(screen.getByRole('menuitem', { name: '編集' })).toHaveFocus()
    // 移っただけでは閉じず、どの項目の処理も動かない。
    expect(screen.getByRole('menu')).toBeInTheDocument()
    expect(selected).toEqual([])
  })
})

describe('項目を選んだとき（判断41・境界値と異常系の指摘9）', () => {
  test('補助 判断41: onSelect が呼ばれた時点で、フォーカスは「⋯」に戻っている（呼び出し側がダイアログを開く前の戻り先を取れる）', async () => {
    let activeAtSelect: Element | null = null
    render(
      <MenuButton
        label={BUTTON_NAME}
        items={[
          {
            label: '削除',
            onSelect: () => {
              activeAtSelect = document.activeElement
            },
          },
        ]}
      />,
    )
    fireEvent.click(triggerButton())
    await waitFor(() => expect(menuItems()[0]).toHaveFocus())

    pointerPress(screen.getByRole('menuitem', { name: '削除' }))

    expect(activeAtSelect).toBe(triggerButton())
  })
})

describe('「⋯」でのキー操作で開く（判断41・デザインの指摘8）', () => {
  test('補助 判断41: 「⋯」にフォーカスがあって下矢印を押すと、メニューが開いて先頭の項目にフォーカスが移る', async () => {
    renderMenuButton()
    triggerButton().focus()

    fireEvent.keyDown(triggerButton(), { key: 'ArrowDown' })

    await waitFor(() => expect(screen.getByRole('menu')).toBeInTheDocument())
    await waitFor(() => expect(screen.getByRole('menuitem', { name: '編集' })).toHaveFocus())
    expect(triggerButton()).toHaveAttribute('aria-expanded', 'true')
  })

  test('補助 判断41: 「⋯」にフォーカスがあって上矢印を押すと、メニューが開いて最後の項目にフォーカスが移る', async () => {
    renderMenuButton()
    triggerButton().focus()

    fireEvent.keyDown(triggerButton(), { key: 'ArrowUp' })

    await waitFor(() => expect(screen.getByRole('menu')).toBeInTheDocument())
    await waitFor(() => expect(screen.getByRole('menuitem', { name: '削除' })).toHaveFocus())
  })
})

describe('項目が空のとき（境界値と異常系の指摘10）', () => {
  test('補助 判断41: 項目が空なら、「⋯」を押しても下矢印を押しても開かない（aria-expanded は "false" のまま、メニューも出ない）', () => {
    render(<MenuButton label={BUTTON_NAME} items={[]} />)

    fireEvent.click(triggerButton())
    expect(screen.queryByRole('menu')).not.toBeInTheDocument()
    expect(triggerButton()).toHaveAttribute('aria-expanded', 'false')

    triggerButton().focus()
    fireEvent.keyDown(triggerButton(), { key: 'ArrowDown' })
    expect(screen.queryByRole('menu')).not.toBeInTheDocument()
    expect(triggerButton()).toHaveAttribute('aria-expanded', 'false')
  })
})

describe('フォーカスが項目の外へ出たとき（判断41・仕様の指摘3・境界値と異常系の指摘4）', () => {
  test('補助 判断41: 開いた後にフォーカスが body に移っても（項目の外の余白を押した場合）メニューは開いたままで、document で Esc を押すと閉じて「⋯」にフォーカスが移る', async () => {
    renderMenuButton()
    await openMenu()

    // 項目の外の余白（フォーカスできない所）を押すと、フォーカスは body に移る。
    const focused = document.activeElement
    if (focused instanceof HTMLElement) {
      const element = focused
      act(() => element.blur())
    }
    expect(document.body).toHaveFocus()
    expect(screen.getByRole('menu')).toBeInTheDocument()

    fireEvent.keyDown(document, { key: 'Escape' })

    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
    await waitFor(() => expect(triggerButton()).toHaveFocus())
    expect(selected).toEqual([])
  })

  test('補助 判断41: フォーカスが「⋯」とメニューの両方の外（外のボタン）へ移ったら、メニューが閉じる（フォーカスは外のボタンに残る）', async () => {
    renderMenuButton()
    await openMenu()

    act(() => screen.getByRole('button', { name: OUTSIDE_BUTTON }).focus())

    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
    expect(triggerButton()).toHaveAttribute('aria-expanded', 'false')
    expect(screen.getByRole('button', { name: OUTSIDE_BUTTON })).toHaveFocus()
    expect(selected).toEqual([])
  })

  test('補助 判断41: フォーカスがメニューの中の項目から「⋯」へ移っても、閉じない（「⋯」もメニューの仲間）', async () => {
    renderMenuButton()
    await openMenu()

    act(() => triggerButton().focus())

    expect(screen.getByRole('menu')).toBeInTheDocument()
  })
})

describe('スクロールと画面の大きさの変化（判断41・境界値と異常系の指摘3・デザインの指摘7）', () => {
  // jsdom は位置を計算しない（getBoundingClientRect はいつも 0）。「⋯」の位置は、ブラウザの API の穴埋めとして、テストが決めた値を返す。
  let triggerTop = 100

  function stubTriggerPosition(): void {
    vi.spyOn(triggerButton(), 'getBoundingClientRect').mockImplementation(
      () =>
        ({
          x: 200,
          y: triggerTop,
          top: triggerTop,
          bottom: triggerTop + 24,
          left: 200,
          right: 232,
          width: 32,
          height: 24,
          toJSON: () => ({}),
        }) as DOMRect,
    )
  }

  afterEach(() => {
    vi.restoreAllMocks()
    triggerTop = 100
  })

  test('補助 判断41: window のスクロールで「⋯」の位置が変わっていなければ、メニューは閉じない（開いた直後の慣性のスクロールなど）', async () => {
    renderMenuButton()
    stubTriggerPosition()
    await openMenu()

    fireEvent.scroll(window)

    expect(screen.getByRole('menu')).toBeInTheDocument()
    expect(triggerButton()).toHaveAttribute('aria-expanded', 'true')
  })

  test('補助 判断41: window のスクロールで「⋯」の位置が変わっていれば、メニューが閉じる。フォーカスがメニューの中にあったので、「⋯」に戻る', async () => {
    renderMenuButton()
    stubTriggerPosition()
    await openMenu()
    expect(menuItems()[0]).toHaveFocus()

    triggerTop = 60
    fireEvent.scroll(window)

    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
    await waitFor(() => expect(triggerButton()).toHaveFocus())
  })

  test('補助 判断41: メニュー自身のスクロールでは、「⋯」の位置が変わっていても閉じない', async () => {
    renderMenuButton()
    stubTriggerPosition()
    await openMenu()

    triggerTop = 60
    fireEvent.scroll(screen.getByRole('menu'))

    expect(screen.getByRole('menu')).toBeInTheDocument()
  })

  test('補助 判断41: window の resize でメニューが閉じ、フォーカスがメニューの中にあったので「⋯」に戻る', async () => {
    renderMenuButton()
    await openMenu()
    expect(menuItems()[0]).toHaveFocus()

    fireEvent(window, new Event('resize'))

    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
    await waitFor(() => expect(triggerButton()).toHaveFocus())
  })

  test('補助 判断41: window の resize でメニューが閉じても、フォーカスがメニューの外（外のボタン）にあれば、「⋯」には戻さない', async () => {
    renderMenuButton()
    await openMenu()
    // フォーカスをメニューの外へ移す（フォーカスの移動で先に閉じる実装でも、フォーカスは外のボタンにある）。
    act(() => screen.getByRole('button', { name: OUTSIDE_BUTTON }).focus())

    fireEvent(window, new Event('resize'))

    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
    expect(screen.getByRole('button', { name: OUTSIDE_BUTTON })).toHaveFocus()
  })
})
