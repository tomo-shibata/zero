// キーボードで並べ替えるときに、支援技術へ読み上げさせる文（要件20）。位置の番号 n は、先頭を1とする。

/** 持ち上げたとき。 */
export function pickedUpAnnouncement(code: string, name: string): string {
  return `${code} ${name}を持ち上げました。矢印キーで動かし、スペースキーで置きます。`
}

/** 動かしたとき（動かした先の位置）。 */
export function movedAnnouncement(position: number): string {
  return `${position}番目に動かしました。`
}

/** 置いたとき（置いた位置）。 */
export function droppedAnnouncement(code: string, name: string, position: number): string {
  return `${code} ${name}を${position}番目に置きました。`
}

/** 取り消したとき。 */
export const CANCELLED_ANNOUNCEMENT = '並べ替えを取り消しました。'

/** カードの操作の説明（画面には出さない。カードの aria-describedby が指す）。 */
export const SCREEN_READER_INSTRUCTIONS =
  'スペースキーで持ち上げ、矢印キーで動かし、もう一度スペースキーで置きます。Esc で取り消します。Enter キーで売買履歴を開きます。'
