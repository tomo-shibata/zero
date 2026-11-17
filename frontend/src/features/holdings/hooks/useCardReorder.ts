import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type {
  DragEvent,
  FocusEvent,
  KeyboardEvent as ReactKeyboardEvent,
  MouseEvent as ReactMouseEvent,
  RefObject,
  TouchEvent as ReactTouchEvent,
} from 'react'
import type { Holding } from '../../../entities/holding/model'
import { edgeScrollDelta, slotAtPoint, verticalSlot } from '../lib/cardSlots'
import type { Point, Rect } from '../lib/cardSlots'
import {
  CANCELLED_ANNOUNCEMENT,
  droppedAnnouncement,
  movedAnnouncement,
  pickedUpAnnouncement,
} from '../lib/reorderAnnouncements'
import { reorderCodes } from '../lib/reorderCodes'

/** カード（<li>）に付ける、銘柄コードを持たせる属性（フォーカスのあったカードを覚えて、並びが変わった後に探すため）。 */
export const CODE_ATTRIBUTE = 'data-holding-code'

/** 「⋯」を包む要素に付ける属性。この中で押したマウス・タッチでは、ドラッグを始めない（要件21）。 */
const NO_DRAG_SELECTOR = '[data-no-drag]'

/** マウスで、この距離（px）動かしたらドラッグを始める。クリックでドラッグを始めないため（判断9）。 */
const MOUSE_DRAG_DISTANCE_PX = 5
/** タッチで、押し続けたらドラッグを始める時間（ミリ秒）と、その間に動いてよい距離（px。超えたらスクロールとして扱う。要件18）。 */
const TOUCH_DRAG_DELAY_MS = 250
const TOUCH_DRAG_TOLERANCE_PX = 5

/** キーボードで動かした先のマスを画面に入れるときの、画面の端との余白（px）。 */
const SCROLL_MARGIN_PX = 16

/** MouseEvent.buttons の、左ボタンのビット。 */
const LEFT_BUTTON = 1

/** 前と同じ読み上げの文を続けて入れるとき、一度空にしてから入れ直すまでの時間（ミリ秒。内容が同じだと変化として伝わらないため）。 */
const REANNOUNCE_DELAY_MS = 50

/** マウスのドラッグ中に、ページに付ける class（文字の選択を止める。HoldingCardList.css）。 */
const MOUSE_DRAGGING_BODY_CLASS = 'holding-card-reorder-mouse'

type Mode = 'mouse' | 'touch' | 'keyboard'

/**
 * ドラッグ中の状態。持ち上げたときに測ったマスは、ドラッグの間、変えない（グリッドは詰めて並べるので、位置が動かない）。
 * 座標は、マス・押した点・付いていく量のすべてが、カードの並び（<ul>）の左上からの相対（ページのスクロールや、並びより上の欄の高さに依らないため）。
 */
type Drag = {
  /** 持ち上げたカードの銘柄コード。 */
  code: string
  mode: Mode
  /** 持ち上げたときの並び（銘柄コード）。マスの番号はこの並びの番号。 */
  order: readonly string[]
  /** 持ち上げたときの、各カードの矩形（<ul> からの相対）。order と同じ並び。 */
  slots: readonly Rect[]
  /** 持ち上げたときのカードの並び（<ul>）の範囲（<ul> からの相対なので、左上は 0, 0）。この外なら置き場所なし。 */
  bounds: Rect
  /** 持ち上げたときの画面の幅。変わったら取り消す。 */
  viewportWidth: number
  /** マウス・タッチ: 押した点（<ul> からの相対）。 */
  grab: Point
  /** マウス・タッチ: ポインターの今の位置（画面の座標）。スクロールしたとき、同じ位置から決め直すために覚える。 */
  client: Point | null
  /** マウス・タッチ: 持ち上げたカードをポインターに付いていかせる量（押した点からの、<ul> からの相対の差）。 */
  offset: Point
  /** 今の置き場所（何番目のマスか。なしなら null）。 */
  over: number | null
  /** マウス: mousedown のときに左ボタンが押されていると分かった（buttons を持つイベントだった）。mousemove で左ボタンが離れていたら取り消す。 */
  checkButtons: boolean
  /** タッチ: 始めた指の identifier。 */
  touchId: number | null
}

/** 始めるのを待っている状態（マウスは5px 動くまで、タッチは0.25秒たつまで）。 */
type Pending = {
  mode: 'mouse' | 'touch'
  code: string
  startClient: Point
  /** 押した点（<ul> からの相対）。 */
  startGrab: Point
  timer: ReturnType<typeof setTimeout> | null
  checkButtons: boolean
  touchId: number | null
}

