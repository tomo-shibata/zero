import { z } from 'zod'
import type { HoldingsList } from '../../../entities/holding/model'
import {
  CURRENCIES,
  type Currency,
  UNIT_PRICE_PATTERNS,
  UNREALIZED_GAIN_PATTERNS,
  UNREALIZED_GAIN_RATE_PATTERN,
  VALUATION_PATTERNS,
} from '../../../shared/lib/currency'

/** 保有株式一覧の API（売買履歴のプラン 4.3「一覧の API の形」）。開発・E2E では Vite の proxy が API サーバーへ転送する。 */
const HOLDINGS_API_PATH = '/api/holdings'

/**
 * JSON の数のうち、整数として扱うもの（保有数量）は、Number.isSafeInteger で確かめる。
 * 2^53 を超えると JavaScript の数では精度が落ちるため（保有株式一覧のプラン 6章の判断16）。
 * 金額（単価・評価額・合計評価額）は、円もドルも文字列で受け取る（売買履歴のプラン 6章の判断7）ので、この問題はない。
 */
const SAFE_INTEGER_MESSAGE = '安全に扱える整数ではありません'

/** 保有数量。正の安全な整数（DB の CHECK で数量は1以上。PR④ ステップ6 の EC-3）。 */
const quantitySchema = z.number().positive().refine(Number.isSafeInteger, SAFE_INTEGER_MESSAGE)

/** 通貨。円（JPY）かドル（USD）だけ。知らない通貨・小文字は reject する。 */
const currencySchema = z.enum(CURRENCIES)

/**
 * 金額の文字列が、通貨に合う形かを確かめる。外れたら、path の場所に誤りを足す。
 * 円の単価に小数2桁、ドルの単価に小数1桁などは reject（通貨ごとの形は UNIT_PRICE_PATTERNS・VALUATION_PATTERNS）。
 */
function checkAmountFormat(
  context: z.RefinementCtx,
  patterns: Record<Currency, RegExp>,
  currency: Currency,
  amount: string,
  path: string,
): void {
  if (!patterns[currency].test(amount)) {
    context.addIssue({
      code: 'custom',
      message: `${currency} の ${path} の形が正しくありません`,
      path: [path],
    })
  }
}

/** 含み損益と率の文字列が、通貨に合う形かを確かめる（含み損益は UNREALIZED_GAIN_PATTERNS、率は UNREALIZED_GAIN_RATE_PATTERN）。 */
function checkGainFormat(context: z.RefinementCtx, currency: Currency, gain: string, rate: string): void {
  checkAmountFormat(context, UNREALIZED_GAIN_PATTERNS, currency, gain, 'unrealizedGain')
  if (!UNREALIZED_GAIN_RATE_PATTERN.test(rate)) {
    context.addIssue({
      code: 'custom',
      message: 'unrealizedGainRate の形が正しくありません',
      path: ['unrealizedGainRate'],
    })
  }
}

const holdingBaseSchema = z.object({
  code: z.string(),
  name: z.string(),
  currency: currencySchema,
  quantity: quantitySchema,
  acquisitionPrice: z.string(),
})

/** 価格のある行。単価と評価額の形は、行の通貨に合わせて確かめる。 */
const pricedHoldingSchema = holdingBaseSchema
  .extend({
    currentPrice: z.string(),
    priceDate: z.iso.date(),
    valuation: z.string(),
    unrealizedGain: z.string(),
    unrealizedGainRate: z.string(),
  })
  .superRefine((row, context) => {
    checkAmountFormat(context, UNIT_PRICE_PATTERNS, row.currency, row.acquisitionPrice, 'acquisitionPrice')
    checkAmountFormat(context, UNIT_PRICE_PATTERNS, row.currency, row.currentPrice, 'currentPrice')
    checkAmountFormat(context, VALUATION_PATTERNS, row.currency, row.valuation, 'valuation')
    checkGainFormat(context, row.currency, row.unrealizedGain, row.unrealizedGainRate)
  })

/** 価格のない行。現在の価格・価格の基準日・評価額・含み損益・率がそろって null。 */
const unpricedHoldingSchema = holdingBaseSchema
  .extend({
    currentPrice: z.null(),
    priceDate: z.null(),
    valuation: z.null(),
    unrealizedGain: z.null(),
    unrealizedGainRate: z.null(),
  })
  .superRefine((row, context) => {
    checkAmountFormat(context, UNIT_PRICE_PATTERNS, row.currency, row.acquisitionPrice, 'acquisitionPrice')
  })

/**
 * 通貨ごとの合計評価額と含み損益の合計。valuation の形は、その通貨の評価額と同じ。
 * 含み損益と率は、両方とも null（その通貨に価格のある銘柄がない）か、両方とも文字列（その通貨の含み損益の形）。片方だけ null は reject する。
 */
const currencyTotalSchema = z
  .object({
    currency: currencySchema,
    valuation: z.string(),
    excludesUnpriced: z.boolean(),
    unrealizedGain: z.string().nullable(),
    unrealizedGainRate: z.string().nullable(),
  })
  .superRefine((total, context) => {
    checkAmountFormat(context, VALUATION_PATTERNS, total.currency, total.valuation, 'valuation')
    const { unrealizedGain: gain, unrealizedGainRate: rate } = total
    if (gain !== null && rate !== null) {
      checkGainFormat(context, total.currency, gain, rate)
    } else if (gain !== null || rate !== null) {
      context.addIssue({
        code: 'custom',
        message: 'unrealizedGain と unrealizedGainRate は、両方とも null か両方とも文字列でなければなりません',
        path: ['unrealizedGainRate'],
      })
    }
  })

const holdingsListSchema = z.object({
  holdings: z.array(z.union([pricedHoldingSchema, unpricedHoldingSchema])),
  totals: z.array(currencyTotalSchema),
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
