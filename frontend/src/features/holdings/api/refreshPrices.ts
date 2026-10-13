import { z } from 'zod'
import type { PriceRefreshResult } from '../../../entities/holding/model'

/** 価格の今すぐ更新の API（値段の更新のプラン 4.4「API」）。開発・E2E では Vite の proxy が API サーバーへ転送する。 */
const PRICE_REFRESH_API_PATH = '/api/price-refresh'

const priceRefreshResultSchema = z.object({
  saved: z.array(z.string()),
  failed: z.array(z.string()),
})

/**
 * ログイン中の利用者の保有銘柄の価格を、今すぐ更新する（要件16）。本文はなし。
 * 通信の失敗、2xx 以外の応答、形の正しくない応答のときは reject する（画面は更新の失敗として扱う。要件19）。
 */
export async function refreshPrices(): Promise<PriceRefreshResult> {
  const response = await fetch(PRICE_REFRESH_API_PATH, { method: 'POST' })
  if (!response.ok) {
    throw new Error(`価格の更新の API がエラーを返しました（HTTP ${response.status}）`)
  }
  const body: unknown = await response.json()
  return priceRefreshResultSchema.parse(body)
}
