import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { LoginPage } from '../features/auth'
import { DashboardPage } from '../features/dashboard'
import { HoldingsPage } from '../features/holdings'

/** サーバー状態（API から取得するデータ）のキャッシュ。アプリ全体で1つ。取得の設定は各機能のフックで決める。 */
const queryClient = new QueryClient()

/**
 * アプリ全体のルーティングとプロバイダを定義する。
 */
export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/dashboard" element={<DashboardPage />} />
          <Route path="/holdings" element={<HoldingsPage />} />
          <Route path="*" element={<Navigate to="/login" replace />} />
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  )
}
