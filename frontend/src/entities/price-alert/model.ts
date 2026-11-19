import type { Currency } from '../../shared/lib/currency'

// 価格の変動の通知のモデル。GET /api/notifications の応答と同じ形（価格の変動の通知のプラン 4.5）。

/** 条件の種類（要件1）。取得価格から・最高値から・前の取引日から・含み損益の金額・決めた価格。 */
export const ALERT_KINDS = ['cost', 'peak', 'prev', 'pnl', 'price'] as const
export type AlertKind = (typeof ALERT_KINDS)[number]

/** 条件の対象（要件1）。全銘柄か、1つの銘柄。 */
export const ALERT_SCOPES = ['all', 'stock'] as const
export type AlertScope = (typeof ALERT_SCOPES)[number]

/** 条件の向き（要件2）。種類ごとに使える向きが決まっている（下の ALERT_DIRECTIONS_BY_KIND）。 */
export const ALERT_DIRECTIONS = ['up', 'down', 'either', 'loss', 'gain', 'above', 'below'] as const
export type AlertDirection = (typeof ALERT_DIRECTIONS)[number]

/** 種類ごとに使える向き（要件2の表）。 */
export const ALERT_DIRECTIONS_BY_KIND: Record<AlertKind, readonly AlertDirection[]> = {
  cost: ['up', 'down'],
  peak: ['down'],
  prev: ['up', 'down', 'either'],
  pnl: ['loss', 'gain'],
  price: ['above', 'below'],
}

/**
 * 条件の要約（通知に写して持っている、作ったときの条件）。
 * threshold の形は、% は小数1桁（"20.0"）、円の金額は整数、ドルの金額は小数2桁、円の価格は小数1桁、ドルの価格は小数2桁。
 */
export type AlertConditionSummary = {
  kind: AlertKind
  scope: AlertScope
  /** 全銘柄の条件は null。 */
  stockCode: string | null
  direction: AlertDirection
  threshold: string
}

/**
 * 通知1件。種類で使わない値は null。名前に PriceAlert を付けるのは、ブラウザーの Notification と同じ名前だと、
 * import を忘れても黙ってコンパイルが通るため。
 */
export type PriceAlertNotification = {
  id: string
  stockCode: string
  stockName: string
  currency: Currency
  /** 判定した終値の取引日。"YYYY-MM-DD"。 */
  tradingDate: string
  /** 取得した時点で見ていなかったか。 */
  isNew: boolean
  condition: AlertConditionSummary
  /** 判定した終値（単価の形）。 */
  close: string
  /** 率（%。符号付きの小数1桁。プラスに「+」は付かない）。取得価格から・最高値から・前の取引日から。 */
  rate: string | null
  /** 取得価格・最高値・前の終値（単価の形）。含み損益の金額の通知は取得価格。 */
  basePrice: string | null
  /** 最高値・前の終値の取引日。 */
  baseDate: string | null
  /** 含み損益（符号付き。円は整数、ドルは小数2桁）。含み損益の金額。 */
  unrealizedGain: string | null
  /** 保有数量。含み損益の金額。 */
  quantity: number | null
  /** 決めた価格を分割で割った値（単価の形）。割る数が1より大きいときだけ。 */
  adjustedThreshold: string | null
}

/** 通知のリスト。latestId は、その利用者の通知の最大の ID（100件より古いものを含む）。0件なら null。 */
export type PriceAlertNotificationList = {
  items: PriceAlertNotification[]
  latestId: string | null
}
