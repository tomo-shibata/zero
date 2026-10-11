import { Link, useNavigate } from 'react-router-dom'
import { Button } from '../../../shared/ui/Button'

/**
 * ログイン後に表示するダッシュボード画面（デモ用の適当なページ）。
 */
export function DashboardPage() {
  const navigate = useNavigate()

  return (
    <main style={{ maxWidth: '640px', margin: '0 auto', padding: '2rem 1.5rem' }}>
      <h1 style={{ fontSize: '1.75rem' }}>ダッシュボード</h1>
      <p>ログインに成功しました。ここはログイン後に表示される適当なページです。</p>
      <p>
        <Link to="/holdings" style={linkStyle}>
          保有株式一覧
        </Link>
      </p>
      <div style={{ maxWidth: '200px', marginTop: '1.5rem' }}>
        <Button
          type="button"
          variant="secondary"
          onClick={() => navigate('/login')}
        >
          ログアウト
        </Button>
      </div>
    </main>
  )
}

const linkStyle = {
  color: 'var(--color-primary)',
  fontWeight: 600,
} as const
