// 金額・数量を画面に出す文字列にする（要件 FR-11〜13）。
//
// ありえない値（形の違う単価、小数・NaN・2^53 を超える数）は、丸めたり "NaN円" にしたりして誤った表示を出さずに throw する
// （PR④ ステップ6 の EC-3）。画面の値はどれも API の境界（fetchHoldings の zod の検証）を通ったものなので、
// 描画中にここで throw することはない。そのため、描画中の例外を受け止めるエラーバウンダリは置いていない（PR④ ステップ6 の EC-4）。
// 検証を通らない値をここに渡す呼び出し側を足すときは、エラーバウンダリを置くか、渡す前に確かめる。

const YEN_SUFFIX = '円'
const SHARES_SUFFIX = '株'
/** 数の書式に使うロケール（3桁ごとに「,」で区切る）。 */
const NUMBER_LOCALE = 'ja-JP'

/** 3桁区切り（例 1234567 → "1,234,567"）。 */
const groupedInteger = new Intl.NumberFormat(NUMBER_LOCALE, {
  maximumFractionDigits: 0,
  useGrouping: true,
})

/**
 * 単価の文字列の形（API の単価。小数第1位を1桁付けた文字列。例 "2500.0"、"1234.5"）。
 * 1つ目のグループが整数部、2つ目が小数第1位。
 */
export const UNIT_PRICE_PATTERN = /^([0-9]+)\.([0-9])$/

/** 小数第1位がこの値なら、単価を整数で表示する（FR-12）。 */
const ZERO_TENTHS_DIGIT = '0'

/**
 * 整数を3桁区切りにする（例 1234567 → "1,234,567"）。
 * 安全な整数（Number.isSafeInteger）でなければ throw する（このファイルの先頭のコメントのとおり、描画中には届かない）。
 */
function formatSafeInteger(value: number): string {
  if (!Number.isSafeInteger(value)) {
    throw new Error(`安全に扱える整数ではありません: ${value}`)
  }
  return groupedInteger.format(value)
}

/** 評価額・合計評価額（円の整数）を表示する。例 300000 → "300,000円"、0 → "0円"（FR-11）。 */
export function formatYen(amount: number): string {
  return `${formatSafeInteger(amount)}${YEN_SUFFIX}`
}

/**
 * 単価（取得価格・現在の価格）を表示する。例 "1234.5" → "1,234.5円"、"2500.0" → "2,500円"（FR-12）。
 * price は UNIT_PRICE_PATTERN の形の文字列（API の境界で検証済み）。
 * 整数部を Number にせず BigInt で区切るのは、桁が大きくても値を丸めないため。
 */
export function formatUnitPrice(price: string): string {
  const match = UNIT_PRICE_PATTERN.exec(price)
  if (!match) {
    // このファイルの先頭のコメントのとおり、描画中には届かない。
    throw new Error(`単価の形が正しくありません: ${JSON.stringify(price)}`)
  }
  const [, integerPart, tenthsDigit] = match
  const groupedIntegerPart = groupedInteger.format(BigInt(integerPart))
  if (tenthsDigit === ZERO_TENTHS_DIGIT) {
    return `${groupedIntegerPart}${YEN_SUFFIX}`
  }
  return `${groupedIntegerPart}.${tenthsDigit}${YEN_SUFFIX}`
}

/** 保有数量を表示する。例 100 → "100株"、1000 → "1,000株"（FR-13）。 */
export function formatShares(quantity: number): string {
  return `${formatSafeInteger(quantity)}${SHARES_SUFFIX}`
}
