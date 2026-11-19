// 機能をまたいで使う query key。features/holdings と features/notifications は互いを import しない（価格の変動の通知のプラン 判断8）ので、
// 両方が使う key はここに置く。

/**
 * 見ていない通知の数の query key（価格の変動の通知の要件25・要件27）。
 * ベル（features/notifications）が取得し、今すぐ更新の後（features/holdings）と、通知のリストを見たにした後（features/notifications）に無効にする。
 * 一覧の key（['notifications', 'list']）と2つ目が違うので、これを無効にしても一覧は無効にならない。
 */
export const UNSEEN_NOTIFICATION_COUNT_QUERY_KEY = ['notifications', 'unseen-count'] as const
