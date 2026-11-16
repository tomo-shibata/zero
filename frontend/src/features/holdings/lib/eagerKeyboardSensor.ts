import { KeyboardCode, KeyboardSensor } from '@dnd-kit/core'

type Listeners = { add: (eventName: string, handler: (event: Event) => void) => void }

/** KeyboardSensor の中で使われる、型定義では private の部分（dnd-kit 6.3.1。下の attach が使う）。 */
type KeyboardSensorInternals = {
  props: { event: Event; options?: { keyboardCodes?: { end?: readonly string[] } } }
  listeners?: Listeners
  windowListeners?: Listeners
  handleStart?: () => void
  handleKeyDown?: (event: Event) => void
  handleCancel?: (event: Event) => void
}

/**
 * スペースキーで持ち上げた直後から、すぐにキーを受け付ける KeyboardSensor（要件19）。
 *
 * dnd-kit の KeyboardSensor は、持ち上げた直後に1回 setTimeout(0) してから、キー入力の見張りを始める
 * （持ち上げたスペースキーの keydown が、そのまま「置く」として受け取られないようにするため）。
 * 持ち上げの画面の更新は、その setTimeout より前に終わり、読み上げ（「持ち上げました」）が出る。
 * 更新が重いと、読み上げが出た直後に押した矢印キーが、見張りの始まる前に届いて捨てられる。
 * そこで、見張りをすぐに始め、持ち上げたキー自身のイベントだけを読み飛ばす。
 * キーを押し続けたときの連打（event.repeat）は、置く操作として扱わない（持ち上げたスペースキーを押し続けると、すぐに置いてしまうため）。
 * 取り消し・動かす先の決め方など、ほかの動きは KeyboardSensor のまま。
 *
 * dnd-kit の内部（listeners・windowListeners・handleStart など）に頼るため、@dnd-kit/core の版は固定している（package.json）。
 * 頼っている部品が見つからなければ、元の KeyboardSensor の attach を呼ぶ。
 */
export class EagerKeyboardSensor extends KeyboardSensor {}

// KeyboardSensor のコンストラクタが呼ぶ attach を、見張りをすぐに始めるものに置き換える（元の attach との違いは、setTimeout をやめたことと、
// 持ち上げたキー自身のイベント（props.event）を読み飛ばすことと、押し続けたキーで置かないことだけ）。
const originalAttach = (KeyboardSensor.prototype as unknown as { attach: (this: unknown) => void }).attach

;(EagerKeyboardSensor.prototype as unknown as { attach: (this: KeyboardSensorInternals) => void }).attach = function attach(
  this: KeyboardSensorInternals,
) {
  const { listeners, windowListeners, handleStart, handleKeyDown, handleCancel } = this
  if (
    listeners === undefined ||
    windowListeners === undefined ||
    typeof handleStart !== 'function' ||
    typeof handleKeyDown !== 'function' ||
    typeof handleCancel !== 'function'
  ) {
    // 頼っている内部の部品がない（dnd-kit の版が変わった）。元の動きに戻す。
    originalAttach.call(this)
    return
  }
  handleStart.call(this)
  windowListeners.add('resize', handleCancel)
  windowListeners.add('visibilitychange', handleCancel)
  const activation = this.props.event
  const endCodes: readonly string[] = this.props.options?.keyboardCodes?.end ?? [KeyboardCode.Space, KeyboardCode.Enter]
  listeners.add('keydown', (event) => {
    if (event === activation) return
    if (event instanceof KeyboardEvent && event.repeat && endCodes.includes(event.code)) return
    handleKeyDown.call(this, event)
  })
}
