import type { PriceAlertNotification } from '../../../entities/price-alert/model'
import type { Currency } from '../../../shared/lib/currency'
import { formatShares, formatUnitPrice, formatValuation, unrealizedGainTone } from '../../../shared/lib/format'

// 通知の「何が起きたか」「前後の値」の文と、率の色の向き（価格の変動の通知の要件32・要件33）。
// 値は API の境界（api/fetchNotifications）で、形と、種類ごとに要る値の有無を確かめてある。外れたら throw する（描画中には届かない）。

/** 率と含み損益の色の向き。プラスは 'gain'（赤）、マイナスは 'loss'（青）、0 と色を付けないものは 'none'（要件33）。 */
export type NotificationTone = 'gain' | 'loss' | 'none'

/** 「何が起きたか」。label + value が文。value が色のつく部分。 */
export type NotificationHeadline = { label: string; value: string; tone: NotificationTone }

const RATE_PATTERN = /^(-?)([0-9]+)\.([0-9])$/

/** 種類の言葉（「何が起きたか」の label の前半）。後ろに空白を付けて label にする。 */
const HEADLINE_LABELS = {
  cost: '取得価格から',
  peak: '最高値から',
  prev: '前の取引日から',
  pnl: '含み損益',
  price: '終値',
} as const

const LABEL_SEPARATOR = ' '
const ARROW = ' → '
/** 日付の閉じ括弧の直後の矢印（全角の括弧の後ろには空白を入れない。要件32 の表）。 */
const ARROW_AFTER_DATE = '→ '

/** 値がない（API の境界で確かめてあるので、描画中には届かない）。 */
function missing(name: string): never {
  throw new Error(`通知の${name}がありません`)
}

/** 率の符号（要件33）。0 は符号なし（"-0.0" のような値も0として扱う）。 */
function rateSign(rate: string): { sign: '+' | '-' | ''; digits: string } {
  const match = RATE_PATTERN.exec(rate)
  if (!match) throw new Error('率の形が正しくありません')
  const [, minus, integer, fraction] = match
  const digits = `${integer}.${fraction}`
  if (Number(integer) === 0 && fraction === '0') return { sign: '', digits }
  return { sign: minus === '-' ? '-' : '+', digits }
}

function toneOf(sign: '+' | '-' | ''): NotificationTone {
  if (sign === '+') return 'gain'
  if (sign === '-') return 'loss'
  return 'none'
}

/**
 * 率を、符号付きの小数第1位までの文字列にする（要件33）。プラスは「+」、マイナスは「-」、0 は符号なし。
 * 例 "-20.4" → "-20.4%"、"6.1" → "+6.1%"、"0.0" → "0.0%"。
 */
export function formatSignedRate(rate: string): string {
  const { sign, digits } = rateSign(rate)
  return `${sign}${digits}%`
}

/** 含み損益 gain（符号付き。円は整数、ドルは小数2桁）を、符号付きの金額（プラスは「+」）にする。例 "-104000" → "-104,000円"。 */
function formatSignedGain(currency: Currency, gain: string): string {
  const tone = unrealizedGainTone(gain)
  const sign = tone === 'gain' ? '+' : tone === 'loss' ? '-' : ''
  return `${sign}${formatValuation(currency, gain.replace(/^-/, ''))}`
}

/**
 * 「何が起きたか」（要件32）。例 { label: '取得価格から ', value: '-20.4%', tone: 'loss' }。
 * 率と含み損益の金額の値は、プラスなら 'gain'・マイナスなら 'loss'・0 なら 'none'。決めた価格の通知の終値は 'none'（要件33）。
 */
export function notificationHeadline(n: PriceAlertNotification): NotificationHeadline {
  const label = `${HEADLINE_LABELS[n.condition.kind]}${LABEL_SEPARATOR}`
  switch (n.condition.kind) {
    case 'cost':
    case 'peak':
    case 'prev': {
      if (n.rate === null) return missing('率')
      const { sign } = rateSign(n.rate)
      return { label, value: formatSignedRate(n.rate), tone: toneOf(sign) }
    }
    case 'pnl': {
      if (n.unrealizedGain === null) return missing('含み損益')
      const tone = unrealizedGainTone(n.unrealizedGain)
      return { label, value: formatSignedGain(n.currency, n.unrealizedGain), tone: tone === 'even' ? 'none' : tone }
    }
    case 'price':
      return { label, value: formatUnitPrice(n.currency, n.close), tone: 'none' }
  }
}

/**
 * 「前後の値」（要件32）。例「取得価格 2,500円 → 終値 1,990円」。
 * 決めた価格を分割で割ったときは「決めた価格 4,000円 以上（分割に合わせた値。足したときは 8,000円）」。
 */
export function notificationDetail(n: PriceAlertNotification): string {
  const close = `終値 ${formatUnitPrice(n.currency, n.close)}`
  const basePrice = () => (n.basePrice === null ? missing('基準の価格') : formatUnitPrice(n.currency, n.basePrice))
  const baseDate = () => (n.baseDate === null ? missing('基準の日') : n.baseDate)
  switch (n.condition.kind) {
    case 'cost':
      return `取得価格 ${basePrice()}${ARROW}${close}`
    case 'peak':
      return `最高値 ${basePrice()}（${baseDate()}）${ARROW_AFTER_DATE}${close}`
    case 'prev':
      return `前の終値 ${basePrice()}（${baseDate()}）${ARROW_AFTER_DATE}${close}`
    case 'pnl':
      if (n.quantity === null) return missing('保有数量')
      return `取得価格 ${basePrice()}${ARROW}${close}（${formatShares(n.quantity)}）`
    case 'price': {
      const addedPrice = formatUnitPrice(n.currency, n.condition.threshold)
      const side = n.condition.direction === 'below' ? '以下' : '以上'
      if (n.adjustedThreshold === null) return `決めた価格 ${addedPrice} ${side}`
      const adjusted = formatUnitPrice(n.currency, n.adjustedThreshold)
      return `決めた価格 ${adjusted} ${side}（分割に合わせた値。足したときは ${addedPrice}）`
    }
  }
}
