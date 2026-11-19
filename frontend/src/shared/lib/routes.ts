// 画面のパス（銘柄ごとの売買履歴の要件1・要件4。プラン 4.8）。
// 機能どうしは直接つながない（features/holdings は features/stock-history を import しない）ので、移り先のパスはここで作る。

/** 保有株式一覧の画面。 */
export const HOLDINGS_PATH = '/holdings'

/** 通知のリストの画面（価格の変動の通知の要件28）。 */
export const NOTIFICATIONS_PATH = '/notifications'

/** 通知の条件を決める画面（価格の変動の通知の要件29）。 */
export const ALERTS_PATH = '/alerts'

/**
 * 銘柄 code の売買履歴の画面のパス（例 `/holdings/7203`）。
 * 銘柄コードに「.」「-」が使える（例 BRK.B）ので、それらはそのまま、「/」などは符号化する。
 */
export function stockHistoryPath(code: string): string {
  return `${HOLDINGS_PATH}/${encodeURIComponent(code)}`
}
