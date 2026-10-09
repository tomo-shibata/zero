import type { FormEvent } from 'react'
import { Button } from '../../../shared/ui/Button'

type LoginFormProps = {
  onSubmit: () => void
}

/**
 * ログインフォームの見た目のみを担当する（プレゼンテーション）。
 * 入力値は受け取らない（デモのため認証処理なし）。
 */
export function LoginForm({ onSubmit }: LoginFormProps) {
  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    onSubmit()
  }

  return (
    <form onSubmit={handleSubmit} style={{ display: 'grid', gap: '1rem' }}>
      <label style={{ display: 'grid', gap: '0.25rem' }}>
        <span style={{ fontSize: '0.875rem' }}>メールアドレス</span>
        <input
          type="email"
          name="email"
          autoComplete="email"
          required
          placeholder="you@example.com"
          style={inputStyle}
        />
      </label>
      <label style={{ display: 'grid', gap: '0.25rem' }}>
        <span style={{ fontSize: '0.875rem' }}>パスワード</span>
        <input
          type="password"
          name="password"
          autoComplete="current-password"
          required
          placeholder="••••••••"
          style={inputStyle}
        />
      </label>
      <Button type="submit">ログイン</Button>
    </form>
  )
}

const inputStyle = {
  padding: '0.625rem 0.75rem',
  fontSize: '1rem',
  border: '1px solid #d1d5db',
  borderRadius: '0.5rem',
} as const
