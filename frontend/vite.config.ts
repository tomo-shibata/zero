import react from '@vitejs/plugin-react'
import type { Plugin } from 'vite'
import { defineConfig } from 'vitest/config'

/**
 * dev サーバーが /api を転送する先の既定値（API サーバーの既定の待ち受け。プラン 4.5）。
 * ホストを localhost にしないのは、Windows で先に IPv6 の ::1 に解決され、接続が遅れたり別のサーバーにつながったりするため。
 */
const DEFAULT_API_PROXY_TARGET = 'http://127.0.0.1:8080'

/**
 * /api の転送先。環境変数 API_PROXY_TARGET で変える（E2E は 8081 の API に向ける）。
 * VITE_ を付けないので、ブラウザ側のコードには埋め込まれない。
 */
const apiProxyTarget = process.env.API_PROXY_TARGET || DEFAULT_API_PROXY_TARGET

/** server.host・preview.host に許す値（ループバック）。未設定（undefined）は Vite が localhost で待ち受ける。 */
const LOOPBACK_LISTEN_HOSTS: readonly (string | boolean | undefined)[] = [
  undefined,
  'localhost',
  '127.0.0.1',
  '::1',
]

/** API_PROXY_TARGET の URL に許すホスト名（URL の hostname の形。IPv6 は角かっこ付き）。 */
const LOOPBACK_URL_HOSTNAMES: readonly string[] = ['localhost', '127.0.0.1', '[::1]']

/** API_PROXY_TARGET の URL に許すスキーム。 */
const PROXY_TARGET_PROTOCOLS: readonly string[] = ['http:', 'https:']

/**
 * Vite が allowedHosts にホストを足すための環境変数（Vite の内部の仕組み）。
 * 空でない値が設定されていると、Vite はその値を server.allowedHosts の末尾に足す（Vite 6.4.3 の resolveServerOptions で確認）。
 */
const ADDITIONAL_ALLOWED_HOSTS_ENV = '__VITE_ADDITIONAL_SERVER_ALLOWED_HOSTS'

/** value がループバックの http(s) の URL か。URL として読めなければ false。 */
function isLoopbackHttpUrl(value: string): boolean {
  let url: URL
  try {
    url = new URL(value)
  } catch {
    return false
  }
  return PROXY_TARGET_PROTOCOLS.includes(url.protocol) && LOOPBACK_URL_HOSTNAMES.includes(url.hostname)
}

/**
 * dev サーバー・preview サーバーを、この PC の中（ループバック）だけで使う設定でなければ、起動時にエラーにする。
 * - server.host・preview.host がループバック以外（--host を含む）: Vite を LAN に公開すると、changeOrigin: true の /api の転送を通して、
 *   127.0.0.1 だけで待ち受ける API（プラン 6章の判断11・12）の資産データを、ほかの PC から読めてしまうため。
 * - API_PROXY_TARGET がループバックの http(s) の URL でない: 転送先が外部のサーバーだと、ブラウザの要求（Cookie などを含む）を
 *   外部に送ってしまい、API の 127.0.0.1 限定の前提も崩れるため。形の誤り（スキームのない値など）も、ここで気づけるようにする。
 * - server.allowedHosts・preview.allowedHosts が空でない（true を含む）、または環境変数 __VITE_ADDITIONAL_SERVER_ALLOWED_HOSTS が設定されている:
 *   Vite のポートへの DNS リバインディングは、Vite の allowedHosts の既定（localhost・*.localhost・IP アドレスだけを許す）が防ぐ前提で、
 *   changeOrigin: true の /api の転送は Host を 127.0.0.1:<port> に書き換えて API の Host の許可リストを通してしまう（プラン 6章の判断12）。
 *   ホストを足すと、そのホスト名を攻撃者の DNS で 127.0.0.1 に向けたサイトから、/api の転送を通して資産データを読めてしまうため。
 * （PR④ ステップ6 の S-2・EC-10、2ラウンド目の EC-14、プラン 4.5）
 */