/** 置いた並びを覚える。base は置いたときの holdings の props（props が変わったら、この並びは捨てて props に従う）。 */
type Dropped = { codes: readonly string[]; base: readonly Holding[] }

/** effect の中の処理を、カードのイベントの受け取りから呼ぶための入り口。 */
type Api = {
  mouseDown: (event: ReactMouseEvent<HTMLElement>, code: string) => void
  touchStart: (event: ReactTouchEvent<HTMLElement>, code: string) => void
  keyDown: (event: ReactKeyboardEvent<HTMLElement>, code: string) => void
  click: (event: ReactMouseEvent<HTMLElement>, code: string) => void
  cancel: () => void
  dragStart: () => void
  /** フォーカスが外れたとき、キーボードで持ち上げているカードから外へ移ったなら取り消す。 */
  focusOut: (event: FocusEvent<HTMLUListElement>) => void
}

type CardProps = {
  [CODE_ATTRIBUTE]: string
  className: string | undefined
  style: { transform: string } | undefined
  onMouseDown: (event: ReactMouseEvent<HTMLElement>) => void
  onTouchStart: (event: ReactTouchEvent<HTMLElement>) => void
  onKeyDown: (event: ReactKeyboardEvent<HTMLElement>) => void
  onClick: (event: ReactMouseEvent<HTMLElement>) => void
}

type ListProps = {
  ref: RefObject<HTMLUListElement | null>
  onFocus: (event: FocusEvent<HTMLUListElement>) => void
  onBlur: (event: FocusEvent<HTMLUListElement>) => void
  onDragStart: (event: DragEvent<HTMLUListElement>) => void
  'data-reordering': string | undefined
}

type CardReorder = {
  /** 描くカード（置いた直後は、置いた並び。それ以外は props の並び）。 */
  holdings: readonly Holding[]
  /** 読み上げさせる文（ライブリージョンに入れる）。 */
  announcement: string
  listProps: ListProps
  /** カード（<li>）に足す属性。 */
  cardProps: (code: string) => CardProps
}

type Options = {
  holdings: readonly Holding[]
  /** 置いたときに、並びが変わるなら呼ばれる。overCode は置いた位置にあったカードの銘柄コード。 */
  onMove: (activeCode: string, overCode: string) => void
  /**
   * カードを「押した」ときに呼ばれる（銘柄ごとの売買履歴の要件1〜要件3。プラン 4.5）。押すとは、ドラッグを始めずに離すこと
   * （マウスは同じカードの上で左ボタンを離す、タッチは0.25秒未満で指を動かさずに離す）と、何もしていないときのカードでの Enter。
   * 「⋯」の中のできごとと、持ち上げている間の Enter では呼ばれない。
   */
  onOpen: (code: string) => void
}

/**
 * カードの並べ替えの操作（マウス・タッチ・キーボード。要件17〜22。カード表示のプラン 4.4「PR② 並べ替え」）。
 * 保存はしない（置いたときに onMove を呼ぶだけ）。
 *
 * 持ち上げたときに、各カードの矩形（マス）を測って覚える。ドラッグの間、DOM の順は変えず（変えるとフォーカスが外れる）、
 * 離したときの並びを、各カードを元のマスから新しいマスへ transform で動かして先に見せる。
 * `document`・`window` の見張りは、ref に持った状態を読む（描画の前の古い値を読まないため）。
 */
