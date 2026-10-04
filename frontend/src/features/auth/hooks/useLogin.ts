import { useCallback } from 'react'
import { useNavigate } from 'react-router-dom'

/**
 * ログイン操作のロジックを担うフック。
 * ※ 認証処理は未実装（デモ）。ログイン後はダッシュボードへ遷移するだけ。
 */
export function useLogin() {
  const navigate = useNavigate()

  const login = useCallback(() => {
    // TODO: 実際の認証処理（APIコール）はここに実装する。
    navigate('/dashboard')
  }, [navigate])

  return { login }
}
