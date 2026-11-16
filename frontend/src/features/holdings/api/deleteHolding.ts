/** 銘柄の削除の API のパスの前半（値段の更新のプラン 4.4「API」）。銘柄コードを足して使う。開発・E2E では Vite の proxy が API サーバーへ転送する。 */
const HOLDINGS_API_PATH = '/api/holdings'

/** 削除に成功したときの HTTP のステータス（本文なし）。 */
const NO_CONTENT = 204

/**
 * 応答を待つ上限（ミリ秒）。超えたら要求を中断して reject する（値段の更新のプラン 6章の判断37）。
 * 削除の確認のダイアログは、削除中は閉じられないので、応答が返らないままでは利用者がそこから出られなくなるため。
 */
const DELETE_TIMEOUT_MS = 10_000

/**
 * ログイン中の利用者の、銘柄 code の保有データをすべて削除する（要件14）。
 * 銘柄コードは encodeURIComponent してパスに入れる（"/" などを含んでも、別のパスにならないように）。
 * 204 以外の応答（保有していない銘柄の 404 を含む）、通信の失敗、10秒たっても応答がないときは reject する
 * （画面は削除の失敗として扱う。要件15）。
 */
export async function deleteHolding(code: string): Promise<void> {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), DELETE_TIMEOUT_MS)
  try {
    const response = await fetch(`${HOLDINGS_API_PATH}/${encodeURIComponent(code)}`, {
      method: 'DELETE',
      signal: controller.signal,
    })
    if (response.status !== NO_CONTENT) {
      throw new Error(`銘柄の削除の API がエラーを返しました（HTTP ${response.status}）`)
    }
  } finally {
    // 成功でも失敗でも片づける（残すと、あとで使われない中断が走り、テストの時計にも残る）。
    clearTimeout(timer)
  }
}
