import { useLogin } from '../hooks/useLogin'
import { LoginForm } from './LoginForm'

/**
 * ログイン画面。ロジック（useLogin）と見た目（LoginForm）をつなぐコンテナ。
 */
export function LoginPage() {
  const { login } = useLogin()

  return (
    <main
      style={{
        minHeight: '100vh',
        display: 'grid',
        placeItems: 'center',
        padding: '1.5rem',
      }}
    >
      <div
        style={{
          width: '100%',
          maxWidth: '360px',
          padding: '2rem',
          background: '#ffffff',
          borderRadius: '1rem',
          boxShadow: '0 10px 30px rgba(17, 24, 39, 0.08)',
        }}
      >
        <h1 style={{ margin: '0 0 1.5rem', fontSize: '1.5rem' }}>ログイン</h1>
        <LoginForm onSubmit={login} />
      </div>
    </main>
  )
}
