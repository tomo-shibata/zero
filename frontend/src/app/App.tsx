import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { LoginPage } from '../features/auth'
import { DashboardPage } from '../features/dashboard'
import { HoldingsPage } from '../features/holdings'
import { NotificationBell, NotificationsPage } from '../features/notifications'
import { StockHistoryPage } from '../features/stock-history'
import { HOLDINGS_PATH, NOTIFICATIONS_PATH } from '../shared/lib/routes'

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
          {/* features/holdings と features/notifications は互いを import しないので、ベルは app 層が見出しの行に差し込む。 */}
          <Route path={HOLDINGS_PATH} element={<HoldingsPage headerActions={<NotificationBell />} />} />
          <Route path={`${HOLDINGS_PATH}/:code`} element={<StockHistoryPage />} />
          <Route path={NOTIFICATIONS_PATH} element={<NotificationsPage />} />
          <Route path="*" element={<Navigate to="/login" replace />} />
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  )
}
