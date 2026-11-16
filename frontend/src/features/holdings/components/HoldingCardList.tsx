import {
  closestCenter,
  DndContext,
  KeyboardCode,
  MouseSensor,
  TouchSensor,
  useSensor,
  useSensors,
} from '@dnd-kit/core'
import type {
  Announcements,
  CollisionDetection,
  DraggableSyntheticListeners,
  KeyboardCoordinateGetter,
} from '@dnd-kit/core'
import { rectSortingStrategy, SortableContext, sortableKeyboardCoordinates, useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { useCallback, useEffect, useId, useLayoutEffect, useMemo, useRef } from 'react'
import type { ComponentProps, FocusEvent, SyntheticEvent } from 'react'
import type { Holding } from '../../../entities/holding/model'
import {
  CANCELLED_ANNOUNCEMENT,
  droppedAnnouncement,
  movedAnnouncement,
  pickedUpAnnouncement,
  SCREEN_READER_INSTRUCTIONS,
} from '../lib/reorderAnnouncements'
import { EagerKeyboardSensor } from '../lib/eagerKeyboardSensor'
import { HoldingCard } from './HoldingCard'
import './HoldingCardList.css'

/** カードの並びの見出し（画面には出さず、支援技術が読む。リストの名前にもなる）。 */
const LIST_LABEL = '保有株式'

/** カードの役割の説明（aria-roledescription。要件19）。dnd-kit の既定の英語（sortable）の代わりに付ける。 */
const CARD_ROLE_DESCRIPTION = '並べ替えできるカード'

/** カード（<li>）に付ける、銘柄コードを持たせる属性（フォーカスのあったカードを覚えて、並びが変わった後に探すため）。 */
const CODE_ATTRIBUTE = 'data-holding-code'

/** マウスで、この距離（px）動かしたらドラッグを始める。クリックでドラッグを始めないため（判断9）。 */
const MOUSE_DRAG_DISTANCE_PX = 5
/** タッチで、押し続けたらドラッグを始める時間（ミリ秒）と、その間に動いてよい距離（px。超えたらスクロールとして扱う。要件18）。 */
const TOUCH_DRAG_DELAY_MS = 250
const TOUCH_DRAG_TOLERANCE_PX = 5

type HoldingCardListProps = {
  /** 表示するカード（渡された順に並べる。API の並び）。 */
  holdings: readonly Holding[]
  /** カードのメニューの「削除」が選ばれたときに呼ばれる。確認のダイアログを開くのは呼び出し側。 */
  onDelete: (holding: Holding) => void
  /**
   * カードを置いたときに呼ばれる。overCode は置いた位置にあるカードの銘柄コード。どのカードの上でもない
   * （カードの並びの外で離した）ときは null。並びを変えて保存するのは呼び出し側（このコンポーネントは保存しない）。
   */
  onMove: (activeCode: string, overCode: string | null) => void
}

/**
 * 保有株式のカードの並び（要件1）。1銘柄1枚のカードを、画面の幅に応じた列数のグリッドに詰めて並べ、
 * マウス・タッチ・キーボードで並べ替えられるようにする（要件17〜22。dnd-kit。カード表示のプラン 4.4「PR② 並べ替え」）。
 * 直前に、画面に出さない見出し h2「保有株式」を置き、リストの名前をその見出しから付ける
 * （見出しをたどる操作で、合計の欄 h2「合計」の次にカードの並びへ移れる）。
 * list-style: none だと Safari の VoiceOver がリストとして読まないことがあるので、role を明示する。
 */
export function HoldingCardList({ holdings, onDelete, onMove }: HoldingCardListProps) {
  const titleId = useId()
  const listRef = useRef<HTMLUListElement>(null)
  const codes = useMemo(() => holdings.map((holding) => holding.code), [holdings])
  // センサーの設定は、ドラッグの途中で作り直されても、始めたときの設定が使われ続ける。
  // そのため、動かす先を決める関数は、最新の並びを ref から読む。
  const codesRef = useRef(codes)
  useEffect(() => {
    codesRef.current = codes
  }, [codes])

  // フォーカスのあったカードの銘柄コード。保存の失敗などで並びが変わり、フォーカスが body などに落ちたときに、
  // そのカードへ戻すため（キーボードの利用者が、操作の続きをできるように）。
  const focusedCodeRef = useRef<string | null>(null)
  const rememberFocusedCard = (event: FocusEvent<HTMLUListElement>) => {
    const card = event.target instanceof Element ? event.target.closest('li') : null
    focusedCodeRef.current = card?.getAttribute(CODE_ATTRIBUTE) ?? null
  }
  // フォーカスがこの並びの外のほかの要素へ移ったなら、覚えたカードは忘れる（そこから勝手に引き戻さないため）。
  const forgetFocusedCard = (event: FocusEvent<HTMLUListElement>) => {
    if (event.relatedTarget instanceof Node && !event.currentTarget.contains(event.relatedTarget)) {
      focusedCodeRef.current = null
    }
  }
  // 並びの文字列。並びが本当に変わったときだけ、下の effect を動かす（今すぐ更新などで一覧を取り直しただけで並びが同じときに、
  // 余白を押して離れたカードへフォーカスを引き戻さないため。PR② のデザインの指摘8・エッジの指摘7）。
  const orderKey = codes.join('\n')
  useLayoutEffect(() => {
    const code = focusedCodeRef.current
    const list = listRef.current
    if (code === null || list === null) return
    // どこかの要素にフォーカスがあるなら、何もしない（落ちたとき、document.activeElement は body になる）。
    if (document.activeElement !== null && document.activeElement !== document.body) return
    const card = Array.from(list.children).find((child) => child.getAttribute(CODE_ATTRIBUTE) === code)
    if (card instanceof HTMLElement) card.focus({ preventScroll: true })
  }, [orderKey])

  const keyboardCoordinates = useMemo<KeyboardCoordinateGetter>(
    () => (event, args) => {
      // ↑ と ↓ は、同じ列の1行上・1行下（dnd-kit の位置での選び方）。
      if (event.code !== KeyboardCode.Left && event.code !== KeyboardCode.Right) {
        return sortableKeyboardCoordinates(event, args)
      }
      // ← と → は、今の並び（ドラッグ中の並べ替えの表示を含む）で1つ前・1つ後ろのカードへ。行をまたいでよい。
      // 位置で選ぶと、1列の表示では動かず、3列の行の端で次の行へ移れないため（判断9）。
      event.preventDefault()
      const current = codesRef.current.indexOf(String(args.context.over?.id ?? args.active))
      const target = codesRef.current[current + (event.code === KeyboardCode.Left ? -1 : 1)]
      if (current < 0 || target === undefined) return undefined
      const rect = args.context.droppableRects.get(target)
      // dnd-kit は返した座標を、持ち上げたカードの左上として扱う。変形を除いた矩形の左上を返す（sortableKeyboardCoordinates と同じ）。
      return rect ? { x: rect.left, y: rect.top } : undefined
    },
    [],
  )
  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: MOUSE_DRAG_DISTANCE_PX } }),
    useSensor(TouchSensor, {
      activationConstraint: { delay: TOUCH_DRAG_DELAY_MS, tolerance: TOUCH_DRAG_TOLERANCE_PX },
    }),
    useSensor(EagerKeyboardSensor, {
      // 始める・置くはスペースキーだけ（Enter では始めない）。取り消しは Esc と Tab（持ち上げ中に Tab でフォーカスだけが
      // カードから離れると、持ち上げたままほかの要素のキーを奪うため。判断9）。
      keyboardCodes: { start: [KeyboardCode.Space], cancel: [KeyboardCode.Esc, KeyboardCode.Tab], end: [KeyboardCode.Space] },
      coordinateGetter: keyboardCoordinates,
      // 画面の外へ動かすときのスクロールは、すぐに行う。滑らかなスクロール（既定）の間は、置き場所が決まらず、
      // 動かした直後にスペースキーで置くと、動かす前の位置に置いてしまうため。
      scrollBehavior: 'auto',
    }),
  )

  // カードの並びの外で離したら取り消す（要件22。判断10）。マウス・タッチ（pointerCoordinates がある）のときは、
  // ポインターがカードの並び（<ul>）の外にあれば「どのカードの上でもない」にする。キーボードは closestCenter のまま。
  const collisionDetection = useCallback<CollisionDetection>((args) => {
    const pointer = args.pointerCoordinates
    if (pointer) {
      const list = listRef.current?.getBoundingClientRect()
      const inside =
        list !== undefined &&
        pointer.x >= list.left &&
        pointer.x <= list.right &&
        pointer.y >= list.top &&
        pointer.y <= list.bottom
      if (!inside) return []
    }
    return closestCenter(args)
  }, [])

  // 持ち上げた直後に、dnd-kit が最初の置き場所（自分自身の位置）を知らせる onDragOver を1回呼ぶ。
  // それを「動かしました」と読み上げると、「持ち上げました」がすぐ上書きされて聞こえないので、最初の1回は読み上げない。
  const justPickedUpRef = useRef(false)
  const announcements = useMemo<Announcements>(() => {
    const nameOf = (code: string) => holdings.find((holding) => holding.code === code)?.name ?? ''
    const positionOf = (code: string) => codes.indexOf(code) + 1
    return {
      onDragStart: ({ active }) => {
        justPickedUpRef.current = true
        return pickedUpAnnouncement(String(active.id), nameOf(String(active.id)))
      },
      onDragOver: ({ over }) => {
        const initial = justPickedUpRef.current
        justPickedUpRef.current = false
        if (initial || !over) return undefined
        return movedAnnouncement(positionOf(String(over.id)))
      },
      onDragEnd: ({ active, over }) => {
        justPickedUpRef.current = false
        if (!over) return CANCELLED_ANNOUNCEMENT
        const code = String(active.id)
        return droppedAnnouncement(code, nameOf(code), positionOf(String(over.id)))
      },
      onDragCancel: () => {
        justPickedUpRef.current = false
        return CANCELLED_ANNOUNCEMENT
      },
    }
  }, [holdings, codes])

  return (
    <>
      <h2 id={titleId} className="visually-hidden">
        {LIST_LABEL}
      </h2>
      <DndContext
        sensors={sensors}
        collisionDetection={collisionDetection}
        accessibility={{ announcements, screenReaderInstructions: { draggable: SCREEN_READER_INSTRUCTIONS } }}
        onDragEnd={({ active, over }) => onMove(String(active.id), over ? String(over.id) : null)}
      >
        <SortableContext items={codes} strategy={rectSortingStrategy}>
          {/* aria-label は、名前の付け方が変わっても属性で探すテスト（E2E の基準31）が使えるように、aria-labelledby と同じ文言で残す。 */}
          <ul
            ref={listRef}
            role="list"
            aria-labelledby={titleId}
            aria-label={LIST_LABEL}
            className="holding-card-list"
            onFocus={rememberFocusedCard}
            onBlur={forgetFocusedCard}
          >
            {holdings.map((holding) => (
              <SortableHoldingCard key={holding.code} holding={holding} onDelete={onDelete} />
            ))}
          </ul>
        </SortableContext>
      </DndContext>
    </>
  )
}