function requireLoopbackOnly(): Plugin {
  return {
    name: 'zero:require-loopback-only',
    configResolved(config) {
      // ビルドはサーバーを起動せず、転送もしないので確かめない。
      if (config.command === 'build') {
        return
      }
      const listenHosts = { 'server.host': config.server.host, 'preview.host': config.preview.host }
      for (const [option, host] of Object.entries(listenHosts)) {
        if (!LOOPBACK_LISTEN_HOSTS.includes(host)) {
          throw new Error(
            `${option} がループバック（localhost・127.0.0.1・::1）ではありません: ${JSON.stringify(host)}。` +
              '/api の転送を通して API の資産データを LAN に公開してしまうので、起動しません。',
          )
        }
      }
      if (!isLoopbackHttpUrl(apiProxyTarget)) {
        throw new Error(
          `API_PROXY_TARGET がループバック（localhost・127.0.0.1・[::1]）の http(s) の URL ではありません: ${JSON.stringify(apiProxyTarget)}。` +
            '/api の転送先は、この PC の API サーバー（例 http://127.0.0.1:8080）にしてください。',
        )
      }
      // 環境変数を allowedHosts より先に確かめる。Vite はこの値を server.allowedHosts に足すので、
      // 先に allowedHosts で止めると、原因が環境変数だと分かりにくいため。
      // Vite は空の値を無視するが、ここでは空でも設定されていればエラーにする（設定の誤りに気づけるように）。
      const additionalAllowedHosts = process.env[ADDITIONAL_ALLOWED_HOSTS_ENV]
      if (additionalAllowedHosts !== undefined) {
        throw new Error(
          `環境変数 ${ADDITIONAL_ALLOWED_HOSTS_ENV} が設定されています: ${JSON.stringify(additionalAllowedHosts)}。` +
            'Vite の allowedHosts にホストを足すと、DNS リバインディングで /api の転送を通して資産データを読まれるので、起動しません。',
        )
      }
      // 未設定のとき Vite は server.allowedHosts を空の配列にし、preview.allowedHosts は server.allowedHosts を引き継ぐ
      // （Vite 6.4.3 の resolveServerOptions・resolvePreviewOptions で確認）。空の配列だけを既定として許す。
      const allowedHosts = {
        'server.allowedHosts': config.server.allowedHosts,
        'preview.allowedHosts': config.preview.allowedHosts,
      }
      for (const [option, hosts] of Object.entries(allowedHosts)) {
        if (hosts === true || hosts.length > 0) {
          throw new Error(
            `${option} が既定（空）ではありません: ${JSON.stringify(hosts)}。` +
              'Vite の allowedHosts にホストを足すと、DNS リバインディングで /api の転送を通して資産データを読まれるので、起動しません。',
          )
        }
      }
    },
  }
}

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), requireLoopbackOnly()],
  server: {
    // server.host と server.allowedHosts は既定のまま（localhost で待ち受け、localhost・IP アドレス以外の Host を拒む）。
    // host や allowedHosts を変えて起動しようとすると requireLoopbackOnly がエラーにする。
    // changeOrigin: true で API へ転送する Host が 127.0.0.1:<port> になり、API の Host の許可リストを通る。
    // そのため Vite のポートへの DNS リバインディングは、Vite の allowedHosts が防ぐ（プラン 6章の判断12）。
    proxy: {
      '/api': {
        target: apiProxyTarget,
        changeOrigin: true,
      },
    },
    // CORS を無効にする（CORS のヘッダーを付けない）。Vite の既定の CORS は localhost の任意のポートのオリジンを許し、
    // /api の転送の応答にも access-control-allow-origin を付けるので、同じ PC の別の localhost のページ（ほかの開発サーバーなど）から
    // 資産データを読めてしまうため。画面は同じオリジンから /api を呼ぶので、CORS は要らない（プラン 6章の判断21、PR④ ステップ6 の S-1）。
    // preview.cors は既定で server.cors と同じ値になる。
    cors: false,
  },
  test: {
    // フロントエンドのテストは frontend/test/ に置く（エージェント向けREADME.md のテスト方針）。
    // 既定の環境は node。DOM が要るテストはファイルの先頭の // @vitest-environment jsdom で切り替える。
    include: ['test/**/*.test.{ts,tsx}'],
  },
})
