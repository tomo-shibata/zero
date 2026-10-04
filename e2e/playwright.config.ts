import { defineConfig, devices } from '@playwright/test'

const PORT = 5178
const BASE_URL = `http://localhost:${PORT}`

export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  reporter: 'list',
  use: {
    baseURL: BASE_URL,
    trace: 'on-first-retry',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  // フロントエンドの dev サーバーを起動（既に起動していれば再利用する）
  webServer: {
    command: 'npm --prefix ../frontend run dev -- --port ' + PORT,
    url: BASE_URL,
    reuseExistingServer: true,
    timeout: 60_000,
  },
})