type SortableHoldingCardProps = {
  holding: Holding
  onDelete: (holding: Holding) => void
}

/** 並べ替えられるカード1枚。dnd-kit の属性とイベントを <li> に付ける。 */
function SortableHoldingCard({ holding, onDelete }: SortableHoldingCardProps) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: holding.code })
  const liProps: ComponentProps<'li'> = {
    ref: setNodeRef,
    style: { transform: CSS.Transform.toString(transform), transition },
    className: isDragging ? 'holding-card--dragging' : undefined,
    // dnd-kit が付ける role="button"（と aria-pressed）は付けない。中に「⋯」のボタンがあり、ボタンの中にボタンを入れないため。
    tabIndex: attributes.tabIndex,
    'aria-label': `${holding.code} ${holding.name}`,
    'aria-roledescription': CARD_ROLE_DESCRIPTION,
    'aria-describedby': attributes['aria-describedby'],
    ...{ [CODE_ATTRIBUTE]: holding.code },
    ...guardListeners(listeners),
  }
  return <HoldingCard holding={holding} onDelete={onDelete} liProps={liProps} />
}

/**
 * dnd-kit のイベントの受け取りを、「⋯」では始めないよう包む（要件21）。イベントの伝わりは止めない
 * （止めると、document で Esc を見張っている MenuButton のメニューが閉じなくなる）。
 * - keydown: イベントの対象がカード自身のときだけ渡す（「⋯」やメニューの中のキーでは始めない）。
 * - mousedown・touchstart: 「⋯」を包む要素（data-no-drag）の中で起きたものは渡さない。
 */
function guardListeners(listeners: DraggableSyntheticListeners): Record<string, (event: SyntheticEvent) => void> {
  const guarded: Record<string, (event: SyntheticEvent) => void> = {}
  if (!listeners) return guarded
  const isOnCard = (event: SyntheticEvent) => event.target === event.currentTarget
  const isOutsideNoDrag = (event: SyntheticEvent) =>
    !(event.target instanceof Element && event.target.closest('[data-no-drag]'))
  const allow: Record<string, (event: SyntheticEvent) => boolean> = {
    onKeyDown: isOnCard,
    onMouseDown: isOutsideNoDrag,
    onTouchStart: isOutsideNoDrag,
  }
  for (const [name, listener] of Object.entries(listeners)) {
    const shouldPass = allow[name]
    guarded[name] = (event) => {
      if (shouldPass === undefined || shouldPass(event)) (listener as (event: SyntheticEvent) => void)(event)
    }
  }
  return guarded
}