export function useCardReorder({ holdings, onMove, onOpen }: Options): CardReorder {
  const listRef = useRef<HTMLUListElement | null>(null)
  const [drag, setDrag] = useState<Drag | null>(null)
  const [announcement, setAnnouncement] = useState('')
  const [dropped, setDropped] = useState<Dropped | null>(null)

  // 置いた直後は、props（TanStack Query のキャッシュ）が変わるのを待たずに、置いた並びで描く。
  // props が変わったら（キャッシュが書き換わった、保存の失敗で戻した、取り直した）、この並びは捨てて props に従う。
  const rendered = useMemo(() => {
    if (dropped === null || dropped.base !== holdings) return holdings
    const byCode = new Map(holdings.map((holding) => [holding.code, holding]))
    return dropped.codes.flatMap((code) => byCode.get(code) ?? [])
  }, [holdings, dropped])

  const latestRef = useRef({ rendered, base: holdings, onMove, onOpen })
  useLayoutEffect(() => {
    latestRef.current = { rendered, base: holdings, onMove, onOpen }
  })
  const dragRef = useRef<Drag | null>(null)
  const pendingRef = useRef<Pending | null>(null)
  const apiRef = useRef<Api | null>(null)

  useEffect(() => {
    let frame: number | null = null
    // 持ち上げた keydown そのもの（同じネイティブイベント）。React は #root でイベントを受けるので、同じスペースの keydown が
    // この後 document に届く。読み飛ばさないと、持ち上げた瞬間に置いてしまう。
    let liftEvent: Event | null = null

    const commit = (next: Drag | null) => {
      dragRef.current = next
      setDrag(next)
    }
    const nameOf = (code: string) => latestRef.current.rendered.find((holding) => holding.code === code)?.name ?? ''
    /** 画面の座標を、<ul> の左上からの相対にする。<ul> の位置はその場で読む（スクロールや、上の欄の高さの変化に依らないため）。 */
    const toListRelative = (client: Point): Point | null => {
      const list = listRef.current
      if (list === null) return null
      const rect = list.getBoundingClientRect()
      return { x: client.x - rect.left, y: client.y - rect.top }
    }

    // 読み上げ。前と同じ文を続けて入れるときは、一度空にしてから入れる（内容が同じだと変化として伝わらず、読み上げられないため）。
    let announced = ''
    let announceTimer: ReturnType<typeof setTimeout> | null = null
    const announce = (text: string) => {
      if (announceTimer !== null) clearTimeout(announceTimer)
      announceTimer = null
      if (text === announced) {
        setAnnouncement('')
        announceTimer = setTimeout(() => {
          announceTimer = null
          setAnnouncement(text)
        }, REANNOUNCE_DELAY_MS)
      } else {
        setAnnouncement(text)
      }
      announced = text
    }

    const stopAutoScroll = () => {
      if (frame !== null) cancelAnimationFrame(frame)
      frame = null
    }
    const clearPending = () => {
      const pending = pendingRef.current
      if (pending?.timer != null) clearTimeout(pending.timer)
      pendingRef.current = null
    }

    /** その場で取り消す（並びは変えず、保存もしない）。ドラッグ中だったときだけ読み上げる。 */
    const cancel = () => {
      clearPending()
      if (dragRef.current === null) return
      stopAutoScroll()
      commit(null)
      announce(CANCELLED_ANNOUNCEMENT)
    }

    /** 持ち上げる。マウス・タッチは、ポインターの位置から置き場所を決める（並びの外なら null）。 */
    const startDrag = (
      mode: Mode,
      code: string,
      grab: Point,
      client: Point | null,
      extra: { checkButtons: boolean; touchId: number | null },
    ) => {
      const list = listRef.current
      if (list === null) return
      const order = latestRef.current.rendered.map((holding) => holding.code)
      const startIndex = order.indexOf(code)
      const measured = measureSlots(list, order)
      if (startIndex < 0 || measured === null) return
      let over: number | null = startIndex
      let offset: Point = { x: 0, y: 0 }
      if (mode !== 'keyboard' && client !== null) {
        const point = toListRelative(client)
        if (point === null) return
        over = slotAtPoint(measured.slots, measured.bounds, point)
        offset = { x: point.x - grab.x, y: point.y - grab.y }
      }
      commit({
        code,
        mode,
        order,
        slots: measured.slots,
        bounds: measured.bounds,
        viewportWidth: window.innerWidth,
        grab,
        client,
        offset,
        over,
        checkButtons: extra.checkButtons,
        touchId: extra.touchId,
      })
      announce(pickedUpAnnouncement(code, nameOf(code)))
      scheduleAutoScroll()
    }

    /** マウス・タッチ: ポインターの位置から、付いていく量と置き場所を決め直す。 */
    const applyPointer = (client: Point) => {
      const current = dragRef.current
      if (current === null || current.mode === 'keyboard') return
      const point = toListRelative(client)
      if (point === null) return
      const over = slotAtPoint(current.slots, current.bounds, point)
      const offset = { x: point.x - current.grab.x, y: point.y - current.grab.y }
      const unchanged =
        current.over === over &&
        current.client?.x === client.x &&
        current.client.y === client.y &&
        current.offset.x === offset.x &&
        current.offset.y === offset.y
      if (!unchanged) {
        commit({ ...current, client, offset, over })
        // 置き場所なしになったときは読み上げない（前の文のまま）。
        if (over !== null && over !== current.over) announce(movedAnnouncement(over + 1))
      }
      scheduleAutoScroll()
    }

    /** ポインターが画面の上端・下端に近いとき、1フレームごとに少しずつスクロールして、置き場所を決め直す。 */
    function scheduleAutoScroll() {
      if (frame !== null) return
      const current = dragRef.current
      if (current?.client == null || edgeScrollDelta(current.client.y, window.innerHeight) === 0) return
      frame = requestAnimationFrame(() => {
        frame = null
        const latest = dragRef.current
        if (latest?.client == null) return
        const delta = edgeScrollDelta(latest.client.y, window.innerHeight)
        if (delta === 0) return
        const before = window.scrollY
        // 動かなかった（端に着いた）ら、次のポインターの動きまでやめる。
        if (scrollWindowBy(delta) !== before) applyPointer(latest.client)
      })
    }

    /** 置く。置き場所なしなら取り消し。並びが変わるときは、置いた並びを覚えて描き、onMove を呼ぶ。 */
    const finish = () => {
      const current = dragRef.current
      if (current === null) return
      if (current.over === null) {
        cancel()
        return
      }
      const overCode = current.order[current.over]
      const next = reorderCodes(current.order, current.code, overCode)
      stopAutoScroll()
      commit(null)
      announce(droppedAnnouncement(current.code, nameOf(current.code), current.over + 1))
      if (next !== null) {
        setDropped({ codes: next, base: latestRef.current.base })
        latestRef.current.onMove(current.code, overCode)
      }
    }

    /** キーボード: 置き場所を別のマスへ動かす（動かないなら何もしない）。 */
    const moveOver = (next: number | null) => {
      const current = dragRef.current
      if (current === null || next === null || next === current.over) return
      if (next < 0 || next >= current.slots.length) return
      commit({ ...current, over: next })
      announce(movedAnnouncement(next + 1))
      scrollIntoView(listRef.current, current.slots[next])
    }

    // mouseup で開いたことの印。すぐ後に届く click（detail 0）で、同じ押す操作を2回開かないため。
    // 残しっぱなしにすると、後の支援技術の click を飲み込むので、次の mousedown か、次のタスクで消す。
    let openedByMouseUp = false
    let openedTimer: ReturnType<typeof setTimeout> | null = null
    const clearOpenedByMouseUp = () => {
      openedByMouseUp = false
      if (openedTimer !== null) clearTimeout(openedTimer)
      openedTimer = null
    }
    const markOpenedByMouseUp = () => {
      clearOpenedByMouseUp()
      openedByMouseUp = true
      openedTimer = setTimeout(clearOpenedByMouseUp, 0)
    }

    const idle = () => dragRef.current === null && pendingRef.current === null
    const isInNoDrag = (target: EventTarget | null) =>
      target instanceof Element && target.closest(NO_DRAG_SELECTOR) !== null
    /** target を含むカード（<li>）の銘柄コード。カードの外なら null。 */
    const cardCodeOf = (target: EventTarget | null): string | null =>
      target instanceof Element ? (target.closest('li')?.getAttribute(CODE_ATTRIBUTE) ?? null) : null

    const api: Api = {
      cancel,
      dragStart: () => {
        // 選んだ文字やリンクのブラウザのドラッグが始まった。こちらのドラッグを待つのをやめる。
        if (pendingRef.current?.mode === 'mouse') clearPending()
      },
      focusOut: (event) => {
        // キーボードで持ち上げているカードから、フォーカスがほかの要素へ移った（移った先のキーを奪わないよう取り消す）。
        // カードの中の「⋯」へ移ったときも取り消す（キーボードでは Tab で取り消すので、ここへ来るのはマウスで「⋯」を押したときだけ。
        // 持ち上げたままだと、メニューの中の矢印キー・スペースがカードも動かしてしまうため。PR② のデザインの指摘12・エッジの指摘15）。
        const current = dragRef.current
        if (current?.mode !== 'keyboard') return
        const card = event.target instanceof Element ? event.target.closest('li') : null
        if (card?.getAttribute(CODE_ATTRIBUTE) !== current.code) return
        const next = event.relatedTarget
        if (next === card) return
        cancel()
      },
      mouseDown: (event, code) => {
        // 新しい押す操作が始まったので、前の印は捨てる。
        clearOpenedByMouseUp()
        if (!idle() || event.button !== 0 || isInNoDrag(event.target)) return
        const startClient = { x: event.clientX, y: event.clientY }
        const startGrab = toListRelative(startClient)
        if (startGrab === null) return
        // buttons を持つイベント（左ボタンが押されている）のときだけ、mousemove で左ボタンが離れていないかを見る。
        const checkButtons = (event.buttons & LEFT_BUTTON) !== 0
        pendingRef.current = { mode: 'mouse', code, startClient, startGrab, timer: null, checkButtons, touchId: null }
      },
      touchStart: (event, code) => {
        if (!idle() || isInNoDrag(event.target)) return
        const touch = event.touches[0]
        if (event.touches.length !== 1 || touch === undefined) return
        const startClient = { x: touch.clientX, y: touch.clientY }
        const startGrab = toListRelative(startClient)
        if (startGrab === null) return
        const timer = setTimeout(() => {
          const pending = pendingRef.current
          if (pending?.mode !== 'touch' || pending.timer !== timer) return
          pendingRef.current = null
          startDrag('touch', pending.code, pending.startGrab, pending.startClient, {
            checkButtons: false,
            touchId: pending.touchId,
          })
        }, TOUCH_DRAG_DELAY_MS)
        pendingRef.current = {
          mode: 'touch',
          code,
          startClient,
          startGrab,
          timer,
          checkButtons: false,
          touchId: touch.identifier,
        }
      },
      click: (event, code) => {
        // detail が 0 の click は、マウスを押したものではなく、支援技術（NVDA の閲覧モードの Enter・VoiceOver のダブルタップ）が作るもの。
        // 何もしていないときだけ、開く操作として受ける。マウスの click（detail が1以上。ドラッグの後にも届く）は使わない（プラン 4.5）。
        if (event.detail !== 0 || !idle() || isInNoDrag(event.target)) return
        // 直前の mouseup で開いたなら、この click（同じ押す操作の続き）では開かない。
        if (openedByMouseUp) {
          clearOpenedByMouseUp()
          return
        }
        latestRef.current.onOpen(code)
      },
      keyDown: (event, code) => {
        // 何もしていないときだけ扱う（持ち上げている間の Enter は開かない）。「⋯」やメニューの中のキーでは始めない（要件21）。
        if (!idle() || event.target !== event.currentTarget || event.repeat) return
        if (event.key === 'Enter') {
          latestRef.current.onOpen(code)
          return
        }
        if (!isSpace(event)) return
        event.preventDefault()
        liftEvent = event.nativeEvent
        startDrag('keyboard', code, { x: 0, y: 0 }, null, { checkButtons: false, touchId: null })
      },
    }
    apiRef.current = api

    const onMouseMove = (event: MouseEvent) => {
      const client = { x: event.clientX, y: event.clientY }
      const leftReleased = (event.buttons & LEFT_BUTTON) === 0
      const pending = pendingRef.current
      if (pending?.mode === 'mouse') {
        // 左ボタンが離されたのに mouseup が届かなかった（ブラウザの文字のドラッグの後など）。待つのをやめる。
        if (pending.checkButtons && leftReleased) {
          clearPending()
          return
        }
        if (distance(client, pending.startClient) < MOUSE_DRAG_DISTANCE_PX) return
        pendingRef.current = null
        // 押した点から付いていく。始めたときに、文字の選択が始まっていたら消す。
        window.getSelection()?.removeAllRanges()
        startDrag('mouse', pending.code, pending.startGrab, client, { checkButtons: pending.checkButtons, touchId: null })
        return
      }
      const current = dragRef.current
      if (current?.mode !== 'mouse') return
      if (current.checkButtons && leftReleased) {
        cancel()
        return
      }
      applyPointer(client)
    }
    const onMouseUp = (event: MouseEvent) => {
      // 左ボタン以外の mouseup では置かない（左ボタンで持ち上げている間に、ほかのボタンを離しただけのとき）。
      if (event.button !== 0) return
      const pending = pendingRef.current
      clearPending()
      if (dragRef.current?.mode === 'mouse') {
        finish()
        return
      }
      // 待っている状態のまま（5px 動かさずに）、押したのと同じカードの上で離した: 押した（カードを開く。プラン 4.5）。
      if (pending?.mode === 'mouse' && !isInNoDrag(event.target) && cardCodeOf(event.target) === pending.code) {
        // 支援技術の押す操作は、作り物の mousedown → mouseup → click（detail 0）を続けて送る。続く click で2回目を開かないよう、印を残す。
        markOpenedByMouseUp()
        latestRef.current.onOpen(pending.code)
      }
    }

    /** 待っている・ドラッグ中のタッチの、始めた指の identifier（なければ null）。 */
    const activeTouchId = (): number | null => {
      const pending = pendingRef.current
      if (pending?.mode === 'touch') return pending.touchId
      const current = dragRef.current
      return current?.mode === 'touch' ? current.touchId : null
    }
    const onTouchStart = (event: TouchEvent) => {
      // 2本目の指が触れた: 待つのをやめる（ブラウザのスクロールやピンチに任せる）／ドラッグを取り消す。
      if (event.touches.length < 2) return
      if (pendingRef.current?.mode === 'touch') clearPending()
      if (dragRef.current?.mode === 'touch') cancel()
    }
    const onTouchMove = (event: TouchEvent) => {
      const id = activeTouchId()
      if (id === null) return
      const pending = pendingRef.current
      const touch = findTouch(event.changedTouches, id) ?? findTouch(event.touches, id)
      if (pending?.mode === 'touch') {
        // 押し続ける前に動かした（スクロール）: 待つのをやめて、ブラウザのスクロールに任せる。
        if (touch !== undefined && distance({ x: touch.clientX, y: touch.clientY }, pending.startClient) > TOUCH_DRAG_TOLERANCE_PX) {
          clearPending()
        }
        return
      }
      // ドラッグ中だけ、画面のスクロールを止める（待っている間は止めない）。
      if (event.cancelable) event.preventDefault()
      if (touch !== undefined) applyPointer({ x: touch.clientX, y: touch.clientY })
    }
    const onTouchEnd = (event: TouchEvent) => {
      const id = activeTouchId()
      if (id === null || findTouch(event.changedTouches, id) === undefined) return
      const pending = pendingRef.current
      clearPending()
      // 長押しでドラッグを始めていた（0.25秒以上、指を動かさずに離した場合も含む）: 置く。この touchend の既定の動作も止める。止めないと、
      // 指を動かさずに離したとき、ブラウザがタップとして互換の mousedown・mouseup を送り、マウスの側でカードが開いてしまう（要件3）。
      if (dragRef.current?.mode === 'touch') {
        if (event.cancelable) event.preventDefault()
        finish()
        return
      }
      // 0.25秒たつ前に、指を動かさずに離した: 押した（カードを開く）。この touchend の既定の動作を止める。止めないと、ブラウザが
      // タップの後に互換の mousedown・mouseup を送り、マウスの側でもう一度開いて、履歴に同じ画面が2つ積まれうる（プラン 4.5）。
      if (pending?.mode === 'touch') {
        if (event.cancelable) event.preventDefault()
        latestRef.current.onOpen(pending.code)
      }
    }
    const onTouchCancel = (event: TouchEvent) => {
      const id = activeTouchId()
      if (id === null || findTouch(event.changedTouches, id) === undefined) return
      clearPending()
      if (dragRef.current?.mode === 'touch') cancel()
    }
    const onScroll = () => {
      // ホイールでのスクロールでは mousemove が来ないので、ポインターの位置から決め直す。キーボードは決め直さない。
      const current = dragRef.current
      if (current?.client != null) applyPointer(current.client)
    }
    const onContextMenu = (event: Event) => {
      if (dragRef.current !== null && dragRef.current.mode !== 'keyboard') event.preventDefault()
    }
    const onResize = () => {
      const current = dragRef.current
      if (current !== null && window.innerWidth !== current.viewportWidth) cancel()
    }
    const onHidden = () => {
      if (document.visibilityState === 'hidden') cancel()
    }
    const onKeyDown = (event: KeyboardEvent) => {
      const current = dragRef.current
      if (current === null || event === liftEvent) return
      if (event.key === 'Escape') {
        event.preventDefault()
        cancel()
        return
      }
      if (current.mode !== 'keyboard') return
      switch (event.key) {
        case 'Tab':
          // フォーカスはカードに残す（持ち上げたまま別の要素のキーを奪わないよう、取り消す）。
          event.preventDefault()
          cancel()
          return
        case ' ':
        case 'Spacebar':
          event.preventDefault()
          if (!event.repeat) finish()
          return
        case 'ArrowLeft':
        case 'ArrowRight': {
          event.preventDefault()
          if (current.over !== null) moveOver(current.over + (event.key === 'ArrowLeft' ? -1 : 1))
          return
        }
        case 'ArrowUp':
        case 'ArrowDown':
          event.preventDefault()
          if (current.over !== null) moveOver(verticalSlot(current.slots, current.over, event.key === 'ArrowUp' ? 'up' : 'down'))
          return
      }
    }

    // touchmove は、画面を置いている間ずっと付けておく（途中で足した見張りの preventDefault は、iOS Safari で効かないため）。
    window.addEventListener('touchstart', onTouchStart, { passive: true })
    window.addEventListener('touchmove', onTouchMove, { passive: false })
    window.addEventListener('touchend', onTouchEnd)
    window.addEventListener('touchcancel', onTouchCancel)
    window.addEventListener('scroll', onScroll, { passive: true })
    window.addEventListener('contextmenu', onContextMenu)
    window.addEventListener('resize', onResize)
    window.addEventListener('blur', cancel)
    document.addEventListener('visibilitychange', onHidden)
    document.addEventListener('mousemove', onMouseMove)
    document.addEventListener('mouseup', onMouseUp)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      window.removeEventListener('touchstart', onTouchStart)
      window.removeEventListener('touchmove', onTouchMove)
      window.removeEventListener('touchend', onTouchEnd)
      window.removeEventListener('touchcancel', onTouchCancel)
      window.removeEventListener('scroll', onScroll)
      window.removeEventListener('contextmenu', onContextMenu)
      window.removeEventListener('resize', onResize)
      window.removeEventListener('blur', cancel)
      document.removeEventListener('visibilitychange', onHidden)
      document.removeEventListener('mousemove', onMouseMove)
      document.removeEventListener('mouseup', onMouseUp)
      document.removeEventListener('keydown', onKeyDown)
      stopAutoScroll()
      clearPending()
      if (announceTimer !== null) clearTimeout(announceTimer)
      clearOpenedByMouseUp()
      apiRef.current = null
      dragRef.current = null
    }
  }, [])

  // 並びの文字列。並びが本当に変わったときだけ、下の effect を動かす（取り直しで並びが同じときに、
  // 余白を押して離れたカードへフォーカスを引き戻さないため。ドラッグも、値段だけが変わった取り直しでは取り消さない）。
  const orderKey = rendered.map((holding) => holding.code).join('\n')

  // カードの並び（銘柄コードの並び）が変わったら（取り直しで銘柄が増減した、保存の失敗で戻った）、その場で取り消す
  // （測ったマスが合わなくなるため）。値段だけが変わったときは、マスは <ul> からの相対でカードの高さも変わらないので続ける。
  useEffect(() => {
    apiRef.current?.cancel()
  }, [orderKey])

  // マウスでドラッグしている間は、ページの文字の選択を止める。
  const mouseDragging = drag?.mode === 'mouse'
  useEffect(() => {
    if (!mouseDragging) return
    document.body.classList.add(MOUSE_DRAGGING_BODY_CLASS)
    return () => document.body.classList.remove(MOUSE_DRAGGING_BODY_CLASS)
  }, [mouseDragging])

  // フォーカスのあったカードの銘柄コード。並びが変わり、フォーカスが body に落ちたときに、そのカードへ戻すため
  // （キーボードの利用者が、操作の続きをできるように。要件19・要件30）。
  const focusedCodeRef = useRef<string | null>(null)
  const rememberFocusedCard = (event: FocusEvent<HTMLUListElement>) => {
    const card = event.target instanceof Element ? event.target.closest('li') : null
    focusedCodeRef.current = card?.getAttribute(CODE_ATTRIBUTE) ?? null
  }
  // フォーカスがこの並びの外のほかの要素へ移ったなら、覚えたカードは忘れる（そこから勝手に引き戻さないため）。
  // relatedTarget が null（body に落ちた）のときは忘れない。
  // また、キーボードで持ち上げているカードからフォーカスが外れたら、取り消す。
  const handleBlur = (event: FocusEvent<HTMLUListElement>) => {
    if (event.relatedTarget instanceof Node && !event.currentTarget.contains(event.relatedTarget)) {
      focusedCodeRef.current = null
    }
    apiRef.current?.focusOut(event)
  }
  // 選んだ文字やリンクのブラウザのドラッグは止める。こちらのドラッグを待っていたなら、やめる。
  const handleDragStart = (event: DragEvent<HTMLUListElement>) => {
    event.preventDefault()
    apiRef.current?.dragStart()
  }
  useLayoutEffect(() => {
    const code = focusedCodeRef.current
    const list = listRef.current
    if (code === null || list === null) return
    // どこかの要素にフォーカスがあるなら、何もしない（DOM の順が変わってフォーカスが外れたとき、document.activeElement は body になる）。
    if (document.activeElement !== null && document.activeElement !== document.body) return
    const card = Array.from(list.children).find((child) => child.getAttribute(CODE_ATTRIBUTE) === code)
    if (card instanceof HTMLElement) card.focus({ preventScroll: true })
  }, [orderKey])

  // 先行表示の並び（要件17・要件22）。元のマス・置き場所なしのときは null（ほかのカードは動かさない）。
  const preview =
    drag !== null && drag.over !== null ? reorderCodes(drag.order, drag.code, drag.order[drag.over]) : null

  const cardProps = (code: string): CardProps => {
    const dragging = drag !== null && drag.code === code
    const following = dragging && drag.mode !== 'keyboard'
    const classes = [dragging ? 'holding-card--dragging' : null, following ? 'holding-card--following' : null]
      .filter((name) => name !== null)
      .join(' ')
    return {
      [CODE_ATTRIBUTE]: code,
      className: classes === '' ? undefined : classes,
      style: drag === null ? undefined : transformOf(drag, preview, code),
      onMouseDown: (event) => apiRef.current?.mouseDown(event, code),
      onTouchStart: (event) => apiRef.current?.touchStart(event, code),
      onKeyDown: (event) => apiRef.current?.keyDown(event, code),
      onClick: (event) => apiRef.current?.click(event, code),
    }
  }

  return {
    holdings: rendered,
    announcement,
    listProps: {
      ref: listRef,
      onFocus: rememberFocusedCard,
      onBlur: handleBlur,
      onDragStart: handleDragStart,
      'data-reordering': drag === null ? undefined : 'true',
    },
    cardProps,
  }
}

