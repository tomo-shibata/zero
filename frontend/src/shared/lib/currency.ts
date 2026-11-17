// 通貨（円 JPY とドル USD）と、API の金額の文字列の形（売買履歴の要件1・要件14、プラン 4.3・4.7）。
//
// 画面の Currency 型はここに置き、entities/holding/model.ts と shared/lib/format.ts が import する。
// shared は entities を import しない（依存の向きは内側＝shared へ向かうため）ので、shared 側に置く。

/** 画面が扱う通貨。API の currency と同じ文字列。 */
export const CURRENCIES = ['JPY', 'USD'] as const

/** 銘柄の通貨。円（JPY）とドル（USD）だけ。 */
export type Currency = (typeof CURRENCIES)[number]

/**
 * 単価（取得価格・現在の価格）の文字列の形。
 * 円は小数第1位を1桁付けた文字列（例 "2500.0"）、ドルは小数第2位を2桁付けた文字列（例 "150.25"）。
 * 1つ目のグループが整数部、2つ目が小数部。
 */
export const UNIT_PRICE_PATTERNS: Record<Currency, RegExp> = {
  JPY: /^([0-9]+)\.([0-9])$/,
  USD: /^([0-9]+)\.([0-9]{2})$/,
}

/**
 * 評価額・合計評価額の文字列の形。
 * 円は整数（例 "450000"。1円未満は切り捨ててあるので小数を持たない）、ドルは小数第2位を2桁付けた文字列（例 "1800.00"）。
 * 1つ目のグループが整数部、2つ目が小数部（円にはない）。
 */
export const VALUATION_PATTERNS: Record<Currency, RegExp> = {
  JPY: /^([0-9]+)()$/,
  USD: /^([0-9]+)\.([0-9]{2})$/,
}

/**
 * 含み損益の文字列の形（カード表示のプラン 4.2）。符号付き（マイナスだけ「-」が付く。プラスに「+」は付かない）。
 * 円は整数（例 "50000"・"-501"）、ドルは小数第2位を2桁付けた文字列（例 "297.50"・"-0.01"）。
 * 1つ目のグループが符号（なければ空）、2つ目が整数部、3つ目が小数部（円にはない）。
 */
export const UNREALIZED_GAIN_PATTERNS: Record<Currency, RegExp> = {
  JPY: /^(-?)([0-9]+)()$/,
  USD: /^(-?)([0-9]+)\.([0-9]{2})$/,
}

/**
 * 含み損益の率（%）の文字列の形。通貨によらず、符号付きの小数第1位まで（例 "20.0"・"-12.5"）。
 * 1つ目のグループが符号（なければ空）、2つ目が整数部、3つ目が小数部。
 */
export const UNREALIZED_GAIN_RATE_PATTERN = /^(-?)([0-9]+)\.([0-9])$/
