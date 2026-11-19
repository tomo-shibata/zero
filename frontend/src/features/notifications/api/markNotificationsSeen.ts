/** 通知を見たにする API のパス。開発・E2E では Vite の proxy が API サーバーへ転送する。 */
const SEEN_API_PATH = '/api/notifications/seen'

/** 見たにするのが成功したときの HTTP のステータス（本文なし）。 */
const NO_CONTENT = 204

/**
 * 通知を、ID が upToId 以下のものまで見たにする（POST /api/notifications/seen、本文 {"upToId": upToId}。要件34）。
 * 204 以外の応答と通信の失敗のときは reject する（画面は失敗を黙って捨てる）。
 */
export async function markNotificationsSeen(upToId: string): Promise<void> {
  const response = await fetch(SEEN_API_PATH, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ upToId }),
  })
  if (response.status !== NO_CONTENT) {
    throw new Error(`通知を見たにする API がエラーを返しました（HTTP ${response.status}）`)
  }
}