/**
 * カード code の transform。マウス・タッチで持ち上げたカードはポインターに付いていく。それ以外は、元のマスから、
 * 先行表示の並びでのマスへ動かす。動かさないなら undefined。
 */
function transformOf(drag: Drag, preview: readonly string[] | null, code: string): { transform: string } | undefined {
  if (code === drag.code && drag.mode !== 'keyboard') {
    return { transform: `translate(${drag.offset.x}px, ${drag.offset.y}px)` }
  }
  if (preview === null) return undefined
  const from = drag.order.indexOf(code)
  const to = preview.indexOf(code)
  if (from < 0 || to < 0 || from === to) return undefined
  const dx = drag.slots[to].left - drag.slots[from].left
  const dy = drag.slots[to].top - drag.slots[from].top
  return { transform: `translate(${dx}px, ${dy}px)` }
}

/** order の順に、各カードの矩形（<ul> の左上からの相対）と、カードの並びの範囲（左上が 0, 0）を測る。カードが見つからなければ null。 */
function measureSlots(list: HTMLElement, order: readonly string[]): { slots: Rect[]; bounds: Rect } | null {
  const listRect = list.getBoundingClientRect()
  const toRect = (element: Element): Rect => {
    const rect = element.getBoundingClientRect()
    return { left: rect.left - listRect.left, top: rect.top - listRect.top, width: rect.width, height: rect.height }
  }
  const byCode = new Map<string, Element>()
  for (const child of Array.from(list.children)) {
    const code = child.getAttribute(CODE_ATTRIBUTE)
    if (code !== null) byCode.set(code, child)
  }
  const slots: Rect[] = []
  for (const code of order) {
    const element = byCode.get(code)
    if (element === undefined) return null
    slots.push(toRect(element))
  }
  return { slots, bounds: { left: 0, top: 0, width: listRect.width, height: listRect.height } }
}

