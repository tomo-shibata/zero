import type { ComponentPropsWithRef } from 'react'
import './Button.css'

type ButtonVariant = 'primary' | 'secondary'

// ref も渡せるようにする（React 19 では、関数コンポーネントが ref を props として受け取れる）。
type ButtonProps = ComponentPropsWithRef<'button'> & {
  variant?: ButtonVariant
}

/**
 * 汎用ボタン。見た目と状態（hover/active/focus/disabled）を担当する。
 */
export function Button({
  variant = 'primary',
  className,
  ...props
}: ButtonProps) {
  const classes = ['button', `button--${variant}`, className]
    .filter(Boolean)
    .join(' ')
  return <button {...props} className={classes} />
}
