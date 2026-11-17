import { z } from 'zod'
import type { StockHistory } from '../../../entities/stock-history/model'
import { CURRENCIES, UNIT_PRICE_PATTERNS } from '../../../shared/lib/currency'

/** 銘柄ごとの売買履歴の API のパス（銘柄コードを入れて使う）。開発・E2E では Vite の proxy が API サーバーへ転送する。 */
const HISTORY_API_PATH_PREFIX = '/api/holdings'
const HISTORY_API_PATH_SUFFIX = '/history'

/** 売買履歴がない（HTTP 404。要件20）。 */
const NOT_FOUND_STATUS = 404

const SAFE_INTEGER_MESSAGE = '安全に扱える整数ではありません'

/** 株数・分割後の株数。正の安全な整数（2^53 を超えると JavaScript の数では精度が落ちるため。一覧の保有数量と同じ）。 */
const positiveSafeIntegerSchema = z.number().positive().refine(Number.isSafeInteger, SAFE_INTEGER_MESSAGE)

/**
 * 割る数。整数の文字列（先頭が0でない正の整数）。分割を重ねると 2^53 を超えうるので、数にせず文字列で受ける。
 * Number にするのは図の座標の計算のときだけ（lib/chartModel.ts）。
 */
const splitDivisorSchema = z.string().regex(/^[1-9][0-9]*$/)

const dateSchema = z.iso.date()

const tradeSchema = z.object({
  kind: z.literal('trade'),
  date: dateSchema,
  side: z.enum(['buy', 'sell']),
  quantity: positiveSafeIntegerSchema,
  price: z.string(),
  amount: z.string(),
  splitDivisor: splitDivisorSchema,
  adjustedPrice: z.string(),
})

const splitSchema = z.object({
  kind: z.literal('split'),
  date: dateSchema,
  ratio: positiveSafeIntegerSchema,
})

const closeSchema = z.object({
  date: dateSchema,
  price: z.string(),
  splitDivisor: splitDivisorSchema,
  adjustedPrice: z.string(),
})

const stockHistorySchema = z
  .object({
    stock: z.object({ code: z.string(), name: z.string(), currency: z.enum(CURRENCIES) }),
    entries: z.array(z.discriminatedUnion('kind', [tradeSchema, splitSchema])),
    closes: z.array(closeSchema),
  })
  .superRefine((history, context) => {
    // 売買履歴が1件もないときは 404 になる（要件20）ので、found の応答には売買が1件以上ある。図の範囲（最初の売買の取引日）はこれを前提にする。
    if (!history.entries.some((entry) => entry.kind === 'trade')) {
      context.addIssue({ code: 'custom', message: 'entries に売買が1件もありません', path: ['entries'] })
    }
    // 金額の文字列の形は、銘柄の通貨に合わせて確かめる（円は小数1桁、ドルは小数2桁。一覧の単価と同じ形）。
    const pattern = UNIT_PRICE_PATTERNS[history.stock.currency]
    const check = (value: string, path: (string | number)[]) => {
      if (!pattern.test(value)) {
        context.addIssue({ code: 'custom', message: `${history.stock.currency} の ${path.join('.')} の形が正しくありません`, path })
      }
    }
    history.entries.forEach((entry, index) => {
      if (entry.kind !== 'trade') return
      check(entry.price, ['entries', index, 'price'])
      check(entry.amount, ['entries', index, 'amount'])
      check(entry.adjustedPrice, ['entries', index, 'adjustedPrice'])
    })
    history.closes.forEach((close, index) => {
      check(close.price, ['closes', index, 'price'])
      check(close.adjustedPrice, ['closes', index, 'adjustedPrice'])
    })
  })

/** 取得の結果。売買履歴がなければ（404）、エラーにせず notFound にする。 */
export type StockHistoryResult = { kind: 'found'; history: StockHistory } | { kind: 'notFound' }

/** 404 の本文を読んで、売買履歴がない（{"error":"not_found"}）ことを示すものかを返す。読めない本文は false。 */
async function isNotFoundBody(response: Response): Promise<boolean> {
  try {
    const body: unknown = await response.json()
    return z.object({ error: z.literal('not_found') }).safeParse(body).success
  } catch {
    return false
  }
}

/**
 * 銘柄 code の売買履歴を取得する（GET /api/holdings/{code}/history）。
 * 404 は、本文が {"error":"not_found"} のときだけ { kind: 'notFound' }（存在しない URL・proxy のエラーなど、別の 404 は失敗）。
 * 通信の失敗、それ以外の 2xx でない応答、形の正しくない応答のときは reject する（画面は取得失敗として扱う。要件23）。
 */
export async function fetchStockHistory(code: string): Promise<StockHistoryResult> {
  const response = await fetch(`${HISTORY_API_PATH_PREFIX}/${encodeURIComponent(code)}${HISTORY_API_PATH_SUFFIX}`)
  if (response.status === NOT_FOUND_STATUS) {
    if (await isNotFoundBody(response)) return { kind: 'notFound' }
    throw new Error('売買履歴の API が想定外の 404 を返しました')
  }
  if (!response.ok) {
    throw new Error(`売買履歴の API がエラーを返しました（HTTP ${response.status}）`)
  }
  const body: unknown = await response.json()
  return { kind: 'found', history: stockHistorySchema.parse(body) }
}
