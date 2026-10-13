import type { PriceRefreshResult } from '../../../entities/holding/model'

const NOTHING_TO_REFRESH_MESSAGE = '更新する銘柄はありません'
const FAILED_CODES_SEPARATOR = ', '

/** 保存できた銘柄が1件以上のときの文言（要件18。n は保存できた銘柄の数）。 */
function savedMessage(count: number): string {
  return `${count}銘柄の価格を更新しました`
}

/** 取得・保存できなかった銘柄が1件以上のときの文言（銘柄コードを「, 」でつなぐ）。 */
function failedMessage(codes: readonly string[]): string {
  return `価格を取得できなかった銘柄: ${codes.join(FAILED_CODES_SEPARATOR)}`
}

/**
 * 今すぐ更新の結果を、画面に出す文を表示する順に返す（要件18）。
 * 保存できた銘柄があれば「n銘柄の価格を更新しました」、できなかった銘柄があれば「価格を取得できなかった銘柄: 1301, 9984」
 * （銘柄コードは昇順に並べ直す。渡された配列は書き換えない）。どちらも0件なら「更新する銘柄はありません」だけ。
 */
export function refreshResultMessages(result: PriceRefreshResult): string[] {
  const messages: string[] = []
  if (result.saved.length > 0) {
    messages.push(savedMessage(result.saved.length))
  }
  if (result.failed.length > 0) {
    messages.push(failedMessage([...result.failed].sort()))
  }
  if (messages.length === 0) {
    messages.push(NOTHING_TO_REFRESH_MESSAGE)
  }
  return messages
}
