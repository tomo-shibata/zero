import { z } from 'zod'
import type { HoldingsList } from '../../../entities/holding/model'
import { UNIT_PRICE_PATTERN } from '../../../shared/lib/format'

/** 保有株式一覧の API（プラン 4.4「API」）。開発・E2E では Vite の proxy が API サーバーへ転送する。 */
const HOLDINGS_API_PATH = '/api/holdings'

/**
 * JSON の数のうち、整数として扱うもの（保有数量・評価額・合計評価額）は、Number.isSafeInteger で確かめる。
 * 2^53 を超えると JavaScript の数では精度が落ちるため（プラン 6章の判断16）。
 */
const SAFE_INTEGER_MESSAGE = '安全に扱える整数ではありません'

/** 保有数量。正の安全な整数（DB の CHECK で数量は1以上。プラン 4.4、PR④ ステップ6 の EC-3）。 */
const quantitySchema = z.number().positive().refine(Number.isSafeInteger, SAFE_INTEGER_MESSAGE)

/** 評価額・合計評価額（円）。0以上の安全な整数（すべての行に価格がなければ合計は 0。要件 FR-6、PR④ ステップ6 の EC-3）。 */
const yenAmountSchema = z.number().nonnegative().refine(Number.isSafeInteger, SAFE_INTEGER_MESSAGE)

/** 単価（取得価格・現在の価格）。小数第1位を1桁付けた文字列（例 "2500.0"）。 */
const unitPriceSchema = z.string().regex(UNIT_PRICE_PATTERN)

const holdingBaseSchema = z.object({
  code: z.string(),
  name: z.string(),
  quantity: quantitySchema,
  acquisitionPrice: unitPriceSchema,
})

/** 価格のある行。 */
const pricedHoldingSchema = holdingBaseSchema.extend({
  currentPrice: unitPriceSchema,
  priceDate: z.iso.date(),
  valuation: yenAmountSchema,
})

/** 価格のない行。現在の価格・価格の基準日・評価額がそろって null（プラン 4.4「API」）。 */
const unpricedHoldingSchema = holdingBaseSchema.extend({
  currentPrice: z.null(),
  priceDate: z.null(),
  valuation: z.null(),
})

const holdingsListSchema = z.object({
  holdings: z.array(z.union([pricedHoldingSchema, unpricedHoldingSchema])),
  totalValuation: yenAmountSchema,
  totalExcludesUnpriced: z.boolean(),
})

/**
 * 利用者の保有株式一覧を取得する。
 * 通信の失敗、2xx 以外の応答、形の正しくない応答のときは reject する（画面は取得失敗として扱う。要件 FR-22）。
 */
export async function fetchHoldings(): Promise<HoldingsList> {
  const response = await fetch(HOLDINGS_API_PATH)
  if (!response.ok) {
    throw new Error(`保有株式一覧の API がエラーを返しました（HTTP ${response.status}）`)
  }
  const body: unknown = await response.json()
  return holdingsListSchema.parse(body)
}
