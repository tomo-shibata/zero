import type { PriceAlertNotification, PriceAlertNotificationList } from '../../../src/entities/price-alert/model'

// 通知の画面・取得・文のテストが使う通知の例。要件 docs/requirements/price-alerts.md の「通知のリスト」の基準35〜基準37 の5件（通知（直接））と同じ値。
// 値の形は API の形（プラン docs/plans/price-alerts/plan.md 4.5）: 単価は円が小数1桁・ドルが小数2桁、含み損益は符号付き、率は符号付きの小数1桁（プラスに + は付けない）。

/** 空の値（種類で使わない値は null）の通知。overrides で変える。 */
export function notification(overrides: Partial<PriceAlertNotification>): PriceAlertNotification {
  return {
    id: '1',
    stockCode: '7203',
    stockName: 'トヨタ自動車',
    currency: 'JPY',
    tradingDate: '2026-11-14',
    isNew: false,
    condition: { kind: 'cost', scope: 'all', stockCode: null, direction: 'down', threshold: '20.0' },
    close: '1990.0',
    rate: null,
    basePrice: null,
    baseDate: null,
    unrealizedGain: null,
    quantity: null,
    adjustedThreshold: null,
    ...overrides,
  }
}

/** 2026-11-14 7203 取得価格から（取得価格 2,500円 → 終値 1,990円、-20.4%。条件: 全銘柄・取得価格から 20% 以上 下がったら）（見ていない）。 */
export const TOYOTA_COST = notification({
  id: '12',
  isNew: true,
  rate: '-20.4',
  basePrice: '2500.0',
})

/** 2026-11-14 AAPL 前の取引日から（前の終値 180.25ドル（2026-11-13）→ 終値 191.24ドル、+6.1%。条件: 全銘柄・前の取引日から 5% 以上 動いたら（上がっても下がっても））（見ていない）。 */
export const APPLE_PREV = notification({
  id: '13',
  stockCode: 'AAPL',
  stockName: 'Apple',
  currency: 'USD',
  tradingDate: '2026-11-14',
  isNew: true,
  condition: { kind: 'prev', scope: 'all', stockCode: null, direction: 'either', threshold: '5.0' },
  close: '191.24',
  rate: '6.1',
  basePrice: '180.25',
  baseDate: '2026-11-13',
})

/** 2026-11-13 9984 決めた価格（終値 8,120円、決めた価格 8,000円 以上。条件: 9984・8,000円 以上になったら）（見ていない）。 */
export const SOFTBANK_PRICE = notification({
  id: '11',
  stockCode: '9984',
  stockName: 'ソフトバンクグループ',
  tradingDate: '2026-11-13',
  isNew: true,
  condition: { kind: 'price', scope: 'stock', stockCode: '9984', direction: 'above', threshold: '8000.0' },
  close: '8120.0',
})

/** 2026-11-10 6758 最高値から（最高値 4,000円（2026-09-30）→ 終値 3,392円、-15.2%）（見た）。 */
export const SONY_PEAK = notification({
  id: '10',
  stockCode: '6758',
  stockName: 'ソニーグループ',
  tradingDate: '2026-11-10',
  isNew: false,
  condition: { kind: 'peak', scope: 'all', stockCode: null, direction: 'down', threshold: '15.0' },
  close: '3392.0',
  rate: '-15.2',
  basePrice: '4000.0',
  baseDate: '2026-09-30',
})

/** 2026-11-04 6758 含み損益の金額（取得価格 4,000円 → 終値 3,480円、200株、含み損益 -104,000円。条件: 6758・含み損が 100,000円 以上になったら）（見た）。 */
export const SONY_PNL = notification({
  id: '9',
  stockCode: '6758',
  stockName: 'ソニーグループ',
  tradingDate: '2026-11-04',
  isNew: false,
  condition: { kind: 'pnl', scope: 'stock', stockCode: '6758', direction: 'loss', threshold: '100000' },
  close: '3480.0',
  basePrice: '4000.0',
  unrealizedGain: '-104000',
  quantity: 200,
})

/** 基準35〜基準37 の5件（取引日の新しい順）。latestId は5件のどれより大きい '15'（100件より古い通知を含む最大の ID を想定）。 */
export function fiveNotifications(): PriceAlertNotificationList {
  return { items: [TOYOTA_COST, APPLE_PREV, SOFTBANK_PRICE, SONY_PEAK, SONY_PNL], latestId: '15' }
}
