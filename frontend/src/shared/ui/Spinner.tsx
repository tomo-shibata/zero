import './Spinner.css'

type SpinnerProps = {
  /** 支援技術に伝えるラベル（例「読み込み中」）。 */
  label: string
}

/**
 * ローディングのアイコン。見た目は回転する円で、支援技術には role="status" と label で伝える。
 */
export function Spinner({ label }: SpinnerProps) {
  return <div role="status" aria-label={label} className="spinner" />
}
