import type { TotalsView } from '../lib/totalsViewStorage'
import './TotalsViewSwitch.css'

const SWITCH_LABEL = '合計の表示'

/** ボタンの並びと文言（通貨ごと、円にまとめる）。 */
const OPTIONS: readonly { view: TotalsView; label: string }[] = [
  { view: 'byCurrency', label: '通貨ごと' },
  { view: 'combinedYen', label: '円にまとめる' },
]

type TotalsViewSwitchProps = {
  /** 今選んでいる表示。 */
  view: TotalsView
  /** ボタンを押したときに、押された表示で呼ばれる。 */
  onChange: (view: TotalsView) => void
}

/**
 * 合計の欄の表示の切り替え（要件1）。「通貨ごと」「円にまとめる」の2つのボタンを1組にし、
 * 選んでいる方は塗りつぶした色で示し、支援技術には押されている状態（aria-pressed）として伝える。
 */
export function TotalsViewSwitch({ view, onChange }: TotalsViewSwitchProps) {
  return (
    <div role="group" aria-label={SWITCH_LABEL} className="totals-view-switch">
      {OPTIONS.map((option) => (
        <button
          key={option.view}
          type="button"
          aria-pressed={view === option.view}
          className="totals-view-switch__option"
          onClick={() => onChange(option.view)}
        >
          {option.label}
        </button>
      ))}
    </div>
  )
}
