/** 並びの保存の API のパス（カード表示のプラン 判断13）。開発・E2E では Vite の proxy が API サーバーへ転送する。 */
const HOLDING_ORDER_API_PATH = '/api/holding-order'

/** 保存に成功したときの HTTP のステータス（本文なし）。 */
const NO_CONTENT = 204

/**
 * 応答を待つ上限（ミリ秒）。超えたら要求を中断して reject する（カード表示のプラン 判断14。削除と同じ）。
 * 保存は1つずつ送るので、応答が返らないままだと、後の並べ替えの保存がいつまでも送られなくなるため。
 */
const SAVE_TIMEOUT_MS = 10_000

/**
 * 204 以外の応答で reject するときのエラー。status で失敗の種類を区別できる
 * （400 は画面の一覧が古い証拠なので取り直す。通信の失敗・中断は、このエラーではない。プラン 判断11）。
 */
export class SaveHoldingsOrderError extends Error {
  readonly status: number

  constructor(status: number) {
    super(`並びの保存の API がエラーを返しました（HTTP ${status}）`)
    this.name = 'SaveHoldingsOrderError'
    this.status = status
  }
}

/**
 * 並び（銘柄コードを表示する順に）を保存する（要件23）。PUT /api/holding-order、本文 {"codes": codes}。
 * 204 以外の応答（400・500・403 を含む。SaveHoldingsOrderError）、通信の失敗、10秒たっても応答がないときは reject する（画面は保存の失敗として扱う。要件30）。
 */
export async function saveHoldingsOrder(codes: readonly string[]): Promise<void> {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), SAVE_TIMEOUT_MS)
  try {
    const response = await fetch(HOLDING_ORDER_API_PATH, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ codes }),
      signal: controller.signal,
    })
    if (response.status !== NO_CONTENT) {
      throw new SaveHoldingsOrderError(response.status)
    }
  } finally {
    // 成功でも失敗でも片づける（残すと、あとで使われない中断が走り、テストの時計にも残る）。
    clearTimeout(timer)
  }
}
