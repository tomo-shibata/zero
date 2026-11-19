import { z } from 'zod'
import {
  ALERT_DIRECTIONS,
  ALERT_DIRECTIONS_BY_KIND,
  ALERT_KINDS,
  ALERT_SCOPES,
  type PriceAlertNotificationList,
} from '../../../entities/price-alert/model'
import {
  CURRENCIES,
  UNIT_PRICE_PATTERNS,
  UNREALIZED_GAIN_PATTERNS,
  UNREALIZED_GAIN_RATE_PATTERN,
  VALUATION_PATTERNS,
} from '../../../shared/lib/currency'

/** 通知の API のパス。開発・E2E では Vite の proxy が API サーバーへ転送する。 */
const NOTIFICATIONS_API_PATH = '/api/notifications'

const SAFE_INTEGER_MESSAGE = '安全に扱える整数ではありません'

/** id・latestId。正の整数の文字列（数にすると 2^53 を超えうるので文字列のまま持つ）。 */
const idSchema = z.string().regex(/^[1-9][0-9]*$/)

/** 保有数量。正の安全な整数（一覧の保有数量と同じ）。 */
const quantitySchema = z.number().positive().refine(Number.isSafeInteger, SAFE_INTEGER_MESSAGE)

/** % の条件の値の形（小数第1位まで。例 "20.0"）。 */
const PERCENT_THRESHOLD_PATTERN = /^[0-9]+\.[0-9]$/

const notificationSchema = z.object({
  id: idSchema,
  stockCode: z.string(),
  stockName: z.string(),
  currency: z.enum(CURRENCIES),
  tradingDate: z.iso.date(),
  isNew: z.boolean(),
  condition: z.object({
    kind: z.enum(ALERT_KINDS),
    scope: z.enum(ALERT_SCOPES),
    stockCode: z.string().nullable(),
    direction: z.enum(ALERT_DIRECTIONS),
    threshold: z.string(),
  }),
  close: z.string(),
  rate: z.string().nullable(),
  basePrice: z.string().nullable(),
  baseDate: z.iso.date().nullable(),
  unrealizedGain: z.string().nullable(),
  quantity: quantitySchema.nullable(),
  adjustedThreshold: z.string().nullable(),
})

/**
 * 通知1件の、値の形と、種類ごとに要る値の有無を確かめる。
 * 文を作る関数（lib/notificationTexts・entities の conditionText）は、形の合わない値で throw するので、描画に届く前にここで断る
 * （shared/lib/format.ts の先頭のコメントと同じ考え方）。
 */
const checkedNotificationSchema = notificationSchema.superRefine((n, context) => {
  const issue = (path: (string | number)[], message: string) => context.addIssue({ code: 'custom', message, path })
  const unitPrice = UNIT_PRICE_PATTERNS[n.currency]
  if (!unitPrice.test(n.close)) issue(['close'], '終値の形が正しくありません')
  if (n.basePrice !== null && !unitPrice.test(n.basePrice)) issue(['basePrice'], '基準の価格の形が正しくありません')
  if (n.adjustedThreshold !== null && !unitPrice.test(n.adjustedThreshold)) {
    issue(['adjustedThreshold'], '分割で割った値の形が正しくありません')
  }
  if (n.rate !== null && !UNREALIZED_GAIN_RATE_PATTERN.test(n.rate)) issue(['rate'], '率の形が正しくありません')
  if (n.unrealizedGain !== null && !UNREALIZED_GAIN_PATTERNS[n.currency].test(n.unrealizedGain)) {
    issue(['unrealizedGain'], '含み損益の形が正しくありません')
  }

  const { kind, direction, threshold } = n.condition
  if (!ALERT_DIRECTIONS_BY_KIND[kind].includes(direction)) {
    issue(['condition', 'direction'], '条件の向きが種類に合いません')
  }
  const thresholdPattern =
    kind === 'pnl' ? VALUATION_PATTERNS[n.currency] : kind === 'price' ? unitPrice : PERCENT_THRESHOLD_PATTERN
  if (!thresholdPattern.test(threshold)) issue(['condition', 'threshold'], '条件の値の形が正しくありません')

  // 種類ごとに要る値（要件32 の表）。
  const percentKind = kind === 'cost' || kind === 'peak' || kind === 'prev'
  if (percentKind && n.rate === null) issue(['rate'], '率がありません')
  if ((percentKind || kind === 'pnl') && n.basePrice === null) issue(['basePrice'], '基準の価格がありません')
  if ((kind === 'peak' || kind === 'prev') && n.baseDate === null) issue(['baseDate'], '基準の日がありません')
  if (kind === 'pnl' && (n.unrealizedGain === null || n.quantity === null)) {
    issue(['unrealizedGain'], '含み損益か保有数量がありません')
  }
})

const notificationListSchema = z.object({
  items: z.array(checkedNotificationSchema),
  latestId: idSchema.nullable(),
})

/**
 * 通知の一覧を取得する（GET /api/notifications）。
 * 通信の失敗、2xx でない応答、形の正しくない応答のときは reject する（画面は取得失敗として扱う。要件37）。
 */
export async function fetchNotifications(): Promise<PriceAlertNotificationList> {
  const response = await fetch(NOTIFICATIONS_API_PATH)
  if (!response.ok) {
    throw new Error(`通知の API がエラーを返しました（HTTP ${response.status}）`)
  }
  const body: unknown = await response.json()
  return notificationListSchema.parse(body)
}