/** 置き場所のマス（<ul> からの相対）が画面の外にあるときだけ、入るようにスクロールする（滑らかにしない: 動かした直後にスペースで置いても、置き場所が決まっているように）。 */
function scrollIntoView(list: HTMLElement | null, slot: Rect): void {
  if (list === null) return
  const top = list.getBoundingClientRect().top + slot.top
  const bottom = top + slot.height
  if (top >= 0 && bottom <= window.innerHeight) return
  const target = top < 0 ? top - SCROLL_MARGIN_PX : bottom - window.innerHeight + SCROLL_MARGIN_PX
  window.scrollTo({ top: Math.max(0, window.scrollY + target), behavior: 'auto' })
}

/** touches・changedTouches から、identifier が id の指を探す。 */
function findTouch(touches: TouchList, id: number): Touch | undefined {
  for (let index = 0; index < touches.length; index += 1) {
    if (touches[index].identifier === id) return touches[index]
  }
  return undefined
}

/** 縦に scrollBy で動かして、動かした後の scrollY を返す。 */
function scrollWindowBy(delta: number): number {
  window.scrollBy({ top: delta, behavior: 'auto' })
  return window.scrollY
}

function isSpace(event: ReactKeyboardEvent<HTMLElement>): boolean {
  return event.code === 'Space' || event.key === ' '
}

function distance(a: Point, b: Point): number {
  return Math.hypot(a.x - b.x, a.y - b.y)
}
