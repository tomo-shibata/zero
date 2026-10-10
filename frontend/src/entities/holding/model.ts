// 保有株式一覧のモデル。GET /api/holdings の応答と同じ形（プラン 4.4「API」）。

/** 価格が保存済みの銘柄の、価格の欄。 */
type PricedFields = {
  /** 現在の価格（1株あたり）。小数第1位を1桁付けた文字列（例 "3000.0"）。 */
  currentPrice: string
  /** 価格の基準日（その終値の取引日）。"YYYY-MM-DD"。 */
  priceDate: string
  /** 評価額（円の整数）。 */
  valuation: number
}

/** 一度も価格を保存していない銘柄の、価格の欄（要件 FR-10）。3つとも null。 */
type UnpricedFields = {
  currentPrice: null
  priceDate: null
  valuation: null
}

/** 一覧の1行（同じ銘柄はまとめて1行。要件 FR-4）。 */
export type Holding = {
  /** 銘柄コード。 */
  code: string
  /** 銘柄名。 */
  name: string
  /** 保有数量（株）。 */
  quantity: number
  /** 取得価格（1株あたり、加重平均）。小数第1位を1桁付けた文字列（例 "2500.0"）。 */
  acquisitionPrice: string
} & (PricedFields | UnpricedFields)

/** 保有株式一覧。 */
export type HoldingsList = {
  /** 銘柄コードの昇順（要件 FR-5）。0件のときは空の配列。 */
  holdings: Holding[]
  /** 合計評価額（円の整数）。価格のある行の評価額の合計（要件 FR-6）。 */
  totalValuation: number
  /** 価格のない行が1件以上あれば true（要件 FR-6）。 */
  totalExcludesUnpriced: boolean
}
