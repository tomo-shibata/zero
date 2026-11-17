import type { Currency } from '../../shared/lib/currency'

// 保有株式一覧のモデル。GET /api/holdings の応答と同じ形（売買履歴のプラン 4.3「一覧の API の形」）。

/** 価格が保存済みの銘柄の、価格の欄。 */
type PricedFields = {
  /** 現在の価格（1株あたり）。円は小数第1位を1桁（例 "3000.0"）、ドルは小数第2位を2桁（例 "180.00"）付けた文字列。 */
  currentPrice: string
  /** 価格の基準日（その終値の取引日）。"YYYY-MM-DD"。 */
  priceDate: string
  /** 評価額の文字列。円は整数（例 "300000"。1円未満は切り捨て済み）、ドルは小数第2位を2桁付けた文字列（例 "1800.00"）。 */
  valuation: string
  /** 表示する含み損益。円は符号付きの整数（例 "50000"・"-501"）、ドルは符号付きの小数第2位まで（例 "297.50"・"-0.01"）。プラスに「+」は付かない。 */
  unrealizedGain: string
  /** 含み損益の率（%）。符号付きの小数第1位まで（例 "20.0"・"-12.5"）。 */
  unrealizedGainRate: string
}

/** 一度も価格を保存していない銘柄の、価格の欄（要件 FR-10）。5つとも null。 */
type UnpricedFields = {
  currentPrice: null
  priceDate: null
  valuation: null
  unrealizedGain: null
  unrealizedGainRate: null
}

/** 一覧の1行（同じ銘柄はまとめて1行。要件6）。 */
export type Holding = {
  /** 銘柄コード。 */
  code: string
  /** 銘柄名。 */
  name: string
  /** 銘柄の通貨（円 JPY かドル USD）。金額の文字列の形と表示の単位は、これで決まる。 */
  currency: Currency
  /** 保有数量（株）。 */
  quantity: number
  /** 取得価格（1株あたり、移動平均法）。円は小数第1位を1桁（例 "2500.0"）、ドルは小数第2位を2桁（例 "150.25"）付けた文字列。 */
  acquisitionPrice: string
} & (PricedFields | UnpricedFields)

/** 1つの通貨の合計評価額と含み損益の合計（要件12・13。通貨をまたいで足さず、為替の換算はしない）。 */
export type CurrencyTotal = {
  currency: Currency
  /** 合計評価額の文字列。形は Holding の valuation と同じ（その通貨の、価格のある行の評価額の合計）。 */
  valuation: string
  /** その通貨に価格のない行が1件以上あれば true（合計評価額・含み損益の合計にその銘柄を含まない）。 */
  excludesUnpriced: boolean
  /** 含み損益の合計。形は Holding の unrealizedGain と同じ。その通貨に価格のある銘柄がなければ null（unrealizedGainRate と同時に null）。 */
  unrealizedGain: string | null
  /** 含み損益の合計の率（%）。形は Holding の unrealizedGainRate と同じ。unrealizedGain が null のときだけ null。 */
  unrealizedGainRate: string | null
}

/** 保有株式一覧。 */
export type HoldingsList = {
  /** 銘柄コードの昇順（通貨によらない。要件15）。0件のときは空の配列。 */
  holdings: Holding[]
  /** 通貨ごとの合計評価額。保有のある通貨だけを、円、ドルの順に入れる。0件のときは空の配列。 */
  totals: CurrencyTotal[]
}

/**
 * 価格の今すぐ更新の結果。POST /api/price-refresh の応答と同じ形（値段の更新のプラン 4.4「API」）。
 * どちらも銘柄コードの昇順。0件のときは空の配列。
 */
export type PriceRefreshResult = {
  /** 価格を保存できた銘柄コード。 */
  saved: string[]
  /** 価格を取得・保存できなかった銘柄コード。 */
  failed: string[]
}
