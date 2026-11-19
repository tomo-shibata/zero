import type { Currency } from '../../shared/lib/currency'
import { formatUnitPrice, formatValuation } from '../../shared/lib/format'
import type { AlertConditionSummary, AlertDirection } from './model'

// 条件の文（価格の変動の通知の要件31・要件41）。通知のリストと、条件を決める画面の両方が使うので entities に置く。

/** % の値の文字列の形（小数第1位まで。例 "20.0"）。 */
const PERCENT_PATTERN = /^([0-9]+)\.([0-9])$/

const ALL_SCOPE_LABEL = '全銘柄'
const LABEL_SEPARATOR = '・'
const THRESHOLD_SUFFIX = '以上'

/** 向きごとの、% の種類の条件の文の後半（要件41）。 */
const PERCENT_TEXTS = {
  cost: { prefix: '取得価格から', directions: { up: '上がったら', down: '下がったら' } },
  peak: { prefix: '最高値から', directions: { down: '下がったら' } },
  prev: {
    prefix: '前の取引日から',
    directions: { up: '上がったら', down: '下がったら', either: '動いたら（上がっても下がっても）' },
  },
} as const

/**
 * % の値を、小数第1位が0なら整数にして「%」を付ける（要件41。例 "20.0" → "20%"、"2.5" → "2.5%"）。
 * 形が合わなければ throw する（画面の値は API の境界で検証済みなので、描画中には届かない。format.ts と同じ）。
 */
export function formatThresholdPercent(threshold: string): string {
  const match = PERCENT_PATTERN.exec(threshold)
  if (!match) throw new Error('% の値の形が正しくありません')
  const [, integer, fraction] = match
  return fraction === '0' ? `${integer}%` : `${integer}.${fraction}%`
}

/** 金額と価格の種類で、通貨が要る。ないときは throw する（呼ぶ側の誤り）。 */
function requireCurrency(currency: Currency | null): Currency {
  if (currency === null) throw new Error('金額と価格の条件には通貨が要ります')
  return currency
}

/** 向きが、その種類で使えないときの throw（API の境界で検証済みなので、描画中には届かない）。 */
function unsupportedDirection(): never {
  throw new Error('条件の向きが種類に合いません')
}

/**
 * 条件の文（要件41）。例「取得価格から 20% 以上 下がったら」「含み損が 100,000円 以上になったら」「3,500円 以上になったら」。
 * currency は、金額と価格の種類（含み損益の金額・決めた価格）だけで使う（ほかの種類は null でよい）。
 */
export function conditionText(c: AlertConditionSummary, currency: Currency | null): string {
  switch (c.kind) {
    case 'cost':
    case 'peak':
    case 'prev': {
      const { prefix, directions } = PERCENT_TEXTS[c.kind]
      const directionText = (directions as Partial<Record<AlertDirection, string>>)[c.direction]
      if (directionText === undefined) return unsupportedDirection()
      return `${prefix} ${formatThresholdPercent(c.threshold)} ${THRESHOLD_SUFFIX} ${directionText}`
    }
    case 'pnl': {
      if (c.direction !== 'loss' && c.direction !== 'gain') return unsupportedDirection()
      const amount = formatValuation(requireCurrency(currency), c.threshold)
      return `${c.direction === 'loss' ? '含み損' : '含み益'}が ${amount} ${THRESHOLD_SUFFIX}になったら`
    }
    case 'price': {
      if (c.direction !== 'above' && c.direction !== 'below') return unsupportedDirection()
      const price = formatUnitPrice(requireCurrency(currency), c.threshold)
      return `${price} ${c.direction === 'above' ? THRESHOLD_SUFFIX : '以下'}になったら`
    }
  }
}

/**
 * 「<対象>・<条件の文>」（要件31）。対象は、全銘柄なら「全銘柄」、1つの銘柄ならその銘柄コード。
 * 例「全銘柄・取得価格から 20% 以上 下がったら」「9984・8,000円 以上になったら」。
 */
export function conditionLabel(c: AlertConditionSummary, currency: Currency | null): string {
  const target = c.scope === 'all' || c.stockCode === null ? ALL_SCOPE_LABEL : c.stockCode
  return `${target}${LABEL_SEPARATOR}${conditionText(c, currency)}`
}
