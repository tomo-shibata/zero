import { useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import type { KeyboardEvent } from 'react'
import './MenuButton.css'

/** ボタンに見える文字。aria-label が名前になるので、見た目の文字は固定にする。 */
const TRIGGER_TEXT = '⋯'

/** ボタンとメニューの間のすき間（px）。 */
const MENU_GAP_PX = 4

/** メニューの項目。 */
export type MenuItem = {
  /** 項目に出す文字。項目のアクセシブルな名前にもなる。 */
  label: string
  /** 項目を選んだとき（押す、または Enter・Space）に呼ばれる。メニューは先に閉じる。 */
  onSelect: () => void
}

type MenuButtonProps = {
  /** 「⋯」のボタンのアクセシブルな名前（例: 「トヨタ自動車の操作」）。 */
  label: string
  /** メニューの項目（表示する順）。空なら、メニューは開かない。 */
  items: readonly MenuItem[]
}

/** 開いたときにフォーカスを移す項目。 */
type InitialFocus = 'first' | 'last'

/**
 * 「⋯」のボタンと、押すと開くメニュー（値段の更新のプラン 6章の判断21・41）。開閉の状態はこのコンポーネントが持つ。
 * - 開くと先頭の項目にフォーカスが移る。「⋯」で上矢印を押して開いたときは最後の項目（下矢印は先頭）。
 *   開いたら、上下の矢印（端で折り返す）と Home・End で項目を移る。
 * - Esc で閉じて「⋯」にフォーカスを戻す。Esc は document で見張るので、メニューの外の余白を押してフォーカスが body に移っても閉じられる。
 *   Tab でも閉じる（Tab の移動は「⋯」から始まる）。
 * - メニューの外を押す、フォーカスが「⋯」とメニューの両方の外の要素へ移る、「⋯」の位置が変わるスクロール、画面の大きさの変化でも閉じる。
 *   スクロールと画面の大きさの変化で閉じるときは、フォーカスがメニューの中にあれば「⋯」に戻す（外にあればそのまま）。
 * - 項目を選ぶと、メニューを閉じて「⋯」にフォーカスを戻してから onSelect を呼ぶ。呼び出し側がダイアログを開くとき、
 *   ダイアログを閉じた後のフォーカスの戻り先（「⋯」）を、document.activeElement から取れる。
 * - 項目が空なら開かない。
 *
 * メニューは画面に固定した位置（position: fixed）に出す。表を囲む領域が横にスクロールしても、メニューが切れないため。
 * 位置はボタンの右下に合わせ、下に入りきらず上に空きがあるときは上に出す。どちらでも、画面の中に収める。
 * 通常の要素で描く（popover 属性は使わない。jsdom に Popover API がなく、テストで動かせないため）。
 */
export function MenuButton({ label, items }: MenuButtonProps) {
  const [open, setOpen] = useState(false)
  const menuId = useId()
  const triggerRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  // 開いたときにフォーカスを移す項目。開く操作（openMenu を呼ぶ所）が決める。
  const initialFocusRef = useRef<InitialFocus>('first')

  const openMenu = (initialFocus: InitialFocus) => {
    if (items.length === 0) return
    initialFocusRef.current = initialFocus
    setOpen(true)
  }

  // 開いたら、位置を決める（描画の前に決めるので、別の場所に一瞬出ることがない）。
  useLayoutEffect(() => {
    const trigger = triggerRef.current
    const menu = menuRef.current
    if (!open || !trigger || !menu) return
    const triggerRect = trigger.getBoundingClientRect()
    const menuRect = menu.getBoundingClientRect()
    // スクロールバーを除いた、見えている領域。
    const viewportWidth = document.documentElement.clientWidth
    const viewportHeight = document.documentElement.clientHeight
    const below = triggerRect.bottom + MENU_GAP_PX
    const above = triggerRect.top - MENU_GAP_PX - menuRect.height
    const fitsBelow = below + menuRect.height <= viewportHeight
    const top = !fitsBelow && above >= 0 ? above : below
    // 画面の中に収める（上端・下端、右端・左端からはみ出さない）。
    menu.style.top = `${clamp(top, 0, viewportHeight - menuRect.height)}px`
    menu.style.right = `${clamp(viewportWidth - triggerRect.right, 0, viewportWidth - menuRect.width)}px`
  }, [open])

  // 開いている間だけ、フォーカスを移し、閉じる条件を見張る。
  useEffect(() => {
    if (!open) return
    const entries = menuItems(menuRef.current)
    // 画面のスクロールを起こさない（スクロールで閉じる見張りに、開いた直後の自分の動きを拾わせないため）。
    const target = initialFocusRef.current === 'last' ? entries[entries.length - 1] : entries[0]
    target?.focus({ preventScroll: true })

    const openedRect = triggerRef.current?.getBoundingClientRect()
    const isInside = (node: EventTarget | null) =>
      node instanceof Node &&
      (triggerRef.current?.contains(node) === true || menuRef.current?.contains(node) === true)

    // スクロールや画面の大きさの変化で閉じるとき、フォーカスがメニューの中にあれば「⋯」に戻す。
    // 戻さないと、メニューが消えてフォーカスが body に落ちる。外にあるなら、利用者がいる所のままにする。
    const closeFromViewportChange = () => {
      const focusWasInMenu = menuRef.current?.contains(document.activeElement) === true
      setOpen(false)
      if (focusWasInMenu) triggerRef.current?.focus({ preventScroll: true })
    }
    // 押した瞬間（pointerdown）に判定する。「⋯」の上は対象外にして、「⋯」を押して閉じる動きは click に任せる。
    // メニューの中も対象外にするので、項目の click が届かなくならない。
    const closeOnOutsidePress = (event: PointerEvent) => {
      if (!isInside(event.target)) setOpen(false)
    }
    // フォーカスが「⋯」とメニューの両方の外の要素へ移ったら閉じる。body に移っただけ（余白を押した）では閉じない
    // （focusin は、フォーカスを受け取る要素にだけ起きる）。閉じるだけで、フォーカスは移した先に残す。
    const closeOnFocusOutside = (event: FocusEvent) => {
      if (!isInside(event.target)) setOpen(false)
    }
    // Esc は、フォーカスがどこにあっても（body でも）閉じて「⋯」に戻す。
    const closeOnEscape = (event: globalThis.KeyboardEvent) => {
      if (event.key !== 'Escape') return
      event.preventDefault()
      setOpen(false)
      triggerRef.current?.focus()
    }
    // 「⋯」の位置が変わるスクロールのときだけ閉じる。位置が同じなら、メニューは「⋯」に合ったままなので閉じない
    // （開いた直後に届く慣性のスクロールなどで閉じないように）。メニュー自身のスクロールでは閉じない。
    const closeOnScroll = (event: Event) => {
      if (event.target instanceof Node && menuRef.current?.contains(event.target)) return
      const rect = triggerRef.current?.getBoundingClientRect()
      if (rect && openedRect && (rect.top !== openedRect.top || rect.left !== openedRect.left)) {
        closeFromViewportChange()
      }
    }

    document.addEventListener('pointerdown', closeOnOutsidePress, true)
    document.addEventListener('focusin', closeOnFocusOutside)
    document.addEventListener('keydown', closeOnEscape)
    // scroll は泡立たないので、捕捉の段階（true）で、表を囲む領域などあらゆる要素のスクロールを拾う。
    window.addEventListener('scroll', closeOnScroll, true)
    window.addEventListener('resize', closeFromViewportChange)
    return () => {
      document.removeEventListener('pointerdown', closeOnOutsidePress, true)
      document.removeEventListener('focusin', closeOnFocusOutside)
      document.removeEventListener('keydown', closeOnEscape)
      window.removeEventListener('scroll', closeOnScroll, true)
      window.removeEventListener('resize', closeFromViewportChange)
    }
  }, [open])

  const closeAndFocusTrigger = () => {
    setOpen(false)
    triggerRef.current?.focus()
  }

  // 「⋯」にフォーカスがあるときの矢印キーで、メニューを開く（開いたメニューの中のキーは handleMenuKeyDown）。
  const handleTriggerKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (open) return
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      openMenu(event.key === 'ArrowDown' ? 'first' : 'last')
    }
  }

  // Esc は、フォーカスの場所によらず動くよう document で見張っている（上の effect）。
  const handleMenuKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const entries = menuItems(menuRef.current)
    const index = entries.findIndex((entry) => entry === event.target)
    switch (event.key) {
      case 'ArrowDown':
        event.preventDefault()
        entries[(index + 1) % entries.length]?.focus()
        break
      case 'ArrowUp':
        event.preventDefault()
        entries[index <= 0 ? entries.length - 1 : index - 1]?.focus()
        break
      case 'Home':
        event.preventDefault()
        entries[0]?.focus()
        break
      case 'End':
        event.preventDefault()
        entries[entries.length - 1]?.focus()
        break
      case 'Tab':
        // 移動はブラウザに任せる。「⋯」から始まるので、次（Shift なら前）の要素に移る。
        closeAndFocusTrigger()
        break
    }
  }

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        className="menu-button__trigger"
        aria-label={label}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        onClick={() => (open ? setOpen(false) : openMenu('first'))}
        onKeyDown={handleTriggerKeyDown}
      >
        {TRIGGER_TEXT}
      </button>
      {open && (
        <div
          ref={menuRef}
          id={menuId}
          role="menu"
          aria-label={label}
          className="menu-button__menu"
          onKeyDown={handleMenuKeyDown}
        >
          {items.map((item, index) => (
            <button
              // 同じ文言の項目があっても重ならないよう、位置も足す。
              key={`${index}:${item.label}`}
              type="button"
              role="menuitem"
              // 矢印キーで移る項目なので、Tab の順には入れない。
              tabIndex={-1}
              className="menu-button__item"
              onClick={() => {
                closeAndFocusTrigger()
                item.onSelect()
              }}
            >
              {item.label}
            </button>
          ))}
        </div>
      )}
    </>
  )
}

/** メニューの中の項目（表示された順）。メニューが開いていなければ空。 */
function menuItems(menu: HTMLElement | null): HTMLElement[] {
  return menu ? Array.from(menu.querySelectorAll<HTMLElement>('[role="menuitem"]')) : []
}

/** value を min 以上 max 以下にする。max が min より小さい（メニューが画面より大きい）ときは min（上端・右端を優先して見せる）。 */
function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(value, max))
}
