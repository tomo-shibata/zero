import { z } from 'zod'

/** 見ていない通知の数の API のパス。開発・E2E では Vite の proxy が API サーバーへ転送する。 */
const UNSEEN_COUNT_API_PATH = '/api/notifications/unseen-count'

const unseenCountSchema = z.object({
  count: z
    .number()
    .int()
    .nonnegative()
    .refine(Number.isSafeInteger, '安全に扱える整数ではありません'),
})

/**
 * 見ていない通知の数を取得する（GET /api/notifications/unseen-count）。
 * 通信の失敗、2xx でない応答、形の正しくない応答のときは reject する（ベルはバッジを出さない。要件27）。
 */
export async function fetchUnseenNotificationCount(): Promise<number> {
  const response = await fetch(UNSEEN_COUNT_API_PATH)
  if (!response.ok) {
    throw new Error(`見ていない通知の数の API がエラーを返しました（HTTP ${response.status}）`)
  }
  const body: unknown = await response.json()
  return unseenCountSchema.parse(body).count
}
