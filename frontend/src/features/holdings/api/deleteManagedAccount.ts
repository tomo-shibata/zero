/** おまかせ運用の口座の削除の API のパス（おまかせ運用のプラン 4.4）。名前はクエリの name に入れる。開発・E2E では Vite の proxy が API サーバーへ転送する。 */
const MANAGED_ACCOUNTS_API_PATH = '/api/managed-accounts'

/** 削除に成功したときの HTTP のステータス（本文なし）。 */
const NO_CONTENT = 204

/**
 * 応答を待つ上限（ミリ秒）。超えたら要求を中断して reject する（deleteHolding と同じ理由。
 * 削除の確認のダイアログは削除中は閉じられないので、応答が返らないままでは利用者がそこから出られなくなるため）。
 */
const DELETE_TIMEOUT_MS = 10_000

/**
 * ログイン中の利用者の、口座 name の記録をすべて削除する（おまかせ運用の要件24）。
 * 名前は URLSearchParams でクエリに入れる（「&」「#」「空白」などを含んでも、別のクエリにならないように）。
 * 204 以外の応答（持っていない口座の 404 を含む）、通信の失敗、10秒たっても応答がないときは reject する
 * （画面は削除の失敗として扱う）。
 */
export async function deleteManagedAccount(name: string): Promise<void> {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), DELETE_TIMEOUT_MS)
  try {
    const query = new URLSearchParams({ name })
    const response = await fetch(`${MANAGED_ACCOUNTS_API_PATH}?${query.toString()}`, {
      method: 'DELETE',
      signal: controller.signal,
    })
    if (response.status !== NO_CONTENT) {
      throw new Error(`口座の削除の API がエラーを返しました（HTTP ${response.status}）`)
    }
  } finally {
    // 成功でも失敗でも片づける（残すと、あとで使われない中断が走り、テストの時計にも残る）。
    clearTimeout(timer)
  }
}
