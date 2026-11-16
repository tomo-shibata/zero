// 金額・数量を画面に出す文字列にする（保有株式一覧の要件 FR-11〜13、売買履歴の要件14）。
//
// ありえない値（通貨に合わない形の金額、小数・NaN・2^53 を超える数量）は、丸めたり "NaN円" にしたりして誤った表示を出さずに throw する
// （PR④ ステップ6 の EC-3）。画面の値はどれも API の境界（fetchHoldings の zod の検証）を通ったものなので、
// 描画中にここで throw することはない。そのため、描画中の例外を受け止めるエラーバウンダリは置いていない（PR④ ステップ6 の EC-4）。
// 検証を通らない値をここに渡す呼び出し側を足すときは、エラーバウンダリを置くか、渡す前に確かめる。

import { type Currency, UNIT_PRICE_PATTERNS, VALUATION_PATTERNS } from './currency'

/** 通貨ごとの、金額の後ろに付ける単位。 */
const CURRENCY_SUFFIXES: Record<Currency, string> = { JPY: '円', USD: 'ドル' }
const SHARES_SUFFIX = '株'
/** 数の書式に使うロケール（3桁ごとに「,」で区切る）。 */
const NUMBER_LOCALE = 'ja-JP'

/** 3桁区切り（例 1234567 → "1,234,567"）。 */
const groupedInteger = new Intl.NumberFormat(NUMBER_LOCALE, {
  maximumFractionDigits: 0,
  useGrouping: true,
})

/** 円の単価で、小数第1位がこの値なら、整数で表示する（FR-12）。ドルは小数を省かない。 */
const ZERO_TENTHS_DIGIT = '0'

/**
 * 整数を3桁区切りにする（例 1234567 → "1,234,567"）。
 * 安全な整数（Number.isSafeInteger）でなければ throw する（このファイルの先頭のコメントのとおり、描画中には届かない）。
 */
function formatSafeInteger(value: number): string {
  if (!Number.isSafeInteger(value)) {
    // 保有数量は資産の情報なので、値はエラーの文言に入れない（バックエンドと同じ）。
    throw new Error('保有数量が安全に扱える整数ではありません')
  }
  return groupedInteger.format(value)
}

/**
 * 金額の文字列 amount を、pattern（整数部と小数部のグループを持つ）で分けて、整数部を3桁区切りにした文字列と、小数部にする。
 * 整数部を Number にせず BigInt で区切るのは、桁が大きくても値を丸めないため。
 * pattern に合わなければ throw する（このファイルの先頭のコメントのとおり、描画中には届かない）。
 */
function splitAmount(
  pattern: RegExp,
  amount: string,
  kind: string,
  currency: Currency,
): { integer: string; fraction: string } {
  const match = pattern.exec(amount)
  if (!match) {
    // 資産の額はエラーの文言に入れない（バックエンドと同じ）。種類と通貨だけを書く。
    throw new Error(`${kind}の形が正しくありません（通貨 ${currency}）`)
  }
  const [, integerPart, fraction] = match
  return { integer: groupedInteger.format(BigInt(integerPart)), fraction }
}

/**
 * 評価額・合計評価額を表示する。
 * 円は整数の文字列を3桁区切りにする（例 "300000" → "300,000円"、"0" → "0円"。FR-11）。
 * ドルは小数第2位を必ず2桁付ける（例 "1800.00" → "1,800.00ドル"、"0.00" → "0.00ドル"。要件14）。
 * valuation は VALUATION_PATTERNS の形の文字列（API の境界で検証済み）。形が通貨に合わなければ throw する。
 */
export function formatValuation(currency: Currency, valuation: string): string {
  const { integer, fraction } = splitAmount(VALUATION_PATTERNS[currency], valuation, '評価額', currency)
  const suffix = CURRENCY_SUFFIXES[currency]
  return currency === 'JPY' ? `${integer}${suffix}` : `${integer}.${fraction}${suffix}`
}

/**
 * 単価（取得価格・現在の価格）を表示する。
 * 円は小数第1位が0なら整数で表示する（例 "1234.5" → "1,234.5円"、"2500.0" → "2,500円"。FR-12）。
 * ドルは小数第2位を省かない（例 "150.25" → "150.25ドル"、"180.00" → "180.00ドル"。要件14）。
 * price は UNIT_PRICE_PATTERNS の形の文字列（API の境界で検証済み）。形が通貨に合わなければ throw する。
 */
export function formatUnitPrice(currency: Currency, price: string): string {
  const { integer, fraction } = splitAmount(UNIT_PRICE_PATTERNS[currency], price, '単価', currency)
  const suffix = CURRENCY_SUFFIXES[currency]
  if (currency === 'JPY' && fraction === ZERO_TENTHS_DIGIT) {
    return `${integer}${suffix}`
  }
  return `${integer}.${fraction}${suffix}`
}

/** 保有数量を表示する。例 100 → "100株"、1000 → "1,000株"（FR-13）。 */
export function formatShares(quantity: number): string {
  return `${formatSafeInteger(quantity)}${SHARES_SUFFIX}`
}
