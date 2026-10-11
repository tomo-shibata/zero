import { defineConfig, devices } from '@playwright/test'

/** フロントエンドの dev サーバーのポート。 */
const FRONTEND_PORT = 5178
const BASE_URL = `http://localhost:${FRONTEND_PORT}`

/**
 * E2E 用の API サーバー（task e2e:api）の待ち受け。開発用の API（8080）とは別のポートと DB（zero_e2e）を使う（プラン 4.5）。
 * ホストを localhost にしないのは、Windows で先に IPv6 の ::1 に解決され、接続が遅れたり別のサーバーにつながったりするため。
 */
const API_URL = 'http://127.0.0.1:8081'

/** サーバーの起動を待つ上限。API は go run のビルドと DB の作り直しを含むので、長めにとる。 */
const WEB_SERVER_TIMEOUT_MS = 120_000

export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  // 1つのワーカーで順に実行する。テストは同じ DB（zero_e2e）を使い、AC-18 の計測中に他のテストを同時に動かさないため（プラン 4.5）。
  workers: 1,
  reporter: 'list',
  use: {
    baseURL: BASE_URL,
    // 再試行しない設定（retries は既定の 0）でも、失敗したテストの trace を残す（原因を後から調べられるように。プラン 4.5、PR④ ステップ6 の EC-7）。
    trace: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  // どちらも起動済みのサーバーを使い回さない（古いサーバーや別の DB を向いたサーバーでテストしないため。プラン 4.5）。
  webServer: [
    {
      // zero_e2e を作り直してから（migrate -recreate -seed db/seed/test.sql）、利用者A として API を起動する。
      command: 'task e2e:api',
      url: `${API_URL}/healthz`,
      reuseExistingServer: false,
      timeout: WEB_SERVER_TIMEOUT_MS,
    },
    {
      // --strictPort: ポートが使用中なら、別のポートに移らずに起動を失敗させる
      // （移ると、BASE_URL のポートで動いている別のサーバーに対してテストしてしまうため。プラン 4.5、PR④ ステップ6 の EC-6）。
      command: `npm --prefix ../frontend run dev -- --port ${FRONTEND_PORT} --strictPort`,
      url: BASE_URL,
      reuseExistingServer: false,
      timeout: WEB_SERVER_TIMEOUT_MS,
      // dev サーバーの /api を E2E 用の API に転送する。
      env: { API_PROXY_TARGET: API_URL },
    },
  ],
})
